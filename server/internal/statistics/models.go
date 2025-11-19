package statistics

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// WeekStartDay represents the first day of the week for calculations
type WeekStartDay string

const (
	WeekStartSunday WeekStartDay = "sunday"
	WeekStartMonday WeekStartDay = "monday"
)

// TimeOfDay represents different periods of the day
type TimeOfDay string

const (
	TimeOfDayMorning   TimeOfDay = "morning"   // 06:00 - 12:00
	TimeOfDayAfternoon TimeOfDay = "afternoon" // 12:00 - 18:00
	TimeOfDayEvening   TimeOfDay = "evening"   // 18:00 - 23:00
	TimeOfDayNight     TimeOfDay = "night"     // 23:00 - 06:00
)

// DayOfWeekStats represents statistics for a specific day of the week
type DayOfWeekStats struct {
	DayOfWeek    string  `json:"day_of_week"`    // Monday, Tuesday, etc.
	CheckinCount int     `json:"checkin_count"`  // Total check-ins on this day
	TotalMinutes int     `json:"total_minutes"`  // Total duration in minutes
	AverageCount float64 `json:"average_count"`  // Average check-ins per occurrence
	Percentage   float64 `json:"percentage"`     // Percentage of weekly total
}

// TimeDistribution represents check-in distribution across time periods
type TimeDistribution struct {
	TimeOfDay    TimeOfDay `json:"time_of_day"`
	CheckinCount int       `json:"checkin_count"`
	TotalMinutes int       `json:"total_minutes"`
	Percentage   float64   `json:"percentage"`
}

// CategoryStats represents statistics for a specific category
type CategoryStats struct {
	CategoryID   *uuid.UUID `json:"category_id,omitempty"`
	CategoryName string     `json:"category_name"`
	CheckinCount int        `json:"checkin_count"`
	TotalMinutes int        `json:"total_minutes"`
	Percentage   float64    `json:"percentage"`
	AveragePerDay float64   `json:"average_per_day"`
}

// WeekComparison represents week-over-week comparison
type WeekComparison struct {
	PreviousWeekTotal    int     `json:"previous_week_total"`
	CurrentWeekTotal     int     `json:"current_week_total"`
	Change               int     `json:"change"`                 // Difference in check-ins
	ChangePercentage     float64 `json:"change_percentage"`      // Percentage change
	IsImprovement        bool    `json:"is_improvement"`         // True if current > previous
	PreviousTotalMinutes int     `json:"previous_total_minutes"`
	CurrentTotalMinutes  int     `json:"current_total_minutes"`
	MinutesChange        int     `json:"minutes_change"`
	MinutesChangePercent float64 `json:"minutes_change_percent"`
}

// StreakInfo represents streak information
type StreakInfo struct {
	CurrentStreak     int    `json:"current_streak"`      // Current consecutive days with check-ins
	LongestStreak     int    `json:"longest_streak"`      // Longest streak in this week
	IsStreakActive    bool   `json:"is_streak_active"`    // True if streak extends to today
	LastCheckinDate   string `json:"last_checkin_date"`   // Date of last check-in (YYYY-MM-DD)
	NextMilestone     int    `json:"next_milestone"`      // Next streak milestone (7, 14, 21, 30, etc.)
	DaysUntilMilestone int   `json:"days_until_milestone"`
}

// WeeklySummary represents aggregate statistics for the week
type WeeklySummary struct {
	TotalCheckins     int     `json:"total_checkins"`      // Total check-ins in the week
	TotalMinutes      int     `json:"total_minutes"`       // Total duration in minutes
	DaysWithCheckins  int     `json:"days_with_checkins"`  // Number of days with at least one check-in
	AveragePerDay     float64 `json:"average_per_day"`     // Average check-ins per day
	CompletionRate    float64 `json:"completion_rate"`     // Percentage of days with check-ins (out of 7)
	MostProductiveDay string  `json:"most_productive_day"` // Day with most check-ins (YYYY-MM-DD)
	MostProductiveCount int   `json:"most_productive_count"`
	LeastProductiveDay string  `json:"least_productive_day,omitempty"` // Day with least check-ins (excluding 0)
}

