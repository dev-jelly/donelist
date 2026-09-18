-- Migration 000049: Add Job Tracking and Archives
-- Adds tables for tracking background job execution and data archives

-- Job Executions Table
-- Tracks the execution history of background jobs
CREATE TABLE IF NOT EXISTS job_executions (
    job_name VARCHAR(100) PRIMARY KEY,
    success BOOLEAN DEFAULT TRUE,
    error_message TEXT,
    duration_ms BIGINT,
    executed_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    execution_count INTEGER DEFAULT 1,
    last_success_at TIMESTAMP WITH TIME ZONE,
    last_failure_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_job_executions_executed_at ON job_executions(executed_at DESC);
CREATE INDEX idx_job_executions_success ON job_executions(success);

-- User Data Archives Table
-- Stores archived user data before deletion for compliance
CREATE TABLE IF NOT EXISTS user_data_archives (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL,
    email_hash VARCHAR(255) NOT NULL, -- One-way hash for identification
    data JSONB NOT NULL, -- Archived user data
    archived_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL, -- When to permanently delete
    deleted_at TIMESTAMP WITH TIME ZONE -- When actually deleted
);

CREATE INDEX idx_user_data_archives_user_id ON user_data_archives(user_id);
CREATE INDEX idx_user_data_archives_expires_at ON user_data_archives(expires_at);
CREATE INDEX idx_user_data_archives_archived_at ON user_data_archives(archived_at);

-- Security Events Archive Table
-- Long-term storage for important security events
CREATE TABLE IF NOT EXISTS security_events_archive (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL,
    event_type VARCHAR(50) NOT NULL,
    event_details JSONB,
    original_created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    archived_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_security_events_archive_user_id ON security_events_archive(user_id);
CREATE INDEX idx_security_events_archive_event_type ON security_events_archive(event_type);
CREATE INDEX idx_security_events_archive_created_at ON security_events_archive(original_created_at);

-- Add job monitoring columns to existing tables
ALTER TABLE data_export_requests ADD COLUMN IF NOT EXISTS processing_started_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE data_export_requests ADD COLUMN IF NOT EXISTS retry_count INTEGER DEFAULT 0;
ALTER TABLE data_export_requests ADD COLUMN IF NOT EXISTS file_size_bytes BIGINT;

-- Add index for job processing
CREATE INDEX IF NOT EXISTS idx_data_export_requests_processing ON data_export_requests(status, processing_started_at)
    WHERE status IN ('pending', 'processing');

-- Function to update job execution stats
CREATE OR REPLACE FUNCTION update_job_execution_stats()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.success THEN
        NEW.last_success_at = CURRENT_TIMESTAMP;
    ELSE
        NEW.last_failure_at = CURRENT_TIMESTAMP;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger to update job execution timestamps
CREATE TRIGGER update_job_execution_timestamps
BEFORE INSERT OR UPDATE ON job_executions
FOR EACH ROW
EXECUTE FUNCTION update_job_execution_stats();

-- Function to automatically cleanup old archives
CREATE OR REPLACE FUNCTION cleanup_expired_archives()
RETURNS void AS $$
BEGIN
    -- Delete expired user data archives
    DELETE FROM user_data_archives
    WHERE expires_at < CURRENT_TIMESTAMP
      AND deleted_at IS NULL;

    -- Mark as deleted but keep record for audit
    UPDATE user_data_archives
    SET deleted_at = CURRENT_TIMESTAMP,
        data = '{"deleted": true}'::jsonb
    WHERE expires_at < CURRENT_TIMESTAMP
      AND deleted_at IS NULL;

    -- Delete very old security event archives (> 3 years)
    DELETE FROM security_events_archive
    WHERE archived_at < CURRENT_TIMESTAMP - INTERVAL '3 years';
END;
$$ LANGUAGE plpgsql;

-- Comments for documentation
COMMENT ON TABLE job_executions IS 'Tracks execution history and status of background jobs';
COMMENT ON TABLE user_data_archives IS 'Stores archived user data before deletion for compliance and recovery';
COMMENT ON TABLE security_events_archive IS 'Long-term archive of important security events';

COMMENT ON COLUMN job_executions.duration_ms IS 'Job execution time in milliseconds';
COMMENT ON COLUMN user_data_archives.email_hash IS 'One-way hash of user email for identification without storing PII';
COMMENT ON COLUMN user_data_archives.expires_at IS 'When this archive should be permanently deleted';

-- Initial job records
INSERT INTO job_executions (job_name, success, executed_at, execution_count)
VALUES
    ('account_deletion', true, CURRENT_TIMESTAMP, 0),
    ('deletion_reminder', true, CURRENT_TIMESTAMP, 0),
    ('data_retention', true, CURRENT_TIMESTAMP, 0),
    ('export_cleanup', true, CURRENT_TIMESTAMP, 0)
ON CONFLICT (job_name) DO NOTHING;