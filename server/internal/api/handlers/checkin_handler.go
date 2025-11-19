package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/dev-jelly/donelist/internal/api/middleware"
	"github.com/dev-jelly/donelist/internal/checkin"
	"go.uber.org/zap"
)

// CheckinHandler handles check-in endpoints
type CheckinHandler struct {
	checkinService *checkin.Service
	logger         *zap.Logger
}

// NewCheckinHandler creates a new checkin handler
func NewCheckinHandler(checkinService *checkin.Service, logger *zap.Logger) *CheckinHandler {
	return &CheckinHandler{
		checkinService: checkinService,
		logger:         logger,
	}
}

// CreateRequest represents check-in creation request
type CreateCheckinRequest struct {
	CategoryID      *uuid.UUID `json:"category_id"`
	Content         string     `json:"content" binding:"required,min=1,max=500"`
	CheckinTime     *time.Time `json:"checkin_time"`
	DurationMinutes int        `json:"duration_minutes" binding:"required,oneof=15 30 45 120"`
	Tags            []string   `json:"tags"`
}

// UpdateCheckinRequest represents check-in update request
type UpdateCheckinRequest struct {
	Content    *string    `json:"content" binding:"omitempty,min=1,max=500"`
	CategoryID *uuid.UUID `json:"category_id"`
	Tags       []string   `json:"tags"`
	EditReason *string    `json:"edit_reason" binding:"omitempty,max=200"` // Optional reason for audit trail
	Version    int        `json:"version" binding:"required,min=0"`         // Current version for optimistic locking
}

// Create creates a new check-in
// @Summary Create a new check-in
// @Description Create a new check-in entry with content, category, and tags. Duration must be one of: 15, 30, 45, or 120 minutes
// @Tags Check-ins
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateCheckinRequest true "Check-in details"
// @Success 201 {object} checkin.Checkin "Created check-in"
// @Failure 400 {object} map[string]string "Invalid request body"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 409 {object} map[string]interface{} "Duplicate check-in error"
// @Failure 429 {object} map[string]interface{} "Check-in interval violation"
// @Router /checkins [post]
func (h *CheckinHandler) Create(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req CreateCheckinRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Invalid create request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Use current time (UTC) if not provided
	checkinTime := time.Now().UTC()
	if req.CheckinTime != nil {
		checkinTime = *req.CheckinTime
	}

	newCheckin, err := h.checkinService.Create(c.Request.Context(), checkin.CreateInput{
		UserID:          userID,
		CategoryID:      req.CategoryID,
		Content:         req.Content,
		CheckinTime:     checkinTime,
		DurationMinutes: req.DurationMinutes,
		TagNames:        req.Tags,
	})
	if err != nil {
		// Check if it's a duplicate check-in error
		if dupErr, ok := err.(*checkin.DuplicateCheckinError); ok {
			h.logger.Warn("Duplicate check-in attempt",
				zap.String("user_id", userID.String()),
				zap.Time("checkin_time", dupErr.CheckinTime),
			)
			c.JSON(http.StatusConflict, dupErr.ToAPIResponse())
			return
		}

		// Check if it's a check-in interval error
		if intervalErr, ok := err.(*checkin.CheckinIntervalError); ok {
			h.logger.Warn("Check-in interval violation",
				zap.String("user_id", userID.String()),
				zap.Time("last_checkin_time", intervalErr.LastCheckinTime),
				zap.Duration("requested_interval", intervalErr.RequestedInterval),
			)
			c.JSON(http.StatusTooManyRequests, intervalErr.ToAPIResponse())
			return
		}

		h.logger.Error("Failed to create check-in", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, newCheckin)
}

// GetByID retrieves a check-in by ID
// @Summary Get check-in by ID
// @Description Retrieve a specific check-in by its ID
// @Tags Check-ins
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Check-in ID (UUID)"
// @Success 200 {object} checkin.Checkin "Check-in details"
// @Failure 400 {object} map[string]string "Invalid checkin ID"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "Check-in not found"
// @Router /checkins/{id} [get]
func (h *CheckinHandler) GetByID(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid checkin id"})
		return
	}

	foundCheckin, err := h.checkinService.GetByID(c.Request.Context(), id, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "check-in not found"})
		return
	}

	c.JSON(http.StatusOK, foundCheckin)
}

