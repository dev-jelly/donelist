# Database Performance Optimization

This document describes the database performance optimizations implemented in the Donelist backend.

## Overview

The database optimization includes:
1. Connection pool tuning
2. Redis query result caching
3. Smart cache invalidation strategies
4. Query performance monitoring
5. Prepared statement management
6. Performance benchmarking

## 1. Connection Pool Optimization

### Configuration

The connection pool is now fully configurable via environment variables:

```env
# Database Connection Pool Settings
DB_MAX_OPEN_CONNS=100              # Maximum number of open connections
DB_MAX_IDLE_CONNS=25               # Maximum number of idle connections
DB_CONN_MAX_LIFETIME=30m           # Maximum connection lifetime
DB_CONN_MAX_IDLE_TIME=10m          # Maximum idle time for connections
```

### Default Values

The system uses production-optimized defaults:
- **MaxOpenConns**: 100 (allows high concurrency)
- **MaxIdleConns**: 25 (maintains warm connections)
- **ConnMaxLifetime**: 30 minutes (prevents stale connections)
- **ConnMaxIdleTime**: 10 minutes (balances reuse and cleanup)

### Monitoring

Connection pool stats are logged every 5 minutes in production and available via health check endpoints.

## 2. Redis Caching Layer

### Cache Package

Location: `/pkg/cache/`

The caching layer provides:
- Automatic serialization/deserialization
- TTL-based expiration
- Pattern-based key management
- Cache hit/miss metrics

### Cache TTL Presets

Different data types have optimized TTL values:

```go
TTLUserData     = 5 minutes   // User data changes moderately
TTLCheckins     = 30 minutes  // Checkins are relatively stable
TTLCategories   = 1 hour      // Categories change infrequently
TTLTags         = 1 hour      // Tags change infrequently
TTLStatistics   = 15 minutes  // Statistics recalculated periodically
TTLSearchResults = 10 minutes // Search results can be cached briefly
TTLShortLived   = 1 minute    // For very dynamic data
```

### Usage Example

```go
// Initialize cache
cache := cache.NewCache(redisClient, cache.Config{
    Prefix:     "donelist:",
    DefaultTTL: 5 * time.Minute,
}, logger)

// Get or set pattern (recommended)
var user User
err := cache.GetOrSet(ctx, cache.MakeUserKey(userID), cache.TTLUserData, &user, func() (interface{}, error) {
    return userRepo.GetByID(ctx, userID)
})

// Direct operations
cache.Set(ctx, key, value, ttl)
cache.Get(ctx, key, &dest)
cache.Delete(ctx, keys...)
cache.DeletePattern(ctx, "user:*")
```

## 3. Cache Invalidation Strategy

### Invalidator Package

Location: `/pkg/cache/invalidation.go`

The invalidation strategy ensures cache consistency by invalidating related data when mutations occur.

### Invalidation Patterns

#### On Checkin Create/Update/Delete
```go
invalidator.InvalidateOnCheckinCreate(ctx, userID, date)
```
Invalidates:
- Checkin lists for the date
- Timeline for the date
- All statistics (dependent data)
- Calendar views
- Search results

#### On Category Change
```go
invalidator.InvalidateCategories(ctx, userID)
```
Invalidates:
- Category lists
- Timeline (shows category names)
- Statistics (grouped by category)

#### On Tag Change
```go
invalidator.InvalidateTags(ctx, userID)
```
Invalidates:
- Tag lists
- Timeline (shows tags)
- Search results (tag-based)

### Complete User Invalidation

For operations like user deletion or major data changes:

```go
invalidator.InvalidateUser(ctx, userID)
```

This invalidates all user-related cache entries.

## 4. Query Performance Monitoring

### Performance Monitor

Location: `/pkg/database/performance.go`

Tracks query execution metrics:
- Execution count
- Average/min/max duration
- Error count
- Last execution time

### Configuration

```go
perfMonitor := database.NewPerformanceMonitor(db, database.PerformanceConfig{
    SlowQueryThreshold: 100 * time.Millisecond,
    Enabled:            true,
}, logger)
```

### Usage

```go
// Wrap queries with tracking
err := perfMonitor.TrackQuery(ctx, "SELECT * FROM users WHERE id = $1", func() error {
    return db.QueryContext(ctx, query, userID)
})
```

### Slow Query Detection

Queries exceeding the threshold are automatically logged:

```
WARN: Slow query detected
  query: SELECT * FROM users ORDER BY created_at DESC
  duration: 150ms
  threshold: 100ms
```

### Monitoring Endpoints

Available at `/admin/performance/*`:
- GET `/metrics` - All query metrics
- GET `/slow-queries` - Queries exceeding threshold
- GET `/pool-stats` - Connection pool statistics
- POST `/reset` - Clear metrics
- POST `/enable` - Enable monitoring
- POST `/disable` - Disable monitoring

## 5. Prepared Statements

### Statement Manager

Location: `/pkg/database/performance.go`

Manages prepared statements for frequently used queries:

```go
psm := database.NewPreparedStatementManager(db, logger)

// Prepare common statements
database.PrepareCommonStatements(psm)

// Use prepared statements
stmt, err := psm.Get(database.StmtGetUserByID)
```

### Benefits

- Reduced query parsing overhead
- Protection against SQL injection
- Better query plan caching
- Improved performance for repeated queries

## 6. Performance Benchmarks

### Running Benchmarks

```bash
go test ./pkg/database/... -bench=. -benchmem
```

### Benchmark Results

