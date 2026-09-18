package checkin

import (
	"context"

	"github.com/google/uuid"
)

// CacheInvalidator defines the interface for invalidating timeline caches
// This allows the checkin service to notify the timeline service when data changes
type CacheInvalidator interface {
	// InvalidateCache invalidates all cached timelines for a user
	InvalidateCache(ctx context.Context, userID uuid.UUID) error

	// InvalidateDateCache invalidates cached timelines for a specific date
	InvalidateDateCache(ctx context.Context, userID uuid.UUID, date string) error
}

// NoOpCacheInvalidator is a no-op implementation for when caching is disabled
type NoOpCacheInvalidator struct{}

func (n *NoOpCacheInvalidator) InvalidateCache(ctx context.Context, userID uuid.UUID) error {
	return nil
}

func (n *NoOpCacheInvalidator) InvalidateDateCache(ctx context.Context, userID uuid.UUID, date string) error {
	return nil
}
