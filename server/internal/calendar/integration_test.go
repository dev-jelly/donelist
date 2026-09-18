package calendar_test

import (
	"context"
	"testing"
	"time"

	"github.com/dev-jelly/donelist/internal/calendar"
	"github.com/dev-jelly/donelist/pkg/database"
	"github.com/dev-jelly/donelist/pkg/logger"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMonthlyCalendarIntegration tests the complete flow of monthly calendar generation
func TestMonthlyCalendarIntegration(t *testing.T) {
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
	repo := calendar.NewRepository(db)
	service := calendar.NewService(repo, log)

	// Create test user
	userID := uuid.New()
	ctx := context.Background()

	// Insert test user
	_, err = db.ExecContext(ctx, `
		INSERT INTO users (id, email, username, password_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, userID, "calendar-test@example.com", "calendaruser", "hashedpassword")
	require.NoError(t, err)

	// Create test category
	categoryID := uuid.New()
	_, err = db.ExecContext(ctx, `
		INSERT INTO categories (id, user_id, name, color, icon, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, categoryID, userID, "Work", "#FF5733", "work", )
	require.NoError(t, err)

	// Insert test check-ins across a month (November 2024)
	baseDate := time.Date(2024, 11, 1, 0, 0, 0, 0, time.UTC)
	checkinData := []struct {
		day     int
		count   int
		minutes int
	}{
		{1, 10, 480},   // Nov 1: 10 check-ins, 8 hours
		{2, 8, 360},    // Nov 2: 8 check-ins, 6 hours
		{4, 12, 540},   // Nov 4: 12 check-ins, 9 hours
		{5, 15, 600},   // Nov 5: 15 check-ins, 10 hours (most productive)
		{6, 7, 300},    // Nov 6: 7 check-ins, 5 hours
		{8, 9, 420},    // Nov 8: 9 check-ins, 7 hours
		{9, 11, 480},   // Nov 9: 11 check-ins, 8 hours
		{11, 10, 450},  // Nov 11: 10 check-ins, 7.5 hours
		{12, 8, 360},   // Nov 12: 8 check-ins, 6 hours
		{13, 14, 540},  // Nov 13: 14 check-ins, 9 hours
		{15, 10, 480},  // Nov 15: 10 check-ins, 8 hours
		{16, 9, 420},   // Nov 16: 9 check-ins, 7 hours
		{18, 11, 480},  // Nov 18: 11 check-ins, 8 hours
		{19, 8, 360},   // Nov 19: 8 check-ins, 6 hours
		{20, 13, 540},  // Nov 20: 13 check-ins, 9 hours
	}

	// Insert check-ins
	for _, data := range checkinData {
		checkinDate := baseDate.AddDate(0, 0, data.day-1).Add(time.Duration(9) * time.Hour)
		for i := 0; i < data.count; i++ {
			_, err = db.ExecContext(ctx, `
				INSERT INTO checkins (id, user_id, category_id, checkin_time, duration_minutes, description, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
			`, uuid.New(), userID, categoryID, checkinDate.Add(time.Duration(i*30)*time.Minute), data.minutes, "Test checkin", )
			require.NoError(t, err)
		}
	}

	// Test GetMonthlyCalendar
	t.Run("GetMonthlyCalendar_November2024", func(t *testing.T) {
		opts := calendar.GetOptions{
			UserID:   userID,
			Year:     2024,
			Month:    11,
			StartDay: calendar.StartDayMonday,
			Timezone: "UTC",
		}

		cal, err := service.GetMonthlyCalendar(ctx, opts)
		require.NoError(t, err)
		require.NotNil(t, cal)

		// Verify basic structure
		assert.Equal(t, 2024, cal.Year)
		assert.Equal(t, 11, cal.Month)
		assert.Equal(t, "November", cal.MonthName)
		assert.Equal(t, calendar.StartDayMonday, cal.StartDay)
		assert.NotEmpty(t, cal.Weeks)

		// Verify weeks structure (November 2024 should have 5 weeks with Monday start)
		assert.GreaterOrEqual(t, len(cal.Weeks), 4)
		assert.LessOrEqual(t, len(cal.Weeks), 6)

		// Verify each week has 7 days
		for i, week := range cal.Weeks {
			assert.Len(t, week.Days, 7, "Week %d should have 7 days", i)
		}

		// Verify summary
		assert.NotNil(t, cal.Summary)
		assert.Equal(t, 30, cal.Summary.TotalDaysInMonth)
		assert.Equal(t, 15, cal.Summary.DaysWithCheckins)
		assert.Greater(t, cal.Summary.TotalCheckins, 0)
		assert.Greater(t, cal.Summary.TotalMinutes, 0)
		assert.Greater(t, cal.Summary.AveragePerDay, 0.0)
		assert.Greater(t, cal.Summary.CompletionRate, 0.0)
		assert.Equal(t, "2024-11-05", cal.Summary.MostProductiveDay)
		assert.Equal(t, 15, cal.Summary.MostProductiveCount)

		// Verify navigation
		assert.Equal(t, "2024-10", cal.PreviousMonth)
		assert.Equal(t, "2024-12", cal.NextMonth)

		// Verify categories
		assert.NotEmpty(t, cal.Categories)
		assert.Equal(t, "Work", cal.Categories[0].CategoryName)
		assert.Equal(t, 100.0, cal.Categories[0].Percentage)

		// Verify specific days
		foundDay1 := false
		foundDay5 := false
		for _, week := range cal.Weeks {
			for _, day := range week.Days {
				if day.Date == "2024-11-01" {
					foundDay1 = true
					assert.True(t, day.IsCurrentMonth)
					assert.True(t, day.HasCheckins)
					assert.Equal(t, 10, day.CheckinCount)
					assert.Equal(t, 480, day.TotalMinutes)
					assert.Greater(t, day.CompletionPercent, 0.0)
					assert.Greater(t, day.ColorIntensity, 0)
				}
				if day.Date == "2024-11-05" {
					foundDay5 = true
					assert.True(t, day.IsCurrentMonth)
					assert.True(t, day.HasCheckins)
					assert.Equal(t, 15, day.CheckinCount)
					assert.Equal(t, 600, day.TotalMinutes)
				}
			}
		}
		assert.True(t, foundDay1, "Should find November 1st")
		assert.True(t, foundDay5, "Should find November 5th (most productive)")
	})

	// Test with Sunday start day
	t.Run("GetMonthlyCalendar_SundayStart", func(t *testing.T) {
		opts := calendar.GetOptions{
			UserID:   userID,
			Year:     2024,
			Month:    11,
			StartDay: calendar.StartDaySunday,
			Timezone: "UTC",
		}

		cal, err := service.GetMonthlyCalendar(ctx, opts)
		require.NoError(t, err)
		require.NotNil(t, cal)

		assert.Equal(t, calendar.StartDaySunday, cal.StartDay)
		assert.NotEmpty(t, cal.Weeks)

		// November 1, 2024 is a Friday, so with Sunday start, it should be in the first week
		firstWeek := cal.Weeks[0]
		assert.Len(t, firstWeek.Days, 7)
	})

	// Test with different timezone
	t.Run("GetMonthlyCalendar_DifferentTimezone", func(t *testing.T) {
		opts := calendar.GetOptions{
			UserID:   userID,
			Year:     2024,
			Month:    11,
			StartDay: calendar.StartDayMonday,
			Timezone: "America/New_York",
		}

		cal, err := service.GetMonthlyCalendar(ctx, opts)
		require.NoError(t, err)
		require.NotNil(t, cal)

		assert.Equal(t, 2024, cal.Year)
		assert.Equal(t, 11, cal.Month)
	})

	// Test empty month
	t.Run("GetMonthlyCalendar_EmptyMonth", func(t *testing.T) {
		emptyUserID := uuid.New()
		_, err = db.ExecContext(ctx, `
			INSERT INTO users (id, email, username, password_hash, created_at, updated_at)
			VALUES ($1, $2, $3, $4, NOW(), NOW())
			ON CONFLICT (id) DO NOTHING
		`, emptyUserID, "empty@example.com", "emptyuser", "hashedpassword")
		require.NoError(t, err)

		opts := calendar.GetOptions{
			UserID:   emptyUserID,
			Year:     2024,
			Month:    11,
			StartDay: calendar.StartDayMonday,
			Timezone: "UTC",
		}

		cal, err := service.GetMonthlyCalendar(ctx, opts)
		require.NoError(t, err)
		require.NotNil(t, cal)

		// Should still have proper structure
		assert.Equal(t, 2024, cal.Year)
		assert.Equal(t, 11, cal.Month)
		assert.NotEmpty(t, cal.Weeks)

		// Summary should reflect no activity
		assert.Equal(t, 0, cal.Summary.TotalCheckins)
		assert.Equal(t, 0, cal.Summary.DaysWithCheckins)
		assert.Equal(t, 0.0, cal.Summary.AveragePerDay)
	})

	// Cleanup
	_, _ = db.ExecContext(ctx, "DELETE FROM checkins WHERE user_id = $1", userID)
	_, _ = db.ExecContext(ctx, "DELETE FROM categories WHERE user_id = $1", userID)
	_, _ = db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", userID)
}

// TestHeatmapIntegration tests the heatmap data generation
func TestHeatmapIntegration(t *testing.T) {
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
	repo := calendar.NewRepository(db)
	service := calendar.NewService(repo, log)

	// Create test user
	userID := uuid.New()
	ctx := context.Background()

	// Insert test user
	_, err = db.ExecContext(ctx, `
		INSERT INTO users (id, email, username, password_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, userID, "heatmap-test@example.com", "heatmapuser", "hashedpassword")
	require.NoError(t, err)

	// Insert check-ins for a 3-month period
	startDate := time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2024, 11, 30, 23, 59, 59, 0, time.UTC)

	// Add varying levels of activity
	checkinDays := []int{1, 2, 3, 7, 8, 9, 15, 16, 20, 25, 30, 35, 40, 50, 60, 70, 80, 85, 88, 89, 90}
	for _, dayOffset := range checkinDays {
		checkinDate := startDate.AddDate(0, 0, dayOffset).Add(time.Duration(10) * time.Hour)
		checkinCount := 5 + (dayOffset % 10)
		for i := 0; i < checkinCount; i++ {
			_, err = db.ExecContext(ctx, `
				INSERT INTO checkins (id, user_id, checkin_time, duration_minutes, description, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
			`, uuid.New(), userID, checkinDate.Add(time.Duration(i*15)*time.Minute), 30, "Heatmap test checkin", )
			require.NoError(t, err)
		}
	}

	// Test GetHeatmap
	t.Run("GetHeatmap_ThreeMonths", func(t *testing.T) {
		opts := calendar.HeatmapOptions{
			UserID:    userID,
			StartDate: startDate,
			EndDate:   endDate,
			Timezone:  "UTC",
		}

		heatmap, err := service.GetHeatmap(ctx, opts)
		require.NoError(t, err)
		require.NotNil(t, heatmap)

		// Verify basic structure
		assert.Equal(t, "2024-09-01", heatmap.StartDate)
		assert.Equal(t, "2024-11-30", heatmap.EndDate)
		assert.NotEmpty(t, heatmap.Days)

		// Calculate expected days: Sept (30) + Oct (31) + Nov (30) = 91 days
		expectedDays := 91
		assert.Equal(t, expectedDays, heatmap.TotalDays)
		assert.Len(t, heatmap.Days, expectedDays)

		// Verify active days
		assert.Equal(t, len(checkinDays), heatmap.ActiveDays)
		assert.Greater(t, heatmap.TotalCheckins, 0)

		// Verify specific days have correct data
		activeDayFound := false
		inactiveDayFound := false
		for _, day := range heatmap.Days {
			if day.Date == "2024-09-02" { // Day with activity
				activeDayFound = true
				assert.Greater(t, day.CheckinCount, 0)
				assert.Greater(t, day.TotalMinutes, 0)
				assert.Greater(t, day.CompletionPercent, 0.0)
				assert.Greater(t, day.ColorIntensity, 0)
			}
			if day.Date == "2024-09-05" { // Day without activity
				inactiveDayFound = true
				assert.Equal(t, 0, day.CheckinCount)
				assert.Equal(t, 0, day.TotalMinutes)
				assert.Equal(t, 0.0, day.CompletionPercent)
				assert.Equal(t, 0, day.ColorIntensity)
			}
		}
		assert.True(t, activeDayFound, "Should find an active day")
		assert.True(t, inactiveDayFound, "Should find an inactive day")
	})

	// Test heatmap with single day
	t.Run("GetHeatmap_SingleDay", func(t *testing.T) {
		singleDay := time.Date(2024, 9, 2, 0, 0, 0, 0, time.UTC)
		opts := calendar.HeatmapOptions{
			UserID:    userID,
			StartDate: singleDay,
			EndDate:   singleDay,
			Timezone:  "UTC",
		}

		heatmap, err := service.GetHeatmap(ctx, opts)
		require.NoError(t, err)
		require.NotNil(t, heatmap)

		assert.Equal(t, 1, heatmap.TotalDays)
		assert.Len(t, heatmap.Days, 1)
		assert.Equal(t, "2024-09-02", heatmap.Days[0].Date)
	})

	// Test heatmap with different timezone
	t.Run("GetHeatmap_DifferentTimezone", func(t *testing.T) {
		opts := calendar.HeatmapOptions{
			UserID:    userID,
			StartDate: startDate,
			EndDate:   time.Date(2024, 9, 30, 0, 0, 0, 0, time.UTC),
			Timezone:  "Asia/Seoul",
		}

		heatmap, err := service.GetHeatmap(ctx, opts)
		require.NoError(t, err)
		require.NotNil(t, heatmap)

		assert.NotEmpty(t, heatmap.Days)
		assert.Equal(t, 30, heatmap.TotalDays)
	})

	// Test heatmap with no data
	t.Run("GetHeatmap_NoData", func(t *testing.T) {
		emptyUserID := uuid.New()
		_, err = db.ExecContext(ctx, `
			INSERT INTO users (id, email, username, password_hash, created_at, updated_at)
			VALUES ($1, $2, $3, $4, NOW(), NOW())
			ON CONFLICT (id) DO NOTHING
		`, emptyUserID, "heatmap-empty@example.com", "heatmapempty", "hashedpassword")
		require.NoError(t, err)

		opts := calendar.HeatmapOptions{
			UserID:    emptyUserID,
			StartDate: startDate,
			EndDate:   endDate,
			Timezone:  "UTC",
		}

		heatmap, err := service.GetHeatmap(ctx, opts)
		require.NoError(t, err)
		require.NotNil(t, heatmap)

		assert.Equal(t, 0, heatmap.ActiveDays)
		assert.Equal(t, 0, heatmap.TotalCheckins)
		assert.NotEmpty(t, heatmap.Days) // Should still have all days, just with no activity
	})

	// Cleanup
	_, _ = db.ExecContext(ctx, "DELETE FROM checkins WHERE user_id = $1", userID)
	_, _ = db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", userID)
}
