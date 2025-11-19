-- Rollback: Date-Based Partitioning for Checkins Table
-- Task: 18.4 - 체크인 테이블 날짜 기준 파티셔닝 설계 및 온라인 마이그레이션
-- Description: Safely rollback partitioning changes

-- ============================================================================
-- SAFETY CHECKS
-- ============================================================================

DO $$
BEGIN
    RAISE NOTICE '=====================================================';
    RAISE NOTICE 'Rolling back Partitioning Setup (Task 18.4)';
    RAISE NOTICE '=====================================================';
    RAISE NOTICE 'WARNING: This will drop the partitioned table structure';
    RAISE NOTICE 'Ensure data migration has NOT started or migrate data back';
    RAISE NOTICE '=====================================================';
END $$;

-- ============================================================================
-- PHASE 1: DROP VIEWS AND FUNCTIONS
-- ============================================================================

DROP VIEW IF EXISTS performance.partition_health CASCADE;
DROP FUNCTION IF EXISTS performance.get_partition_stats() CASCADE;
DROP FUNCTION IF EXISTS partitions.cleanup_old_partitions(INTEGER) CASCADE;
DROP FUNCTION IF EXISTS partitions.ensure_future_partitions(INTEGER) CASCADE;
DROP FUNCTION IF EXISTS partitions.create_checkins_partition(DATE) CASCADE;

-- ============================================================================
-- PHASE 2: DROP TRACKING TABLES
-- ============================================================================

DROP TABLE IF EXISTS performance.partition_migration_log CASCADE;
DROP TABLE IF EXISTS performance.partition_metadata CASCADE;

-- ============================================================================
-- PHASE 3: DROP PARTITIONED TABLE AND ALL PARTITIONS
-- ============================================================================

-- This will cascade drop all child partitions
DROP TABLE IF EXISTS checkins_partitioned CASCADE;

-- Clean up any orphaned partitions in the partitions schema
DO $$
DECLARE
    partition_record RECORD;
BEGIN
    FOR partition_record IN
        SELECT tablename
        FROM pg_tables
        WHERE schemaname = 'partitions'
            AND tablename LIKE 'checkins_%'
    LOOP
        EXECUTE format('DROP TABLE IF EXISTS partitions.%I CASCADE', partition_record.tablename);
        RAISE NOTICE 'Dropped partition: %', partition_record.tablename;
    END LOOP;
END $$;

-- ============================================================================
-- PHASE 4: CLEANUP SCHEMAS (if empty)
-- ============================================================================

-- Only drop schemas if they're empty
DO $$
BEGIN
    -- Check if partitions schema is empty
    IF NOT EXISTS (
        SELECT 1 FROM pg_tables WHERE schemaname = 'partitions'
    ) THEN
        DROP SCHEMA IF EXISTS partitions CASCADE;
        RAISE NOTICE 'Dropped empty partitions schema';
    ELSE
        RAISE NOTICE 'Partitions schema not empty, keeping it';
    END IF;

    -- Performance schema may have other objects, so we don't drop it
    RAISE NOTICE 'Performance schema retained (may contain other objects)';
END $$;

-- ============================================================================
-- COMPLETION LOG
-- ============================================================================

DO $$
BEGIN
    RAISE NOTICE '=====================================================';
    RAISE NOTICE 'Partitioning Rollback Complete';
    RAISE NOTICE '=====================================================';
    RAISE NOTICE 'Original checkins table is still active';
    RAISE NOTICE 'All partitioning infrastructure removed';
    RAISE NOTICE '=====================================================';
END $$;
