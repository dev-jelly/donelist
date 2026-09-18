package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Cache provides Redis-based caching functionality
type Cache struct {
	client  *redis.Client
	logger  *zap.Logger
	prefix  string
	metrics *Metrics
}

// Metrics tracks cache performance
type Metrics struct {
	Hits   int64
	Misses int64
	Errors int64
}

// Config holds cache configuration
type Config struct {
	Prefix string
	DefaultTTL time.Duration
}

// TTL presets for different data types
const (
	TTLUserData     = 5 * time.Minute  // User data changes moderately
	TTLCheckins     = 30 * time.Minute // Checkins are relatively stable
	TTLCategories   = 1 * time.Hour    // Categories change infrequently
	TTLTags         = 1 * time.Hour    // Tags change infrequently
	TTLStatistics   = 15 * time.Minute // Statistics recalculated periodically
	TTLSearchResults = 10 * time.Minute // Search results can be cached briefly
	TTLShortLived   = 1 * time.Minute  // For very dynamic data
)

// NewCache creates a new cache instance
func NewCache(client *redis.Client, cfg Config, logger *zap.Logger) *Cache {
	if cfg.DefaultTTL == 0 {
		cfg.DefaultTTL = 5 * time.Minute
	}
	if cfg.Prefix == "" {
		cfg.Prefix = "donelist:"
	}

	return &Cache{
		client:  client,
		logger:  logger,
		prefix:  cfg.Prefix,
		metrics: &Metrics{},
	}
}

// Get retrieves a value from cache and unmarshals it
func (c *Cache) Get(ctx context.Context, key string, dest interface{}) error {
	fullKey := c.makeKey(key)

	val, err := c.client.Get(ctx, fullKey).Result()
	if err != nil {
		if err == redis.Nil {
			c.metrics.Misses++
			c.logger.Debug("Cache miss", zap.String("key", key))
			return ErrCacheMiss
		}
		c.metrics.Errors++
		c.logger.Error("Cache get error", zap.String("key", key), zap.Error(err))
		return fmt.Errorf("cache get error: %w", err)
	}

	c.metrics.Hits++
	c.logger.Debug("Cache hit", zap.String("key", key))

	if err := json.Unmarshal([]byte(val), dest); err != nil {
		c.metrics.Errors++
		return fmt.Errorf("cache unmarshal error: %w", err)
	}

	return nil
}

// Set stores a value in cache with the specified TTL
func (c *Cache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	fullKey := c.makeKey(key)

	data, err := json.Marshal(value)
	if err != nil {
		c.metrics.Errors++
		return fmt.Errorf("cache marshal error: %w", err)
	}

	if err := c.client.Set(ctx, fullKey, data, ttl).Err(); err != nil {
		c.metrics.Errors++
		c.logger.Error("Cache set error", zap.String("key", key), zap.Error(err))
		return fmt.Errorf("cache set error: %w", err)
	}

	c.logger.Debug("Cache set", zap.String("key", key), zap.Duration("ttl", ttl))
	return nil
}

// Delete removes a value from cache
func (c *Cache) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}

	fullKeys := make([]string, len(keys))
	for i, key := range keys {
		fullKeys[i] = c.makeKey(key)
	}

	if err := c.client.Del(ctx, fullKeys...).Err(); err != nil {
		c.metrics.Errors++
		c.logger.Error("Cache delete error", zap.Strings("keys", keys), zap.Error(err))
		return fmt.Errorf("cache delete error: %w", err)
	}

	c.logger.Debug("Cache delete", zap.Strings("keys", keys))
	return nil
}

