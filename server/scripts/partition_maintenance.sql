-- ============================================================================
-- PARTITION MAINTENANCE AUTOMATION
-- ============================================================================
-- Task: 18.4 - 체크인 테이블 날짜 기준 파티셔닝 설계 및 온라인 마이그레이션
-- Purpose: Automated partition creation and cleanup
--
-- USAGE:
--   Run this script monthly via cron or scheduled job
--   Example crontab: 0 2 1 * * psql -d donelist -f partition_maintenance.sql
--
-- WHAT IT DOES:
--   1. Creates future partitions (3 months ahead)
--   2. Cleans up old partitions (older than retention policy)
--   3. Updates partition metadata
--   4. Analyzes partition statistics
--   5. Generates health report
-- ============================================================================

\set retention_months 24
\set future_months 3

-- ============================================================================
-- STEP 1: CREATE FUTURE PARTITIONS
-- ============================================================================

DO $$
DECLARE
    result RECORD;
    created_count INTEGER := 0;
    exists_count INTEGER := 0;
BEGIN
    RAISE NOTICE '=====================================================';
    RAISE NOTICE 'Partition Maintenance - %', CURRENT_TIMESTAMP;
    RAISE NOTICE '=====================================================';
    RAISE NOTICE 'Step 1: Creating Future Partitions';
    RAISE NOTICE '-----------------------------------------------------';

    FOR result IN
        SELECT * FROM partitions.ensure_future_partitions(3)
    LOOP
        IF result.status = 'created' THEN
            created_count := created_count + 1;
            RAISE NOTICE 'Created: %', result.partition_name;
        ELSE
            exists_count := exists_count + 1;
        END IF;
    END LOOP;

    RAISE NOTICE 'Summary: % new partitions created, % already existed',
        created_count, exists_count;
    RAISE NOTICE '';
END $$;

-- ============================================================================
-- STEP 2: UPDATE PARTITION METADATA
-- ============================================================================

DO $$
DECLARE
    partition_record RECORD;
    rows BIGINT;
    size_bytes BIGINT;
    partition_start DATE;
    partition_end DATE;
BEGIN
    RAISE NOTICE 'Step 2: Updating Partition Metadata';
    RAISE NOTICE '-----------------------------------------------------';

    FOR partition_record IN
        SELECT
            c.relname as partition_name,
            s.n_live_tup as row_count,
            pg_total_relation_size(c.oid) as size_bytes
        FROM pg_class c
        JOIN pg_inherits i ON c.oid = i.inhrelid
        JOIN pg_class p ON i.inhparent = p.oid
        LEFT JOIN pg_stat_user_tables s ON c.oid = s.relid
        WHERE p.relname = 'checkins_partitioned'
            AND c.relname LIKE 'checkins_y%'
            AND c.relname != 'checkins_default'
    LOOP
        -- Parse partition date from name
        partition_start := TO_DATE(
            substring(partition_record.partition_name from 'y(\d{4})') || '-' ||
            substring(partition_record.partition_name from 'm(\d{2})') || '-01',
            'YYYY-MM-DD'
        );
        partition_end := (partition_start + INTERVAL '1 month' - INTERVAL '1 day')::DATE;

        -- Insert or update metadata
        INSERT INTO performance.partition_metadata (
            table_name,
            partition_name,
            partition_start,
            partition_end,
            row_count,
            size_bytes,
            last_analyzed,
            status
        )
        VALUES (
            'checkins_partitioned',
            partition_record.partition_name,
            partition_start,
            partition_end,
            partition_record.row_count,
            partition_record.size_bytes,
            CURRENT_TIMESTAMP,
            'active'
        )
        ON CONFLICT (partition_name) DO UPDATE SET
            row_count = EXCLUDED.row_count,
            size_bytes = EXCLUDED.size_bytes,
            last_analyzed = CURRENT_TIMESTAMP;
    END LOOP;

    RAISE NOTICE 'Partition metadata updated';
    RAISE NOTICE '';
END $$;

-- ============================================================================
-- STEP 3: ANALYZE PARTITIONS
-- ============================================================================

DO $$
DECLARE
    partition_record RECORD;
    analyzed_count INTEGER := 0;
BEGIN
    RAISE NOTICE 'Step 3: Analyzing Partitions';
    RAISE NOTICE '-----------------------------------------------------';

    FOR partition_record IN
        SELECT tablename
        FROM pg_tables
        WHERE schemaname = 'partitions'
            AND tablename LIKE 'checkins_y%'
    LOOP
        EXECUTE format('ANALYZE partitions.%I', partition_record.tablename);
        analyzed_count := analyzed_count + 1;

        -- Only log every 10th partition
        IF analyzed_count % 10 = 0 THEN
            RAISE NOTICE 'Analyzed % partitions...', analyzed_count;
        END IF;
    END LOOP;

    RAISE NOTICE 'Analyzed % partitions total', analyzed_count;
    RAISE NOTICE '';
END $$;

-- ============================================================================
-- STEP 4: CLEANUP OLD PARTITIONS
-- ============================================================================

