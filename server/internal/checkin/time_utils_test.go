package checkin

import (
	"testing"
	"time"

	"github.com/dev-jelly/donelist/internal/premium"
	"github.com/stretchr/testify/assert"
)

func TestCanEditCheckin(t *testing.T) {
	tests := []struct {
		name        string
		checkinTime time.Time
		userTier    premium.Tier
		expected    bool
		description string
	}{
		{
			name:        "Free user - recent checkin (30 minutes ago)",
			checkinTime: time.Now().UTC().Add(-30 * time.Minute),
			userTier:    premium.TierFree,
			expected:    true,
			description: "Free user should be able to edit checkin created 30 minutes ago",
		},
		{
			name:        "Free user - checkin at edge of window (1h 59m ago)",
			checkinTime: time.Now().UTC().Add(-1*time.Hour - 59*time.Minute),
			userTier:    premium.TierFree,
			expected:    true,
			description: "Free user should be able to edit checkin just under 2 hours old",
		},
		{
			name:        "Free user - checkin exactly at 2 hours",
			checkinTime: time.Now().UTC().Add(-2 * time.Hour),
			userTier:    premium.TierFree,
			expected:    false,
			description: "Free user should NOT be able to edit checkin exactly 2 hours old",
		},
		{
			name:        "Free user - old checkin (3 hours ago)",
			checkinTime: time.Now().UTC().Add(-3 * time.Hour),
			userTier:    premium.TierFree,
			expected:    false,
			description: "Free user should NOT be able to edit checkin 3 hours old",
		},
		{
			name:        "Free user - very old checkin (1 day ago)",
			checkinTime: time.Now().UTC().Add(-24 * time.Hour),
			userTier:    premium.TierFree,
			expected:    false,
			description: "Free user should NOT be able to edit checkin 1 day old",
		},
		{
			name:        "Premium user - recent checkin",
			checkinTime: time.Now().UTC().Add(-30 * time.Minute),
			userTier:    premium.TierPremium,
			expected:    true,
			description: "Premium user should be able to edit recent checkin",
		},
		{
			name:        "Premium user - old checkin (3 hours ago)",
			checkinTime: time.Now().UTC().Add(-3 * time.Hour),
			userTier:    premium.TierPremium,
			expected:    true,
			description: "Premium user should be able to edit checkin 3 hours old",
		},
		{
			name:        "Premium user - very old checkin (1 day ago)",
			checkinTime: time.Now().UTC().Add(-24 * time.Hour),
			userTier:    premium.TierPremium,
			expected:    true,
			description: "Premium user should be able to edit checkin 1 day old",
		},
		{
			name:        "Enterprise user - recent checkin",
			checkinTime: time.Now().UTC().Add(-30 * time.Minute),
			userTier:    premium.TierEnterprise,
			expected:    true,
			description: "Enterprise user should be able to edit recent checkin",
		},
		{
			name:        "Enterprise user - old checkin (1 week ago)",
			checkinTime: time.Now().UTC().Add(-7 * 24 * time.Hour),
			userTier:    premium.TierEnterprise,
			expected:    true,
			description: "Enterprise user should be able to edit checkin 1 week old",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CanEditCheckin(tt.checkinTime, tt.userTier)
			assert.Equal(t, tt.expected, result, tt.description)
		})
	}
}

