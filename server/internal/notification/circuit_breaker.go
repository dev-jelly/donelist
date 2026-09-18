package notification

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Circuit breaker states
type CircuitState string

const (
	// CircuitStateClosed - normal operation, requests pass through
	CircuitStateClosed CircuitState = "closed"

	// CircuitStateOpen - circuit is open, requests fail immediately
	CircuitStateOpen CircuitState = "open"

	// CircuitStateHalfOpen - testing if the service has recovered
	CircuitStateHalfOpen CircuitState = "half_open"
)

// Circuit breaker Redis keys
const (
	circuitBreakerKeyPrefix = "notification:circuit_breaker"
)

var (
	// ErrCircuitOpen indicates the circuit breaker is open
	ErrCircuitOpen = errors.New("circuit breaker is open")

	// ErrCircuitHalfOpen indicates the circuit breaker is in half-open state
	ErrCircuitHalfOpen = errors.New("circuit breaker is half-open")
)

// CircuitBreakerConfig holds circuit breaker configuration
type CircuitBreakerConfig struct {
	// FailureThreshold is the number of failures before opening the circuit
	FailureThreshold int

	// SuccessThreshold is the number of successes needed in half-open to close
	SuccessThreshold int

	// Timeout is the duration to wait before transitioning from open to half-open
	// Circuit breaker with 5-minute cooldown
	Timeout time.Duration

	// HalfOpenMaxRequests is the max concurrent requests in half-open state
	HalfOpenMaxRequests int

	// ResetTimeout is how long to track failures before resetting counter
	ResetTimeout time.Duration
}

// DefaultCircuitBreakerConfig returns default circuit breaker configuration
// Circuit breaker with 5-minute cooldown
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold:    5,
		SuccessThreshold:    2,
		Timeout:             5 * time.Minute,
		HalfOpenMaxRequests: 3,
		ResetTimeout:        1 * time.Minute,
	}
}

// CircuitBreaker implements the circuit breaker pattern for notification providers
type CircuitBreaker struct {
	redis    *redis.Client
	logger   *zap.Logger
	config   CircuitBreakerConfig
	provider PushNotificationProvider

	mu                  sync.RWMutex
	state               CircuitState
	failures            int
	successes           int
	lastFailureTime     time.Time
	lastStateChange     time.Time
	halfOpenRequests    int
}

// NewCircuitBreaker creates a new circuit breaker for a provider
func NewCircuitBreaker(
	redisClient *redis.Client,
	logger *zap.Logger,
	config CircuitBreakerConfig,
	provider PushNotificationProvider,
) *CircuitBreaker {
	return &CircuitBreaker{
		redis:           redisClient,
		logger:          logger.With(zap.String("provider", string(provider.GetProviderType()))),
		config:          config,
		provider:        provider,
		state:           CircuitStateClosed,
		lastStateChange: time.Now(),
	}
}

// GetRedisKey returns the Redis key for this circuit breaker
func (cb *CircuitBreaker) GetRedisKey() string {
	return fmt.Sprintf("%s:%s", circuitBreakerKeyPrefix, cb.provider.GetProviderType())
}

// GetState returns the current state of the circuit breaker
func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Call executes a function through the circuit breaker
func (cb *CircuitBreaker) Call(ctx context.Context, fn func() error) error {
	// Check if we can proceed
	if err := cb.beforeCall(); err != nil {
		return err
	}

	// Execute the function
	err := fn()

	// Record the result
	cb.afterCall(err)

	return err
}

// beforeCall checks if a call can proceed based on circuit state
func (cb *CircuitBreaker) beforeCall() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	switch cb.state {
	case CircuitStateClosed:
		// Check if we should reset failure count
		if now.Sub(cb.lastFailureTime) > cb.config.ResetTimeout {
			cb.failures = 0
		}
		return nil

	case CircuitStateOpen:
		// Check if timeout has elapsed to transition to half-open
		if now.Sub(cb.lastStateChange) >= cb.config.Timeout {
			cb.logger.Info("Circuit breaker transitioning to half-open",
				zap.String("provider", string(cb.provider.GetProviderType())),
				zap.Duration("elapsed", now.Sub(cb.lastStateChange)),
			)
			cb.state = CircuitStateHalfOpen
			cb.halfOpenRequests = 0
			cb.lastStateChange = now
			return nil
		}
		return ErrCircuitOpen

	case CircuitStateHalfOpen:
		// Check if we can allow more requests in half-open state
		if cb.halfOpenRequests >= cb.config.HalfOpenMaxRequests {
			return ErrCircuitHalfOpen
		}
		cb.halfOpenRequests++
		return nil

	default:
		return fmt.Errorf("unknown circuit state: %s", cb.state)
	}
}

// afterCall records the result of a call and updates circuit state
func (cb *CircuitBreaker) afterCall(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	if err != nil {
		cb.onFailure(now)
	} else {
		cb.onSuccess(now)
	}
}

// onFailure handles a failed call
func (cb *CircuitBreaker) onFailure(now time.Time) {
	cb.failures++
	cb.lastFailureTime = now

	switch cb.state {
	case CircuitStateClosed:
		if cb.failures >= cb.config.FailureThreshold {
			cb.logger.Warn("Circuit breaker opening due to failures",
				zap.String("provider", string(cb.provider.GetProviderType())),
				zap.Int("failures", cb.failures),
				zap.Int("threshold", cb.config.FailureThreshold),
			)
			cb.state = CircuitStateOpen
			cb.lastStateChange = now
			cb.persistState()
		}

	case CircuitStateHalfOpen:
		cb.logger.Warn("Circuit breaker reopening due to failure in half-open state",
			zap.String("provider", string(cb.provider.GetProviderType())),
		)
		cb.state = CircuitStateOpen
		cb.lastStateChange = now
		cb.successes = 0
		cb.halfOpenRequests = 0
		cb.persistState()
	}
}

