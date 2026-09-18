package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// TimezoneIntegrationExample demonstrates how to use timezone handling with notifications
type TimezoneIntegrationExample struct {
	tzHandler       *TimezoneHandler
	tzManager       *TimezoneManager
	dndScheduler    *DNDScheduler
	dndOverride     *DNDOverrideService
	settingsEngine  *SettingsEngine
	logger          *zap.Logger
}

// NewTimezoneIntegrationExample creates a new integration example
func NewTimezoneIntegrationExample(
	tzHandler *TimezoneHandler,
	tzManager *TimezoneManager,
	dndScheduler *DNDScheduler,
	dndOverride *DNDOverrideService,
	settingsEngine *SettingsEngine,
	logger *zap.Logger,
) *TimezoneIntegrationExample {
	return &TimezoneIntegrationExample{
		tzHandler:      tzHandler,
		tzManager:      tzManager,
		dndScheduler:   dndScheduler,
		dndOverride:    dndOverride,
		settingsEngine: settingsEngine,
		logger:         logger,
	}
}

// Example1_ScheduleNotificationInUserTimezone demonstrates scheduling a notification
// at a specific local time (e.g., 9 AM in user's timezone)
func (e *TimezoneIntegrationExample) Example1_ScheduleNotificationInUserTimezone(
	ctx context.Context,
	userID uuid.UUID,
	localHour, localMinute int,
) (*Notification, error) {
	// Get user settings
	settings, err := e.settingsEngine.settingsProvider.GetUserNotificationSettings(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user settings: %w", err)
	}

	// Convert local time to UTC
	referenceDate := time.Now()
	scheduledUTC, err := e.tzHandler.ConvertScheduleTime(localHour, localMinute, settings.Timezone, referenceDate)
	if err != nil {
		return nil, fmt.Errorf("failed to convert schedule time: %w", err)
	}

	// Check if the time falls in DND period
	shouldSend, err := e.settingsEngine.ShouldSendNotification(ctx, userID, NotificationTypeReminder, scheduledUTC)
	if err != nil {
		return nil, fmt.Errorf("failed to check DND: %w", err)
	}

	if !shouldSend {
		// Reschedule to next available time
		scheduledUTC, err = e.settingsEngine.GetNextAvailableTime(ctx, userID, scheduledUTC)
		if err != nil {
			return nil, fmt.Errorf("failed to get next available time: %w", err)
		}
	}

	// Create notification
	notification := &Notification{
		ID:           uuid.New(),
		UserID:       userID,
		Type:         NotificationTypeReminder,
		Priority:     PriorityNormal,
		Status:       StatusPending,
		Title:        "Daily Check-in",
		Body:         "Time to update your progress!",
		ScheduledFor: scheduledUTC,
		CreatedAt:    time.Now(),
	}

	e.logger.Info("Scheduled notification in user timezone",
		zap.String("user_id", userID.String()),
		zap.Int("local_hour", localHour),
		zap.Int("local_minute", localMinute),
		zap.String("timezone", settings.Timezone),
		zap.Time("scheduled_utc", scheduledUTC),
	)

	return notification, nil
}

// Example2_HandleUserTravel demonstrates handling timezone changes when users travel
func (e *TimezoneIntegrationExample) Example2_HandleUserTravel(
	ctx context.Context,
	userID uuid.UUID,
	newTimezone string,
) error {
	// Get user settings
	settings, err := e.settingsEngine.settingsProvider.GetUserNotificationSettings(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user settings: %w", err)
	}

	// Create user preference object
	pref := &UserTimezonePreference{
		UserID:   userID.String(),
		Timezone: settings.Timezone,
	}

	// Update timezone and detect travel
	travelDetected, err := e.tzManager.UpdateUserTimezone(pref, newTimezone)
	if err != nil {
		return fmt.Errorf("failed to update timezone: %w", err)
	}

	if travelDetected {
		e.logger.Info("Travel detected, creating DND override",
			zap.String("user_id", userID.String()),
			zap.String("old_timezone", settings.Timezone),
			zap.String("new_timezone", newTimezone),
		)

		// Create a temporary DND override to allow adjustment period
		_, err := e.dndOverride.CreateTravelOverride(ctx, userID, newTimezone, 24*time.Hour)
		if err != nil {
			return fmt.Errorf("failed to create travel override: %w", err)
		}
	}

	// Update user settings
	settings.Timezone = newTimezone

	return nil
}

