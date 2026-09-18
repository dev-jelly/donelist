package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dev-jelly/donelist/internal/health"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupHealthTestRouter(service *health.Service) (*gin.Engine, *HealthHandler) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	logger, _ := zap.NewDevelopment()
	handler := NewHealthHandler(service, logger)
	return router, handler
}

func TestHealthHandler_HealthCheck(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	_ = health.NewService("1.0.0", logger)

	tests := []struct {
		name               string
		checkers           []health.Checker
		expectedStatus     int
		expectedHealthy    bool
		expectedStatusText string
	}{
		{
			name:               "all healthy",
			checkers:           []health.Checker{health.NewDummyChecker("test", health.StatusHealthy)},
			expectedStatus:     http.StatusOK,
			expectedHealthy:    true,
			expectedStatusText: "healthy",
		},
		{
			name:               "one unhealthy",
			checkers:           []health.Checker{health.NewDummyChecker("test", health.StatusUnhealthy)},
			expectedStatus:     http.StatusServiceUnavailable,
			expectedHealthy:    false,
			expectedStatusText: "unhealthy",
		},
		{
			name:               "degraded",
			checkers:           []health.Checker{health.NewDummyChecker("test", health.StatusDegraded)},
			expectedStatus:     http.StatusServiceUnavailable,
			expectedHealthy:    false,
			expectedStatusText: "degraded",
		},
		{
			name:               "no checkers",
			checkers:           []health.Checker{},
			expectedStatus:     http.StatusOK,
			expectedHealthy:    true,
			expectedStatusText: "healthy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testService := health.NewService("1.0.0", logger)
			for _, checker := range tt.checkers {
				testService.RegisterChecker(checker)
			}

			router, handler := setupHealthTestRouter(testService)
			router.GET("/health", handler.HealthCheck)

			req := httptest.NewRequest(http.MethodGet, "/health", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatusText, response["status"])
			assert.NotEmpty(t, response["time"])
			assert.NotEmpty(t, response["version"])
		})
	}
}

