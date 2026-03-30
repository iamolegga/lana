package x

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/endpoints"

	"github.com/iamolegga/lana/internal/config"
	"github.com/iamolegga/lana/internal/oauth"
)

type Provider struct {
	config         *oauth2.Config
	providerConfig *config.OAuthProvider
}

type xUser struct {
	Data struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		ConfirmedEmail string `json:"confirmed_email"`
	} `json:"data"`
}

func New(providerConfig *config.OAuthProvider) (oauth.Provider, error) {
	oauthConfig := &oauth2.Config{
		ClientID:     providerConfig.ClientID,
		ClientSecret: providerConfig.ClientSecret,
		Scopes:       []string{"users.read", "tweet.read", "users.email"},
		Endpoint:     endpoints.X,
	}

	return &Provider{
		config:         oauthConfig,
		providerConfig: providerConfig,
	}, nil
}

func (p *Provider) GetAuthURL(
	state string,
	redirectURL string,
) (string, string) {
	configCopy := *p.config
	configCopy.RedirectURL = redirectURL

	verifier := oauth2.GenerateVerifier()

	slog.Debug(
		"generating authorization url",
		"provider",
		"x",
		"redirect_uri",
		redirectURL,
	)
	return configCopy.AuthCodeURL(
		state,
		oauth2.S256ChallengeOption(verifier),
	), verifier
}

func (p *Provider) ExchangeCode(
	ctx context.Context,
	code string,
	redirectURL string,
	codeVerifier string,
) (*oauth.TokenResponse, error) {
	configCopy := *p.config
	configCopy.RedirectURL = redirectURL

	exchangeCtx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	slog.Debug("exchanging authorization code for token", "provider", "x")
	token, err := configCopy.Exchange(
		exchangeCtx,
		code,
		oauth2.VerifierOption(codeVerifier),
	)
	if err != nil {
		slog.Error(
			"failed to exchange auth code for token",
			"provider",
			"x",
			"error",
			err,
		)
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

func (p *Provider) GetUser(
	ctx context.Context,
	tokens *oauth.TokenResponse,
) (*oauth.User, error) {
	userCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	slog.Debug("fetching user info from x", "provider", "x")

	userInfo, err := p.fetchUserInfo(userCtx, tokens.AccessToken)
	if err != nil {
		slog.Error("failed to fetch user info", "provider", "x", "error", err)
		return nil, fmt.Errorf("failed to fetch user info: %w", err)
	}

	if userInfo.Data.ID == "" {
		slog.Error("x user missing ID", "provider", "x")
		return nil, fmt.Errorf("user ID not available from X")
	}

	user := &oauth.User{
		ID:    userInfo.Data.ID,
		Email: userInfo.Data.ConfirmedEmail,
		Name:  userInfo.Data.Name,
	}

	slog.Debug("successfully retrieved user info",
		"provider", "x",
		"user_id", user.ID,
		"name", user.Name)

	return user, nil
}

func (p *Provider) Name() string {
	return "x"
}

func (p *Provider) fetchUserInfo(
	ctx context.Context,
	accessToken string,
) (*xUser, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		"https://api.x.com/2/users/me?user.fields=id,name,confirmed_email",
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		slog.Error("x api error", "status", resp.StatusCode, "body", string(body))
		return nil, fmt.Errorf("X API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	var userInfo xUser
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return nil, fmt.Errorf("unmarshaling user info: %w", err)
	}

	return &userInfo, nil
}
