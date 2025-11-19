-- Add edit_reason column to edit_history table
ALTER TABLE edit_history ADD COLUMN edit_reason TEXT;

-- Create index for faster queries on edit_reason
CREATE INDEX idx_edit_history_edit_reason ON edit_history(edit_reason) WHERE edit_reason IS NOT NULL;

-- Add version column to checkins for optimistic locking
ALTER TABLE checkins ADD COLUMN version INTEGER DEFAULT 1 NOT NULL;

-- Update enterprise tier support in users table
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_tier_check;
ALTER TABLE users ADD CONSTRAINT users_tier_check CHECK (tier IN ('free', 'premium', 'enterprise'));
