package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// SettingsEngine manages the application of notification settings and DnD rules
type SettingsEngine struct {
	logger           *zap.Logger
	settingsProvider SettingsProvider
	dndScheduler     *DNDScheduler
}

// NotificationSettings represents a user's notification preferences
type NotificationSettings struct {
	UserID                  uuid.UUID
	EmailNotifications      bool
	PushNotifications       bool
	CheckinReminders        bool
	ReminderIntervalMinutes int
	DNDEnabled              bool
	DNDStartTime            *time.Time // Store as UTC time
	DNDEndTime              *time.Time // Store as UTC time
	DNDDays                 []int      // Days of week (0=Sunday, 6=Saturday)
	Timezone                string     // User's timezone
}

// SettingsProvider is an interface for retrieving user notification settings
type SettingsProvider interface {
	GetUserNotificationSettings(ctx context.Context, userID uuid.UUID) (*NotificationSettings, error)
	GetUsersWithRemindersEnabled(ctx context.Context) ([]uuid.UUID, error)
	UpdateLastReminderSent(ctx context.Context, userID uuid.UUID, timestamp time.Time) error
}

// NewSettingsEngine creates a new settings engine
func NewSettingsEngine(logger *zap.Logger, provider SettingsProvider) *SettingsEngine {
	return &SettingsEngine{
		logger:           logger,
		settingsProvider: provider,
		dndScheduler:     NewDNDScheduler(logger),
	}
}

// ShouldSendNotification determines if a notification should be sent based on user settings
func (s *SettingsEngine) ShouldSendNotification(ctx context.Context, userID uuid.UUID, notificationType NotificationType, scheduledTime time.Time) (bool, error) {
	settings, err := s.settingsProvider.GetUserNotificationSettings(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("failed to get user notification settings: %w", err)
	}

	// Check if notifications are enabled for this type
	if !s.isNotificationTypeEnabled(settings, notificationType) {
		s.logger.Debug("Notification type disabled for user",
			zap.String("user_id", userID.String()),
			zap.String("type", string(notificationType)),
		)
		return false, nil
	}

	// Check DnD status
	if settings.DNDEnabled {
		inDND, reason := s.dndScheduler.IsInDNDPeriod(settings, scheduledTime)
		if inDND {
			s.logger.Debug("User is in DnD period",
				zap.String("user_id", userID.String()),
				zap.String("reason", reason),
				zap.Time("scheduled_time", scheduledTime),
			)
			return false, nil
		}
	}

	return true, nil
}

// isNotificationTypeEnabled checks if a specific notification type is enabled
func (s *SettingsEngine) isNotificationTypeEnabled(settings *NotificationSettings, notificationType NotificationType) bool {
	switch notificationType {
	case NotificationTypeEmail:
		return settings.EmailNotifications
	case NotificationTypePush:
		return settings.PushNotifications
	case NotificationTypeReminder:
		return settings.CheckinReminders
	default:
		// Unknown types are disabled by default
		return false
	}
}

// GetNextAvailableTime calculates the next available time to send a notification outside DnD
func (s *SettingsEngine) GetNextAvailableTime(ctx context.Context, userID uuid.UUID, requestedTime time.Time) (time.Time, error) {
	settings, err := s.settingsProvider.GetUserNotificationSettings(ctx, userID)
	if err != nil {
		return requestedTime, fmt.Errorf("failed to get user notification settings: %w", err)
	}

	if !settings.DNDEnabled {
		return requestedTime, nil
	}

	return s.dndScheduler.GetNextAvailableTime(settings, requestedTime)
}

// ProcessReminderSchedule processes reminder schedules for all users with reminders enabled
func (s *SettingsEngine) ProcessReminderSchedule(ctx context.Context) error {
	users, err := s.settingsProvider.GetUsersWithRemindersEnabled(ctx)
	if err != nil {
		return fmt.Errorf("failed to get users with reminders enabled: %w", err)
	}

	s.logger.Info("Processing reminder schedules", zap.Int("user_count", len(users)))

	for _, userID := range users {
		if err := s.processUserReminder(ctx, userID); err != nil {
			s.logger.Error("Failed to process user reminder",
				zap.String("user_id", userID.String()),
				zap.Error(err),
			)
			// Continue processing other users
		}
	}

	return nil
}

