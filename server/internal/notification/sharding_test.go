package notification

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestShardRouter_GetShardID(t *testing.T) {
	logger := zaptest.NewLogger(t)
	router := NewShardRouter(ShardingByUser, 10, logger)

	userID := uuid.New()
	tenantID := "tenant-1"

	// Test consistent routing
	shardID1 := router.GetShardID(userID, tenantID)
	shardID2 := router.GetShardID(userID, tenantID)

	assert.Equal(t, shardID1, shardID2, "Same user should route to same shard")
	assert.GreaterOrEqual(t, shardID1, 0, "Shard ID should be >= 0")
	assert.Less(t, shardID1, 10, "Shard ID should be < shard count")
}

func TestShardRouter_ShardingStrategy(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name     string
		strategy ShardingStrategy
	}{
		{"ByUser", ShardingByUser},
		{"ByTenant", ShardingByTenant},
		{"ConsistentHash", ShardingConsistentHash},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewShardRouter(tt.strategy, 10, logger)

			userID := uuid.New()
			tenantID := "tenant-1"

			shardID := router.GetShardID(userID, tenantID)
			assert.GreaterOrEqual(t, shardID, 0)
			assert.Less(t, shardID, 10)
		})
	}
}

func TestShardRouter_CheckQuota(t *testing.T) {
	logger := zaptest.NewLogger(t)
	router := NewShardRouter(ShardingByUser, 10, logger)

	tenantID := "tenant-1"
	ctx := context.Background()

	// No quota set - should pass
	err := router.CheckQuota(ctx, tenantID, PriorityNormal)
	assert.NoError(t, err)

	// Set quota
	quota := &TenantQuota{
		MaxNotificationsPerHour: 10,
		MaxNotificationsPerDay:  100,
		PriorityMultiplier:      1.5,
	}
	router.SetTenantQuota(tenantID, quota)

	// Should pass within quota
	for i := 0; i < 5; i++ {
		err = router.CheckQuota(ctx, tenantID, PriorityNormal)
		assert.NoError(t, err)
	}

	// Verify quota is being tracked
	currentQuota := router.GetTenantQuota(tenantID)
	assert.Equal(t, 5, currentQuota.CurrentHourCount)
}

func TestShardRouter_QuotaExceeded(t *testing.T) {
	logger := zaptest.NewLogger(t)
	router := NewShardRouter(ShardingByUser, 10, logger)

	tenantID := "tenant-1"
	ctx := context.Background()

	// Set low quota
	quota := &TenantQuota{
		MaxNotificationsPerHour: 3,
		MaxNotificationsPerDay:  10,
		PriorityMultiplier:      1.5,
	}
	router.SetTenantQuota(tenantID, quota)

	// Use up quota
	for i := 0; i < 3; i++ {
		err := router.CheckQuota(ctx, tenantID, PriorityNormal)
		require.NoError(t, err)
	}

	// Should fail when quota exceeded
	err := router.CheckQuota(ctx, tenantID, PriorityNormal)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "hourly quota exceeded")
}

func TestShardRouter_PriorityMultiplier(t *testing.T) {
	logger := zaptest.NewLogger(t)
	router := NewShardRouter(ShardingByUser, 10, logger)

	tenantID := "tenant-1"
	ctx := context.Background()

	// Set quota with multiplier
	quota := &TenantQuota{
		MaxNotificationsPerHour: 10,
		PriorityMultiplier:      2.0, // 2x for urgent
	}
	router.SetTenantQuota(tenantID, quota)

	// Urgent notifications should get higher quota (20 instead of 10)
	for i := 0; i < 15; i++ {
		err := router.CheckQuota(ctx, tenantID, PriorityUrgent)
		assert.NoError(t, err, "Should allow more urgent notifications due to multiplier")
	}

	currentQuota := router.GetTenantQuota(tenantID)
	assert.Equal(t, 15, currentQuota.CurrentHourCount)
}

func TestShardRouter_QuotaReset(t *testing.T) {
	logger := zaptest.NewLogger(t)
	router := NewShardRouter(ShardingByUser, 10, logger)

	tenantID := "tenant-1"
	ctx := context.Background()

	// Set quota with past reset time
	quota := &TenantQuota{
		MaxNotificationsPerHour: 5,
		CurrentHourCount:        5,
		LastResetHour:           time.Now().Add(-2 * time.Hour),
	}
	router.SetTenantQuota(tenantID, quota)

	// Should reset and allow new notifications
	err := router.CheckQuota(ctx, tenantID, PriorityNormal)
	assert.NoError(t, err, "Quota should reset after time period")

	currentQuota := router.GetTenantQuota(tenantID)
	assert.Equal(t, 1, currentQuota.CurrentHourCount, "Counter should reset")
}

