package timezone

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Package timezone provides utilities for timezone detection, validation, and DST handling.
//
// # Timezone Strategy
//
// This package follows IANA timezone database standards and provides:
// - Validation of IANA timezone identifiers
// - DST transition detection and handling
// - Timezone list API for manual selection UI
// - Auto-detection support via client-side helpers
//
// ## IANA Timezone Format
//
// Valid timezone identifiers follow the format: Area/Location
// Examples: "America/New_York", "Europe/London", "Asia/Tokyo"
//
// ## DST Handling
//
// The package detects DST transitions and provides:
// - Current DST offset calculation
// - Next DST transition time prediction
// - Historical DST boundary handling
//
// ## Storage Strategy
//
// User timezone preferences are stored in user_profiles table:
//   - timezone: VARCHAR(100) - IANA timezone identifier
//   - timezone_auto_detected: BOOLEAN - Whether auto-detected or manually set
//
// ## Client Integration
//
// Client-side timezone detection uses:
//   - Browser: Intl.DateTimeFormat().resolvedOptions().timeZone
//   - Mobile: OS timezone APIs (iOS: TimeZone.current, Android: TimeZone.getDefault())
//   - Fallback: GeoIP-based timezone detection

// Common timezone groups for UI organization
const (
	GroupAmericas = "Americas"
	GroupEurope   = "Europe"
	GroupAsia     = "Asia"
	GroupPacific  = "Pacific"
	GroupAfrica   = "Africa"
	GroupOther    = "Other"
)

// Timezone represents a timezone with metadata for UI display
type Timezone struct {
	// IANA timezone identifier (e.g., "America/New_York")
	ID string `json:"id"`

	// Display name (e.g., "Eastern Time - New York")
	DisplayName string `json:"display_name"`

	// Current UTC offset in minutes (e.g., -300 for EST, -240 for EDT)
	OffsetMinutes int `json:"offset_minutes"`

	// Formatted offset string (e.g., "UTC-05:00", "UTC-04:00")
	OffsetString string `json:"offset_string"`

	// Geographic group for UI organization
	Group string `json:"group"`

	// Whether this timezone observes DST
	ObservesDST bool `json:"observes_dst"`

	// Current DST status
	IsDST bool `json:"is_dst"`

	// Abbreviation (e.g., "EST", "EDT", "PST", "PDT")
	Abbreviation string `json:"abbreviation"`
}

// DSTTransition represents a DST transition event
type DSTTransition struct {
	// Time when the transition occurs
	Time time.Time `json:"time"`

	// Offset before transition (in minutes)
	OffsetBefore int `json:"offset_before"`

	// Offset after transition (in minutes)
	OffsetAfter int `json:"offset_after"`

	// Whether this is a spring forward (true) or fall back (false)
	IsSpringForward bool `json:"is_spring_forward"`

	// Human-readable description
	Description string `json:"description"`
}

// ValidationError represents a timezone validation error
type ValidationError struct {
	Timezone string
	Reason   string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("invalid timezone %q: %s", e.Timezone, e.Reason)
}

// IsValidTimezone validates an IANA timezone identifier
// Returns true if the timezone is recognized by Go's time package
func IsValidTimezone(tz string) bool {
	if tz == "" {
		return false
	}

	// Try to load the timezone location
	_, err := time.LoadLocation(tz)
	return err == nil
}

// ValidateTimezone validates a timezone identifier and returns a detailed error if invalid
func ValidateTimezone(tz string) error {
	if tz == "" {
		return &ValidationError{
			Timezone: tz,
			Reason:   "timezone cannot be empty",
		}
	}

	if len(tz) > 100 {
		return &ValidationError{
			Timezone: tz,
			Reason:   "timezone identifier too long (max 100 characters)",
		}
	}

	// Check if timezone can be loaded
	_, err := time.LoadLocation(tz)
	if err != nil {
		return &ValidationError{
			Timezone: tz,
			Reason:   "not a valid IANA timezone identifier",
		}
	}

	return nil
}

// GetTimezoneInfo returns detailed information about a timezone
func GetTimezoneInfo(tzID string) (*Timezone, error) {
	if err := ValidateTimezone(tzID); err != nil {
		return nil, err
	}

	loc, _ := time.LoadLocation(tzID)
	now := time.Now().In(loc)

	// Get current offset
	_, offset := now.Zone()
	offsetMinutes := offset / 60

	// Check if DST is currently active by comparing with January and July
	jan := time.Date(now.Year(), 1, 15, 12, 0, 0, 0, loc)
	jul := time.Date(now.Year(), 7, 15, 12, 0, 0, 0, loc)

	_, janOffset := jan.Zone()
	_, julOffset := jul.Zone()

	observesDST := janOffset != julOffset
	isDST := false

	if observesDST {
		// If offsets differ, DST is observed
		// The larger offset (more positive/less negative) is DST
		isDST = offset > janOffset || offset > julOffset
	}

	abbr, _ := now.Zone()

	return &Timezone{
		ID:            tzID,
		DisplayName:   formatDisplayName(tzID),
		OffsetMinutes: offsetMinutes,
		OffsetString:  formatOffset(offsetMinutes),
		Group:         getTimezoneGroup(tzID),
		ObservesDST:   observesDST,
		IsDST:         isDST,
		Abbreviation:  abbr,
	}, nil
}

