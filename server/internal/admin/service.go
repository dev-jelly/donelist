package admin

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/dev-jelly/donelist/internal/user"
	"go.uber.org/zap"
)

// Service handles admin operations
type Service struct {
	userRepo  *user.Repository
	auditRepo *AuditRepository
	logger    *zap.Logger
}

// NewService creates a new admin service
func NewService(
	userRepo *user.Repository,
	auditRepo *AuditRepository,
	logger *zap.Logger,
) *Service {
	return &Service{
		userRepo:  userRepo,
		auditRepo: auditRepo,
		logger:    logger,
	}
}

// UpdateUserRole updates a user's role (admin operation)
func (s *Service) UpdateUserRole(ctx context.Context, adminID, targetUserID uuid.UUID, newRole user.UserRole) error {
	// Validate role
	if newRole != user.RoleUser && newRole != user.RoleAdmin && newRole != user.RoleSuperAdmin {
		return fmt.Errorf("invalid role: %s", newRole)
	}

	// Get target user
	targetUser, err := s.userRepo.GetByID(ctx, targetUserID)
	if err != nil {
		return fmt.Errorf("failed to get target user: %w", err)
	}

	oldRole := targetUser.Role

	// Update role
	if err := s.userRepo.UpdateRole(ctx, targetUserID, newRole, adminID); err != nil {
		s.logger.Error("Failed to update user role",
			zap.Error(err),
			zap.String("admin_id", adminID.String()),
			zap.String("target_user_id", targetUserID.String()),
			zap.String("new_role", string(newRole)),
		)
		return fmt.Errorf("failed to update role: %w", err)
	}

	// Log audit trail
	if err := s.auditRepo.Log(ctx, AuditLogInput{
		AdminID:      adminID,
		Action:       "update_role",
		ResourceType: "user",
		ResourceID:   &targetUserID,
		TargetUserID: &targetUserID,
		Metadata: map[string]interface{}{
			"old_role": oldRole,
			"new_role": string(newRole),
		},
	}); err != nil {
		s.logger.Warn("Failed to log role update audit",
			zap.Error(err),
			zap.String("admin_id", adminID.String()),
			zap.String("target_user_id", targetUserID.String()),
		)
	}

	s.logger.Info("User role updated",
		zap.String("admin_id", adminID.String()),
		zap.String("target_user_id", targetUserID.String()),
		zap.String("old_role", oldRole),
		zap.String("new_role", string(newRole)),
	)

	return nil
}

// GetUserByID retrieves a user by ID (admin operation)
func (s *Service) GetUserByID(ctx context.Context, adminID, targetUserID uuid.UUID) (*user.User, error) {
	u, err := s.userRepo.GetByID(ctx, targetUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Log audit trail for viewing user details
	if err := s.auditRepo.Log(ctx, AuditLogInput{
		AdminID:      adminID,
		Action:       "view_user",
		ResourceType: "user",
		ResourceID:   &targetUserID,
		TargetUserID: &targetUserID,
	}); err != nil {
		s.logger.Warn("Failed to log user view audit",
			zap.Error(err),
			zap.String("admin_id", adminID.String()),
			zap.String("target_user_id", targetUserID.String()),
		)
	}

	return u, nil
}

// DeleteUser soft deletes a user (admin operation)
func (s *Service) DeleteUser(ctx context.Context, adminID, targetUserID uuid.UUID) error {
	// Get user details before deletion for audit
	targetUser, err := s.userRepo.GetByID(ctx, targetUserID)
	if err != nil {
		return fmt.Errorf("failed to get target user: %w", err)
	}

	// Delete user
	if err := s.userRepo.Delete(ctx, targetUserID); err != nil {
		s.logger.Error("Failed to delete user",
			zap.Error(err),
			zap.String("admin_id", adminID.String()),
			zap.String("target_user_id", targetUserID.String()),
		)
		return fmt.Errorf("failed to delete user: %w", err)
	}

	// Log audit trail
	if err := s.auditRepo.Log(ctx, AuditLogInput{
		AdminID:      adminID,
		Action:       "delete_user",
		ResourceType: "user",
		ResourceID:   &targetUserID,
		TargetUserID: &targetUserID,
		Metadata: map[string]interface{}{
			"email": targetUser.Email,
			"role":  targetUser.Role,
		},
	}); err != nil {
		s.logger.Warn("Failed to log user deletion audit",
			zap.Error(err),
			zap.String("admin_id", adminID.String()),
			zap.String("target_user_id", targetUserID.String()),
		)
	}

	s.logger.Info("User deleted",
		zap.String("admin_id", adminID.String()),
		zap.String("target_user_id", targetUserID.String()),
		zap.String("target_email", targetUser.Email),
	)

	return nil
}

// GetAuditLogs retrieves audit logs with pagination
func (s *Service) GetAuditLogs(ctx context.Context, startDate, endDate time.Time, limit, offset int) ([]AuditLog, error) {
	return s.auditRepo.GetByDateRange(ctx, startDate, endDate, limit, offset)
}

// GetAuditLogsByAdmin retrieves audit logs for a specific admin
func (s *Service) GetAuditLogsByAdmin(ctx context.Context, adminID uuid.UUID, limit, offset int) ([]AuditLog, error) {
	return s.auditRepo.GetByAdminID(ctx, adminID, limit, offset)
}

// GetAuditLogsByTargetUser retrieves audit logs affecting a specific user
func (s *Service) GetAuditLogsByTargetUser(ctx context.Context, targetUserID uuid.UUID, limit, offset int) ([]AuditLog, error) {
	return s.auditRepo.GetByTargetUserID(ctx, targetUserID, limit, offset)
}
