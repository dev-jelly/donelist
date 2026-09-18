package category

import (
	"context"
	"fmt"
	"time"

	"github.com/dev-jelly/donelist/pkg/cache"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// CachedService wraps the category service with caching
type CachedService struct {
	service     *Service
	cache       *cache.Cache
	invalidator *cache.Invalidator
	logger      *zap.Logger
}

// NewCachedService creates a new cached category service
func NewCachedService(service *Service, cacheClient *cache.Cache, logger *zap.Logger) *CachedService {
	return &CachedService{
		service:     service,
		cache:       cacheClient,
		invalidator: cache.NewInvalidator(cacheClient, logger),
		logger:      logger,
	}
}

// makeCategoryKey creates a cache key for a single category
func makeCategoryKey(userID uuid.UUID, categoryID uuid.UUID) string {
	return fmt.Sprintf("category:%s:%s", userID.String(), categoryID.String())
}

// makeCategoriesKey creates a cache key for user's category list
func makeCategoriesKey(userID uuid.UUID) string {
	return fmt.Sprintf("categories:user:%s", userID.String())
}

// makeColorRecommendationsKey creates a cache key for color recommendations
func makeColorRecommendationsKey(userID uuid.UUID, count int) string {
	return fmt.Sprintf("categories:colors:recommend:%s:%d", userID.String(), count)
}

// Create creates a new category and invalidates cache
func (cs *CachedService) Create(ctx context.Context, input CreateInput) (*Category, error) {
	// Create the category
	category, err := cs.service.Create(ctx, input)
	if err != nil {
		return nil, err
	}

	// Invalidate the user's category list cache
	if err := cs.invalidateCategoryCache(ctx, input.UserID); err != nil {
		cs.logger.Warn("Failed to invalidate cache after category creation",
			zap.String("user_id", input.UserID.String()),
			zap.Error(err))
	}

	return category, nil
}

// GetByID retrieves a category by ID with caching
func (cs *CachedService) GetByID(ctx context.Context, id, userID uuid.UUID) (*Category, error) {
	cacheKey := makeCategoryKey(userID, id)

	var category Category
	err := cs.cache.GetOrSet(ctx, cacheKey, cache.TTLCategories, &category, func() (interface{}, error) {
		return cs.service.GetByID(ctx, id, userID)
	})

	if err != nil {
		return nil, err
	}

	return &category, nil
}

// List retrieves all categories for a user with caching
func (cs *CachedService) List(ctx context.Context, userID uuid.UUID) ([]*Category, error) {
	cacheKey := makeCategoriesKey(userID)

	var categories []*Category
	err := cs.cache.GetOrSet(ctx, cacheKey, cache.TTLCategories, &categories, func() (interface{}, error) {
		return cs.service.List(ctx, userID)
	})

	if err != nil {
		return nil, err
	}

	return categories, nil
}

// Update updates a category and invalidates cache
func (cs *CachedService) Update(ctx context.Context, id, userID uuid.UUID, input UpdateInput) (*Category, error) {
	// Update the category
	category, err := cs.service.Update(ctx, id, userID, input)
	if err != nil {
		return nil, err
	}

	// Invalidate caches
	if err := cs.invalidateCategoryCache(ctx, userID); err != nil {
		cs.logger.Warn("Failed to invalidate cache after category update",
			zap.String("user_id", userID.String()),
			zap.String("category_id", id.String()),
			zap.Error(err))
	}

	// Also invalidate the specific category cache
	if err := cs.cache.Delete(ctx, makeCategoryKey(userID, id)); err != nil {
		cs.logger.Warn("Failed to invalidate specific category cache",
			zap.String("category_id", id.String()),
			zap.Error(err))
	}

	return category, nil
}

// Delete deletes a category and invalidates cache
func (cs *CachedService) Delete(ctx context.Context, id, userID uuid.UUID) error {
	// Delete the category
	if err := cs.service.Delete(ctx, id, userID); err != nil {
		return err
	}

	// Invalidate caches
	if err := cs.invalidateCategoryCache(ctx, userID); err != nil {
		cs.logger.Warn("Failed to invalidate cache after category deletion",
			zap.String("user_id", userID.String()),
			zap.String("category_id", id.String()),
			zap.Error(err))
	}

	// Also invalidate the specific category cache
	if err := cs.cache.Delete(ctx, makeCategoryKey(userID, id)); err != nil {
		cs.logger.Warn("Failed to invalidate specific category cache",
			zap.String("category_id", id.String()),
			zap.Error(err))
	}

	return nil
}

// GetRecommendedColors returns a list of recommended colors with caching
func (cs *CachedService) GetRecommendedColors(ctx context.Context, userID uuid.UUID, count int) ([]string, error) {
	cacheKey := makeColorRecommendationsKey(userID, count)

	var colors []string
	// Use shorter TTL for recommendations as they change based on categories
	err := cs.cache.GetOrSet(ctx, cacheKey, 30*time.Minute, &colors, func() (interface{}, error) {
		return cs.service.GetRecommendedColors(ctx, userID, count)
	})

	if err != nil {
		return nil, err
	}

	return colors, nil
}

// GetColorInfo returns detailed information about a color (no caching needed as it's deterministic)
func (cs *CachedService) GetColorInfo(colorStr string) (*ColorPaletteInfo, error) {
	// This is a pure function, no need for caching
	return cs.service.GetColorInfo(colorStr)
}

// invalidateCategoryCache invalidates all category-related caches for a user
func (cs *CachedService) invalidateCategoryCache(ctx context.Context, userID uuid.UUID) error {
	patterns := []string{
		fmt.Sprintf("categories:user:%s", userID.String()),
		fmt.Sprintf("category:%s:*", userID.String()),
		fmt.Sprintf("categories:colors:recommend:%s:*", userID.String()),
		// Invalidate any task-related caches that might reference categories
		fmt.Sprintf("tasks:user:%s:*", userID.String()),
	}

	for _, pattern := range patterns {
		if err := cs.cache.DeletePattern(ctx, pattern); err != nil {
			cs.logger.Error("Failed to invalidate category pattern",
				zap.String("pattern", pattern),
				zap.Error(err))
			// Continue invalidating other patterns
		}
	}

	cs.logger.Debug("Invalidated category cache",
		zap.String("user_id", userID.String()))

	return nil
}

// WarmCache pre-loads frequently accessed data into cache
func (cs *CachedService) WarmCache(ctx context.Context, userID uuid.UUID) error {
	// Pre-load user's categories
	_, err := cs.List(ctx, userID)
	if err != nil {
		cs.logger.Warn("Failed to warm category list cache",
			zap.String("user_id", userID.String()),
			zap.Error(err))
	}

	// Pre-load color recommendations
	_, err = cs.GetRecommendedColors(ctx, userID, 10)
	if err != nil {
		cs.logger.Warn("Failed to warm color recommendations cache",
			zap.String("user_id", userID.String()),
			zap.Error(err))
	}

	return nil
}