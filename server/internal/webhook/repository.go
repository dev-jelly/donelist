package webhook

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Repository handles database operations for webhooks
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a new webhook repository
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// CreateWebhook creates a new webhook
func (r *Repository) CreateWebhook(ctx context.Context, webhook *Webhook) error {
	return r.db.WithContext(ctx).Create(webhook).Error
}

// UpdateWebhook updates an existing webhook
func (r *Repository) UpdateWebhook(ctx context.Context, webhook *Webhook) error {
	return r.db.WithContext(ctx).Save(webhook).Error
}

// DeleteWebhook soft deletes a webhook
func (r *Repository) DeleteWebhook(ctx context.Context, webhookID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Model(&Webhook{}).
		Where("id = ?", webhookID).
		Update("deleted_at", time.Now()).Error
}

// GetWebhookByID gets a webhook by ID
func (r *Repository) GetWebhookByID(ctx context.Context, webhookID uuid.UUID) (*Webhook, error) {
	var webhook Webhook
	err := r.db.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", webhookID).
		First(&webhook).Error
	if err != nil {
		return nil, err
	}
	return &webhook, nil
}

// GetWebhookByIDAndUser gets a webhook by ID and user ID
func (r *Repository) GetWebhookByIDAndUser(ctx context.Context, webhookID, userID uuid.UUID) (*Webhook, error) {
	var webhook Webhook
	err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ? AND deleted_at IS NULL", webhookID, userID).
		First(&webhook).Error
	if err != nil {
		return nil, err
	}
	return &webhook, nil
}

// ListWebhooksByUser lists webhooks for a user
func (r *Repository) ListWebhooksByUser(ctx context.Context, userID uuid.UUID) ([]*Webhook, error) {
	var webhooks []*Webhook
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Order("created_at DESC").
		Find(&webhooks).Error
	return webhooks, err
}

// GetActiveWebhooksForEvent gets active webhooks subscribed to an event
// Optimized version using PostgreSQL array containment operator (@>)
// Expected 10-100x improvement (50-200ms to 2-5ms) for users with many webhooks
func (r *Repository) GetActiveWebhooksForEvent(ctx context.Context, userID uuid.UUID, eventType EventType) ([]*Webhook, error) {
	var webhooks []*Webhook

	// Use PostgreSQL array containment operator (@>) for efficient filtering at DB level
	// This replaces in-memory filtering of potentially large webhook lists
	// Requires GIN index on events column for optimal performance
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND active = ? AND deleted_at IS NULL", userID, true).
		Where("events @> ARRAY[?]::varchar[] OR events @> ARRAY['*']::varchar[]", string(eventType)).
		Find(&webhooks).Error

	if err != nil {
		return nil, err
	}

	return webhooks, nil
}

// UpdateWebhookStats updates webhook statistics
func (r *Repository) UpdateWebhookStats(ctx context.Context, webhookID uuid.UUID, success bool) error {
	updates := map[string]interface{}{
		"last_triggered_at": time.Now(),
		"total_deliveries":  gorm.Expr("total_deliveries + 1"),
	}
	if !success {
		updates["failed_deliveries"] = gorm.Expr("failed_deliveries + 1")
	}
	return r.db.WithContext(ctx).
		Model(&Webhook{}).
		Where("id = ?", webhookID).
		Updates(updates).Error
}

// CreateDelivery creates a new webhook delivery
func (r *Repository) CreateDelivery(ctx context.Context, delivery *WebhookDelivery) error {
	return r.db.WithContext(ctx).Create(delivery).Error
}

// UpdateDelivery updates a webhook delivery
func (r *Repository) UpdateDelivery(ctx context.Context, delivery *WebhookDelivery) error {
	return r.db.WithContext(ctx).Save(delivery).Error
}

// GetDeliveryByID gets a delivery by ID
func (r *Repository) GetDeliveryByID(ctx context.Context, deliveryID uuid.UUID) (*WebhookDelivery, error) {
	var delivery WebhookDelivery
	err := r.db.WithContext(ctx).Where("id = ?", deliveryID).First(&delivery).Error
	if err != nil {
		return nil, err
	}
	return &delivery, nil
}

// ListDeliveriesByWebhook lists deliveries for a webhook
func (r *Repository) ListDeliveriesByWebhook(ctx context.Context, webhookID uuid.UUID, limit int) ([]*WebhookDelivery, error) {
	var deliveries []*WebhookDelivery
	query := r.db.WithContext(ctx).
		Where("webhook_id = ?", webhookID).
		Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&deliveries).Error
	return deliveries, err
}

