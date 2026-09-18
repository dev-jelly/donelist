package notification

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// MockProvider implements PushNotificationProvider for testing
type MockProvider struct {
	// Configuration
	config *MockProviderConfig

	// State tracking
	sentNotifications    []*PushNotification
	sentResults          []*PushResult
	mu                   sync.RWMutex
	callCount            atomic.Int64
	shouldFail           atomic.Bool
	shouldBeUnavailable  atomic.Bool
	nextError            atomic.Value // stores error to return
	simulateDelay        time.Duration

	// Metrics
	metrics              *mockMetrics

	// Behavior configuration
	failureRate          float64
	tokenExpiryPattern   string // Pattern to match for expired tokens
	invalidTokenPattern  string // Pattern to match for invalid tokens
	rateLimitAfter       int    // Start rate limiting after N requests
	customResponseFunc   func(*PushNotification) *PushResult
}

// MockProviderConfig holds configuration for the mock provider
type MockProviderConfig struct {
	// Default behavior
	AlwaysSucceed        bool
	AlwaysFail           bool
	FailureRate          float64 // Probability of failure (0.0 to 1.0)

	// Specific behaviors
	SimulateDelay        time.Duration
	SimulateRateLimit    bool
	RateLimitAfter       int

	// Token validation patterns
	InvalidTokenPattern  string // Tokens matching this pattern are invalid
	ExpiredTokenPattern  string // Tokens matching this pattern are expired

	// Custom response function
	CustomResponseFunc   func(*PushNotification) *PushResult
}

type mockMetrics struct {
	totalSent            atomic.Int64
	totalFailed          atomic.Int64
	totalBatches         atomic.Int64
	lastNotification     atomic.Value
	validationErrors     atomic.Int64
	rateLimitHits        atomic.Int64
}

// NewMockProvider creates a new mock push notification provider
func NewMockProvider(config *MockProviderConfig) *MockProvider {
	if config == nil {
		config = &MockProviderConfig{
			AlwaysSucceed: true,
		}
	}

	provider := &MockProvider{
		config:               config,
		sentNotifications:    make([]*PushNotification, 0),
		sentResults:          make([]*PushResult, 0),
		metrics:              &mockMetrics{},
		simulateDelay:        config.SimulateDelay,
		failureRate:          config.FailureRate,
		invalidTokenPattern:  config.InvalidTokenPattern,
		tokenExpiryPattern:   config.ExpiredTokenPattern,
		rateLimitAfter:       config.RateLimitAfter,
		customResponseFunc:   config.CustomResponseFunc,
	}

	if config.AlwaysFail {
		provider.shouldFail.Store(true)
		provider.failureRate = 1.0
	} else if config.AlwaysSucceed {
		provider.failureRate = 0.0
	}

	return provider
}

