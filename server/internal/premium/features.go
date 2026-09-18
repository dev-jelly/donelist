package premium

import "time"

// Feature represents a premium feature
type Feature string

const (
	// Core features
	FeatureUnlimitedCheckins      Feature = "unlimited_checkins"
	FeatureExtendedEditHistory    Feature = "extended_edit_history"
	FeatureUnlimitedCategories    Feature = "unlimited_categories"
	FeatureAdvancedAnalytics      Feature = "advanced_analytics"
	FeatureDataExport             Feature = "data_export"
	FeatureAPIAccess              Feature = "api_access"
	FeatureWebhooks               Feature = "webhooks"
	FeatureCustomIntegrations     Feature = "custom_integrations"
	FeaturePrioritySupport        Feature = "priority_support"

	// Team features
	FeatureTeamCollaboration      Feature = "team_collaboration"
	FeatureSSO                    Feature = "sso"
	FeatureAuditLogs              Feature = "audit_logs"
	FeatureCustomBranding         Feature = "custom_branding"
	FeatureDedicatedSupport       Feature = "dedicated_support"

	// Advanced features
	FeatureAdvancedSearch         Feature = "advanced_search"
	FeatureBulkOperations         Feature = "bulk_operations"
	FeatureScheduledReports       Feature = "scheduled_reports"
	FeatureCustomFields           Feature = "custom_fields"
	FeatureThirdPartySync         Feature = "third_party_sync"
)

// TierConfig represents configuration for a subscription tier
type TierConfig struct {
	Name        string
	Description string
	Features    []Feature
	Limits      map[string]int
	Price       int // in cents
}

// DefaultTiers returns the default tier configurations
func DefaultTiers() map[Tier]TierConfig {
	return map[Tier]TierConfig{
		TierFree: {
			Name:        "Free",
			Description: "Basic features for personal use",
			Features: []Feature{
				// Basic features only
			},
			Limits: map[string]int{
				"checkins_per_day":    50,
				"checkins_per_month":  1500,
				"categories_count":    5,
				"edit_history_hours":  2,
				"data_retention_days": 30,
				"api_calls_per_hour":  100,
				"exports_per_month":   1,
			},
			Price: 0,
		},
		TierPremium: {
			Name:        "Premium",
			Description: "Advanced features for power users",
			Features: []Feature{
				FeatureUnlimitedCheckins,
				FeatureExtendedEditHistory,
				FeatureUnlimitedCategories,
				FeatureAdvancedAnalytics,
				FeatureDataExport,
				FeatureAPIAccess,
				FeatureWebhooks,
				FeaturePrioritySupport,
				FeatureAdvancedSearch,
				FeatureBulkOperations,
			},
			Limits: map[string]int{
				"checkins_per_day":    -1, // unlimited
				"checkins_per_month":  -1, // unlimited
				"categories_count":    -1, // unlimited
				"edit_history_hours":  -1, // unlimited
				"data_retention_days": 365,
				"api_calls_per_hour":  1000,
				"exports_per_month":   -1, // unlimited
				"webhooks_count":      10,
				"api_tokens_count":    5,
			},
			Price: 999, // $9.99
		},
		TierEnterprise: {
			Name:        "Enterprise",
			Description: "Full features for teams and organizations",
			Features: []Feature{
				// All Premium features
				FeatureUnlimitedCheckins,
				FeatureExtendedEditHistory,
				FeatureUnlimitedCategories,
				FeatureAdvancedAnalytics,
				FeatureDataExport,
				FeatureAPIAccess,
				FeatureWebhooks,
				FeaturePrioritySupport,
				FeatureAdvancedSearch,
				FeatureBulkOperations,
				// Plus Enterprise features
				FeatureTeamCollaboration,
				FeatureSSO,
				FeatureAuditLogs,
				FeatureCustomBranding,
				FeatureDedicatedSupport,
				FeatureScheduledReports,
				FeatureCustomFields,
				FeatureThirdPartySync,
				FeatureCustomIntegrations,
			},
			Limits: map[string]int{
				"checkins_per_day":    -1, // unlimited
				"checkins_per_month":  -1, // unlimited
				"categories_count":    -1, // unlimited
				"edit_history_hours":  -1, // unlimited
				"data_retention_days": -1, // unlimited
				"api_calls_per_hour":  -1, // unlimited
				"exports_per_month":   -1, // unlimited
				"webhooks_count":      -1, // unlimited
				"api_tokens_count":    -1, // unlimited
				"team_members":        -1, // unlimited
			},
			Price: 2999, // $29.99
		},
	}
}

