package notification

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

var (
	// Prometheus metrics
	notificationsSentTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "notifications_sent_total",
			Help: "Total number of notifications sent",
		},
		[]string{"type", "platform", "tenant", "priority"},
	)

	notificationsFailedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "notifications_failed_total",
			Help: "Total number of failed notifications",
		},
		[]string{"type", "platform", "tenant", "error_type"},
	)

	notificationsRetriedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "notifications_retried_total",
			Help: "Total number of notification retries",
		},
		[]string{"type", "platform", "tenant"},
	)

	notificationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "notification_duration_seconds",
			Help:    "Duration of notification processing",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms to ~32s
		},
		[]string{"type", "platform", "status"},
	)

	queueDepth = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "notification_queue_depth",
			Help: "Current depth of notification queue",
		},
		[]string{"priority", "shard"},
	)

	activeWorkers = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "notification_active_workers",
			Help: "Number of active notification workers",
		},
		[]string{"shard"},
	)

	quotaUsage = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "notification_quota_usage",
			Help: "Current quota usage by tenant",
		},
		[]string{"tenant", "period"},
	)

	quotaLimit = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "notification_quota_limit",
			Help: "Quota limit by tenant",
		},
		[]string{"tenant", "period"},
	)

	dndBlockedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "notifications_dnd_blocked_total",
			Help: "Total number of notifications blocked by DND",
		},
		[]string{"type", "tenant"},
	)

	dlqMessages = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "notification_dlq_messages",
			Help: "Number of messages in dead letter queue",
		},
		[]string{"reason"},
	)
)

// NotificationTracer handles OpenTelemetry tracing
type NotificationTracer struct {
	tracer trace.Tracer
	meter  metric.Meter
	logger *zap.Logger

	// OpenTelemetry metrics
	sentCounter    metric.Int64Counter
	failedCounter  metric.Int64Counter
	durationHist   metric.Float64Histogram
	queueGauge     metric.Int64UpDownCounter
}

// NewNotificationTracer creates a new tracer
func NewNotificationTracer(logger *zap.Logger) (*NotificationTracer, error) {
	tracer := otel.Tracer("notification-service")
	meter := otel.Meter("notification-service")

	sentCounter, err := meter.Int64Counter(
		"notification.sent",
		metric.WithDescription("Number of notifications sent"),
		metric.WithUnit("{notification}"),
	)
	if err != nil {
		return nil, err
	}

	failedCounter, err := meter.Int64Counter(
		"notification.failed",
		metric.WithDescription("Number of failed notifications"),
		metric.WithUnit("{notification}"),
	)
	if err != nil {
		return nil, err
	}

	durationHist, err := meter.Float64Histogram(
		"notification.duration",
		metric.WithDescription("Duration of notification processing"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	queueGauge, err := meter.Int64UpDownCounter(
		"notification.queue_depth",
		metric.WithDescription("Current notification queue depth"),
		metric.WithUnit("{notification}"),
	)
	if err != nil {
		return nil, err
	}

	return &NotificationTracer{
		tracer:        tracer,
		meter:         meter,
		logger:        logger,
		sentCounter:   sentCounter,
		failedCounter: failedCounter,
		durationHist:  durationHist,
		queueGauge:    queueGauge,
	}, nil
}

// StartSpan starts a new trace span
func (nt *NotificationTracer) StartSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return nt.tracer.Start(ctx, name, trace.WithAttributes(attrs...))
}

// RecordSent records a successful notification send
func (nt *NotificationTracer) RecordSent(ctx context.Context, notifType, platform, tenant string, priority Priority) {
	// Prometheus metrics
	notificationsSentTotal.WithLabelValues(notifType, platform, tenant, string(priority)).Inc()

	// OpenTelemetry metrics
	nt.sentCounter.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("type", notifType),
			attribute.String("platform", platform),
			attribute.String("tenant", tenant),
			attribute.String("priority", string(priority)),
		),
	)

	// Structured logging
	nt.logger.Info("Notification sent",
		zap.String("type", notifType),
		zap.String("platform", platform),
		zap.String("tenant", tenant),
		zap.String("priority", string(priority)),
	)
}

// RecordFailed records a failed notification
func (nt *NotificationTracer) RecordFailed(ctx context.Context, notifType, platform, tenant, errorType string) {
	// Prometheus metrics
	notificationsFailedTotal.WithLabelValues(notifType, platform, tenant, errorType).Inc()

	// OpenTelemetry metrics
	nt.failedCounter.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("type", notifType),
			attribute.String("platform", platform),
			attribute.String("tenant", tenant),
			attribute.String("error_type", errorType),
		),
	)

	// Structured logging
	nt.logger.Error("Notification failed",
		zap.String("type", notifType),
		zap.String("platform", platform),
		zap.String("tenant", tenant),
		zap.String("error_type", errorType),
	)
}

// RecordRetry records a notification retry
func (nt *NotificationTracer) RecordRetry(ctx context.Context, notifType, platform, tenant string, attempt int) {
	// Prometheus metrics
	notificationsRetriedTotal.WithLabelValues(notifType, platform, tenant).Inc()

	// Structured logging
	nt.logger.Warn("Notification retry",
		zap.String("type", notifType),
		zap.String("platform", platform),
		zap.String("tenant", tenant),
		zap.Int("attempt", attempt),
	)
}

