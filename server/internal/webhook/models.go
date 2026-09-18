package webhook

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// EventType represents the type of webhook event
type EventType string

const (
	EventCheckinCreated      EventType = "checkin.created"
	EventCheckinUpdated      EventType = "checkin.updated"
	EventCheckinDeleted      EventType = "checkin.deleted"
	EventCategoryCreated     EventType = "category.created"
	EventCategoryUpdated     EventType = "category.updated"
	EventCategoryDeleted     EventType = "category.deleted"
	EventUserUpdated         EventType = "user.updated"
	EventUserUpgraded        EventType = "user.upgraded"
	EventSubscriptionChanged EventType = "subscription.changed"
)

// Webhook represents a webhook configuration
type Webhook struct {
	ID      uuid.UUID `json:"id" gorm:"type:uuid;primaryKey"`
	UserID  uuid.UUID `json:"user_id" gorm:"type:uuid;index"`
	Name    string    `json:"name"`
	URL     string    `json:"url"`
	Secret  string    `json:"-" gorm:"column:secret"` // Used for HMAC signature
	Events  []string  `json:"events" gorm:"serializer:json"`
	Active  bool      `json:"active"`
	Headers map[string]string `json:"headers" gorm:"serializer:json"`

	// Statistics
	LastTriggeredAt  *time.Time `json:"last_triggered_at,omitempty"`
	TotalDeliveries  int        `json:"total_deliveries"`
	FailedDeliveries int        `json:"failed_deliveries"`

	// Timestamps
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty" gorm:"index"`
}

// TableName returns the table name for Webhook
func (Webhook) TableName() string {
	return "webhooks"
}

// WebhookDelivery represents a webhook delivery attempt
type WebhookDelivery struct {
	ID          uuid.UUID       `json:"id" gorm:"type:uuid;primaryKey"`
	WebhookID   uuid.UUID       `json:"webhook_id" gorm:"type:uuid;index"`
	EventType   EventType       `json:"event_type"`
	EventID     string          `json:"event_id"`
	Payload     json.RawMessage `json:"payload" gorm:"serializer:json"`
	Status      DeliveryStatus  `json:"status"`
	Attempts    int             `json:"attempts"`
	StatusCode  *int            `json:"status_code,omitempty"`
	Response    *string         `json:"response,omitempty" gorm:"type:text"`
	Error       *string         `json:"error,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	DeliveredAt *time.Time      `json:"delivered_at,omitempty"`
	NextRetryAt *time.Time      `json:"next_retry_at,omitempty"`
}

// TableName returns the table name for WebhookDelivery
func (WebhookDelivery) TableName() string {
	return "webhook_deliveries"
}

// DeliveryStatus represents the status of webhook delivery
type DeliveryStatus string

const (
	StatusPending   DeliveryStatus = "pending"
	StatusDelivered DeliveryStatus = "delivered"
	StatusFailed    DeliveryStatus = "failed"
	StatusRetrying  DeliveryStatus = "retrying"
	StatusSkipped   DeliveryStatus = "skipped"
)

// WebhookDLQ represents a dead letter queue entry
type WebhookDLQ struct {
	ID                 uuid.UUID       `json:"id" gorm:"type:uuid;primaryKey"`
	WebhookID          uuid.UUID       `json:"webhook_id" gorm:"type:uuid;index"`
	WebhookURL         string          `json:"webhook_url"`
	EventType          EventType       `json:"event_type"`
	EventID            string          `json:"event_id"`
	Payload            json.RawMessage `json:"payload" gorm:"serializer:json"`
	TotalAttempts      int             `json:"total_attempts"`
	LastError          *string         `json:"last_error,omitempty"`
	LastStatusCode     *int            `json:"last_status_code,omitempty"`
	OriginalDeliveryID uuid.UUID       `json:"original_delivery_id"`
	CreatedAt          time.Time       `json:"created_at"`
	FailedAt           time.Time       `json:"failed_at"`
	Metadata           json.RawMessage `json:"metadata,omitempty" gorm:"serializer:json"`
}

// TableName returns the table name for WebhookDLQ
func (WebhookDLQ) TableName() string {
	return "webhook_dead_letter_queue"
}

// WebhookEventLog represents a webhook event log entry
type WebhookEventLog struct {
	ID            int64          `json:"id" gorm:"primaryKey;autoIncrement"`
	WebhookID     uuid.UUID      `json:"webhook_id" gorm:"type:uuid;index"`
	DeliveryID    uuid.UUID      `json:"delivery_id" gorm:"type:uuid"`
	EventType     string         `json:"event_type"`
	Status        DeliveryStatus `json:"status"`
	AttemptNumber int            `json:"attempt_number"`
	DurationMs    *int           `json:"duration_ms,omitempty"`
	StatusCode    *int           `json:"status_code,omitempty"`
	Error         *string        `json:"error,omitempty"`
	LoggedAt      time.Time      `json:"logged_at"`
}

// TableName returns the table name for WebhookEventLog
func (WebhookEventLog) TableName() string {
	return "webhook_event_logs"
}

// WebhookPayload represents the payload sent to webhooks
type WebhookPayload struct {
	Event     string          `json:"event"`
	EventID   string          `json:"event_id"`
	Webhook   string          `json:"webhook"`
	Timestamp int64           `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

// WebhookStats represents aggregated webhook statistics
type WebhookStats struct {
	WebhookID            uuid.UUID `json:"webhook_id"`
	TotalDeliveries      int64     `json:"total_deliveries"`
	SuccessfulDeliveries int64     `json:"successful_deliveries"`
	FailedDeliveries     int64     `json:"failed_deliveries"`
	AvgResponseTimeMs    int       `json:"avg_response_time_ms"`
}

// CreateWebhookRequest represents a request to create a webhook
type CreateWebhookRequest struct {
	Name    string            `json:"name" binding:"required"`
	URL     string            `json:"url" binding:"required,url"`
	Events  []string          `json:"events" binding:"required,min=1"`
	Secret  string            `json:"secret"`
	Active  bool              `json:"active"`
	Headers map[string]string `json:"headers"`
}

// UpdateWebhookRequest represents a request to update a webhook
type UpdateWebhookRequest struct {
	Name    *string           `json:"name"`
	URL     *string           `json:"url" binding:"omitempty,url"`
	Events  []string          `json:"events" binding:"omitempty,min=1"`
	Active  *bool             `json:"active"`
	Headers map[string]string `json:"headers"`
}

// WebhookResponse represents a webhook response
type WebhookResponse struct {
	*Webhook
	SecretPreview string `json:"secret_preview,omitempty"`
}

// ToResponse converts a Webhook to WebhookResponse with secret preview
func (w *Webhook) ToResponse() *WebhookResponse {
	resp := &WebhookResponse{
		Webhook: w,
	}
	if len(w.Secret) > 8 {
		resp.SecretPreview = w.Secret[:4] + "..." + w.Secret[len(w.Secret)-4:]
	}
	return resp
}
