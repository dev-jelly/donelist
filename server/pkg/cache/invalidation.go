package cache

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

// Invalidator handles cache invalidation strategies
type Invalidator struct {
	cache  *Cache
	logger *zap.Logger
}

// NewInvalidator creates a new cache invalidator
func NewInvalidator(cache *Cache, logger *zap.Logger) *Invalidator {
	return &Invalidator{
		cache:  cache,
		logger: logger,
	}
}

// InvalidateUser invalidates all cache entries for a specific user
func (i *Invalidator) InvalidateUser(ctx context.Context, userID int64) error {
	patterns := []string{
		fmt.Sprintf("user:%d*", userID),
		fmt.Sprintf("checkins:%d:*", userID),
		fmt.Sprintf("categories:%d*", userID),
		fmt.Sprintf("tags:%d*", userID),
		fmt.Sprintf("stats:%d:*", userID),
		fmt.Sprintf("search:%d:*", userID),
		fmt.Sprintf("calendar:%d:*", userID),
		fmt.Sprintf("timeline:%d:*", userID),
	}

	for _, pattern := range patterns {
		if err := i.cache.DeletePattern(ctx, pattern); err != nil {
			i.logger.Error("Failed to invalidate user cache",
				zap.Int64("user_id", userID),
				zap.String("pattern", pattern),
				zap.Error(err))
			return fmt.Errorf("invalidate user cache: %w", err)
		}
	}

	i.logger.Info("Invalidated user cache", zap.Int64("user_id", userID))
	return nil
}

// InvalidateCheckins invalidates checkin-related cache for a user
func (i *Invalidator) InvalidateCheckins(ctx context.Context, userID int64, date string) error {
	keys := []string{
		MakeCheckinsKey(userID, date),
		fmt.Sprintf("checkins:list:%d:*", userID), // Pattern for paginated lists
		MakeTimelineKey(userID, date),
	}

	// Also invalidate statistics as they depend on checkins
	if err := i.cache.DeletePattern(ctx, fmt.Sprintf("stats:%d:*", userID)); err != nil {
		i.logger.Warn("Failed to invalidate stats cache",
			zap.Int64("user_id", userID),
			zap.Error(err))
	}

	for _, key := range keys {
		if err := i.cache.DeletePattern(ctx, key); err != nil {
			i.logger.Error("Failed to invalidate checkin cache",
				zap.Int64("user_id", userID),
				zap.String("key", key),
				zap.Error(err))
			return fmt.Errorf("invalidate checkin cache: %w", err)
		}
	}

	i.logger.Debug("Invalidated checkin cache",
		zap.Int64("user_id", userID),
		zap.String("date", date))
	return nil
}

// InvalidateCategories invalidates category-related cache for a user
func (i *Invalidator) InvalidateCategories(ctx context.Context, userID int64) error {
	patterns := []string{
		MakeCategoriesKey(userID),
		// When categories change, timeline might show different category names
		fmt.Sprintf("timeline:%d:*", userID),
		// Statistics might be grouped by category
		fmt.Sprintf("stats:%d:*", userID),
		// UUID-based patterns for new category cache
		fmt.Sprintf("categories:user:*"),
		fmt.Sprintf("category:*"),
		fmt.Sprintf("categories:colors:recommend:*"),
	}

	for _, pattern := range patterns {
		if err := i.cache.DeletePattern(ctx, pattern); err != nil {
			i.logger.Error("Failed to invalidate category cache",
				zap.Int64("user_id", userID),
				zap.String("pattern", pattern),
				zap.Error(err))
			return fmt.Errorf("invalidate category cache: %w", err)
		}
	}

	i.logger.Debug("Invalidated category cache", zap.Int64("user_id", userID))
	return nil
}

// InvalidateTags invalidates tag-related cache for a user
func (i *Invalidator) InvalidateTags(ctx context.Context, userID int64) error {
	patterns := []string{
		MakeTagsKey(userID),
		// Tags appear in timeline and search results
		fmt.Sprintf("timeline:%d:*", userID),
		fmt.Sprintf("search:%d:*", userID),
		// UUID-based patterns for new tag cache
		fmt.Sprintf("tags:user:*"),
		fmt.Sprintf("tag:*"),
		fmt.Sprintf("tags:suggest:*"),
		fmt.Sprintf("tags:popular:*"),
	}

	for _, pattern := range patterns {
		if err := i.cache.DeletePattern(ctx, pattern); err != nil {
			i.logger.Error("Failed to invalidate tag cache",
				zap.Int64("user_id", userID),
				zap.String("pattern", pattern),
				zap.Error(err))
			return fmt.Errorf("invalidate tag cache: %w", err)
		}
	}

	i.logger.Debug("Invalidated tag cache", zap.Int64("user_id", userID))
	return nil
}

// InvalidateStatistics invalidates statistics cache for a user
func (i *Invalidator) InvalidateStatistics(ctx context.Context, userID int64, startDate, endDate string) error {
	// If specific date range provided, invalidate that specific key
	if startDate != "" && endDate != "" {
		key := MakeStatisticsKey(userID, startDate, endDate)
		if err := i.cache.Delete(ctx, key); err != nil {
			i.logger.Error("Failed to invalidate statistics cache",
				zap.Int64("user_id", userID),
				zap.String("start_date", startDate),
				zap.String("end_date", endDate),
				zap.Error(err))
			return fmt.Errorf("invalidate statistics cache: %w", err)
		}
	} else {
		// Invalidate all statistics for user
		pattern := fmt.Sprintf("stats:%d:*", userID)
		if err := i.cache.DeletePattern(ctx, pattern); err != nil {
			i.logger.Error("Failed to invalidate all statistics cache",
				zap.Int64("user_id", userID),
				zap.Error(err))
			return fmt.Errorf("invalidate all statistics cache: %w", err)
		}
	}

	i.logger.Debug("Invalidated statistics cache",
		zap.Int64("user_id", userID),
		zap.String("start_date", startDate),
		zap.String("end_date", endDate))
	return nil
}

