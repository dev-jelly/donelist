package tag

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dev-jelly/donelist/pkg/cache"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// CachedService wraps the tag service with caching
type CachedService struct {
	service     *Service
	cache       *cache.Cache
	invalidator *cache.Invalidator
	logger      *zap.Logger
}

// NewCachedService creates a new cached tag service
func NewCachedService(service *Service, cacheClient *cache.Cache, logger *zap.Logger) *CachedService {
	return &CachedService{
		service:     service,
		cache:       cacheClient,
		invalidator: cache.NewInvalidator(cacheClient, logger),
		logger:      logger,
	}
}

// makeTagKey creates a cache key for a single tag
func makeTagKey(userID uuid.UUID, tagID uuid.UUID) string {
	return fmt.Sprintf("tag:%s:%s", userID.String(), tagID.String())
}

// makeTagsKey creates a cache key for user's tag list
func makeTagsKey(userID uuid.UUID) string {
	return fmt.Sprintf("tags:user:%s", userID.String())
}

// makeTagSuggestKey creates a cache key for tag suggestions
func makeTagSuggestKey(userID uuid.UUID, query string, limit int) string {
	// Normalize query for cache key
	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	return fmt.Sprintf("tags:suggest:%s:%s:%d", userID.String(), normalizedQuery, limit)
}

// makePopularTagsKey creates a cache key for popular tags
func makePopularTagsKey(userID uuid.UUID, limit int) string {
	return fmt.Sprintf("tags:popular:%s:%d", userID.String(), limit)
}

// Create creates a new tag and invalidates cache
func (ts *CachedService) Create(ctx context.Context, input CreateInput) (*Tag, error) {
	// Create the tag
	tag, err := ts.service.Create(ctx, input)
	if err != nil {
		return nil, err
	}

	// Invalidate caches
	if err := ts.invalidateTagCache(ctx, input.UserID); err != nil {
		ts.logger.Warn("Failed to invalidate cache after tag creation",
			zap.String("user_id", input.UserID.String()),
			zap.Error(err))
	}

	return tag, nil
}

// GetByID retrieves a tag by ID with caching
func (ts *CachedService) GetByID(ctx context.Context, id, userID uuid.UUID) (*Tag, error) {
	cacheKey := makeTagKey(userID, id)

	var tag Tag
	err := ts.cache.GetOrSet(ctx, cacheKey, cache.TTLTags, &tag, func() (interface{}, error) {
		return ts.service.GetByID(ctx, id, userID)
	})

	if err != nil {
		return nil, err
	}

	return &tag, nil
}

// List retrieves all tags for a user with caching
func (ts *CachedService) List(ctx context.Context, userID uuid.UUID) ([]*Tag, error) {
	cacheKey := makeTagsKey(userID)

	var tags []*Tag
	err := ts.cache.GetOrSet(ctx, cacheKey, cache.TTLTags, &tags, func() (interface{}, error) {
		return ts.service.List(ctx, userID)
	})

	if err != nil {
		return nil, err
	}

	return tags, nil
}

// Autocomplete searches for tags with caching for suggestions
func (ts *CachedService) Autocomplete(ctx context.Context, userID uuid.UUID, query string, limit int) ([]*Tag, error) {
	// For very short queries (1-2 chars) or empty queries, use shorter TTL
	// as these are more likely to change and less specific
	var ttl time.Duration
	if len(strings.TrimSpace(query)) <= 2 {
		ttl = 5 * time.Minute
	} else {
		ttl = 15 * time.Minute
	}

	cacheKey := makeTagSuggestKey(userID, query, limit)

	var tags []*Tag
	err := ts.cache.GetOrSet(ctx, cacheKey, ttl, &tags, func() (interface{}, error) {
		return ts.service.Autocomplete(ctx, userID, query, limit)
	})

	if err != nil {
		return nil, err
	}

	return tags, nil
}

// GetPopularTags retrieves the most frequently used tags with caching
func (ts *CachedService) GetPopularTags(ctx context.Context, userID uuid.UUID, limit int) ([]*Tag, error) {
	cacheKey := makePopularTagsKey(userID, limit)

	var tags []*Tag
	// Popular tags change less frequently, use longer TTL
	err := ts.cache.GetOrSet(ctx, cacheKey, 30*time.Minute, &tags, func() (interface{}, error) {
		return ts.service.GetPopularTags(ctx, userID, limit)
	})

	if err != nil {
		return nil, err
	}

	return tags, nil
}

// invalidateTagCache invalidates all tag-related caches for a user
func (ts *CachedService) invalidateTagCache(ctx context.Context, userID uuid.UUID) error {
	patterns := []string{
		fmt.Sprintf("tags:user:%s", userID.String()),
		fmt.Sprintf("tag:%s:*", userID.String()),
		fmt.Sprintf("tags:suggest:%s:*", userID.String()),
		fmt.Sprintf("tags:popular:%s:*", userID.String()),
		// Invalidate any task-related caches that might reference tags
		fmt.Sprintf("tasks:user:%s:*", userID.String()),
	}

	for _, pattern := range patterns {
		if err := ts.cache.DeletePattern(ctx, pattern); err != nil {
			ts.logger.Error("Failed to invalidate tag pattern",
				zap.String("pattern", pattern),
				zap.Error(err))
			// Continue invalidating other patterns
		}
	}

	ts.logger.Debug("Invalidated tag cache",
		zap.String("user_id", userID.String()))

	return nil
}

// WarmCache pre-loads frequently accessed data into cache
func (ts *CachedService) WarmCache(ctx context.Context, userID uuid.UUID) error {
	// Pre-load user's tags
	_, err := ts.List(ctx, userID)
	if err != nil {
		ts.logger.Warn("Failed to warm tag list cache",
			zap.String("user_id", userID.String()),
			zap.Error(err))
	}

	// Pre-load popular tags
	_, err = ts.GetPopularTags(ctx, userID, 20)
	if err != nil {
		ts.logger.Warn("Failed to warm popular tags cache",
			zap.String("user_id", userID.String()),
			zap.Error(err))
	}

	// Pre-load common autocomplete queries (empty query shows popular tags)
	_, err = ts.Autocomplete(ctx, userID, "", 10)
	if err != nil {
		ts.logger.Warn("Failed to warm autocomplete cache",
			zap.String("user_id", userID.String()),
			zap.Error(err))
	}

	return nil
}

// InvalidateUserTagSuggestions invalidates only the suggestion caches for a user
// This is useful when task data changes but tags themselves haven't changed
func (ts *CachedService) InvalidateUserTagSuggestions(ctx context.Context, userID uuid.UUID) error {
	patterns := []string{
		fmt.Sprintf("tags:suggest:%s:*", userID.String()),
		fmt.Sprintf("tags:popular:%s:*", userID.String()),
	}

	for _, pattern := range patterns {
		if err := ts.cache.DeletePattern(ctx, pattern); err != nil {
			ts.logger.Error("Failed to invalidate tag suggestion pattern",
				zap.String("pattern", pattern),
				zap.Error(err))
		}
	}

	return nil
}