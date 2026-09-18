package checkin

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// CalendarCacheInvalidator defines the interface for invalidating calendar caches
type CalendarCacheInvalidator interface {
	// InvalidateUserCalendarCache invalidates all calendar cache entries for a user
	InvalidateUserCalendarCache(userID uuid.UUID) error

	// InvalidateMonthCalendarCache invalidates calendar cache for a specific month
	InvalidateMonthCalendarCache(userID uuid.UUID, year, month int) error
}

// CompositeCacheInvalidator combines timeline and calendar cache invalidation
type CompositeCacheInvalidator struct {
	timelineInvalidator CacheInvalidator
	calendarInvalidator CalendarCacheInvalidator
	logger             *zap.Logger
}

// NewCompositeCacheInvalidator creates a new composite cache invalidator
func NewCompositeCacheInvalidator(
	timelineInvalidator CacheInvalidator,
	calendarInvalidator CalendarCacheInvalidator,
	logger *zap.Logger,
) *CompositeCacheInvalidator {
	return &CompositeCacheInvalidator{
		timelineInvalidator: timelineInvalidator,
		calendarInvalidator: calendarInvalidator,
		logger:             logger,
	}
}

// InvalidateCache invalidates all cached data for a user (both timeline and calendar)
func (c *CompositeCacheInvalidator) InvalidateCache(ctx context.Context, userID uuid.UUID) error {
	// Invalidate timeline cache
	if c.timelineInvalidator != nil {
		if err := c.timelineInvalidator.InvalidateCache(ctx, userID); err != nil {
			c.logger.Error("Failed to invalidate timeline cache",
				zap.Error(err),
				zap.String("user_id", userID.String()))
			// Continue with calendar invalidation even if timeline fails
		}
	}

	// Invalidate calendar cache
	if c.calendarInvalidator != nil {
		if err := c.calendarInvalidator.InvalidateUserCalendarCache(userID); err != nil {
			c.logger.Error("Failed to invalidate calendar cache",
				zap.Error(err),
				zap.String("user_id", userID.String()))
			// Return error but timeline may have been invalidated
			return err
		}
	}

	return nil
}

// InvalidateDateCache invalidates cached data for a specific date
func (c *CompositeCacheInvalidator) InvalidateDateCache(ctx context.Context, userID uuid.UUID, date string) error {
	// Invalidate timeline cache for the date
	if c.timelineInvalidator != nil {
		if err := c.timelineInvalidator.InvalidateDateCache(ctx, userID, date); err != nil {
			c.logger.Error("Failed to invalidate timeline cache for date",
				zap.Error(err),
				zap.String("user_id", userID.String()),
				zap.String("date", date))
			// Continue with calendar invalidation
		}
	}

	// Parse date to get year and month for calendar invalidation
	if c.calendarInvalidator != nil {
		parsedDate, err := time.Parse("2006-01-02", date)
		if err != nil {
			c.logger.Warn("Failed to parse date for calendar invalidation",
				zap.Error(err),
				zap.String("date", date))
		} else {
			year := parsedDate.Year()
			month := int(parsedDate.Month())

			// Invalidate calendar cache for the affected month
			if err := c.calendarInvalidator.InvalidateMonthCalendarCache(userID, year, month); err != nil {
				c.logger.Error("Failed to invalidate calendar cache for month",
					zap.Error(err),
					zap.String("user_id", userID.String()),
					zap.Int("year", year),
					zap.Int("month", month))
				return err
			}

			// Also invalidate adjacent months if near month boundaries
			if parsedDate.Day() <= 7 {
				// Near beginning of month, invalidate previous month
				prevMonth := parsedDate.AddDate(0, -1, 0)
				_ = c.calendarInvalidator.InvalidateMonthCalendarCache(
					userID, prevMonth.Year(), int(prevMonth.Month()))
			}

			if parsedDate.Day() >= 24 {
				// Near end of month, invalidate next month
				nextMonth := parsedDate.AddDate(0, 1, 0)
				_ = c.calendarInvalidator.InvalidateMonthCalendarCache(
					userID, nextMonth.Year(), int(nextMonth.Month()))
			}
		}
	}

	return nil
}