package facebook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/facebook"

	"github.com/iamolegga/lana/internal/config"
	"github.com/iamolegga/lana/internal/oauth"
)

type Provider struct {
	config         *oauth2.Config
	providerConfig *config.OAuthProvider
}

type FacebookUser struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func New(providerConfig *config.OAuthProvider) (oauth.Provider, error) {
	oauthConfig := &oauth2.Config{
		ClientID:     providerConfig.ClientID,
		ClientSecret: providerConfig.ClientSecret,
		Scopes:       []string{"email", "public_profile"},
		Endpoint:     facebook.Endpoint,
	}

	return &Provider{
		config:         oauthConfig,
		providerConfig: providerConfig,
	}, nil
}

func (p *Provider) GetAuthURL(state string, redirectURL string) string {
	configCopy := *p.config
	configCopy.RedirectURL = redirectURL

	slog.Debug("generating authorization url", "provider", "facebook", "redirect_uri", redirectURL)
	return configCopy.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

func (p *Provider) ExchangeCode(ctx context.Context, code string, redirectURL string) (*oauth.TokenResponse, error) {
	configCopy := *p.config
	configCopy.RedirectURL = redirectURL

	exchangeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	slog.Debug("exchanging authorization code for token", "provider", "facebook")
	token, err := configCopy.Exchange(exchangeCtx, code)
	if err != nil {
		slog.Error("failed to exchange auth code for token", "provider", "facebook", "error", err)
		return nil, fmt.Errorf("failed to exchange auth code: %w", err)
	}

	response := &oauth.TokenResponse{
		AccessToken:  token.AccessToken,
		TokenType:    token.TokenType,
		RefreshToken: token.RefreshToken,
		ExpiresIn:    int(time.Until(token.Expiry).Seconds()),
	}

	return response, nil
}

func (p *Provider) GetUser(ctx context.Context, tokens *oauth.TokenResponse) (*oauth.User, error) {
	userCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	slog.Debug("fetching user info from facebook", "provider", "facebook")

	userInfo, err := p.fetchUserInfo(userCtx, tokens.AccessToken)
	if err != nil {
		slog.Error("failed to fetch user info", "provider", "facebook", "error", err)
		return nil, fmt.Errorf("failed to fetch user info: %w", err)
	}

	if userInfo.Email == "" {
		slog.Error("facebook user missing email", "provider", "facebook", "user_id", userInfo.ID)
		return nil, fmt.Errorf("user email not available from Facebook")
	}

	if userInfo.ID == "" {
		slog.Error("facebook user missing ID", "provider", "facebook")
		return nil, fmt.Errorf("user ID not available from Facebook")
	}

	user := &oauth.User{
		ID:    userInfo.ID,
		Email: userInfo.Email,
		Name:  userInfo.Name,
	}

	slog.Debug("successfully retrieved user info",
		"provider", "facebook",
		"user_id", user.ID,
		"email", user.Email,
		"name", user.Name)

	return user, nil
}

func (p *Provider) Name() string {
	return "facebook"
}

func (p *Provider) fetchUserInfo(ctx context.Context, accessToken string) (*FacebookUser, error) {
	url := "https://graph.facebook.com/me?fields=id,name,email&access_token=" + accessToken

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		slog.Error("facebook api error", "status", resp.StatusCode, "body", string(body))
		return nil, fmt.Errorf("facebook API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	var userInfo FacebookUser
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return nil, fmt.Errorf("unmarshaling user info: %w", err)
	}

	return &userInfo, nil
}
