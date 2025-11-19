-- API Keys table
CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    key_prefix VARCHAR(10) NOT NULL,
    key_hash VARCHAR(255) UNIQUE NOT NULL,
    scopes TEXT[] NOT NULL DEFAULT '{}',
    rate_limit_per_day INTEGER DEFAULT 10000,
    rate_limit_per_hour INTEGER DEFAULT 1000,
    last_used_at TIMESTAMP WITH TIME ZONE,
    expires_at TIMESTAMP WITH TIME ZONE,
    revoked BOOLEAN DEFAULT FALSE,
    revoked_at TIMESTAMP WITH TIME ZONE,
    revoked_reason TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_api_keys_user_id ON api_keys(user_id);
CREATE INDEX idx_api_keys_key_hash ON api_keys(key_hash);
CREATE INDEX idx_api_keys_key_prefix ON api_keys(key_prefix);
CREATE INDEX idx_api_keys_revoked ON api_keys(revoked);
CREATE INDEX idx_api_keys_expires_at ON api_keys(expires_at);

-- API Key usage tracking table
CREATE TABLE api_key_usage (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    api_key_id UUID NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    endpoint VARCHAR(255) NOT NULL,
    method VARCHAR(10) NOT NULL,
    status_code INTEGER NOT NULL,
    response_time_ms INTEGER,
    ip_address VARCHAR(45),
    user_agent TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_api_key_usage_api_key_id ON api_key_usage(api_key_id);
CREATE INDEX idx_api_key_usage_created_at ON api_key_usage(created_at);
CREATE INDEX idx_api_key_usage_api_key_date ON api_key_usage(api_key_id, created_at);

-- API Key rate limit tracking table (using Redis-like counters in PostgreSQL)
CREATE TABLE api_key_rate_limits (
    api_key_id UUID NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    time_window VARCHAR(20) NOT NULL, -- 'hour:2024-01-01:12' or 'day:2024-01-01'
    request_count INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (api_key_id, time_window)
);

CREATE INDEX idx_api_key_rate_limits_time_window ON api_key_rate_limits(time_window);

-- Trigger for updated_at on api_keys
CREATE TRIGGER update_api_keys_updated_at BEFORE UPDATE ON api_keys
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Trigger for updated_at on api_key_rate_limits
CREATE TRIGGER update_api_key_rate_limits_updated_at BEFORE UPDATE ON api_key_rate_limits
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Function to clean up old usage records (older than 90 days)
CREATE OR REPLACE FUNCTION cleanup_old_api_key_usage()
RETURNS void AS $$
BEGIN
    DELETE FROM api_key_usage
    WHERE created_at < CURRENT_TIMESTAMP - INTERVAL '90 days';
END;
$$ LANGUAGE plpgsql;

-- Function to clean up old rate limit records (older than 2 days)
CREATE OR REPLACE FUNCTION cleanup_old_rate_limits()
RETURNS void AS $$
BEGIN
    DELETE FROM api_key_rate_limits
    WHERE updated_at < CURRENT_TIMESTAMP - INTERVAL '2 days';
END;
$$ LANGUAGE plpgsql;
