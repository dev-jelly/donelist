-- Revert refresh token rotation fields
DROP INDEX IF EXISTS idx_refresh_tokens_jti_unique;
DROP INDEX IF EXISTS idx_refresh_tokens_jti;

ALTER TABLE refresh_tokens
DROP COLUMN IF EXISTS used_at,
DROP COLUMN IF EXISTS jti;
