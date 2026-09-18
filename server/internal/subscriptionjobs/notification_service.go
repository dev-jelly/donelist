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

// NotificationService handles subscription-related notifications
type NotificationService struct {
	db                *sqlx.DB
	repo              *subscription.Repository
	notificationQueue NotificationQueue
	logger            *zap.Logger
}

// NotificationQueue defines the interface for sending notifications
type NotificationQueue interface {
	SendNotification(ctx context.Context, notif *notification.Notification) error
}

// NotificationConfig holds notification configuration
type NotificationConfig struct {
	PreExpiryDays      []int // Days before expiry to send notifications (e.g., 7, 3, 1)
	PostExpiryDays     []int // Days after expiry to send notifications (e.g., 1, 3, 7)
	EnableNotifications bool
}

// DefaultNotificationConfig returns the default notification configuration
func DefaultNotificationConfig() *NotificationConfig {
	return &NotificationConfig{
		PreExpiryDays:      []int{7, 3, 1},
		PostExpiryDays:     []int{1, 3, 7},
		EnableNotifications: true,
	}
}

// NewNotificationService creates a new subscription notification service
func NewNotificationService(db *sqlx.DB, repo *subscription.Repository, notificationQueue NotificationQueue, logger *zap.Logger) *NotificationService {
	return &NotificationService{
		db:                db,
		repo:              repo,
		notificationQueue: notificationQueue,
		logger:            logger,
	}
}

// ProcessExpiryNotifications finds and sends pre-expiry notifications
func (s *NotificationService) ProcessExpiryNotifications(ctx context.Context, config *NotificationConfig) error {
	s.logger.Info("Processing subscription expiry notifications")

	// Process pre-expiry notifications
	if err := s.processPreExpiryNotifications(ctx, config); err != nil {
		s.logger.Error("Failed to process pre-expiry notifications", zap.Error(err))
		return err
	}

	// Process post-expiry notifications
	if err := s.processPostExpiryNotifications(ctx, config); err != nil {
		s.logger.Error("Failed to process post-expiry notifications", zap.Error(err))
		return err
	}

	return nil
}

// processPreExpiryNotifications sends notifications before subscription expiry
func (s *NotificationService) processPreExpiryNotifications(ctx context.Context, config *NotificationConfig) error {
	for _, days := range config.PreExpiryDays {
		subs, err := s.getSubscriptionsExpiringInDays(ctx, days)
		if err != nil {
			s.logger.Error("Failed to get subscriptions expiring in days",
				zap.Int("days", days),
				zap.Error(err))
			continue
		}

		s.logger.Info("Found subscriptions expiring soon",
			zap.Int("days", days),
			zap.Int("count", len(subs)))

		for _, sub := range subs {
			if err := s.sendPreExpiryNotification(ctx, sub, days); err != nil {
				s.logger.Error("Failed to send pre-expiry notification",
					zap.String("subscription_id", sub.ID.String()),
					zap.Int("days_until_expiry", days),
					zap.Error(err))
			}
		}
	}

	return nil
}

// processPostExpiryNotifications sends notifications after subscription expiry
func (s *NotificationService) processPostExpiryNotifications(ctx context.Context, config *NotificationConfig) error {
	for _, days := range config.PostExpiryDays {
		subs, err := s.getSubscriptionsExpiredForDays(ctx, days)
		if err != nil {
			s.logger.Error("Failed to get subscriptions expired for days",
				zap.Int("days", days),
				zap.Error(err))
			continue
		}

		s.logger.Info("Found expired subscriptions",
			zap.Int("days_expired", days),
			zap.Int("count", len(subs)))

		for _, sub := range subs {
			if err := s.sendPostExpiryNotification(ctx, sub, days); err != nil {
				s.logger.Error("Failed to send post-expiry notification",
					zap.String("subscription_id", sub.ID.String()),
					zap.Int("days_expired", days),
					zap.Error(err))
			}
		}
	}

	return nil
}

