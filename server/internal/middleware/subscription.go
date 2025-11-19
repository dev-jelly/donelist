package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/dev-jelly/donelist/internal/premium"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// UserRepository defines the interface for user data access
type UserRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*UserWithTier, error)
}

// UserWithTier represents user data with tier information
type UserWithTier struct {
	ID            uuid.UUID
	Email         string
	Tier          string
	TierExpiresAt *time.Time
}

// SubscriptionMiddleware enriches the context with user's subscription tier information
// This middleware should be used after AuthMiddleware to ensure user_id is available
func SubscriptionMiddleware(userRepo UserRepository, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get user ID from context (set by AuthMiddleware)
		userID, exists := c.Get("user_id")
		if !exists {
			logger.Warn("Subscription middleware: user_id not found in context",
				zap.String("path", c.Request.URL.Path),
			)
			c.Next()
			return
		}

		id, ok := userID.(uuid.UUID)
		if !ok {
			logger.Error("Subscription middleware: invalid user_id type",
				zap.Any("user_id", userID),
			)
			c.Next()
			return
		}

		// Get user with tier information
		user, err := userRepo.GetByID(c.Request.Context(), id)
		if err != nil {
			logger.Error("Failed to get user tier information",
				zap.String("user_id", id.String()),
				zap.Error(err),
			)
			// Don't abort - continue with free tier assumption
			c.Set("user_tier", premium.TierFree)
			c.Next()
			return
		}

		// Parse and validate tier
		tier := premium.Tier(user.Tier)
		if !tier.IsValid() {
			logger.Warn("Invalid user tier, defaulting to free",
				zap.String("user_id", id.String()),
				zap.String("tier", user.Tier),
			)
			tier = premium.TierFree
		}

		// Check tier expiration
		if user.TierExpiresAt != nil && user.TierExpiresAt.Before(time.Now().UTC()) {
			logger.Info("User tier expired, downgrading to free",
				zap.String("user_id", id.String()),
				zap.String("expired_tier", string(tier)),
				zap.Time("expired_at", *user.TierExpiresAt),
			)
			tier = premium.TierFree
		}

		// Set tier in context for downstream handlers
		c.Set("user_tier", tier)
		c.Set("tier_expires_at", user.TierExpiresAt)

		logger.Debug("User tier loaded",
			zap.String("user_id", id.String()),
			zap.String("tier", string(tier)),
		)

		c.Next()
	}
}

// GetUserTier extracts user tier from gin context
// Returns TierFree if tier is not found or invalid
func GetUserTier(c *gin.Context) premium.Tier {
	tierValue, exists := c.Get("user_tier")
	if !exists {
		return premium.TierFree
	}

	tier, ok := tierValue.(premium.Tier)
	if !ok {
		return premium.TierFree
	}

	return tier
}

// GetTierExpiresAt extracts tier expiration time from gin context
func GetTierExpiresAt(c *gin.Context) *time.Time {
	expiresAtValue, exists := c.Get("tier_expires_at")
	if !exists {
		return nil
	}

	expiresAt, ok := expiresAtValue.(*time.Time)
	if !ok {
		return nil
	}

	return expiresAt
}

// RequirePremiumTier is a middleware that requires premium or enterprise tier
func RequirePremiumTier(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		tier := GetUserTier(c)

		if tier == premium.TierFree {
			logger.Warn("Premium feature access denied",
				zap.String("path", c.Request.URL.Path),
				zap.String("tier", string(tier)),
			)

			c.JSON(http.StatusForbidden, gin.H{
				"error":         "premium_required",
				"message":       "This feature requires a premium subscription",
				"current_tier":  string(tier),
				"required_tier": string(premium.TierPremium),
				"upgrade_url":   "/api/v1/subscription/upgrade",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
