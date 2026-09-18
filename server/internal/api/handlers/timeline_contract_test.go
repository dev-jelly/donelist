// +build contract

package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dev-jelly/donelist/internal/api/middleware"
	"github.com/dev-jelly/donelist/internal/auth"
	"github.com/dev-jelly/donelist/internal/category"
	"github.com/dev-jelly/donelist/internal/checkin"
	"github.com/dev-jelly/donelist/internal/testutil"
	"github.com/dev-jelly/donelist/internal/timeline"
	"github.com/dev-jelly/donelist/internal/user"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestTimelineAPIContract validates that the Timeline API adheres to the OpenAPI specification
func TestTimelineAPIContract(t *testing.T) {
	// Setup test database
	testDB := testutil.SetupTestDB(t)
	defer testDB.TearDown(t)

	// Initialize repositories
	userRepo := user.NewRepository(testDB.DB)
	checkinRepo := checkin.NewRepository(testDB.DB)
	categoryRepo := category.NewRepository(testDB.DB)

	// Setup logger
	logger := zap.NewNop()

	// Setup timeline service (without cache for simplicity)
	timelineService := timeline.NewService(checkinRepo, categoryRepo, nil, logger)

	// Setup handler
	handler := NewTimelineHandler(timelineService, logger)

	// Setup router with auth
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Create test user
	testUser := &user.User{
		ID:          uuid.New(),
		Email:       "timeline@example.com",
		Username:    "timelineuser",
		DisplayName: "Timeline User",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err := userRepo.Create(testUser, "hashedpassword")
	require.NoError(t, err)

	// Mock auth middleware for testing
	authMiddleware := func(c *gin.Context) {
		c.Set("user_id", testUser.ID)
		c.Next()
	}

	// Setup routes
	v1 := router.Group("/api/v1")
	timeline := v1.Group("/timeline")
	timeline.Use(authMiddleware)
	{
		timeline.GET("/daily", handler.GetDaily)
		timeline.GET("/daily/enhanced", handler.GetDailyEnhanced)
		timeline.GET("/weekly", handler.GetWeekly)
		timeline.GET("/monthly", handler.GetMonthly)
	}

	// Create test data
	cat := &category.Category{
		ID:     uuid.New(),
		UserID: testUser.ID,
		Name:   "Work",
		Color:  strPtr("#3498db"),
		Icon:   strPtr("briefcase"),
	}
	err = categoryRepo.Create(cat)
	require.NoError(t, err)

	// Create check-ins for testing
	baseTime := time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		checkinTime := baseTime.Add(time.Duration(i*2) * time.Hour)
		c := &checkin.Checkin{
			ID:              uuid.New(),
			UserID:          testUser.ID,
			Title:           "Task " + string(rune('A'+i)),
			Description:     strPtr("Description for task " + string(rune('A'+i))),
			CategoryID:      &cat.ID,
			CheckinTime:     checkinTime,
			DurationMinutes: 30,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}
		err = checkinRepo.Create(c)
		require.NoError(t, err)
	}

	t.Run("GET /timeline/daily - Contract", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline/daily?date=2024-01-15", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Validate status code
		assert.Equal(t, http.StatusOK, w.Code)

		// Validate response body structure
		var response timeline.DayView
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err, "Response should be valid JSON")

		// Validate required fields per OpenAPI spec
		assert.NotEmpty(t, response.Date, "date is required")
		assert.NotNil(t, response.Checkins, "checkins is required")
		assert.GreaterOrEqual(t, response.Total, 0, "total is required")

		// Validate date format (YYYY-MM-DD)
		_, err = time.Parse("2006-01-02", response.Date)
		assert.NoError(t, err, "date should be in YYYY-MM-DD format")

		// Validate content type
		assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))
	})

	t.Run("GET /timeline/daily/enhanced - Contract (Non-Paginated)", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline/daily/enhanced?date=2024-01-15&block=30&timezone=UTC", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Validate status code
		assert.Equal(t, http.StatusOK, w.Code)

		// Validate response body structure
		var response timeline.EnhancedDayView
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err, "Response should be valid JSON")

		// Validate required fields per OpenAPI spec
		assert.NotEmpty(t, response.Date, "date is required")
		assert.NotEmpty(t, response.Timezone, "timezone is required")
		assert.NotZero(t, response.BlockGranularity, "block_granularity is required")
		assert.NotNil(t, response.Blocks, "blocks is required")
		assert.NotNil(t, response.Gaps, "gaps is required")
		assert.NotNil(t, response.Summary, "summary is required")
		assert.NotNil(t, response.CategoryLegend, "category_legend is required")
		assert.NotEmpty(t, response.PreviousDay, "previous_day is required")
		assert.NotEmpty(t, response.NextDay, "next_day is required")
		assert.False(t, response.GeneratedAt.IsZero(), "generated_at is required")

		// Validate block_granularity is one of allowed values
		assert.Contains(t, []int{15, 30, 45, 120}, response.BlockGranularity)

		// Validate cache headers for non-paginated requests
		assert.NotEmpty(t, w.Header().Get("Cache-Control"), "Cache-Control header should be set")
		assert.NotEmpty(t, w.Header().Get("ETag"), "ETag header should be set")
		assert.NotEmpty(t, w.Header().Get("Last-Modified"), "Last-Modified header should be set")

		// Validate blocks structure
		for _, block := range response.Blocks {
			assert.False(t, block.StartTime.IsZero(), "block start_time is required")
			assert.False(t, block.EndTime.IsZero(), "block end_time is required")
			assert.NotZero(t, block.DurationMins, "block duration_mins is required")
			assert.NotNil(t, block.Checkins, "block checkins is required")
			// is_empty and is_gap are boolean, so they'll have default false values
		}

		// Validate summary structure
		assert.NotEmpty(t, response.Summary.Date, "summary date is required")
		assert.GreaterOrEqual(t, response.Summary.TotalCheckins, 0, "total_checkins should be non-negative")
		assert.GreaterOrEqual(t, response.Summary.TotalMinutes, 0, "total_minutes should be non-negative")
		assert.GreaterOrEqual(t, response.Summary.CompletionPercent, float64(0), "completion_percent should be non-negative")
		assert.LessOrEqual(t, response.Summary.CompletionPercent, float64(100), "completion_percent should not exceed 100")
	})

	t.Run("GET /timeline/daily/enhanced - Contract (Paginated)", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline/daily/enhanced?date=2024-01-15&block=30&limit=24", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Validate status code
		assert.Equal(t, http.StatusOK, w.Code)

		// Validate response body structure
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err, "Response should be valid JSON")

		// For paginated requests, cache headers should NOT be set
		cacheControl := w.Header().Get("Cache-Control")
		if cacheControl != "" {
			assert.Equal(t, "no-cache", cacheControl, "Paginated requests should not be cached")
		}

		// Pagination metadata should be present
		if pagination, ok := response["pagination"]; ok {
			paginationMap := pagination.(map[string]interface{})
			assert.NotNil(t, paginationMap["limit"], "pagination limit is required")
			assert.NotNil(t, paginationMap["has_next"], "pagination has_next is required")
			assert.NotNil(t, paginationMap["total_count"], "pagination total_count is required")

			// If has_next is true, next_cursor should be present
			if hasNext, ok := paginationMap["has_next"].(bool); ok && hasNext {
				assert.NotNil(t, paginationMap["next_cursor"], "next_cursor should be present when has_next is true")
			}
		}
	})

	t.Run("GET /timeline/daily/enhanced - Invalid Block Granularity", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline/daily/enhanced?date=2024-01-15&block=25", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Invalid block should return 400 Bad Request
		assert.Equal(t, http.StatusBadRequest, w.Code)

		// Validate error response structure
		var errorResponse map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &errorResponse)
		require.NoError(t, err)
		assert.Contains(t, errorResponse, "error", "Error response should contain 'error' field")
	})

	t.Run("GET /timeline/daily/enhanced - Invalid Date Format", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline/daily/enhanced?date=invalid-date", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Invalid date should return 400 Bad Request
		assert.Equal(t, http.StatusBadRequest, w.Code)

		var errorResponse map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &errorResponse)
		require.NoError(t, err)
		assert.Contains(t, errorResponse, "error")
	})

	t.Run("GET /timeline/daily/enhanced - Invalid Limit", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline/daily/enhanced?date=2024-01-15&limit=2000", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Limit > 1000 should return 400 Bad Request
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("GET /timeline/daily/enhanced - ETag Support", func(t *testing.T) {
		// First request
		req1 := httptest.NewRequest(http.MethodGet, "/api/v1/timeline/daily/enhanced?date=2024-01-15", nil)
		w1 := httptest.NewRecorder()
		router.ServeHTTP(w1, req1)

		etag := w1.Header().Get("ETag")
		require.NotEmpty(t, etag, "First response should include ETag")

		// Second request with If-None-Match header
		req2 := httptest.NewRequest(http.MethodGet, "/api/v1/timeline/daily/enhanced?date=2024-01-15", nil)
		req2.Header.Set("If-None-Match", etag)
		w2 := httptest.NewRecorder()
		router.ServeHTTP(w2, req2)

		// Should return 304 Not Modified
		assert.Equal(t, http.StatusNotModified, w2.Code)
		assert.Empty(t, w2.Body.String(), "304 response should have empty body")
	})

	t.Run("GET /timeline/weekly - Contract", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline/weekly?date=2024-01-15", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response timeline.WeekView
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Validate required fields
		assert.NotEmpty(t, response.StartDate)
		assert.NotEmpty(t, response.EndDate)
		assert.NotNil(t, response.Days)
		assert.Equal(t, 7, len(response.Days), "Week should have 7 days")
		assert.GreaterOrEqual(t, response.Total, 0)

		// Validate date formats
		_, err = time.Parse("2006-01-02", response.StartDate)
		assert.NoError(t, err)
		_, err = time.Parse("2006-01-02", response.EndDate)
		assert.NoError(t, err)

		// Validate each day
		for _, day := range response.Days {
			assert.NotEmpty(t, day.Date)
			assert.NotNil(t, day.Checkins)
			assert.GreaterOrEqual(t, day.Total, 0)
		}
	})

	t.Run("GET /timeline/monthly - Contract", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline/monthly?date=2024-01-15", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response timeline.MonthView
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Validate required fields
		assert.NotZero(t, response.Year)
		assert.NotZero(t, response.Month)
		assert.GreaterOrEqual(t, response.Month, 1)
		assert.LessOrEqual(t, response.Month, 12)
		assert.NotNil(t, response.Days)
		assert.GreaterOrEqual(t, response.Total, 0)

		// January 2024 should have 31 days
		assert.Equal(t, 31, len(response.Days))

		// Validate each day
		for _, day := range response.Days {
			assert.NotEmpty(t, day.Date)
			assert.NotNil(t, day.Checkins)
			assert.GreaterOrEqual(t, day.Total, 0)
		}
	})

	t.Run("Unauthorized Access - No Token", func(t *testing.T) {
		// Create router without auth middleware
		noAuthRouter := gin.New()
		v1 := noAuthRouter.Group("/api/v1")

		// JWT config for auth middleware
		jwtConfig := auth.JWTConfig{
			SigningMethod: auth.SigningMethodHS256,
			Secret:        "test-secret-for-contract-tests",
			AccessExpiry:  15 * time.Minute,
			RefreshExpiry: 24 * time.Hour,
		}
		jwtManager, err := auth.NewJWTManager(jwtConfig)
		require.NoError(t, err)

		timeline := v1.Group("/timeline")
		timeline.Use(middleware.AuthMiddleware(jwtManager, logger))
		{
			timeline.GET("/daily/enhanced", handler.GetDailyEnhanced)
		}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline/daily/enhanced?date=2024-01-15", nil)
		w := httptest.NewRecorder()
		noAuthRouter.ServeHTTP(w, req)

		// Should return 401 Unauthorized
		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var errorResponse map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &errorResponse)
		require.NoError(t, err)
		assert.Contains(t, errorResponse, "error")
	})
}

