-- Migration: Add GIN Indexes for Full-Text Search Performance
-- GIN indexes provide fast full-text search capabilities

-- Create GIN index on checkins search vector
-- This is the primary index for full-text search on checkins
CREATE INDEX idx_checkins_search_vector ON checkins USING GIN(search_vector);

-- Create GIN index on categories search vector
CREATE INDEX idx_categories_search_vector ON categories USING GIN(search_vector);

-- Create GIN index on tags search vector
CREATE INDEX idx_tags_search_vector ON tags USING GIN(search_vector);

-- Create composite indexes for common filter combinations
-- These indexes optimize queries that combine full-text search with other filters

-- Index for user + time range queries (most common pattern)
CREATE INDEX idx_checkins_user_time_range ON checkins(user_id, checkin_time DESC, deleted_at)
WHERE deleted_at IS NULL;

-- Index for user + category filtering
CREATE INDEX idx_checkins_user_category ON checkins(user_id, category_id, checkin_time DESC)
WHERE deleted_at IS NULL;

-- Index for duration filtering (useful for analytics)
CREATE INDEX idx_checkins_user_duration ON checkins(user_id, duration_minutes, checkin_time DESC)
WHERE deleted_at IS NULL;

-- Partial index for non-deleted checkins (most queries filter these)
CREATE INDEX idx_checkins_active ON checkins(user_id, checkin_time DESC)
WHERE deleted_at IS NULL;

-- Index for tag-based searches via junction table
CREATE INDEX idx_checkin_tags_composite ON checkin_tags(tag_id, checkin_id);

-- Create statistics for better query planning
ANALYZE checkins;
ANALYZE categories;
ANALYZE tags;
ANALYZE checkin_tags;

-- Add comments to document the indexes
COMMENT ON INDEX idx_checkins_search_vector IS 'GIN index for full-text search on checkins content, categories, and tags';
COMMENT ON INDEX idx_categories_search_vector IS 'GIN index for full-text search on category names';
COMMENT ON INDEX idx_tags_search_vector IS 'GIN index for full-text search on tag names';
COMMENT ON INDEX idx_checkins_user_time_range IS 'Composite index for user + time range queries with deleted_at filter';
COMMENT ON INDEX idx_checkins_user_category IS 'Composite index for user + category filtering';
COMMENT ON INDEX idx_checkins_user_duration IS 'Composite index for duration-based queries';
COMMENT ON INDEX idx_checkins_active IS 'Partial index for active (non-deleted) checkins';