// InvalidateSearch invalidates search cache for a user
func (i *Invalidator) InvalidateSearch(ctx context.Context, userID int64) error {
	pattern := fmt.Sprintf("search:%d:*", userID)
	if err := i.cache.DeletePattern(ctx, pattern); err != nil {
		i.logger.Error("Failed to invalidate search cache",
			zap.Int64("user_id", userID),
			zap.Error(err))
		return fmt.Errorf("invalidate search cache: %w", err)
	}

	i.logger.Debug("Invalidated search cache", zap.Int64("user_id", userID))
	return nil
}

// InvalidateCalendar invalidates calendar cache for a user
func (i *Invalidator) InvalidateCalendar(ctx context.Context, userID int64, year, month int) error {
	// If specific year/month provided, invalidate that specific key
	if year > 0 && month > 0 {
		key := MakeCalendarKey(userID, year, month)
		if err := i.cache.Delete(ctx, key); err != nil {
			i.logger.Error("Failed to invalidate calendar cache",
				zap.Int64("user_id", userID),
				zap.Int("year", year),
				zap.Int("month", month),
				zap.Error(err))
			return fmt.Errorf("invalidate calendar cache: %w", err)
		}
	} else {
		// Invalidate all calendar data for user
		pattern := fmt.Sprintf("calendar:%d:*", userID)
		if err := i.cache.DeletePattern(ctx, pattern); err != nil {
			i.logger.Error("Failed to invalidate all calendar cache",
				zap.Int64("user_id", userID),
				zap.Error(err))
			return fmt.Errorf("invalidate all calendar cache: %w", err)
		}
	}

	i.logger.Debug("Invalidated calendar cache",
		zap.Int64("user_id", userID),
		zap.Int("year", year),
		zap.Int("month", month))
	return nil
}

// InvalidateTimeline invalidates timeline cache for a user
func (i *Invalidator) InvalidateTimeline(ctx context.Context, userID int64, date string) error {
	// If specific date provided, invalidate that specific key
	if date != "" {
		key := MakeTimelineKey(userID, date)
		if err := i.cache.Delete(ctx, key); err != nil {
			i.logger.Error("Failed to invalidate timeline cache",
				zap.Int64("user_id", userID),
				zap.String("date", date),
				zap.Error(err))
			return fmt.Errorf("invalidate timeline cache: %w", err)
		}
	} else {
		// Invalidate all timeline data for user
		pattern := fmt.Sprintf("timeline:%d:*", userID)
		if err := i.cache.DeletePattern(ctx, pattern); err != nil {
			i.logger.Error("Failed to invalidate all timeline cache",
				zap.Int64("user_id", userID),
				zap.Error(err))
			return fmt.Errorf("invalidate all timeline cache: %w", err)
		}
	}

	i.logger.Debug("Invalidated timeline cache",
		zap.Int64("user_id", userID),
		zap.String("date", date))
	return nil
}

// InvalidateOnCheckinCreate invalidates all affected caches when a new checkin is created
func (i *Invalidator) InvalidateOnCheckinCreate(ctx context.Context, userID int64, date string) error {
	// Checkin creation affects:
	// 1. Checkin lists for that date
	// 2. Timeline for that date
	// 3. Statistics (all date ranges)
	// 4. Calendar view
	// 5. Search results

	patterns := []string{
		fmt.Sprintf("checkins:%d:*", userID),
		MakeTimelineKey(userID, date),
		fmt.Sprintf("stats:%d:*", userID),
		fmt.Sprintf("calendar:%d:*", userID),
		fmt.Sprintf("search:%d:*", userID),
	}

	for _, pattern := range patterns {
		if err := i.cache.DeletePattern(ctx, pattern); err != nil {
			i.logger.Warn("Failed to invalidate cache on checkin create",
				zap.Int64("user_id", userID),
				zap.String("pattern", pattern),
				zap.Error(err))
		}
	}

	i.logger.Debug("Invalidated caches on checkin create",
		zap.Int64("user_id", userID),
		zap.String("date", date))
	return nil
}

// InvalidateOnCheckinUpdate invalidates caches when a checkin is updated
func (i *Invalidator) InvalidateOnCheckinUpdate(ctx context.Context, userID int64, oldDate, newDate string) error {
	// Checkin update affects both old and new dates if date changed
	dates := []string{oldDate}
	if newDate != "" && newDate != oldDate {
		dates = append(dates, newDate)
	}

	for _, date := range dates {
		if err := i.InvalidateOnCheckinCreate(ctx, userID, date); err != nil {
			i.logger.Warn("Failed to invalidate cache on checkin update",
				zap.Int64("user_id", userID),
				zap.String("date", date),
				zap.Error(err))
		}
	}

	return nil
}

// InvalidateOnCheckinDelete invalidates caches when a checkin is deleted
func (i *Invalidator) InvalidateOnCheckinDelete(ctx context.Context, userID int64, date string) error {
	// Same invalidation as create
	return i.InvalidateOnCheckinCreate(ctx, userID, date)
}
