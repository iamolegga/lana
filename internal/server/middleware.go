package server

import (
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/iamolegga/lana/internal/metrics"
)

// metricsMiddleware wraps an HTTP handler to collect metrics
func metricsMiddleware(next http.Handler, ignorePaths ...string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !metrics.Enabled() || slices.Contains(ignorePaths, r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()

		// Create a response writer wrapper to capture status code
		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK, // Default to 200
		}

		// Call the next handler
		next.ServeHTTP(wrapped, r)

		// Record metrics
		duration := time.Since(start).Seconds()
		statusCode := strconv.Itoa(wrapped.statusCode)
		host := r.Host

		metrics.RecordHTTPRequest(r.Method, r.URL.Path, statusCode, host)
		metrics.RecordHTTPDuration(r.Method, r.URL.Path, host, duration)
	})
}

// responseWriter wraps http.ResponseWriter to capture the status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
