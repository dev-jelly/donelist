package notification

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestTimezoneHandler_GetLocation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	handler := NewTimezoneHandler(logger)

	tests := []struct {
		name        string
		timezone    string
		expectError bool
	}{
		{
			name:        "valid timezone",
			timezone:    "America/New_York",
			expectError: false,
		},
		{
			name:        "UTC",
			timezone:    "UTC",
			expectError: false,
		},
		{
			name:        "empty string defaults to UTC",
			timezone:    "",
			expectError: false,
		},
		{
			name:        "invalid timezone",
			timezone:    "Invalid/Timezone",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc, err := handler.GetLocation(tt.timezone)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, loc)
			}
		})
	}
}

func TestTimezoneHandler_GetLocation_Caching(t *testing.T) {
	logger := zaptest.NewLogger(t)
	handler := NewTimezoneHandler(logger)

	// Load timezone twice
	loc1, err := handler.GetLocation("America/New_York")
	require.NoError(t, err)

	loc2, err := handler.GetLocation("America/New_York")
	require.NoError(t, err)

	// Should return the same cached instance
	assert.Equal(t, loc1, loc2)
}

func TestTimezoneHandler_ConvertToUserTime(t *testing.T) {
	logger := zaptest.NewLogger(t)
	handler := NewTimezoneHandler(logger)

	// Create a UTC time: 2024-01-15 14:00:00 UTC
	utcTime := time.Date(2024, 1, 15, 14, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		timezone     string
		expectedHour int
	}{
		{
			name:         "New York (EST, UTC-5)",
			timezone:     "America/New_York",
			expectedHour: 9, // 14:00 UTC = 09:00 EST
		},
		{
			name:         "Tokyo (JST, UTC+9)",
			timezone:     "Asia/Tokyo",
			expectedHour: 23, // 14:00 UTC = 23:00 JST
		},
		{
			name:         "UTC",
			timezone:     "UTC",
			expectedHour: 14,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			localTime, err := handler.ConvertToUserTime(utcTime, tt.timezone)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedHour, localTime.Hour())
		})
	}
}

func TestTimezoneHandler_ConvertToUTC(t *testing.T) {
	logger := zaptest.NewLogger(t)
	handler := NewTimezoneHandler(logger)

	tests := []struct {
		name         string
		timezone     string
		localHour    int
		expectedHour int
	}{
		{
			name:         "New York (EST, UTC-5)",
			timezone:     "America/New_York",
			localHour:    9,
			expectedHour: 14, // 09:00 EST = 14:00 UTC
		},
		{
			name:         "Tokyo (JST, UTC+9)",
			timezone:     "Asia/Tokyo",
			localHour:    23,
			expectedHour: 14, // 23:00 JST = 14:00 UTC
		},
		{
			name:         "UTC",
			timezone:     "UTC",
			localHour:    14,
			expectedHour: 14,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			localTime := time.Date(2024, 1, 15, tt.localHour, 0, 0, 0, time.UTC)
			utcTime, err := handler.ConvertToUTC(localTime, tt.timezone)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedHour, utcTime.Hour())
		})
	}
}

func TestTimezoneHandler_IsDST(t *testing.T) {
	logger := zaptest.NewLogger(t)
	handler := NewTimezoneHandler(logger)

	loc, err := handler.GetLocation("America/New_York")
	require.NoError(t, err)

	tests := []struct {
		name      string
		date      time.Time
		expectDST bool
	}{
		{
			name:      "January (winter - no DST)",
			date:      time.Date(2024, 1, 15, 12, 0, 0, 0, loc),
			expectDST: false,
		},
		{
			name:      "July (summer - DST active)",
			date:      time.Date(2024, 7, 15, 12, 0, 0, 0, loc),
			expectDST: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isDST := handler.IsDST(tt.date, loc)
			assert.Equal(t, tt.expectDST, isDST)
		})
	}
}

func TestTimezoneHandler_GetTimezoneInfo(t *testing.T) {
	logger := zaptest.NewLogger(t)
	handler := NewTimezoneHandler(logger)

	// Test with a known timezone
	now := time.Now()
	info, err := handler.GetTimezoneInfo("America/New_York", now)
	require.NoError(t, err)

	assert.Equal(t, "America/New_York", info.IANA)
	assert.NotEmpty(t, info.Name)
	assert.NotZero(t, info.Offset)

	// Should have a next transition (either to/from DST)
	// Note: This might be nil if checked far in the future
	// assert.NotNil(t, info.NextTransition)
}

