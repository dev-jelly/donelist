package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/dev-jelly/donelist/internal/auth"
	"github.com/dev-jelly/donelist/internal/ratelimit"
	"github.com/dev-jelly/donelist/internal/user"
	"github.com/gin-gonic/gin"
)

// RateLimitConfig holds configuration for rate limit middleware
type RateLimitConfig struct {
	RateLimitService *ratelimit.Service
	UserService      *user.Service
	SkipPaths        []string // Paths to skip rate limiting
	BypassHeader     string   // Header for internal service bypass
	BypassToken      string   // Token for internal service bypass
}

// calculateBackoff calculates suggested backoff time using exponential strategy
func calculateBackoff(remaining int, retryAfter float64) int {
	// If we have no remaining requests, suggest exponential backoff
	if remaining <= 0 {
		// Start with 1 second, double up to retry_after time
		backoff := 1.0
		for backoff < retryAfter && backoff < 60 {
			backoff *= 2
		}
		if backoff > retryAfter {
			backoff = retryAfter
		}
		return int(backoff)
	}
	// If close to limit, suggest waiting
	return int(retryAfter)
}

// RateLimitMiddleware creates a rate limiting middleware
func RateLimitMiddleware(cfg RateLimitConfig) gin.HandlerFunc {
	// Build skip paths map for faster lookup
	skipPaths := make(map[string]bool)
	for _, path := range cfg.SkipPaths {
		skipPaths[path] = true
	}

	return func(c *gin.Context) {
		// Check if path should be skipped
		if skipPaths[c.Request.URL.Path] {
			c.Next()
			return
		}

		// Check for internal service bypass
		if cfg.BypassHeader != "" && cfg.BypassToken != "" {
			if c.GetHeader(cfg.BypassHeader) == cfg.BypassToken {
				c.Next()
				return
			}
		}

		// Get user info from context (set by auth middleware)
		var userID, tier string
		var isAuthenticated bool

		if claims, exists := c.Get("claims"); exists {
			if jwtClaims, ok := claims.(*auth.Claims); ok {
				userID = jwtClaims.UserID.String()
				isAuthenticated = true

				// Get user tier from service
				if cfg.UserService != nil {
					user, err := cfg.UserService.GetByID(c.Request.Context(), jwtClaims.UserID)
					if err == nil && user != nil {
						tier = user.Tier
					}
				}
			}
		}

		// Get endpoint path
		endpoint := c.Request.URL.Path

		// Check rate limit
		var allowed bool
		var remaining int
		var resetTime time.Time
		var err error

		ctx := c.Request.Context()

		if isAuthenticated {
			// Check burst first
			burstAllowed, burstErr := cfg.RateLimitService.CheckBurst(ctx, userID, endpoint)
			if burstErr != nil {
				fmt.Printf("Burst check error: %v\n", burstErr)
			} else if !burstAllowed {
				// Burst limit exceeded
				c.Header("X-RateLimit-Burst-Exceeded", "true")
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error":   "Burst limit exceeded",
					"message": "Too many requests in a short period. Please slow down.",
				})
				c.Abort()
				return
			}

			// Check normal limit for authenticated user
			allowed, remaining, resetTime, err = cfg.RateLimitService.CheckLimit(ctx, userID, endpoint, tier)
		} else {
			// Check limit for IP address (anonymous)
			ip := c.ClientIP()
			allowed, remaining, resetTime, err = cfg.RateLimitService.CheckLimitForIP(ctx, ip, endpoint)
		}

		// Handle errors
		if err != nil {
			// Log error but allow request (fail open)
			fmt.Printf("Rate limit check error: %v\n", err)
			c.Next()
			return
		}

		// Add standard rate limit headers
		c.Header("X-RateLimit-Limit", strconv.FormatInt(int64(remaining)+1, 10))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
		c.Header("X-RateLimit-Reset", strconv.FormatInt(resetTime.Unix(), 10))

		// Add burst info if authenticated
		if isAuthenticated {
			burstCurrent, burstLimit, burstErr := cfg.RateLimitService.GetBurstInfo(ctx, userID, endpoint)
			if burstErr == nil && burstLimit > 0 {
				c.Header("X-RateLimit-Burst-Limit", strconv.FormatInt(burstLimit, 10))
				c.Header("X-RateLimit-Burst-Remaining", strconv.FormatInt(burstLimit-burstCurrent, 10))
			}
		}

		// Check if request is allowed
		if !allowed {
			// Calculate retry after
			retryAfter := time.Until(resetTime).Seconds()
			if retryAfter < 1 {
				retryAfter = 1
			}

			c.Header("Retry-After", strconv.Itoa(int(retryAfter)))

			// Calculate progressive backoff suggestion
			backoffSeconds := calculateBackoff(remaining, retryAfter)

			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":   "Rate limit exceeded",
				"message": "Too many requests. Please implement exponential backoff.",
				"details": gin.H{
					"retry_after_seconds": int(retryAfter),
					"reset_time":          resetTime.Unix(),
					"reset_time_iso":      resetTime.Format(time.RFC3339),
					"suggested_backoff": gin.H{
						"next_retry_seconds": backoffSeconds,
						"strategy":           "exponential",
						"guidance":           "Implement exponential backoff: wait 1s, then 2s, then 4s, etc.",
					},
				},
			})
			c.Abort()
			return
		}

		// Continue to next handler
		c.Next()
	}
}

