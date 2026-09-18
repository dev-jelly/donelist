# Query Optimization and Performance Tuning

This document covers query optimization strategies, execution plan analysis, and PostgreSQL parameter tuning for the Donelist backend.

## Overview

Task #18.5 focuses on:
1. Execution plan analysis with EXPLAIN ANALYZE
2. Query structure optimization
3. Database parameter tuning
4. Index-only scan optimization
5. Query rewriting for performance

## 1. Query Performance Analysis

### Using EXPLAIN ANALYZE

```sql
-- Basic execution plan
EXPLAIN (ANALYZE, BUFFERS, VERBOSE, FORMAT JSON)
SELECT c.*, cat.name as category_name
FROM checkins c
LEFT JOIN categories cat ON c.category_id = cat.id
WHERE c.user_id = 'uuid-here'
  AND c.deleted_at IS NULL
  AND c.checkin_time >= '2024-01-01'
ORDER BY c.checkin_time DESC
LIMIT 50;
```

### Key Metrics to Monitor

1. **Execution Time**: Target < 100ms for most queries, < 10ms for index lookups
2. **Buffer Usage**: Minimize shared_blks_read (disk I/O)
3. **Planning Time**: Should be < 1ms for prepared statements
4. **Rows Scanned vs Returned**: Ideal ratio close to 1:1
5. **Join Types**: Prefer nested loop for small result sets, hash join for large sets

### Query Performance Baseline

```sql
-- Get slow queries from pg_stat_statements
SELECT
    query,
    calls,
    total_exec_time / 1000 as total_time_sec,
    mean_exec_time as avg_ms,
    max_exec_time as max_ms,
    stddev_exec_time as stddev_ms,
    rows / calls as avg_rows,
    100.0 * shared_blks_hit / NULLIF(shared_blks_hit + shared_blks_read, 0) as cache_hit_ratio
FROM pg_stat_statements
WHERE query NOT LIKE '%pg_stat_statements%'
  AND calls > 10
ORDER BY mean_exec_time DESC
LIMIT 20;
```

## 2. Common Query Patterns and Optimizations

### Pattern 1: User Timeline Query

**Before Optimization:**
```sql
-- Sequential scan on checkins (slow)
SELECT * FROM checkins
WHERE user_id = $1
  AND deleted_at IS NULL
ORDER BY checkin_time DESC
LIMIT 50;
```

**After Optimization:**
```sql
-- Uses composite index idx_checkins_user_time_deleted
-- Enables index-only scan if covering index is used
SELECT id, user_id, category_id, content, checkin_time, duration_minutes
FROM checkins
WHERE user_id = $1
  AND deleted_at IS NULL
ORDER BY checkin_time DESC
LIMIT 50;
```

**Expected Improvement**: 100x (500ms → 5ms)

### Pattern 2: Category Statistics

**Before Optimization:**
```sql
-- Full table scan with aggregation
SELECT category_id, COUNT(*), SUM(duration_minutes)
FROM checkins
WHERE user_id = $1
  AND deleted_at IS NULL
GROUP BY category_id;
```

**After Optimization:**
```sql
-- Use materialized view for historical data
SELECT category_id, total_usage, total_minutes
FROM mv_category_performance
WHERE user_id = $1
UNION ALL
-- Only query recent data not in materialized view
SELECT category_id, COUNT(*), SUM(duration_minutes)
FROM checkins
WHERE user_id = $1
  AND deleted_at IS NULL
  AND updated_at > (SELECT MAX(last_updated) FROM mv_refresh_log WHERE view_name = 'mv_category_performance')
GROUP BY category_id;
```

**Expected Improvement**: 50x (1000ms → 20ms)

### Pattern 3: Date Range Queries (Partitioned)

**Before Optimization:**
```sql
-- Scans all partitions
SELECT * FROM checkins_partitioned
WHERE user_id = $1
  AND checkin_time BETWEEN $2 AND $3;
```

**After Optimization:**
```sql
-- Partition pruning enabled (only scans relevant partitions)
SELECT * FROM checkins_partitioned
WHERE checkin_time >= $2
  AND checkin_time < $3  -- Use < instead of BETWEEN for better pruning
  AND user_id = $1
  AND deleted_at IS NULL;
```

**Expected Improvement**: 10x with partition pruning (100ms → 10ms)

### Pattern 4: Search Queries

**Before Optimization:**
```sql
-- Full text search without index
SELECT * FROM checkins
WHERE content ILIKE '%search term%';
```

