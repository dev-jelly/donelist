package notification

import (
	"fmt"
	"time"

	"go.uber.org/zap"
)

// DNDScheduler manages Do Not Disturb scheduling logic
type DNDScheduler struct {
	logger *zap.Logger
}

// NewDNDScheduler creates a new DND scheduler
func NewDNDScheduler(logger *zap.Logger) *DNDScheduler {
	return &DNDScheduler{
		logger: logger,
	}
}

// IsInDNDPeriod checks if the given time falls within the user's DND period
func (d *DNDScheduler) IsInDNDPeriod(settings *NotificationSettings, checkTime time.Time) (bool, string) {
	if !settings.DNDEnabled {
		return false, "DND not enabled"
	}

	if settings.DNDStartTime == nil || settings.DNDEndTime == nil {
		d.logger.Warn("DND enabled but times not set",
			zap.String("user_id", settings.UserID.String()),
		)
		return false, "DND times not configured"
	}

	// Convert check time to user's timezone
	userTime, err := d.convertToUserTimezone(checkTime, settings.Timezone)
	if err != nil {
		d.logger.Error("Failed to convert to user timezone",
			zap.String("timezone", settings.Timezone),
			zap.Error(err),
		)
		// Default to UTC if timezone conversion fails
		userTime = checkTime
	}

	// Check if current day is in DND days
	if len(settings.DNDDays) > 0 {
		currentDay := int(userTime.Weekday())
		dayIncluded := false
		for _, day := range settings.DNDDays {
			if day == currentDay {
				dayIncluded = true
				break
			}
		}
		if !dayIncluded {
			return false, fmt.Sprintf("Day %s not in DND schedule", userTime.Weekday().String())
		}
	}

	// Extract time components for comparison
	currentTimeMinutes := userTime.Hour()*60 + userTime.Minute()

	// Convert DND times to minutes since midnight
	startMinutes := settings.DNDStartTime.Hour()*60 + settings.DNDStartTime.Minute()
	endMinutes := settings.DNDEndTime.Hour()*60 + settings.DNDEndTime.Minute()

	// Handle case where DND period spans midnight
	if startMinutes > endMinutes {
		// DND period spans midnight (e.g., 22:00 to 06:00)
		if currentTimeMinutes >= startMinutes || currentTimeMinutes < endMinutes {
			return true, fmt.Sprintf("In DND period (%02d:%02d-%02d:%02d spanning midnight)",
				settings.DNDStartTime.Hour(), settings.DNDStartTime.Minute(),
				settings.DNDEndTime.Hour(), settings.DNDEndTime.Minute())
		}
	} else {
		// DND period within same day (e.g., 00:00 to 08:00)
		if currentTimeMinutes >= startMinutes && currentTimeMinutes < endMinutes {
			return true, fmt.Sprintf("In DND period (%02d:%02d-%02d:%02d)",
				settings.DNDStartTime.Hour(), settings.DNDStartTime.Minute(),
				settings.DNDEndTime.Hour(), settings.DNDEndTime.Minute())
		}
	}

	return false, "Outside DND period"
}

// GetNextAvailableTime calculates the next time outside of DND period
func (d *DNDScheduler) GetNextAvailableTime(settings *NotificationSettings, requestedTime time.Time) (time.Time, error) {
	if !settings.DNDEnabled {
		return requestedTime, nil
	}

	if settings.DNDStartTime == nil || settings.DNDEndTime == nil {
		return requestedTime, nil
	}

	// Convert to user's timezone
	userTime, err := d.convertToUserTimezone(requestedTime, settings.Timezone)
	if err != nil {
		return requestedTime, fmt.Errorf("failed to convert to user timezone: %w", err)
	}

	// Check if current time is in DND
	inDND, _ := d.IsInDNDPeriod(settings, requestedTime)
	if !inDND {
		return requestedTime, nil
	}

	// Calculate next available time after DND ends
	nextAvailable := d.calculateNextAvailableTime(userTime, settings)

	// Convert back to UTC
	utcTime, err := d.convertFromUserTimezone(nextAvailable, settings.Timezone)
	if err != nil {
		return requestedTime, fmt.Errorf("failed to convert from user timezone: %w", err)
	}

	return utcTime, nil
}