Current performance metrics (Apple M1 Max):

```
BenchmarkConnectionPooling/QueryExecution-10          1,415,083    828 ns/op    448 B/op    10 allocs/op
BenchmarkCacheOperations/CacheKeyGeneration-10       22,715,458     58 ns/op      8 B/op     1 allocs/op
BenchmarkCacheOperations/MultipleKeyGeneration-10     4,821,072    266 ns/op     72 B/op     5 allocs/op
BenchmarkPerformanceMonitoring/WithMonitoring-10      8,109,559    148 ns/op      0 B/op     0 allocs/op
BenchmarkPerformanceMonitoring/WithoutMonitoring-10  83,488,668     14 ns/op      0 B/op     0 allocs/op
BenchmarkPreparedStatements/PreparedStatementManager  84,078,260     15 ns/op      0 B/op     0 allocs/op
```

### Performance Overhead

- Cache key generation: ~60ns (negligible)
- Performance monitoring (when enabled): ~148ns overhead per query
- Performance monitoring (when disabled): ~14ns overhead (minimal)
- Prepared statement lookup: ~15ns (very fast)

## 7. Best Practices

### When to Use Cache

**Good candidates:**
- User profiles (change infrequently)
- Categories and tags (stable data)
- Historical checkins (immutable)
- Aggregated statistics (expensive to calculate)
- Search results (can tolerate slight staleness)

**Poor candidates:**
- Real-time data (WebSocket messages)
- Write-heavy data (frequently updated)
- User-specific sensitive data (consider security)

### Cache Key Naming

Use the provided key generators for consistency:

```go
cache.MakeUserKey(userID)                          // "user:123"
cache.MakeCheckinsKey(userID, date)                // "checkins:123:2024-01-15"
cache.MakeCategoriesKey(userID)                    // "categories:123"
cache.MakeStatisticsKey(userID, start, end)        // "stats:123:2024-01-01:2024-01-31"
```

### Invalidation Strategy

**Eager invalidation (recommended):**
- Invalidate immediately after writes
- Ensures cache consistency
- Slight write overhead

**Lazy invalidation:**
- Let TTL expire naturally
- Better write performance
- May serve stale data

For critical data (user profiles, balances), use eager invalidation.
For analytics and aggregates, lazy invalidation is acceptable.

### Connection Pool Tuning

Adjust based on workload:

**High traffic (many concurrent users):**
```env
DB_MAX_OPEN_CONNS=200
DB_MAX_IDLE_CONNS=50
```

**Low traffic (development):**
```env
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=5
```

**Monitor:**
- If `wait_count` is high, increase `MAX_OPEN_CONNS`
- If `max_idle_closed` is high, increase `MAX_IDLE_CONNS`
- If `max_lifetime_closed` is high, increase `CONN_MAX_LIFETIME`

## 8. Monitoring and Alerting

### Health Check Endpoints

Connection pool stats available at:
- `/health/detail` - Includes DB connection details
- `/ready` - Checks DB and Redis connectivity
- `/metrics` - Prometheus metrics including pool stats

### Metrics to Monitor

1. **Cache hit rate**: `hits / (hits + misses)` - Target: >80%
2. **Slow query count**: Target: <5% of total queries
3. **Pool wait count**: Target: minimal growth
4. **Pool utilization**: `in_use / max_open` - Target: 50-70%
5. **Connection wait time**: Target: <10ms

### Setting Up Alerts

Example Prometheus alerts:

```yaml
- alert: HighCacheMissRate
  expr: cache_miss_rate > 0.5
  annotations:
    summary: Cache miss rate above 50%

- alert: DatabasePoolExhaustion
  expr: db_pool_wait_count_rate > 10
  annotations:
    summary: Database connection pool exhaustion

- alert: SlowQueries
  expr: slow_query_count > 100
  annotations:
    summary: High number of slow queries detected
```

## 9. Testing

### Unit Tests

```bash
# Cache tests
go test ./pkg/cache/... -v

# Performance monitor tests
go test ./pkg/database/... -v -run TestPerformance

# Invalidation tests
go test ./pkg/cache/... -v -run TestInvalidator
```

### Integration Tests

Cache and database integration is tested with real PostgreSQL and Redis instances using testcontainers.

### Load Testing

Use the provided benchmarks to establish baseline performance:

```bash
# Run benchmarks and save results
go test ./pkg/database/... -bench=. -benchmem > baseline.txt

# After optimization
go test ./pkg/database/... -bench=. -benchmem > optimized.txt

# Compare
benchstat baseline.txt optimized.txt
```

## 10. Future Improvements

Potential optimizations to consider:

1. **Read Replicas**: Route read queries to replicas
2. **Query Result Pagination**: Cursor-based pagination for large datasets
3. **Materialized Views**: Pre-compute expensive aggregations
4. **Partial Indexes**: Create indexes on filtered subsets
5. **Connection Multiplexing**: Use pgbouncer for connection pooling
6. **Redis Cluster**: Scale Redis horizontally for larger caches
7. **Cache Warming**: Pre-populate cache on startup
8. **Adaptive TTL**: Adjust TTL based on data volatility

## References

- [PostgreSQL Connection Pool Best Practices](https://wiki.postgresql.org/wiki/Number_Of_Database_Connections)
- [Redis Caching Strategies](https://redis.io/docs/manual/patterns/cache-aside/)
- [Go database/sql Package](https://pkg.go.dev/database/sql)
- [sqlx Documentation](https://jmoiron.github.io/sqlx/)
