-- Drop triggers first
DROP TRIGGER IF EXISTS auto_expire_user_tier ON users;
DROP TRIGGER IF EXISTS update_checkins_updated_at ON checkins;
DROP TRIGGER IF EXISTS update_categories_updated_at ON categories;
DROP TRIGGER IF EXISTS update_users_updated_at ON users;

-- Drop functions
DROP FUNCTION IF EXISTS check_user_tier_expiry();
DROP FUNCTION IF EXISTS update_updated_at_column();

-- Drop tables in reverse order (respecting foreign key constraints)
DROP TABLE IF EXISTS edit_history;
DROP TABLE IF EXISTS checkin_tags;
DROP TABLE IF EXISTS checkins;
DROP TABLE IF EXISTS tags;
DROP TABLE IF EXISTS categories;
DROP TABLE IF EXISTS users;

-- Drop extension
DROP EXTENSION IF EXISTS "uuid-ossp";
