package oauth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/iamolegga/lana/internal/config"
)

type User struct {
	Email string
	Name  string
	ID    string
}

type TokenResponse struct {
	AccessToken  string
	TokenType    string
	RefreshToken string
	IDToken      string
	ExpiresIn    int
}

type Provider interface {
	GetAuthURL(state string, redirectURL string) string

	ExchangeCode(ctx context.Context, code string, redirectURL string) (*TokenResponse, error)

	GetUser(ctx context.Context, tokens *TokenResponse) (*User, error)

	Name() string
}

type Factory func(providerConfig *config.OAuthProvider) (Provider, error)

type Registry struct {
	factories map[string]Factory
}

func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]Factory),
	}
}

func (r *Registry) Register(name string, factory Factory) {
	r.factories[name] = factory
	slog.Debug("registered provider", "name", name)
}

func (r *Registry) Create(providerType string, providerConfig *config.OAuthProvider) (Provider, error) {
	factory, exists := r.factories[providerType]
	if !exists {
		slog.Error("unknown provider type", "type", providerType)
		return nil, errors.New("unknown provider type: " + providerType)
	}

	provider, err := factory(providerConfig)
	if err != nil {
		return nil, err
	}

	slog.Debug("created provider instance", "type", providerType)
	return provider, nil
}

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}
