package security

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// LockoutManager manages account lockout for failed login attempts
type LockoutManager struct {
	redisClient      *redis.Client
	maxAttempts      int
	lockoutDuration  time.Duration
	attemptWindow    time.Duration
}

// LockoutConfig holds configuration for account lockout
type LockoutConfig struct {
	RedisClient     *redis.Client
	MaxAttempts     int           // Maximum failed attempts before lockout
	LockoutDuration time.Duration // Duration of lockout
	AttemptWindow   time.Duration // Time window to track attempts
}

// NewLockoutManager creates a new lockout manager
func NewLockoutManager(cfg LockoutConfig) *LockoutManager {
	// Set defaults
	if cfg.MaxAttempts == 0 {
		cfg.MaxAttempts = 5
	}
	if cfg.LockoutDuration == 0 {
		cfg.LockoutDuration = 15 * time.Minute
	}
	if cfg.AttemptWindow == 0 {
		cfg.AttemptWindow = 30 * time.Minute
	}

	return &LockoutManager{
		redisClient:     cfg.RedisClient,
		maxAttempts:     cfg.MaxAttempts,
		lockoutDuration: cfg.LockoutDuration,
		attemptWindow:   cfg.AttemptWindow,
	}
}

// RecordFailedAttempt records a failed login attempt
func (m *LockoutManager) RecordFailedAttempt(ctx context.Context, identifier string) error {
	key := m.getAttemptsKey(identifier)
	lockoutKey := m.getLockoutKey(identifier)

	// Check if already locked out
	isLocked, err := m.redisClient.Exists(ctx, lockoutKey).Result()
	if err != nil {
		return fmt.Errorf("failed to check lockout status: %w", err)
	}
	if isLocked > 0 {
		return nil // Already locked out
	}

	// Increment attempt count
	attempts, err := m.redisClient.Incr(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("failed to increment attempts: %w", err)
	}

	// Set expiry on first attempt
	if attempts == 1 {
		err = m.redisClient.Expire(ctx, key, m.attemptWindow).Err()
		if err != nil {
			return fmt.Errorf("failed to set attempt expiry: %w", err)
		}
	}

	// Lock account if max attempts reached
	if attempts >= int64(m.maxAttempts) {
		err = m.redisClient.Set(ctx, lockoutKey, time.Now().Unix(), m.lockoutDuration).Err()
		if err != nil {
			return fmt.Errorf("failed to set lockout: %w", err)
		}

		// Delete attempts counter
		m.redisClient.Del(ctx, key)
	}

	return nil
}

// RecordSuccessfulAttempt clears failed attempts on successful login
func (m *LockoutManager) RecordSuccessfulAttempt(ctx context.Context, identifier string) error {
	key := m.getAttemptsKey(identifier)
	return m.redisClient.Del(ctx, key).Err()
}

// IsLockedOut checks if an account is locked out
func (m *LockoutManager) IsLockedOut(ctx context.Context, identifier string) (bool, time.Time, error) {
	lockoutKey := m.getLockoutKey(identifier)

	_ , err := m.redisClient.Get(ctx, lockoutKey).Int64()
	if err != nil {
		if err == redis.Nil {
			// Not locked out
			return false, time.Time{}, nil
		}
		return false, time.Time{}, fmt.Errorf("failed to check lockout: %w", err)
	}

	// Get TTL to calculate unlock time
	ttl, err := m.redisClient.TTL(ctx, lockoutKey).Result()
	if err != nil {
		return false, time.Time{}, fmt.Errorf("failed to get lockout TTL: %w", err)
	}

	if ttl <= 0 {
		// Lockout expired
		return false, time.Time{}, nil
	}

	unlockTime := time.Now().Add(ttl)
	return true, unlockTime, nil
}

// GetFailedAttempts gets the number of failed attempts
func (m *LockoutManager) GetFailedAttempts(ctx context.Context, identifier string) (int, error) {
	key := m.getAttemptsKey(identifier)

	attempts, err := m.redisClient.Get(ctx, key).Int()
	if err != nil {
		if err == redis.Nil {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to get attempts: %w", err)
	}

	return attempts, nil
}

// GetRemainingAttempts gets the number of remaining attempts before lockout
func (m *LockoutManager) GetRemainingAttempts(ctx context.Context, identifier string) (int, error) {
	attempts, err := m.GetFailedAttempts(ctx, identifier)
	if err != nil {
		return 0, err
	}

	remaining := m.maxAttempts - attempts
	if remaining < 0 {
		return 0, nil
	}

	return remaining, nil
}

// UnlockAccount manually unlocks an account (admin function)
func (m *LockoutManager) UnlockAccount(ctx context.Context, identifier string) error {
	lockoutKey := m.getLockoutKey(identifier)
	attemptsKey := m.getAttemptsKey(identifier)

	// Delete both lockout and attempts keys
	pipe := m.redisClient.Pipeline()
	pipe.Del(ctx, lockoutKey)
	pipe.Del(ctx, attemptsKey)
	_, err := pipe.Exec(ctx)

	if err != nil {
		return fmt.Errorf("failed to unlock account: %w", err)
	}

	return nil
}

// ResetAttempts resets the failed attempts counter
func (m *LockoutManager) ResetAttempts(ctx context.Context, identifier string) error {
	key := m.getAttemptsKey(identifier)
	return m.redisClient.Del(ctx, key).Err()
}

// getAttemptsKey generates Redis key for attempts counter
func (m *LockoutManager) getAttemptsKey(identifier string) string {
	return fmt.Sprintf("lockout:attempts:%s", identifier)
}

// getLockoutKey generates Redis key for lockout status
func (m *LockoutManager) getLockoutKey(identifier string) string {
	return fmt.Sprintf("lockout:locked:%s", identifier)
}

// GetLockoutInfo returns comprehensive lockout information
type LockoutInfo struct {
	IsLockedOut       bool      `json:"is_locked_out"`
	FailedAttempts    int       `json:"failed_attempts"`
	RemainingAttempts int       `json:"remaining_attempts"`
	UnlockTime        time.Time `json:"unlock_time,omitempty"`
	MaxAttempts       int       `json:"max_attempts"`
}

// GetInfo returns comprehensive lockout information
func (m *LockoutManager) GetInfo(ctx context.Context, identifier string) (*LockoutInfo, error) {
	isLocked, unlockTime, err := m.IsLockedOut(ctx, identifier)
	if err != nil {
		return nil, err
	}

	failedAttempts, err := m.GetFailedAttempts(ctx, identifier)
	if err != nil {
		return nil, err
	}

	remainingAttempts, err := m.GetRemainingAttempts(ctx, identifier)
	if err != nil {
		return nil, err
	}

	return &LockoutInfo{
		IsLockedOut:       isLocked,
		FailedAttempts:    failedAttempts,
		RemainingAttempts: remainingAttempts,
		UnlockTime:        unlockTime,
		MaxAttempts:       m.maxAttempts,
	}, nil
}

// Progressive delay based on failed attempts
func (m *LockoutManager) GetDelayDuration(failedAttempts int) time.Duration {
	// Exponential backoff: 2^attempts seconds, capped at 60 seconds
	delay := time.Duration(1<<uint(failedAttempts)) * time.Second
	maxDelay := 60 * time.Second
	if delay > maxDelay {
		return maxDelay
	}
	return delay
}
