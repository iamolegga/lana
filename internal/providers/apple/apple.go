package apple

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/coreos/go-oidc"
	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/endpoints"

	"github.com/iamolegga/lana/internal/config"
	"github.com/iamolegga/lana/internal/oauth"
)

type Provider struct {
	config         *oauth2.Config
	oidcProvider   *oidc.Provider
	providerConfig *config.OAuthProvider
	privateKey     *ecdsa.PrivateKey
}

func New(providerConfig *config.OAuthProvider) (oauth.Provider, error) {
	keyData, err := os.ReadFile(providerConfig.PrivateKeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read Apple private key file: %w", err)
	}

	block, _ := pem.Decode(keyData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block from Apple private key")
	}

	parsedKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Apple private key: %w", err)
	}

	ecKey, ok := parsedKey.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("Apple private key is not an ECDSA key")
	}

	oauthConfig := &oauth2.Config{
		ClientID: providerConfig.ServicesID,
		Scopes:   []string{"name", "email"},
		Endpoint: endpoints.Apple,
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}
	ctx := oidc.ClientContext(context.Background(), httpClient)

	oidcProvider, err := oidc.NewProvider(ctx, "https://appleid.apple.com")
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Apple OIDC provider: %w", err)
	}

	return &Provider{
		config:         oauthConfig,
		oidcProvider:   oidcProvider,
		providerConfig: providerConfig,
		privateKey:     ecKey,
	}, nil
}

func (p *Provider) GetAuthURL(state string, redirectURL string) (string, string) {
	configCopy := *p.config
	configCopy.RedirectURL = redirectURL

	slog.Debug("generating authorization url", "provider", "apple", "redirect_uri", redirectURL)
	return configCopy.AuthCodeURL(
		state,
		oauth2.SetAuthURLParam("response_mode", "form_post"),
		oauth2.SetAuthURLParam("response_type", "code"),
	), ""
}

func (p *Provider) ExchangeCode(ctx context.Context, code string, redirectURL string, _ string) (*oauth.TokenResponse, error) {
	clientSecret, err := p.generateClientSecret()
	if err != nil {
		return nil, fmt.Errorf("failed to generate client secret: %w", err)
	}

	configCopy := *p.config
	configCopy.RedirectURL = redirectURL
	configCopy.ClientSecret = clientSecret

	exchangeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	slog.Debug("exchanging authorization code for token", "provider", "apple")
	token, err := configCopy.Exchange(exchangeCtx, code)
	if err != nil {
		slog.Error("failed to exchange auth code for token", "provider", "apple", "error", err)
		return nil, fmt.Errorf("failed to exchange auth code: %w", err)
	}

	response := &oauth.TokenResponse{
		AccessToken:  token.AccessToken,
		TokenType:    token.TokenType,
		RefreshToken: token.RefreshToken,
		ExpiresIn:    int(time.Until(token.Expiry).Seconds()),
	}

	if idToken, ok := token.Extra("id_token").(string); ok {
		response.IDToken = idToken
	}

	return response, nil
}

func (p *Provider) GetUser(ctx context.Context, tokens *oauth.TokenResponse) (*oauth.User, error) {
	if tokens.IDToken == "" {
		slog.Error("id_token not found in OAuth response", "provider", "apple")
		return nil, fmt.Errorf("id_token not found in OAuth response")
	}

	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 5 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			DisableKeepAlives:     true,
			MaxIdleConnsPerHost:   -1,
		},
	}

	verifyCtx := oidc.ClientContext(ctx, httpClient)

	verifier := p.oidcProvider.Verifier(&oidc.Config{
		ClientID: p.providerConfig.ServicesID,
	})

	idToken, err := verifier.Verify(verifyCtx, tokens.IDToken)
	if err != nil {
		slog.Error("failed to verify ID token", "provider", "apple", "error", err)
		return nil, fmt.Errorf("failed to verify ID token: %w", err)
	}

	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Sub           string `json:"sub"`
	}

	if err := idToken.Claims(&claims); err != nil {
		slog.Error("failed to parse claims", "provider", "apple", "error", err)
		return nil, fmt.Errorf("failed to parse claims: %w", err)
	}

	email := claims.Email
	if !claims.EmailVerified {
		slog.Debug("user's email is not verified, omitting email", "provider", "apple", "email", claims.Email)
		email = ""
	}

	user := &oauth.User{
		ID:    claims.Sub,
		Email: email,
	}

	// Apple sends user name only on first authorization via form POST `user` field
	if tokens.RawUserInfo != "" {
		var appleUser struct {
			Name struct {
				FirstName string `json:"firstName"`
				LastName  string `json:"lastName"`
			} `json:"name"`
		}
		if err := json.Unmarshal([]byte(tokens.RawUserInfo), &appleUser); err != nil {
			slog.Debug("failed to parse Apple user info", "provider", "apple", "error", err)
		} else {
			name := strings.TrimSpace(appleUser.Name.FirstName + " " + appleUser.Name.LastName)
			if name != "" {
				user.Name = name
			}
		}
	}

	return user, nil
}

func (p *Provider) Name() string {
	return "apple"
}

func (p *Provider) generateClientSecret() (string, error) {
	now := time.Now()

	claims := jwt.MapClaims{
		"iss": p.providerConfig.TeamID,
		"sub": p.providerConfig.ServicesID,
		"aud": "https://appleid.apple.com",
		"iat": now.Unix(),
		"exp": now.Add(5 * time.Minute).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = p.providerConfig.KeyID

	signed, err := token.SignedString(p.privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign client secret JWT: %w", err)
	}

	return signed, nil
}