func TestHealthHandler_DetailedHealthCheck(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	tests := []struct {
		name           string
		checkers       []health.Checker
		expectedStatus int
		checkFields    func(t *testing.T, response map[string]interface{})
	}{
		{
			name: "detailed healthy response",
			checkers: []health.Checker{
				health.NewDummyChecker("component1", health.StatusHealthy),
				health.NewDummyChecker("component2", health.StatusHealthy),
			},
			expectedStatus: http.StatusOK,
			checkFields: func(t *testing.T, response map[string]interface{}) {
				assert.Equal(t, "healthy", response["status"])
				assert.NotEmpty(t, response["version"])
				assert.NotNil(t, response["build_info"])
				assert.NotNil(t, response["components"])

				components := response["components"].(map[string]interface{})
				assert.Len(t, components, 2)
				assert.NotNil(t, components["component1"])
				assert.NotNil(t, components["component2"])
			},
		},
		{
			name: "detailed unhealthy response",
			checkers: []health.Checker{
				health.NewDummyChecker("component1", health.StatusHealthy),
				health.NewDummyChecker("component2", health.StatusUnhealthy),
			},
			expectedStatus: http.StatusServiceUnavailable,
			checkFields: func(t *testing.T, response map[string]interface{}) {
				assert.Equal(t, "unhealthy", response["status"])
				components := response["components"].(map[string]interface{})
				assert.Len(t, components, 2)
			},
		},
		{
			name: "build info included",
			checkers: []health.Checker{
				health.NewDummyChecker("test", health.StatusHealthy),
			},
			expectedStatus: http.StatusOK,
			checkFields: func(t *testing.T, response map[string]interface{}) {
				buildInfo := response["build_info"].(map[string]interface{})
				assert.NotEmpty(t, buildInfo["version"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testService := health.NewService("1.0.0", logger)
			for _, checker := range tt.checkers {
				testService.RegisterChecker(checker)
			}

			router, handler := setupHealthTestRouter(testService)
			router.GET("/health/detail", handler.DetailedHealthCheck)

			req := httptest.NewRequest(http.MethodGet, "/health/detail", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			tt.checkFields(t, response)
		})
	}
}

func TestHealthHandler_ReadinessCheck(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	tests := []struct {
		name           string
		checkers       []health.Checker
		expectedStatus int
		expectedReady  bool
		checkResponse  func(t *testing.T, response map[string]interface{})
	}{
		{
			name: "ready when all critical components healthy",
			checkers: []health.Checker{
				health.NewDummyChecker("postgresql", health.StatusHealthy),
				health.NewDummyChecker("redis", health.StatusHealthy),
			},
			expectedStatus: http.StatusOK,
			expectedReady:  true,
			checkResponse: func(t *testing.T, response map[string]interface{}) {
				assert.Equal(t, "ready", response["status"])
				assert.NotNil(t, response["database"])
				assert.NotNil(t, response["redis"])
				assert.NotEmpty(t, response["version"])
				assert.NotEmpty(t, response["git_commit"])
				assert.NotEmpty(t, response["uptime"])
			},
		},
		{
			name: "not ready when database unhealthy",
			checkers: []health.Checker{
				health.NewDummyChecker("postgresql", health.StatusUnhealthy),
				health.NewDummyChecker("redis", health.StatusHealthy),
			},
			expectedStatus: http.StatusServiceUnavailable,
			expectedReady:  false,
			checkResponse: func(t *testing.T, response map[string]interface{}) {
				assert.Equal(t, "not ready", response["status"])
				assert.Equal(t, "database not available", response["error"])
			},
		},
		{
			name: "not ready when redis unhealthy",
			checkers: []health.Checker{
				health.NewDummyChecker("postgresql", health.StatusHealthy),
				health.NewDummyChecker("redis", health.StatusUnhealthy),
			},
			expectedStatus: http.StatusServiceUnavailable,
			expectedReady:  false,
			checkResponse: func(t *testing.T, response map[string]interface{}) {
				assert.Equal(t, "not ready", response["status"])
				assert.Equal(t, "redis not available", response["error"])
			},
		},
		{
			name: "not ready when database degraded",
			checkers: []health.Checker{
				health.NewDummyChecker("postgresql", health.StatusDegraded),
				health.NewDummyChecker("redis", health.StatusHealthy),
			},
			expectedStatus: http.StatusServiceUnavailable,
			expectedReady:  false,
			checkResponse: func(t *testing.T, response map[string]interface{}) {
				assert.Equal(t, "not ready", response["status"])
				assert.Contains(t, response["error"], "database")
			},
		},
		{
			name: "not ready when critical component missing",
			checkers: []health.Checker{
				health.NewDummyChecker("system", health.StatusHealthy),
			},
			expectedStatus: http.StatusServiceUnavailable,
			expectedReady:  false,
			checkResponse: func(t *testing.T, response map[string]interface{}) {
				assert.Equal(t, "not ready", response["status"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testService := health.NewService("1.0.0", logger)
			for _, checker := range tt.checkers {
				testService.RegisterChecker(checker)
			}

			router, handler := setupHealthTestRouter(testService)
			router.GET("/ready", handler.ReadinessCheck)

			req := httptest.NewRequest(http.MethodGet, "/ready", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			tt.checkResponse(t, response)
		})
	}
}

func TestHealthHandler_LivenessCheck(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	_ = health.NewService("1.0.0", logger)

	tests := []struct {
		name           string
		sleepDuration  time.Duration
		checkResponse  func(t *testing.T, response map[string]interface{})
	}{
		{
			name:          "basic liveness check",
			sleepDuration: 0,
			checkResponse: func(t *testing.T, response map[string]interface{}) {
				assert.Equal(t, "alive", response["status"])
				assert.NotEmpty(t, response["time"])
				assert.NotEmpty(t, response["version"])
				assert.NotEmpty(t, response["git_commit"])
				assert.NotEmpty(t, response["uptime"])
			},
		},
		{
			name:          "liveness with uptime",
			sleepDuration: 100 * time.Millisecond,
			checkResponse: func(t *testing.T, response map[string]interface{}) {
				assert.Equal(t, "alive", response["status"])
				uptime := response["uptime"].(float64)
				assert.Greater(t, uptime, 0.0)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testService := health.NewService("1.0.0", logger)
			router, handler := setupHealthTestRouter(testService)
			router.GET("/live", handler.LivenessCheck)

			if tt.sleepDuration > 0 {
				time.Sleep(tt.sleepDuration)
			}

			req := httptest.NewRequest(http.MethodGet, "/live", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			tt.checkResponse(t, response)
		})
	}
}

func TestHealthHandler_KubernetesStyleEndpoints(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	service := health.NewService("1.0.0", logger)
	service.RegisterChecker(health.NewDummyChecker("postgresql", health.StatusHealthy))
	service.RegisterChecker(health.NewDummyChecker("redis", health.StatusHealthy))

	router, handler := setupHealthTestRouter(service)

	// Register both standard and Kubernetes-style endpoints
	router.GET("/health", handler.HealthCheck)
	router.GET("/healthz", handler.HealthCheck)
	router.GET("/ready", handler.ReadinessCheck)
	router.GET("/readyz", handler.ReadinessCheck)
	router.GET("/live", handler.LivenessCheck)
	router.GET("/livez", handler.LivenessCheck)

	endpoints := []struct {
		path           string
		expectedStatus int
	}{
		{"/health", http.StatusOK},
		{"/healthz", http.StatusOK},
		{"/ready", http.StatusOK},
		{"/readyz", http.StatusOK},
		{"/live", http.StatusOK},
		{"/livez", http.StatusOK},
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, endpoint.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, endpoint.expectedStatus, w.Code)
		})
	}
}

func TestHealthHandler_BuildInfoInResponses(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	service := health.NewService("1.0.0", logger)
	service.RegisterChecker(health.NewDummyChecker("postgresql", health.StatusHealthy))
	service.RegisterChecker(health.NewDummyChecker("redis", health.StatusHealthy))

	router, handler := setupHealthTestRouter(service)
	router.GET("/health", handler.HealthCheck)
	router.GET("/ready", handler.ReadinessCheck)
	router.GET("/live", handler.LivenessCheck)

	tests := []struct {
		path         string
		checkVersion bool
		checkCommit  bool
		checkBranch  bool
	}{
		{"/health", true, true, false},
		{"/ready", true, true, true},
		{"/live", true, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			if tt.checkVersion {
				assert.NotEmpty(t, response["version"])
			}
			if tt.checkCommit {
				assert.NotEmpty(t, response["git_commit"])
			}
			if tt.checkBranch {
				assert.NotEmpty(t, response["git_branch"])
			}
		})
	}
}

func BenchmarkHealthHandler_HealthCheck(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	service := health.NewService("1.0.0", logger)
	service.RegisterChecker(health.NewDummyChecker("test", health.StatusHealthy))

	router, handler := setupHealthTestRouter(service)
	router.GET("/health", handler.HealthCheck)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

func BenchmarkHealthHandler_ReadinessCheck(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	service := health.NewService("1.0.0", logger)
	service.RegisterChecker(health.NewDummyChecker("postgresql", health.StatusHealthy))
	service.RegisterChecker(health.NewDummyChecker("redis", health.StatusHealthy))

	router, handler := setupHealthTestRouter(service)
	router.GET("/ready", handler.ReadinessCheck)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/ready", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}
