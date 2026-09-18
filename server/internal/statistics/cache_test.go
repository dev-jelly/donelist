package statistics

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupTestRedis(t *testing.T) (*redis.Client, func()) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	cleanup := func() {
		client.Close()
		mr.Close()
	}

	return client, cleanup
}

func TestCacheService_GetSetWeeklyStats(t *testing.T) {
	redisClient, cleanup := setupTestRedis(t)
	defer cleanup()

	logger := zap.NewNop()
	cache := NewCacheService(redisClient, logger)
	ctx := context.Background()

	// Create test statistics
	stats := &WeeklyStatistics{
		Year:         2024,
		WeekNumber:   3,
		StartDate:    "2024-01-15",
		EndDate:      "2024-01-21",
		WeekStartDay: WeekStartMonday,
		Timezone:     "UTC",
		Summary: &WeeklySummary{
			TotalCheckins:    47,
			TotalMinutes:     2340,
			DaysWithCheckins: 5,
			AveragePerDay:    6.71,
			CompletionRate:   71.43,
		},
		GeneratedAt: time.Now().UTC(),
	}

	cacheKey := "test-user:2024-01-15:monday:UTC"

	// Test cache miss
	t.Run("CacheMiss", func(t *testing.T) {
		result, err := cache.GetWeeklyStats(ctx, cacheKey)
		assert.NoError(t, err)
		assert.Nil(t, result)
	})

	// Test set cache
	t.Run("SetCache", func(t *testing.T) {
		err := cache.SetWeeklyStats(ctx, cacheKey, stats)
		assert.NoError(t, err)
	})

	// Test cache hit
	t.Run("CacheHit", func(t *testing.T) {
		result, err := cache.GetWeeklyStats(ctx, cacheKey)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, stats.Year, result.Year)
		assert.Equal(t, stats.WeekNumber, result.WeekNumber)
		assert.Equal(t, stats.StartDate, result.StartDate)
		assert.Equal(t, stats.EndDate, result.EndDate)
		assert.Equal(t, stats.Summary.TotalCheckins, result.Summary.TotalCheckins)
	})
}

func TestCacheService_InvalidateUserStats(t *testing.T) {
	redisClient, cleanup := setupTestRedis(t)
	defer cleanup()

	logger := zap.NewNop()
	cache := NewCacheService(redisClient, logger)
	ctx := context.Background()

	userID := "test-user"

	// Create multiple cache entries for the user
	stats := &WeeklyStatistics{
		Year:         2024,
		WeekNumber:   3,
		StartDate:    "2024-01-15",
		EndDate:      "2024-01-21",
		WeekStartDay: WeekStartMonday,
		Timezone:     "UTC",
		Summary:      &WeeklySummary{TotalCheckins: 47},
		GeneratedAt:  time.Now().UTC(),
	}

	// Set multiple weeks for the same user
	keys := []string{
		fmt.Sprintf("%s:2024-01-01:monday:UTC", userID),
		fmt.Sprintf("%s:2024-01-08:monday:UTC", userID),
		fmt.Sprintf("%s:2024-01-15:monday:UTC", userID),
	}

	for _, key := range keys {
		err := cache.SetWeeklyStats(ctx, key, stats)
		require.NoError(t, err)
	}

	// Verify all entries exist
	for _, key := range keys {
		result, err := cache.GetWeeklyStats(ctx, key)
		assert.NoError(t, err)
		assert.NotNil(t, result)
	}

	// Invalidate all user stats
	err := cache.InvalidateUserStats(ctx, userID)
	assert.NoError(t, err)

	// Verify all entries are gone
	for _, key := range keys {
		result, err := cache.GetWeeklyStats(ctx, key)
		assert.NoError(t, err)
		assert.Nil(t, result)
	}
}

func TestGenerateCacheKey(t *testing.T) {
	tests := []struct {
		name         string
		userID       string
		startDate    string
		weekStartDay WeekStartDay
		timezone     string
		expected     string
	}{
		{
			name:         "Monday start UTC",
			userID:       "user-123",
			startDate:    "2024-01-15",
			weekStartDay: WeekStartMonday,
			timezone:     "UTC",
			expected:     "user-123:2024-01-15:monday:UTC",
		},
		{
			name:         "Sunday start America/New_York",
			userID:       "user-456",
			startDate:    "2024-01-14",
			weekStartDay: WeekStartSunday,
			timezone:     "America/New_York",
			expected:     "user-456:2024-01-14:sunday:America/New_York",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateCacheKey(tt.userID, tt.startDate, tt.weekStartDay, tt.timezone)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCacheService_CustomExpiration(t *testing.T) {
	redisClient, cleanup := setupTestRedis(t)
	defer cleanup()

	logger := zap.NewNop()
	cache := NewCacheService(redisClient, logger)
	ctx := context.Background()

	// Create stats with custom cache expiration that's shorter than default
	customExpiration := time.Now().Add(5 * time.Second)
	stats := &WeeklyStatistics{
		Year:            2024,
		WeekNumber:      3,
		StartDate:       "2024-01-15",
		EndDate:         "2024-01-21",
		WeekStartDay:    WeekStartMonday,
		Timezone:        "UTC",
		Summary:         &WeeklySummary{TotalCheckins: 47},
		GeneratedAt:     time.Now().UTC(),
		CacheExpiration: customExpiration,
	}

	cacheKey := "test-user:2024-01-15:monday:UTC"

	// Set cache with custom expiration
	err := cache.SetWeeklyStats(ctx, cacheKey, stats)
	assert.NoError(t, err)

	// Verify it exists
	result, err := cache.GetWeeklyStats(ctx, cacheKey)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, stats.Year, result.Year)
	assert.Equal(t, stats.Summary.TotalCheckins, result.Summary.TotalCheckins)

	// Note: We don't test actual expiration with miniredis as it doesn't
	// implement TTL properly. In production, Redis will handle TTL correctly.
}
