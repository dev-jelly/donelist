package notification

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"go.uber.org/zap"
	"google.golang.org/api/option"
)

// FCMProvider implements PushNotificationProvider for Firebase Cloud Messaging
type FCMProvider struct {
	client       *messaging.Client
	config       *PushProviderConfig
	logger       *zap.Logger
	metrics      *fcmMetrics
	rateLimiter  *rateLimiter
	isAvailable  atomic.Bool
	mu           sync.RWMutex
}

type fcmMetrics struct {
	totalSent      atomic.Int64
	totalFailed    atomic.Int64
	totalBatches   atomic.Int64
	lastError      atomic.Value
	lastErrorTime  atomic.Value
}

// NewFCMProvider creates a new FCM push notification provider
func NewFCMProvider(config *PushProviderConfig, logger *zap.Logger) (*FCMProvider, error) {
	if config.FCMCredentialsJSON == "" {
		return nil, fmt.Errorf("FCM credentials JSON is required")
	}

	// Initialize Firebase app
	opt := option.WithCredentialsJSON([]byte(config.FCMCredentialsJSON))
	app, err := firebase.NewApp(context.Background(), nil, opt)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Firebase app: %w", err)
	}

	// Get messaging client
	client, err := app.Messaging(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get messaging client: %w", err)
	}

	provider := &FCMProvider{
		client:      client,
		config:      config,
		logger:      logger,
		metrics:     &fcmMetrics{},
		rateLimiter: newRateLimiter(config.RateLimitPerSec),
	}

	provider.isAvailable.Store(true)

	// Start health check goroutine
	go provider.healthCheck()

	logger.Info("FCM provider initialized successfully")

	return provider, nil
}

// Send sends a single push notification via FCM
func (p *FCMProvider) Send(ctx context.Context, notification *PushNotification) (*PushResult, error) {
	if !p.isAvailable.Load() {
		return &PushResult{
			Success:   false,
			Error:     ErrProviderUnavailable,
			Retryable: true,
			SentAt:    time.Now(),
		}, ErrProviderUnavailable
	}

	// Validate notification
	if err := ValidatePushNotification(notification); err != nil {
		return &PushResult{
			Success: false,
			Error:   err,
			SentAt:  time.Now(),
		}, err
	}

	// Apply rate limiting
	if err := p.rateLimiter.Wait(ctx); err != nil {
		return &PushResult{
			Success:   false,
			Error:     ErrRateLimitExceeded,
			Retryable: true,
			SentAt:    time.Now(),
		}, ErrRateLimitExceeded
	}

	// Build FCM message
	message := p.buildFCMMessage(notification)

	// Send the message
	messageID, err := p.client.Send(ctx, message)
	if err != nil {
		p.metrics.totalFailed.Add(1)
		p.metrics.lastError.Store(err.Error())
		p.metrics.lastErrorTime.Store(time.Now())

		result := p.handleFCMError(err)
		return result, err
	}

	p.metrics.totalSent.Add(1)

	return &PushResult{
		MessageID: messageID,
		Success:   true,
		SentAt:    time.Now(),
	}, nil
}

// SendBatch sends multiple notifications in a batch
func (p *FCMProvider) SendBatch(ctx context.Context, notifications []*PushNotification) ([]*PushResult, error) {
	if !p.isAvailable.Load() {
		results := make([]*PushResult, len(notifications))
		for i := range results {
			results[i] = &PushResult{
				Success:   false,
				Error:     ErrProviderUnavailable,
				Retryable: true,
				SentAt:    time.Now(),
			}
		}
		return results, ErrProviderUnavailable
	}

	results := make([]*PushResult, len(notifications))

	// FCM batch limit is 500 messages
	batchSize := p.config.MaxBatchSize
	if batchSize > 500 {
		batchSize = 500
	}

	for i := 0; i < len(notifications); i += batchSize {
		end := i + batchSize
		if end > len(notifications) {
			end = len(notifications)
		}

		batch := notifications[i:end]
		messages := make([]*messaging.Message, len(batch))

		// Build messages for batch
		for j, notification := range batch {
			if err := ValidatePushNotification(notification); err != nil {
				results[i+j] = &PushResult{
					Success: false,
					Error:   err,
					SentAt:  time.Now(),
				}
				continue
			}
			messages[j] = p.buildFCMMessage(notification)
		}

		// Apply rate limiting for batch
		if err := p.rateLimiter.WaitN(ctx, len(messages)); err != nil {
			for j := range batch {
				results[i+j] = &PushResult{
					Success:   false,
					Error:     ErrRateLimitExceeded,
					Retryable: true,
					SentAt:    time.Now(),
				}
			}
			continue
		}

		// Send batch
		batchResponse, err := p.client.SendAll(ctx, messages)
		if err != nil {
			p.metrics.totalFailed.Add(int64(len(batch)))
			p.metrics.lastError.Store(err.Error())
			p.metrics.lastErrorTime.Store(time.Now())

			// Set error for all messages in batch
			for j := range batch {
				results[i+j] = p.handleFCMError(err)
			}
			continue
		}

		p.metrics.totalBatches.Add(1)

		// Process individual responses
		for j, response := range batchResponse.Responses {
			if response.Success {
				p.metrics.totalSent.Add(1)
				results[i+j] = &PushResult{
					MessageID: response.MessageID,
					Success:   true,
					SentAt:    time.Now(),
				}
			} else {
				p.metrics.totalFailed.Add(1)
				results[i+j] = p.handleFCMError(response.Error)
			}
		}
	}

	return results, nil
}

