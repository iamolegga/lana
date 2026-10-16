package microsoft

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/coreos/go-oidc"
	"golang.org/x/oauth2"

	"github.com/iamolegga/lana/internal/config"
	"github.com/iamolegga/lana/internal/oauth"
)

const (
	// The "common" endpoints accept both work/school accounts from any
	// Microsoft Entra tenant and personal Microsoft accounts.
	authURL  = "https://login.microsoftonline.com/common/oauth2/v2.0/authorize"
	tokenURL = "https://login.microsoftonline.com/common/oauth2/v2.0/token"
	jwksURL  = "https://login.microsoftonline.com/common/discovery/v2.0/keys"

	// Tenant that every personal Microsoft account (outlook.com, hotmail.com,
	// live.com, ...) signs in through.
	ConsumerTenantID = "9188040d-6c67-4c5b-b112-36a304b66dad"
)

type Provider struct {
	config   *oauth2.Config
	verifier *oidc.IDTokenVerifier
}

func New(providerConfig *config.OAuthProvider) (oauth.Provider, error) {
	httpClient := &http.Client{Timeout: 10 * time.Second}
	keySet := oidc.NewRemoteKeySet(
		oidc.ClientContext(context.Background(), httpClient),
		jwksURL,
	)

	// A multi-tenant app gets tokens whose issuer embeds the user's tenant,
	// so no single issuer URL can be configured up front. The issuer is
	// checked against the token's own tid claim in GetUser instead.
	verifier := oidc.NewVerifier("", keySet, &oidc.Config{
		ClientID:        providerConfig.ClientID,
		SkipIssuerCheck: true,
	})

	return &Provider{
		config: &oauth2.Config{
			ClientID:     providerConfig.ClientID,
			ClientSecret: providerConfig.ClientSecret,
			Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
			Endpoint: oauth2.Endpoint{
				AuthURL:   authURL,
				TokenURL:  tokenURL,
				AuthStyle: oauth2.AuthStyleInParams,
			},
		},
		verifier: verifier,
	}, nil
}

func (p *Provider) GetAuthURL(state string, redirectURL string) (string, string) {
	configCopy := *p.config
	configCopy.RedirectURL = redirectURL

	verifier := oauth2.GenerateVerifier()

	slog.Debug("generating authorization url", "provider", "microsoft", "redirect_uri", redirectURL)
	return configCopy.AuthCodeURL(
		state,
		oauth2.S256ChallengeOption(verifier),
		// Always show the account picker: people often have a personal and
		// a work account signed in, and a team seat needs the work one.
		oauth2.SetAuthURLParam("prompt", "select_account"),
	), verifier
}

func (p *Provider) ExchangeCode(ctx context.Context, code string, redirectURL string, codeVerifier string) (*oauth.TokenResponse, error) {
	configCopy := *p.config
	configCopy.RedirectURL = redirectURL

	exchangeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	slog.Debug("exchanging authorization code for token", "provider", "microsoft")
	token, err := configCopy.Exchange(exchangeCtx, code, oauth2.VerifierOption(codeVerifier))
	if err != nil {
		slog.Error("failed to exchange auth code for token", "provider", "microsoft", "error", err)
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

// flexBool accepts a JSON boolean as well as the "true"/"1" strings some
// optional claims have been seen to arrive as.
type flexBool bool

func (b *flexBool) UnmarshalJSON(data []byte) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	switch t := v.(type) {
	case bool:
		*b = flexBool(t)
	case string:
		*b = flexBool(t == "true" || t == "1")
	case float64:
		*b = flexBool(t == 1)
	}
	return nil
}

func (p *Provider) GetUser(ctx context.Context, tokens *oauth.TokenResponse) (*oauth.User, error) {
	if tokens.IDToken == "" {
		slog.Error("id_token not found in OAuth response", "provider", "microsoft")
		return nil, errors.New("id_token not found in OAuth response")
	}

	idToken, err := p.verifier.Verify(ctx, tokens.IDToken)
	if err != nil {
		slog.Error("failed to verify ID token", "provider", "microsoft", "error", err)
		return nil, fmt.Errorf("failed to verify ID token: %w", err)
	}

	var claims struct {
		Issuer string   `json:"iss"`
		Tenant string   `json:"tid"`
		Object string   `json:"oid"`
		Sub    string   `json:"sub"`
		Email  string   `json:"email"`
		Name   string   `json:"name"`
		EDOV   flexBool `json:"xms_edov"`
	}
	if err := idToken.Claims(&claims); err != nil {
		slog.Error("failed to parse claims", "provider", "microsoft", "error", err)
		return nil, fmt.Errorf("failed to parse claims: %w", err)
	}

	if claims.Tenant == "" || claims.Issuer != "https://login.microsoftonline.com/"+claims.Tenant+"/v2.0" {
		slog.Error("unexpected issuer", "provider", "microsoft", "iss", claims.Issuer, "tid", claims.Tenant)
		return nil, fmt.Errorf("unexpected issuer %q for tenant %q", claims.Issuer, claims.Tenant)
	}

	// The email claim of a work account is whatever the tenant admin typed
	// into the user's profile, so anyone who controls a tenant could claim
	// someone else's address. Trust it only when Microsoft says the tenant
	// owns the domain (xms_edov), or for personal accounts, whose addresses
	// Microsoft verifies itself.
	email := claims.Email
	if email != "" && !bool(claims.EDOV) && claims.Tenant != ConsumerTenantID {
		slog.Debug("email domain not verified, omitting email", "provider", "microsoft", "tid", claims.Tenant)
		email = ""
	}

	id := claims.Object
	if id == "" {
		id = claims.Sub
	}

	return &oauth.User{
		Email:    email,
		Name:     claims.Name,
		ID:       id,
		TenantID: claims.Tenant,
	}, nil
}

func (p *Provider) Name() string {
	return "microsoft"
}
