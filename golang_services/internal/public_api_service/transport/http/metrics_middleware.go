package http

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware" // Required for NewWrapResponseWriter
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests.",
		},
		[]string{"method", "path", "status_code"},
	)

	httpRequestDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of HTTP requests.",
			Buckets: prometheus.DefBuckets, // Default buckets, can be customized
		},
		[]string{"method", "path"},
	)
)

// PrometheusMetricsMiddleware is a Chi middleware that records Prometheus metrics for HTTP requests.
func PrometheusMetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		// Using chi's NewWrapResponseWriter to get status code
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r) // Process request

		duration := time.Since(start)

		// Get the path pattern from Chi's routing context
		path := chi.RouteContext(r.Context()).RoutePattern()
		if path == "" {
			path = "unknown" // Fallback if path is not found in context
		}

		statusCode := ww.Status()
		if statusCode == 0 { // Typically indicates no response was written (e.g. hijacked connection)
			statusCode = http.StatusOK // Or another appropriate default if necessary
		}
		statusCodeStr := strconv.Itoa(statusCode)

		// Record metrics
		httpRequestDurationSeconds.WithLabelValues(r.Method, path).Observe(duration.Seconds())
		httpRequestsTotal.WithLabelValues(r.Method, path, statusCodeStr).Inc()
	})
}
