package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"runtime"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"go.uber.org/zap"
)

const (
	// Task types
	TypeWebhookDelivery = "webhook:delivery"
	TypeWebhookCleanup  = "webhook:cleanup"

	// Retry configuration
	MaxRetries           = 5
	InitialRetryDelay    = 1 * time.Minute
	MaxRetryDelay        = 1 * time.Hour
	DeliveryTimeout      = 30 * time.Second
	CleanupRetentionDays = 30
)

// Service handles webhook operations with Asynq job queue
type Service struct {
	repo       *Repository
	httpClient *http.Client
	logger     *zap.Logger
	asynqClient *asynq.Client
	asynqServer *asynq.Server
}

// NewService creates a new webhook service with Asynq
// Concurrency is dynamically calculated based on CPU cores for optimal performance
func NewService(repo *Repository, redisAddr string, logger *zap.Logger) *Service {
	redisOpt := asynq.RedisClientOpt{Addr: redisAddr}

	// Calculate concurrency based on CPU cores
	// Webhook delivery is I/O bound, so we can have more workers than CPU cores
	numCPU := runtime.NumCPU()
	concurrency := numCPU * 10 // 10 workers per core for I/O-bound tasks

	// Apply min/max limits
	if concurrency < 20 {
		concurrency = 20 // Minimum 20 workers
	}
	if concurrency > 200 {
		concurrency = 200 // Maximum 200 workers to prevent resource exhaustion
	}

	logger.Info("Initializing webhook service",
		zap.Int("cpu_cores", numCPU),
		zap.Int("worker_concurrency", concurrency),
	)

	return &Service{
		repo: repo,
		// HTTP client with connection pooling for better performance
		httpClient: &http.Client{
			Timeout: DeliveryTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,              // Total idle connections
				MaxIdleConnsPerHost: 20,               // Per-host idle connections
				IdleConnTimeout:     90 * time.Second, // Keep connections alive
				TLSHandshakeTimeout: 10 * time.Second,
				DisableKeepAlives:   false,            // Enable keep-alive
				ForceAttemptHTTP2:   true,             // Use HTTP/2 when possible
			},
		},
		logger:      logger,
		asynqClient: asynq.NewClient(redisOpt),
		asynqServer: asynq.NewServer(
			redisOpt,
			asynq.Config{
				Concurrency: concurrency, // Dynamic concurrency based on CPU
				Queues: map[string]int{
					"critical": 7, // 70% of workers
					"default":  2, // 20% of workers
					"low":      1, // 10% of workers
				},
				StrictPriority: true, // Ensure critical tasks get priority
				RetryDelayFunc: exponentialBackoff,
				ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
					logger.Error("Task failed",
						zap.String("type", task.Type()),
						zap.Error(err),
					)
				}),
			},
		),
	}
}

// exponentialBackoff calculates retry delay with exponential backoff
func exponentialBackoff(n int, e error, t *asynq.Task) time.Duration {
	// exponential: 1m, 2m, 4m, 8m, 16m, capped at 1h
	delay := time.Duration(math.Pow(2, float64(n))) * InitialRetryDelay
	if delay > MaxRetryDelay {
		delay = MaxRetryDelay
	}
	return delay
}

// Start starts the Asynq server and background workers
func (s *Service) Start() error {
	mux := asynq.NewServeMux()
	mux.HandleFunc(TypeWebhookDelivery, s.handleDeliveryTask)
	mux.HandleFunc(TypeWebhookCleanup, s.handleCleanupTask)

	go func() {
		if err := s.asynqServer.Start(mux); err != nil {
			s.logger.Fatal("Failed to start webhook worker", zap.Error(err))
		}
	}()

	s.logger.Info("Webhook service started")
	return nil
}

// Stop gracefully stops the webhook service
func (s *Service) Stop() error {
	s.asynqServer.Shutdown()
	s.asynqClient.Close()
	s.logger.Info("Webhook service stopped")
	return nil
}

