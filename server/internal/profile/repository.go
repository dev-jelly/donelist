package profile

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// Repository handles profile database operations
type Repository struct {
	db *sqlx.DB
}

// NewRepository creates a new profile repository
func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

// GetProfileByUserID retrieves a profile by user ID
func (r *Repository) GetProfileByUserID(ctx context.Context, userID uuid.UUID) (*Profile, error) {
	query := `
		SELECT id, user_id, bio, avatar_url, timezone, timezone_auto_detected, locale, created_at, updated_at, version
		FROM user_profiles
		WHERE user_id = $1
	`

	var profile Profile
	err := r.db.GetContext(ctx, &profile, query, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("profile not found")
		}
		return nil, fmt.Errorf("failed to get profile: %w", err)
	}

	return &profile, nil
}

// UpdateProfile updates a user profile with optimistic locking
func (r *Repository) UpdateProfile(ctx context.Context, userID uuid.UUID, input UpdateProfileInput) (*Profile, error) {
	query := `
		UPDATE user_profiles
		SET bio = COALESCE($2, bio),
		    avatar_url = COALESCE($3, avatar_url),
		    timezone = COALESCE($4, timezone),
		    timezone_auto_detected = COALESCE($5, timezone_auto_detected),
		    locale = COALESCE($6, locale),
		    updated_at = CURRENT_TIMESTAMP
		WHERE user_id = $1 AND version = $7
		RETURNING id, user_id, bio, avatar_url, timezone, timezone_auto_detected, locale, created_at, updated_at, version
	`

	var profile Profile
	err := r.db.GetContext(ctx, &profile, query,
		userID,
		input.Bio,
		input.AvatarURL,
		input.Timezone,
		input.TimezoneAutoDetected,
		input.Locale,
		input.Version,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("profile not found or version mismatch (concurrent update detected)")
		}
		return nil, fmt.Errorf("failed to update profile: %w", err)
	}

	return &profile, nil
}

// GetSettingsByUserID retrieves settings by user ID
func (r *Repository) GetSettingsByUserID(ctx context.Context, userID uuid.UUID) (*Settings, error) {
	query := `
		SELECT id, user_id, theme, language, date_format, time_format, week_start_day, created_at, updated_at, version
		FROM user_settings
		WHERE user_id = $1
	`

	var settings Settings
	err := r.db.GetContext(ctx, &settings, query, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("settings not found")
		}
		return nil, fmt.Errorf("failed to get settings: %w", err)
	}

	return &settings, nil
}

// UpdateSettings updates user settings with optimistic locking
func (r *Repository) UpdateSettings(ctx context.Context, userID uuid.UUID, input UpdateSettingsInput) (*Settings, error) {
	query := `
		UPDATE user_settings
		SET theme = COALESCE($2, theme),
		    language = COALESCE($3, language),
		    date_format = COALESCE($4, date_format),
		    time_format = COALESCE($5, time_format),
		    week_start_day = COALESCE($6, week_start_day),
		    updated_at = CURRENT_TIMESTAMP
		WHERE user_id = $1 AND version = $7
		RETURNING id, user_id, theme, language, date_format, time_format, week_start_day, created_at, updated_at, version
	`

	var settings Settings
	err := r.db.GetContext(ctx, &settings, query,
		userID,
		input.Theme,
		input.Language,
		input.DateFormat,
		input.TimeFormat,
		input.WeekStartDay,
		input.Version,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("settings not found or version mismatch (concurrent update detected)")
		}
		return nil, fmt.Errorf("failed to update settings: %w", err)
	}

	return &settings, nil
}

// GetNotificationSettingsByUserID retrieves notification settings by user ID
func (r *Repository) GetNotificationSettingsByUserID(ctx context.Context, userID uuid.UUID) (*NotificationSettings, error) {
	query := `
		SELECT id, user_id, email_notifications, push_notifications, checkin_reminders,
		       reminder_interval_minutes, dnd_enabled, dnd_start_time, dnd_end_time,
		       dnd_days, created_at, updated_at, version
		FROM notification_settings
		WHERE user_id = $1
	`

	var settings NotificationSettings
	err := r.db.GetContext(ctx, &settings, query, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("notification settings not found")
		}
		return nil, fmt.Errorf("failed to get notification settings: %w", err)
	}

	return &settings, nil
}