// Send sends a single push notification (mock implementation)
func (p *MockProvider) Send(ctx context.Context, notification *PushNotification) (*PushResult, error) {
	// Track the call
	p.callCount.Add(1)
	currentCount := int(p.callCount.Load())

	// Simulate delay if configured
	if p.simulateDelay > 0 {
		select {
		case <-time.After(p.simulateDelay):
		case <-ctx.Done():
			return &PushResult{
				Success: false,
				Error:   ctx.Err(),
				SentAt:  time.Now(),
			}, ctx.Err()
		}
	}

	// Check if provider is unavailable
	if p.shouldBeUnavailable.Load() {
		result := &PushResult{
			Success:   false,
			Error:     ErrProviderUnavailable,
			Retryable: true,
			SentAt:    time.Now(),
		}
		p.recordResult(notification, result)
		return result, ErrProviderUnavailable
	}

	// Validate notification
	if err := ValidatePushNotification(notification); err != nil {
		p.metrics.validationErrors.Add(1)
		result := &PushResult{
			Success: false,
			Error:   err,
			SentAt:  time.Now(),
		}
		p.recordResult(notification, result)
		return result, err
	}

	// Check for rate limiting
	if p.config.SimulateRateLimit && p.rateLimitAfter > 0 && currentCount > p.rateLimitAfter {
		p.metrics.rateLimitHits.Add(1)
		retryAfter := time.Second * 30
		result := &PushResult{
			Success:    false,
			Error:      ErrRateLimitExceeded,
			Retryable:  true,
			RetryAfter: &retryAfter,
			SentAt:     time.Now(),
		}
		p.recordResult(notification, result)
		return result, ErrRateLimitExceeded
	}

	// Check token patterns
	if p.invalidTokenPattern != "" && contains(notification.DeviceToken, p.invalidTokenPattern) {
		result := &PushResult{
			Success:           false,
			Error:             ErrInvalidToken,
			ShouldRemoveToken: true,
			SentAt:            time.Now(),
		}
		p.recordResult(notification, result)
		p.metrics.totalFailed.Add(1)
		return result, ErrInvalidToken
	}

	if p.tokenExpiryPattern != "" && contains(notification.DeviceToken, p.tokenExpiryPattern) {
		result := &PushResult{
			Success:           false,
			Error:             ErrTokenExpired,
			ShouldRemoveToken: true,
			SentAt:            time.Now(),
		}
		p.recordResult(notification, result)
		p.metrics.totalFailed.Add(1)
		return result, ErrTokenExpired
	}

	// Check for custom response function
	if p.customResponseFunc != nil {
		result := p.customResponseFunc(notification)
		p.recordResult(notification, result)
		if result.Success {
			p.metrics.totalSent.Add(1)
		} else {
			p.metrics.totalFailed.Add(1)
		}
		return result, result.Error
	}

	// Check if should fail based on configuration
	if p.shouldFail.Load() {
		if err := p.nextError.Load(); err != nil {
			result := &PushResult{
				Success:   false,
				Error:     err.(error),
				Retryable: IsRetryableError(err.(error)),
				SentAt:    time.Now(),
			}
			p.recordResult(notification, result)
			p.metrics.totalFailed.Add(1)
			return result, err.(error)
		}

		result := &PushResult{
			Success:   false,
			Error:     fmt.Errorf("mock provider configured to fail"),
			Retryable: true,
			SentAt:    time.Now(),
		}
		p.recordResult(notification, result)
		p.metrics.totalFailed.Add(1)
		return result, result.Error
	}

	// Simulate random failures based on failure rate
	if p.failureRate > 0 && p.shouldSimulateFailure() {
		result := &PushResult{
			Success:   false,
			Error:     fmt.Errorf("simulated random failure"),
			Retryable: true,
			SentAt:    time.Now(),
		}
		p.recordResult(notification, result)
		p.metrics.totalFailed.Add(1)
		return result, result.Error
	}

	// Success case
	result := &PushResult{
		MessageID: fmt.Sprintf("mock_%d_%d", time.Now().Unix(), p.callCount.Load()),
		Success:   true,
		SentAt:    time.Now(),
	}

	p.recordResult(notification, result)
	p.metrics.totalSent.Add(1)
	p.metrics.lastNotification.Store(notification)

	return result, nil
}

// SendBatch sends multiple notifications in a batch
func (p *MockProvider) SendBatch(ctx context.Context, notifications []*PushNotification) ([]*PushResult, error) {
	p.metrics.totalBatches.Add(1)

	results := make([]*PushResult, len(notifications))
	for i, notification := range notifications {
		result, _ := p.Send(ctx, notification)
		results[i] = result
	}

	return results, nil
}

// ValidateToken validates a device token
func (p *MockProvider) ValidateToken(token string) error {
	if token == "" {
		return ErrInvalidToken
	}

	if p.invalidTokenPattern != "" && contains(token, p.invalidTokenPattern) {
		return ErrInvalidToken
	}

	if p.tokenExpiryPattern != "" && contains(token, p.tokenExpiryPattern) {
		return ErrTokenExpired
	}

	return nil
}

// GetProviderType returns the provider type
func (p *MockProvider) GetProviderType() PushProvider {
	return PushProviderMock
}

// IsAvailable checks if the provider is available
func (p *MockProvider) IsAvailable(ctx context.Context) bool {
	return !p.shouldBeUnavailable.Load()
}

// GetMetrics returns provider metrics
func (p *MockProvider) GetMetrics() map[string]interface{} {
	lastNotif := (*PushNotification)(nil)
	if v := p.metrics.lastNotification.Load(); v != nil {
		lastNotif = v.(*PushNotification)
	}

	return map[string]interface{}{
		"provider":           "mock",
		"total_sent":         p.metrics.totalSent.Load(),
		"total_failed":       p.metrics.totalFailed.Load(),
		"total_batches":      p.metrics.totalBatches.Load(),
		"total_calls":        p.callCount.Load(),
		"validation_errors":  p.metrics.validationErrors.Load(),
		"rate_limit_hits":    p.metrics.rateLimitHits.Load(),
		"is_available":       !p.shouldBeUnavailable.Load(),
		"last_notification":  lastNotif,
		"sent_count":         len(p.sentNotifications),
	}
}

