package metrics

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestMetricsForIntegration() *Metrics {
	// Create separate test metrics to avoid global registry conflicts
	registry := prometheus.NewRegistry()

	counter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "test_integration_http_requests_total",
			Help: "Test counter",
		},
		[]string{"method", "path", "status"},
	)
	histogram := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "test_integration_http_request_duration_seconds",
			Help: "Test histogram",
		},
		[]string{"method", "path"},
	)
	requestSize := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "test_integration_http_request_size_bytes",
			Help: "Test histogram",
		},
		[]string{"method", "path"},
	)
	responseSize := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "test_integration_http_response_size_bytes",
			Help: "Test histogram",
		},
		[]string{"method", "path"},
	)
	inFlight := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "test_integration_http_requests_in_flight",
			Help: "Test gauge",
		},
		[]string{"method"},
	)
	errorCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "test_integration_http_errors_total",
			Help: "Test error counter",
		},
		[]string{"method", "path", "status_code"},
	)

	registry.MustRegister(counter, histogram, requestSize, responseSize, inFlight, errorCounter)

	return &Metrics{
		HTTPRequestsTotal:    counter,
		HTTPRequestDuration:  histogram,
		HTTPRequestSize:      requestSize,
		HTTPResponseSize:     responseSize,
		HTTPRequestsInFlight: inFlight,
		HTTPErrorsTotal:      errorCounter,
	}
}

// TestMetricsEndpoint verifies that the /metrics endpoint works correctly
func TestMetricsEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ResetPathRegistry()

	// Use pre-created test metrics to avoid duplicate registration
	m := setupTestMetricsForIntegration()
	router := gin.New()
	router.Use(MetricsMiddleware(m))

	// Expose metrics endpoint
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Add some test routes
	router.GET("/api/v1/users/:id", func(c *gin.Context) {
		c.JSON(200, gin.H{"id": c.Param("id")})
	})
	router.GET("/api/v1/error", func(c *gin.Context) {
		c.JSON(500, gin.H{"error": "test error"})
	})

	// Make some requests to generate metrics
	testCases := []struct {
		path   string
		status int
	}{
		{"/api/v1/users/123", 200},
		{"/api/v1/users/456", 200},
		{"/api/v1/error", 500},
	}

	for _, tc := range testCases {
		req := httptest.NewRequest("GET", tc.path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, tc.status, w.Code)
	}

	// Now fetch metrics
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	body := w.Body.String()

	// Verify some default metrics exist (these are always present from Go runtime)
	t.Run("go_goroutines exists", func(t *testing.T) {
		assert.Contains(t, body, "go_goroutines")
	})

	t.Run("go_memstats exists", func(t *testing.T) {
		assert.Contains(t, body, "go_memstats")
	})

	t.Run("process_cpu_seconds_total exists", func(t *testing.T) {
		assert.Contains(t, body, "process_cpu_seconds_total")
	})
}

// TestMetricsFormat verifies that metrics follow Prometheus conventions
func TestMetricsFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ResetPathRegistry()

	m := setupTestMetricsForIntegration()
	router := gin.New()
	router.Use(MetricsMiddleware(m))
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Make a request
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	body := w.Body.String()
	lines := strings.Split(body, "\n")

	// Check that each metric has proper format
	for _, line := range lines {
		if strings.HasPrefix(line, "#") || line == "" {
			continue // Skip comments and empty lines
		}

		// Metric lines should contain a space separating name and value
		assert.Contains(t, line, " ", "Metric line should have format: name{labels} value")
	}
}

// TestMetricsContentType verifies the correct content type
func TestMetricsContentType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ResetPathRegistry()

	m := setupTestMetricsForIntegration()
	router := gin.New()
	router.Use(MetricsMiddleware(m))
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)

	contentType := w.Header().Get("Content-Type")
	// Prometheus metrics endpoint should return text/plain or a similar format
	assert.Contains(t, contentType, "text/plain")
}

// TestMetricsWithLabels verifies that labels are properly attached
func TestMetricsWithLabels(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ResetPathRegistry()

	// Create custom registry with our metrics
	registry := prometheus.NewRegistry()
	m := setupTestMetricsForIntegration()
	// Register metrics with custom registry
	registry.MustRegister(m.HTTPRequestsTotal, m.HTTPRequestDuration, m.HTTPRequestSize,
		m.HTTPResponseSize, m.HTTPRequestsInFlight, m.HTTPErrorsTotal)

	router := gin.New()
	router.Use(MetricsMiddleware(m))
	router.GET("/metrics", gin.WrapH(promhttp.HandlerFor(registry, promhttp.HandlerOpts{})))
	router.GET("/api/v1/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Make a request
	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)

	// Fetch metrics
	req = httptest.NewRequest("GET", "/metrics", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)

	body := w.Body.String()

	// Verify labels are present
	t.Run("method label exists", func(t *testing.T) {
		assert.Contains(t, body, `method="GET"`)
	})

	t.Run("path label exists", func(t *testing.T) {
		assert.Contains(t, body, `path="/api/v1/test"`)
	})

	t.Run("status label exists", func(t *testing.T) {
		assert.Contains(t, body, `status="200"`)
	})
}

