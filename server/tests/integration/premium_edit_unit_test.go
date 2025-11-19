package integration

import (
	"testing"
	"time"

	"github.com/dev-jelly/donelist/internal/checkin"
	"github.com/dev-jelly/donelist/internal/premium"
	"github.com/stretchr/testify/assert"
)

// Unit tests for permission logic that don't require database
// These tests verify the core business logic without integration dependencies

func TestPermissionLogic_CanEditCheckin(t *testing.T) {
	t.Run("FreeUser_WithinWindow_CanEdit", func(t *testing.T) {
		// Checkin created 1 hour ago
		checkinTime := time.Now().UTC().Add(-1 * time.Hour)
		canEdit := checkin.CanEditCheckin(checkinTime, premium.TierFree)
		assert.True(t, canEdit, "Free user should be able to edit checkin within 2 hours")
	})

	t.Run("FreeUser_AtBoundary_CannotEdit", func(t *testing.T) {
		// Checkin created exactly 2 hours ago
		checkinTime := time.Now().UTC().Add(-2 * time.Hour)
		canEdit := checkin.CanEditCheckin(checkinTime, premium.TierFree)
		assert.False(t, canEdit, "Free user should NOT be able to edit at 2-hour boundary")
	})

	t.Run("FreeUser_BeyondWindow_CannotEdit", func(t *testing.T) {
		// Checkin created 3 hours ago
		checkinTime := time.Now().UTC().Add(-3 * time.Hour)
		canEdit := checkin.CanEditCheckin(checkinTime, premium.TierFree)
		assert.False(t, canEdit, "Free user should NOT be able to edit beyond 2 hours")
	})

	t.Run("FreeUser_JustUnderBoundary_CanEdit", func(t *testing.T) {
		// Checkin created 119 minutes ago (just under 2 hours)
		checkinTime := time.Now().UTC().Add(-119 * time.Minute)
		canEdit := checkin.CanEditCheckin(checkinTime, premium.TierFree)
		assert.True(t, canEdit, "Free user should be able to edit just under 2 hours")
	})

	t.Run("PremiumUser_AnyTime_CanEdit", func(t *testing.T) {
		// Checkin created 1 week ago
		checkinTime := time.Now().UTC().Add(-7 * 24 * time.Hour)
		canEdit := checkin.CanEditCheckin(checkinTime, premium.TierPremium)
		assert.True(t, canEdit, "Premium user should be able to edit any time")
	})

	t.Run("EnterpriseUser_AnyTime_CanEdit", func(t *testing.T) {
		// Checkin created 1 month ago
		checkinTime := time.Now().UTC().Add(-30 * 24 * time.Hour)
		canEdit := checkin.CanEditCheckin(checkinTime, premium.TierEnterprise)
		assert.True(t, canEdit, "Enterprise user should be able to edit any time")
	})

	t.Run("FreeUser_Future_CanEdit", func(t *testing.T) {
		// Edge case: checkin time is in the future (clock skew)
		checkinTime := time.Now().UTC().Add(5 * time.Minute)
		canEdit := checkin.CanEditCheckin(checkinTime, premium.TierFree)
		assert.True(t, canEdit, "Should be able to edit future-dated checkin")
	})
}

