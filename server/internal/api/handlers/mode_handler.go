package handlers

import (
	"net/http"

	"github.com/dev-jelly/donelist/internal/mode"
	"github.com/dev-jelly/donelist/internal/team"
	"github.com/dev-jelly/donelist/internal/user"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ModeHandler handles mode-related HTTP requests
type ModeHandler struct {
	modeService *mode.Service
	teamService *team.Service
	userRepo    *user.Repository
	logger      *zap.Logger
}

// NewModeHandler creates a new mode handler
func NewModeHandler(
	modeService *mode.Service,
	teamService *team.Service,
	userRepo *user.Repository,
	logger *zap.Logger,
) *ModeHandler {
	return &ModeHandler{
		modeService: modeService,
		teamService: teamService,
		userRepo:    userRepo,
		logger:      logger,
	}
}

// SwitchModeRequest represents the request body for switching modes
type SwitchModeRequest struct {
	Mode   string     `json:"mode" binding:"required"` // "personal" or "team"
	TeamID *uuid.UUID `json:"team_id"`                 // Required when mode is "team"
}

// SwitchMode handles POST /api/v1/users/mode
func (h *ModeHandler) SwitchMode(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	uid, ok := userID.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user ID format"})
		return
	}

	var req SwitchModeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate mode
	newMode := mode.Mode(req.Mode)
	if !newMode.IsValid() {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_mode",
			"message": "mode must be 'personal' or 'team'",
		})
		return
	}

	// Validate team mode requirements
	if newMode == mode.ModeTeam {
		if req.TeamID == nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "team_id_required",
				"message": "team_id is required when switching to team mode",
			})
			return
		}

		// Verify user has access to the team
		hasAccess, err := h.teamService.HasTeamAccess(c.Request.Context(), *req.TeamID, uid)
		if err != nil {
			h.logger.Error("Failed to check team access",
				zap.Error(err),
				zap.String("user_id", uid.String()),
				zap.String("team_id", req.TeamID.String()),
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to verify team access"})
			return
		}

		if !hasAccess {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "team_access_denied",
				"message": "you are not a member of this team",
			})
			return
		}
	}

	// Switch mode using the mode service
	modeCtx, err := h.modeService.SwitchMode(c.Request.Context(), mode.SwitchModeRequest{
		UserID:  uid,
		NewMode: newMode,
		TeamID:  req.TeamID,
	})
	if err != nil {
		h.logger.Error("Failed to switch mode",
			zap.Error(err),
			zap.String("user_id", uid.String()),
			zap.String("mode", string(newMode)),
		)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update user's mode preference in database
	err = h.userRepo.UpdateModePreference(c.Request.Context(), uid, string(newMode))
	if err != nil {
		h.logger.Error("Failed to update mode preference",
			zap.Error(err),
			zap.String("user_id", uid.String()),
			zap.String("mode", string(newMode)),
		)
		// Don't fail the request - mode is switched in context even if DB update fails
	}

	response := gin.H{
		"message":      "mode switched successfully",
		"current_mode": string(modeCtx.CurrentMode),
		"user_id":      modeCtx.UserID.String(),
	}

	if modeCtx.TeamID != nil {
		response["team_id"] = modeCtx.TeamID.String()
	}

	c.JSON(http.StatusOK, response)
}

// GetCurrentMode handles GET /api/v1/users/mode
func (h *ModeHandler) GetCurrentMode(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	uid, ok := userID.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user ID format"})
		return
	}

	// Get user from database to retrieve mode preference
	user, err := h.userRepo.GetByID(c.Request.Context(), uid)
	if err != nil {
		h.logger.Error("Failed to get user", zap.Error(err), zap.String("user_id", uid.String()))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve user information"})
		return
	}

	// Get current mode from context (if set by middleware)
	currentMode := h.modeService.GetModeFromContext(c.Request.Context())
	teamID := h.modeService.GetTeamIDFromContext(c.Request.Context())

	response := gin.H{
		"user_id":         uid.String(),
		"mode_preference": user.ModePreference,
		"current_mode":    string(currentMode),
	}

	if teamID != nil {
		response["team_id"] = teamID.String()
	}

	c.JSON(http.StatusOK, response)
}

// GetAvailableTeams handles GET /api/v1/users/available-teams
func (h *ModeHandler) GetAvailableTeams(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	uid, ok := userID.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user ID format"})
		return
	}

	teams, err := h.teamService.GetUserTeams(c.Request.Context(), uid)
	if err != nil {
		h.logger.Error("Failed to get user teams", zap.Error(err), zap.String("user_id", uid.String()))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve teams"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"teams": teams,
		"count": len(teams),
	})
}