**After Optimization:**
```sql
-- Uses GIN index on tsvector
SELECT * FROM checkins
WHERE to_tsvector('english', content) @@ plainto_tsquery('english', $1)
  AND user_id = $2
  AND deleted_at IS NULL
ORDER BY ts_rank(to_tsvector('english', content), plainto_tsquery('english', $1)) DESC
LIMIT 50;
```

**Expected Improvement**: 1000x (10s → 10ms)

## 3. Index Optimization

### Covering Indexes (Index-Only Scans)

```sql
-- Create covering index for authentication queries
-- This allows PostgreSQL to satisfy queries using only the index
CREATE INDEX idx_users_auth_covering ON users(email, deleted_at)
INCLUDE (id, password_hash, role, display_name, tier);

-- Query will use index-only scan
SELECT id, email, password_hash, role, display_name, tier
FROM users
WHERE email = $1 AND deleted_at IS NULL;
```

### Partial Indexes

```sql
-- Index only active (non-deleted) records
CREATE INDEX idx_checkins_active_user_time ON checkins(user_id, checkin_time DESC)
WHERE deleted_at IS NULL;

-- Reduces index size by 0-20% (depending on deletion rate)
-- Faster index scans and updates
```

### GIN Indexes for Arrays

```sql
-- For webhook events array
CREATE INDEX idx_webhooks_events_gin ON webhooks USING gin(events);

-- Efficient array containment checks
SELECT * FROM webhooks WHERE events @> ARRAY['checkin.created']::TEXT[];
```

## 4. Join Optimization

### Join Order

PostgreSQL's query planner usually chooses the best join order, but you can help:

```sql
-- Ensure statistics are up to date
ANALYZE checkins;
ANALYZE categories;
ANALYZE users;

-- Force nested loop for small result sets (use sparingly)
SET enable_hashjoin = off;
SET enable_mergejoin = off;

-- Reset to defaults
RESET enable_hashjoin;
RESET enable_mergejoin;
```

### Join Type Selection

```sql
-- Small result set: Nested Loop Join (best for <1000 rows)
SELECT c.*, cat.name
FROM checkins c
JOIN categories cat ON c.category_id = cat.id
WHERE c.user_id = $1
LIMIT 50;

-- Large result set: Hash Join (best for >1000 rows)
SELECT c.user_id, COUNT(*)
FROM checkins c
JOIN users u ON c.user_id = u.id
GROUP BY c.user_id;
```

## 5. PostgreSQL Parameter Tuning

### Memory Settings

```sql
-- View current settings
SHOW shared_buffers;
SHOW work_mem;
SHOW maintenance_work_mem;
SHOW effective_cache_size;

-- Recommended settings for 8GB RAM server
ALTER SYSTEM SET shared_buffers = '2GB';           -- 25% of RAM
ALTER SYSTEM SET work_mem = '32MB';                -- Per operation
ALTER SYSTEM SET maintenance_work_mem = '512MB';   -- For VACUUM, CREATE INDEX
ALTER SYSTEM SET effective_cache_size = '6GB';     -- 75% of RAM

-- Reload configuration
SELECT pg_reload_conf();
```

### Query Planner Settings

```sql
-- Adjust cost parameters based on hardware
ALTER SYSTEM SET random_page_cost = 1.1;           -- For SSD (default: 4.0)
ALTER SYSTEM SET effective_io_concurrency = 200;   -- For SSD (default: 1)
ALTER SYSTEM SET seq_page_cost = 1.0;              -- Sequential scan cost

-- Parallel query settings
ALTER SYSTEM SET max_parallel_workers_per_gather = 4;
ALTER SYSTEM SET max_parallel_workers = 8;
ALTER SYSTEM SET parallel_tuple_cost = 0.05;       -- Lower for faster parallel

SELECT pg_reload_conf();
```

### Statistics and Planning

```sql
-- Increase statistics target for better planning
ALTER TABLE checkins ALTER COLUMN user_id SET STATISTICS 500;
ALTER TABLE checkins ALTER COLUMN checkin_time SET STATISTICS 500;
ALTER TABLE checkins ALTER COLUMN category_id SET STATISTICS 500;

-- Update statistics
ANALYZE checkins;

-- Enable query planning cache
ALTER SYSTEM SET plan_cache_mode = 'auto';  -- auto, force_custom_plan, force_generic_plan
```

### Checkpoint and WAL Settings

