package server

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

// Error codes the callback hands back to the app as ?error=<code>. Kept
// short and provider-neutral so the app can map each to its own page.
const (
	// The user cancelled or declined the sign-in.
	ErrCodeAccessDenied = "access_denied"
	// The user's organisation must approve the app before anyone in it can
	// sign in (Microsoft Entra admin consent).
	ErrCodeConsentRequired = "consent_required"
	// Anything else: the provider failed, the code exchange failed, the ID
	// token did not verify.
	ErrCodeProviderError = "provider_error"
)

// Microsoft Entra reports "an admin must approve this app" through a few
// AADSTS codes in error_description rather than a dedicated error value.
var consentRequiredMarkers = []string{
	"AADSTS65001", // the user or administrator has not consented
	"AADSTS90094", // the grant requires admin permission
	"AADSTS90095", // admin consent is required (admin consent workflow)
}

func classifyProviderError(errorCode, description string) string {
	if errorCode == "consent_required" {
		return ErrCodeConsentRequired
	}
	for _, marker := range consentRequiredMarkers {
		if strings.Contains(description, marker) {
			return ErrCodeConsentRequired
		}
	}
	if errorCode == "access_denied" {
		return ErrCodeAccessDenied
	}
	return ErrCodeProviderError
}

// redirectWithError clears the state cookie and sends the user back to the
// app's redirect URL with ?error=<code>. Only call it once the redirect URL
// has been read from the encrypted state cookie.
func (s *Server) redirectWithError(w http.ResponseWriter, r *http.Request, redirectURL, code string) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.cookieName,
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isSecure(r),
		SameSite: http.SameSiteLaxMode,
	})

	parsedURL, err := url.Parse(redirectURL)
	if err != nil {
		slog.Debug("failed to parse redirect URL", "error", err)
		http.Error(w, "Authentication failed: "+code, http.StatusBadRequest)
		return
	}
	q := parsedURL.Query()
	q.Set("error", code)
	parsedURL.RawQuery = q.Encode()
	http.Redirect(w, r, parsedURL.String(), http.StatusSeeOther)
}
