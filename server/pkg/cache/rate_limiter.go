package cache

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter provides per-key rate limiting
type RateLimiter struct {
	limiters        map[string]*rate.Limiter
	mu              sync.RWMutex
	rate            rate.Limit
	burst           int
	cleanupInterval time.Duration
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(r rate.Limit, burst int) *RateLimiter {
	rl := &RateLimiter{
		limiters:        make(map[string]*rate.Limiter),
		rate:            r,
		burst:           burst,
		cleanupInterval: 5 * time.Minute, // Cleanup unused limiters after 5 minutes
	}

	// Start cleanup goroutine
	go rl.cleanupRoutine()

	return rl
}

// Allow checks if a request is allowed for the given key
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	limiter, exists := rl.limiters[key]
	if !exists {
		limiter = rate.NewLimiter(rl.rate, rl.burst)
		rl.limiters[key] = limiter
	}
	rl.mu.Unlock()

	return limiter.Allow()
}

// Wait waits for permission to proceed
func (rl *RateLimiter) Wait(ctx context.Context, key string) error {
	rl.mu.Lock()
	limiter, exists := rl.limiters[key]
	if !exists {
		limiter = rate.NewLimiter(rl.rate, rl.burst)
		rl.limiters[key] = limiter
	}
	rl.mu.Unlock()

	return limiter.Wait(ctx)
}

// cleanupRoutine periodically removes unused limiters
func (rl *RateLimiter) cleanupRoutine() {
	ticker := time.NewTicker(rl.cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		rl.cleanup()
	}
}

// cleanup removes limiters that haven't been used recently
func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// In a production system, you'd track last access time
	// For now, we'll clear the map if it grows too large
	if len(rl.limiters) > 1000 {
		// Keep only the most recent entries
		rl.limiters = make(map[string]*rate.Limiter)
	}
}

// TagSuggestionRateLimiter provides rate limiting specifically for tag suggestions
type TagSuggestionRateLimiter struct {
	limiter *RateLimiter
}

// NewTagSuggestionRateLimiter creates a rate limiter for tag suggestions
// Default: 10 requests per second with burst of 20
func NewTagSuggestionRateLimiter() *TagSuggestionRateLimiter {
	return &TagSuggestionRateLimiter{
		limiter: NewRateLimiter(rate.Limit(10), 20),
	}
}

// AllowUser checks if a user can make a tag suggestion request
func (trl *TagSuggestionRateLimiter) AllowUser(userID string) bool {
	key := fmt.Sprintf("tag:suggest:%s", userID)
	return trl.limiter.Allow(key)
}

// WaitUser waits for permission for a user to make a tag suggestion request
func (trl *TagSuggestionRateLimiter) WaitUser(ctx context.Context, userID string) error {
	key := fmt.Sprintf("tag:suggest:%s", userID)
	return trl.limiter.Wait(ctx, key)
}

// CategoryRateLimiter provides rate limiting for category operations
type CategoryRateLimiter struct {
	limiter *RateLimiter
}

// NewCategoryRateLimiter creates a rate limiter for category operations
// Default: 5 requests per second with burst of 10
func NewCategoryRateLimiter() *CategoryRateLimiter {
	return &CategoryRateLimiter{
		limiter: NewRateLimiter(rate.Limit(5), 10),
	}
}

// AllowUser checks if a user can make a category operation
func (crl *CategoryRateLimiter) AllowUser(userID string) bool {
	key := fmt.Sprintf("category:%s", userID)
	return crl.limiter.Allow(key)
}

// WaitUser waits for permission for a user to make a category operation
func (crl *CategoryRateLimiter) WaitUser(ctx context.Context, userID string) error {
	key := fmt.Sprintf("category:%s", userID)
	return crl.limiter.Wait(ctx, key)
}