package statistics_test

import (
	"context"
	"testing"
	"time"

	"github.com/dev-jelly/donelist/internal/statistics"
	"github.com/dev-jelly/donelist/pkg/database"
	"github.com/dev-jelly/donelist/pkg/logger"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// Helper functions
func setupTestDatabase(t *testing.T) (*sqlx.DB, *zap.Logger) {
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
	}

	return db, log
}

func setupTestUser(t *testing.T, db *sqlx.DB, userID uuid.UUID) {
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `
		INSERT INTO users (id, email, username, password_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, userID, "test@example.com", "testuser", "hashedpassword")
	require.NoError(t, err)
}

func cleanupTestData(t *testing.T, db *sqlx.DB, userID uuid.UUID) {
	ctx := context.Background()
	_, _ = db.ExecContext(ctx, "DELETE FROM checkins WHERE user_id = $1", userID)
	_, _ = db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", userID)
}

// TestEdgeCases tests various edge cases and boundary conditions
func TestEdgeCases(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db, log := setupTestDatabase(t)
	defer database.Close(db, log)

	repo := statistics.NewRepository(db)
	service := statistics.NewService(repo, log)

	ctx := context.Background()

	t.Run("EmptyWeek", func(t *testing.T) {
		userID := uuid.New()
		setupTestUser(t, db, userID)

		opts := statistics.GetWeeklyOptions{
			UserID:       userID,
			Date:         time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC),
			WeekStartDay: statistics.WeekStartMonday,
			Timezone:     "UTC",
		}

		stats, err := service.GetWeeklyStatistics(ctx, opts)
		require.NoError(t, err)
		require.NotNil(t, stats)

		// Should return valid structure with zero values
		assert.Equal(t, 0, stats.Summary.TotalCheckins)
		assert.Equal(t, 0, stats.Summary.TotalMinutes)
		assert.Equal(t, 0, stats.Summary.DaysWithCheckins)
		assert.Equal(t, 0.0, stats.Summary.AveragePerDay)
		assert.Len(t, stats.DailyBreakdown, 7)
		assert.Empty(t, stats.Summary.MostProductiveDay)

		// All days should have no check-ins
		for _, day := range stats.DailyBreakdown {
			assert.False(t, day.HasCheckins)
			assert.Equal(t, 0, day.CheckinCount)
		}

		cleanupTestData(t, db, userID)
	})

	t.Run("SingleCheckin", func(t *testing.T) {
		userID := uuid.New()
		setupTestUser(t, db, userID)

		// Single check-in on Wednesday
		checkinDate := time.Date(2024, 1, 10, 14, 30, 0, 0, time.UTC)
		_, err := db.ExecContext(ctx, `
			INSERT INTO checkins (id, user_id, checkin_time, duration_minutes, description, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		`, uuid.New(), userID, checkinDate, 45, "Single checkin")
		require.NoError(t, err)

		opts := statistics.GetWeeklyOptions{
			UserID:       userID,
			Date:         time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
			WeekStartDay: statistics.WeekStartMonday,
			Timezone:     "UTC",
		}

		stats, err := service.GetWeeklyStatistics(ctx, opts)
		require.NoError(t, err)
		require.NotNil(t, stats)

		assert.Equal(t, 1, stats.Summary.TotalCheckins)
		assert.Equal(t, 45, stats.Summary.TotalMinutes)
		assert.Equal(t, 1, stats.Summary.DaysWithCheckins)
		assert.Equal(t, "2024-01-10", stats.Summary.MostProductiveDay)
		assert.Equal(t, 1, stats.Summary.MostProductiveCount)

		cleanupTestData(t, db, userID)
	})

	t.Run("WeekBoundary", func(t *testing.T) {
		userID := uuid.New()
		setupTestUser(t, db, userID)

		// Check-ins at week boundaries
		dates := []time.Time{
			time.Date(2024, 1, 8, 0, 0, 1, 0, time.UTC),     // First second of Monday
			time.Date(2024, 1, 14, 23, 59, 59, 0, time.UTC), // Last second of Sunday
		}

		for _, date := range dates {
			_, err := db.ExecContext(ctx, `
				INSERT INTO checkins (id, user_id, checkin_time, duration_minutes, description, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
			`, uuid.New(), userID, date, 30, "Boundary checkin")
			require.NoError(t, err)
		}

		opts := statistics.GetWeeklyOptions{
			UserID:       userID,
			Date:         time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
			WeekStartDay: statistics.WeekStartMonday,
			Timezone:     "UTC",
		}

		stats, err := service.GetWeeklyStatistics(ctx, opts)
		require.NoError(t, err)
		require.NotNil(t, stats)

		assert.Equal(t, 2, stats.Summary.TotalCheckins)
		assert.Equal(t, 2, stats.Summary.DaysWithCheckins)

		cleanupTestData(t, db, userID)
	})

	t.Run("TimezoneBoundary", func(t *testing.T) {
		userID := uuid.New()
		setupTestUser(t, db, userID)

		// Check-in at midnight UTC
		checkinDate := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)
		_, err := db.ExecContext(ctx, `
			INSERT INTO checkins (id, user_id, checkin_time, duration_minutes, description, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		`, uuid.New(), userID, checkinDate, 30, "Midnight checkin")
		require.NoError(t, err)

		// Test with different timezones
		timezones := []string{
			"UTC",
			"America/New_York", // UTC-5, would be Jan 9 19:00
			"Asia/Tokyo",       // UTC+9, would be Jan 10 09:00
		}

		for _, tz := range timezones {
			t.Run(tz, func(t *testing.T) {
				opts := statistics.GetWeeklyOptions{
					UserID:       userID,
					Date:         time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
					WeekStartDay: statistics.WeekStartMonday,
					Timezone:     tz,
				}

				stats, err := service.GetWeeklyStatistics(ctx, opts)
				require.NoError(t, err)
				require.NotNil(t, stats)

				assert.Equal(t, tz, stats.Timezone)
				assert.Equal(t, 1, stats.Summary.TotalCheckins)
			})
		}

		cleanupTestData(t, db, userID)
	})

	t.Run("YearBoundary", func(t *testing.T) {
		userID := uuid.New()
		setupTestUser(t, db, userID)

		// Check-ins around New Year
		dates := []time.Time{
			time.Date(2023, 12, 31, 12, 0, 0, 0, time.UTC),
			time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		}

		for _, date := range dates {
			_, err := db.ExecContext(ctx, `
				INSERT INTO checkins (id, user_id, checkin_time, duration_minutes, description, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
			`, uuid.New(), userID, date, 30, "New Year checkin")
			require.NoError(t, err)
		}

		opts := statistics.GetWeeklyOptions{
			UserID:       userID,
			Date:         time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			WeekStartDay: statistics.WeekStartMonday,
			Timezone:     "UTC",
		}

		stats, err := service.GetWeeklyStatistics(ctx, opts)
		require.NoError(t, err)
		require.NotNil(t, stats)

		// Should handle year transition correctly
		assert.NotNil(t, stats.Summary)
		assert.NotEmpty(t, stats.PreviousWeek)
		assert.NotEmpty(t, stats.NextWeek)

		cleanupTestData(t, db, userID)
	})

	t.Run("LeapYear", func(t *testing.T) {
		userID := uuid.New()
		setupTestUser(t, db, userID)

		// Check-in on leap day
		checkinDate := time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC)
		_, err := db.ExecContext(ctx, `
			INSERT INTO checkins (id, user_id, checkin_time, duration_minutes, description, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		`, uuid.New(), userID, checkinDate, 30, "Leap day checkin")
		require.NoError(t, err)

		opts := statistics.GetWeeklyOptions{
			UserID:       userID,
			Date:         time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC),
			WeekStartDay: statistics.WeekStartMonday,
			Timezone:     "UTC",
		}

		stats, err := service.GetWeeklyStatistics(ctx, opts)
		require.NoError(t, err)
		require.NotNil(t, stats)

		assert.Equal(t, 1, stats.Summary.TotalCheckins)

		cleanupTestData(t, db, userID)
	})

	t.Run("ExtremelyLongDuration", func(t *testing.T) {
		userID := uuid.New()
		setupTestUser(t, db, userID)

		// Check-in with duration longer than a day
		checkinDate := time.Date(2024, 1, 10, 12, 0, 0, 0, time.UTC)
		_, err := db.ExecContext(ctx, `
			INSERT INTO checkins (id, user_id, checkin_time, duration_minutes, description, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		`, uuid.New(), userID, checkinDate, 2000, "Long duration checkin") // 33+ hours
		require.NoError(t, err)

		opts := statistics.GetWeeklyOptions{
			UserID:       userID,
			Date:         time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
			WeekStartDay: statistics.WeekStartMonday,
			Timezone:     "UTC",
		}

		stats, err := service.GetWeeklyStatistics(ctx, opts)
		require.NoError(t, err)
		require.NotNil(t, stats)

		assert.Equal(t, 1, stats.Summary.TotalCheckins)
		assert.Equal(t, 2000, stats.Summary.TotalMinutes)

		// Completion percent should cap at 100%
		for _, day := range stats.DailyBreakdown {
			if day.Date == "2024-01-10" {
				assert.LessOrEqual(t, day.CompletionPercent, 100.0)
			}
		}

		cleanupTestData(t, db, userID)
	})

	t.Run("MultipleCheckinsAtSameTime", func(t *testing.T) {
		userID := uuid.New()
		setupTestUser(t, db, userID)

		// Multiple check-ins at exact same timestamp
		checkinDate := time.Date(2024, 1, 10, 12, 0, 0, 0, time.UTC)
		for i := 0; i < 5; i++ {
			_, err := db.ExecContext(ctx, `
				INSERT INTO checkins (id, user_id, checkin_time, duration_minutes, description, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
			`, uuid.New(), userID, checkinDate, 30, "Simultaneous checkin")
			require.NoError(t, err)
		}

		opts := statistics.GetWeeklyOptions{
			UserID:       userID,
			Date:         time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
			WeekStartDay: statistics.WeekStartMonday,
			Timezone:     "UTC",
		}

		stats, err := service.GetWeeklyStatistics(ctx, opts)
		require.NoError(t, err)
		require.NotNil(t, stats)

		assert.Equal(t, 5, stats.Summary.TotalCheckins)
		assert.Equal(t, 150, stats.Summary.TotalMinutes)

		cleanupTestData(t, db, userID)
	})

	t.Run("WeekWithOnlyWeekendActivity", func(t *testing.T) {
		userID := uuid.New()
		setupTestUser(t, db, userID)

		// Check-ins only on Saturday and Sunday
		dates := []time.Time{
			time.Date(2024, 1, 13, 10, 0, 0, 0, time.UTC), // Saturday
			time.Date(2024, 1, 14, 10, 0, 0, 0, time.UTC), // Sunday
		}

		for _, date := range dates {
			for i := 0; i < 5; i++ {
				_, err := db.ExecContext(ctx, `
					INSERT INTO checkins (id, user_id, checkin_time, duration_minutes, description, created_at, updated_at)
					VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
				`, uuid.New(), userID, date.Add(time.Duration(i)*time.Minute), 30, "Weekend checkin")
				require.NoError(t, err)
			}
		}

		opts := statistics.GetWeeklyOptions{
			UserID:       userID,
			Date:         time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
			WeekStartDay: statistics.WeekStartMonday,
			Timezone:     "UTC",
		}

		stats, err := service.GetWeeklyStatistics(ctx, opts)
		require.NoError(t, err)
		require.NotNil(t, stats)

		assert.Equal(t, 10, stats.Summary.TotalCheckins)
		assert.Equal(t, 2, stats.Summary.DaysWithCheckins)
		assert.InDelta(t, 28.57, stats.Summary.CompletionRate, 0.1) // 2/7 * 100

		cleanupTestData(t, db, userID)
	})

	t.Run("AllDaysEqualActivity", func(t *testing.T) {
		userID := uuid.New()
		setupTestUser(t, db, userID)

		// Same number of check-ins every day
		baseDate := time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC)
		for day := 0; day < 7; day++ {
			for i := 0; i < 5; i++ {
				checkinDate := baseDate.AddDate(0, 0, day).Add(time.Duration(i) * time.Hour)
				_, err := db.ExecContext(ctx, `
					INSERT INTO checkins (id, user_id, checkin_time, duration_minutes, description, created_at, updated_at)
					VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
				`, uuid.New(), userID, checkinDate, 30, "Equal checkin")
				require.NoError(t, err)
			}
		}

		opts := statistics.GetWeeklyOptions{
			UserID:       userID,
			Date:         time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
			WeekStartDay: statistics.WeekStartMonday,
			Timezone:     "UTC",
		}

		stats, err := service.GetWeeklyStatistics(ctx, opts)
		require.NoError(t, err)
		require.NotNil(t, stats)

		assert.Equal(t, 35, stats.Summary.TotalCheckins) // 5 * 7
		assert.Equal(t, 7, stats.Summary.DaysWithCheckins)
		assert.Equal(t, 100.0, stats.Summary.CompletionRate)

		// All days should have same count
		for _, day := range stats.DailyBreakdown {
			if day.HasCheckins {
				assert.Equal(t, 5, day.CheckinCount)
			}
		}

		cleanupTestData(t, db, userID)
	})

	t.Run("NegativeDurationHandling", func(t *testing.T) {
		userID := uuid.New()
		setupTestUser(t, db, userID)

		// Try to insert with negative duration (should be prevented by validation)
		checkinDate := time.Date(2024, 1, 10, 12, 0, 0, 0, time.UTC)
		_, err := db.ExecContext(ctx, `
			INSERT INTO checkins (id, user_id, checkin_time, duration_minutes, description, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		`, uuid.New(), userID, checkinDate, -30, "Negative duration")
		// This might fail due to database constraints - that's expected

		// Even if it succeeds, statistics should handle it
		opts := statistics.GetWeeklyOptions{
			UserID:       userID,
			Date:         time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
			WeekStartDay: statistics.WeekStartMonday,
			Timezone:     "UTC",
		}

		stats, err := service.GetWeeklyStatistics(ctx, opts)
		require.NoError(t, err)
		require.NotNil(t, stats)

		// Total minutes should not be negative
		assert.GreaterOrEqual(t, stats.Summary.TotalMinutes, 0)

		cleanupTestData(t, db, userID)
	})

	t.Run("FutureWeek", func(t *testing.T) {
		userID := uuid.New()
		setupTestUser(t, db, userID)

		// Request statistics for a future week
		futureDate := time.Now().AddDate(0, 0, 30)
		opts := statistics.GetWeeklyOptions{
			UserID:       userID,
			Date:         futureDate,
			WeekStartDay: statistics.WeekStartMonday,
			Timezone:     "UTC",
		}

		stats, err := service.GetWeeklyStatistics(ctx, opts)
		require.NoError(t, err)
		require.NotNil(t, stats)

		// Should return empty statistics
		assert.Equal(t, 0, stats.Summary.TotalCheckins)
		assert.Equal(t, 0, stats.Summary.DaysWithCheckins)

		cleanupTestData(t, db, userID)
	})

	t.Run("VeryOldWeek", func(t *testing.T) {
		userID := uuid.New()
		setupTestUser(t, db, userID)

		// Check-in from many years ago
		oldDate := time.Date(2000, 1, 10, 12, 0, 0, 0, time.UTC)
		_, err := db.ExecContext(ctx, `
			INSERT INTO checkins (id, user_id, checkin_time, duration_minutes, description, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		`, uuid.New(), userID, oldDate, 30, "Old checkin")
		require.NoError(t, err)

		opts := statistics.GetWeeklyOptions{
			UserID:       userID,
			Date:         oldDate,
			WeekStartDay: statistics.WeekStartMonday,
			Timezone:     "UTC",
		}

		stats, err := service.GetWeeklyStatistics(ctx, opts)
		require.NoError(t, err)
		require.NotNil(t, stats)

		assert.Equal(t, 2000, stats.Year)
		assert.Equal(t, 1, stats.Summary.TotalCheckins)

		cleanupTestData(t, db, userID)
	})
}

// TestStreakEdgeCases tests edge cases for streak calculation
func TestStreakEdgeCases(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db, log := setupTestDatabase(t)
	defer database.Close(db, log)

	repo := statistics.NewRepository(db)
	service := statistics.NewService(repo, log)
	ctx := context.Background()

	t.Run("PerfectStreak", func(t *testing.T) {
		userID := uuid.New()
		setupTestUser(t, db, userID)

		// Check-ins every day for 30 days up to today
		today := time.Now()
		for i := 0; i < 30; i++ {
			date := today.AddDate(0, 0, -i)
			_, err := db.ExecContext(ctx, `
				INSERT INTO checkins (id, user_id, checkin_time, duration_minutes, description, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
			`, uuid.New(), userID, date, 30, "Streak checkin")
			require.NoError(t, err)
		}

		opts := statistics.GetWeeklyOptions{
			UserID:       userID,
			Date:         today,
			WeekStartDay: statistics.WeekStartMonday,
			Timezone:     "UTC",
		}

		stats, err := service.GetWeeklyStatistics(ctx, opts)
		require.NoError(t, err)
		require.NotNil(t, stats)

		assert.NotNil(t, stats.Streak)
		assert.GreaterOrEqual(t, stats.Streak.CurrentStreak, 7) // At least 7 days
		assert.True(t, stats.Streak.IsStreakActive)

		cleanupTestData(t, db, userID)
	})

	t.Run("BrokenStreak", func(t *testing.T) {
		userID := uuid.New()
		setupTestUser(t, db, userID)

		// Check-ins with a gap
		dates := []time.Time{
			time.Now().AddDate(0, 0, -10),
			time.Now().AddDate(0, 0, -9),
			time.Now().AddDate(0, 0, -8),
			// Gap here
			time.Now().AddDate(0, 0, -5),
			time.Now().AddDate(0, 0, -4),
		}

		for _, date := range dates {
			_, err := db.ExecContext(ctx, `
				INSERT INTO checkins (id, user_id, checkin_time, duration_minutes, description, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
			`, uuid.New(), userID, date, 30, "Gap checkin")
			require.NoError(t, err)
		}

		opts := statistics.GetWeeklyOptions{
			UserID:       userID,
			Date:         time.Now(),
			WeekStartDay: statistics.WeekStartMonday,
			Timezone:     "UTC",
		}

		stats, err := service.GetWeeklyStatistics(ctx, opts)
		require.NoError(t, err)
		require.NotNil(t, stats)

		assert.NotNil(t, stats.Streak)
		// Streak should be broken
		assert.False(t, stats.Streak.IsStreakActive)

		cleanupTestData(t, db, userID)
	})
}
