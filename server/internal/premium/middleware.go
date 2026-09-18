package premium

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dev-jelly/donelist/internal/auth"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Middleware provides premium feature middleware
type Middleware struct {
	service *Service
	logger  *zap.Logger
}

// NewMiddleware creates a new premium middleware
func NewMiddleware(service *Service, logger *zap.Logger) *Middleware {
	return &Middleware{
		service: service,
		logger:  logger,
	}
}

// RequireTier creates middleware that requires a minimum tier
func (m *Middleware) RequireTier(minTier Tier) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get user from context (set by auth middleware)
		claims, exists := c.Get("claims")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authentication required",
				"code":  "AUTH_REQUIRED",
			})
			c.Abort()
			return
		}

		jwtClaims, ok := claims.(*auth.Claims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid authentication",
				"code":  "INVALID_AUTH",
			})
			c.Abort()
			return
		}

		userID := jwtClaims.UserID

		// Check tier access
		err := m.service.CheckTierAccess(c.Request.Context(), userID, minTier)
		if err != nil {
			if err == ErrInsufficientTier {
				// Get user's current tier for response
				info, _ := m.service.GetUserTierInfo(c.Request.Context(), userID)
				c.JSON(http.StatusForbidden, gin.H{
					"error":         "Insufficient subscription tier",
					"code":          "INSUFFICIENT_TIER",
					"required_tier": string(minTier),
					"current_tier":  string(info.Tier),
					"upgrade_url":   "/api/v1/subscription/upgrade",
					"upgrade_options": GetUpgradeOptions(info.Tier),
				})
			} else if err == ErrSubscriptionExpired {
				c.JSON(http.StatusForbidden, gin.H{
					"error":       "Subscription expired",
					"code":        "SUBSCRIPTION_EXPIRED",
					"renew_url":   "/api/v1/subscription/renew",
				})
			} else {
				m.logger.Error("Failed to check tier access",
					zap.Error(err),
					zap.String("user_id", userID.String()),
					zap.String("required_tier", string(minTier)),
				)
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": "Failed to verify subscription",
					"code":  "SUBSCRIPTION_CHECK_FAILED",
				})
			}
			c.Abort()
			return
		}

		// Add tier info to context for use in handlers
		if info, err := m.service.GetUserTierInfo(c.Request.Context(), userID); err == nil {
			c.Set("tier_info", info)
		}

		c.Next()
	}
}

// RequireFeature creates middleware that requires a specific feature
func (m *Middleware) RequireFeature(feature Feature) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get user from context
		claims, exists := c.Get("claims")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authentication required",
				"code":  "AUTH_REQUIRED",
			})
			c.Abort()
			return
		}

		jwtClaims, ok := claims.(*auth.Claims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid authentication",
				"code":  "INVALID_AUTH",
			})
			c.Abort()
			return
		}

		userID := jwtClaims.UserID

		// Check feature access
		err := m.service.CheckFeatureAccess(c.Request.Context(), userID, feature)
		if err != nil {
			if err == ErrFeatureNotAvailable {
				// Find minimum tier for this feature
				minTier := m.getMinimumTierForFeature(feature)
				c.JSON(http.StatusForbidden, gin.H{
					"error":         "Feature not available",
					"code":          "FEATURE_NOT_AVAILABLE",
					"feature":       string(feature),
					"description":   GetFeatureDescription(feature),
					"required_tier": string(minTier),
					"upgrade_url":   "/api/v1/subscription/upgrade",
				})
			} else if err == ErrSubscriptionExpired {
				c.JSON(http.StatusForbidden, gin.H{
					"error":     "Subscription expired",
					"code":      "SUBSCRIPTION_EXPIRED",
					"renew_url": "/api/v1/subscription/renew",
				})
			} else {
				m.logger.Error("Failed to check feature access",
					zap.Error(err),
					zap.String("user_id", userID.String()),
					zap.String("feature", string(feature)),
				)
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": "Failed to verify feature access",
					"code":  "FEATURE_CHECK_FAILED",
				})
			}
			c.Abort()
			return
		}

		c.Next()
	}
}

