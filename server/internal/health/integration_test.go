package health_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/dev-jelly/donelist/internal/health"
)

func setupTestRouter(healthService *health.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Simple health check endpoint
	healthHandler := func(c *gin.Context) {
		status, _ := healthService.CheckHealthSimple()
		httpStatus := http.StatusOK
		if status != health.StatusHealthy {
			httpStatus = http.StatusServiceUnavailable
		}
		buildInfo := health.GetBuildInfo()
		c.JSON(httpStatus, gin.H{
			"status":     status,
			"time":       time.Now().Format(time.RFC3339),
			"version":    buildInfo.Version,
			"git_commit": buildInfo.GitCommit,
		})
	}
	router.GET("/healthz", healthHandler)

	// Detailed health check endpoint
	detailHandler := func(c *gin.Context) {
		report := healthService.CheckHealthWithBuildInfo()
		httpStatus := http.StatusOK
		if report.Status != health.StatusHealthy {
			httpStatus = http.StatusServiceUnavailable
		}
		c.JSON(httpStatus, report)
	}
	router.GET("/healthz/detail", detailHandler)

	// Readiness check endpoint
	readyHandler := func(c *gin.Context) {
		report := healthService.CheckHealth()
		buildInfo := health.GetBuildInfo()

		response := gin.H{
			"version":    buildInfo.Version,
			"git_commit": buildInfo.GitCommit,
			"uptime":     healthService.GetUptime().Seconds(),
		}

		// Check if all critical components are healthy
		dbHealth, dbOk := report.Components["postgresql"]
		redisHealth, redisOk := report.Components["redis"]

		if !dbOk || dbHealth.Status != health.StatusHealthy {
			response["status"] = "not ready"
			response["error"] = "database not available"
			if dbHealth.Error != "" {
				response["details"] = dbHealth.Error
			}
			c.JSON(http.StatusServiceUnavailable, response)
			return
		}

		if !redisOk || redisHealth.Status != health.StatusHealthy {
			response["status"] = "not ready"
			response["error"] = "redis not available"
			if redisHealth.Error != "" {
				response["details"] = redisHealth.Error
			}
			c.JSON(http.StatusServiceUnavailable, response)
			return
		}

		response["status"] = "ready"
		response["database"] = "connected"
		response["redis"] = "connected"
		c.JSON(http.StatusOK, response)
	}
	router.GET("/readyz", readyHandler)

	return router
}

func TestHealthzEndpoint_Healthy(t *testing.T) {
	logger := zap.NewNop()
	service := health.NewService("1.0.0", logger)

	// Register healthy checkers
	service.RegisterChecker(health.NewDummyChecker("test", health.StatusHealthy))

	router := setupTestRouter(service)

	// Make request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/healthz", nil)
	router.ServeHTTP(w, req)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "healthy", response["status"])
	assert.NotEmpty(t, response["time"])
	assert.NotEmpty(t, response["version"])
}

func TestHealthzEndpoint_Unhealthy(t *testing.T) {
	logger := zap.NewNop()
	service := health.NewService("1.0.0", logger)

	// Register unhealthy checker
	service.RegisterChecker(health.NewDummyChecker("test", health.StatusUnhealthy))

	router := setupTestRouter(service)

	// Make request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/healthz", nil)
	router.ServeHTTP(w, req)

	// Verify response - should return 503
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "unhealthy", response["status"])
}

func TestHealthzDetailEndpoint_WithComponents(t *testing.T) {
	logger := zap.NewNop()
	service := health.NewService("1.0.0", logger)

	// Register checkers
	service.RegisterChecker(health.NewDummyChecker("component1", health.StatusHealthy))
	service.RegisterChecker(health.NewDummyChecker("component2", health.StatusHealthy))

	router := setupTestRouter(service)

	// Make request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/healthz/detail", nil)
	router.ServeHTTP(w, req)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "healthy", response["status"])
	assert.NotEmpty(t, response["version"])

	// Check build info
	buildInfo, ok := response["build_info"].(map[string]interface{})
	require.True(t, ok)
	assert.NotEmpty(t, buildInfo["version"])

	// Check components
	components, ok := response["components"].(map[string]interface{})
	require.True(t, ok)
	assert.Len(t, components, 2)
}

