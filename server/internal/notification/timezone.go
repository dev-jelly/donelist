package notification

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// TimezoneHandler manages timezone conversions and DST transitions
type TimezoneHandler struct {
	logger        *zap.Logger
	locationCache sync.Map // Cache for loaded time.Location objects
}

// NewTimezoneHandler creates a new timezone handler
func NewTimezoneHandler(logger *zap.Logger) *TimezoneHandler {
	return &TimezoneHandler{
		logger: logger,
	}
}

// TimezoneInfo represents timezone information with DST details
type TimezoneInfo struct {
	IANA         string    `json:"iana"`          // IANA timezone name (e.g., "America/New_York")
	Offset       int       `json:"offset"`        // Current offset from UTC in seconds
	IsDST        bool      `json:"is_dst"`        // Whether DST is currently active
	Name         string    `json:"name"`          // Timezone abbreviation (e.g., "EST", "EDT")
	NextTransition *time.Time `json:"next_transition,omitempty"` // Next DST transition
}

// UserTimezonePreference represents a user's timezone preferences
type UserTimezonePreference struct {
	UserID           string    `json:"user_id" db:"user_id"`
	Timezone         string    `json:"timezone" db:"timezone"`               // IANA timezone
	AutoDetect       bool      `json:"auto_detect" db:"auto_detect"`         // Auto-detect from device
	LastLocation     *string   `json:"last_location,omitempty" db:"last_location"` // Last known location
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
	TimezoneChangedAt *time.Time `json:"timezone_changed_at,omitempty" db:"timezone_changed_at"`
}

// GetLocation retrieves a time.Location by IANA timezone name with caching
func (h *TimezoneHandler) GetLocation(timezone string) (*time.Location, error) {
	if timezone == "" || timezone == "UTC" {
		return time.UTC, nil
	}

	// Check cache first
	if cached, ok := h.locationCache.Load(timezone); ok {
		return cached.(*time.Location), nil
	}

	// Load location
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		h.logger.Warn("Failed to load timezone, falling back to UTC",
			zap.String("timezone", timezone),
			zap.Error(err),
		)
		return time.UTC, fmt.Errorf("invalid timezone %s: %w", timezone, err)
	}

	// Cache the location
	h.locationCache.Store(timezone, loc)
	return loc, nil
}

// ConvertToUserTime converts a UTC time to the user's local timezone
func (h *TimezoneHandler) ConvertToUserTime(utcTime time.Time, timezone string) (time.Time, error) {
	loc, err := h.GetLocation(timezone)
	if err != nil {
		return utcTime, err
	}

	return utcTime.In(loc), nil
}

// ConvertToUTC converts a time from user's timezone to UTC
func (h *TimezoneHandler) ConvertToUTC(localTime time.Time, timezone string) (time.Time, error) {
	loc, err := h.GetLocation(timezone)
	if err != nil {
		return localTime, err
	}

	// Create a time in the user's timezone
	userTime := time.Date(
		localTime.Year(),
		localTime.Month(),
		localTime.Day(),
		localTime.Hour(),
		localTime.Minute(),
		localTime.Second(),
		localTime.Nanosecond(),
		loc,
	)

	return userTime.UTC(), nil
}

// GetTimezoneInfo returns detailed information about a timezone
func (h *TimezoneHandler) GetTimezoneInfo(timezone string, at time.Time) (*TimezoneInfo, error) {
	loc, err := h.GetLocation(timezone)
	if err != nil {
		return nil, err
	}

	t := at.In(loc)
	name, offset := t.Zone()

	info := &TimezoneInfo{
		IANA:   timezone,
		Offset: offset,
		Name:   name,
		IsDST:  h.IsDST(t, loc),
	}

	// Calculate next DST transition
	nextTransition := h.GetNextDSTTransition(t, loc)
	if nextTransition != nil {
		info.NextTransition = nextTransition
	}

	return info, nil
}

// IsDST checks if Daylight Saving Time is active at the given time
func (h *TimezoneHandler) IsDST(t time.Time, loc *time.Location) bool {
	// Get the standard time offset by checking January (winter in Northern Hemisphere)
	jan := time.Date(t.Year(), time.January, 1, 0, 0, 0, 0, loc)
	_, janOffset := jan.Zone()

	// Get current offset
	_, currentOffset := t.Zone()

	// DST is active if current offset is greater than standard (winter) offset
	return currentOffset > janOffset
}

// GetNextDSTTransition finds the next DST transition after the given time
func (h *TimezoneHandler) GetNextDSTTransition(t time.Time, loc *time.Location) *time.Time {
	// Check the next 365 days for a DST transition
	current := t
	_, currentOffset := current.In(loc).Zone()

	for i := 0; i < 365; i++ {
		next := current.Add(24 * time.Hour)
		_, nextOffset := next.In(loc).Zone()

		if nextOffset != currentOffset {
			// Found a transition, now narrow it down to the hour
			transition := h.findTransitionTime(current, next, loc)
			return &transition
		}

		current = next
	}

	return nil
}

