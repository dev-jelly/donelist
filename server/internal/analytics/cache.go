package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// CacheTTL defines cache expiration times for different data types
var (
	CacheTTLRealtime     = 1 * time.Minute      // Real-time metrics
	CacheTTLHourly       = 5 * time.Minute      // Hourly aggregates
	CacheTTLDaily        = 30 * time.Minute     // Daily summaries
	CacheTTLWeekly       = 2 * time.Hour        // Weekly reports
	CacheTTLMonthly      = 6 * time.Hour        // Monthly reports
	CacheTTLTrends       = 15 * time.Minute     // Trend data
	CacheTTLDashboard    = 10 * time.Minute     // User dashboards
	CacheTTLMaterialized = 1 * time.Hour        // Materialized view results
)

// AnalyticsCache provides Redis caching for analytics data
type AnalyticsCache struct {
	redis  *redis.Client
	logger *zap.Logger
	prefix string // Cache key prefix for namespacing
}

// NewAnalyticsCache creates a new analytics cache
func NewAnalyticsCache(redis *redis.Client, logger *zap.Logger) *AnalyticsCache {
	return &AnalyticsCache{
		redis:  redis,
		logger: logger,
		prefix: "analytics",
	}
}

// CacheKey generates a namespaced cache key
func (c *AnalyticsCache) CacheKey(parts ...string) string {
	key := c.prefix
	for _, part := range parts {
		key += ":" + part
	}
	return key
}

// Set stores a value in cache with TTL
func (c *AnalyticsCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		c.logger.Error("Failed to marshal cache value",
			zap.String("key", key),
			zap.Error(err))
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	err = c.redis.Set(ctx, key, data, ttl).Err()
	if err != nil {
		c.logger.Error("Failed to set cache value",
			zap.String("key", key),
			zap.Error(err))
		return fmt.Errorf("failed to set cache: %w", err)
	}

	c.logger.Debug("Cache set",
		zap.String("key", key),
		zap.Duration("ttl", ttl))

	return nil
}

// Get retrieves a value from cache
func (c *AnalyticsCache) Get(ctx context.Context, key string, dest interface{}) (bool, error) {
	data, err := c.redis.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		c.logger.Error("Failed to get cache value",
			zap.String("key", key),
			zap.Error(err))
		return false, fmt.Errorf("failed to get cache: %w", err)
	}

	err = json.Unmarshal(data, dest)
	if err != nil {
		c.logger.Error("Failed to unmarshal cache value",
			zap.String("key", key),
			zap.Error(err))
		return false, fmt.Errorf("failed to unmarshal value: %w", err)
	}

	c.logger.Debug("Cache hit", zap.String("key", key))
	return true, nil
}

// Delete removes a key from cache
func (c *AnalyticsCache) Delete(ctx context.Context, key string) error {
	err := c.redis.Del(ctx, key).Err()
	if err != nil {
		c.logger.Error("Failed to delete cache key",
			zap.String("key", key),
			zap.Error(err))
		return fmt.Errorf("failed to delete cache: %w", err)
	}

	c.logger.Debug("Cache key deleted", zap.String("key", key))
	return nil
}

// InvalidatePattern removes all keys matching a pattern
func (c *AnalyticsCache) InvalidatePattern(ctx context.Context, pattern string) error {
	keys, err := c.redis.Keys(ctx, pattern).Result()
	if err != nil {
		c.logger.Error("Failed to get keys for pattern",
			zap.String("pattern", pattern),
			zap.Error(err))
		return fmt.Errorf("failed to get keys: %w", err)
	}

	if len(keys) == 0 {
		return nil
	}

	err = c.redis.Del(ctx, keys...).Err()
	if err != nil {
		c.logger.Error("Failed to delete cache keys",
			zap.String("pattern", pattern),
			zap.Int("count", len(keys)),
			zap.Error(err))
		return fmt.Errorf("failed to delete keys: %w", err)
	}

	c.logger.Info("Cache invalidated",
		zap.String("pattern", pattern),
		zap.Int("keys_deleted", len(keys)))

	return nil
}

// CacheUserDashboard caches a user's analytics dashboard
func (c *AnalyticsCache) CacheUserDashboard(ctx context.Context, userID uuid.UUID, period string, data interface{}) error {
	key := c.CacheKey("dashboard", userID.String(), period)
	return c.Set(ctx, key, data, CacheTTLDashboard)
}

