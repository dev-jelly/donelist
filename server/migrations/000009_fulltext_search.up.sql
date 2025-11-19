-- Migration: Add PostgreSQL Full-Text Search Infrastructure
-- This migration adds tsvector columns and triggers for full-text search

-- Add tsvector column to checkins table for full-text search
ALTER TABLE checkins ADD COLUMN search_vector tsvector;

-- Create function to update checkin search vector
-- Combines content with category name and tags for comprehensive search
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

    -- Build search vector with weighted fields
    -- Content (A weight): Most important
    -- Category name (B weight): Important
    -- Tags (C weight): Less important but still relevant
    NEW.search_vector :=
        setweight(to_tsvector('english', COALESCE(NEW.content, '')), 'A') ||
        setweight(to_tsvector('english', COALESCE(category_name, '')), 'B') ||
        setweight(to_tsvector('english', COALESCE(tag_names, '')), 'C');

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger to update search vector on INSERT or UPDATE
CREATE TRIGGER checkins_search_vector_update
    BEFORE INSERT OR UPDATE OF content, category_id
    ON checkins
    FOR EACH ROW
    EXECUTE FUNCTION update_checkin_search_vector();

-- Add tsvector column to categories table
ALTER TABLE categories ADD COLUMN search_vector tsvector;

-- Create function to update category search vector
CREATE OR REPLACE FUNCTION update_category_search_vector()
RETURNS TRIGGER AS $$
BEGIN
    NEW.search_vector :=
        setweight(to_tsvector('english', COALESCE(NEW.name, '')), 'A');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger for categories
CREATE TRIGGER categories_search_vector_update
    BEFORE INSERT OR UPDATE OF name
    ON categories
    FOR EACH ROW
    EXECUTE FUNCTION update_category_search_vector();

-- Add tsvector column to tags table
ALTER TABLE tags ADD COLUMN search_vector tsvector;

-- Create function to update tag search vector
CREATE OR REPLACE FUNCTION update_tag_search_vector()
RETURNS TRIGGER AS $$
BEGIN
    NEW.search_vector := to_tsvector('english', COALESCE(NEW.name, ''));
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger for tags
CREATE TRIGGER tags_search_vector_update
    BEFORE INSERT OR UPDATE OF name
    ON tags
    FOR EACH ROW
    EXECUTE FUNCTION update_tag_search_vector();

-- Create function to update checkin search vector when tags change
-- This ensures checkin search vectors stay in sync when tags are added/removed
CREATE OR REPLACE FUNCTION update_checkin_search_on_tag_change()
RETURNS TRIGGER AS $$
DECLARE
    checkin_record RECORD;
BEGIN
    -- Get the checkin ID from either OLD or NEW
    IF TG_OP = 'DELETE' THEN
        -- Fetch checkin and trigger its update
        SELECT * INTO checkin_record FROM checkins WHERE id = OLD.checkin_id;
        IF FOUND THEN
            UPDATE checkins SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.checkin_id;
        END IF;
    ELSE
        -- For INSERT or UPDATE
        SELECT * INTO checkin_record FROM checkins WHERE id = NEW.checkin_id;
        IF FOUND THEN
            UPDATE checkins SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.checkin_id;
        END IF;
    END IF;

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    ELSE
        RETURN NEW;
    END IF;
END;
$$ LANGUAGE plpgsql;

-- Create trigger on checkin_tags to update checkin search vector
CREATE TRIGGER checkin_tags_search_update
    AFTER INSERT OR DELETE ON checkin_tags
    FOR EACH ROW
    EXECUTE FUNCTION update_checkin_search_on_tag_change();

-- Populate search vectors for existing data
UPDATE checkins SET updated_at = updated_at; -- Triggers search vector update
UPDATE categories SET updated_at = updated_at; -- Triggers search vector update
UPDATE tags SET created_at = created_at; -- Triggers search vector update

-- Add comment to document the search vector columns
COMMENT ON COLUMN checkins.search_vector IS 'Full-text search vector combining content, category name, and tags';
COMMENT ON COLUMN categories.search_vector IS 'Full-text search vector for category name';
COMMENT ON COLUMN tags.search_vector IS 'Full-text search vector for tag name';