// getSubscriptionsExpiringInDays retrieves subscriptions expiring in N days
func (s *NotificationService) getSubscriptionsExpiringInDays(ctx context.Context, days int) ([]*Subscription, error) {
	query := `
		SELECT s.id, s.user_id, s.plan_id, s.status, s.current_period_start, s.current_period_end,
			s.cancel_at_period_end, s.canceled_at, s.trial_start, s.trial_end,
			s.stripe_customer_id, s.stripe_subscription_id, s.metadata, s.created_at, s.updated_at
		FROM subscriptions s
		WHERE s.status IN ('active', 'trial')
			AND s.current_period_end BETWEEN $1 AND $2
			AND NOT EXISTS (
				SELECT 1 FROM subscription_notifications sn
				WHERE sn.subscription_id = s.id
					AND sn.notification_type = 'pre_expiry'
					AND sn.days_before_expiry = $3
					AND sn.sent_at > CURRENT_TIMESTAMP - INTERVAL '1 day'
			)
	`

	// Calculate time range
	start := time.Now().Add(time.Duration(days) * 24 * time.Hour)
	end := start.Add(24 * time.Hour)

	var subs []*Subscription
	err := s.db.SelectContext(ctx, &subs, query, start, end, days)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscriptions expiring in %d days: %w", days, err)
	}

	return subs, nil
}

// getSubscriptionsExpiredForDays retrieves subscriptions expired for N days
func (s *NotificationService) getSubscriptionsExpiredForDays(ctx context.Context, days int) ([]*Subscription, error) {
	query := `
		SELECT s.id, s.user_id, s.plan_id, s.status, s.current_period_start, s.current_period_end,
			s.cancel_at_period_end, s.canceled_at, s.trial_start, s.trial_end,
			s.stripe_customer_id, s.stripe_subscription_id, s.metadata, s.created_at, s.updated_at
		FROM subscriptions s
		WHERE s.status = 'expired'
			AND s.current_period_end BETWEEN $1 AND $2
			AND NOT EXISTS (
				SELECT 1 FROM subscription_notifications sn
				WHERE sn.subscription_id = s.id
					AND sn.notification_type = 'post_expiry'
					AND sn.days_after_expiry = $3
					AND sn.sent_at > CURRENT_TIMESTAMP - INTERVAL '1 day'
			)
	`

	// Calculate time range (days ago)
	end := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	start := end.Add(-24 * time.Hour)

	var subs []*Subscription
	err := s.db.SelectContext(ctx, &subs, query, start, end, days)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscriptions expired for %d days: %w", days, err)
	}

	return subs, nil
}

// sendPreExpiryNotification sends a notification before subscription expiry
func (s *NotificationService) sendPreExpiryNotification(ctx context.Context, sub *Subscription, daysUntilExpiry int) error {
	title := fmt.Sprintf("Your subscription expires in %d day(s)", daysUntilExpiry)
	body := s.getPreExpiryMessage(sub, daysUntilExpiry)

	notif := &notification.Notification{
		ID:           uuid.New(),
		UserID:       sub.UserID,
		Type:         notification.NotificationTypeSubscription,
		Title:        title,
		Body:         body,
		Priority:     s.getNotificationPriority(daysUntilExpiry),
		Status:       notification.StatusPending,
		ScheduledFor: time.Now(),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		MaxRetries:   3,
		Data: map[string]interface{}{
			"subscription_id":   sub.ID.String(),
			"plan_id":           sub.PlanID,
			"expiry_date":       sub.CurrentPeriodEnd.Format(time.RFC3339),
			"days_until_expiry": daysUntilExpiry,
			"notification_type": "pre_expiry",
		},
	}

	if err := s.notificationQueue.SendNotification(ctx, notif); err != nil {
		return fmt.Errorf("failed to send notification: %w", err)
	}

	// Record notification sent
	if err := s.recordNotificationSent(ctx, sub.ID, "pre_expiry", daysUntilExpiry, 0); err != nil {
		s.logger.Warn("Failed to record notification",
			zap.String("subscription_id", sub.ID.String()),
			zap.Error(err))
	}

	s.logger.Info("Pre-expiry notification sent",
		zap.String("subscription_id", sub.ID.String()),
		zap.String("user_id", sub.UserID.String()),
		zap.Int("days_until_expiry", daysUntilExpiry))

	return nil
}

