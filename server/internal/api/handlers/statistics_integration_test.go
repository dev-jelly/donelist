package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/dev-jelly/donelist/internal/api/handlers"
	"github.com/dev-jelly/donelist/internal/api/middleware"
	"github.com/dev-jelly/donelist/internal/statistics"
	"github.com/dev-jelly/donelist/pkg/database"
	"github.com/dev-jelly/donelist/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStatisticsAPIIntegration tests the complete API endpoint flow
func TestStatisticsAPIIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test environment
	db, log := setupTestDatabase(t)
	defer database.Close(db, log)

	redisClient, redisCleanup := setupTestRedis(t)
	defer redisCleanup()

	// Create services
	repo := statistics.NewRepository(db)
	cache := statistics.NewCacheService(redisClient, log)
	service := statistics.NewService(repo, log)
	service.SetCache(cache)

	// Create handler
	handler := handlers.NewStatisticsHandler(service, log)

	// Setup router
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Create test user and context
	userID := uuid.New()
	setupTestUser(t, db, userID)
	baseDate := time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC) // Monday
	setupTestCheckins(t, db, userID, baseDate)

	// Add authentication middleware mock
	router.Use(func(c *gin.Context) {
		c.Set(middleware.UserIDKey, userID)
		c.Next()
	})

	router.GET("/statistics/weekly", handler.GetWeeklyStatistics)

	t.Run("BasicWeeklyStatistics", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/statistics/weekly?date=2024-01-10&week_start=monday&timezone=UTC", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var stats statistics.WeeklyStatistics
		err := json.Unmarshal(w.Body.Bytes(), &stats)
		require.NoError(t, err)

		// Verify basic structure
		assert.Equal(t, 2024, stats.Year)
		assert.Equal(t, "2024-01-08", stats.StartDate)
		assert.Equal(t, "2024-01-14", stats.EndDate)
		assert.Equal(t, statistics.WeekStartMonday, stats.WeekStartDay)
		assert.Equal(t, "UTC", stats.Timezone)

		// Verify summary exists
		assert.NotNil(t, stats.Summary)
		assert.Greater(t, stats.Summary.TotalCheckins, 0)
		assert.Greater(t, stats.Summary.TotalMinutes, 0)

		// Verify all sections
		assert.Len(t, stats.DailyBreakdown, 7)
		assert.NotNil(t, stats.DayOfWeekAnalysis)
		assert.NotNil(t, stats.TimeDistribution)
		assert.NotNil(t, stats.CategoryBreakdown)
		assert.NotNil(t, stats.Comparison)
		assert.NotNil(t, stats.Streak)
	})

	t.Run("SundayStartWeek", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/statistics/weekly?date=2024-01-10&week_start=sunday&timezone=UTC", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var stats statistics.WeeklyStatistics
		err := json.Unmarshal(w.Body.Bytes(), &stats)
		require.NoError(t, err)

		assert.Equal(t, statistics.WeekStartSunday, stats.WeekStartDay)
		assert.Len(t, stats.DailyBreakdown, 7)
		assert.Equal(t, "Sunday", stats.DailyBreakdown[0].DayOfWeek)
	})

	t.Run("DifferentTimezones", func(t *testing.T) {
		timezones := []string{
			"UTC",
			"America/New_York",
			"America/Los_Angeles",
			"Asia/Seoul",
			"Europe/London",
		}

		for _, tz := range timezones {
			t.Run(tz, func(t *testing.T) {
				w := httptest.NewRecorder()
				req := httptest.NewRequest("GET", fmt.Sprintf("/statistics/weekly?date=2024-01-10&week_start=monday&timezone=%s", tz), nil)
				router.ServeHTTP(w, req)

				assert.Equal(t, http.StatusOK, w.Code)

				var stats statistics.WeeklyStatistics
				err := json.Unmarshal(w.Body.Bytes(), &stats)
				require.NoError(t, err)

				assert.Equal(t, tz, stats.Timezone)
				assert.Len(t, stats.DailyBreakdown, 7)
			})
		}
	})

	t.Run("CacheEfficiency", func(t *testing.T) {
		// Clear cache
		ctx := context.Background()
		err := cache.InvalidateUserStats(ctx, userID.String())
		require.NoError(t, err)

		// First request - cache miss
		start1 := time.Now()
		w1 := httptest.NewRecorder()
		req1 := httptest.NewRequest("GET", "/statistics/weekly?date=2024-01-10&week_start=monday&timezone=UTC", nil)
		router.ServeHTTP(w1, req1)
		duration1 := time.Since(start1)

		assert.Equal(t, http.StatusOK, w1.Code)

		// Second request - cache hit
		start2 := time.Now()
		w2 := httptest.NewRecorder()
		req2 := httptest.NewRequest("GET", "/statistics/weekly?date=2024-01-10&week_start=monday&timezone=UTC", nil)
		router.ServeHTTP(w2, req2)
		duration2 := time.Since(start2)

		assert.Equal(t, http.StatusOK, w2.Code)

		// Cache hit should be significantly faster
		assert.Less(t, duration2, duration1/2, "Cache hit should be at least 2x faster")

		// Response should be identical
		assert.JSONEq(t, w1.Body.String(), w2.Body.String())

		t.Logf("First request (cache miss): %v", duration1)
		t.Logf("Second request (cache hit): %v", duration2)
		t.Logf("Speedup: %.2fx", float64(duration1)/float64(duration2))
	})

	t.Run("InvalidParameters", func(t *testing.T) {
		testCases := []struct {
			name       string
			query      string
			wantStatus int
		}{
			{
				name:       "InvalidDateFormat",
				query:      "/statistics/weekly?date=2024/01/10",
				wantStatus: http.StatusBadRequest,
			},
			{
				name:       "InvalidWeekStart",
				query:      "/statistics/weekly?date=2024-01-10&week_start=tuesday",
				wantStatus: http.StatusBadRequest,
			},
			{
				name:       "InvalidTimezone",
				query:      "/statistics/weekly?date=2024-01-10&timezone=Invalid/Timezone",
				wantStatus: http.StatusBadRequest,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				w := httptest.NewRecorder()
				req := httptest.NewRequest("GET", tc.query, nil)
				router.ServeHTTP(w, req)

				assert.Equal(t, tc.wantStatus, w.Code)
			})
		}
	})

	t.Run("DefaultParameters", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/statistics/weekly", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var stats statistics.WeeklyStatistics
		err := json.Unmarshal(w.Body.Bytes(), &stats)
		require.NoError(t, err)

		// Should use current date, Monday start, and UTC timezone
		assert.Equal(t, statistics.WeekStartMonday, stats.WeekStartDay)
		assert.Equal(t, "UTC", stats.Timezone)
	})

	// Cleanup
	cleanupTestData(t, db, userID)
}

