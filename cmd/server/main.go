package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/iamolegga/lana/internal/config"
	"github.com/iamolegga/lana/internal/logging"
	"github.com/iamolegga/lana/internal/metrics"
	"github.com/iamolegga/lana/internal/oauth"
	"github.com/iamolegga/lana/internal/providers/apple"
	"github.com/iamolegga/lana/internal/providers/facebook"
	"github.com/iamolegga/lana/internal/providers/google"
	xprovider "github.com/iamolegga/lana/internal/providers/x"
	"github.com/iamolegga/lana/internal/ratelimit"
	"github.com/iamolegga/lana/internal/server"
)

var configPath string

func init() {
	flag.StringVar(
		&configPath,
		"config",
		"config.yaml",
		"path to the config file",
	)
	flag.Parse()
}

func main() {
	server.SubscribeForShutdown()

	cfg, err := config.New(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	logging.Setup(cfg.Logging.Level, cfg.Logging.Format, cfg.Env)
	slog.Info("starting auth server")

	// Initialize metrics system. When enabled, collectors are registered and the
	// observability listener exposes /metrics.
	metrics.Init(cfg.Observability.Metrics.Enabled, cfg.Observability.Metrics.GoMetrics)

	limiterConfig := ratelimit.Config{
		RequestsPerMinute:  cfg.RateLimit.RequestsPerMinute,
		CleanupInterval:    cfg.RateLimit.CleanupInterval,
		XForwardedForIndex: cfg.RateLimit.XForwardedForIndex,
	}
	limiter := ratelimit.New(server.GetServerBaseContext(), limiterConfig, nil)

	registry := oauth.NewRegistry()
	registry.Register("google", google.New)
	registry.Register("facebook", facebook.New)
	registry.Register("x", xprovider.New)
	registry.Register("apple", apple.New)

	srv, err := server.New(server.Config{
		Config:      cfg,
		RateLimiter: limiter,
		Registry:    registry,
	})
	if err != nil {
		slog.Error("failed to initialize server", "error", err)
		os.Exit(1)
	}

	go func() {
		slog.Info("starting HTTP server", "port", cfg.Server.Port)
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	adminHTTP := server.NewAdminServer(cfg, srv.LoginDirs())
	if adminHTTP != nil {
		go func() {
			if err := server.StartAdmin(adminHTTP); err != nil && err != http.ErrServerClosed {
				slog.Error("admin server failed", "error", err)
				os.Exit(1)
			}
		}()
	}

	obsHTTP := server.NewObservabilityServer(cfg)
	go func() {
		if err := server.StartObservability(obsHTTP); err != nil && err != http.ErrServerClosed {
			slog.Error("observability server failed", "error", err)
			os.Exit(1)
		}
	}()

	slog.Info("server started successfully", "port", cfg.Server.Port)
	server.WaitForShutdown(srv.GetHTTPServer(), adminHTTP, obsHTTP)
}