// DeletePattern deletes all keys matching a pattern
func (c *Cache) DeletePattern(ctx context.Context, pattern string) error {
	fullPattern := c.makeKey(pattern)

	iter := c.client.Scan(ctx, 0, fullPattern, 0).Iterator()
	var keys []string

	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}

	if err := iter.Err(); err != nil {
		c.metrics.Errors++
		c.logger.Error("Cache scan error", zap.String("pattern", pattern), zap.Error(err))
		return fmt.Errorf("cache scan error: %w", err)
	}

	if len(keys) > 0 {
		if err := c.client.Del(ctx, keys...).Err(); err != nil {
			c.metrics.Errors++
			c.logger.Error("Cache delete pattern error", zap.String("pattern", pattern), zap.Error(err))
			return fmt.Errorf("cache delete pattern error: %w", err)
		}
		c.logger.Debug("Cache delete pattern", zap.String("pattern", pattern), zap.Int("count", len(keys)))
	}

	return nil
}

// Exists checks if a key exists in cache
func (c *Cache) Exists(ctx context.Context, key string) (bool, error) {
	fullKey := c.makeKey(key)

	n, err := c.client.Exists(ctx, fullKey).Result()
	if err != nil {
		c.metrics.Errors++
		return false, fmt.Errorf("cache exists error: %w", err)
	}

	return n > 0, nil
}

// GetOrSet retrieves from cache or sets it using the provided function
func (c *Cache) GetOrSet(ctx context.Context, key string, ttl time.Duration, dest interface{}, fetchFunc func() (interface{}, error)) error {
	// Try to get from cache first
	err := c.Get(ctx, key, dest)
	if err == nil {
		return nil // Cache hit
	}

	if err != ErrCacheMiss {
		// If error is not a cache miss, log but continue to fetch
		c.logger.Warn("Cache get error, fetching from source", zap.String("key", key), zap.Error(err))
	}

	// Cache miss or error, fetch from source
	value, err := fetchFunc()
	if err != nil {
		return fmt.Errorf("fetch function error: %w", err)
	}

	// Store in cache (fire and forget - don't fail if cache set fails)
	if err := c.Set(ctx, key, value, ttl); err != nil {
		c.logger.Warn("Failed to set cache after fetch", zap.String("key", key), zap.Error(err))
	}

	// Marshal the fetched value into dest
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}

	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("unmarshal error: %w", err)
	}

	return nil
}

// GetMetrics returns current cache metrics
func (c *Cache) GetMetrics() Metrics {
	return *c.metrics
}

// ResetMetrics resets cache metrics
func (c *Cache) ResetMetrics() {
	c.metrics = &Metrics{}
}

// makeKey creates a full cache key with prefix
func (c *Cache) makeKey(key string) string {
	return c.prefix + key
}

// MakeUserKey creates a cache key for user data
func MakeUserKey(userID int64) string {
	return fmt.Sprintf("user:%d", userID)
}

// MakeCheckinsKey creates a cache key for user checkins
func MakeCheckinsKey(userID int64, date string) string {
	return fmt.Sprintf("checkins:%d:%s", userID, date)
}

// MakeUserCheckinsListKey creates a cache key for user checkin lists with pagination
func MakeUserCheckinsListKey(userID int64, limit, offset int) string {
	return fmt.Sprintf("checkins:list:%d:%d:%d", userID, limit, offset)
}

// MakeCategoriesKey creates a cache key for user categories
func MakeCategoriesKey(userID int64) string {
	return fmt.Sprintf("categories:%d", userID)
}

// MakeTagsKey creates a cache key for user tags
func MakeTagsKey(userID int64) string {
	return fmt.Sprintf("tags:%d", userID)
}

// MakeStatisticsKey creates a cache key for user statistics
func MakeStatisticsKey(userID int64, startDate, endDate string) string {
	return fmt.Sprintf("stats:%d:%s:%s", userID, startDate, endDate)
}

// MakeSearchKey creates a cache key for search results
func MakeSearchKey(userID int64, query string, limit, offset int) string {
	return fmt.Sprintf("search:%d:%s:%d:%d", userID, query, limit, offset)
}

// MakeCalendarKey creates a cache key for calendar data
func MakeCalendarKey(userID int64, year, month int) string {
	return fmt.Sprintf("calendar:%d:%d:%d", userID, year, month)
}

// MakeTimelineKey creates a cache key for timeline data
func MakeTimelineKey(userID int64, date string) string {
	return fmt.Sprintf("timeline:%d:%s", userID, date)
}
