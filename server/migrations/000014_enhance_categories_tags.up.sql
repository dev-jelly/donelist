-- Enable pg_trgm extension for trigram similarity search (autocomplete)
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Enhance categories table
ALTER TABLE categories
    -- Add slug for URL-friendly names (e.g., "work-projects")
    ADD COLUMN slug VARCHAR(100),
    -- Add soft delete support
    ADD COLUMN deleted_at TIMESTAMP WITH TIME ZONE,
    -- Add usage tracking for statistics
    ADD COLUMN usage_count INTEGER DEFAULT 0 NOT NULL,
    -- Add metadata for future extensibility (custom fields, preferences, etc.)
    ADD COLUMN metadata JSONB DEFAULT '{}'::jsonb,
    -- Add description field
    ADD COLUMN description TEXT;

-- Update existing categories to generate slugs from names
UPDATE categories
SET slug = LOWER(REGEXP_REPLACE(REGEXP_REPLACE(name, '[^\w\s-]', '', 'g'), '\s+', '-', 'g'))
WHERE slug IS NULL;

-- Make slug required and add unique constraint
ALTER TABLE categories
    ALTER COLUMN slug SET NOT NULL,
    -- Unique constraint: user can't have duplicate slugs, but slug must allow NULL during deletion
    ADD CONSTRAINT unique_category_slug_per_user UNIQUE (user_id, slug);

-- Add color validation constraint (hex color format)
ALTER TABLE categories
    ADD CONSTRAINT check_category_color_format
    CHECK (color IS NULL OR color ~ '^#[0-9A-Fa-f]{6}$');

-- Create indexes for categories
CREATE INDEX idx_categories_slug ON categories(slug);
CREATE INDEX idx_categories_deleted_at ON categories(deleted_at) WHERE deleted_at IS NULL;
CREATE INDEX idx_categories_usage_count ON categories(usage_count DESC);
-- Trigram index for autocomplete on category names
CREATE INDEX idx_categories_name_trgm ON categories USING gin(name gin_trgm_ops);
-- GIN index for metadata JSONB queries
CREATE INDEX idx_categories_metadata ON categories USING gin(metadata);

-- Enhance tags table
ALTER TABLE tags
    -- Add slug for normalized tag names
    ADD COLUMN slug VARCHAR(100),
    -- Add soft delete support
    ADD COLUMN deleted_at TIMESTAMP WITH TIME ZONE,
    -- Add usage tracking
    ADD COLUMN usage_count INTEGER DEFAULT 0 NOT NULL,
    -- Add metadata for future extensibility
    ADD COLUMN metadata JSONB DEFAULT '{}'::jsonb,
    -- Add updated_at for consistency
    ADD COLUMN updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP;

-- Update existing tags to generate slugs from names
UPDATE tags
SET slug = LOWER(REGEXP_REPLACE(REGEXP_REPLACE(name, '[^\w\s-]', '', 'g'), '\s+', '-', 'g'))
WHERE slug IS NULL;

-- Make slug required and add unique constraint
ALTER TABLE tags
    ALTER COLUMN slug SET NOT NULL,
    ADD CONSTRAINT unique_tag_slug_per_user UNIQUE (user_id, slug);

-- Create indexes for tags
CREATE INDEX idx_tags_slug ON tags(slug);
CREATE INDEX idx_tags_deleted_at ON tags(deleted_at) WHERE deleted_at IS NULL;
CREATE INDEX idx_tags_usage_count ON tags(usage_count DESC);
-- Trigram index for autocomplete on tag names
CREATE INDEX idx_tags_name_trgm ON tags USING gin(name gin_trgm_ops);
-- GIN index for metadata JSONB queries
CREATE INDEX idx_tags_metadata ON tags USING gin(metadata);

-- Add trigger to update tags.updated_at
CREATE TRIGGER update_tags_updated_at BEFORE UPDATE ON tags
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Function to automatically update usage_count when tags are used
CREATE OR REPLACE FUNCTION update_tag_usage_count()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        UPDATE tags SET usage_count = usage_count + 1 WHERE id = NEW.tag_id;
    ELSIF TG_OP = 'DELETE' THEN
        UPDATE tags SET usage_count = GREATEST(usage_count - 1, 0) WHERE id = OLD.tag_id;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

