-- Migration: Date-Based Partitioning for Checkins Table
-- Task: 18.4 - 체크인 테이블 날짜 기준 파티셔닝 설계 및 온라인 마이그레이션
-- Description: Implement monthly RANGE partitioning on checkin_time with zero-downtime migration
-- Strategy: Create new partitioned table, gradually migrate data, then swap

-- ============================================================================
-- PARTITIONING STRATEGY
-- ============================================================================
-- Partition Key: checkin_time (TIMESTAMP WITH TIME ZONE)
-- Partition Type: RANGE (monthly boundaries)
-- Retention: Keep 24 months of data (configurable)
-- Auto-creation: New partitions created 3 months in advance
-- Naming: checkins_y{YYYY}_m{MM} (e.g., checkins_y2024_m11)
-- ============================================================================

-- Create performance schema if not exists (for monitoring and management)
CREATE SCHEMA IF NOT EXISTS performance;
CREATE SCHEMA IF NOT EXISTS partitions;

-- ============================================================================
-- PHASE 1: CREATE PARTITIONED TABLE STRUCTURE
-- ============================================================================

-- Create the new partitioned table with same structure as checkins
CREATE TABLE IF NOT EXISTS checkins_partitioned (
    id UUID NOT NULL DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL,
    category_id UUID,
    content TEXT NOT NULL,
    checkin_time TIMESTAMP WITH TIME ZONE NOT NULL,
    duration_minutes INTEGER DEFAULT 15 CHECK (duration_minutes IN (15, 30, 45, 120)),
    is_edited BOOLEAN DEFAULT FALSE,
    edit_count INTEGER DEFAULT 0,
    last_edited_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE,

    -- Primary key must include partition key
    PRIMARY KEY (id, checkin_time)
) PARTITION BY RANGE (checkin_time);

COMMENT ON TABLE checkins_partitioned IS
'Partitioned version of checkins table using monthly RANGE partitioning on checkin_time. Migration in progress.';

-- ============================================================================
-- PHASE 2: CREATE INITIAL PARTITIONS
-- ============================================================================

-- Create partitions for past 6 months, current month, and next 6 months
-- This ensures we have coverage for existing data and future data

-- Function to create a partition for a specific month
CREATE OR REPLACE FUNCTION partitions.create_checkins_partition(
    partition_date DATE
) RETURNS TEXT AS $$
DECLARE
    partition_name TEXT;
    start_date DATE;
    end_date DATE;
BEGIN
    -- Calculate partition boundaries (first day of month to first day of next month)
    start_date := DATE_TRUNC('month', partition_date)::DATE;
    end_date := (DATE_TRUNC('month', partition_date) + INTERVAL '1 month')::DATE;

    -- Generate partition name: checkins_y2024_m11
    partition_name := 'checkins_y' || TO_CHAR(start_date, 'YYYY') || '_m' || TO_CHAR(start_date, 'MM');

    -- Create partition if it doesn't exist
    EXECUTE format(
        'CREATE TABLE IF NOT EXISTS partitions.%I PARTITION OF checkins_partitioned
         FOR VALUES FROM (%L) TO (%L)',
        partition_name,
        start_date,
        end_date
    );

    -- Add indexes to the partition
    -- These will be created automatically on the partition
    EXECUTE format(
        'CREATE INDEX IF NOT EXISTS %I ON partitions.%I (user_id, checkin_time DESC, deleted_at) WHERE deleted_at IS NULL',
        partition_name || '_user_time_deleted_idx',
        partition_name
    );

    EXECUTE format(
        'CREATE INDEX IF NOT EXISTS %I ON partitions.%I (category_id, checkin_time DESC) WHERE category_id IS NOT NULL AND deleted_at IS NULL',
        partition_name || '_category_time_idx',
        partition_name
    );

    EXECUTE format(
        'CREATE INDEX IF NOT EXISTS %I ON partitions.%I USING gin(to_tsvector(''english'', content))',
        partition_name || '_content_fts_idx',
        partition_name
    );

    RETURN partition_name;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION partitions.create_checkins_partition(DATE) IS
'Creates a monthly partition for checkins_partitioned table with all necessary indexes';

-- Create partitions for the range: 6 months ago to 6 months in the future
DO $$
DECLARE
    partition_month DATE;
    partition_name TEXT;