// findTransitionTime narrows down the exact transition time between two dates
func (h *TimezoneHandler) findTransitionTime(start, end time.Time, loc *time.Location) time.Time {
	_, startOffset := start.In(loc).Zone()

	// Binary search for the transition time
	for end.Sub(start) > time.Minute {
		mid := start.Add(end.Sub(start) / 2)
		_, midOffset := mid.In(loc).Zone()

		if midOffset == startOffset {
			start = mid
		} else {
			end = mid
		}
	}

	return end
}

// ValidateTimezone checks if a timezone string is valid
func (h *TimezoneHandler) ValidateTimezone(timezone string) error {
	if timezone == "" {
		return fmt.Errorf("timezone cannot be empty")
	}

	if timezone == "UTC" {
		return nil
	}

	_, err := h.GetLocation(timezone)
	return err
}

// ConvertScheduleTime converts a schedule time to UTC handling DST transitions
func (h *TimezoneHandler) ConvertScheduleTime(hour, minute int, timezone string, referenceDate time.Time) (time.Time, error) {
	loc, err := h.GetLocation(timezone)
	if err != nil {
		return time.Time{}, err
	}

	// Create the scheduled time in the user's timezone
	scheduledTime := time.Date(
		referenceDate.Year(),
		referenceDate.Month(),
		referenceDate.Day(),
		hour,
		minute,
		0,
		0,
		loc,
	)

	return scheduledTime.UTC(), nil
}

// AdjustForDSTTransition adjusts a recurring schedule time for DST transitions
// This ensures that "9 AM local time" stays at 9 AM even after DST changes
func (h *TimezoneHandler) AdjustForDSTTransition(scheduledUTC time.Time, timezone string) (time.Time, error) {
	loc, err := h.GetLocation(timezone)
	if err != nil {
		return scheduledUTC, err
	}

	// Convert to user's local time
	localTime := scheduledUTC.In(loc)

	// Recreate the time with the same clock time but current DST rules
	adjusted := time.Date(
		localTime.Year(),
		localTime.Month(),
		localTime.Day(),
		localTime.Hour(),
		localTime.Minute(),
		localTime.Second(),
		localTime.Nanosecond(),
		loc,
	)

	return adjusted.UTC(), nil
}

// GetCommonTimezones returns a list of commonly used timezones
func (h *TimezoneHandler) GetCommonTimezones() []string {
	return []string{
		// Americas
		"America/New_York",
		"America/Chicago",
		"America/Denver",
		"America/Los_Angeles",
		"America/Toronto",
		"America/Vancouver",
		"America/Mexico_City",
		"America/Sao_Paulo",
		"America/Argentina/Buenos_Aires",

		// Europe
		"Europe/London",
		"Europe/Paris",
		"Europe/Berlin",
		"Europe/Madrid",
		"Europe/Rome",
		"Europe/Amsterdam",
		"Europe/Brussels",
		"Europe/Vienna",
		"Europe/Stockholm",
		"Europe/Moscow",

		// Asia
		"Asia/Tokyo",
		"Asia/Seoul",
		"Asia/Shanghai",
		"Asia/Hong_Kong",
		"Asia/Singapore",
		"Asia/Bangkok",
		"Asia/Dubai",
		"Asia/Kolkata",
		"Asia/Jakarta",
		"Asia/Manila",

		// Oceania
		"Australia/Sydney",
		"Australia/Melbourne",
		"Australia/Perth",
		"Pacific/Auckland",

		// Africa
		"Africa/Cairo",
		"Africa/Johannesburg",
		"Africa/Lagos",
		"Africa/Nairobi",

		// UTC
		"UTC",
	}
}

// DetectTimezoneChange detects if a user has changed timezones (e.g., traveling)
func (h *TimezoneHandler) DetectTimezoneChange(previousTimezone, currentTimezone string, threshold time.Duration) (bool, error) {
	if previousTimezone == currentTimezone {
		return false, nil
	}

	if previousTimezone == "" {
		return true, nil // First time setting timezone
	}

	// Get locations
	prevLoc, err := h.GetLocation(previousTimezone)
	if err != nil {
		return false, fmt.Errorf("invalid previous timezone: %w", err)
	}

	currentLoc, err := h.GetLocation(currentTimezone)
	if err != nil {
		return false, fmt.Errorf("invalid current timezone: %w", err)
	}

	// Compare offsets at current time
	now := time.Now()
	_, prevOffset := now.In(prevLoc).Zone()
	_, currentOffset := now.In(currentLoc).Zone()

	offsetDiff := time.Duration(abs(currentOffset-prevOffset)) * time.Second

	// Consider it a significant change if difference is greater than threshold
	return offsetDiff >= threshold, nil
}

