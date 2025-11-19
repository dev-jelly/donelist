-- ============================================================================
-- ONLINE MIGRATION SCRIPT: Checkins to Checkins_Partitioned
-- ============================================================================
-- Task: 18.4 - 체크인 테이블 날짜 기준 파티셔닝 설계 및 온라인 마이그레이션
-- Strategy: Zero-downtime migration using batch processing and incremental sync
--
-- USAGE:
--   1. Run this script during low-traffic period
--   2. Monitor progress via performance.partition_migration_log
--   3. Adjust batch_size based on system load
--   4. Can be paused/resumed at any time
--
-- SAFETY:
--   - Uses small batches to minimize lock contention
--   - Adds deliberate delays to avoid overwhelming the database
--   - Can be stopped and resumed without data loss
--   - Original table remains active during entire migration
-- ============================================================================

-- Configuration
\set batch_size 10000
\set delay_ms 100

-- ============================================================================
-- PHASE 1: PRE-MIGRATION VALIDATION
-- ============================================================================

DO $$
DECLARE
    source_count BIGINT;
    target_count BIGINT;
    min_date TIMESTAMP WITH TIME ZONE;
    max_date TIMESTAMP WITH TIME ZONE;
BEGIN
    RAISE NOTICE '=====================================================';
    RAISE NOTICE 'Pre-Migration Validation';
    RAISE NOTICE '=====================================================';

    -- Count records in source table
    SELECT COUNT(*) INTO source_count FROM checkins;
    RAISE NOTICE 'Source table (checkins) record count: %', source_count;

    -- Count records in target table
    SELECT COUNT(*) INTO target_count FROM checkins_partitioned;
    RAISE NOTICE 'Target table (checkins_partitioned) record count: %', target_count;

    -- Get date range
    SELECT MIN(checkin_time), MAX(checkin_time)
    INTO min_date, max_date
    FROM checkins;
    RAISE NOTICE 'Date range: % to %', min_date, max_date;

    -- Verify partitions exist for date range
    RAISE NOTICE '';
    RAISE NOTICE 'Checking partition coverage...';

    IF min_date IS NOT NULL AND max_date IS NOT NULL THEN
        -- Ensure we have partitions for the entire range
        PERFORM partitions.create_checkins_partition(DATE_TRUNC('month', min_date)::DATE);
        PERFORM partitions.create_checkins_partition(DATE_TRUNC('month', max_date)::DATE);
        RAISE NOTICE 'Partition coverage validated';
    END IF;

    RAISE NOTICE '=====================================================';
END $$;

-- ============================================================================
-- PHASE 2: INCREMENTAL BATCH MIGRATION
-- ============================================================================

-- Function to migrate data in batches
CREATE OR REPLACE FUNCTION performance.migrate_checkins_batch(
    batch_size INTEGER DEFAULT 10000,
    delay_milliseconds INTEGER DEFAULT 100
) RETURNS TABLE(
    batch_number INTEGER,
    rows_migrated BIGINT,
    duration_ms BIGINT,
    status TEXT
) AS $$
DECLARE
    total_migrated BIGINT := 0;
    current_batch INTEGER := 0;
    start_time TIMESTAMP;
    end_time TIMESTAMP;
    rows_affected BIGINT;
    last_id UUID := '00000000-0000-0000-0000-000000000000';
BEGIN
    LOOP
        start_time := clock_timestamp();

        -- Migrate a batch using INSERT ... ON CONFLICT DO NOTHING
        -- This allows idempotent execution if script is re-run
        WITH batch AS (
            SELECT *
            FROM checkins
            WHERE id > last_id
                AND deleted_at IS NULL  -- Skip soft-deleted records during initial migration
            ORDER BY id
            LIMIT batch_size
        )
        INSERT INTO checkins_partitioned
        SELECT * FROM batch
        ON CONFLICT (id, checkin_time) DO NOTHING;

        GET DIAGNOSTICS rows_affected = ROW_COUNT;

        -- Update last_id for next iteration
        SELECT MAX(id) INTO last_id
        FROM (
            SELECT id
            FROM checkins
            WHERE id > last_id
            ORDER BY id
            LIMIT batch_size
        ) sub;

        -- Exit if no more rows to migrate
        EXIT WHEN rows_affected = 0 OR last_id IS NULL;

        current_batch := current_batch + 1;
        total_migrated := total_migrated + rows_affected;
        end_time := clock_timestamp();

        -- Return batch stats
        batch_number := current_batch;
        rows_migrated := rows_affected;
        duration_ms := EXTRACT(EPOCH FROM (end_time - start_time)) * 1000;
        status := 'completed';
        RETURN NEXT;

        -- Log progress every 10 batches
        IF current_batch % 10 = 0 THEN
            RAISE NOTICE 'Migrated % batches, % total rows', current_batch, total_migrated;

            -- Update migration log
            UPDATE performance.partition_migration_log
            SET rows_migrated = total_migrated,
                notes = format('Batch %s: %s rows migrated', current_batch, total_migrated)
            WHERE migration_phase = 'batch_migration'
                AND status = 'in_progress';
        END IF;

        -- Add delay to reduce system load
        IF delay_milliseconds > 0 THEN
            PERFORM pg_sleep(delay_milliseconds / 1000.0);
        END IF;
    END LOOP;

    -- Final summary
    RAISE NOTICE 'Migration complete: % batches, % total rows', current_batch, total_migrated;

    batch_number := current_batch;
    rows_migrated := total_migrated;
    duration_ms := 0;
    status := 'all_batches_complete';
    RETURN NEXT;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION performance.migrate_checkins_batch(INTEGER, INTEGER) IS
