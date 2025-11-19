package handlers

import (
	"net/http"

	"github.com/dev-jelly/donelist/internal/team"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// TeamHandler handles team-related HTTP requests
type TeamHandler struct {
	teamService *team.Service
	logger      *zap.Logger
}

// NewTeamHandler creates a new team handler
func NewTeamHandler(teamService *team.Service, logger *zap.Logger) *TeamHandler {
	return &TeamHandler{
		teamService: teamService,
		logger:      logger,
	}
}

// CreateTeamRequest represents the request body for creating a team
type CreateTeamRequest struct {
	Name        string  `json:"name" binding:"required"`
	Description *string `json:"description"`
}

// CreateTeam handles POST /api/v1/teams
func (h *TeamHandler) CreateTeam(c *gin.Context) {
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

	var req CreateTeamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	team, err := h.teamService.CreateTeam(c.Request.Context(), req.Name, req.Description, uid)
	if err != nil {
		h.logger.Error("Failed to create team", zap.Error(err), zap.String("user_id", uid.String()))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, team)
}

// GetTeam handles GET /api/v1/teams/:id
func (h *TeamHandler) GetTeam(c *gin.Context) {
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

	teamIDStr := c.Param("id")
	teamID, err := uuid.Parse(teamIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid team ID"})
		return
	}

	team, err := h.teamService.GetTeam(c.Request.Context(), teamID, uid)
	if err != nil {
		h.logger.Error("Failed to get team", zap.Error(err), zap.String("team_id", teamID.String()))
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, team)
}

// UpdateTeamRequest represents the request body for updating a team
type UpdateTeamRequest struct {
	Name        *string         `json:"name"`
	Description *string         `json:"description"`
	Settings    *team.Settings `json:"settings"`
}

// UpdateTeam handles PATCH /api/v1/teams/:id
func (h *TeamHandler) UpdateTeam(c *gin.Context) {
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

	teamIDStr := c.Param("id")
	teamID, err := uuid.Parse(teamIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid team ID"})
		return
	}

	var req UpdateTeamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updated, err := h.teamService.UpdateTeam(c.Request.Context(), teamID, uid, team.UpdateTeamInput{
		Name:        req.Name,
		Description: req.Description,
		Settings:    req.Settings,
	})
	if err != nil {
		h.logger.Error("Failed to update team", zap.Error(err), zap.String("team_id", teamID.String()))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, updated)
}

// DeleteTeam handles DELETE /api/v1/teams/:id
func (h *TeamHandler) DeleteTeam(c *gin.Context) {
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

	teamIDStr := c.Param("id")
	teamID, err := uuid.Parse(teamIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid team ID"})
		return
	}

	err = h.teamService.DeleteTeam(c.Request.Context(), teamID, uid)
	if err != nil {
		h.logger.Error("Failed to delete team", zap.Error(err), zap.String("team_id", teamID.String()))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "team deleted successfully"})
}

// GetUserTeams handles GET /api/v1/teams
func (h *TeamHandler) GetUserTeams(c *gin.Context) {
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"teams": teams})
}

// AddMemberRequest represents the request body for adding a team member
type AddMemberRequest struct {
	UserID uuid.UUID     `json:"user_id" binding:"required"`
	Role   team.MemberRole `json:"role" binding:"required"`
}

// AddMember handles POST /api/v1/teams/:id/members
func (h *TeamHandler) AddMember(c *gin.Context) {
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

	teamIDStr := c.Param("id")
	teamID, err := uuid.Parse(teamIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid team ID"})
		return
	}

	var req AddMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	member, err := h.teamService.AddMember(c.Request.Context(), teamID, uid, req.UserID, req.Role)
	if err != nil {
		h.logger.Error("Failed to add team member",
			zap.Error(err),
			zap.String("team_id", teamID.String()),
			zap.String("new_member_id", req.UserID.String()),
		)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, member)
}

// RemoveMember handles DELETE /api/v1/teams/:id/members/:user_id
func (h *TeamHandler) RemoveMember(c *gin.Context) {
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

	teamIDStr := c.Param("id")
	teamID, err := uuid.Parse(teamIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid team ID"})
		return
	}

	memberIDStr := c.Param("user_id")
	memberID, err := uuid.Parse(memberIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid member ID"})
		return
	}

	err = h.teamService.RemoveMember(c.Request.Context(), teamID, uid, memberID)
	if err != nil {
		h.logger.Error("Failed to remove team member",
			zap.Error(err),
			zap.String("team_id", teamID.String()),
			zap.String("member_id", memberID.String()),
		)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "member removed successfully"})
}

// UpdateMemberRoleRequest represents the request body for updating a member's role
type UpdateMemberRoleRequest struct {
	Role team.MemberRole `json:"role" binding:"required"`
}

// UpdateMemberRole handles PATCH /api/v1/teams/:id/members/:user_id
func (h *TeamHandler) UpdateMemberRole(c *gin.Context) {
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

	teamIDStr := c.Param("id")
	teamID, err := uuid.Parse(teamIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid team ID"})
		return
	}

	memberIDStr := c.Param("user_id")
	memberID, err := uuid.Parse(memberIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid member ID"})
		return
	}

	var req UpdateMemberRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	member, err := h.teamService.UpdateMemberRole(c.Request.Context(), teamID, uid, memberID, req.Role)
	if err != nil {
		h.logger.Error("Failed to update member role",
			zap.Error(err),
			zap.String("team_id", teamID.String()),
			zap.String("member_id", memberID.String()),
		)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, member)
}

// GetTeamMembers handles GET /api/v1/teams/:id/members
func (h *TeamHandler) GetTeamMembers(c *gin.Context) {
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

	teamIDStr := c.Param("id")
	teamID, err := uuid.Parse(teamIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid team ID"})
		return
	}

	members, err := h.teamService.GetActiveTeamMembers(c.Request.Context(), teamID, uid)
	if err != nil {
		h.logger.Error("Failed to get team members", zap.Error(err), zap.String("team_id", teamID.String()))
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"members": members})
}

// TransferOwnershipRequest represents the request body for transferring ownership
type TransferOwnershipRequest struct {
	NewOwnerID uuid.UUID `json:"new_owner_id" binding:"required"`
}

// TransferOwnership handles POST /api/v1/teams/:id/transfer-ownership
func (h *TeamHandler) TransferOwnership(c *gin.Context) {
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

	teamIDStr := c.Param("id")
	teamID, err := uuid.Parse(teamIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid team ID"})
		return
	}

	var req TransferOwnershipRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err = h.teamService.TransferOwnership(c.Request.Context(), teamID, uid, req.NewOwnerID)
	if err != nil {
		h.logger.Error("Failed to transfer ownership",
			zap.Error(err),
			zap.String("team_id", teamID.String()),
			zap.String("new_owner_id", req.NewOwnerID.String()),
		)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "ownership transferred successfully"})
}
