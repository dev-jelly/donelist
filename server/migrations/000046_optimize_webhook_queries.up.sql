-- Optimize webhook event filtering with GIN index
-- Expected 10-100x improvement for users with many webhooks

-- Add GIN index on events array for fast containment checks
CREATE INDEX IF NOT EXISTS idx_webhooks_events_gin ON webhooks USING gin(events);

-- Add composite index for active user webhooks
CREATE INDEX IF NOT EXISTS idx_webhooks_active_user ON webhooks(user_id, active) WHERE deleted_at IS NULL;

-- Add comment explaining optimization
COMMENT ON INDEX idx_webhooks_events_gin IS 'GIN index for efficient array containment queries (@> operator)';
COMMENT ON INDEX idx_webhooks_active_user IS 'Composite index for active webhook lookups by user';
