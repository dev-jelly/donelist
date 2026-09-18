package analytics

import (
	"context"
	"testing"
	"time"

	"github.com/dev-jelly/donelist/internal/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestAnalyticsCache(t *testing.T) {
	// Setup Redis test container
	redisClient, cleanup := testutil.SetupRedisContainer(t)
	defer cleanup()

	logger := zap.NewNop()
	cache := NewAnalyticsCache(redisClient, logger)
	ctx := context.Background()

	t.Run("Set and Get", func(t *testing.T) {
		key := "test:key"
		data := map[string]interface{}{
			"value": "test_value",
			"count": 42,
		}

		err := cache.Set(ctx, key, data, 1*time.Minute)
		require.NoError(t, err)

		var retrieved map[string]interface{}
		found, err := cache.Get(ctx, key, &retrieved)
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, "test_value", retrieved["value"])
		assert.Equal(t, float64(42), retrieved["count"]) // JSON unmarshals numbers as float64
	})

	t.Run("Get non-existent key", func(t *testing.T) {
		var result map[string]interface{}
		found, err := cache.Get(ctx, "non:existent:key", &result)
		require.NoError(t, err)
		assert.False(t, found)
	})

	t.Run("Delete key", func(t *testing.T) {
		key := "test:delete"
		err := cache.Set(ctx, key, "value", 1*time.Minute)
		require.NoError(t, err)

		err = cache.Delete(ctx, key)
		require.NoError(t, err)

		var result string
		found, err := cache.Get(ctx, key, &result)
		require.NoError(t, err)
		assert.False(t, found)
	})

	t.Run("CacheKey generation", func(t *testing.T) {
		key := cache.CacheKey("user", "123", "daily", "2024-01-01")
		expected := "analytics:user:123:daily:2024-01-01"
		assert.Equal(t, expected, key)
	})
}

func TestUserDashboardCache(t *testing.T) {
	redisClient, cleanup := testutil.SetupRedisContainer(t)
	defer cleanup()

	logger := zap.NewNop()
	cache := NewAnalyticsCache(redisClient, logger)
	ctx := context.Background()

	userID := uuid.New()
	dashboard := map[string]interface{}{
		"total_checkins": 100,
		"streak":         15,
	}

	t.Run("Cache and retrieve dashboard", func(t *testing.T) {
		err := cache.CacheUserDashboard(ctx, userID, "week", dashboard)
		require.NoError(t, err)

		var retrieved map[string]interface{}
		found, err := cache.GetUserDashboard(ctx, userID, "week", &retrieved)
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, float64(100), retrieved["total_checkins"])
	})

	t.Run("Invalidate user cache", func(t *testing.T) {
		// Cache multiple periods
		cache.CacheUserDashboard(ctx, userID, "week", dashboard)
		cache.CacheUserDashboard(ctx, userID, "month", dashboard)

		err := cache.InvalidateUserCache(ctx, userID)
		require.NoError(t, err)

		// Both should be invalidated
		var result map[string]interface{}
		found, _ := cache.GetUserDashboard(ctx, userID, "week", &result)
		assert.False(t, found)

		found, _ = cache.GetUserDashboard(ctx, userID, "month", &result)
		assert.False(t, found)
	})
}

func TestDailySummaryCache(t *testing.T) {
	redisClient, cleanup := testutil.SetupRedisContainer(t)
	defer cleanup()

	logger := zap.NewNop()
	cache := NewAnalyticsCache(redisClient, logger)
	ctx := context.Background()

	userID := uuid.New()
	date := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	summary := DailyActivitySummary{
		UserID:        userID,
		ActivityDate:  date,
		TotalCheckins: 25,
		TotalMinutes:  120,
	}

	t.Run("Cache and retrieve daily summary", func(t *testing.T) {
		err := cache.CacheDailySummary(ctx, userID, date, &summary)
		require.NoError(t, err)

		var retrieved DailyActivitySummary
		found, err := cache.GetDailySummary(ctx, userID, date, &retrieved)
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, 25, retrieved.TotalCheckins)
		assert.Equal(t, 120, retrieved.TotalMinutes)
	})
}

