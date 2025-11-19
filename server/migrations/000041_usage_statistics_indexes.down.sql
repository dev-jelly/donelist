-- Drop usage statistics indexes and materialized views

-- Drop function
DROP FUNCTION IF EXISTS refresh_usage_statistics();

-- Drop materialized views
DROP MATERIALIZED VIEW IF EXISTS tag_usage_summary;
DROP MATERIALIZED VIEW IF EXISTS category_usage_summary;

-- Drop indexes
DROP INDEX IF EXISTS idx_checkins_dow_hour;
DROP INDEX IF EXISTS idx_checkins_recent_category;
DROP INDEX IF EXISTS idx_tags_user_deleted;
DROP INDEX IF EXISTS idx_categories_user_deleted;
DROP INDEX IF EXISTS idx_checkins_created_deleted;
DROP INDEX IF EXISTS idx_checkin_tags_tag_checkin;
DROP INDEX IF EXISTS idx_checkins_user_category_created;
DROP INDEX IF EXISTS idx_checkins_category_created;