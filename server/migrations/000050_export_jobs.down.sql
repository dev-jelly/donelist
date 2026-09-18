-- Migration 000050: Export Jobs Table (Rollback)

-- Drop trigger and function
DROP TRIGGER IF EXISTS trigger_update_export_jobs_updated_at ON export_jobs;
DROP FUNCTION IF EXISTS update_export_jobs_updated_at();

-- Drop indexes
DROP INDEX IF EXISTS idx_export_jobs_active;
DROP INDEX IF EXISTS idx_export_jobs_created_at;
DROP INDEX IF EXISTS idx_export_jobs_expires_at;
DROP INDEX IF EXISTS idx_export_jobs_status;
DROP INDEX IF EXISTS idx_export_jobs_user_id;

-- Drop table
DROP TABLE IF EXISTS export_jobs;