// CreateWebhook creates a new webhook
func (s *Service) CreateWebhook(ctx context.Context, req *CreateWebhookRequest, userID uuid.UUID) (*Webhook, error) {
	webhook := &Webhook{
		ID:      uuid.New(),
		UserID:  userID,
		Name:    req.Name,
		URL:     req.URL,
		Events:  req.Events,
		Active:  req.Active,
		Headers: req.Headers,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Generate secret if not provided
	if req.Secret != "" {
		webhook.Secret = req.Secret
	} else {
		webhook.Secret = s.generateSecret()
	}

	if err := s.repo.CreateWebhook(ctx, webhook); err != nil {
		return nil, fmt.Errorf("failed to create webhook: %w", err)
	}

	s.logger.Info("Webhook created",
		zap.String("id", webhook.ID.String()),
		zap.String("url", webhook.URL),
		zap.Strings("events", webhook.Events),
	)

	return webhook, nil
}

// UpdateWebhook updates an existing webhook
func (s *Service) UpdateWebhook(ctx context.Context, webhookID, userID uuid.UUID, req *UpdateWebhookRequest) (*Webhook, error) {
	webhook, err := s.repo.GetWebhookByIDAndUser(ctx, webhookID, userID)
	if err != nil {
		return nil, fmt.Errorf("webhook not found: %w", err)
	}

	if req.Name != nil {
		webhook.Name = *req.Name
	}
	if req.URL != nil {
		webhook.URL = *req.URL
	}
	if req.Events != nil {
		webhook.Events = req.Events
	}
	if req.Active != nil {
		webhook.Active = *req.Active
	}
	if req.Headers != nil {
		webhook.Headers = req.Headers
	}

	webhook.UpdatedAt = time.Now()

	if err := s.repo.UpdateWebhook(ctx, webhook); err != nil {
		return nil, fmt.Errorf("failed to update webhook: %w", err)
	}

	s.logger.Info("Webhook updated", zap.String("id", webhook.ID.String()))
	return webhook, nil
}

// DeleteWebhook soft deletes a webhook
func (s *Service) DeleteWebhook(ctx context.Context, webhookID, userID uuid.UUID) error {
	// Verify ownership
	_, err := s.repo.GetWebhookByIDAndUser(ctx, webhookID, userID)
	if err != nil {
		return fmt.Errorf("webhook not found: %w", err)
	}

	if err := s.repo.DeleteWebhook(ctx, webhookID); err != nil {
		return fmt.Errorf("failed to delete webhook: %w", err)
	}

	s.logger.Info("Webhook deleted", zap.String("id", webhookID.String()))
	return nil
}

// GetWebhook gets a webhook by ID
func (s *Service) GetWebhook(ctx context.Context, webhookID, userID uuid.UUID) (*Webhook, error) {
	return s.repo.GetWebhookByIDAndUser(ctx, webhookID, userID)
}

// ListWebhooks lists webhooks for a user
func (s *Service) ListWebhooks(ctx context.Context, userID uuid.UUID) ([]*Webhook, error) {
	return s.repo.ListWebhooksByUser(ctx, userID)
}

// TriggerEvent triggers webhooks for an event
func (s *Service) TriggerEvent(ctx context.Context, eventType EventType, userID uuid.UUID, payload interface{}) error {
	// Get active webhooks for this event
	webhooks, err := s.repo.GetActiveWebhooksForEvent(ctx, userID, eventType)
	if err != nil {
		return fmt.Errorf("failed to get webhooks: %w", err)
	}

	if len(webhooks) == 0 {
		return nil // No webhooks to trigger
	}

	// Marshal payload
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
		if err := s.repo.CreateDelivery(ctx, delivery); err != nil {
			s.logger.Error("Failed to create webhook delivery",
				zap.Error(err),
				zap.String("webhook_id", webhook.ID.String()),
			)
			continue
		}

		// Enqueue delivery task
		if err := s.enqueueDelivery(delivery.ID); err != nil {
			s.logger.Error("Failed to enqueue webhook delivery",
				zap.Error(err),
				zap.String("delivery_id", delivery.ID.String()),
			)
		}
	}

	return nil
}

// enqueueDelivery enqueues a webhook delivery task
func (s *Service) enqueueDelivery(deliveryID uuid.UUID) error {
	payload, err := json.Marshal(map[string]string{
		"delivery_id": deliveryID.String(),
	})
	if err != nil {
		return err
	}

	task := asynq.NewTask(TypeWebhookDelivery, payload,
		asynq.MaxRetry(MaxRetries),
		asynq.Queue("default"),
		asynq.Timeout(DeliveryTimeout),
	)

	_, err = s.asynqClient.Enqueue(task)
	return err
}

