package timeline

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// CacheService handles caching for timeline data
type CacheService struct {
	redis  *redis.Client
	logger *zap.Logger
}

// NewCacheService creates a new timeline cache service
func NewCacheService(redis *redis.Client, logger *zap.Logger) *CacheService {
	return &CacheService{
		redis:  redis,
		logger: logger,
	}
}

// CacheKey generates a cache key for timeline data
// Format: timeline:daily:enhanced:{userID}:{date}:{block}:{timezone}:{cursor}:{limit}
func (cs *CacheService) CacheKey(userID uuid.UUID, date, timezone string, blockGranularity, limit int, cursor string) string {
	// Create a hash of the timezone to keep key shorter
	tzHash := fmt.Sprintf("%x", sha256.Sum256([]byte(timezone)))[:8]

	if cursor == "" {
		return fmt.Sprintf("timeline:daily:enhanced:%s:%s:%d:%s:first:%d",
			userID.String(), date, blockGranularity, tzHash, limit)
	}

	// Hash the cursor to keep key manageable
	cursorHash := fmt.Sprintf("%x", sha256.Sum256([]byte(cursor)))[:8]
	return fmt.Sprintf("timeline:daily:enhanced:%s:%s:%d:%s:%s:%d",
		userID.String(), date, blockGranularity, tzHash, cursorHash, limit)
}

// Get retrieves cached enhanced day view
func (cs *CacheService) Get(ctx context.Context, key string) (*EnhancedDayView, error) {
	data, err := cs.redis.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			// Cache miss
			return nil, nil
		}
		cs.logger.Error("Failed to get from cache", zap.String("key", key), zap.Error(err))
		return nil, fmt.Errorf("cache get error: %w", err)
	}

	var view EnhancedDayView
	if err := json.Unmarshal(data, &view); err != nil {
		cs.logger.Error("Failed to unmarshal cached data", zap.String("key", key), zap.Error(err))
		// Return nil to force re-fetch from database
		return nil, nil
	}

	cs.logger.Debug("Cache hit", zap.String("key", key))
	return &view, nil
}

// Set stores enhanced day view in cache
func (cs *CacheService) Set(ctx context.Context, key string, view *EnhancedDayView, ttl time.Duration) error {
	data, err := json.Marshal(view)
	if err != nil {
		cs.logger.Error("Failed to marshal view for caching", zap.String("key", key), zap.Error(err))
		return fmt.Errorf("cache marshal error: %w", err)
	}

	if err := cs.redis.Set(ctx, key, data, ttl).Err(); err != nil {
		cs.logger.Error("Failed to set cache", zap.String("key", key), zap.Error(err))
		return fmt.Errorf("cache set error: %w", err)
	}

	cs.logger.Debug("Cache set", zap.String("key", key), zap.Duration("ttl", ttl))
	return nil
}

// GetETag retrieves ETag for a timeline request
func (cs *CacheService) GetETag(ctx context.Context, key string) (string, error) {
	etagKey := fmt.Sprintf("%s:etag", key)
	etag, err := cs.redis.Get(ctx, etagKey).Result()
	if err != nil {
		if err == redis.Nil {
			return "", nil
		}
		return "", err
	}
	return etag, nil
}

// SetETag stores ETag for a timeline request
func (cs *CacheService) SetETag(ctx context.Context, key, etag string, ttl time.Duration) error {
	etagKey := fmt.Sprintf("%s:etag", key)
	return cs.redis.Set(ctx, etagKey, etag, ttl).Err()
}

// GetLastModified retrieves last modified timestamp for a timeline
func (cs *CacheService) GetLastModified(ctx context.Context, key string) (time.Time, error) {
	lmKey := fmt.Sprintf("%s:last_modified", key)
	timestamp, err := cs.redis.Get(ctx, lmKey).Result()
	if err != nil {
		if err == redis.Nil {
			return time.Time{}, nil
		}
		return time.Time{}, err
	}

	return time.Parse(time.RFC3339, timestamp)
}

