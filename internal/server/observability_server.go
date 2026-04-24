package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/iamolegga/lana/internal/config"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// NewObservabilityServer builds a dedicated http.Server for operator endpoints
// on a port that is NOT exposed via the public Ingress. /healthz is always
// registered (kubelet needs it); /metrics is registered only when metrics are
// enabled in config, so scrapes never hit a 404 when metrics are off.
//
// The handler is the bare mux with no logging or metrics middleware: scrape
// traffic should not self-report, and we don't want to spam the access log
// with kubelet probe lines.
func NewObservabilityServer(cfg config.Config) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("GET /healthz", http.HandlerFunc(healthCheckHandler))
	if cfg.Observability.Metrics.Enabled {
		mux.Handle("GET /metrics", promhttp.Handler())
	}

	addr := fmt.Sprintf(":%d", cfg.Observability.Port)
	return &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
		BaseContext: func(_ net.Listener) context.Context {
			return GetServerBaseContext()
		},
	}
}

// StartObservability blocks on ListenAndServe for the observability server.
func StartObservability(server *http.Server) error {
	slog.Info("starting observability server", "addr", server.Addr)
	return server.ListenAndServe()
}
