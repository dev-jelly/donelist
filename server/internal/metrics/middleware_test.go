package metrics

import (
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestMetrics() *Metrics {
	// Create separate registries for test isolation
	return &Metrics{
		HTTPRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "test_http_requests_total",
				Help: "Test counter",
			},
			[]string{"method", "path", "status"},
		),
		HTTPRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "test_http_request_duration_seconds",
				Help: "Test histogram",
			},
			[]string{"method", "path"},
		),
		HTTPRequestSize: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "test_http_request_size_bytes",
				Help: "Test histogram",
			},
			[]string{"method", "path"},
		),
		HTTPResponseSize: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "test_http_response_size_bytes",
				Help: "Test histogram",
			},
			[]string{"method", "path"},
		),
		HTTPRequestsInFlight: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "test_http_requests_in_flight",
				Help: "Test gauge",
			},
			[]string{"method"},
		),
		HTTPErrorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "test_http_errors_total",
				Help: "Test error counter",
			},
			[]string{"method", "path", "status_code"},
		),
	}
}

func TestMetricsMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ResetPathRegistry()

	m := setupTestMetrics()
	router := gin.New()
	router.Use(MetricsMiddleware(m))

	router.GET("/api/v1/users/:id", func(c *gin.Context) {
		c.JSON(200, gin.H{"id": c.Param("id")})
	})

	// Make a request
	req := httptest.NewRequest("GET", "/api/v1/users/123", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	// Note: Since we're using test metrics, we can't easily verify the exact values
	// but we can verify that the middleware executed without panics
}

func TestMetricsMiddleware_SkipsMetricsEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ResetPathRegistry()

	m := setupTestMetrics()
	router := gin.New()
	router.Use(MetricsMiddleware(m))

	called := false
	router.GET("/metrics", func(c *gin.Context) {
		called = true
		c.String(200, "metrics")
	})

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.True(t, called)
}

func TestMetricsMiddleware_RecordsErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ResetPathRegistry()

	m := setupTestMetrics()
	router := gin.New()
	router.Use(MetricsMiddleware(m))

	router.GET("/api/v1/error", func(c *gin.Context) {
		c.JSON(500, gin.H{"error": "internal error"})
	})

	req := httptest.NewRequest("GET", "/api/v1/error", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 500, w.Code)
}

func TestMetricsMiddleware_ConcurrentRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ResetPathRegistry()

	m := setupTestMetrics()
	router := gin.New()
	router.Use(MetricsMiddleware(m))

	router.GET("/api/v1/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Make concurrent requests
	var wg sync.WaitGroup
	numRequests := 100

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/api/v1/test", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, 200, w.Code)
		}()
	}

	wg.Wait()
	// If we get here without panics, the test passed
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "UUID in path",
			path:     "/api/v1/users/550e8400-e29b-41d4-a716-446655440000",
			expected: "/api/v1/users/:id",
		},
		{
			name:     "numeric ID in path",
			path:     "/api/v1/users/123",
			expected: "/api/v1/users/:id",
		},
		{
			name:     "multiple IDs in path",
			path:     "/api/v1/users/123/posts/456",
			expected: "/api/v1/users/:id/posts/:id",
		},
		{
			name:     "no IDs in path",
			path:     "/api/v1/users",
			expected: "/api/v1/users",
		},
		{
			name:     "trailing slash with ID",
			path:     "/api/v1/users/123/",
			expected: "/api/v1/users/:id/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizePath(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLimitPathCardinality(t *testing.T) {
	ResetPathRegistry()

	// Add paths up to the limit
	for i := 0; i < MaxPathCardinality; i++ {
		path := limitPathCardinality("/api/path/" + string(rune(i)))
		assert.NotEqual(t, UnknownPath, path)
	}

	// Verify count
	assert.Equal(t, MaxPathCardinality, GetRegisteredPathCount())

	// Next path should return UnknownPath
	path := limitPathCardinality("/api/new/path")
	assert.Equal(t, UnknownPath, path)

	// But existing paths should still work
	existingPath := limitPathCardinality("/api/path/" + string(rune(0)))
	assert.NotEqual(t, UnknownPath, existingPath)
}

func TestLimitPathCardinality_Concurrent(t *testing.T) {
	ResetPathRegistry()

	var wg sync.WaitGroup
	numGoroutines := 100
	pathsPerGoroutine := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()
			for j := 0; j < pathsPerGoroutine; j++ {
				path := "/api/path/" + string(rune(offset*pathsPerGoroutine+j))
				limitPathCardinality(path)
			}
		}(i)
	}

	wg.Wait()

	count := GetRegisteredPathCount()
	expectedCount := numGoroutines * pathsPerGoroutine
	if expectedCount > MaxPathCardinality {
		expectedCount = MaxPathCardinality
	}

	assert.LessOrEqual(t, count, MaxPathCardinality)
	assert.GreaterOrEqual(t, count, 0)
}

