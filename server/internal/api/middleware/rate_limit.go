package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// RateLimiter provides Redis-based sliding window rate limiting
type RateLimiter struct {
	redis  *redis.Client
	logger *zap.Logger
	limits map[string]RateLimit
}

// RateLimit defines rate limiting parameters
type RateLimit struct {
	Requests int           // Maximum requests allowed
	Window   time.Duration // Time window
}

// NewRateLimiter creates a new rate limiter with predefined limits
func NewRateLimiter(redis *redis.Client, logger *zap.Logger) *RateLimiter {
	return &RateLimiter{
		redis:  redis,
		logger: logger,
		limits: map[string]RateLimit{
			// Authentication endpoints - strict limits to prevent brute force
			"auth:login":    {Requests: 5, Window: 15 * time.Minute},   // 5 login attempts per 15 min
			"auth:register": {Requests: 3, Window: time.Hour},           // 3 registrations per hour
			"auth:refresh":  {Requests: 10, Window: time.Hour},          // 10 refresh attempts per hour

			// API endpoints - more permissive
			"api:read":   {Requests: 100, Window: time.Minute}, // 100 reads per minute
			"api:write":  {Requests: 30, Window: time.Minute},  // 30 writes per minute
			"api:delete": {Requests: 10, Window: time.Minute},  // 10 deletes per minute

			// Default limit
			"default": {Requests: 60, Window: time.Minute}, // 60 requests per minute
		},
	}
}

// RateLimitMiddleware creates a rate limiting middleware for a specific limit key
func (rl *RateLimiter) RateLimitMiddleware(limitKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get identifier (IP + User-Agent for anonymous, user ID for authenticated)
		identifier := rl.getIdentifier(c)

		// Get rate limit config
		limit, ok := rl.limits[limitKey]
		if !ok {
			limit = rl.limits["default"]
		}

		// Check rate limit using sliding window
		allowed, remaining, resetTime, err := rl.checkLimit(c.Request.Context(), limitKey, identifier, limit)

		if err != nil {
			rl.logger.Error("Rate limit check failed", zap.Error(err))
			// Fail open to avoid blocking legitimate users on Redis failure
			c.Next()
			return
		}

		// Set rate limit headers (informative for clients)
		c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", limit.Requests))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", resetTime.Unix()))

		if !allowed {
			rl.logger.Warn("Rate limit exceeded",
				zap.String("identifier", identifier),
				zap.String("limit_key", limitKey),
				zap.String("path", c.Request.URL.Path),
				zap.Int("limit", limit.Requests),
				zap.Duration("window", limit.Window),
			)

			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "rate limit exceeded",
				"retry_after": int(resetTime.Sub(time.Now()).Seconds()),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// getIdentifier returns a unique identifier for rate limiting
func (rl *RateLimiter) getIdentifier(c *gin.Context) string {
	// Check if user is authenticated
	if userID, exists := c.Get("user_id"); exists {
		return fmt.Sprintf("user:%v", userID)
	}

	// Use IP + User-Agent for anonymous requests
	ip := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")

	// Hash user agent to keep identifier shorter
	return fmt.Sprintf("ip:%s:ua:%s", ip, hashString(userAgent))
}

// checkLimit checks if request is within rate limit using sliding window algorithm
func (rl *RateLimiter) checkLimit(ctx context.Context, limitKey, identifier string, limit RateLimit) (allowed bool, remaining int, resetTime time.Time, err error) {
	key := fmt.Sprintf("ratelimit:%s:%s", limitKey, identifier)
	now := time.Now()
	windowStart := now.Add(-limit.Window)

	// Use Redis pipeline for atomic operations
	pipe := rl.redis.Pipeline()

	// Remove old entries outside window
	pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart.UnixNano()))

	// Count requests in current window
	countCmd := pipe.ZCard(ctx, key)

	// Add current request timestamp
	pipe.ZAdd(ctx, key, redis.Z{
		Score:  float64(now.UnixNano()),
		Member: fmt.Sprintf("%d", now.UnixNano()),
	})

	// Set expiration to window duration + buffer
	pipe.Expire(ctx, key, limit.Window+time.Minute)

	// Execute pipeline
	_, err = pipe.Exec(ctx)
	if err != nil {
		return false, 0, time.Time{}, err
	}

	// Get count before adding current request
	count := int(countCmd.Val())

	// Calculate remaining requests
	remaining = limit.Requests - count - 1
	if remaining < 0 {
		remaining = 0
	}

	// Calculate reset time (end of current window)
	resetTime = now.Add(limit.Window)

	// Check if under limit (count is BEFORE adding current request)
	allowed = count < limit.Requests

	return allowed, remaining, resetTime, nil
}

// hashString creates a simple hash of a string for shorter identifiers
func hashString(s string) string {
	if len(s) <= 10 {
		return s
	}
	// Simple hash: take first 5 and last 5 characters
	return s[:5] + "..." + s[len(s)-5:]
}

// SetCustomLimit allows setting custom rate limits dynamically
func (rl *RateLimiter) SetCustomLimit(key string, requests int, window time.Duration) {
	rl.limits[key] = RateLimit{
		Requests: requests,
		Window:   window,
	}
}

// GetLimit returns the rate limit for a given key
func (rl *RateLimiter) GetLimit(key string) (RateLimit, bool) {
	limit, ok := rl.limits[key]
	return limit, ok
}
