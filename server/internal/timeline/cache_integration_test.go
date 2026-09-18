package timeline

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupTestRedis(t *testing.T) *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   1, // Use DB 1 for testing
	})

	// Ping to check connection
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available for testing:", err)
	}

	// Flush test database
	client.FlushDB(ctx)

	return client
}

func TestTimelineCacheIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup Redis
	redisClient := setupTestRedis(t)
	defer redisClient.Close()

	// Setup logger
	logger, _ := zap.NewDevelopment()

	// Setup cache service
	cacheService := NewCacheService(redisClient, logger)

	ctx := context.Background()
	userID := uuid.New()
	date := "2024-01-15"
	timezone := "UTC"
	blockGranularity := 30
	limit := 0
	cursor := ""

	// Test data
	view := &EnhancedDayView{
		Date:             date,
		Timezone:         timezone,
		BlockGranularity: blockGranularity,
		Blocks:           []*TimeBlock{},
		Gaps:             []*Gap{},
		Summary: &DailySummary{
			Date:          date,
			TotalCheckins: 5,
			TotalMinutes:  150,
		},
		CategoryLegend: []*CategoryLegend{},
		PreviousDay:    "2024-01-14",
		NextDay:        "2024-01-16",
		GeneratedAt:    time.Now().UTC(),
	}

	t.Run("Cache Set and Get", func(t *testing.T) {
		key := cacheService.CacheKey(userID, date, timezone, blockGranularity, limit, cursor)
		ttl := 5 * time.Minute

		// Set cache
		err := cacheService.Set(ctx, key, view, ttl)
		require.NoError(t, err)

		// Get from cache
		cached, err := cacheService.Get(ctx, key)
		require.NoError(t, err)
		require.NotNil(t, cached)

		assert.Equal(t, view.Date, cached.Date)
		assert.Equal(t, view.Summary.TotalCheckins, cached.Summary.TotalCheckins)
	})

	t.Run("Cache Miss Returns Nil", func(t *testing.T) {
		missingKey := cacheService.CacheKey(uuid.New(), "2099-12-31", timezone, blockGranularity, limit, cursor)

		cached, err := cacheService.Get(ctx, missingKey)
		require.NoError(t, err)
		assert.Nil(t, cached)
	})

	t.Run("ETag Generation and Storage", func(t *testing.T) {
		key := cacheService.CacheKey(userID, date, timezone, blockGranularity, limit, cursor)
		etag := ComputeETag(view)
		ttl := 5 * time.Minute

		// Set ETag
		err := cacheService.SetETag(ctx, key, etag, ttl)
		require.NoError(t, err)

		// Get ETag
		retrievedETag, err := cacheService.GetETag(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, etag, retrievedETag)
	})

	t.Run("Last Modified Timestamp", func(t *testing.T) {
		key := cacheService.CacheKey(userID, date, timezone, blockGranularity, limit, cursor)
		timestamp := time.Now().UTC()
		ttl := 5 * time.Minute

		// Set last modified
		err := cacheService.SetLastModified(ctx, key, timestamp, ttl)
		require.NoError(t, err)

		// Get last modified
		retrieved, err := cacheService.GetLastModified(ctx, key)
		require.NoError(t, err)

		// Allow 1 second difference due to truncation
		assert.WithinDuration(t, timestamp, retrieved, time.Second)
	})

	t.Run("Invalidate Date Cache", func(t *testing.T) {
		// Create multiple cache entries for the same date
		for i := 0; i < 3; i++ {
			key := cacheService.CacheKey(userID, date, timezone, 15+i*15, limit, cursor)
			err := cacheService.Set(ctx, key, view, 5*time.Minute)
			require.NoError(t, err)
		}

		// Invalidate all for this date
		err := cacheService.InvalidateDate(ctx, userID, date)
		require.NoError(t, err)

		// Verify all are invalidated
		for i := 0; i < 3; i++ {
			key := cacheService.CacheKey(userID, date, timezone, 15+i*15, limit, cursor)
			cached, err := cacheService.Get(ctx, key)
			require.NoError(t, err)
			assert.Nil(t, cached)
		}
	})

	t.Run("Invalidate User Timeline", func(t *testing.T) {
		// Create cache entries for multiple dates
		dates := []string{"2024-01-15", "2024-01-16", "2024-01-17"}
		for _, d := range dates {
			key := cacheService.CacheKey(userID, d, timezone, blockGranularity, limit, cursor)
			err := cacheService.Set(ctx, key, view, 5*time.Minute)
			require.NoError(t, err)
		}

		// Invalidate all for this user
		err := cacheService.InvalidateUserTimeline(ctx, userID)
		require.NoError(t, err)

		// Verify all are invalidated
		for _, d := range dates {
			key := cacheService.CacheKey(userID, d, timezone, blockGranularity, limit, cursor)
			cached, err := cacheService.Get(ctx, key)
			require.NoError(t, err)
			assert.Nil(t, cached)
		}
	})

	t.Run("TTL Calculation", func(t *testing.T) {
		now := time.Now().UTC()
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

		// Current day should have short TTL
		ttl := CalculateTTL(today)
		assert.Equal(t, 5*time.Minute, ttl)

		// Past day should have long TTL
		yesterday := today.AddDate(0, 0, -1)
		ttl = CalculateTTL(yesterday)
		assert.Equal(t, 1*time.Hour, ttl)

		// Future day should have medium TTL
		tomorrow := today.AddDate(0, 0, 1)
		ttl = CalculateTTL(tomorrow)
		assert.Equal(t, 15*time.Minute, ttl)
	})
}

