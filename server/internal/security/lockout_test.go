package security

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	return client, mr
}

func TestLockoutManager_RecordFailedAttempt(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()

	manager := NewLockoutManager(LockoutConfig{
		RedisClient:     client,
		MaxAttempts:     3,
		LockoutDuration: 5 * time.Minute,
		AttemptWindow:   10 * time.Minute,
	})

	ctx := context.Background()
	identifier := "test@example.com"

	// Record first failed attempt
	err := manager.RecordFailedAttempt(ctx, identifier)
	require.NoError(t, err)

	attempts, err := manager.GetFailedAttempts(ctx, identifier)
	require.NoError(t, err)
	assert.Equal(t, 1, attempts)

	// Record second failed attempt
	err = manager.RecordFailedAttempt(ctx, identifier)
	require.NoError(t, err)

	attempts, err = manager.GetFailedAttempts(ctx, identifier)
	require.NoError(t, err)
	assert.Equal(t, 2, attempts)

	// Not locked yet
	isLocked, _, err := manager.IsLockedOut(ctx, identifier)
	require.NoError(t, err)
	assert.False(t, isLocked)
}

func TestLockoutManager_AccountLockout(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()

	manager := NewLockoutManager(LockoutConfig{
		RedisClient:     client,
		MaxAttempts:     3,
		LockoutDuration: 5 * time.Minute,
		AttemptWindow:   10 * time.Minute,
	})

	ctx := context.Background()
	identifier := "test@example.com"

	// Record attempts up to max
	for i := 0; i < 3; i++ {
		err := manager.RecordFailedAttempt(ctx, identifier)
		require.NoError(t, err)
	}

	// Should be locked out now
	isLocked, unlockTime, err := manager.IsLockedOut(ctx, identifier)
	require.NoError(t, err)
	assert.True(t, isLocked)
	assert.True(t, unlockTime.After(time.Now()))
}

func TestLockoutManager_RecordSuccessfulAttempt(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()

	manager := NewLockoutManager(LockoutConfig{
		RedisClient:     client,
		MaxAttempts:     3,
		LockoutDuration: 5 * time.Minute,
		AttemptWindow:   10 * time.Minute,
	})

	ctx := context.Background()
	identifier := "test@example.com"

	// Record some failed attempts
	err := manager.RecordFailedAttempt(ctx, identifier)
	require.NoError(t, err)
	err = manager.RecordFailedAttempt(ctx, identifier)
	require.NoError(t, err)

	attempts, err := manager.GetFailedAttempts(ctx, identifier)
	require.NoError(t, err)
	assert.Equal(t, 2, attempts)

	// Successful login should clear attempts
	err = manager.RecordSuccessfulAttempt(ctx, identifier)
	require.NoError(t, err)

	attempts, err = manager.GetFailedAttempts(ctx, identifier)
	require.NoError(t, err)
	assert.Equal(t, 0, attempts)
}

func TestLockoutManager_GetRemainingAttempts(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()

	manager := NewLockoutManager(LockoutConfig{
		RedisClient:     client,
		MaxAttempts:     5,
		LockoutDuration: 5 * time.Minute,
		AttemptWindow:   10 * time.Minute,
	})

	ctx := context.Background()
	identifier := "test@example.com"

	// Initially should have all attempts remaining
	remaining, err := manager.GetRemainingAttempts(ctx, identifier)
	require.NoError(t, err)
	assert.Equal(t, 5, remaining)

	// After one failed attempt
	err = manager.RecordFailedAttempt(ctx, identifier)
	require.NoError(t, err)

	remaining, err = manager.GetRemainingAttempts(ctx, identifier)
	require.NoError(t, err)
	assert.Equal(t, 4, remaining)

	// After two more failed attempts
	err = manager.RecordFailedAttempt(ctx, identifier)
	require.NoError(t, err)
	err = manager.RecordFailedAttempt(ctx, identifier)
	require.NoError(t, err)

	remaining, err = manager.GetRemainingAttempts(ctx, identifier)
	require.NoError(t, err)
	assert.Equal(t, 2, remaining)
}