BEGIN
    -- Start from 6 months ago
    partition_month := DATE_TRUNC('month', CURRENT_DATE - INTERVAL '6 months');

    -- Create 13 partitions (6 past + current + 6 future)
    FOR i IN 0..12 LOOP
        partition_name := partitions.create_checkins_partition(partition_month);
        RAISE NOTICE 'Created partition: %', partition_name;
        partition_month := partition_month + INTERVAL '1 month';
    END LOOP;
END $$;

-- Create a default partition for any data outside the range
CREATE TABLE IF NOT EXISTS partitions.checkins_default PARTITION OF checkins_partitioned DEFAULT;

COMMENT ON TABLE partitions.checkins_default IS
'Default partition for checkins outside the defined date ranges. Should be monitored and rarely used.';

-- ============================================================================
-- PHASE 3: PARTITION MANAGEMENT AUTOMATION
-- ============================================================================

-- Table to track partition metadata
CREATE TABLE IF NOT EXISTS performance.partition_metadata (
    id SERIAL PRIMARY KEY,
    table_name TEXT NOT NULL,
    partition_name TEXT NOT NULL UNIQUE,
    partition_start DATE NOT NULL,
    partition_end DATE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    row_count BIGINT,
    size_bytes BIGINT,
    last_analyzed TIMESTAMP WITH TIME ZONE,
    status TEXT CHECK (status IN ('active', 'archived', 'dropped')) DEFAULT 'active',
    notes TEXT
);

CREATE INDEX idx_partition_metadata_table_start ON performance.partition_metadata(table_name, partition_start);
CREATE INDEX idx_partition_metadata_status ON performance.partition_metadata(status);

COMMENT ON TABLE performance.partition_metadata IS
'Tracks partition lifecycle, size, and statistics for automated management';

-- Function to auto-create future partitions
CREATE OR REPLACE FUNCTION partitions.ensure_future_partitions(
    months_ahead INTEGER DEFAULT 3
) RETURNS TABLE(partition_name TEXT, status TEXT) AS $$
DECLARE
    target_month DATE;
    created_partition TEXT;
BEGIN
    -- Get the latest partition end date
    -- Create partitions up to N months ahead
    FOR i IN 1..months_ahead LOOP
        target_month := DATE_TRUNC('month', CURRENT_DATE + (i || ' months')::INTERVAL);

        BEGIN
            created_partition := partitions.create_checkins_partition(target_month);
            partition_name := created_partition;
            status := 'created';
            RETURN NEXT;
        EXCEPTION WHEN duplicate_table THEN
            partition_name := 'checkins_y' || TO_CHAR(target_month, 'YYYY') || '_m' || TO_CHAR(target_month, 'MM');
            status := 'already_exists';
            RETURN NEXT;
        END;
    END LOOP;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION partitions.ensure_future_partitions(INTEGER) IS
'Automatically creates future partitions to ensure continuous operation. Run monthly via cron.';

-- Function to drop old partitions based on retention policy
CREATE OR REPLACE FUNCTION partitions.cleanup_old_partitions(
    retention_months INTEGER DEFAULT 24
) RETURNS TABLE(partition_name TEXT, action TEXT, row_count BIGINT) AS $$
DECLARE
    cutoff_date DATE;
    partition_record RECORD;
BEGIN
    cutoff_date := DATE_TRUNC('month', CURRENT_DATE - (retention_months || ' months')::INTERVAL);

    FOR partition_record IN
        SELECT
            c.relname,
            pg_catalog.pg_get_expr(c.relpartbound, c.oid) as partition_expr
        FROM pg_class c
        JOIN pg_inherits i ON c.oid = i.inhrelid
        JOIN pg_class p ON i.inhparent = p.oid
        WHERE p.relname = 'checkins_partitioned'
            AND c.relname LIKE 'checkins_y%'
            AND c.relname != 'checkins_default'
    LOOP
        -- Extract date from partition name (format: checkins_y2024_m11)
        DECLARE
            year_month TEXT;
            partition_date DATE;
        BEGIN
            -- Parse partition name to get date
            year_month := substring(partition_record.relname from 'y(\d{4})_m(\d{2})');
            partition_date := TO_DATE(
                substring(partition_record.relname from 'y(\d{4})') || '-' ||
                substring(partition_record.relname from 'm(\d{2})') || '-01',
                'YYYY-MM-DD'
            );

            IF partition_date < cutoff_date THEN
                -- Get row count before dropping
                EXECUTE format('SELECT COUNT(*) FROM partitions.%I', partition_record.relname) INTO row_count;

                partition_name := partition_record.relname;
                action := 'dropped';

                -- Drop the partition
                EXECUTE format('DROP TABLE IF EXISTS partitions.%I', partition_record.relname);

                RETURN NEXT;

                RAISE NOTICE 'Dropped partition: % (% rows, older than %)',
                    partition_record.relname, row_count, cutoff_date;
            END IF;
        END;
    END LOOP;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION partitions.cleanup_old_partitions(INTEGER) IS
