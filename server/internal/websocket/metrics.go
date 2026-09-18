package websocket

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

var (
	// Prometheus metrics
	wsConnectionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "websocket_connections_total",
			Help: "Total number of WebSocket connections",
		},
		[]string{"status", "user_type"},
	)

	wsConnectionsActive = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "websocket_connections_active",
			Help: "Number of active WebSocket connections",
		},
		[]string{"room", "user_type"},
	)

	wsMessagesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "websocket_messages_total",
			Help: "Total number of WebSocket messages",
		},
		[]string{"type", "direction", "status"},
	)

	wsMessageSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "websocket_message_size_bytes",
			Help:    "Size of WebSocket messages in bytes",
			Buckets: []float64{100, 500, 1000, 5000, 10000, 50000, 100000},
		},
		[]string{"type", "direction"},
	)

	wsMessageLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "websocket_message_latency_ms",
			Help:    "Latency of WebSocket message processing in milliseconds",
			Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000},
		},
		[]string{"type", "operation"},
	)

	wsReconnections = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "websocket_reconnections_total",
			Help: "Total number of WebSocket reconnections",
		},
		[]string{"reason", "success"},
	)

	wsRateLimitHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "websocket_rate_limit_hits_total",
			Help: "Total number of rate limit hits",
		},
		[]string{"user_id", "limit_type"},
	)

	wsAuthFailures = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "websocket_auth_failures_total",
			Help: "Total number of WebSocket authentication failures",
		},
		[]string{"reason"},
	)

	wsHealthCheckLatency = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "websocket_health_check_latency_ms",
			Help:    "Latency of WebSocket health checks in milliseconds",
			Buckets: []float64{10, 25, 50, 100, 250, 500, 1000},
		},
	)

	wsSequenceGaps = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "websocket_sequence_gaps_total",
			Help: "Total number of detected sequence gaps",
		},
		[]string{"partition", "recovered"},
	)

	wsBufferSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "websocket_buffer_size",
			Help: "Current size of message buffers",
		},
		[]string{"buffer_type", "partition"},
	)

	wsRoomSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "websocket_room_size",
			Help: "Number of clients in each room",
		},
		[]string{"room_id"},
	)

	wsSessionDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "websocket_session_duration_seconds",
			Help:    "Duration of WebSocket sessions in seconds",
			Buckets: []float64{10, 30, 60, 300, 600, 1800, 3600, 7200, 14400},
		},
		[]string{"disconnection_reason"},
	)
)

