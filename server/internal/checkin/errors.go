package checkin

import (
	"fmt"
	"time"

	"github.com/dev-jelly/donelist/internal/premium"
)

// EditPermissionError represents an error when a user doesn't have permission to edit a checkin
type EditPermissionError struct {
	CheckinID       string
	CheckinAge      time.Duration
	UserTier        premium.Tier
	RequiredTier    premium.Tier
	TimeRemaining   time.Duration
	EditWindowHours int
	Message         string
}

// Error implements the error interface
func (e *EditPermissionError) Error() string {
	if e.Message != "" {
		return e.Message
	}

	if e.TimeRemaining > 0 {
		remaining := FormatTimeRemaining(e.TimeRemaining)
		return fmt.Sprintf(
			"premium subscription required to edit check-ins older than %d hours (time remaining in free edit window: %s)",
			e.EditWindowHours,
			remaining,
		)
	}

	return fmt.Sprintf(
		"premium subscription required to edit check-ins older than %d hours",
		e.EditWindowHours,
	)
}

// ToAPIResponse converts the error to a structured API response
func (e *EditPermissionError) ToAPIResponse() map[string]interface{} {
	response := map[string]interface{}{
		"error":         "edit_permission_denied",
		"message":       e.Error(),
		"required_tier": string(e.RequiredTier),
		"current_tier":  string(e.UserTier),
		"upgrade_url":   "/api/v1/subscription/upgrade",
	}

	if e.TimeRemaining > 0 {
		response["time_remaining"] = FormatTimeRemaining(e.TimeRemaining)
		response["time_remaining_seconds"] = int(e.TimeRemaining.Seconds())
	} else {
		response["edit_window_expired"] = true
		response["edit_window_hours"] = e.EditWindowHours
	}

	return response
}

// IsEditPermissionError checks if an error is an EditPermissionError
func IsEditPermissionError(err error) bool {
	_, ok := err.(*EditPermissionError)
	return ok
}

// NewEditPermissionError creates a new EditPermissionError
func NewEditPermissionError(
	checkinID string,
	checkinTime time.Time,
	userTier premium.Tier,
) *EditPermissionError {
	checkinAge := time.Since(checkinTime.UTC())
	timeRemaining := GetEditTimeRemaining(checkinTime)
	editWindowHours := int(EditTimeWindow.Hours())

	return &EditPermissionError{
		CheckinID:       checkinID,
		CheckinAge:      checkinAge,
		UserTier:        userTier,
		RequiredTier:    premium.TierPremium,
		TimeRemaining:   timeRemaining,
		EditWindowHours: editWindowHours,
	}
}

// CheckinIntervalError represents an error when check-in interval rules are violated
type CheckinIntervalError struct {
	LastCheckinTime     time.Time
	RequestedInterval   time.Duration
	AllowedIntervals    []time.Duration
	TimeSinceLastCheckin time.Duration
	NextEligibleTime    time.Time
	TimeUntilEligible   time.Duration
	Message             string
}

// Error implements the error interface
func (e *CheckinIntervalError) Error() string {
	if e.Message != "" {
		return e.Message
	}

	timeUntil := FormatTimeRemaining(e.TimeUntilEligible)

	return fmt.Sprintf(
		"check-in interval violation: must wait %s before next check-in (next eligible at %s)",
		timeUntil,
		e.NextEligibleTime.Format(time.RFC3339),
	)
}

// ToAPIResponse converts the error to a structured API response
func (e *CheckinIntervalError) ToAPIResponse() map[string]interface{} {
	allowedIntervals := make([]string, len(e.AllowedIntervals))
	for i, interval := range e.AllowedIntervals {
		allowedIntervals[i] = FormatTimeRemaining(interval)
	}

	return map[string]interface{}{
		"error":                    "checkin_interval_violation",
		"message":                  e.Error(),
		"last_checkin_time":        e.LastCheckinTime.UTC().Format(time.RFC3339),
		"time_since_last_checkin":  FormatTimeRemaining(e.TimeSinceLastCheckin),
		"requested_interval":       FormatTimeRemaining(e.RequestedInterval),
		"allowed_intervals":        allowedIntervals,
		"next_eligible_time":       e.NextEligibleTime.UTC().Format(time.RFC3339),
		"time_until_eligible":      FormatTimeRemaining(e.TimeUntilEligible),
		"time_until_eligible_seconds": int(e.TimeUntilEligible.Seconds()),
	}
}

// IsCheckinIntervalError checks if an error is a CheckinIntervalError
func IsCheckinIntervalError(err error) bool {
	_, ok := err.(*CheckinIntervalError)
	return ok
}

// NewCheckinIntervalError creates a new CheckinIntervalError
func NewCheckinIntervalError(
	lastCheckinTime time.Time,
	requestedInterval time.Duration,
) *CheckinIntervalError {
	now := time.Now().UTC()
	timeSince := now.Sub(lastCheckinTime.UTC())
	allowedIntervals := GetAllowedIntervals(timeSince)
	nextEligibleTime := NextEligibleCheckinTime(lastCheckinTime, requestedInterval)
	timeUntilEligible := GetTimeUntilNextCheckin(lastCheckinTime, requestedInterval)

	return &CheckinIntervalError{
		LastCheckinTime:      lastCheckinTime,
		RequestedInterval:    requestedInterval,
		AllowedIntervals:     allowedIntervals,
		TimeSinceLastCheckin: timeSince,
		NextEligibleTime:     nextEligibleTime,
		TimeUntilEligible:    timeUntilEligible,
	}
}

// DuplicateCheckinError represents an error when attempting to create a duplicate check-in
type DuplicateCheckinError struct {
	CheckinTime time.Time
	Message     string
}

// Error implements the error interface
func (e *DuplicateCheckinError) Error() string {
	if e.Message != "" {
		return e.Message
	}

	return fmt.Sprintf(
		"duplicate check-in detected: a check-in already exists at %s",
		e.CheckinTime.Format(time.RFC3339),
	)
}

// ToAPIResponse converts the error to a structured API response
func (e *DuplicateCheckinError) ToAPIResponse() map[string]interface{} {
	return map[string]interface{}{
		"error":        "duplicate_checkin",
		"message":      e.Error(),
		"checkin_time": e.CheckinTime.UTC().Format(time.RFC3339),
		"suggestion":   "Please wait a few seconds before trying again or use a different time",
	}
}

// IsDuplicateCheckinError checks if an error is a DuplicateCheckinError
func IsDuplicateCheckinError(err error) bool {
	_, ok := err.(*DuplicateCheckinError)
	return ok
}

// NewDuplicateCheckinError creates a new DuplicateCheckinError
func NewDuplicateCheckinError(checkinTime time.Time) *DuplicateCheckinError {
	return &DuplicateCheckinError{
		CheckinTime: checkinTime,
	}
}