// onSuccess handles a successful call
func (cb *CircuitBreaker) onSuccess(now time.Time) {
	switch cb.state {
	case CircuitStateClosed:
		cb.failures = 0

	case CircuitStateHalfOpen:
		cb.successes++
		if cb.successes >= cb.config.SuccessThreshold {
			cb.logger.Info("Circuit breaker closing after successful recovery",
				zap.String("provider", string(cb.provider.GetProviderType())),
				zap.Int("successes", cb.successes),
			)
			cb.state = CircuitStateClosed
			cb.lastStateChange = now
			cb.failures = 0
			cb.successes = 0
			cb.halfOpenRequests = 0
			cb.persistState()
		}
	}
}

// persistState persists the circuit breaker state to Redis
func (cb *CircuitBreaker) persistState() {
	ctx := context.Background()
	key := cb.GetRedisKey()

	stateData := map[string]interface{}{
		"state":             string(cb.state),
		"failures":          cb.failures,
		"successes":         cb.successes,
		"last_state_change": cb.lastStateChange.Unix(),
	}

	// Store state with TTL slightly longer than timeout
	ttl := cb.config.Timeout + (1 * time.Minute)
	if err := cb.redis.HSet(ctx, key, stateData).Err(); err != nil {
		cb.logger.Error("Failed to persist circuit breaker state",
			zap.Error(err),
		)
	}
	cb.redis.Expire(ctx, key, ttl)
}

// loadState loads the circuit breaker state from Redis
func (cb *CircuitBreaker) loadState() error {
	ctx := context.Background()
	key := cb.GetRedisKey()

	result, err := cb.redis.HGetAll(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("failed to load circuit breaker state: %w", err)
	}

	if len(result) == 0 {
		return nil // No persisted state
	}

	// Parse state (simplified - production should handle errors)
	if stateStr, ok := result["state"]; ok {
		cb.state = CircuitState(stateStr)
	}

	return nil
}

// Reset manually resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.logger.Info("Manually resetting circuit breaker",
		zap.String("provider", string(cb.provider.GetProviderType())),
		zap.String("previous_state", string(cb.state)),
	)

	cb.state = CircuitStateClosed
	cb.failures = 0
	cb.successes = 0
	cb.halfOpenRequests = 0
	cb.lastStateChange = time.Now()
	cb.persistState()
}

// GetMetrics returns circuit breaker metrics
func (cb *CircuitBreaker) GetMetrics() map[string]interface{} {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return map[string]interface{}{
		"provider":               string(cb.provider.GetProviderType()),
		"state":                  string(cb.state),
		"failures":               cb.failures,
		"successes":              cb.successes,
		"failure_threshold":      cb.config.FailureThreshold,
		"success_threshold":      cb.config.SuccessThreshold,
		"timeout_seconds":        cb.config.Timeout.Seconds(),
		"last_state_change":      cb.lastStateChange.Unix(),
		"time_since_state_change": time.Since(cb.lastStateChange).Seconds(),
	}
}

// IsHealthy returns whether the circuit breaker is in a healthy state
func (cb *CircuitBreaker) IsHealthy() bool {
	state := cb.GetState()
	return state == CircuitStateClosed || state == CircuitStateHalfOpen
}

// CircuitBreakerManager manages multiple circuit breakers for different providers
type CircuitBreakerManager struct {
	redis    *redis.Client
	logger   *zap.Logger
	config   CircuitBreakerConfig
	breakers map[PushProvider]*CircuitBreaker
	mu       sync.RWMutex
}

// NewCircuitBreakerManager creates a new circuit breaker manager
func NewCircuitBreakerManager(
	redisClient *redis.Client,
	logger *zap.Logger,
	config CircuitBreakerConfig,
) *CircuitBreakerManager {
	return &CircuitBreakerManager{
		redis:    redisClient,
		logger:   logger,
		config:   config,
		breakers: make(map[PushProvider]*CircuitBreaker),
	}
}

// GetBreaker gets or creates a circuit breaker for a provider
func (cbm *CircuitBreakerManager) GetBreaker(provider PushNotificationProvider) *CircuitBreaker {
	cbm.mu.Lock()
	defer cbm.mu.Unlock()

	providerType := provider.GetProviderType()
	if breaker, exists := cbm.breakers[providerType]; exists {
		return breaker
	}

	// Create new circuit breaker
	breaker := NewCircuitBreaker(cbm.redis, cbm.logger, cbm.config, provider)
	breaker.loadState() // Try to load persisted state

	cbm.breakers[providerType] = breaker

	cbm.logger.Info("Created circuit breaker for provider",
		zap.String("provider", string(providerType)),
	)

	return breaker
}

// GetAllMetrics returns metrics for all circuit breakers
func (cbm *CircuitBreakerManager) GetAllMetrics() map[string]interface{} {
	cbm.mu.RLock()
	defer cbm.mu.RUnlock()

	metrics := make(map[string]interface{})
	for providerType, breaker := range cbm.breakers {
		metrics[string(providerType)] = breaker.GetMetrics()
	}

	return metrics
}

// ResetAll resets all circuit breakers
func (cbm *CircuitBreakerManager) ResetAll() {
	cbm.mu.RLock()
	defer cbm.mu.RUnlock()

	for _, breaker := range cbm.breakers {
		breaker.Reset()
	}

	cbm.logger.Info("Reset all circuit breakers")
}