'Drops partitions older than retention policy. Default: 24 months. Run monthly via cron.';

-- ============================================================================
-- PHASE 4: MIGRATION TRACKING AND MONITORING
-- ============================================================================

-- Table to track migration progress
CREATE TABLE IF NOT EXISTS performance.partition_migration_log (
    id SERIAL PRIMARY KEY,
    migration_phase TEXT NOT NULL,
    rows_migrated BIGINT DEFAULT 0,
    started_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP WITH TIME ZONE,
    duration_seconds INTEGER,
    status TEXT CHECK (status IN ('in_progress', 'completed', 'failed', 'paused')) DEFAULT 'in_progress',
    error_message TEXT,
    notes TEXT
);

COMMENT ON TABLE performance.partition_migration_log IS
'Tracks progress of online migration from checkins to checkins_partitioned';

-- Function to get partition statistics
CREATE OR REPLACE FUNCTION performance.get_partition_stats()
RETURNS TABLE(
    partition_name TEXT,
    partition_start DATE,
    partition_end DATE,
    row_count BIGINT,
    size_mb NUMERIC,
    avg_rows_per_day NUMERIC,
    is_current_month BOOLEAN
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        c.relname::TEXT,
        -- Extract start date from partition constraint
        TO_DATE(
            substring(c.relname from 'y(\d{4})') || '-' ||
            substring(c.relname from 'm(\d{2})') || '-01',
            'YYYY-MM-DD'
        ) as partition_start,
        (TO_DATE(
            substring(c.relname from 'y(\d{4})') || '-' ||
            substring(c.relname from 'm(\d{2})') || '-01',
            'YYYY-MM-DD'
        ) + INTERVAL '1 month' - INTERVAL '1 day')::DATE as partition_end,
        s.n_live_tup as row_count,
        ROUND(pg_total_relation_size(c.oid) / 1024.0 / 1024.0, 2) as size_mb,
        ROUND(s.n_live_tup::NUMERIC / 30.0, 2) as avg_rows_per_day,
        DATE_TRUNC('month', CURRENT_DATE) = TO_DATE(
            substring(c.relname from 'y(\d{4})') || '-' ||
            substring(c.relname from 'm(\d{2})') || '-01',
            'YYYY-MM-DD'
        ) as is_current_month
    FROM pg_class c
    JOIN pg_inherits i ON c.oid = i.inhrelid
    JOIN pg_class p ON i.inhparent = p.oid
    LEFT JOIN pg_stat_user_tables s ON c.oid = s.relid
    WHERE p.relname = 'checkins_partitioned'
        AND c.relname LIKE 'checkins_y%'
        AND c.relname != 'checkins_default'
    ORDER BY partition_start DESC;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION performance.get_partition_stats() IS
'Returns statistics for all checkins partitions including size and row counts';

-- View for monitoring partition health
CREATE OR REPLACE VIEW performance.partition_health AS
SELECT
    ps.*,
    CASE
        WHEN ps.size_mb > 10240 THEN 'WARNING: Partition over 10GB'
        WHEN ps.row_count = 0 AND ps.is_current_month = false THEN 'INFO: Empty partition'
        WHEN ps.avg_rows_per_day < 1 AND ps.is_current_month = false THEN 'INFO: Low activity'
        ELSE 'OK'
    END as health_status,
    CASE
        WHEN ps.is_current_month THEN 'Current - Active writes'
        WHEN ps.partition_start > CURRENT_DATE THEN 'Future - Awaiting data'
        WHEN ps.partition_start < CURRENT_DATE - INTERVAL '12 months' THEN 'Old - Consider archival'
        ELSE 'Historical - Read-only'
    END as partition_type
FROM performance.get_partition_stats() ps;

COMMENT ON VIEW performance.partition_health IS
'Monitor health and status of all checkins partitions';

-- ============================================================================
-- PHASE 5: FOREIGN KEY CONSTRAINTS (Added to partitioned table)
-- ============================================================================

-- Note: Foreign keys must be added AFTER migration is complete
-- For now, we add them but they won't be enforced on the old table

-- These will be enabled after full migration
ALTER TABLE checkins_partitioned
    ADD CONSTRAINT fk_checkins_partitioned_user_id
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE NOT VALID;

ALTER TABLE checkins_partitioned
    ADD CONSTRAINT fk_checkins_partitioned_category_id
    FOREIGN KEY (category_id) REFERENCES categories(id) ON DELETE SET NULL NOT VALID;

-- Validate constraints asynchronously (can be done during low traffic)
-- These are marked NOT VALID initially to avoid locking during migration
-- Run validation after migration: ALTER TABLE checkins_partitioned VALIDATE CONSTRAINT fk_checkins_partitioned_user_id;

COMMENT ON CONSTRAINT fk_checkins_partitioned_user_id ON checkins_partitioned IS
'Foreign key to users table. Added during partitioning migration (Task 18.4).';

COMMENT ON CONSTRAINT fk_checkins_partitioned_category_id ON checkins_partitioned IS
'Foreign key to categories table. Added during partitioning migration (Task 18.4).';

-- ============================================================================
-- PHASE 6: GRANT PERMISSIONS
-- ============================================================================

GRANT SELECT ON performance.partition_metadata TO PUBLIC;
GRANT SELECT ON performance.partition_migration_log TO PUBLIC;
GRANT SELECT ON performance.partition_health TO PUBLIC;
GRANT EXECUTE ON FUNCTION partitions.create_checkins_partition(DATE) TO PUBLIC;
GRANT EXECUTE ON FUNCTION partitions.ensure_future_partitions(INTEGER) TO PUBLIC;
GRANT EXECUTE ON FUNCTION performance.get_partition_stats() TO PUBLIC;

-- Only superuser should be able to drop partitions
-- GRANT EXECUTE ON FUNCTION partitions.cleanup_old_partitions(INTEGER) TO admin_role;

-- ============================================================================
-- PHASE 7: COMPLETION LOG AND NEXT STEPS
-- ============================================================================

DO $$
BEGIN
    RAISE NOTICE '=====================================================';
    RAISE NOTICE 'Partitioning Infrastructure Setup Complete';
    RAISE NOTICE '=====================================================';
    RAISE NOTICE 'Table: checkins_partitioned';
    RAISE NOTICE 'Partition Type: RANGE (monthly)';
    RAISE NOTICE 'Partition Key: checkin_time';
    RAISE NOTICE 'Initial Partitions: 13 (6 past + current + 6 future)';
    RAISE NOTICE '';
    RAISE NOTICE 'IMPORTANT: Migration NOT yet started';
    RAISE NOTICE 'The original checkins table is still in use';
    RAISE NOTICE '';
    RAISE NOTICE 'Next Steps:';
    RAISE NOTICE '1. Run online migration script (see migration documentation)';
    RAISE NOTICE '2. Monitor partition health: SELECT * FROM performance.partition_health;';
    RAISE NOTICE '3. Schedule partition maintenance:';
    RAISE NOTICE '   - Monthly: SELECT * FROM partitions.ensure_future_partitions(3);';
    RAISE NOTICE '   - Monthly: SELECT * FROM partitions.cleanup_old_partitions(24);';
    RAISE NOTICE '4. After migration: Validate FK constraints';
    RAISE NOTICE '5. After migration: Update application to use checkins_partitioned';
    RAISE NOTICE '=====================================================';
END $$;

-- Log the migration setup
INSERT INTO performance.partition_migration_log (migration_phase, status, notes)
VALUES ('setup_partitioned_table', 'completed', 'Created checkins_partitioned with 13 initial partitions');
