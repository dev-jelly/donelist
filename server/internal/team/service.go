package team

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Service handles team business logic
type Service struct {
	repo   *Repository
	logger *zap.Logger
}

// NewService creates a new team service
func NewService(repo *Repository, logger *zap.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
	}
}

// CreateTeam creates a new team with the user as owner
func (s *Service) CreateTeam(ctx context.Context, name string, description *string, ownerID uuid.UUID) (*Team, error) {
	if name == "" {
		return nil, fmt.Errorf("team name is required")
	}

	// Default settings
	settings := Settings{
		AllowPublicCheckins: false,
		DefaultVisibility:   "team",
		RequireApproval:     false,
		TimeZone:            "UTC",
	}

	team, err := s.repo.CreateTeam(ctx, CreateTeamInput{
		Name:        name,
		Description: description,
		OwnerID:     ownerID,
		Settings:    settings,
	})
	if err != nil {
		return nil, err
	}

	// Automatically add owner as a team member with owner role
	_, err = s.repo.AddMember(ctx, team.ID, ownerID, RoleOwner)
	if err != nil {
		s.logger.Error("Failed to add owner as team member",
			zap.Error(err),
			zap.String("team_id", team.ID.String()),
			zap.String("owner_id", ownerID.String()),
		)
		// Continue even if adding owner fails - team is still created
	}

	return team, nil
}

// GetTeam retrieves a team by ID
func (s *Service) GetTeam(ctx context.Context, teamID, userID uuid.UUID) (*Team, error) {
	// Check if user has access to this team
	hasAccess, err := s.HasTeamAccess(ctx, teamID, userID)
	if err != nil {
		return nil, err
	}

	if !hasAccess {
		s.logger.Warn("Access denied to team",
			zap.String("team_id", teamID.String()),
			zap.String("user_id", userID.String()),
		)
		return nil, fmt.Errorf("access denied: user is not a member of this team")
	}

	return s.repo.GetTeamByID(ctx, teamID)
}

// UpdateTeam updates team details
func (s *Service) UpdateTeam(ctx context.Context, teamID, userID uuid.UUID, input UpdateTeamInput) (*Team, error) {
	// Check if user can edit team settings
	canEdit, err := s.CanEditTeamSettings(ctx, teamID, userID)
	if err != nil {
		return nil, err
	}

	if !canEdit {
		s.logger.Warn("User cannot edit team settings",
			zap.String("team_id", teamID.String()),
			zap.String("user_id", userID.String()),
		)
		return nil, fmt.Errorf("permission denied: only owners and admins can edit team settings")
	}

	return s.repo.UpdateTeam(ctx, teamID, input)
}

// DeleteTeam deletes a team (only owner can delete)
func (s *Service) DeleteTeam(ctx context.Context, teamID, userID uuid.UUID) error {
	// Only owner can delete team
	isOwner, err := s.repo.IsTeamOwner(ctx, teamID, userID)
	if err != nil {
		return err
	}

	if !isOwner {
		s.logger.Warn("User cannot delete team",
			zap.String("team_id", teamID.String()),
			zap.String("user_id", userID.String()),
		)
		return fmt.Errorf("permission denied: only the team owner can delete the team")
	}

	return s.repo.DeleteTeam(ctx, teamID)
}

// GetUserTeams retrieves all teams for a user
func (s *Service) GetUserTeams(ctx context.Context, userID uuid.UUID) ([]Team, error) {
	return s.repo.GetUserTeams(ctx, userID)
}

// AddMember adds a new member to the team
func (s *Service) AddMember(ctx context.Context, teamID, userID, newMemberID uuid.UUID, role MemberRole) (*TeamMember, error) {
	// Validate role
	if !role.IsValid() {
		return nil, fmt.Errorf("invalid role: %s", role)
	}

	// Check if user can manage members
	canManage, err := s.CanManageMembers(ctx, teamID, userID)
	if err != nil {
		return nil, err
	}

	if !canManage {
		s.logger.Warn("User cannot manage team members",
			zap.String("team_id", teamID.String()),
			zap.String("user_id", userID.String()),
		)
		return nil, fmt.Errorf("permission denied: only owners and admins can manage team members")
	}

	// Check if trying to add owner role (only one owner per team)
	if role == RoleOwner {
		s.logger.Warn("Cannot add additional owner",
			zap.String("team_id", teamID.String()),
			zap.String("user_id", userID.String()),
		)
		return nil, fmt.Errorf("cannot add additional owner: team can only have one owner")
	}

	// Check if member already exists
	existing, err := s.repo.GetTeamMember(ctx, teamID, newMemberID)
	if err == nil && existing != nil {
		return nil, fmt.Errorf("user is already a member of this team")
	}

	return s.repo.AddMember(ctx, teamID, newMemberID, role)
}

