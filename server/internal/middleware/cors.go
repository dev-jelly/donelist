package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/dev-jelly/donelist/internal/config"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// CORSMiddleware creates a CORS middleware with strict origin validation
func CORSMiddleware(cfg config.CORSConfig, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// If no origin header, it's not a CORS request
		if origin == "" {
			c.Next()
			return
		}

		// Validate origin against whitelist
		allowedOrigin := validateOrigin(origin, cfg.AllowedOrigins)

		if allowedOrigin == "" {
			// Origin not allowed - log and reject
			logger.Warn("CORS request from unauthorized origin",
				zap.String("origin", origin),
				zap.String("path", c.Request.URL.Path),
				zap.String("method", c.Request.Method),
			)

			// Don't set CORS headers for unauthorized origins
			// This prevents the browser from reading the response
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "Origin not allowed",
			})
			return
		}

		// Set CORS headers for allowed origins
		c.Header("Access-Control-Allow-Origin", allowedOrigin)

		// Set credentials header if enabled
		if cfg.AllowCredentials {
			c.Header("Access-Control-Allow-Credentials", "true")
		}

		// Handle preflight requests
		if c.Request.Method == "OPTIONS" {
			// Set allowed methods
			if len(cfg.AllowedMethods) > 0 {
				c.Header("Access-Control-Allow-Methods", strings.Join(cfg.AllowedMethods, ", "))
			}

			// Set allowed headers
			if len(cfg.AllowedHeaders) > 0 {
				c.Header("Access-Control-Allow-Headers", strings.Join(cfg.AllowedHeaders, ", "))
			}

			// Set max age for preflight cache
			if cfg.MaxAge > 0 {
				c.Header("Access-Control-Max-Age", fmt.Sprintf("%d", cfg.MaxAge))
			}

			logger.Debug("CORS preflight request",
				zap.String("origin", origin),
				zap.String("path", c.Request.URL.Path),
			)

			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		// Set exposed headers for actual requests
		if len(cfg.ExposedHeaders) > 0 {
			c.Header("Access-Control-Expose-Headers", strings.Join(cfg.ExposedHeaders, ", "))
		}

		c.Next()
	}
}

// validateOrigin checks if the origin is in the allowed list
// Supports exact matches and wildcard patterns
func validateOrigin(origin string, allowedOrigins []string) string {
	if len(allowedOrigins) == 0 {
		return ""
	}

	// Normalize origin (remove trailing slash)
	origin = strings.TrimSuffix(origin, "/")

	for _, allowed := range allowedOrigins {
		allowed = strings.TrimSuffix(allowed, "/")

		// Check for wildcard match
		if strings.Contains(allowed, "*") {
			if matchWildcard(origin, allowed) {
				return origin
			}
		} else {
			// Exact match
			if origin == allowed {
				return origin
			}
		}
	}

	return ""
}

// matchWildcard checks if origin matches a wildcard pattern
// Supports patterns like:
// - *.example.com (subdomain wildcard)
// - https://*.example.com (subdomain with protocol)
// - http://localhost:* (port wildcard)
func matchWildcard(origin, pattern string) bool {
	// Simple wildcard matching
	if pattern == "*" {
		return true
	}

	// Split by wildcard
	parts := strings.Split(pattern, "*")
	if len(parts) != 2 {
		return false
	}

	prefix := parts[0]
	suffix := parts[1]

	// Check if origin starts with prefix and ends with suffix
	return strings.HasPrefix(origin, prefix) && strings.HasSuffix(origin, suffix)
}

// CreateStrictCORSConfig creates a production-ready CORS configuration
// with strict origin validation
func CreateStrictCORSConfig(allowedOrigins []string) config.CORSConfig {
	return config.CORSConfig{
		AllowedOrigins: allowedOrigins,
		AllowedMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodOptions,
		},
		AllowedHeaders: []string{
			"Accept",
			"Authorization",
			"Content-Type",
			"X-CSRF-Token",
			"X-Request-ID",
		},
		ExposedHeaders: []string{
			"Link",
			"X-RateLimit-Limit",
			"X-RateLimit-Remaining",
			"X-RateLimit-Reset",
		},
		AllowCredentials: true,
		MaxAge:           300, // 5 minutes
	}
}

// CreateDevelopmentCORSConfig creates a more permissive CORS configuration
// for development environments
func CreateDevelopmentCORSConfig() config.CORSConfig {
	return config.CORSConfig{
		AllowedOrigins: []string{
			"http://localhost:*",
			"http://127.0.0.1:*",
			"https://localhost:*",
			"https://127.0.0.1:*",
		},
		AllowedMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodOptions,
			http.MethodHead,
		},
		AllowedHeaders: []string{
			"*", // Allow all headers in development
		},
		ExposedHeaders: []string{
			"*", // Expose all headers in development
		},
		AllowCredentials: true,
		MaxAge:           600, // 10 minutes
	}
}