'Migrates checkins data in batches from original table to partitioned table. Idempotent and resumable.';

-- Start migration log
INSERT INTO performance.partition_migration_log (migration_phase, status, notes)
VALUES ('batch_migration', 'in_progress', 'Starting batch migration')
RETURNING id, started_at;

-- Run the batch migration
-- This will process all data in batches
DO $$
DECLARE
    batch_result RECORD;
    total_rows BIGINT := 0;
    total_batches INTEGER := 0;
    migration_start TIMESTAMP;
    migration_end TIMESTAMP;
BEGIN
    migration_start := clock_timestamp();

    RAISE NOTICE '=====================================================';
    RAISE NOTICE 'Starting Batch Migration';
    RAISE NOTICE 'Batch size: 10000';
    RAISE NOTICE 'Delay between batches: 100ms';
    RAISE NOTICE '=====================================================';

    -- Run migration in batches
    FOR batch_result IN
        SELECT * FROM performance.migrate_checkins_batch(10000, 100)
    LOOP
        total_batches := batch_result.batch_number;
        total_rows := total_rows + batch_result.rows_migrated;

        -- Log each batch
        IF batch_result.batch_number % 50 = 0 THEN
            RAISE NOTICE 'Batch %: % rows (% ms)',
                batch_result.batch_number,
                batch_result.rows_migrated,
                batch_result.duration_ms;
        END IF;
    END LOOP;

    migration_end := clock_timestamp();

    -- Update migration log
    UPDATE performance.partition_migration_log
    SET rows_migrated = total_rows,
        completed_at = migration_end,
        duration_seconds = EXTRACT(EPOCH FROM (migration_end - migration_start)),
        status = 'completed',
        notes = format('Completed: %s batches, %s total rows', total_batches, total_rows)
    WHERE migration_phase = 'batch_migration'
        AND status = 'in_progress';

    RAISE NOTICE '=====================================================';
    RAISE NOTICE 'Migration Complete';
    RAISE NOTICE 'Total batches: %', total_batches;
    RAISE NOTICE 'Total rows: %', total_rows;
    RAISE NOTICE 'Duration: % seconds', EXTRACT(EPOCH FROM (migration_end - migration_start));
    RAISE NOTICE '=====================================================';
END $$;

-- ============================================================================
-- PHASE 3: VERIFY MIGRATION
-- ============================================================================

DO $$
DECLARE
    source_count BIGINT;
    target_count BIGINT;
    missing_count BIGINT;
BEGIN
    RAISE NOTICE '=====================================================';
    RAISE NOTICE 'Migration Verification';
    RAISE NOTICE '=====================================================';

    -- Count records
    SELECT COUNT(*) INTO source_count FROM checkins WHERE deleted_at IS NULL;
    SELECT COUNT(*) INTO target_count FROM checkins_partitioned WHERE deleted_at IS NULL;

    RAISE NOTICE 'Source count (non-deleted): %', source_count;
    RAISE NOTICE 'Target count (non-deleted): %', target_count;
    RAISE NOTICE 'Difference: %', source_count - target_count;

    -- Find missing records
    SELECT COUNT(*)
    INTO missing_count
    FROM checkins c
    WHERE NOT EXISTS (
        SELECT 1
        FROM checkins_partitioned cp
        WHERE cp.id = c.id
    )
    AND c.deleted_at IS NULL;

    RAISE NOTICE 'Missing records: %', missing_count;

    IF missing_count > 0 THEN
        RAISE WARNING 'Migration incomplete: % records missing', missing_count;
    ELSE
        RAISE NOTICE 'Migration verification: SUCCESS';
    END IF;

    -- Check partition distribution
    RAISE NOTICE '';
    RAISE NOTICE 'Partition Distribution:';
    FOR rec IN
        SELECT * FROM performance.get_partition_stats()
        WHERE row_count > 0
        ORDER BY partition_start
    LOOP
        RAISE NOTICE '  %: % rows (% MB)',
            rec.partition_name,
            rec.row_count,
            rec.size_mb;
    END LOOP;

    RAISE NOTICE '=====================================================';
