package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/dev-jelly/donelist/internal/api/middleware"
	"github.com/dev-jelly/donelist/internal/apikey"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// APIKeyHandler handles API key management endpoints
type APIKeyHandler struct {
	service *apikey.Service
	logger  *zap.Logger
}

// NewAPIKeyHandler creates a new API key handler
func NewAPIKeyHandler(service *apikey.Service, logger *zap.Logger) *APIKeyHandler {
	return &APIKeyHandler{
		service: service,
		logger:  logger,
	}
}

// CreateAPIKeyRequest represents the request body for creating an API key
type CreateAPIKeyRequest struct {
	Name             string          `json:"name" binding:"required,min=3,max=100"`
	Scopes           []apikey.Scope  `json:"scopes" binding:"required,min=1"`
	RateLimitPerDay  *int            `json:"rate_limit_per_day,omitempty"`
	RateLimitPerHour *int            `json:"rate_limit_per_hour,omitempty"`
	ExpiresInDays    *int            `json:"expires_in_days,omitempty"`
}

// UpdateAPIKeyRequest represents the request body for updating an API key
type UpdateAPIKeyRequest struct {
	Name             *string         `json:"name,omitempty"`
	Scopes           []apikey.Scope  `json:"scopes,omitempty"`
	RateLimitPerDay  *int            `json:"rate_limit_per_day,omitempty"`
	RateLimitPerHour *int            `json:"rate_limit_per_hour,omitempty"`
	ExpiresInDays    *int            `json:"expires_in_days,omitempty"`
}

// RevokeAPIKeyRequest represents the request body for revoking an API key
type RevokeAPIKeyRequest struct {
	Reason string `json:"reason" binding:"required,min=3,max=200"`
}

// CreateAPIKey creates a new API key
// @Summary Create API key
// @Description Create a new API key with specified scopes and rate limits
// @Tags API Keys
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateAPIKeyRequest true "API key creation request"
// @Success 201 {object} apikey.APIKeyWithPlainText
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/api-keys [post]
func (h *APIKeyHandler) CreateAPIKey(c *gin.Context) {
	var req CreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Build input
	input := apikey.CreateAPIKeyInput{
		UserID: userID,
		Name:   req.Name,
		Scopes: req.Scopes,
	}

	if req.RateLimitPerDay != nil {
		input.RateLimitPerDay = *req.RateLimitPerDay
	}
	if req.RateLimitPerHour != nil {
		input.RateLimitPerHour = *req.RateLimitPerHour
	}
	if req.ExpiresInDays != nil && *req.ExpiresInDays > 0 {
		expiresAt := time.Now().AddDate(0, 0, *req.ExpiresInDays)
		input.ExpiresAt = &expiresAt
	}

	apiKey, err := h.service.CreateAPIKey(c.Request.Context(), input)
	if err != nil {
		h.logger.Error("Failed to create API key", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create API key"})
		return
	}

	c.JSON(http.StatusCreated, apiKey)
}

// ListAPIKeys lists all API keys for the authenticated user
// @Summary List API keys
// @Description List all API keys for the authenticated user
// @Tags API Keys
// @Produce json
// @Security BearerAuth
// @Param include_revoked query bool false "Include revoked keys"
// @Success 200 {array} apikey.APIKey
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/api-keys [get]
func (h *APIKeyHandler) ListAPIKeys(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	includeRevoked := c.Query("include_revoked") == "true"

	apiKeys, err := h.service.ListAPIKeys(c.Request.Context(), userID, includeRevoked)
	if err != nil {
		h.logger.Error("Failed to list API keys", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list API keys"})
		return
	}

	c.JSON(http.StatusOK, apiKeys)
}

// GetAPIKey retrieves a specific API key
// @Summary Get API key
// @Description Get details of a specific API key
// @Tags API Keys
// @Produce json
// @Security BearerAuth
// @Param id path string true "API Key ID"
// @Success 200 {object} apikey.APIKey
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/api-keys/{id} [get]
func (h *APIKeyHandler) GetAPIKey(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid API key ID"})
		return
	}

	apiKey, err := h.service.GetAPIKey(c.Request.Context(), id, userID)
	if err != nil {
		if err == apikey.ErrAPIKeyNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "API key not found"})
			return
		}
		h.logger.Error("Failed to get API key", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get API key"})
		return
	}

	c.JSON(http.StatusOK, apiKey)
}

