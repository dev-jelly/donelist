-- Create audit_logs table for security and compliance logging
CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type VARCHAR(100) NOT NULL,
    severity VARCHAR(20) NOT NULL DEFAULT 'info',
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    ip_address VARCHAR(45) NOT NULL, -- Supports both IPv4 and IPv6
    user_agent TEXT NOT NULL,
    action TEXT NOT NULL,
    resource VARCHAR(100),
    resource_id UUID,
    details JSONB,
    success BOOLEAN NOT NULL DEFAULT true,
    error_message TEXT,
    request_id VARCHAR(100),
    session_id VARCHAR(255),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for common queries
CREATE INDEX idx_audit_logs_user_id ON audit_logs(user_id);
CREATE INDEX idx_audit_logs_event_type ON audit_logs(event_type);
CREATE INDEX idx_audit_logs_severity ON audit_logs(severity);
CREATE INDEX idx_audit_logs_created_at ON audit_logs(created_at DESC);
CREATE INDEX idx_audit_logs_ip_address ON audit_logs(ip_address);
CREATE INDEX idx_audit_logs_success ON audit_logs(success);
CREATE INDEX idx_audit_logs_request_id ON audit_logs(request_id);

-- Create composite index for common query patterns
CREATE INDEX idx_audit_logs_user_created ON audit_logs(user_id, created_at DESC);
CREATE INDEX idx_audit_logs_event_severity ON audit_logs(event_type, severity);

-- Add comment to table
COMMENT ON TABLE audit_logs IS 'Stores audit trail for security and compliance';

-- Add comments to columns
COMMENT ON COLUMN audit_logs.event_type IS 'Type of event (e.g., auth.login.success, security.rate_limit_exceeded)';
COMMENT ON COLUMN audit_logs.severity IS 'Severity level: info, warning, error, critical';
COMMENT ON COLUMN audit_logs.details IS 'Additional context stored as JSON';
COMMENT ON COLUMN audit_logs.resource IS 'Resource type being accessed (e.g., user, checkin, category)';
COMMENT ON COLUMN audit_logs.resource_id IS 'ID of the specific resource';
