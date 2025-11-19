-- Migration Rollback: Disable Performance Monitoring

-- Drop views
DROP VIEW IF EXISTS performance.index_usage_stats;
DROP VIEW IF EXISTS performance.table_stats;
DROP VIEW IF EXISTS performance.current_db_stats;
DROP VIEW IF EXISTS performance.top_slow_queries;

-- Drop functions
DROP FUNCTION IF EXISTS performance.capture_query_stats_snapshot();
DROP FUNCTION IF EXISTS performance.capture_baseline_metrics();

-- Drop tables
DROP TABLE IF EXISTS performance.query_stats_snapshot;
DROP TABLE IF EXISTS performance.slow_query_log;
DROP TABLE IF EXISTS performance.baseline_metrics;

-- Drop schema
DROP SCHEMA IF EXISTS performance CASCADE;

-- Note: pg_stat_statements extension is not dropped as it may be used by other monitoring tools
-- To manually drop: DROP EXTENSION IF EXISTS pg_stat_statements;