// MetricsCollector collects and exports WebSocket metrics
type MetricsCollector struct {
	tracer trace.Tracer
	meter  metric.Meter
	logger *zap.Logger

	// OpenTelemetry metrics
	connectionsCounter   metric.Int64Counter
	messagesCounter      metric.Int64Counter
	latencyHistogram     metric.Float64Histogram
	activeConnectionsGauge metric.Int64ObservableGauge
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(logger *zap.Logger) (*MetricsCollector, error) {
	tracer := otel.Tracer("websocket")
	meter := otel.Meter("websocket")

	// Create OpenTelemetry metrics
	connectionsCounter, err := meter.Int64Counter(
		"websocket.connections",
		metric.WithDescription("Total WebSocket connections"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, err
	}

	messagesCounter, err := meter.Int64Counter(
		"websocket.messages",
		metric.WithDescription("Total WebSocket messages"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, err
	}

	latencyHistogram, err := meter.Float64Histogram(
		"websocket.latency",
		metric.WithDescription("WebSocket operation latency"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	mc := &MetricsCollector{
		tracer:             tracer,
		meter:              meter,
		logger:             logger,
		connectionsCounter: connectionsCounter,
		messagesCounter:    messagesCounter,
		latencyHistogram:   latencyHistogram,
	}

	// Register observable gauge for active connections
	activeConnectionsGauge, err := meter.Int64ObservableGauge(
		"websocket.connections.active",
		metric.WithDescription("Number of active WebSocket connections"),
		metric.WithUnit("1"),
		metric.WithInt64Callback(mc.observeActiveConnections),
	)
	if err != nil {
		return nil, err
	}
	mc.activeConnectionsGauge = activeConnectionsGauge

	return mc, nil
}

// RecordConnection records a new WebSocket connection
func (mc *MetricsCollector) RecordConnection(ctx context.Context, status string, userType string) {
	// Prometheus
	wsConnectionsTotal.WithLabelValues(status, userType).Inc()
	if status == "connected" {
		wsConnectionsActive.WithLabelValues("", userType).Inc()
	}

	// OpenTelemetry
	mc.connectionsCounter.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("status", status),
			attribute.String("user_type", userType),
		),
	)
}

// RecordDisconnection records a WebSocket disconnection
func (mc *MetricsCollector) RecordDisconnection(ctx context.Context, reason string, duration time.Duration) {
	// Prometheus
	wsConnectionsActive.WithLabelValues("", "").Dec()
	wsSessionDuration.WithLabelValues(reason).Observe(duration.Seconds())

	// OpenTelemetry
	mc.connectionsCounter.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("status", "disconnected"),
			attribute.String("reason", reason),
		),
	)
}

// RecordMessage records a WebSocket message
func (mc *MetricsCollector) RecordMessage(ctx context.Context, msgType, direction, status string, size int, latency time.Duration) {
	// Prometheus
	wsMessagesTotal.WithLabelValues(msgType, direction, status).Inc()
	wsMessageSize.WithLabelValues(msgType, direction).Observe(float64(size))
	if latency > 0 {
		wsMessageLatency.WithLabelValues(msgType, direction).Observe(float64(latency.Milliseconds()))
	}

	// OpenTelemetry
	mc.messagesCounter.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("type", msgType),
			attribute.String("direction", direction),
			attribute.String("status", status),
		),
	)

	if latency > 0 {
		mc.latencyHistogram.Record(ctx, float64(latency.Milliseconds()),
			metric.WithAttributes(
				attribute.String("operation", "message_processing"),
				attribute.String("type", msgType),
			),
		)
	}
}

// RecordReconnection records a reconnection attempt
func (mc *MetricsCollector) RecordReconnection(reason string, success bool) {
	successStr := "false"
	if success {
		successStr = "true"
	}
	wsReconnections.WithLabelValues(reason, successStr).Inc()
}

// RecordRateLimitHit records a rate limit hit
func (mc *MetricsCollector) RecordRateLimitHit(userID, limitType string) {
	wsRateLimitHits.WithLabelValues(userID, limitType).Inc()
}

// RecordAuthFailure records an authentication failure
func (mc *MetricsCollector) RecordAuthFailure(reason string) {
	wsAuthFailures.WithLabelValues(reason).Inc()
}

// RecordHealthCheckLatency records health check latency
func (mc *MetricsCollector) RecordHealthCheckLatency(latency time.Duration) {
	wsHealthCheckLatency.Observe(float64(latency.Milliseconds()))
}

// RecordSequenceGap records a detected sequence gap
func (mc *MetricsCollector) RecordSequenceGap(partition string, recovered bool) {
	recoveredStr := "false"
	if recovered {
		recoveredStr = "true"
	}
	wsSequenceGaps.WithLabelValues(partition, recoveredStr).Inc()
}

// UpdateBufferSize updates the current buffer size metric
func (mc *MetricsCollector) UpdateBufferSize(bufferType, partition string, size int) {
	wsBufferSize.WithLabelValues(bufferType, partition).Set(float64(size))
}

// UpdateRoomSize updates the room size metric
func (mc *MetricsCollector) UpdateRoomSize(roomID string, size int) {
	wsRoomSize.WithLabelValues(roomID).Set(float64(size))
}

// observeActiveConnections is a callback for the active connections gauge
func (mc *MetricsCollector) observeActiveConnections(_ context.Context, observer metric.Int64Observer) error {
	// This would typically get the actual count from the Hub
	// For now, we'll use the Prometheus gauge value
	// In production, this should query the actual state
	return nil
}

