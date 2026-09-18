package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/dev-jelly/donelist/pkg/cache"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// CacheInvalidator handles cache invalidation for various operations
type CacheInvalidator struct {
	cache  *cache.Cache
	logger *zap.Logger
}

// NewCacheInvalidator creates a new cache invalidator
func NewCacheInvalidator(cache *cache.Cache, logger *zap.Logger) *CacheInvalidator {
	return &CacheInvalidator{
		cache:  cache,
		logger: logger,
	}
}

// InvalidateCheckinCache invalidates caches affected by check-in mutations
func (ci *CacheInvalidator) InvalidateCheckinCache(userID uuid.UUID, checkinDate time.Time) error {
	ctx := context.Background()

	// Extract year and month from check-in date
	year := checkinDate.Year()
	month := int(checkinDate.Month())

	// Build patterns to invalidate
	patterns := []string{
		// Calendar cache for the affected month
		fmt.Sprintf("calendar:v2:%s:%d-%02d:*", userID.String(), year, month),
		// Timeline cache for the affected date
		fmt.Sprintf("timeline:%s:%s", userID.String(), checkinDate.Format("2006-01-02")),
		// Statistics cache that might include this date
		fmt.Sprintf("stats:%s:*", userID.String()),
		// Weekly statistics that might include this check-in
		fmt.Sprintf("weekly-stats:%s:*", userID.String()),
	}

	// Also invalidate adjacent months if near month boundaries
	if checkinDate.Day() <= 7 {
		// Near beginning of month, invalidate previous month
		prevMonth := checkinDate.AddDate(0, -1, 0)
		patterns = append(patterns,
			fmt.Sprintf("calendar:v2:%s:%d-%02d:*", userID.String(), prevMonth.Year(), int(prevMonth.Month())))
	}

	if checkinDate.Day() >= 24 {
		// Near end of month, invalidate next month
		nextMonth := checkinDate.AddDate(0, 1, 0)
		patterns = append(patterns,
			fmt.Sprintf("calendar:v2:%s:%d-%02d:*", userID.String(), nextMonth.Year(), int(nextMonth.Month())))
	}

	// Invalidate all patterns
	for _, pattern := range patterns {
		ci.logger.Debug("Invalidating cache pattern",
			zap.String("pattern", pattern),
			zap.String("user_id", userID.String()))

		if err := ci.cache.DeletePattern(ctx, pattern); err != nil {
			ci.logger.Error("Failed to invalidate cache pattern",
				zap.Error(err),
				zap.String("pattern", pattern))
			// Continue with other patterns even if one fails
		}
	}

	return nil
}

// InvalidateCalendarCache specifically invalidates calendar caches
func (ci *CacheInvalidator) InvalidateCalendarCache(userID uuid.UUID, year, month int) error {
	ctx := context.Background()
	pattern := fmt.Sprintf("calendar:v2:%s:%d-%02d:*", userID.String(), year, month)

	ci.logger.Debug("Invalidating calendar cache",
		zap.String("pattern", pattern),
		zap.String("user_id", userID.String()),
		zap.Int("year", year),
		zap.Int("month", month))

	if err := ci.cache.DeletePattern(ctx, pattern); err != nil {
		ci.logger.Error("Failed to invalidate calendar cache",
			zap.Error(err),
			zap.String("pattern", pattern))
		return err
	}

	return nil
}

// InvalidateAllUserCaches invalidates all caches for a user
func (ci *CacheInvalidator) InvalidateAllUserCaches(userID uuid.UUID) error {
	ctx := context.Background()
	patterns := []string{
		fmt.Sprintf("calendar:v2:%s:*", userID.String()),
		fmt.Sprintf("timeline:%s:*", userID.String()),
		fmt.Sprintf("stats:%s:*", userID.String()),
		fmt.Sprintf("weekly-stats:%s:*", userID.String()),
		fmt.Sprintf("checkins:%s:*", userID.String()),
		fmt.Sprintf("categories:%s", userID.String()),
		fmt.Sprintf("tags:%s", userID.String()),
	}

	ci.logger.Info("Invalidating all caches for user",
		zap.String("user_id", userID.String()))

	for _, pattern := range patterns {
		if err := ci.cache.DeletePattern(ctx, pattern); err != nil {
			ci.logger.Error("Failed to invalidate cache pattern",
				zap.Error(err),
				zap.String("pattern", pattern))
			// Continue with other patterns
		}
	}

	return nil
}