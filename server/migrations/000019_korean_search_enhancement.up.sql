-- Migration: Enhanced Korean Language Search Support
-- This migration adds Korean-specific text search configuration and optimization

-- Enable required extensions if not already enabled
CREATE EXTENSION IF NOT EXISTS pg_trgm;      -- Trigram matching for fuzzy search
CREATE EXTENSION IF NOT EXISTS unaccent;     -- Remove accents (useful for romanization)

-- Create a custom Korean text search configuration
-- Based on 'simple' parser to avoid English-specific stemming

-- Step 1: Create Korean text search configuration
DROP TEXT SEARCH CONFIGURATION IF EXISTS korean CASCADE;
CREATE TEXT SEARCH CONFIGURATION korean (PARSER = default);

-- Configure token types for Korean
-- We want to index: words, numbers, and email/URLs, but skip punctuation
ALTER TEXT SEARCH CONFIGURATION korean
    ADD MAPPING FOR asciiword, word, numword, email, url, host, file, sfloat, float, int, uint
    WITH simple;

-- Create custom Korean dictionary that handles common Korean patterns
-- This is a simple approach; can be enhanced with custom dictionary files later

-- Step 2: Add trigram indexes for fuzzy Korean search
-- These indexes help with typos and partial matching
CREATE INDEX IF NOT EXISTS idx_checkins_content_trgm
    ON checkins USING gin (content gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_categories_name_trgm
    ON categories USING gin (name gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_tags_name_trgm
    ON tags USING gin (name gin_trgm_ops);

-- Step 3: Update search vector functions to use Korean configuration

-- Drop existing functions and recreate with Korean support
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

    -- Build search vector with Korean configuration and weighted fields
    -- Content (A weight): Most important - use Korean config
    -- Category name (B weight): Important - use Korean config
    -- Tags (C weight): Less important but still relevant - use Korean config
    NEW.search_vector :=
        setweight(to_tsvector('korean', COALESCE(NEW.content, '')), 'A') ||
        setweight(to_tsvector('korean', COALESCE(category_name, '')), 'B') ||
        setweight(to_tsvector('korean', COALESCE(tag_names, '')), 'C');

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Recreate trigger
DROP TRIGGER IF EXISTS checkins_search_vector_update ON checkins;
CREATE TRIGGER checkins_search_vector_update
    BEFORE INSERT OR UPDATE OF content, category_id
    ON checkins
    FOR EACH ROW
    EXECUTE FUNCTION update_checkin_search_vector();

-- Update category search vector function
DROP FUNCTION IF EXISTS update_category_search_vector() CASCADE;
CREATE OR REPLACE FUNCTION update_category_search_vector()
RETURNS TRIGGER AS $$
BEGIN
    NEW.search_vector :=
        setweight(to_tsvector('korean', COALESCE(NEW.name, '')), 'A');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Recreate category trigger
DROP TRIGGER IF EXISTS categories_search_vector_update ON categories;
CREATE TRIGGER categories_search_vector_update
    BEFORE INSERT OR UPDATE OF name
    ON categories
    FOR EACH ROW
    EXECUTE FUNCTION update_category_search_vector();

-- Update tag search vector function
DROP FUNCTION IF EXISTS update_tag_search_vector() CASCADE;
CREATE OR REPLACE FUNCTION update_tag_search_vector()
RETURNS TRIGGER AS $$
BEGIN
    NEW.search_vector := to_tsvector('korean', COALESCE(NEW.name, ''));
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Recreate tag trigger
DROP TRIGGER IF EXISTS tags_search_vector_update ON tags;
CREATE TRIGGER tags_search_vector_update
    BEFORE INSERT OR UPDATE OF name
    ON tags
    FOR EACH ROW
    EXECUTE FUNCTION update_tag_search_vector();

-- Step 4: Create helper function for Korean search with fuzzy matching
-- This function combines full-text search with trigram similarity
CREATE OR REPLACE FUNCTION search_checkins_korean(
    p_user_id UUID,
    p_query TEXT,
    p_similarity_threshold REAL DEFAULT 0.3,
    p_limit INTEGER DEFAULT 100
)
RETURNS TABLE (
    checkin_id UUID,
    content TEXT,
    checkin_time TIMESTAMP WITH TIME ZONE,
    rank REAL,
    similarity REAL
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        c.id AS checkin_id,
        c.content,
        c.checkin_time,
        ts_rank(c.search_vector, to_tsquery('korean', p_query)) AS rank,
        similarity(c.content, p_query) AS similarity
    FROM checkins c
    WHERE
        c.user_id = p_user_id
        AND c.deleted_at IS NULL
        AND (
            -- Full-text search match
            c.search_vector @@ to_tsquery('korean', p_query)
            OR
            -- Trigram similarity match (fuzzy)
            similarity(c.content, p_query) > p_similarity_threshold
        )
    ORDER BY
        -- Prioritize full-text matches, then similarity
        (c.search_vector @@ to_tsquery('korean', p_query)) DESC,
        ts_rank(c.search_vector, to_tsquery('korean', p_query)) DESC,
        similarity(c.content, p_query) DESC
    LIMIT p_limit;
END;
$$ LANGUAGE plpgsql STABLE;

-- Step 5: Create helper function for phrase search (exact matching)
CREATE OR REPLACE FUNCTION search_checkins_phrase(
    p_user_id UUID,
    p_phrase TEXT,
    p_limit INTEGER DEFAULT 100
)
RETURNS TABLE (
    checkin_id UUID,
    content TEXT,
    checkin_time TIMESTAMP WITH TIME ZONE,
    rank REAL
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        c.id AS checkin_id,
        c.content,
        c.checkin_time,
        ts_rank(c.search_vector, phraseto_tsquery('korean', p_phrase)) AS rank
    FROM checkins c
    WHERE
        c.user_id = p_user_id
        AND c.deleted_at IS NULL
        AND c.search_vector @@ phraseto_tsquery('korean', p_phrase)
    ORDER BY
        ts_rank(c.search_vector, phraseto_tsquery('korean', p_phrase)) DESC
    LIMIT p_limit;
END;
$$ LANGUAGE plpgsql STABLE;

-- Step 6: Create a view for common Korean search patterns
CREATE OR REPLACE VIEW korean_search_stats AS
SELECT
    date_trunc('day', created_at) as search_date,
    substring(query for 50) as search_query,
    COUNT(*) as query_count,
    AVG(result_count) as avg_results
FROM search_history
WHERE LENGTH(query) > 0
GROUP BY date_trunc('day', created_at), substring(query for 50)
ORDER BY search_date DESC, query_count DESC;

-- Step 7: Reindex all existing data with Korean configuration
-- This updates search_vector for all existing records
UPDATE checkins SET updated_at = updated_at WHERE deleted_at IS NULL;
UPDATE categories SET updated_at = updated_at;
UPDATE tags SET created_at = created_at;

-- Analyze tables for better query planning
ANALYZE checkins;
ANALYZE categories;
ANALYZE tags;

-- Add comments documenting Korean search enhancement
COMMENT ON TEXT SEARCH CONFIGURATION korean IS
'Korean text search configuration optimized for Korean language content';

COMMENT ON INDEX idx_checkins_content_trgm IS
'Trigram GIN index for fuzzy Korean search on checkin content';

COMMENT ON INDEX idx_categories_name_trgm IS
'Trigram GIN index for fuzzy Korean search on category names';

COMMENT ON INDEX idx_tags_name_trgm IS
'Trigram GIN index for fuzzy Korean search on tag names';

COMMENT ON FUNCTION search_checkins_korean IS
'Search checkins with Korean text search and trigram fuzzy matching.
Combines full-text search with similarity matching for better Korean results.
Parameters:
- p_user_id: User UUID to search within
- p_query: Search query (will be parsed as tsquery)
- p_similarity_threshold: Minimum trigram similarity (0.0-1.0, default 0.3)
- p_limit: Maximum results to return (default 100)';

COMMENT ON FUNCTION search_checkins_phrase IS
'Phrase search for exact Korean text matching.
Parameters:
- p_user_id: User UUID to search within
- p_phrase: Exact phrase to search for
- p_limit: Maximum results to return (default 100)';

COMMENT ON VIEW korean_search_stats IS
'Daily aggregated statistics of search queries for Korean content analysis';
