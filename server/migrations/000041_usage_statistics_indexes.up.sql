-- Create indexes for usage statistics collection

-- Indexes for category usage queries
CREATE INDEX IF NOT EXISTS idx_checkins_category_created
ON checkins(category_id, created_at)
WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_checkins_user_category_created
ON checkins(user_id, category_id, created_at)
WHERE deleted_at IS NULL;

-- Indexes for tag usage queries
CREATE INDEX IF NOT EXISTS idx_checkin_tags_tag_checkin
ON checkin_tags(tag_id, checkin_id);

CREATE INDEX IF NOT EXISTS idx_checkins_created_deleted
ON checkins(created_at, deleted_at)
WHERE deleted_at IS NULL;

-- Index for finding unused categories and tags
CREATE INDEX IF NOT EXISTS idx_categories_user_deleted
ON categories(user_id, deleted_at)
WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_tags_user_deleted
ON tags(user_id, deleted_at)
WHERE deleted_at IS NULL;

-- Partial indexes for active items (used in last 7 days)
CREATE INDEX IF NOT EXISTS idx_checkins_recent_category
ON checkins(category_id, created_at)
WHERE deleted_at IS NULL AND created_at > (NOW() - INTERVAL '7 days');

-- Index for hour and day of week patterns
CREATE INDEX IF NOT EXISTS idx_checkins_dow_hour
ON checkins(
    EXTRACT(DOW FROM created_at),
    EXTRACT(HOUR FROM created_at),
    category_id
)
WHERE deleted_at IS NULL;

-- Create materialized view for category usage summary (optional, for performance)
CREATE MATERIALIZED VIEW IF NOT EXISTS category_usage_summary AS
SELECT
    c.id as category_id,
    c.user_id,
    c.name as category_name,
    c.color,
    c.icon,
    COUNT(ch.id) as total_usage,
    COUNT(DISTINCT DATE(ch.created_at)) as unique_days,
    MAX(ch.created_at) as last_used,
    MIN(ch.created_at) as first_used
FROM categories c
LEFT JOIN checkins ch ON ch.category_id = c.id AND ch.deleted_at IS NULL
WHERE c.deleted_at IS NULL
GROUP BY c.id, c.user_id, c.name, c.color, c.icon;

-- Create index on materialized view
CREATE UNIQUE INDEX IF NOT EXISTS idx_category_usage_summary_pk
ON category_usage_summary(category_id);

CREATE INDEX IF NOT EXISTS idx_category_usage_summary_user
ON category_usage_summary(user_id, total_usage DESC);

-- Create materialized view for tag usage summary
CREATE MATERIALIZED VIEW IF NOT EXISTS tag_usage_summary AS
SELECT
    t.id as tag_id,
    t.user_id,
    t.name as tag_name,
    COUNT(ct.checkin_id) as total_usage,
    COUNT(DISTINCT DATE(ch.created_at)) as unique_days,
    MAX(ch.created_at) as last_used,
    MIN(ch.created_at) as first_used
FROM tags t
LEFT JOIN checkin_tags ct ON ct.tag_id = t.id
LEFT JOIN checkins ch ON ch.id = ct.checkin_id AND ch.deleted_at IS NULL
WHERE t.deleted_at IS NULL
GROUP BY t.id, t.user_id, t.name;

-- Create index on materialized view
CREATE UNIQUE INDEX IF NOT EXISTS idx_tag_usage_summary_pk
ON tag_usage_summary(tag_id);

CREATE INDEX IF NOT EXISTS idx_tag_usage_summary_user
ON tag_usage_summary(user_id, total_usage DESC);

-- Function to refresh materialized views (call periodically)
CREATE OR REPLACE FUNCTION refresh_usage_statistics()
RETURNS void AS $$
BEGIN
    REFRESH MATERIALIZED VIEW CONCURRENTLY category_usage_summary;
    REFRESH MATERIALIZED VIEW CONCURRENTLY tag_usage_summary;
END;
$$ LANGUAGE plpgsql;

-- Add comment
COMMENT ON FUNCTION refresh_usage_statistics() IS 'Refreshes usage statistics materialized views. Should be called periodically (e.g., hourly) via cron job or scheduler.';