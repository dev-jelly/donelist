// +build contract

package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dev-jelly/donelist/internal/calendar"
	"github.com/dev-jelly/donelist/internal/category"
	"github.com/dev-jelly/donelist/internal/checkin"
	"github.com/dev-jelly/donelist/internal/testutil"
	"github.com/dev-jelly/donelist/internal/user"
	"github.com/dev-jelly/donelist/pkg/cache"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"
	"go.uber.org/zap"
)

// Calendar API JSON Schemas for contract validation
var monthlyCalendarSchema = `{
	"$schema": "http://json-schema.org/draft-07/schema#",
	"type": "object",
	"required": ["year", "month", "month_name", "start_day", "weeks", "summary", "previous_month", "next_month", "generated_at"],
	"properties": {
		"year": {"type": "integer", "minimum": 1970, "maximum": 2100},
		"month": {"type": "integer", "minimum": 1, "maximum": 12},
		"month_name": {"type": "string", "minLength": 1},
		"start_day": {"type": "string", "enum": ["sunday", "monday"]},
		"weeks": {
			"type": "array",
			"minItems": 4,
			"maxItems": 6,
			"items": {
				"type": "object",
				"required": ["days"],
				"properties": {
					"days": {
						"type": "array",
						"minItems": 7,
						"maxItems": 7,
						"items": {
							"type": "object",
							"required": ["date", "checkin_count", "total_minutes", "completion_percent", "color_intensity", "is_current_month", "has_checkins"],
							"properties": {
								"date": {"type": "string", "pattern": "^\\d{4}-\\d{2}-\\d{2}$"},
								"checkin_count": {"type": "integer", "minimum": 0},
								"total_minutes": {"type": "integer", "minimum": 0},
								"completion_percent": {"type": "number", "minimum": 0, "maximum": 100},
								"color_intensity": {"type": "integer", "minimum": 0, "maximum": 4},
								"is_current_month": {"type": "boolean"},
								"is_today": {"type": "boolean"},
								"has_checkins": {"type": "boolean"}
							}
						}
					}
				}
			}
		},
		"summary": {
			"type": "object",
			"required": ["total_checkins", "total_minutes", "days_with_checkins", "total_days_in_month", "average_per_day", "completion_rate"],
			"properties": {
				"total_checkins": {"type": "integer", "minimum": 0},
				"total_minutes": {"type": "integer", "minimum": 0},
				"days_with_checkins": {"type": "integer", "minimum": 0},
				"total_days_in_month": {"type": "integer", "minimum": 28, "maximum": 31},
				"average_per_day": {"type": "number", "minimum": 0},
				"completion_rate": {"type": "number", "minimum": 0, "maximum": 100},
				"most_productive_day": {"type": "string"},
				"most_productive_count": {"type": "integer"},
				"current_streak": {"type": "integer", "minimum": 0},
				"longest_streak": {"type": "integer", "minimum": 0}
			}
		},
		"categories": {
			"type": "array",
			"items": {
				"type": "object",
				"required": ["category_name", "count", "percentage"],
				"properties": {
					"category_id": {"type": "string", "format": "uuid"},
					"category_name": {"type": "string"},
					"count": {"type": "integer", "minimum": 0},
					"percentage": {"type": "number", "minimum": 0, "maximum": 100}
				}
			}
		},
		"previous_month": {"type": "string", "pattern": "^\\d{4}-\\d{2}$"},
		"next_month": {"type": "string", "pattern": "^\\d{4}-\\d{2}$"},
		"generated_at": {"type": "string", "format": "date-time"},
		"cache_expiration": {"type": "string", "format": "date-time"}
	}
}`

var monthlyCalendarV2Schema = `{
	"$schema": "http://json-schema.org/draft-07/schema#",
	"type": "object",
	"required": ["schemaVersion", "data"],
	"properties": {
		"schemaVersion": {"type": "string", "pattern": "^\\d+\\.\\d+\\.\\d+$"},
		"data": {
			"$ref": "#/definitions/MonthlyCalendar"
		}
	},
	"definitions": {
		"MonthlyCalendar": {
			"type": "object",
			"required": ["year", "month", "month_name", "start_day", "weeks", "summary", "previous_month", "next_month", "generated_at"],
			"properties": {
				"year": {"type": "integer"},
				"month": {"type": "integer"},
				"month_name": {"type": "string"},
				"start_day": {"type": "string"},
				"weeks": {"type": "array"},
				"summary": {"type": "object"},
				"categories": {"type": "array"},
				"previous_month": {"type": "string"},
				"next_month": {"type": "string"},
				"generated_at": {"type": "string"},
				"cache_expiration": {"type": "string"}
			}
		}
	}
}`

