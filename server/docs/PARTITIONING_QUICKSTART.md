# Partitioning Quick Start Guide

## TL;DR - Implementation Checklist

```bash
# 1. Setup (5 minutes)
psql -d donelist -f migrations/000020_checkins_partitioning.up.sql

# 2. Migrate data (duration depends on data size)
psql -d donelist -f scripts/migrate_checkins_partitioned.sql

# 3. Test performance
psql -d donelist -f scripts/test_partition_performance.sql

# 4. Schedule monthly maintenance
crontab -e
# Add: 0 2 1 * * psql -d donelist -f /path/to/scripts/partition_maintenance.sql

# 5. Update application code (gradual cutover recommended)
# See full guide for cutover strategies
```

## Common Commands

### Check Partition Health
```sql
SELECT * FROM performance.partition_health
ORDER BY partition_start DESC;
```

### Create Future Partitions
```sql
SELECT * FROM partitions.ensure_future_partitions(3);
```

### View Migration Progress
```sql
SELECT * FROM performance.partition_migration_log
ORDER BY started_at DESC;
```

### Get Partition Statistics
```sql
SELECT
    partition_name,
    row_count,
    size_mb,
    is_current_month
FROM performance.get_partition_stats()
ORDER BY partition_start DESC
LIMIT 12;
```

### Test Query Performance
```sql
-- Check partition pruning
EXPLAIN (ANALYZE, BUFFERS)
SELECT * FROM checkins_partitioned
WHERE checkin_time >= '2024-11-01'
  AND checkin_time < '2024-12-01'
  AND user_id = 'your-uuid';

-- Look for "Partitions removed: X of Y" in output
```

### Manual Partition Creation
```sql
-- Create partition for specific month
SELECT partitions.create_checkins_partition('2024-12-01'::DATE);
```

### Check Default Partition (Should be Empty)
```sql
SELECT COUNT(*) FROM partitions.checkins_default;
-- Alert if > 0
```

## Query Patterns

### Optimal (Enables Partition Pruning)
```sql
-- ✅ Good: Date range filter
SELECT * FROM checkins_partitioned
WHERE user_id = 'X'
  AND checkin_time >= '2024-11-01'
  AND checkin_time < '2024-12-01';

-- ✅ Good: Recent data
SELECT * FROM checkins_partitioned
WHERE checkin_time >= CURRENT_DATE - INTERVAL '7 days'
  AND deleted_at IS NULL;
```

### Suboptimal (Poor Performance)
```sql
-- ❌ Bad: No date filter (scans all partitions)
SELECT * FROM checkins_partitioned
WHERE user_id = 'X';

-- ❌ Bad: Function on partition key
SELECT * FROM checkins_partitioned
WHERE DATE_TRUNC('month', checkin_time) = '2024-11-01';
```

## Monitoring Queries

### Partition Size Report
```sql
SELECT
    DATE_TRUNC('month', partition_start) as month,
    SUM(row_count) as total_rows,
    SUM(size_mb) as total_size_mb,
    COUNT(*) as num_partitions
FROM performance.get_partition_stats()
GROUP BY DATE_TRUNC('month', partition_start)
ORDER BY month DESC;
```

### Unused Indexes
```sql
SELECT
    schemaname,
    tablename,
    indexrelname,
    idx_scan,
    pg_size_pretty(pg_relation_size(indexrelid)) as size
FROM pg_stat_user_indexes
WHERE schemaname = 'partitions'
  AND idx_scan = 0
  AND pg_relation_size(indexrelid) > 10485760  -- > 10MB
ORDER BY pg_relation_size(indexrelid) DESC;
```

### Query Performance Tracking
```sql
-- Requires pg_stat_statements extension
SELECT
    LEFT(query, 100) as query_preview,
    calls,
    ROUND(mean_exec_time::numeric, 2) as avg_ms,
    ROUND(total_exec_time::numeric, 2) as total_ms
FROM pg_stat_statements
WHERE query LIKE '%checkins_partitioned%'
  AND query NOT LIKE '%pg_stat%'
ORDER BY mean_exec_time DESC
LIMIT 20;
```

## Troubleshooting

### All Partitions Being Scanned
```sql
-- Check partition key is in WHERE clause
-- Use EXPLAIN to verify:
EXPLAIN SELECT * FROM checkins_partitioned WHERE ...;
-- Look for "Partitions removed" message
```

### Slow Inserts
```sql
-- Check constraint_exclusion is enabled
SHOW constraint_exclusion;  -- Should be 'partition'

-- Verify partition exists for current date
SELECT * FROM partitions.create_checkins_partition(CURRENT_DATE);
```

### Data in Default Partition
```sql
-- Find date range
SELECT MIN(checkin_time), MAX(checkin_time)
FROM partitions.checkins_default;

-- Create missing partitions
SELECT partitions.create_checkins_partition('2024-XX-01'::DATE);

-- Move data from default to proper partition (automatic on next query)
```

## Rollback

### Before Data Migration
```bash
psql -d donelist -f migrations/000020_checkins_partitioning.down.sql
```

### After Data Migration
```sql
-- Option 1: Drop partitioned table
DROP TABLE checkins_partitioned CASCADE;

-- Option 2: Rename back to old table
ALTER TABLE checkins RENAME TO checkins_partitioned_backup;
ALTER TABLE checkins_old RENAME TO checkins;
```

## Maintenance Schedule

| Frequency | Task | Command |
|-----------|------|---------|
| Monthly | Create future partitions | `SELECT * FROM partitions.ensure_future_partitions(3);` |
| Monthly | Update statistics | Run `partition_maintenance.sql` |
| Quarterly | Review retention policy | `SELECT * FROM partitions.cleanup_old_partitions(24);` |
| Weekly | Check partition health | `SELECT * FROM performance.partition_health;` |
| Daily | Monitor default partition | `SELECT COUNT(*) FROM partitions.checkins_default;` |

## Performance Expectations

| Query Type | Expected Improvement |
|------------|---------------------|
| Single month timeline | 10x faster |
| 3-month range | 10x faster |
| Category filtering | 8x faster |
| Full-text search (single month) | 7.5x faster |
| Recent 7 days | 8x faster |

## Key Files

- **Migration**: `migrations/000020_checkins_partitioning.up.sql`
- **Data Migration**: `scripts/migrate_checkins_partitioned.sql`
- **Maintenance**: `scripts/partition_maintenance.sql`
- **Testing**: `scripts/test_partition_performance.sql`
- **Full Guide**: `docs/PARTITIONING_GUIDE.md`

## Support

For detailed information, see `docs/PARTITIONING_GUIDE.md`

Common issues and solutions are documented in the Troubleshooting section of the full guide.
