package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// DNDOverrideService manages DND overrides for urgent notifications
type DNDOverrideService struct {
	logger     *zap.Logger
	repository DNDOverrideRepository
	tzHandler  *TimezoneHandler
}

// DNDOverrideRepository defines the interface for DND override storage
type DNDOverrideRepository interface {
	CreateOverride(ctx context.Context, override *DNDOverride) error
	GetActiveOverride(ctx context.Context, userID uuid.UUID, checkTime time.Time) (*DNDOverride, error)
	GetUserOverrides(ctx context.Context, userID uuid.UUID) ([]*DNDOverride, error)
	DeleteOverride(ctx context.Context, overrideID uuid.UUID) error
	CleanupExpiredOverrides(ctx context.Context) (int, error)
}

// NewDNDOverrideService creates a new DND override service
func NewDNDOverrideService(logger *zap.Logger, repository DNDOverrideRepository) *DNDOverrideService {
	return &DNDOverrideService{
		logger:     logger,
		repository: repository,
		tzHandler:  NewTimezoneHandler(logger),
	}
}

// OverrideReason represents different reasons for DND overrides
type OverrideReason string

const (
	OverrideReasonUrgent     OverrideReason = "urgent"
	OverrideReasonEmergency  OverrideReason = "emergency"
	OverrideReasonManual     OverrideReason = "manual"
	OverrideReasonVIP        OverrideReason = "vip"
	OverrideReasonCritical   OverrideReason = "critical"
	OverrideReasonTravel     OverrideReason = "travel"
)

// CreateOverride creates a temporary DND override
func (s *DNDOverrideService) CreateOverride(ctx context.Context, userID uuid.UUID, duration time.Duration, reason OverrideReason) (*DNDOverride, error) {
	now := time.Now()

	override := &DNDOverride{
		ID:        uuid.New(),
		UserID:    userID,
		StartTime: now,
		EndTime:   now.Add(duration),
		Reason:    string(reason),
		CreatedAt: now,
	}

	if err := s.repository.CreateOverride(ctx, override); err != nil {
		return nil, fmt.Errorf("failed to create override: %w", err)
	}

	s.logger.Info("Created DND override",
		zap.String("user_id", userID.String()),
		zap.String("override_id", override.ID.String()),
		zap.String("reason", string(reason)),
		zap.Duration("duration", duration),
	)

	return override, nil
}

// CreateScheduledOverride creates a DND override for a specific time range
func (s *DNDOverrideService) CreateScheduledOverride(ctx context.Context, userID uuid.UUID, startTime, endTime time.Time, reason OverrideReason) (*DNDOverride, error) {
	if endTime.Before(startTime) {
		return nil, fmt.Errorf("end time must be after start time")
	}

	override := &DNDOverride{
		ID:        uuid.New(),
		UserID:    userID,
		StartTime: startTime,
		EndTime:   endTime,
		Reason:    string(reason),
		CreatedAt: time.Now(),
	}

	if err := s.repository.CreateOverride(ctx, override); err != nil {
		return nil, fmt.Errorf("failed to create scheduled override: %w", err)
	}

	s.logger.Info("Created scheduled DND override",
		zap.String("user_id", userID.String()),
		zap.String("override_id", override.ID.String()),
		zap.Time("start", startTime),
		zap.Time("end", endTime),
		zap.String("reason", string(reason)),
	)

	return override, nil
}

// IsOverrideActive checks if there's an active DND override for the user
func (s *DNDOverrideService) IsOverrideActive(ctx context.Context, userID uuid.UUID, checkTime time.Time) (bool, *DNDOverride, error) {
	override, err := s.repository.GetActiveOverride(ctx, userID, checkTime)
	if err != nil {
		return false, nil, fmt.Errorf("failed to check override: %w", err)
	}

	if override == nil {
		return false, nil, nil
	}

	// Verify the override is still active
	if checkTime.After(override.EndTime) {
		return false, nil, nil
	}

	return true, override, nil
}

// ShouldSendDuringDND determines if a notification should be sent during DND
// taking into account overrides and priority
func (s *DNDOverrideService) ShouldSendDuringDND(ctx context.Context, notification *Notification, settings *NotificationSettings) (bool, string, error) {
	// Check for active override
	hasOverride, override, err := s.IsOverrideActive(ctx, notification.UserID, time.Now())
	if err != nil {
		s.logger.Warn("Failed to check override status",
			zap.String("notification_id", notification.ID.String()),
			zap.Error(err),
		)
		// Continue without override check
	}

	if hasOverride {
		s.logger.Info("DND override active, allowing notification",
			zap.String("notification_id", notification.ID.String()),
			zap.String("override_reason", override.Reason),
		)
		return true, fmt.Sprintf("Override active: %s", override.Reason), nil
	}

	// Check priority-based override
	if s.shouldOverrideByPriority(notification.Priority) {
		s.logger.Info("High priority notification overriding DND",
			zap.String("notification_id", notification.ID.String()),
			zap.String("priority", string(notification.Priority)),
		)
		return true, fmt.Sprintf("Priority override: %s", notification.Priority), nil
	}

	return false, "DND active, no override", nil
}

// shouldOverrideByPriority determines if a notification priority should override DND
func (s *DNDOverrideService) shouldOverrideByPriority(priority Priority) bool {
	switch priority {
	case PriorityUrgent:
		return true
	case PriorityHigh:
		return false // Configurable: could be true based on user preferences
	default:
		return false
	}
}

