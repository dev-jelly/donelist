-- Rollback job tracking migration

-- Remove triggers
DROP TRIGGER IF EXISTS update_job_execution_timestamps ON job_executions;

-- Remove functions
DROP FUNCTION IF EXISTS update_job_execution_stats();
DROP FUNCTION IF EXISTS cleanup_expired_archives();

-- Drop indexes
DROP INDEX IF EXISTS idx_data_export_requests_processing;

-- Remove columns from existing tables
ALTER TABLE data_export_requests DROP COLUMN IF EXISTS processing_started_at;
ALTER TABLE data_export_requests DROP COLUMN IF EXISTS retry_count;
ALTER TABLE data_export_requests DROP COLUMN IF EXISTS file_size_bytes;

-- Drop tables
DROP TABLE IF EXISTS security_events_archive;
DROP TABLE IF EXISTS user_data_archives;
DROP TABLE IF EXISTS job_executions;