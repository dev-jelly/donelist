-- Drop indexes
DROP INDEX IF EXISTS idx_security_alerts_identifier;
DROP INDEX IF EXISTS idx_security_alerts_ip_address;
DROP INDEX IF EXISTS idx_security_alerts_anomaly_type;
DROP INDEX IF EXISTS idx_security_alerts_severity;
DROP INDEX IF EXISTS idx_security_alerts_status;
DROP INDEX IF EXISTS idx_security_alerts_created_at;
DROP INDEX IF EXISTS idx_security_alerts_identifier_created_at;
DROP INDEX IF EXISTS idx_security_alerts_metadata;

DROP INDEX IF EXISTS idx_security_blocks_identifier;
DROP INDEX IF EXISTS idx_security_blocks_ip_address;
DROP INDEX IF EXISTS idx_security_blocks_status;
DROP INDEX IF EXISTS idx_security_blocks_reason;
DROP INDEX IF EXISTS idx_security_blocks_expires_at;
DROP INDEX IF EXISTS idx_security_blocks_blocked_at;
DROP INDEX IF EXISTS idx_security_blocks_active;
DROP INDEX IF EXISTS idx_security_blocks_active_unexpired;
DROP INDEX IF EXISTS idx_security_blocks_metadata;

-- Drop tables
DROP TABLE IF EXISTS security_blocks;
DROP TABLE IF EXISTS security_alerts;