// CheckLimit creates middleware that checks usage limits
func (m *Middleware) CheckLimit(limitType string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get user from context
		claims, exists := c.Get("claims")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authentication required",
				"code":  "AUTH_REQUIRED",
			})
			c.Abort()
			return
		}

		jwtClaims, ok := claims.(*auth.Claims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid authentication",
				"code":  "INVALID_AUTH",
			})
			c.Abort()
			return
		}

		userID := jwtClaims.UserID

		// Check usage limit
		usageInfo, err := m.service.CheckUsageLimit(c.Request.Context(), userID, limitType)
		if err != nil {
			if err == ErrUsageLimitExceeded {
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error":       "Usage limit exceeded",
					"code":        "USAGE_LIMIT_EXCEEDED",
					"limit_type":  limitType,
					"current":     usageInfo.Current,
					"limit":       usageInfo.Limit,
					"reset_at":    usageInfo.ResetAt,
					"upgrade_url": "/api/v1/subscription/upgrade",
				})
			} else {
				m.logger.Error("Failed to check usage limit",
					zap.Error(err),
					zap.String("user_id", userID.String()),
					zap.String("limit_type", limitType),
				)
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": "Failed to check usage limit",
					"code":  "LIMIT_CHECK_FAILED",
				})
			}
			c.Abort()
			return
		}

		// Add usage info to response headers
		c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", usageInfo.Limit))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", usageInfo.Limit-usageInfo.Current))
		if usageInfo.ResetAt != nil {
			c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", usageInfo.ResetAt.Unix()))
		}

		// Store usage info in context for logging
		c.Set("usage_info", usageInfo)

		c.Next()
	}
}

// IncrementUsage creates middleware that increments usage after successful request
func (m *Middleware) IncrementUsage(limitType string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Process request first
		c.Next()

		// Only increment on successful requests
		if c.Writer.Status() >= 200 && c.Writer.Status() < 300 {
			claims, exists := c.Get("claims")
			if exists {
				if jwtClaims, ok := claims.(*auth.Claims); ok {
					userID := jwtClaims.UserID
					// Increment usage asynchronously
					go func() {
						if err := m.service.IncrementUsage(c.Request.Context(), userID, limitType); err != nil {
							m.logger.Warn("Failed to increment usage",
								zap.Error(err),
								zap.String("user_id", userID.String()),
								zap.String("limit_type", limitType),
							)
						}
					}()
				}
			}
		}
	}
}

// InjectTierInfo injects tier information into response headers
func (m *Middleware) InjectTierInfo() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Process request first
		c.Next()

		// Only inject for authenticated requests
		claims, exists := c.Get("claims")
		if exists {
			if jwtClaims, ok := claims.(*auth.Claims); ok {
				userID := jwtClaims.UserID
				// Get tier info
				if info, err := m.service.GetUserTierInfo(c.Request.Context(), userID); err == nil {
					// Add tier info to headers
					c.Header("X-User-Tier", string(info.Tier))
					if info.ExpiresAt != nil {
						c.Header("X-Tier-Expires", info.ExpiresAt.Format(time.RFC3339))
					}
					if info.IsInGrace {
						c.Header("X-Grace-Period", "true")
						if info.GraceEndsAt != nil {
							c.Header("X-Grace-Ends", info.GraceEndsAt.Format(time.RFC3339))
						}
					}

					// Add available features
					features := make([]string, len(info.Features))
					for i, f := range info.Features {
						features[i] = string(f)
					}
					if len(features) > 0 {
						c.Header("X-Available-Features", strings.Join(features, ","))
					}
				}
			}
		}
	}
}

// getMinimumTierForFeature finds the minimum tier required for a feature
func (m *Middleware) getMinimumTierForFeature(feature Feature) Tier {
	tiers := DefaultTiers()

	// Check each tier in order
	tierOrder := []Tier{TierFree, TierPremium, TierEnterprise}
	for _, tier := range tierOrder {
		if config, exists := tiers[tier]; exists {
			for _, f := range config.Features {
				if f == feature {
					return tier
				}
			}
		}
	}

	// Default to enterprise if not found
	return TierEnterprise
}

// PremiumOnly is a convenience middleware for premium tier
func (m *Middleware) PremiumOnly() gin.HandlerFunc {
	return m.RequireTier(TierPremium)
}

// EnterpriseOnly is a convenience middleware for enterprise tier
func (m *Middleware) EnterpriseOnly() gin.HandlerFunc {
	return m.RequireTier(TierEnterprise)
}

// WithGracePeriod wraps a middleware to allow grace period access
func (m *Middleware) WithGracePeriod(next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if user is in grace period
		if tierInfo, exists := c.Get("tier_info"); exists {
			if info, ok := tierInfo.(*TierInfo); ok && info.IsInGrace {
				// Log grace period access
				m.logger.Info("Grace period access",
					zap.String("path", c.Request.URL.Path),
					zap.String("method", c.Request.Method),
					zap.Time("grace_ends", *info.GraceEndsAt),
				)

				// Add warning header
				c.Header("X-Grace-Warning", "Your subscription has expired. Please renew to continue using premium features.")
			}
		}

		next(c)
	}
}