package notification

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupIdempotencyTest(t *testing.T) (*IdempotencyManager, *redis.Client, *miniredis.Miniredis) {
	// Create miniredis instance
	mr, err := miniredis.Run()
	require.NoError(t, err)

	// Create Redis client
	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	logger := zap.NewNop()
	config := DefaultIdempotencyConfig()

	im := NewIdempotencyManager(client, logger, config)

	return im, client, mr
}

func TestDefaultIdempotencyConfig(t *testing.T) {
	config := DefaultIdempotencyConfig()

	assert.Equal(t, 1*time.Hour, config.Window, "Should have 1 hour window")
	assert.True(t, config.Enabled, "Should be enabled by default")
}

func TestGenerateKey(t *testing.T) {
	im, _, mr := setupIdempotencyTest(t)
	defer mr.Close()

	userID := uuid.New()
	job1 := &NotificationJob{
		ID:     uuid.New().String(),
		UserID: userID,
		Type:   "test",
		Payload: map[string]interface{}{
			"message": "Hello",
		},
	}

	job2 := &NotificationJob{
		ID:     uuid.New().String(), // Different job ID
		UserID: userID,              // Same user
		Type:   "test",              // Same type
		Payload: map[string]interface{}{
			"message": "Hello", // Same payload
		},
	}

	job3 := &NotificationJob{
		ID:     uuid.New().String(),
		UserID: userID,
		Type:   "test",
		Payload: map[string]interface{}{
			"message": "Different", // Different payload
		},
	}

	key1 := im.GenerateKey(job1)
	key2 := im.GenerateKey(job2)
	key3 := im.GenerateKey(job3)

	assert.NotEmpty(t, key1, "Key should not be empty")
	assert.Equal(t, key1, key2, "Same content should generate same key")
	assert.NotEqual(t, key1, key3, "Different content should generate different key")
}

func TestCheckDuplicate(t *testing.T) {
	im, _, mr := setupIdempotencyTest(t)
	defer mr.Close()
	ctx := context.Background()

	userID := uuid.New()
	job := &NotificationJob{
		ID:     uuid.New().String(),
		UserID: userID,
		Type:   "test",
		Payload: map[string]interface{}{
			"message": "Hello",
		},
	}

	t.Run("No duplicate on first check", func(t *testing.T) {
		isDup, err := im.CheckDuplicate(ctx, job)
		assert.NoError(t, err)
		assert.False(t, isDup, "Should not be duplicate on first check")
	})

	t.Run("Detect duplicate after marking processed", func(t *testing.T) {
		// Mark as processed
		err := im.MarkProcessed(ctx, job)
		require.NoError(t, err)

		// Check again - should be duplicate
		isDup, err := im.CheckDuplicate(ctx, job)
		assert.NoError(t, err)
		assert.True(t, isDup, "Should detect duplicate after marking processed")
	})

	t.Run("Different job should not be duplicate", func(t *testing.T) {
		differentJob := &NotificationJob{
			ID:     uuid.New().String(),
			UserID: userID,
			Type:   "test",
			Payload: map[string]interface{}{
				"message": "Different message",
			},
		}

		isDup, err := im.CheckDuplicate(ctx, differentJob)
		assert.NoError(t, err)
		assert.False(t, isDup, "Different job should not be duplicate")
	})
}

func TestCheckDuplicate_Disabled(t *testing.T) {
	im, _, mr := setupIdempotencyTest(t)
	defer mr.Close()
	ctx := context.Background()

	// Disable idempotency
	im.config.Enabled = false

	job := &NotificationJob{
		ID:     uuid.New().String(),
		UserID: uuid.New(),
		Type:   "test",
		Payload: map[string]interface{}{
			"message": "Hello",
		},
	}

	// Mark as processed
	err := im.MarkProcessed(ctx, job)
	assert.NoError(t, err, "Should not error when disabled")

	// Check for duplicate
	isDup, err := im.CheckDuplicate(ctx, job)
	assert.NoError(t, err)
	assert.False(t, isDup, "Should always return false when disabled")
}

