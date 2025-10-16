package server

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"log/slog"
	"net"

	"net/http"

	"time"

	"github.com/iamolegga/lana/internal/config"
	"github.com/iamolegga/lana/internal/logging"
	"github.com/iamolegga/lana/internal/oauth"
	"github.com/iamolegga/lana/internal/ratelimit"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type hostData struct {
	allowedRedirectURLs []string
	loginDir            string
	jwtAudience         string
	jwtExpiry           time.Duration
	jwtKeyID            string
	providers           map[string]oauth.Provider
	signKey             *rsa.PrivateKey
}

type Server struct {
	cookieName    string
	cookieSecret  string
	serverPort    string
	rateLimiter   ratelimit.Limiter
	hosts         map[string]*hostData
	httpServer    *http.Server
	metricsConfig config.Config
}

type Config struct {
	Config      config.Config
	RateLimiter ratelimit.Limiter
	Registry    *oauth.Registry
}

func New(cfg Config) (*Server, error) {
	if cfg.RateLimiter == nil {
		return nil, errors.New("rate limiter is required")
	}

	if cfg.Registry == nil {
		return nil, errors.New("OAuth registry is required")
	}

	if len(cfg.Config.Hosts) == 0 {
		return nil, errors.New("at least one host is required")
	}

	hosts := make(map[string]*hostData)
	for hostname, hostConfig := range cfg.Config.Hosts {
		signKey, err := loadSigningKey(hostConfig.JWT.PrivateKeyFile)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to load signing key for host %s: %w",
				hostname,
				err,
			)
		}

		providers := make(map[string]oauth.Provider)
		for providerName, providerConfig := range hostConfig.Providers {
			provider, err := cfg.Registry.Create(providerName, &providerConfig)
			if err != nil {
				slog.Error("failed to create provider",
					"provider", providerName,
					"host", hostname,
					"error", err,
				)
				continue
			}

			providers[providerName] = provider
			slog.Info(
				"initialized provider",
				"provider",
				providerName,
				"host",
				hostname,
			)
		}

		if len(providers) == 0 {
			return nil, fmt.Errorf("host %s has no providers configured", hostname)
		}

		expiry, err := time.ParseDuration(hostConfig.JWT.Expiry)
		if err != nil {
			return nil, fmt.Errorf(
				"invalid JWT expiry for host %s: %w",
				hostname,
				err,
			)
		}

		hosts[hostname] = &hostData{
			allowedRedirectURLs: hostConfig.AllowedRedirectURLs,
			loginDir:            hostConfig.LoginDir,
			jwtAudience:         hostConfig.JWT.Audience,
			jwtExpiry:           expiry,
			jwtKeyID:            hostConfig.JWT.KeyID,
			providers:           providers,
			signKey:             signKey,
		}
	}

	server := &Server{
		cookieName:    cfg.Config.Cookie.Name,
		cookieSecret:  cfg.Config.Cookie.Secret,
		serverPort:    cfg.Config.Server.Port,
		rateLimiter:   cfg.RateLimiter,
		hosts:         hosts,
		metricsConfig: cfg.Config,
	}

	addr := fmt.Sprintf(":%s", server.serverPort)
	mux := server.setupRoutes()

	httpServer := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
		BaseContext: func(_ net.Listener) context.Context {
			return GetServerBaseContext()
		},
	}

	server.httpServer = httpServer

	return server, nil
}

func (s *Server) Start() error {
	slog.Info("starting server", "port", s.serverPort)
	return s.httpServer.ListenAndServe()
}

func (s *Server) Stop(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) GetHTTPServer() *http.Server {
	return s.httpServer
}

func (s *Server) setupRoutes() http.Handler {
	mux := http.NewServeMux()

	withRateLimit := func(handler http.Handler) http.Handler {
		return s.rateLimiter.Limit(handler)
	}

	// Health check endpoint (no rate limiting - used by orchestrator)
	mux.Handle("GET /healthz", http.HandlerFunc(healthCheckHandler))

	mux.Handle(
		"GET /.well-known/jwks.json",
		withRateLimit(http.HandlerFunc(s.handlerJwks)),
	)
	mux.Handle(
		"GET /oauth/login/{provider}",
		withRateLimit(http.HandlerFunc(s.handlerLogin)),
	)
	mux.Handle(
		"GET /oauth/callback/{provider}",
		withRateLimit(http.HandlerFunc(s.handlerCallback)),
	)
	mux.Handle("GET /", withRateLimit(http.HandlerFunc(s.handlerRoot)))

	// Register metrics endpoint if enabled
	metricsPath := ""
	if s.metricsConfig.Metrics.Enable {
		metricsPath = s.metricsConfig.Metrics.Path
		if metricsPath == "" {
			metricsPath = "/metrics"
		}
		mux.Handle("GET "+metricsPath, promhttp.Handler())
		slog.Info("metrics endpoint enabled", "path", metricsPath)
	}

	// Apply middleware in reverse order (last applied is executed first)
	handler := logging.Middleware(mux)
	handler = metricsMiddleware(handler, metricsPath)

	return handler
}
