-- Rollback Migration 000015: User Settings and Profile Management

-- Drop triggers first
DROP TRIGGER IF EXISTS update_user_last_activity_on_checkin ON checkins;
DROP TRIGGER IF EXISTS increment_data_retention_settings_version ON data_retention_settings;
DROP TRIGGER IF EXISTS increment_notification_settings_version ON notification_settings;
DROP TRIGGER IF EXISTS increment_user_settings_version ON user_settings;
DROP TRIGGER IF EXISTS increment_user_profiles_version ON user_profiles;
DROP TRIGGER IF EXISTS update_account_deletion_requests_updated_at ON account_deletion_requests;
DROP TRIGGER IF EXISTS update_data_retention_settings_updated_at ON data_retention_settings;
DROP TRIGGER IF EXISTS update_notification_settings_updated_at ON notification_settings;
DROP TRIGGER IF EXISTS update_user_settings_updated_at ON user_settings;
DROP TRIGGER IF EXISTS update_user_profiles_updated_at ON user_profiles;
DROP TRIGGER IF EXISTS create_user_default_settings ON users;

-- Drop functions
DROP FUNCTION IF EXISTS update_last_activity();
DROP FUNCTION IF EXISTS increment_version();
DROP FUNCTION IF EXISTS create_default_user_settings();

-- Drop tables in reverse order of creation (respecting foreign key dependencies)
DROP TABLE IF EXISTS settings_audit_log;
DROP TABLE IF EXISTS account_deletion_requests;
DROP TABLE IF EXISTS data_retention_settings;
DROP TABLE IF EXISTS notification_settings;
DROP TABLE IF EXISTS user_settings;
DROP TABLE IF EXISTS user_profiles;
