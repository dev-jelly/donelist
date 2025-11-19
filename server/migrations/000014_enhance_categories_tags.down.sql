-- Drop the default category templates function
DROP FUNCTION IF EXISTS create_default_categories_for_user(UUID);

-- Drop default category templates
DELETE FROM categories WHERE user_id = '00000000-0000-0000-0000-000000000000'::uuid;

-- Drop triggers before dropping functions
DROP TRIGGER IF EXISTS trigger_update_category_usage_count ON checkins;
DROP TRIGGER IF EXISTS trigger_update_tag_usage_count ON checkin_tags;
DROP TRIGGER IF EXISTS update_tags_updated_at ON tags;

-- Drop functions
DROP FUNCTION IF EXISTS update_category_usage_count();
DROP FUNCTION IF EXISTS update_tag_usage_count();

-- Drop indexes for categories
DROP INDEX IF EXISTS idx_categories_metadata;
DROP INDEX IF EXISTS idx_categories_name_trgm;
DROP INDEX IF EXISTS idx_categories_usage_count;
DROP INDEX IF EXISTS idx_categories_deleted_at;
DROP INDEX IF EXISTS idx_categories_slug;

-- Remove constraints from categories
ALTER TABLE categories
    DROP CONSTRAINT IF EXISTS check_category_color_format,
    DROP CONSTRAINT IF EXISTS unique_category_slug_per_user;

-- Remove columns from categories
ALTER TABLE categories
    DROP COLUMN IF EXISTS description,
    DROP COLUMN IF EXISTS metadata,
    DROP COLUMN IF EXISTS usage_count,
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS slug;

-- Drop indexes for tags
DROP INDEX IF EXISTS idx_tags_metadata;
DROP INDEX IF EXISTS idx_tags_name_trgm;
DROP INDEX IF EXISTS idx_tags_usage_count;
DROP INDEX IF EXISTS idx_tags_deleted_at;
DROP INDEX IF EXISTS idx_tags_slug;

-- Remove constraints from tags
ALTER TABLE tags
    DROP CONSTRAINT IF EXISTS unique_tag_slug_per_user;

-- Remove columns from tags
ALTER TABLE tags
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS metadata,
    DROP COLUMN IF EXISTS usage_count,
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS slug;

-- Note: We don't drop pg_trgm extension as other parts of the system may use it
-- If you want to drop it, uncomment the following line:
-- DROP EXTENSION IF EXISTS pg_trgm;
