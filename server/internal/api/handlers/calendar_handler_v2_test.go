package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dev-jelly/donelist/internal/api/middleware"
	"github.com/dev-jelly/donelist/internal/calendar"
	"github.com/dev-jelly/donelist/internal/testutil"
	"github.com/dev-jelly/donelist/pkg/cache"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// MockCalendarService is a mock implementation of calendar.Service
type MockCalendarService struct {
	mock.Mock
}

func (m *MockCalendarService) GetMonthlyCalendar(ctx context.Context, opts calendar.GetOptions) (*calendar.MonthlyCalendar, error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*calendar.MonthlyCalendar), args.Error(1)
}

func (m *MockCalendarService) GetHeatmap(ctx context.Context, opts calendar.HeatmapOptions) (*calendar.HeatmapData, error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*calendar.HeatmapData), args.Error(1)
}

func TestGetMonthlyCalendarV2(t *testing.T) {
	// Setup
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()

	// Setup Redis client for cache
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   1, // Use test database
	})
	defer redisClient.Close()

	// Clear test database
	redisClient.FlushDB(context.Background())

	cacheConfig := cache.Config{
		Prefix:     "test:",
		DefaultTTL: 15 * time.Minute,
	}
	testCache := cache.NewCache(redisClient, cacheConfig, logger)

	// Create test user ID
	userID := uuid.New()

	// Sample calendar data
	sampleCalendar := &calendar.MonthlyCalendar{
		Year:      2024,
		Month:     11,
		MonthName: "November",
		StartDay:  calendar.StartDayMonday,
		Weeks: []*calendar.CalendarWeek{
			{
				Days: []*calendar.CalendarDay{
					{
						Date:              "2024-11-01",
						CheckinCount:      5,
						TotalMinutes:      120,
						CompletionPercent: 8.33,
						ColorIntensity:    1,
						IsCurrentMonth:    true,
						HasCheckins:       true,
					},
				},
			},
		},
		Summary: &calendar.MonthlySummary{
			TotalCheckins:    50,
			TotalMinutes:     1200,
			DaysWithCheckins: 20,
			TotalDaysInMonth: 30,
			AveragePerDay:    1.67,
			CompletionRate:   66.67,
		},
		GeneratedAt: time.Now(),
	}

	t.Run("First request returns 200 with data", func(t *testing.T) {
		mockService := new(MockCalendarService)
		handler := NewCalendarHandlerV2(mockService, testCache, logger)

		// Setup mock expectation
		mockService.On("GetMonthlyCalendar", mock.Anything, mock.MatchedBy(func(opts calendar.GetOptions) bool {
			return opts.Year == 2024 && opts.Month == 11
		})).Return(sampleCalendar, nil)

		// Create request
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("user_id", userID)
			c.Next()
		})
		router.GET("/calendar/month", handler.GetMonthlyCalendarV2)

		req := httptest.NewRequest("GET", "/calendar/month?year=2024&month=11", nil)
		w := httptest.NewRecorder()

		// Execute request
		router.ServeHTTP(w, req)

		// Assertions
		assert.Equal(t, http.StatusOK, w.Code)

		// Check headers
		assert.NotEmpty(t, w.Header().Get("ETag"))
		assert.Equal(t, "private, max-age=900", w.Header().Get("Cache-Control"))
		assert.Equal(t, "1.0.0", w.Header().Get("X-Schema-Version"))

		// Check response body
		var response MonthlyCalendarResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "1.0.0", response.SchemaVersion)
		assert.NotNil(t, response.Data)
		assert.Equal(t, 2024, response.Data.Year)
		assert.Equal(t, 11, response.Data.Month)

		mockService.AssertExpectations(t)
	})

	t.Run("Second request with same parameters returns cached data", func(t *testing.T) {
		mockService := new(MockCalendarService)
		handler := NewCalendarHandlerV2(mockService, testCache, logger)

		// Mock should NOT be called since data is cached
		// No expectations set

		// Create request
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("user_id", userID)
			c.Next()
		})
		router.GET("/calendar/month", handler.GetMonthlyCalendarV2)

		req := httptest.NewRequest("GET", "/calendar/month?year=2024&month=11", nil)
		w := httptest.NewRecorder()

		// Execute request
		router.ServeHTTP(w, req)

		// Assertions
		assert.Equal(t, http.StatusOK, w.Code)

		// Should have same headers
		assert.NotEmpty(t, w.Header().Get("ETag"))
		assert.Equal(t, "private, max-age=900", w.Header().Get("Cache-Control"))

		// Verify mock was not called (data came from cache)
		mockService.AssertNotCalled(t, "GetMonthlyCalendar")
	})

	t.Run("Request with ETag returns 304 Not Modified", func(t *testing.T) {
		mockService := new(MockCalendarService)
		handler := NewCalendarHandlerV2(mockService, testCache, logger)

		// First, make a request to get the ETag
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("user_id", userID)
			c.Next()
		})
		router.GET("/calendar/month", handler.GetMonthlyCalendarV2)

		req1 := httptest.NewRequest("GET", "/calendar/month?year=2024&month=11", nil)
		w1 := httptest.NewRecorder()
		router.ServeHTTP(w1, req1)
		etag := w1.Header().Get("ETag")

		// Now make request with If-None-Match header
		req2 := httptest.NewRequest("GET", "/calendar/month?year=2024&month=11", nil)
		req2.Header.Set("If-None-Match", etag)
		w2 := httptest.NewRecorder()

		router.ServeHTTP(w2, req2)

		// Assertions
		assert.Equal(t, http.StatusNotModified, w2.Code)
		assert.Equal(t, etag, w2.Header().Get("ETag"))
		assert.Empty(t, w2.Body.String()) // No body for 304

		mockService.AssertNotCalled(t, "GetMonthlyCalendar")
	})

	t.Run("Invalid year returns 400", func(t *testing.T) {
		mockService := new(MockCalendarService)
		handler := NewCalendarHandlerV2(mockService, testCache, logger)

		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("user_id", userID)
			c.Next()
		})
		router.GET("/calendar/month", handler.GetMonthlyCalendarV2)

		req := httptest.NewRequest("GET", "/calendar/month?year=3000&month=11", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var response map[string]string
		json.Unmarshal(w.Body.Bytes(), &response)
		assert.Contains(t, response["error"], "invalid year")
	})

	t.Run("Invalid month returns 400", func(t *testing.T) {
		mockService := new(MockCalendarService)
		handler := NewCalendarHandlerV2(mockService, testCache, logger)

		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("user_id", userID)
			c.Next()
		})
		router.GET("/calendar/month", handler.GetMonthlyCalendarV2)

		req := httptest.NewRequest("GET", "/calendar/month?year=2024&month=13", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var response map[string]string
		json.Unmarshal(w.Body.Bytes(), &response)
		assert.Contains(t, response["error"], "invalid month")
	})

	t.Run("Missing year parameter returns 400", func(t *testing.T) {
		mockService := new(MockCalendarService)
		handler := NewCalendarHandlerV2(mockService, testCache, logger)

		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("user_id", userID)
			c.Next()
		})
		router.GET("/calendar/month", handler.GetMonthlyCalendarV2)

		req := httptest.NewRequest("GET", "/calendar/month?month=11", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var response map[string]string
		json.Unmarshal(w.Body.Bytes(), &response)
		assert.Contains(t, response["error"], "year parameter is required")
	})

	t.Run("Unauthorized request returns 401", func(t *testing.T) {
		mockService := new(MockCalendarService)
		handler := NewCalendarHandlerV2(mockService, testCache, logger)

		router := gin.New()
		// No user_id set
		router.GET("/calendar/month", handler.GetMonthlyCalendarV2)

		req := httptest.NewRequest("GET", "/calendar/month?year=2024&month=11", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Service error returns 500", func(t *testing.T) {
		// Clear cache first
		redisClient.FlushDB(context.Background())

		mockService := new(MockCalendarService)
		handler := NewCalendarHandlerV2(mockService, testCache, logger)

		// Setup mock to return error
		mockService.On("GetMonthlyCalendar", mock.Anything, mock.Anything).
			Return(nil, fmt.Errorf("database error"))

		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("user_id", userID)
			c.Next()
		})
		router.GET("/calendar/month", handler.GetMonthlyCalendarV2)

		req := httptest.NewRequest("GET", "/calendar/month?year=2024&month=11", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		var response map[string]string
		json.Unmarshal(w.Body.Bytes(), &response)
		assert.Contains(t, response["error"], "failed to get monthly calendar")

		mockService.AssertExpectations(t)
	})

	t.Run("Different startOfWeek parameter uses different cache key", func(t *testing.T) {
		// Clear cache
		redisClient.FlushDB(context.Background())

		mockService := new(MockCalendarService)
		handler := NewCalendarHandlerV2(mockService, testCache, logger)

		// Setup mock to be called twice (once for each startOfWeek value)
		mockService.On("GetMonthlyCalendar", mock.Anything, mock.MatchedBy(func(opts calendar.GetOptions) bool {
			return opts.StartDay == calendar.StartDaySunday
		})).Return(sampleCalendar, nil).Once()

		mockService.On("GetMonthlyCalendar", mock.Anything, mock.MatchedBy(func(opts calendar.GetOptions) bool {
			return opts.StartDay == calendar.StartDayMonday
		})).Return(sampleCalendar, nil).Once()

		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("user_id", userID)
			c.Next()
		})
		router.GET("/calendar/month", handler.GetMonthlyCalendarV2)

		// Request with startOfWeek=sun
		req1 := httptest.NewRequest("GET", "/calendar/month?year=2024&month=11&startOfWeek=sun", nil)
		w1 := httptest.NewRecorder()
		router.ServeHTTP(w1, req1)
		assert.Equal(t, http.StatusOK, w1.Code)

		// Request with startOfWeek=mon (should not use cache from previous request)
		req2 := httptest.NewRequest("GET", "/calendar/month?year=2024&month=11&startOfWeek=mon", nil)
		w2 := httptest.NewRecorder()
		router.ServeHTTP(w2, req2)
		assert.Equal(t, http.StatusOK, w2.Code)

		// Both requests should have called the service
		mockService.AssertNumberOfCalls(t, "GetMonthlyCalendar", 2)
	})
}