// Example3_SendUrgentNotificationDuringDND demonstrates sending urgent notifications
// that override DND settings
func (e *TimezoneIntegrationExample) Example3_SendUrgentNotificationDuringDND(
	ctx context.Context,
	userID uuid.UUID,
	title, body string,
) (bool, string, error) {
	// Get user settings
	settings, err := e.settingsEngine.settingsProvider.GetUserNotificationSettings(ctx, userID)
	if err != nil {
		return false, "", fmt.Errorf("failed to get user settings: %w", err)
	}

	// Create urgent notification
	notification := &Notification{
		ID:           uuid.New(),
		UserID:       userID,
		Type:         NotificationTypePush,
		Priority:     PriorityUrgent,
		Status:       StatusPending,
		Title:        title,
		Body:         body,
		ScheduledFor: time.Now(),
		CreatedAt:    time.Now(),
	}

	// Check if we should send during DND
	shouldSend, reason, err := e.dndOverride.ShouldSendDuringDND(ctx, notification, settings)
	if err != nil {
		return false, "", fmt.Errorf("failed to check DND override: %w", err)
	}

	e.logger.Info("Checked urgent notification against DND",
		zap.String("user_id", userID.String()),
		zap.Bool("should_send", shouldSend),
		zap.String("reason", reason),
	)

	return shouldSend, reason, nil
}

// Example4_HandleDSTTransition demonstrates handling DST transitions for recurring schedules
func (e *TimezoneIntegrationExample) Example4_HandleDSTTransition(
	ctx context.Context,
	userID uuid.UUID,
	recurringNotifications []*Notification,
) error {
	// Get user settings
	settings, err := e.settingsEngine.settingsProvider.GetUserNotificationSettings(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user settings: %w", err)
	}

	// Get timezone info
	tzInfo, err := e.tzHandler.GetTimezoneInfo(settings.Timezone, time.Now())
	if err != nil {
		return fmt.Errorf("failed to get timezone info: %w", err)
	}

	e.logger.Info("Timezone information",
		zap.String("timezone", tzInfo.IANA),
		zap.Bool("is_dst", tzInfo.IsDST),
		zap.Int("offset_seconds", tzInfo.Offset),
		zap.String("name", tzInfo.Name),
	)

	// Adjust all recurring notifications for DST
	for _, notification := range recurringNotifications {
		adjustedTime, err := e.tzHandler.AdjustForDSTTransition(notification.ScheduledFor, settings.Timezone)
		if err != nil {
			e.logger.Warn("Failed to adjust notification for DST",
				zap.String("notification_id", notification.ID.String()),
				zap.Error(err),
			)
			continue
		}

		if !adjustedTime.Equal(notification.ScheduledFor) {
			e.logger.Info("Adjusted notification for DST",
				zap.String("notification_id", notification.ID.String()),
				zap.Time("old_time", notification.ScheduledFor),
				zap.Time("new_time", adjustedTime),
			)
			notification.ScheduledFor = adjustedTime
		}
	}

	return nil
}

// Example5_GetUserDNDStatus demonstrates retrieving comprehensive DND status
func (e *TimezoneIntegrationExample) Example5_GetUserDNDStatus(
	ctx context.Context,
	userID uuid.UUID,
) (map[string]interface{}, error) {
	// Get user settings
	settings, err := e.settingsEngine.settingsProvider.GetUserNotificationSettings(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user settings: %w", err)
	}

	// Get DND status
	dndStatus := e.dndScheduler.GetDNDStatus(settings)

	// Get override status
	overrideStatus, err := e.dndOverride.GetOverrideStatus(ctx, userID)
	if err != nil {
		e.logger.Warn("Failed to get override status", zap.Error(err))
	}

	// Combine status information
	status := map[string]interface{}{
		"user_id":         userID.String(),
		"timezone":        settings.Timezone,
		"dnd_settings":    dndStatus,
		"dnd_overrides":   overrideStatus,
		"checked_at":      time.Now().Format(time.RFC3339),
	}

	return status, nil
}

// Example6_ScheduleQuietHoursNotifications demonstrates batching notifications
// to respect quiet hours
func (e *TimezoneIntegrationExample) Example6_ScheduleQuietHoursNotifications(
	ctx context.Context,
	notifications []*Notification,
) ([]*Notification, error) {
	// Apply settings to batch
	filtered, err := e.settingsEngine.ApplyBatchSettings(ctx, notifications)
	if err != nil {
		return nil, fmt.Errorf("failed to apply batch settings: %w", err)
	}

	e.logger.Info("Applied quiet hours to notification batch",
		zap.Int("original_count", len(notifications)),
		zap.Int("filtered_count", len(filtered)),
		zap.Int("deferred_count", len(notifications)-len(filtered)),
	)

	return filtered, nil
}

