package notification

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupRetryTest(t *testing.T) (*RetryManager, *Queue, *redis.Client, *miniredis.Miniredis) {
	// Create miniredis instance
	mr, err := miniredis.Run()
	require.NoError(t, err)

	// Create Redis client
	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	logger := zap.NewNop()
	config := DefaultRetryConfig()

	retryManager := NewRetryManager(client, logger, config)
	queue := NewQueue(client, logger, DefaultQueueConfig())

	return retryManager, queue, client, mr
}

func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	assert.Equal(t, 3, config.MaxAttempts, "Should have 3 max attempts")
	assert.Equal(t, 1*time.Second, config.BaseDelay, "Should have 1s base delay")
	assert.Equal(t, 4*time.Second, config.MaxDelay, "Should have 4s max delay")
	assert.Equal(t, 2.0, config.ExponentialBase, "Should have exponential base of 2.0")
	assert.True(t, config.JitterEnabled, "Jitter should be enabled")
}

func TestCalculateBackoff(t *testing.T) {
	rm, _, _, mr := setupRetryTest(t)
	defer mr.Close()

	tests := []struct {
		name           string
		attempt        int
		expectedMin    time.Duration
		expectedMax    time.Duration
		description    string
	}{
		{
			name:        "First attempt",
			attempt:     1,
			expectedMin: 900 * time.Millisecond,  // 1s - 10% jitter
			expectedMax: 1100 * time.Millisecond, // 1s + 10% jitter
			description: "Should be ~1s",
		},
		{
			name:        "Second attempt",
			attempt:     2,
			expectedMin: 1800 * time.Millisecond, // 2s - 10% jitter
			expectedMax: 2200 * time.Millisecond, // 2s + 10% jitter
			description: "Should be ~2s",
		},
		{
			name:        "Third attempt",
			attempt:     3,
			expectedMin: 3600 * time.Millisecond, // 4s - 10% jitter
			expectedMax: 4400 * time.Millisecond, // 4s + 10% jitter
			description: "Should be ~4s (capped)",
		},
		{
			name:        "Fourth attempt (beyond max)",
			attempt:     4,
			expectedMin: 3600 * time.Millisecond, // Should be capped at 4s
			expectedMax: 4400 * time.Millisecond,
			description: "Should be capped at 4s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backoff := rm.CalculateBackoff(tt.attempt)

			assert.GreaterOrEqual(t, backoff, tt.expectedMin,
				"%s: backoff should be >= %v, got %v", tt.description, tt.expectedMin, backoff)
			assert.LessOrEqual(t, backoff, tt.expectedMax,
				"%s: backoff should be <= %v, got %v", tt.description, tt.expectedMax, backoff)
		})
	}
}

func TestCalculateBackoff_ExponentialProgression(t *testing.T) {
	rm, _, _, mr := setupRetryTest(t)
	defer mr.Close()

	// Disable jitter for exact testing
	rm.config.JitterEnabled = false

	backoff1 := rm.CalculateBackoff(1)
	backoff2 := rm.CalculateBackoff(2)
	backoff3 := rm.CalculateBackoff(3)

	assert.Equal(t, 1*time.Second, backoff1, "First attempt should be 1s")
	assert.Equal(t, 2*time.Second, backoff2, "Second attempt should be 2s")
	assert.Equal(t, 4*time.Second, backoff3, "Third attempt should be 4s")
}

func TestShouldRetry(t *testing.T) {
	rm, _, _, mr := setupRetryTest(t)
	defer mr.Close()

	tests := []struct {
		name           string
		job            *NotificationJob
		err            error
		expectedRetry  bool
		description    string
	}{
		{
			name: "Should retry on transient error",
			job: &NotificationJob{
				ID:       uuid.New().String(),
				UserID:   uuid.New(),
				Attempts: 1,
			},
			err:           ErrProviderUnavailable,
			expectedRetry: true,
			description:   "Transient errors should be retried",
		},
		{
			name: "Should not retry after max attempts",
			job: &NotificationJob{
				ID:       uuid.New().String(),
				UserID:   uuid.New(),
				Attempts: 3,
			},
			err:           ErrProviderUnavailable,
			expectedRetry: false,
			description:   "Should not retry after max attempts reached",
		},
		{
			name: "Should not retry permanent errors",
			job: &NotificationJob{
				ID:       uuid.New().String(),
				UserID:   uuid.New(),
				Attempts: 1,
			},
			err:           ErrInvalidToken,
			expectedRetry: false,
			description:   "Permanent errors should not be retried",
		},
		{
			name: "Should retry rate limit errors",
			job: &NotificationJob{
				ID:       uuid.New().String(),
				UserID:   uuid.New(),
				Attempts: 0,
			},
			err:           ErrRateLimitExceeded,
			expectedRetry: true,
			description:   "Rate limit errors should be retried",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldRetry := rm.ShouldRetry(tt.job, tt.err)
			assert.Equal(t, tt.expectedRetry, shouldRetry, tt.description)
		})
	}
}

