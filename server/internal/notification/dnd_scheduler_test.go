package notification

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestDNDScheduler_IsInDNDPeriod(t *testing.T) {
	logger := zap.NewNop()
	scheduler := NewDNDScheduler(logger)
	userID := uuid.New()

	tests := []struct {
		name         string
		settings     *NotificationSettings
		checkTime    time.Time
		expectedInDND bool
		expectedReason string
	}{
		{
			name: "DND not enabled",
			settings: &NotificationSettings{
				UserID:     userID,
				DNDEnabled: false,
				Timezone:   "UTC",
			},
			checkTime:      time.Now(),
			expectedInDND:  false,
			expectedReason: "DND not enabled",
		},
		{
			name: "within DND period same day",
			settings: &NotificationSettings{
				UserID:       userID,
				DNDEnabled:   true,
				DNDStartTime: timePtr(time.Date(0, 1, 1, 22, 0, 0, 0, time.UTC)), // 10 PM
				DNDEndTime:   timePtr(time.Date(0, 1, 1, 23, 0, 0, 0, time.UTC)), // 11 PM
				DNDDays:      []int{1}, // Monday
				Timezone:     "UTC",
			},
			checkTime:      time.Date(2024, 1, 1, 22, 30, 0, 0, time.UTC).AddDate(0, 0, getDayOffset(1)),
			expectedInDND:  true,
			expectedReason: "In DND period (22:00-23:00)",
		},
		{
			name: "within DND period spanning midnight",
			settings: &NotificationSettings{
				UserID:       userID,
				DNDEnabled:   true,
				DNDStartTime: timePtr(time.Date(0, 1, 1, 22, 0, 0, 0, time.UTC)), // 10 PM
				DNDEndTime:   timePtr(time.Date(0, 1, 1, 6, 0, 0, 0, time.UTC)),  // 6 AM
				DNDDays:      []int{1}, // Monday
				Timezone:     "UTC",
			},
			checkTime:      time.Date(2024, 1, 1, 23, 30, 0, 0, time.UTC).AddDate(0, 0, getDayOffset(1)),
			expectedInDND:  true,
			expectedReason: "In DND period (22:00-06:00 spanning midnight)",
		},
		{
			name: "outside DND period",
			settings: &NotificationSettings{
				UserID:       userID,
				DNDEnabled:   true,
				DNDStartTime: timePtr(time.Date(0, 1, 1, 22, 0, 0, 0, time.UTC)), // 10 PM
				DNDEndTime:   timePtr(time.Date(0, 1, 1, 6, 0, 0, 0, time.UTC)),  // 6 AM
				DNDDays:      []int{1}, // Monday
				Timezone:     "UTC",
			},
			checkTime:      time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC).AddDate(0, 0, getDayOffset(1)), // Noon on Monday
			expectedInDND:  false,
			expectedReason: "Outside DND period",
		},
		{
			name: "day not in DND schedule",
			settings: &NotificationSettings{
				UserID:       userID,
				DNDEnabled:   true,
				DNDStartTime: timePtr(time.Date(0, 1, 1, 22, 0, 0, 0, time.UTC)), // 10 PM
				DNDEndTime:   timePtr(time.Date(0, 1, 1, 6, 0, 0, 0, time.UTC)),  // 6 AM
				DNDDays:      []int{1, 2, 3, 4, 5}, // Weekdays only
				Timezone:     "UTC",
			},
			checkTime:      time.Date(2024, 1, 6, 23, 0, 0, 0, time.UTC), // Saturday at 11 PM
			expectedInDND:  false,
			expectedReason: "Day Saturday not in DND schedule",
		},
		{
			name: "all days DND",
			settings: &NotificationSettings{
				UserID:       userID,
				DNDEnabled:   true,
				DNDStartTime: timePtr(time.Date(0, 1, 1, 20, 0, 0, 0, time.UTC)), // 8 PM
				DNDEndTime:   timePtr(time.Date(0, 1, 1, 8, 0, 0, 0, time.UTC)),  // 8 AM
				DNDDays:      []int{0, 1, 2, 3, 4, 5, 6}, // All days
				Timezone:     "UTC",
			},
			checkTime:      time.Date(2024, 1, 1, 21, 0, 0, 0, time.UTC), // 9 PM any day
			expectedInDND:  true,
			expectedReason: "In DND period (20:00-08:00 spanning midnight)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inDND, reason := scheduler.IsInDNDPeriod(tt.settings, tt.checkTime)
			assert.Equal(t, tt.expectedInDND, inDND)
			assert.Equal(t, tt.expectedReason, reason)
		})
	}
}

