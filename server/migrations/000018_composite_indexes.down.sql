-- Migration Rollback: Composite Index Design and Application
-- Task: 18.3 - 복합 인덱스 설계·적용·롤백 전략 구현
-- Description: Safe rollback strategy for composite indexes

-- ============================================================================
-- ROLLBACK STRATEGY
-- ============================================================================
-- This rollback script removes all composite indexes created in the up migration.
-- Using CONCURRENTLY to avoid table locks during rollback.
-- ============================================================================

DO $$
BEGIN
    RAISE NOTICE '=====================================================';
    RAISE NOTICE 'Starting Composite Index Rollback (Task 18.3)';
    RAISE NOTICE '=====================================================';
END $$;

-- ============================================================================
-- DROP MONITORING VIEWS AND FUNCTIONS
-- ============================================================================

DROP VIEW IF EXISTS performance.index_impact_queries;
DROP VIEW IF EXISTS performance.new_index_usage;
DROP FUNCTION IF EXISTS performance.analyze_index_effectiveness();

-- ============================================================================
-- DROP COMPOSITE INDEXES
-- ============================================================================

-- Priority 1 (Critical) Indexes
DROP INDEX CONCURRENTLY IF EXISTS idx_checkins_user_time_deleted;
DROP INDEX CONCURRENTLY IF EXISTS idx_checkins_content_fts;
DROP INDEX CONCURRENTLY IF EXISTS idx_sync_queue_user_status;

-- Priority 2 (High) Indexes
DROP INDEX CONCURRENTLY IF EXISTS idx_checkins_category_time;
DROP INDEX CONCURRENTLY IF EXISTS idx_analytics_user_type_time;

-- Priority 3 (Medium) Indexes
DROP INDEX CONCURRENTLY IF EXISTS idx_search_history_user_time;
DROP INDEX CONCURRENTLY IF EXISTS idx_audit_logs_user_time;
DROP INDEX CONCURRENTLY IF EXISTS idx_webhook_deliveries_status_time;

-- Priority 4 (Low) Indexes
DROP INDEX CONCURRENTLY IF EXISTS idx_checkins_edited;

-- Additional Optimization Indexes
DROP INDEX CONCURRENTLY IF EXISTS idx_team_members_team_user;

-- ============================================================================
-- UPDATE DEPLOYMENT LOG
-- ============================================================================

-- Mark all deployed indexes as rolled back
UPDATE performance.index_deployment_log
SET
    status = 'rolled_back',
    notes = COALESCE(notes, '') || ' [ROLLED BACK: ' || CURRENT_TIMESTAMP || ']'
WHERE index_name IN (
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
AND status = 'completed';

-- ============================================================================
-- DROP DEPLOYMENT LOG TABLE (Optional - Commented Out)
-- ============================================================================

-- Uncomment if you want to completely remove the deployment tracking
-- DROP TABLE IF EXISTS performance.index_deployment_log;

-- ============================================================================
-- RESTORE ORIGINAL INDEXES (If Needed)
-- ============================================================================

-- If original single-column indexes were dropped during optimization,
-- recreate them here. Comment shows examples:

-- Restore basic indexes if they were removed
-- CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_checkins_user_id ON checkins(user_id);
-- CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_checkins_checkin_time ON checkins(checkin_time);
-- CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_checkins_category_id ON checkins(category_id);

-- Note: Check migration 000001_initial_schema.up.sql for original indexes

-- ============================================================================
-- POST-ROLLBACK CLEANUP
-- ============================================================================

-- Vacuum tables that had indexes dropped to reclaim space
VACUUM ANALYZE checkins;
VACUUM ANALYZE sync_queue;
VACUUM ANALYZE analytics_events;
VACUUM ANALYZE search_history;
VACUUM ANALYZE audit_logs;
VACUUM ANALYZE webhook_deliveries;
VACUUM ANALYZE team_members;

-- ============================================================================
-- COMPLETION LOG
-- ============================================================================

DO $$
BEGIN
    RAISE NOTICE '=====================================================';
    RAISE NOTICE 'Composite Index Rollback Complete';
    RAISE NOTICE '=====================================================';
    RAISE NOTICE 'All composite indexes have been dropped';
    RAISE NOTICE 'Deployment log updated with rollback status';
    RAISE NOTICE 'Tables have been vacuumed and analyzed';
    RAISE NOTICE '';
    RAISE NOTICE 'WARNING: Query performance may be degraded';
    RAISE NOTICE 'Consider recreating indexes if performance issues occur';
    RAISE NOTICE '=====================================================';
END $$;
