-- Rollback offline sync system

-- Drop triggers
DROP TRIGGER IF EXISTS update_sync_status_on_queue_change ON sync_queue;
DROP TRIGGER IF EXISTS update_sync_status_updated_at ON sync_status;
DROP TRIGGER IF EXISTS update_sync_queue_updated_at ON sync_queue;

-- Drop functions
DROP FUNCTION IF EXISTS update_sync_status_counts();
DROP FUNCTION IF EXISTS cleanup_old_sync_operations();

-- Drop tables (in reverse dependency order)
DROP TABLE IF EXISTS idempotency_tokens;
DROP TABLE IF EXISTS sync_operation_log;
DROP TABLE IF EXISTS sync_status;
DROP TABLE IF EXISTS sync_queue;

-- Note: We don't drop the version column from checkins as it may be used by other features
-- ALTER TABLE checkins DROP COLUMN IF EXISTS version;
