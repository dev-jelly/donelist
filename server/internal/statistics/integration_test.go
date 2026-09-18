package statistics_test

import (
	"context"
	"testing"
	"time"

	"github.com/dev-jelly/donelist/internal/statistics"
	"github.com/dev-jelly/donelist/pkg/database"
	"github.com/dev-jelly/donelist/pkg/logger"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWeeklyStatisticsIntegration tests the complete flow of weekly statistics generation
func TestWeeklyStatisticsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test database
	cfg := database.PostgresConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "postgres",
		Password: "postgres",
		Database: "donelist_test",
		SSLMode:  "disable",
	}

	log, err := logger.New("test", "info")
	require.NoError(t, err)

	db, err := database.NewPostgres(cfg, log)
	if err != nil {
		t.Skipf("Could not connect to test database: %v", err)
		return
	}
	defer database.Close(db, log)

	// Create test repositories and service
	repo := statistics.NewRepository(db)
	service := statistics.NewService(repo, log)

	// Create test user
	userID := uuid.New()
	ctx := context.Background()

	// Insert test user
	_, err = db.ExecContext(ctx, `
		INSERT INTO users (id, email, username, password_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, userID, "test@example.com", "testuser", "hashedpassword")
	require.NoError(t, err)

	// Insert test check-ins across a week
	baseDate := time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC) // Monday
	checkinData := []struct {
		dayOffset    int
		hour         int
		count        int
		minutes      int
	}{
		{0, 9, 5, 30},   // Monday morning
		{0, 14, 3, 45},  // Monday afternoon
		{1, 10, 7, 60},  // Tuesday morning
		{1, 15, 4, 30},  // Tuesday afternoon
		{2, 11, 6, 45},  // Wednesday morning
		{3, 9, 8, 30},   // Thursday morning
		{3, 19, 2, 60},  // Thursday evening
		{4, 13, 5, 45},  // Friday afternoon
	}

	// Insert check-ins
	for _, data := range checkinData {
		checkinDate := baseDate.AddDate(0, 0, data.dayOffset).Add(time.Duration(data.hour) * time.Hour)
		for i := 0; i < data.count; i++ {
			_, err = db.ExecContext(ctx, `
				INSERT INTO checkins (id, user_id, checkin_time, duration_minutes, description, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
			`, uuid.New(), userID, checkinDate.Add(time.Duration(i)*time.Minute), data.minutes, "Test checkin", )
			require.NoError(t, err)
		}
	}

	// Test GetWeeklyStatistics
	t.Run("GetWeeklyStatistics", func(t *testing.T) {
		opts := statistics.GetWeeklyOptions{
			UserID:       userID,
			Date:         baseDate.AddDate(0, 0, 2), // Wednesday in the test week
			WeekStartDay: statistics.WeekStartMonday,
			Timezone:     "UTC",
		}

		stats, err := service.GetWeeklyStatistics(ctx, opts)
		require.NoError(t, err)
		require.NotNil(t, stats)

		// Verify structure
		assert.Equal(t, 2024, stats.Year)
		assert.Equal(t, "2024-01-08", stats.StartDate)
		assert.Equal(t, "2024-01-14", stats.EndDate)
		assert.Equal(t, statistics.WeekStartMonday, stats.WeekStartDay)
		assert.Equal(t, "UTC", stats.Timezone)

		// Verify summary
		assert.NotNil(t, stats.Summary)
		assert.Greater(t, stats.Summary.TotalCheckins, 0, "Should have check-ins")
		assert.Greater(t, stats.Summary.TotalMinutes, 0, "Should have total minutes")
		assert.Greater(t, stats.Summary.DaysWithCheckins, 0, "Should have days with check-ins")
		assert.LessOrEqual(t, stats.Summary.DaysWithCheckins, 7, "Cannot have more than 7 days")
		assert.Greater(t, stats.Summary.AveragePerDay, 0.0, "Should have average per day")
		assert.NotEmpty(t, stats.Summary.MostProductiveDay, "Should identify most productive day")

		// Verify daily breakdown
		assert.Len(t, stats.DailyBreakdown, 7, "Should have 7 days")
		var daysWithData int
		for _, day := range stats.DailyBreakdown {
			assert.NotEmpty(t, day.Date, "Each day should have a date")
			assert.NotEmpty(t, day.DayOfWeek, "Each day should have day of week")
			if day.HasCheckins {
				daysWithData++
				assert.Greater(t, day.CheckinCount, 0, "Days with check-ins should have count > 0")
				assert.Greater(t, day.TotalMinutes, 0, "Days with check-ins should have minutes > 0")
			}
		}
		assert.Equal(t, stats.Summary.DaysWithCheckins, daysWithData, "Daily breakdown should match summary")

		// Verify day-of-week analysis
		assert.NotEmpty(t, stats.DayOfWeekAnalysis, "Should have day-of-week analysis")
		assert.LessOrEqual(t, len(stats.DayOfWeekAnalysis), 7, "Should have at most 7 days")
		totalPercentage := 0.0
		for _, day := range stats.DayOfWeekAnalysis {
			assert.NotEmpty(t, day.DayOfWeek, "Should have day name")
			totalPercentage += day.Percentage
		}
		if stats.Summary.TotalCheckins > 0 {
			assert.InDelta(t, 100.0, totalPercentage, 0.1, "Percentages should sum to ~100%")
		}

		// Verify time distribution
		assert.NotEmpty(t, stats.TimeDistribution, "Should have time distribution")
		timePercentageTotal := 0.0
		for _, dist := range stats.TimeDistribution {
			assert.NotEmpty(t, dist.TimeOfDay, "Should have time of day")
			timePercentageTotal += dist.Percentage
		}
		if stats.Summary.TotalCheckins > 0 {
			assert.InDelta(t, 100.0, timePercentageTotal, 0.1, "Time percentages should sum to ~100%")
		}

		// Verify category breakdown
		assert.NotNil(t, stats.CategoryBreakdown, "Should have category breakdown (can be empty)")
		categoryPercentageTotal := 0.0
		for _, cat := range stats.CategoryBreakdown {
			assert.NotEmpty(t, cat.CategoryName, "Should have category name")
			assert.Greater(t, cat.CheckinCount, 0, "Category should have check-ins")
			categoryPercentageTotal += cat.Percentage
		}
		if len(stats.CategoryBreakdown) > 0 {
			assert.InDelta(t, 100.0, categoryPercentageTotal, 0.1, "Category percentages should sum to ~100%")
		}

		// Verify comparison
		assert.NotNil(t, stats.Comparison, "Should have comparison")
		assert.Equal(t, stats.Summary.TotalCheckins, stats.Comparison.CurrentWeekTotal, "Current week totals should match")

		// Verify streak
		assert.NotNil(t, stats.Streak, "Should have streak info")
		assert.GreaterOrEqual(t, stats.Streak.CurrentStreak, 0, "Streak should be >= 0")
		assert.GreaterOrEqual(t, stats.Streak.LongestStreak, 0, "Longest streak should be >= 0")

		// Verify navigation
		assert.NotEmpty(t, stats.PreviousWeek, "Should have previous week reference")
		assert.NotEmpty(t, stats.NextWeek, "Should have next week reference")

		// Verify metadata
		assert.False(t, stats.GeneratedAt.IsZero(), "Should have generation time")
	})

	// Test with Sunday start
	t.Run("GetWeeklyStatistics_SundayStart", func(t *testing.T) {
		opts := statistics.GetWeeklyOptions{
			UserID:       userID,
			Date:         baseDate.AddDate(0, 0, 2),
			WeekStartDay: statistics.WeekStartSunday,
			Timezone:     "UTC",
		}

		stats, err := service.GetWeeklyStatistics(ctx, opts)
		require.NoError(t, err)
		require.NotNil(t, stats)

		assert.Equal(t, statistics.WeekStartSunday, stats.WeekStartDay)
		assert.Len(t, stats.DailyBreakdown, 7)
		// First day should be Sunday
		assert.Equal(t, "Sunday", stats.DailyBreakdown[0].DayOfWeek)
	})

	// Test with different timezone
	t.Run("GetWeeklyStatistics_Timezone", func(t *testing.T) {
		opts := statistics.GetWeeklyOptions{
			UserID:       userID,
			Date:         baseDate.AddDate(0, 0, 2),
			WeekStartDay: statistics.WeekStartMonday,
			Timezone:     "America/New_York",
		}

		stats, err := service.GetWeeklyStatistics(ctx, opts)
		require.NoError(t, err)
		require.NotNil(t, stats)

		assert.Equal(t, "America/New_York", stats.Timezone)
	})

	// Cleanup
	_, _ = db.ExecContext(ctx, "DELETE FROM checkins WHERE user_id = $1", userID)
	_, _ = db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", userID)
}