// calculateNextAvailableTime calculates when DND period ends
func (d *DNDScheduler) calculateNextAvailableTime(currentTime time.Time, settings *NotificationSettings) time.Time {
	// Get the end time for today's DND period
	endTime := time.Date(
		currentTime.Year(),
		currentTime.Month(),
		currentTime.Day(),
		settings.DNDEndTime.Hour(),
		settings.DNDEndTime.Minute(),
		0, 0,
		currentTime.Location(),
	)

	startMinutes := settings.DNDStartTime.Hour()*60 + settings.DNDStartTime.Minute()
	endMinutes := settings.DNDEndTime.Hour()*60 + settings.DNDEndTime.Minute()

	// If DND spans midnight and we're past midnight but before end time
	if startMinutes > endMinutes {
		currentMinutes := currentTime.Hour()*60 + currentTime.Minute()
		if currentMinutes < endMinutes {
			// We're in the morning part of a DND that started yesterday
			return endTime
		}
		// We're in the evening part, DND ends tomorrow morning
		endTime = endTime.Add(24 * time.Hour)
	}

	// If the end time has already passed today, move to tomorrow
	if endTime.Before(currentTime) {
		endTime = endTime.Add(24 * time.Hour)
	}

	// Check if the next day is included in DND days
	if len(settings.DNDDays) > 0 {
		for i := 0; i < 7; i++ { // Check up to a week ahead
			dayOfWeek := int(endTime.Weekday())
			for _, dndDay := range settings.DNDDays {
				if dndDay == dayOfWeek {
					// This day has DND, so return the end time
					return endTime
				}
			}
			// This day doesn't have DND, try the next day
			endTime = endTime.Add(24 * time.Hour)
		}
	}

	return endTime
}

// convertToUserTimezone converts a UTC time to the user's timezone
func (d *DNDScheduler) convertToUserTimezone(utcTime time.Time, timezone string) (time.Time, error) {
	if timezone == "" {
		// Default to UTC if no timezone specified
		return utcTime, nil
	}

	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return utcTime, fmt.Errorf("invalid timezone %s: %w", timezone, err)
	}

	return utcTime.In(loc), nil
}

// convertFromUserTimezone converts a time from user's timezone to UTC
func (d *DNDScheduler) convertFromUserTimezone(userTime time.Time, timezone string) (time.Time, error) {
	if timezone == "" {
		// Already in UTC
		return userTime, nil
	}

	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return userTime, fmt.Errorf("invalid timezone %s: %w", timezone, err)
	}

	// Create a time in the user's timezone and convert to UTC
	localTime := time.Date(
		userTime.Year(),
		userTime.Month(),
		userTime.Day(),
		userTime.Hour(),
		userTime.Minute(),
		userTime.Second(),
		userTime.Nanosecond(),
		loc,
	)

	return localTime.UTC(), nil
}

