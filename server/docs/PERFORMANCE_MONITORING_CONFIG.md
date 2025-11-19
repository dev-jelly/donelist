# Performance Monitoring Configuration

**Task 18.1** - 성능 기준선 수립과 슬로우 쿼리 로깅 활성화

## Overview

This document describes the PostgreSQL configuration settings required for performance monitoring, slow query logging, and baseline establishment.

## PostgreSQL Configuration Settings

### Required Settings in `postgresql.conf`

Add or update the following settings in your PostgreSQL configuration file:

```conf
# --------------------------
# Performance Monitoring
# --------------------------

# Enable pg_stat_statements
shared_preload_libraries = 'pg_stat_statements'

# pg_stat_statements configuration
pg_stat_statements.max = 10000                    # Track up to 10,000 distinct queries
pg_stat_statements.track = all                    # Track all queries (top-level and nested)
pg_stat_statements.track_utility = on             # Track utility commands (DDL, etc.)
pg_stat_statements.track_planning = on            # Track query planning time
pg_stat_statements.save = on                      # Save statistics across server restarts

# --------------------------
# Query Logging
# --------------------------

# Log slow queries (queries taking longer than 1000ms)
log_min_duration_statement = 1000                 # Log queries > 1 second

# Additional logging settings
log_line_prefix = '%t [%p] %u@%d '               # Timestamp, PID, user, database
log_statement = 'none'                            # Don't log all statements (only slow ones)
log_duration = off                                # Don't log all query durations
log_connections = on                              # Log connection attempts
log_disconnections = on                           # Log session terminations
log_lock_waits = on                               # Log long lock waits
log_temp_files = 0                                # Log all temp file usage
log_autovacuum_min_duration = 0                   # Log all autovacuum activity

# --------------------------
# Auto Explain
# --------------------------

# Load auto_explain module
# Note: This may need to be added to shared_preload_libraries
# shared_preload_libraries = 'pg_stat_statements,auto_explain'

# Auto explain configuration (set via ALTER SYSTEM or postgresql.conf)
auto_explain.log_min_duration = 1000              # Explain queries > 1 second
auto_explain.log_analyze = on                     # Include actual runtime statistics
auto_explain.log_buffers = on                     # Include buffer usage statistics
auto_explain.log_timing = on                      # Include timing information
auto_explain.log_triggers = on                    # Include trigger execution times
auto_explain.log_verbose = off                    # Don't include verbose output
auto_explain.log_nested_statements = on           # Include nested statements
auto_explain.log_format = json                    # Use JSON format for easier parsing

# --------------------------
# Statistics Collection
# --------------------------

track_activities = on                             # Track current commands
track_counts = on                                 # Collect statistics on database activity
track_io_timing = on                              # Collect I/O timing statistics
track_functions = all                             # Track function call counts and timing
track_activity_query_size = 4096                  # Track longer query texts
stats_temp_directory = 'pg_stat_tmp'              # Temporary statistics directory
```

## Applying Configuration Changes

### Method 1: Edit postgresql.conf directly

1. Locate your PostgreSQL configuration file:
   ```bash
   # Find postgresql.conf location
   psql -U postgres -c "SHOW config_file;"
   ```

2. Edit the file and add the settings above

3. Reload PostgreSQL configuration:
   ```bash
   # Reload without restart (for most settings)
   psql -U postgres -c "SELECT pg_reload_conf();"

   # Or restart PostgreSQL (required for shared_preload_libraries)
   sudo systemctl restart postgresql
   # or
   brew services restart postgresql@14
   ```

### Method 2: Use ALTER SYSTEM (Recommended)

```sql
-- Connect to PostgreSQL
psql -U postgres -d donelist

-- Configure pg_stat_statements (requires restart)
ALTER SYSTEM SET shared_preload_libraries = 'pg_stat_statements';
ALTER SYSTEM SET pg_stat_statements.max = 10000;
ALTER SYSTEM SET pg_stat_statements.track = 'all';
ALTER SYSTEM SET pg_stat_statements.track_utility = on;
ALTER SYSTEM SET pg_stat_statements.track_planning = on;

-- Configure slow query logging (no restart needed)
ALTER SYSTEM SET log_min_duration_statement = 1000;
ALTER SYSTEM SET log_line_prefix = '%t [%p] %u@%d ';
ALTER SYSTEM SET log_connections = on;
ALTER SYSTEM SET log_disconnections = on;
ALTER SYSTEM SET log_lock_waits = on;
ALTER SYSTEM SET log_temp_files = 0;

-- Configure statistics collection (no restart needed)
ALTER SYSTEM SET track_io_timing = on;
ALTER SYSTEM SET track_functions = 'all';
ALTER SYSTEM SET track_activity_query_size = 4096;

-- Reload configuration
SELECT pg_reload_conf();

-- Restart required for shared_preload_libraries changes
-- sudo systemctl restart postgresql
```

## Log Rotation Configuration

Configure log rotation to prevent disk space issues:

### Using logrotate (Linux)

Create `/etc/logrotate.d/postgresql`:

