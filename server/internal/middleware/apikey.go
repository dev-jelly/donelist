package middleware

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dev-jelly/donelist/internal/apikey"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	// APIKeyHeader is the header name for API key authentication
	APIKeyHeader = "X-API-Key"

	// ContextKeyAPIKey is the context key for storing API key info
	ContextKeyAPIKey = "api_key"

	// ContextKeyAPIKeyID is the context key for storing API key ID
	ContextKeyAPIKeyID = "api_key_id"
)

// APIKeyMiddleware creates middleware for API key authentication
func APIKeyMiddleware(service *apikey.Service, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()

		// Extract API key from header
		key := c.GetHeader(APIKeyHeader)
		if key == "" {
			logger.Debug("Missing API key header")
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "API key required",
			})
			c.Abort()
			return
		}

		// Validate API key
		apiKey, err := service.ValidateAPIKey(c.Request.Context(), key)
		if err != nil {
			logger.Warn("Invalid API key",
				zap.Error(err),
				zap.String("key_prefix", extractKeyPrefix(key)),
			)
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid or expired API key",
			})
			c.Abort()
			return
		}

		// Check rate limit
		rateLimitInfo, err := service.CheckRateLimit(c.Request.Context(), apiKey.ID)
		if err != nil {
			if err == apikey.ErrRateLimitExceeded {
				// Set rate limit headers
				c.Header("X-RateLimit-Limit-Hour", strconv.Itoa(rateLimitInfo.HourlyLimit))
				c.Header("X-RateLimit-Remaining-Hour", strconv.Itoa(rateLimitInfo.HourlyRemaining))
				c.Header("X-RateLimit-Limit-Day", strconv.Itoa(rateLimitInfo.DailyLimit))
				c.Header("X-RateLimit-Remaining-Day", strconv.Itoa(rateLimitInfo.DailyRemaining))
				c.Header("X-RateLimit-Reset", rateLimitInfo.ResetAt.Format(time.RFC3339))

				c.JSON(http.StatusTooManyRequests, gin.H{
					"error":      "Rate limit exceeded",
					"reset_at":   rateLimitInfo.ResetAt,
					"limit_hour": rateLimitInfo.HourlyLimit,
					"limit_day":  rateLimitInfo.DailyLimit,
				})
				c.Abort()
				return
			}

			logger.Error("Failed to check rate limit",
				zap.Error(err),
				zap.String("api_key_id", apiKey.ID.String()),
			)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Internal server error",
			})
			c.Abort()
			return
		}

		// Set rate limit headers
		c.Header("X-RateLimit-Limit-Hour", strconv.Itoa(rateLimitInfo.HourlyLimit))
		c.Header("X-RateLimit-Remaining-Hour", strconv.Itoa(rateLimitInfo.HourlyRemaining))
		c.Header("X-RateLimit-Limit-Day", strconv.Itoa(rateLimitInfo.DailyLimit))
		c.Header("X-RateLimit-Remaining-Day", strconv.Itoa(rateLimitInfo.DailyRemaining))
		c.Header("X-RateLimit-Reset", rateLimitInfo.ResetAt.Format(time.RFC3339))

		// Store API key and user info in context
		c.Set(ContextKeyAPIKey, apiKey)
		c.Set(ContextKeyAPIKeyID, apiKey.ID)
		c.Set("user_id", apiKey.UserID)

		// Process request
		c.Next()

		// Record usage after request completes
		go recordAPIKeyUsage(service, apiKey.ID, c, startTime, logger)
	}
}

// RequireScope creates middleware that requires specific API key scopes
func RequireScope(scopes ...apikey.Scope) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get API key from context
		apiKeyVal, exists := c.Get(ContextKeyAPIKey)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "API key authentication required",
			})
			c.Abort()
			return
		}

		apiKey, ok := apiKeyVal.(*apikey.APIKey)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Invalid API key context",
			})
			c.Abort()
			return
		}

		// Check if API key has any of the required scopes
		if !apiKey.HasAnyScope(scopes...) {
			c.JSON(http.StatusForbidden, gin.H{
				"error":          "Insufficient permissions",
				"required_scope": scopes,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// GetAPIKeyFromContext retrieves the API key from the context
func GetAPIKeyFromContext(c *gin.Context) (*apikey.APIKey, bool) {
	apiKeyVal, exists := c.Get(ContextKeyAPIKey)
	if !exists {
		return nil, false
	}

	apiKey, ok := apiKeyVal.(*apikey.APIKey)
	return apiKey, ok
}

// GetAPIKeyIDFromContext retrieves the API key ID from the context
func GetAPIKeyIDFromContext(c *gin.Context) (uuid.UUID, bool) {
	idVal, exists := c.Get(ContextKeyAPIKeyID)
	if !exists {
		return uuid.Nil, false
	}

	id, ok := idVal.(uuid.UUID)
	return id, ok
}

// recordAPIKeyUsage records API key usage asynchronously
func recordAPIKeyUsage(service *apikey.Service, apiKeyID uuid.UUID, c *gin.Context, startTime time.Time, logger *zap.Logger) {
	ctx := context.Background()

	duration := time.Since(startTime)
	durationMs := int(duration.Milliseconds())

	ipAddress := c.ClientIP()
	userAgent := c.Request.UserAgent()

	usage := apikey.APIKeyUsage{
		APIKeyID:       apiKeyID,
		Endpoint:       c.Request.URL.Path,
		Method:         c.Request.Method,
		StatusCode:     c.Writer.Status(),
		ResponseTimeMs: &durationMs,
		IPAddress:      &ipAddress,
		UserAgent:      &userAgent,
	}

	if err := service.RecordUsage(ctx, usage); err != nil {
		logger.Error("Failed to record API key usage",
			zap.Error(err),
			zap.String("api_key_id", apiKeyID.String()),
		)
	}
}

// extractKeyPrefix extracts a safe prefix from the API key for logging
func extractKeyPrefix(key string) string {
	parts := strings.Split(key, "_")
	if len(parts) < 2 {
		return "invalid"
	}
	// Return prefix and first 4 characters of random part
	randomPart := parts[1]
	if len(randomPart) > 4 {
		randomPart = randomPart[:4]
	}
	return parts[0] + "_" + randomPart + "..."
}