func TestScheduleRetry(t *testing.T) {
	rm, queue, _, mr := setupRetryTest(t)
	defer mr.Close()
	ctx := context.Background()

	t.Run("Successfully schedule retry", func(t *testing.T) {
		job := &NotificationJob{
			ID:          uuid.New().String(),
			UserID:      uuid.New(),
			Type:        "test",
			Payload:     map[string]interface{}{"test": "data"},
			ScheduledAt: time.Now(),
			CreatedAt:   time.Now(),
			Attempts:    0,
			MaxAttempts: 3,
		}

		err := rm.ScheduleRetry(ctx, job, queue, ErrProviderUnavailable)
		assert.NoError(t, err, "Should successfully schedule retry")
		assert.Equal(t, 1, job.Attempts, "Attempts should be incremented")
		assert.NotEmpty(t, job.LastError, "Last error should be set")
	})

	t.Run("Fail to schedule retry after max attempts", func(t *testing.T) {
		job := &NotificationJob{
			ID:          uuid.New().String(),
			UserID:      uuid.New(),
			Type:        "test",
			Payload:     map[string]interface{}{"test": "data"},
			ScheduledAt: time.Now(),
			CreatedAt:   time.Now(),
			Attempts:    3,
			MaxAttempts: 3,
		}

		err := rm.ScheduleRetry(ctx, job, queue, ErrProviderUnavailable)
		assert.Error(t, err, "Should fail to schedule retry after max attempts")
	})

	t.Run("Fail to schedule retry for permanent error", func(t *testing.T) {
		job := &NotificationJob{
			ID:          uuid.New().String(),
			UserID:      uuid.New(),
			Type:        "test",
			Payload:     map[string]interface{}{"test": "data"},
			ScheduledAt: time.Now(),
			CreatedAt:   time.Now(),
			Attempts:    0,
			MaxAttempts: 3,
		}

		err := rm.ScheduleRetry(ctx, job, queue, ErrInvalidToken)
		assert.Error(t, err, "Should fail to schedule retry for permanent error")
	})
}

func TestGetRetryMetrics(t *testing.T) {
	rm, _, _, mr := setupRetryTest(t)
	defer mr.Close()
	ctx := context.Background()

	metrics, err := rm.GetRetryMetrics(ctx)
	assert.NoError(t, err, "Should get retry metrics")
	assert.NotNil(t, metrics, "Metrics should not be nil")
	assert.Equal(t, 3, metrics["max_attempts"], "Should have correct max attempts")
	assert.Equal(t, int64(1000), metrics["base_delay_ms"], "Should have correct base delay")
	assert.Equal(t, int64(4000), metrics["max_delay_ms"], "Should have correct max delay")
}

func TestIsTransientError(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		isTransient bool
	}{
		{"Provider unavailable", ErrProviderUnavailable, true},
		{"Rate limit exceeded", ErrRateLimitExceeded, true},
		{"Invalid token", ErrInvalidToken, false},
		{"Token expired", ErrTokenExpired, false},
		{"Generic error", errors.New("generic error"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsTransientError(tt.err)
			assert.Equal(t, tt.isTransient, result)
		})
	}
}

func TestIsPermanentFailure(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		isPermanent bool
	}{
		{"Invalid token", ErrInvalidToken, true},
		{"Token expired", ErrTokenExpired, true},
		{"Authentication failed", ErrAuthenticationFailed, true},
		{"Provider unavailable", ErrProviderUnavailable, false},
		{"Rate limit", ErrRateLimitExceeded, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsPermanentFailure(tt.err)
			assert.Equal(t, tt.isPermanent, result)
		})
	}
}
