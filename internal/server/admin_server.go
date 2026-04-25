package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/iamolegga/lana/internal/config"
	"github.com/iamolegga/lana/internal/logging"
)

// NewAdminServer builds a second http.Server that exposes operator endpoints
// (currently: POST /admin/login-assets/{host}) on a dedicated port. It is
// intended to be bound to a ClusterIP Service with no Ingress — access is
// gated by Kubernetes RBAC (port-forward / exec), not by application-level
// auth.
//
// loginDirs maps each configured host to its login directory on disk.
//
// Returns nil when the admin feature is disabled.
func NewAdminServer(cfg config.Config, loginDirs map[string]string) *http.Server {
	if !cfg.Admin.Enabled {
		return nil
	}

	mux := http.NewServeMux()
	mux.Handle(
		"POST /admin/login-assets/{host}",
		handlerAdminLoginAssetsUpload(loginDirs),
	)

	var handler http.Handler = mux
	handler = logging.Middleware(handler)
	handler = metricsMiddleware(handler, classifyAdminPath)

	addr := fmt.Sprintf(":%d", cfg.Admin.Port)
	return &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  0, // uploads can be large; no read timeout for the admin port
		WriteTimeout: 0,
		IdleTimeout:  60 * time.Second,
		BaseContext: func(_ net.Listener) context.Context {
			return GetServerBaseContext()
		},
	}
}

// classifyAdminPath maps a request on the admin listener to a bounded
// `path` label value. The only registered route is the login-asset upload;
// anything else is bucketed as "unknown" to keep label cardinality bounded.
func classifyAdminPath(r *http.Request) string {
	if strings.HasPrefix(r.URL.Path, "/admin/login-assets/") {
		return r.URL.Path
	}
	return "unknown"
}

// StartAdmin blocks on ListenAndServe for the given admin server. Returns
// nil if server is nil (admin not enabled).
func StartAdmin(server *http.Server) error {
	if server == nil {
		return nil
	}
	slog.Info("starting admin server", "addr", server.Addr)
	return server.ListenAndServe()
}
