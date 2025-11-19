# Composite Index Deployment and Rollback Guide

**Task 18.3** - 복합 인덱스 설계·적용·롤백 전략 구현

## Overview

This guide provides a complete strategy for deploying composite indexes based on workload analysis, testing their effectiveness, and rolling back if needed.

## Quick Start

### Deploy All Indexes

```bash
cd server
# Apply migration
migrate -path migrations -database "postgresql://localhost/donelist" up 1

# Or using make (if configured)
make migrate-up
```

### Check Index Status

```bash
psql $DATABASE_URL -c "SELECT * FROM performance.new_index_usage ORDER BY scans_count DESC;"
```

### Rollback If Needed

```bash
migrate -path migrations -database "postgresql://localhost/donelist" down 1
```

## Pre-Deployment Checklist

Before deploying composite indexes in production:

- [ ] Performance monitoring is enabled (Task 18.1 completed)
- [ ] Workload analysis has been run (Task 18.2 completed)
- [ ] At least 7 days of baseline metrics collected
- [ ] Database backup completed
- [ ] Maintenance window scheduled (optional, indexes created CONCURRENTLY)
- [ ] Monitoring dashboards ready to track impact
- [ ] Rollback plan tested in staging environment

## Deployment Strategy

### Phase 1: Critical Indexes (Priority 1)

Deploy these indexes first as they have the highest impact:

```sql
-- 1. User timeline queries (highest traffic)
CREATE INDEX CONCURRENTLY idx_checkins_user_time_deleted
ON checkins(user_id, checkin_time DESC, deleted_at)
WHERE deleted_at IS NULL;

-- 2. Full-text search
CREATE INDEX CONCURRENTLY idx_checkins_content_fts
ON checkins USING gin(to_tsvector('english', content));

-- 3. Sync queue processing
CREATE INDEX CONCURRENTLY idx_sync_queue_user_status
ON sync_queue(user_id, status, created_at)
WHERE status = 'pending';
```

**Expected Duration:** 5-30 minutes depending on table size

**Monitoring During Creation:**

```sql
-- Check progress
SELECT
    now()::time,
    query,
    state,
    wait_event_type,
    wait_event
FROM pg_stat_activity
WHERE query LIKE '%CREATE INDEX%'
    AND state != 'idle';
```

### Phase 2: High Priority Indexes (Priority 2)

Deploy after Phase 1 is verified:

```sql
-- 4. Category-based filtering
CREATE INDEX CONCURRENTLY idx_checkins_category_time
ON checkins(category_id, checkin_time DESC)
WHERE category_id IS NOT NULL AND deleted_at IS NULL;

-- 5. Analytics aggregation
CREATE INDEX CONCURRENTLY idx_analytics_user_type_time
ON analytics_events(user_id, event_type, created_at DESC);
```

**Expected Duration:** 2-15 minutes

### Phase 3: Medium/Low Priority Indexes

Deploy based on specific use cases:

```sql
-- Search history, audit logs, webhooks, etc.
-- See migration file for complete list
```

## Testing Index Effectiveness

### 1. Capture Baseline Metrics

**Before deploying indexes:**

```sql
-- Create baseline snapshot
INSERT INTO performance.query_stats_snapshot
SELECT
    NOW(),
    md5(query),
    query,
    calls,
    total_exec_time,
    mean_exec_time,
    min_exec_time,
    max_exec_time,
    stddev_exec_time,
    rows,
    shared_blks_hit,
    shared_blks_read,
    shared_blks_written,
    temp_blks_read,
    temp_blks_written
FROM pg_stat_statements
WHERE query ILIKE '%checkins%'
   OR query ILIKE '%sync_queue%'
   OR query ILIKE '%analytics_events%'
ORDER BY mean_exec_time DESC
LIMIT 50;
```

### 2. Deploy Indexes

Run the migration as described above.

### 3. Verify Index Creation

```sql
-- Check that all indexes were created
SELECT
    schemaname,
    tablename,
    indexname,
    pg_size_pretty(pg_relation_size(indexrelid)) as size
FROM pg_stat_user_indexes
WHERE indexrelname LIKE 'idx_checkins_%'
   OR indexrelname LIKE 'idx_sync_queue_%'
   OR indexrelname LIKE 'idx_analytics_%';
```

### 4. Run ANALYZE

Force PostgreSQL to update statistics:

```sql
ANALYZE checkins;
ANALYZE sync_queue;
ANALYZE analytics_events;
ANALYZE search_history;
ANALYZE audit_logs;
ANALYZE webhook_deliveries;
ANALYZE team_members;
```

### 5. Test Query Performance