// TestTimelineResponseExamples validates that responses match the examples in OpenAPI spec
func TestTimelineResponseExamples(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.TearDown(t)

	userRepo := user.NewRepository(testDB.DB)
	checkinRepo := checkin.NewRepository(testDB.DB)
	categoryRepo := category.NewRepository(testDB.DB)
	logger := zap.NewNop()

	timelineService := timeline.NewService(checkinRepo, categoryRepo, nil, logger)
	handler := NewTimelineHandler(timelineService, logger)

	gin.SetMode(gin.TestMode)
	router := gin.New()

	testUser := &user.User{
		ID:          uuid.New(),
		Email:       "example@example.com",
		Username:    "exampleuser",
		DisplayName: "Example User",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err := userRepo.Create(testUser, "hashedpassword")
	require.NoError(t, err)

	authMiddleware := func(c *gin.Context) {
		c.Set("user_id", testUser.ID)
		c.Next()
	}

	v1 := router.Group("/api/v1")
	timeline := v1.Group("/timeline")
	timeline.Use(authMiddleware)
	{
		timeline.GET("/daily/enhanced", handler.GetDailyEnhanced)
	}

	t.Run("Response matches OpenAPI example structure", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline/daily/enhanced?date=2024-01-15&block=30", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response timeline.EnhancedDayView
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Validate structure matches example
		assert.IsType(t, "", response.Date)
		assert.IsType(t, "", response.Timezone)
		assert.IsType(t, 0, response.BlockGranularity)
		assert.IsType(t, []*timeline.TimeBlock{}, response.Blocks)
		assert.IsType(t, []*timeline.Gap{}, response.Gaps)
		assert.IsType(t, &timeline.DailySummary{}, response.Summary)
		assert.IsType(t, []*timeline.CategoryLegend{}, response.CategoryLegend)
		assert.IsType(t, "", response.PreviousDay)
		assert.IsType(t, "", response.NextDay)
		assert.IsType(t, time.Time{}, response.GeneratedAt)
	})
}

func strPtr(s string) *string {
	return &s
}
