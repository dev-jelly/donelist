package user

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Service handles user business logic
type Service struct {
	repo   *Repository
	logger *zap.Logger
}

// NewService creates a new user service
func NewService(repo *Repository, logger *zap.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
	}
}

// GetByID retrieves a user by ID
func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (*User, error) {
	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		s.logger.Warn("Failed to get user by ID",
			zap.String("user_id", id.String()),
			zap.Error(err),
		)
		return nil, fmt.Errorf("user not found")
	}

	return user, nil
}

// Update updates a user's profile
func (s *Service) Update(ctx context.Context, id uuid.UUID, input UpdateUserInput) (*User, error) {
	// Validate display name if provided
	if input.DisplayName != nil && len(*input.DisplayName) > 100 {
		return nil, fmt.Errorf("display name must be less than 100 characters")
	}

	user, err := s.repo.Update(ctx, id, input)
	if err != nil {
		s.logger.Error("Failed to update user",
			zap.String("user_id", id.String()),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to update user")
	}

	s.logger.Info("User updated successfully",
		zap.String("user_id", user.ID.String()),
	)

	return user, nil
}

// Delete soft deletes a user
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		s.logger.Error("Failed to delete user",
			zap.String("user_id", id.String()),
			zap.Error(err),
		)
		return fmt.Errorf("failed to delete user")
	}

	s.logger.Info("User deleted successfully",
		zap.String("user_id", id.String()),
	)

	return nil
}