// HasFeature checks if a tier has a specific feature
func HasFeature(tier Tier, feature Feature) bool {
	tiers := DefaultTiers()
	config, exists := tiers[tier]
	if !exists {
		return false
	}

	for _, f := range config.Features {
		if f == feature {
			return true
		}
	}
	return false
}

// GetLimit returns the limit for a specific resource in a tier
func GetLimit(tier Tier, limitType string) int {
	tiers := DefaultTiers()
	config, exists := tiers[tier]
	if !exists {
		return 0
	}

	limit, exists := config.Limits[limitType]
	if !exists {
		return 0
	}
	return limit
}

// CompareTiers compares two tiers and returns:
// -1 if tier1 < tier2
// 0 if tier1 == tier2
// 1 if tier1 > tier2
func CompareTiers(tier1, tier2 Tier) int {
	tierOrder := map[Tier]int{
		TierFree:       0,
		TierPremium:    1,
		TierEnterprise: 2,
	}

	order1, exists1 := tierOrder[tier1]
	order2, exists2 := tierOrder[tier2]

	if !exists1 {
		order1 = 0
	}
	if !exists2 {
		order2 = 0
	}

	if order1 < order2 {
		return -1
	} else if order1 > order2 {
		return 1
	}
	return 0
}

// CanUpgrade checks if a tier can be upgraded to another tier
func CanUpgrade(from, to Tier) bool {
	return CompareTiers(from, to) < 0
}

// CanDowngrade checks if a tier can be downgraded to another tier
func CanDowngrade(from, to Tier) bool {
	return CompareTiers(from, to) > 0
}

// GetUpgradeOptions returns available upgrade options for a tier
func GetUpgradeOptions(currentTier Tier) []Tier {
	allTiers := []Tier{TierFree, TierPremium, TierEnterprise}
	options := []Tier{}

	for _, tier := range allTiers {
		if CanUpgrade(currentTier, tier) {
			options = append(options, tier)
		}
	}
	return options
}

// GetDowngradeOptions returns available downgrade options for a tier
func GetDowngradeOptions(currentTier Tier) []Tier {
	allTiers := []Tier{TierFree, TierPremium, TierEnterprise}
	options := []Tier{}

	for _, tier := range allTiers {
		if CanDowngrade(currentTier, tier) {
			options = append(options, tier)
		}
	}
	return options
}

// CheckEditPermission checks if a user can edit a checkin based on tier and time
func CheckEditPermission(tier Tier, checkinTime time.Time) (canEdit bool, reason string) {
	now := time.Now()
	hoursSince := now.Sub(checkinTime).Hours()

	editLimit := GetLimit(tier, "edit_history_hours")

	// If unlimited (-1), always allow
	if editLimit == -1 {
		return true, ""
	}

	// Check if within edit window
	if hoursSince <= float64(editLimit) {
		return true, ""
	}

	return false, "Edit window has expired. Upgrade to Premium for unlimited edit history."
}

// GetFeatureDescription returns a human-readable description of a feature
func GetFeatureDescription(feature Feature) string {
	descriptions := map[Feature]string{
		FeatureUnlimitedCheckins:   "Create unlimited check-ins without daily or monthly limits",
		FeatureExtendedEditHistory: "Edit your check-ins anytime, no matter how old they are",
		FeatureUnlimitedCategories: "Create unlimited categories to organize your check-ins",
		FeatureAdvancedAnalytics:   "Access detailed analytics and insights about your habits",
		FeatureDataExport:          "Export your data in various formats (CSV, JSON, PDF)",
		FeatureAPIAccess:           "Access the API for custom integrations",
		FeatureWebhooks:            "Set up webhooks to integrate with other services",
		FeatureCustomIntegrations:  "Build custom integrations with your tools",
		FeaturePrioritySupport:     "Get priority customer support",
		FeatureTeamCollaboration:   "Collaborate with your team members",
		FeatureSSO:                 "Single Sign-On for enterprise authentication",
		FeatureAuditLogs:           "Detailed audit logs for compliance",
		FeatureCustomBranding:      "Customize the app with your brand",
		FeatureDedicatedSupport:    "Dedicated account manager and support",
		FeatureAdvancedSearch:      "Advanced search filters and operators",
		FeatureBulkOperations:      "Perform bulk operations on multiple items",
		FeatureScheduledReports:    "Schedule automatic report generation",
		FeatureCustomFields:        "Add custom fields to your check-ins",
		FeatureThirdPartySync:      "Sync with third-party services automatically",
	}

	if desc, exists := descriptions[feature]; exists {
		return desc
	}
	return string(feature)
}