// List retrieves check-ins with filtering
// @Summary List check-ins
// @Description Retrieve a paginated list of check-ins with optional filtering by date range and category
// @Tags Check-ins
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param start_date query string false "Start date (RFC3339 format)"
// @Param end_date query string false "End date (RFC3339 format)"
// @Param category_id query string false "Category ID (UUID)"
// @Param limit query int false "Number of items to return (default: 50)"
// @Param offset query int false "Number of items to skip (default: 0)"
// @Success 200 {object} map[string]interface{} "checkins, total, limit, offset"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Failed to list check-ins"
// @Router /checkins [get]
func (h *CheckinHandler) List(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Parse query parameters
	var startDate, endDate *time.Time
	if startStr := c.Query("start_date"); startStr != "" {
		if parsed, err := time.Parse(time.RFC3339, startStr); err == nil {
			startDate = &parsed
		}
	}
	if endStr := c.Query("end_date"); endStr != "" {
		if parsed, err := time.Parse(time.RFC3339, endStr); err == nil {
			endDate = &parsed
		}
	}

	var categoryID *uuid.UUID
	if catStr := c.Query("category_id"); catStr != "" {
		if parsed, err := uuid.Parse(catStr); err == nil {
			categoryID = &parsed
		}
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	checkins, total, err := h.checkinService.List(c.Request.Context(), userID, startDate, endDate, categoryID, limit, offset)
	if err != nil {
		h.logger.Error("Failed to list check-ins", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list check-ins"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"checkins": checkins,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
	})
}

// Update updates a check-in with premium historical edit support
// @Summary Update check-in (Premium historical edit)
// @Description Update an existing check-in's content, category, or tags. Free users can edit within 2 hours; Premium users can edit any time. Includes optimistic locking, audit logging, and edit history preservation.
// @Tags Check-ins
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Check-in ID (UUID)"
// @Param request body UpdateCheckinRequest true "Update details (requires version for concurrent edit protection)"
// @Success 200 {object} checkin.Checkin "Updated check-in with incremented version"
// @Failure 400 {object} map[string]string "Invalid request"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 403 {object} map[string]interface{} "Edit permission denied (requires premium for historical edits)"
// @Failure 404 {object} map[string]string "Check-in not found"
// @Failure 409 {object} map[string]interface{} "Concurrent edit detected (version mismatch)"
// @Failure 422 {object} map[string]string "Validation error"
// @Router /checkins/{id} [patch]
func (h *CheckinHandler) Update(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid checkin id"})
		return
	}

	var req UpdateCheckinRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Invalid update request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Perform the update with all integrated features:
	// - Subscription/tier validation (in service layer)
	// - Time boundary validation (in service layer)
	// - Optimistic locking via version
	// - Original preservation in edit_history table
	// - Edit reason for audit trail
	updated, err := h.checkinService.Update(c.Request.Context(), id, userID, checkin.UpdateInput{
		Content:    req.Content,
		CategoryID: req.CategoryID,
		TagNames:   req.Tags,
		EditReason: req.EditReason,
		Version:    req.Version,
	})
	if err != nil {
		// Check if it's an edit permission error (time boundary + tier validation)
		if editErr, ok := err.(*checkin.EditPermissionError); ok {
			h.logger.Warn("Edit permission denied",
				zap.String("checkin_id", id.String()),
				zap.String("user_id", userID.String()),
				zap.String("user_tier", string(editErr.UserTier)),
			)
			c.JSON(http.StatusForbidden, editErr.ToAPIResponse())
			return
		}

		// Check for concurrent edit (optimistic locking failure)
		if err.Error() == "concurrent edit detected: please refresh and try again" {
			h.logger.Warn("Concurrent edit attempt",
				zap.String("checkin_id", id.String()),
				zap.String("user_id", userID.String()),
				zap.Int("attempted_version", req.Version),
			)
			c.JSON(http.StatusConflict, gin.H{
				"error":   "concurrent_edit_detected",
				"message": "This check-in was modified by another request. Please refresh and try again.",
				"checkin_id": id.String(),
			})
			return
		}

		// Check for not found
		if err.Error() == "check-in not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "check-in not found"})
			return
		}

		// Validation errors
		h.logger.Error("Failed to update check-in", zap.Error(err))
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	// Log successful edit to structured logger
	h.logger.Info("Check-in updated successfully",
		zap.String("checkin_id", id.String()),
		zap.String("user_id", userID.String()),
		zap.Int("old_version", req.Version),
		zap.Int("new_version", updated.Version),
		zap.Bool("has_edit_reason", req.EditReason != nil),
	)

	c.JSON(http.StatusOK, updated)
}

// Delete deletes a check-in
// @Summary Delete check-in
// @Description Permanently delete a check-in entry
// @Tags Check-ins
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Check-in ID (UUID)"
// @Success 200 {object} map[string]string "message"
// @Failure 400 {object} map[string]string "Invalid checkin ID"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Failed to delete check-in"
// @Router /checkins/{id} [delete]
func (h *CheckinHandler) Delete(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid checkin id"})
		return
	}

	if err := h.checkinService.Delete(c.Request.Context(), id, userID); err != nil {
		h.logger.Error("Failed to delete check-in", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete check-in"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "check-in deleted successfully"})
}

// GetEditHistory retrieves edit history for a check-in (Premium feature)
// @Summary Get check-in edit history
// @Description Retrieve the edit history for a specific check-in (Premium feature)
// @Tags Check-ins
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Check-in ID (UUID)"
// @Success 200 {object} map[string]interface{} "history"
// @Failure 400 {object} map[string]string "Invalid checkin ID"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 403 {object} map[string]string "Permission denied or premium feature required"
// @Router /checkins/{id}/history [get]
func (h *CheckinHandler) GetEditHistory(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid checkin id"})
		return
	}

	history, err := h.checkinService.GetEditHistory(c.Request.Context(), id, userID)
	if err != nil {
		h.logger.Error("Failed to get edit history", zap.Error(err))
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"history": history})
}