func TestCacheInvalidation(t *testing.T) {
	// Setup
	logger := zap.NewNop()

	// Setup Redis client for cache
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   1, // Use test database
	})
	defer redisClient.Close()

	// Clear test database
	redisClient.FlushDB(context.Background())

	cacheConfig := cache.Config{
		Prefix:     "test:",
		DefaultTTL: 15 * time.Minute,
	}
	testCache := cache.NewCache(redisClient, cacheConfig, logger)

	userID := uuid.New()

	t.Run("InvalidateUserCalendarCache removes all user calendar entries", func(t *testing.T) {
		handler := NewCalendarHandlerV2(nil, testCache, logger)

		// Add some cache entries
		ctx := context.Background()
		for i := 1; i <= 3; i++ {
			key := fmt.Sprintf("calendar:v2:%s:2024-%02d:monday:1.0.0", userID.String(), i)
			testCache.Set(ctx, key, "test-data", time.Hour)
		}

		// Invalidate
		err := handler.InvalidateUserCalendarCache(userID)
		assert.NoError(t, err)

		// Verify entries are gone
		for i := 1; i <= 3; i++ {
			key := fmt.Sprintf("calendar:v2:%s:2024-%02d:monday:1.0.0", userID.String(), i)
			var result string
			err := testCache.Get(ctx, key, &result)
			assert.Error(t, err)
		}
	})

	t.Run("InvalidateMonthCalendarCache removes specific month entries", func(t *testing.T) {
		handler := NewCalendarHandlerV2(nil, testCache, logger)

		// Add cache entries for multiple months
		ctx := context.Background()
		key1 := fmt.Sprintf("calendar:v2:%s:2024-11:monday:1.0.0", userID.String())
		key2 := fmt.Sprintf("calendar:v2:%s:2024-11:sunday:1.0.0", userID.String())
		key3 := fmt.Sprintf("calendar:v2:%s:2024-12:monday:1.0.0", userID.String())

		testCache.Set(ctx, key1, "test1", time.Hour)
		testCache.Set(ctx, key2, "test2", time.Hour)
		testCache.Set(ctx, key3, "test3", time.Hour)

		// Invalidate only November
		err := handler.InvalidateMonthCalendarCache(userID, 2024, 11)
		assert.NoError(t, err)

		// November entries should be gone
		var result string
		err = testCache.Get(ctx, key1, &result)
		assert.Error(t, err)
		err = testCache.Get(ctx, key2, &result)
		assert.Error(t, err)

		// December entry should still exist
		err = testCache.Get(ctx, key3, &result)
		assert.NoError(t, err)
	})
}