// CalculateLocalMidnight calculates midnight in the user's timezone
func (h *TimezoneHandler) CalculateLocalMidnight(date time.Time, timezone string) (time.Time, error) {
	loc, err := h.GetLocation(timezone)
	if err != nil {
		return time.Time{}, err
	}

	localDate := date.In(loc)
	midnight := time.Date(
		localDate.Year(),
		localDate.Month(),
		localDate.Day(),
		0, 0, 0, 0,
		loc,
	)

	return midnight.UTC(), nil
}

// GetLocalTimeOfDay returns the time of day in the user's timezone
func (h *TimezoneHandler) GetLocalTimeOfDay(t time.Time, timezone string) (hour, minute, second int, err error) {
	loc, err := h.GetLocation(timezone)
	if err != nil {
		return 0, 0, 0, err
	}

	localTime := t.In(loc)
	return localTime.Hour(), localTime.Minute(), localTime.Second(), nil
}

// FormatInTimezone formats a time in the user's timezone
func (h *TimezoneHandler) FormatInTimezone(t time.Time, timezone string, layout string) (string, error) {
	loc, err := h.GetLocation(timezone)
	if err != nil {
		return "", err
	}

	return t.In(loc).Format(layout), nil
}

// ParseTimeInTimezone parses a time string in the user's timezone
func (h *TimezoneHandler) ParseTimeInTimezone(timeStr, timezone, layout string) (time.Time, error) {
	loc, err := h.GetLocation(timezone)
	if err != nil {
		return time.Time{}, err
	}

	return time.ParseInLocation(layout, timeStr, loc)
}

// GetDSTTransitionDates returns the DST transition dates for a given year
func (h *TimezoneHandler) GetDSTTransitionDates(year int, timezone string) (spring, fall *time.Time, err error) {
	loc, err := h.GetLocation(timezone)
	if err != nil {
		return nil, nil, err
	}

	// Start from January
	current := time.Date(year, time.January, 1, 0, 0, 0, 0, loc)
	_, winterOffset := current.Zone()

	var springTransition, fallTransition *time.Time

	// Check each day of the year
	for i := 0; i < 365; i++ {
		next := current.Add(24 * time.Hour)
		_, nextOffset := next.In(loc).Zone()

		if nextOffset != winterOffset && springTransition == nil {
			// Spring forward (winter -> summer)
			transition := h.findTransitionTime(current, next, loc)
			springTransition = &transition
		} else if nextOffset == winterOffset && springTransition != nil && fallTransition == nil {
			// Fall back (summer -> winter)
			transition := h.findTransitionTime(current, next, loc)
			fallTransition = &transition
		}

		if springTransition != nil && fallTransition != nil {
			break
		}

		current = next
	}

	return springTransition, fallTransition, nil
}

// abs returns the absolute value of an integer
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// TimezoneManager handles user timezone preferences with travel detection
type TimezoneManager struct {
	handler *TimezoneHandler
	logger  *zap.Logger
}

// NewTimezoneManager creates a new timezone manager
func NewTimezoneManager(logger *zap.Logger) *TimezoneManager {
	return &TimezoneManager{
		handler: NewTimezoneHandler(logger),
		logger:  logger,
	}
}

// UpdateUserTimezone updates a user's timezone and detects travel
func (m *TimezoneManager) UpdateUserTimezone(pref *UserTimezonePreference, newTimezone string) (travelDetected bool, err error) {
	// Validate new timezone
	if err := m.handler.ValidateTimezone(newTimezone); err != nil {
		return false, fmt.Errorf("invalid timezone: %w", err)
	}

	// Detect timezone change (consider 1 hour as threshold)
	if pref.Timezone != "" {
		changed, err := m.handler.DetectTimezoneChange(pref.Timezone, newTimezone, time.Hour)
		if err != nil {
			m.logger.Warn("Failed to detect timezone change",
				zap.String("user_id", pref.UserID),
				zap.Error(err),
			)
		}
		travelDetected = changed
	}

	// Update preference
	oldTimezone := pref.Timezone
	pref.Timezone = newTimezone
	pref.UpdatedAt = time.Now()

	if travelDetected {
		now := time.Now()
		pref.TimezoneChangedAt = &now
		pref.LastLocation = &oldTimezone

		m.logger.Info("User timezone changed - travel detected",
			zap.String("user_id", pref.UserID),
			zap.String("from", oldTimezone),
			zap.String("to", newTimezone),
		)
	}

	return travelDetected, nil
}

// GetTimezoneForScheduling returns the appropriate timezone for scheduling
func (m *TimezoneManager) GetTimezoneForScheduling(pref *UserTimezonePreference) string {
	if pref.Timezone == "" {
		return "UTC"
	}
	return pref.Timezone
}

// ShouldAdjustSchedules determines if existing schedules should be adjusted due to timezone change
func (m *TimezoneManager) ShouldAdjustSchedules(pref *UserTimezonePreference) bool {
	// Only adjust if timezone was changed within the last 24 hours
	if pref.TimezoneChangedAt == nil {
		return false
	}

	return time.Since(*pref.TimezoneChangedAt) < 24*time.Hour
}
