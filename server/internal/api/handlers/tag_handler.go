package handlers

import (
	"net/http"
	"strconv"

	"github.com/dev-jelly/donelist/internal/api/middleware"
	"github.com/dev-jelly/donelist/internal/tag"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// TagHandler handles tag endpoints
type TagHandler struct {
	tagService *tag.Service
	logger     *zap.Logger
}

// NewTagHandler creates a new tag handler
func NewTagHandler(tagService *tag.Service, logger *zap.Logger) *TagHandler {
	return &TagHandler{
		tagService: tagService,
		logger:     logger,
	}
}

// CreateTagRequest represents the request body for creating a tag
type CreateTagRequest struct {
	Name string `json:"name" binding:"required"`
}

// Create creates a new tag
// @Summary Create a new tag
// @Description Create a new tag with a given name
// @Tags Tags
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateTagRequest true "Tag details"
// @Success 201 {object} tag.Tag "Created tag"
// @Failure 400 {object} map[string]string "Invalid request body"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Router /tags [post]
func (h *TagHandler) Create(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req CreateTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Invalid request body", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	t, err := h.tagService.Create(c.Request.Context(), tag.CreateInput{
		UserID: userID,
		Name:   req.Name,
	})
	if err != nil {
		h.logger.Error("Failed to create tag", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, t)
}

// List retrieves all tags for the authenticated user
// @Summary List tags
// @Description Retrieve all tags for the authenticated user
// @Tags Tags
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{} "tags"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Failed to list tags"
// @Router /tags [get]
func (h *TagHandler) List(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	tags, err := h.tagService.List(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to list tags", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list tags"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"tags": tags})
}

// GetByID retrieves a tag by ID
// @Summary Get tag by ID
// @Description Retrieve a specific tag by its ID
// @Tags Tags
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Tag ID (UUID)"
// @Success 200 {object} tag.Tag "Tag details"
// @Failure 400 {object} map[string]string "Invalid tag ID"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "Tag not found"
// @Router /tags/{id} [get]
func (h *TagHandler) GetByID(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.logger.Warn("Invalid tag ID", zap.String("id", idStr), zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tag id"})
		return
	}

	t, err := h.tagService.GetByID(c.Request.Context(), id, userID)
	if err != nil {
		h.logger.Error("Failed to get tag", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "tag not found"})
		return
	}

	c.JSON(http.StatusOK, t)
}

// Autocomplete provides tag autocompletion suggestions
// @Summary Autocomplete tags
// @Description Get tag suggestions based on a query string
// @Tags Tags
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param q query string true "Search query"
// @Param limit query int false "Maximum number of results (default: 10, max: 50)"
// @Success 200 {object} map[string]interface{} "tags, query, count"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Failed to search tags"
// @Router /tags/autocomplete [get]
func (h *TagHandler) Autocomplete(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	query := c.Query("q")
	limitStr := c.DefaultQuery("limit", "10")

	limit := 10
	if val, err := strconv.Atoi(limitStr); err == nil && val > 0 && val <= 50 {
		limit = val
	}

	tags, err := h.tagService.Autocomplete(c.Request.Context(), userID, query, limit)
	if err != nil {
		h.logger.Error("Failed to autocomplete tags", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to search tags"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tags":  tags,
		"query": query,
		"count": len(tags),
	})
}

// GetPopular retrieves the most popular tags
// @Summary Get popular tags
// @Description Retrieve the most frequently used tags for the authenticated user
// @Tags Tags
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param limit query int false "Maximum number of results (default: 10, max: 50)"
// @Success 200 {object} map[string]interface{} "tags, count"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Failed to get popular tags"
// @Router /tags/popular [get]
func (h *TagHandler) GetPopular(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	limitStr := c.DefaultQuery("limit", "10")
	limit := 10
	if val, err := strconv.Atoi(limitStr); err == nil && val > 0 && val <= 50 {
		limit = val
	}

	tags, err := h.tagService.GetPopularTags(c.Request.Context(), userID, limit)
	if err != nil {
		h.logger.Error("Failed to get popular tags", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get popular tags"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tags":  tags,
		"count": len(tags),
	})
}