// RemoveMember removes a member from the team
func (s *Service) RemoveMember(ctx context.Context, teamID, userID, memberID uuid.UUID) error {
	// Check if user can manage members
	canManage, err := s.CanManageMembers(ctx, teamID, userID)
	if err != nil {
		return err
	}

	if !canManage {
		s.logger.Warn("User cannot manage team members",
			zap.String("team_id", teamID.String()),
			zap.String("user_id", userID.String()),
		)
		return fmt.Errorf("permission denied: only owners and admins can manage team members")
	}

	// Cannot remove the owner
	isOwner, err := s.repo.IsTeamOwner(ctx, teamID, memberID)
	if err != nil {
		return err
	}

	if isOwner {
		s.logger.Warn("Cannot remove team owner",
			zap.String("team_id", teamID.String()),
			zap.String("user_id", userID.String()),
		)
		return fmt.Errorf("cannot remove team owner: transfer ownership first")
	}

	return s.repo.RemoveMember(ctx, teamID, memberID)
}

// UpdateMemberRole updates a team member's role
func (s *Service) UpdateMemberRole(ctx context.Context, teamID, userID, memberID uuid.UUID, newRole MemberRole) (*TeamMember, error) {
	// Validate role
	if !newRole.IsValid() {
		return nil, fmt.Errorf("invalid role: %s", newRole)
	}

	// Check if user can manage members
	canManage, err := s.CanManageMembers(ctx, teamID, userID)
	if err != nil {
		return nil, err
	}

	if !canManage {
		s.logger.Warn("User cannot manage team members",
			zap.String("team_id", teamID.String()),
			zap.String("user_id", userID.String()),
		)
		return nil, fmt.Errorf("permission denied: only owners and admins can manage team members")
	}

	// Cannot change owner role
	isOwner, err := s.repo.IsTeamOwner(ctx, teamID, memberID)
	if err != nil {
		return nil, err
	}

	if isOwner {
		s.logger.Warn("Cannot change owner role",
			zap.String("team_id", teamID.String()),
			zap.String("user_id", userID.String()),
		)
		return nil, fmt.Errorf("cannot change owner role: transfer ownership instead")
	}

	// Cannot promote to owner
	if newRole == RoleOwner {
		s.logger.Warn("Cannot promote to owner",
			zap.String("team_id", teamID.String()),
			zap.String("user_id", userID.String()),
		)
		return nil, fmt.Errorf("cannot promote to owner: use transfer ownership instead")
	}

	return s.repo.UpdateMemberRole(ctx, teamID, memberID, newRole)
}

// GetTeamMembers retrieves all members of a team
func (s *Service) GetTeamMembers(ctx context.Context, teamID, userID uuid.UUID) ([]TeamMember, error) {
	// Check if user has access to this team
	hasAccess, err := s.HasTeamAccess(ctx, teamID, userID)
	if err != nil {
		return nil, err
	}

	if !hasAccess {
		s.logger.Warn("Access denied to team members",
			zap.String("team_id", teamID.String()),
			zap.String("user_id", userID.String()),
		)
		return nil, fmt.Errorf("access denied: user is not a member of this team")
	}

	return s.repo.GetTeamMembers(ctx, teamID)
}

// GetActiveTeamMembers retrieves active team members with user details
func (s *Service) GetActiveTeamMembers(ctx context.Context, teamID, userID uuid.UUID) ([]ActiveTeamMember, error) {
	// Check if user has access to this team
	hasAccess, err := s.HasTeamAccess(ctx, teamID, userID)
	if err != nil {
		return nil, err
	}

	if !hasAccess {
		s.logger.Warn("Access denied to team members",
			zap.String("team_id", teamID.String()),
			zap.String("user_id", userID.String()),
		)
		return nil, fmt.Errorf("access denied: user is not a member of this team")
	}

	return s.repo.GetActiveTeamMembers(ctx, teamID)
}

