package signing

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

func TestNonceTracker_CheckAndRecordNonce(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	tracker := NewNonceTracker(client, 5*time.Minute)
	ctx := context.Background()

	t.Run("first use of nonce succeeds", func(t *testing.T) {
		nonce := "test-nonce-1"
		err := tracker.CheckAndRecordNonce(ctx, nonce)
		assert.NoError(t, err)
	})

	t.Run("reuse of nonce fails", func(t *testing.T) {
		nonce := "test-nonce-2"

		// First use should succeed
		err := tracker.CheckAndRecordNonce(ctx, nonce)
		require.NoError(t, err)

		// Second use should fail
		err = tracker.CheckAndRecordNonce(ctx, nonce)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nonce has already been used")
		assert.Contains(t, err.Error(), "replay attack")
	})

	t.Run("empty nonce fails", func(t *testing.T) {
		err := tracker.CheckAndRecordNonce(ctx, "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nonce cannot be empty")
	})

	t.Run("nonce expires after TTL", func(t *testing.T) {
		shortTTL := 100 * time.Millisecond
		shortTracker := NewNonceTracker(client, shortTTL)
		nonce := "test-nonce-expiring"

		// First use succeeds
		err := shortTracker.CheckAndRecordNonce(ctx, nonce)
		require.NoError(t, err)

		// Second use immediately fails
		err = shortTracker.CheckAndRecordNonce(ctx, nonce)
		assert.Error(t, err)

		// Fast forward time in miniredis
		mr.FastForward(200 * time.Millisecond)

		// After TTL, should be able to use again
		err = shortTracker.CheckAndRecordNonce(ctx, nonce)
		assert.NoError(t, err)
	})

	t.Run("different nonces work independently", func(t *testing.T) {
		nonce1 := "test-nonce-a"
		nonce2 := "test-nonce-b"

		err := tracker.CheckAndRecordNonce(ctx, nonce1)
		require.NoError(t, err)

		err = tracker.CheckAndRecordNonce(ctx, nonce2)
		assert.NoError(t, err)

		// Both should fail on reuse
		err = tracker.CheckAndRecordNonce(ctx, nonce1)
		assert.Error(t, err)

		err = tracker.CheckAndRecordNonce(ctx, nonce2)
		assert.Error(t, err)
	})
}

func TestNonceTracker_IsNonceUsed(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	tracker := NewNonceTracker(client, 5*time.Minute)
	ctx := context.Background()

	t.Run("unused nonce returns false", func(t *testing.T) {
		nonce := "test-nonce-unused"
		used, err := tracker.IsNonceUsed(ctx, nonce)
		assert.NoError(t, err)
		assert.False(t, used)
	})

	t.Run("used nonce returns true", func(t *testing.T) {
		nonce := "test-nonce-used"

		// Record the nonce
		err := tracker.CheckAndRecordNonce(ctx, nonce)
		require.NoError(t, err)

		// Check if it's used
		used, err := tracker.IsNonceUsed(ctx, nonce)
		assert.NoError(t, err)
		assert.True(t, used)
	})

	t.Run("empty nonce fails", func(t *testing.T) {
		_, err := tracker.IsNonceUsed(ctx, "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nonce cannot be empty")
	})
}

func TestGenerateNonce(t *testing.T) {
	t.Run("generate valid nonce", func(t *testing.T) {
		nonce, err := GenerateNonce()
		assert.NoError(t, err)
		assert.NotEmpty(t, nonce)

		// Nonce should be 32 hex characters (16 bytes)
		assert.Len(t, nonce, 32)
		assert.Regexp(t, "^[a-f0-9]{32}$", nonce)
	})

	t.Run("generate unique nonces", func(t *testing.T) {
		nonce1, err := GenerateNonce()
		require.NoError(t, err)

		nonce2, err := GenerateNonce()
		require.NoError(t, err)

		assert.NotEqual(t, nonce1, nonce2, "Each generated nonce should be unique")
	})

	t.Run("generate multiple nonces", func(t *testing.T) {
		nonces := make(map[string]bool)
		for i := 0; i < 100; i++ {
			nonce, err := GenerateNonce()
			require.NoError(t, err)
			assert.False(t, nonces[nonce], "Nonce collision detected")
			nonces[nonce] = true
		}
	})
}

func TestNonceTracker_ConcurrentAccess(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	tracker := NewNonceTracker(client, 5*time.Minute)
	ctx := context.Background()
	nonce := "test-concurrent-nonce"

	// Try to use the same nonce concurrently
	done := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func() {
			done <- tracker.CheckAndRecordNonce(ctx, nonce)
		}()
	}

	// Collect results
	successCount := 0
	failureCount := 0
	for i := 0; i < 10; i++ {
		err := <-done
		if err == nil {
			successCount++
		} else {
			failureCount++
		}
	}

	// Only one should succeed due to Redis atomicity
	assert.Equal(t, 1, successCount, "Only one goroutine should succeed")
	assert.Equal(t, 9, failureCount, "Nine goroutines should fail")
}

func TestNonceTracker_RedisKey(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	tracker := NewNonceTracker(client, 5*time.Minute)
	ctx := context.Background()
	nonce := "test-key-format"

	err := tracker.CheckAndRecordNonce(ctx, nonce)
	require.NoError(t, err)

	// Check that the key exists in Redis with correct prefix
	key := "signing:nonce:" + nonce
	exists, err := client.Exists(ctx, key).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), exists)
}