// GetUserDashboard retrieves a cached dashboard
func (c *AnalyticsCache) GetUserDashboard(ctx context.Context, userID uuid.UUID, period string, dest interface{}) (bool, error) {
	key := c.CacheKey("dashboard", userID.String(), period)
	return c.Get(ctx, key, dest)
}

// InvalidateUserCache invalidates all cache for a specific user
func (c *AnalyticsCache) InvalidateUserCache(ctx context.Context, userID uuid.UUID) error {
	pattern := c.CacheKey("*", userID.String(), "*")
	return c.InvalidatePattern(ctx, pattern)
}

// CacheDailySummary caches daily summary data
func (c *AnalyticsCache) CacheDailySummary(ctx context.Context, userID uuid.UUID, date time.Time, data interface{}) error {
	dateStr := date.Format("2006-01-02")
	key := c.CacheKey("daily", userID.String(), dateStr)
	return c.Set(ctx, key, data, CacheTTLDaily)
}

// GetDailySummary retrieves cached daily summary
func (c *AnalyticsCache) GetDailySummary(ctx context.Context, userID uuid.UUID, date time.Time, dest interface{}) (bool, error) {
	dateStr := date.Format("2006-01-02")
	key := c.CacheKey("daily", userID.String(), dateStr)
	return c.Get(ctx, key, dest)
}

// CacheWeeklySummary caches weekly summary data
func (c *AnalyticsCache) CacheWeeklySummary(ctx context.Context, userID uuid.UUID, weekStart time.Time, data interface{}) error {
	weekStr := weekStart.Format("2006-W02")
	key := c.CacheKey("weekly", userID.String(), weekStr)
	return c.Set(ctx, key, data, CacheTTLWeekly)
}

// GetWeeklySummary retrieves cached weekly summary
func (c *AnalyticsCache) GetWeeklySummary(ctx context.Context, userID uuid.UUID, weekStart time.Time, dest interface{}) (bool, error) {
	weekStr := weekStart.Format("2006-W02")
	key := c.CacheKey("weekly", userID.String(), weekStr)
	return c.Get(ctx, key, dest)
}

// CacheCategoryTrends caches category trend data
func (c *AnalyticsCache) CacheCategoryTrends(ctx context.Context, userID uuid.UUID, categoryID uuid.UUID, period string, data interface{}) error {
	key := c.CacheKey("trends", "category", userID.String(), categoryID.String(), period)
	return c.Set(ctx, key, data, CacheTTLTrends)
}

// GetCategoryTrends retrieves cached category trends
func (c *AnalyticsCache) GetCategoryTrends(ctx context.Context, userID uuid.UUID, categoryID uuid.UUID, period string, dest interface{}) (bool, error) {
	key := c.CacheKey("trends", "category", userID.String(), categoryID.String(), period)
	return c.Get(ctx, key, dest)
}

// CacheStreakInfo caches user streak information
func (c *AnalyticsCache) CacheStreakInfo(ctx context.Context, userID uuid.UUID, data interface{}) error {
	key := c.CacheKey("streak", userID.String())
	return c.Set(ctx, key, data, CacheTTLDaily)
}

// GetStreakInfo retrieves cached streak information
func (c *AnalyticsCache) GetStreakInfo(ctx context.Context, userID uuid.UUID, dest interface{}) (bool, error) {
	key := c.CacheKey("streak", userID.String())
	return c.Get(ctx, key, dest)
}

// CacheMaterializedView caches results from materialized views
func (c *AnalyticsCache) CacheMaterializedView(ctx context.Context, viewName string, filters map[string]string, data interface{}) error {
	// Create a stable key from filters
	filterKey := ""
	if filters != nil && len(filters) > 0 {
		filterData, _ := json.Marshal(filters)
		filterKey = string(filterData)
	}

	key := c.CacheKey("mv", viewName, filterKey)
	return c.Set(ctx, key, data, CacheTTLMaterialized)
}

// GetMaterializedView retrieves cached materialized view results
func (c *AnalyticsCache) GetMaterializedView(ctx context.Context, viewName string, filters map[string]string, dest interface{}) (bool, error) {
	filterKey := ""
	if filters != nil && len(filters) > 0 {
		filterData, _ := json.Marshal(filters)
		filterKey = string(filterData)
	}

	key := c.CacheKey("mv", viewName, filterKey)
	return c.Get(ctx, key, dest)
}