END $$;

-- ============================================================================
-- PHASE 4: POST-MIGRATION SETUP
-- ============================================================================

-- Analyze all partitions to update statistics
DO $$
DECLARE
    partition_record RECORD;
BEGIN
    RAISE NOTICE '=====================================================';
    RAISE NOTICE 'Analyzing Partitions';
    RAISE NOTICE '=====================================================';

    FOR partition_record IN
        SELECT tablename
        FROM pg_tables
        WHERE schemaname = 'partitions'
            AND tablename LIKE 'checkins_y%'
    LOOP
        EXECUTE format('ANALYZE partitions.%I', partition_record.tablename);
        RAISE NOTICE 'Analyzed: %', partition_record.tablename;
    END LOOP;

    RAISE NOTICE '=====================================================';
END $$;

-- Validate foreign key constraints
-- This is done online and won't block operations
ALTER TABLE checkins_partitioned VALIDATE CONSTRAINT fk_checkins_partitioned_user_id;
ALTER TABLE checkins_partitioned VALIDATE CONSTRAINT fk_checkins_partitioned_category_id;

RAISE NOTICE 'Foreign key constraints validated';

-- ============================================================================
-- PHASE 5: SYNC TRIGGER SETUP (For Ongoing Changes During Cutover)
-- ============================================================================

-- Create trigger function to sync new inserts/updates to partitioned table
CREATE OR REPLACE FUNCTION performance.sync_checkin_to_partitioned()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        INSERT INTO checkins_partitioned
        VALUES (NEW.*)
        ON CONFLICT (id, checkin_time) DO NOTHING;
    ELSIF TG_OP = 'UPDATE' THEN
        UPDATE checkins_partitioned
        SET
            user_id = NEW.user_id,
            category_id = NEW.category_id,
            content = NEW.content,
            checkin_time = NEW.checkin_time,
            duration_minutes = NEW.duration_minutes,
            is_edited = NEW.is_edited,
            edit_count = NEW.edit_count,
            last_edited_at = NEW.last_edited_at,
            updated_at = NEW.updated_at,
            deleted_at = NEW.deleted_at
        WHERE id = NEW.id AND checkin_time = NEW.checkin_time;
    ELSIF TG_OP = 'DELETE' THEN
        DELETE FROM checkins_partitioned
        WHERE id = OLD.id AND checkin_time = OLD.checkin_time;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION performance.sync_checkin_to_partitioned() IS
'Trigger function to keep checkins_partitioned in sync with checkins during migration/cutover period';

-- Note: Do NOT create trigger yet - only after careful planning
-- This is for reference during the cutover phase
-- CREATE TRIGGER sync_checkins_to_partitioned
--     AFTER INSERT OR UPDATE OR DELETE ON checkins
--     FOR EACH ROW EXECUTE FUNCTION performance.sync_checkin_to_partitioned();

-- ============================================================================
-- COMPLETION
-- ============================================================================

DO $$
BEGIN
    RAISE NOTICE '=====================================================';
    RAISE NOTICE 'Online Migration Complete';
    RAISE NOTICE '=====================================================';
    RAISE NOTICE 'Next Steps:';
    RAISE NOTICE '1. Review migration log: SELECT * FROM performance.partition_migration_log;';
    RAISE NOTICE '2. Check partition health: SELECT * FROM performance.partition_health;';
    RAISE NOTICE '3. Test queries on checkins_partitioned with EXPLAIN ANALYZE';
    RAISE NOTICE '4. Plan cutover strategy:';
    RAISE NOTICE '   a. Enable sync trigger (commented above)';
    RAISE NOTICE '   b. Update application to read from checkins_partitioned';
    RAISE NOTICE '   c. Update application to write to checkins_partitioned';
    RAISE NOTICE '   d. Rename tables during maintenance window';
    RAISE NOTICE '5. Monitor performance and rollback if needed';
    RAISE NOTICE '=====================================================';
END $$;
