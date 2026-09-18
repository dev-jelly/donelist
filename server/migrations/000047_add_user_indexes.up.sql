-- Add missing database indexes for query performance optimization
-- Expected 10x improvement (5-20ms to 0.5-2ms)

-- Index for email lookups (frequently used in authentication)
CREATE INDEX IF NOT EXISTS idx_users_email_deleted ON users(email) WHERE deleted_at IS NULL;

-- Covering index for authentication queries (includes password_hash and role)
-- This allows index-only scans for login operations
CREATE INDEX IF NOT EXISTS idx_users_auth_lookup ON users(email, deleted_at)
INCLUDE (id, password_hash, role, display_name, tier, created_at, updated_at);

-- Index for active user queries by ID
CREATE INDEX IF NOT EXISTS idx_users_id_deleted ON users(id) WHERE deleted_at IS NULL;

-- Comments for documentation
COMMENT ON INDEX idx_users_email_deleted IS 'Partial index for email lookups excluding soft-deleted users';
COMMENT ON INDEX idx_users_auth_lookup IS 'Covering index for authentication queries - enables index-only scans';
COMMENT ON INDEX idx_users_id_deleted IS 'Partial index for ID lookups excluding soft-deleted users';