// handleDeliveryTask handles webhook delivery task
func (s *Service) handleDeliveryTask(ctx context.Context, task *asynq.Task) error {
	var payload map[string]string
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}

	deliveryID, err := uuid.Parse(payload["delivery_id"])
	if err != nil {
		return fmt.Errorf("invalid delivery_id: %w", err)
	}

	// Get delivery
	delivery, err := s.repo.GetDeliveryByID(ctx, deliveryID)
	if err != nil {
		return fmt.Errorf("delivery not found: %w", err)
	}

	// Get webhook
	webhook, err := s.repo.GetWebhookByID(ctx, delivery.WebhookID)
	if err != nil {
		return fmt.Errorf("webhook not found: %w", err)
	}

	// Skip if webhook is inactive
	if !webhook.Active {
		delivery.Status = StatusSkipped
		s.repo.UpdateDelivery(ctx, delivery)
		return nil
	}

	// Deliver webhook
	startTime := time.Now()
	err = s.deliverWebhook(ctx, webhook, delivery)
	duration := time.Since(startTime)

	// Log event
	s.logDeliveryAttempt(ctx, delivery, int(duration.Milliseconds()))

	return err
}

// deliverWebhook delivers a webhook
func (s *Service) deliverWebhook(ctx context.Context, webhook *Webhook, delivery *WebhookDelivery) error {
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
		return s.handleDeliveryError(ctx, delivery, webhook, fmt.Errorf("failed to marshal payload: %w", err))
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", webhook.URL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return s.handleDeliveryError(ctx, delivery, webhook, fmt.Errorf("failed to create request: %w", err))
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "DoneList-Webhook/2.0")
	req.Header.Set("X-Webhook-Event", string(delivery.EventType))
	req.Header.Set("X-Webhook-Event-ID", delivery.EventID)
	req.Header.Set("X-Webhook-Delivery-ID", delivery.ID.String())
	req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

	// Add custom headers
	for key, value := range webhook.Headers {
		req.Header.Set(key, value)
	}

	// Add HMAC signature
	signature := s.calculateSignature(payloadBytes, webhook.Secret)
	req.Header.Set("X-Webhook-Signature", signature)
	req.Header.Set("X-Webhook-Signature-256", signature) // Alternative header name

	// Send request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return s.handleDeliveryError(ctx, delivery, webhook, fmt.Errorf("request failed: %w", err))
	}
	defer resp.Body.Close()

	// Read response
	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024)) // Limit to 1MB
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
		s.repo.UpdateWebhookStats(ctx, webhook.ID, true)

		s.logger.Info("Webhook delivered successfully",
			zap.String("webhook_id", webhook.ID.String()),
			zap.String("delivery_id", delivery.ID.String()),
			zap.Int("status_code", resp.StatusCode),
		)
	} else {
		return s.handleDeliveryError(ctx, delivery, webhook, fmt.Errorf("received status code %d", resp.StatusCode))
	}

	// Update delivery
	return s.repo.UpdateDelivery(ctx, delivery)
}

// handleDeliveryError handles webhook delivery errors
func (s *Service) handleDeliveryError(ctx context.Context, delivery *WebhookDelivery, webhook *Webhook, err error) error {
	errStr := err.Error()
	delivery.Error = &errStr

	// Determine if we should retry
	if delivery.Attempts < MaxRetries {
		delivery.Status = StatusRetrying
		nextRetry := time.Now().Add(exponentialBackoff(delivery.Attempts, err, nil))
		delivery.NextRetryAt = &nextRetry

		s.logger.Warn("Webhook delivery failed, will retry",
			zap.String("delivery_id", delivery.ID.String()),
			zap.Int("attempts", delivery.Attempts),
			zap.Time("next_retry", nextRetry),
			zap.Error(err),
		)

		s.repo.UpdateDelivery(ctx, delivery)
		return err // Return error to trigger Asynq retry
	}

	// Max retries exceeded, move to DLQ
	delivery.Status = StatusFailed
	s.repo.UpdateDelivery(ctx, delivery)

	// Update webhook stats
	s.repo.UpdateWebhookStats(ctx, webhook.ID, false)

	// Move to dead letter queue
	if dlqErr := s.repo.MoveToDLQ(ctx, delivery, webhook); dlqErr != nil {
		s.logger.Error("Failed to move delivery to DLQ",
			zap.String("delivery_id", delivery.ID.String()),
			zap.Error(dlqErr),
		)
	}

	s.logger.Error("Webhook delivery failed permanently",
		zap.String("delivery_id", delivery.ID.String()),
		zap.Int("attempts", delivery.Attempts),
		zap.Error(err),
	)

	return nil // Don't return error to prevent further retries
}

// logDeliveryAttempt logs a delivery attempt
func (s *Service) logDeliveryAttempt(ctx context.Context, delivery *WebhookDelivery, durationMs int) {
	// This is automatically handled by the database trigger
	// But we can add additional metrics here if needed
}