func TestTimelineServiceWithCache(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This test requires a real database connection
	// Setup your test database connection here
	t.Skip("Requires database setup - implement when test infrastructure is ready")

	// Example structure for future implementation:
	// - Setup test database with migrations
	// - Create test data (users, categories, checkins)
	// - Test cache hits, misses, and invalidation with real data
	// - Verify cache invalidation on create/update/delete
}

func TestPaginationWithCache(t *testing.T) {
	// Test that pagination works correctly
	// Create test blocks
	blocks := make([]*TimeBlock, 96) // 96 blocks for 24 hours at 15min intervals
	startTime := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	for i := range blocks {
		blocks[i] = &TimeBlock{
			StartTime:    startTime.Add(time.Duration(i*15) * time.Minute),
			EndTime:      startTime.Add(time.Duration((i+1)*15) * time.Minute),
			DurationMins: 15,
			Checkins:     []*CheckinWithMeta{},
			IsEmpty:      true,
		}
	}

	t.Run("Paginate First Page", func(t *testing.T) {
		limit := 24
		paginated, meta, err := PaginateBlocks(blocks, "", limit)
		require.NoError(t, err)

		assert.Len(t, paginated, limit)
		assert.True(t, meta.HasNext)
		assert.NotNil(t, meta.NextCursor)
		assert.Equal(t, 96, meta.TotalCount)
	})

	t.Run("Paginate Second Page", func(t *testing.T) {
		limit := 24
		// Get first page to get cursor
		_, meta1, err := PaginateBlocks(blocks, "", limit)
		require.NoError(t, err)
		require.NotNil(t, meta1.NextCursor)

		// Get second page
		paginated, meta2, err := PaginateBlocks(blocks, *meta1.NextCursor, limit)
		require.NoError(t, err)

		assert.Len(t, paginated, limit)
		assert.True(t, meta2.HasNext)
	})

	t.Run("Paginate Last Page", func(t *testing.T) {
		limit := 24

		// Navigate to last page
		var cursor string
		for i := 0; i < 3; i++ {
			_, meta, err := PaginateBlocks(blocks, cursor, limit)
			require.NoError(t, err)
			if meta.NextCursor != nil {
				cursor = *meta.NextCursor
			}
		}

		// Get last page
		paginated, meta, err := PaginateBlocks(blocks, cursor, limit)
		require.NoError(t, err)

		assert.LessOrEqual(t, len(paginated), limit)
		assert.False(t, meta.HasNext)
		assert.Nil(t, meta.NextCursor)
	})
}

func BenchmarkCacheOperations(b *testing.B) {
	redisClient := setupTestRedis(&testing.T{})
	defer redisClient.Close()

	logger, _ := zap.NewDevelopment()
	cacheService := NewCacheService(redisClient, logger)

	ctx := context.Background()
	userID := uuid.New()
	date := "2024-01-15"
	timezone := "UTC"
	blockGranularity := 30
	key := cacheService.CacheKey(userID, date, timezone, blockGranularity, 0, "")

	view := &EnhancedDayView{
		Date:             date,
		Timezone:         timezone,
		BlockGranularity: blockGranularity,
		Blocks:           make([]*TimeBlock, 48),
		Gaps:             []*Gap{},
		Summary: &DailySummary{
			Date:          date,
			TotalCheckins: 50,
		},
		GeneratedAt: time.Now().UTC(),
	}

	b.Run("Set", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = cacheService.Set(ctx, key, view, 5*time.Minute)
		}
	})

	// Set once for get benchmark
	_ = cacheService.Set(ctx, key, view, 5*time.Minute)

	b.Run("Get", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = cacheService.Get(ctx, key)
		}
	})

	b.Run("InvalidateDate", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = cacheService.InvalidateDate(ctx, userID, date)
		}
	})
}
