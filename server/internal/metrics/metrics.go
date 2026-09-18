package metrics

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for the application
type Metrics struct {
	// HTTP Request metrics
	HTTPRequestsTotal      *prometheus.CounterVec
	HTTPRequestDuration    *prometheus.HistogramVec
	HTTPRequestSize        *prometheus.HistogramVec
	HTTPResponseSize       *prometheus.HistogramVec
	HTTPRequestsInFlight   *prometheus.GaugeVec
	HTTPErrorsTotal        *prometheus.CounterVec // New: HTTP error counter

	// Application-specific metrics
	CheckinsTotal          *prometheus.CounterVec
	CheckinOperationTotal  *prometheus.CounterVec
	ActiveUsers            prometheus.Gauge
	WebSocketConnections   prometheus.Gauge

	// Database metrics
	DBConnectionsTotal     prometheus.Gauge
	DBConnectionsInUse     prometheus.Gauge // New: Connections in use
	DBConnectionsIdle      prometheus.Gauge // New: Idle connections
	DBOperationDuration    *prometheus.HistogramVec
	DBOperationErrors      *prometheus.CounterVec

	// Redis metrics
	CacheHits              *prometheus.CounterVec
	CacheMisses            *prometheus.CounterVec
	CacheOperationDuration *prometheus.HistogramVec

	// Job Queue metrics
	JobQueueLength         *prometheus.GaugeVec   // New: Queue length by queue name
	JobQueueWaitTime       *prometheus.HistogramVec // New: Time jobs wait in queue
	JobProcessingTime      *prometheus.HistogramVec // New: Job processing duration
	JobsProcessedTotal     *prometheus.CounterVec // New: Total jobs processed
	JobsFailedTotal        *prometheus.CounterVec // New: Total failed jobs

	// System metrics
	HealthCheckStatus      *prometheus.GaugeVec
}

// NewMetrics creates and registers all Prometheus metrics
func NewMetrics() *Metrics {
	m := &Metrics{
		// HTTP Request metrics
		HTTPRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_requests_total",
				Help: "Total number of HTTP requests",
			},
			[]string{"method", "path", "status"},
		),
		HTTPRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "http_request_duration_seconds",
				Help: "HTTP request latency in seconds",
				// Custom buckets optimized for API percentiles (p50, p90, p95, p99)
				// 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s
				Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			},
			[]string{"method", "path"},
		),
		HTTPRequestSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_request_size_bytes",
				Help:    "HTTP request size in bytes",
				Buckets: prometheus.ExponentialBuckets(100, 10, 8),
			},
			[]string{"method", "path"},
		),
		HTTPResponseSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_response_size_bytes",
				Help:    "HTTP response size in bytes",
				Buckets: prometheus.ExponentialBuckets(100, 10, 8),
			},
			[]string{"method", "path"},
		),
		HTTPRequestsInFlight: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "http_requests_in_flight",
				Help: "Current number of HTTP requests being processed",
			},
			[]string{"method"},
		),
		HTTPErrorsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_errors_total",
				Help: "Total number of HTTP errors by status code",
			},
			[]string{"method", "path", "status_code"}, // Limited cardinality: method, path template, status code
		),

		// Application-specific metrics
		CheckinsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "checkins_total",
				Help: "Total number of check-ins created",
			},
			[]string{"user_id", "category"},
		),
		CheckinOperationTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "checkin_operations_total",
				Help: "Total number of check-in operations",
			},
			[]string{"operation", "status"},
		),
		ActiveUsers: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "active_users",
				Help: "Current number of active users (with sessions)",
			},
		),
		WebSocketConnections: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "websocket_connections",
				Help: "Current number of active WebSocket connections",
			},
		),

		// Database metrics
		DBConnectionsTotal: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "db_connections_total",
				Help: "Current number of database connections",
			},
		),
		DBConnectionsInUse: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "db_connections_in_use",
				Help: "Current number of database connections in use",
			},
		),
		DBConnectionsIdle: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "db_connections_idle",
				Help: "Current number of idle database connections",
			},
		),
		DBOperationDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "db_operation_duration_seconds",
				Help: "Database operation duration in seconds",
				// DB operations should be faster - 1ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s
				Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
			},
			[]string{"operation", "table"},
		),
		DBOperationErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "db_operation_errors_total",
				Help: "Total number of database operation errors",
			},
			[]string{"operation", "table"},
		),

		// Redis metrics
		CacheHits: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cache_hits_total",
				Help: "Total number of cache hits",
			},
			[]string{"cache_name"},
		),
		CacheMisses: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cache_misses_total",
				Help: "Total number of cache misses",
			},
			[]string{"cache_name"},
		),
		CacheOperationDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "cache_operation_duration_seconds",
				Help: "Cache operation duration in seconds",
				// Cache should be very fast - 0.1ms, 0.5ms, 1ms, 5ms, 10ms, 25ms, 50ms, 100ms
				Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1},
			},
			[]string{"operation"},
		),

		// Job Queue metrics
		JobQueueLength: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "job_queue_length",
				Help: "Current number of jobs waiting in queue",
			},
			[]string{"queue_name"},
		),
		JobQueueWaitTime: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "job_queue_wait_time_seconds",
				Help: "Time jobs spend waiting in queue before processing",
				// Queue wait times: 100ms, 500ms, 1s, 5s, 10s, 30s, 60s, 120s
				Buckets: []float64{0.1, 0.5, 1, 5, 10, 30, 60, 120},
			},
			[]string{"queue_name", "job_type"},
		),
		JobProcessingTime: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "job_processing_time_seconds",
				Help: "Time spent processing jobs",
				// Processing times: 100ms, 500ms, 1s, 5s, 10s, 30s, 60s, 300s
				Buckets: []float64{0.1, 0.5, 1, 5, 10, 30, 60, 300},
			},
			[]string{"queue_name", "job_type"},
		),
		JobsProcessedTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "jobs_processed_total",
				Help: "Total number of jobs processed",
			},
			[]string{"queue_name", "job_type", "status"}, // status: success, failed
		),
		JobsFailedTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "jobs_failed_total",
				Help: "Total number of failed jobs",
			},
			[]string{"queue_name", "job_type", "error_type"},
		),

		// System metrics
		HealthCheckStatus: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "health_check_status",
				Help: "Health check status (1 = healthy, 0 = unhealthy)",
			},
			[]string{"component"},
		),
	}

	return m
}