func TestReadyzEndpoint_AllHealthy(t *testing.T) {
	logger := zap.NewNop()
	service := health.NewService("1.0.0", logger)

	// Register healthy PostgreSQL and Redis checkers
	service.RegisterChecker(health.NewDummyChecker("postgresql", health.StatusHealthy))
	service.RegisterChecker(health.NewDummyChecker("redis", health.StatusHealthy))

	router := setupTestRouter(service)

	// Make request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/readyz", nil)
	router.ServeHTTP(w, req)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "ready", response["status"])
	assert.Equal(t, "connected", response["database"])
	assert.Equal(t, "connected", response["redis"])
}

func TestReadyzEndpoint_DatabaseUnhealthy(t *testing.T) {
	logger := zap.NewNop()
	service := health.NewService("1.0.0", logger)

	// Register unhealthy PostgreSQL checker
	service.RegisterChecker(health.NewDummyChecker("postgresql", health.StatusUnhealthy))
	service.RegisterChecker(health.NewDummyChecker("redis", health.StatusHealthy))

	router := setupTestRouter(service)

	// Make request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/readyz", nil)
	router.ServeHTTP(w, req)

	// Verify response - should return 503
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "not ready", response["status"])
	assert.Equal(t, "database not available", response["error"])
}

func TestReadyzEndpoint_RedisUnhealthy(t *testing.T) {
	logger := zap.NewNop()
	service := health.NewService("1.0.0", logger)

	// Register healthy PostgreSQL but unhealthy Redis
	service.RegisterChecker(health.NewDummyChecker("postgresql", health.StatusHealthy))
	service.RegisterChecker(health.NewDummyChecker("redis", health.StatusUnhealthy))

	router := setupTestRouter(service)

	// Make request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/readyz", nil)
	router.ServeHTTP(w, req)

	// Verify response - should return 503
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "not ready", response["status"])
	assert.Equal(t, "redis not available", response["error"])
}

func TestPostgresChecker_Integration_Unhealthy(t *testing.T) {
	// Create mock database that will fail
	mockDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)
	defer mockDB.Close()

	db := sqlx.NewDb(mockDB, "postgres")
	db.SetMaxOpenConns(10)

	// Expect ping to fail
	mock.ExpectPing().WillReturnError(assert.AnError)

	logger := zap.NewNop()
	service := health.NewService("1.0.0", logger)

	// Register the unhealthy PostgreSQL checker
	service.RegisterChecker(health.NewPostgresChecker(db, 2*time.Second))

	router := setupTestRouter(service)

	// Make request to /readyz
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/readyz", nil)
	router.ServeHTTP(w, req)

	// Verify response returns 503
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "not ready", response["status"])
	assert.Equal(t, "database not available", response["error"])

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRedisChecker_Integration(t *testing.T) {
	// Create a Redis client (will fail if Redis is not running)
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:9999", // Intentionally wrong port
	})
	defer client.Close()

	logger := zap.NewNop()
	service := health.NewService("1.0.0", logger)

	// Register healthy PostgreSQL checker and unhealthy Redis checker
	service.RegisterChecker(health.NewDummyChecker("postgresql", health.StatusHealthy))
	service.RegisterChecker(health.NewRedisChecker(client, 100*time.Millisecond))

	router := setupTestRouter(service)

	// Make request to /readyz
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/readyz", nil)
	router.ServeHTTP(w, req)

	// Verify response returns 503
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "not ready", response["status"])
	assert.Equal(t, "redis not available", response["error"])
}

func TestHealthEndpoints_ResponseSchema(t *testing.T) {
	// This test verifies the response schema is consistent
	logger := zap.NewNop()
	service := health.NewService("1.0.0", logger)

	// For /readyz, we need both postgresql and redis checkers
	service.RegisterChecker(health.NewDummyChecker("postgresql", health.StatusHealthy))
	service.RegisterChecker(health.NewDummyChecker("redis", health.StatusHealthy))

	router := setupTestRouter(service)

	tests := []struct {
		name             string
		endpoint         string
		expectedFields   []string
		expectedStatus   int
	}{
		{
			name:           "/healthz schema",
			endpoint:       "/healthz",
			expectedFields: []string{"status", "time", "version", "git_commit"},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "/healthz/detail schema",
			endpoint:       "/healthz/detail",
			expectedFields: []string{"status", "version", "build_info", "timestamp", "uptime_seconds", "components"},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "/readyz schema",
			endpoint:       "/readyz",
			expectedFields: []string{"status", "version", "git_commit", "uptime"},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", tt.endpoint, nil)
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			// Verify all expected fields are present
			for _, field := range tt.expectedFields {
				assert.Contains(t, response, field, "Response should contain field: %s", field)
			}
		})
	}
}