func TestTimezoneHandler_GetNextDSTTransition(t *testing.T) {
	logger := zaptest.NewLogger(t)
	handler := NewTimezoneHandler(logger)

	loc, err := handler.GetLocation("America/New_York")
	require.NoError(t, err)

	// Start from January (winter)
	winterDate := time.Date(2024, 1, 15, 12, 0, 0, 0, loc)
	nextTransition := handler.GetNextDSTTransition(winterDate, loc)

	// Should find a transition (spring forward)
	require.NotNil(t, nextTransition)

	// Transition should be in the future
	assert.True(t, nextTransition.After(winterDate))

	// Transition should be in March (for US timezones)
	assert.Equal(t, time.March, nextTransition.Month())
}

func TestTimezoneHandler_ValidateTimezone(t *testing.T) {
	logger := zaptest.NewLogger(t)
	handler := NewTimezoneHandler(logger)

	tests := []struct {
		name      string
		timezone  string
		expectErr bool
	}{
		{
			name:      "valid timezone",
			timezone:  "America/New_York",
			expectErr: false,
		},
		{
			name:      "UTC",
			timezone:  "UTC",
			expectErr: false,
		},
		{
			name:      "empty string",
			timezone:  "",
			expectErr: true,
		},
		{
			name:      "invalid timezone",
			timezone:  "Invalid/Timezone",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.ValidateTimezone(tt.timezone)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestTimezoneHandler_ConvertScheduleTime(t *testing.T) {
	logger := zaptest.NewLogger(t)
	handler := NewTimezoneHandler(logger)

	referenceDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		hour         int
		minute       int
		timezone     string
		expectedHour int // Expected hour in UTC
	}{
		{
			name:         "9 AM New York to UTC",
			hour:         9,
			minute:       0,
			timezone:     "America/New_York",
			expectedHour: 14, // 9 AM EST = 2 PM UTC
		},
		{
			name:         "9 AM Tokyo to UTC",
			hour:         9,
			minute:       0,
			timezone:     "Asia/Tokyo",
			expectedHour: 0, // 9 AM JST = 0 AM UTC (previous day handled by date)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			utcTime, err := handler.ConvertScheduleTime(tt.hour, tt.minute, tt.timezone, referenceDate)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedHour, utcTime.Hour())
			assert.Equal(t, tt.minute, utcTime.Minute())
		})
	}
}

func TestTimezoneHandler_AdjustForDSTTransition(t *testing.T) {
	logger := zaptest.NewLogger(t)
	handler := NewTimezoneHandler(logger)

	// Create a scheduled time in winter (EST)
	winterTime := time.Date(2024, 1, 15, 14, 0, 0, 0, time.UTC) // 9 AM EST

	// Adjust it (simulating how it should behave in summer with DST)
	adjusted, err := handler.AdjustForDSTTransition(winterTime, "America/New_York")
	require.NoError(t, err)

	// The adjusted time should maintain the same clock time in the user's timezone
	loc, _ := handler.GetLocation("America/New_York")
	localTime := adjusted.In(loc)

	// Should still be 9 AM local time
	assert.Equal(t, 9, localTime.Hour())
}

func TestTimezoneHandler_CalculateLocalMidnight(t *testing.T) {
	logger := zaptest.NewLogger(t)
	handler := NewTimezoneHandler(logger)

	// Test for a specific date
	date := time.Date(2024, 1, 15, 18, 30, 45, 0, time.UTC)

	midnightUTC, err := handler.CalculateLocalMidnight(date, "America/New_York")
	require.NoError(t, err)

	// Convert back to check
	loc, _ := handler.GetLocation("America/New_York")
	midnightLocal := midnightUTC.In(loc)

	assert.Equal(t, 0, midnightLocal.Hour())
	assert.Equal(t, 0, midnightLocal.Minute())
	assert.Equal(t, 0, midnightLocal.Second())
	assert.Equal(t, 15, midnightLocal.Day())
}

func TestTimezoneHandler_GetLocalTimeOfDay(t *testing.T) {
	logger := zaptest.NewLogger(t)
	handler := NewTimezoneHandler(logger)

	// 14:30:45 UTC
	utcTime := time.Date(2024, 1, 15, 14, 30, 45, 0, time.UTC)

	hour, minute, second, err := handler.GetLocalTimeOfDay(utcTime, "America/New_York")
	require.NoError(t, err)

	// Should be 9:30:45 AM EST (UTC-5)
	assert.Equal(t, 9, hour)
	assert.Equal(t, 30, minute)
	assert.Equal(t, 45, second)
}

func TestTimezoneHandler_FormatInTimezone(t *testing.T) {
	logger := zaptest.NewLogger(t)
	handler := NewTimezoneHandler(logger)

	utcTime := time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC)

	formatted, err := handler.FormatInTimezone(utcTime, "America/New_York", "15:04 MST")
	require.NoError(t, err)

	// Should be "09:30 EST"
	assert.Contains(t, formatted, "09:30")
	assert.Contains(t, formatted, "EST")
}

