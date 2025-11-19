-- Rollback: Remove Search History and Saved Searches Features

-- Drop trigger
DROP TRIGGER IF EXISTS update_saved_searches_updated_at ON saved_searches;

-- Drop tables
DROP TABLE IF EXISTS saved_searches;
DROP TABLE IF EXISTS search_history;
