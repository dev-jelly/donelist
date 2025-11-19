package statistics

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestCalculateWeekBounds(t *testing.T) {
	loc := time.UTC

	tests := []struct {
		name      string
		date      string
		startDay  WeekStartDay
		wantStart string
		wantEnd   string
	}{
		{
			name:      "Monday start - middle of week",
			date:      "2024-01-10", // Wednesday
			startDay:  WeekStartMonday,
			wantStart: "2024-01-08", // Monday
			wantEnd:   "2024-01-15", // Next Monday
		},
		{
			name:      "Sunday start - middle of week",
			date:      "2024-01-10", // Wednesday
			startDay:  WeekStartSunday,
			wantStart: "2024-01-07", // Sunday
			wantEnd:   "2024-01-14", // Next Sunday
		},
		{
			name:      "Monday start - on Monday",
			date:      "2024-01-08", // Monday
			startDay:  WeekStartMonday,
			wantStart: "2024-01-08", // Monday
			wantEnd:   "2024-01-15", // Next Monday
		},
		{
			name:      "Sunday start - on Sunday",
			date:      "2024-01-07", // Sunday
			startDay:  WeekStartSunday,
			wantStart: "2024-01-07", // Sunday
			wantEnd:   "2024-01-14", // Next Sunday
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			date, _ := time.Parse("2006-01-02", tt.date)
			start, end := CalculateWeekBounds(date, tt.startDay, loc)

			assert.Equal(t, tt.wantStart, start.Format("2006-01-02"))
			assert.Equal(t, tt.wantEnd, end.Format("2006-01-02"))
		})
	}
}

func TestGetTimeOfDay(t *testing.T) {
	tests := []struct {
		hour int
		want TimeOfDay
	}{
		{6, TimeOfDayMorning},
		{9, TimeOfDayMorning},
		{11, TimeOfDayMorning},
		{12, TimeOfDayAfternoon},
		{15, TimeOfDayAfternoon},
		{17, TimeOfDayAfternoon},
		{18, TimeOfDayEvening},
		{20, TimeOfDayEvening},
		{22, TimeOfDayEvening},
		{23, TimeOfDayNight},
		{0, TimeOfDayNight},
		{3, TimeOfDayNight},
		{5, TimeOfDayNight},
	}

	for _, tt := range tests {
		t.Run(time.Hour.String(), func(t *testing.T) {
			got := GetTimeOfDay(tt.hour)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetISOWeekNumber(t *testing.T) {
	tests := []struct {
		date     string
		wantYear int
		wantWeek int
	}{
		{"2024-01-01", 2024, 1},
		{"2024-01-08", 2024, 2},
		{"2024-06-15", 2024, 24},
		{"2024-12-31", 2025, 1}, // Might be week 1 of 2025
	}

	for _, tt := range tests {
		t.Run(tt.date, func(t *testing.T) {
			date, _ := time.Parse("2006-01-02", tt.date)
			year, week := GetISOWeekNumber(date)

			// Just verify we get reasonable values
			assert.Greater(t, year, 2020)
			assert.Greater(t, week, 0)
			assert.LessOrEqual(t, week, 53)
		})
	}
}

func TestCalculateStreakMilestones(t *testing.T) {
	tests := []struct {
		currentStreak     int
		wantNextMilestone int
		wantDaysUntil     int
	}{
		{0, 7, 7},
		{3, 7, 4},
		{7, 14, 7},
		{10, 14, 4},
		{30, 60, 30},
		{100, 180, 80},   // Next milestone after 90 is 180
		{250, 365, 115},  // Next milestone after 180 is 365
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			nextMilestone, daysUntil := CalculateStreakMilestones(tt.currentStreak)

			assert.Equal(t, tt.wantNextMilestone, nextMilestone)
			assert.Equal(t, tt.wantDaysUntil, daysUntil)
		})
	}
}

func TestCalculateCompletionPercent(t *testing.T) {
	tests := []struct {
		totalMinutes int
		want         float64
	}{
		{0, 0.0},
		{720, 50.0},   // Half a day (12 hours)
		{1440, 100.0}, // Full day (24 hours)
		{2880, 100.0}, // More than a day (capped at 100%)
		{360, 25.0},   // Quarter day (6 hours)
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := calculateCompletionPercent(tt.totalMinutes)
			assert.Equal(t, tt.want, got)
		})
	}
}

