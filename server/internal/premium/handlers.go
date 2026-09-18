package premium

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/dev-jelly/donelist/internal/auth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Handler handles subscription management endpoints
type Handler struct {
	service *Service
	logger  *zap.Logger
}

// NewHandler creates a new subscription handler
func NewHandler(service *Service, logger *zap.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  logger,
	}
}

// GetCurrentSubscription gets the current user's subscription info
// @Summary Get current subscription
// @Description Get detailed information about the current user's subscription
// @Tags subscription
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} TierInfo
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/subscription/current [get]
func (h *Handler) GetCurrentSubscription(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
			"code":  "INVALID_USER_ID",
		})
		return
	}

	info, err := h.service.GetUserTierInfo(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get tier info",
			zap.Error(err),
			zap.String("user_id", userID.String()),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get subscription info",
			"code":  "SUBSCRIPTION_INFO_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, info)
}

// GetAvailablePlans gets available subscription plans
// @Summary Get available plans
// @Description Get list of all available subscription plans
// @Tags subscription
// @Accept json
// @Produce json
// @Success 200 {array} map[string]interface{}
// @Router /api/v1/subscription/plans [get]
func (h *Handler) GetAvailablePlans(c *gin.Context) {
	tiers := DefaultTiers()
	plans := make([]map[string]interface{}, 0)

	for tier, config := range tiers {
		// Only show active plans
		if tier == TierEnterprise {
			continue // Skip enterprise for now
		}

		plan := map[string]interface{}{
			"id":          tier,
			"name":        config.Name,
			"description": config.Description,
			"price": map[string]interface{}{
				"monthly": config.Price,
				"yearly":  config.Price * 10, // 2 months free on yearly
			},
			"features": config.Features,
			"limits":   config.Limits,
		}
		plans = append(plans, plan)
	}

	c.JSON(http.StatusOK, gin.H{
		"plans": plans,
	})
}

// UpgradeSubscription upgrades the user's subscription
// @Summary Upgrade subscription
// @Description Upgrade to a higher subscription tier
// @Tags subscription
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body UpgradeRequest true "Upgrade request"
// @Success 200 {object} map[string]interface{} "Upgrade successful"
// @Failure 400 {object} map[string]interface{} "Bad request"
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/subscription/upgrade [post]
func (h *Handler) UpgradeSubscription(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
			"code":  "INVALID_USER_ID",
		})
		return
	}

	var req UpgradeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request",
			"code":  "INVALID_REQUEST",
		})
		return
	}

	// Validate tier
	newTier := Tier(req.Tier)
	if !newTier.IsValid() {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid tier",
			"code":  "INVALID_TIER",
		})
		return
	}

	// Determine duration
	var duration time.Duration
	switch req.BillingPeriod {
	case "monthly":
		duration = 30 * 24 * time.Hour
	case "yearly":
		duration = 365 * 24 * time.Hour
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid billing period",
			"code":  "INVALID_BILLING_PERIOD",
		})
		return
	}

	// Process upgrade
	err = h.service.UpgradeTier(c.Request.Context(), userID, newTier, duration)
	if err != nil {
		h.logger.Error("Failed to upgrade tier",
			zap.Error(err),
			zap.String("user_id", userID.String()),
			zap.String("new_tier", string(newTier)),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to upgrade subscription",
			"code":  "UPGRADE_FAILED",
		})
		return
	}

	// Get updated tier info
	info, _ := h.service.GetUserTierInfo(c.Request.Context(), userID)

	c.JSON(http.StatusOK, gin.H{
		"message":      "Subscription upgraded successfully",
		"subscription": info,
	})
}

// DowngradeSubscription downgrades the user's subscription
// @Summary Downgrade subscription
// @Description Downgrade to a lower subscription tier
// @Tags subscription
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body DowngradeRequest true "Downgrade request"
// @Success 200 {object} map[string]interface{} "Downgrade successful"
// @Failure 400 {object} map[string]interface{} "Bad request"
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/subscription/downgrade [post]
func (h *Handler) DowngradeSubscription(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
			"code":  "INVALID_USER_ID",
		})
		return
	}

	var req DowngradeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request",
			"code":  "INVALID_REQUEST",
		})
		return
	}

	// Validate tier
	newTier := Tier(req.Tier)
	if !newTier.IsValid() {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid tier",
			"code":  "INVALID_TIER",
		})
		return
	}

	// Process downgrade
	err = h.service.DowngradeTier(c.Request.Context(), userID, newTier)
	if err != nil {
		h.logger.Error("Failed to downgrade tier",
			zap.Error(err),
			zap.String("user_id", userID.String()),
			zap.String("new_tier", string(newTier)),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to downgrade subscription",
			"code":  "DOWNGRADE_FAILED",
		})
		return
	}

	// Get updated tier info
	info, _ := h.service.GetUserTierInfo(c.Request.Context(), userID)

	c.JSON(http.StatusOK, gin.H{
		"message":      "Subscription downgraded successfully",
		"subscription": info,
	})
}

