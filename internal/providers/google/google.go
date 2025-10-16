package google

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/coreos/go-oidc"
	"golang.org/x/oauth2"
	goauth "golang.org/x/oauth2/google"

	"github.com/iamolegga/lana/internal/config"
	"github.com/iamolegga/lana/internal/oauth"
)

type Provider struct {
	config         *oauth2.Config
	provider       *oidc.Provider
	providerConfig *config.OAuthProvider
}

func New(providerConfig *config.OAuthProvider) (oauth.Provider, error) {
	oauthConfig := &oauth2.Config{
		ClientID:     providerConfig.ClientID,
		ClientSecret: providerConfig.ClientSecret,
		Scopes:       []string{"email", "profile"},
		Endpoint:     goauth.Endpoint,
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}

	ctx := context.Background()

	issuerURL := "https://accounts.google.com"

	ctxWithClient := oidc.ClientContext(ctx, httpClient)

	provider, err := oidc.NewProvider(ctxWithClient, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize OIDC provider: %w", err)
	}

	return &Provider{
		config:         oauthConfig,
		provider:       provider,
		providerConfig: providerConfig,
	}, nil
}

func (p *Provider) GetAuthURL(state string, redirectURL string) string {
	configCopy := *p.config
	configCopy.RedirectURL = redirectURL

	slog.Debug("generating authorization url", "provider", "google", "redirect_uri", redirectURL)
	return configCopy.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
}

func (p *Provider) ExchangeCode(ctx context.Context, code string, redirectURL string) (*oauth.TokenResponse, error) {
	configCopy := *p.config
	configCopy.RedirectURL = redirectURL

	exchangeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	slog.Debug("exchanging authorization code for token", "provider", "google")
	token, err := configCopy.Exchange(exchangeCtx, code)
	if err != nil {
		slog.Error("failed to exchange auth code for token", "provider", "google", "error", err)
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
		slog.Error("id_token not found in OAuth response", "provider", "google")
		return nil, errors.New("id_token not found in OAuth response")
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

	verifier := p.provider.Verifier(&oidc.Config{
		ClientID: p.providerConfig.ClientID,
	})

	idToken, err := verifier.Verify(verifyCtx, tokens.IDToken)
	if err != nil {
		slog.Error("failed to verify ID token", "provider", "google", "error", err)
		return nil, fmt.Errorf("failed to verify ID token: %w", err)
	}

	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		Sub           string `json:"sub"`
	}

	if err := idToken.Claims(&claims); err != nil {
		slog.Error("failed to parse claims", "provider", "google", "error", err)
		return nil, fmt.Errorf("failed to parse claims: %w", err)
	}

	if !claims.EmailVerified {
		slog.Debug("user's email is not verified", "email", claims.Email)
		return nil, errors.New("email not verified")
	}

	user := &oauth.User{
		Email: claims.Email,
		Name:  claims.Name,
		ID:    claims.Sub,
	}

	if (claims.Name == "" || claims.Email == "") && tokens.AccessToken != "" {
		slog.Debug("fetching user info", "provider", "google")
		userInfo, err := p.fetchUserInfo(ctx, tokens.AccessToken)
		if err != nil {
			slog.Error("failed to fetch user info", "provider", "google", "error", err)
			return nil, fmt.Errorf("failed to fetch user info: %w", err)
		}
		if user.Email == "" && userInfo.Email != "" {
			user.Email = userInfo.Email
		}
		if user.Name == "" && userInfo.Name != "" {
			user.Name = userInfo.Name
		}
	}

	return user, nil
}

func (p *Provider) Name() string {
	return "google"
}

func (p *Provider) fetchUserInfo(ctx context.Context, accessToken string) (*struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}, error) {
	userinfoURL := "https://www.googleapis.com/oauth2/v3/userinfo"

	userinfoCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(userinfoCtx, http.MethodGet, userinfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo request failed: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		return nil, err
	}

	var userInfo struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return nil, err
	}

	return &userInfo, nil
}