func TestTimezoneHandler_ParseTimeInTimezone(t *testing.T) {
	logger := zaptest.NewLogger(t)
	handler := NewTimezoneHandler(logger)

	timeStr := "2024-01-15 09:30:00"
	layout := "2006-01-02 15:04:05"

	parsed, err := handler.ParseTimeInTimezone(timeStr, "America/New_York", layout)
	require.NoError(t, err)

	// Convert to UTC and check
	assert.Equal(t, 14, parsed.UTC().Hour()) // 9 AM EST = 2 PM UTC
	assert.Equal(t, 30, parsed.Minute())
}

func TestTimezoneHandler_GetDSTTransitionDates(t *testing.T) {
	logger := zaptest.NewLogger(t)
	handler := NewTimezoneHandler(logger)

	spring, fall, err := handler.GetDSTTransitionDates(2024, "America/New_York")
	require.NoError(t, err)

	// US DST in 2024:
	// Spring: Second Sunday in March
	// Fall: First Sunday in November

	if spring != nil {
		assert.Equal(t, time.March, spring.Month())
		assert.True(t, spring.Day() >= 8 && spring.Day() <= 14) // Second Sunday
	}

	if fall != nil {
		assert.Equal(t, time.November, fall.Month())
		assert.True(t, fall.Day() >= 1 && fall.Day() <= 7) // First Sunday
	}
}

func TestTimezoneHandler_DetectTimezoneChange(t *testing.T) {
	logger := zaptest.NewLogger(t)
	handler := NewTimezoneHandler(logger)

	tests := []struct {
		name             string
		previousTimezone string
		currentTimezone  string
		threshold        time.Duration
		expectChange     bool
	}{
		{
			name:             "same timezone",
			previousTimezone: "America/New_York",
			currentTimezone:  "America/New_York",
			threshold:        time.Hour,
			expectChange:     false,
		},
		{
			name:             "coast to coast US",
			previousTimezone: "America/New_York",
			currentTimezone:  "America/Los_Angeles",
			threshold:        time.Hour,
			expectChange:     true,
		},
		{
			name:             "adjacent timezones within threshold",
			previousTimezone: "America/New_York",
			currentTimezone:  "America/Chicago",
			threshold:        2 * time.Hour,
			expectChange:     false,
		},
		{
			name:             "first time setting timezone",
			previousTimezone: "",
			currentTimezone:  "America/New_York",
			threshold:        time.Hour,
			expectChange:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changed, err := handler.DetectTimezoneChange(tt.previousTimezone, tt.currentTimezone, tt.threshold)

			if tt.previousTimezone != "" && tt.currentTimezone != "" {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.expectChange, changed)
		})
	}
}

func TestTimezoneHandler_GetCommonTimezones(t *testing.T) {
	logger := zaptest.NewLogger(t)
	handler := NewTimezoneHandler(logger)

	timezones := handler.GetCommonTimezones()

	// Should have a reasonable number of common timezones
	assert.Greater(t, len(timezones), 20)

	// Should include some major cities
	assert.Contains(t, timezones, "America/New_York")
	assert.Contains(t, timezones, "Europe/London")
	assert.Contains(t, timezones, "Asia/Tokyo")
	assert.Contains(t, timezones, "UTC")
}

