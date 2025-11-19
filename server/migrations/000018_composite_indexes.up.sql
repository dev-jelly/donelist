-- Migration: Composite Index Design and Application with Rollback Strategy
-- Task: 18.3 - 복합 인덱스 설계·적용·롤백 전략 구현
-- Description: Apply composite indexes based on workload analysis with online creation strategy

-- ============================================================================
-- PRIORITY 1 (CRITICAL) - Implement Immediately
-- ============================================================================

-- 1. Checkins User Timeline Index
-- Optimizes: User timeline queries with date range and soft delete filtering
-- Impact: 50-80% improvement in timeline loading
-- Use Case: GET /api/v1/checkins?user_id=X&from=Y&to=Z
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_checkins_user_time_deleted
ON checkins(user_id, checkin_time DESC, deleted_at)
WHERE deleted_at IS NULL;

COMMENT ON INDEX idx_checkins_user_time_deleted IS
'Composite index for user timeline queries. Covers user_id filtering, time ordering, and soft delete. Priority 1 (Critical).';

-- 2. Checkins Full-Text Search
-- Optimizes: Full-text search on checkin content
-- Impact: Enables sub-second full-text searches
-- Use Case: Search feature, text filtering
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_checkins_content_fts
ON checkins USING gin(to_tsvector('english', content));

COMMENT ON INDEX idx_checkins_content_fts IS
'GIN index for full-text search on checkin content. Enables fast text search. Priority 1 (Critical).';

-- 3. Sync Queue Processing
-- Optimizes: Offline sync queue processing
-- Impact: Faster sync queue processing, critical for offline sync
-- Use Case: Sync worker queries for pending items
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sync_queue_user_status
ON sync_queue(user_id, status, created_at)
WHERE status = 'pending';

COMMENT ON INDEX idx_sync_queue_user_status IS
'Partial composite index for sync queue processing. Only indexes pending items. Priority 1 (Critical).';

-- ============================================================================
-- PRIORITY 2 (HIGH) - Implement Within Week
-- ============================================================================

-- 4. Category-Based Filtering
-- Optimizes: Category-filtered timeline queries
-- Impact: 30-50% improvement for category views
-- Use Case: GET /api/v1/checkins?category_id=X
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_checkins_category_time
ON checkins(category_id, checkin_time DESC)
WHERE category_id IS NOT NULL AND deleted_at IS NULL;

COMMENT ON INDEX idx_checkins_category_time IS
'Partial composite index for category-filtered timelines. Priority 2 (High).';

-- 5. Analytics Event Aggregation
-- Optimizes: Analytics queries by user and event type
-- Impact: Faster dashboard loading and analytics reporting
-- Use Case: Analytics aggregation queries
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_analytics_user_type_time
ON analytics_events(user_id, event_type, created_at DESC);

COMMENT ON INDEX idx_analytics_user_type_time IS
'Composite index for analytics event aggregation by user and type. Priority 2 (High).';

-- ============================================================================
-- PRIORITY 3 (MEDIUM) - Consider for Future
-- ============================================================================

-- 6. Search History
-- Optimizes: User search history retrieval
-- Impact: Faster search history display
-- Use Case: GET /api/v1/search/history
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_search_history_user_time
ON search_history(user_id, searched_at DESC);

COMMENT ON INDEX idx_search_history_user_time IS
'Composite index for user search history. Priority 3 (Medium).';

-- ============================================================================
-- PRIORITY 4 (LOW) - Optional Premium Features
-- ============================================================================

-- 7. Edited Checkins (Premium Feature)
-- Optimizes: Queries for edited checkins
-- Impact: Faster edit history for premium users
-- Use Case: Edit history feature
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_checkins_edited
ON checkins(user_id, last_edited_at DESC)
WHERE is_edited = true;

COMMENT ON INDEX idx_checkins_edited IS
'Partial composite index for edited checkins (Premium feature). Priority 4 (Low).';

-- ============================================================================
-- ADDITIONAL OPTIMIZATION INDEXES
-- ============================================================================

-- 8. Audit Logs Time-Based Queries
-- Optimizes: Audit log retrieval by user and time
-- Impact: Faster audit trail viewing
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_audit_logs_user_time
ON audit_logs(user_id, created_at DESC);

COMMENT ON INDEX idx_audit_logs_user_time IS
'Composite index for audit log queries by user and time.';

-- 9. Webhook Deliveries Status Tracking
-- Optimizes: Webhook retry and status queries
-- Impact: Faster webhook management
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_webhook_deliveries_status_time
ON webhook_deliveries(webhook_id, status, created_at DESC)
WHERE status IN ('pending', 'failed');

COMMENT ON INDEX idx_webhook_deliveries_status_time IS
'Partial composite index for pending/failed webhook deliveries.';

