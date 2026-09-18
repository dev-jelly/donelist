package cache

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupTestInvalidator(t *testing.T) (*Invalidator, *Cache, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	logger, _ := zap.NewDevelopment()
	cache := NewCache(client, Config{
		Prefix:     "test:",
		DefaultTTL: TTLUserData,
	}, logger)

	invalidator := NewInvalidator(cache, logger)

	return invalidator, cache, mr
}

func TestInvalidator_InvalidateUser(t *testing.T) {
	inv, cache, mr := setupTestInvalidator(t)
	defer mr.Close()

	ctx := context.Background()
	userID := int64(123)

	// Set various user-related cache entries
	cache.Set(ctx, MakeUserKey(userID), "user data", TTLUserData)
	cache.Set(ctx, MakeCheckinsKey(userID, "2024-01-15"), "checkins", TTLCheckins)
	cache.Set(ctx, MakeCategoriesKey(userID), "categories", TTLCategories)
	cache.Set(ctx, MakeTagsKey(userID), "tags", TTLTags)

	// Set cache for another user (should not be invalidated)
	cache.Set(ctx, MakeUserKey(456), "other user", TTLUserData)

	// Invalidate all cache for user 123
	err := inv.InvalidateUser(ctx, userID)
	require.NoError(t, err)

	// Verify user 123 cache is gone
	var result string
	err = cache.Get(ctx, MakeUserKey(userID), &result)
	assert.ErrorIs(t, err, ErrCacheMiss)

	// Verify other user cache still exists
	err = cache.Get(ctx, MakeUserKey(456), &result)
	require.NoError(t, err)
}

func TestInvalidator_InvalidateCheckins(t *testing.T) {
	inv, cache, mr := setupTestInvalidator(t)
	defer mr.Close()

	ctx := context.Background()
	userID := int64(123)
	date := "2024-01-15"

	// Set checkin-related cache
	cache.Set(ctx, MakeCheckinsKey(userID, date), "checkins", TTLCheckins)
	cache.Set(ctx, MakeTimelineKey(userID, date), "timeline", TTLCheckins)
	cache.Set(ctx, MakeStatisticsKey(userID, "2024-01-01", "2024-01-31"), "stats", TTLStatistics)

	// Set cache for different date (should not be invalidated)
	cache.Set(ctx, MakeCheckinsKey(userID, "2024-01-16"), "other checkins", TTLCheckins)

	// Invalidate checkins for specific date
	err := inv.InvalidateCheckins(ctx, userID, date)
	require.NoError(t, err)

	// Verify specific date cache is gone
	var result string
	err = cache.Get(ctx, MakeCheckinsKey(userID, date), &result)
	assert.ErrorIs(t, err, ErrCacheMiss)

	// Verify stats are also invalidated (dependent data)
	err = cache.Get(ctx, MakeStatisticsKey(userID, "2024-01-01", "2024-01-31"), &result)
	assert.ErrorIs(t, err, ErrCacheMiss)
}

func TestInvalidator_InvalidateCategories(t *testing.T) {
	inv, cache, mr := setupTestInvalidator(t)
	defer mr.Close()

	ctx := context.Background()
	userID := int64(123)

	// Set category-related cache
	cache.Set(ctx, MakeCategoriesKey(userID), "categories", TTLCategories)
	cache.Set(ctx, MakeTimelineKey(userID, "2024-01-15"), "timeline", TTLCheckins)

	// Invalidate categories
	err := inv.InvalidateCategories(ctx, userID)
	require.NoError(t, err)

	// Verify category cache is gone
	var result string
	err = cache.Get(ctx, MakeCategoriesKey(userID), &result)
	assert.ErrorIs(t, err, ErrCacheMiss)
}

func TestInvalidator_InvalidateTags(t *testing.T) {
	inv, cache, mr := setupTestInvalidator(t)
	defer mr.Close()

	ctx := context.Background()
	userID := int64(123)

	// Set tag-related cache
	cache.Set(ctx, MakeTagsKey(userID), "tags", TTLTags)
	cache.Set(ctx, MakeSearchKey(userID, "test", 10, 0), "search", TTLSearchResults)

	// Invalidate tags
	err := inv.InvalidateTags(ctx, userID)
	require.NoError(t, err)

	// Verify tag cache is gone
	var result string
	err = cache.Get(ctx, MakeTagsKey(userID), &result)
	assert.ErrorIs(t, err, ErrCacheMiss)
}

func TestInvalidator_InvalidateStatistics(t *testing.T) {
	inv, cache, mr := setupTestInvalidator(t)
	defer mr.Close()

	ctx := context.Background()
	userID := int64(123)

	// Set statistics cache for different date ranges
	cache.Set(ctx, MakeStatisticsKey(userID, "2024-01-01", "2024-01-31"), "stats1", TTLStatistics)
	cache.Set(ctx, MakeStatisticsKey(userID, "2024-02-01", "2024-02-29"), "stats2", TTLStatistics)

	t.Run("Invalidate specific date range", func(t *testing.T) {
		err := inv.InvalidateStatistics(ctx, userID, "2024-01-01", "2024-01-31")
		require.NoError(t, err)

		// Verify specific range is gone
		var result string
		err = cache.Get(ctx, MakeStatisticsKey(userID, "2024-01-01", "2024-01-31"), &result)
		assert.ErrorIs(t, err, ErrCacheMiss)

		// Verify other range still exists
		err = cache.Get(ctx, MakeStatisticsKey(userID, "2024-02-01", "2024-02-29"), &result)
		require.NoError(t, err)
	})

	t.Run("Invalidate all statistics", func(t *testing.T) {
		err := inv.InvalidateStatistics(ctx, userID, "", "")
		require.NoError(t, err)

		// Verify all stats are gone
		var result string
		err = cache.Get(ctx, MakeStatisticsKey(userID, "2024-02-01", "2024-02-29"), &result)
		assert.ErrorIs(t, err, ErrCacheMiss)
	})
}

