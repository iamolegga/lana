package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/iamolegga/lana/internal/metrics"
)

func (s *Server) handlerCallback(w http.ResponseWriter, r *http.Request) {
	host, exists := s.hosts[r.Host]
	if !exists {
		http.Error(w, "Unknown host", http.StatusBadRequest)
		return
	}

	providerName := r.PathValue("provider")
	provider, providerExists := host.providers[providerName]
	if !providerExists {
		slog.Debug("callback for unknown provider",
			"provider", providerName,
			"host", r.Host,
		)
		http.NotFound(w, r)
		return
	}

	cookie, err := r.Cookie(s.cookieName)
	if err != nil {
		slog.Debug("missing state cookie")
		http.Error(w, "Missing state cookie", http.StatusBadRequest)
		return
	}

	expectedState, redirectURL, err := decryptState(
		[]byte(s.cookieSecret),
		cookie.Value,
	)
	if err != nil {
		slog.Debug("invalid state cookie", "error", err)
		http.Error(w, "Invalid state cookie", http.StatusBadRequest)
		return
	}

	if expectedState == "" || redirectURL == "" {
		slog.Error("missing data in state cookie")
		http.Error(w, "Invalid state cookie", http.StatusBadRequest)
		return
	}

	actualState := r.URL.Query().Get("state")
	if actualState != expectedState {
		slog.Debug("state mismatch",
			"expected", expectedState,
			"actual", actualState,
		)
		metrics.RecordAuthentication(
			providerName,
			r.Host,
			"failure",
			"state_mismatch",
		)
		http.Error(w, "State mismatch", http.StatusBadRequest)
		return
	}

	if errorMsg := r.URL.Query().Get("error"); errorMsg != "" {
		slog.Debug("OAuth error response",
			"error", errorMsg,
			"description", r.URL.Query().Get("error_description"),
		)
		metrics.RecordAuthentication(providerName, r.Host, "failure", "user_denied")
		http.Error(w, "Authentication failed: "+errorMsg, http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Missing code parameter", http.StatusBadRequest)
		return
	}

	callbackURL := fmt.Sprintf("%s://%s/oauth/callback/%s",
		getScheme(r),
		r.Host,
		providerName,
	)

	tokens, err := provider.ExchangeCode(r.Context(), code, callbackURL)
	if err != nil {
		slog.Debug("token exchange failed", "error", err)
		metrics.RecordAuthentication(
			providerName,
			r.Host,
			"failure",
			"provider_error",
		)
		http.Error(
			w,
			"Failed to exchange authorization code",
			http.StatusBadRequest,
		)
		return
	}

	user, err := provider.GetUser(r.Context(), tokens)
	if err != nil {
		slog.Debug("failed to get user info", "error", err)
		metrics.RecordAuthentication(
			providerName,
			r.Host,
			"failure",
			"provider_error",
		)
		http.Error(w, "Failed to get user information", http.StatusUnauthorized)
		return
	}

	if user.Email == "" {
		metrics.RecordAuthentication(
			providerName,
			r.Host,
			"failure",
			"email_missing",
		)
		http.Error(w, "User email not available", http.StatusUnauthorized)
		return
	}

	jwtClaims := jwt.MapClaims{
		"iss":   fmt.Sprintf("%s://%s", getScheme(r), r.Host),
		"aud":   host.jwtAudience,
		"sub":   user.Email,
		"email": user.Email,
		"exp":   time.Now().Add(host.jwtExpiry).Unix(),
		"iat":   time.Now().Unix(),
	}

	if user.Name != "" {
		jwtClaims["name"] = user.Name
	}

	appToken := jwt.NewWithClaims(jwt.SigningMethodRS256, jwtClaims)
	appToken.Header["kid"] = host.jwtKeyID

	signedToken, err := appToken.SignedString(host.signKey)
	if err != nil {
		slog.Debug("failed to sign JWT", "error", err)
		http.Error(
			w,
			"Failed to create authentication token",
			http.StatusInternalServerError,
		)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     s.cookieName,
		Path:     "/",
		MaxAge:   -1, // Delete the cookie
		HttpOnly: true,
		Secure:   isSecure(r),
		SameSite: http.SameSiteLaxMode,
	})

	parsedURL, err := url.Parse(redirectURL)
	if err != nil {
		slog.Debug("failed to parse redirect URL", "error", err)
		http.Error(w, "Failed to parse redirect URL", http.StatusBadRequest)
		return
	}
	q := parsedURL.Query()
	q.Add("token", signedToken)
	parsedURL.RawQuery = q.Encode()
	finalRedirectURL := parsedURL.String()

	metrics.RecordAuthentication(providerName, r.Host, "success", "")
	http.Redirect(w, r, finalRedirectURL, http.StatusSeeOther)
}