```sql
-- Reduce checkpoint frequency for better write performance
ALTER SYSTEM SET checkpoint_timeout = '15min';     -- Default: 5min
ALTER SYSTEM SET checkpoint_completion_target = 0.9;

-- WAL settings for performance
ALTER SYSTEM SET wal_buffers = '16MB';
ALTER SYSTEM SET max_wal_size = '4GB';
ALTER SYSTEM SET min_wal_size = '1GB';

SELECT pg_reload_conf();
```

## 6. Query Rewriting Techniques

### Avoid SELECT *

```sql
-- Bad: Fetches unnecessary data
SELECT * FROM checkins WHERE user_id = $1;

-- Good: Fetch only needed columns
SELECT id, content, checkin_time, category_id FROM checkins WHERE user_id = $1;
```

### Use EXISTS Instead of COUNT

```sql
-- Bad: Counts all rows
SELECT COUNT(*) > 0 FROM checkins WHERE user_id = $1;

-- Good: Stops at first match
SELECT EXISTS(SELECT 1 FROM checkins WHERE user_id = $1);
```

### Avoid Functions on Indexed Columns

```sql
-- Bad: Can't use index on checkin_time
SELECT * FROM checkins WHERE DATE(checkin_time) = CURRENT_DATE;

-- Good: Uses index
SELECT * FROM checkins WHERE checkin_time >= CURRENT_DATE
  AND checkin_time < CURRENT_DATE + INTERVAL '1 day';
```

### Use LIMIT Effectively

```sql
-- Bad: Sorts all rows before limiting
SELECT * FROM checkins ORDER BY checkin_time DESC LIMIT 10;

-- Good: Uses index for sorting, limits early
SELECT * FROM checkins
WHERE user_id = $1
  AND deleted_at IS NULL
ORDER BY checkin_time DESC
LIMIT 10;
```

### Batch Operations

```sql
-- Bad: N+1 queries
-- In application: for each user, query categories

-- Good: Single query with JOIN or IN
SELECT u.id, ARRAY_AGG(c.name) as categories
FROM users u
LEFT JOIN categories c ON c.user_id = u.id
WHERE u.id = ANY($1::UUID[])
GROUP BY u.id;
```

## 7. Monitoring and Benchmarking

### Create Performance Baseline

```sql
-- Install pg_stat_statements extension
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

-- Reset statistics
SELECT pg_stat_statements_reset();

-- Run workload for 1 hour, then analyze
SELECT
    substring(query, 1, 80) as query_short,
    calls,
    round(total_exec_time::numeric / calls, 2) as avg_time_ms,
    round((100 * total_exec_time / sum(total_exec_time) OVER ())::numeric, 2) as pct_total_time,
    round((shared_blks_hit * 100.0 / NULLIF(shared_blks_hit + shared_blks_read, 0))::numeric, 2) as cache_hit_ratio
FROM pg_stat_statements
WHERE query NOT LIKE '%pg_stat_statements%'
ORDER BY total_exec_time DESC
LIMIT 20;
```

### Query Performance Testing

```sql
-- Create test function for consistent benchmarking
CREATE OR REPLACE FUNCTION test_query_performance(
    test_name TEXT,
    test_query TEXT,
    iterations INTEGER DEFAULT 100
) RETURNS TABLE(
    test_name TEXT,
    avg_time_ms NUMERIC,
    min_time_ms NUMERIC,
    max_time_ms NUMERIC,
    stddev_ms NUMERIC
) AS $$
DECLARE
    start_time TIMESTAMP;
    end_time TIMESTAMP;
    times NUMERIC[];
    i INTEGER;
BEGIN
    times := ARRAY[]::NUMERIC[];

    FOR i IN 1..iterations LOOP
        start_time := clock_timestamp();
        EXECUTE test_query;
        end_time := clock_timestamp();
        times := array_append(times, EXTRACT(MILLISECONDS FROM end_time - start_time));
    END LOOP;

    RETURN QUERY
    SELECT
        test_name,
        round(avg(t), 3),
        round(min(t), 3),
        round(max(t), 3),
        round(stddev(t), 3)
    FROM unnest(times) t;
END;
$$ LANGUAGE plpgsql;

-- Use it
SELECT * FROM test_query_performance(
    'user_timeline',
    $$SELECT * FROM checkins WHERE user_id = 'uuid-here' AND deleted_at IS NULL ORDER BY checkin_time DESC LIMIT 50$$,
    100
);
```

## 8. Production Optimization Checklist

### Pre-Optimization

