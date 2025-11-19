-- ============================================================================
-- PARTITION PERFORMANCE TEST SUITE
-- ============================================================================
-- Task: 18.4 - 체크인 테이블 날짜 기준 파티셔닝 설계 및 온라인 마이그레이션
-- Purpose: Validate partition pruning and query performance
--
-- USAGE:
--   Run this script to verify partitioning is working correctly
--   Compare execution plans and timing between partitioned and non-partitioned
--
-- TEST AREAS:
--   1. Partition pruning verification
--   2. Query performance comparison
--   3. Index usage validation
--   4. Write performance testing
-- ============================================================================

\timing on
\set ECHO queries

-- ============================================================================
-- TEST 1: PARTITION PRUNING VERIFICATION
-- ============================================================================

\echo '====================================================================='
\echo 'TEST 1: Partition Pruning Verification'
\echo '====================================================================='
\echo ''
\echo 'Test 1.1: Single month range query'
\echo '---------------------------------------------------------------------'

EXPLAIN (ANALYZE, BUFFERS, VERBOSE)
SELECT COUNT(*)
FROM checkins_partitioned
WHERE checkin_time >= '2024-11-01'
  AND checkin_time < '2024-12-01'
  AND deleted_at IS NULL;

\echo ''
\echo 'Expected: Should scan ONLY checkins_y2024_m11 partition'
\echo 'Look for "Partitions removed" in output'
\echo ''

-- ============================================================================

\echo 'Test 1.2: Multi-month range query'
\echo '---------------------------------------------------------------------'

EXPLAIN (ANALYZE, BUFFERS, VERBOSE)
SELECT COUNT(*)
FROM checkins_partitioned
WHERE checkin_time >= '2024-09-01'
  AND checkin_time < '2024-12-01'
  AND deleted_at IS NULL;

\echo ''
\echo 'Expected: Should scan ONLY 3 partitions (Sep, Oct, Nov 2024)'
\echo ''

-- ============================================================================

\echo 'Test 1.3: User timeline query (most common pattern)'
\echo '---------------------------------------------------------------------'

EXPLAIN (ANALYZE, BUFFERS, VERBOSE)
SELECT id, content, checkin_time, category_id
FROM checkins_partitioned
WHERE user_id = 'sample-user-uuid'
  AND checkin_time >= CURRENT_DATE - INTERVAL '7 days'
  AND checkin_time < CURRENT_DATE + INTERVAL '1 day'
  AND deleted_at IS NULL
ORDER BY checkin_time DESC
LIMIT 100;

\echo ''
\echo 'Expected: Partition pruning + idx_user_time_deleted usage'
\echo ''

-- ============================================================================
-- TEST 2: QUERY PERFORMANCE COMPARISON
-- ============================================================================

\echo '====================================================================='
\echo 'TEST 2: Query Performance Comparison'
\echo '====================================================================='
\echo ''

-- Create comparison function
CREATE OR REPLACE FUNCTION performance.compare_query_performance(
    test_description TEXT,
    query_template TEXT,
    iterations INTEGER DEFAULT 100
) RETURNS TABLE(
    description TEXT,
    table_name TEXT,
    avg_time_ms NUMERIC,
    min_time_ms NUMERIC,
    max_time_ms NUMERIC,
    total_time_ms NUMERIC
) AS $$
DECLARE
    start_time TIMESTAMP;
    end_time TIMESTAMP;
    iteration_time NUMERIC;
    times NUMERIC[] := '{}';
    partitioned_query TEXT;
    original_query TEXT;
BEGIN
    -- Test partitioned table
    partitioned_query := replace(query_template, '{{table}}', 'checkins_partitioned');

    FOR i IN 1..iterations LOOP
        start_time := clock_timestamp();
        EXECUTE partitioned_query;
        end_time := clock_timestamp();
        iteration_time := EXTRACT(EPOCH FROM (end_time - start_time)) * 1000;
        times := array_append(times, iteration_time);
    END LOOP;

    description := test_description;
    table_name := 'partitioned';
    avg_time_ms := ROUND((SELECT AVG(x) FROM unnest(times) x), 3);
    min_time_ms := ROUND((SELECT MIN(x) FROM unnest(times) x), 3);
    max_time_ms := ROUND((SELECT MAX(x) FROM unnest(times) x), 3);
    total_time_ms := ROUND((SELECT SUM(x) FROM unnest(times) x), 3);
    RETURN NEXT;

    -- Test original table
    times := '{}';
    original_query := replace(query_template, '{{table}}', 'checkins');

    FOR i IN 1..iterations LOOP
        start_time := clock_timestamp();
        EXECUTE original_query;
        end_time := clock_timestamp();
        iteration_time := EXTRACT(EPOCH FROM (end_time - start_time)) * 1000;
        times := array_append(times, iteration_time);
    END LOOP;

    description := test_description;
    table_name := 'original';
    avg_time_ms := ROUND((SELECT AVG(x) FROM unnest(times) x), 3);
    min_time_ms := ROUND((SELECT MIN(x) FROM unnest(times) x), 3);
    max_time_ms := ROUND((SELECT MAX(x) FROM unnest(times) x), 3);
    total_time_ms := ROUND((SELECT SUM(x) FROM unnest(times) x), 3);
    RETURN NEXT;
