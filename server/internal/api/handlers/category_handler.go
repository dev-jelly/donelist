package handlers

import (
	"net/http"
	"strconv"

	"github.com/dev-jelly/donelist/internal/api/middleware"
	"github.com/dev-jelly/donelist/internal/category"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// CategoryHandler handles category endpoints
type CategoryHandler struct {
	categoryService *category.Service
	logger          *zap.Logger
}

// NewCategoryHandler creates a new category handler
func NewCategoryHandler(categoryService *category.Service, logger *zap.Logger) *CategoryHandler {
	return &CategoryHandler{
		categoryService: categoryService,
		logger:          logger,
	}
}

// CreateRequest represents the request body for creating a category
type CreateCategoryRequest struct {
	Name  string  `json:"name" binding:"required"`
	Color *string `json:"color"`
	Icon  *string `json:"icon"`
}

// UpdateCategoryRequest represents the request body for updating a category
type UpdateCategoryRequest struct {
	Name  *string `json:"name"`
	Color *string `json:"color"`
	Icon  *string `json:"icon"`
}

// Create creates a new category
// @Summary Create a new category
// @Description Create a new category with name, optional color and icon
// @Tags Categories
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateCategoryRequest true "Category details"
// @Success 201 {object} category.Category "Created category"
// @Failure 400 {object} map[string]string "Invalid request body"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Router /categories [post]
func (h *CategoryHandler) Create(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req CreateCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Invalid request body", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	cat, err := h.categoryService.Create(c.Request.Context(), category.CreateInput{
		UserID: userID,
		Name:   req.Name,
		Color:  req.Color,
		Icon:   req.Icon,
	})
	if err != nil {
		h.logger.Error("Failed to create category", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, cat)
}

// List retrieves all categories for the authenticated user
// @Summary List categories
// @Description Retrieve all categories for the authenticated user
// @Tags Categories
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{} "categories"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Failed to list categories"
// @Router /categories [get]
func (h *CategoryHandler) List(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	categories, err := h.categoryService.List(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to list categories", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list categories"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"categories": categories})
}

// GetByID retrieves a category by ID
// @Summary Get category by ID
// @Description Retrieve a specific category by its ID
// @Tags Categories
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Category ID (UUID)"
// @Success 200 {object} category.Category "Category details"
// @Failure 400 {object} map[string]string "Invalid category ID"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "Category not found"
// @Router /categories/{id} [get]
func (h *CategoryHandler) GetByID(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.logger.Warn("Invalid category ID", zap.String("id", idStr), zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid category id"})
		return
	}

	cat, err := h.categoryService.GetByID(c.Request.Context(), id, userID)
	if err != nil {
		h.logger.Error("Failed to get category", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "category not found"})
		return
	}

	c.JSON(http.StatusOK, cat)
}

// Update updates a category
// @Summary Update category
// @Description Update an existing category's name, color, or icon
// @Tags Categories
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Category ID (UUID)"
// @Param request body UpdateCategoryRequest true "Category update details"
// @Success 200 {object} category.Category "Updated category"
// @Failure 400 {object} map[string]string "Invalid request"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Router /categories/{id} [patch]
func (h *CategoryHandler) Update(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.logger.Warn("Invalid category ID", zap.String("id", idStr), zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid category id"})
		return
	}

	var req UpdateCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Invalid request body", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	cat, err := h.categoryService.Update(c.Request.Context(), id, userID, category.UpdateInput{
		Name:  req.Name,
		Color: req.Color,
		Icon:  req.Icon,
	})
	if err != nil {
		h.logger.Error("Failed to update category", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, cat)
}

// Delete deletes a category
// @Summary Delete category
// @Description Permanently delete a category
// @Tags Categories
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Category ID (UUID)"
// @Success 200 {object} map[string]string "message"
// @Failure 400 {object} map[string]string "Invalid category ID"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Failed to delete category"
// @Router /categories/{id} [delete]
func (h *CategoryHandler) Delete(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.logger.Warn("Invalid category ID", zap.String("id", idStr), zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid category id"})
		return
	}

	if err := h.categoryService.Delete(c.Request.Context(), id, userID); err != nil {
		h.logger.Error("Failed to delete category", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete category"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "category deleted successfully"})
}

// GetRecommendedColors retrieves recommended colors for the user
// @Summary Get recommended colors
// @Description Get a list of recommended colors based on existing categories
// @Tags Categories
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param count query int false "Number of recommendations (default 5)" default(5)
// @Success 200 {object} map[string]interface{} "recommendations"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Failed to get recommendations"
// @Router /categories/colors/recommendations [get]
func (h *CategoryHandler) GetRecommendedColors(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Parse count parameter
	count := 5
	if countStr := c.Query("count"); countStr != "" {
		if parsedCount, err := strconv.Atoi(countStr); err == nil && parsedCount > 0 && parsedCount <= 10 {
			count = parsedCount
		}
	}

	recommendations, err := h.categoryService.GetRecommendedColors(c.Request.Context(), userID, count)
	if err != nil {
		h.logger.Error("Failed to get color recommendations", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get recommendations"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"recommendations": recommendations})
}

// GetColorInfo retrieves detailed information about a color
// @Summary Get color information
// @Description Get detailed information about a color including WCAG compliance
// @Tags Categories
// @Accept json
// @Produce json
// @Param color query string true "Color in hex or HSL format"
// @Success 200 {object} category.ColorPaletteInfo "Color information"
// @Failure 400 {object} map[string]string "Invalid color format"
// @Router /categories/colors/info [get]
func (h *CategoryHandler) GetColorInfo(c *gin.Context) {
	colorStr := c.Query("color")
	if colorStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "color parameter is required"})
		return
	}

	colorInfo, err := h.categoryService.GetColorInfo(colorStr)
	if err != nil {
		h.logger.Warn("Invalid color format", zap.String("color", colorStr), zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid color format"})
		return
	}

	c.JSON(http.StatusOK, colorInfo)
}
