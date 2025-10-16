package server

import (
	"log/slog"
	"math/big"
	"net/http"
)

func (s *Server) handlerJwks(w http.ResponseWriter, r *http.Request) {
	host, exists := s.hosts[r.Host]
	if !exists {
		http.Error(w, "Unknown host", http.StatusBadRequest)
		return
	}

	pubKey := host.signKey.PublicKey

	n := base64URL(pubKey.N.Bytes())
	e := base64URL(big.NewInt(int64(pubKey.E)).Bytes())

	jwk := map[string]any{
		"kty": "RSA",
		"use": "sig",
		"kid": host.jwtKeyID,
		"alg": "RS256",
		"n":   n,
		"e":   e,
	}

	jwks := map[string]any{
		"keys": []any{jwk},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")

	if err := writeJSON(w, jwks); err != nil {
		slog.Error("failed to write JWKS response", "error", err)
	}
}