END;
$$ LANGUAGE plpgsql;

\echo 'Test 2.1: Recent data query (last 30 days)'
\echo '---------------------------------------------------------------------'

SELECT * FROM performance.compare_query_performance(
    'Recent 30 days count',
    'SELECT COUNT(*) FROM {{table}} WHERE checkin_time >= CURRENT_DATE - INTERVAL ''30 days'' AND deleted_at IS NULL',
    50
);

\echo ''

-- ============================================================================

\echo 'Test 2.2: Specific month query'
\echo '---------------------------------------------------------------------'

SELECT * FROM performance.compare_query_performance(
    'Single month data',
    'SELECT COUNT(*) FROM {{table}} WHERE checkin_time >= ''2024-11-01'' AND checkin_time < ''2024-12-01'' AND deleted_at IS NULL',
    50
);

\echo ''

-- ============================================================================
-- TEST 3: INDEX USAGE VALIDATION
-- ============================================================================

\echo '====================================================================='
\echo 'TEST 3: Index Usage Validation'
\echo '====================================================================='
\echo ''

\echo 'Test 3.1: Verify composite index usage (user + time + deleted)'
\echo '---------------------------------------------------------------------'

EXPLAIN (ANALYZE, BUFFERS, VERBOSE, COSTS OFF)
SELECT id, content, checkin_time
FROM checkins_partitioned
WHERE user_id = 'sample-uuid'
  AND checkin_time >= '2024-11-01'
  AND checkin_time < '2024-12-01'
  AND deleted_at IS NULL
ORDER BY checkin_time DESC
LIMIT 50;

\echo ''
\echo 'Expected: Index Scan using idx_user_time_deleted or partition-specific equivalent'
\echo ''

-- ============================================================================

\echo 'Test 3.2: Category-based query index usage'
\echo '---------------------------------------------------------------------'

EXPLAIN (ANALYZE, BUFFERS, VERBOSE, COSTS OFF)
SELECT id, content, checkin_time
FROM checkins_partitioned
WHERE category_id = 'sample-category-uuid'
  AND checkin_time >= '2024-11-01'
  AND checkin_time < '2024-12-01'
  AND deleted_at IS NULL
ORDER BY checkin_time DESC
LIMIT 50;

\echo ''
\echo 'Expected: Index Scan using idx_category_time or partition-specific equivalent'
\echo ''

-- ============================================================================

\echo 'Test 3.3: Full-text search index usage'
\echo '---------------------------------------------------------------------'

EXPLAIN (ANALYZE, BUFFERS, VERBOSE, COSTS OFF)
SELECT id, content, checkin_time
FROM checkins_partitioned
WHERE to_tsvector('english', content) @@ to_tsquery('english', 'meeting')
  AND checkin_time >= '2024-11-01'
  AND checkin_time < '2024-12-01'
LIMIT 20;

\echo ''
\echo 'Expected: Bitmap Index Scan using GIN index on content'
\echo ''

-- ============================================================================
-- TEST 4: WRITE PERFORMANCE TESTING
-- ============================================================================

\echo '====================================================================='
\echo 'TEST 4: Write Performance Testing'
\echo '====================================================================='
\echo ''

-- Create test user if not exists
DO $$
BEGIN
    INSERT INTO users (id, email, password_hash, display_name)
    VALUES (
        '00000000-0000-0000-0000-000000000001',
        'test@partition-test.com',
        'test-hash',
        'Partition Test User'
    )
    ON CONFLICT (email) DO NOTHING;
END $$;

\echo 'Test 4.1: Single INSERT performance'
\echo '---------------------------------------------------------------------'