var heatmapSchema = `{
	"$schema": "http://json-schema.org/draft-07/schema#",
	"type": "object",
	"required": ["start_date", "end_date", "days", "total_days", "active_days", "total_checkins", "generated_at"],
	"properties": {
		"start_date": {"type": "string", "pattern": "^\\d{4}-\\d{2}-\\d{2}$"},
		"end_date": {"type": "string", "pattern": "^\\d{4}-\\d{2}-\\d{2}$"},
		"days": {
			"type": "array",
			"items": {
				"type": "object",
				"required": ["date", "checkin_count", "total_minutes", "completion_percent", "color_intensity"],
				"properties": {
					"date": {"type": "string", "pattern": "^\\d{4}-\\d{2}-\\d{2}$"},
					"checkin_count": {"type": "integer", "minimum": 0},
					"total_minutes": {"type": "integer", "minimum": 0},
					"completion_percent": {"type": "number", "minimum": 0, "maximum": 100},
					"color_intensity": {"type": "integer", "minimum": 0, "maximum": 4}
				}
			}
		},
		"total_days": {"type": "integer", "minimum": 1, "maximum": 366},
		"active_days": {"type": "integer", "minimum": 0},
		"total_checkins": {"type": "integer", "minimum": 0},
		"generated_at": {"type": "string", "format": "date-time"}
	}
}`

