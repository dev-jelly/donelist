# Workload Analysis and Index Optimization Guide

**Task 18.2** - 워크로드 분석 및 인덱스 후보 도출 리포트 작성

## Overview

This guide explains how to analyze database workload patterns, identify performance bottlenecks, and generate index recommendations for the DoneList application.

## Prerequisites

1. Performance monitoring must be enabled (Task 18.1)
2. At least 24 hours of baseline data collected (7 days recommended)
3. `pg_stat_statements` extension enabled
4. Access to the workload analyzer CLI tool

## Running Workload Analysis

### Full Analysis

Generate a comprehensive workload analysis report:

```bash
cd server
go run cmd/workload-analyzer/main.go analyze

# Output as JSON
go run cmd/workload-analyzer/main.go analyze --json

# Save to file
go run cmd/workload-analyzer/main.go analyze --json --output=workload-report.json
```

### Specific Analysis Commands

#### 1. Unused Indexes

Find indexes that are rarely or never used:

```bash
go run cmd/workload-analyzer/main.go unused-indexes
```

Output includes:
- Index name and table
- Number of scans
- Index size
- Recommendation to drop if appropriate

**When to drop an index:**
- Index has 0 scans and has existed for > 30 days
- Index has < 10 scans but is very large (> 100 MB)
- Index duplicates another index

**How to safely drop:**
```sql
-- Test first by making it invalid
DROP INDEX CONCURRENTLY IF EXISTS idx_name;
```

#### 2. Missing Indexes

Identify tables with high sequential scans that might benefit from indexes:

```bash
go run cmd/workload-analyzer/main.go missing-indexes
```

Look for:
- Tables with > 1000 sequential scans
- Large tables (> 10,000 rows)
- High average rows per scan (> 100)

#### 3. Index Candidates

Get specific index creation recommendations:

```bash
go run cmd/workload-analyzer/main.go index-candidates
```

Recommendations are prioritized by:
1. **Critical** - High impact on query performance
2. **High** - Significant improvement expected
3. **Medium** - Moderate benefit
4. **Low** - Niche use cases

## Index Recommendations

### Priority 1 (Critical) - Implement Immediately

#### 1. Checkins User Timeline Index

```sql
-- Optimize user timeline queries with date range and soft delete
CREATE INDEX CONCURRENTLY idx_checkins_user_time_deleted
ON checkins(user_id, checkin_time DESC, deleted_at)
WHERE deleted_at IS NULL;
```

**Why:** Most frequent query pattern in the application
**Impact:** 50-80% improvement in timeline loading
**Use case:** `/api/v1/checkins?user_id=X&from=Y&to=Z`

#### 2. Checkins Full-Text Search

```sql
-- Enable fast full-text search on checkin content
CREATE INDEX CONCURRENTLY idx_checkins_content_fts
ON checkins USING gin(to_tsvector('english', content));
```

**Why:** Critical for search functionality
**Impact:** Enables sub-second full-text searches
**Use case:** Search feature, text filtering

#### 3. Sync Queue Processing

```sql
-- Optimize offline sync queue processing
CREATE INDEX CONCURRENTLY idx_sync_queue_user_status
ON sync_queue(user_id, status, created_at)
WHERE status = 'pending';
```

**Why:** Critical for offline sync performance
**Impact:** Faster sync queue processing
**Use case:** Offline sync worker queries

### Priority 2 (High) - Implement Within Week

#### 4. Category-Based Filtering

```sql
-- Speed up category-filtered timeline queries
CREATE INDEX CONCURRENTLY idx_checkins_category_time
ON checkins(category_id, checkin_time DESC)
WHERE category_id IS NOT NULL AND deleted_at IS NULL;
```

**Why:** Common filtering operation
**Impact:** 30-50% improvement for category views

#### 5. Analytics Event Aggregation

```sql
-- Optimize analytics queries by user and event type
CREATE INDEX CONCURRENTLY idx_analytics_user_type_time
ON analytics_events(user_id, event_type, created_at DESC);
```

**Why:** Used in analytics reporting
**Impact:** Faster dashboard loading

### Priority 3 (Medium) - Consider for Future

#### 6. Search History

```sql
-- Speed up user search history retrieval
CREATE INDEX CONCURRENTLY idx_search_history_user_time
ON search_history(user_id, searched_at DESC);
```