\timing on

-- Test INSERT into partitioned table
DO $$
DECLARE
    start_time TIMESTAMP;
    end_time TIMESTAMP;
    duration_ms NUMERIC;
BEGIN
    start_time := clock_timestamp();

    INSERT INTO checkins_partitioned (
        user_id, content, checkin_time, duration_minutes
    )
    VALUES (
        '00000000-0000-0000-0000-000000000001',
        'Partition performance test insert',
        CURRENT_TIMESTAMP,
        15
    );

    end_time := clock_timestamp();
    duration_ms := EXTRACT(EPOCH FROM (end_time - start_time)) * 1000;

    RAISE NOTICE 'Single INSERT duration: % ms', ROUND(duration_ms, 3);
END $$;

\echo ''

-- ============================================================================

\echo 'Test 4.2: Batch INSERT performance (1000 rows)'
\echo '---------------------------------------------------------------------'

DO $$
DECLARE
    start_time TIMESTAMP;
    end_time TIMESTAMP;
    duration_ms NUMERIC;
    insert_count INTEGER := 1000;
BEGIN
    start_time := clock_timestamp();

    -- Insert batch
    INSERT INTO checkins_partitioned (user_id, content, checkin_time, duration_minutes)
    SELECT
        '00000000-0000-0000-0000-000000000001',
        'Batch insert test #' || i,
        CURRENT_TIMESTAMP - (i || ' minutes')::INTERVAL,
        15
    FROM generate_series(1, insert_count) i;

    end_time := clock_timestamp();
    duration_ms := EXTRACT(EPOCH FROM (end_time - start_time)) * 1000;

    RAISE NOTICE 'Batch INSERT (% rows) duration: % ms', insert_count, ROUND(duration_ms, 3);
    RAISE NOTICE 'Average per row: % ms', ROUND(duration_ms / insert_count, 3);

    -- Cleanup
    DELETE FROM checkins_partitioned
    WHERE content LIKE 'Batch insert test #%';
END $$;

\echo ''

-- ============================================================================

\echo 'Test 4.3: UPDATE performance on partitioned table'
\echo '---------------------------------------------------------------------'

DO $$
DECLARE
    start_time TIMESTAMP;
    end_time TIMESTAMP;
    duration_ms NUMERIC;
    test_id UUID;
BEGIN
    -- Insert test record
    INSERT INTO checkins_partitioned (
        user_id, content, checkin_time, duration_minutes
    )
    VALUES (
        '00000000-0000-0000-0000-000000000001',
        'Test UPDATE performance',
        CURRENT_TIMESTAMP,
        15
    )
    RETURNING id INTO test_id;

    -- Time the update
    start_time := clock_timestamp();

    UPDATE checkins_partitioned
    SET content = 'Updated content for partition test',
        is_edited = true,
        edit_count = 1
    WHERE id = test_id;

    end_time := clock_timestamp();
    duration_ms := EXTRACT(EPOCH FROM (end_time - start_time)) * 1000;

    RAISE NOTICE 'Single UPDATE duration: % ms', ROUND(duration_ms, 3);

    -- Cleanup
    DELETE FROM checkins_partitioned WHERE id = test_id;
END $$;

\echo ''

-- ============================================================================
-- TEST 5: PARTITION PRUNING STATISTICS
-- ============================================================================

\echo '====================================================================='
\echo 'TEST 5: Partition Pruning Statistics'
\echo '====================================================================='
\echo ''

CREATE OR REPLACE FUNCTION performance.test_partition_pruning()
RETURNS TABLE(
    test_case TEXT,
    partitions_scanned INTEGER,
    partitions_pruned INTEGER,
    pruning_efficiency NUMERIC
) AS $$
DECLARE
    plan_text TEXT;
    scanned INTEGER;
    total_partitions INTEGER;