func TestMarkProcessed(t *testing.T) {
	im, client, mr := setupIdempotencyTest(t)
	defer mr.Close()
	ctx := context.Background()

	job := &NotificationJob{
		ID:     uuid.New().String(),
		UserID: uuid.New(),
		Type:   "test",
		Payload: map[string]interface{}{
			"message": "Hello",
		},
	}

	t.Run("Successfully mark as processed", func(t *testing.T) {
		err := im.MarkProcessed(ctx, job)
		assert.NoError(t, err)

		// Verify key exists in Redis
		key := im.GetRedisKey(im.GenerateKey(job))
		exists, err := client.Exists(ctx, key).Result()
		assert.NoError(t, err)
		assert.Equal(t, int64(1), exists, "Key should exist in Redis")

		// Verify stored value is job ID
		value, err := client.Get(ctx, key).Result()
		assert.NoError(t, err)
		assert.Equal(t, job.ID, value, "Stored value should be job ID")
	})

	t.Run("Key has correct TTL", func(t *testing.T) {
		job2 := &NotificationJob{
			ID:     uuid.New().String(),
			UserID: uuid.New(),
			Type:   "test2",
			Payload: map[string]interface{}{
				"message": "Hello2",
			},
		}

		err := im.MarkProcessed(ctx, job2)
		require.NoError(t, err)

		key := im.GetRedisKey(im.GenerateKey(job2))
		ttl, err := client.TTL(ctx, key).Result()
		assert.NoError(t, err)
		assert.Greater(t, ttl, time.Duration(0), "TTL should be positive")
		assert.LessOrEqual(t, ttl, im.config.Window, "TTL should not exceed window")
	})
}

func TestRemoveKey(t *testing.T) {
	im, client, mr := setupIdempotencyTest(t)
	defer mr.Close()
	ctx := context.Background()

	job := &NotificationJob{
		ID:     uuid.New().String(),
		UserID: uuid.New(),
		Type:   "test",
		Payload: map[string]interface{}{
			"message": "Hello",
		},
	}

	// Mark as processed
	err := im.MarkProcessed(ctx, job)
	require.NoError(t, err)

	// Verify it exists
	isDup, err := im.CheckDuplicate(ctx, job)
	require.NoError(t, err)
	assert.True(t, isDup, "Should be marked as duplicate")

	// Remove the key
	err = im.RemoveKey(ctx, job)
	assert.NoError(t, err)

	// Verify it's removed
	key := im.GetRedisKey(im.GenerateKey(job))
	exists, err := client.Exists(ctx, key).Result()
	assert.NoError(t, err)
	assert.Equal(t, int64(0), exists, "Key should not exist after removal")

	// Check duplicate again - should be false
	isDup, err = im.CheckDuplicate(ctx, job)
	assert.NoError(t, err)
	assert.False(t, isDup, "Should not be duplicate after key removal")
}

func TestGetStats(t *testing.T) {
	im, _, mr := setupIdempotencyTest(t)
	defer mr.Close()
	ctx := context.Background()

	// Mark several jobs as processed
	for i := 0; i < 3; i++ {
		job := &NotificationJob{
			ID:     uuid.New().String(),
			UserID: uuid.New(),
			Type:   "test",
			Payload: map[string]interface{}{
				"message": "Hello",
				"index":   i,
			},
		}
		err := im.MarkProcessed(ctx, job)
		require.NoError(t, err)
	}

	stats, err := im.GetStats(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.True(t, stats["enabled"].(bool), "Should be enabled")
	assert.Equal(t, float64(3600), stats["window_seconds"], "Should have 1 hour window")
	assert.Equal(t, 3, stats["active_keys"], "Should have 3 active keys")
}

func TestGetRedisKey(t *testing.T) {
	im, _, mr := setupIdempotencyTest(t)
	defer mr.Close()

	idempotencyKey := "test-key-123"
	redisKey := im.GetRedisKey(idempotencyKey)

	assert.Contains(t, redisKey, idempotencyKeyPrefix, "Should contain prefix")
	assert.Contains(t, redisKey, idempotencyKey, "Should contain idempotency key")
}

func TestIdempotencyWindow(t *testing.T) {
	// Create custom config with short window for testing
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	config := IdempotencyConfig{
		Window:  2 * time.Second, // Short window for testing
		Enabled: true,
	}

	im := NewIdempotencyManager(client, zap.NewNop(), config)
	ctx := context.Background()

	job := &NotificationJob{
		ID:     uuid.New().String(),
		UserID: uuid.New(),
		Type:   "test",
		Payload: map[string]interface{}{
			"message": "Hello",
		},
	}

	// Mark as processed
	err = im.MarkProcessed(ctx, job)
	require.NoError(t, err)

	// Should be duplicate immediately
	isDup, err := im.CheckDuplicate(ctx, job)
	assert.NoError(t, err)
	assert.True(t, isDup, "Should be duplicate within window")

	// Fast forward time in miniredis
	mr.FastForward(3 * time.Second)

	// Should no longer be duplicate after window expires
	isDup, err = im.CheckDuplicate(ctx, job)
	assert.NoError(t, err)
	assert.False(t, isDup, "Should not be duplicate after window expires")
}
