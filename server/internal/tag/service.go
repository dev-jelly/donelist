package tag

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Service handles tag business logic
type Service struct {
	repo   *Repository
	logger *zap.Logger
}

// NewService creates a new tag service
func NewService(repo *Repository, logger *zap.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
	}
}

// CreateInput represents service-level create input
type CreateInput struct {
	UserID uuid.UUID
	Name   string
}

// Create creates a new tag
func (s *Service) Create(ctx context.Context, input CreateInput) (*Tag, error) {
	// Validate name
	if input.Name == "" {
		return nil, fmt.Errorf("name cannot be empty")
	}
	if len(input.Name) > 30 {
		return nil, fmt.Errorf("name must be less than 30 characters")
	}

	// Check if name already exists
	exists, err := s.repo.NameExists(ctx, input.UserID, input.Name)
	if err != nil {
		s.logger.Error("Failed to check tag name", zap.Error(err))
		return nil, fmt.Errorf("failed to check tag name")
	}
	if exists {
		return nil, fmt.Errorf("tag with this name already exists")
	}

	tag, err := s.repo.Create(ctx, CreateTagInput{
		UserID: input.UserID,
		Name:   input.Name,
	})
	if err != nil {
		s.logger.Error("Failed to create tag", zap.Error(err))
		return nil, fmt.Errorf("failed to create tag")
	}

	s.logger.Info("Tag created",
		zap.String("tag_id", tag.ID.String()),
		zap.String("user_id", input.UserID.String()),
	)

	return tag, nil
}

// GetByID retrieves a tag by ID
func (s *Service) GetByID(ctx context.Context, id, userID uuid.UUID) (*Tag, error) {
	tag, err := s.repo.GetByID(ctx, id, userID)
	if err != nil {
		return nil, fmt.Errorf("tag not found")
	}

	return tag, nil
}

// List retrieves all tags for a user
func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]*Tag, error) {
	tags, err := s.repo.List(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to list tags", zap.Error(err))
		return nil, fmt.Errorf("failed to list tags")
	}

	return tags, nil
}

// Autocomplete searches for tags matching the given query with autocomplete suggestions
// Uses intelligent ranking: prefix match > trigram similarity > usage frequency
func (s *Service) Autocomplete(ctx context.Context, userID uuid.UUID, query string, limit int) ([]*Tag, error) {
	// Normalize the query
	normalizedQuery := strings.TrimSpace(strings.ToLower(query))

	// Use the intelligent SuggestTags method which handles:
	// - Empty query -> popular tags
	// - Short query (1-2 chars) -> prefix matching
	// - Longer query -> trigram similarity with fuzzy matching
	tags, err := s.repo.SuggestTags(ctx, userID, normalizedQuery, limit)
	if err != nil {
		s.logger.Error("Failed to suggest tags", zap.Error(err))
		return nil, fmt.Errorf("failed to search tags")
	}

	s.logger.Debug("Tag autocomplete",
		zap.String("user_id", userID.String()),
		zap.String("query", query),
		zap.Int("results", len(tags)),
	)

	return tags, nil
}

// GetPopularTags retrieves the most frequently used tags
func (s *Service) GetPopularTags(ctx context.Context, userID uuid.UUID, limit int) ([]*Tag, error) {
	tags, err := s.repo.GetPopularTags(ctx, userID, limit)
	if err != nil {
		s.logger.Error("Failed to get popular tags", zap.Error(err))
		return nil, fmt.Errorf("failed to get popular tags")
	}

	return tags, nil
}