DO $$
DECLARE
    result RECORD;
    dropped_count INTEGER := 0;
    total_rows_dropped BIGINT := 0;
    retention_months INTEGER := 24;
BEGIN
    RAISE NOTICE 'Step 4: Cleaning Up Old Partitions';
    RAISE NOTICE 'Retention Policy: % months', retention_months;
    RAISE NOTICE '-----------------------------------------------------';

    -- Note: This is a dry run by default - uncomment to actually drop
    -- FOR result IN
    --     SELECT * FROM partitions.cleanup_old_partitions(retention_months)
    -- LOOP
    --     dropped_count := dropped_count + 1;
    --     total_rows_dropped := total_rows_dropped + result.row_count;
    --     RAISE NOTICE 'Dropped: % (% rows)', result.partition_name, result.row_count;
    -- END LOOP;

    -- IF dropped_count > 0 THEN
    --     RAISE NOTICE 'Summary: % partitions dropped, % total rows removed',
    --         dropped_count, total_rows_dropped;
    -- ELSE
    --     RAISE NOTICE 'No partitions to drop';
    -- END IF;

    -- Dry run - show what would be dropped
    RAISE NOTICE 'DRY RUN: Showing partitions that would be dropped';
    FOR result IN
        SELECT
            pm.partition_name,
            pm.row_count,
            pm.partition_start,
            CURRENT_DATE - pm.partition_start as age_days
        FROM performance.partition_metadata pm
        WHERE pm.partition_start < DATE_TRUNC('month', CURRENT_DATE - (retention_months || ' months')::INTERVAL)
            AND pm.status = 'active'
        ORDER BY pm.partition_start
    LOOP
        RAISE NOTICE 'Would drop: % (% rows, % days old)',
            result.partition_name,
            result.row_count,
            result.age_days;
        dropped_count := dropped_count + 1;
    END LOOP;

    IF dropped_count = 0 THEN
        RAISE NOTICE 'No partitions older than retention policy';
    ELSE
        RAISE NOTICE '';
        RAISE NOTICE 'To actually drop these partitions, uncomment the cleanup code';
        RAISE NOTICE 'in partition_maintenance.sql and re-run';
    END IF;

    RAISE NOTICE '';
END $$;

-- ============================================================================
-- STEP 5: PARTITION HEALTH REPORT
-- ============================================================================

DO $$
DECLARE
    report_record RECORD;
    total_partitions INTEGER := 0;
    total_rows BIGINT := 0;
    total_size_mb NUMERIC := 0;
    warning_count INTEGER := 0;
BEGIN
    RAISE NOTICE 'Step 5: Partition Health Report';
    RAISE NOTICE '=====================================================';

    -- Summary statistics
    SELECT
        COUNT(*),
        SUM(row_count),
        SUM(size_mb)
    INTO
        total_partitions,
        total_rows,
        total_size_mb
    FROM performance.get_partition_stats();

    RAISE NOTICE 'Total Partitions: %', total_partitions;
    RAISE NOTICE 'Total Rows: %', total_rows;
    RAISE NOTICE 'Total Size: % MB (% GB)', total_size_mb, ROUND(total_size_mb / 1024.0, 2);
    RAISE NOTICE '';

    -- Detailed partition health
    RAISE NOTICE 'Partition Details:';
    RAISE NOTICE '%-25s %-15s %-12s %-12s %s',
        'Partition', 'Date Range', 'Rows', 'Size (MB)', 'Status';
    RAISE NOTICE '%',
        REPEAT('-', 90);

    FOR report_record IN
        SELECT
            ps.partition_name,
            TO_CHAR(ps.partition_start, 'YYYY-MM') as month,
            ps.row_count,
            ps.size_mb,
            ph.health_status,
            ph.partition_type
        FROM performance.get_partition_stats() ps
        LEFT JOIN performance.partition_health ph ON ph.partition_name = ps.partition_name
        ORDER BY ps.partition_start DESC
        LIMIT 12  -- Show last 12 months
    LOOP
        RAISE NOTICE '%-25s %-15s %12s %12s %s',
            report_record.partition_name,
            report_record.month,
            COALESCE(report_record.row_count::TEXT, '0'),
            COALESCE(ROUND(report_record.size_mb, 2)::TEXT, '0'),
            report_record.partition_type;

        -- Count warnings
        IF report_record.health_status NOT LIKE 'OK%' AND report_record.health_status NOT LIKE 'INFO%' THEN
            warning_count := warning_count + 1;
        END IF;
    END LOOP;

    RAISE NOTICE '';

    -- Check for warnings
    IF warning_count > 0 THEN
        RAISE WARNING '% partition(s) have health warnings - review performance.partition_health view',
            warning_count;
    END IF;

    -- Check for data in default partition (should be empty)
    DECLARE
        default_count BIGINT;
    BEGIN
        SELECT COUNT(*) INTO default_count FROM partitions.checkins_default;
        IF default_count > 0 THEN
            RAISE WARNING 'Default partition contains % rows - investigate date ranges', default_count;
        END IF;
    END;

    RAISE NOTICE '';
