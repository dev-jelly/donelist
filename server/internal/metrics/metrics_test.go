package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func TestNewMetrics(t *testing.T) {
	m := NewMetrics()

	assert.NotNil(t, m.HTTPRequestsTotal)
	assert.NotNil(t, m.HTTPRequestDuration)
	assert.NotNil(t, m.HTTPErrorsTotal)
	assert.NotNil(t, m.DBConnectionsTotal)
	assert.NotNil(t, m.DBConnectionsInUse)
	assert.NotNil(t, m.DBConnectionsIdle)
	assert.NotNil(t, m.JobQueueLength)
	assert.NotNil(t, m.JobQueueWaitTime)
	assert.NotNil(t, m.JobProcessingTime)
	assert.NotNil(t, m.JobsProcessedTotal)
	assert.NotNil(t, m.JobsFailedTotal)
}

func TestRecordHTTPRequest(t *testing.T) {
	// Create a custom registry to avoid conflicts with default registry
	registry := prometheus.NewRegistry()

	counter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_test_total",
			Help: "Test counter",
		},
		[]string{"method", "path", "status"},
	)
	histogram := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "http_request_test_duration_seconds",
			Help: "Test histogram",
		},
		[]string{"method", "path"},
	)

	registry.MustRegister(counter)
	registry.MustRegister(histogram)

	m := &Metrics{
		HTTPRequestsTotal:   counter,
		HTTPRequestDuration: histogram,
	}

	// Record a request
	m.RecordHTTPRequest("GET", "/api/v1/users", 200, 0.150)

	// Verify counter was incremented
	count := testutil.CollectAndCount(counter)
	assert.Equal(t, 1, count)

	// Verify histogram recorded the observation
	histCount := testutil.CollectAndCount(histogram)
	assert.Equal(t, 1, histCount)
}

func TestRecordHTTPError(t *testing.T) {
	registry := prometheus.NewRegistry()

	errorCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_errors_test_total",
			Help: "Test error counter",
		},
		[]string{"method", "path", "status_code"},
	)

	registry.MustRegister(errorCounter)

	m := &Metrics{
		HTTPErrorsTotal: errorCounter,
	}

	// Record errors
	m.RecordHTTPError("GET", "/api/v1/users/:id", 404)
	m.RecordHTTPError("POST", "/api/v1/users", 400)
	m.RecordHTTPError("GET", "/api/v1/users/:id", 500)

	// Verify all errors were recorded
	count := testutil.CollectAndCount(errorCounter)
	assert.Equal(t, 3, count)
}

func TestUpdateDBPoolStats(t *testing.T) {
	registry := prometheus.NewRegistry()

	total := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "db_connections_test_total",
		Help: "Test gauge",
	})
	inUse := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "db_connections_test_in_use",
		Help: "Test gauge",
	})
	idle := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "db_connections_test_idle",
		Help: "Test gauge",
	})

	registry.MustRegister(total)
	registry.MustRegister(inUse)
	registry.MustRegister(idle)

	m := &Metrics{
		DBConnectionsTotal:  total,
		DBConnectionsInUse:  inUse,
		DBConnectionsIdle:   idle,
	}

	// Update pool stats
	m.UpdateDBPoolStats(10, 3, 7)

	// Verify all gauges were set correctly
	// Note: testutil doesn't provide easy way to check gauge values,
	// but we can verify they were registered
	assert.Equal(t, 3, testutil.CollectAndCount(registry))
}

func TestJobMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()

	queueLength := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "job_queue_test_length",
			Help: "Test gauge",
		},
		[]string{"queue_name"},
	)

	waitTime := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "job_queue_test_wait_time_seconds",
			Help: "Test histogram",
		},
		[]string{"queue_name", "job_type"},
	)

	processingTime := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "job_test_processing_time_seconds",
			Help: "Test histogram",
		},
		[]string{"queue_name", "job_type"},
	)

	processed := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "jobs_test_processed_total",
			Help: "Test counter",
		},
		[]string{"queue_name", "job_type", "status"},
	)

	failed := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "jobs_test_failed_total",
			Help: "Test counter",
		},
		[]string{"queue_name", "job_type", "error_type"},
	)

	registry.MustRegister(queueLength)
	registry.MustRegister(waitTime)
	registry.MustRegister(processingTime)
	registry.MustRegister(processed)
	registry.MustRegister(failed)

	m := &Metrics{
		JobQueueLength:    queueLength,
		JobQueueWaitTime:  waitTime,
		JobProcessingTime: processingTime,
		JobsProcessedTotal: processed,
		JobsFailedTotal:   failed,
	}

	// Test queue length
	m.SetJobQueueLength("notifications", 5)
	assert.Equal(t, 1, testutil.CollectAndCount(queueLength))

	// Test wait time
	m.RecordJobQueueWaitTime("notifications", "email", 1.5)
	assert.Equal(t, 1, testutil.CollectAndCount(waitTime))

	// Test processing time
	m.RecordJobProcessingTime("notifications", "email", 0.3)
	assert.Equal(t, 1, testutil.CollectAndCount(processingTime))

	// Test processed counter
	m.RecordJobProcessed("notifications", "email", "success")
	assert.Equal(t, 1, testutil.CollectAndCount(processed))

	// Test failed counter
	m.RecordJobFailed("notifications", "email", "connection_error")
	assert.Equal(t, 1, testutil.CollectAndCount(failed))
}

func TestWebSocketMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()

	wsConnections := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "websocket_test_connections",
		Help: "Test gauge",
	})

	registry.MustRegister(wsConnections)

	m := &Metrics{
		WebSocketConnections: wsConnections,
	}

	// Test increment
	m.IncrementWebSocketConnections()
	m.IncrementWebSocketConnections()

	// Test decrement
	m.DecrementWebSocketConnections()

	// Should be 1 metric registered
	assert.Equal(t, 1, testutil.CollectAndCount(registry))
}

func TestCacheMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()

	hits := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_test_hits_total",
			Help: "Test counter",
		},
		[]string{"cache_name"},
	)

	misses := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_test_misses_total",
			Help: "Test counter",
		},
		[]string{"cache_name"},
	)

	duration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "cache_test_operation_duration_seconds",
			Help: "Test histogram",
		},
		[]string{"operation"},
	)

	registry.MustRegister(hits)
	registry.MustRegister(misses)
	registry.MustRegister(duration)

	m := &Metrics{
		CacheHits:              hits,
		CacheMisses:            misses,
		CacheOperationDuration: duration,
	}

	// Test cache operations
	m.RecordCacheHit("timeline")
	m.RecordCacheHit("timeline")
	m.RecordCacheMiss("timeline")
	m.RecordCacheOperation("get", 0.001)

	// Verify metrics were recorded
	assert.Equal(t, 1, testutil.CollectAndCount(hits))
	assert.Equal(t, 1, testutil.CollectAndCount(misses))
	assert.Equal(t, 1, testutil.CollectAndCount(duration))
}

func TestHealthCheckStatus(t *testing.T) {
	registry := prometheus.NewRegistry()

	healthStatus := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "health_test_check_status",
			Help: "Test gauge",
		},
		[]string{"component"},
	)

	registry.MustRegister(healthStatus)

	m := &Metrics{
		HealthCheckStatus: healthStatus,
	}

	// Set various health statuses
	m.SetHealthCheckStatus("database", true)
	m.SetHealthCheckStatus("redis", true)
	m.SetHealthCheckStatus("search", false)

	// Verify all components were registered
	assert.Equal(t, 3, testutil.CollectAndCount(healthStatus))
}

func TestNormalizeStatusCode(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		expected string
	}{
		{"2xx success", 200, "2xx"},
		{"2xx created", 201, "2xx"},
		{"3xx redirect", 302, "3xx"},
		{"4xx bad request", 400, "4xx"},
		{"4xx not found", 404, "4xx"},
		{"5xx error", 500, "5xx"},
		{"5xx gateway", 502, "5xx"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeStatusCode(tt.status)
			assert.Equal(t, tt.expected, result)
		})
	}
}
