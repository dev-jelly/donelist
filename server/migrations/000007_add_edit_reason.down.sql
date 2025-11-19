-- Remove edit_reason column and index
DROP INDEX IF EXISTS idx_edit_history_edit_reason;
ALTER TABLE edit_history DROP COLUMN IF EXISTS edit_reason;

-- Remove version column
ALTER TABLE checkins DROP COLUMN IF EXISTS version;

-- Revert tier check constraint
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_tier_check;
ALTER TABLE users ADD CONSTRAINT users_tier_check CHECK (tier IN ('free', 'premium'));