// DailyBreakdown represents daily statistics within the week
type DailyBreakdown struct {
	Date         string  `json:"date"`          // YYYY-MM-DD
	DayOfWeek    string  `json:"day_of_week"`   // Monday, Tuesday, etc.
	CheckinCount int     `json:"checkin_count"`
	TotalMinutes int     `json:"total_minutes"`
	IsToday      bool    `json:"is_today"`
	HasCheckins  bool    `json:"has_checkins"`
	CompletionPercent float64 `json:"completion_percent"` // Based on 1440 minutes per day
}

// WeeklyStatistics represents comprehensive weekly statistics and analysis
type WeeklyStatistics struct {
	// Time period
	Year         int          `json:"year"`
	WeekNumber   int          `json:"week_number"`   // ISO week number (1-53)
	StartDate    string       `json:"start_date"`    // YYYY-MM-DD
	EndDate      string       `json:"end_date"`      // YYYY-MM-DD
	WeekStartDay WeekStartDay `json:"week_start_day"` // sunday or monday
	Timezone     string       `json:"timezone"`      // IANA timezone

	// Summary statistics
	Summary *WeeklySummary `json:"summary"`

	// Daily breakdown
	DailyBreakdown []*DailyBreakdown `json:"daily_breakdown"` // 7 days

	// Day-of-week analysis
	DayOfWeekAnalysis []*DayOfWeekStats `json:"day_of_week_analysis"`

	// Time distribution
	TimeDistribution []*TimeDistribution `json:"time_distribution"`

	// Category breakdown
	CategoryBreakdown []*CategoryStats `json:"category_breakdown"`

	// Week-over-week comparison
	Comparison *WeekComparison `json:"comparison"`

	// Streak information
	Streak *StreakInfo `json:"streak"`

	// Navigation
	PreviousWeek string    `json:"previous_week"` // YYYY-Www format (ISO week)
	NextWeek     string    `json:"next_week"`
	GeneratedAt  time.Time `json:"generated_at"`
	CacheExpiration time.Time `json:"cache_expiration,omitempty"`
}

// GetTimeOfDay determines the time of day for a given hour
func GetTimeOfDay(hour int) TimeOfDay {
	switch {
	case hour >= 6 && hour < 12:
		return TimeOfDayMorning
	case hour >= 12 && hour < 18:
		return TimeOfDayAfternoon
	case hour >= 18 && hour < 23:
		return TimeOfDayEvening
	default:
		return TimeOfDayNight
	}
}

// GetDayOfWeekName returns the name of the day for a given time.Time
func GetDayOfWeekName(t time.Time) string {
	return t.Weekday().String()
}

// CalculateWeekBounds calculates the start and end of a week for a given date
func CalculateWeekBounds(date time.Time, startDay WeekStartDay, loc *time.Location) (start, end time.Time) {
	// Normalize to start of day in the specified timezone
	normalized := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, loc)

	weekday := int(normalized.Weekday())

	var daysBack int
	if startDay == WeekStartSunday {
		// Sunday = 0, so we go back weekday days
		daysBack = weekday
	} else {
		// Monday start: Sunday = 0 becomes 6 days back, Monday = 1 becomes 0 days back
		if weekday == 0 {
			daysBack = 6
		} else {
			daysBack = weekday - 1
		}
	}

	start = normalized.AddDate(0, 0, -daysBack)
	end = start.AddDate(0, 0, 7)

	return start, end
}

// GetISOWeekNumber returns the ISO week number (1-53) for a given date
func GetISOWeekNumber(date time.Time) (year, week int) {
	return date.ISOWeek()
}

// FormatISOWeek formats a year and week number as YYYY-Www (e.g., "2024-W15")
func FormatISOWeek(year, week int) string {
	return time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).
		Add(time.Duration((week-1)*7*24) * time.Hour).
		Format("2006") + "-W" + fmt.Sprintf("%02d", week)
}

// CalculateStreakMilestones returns the next milestone and days until it
func CalculateStreakMilestones(currentStreak int) (nextMilestone, daysUntil int) {
	milestones := []int{7, 14, 21, 30, 60, 90, 180, 365}

	for _, milestone := range milestones {
		if currentStreak < milestone {
			return milestone, milestone - currentStreak
		}
	}

	// If beyond all milestones, next is +100
	nextMilestone = ((currentStreak / 100) + 1) * 100
	daysUntil = nextMilestone - currentStreak

	return nextMilestone, daysUntil
}
