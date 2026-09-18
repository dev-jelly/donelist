-- Add timezone support to users table and notification settings

-- Add timezone column to users table
ALTER TABLE users
ADD COLUMN timezone VARCHAR(100) DEFAULT 'UTC',
ADD COLUMN timezone_auto_detect BOOLEAN DEFAULT true,
ADD COLUMN last_timezone VARCHAR(100),
ADD COLUMN timezone_changed_at TIMESTAMP WITH TIME ZONE;

-- Create index for timezone queries
CREATE INDEX idx_users_timezone ON users(timezone);

-- Add timezone and DND fields to notification settings if they don't exist
DO $$
BEGIN
    -- Check if notification_settings table exists
    IF EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'notification_settings') THEN
        -- Add timezone column if it doesn't exist
        IF NOT EXISTS (SELECT FROM information_schema.columns
                      WHERE table_name = 'notification_settings'
                      AND column_name = 'timezone') THEN
            ALTER TABLE notification_settings
            ADD COLUMN timezone VARCHAR(100) DEFAULT 'UTC';
        END IF;

        -- Add DND fields if they don't exist
        IF NOT EXISTS (SELECT FROM information_schema.columns
                      WHERE table_name = 'notification_settings'
                      AND column_name = 'dnd_enabled') THEN
            ALTER TABLE notification_settings
            ADD COLUMN dnd_enabled BOOLEAN DEFAULT false,
            ADD COLUMN dnd_start_time TIME,
            ADD COLUMN dnd_end_time TIME,
            ADD COLUMN dnd_days INTEGER[] DEFAULT ARRAY[1,2,3,4,5]; -- Weekdays by default
        END IF;
    END IF;
END $$;

-- Create DND overrides table
CREATE TABLE IF NOT EXISTS dnd_overrides (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    start_time TIMESTAMP WITH TIME ZONE NOT NULL,
    end_time TIMESTAMP WITH TIME ZONE NOT NULL,
    reason VARCHAR(50) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    -- Constraints
    CONSTRAINT dnd_override_valid_time CHECK (end_time > start_time),
    CONSTRAINT dnd_override_valid_duration CHECK (end_time <= start_time + INTERVAL '7 days')
);

-- Create indexes for DND overrides
CREATE INDEX idx_dnd_overrides_user_id ON dnd_overrides(user_id);
CREATE INDEX idx_dnd_overrides_active ON dnd_overrides(user_id, start_time, end_time)
    WHERE end_time > NOW();
CREATE INDEX idx_dnd_overrides_cleanup ON dnd_overrides(end_time)
    WHERE end_time < NOW();

-- Create timezone change audit table for tracking user travels
CREATE TABLE IF NOT EXISTS user_timezone_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    old_timezone VARCHAR(100),
    new_timezone VARCHAR(100) NOT NULL,
    changed_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    change_source VARCHAR(50) DEFAULT 'manual', -- manual, auto_detect, api
    ip_address INET,
    user_agent TEXT
);

-- Create index for timezone history
CREATE INDEX idx_user_timezone_history_user_id ON user_timezone_history(user_id, changed_at DESC);
CREATE INDEX idx_user_timezone_history_changed_at ON user_timezone_history(changed_at);

-- Add comments for documentation
COMMENT ON TABLE dnd_overrides IS 'Temporary DND overrides for urgent notifications or travel';
COMMENT ON TABLE user_timezone_history IS 'Audit log of user timezone changes for travel detection';
COMMENT ON COLUMN users.timezone IS 'User primary timezone (IANA timezone name)';
COMMENT ON COLUMN users.timezone_auto_detect IS 'Whether to auto-detect timezone from device';
COMMENT ON COLUMN users.last_timezone IS 'Previous timezone before most recent change';
COMMENT ON COLUMN users.timezone_changed_at IS 'When timezone was last changed (for travel detection)';
COMMENT ON COLUMN dnd_overrides.reason IS 'Reason for override: urgent, emergency, manual, vip, critical, travel';

-- Create function to automatically log timezone changes
CREATE OR REPLACE FUNCTION log_timezone_change()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.timezone IS DISTINCT FROM OLD.timezone THEN
        INSERT INTO user_timezone_history (
            user_id,
            old_timezone,
            new_timezone,
            changed_at
        ) VALUES (
            NEW.id,
            OLD.timezone,
            NEW.timezone,
            NOW()
        );

        NEW.last_timezone := OLD.timezone;
        NEW.timezone_changed_at := NOW();
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger for timezone change logging
DROP TRIGGER IF EXISTS trigger_log_timezone_change ON users;
CREATE TRIGGER trigger_log_timezone_change
    BEFORE UPDATE ON users
    FOR EACH ROW
    WHEN (NEW.timezone IS DISTINCT FROM OLD.timezone)
    EXECUTE FUNCTION log_timezone_change();

-- Create function to cleanup expired DND overrides
CREATE OR REPLACE FUNCTION cleanup_expired_dnd_overrides()
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM dnd_overrides
    WHERE end_time < NOW() - INTERVAL '7 days';

    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- Grant permissions
GRANT SELECT, INSERT, UPDATE, DELETE ON dnd_overrides TO donelist_user;
GRANT SELECT, INSERT ON user_timezone_history TO donelist_user;
GRANT EXECUTE ON FUNCTION cleanup_expired_dnd_overrides() TO donelist_user;
