-- Drop admin permissions table
DROP TABLE IF EXISTS admin_permissions;

-- Drop admin audit log table
DROP TABLE IF EXISTS admin_audit_log;

-- Drop role index
DROP INDEX IF EXISTS idx_users_role;

-- Remove role column from users
ALTER TABLE users DROP COLUMN IF EXISTS role;
