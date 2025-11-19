package timezone

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsValidTimezone(t *testing.T) {
	tests := []struct {
		name     string
		timezone string
		want     bool
	}{
		{
			name:     "Valid US Eastern",
			timezone: "America/New_York",
			want:     true,
		},
		{
			name:     "Valid UTC",
			timezone: "UTC",
			want:     true,
		},
		{
			name:     "Valid Asia Tokyo",
			timezone: "Asia/Tokyo",
			want:     true,
		},
		{
			name:     "Valid Europe London",
			timezone: "Europe/London",
			want:     true,
		},
		{
			name:     "Invalid timezone",
			timezone: "Invalid/Timezone",
			want:     false,
		},
		{
			name:     "Empty string",
			timezone: "",
			want:     false,
		},
		{
			name:     "Random string",
			timezone: "not-a-timezone",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidTimezone(tt.timezone)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidateTimezone(t *testing.T) {
	tests := []struct {
		name      string
		timezone  string
		wantError bool
	}{
		{
			name:      "Valid timezone",
			timezone:  "America/New_York",
			wantError: false,
		},
		{
			name:      "Empty timezone",
			timezone:  "",
			wantError: true,
		},
		{
			name:      "Too long timezone",
			timezone:  "A/Very/Long/Timezone/Identifier/That/Exceeds/One/Hundred/Characters/And/Should/Be/Rejected/By/Validation/Logic/Because/Its/Too/Long",
			wantError: true,
		},
		{
			name:      "Invalid IANA identifier",
			timezone:  "Invalid/Zone",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTimezone(tt.timezone)
			if tt.wantError {
				assert.Error(t, err)
				var validationErr *ValidationError
				assert.ErrorAs(t, err, &validationErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetTimezoneInfo(t *testing.T) {
	tests := []struct {
		name         string
		timezone     string
		wantError    bool
		checkOffset  bool
		checkGroup   bool
		expectedGroup string
	}{
		{
			name:          "America/New_York",
			timezone:      "America/New_York",
			wantError:     false,
			checkGroup:    true,
			expectedGroup: GroupAmericas,
		},
		{
			name:          "Europe/London",
			timezone:      "Europe/London",
			wantError:     false,
			checkGroup:    true,
			expectedGroup: GroupEurope,
		},
		{
			name:          "Asia/Tokyo",
			timezone:      "Asia/Tokyo",
			wantError:     false,
			checkGroup:    true,
			expectedGroup: GroupAsia,
		},
		{
			name:      "UTC",
			timezone:  "UTC",
			wantError: false,
		},
		{
			name:      "Invalid timezone",
			timezone:  "Invalid/Zone",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := GetTimezoneInfo(tt.timezone)

			if tt.wantError {
				assert.Error(t, err)
				assert.Nil(t, info)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, info)

			assert.Equal(t, tt.timezone, info.ID)
			assert.NotEmpty(t, info.DisplayName)
			assert.NotEmpty(t, info.OffsetString)
			assert.NotEmpty(t, info.Group)

			if tt.checkGroup {
				assert.Equal(t, tt.expectedGroup, info.Group)
			}
		})
	}
}

func TestGetNextDSTTransition(t *testing.T) {
	// Test with timezones that have DST
	t.Run("America/New_York has DST", func(t *testing.T) {
		transition, err := GetNextDSTTransition("America/New_York")
		require.NoError(t, err)

		// New York observes DST, so there should be a transition
		if transition != nil {
			assert.NotZero(t, transition.Time)
			assert.NotEqual(t, transition.OffsetBefore, transition.OffsetAfter)
			assert.NotEmpty(t, transition.Description)
		}
	})

	// Test with timezone that doesn't observe DST
	t.Run("Asia/Tokyo no DST", func(t *testing.T) {
		transition, err := GetNextDSTTransition("Asia/Tokyo")
		require.NoError(t, err)

		// Tokyo doesn't observe DST, so transition may be nil
		// This is acceptable
		if transition != nil {
			t.Logf("Unexpected transition found: %+v", transition)
		}
	})

	t.Run("Invalid timezone", func(t *testing.T) {
		transition, err := GetNextDSTTransition("Invalid/Zone")
		assert.Error(t, err)
		assert.Nil(t, transition)
	})
}

func TestDSTBoundaryCalculations(t *testing.T) {
	// Test DST transition edge cases
	t.Run("Spring forward edge case", func(t *testing.T) {
		// March 2024 DST transition in New York
		// Spring forward: 2:00 AM becomes 3:00 AM
		loc, err := time.LoadLocation("America/New_York")
		require.NoError(t, err)

		// Create a time just before the spring forward
		// In 2024, DST starts on March 10 at 2:00 AM
		beforeTransition := time.Date(2024, 3, 10, 1, 59, 0, 0, loc)
		afterTransition := time.Date(2024, 3, 10, 3, 1, 0, 0, loc)

		_, beforeOffset := beforeTransition.Zone()
		_, afterOffset := afterTransition.Zone()

		// Spring forward means offset increases (becomes less negative)
		assert.Greater(t, afterOffset, beforeOffset, "Spring forward should increase offset")

		// The difference should be 1 hour (3600 seconds)
		assert.Equal(t, 3600, afterOffset-beforeOffset, "DST offset change should be 1 hour")
	})

	t.Run("Fall back edge case", func(t *testing.T) {
		// November 2024 DST transition in New York
		// Fall back: 2:00 AM EDT becomes 1:00 AM EST
		loc, err := time.LoadLocation("America/New_York")
		require.NoError(t, err)

		// Create times before and after fall back
		// In 2024, DST ends on November 3 at 2:00 AM
		// Before: October 3, 1:59 AM EDT (still in DST)
		// After: November 3, 3:00 AM EST (after DST ends)
		beforeTransition := time.Date(2024, 11, 3, 1, 59, 0, 0, loc)
		afterTransition := time.Date(2024, 11, 3, 3, 0, 0, 0, loc)

		_, beforeOffset := beforeTransition.Zone()
		_, afterOffset := afterTransition.Zone()

		// Fall back means offset decreases (becomes more negative)
		// EDT is UTC-4 (-14400), EST is UTC-5 (-18000)
		assert.Less(t, afterOffset, beforeOffset, "Fall back should decrease offset (more negative)")
		assert.Equal(t, -3600, afterOffset-beforeOffset, "DST offset change should be -1 hour")
	})
}

func TestGetAllTimezones(t *testing.T) {
	timezones, err := GetAllTimezones()
	require.NoError(t, err)
	require.NotEmpty(t, timezones)

	t.Run("Contains common timezones", func(t *testing.T) {
		expectedTimezones := []string{
			"America/New_York",
			"America/Los_Angeles",
			"Europe/London",
			"Asia/Tokyo",
			"UTC",
		}

		for _, expected := range expectedTimezones {
			found := false
			for _, tz := range timezones {
				if tz.ID == expected {
					found = true
					break
				}
			}
			assert.True(t, found, "Should contain %s", expected)
		}
	})

	t.Run("All timezones have required fields", func(t *testing.T) {
		for _, tz := range timezones {
			assert.NotEmpty(t, tz.ID)
			assert.NotEmpty(t, tz.DisplayName)
			assert.NotEmpty(t, tz.OffsetString)
			assert.NotEmpty(t, tz.Group)
		}
	})

	t.Run("Timezones are sorted by offset", func(t *testing.T) {
		for i := 1; i < len(timezones); i++ {
			if timezones[i].OffsetMinutes < timezones[i-1].OffsetMinutes {
				t.Errorf("Timezones not sorted by offset at index %d: %d < %d",
					i, timezones[i].OffsetMinutes, timezones[i-1].OffsetMinutes)
			}
		}
	})
}

func TestSearchTimezones(t *testing.T) {
	tests := []struct {
		name          string
		query         string
		expectMatches []string
		minResults    int
	}{
		{
			name:          "Search by city name",
			query:         "New York",
			expectMatches: []string{"America/New_York"},
			minResults:    1,
		},
		{
			name:          "Search by region",
			query:         "Europe",
			minResults:    10,
		},
		{
			name:       "Empty query returns all",
			query:      "",
			minResults: 50,
		},
		{
			name:          "Search by abbreviation",
			query:         "UTC",
			expectMatches: []string{"UTC"},
			minResults:    1,
		},
		{
			name:       "Case insensitive search",
			query:      "TOKYO",
			minResults: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := SearchTimezones(tt.query)
			require.NoError(t, err)

			if tt.minResults > 0 {
				assert.GreaterOrEqual(t, len(results), tt.minResults,
					"Should have at least %d results", tt.minResults)
			}

			for _, expected := range tt.expectMatches {
				found := false
				for _, tz := range results {
					if tz.ID == expected {
						found = true
						break
					}
				}
				assert.True(t, found, "Should find %s in results", expected)
			}
		})
	}
}

func TestConvertTime(t *testing.T) {
	tests := []struct {
		name    string
		time    time.Time
		fromTZ  string
		toTZ    string
		wantErr bool
	}{
		{
			name:    "Convert NY to London",
			time:    time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
			fromTZ:  "America/New_York",
			toTZ:    "Europe/London",
			wantErr: false,
		},
		{
			name:    "Convert Tokyo to UTC",
			time:    time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
			fromTZ:  "Asia/Tokyo",
			toTZ:    "UTC",
			wantErr: false,
		},
		{
			name:    "Invalid from timezone",
			time:    time.Now(),
			fromTZ:  "Invalid/Zone",
			toTZ:    "UTC",
			wantErr: true,
		},
		{
			name:    "Invalid to timezone",
			time:    time.Now(),
			fromTZ:  "UTC",
			toTZ:    "Invalid/Zone",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converted, err := ConvertTime(tt.time, tt.fromTZ, tt.toTZ)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotZero(t, converted)

			// Verify the underlying instant is the same (UTC)
			assert.Equal(t, tt.time.Unix(), converted.Unix())
		})
	}
}

func TestGetUserLocalTime(t *testing.T) {
	tests := []struct {
		name     string
		timezone string
		wantErr  bool
	}{
		{
			name:     "Valid timezone",
			timezone: "America/New_York",
			wantErr:  false,
		},
		{
			name:     "UTC",
			timezone: "UTC",
			wantErr:  false,
		},
		{
			name:     "Invalid timezone",
			timezone: "Invalid/Zone",
			wantErr:  true,
		},
		{
			name:     "Empty timezone",
			timezone: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			localTime, err := GetUserLocalTime(tt.timezone)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotZero(t, localTime)

			// Verify it's a recent time (within last minute)
			now := time.Now().UTC()
			diff := now.Sub(localTime.UTC())
			assert.Less(t, diff.Abs(), time.Minute, "Local time should be recent")
		})
	}
}

func TestFormatOffset(t *testing.T) {
	tests := []struct {
		name          string
		offsetMinutes int
		want          string
	}{
		{
			name:          "UTC",
			offsetMinutes: 0,
			want:          "UTC+00:00",
		},
		{
			name:          "EST (UTC-5)",
			offsetMinutes: -300,
			want:          "UTC-05:00",
		},
		{
			name:          "IST (UTC+5:30)",
			offsetMinutes: 330,
			want:          "UTC+05:30",
		},
		{
			name:          "JST (UTC+9)",
			offsetMinutes: 540,
			want:          "UTC+09:00",
		},
		{
			name:          "NZST (UTC+12)",
			offsetMinutes: 720,
			want:          "UTC+12:00",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatOffset(tt.offsetMinutes)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTimezoneGroups(t *testing.T) {
	tests := []struct {
		timezone      string
		expectedGroup string
	}{
		{"America/New_York", GroupAmericas},
		{"America/Los_Angeles", GroupAmericas},
		{"Europe/London", GroupEurope},
		{"Europe/Paris", GroupEurope},
		{"Asia/Tokyo", GroupAsia},
		{"Asia/Shanghai", GroupAsia},
		{"Pacific/Auckland", GroupPacific},
		{"Australia/Sydney", GroupPacific},
		{"Africa/Cairo", GroupAfrica},
		{"UTC", GroupOther},
	}

	for _, tt := range tests {
		t.Run(tt.timezone, func(t *testing.T) {
			group := getTimezoneGroup(tt.timezone)
			assert.Equal(t, tt.expectedGroup, group)
		})
	}
}

// Benchmark tests for performance-critical operations
func BenchmarkIsValidTimezone(b *testing.B) {
	for i := 0; i < b.N; i++ {
		IsValidTimezone("America/New_York")
	}
}

func BenchmarkGetTimezoneInfo(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GetTimezoneInfo("America/New_York")
	}
}

func BenchmarkGetAllTimezones(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GetAllTimezones()
	}
}

func BenchmarkSearchTimezones(b *testing.B) {
	for i := 0; i < b.N; i++ {
		SearchTimezones("New York")
	}
}