// TestMetricsCardinality verifies label cardinality is limited
func TestMetricsCardinality(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ResetPathRegistry()

	m := setupTestMetricsForIntegration()
	router := gin.New()
	router.Use(MetricsMiddleware(m))

	// Catch-all route
	router.NoRoute(func(c *gin.Context) {
		c.JSON(404, gin.H{"error": "not found"})
	})

	// Make requests to many completely different paths (not just different IDs)
	// to actually generate different normalized paths
	for i := 0; i < MaxPathCardinality+100; i++ {
		// Create truly unique paths by varying the path structure
		path := fmt.Sprintf("/api/path%d/resource%d", i, i)
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	// Verify that we haven't exceeded the cardinality limit
	count := GetRegisteredPathCount()
	assert.LessOrEqual(t, count, MaxPathCardinality, "Path cardinality should be limited")

	// We should have reached the limit
	assert.Greater(t, count, 100, "Should have registered many paths")
}

// TestDBPoolMetrics verifies database pool metrics can be updated
func TestDBPoolMetrics(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create custom registry for DB metrics
	registry := prometheus.NewRegistry()
	dbTotal := prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_db_connections_total"})
	dbInUse := prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_db_connections_in_use"})
	dbIdle := prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_db_connections_idle"})
	registry.MustRegister(dbTotal, dbInUse, dbIdle)

	m := &Metrics{
		DBConnectionsTotal: dbTotal,
		DBConnectionsInUse: dbInUse,
		DBConnectionsIdle:  dbIdle,
	}

	router := gin.New()
	router.GET("/metrics", gin.WrapH(promhttp.HandlerFor(registry, promhttp.HandlerOpts{})))

	// Simulate DB pool updates
	m.UpdateDBPoolStats(10, 3, 7)
	m.SetDBConnections(10)
	m.SetDBConnectionsInUse(4)
	m.SetDBConnectionsIdle(6)

	// Fetch metrics
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)

	body := w.Body.String()

	// Verify DB metrics exist
	assert.Contains(t, body, "db_connections_total")
	assert.Contains(t, body, "db_connections_in_use")
	assert.Contains(t, body, "db_connections_idle")
}

// TestJobQueueMetrics verifies job queue metrics can be recorded
func TestJobQueueMetrics(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create custom registry for job metrics
	registry := prometheus.NewRegistry()
	queueLength := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "test_job_queue_length"},
		[]string{"queue_name"},
	)
	waitTime := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{Name: "test_job_queue_wait_time_seconds"},
		[]string{"queue_name", "job_type"},
	)
	procTime := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{Name: "test_job_processing_time_seconds"},
		[]string{"queue_name", "job_type"},
	)
	processed := prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "test_jobs_processed_total"},
		[]string{"queue_name", "job_type", "status"},
	)
	failed := prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "test_jobs_failed_total"},
		[]string{"queue_name", "job_type", "error_type"},
	)
	registry.MustRegister(queueLength, waitTime, procTime, processed, failed)

	m := &Metrics{
		JobQueueLength:     queueLength,
		JobQueueWaitTime:   waitTime,
		JobProcessingTime:  procTime,
		JobsProcessedTotal: processed,
		JobsFailedTotal:    failed,
	}

	router := gin.New()
	router.GET("/metrics", gin.WrapH(promhttp.HandlerFor(registry, promhttp.HandlerOpts{})))

	// Simulate job queue operations
	m.SetJobQueueLength("notifications", 5)
	m.RecordJobQueueWaitTime("notifications", "email", 1.5)
	m.RecordJobProcessingTime("notifications", "email", 0.3)
	m.RecordJobProcessed("notifications", "email", "success")
	m.RecordJobFailed("notifications", "sms", "connection_error")

	// Fetch metrics
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)

	body := w.Body.String()

	// Verify job metrics exist
	assert.Contains(t, body, "job_queue_length")
	assert.Contains(t, body, "job_queue_wait_time_seconds")
	assert.Contains(t, body, "job_processing_time_seconds")
	assert.Contains(t, body, "jobs_processed_total")
	assert.Contains(t, body, "jobs_failed_total")
	assert.Contains(t, body, `queue_name="notifications"`)
}

// TestErrorMetrics verifies error tracking works correctly
func TestErrorMetrics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ResetPathRegistry()

	// Create custom registry to ensure we see our metrics
	registry := prometheus.NewRegistry()
	m := setupTestMetricsForIntegration()
	registry.MustRegister(m.HTTPRequestsTotal, m.HTTPRequestDuration, m.HTTPRequestSize,
		m.HTTPResponseSize, m.HTTPRequestsInFlight, m.HTTPErrorsTotal)

	router := gin.New()
	router.Use(MetricsMiddleware(m))
	router.GET("/metrics", gin.WrapH(promhttp.HandlerFor(registry, promhttp.HandlerOpts{})))

	router.GET("/api/v1/error400", func(c *gin.Context) {
		c.JSON(400, gin.H{"error": "bad request"})
	})
	router.GET("/api/v1/error404", func(c *gin.Context) {
		c.JSON(404, gin.H{"error": "not found"})
	})
	router.GET("/api/v1/error500", func(c *gin.Context) {
		c.JSON(500, gin.H{"error": "internal error"})
	})

	// Generate errors
	errorPaths := []string{"/api/v1/error400", "/api/v1/error404", "/api/v1/error500"}
	for _, path := range errorPaths {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	// Fetch metrics
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)

	body := w.Body.String()

	// Verify error metrics exist
	assert.Contains(t, body, "test_integration_http_errors_total")
	assert.Contains(t, body, `status_code="400"`)
	assert.Contains(t, body, `status_code="404"`)
	assert.Contains(t, body, `status_code="500"`)
}