// TestCalendarAPIContract validates that the Calendar API adheres to the contract
func TestCalendarAPIContract(t *testing.T) {
	// Setup test database
	testDB := testutil.SetupTestDB(t)
	defer testDB.TearDown(t)

	// Initialize repositories
	userRepo := user.NewRepository(testDB.DB)
	checkinRepo := checkin.NewRepository(testDB.DB)
	categoryRepo := category.NewRepository(testDB.DB)

	// Setup logger
	logger := zap.NewNop()

	// Setup calendar service
	calendarService := calendar.NewService(checkinRepo, categoryRepo, logger)

	// Setup handler
	handler := NewCalendarHandler(calendarService, logger)

	// Setup router with auth
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Create test user
	testUser := &user.User{
		ID:          uuid.New(),
		Email:       "calendar@example.com",
		Username:    "calendaruser",
		DisplayName: "Calendar User",
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
	calendarGroup := v1.Group("/calendar")
	calendarGroup.Use(authMiddleware)
	{
		calendarGroup.GET("/monthly", handler.GetMonthlyCalendar)
		calendarGroup.GET("/heatmap", handler.GetHeatmap)
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

	// Create check-ins across the month
	baseDate := time.Date(2024, 11, 1, 9, 0, 0, 0, time.UTC)
	for i := 0; i < 15; i++ {
		checkinTime := baseDate.AddDate(0, 0, i*2)
		c := &checkin.Checkin{
			ID:              uuid.New(),
			UserID:          testUser.ID,
			Title:           "Check-in " + string(rune('A'+i%26)),
			CategoryID:      &cat.ID,
			CheckinTime:     checkinTime,
			DurationMinutes: 60,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}
		err = checkinRepo.Create(c)
		require.NoError(t, err)
	}

	t.Run("MonthlyCalendar_Contract", func(t *testing.T) {
		testCases := []struct {
			name        string
			queryParams string
			statusCode  int
		}{
			{
				name:        "Default_CurrentMonth",
				queryParams: "",
				statusCode:  http.StatusOK,
			},
			{
				name:        "November_2024_Monday",
				queryParams: "?year=2024&month=11&start_day=monday",
				statusCode:  http.StatusOK,
			},
			{
				name:        "November_2024_Sunday",
				queryParams: "?year=2024&month=11&start_day=sunday",
				statusCode:  http.StatusOK,
			},
			{
				name:        "February_2024_LeapYear",
				queryParams: "?year=2024&month=2",
				statusCode:  http.StatusOK,
			},
			{
				name:        "February_2023_NonLeapYear",
				queryParams: "?year=2023&month=2",
				statusCode:  http.StatusOK,
			},
			{
				name:        "January_2024",
				queryParams: "?year=2024&month=1",
				statusCode:  http.StatusOK,
			},
			{
				name:        "December_2024",
				queryParams: "?year=2024&month=12",
				statusCode:  http.StatusOK,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				req := httptest.NewRequest("GET", "/api/v1/calendar/monthly"+tc.queryParams, nil)
				w := httptest.NewRecorder()

				router.ServeHTTP(w, req)

				assert.Equal(t, tc.statusCode, w.Code, "Status code mismatch")

				if w.Code == http.StatusOK {
					// Validate response against JSON schema
					schemaLoader := gojsonschema.NewStringLoader(monthlyCalendarSchema)
					documentLoader := gojsonschema.NewStringLoader(w.Body.String())

					result, err := gojsonschema.Validate(schemaLoader, documentLoader)
					require.NoError(t, err, "Schema validation error")

					if !result.Valid() {
						for _, desc := range result.Errors() {
							t.Errorf("Schema validation error: %s", desc)
						}
					}
					assert.True(t, result.Valid(), "Response should match JSON schema")

					// Parse and validate specific fields
					var response calendar.MonthlyCalendar
					err = json.Unmarshal(w.Body.Bytes(), &response)
					require.NoError(t, err)

					// Validate week count (4-6 weeks)
					assert.GreaterOrEqual(t, len(response.Weeks), 4, "Should have at least 4 weeks")
					assert.LessOrEqual(t, len(response.Weeks), 6, "Should have at most 6 weeks")

					// Validate each week has 7 days
					for i, week := range response.Weeks {
						assert.Len(t, week.Days, 7, "Week %d should have 7 days", i+1)
					}

					// Validate summary
					assert.NotNil(t, response.Summary, "Summary should be present")
					assert.GreaterOrEqual(t, response.Summary.TotalDaysInMonth, 28, "Month should have at least 28 days")
					assert.LessOrEqual(t, response.Summary.TotalDaysInMonth, 31, "Month should have at most 31 days")
				}
			})
		}
	})

	t.Run("MonthlyCalendar_Validation", func(t *testing.T) {
		testCases := []struct {
			name        string
			queryParams string
			statusCode  int
			errorMsg    string
		}{
			{
				name:        "Invalid_Year_TooLow",
				queryParams: "?year=1900&month=1",
				statusCode:  http.StatusBadRequest,
				errorMsg:    "invalid year",
			},
			{
				name:        "Invalid_Year_TooHigh",
				queryParams: "?year=2200&month=1",
				statusCode:  http.StatusBadRequest,
				errorMsg:    "invalid year",
			},
			{
				name:        "Invalid_Month_Zero",
				queryParams: "?year=2024&month=0",
				statusCode:  http.StatusBadRequest,
				errorMsg:    "invalid month",
			},
			{
				name:        "Invalid_Month_Thirteen",
				queryParams: "?year=2024&month=13",
				statusCode:  http.StatusBadRequest,
				errorMsg:    "invalid month",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				req := httptest.NewRequest("GET", "/api/v1/calendar/monthly"+tc.queryParams, nil)
				w := httptest.NewRecorder()

				router.ServeHTTP(w, req)

				assert.Equal(t, tc.statusCode, w.Code)

				var response map[string]string
				json.Unmarshal(w.Body.Bytes(), &response)
				assert.Contains(t, response["error"], tc.errorMsg)
			})
		}
	})

	t.Run("Heatmap_Contract", func(t *testing.T) {
		testCases := []struct {
			name        string
			queryParams string
			statusCode  int
		}{
			{
				name:        "OneMonth",
				queryParams: "?start_date=2024-11-01&end_date=2024-11-30",
				statusCode:  http.StatusOK,
			},
			{
				name:        "ThreeMonths",
				queryParams: "?start_date=2024-01-01&end_date=2024-03-31",
				statusCode:  http.StatusOK,
			},
			{
				name:        "FullYear",
				queryParams: "?start_date=2024-01-01&end_date=2024-12-31",
				statusCode:  http.StatusOK,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				req := httptest.NewRequest("GET", "/api/v1/calendar/heatmap"+tc.queryParams, nil)
				w := httptest.NewRecorder()

				router.ServeHTTP(w, req)

				assert.Equal(t, tc.statusCode, w.Code, "Status code mismatch")

				if w.Code == http.StatusOK {
					// Validate response against JSON schema
					schemaLoader := gojsonschema.NewStringLoader(heatmapSchema)
					documentLoader := gojsonschema.NewStringLoader(w.Body.String())

					result, err := gojsonschema.Validate(schemaLoader, documentLoader)
					require.NoError(t, err, "Schema validation error")

					if !result.Valid() {
						for _, desc := range result.Errors() {
							t.Errorf("Schema validation error: %s", desc)
						}
					}
					assert.True(t, result.Valid(), "Response should match JSON schema")
				}
			})
		}
	})

	t.Run("Heatmap_Validation", func(t *testing.T) {
		testCases := []struct {
			name        string
			queryParams string
			statusCode  int
			errorMsg    string
		}{
			{
				name:        "Missing_StartDate",
				queryParams: "?end_date=2024-11-30",
				statusCode:  http.StatusBadRequest,
				errorMsg:    "start_date",
			},
			{
				name:        "Missing_EndDate",
				queryParams: "?start_date=2024-11-01",
				statusCode:  http.StatusBadRequest,
				errorMsg:    "end_date",
			},
			{
				name:        "EndDate_Before_StartDate",
				queryParams: "?start_date=2024-11-30&end_date=2024-11-01",
				statusCode:  http.StatusBadRequest,
				errorMsg:    "end_date must be after start_date",
			},
			{
				name:        "DateRange_TooLarge",
				queryParams: "?start_date=2023-01-01&end_date=2024-12-31",
				statusCode:  http.StatusBadRequest,
				errorMsg:    "cannot exceed",
			},
			{
				name:        "Invalid_StartDate_Format",
				queryParams: "?start_date=2024/11/01&end_date=2024-11-30",
				statusCode:  http.StatusBadRequest,
				errorMsg:    "invalid start_date",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				req := httptest.NewRequest("GET", "/api/v1/calendar/heatmap"+tc.queryParams, nil)
				w := httptest.NewRecorder()

				router.ServeHTTP(w, req)

				assert.Equal(t, tc.statusCode, w.Code)

				var response map[string]string
				json.Unmarshal(w.Body.Bytes(), &response)
				assert.Contains(t, response["error"], tc.errorMsg)
			})
		}
	})
}