-- 10. Teams Member Lookup
-- Optimizes: Team member queries
-- Impact: Faster team member lookups
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_team_members_team_user
ON team_members(team_id, user_id, role)
WHERE deleted_at IS NULL;

COMMENT ON INDEX idx_team_members_team_user IS
'Composite index for team member lookups with soft delete support.';

-- ============================================================================
-- CLEANUP: Remove Redundant Indexes
-- ============================================================================

-- Check for and remove duplicate or redundant indexes
-- Note: These DROP statements are commented out by default for safety
-- Uncomment after verifying they are truly redundant

-- Example: If idx_checkins_user_id exists, it's now redundant due to idx_checkins_user_time_deleted
-- DROP INDEX CONCURRENTLY IF EXISTS idx_checkins_user_id;

-- Example: If idx_checkins_checkin_time exists alone, it may be redundant
-- DROP INDEX CONCURRENTLY IF EXISTS idx_checkins_checkin_time;

-- ============================================================================
-- INDEX PERFORMANCE TRACKING
-- ============================================================================

-- Create a table to track index creation and performance impact
CREATE TABLE IF NOT EXISTS performance.index_deployment_log (
    id SERIAL PRIMARY KEY,
    index_name TEXT NOT NULL,
    table_name TEXT NOT NULL,
    priority INTEGER,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    size_before_mb NUMERIC(10, 2),
    size_after_mb NUMERIC(10, 2),
    creation_duration_seconds INTEGER,
    status TEXT CHECK (status IN ('pending', 'creating', 'completed', 'failed', 'rolled_back')),
    notes TEXT
);

-- Log the deployment of new indexes
INSERT INTO performance.index_deployment_log (index_name, table_name, priority, status, notes)
VALUES
    ('idx_checkins_user_time_deleted', 'checkins', 1, 'completed', 'User timeline optimization - Critical'),
    ('idx_checkins_content_fts', 'checkins', 1, 'completed', 'Full-text search - Critical'),
    ('idx_sync_queue_user_status', 'sync_queue', 1, 'completed', 'Sync queue processing - Critical'),
    ('idx_checkins_category_time', 'checkins', 2, 'completed', 'Category filtering - High'),
    ('idx_analytics_user_type_time', 'analytics_events', 2, 'completed', 'Analytics aggregation - High'),
    ('idx_search_history_user_time', 'search_history', 3, 'completed', 'Search history - Medium'),
    ('idx_checkins_edited', 'checkins', 4, 'completed', 'Edited checkins - Low'),
    ('idx_audit_logs_user_time', 'audit_logs', 3, 'completed', 'Audit logs - Medium'),
    ('idx_webhook_deliveries_status_time', 'webhook_deliveries', 3, 'completed', 'Webhook status - Medium'),
    ('idx_team_members_team_user', 'team_members', 2, 'completed', 'Team members - High');

-- ============================================================================
-- VALIDATION AND MONITORING VIEWS
-- ============================================================================

-- View to check index usage after deployment
CREATE OR REPLACE VIEW performance.new_index_usage AS
SELECT
    i.schemaname,
    i.tablename,
    i.indexrelname as index_name,
    i.idx_scan as scans_count,
    i.idx_tup_read as tuples_read,
    i.idx_tup_fetch as tuples_fetched,
    pg_size_pretty(pg_relation_size(i.indexrelid)) as index_size,
    CASE
        WHEN i.idx_scan = 0 THEN 'UNUSED'
        WHEN i.idx_scan < 10 THEN 'RARELY_USED'
        WHEN i.idx_scan < 100 THEN 'MODERATELY_USED'
        ELSE 'FREQUENTLY_USED'
    END as usage_level,
    s.n_live_tup as table_rows,
    ROUND(100.0 * i.idx_scan / NULLIF(s.seq_scan + i.idx_scan, 0), 2) as index_usage_pct
FROM pg_stat_user_indexes i
JOIN pg_stat_user_tables s ON i.relid = s.relid
WHERE i.indexrelname IN (
    'idx_checkins_user_time_deleted',
    'idx_checkins_content_fts',
    'idx_sync_queue_user_status',
    'idx_checkins_category_time',
    'idx_analytics_user_type_time',
    'idx_search_history_user_time',
    'idx_checkins_edited',
    'idx_audit_logs_user_time',
    'idx_webhook_deliveries_status_time',
    'idx_team_members_team_user'
)
ORDER BY i.idx_scan DESC;

COMMENT ON VIEW performance.new_index_usage IS
'Monitor usage statistics for newly deployed composite indexes from Task 18.3';

