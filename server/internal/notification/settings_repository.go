package notification

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// SettingsRepository implements the SettingsProvider interface using PostgreSQL
type SettingsRepository struct {
	db     *sqlx.DB
	logger *zap.Logger
}

// NewSettingsRepository creates a new settings repository
func NewSettingsRepository(db *sqlx.DB, logger *zap.Logger) *SettingsRepository {
	return &SettingsRepository{
		db:     db,
		logger: logger,
	}
}

// GetUserNotificationSettings retrieves notification settings for a user
func (r *SettingsRepository) GetUserNotificationSettings(ctx context.Context, userID uuid.UUID) (*NotificationSettings, error) {
	query := `
		SELECT
			ns.user_id,
			ns.email_notifications,
			ns.push_notifications,
			ns.checkin_reminders,
			ns.reminder_interval_minutes,
			ns.dnd_enabled,
			ns.dnd_start_time,
			ns.dnd_end_time,
			ns.dnd_days,
			COALESCE(p.timezone, 'UTC') as timezone
		FROM notification_settings ns
		LEFT JOIN profiles p ON p.user_id = ns.user_id
		WHERE ns.user_id = $1
	`

	var settings struct {
		UserID                  uuid.UUID      `db:"user_id"`
		EmailNotifications      bool           `db:"email_notifications"`
		PushNotifications       bool           `db:"push_notifications"`
		CheckinReminders        bool           `db:"checkin_reminders"`
		ReminderIntervalMinutes int            `db:"reminder_interval_minutes"`
		DNDEnabled              bool           `db:"dnd_enabled"`
		DNDStartTime            *time.Time     `db:"dnd_start_time"`
		DNDEndTime              *time.Time     `db:"dnd_end_time"`
		DNDDaysJSON             sql.NullString `db:"dnd_days"`
		Timezone                string         `db:"timezone"`
	}

	err := r.db.GetContext(ctx, &settings, query, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			// Return default settings if none exist
			return r.createDefaultSettings(userID), nil
		}
		return nil, fmt.Errorf("failed to get notification settings: %w", err)
	}

	// Parse DND days from JSON
	var dndDays []int
	if settings.DNDDaysJSON.Valid && settings.DNDDaysJSON.String != "" {
		if err := json.Unmarshal([]byte(settings.DNDDaysJSON.String), &dndDays); err != nil {
			r.logger.Warn("Failed to parse DND days JSON",
				zap.String("user_id", userID.String()),
				zap.Error(err),
			)
			dndDays = []int{}
		}
	}

	return &NotificationSettings{
		UserID:                  settings.UserID,
		EmailNotifications:      settings.EmailNotifications,
		PushNotifications:       settings.PushNotifications,
		CheckinReminders:        settings.CheckinReminders,
		ReminderIntervalMinutes: settings.ReminderIntervalMinutes,
		DNDEnabled:              settings.DNDEnabled,
		DNDStartTime:            settings.DNDStartTime,
		DNDEndTime:              settings.DNDEndTime,
		DNDDays:                 dndDays,
		Timezone:                settings.Timezone,
	}, nil
}

// GetUsersWithRemindersEnabled retrieves all users with reminders enabled
func (r *SettingsRepository) GetUsersWithRemindersEnabled(ctx context.Context) ([]uuid.UUID, error) {
	query := `
		SELECT DISTINCT ns.user_id
		FROM notification_settings ns
		INNER JOIN users u ON u.id = ns.user_id
		WHERE ns.checkin_reminders = true
			AND u.is_active = true
			AND (
				ns.last_reminder_sent IS NULL
				OR ns.last_reminder_sent < NOW() - INTERVAL '1 minute' * ns.reminder_interval_minutes
			)
		ORDER BY ns.user_id
	`

	var userIDs []uuid.UUID
	err := r.db.SelectContext(ctx, &userIDs, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get users with reminders enabled: %w", err)
	}

	return userIDs, nil
}

