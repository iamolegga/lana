package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"
)

var isShuttingDown atomic.Bool
var rootCtx context.Context
var rootCancel context.CancelFunc
var ongoingCtx context.Context
var ongoingCancel context.CancelFunc

const (
	shutdownPeriod      = 15 * time.Second
	shutdownHardPeriod  = 3 * time.Second
	readinessDrainDelay = 5 * time.Second
)

func SubscribeForShutdown() {
	rootCtx, rootCancel = signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	ongoingCtx, ongoingCancel = context.WithCancel(context.Background())
}

// GetServerBaseContext returns the context that should be used as BaseContext
// for http.Server. This allows propagating shutdown signal to all handlers.
func GetServerBaseContext() context.Context {
	return ongoingCtx
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	if isShuttingDown.Load() {
		http.Error(w, "Shutting down", http.StatusServiceUnavailable)
		return
	}
	fmt.Fprintln(w, "OK")
}

func WaitForShutdown(server *http.Server) {
	<-rootCtx.Done()
	rootCancel()
	isShuttingDown.Store(true)
	slog.Info("shutdown signal received, starting graceful shutdown")

	// Phase 1: Readiness drain - wait for load balancers to stop sending traffic
	slog.Info(
		"waiting for readiness check propagation",
		"duration",
		readinessDrainDelay,
	)
	time.Sleep(readinessDrainDelay)
	slog.Info("readiness check propagated, shutting down HTTP server")

	// Phase 2: Graceful shutdown - stop accepting new connections, wait for handlers
	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		shutdownPeriod,
	)
	defer cancel()

	err := server.Shutdown(shutdownCtx)
	if err != nil {
		slog.Error(
			"server shutdown encountered an error",
			"error",
			err,
			"timeout",
			shutdownPeriod,
		)
	} else {
		slog.Info("server shutdown completed successfully")
	}

	// Phase 3: Cancel ongoing operations context
	slog.Info("cancelling ongoing operations context")
	ongoingCancel()

	// Wait a bit for operations to finish after context cancellation
	time.Sleep(shutdownHardPeriod)
	slog.Info("graceful shutdown complete")
}
