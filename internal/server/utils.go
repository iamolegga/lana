package server

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
)

func getScheme(r *http.Request) string {
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		return proto
	}

	if scheme := r.Header.Get("X-Scheme"); scheme != "" {
		return scheme
	}

	if r.TLS != nil {
		return "https"
	}

	return "http"
}

func isSecure(r *http.Request) bool {
	return getScheme(r) == "https"
}

func generateRandomString(length int) string {
	if length <= 0 {
		return ""
	}

	b := make([]byte, length)

	if _, err := rand.Read(b); err != nil {
		return ""
	}

	encoded := base64.URLEncoding.EncodeToString(b)

	if len(encoded) > length {
		return encoded[:length]
	}

	return encoded
}

func base64URL(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func writeJSON(w http.ResponseWriter, data any) error {
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(data); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error": "Internal server error"}`)
		return err
	}

	return nil
}