#### User Timeline Query Test

```sql
-- Enable timing
\timing on

-- Test query (use real user_id from your database)
EXPLAIN (ANALYZE, BUFFERS)
SELECT *
FROM checkins
WHERE user_id = 'your-user-uuid-here'
  AND checkin_time >= NOW() - INTERVAL '30 days'
  AND deleted_at IS NULL
ORDER BY checkin_time DESC
LIMIT 50;
```

**Expected Results:**
- Should show `Index Scan using idx_checkins_user_time_deleted`
- Execution time should be < 50ms for typical datasets
- Buffers hit should be minimal (< 100)

#### Category Filter Query Test

```sql
EXPLAIN (ANALYZE, BUFFERS)
SELECT *
FROM checkins
WHERE user_id = 'your-user-uuid-here'
  AND category_id = 'your-category-uuid-here'
  AND deleted_at IS NULL
ORDER BY checkin_time DESC;
```

**Expected Results:**
- Should use `idx_checkins_category_time` or `idx_checkins_user_time_deleted`
- Execution time < 30ms

#### Full-Text Search Test

```sql
EXPLAIN (ANALYZE, BUFFERS)
SELECT *
FROM checkins
WHERE to_tsvector('english', content) @@ plainto_tsquery('english', 'meeting')
  AND deleted_at IS NULL
LIMIT 20;
```

**Expected Results:**
- Should use `idx_checkins_content_fts` (Bitmap Index Scan)
- Execution time varies with result set, but should be < 100ms

#### Sync Queue Test

```sql
EXPLAIN (ANALYZE, BUFFERS)
SELECT *
FROM sync_queue
WHERE user_id = 'your-user-uuid-here'
  AND status = 'pending'
ORDER BY created_at ASC
LIMIT 100;
```

**Expected Results:**
- Should use `idx_sync_queue_user_status`
- Execution time < 20ms

### 6. Monitor Index Usage

Wait 24-48 hours after deployment, then check:

```sql
-- Check index usage statistics
SELECT * FROM performance.new_index_usage
ORDER BY scans_count DESC;

-- Analyze effectiveness
SELECT * FROM performance.analyze_index_effectiveness();
```

### 7. Compare Performance Metrics

```sql
-- Compare query performance before/after
WITH before AS (
    SELECT
        query_hash,
        AVG(mean_exec_time_ms) as avg_time_before
    FROM performance.query_stats_snapshot
    WHERE snapshot_time < (SELECT created_at FROM performance.index_deployment_log WHERE index_name = 'idx_checkins_user_time_deleted' LIMIT 1)
    GROUP BY query_hash
),
after AS (
    SELECT
        query_hash,
        AVG(mean_exec_time_ms) as avg_time_after
    FROM performance.query_stats_snapshot
    WHERE snapshot_time >= (SELECT created_at FROM performance.index_deployment_log WHERE index_name = 'idx_checkins_user_time_deleted' LIMIT 1)
    GROUP BY query_hash
)
SELECT
    b.query_hash,
    ROUND(b.avg_time_before, 2) as before_ms,
    ROUND(a.avg_time_after, 2) as after_ms,
    ROUND(((b.avg_time_before - a.avg_time_after) / b.avg_time_before * 100), 2) as improvement_pct
FROM before b
JOIN after a ON b.query_hash = a.query_hash
WHERE b.avg_time_before > 10  -- Only queries that took > 10ms
ORDER BY improvement_pct DESC
LIMIT 20;
```

## Rollback Procedures

### When to Rollback

Consider rolling back if:

1. **Index Not Used:** Index has 0 scans after 48 hours
2. **Performance Degradation:** Query performance worse than baseline
3. **Write Performance Impact:** INSERT/UPDATE operations significantly slower
4. **Disk Space Issues:** Index size exceeds expectations
5. **Lock Contention:** Experiencing unexpected locking issues

### Rollback Steps

#### Quick Rollback (Entire Migration)

```bash
migrate -path migrations -database "postgresql://localhost/donelist" down 1
```

#### Selective Index Removal

If only specific indexes are problematic:

```sql
-- Drop specific problematic index
DROP INDEX CONCURRENTLY idx_checkins_content_fts;

-- Update deployment log
UPDATE performance.index_deployment_log
SET status = 'rolled_back',
    notes = 'Removed due to [reason]'
WHERE index_name = 'idx_checkins_content_fts';
```

#### Restore Original Indexes

If you dropped redundant indexes during optimization:

```sql
-- Check what was dropped
SELECT * FROM performance.index_deployment_log
WHERE notes LIKE '%redundant%';

-- Recreate if needed
CREATE INDEX CONCURRENTLY idx_original_name ON table_name(column);
```