// RecordHTTPRequest records an HTTP request metric
func (m *Metrics) RecordHTTPRequest(method, path string, status int, duration float64) {
	m.HTTPRequestsTotal.WithLabelValues(method, path, string(rune(status))).Inc()
	m.HTTPRequestDuration.WithLabelValues(method, path).Observe(duration)
}

// RecordCheckin records a check-in creation
func (m *Metrics) RecordCheckin(userID, category string) {
	m.CheckinsTotal.WithLabelValues(userID, category).Inc()
}

// RecordCheckinOperation records a check-in operation
func (m *Metrics) RecordCheckinOperation(operation, status string) {
	m.CheckinOperationTotal.WithLabelValues(operation, status).Inc()
}

// SetActiveUsers sets the current number of active users
func (m *Metrics) SetActiveUsers(count int) {
	m.ActiveUsers.Set(float64(count))
}

// IncrementWebSocketConnections increments WebSocket connection count
func (m *Metrics) IncrementWebSocketConnections() {
	m.WebSocketConnections.Inc()
}

// DecrementWebSocketConnections decrements WebSocket connection count
func (m *Metrics) DecrementWebSocketConnections() {
	m.WebSocketConnections.Dec()
}

// SetDBConnections sets the current number of database connections
func (m *Metrics) SetDBConnections(count int) {
	m.DBConnectionsTotal.Set(float64(count))
}

// SetDBConnectionsInUse sets the current number of database connections in use
func (m *Metrics) SetDBConnectionsInUse(count int) {
	m.DBConnectionsInUse.Set(float64(count))
}

// SetDBConnectionsIdle sets the current number of idle database connections
func (m *Metrics) SetDBConnectionsIdle(count int) {
	m.DBConnectionsIdle.Set(float64(count))
}

// UpdateDBPoolStats updates all database pool statistics
func (m *Metrics) UpdateDBPoolStats(total, inUse, idle int) {
	m.DBConnectionsTotal.Set(float64(total))
	m.DBConnectionsInUse.Set(float64(inUse))
	m.DBConnectionsIdle.Set(float64(idle))
}

// RecordDBOperation records a database operation
func (m *Metrics) RecordDBOperation(operation, table string, duration float64, err error) {
	m.DBOperationDuration.WithLabelValues(operation, table).Observe(duration)
	if err != nil {
		m.DBOperationErrors.WithLabelValues(operation, table).Inc()
	}
}

// RecordCacheHit records a cache hit
func (m *Metrics) RecordCacheHit(cacheName string) {
	m.CacheHits.WithLabelValues(cacheName).Inc()
}

// RecordCacheMiss records a cache miss
func (m *Metrics) RecordCacheMiss(cacheName string) {
	m.CacheMisses.WithLabelValues(cacheName).Inc()
}

// RecordCacheOperation records a cache operation
func (m *Metrics) RecordCacheOperation(operation string, duration float64) {
	m.CacheOperationDuration.WithLabelValues(operation).Observe(duration)
}

// SetHealthCheckStatus sets the health check status for a component
func (m *Metrics) SetHealthCheckStatus(component string, healthy bool) {
	status := 0.0
	if healthy {
		status = 1.0
	}
	m.HealthCheckStatus.WithLabelValues(component).Set(status)
}

// RecordHTTPError records an HTTP error
func (m *Metrics) RecordHTTPError(method, path string, statusCode int) {
	m.HTTPErrorsTotal.WithLabelValues(method, path, fmt.Sprintf("%d", statusCode)).Inc()
}

// SetJobQueueLength sets the current queue length for a specific queue
func (m *Metrics) SetJobQueueLength(queueName string, length int) {
	m.JobQueueLength.WithLabelValues(queueName).Set(float64(length))
}

// RecordJobQueueWaitTime records how long a job waited in queue
func (m *Metrics) RecordJobQueueWaitTime(queueName, jobType string, duration float64) {
	m.JobQueueWaitTime.WithLabelValues(queueName, jobType).Observe(duration)
}

// RecordJobProcessingTime records how long a job took to process
func (m *Metrics) RecordJobProcessingTime(queueName, jobType string, duration float64) {
	m.JobProcessingTime.WithLabelValues(queueName, jobType).Observe(duration)
}

// RecordJobProcessed records a processed job with its status
func (m *Metrics) RecordJobProcessed(queueName, jobType, status string) {
	m.JobsProcessedTotal.WithLabelValues(queueName, jobType, status).Inc()
}

// RecordJobFailed records a failed job with error type
func (m *Metrics) RecordJobFailed(queueName, jobType, errorType string) {
	m.JobsFailedTotal.WithLabelValues(queueName, jobType, errorType).Inc()
}
