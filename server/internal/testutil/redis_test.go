package testutil_test

import (
	"context"
	"testing"
	"time"

	"github.com/dev-jelly/donelist/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupTestRedis(t *testing.T) {
	redis := testutil.SetupTestRedis(t)
	defer redis.Close()

	assert.NotNil(t, redis.Server)
	assert.NotNil(t, redis.Client)

	// Test basic operations
	ctx := context.Background()
	err := redis.Client.Set(ctx, "test-key", "test-value", 0).Err()
	require.NoError(t, err)

	val, err := redis.Client.Get(ctx, "test-key").Result()
	require.NoError(t, err)
	assert.Equal(t, "test-value", val)
}

func TestFlush(t *testing.T) {
	redis := testutil.SetupTestRedis(t)
	defer redis.Close()

	ctx := context.Background()

	// Set some data
	redis.Client.Set(ctx, "key1", "value1", 0)
	redis.Client.Set(ctx, "key2", "value2", 0)

	// Verify data exists
	val, _ := redis.Client.Get(ctx, "key1").Result()
	assert.Equal(t, "value1", val)

	// Flush all data
	redis.Flush(t)

	// Verify data is gone
	_, err := redis.Client.Get(ctx, "key1").Result()
	assert.Error(t, err)
}

func TestFastForward(t *testing.T) {
	redis := testutil.SetupTestRedis(t)
	defer redis.Close()

	ctx := context.Background()

	// Set key with 10 second TTL
	redis.Client.Set(ctx, "expiring-key", "value", 10*time.Second)

	// Fast forward 5 seconds
	redis.FastForward(t, 5*time.Second)

	// Key should still exist
	val, err := redis.Client.Get(ctx, "expiring-key").Result()
	require.NoError(t, err)
	assert.Equal(t, "value", val)

	// Fast forward another 6 seconds (total 11 seconds)
	redis.FastForward(t, 6*time.Second)

	// Key should be expired
	_, err = redis.Client.Get(ctx, "expiring-key").Result()
	assert.Error(t, err)
}

func TestMultipleTests(t *testing.T) {
	t.Run("test 1", func(t *testing.T) {
		redis := testutil.SetupTestRedis(t)
		defer redis.Close()

		ctx := context.Background()
		redis.Client.Set(ctx, "key1", "value1", 0)

		val, err := redis.Client.Get(ctx, "key1").Result()
		require.NoError(t, err)
		assert.Equal(t, "value1", val)
	})

	t.Run("test 2", func(t *testing.T) {
		redis := testutil.SetupTestRedis(t)
		defer redis.Close()

		ctx := context.Background()

		// Should not see data from test 1
		_, err := redis.Client.Get(ctx, "key1").Result()
		assert.Error(t, err)

		redis.Client.Set(ctx, "key2", "value2", 0)
		val, err := redis.Client.Get(ctx, "key2").Result()
		require.NoError(t, err)
		assert.Equal(t, "value2", val)
	})
}