func TestTimeUtilities_GetEditTimeRemaining(t *testing.T) {
	t.Run("RecentCheckin_HasTimeRemaining", func(t *testing.T) {
		checkinTime := time.Now().UTC().Add(-30 * time.Minute)
		remaining := checkin.GetEditTimeRemaining(checkinTime)

		// Should have approximately 90 minutes remaining
		assert.Greater(t, remaining, 89*time.Minute, "Should have at least 89 minutes")
		assert.Less(t, remaining, 91*time.Minute, "Should have at most 91 minutes")
	})

	t.Run("OldCheckin_NoTimeRemaining", func(t *testing.T) {
		checkinTime := time.Now().UTC().Add(-3 * time.Hour)
		remaining := checkin.GetEditTimeRemaining(checkinTime)
		assert.Equal(t, time.Duration(0), remaining, "Old checkin should have no time remaining")
	})

	t.Run("AtBoundary_NoTimeRemaining", func(t *testing.T) {
		checkinTime := time.Now().UTC().Add(-2 * time.Hour)
		remaining := checkin.GetEditTimeRemaining(checkinTime)
		assert.Equal(t, time.Duration(0), remaining, "At boundary should have no time remaining")
	})

	t.Run("JustCreated_FullTimeRemaining", func(t *testing.T) {
		checkinTime := time.Now().UTC()
		remaining := checkin.GetEditTimeRemaining(checkinTime)

		// Should have approximately 2 hours remaining
		assert.Greater(t, remaining, 119*time.Minute, "Should have close to 2 hours")
		assert.LessOrEqual(t, remaining, 120*time.Minute, "Should not exceed 2 hours")
	})
}

func TestTimeUtilities_IsWithinEditWindow(t *testing.T) {
	t.Run("RecentCheckin_IsWithinWindow", func(t *testing.T) {
		checkinTime := time.Now().UTC().Add(-1 * time.Hour)
		isWithin := checkin.IsWithinEditWindow(checkinTime)
		assert.True(t, isWithin)
	})

	t.Run("OldCheckin_IsNotWithinWindow", func(t *testing.T) {
		checkinTime := time.Now().UTC().Add(-3 * time.Hour)
		isWithin := checkin.IsWithinEditWindow(checkinTime)
		assert.False(t, isWithin)
	})

	t.Run("AtBoundary_IsNotWithinWindow", func(t *testing.T) {
		checkinTime := time.Now().UTC().Add(-2 * time.Hour)
		isWithin := checkin.IsWithinEditWindow(checkinTime)
		assert.False(t, isWithin)
	})
}

func TestTimeUtilities_GetEditWindowExpiry(t *testing.T) {
	t.Run("ExpiryTime_IsCorrect", func(t *testing.T) {
		checkinTime := time.Now().UTC()
		expiry := checkin.GetEditWindowExpiry(checkinTime)
		expected := checkinTime.Add(2 * time.Hour)

		// Allow 1 second tolerance for test execution time
		diff := expiry.Sub(expected)
		assert.Less(t, diff, time.Second)
		assert.Greater(t, diff, -time.Second)
	})
}