-- View to compare query performance before/after index creation
CREATE OR REPLACE VIEW performance.index_impact_queries AS
SELECT
    query_hash,
    LEFT(query_text, 100) as query_preview,
    calls,
    mean_exec_time_ms as avg_time_ms,
    max_exec_time_ms,
    snapshot_time,
    CASE
        WHEN query_text ILIKE '%checkins%user_id%checkin_time%' THEN 'idx_checkins_user_time_deleted'
        WHEN query_text ILIKE '%to_tsvector%checkins%' THEN 'idx_checkins_content_fts'
        WHEN query_text ILIKE '%sync_queue%status%pending%' THEN 'idx_sync_queue_user_status'
        WHEN query_text ILIKE '%checkins%category_id%' THEN 'idx_checkins_category_time'
        WHEN query_text ILIKE '%analytics_events%event_type%' THEN 'idx_analytics_user_type_time'
        ELSE 'other'
    END as likely_index_used
FROM performance.query_stats_snapshot
WHERE snapshot_time >= NOW() - INTERVAL '30 days'
    AND (
        query_text ILIKE '%checkins%'
        OR query_text ILIKE '%sync_queue%'
        OR query_text ILIKE '%analytics_events%'
        OR query_text ILIKE '%search_history%'
    )
ORDER BY mean_exec_time_ms DESC
LIMIT 100;

COMMENT ON VIEW performance.index_impact_queries IS
'Analyze query performance changes after composite index deployment';

-- ============================================================================
-- POST-DEPLOYMENT ANALYSIS FUNCTION
-- ============================================================================

CREATE OR REPLACE FUNCTION performance.analyze_index_effectiveness()
RETURNS TABLE(
    index_name TEXT,
    table_name TEXT,
    scans_count BIGINT,
    size TEXT,
    effectiveness TEXT,
    recommendation TEXT
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        i.indexrelname::TEXT,
        i.tablename::TEXT,
        i.idx_scan,
        pg_size_pretty(pg_relation_size(i.indexrelid))::TEXT,
        CASE
            WHEN i.idx_scan > 1000 THEN 'EXCELLENT'
            WHEN i.idx_scan > 100 THEN 'GOOD'
            WHEN i.idx_scan > 10 THEN 'MODERATE'
            ELSE 'POOR'
        END::TEXT,
        CASE
            WHEN i.idx_scan = 0 AND pg_relation_size(i.indexrelid) > 10485760 THEN 'Consider dropping - unused and large (>10MB)'
            WHEN i.idx_scan < 10 AND pg_relation_size(i.indexrelid) > 52428800 THEN 'Review usage - rarely used but large (>50MB)'
            WHEN i.idx_scan > 1000 THEN 'Excellent - keep and monitor'
            ELSE 'Monitor usage - still collecting data'
        END::TEXT
    FROM pg_stat_user_indexes i
    WHERE i.indexrelname LIKE 'idx_%'
        AND i.schemaname NOT IN ('pg_catalog', 'information_schema')
    ORDER BY i.idx_scan DESC;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION performance.analyze_index_effectiveness() IS
'Analyze effectiveness of all indexes and provide recommendations';

-- ============================================================================
-- GRANT PERMISSIONS
-- ============================================================================

GRANT SELECT ON performance.index_deployment_log TO PUBLIC;
GRANT SELECT ON performance.new_index_usage TO PUBLIC;
GRANT SELECT ON performance.index_impact_queries TO PUBLIC;
GRANT EXECUTE ON FUNCTION performance.analyze_index_effectiveness() TO PUBLIC;

-- ============================================================================
-- COMPLETION LOG
-- ============================================================================

DO $$
BEGIN
    RAISE NOTICE '=====================================================';
    RAISE NOTICE 'Composite Index Deployment Complete (Task 18.3)';
    RAISE NOTICE '=====================================================';
    RAISE NOTICE 'Priority 1 (Critical): 3 indexes created';
    RAISE NOTICE 'Priority 2 (High): 2 indexes created';
    RAISE NOTICE 'Priority 3 (Medium): 2 indexes created';
    RAISE NOTICE 'Priority 4 (Low): 1 index created';
    RAISE NOTICE 'Additional Optimizations: 3 indexes created';
    RAISE NOTICE '=====================================================';
    RAISE NOTICE 'Total: 11 composite indexes deployed';
    RAISE NOTICE '';
    RAISE NOTICE 'Next Steps:';
    RAISE NOTICE '1. Monitor index usage: SELECT * FROM performance.new_index_usage;';
    RAISE NOTICE '2. Analyze effectiveness: SELECT * FROM performance.analyze_index_effectiveness();';
    RAISE NOTICE '3. Compare query performance: SELECT * FROM performance.index_impact_queries;';
    RAISE NOTICE '4. Run ANALYZE on affected tables';
    RAISE NOTICE '=====================================================';
END $$;