// processUserReminder processes a single user's reminder schedule
func (s *SettingsEngine) processUserReminder(ctx context.Context, userID uuid.UUID) error {
	settings, err := s.settingsProvider.GetUserNotificationSettings(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user settings: %w", err)
	}

	if !settings.CheckinReminders {
		return nil
	}

	now := time.Now()
	shouldSend, err := s.ShouldSendNotification(ctx, userID, NotificationTypeReminder, now)
	if err != nil {
		return fmt.Errorf("failed to check notification eligibility: %w", err)
	}

	if shouldSend {
		// Create reminder notification
		notification := &Notification{
			ID:        uuid.New(),
			UserID:    userID,
			Type:      NotificationTypeReminder,
			Title:     "Check-in Reminder",
			Body:      "It's time to update your progress!",
			Priority:  PriorityNormal,
			CreatedAt: now,
		}

		// Queue the notification (implementation would go here)
		s.logger.Info("Queuing reminder notification",
			zap.String("user_id", userID.String()),
			zap.String("notification_id", notification.ID.String()),
		)

		// Update last reminder sent time
		if err := s.settingsProvider.UpdateLastReminderSent(ctx, userID, now); err != nil {
			return fmt.Errorf("failed to update last reminder time: %w", err)
		}
	}

	return nil
}

// ApplyBatchSettings applies notification settings to a batch of notifications
func (s *SettingsEngine) ApplyBatchSettings(ctx context.Context, notifications []*Notification) ([]*Notification, error) {
	filtered := make([]*Notification, 0, len(notifications))

	for _, notification := range notifications {
		shouldSend, err := s.ShouldSendNotification(ctx, notification.UserID, notification.Type, notification.ScheduledFor)
		if err != nil {
			s.logger.Warn("Failed to check notification settings",
				zap.String("notification_id", notification.ID.String()),
				zap.Error(err),
			)
			// Skip this notification on error
			continue
		}

		if shouldSend {
			// Check if we need to reschedule due to DnD
			availableTime, err := s.GetNextAvailableTime(ctx, notification.UserID, notification.ScheduledFor)
			if err != nil {
				s.logger.Warn("Failed to get next available time",
					zap.String("notification_id", notification.ID.String()),
					zap.Error(err),
				)
				// Keep original schedule on error
				filtered = append(filtered, notification)
				continue
			}

			if !availableTime.Equal(notification.ScheduledFor) {
				notification.ScheduledFor = availableTime
				s.logger.Debug("Rescheduled notification due to DnD",
					zap.String("notification_id", notification.ID.String()),
					zap.Time("new_time", availableTime),
				)
			}

			filtered = append(filtered, notification)
		} else {
			s.logger.Debug("Filtered out notification based on settings",
				zap.String("notification_id", notification.ID.String()),
				zap.String("user_id", notification.UserID.String()),
			)
		}
	}

	s.logger.Info("Applied batch settings",
		zap.Int("original_count", len(notifications)),
		zap.Int("filtered_count", len(filtered)),
	)

	return filtered, nil
}

// GetUserNotificationStatus returns the current notification status for a user
func (s *SettingsEngine) GetUserNotificationStatus(ctx context.Context, userID uuid.UUID) (map[string]interface{}, error) {
	settings, err := s.settingsProvider.GetUserNotificationSettings(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user settings: %w", err)
	}

	now := time.Now()
	inDND := false
	var dndReason string

	if settings.DNDEnabled {
		inDND, dndReason = s.dndScheduler.IsInDNDPeriod(settings, now)
	}

	status := map[string]interface{}{
		"user_id":             userID.String(),
		"email_enabled":       settings.EmailNotifications,
		"push_enabled":        settings.PushNotifications,
		"reminders_enabled":   settings.CheckinReminders,
		"reminder_interval":   settings.ReminderIntervalMinutes,
		"dnd_enabled":         settings.DNDEnabled,
		"currently_in_dnd":    inDND,
		"dnd_reason":          dndReason,
		"timezone":            settings.Timezone,
		"checked_at":          now.Format(time.RFC3339),
	}

	if settings.DNDEnabled && settings.DNDStartTime != nil && settings.DNDEndTime != nil {
		status["dnd_start"] = settings.DNDStartTime.Format("15:04")
		status["dnd_end"] = settings.DNDEndTime.Format("15:04")
		status["dnd_days"] = settings.DNDDays
	}

	return status, nil
}