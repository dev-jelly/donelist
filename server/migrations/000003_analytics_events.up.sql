-- Create analytics_events table for user activity tracking
CREATE TABLE IF NOT EXISTS analytics_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    event_type VARCHAR(100) NOT NULL,
    event_name VARCHAR(255) NOT NULL,
    event_data JSONB,
    category_id UUID REFERENCES categories(id) ON DELETE SET NULL,
    checkin_id UUID REFERENCES checkins(id) ON DELETE CASCADE,
    session_id VARCHAR(255),
    ip_address INET,
    user_agent TEXT,
    referrer TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for analytics queries
CREATE INDEX idx_analytics_events_user_id ON analytics_events(user_id);
CREATE INDEX idx_analytics_events_event_type ON analytics_events(event_type);
CREATE INDEX idx_analytics_events_created_at ON analytics_events(created_at DESC);
CREATE INDEX idx_analytics_events_session_id ON analytics_events(session_id);
CREATE INDEX idx_analytics_events_checkin_id ON analytics_events(checkin_id) WHERE checkin_id IS NOT NULL;

-- Create partial index for common queries
CREATE INDEX idx_analytics_events_user_date ON analytics_events(user_id, created_at DESC)
    WHERE user_id IS NOT NULL;

-- Add comments
COMMENT ON TABLE analytics_events IS 'Stores user activity events for analytics and reporting';
COMMENT ON COLUMN analytics_events.event_type IS 'Type of event (e.g., checkin_created, user_login, export_requested)';
COMMENT ON COLUMN analytics_events.event_data IS 'Additional event-specific data in JSON format';