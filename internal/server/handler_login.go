package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/IGLOU-EU/go-wildcard"
)

func (s *Server) handlerLogin(w http.ResponseWriter, r *http.Request) {
	host, exists := s.hosts[r.Host]
	if !exists {
		http.Error(w, "Unknown host", http.StatusBadRequest)
		return
	}

	providerName := r.PathValue("provider")
	provider, providerExists := host.providers[providerName]
	if !providerExists {
		http.NotFound(w, r)
		return
	}

	redirectURLEncoded := r.URL.Query().Get("redirect")
	if redirectURLEncoded == "" {
		http.Error(w, "Missing redirect URL query parameter", http.StatusBadRequest)
		return
	}
	redirectURL, err := url.QueryUnescape(redirectURLEncoded)
	if err != nil {
		http.Error(w, "Invalid redirect URL query parameter", http.StatusBadRequest)
		return
	}

	allowed := false
	for _, pattern := range host.allowedRedirectURLs {
		if wildcard.Match(pattern, redirectURL) {
			allowed = true
			break
		}
	}
	if !allowed {
		http.Error(w, "Redirect URL not allowed", http.StatusBadRequest)
		return
	}

	state := generateRandomString(16)
	if state == "" {
		slog.Error("failed to generate random state")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	encryptedState, err := encryptState(
		[]byte(s.cookieSecret),
		state,
		redirectURLEncoded,
	)
	if err != nil {
		slog.Error("failed to encrypt state data", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     s.cookieName,
		Value:    encryptedState,
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecure(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   3600,
	})

	callbackURL := fmt.Sprintf("%s://%s/oauth/callback/%s",
		getScheme(r),
		r.Host,
		providerName,
	)

	authURL := provider.GetAuthURL(state, callbackURL)
	http.Redirect(w, r, authURL, http.StatusFound)
}
