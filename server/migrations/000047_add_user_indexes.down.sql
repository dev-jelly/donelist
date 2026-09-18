-- Rollback user performance indexes

DROP INDEX IF EXISTS idx_users_email_deleted;
DROP INDEX IF EXISTS idx_users_auth_lookup;
DROP INDEX IF EXISTS idx_users_id_deleted;
