-- Rollback: Remove Search Performance Indexes

-- Drop composite and partial indexes
DROP INDEX IF EXISTS idx_checkins_active;
DROP INDEX IF EXISTS idx_checkins_user_duration;
DROP INDEX IF EXISTS idx_checkins_user_category;
DROP INDEX IF EXISTS idx_checkins_user_time_range;
DROP INDEX IF EXISTS idx_checkin_tags_composite;

-- Drop GIN indexes
DROP INDEX IF EXISTS idx_tags_search_vector;
DROP INDEX IF EXISTS idx_categories_search_vector;
DROP INDEX IF EXISTS idx_checkins_search_vector;
