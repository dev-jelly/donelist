package premium

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHasFeature(t *testing.T) {
	tests := []struct {
		name     string
		tier     Tier
		feature  Feature
		expected bool
	}{
		{
			name:     "Free tier - no premium features",
			tier:     TierFree,
			feature:  FeatureAdvancedAnalytics,
			expected: false,
		},
		{
			name:     "Premium tier - has analytics",
			tier:     TierPremium,
			feature:  FeatureAdvancedAnalytics,
			expected: true,
		},
		{
			name:     "Premium tier - no enterprise features",
			tier:     TierPremium,
			feature:  FeatureSSO,
			expected: false,
		},
		{
			name:     "Enterprise tier - has all features",
			tier:     TierEnterprise,
			feature:  FeatureSSO,
			expected: true,
		},
		{
			name:     "Enterprise tier - has premium features",
			tier:     TierEnterprise,
			feature:  FeatureAdvancedAnalytics,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasFeature(tt.tier, tt.feature)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetLimit(t *testing.T) {
	tests := []struct {
		name      string
		tier      Tier
		limitType string
		expected  int
	}{
		{
			name:      "Free tier - checkins per day",
			tier:      TierFree,
			limitType: "checkins_per_day",
			expected:  50,
		},
		{
			name:      "Free tier - categories count",
			tier:      TierFree,
			limitType: "categories_count",
			expected:  5,
		},
		{
			name:      "Premium tier - unlimited checkins",
			tier:      TierPremium,
			limitType: "checkins_per_day",
			expected:  -1,
		},
		{
			name:      "Premium tier - webhooks count",
			tier:      TierPremium,
			limitType: "webhooks_count",
			expected:  10,
		},
		{
			name:      "Enterprise tier - unlimited everything",
			tier:      TierEnterprise,
			limitType: "webhooks_count",
			expected:  -1,
		},
		{
			name:      "Unknown limit type",
			tier:      TierFree,
			limitType: "unknown_limit",
			expected:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetLimit(tt.tier, tt.limitType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCompareTiers(t *testing.T) {
	tests := []struct {
		name     string
		tier1    Tier
		tier2    Tier
		expected int
	}{
		{
			name:     "Free < Premium",
			tier1:    TierFree,
			tier2:    TierPremium,
			expected: -1,
		},
		{
			name:     "Premium > Free",
			tier1:    TierPremium,
			tier2:    TierFree,
			expected: 1,
		},
		{
			name:     "Premium < Enterprise",
			tier1:    TierPremium,
			tier2:    TierEnterprise,
			expected: -1,
		},
		{
			name:     "Enterprise > Premium",
			tier1:    TierEnterprise,
			tier2:    TierPremium,
			expected: 1,
		},
		{
			name:     "Premium == Premium",
			tier1:    TierPremium,
			tier2:    TierPremium,
			expected: 0,
		},
		{
			name:     "Invalid tier defaults to free",
			tier1:    Tier("invalid"),
			tier2:    TierFree,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CompareTiers(tt.tier1, tt.tier2)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCanUpgrade(t *testing.T) {
	tests := []struct {
		name     string
		from     Tier
		to       Tier
		expected bool
	}{
		{
			name:     "Can upgrade from free to premium",
			from:     TierFree,
			to:       TierPremium,
			expected: true,
		},
		{
			name:     "Can upgrade from free to enterprise",
			from:     TierFree,
			to:       TierEnterprise,
			expected: true,
		},
		{
			name:     "Can upgrade from premium to enterprise",
			from:     TierPremium,
			to:       TierEnterprise,
			expected: true,
		},
		{
			name:     "Cannot upgrade from premium to free",
			from:     TierPremium,
			to:       TierFree,
			expected: false,
		},
		{
			name:     "Cannot upgrade from same tier",
			from:     TierPremium,
			to:       TierPremium,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CanUpgrade(tt.from, tt.to)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCanDowngrade(t *testing.T) {
	tests := []struct {
		name     string
		from     Tier
		to       Tier
		expected bool
	}{
		{
			name:     "Can downgrade from premium to free",
			from:     TierPremium,
			to:       TierFree,
			expected: true,
		},
		{
			name:     "Can downgrade from enterprise to premium",
			from:     TierEnterprise,
			to:       TierPremium,
			expected: true,
		},
		{
			name:     "Can downgrade from enterprise to free",
			from:     TierEnterprise,
			to:       TierFree,
			expected: true,
		},
		{
			name:     "Cannot downgrade from free to premium",
			from:     TierFree,
			to:       TierPremium,
			expected: false,
		},
		{
			name:     "Cannot downgrade from same tier",
			from:     TierPremium,
			to:       TierPremium,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CanDowngrade(tt.from, tt.to)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetUpgradeOptions(t *testing.T) {
	tests := []struct {
		name         string
		currentTier  Tier
		expectedOpts []Tier
	}{
		{
			name:         "Free tier can upgrade to premium and enterprise",
			currentTier:  TierFree,
			expectedOpts: []Tier{TierPremium, TierEnterprise},
		},
		{
			name:         "Premium tier can upgrade to enterprise",
			currentTier:  TierPremium,
			expectedOpts: []Tier{TierEnterprise},
		},
		{
			name:         "Enterprise tier has no upgrade options",
			currentTier:  TierEnterprise,
			expectedOpts: []Tier{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetUpgradeOptions(tt.currentTier)
			assert.Equal(t, tt.expectedOpts, result)
		})
	}
}

func TestGetDowngradeOptions(t *testing.T) {
	tests := []struct {
		name         string
		currentTier  Tier
		expectedOpts []Tier
	}{
		{
			name:         "Free tier has no downgrade options",
			currentTier:  TierFree,
			expectedOpts: []Tier{},
		},
		{
			name:         "Premium tier can downgrade to free",
			currentTier:  TierPremium,
			expectedOpts: []Tier{TierFree},
		},
		{
			name:         "Enterprise tier can downgrade to free and premium",
			currentTier:  TierEnterprise,
			expectedOpts: []Tier{TierFree, TierPremium},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetDowngradeOptions(tt.currentTier)
			assert.Equal(t, tt.expectedOpts, result)
		})
	}
}

func TestCheckEditPermission(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name         string
		tier         Tier
		checkinTime  time.Time
		expectedEdit bool
		expectedMsg  string
	}{
		{
			name:         "Free tier - within 2 hours",
			tier:         TierFree,
			checkinTime:  now.Add(-1 * time.Hour),
			expectedEdit: true,
			expectedMsg:  "",
		},
		{
			name:         "Free tier - outside 2 hours",
			tier:         TierFree,
			checkinTime:  now.Add(-3 * time.Hour),
			expectedEdit: false,
			expectedMsg:  "Edit window has expired. Upgrade to Premium for unlimited edit history.",
		},
		{
			name:         "Premium tier - unlimited edit",
			tier:         TierPremium,
			checkinTime:  now.Add(-720 * time.Hour), // 30 days ago
			expectedEdit: true,
			expectedMsg:  "",
		},
		{
			name:         "Enterprise tier - unlimited edit",
			tier:         TierEnterprise,
			checkinTime:  now.Add(-8760 * time.Hour), // 1 year ago
			expectedEdit: true,
			expectedMsg:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			canEdit, reason := CheckEditPermission(tt.tier, tt.checkinTime)
			assert.Equal(t, tt.expectedEdit, canEdit)
			assert.Equal(t, tt.expectedMsg, reason)
		})
	}
}

func TestGetFeatureDescription(t *testing.T) {
	tests := []struct {
		name        string
		feature     Feature
		expectEmpty bool
	}{
		{
			name:        "Known feature has description",
			feature:     FeatureAdvancedAnalytics,
			expectEmpty: false,
		},
		{
			name:        "Unknown feature returns feature name",
			feature:     Feature("unknown_feature"),
			expectEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetFeatureDescription(tt.feature)
			if tt.expectEmpty {
				assert.Empty(t, result)
			} else {
				assert.NotEmpty(t, result)
			}
		})
	}
}

func TestDefaultTiers(t *testing.T) {
	tiers := DefaultTiers()

	t.Run("All tiers exist", func(t *testing.T) {
		assert.Contains(t, tiers, TierFree)
		assert.Contains(t, tiers, TierPremium)
		assert.Contains(t, tiers, TierEnterprise)
	})

	t.Run("Free tier configuration", func(t *testing.T) {
		free := tiers[TierFree]
		assert.Equal(t, "Free", free.Name)
		assert.Equal(t, 0, free.Price)
		assert.Equal(t, 50, free.Limits["checkins_per_day"])
		assert.Equal(t, 5, free.Limits["categories_count"])
	})

	t.Run("Premium tier configuration", func(t *testing.T) {
		premium := tiers[TierPremium]
		assert.Equal(t, "Premium", premium.Name)
		assert.Equal(t, 999, premium.Price)
		assert.Equal(t, -1, premium.Limits["checkins_per_day"]) // unlimited
		assert.Contains(t, premium.Features, FeatureAdvancedAnalytics)
		assert.Contains(t, premium.Features, FeatureDataExport)
	})

	t.Run("Enterprise tier configuration", func(t *testing.T) {
		enterprise := tiers[TierEnterprise]
		assert.Equal(t, "Enterprise", enterprise.Name)
		assert.Equal(t, 2999, enterprise.Price)
		assert.Contains(t, enterprise.Features, FeatureSSO)
		assert.Contains(t, enterprise.Features, FeatureTeamCollaboration)
		assert.Equal(t, -1, enterprise.Limits["team_members"]) // unlimited
	})
}

func TestTierValidation(t *testing.T) {
	tests := []struct {
		name     string
		tier     Tier
		expected bool
	}{
		{
			name:     "Valid free tier",
			tier:     TierFree,
			expected: true,
		},
		{
			name:     "Valid premium tier",
			tier:     TierPremium,
			expected: true,
		},
		{
			name:     "Valid enterprise tier",
			tier:     TierEnterprise,
			expected: true,
		},
		{
			name:     "Invalid tier",
			tier:     Tier("invalid"),
			expected: false,
		},
		{
			name:     "Empty tier",
			tier:     Tier(""),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.tier.IsValid()
			assert.Equal(t, tt.expected, result)
		})
	}
}