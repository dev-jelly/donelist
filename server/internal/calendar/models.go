package calendar

import (
	"time"

	"github.com/google/uuid"
)

// StartDay represents the first day of the week for calendar display
type StartDay string

const (
	StartDaySunday StartDay = "sunday"
	StartDayMonday StartDay = "monday"
)

// CalendarDay represents a single day in the calendar view
type CalendarDay struct {
	Date              string  `json:"date"`               // YYYY-MM-DD format
	CheckinCount      int     `json:"checkin_count"`      // Number of check-ins on this day
	TotalMinutes      int     `json:"total_minutes"`      // Total duration of all check-ins
	CompletionPercent float64 `json:"completion_percent"` // Percentage of day covered (0-100)
	ColorIntensity    int     `json:"color_intensity"`    // Color intensity level (0-4)
	IsCurrentMonth    bool    `json:"is_current_month"`   // True if day belongs to current month
	IsToday           bool    `json:"is_today"`           // True if this is today's date
	HasCheckins       bool    `json:"has_checkins"`       // True if there are any check-ins
}

// CalendarWeek represents a week in the calendar (7 days)
type CalendarWeek struct {
	Days []*CalendarDay `json:"days"` // Always 7 days (Sun-Sat or Mon-Sun based on preference)
}

// MonthlySummary represents aggregate statistics for the month
type MonthlySummary struct {
	TotalCheckins       int     `json:"total_checkins"`        // Total number of check-ins
	TotalMinutes        int     `json:"total_minutes"`         // Total duration of all check-ins
	DaysWithCheckins    int     `json:"days_with_checkins"`    // Number of days with at least one check-in
	TotalDaysInMonth    int     `json:"total_days_in_month"`   // Total days in the month
	AveragePerDay       float64 `json:"average_per_day"`       // Average check-ins per day
	CompletionRate      float64 `json:"completion_rate"`       // Percentage of days with check-ins
	MostProductiveDay   string  `json:"most_productive_day"`   // Date with most check-ins (YYYY-MM-DD)
	MostProductiveCount int     `json:"most_productive_count"` // Check-in count on most productive day
	CurrentStreak       int     `json:"current_streak"`        // Current consecutive days with check-ins
	LongestStreak       int     `json:"longest_streak"`        // Longest streak in this month
}

// CategoryBreakdown represents check-in distribution by category
type CategoryBreakdown struct {
	CategoryID   *uuid.UUID `json:"category_id,omitempty"`
	CategoryName string     `json:"category_name"`
	Count        int        `json:"count"`
	Percentage   float64    `json:"percentage"`
}

// MonthlyCalendar represents the full calendar view for a month
type MonthlyCalendar struct {
	Year            int                  `json:"year"`
	Month           int                  `json:"month"`             // 1-12
	MonthName       string               `json:"month_name"`        // e.g., "January"
	StartDay        StartDay             `json:"start_day"`         // "sunday" or "monday"
	Weeks           []*CalendarWeek      `json:"weeks"`             // 4-6 weeks depending on month
	Summary         *MonthlySummary      `json:"summary"`           // Monthly aggregate statistics
	Categories      []*CategoryBreakdown `json:"categories"`        // Category breakdown
	PreviousMonth   string               `json:"previous_month"`    // YYYY-MM format for navigation
	NextMonth       string               `json:"next_month"`        // YYYY-MM format for navigation
	GeneratedAt     time.Time            `json:"generated_at"`      // Timestamp of generation (for caching)
	CacheExpiration time.Time            `json:"cache_expiration"`  // When this cache expires
}

// ColorIntensityLevel maps completion percentage to intensity (0-4)
// 0: No check-ins (0%)
// 1: Low (1-25%)
// 2: Medium-low (26-50%)
// 3: Medium-high (51-75%)
// 4: High (76-100%)
func GetColorIntensity(completionPercent float64) int {
	if completionPercent == 0 {
		return 0
	} else if completionPercent <= 25 {
		return 1
	} else if completionPercent <= 50 {
		return 2
	} else if completionPercent <= 75 {
		return 3
	}
	return 4
}

// CalculateCompletionPercent calculates the percentage of day covered by check-ins
// Based on the ideal of checking in every 15 minutes (96 check-ins per day)
// Or based on total minutes tracked vs 1440 minutes in a day
func CalculateCompletionPercent(totalMinutes int) float64 {
	const minutesPerDay = 1440 // 24 hours * 60 minutes
	if totalMinutes >= minutesPerDay {
		return 100.0
	}
	return (float64(totalMinutes) / float64(minutesPerDay)) * 100.0
}

// HeatmapDay represents a single day in the heatmap view
type HeatmapDay struct {
	Date              string  `json:"date"`               // YYYY-MM-DD format
	CheckinCount      int     `json:"checkin_count"`      // Number of check-ins on this day
	TotalMinutes      int     `json:"total_minutes"`      // Total duration of all check-ins
	CompletionPercent float64 `json:"completion_percent"` // Percentage of day covered (0-100)
	ColorIntensity    int     `json:"color_intensity"`    // Color intensity level (0-4)
}

// HeatmapData represents heatmap visualization data for a date range
type HeatmapData struct {
	StartDate   string        `json:"start_date"`   // YYYY-MM-DD format
	EndDate     string        `json:"end_date"`     // YYYY-MM-DD format
	Days        []*HeatmapDay `json:"days"`         // Array of days with activity data
	TotalDays   int           `json:"total_days"`   // Total number of days in range
	ActiveDays  int           `json:"active_days"`  // Days with at least one check-in
	TotalCheckins int         `json:"total_checkins"` // Total check-ins in the range
	GeneratedAt time.Time     `json:"generated_at"` // Timestamp of generation
}