func TestInvalidator_InvalidateCalendar(t *testing.T) {
	inv, cache, mr := setupTestInvalidator(t)
	defer mr.Close()

	ctx := context.Background()
	userID := int64(123)

	// Set calendar cache for different months
	cache.Set(ctx, MakeCalendarKey(userID, 2024, 1), "jan", TTLCheckins)
	cache.Set(ctx, MakeCalendarKey(userID, 2024, 2), "feb", TTLCheckins)

	t.Run("Invalidate specific month", func(t *testing.T) {
		err := inv.InvalidateCalendar(ctx, userID, 2024, 1)
		require.NoError(t, err)

		// Verify specific month is gone
		var result string
		err = cache.Get(ctx, MakeCalendarKey(userID, 2024, 1), &result)
		assert.ErrorIs(t, err, ErrCacheMiss)

		// Verify other month still exists
		err = cache.Get(ctx, MakeCalendarKey(userID, 2024, 2), &result)
		require.NoError(t, err)
	})

	t.Run("Invalidate all calendar", func(t *testing.T) {
		err := inv.InvalidateCalendar(ctx, userID, 0, 0)
		require.NoError(t, err)

		// Verify all calendar is gone
		var result string
		err = cache.Get(ctx, MakeCalendarKey(userID, 2024, 2), &result)
		assert.ErrorIs(t, err, ErrCacheMiss)
	})
}

func TestInvalidator_InvalidateOnCheckinCreate(t *testing.T) {
	inv, cache, mr := setupTestInvalidator(t)
	defer mr.Close()

	ctx := context.Background()
	userID := int64(123)
	date := "2024-01-15"

	// Set all related caches that should be invalidated
	cache.Set(ctx, MakeCheckinsKey(userID, date), "checkins", TTLCheckins)
	cache.Set(ctx, MakeTimelineKey(userID, date), "timeline", TTLCheckins)
	cache.Set(ctx, MakeStatisticsKey(userID, "2024-01-01", "2024-01-31"), "stats", TTLStatistics)
	cache.Set(ctx, MakeCalendarKey(userID, 2024, 1), "calendar", TTLCheckins)
	cache.Set(ctx, MakeSearchKey(userID, "test", 10, 0), "search", TTLSearchResults)

	// Trigger checkin create invalidation
	err := inv.InvalidateOnCheckinCreate(ctx, userID, date)
	require.NoError(t, err)

	// Verify all affected caches are invalidated
	var result string

	err = cache.Get(ctx, MakeCheckinsKey(userID, date), &result)
	assert.ErrorIs(t, err, ErrCacheMiss, "checkins should be invalidated")

	err = cache.Get(ctx, MakeTimelineKey(userID, date), &result)
	assert.ErrorIs(t, err, ErrCacheMiss, "timeline should be invalidated")

	err = cache.Get(ctx, MakeStatisticsKey(userID, "2024-01-01", "2024-01-31"), &result)
	assert.ErrorIs(t, err, ErrCacheMiss, "statistics should be invalidated")

	err = cache.Get(ctx, MakeCalendarKey(userID, 2024, 1), &result)
	assert.ErrorIs(t, err, ErrCacheMiss, "calendar should be invalidated")

	err = cache.Get(ctx, MakeSearchKey(userID, "test", 10, 0), &result)
	assert.ErrorIs(t, err, ErrCacheMiss, "search should be invalidated")
}

func TestInvalidator_InvalidateOnCheckinUpdate(t *testing.T) {
	inv, cache, mr := setupTestInvalidator(t)
	defer mr.Close()

	ctx := context.Background()
	userID := int64(123)
	oldDate := "2024-01-15"
	newDate := "2024-01-16"

	// Set caches for both dates
	cache.Set(ctx, MakeCheckinsKey(userID, oldDate), "old checkins", TTLCheckins)
	cache.Set(ctx, MakeCheckinsKey(userID, newDate), "new checkins", TTLCheckins)

	// Update changes date
	err := inv.InvalidateOnCheckinUpdate(ctx, userID, oldDate, newDate)
	require.NoError(t, err)

	// Verify both dates are invalidated
	var result string
	err = cache.Get(ctx, MakeCheckinsKey(userID, oldDate), &result)
	assert.ErrorIs(t, err, ErrCacheMiss, "old date should be invalidated")

	err = cache.Get(ctx, MakeCheckinsKey(userID, newDate), &result)
	assert.ErrorIs(t, err, ErrCacheMiss, "new date should be invalidated")
}

func TestInvalidator_InvalidateOnCheckinDelete(t *testing.T) {
	inv, cache, mr := setupTestInvalidator(t)
	defer mr.Close()

	ctx := context.Background()
	userID := int64(123)
	date := "2024-01-15"

	// Set related caches
	cache.Set(ctx, MakeCheckinsKey(userID, date), "checkins", TTLCheckins)
	cache.Set(ctx, MakeStatisticsKey(userID, "2024-01-01", "2024-01-31"), "stats", TTLStatistics)

	// Trigger delete invalidation
	err := inv.InvalidateOnCheckinDelete(ctx, userID, date)
	require.NoError(t, err)

	// Verify caches are invalidated
	var result string
	err = cache.Get(ctx, MakeCheckinsKey(userID, date), &result)
	assert.ErrorIs(t, err, ErrCacheMiss)
}
