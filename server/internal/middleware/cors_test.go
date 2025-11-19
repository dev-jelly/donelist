package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dev-jelly/donelist/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestCORSMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger, _ := zap.NewDevelopment()

	tests := []struct {
		name               string
		corsConfig         config.CORSConfig
		origin             string
		method             string
		expectedStatus     int
		expectOriginHeader bool
	}{
		{
			name: "Allowed origin - exact match",
			corsConfig: config.CORSConfig{
				AllowedOrigins:   []string{"http://localhost:3000"},
				AllowedMethods:   []string{"GET", "POST"},
				AllowedHeaders:   []string{"Content-Type"},
				AllowCredentials: true,
			},
			origin:             "http://localhost:3000",
			method:             "GET",
			expectedStatus:     http.StatusOK,
			expectOriginHeader: true,
		},
		{
			name: "Disallowed origin",
			corsConfig: config.CORSConfig{
				AllowedOrigins: []string{"http://localhost:3000"},
			},
			origin:             "http://evil.com",
			method:             "GET",
			expectedStatus:     http.StatusForbidden,
			expectOriginHeader: false,
		},
		{
			name: "Preflight request - allowed",
			corsConfig: config.CORSConfig{
				AllowedOrigins:   []string{"http://localhost:3000"},
				AllowedMethods:   []string{"GET", "POST", "DELETE"},
				AllowedHeaders:   []string{"Content-Type", "Authorization"},
				AllowCredentials: true,
				MaxAge:           300,
			},
			origin:             "http://localhost:3000",
			method:             "OPTIONS",
			expectedStatus:     http.StatusNoContent,
			expectOriginHeader: true,
		},
		{
			name: "Wildcard subdomain match",
			corsConfig: config.CORSConfig{
				AllowedOrigins: []string{"https://*.example.com"},
			},
			origin:             "https://app.example.com",
			method:             "GET",
			expectedStatus:     http.StatusOK,
			expectOriginHeader: true,
		},
		{
			name: "Port wildcard match",
			corsConfig: config.CORSConfig{
				AllowedOrigins: []string{"http://localhost:*"},
			},
			origin:             "http://localhost:5173",
			method:             "GET",
			expectedStatus:     http.StatusOK,
			expectOriginHeader: true,
		},
		{
			name: "No origin header - not a CORS request",
			corsConfig: config.CORSConfig{
				AllowedOrigins: []string{"http://localhost:3000"},
			},
			origin:             "",
			method:             "GET",
			expectedStatus:     http.StatusOK,
			expectOriginHeader: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			router := gin.New()
			router.Use(CORSMiddleware(tt.corsConfig, logger))
			router.GET("/test", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "success"})
			})
			router.OPTIONS("/test", func(c *gin.Context) {
				// Preflight requests are handled by middleware
			})

			// Create request
			req, _ := http.NewRequest(tt.method, "/test", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}

			// Record response
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Assert status
			assert.Equal(t, tt.expectedStatus, w.Code)

			// Assert CORS headers
			originHeader := w.Header().Get("Access-Control-Allow-Origin")
			if tt.expectOriginHeader {
				assert.NotEmpty(t, originHeader)
				if tt.origin != "" {
					assert.Equal(t, tt.origin, originHeader)
				}

				// Check credentials header if enabled
				if tt.corsConfig.AllowCredentials {
					assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
				}

				// Check preflight headers
				if tt.method == "OPTIONS" {
					assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Methods"))
					assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Headers"))
				}
			} else {
				assert.Empty(t, originHeader)
			}
		})
	}
}

func TestValidateOrigin(t *testing.T) {
	tests := []struct {
		name           string
		origin         string
		allowedOrigins []string
		expectedResult string
	}{
		{
			name:           "Exact match",
			origin:         "http://localhost:3000",
			allowedOrigins: []string{"http://localhost:3000", "https://example.com"},
			expectedResult: "http://localhost:3000",
		},
		{
			name:           "No match",
			origin:         "http://evil.com",
			allowedOrigins: []string{"http://localhost:3000"},
			expectedResult: "",
		},
		{
			name:           "Wildcard subdomain",
			origin:         "https://app.example.com",
			allowedOrigins: []string{"https://*.example.com"},
			expectedResult: "https://app.example.com",
		},
		{
			name:           "Wildcard port",
			origin:         "http://localhost:5173",
			allowedOrigins: []string{"http://localhost:*"},
			expectedResult: "http://localhost:5173",
		},
		{
			name:           "Trailing slash normalization",
			origin:         "http://localhost:3000/",
			allowedOrigins: []string{"http://localhost:3000"},
			expectedResult: "http://localhost:3000",
		},
		{
			name:           "Universal wildcard",
			origin:         "https://any-site.com",
			allowedOrigins: []string{"*"},
			expectedResult: "https://any-site.com",
		},
		{
			name:           "Empty allowed list",
			origin:         "http://localhost:3000",
			allowedOrigins: []string{},
			expectedResult: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validateOrigin(tt.origin, tt.allowedOrigins)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestMatchWildcard(t *testing.T) {
	tests := []struct {
		name     string
		origin   string
		pattern  string
		expected bool
	}{
		{
			name:     "Universal wildcard",
			origin:   "https://example.com",
			pattern:  "*",
			expected: true,
		},
		{
			name:     "Subdomain wildcard - match",
			origin:   "https://app.example.com",
			pattern:  "https://*.example.com",
			expected: true,
		},
		{
			name:     "Subdomain wildcard - no match",
			origin:   "https://app.other.com",
			pattern:  "https://*.example.com",
			expected: false,
		},
		{
			name:     "Port wildcard - match",
			origin:   "http://localhost:5173",
			pattern:  "http://localhost:*",
			expected: true,
		},
		{
			name:     "Port wildcard - no match",
			origin:   "https://localhost:5173",
			pattern:  "http://localhost:*",
			expected: false,
		},
		{
			name:     "Multiple wildcards - invalid",
			origin:   "https://app.example.com",
			pattern:  "*://*.example.com",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchWildcard(tt.origin, tt.pattern)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCreateStrictCORSConfig(t *testing.T) {
	allowedOrigins := []string{"https://app.example.com", "https://www.example.com"}
	cfg := CreateStrictCORSConfig(allowedOrigins)

	assert.Equal(t, allowedOrigins, cfg.AllowedOrigins)
	assert.Contains(t, cfg.AllowedMethods, "GET")
	assert.Contains(t, cfg.AllowedMethods, "POST")
	assert.Contains(t, cfg.AllowedHeaders, "Authorization")
	assert.True(t, cfg.AllowCredentials)
	assert.Equal(t, 300, cfg.MaxAge)
}

func TestCreateDevelopmentCORSConfig(t *testing.T) {
	cfg := CreateDevelopmentCORSConfig()

	assert.Contains(t, cfg.AllowedOrigins, "http://localhost:*")
	assert.Contains(t, cfg.AllowedMethods, "GET")
	assert.True(t, cfg.AllowCredentials)
	assert.Greater(t, cfg.MaxAge, 0)
}
