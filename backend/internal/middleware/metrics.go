package middleware

import (
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	metricsOnce           sync.Once
	sharedRequestDuration *prometheus.HistogramVec
)

// MetricsMiddleware records HTTP request durations and counts as Prometheus metrics.
type MetricsMiddleware struct {
	requestDuration *prometheus.HistogramVec
}

// NewMetricsMiddleware initializes the Prometheus metrics for HTTP requests.
func NewMetricsMiddleware() *MetricsMiddleware {
	metricsOnce.Do(func() {
		sharedRequestDuration = promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "vitalwatch_http_request_duration_seconds",
				Help:    "Duration of HTTP requests in seconds.",
				Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			},
			[]string{"method", "path", "status"},
		)
	})

	return &MetricsMiddleware{
		requestDuration: sharedRequestDuration,
	}
}

// Middleware returns a Gin HandlerFunc that measures request durations.
func (m *MetricsMiddleware) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		duration := time.Since(start).Seconds()

		// Use c.FullPath() to avoid high cardinality from dynamic path parameters (e.g., /users/:id)
		// If FullPath is empty, it means the route was not found (404), use a fallback.
		path := c.FullPath()
		if path == "" {
			path = "unknown_route"
		}

		status := strconv.Itoa(c.Writer.Status())
		method := c.Request.Method

		m.requestDuration.WithLabelValues(method, path, status).Observe(duration)
	}
}