// sendPostExpiryNotification sends a notification after subscription expiry
func (s *NotificationService) sendPostExpiryNotification(ctx context.Context, sub *Subscription, daysExpired int) error {
	title := "Your subscription has expired"
	body := s.getPostExpiryMessage(sub, daysExpired)

	notif := &notification.Notification{
		ID:           uuid.New(),
		UserID:       sub.UserID,
		Type:         notification.NotificationTypeSubscription,
		Title:        title,
		Body:         body,
		Priority:     notification.PriorityNormal,
		Status:       notification.StatusPending,
		ScheduledFor: time.Now(),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		MaxRetries:   3,
		Data: map[string]interface{}{
			"subscription_id":   sub.ID.String(),
			"plan_id":           sub.PlanID,
			"expiry_date":       sub.CurrentPeriodEnd.Format(time.RFC3339),
			"days_expired":      daysExpired,
			"notification_type": "post_expiry",
		},
	}

	if err := s.notificationQueue.SendNotification(ctx, notif); err != nil {
		return fmt.Errorf("failed to send notification: %w", err)
	}

	// Record notification sent
	if err := s.recordNotificationSent(ctx, sub.ID, "post_expiry", 0, daysExpired); err != nil {
		s.logger.Warn("Failed to record notification",
			zap.String("subscription_id", sub.ID.String()),
			zap.Error(err))
	}

	s.logger.Info("Post-expiry notification sent",
		zap.String("subscription_id", sub.ID.String()),
		zap.String("user_id", sub.UserID.String()),
		zap.Int("days_expired", daysExpired))

	return nil
}

// getPreExpiryMessage generates the message for pre-expiry notifications
func (s *NotificationService) getPreExpiryMessage(sub *Subscription, days int) string {
	plan, _ := subscription.GetPlan(subscription.PlanID(sub.PlanID))
	planName := "Premium"
	if plan != nil {
		planName = plan.Name
	}

	switch days {
	case 1:
		return fmt.Sprintf("Your %s subscription expires tomorrow. Renew now to keep your premium features!", planName)
	case 3:
		return fmt.Sprintf("Your %s subscription expires in 3 days. Don't lose access to your premium features.", planName)
	case 7:
		return fmt.Sprintf("Your %s subscription expires in 7 days. Renew early and never miss a beat!", planName)
	default:
		return fmt.Sprintf("Your %s subscription expires in %d days. Renew to continue enjoying premium features.", planName, days)
	}
}

// getPostExpiryMessage generates the message for post-expiry notifications
func (s *NotificationService) getPostExpiryMessage(sub *Subscription, days int) string {
	plan, _ := subscription.GetPlan(subscription.PlanID(sub.PlanID))
	planName := "Premium"
	if plan != nil {
		planName = plan.Name
	}

	switch days {
	case 1:
		return fmt.Sprintf("Your %s subscription expired yesterday. Resubscribe now to regain access to premium features!", planName)
	case 3:
		return fmt.Sprintf("Your %s subscription expired 3 days ago. We miss you! Resubscribe to continue where you left off.", planName)
	case 7:
		return fmt.Sprintf("Your %s subscription expired a week ago. Come back and unlock premium features again!", planName)
	default:
		return fmt.Sprintf("Your %s subscription expired %d days ago. Resubscribe anytime to restore premium access.", planName, days)
	}
}

// getNotificationPriority returns the priority based on days until expiry
func (s *NotificationService) getNotificationPriority(daysUntilExpiry int) notification.Priority {
	switch {
	case daysUntilExpiry <= 1:
		return notification.PriorityHigh
	case daysUntilExpiry <= 3:
		return notification.PriorityNormal
	default:
		return notification.PriorityLow
	}
}

// recordNotificationSent records that a notification was sent
func (s *NotificationService) recordNotificationSent(ctx context.Context, subscriptionID uuid.UUID, notificationType string, daysBefore, daysAfter int) error {
	query := `
		INSERT INTO subscription_notifications (
			id, subscription_id, notification_type, days_before_expiry, days_after_expiry, sent_at
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (subscription_id, notification_type, days_before_expiry, days_after_expiry)
		DO UPDATE SET sent_at = $6
	`

	_, err := s.db.ExecContext(ctx, query,
		uuid.New(), subscriptionID, notificationType, daysBefore, daysAfter, time.Now())
	return err
}
