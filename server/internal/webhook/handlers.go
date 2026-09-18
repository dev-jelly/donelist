package webhook

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Handler handles HTTP requests for webhooks
type Handler struct {
	service *Service
	logger  *zap.Logger
}

// NewHandler creates a new webhook handler
func NewHandler(service *Service, logger *zap.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  logger,
	}
}

// RegisterRoutes registers webhook routes
func (h *Handler) RegisterRoutes(r *gin.RouterGroup) {
	webhooks := r.Group("/webhooks")
	{
		webhooks.POST("", h.CreateWebhook)
		webhooks.GET("", h.ListWebhooks)
		webhooks.GET("/:id", h.GetWebhook)
		webhooks.PUT("/:id", h.UpdateWebhook)
		webhooks.DELETE("/:id", h.DeleteWebhook)
		webhooks.POST("/:id/test", h.TestWebhook)
		webhooks.GET("/:id/stats", h.GetWebhookStats)

		// Deliveries
		webhooks.GET("/:id/deliveries", h.GetDeliveries)
		webhooks.POST("/deliveries/:delivery_id/resend", h.ResendDelivery)

		// Dead letter queue
		webhooks.GET("/dlq", h.GetDLQEntries)
	}
}

// CreateWebhook creates a new webhook
// @Summary Create webhook
// @Description Create a new webhook configuration
// @Tags webhooks
// @Accept json
// @Produce json
// @Security Bearer
// @Param webhook body CreateWebhookRequest true "Webhook configuration"
// @Success 201 {object} WebhookResponse
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /webhooks [post]
func (h *Handler) CreateWebhook(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req CreateWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	webhook, err := h.service.CreateWebhook(c.Request.Context(), &req, userID.(uuid.UUID))
	if err != nil {
		h.logger.Error("Failed to create webhook", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create webhook"})
		return
	}

	c.JSON(http.StatusCreated, webhook.ToResponse())
}

// ListWebhooks lists all webhooks for the authenticated user
// @Summary List webhooks
// @Description Get all webhooks for the authenticated user
// @Tags webhooks
// @Produce json
// @Security Bearer
// @Success 200 {array} WebhookResponse
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /webhooks [get]
func (h *Handler) ListWebhooks(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	webhooks, err := h.service.ListWebhooks(c.Request.Context(), userID.(uuid.UUID))
	if err != nil {
		h.logger.Error("Failed to list webhooks", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list webhooks"})
		return
	}

	// Convert to response format
	response := make([]*WebhookResponse, len(webhooks))
	for i, w := range webhooks {
		response[i] = w.ToResponse()
	}

	c.JSON(http.StatusOK, response)
}

// GetWebhook gets a webhook by ID
// @Summary Get webhook
// @Description Get a webhook by ID
// @Tags webhooks
// @Produce json
// @Security Bearer
// @Param id path string true "Webhook ID"
// @Success 200 {object} WebhookResponse
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /webhooks/{id} [get]
func (h *Handler) GetWebhook(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	webhookID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid webhook id"})
		return
	}

	webhook, err := h.service.GetWebhook(c.Request.Context(), webhookID, userID.(uuid.UUID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "webhook not found"})
		return
	}

	c.JSON(http.StatusOK, webhook.ToResponse())
}

// UpdateWebhook updates a webhook
// @Summary Update webhook
// @Description Update a webhook configuration
// @Tags webhooks
// @Accept json
// @Produce json
// @Security Bearer
// @Param id path string true "Webhook ID"
// @Param webhook body UpdateWebhookRequest true "Webhook updates"
// @Success 200 {object} WebhookResponse
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /webhooks/{id} [put]
func (h *Handler) UpdateWebhook(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	webhookID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid webhook id"})
		return
	}

	var req UpdateWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	webhook, err := h.service.UpdateWebhook(c.Request.Context(), webhookID, userID.(uuid.UUID), &req)
	if err != nil {
		h.logger.Error("Failed to update webhook", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update webhook"})
		return
	}

	c.JSON(http.StatusOK, webhook.ToResponse())
}

// DeleteWebhook deletes a webhook
// @Summary Delete webhook
// @Description Delete a webhook
// @Tags webhooks
// @Security Bearer
// @Param id path string true "Webhook ID"
// @Success 204
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /webhooks/{id} [delete]
func (h *Handler) DeleteWebhook(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	webhookID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid webhook id"})
		return
	}

	if err := h.service.DeleteWebhook(c.Request.Context(), webhookID, userID.(uuid.UUID)); err != nil {
		h.logger.Error("Failed to delete webhook", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete webhook"})
		return
	}

	c.Status(http.StatusNoContent)
}

