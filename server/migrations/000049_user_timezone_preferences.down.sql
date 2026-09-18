-- Rollback timezone support

-- Drop trigger and function
DROP TRIGGER IF EXISTS trigger_log_timezone_change ON users;
DROP FUNCTION IF EXISTS log_timezone_change();
DROP FUNCTION IF EXISTS cleanup_expired_dnd_overrides();

-- Drop timezone history table
DROP TABLE IF EXISTS user_timezone_history;

-- Drop DND overrides table
DROP TABLE IF EXISTS dnd_overrides;

-- Remove timezone columns from users table
ALTER TABLE users
DROP COLUMN IF EXISTS timezone,
DROP COLUMN IF EXISTS timezone_auto_detect,
DROP COLUMN IF EXISTS last_timezone,
DROP COLUMN IF EXISTS timezone_changed_at;

-- Remove DND columns from notification_settings if they exist
DO $$
BEGIN
    IF EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'notification_settings') THEN
        ALTER TABLE notification_settings
        DROP COLUMN IF EXISTS timezone,
        DROP COLUMN IF EXISTS dnd_enabled,
        DROP COLUMN IF EXISTS dnd_start_time,
        DROP COLUMN IF EXISTS dnd_end_time,
        DROP COLUMN IF EXISTS dnd_days;
    END IF;
END $$;

-- Drop indexes
DROP INDEX IF EXISTS idx_users_timezone;