// InvalidateMaterializedViewCache invalidates all cached materialized view results
func (c *AnalyticsCache) InvalidateMaterializedViewCache(ctx context.Context, viewName string) error {
	pattern := c.CacheKey("mv", viewName, "*")
	return c.InvalidatePattern(ctx, pattern)
}

// IncrementCounter increments a counter in Redis
func (c *AnalyticsCache) IncrementCounter(ctx context.Context, key string, ttl time.Duration) error {
	pipe := c.redis.Pipeline()
	pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, ttl)
	_, err := pipe.Exec(ctx)

	if err != nil {
		c.logger.Error("Failed to increment counter",
			zap.String("key", key),
			zap.Error(err))
		return fmt.Errorf("failed to increment counter: %w", err)
	}

	return nil
}

// GetCounter retrieves a counter value
func (c *AnalyticsCache) GetCounter(ctx context.Context, key string) (int64, error) {
	val, err := c.redis.Get(ctx, key).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		c.logger.Error("Failed to get counter",
			zap.String("key", key),
			zap.Error(err))
		return 0, fmt.Errorf("failed to get counter: %w", err)
	}

	return val, nil
}

// SetCounterWithExpiry sets a counter with explicit expiry
func (c *AnalyticsCache) SetCounterWithExpiry(ctx context.Context, key string, value int64, ttl time.Duration) error {
	err := c.redis.Set(ctx, key, value, ttl).Err()
	if err != nil {
		c.logger.Error("Failed to set counter",
			zap.String("key", key),
			zap.Error(err))
		return fmt.Errorf("failed to set counter: %w", err)
	}

	return nil
}

// CacheHeatmapData caches heatmap visualization data
func (c *AnalyticsCache) CacheHeatmapData(ctx context.Context, userID uuid.UUID, dataType string, data interface{}) error {
	key := c.CacheKey("heatmap", dataType, userID.String())
	return c.Set(ctx, key, data, CacheTTLDaily)
}

// GetHeatmapData retrieves cached heatmap data
func (c *AnalyticsCache) GetHeatmapData(ctx context.Context, userID uuid.UUID, dataType string, dest interface{}) (bool, error) {
	key := c.CacheKey("heatmap", dataType, userID.String())
	return c.Get(ctx, key, dest)
}

// WarmCache pre-populates cache with frequently accessed data
func (c *AnalyticsCache) WarmCache(ctx context.Context, userID uuid.UUID, warmFunc func(context.Context) error) error {
	c.logger.Info("Warming cache for user", zap.String("user_id", userID.String()))

	if err := warmFunc(ctx); err != nil {
		c.logger.Error("Failed to warm cache",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		return fmt.Errorf("failed to warm cache: %w", err)
	}

	c.logger.Info("Cache warmed successfully", zap.String("user_id", userID.String()))
	return nil
}

// GetCacheStats returns cache statistics
func (c *AnalyticsCache) GetCacheStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Get Redis info
	info, err := c.redis.Info(ctx, "stats").Result()
	if err != nil {
		c.logger.Warn("Failed to get Redis stats", zap.Error(err))
		return stats, nil
	}

	stats["redis_info"] = info

	// Count keys by prefix
	pattern := c.CacheKey("*")
	keys, err := c.redis.Keys(ctx, pattern).Result()
	if err != nil {
		c.logger.Warn("Failed to count cache keys", zap.Error(err))
	} else {
		stats["total_keys"] = len(keys)

		// Count by type
		typeCounts := make(map[string]int)
		for _, key := range keys {
			// Extract type from key (first part after prefix)
			parts := []byte(key)
			prefixLen := len(c.prefix) + 1
			if len(parts) > prefixLen {
				typeEnd := prefixLen
				for i := prefixLen; i < len(parts); i++ {
					if parts[i] == ':' {
						typeEnd = i
						break
					}
				}
				keyType := string(parts[prefixLen:typeEnd])
				typeCounts[keyType]++
			}
		}
		stats["keys_by_type"] = typeCounts
	}

	return stats, nil
}

// Flush removes all analytics cache entries
func (c *AnalyticsCache) Flush(ctx context.Context) error {
	pattern := c.CacheKey("*")
	return c.InvalidatePattern(ctx, pattern)
}
