-- Migration 000015: User Settings and Profile Management
-- Creates comprehensive user settings, profile customization, notification preferences,
-- data retention policies, and account deletion request tracking

-- User Profiles Table
-- Stores extended user profile information
CREATE TABLE user_profiles (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    bio TEXT,
    avatar_url VARCHAR(500),
    timezone VARCHAR(100) DEFAULT 'UTC',
    timezone_auto_detected BOOLEAN DEFAULT TRUE,
    locale VARCHAR(10) DEFAULT 'en-US',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    version INTEGER DEFAULT 1 -- Optimistic locking
);

CREATE INDEX idx_user_profiles_user_id ON user_profiles(user_id);
CREATE INDEX idx_user_profiles_timezone ON user_profiles(timezone);

-- User Settings Table
-- Stores user preferences and customization options
CREATE TABLE user_settings (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    theme VARCHAR(20) DEFAULT 'system' CHECK (theme IN ('light', 'dark', 'system')),
    language VARCHAR(10) DEFAULT 'auto',
    date_format VARCHAR(20) DEFAULT 'YYYY-MM-DD',
    time_format VARCHAR(10) DEFAULT '24h' CHECK (time_format IN ('12h', '24h')),
    week_start_day INTEGER DEFAULT 1 CHECK (week_start_day BETWEEN 0 AND 6), -- 0=Sunday, 1=Monday
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    version INTEGER DEFAULT 1 -- Optimistic locking
);

CREATE INDEX idx_user_settings_user_id ON user_settings(user_id);
CREATE INDEX idx_user_settings_theme ON user_settings(theme);

-- Notification Settings Table
-- Manages notification preferences including DND periods
CREATE TABLE notification_settings (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    email_notifications BOOLEAN DEFAULT TRUE,
    push_notifications BOOLEAN DEFAULT TRUE,
    checkin_reminders BOOLEAN DEFAULT TRUE,
    reminder_interval_minutes INTEGER DEFAULT 120 CHECK (reminder_interval_minutes IN (15, 30, 45, 60, 90, 120, 180, 240)),
    dnd_enabled BOOLEAN DEFAULT FALSE,
    dnd_start_time TIME,
    dnd_end_time TIME,
    dnd_days INTEGER[] DEFAULT ARRAY[1,2,3,4,5,6,7], -- Days of week: 1=Monday, 7=Sunday
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    version INTEGER DEFAULT 1 -- Optimistic locking
);

CREATE INDEX idx_notification_settings_user_id ON notification_settings(user_id);
CREATE INDEX idx_notification_settings_dnd_enabled ON notification_settings(dnd_enabled);

-- Constraint to ensure DND times are set if DND is enabled
ALTER TABLE notification_settings
ADD CONSTRAINT check_dnd_times
CHECK (
    (dnd_enabled = FALSE) OR
    (dnd_enabled = TRUE AND dnd_start_time IS NOT NULL AND dnd_end_time IS NOT NULL)
);

-- Data Retention Settings Table
-- Manages data retention and deletion policies
CREATE TABLE data_retention_settings (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    auto_delete_enabled BOOLEAN DEFAULT FALSE,
    retention_days INTEGER CHECK (retention_days IS NULL OR retention_days > 0),
    delete_after_inactivity_days INTEGER CHECK (delete_after_inactivity_days IS NULL OR delete_after_inactivity_days >= 30),
    last_activity_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    version INTEGER DEFAULT 1 -- Optimistic locking
);

CREATE INDEX idx_data_retention_user_id ON data_retention_settings(user_id);
CREATE INDEX idx_data_retention_auto_delete ON data_retention_settings(auto_delete_enabled);
CREATE INDEX idx_data_retention_last_activity ON data_retention_settings(last_activity_at);