// UpdateNotificationSettings updates notification settings with optimistic locking
func (r *Repository) UpdateNotificationSettings(ctx context.Context, userID uuid.UUID, input UpdateNotificationSettingsInput) (*NotificationSettings, error) {
	query := `
		UPDATE notification_settings
		SET email_notifications = COALESCE($2, email_notifications),
		    push_notifications = COALESCE($3, push_notifications),
		    checkin_reminders = COALESCE($4, checkin_reminders),
		    reminder_interval_minutes = COALESCE($5, reminder_interval_minutes),
		    dnd_enabled = COALESCE($6, dnd_enabled),
		    dnd_start_time = COALESCE($7, dnd_start_time),
		    dnd_end_time = COALESCE($8, dnd_end_time),
		    dnd_days = COALESCE($9, dnd_days),
		    updated_at = CURRENT_TIMESTAMP
		WHERE user_id = $1 AND version = $10
		RETURNING id, user_id, email_notifications, push_notifications, checkin_reminders,
		          reminder_interval_minutes, dnd_enabled, dnd_start_time, dnd_end_time,
		          dnd_days, created_at, updated_at, version
	`

	var settings NotificationSettings
	err := r.db.GetContext(ctx, &settings, query,
		userID,
		input.EmailNotifications,
		input.PushNotifications,
		input.CheckinReminders,
		input.ReminderIntervalMinutes,
		input.DNDEnabled,
		input.DNDStartTime,
		input.DNDEndTime,
		input.DNDDays,
		input.Version,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("notification settings not found or version mismatch (concurrent update detected)")
		}
		return nil, fmt.Errorf("failed to update notification settings: %w", err)
	}

	return &settings, nil
}

// GetDataRetentionSettingsByUserID retrieves data retention settings by user ID
func (r *Repository) GetDataRetentionSettingsByUserID(ctx context.Context, userID uuid.UUID) (*DataRetentionSettings, error) {
	query := `
		SELECT id, user_id, auto_delete_enabled, retention_days, delete_after_inactivity_days,
		       last_activity_at, created_at, updated_at, version
		FROM data_retention_settings
		WHERE user_id = $1
	`

	var settings DataRetentionSettings
	err := r.db.GetContext(ctx, &settings, query, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("data retention settings not found")
		}
		return nil, fmt.Errorf("failed to get data retention settings: %w", err)
	}

	return &settings, nil
}

// UpdateDataRetentionSettings updates data retention settings with optimistic locking
func (r *Repository) UpdateDataRetentionSettings(ctx context.Context, userID uuid.UUID, input UpdateDataRetentionSettingsInput) (*DataRetentionSettings, error) {
	query := `
		UPDATE data_retention_settings
		SET auto_delete_enabled = COALESCE($2, auto_delete_enabled),
		    retention_days = COALESCE($3, retention_days),
		    delete_after_inactivity_days = COALESCE($4, delete_after_inactivity_days),
		    updated_at = CURRENT_TIMESTAMP
		WHERE user_id = $1 AND version = $5
		RETURNING id, user_id, auto_delete_enabled, retention_days, delete_after_inactivity_days,
		          last_activity_at, created_at, updated_at, version
	`

	var settings DataRetentionSettings
	err := r.db.GetContext(ctx, &settings, query,
		userID,
		input.AutoDeleteEnabled,
		input.RetentionDays,
		input.DeleteAfterInactivityDays,
		input.Version,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("data retention settings not found or version mismatch (concurrent update detected)")
		}
		return nil, fmt.Errorf("failed to update data retention settings: %w", err)
	}

	return &settings, nil
}

// GetAllByUserID retrieves all profile and settings for a user
func (r *Repository) GetAllByUserID(ctx context.Context, userID uuid.UUID) (*ProfileWithSettings, error) {
	profile, err := r.GetProfileByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	settings, err := r.GetSettingsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	notificationSettings, err := r.GetNotificationSettingsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	dataRetentionSettings, err := r.GetDataRetentionSettingsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &ProfileWithSettings{
		Profile:              profile,
		Settings:             settings,
		NotificationSettings: notificationSettings,
		DataRetentionSettings: dataRetentionSettings,
	}, nil
}
