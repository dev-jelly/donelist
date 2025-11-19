package metrics

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// MetricsMiddleware creates a Gin middleware that tracks HTTP metrics
func MetricsMiddleware(m *Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip metrics for the metrics endpoint itself to avoid recursion
		if c.Request.URL.Path == "/metrics" {
			c.Next()
			return
		}

		start := time.Now()
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		method := c.Request.Method

		// Track in-flight requests
		m.HTTPRequestsInFlight.WithLabelValues(method).Inc()
		defer m.HTTPRequestsInFlight.WithLabelValues(method).Dec()

		// Record request size
		if c.Request.ContentLength > 0 {
			m.HTTPRequestSize.WithLabelValues(method, path).Observe(float64(c.Request.ContentLength))
		}

		// Process request
		c.Next()

		// Calculate duration
		duration := time.Since(start).Seconds()

		// Get status code
		status := c.Writer.Status()
		statusStr := strconv.Itoa(status)

		// Record metrics
		m.HTTPRequestsTotal.WithLabelValues(method, path, statusStr).Inc()
		m.HTTPRequestDuration.WithLabelValues(method, path).Observe(duration)

		// Record response size
		responseSize := c.Writer.Size()
		if responseSize > 0 {
			m.HTTPResponseSize.WithLabelValues(method, path).Observe(float64(responseSize))
		}
	}
}

// NormalizeStatusCode converts status code to string
func NormalizeStatusCode(status int) string {
	return fmt.Sprintf("%dxx", status/100)
}