END $$;

-- ============================================================================
-- STEP 6: INDEX USAGE ANALYSIS
-- ============================================================================

DO $$
DECLARE
    index_record RECORD;
    unused_count INTEGER := 0;
BEGIN
    RAISE NOTICE 'Step 6: Index Usage on Partitions';
    RAISE NOTICE '=====================================================';

    -- Check for unused indexes on partitions
    FOR index_record IN
        SELECT
            schemaname,
            tablename,
            indexrelname,
            idx_scan,
            pg_size_pretty(pg_relation_size(indexrelid)) as index_size
        FROM pg_stat_user_indexes
        WHERE schemaname = 'partitions'
            AND tablename LIKE 'checkins_y%'
            AND idx_scan = 0
        ORDER BY pg_relation_size(indexrelid) DESC
        LIMIT 10
    LOOP
        unused_count := unused_count + 1;
        IF unused_count = 1 THEN
            RAISE NOTICE 'Unused Indexes (Top 10 by size):';
        END IF;
        RAISE NOTICE '  %: % (never scanned, size: %)',
            index_record.tablename,
            index_record.indexrelname,
            index_record.index_size;
    END LOOP;

    IF unused_count = 0 THEN
        RAISE NOTICE 'All partition indexes are being used';
    ELSE
        RAISE NOTICE '';
        RAISE NOTICE 'Review unused indexes - may indicate inefficient queries';
    END IF;

    RAISE NOTICE '';
END $$;

-- ============================================================================
-- STEP 7: PERFORMANCE RECOMMENDATIONS
-- ============================================================================

DO $$
DECLARE
    avg_size_mb NUMERIC;
    max_size_mb NUMERIC;
    largest_partition TEXT;
BEGIN
    RAISE NOTICE 'Step 7: Performance Recommendations';
    RAISE NOTICE '=====================================================';

    -- Calculate average and max partition size
    SELECT
        AVG(size_mb),
        MAX(size_mb)
    INTO
        avg_size_mb,
        max_size_mb
    FROM performance.get_partition_stats()
    WHERE row_count > 0;

    SELECT partition_name
    INTO largest_partition
    FROM performance.get_partition_stats()
    WHERE size_mb = max_size_mb
    LIMIT 1;

    RAISE NOTICE 'Average partition size: % MB', ROUND(avg_size_mb, 2);
    RAISE NOTICE 'Largest partition: % (% MB)', largest_partition, ROUND(max_size_mb, 2);
    RAISE NOTICE '';

    -- Recommendations
    IF max_size_mb > 10240 THEN  -- 10 GB
        RAISE WARNING 'Largest partition exceeds 10GB - consider sub-partitioning or data archival';
    ELSIF max_size_mb > 5120 THEN  -- 5 GB
        RAISE NOTICE 'Recommendation: Monitor partition growth - approaching 5GB threshold';
    ELSE
        RAISE NOTICE 'Partition sizes are within healthy range';
    END IF;

    -- Growth rate analysis
    DECLARE
        current_month_rows BIGINT;
        last_month_rows BIGINT;
        growth_rate NUMERIC;
    BEGIN
        SELECT row_count INTO current_month_rows
        FROM performance.get_partition_stats()
        WHERE partition_start = DATE_TRUNC('month', CURRENT_DATE)::DATE;

        SELECT row_count INTO last_month_rows
        FROM performance.get_partition_stats()
        WHERE partition_start = (DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '1 month')::DATE;

        IF current_month_rows IS NOT NULL AND last_month_rows IS NOT NULL AND last_month_rows > 0 THEN
            growth_rate := ((current_month_rows::NUMERIC - last_month_rows) / last_month_rows) * 100;
            RAISE NOTICE 'Month-over-month growth: %% (current: %, last: %)',
                ROUND(growth_rate, 2),
                current_month_rows,
                last_month_rows;
        END IF;
    END;

    RAISE NOTICE '';
END $$;

-- ============================================================================
-- COMPLETION
-- ============================================================================

DO $$
BEGIN
    RAISE NOTICE '=====================================================';
    RAISE NOTICE 'Partition Maintenance Complete';
    RAISE NOTICE 'Completed at: %', CURRENT_TIMESTAMP;
    RAISE NOTICE '=====================================================';
    RAISE NOTICE '';
    RAISE NOTICE 'Next Steps:';
    RAISE NOTICE '1. Review health report above for any warnings';
    RAISE NOTICE '2. Check partition_metadata table for detailed stats';
    RAISE NOTICE '3. Monitor query performance using EXPLAIN ANALYZE';
    RAISE NOTICE '4. Schedule this script to run monthly';
    RAISE NOTICE '';
    RAISE NOTICE 'For detailed analysis:';
    RAISE NOTICE '  SELECT * FROM performance.partition_health;';
    RAISE NOTICE '  SELECT * FROM performance.analyze_index_effectiveness();';
    RAISE NOTICE '=====================================================';
END $$;