// SetLastModified stores last modified timestamp for a timeline
func (cs *CacheService) SetLastModified(ctx context.Context, key string, timestamp time.Time, ttl time.Duration) error {
	lmKey := fmt.Sprintf("%s:last_modified", key)
	return cs.redis.Set(ctx, lmKey, timestamp.Format(time.RFC3339), ttl).Err()
}

// InvalidateUserTimeline invalidates all cached timelines for a user
// This should be called when a user creates, updates, or deletes a check-in
func (cs *CacheService) InvalidateUserTimeline(ctx context.Context, userID uuid.UUID) error {
	pattern := fmt.Sprintf("timeline:daily:enhanced:%s:*", userID.String())

	// Use SCAN to find all matching keys
	iter := cs.redis.Scan(ctx, 0, pattern, 0).Iterator()
	var keys []string

	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
		// Also collect related etag and last_modified keys
		keys = append(keys, fmt.Sprintf("%s:etag", iter.Val()))
		keys = append(keys, fmt.Sprintf("%s:last_modified", iter.Val()))
	}

	if err := iter.Err(); err != nil {
		cs.logger.Error("Failed to scan cache keys for invalidation", zap.Error(err))
		return fmt.Errorf("cache scan error: %w", err)
	}

	if len(keys) == 0 {
		cs.logger.Debug("No cache keys to invalidate", zap.String("pattern", pattern))
		return nil
	}

	// Delete all found keys
	if err := cs.redis.Del(ctx, keys...).Err(); err != nil {
		cs.logger.Error("Failed to delete cache keys", zap.Int("count", len(keys)), zap.Error(err))
		return fmt.Errorf("cache delete error: %w", err)
	}

	cs.logger.Info("Invalidated user timeline cache",
		zap.String("user_id", userID.String()),
		zap.Int("keys_deleted", len(keys)))

	return nil
}

// InvalidateDate invalidates all cached timelines for a specific date
func (cs *CacheService) InvalidateDate(ctx context.Context, userID uuid.UUID, date string) error {
	pattern := fmt.Sprintf("timeline:daily:enhanced:%s:%s:*", userID.String(), date)

	iter := cs.redis.Scan(ctx, 0, pattern, 0).Iterator()
	var keys []string

	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
		keys = append(keys, fmt.Sprintf("%s:etag", iter.Val()))
		keys = append(keys, fmt.Sprintf("%s:last_modified", iter.Val()))
	}

	if err := iter.Err(); err != nil {
		cs.logger.Error("Failed to scan cache keys for date invalidation", zap.Error(err))
		return fmt.Errorf("cache scan error: %w", err)
	}

	if len(keys) == 0 {
		return nil
	}

	if err := cs.redis.Del(ctx, keys...).Err(); err != nil {
		cs.logger.Error("Failed to delete cache keys for date", zap.Int("count", len(keys)), zap.Error(err))
		return fmt.Errorf("cache delete error: %w", err)
	}

	cs.logger.Info("Invalidated date timeline cache",
		zap.String("user_id", userID.String()),
		zap.String("date", date),
		zap.Int("keys_deleted", len(keys)))

	return nil
}

// ComputeETag computes an ETag based on view content
func ComputeETag(view *EnhancedDayView) string {
	// Simple ETag based on generated_at timestamp and content hash
	data, _ := json.Marshal(view)
	hash := sha256.Sum256(data)
	return fmt.Sprintf(`"%x"`, hash[:16]) // First 16 bytes as hex, wrapped in quotes
}

// CalculateTTL calculates appropriate TTL based on the date
// - Current day: shorter TTL (5 minutes) since it's actively changing
// - Past days: longer TTL (1 hour) since they're unlikely to change
// - Future days: medium TTL (15 minutes)
func CalculateTTL(date time.Time) time.Duration {
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	dateStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)

	switch {
	case dateStart.Equal(today):
		// Current day - short TTL
		return 5 * time.Minute
	case dateStart.Before(today):
		// Past day - long TTL
		return 1 * time.Hour
	default:
		// Future day - medium TTL
		return 15 * time.Minute
	}
}