func TestWeeklySummaryCache(t *testing.T) {
	redisClient, cleanup := testutil.SetupRedisContainer(t)
	defer cleanup()

	logger := zap.NewNop()
	cache := NewAnalyticsCache(redisClient, logger)
	ctx := context.Background()

	userID := uuid.New()
	weekStart := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	report := WeeklyActivityReport{
		UserID:        userID,
		WeekStart:     weekStart,
		TotalCheckins: 150,
	}

	t.Run("Cache and retrieve weekly summary", func(t *testing.T) {
		err := cache.CacheWeeklySummary(ctx, userID, weekStart, &report)
		require.NoError(t, err)

		var retrieved WeeklyActivityReport
		found, err := cache.GetWeeklySummary(ctx, userID, weekStart, &retrieved)
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, 150, retrieved.TotalCheckins)
	})
}

func TestCategoryTrendsCache(t *testing.T) {
	redisClient, cleanup := testutil.SetupRedisContainer(t)
	defer cleanup()

	logger := zap.NewNop()
	cache := NewAnalyticsCache(redisClient, logger)
	ctx := context.Background()

	userID := uuid.New()
	categoryID := uuid.New()
	trends := map[string]interface{}{
		"upward_trend": true,
		"percentage":   25.5,
	}

	t.Run("Cache and retrieve category trends", func(t *testing.T) {
		err := cache.CacheCategoryTrends(ctx, userID, categoryID, "month", trends)
		require.NoError(t, err)

		var retrieved map[string]interface{}
		found, err := cache.GetCategoryTrends(ctx, userID, categoryID, "month", &retrieved)
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, true, retrieved["upward_trend"])
	})
}

func TestStreakInfoCache(t *testing.T) {
	redisClient, cleanup := testutil.SetupRedisContainer(t)
	defer cleanup()

	logger := zap.NewNop()
	cache := NewAnalyticsCache(redisClient, logger)
	ctx := context.Background()

	userID := uuid.New()
	streakInfo := StreakMetrics{
		UserID:        userID,
		CurrentStreak: 15,
		LongestStreak: 30,
		IsActive:      true,
	}

	t.Run("Cache and retrieve streak info", func(t *testing.T) {
		err := cache.CacheStreakInfo(ctx, userID, &streakInfo)
		require.NoError(t, err)

		var retrieved StreakMetrics
		found, err := cache.GetStreakInfo(ctx, userID, &retrieved)
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, 15, retrieved.CurrentStreak)
		assert.Equal(t, 30, retrieved.LongestStreak)
		assert.True(t, retrieved.IsActive)
	})
}

func TestMaterializedViewCache(t *testing.T) {
	redisClient, cleanup := testutil.SetupRedisContainer(t)
	defer cleanup()

	logger := zap.NewNop()
	cache := NewAnalyticsCache(redisClient, logger)
	ctx := context.Background()

	viewName := "test_view"
	filters := map[string]string{
		"user_id": uuid.New().String(),
		"limit":   "10",
	}
	data := []map[string]interface{}{
		{"id": 1, "name": "test1"},
		{"id": 2, "name": "test2"},
	}

	t.Run("Cache and retrieve materialized view", func(t *testing.T) {
		err := cache.CacheMaterializedView(ctx, viewName, filters, data)
		require.NoError(t, err)

		var retrieved []map[string]interface{}
		found, err := cache.GetMaterializedView(ctx, viewName, filters, &retrieved)
		require.NoError(t, err)
		assert.True(t, found)
		assert.Len(t, retrieved, 2)
	})

	t.Run("Invalidate materialized view cache", func(t *testing.T) {
		err := cache.CacheMaterializedView(ctx, viewName, filters, data)
		require.NoError(t, err)

		err = cache.InvalidateMaterializedViewCache(ctx, viewName)
		require.NoError(t, err)

		var retrieved []map[string]interface{}
		found, _ := cache.GetMaterializedView(ctx, viewName, filters, &retrieved)
		assert.False(t, found)
	})
}

func TestCounterOperations(t *testing.T) {
	redisClient, cleanup := testutil.SetupRedisContainer(t)
	defer cleanup()

	logger := zap.NewNop()
	cache := NewAnalyticsCache(redisClient, logger)
	ctx := context.Background()

	t.Run("Increment counter", func(t *testing.T) {
		key := "test:counter"

		// Increment multiple times
		for i := 0; i < 5; i++ {
			err := cache.IncrementCounter(ctx, key, 1*time.Minute)
			require.NoError(t, err)
		}

		// Check value
		val, err := cache.GetCounter(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, int64(5), val)
	})

	t.Run("Set counter with expiry", func(t *testing.T) {
		key := "test:counter:set"
		err := cache.SetCounterWithExpiry(ctx, key, 100, 1*time.Minute)
		require.NoError(t, err)

		val, err := cache.GetCounter(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, int64(100), val)
	})

	t.Run("Get non-existent counter", func(t *testing.T) {
		val, err := cache.GetCounter(ctx, "non:existent:counter")
		require.NoError(t, err)
		assert.Equal(t, int64(0), val)
	})
}

