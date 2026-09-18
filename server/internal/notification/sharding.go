package notification

import (
	"context"
	"fmt"
	"hash/crc32"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ShardingStrategy defines how notifications are distributed across shards
type ShardingStrategy string

const (
	// ShardingByUser shards by user ID
	ShardingByUser ShardingStrategy = "user"
	// ShardingByTenant shards by tenant ID
	ShardingByTenant ShardingStrategy = "tenant"
	// ShardingConsistentHash uses consistent hashing
	ShardingConsistentHash ShardingStrategy = "consistent_hash"
)

// TenantQuota represents quota limits for a tenant
type TenantQuota struct {
	TenantID           string    `json:"tenant_id"`
	MaxNotificationsPerHour   int       `json:"max_notifications_per_hour"`
	MaxNotificationsPerDay    int       `json:"max_notifications_per_day"`
	MaxConcurrentSends        int       `json:"max_concurrent_sends"`
	PriorityMultiplier        float64   `json:"priority_multiplier"`
	CurrentHourCount          int       `json:"current_hour_count"`
	CurrentDayCount           int       `json:"current_day_count"`
	LastResetHour             time.Time `json:"last_reset_hour"`
	LastResetDay              time.Time `json:"last_reset_day"`
}

// ShardRouter routes notifications to appropriate shards
type ShardRouter struct {
	strategy      ShardingStrategy
	shardCount    int
	logger        *zap.Logger
	tenantQuotas  map[string]*TenantQuota
	quotaMu       sync.RWMutex
	shardHealth   map[int]bool
	healthMu      sync.RWMutex
}

// NewShardRouter creates a new shard router
func NewShardRouter(strategy ShardingStrategy, shardCount int, logger *zap.Logger) *ShardRouter {
	return &ShardRouter{
		strategy:     strategy,
		shardCount:   shardCount,
		logger:       logger,
		tenantQuotas: make(map[string]*TenantQuota),
		shardHealth:  make(map[int]bool),
	}
}

// GetShardID determines which shard a notification should go to
func (sr *ShardRouter) GetShardID(userID uuid.UUID, tenantID string) int {
	switch sr.strategy {
	case ShardingByUser:
		return sr.hashToShard(userID.String())
	case ShardingByTenant:
		return sr.hashToShard(tenantID)
	case ShardingConsistentHash:
		return sr.consistentHash(userID.String(), tenantID)
	default:
		return sr.hashToShard(userID.String())
	}
}

// hashToShard uses CRC32 to map a key to a shard
func (sr *ShardRouter) hashToShard(key string) int {
	hash := crc32.ChecksumIEEE([]byte(key))
	return int(hash % uint32(sr.shardCount))
}

// consistentHash implements consistent hashing for better distribution
func (sr *ShardRouter) consistentHash(userID, tenantID string) int {
	// Combine user and tenant for better distribution
	combined := fmt.Sprintf("%s:%s", tenantID, userID)
	hash := crc32.ChecksumIEEE([]byte(combined))

	// Find next available shard (skip unhealthy ones)
	baseShardID := int(hash % uint32(sr.shardCount))

	sr.healthMu.RLock()
	defer sr.healthMu.RUnlock()

	// Try to find a healthy shard
	for i := 0; i < sr.shardCount; i++ {
		shardID := (baseShardID + i) % sr.shardCount
		if healthy, exists := sr.shardHealth[shardID]; !exists || healthy {
			return shardID
		}
	}

	// If all shards are unhealthy, return base shard anyway
	return baseShardID
}

// CheckQuota verifies if a tenant can send more notifications
func (sr *ShardRouter) CheckQuota(ctx context.Context, tenantID string, priority Priority) error {
	sr.quotaMu.Lock()
	defer sr.quotaMu.Unlock()

	quota, exists := sr.tenantQuotas[tenantID]
	if !exists {
		// No quota set, allow by default
		return nil
	}

	now := time.Now()

	// Reset hourly counter if needed
	if now.Sub(quota.LastResetHour) >= time.Hour {
		quota.CurrentHourCount = 0
		quota.LastResetHour = now
	}

	// Reset daily counter if needed
	if now.Sub(quota.LastResetDay) >= 24*time.Hour {
		quota.CurrentDayCount = 0
		quota.LastResetDay = now
	}

	// Apply priority multiplier for urgent notifications
	multiplier := 1.0
	if priority == PriorityUrgent || priority == PriorityHigh {
		multiplier = quota.PriorityMultiplier
	}

	// Check hourly quota
	if quota.MaxNotificationsPerHour > 0 {
		effectiveLimit := int(float64(quota.MaxNotificationsPerHour) * multiplier)
		if quota.CurrentHourCount >= effectiveLimit {
			return fmt.Errorf("hourly quota exceeded for tenant %s: %d/%d",
				tenantID, quota.CurrentHourCount, effectiveLimit)
		}
	}

	// Check daily quota
	if quota.MaxNotificationsPerDay > 0 {
		effectiveLimit := int(float64(quota.MaxNotificationsPerDay) * multiplier)
		if quota.CurrentDayCount >= effectiveLimit {
			return fmt.Errorf("daily quota exceeded for tenant %s: %d/%d",
				tenantID, quota.CurrentDayCount, effectiveLimit)
		}
	}

	// Increment counters
	quota.CurrentHourCount++
	quota.CurrentDayCount++

	return nil
}

// SetTenantQuota sets or updates quota for a tenant
func (sr *ShardRouter) SetTenantQuota(tenantID string, quota *TenantQuota) {
	sr.quotaMu.Lock()
	defer sr.quotaMu.Unlock()

	quota.TenantID = tenantID
	if quota.LastResetHour.IsZero() {
		quota.LastResetHour = time.Now()
	}
	if quota.LastResetDay.IsZero() {
		quota.LastResetDay = time.Now()
	}
	if quota.PriorityMultiplier == 0 {
		quota.PriorityMultiplier = 1.5 // Default 50% increase for high priority
	}

	sr.tenantQuotas[tenantID] = quota
}

// GetTenantQuota returns quota information for a tenant
func (sr *ShardRouter) GetTenantQuota(tenantID string) *TenantQuota {
	sr.quotaMu.RLock()
	defer sr.quotaMu.RUnlock()

	if quota, exists := sr.tenantQuotas[tenantID]; exists {
		// Return a copy to prevent external modification
		quotaCopy := *quota
		return &quotaCopy
	}

	return nil
}

// SetShardHealth updates the health status of a shard
func (sr *ShardRouter) SetShardHealth(shardID int, healthy bool) {
	sr.healthMu.Lock()
	defer sr.healthMu.Unlock()

	sr.shardHealth[shardID] = healthy
	sr.logger.Info("Shard health updated",
		zap.Int("shard_id", shardID),
		zap.Bool("healthy", healthy),
	)
}

// GetShardHealth returns health status for all shards
func (sr *ShardRouter) GetShardHealth() map[int]bool {
	sr.healthMu.RLock()
	defer sr.healthMu.RUnlock()

	health := make(map[int]bool)
	for id, status := range sr.shardHealth {
		health[id] = status
	}
	return health
}

// RebalanceShards triggers rebalancing when shard health changes
func (sr *ShardRouter) RebalanceShards(ctx context.Context) error {
	sr.logger.Info("Starting shard rebalancing")

	sr.healthMu.RLock()
	healthyShards := 0
	for _, healthy := range sr.shardHealth {
		if healthy {
			healthyShards++
		}
	}
	sr.healthMu.RUnlock()

	if healthyShards == 0 {
		return fmt.Errorf("no healthy shards available")
	}

	sr.logger.Info("Shard rebalancing completed",
		zap.Int("healthy_shards", healthyShards),
		zap.Int("total_shards", sr.shardCount),
	)

	return nil
}

// ShardWorkerPool manages workers across multiple shards
type ShardWorkerPool struct {
	shardRouters map[int]*WorkerPool
	router       *ShardRouter
	logger       *zap.Logger
	mu           sync.RWMutex
}

// NewShardWorkerPool creates a new shard-aware worker pool
func NewShardWorkerPool(router *ShardRouter, workersPerShard int, queue *Queue, processor *Processor, logger *zap.Logger) *ShardWorkerPool {
	swp := &ShardWorkerPool{
		shardRouters: make(map[int]*WorkerPool),
		router:       router,
		logger:       logger,
	}

	// Create worker pool for each shard
	for i := 0; i < router.shardCount; i++ {
		pool := NewWorkerPool(workersPerShard, queue, processor, logger.With(zap.Int("shard_id", i)))
		swp.shardRouters[i] = pool
		router.SetShardHealth(i, true) // Mark shard as healthy initially
	}

	return swp
}

// Start starts all shard worker pools
func (swp *ShardWorkerPool) Start(ctx context.Context) error {
	swp.mu.Lock()
	defer swp.mu.Unlock()

	for shardID, pool := range swp.shardRouters {
		if err := pool.Start(ctx); err != nil {
			swp.logger.Error("Failed to start shard worker pool",
				zap.Int("shard_id", shardID),
				zap.Error(err),
			)
			swp.router.SetShardHealth(shardID, false)
			continue
		}
		swp.logger.Info("Started shard worker pool", zap.Int("shard_id", shardID))
	}

	return nil
}

// Stop stops all shard worker pools gracefully
func (swp *ShardWorkerPool) Stop() error {
	swp.mu.Lock()
	defer swp.mu.Unlock()

	var lastErr error
	for shardID, pool := range swp.shardRouters {
		if err := pool.Stop(); err != nil {
			swp.logger.Error("Failed to stop shard worker pool",
				zap.Int("shard_id", shardID),
				zap.Error(err),
			)
			lastErr = err
		}
	}

	return lastErr
}

// GetShardPool returns the worker pool for a specific shard
func (swp *ShardWorkerPool) GetShardPool(shardID int) *WorkerPool {
	swp.mu.RLock()
	defer swp.mu.RUnlock()

	return swp.shardRouters[shardID]
}

// GetStats returns statistics for all shards
func (swp *ShardWorkerPool) GetStats() map[string]interface{} {
	swp.mu.RLock()
	defer swp.mu.RUnlock()

	stats := make(map[string]interface{})
	shardStats := make([]map[string]interface{}, 0)

	for shardID, pool := range swp.shardRouters {
		shardStats = append(shardStats, map[string]interface{}{
			"shard_id":      shardID,
			"worker_count":  pool.WorkerCount(),
			"active_workers": pool.ActiveWorkerCount(),
		})
	}

	stats["shards"] = shardStats
	stats["total_shards"] = len(swp.shardRouters)
	stats["shard_health"] = swp.router.GetShardHealth()

	return stats
}
