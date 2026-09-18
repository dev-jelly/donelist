package subscriptionjobs

import (
	"context"
	"fmt"
	"time"

	"github.com/dev-jelly/donelist/internal/notification"
	"github.com/dev-jelly/donelist/internal/subscription"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// DunningService handles payment failure retry logic
type DunningService struct {
	db                *sqlx.DB
	repo              *subscription.Repository
	notificationQueue NotificationQueue
	paymentService    PaymentRetryService
	logger            *zap.Logger
}

// PaymentRetryService defines the interface for retrying payments
type PaymentRetryService interface {
	RetryPayment(ctx context.Context, subscriptionID uuid.UUID) error
}

// DunningConfig holds dunning configuration
type DunningConfig struct {
	MaxRetries           int           // Maximum number of retry attempts
	RetryIntervals       []time.Duration // Retry intervals (e.g., 3 days, 7 days, 14 days)
	GracePeriodDays      int           // Days of grace period after last retry
	EnableNotifications  bool          // Send notifications on payment failures
	AutoCancelAfterGrace bool          // Automatically cancel after grace period
}

// DefaultDunningConfig returns the default dunning configuration
func DefaultDunningConfig() *DunningConfig {
	return &DunningConfig{
		MaxRetries: 3,
		RetryIntervals: []time.Duration{
			3 * 24 * time.Hour,  // 3 days
			7 * 24 * time.Hour,  // 7 days
			14 * 24 * time.Hour, // 14 days
		},
		GracePeriodDays:      7,
		EnableNotifications:  true,
		AutoCancelAfterGrace: true,
	}
}

// DunningAttempt represents a payment retry attempt
type DunningAttempt struct {
	ID             uuid.UUID  `db:"id" json:"id"`
	SubscriptionID uuid.UUID  `db:"subscription_id" json:"subscription_id"`
	PaymentID      *uuid.UUID `db:"payment_id" json:"payment_id,omitempty"`
	AttemptNumber  int        `db:"attempt_number" json:"attempt_number"`
	Status         string     `db:"status" json:"status"` // pending, retrying, succeeded, failed
	ScheduledFor   time.Time  `db:"scheduled_for" json:"scheduled_for"`
	AttemptedAt    *time.Time `db:"attempted_at" json:"attempted_at,omitempty"`
	SucceededAt    *time.Time `db:"succeeded_at" json:"succeeded_at,omitempty"`
	FailedAt       *time.Time `db:"failed_at" json:"failed_at,omitempty"`
	FailureReason  *string    `db:"failure_reason" json:"failure_reason,omitempty"`
	NextRetryAt    *time.Time `db:"next_retry_at" json:"next_retry_at,omitempty"`
	CreatedAt      time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time  `db:"updated_at" json:"updated_at"`
}

// Dunning attempt statuses
const (
	DunningStatusPending   = "pending"
	DunningStatusRetrying  = "retrying"
	DunningStatusSucceeded = "succeeded"
	DunningStatusFailed    = "failed"
	DunningStatusCanceled  = "canceled"
)

// NewDunningService creates a new dunning service
func NewDunningService(db *sqlx.DB, repo *subscription.Repository, notificationQueue NotificationQueue, paymentService PaymentRetryService, logger *zap.Logger) *DunningService {
	return &DunningService{
		db:                db,
		repo:              repo,
		notificationQueue: notificationQueue,
		paymentService:    paymentService,
		logger:            logger,
	}
}

// ProcessPaymentFailures handles payment failures and creates dunning attempts
func (s *DunningService) ProcessPaymentFailures(ctx context.Context, config *DunningConfig) error {
	s.logger.Info("Processing payment failures and dunning retries")

	// Process new payment failures
	if err := s.processNewFailures(ctx, config); err != nil {
		s.logger.Error("Failed to process new payment failures", zap.Error(err))
		return err
	}

	// Process scheduled retries
	if err := s.processScheduledRetries(ctx, config); err != nil {
		s.logger.Error("Failed to process scheduled retries", zap.Error(err))
		return err
	}

	// Process grace period expirations
	if err := s.processGracePeriodExpirations(ctx, config); err != nil {
		s.logger.Error("Failed to process grace period expirations", zap.Error(err))
		return err
	}

	return nil
}

// processNewFailures creates dunning attempts for new payment failures
func (s *DunningService) processNewFailures(ctx context.Context, config *DunningConfig) error {
	// Find subscriptions in past_due status without active dunning
	query := `
		SELECT s.id, s.user_id, s.plan_id, s.status, s.current_period_start, s.current_period_end,
			s.cancel_at_period_end, s.canceled_at, s.trial_start, s.trial_end,
			s.stripe_customer_id, s.stripe_subscription_id, s.metadata, s.created_at, s.updated_at
		FROM subscriptions s
		WHERE s.status = 'past_due'
			AND NOT EXISTS (
				SELECT 1 FROM dunning_attempts da
				WHERE da.subscription_id = s.id
					AND da.status IN ('pending', 'retrying')
			)
	`

	var subs []*Subscription
	if err := s.db.SelectContext(ctx, &subs, query); err != nil {
		return fmt.Errorf("failed to get past due subscriptions: %w", err)
	}

	s.logger.Info("Found subscriptions with payment failures",
		zap.Int("count", len(subs)))

	for _, sub := range subs {
		if err := s.createDunningSequence(ctx, sub, config); err != nil {
			s.logger.Error("Failed to create dunning sequence",
				zap.String("subscription_id", sub.ID.String()),
				zap.Error(err))
		}
	}

	return nil
}

// createDunningSequence creates a sequence of dunning attempts
func (s *DunningService) createDunningSequence(ctx context.Context, sub *Subscription, config *DunningConfig) error {
	s.logger.Info("Creating dunning sequence",
		zap.String("subscription_id", sub.ID.String()),
		zap.Int("max_retries", config.MaxRetries))

	now := time.Now()
	for i := 0; i < config.MaxRetries; i++ {
		var scheduledFor time.Time
		if i < len(config.RetryIntervals) {
			scheduledFor = now.Add(config.RetryIntervals[i])
		} else {
			// Use last interval for any additional retries
			scheduledFor = now.Add(config.RetryIntervals[len(config.RetryIntervals)-1])
		}

		attempt := &DunningAttempt{
			ID:             uuid.New(),
			SubscriptionID: sub.ID,
			AttemptNumber:  i + 1,
			Status:         DunningStatusPending,
			ScheduledFor:   scheduledFor,
			CreatedAt:      now,
			UpdatedAt:      now,
		}

		if err := s.createDunningAttempt(ctx, attempt); err != nil {
			return fmt.Errorf("failed to create dunning attempt: %w", err)
		}

		s.logger.Info("Dunning attempt scheduled",
			zap.String("subscription_id", sub.ID.String()),
			zap.Int("attempt", i+1),
			zap.Time("scheduled_for", scheduledFor))
	}

	// Send initial payment failure notification
	if config.EnableNotifications {
		if err := s.sendPaymentFailureNotification(ctx, sub, 1, config.MaxRetries); err != nil {
			s.logger.Error("Failed to send payment failure notification",
				zap.String("subscription_id", sub.ID.String()),
				zap.Error(err))
		}
	}

	// Log event
	if err := s.repo.LogSubscriptionEvent(ctx, sub.ID, subscription.EventTypePaymentFailed, nil, map[string]interface{}{
		"dunning_started": true,
		"max_retries":     config.MaxRetries,
	}, "Payment failed, dunning sequence started"); err != nil {
		s.logger.Warn("Failed to log subscription event", zap.Error(err))
	}

	return nil
}

// processScheduledRetries processes dunning attempts that are due
func (s *DunningService) processScheduledRetries(ctx context.Context, config *DunningConfig) error {
	query := `
		SELECT id, subscription_id, payment_id, attempt_number, status, scheduled_for,
			attempted_at, succeeded_at, failed_at, failure_reason, next_retry_at,
			created_at, updated_at
		FROM dunning_attempts
		WHERE status = 'pending'
			AND scheduled_for <= $1
		ORDER BY scheduled_for ASC
	`

	var attempts []*DunningAttempt
	if err := s.db.SelectContext(ctx, &attempts, query, time.Now()); err != nil {
		return fmt.Errorf("failed to get scheduled dunning attempts: %w", err)
	}

	s.logger.Info("Processing scheduled dunning retries",
		zap.Int("count", len(attempts)))

	for _, attempt := range attempts {
		if err := s.processDunningAttempt(ctx, attempt, config); err != nil {
			s.logger.Error("Failed to process dunning attempt",
				zap.String("attempt_id", attempt.ID.String()),
				zap.Error(err))
		}
	}

	return nil
}

// processDunningAttempt processes a single dunning attempt
func (s *DunningService) processDunningAttempt(ctx context.Context, attempt *DunningAttempt, config *DunningConfig) error {
	s.logger.Info("Processing dunning attempt",
		zap.String("attempt_id", attempt.ID.String()),
		zap.String("subscription_id", attempt.SubscriptionID.String()),
		zap.Int("attempt_number", attempt.AttemptNumber))

	// Update status to retrying
	now := time.Now()
	attempt.Status = DunningStatusRetrying
	attempt.AttemptedAt = &now
	attempt.UpdatedAt = now

	if err := s.updateDunningAttempt(ctx, attempt); err != nil {
		return fmt.Errorf("failed to update dunning attempt status: %w", err)
	}

	// Attempt payment retry
	err := s.paymentService.RetryPayment(ctx, attempt.SubscriptionID)
	if err != nil {
		// Payment retry failed
		failureReason := err.Error()
		attempt.Status = DunningStatusFailed
		attempt.FailedAt = &now
		attempt.FailureReason = &failureReason

		if err := s.updateDunningAttempt(ctx, attempt); err != nil {
			return fmt.Errorf("failed to update failed dunning attempt: %w", err)
		}

		// Send notification
		if config.EnableNotifications {
			sub, err := s.repo.GetSubscription(ctx, attempt.SubscriptionID)
			if err != nil {
				s.logger.Error("Failed to get subscription for notification",
					zap.String("subscription_id", attempt.SubscriptionID.String()),
					zap.Error(err))
			} else {
				if err := s.sendPaymentFailureNotification(ctx, sub, attempt.AttemptNumber, config.MaxRetries); err != nil {
					s.logger.Error("Failed to send payment failure notification",
						zap.String("subscription_id", sub.ID.String()),
						zap.Error(err))
				}
			}
		}

		s.logger.Warn("Dunning attempt failed",
			zap.String("attempt_id", attempt.ID.String()),
			zap.Int("attempt_number", attempt.AttemptNumber),
			zap.String("reason", failureReason))
	} else {
		// Payment retry succeeded
		attempt.Status = DunningStatusSucceeded
		attempt.SucceededAt = &now

		if err := s.updateDunningAttempt(ctx, attempt); err != nil {
			return fmt.Errorf("failed to update successful dunning attempt: %w", err)
		}

		// Cancel remaining pending attempts
		if err := s.cancelRemainingAttempts(ctx, attempt.SubscriptionID); err != nil {
			s.logger.Warn("Failed to cancel remaining attempts", zap.Error(err))
		}

		// Send success notification
		if config.EnableNotifications {
			sub, err := s.repo.GetSubscription(ctx, attempt.SubscriptionID)
			if err != nil {
				s.logger.Error("Failed to get subscription for notification",
					zap.String("subscription_id", attempt.SubscriptionID.String()),
					zap.Error(err))
			} else {
				if err := s.sendPaymentSuccessNotification(ctx, sub); err != nil {
					s.logger.Error("Failed to send payment success notification",
						zap.String("subscription_id", sub.ID.String()),
						zap.Error(err))
				}
			}
		}

		s.logger.Info("Dunning attempt succeeded",
			zap.String("attempt_id", attempt.ID.String()),
			zap.Int("attempt_number", attempt.AttemptNumber))
	}

	return nil
}

// processGracePeriodExpirations handles subscriptions past grace period
func (s *DunningService) processGracePeriodExpirations(ctx context.Context, config *DunningConfig) error {
	// Find subscriptions with all dunning attempts failed and past grace period
	query := `
		SELECT DISTINCT s.id, s.user_id, s.plan_id, s.status, s.current_period_start, s.current_period_end,
			s.cancel_at_period_end, s.canceled_at, s.trial_start, s.trial_end,
			s.stripe_customer_id, s.stripe_subscription_id, s.metadata, s.created_at, s.updated_at
		FROM subscriptions s
		INNER JOIN dunning_attempts da ON da.subscription_id = s.id
		WHERE s.status = 'past_due'
			AND da.status = 'failed'
			AND da.failed_at <= $1
			AND NOT EXISTS (
				SELECT 1 FROM dunning_attempts da2
				WHERE da2.subscription_id = s.id
					AND da2.status IN ('pending', 'retrying')
			)
		GROUP BY s.id
		HAVING MAX(da.attempt_number) >= $2
	`

	gracePeriodEnd := time.Now().Add(-time.Duration(config.GracePeriodDays) * 24 * time.Hour)
	var subs []*Subscription
	if err := s.db.SelectContext(ctx, &subs, query, gracePeriodEnd, config.MaxRetries); err != nil {
		return fmt.Errorf("failed to get grace period expired subscriptions: %w", err)
	}

	s.logger.Info("Found subscriptions past grace period",
		zap.Int("count", len(subs)))

	for _, sub := range subs {
		if err := s.handleGracePeriodExpiration(ctx, sub, config); err != nil {
			s.logger.Error("Failed to handle grace period expiration",
				zap.String("subscription_id", sub.ID.String()),
				zap.Error(err))
		}
	}

	return nil
}

// handleGracePeriodExpiration handles a subscription that exceeded grace period
func (s *DunningService) handleGracePeriodExpiration(ctx context.Context, sub *Subscription, config *DunningConfig) error {
	s.logger.Info("Handling grace period expiration",
		zap.String("subscription_id", sub.ID.String()),
		zap.Bool("auto_cancel", config.AutoCancelAfterGrace))

	if config.AutoCancelAfterGrace {
		// Transition to expired status
		sub.Status = subscription.StatusExpired
		if err := s.repo.UpdateSubscription(ctx, sub); err != nil {
			return fmt.Errorf("failed to update subscription status: %w", err)
		}

		// Log event
		if err := s.repo.LogSubscriptionEvent(ctx, sub.ID, subscription.EventTypeExpired, nil, nil, subscription.ReasonGracePeriodExpired.Description); err != nil {
			s.logger.Warn("Failed to log subscription event", zap.Error(err))
		}

		// Send final notification
		if config.EnableNotifications {
			if err := s.sendGracePeriodExpiredNotification(ctx, sub); err != nil {
				s.logger.Error("Failed to send grace period expired notification",
					zap.String("subscription_id", sub.ID.String()),
					zap.Error(err))
			}
		}

		s.logger.Info("Subscription expired after grace period",
			zap.String("subscription_id", sub.ID.String()))
	}

	return nil
}

// sendPaymentFailureNotification sends a notification about payment failure
func (s *DunningService) sendPaymentFailureNotification(ctx context.Context, sub *Subscription, attemptNumber, maxAttempts int) error {
	title := "Payment Failed"
	body := fmt.Sprintf("We couldn't process your payment for %s subscription. Attempt %d of %d. Please update your payment method.",
		sub.PlanID, attemptNumber, maxAttempts)

	notif := &notification.Notification{
		ID:           uuid.New(),
		UserID:       sub.UserID,
		Type:         notification.NotificationTypePayment,
		Title:        title,
		Body:         body,
		Priority:     notification.PriorityHigh,
		Status:       notification.StatusPending,
		ScheduledFor: time.Now(),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		MaxRetries:   3,
		Data: map[string]interface{}{
			"subscription_id": sub.ID.String(),
			"plan_id":         sub.PlanID,
			"attempt_number":  attemptNumber,
			"max_attempts":    maxAttempts,
			"event_type":      "payment_failed",
		},
	}

	return s.notificationQueue.SendNotification(ctx, notif)
}

// sendPaymentSuccessNotification sends a notification about successful payment
func (s *DunningService) sendPaymentSuccessNotification(ctx context.Context, sub *Subscription) error {
	title := "Payment Successful"
	body := fmt.Sprintf("Your payment for %s subscription has been processed successfully. Thank you!", sub.PlanID)

	notif := &notification.Notification{
		ID:           uuid.New(),
		UserID:       sub.UserID,
		Type:         notification.NotificationTypePayment,
		Title:        title,
		Body:         body,
		Priority:     notification.PriorityNormal,
		Status:       notification.StatusPending,
		ScheduledFor: time.Now(),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		MaxRetries:   3,
		Data: map[string]interface{}{
			"subscription_id": sub.ID.String(),
			"plan_id":         sub.PlanID,
			"event_type":      "payment_succeeded",
		},
	}

	return s.notificationQueue.SendNotification(ctx, notif)
}

// sendGracePeriodExpiredNotification sends notification when grace period expires
func (s *DunningService) sendGracePeriodExpiredNotification(ctx context.Context, sub *Subscription) error {
	title := "Subscription Expired"
	body := fmt.Sprintf("Your %s subscription has been canceled due to payment failure. Resubscribe anytime to restore access.", sub.PlanID)

	notif := &notification.Notification{
		ID:           uuid.New(),
		UserID:       sub.UserID,
		Type:         notification.NotificationTypeSubscription,
		Title:        title,
		Body:         body,
		Priority:     notification.PriorityHigh,
		Status:       notification.StatusPending,
		ScheduledFor: time.Now(),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		MaxRetries:   3,
		Data: map[string]interface{}{
			"subscription_id": sub.ID.String(),
			"plan_id":         sub.PlanID,
			"event_type":      "grace_period_expired",
		},
	}

	return s.notificationQueue.SendNotification(ctx, notif)
}

// createDunningAttempt creates a new dunning attempt record
func (s *DunningService) createDunningAttempt(ctx context.Context, attempt *DunningAttempt) error {
	query := `
		INSERT INTO dunning_attempts (
			id, subscription_id, payment_id, attempt_number, status, scheduled_for,
			attempted_at, succeeded_at, failed_at, failure_reason, next_retry_at,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`

	_, err := s.db.ExecContext(ctx, query,
		attempt.ID, attempt.SubscriptionID, attempt.PaymentID, attempt.AttemptNumber,
		attempt.Status, attempt.ScheduledFor, attempt.AttemptedAt, attempt.SucceededAt,
		attempt.FailedAt, attempt.FailureReason, attempt.NextRetryAt,
		attempt.CreatedAt, attempt.UpdatedAt)
	return err
}

// updateDunningAttempt updates a dunning attempt record
func (s *DunningService) updateDunningAttempt(ctx context.Context, attempt *DunningAttempt) error {
	query := `
		UPDATE dunning_attempts SET
			status = $2, attempted_at = $3, succeeded_at = $4, failed_at = $5,
			failure_reason = $6, next_retry_at = $7, updated_at = $8
		WHERE id = $1
	`

	attempt.UpdatedAt = time.Now()
	_, err := s.db.ExecContext(ctx, query,
		attempt.ID, attempt.Status, attempt.AttemptedAt, attempt.SucceededAt,
		attempt.FailedAt, attempt.FailureReason, attempt.NextRetryAt, attempt.UpdatedAt)
	return err
}

// cancelRemainingAttempts cancels all pending dunning attempts for a subscription
func (s *DunningService) cancelRemainingAttempts(ctx context.Context, subscriptionID uuid.UUID) error {
	query := `
		UPDATE dunning_attempts
		SET status = 'canceled', updated_at = $2
		WHERE subscription_id = $1 AND status = 'pending'
	`

	_, err := s.db.ExecContext(ctx, query, subscriptionID, time.Now())
	return err
}

// GetDunningStatus returns the current dunning status for a subscription
func (s *DunningService) GetDunningStatus(ctx context.Context, subscriptionID uuid.UUID) (map[string]interface{}, error) {
	query := `
		SELECT id, subscription_id, payment_id, attempt_number, status, scheduled_for,
			attempted_at, succeeded_at, failed_at, failure_reason, next_retry_at,
			created_at, updated_at
		FROM dunning_attempts
		WHERE subscription_id = $1
		ORDER BY attempt_number ASC
	`

	var attempts []*DunningAttempt
	if err := s.db.SelectContext(ctx, &attempts, query, subscriptionID); err != nil {
		return nil, fmt.Errorf("failed to get dunning attempts: %w", err)
	}

	status := map[string]interface{}{
		"subscription_id":  subscriptionID.String(),
		"total_attempts":   len(attempts),
		"pending_attempts": 0,
		"failed_attempts":  0,
		"attempts":         attempts,
		"in_grace_period":  false,
	}

	for _, attempt := range attempts {
		switch attempt.Status {
		case DunningStatusPending:
			status["pending_attempts"] = status["pending_attempts"].(int) + 1
		case DunningStatusFailed:
			status["failed_attempts"] = status["failed_attempts"].(int) + 1
		case DunningStatusSucceeded:
			status["recovered_at"] = attempt.SucceededAt
		}
	}

	// Check if in grace period
	if len(attempts) > 0 && status["pending_attempts"].(int) == 0 && status["failed_attempts"].(int) > 0 {
		lastAttempt := attempts[len(attempts)-1]
		if lastAttempt.FailedAt != nil {
			status["in_grace_period"] = true
			status["grace_period_ends"] = lastAttempt.FailedAt.Add(7 * 24 * time.Hour)
		}
	}

	return status, nil
}