// StartSpan starts a new OpenTelemetry span
func (mc *MetricsCollector) StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return mc.tracer.Start(ctx, name, opts...)
}

// TrackedOperation wraps an operation with metrics and tracing
func (mc *MetricsCollector) TrackedOperation(ctx context.Context, name string, operation func() error) error {
	// Start span
	ctx, span := mc.StartSpan(ctx, name)
	defer span.End()

	// Track timing
	start := time.Now()

	// Execute operation
	err := operation()

	// Record latency
	latency := time.Since(start)
	mc.latencyHistogram.Record(ctx, float64(latency.Milliseconds()),
		metric.WithAttributes(
			attribute.String("operation", name),
			attribute.Bool("success", err == nil),
		),
	)

	// Record error if any
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())

		mc.logger.Error("Operation failed",
			zap.String("operation", name),
			zap.Error(err),
			zap.Duration("latency", latency),
		)
	} else {
		span.SetStatus(codes.Ok, "")
	}

	return err
}

// MetricsMiddleware provides metrics collection for WebSocket handlers
type MetricsMiddleware struct {
	collector *MetricsCollector
	logger    *zap.Logger
}

// NewMetricsMiddleware creates a new metrics middleware
func NewMetricsMiddleware(logger *zap.Logger) (*MetricsMiddleware, error) {
	collector, err := NewMetricsCollector(logger)
	if err != nil {
		return nil, err
	}

	return &MetricsMiddleware{
		collector: collector,
		logger:    logger,
	}, nil
}

// WrapHandler wraps a WebSocket handler with metrics collection
func (mm *MetricsMiddleware) WrapHandler(handler func(*Client)) func(*Client) {
	return func(client *Client) {
		// Record connection
		ctx := context.Background()
		mm.collector.RecordConnection(ctx, "connected", "authenticated")

		// Track session duration
		sessionStart := time.Now()

		// Execute handler
		handler(client)

		// Record disconnection
		duration := time.Since(sessionStart)
		mm.collector.RecordDisconnection(ctx, "normal", duration)
	}
}

// StructuredLogger provides structured logging with context
type StructuredLogger struct {
	logger *zap.Logger
}

// NewStructuredLogger creates a new structured logger
func NewStructuredLogger(logger *zap.Logger) *StructuredLogger {
	return &StructuredLogger{
		logger: logger,
	}
}

// LogConnection logs a connection event
func (sl *StructuredLogger) LogConnection(userID string, sessionID string, remoteAddr string) {
	sl.logger.Info("WebSocket connection established",
		zap.String("user_id", userID),
		zap.String("session_id", sessionID),
		zap.String("remote_addr", remoteAddr),
		zap.Time("connected_at", time.Now()),
	)
}

// LogMessage logs a message event
func (sl *StructuredLogger) LogMessage(userID string, msgType string, size int, success bool) {
	if success {
		sl.logger.Debug("WebSocket message processed",
			zap.String("user_id", userID),
			zap.String("message_type", msgType),
			zap.Int("size", size),
		)
	} else {
		sl.logger.Warn("WebSocket message processing failed",
			zap.String("user_id", userID),
			zap.String("message_type", msgType),
			zap.Int("size", size),
		)
	}
}

// LogDisconnection logs a disconnection event
func (sl *StructuredLogger) LogDisconnection(userID string, sessionID string, reason string, duration time.Duration) {
	sl.logger.Info("WebSocket connection closed",
		zap.String("user_id", userID),
		zap.String("session_id", sessionID),
		zap.String("reason", reason),
		zap.Duration("session_duration", duration),
		zap.Time("disconnected_at", time.Now()),
	)
}

// LogError logs an error event
func (sl *StructuredLogger) LogError(userID string, operation string, err error) {
	sl.logger.Error("WebSocket operation error",
		zap.String("user_id", userID),
		zap.String("operation", operation),
		zap.Error(err),
	)
}