// CalculateDNDWindows calculates DND windows for the next N days
func (d *DNDScheduler) CalculateDNDWindows(settings *NotificationSettings, days int) ([]DNDWindow, error) {
	if !settings.DNDEnabled || settings.DNDStartTime == nil || settings.DNDEndTime == nil {
		return []DNDWindow{}, nil
	}

	windows := make([]DNDWindow, 0)
	now := time.Now()

	// Convert to user's timezone
	userNow, err := d.convertToUserTimezone(now, settings.Timezone)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to user timezone: %w", err)
	}

	for i := 0; i < days; i++ {
		checkDate := userNow.Add(time.Duration(i) * 24 * time.Hour)

		// Check if this day has DND
		if len(settings.DNDDays) > 0 {
			dayIncluded := false
			for _, day := range settings.DNDDays {
				if day == int(checkDate.Weekday()) {
					dayIncluded = true
					break
				}
			}
			if !dayIncluded {
				continue
			}
		}

		// Calculate start and end times for this day
		startTime := time.Date(
			checkDate.Year(),
			checkDate.Month(),
			checkDate.Day(),
			settings.DNDStartTime.Hour(),
			settings.DNDStartTime.Minute(),
			0, 0,
			checkDate.Location(),
		)

		endTime := time.Date(
			checkDate.Year(),
			checkDate.Month(),
			checkDate.Day(),
			settings.DNDEndTime.Hour(),
			settings.DNDEndTime.Minute(),
			0, 0,
			checkDate.Location(),
		)

		// Handle DND spanning midnight
		if settings.DNDStartTime.Hour()*60+settings.DNDStartTime.Minute() >
		   settings.DNDEndTime.Hour()*60+settings.DNDEndTime.Minute() {
			endTime = endTime.Add(24 * time.Hour)
		}

		// Convert to UTC
		startUTC, _ := d.convertFromUserTimezone(startTime, settings.Timezone)
		endUTC, _ := d.convertFromUserTimezone(endTime, settings.Timezone)

		windows = append(windows, DNDWindow{
			Start: startUTC,
			End:   endUTC,
			Day:   checkDate.Weekday().String(),
		})
	}

	return windows, nil
}

// DNDWindow represents a DND time window
type DNDWindow struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	Day   string    `json:"day"`
}

// ValidateDNDSettings validates DND configuration
func (d *DNDScheduler) ValidateDNDSettings(settings *NotificationSettings) error {
	if !settings.DNDEnabled {
		return nil
	}

	if settings.DNDStartTime == nil || settings.DNDEndTime == nil {
		return fmt.Errorf("DND enabled but start or end time not set")
	}

	// Validate days of week
	for _, day := range settings.DNDDays {
		if day < 0 || day > 6 {
			return fmt.Errorf("invalid day of week: %d (must be 0-6)", day)
		}
	}

	// Validate timezone
	if settings.Timezone != "" {
		if _, err := time.LoadLocation(settings.Timezone); err != nil {
			return fmt.Errorf("invalid timezone: %s", settings.Timezone)
		}
	}

	return nil
}

// GetDNDStatus returns detailed DND status for debugging
func (d *DNDScheduler) GetDNDStatus(settings *NotificationSettings) map[string]interface{} {
	status := map[string]interface{}{
		"enabled": settings.DNDEnabled,
	}

	if !settings.DNDEnabled {
		return status
	}

	now := time.Now()
	inDND, reason := d.IsInDNDPeriod(settings, now)

	status["currently_active"] = inDND
	status["reason"] = reason
	status["timezone"] = settings.Timezone

	if settings.DNDStartTime != nil && settings.DNDEndTime != nil {
		status["start_time"] = fmt.Sprintf("%02d:%02d", settings.DNDStartTime.Hour(), settings.DNDStartTime.Minute())
		status["end_time"] = fmt.Sprintf("%02d:%02d", settings.DNDEndTime.Hour(), settings.DNDEndTime.Minute())
	}

	if len(settings.DNDDays) > 0 {
		dayNames := make([]string, len(settings.DNDDays))
		for i, day := range settings.DNDDays {
			dayNames[i] = time.Weekday(day).String()
		}
		status["active_days"] = dayNames
	}

	// Calculate next DND windows
	windows, err := d.CalculateDNDWindows(settings, 7)
	if err == nil {
		status["upcoming_windows"] = windows
	}

	// Calculate next available time if currently in DND
	if inDND {
		nextAvailable, err := d.GetNextAvailableTime(settings, now)
		if err == nil {
			status["next_available"] = nextAvailable.Format(time.RFC3339)
		}
	}

	return status
}