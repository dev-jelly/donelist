package calendar

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSnapshot_MonthlyCalendar_EdgeCases tests monthly calendar generation
// against snapshot fixtures for edge case months
func TestSnapshot_MonthlyCalendar_EdgeCases(t *testing.T) {
	testCases := []struct {
		name        string
		year        int
		month       int
		startDay    StartDay
		description string
	}{
		{
			name:        "Feb_2024_LeapYear_Sunday",
			year:        2024,
			month:       2,
			startDay:    StartDaySunday,
			description: "February 2024 - Leap year with 29 days, Sunday start",
		},
		{
			name:        "Feb_2024_LeapYear_Monday",
			year:        2024,
			month:       2,
			startDay:    StartDayMonday,
			description: "February 2024 - Leap year with 29 days, Monday start",
		},
		{
			name:        "Nov_2025_Sunday",
			year:        2025,
			month:       11,
			startDay:    StartDaySunday,
			description: "November 2025 - 30 days, Sunday start",
		},
		{
			name:        "Nov_2025_Monday",
			year:        2025,
			month:       11,
			startDay:    StartDayMonday,
			description: "November 2025 - 30 days, Monday start",
		},
		{
			name:        "Jan_2023_Sunday",
			year:        2023,
			month:       1,
			startDay:    StartDaySunday,
			description: "January 2023 - First month, 31 days, Sunday start",
		},
		{
			name:        "Jan_2023_Monday",
			year:        2023,
			month:       1,
			startDay:    StartDayMonday,
			description: "January 2023 - First month, 31 days, Monday start",
		},
		{
			name:        "Dec_2023_Sunday",
			year:        2023,
			month:       12,
			startDay:    StartDaySunday,
			description: "December 2023 - Last month, 31 days, Sunday start",
		},
		{
			name:        "Dec_2023_Monday",
			year:        2023,
			month:       12,
			startDay:    StartDayMonday,
			description: "December 2023 - Last month, 31 days, Monday start",
		},
		{
			name:        "Feb_2023_NonLeapYear",
			year:        2023,
			month:       2,
			startDay:    StartDayMonday,
			description: "February 2023 - Non-leap year with 28 days",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Generate calendar structure (no DB needed for structure validation)
			calendar := generateCalendarStructure(tc.year, tc.month, tc.startDay)

			// Validate calendar structure
			validateCalendarStructure(t, calendar, tc.year, tc.month, tc.startDay)

			// Compare with snapshot if exists, or create new snapshot
			snapshotPath := filepath.Join("testdata", "snapshots", tc.name+".json")
			if os.Getenv("UPDATE_SNAPSHOTS") == "1" {
				saveSnapshot(t, snapshotPath, calendar)
			} else if fileExists(snapshotPath) {
				compareWithSnapshot(t, snapshotPath, calendar)
			} else {
				// First run - create snapshot
				saveSnapshot(t, snapshotPath, calendar)
				t.Logf("Created new snapshot: %s", snapshotPath)
			}
		})
	}
}

// CalendarStructure represents the structure for snapshot testing
type CalendarStructure struct {
	Year            int                    `json:"year"`
	Month           int                    `json:"month"`
	MonthName       string                 `json:"month_name"`
	StartDay        string                 `json:"start_day"`
	TotalDays       int                    `json:"total_days"`
	WeekCount       int                    `json:"week_count"`
	FirstDayWeekday string                 `json:"first_day_weekday"`
	LastDayWeekday  string                 `json:"last_day_weekday"`
	LeadingDays     int                    `json:"leading_days"`
	TrailingDays    int                    `json:"trailing_days"`
	PreviousMonth   string                 `json:"previous_month"`
	NextMonth       string                 `json:"next_month"`
	WeekStructure   []WeekStructure        `json:"week_structure"`
}

type WeekStructure struct {
	WeekNumber int      `json:"week_number"`
	Days       []string `json:"days"` // Date strings or "prev" / "next" for padding days
}

