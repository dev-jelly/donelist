package handlers

import (
	"net/http"

	"github.com/dev-jelly/donelist/internal/api/middleware"
	"github.com/dev-jelly/donelist/internal/category"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// CategoryMergeHandler handles category merge and batch operation endpoints
type CategoryMergeHandler struct {
	mergeService *category.MergeService
	logger       *zap.Logger
}

// NewCategoryMergeHandler creates a new category merge handler
func NewCategoryMergeHandler(db *sqlx.DB, categoryRepo category.RepositoryInterface, logger *zap.Logger) *CategoryMergeHandler {
	mergeService := category.NewMergeService(db, categoryRepo, logger)
	return &CategoryMergeHandler{
		mergeService: mergeService,
		logger:       logger,
	}
}

// MergeCategoriesRequest represents the request for merging categories
type MergeCategoriesRequest struct {
	SourceCategoryIDs []string `json:"source_category_ids" binding:"required,min=1"`
	TargetCategoryID  string   `json:"target_category_id" binding:"required"`
}

// BatchUpdateCategoriesRequest represents the request for batch updating categories
type BatchUpdateCategoriesRequest struct {
	CategoryIDs []string `json:"category_ids" binding:"required,min=1"`
	Updates     struct {
		Color *string `json:"color"`
		Icon  *string `json:"icon"`
	} `json:"updates" binding:"required"`
}

// BatchDeleteCategoriesRequest represents the request for batch deleting categories
type BatchDeleteCategoriesRequest struct {
	CategoryIDs            []string `json:"category_ids" binding:"required,min=1"`
	DeleteOrphanedCheckins bool     `json:"delete_orphaned_checkins"`
}

// MergeCategories merges multiple source categories into a target category
// @Summary Merge categories
// @Description Merge multiple source categories into a target category, updating all associated checkins
// @Tags Categories
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body MergeCategoriesRequest true "Merge details"
// @Success 200 {object} category.MergeResult "Merge result"
// @Failure 400 {object} map[string]string "Invalid request"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Router /categories/merge [post]
func (h *CategoryMergeHandler) MergeCategories(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req MergeCategoriesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Invalid merge request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Parse target category ID
	targetID, err := uuid.Parse(req.TargetCategoryID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target category ID"})
		return
	}

	// Parse source category IDs
	sourceIDs := make([]uuid.UUID, 0, len(req.SourceCategoryIDs))
	for _, idStr := range req.SourceCategoryIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid source category ID: " + idStr})
			return
		}
		sourceIDs = append(sourceIDs, id)
	}

	// Perform merge
	result, err := h.mergeService.MergeCategories(c.Request.Context(), category.MergeInput{
		SourceCategoryIDs: sourceIDs,
		TargetCategoryID:  targetID,
		UserID:            userID,
	})
	if err != nil {
		h.logger.Error("Failed to merge categories", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// BatchUpdateCategories updates multiple categories with the same values
// @Summary Batch update categories
// @Description Update multiple categories with the same color and/or icon values
// @Tags Categories
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body BatchUpdateCategoriesRequest true "Batch update details"
// @Success 200 {object} category.BatchUpdateResult "Update result"
// @Failure 400 {object} map[string]string "Invalid request"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Router /categories/batch-update [patch]
func (h *CategoryMergeHandler) BatchUpdateCategories(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req BatchUpdateCategoriesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Invalid batch update request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Validate that at least one update field is provided
	if req.Updates.Color == nil && req.Updates.Icon == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one field must be updated (color or icon)"})
		return
	}

	// Parse category IDs
	categoryIDs := make([]uuid.UUID, 0, len(req.CategoryIDs))
	for _, idStr := range req.CategoryIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid category ID: " + idStr})
			return
		}
		categoryIDs = append(categoryIDs, id)
	}

	// Perform batch update
	result, err := h.mergeService.BatchUpdateCategories(c.Request.Context(), category.BatchUpdateInput{
		CategoryIDs: categoryIDs,
		UserID:      userID,
		Updates: category.BatchUpdateFields{
			Color: req.Updates.Color,
			Icon:  req.Updates.Icon,
		},
	})
	if err != nil {
		h.logger.Error("Failed to batch update categories", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// BatchDeleteCategories deletes multiple categories
// @Summary Batch delete categories
// @Description Delete multiple categories and optionally their associated checkins
// @Tags Categories
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body BatchDeleteCategoriesRequest true "Batch delete details"
// @Success 200 {object} category.BatchDeleteResult "Delete result"
// @Failure 400 {object} map[string]string "Invalid request"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Router /categories/batch-delete [delete]
func (h *CategoryMergeHandler) BatchDeleteCategories(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req BatchDeleteCategoriesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Invalid batch delete request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Parse category IDs
	categoryIDs := make([]uuid.UUID, 0, len(req.CategoryIDs))
	for _, idStr := range req.CategoryIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid category ID: " + idStr})
			return
		}
		categoryIDs = append(categoryIDs, id)
	}

	// Perform batch delete
	result, err := h.mergeService.BatchDeleteCategories(c.Request.Context(), category.BatchDeleteInput{
		CategoryIDs:            categoryIDs,
		UserID:                 userID,
		DeleteOrphanedCheckins: req.DeleteOrphanedCheckins,
	})
	if err != nil {
		h.logger.Error("Failed to batch delete categories", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}