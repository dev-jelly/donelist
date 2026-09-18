-- Rollback 2FA support migration

-- Remove triggers
DROP TRIGGER IF EXISTS log_user_security_changes ON users;

-- Remove functions
DROP FUNCTION IF EXISTS log_security_event();

-- Drop tables
DROP TABLE IF EXISTS account_recovery_tokens;
DROP TABLE IF EXISTS data_export_requests;
DROP TABLE IF EXISTS security_events;
DROP TABLE IF EXISTS password_history;
DROP TABLE IF EXISTS two_factor_backup_codes;

-- Remove profile privacy columns
ALTER TABLE user_profiles DROP COLUMN IF EXISTS privacy_level;
ALTER TABLE user_profiles DROP COLUMN IF EXISTS show_email;
ALTER TABLE user_profiles DROP COLUMN IF EXISTS show_activity;
ALTER TABLE user_profiles DROP COLUMN IF EXISTS searchable;

-- Remove 2FA columns from users table
ALTER TABLE users DROP COLUMN IF EXISTS two_factor_enabled;
ALTER TABLE users DROP COLUMN IF EXISTS two_factor_secret;
ALTER TABLE users DROP COLUMN IF EXISTS two_factor_verified_at;
ALTER TABLE users DROP COLUMN IF EXISTS recovery_email;
ALTER TABLE users DROP COLUMN IF EXISTS recovery_email_verified;