### Post-Rollback Actions

1. **Vacuum and Analyze:**
   ```sql
   VACUUM ANALYZE checkins;
   VACUUM ANALYZE sync_queue;
   ```

2. **Monitor Performance:**
   ```sql
   -- Check if performance returned to baseline
   SELECT * FROM performance.current_db_stats;
   ```

3. **Document Reason:**
   ```sql
   UPDATE performance.index_deployment_log
   SET notes = 'Rolled back because: [detailed reason]'
   WHERE status = 'rolled_back';
   ```

## Performance Benchmarks

### Expected Impact by Index

| Index | Target Query | Before | After | Improvement |
|-------|-------------|--------|-------|-------------|
| `idx_checkins_user_time_deleted` | User timeline (30 days) | 245ms | 12ms | 95% |
| `idx_checkins_content_fts` | Full-text search | 1200ms | 45ms | 96% |
| `idx_sync_queue_user_status` | Sync queue fetch | 180ms | 8ms | 96% |
| `idx_checkins_category_time` | Category filter | 150ms | 25ms | 83% |
| `idx_analytics_user_type_time` | Analytics query | 320ms | 65ms | 80% |

*Note: Actual results vary based on data volume and distribution*

### Index Size Estimates

| Table | Current Rows | Index | Estimated Size |
|-------|-------------|-------|----------------|
| checkins | 100,000 | `idx_checkins_user_time_deleted` | ~15 MB |
| checkins | 100,000 | `idx_checkins_content_fts` | ~25 MB |
| sync_queue | 10,000 | `idx_sync_queue_user_status` | ~2 MB |
| analytics_events | 500,000 | `idx_analytics_user_type_time` | ~40 MB |

## Monitoring and Maintenance

### Daily Checks

```sql
-- Quick health check
SELECT
    indexrelname,
    idx_scan,
    idx_tup_read,
    idx_tup_fetch
FROM pg_stat_user_indexes
WHERE indexrelname LIKE 'idx_checkins_%'
   OR indexrelname LIKE 'idx_sync_%'
ORDER BY idx_scan DESC;
```

### Weekly Review

```bash
# Generate comprehensive report
psql $DATABASE_URL << EOF
SELECT * FROM performance.analyze_index_effectiveness();
SELECT * FROM performance.new_index_usage;
EOF
```

### Monthly Maintenance

```sql
-- Check for index bloat
SELECT
    schemaname,
    tablename,
    indexrelname,
    pg_size_pretty(pg_relation_size(indexrelid)) as size,
    idx_scan
FROM pg_stat_user_indexes
WHERE pg_relation_size(indexrelid) > 104857600  -- > 100 MB
ORDER BY pg_relation_size(indexrelid) DESC;

-- Reindex if bloated
REINDEX INDEX CONCURRENTLY idx_name;
```

## Troubleshooting

### Problem: Index Not Being Used

**Symptoms:**
- EXPLAIN shows Seq Scan instead of Index Scan
- `idx_scan` remains at 0

**Solutions:**

1. Update statistics:
   ```sql
   ANALYZE table_name;
   ```

2. Check index definition matches query pattern:
   ```sql
   \d+ table_name
   ```

3. Force index scan for testing:
   ```sql
   SET enable_seqscan = OFF;
   EXPLAIN SELECT ...;
   SET enable_seqscan = ON;
   ```

4. Check planner costs:
   ```sql
   SHOW random_page_cost;
   SHOW seq_page_cost;
   -- For SSD, try:
   SET random_page_cost = 1.1;
   ```

### Problem: Write Performance Degraded

**Symptoms:**
- INSERT/UPDATE operations slower
- Increased lock wait time

**Solutions:**

1. Check index size vs benefit:
   ```sql
   SELECT * FROM performance.analyze_index_effectiveness()
   WHERE effectiveness IN ('POOR', 'MODERATE');
   ```

2. Consider partial indexes for rarely updated data:
   ```sql
   -- Already using partial indexes like:
   -- WHERE deleted_at IS NULL
   -- WHERE status = 'pending'
   ```

3. Monitor write vs read ratio:
   ```sql
   SELECT
       relname,
       n_tup_ins as inserts,
       n_tup_upd as updates,
       idx_scan as index_scans,
       ROUND(idx_scan::numeric / NULLIF(n_tup_ins + n_tup_upd, 0), 2) as read_write_ratio
   FROM pg_stat_user_tables
   WHERE relname IN ('checkins', 'sync_queue', 'analytics_events');
   ```

