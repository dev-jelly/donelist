package admin

import (
	"net/http"
	"strconv"

	"github.com/dev-jelly/donelist/internal/subscription"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// SubscriptionHandlers handles admin HTTP requests for subscriptions
type SubscriptionHandlers struct {
	operations *SubscriptionOperations
	logger     *zap.Logger
}

// NewSubscriptionHandlers creates a new subscription handlers
func NewSubscriptionHandlers(operations *SubscriptionOperations, logger *zap.Logger) *SubscriptionHandlers {
	return &SubscriptionHandlers{
		operations: operations,
		logger:     logger,
	}
}

// RegisterRoutes registers admin subscription routes
func (h *SubscriptionHandlers) RegisterRoutes(router *gin.RouterGroup) {
	subs := router.Group("/subscriptions")
	{
		subs.GET("", h.ListSubscriptions)
		subs.GET("/:id", h.GetSubscription)
		subs.POST("/:id/manual-update", h.ManualUpdate)
		subs.GET("/metrics", h.GetMetrics)
	}

	payments := router.Group("/payments")
	{
		payments.GET("", h.GetPaymentHistory)
		payments.GET("/failed", h.GetFailedPayments)
		payments.POST("/:id/refund", h.CreateRefund)
	}
}

// ListSubscriptions handles GET /admin/subscriptions
func (h *SubscriptionHandlers) ListSubscriptions(c *gin.Context) {
	// Parse query parameters
	var filter subscription.SubscriptionFilter

	if status := c.Query("status"); status != "" {
		s := subscription.SubscriptionStatus(status)
		if s.IsValid() {
			filter.Status = &s
		}
	}

	if planID := c.Query("plan_id"); planID != "" {
		p := subscription.PlanID(planID)
		filter.PlanID = &p
	}

	if userID := c.Query("user_id"); userID != "" {
		id, err := uuid.Parse(userID)
		if err == nil {
			filter.UserID = &id
		}
	}

	// Pagination
	page := 0
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil {
			page = p
		}
	}

	pageSize := 50
	if pageSizeStr := c.Query("page_size"); pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 && ps <= 100 {
			pageSize = ps
		}
	}

	filter.Limit = pageSize
	filter.Offset = page * pageSize

	// Sorting
	if sortBy := c.Query("sort_by"); sortBy != "" {
		filter.SortBy = sortBy
	}
	filter.SortDesc = c.Query("sort_order") == "desc"

	// Get subscriptions
	response, err := h.operations.ListSubscriptions(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("Failed to list subscriptions", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve subscriptions"})
		return
	}

	c.JSON(http.StatusOK, response)
}

// GetSubscription handles GET /admin/subscriptions/:id
func (h *SubscriptionHandlers) GetSubscription(c *gin.Context) {
	subscriptionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid subscription ID"})
		return
	}

	detail, err := h.operations.GetSubscription(c.Request.Context(), subscriptionID)
	if err != nil {
		h.logger.Error("Failed to get subscription", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve subscription"})
		return
	}

	c.JSON(http.StatusOK, detail)
}

// ManualUpdate handles POST /admin/subscriptions/:id/manual-update
func (h *SubscriptionHandlers) ManualUpdate(c *gin.Context) {
	subscriptionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid subscription ID"})
		return
	}

	var update ManualSubscriptionUpdate
	if err := c.ShouldBindJSON(&update); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	update.SubscriptionID = subscriptionID

	// Validate action
	validActions := map[string]bool{
		"cancel":      true,
		"reactivate":  true,
		"change_plan": true,
		"extend":      true,
	}
	if !validActions[update.Action] {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid action. Must be one of: cancel, reactivate, change_plan, extend",
		})
		return
	}

	// Require reason
	if update.Reason == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Reason is required for manual updates"})
		return
	}

	err = h.operations.PerformManualUpdate(c.Request.Context(), &update)
	if err != nil {
		h.logger.Error("Failed to perform manual update",
			zap.Error(err),
			zap.String("subscription_id", subscriptionID.String()),
			zap.String("action", update.Action),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Get updated subscription
	detail, err := h.operations.GetSubscription(c.Request.Context(), subscriptionID)
	if err != nil {
		h.logger.Warn("Failed to get updated subscription", zap.Error(err))
		c.JSON(http.StatusOK, gin.H{"message": "Update successful"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":      "Update successful",
		"subscription": detail,
	})
}

// GetMetrics handles GET /admin/subscriptions/metrics
func (h *SubscriptionHandlers) GetMetrics(c *gin.Context) {
	metrics, err := h.operations.GetSubscriptionMetrics(c.Request.Context())
	if err != nil {
		h.logger.Error("Failed to get subscription metrics", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve metrics"})
		return
	}

	c.JSON(http.StatusOK, metrics)
}

// GetPaymentHistory handles GET /admin/payments
func (h *SubscriptionHandlers) GetPaymentHistory(c *gin.Context) {
	var userID *uuid.UUID
	if userIDStr := c.Query("user_id"); userIDStr != "" {
		id, err := uuid.Parse(userIDStr)
		if err == nil {
			userID = &id
		}
	}

	// Pagination
	limit := 50
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	offset := 0
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	response, err := h.operations.GetPaymentHistory(c.Request.Context(), userID, limit, offset)
	if err != nil {
		h.logger.Error("Failed to get payment history", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve payment history"})
		return
	}

	c.JSON(http.StatusOK, response)
}

// GetFailedPayments handles GET /admin/payments/failed
func (h *SubscriptionHandlers) GetFailedPayments(c *gin.Context) {
	limit := 100
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	failedPayments, err := h.operations.GetFailedPayments(c.Request.Context(), limit)
	if err != nil {
		h.logger.Error("Failed to get failed payments", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve failed payments"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"failed_payments": failedPayments,
		"count":           len(failedPayments),
	})
}

// CreateRefund handles POST /admin/payments/:id/refund
func (h *SubscriptionHandlers) CreateRefund(c *gin.Context) {
	paymentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payment ID"})
		return
	}

	var req struct {
		Amount int    `json:"amount"` // Amount in cents, 0 for full refund
		Reason string `json:"reason" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err = h.operations.CreateRefund(c.Request.Context(), paymentID, req.Amount, req.Reason)
	if err != nil {
		h.logger.Error("Failed to create refund",
			zap.Error(err),
			zap.String("payment_id", paymentID.String()),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "Refund created successfully",
		"payment_id": paymentID,
		"amount":     req.Amount,
	})
}