**Why:** Improves search UX
**Impact:** Faster search history display

### Priority 4 (Low) - Optional

#### 7. Edited Checkins (Premium Feature)

```sql
-- Optimize queries for edited checkins
CREATE INDEX CONCURRENTLY idx_checkins_edited
ON checkins(user_id, last_edited_at DESC)
WHERE is_edited = true;
```

**Why:** Premium feature, smaller user base
**Impact:** Faster edit history for premium users

## Index Maintenance Best Practices

### Creating Indexes

Always use `CONCURRENTLY` to avoid table locks:

```sql
-- Good - doesn't lock table
CREATE INDEX CONCURRENTLY idx_name ON table(column);

-- Bad - locks entire table
CREATE INDEX idx_name ON table(column);
```

Monitor progress:
```sql
SELECT now()::time, query
FROM pg_stat_activity
WHERE query LIKE '%CREATE INDEX%';
```

### Dropping Indexes

Before dropping, verify it's unused:

```sql
-- Check index usage
SELECT
    schemaname,
    tablename,
    indexname,
    idx_scan,
    pg_size_pretty(pg_relation_size(indexrelid))
FROM pg_stat_user_indexes
WHERE indexrelname = 'idx_name';

-- Drop safely
DROP INDEX CONCURRENTLY IF EXISTS idx_name;
```

### Reindexing

Rebuild bloated indexes:

```sql
-- Check for index bloat
SELECT
    schemaname,
    tablename,
    indexname,
    pg_size_pretty(pg_relation_size(indexrelid)) as size
FROM pg_stat_user_indexes
ORDER BY pg_relation_size(indexrelid) DESC;

-- Rebuild concurrently (PostgreSQL 12+)
REINDEX INDEX CONCURRENTLY idx_name;
```

## Query Pattern Analysis

### Common Query Patterns in DoneList

#### 1. User Timeline Queries

```sql
-- Pattern: Get user checkins in date range
SELECT * FROM checkins
WHERE user_id = $1
  AND checkin_time >= $2
  AND checkin_time <= $3
  AND deleted_at IS NULL
ORDER BY checkin_time DESC;

-- Optimized by: idx_checkins_user_time_deleted
```

#### 2. Category Filtering

```sql
-- Pattern: Get checkins by category
SELECT * FROM checkins
WHERE user_id = $1
  AND category_id = $2
  AND deleted_at IS NULL
ORDER BY checkin_time DESC;

-- Optimized by: idx_checkins_category_time
```

#### 3. Full-Text Search

```sql
-- Pattern: Search checkin content
SELECT * FROM checkins
WHERE user_id = $1
  AND to_tsvector('english', content) @@ plainto_tsquery('english', $2)
  AND deleted_at IS NULL;

-- Optimized by: idx_checkins_content_fts
```

#### 4. Sync Queue Processing

```sql
-- Pattern: Get pending sync items
SELECT * FROM sync_queue
WHERE user_id = $1
  AND status = 'pending'
ORDER BY created_at ASC
LIMIT 100;

-- Optimized by: idx_sync_queue_user_status
```

## Testing Index Impact

### Before Creating Index

1. Enable query timing:
```sql
\timing on
```

2. Run representative queries:
```sql
EXPLAIN ANALYZE
SELECT * FROM checkins
WHERE user_id = 'user-uuid'
  AND checkin_time >= NOW() - INTERVAL '7 days'
  AND deleted_at IS NULL
ORDER BY checkin_time DESC;
```

3. Note execution time and plan cost

### After Creating Index

1. Run the same queries
2. Compare execution time and plan
3. Verify index is being used:

```sql
EXPLAIN (ANALYZE, BUFFERS)
SELECT * FROM checkins
WHERE user_id = 'user-uuid'
  AND checkin_time >= NOW() - INTERVAL '7 days'
  AND deleted_at IS NULL
ORDER BY checkin_time DESC;
```

Look for `Index Scan using idx_checkins_user_time_deleted` in the plan.

### Benchmark Results Template