// ValidateToken validates an FCM device token
func (p *FCMProvider) ValidateToken(token string) error {
	if token == "" {
		return ErrInvalidToken
	}

	// FCM tokens are typically 152+ characters
	if len(token) < 100 {
		return ErrInvalidToken
	}

	// Basic format check - FCM tokens are base64-like strings
	for _, r := range token {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == ':' || r == '_' || r == '-') {
			return ErrInvalidToken
		}
	}

	return nil
}

// GetProviderType returns the provider type
func (p *FCMProvider) GetProviderType() PushProvider {
	return PushProviderFCM
}

// IsAvailable checks if the provider is available
func (p *FCMProvider) IsAvailable(ctx context.Context) bool {
	return p.isAvailable.Load()
}

// GetMetrics returns provider metrics
func (p *FCMProvider) GetMetrics() map[string]interface{} {
	lastError := ""
	if v := p.metrics.lastError.Load(); v != nil {
		lastError = v.(string)
	}

	lastErrorTime := time.Time{}
	if v := p.metrics.lastErrorTime.Load(); v != nil {
		lastErrorTime = v.(time.Time)
	}

	return map[string]interface{}{
		"provider":        "fcm",
		"total_sent":      p.metrics.totalSent.Load(),
		"total_failed":    p.metrics.totalFailed.Load(),
		"total_batches":   p.metrics.totalBatches.Load(),
		"is_available":    p.isAvailable.Load(),
		"last_error":      lastError,
		"last_error_time": lastErrorTime,
		"success_rate":    p.calculateSuccessRate(),
	}
}

// Close closes the provider
func (p *FCMProvider) Close() error {
	p.isAvailable.Store(false)
	p.logger.Info("FCM provider closed")
	return nil
}

// buildFCMMessage builds an FCM message from a push notification
func (p *FCMProvider) buildFCMMessage(notification *PushNotification) *messaging.Message {
	message := &messaging.Message{
		Token: notification.DeviceToken,
	}

	// Build notification payload
	if !notification.Silent {
		messageNotification := &messaging.Notification{
			Title: notification.Title,
			Body:  notification.Body,
		}

		message.Notification = messageNotification
	}

	// Build Android-specific configuration
	androidConfig := &messaging.AndroidConfig{
		Priority: "normal",
	}

	if notification.HighPriority {
		androidConfig.Priority = "high"
	}

	if notification.TTL > 0 {
		ttl := time.Duration(notification.TTL) * time.Second
		androidConfig.TTL = &ttl
	}

	if notification.CollapseKey != "" {
		androidConfig.CollapseKey = notification.CollapseKey
	}

	// Android notification settings
	if !notification.Silent {
		androidNotification := &messaging.AndroidNotification{}

		if notification.Sound != "" {
			androidNotification.Sound = notification.Sound
		}

		androidConfig.Notification = androidNotification
	}

	message.Android = androidConfig

	// Build iOS-specific configuration (APNs via FCM)
	apnsConfig := &messaging.APNSConfig{
		Headers: make(map[string]string),
	}

	if notification.HighPriority {
		apnsConfig.Headers["apns-priority"] = "10"
	} else {
		apnsConfig.Headers["apns-priority"] = "5"
	}

	if notification.TTL > 0 {
		expiration := time.Now().Add(time.Duration(notification.TTL) * time.Second)
		apnsConfig.Headers["apns-expiration"] = fmt.Sprintf("%d", expiration.Unix())
	}

	if notification.CollapseKey != "" {
		apnsConfig.Headers["apns-collapse-id"] = notification.CollapseKey
	}

	if notification.ThreadID != "" {
		apnsConfig.Headers["thread-id"] = notification.ThreadID
	}

	// Build APNs payload
	aps := &messaging.Aps{
		Alert: &messaging.ApsAlert{
			Title: notification.Title,
			Body:  notification.Body,
		},
	}

	if notification.Badge != nil {
		aps.Badge = notification.Badge
	}

	if notification.Sound != "" {
		aps.Sound = notification.Sound
	}

	if notification.Category != "" {
		aps.Category = notification.Category
	}

	if notification.MutableContent {
		aps.MutableContent = true
	}

	if notification.ContentAvailable {
		aps.ContentAvailable = true
	}

	if notification.Silent {
		aps.ContentAvailable = true
		aps.Alert = nil
	}

	apnsConfig.Payload = &messaging.APNSPayload{
		Aps: aps,
	}

	message.APNS = apnsConfig

	// Add custom data
	if len(notification.Data) > 0 {
		message.Data = notification.Data
	}

	return message
}