-- Account Deletion Requests Table
-- Tracks account deletion requests with grace period
CREATE TABLE account_deletion_requests (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    requested_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    scheduled_deletion_at TIMESTAMP WITH TIME ZONE NOT NULL,
    reason TEXT,
    status VARCHAR(20) DEFAULT 'pending' CHECK (status IN ('pending', 'cancelled', 'completed')),
    cancelled_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_account_deletion_user_id ON account_deletion_requests(user_id);
CREATE INDEX idx_account_deletion_status ON account_deletion_requests(status);
CREATE INDEX idx_account_deletion_scheduled ON account_deletion_requests(scheduled_deletion_at);

-- Settings Audit Log Table
-- Tracks changes to sensitive settings for security and compliance
CREATE TABLE settings_audit_log (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    setting_type VARCHAR(50) NOT NULL, -- 'profile', 'notification', 'retention', 'theme', etc.
    setting_name VARCHAR(100) NOT NULL,
    old_value TEXT,
    new_value TEXT,
    changed_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    ip_address INET,
    user_agent TEXT
);

CREATE INDEX idx_settings_audit_user_id ON settings_audit_log(user_id);
CREATE INDEX idx_settings_audit_changed_at ON settings_audit_log(changed_at);
CREATE INDEX idx_settings_audit_setting_type ON settings_audit_log(setting_type);

-- Function to automatically create default settings for new users
CREATE OR REPLACE FUNCTION create_default_user_settings()
RETURNS TRIGGER AS $$
BEGIN
    -- Create default profile
    INSERT INTO user_profiles (user_id)
    VALUES (NEW.id);

    -- Create default settings
    INSERT INTO user_settings (user_id)
    VALUES (NEW.id);

    -- Create default notification settings
    INSERT INTO notification_settings (user_id)
    VALUES (NEW.id);

    -- Create default data retention settings
    INSERT INTO data_retention_settings (user_id)
    VALUES (NEW.id);

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger to create default settings for new users
CREATE TRIGGER create_user_default_settings
AFTER INSERT ON users
FOR EACH ROW
EXECUTE FUNCTION create_default_user_settings();

-- Triggers for updated_at columns
CREATE TRIGGER update_user_profiles_updated_at
BEFORE UPDATE ON user_profiles
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_user_settings_updated_at
BEFORE UPDATE ON user_settings
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_notification_settings_updated_at
BEFORE UPDATE ON notification_settings
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_data_retention_settings_updated_at
BEFORE UPDATE ON data_retention_settings
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_account_deletion_requests_updated_at
BEFORE UPDATE ON account_deletion_requests
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

-- Function to increment version for optimistic locking
CREATE OR REPLACE FUNCTION increment_version()
RETURNS TRIGGER AS $$
BEGIN
    NEW.version = OLD.version + 1;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Triggers for optimistic locking
CREATE TRIGGER increment_user_profiles_version
BEFORE UPDATE ON user_profiles
FOR EACH ROW
EXECUTE FUNCTION increment_version();

CREATE TRIGGER increment_user_settings_version
BEFORE UPDATE ON user_settings
FOR EACH ROW
EXECUTE FUNCTION increment_version();

CREATE TRIGGER increment_notification_settings_version
BEFORE UPDATE ON notification_settings
FOR EACH ROW
EXECUTE FUNCTION increment_version();

CREATE TRIGGER increment_data_retention_settings_version
BEFORE UPDATE ON data_retention_settings
FOR EACH ROW
EXECUTE FUNCTION increment_version();

-- Function to update last_activity_at on user activity
CREATE OR REPLACE FUNCTION update_last_activity()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE data_retention_settings
    SET last_activity_at = CURRENT_TIMESTAMP
    WHERE user_id = NEW.user_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger to update last activity on checkin creation
CREATE TRIGGER update_user_last_activity_on_checkin
AFTER INSERT ON checkins
FOR EACH ROW
EXECUTE FUNCTION update_last_activity();

-- Seed default settings for existing users
INSERT INTO user_profiles (user_id)
SELECT id FROM users WHERE id NOT IN (SELECT user_id FROM user_profiles);

INSERT INTO user_settings (user_id)
SELECT id FROM users WHERE id NOT IN (SELECT user_id FROM user_settings);

INSERT INTO notification_settings (user_id)
SELECT id FROM users WHERE id NOT IN (SELECT user_id FROM notification_settings);

INSERT INTO data_retention_settings (user_id)
SELECT id FROM users WHERE id NOT IN (SELECT user_id FROM data_retention_settings);

-- Comments for documentation
COMMENT ON TABLE user_profiles IS 'Extended user profile information including bio, avatar, timezone, and locale settings';
COMMENT ON TABLE user_settings IS 'User UI/UX preferences including theme, language, date/time formats';
COMMENT ON TABLE notification_settings IS 'Notification preferences including Do Not Disturb schedules and reminder intervals';
COMMENT ON TABLE data_retention_settings IS 'Data retention policies and automatic deletion settings';
COMMENT ON TABLE account_deletion_requests IS 'Tracks account deletion requests with grace period before permanent deletion';
COMMENT ON TABLE settings_audit_log IS 'Audit trail for settings changes for security and compliance';

COMMENT ON COLUMN user_profiles.version IS 'Version number for optimistic locking to prevent concurrent update conflicts';
COMMENT ON COLUMN notification_settings.dnd_days IS 'Array of days when DND is active (1=Monday, 7=Sunday)';
COMMENT ON COLUMN notification_settings.reminder_interval_minutes IS 'How often to send check-in reminders';
COMMENT ON COLUMN data_retention_settings.retention_days IS 'Number of days to retain data before auto-deletion (null = keep forever)';
COMMENT ON COLUMN data_retention_settings.delete_after_inactivity_days IS 'Delete account after this many days of inactivity (minimum 30 days)';
