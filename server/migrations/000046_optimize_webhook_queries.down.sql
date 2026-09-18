-- Rollback webhook query optimization indexes

DROP INDEX IF EXISTS idx_webhooks_events_gin;
DROP INDEX IF EXISTS idx_webhooks_active_user;
