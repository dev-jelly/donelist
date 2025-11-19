-- Migration: Enable Performance Monitoring and Slow Query Logging
-- Task: 18.1 - 성능 기준선 수립과 슬로우 쿼리 로깅 활성화

-- Enable pg_stat_statements extension for query performance tracking
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

-- Create schema for performance monitoring
CREATE SCHEMA IF NOT EXISTS performance;

-- Performance baseline metrics table
CREATE TABLE performance.baseline_metrics (
    id SERIAL PRIMARY KEY,
    measurement_date DATE NOT NULL,
    tps_avg NUMERIC(10, 2),
    tps_peak NUMERIC(10, 2),
    latency_p50_ms NUMERIC(10, 2),
    latency_p95_ms NUMERIC(10, 2),
    latency_p99_ms NUMERIC(10, 2),
    active_connections_avg INTEGER,
    active_connections_peak INTEGER,
    cache_hit_ratio NUMERIC(5, 4),
    total_queries BIGINT,
    slow_queries_count INTEGER,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(measurement_date)
);

CREATE INDEX idx_baseline_metrics_date ON performance.baseline_metrics(measurement_date DESC);

-- Slow query log table for capturing queries that exceed threshold
CREATE TABLE performance.slow_query_log (
    id BIGSERIAL PRIMARY KEY,
    query_hash TEXT NOT NULL,
    query_text TEXT NOT NULL,
    execution_time_ms NUMERIC(10, 2) NOT NULL,
    rows_returned BIGINT,
    rows_examined BIGINT,
    user_name TEXT,
    database_name TEXT,
    occurred_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    explain_plan JSONB
);

CREATE INDEX idx_slow_query_log_occurred_at ON performance.slow_query_log(occurred_at DESC);
CREATE INDEX idx_slow_query_log_execution_time ON performance.slow_query_log(execution_time_ms DESC);
CREATE INDEX idx_slow_query_log_hash ON performance.slow_query_log(query_hash);

-- Query performance statistics snapshot table
CREATE TABLE performance.query_stats_snapshot (
    id BIGSERIAL PRIMARY KEY,
    snapshot_time TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    query_hash TEXT NOT NULL,
    query_text TEXT NOT NULL,
    calls BIGINT NOT NULL,
    total_exec_time_ms NUMERIC(15, 2) NOT NULL,
    mean_exec_time_ms NUMERIC(10, 2) NOT NULL,
    min_exec_time_ms NUMERIC(10, 2) NOT NULL,
    max_exec_time_ms NUMERIC(10, 2) NOT NULL,
    stddev_exec_time_ms NUMERIC(10, 2),
    rows_returned BIGINT,
    shared_blks_hit BIGINT,
    shared_blks_read BIGINT,
    shared_blks_written BIGINT,
    temp_blks_read BIGINT,
    temp_blks_written BIGINT
);

CREATE INDEX idx_query_stats_snapshot_time ON performance.query_stats_snapshot(snapshot_time DESC);
CREATE INDEX idx_query_stats_snapshot_exec_time ON performance.query_stats_snapshot(mean_exec_time_ms DESC);

-- Function to capture current performance baseline
CREATE OR REPLACE FUNCTION performance.capture_baseline_metrics()
RETURNS void AS $$
DECLARE
    v_tps_avg NUMERIC(10, 2);
    v_tps_peak NUMERIC(10, 2);
    v_cache_hit_ratio NUMERIC(5, 4);
    v_active_connections INTEGER;
BEGIN
    -- Calculate cache hit ratio
    SELECT
        CASE
            WHEN sum(blks_hit) + sum(blks_read) > 0 THEN
                round(sum(blks_hit)::numeric / (sum(blks_hit) + sum(blks_read)), 4)
            ELSE 0
        END INTO v_cache_hit_ratio
    FROM pg_stat_database
    WHERE datname = current_database();

    -- Get active connections
    SELECT count(*) INTO v_active_connections
    FROM pg_stat_activity
    WHERE state = 'active' AND datname = current_database();

    -- Insert baseline metrics
    INSERT INTO performance.baseline_metrics (
        measurement_date,
        cache_hit_ratio,
        active_connections_avg,
        total_queries
    )
    VALUES (
        CURRENT_DATE,
        v_cache_hit_ratio,
        v_active_connections,
        (SELECT sum(calls) FROM pg_stat_statements)
    )
    ON CONFLICT (measurement_date)
    DO UPDATE SET
        cache_hit_ratio = EXCLUDED.cache_hit_ratio,
        active_connections_avg = (performance.baseline_metrics.active_connections_avg + EXCLUDED.active_connections_avg) / 2,
        total_queries = EXCLUDED.total_queries,
        created_at = CURRENT_TIMESTAMP;
END;
$$ LANGUAGE plpgsql;

