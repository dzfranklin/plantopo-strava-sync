package middleware

import (
	"net/http"
	"strconv"
	"time"

	"plantopo-strava-sync/internal/metrics"
)

// responseWriter wraps http.ResponseWriter to capture the status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.statusCode = code
		rw.written = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}

// MetricsMiddleware wraps an HTTP handler with Prometheus metrics
func MetricsMiddleware(endpoint string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap the response writer to capture status code
			wrapped := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
				written:        false,
			}

			// Call the next handler
			next.ServeHTTP(wrapped, r)

			// Record metrics
			duration := time.Since(start).Seconds()
			statusStr := strconv.Itoa(wrapped.statusCode)
			metrics.HTTPRequestsTotal.WithLabelValues(endpoint, statusStr).Inc()
			metrics.HTTPRequestDuration.WithLabelValues(endpoint, statusStr).Observe(duration)
		})
	}
}

// WrapHandler is a convenience function to wrap a HandlerFunc with metrics
func WrapHandler(endpoint string, handler http.HandlerFunc) http.Handler {
	return MetricsMiddleware(endpoint)(handler)
}