```markdown
## Index Performance Test Results

**Index:** idx_checkins_user_time_deleted
**Created:** 2025-11-13
**Test Query:** User timeline for last 7 days

### Before Index
- Execution Time: 245ms
- Planning Time: 1ms
- Rows Scanned: 15,420
- Buffers Hit: 3,234

### After Index
- Execution Time: 12ms (95% improvement)
- Planning Time: 0.5ms
- Rows Scanned: 127
- Buffers Hit: 45
- Index Used: Yes

**Verdict:** ✅ High impact, keep index
```

## Monitoring Index Performance

### Daily Checks

```sql
-- Index hit rate (should be > 99%)
SELECT
    schemaname,
    tablename,
    indexrelname,
    idx_scan,
    round(100.0 * idx_scan / NULLIF(seq_scan + idx_scan, 0), 2) as index_usage_pct
FROM pg_stat_user_tables t
JOIN pg_stat_user_indexes i ON t.relid = i.relid
WHERE schemaname NOT IN ('pg_catalog', 'information_schema')
ORDER BY idx_scan DESC;
```

### Weekly Reviews

```bash
# Generate weekly workload report
go run cmd/workload-analyzer/main.go analyze --output=reports/workload-$(date +%Y%m%d).json

# Check for new slow queries
go run cmd/perf-monitor/main.go slow-queries --limit=20

# Review unused indexes
go run cmd/workload-analyzer/main.go unused-indexes
```

### Monthly Maintenance

```sql
-- Vacuum and analyze all tables
VACUUM ANALYZE;

-- Update statistics
ANALYZE;

-- Reindex if needed
REINDEX DATABASE donelist CONCURRENTLY;
```

## Integration with CI/CD

### Pre-Deployment Index Creation

```bash
#!/bin/bash
# scripts/create-indexes.sh

set -e

echo "Creating performance indexes..."

psql $DATABASE_URL <<EOF
-- Create all recommended indexes concurrently
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_checkins_user_time_deleted
  ON checkins(user_id, checkin_time DESC, deleted_at)
  WHERE deleted_at IS NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_checkins_content_fts
  ON checkins USING gin(to_tsvector('english', content));

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sync_queue_user_status
  ON sync_queue(user_id, status, created_at)
  WHERE status = 'pending';

EOF

echo "Indexes created successfully"
```

### Automated Workload Analysis

```yaml
# .github/workflows/workload-analysis.yml
name: Weekly Workload Analysis

on:
  schedule:
    - cron: '0 2 * * 1'  # Every Monday at 2 AM

jobs:
  analyze:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Run workload analysis
        run: |
          cd server
          go run cmd/workload-analyzer/main.go analyze --json --output=workload-report.json
      - name: Upload report
        uses: actions/upload-artifact@v3
        with:
          name: workload-report
          path: server/workload-report.json
```

## Troubleshooting

### Index Not Being Used

**Problem:** Created index but queries still use sequential scan

**Solutions:**
1. Run ANALYZE to update statistics:
   ```sql
   ANALYZE checkins;
   ```

2. Check if planner estimates are accurate:
   ```sql
   EXPLAIN ANALYZE SELECT ...;
   ```

3. Increase `random_page_cost` if needed:
   ```sql
   SET random_page_cost = 1.1;  -- For SSD
   ```

### Index Bloat

**Problem:** Index size growing too large

**Solution:**
```sql
-- Check bloat
SELECT
    schemaname,
    tablename,
    indexrelname,
    pg_size_pretty(pg_relation_size(indexrelid)) as index_size
FROM pg_stat_user_indexes
WHERE pg_relation_size(indexrelid) > 100000000  -- > 100 MB
ORDER BY pg_relation_size(indexrelid) DESC;

-- Rebuild
REINDEX INDEX CONCURRENTLY idx_name;
```

### Slow Index Creation

**Problem:** Index creation taking too long

**Options:**
1. Increase `maintenance_work_mem`:
   ```sql
   SET maintenance_work_mem = '2GB';
   ```

2. Create during low-traffic period

3. Use parallel index build (PostgreSQL 11+):
   ```sql
   SET max_parallel_maintenance_workers = 4;
   ```

## References

- [PostgreSQL Index Types](https://www.postgresql.org/docs/current/indexes-types.html)
- [Index Scanning Performance](https://www.postgresql.org/docs/current/indexes-examine.html)
- [Concurrent Index Creation](https://www.postgresql.org/docs/current/sql-createindex.html#SQL-CREATEINDEX-CONCURRENTLY)
