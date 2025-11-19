package apikey

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Service handles API key business logic
type Service struct {
	repo      *Repository
	generator *KeyGenerator
	logger    *zap.Logger
}

// NewService creates a new API key service
func NewService(repo *Repository, logger *zap.Logger) *Service {
	return &Service{
		repo:      repo,
		generator: NewKeyGenerator(),
		logger:    logger,
	}
}

// CreateAPIKey creates a new API key
func (s *Service) CreateAPIKey(ctx context.Context, input CreateAPIKeyInput) (*APIKeyWithPlainText, error) {
	// Validate scopes
	if err := ValidateScopes(input.Scopes); err != nil {
		return nil, err
	}

	// Set default rate limits if not provided
	if input.RateLimitPerDay == 0 {
		input.RateLimitPerDay = 10000
	}
	if input.RateLimitPerHour == 0 {
		input.RateLimitPerHour = 1000
	}

	// Generate API key
	keyPrefix, fullKey, keyHash, err := s.generator.GenerateKey()
	if err != nil {
		s.logger.Error("Failed to generate API key", zap.Error(err))
		return nil, fmt.Errorf("failed to generate API key: %w", err)
	}

	// Create in database
	apiKey, err := s.repo.Create(ctx, input, keyPrefix, keyHash)
	if err != nil {
		s.logger.Error("Failed to create API key in database",
			zap.Error(err),
			zap.String("user_id", input.UserID.String()),
		)
		return nil, err
	}

	s.logger.Info("API key created",
		zap.String("api_key_id", apiKey.ID.String()),
		zap.String("user_id", apiKey.UserID.String()),
		zap.String("name", apiKey.Name),
	)

	return &APIKeyWithPlainText{
		APIKey:       *apiKey,
		PlainTextKey: fullKey,
	}, nil
}

// ValidateAPIKey validates an API key and returns the key details
func (s *Service) ValidateAPIKey(ctx context.Context, key string) (*APIKey, error) {
	// Validate format
	if err := s.generator.ValidateKeyFormat(key); err != nil {
		return nil, err
	}

	// Hash the key
	keyHash := s.generator.HashKey(key)

	// Get from database
	apiKey, err := s.repo.GetByHash(ctx, keyHash)
	if err != nil {
		return nil, err
	}

	// Check if revoked
	if apiKey.Revoked {
		return nil, ErrAPIKeyRevoked
	}

	// Check if expired
	if apiKey.IsExpired() {
		return nil, ErrAPIKeyExpired
	}

	return apiKey, nil
}

// CheckRateLimit checks if the API key has exceeded its rate limit
func (s *Service) CheckRateLimit(ctx context.Context, apiKeyID uuid.UUID) (*RateLimitInfo, error) {
	info, apiKey, err := s.repo.GetRateLimitUsage(ctx, apiKeyID)
	if err != nil {
		return nil, err
	}

	// Check hourly limit
	if info.HourlyUsed >= apiKey.RateLimitPerHour {
		s.logger.Warn("Hourly rate limit exceeded",
			zap.String("api_key_id", apiKeyID.String()),
			zap.Int("used", info.HourlyUsed),
			zap.Int("limit", apiKey.RateLimitPerHour),
		)
		return info, ErrRateLimitExceeded
	}

	// Check daily limit
	if info.DailyUsed >= apiKey.RateLimitPerDay {
		s.logger.Warn("Daily rate limit exceeded",
			zap.String("api_key_id", apiKeyID.String()),
			zap.Int("used", info.DailyUsed),
			zap.Int("limit", apiKey.RateLimitPerDay),
		)
		return info, ErrRateLimitExceeded
	}

	return info, nil
}

// RecordUsage records an API key usage and updates rate limits
func (s *Service) RecordUsage(ctx context.Context, usage APIKeyUsage) error {
	// Record usage
	if err := s.repo.RecordUsage(ctx, usage); err != nil {
		s.logger.Error("Failed to record API key usage",
			zap.Error(err),
			zap.String("api_key_id", usage.APIKeyID.String()),
		)
		return err
	}

	// Increment rate limit
	if err := s.repo.IncrementRateLimit(ctx, usage.APIKeyID); err != nil {
		s.logger.Error("Failed to increment rate limit",
			zap.Error(err),
			zap.String("api_key_id", usage.APIKeyID.String()),
		)
		return err
	}

	// Update last used timestamp (async, don't wait for it)
	go func() {
		ctx := context.Background()
		if err := s.repo.UpdateLastUsed(ctx, usage.APIKeyID); err != nil {
			s.logger.Error("Failed to update last used timestamp",
				zap.Error(err),
				zap.String("api_key_id", usage.APIKeyID.String()),
			)
		}
	}()

	return nil
}

// GetAPIKey retrieves an API key by ID
func (s *Service) GetAPIKey(ctx context.Context, id uuid.UUID, userID uuid.UUID) (*APIKey, error) {
	apiKey, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Verify ownership
	if apiKey.UserID != userID {
		return nil, ErrAPIKeyNotFound
	}

	return apiKey, nil
}