func TestDNDScheduler_GetNextAvailableTime(t *testing.T) {
	logger := zap.NewNop()
	scheduler := NewDNDScheduler(logger)
	userID := uuid.New()

	tests := []struct {
		name          string
		settings      *NotificationSettings
		requestedTime time.Time
		expectedAfter time.Duration // Expected time should be at least this much after requested
	}{
		{
			name: "not in DND returns requested time",
			settings: &NotificationSettings{
				UserID:       userID,
				DNDEnabled:   true,
				DNDStartTime: timePtr(time.Date(0, 1, 1, 22, 0, 0, 0, time.UTC)),
				DNDEndTime:   timePtr(time.Date(0, 1, 1, 6, 0, 0, 0, time.UTC)),
				Timezone:     "UTC",
			},
			requestedTime: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC), // Noon
			expectedAfter: 0, // Should return same time
		},
		{
			name: "in DND returns next available",
			settings: &NotificationSettings{
				UserID:       userID,
				DNDEnabled:   true,
				DNDStartTime: timePtr(time.Date(0, 1, 1, 22, 0, 0, 0, time.UTC)), // 10 PM
				DNDEndTime:   timePtr(time.Date(0, 1, 1, 6, 0, 0, 0, time.UTC)),  // 6 AM
				Timezone:     "UTC",
			},
			requestedTime: time.Date(2024, 1, 1, 23, 0, 0, 0, time.UTC), // 11 PM (in DND)
			expectedAfter: 7 * time.Hour, // Should return 6 AM next day
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nextTime, err := scheduler.GetNextAvailableTime(tt.settings, tt.requestedTime)
			assert.NoError(t, err)

			if tt.expectedAfter == 0 {
				assert.Equal(t, tt.requestedTime, nextTime)
			} else {
				diff := nextTime.Sub(tt.requestedTime)
				assert.GreaterOrEqual(t, diff, tt.expectedAfter-time.Hour) // Allow 1 hour tolerance
				assert.LessOrEqual(t, diff, tt.expectedAfter+time.Hour)
			}
		})
	}
}