// GetDeliveries gets webhook deliveries
func (s *Service) GetDeliveries(ctx context.Context, webhookID, userID uuid.UUID, limit int) ([]*WebhookDelivery, error) {
	// Verify webhook ownership
	_, err := s.repo.GetWebhookByIDAndUser(ctx, webhookID, userID)
	if err != nil {
		return nil, fmt.Errorf("webhook not found: %w", err)
	}

	return s.repo.ListDeliveriesByWebhook(ctx, webhookID, limit)
}

// GetDLQEntries gets dead letter queue entries
func (s *Service) GetDLQEntries(ctx context.Context, webhookID, userID uuid.UUID, limit int) ([]*WebhookDLQ, error) {
	// Verify webhook ownership if webhookID is provided
	if webhookID != uuid.Nil {
		_, err := s.repo.GetWebhookByIDAndUser(ctx, webhookID, userID)
		if err != nil {
			return nil, fmt.Errorf("webhook not found: %w", err)
		}
	}

	return s.repo.GetDLQEntries(ctx, webhookID, limit)
}

// ResendDelivery resends a failed delivery
func (s *Service) ResendDelivery(ctx context.Context, deliveryID, userID uuid.UUID) error {
	delivery, err := s.repo.GetDeliveryByID(ctx, deliveryID)
	if err != nil {
		return fmt.Errorf("delivery not found: %w", err)
	}

	// Verify webhook ownership
	_, err = s.repo.GetWebhookByIDAndUser(ctx, delivery.WebhookID, userID)
	if err != nil {
		return fmt.Errorf("webhook not found: %w", err)
	}

	// Reset delivery status
	delivery.Status = StatusPending
	delivery.Attempts = 0
	delivery.Error = nil
	delivery.NextRetryAt = nil

	if err := s.repo.UpdateDelivery(ctx, delivery); err != nil {
		return fmt.Errorf("failed to update delivery: %w", err)
	}

	// Enqueue delivery
	return s.enqueueDelivery(delivery.ID)
}

// TestWebhook sends a test webhook
func (s *Service) TestWebhook(ctx context.Context, webhookID, userID uuid.UUID) error {
	webhook, err := s.GetWebhook(ctx, webhookID, userID)
	if err != nil {
		return err
	}

	testPayload := map[string]interface{}{
		"test":      true,
		"message":   "This is a test webhook from DoneList",
		"timestamp": time.Now().Unix(),
	}

	return s.TriggerEvent(ctx, "test.webhook", webhook.UserID, testPayload)
}

// GetWebhookStats gets aggregated statistics for a webhook
func (s *Service) GetWebhookStats(ctx context.Context, webhookID, userID uuid.UUID, since time.Time) (*WebhookStats, error) {
	// Verify webhook ownership
	_, err := s.repo.GetWebhookByIDAndUser(ctx, webhookID, userID)
	if err != nil {
		return nil, fmt.Errorf("webhook not found: %w", err)
	}

	return s.repo.GetWebhookStats(ctx, webhookID, since)
}

// handleCleanupTask handles periodic cleanup of old records
func (s *Service) handleCleanupTask(ctx context.Context, task *asynq.Task) error {
	cutoff := time.Now().AddDate(0, 0, -CleanupRetentionDays)

	// Cleanup old deliveries
	deliveriesDeleted, err := s.repo.CleanupOldDeliveries(ctx, cutoff)
	if err != nil {
		s.logger.Error("Failed to cleanup old deliveries", zap.Error(err))
	} else {
		s.logger.Info("Cleaned up old deliveries", zap.Int64("count", deliveriesDeleted))
	}

	// Cleanup old event logs
	logsDeleted, err := s.repo.CleanupOldEventLogs(ctx, cutoff)
	if err != nil {
		s.logger.Error("Failed to cleanup old event logs", zap.Error(err))
	} else {
		s.logger.Info("Cleaned up old event logs", zap.Int64("count", logsDeleted))
	}

	return nil
}

// ScheduleCleanup schedules periodic cleanup
func (s *Service) ScheduleCleanup() error {
	task := asynq.NewTask(TypeWebhookCleanup, nil,
		asynq.Queue("low"),
	)

	// Schedule daily cleanup at 3 AM
	_, err := s.asynqClient.Enqueue(task,
		asynq.ProcessIn(24*time.Hour),
	)
	return err
}

// calculateSignature calculates HMAC-SHA256 signature
func (s *Service) calculateSignature(payload []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}

// VerifySignature verifies HMAC signature
func (s *Service) VerifySignature(payload []byte, signature, secret string) bool {
	expectedSignature := s.calculateSignature(payload, secret)
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

// generateSecret generates a random webhook secret
func (s *Service) generateSecret() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}