// RateLimitByKeyMiddleware creates a custom rate limiter based on a key function
func RateLimitByKeyMiddleware(service *ratelimit.Service, keyFunc func(*gin.Context) string, tier string) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := keyFunc(c)
		if key == "" {
			c.Next()
			return
		}

		endpoint := c.Request.URL.Path
		ctx := c.Request.Context()

		// Check rate limit
		allowed, remaining, resetTime, err := service.CheckLimit(ctx, key, endpoint, tier)
		if err != nil {
			// Log error but allow request (fail open)
			fmt.Printf("Rate limit check error: %v\n", err)
			c.Next()
			return
		}

		// Add rate limit headers
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
		c.Header("X-RateLimit-Reset", strconv.FormatInt(resetTime.Unix(), 10))

		if !allowed {
			retryAfter := time.Until(resetTime).Seconds()
			if retryAfter < 1 {
				retryAfter = 1
			}

			c.Header("Retry-After", strconv.Itoa(int(retryAfter)))
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "Rate limit exceeded",
				"retry_after": int(retryAfter),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// APIKeyRateLimitMiddleware creates rate limiting based on API key
func APIKeyRateLimitMiddleware(service *ratelimit.Service) gin.HandlerFunc {
	return RateLimitByKeyMiddleware(service, func(c *gin.Context) string {
		// Extract API key from header
		apiKey := c.GetHeader("X-API-Key")
		if apiKey != "" {
			return "api:" + apiKey
		}
		// Fall back to IP for requests without API key
		return "ip:" + c.ClientIP()
	}, "api")
}

// AdminRateLimitBypass bypasses rate limiting for admin users
func AdminRateLimitBypass() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if user is admin
		if claims, exists := c.Get("claims"); exists {
			if jwtClaims, ok := claims.(*auth.Claims); ok {
				_ = jwtClaims // Role field not available
				// Role field not available in Claims
				if false {
					// Set a flag to bypass rate limiting
					c.Set("bypass_rate_limit", true)
				}
			}
		}
		c.Next()
	}
}

// GetRateLimitInfo returns current rate limit information for a user
func GetRateLimitInfo(service *ratelimit.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.Param("user_id")
		if userID == "" {
			// Get from auth context
			if claims, exists := c.Get("claims"); exists {
				if jwtClaims, ok := claims.(*auth.Claims); ok {
					userID = jwtClaims.UserID.String()
				}
			}
		}

		if userID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "User ID required"})
			return
		}

		endpoint := c.Query("endpoint")
		tier := c.Query("tier")
		if tier == "" {
			tier = "free"
		}

		ctx := context.Background()
		info, err := service.GetLimitInfo(ctx, userID, endpoint, tier)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"limit":     info.Limit,
			"remaining": info.Remaining,
			"reset":     info.Reset,
		})
	}
}

// ResetRateLimitHandler creates a handler to reset rate limits (admin only)
func ResetRateLimitHandler(service *ratelimit.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check admin permission
		if claims, exists := c.Get("claims"); exists {
			if jwtClaims, ok := claims.(*auth.Claims); ok {
				_ = jwtClaims // Role field not available
				// Role field not available in Claims
				if true {
					c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
					return
				}
			}
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
			return
		}

		userID := c.Param("user_id")
		endpoint := c.Query("endpoint")

		ctx := context.Background()

		if endpoint != "" {
			err := service.ResetLimit(ctx, userID, endpoint)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		} else {
			err := service.ResetAllLimits(ctx, userID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Rate limits reset successfully",
			"user_id": userID,
		})
	}
}