// GetNextDSTTransition finds the next DST transition for a timezone
// Returns nil if no transition is found within the next year
func GetNextDSTTransition(tzID string) (*DSTTransition, error) {
	if err := ValidateTimezone(tzID); err != nil {
		return nil, err
	}

	loc, _ := time.LoadLocation(tzID)
	now := time.Now().In(loc)

	// Check each hour for the next 365 days to find a transition
	// This is a pragmatic approach that works for all timezone rules
	currentTime := now
	_, currentOffset := currentTime.Zone()

	for i := 0; i < 365*24; i++ {
		nextTime := currentTime.Add(time.Hour)
		_, nextOffset := nextTime.Zone()

		if currentOffset != nextOffset {
			// Found a transition
			// Binary search for exact transition time (within 1 minute)
			transitionTime := findExactTransition(currentTime, nextTime, loc)

			beforeTime := transitionTime.Add(-1 * time.Minute)
			afterTime := transitionTime.Add(1 * time.Minute)

			_, offsetBefore := beforeTime.Zone()
			_, offsetAfter := afterTime.Zone()

			isSpringForward := offsetAfter > offsetBefore

			return &DSTTransition{
				Time:            transitionTime,
				OffsetBefore:    offsetBefore / 60,
				OffsetAfter:     offsetAfter / 60,
				IsSpringForward: isSpringForward,
				Description:     formatTransitionDescription(transitionTime, offsetBefore/60, offsetAfter/60, isSpringForward),
			}, nil
		}

		currentTime = nextTime
		currentOffset = nextOffset
	}

	return nil, nil // No transition found in next year
}

// findExactTransition uses binary search to find the exact DST transition time
func findExactTransition(start, end time.Time, loc *time.Location) time.Time {
	// Binary search with 1-minute precision
	for end.Sub(start) > time.Minute {
		mid := start.Add(end.Sub(start) / 2)
		_, startOffset := start.Zone()
		_, midOffset := mid.Zone()

		if startOffset != midOffset {
			end = mid
		} else {
			start = mid
		}
	}

	return end
}

// GetAllTimezones returns a list of all supported timezones grouped by region
func GetAllTimezones() ([]*Timezone, error) {
	// Common timezones that users typically need
	// This list is curated for better UX rather than listing all 500+ IANA zones
	commonZones := []string{
		// Americas
		"America/New_York",
		"America/Chicago",
		"America/Denver",
		"America/Los_Angeles",
		"America/Anchorage",
		"America/Phoenix",
		"America/Toronto",
		"America/Vancouver",
		"America/Mexico_City",
		"America/Sao_Paulo",
		"America/Buenos_Aires",
		"America/Santiago",
		"America/Bogota",
		"America/Lima",
		"America/Caracas",

		// Europe
		"Europe/London",
		"Europe/Paris",
		"Europe/Berlin",
		"Europe/Rome",
		"Europe/Madrid",
		"Europe/Amsterdam",
		"Europe/Brussels",
		"Europe/Vienna",
		"Europe/Stockholm",
		"Europe/Oslo",
		"Europe/Copenhagen",
		"Europe/Helsinki",
		"Europe/Warsaw",
		"Europe/Prague",
		"Europe/Athens",
		"Europe/Istanbul",
		"Europe/Moscow",
		"Europe/Dublin",
		"Europe/Lisbon",
		"Europe/Zurich",

		// Asia
		"Asia/Dubai",
		"Asia/Kolkata",
		"Asia/Singapore",
		"Asia/Hong_Kong",
		"Asia/Tokyo",
		"Asia/Seoul",
		"Asia/Shanghai",
		"Asia/Bangkok",
		"Asia/Jakarta",
		"Asia/Manila",
		"Asia/Taipei",
		"Asia/Kuala_Lumpur",
		"Asia/Ho_Chi_Minh",
		"Asia/Dhaka",
		"Asia/Karachi",
		"Asia/Tehran",
		"Asia/Baghdad",
		"Asia/Jerusalem",
		"Asia/Riyadh",

		// Pacific
		"Pacific/Auckland",
		"Pacific/Fiji",
		"Pacific/Honolulu",
		"Pacific/Guam",
		"Pacific/Tahiti",

		// Australia
		"Australia/Sydney",
		"Australia/Melbourne",
		"Australia/Brisbane",
		"Australia/Perth",
		"Australia/Adelaide",
		"Australia/Darwin",

		// Africa
		"Africa/Cairo",
		"Africa/Johannesburg",
		"Africa/Lagos",
		"Africa/Nairobi",
		"Africa/Casablanca",
		"Africa/Algiers",

		// Atlantic
		"Atlantic/Reykjavik",
		"Atlantic/Azores",

		// UTC
		"UTC",
	}

	timezones := make([]*Timezone, 0, len(commonZones))

	for _, tzID := range commonZones {
		tz, err := GetTimezoneInfo(tzID)
		if err != nil {
			continue // Skip invalid timezones
		}
		timezones = append(timezones, tz)
	}

	// Sort by offset, then by display name
	sort.Slice(timezones, func(i, j int) bool {
		if timezones[i].OffsetMinutes != timezones[j].OffsetMinutes {
			return timezones[i].OffsetMinutes < timezones[j].OffsetMinutes
		}
		return timezones[i].DisplayName < timezones[j].DisplayName
	})

	return timezones, nil
}

