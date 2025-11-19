package statistics

import (
	"time"

	"github.com/google/uuid"
)

// UsageMetrics represents usage metrics for a specific item
type UsageMetrics struct {
	TotalUsage      int       `json:"total_usage"`       // Total number of times used
	UniqueUsers     int       `json:"unique_users"`      // Number of unique users (for shared items)
	LastUsed        time.Time `json:"last_used"`         // Last time this was used
	FirstUsed       time.Time `json:"first_used"`        // First time this was used
	DailyAverage    float64   `json:"daily_average"`     // Average usage per day
	WeeklyAverage   float64   `json:"weekly_average"`    // Average usage per week
	MonthlyAverage  float64   `json:"monthly_average"`   // Average usage per month
	TrendDirection  string    `json:"trend_direction"`   // "up", "down", "stable"
	TrendPercentage float64   `json:"trend_percentage"`  // Percentage change from previous period
}

// CategoryUsageStats represents usage statistics for a category
type CategoryUsageStats struct {
	CategoryID   uuid.UUID     `json:"category_id"`
	CategoryName string        `json:"category_name"`
	Color        *string       `json:"color,omitempty"`
	Icon         *string       `json:"icon,omitempty"`
	Metrics      UsageMetrics  `json:"metrics"`
	Rank         int           `json:"rank"`              // Rank by usage frequency
	Percentage   float64       `json:"percentage"`        // Percentage of total usage
	IsActive     bool          `json:"is_active"`         // Used in last 7 days
	DayPattern   []int         `json:"day_pattern"`       // Usage count per day of week [Sun-Sat]
	HourPattern  []int         `json:"hour_pattern"`      // Usage count per hour [0-23]
}

// TagUsageStats represents usage statistics for a tag
type TagUsageStats struct {
	TagID          uuid.UUID     `json:"tag_id"`
	TagName        string        `json:"tag_name"`
	Metrics        UsageMetrics  `json:"metrics"`
	Rank           int           `json:"rank"`              // Rank by usage frequency
	Percentage     float64       `json:"percentage"`        // Percentage of total usage
	IsActive       bool          `json:"is_active"`         // Used in last 7 days
	DayPattern     []int         `json:"day_pattern"`       // Usage count per day of week [Sun-Sat]
	HourPattern    []int         `json:"hour_pattern"`      // Usage count per hour [0-23]
	CoOccurrences  []TagPair     `json:"co_occurrences"`    // Tags frequently used together
}

// TagPair represents tags that frequently appear together
type TagPair struct {
	TagID      uuid.UUID `json:"tag_id"`
	TagName    string    `json:"tag_name"`
	Count      int       `json:"count"`       // Number of times they appeared together
	Confidence float64   `json:"confidence"`  // Confidence score (0-1)
}

// UsagePattern represents temporal usage patterns
type UsagePattern struct {
	Period          string    `json:"period"`           // "daily", "weekly", "monthly"
	PeakHours       []int     `json:"peak_hours"`       // Hours with highest usage
	PeakDays        []string  `json:"peak_days"`        // Days with highest usage
	LowActivityTime []int     `json:"low_activity_time"` // Hours with lowest usage
}

// CategoryTagMatrix represents the relationship between categories and tags
type CategoryTagMatrix struct {
	CategoryID   uuid.UUID `json:"category_id"`
	CategoryName string    `json:"category_name"`
	TagUsage     []struct {
		TagID      uuid.UUID `json:"tag_id"`
		TagName    string    `json:"tag_name"`
		UsageCount int       `json:"usage_count"`
		Percentage float64   `json:"percentage"` // Percentage within this category
	} `json:"tag_usage"`
}

