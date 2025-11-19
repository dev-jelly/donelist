package metrics

import (
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

	// Application-specific metrics
	CheckinsTotal          *prometheus.CounterVec
	CheckinOperationTotal  *prometheus.CounterVec
	ActiveUsers            prometheus.Gauge
	WebSocketConnections   prometheus.Gauge

	// Database metrics
	DBConnectionsTotal     prometheus.Gauge
	DBOperationDuration    *prometheus.HistogramVec
	DBOperationErrors      *prometheus.CounterVec

	// Redis metrics
	CacheHits              *prometheus.CounterVec
	CacheMisses            *prometheus.CounterVec
	CacheOperationDuration *prometheus.HistogramVec

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
				Name:    "http_request_duration_seconds",
				Help:    "HTTP request latency in seconds",
				Buckets: prometheus.DefBuckets,
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
		DBOperationDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "db_operation_duration_seconds",
				Help:    "Database operation duration in seconds",
				Buckets: prometheus.DefBuckets,
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
				Name:    "cache_operation_duration_seconds",
				Help:    "Cache operation duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"operation"},
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