// Helper functions

func setupTestDatabase(t *testing.T) (*database.DB, *logger.Logger) {
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

func setupTestRedis(t *testing.T) (*redis.Client, func()) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	cleanup := func() {
		client.Close()
		mr.Close()
	}

	return client, cleanup
}

func setupTestUser(t *testing.T, db *database.DB, userID uuid.UUID) {
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `
		INSERT INTO users (id, email, username, password_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, userID, "test@example.com", "testuser", "hashedpassword")
	require.NoError(t, err)
}

func setupTestCheckins(t *testing.T, db *database.DB, userID uuid.UUID, baseDate time.Time) {
	ctx := context.Background()

	checkinData := []struct {
		dayOffset int
		hour      int
		count     int
		minutes   int
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

	for _, data := range checkinData {
		checkinDate := baseDate.AddDate(0, 0, data.dayOffset).Add(time.Duration(data.hour) * time.Hour)
		for i := 0; i < data.count; i++ {
			_, err := db.ExecContext(ctx, `
				INSERT INTO checkins (id, user_id, checkin_time, duration_minutes, description, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
			`, uuid.New(), userID, checkinDate.Add(time.Duration(i)*time.Minute), data.minutes, "Test checkin")
			require.NoError(t, err)
		}
	}
}

func cleanupTestData(t *testing.T, db *database.DB, userID uuid.UUID) {
	ctx := context.Background()
	_, _ = db.ExecContext(ctx, "DELETE FROM checkins WHERE user_id = $1", userID)
	_, _ = db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", userID)
}