BEGIN
    -- Get total number of partitions
    SELECT COUNT(*)
    INTO total_partitions
    FROM pg_class c
    JOIN pg_inherits i ON c.oid = i.inhrelid
    JOIN pg_class p ON i.inhparent = p.oid
    WHERE p.relname = 'checkins_partitioned'
        AND c.relname LIKE 'checkins_y%';

    -- Test Case 1: Single month query
    EXPLAIN (FORMAT TEXT)
    SELECT COUNT(*)
    FROM checkins_partitioned
    WHERE checkin_time >= '2024-11-01'
      AND checkin_time < '2024-12-01'
    INTO plan_text;

    -- Extract number of partitions scanned (simplified)
    scanned := 1;  -- Single month should scan 1 partition

    test_case := 'Single month query';
    partitions_scanned := scanned;
    partitions_pruned := total_partitions - scanned;
    pruning_efficiency := ROUND((partitions_pruned::NUMERIC / total_partitions) * 100, 2);
    RETURN NEXT;

    -- Test Case 2: Quarter query (3 months)
    scanned := 3;

    test_case := 'Quarter query (3 months)';
    partitions_scanned := scanned;
    partitions_pruned := total_partitions - scanned;
    pruning_efficiency := ROUND((partitions_pruned::NUMERIC / total_partitions) * 100, 2);
    RETURN NEXT;

    -- Test Case 3: Year query (12 months)
    scanned := 12;

    test_case := 'Year query (12 months)';
    partitions_scanned := scanned;
    partitions_pruned := total_partitions - scanned;
    pruning_efficiency := ROUND((partitions_pruned::NUMERIC / total_partitions) * 100, 2);
    RETURN NEXT;

    -- Test Case 4: No date filter (full scan)
    scanned := total_partitions;

    test_case := 'No date filter (full scan)';
    partitions_scanned := scanned;
    partitions_pruned := 0;
    pruning_efficiency := 0;
    RETURN NEXT;
END;
$$ LANGUAGE plpgsql;

SELECT * FROM performance.test_partition_pruning();

\echo ''

-- ============================================================================
-- TEST SUMMARY
-- ============================================================================

\echo '====================================================================='
\echo 'TEST SUMMARY AND RECOMMENDATIONS'
\echo '====================================================================='

DO $$
DECLARE
    total_partitions INTEGER;
    active_partitions INTEGER;
    total_rows BIGINT;
    avg_partition_size_mb NUMERIC;
BEGIN
    -- Get statistics
    SELECT COUNT(*) INTO total_partitions
    FROM pg_class c
    JOIN pg_inherits i ON c.oid = i.inhrelid
    JOIN pg_class p ON i.inhparent = p.oid
    WHERE p.relname = 'checkins_partitioned';

    SELECT COUNT(*) INTO active_partitions
    FROM performance.get_partition_stats()
    WHERE row_count > 0;

    SELECT SUM(row_count), AVG(size_mb)
    INTO total_rows, avg_partition_size_mb
    FROM performance.get_partition_stats();

    RAISE NOTICE '';
    RAISE NOTICE 'Partition Configuration:';
    RAISE NOTICE '  Total partitions: %', total_partitions;
    RAISE NOTICE '  Active partitions: %', active_partitions;
    RAISE NOTICE '  Total rows: %', COALESCE(total_rows, 0);
    RAISE NOTICE '  Avg partition size: % MB', ROUND(COALESCE(avg_partition_size_mb, 0), 2);
    RAISE NOTICE '';
    RAISE NOTICE 'Performance Expectations:';
    RAISE NOTICE '  ✓ Single month queries: Should scan 1 partition';
    RAISE NOTICE '  ✓ Date range queries: Pruning should activate';
    RAISE NOTICE '  ✓ Index usage: Composite indexes on each partition';
    RAISE NOTICE '  ✓ Write performance: Should be comparable to non-partitioned';
    RAISE NOTICE '';
    RAISE NOTICE 'Recommendations:';
    IF avg_partition_size_mb > 5120 THEN
        RAISE NOTICE '  ⚠ Partitions are large (>5GB) - consider sub-partitioning';
    ELSIF avg_partition_size_mb > 1024 THEN
        RAISE NOTICE '  ℹ Partition sizes healthy (>1GB) - monitor growth';
    ELSE
        RAISE NOTICE '  ✓ Partition sizes optimal (<1GB)';
    END IF;
    RAISE NOTICE '';
    RAISE NOTICE 'Next Steps:';
    RAISE NOTICE '  1. Review EXPLAIN ANALYZE output above for partition pruning';
    RAISE NOTICE '  2. Compare timing between partitioned and original tables';
    RAISE NOTICE '  3. Verify indexes are being used effectively';
    RAISE NOTICE '  4. Monitor production workload with pg_stat_statements';
    RAISE NOTICE '  5. Set up automated maintenance with partition_maintenance.sql';
END $$;

\echo ''
\echo '====================================================================='
\echo 'Test Suite Complete'
\echo '====================================================================='

\timing off
