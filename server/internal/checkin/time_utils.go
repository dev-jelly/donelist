package checkin

import (
	"fmt"
	"time"

	"github.com/dev-jelly/donelist/internal/premium"
)

// Package checkin provides timezone-safe time handling for check-in operations.
//
// # Timezone Safety Strategy
//
// All time calculations in this package use UTC to ensure consistency across:
// - Different user timezones
// - Daylight Saving Time (DST) transitions
// - Server/client clock differences
//
// ## Storage Layer (PostgreSQL)
//
// The database uses TIMESTAMP WITH TIME ZONE for all time fields:
//   - checkin_time: TIMESTAMP WITH TIME ZONE
//   - created_at: TIMESTAMP WITH TIME ZONE
//   - updated_at: TIMESTAMP WITH TIME ZONE
//
// PostgreSQL behavior:
//   - Stores all times in UTC internally
//   - Accepts times in any timezone (RFC3339 format)
//   - Converts to UTC for storage
//   - Can return times in any timezone when queried
//
// ## Application Layer (Go)
//
// All time.Now() calls use .UTC() to ensure UTC time:
//   now := time.Now().UTC()
//
// All incoming times are converted to UTC:
//   checkinTimeUTC := checkinTime.UTC()
//
// This prevents:
//   - Timezone manipulation to bypass interval limits
//   - DST transition edge cases
//   - Server timezone changes affecting calculations
//
// ## API Layer (JSON)
//
// Times are exchanged in RFC3339 format (ISO 8601 with timezone):
//   "2025-03-09T15:00:00Z"        // UTC
//   "2025-03-09T10:00:00-05:00"   // EST with offset
//
// Go's time.Time marshals to RFC3339 automatically.
// Clients can send times in their local timezone; the server converts to UTC.
//
// ## DST Handling
//
// UTC-based calculations avoid DST issues:
//   - Spring forward: No "lost hour" problems
//   - Fall back: No "repeated hour" ambiguity
//   - Cross-DST intervals: Consistent duration calculations
//
// Example: User checks in at 2:30 AM on DST transition day
//   - Stored as UTC: "2025-03-09T07:30:00Z"
//   - No ambiguity about which "2:30 AM" it is
//   - Interval calculations work correctly
//
// ## Clock Skew Tolerance
//
// ClockSkewTolerance (1 second) handles:
//   - System clock precision differences
//   - Network latency in time transmission
//   - Boundary condition edge cases (exactly 120:00.000)
//
// Without tolerance: 120:00.001 might be treated differently than 120:00.000
// With tolerance: Sub-second differences near boundaries are normalized
//
// # Time interval constants for check-in timing rules
const (
	// EditTimeWindow defines the time window within which free users can edit checkins
	EditTimeWindow = 2 * time.Hour

	// Check-in interval durations
	Interval15Min  = 15 * time.Minute
	Interval30Min  = 30 * time.Minute
	Interval45Min  = 45 * time.Minute
	Interval2Hours = 2 * time.Hour

	// TwoHourBoundary is the threshold after which only 2-hour intervals are allowed
	// At exactly 2 hours (>=120m 0s), the boundary is crossed
	// Below 2 hours (119:59.999 or less), fine-grained intervals are allowed
	TwoHourBoundary = 2 * time.Hour

	// ClockSkewTolerance allows for small timing differences due to system clock precision
	// This prevents edge cases where 120m 0.0001s would be treated differently than 120m
	ClockSkewTolerance = 1 * time.Second
)

// CanEditCheckin determines if a user can edit a checkin based on its age and their tier
// Free users can only edit checkins created within EditTimeWindow (2 hours)
// Premium and Enterprise users can edit checkins at any time
func CanEditCheckin(checkinTime time.Time, userTier premium.Tier) bool {
	// Premium and Enterprise users have unlimited edit access
	if userTier == premium.TierPremium || userTier == premium.TierEnterprise {
		return true
	}

	// Free users can only edit checkins within the time window
	return GetEditTimeRemaining(checkinTime) > 0
}

// GetEditTimeRemaining returns how much time is left in the edit window
// Returns 0 if the edit window has expired
// Uses UTC time consistently to prevent timezone manipulation
func GetEditTimeRemaining(checkinTime time.Time) time.Duration {
	// Ensure we're working with UTC time
	now := time.Now().UTC()
	checkinTimeUTC := checkinTime.UTC()

	// Calculate when the edit window expires
	editWindowExpiry := checkinTimeUTC.Add(EditTimeWindow)

	// If current time is past the expiry, edit window is closed
	if now.After(editWindowExpiry) {
		return 0
	}

	// Return remaining time in the edit window
	return editWindowExpiry.Sub(now)
}

// IsWithinEditWindow checks if a checkin was created within the edit window
// This is a convenience function that returns a boolean
func IsWithinEditWindow(checkinTime time.Time) bool {
	return GetEditTimeRemaining(checkinTime) > 0
}

// GetEditWindowExpiry returns the exact time when the edit window expires for a checkin
func GetEditWindowExpiry(checkinTime time.Time) time.Time {
	return checkinTime.UTC().Add(EditTimeWindow)
}

