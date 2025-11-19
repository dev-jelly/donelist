-- Rollback: Enhanced Korean Language Search Support

-- Drop helper functions
DROP FUNCTION IF EXISTS search_checkins_korean(UUID, TEXT, REAL, INTEGER);
DROP FUNCTION IF EXISTS search_checkins_phrase(UUID, TEXT, INTEGER);

-- Drop view
DROP VIEW IF EXISTS korean_search_stats;

-- Drop trigram indexes
DROP INDEX IF EXISTS idx_checkins_content_trgm;
DROP INDEX IF EXISTS idx_categories_name_trgm;
DROP INDEX IF EXISTS idx_tags_name_trgm;

-- Restore original functions with English configuration
DROP FUNCTION IF EXISTS update_checkin_search_vector() CASCADE;
CREATE OR REPLACE FUNCTION update_checkin_search_vector()
RETURNS TRIGGER AS $$
DECLARE
    category_name TEXT := '';
    tag_names TEXT := '';
BEGIN
    -- Get category name if exists
    IF NEW.category_id IS NOT NULL THEN
        SELECT name INTO category_name
        FROM categories
        WHERE id = NEW.category_id;
    END IF;

    -- Get all tag names for this checkin
    SELECT string_agg(t.name, ' ') INTO tag_names
    FROM tags t
    INNER JOIN checkin_tags ct ON ct.tag_id = t.id
    WHERE ct.checkin_id = NEW.id;

    -- Build search vector with weighted fields (using English config)
    NEW.search_vector :=
        setweight(to_tsvector('english', COALESCE(NEW.content, '')), 'A') ||
        setweight(to_tsvector('english', COALESCE(category_name, '')), 'B') ||
        setweight(to_tsvector('english', COALESCE(tag_names, '')), 'C');

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Recreate trigger
CREATE TRIGGER checkins_search_vector_update
    BEFORE INSERT OR UPDATE OF content, category_id
    ON checkins
    FOR EACH ROW
    EXECUTE FUNCTION update_checkin_search_vector();

-- Restore category function
DROP FUNCTION IF EXISTS update_category_search_vector() CASCADE;
CREATE OR REPLACE FUNCTION update_category_search_vector()
RETURNS TRIGGER AS $$
BEGIN
    NEW.search_vector :=
        setweight(to_tsvector('english', COALESCE(NEW.name, '')), 'A');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Recreate category trigger
CREATE TRIGGER categories_search_vector_update
    BEFORE INSERT OR UPDATE OF name
    ON categories
    FOR EACH ROW
    EXECUTE FUNCTION update_category_search_vector();

-- Restore tag function
DROP FUNCTION IF EXISTS update_tag_search_vector() CASCADE;
CREATE OR REPLACE FUNCTION update_tag_search_vector()
RETURNS TRIGGER AS $$
BEGIN
    NEW.search_vector := to_tsvector('english', COALESCE(NEW.name, ''));
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Recreate tag trigger
CREATE TRIGGER tags_search_vector_update
    BEFORE INSERT OR UPDATE OF name
    ON tags
    FOR EACH ROW
    EXECUTE FUNCTION update_tag_search_vector();

-- Drop Korean text search configuration
DROP TEXT SEARCH CONFIGURATION IF EXISTS korean CASCADE;

-- Reindex with English configuration
UPDATE checkins SET updated_at = updated_at WHERE deleted_at IS NULL;
UPDATE categories SET updated_at = updated_at;
UPDATE tags SET created_at = created_at;

-- Analyze tables
ANALYZE checkins;
ANALYZE categories;
ANALYZE tags;