// UpdateLastReminderSent updates the last reminder sent timestamp for a user
func (r *SettingsRepository) UpdateLastReminderSent(ctx context.Context, userID uuid.UUID, timestamp time.Time) error {
	query := `
		UPDATE notification_settings
		SET last_reminder_sent = $2, updated_at = NOW()
		WHERE user_id = $1
	`

	result, err := r.db.ExecContext(ctx, query, userID, timestamp)
	if err != nil {
		return fmt.Errorf("failed to update last reminder sent: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("no notification settings found for user %s", userID)
	}

	return nil
}

// createDefaultSettings creates default notification settings
func (r *SettingsRepository) createDefaultSettings(userID uuid.UUID) *NotificationSettings {
	return &NotificationSettings{
		UserID:                  userID,
		EmailNotifications:      true,
		PushNotifications:       false,
		CheckinReminders:        true,
		ReminderIntervalMinutes: 180, // 3 hours default
		DNDEnabled:              false,
		DNDStartTime:            nil,
		DNDEndTime:              nil,
		DNDDays:                 []int{},
		Timezone:                "UTC",
	}
}

// CreateNotificationSettings creates new notification settings for a user
func (r *SettingsRepository) CreateNotificationSettings(ctx context.Context, settings *NotificationSettings) error {
	// Convert DND days to JSON
	dndDaysJSON, err := json.Marshal(settings.DNDDays)
	if err != nil {
		return fmt.Errorf("failed to marshal DND days: %w", err)
	}

	query := `
		INSERT INTO notification_settings (
			id, user_id, email_notifications, push_notifications,
			checkin_reminders, reminder_interval_minutes,
			dnd_enabled, dnd_start_time, dnd_end_time, dnd_days,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW()
		)
	`

	_, err = r.db.ExecContext(
		ctx, query,
		uuid.New(),
		settings.UserID,
		settings.EmailNotifications,
		settings.PushNotifications,
		settings.CheckinReminders,
		settings.ReminderIntervalMinutes,
		settings.DNDEnabled,
		settings.DNDStartTime,
		settings.DNDEndTime,
		string(dndDaysJSON),
	)

	if err != nil {
		return fmt.Errorf("failed to create notification settings: %w", err)
	}

	return nil
}

// UpdateNotificationSettings updates existing notification settings
func (r *SettingsRepository) UpdateNotificationSettings(ctx context.Context, settings *NotificationSettings) error {
	// Convert DND days to JSON
	dndDaysJSON, err := json.Marshal(settings.DNDDays)
	if err != nil {
		return fmt.Errorf("failed to marshal DND days: %w", err)
	}

	query := `
		UPDATE notification_settings
		SET
			email_notifications = $2,
			push_notifications = $3,
			checkin_reminders = $4,
			reminder_interval_minutes = $5,
			dnd_enabled = $6,
			dnd_start_time = $7,
			dnd_end_time = $8,
			dnd_days = $9,
			updated_at = NOW()
		WHERE user_id = $1
	`

	result, err := r.db.ExecContext(
		ctx, query,
		settings.UserID,
		settings.EmailNotifications,
		settings.PushNotifications,
		settings.CheckinReminders,
		settings.ReminderIntervalMinutes,
		settings.DNDEnabled,
		settings.DNDStartTime,
		settings.DNDEndTime,
		string(dndDaysJSON),
	)

	if err != nil {
		return fmt.Errorf("failed to update notification settings: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		// Create settings if they don't exist
		return r.CreateNotificationSettings(ctx, settings)
	}

	return nil
}

// GetUsersByDNDStatus retrieves users filtered by DND status
func (r *SettingsRepository) GetUsersByDNDStatus(ctx context.Context, dndActive bool) ([]uuid.UUID, error) {
	currentHour := time.Now().Hour()
	currentMinute := time.Now().Minute()
	currentDay := int(time.Now().Weekday())

	query := `
		SELECT DISTINCT ns.user_id
		FROM notification_settings ns
		INNER JOIN users u ON u.id = ns.user_id
		WHERE u.is_active = true
			AND ns.dnd_enabled = $1
	`

	if dndActive {
		// Add additional filtering for active DND periods
		query += fmt.Sprintf(`
			AND ns.dnd_start_time IS NOT NULL
			AND ns.dnd_end_time IS NOT NULL
			AND (
				-- Check if current day is in DND days
				ns.dnd_days::jsonb @> '[%d]'
				-- Check if current time is within DND period
				AND (
					(EXTRACT(HOUR FROM ns.dnd_start_time) * 60 + EXTRACT(MINUTE FROM ns.dnd_start_time) <= %d
					 AND EXTRACT(HOUR FROM ns.dnd_end_time) * 60 + EXTRACT(MINUTE FROM ns.dnd_end_time) > %d)
					OR
					-- Handle DND spanning midnight
					(EXTRACT(HOUR FROM ns.dnd_start_time) * 60 + EXTRACT(MINUTE FROM ns.dnd_start_time) >
					 EXTRACT(HOUR FROM ns.dnd_end_time) * 60 + EXTRACT(MINUTE FROM ns.dnd_end_time)
					 AND (EXTRACT(HOUR FROM ns.dnd_start_time) * 60 + EXTRACT(MINUTE FROM ns.dnd_start_time) <= %d
					      OR EXTRACT(HOUR FROM ns.dnd_end_time) * 60 + EXTRACT(MINUTE FROM ns.dnd_end_time) > %d))
				)
			)
		`, currentDay, currentHour*60+currentMinute, currentHour*60+currentMinute,
		   currentHour*60+currentMinute, currentHour*60+currentMinute)
	}

	var userIDs []uuid.UUID
	err := r.db.SelectContext(ctx, &userIDs, query, dndActive)
	if err != nil {
		return nil, fmt.Errorf("failed to get users by DND status: %w", err)
	}

	return userIDs, nil
}

// GetDNDOverrides retrieves active DND overrides for a user
func (r *SettingsRepository) GetDNDOverrides(ctx context.Context, userID uuid.UUID) ([]DNDOverride, error) {
	query := `
		SELECT id, user_id, start_time, end_time, reason, created_at
		FROM dnd_overrides
		WHERE user_id = $1
			AND start_time <= NOW()
			AND end_time > NOW()
		ORDER BY start_time
	`

	var overrides []DNDOverride
	err := r.db.SelectContext(ctx, &overrides, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get DND overrides: %w", err)
	}

	return overrides, nil
}

// CreateDNDOverride creates a new DND override
func (r *SettingsRepository) CreateDNDOverride(ctx context.Context, override *DNDOverride) error {
	query := `
		INSERT INTO dnd_overrides (id, user_id, start_time, end_time, reason, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
	`

	override.ID = uuid.New()
	_, err := r.db.ExecContext(
		ctx, query,
		override.ID,
		override.UserID,
		override.StartTime,
		override.EndTime,
		override.Reason,
	)

	if err != nil {
		return fmt.Errorf("failed to create DND override: %w", err)
	}

	return nil
}

// DeleteExpiredDNDOverrides removes expired DND overrides
func (r *SettingsRepository) DeleteExpiredDNDOverrides(ctx context.Context) (int64, error) {
	query := `
		DELETE FROM dnd_overrides
		WHERE end_time < NOW()
	`

	result, err := r.db.ExecContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired DND overrides: %w", err)
	}

	return result.RowsAffected()
}