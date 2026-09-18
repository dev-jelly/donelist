package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
	"github.com/dev-jelly/donelist/internal/checkin"
)

// IntervalChecker checks user checkin intervals and triggers notifications
type IntervalChecker struct {
	db           *sqlx.DB
	checkinRepo  *checkin.Repository
	service      *Service
	logger       *zap.Logger
}

// NewIntervalChecker creates a new interval checker
func NewIntervalChecker(db *sqlx.DB, checkinRepo *checkin.Repository, service *Service, logger *zap.Logger) *IntervalChecker {
	return &IntervalChecker{
		db:          db,
		checkinRepo: checkinRepo,
		service:     service,
		logger:      logger,
	}
}

// IntervalRule defines when to send notifications based on time since last checkin
type IntervalRule struct {
	Duration time.Duration
	Message  string
}

// GetIntervalRules returns the notification rules for different intervals
func GetIntervalRules() []IntervalRule {
	return []IntervalRule{
		{Duration: 15 * time.Minute, Message: "15 minutes since your last check-in!"},
		{Duration: 30 * time.Minute, Message: "30 minutes have passed. Time to check in?"},
		{Duration: 45 * time.Minute, Message: "45 minutes since your last activity. What are you working on?"},
		{Duration: 2 * time.Hour, Message: "2 hours since your last check-in. Don't forget to track your progress!"},
	}
}

// CheckUserInterval checks a single user's checkin interval and sends notifications if needed
func (ic *IntervalChecker) CheckUserInterval(ctx context.Context, userID uuid.UUID) error {
	// Get user's last checkin
	lastCheckin, err := ic.checkinRepo.GetLastCheckin(ctx, userID)
	if err != nil {
		// If no checkin found, skip this user
		if err.Error() == "no checkin found" {
			return nil
		}
		return fmt.Errorf("failed to get last checkin for user %s: %w", userID, err)
	}

	// Skip if last checkin is nil
	if lastCheckin == nil {
		return nil
	}

	// Calculate time since last checkin
	timeSinceLastCheckin := time.Since(lastCheckin.CheckinTime)

	// Get user's notification settings
	settings, err := ic.service.GetUserNotificationSettings(ctx, userID)
	if err != nil {
		ic.logger.Warn("Failed to get user notification settings, using defaults",
			zap.String("user_id", userID.String()),
			zap.Error(err),
		)
		// Continue with default behavior if settings not found
		settings = &NotificationSettings{
			UserID:           userID,
			CheckinReminders: true, // Default to enabled
		}
	}

	// Skip if checkin reminders are disabled
	if !settings.CheckinReminders {
		return nil
	}

	// Check interval rules
	for _, rule := range GetIntervalRules() {
		// Check if we should send notification for this interval
		if ic.shouldSendIntervalNotification(timeSinceLastCheckin, rule.Duration) {
			// Create notification
			notification := &Notification{
				ID:           uuid.New(),
				UserID:       userID,
				Type:         NotificationTypeReminder,
				Title:        "Time to Check In",
				Body:         rule.Message,
				Priority:     PriorityNormal,
				Status:       StatusPending,
				ScheduledFor: time.Now(),
				CreatedAt:    time.Now(),
				UpdatedAt:    time.Now(),
				MaxRetries:   3,
				Data: map[string]interface{}{
					"interval_minutes":      int(rule.Duration.Minutes()),
					"last_checkin_time":     lastCheckin.CheckinTime.Format(time.RFC3339),
					"time_since_checkin":    timeSinceLastCheckin.String(),
				},
			}

			// Send notification
			if err := ic.service.SendNotification(ctx, notification); err != nil {
				ic.logger.Error("Failed to send interval notification",
					zap.String("user_id", userID.String()),
					zap.Duration("interval", rule.Duration),
					zap.Error(err),
				)
				// Continue with other rules even if one fails
				continue
			}

			ic.logger.Info("Sent interval notification",
				zap.String("user_id", userID.String()),
				zap.Duration("interval", rule.Duration),
				zap.Time("last_checkin", lastCheckin.CheckinTime),
			)

			// Only send one notification per check
			break
		}
	}

	return nil
}

// shouldSendIntervalNotification determines if a notification should be sent for a given interval
func (ic *IntervalChecker) shouldSendIntervalNotification(timeSinceLastCheckin, targetInterval time.Duration) bool {
	// We want to send notification when user has been idle for exactly the target interval
	// Allow a 5-minute window for processing delays
	const tolerance = 5 * time.Minute

	// Check if time since last checkin is within the target interval window
	lowerBound := targetInterval - tolerance
	upperBound := targetInterval + tolerance

	return timeSinceLastCheckin >= lowerBound && timeSinceLastCheckin <= upperBound
}

// ProcessIntervalChecks processes interval checks for all active users
func (ic *IntervalChecker) ProcessIntervalChecks(ctx context.Context) error {
	// Get users who have checked in recently (within last 24 hours)
	query := `
		SELECT DISTINCT user_id
		FROM checkins
		WHERE checkin_time > NOW() - INTERVAL '24 hours'
		  AND deleted_at IS NULL
	`

	var userIDs []uuid.UUID
	err := ic.db.SelectContext(ctx, &userIDs, query)
	if err != nil {
		return fmt.Errorf("failed to get active users: %w", err)
	}

	ic.logger.Info("Processing interval checks",
		zap.Int("user_count", len(userIDs)),
	)

	// Process each user
	errCount := 0
	for _, userID := range userIDs {
		if err := ic.CheckUserInterval(ctx, userID); err != nil {
			ic.logger.Error("Failed to check user interval",
				zap.String("user_id", userID.String()),
				zap.Error(err),
			)
			errCount++
			// Continue processing other users
		}
	}

	if errCount > 0 {
		return fmt.Errorf("failed to process %d users", errCount)
	}

	return nil
}

// StartIntervalCheckLoop starts the background loop for interval checking
func (ic *IntervalChecker) StartIntervalCheckLoop(ctx context.Context) {
	// Check every 5 minutes
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	ic.logger.Info("Starting interval check loop")

	// Run initial check
	if err := ic.ProcessIntervalChecks(ctx); err != nil {
		ic.logger.Error("Initial interval check failed", zap.Error(err))
	}

	for {
		select {
		case <-ctx.Done():
			ic.logger.Info("Interval check loop stopped")
			return
		case <-ticker.C:
			if err := ic.ProcessIntervalChecks(ctx); err != nil {
				ic.logger.Error("Interval check failed", zap.Error(err))
			}
		}
	}
}