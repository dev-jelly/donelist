package profile

import (
	"time"

	"github.com/google/uuid"
)

// Profile represents a user profile
type Profile struct {
	ID                  uuid.UUID  `db:"id" json:"id"`
	UserID              uuid.UUID  `db:"user_id" json:"user_id"`
	Bio                 *string    `db:"bio" json:"bio,omitempty"`
	AvatarURL           *string    `db:"avatar_url" json:"avatar_url,omitempty"`
	Timezone            string     `db:"timezone" json:"timezone"`
	TimezoneAutoDetected bool      `db:"timezone_auto_detected" json:"timezone_auto_detected"`
	Locale              string     `db:"locale" json:"locale"`
	CreatedAt           time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt           time.Time  `db:"updated_at" json:"updated_at"`
	Version             int        `db:"version" json:"version"`
}

// Settings represents user UI/UX settings
type Settings struct {
	ID           uuid.UUID `db:"id" json:"id"`
	UserID       uuid.UUID `db:"user_id" json:"user_id"`
	Theme        string    `db:"theme" json:"theme"`
	Language     string    `db:"language" json:"language"`
	DateFormat   string    `db:"date_format" json:"date_format"`
	TimeFormat   string    `db:"time_format" json:"time_format"`
	WeekStartDay int       `db:"week_start_day" json:"week_start_day"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time `db:"updated_at" json:"updated_at"`
	Version      int       `db:"version" json:"version"`
}

// NotificationSettings represents notification preferences
type NotificationSettings struct {
	ID                     uuid.UUID  `db:"id" json:"id"`
	UserID                 uuid.UUID  `db:"user_id" json:"user_id"`
	EmailNotifications     bool       `db:"email_notifications" json:"email_notifications"`
	PushNotifications      bool       `db:"push_notifications" json:"push_notifications"`
	CheckinReminders       bool       `db:"checkin_reminders" json:"checkin_reminders"`
	ReminderIntervalMinutes int       `db:"reminder_interval_minutes" json:"reminder_interval_minutes"`
	DNDEnabled             bool       `db:"dnd_enabled" json:"dnd_enabled"`
	DNDStartTime           *time.Time `db:"dnd_start_time" json:"dnd_start_time,omitempty"`
	DNDEndTime             *time.Time `db:"dnd_end_time" json:"dnd_end_time,omitempty"`
	DNDDays                []int      `db:"dnd_days" json:"dnd_days"`
	CreatedAt              time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt              time.Time  `db:"updated_at" json:"updated_at"`
	Version                int        `db:"version" json:"version"`
}

// DataRetentionSettings represents data retention policies
type DataRetentionSettings struct {
	ID                        uuid.UUID  `db:"id" json:"id"`
	UserID                    uuid.UUID  `db:"user_id" json:"user_id"`
	AutoDeleteEnabled         bool       `db:"auto_delete_enabled" json:"auto_delete_enabled"`
	RetentionDays             *int       `db:"retention_days" json:"retention_days,omitempty"`
	DeleteAfterInactivityDays *int       `db:"delete_after_inactivity_days" json:"delete_after_inactivity_days,omitempty"`
	LastActivityAt            time.Time  `db:"last_activity_at" json:"last_activity_at"`
	CreatedAt                 time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt                 time.Time  `db:"updated_at" json:"updated_at"`
	Version                   int        `db:"version" json:"version"`
}

// CreateProfileInput represents input for creating a profile
type CreateProfileInput struct {
	UserID   uuid.UUID
	Bio      *string
	AvatarURL *string
	Timezone string
	Locale   string
}

// UpdateProfileInput represents input for updating a profile
type UpdateProfileInput struct {
	Bio                  *string
	AvatarURL            *string
	Timezone             *string
	TimezoneAutoDetected *bool
	Locale               *string
	Version              int // For optimistic locking
}

// UpdateSettingsInput represents input for updating settings
type UpdateSettingsInput struct {
	Theme        *string
	Language     *string
	DateFormat   *string
	TimeFormat   *string
	WeekStartDay *int
	Version      int // For optimistic locking
}

// UpdateNotificationSettingsInput represents input for updating notification settings
type UpdateNotificationSettingsInput struct {
	EmailNotifications     *bool
	PushNotifications      *bool
	CheckinReminders       *bool
	ReminderIntervalMinutes *int
	DNDEnabled             *bool
	DNDStartTime           *time.Time
	DNDEndTime             *time.Time
	DNDDays                *[]int
	Version                int // For optimistic locking
}

// UpdateDataRetentionSettingsInput represents input for updating data retention settings
type UpdateDataRetentionSettingsInput struct {
	AutoDeleteEnabled         *bool
	RetentionDays             *int
	DeleteAfterInactivityDays *int
	Version                   int // For optimistic locking
}

// ProfileWithSettings combines profile with all settings
type ProfileWithSettings struct {
	Profile              *Profile              `json:"profile"`
	Settings             *Settings             `json:"settings"`
	NotificationSettings *NotificationSettings `json:"notification_settings"`
	DataRetentionSettings *DataRetentionSettings `json:"data_retention_settings"`
}