// RecordDuration records processing duration
func (nt *NotificationTracer) RecordDuration(ctx context.Context, notifType, platform, status string, duration time.Duration) {
	seconds := duration.Seconds()

	// Prometheus metrics
	notificationDuration.WithLabelValues(notifType, platform, status).Observe(seconds)

	// OpenTelemetry metrics
	nt.durationHist.Record(ctx, float64(duration.Milliseconds()),
		metric.WithAttributes(
			attribute.String("type", notifType),
			attribute.String("platform", platform),
			attribute.String("status", status),
		),
	)

	// Log slow notifications
	if duration > 5*time.Second {
		nt.logger.Warn("Slow notification processing",
			zap.String("type", notifType),
			zap.String("platform", platform),
			zap.Duration("duration", duration),
		)
	}
}

// RecordQueueDepth records current queue depth
func (nt *NotificationTracer) RecordQueueDepth(ctx context.Context, priority Priority, shardID int, depth int64) {
	// Prometheus metrics
	queueDepth.WithLabelValues(string(priority), string(rune(shardID))).Set(float64(depth))

	// OpenTelemetry metrics
	nt.queueGauge.Add(ctx, depth,
		metric.WithAttributes(
			attribute.String("priority", string(priority)),
			attribute.Int("shard", shardID),
		),
	)
}

// RecordActiveWorkers records number of active workers
func (nt *NotificationTracer) RecordActiveWorkers(shardID int, count int) {
	activeWorkers.WithLabelValues(string(rune(shardID))).Set(float64(count))
}

// RecordQuotaUsage records quota usage for a tenant
func (nt *NotificationTracer) RecordQuotaUsage(tenant string, period string, usage, limit int) {
	quotaUsage.WithLabelValues(tenant, period).Set(float64(usage))
	quotaLimit.WithLabelValues(tenant, period).Set(float64(limit))

	// Log when approaching quota
	if limit > 0 && float64(usage)/float64(limit) > 0.8 {
		nt.logger.Warn("Quota usage high",
			zap.String("tenant", tenant),
			zap.String("period", period),
			zap.Int("usage", usage),
			zap.Int("limit", limit),
			zap.Float64("percentage", float64(usage)/float64(limit)*100),
		)
	}
}

// RecordDNDBlock records a notification blocked by DND
func (nt *NotificationTracer) RecordDNDBlock(ctx context.Context, notifType, tenant string) {
	dndBlockedTotal.WithLabelValues(notifType, tenant).Inc()

	nt.logger.Debug("Notification blocked by DND",
		zap.String("type", notifType),
		zap.String("tenant", tenant),
	)
}

// RecordDLQMessage records a message sent to DLQ
func (nt *NotificationTracer) RecordDLQMessage(reason string, count int) {
	dlqMessages.WithLabelValues(reason).Set(float64(count))

	nt.logger.Warn("Message sent to DLQ",
		zap.String("reason", reason),
		zap.Int("count", count),
	)
}

// CorrelationIDKey is the context key for correlation ID
type contextKey string

const CorrelationIDKey contextKey = "correlation_id"

// WithCorrelationID adds a correlation ID to the context
func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, CorrelationIDKey, correlationID)
}

// GetCorrelationID retrieves the correlation ID from context
func GetCorrelationID(ctx context.Context) string {
	if id, ok := ctx.Value(CorrelationIDKey).(string); ok {
		return id
	}
	return ""
}

// LogWithCorrelation creates a logger with correlation ID
func LogWithCorrelation(logger *zap.Logger, ctx context.Context) *zap.Logger {
	if correlationID := GetCorrelationID(ctx); correlationID != "" {
		return logger.With(zap.String("correlation_id", correlationID))
	}
	return logger
}

// NotificationMetrics aggregates metrics for reporting
type NotificationMetrics struct {
	TotalSent      int64                 `json:"total_sent"`
	TotalFailed    int64                 `json:"total_failed"`
	TotalRetried   int64                 `json:"total_retried"`
	DNDBlocked     int64                 `json:"dnd_blocked"`
	AverageDuration float64              `json:"average_duration_ms"`
	QueueDepth     map[string]int64      `json:"queue_depth"`
	QuotaUsage     map[string]QuotaStats `json:"quota_usage"`
}

// QuotaStats represents quota statistics
type QuotaStats struct {
	Usage      int     `json:"usage"`
	Limit      int     `json:"limit"`
	Percentage float64 `json:"percentage"`
}

// GetMetrics returns current metrics snapshot
func GetMetrics(router *ShardRouter) *NotificationMetrics {
	metrics := &NotificationMetrics{
		QueueDepth: make(map[string]int64),
		QuotaUsage: make(map[string]QuotaStats),
	}

	// Get quota information
	if router != nil {
		// This would normally query the router for tenant quotas
		// For now, we'll just return the structure
	}

	return metrics
}