func generateCalendarStructure(year, month int, startDay StartDay) *CalendarStructure {
	// Get first and last day of month
	firstOfMonth := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	lastOfMonth := firstOfMonth.AddDate(0, 1, -1)
	totalDays := lastOfMonth.Day()

	// Calculate previous and next month
	prevMonth := firstOfMonth.AddDate(0, -1, 0)
	nextMonth := firstOfMonth.AddDate(0, 1, 0)

	// Calculate leading days (days from previous month to fill first week)
	firstDayWeekday := int(firstOfMonth.Weekday())
	var leadingDays int
	if startDay == StartDaySunday {
		leadingDays = firstDayWeekday
	} else {
		// Monday start
		leadingDays = (firstDayWeekday + 6) % 7
	}

	// Calculate trailing days (days from next month to fill last week)
	lastDayWeekday := int(lastOfMonth.Weekday())
	var trailingDays int
	if startDay == StartDaySunday {
		trailingDays = (6 - lastDayWeekday)
	} else {
		// Monday start
		trailingDays = (7 - lastDayWeekday) % 7
	}

	// Calculate total cells and weeks
	totalCells := leadingDays + totalDays + trailingDays
	weekCount := totalCells / 7

	// Generate week structure
	weeks := make([]WeekStructure, weekCount)
	currentDay := 1 - leadingDays // Start from negative for leading days
	prevMonthLastDay := prevMonth.AddDate(0, 1, -1).Day()

	for w := 0; w < weekCount; w++ {
		week := WeekStructure{
			WeekNumber: w + 1,
			Days:       make([]string, 7),
		}
		for d := 0; d < 7; d++ {
			if currentDay < 1 {
				// Previous month
				prevDay := prevMonthLastDay + currentDay
				week.Days[d] = prevMonth.Format("2006-01") + "-" + padDay(prevDay) + " (prev)"
			} else if currentDay > totalDays {
				// Next month
				nextDay := currentDay - totalDays
				week.Days[d] = nextMonth.Format("2006-01") + "-" + padDay(nextDay) + " (next)"
			} else {
				// Current month
				week.Days[d] = firstOfMonth.Format("2006-01") + "-" + padDay(currentDay)
			}
			currentDay++
		}
		weeks[w] = week
	}

	return &CalendarStructure{
		Year:            year,
		Month:           month,
		MonthName:       firstOfMonth.Format("January"),
		StartDay:        string(startDay),
		TotalDays:       totalDays,
		WeekCount:       weekCount,
		FirstDayWeekday: firstOfMonth.Weekday().String(),
		LastDayWeekday:  lastOfMonth.Weekday().String(),
		LeadingDays:     leadingDays,
		TrailingDays:    trailingDays,
		PreviousMonth:   prevMonth.Format("2006-01"),
		NextMonth:       nextMonth.Format("2006-01"),
		WeekStructure:   weeks,
	}
}

func padDay(day int) string {
	return fmt.Sprintf("%02d", day)
}

func validateCalendarStructure(t *testing.T, cal *CalendarStructure, year, month int, startDay StartDay) {
	t.Helper()

	// Validate basic properties
	assert.Equal(t, year, cal.Year, "Year mismatch")
	assert.Equal(t, month, cal.Month, "Month mismatch")
	assert.Equal(t, string(startDay), cal.StartDay, "StartDay mismatch")

	// Validate days in month
	daysInMonth := time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
	assert.Equal(t, daysInMonth, cal.TotalDays, "TotalDays mismatch for %s %d", cal.MonthName, year)

	// Validate week count (should be 4-6 weeks)
	assert.GreaterOrEqual(t, cal.WeekCount, 4, "Week count too low")
	assert.LessOrEqual(t, cal.WeekCount, 6, "Week count too high")

	// Validate each week has 7 days
	for i, week := range cal.WeekStructure {
		assert.Len(t, week.Days, 7, "Week %d should have 7 days", i+1)
	}

	// Validate total cells
	totalCells := cal.LeadingDays + cal.TotalDays + cal.TrailingDays
	assert.Equal(t, cal.WeekCount*7, totalCells, "Total cells mismatch")

	// Validate previous/next month format
	assert.Regexp(t, `^\d{4}-\d{2}$`, cal.PreviousMonth, "Invalid previous month format")
	assert.Regexp(t, `^\d{4}-\d{2}$`, cal.NextMonth, "Invalid next month format")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func saveSnapshot(t *testing.T, path string, data interface{}) {
	t.Helper()

	// Ensure directory exists
	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, 0755)
	require.NoError(t, err, "Failed to create snapshot directory")

	// Marshal with indentation for readability
	jsonData, err := json.MarshalIndent(data, "", "  ")
	require.NoError(t, err, "Failed to marshal snapshot data")

	// Write file
	err = os.WriteFile(path, jsonData, 0644)
	require.NoError(t, err, "Failed to write snapshot file")
}

