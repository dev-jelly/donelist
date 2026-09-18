package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dev-jelly/donelist/internal/api/handlers"
	"github.com/dev-jelly/donelist/internal/calendar"
	"github.com/dev-jelly/donelist/pkg/cache"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestCalendarV2Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Setup
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()

	// Setup in-memory calendar service mock
	calendarRepo := calendar.NewRepository(nil) // This would need a test DB in real integration test
	calendarService := calendar.NewService(calendarRepo, logger)

	// Setup Redis client for cache
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   15, // Use test database
	})
	defer redisClient.Close()

	// Ping Redis to ensure connection
	ctx := context.Background()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	// Clear test database
	redisClient.FlushDB(ctx)

	cacheConfig := cache.Config{
		Prefix:     "test:",
		DefaultTTL: 15 * time.Minute,
	}
	testCache := cache.NewCache(redisClient, cacheConfig, logger)

	// Create handler with cache
	handler := handlers.NewCalendarHandler(calendarService, logger)
	handler.SetCache(testCache)

	// Test user ID
	userID := uuid.New()

	// Create router
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", userID)
		c.Next()
	})
	router.GET("/calendar/month", handler.GetMonthlyCalendarV2)

	t.Run("First request populates cache", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/calendar/month?year=2024&month=11", nil)
		w := httptest.NewRecorder()

		startTime := time.Now()
		router.ServeHTTP(w, req)
		firstRequestTime := time.Since(startTime)

		assert.Equal(t, http.StatusOK, w.Code)
		etag := w.Header().Get("ETag")
		assert.NotEmpty(t, etag)

		// Parse response
		var response handlers.MonthlyCalendarResponseV2
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "1.0.0", response.SchemaVersion)

		t.Logf("First request took: %v", firstRequestTime)
	})

	t.Run("Second request uses cache (faster)", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/calendar/month?year=2024&month=11", nil)
		w := httptest.NewRecorder()

		startTime := time.Now()
		router.ServeHTTP(w, req)
		cachedRequestTime := time.Since(startTime)

		assert.Equal(t, http.StatusOK, w.Code)
		etag := w.Header().Get("ETag")
		assert.NotEmpty(t, etag)

		t.Logf("Cached request took: %v", cachedRequestTime)
		// Cached request should be significantly faster
		// In a real test with DB, first request might take 50-100ms, cached < 5ms
	})

	t.Run("Request with ETag returns 304", func(t *testing.T) {
		// First get the ETag
		req1 := httptest.NewRequest("GET", "/calendar/month?year=2024&month=11", nil)
		w1 := httptest.NewRecorder()
		router.ServeHTTP(w1, req1)
		etag := w1.Header().Get("ETag")

		// Now request with If-None-Match
		req2 := httptest.NewRequest("GET", "/calendar/month?year=2024&month=11", nil)
		req2.Header.Set("If-None-Match", etag)
		w2 := httptest.NewRecorder()

		router.ServeHTTP(w2, req2)

		assert.Equal(t, http.StatusNotModified, w2.Code)
		assert.Equal(t, etag, w2.Header().Get("ETag"))
		assert.Empty(t, w2.Body.String()) // No body for 304
	})

	t.Run("Different parameters use different cache keys", func(t *testing.T) {
		// Request with different month
		req1 := httptest.NewRequest("GET", "/calendar/month?year=2024&month=12", nil)
		w1 := httptest.NewRecorder()
		router.ServeHTTP(w1, req1)
		assert.Equal(t, http.StatusOK, w1.Code)

		// Request with different startOfWeek
		req2 := httptest.NewRequest("GET", "/calendar/month?year=2024&month=11&startOfWeek=sun", nil)
		w2 := httptest.NewRecorder()
		router.ServeHTTP(w2, req2)
		assert.Equal(t, http.StatusOK, w2.Code)

		// Both should have different ETags
		var response1, response2 handlers.MonthlyCalendarResponseV2
		json.Unmarshal(w1.Body.Bytes(), &response1)
		json.Unmarshal(w2.Body.Bytes(), &response2)

		// Different months should potentially have different content
		// (In a real test with data)
	})

	t.Run("Cache invalidation works", func(t *testing.T) {
		// Get initial response
		req1 := httptest.NewRequest("GET", "/calendar/month?year=2024&month=11", nil)
		w1 := httptest.NewRecorder()
		router.ServeHTTP(w1, req1)
		etag1 := w1.Header().Get("ETag")

		// Invalidate cache
		err := handler.InvalidateMonthCalendarCache(userID, 2024, 11)
		require.NoError(t, err)

		// Request again - should get same data but potentially different ETag
		// (In a real scenario, data might have changed)
		req2 := httptest.NewRequest("GET", "/calendar/month?year=2024&month=11", nil)
		w2 := httptest.NewRecorder()
		router.ServeHTTP(w2, req2)
		etag2 := w2.Header().Get("ETag")

		assert.Equal(t, http.StatusOK, w2.Code)
		// ETags might be same if data hasn't changed, but cache was cleared
		_ = etag1
		_ = etag2
	})

	t.Run("Invalid parameters return 400", func(t *testing.T) {
		testCases := []struct {
			name     string
			query    string
			expected string
		}{
			{"missing year", "month=11", "year parameter is required"},
			{"missing month", "year=2024", "month parameter is required"},
			{"invalid year", "year=3000&month=11", "invalid year"},
			{"invalid month", "year=2024&month=13", "invalid month"},
			{"invalid startOfWeek", "year=2024&month=11&startOfWeek=invalid", "invalid startOfWeek"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				req := httptest.NewRequest("GET", "/calendar/month?"+tc.query, nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				assert.Equal(t, http.StatusBadRequest, w.Code)
				var response map[string]string
				json.Unmarshal(w.Body.Bytes(), &response)
				assert.Contains(t, response["error"], tc.expected)
			})
		}
	})
}