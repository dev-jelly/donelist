-- Create signing keys table for request signing
CREATE TABLE IF NOT EXISTS signing_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key_hash VARCHAR(64) NOT NULL, -- SHA-256 hash of the signing key
    name VARCHAR(255) NOT NULL,
    algorithm VARCHAR(50) NOT NULL DEFAULT 'hmac-sha256',
    is_active BOOLEAN NOT NULL DEFAULT true,
    last_used TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP,
    CONSTRAINT fk_signing_keys_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Create index for efficient lookups by user
CREATE INDEX IF NOT EXISTS idx_signing_keys_user_id ON signing_keys(user_id);

-- Create index for active keys
CREATE INDEX IF NOT EXISTS idx_signing_keys_active ON signing_keys(user_id, is_active, expires_at) WHERE is_active = true;

-- Create index for cleanup of expired keys
CREATE INDEX IF NOT EXISTS idx_signing_keys_expired ON signing_keys(expires_at) WHERE expires_at IS NOT NULL;

-- Add comment to table
COMMENT ON TABLE signing_keys IS 'Stores signing keys for HMAC-based request signing to ensure request integrity and authenticity';

-- Add comments to columns
COMMENT ON COLUMN signing_keys.key_hash IS 'SHA-256 hash of the signing key for verification';
COMMENT ON COLUMN signing_keys.algorithm IS 'Signing algorithm used (currently only hmac-sha256)';
COMMENT ON COLUMN signing_keys.is_active IS 'Whether the key is currently active and can be used';
COMMENT ON COLUMN signing_keys.last_used IS 'Last time this key was used for signing a request';
COMMENT ON COLUMN signing_keys.expires_at IS 'Optional expiration timestamp for the key';
