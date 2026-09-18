package notification

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Common errors for push notification providers
var (
	// ErrInvalidToken indicates the device token is invalid or malformed
	ErrInvalidToken = errors.New("invalid device token")

	// ErrTokenExpired indicates the device token has expired
	ErrTokenExpired = errors.New("device token expired")

	// ErrProviderUnavailable indicates the provider service is temporarily unavailable
	ErrProviderUnavailable = errors.New("provider service unavailable")

	// ErrRateLimitExceeded indicates the provider's rate limit has been exceeded
	ErrRateLimitExceeded = errors.New("rate limit exceeded")

	// ErrPayloadTooLarge indicates the notification payload exceeds size limits
	ErrPayloadTooLarge = errors.New("payload too large")

	// ErrAuthenticationFailed indicates authentication with the provider failed
	ErrAuthenticationFailed = errors.New("authentication failed")

	// ErrQuotaExceeded indicates the daily/monthly quota has been exceeded
	ErrQuotaExceeded = errors.New("quota exceeded")
)

// PushProvider represents the type of push notification provider
type PushProvider string

const (
	// PushProviderFCM represents Firebase Cloud Messaging (Android)
	PushProviderFCM PushProvider = "fcm"

	// PushProviderAPNs represents Apple Push Notification service (iOS)
	PushProviderAPNs PushProvider = "apns"

	// PushProviderMock represents a mock provider for testing
	PushProviderMock PushProvider = "mock"
)

// PushNotification represents a push notification to be sent
type PushNotification struct {
	// Device token (FCM token or APNs device token)
	DeviceToken string

	// Title of the notification
	Title string

	// Body text of the notification
	Body string

	// Optional badge number (iOS)
	Badge *int

	// Sound to play (default, custom sound file, or empty for silent)
	Sound string

	// Additional data payload
	Data map[string]string

	// Notification priority (high or normal)
	HighPriority bool

	// Time to live in seconds (how long to keep trying if device is offline)
	TTL int

	// Collapse key for replacing notifications (Android)
	CollapseKey string

	// Thread ID for grouping notifications (iOS)
	ThreadID string

	// Category for actionable notifications
	Category string

	// Whether this is a silent/data-only notification
	Silent bool

	// Mutable content flag (iOS) - allows notification service extension to modify
	MutableContent bool

	// Content available flag (iOS) - for background updates
	ContentAvailable bool
}

// PushResult represents the result of sending a push notification
type PushResult struct {
	// Unique message ID from the provider
	MessageID string

	// Whether the send was successful
	Success bool

	// Error if the send failed
	Error error

	// Whether the error is retryable
	Retryable bool

	// Suggested retry after time (if retryable)
	RetryAfter *time.Duration

	// Updated device token (if provider suggests a canonical token)
	CanonicalToken string

	// Whether the device token should be removed (uninstalled app)
	ShouldRemoveToken bool

	// Provider-specific response data
	ProviderResponse map[string]interface{}

	// Timestamp when the notification was sent
	SentAt time.Time
}

// PushNotificationProvider defines the interface for push notification services
type PushNotificationProvider interface {
	// Send sends a push notification to a device
	Send(ctx context.Context, notification *PushNotification) (*PushResult, error)

	// SendBatch sends multiple notifications in a batch (if supported by provider)
	SendBatch(ctx context.Context, notifications []*PushNotification) ([]*PushResult, error)

	// ValidateToken validates a device token format
	ValidateToken(token string) error

	// GetProviderType returns the type of this provider
	GetProviderType() PushProvider

	// IsAvailable checks if the provider is currently available
	IsAvailable(ctx context.Context) bool

	// GetMetrics returns provider-specific metrics
	GetMetrics() map[string]interface{}

	// Close closes any open connections or resources
	Close() error
}

// PushProviderConfig holds configuration for push providers
type PushProviderConfig struct {
	// FCM configuration
	FCMProjectID      string
	FCMCredentialsJSON string // JSON credentials for service account

	// APNs configuration
	APNsKeyID        string
	APNsTeamID       string
	APNsAuthKeyPath  string // Path to .p8 file
	APNsBundleID     string
	APNsProduction   bool   // Use production or development environment

	// Common configuration
	MaxRetries       int
	RetryBackoff     time.Duration
	Timeout          time.Duration
	MaxBatchSize     int
	RateLimitPerSec  int
}

// DefaultPushProviderConfig returns default configuration
func DefaultPushProviderConfig() *PushProviderConfig {
	return &PushProviderConfig{
		MaxRetries:      3,
		RetryBackoff:    time.Second * 2,
		Timeout:         time.Second * 30,
		MaxBatchSize:    500,
		RateLimitPerSec: 1000,
	}
}

// IsRetryableError determines if an error is retryable
func IsRetryableError(err error) bool {
	switch {
	case errors.Is(err, ErrProviderUnavailable):
		return true
	case errors.Is(err, ErrRateLimitExceeded):
		return true
	case errors.Is(err, context.DeadlineExceeded):
		return true
	case errors.Is(err, context.Canceled):
		return false
	default:
		return false
	}
}

// IsPermanentError determines if an error is permanent (token should be removed)
func IsPermanentError(err error) bool {
	switch {
	case errors.Is(err, ErrInvalidToken):
		return true
	case errors.Is(err, ErrTokenExpired):
		return true
	case errors.Is(err, ErrAuthenticationFailed):
		return true
	default:
		return false
	}
}

// GetRetryDelay calculates the retry delay based on attempt number
func GetRetryDelay(attempt int, baseDelay time.Duration) time.Duration {
	if attempt <= 0 {
		return baseDelay
	}

	// Exponential backoff with jitter
	delay := baseDelay * time.Duration(1<<uint(attempt-1))

	// Cap at 5 minutes
	maxDelay := 5 * time.Minute
	if delay > maxDelay {
		delay = maxDelay
	}

	// Add jitter (±25%)
	jitter := delay / 4
	delay = delay - jitter + time.Duration(randInt63n(int64(jitter*2)))

	return delay
}

// Helper function for random number generation (will be replaced with proper implementation)
func randInt63n(n int64) int64 {
	// Simple implementation - in production, use crypto/rand or math/rand with proper seeding
	return time.Now().UnixNano() % n
}

// ValidatePushNotification validates a push notification before sending
func ValidatePushNotification(notification *PushNotification) error {
	if notification == nil {
		return fmt.Errorf("notification cannot be nil")
	}

	if notification.DeviceToken == "" {
		return fmt.Errorf("device token is required")
	}

	// Check payload size (rough estimate)
	estimatedSize := len(notification.Title) + len(notification.Body)
	for k, v := range notification.Data {
		estimatedSize += len(k) + len(v)
	}

	// FCM has a 4KB limit, APNs has a 4KB limit for regular notifications
	if estimatedSize > 4000 {
		return ErrPayloadTooLarge
	}

	// Validate TTL
	if notification.TTL < 0 {
		return fmt.Errorf("TTL cannot be negative")
	}

	if notification.TTL > 2419200 { // 28 days max for most providers
		return fmt.Errorf("TTL exceeds maximum allowed value")
	}

	return nil
}

// FormatDeviceToken formats a device token for the specific provider
func FormatDeviceToken(token string, provider PushProvider) string {
	switch provider {
	case PushProviderAPNs:
		// Remove any spaces or angle brackets from APNs tokens
		formatted := ""
		for _, r := range token {
			if (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') || (r >= '0' && r <= '9') {
				formatted += string(r)
			}
		}
		return formatted

	case PushProviderFCM:
		// FCM tokens are already in the correct format
		return token

	default:
		return token
	}
}