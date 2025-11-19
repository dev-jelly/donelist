-- Drop view
DROP VIEW IF EXISTS active_team_members;

-- Drop triggers
DROP TRIGGER IF EXISTS update_team_members_updated_at ON team_members;
DROP TRIGGER IF EXISTS update_teams_updated_at ON teams;

-- Drop function
DROP FUNCTION IF EXISTS update_updated_at_column();

-- Remove team columns from categories
DROP INDEX IF EXISTS idx_categories_team_id;
ALTER TABLE categories DROP COLUMN IF EXISTS is_shared;
ALTER TABLE categories DROP COLUMN IF EXISTS team_id;

-- Remove team columns from checkins
DROP INDEX IF EXISTS idx_checkins_team_id;
ALTER TABLE checkins DROP COLUMN IF EXISTS visibility;
ALTER TABLE checkins DROP COLUMN IF EXISTS team_id;

-- Drop team_members table
DROP INDEX IF EXISTS idx_team_members_deleted_at;
DROP INDEX IF EXISTS idx_team_members_user_id;
DROP INDEX IF EXISTS idx_team_members_team_id;
DROP TABLE IF EXISTS team_members;

-- Drop teams table
DROP INDEX IF EXISTS idx_teams_deleted_at;
DROP INDEX IF EXISTS idx_teams_owner_id;
DROP TABLE IF EXISTS teams;

-- Remove mode preference from users
ALTER TABLE users DROP COLUMN IF EXISTS mode_preference;