```conf
/var/log/postgresql/*.log {
    daily
    rotate 7
    missingok
    notifempty
    compress
    delaycompress
    sharedscripts
    postrotate
        /usr/bin/killall -HUP syslogd
    endscript
}
```

### PostgreSQL Built-in Log Rotation

In `postgresql.conf`:

```conf
logging_collector = on
log_directory = 'log'
log_filename = 'postgresql-%Y-%m-%d_%H%M%S.log'
log_rotation_age = 1d
log_rotation_size = 100MB
log_truncate_on_rotation = on
```

## Verification

After applying the configuration, verify the settings:

```sql
-- Check if pg_stat_statements is loaded
SELECT * FROM pg_extension WHERE extname = 'pg_stat_statements';

-- Check current configuration
SHOW shared_preload_libraries;
SHOW log_min_duration_statement;
SHOW track_io_timing;

-- Test pg_stat_statements
SELECT query, calls, total_exec_time, mean_exec_time
FROM pg_stat_statements
ORDER BY mean_exec_time DESC
LIMIT 10;

-- Check if performance schema exists
SELECT schema_name
FROM information_schema.schemata
WHERE schema_name = 'performance';
```

## Baseline Collection

After configuration, collect baseline metrics for 7 days:

```sql
-- Manual baseline capture (run daily or via cron)
SELECT performance.capture_baseline_metrics();
SELECT performance.capture_query_stats_snapshot();

-- View current statistics
SELECT * FROM performance.current_db_stats;
SELECT * FROM performance.top_slow_queries;
SELECT * FROM performance.table_stats;
SELECT * FROM performance.index_usage_stats;
```

### Automated Collection (via cron)

```bash
# Add to crontab to run daily at 2 AM
0 2 * * * psql -U postgres -d donelist -c "SELECT performance.capture_baseline_metrics(); SELECT performance.capture_query_stats_snapshot();"

# Or every 6 hours
0 */6 * * * psql -U postgres -d donelist -c "SELECT performance.capture_query_stats_snapshot();"
```

## Monitoring and Alerts

### Key Metrics to Monitor

1. **Cache Hit Ratio**: Should be > 95%
   ```sql
   SELECT * FROM performance.current_db_stats;
   ```

2. **Slow Queries**: Identify queries > 1 second
   ```sql
   SELECT * FROM performance.top_slow_queries;
   ```

3. **Index Usage**: Find unused indexes
   ```sql
   SELECT * FROM performance.index_usage_stats WHERE index_scans = 0;
   ```

4. **Table Bloat**: Monitor dead row percentage
   ```sql
   SELECT * FROM performance.table_stats WHERE dead_row_pct > 10;
   ```

### Sample Monitoring Queries

```sql
-- Get baseline trends over last 7 days
SELECT
    measurement_date,
    cache_hit_ratio,
    active_connections_avg,
    total_queries,
    slow_queries_count
FROM performance.baseline_metrics
WHERE measurement_date >= CURRENT_DATE - INTERVAL '7 days'
ORDER BY measurement_date DESC;

-- Find queries with highest average execution time
SELECT
    query_hash,
    query_preview,
    calls,
    avg_time_ms,
    total_time_ms,
    cache_hit_pct
FROM performance.top_slow_queries
WHERE avg_time_ms > 100
ORDER BY avg_time_ms DESC;

-- Check for missing indexes (sequential scans on large tables)
SELECT
    table_name,
    sequential_scans,
    rows_seq_scanned,
    live_rows,
    dead_row_pct
FROM performance.table_stats
WHERE sequential_scans > 1000
  AND live_rows > 10000
ORDER BY sequential_scans DESC;
```

## Performance Thresholds

Based on 7-day baseline collection, establish these thresholds:

- **Query Execution Time**:
  - Warning: > 1000ms (logged)
  - Critical: > 5000ms (requires investigation)

- **Cache Hit Ratio**:
  - Healthy: > 95%
  - Warning: < 95%
  - Critical: < 90%

- **Active Connections**:
  - Normal: < 50
  - Warning: 50-80
  - Critical: > 80

- **Dead Row Percentage**:
  - Healthy: < 5%
  - Warning: 5-10%
  - Critical: > 10% (needs VACUUM)

## Integration with Application

The Go application can query performance metrics programmatically:

```go
// Example: Get current database statistics
type DBStats struct {
    ActiveConnections int     `db:"active_connections"`
    CacheHitRatio    float64 `db:"cache_hit_ratio_pct"`
    DeadLocks        int     `db:"deadlocks"`
}

func GetDBStats(db *sqlx.DB) (*DBStats, error) {
    var stats DBStats
    err := db.Get(&stats, "SELECT * FROM performance.current_db_stats")
    return &stats, err
}
```

## Next Steps (Task 18.2)

After collecting 7 days of baseline metrics:
1. Analyze workload patterns
2. Identify top slow queries
3. Generate index candidates
4. Create optimization report

## References

- [PostgreSQL pg_stat_statements](https://www.postgresql.org/docs/current/pgstatstatements.html)
- [PostgreSQL auto_explain](https://www.postgresql.org/docs/current/auto-explain.html)
- [PostgreSQL Runtime Statistics](https://www.postgresql.org/docs/current/monitoring-stats.html)
