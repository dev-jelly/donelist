package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// RetryConfig holds retry configuration
type RetryConfig struct {
	MaxAttempts      int           // Maximum retry attempts (default: 3)
	BaseDelay        time.Duration // Base delay for exponential backoff (default: 1s)
	MaxDelay         time.Duration // Maximum delay cap (default: 4s)
	ExponentialBase  float64       // Exponential base multiplier (default: 2.0)
	JitterEnabled    bool          // Add jitter to prevent thundering herd
	JitterPercent    float64       // Jitter percentage (default: 0.1 = 10%)
}

// DefaultRetryConfig returns the default retry configuration
// Max retry attempts: 3 for transient failures
// Exponential backoff: 1s, 2s, 4s
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:     3,
		BaseDelay:       1 * time.Second,
		MaxDelay:        4 * time.Second,
		ExponentialBase: 2.0,
		JitterEnabled:   true,
		JitterPercent:   0.1,
	}
}

// RetryManager handles retry logic with exponential backoff
type RetryManager struct {
	redis  *redis.Client
	logger *zap.Logger
	config RetryConfig
}

// NewRetryManager creates a new retry manager
func NewRetryManager(redisClient *redis.Client, logger *zap.Logger, config RetryConfig) *RetryManager {
	return &RetryManager{
		redis:  redisClient,
		logger: logger,
		config: config,
	}
}

// CalculateBackoff calculates the backoff duration for a given attempt
// Uses exponential backoff: 1s, 2s, 4s with optional jitter
func (rm *RetryManager) CalculateBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return rm.config.BaseDelay
	}

	// Calculate exponential backoff: baseDelay * (exponentialBase ^ (attempt - 1))
	// attempt 1: 1s * (2^0) = 1s
	// attempt 2: 1s * (2^1) = 2s
	// attempt 3: 1s * (2^2) = 4s
	backoff := rm.config.BaseDelay
	for i := 1; i < attempt; i++ {
		backoff = time.Duration(float64(backoff) * rm.config.ExponentialBase)
		if backoff > rm.config.MaxDelay {
			backoff = rm.config.MaxDelay
			break
		}
	}

	// Cap at max delay
	if backoff > rm.config.MaxDelay {
		backoff = rm.config.MaxDelay
	}

	// Add jitter if enabled to prevent thundering herd
	if rm.config.JitterEnabled {
		jitter := float64(backoff) * rm.config.JitterPercent
		jitterRange := int64(jitter * 2)
		if jitterRange > 0 {
			jitterOffset := time.Duration(randInt63n(jitterRange)) - time.Duration(jitter)
			backoff = backoff + jitterOffset
		}
	}

	return backoff
}

// ShouldRetry determines if a notification should be retried
func (rm *RetryManager) ShouldRetry(job *NotificationJob, err error) bool {
	// Don't retry if max attempts reached
	if job.Attempts >= rm.config.MaxAttempts {
		rm.logger.Info("Max retry attempts reached",
			zap.String("job_id", job.ID),
			zap.Int("attempts", job.Attempts),
			zap.Int("max_attempts", rm.config.MaxAttempts),
		)
		return false
	}

	// Check if the error is retryable
	if !IsRetryableError(err) {
		rm.logger.Info("Error is not retryable",
			zap.String("job_id", job.ID),
			zap.Error(err),
		)
		return false
	}

	return true
}

// ScheduleRetry schedules a job for retry with exponential backoff
func (rm *RetryManager) ScheduleRetry(ctx context.Context, job *NotificationJob, queue *Queue, err error) error {
	job.Attempts++
	job.LastError = err.Error()

	if !rm.ShouldRetry(job, err) {
		rm.logger.Warn("Not scheduling retry for job",
			zap.String("job_id", job.ID),
			zap.Int("attempts", job.Attempts),
			zap.Error(err),
		)
		return fmt.Errorf("retry not allowed: %w", err)
	}

	// Calculate backoff delay
	backoffDelay := rm.CalculateBackoff(job.Attempts)
	job.ScheduledAt = time.Now().Add(backoffDelay)

	// Re-enqueue with new scheduled time
	if err := queue.Enqueue(ctx, job); err != nil {
		return fmt.Errorf("failed to enqueue retry: %w", err)
	}

	rm.logger.Info("Scheduled job for retry",
		zap.String("job_id", job.ID),
		zap.Int("attempt", job.Attempts),
		zap.Int("max_attempts", rm.config.MaxAttempts),
		zap.Duration("backoff", backoffDelay),
		zap.Time("retry_at", job.ScheduledAt),
		zap.String("error", err.Error()),
	)

	return nil
}

// GetRetryMetrics returns retry metrics
func (rm *RetryManager) GetRetryMetrics(ctx context.Context) (map[string]interface{}, error) {
	// This would typically query metrics from Redis or a metrics store
	// For now, return configuration as metrics
	return map[string]interface{}{
		"max_attempts":     rm.config.MaxAttempts,
		"base_delay_ms":    rm.config.BaseDelay.Milliseconds(),
		"max_delay_ms":     rm.config.MaxDelay.Milliseconds(),
		"exponential_base": rm.config.ExponentialBase,
		"jitter_enabled":   rm.config.JitterEnabled,
		"jitter_percent":   rm.config.JitterPercent,
	}, nil
}

// IsTransientError determines if an error is transient and should be retried
func IsTransientError(err error) bool {
	// Check for common transient errors
	return IsRetryableError(err)
}

// IsPermanentFailure determines if a failure is permanent and should go to DLQ
func IsPermanentFailure(err error) bool {
	return IsPermanentError(err)
}
