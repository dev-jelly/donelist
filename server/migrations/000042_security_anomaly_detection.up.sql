-- Security Alerts Table
CREATE TABLE IF NOT EXISTS security_alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    anomaly_type VARCHAR(50) NOT NULL,
    severity VARCHAR(20) NOT NULL CHECK (severity IN ('low', 'medium', 'high', 'critical')),
    priority VARCHAR(20) NOT NULL CHECK (priority IN ('low', 'medium', 'high', 'critical')),
    identifier VARCHAR(255) NOT NULL,
    ip_address VARCHAR(45) NOT NULL,
    user_agent TEXT,
    description TEXT NOT NULL,
    metadata JSONB DEFAULT '{}',
    score DECIMAL(5,2) DEFAULT 0.0,
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'resolved', 'ignored')),
    channels JSONB DEFAULT '[]',
    sent_at TIMESTAMP,
    resolved_at TIMESTAMP,
    resolved_by UUID,
    notes TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Security Blocks Table
CREATE TABLE IF NOT EXISTS security_blocks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    identifier VARCHAR(255) NOT NULL,
    ip_address VARCHAR(45) NOT NULL,
    reason VARCHAR(50) NOT NULL,
    description TEXT NOT NULL,
    severity VARCHAR(20) NOT NULL CHECK (severity IN ('low', 'medium', 'high', 'critical')),
    metadata JSONB DEFAULT '{}',
    status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'expired', 'released')),
    blocked_at TIMESTAMP NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP NOT NULL,
    released_at TIMESTAMP,
    released_by UUID,
    notes TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Indexes for security_alerts
CREATE INDEX idx_security_alerts_identifier ON security_alerts(identifier);
CREATE INDEX idx_security_alerts_ip_address ON security_alerts(ip_address);
CREATE INDEX idx_security_alerts_anomaly_type ON security_alerts(anomaly_type);
CREATE INDEX idx_security_alerts_severity ON security_alerts(severity);
CREATE INDEX idx_security_alerts_status ON security_alerts(status);
CREATE INDEX idx_security_alerts_created_at ON security_alerts(created_at);
CREATE INDEX idx_security_alerts_identifier_created_at ON security_alerts(identifier, created_at DESC);

-- Indexes for security_blocks
CREATE INDEX idx_security_blocks_identifier ON security_blocks(identifier);
CREATE INDEX idx_security_blocks_ip_address ON security_blocks(ip_address);
CREATE INDEX idx_security_blocks_status ON security_blocks(status);
CREATE INDEX idx_security_blocks_reason ON security_blocks(reason);
CREATE INDEX idx_security_blocks_expires_at ON security_blocks(expires_at);
CREATE INDEX idx_security_blocks_blocked_at ON security_blocks(blocked_at);
CREATE INDEX idx_security_blocks_active ON security_blocks(identifier, status, expires_at) WHERE status = 'active';

-- Partial index for active blocks
CREATE INDEX idx_security_blocks_active_unexpired ON security_blocks(identifier, expires_at)
WHERE status = 'active' AND expires_at > NOW();

-- GIN indexes for JSONB columns
CREATE INDEX idx_security_alerts_metadata ON security_alerts USING GIN (metadata);
CREATE INDEX idx_security_blocks_metadata ON security_blocks USING GIN (metadata);

-- Add comments
COMMENT ON TABLE security_alerts IS 'Security alerts from anomaly detection system';
COMMENT ON TABLE security_blocks IS 'Blocked identifiers due to security anomalies';

COMMENT ON COLUMN security_alerts.anomaly_type IS 'Type of detected anomaly (rapid_requests, brute_force, etc.)';
COMMENT ON COLUMN security_alerts.severity IS 'Severity level of the anomaly';
COMMENT ON COLUMN security_alerts.priority IS 'Alert priority for notification routing';
COMMENT ON COLUMN security_alerts.identifier IS 'User ID, IP address, or other identifier';
COMMENT ON COLUMN security_alerts.score IS 'Anomaly score (0-100)';
COMMENT ON COLUMN security_alerts.channels IS 'Notification channels (webhook, email, slack, etc.)';

COMMENT ON COLUMN security_blocks.identifier IS 'Blocked identifier (user ID or IP)';
COMMENT ON COLUMN security_blocks.reason IS 'Reason for blocking';
COMMENT ON COLUMN security_blocks.status IS 'Current status of the block';
COMMENT ON COLUMN security_blocks.expires_at IS 'When the block automatically expires';