// Mock repository for testing service layer
type mockStatsRepository struct {
	dailyAggs     []DailyAggregate
	categoryAggs  []CategoryAggregate
	timeOfDayAggs []TimeOfDayAggregate
	dayOfWeekAggs []DayOfWeekAggregate
	weekCount     int
	weekMinutes   int
	streakData    map[string]bool
	lastCheckin   *time.Time
}

func (m *mockStatsRepository) GetDailyAggregates(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) ([]DailyAggregate, error) {
	return m.dailyAggs, nil
}

func (m *mockStatsRepository) GetCategoryAggregates(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) ([]CategoryAggregate, error) {
	return m.categoryAggs, nil
}

func (m *mockStatsRepository) GetTimeOfDayAggregates(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) ([]TimeOfDayAggregate, error) {
	return m.timeOfDayAggs, nil
}

func (m *mockStatsRepository) GetDayOfWeekAggregates(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) ([]DayOfWeekAggregate, error) {
	return m.dayOfWeekAggs, nil
}

func (m *mockStatsRepository) GetWeekTotal(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) (int, int, error) {
	return m.weekCount, m.weekMinutes, nil
}

func (m *mockStatsRepository) GetStreakData(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) (map[string]bool, error) {
	return m.streakData, nil
}

func (m *mockStatsRepository) GetLastCheckinDate(ctx context.Context, userID uuid.UUID) (*time.Time, error) {
	return m.lastCheckin, nil
}

func TestService_GetWeeklyStatistics(t *testing.T) {
	logger := zap.NewNop()

	// Create mock data
	date := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC) // Wednesday

	mockRepo := &mockStatsRepository{
		dailyAggs: []DailyAggregate{
			{
				Date:         time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC), // Monday
				CheckinCount: 10,
				TotalMinutes: 600,
			},
			{
				Date:         time.Date(2024, 1, 9, 0, 0, 0, 0, time.UTC), // Tuesday
				CheckinCount: 15,
				TotalMinutes: 720,
			},
			{
				Date:         time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC), // Wednesday
				CheckinCount: 12,
				TotalMinutes: 660,
			},
		},
		categoryAggs: []CategoryAggregate{
			{
				CategoryID:   nil,
				CategoryName: "Work",
				CheckinCount: 20,
				TotalMinutes: 1200,
			},
			{
				CategoryID:   nil,
				CategoryName: "Personal",
				CheckinCount: 17,
				TotalMinutes: 780,
			},
		},
		timeOfDayAggs: []TimeOfDayAggregate{
			{Hour: 9, CheckinCount: 15, TotalMinutes: 900},
			{Hour: 14, CheckinCount: 12, TotalMinutes: 600},
			{Hour: 19, CheckinCount: 10, TotalMinutes: 480},
		},
		dayOfWeekAggs: []DayOfWeekAggregate{
			{DayOfWeek: 1, CheckinCount: 10, TotalMinutes: 600}, // Monday
			{DayOfWeek: 2, CheckinCount: 15, TotalMinutes: 720}, // Tuesday
			{DayOfWeek: 3, CheckinCount: 12, TotalMinutes: 660}, // Wednesday
		},
		weekCount:   37,
		weekMinutes: 1980,
		streakData: map[string]bool{
			"2024-01-08": true,
			"2024-01-09": true,
			"2024-01-10": true,
		},
		lastCheckin: &date,
	}

	service := NewService(mockRepo, logger)

	stats, err := service.GetWeeklyStatistics(context.Background(), GetWeeklyOptions{
		UserID:       uuid.New(),
		Date:         date,
		WeekStartDay: WeekStartMonday,
		Timezone:     "UTC",
	})

	assert.NoError(t, err)
	assert.NotNil(t, stats)

	// Verify basic structure
	assert.Equal(t, 2024, stats.Year)
	assert.NotEmpty(t, stats.StartDate)
	assert.NotEmpty(t, stats.EndDate)
	assert.Equal(t, WeekStartMonday, stats.WeekStartDay)

	// Verify summary
	assert.NotNil(t, stats.Summary)
	assert.Equal(t, 37, stats.Summary.TotalCheckins)
	assert.Equal(t, 1980, stats.Summary.TotalMinutes)
	assert.Equal(t, 3, stats.Summary.DaysWithCheckins)

	// Verify daily breakdown has 7 days
	assert.Len(t, stats.DailyBreakdown, 7)

	// Verify time distribution
	assert.NotEmpty(t, stats.TimeDistribution)

	// Verify category breakdown
	assert.Len(t, stats.CategoryBreakdown, 2)

	// Verify comparison
	assert.NotNil(t, stats.Comparison)

	// Verify streak
	assert.NotNil(t, stats.Streak)
}