-- Trigger to update tag usage count when checkin_tags changes
CREATE TRIGGER trigger_update_tag_usage_count
AFTER INSERT OR DELETE ON checkin_tags
FOR EACH ROW EXECUTE FUNCTION update_tag_usage_count();

-- Function to update category usage_count
CREATE OR REPLACE FUNCTION update_category_usage_count()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' AND NEW.category_id IS NOT NULL THEN
        UPDATE categories SET usage_count = usage_count + 1 WHERE id = NEW.category_id;
    ELSIF TG_OP = 'UPDATE' THEN
        IF OLD.category_id IS NOT NULL AND NEW.category_id IS NULL THEN
            UPDATE categories SET usage_count = GREATEST(usage_count - 1, 0) WHERE id = OLD.category_id;
        ELSIF OLD.category_id IS NULL AND NEW.category_id IS NOT NULL THEN
            UPDATE categories SET usage_count = usage_count + 1 WHERE id = NEW.category_id;
        ELSIF OLD.category_id IS NOT NULL AND NEW.category_id IS NOT NULL AND OLD.category_id != NEW.category_id THEN
            UPDATE categories SET usage_count = GREATEST(usage_count - 1, 0) WHERE id = OLD.category_id;
            UPDATE categories SET usage_count = usage_count + 1 WHERE id = NEW.category_id;
        END IF;
    ELSIF TG_OP = 'DELETE' AND OLD.category_id IS NOT NULL THEN
        UPDATE categories SET usage_count = GREATEST(usage_count - 1, 0) WHERE id = OLD.category_id;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

-- Trigger to update category usage count when checkins change
CREATE TRIGGER trigger_update_category_usage_count
AFTER INSERT OR UPDATE OR DELETE ON checkins
FOR EACH ROW EXECUTE FUNCTION update_category_usage_count();

-- Initialize usage counts for existing data
UPDATE tags t
SET usage_count = (
    SELECT COUNT(*)
    FROM checkin_tags ct
    JOIN checkins c ON ct.checkin_id = c.id
    WHERE ct.tag_id = t.id AND c.deleted_at IS NULL
);

UPDATE categories c
SET usage_count = (
    SELECT COUNT(*)
    FROM checkins ch
    WHERE ch.category_id = c.id AND ch.deleted_at IS NULL
);

-- Insert default category templates
-- These provide starter categories for new users
INSERT INTO categories (user_id, name, slug, color, icon, description, metadata) VALUES
    -- System templates (using a placeholder UUID - these will be copied per user)
    -- Note: In production, these should be inserted when a user signs up
    ('00000000-0000-0000-0000-000000000000'::uuid, 'Work', 'work', '#3B82F6', '💼', 'Work-related tasks and projects', '{"template": true, "category": "productivity"}'),
    ('00000000-0000-0000-0000-000000000000'::uuid, 'Personal', 'personal', '#10B981', '🏠', 'Personal activities and life tasks', '{"template": true, "category": "personal"}'),
    ('00000000-0000-0000-0000-000000000000'::uuid, 'Health', 'health', '#EF4444', '❤️', 'Health, fitness, and wellness', '{"template": true, "category": "health"}'),
    ('00000000-0000-0000-0000-000000000000'::uuid, 'Learning', 'learning', '#8B5CF6', '📚', 'Education and skill development', '{"template": true, "category": "education"}'),
    ('00000000-0000-0000-0000-000000000000'::uuid, 'Social', 'social', '#F59E0B', '👥', 'Social activities and relationships', '{"template": true, "category": "social"}'),
    ('00000000-0000-0000-0000-000000000000'::uuid, 'Hobbies', 'hobbies', '#EC4899', '🎨', 'Hobbies and recreational activities', '{"template": true, "category": "hobbies"}')
ON CONFLICT (user_id, slug) DO NOTHING;

-- Create a function to copy default templates to a new user
CREATE OR REPLACE FUNCTION create_default_categories_for_user(target_user_id UUID)
RETURNS void AS $$
BEGIN
    INSERT INTO categories (user_id, name, slug, color, icon, description, metadata)
    SELECT
        target_user_id,
        name,
        slug,
        color,
        icon,
        description,
        metadata - 'template' -- Remove template flag from user copies
    FROM categories
    WHERE user_id = '00000000-0000-0000-0000-000000000000'::uuid
    ON CONFLICT (user_id, slug) DO NOTHING;
END;
$$ LANGUAGE plpgsql;