func TestTimeUtilities_FormatTimeRemaining(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{
			name:     "NoTime",
			duration: 0,
			expected: "expired",
		},
		{
			name:     "NegativeTime",
			duration: -5 * time.Minute,
			expected: "expired",
		},
		{
			name:     "OnlySeconds",
			duration: 30 * time.Second,
			expected: "30s",
		},
		{
			name:     "OnlyMinutes",
			duration: 45 * time.Minute,
			expected: "45m",
		},
		{
			name:     "OnlyHours",
			duration: 2 * time.Hour,
			expected: "2h",
		},
		{
			name:     "HoursAndMinutes",
			duration: 1*time.Hour + 30*time.Minute,
			expected: "1h 30m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checkin.FormatTimeRemaining(tt.duration)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBoundaryConditions_TwoHourBoundary(t *testing.T) {
	t.Run("Just_Under_Boundary", func(t *testing.T) {
		// 119 minutes, 50 seconds (well under boundary considering 1s tolerance)
		elapsed := 119*time.Minute + 50*time.Second
		assert.True(t, checkin.IsWithinTwoHourBoundary(elapsed))
	})

	t.Run("Exactly_At_Boundary", func(t *testing.T) {
		// Exactly 120 minutes
		elapsed := 120 * time.Minute
		// The boundary is exclusive, so at exactly 2 hours we should be beyond
		assert.False(t, checkin.IsWithinTwoHourBoundary(elapsed))
	})

	t.Run("Just_Beyond_Boundary", func(t *testing.T) {
		// 120 minutes, 1 second
		elapsed := 120*time.Minute + 1*time.Second
		assert.False(t, checkin.IsWithinTwoHourBoundary(elapsed))
	})

	t.Run("Well_Beyond_Boundary", func(t *testing.T) {
		// 3 hours
		elapsed := 3 * time.Hour
		assert.False(t, checkin.IsWithinTwoHourBoundary(elapsed))
	})
}

func TestCheckinIntervals_CanCheckinWithInterval(t *testing.T) {
	baseTime := time.Now().UTC().Add(-1 * time.Hour)

	t.Run("Within2Hours_15MinInterval_Allowed", func(t *testing.T) {
		lastCheckin := time.Now().UTC().Add(-30 * time.Minute)
		canCheckin := checkin.CanCheckinWithInterval(lastCheckin, 15*time.Minute)
		assert.True(t, canCheckin)
	})

	t.Run("Within2Hours_30MinInterval_Allowed", func(t *testing.T) {
		lastCheckin := time.Now().UTC().Add(-30 * time.Minute)
		canCheckin := checkin.CanCheckinWithInterval(lastCheckin, 30*time.Minute)
		assert.True(t, canCheckin)
	})

	t.Run("Within2Hours_2HourInterval_Allowed", func(t *testing.T) {
		lastCheckin := time.Now().UTC().Add(-30 * time.Minute)
		canCheckin := checkin.CanCheckinWithInterval(lastCheckin, 2*time.Hour)
		assert.True(t, canCheckin)
	})

	t.Run("Beyond2Hours_15MinInterval_NotAllowed", func(t *testing.T) {
		lastCheckin := time.Now().UTC().Add(-3 * time.Hour)
		canCheckin := checkin.CanCheckinWithInterval(lastCheckin, 15*time.Minute)
		assert.False(t, canCheckin)
	})

	t.Run("Beyond2Hours_2HourInterval_Allowed", func(t *testing.T) {
		lastCheckin := time.Now().UTC().Add(-3 * time.Hour)
		canCheckin := checkin.CanCheckinWithInterval(lastCheckin, 2*time.Hour)
		assert.True(t, canCheckin)
	})

	t.Run("InvalidInterval_NotAllowed", func(t *testing.T) {
		lastCheckin := baseTime
		canCheckin := checkin.CanCheckinWithInterval(lastCheckin, 10*time.Minute)
		assert.False(t, canCheckin, "Invalid interval should not be allowed")
	})
}

func TestPremiumTiers_Validation(t *testing.T) {
	t.Run("ValidTiers", func(t *testing.T) {
		assert.True(t, premium.TierFree.IsValid())
		assert.True(t, premium.TierPremium.IsValid())
		assert.True(t, premium.TierEnterprise.IsValid())
	})

	t.Run("InvalidTier", func(t *testing.T) {
		invalidTier := premium.Tier("invalid")
		assert.False(t, invalidTier.IsValid())
	})

	t.Run("TierStrings", func(t *testing.T) {
		assert.Equal(t, "free", premium.TierFree.String())
		assert.Equal(t, "premium", premium.TierPremium.String())
		assert.Equal(t, "enterprise", premium.TierEnterprise.String())
	})
}

// Benchmark tests for performance validation
func BenchmarkCanEditCheckin_FreeUser(b *testing.B) {
	checkinTime := time.Now().UTC().Add(-1 * time.Hour)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checkin.CanEditCheckin(checkinTime, premium.TierFree)
	}
}

func BenchmarkCanEditCheckin_PremiumUser(b *testing.B) {
	checkinTime := time.Now().UTC().Add(-1 * time.Hour)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checkin.CanEditCheckin(checkinTime, premium.TierPremium)
	}
}

func BenchmarkGetEditTimeRemaining(b *testing.B) {
	checkinTime := time.Now().UTC().Add(-30 * time.Minute)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checkin.GetEditTimeRemaining(checkinTime)
	}
}
