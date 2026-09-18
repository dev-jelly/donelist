package notification

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// EnhancedProcessor handles notification processing with retry, idempotency, and circuit breaker
type EnhancedProcessor struct {
	logger              *zap.Logger
	queue               *Queue
	retryManager        *RetryManager
	idempotencyManager  *IdempotencyManager
	circuitBreakerMgr   *CircuitBreakerManager
	providerManager     *ProviderManager // Will be set in subtask 2.3
}

// NewEnhancedProcessor creates a new enhanced processor with all retry and resilience features
func NewEnhancedProcessor(
	logger *zap.Logger,
	queue *Queue,
	redisClient *redis.Client,
) *EnhancedProcessor {
	return &EnhancedProcessor{
		logger:              logger,
		queue:               queue,
		retryManager:        NewRetryManager(redisClient, logger, DefaultRetryConfig()),
		idempotencyManager:  NewIdempotencyManager(redisClient, logger, DefaultIdempotencyConfig()),
		circuitBreakerMgr:   NewCircuitBreakerManager(redisClient, logger, DefaultCircuitBreakerConfig()),
	}
}

// SetProviderManager sets the provider manager (will be called after provider setup in subtask 2.3)
func (ep *EnhancedProcessor) SetProviderManager(pm *ProviderManager) {
	ep.providerManager = pm
}

// Process processes a notification job with full retry, idempotency, and circuit breaker logic
func (ep *EnhancedProcessor) Process(ctx context.Context, job *NotificationJob) error {
	ep.logger.Debug("Processing notification with enhanced processor",
		zap.String("job_id", job.ID),
		zap.String("user_id", job.UserID.String()),
		zap.String("type", job.Type),
		zap.Int("attempt", job.Attempts+1),
	)

	// Step 1: Check for duplicates using idempotency
	isDuplicate, err := ep.idempotencyManager.CheckDuplicate(ctx, job)
	if err != nil {
		ep.logger.Error("Failed to check idempotency",
			zap.String("job_id", job.ID),
			zap.Error(err),
		)
		// Continue processing despite idempotency check failure
	} else if isDuplicate {
		ep.logger.Info("Skipping duplicate notification",
			zap.String("job_id", job.ID),
			zap.String("user_id", job.UserID.String()),
			zap.String("type", job.Type),
		)
		// Mark as complete since it's a duplicate
		return nil
	}

	// Step 2: Validate the job
	if err := ep.validateJob(job); err != nil {
		// Validation errors are permanent - move to DLQ
		ep.logger.Error("Job validation failed",
			zap.String("job_id", job.ID),
			zap.Error(err),
		)
		return ep.queue.MoveToDLQ(ctx, job, fmt.Sprintf("validation failed: %s", err.Error()))
	}

	// Step 3: Process the notification (placeholder until subtask 2.3)
	err = ep.processNotification(ctx, job)

	// Step 4: Handle the result
	if err != nil {
		return ep.handleFailure(ctx, job, err)
	}

	// Step 5: Mark as processed in idempotency store
	if err := ep.idempotencyManager.MarkProcessed(ctx, job); err != nil {
		ep.logger.Error("Failed to mark notification as processed in idempotency store",
			zap.String("job_id", job.ID),
			zap.Error(err),
		)
		// Don't fail the job just because idempotency marking failed
	}

	ep.logger.Info("Notification processed successfully",
		zap.String("job_id", job.ID),
		zap.String("user_id", job.UserID.String()),
		zap.String("type", job.Type),
	)

	return nil
}

// processNotification sends the actual notification through the provider
// This is a placeholder that will be fully implemented in subtask 2.3
func (ep *EnhancedProcessor) processNotification(ctx context.Context, job *NotificationJob) error {
	// TODO: Implement in subtask 2.3 with actual FCM/APNs providers
	// For now, this is a placeholder that demonstrates the flow

	// Example of how circuit breaker will be used:
	// provider := ep.providerManager.GetProvider(job.ProviderType)
	// breaker := ep.circuitBreakerMgr.GetBreaker(provider)
	//
	// return breaker.Call(ctx, func() error {
	//     return ep.providerManager.SendNotification(ctx, notification)
	// })

	ep.logger.Debug("Processing notification (placeholder - to be implemented in subtask 2.3)",
		zap.String("job_id", job.ID),
	)

	return nil // Success for now
}

// handleFailure handles a failed notification with retry logic
func (ep *EnhancedProcessor) handleFailure(ctx context.Context, job *NotificationJob, err error) error {
	ep.logger.Error("Notification processing failed",
		zap.String("job_id", job.ID),
		zap.Error(err),
		zap.Int("attempt", job.Attempts+1),
	)

	// Check if this is a permanent error
	if IsPermanentFailure(err) {
		ep.logger.Warn("Permanent failure detected, moving to DLQ",
			zap.String("job_id", job.ID),
			zap.Error(err),
		)
		return ep.queue.MoveToDLQ(ctx, job, fmt.Sprintf("permanent failure: %s", err.Error()))
	}

	// Try to schedule a retry
	if err := ep.retryManager.ScheduleRetry(ctx, job, ep.queue, err); err != nil {
		// If retry scheduling fails, move to DLQ
		ep.logger.Error("Failed to schedule retry, moving to DLQ",
			zap.String("job_id", job.ID),
			zap.Error(err),
		)
		return ep.queue.MoveToDLQ(ctx, job, fmt.Sprintf("retry scheduling failed: %s", err.Error()))
	}

	return nil // Successfully scheduled for retry
}

// validateJob validates a notification job
func (ep *EnhancedProcessor) validateJob(job *NotificationJob) error {
	if job.ID == "" {
		return fmt.Errorf("job ID is required")
	}
	if job.UserID.String() == "" || job.UserID.String() == "00000000-0000-0000-0000-000000000000" {
		return fmt.Errorf("valid user ID is required")
	}
	if job.Type == "" {
		return fmt.Errorf("job type is required")
	}
	if job.Payload == nil {
		return fmt.Errorf("job payload is required")
	}

	return nil
}

// GetMetrics returns metrics from all components
func (ep *EnhancedProcessor) GetMetrics(ctx context.Context) (map[string]interface{}, error) {
	retryMetrics, _ := ep.retryManager.GetRetryMetrics(ctx)
	idempotencyMetrics, _ := ep.idempotencyManager.GetStats(ctx)
	circuitBreakerMetrics := ep.circuitBreakerMgr.GetAllMetrics()

	return map[string]interface{}{
		"retry":          retryMetrics,
		"idempotency":    idempotencyMetrics,
		"circuit_breaker": circuitBreakerMetrics,
	}, nil
}
