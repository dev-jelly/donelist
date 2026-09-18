-- Migration 000050: Export Jobs Table
-- Creates table for tracking user data export jobs

CREATE TABLE IF NOT EXISTS export_jobs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    format VARCHAR(10) NOT NULL CHECK (format IN ('csv', 'json', 'pdf')),
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
    file_path TEXT,
    file_size BIGINT,
    record_count INTEGER DEFAULT 0,
    error TEXT,
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for efficient queries
CREATE INDEX idx_export_jobs_user_id ON export_jobs(user_id);
CREATE INDEX idx_export_jobs_status ON export_jobs(status);
CREATE INDEX idx_export_jobs_expires_at ON export_jobs(expires_at);
CREATE INDEX idx_export_jobs_created_at ON export_jobs(created_at DESC);

-- Index for finding pending/processing jobs
CREATE INDEX idx_export_jobs_active ON export_jobs(user_id, status)
    WHERE status IN ('pending', 'processing');

-- Trigger to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_export_jobs_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_update_export_jobs_updated_at
BEFORE UPDATE ON export_jobs
FOR EACH ROW
EXECUTE FUNCTION update_export_jobs_updated_at();

-- Comments for documentation
COMMENT ON TABLE export_jobs IS 'Tracks user data export jobs with background processing';
COMMENT ON COLUMN export_jobs.format IS 'Export format: csv, json, or pdf';
COMMENT ON COLUMN export_jobs.status IS 'Job status: pending, processing, completed, or failed';
COMMENT ON COLUMN export_jobs.file_path IS 'Local file system path to exported file';
COMMENT ON COLUMN export_jobs.file_size IS 'Size of exported file in bytes';
COMMENT ON COLUMN export_jobs.record_count IS 'Number of records included in export';
COMMENT ON COLUMN export_jobs.expires_at IS 'When the export file will be automatically deleted';