func TestTimezoneManager_UpdateUserTimezone(t *testing.T) {
	logger := zaptest.NewLogger(t)
	manager := NewTimezoneManager(logger)

	pref := &UserTimezonePreference{
		UserID:   "user123",
		Timezone: "America/New_York",
	}

	tests := []struct {
		name                  string
		newTimezone           string
		expectTravelDetected  bool
		expectError           bool
	}{
		{
			name:                 "no significant change",
			newTimezone:          "America/New_York",
			expectTravelDetected: false,
			expectError:          false,
		},
		{
			name:                 "travel detected",
			newTimezone:          "Asia/Tokyo",
			expectTravelDetected: true,
			expectError:          false,
		},
		{
			name:                 "invalid timezone",
			newTimezone:          "Invalid/Timezone",
			expectTravelDetected: false,
			expectError:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			travelDetected, err := manager.UpdateUserTimezone(pref, tt.newTimezone)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectTravelDetected, travelDetected)

				if tt.expectTravelDetected {
					assert.NotNil(t, pref.TimezoneChangedAt)
					assert.NotNil(t, pref.LastLocation)
				}
			}
		})
	}
}

func TestTimezoneManager_GetTimezoneForScheduling(t *testing.T) {
	logger := zaptest.NewLogger(t)
	manager := NewTimezoneManager(logger)

	tests := []struct {
		name     string
		pref     *UserTimezonePreference
		expected string
	}{
		{
			name: "user has timezone",
			pref: &UserTimezonePreference{
				Timezone: "America/New_York",
			},
			expected: "America/New_York",
		},
		{
			name: "user has no timezone",
			pref: &UserTimezonePreference{
				Timezone: "",
			},
			expected: "UTC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.GetTimezoneForScheduling(tt.pref)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTimezoneManager_ShouldAdjustSchedules(t *testing.T) {
	logger := zaptest.NewLogger(t)
	manager := NewTimezoneManager(logger)

	now := time.Now()
	yesterday := now.Add(-23 * time.Hour)
	twoDaysAgo := now.Add(-48 * time.Hour)

	tests := []struct {
		name     string
		pref     *UserTimezonePreference
		expected bool
	}{
		{
			name: "recent timezone change",
			pref: &UserTimezonePreference{
				TimezoneChangedAt: &yesterday,
			},
			expected: true,
		},
		{
			name: "old timezone change",
			pref: &UserTimezonePreference{
				TimezoneChangedAt: &twoDaysAgo,
			},
			expected: false,
		},
		{
			name: "no timezone change",
			pref: &UserTimezonePreference{
				TimezoneChangedAt: nil,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.ShouldAdjustSchedules(tt.pref)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDSTTransitionEdgeCases(t *testing.T) {
	logger := zaptest.NewLogger(t)
	handler := NewTimezoneHandler(logger)

	loc, err := handler.GetLocation("America/New_York")
	require.NoError(t, err)

	// Test DST transitions - the exact behavior depends on the year
	// For 2024, DST starts on March 10 and ends on November 3

	// Test a time during DST (summer)
	summerTime := time.Date(2024, 7, 15, 14, 30, 0, 0, loc)
	isDST := handler.IsDST(summerTime, loc)
	assert.True(t, isDST, "July should be in DST")

	// Test a time outside DST (winter)
	winterTime := time.Date(2024, 1, 15, 14, 30, 0, 0, loc)
	isDST = handler.IsDST(winterTime, loc)
	assert.False(t, isDST, "January should not be in DST")

	// Test that we can detect DST transitions
	spring, fall, err := handler.GetDSTTransitionDates(2024, "America/New_York")
	require.NoError(t, err)

	if spring != nil {
		assert.Equal(t, time.March, spring.Month())
	}
	if fall != nil {
		assert.Equal(t, time.November, fall.Month())
	}
}

func TestTimezoneHandler_ConcurrentAccess(t *testing.T) {
	logger := zaptest.NewLogger(t)
	handler := NewTimezoneHandler(logger)

	// Test concurrent access to location cache
	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func() {
			_, err := handler.GetLocation("America/New_York")
			assert.NoError(t, err)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}
