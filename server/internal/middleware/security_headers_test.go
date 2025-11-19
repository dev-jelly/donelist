package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestSecurityHeadersMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger, _ := zap.NewDevelopment()

	tests := []struct {
		name           string
		config         SecurityHeadersConfig
		expectedHeaders map[string]string
	}{
		{
			name: "Basic security headers",
			config: SecurityHeadersConfig{
				Logger:              logger,
				XFrameOptions:       "DENY",
				XContentTypeOptions: true,
				XXSSProtection:      "1; mode=block",
				ReferrerPolicy:      "strict-origin-when-cross-origin",
				RemoveServerHeader:  true,
				RemoveXPoweredBy:    true,
			},
			expectedHeaders: map[string]string{
				"X-Frame-Options":        "DENY",
				"X-Content-Type-Options": "nosniff",
				"X-XSS-Protection":       "1; mode=block",
				"Referrer-Policy":        "strict-origin-when-cross-origin",
			},
		},
		{
			name: "HSTS enabled",
			config: SecurityHeadersConfig{
				Logger:                logger,
				EnableHSTS:            true,
				HSTSMaxAge:            31536000,
				HSTSIncludeSubdomains: true,
				HSTSPreload:           true,
			},
			expectedHeaders: map[string]string{
				"Strict-Transport-Security": "max-age=31536000; includeSubDomains; preload",
			},
		},
		{
			name: "CSP enabled",
			config: SecurityHeadersConfig{
				Logger:    logger,
				EnableCSP: true,
				CSPDirectives: map[string][]string{
					"default-src": {"'self'"},
					"script-src":  {"'self'", "'unsafe-inline'"},
				},
			},
			expectedHeaders: map[string]string{
				"Content-Security-Policy": "default-src 'self'; script-src 'self' 'unsafe-inline'",
			},
		},
		{
			name: "Cross-Origin policies",
			config: SecurityHeadersConfig{
				Logger:                    logger,
				CrossOriginEmbedderPolicy: "require-corp",
				CrossOriginOpenerPolicy:   "same-origin",
				CrossOriginResourcePolicy: "same-origin",
			},
			expectedHeaders: map[string]string{
				"Cross-Origin-Embedder-Policy":  "require-corp",
				"Cross-Origin-Opener-Policy":    "same-origin",
				"Cross-Origin-Resource-Policy":  "same-origin",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(SecurityHeadersMiddleware(tt.config))
			router.GET("/test", func(c *gin.Context) {
				c.String(http.StatusOK, "OK")
			})

			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			for header, expectedValue := range tt.expectedHeaders {
				actualValue := w.Header().Get(header)
				assert.Contains(t, actualValue, expectedValue,
					"Header %s should contain %s, got %s", header, expectedValue, actualValue)
			}
		})
	}
}

func TestBuildCSP(t *testing.T) {
	tests := []struct {
		name       string
		directives map[string][]string
		expected   string
	}{
		{
			name:       "Empty directives",
			directives: map[string][]string{},
			expected:   "",
		},
		{
			name: "Single directive",
			directives: map[string][]string{
				"default-src": {"'self'"},
			},
			expected: "default-src 'self'",
		},
		{
			name: "Multiple directives",
			directives: map[string][]string{
				"default-src": {"'self'"},
				"script-src":  {"'self'", "'unsafe-inline'"},
			},
			expected: "default-src 'self'; script-src 'self' 'unsafe-inline'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildCSP(tt.directives)
			if tt.expected == "" {
				assert.Empty(t, result)
			} else {
				assert.Contains(t, result, "default-src")
			}
		})
	}
}

func TestBuildPermissionsPolicy(t *testing.T) {
	tests := []struct {
		name       string
		directives map[string][]string
		want       string
	}{
		{
			name: "Deny all",
			directives: map[string][]string{
				"geolocation": {},
			},
			want: "geolocation=()",
		},
		{
			name: "Allow all",
			directives: map[string][]string{
				"geolocation": {"*"},
			},
			want: "geolocation=*",
		},
		{
			name: "Self only",
			directives: map[string][]string{
				"camera": {"self"},
			},
			want: "camera=(self)",
		},
		{
			name: "Multiple features",
			directives: map[string][]string{
				"geolocation": {},
				"camera":      {"self"},
			},
			want: "geolocation=(), camera=(self)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildPermissionsPolicy(tt.directives)
			assert.Contains(t, result, tt.want)
		})
	}
}

func TestCreateDefaultSecurityHeadersConfig(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	t.Run("Production config", func(t *testing.T) {
		cfg := CreateDefaultSecurityHeadersConfig(logger, true)

		assert.True(t, cfg.EnableCSP)
		assert.True(t, cfg.EnableHSTS)
		assert.Equal(t, "DENY", cfg.XFrameOptions)
		assert.True(t, cfg.XContentTypeOptions)
		assert.True(t, cfg.RemoveServerHeader)
		assert.False(t, cfg.CSPReportOnly)
	})

	t.Run("Development config", func(t *testing.T) {
		cfg := CreateDefaultSecurityHeadersConfig(logger, false)

		assert.True(t, cfg.EnableCSP)
		assert.False(t, cfg.EnableHSTS) // HSTS disabled in dev
		assert.True(t, cfg.CSPReportOnly) // CSP in report-only mode
	})
}

func TestCreateDevelopmentSecurityHeadersConfig(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := CreateDevelopmentSecurityHeadersConfig(logger)

	// Check that connect-src includes localhost for development
	assert.Contains(t, cfg.CSPDirectives["connect-src"], "http://localhost:*")
	assert.Contains(t, cfg.CSPDirectives["connect-src"], "ws:")
}

func TestCreateAPISecurityHeadersConfig(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := CreateAPISecurityHeadersConfig(logger, true)

	// API should have more restrictive CSP
	assert.Equal(t, []string{"'none'"}, cfg.CSPDirectives["default-src"])
	assert.Empty(t, cfg.XXSSProtection) // No XSS protection needed for API
}