// handleFCMError handles FCM-specific errors
func (p *FCMProvider) handleFCMError(err error) *PushResult {
	if err == nil {
		return &PushResult{
			Success: true,
			SentAt:  time.Now(),
		}
	}

	result := &PushResult{
		Success: false,
		Error:   err,
		SentAt:  time.Now(),
	}

	// Parse FCM error to determine if it's retryable
	errorStr := err.Error()

	switch {
	case contains(errorStr, "registration-token-not-registered"):
		result.Error = ErrTokenExpired
		result.ShouldRemoveToken = true
		result.Retryable = false

	case contains(errorStr, "invalid-registration-token"):
		result.Error = ErrInvalidToken
		result.ShouldRemoveToken = true
		result.Retryable = false

	case contains(errorStr, "message-rate-exceeded"):
		result.Error = ErrRateLimitExceeded
		result.Retryable = true
		retryAfter := time.Minute
		result.RetryAfter = &retryAfter

	case contains(errorStr, "service-unavailable"):
		result.Error = ErrProviderUnavailable
		result.Retryable = true
		retryAfter := time.Minute * 5
		result.RetryAfter = &retryAfter

	case contains(errorStr, "internal-error"):
		result.Error = ErrProviderUnavailable
		result.Retryable = true

	case contains(errorStr, "quota-exceeded"):
		result.Error = ErrQuotaExceeded
		result.Retryable = false

	case contains(errorStr, "authentication-error"):
		result.Error = ErrAuthenticationFailed
		result.Retryable = false
		// This is a configuration error, mark provider as unavailable
		p.isAvailable.Store(false)

	default:
		// Unknown error, consider retryable
		result.Retryable = true
	}

	return result
}

// healthCheck performs periodic health checks
func (p *FCMProvider) healthCheck() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		// Simple health check - could be enhanced to ping FCM service
		successRate := p.calculateSuccessRate()
		if successRate < 0.5 && p.metrics.totalSent.Load() > 100 {
			p.logger.Warn("FCM provider success rate is low",
				zap.Float64("success_rate", successRate))
		}
	}
}

// calculateSuccessRate calculates the success rate of sent notifications
func (p *FCMProvider) calculateSuccessRate() float64 {
	sent := p.metrics.totalSent.Load()
	failed := p.metrics.totalFailed.Load()
	total := sent + failed

	if total == 0 {
		return 1.0
	}

	return float64(sent) / float64(total)
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr ||
		len(s) >= len(substr) && s[len(s)-len(substr):] == substr ||
		len(substr) < len(s) && containsMiddle(s, substr)
}

func containsMiddle(s, substr string) bool {
	for i := 1; i < len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// rateLimiter implements a simple token bucket rate limiter
type rateLimiter struct {
	tokens    atomic.Int64
	maxTokens int64
	refillRate int64
	lastRefill atomic.Value // time.Time
	mu        sync.Mutex
}

func newRateLimiter(ratePerSec int) *rateLimiter {
	rl := &rateLimiter{
		maxTokens:  int64(ratePerSec),
		refillRate: int64(ratePerSec),
	}
	rl.tokens.Store(int64(ratePerSec))
	rl.lastRefill.Store(time.Now())

	// Start refill goroutine
	go rl.refillLoop()

	return rl
}

func (rl *rateLimiter) Wait(ctx context.Context) error {
	return rl.WaitN(ctx, 1)
}

func (rl *rateLimiter) WaitN(ctx context.Context, n int) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if rl.tryAcquire(int64(n)) {
				return nil
			}
			time.Sleep(time.Millisecond * 10)
		}
	}
}

func (rl *rateLimiter) tryAcquire(n int64) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	current := rl.tokens.Load()
	if current >= n {
		rl.tokens.Add(-n)
		return true
	}
	return false
}

func (rl *rateLimiter) refillLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		current := rl.tokens.Load()
		if current < rl.maxTokens {
			newTokens := current + rl.refillRate
			if newTokens > rl.maxTokens {
				newTokens = rl.maxTokens
			}
			rl.tokens.Store(newTokens)
		}
		rl.mu.Unlock()
	}
}