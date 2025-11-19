package ratelimit

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ulule/limiter/v3"
)

func setupTestRedis(t *testing.T) *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   15, // Use DB 15 for testing
	})

	// Clear test DB
	ctx := context.Background()
	client.FlushDB(ctx)

	// Test connection
	_, err := client.Ping(ctx).Result()
	if err != nil {
		t.Skip("Redis not available, skipping test")
	}

	return client
}

func TestService_CheckLimit(t *testing.T) {
	redisClient := setupTestRedis(t)
	defer redisClient.Close()

	cfg := Config{
		RedisClient: redisClient,
		DefaultRate: limiter.Rate{
			Period: 1 * time.Second,
			Limit:  5,
		},
		TierLimits: TierLimits{
			Free: limiter.Rate{
				Period: 1 * time.Second,
				Limit:  3,
			},
			Premium: limiter.Rate{
				Period: 1 * time.Second,
				Limit:  10,
			},
			Enterprise: limiter.Rate{
				Period: 1 * time.Second,
				Limit:  100,
			},
		},
	}

	service, err := NewService(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	userID := "test-user-123"
	endpoint := "/api/v1/test"

	t.Run("Free tier limits", func(t *testing.T) {
		redisClient.FlushDB(ctx)

		// Should allow first 3 requests
		for i := 0; i < 3; i++ {
			allowed, remaining, _, err := service.CheckLimit(ctx, userID, endpoint, "free")
			require.NoError(t, err)
			assert.True(t, allowed, "Request %d should be allowed", i+1)
			assert.Equal(t, 2-i, remaining)
		}

		// 4th request should be blocked
		allowed, remaining, _, err := service.CheckLimit(ctx, userID, endpoint, "free")
		require.NoError(t, err)
		assert.False(t, allowed, "4th request should be blocked")
		assert.Equal(t, 0, remaining)

		// Wait for reset
		time.Sleep(1100 * time.Millisecond)

		// Should be allowed again
		allowed, _, _, err = service.CheckLimit(ctx, userID, endpoint, "free")
		require.NoError(t, err)
		assert.True(t, allowed, "Should be allowed after reset")
	})

	t.Run("Premium tier limits", func(t *testing.T) {
		redisClient.FlushDB(ctx)

		// Should allow 10 requests for premium
		for i := 0; i < 10; i++ {
			allowed, _, _, err := service.CheckLimit(ctx, userID, endpoint, "premium")
			require.NoError(t, err)
			assert.True(t, allowed, "Premium request %d should be allowed", i+1)
		}

		// 11th request should be blocked
		allowed, _, _, err := service.CheckLimit(ctx, userID, endpoint, "premium")
		require.NoError(t, err)
		assert.False(t, allowed, "11th premium request should be blocked")
	})

	t.Run("Enterprise tier limits", func(t *testing.T) {
		redisClient.FlushDB(ctx)

		// Should allow many requests for enterprise
		for i := 0; i < 50; i++ {
			allowed, _, _, err := service.CheckLimit(ctx, userID, endpoint, "enterprise")
			require.NoError(t, err)
			assert.True(t, allowed, "Enterprise request %d should be allowed", i+1)
		}
	})
}

func TestService_CheckLimitForIP(t *testing.T) {
	redisClient := setupTestRedis(t)
	defer redisClient.Close()

	cfg := Config{
		RedisClient: redisClient,
		DefaultRate: limiter.Rate{
			Period: 1 * time.Second,
			Limit:  4,
		},
		TierLimits: TierLimits{},
	}

	service, err := NewService(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	ip := "192.168.1.100"
	endpoint := "/api/v1/test"

	// Anonymous users get half the default rate (2 requests)
	for i := 0; i < 2; i++ {
		allowed, remaining, _, err := service.CheckLimitForIP(ctx, ip, endpoint)
		require.NoError(t, err)
		assert.True(t, allowed, "Request %d should be allowed", i+1)
		assert.Equal(t, 1-i, remaining)
	}

	// 3rd request should be blocked
	allowed, _, _, err := service.CheckLimitForIP(ctx, ip, endpoint)
	require.NoError(t, err)
	assert.False(t, allowed, "3rd request should be blocked for anonymous")
}

func TestService_ResetLimit(t *testing.T) {
	redisClient := setupTestRedis(t)
	defer redisClient.Close()

	cfg := Config{
		RedisClient: redisClient,
		DefaultRate: limiter.Rate{
			Period: 1 * time.Minute,
			Limit:  2,
		},
		TierLimits: TierLimits{},
	}

	service, err := NewService(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	userID := "reset-test-user"
	endpoint := "/api/v1/test"

	// Consume all requests
	for i := 0; i < 2; i++ {
		_, _, _, err := service.CheckLimit(ctx, userID, endpoint, "free")
		require.NoError(t, err)
	}

	// Should be blocked
	allowed, _, _, err := service.CheckLimit(ctx, userID, endpoint, "free")
	require.NoError(t, err)
	assert.False(t, allowed, "Should be blocked after limit")

	// Reset the limit
	err = service.ResetLimit(ctx, userID, endpoint)
	require.NoError(t, err)

	// Should be allowed again
	allowed, _, _, err = service.CheckLimit(ctx, userID, endpoint, "free")
	require.NoError(t, err)
	assert.True(t, allowed, "Should be allowed after reset")
}

func TestService_EndpointSpecificLimits(t *testing.T) {
	redisClient := setupTestRedis(t)
	defer redisClient.Close()

	cfg := Config{
		RedisClient: redisClient,
		DefaultRate: limiter.Rate{
			Period: 1 * time.Second,
			Limit:  10,
		},
		TierLimits: TierLimits{},
		Endpoints: []EndpointConfig{
			{
				Path: "/api/v1/auth/login",
				Rate: limiter.Rate{
					Period: 1 * time.Second,
					Limit:  2, // Strict limit for login
				},
			},
		},
	}

	service, err := NewService(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	userID := "test-user"

	t.Run("Login endpoint strict limit", func(t *testing.T) {
		redisClient.FlushDB(ctx)

		// Login endpoint should only allow 2 requests
		for i := 0; i < 2; i++ {
			allowed, _, _, err := service.CheckLimit(ctx, userID, "/api/v1/auth/login", "free")
			require.NoError(t, err)
			assert.True(t, allowed, "Login request %d should be allowed", i+1)
		}

		// 3rd login should be blocked
		allowed, _, _, err := service.CheckLimit(ctx, userID, "/api/v1/auth/login", "free")
		require.NoError(t, err)
		assert.False(t, allowed, "3rd login should be blocked")
	})

	t.Run("Other endpoints use default limit", func(t *testing.T) {
		redisClient.FlushDB(ctx)

		// Other endpoints should allow 10 requests
		for i := 0; i < 10; i++ {
			allowed, _, _, err := service.CheckLimit(ctx, userID, "/api/v1/users", "free")
			require.NoError(t, err)
			assert.True(t, allowed, "Request %d should be allowed", i+1)
		}

		// 11th should be blocked
		allowed, _, _, err := service.CheckLimit(ctx, userID, "/api/v1/users", "free")
		require.NoError(t, err)
		assert.False(t, allowed, "11th request should be blocked")
	})
}

func TestService_GetLimitInfo(t *testing.T) {
	redisClient := setupTestRedis(t)
	defer redisClient.Close()

	cfg := Config{
		RedisClient: redisClient,
		DefaultRate: limiter.Rate{
			Period: 1 * time.Second,
			Limit:  5,
		},
		TierLimits: TierLimits{
			Free: limiter.Rate{
				Period: 1 * time.Second,
				Limit:  3,
			},
		},
	}

	service, err := NewService(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	userID := "info-test-user"
	endpoint := "/api/v1/test"

	// Get info without consuming
	info, err := service.GetLimitInfo(ctx, userID, endpoint, "free")
	require.NoError(t, err)
	assert.Equal(t, int64(3), info.Limit)
	assert.Equal(t, int64(3), info.Remaining)

	// Consume one request
	_, _, _, err = service.CheckLimit(ctx, userID, endpoint, "free")
	require.NoError(t, err)

	// Check info again
	info, err = service.GetLimitInfo(ctx, userID, endpoint, "free")
	require.NoError(t, err)
	assert.Equal(t, int64(3), info.Limit)
	assert.Equal(t, int64(2), info.Remaining)
}

func TestService_BurstControl(t *testing.T) {
	redisClient := setupTestRedis(t)
	defer redisClient.Close()

	cfg := Config{
		RedisClient: redisClient,
		DefaultRate: limiter.Rate{
			Period: 1 * time.Minute,
			Limit:  100,
		},
		TierLimits: TierLimits{},
		Endpoints: []EndpointConfig{
			{
				Path: "/api/v1/auth/login",
				Rate: limiter.Rate{
					Period: 5 * time.Minute,
					Limit:  10,
				},
				Burst: &BurstConfig{
					AllowBurst:  true,
					BurstSize:   3,
					BurstPeriod: 10 * time.Second,
				},
			},
		},
	}

	service, err := NewService(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	userID := "burst-test-user"
	endpoint := "/api/v1/auth/login"

	t.Run("Burst limit enforcement", func(t *testing.T) {
		redisClient.FlushDB(ctx)

		// First 3 requests should be allowed (within burst limit)
		for i := 0; i < 3; i++ {
			allowed, err := service.CheckBurst(ctx, userID, endpoint)
			require.NoError(t, err)
			assert.True(t, allowed, "Burst request %d should be allowed", i+1)
		}

		// 4th request should be blocked (exceeds burst)
		allowed, err := service.CheckBurst(ctx, userID, endpoint)
		require.NoError(t, err)
		assert.False(t, allowed, "4th burst request should be blocked")

		// Wait for burst period to reset
		time.Sleep(11 * time.Second)

		// Should be allowed again after burst period
		allowed, err = service.CheckBurst(ctx, userID, endpoint)
		require.NoError(t, err)
		assert.True(t, allowed, "Should be allowed after burst period reset")
	})

	t.Run("Get burst info", func(t *testing.T) {
		redisClient.FlushDB(ctx)

		// No bursts yet
		current, limit, err := service.GetBurstInfo(ctx, userID, endpoint)
		require.NoError(t, err)
		assert.Equal(t, int64(0), current)
		assert.Equal(t, int64(3), limit)

		// Make 2 burst requests
		service.CheckBurst(ctx, userID, endpoint)
		service.CheckBurst(ctx, userID, endpoint)

		// Check burst info
		current, limit, err = service.GetBurstInfo(ctx, userID, endpoint)
		require.NoError(t, err)
		assert.Equal(t, int64(2), current)
		assert.Equal(t, int64(3), limit)
	})

	t.Run("Reset burst", func(t *testing.T) {
		redisClient.FlushDB(ctx)

		// Make burst requests
		for i := 0; i < 3; i++ {
			service.CheckBurst(ctx, userID, endpoint)
		}

		// Should be blocked
		allowed, err := service.CheckBurst(ctx, userID, endpoint)
		require.NoError(t, err)
		assert.False(t, allowed)

		// Reset burst
		err = service.ResetBurst(ctx, userID, endpoint)
		require.NoError(t, err)

		// Should be allowed again
		allowed, err = service.CheckBurst(ctx, userID, endpoint)
		require.NoError(t, err)
		assert.True(t, allowed, "Should be allowed after burst reset")
	})

	t.Run("Endpoint without burst config", func(t *testing.T) {
		redisClient.FlushDB(ctx)

		// Endpoint without burst config should always allow
		endpointNoBurst := "/api/v1/users"
		for i := 0; i < 10; i++ {
			allowed, err := service.CheckBurst(ctx, userID, endpointNoBurst)
			require.NoError(t, err)
			assert.True(t, allowed, "Requests without burst config should always be allowed")
		}
	})
}

func TestService_ConcurrentBurstRequests(t *testing.T) {
	redisClient := setupTestRedis(t)
	defer redisClient.Close()

	cfg := Config{
		RedisClient: redisClient,
		DefaultRate: limiter.Rate{
			Period: 1 * time.Minute,
			Limit:  1000,
		},
		TierLimits: TierLimits{},
		Endpoints: []EndpointConfig{
			{
				Path: "/api/v1/test",
				Rate: limiter.Rate{
					Period: 1 * time.Minute,
					Limit:  100,
				},
				Burst: &BurstConfig{
					AllowBurst:  true,
					BurstSize:   5,
					BurstPeriod: 5 * time.Second,
				},
			},
		},
	}

	service, err := NewService(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	userID := "concurrent-test-user"
	endpoint := "/api/v1/test"

	redisClient.FlushDB(ctx)

	// Simulate concurrent requests
	numGoroutines := 10
	allowedCount := 0
	blockedCount := 0
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed, err := service.CheckBurst(ctx, userID, endpoint)
			if err != nil {
				t.Errorf("Burst check error: %v", err)
				return
			}
			mu.Lock()
			if allowed {
				allowedCount++
			} else {
				blockedCount++
			}
			mu.Unlock()
		}()
	}

	wg.Wait()

	// Should allow up to burst size (5) and block the rest
	assert.LessOrEqual(t, allowedCount, 5, "Should not allow more than burst size")
	assert.Equal(t, numGoroutines-allowedCount, blockedCount, "Blocked count should match")
}

func TestService_IPBasedBurstControl(t *testing.T) {
	redisClient := setupTestRedis(t)
	defer redisClient.Close()

	cfg := Config{
		RedisClient: redisClient,
		DefaultRate: limiter.Rate{
			Period: 1 * time.Minute,
			Limit:  10,
		},
		TierLimits: TierLimits{},
	}

	service, err := NewService(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	ip1 := "192.168.1.100"
	ip2 := "192.168.1.101"
	endpoint := "/api/v1/test"

	t.Run("Different IPs have separate limits", func(t *testing.T) {
		redisClient.FlushDB(ctx)

		// IP1 uses its limit
		for i := 0; i < 5; i++ {
			allowed, _, _, err := service.CheckLimitForIP(ctx, ip1, endpoint)
			require.NoError(t, err)
			assert.True(t, allowed)
		}

		// IP2 should still have full quota
		allowed, remaining, _, err := service.CheckLimitForIP(ctx, ip2, endpoint)
		require.NoError(t, err)
		assert.True(t, allowed)
		assert.Equal(t, 4, remaining) // Half of default (10/2) = 5, minus 1 = 4
	})
}

func BenchmarkService_CheckLimit(b *testing.B) {
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   15,
	})
	defer redisClient.Close()

	// Test connection
	ctx := context.Background()
	_, err := redisClient.Ping(ctx).Result()
	if err != nil {
		b.Skip("Redis not available, skipping benchmark")
	}

	redisClient.FlushDB(ctx)

	cfg := Config{
		RedisClient: redisClient,
		DefaultRate: limiter.Rate{
			Period: 1 * time.Minute,
			Limit:  10000,
		},
		TierLimits: TierLimits{},
	}

	service, err := NewService(cfg)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		userID := "bench-user"
		endpoint := "/api/v1/test"

		for pb.Next() {
			_, _, _, _ = service.CheckLimit(ctx, userID, endpoint, "free")
		}
	})
}