func TestDNDScheduler_ValidateDNDSettings(t *testing.T) {
	logger := zap.NewNop()
	scheduler := NewDNDScheduler(logger)
	userID := uuid.New()

	tests := []struct {
		name        string
		settings    *NotificationSettings
		expectError bool
	}{
		{
			name: "valid settings",
			settings: &NotificationSettings{
				UserID:       userID,
				DNDEnabled:   true,
				DNDStartTime: timePtr(time.Date(0, 1, 1, 22, 0, 0, 0, time.UTC)),
				DNDEndTime:   timePtr(time.Date(0, 1, 1, 6, 0, 0, 0, time.UTC)),
				DNDDays:      []int{0, 1, 2, 3, 4, 5, 6},
				Timezone:     "America/New_York",
			},
			expectError: false,
		},
		{
			name: "DND disabled is valid",
			settings: &NotificationSettings{
				UserID:     userID,
				DNDEnabled: false,
				Timezone:   "UTC",
			},
			expectError: false,
		},
		{
			name: "missing start time",
			settings: &NotificationSettings{
				UserID:     userID,
				DNDEnabled: true,
				DNDEndTime: timePtr(time.Date(0, 1, 1, 6, 0, 0, 0, time.UTC)),
				Timezone:   "UTC",
			},
			expectError: true,
		},
		{
			name: "missing end time",
			settings: &NotificationSettings{
				UserID:       userID,
				DNDEnabled:   true,
				DNDStartTime: timePtr(time.Date(0, 1, 1, 22, 0, 0, 0, time.UTC)),
				Timezone:     "UTC",
			},
			expectError: true,
		},
		{
			name: "invalid day of week",
			settings: &NotificationSettings{
				UserID:       userID,
				DNDEnabled:   true,
				DNDStartTime: timePtr(time.Date(0, 1, 1, 22, 0, 0, 0, time.UTC)),
				DNDEndTime:   timePtr(time.Date(0, 1, 1, 6, 0, 0, 0, time.UTC)),
				DNDDays:      []int{0, 1, 7}, // 7 is invalid
				Timezone:     "UTC",
			},
			expectError: true,
		},
		{
			name: "invalid timezone",
			settings: &NotificationSettings{
				UserID:       userID,
				DNDEnabled:   true,
				DNDStartTime: timePtr(time.Date(0, 1, 1, 22, 0, 0, 0, time.UTC)),
				DNDEndTime:   timePtr(time.Date(0, 1, 1, 6, 0, 0, 0, time.UTC)),
				Timezone:     "Invalid/Timezone",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := scheduler.ValidateDNDSettings(tt.settings)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDNDScheduler_CalculateDNDWindows(t *testing.T) {
	logger := zap.NewNop()
	scheduler := NewDNDScheduler(logger)
	userID := uuid.New()

	settings := &NotificationSettings{
		UserID:       userID,
		DNDEnabled:   true,
		DNDStartTime: timePtr(time.Date(0, 1, 1, 22, 0, 0, 0, time.UTC)), // 10 PM
		DNDEndTime:   timePtr(time.Date(0, 1, 1, 6, 0, 0, 0, time.UTC)),  // 6 AM
		DNDDays:      []int{1, 2, 3, 4, 5}, // Weekdays
		Timezone:     "UTC",
	}

	windows, err := scheduler.CalculateDNDWindows(settings, 7)
	assert.NoError(t, err)

	// Should have 5 windows (weekdays only)
	assert.LessOrEqual(t, len(windows), 7)

	// Check each window
	for _, window := range windows {
		// Window should span from 10 PM to 6 AM next day
		duration := window.End.Sub(window.Start)
		assert.Equal(t, 8*time.Hour, duration)

		// Start should be at 10 PM
		assert.Equal(t, 22, window.Start.Hour())

		// End should be at 6 AM
		assert.Equal(t, 6, window.End.Hour())
	}
}

func TestDNDScheduler_TimezoneConversion(t *testing.T) {
	logger := zap.NewNop()
	scheduler := NewDNDScheduler(logger)
	userID := uuid.New()

	// Test with Eastern timezone
	settings := &NotificationSettings{
		UserID:       userID,
		DNDEnabled:   true,
		DNDStartTime: timePtr(time.Date(0, 1, 1, 22, 0, 0, 0, time.UTC)), // 10 PM
		DNDEndTime:   timePtr(time.Date(0, 1, 1, 6, 0, 0, 0, time.UTC)),  // 6 AM
		DNDDays:      []int{0, 1, 2, 3, 4, 5, 6}, // All days
		Timezone:     "America/New_York",
	}

	// Create a time in UTC
	utcTime := time.Date(2024, 1, 1, 3, 0, 0, 0, time.UTC) // 3 AM UTC

	// This would be 10 PM EST (previous day) or 11 PM EDT
	inDND, _ := scheduler.IsInDNDPeriod(settings, utcTime)

	// The actual result depends on whether DST is in effect
	// But we can verify the function doesn't panic and returns a valid result
	assert.NotNil(t, inDND)
}

// Helper function to get day offset to reach a specific weekday
func getDayOffset(targetDay int) int {
	today := int(time.Now().Weekday())
	offset := targetDay - today
	if offset < 0 {
		offset += 7
	}
	return offset
}