// UsageDashboard represents the complete usage statistics dashboard
type UsageDashboard struct {
	// Time information
	GeneratedAt time.Time `json:"generated_at"`
	Period      struct {
		Start time.Time `json:"start"`
		End   time.Time `json:"end"`
		Days  int       `json:"days"`
	} `json:"period"`

	// Summary statistics
	Summary struct {
		TotalCheckins      int     `json:"total_checkins"`
		TotalCategories    int     `json:"total_categories"`
		ActiveCategories   int     `json:"active_categories"`   // Used in period
		TotalTags          int     `json:"total_tags"`
		ActiveTags         int     `json:"active_tags"`         // Used in period
		AverageTagsPerItem float64 `json:"average_tags_per_item"`
		CategoryCoverage   float64 `json:"category_coverage"`   // % of items with categories
		TagCoverage        float64 `json:"tag_coverage"`        // % of items with tags
	} `json:"summary"`

	// Top performers
	TopCategories []*CategoryUsageStats `json:"top_categories"`
	TopTags       []*TagUsageStats      `json:"top_tags"`

	// Unused items (for cleanup suggestions)
	UnusedCategories []struct {
		CategoryID   uuid.UUID  `json:"category_id"`
		CategoryName string     `json:"category_name"`
		LastUsed     *time.Time `json:"last_used,omitempty"`
		DaysSinceUse int        `json:"days_since_use"`
	} `json:"unused_categories"`

	UnusedTags []struct {
		TagID        uuid.UUID  `json:"tag_id"`
		TagName      string     `json:"tag_name"`
		LastUsed     *time.Time `json:"last_used,omitempty"`
		DaysSinceUse int        `json:"days_since_use"`
	} `json:"unused_tags"`

	// Patterns and insights
	UsagePatterns struct {
		CategoryPatterns []UsagePattern `json:"category_patterns"`
		TagPatterns      []UsagePattern `json:"tag_patterns"`
	} `json:"usage_patterns"`

	// Relationships
	CategoryTagMatrix []*CategoryTagMatrix `json:"category_tag_matrix"`

	// Growth metrics
	Growth struct {
		NewCategories      int     `json:"new_categories"`       // Created in period
		NewTags            int     `json:"new_tags"`             // Created in period
		CategoryGrowthRate float64 `json:"category_growth_rate"` // % change
		TagGrowthRate      float64 `json:"tag_growth_rate"`      // % change
	} `json:"growth"`

	// Recommendations
	Recommendations struct {
		SuggestedMerges []struct {
			Type   string    `json:"type"` // "category" or "tag"
			Item1  string    `json:"item1_name"`
			Item1ID uuid.UUID `json:"item1_id"`
			Item2  string    `json:"item2_name"`
			Item2ID uuid.UUID `json:"item2_id"`
			Reason string    `json:"reason"`
		} `json:"suggested_merges"`

		CleanupCandidates []struct {
			Type       string     `json:"type"` // "category" or "tag"
			ItemName   string     `json:"item_name"`
			ItemID     uuid.UUID  `json:"item_id"`
			LastUsed   *time.Time `json:"last_used,omitempty"`
			UsageCount int        `json:"usage_count"`
			Reason     string     `json:"reason"`
		} `json:"cleanup_candidates"`
	} `json:"recommendations"`
}

// UsageAggregation represents aggregated usage data for a time period
type UsageAggregation struct {
	Period    string    `json:"period"`    // "hour", "day", "week", "month"
	Timestamp time.Time `json:"timestamp"` // Start of the period
	Count     int       `json:"count"`     // Number of uses in this period
	UniqueItems int     `json:"unique_items"` // Number of unique items used
}

// UsageTrend represents trend analysis for usage over time
type UsageTrend struct {
	ItemID       uuid.UUID          `json:"item_id"`
	ItemName     string             `json:"item_name"`
	ItemType     string             `json:"item_type"` // "category" or "tag"
	Aggregations []UsageAggregation `json:"aggregations"`
	TrendLine    []float64          `json:"trend_line"`     // Smoothed trend values
	Prediction   []float64          `json:"prediction"`     // Future predictions
	Seasonality  map[string]float64 `json:"seasonality"`    // Seasonal patterns
}