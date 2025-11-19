package profile

import (
	"context"
	"fmt"
	"strings"

	"github.com/dev-jelly/donelist/internal/timezone"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Service handles profile business logic
type Service struct {
	repo   *Repository
	logger *zap.Logger
}

// NewService creates a new profile service
func NewService(repo *Repository, logger *zap.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
	}
}

// GetProfile retrieves a user's profile
func (s *Service) GetProfile(ctx context.Context, userID uuid.UUID) (*Profile, error) {
	profile, err := s.repo.GetProfileByUserID(ctx, userID)
	if err != nil {
		s.logger.Warn("Failed to get profile",
			zap.String("user_id", userID.String()),
			zap.Error(err),
		)
		return nil, fmt.Errorf("profile not found")
	}

	return profile, nil
}

// UpdateProfile updates a user's profile with validation
func (s *Service) UpdateProfile(ctx context.Context, userID uuid.UUID, input UpdateProfileInput) (*Profile, error) {
	// Validate bio length
	if input.Bio != nil && len(*input.Bio) > 1000 {
		return nil, fmt.Errorf("bio must be less than 1000 characters")
	}

	// Validate avatar URL
	if input.AvatarURL != nil && len(*input.AvatarURL) > 500 {
		return nil, fmt.Errorf("avatar URL must be less than 500 characters")
	}

	// Validate timezone format using timezone package
	if input.Timezone != nil {
		if err := timezone.ValidateTimezone(*input.Timezone); err != nil {
			return nil, fmt.Errorf("invalid timezone: %w", err)
		}
	}

	// Validate locale format
	if input.Locale != nil && len(*input.Locale) > 10 {
		return nil, fmt.Errorf("locale must be less than 10 characters")
	}

	profile, err := s.repo.UpdateProfile(ctx, userID, input)
	if err != nil {
		if strings.Contains(err.Error(), "version mismatch") {
			s.logger.Warn("Profile update conflict detected",
				zap.String("user_id", userID.String()),
				zap.Int("version", input.Version),
			)
			return nil, fmt.Errorf("profile was updated by another process, please refresh and try again")
		}

		s.logger.Error("Failed to update profile",
			zap.String("user_id", userID.String()),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to update profile")
	}

	s.logger.Info("Profile updated successfully",
		zap.String("user_id", userID.String()),
		zap.Int("new_version", profile.Version),
	)

	return profile, nil
}

// GetSettings retrieves a user's settings
func (s *Service) GetSettings(ctx context.Context, userID uuid.UUID) (*Settings, error) {
	settings, err := s.repo.GetSettingsByUserID(ctx, userID)
	if err != nil {
		s.logger.Warn("Failed to get settings",
			zap.String("user_id", userID.String()),
			zap.Error(err),
		)
		return nil, fmt.Errorf("settings not found")
	}

	return settings, nil
}

// UpdateSettings updates user settings with validation
func (s *Service) UpdateSettings(ctx context.Context, userID uuid.UUID, input UpdateSettingsInput) (*Settings, error) {
	// Validate theme
	if input.Theme != nil {
		validThemes := map[string]bool{"light": true, "dark": true, "system": true}
		if !validThemes[*input.Theme] {
			return nil, fmt.Errorf("invalid theme: must be 'light', 'dark', or 'system'")
		}
	}

	// Validate time format
	if input.TimeFormat != nil {
		validFormats := map[string]bool{"12h": true, "24h": true}
		if !validFormats[*input.TimeFormat] {
			return nil, fmt.Errorf("invalid time format: must be '12h' or '24h'")
		}
	}

	// Validate week start day
	if input.WeekStartDay != nil && (*input.WeekStartDay < 0 || *input.WeekStartDay > 6) {
		return nil, fmt.Errorf("invalid week start day: must be between 0 (Sunday) and 6 (Saturday)")
	}

	settings, err := s.repo.UpdateSettings(ctx, userID, input)
	if err != nil {
		if strings.Contains(err.Error(), "version mismatch") {
			s.logger.Warn("Settings update conflict detected",
				zap.String("user_id", userID.String()),
				zap.Int("version", input.Version),
			)
			return nil, fmt.Errorf("settings were updated by another process, please refresh and try again")
		}

		s.logger.Error("Failed to update settings",
			zap.String("user_id", userID.String()),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to update settings")
	}

	s.logger.Info("Settings updated successfully",
		zap.String("user_id", userID.String()),
		zap.Int("new_version", settings.Version),
	)

	return settings, nil
}

// GetNotificationSettings retrieves notification settings
func (s *Service) GetNotificationSettings(ctx context.Context, userID uuid.UUID) (*NotificationSettings, error) {
	settings, err := s.repo.GetNotificationSettingsByUserID(ctx, userID)
	if err != nil {
		s.logger.Warn("Failed to get notification settings",
			zap.String("user_id", userID.String()),
			zap.Error(err),
		)
		return nil, fmt.Errorf("notification settings not found")
	}

	return settings, nil
}

// UpdateNotificationSettings updates notification settings with validation
func (s *Service) UpdateNotificationSettings(ctx context.Context, userID uuid.UUID, input UpdateNotificationSettingsInput) (*NotificationSettings, error) {
	// Validate reminder interval
	if input.ReminderIntervalMinutes != nil {
		validIntervals := map[int]bool{15: true, 30: true, 45: true, 60: true, 90: true, 120: true, 180: true, 240: true}
		if !validIntervals[*input.ReminderIntervalMinutes] {
			return nil, fmt.Errorf("invalid reminder interval: must be one of 15, 30, 45, 60, 90, 120, 180, or 240 minutes")
		}
	}

	// Validate DND settings
	if input.DNDEnabled != nil && *input.DNDEnabled {
		if input.DNDStartTime == nil || input.DNDEndTime == nil {
			return nil, fmt.Errorf("DND start and end times are required when DND is enabled")
		}
	}

	// Validate DND days
	if input.DNDDays != nil {
		for _, day := range *input.DNDDays {
			if day < 1 || day > 7 {
				return nil, fmt.Errorf("invalid DND day: must be between 1 (Monday) and 7 (Sunday)")
			}
		}
	}

	settings, err := s.repo.UpdateNotificationSettings(ctx, userID, input)
	if err != nil {
		if strings.Contains(err.Error(), "version mismatch") {
			s.logger.Warn("Notification settings update conflict detected",
				zap.String("user_id", userID.String()),
				zap.Int("version", input.Version),
			)
			return nil, fmt.Errorf("notification settings were updated by another process, please refresh and try again")
		}

		s.logger.Error("Failed to update notification settings",
			zap.String("user_id", userID.String()),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to update notification settings")
	}

	s.logger.Info("Notification settings updated successfully",
		zap.String("user_id", userID.String()),
		zap.Int("new_version", settings.Version),
	)

	return settings, nil
}

// GetDataRetentionSettings retrieves data retention settings
func (s *Service) GetDataRetentionSettings(ctx context.Context, userID uuid.UUID) (*DataRetentionSettings, error) {
	settings, err := s.repo.GetDataRetentionSettingsByUserID(ctx, userID)
	if err != nil {
		s.logger.Warn("Failed to get data retention settings",
			zap.String("user_id", userID.String()),
			zap.Error(err),
		)
		return nil, fmt.Errorf("data retention settings not found")
	}

	return settings, nil
}

// UpdateDataRetentionSettings updates data retention settings with validation
func (s *Service) UpdateDataRetentionSettings(ctx context.Context, userID uuid.UUID, input UpdateDataRetentionSettingsInput) (*DataRetentionSettings, error) {
	// Validate retention days
	if input.RetentionDays != nil && *input.RetentionDays <= 0 {
		return nil, fmt.Errorf("retention days must be greater than 0")
	}

	// Validate delete after inactivity days (minimum 30 days for safety)
	if input.DeleteAfterInactivityDays != nil && *input.DeleteAfterInactivityDays < 30 {
		return nil, fmt.Errorf("delete after inactivity days must be at least 30 days")
	}

	settings, err := s.repo.UpdateDataRetentionSettings(ctx, userID, input)
	if err != nil {
		if strings.Contains(err.Error(), "version mismatch") {
			s.logger.Warn("Data retention settings update conflict detected",
				zap.String("user_id", userID.String()),
				zap.Int("version", input.Version),
			)
			return nil, fmt.Errorf("data retention settings were updated by another process, please refresh and try again")
		}

		s.logger.Error("Failed to update data retention settings",
			zap.String("user_id", userID.String()),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to update data retention settings")
	}

	s.logger.Info("Data retention settings updated successfully",
		zap.String("user_id", userID.String()),
		zap.Int("new_version", settings.Version),
	)

	return settings, nil
}

// GetAll retrieves all profile and settings for a user
func (s *Service) GetAll(ctx context.Context, userID uuid.UUID) (*ProfileWithSettings, error) {
	result, err := s.repo.GetAllByUserID(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to get all profile settings",
			zap.String("user_id", userID.String()),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to retrieve profile and settings")
	}

	return result, nil
}