### Problem: CONCURRENT Index Creation Failed

**Symptoms:**
- `CREATE INDEX CONCURRENTLY` returned error
- Index marked as INVALID

**Solutions:**

1. Check for invalid indexes:
   ```sql
   SELECT
       schemaname,
       tablename,
       indexname
   FROM pg_indexes
   WHERE schemaname = 'public'
     AND indexdef LIKE '%INVALID%';
   ```

2. Drop and recreate:
   ```sql
   DROP INDEX CONCURRENTLY idx_name;
   CREATE INDEX CONCURRENTLY idx_name ON table(columns);
   ```

3. Check for blocking transactions:
   ```sql
   SELECT
       pid,
       state,
       query,
       wait_event_type,
       wait_event
   FROM pg_stat_activity
   WHERE state != 'idle'
     AND query NOT LIKE '%pg_stat_activity%';
   ```

## Integration with CI/CD

### Pre-Production Testing

```yaml
# .github/workflows/test-indexes.yml
name: Test Composite Indexes

on:
  pull_request:
    paths:
      - 'server/migrations/*composite_indexes*'

jobs:
  test-indexes:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:15
        env:
          POSTGRES_DB: donelist_test
          POSTGRES_PASSWORD: postgres
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5

    steps:
      - uses: actions/checkout@v3

      - name: Run migrations
        run: |
          migrate -path server/migrations -database "postgresql://postgres:postgres@localhost:5432/donelist_test?sslmode=disable" up

      - name: Verify indexes created
        run: |
          psql "postgresql://postgres:postgres@localhost:5432/donelist_test?sslmode=disable" -c "
            SELECT COUNT(*) FROM pg_indexes
            WHERE indexname LIKE 'idx_checkins_%'
               OR indexname LIKE 'idx_sync_%'
               OR indexname LIKE 'idx_analytics_%';
          "

      - name: Test rollback
        run: |
          migrate -path server/migrations -database "postgresql://postgres:postgres@localhost:5432/donelist_test?sslmode=disable" down 1
```

### Production Deployment Script

```bash
#!/bin/bash
# scripts/deploy-indexes.sh

set -e

echo "=== Composite Index Deployment Script ==="
echo "Task 18.3 - Deploying composite indexes"

# Check prerequisites
echo "Checking prerequisites..."
psql $DATABASE_URL -c "SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements';" | grep -q 1 || {
    echo "ERROR: pg_stat_statements not enabled"
    exit 1
}

# Backup current index definitions
echo "Backing up current indexes..."
psql $DATABASE_URL -c "\d+" > "backups/indexes-backup-$(date +%Y%m%d-%H%M%S).sql"

# Capture pre-deployment metrics
echo "Capturing baseline metrics..."
psql $DATABASE_URL -c "SELECT performance.capture_query_stats_snapshot();"

# Apply migration
echo "Applying migration 000018_composite_indexes..."
migrate -path migrations -database "$DATABASE_URL" up 1

# Run ANALYZE
echo "Updating table statistics..."
psql $DATABASE_URL -c "ANALYZE checkins; ANALYZE sync_queue; ANALYZE analytics_events;"

# Verify indexes created
echo "Verifying index creation..."
INDEX_COUNT=$(psql $DATABASE_URL -t -c "
    SELECT COUNT(*)
    FROM pg_indexes
    WHERE indexname IN (
        'idx_checkins_user_time_deleted',
        'idx_checkins_content_fts',
        'idx_sync_queue_user_status'
    );
")

if [ "$INDEX_COUNT" -lt 3 ]; then
    echo "ERROR: Expected 3+ critical indexes, found $INDEX_COUNT"
    exit 1
fi

echo "=== Deployment Complete ==="
echo "Monitor with: SELECT * FROM performance.new_index_usage;"
```

## Next Steps

After successful deployment of composite indexes:

1. **Task 18.4:** Implement table partitioning for checkins
2. **Task 18.5:** Query tuning based on execution plans
3. **Task 18.6:** Connection pooling optimization with pgBouncer
4. **Task 18.7:** Automated backup and PITR setup

## References

- [PostgreSQL CREATE INDEX CONCURRENTLY](https://www.postgresql.org/docs/current/sql-createindex.html#SQL-CREATEINDEX-CONCURRENTLY)
- [Workload Analysis Guide](./WORKLOAD_ANALYSIS_GUIDE.md) (Task 18.2)
- [Performance Monitoring Setup](../migrations/000017_enable_performance_monitoring.up.sql) (Task 18.1)
- [Index Types Documentation](https://www.postgresql.org/docs/current/indexes-types.html)