// UpdateAPIKey updates an API key
// @Summary Update API key
// @Description Update an API key's settings
// @Tags API Keys
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "API Key ID"
// @Param request body UpdateAPIKeyRequest true "API key update request"
// @Success 200 {object} apikey.APIKey
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/api-keys/{id} [patch]
func (h *APIKeyHandler) UpdateAPIKey(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid API key ID"})
		return
	}

	var req UpdateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Build input
	input := apikey.UpdateAPIKeyInput{
		Name:             req.Name,
		Scopes:           req.Scopes,
		RateLimitPerDay:  req.RateLimitPerDay,
		RateLimitPerHour: req.RateLimitPerHour,
	}

	if req.ExpiresInDays != nil && *req.ExpiresInDays > 0 {
		expiresAt := time.Now().AddDate(0, 0, *req.ExpiresInDays)
		input.ExpiresAt = &expiresAt
	}

	apiKey, err := h.service.UpdateAPIKey(c.Request.Context(), id, userID, input)
	if err != nil {
		if err == apikey.ErrAPIKeyNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "API key not found"})
			return
		}
		if err == apikey.ErrAPIKeyRevoked {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot update revoked API key"})
			return
		}
		h.logger.Error("Failed to update API key", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update API key"})
		return
	}

	c.JSON(http.StatusOK, apiKey)
}

// RevokeAPIKey revokes an API key
// @Summary Revoke API key
// @Description Revoke an API key (soft delete)
// @Tags API Keys
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "API Key ID"
// @Param request body RevokeAPIKeyRequest true "Revoke reason"
// @Success 204
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/api-keys/{id}/revoke [post]
func (h *APIKeyHandler) RevokeAPIKey(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid API key ID"})
		return
	}

	var req RevokeAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err = h.service.RevokeAPIKey(c.Request.Context(), id, userID, req.Reason)
	if err != nil {
		if err == apikey.ErrAPIKeyNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "API key not found"})
			return
		}
		h.logger.Error("Failed to revoke API key", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke API key"})
		return
	}

	c.Status(http.StatusNoContent)
}

// DeleteAPIKey permanently deletes an API key
// @Summary Delete API key
// @Description Permanently delete an API key
// @Tags API Keys
// @Produce json
// @Security BearerAuth
// @Param id path string true "API Key ID"
// @Success 204
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/api-keys/{id} [delete]
func (h *APIKeyHandler) DeleteAPIKey(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid API key ID"})
		return
	}

	err = h.service.DeleteAPIKey(c.Request.Context(), id, userID)
	if err != nil {
		if err == apikey.ErrAPIKeyNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "API key not found"})
			return
		}
		h.logger.Error("Failed to delete API key", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete API key"})
		return
	}

	c.Status(http.StatusNoContent)
}

// RotateAPIKey rotates an API key
// @Summary Rotate API key
// @Description Create a new API key and revoke the old one
// @Tags API Keys
// @Produce json
// @Security BearerAuth
// @Param id path string true "API Key ID"
// @Success 201 {object} apikey.APIKeyWithPlainText
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/api-keys/{id}/rotate [post]
func (h *APIKeyHandler) RotateAPIKey(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid API key ID"})
		return
	}

	newKey, err := h.service.RotateAPIKey(c.Request.Context(), id, userID)
	if err != nil {
		if err == apikey.ErrAPIKeyNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "API key not found"})
			return
		}
		h.logger.Error("Failed to rotate API key", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to rotate API key"})
		return
	}

	c.JSON(http.StatusCreated, newKey)
}

// GetAPIKeyUsageStatistics retrieves usage statistics for an API key
// @Summary Get API key usage statistics
// @Description Get usage statistics for a specific API key
// @Tags API Keys
// @Produce json
// @Security BearerAuth
// @Param id path string true "API Key ID"
// @Param days query int false "Number of days (default: 30, max: 90)"
// @Success 200 {object} apikey.UsageStatistics
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/api-keys/{id}/statistics [get]
func (h *APIKeyHandler) GetAPIKeyUsageStatistics(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid API key ID"})
		return
	}

	days := 30
	if daysStr := c.Query("days"); daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil && d > 0 {
			days = d
		}
	}

	stats, err := h.service.GetUsageStatistics(c.Request.Context(), id, userID, days)
	if err != nil {
		if err == apikey.ErrAPIKeyNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "API key not found"})
			return
		}
		h.logger.Error("Failed to get usage statistics", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get usage statistics"})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// GetAvailableScopes returns all available API key scopes
// @Summary Get available scopes
// @Description Get a list of all available API key scopes
// @Tags API Keys
// @Produce json
// @Security BearerAuth
// @Success 200 {array} string
// @Failure 401 {object} map[string]interface{}
// @Router /api/v1/api-keys/scopes [get]
func (h *APIKeyHandler) GetAvailableScopes(c *gin.Context) {
	scopes := make([]string, len(apikey.AllScopes))
	for i, scope := range apikey.AllScopes {
		scopes[i] = string(scope)
	}

	c.JSON(http.StatusOK, gin.H{
		"scopes": scopes,
	})
}
