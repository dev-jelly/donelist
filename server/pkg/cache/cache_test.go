package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupTestCache(t *testing.T) (*Cache, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	logger, _ := zap.NewDevelopment()
	cache := NewCache(client, Config{
		Prefix:     "test:",
		DefaultTTL: 5 * time.Minute,
	}, logger)

	return cache, mr
}

func TestCache_SetAndGet(t *testing.T) {
	cache, mr := setupTestCache(t)
	defer mr.Close()

	ctx := context.Background()
	key := "user:123"
	value := map[string]interface{}{
		"id":   123,
		"name": "Test User",
	}

	// Set value
	err := cache.Set(ctx, key, value, 1*time.Minute)
	require.NoError(t, err)

	// Get value
	var result map[string]interface{}
	err = cache.Get(ctx, key, &result)
	require.NoError(t, err)

	assert.Equal(t, float64(123), result["id"])
	assert.Equal(t, "Test User", result["name"])
}

func TestCache_Miss(t *testing.T) {
	cache, mr := setupTestCache(t)
	defer mr.Close()

	ctx := context.Background()
	var result map[string]interface{}

	err := cache.Get(ctx, "nonexistent", &result)
	assert.ErrorIs(t, err, ErrCacheMiss)
	assert.Equal(t, int64(1), cache.GetMetrics().Misses)
}

func TestCache_Delete(t *testing.T) {
	cache, mr := setupTestCache(t)
	defer mr.Close()

	ctx := context.Background()
	key := "user:123"

	// Set value
	err := cache.Set(ctx, key, "test", 1*time.Minute)
	require.NoError(t, err)

	// Verify it exists
	exists, err := cache.Exists(ctx, key)
	require.NoError(t, err)
	assert.True(t, exists)

	// Delete
	err = cache.Delete(ctx, key)
	require.NoError(t, err)

	// Verify it's gone
	exists, err = cache.Exists(ctx, key)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestCache_DeletePattern(t *testing.T) {
	cache, mr := setupTestCache(t)
	defer mr.Close()

	ctx := context.Background()

	// Set multiple values
	cache.Set(ctx, "user:1", "test1", 1*time.Minute)
	cache.Set(ctx, "user:2", "test2", 1*time.Minute)
	cache.Set(ctx, "category:1", "test3", 1*time.Minute)

	// Delete by pattern
	err := cache.DeletePattern(ctx, "user:*")
	require.NoError(t, err)

	// Verify user keys are gone
	exists, _ := cache.Exists(ctx, "user:1")
	assert.False(t, exists)

	exists, _ = cache.Exists(ctx, "user:2")
	assert.False(t, exists)

	// Verify category key still exists
	exists, _ = cache.Exists(ctx, "category:1")
	assert.True(t, exists)
}

func TestCache_GetOrSet(t *testing.T) {
	cache, mr := setupTestCache(t)
	defer mr.Close()

	ctx := context.Background()
	key := "user:123"
	fetchCalled := false

	fetchFunc := func() (interface{}, error) {
		fetchCalled = true
		return map[string]string{"name": "John"}, nil
	}

	// First call should fetch
	var result map[string]string
	err := cache.GetOrSet(ctx, key, 1*time.Minute, &result, fetchFunc)
	require.NoError(t, err)
	assert.True(t, fetchCalled)
	assert.Equal(t, "John", result["name"])

	// Second call should use cache
	fetchCalled = false
	err = cache.GetOrSet(ctx, key, 1*time.Minute, &result, fetchFunc)
	require.NoError(t, err)
	assert.False(t, fetchCalled)
	assert.Equal(t, "John", result["name"])
}

func TestCache_Metrics(t *testing.T) {
	cache, mr := setupTestCache(t)
	defer mr.Close()

	ctx := context.Background()

	// Set a value
	cache.Set(ctx, "key1", "value1", 1*time.Minute)

	// Hit
	var result string
	cache.Get(ctx, "key1", &result)

	// Miss
	cache.Get(ctx, "key2", &result)

	metrics := cache.GetMetrics()
	assert.Equal(t, int64(1), metrics.Hits)
	assert.Equal(t, int64(1), metrics.Misses)

	// Reset
	cache.ResetMetrics()
	metrics = cache.GetMetrics()
	assert.Equal(t, int64(0), metrics.Hits)
	assert.Equal(t, int64(0), metrics.Misses)
}

func TestCache_KeyGenerators(t *testing.T) {
	tests := []struct {
		name     string
		genFunc  func() string
		expected string
	}{
		{
			name:     "User key",
			genFunc:  func() string { return MakeUserKey(123) },
			expected: "user:123",
		},
		{
			name:     "Checkins key",
			genFunc:  func() string { return MakeCheckinsKey(123, "2024-01-15") },
			expected: "checkins:123:2024-01-15",
		},
		{
			name:     "Categories key",
			genFunc:  func() string { return MakeCategoriesKey(123) },
			expected: "categories:123",
		},
		{
			name:     "Tags key",
			genFunc:  func() string { return MakeTagsKey(123) },
			expected: "tags:123",
		},
		{
			name:     "Statistics key",
			genFunc:  func() string { return MakeStatisticsKey(123, "2024-01-01", "2024-01-31") },
			expected: "stats:123:2024-01-01:2024-01-31",
		},
		{
			name:     "Search key",
			genFunc:  func() string { return MakeSearchKey(123, "test query", 10, 0) },
			expected: "search:123:test query:10:0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.genFunc()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCache_TTL(t *testing.T) {
	cache, mr := setupTestCache(t)
	defer mr.Close()

	ctx := context.Background()
	key := "ttl-test"

	// Set with short TTL
	err := cache.Set(ctx, key, "value", 100*time.Millisecond)
	require.NoError(t, err)

	// Verify it exists
	var result string
	err = cache.Get(ctx, key, &result)
	require.NoError(t, err)

	// Fast forward time in miniredis
	mr.FastForward(200 * time.Millisecond)

	// Verify it's expired
	err = cache.Get(ctx, key, &result)
	assert.ErrorIs(t, err, ErrCacheMiss)
}
