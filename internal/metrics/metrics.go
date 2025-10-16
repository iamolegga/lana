package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HTTPRequestsTotal tracks the total number of HTTP requests
	HTTPRequestsTotal *prometheus.CounterVec

	// HTTPRequestDuration tracks the duration of HTTP requests
	HTTPRequestDuration *prometheus.HistogramVec

	// AuthenticationsTotal tracks authentication attempts
	AuthenticationsTotal *prometheus.CounterVec

	enabled bool
)

// Init initializes the metrics system
func Init(enable bool, goMetrics bool) {
	enabled = enable

	if !enabled {
		return
	}

	// If goMetrics is false, unregister default Go collectors
	if !goMetrics {
		prometheus.Unregister(prometheus.NewGoCollector())
		prometheus.Unregister(
			prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
		)
	}

	// Initialize HTTP request counter
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "lana_http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status_code", "host"},
	)

	// Initialize HTTP request duration histogram
	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "lana_http_request_duration_seconds",
			Help: "HTTP request latency in seconds",
			Buckets: []float64{
				0.001,
				0.005,
				0.01,
				0.05,
				0.1,
				0.5,
				1.0,
				2.5,
				5.0,
				10.0,
			},
		},
		[]string{"method", "path", "host"},
	)

	// Initialize authentication counter
	AuthenticationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "lana_authentications_total",
			Help: "Total number of authentication attempts",
		},
		[]string{"provider", "host", "status", "reason"},
	)
}

// Enabled returns whether metrics are enabled
func Enabled() bool {
	return enabled
}

// RecordHTTPRequest records an HTTP request metric
func RecordHTTPRequest(method, path, statusCode, host string) {
	if !enabled || HTTPRequestsTotal == nil {
		return
	}
	HTTPRequestsTotal.WithLabelValues(method, path, statusCode, host).Inc()
}

// RecordHTTPDuration records an HTTP request duration metric
func RecordHTTPDuration(method, path, host string, duration float64) {
	if !enabled || HTTPRequestDuration == nil {
		return
	}
	HTTPRequestDuration.WithLabelValues(method, path, host).Observe(duration)
}

// RecordAuthentication records an authentication attempt metric
func RecordAuthentication(provider, host, status, reason string) {
	if !enabled || AuthenticationsTotal == nil {
		return
	}
	AuthenticationsTotal.WithLabelValues(provider, host, status, reason).Inc()
}
