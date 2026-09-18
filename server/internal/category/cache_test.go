package category

import (
	"context"
	"testing"
	"time"

	"github.com/dev-jelly/donelist/internal/testutil"
	"github.com/dev-jelly/donelist/pkg/cache"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestCachedService_List(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	// Setup Redis for testing
	redisClient := testutil.SetupTestRedis(t)
	cacheConfig := cache.Config{
		Prefix:     "test:",
		DefaultTTL: 5 * time.Minute,
	}
	cacheClient := cache.NewCache(redisClient, cacheConfig, logger)

	// Create mock repository
	mockRepo := new(MockRepositoryInterface)

	// Create service and cached service
	service := NewService(mockRepo, logger)
	cachedService := NewCachedService(service, cacheClient, logger)

	userID := uuid.New()
	categories := []*Category{
		{
			ID:        uuid.New(),
			UserID:    userID,
			Name:      "Work",
			Color:     stringPtr("#FF5733"),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		{
			ID:        uuid.New(),
			UserID:    userID,
			Name:      "Personal",
			Color:     stringPtr("#33FF57"),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}

	// Setup expectation for first call (cache miss)
	mockRepo.On("List", ctx, userID).Return(categories, nil).Once()

	// First call - should hit the database
	result1, err := cachedService.List(ctx, userID)
	require.NoError(t, err)
	assert.Len(t, result1, 2)
	assert.Equal(t, categories[0].Name, result1[0].Name)

	// Second call - should hit the cache
	result2, err := cachedService.List(ctx, userID)
	require.NoError(t, err)
	assert.Len(t, result2, 2)
	assert.Equal(t, categories[0].Name, result2[0].Name)

	// Verify that repository was only called once
	mockRepo.AssertExpectations(t)
}

func TestCachedService_Create_InvalidatesCache(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	// Setup Redis for testing
	redisClient := testutil.SetupTestRedis(t)
	cacheConfig := cache.Config{
		Prefix:     "test:",
		DefaultTTL: 5 * time.Minute,
	}
	cacheClient := cache.NewCache(redisClient, cacheConfig, logger)

	// Create mock repository
	mockRepo := new(MockRepositoryInterface)

	// Create service and cached service
	service := NewService(mockRepo, logger)
	cachedService := NewCachedService(service, cacheClient, logger)

	userID := uuid.New()

	// Pre-populate cache
	cacheKey := makeCategoriesKey(userID)
	existingCategories := []*Category{
		{
			ID:     uuid.New(),
			UserID: userID,
			Name:   "Existing",
		},
	}
	err := cacheClient.Set(ctx, cacheKey, existingCategories, cache.TTLCategories)
	require.NoError(t, err)

	// Verify cache is populated
	var cached []*Category
	err = cacheClient.Get(ctx, cacheKey, &cached)
	require.NoError(t, err)
	assert.Len(t, cached, 1)

	// Create new category
	newCategory := &Category{
		ID:     uuid.New(),
		UserID: userID,
		Name:   "New Category",
		Color:  stringPtr("#123456"),
	}

	mockRepo.On("NameExists", ctx, userID, "New Category").Return(false, nil)
	mockRepo.On("Create", ctx, mock.Anything).Return(newCategory, nil)

	input := CreateInput{
		UserID: userID,
		Name:   "New Category",
		Color:  stringPtr("#123456"),
	}

	result, err := cachedService.Create(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, newCategory.Name, result.Name)

	// Verify cache was invalidated
	err = cacheClient.Get(ctx, cacheKey, &cached)
	assert.Equal(t, cache.ErrCacheMiss, err)

	mockRepo.AssertExpectations(t)
}

func TestCachedService_GetRecommendedColors(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	// Setup Redis for testing
	redisClient := testutil.SetupTestRedis(t)
	cacheConfig := cache.Config{
		Prefix:     "test:",
		DefaultTTL: 5 * time.Minute,
	}
	cacheClient := cache.NewCache(redisClient, cacheConfig, logger)

	// Create mock repository
	mockRepo := new(MockRepositoryInterface)

	// Create service and cached service
	service := NewService(mockRepo, logger)
	cachedService := NewCachedService(service, cacheClient, logger)

	userID := uuid.New()
	count := 5

	// Setup expectations
	mockRepo.On("List", ctx, userID).Return([]*Category{
		{
			ID:    uuid.New(),
			Color: stringPtr("#FF5733"),
		},
	}, nil).Once()

	// First call - should generate recommendations
	colors1, err := cachedService.GetRecommendedColors(ctx, userID, count)
	require.NoError(t, err)
	assert.Len(t, colors1, count)

	// Second call - should hit cache
	colors2, err := cachedService.GetRecommendedColors(ctx, userID, count)
	require.NoError(t, err)
	assert.Equal(t, colors1, colors2)

	// Verify repository was only called once
	mockRepo.AssertExpectations(t)
}

func TestCachedService_Update_InvalidatesCache(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	// Setup Redis for testing
	redisClient := testutil.SetupTestRedis(t)
	cacheConfig := cache.Config{
		Prefix:     "test:",
		DefaultTTL: 5 * time.Minute,
	}
	cacheClient := cache.NewCache(redisClient, cacheConfig, logger)

	// Create mock repository
	mockRepo := new(MockRepositoryInterface)

	// Create service and cached service
	service := NewService(mockRepo, logger)
	cachedService := NewCachedService(service, cacheClient, logger)

	userID := uuid.New()
	categoryID := uuid.New()

	// Pre-populate caches
	categoryKey := makeCategoryKey(userID, categoryID)
	categoriesKey := makeCategoriesKey(userID)

	category := &Category{
		ID:     categoryID,
		UserID: userID,
		Name:   "Original",
		Color:  stringPtr("#FF5733"),
	}

	err := cacheClient.Set(ctx, categoryKey, category, cache.TTLCategories)
	require.NoError(t, err)

	err = cacheClient.Set(ctx, categoriesKey, []*Category{category}, cache.TTLCategories)
	require.NoError(t, err)

	// Update category
	updatedCategory := &Category{
		ID:     categoryID,
		UserID: userID,
		Name:   "Updated",
		Color:  stringPtr("#33FF57"),
	}

	mockRepo.On("List", ctx, userID).Return([]*Category{category}, nil)
	mockRepo.On("Update", ctx, categoryID, userID, mock.Anything).Return(updatedCategory, nil)

	updateInput := UpdateInput{
		Name:  stringPtr("Updated"),
		Color: stringPtr("#33FF57"),
	}

	result, err := cachedService.Update(ctx, categoryID, userID, updateInput)
	require.NoError(t, err)
	assert.Equal(t, "Updated", result.Name)

	// Verify both caches were invalidated
	var cached Category
	err = cacheClient.Get(ctx, categoryKey, &cached)
	assert.Equal(t, cache.ErrCacheMiss, err)

	var cachedList []*Category
	err = cacheClient.Get(ctx, categoriesKey, &cachedList)
	assert.Equal(t, cache.ErrCacheMiss, err)

	mockRepo.AssertExpectations(t)
}

// Helper function
func stringPtr(s string) *string {
	return &s
}