func TestLockoutManager_UnlockAccount(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()

	manager := NewLockoutManager(LockoutConfig{
		RedisClient:     client,
		MaxAttempts:     2,
		LockoutDuration: 5 * time.Minute,
		AttemptWindow:   10 * time.Minute,
	})

	ctx := context.Background()
	identifier := "test@example.com"

	// Lock the account
	for i := 0; i < 2; i++ {
		err := manager.RecordFailedAttempt(ctx, identifier)
		require.NoError(t, err)
	}

	// Verify locked
	isLocked, _, err := manager.IsLockedOut(ctx, identifier)
	require.NoError(t, err)
	assert.True(t, isLocked)

	// Unlock the account
	err = manager.UnlockAccount(ctx, identifier)
	require.NoError(t, err)

	// Verify unlocked
	isLocked, _, err = manager.IsLockedOut(ctx, identifier)
	require.NoError(t, err)
	assert.False(t, isLocked)

	// Attempts should also be reset
	attempts, err := manager.GetFailedAttempts(ctx, identifier)
	require.NoError(t, err)
	assert.Equal(t, 0, attempts)
}

func TestLockoutManager_GetInfo(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()

	manager := NewLockoutManager(LockoutConfig{
		RedisClient:     client,
		MaxAttempts:     5,
		LockoutDuration: 5 * time.Minute,
		AttemptWindow:   10 * time.Minute,
	})

	ctx := context.Background()
	identifier := "test@example.com"

	// Record some failed attempts
	err := manager.RecordFailedAttempt(ctx, identifier)
	require.NoError(t, err)
	err = manager.RecordFailedAttempt(ctx, identifier)
	require.NoError(t, err)

	// Get info
	info, err := manager.GetInfo(ctx, identifier)
	require.NoError(t, err)

	assert.False(t, info.IsLockedOut)
	assert.Equal(t, 2, info.FailedAttempts)
	assert.Equal(t, 3, info.RemainingAttempts)
	assert.Equal(t, 5, info.MaxAttempts)
}

func TestLockoutManager_GetDelayDuration(t *testing.T) {
	manager := NewLockoutManager(LockoutConfig{
		MaxAttempts: 5,
	})

	tests := []struct {
		name           string
		failedAttempts int
		maxDuration    time.Duration
	}{
		{
			name:           "First attempt",
			failedAttempts: 1,
			maxDuration:    2 * time.Second,
		},
		{
			name:           "Second attempt",
			failedAttempts: 2,
			maxDuration:    4 * time.Second,
		},
		{
			name:           "Third attempt",
			failedAttempts: 3,
			maxDuration:    8 * time.Second,
		},
		{
			name:           "Many attempts - capped at 60s",
			failedAttempts: 10,
			maxDuration:    60 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delay := manager.GetDelayDuration(tt.failedAttempts)
			assert.LessOrEqual(t, delay, tt.maxDuration)
		})
	}
}

func TestLockoutManager_ResetAttempts(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()

	manager := NewLockoutManager(LockoutConfig{
		RedisClient:     client,
		MaxAttempts:     5,
		LockoutDuration: 5 * time.Minute,
		AttemptWindow:   10 * time.Minute,
	})

	ctx := context.Background()
	identifier := "test@example.com"

	// Record some failed attempts
	for i := 0; i < 3; i++ {
		err := manager.RecordFailedAttempt(ctx, identifier)
		require.NoError(t, err)
	}

	attempts, err := manager.GetFailedAttempts(ctx, identifier)
	require.NoError(t, err)
	assert.Equal(t, 3, attempts)

	// Reset attempts
	err = manager.ResetAttempts(ctx, identifier)
	require.NoError(t, err)

	// Verify reset
	attempts, err = manager.GetFailedAttempts(ctx, identifier)
	require.NoError(t, err)
	assert.Equal(t, 0, attempts)
}