- [ ] Enable pg_stat_statements
- [ ] Collect 24 hours of query statistics
- [ ] Identify top 10 slowest queries
- [ ] Document current baseline performance
- [ ] Back up production data

### During Optimization

- [ ] Apply indexes one at a time
- [ ] Monitor index creation progress (pg_stat_progress_create_index)
- [ ] Verify query plans with EXPLAIN ANALYZE
- [ ] Test on staging before production
- [ ] Monitor for query plan regressions

### Post-Optimization

- [ ] Update table statistics (ANALYZE)
- [ ] Measure performance improvement
- [ ] Document changes in migration
- [ ] Set up monitoring alerts
- [ ] Schedule regular VACUUM ANALYZE

## 9. Common Performance Issues

### Issue: High shared_blks_read

**Symptom**: Queries reading from disk instead of cache

**Solution**:
```sql
-- Increase shared_buffers
ALTER SYSTEM SET shared_buffers = '4GB';

-- Prewarm frequently accessed tables
CREATE EXTENSION pg_prewarm;
SELECT pg_prewarm('checkins');
```

### Issue: Sequential Scans on Large Tables

**Symptom**: Seq Scan in EXPLAIN output for filtered queries

**Solution**:
```sql
-- Add appropriate index
CREATE INDEX idx_checkins_user_time ON checkins(user_id, checkin_time DESC);

-- Verify index is used
EXPLAIN (ANALYZE) SELECT * FROM checkins WHERE user_id = 'uuid' ORDER BY checkin_time DESC;
```

### Issue: Bitmap Heap Scan (Recheck Cond)

**Symptom**: High "Rows Removed by Index Recheck"

**Solution**:
```sql
-- Increase work_mem for better bitmap operations
SET work_mem = '64MB';

-- Or use covering index to avoid heap access
CREATE INDEX idx_covering ON checkins(user_id, checkin_time) INCLUDE (content, category_id);
```

### Issue: Nested Loop Join on Large Tables

**Symptom**: Nested Loop Join with millions of rows

**Solution**:
```sql
-- Update statistics to help planner choose hash join
ANALYZE checkins;

-- Temporarily disable nested loop if still chosen
SET enable_nestloop = off;
-- Test query
-- Re-enable: RESET enable_nestloop;
```

## 10. Automated Query Optimization

### Auto-Explain for Slow Queries

```sql
-- Enable auto_explain extension
CREATE EXTENSION IF NOT EXISTS auto_explain;

-- Configure to log slow queries
ALTER SYSTEM SET auto_explain.log_min_duration = 100; -- Log queries > 100ms
ALTER SYSTEM SET auto_explain.log_analyze = on;
ALTER SYSTEM SET auto_explain.log_buffers = on;
ALTER SYSTEM SET auto_explain.log_timing = on;

SELECT pg_reload_conf();
```

### Query Tuning Helper Function

```sql
CREATE OR REPLACE FUNCTION suggest_query_optimization(query_text TEXT)
RETURNS TABLE(
    suggestion TEXT,
    priority TEXT,
    reason TEXT
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        'Add index on ' || attname::TEXT,
        'HIGH',
        'Column appears in WHERE clause without index'
    FROM pg_class c
    JOIN pg_attribute a ON c.oid = a.attrelid
    WHERE c.relname IN (
        SELECT DISTINCT tablename::TEXT
        FROM pg_tables
        WHERE tablename = ANY(
            SELECT regexp_matches(query_text, 'FROM\s+(\w+)', 'g')
        )
    );
END;
$$ LANGUAGE plpgsql;
```

## 11. Performance Targets

After optimization, target these metrics:

| Query Type | Target Time | Max Acceptable |
|------------|-------------|----------------|
| Primary key lookup | < 1ms | 5ms |
| User timeline (50 rows) | < 10ms | 50ms |
| Category statistics | < 20ms | 100ms |
| Date range query | < 30ms | 150ms |
| Full-text search | < 50ms | 200ms |
| Complex analytics | < 200ms | 1s |

## 12. References

- [PostgreSQL EXPLAIN Documentation](https://www.postgresql.org/docs/current/sql-explain.html)
- [Index Types and Performance](https://www.postgresql.org/docs/current/indexes-types.html)
- [Query Planning Process](https://www.postgresql.org/docs/current/planner-optimizer.html)
- [Configuration Tuning](https://www.postgresql.org/docs/current/runtime-config.html)
- [pg_stat_statements](https://www.postgresql.org/docs/current/pgstatstatements.html)