// SearchTimezones searches for timezones matching a query string
func SearchTimezones(query string) ([]*Timezone, error) {
	allTimezones, err := GetAllTimezones()
	if err != nil {
		return nil, err
	}

	if query == "" {
		return allTimezones, nil
	}

	query = strings.ToLower(query)
	var results []*Timezone

	for _, tz := range allTimezones {
		if strings.Contains(strings.ToLower(tz.ID), query) ||
			strings.Contains(strings.ToLower(tz.DisplayName), query) ||
			strings.Contains(strings.ToLower(tz.Abbreviation), query) ||
			strings.Contains(strings.ToLower(tz.Group), query) {
			results = append(results, tz)
		}
	}

	return results, nil
}

// formatDisplayName converts timezone ID to a human-readable display name
func formatDisplayName(tzID string) string {
	if tzID == "UTC" {
		return "Coordinated Universal Time (UTC)"
	}

	parts := strings.Split(tzID, "/")
	if len(parts) < 2 {
		return tzID
	}

	region := parts[0]
	city := parts[len(parts)-1]

	// Replace underscores with spaces
	city = strings.ReplaceAll(city, "_", " ")

	return fmt.Sprintf("%s - %s", region, city)
}

// formatOffset formats UTC offset in minutes to a string like "UTC+05:30" or "UTC-08:00"
func formatOffset(offsetMinutes int) string {
	if offsetMinutes == 0 {
		return "UTC+00:00"
	}

	sign := "+"
	if offsetMinutes < 0 {
		sign = "-"
		offsetMinutes = -offsetMinutes
	}

	hours := offsetMinutes / 60
	minutes := offsetMinutes % 60

	return fmt.Sprintf("UTC%s%02d:%02d", sign, hours, minutes)
}

// getTimezoneGroup returns the geographic group for a timezone
func getTimezoneGroup(tzID string) string {
	if strings.HasPrefix(tzID, "America/") {
		return GroupAmericas
	}
	if strings.HasPrefix(tzID, "Europe/") {
		return GroupEurope
	}
	if strings.HasPrefix(tzID, "Asia/") {
		return GroupAsia
	}
	if strings.HasPrefix(tzID, "Pacific/") || strings.HasPrefix(tzID, "Australia/") {
		return GroupPacific
	}
	if strings.HasPrefix(tzID, "Africa/") {
		return GroupAfrica
	}
	return GroupOther
}

// formatTransitionDescription creates a human-readable DST transition description
func formatTransitionDescription(t time.Time, offsetBefore, offsetAfter int, isSpringForward bool) string {
	if isSpringForward {
		return fmt.Sprintf("Spring forward: UTC%s → UTC%s on %s",
			formatOffset(offsetBefore), formatOffset(offsetAfter), t.Format("January 2, 2006 at 3:04 PM"))
	}
	return fmt.Sprintf("Fall back: UTC%s → UTC%s on %s",
		formatOffset(offsetBefore), formatOffset(offsetAfter), t.Format("January 2, 2006 at 3:04 PM"))
}

// ConvertTime converts a time from one timezone to another
func ConvertTime(t time.Time, fromTZ, toTZ string) (time.Time, error) {
	if err := ValidateTimezone(fromTZ); err != nil {
		return time.Time{}, err
	}
	if err := ValidateTimezone(toTZ); err != nil {
		return time.Time{}, err
	}

	toLoc, _ := time.LoadLocation(toTZ)
	return t.In(toLoc), nil
}

// GetUserLocalTime returns the current time in a user's timezone
func GetUserLocalTime(userTimezone string) (time.Time, error) {
	if err := ValidateTimezone(userTimezone); err != nil {
		return time.Time{}, err
	}

	loc, _ := time.LoadLocation(userTimezone)
	return time.Now().In(loc), nil
}