// TestWebhook sends a test webhook
// @Summary Test webhook
// @Description Send a test webhook
// @Tags webhooks
// @Security Bearer
// @Param id path string true "Webhook ID"
// @Success 202 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /webhooks/{id}/test [post]
func (h *Handler) TestWebhook(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	webhookID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid webhook id"})
		return
	}

	if err := h.service.TestWebhook(c.Request.Context(), webhookID, userID.(uuid.UUID)); err != nil {
		h.logger.Error("Failed to test webhook", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to test webhook"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"message": "test webhook queued"})
}

// GetWebhookStats gets webhook statistics
// @Summary Get webhook statistics
// @Description Get aggregated statistics for a webhook
// @Tags webhooks
// @Produce json
// @Security Bearer
// @Param id path string true "Webhook ID"
// @Param since query string false "Start time (RFC3339 format)" default:"7 days ago"
// @Success 200 {object} WebhookStats
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /webhooks/{id}/stats [get]
func (h *Handler) GetWebhookStats(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	webhookID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid webhook id"})
		return
	}

	// Parse since parameter
	since := time.Now().AddDate(0, 0, -7) // Default: 7 days ago
	if sinceStr := c.Query("since"); sinceStr != "" {
		if parsedTime, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			since = parsedTime
		}
	}

	stats, err := h.service.GetWebhookStats(c.Request.Context(), webhookID, userID.(uuid.UUID), since)
	if err != nil {
		h.logger.Error("Failed to get webhook stats", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get webhook stats"})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// GetDeliveries gets webhook deliveries
// @Summary Get webhook deliveries
// @Description Get delivery history for a webhook
// @Tags webhooks
// @Produce json
// @Security Bearer
// @Param id path string true "Webhook ID"
// @Param limit query int false "Limit number of results" default:50
// @Success 200 {array} WebhookDelivery
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /webhooks/{id}/deliveries [get]
func (h *Handler) GetDeliveries(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	webhookID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid webhook id"})
		return
	}

	limit := 50
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
			if limit > 100 {
				limit = 100 // Max limit
			}
		}
	}

	deliveries, err := h.service.GetDeliveries(c.Request.Context(), webhookID, userID.(uuid.UUID), limit)
	if err != nil {
		h.logger.Error("Failed to get deliveries", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get deliveries"})
		return
	}

	c.JSON(http.StatusOK, deliveries)
}

// ResendDelivery resends a failed webhook delivery
// @Summary Resend delivery
// @Description Resend a failed webhook delivery
// @Tags webhooks
// @Security Bearer
// @Param delivery_id path string true "Delivery ID"
// @Success 202 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /webhooks/deliveries/{delivery_id}/resend [post]
func (h *Handler) ResendDelivery(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	deliveryID, err := uuid.Parse(c.Param("delivery_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid delivery id"})
		return
	}

	if err := h.service.ResendDelivery(c.Request.Context(), deliveryID, userID.(uuid.UUID)); err != nil {
		h.logger.Error("Failed to resend delivery", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resend delivery"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"message": "delivery queued for resend"})
}

// GetDLQEntries gets dead letter queue entries
// @Summary Get dead letter queue
// @Description Get failed webhook deliveries from dead letter queue
// @Tags webhooks
// @Produce json
// @Security Bearer
// @Param webhook_id query string false "Filter by webhook ID"
// @Param limit query int false "Limit number of results" default:50
// @Success 200 {array} WebhookDLQ
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /webhooks/dlq [get]
func (h *Handler) GetDLQEntries(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var webhookID uuid.UUID
	if webhookIDStr := c.Query("webhook_id"); webhookIDStr != "" {
		parsed, err := uuid.Parse(webhookIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid webhook id"})
			return
		}
		webhookID = parsed
	}

	limit := 50
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
			if limit > 100 {
				limit = 100 // Max limit
			}
		}
	}

	entries, err := h.service.GetDLQEntries(c.Request.Context(), webhookID, userID.(uuid.UUID), limit)
	if err != nil {
		h.logger.Error("Failed to get DLQ entries", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get DLQ entries"})
		return
	}

	c.JSON(http.StatusOK, entries)
}

// GetAvailableEvents returns the list of available webhook events
// @Summary Get available events
// @Description Get list of all available webhook event types
// @Tags webhooks
// @Produce json
// @Security Bearer
// @Success 200 {array} map[string]string
// @Router /webhooks/events [get]
func (h *Handler) GetAvailableEvents(c *gin.Context) {
	events := []map[string]string{
		{
			"type":        string(EventCheckinCreated),
			"description": "Triggered when a new check-in is created",
		},
		{
			"type":        string(EventCheckinUpdated),
			"description": "Triggered when a check-in is updated",
		},
		{
			"type":        string(EventCheckinDeleted),
			"description": "Triggered when a check-in is deleted",
		},
		{
			"type":        string(EventCategoryCreated),
			"description": "Triggered when a new category is created",
		},
		{
			"type":        string(EventCategoryUpdated),
			"description": "Triggered when a category is updated",
		},
		{
			"type":        string(EventCategoryDeleted),
			"description": "Triggered when a category is deleted",
		},
		{
			"type":        string(EventUserUpdated),
			"description": "Triggered when user profile is updated",
		},
		{
			"type":        string(EventUserUpgraded),
			"description": "Triggered when user upgrades subscription",
		},
		{
			"type":        string(EventSubscriptionChanged),
			"description": "Triggered when subscription status changes",
		},
	}

	c.JSON(http.StatusOK, events)
}
