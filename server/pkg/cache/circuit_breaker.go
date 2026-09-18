package cache

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/sony/gobreaker"
	"go.uber.org/zap"
)

// CircuitBreakerConfig holds configuration for the circuit breaker
type CircuitBreakerConfig struct {
	// Name identifies the circuit breaker
	Name string

	// MaxRequests is the maximum number of requests allowed to pass through
	// when the circuit breaker is half-open
	MaxRequests uint32

	// Interval is the cyclic period of the closed state for the circuit breaker
	// to clear the internal counts. If Interval is 0, the circuit breaker doesn't
	// clear internal counts during the closed state
	Interval time.Duration

	// Timeout is the period of the open state, after which the state of the
	// circuit breaker becomes half-open. If Timeout is 0, the timeout value
	// is set to 60 seconds
	Timeout time.Duration

	// ReadyToTrip is called with a copy of Counts whenever a request fails in
	// the closed state. If ReadyToTrip returns true, the circuit breaker will
	// be placed into the open state. If ReadyToTrip is nil, default behavior
	// is used: the circuit breaker trips when the number of consecutive failures
	// is more than 5
	FailureThreshold uint32

	// OnStateChange is called whenever the state of the circuit breaker changes
	OnStateChange func(name string, from gobreaker.State, to gobreaker.State)
}

// DefaultCircuitBreakerConfig returns a default circuit breaker configuration
func DefaultCircuitBreakerConfig(name string) CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Name:             name,
		MaxRequests:      3,
		Interval:         10 * time.Second,
		Timeout:          30 * time.Second,
		FailureThreshold: 5,
	}
}

// CircuitBreakerCache wraps a cache with circuit breaker protection
type CircuitBreakerCache struct {
	cache  *Cache
	cb     *gobreaker.CircuitBreaker
	logger *zap.Logger
	mu     sync.RWMutex
	state  gobreaker.State
}

// NewCircuitBreakerCache creates a new cache with circuit breaker protection
func NewCircuitBreakerCache(cache *Cache, config CircuitBreakerConfig, logger *zap.Logger) *CircuitBreakerCache {
	cbc := &CircuitBreakerCache{
		cache:  cache,
		logger: logger,
	}

	settings := gobreaker.Settings{
		Name:        config.Name,
		MaxRequests: config.MaxRequests,
		Interval:    config.Interval,
		Timeout:     config.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= config.FailureThreshold && failureRatio >= 0.6
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			cbc.mu.Lock()
			cbc.state = to
			cbc.mu.Unlock()

			logger.Info("Circuit breaker state changed",
				zap.String("name", name),
				zap.String("from", from.String()),
				zap.String("to", to.String()))

			if config.OnStateChange != nil {
				config.OnStateChange(name, from, to)
			}
		},
	}

	cbc.cb = gobreaker.NewCircuitBreaker(settings)
	return cbc
}

// Get retrieves a value from cache with circuit breaker protection
func (cbc *CircuitBreakerCache) Get(ctx context.Context, key string, dest interface{}) error {
	_, err := cbc.cb.Execute(func() (interface{}, error) {
		return nil, cbc.cache.Get(ctx, key, dest)
	})

	if err != nil {
		// Check if it's a circuit breaker error
		if err == gobreaker.ErrOpenState || err == gobreaker.ErrTooManyRequests {
			cbc.logger.Warn("Circuit breaker prevented cache get",
				zap.String("key", key),
				zap.Error(err))
			// Return cache miss to allow fallback to source
			return ErrCacheMiss
		}
		return err
	}

	return nil
}

// Set stores a value in cache with circuit breaker protection
func (cbc *CircuitBreakerCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	_, err := cbc.cb.Execute(func() (interface{}, error) {
		return nil, cbc.cache.Set(ctx, key, value, ttl)
	})

	if err != nil {
		// Log but don't fail the operation if circuit is open
		if err == gobreaker.ErrOpenState || err == gobreaker.ErrTooManyRequests {
			cbc.logger.Warn("Circuit breaker prevented cache set",
				zap.String("key", key),
				zap.Error(err))
			// Don't return error for set operations to prevent cascading failures
			return nil
		}
		return err
	}

	return nil
}

// Delete removes a value from cache with circuit breaker protection
func (cbc *CircuitBreakerCache) Delete(ctx context.Context, keys ...string) error {
	_, err := cbc.cb.Execute(func() (interface{}, error) {
		return nil, cbc.cache.Delete(ctx, keys...)
	})

	if err != nil {
		// Log but don't fail the operation if circuit is open
		if err == gobreaker.ErrOpenState || err == gobreaker.ErrTooManyRequests {
			cbc.logger.Warn("Circuit breaker prevented cache delete",
				zap.Strings("keys", keys),
				zap.Error(err))
			// Don't return error for delete operations
			return nil
		}
		return err
	}

	return nil
}

// DeletePattern deletes all keys matching a pattern with circuit breaker protection
func (cbc *CircuitBreakerCache) DeletePattern(ctx context.Context, pattern string) error {
	_, err := cbc.cb.Execute(func() (interface{}, error) {
		return nil, cbc.cache.DeletePattern(ctx, pattern)
	})

	if err != nil {
		// Log but don't fail the operation if circuit is open
		if err == gobreaker.ErrOpenState || err == gobreaker.ErrTooManyRequests {
			cbc.logger.Warn("Circuit breaker prevented cache delete pattern",
				zap.String("pattern", pattern),
				zap.Error(err))
			// Don't return error for delete operations
			return nil
		}
		return err
	}

	return nil
}

// GetOrSet retrieves from cache or sets it using the provided function with circuit breaker protection
func (cbc *CircuitBreakerCache) GetOrSet(ctx context.Context, key string, ttl time.Duration, dest interface{}, fetchFunc func() (interface{}, error)) error {
	// Try to get from cache first
	err := cbc.Get(ctx, key, dest)
	if err == nil {
		return nil // Cache hit
	}

	if err != ErrCacheMiss {
		// Log cache error but continue to fetch from source
		cbc.logger.Warn("Cache get error in GetOrSet, fetching from source",
			zap.String("key", key),
			zap.Error(err))
	}

	// Cache miss or error, fetch from source
	value, err := fetchFunc()
	if err != nil {
		return err
	}

	// Try to store in cache, but don't fail if cache is down
	if setErr := cbc.Set(ctx, key, value, ttl); setErr != nil {
		cbc.logger.Warn("Failed to set cache after fetch",
			zap.String("key", key),
			zap.Error(setErr))
	}

	// Copy the fetched value to dest
	if err := copyValue(value, dest); err != nil {
		return err
	}

	return nil
}

// GetState returns the current state of the circuit breaker
func (cbc *CircuitBreakerCache) GetState() gobreaker.State {
	cbc.mu.RLock()
	defer cbc.mu.RUnlock()
	return cbc.state
}

// IsHealthy returns true if the circuit breaker is in closed state (healthy)
func (cbc *CircuitBreakerCache) IsHealthy() bool {
	return cbc.GetState() == gobreaker.StateClosed
}

// GetMetrics returns the underlying cache metrics
func (cbc *CircuitBreakerCache) GetMetrics() Metrics {
	return cbc.cache.GetMetrics()
}

// copyValue is a helper function to copy values between interfaces
func copyValue(src, dest interface{}) error {
	// This is a simplified version. In production, you might want to use
	// a more sophisticated method or a library like mapstructure
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}