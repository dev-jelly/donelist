package notification

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

// Processor handles the actual sending of notifications
type Processor struct {
	logger *zap.Logger
	// Will be extended in later subtasks with:
	// - FCM sender
	// - APNs sender
	// - User settings repository
	// - Checkin repository
}

// NewProcessor creates a new notification processor
func NewProcessor(logger *zap.Logger) *Processor {
	return &Processor{
		logger: logger,
	}
}

// Process processes a notification job
// This is a placeholder implementation that will be extended in subtask 2.3
func (p *Processor) Process(ctx context.Context, job *NotificationJob) error {
	p.logger.Info("Processing notification (placeholder)",
		zap.String("job_id", job.ID),
		zap.String("user_id", job.UserID.String()),
		zap.String("type", job.Type),
	)

	// TODO: Implement in subtask 2.3 (FCM/APNs adapter)
	// - Check user notification settings
	// - Check DnD settings
	// - Send via appropriate provider (FCM/APNs)
	// - Handle provider-specific errors

	return nil
}

// ValidateJob validates a notification job before processing
func (p *Processor) ValidateJob(job *NotificationJob) error {
	if job.ID == "" {
		return fmt.Errorf("job ID is required")
	}
	if job.UserID.String() == "" {
		return fmt.Errorf("user ID is required")
	}
	if job.Type == "" {
		return fmt.Errorf("job type is required")
	}

	return nil
}