// CancelOverride cancels an active DND override
func (s *DNDOverrideService) CancelOverride(ctx context.Context, overrideID uuid.UUID) error {
	if err := s.repository.DeleteOverride(ctx, overrideID); err != nil {
		return fmt.Errorf("failed to cancel override: %w", err)
	}

	s.logger.Info("Cancelled DND override",
		zap.String("override_id", overrideID.String()),
	)

	return nil
}

// GetUserOverrides retrieves all overrides for a user
func (s *DNDOverrideService) GetUserOverrides(ctx context.Context, userID uuid.UUID) ([]*DNDOverride, error) {
	overrides, err := s.repository.GetUserOverrides(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user overrides: %w", err)
	}

	return overrides, nil
}

// CleanupExpired removes expired DND overrides
func (s *DNDOverrideService) CleanupExpired(ctx context.Context) error {
	count, err := s.repository.CleanupExpiredOverrides(ctx)
	if err != nil {
		return fmt.Errorf("failed to cleanup expired overrides: %w", err)
	}

	s.logger.Info("Cleaned up expired DND overrides",
		zap.Int("count", count),
	)

	return nil
}

// CreateTravelOverride creates a DND override for travel (different timezone)
func (s *DNDOverrideService) CreateTravelOverride(ctx context.Context, userID uuid.UUID, newTimezone string, duration time.Duration) (*DNDOverride, error) {
	// Validate timezone
	if err := s.tzHandler.ValidateTimezone(newTimezone); err != nil {
		return nil, fmt.Errorf("invalid timezone: %w", err)
	}

	override, err := s.CreateOverride(ctx, userID, duration, OverrideReasonTravel)
	if err != nil {
		return nil, err
	}

	s.logger.Info("Created travel DND override",
		zap.String("user_id", userID.String()),
		zap.String("new_timezone", newTimezone),
		zap.Duration("duration", duration),
	)

	return override, nil
}

// GetOverrideStatus returns detailed status of DND overrides for a user
func (s *DNDOverrideService) GetOverrideStatus(ctx context.Context, userID uuid.UUID) (map[string]interface{}, error) {
	now := time.Now()

	// Check for active override
	hasActive, activeOverride, err := s.IsOverrideActive(ctx, userID, now)
	if err != nil {
		return nil, fmt.Errorf("failed to get override status: %w", err)
	}

	status := map[string]interface{}{
		"has_active_override": hasActive,
		"checked_at":          now.Format(time.RFC3339),
	}

	if hasActive {
		status["active_override"] = map[string]interface{}{
			"id":         activeOverride.ID.String(),
			"reason":     activeOverride.Reason,
			"start_time": activeOverride.StartTime.Format(time.RFC3339),
			"end_time":   activeOverride.EndTime.Format(time.RFC3339),
			"remaining":  activeOverride.EndTime.Sub(now).String(),
		}
	}

	// Get all user overrides
	allOverrides, err := s.GetUserOverrides(ctx, userID)
	if err != nil {
		s.logger.Warn("Failed to get all overrides",
			zap.String("user_id", userID.String()),
			zap.Error(err),
		)
	} else {
		status["total_overrides"] = len(allOverrides)

		upcoming := make([]map[string]interface{}, 0)
		for _, override := range allOverrides {
			if override.StartTime.After(now) {
				upcoming = append(upcoming, map[string]interface{}{
					"id":         override.ID.String(),
					"reason":     override.Reason,
					"start_time": override.StartTime.Format(time.RFC3339),
					"end_time":   override.EndTime.Format(time.RFC3339),
				})
			}
		}
		status["upcoming_overrides"] = upcoming
	}

	return status, nil
}

// CreateEmergencyOverride creates an emergency DND override (24 hours)
func (s *DNDOverrideService) CreateEmergencyOverride(ctx context.Context, userID uuid.UUID) (*DNDOverride, error) {
	return s.CreateOverride(ctx, userID, 24*time.Hour, OverrideReasonEmergency)
}

// CreateVIPOverride creates a VIP override for important notifications
func (s *DNDOverrideService) CreateVIPOverride(ctx context.Context, userID uuid.UUID, duration time.Duration) (*DNDOverride, error) {
	return s.CreateOverride(ctx, userID, duration, OverrideReasonVIP)
}

// ValidateOverride validates a DND override configuration
func (s *DNDOverrideService) ValidateOverride(override *DNDOverride) error {
	if override.UserID == uuid.Nil {
		return fmt.Errorf("user ID is required")
	}

	if override.EndTime.Before(override.StartTime) {
		return fmt.Errorf("end time must be after start time")
	}

	if override.Reason == "" {
		return fmt.Errorf("reason is required")
	}

	// Validate duration (max 7 days)
	duration := override.EndTime.Sub(override.StartTime)
	if duration > 7*24*time.Hour {
		return fmt.Errorf("override duration cannot exceed 7 days")
	}

	return nil
}

// ExtendOverride extends an existing DND override
func (s *DNDOverrideService) ExtendOverride(ctx context.Context, overrideID uuid.UUID, additionalDuration time.Duration) error {
	// This would require updating the repository to support updates
	// For now, return not implemented
	return fmt.Errorf("extend override not implemented")
}

// GetActiveOverridesCount returns the count of active overrides system-wide
func (s *DNDOverrideService) GetActiveOverridesCount(ctx context.Context) (int, error) {
	// This would require a new repository method
	// For now, return not implemented
	return 0, fmt.Errorf("get active overrides count not implemented")
}