// GetPendingRetries gets deliveries that need to be retried
func (r *Repository) GetPendingRetries(ctx context.Context, limit int) ([]*WebhookDelivery, error) {
	var deliveries []*WebhookDelivery
	err := r.db.WithContext(ctx).
		Where("status = ? AND next_retry_at <= ?", StatusRetrying, time.Now()).
		Order("next_retry_at ASC").
		Limit(limit).
		Find(&deliveries).Error
	return deliveries, err
}

// MoveToDLQ moves a failed delivery to the dead letter queue
func (r *Repository) MoveToDLQ(ctx context.Context, delivery *WebhookDelivery, webhook *Webhook) error {
	dlq := &WebhookDLQ{
		ID:                 uuid.New(),
		WebhookID:          delivery.WebhookID,
		WebhookURL:         webhook.URL,
		EventType:          delivery.EventType,
		EventID:            delivery.EventID,
		Payload:            delivery.Payload,
		TotalAttempts:      delivery.Attempts,
		LastError:          delivery.Error,
		LastStatusCode:     delivery.StatusCode,
		OriginalDeliveryID: delivery.ID,
		CreatedAt:          time.Now(),
		FailedAt:           time.Now(),
	}

	return r.db.WithContext(ctx).Create(dlq).Error
}

// GetDLQEntries gets dead letter queue entries
func (r *Repository) GetDLQEntries(ctx context.Context, webhookID uuid.UUID, limit int) ([]*WebhookDLQ, error) {
	var entries []*WebhookDLQ
	query := r.db.WithContext(ctx).Order("failed_at DESC")

	if webhookID != uuid.Nil {
		query = query.Where("webhook_id = ?", webhookID)
	}

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&entries).Error
	return entries, err
}

// GetEventLogs gets webhook event logs
func (r *Repository) GetEventLogs(ctx context.Context, webhookID uuid.UUID, limit int) ([]*WebhookEventLog, error) {
	var logs []*WebhookEventLog
	query := r.db.WithContext(ctx).
		Where("webhook_id = ?", webhookID).
		Order("logged_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&logs).Error
	return logs, err
}

// GetWebhookStats gets aggregated statistics for a webhook
func (r *Repository) GetWebhookStats(ctx context.Context, webhookID uuid.UUID, since time.Time) (*WebhookStats, error) {
	stats := &WebhookStats{
		WebhookID: webhookID,
	}

	// Get total deliveries
	r.db.WithContext(ctx).
		Model(&WebhookDelivery{}).
		Where("webhook_id = ? AND created_at >= ?", webhookID, since).
		Count(&stats.TotalDeliveries)

	// Get successful deliveries
	r.db.WithContext(ctx).
		Model(&WebhookDelivery{}).
		Where("webhook_id = ? AND status = ? AND created_at >= ?", webhookID, StatusDelivered, since).
		Count(&stats.SuccessfulDeliveries)

	// Get failed deliveries
	r.db.WithContext(ctx).
		Model(&WebhookDelivery{}).
		Where("webhook_id = ? AND status = ? AND created_at >= ?", webhookID, StatusFailed, since).
		Count(&stats.FailedDeliveries)

	// Get average response time
	var avgDuration float64
	r.db.WithContext(ctx).
		Model(&WebhookEventLog{}).
		Where("webhook_id = ? AND logged_at >= ? AND duration_ms IS NOT NULL", webhookID, since).
		Select("AVG(duration_ms)").
		Scan(&avgDuration)
	stats.AvgResponseTimeMs = int(avgDuration)

	return stats, nil
}

// CleanupOldDeliveries deletes old delivery records
func (r *Repository) CleanupOldDeliveries(ctx context.Context, olderThan time.Time) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("created_at < ? AND status IN (?, ?)", olderThan, StatusDelivered, StatusFailed).
		Delete(&WebhookDelivery{})
	return result.RowsAffected, result.Error
}

// CleanupOldEventLogs deletes old event logs
func (r *Repository) CleanupOldEventLogs(ctx context.Context, olderThan time.Time) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("logged_at < ?", olderThan).
		Delete(&WebhookEventLog{})
	return result.RowsAffected, result.Error
}
