package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// EventType represents the type of webhook event
type EventType string

const (
	EventCheckinCreated   EventType = "checkin.created"
	EventCheckinUpdated   EventType = "checkin.updated"
	EventCheckinDeleted   EventType = "checkin.deleted"
	EventCategoryCreated  EventType = "category.created"
	EventCategoryUpdated  EventType = "category.updated"
	EventCategoryDeleted  EventType = "category.deleted"
	EventUserUpdated      EventType = "user.updated"
	EventSubscriptionChanged EventType = "subscription.changed"
)

// Webhook represents a webhook configuration
type Webhook struct {
	ID        uuid.UUID  `json:"id" gorm:"type:uuid;primaryKey"`
	UserID    uuid.UUID  `json:"user_id" gorm:"type:uuid;index"`
	Name      string     `json:"name"`
	URL       string     `json:"url"`
	Secret    string     `json:"-" gorm:"column:secret"` // Used for HMAC signature
	Events    []string   `json:"events" gorm:"type:jsonb"`
	Active    bool       `json:"active"`
	Headers   map[string]string `json:"headers" gorm:"type:jsonb"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty" gorm:"index"`

	// Statistics
	LastTriggeredAt *time.Time `json:"last_triggered_at,omitempty"`
	TotalDeliveries int        `json:"total_deliveries"`
	FailedDeliveries int       `json:"failed_deliveries"`
}

// TableName returns the table name for Webhook
func (Webhook) TableName() string {
	return "webhooks"
}