func TestShardRouter_ShardHealth(t *testing.T) {
	logger := zaptest.NewLogger(t)
	router := NewShardRouter(ShardingConsistentHash, 10, logger)

	// Mark some shards as unhealthy
	router.SetShardHealth(3, false)
	router.SetShardHealth(7, false)

	health := router.GetShardHealth()
	assert.False(t, health[3])
	assert.False(t, health[7])

	// Consistent hash should skip unhealthy shards
	userID := uuid.New()
	tenantID := "tenant-1"

	shardID := router.GetShardID(userID, tenantID)
	assert.NotEqual(t, 3, shardID, "Should not route to unhealthy shard 3")
	assert.NotEqual(t, 7, shardID, "Should not route to unhealthy shard 7")
}

func TestShardRouter_Rebalance(t *testing.T) {
	logger := zaptest.NewLogger(t)
	router := NewShardRouter(ShardingConsistentHash, 10, logger)
	ctx := context.Background()

	// Set some shards as healthy
	router.SetShardHealth(0, true)
	router.SetShardHealth(1, true)
	router.SetShardHealth(2, false)

	err := router.RebalanceShards(ctx)
	assert.NoError(t, err)
}

func TestShardRouter_NoHealthyShards(t *testing.T) {
	logger := zaptest.NewLogger(t)
	router := NewShardRouter(ShardingConsistentHash, 3, logger)
	ctx := context.Background()

	// Mark all shards as unhealthy
	router.SetShardHealth(0, false)
	router.SetShardHealth(1, false)
	router.SetShardHealth(2, false)

	err := router.RebalanceShards(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no healthy shards")
}

func TestShardRouter_Distribution(t *testing.T) {
	logger := zaptest.NewLogger(t)
	router := NewShardRouter(ShardingByUser, 10, logger)

	// Test that users are distributed across shards
	shardCounts := make(map[int]int)

	for i := 0; i < 100; i++ {
		userID := uuid.New()
		shardID := router.GetShardID(userID, "tenant-1")
		shardCounts[shardID]++
	}

	// Each shard should get at least some users (not perfect distribution)
	// With 100 users and 10 shards, expect roughly 10 per shard
	nonEmptyShards := 0
	for _, count := range shardCounts {
		if count > 0 {
			nonEmptyShards++
		}
	}

	assert.GreaterOrEqual(t, nonEmptyShards, 7, "Most shards should have users")
}

func TestShardRouter_TenantIsolation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	router := NewShardRouter(ShardingByTenant, 10, logger)

	tenant1 := "tenant-1"
	tenant2 := "tenant-2"

	// Same user in different tenants might go to different shards
	userID := uuid.New()

	shard1 := router.GetShardID(userID, tenant1)
	shard2 := router.GetShardID(userID, tenant2)

	// With tenant-based sharding, all users of same tenant go to same shard
	anotherUser := uuid.New()
	shard1Again := router.GetShardID(anotherUser, tenant1)

	assert.Equal(t, shard1, shard1Again, "Same tenant should route to same shard")
}

func TestTenantQuota_Defaults(t *testing.T) {
	logger := zaptest.NewLogger(t)
	router := NewShardRouter(ShardingByUser, 10, logger)

	quota := &TenantQuota{
		MaxNotificationsPerHour: 100,
	}

	router.SetTenantQuota("tenant-1", quota)

	retrieved := router.GetTenantQuota("tenant-1")
	assert.NotNil(t, retrieved)
	assert.Equal(t, 1.5, retrieved.PriorityMultiplier, "Should have default multiplier")
	assert.False(t, retrieved.LastResetHour.IsZero(), "Should have reset time set")
}

func TestShardRouter_ConcurrentAccess(t *testing.T) {
	logger := zaptest.NewLogger(t)
	router := NewShardRouter(ShardingByUser, 10, logger)
	ctx := context.Background()

	// Set quota
	quota := &TenantQuota{
		MaxNotificationsPerHour: 1000,
	}
	router.SetTenantQuota("tenant-1", quota)

	// Concurrent quota checks
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 10; j++ {
				router.CheckQuota(ctx, "tenant-1", PriorityNormal)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	retrieved := router.GetTenantQuota("tenant-1")
	assert.Equal(t, 100, retrieved.CurrentHourCount)
}