// TestCalendarAPIV2Contract validates the V2 API with caching and ETags
func TestCalendarAPIV2Contract(t *testing.T) {
	// Setup test database
	testDB := testutil.SetupTestDB(t)
	defer testDB.TearDown(t)

	// Skip if Redis is not available
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   1,
	})
	if err := redisClient.Ping(t).Err(); err != nil {
		t.Skip("Skipping V2 tests: Redis not available")
	}
	defer redisClient.Close()
	redisClient.FlushDB(t)

	// Initialize repositories
	userRepo := user.NewRepository(testDB.DB)
	checkinRepo := checkin.NewRepository(testDB.DB)
	categoryRepo := category.NewRepository(testDB.DB)

	// Setup logger
	logger := zap.NewNop()

	// Setup cache
	cacheConfig := cache.Config{
		Prefix:     "test:calendar:",
		DefaultTTL: 15 * time.Minute,
	}
	testCache := cache.NewCache(redisClient, cacheConfig, logger)

	// Setup calendar service
	calendarService := calendar.NewService(checkinRepo, categoryRepo, logger)

	// Setup handler
	handler := NewCalendarHandler(calendarService, logger)
	handler.SetCache(testCache)

	// Setup router with auth
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Create test user
	testUser := &user.User{
		ID:          uuid.New(),
		Email:       "calendarv2@example.com",
		Username:    "calendarv2user",
		DisplayName: "Calendar V2 User",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err := userRepo.Create(testUser, "hashedpassword")
	require.NoError(t, err)

	// Mock auth middleware
	authMiddleware := func(c *gin.Context) {
		c.Set("user_id", testUser.ID)
		c.Next()
	}

	// Setup V2 route
	v1 := router.Group("/api/v1")
	calendarGroup := v1.Group("/calendar")
	calendarGroup.Use(authMiddleware)
	{
		calendarGroup.GET("/month", handler.GetMonthlyCalendarV2)
	}

	t.Run("V2_Response_Headers", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/calendar/month?year=2024&month=11", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// Verify required headers
		assert.NotEmpty(t, w.Header().Get("ETag"), "ETag header should be present")
		assert.Equal(t, "private, max-age=900", w.Header().Get("Cache-Control"), "Cache-Control header should be set")
		assert.NotEmpty(t, w.Header().Get("X-Schema-Version"), "X-Schema-Version header should be present")
		assert.NotEmpty(t, w.Header().Get("Expires"), "Expires header should be present")
	})

	t.Run("V2_ETag_NotModified", func(t *testing.T) {
		// First request to get ETag
		req1 := httptest.NewRequest("GET", "/api/v1/calendar/month?year=2024&month=11", nil)
		w1 := httptest.NewRecorder()
		router.ServeHTTP(w1, req1)

		etag := w1.Header().Get("ETag")
		require.NotEmpty(t, etag, "First request should return ETag")

		// Second request with If-None-Match
		req2 := httptest.NewRequest("GET", "/api/v1/calendar/month?year=2024&month=11", nil)
		req2.Header.Set("If-None-Match", etag)
		w2 := httptest.NewRecorder()
		router.ServeHTTP(w2, req2)

		assert.Equal(t, http.StatusNotModified, w2.Code, "Should return 304 Not Modified")
		assert.Empty(t, w2.Body.String(), "Body should be empty for 304")
		assert.Equal(t, etag, w2.Header().Get("ETag"), "ETag should be same")
	})

	t.Run("V2_Schema_Version", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/calendar/month?year=2024&month=11", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Verify schema version in response
		schemaVersion, ok := response["schemaVersion"].(string)
		assert.True(t, ok, "schemaVersion should be a string")
		assert.Regexp(t, `^\d+\.\d+\.\d+$`, schemaVersion, "schemaVersion should be semver format")
	})

	t.Run("V2_Required_Parameters", func(t *testing.T) {
		testCases := []struct {
			name        string
			queryParams string
			statusCode  int
			errorMsg    string
		}{
			{
				name:        "Missing_Year",
				queryParams: "?month=11",
				statusCode:  http.StatusBadRequest,
				errorMsg:    "year parameter is required",
			},
			{
				name:        "Missing_Month",
				queryParams: "?year=2024",
				statusCode:  http.StatusBadRequest,
				errorMsg:    "month parameter is required",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				req := httptest.NewRequest("GET", "/api/v1/calendar/month"+tc.queryParams, nil)
				w := httptest.NewRecorder()

				router.ServeHTTP(w, req)

				assert.Equal(t, tc.statusCode, w.Code)

				var response map[string]string
				json.Unmarshal(w.Body.Bytes(), &response)
				assert.Contains(t, response["error"], tc.errorMsg)
			})
		}
	})
}

// TestCalendarLeapYearHandling ensures leap years are correctly handled
func TestCalendarLeapYearHandling(t *testing.T) {
	testCases := []struct {
		year        int
		isLeapYear  bool
		febDays     int
	}{
		{2024, true, 29},
		{2023, false, 28},
		{2000, true, 29},  // Divisible by 400
		{1900, false, 28}, // Divisible by 100 but not 400
		{2020, true, 29},
	}

	for _, tc := range testCases {
		t.Run(time.Date(tc.year, 2, 1, 0, 0, 0, 0, time.UTC).Format("2006"), func(t *testing.T) {
			// Calculate days in February
			febEnd := time.Date(tc.year, 3, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
			daysInFeb := febEnd.Day()

			assert.Equal(t, tc.febDays, daysInFeb, "February %d should have %d days", tc.year, tc.febDays)

			// Verify our leap year detection matches
			isLeap := daysInFeb == 29
			assert.Equal(t, tc.isLeapYear, isLeap, "Year %d leap year status", tc.year)
		})
	}
}
