-- Rollback timeline optimization indexes

-- Drop the materialized view first
DROP MATERIALIZED VIEW IF EXISTS daily_checkin_stats;

-- Drop new indexes
DROP INDEX CONCURRENTLY IF EXISTS idx_checkins_user_time_deleted;
DROP INDEX CONCURRENTLY IF EXISTS idx_checkins_user_category_time;
DROP INDEX CONCURRENTLY IF EXISTS idx_checkins_team_visibility_time;
DROP INDEX CONCURRENTLY IF EXISTS idx_checkins_gap_detection;
DROP INDEX CONCURRENTLY IF EXISTS idx_checkins_last_checkin;
DROP INDEX CONCURRENTLY IF EXISTS idx_checkins_date_category_stats;
DROP INDEX CONCURRENTLY IF EXISTS idx_edit_history_user_checkin_time;
DROP INDEX CONCURRENTLY IF EXISTS idx_checkin_tags_checkin_tag;

-- Restore original indexes from migration 000001
CREATE INDEX IF NOT EXISTS idx_checkins_user_id ON checkins(user_id);
CREATE INDEX IF NOT EXISTS idx_checkins_checkin_time ON checkins(checkin_time);
CREATE INDEX IF NOT EXISTS idx_checkins_user_time ON checkins(user_id, checkin_time DESC);
