package metrics

import (
	"fmt"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	// MaxPathCardinality limits the number of unique paths to track
	MaxPathCardinality = 1000
	// UnknownPath is used when path cardinality limit is exceeded
	UnknownPath = "/unknown"
)

var (
	// pathRegistry tracks seen paths to limit cardinality
	pathRegistry     = make(map[string]bool)
	pathRegistryLock sync.RWMutex
	pathCount        = 0

	// uuidRegex matches UUID patterns in paths to normalize them
	uuidRegex = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	// numericIDRegex matches numeric IDs in paths
	numericIDRegex = regexp.MustCompile(`/\d+(/|$)`)
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
		method := c.Request.Method

		// Get path with cardinality limiting
		// Prefer route template from Gin (e.g., /api/v1/users/:id)
		path := c.FullPath()
		if path == "" {
			// If no template available, use raw path but normalize it
			path = normalizePath(c.Request.URL.Path)
		}

		// Apply cardinality limiting
		path = limitPathCardinality(path)

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

		// Record errors (4xx and 5xx status codes)
		if status >= 400 {
			m.RecordHTTPError(method, path, status)
		}
	}
}

// normalizePath normalizes a URL path by replacing variable segments
// Examples:
//   - /api/v1/users/123 -> /api/v1/users/:id
//   - /api/v1/checkins/550e8400-e29b-41d4-a716-446655440000 -> /api/v1/checkins/:id
func normalizePath(path string) string {
	// Replace UUIDs with :id
	path = uuidRegex.ReplaceAllString(path, ":id")

	// Replace numeric IDs with :id
	path = numericIDRegex.ReplaceAllString(path, "/:id$1")

	return path
}

// limitPathCardinality limits the number of unique paths tracked
// to prevent memory exhaustion from high-cardinality labels
func limitPathCardinality(path string) string {
	pathRegistryLock.RLock()
	exists := pathRegistry[path]
	count := pathCount
	pathRegistryLock.RUnlock()

	if exists {
		return path
	}

	// Check if we've hit the limit
	if count >= MaxPathCardinality {
		return UnknownPath
	}

	// Register new path
	pathRegistryLock.Lock()
	// Double-check after acquiring write lock
	if !pathRegistry[path] && pathCount < MaxPathCardinality {
		pathRegistry[path] = true
		pathCount++
	}
	pathRegistryLock.Unlock()

	return path
}

// NormalizeStatusCode converts status code to string in XXX format
func NormalizeStatusCode(status int) string {
	return fmt.Sprintf("%dxx", status/100)
}

// GetRegisteredPathCount returns the current number of registered paths
// Useful for monitoring and alerting on cardinality issues
func GetRegisteredPathCount() int {
	pathRegistryLock.RLock()
	defer pathRegistryLock.RUnlock()
	return pathCount
}

// ResetPathRegistry clears the path registry (useful for testing)
func ResetPathRegistry() {
	pathRegistryLock.Lock()
	defer pathRegistryLock.Unlock()
	pathRegistry = make(map[string]bool)
	pathCount = 0
}