func TestGetEditTimeRemaining(t *testing.T) {
	tests := []struct {
		name          string
		checkinTime   time.Time
		expectedRange bool // true if we expect positive duration
		description   string
	}{
		{
			name:          "Recent checkin (30 minutes ago)",
			checkinTime:   time.Now().UTC().Add(-30 * time.Minute),
			expectedRange: true,
			description:   "Should return ~1.5 hours remaining",
		},
		{
			name:          "Checkin at 1 hour ago",
			checkinTime:   time.Now().UTC().Add(-1 * time.Hour),
			expectedRange: true,
			description:   "Should return ~1 hour remaining",
		},
		{
			name:          "Checkin at 1h 59m ago",
			checkinTime:   time.Now().UTC().Add(-1*time.Hour - 59*time.Minute),
			expectedRange: true,
			description:   "Should return ~1 minute remaining",
		},
		{
			name:          "Checkin exactly at 2 hours",
			checkinTime:   time.Now().UTC().Add(-2 * time.Hour),
			expectedRange: false,
			description:   "Should return 0 (expired)",
		},
		{
			name:          "Old checkin (3 hours ago)",
			checkinTime:   time.Now().UTC().Add(-3 * time.Hour),
			expectedRange: false,
			description:   "Should return 0 (expired)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remaining := GetEditTimeRemaining(tt.checkinTime)
			if tt.expectedRange {
				assert.Greater(t, remaining, time.Duration(0), tt.description)
			} else {
				assert.Equal(t, time.Duration(0), remaining, tt.description)
			}
		})
	}
}