// FormatTimeRemaining formats the remaining time in a human-readable format
// Returns strings like "1h 30m", "45m", "5m", etc.
func FormatTimeRemaining(duration time.Duration) string {
	if duration <= 0 {
		return "expired"
	}

	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60

	if hours > 0 {
		if minutes > 0 {
			return fmt.Sprintf("%dh %dm", hours, minutes)
		}
		return fmt.Sprintf("%dh", hours)
	}

	if minutes > 0 {
		return fmt.Sprintf("%dm", minutes)
	}

	seconds := int(duration.Seconds())
	return fmt.Sprintf("%ds", seconds)
}

// IsWithinTwoHourBoundary determines if the elapsed time is strictly within the 2-hour boundary
// Returns true for times < 2 hours (e.g., 119:59.999)
// Returns false for times >= 2 hours (e.g., 120:00.000, 120:00.001)
// Uses ClockSkewTolerance to handle precision issues near the boundary
func IsWithinTwoHourBoundary(elapsed time.Duration) bool {
	// If elapsed time is at least 2 hours minus tolerance, consider it at/beyond boundary
	return elapsed < (TwoHourBoundary - ClockSkewTolerance)
}

// IsAtOrBeyondTwoHourBoundary determines if the elapsed time has reached or exceeded 2 hours
// Returns true for times >= 2 hours (including exactly 120:00.000)
// Returns false for times < 2 hours
func IsAtOrBeyondTwoHourBoundary(elapsed time.Duration) bool {
	return !IsWithinTwoHourBoundary(elapsed)
}

// GetAllowedIntervals returns the list of allowed check-in intervals based on time since last check-in
// Within first 2 hours (< 120:00): 15min, 30min, 45min, 2hours
// At or after 2 hours (>= 120:00): only 2hours
func GetAllowedIntervals(timeSinceLastCheckin time.Duration) []time.Duration {
	if IsWithinTwoHourBoundary(timeSinceLastCheckin) {
		return []time.Duration{Interval15Min, Interval30Min, Interval45Min, Interval2Hours}
	}
	return []time.Duration{Interval2Hours}
}

// IsValidInterval checks if a given duration is a valid check-in interval
func IsValidInterval(duration time.Duration) bool {
	validIntervals := []time.Duration{Interval15Min, Interval30Min, Interval45Min, Interval2Hours}
	for _, interval := range validIntervals {
		if duration == interval {
			return true
		}
	}
	return false
}

// CanCheckinWithInterval determines if a check-in can be created with the given interval
// based on the time elapsed since the last check-in
// Returns true if the interval is allowed, false otherwise
//
// Boundary rules:
// - Time < 120m: All intervals (15m, 30m, 45m, 2h) allowed
// - Time >= 120m: Only 2-hour intervals allowed
func CanCheckinWithInterval(lastCheckinTime time.Time, desiredInterval time.Duration) bool {
	now := time.Now().UTC()
	lastCheckinUTC := lastCheckinTime.UTC()

	timeSinceLastCheckin := now.Sub(lastCheckinUTC)

	// If desired interval is not a valid interval at all, reject
	if !IsValidInterval(desiredInterval) {
		return false
	}

	// Within first 2 hours (< 120m): all intervals allowed
	if IsWithinTwoHourBoundary(timeSinceLastCheckin) {
		return true
	}

	// At or after 2 hours (>= 120m): only 2-hour intervals allowed
	return desiredInterval == Interval2Hours
}

// NextEligibleCheckinTime calculates the next time a user can create a check-in
// based on their last check-in time and desired interval
// Returns zero time if check-in is immediately available
//
// Example boundary scenarios:
// - Last check-in: 10:00:00, Interval: 15m → Next eligible: 10:15:00
// - Last check-in: 10:00:00, Current: 12:00:01 (>= 2h) → If interval is 15m/30m/45m: NOT allowed
// - Last check-in: 10:00:00, Current: 12:00:01 (>= 2h) → If interval is 2h: Next eligible: 12:00:00 (immediate)
func NextEligibleCheckinTime(lastCheckinTime time.Time, desiredInterval time.Duration) time.Time {
	now := time.Now().UTC()
	lastCheckinUTC := lastCheckinTime.UTC()

	// Calculate the minimum time that must pass before next check-in
	nextEligibleTime := lastCheckinUTC.Add(desiredInterval)

	// If we're past the eligible time, check-in is immediately available
	// Using tolerance to handle sub-second precision
	if now.After(nextEligibleTime) || now.Sub(nextEligibleTime) <= ClockSkewTolerance {
		return time.Time{} // Zero time indicates immediate availability
	}

	return nextEligibleTime
}

// GetTimeUntilNextCheckin returns the duration until the next check-in is eligible
// Returns 0 if check-in is immediately available
func GetTimeUntilNextCheckin(lastCheckinTime time.Time, desiredInterval time.Duration) time.Duration {
	nextTime := NextEligibleCheckinTime(lastCheckinTime, desiredInterval)

	// Zero time means immediately available
	if nextTime.IsZero() {
		return 0
	}

	now := time.Now().UTC()
	remaining := nextTime.Sub(now)

	// Ensure we don't return negative durations due to clock precision
	if remaining < 0 {
		return 0
	}

	return remaining
}