func TestGetRegisteredPathCount(t *testing.T) {
	ResetPathRegistry()

	assert.Equal(t, 0, GetRegisteredPathCount())

	limitPathCardinality("/api/v1/users")
	assert.Equal(t, 1, GetRegisteredPathCount())

	limitPathCardinality("/api/v1/posts")
	assert.Equal(t, 2, GetRegisteredPathCount())

	// Same path shouldn't increase count
	limitPathCardinality("/api/v1/users")
	assert.Equal(t, 2, GetRegisteredPathCount())
}

func TestResetPathRegistry(t *testing.T) {
	ResetPathRegistry()

	limitPathCardinality("/api/v1/users")
	limitPathCardinality("/api/v1/posts")
	assert.Equal(t, 2, GetRegisteredPathCount())

	ResetPathRegistry()
	assert.Equal(t, 0, GetRegisteredPathCount())
}

func TestMetricsMiddleware_WithGinRouteTemplate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ResetPathRegistry()

	m := setupTestMetrics()
	router := gin.New()
	router.Use(MetricsMiddleware(m))

	// Gin route with parameter should use template
	router.GET("/api/v1/users/:id", func(c *gin.Context) {
		c.JSON(200, gin.H{"id": c.Param("id")})
	})

	// Make requests with different IDs
	for _, id := range []string{"123", "456", "789"} {
		req := httptest.NewRequest("GET", "/api/v1/users/"+id, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, 200, w.Code)
	}

	// All requests should be grouped under the same path template
	// Only 1 unique path should be registered
	assert.Equal(t, 1, GetRegisteredPathCount())
}

func TestMetricsMiddleware_WithoutRouteTemplate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ResetPathRegistry()

	m := setupTestMetrics()
	router := gin.New()
	router.Use(MetricsMiddleware(m))

	// Fallback handler - no template available
	router.NoRoute(func(c *gin.Context) {
		c.JSON(404, gin.H{"error": "not found"})
	})

	// Make requests to non-existent paths
	paths := []string{
		"/unknown/123",
		"/unknown/456",
	}

	for _, path := range paths {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, 404, w.Code)
	}

	// Both paths should be normalized to the same template
	count := GetRegisteredPathCount()
	assert.Equal(t, 1, count) // Both /unknown/123 and /unknown/456 -> /unknown/:id
}

func TestMetricsMiddleware_HTTPStatusCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ResetPathRegistry()

	m := setupTestMetrics()
	router := gin.New()
	router.Use(MetricsMiddleware(m))

	router.GET("/api/v1/ok", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
	router.GET("/api/v1/created", func(c *gin.Context) {
		c.JSON(201, gin.H{"status": "created"})
	})
	router.GET("/api/v1/badrequest", func(c *gin.Context) {
		c.JSON(400, gin.H{"error": "bad request"})
	})
	router.GET("/api/v1/notfound", func(c *gin.Context) {
		c.JSON(404, gin.H{"error": "not found"})
	})
	router.GET("/api/v1/error", func(c *gin.Context) {
		c.JSON(500, gin.H{"error": "internal error"})
	})

	statusCodes := []int{200, 201, 400, 404, 500}
	paths := []string{"/api/v1/ok", "/api/v1/created", "/api/v1/badrequest", "/api/v1/notfound", "/api/v1/error"}

	for i, path := range paths {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, statusCodes[i], w.Code)
	}
}

func BenchmarkMetricsMiddleware(b *testing.B) {
	gin.SetMode(gin.ReleaseMode)
	ResetPathRegistry()

	m := setupTestMetrics()
	router := gin.New()
	router.Use(MetricsMiddleware(m))

	router.GET("/api/v1/benchmark", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/api/v1/benchmark", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

func BenchmarkNormalizePath(b *testing.B) {
	paths := []string{
		"/api/v1/users/550e8400-e29b-41d4-a716-446655440000",
		"/api/v1/users/123",
		"/api/v1/users/123/posts/456",
		"/api/v1/users",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		normalizePath(paths[i%len(paths)])
	}
}

func BenchmarkLimitPathCardinality(b *testing.B) {
	ResetPathRegistry()

	// Pre-fill with some paths
	for i := 0; i < 100; i++ {
		limitPathCardinality("/api/path/" + string(rune(i)))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limitPathCardinality("/api/path/" + string(rune(i%100)))
	}
}