// GetMemberRole retrieves a user's role in a team
func (s *Service) GetMemberRole(ctx context.Context, teamID, userID uuid.UUID) (MemberRole, error) {
	member, err := s.repo.GetTeamMember(ctx, teamID, userID)
	if err != nil {
		return "", err
	}

	return member.Role, nil
}

// HasTeamAccess checks if a user has access to a team
func (s *Service) HasTeamAccess(ctx context.Context, teamID, userID uuid.UUID) (bool, error) {
	// Check if user is owner
	isOwner, err := s.repo.IsTeamOwner(ctx, teamID, userID)
	if err != nil {
		return false, err
	}

	if isOwner {
		return true, nil
	}

	// Check if user is a member
	_, err = s.repo.GetTeamMember(ctx, teamID, userID)
	if err != nil {
		return false, nil // Not a member
	}

	return true, nil
}

// CanManageMembers checks if a user can manage team members
func (s *Service) CanManageMembers(ctx context.Context, teamID, userID uuid.UUID) (bool, error) {
	role, err := s.GetMemberRole(ctx, teamID, userID)
	if err != nil {
		return false, err
	}

	return role.CanManageMembers(), nil
}

// CanEditTeamSettings checks if a user can edit team settings
func (s *Service) CanEditTeamSettings(ctx context.Context, teamID, userID uuid.UUID) (bool, error) {
	role, err := s.GetMemberRole(ctx, teamID, userID)
	if err != nil {
		return false, err
	}

	return role.CanEditTeamSettings(), nil
}

// CanViewAllCheckins checks if a user can view all team checkins
func (s *Service) CanViewAllCheckins(ctx context.Context, teamID, userID uuid.UUID) (bool, error) {
	role, err := s.GetMemberRole(ctx, teamID, userID)
	if err != nil {
		return false, err
	}

	return role.CanViewAllCheckins(), nil
}

// TransferOwnership transfers team ownership to another member
func (s *Service) TransferOwnership(ctx context.Context, teamID, currentOwnerID, newOwnerID uuid.UUID) error {
	// Verify current owner
	isOwner, err := s.repo.IsTeamOwner(ctx, teamID, currentOwnerID)
	if err != nil {
		return err
	}

	if !isOwner {
		s.logger.Warn("User is not team owner",
			zap.String("team_id", teamID.String()),
			zap.String("user_id", currentOwnerID.String()),
		)
		return fmt.Errorf("permission denied: only the team owner can transfer ownership")
	}

	// Verify new owner is a member
	newMember, err := s.repo.GetTeamMember(ctx, teamID, newOwnerID)
	if err != nil {
		return fmt.Errorf("new owner must be a team member first")
	}

	// Update team owner
	team, err := s.repo.GetTeamByID(ctx, teamID)
	if err != nil {
		return err
	}

	ownerID := newOwnerID
	_, err = s.repo.UpdateTeam(ctx, teamID, UpdateTeamInput{
		Name:        &team.Name,
		Description: team.Description,
		Settings:    &team.Settings,
	})
	if err != nil {
		return err
	}

	// Update roles: new owner to owner, current owner to admin
	_, err = s.repo.UpdateMemberRole(ctx, teamID, newOwnerID, RoleOwner)
	if err != nil {
		return err
	}

	// Demote current owner to admin
	currentMember, err := s.repo.GetTeamMember(ctx, teamID, currentOwnerID)
	if err == nil && currentMember != nil {
		_, err = s.repo.UpdateMemberRole(ctx, teamID, currentOwnerID, RoleAdmin)
		if err != nil {
			s.logger.Warn("Failed to demote previous owner to admin",
				zap.Error(err),
				zap.String("team_id", teamID.String()),
				zap.String("user_id", currentOwnerID.String()),
			)
		}
	}

	s.logger.Info("Ownership transferred",
		zap.String("team_id", teamID.String()),
		zap.String("from", currentOwnerID.String()),
		zap.String("to", ownerID.String()),
		zap.String("new_member_role", string(newMember.Role)),
	)

	return nil
}
