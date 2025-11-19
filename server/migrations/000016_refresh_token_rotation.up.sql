-- Add JWT rotation fields to refresh_tokens table
-- Note: revoked_at already exists from migration 000002
ALTER TABLE refresh_tokens
ADD COLUMN IF NOT EXISTS jti VARCHAR(255),
ADD COLUMN IF NOT EXISTS used_at TIMESTAMP WITH TIME ZONE;

-- Create index on JTI for quick revocation checks
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_jti ON refresh_tokens(jti) WHERE revoked_at IS NULL;

-- Update existing tokens to have a JTI (migration data)
-- For existing tokens without JTI, generate one from token_hash
UPDATE refresh_tokens
SET jti = CONCAT('legacy-', id::text)
WHERE jti IS NULL;

-- Make JTI NOT NULL after backfilling
ALTER TABLE refresh_tokens
ALTER COLUMN jti SET NOT NULL;

-- Add unique constraint on JTI to prevent duplicates
CREATE UNIQUE INDEX IF NOT EXISTS idx_refresh_tokens_jti_unique ON refresh_tokens(jti);

-- Update existing revoked tokens to have revoked_at timestamp
UPDATE refresh_tokens
SET revoked_at = created_at
WHERE revoked = TRUE AND revoked_at IS NULL;

-- Add comment on new columns
COMMENT ON COLUMN refresh_tokens.jti IS 'JWT ID (jti claim) for token tracking and revocation';
COMMENT ON COLUMN refresh_tokens.revoked_at IS 'Timestamp when token was revoked, NULL if active';
COMMENT ON COLUMN refresh_tokens.used_at IS 'Timestamp when token was used for refresh (one-time use enforcement)';