// Example7_CreateEmergencyOverride demonstrates creating an emergency override
// that bypasses DND for 24 hours
func (e *TimezoneIntegrationExample) Example7_CreateEmergencyOverride(
	ctx context.Context,
	userID uuid.UUID,
) (*DNDOverride, error) {
	// Create emergency override
	override, err := e.dndOverride.CreateEmergencyOverride(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to create emergency override: %w", err)
	}

	e.logger.Info("Created emergency DND override",
		zap.String("user_id", userID.String()),
		zap.String("override_id", override.ID.String()),
		zap.Time("start_time", override.StartTime),
		zap.Time("end_time", override.EndTime),
	)

	return override, nil
}

// Example8_ConvertNotificationTimeForDisplay demonstrates converting UTC times
// to user's local timezone for display
func (e *TimezoneIntegrationExample) Example8_ConvertNotificationTimeForDisplay(
	ctx context.Context,
	notification *Notification,
	targetTimezone string,
) (string, error) {
	// Format in user's timezone
	formatted, err := e.tzHandler.FormatInTimezone(
		notification.ScheduledFor,
		targetTimezone,
		"Monday, January 2, 2006 at 3:04 PM MST",
	)
	if err != nil {
		return "", fmt.Errorf("failed to format time: %w", err)
	}

	e.logger.Debug("Formatted notification time for display",
		zap.String("notification_id", notification.ID.String()),
		zap.Time("utc_time", notification.ScheduledFor),
		zap.String("formatted", formatted),
		zap.String("timezone", targetTimezone),
	)

	return formatted, nil
}

// Example9_GetCommonTimezones demonstrates retrieving a list of common timezones
// for user selection in UI
func (e *TimezoneIntegrationExample) Example9_GetCommonTimezones() []map[string]interface{} {
	timezones := e.tzHandler.GetCommonTimezones()

	result := make([]map[string]interface{}, len(timezones))
	now := time.Now()

	for i, tz := range timezones {
		info, err := e.tzHandler.GetTimezoneInfo(tz, now)
		if err != nil {
			continue
		}

		// Format offset as +/-HH:MM
		offsetHours := info.Offset / 3600
		offsetMinutes := (abs(info.Offset) % 3600) / 60
		offsetSign := "+"
		if offsetHours < 0 {
			offsetSign = "-"
			offsetHours = -offsetHours
		}

		result[i] = map[string]interface{}{
			"iana":   tz,
			"name":   info.Name,
			"offset": fmt.Sprintf("UTC%s%02d:%02d", offsetSign, offsetHours, offsetMinutes),
			"is_dst": info.IsDST,
		}
	}

	return result
}

// Example10_ScheduleDailyReminderWithDST demonstrates scheduling a daily reminder
// that maintains the same local time even across DST transitions
func (e *TimezoneIntegrationExample) Example10_ScheduleDailyReminderWithDST(
	ctx context.Context,
	userID uuid.UUID,
	localHour, localMinute int,
	daysAhead int,
) ([]*Notification, error) {
	// Get user settings
	settings, err := e.settingsEngine.settingsProvider.GetUserNotificationSettings(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user settings: %w", err)
	}

	notifications := make([]*Notification, 0, daysAhead)

	for i := 0; i < daysAhead; i++ {
		referenceDate := time.Now().AddDate(0, 0, i)

		// Calculate midnight in user's timezone
		midnight, err := e.tzHandler.CalculateLocalMidnight(referenceDate, settings.Timezone)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate midnight: %w", err)
		}

		// Add the desired local time
		scheduledUTC, err := e.tzHandler.ConvertScheduleTime(localHour, localMinute, settings.Timezone, midnight)
		if err != nil {
			return nil, fmt.Errorf("failed to convert schedule time: %w", err)
		}

		// Check DND and adjust if necessary
		shouldSend, err := e.settingsEngine.ShouldSendNotification(ctx, userID, NotificationTypeReminder, scheduledUTC)
		if err != nil {
			return nil, fmt.Errorf("failed to check DND: %w", err)
		}

		if !shouldSend {
			scheduledUTC, err = e.settingsEngine.GetNextAvailableTime(ctx, userID, scheduledUTC)
			if err != nil {
				return nil, fmt.Errorf("failed to get next available time: %w", err)
			}
		}

		notification := &Notification{
			ID:           uuid.New(),
			UserID:       userID,
			Type:         NotificationTypeReminder,
			Priority:     PriorityNormal,
			Status:       StatusPending,
			Title:        "Daily Check-in",
			Body:         "Don't forget to log your progress!",
			ScheduledFor: scheduledUTC,
			CreatedAt:    time.Now(),
		}

		notifications = append(notifications, notification)
	}

	e.logger.Info("Scheduled daily reminders with DST handling",
		zap.String("user_id", userID.String()),
		zap.Int("count", len(notifications)),
		zap.Int("local_hour", localHour),
		zap.Int("local_minute", localMinute),
		zap.String("timezone", settings.Timezone),
	)

	return notifications, nil
}
