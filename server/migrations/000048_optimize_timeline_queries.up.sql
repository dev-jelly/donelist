-- Optimize timeline queries with compound indexes
-- These indexes are specifically designed for timeline API queries

-- Composite index for timeline queries: user_id + checkin_time (DESC) + deleted_at
-- This covers the most common query pattern: getting check-ins for a user within a time range
-- The DESC order matches the ORDER BY clause in most timeline queries
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_checkins_user_time_deleted
ON checkins(user_id, checkin_time DESC, deleted_at)
WHERE deleted_at IS NULL;

-- Composite index for category filtering in timelines: user_id + category_id + checkin_time
-- Used when filtering timeline by category
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_checkins_user_category_time
ON checkins(user_id, category_id, checkin_time DESC)
WHERE deleted_at IS NULL AND category_id IS NOT NULL;

-- Composite index for team timelines: team_id + visibility + checkin_time
-- Used for team collaboration features
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_checkins_team_visibility_time
ON checkins(team_id, visibility, checkin_time DESC)
WHERE deleted_at IS NULL AND team_id IS NOT NULL;

-- Partial index for finding gaps: user_id + checkin_time + duration_minutes
-- Used for gap detection between check-ins
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_checkins_gap_detection
ON checkins(user_id, checkin_time, duration_minutes)
WHERE deleted_at IS NULL;

-- Index for finding last check-in efficiently
-- Optimizes GetLastCheckin queries
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_checkins_last_checkin
ON checkins(user_id, checkin_time DESC, created_at DESC)
WHERE deleted_at IS NULL;

-- Index for date range queries with category aggregation
-- Used for daily summary calculations
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_checkins_date_category_stats
ON checkins(user_id, checkin_time, category_id, duration_minutes)
WHERE deleted_at IS NULL;

-- Drop old redundant indexes that are covered by the new composite indexes
-- Only drop if they exist to avoid errors
DROP INDEX IF EXISTS idx_checkins_user_time;
DROP INDEX IF EXISTS idx_checkins_user_id;
DROP INDEX IF EXISTS idx_checkins_checkin_time;

-- Add index on edit_history for timeline audit trail
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_edit_history_user_checkin_time
ON edit_history(user_id, checkin_id, edited_at DESC);

-- Add index on checkin_tags for timeline enrichment
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_checkin_tags_checkin_tag
ON checkin_tags(checkin_id, tag_id);

-- Statistics update for better query planning
ANALYZE checkins;
ANALYZE edit_history;
ANALYZE checkin_tags;

-- Create a materialized view for daily statistics (optional, can be used for caching)
CREATE MATERIALIZED VIEW IF NOT EXISTS daily_checkin_stats AS
SELECT
    user_id,
    DATE(checkin_time) AS checkin_date,
    COUNT(*) AS total_checkins,
    SUM(duration_minutes) AS total_minutes,
    MIN(checkin_time) AS first_checkin,
    MAX(checkin_time) AS last_checkin,
    COUNT(DISTINCT category_id) AS unique_categories,
    ARRAY_AGG(DISTINCT category_id) FILTER (WHERE category_id IS NOT NULL) AS category_ids
FROM checkins
WHERE deleted_at IS NULL
GROUP BY user_id, DATE(checkin_time);

-- Index on the materialized view
CREATE UNIQUE INDEX IF NOT EXISTS idx_daily_stats_user_date
ON daily_checkin_stats(user_id, checkin_date);

-- Comment documentation for indexes
COMMENT ON INDEX idx_checkins_user_time_deleted IS 'Primary timeline query index: filters by user, orders by time DESC, excludes soft-deleted';
COMMENT ON INDEX idx_checkins_user_category_time IS 'Timeline filtered by category';
COMMENT ON INDEX idx_checkins_team_visibility_time IS 'Team timeline queries with visibility filtering';
COMMENT ON INDEX idx_checkins_gap_detection IS 'Optimizes gap detection between consecutive check-ins';
COMMENT ON INDEX idx_checkins_last_checkin IS 'Efficiently finds the most recent check-in for a user';
COMMENT ON INDEX idx_checkins_date_category_stats IS 'Daily summary and category statistics aggregation';
