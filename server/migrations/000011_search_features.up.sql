-- Migration: Add Search History and Saved Searches Features

-- Search history table - tracks user search queries for analytics and suggestions
CREATE TABLE search_history (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    query TEXT NOT NULL,
    filters JSONB DEFAULT '{}', -- Stores filter parameters as JSON
    result_count INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_search_history_user_id ON search_history(user_id);
CREATE INDEX idx_search_history_created_at ON search_history(created_at DESC);
CREATE INDEX idx_search_history_user_recent ON search_history(user_id, created_at DESC);

-- Saved searches table - allows premium users to save frequently used searches
CREATE TABLE saved_searches (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    query TEXT,
    filters JSONB DEFAULT '{}', -- Stores filter parameters as JSON
    is_favorite BOOLEAN DEFAULT FALSE,
    last_used_at TIMESTAMP WITH TIME ZONE,
    use_count INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, name)
);

CREATE INDEX idx_saved_searches_user_id ON saved_searches(user_id);
CREATE INDEX idx_saved_searches_favorite ON saved_searches(user_id, is_favorite);
CREATE INDEX idx_saved_searches_last_used ON saved_searches(user_id, last_used_at DESC);

-- Create trigger for updated_at on saved_searches
CREATE TRIGGER update_saved_searches_updated_at BEFORE UPDATE ON saved_searches
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Add comments
COMMENT ON TABLE search_history IS 'Tracks user search queries for analytics and search suggestions';
COMMENT ON TABLE saved_searches IS 'Allows premium users to save and reuse frequently used searches';
COMMENT ON COLUMN saved_searches.filters IS 'JSON object containing filter parameters (date_range, categories, tags, etc.)';
COMMENT ON COLUMN search_history.filters IS 'JSON object containing filter parameters used in the search';