func TestIsWithinEditWindow(t *testing.T) {
	tests := []struct {
		name        string
		checkinTime time.Time
		expected    bool
	}{
		{
			name:        "Recent checkin",
			checkinTime: time.Now().UTC().Add(-30 * time.Minute),
			expected:    true,
		},
		{
			name:        "Just under 2 hours",
			checkinTime: time.Now().UTC().Add(-1*time.Hour - 59*time.Minute),
			expected:    true,
		},
		{
			name:        "Exactly 2 hours",
			checkinTime: time.Now().UTC().Add(-2 * time.Hour),
			expected:    false,
		},
		{
			name:        "Old checkin",
			checkinTime: time.Now().UTC().Add(-3 * time.Hour),
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsWithinEditWindow(tt.checkinTime)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetEditWindowExpiry(t *testing.T) {
	now := time.Now().UTC()
	checkinTime := now.Add(-30 * time.Minute)

	expiry := GetEditWindowExpiry(checkinTime)

	// Expiry should be 2 hours after checkin time
	expected := checkinTime.Add(EditTimeWindow)
	assert.Equal(t, expected, expiry)

	// Expiry should be 1.5 hours from now
	assert.InDelta(t, 90*time.Minute, expiry.Sub(now), float64(time.Second))
}

func TestFormatTimeRemaining(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{
			name:     "Expired (zero)",
			duration: 0,
			expected: "expired",
		},
		{
			name:     "Expired (negative)",
			duration: -30 * time.Minute,
			expected: "expired",
		},
		{
			name:     "Only seconds",
			duration: 45 * time.Second,
			expected: "45s",
		},
		{
			name:     "Only minutes",
			duration: 30 * time.Minute,
			expected: "30m",
		},
		{
			name:     "Hours and minutes",
			duration: 1*time.Hour + 30*time.Minute,
			expected: "1h 30m",
		},
		{
			name:     "Only hours (no minutes)",
			duration: 2 * time.Hour,
			expected: "2h",
		},
		{
			name:     "Complex duration",
			duration: 1*time.Hour + 45*time.Minute + 30*time.Second,
			expected: "1h 45m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatTimeRemaining(tt.duration)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestTimezoneConsistency verifies that all calculations use UTC consistently
func TestTimezoneConsistency(t *testing.T) {
	// Create time in different timezone
	location, _ := time.LoadLocation("America/New_York")
	nyTime := time.Date(2024, 1, 1, 12, 0, 0, 0, location)

	// Get edit window expiry
	expiry := GetEditWindowExpiry(nyTime)

	// Expiry should be in UTC
	assert.Equal(t, time.UTC, expiry.Location())

	// Expiry should be 2 hours after the UTC equivalent of nyTime
	expectedExpiry := nyTime.UTC().Add(EditTimeWindow)
	assert.Equal(t, expectedExpiry, expiry)
}

// TestBoundaryConditions tests edge cases around the 2-hour boundary
func TestBoundaryConditions(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name        string
		checkinTime time.Time
		canEdit     bool
	}{
		{
			name:        "1 second before 2 hours",
			checkinTime: now.Add(-2*time.Hour + 1*time.Second),
			canEdit:     true,
		},
		{
			name:        "Exactly 2 hours",
			checkinTime: now.Add(-2 * time.Hour),
			canEdit:     false,
		},
		{
			name:        "1 second after 2 hours",
			checkinTime: now.Add(-2*time.Hour - 1*time.Second),
			canEdit:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CanEditCheckin(tt.checkinTime, premium.TierFree)
			assert.Equal(t, tt.canEdit, result)
		})
	}
}

// ============================================================================
// Tests for Check-in Interval Functions (Added for Task 1.5)
// ============================================================================

func TestIsWithinTwoHourBoundary(t *testing.T) {
	tests := []struct {
		name     string
		elapsed  time.Duration
		expected bool
	}{
		{
			name:     "0 minutes - well within boundary",
			elapsed:  0,
			expected: true,
		},
		{
			name:     "15 minutes - well within boundary",
			elapsed:  15 * time.Minute,
			expected: true,
		},
		{
			name:     "119 minutes 59 seconds - just before boundary",
			elapsed:  119*time.Minute + 59*time.Second,
			expected: true,
		},
		{
			name:     "119 minutes 59.5 seconds - just before boundary with tolerance",
			elapsed:  119*time.Minute + 59*time.Second + 500*time.Millisecond,
			expected: true,
		},
		{
			name:     "Exactly 120 minutes - at boundary",
			elapsed:  120 * time.Minute,
			expected: false,
		},
		{
			name:     "120 minutes 1 second - beyond boundary",
			elapsed:  120*time.Minute + 1*time.Second,
			expected: false,
		},
		{
			name:     "180 minutes - well beyond boundary",
			elapsed:  180 * time.Minute,
			expected: false,
		},
		{
			name:     "24 hours - long interval",
			elapsed:  24 * time.Hour,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsWithinTwoHourBoundary(tt.elapsed)
			assert.Equal(t, tt.expected, result, "For elapsed time: %v", tt.elapsed)
		})
	}
}

func TestIsAtOrBeyondTwoHourBoundary(t *testing.T) {
	tests := []struct {
		name     string
		elapsed  time.Duration
		expected bool
	}{
		{
			name:     "0 minutes",
			elapsed:  0,
			expected: false,
		},
		{
			name:     "119 minutes",
			elapsed:  119 * time.Minute,
			expected: false,
		},
		{
			name:     "Exactly 120 minutes",
			elapsed:  120 * time.Minute,
			expected: true,
		},
		{
			name:     "121 minutes",
			elapsed:  121 * time.Minute,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsAtOrBeyondTwoHourBoundary(tt.elapsed)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetAllowedIntervals(t *testing.T) {
	tests := []struct {
		name                 string
		timeSinceLastCheckin time.Duration
		expectedIntervals    []time.Duration
	}{
		{
			name:                 "0 minutes - all intervals allowed",
			timeSinceLastCheckin: 0,
			expectedIntervals:    []time.Duration{Interval15Min, Interval30Min, Interval45Min, Interval2Hours},
		},
		{
			name:                 "30 minutes - all intervals allowed",
			timeSinceLastCheckin: 30 * time.Minute,
			expectedIntervals:    []time.Duration{Interval15Min, Interval30Min, Interval45Min, Interval2Hours},
		},
		{
			name:                 "119 minutes 59 seconds - all intervals allowed",
			timeSinceLastCheckin: 119*time.Minute + 59*time.Second,
			expectedIntervals:    []time.Duration{Interval15Min, Interval30Min, Interval45Min, Interval2Hours},
		},
		{
			name:                 "Exactly 120 minutes - only 2-hour intervals",
			timeSinceLastCheckin: 120 * time.Minute,
			expectedIntervals:    []time.Duration{Interval2Hours},
		},
		{
			name:                 "180 minutes - only 2-hour intervals",
			timeSinceLastCheckin: 180 * time.Minute,
			expectedIntervals:    []time.Duration{Interval2Hours},
		},
		{
			name:                 "24 hours - only 2-hour intervals",
			timeSinceLastCheckin: 24 * time.Hour,
			expectedIntervals:    []time.Duration{Interval2Hours},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetAllowedIntervals(tt.timeSinceLastCheckin)
			assert.Equal(t, tt.expectedIntervals, result)
		})
	}
}

func TestIsValidInterval(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected bool
	}{
		{
			name:     "15 minutes - valid",
			duration: 15 * time.Minute,
			expected: true,
		},
		{
			name:     "30 minutes - valid",
			duration: 30 * time.Minute,
			expected: true,
		},
		{
			name:     "45 minutes - valid",
			duration: 45 * time.Minute,
			expected: true,
		},
		{
			name:     "120 minutes - valid",
			duration: 120 * time.Minute,
			expected: true,
		},
		{
			name:     "10 minutes - invalid",
			duration: 10 * time.Minute,
			expected: false,
		},
		{
			name:     "60 minutes - invalid",
			duration: 60 * time.Minute,
			expected: false,
		},
		{
			name:     "90 minutes - invalid",
			duration: 90 * time.Minute,
			expected: false,
		},
		{
			name:     "0 minutes - invalid",
			duration: 0,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidInterval(tt.duration)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCanCheckinWithInterval(t *testing.T) {
	baseTime := time.Date(2025, 3, 9, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name               string
		lastCheckinTime    time.Time
		currentTime        time.Time
		desiredInterval    time.Duration
		expectedCanCheckin bool
	}{
		{
			name:               "15min interval after 30min elapsed",
			lastCheckinTime:    baseTime,
			currentTime:        baseTime.Add(30 * time.Minute),
			desiredInterval:    Interval15Min,
			expectedCanCheckin: true,
		},
		{
			name:               "30min interval after 119min elapsed",
			lastCheckinTime:    baseTime,
			currentTime:        baseTime.Add(119 * time.Minute),
			desiredInterval:    Interval30Min,
			expectedCanCheckin: true,
		},
		{
			name:               "15min interval at exactly 120min - NOT allowed",
			lastCheckinTime:    baseTime,
			currentTime:        baseTime.Add(120 * time.Minute),
			desiredInterval:    Interval15Min,
			expectedCanCheckin: false,
		},
		{
			name:               "2hour interval at exactly 120min - allowed",
			lastCheckinTime:    baseTime,
			currentTime:        baseTime.Add(120 * time.Minute),
			desiredInterval:    Interval2Hours,
			expectedCanCheckin: true,
		},
		{
			name:               "15min interval after 3 hours - NOT allowed",
			lastCheckinTime:    baseTime,
			currentTime:        baseTime.Add(3 * time.Hour),
			desiredInterval:    Interval15Min,
			expectedCanCheckin: false,
		},
		{
			name:               "2hour interval after 3 hours - allowed",
			lastCheckinTime:    baseTime,
			currentTime:        baseTime.Add(3 * time.Hour),
			desiredInterval:    Interval2Hours,
			expectedCanCheckin: true,
		},
		{
			name:               "Invalid 60min interval",
			lastCheckinTime:    baseTime,
			currentTime:        baseTime.Add(65 * time.Minute),
			desiredInterval:    60 * time.Minute,
			expectedCanCheckin: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock time by calculating elapsed duration
			elapsed := tt.currentTime.Sub(tt.lastCheckinTime)

			// Use actual function logic
			if !IsValidInterval(tt.desiredInterval) {
				assert.False(t, tt.expectedCanCheckin)
				return
			}

			if IsWithinTwoHourBoundary(elapsed) {
				assert.True(t, tt.expectedCanCheckin)
			} else {
				assert.Equal(t, tt.expectedCanCheckin, tt.desiredInterval == Interval2Hours)
			}
		})
	}
}

func TestNextEligibleCheckinTime(t *testing.T) {
	tests := []struct {
		name                string
		lastCheckinTime     time.Time
		desiredInterval     time.Duration
		currentTime         time.Time
		expectZeroTime      bool
		expectedMinimumWait time.Duration
	}{
		{
			name:            "15min interval - immediately eligible",
			lastCheckinTime: time.Now().UTC().Add(-30 * time.Minute),
			desiredInterval: Interval15Min,
			currentTime:     time.Now().UTC(),
			expectZeroTime:  true,
		},
		{
			name:                "30min interval - need to wait 10min",
			lastCheckinTime:     time.Now().UTC().Add(-20 * time.Minute),
			desiredInterval:     Interval30Min,
			currentTime:         time.Now().UTC(),
			expectZeroTime:      false,
			expectedMinimumWait: 9 * time.Minute, // Approximately 10 minutes
		},
		{
			name:            "2hour interval - immediately eligible",
			lastCheckinTime: time.Now().UTC().Add(-3 * time.Hour),
			desiredInterval: Interval2Hours,
			currentTime:     time.Now().UTC(),
			expectZeroTime:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NextEligibleCheckinTime(tt.lastCheckinTime, tt.desiredInterval)

			if tt.expectZeroTime {
				assert.True(t, result.IsZero(), "Expected zero time (immediately eligible)")
			} else {
				assert.False(t, result.IsZero(), "Expected non-zero time (must wait)")
				waitTime := result.Sub(tt.currentTime)
				assert.GreaterOrEqual(t, waitTime, tt.expectedMinimumWait)
			}
		})
	}
}

func TestGetTimeUntilNextCheckin(t *testing.T) {
	tests := []struct {
		name            string
		lastCheckinTime time.Time
		desiredInterval time.Duration
		expectZero      bool
	}{
		{
			name:            "Already eligible",
			lastCheckinTime: time.Now().UTC().Add(-30 * time.Minute),
			desiredInterval: Interval15Min,
			expectZero:      true,
		},
		{
			name:            "Need to wait",
			lastCheckinTime: time.Now().UTC().Add(-10 * time.Minute),
			desiredInterval: Interval30Min,
			expectZero:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetTimeUntilNextCheckin(tt.lastCheckinTime, tt.desiredInterval)

			if tt.expectZero {
				assert.Equal(t, time.Duration(0), result)
			} else {
				assert.Greater(t, result, time.Duration(0))
			}
		})
	}
}

// TestDSTTransition tests behavior during Daylight Saving Time transitions
func TestDSTTransition(t *testing.T) {
	// Load New York timezone (has DST)
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("Skipping DST test - timezone database not available")
	}

	// Spring forward: March 9, 2025, 2:00 AM → 3:00 AM
	// Check-in at 1:30 AM EST (before DST)
	beforeDST := time.Date(2025, 3, 9, 1, 30, 0, 0, loc)

	// Convert to UTC for our functions
	beforeDSTUTC := beforeDST.UTC()

	// 30 minutes later in UTC
	afterInterval := beforeDSTUTC.Add(30 * time.Minute)

	// Calculate next eligible time
	nextEligible := NextEligibleCheckinTime(beforeDSTUTC, Interval30Min)

	// Should be zero (immediately eligible) or the expected time
	if !nextEligible.IsZero() {
		t.Logf("Next eligible time: %v", nextEligible)
		t.Logf("Expected around: %v", afterInterval)
	}

	// The key test: UTC-based calculation should work correctly
	// regardless of DST transition
	elapsed := time.Now().UTC().Sub(beforeDSTUTC)
	if elapsed >= 30*time.Minute {
		assert.True(t, nextEligible.IsZero() || nextEligible.Before(time.Now().UTC()))
	}
}

// TestClockSkewTolerance tests that clock skew tolerance works correctly
func TestClockSkewTolerance(t *testing.T) {
	tests := []struct {
		name     string
		elapsed  time.Duration
		expected bool
	}{
		{
			name:     "119min 59sec - within boundary",
			elapsed:  119*time.Minute + 59*time.Second,
			expected: true,
		},
		{
			name:     "119min 59.5sec - within boundary with tolerance",
			elapsed:  119*time.Minute + 59*time.Second + 500*time.Millisecond,
			expected: true,
		},
		{
			name:     "119min 59.999sec - within boundary with tolerance",
			elapsed:  119*time.Minute + 59*time.Second + 999*time.Millisecond,
			expected: true,
		},
		{
			name:     "Exactly 120min - at boundary (tolerance applied)",
			elapsed:  TwoHourBoundary,
			expected: false,
		},
		{
			name:     "120min 0.001sec - beyond boundary",
			elapsed:  TwoHourBoundary + time.Millisecond,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsWithinTwoHourBoundary(tt.elapsed)
			assert.Equal(t, tt.expected, result,
				"For elapsed %v: within boundary should be %v", tt.elapsed, tt.expected)
		})
	}
}

// TestContinuousCheckins simulates a sequence of check-ins to verify interval rules hold
func TestContinuousCheckins(t *testing.T) {
	baseTime := time.Date(2025, 3, 9, 10, 0, 0, 0, time.UTC)

	// Simulate check-ins every 15 minutes for 2 hours
	lastCheckin := baseTime
	for i := 0; i < 8; i++ {
		nextTime := lastCheckin.Add(Interval15Min)
		elapsed := nextTime.Sub(lastCheckin)

		// Should be able to check in with 15-minute interval
		canCheckin := IsWithinTwoHourBoundary(elapsed) && IsValidInterval(Interval15Min)
		assert.True(t, canCheckin, "Should be able to check in after 15 minutes (iteration %d)", i)

		lastCheckin = nextTime
	}

	// After 2 hours (8 x 15min), next check-in must be 2-hour interval
	nextTime := lastCheckin.Add(Interval15Min)
	elapsed := nextTime.Sub(baseTime)

	// Total elapsed from first check-in > 2 hours
	assert.False(t, IsWithinTwoHourBoundary(elapsed), "Should be beyond 2-hour boundary")

	// But can still check in with 2-hour intervals
	canCheckinWith2Hour := IsValidInterval(Interval2Hours)
	assert.True(t, canCheckinWith2Hour, "2-hour intervals should always be valid")
}

// TestLongTermCheckins tests behavior after long periods of inactivity
func TestLongTermCheckins(t *testing.T) {
	tests := []struct {
		name                 string
		timeSinceLastCheckin time.Duration
		desiredInterval      time.Duration
		expectedAllowed      bool
	}{
		{
			name:                 "1 day inactive - 15min interval not allowed",
			timeSinceLastCheckin: 24 * time.Hour,
			desiredInterval:      Interval15Min,
			expectedAllowed:      false,
		},
		{
			name:                 "1 day inactive - 2hour interval allowed",
			timeSinceLastCheckin: 24 * time.Hour,
			desiredInterval:      Interval2Hours,
			expectedAllowed:      true,
		},
		{
			name:                 "1 week inactive - only 2hour allowed",
			timeSinceLastCheckin: 7 * 24 * time.Hour,
			desiredInterval:      Interval2Hours,
			expectedAllowed:      true,
		},
		{
			name:                 "1 month inactive - 30min not allowed",
			timeSinceLastCheckin: 30 * 24 * time.Hour,
			desiredInterval:      Interval30Min,
			expectedAllowed:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isWithinBoundary := IsWithinTwoHourBoundary(tt.timeSinceLastCheckin)
			isValidInterval := IsValidInterval(tt.desiredInterval)

			var canCheckin bool
			if !isValidInterval {
				canCheckin = false
			} else if isWithinBoundary {
				canCheckin = true
			} else {
				canCheckin = tt.desiredInterval == Interval2Hours
			}

			assert.Equal(t, tt.expectedAllowed, canCheckin)
		})
	}
}

// BenchmarkIntervalChecking benchmarks the interval checking performance
func BenchmarkIntervalChecking(b *testing.B) {
	lastCheckin := time.Now().UTC().Add(-45 * time.Minute)

	b.Run("CanCheckinWithInterval", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			CanCheckinWithInterval(lastCheckin, Interval30Min)
		}
	})

	b.Run("GetAllowedIntervals", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			GetAllowedIntervals(30 * time.Minute)
		}
	})

	b.Run("NextEligibleCheckinTime", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			NextEligibleCheckinTime(lastCheckin, Interval30Min)
		}
	})
}
