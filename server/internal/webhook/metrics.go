package webhook

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Webhook delivery metrics
	webhookDeliveriesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "webhook_deliveries_total",
			Help: "Total number of webhook deliveries",
		},
		[]string{"webhook_id", "event_type", "status"},
	)

	webhookDeliveryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "webhook_delivery_duration_seconds",
			Help:    "Duration of webhook deliveries in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"webhook_id", "event_type"},
	)

	webhookDeliveryAttempts = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "webhook_delivery_attempts",
			Help:    "Number of delivery attempts",
			Buckets: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		},
		[]string{"webhook_id", "event_type"},
	)

	webhookActiveCount = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "webhook_active_count",
			Help: "Number of active webhooks",
		},
		[]string{"user_id"},
	)

	webhookQueueSize = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "webhook_queue_size",
			Help: "Current size of webhook delivery queue",
		},
	)

	webhookDLQSize = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "webhook_dlq_size",
			Help: "Current size of webhook dead letter queue",
		},
	)

	webhookErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "webhook_errors_total",
			Help: "Total number of webhook errors",
		},
		[]string{"webhook_id", "event_type", "error_type"},
	)
)

// MetricsCollector collects webhook metrics
type MetricsCollector struct {
	repo *Repository
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(repo *Repository) *MetricsCollector {
	return &MetricsCollector{repo: repo}
}

// RecordDeliverySuccess records a successful webhook delivery
func (m *MetricsCollector) RecordDeliverySuccess(webhookID, eventType string, duration float64, attempts int) {
	webhookDeliveriesTotal.WithLabelValues(webhookID, eventType, "success").Inc()
	webhookDeliveryDuration.WithLabelValues(webhookID, eventType).Observe(duration)
	webhookDeliveryAttempts.WithLabelValues(webhookID, eventType).Observe(float64(attempts))
}

// RecordDeliveryFailure records a failed webhook delivery
func (m *MetricsCollector) RecordDeliveryFailure(webhookID, eventType, errorType string, duration float64, attempts int) {
	webhookDeliveriesTotal.WithLabelValues(webhookID, eventType, "failure").Inc()
	webhookDeliveryDuration.WithLabelValues(webhookID, eventType).Observe(duration)
	webhookDeliveryAttempts.WithLabelValues(webhookID, eventType).Observe(float64(attempts))
	webhookErrors.WithLabelValues(webhookID, eventType, errorType).Inc()
}

// RecordDeliveryRetry records a webhook delivery retry
func (m *MetricsCollector) RecordDeliveryRetry(webhookID, eventType string) {
	webhookDeliveriesTotal.WithLabelValues(webhookID, eventType, "retry").Inc()
}

// UpdateActiveWebhookCount updates the active webhook count
func (m *MetricsCollector) UpdateActiveWebhookCount(userID string, count float64) {
	webhookActiveCount.WithLabelValues(userID).Set(count)
}

// UpdateQueueSize updates the queue size metric
func (m *MetricsCollector) UpdateQueueSize(size float64) {
	webhookQueueSize.Set(size)
}

// UpdateDLQSize updates the DLQ size metric
func (m *MetricsCollector) UpdateDLQSize(size float64) {
	webhookDLQSize.Set(size)
}