// WebhookDelivery represents a webhook delivery attempt
type WebhookDelivery struct {
	ID         uuid.UUID  `json:"id" gorm:"type:uuid;primaryKey"`
	WebhookID  uuid.UUID  `json:"webhook_id" gorm:"type:uuid;index"`
	EventType  EventType  `json:"event_type"`
	EventID    string     `json:"event_id"`
	Payload    json.RawMessage `json:"payload" gorm:"type:jsonb"`
	Status     DeliveryStatus  `json:"status"`
	Attempts   int        `json:"attempts"`
	StatusCode *int       `json:"status_code,omitempty"`
	Response   *string    `json:"response,omitempty" gorm:"type:text"`
	Error      *string    `json:"error,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	DeliveredAt *time.Time `json:"delivered_at,omitempty"`
	NextRetryAt *time.Time `json:"next_retry_at,omitempty"`
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

// Service handles webhook operations
type Service struct {
	db         *gorm.DB
	httpClient *http.Client
	logger     *zap.Logger
	queue      chan *WebhookDelivery
}

// NewService creates a new webhook service
func NewService(db *gorm.DB, logger *zap.Logger) *Service {
	service := &Service{
		db: db,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
		queue:  make(chan *WebhookDelivery, 1000),
	}

	// Start background workers
	go service.processDeliveryQueue()
	go service.processRetries()

	return service
}

// CreateWebhook creates a new webhook
func (s *Service) CreateWebhook(ctx context.Context, webhook *Webhook) error {
	webhook.ID = uuid.New()
	webhook.CreatedAt = time.Now()
	webhook.UpdatedAt = time.Now()

	// Generate secret if not provided
	if webhook.Secret == "" {
		webhook.Secret = s.generateSecret()
	}

	if err := s.db.WithContext(ctx).Create(webhook).Error; err != nil {
		return fmt.Errorf("failed to create webhook: %w", err)
	}

	s.logger.Info("Webhook created",
		zap.String("id", webhook.ID.String()),
		zap.String("url", webhook.URL),
		zap.Strings("events", webhook.Events),
	)

	return nil
}

// UpdateWebhook updates an existing webhook
func (s *Service) UpdateWebhook(ctx context.Context, webhook *Webhook) error {
	webhook.UpdatedAt = time.Now()

	if err := s.db.WithContext(ctx).Save(webhook).Error; err != nil {
		return fmt.Errorf("failed to update webhook: %w", err)
	}

	return nil
}

// DeleteWebhook soft deletes a webhook
func (s *Service) DeleteWebhook(ctx context.Context, webhookID uuid.UUID) error {
	now := time.Now()
	err := s.db.WithContext(ctx).
		Model(&Webhook{}).
		Where("id = ?", webhookID).
		Update("deleted_at", now).Error

	if err != nil {
		return fmt.Errorf("failed to delete webhook: %w", err)
	}

	return nil
}

// GetWebhook gets a webhook by ID
func (s *Service) GetWebhook(ctx context.Context, webhookID uuid.UUID) (*Webhook, error) {
	var webhook Webhook
	err := s.db.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", webhookID).
		First(&webhook).Error

	if err != nil {
		return nil, err
	}

	return &webhook, nil
}

// ListWebhooks lists webhooks for a user
func (s *Service) ListWebhooks(ctx context.Context, userID uuid.UUID) ([]*Webhook, error) {
	var webhooks []*Webhook
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Find(&webhooks).Error

	if err != nil {
		return nil, err
	}

	return webhooks, nil
}

// TriggerWebhook triggers webhook for an event
func (s *Service) TriggerWebhook(ctx context.Context, eventType EventType, userID uuid.UUID, payload interface{}) error {
	// Get active webhooks for this event
	webhooks, err := s.getWebhooksForEvent(ctx, userID, eventType)
	if err != nil {
		return err
	}

	if len(webhooks) == 0 {
		return nil // No webhooks to trigger
	}

	// Create payload
	payloadData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Create deliveries for each webhook
	for _, webhook := range webhooks {
		delivery := &WebhookDelivery{
			ID:        uuid.New(),
			WebhookID: webhook.ID,
			EventType: eventType,
			EventID:   uuid.New().String(),
			Payload:   payloadData,
			Status:    StatusPending,
			Attempts:  0,
			CreatedAt: time.Now(),
		}

		// Save delivery
		if err := s.db.WithContext(ctx).Create(delivery).Error; err != nil {
			s.logger.Error("Failed to create webhook delivery",
				zap.Error(err),
				zap.String("webhook_id", webhook.ID.String()),
			)
			continue
		}

		// Queue for delivery
		select {
		case s.queue <- delivery:
		default:
			// Queue is full, process synchronously
			go s.deliverWebhook(webhook, delivery)
		}
	}

	return nil
}

// getWebhooksForEvent gets webhooks subscribed to an event
func (s *Service) getWebhooksForEvent(ctx context.Context, userID uuid.UUID, eventType EventType) ([]*Webhook, error) {
	var webhooks []*Webhook
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND active = ? AND deleted_at IS NULL", userID, true).
		Where("events @> ?", fmt.Sprintf(`["%s"]`, eventType)).
		Find(&webhooks).Error

	if err != nil {
		return nil, err
	}

	// Filter webhooks that include this event or "*" (all events)
	var filtered []*Webhook
	for _, webhook := range webhooks {
		for _, event := range webhook.Events {
			if event == string(eventType) || event == "*" {
				filtered = append(filtered, webhook)
				break
			}
		}
	}

	return filtered, nil
}

// processDeliveryQueue processes queued webhook deliveries
func (s *Service) processDeliveryQueue() {
	for delivery := range s.queue {
		// Get webhook
		var webhook Webhook
		err := s.db.Where("id = ?", delivery.WebhookID).First(&webhook).Error
		if err != nil {
			s.logger.Error("Failed to get webhook for delivery",
				zap.Error(err),
				zap.String("delivery_id", delivery.ID.String()),
			)
			continue
		}

		// Deliver webhook
		s.deliverWebhook(&webhook, delivery)
	}
}

// deliverWebhook delivers a webhook
func (s *Service) deliverWebhook(webhook *Webhook, delivery *WebhookDelivery) {
	delivery.Attempts++

	// Create request payload
	eventPayload := WebhookPayload{
		Event:     string(delivery.EventType),
		EventID:   delivery.EventID,
		Webhook:   webhook.Name,
		Timestamp: time.Now().Unix(),
		Data:      delivery.Payload,
	}

	payloadBytes, err := json.Marshal(eventPayload)
	if err != nil {
		s.handleDeliveryError(delivery, fmt.Errorf("failed to marshal payload: %w", err))
		return
	}

	// Create request
	req, err := http.NewRequest("POST", webhook.URL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		s.handleDeliveryError(delivery, fmt.Errorf("failed to create request: %w", err))
		return
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "DoneList-Webhook/1.0")
	req.Header.Set("X-Webhook-Event", string(delivery.EventType))
	req.Header.Set("X-Webhook-Event-ID", delivery.EventID)
	req.Header.Set("X-Webhook-Delivery-ID", delivery.ID.String())

	// Add custom headers
	for key, value := range webhook.Headers {
		req.Header.Set(key, value)
	}

	// Add HMAC signature
	if webhook.Secret != "" {
		signature := s.calculateSignature(payloadBytes, webhook.Secret)
		req.Header.Set("X-Webhook-Signature", signature)
	}

	// Send request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.handleDeliveryError(delivery, fmt.Errorf("request failed: %w", err))
		return
	}
	defer resp.Body.Close()

	// Read response
	responseBody, _ := io.ReadAll(resp.Body)
	responseStr := string(responseBody)
	delivery.StatusCode = &resp.StatusCode
	delivery.Response = &responseStr

	// Check status
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		// Success
		now := time.Now()
		delivery.Status = StatusDelivered
		delivery.DeliveredAt = &now

		// Update webhook stats
		s.db.Model(&Webhook{}).
			Where("id = ?", webhook.ID).
			Updates(map[string]interface{}{
				"last_triggered_at": now,
				"total_deliveries":  gorm.Expr("total_deliveries + 1"),
			})

		s.logger.Info("Webhook delivered successfully",
			zap.String("webhook_id", webhook.ID.String()),
			zap.String("delivery_id", delivery.ID.String()),
			zap.Int("status_code", resp.StatusCode),
		)
	} else {
		// Failed
		s.handleDeliveryError(delivery, fmt.Errorf("received status code %d", resp.StatusCode))
	}

	// Update delivery
	s.db.Save(delivery)
}

// handleDeliveryError handles webhook delivery errors
func (s *Service) handleDeliveryError(delivery *WebhookDelivery, err error) {
	errStr := err.Error()
	delivery.Error = &errStr

	// Determine if we should retry
	if delivery.Attempts < 3 {
		delivery.Status = StatusRetrying
		nextRetry := time.Now().Add(time.Duration(delivery.Attempts*delivery.Attempts) * time.Minute)
		delivery.NextRetryAt = &nextRetry

		s.logger.Warn("Webhook delivery failed, will retry",
			zap.String("delivery_id", delivery.ID.String()),
			zap.Int("attempts", delivery.Attempts),
			zap.Error(err),
		)
	} else {
		delivery.Status = StatusFailed

		// Update webhook failed deliveries count
		s.db.Model(&Webhook{}).
			Where("id = ?", delivery.WebhookID).
			Update("failed_deliveries", gorm.Expr("failed_deliveries + 1"))

		s.logger.Error("Webhook delivery failed permanently",
			zap.String("delivery_id", delivery.ID.String()),
			zap.Int("attempts", delivery.Attempts),
			zap.Error(err),
		)
	}

	s.db.Save(delivery)
}

// processRetries processes webhook retries
func (s *Service) processRetries() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		var deliveries []*WebhookDelivery
		s.db.Where("status = ? AND next_retry_at <= ?", StatusRetrying, time.Now()).
			Limit(100).
			Find(&deliveries)

		for _, delivery := range deliveries {
			// Get webhook
			var webhook Webhook
			if err := s.db.Where("id = ?", delivery.WebhookID).First(&webhook).Error; err != nil {
				continue
			}

			// Retry delivery
			go s.deliverWebhook(&webhook, delivery)
		}
	}
}

// calculateSignature calculates HMAC-SHA256 signature
func (s *Service) calculateSignature(payload []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}

// generateSecret generates a random webhook secret
func (s *Service) generateSecret() string {
	return uuid.New().String()
}

// WebhookPayload represents the payload sent to webhooks
type WebhookPayload struct {
	Event     string          `json:"event"`
	EventID   string          `json:"event_id"`
	Webhook   string          `json:"webhook"`
	Timestamp int64           `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

// GetDeliveries gets webhook deliveries
func (s *Service) GetDeliveries(ctx context.Context, webhookID uuid.UUID, limit int) ([]*WebhookDelivery, error) {
	var deliveries []*WebhookDelivery
	err := s.db.WithContext(ctx).
		Where("webhook_id = ?", webhookID).
		Order("created_at DESC").
		Limit(limit).
		Find(&deliveries).Error

	if err != nil {
		return nil, err
	}

	return deliveries, nil
}

// ResendDelivery resends a webhook delivery
func (s *Service) ResendDelivery(ctx context.Context, deliveryID uuid.UUID) error {
	var delivery WebhookDelivery
	if err := s.db.WithContext(ctx).Where("id = ?", deliveryID).First(&delivery).Error; err != nil {
		return err
	}

	var webhook Webhook
	if err := s.db.WithContext(ctx).Where("id = ?", delivery.WebhookID).First(&webhook).Error; err != nil {
		return err
	}

	// Reset delivery status
	delivery.Status = StatusPending
	delivery.Attempts = 0

	// Queue for delivery
	s.queue <- &delivery

	return nil
}

// TestWebhook sends a test webhook
func (s *Service) TestWebhook(ctx context.Context, webhookID uuid.UUID) error {
	webhook, err := s.GetWebhook(ctx, webhookID)
	if err != nil {
		return err
	}

	testPayload := map[string]interface{}{
		"test": true,
		"message": "This is a test webhook from DoneList",
		"timestamp": time.Now().Unix(),
	}

	return s.TriggerWebhook(ctx, "test.webhook", webhook.UserID, testPayload)
}