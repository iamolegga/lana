package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/iamolegga/lana/internal/metrics"
)

// metricsMiddleware wraps an HTTP handler to collect metrics. The classify
// function maps a request to the `path` label value, allowing callers to
// bucket unbounded paths (e.g. static-file requests) into a fixed set.
func metricsMiddleware(next http.Handler, classify func(*http.Request) string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !metrics.Enabled() {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()

		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start).Seconds()
		statusCode := strconv.Itoa(wrapped.statusCode)
		host := r.Host
		path := classify(r)

		metrics.RecordHTTPRequest(r.Method, path, statusCode, host)
		metrics.RecordHTTPDuration(r.Method, path, host, duration)
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
