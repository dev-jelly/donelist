package health

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
)

// PostgresChecker checks PostgreSQL database health
type PostgresChecker struct {
	db      *sqlx.DB
	timeout time.Duration
}

// NewPostgresChecker creates a new PostgreSQL health checker
func NewPostgresChecker(db *sqlx.DB, timeout time.Duration) *PostgresChecker {
	return &PostgresChecker{
		db:      db,
		timeout: timeout,
	}
}

// Name returns the component name
func (c *PostgresChecker) Name() string {
	return "postgresql"
}

// Check performs the health check
func (c *PostgresChecker) Check() ComponentHealth {
	start := time.Now()
	health := ComponentHealth{
		Name:      c.Name(),
		Status:    StatusHealthy,
		Timestamp: start,
		Metadata:  make(map[string]interface{}),
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	// Check connection
	if err := c.db.PingContext(ctx); err != nil {
		health.Status = StatusUnhealthy
		health.Error = fmt.Sprintf("ping failed: %v", err)
		health.Duration = time.Since(start)
		return health
	}

	// Get connection stats
	stats := c.db.Stats()
	health.Metadata["open_connections"] = stats.OpenConnections
	health.Metadata["in_use"] = stats.InUse
	health.Metadata["idle"] = stats.Idle
	health.Metadata["wait_count"] = stats.WaitCount
	health.Metadata["max_open_connections"] = stats.MaxOpenConnections

	// Check if we can execute a query
	var result int
	query := "SELECT 1"
	if err := c.db.GetContext(ctx, &result, query); err != nil {
		health.Status = StatusDegraded
		health.Error = fmt.Sprintf("query failed: %v", err)
		health.Message = "database is reachable but queries are failing"
	} else {
		health.Message = "database is healthy and responding"
	}

	// Check connection pool health
	if stats.OpenConnections >= stats.MaxOpenConnections {
		health.Status = StatusDegraded
		health.Message = "connection pool is at maximum capacity"
	}

	health.Duration = time.Since(start)
	return health
}

// RedisChecker checks Redis health
type RedisChecker struct {
	client  *redis.Client
	timeout time.Duration
}

// NewRedisChecker creates a new Redis health checker
func NewRedisChecker(client *redis.Client, timeout time.Duration) *RedisChecker {
	return &RedisChecker{
		client:  client,
		timeout: timeout,
	}
}

// Name returns the component name
func (c *RedisChecker) Name() string {
	return "redis"
}

// Check performs the health check
func (c *RedisChecker) Check() ComponentHealth {
	start := time.Now()
	health := ComponentHealth{
		Name:      c.Name(),
		Status:    StatusHealthy,
		Timestamp: start,
		Metadata:  make(map[string]interface{}),
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	// Check connection
	if err := c.client.Ping(ctx).Err(); err != nil {
		health.Status = StatusUnhealthy
		health.Error = fmt.Sprintf("ping failed: %v", err)
		health.Duration = time.Since(start)
		return health
	}

	// Get Redis info
	_, err := c.client.Info(ctx, "server", "stats").Result()
	if err != nil {
		health.Status = StatusDegraded
		health.Error = fmt.Sprintf("info command failed: %v", err)
		health.Message = "redis is reachable but cannot retrieve stats"
	} else {
		health.Message = "redis is healthy and responding"
	}

	// Get pool stats
	stats := c.client.PoolStats()
	health.Metadata["hits"] = stats.Hits
	health.Metadata["misses"] = stats.Misses
	health.Metadata["timeouts"] = stats.Timeouts
	health.Metadata["total_conns"] = stats.TotalConns
	health.Metadata["idle_conns"] = stats.IdleConns
	health.Metadata["stale_conns"] = stats.StaleConns

	// Check for high timeout rate
	if stats.Timeouts > 0 {
		timeoutRate := float64(stats.Timeouts) / float64(stats.TotalConns)
		if timeoutRate > 0.1 {
			health.Status = StatusDegraded
			health.Message = "high timeout rate detected"
		}
	}

	health.Duration = time.Since(start)
	return health
}

// SystemChecker checks system-level health
type SystemChecker struct {
	maxMemoryMB    int64
	maxGoroutines  int
	getCurrentMem  func() int64
	getCurrentGoro func() int
}

// NewSystemChecker creates a new system health checker
func NewSystemChecker(getCurrentMem func() int64, getCurrentGoro func() int) *SystemChecker {
	return &SystemChecker{
		maxMemoryMB:    1024, // 1GB default
		maxGoroutines:  10000,
		getCurrentMem:  getCurrentMem,
		getCurrentGoro: getCurrentGoro,
	}
}

// Name returns the component name
func (c *SystemChecker) Name() string {
	return "system"
}

// Check performs the health check
func (c *SystemChecker) Check() ComponentHealth {
	start := time.Now()
	health := ComponentHealth{
		Name:      c.Name(),
		Status:    StatusHealthy,
		Timestamp: start,
		Metadata:  make(map[string]interface{}),
	}

	// Get current memory usage
	currentMem := c.getCurrentMem()
	health.Metadata["memory_mb"] = currentMem

	// Get current goroutine count
	currentGoro := c.getCurrentGoro()
	health.Metadata["goroutines"] = currentGoro

	// Check memory usage
	if currentMem > c.maxMemoryMB {
		health.Status = StatusDegraded
		health.Message = fmt.Sprintf("memory usage (%d MB) exceeds threshold (%d MB)", currentMem, c.maxMemoryMB)
	}

	// Check goroutine count
	if currentGoro > c.maxGoroutines {
		health.Status = StatusDegraded
		health.Message = fmt.Sprintf("goroutine count (%d) exceeds threshold (%d)", currentGoro, c.maxGoroutines)
	}

	if health.Status == StatusHealthy {
		health.Message = "system resources are within normal limits"
	}

	health.Duration = time.Since(start)
	return health
}

// DummyChecker is a simple health checker that always returns healthy (for testing)
type DummyChecker struct {
	name   string
	status Status
}

// NewDummyChecker creates a new dummy health checker
func NewDummyChecker(name string, status Status) *DummyChecker {
	return &DummyChecker{
		name:   name,
		status: status,
	}
}

// Name returns the component name
func (c *DummyChecker) Name() string {
	return c.name
}

// Check performs the health check
func (c *DummyChecker) Check() ComponentHealth {
	return ComponentHealth{
		Name:      c.name,
		Status:    c.status,
		Timestamp: time.Now(),
		Message:   "dummy check",
	}
}
