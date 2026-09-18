-- Migration 000048: Add Two-Factor Authentication Support
-- Adds TOTP-based 2FA, backup codes, and security settings

-- Add 2FA columns to users table
ALTER TABLE users ADD COLUMN IF NOT EXISTS two_factor_enabled BOOLEAN DEFAULT FALSE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS two_factor_secret VARCHAR(64);
ALTER TABLE users ADD COLUMN IF NOT EXISTS two_factor_verified_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS recovery_email VARCHAR(255);
ALTER TABLE users ADD COLUMN IF NOT EXISTS recovery_email_verified BOOLEAN DEFAULT FALSE;

-- Two-Factor Backup Codes Table
CREATE TABLE IF NOT EXISTS two_factor_backup_codes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash VARCHAR(255) NOT NULL, -- bcrypt hash of the backup code
    used BOOLEAN DEFAULT FALSE,
    used_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_2fa_backup_codes_user_id ON two_factor_backup_codes(user_id);
CREATE INDEX idx_2fa_backup_codes_used ON two_factor_backup_codes(used);

-- Password Change History Table (for security)
CREATE TABLE IF NOT EXISTS password_history (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    password_hash VARCHAR(255) NOT NULL,
    changed_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    changed_by_ip INET,
    change_reason VARCHAR(50) -- 'user_request', 'admin_reset', 'security_breach', 'expired'
);

CREATE INDEX idx_password_history_user_id ON password_history(user_id);
CREATE INDEX idx_password_history_changed_at ON password_history(changed_at);

-- Security Events Table
CREATE TABLE IF NOT EXISTS security_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    event_type VARCHAR(50) NOT NULL, -- 'login', '2fa_enabled', '2fa_disabled', 'password_changed', 'suspicious_activity'
    event_details JSONB,
    ip_address INET,
    user_agent TEXT,
    success BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_security_events_user_id ON security_events(user_id);
CREATE INDEX idx_security_events_type ON security_events(event_type);
CREATE INDEX idx_security_events_created_at ON security_events(created_at);

-- Profile Privacy Settings
ALTER TABLE user_profiles ADD COLUMN IF NOT EXISTS privacy_level VARCHAR(20) DEFAULT 'private'
    CHECK (privacy_level IN ('public', 'private', 'friends'));
ALTER TABLE user_profiles ADD COLUMN IF NOT EXISTS show_email BOOLEAN DEFAULT FALSE;
ALTER TABLE user_profiles ADD COLUMN IF NOT EXISTS show_activity BOOLEAN DEFAULT FALSE;
ALTER TABLE user_profiles ADD COLUMN IF NOT EXISTS searchable BOOLEAN DEFAULT TRUE;

-- Data Export Requests Table
CREATE TABLE IF NOT EXISTS data_export_requests (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status VARCHAR(20) DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'expired')),
    format VARCHAR(10) DEFAULT 'json' CHECK (format IN ('json', 'csv', 'xml')),
    file_url TEXT,
    expires_at TIMESTAMP WITH TIME ZONE,
    requested_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP WITH TIME ZONE,
    error_message TEXT
);

CREATE INDEX idx_data_export_user_id ON data_export_requests(user_id);
CREATE INDEX idx_data_export_status ON data_export_requests(status);
CREATE INDEX idx_data_export_expires_at ON data_export_requests(expires_at);

-- Account Recovery Tokens Table
CREATE TABLE IF NOT EXISTS account_recovery_tokens (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(255) NOT NULL,
    token_type VARCHAR(20) NOT NULL CHECK (token_type IN ('password_reset', 'email_verification', '2fa_recovery')),
    used BOOLEAN DEFAULT FALSE,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    used_at TIMESTAMP WITH TIME ZONE,
    ip_address INET
);

CREATE INDEX idx_recovery_tokens_user_id ON account_recovery_tokens(user_id);
CREATE INDEX idx_recovery_tokens_token_hash ON account_recovery_tokens(token_hash);
CREATE INDEX idx_recovery_tokens_expires_at ON account_recovery_tokens(expires_at);

-- Function to log security events
CREATE OR REPLACE FUNCTION log_security_event()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'UPDATE' THEN
        -- Log 2FA changes
        IF NEW.two_factor_enabled != OLD.two_factor_enabled THEN
            INSERT INTO security_events (user_id, event_type, event_details, success)
            VALUES (
                NEW.id,
                CASE WHEN NEW.two_factor_enabled THEN '2fa_enabled' ELSE '2fa_disabled' END,
                jsonb_build_object('changed_at', CURRENT_TIMESTAMP),
                TRUE
            );
        END IF;

        -- Log password changes
        IF NEW.password_hash != OLD.password_hash THEN
            INSERT INTO password_history (user_id, password_hash, change_reason)
            VALUES (NEW.id, OLD.password_hash, 'user_request');

            INSERT INTO security_events (user_id, event_type, event_details, success)
            VALUES (
                NEW.id,
                'password_changed',
                jsonb_build_object('changed_at', CURRENT_TIMESTAMP),
                TRUE
            );
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger for security event logging
CREATE TRIGGER log_user_security_changes
AFTER UPDATE ON users
FOR EACH ROW
EXECUTE FUNCTION log_security_event();

-- Comments for documentation
COMMENT ON TABLE two_factor_backup_codes IS 'Stores backup codes for 2FA recovery';
COMMENT ON TABLE password_history IS 'Tracks password change history for security auditing';
COMMENT ON TABLE security_events IS 'Logs all security-related events for user accounts';
COMMENT ON TABLE data_export_requests IS 'Tracks user data export requests for GDPR compliance';
COMMENT ON TABLE account_recovery_tokens IS 'Manages tokens for password reset and account recovery';

COMMENT ON COLUMN user_profiles.privacy_level IS 'Profile visibility: public (anyone), private (only user), friends (connected users)';
COMMENT ON COLUMN users.two_factor_secret IS 'TOTP secret for 2FA, encrypted at rest';
COMMENT ON COLUMN security_events.event_type IS 'Type of security event for audit trail';