// Close closes the provider
func (p *MockProvider) Close() error {
	// Nothing to close in mock provider
	return nil
}

// Test helper methods

// SetShouldFail sets whether the provider should fail
func (p *MockProvider) SetShouldFail(shouldFail bool) {
	p.shouldFail.Store(shouldFail)
}

// SetNextError sets the error to return on the next call
func (p *MockProvider) SetNextError(err error) {
	p.nextError.Store(err)
}

// SetAvailable sets whether the provider is available
func (p *MockProvider) SetAvailable(available bool) {
	p.shouldBeUnavailable.Store(!available)
}

// GetSentNotifications returns all sent notifications
func (p *MockProvider) GetSentNotifications() []*PushNotification {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Return a copy to avoid race conditions
	result := make([]*PushNotification, len(p.sentNotifications))
	copy(result, p.sentNotifications)
	return result
}

// GetSentResults returns all sent results
func (p *MockProvider) GetSentResults() []*PushResult {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Return a copy to avoid race conditions
	result := make([]*PushResult, len(p.sentResults))
	copy(result, p.sentResults)
	return result
}

// GetLastNotification returns the last sent notification
func (p *MockProvider) GetLastNotification() *PushNotification {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.sentNotifications) == 0 {
		return nil
	}

	return p.sentNotifications[len(p.sentNotifications)-1]
}

// Clear clears all recorded data
func (p *MockProvider) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.sentNotifications = make([]*PushNotification, 0)
	p.sentResults = make([]*PushResult, 0)
	p.callCount.Store(0)
	p.metrics.totalSent.Store(0)
	p.metrics.totalFailed.Store(0)
	p.metrics.totalBatches.Store(0)
	p.metrics.validationErrors.Store(0)
	p.metrics.rateLimitHits.Store(0)
}

// GetCallCount returns the number of times Send was called
func (p *MockProvider) GetCallCount() int64 {
	return p.callCount.Load()
}

// SetFailureRate sets the random failure rate (0.0 to 1.0)
func (p *MockProvider) SetFailureRate(rate float64) {
	if rate < 0 {
		rate = 0
	}
	if rate > 1 {
		rate = 1
	}
	p.failureRate = rate
}

// SetSimulateDelay sets the delay to simulate for each send
func (p *MockProvider) SetSimulateDelay(delay time.Duration) {
	p.simulateDelay = delay
}

// Private helper methods

func (p *MockProvider) recordResult(notification *PushNotification, result *PushResult) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Make a copy to avoid race conditions
	notifCopy := *notification
	resultCopy := *result

	p.sentNotifications = append(p.sentNotifications, &notifCopy)
	p.sentResults = append(p.sentResults, &resultCopy)
}

func (p *MockProvider) shouldSimulateFailure() bool {
	if p.failureRate <= 0 {
		return false
	}
	if p.failureRate >= 1 {
		return true
	}

	// Simple random failure simulation
	// In production tests, use proper random number generator
	return time.Now().UnixNano()%100 < int64(p.failureRate*100)
}

// FindNotificationByToken finds notifications sent to a specific device token
func (p *MockProvider) FindNotificationByToken(token string) []*PushNotification {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var results []*PushNotification
	for _, notif := range p.sentNotifications {
		if notif.DeviceToken == token {
			// Return a copy
			notifCopy := *notif
			results = append(results, &notifCopy)
		}
	}

	return results
}

// WasNotificationSent checks if a notification with specific properties was sent
func (p *MockProvider) WasNotificationSent(title, body, token string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, notif := range p.sentNotifications {
		if notif.Title == title && notif.Body == body && notif.DeviceToken == token {
			return true
		}
	}

	return false
}

// GetSuccessRate returns the success rate of sent notifications
func (p *MockProvider) GetSuccessRate() float64 {
	sent := p.metrics.totalSent.Load()
	failed := p.metrics.totalFailed.Load()
	total := sent + failed

	if total == 0 {
		return 1.0
	}

	return float64(sent) / float64(total)
}