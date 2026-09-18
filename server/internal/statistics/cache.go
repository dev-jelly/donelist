package statistics

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// CacheService handles Redis caching for statistics
type CacheService struct {
	redis  *redis.Client
	logger *zap.Logger
	prefix string
	ttl    time.Duration
}

// NewCacheService creates a new statistics cache service
func NewCacheService(redisClient *redis.Client, logger *zap.Logger) *CacheService {
	return &CacheService{
		redis:  redisClient,
		logger: logger,
		prefix: "stats:weekly:",
		ttl:    15 * time.Minute, // 15 minutes TTL as per requirements
	}
}

// GetWeeklyStats retrieves cached weekly statistics
func (s *CacheService) GetWeeklyStats(ctx context.Context, key string) (*WeeklyStatistics, error) {
	cacheKey := s.prefix + key
	
	data, err := s.redis.Get(ctx, cacheKey).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // Cache miss
		}
		s.logger.Warn("Failed to get weekly stats from cache",
			zap.String("key", cacheKey),
			zap.Error(err),
		)
		return nil, err
	}

	var stats WeeklyStatistics
	if err := json.Unmarshal(data, &stats); err != nil {
		s.logger.Warn("Failed to unmarshal cached weekly stats",
			zap.String("key", cacheKey),
			zap.Error(err),
		)
		return nil, err
	}

	s.logger.Debug("Weekly stats cache hit", zap.String("key", cacheKey))
	return &stats, nil
}

// SetWeeklyStats caches weekly statistics
func (s *CacheService) SetWeeklyStats(ctx context.Context, key string, stats *WeeklyStatistics) error {
	cacheKey := s.prefix + key
	
	data, err := json.Marshal(stats)
	if err != nil {
		s.logger.Warn("Failed to marshal weekly stats for caching",
			zap.String("key", cacheKey),
			zap.Error(err),
		)
		return err
	}

	// Use custom TTL if cache expiration is set and is sooner than default TTL
	ttl := s.ttl
	if !stats.CacheExpiration.IsZero() {
		customTTL := time.Until(stats.CacheExpiration)
		if customTTL > 0 && customTTL < ttl {
			ttl = customTTL
		}
	}

	err = s.redis.Set(ctx, cacheKey, data, ttl).Err()
	if err != nil {
		s.logger.Warn("Failed to cache weekly stats",
			zap.String("key", cacheKey),
			zap.Error(err),
		)
		return err
	}

	s.logger.Debug("Weekly stats cached",
		zap.String("key", cacheKey),
		zap.Duration("ttl", ttl),
	)
	return nil
}

// InvalidateUserStats invalidates all cached statistics for a user
func (s *CacheService) InvalidateUserStats(ctx context.Context, userID string) error {
	pattern := fmt.Sprintf("%s%s:*", s.prefix, userID)
	
	// Use SCAN to find all matching keys
	iter := s.redis.Scan(ctx, 0, pattern, 0).Iterator()
	var keys []string
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		s.logger.Warn("Failed to scan for user stats keys",
			zap.String("user_id", userID),
			zap.Error(err),
		)
		return err
	}

	if len(keys) == 0 {
		return nil // Nothing to delete
	}

	// Delete all found keys
	if err := s.redis.Del(ctx, keys...).Err(); err != nil {
		s.logger.Warn("Failed to invalidate user stats",
			zap.String("user_id", userID),
			zap.Int("key_count", len(keys)),
			zap.Error(err),
		)
		return err
	}

	s.logger.Info("User stats cache invalidated",
		zap.String("user_id", userID),
		zap.Int("key_count", len(keys)),
	)
	return nil
}

// GenerateCacheKey creates a cache key for weekly statistics
func GenerateCacheKey(userID, startDate string, weekStartDay WeekStartDay, timezone string) string {
	return fmt.Sprintf("%s:%s:%s:%s", userID, startDate, weekStartDay, timezone)
}
