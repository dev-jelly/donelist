-- Rollback: Remove Full-Text Search Infrastructure

-- Drop triggers
DROP TRIGGER IF EXISTS checkin_tags_search_update ON checkin_tags;
DROP TRIGGER IF EXISTS checkins_search_vector_update ON checkins;
DROP TRIGGER IF EXISTS categories_search_vector_update ON categories;
DROP TRIGGER IF EXISTS tags_search_vector_update ON tags;

-- Drop functions
DROP FUNCTION IF EXISTS update_checkin_search_on_tag_change();
DROP FUNCTION IF EXISTS update_checkin_search_vector();
DROP FUNCTION IF EXISTS update_category_search_vector();
DROP FUNCTION IF EXISTS update_tag_search_vector();

-- Drop columns
ALTER TABLE checkins DROP COLUMN IF EXISTS search_vector;
ALTER TABLE categories DROP COLUMN IF EXISTS search_vector;
ALTER TABLE tags DROP COLUMN IF EXISTS search_vector;