// CancelSubscription cancels the user's subscription
// @Summary Cancel subscription
// @Description Cancel the current subscription (downgrade to free tier)
// @Tags subscription
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{} "Cancellation successful"
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/subscription/cancel [post]
func (h *Handler) CancelSubscription(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
			"code":  "INVALID_USER_ID",
		})
		return
	}

	// Downgrade to free tier
	err = h.service.DowngradeTier(c.Request.Context(), userID, TierFree)
	if err != nil {
		h.logger.Error("Failed to cancel subscription",
			zap.Error(err),
			zap.String("user_id", userID.String()),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to cancel subscription",
			"code":  "CANCELLATION_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Subscription cancelled successfully",
	})
}

// GetUsageStatus gets the current usage status
// @Summary Get usage status
// @Description Get current usage status for various limits
// @Tags subscription
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{} "Usage status"
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/subscription/usage [get]
func (h *Handler) GetUsageStatus(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
			"code":  "INVALID_USER_ID",
		})
		return
	}

	// Get tier info
	info, err := h.service.GetUserTierInfo(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get tier info",
			zap.Error(err),
			zap.String("user_id", userID.String()),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get usage status",
			"code":  "USAGE_STATUS_FAILED",
		})
		return
	}

	// Check various usage limits
	limitTypes := []string{
		"checkins_per_day",
		"checkins_per_month",
		"categories_count",
		"api_calls_per_hour",
		"exports_per_month",
	}

	usage := make(map[string]interface{})
	for _, limitType := range limitTypes {
		usageInfo, _ := h.service.CheckUsageLimit(c.Request.Context(), userID, limitType)
		if usageInfo != nil {
			usage[limitType] = map[string]interface{}{
				"current":   usageInfo.Current,
				"limit":     usageInfo.Limit,
				"unlimited": usageInfo.Unlimited,
				"reset_at":  usageInfo.ResetAt,
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"tier":  info.Tier,
		"usage": usage,
	})
}

// GetFeatures gets the list of features available to the user
// @Summary Get available features
// @Description Get list of features available with current subscription
// @Tags subscription
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{} "Available features"
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/subscription/features [get]
func (h *Handler) GetFeatures(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
			"code":  "INVALID_USER_ID",
		})
		return
	}

	// Get tier info
	info, err := h.service.GetUserTierInfo(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get tier info",
			zap.Error(err),
			zap.String("user_id", userID.String()),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get features",
			"code":  "FEATURES_FAILED",
		})
		return
	}

	// Build feature list with descriptions
	features := make([]map[string]interface{}, len(info.Features))
	for i, feature := range info.Features {
		features[i] = map[string]interface{}{
			"key":         feature,
			"description": GetFeatureDescription(feature),
			"enabled":     true,
		}
	}

	// Add all features with enabled status
	allFeatures := []Feature{
		FeatureUnlimitedCheckins,
		FeatureExtendedEditHistory,
		FeatureUnlimitedCategories,
		FeatureAdvancedAnalytics,
		FeatureDataExport,
		FeatureAPIAccess,
		FeatureWebhooks,
		FeaturePrioritySupport,
		FeatureTeamCollaboration,
		FeatureSSO,
		FeatureAuditLogs,
	}

	allFeaturesMap := make([]map[string]interface{}, 0)
	for _, feature := range allFeatures {
		enabled := HasFeature(info.Tier, feature)
		allFeaturesMap = append(allFeaturesMap, map[string]interface{}{
			"key":         feature,
			"description": GetFeatureDescription(feature),
			"enabled":     enabled,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"tier":          info.Tier,
		"all_features":  allFeaturesMap,
		"user_features": features,
	})
}

// GetSubscriptionEvents gets subscription events/audit log
// @Summary Get subscription events
// @Description Get subscription events and audit log
// @Tags subscription
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param limit query int false "Number of events to return" default(50)
// @Success 200 {array} subscription.SubscriptionEvent
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/subscription/events [get]
func (h *Handler) GetSubscriptionEvents(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
			"code":  "INVALID_USER_ID",
		})
		return
	}

	limit := 50
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	events, err := h.service.GetSubscriptionEvents(c.Request.Context(), userID, limit)
	if err != nil {
		h.logger.Error("Failed to get subscription events",
			zap.Error(err),
			zap.String("user_id", userID.String()),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get subscription events",
			"code":  "EVENTS_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"events": events,
	})
}

// getUserID extracts user ID from JWT claims
func (h *Handler) getUserID(c *gin.Context) (uuid.UUID, error) {
	claims, exists := c.Get("claims")
	if !exists {
		return uuid.Nil, fmt.Errorf("no claims found")
	}

	jwtClaims, ok := claims.(*auth.Claims)
	if !ok {
		return uuid.Nil, fmt.Errorf("invalid claims type")
	}

	return jwtClaims.UserID, nil
}

// Request/Response types

// UpgradeRequest represents a subscription upgrade request
type UpgradeRequest struct {
	Tier          string `json:"tier" binding:"required"`
	BillingPeriod string `json:"billing_period" binding:"required,oneof=monthly yearly"`
	PaymentMethod string `json:"payment_method,omitempty"`
}

// DowngradeRequest represents a subscription downgrade request
type DowngradeRequest struct {
	Tier   string `json:"tier" binding:"required"`
	Reason string `json:"reason,omitempty"`
}