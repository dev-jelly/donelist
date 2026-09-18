-- Drop signing keys table
DROP INDEX IF EXISTS idx_signing_keys_expired;
DROP INDEX IF EXISTS idx_signing_keys_active;
DROP INDEX IF EXISTS idx_signing_keys_user_id;
DROP TABLE IF EXISTS signing_keys;