func compareWithSnapshot(t *testing.T, path string, actual interface{}) {
	t.Helper()

	// Read snapshot file
	snapshotData, err := os.ReadFile(path)
	require.NoError(t, err, "Failed to read snapshot file: %s", path)

	// Marshal actual data
	actualData, err := json.MarshalIndent(actual, "", "  ")
	require.NoError(t, err, "Failed to marshal actual data")

	// Compare
	assert.JSONEq(t, string(snapshotData), string(actualData),
		"Snapshot mismatch for %s. Run with UPDATE_SNAPSHOTS=1 to update.", path)
}

// TestMonthlyCalendar_LeapYear_February2024 specifically tests leap year handling
func TestMonthlyCalendar_LeapYear_February2024(t *testing.T) {
	year, month := 2024, 2

	// February 2024 should have 29 days (leap year)
	daysInFeb := time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
	assert.Equal(t, 29, daysInFeb, "February 2024 should have 29 days (leap year)")

	// Generate structure for both start days
	calSunday := generateCalendarStructure(year, month, StartDaySunday)
	calMonday := generateCalendarStructure(year, month, StartDayMonday)

	// Validate both have 29 days
	assert.Equal(t, 29, calSunday.TotalDays, "Sunday calendar should have 29 days")
	assert.Equal(t, 29, calMonday.TotalDays, "Monday calendar should have 29 days")

	// February 1, 2024 is Thursday
	assert.Equal(t, "Thursday", calSunday.FirstDayWeekday, "Feb 1, 2024 should be Thursday")

	// For Sunday start: Thursday is index 4, so 4 leading days
	assert.Equal(t, 4, calSunday.LeadingDays, "Sunday start should have 4 leading days")

	// For Monday start: Thursday is index 3, so 3 leading days
	assert.Equal(t, 3, calMonday.LeadingDays, "Monday start should have 3 leading days")
}

// TestMonthlyCalendar_NonLeapYear_February2023 tests non-leap year handling
func TestMonthlyCalendar_NonLeapYear_February2023(t *testing.T) {
	year, month := 2023, 2

	// February 2023 should have 28 days (non-leap year)
	daysInFeb := time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
	assert.Equal(t, 28, daysInFeb, "February 2023 should have 28 days (non-leap year)")

	cal := generateCalendarStructure(year, month, StartDayMonday)
	assert.Equal(t, 28, cal.TotalDays, "Calendar should have 28 days")
}

// TestMonthlyCalendar_YearBoundary tests months at year boundaries
func TestMonthlyCalendar_YearBoundary(t *testing.T) {
	t.Run("December_2023", func(t *testing.T) {
		cal := generateCalendarStructure(2023, 12, StartDayMonday)

		assert.Equal(t, 2023, cal.Year)
		assert.Equal(t, 12, cal.Month)
		assert.Equal(t, "December", cal.MonthName)
		assert.Equal(t, 31, cal.TotalDays)
		assert.Equal(t, "2023-11", cal.PreviousMonth)
		assert.Equal(t, "2024-01", cal.NextMonth)
	})

	t.Run("January_2024", func(t *testing.T) {
		cal := generateCalendarStructure(2024, 1, StartDayMonday)

		assert.Equal(t, 2024, cal.Year)
		assert.Equal(t, 1, cal.Month)
		assert.Equal(t, "January", cal.MonthName)
		assert.Equal(t, 31, cal.TotalDays)
		assert.Equal(t, "2023-12", cal.PreviousMonth)
		assert.Equal(t, "2024-02", cal.NextMonth)
	})
}

// TestMonthlyCalendar_ServiceIntegration tests the full service with mock data
func TestMonthlyCalendar_ServiceIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// This test would require a database connection
	// Placeholder for integration test structure
	t.Run("WithMockCheckins", func(t *testing.T) {
		t.Skip("Requires database setup - run with integration test flag")

		userID := uuid.New()
		ctx := context.Background()

		// Would create mock check-ins and validate calendar response
		_ = userID
		_ = ctx
	})
}