-- Function to capture query statistics snapshot
CREATE OR REPLACE FUNCTION performance.capture_query_stats_snapshot()
RETURNS void AS $$
BEGIN
    INSERT INTO performance.query_stats_snapshot (
        query_hash,
        query_text,
        calls,
        total_exec_time_ms,
        mean_exec_time_ms,
        min_exec_time_ms,
        max_exec_time_ms,
        stddev_exec_time_ms,
        rows_returned,
        shared_blks_hit,
        shared_blks_read,
        shared_blks_written,
        temp_blks_read,
        temp_blks_written
    )
    SELECT
        md5(query) as query_hash,
        query as query_text,
        calls,
        total_exec_time as total_exec_time_ms,
        mean_exec_time as mean_exec_time_ms,
        min_exec_time as min_exec_time_ms,
        max_exec_time as max_exec_time_ms,
        stddev_exec_time as stddev_exec_time_ms,
        rows,
        shared_blks_hit,
        shared_blks_read,
        shared_blks_written,
        temp_blks_read,
        temp_blks_written
    FROM pg_stat_statements
    WHERE query NOT LIKE '%pg_stat_statements%'
      AND query NOT LIKE '%performance.%'
    ORDER BY mean_exec_time DESC
    LIMIT 100;
END;
$$ LANGUAGE plpgsql;

-- View for top slow queries
CREATE OR REPLACE VIEW performance.top_slow_queries AS
SELECT
    query_hash,
    LEFT(query_text, 100) as query_preview,
    calls,
    round(mean_exec_time_ms::numeric, 2) as avg_time_ms,
    round(total_exec_time_ms::numeric, 2) as total_time_ms,
    round(max_exec_time_ms::numeric, 2) as max_time_ms,
    rows_returned,
    round((shared_blks_hit::numeric / NULLIF(shared_blks_hit + shared_blks_read, 0)) * 100, 2) as cache_hit_pct
FROM performance.query_stats_snapshot
WHERE snapshot_time >= NOW() - INTERVAL '7 days'
ORDER BY mean_exec_time_ms DESC
LIMIT 50;

-- View for current database statistics
CREATE OR REPLACE VIEW performance.current_db_stats AS
SELECT
    numbackends as active_connections,
    xact_commit as transactions_committed,
    xact_rollback as transactions_rolled_back,
    blks_read as blocks_read_from_disk,
    blks_hit as blocks_read_from_cache,
    round((blks_hit::numeric / NULLIF(blks_hit + blks_read, 0)) * 100, 2) as cache_hit_ratio_pct,
    tup_returned as rows_returned,
    tup_fetched as rows_fetched,
    tup_inserted as rows_inserted,
    tup_updated as rows_updated,
    tup_deleted as rows_deleted,
    conflicts,
    temp_files,
    temp_bytes,
    deadlocks,
    stats_reset
FROM pg_stat_database
WHERE datname = current_database();

-- View for table statistics
CREATE OR REPLACE VIEW performance.table_stats AS
SELECT
    schemaname,
    relname as table_name,
    seq_scan as sequential_scans,
    seq_tup_read as rows_seq_scanned,
    idx_scan as index_scans,
    idx_tup_fetch as rows_index_fetched,
    n_tup_ins as rows_inserted,
    n_tup_upd as rows_updated,
    n_tup_del as rows_deleted,
    n_tup_hot_upd as hot_updates,
    n_live_tup as live_rows,
    n_dead_tup as dead_rows,
    round((n_dead_tup::numeric / NULLIF(n_live_tup, 0)) * 100, 2) as dead_row_pct,
    last_vacuum,
    last_autovacuum,
    last_analyze,
    last_autoanalyze
FROM pg_stat_user_tables
ORDER BY seq_scan DESC, n_live_tup DESC;

-- View for index usage statistics
CREATE OR REPLACE VIEW performance.index_usage_stats AS
SELECT
    schemaname,
    tablename,
    indexname,
    idx_scan as index_scans,
    idx_tup_read as tuples_read,
    idx_tup_fetch as tuples_fetched,
    pg_size_pretty(pg_relation_size(indexrelid)) as index_size
FROM pg_stat_user_indexes
ORDER BY idx_scan ASC, pg_relation_size(indexrelid) DESC;

-- Grant permissions
GRANT USAGE ON SCHEMA performance TO PUBLIC;
GRANT SELECT ON ALL TABLES IN SCHEMA performance TO PUBLIC;
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA performance TO PUBLIC;

-- Add comment
COMMENT ON SCHEMA performance IS 'Performance monitoring and query optimization schema (Task 18.1)';
COMMENT ON TABLE performance.baseline_metrics IS 'Daily performance baseline measurements for tracking performance trends over time';
COMMENT ON TABLE performance.slow_query_log IS 'Log of queries that exceed performance thresholds';
COMMENT ON TABLE performance.query_stats_snapshot IS 'Historical snapshots of query performance statistics from pg_stat_statements';