func TestCacheStats(t *testing.T) {
	redisClient, cleanup := testutil.SetupRedisContainer(t)
	defer cleanup()

	logger := zap.NewNop()
	cache := NewAnalyticsCache(redisClient, logger)
	ctx := context.Background()

	// Add some test data
	userID := uuid.New()
	cache.CacheUserDashboard(ctx, userID, "week", map[string]interface{}{"test": "data"})
	cache.CacheDailySummary(ctx, userID, time.Now(), &DailyActivitySummary{})

	t.Run("Get cache stats", func(t *testing.T) {
		stats, err := cache.GetCacheStats(ctx)
		require.NoError(t, err)
		assert.NotNil(t, stats)

		// Should have some keys
		if totalKeys, ok := stats["total_keys"].(int); ok {
			assert.Greater(t, totalKeys, 0)
		}
	})
}

func TestInvalidatePattern(t *testing.T) {
	redisClient, cleanup := testutil.SetupRedisContainer(t)
	defer cleanup()

	logger := zap.NewNop()
	cache := NewAnalyticsCache(redisClient, logger)
	ctx := context.Background()

	// Create multiple keys with same pattern
	userID := uuid.New()
	cache.CacheUserDashboard(ctx, userID, "week", map[string]interface{}{"test": 1})
	cache.CacheUserDashboard(ctx, userID, "month", map[string]interface{}{"test": 2})
	cache.CacheUserDashboard(ctx, uuid.New(), "week", map[string]interface{}{"test": 3})

	t.Run("Invalidate by pattern", func(t *testing.T) {
		// Invalidate all dashboards for the specific user
		pattern := cache.CacheKey("dashboard", userID.String(), "*")
		err := cache.InvalidatePattern(ctx, pattern)
		require.NoError(t, err)

		// Check that user's dashboards are gone
		var result map[string]interface{}
		found, _ := cache.GetUserDashboard(ctx, userID, "week", &result)
		assert.False(t, found)

		found, _ = cache.GetUserDashboard(ctx, userID, "month", &result)
		assert.False(t, found)
	})
}

func TestCacheFlush(t *testing.T) {
	redisClient, cleanup := testutil.SetupRedisContainer(t)
	defer cleanup()

	logger := zap.NewNop()
	cache := NewAnalyticsCache(redisClient, logger)
	ctx := context.Background()

	// Add some test data
	cache.Set(ctx, cache.CacheKey("test1"), "value1", 1*time.Minute)
	cache.Set(ctx, cache.CacheKey("test2"), "value2", 1*time.Minute)

	t.Run("Flush all cache", func(t *testing.T) {
		err := cache.Flush(ctx)
		require.NoError(t, err)

		// Verify all keys are gone
		var result string
		found, _ := cache.Get(ctx, cache.CacheKey("test1"), &result)
		assert.False(t, found)

		found, _ = cache.Get(ctx, cache.CacheKey("test2"), &result)
		assert.False(t, found)
	})
}

func TestCacheTTL(t *testing.T) {
	redisClient, cleanup := testutil.SetupRedisContainer(t)
	defer cleanup()

	logger := zap.NewNop()
	cache := NewAnalyticsCache(redisClient, logger)
	ctx := context.Background()

	t.Run("Cache expires after TTL", func(t *testing.T) {
		key := "test:ttl"
		data := "test_value"

		err := cache.Set(ctx, key, data, 1*time.Second)
		require.NoError(t, err)

		// Verify it exists initially
		var result string
		found, err := cache.Get(ctx, key, &result)
		require.NoError(t, err)
		assert.True(t, found)

		// Wait for expiration
		time.Sleep(2 * time.Second)

		// Should be gone now
		found, err = cache.Get(ctx, key, &result)
		require.NoError(t, err)
		assert.False(t, found)
	})
}
