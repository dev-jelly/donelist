package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/dev-jelly/donelist/internal/api/middleware"
	"github.com/dev-jelly/donelist/internal/user"
	"go.uber.org/zap"
)

// UserHandler handles user endpoints
type UserHandler struct {
	userService *user.Service
	logger      *zap.Logger
}

// NewUserHandler creates a new user handler
func NewUserHandler(userService *user.Service, logger *zap.Logger) *UserHandler {
	return &UserHandler{
		userService: userService,
		logger:      logger,
	}
}

// UpdateRequest represents user update request body
type UpdateRequest struct {
	DisplayName *string `json:"display_name" binding:"omitempty,max=100"`
}

// GetMe retrieves the current user's profile
// @Summary Get current user profile
// @Description Retrieve the authenticated user's profile information
// @Tags Users
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} user.User "User profile"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "User not found"
// @Router /users/me [get]
func (h *UserHandler) GetMe(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "unauthorized",
		})
		return
	}

	currentUser, err := h.userService.GetByID(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get current user", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{
			"error": "user not found",
		})
		return
	}

	c.JSON(http.StatusOK, currentUser)
}

// UpdateMe updates the current user's profile
// @Summary Update current user profile
// @Description Update the authenticated user's profile information
// @Tags Users
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body UpdateRequest true "User update details"
// @Success 200 {object} user.User "Updated user profile"
// @Failure 400 {object} map[string]string "Invalid request body"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Failed to update user"
// @Router /users/me [patch]
func (h *UserHandler) UpdateMe(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "unauthorized",
		})
		return
	}

	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Invalid update request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
		})
		return
	}

	updatedUser, err := h.userService.Update(c.Request.Context(), userID, user.UpdateUserInput{
		DisplayName: req.DisplayName,
	})
	if err != nil {
		h.logger.Error("Failed to update user", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to update user",
		})
		return
	}

	c.JSON(http.StatusOK, updatedUser)
}

// DeleteMe deletes the current user's account
// @Summary Delete current user account
// @Description Permanently delete the authenticated user's account and all associated data
// @Tags Users
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]string "message"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Failed to delete user"
// @Router /users/me [delete]
func (h *UserHandler) DeleteMe(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "unauthorized",
		})
		return
	}

	if err := h.userService.Delete(c.Request.Context(), userID); err != nil {
		h.logger.Error("Failed to delete user", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to delete user",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "user deleted successfully",
	})
}