// ListAPIKeys lists all API keys for a user
func (s *Service) ListAPIKeys(ctx context.Context, userID uuid.UUID, includeRevoked bool) ([]APIKey, error) {
	return s.repo.ListByUserID(ctx, userID, includeRevoked)
}

// UpdateAPIKey updates an API key
func (s *Service) UpdateAPIKey(ctx context.Context, id uuid.UUID, userID uuid.UUID, input UpdateAPIKeyInput) (*APIKey, error) {
	// Verify ownership first
	existing, err := s.GetAPIKey(ctx, id, userID)
	if err != nil {
		return nil, err
	}

	// Don't allow updating revoked keys
	if existing.Revoked {
		return nil, ErrAPIKeyRevoked
	}

	// Validate scopes if provided
	if input.Scopes != nil {
		if err := ValidateScopes(input.Scopes); err != nil {
			return nil, err
		}
	}

	apiKey, err := s.repo.Update(ctx, id, input)
	if err != nil {
		s.logger.Error("Failed to update API key",
			zap.Error(err),
			zap.String("api_key_id", id.String()),
		)
		return nil, err
	}

	s.logger.Info("API key updated",
		zap.String("api_key_id", apiKey.ID.String()),
		zap.String("user_id", apiKey.UserID.String()),
	)

	return apiKey, nil
}

// RevokeAPIKey revokes an API key
func (s *Service) RevokeAPIKey(ctx context.Context, id uuid.UUID, userID uuid.UUID, reason string) error {
	// Verify ownership
	_, err := s.GetAPIKey(ctx, id, userID)
	if err != nil {
		return err
	}

	err = s.repo.Revoke(ctx, id, reason)
	if err != nil {
		s.logger.Error("Failed to revoke API key",
			zap.Error(err),
			zap.String("api_key_id", id.String()),
		)
		return err
	}

	s.logger.Info("API key revoked",
		zap.String("api_key_id", id.String()),
		zap.String("user_id", userID.String()),
		zap.String("reason", reason),
	)

	return nil
}

// DeleteAPIKey permanently deletes an API key
func (s *Service) DeleteAPIKey(ctx context.Context, id uuid.UUID, userID uuid.UUID) error {
	// Verify ownership
	_, err := s.GetAPIKey(ctx, id, userID)
	if err != nil {
		return err
	}

	err = s.repo.Delete(ctx, id)
	if err != nil {
		s.logger.Error("Failed to delete API key",
			zap.Error(err),
			zap.String("api_key_id", id.String()),
		)
		return err
	}

	s.logger.Info("API key deleted",
		zap.String("api_key_id", id.String()),
		zap.String("user_id", userID.String()),
	)

	return nil
}

// GetUsageStatistics retrieves usage statistics for an API key
func (s *Service) GetUsageStatistics(ctx context.Context, apiKeyID uuid.UUID, userID uuid.UUID, days int) (*UsageStatistics, error) {
	// Verify ownership
	_, err := s.GetAPIKey(ctx, apiKeyID, userID)
	if err != nil {
		return nil, err
	}

	if days <= 0 {
		days = 30
	}
	if days > 90 {
		days = 90
	}

	return s.repo.GetUsageStatistics(ctx, apiKeyID, days)
}

// RotateAPIKey creates a new API key and revokes the old one
func (s *Service) RotateAPIKey(ctx context.Context, oldKeyID uuid.UUID, userID uuid.UUID) (*APIKeyWithPlainText, error) {
	// Get old key
	oldKey, err := s.GetAPIKey(ctx, oldKeyID, userID)
	if err != nil {
		return nil, err
	}

	// Create new key with same settings
	newKey, err := s.CreateAPIKey(ctx, CreateAPIKeyInput{
		UserID:           userID,
		Name:             oldKey.Name + " (rotated)",
		Scopes:           stringsToScopes(oldKey.Scopes),
		RateLimitPerDay:  oldKey.RateLimitPerDay,
		RateLimitPerHour: oldKey.RateLimitPerHour,
		ExpiresAt:        oldKey.ExpiresAt,
	})
	if err != nil {
		return nil, err
	}

	// Revoke old key
	err = s.RevokeAPIKey(ctx, oldKeyID, userID, "Rotated to new key")
	if err != nil {
		// Log error but don't fail rotation
		s.logger.Error("Failed to revoke old key during rotation",
			zap.Error(err),
			zap.String("old_key_id", oldKeyID.String()),
			zap.String("new_key_id", newKey.ID.String()),
		)
	}

	s.logger.Info("API key rotated",
		zap.String("old_key_id", oldKeyID.String()),
		zap.String("new_key_id", newKey.ID.String()),
		zap.String("user_id", userID.String()),
	)

	return newKey, nil
}

// Helper functions

func stringsToScopes(strings []string) []Scope {
	scopes := make([]Scope, len(strings))
	for i, s := range strings {
		scopes[i] = Scope(s)
	}
	return scopes
}
