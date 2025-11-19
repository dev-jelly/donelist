-- Offline sync system tables for mobile offline-first architecture

-- Add version field to checkins table if not exists (for optimistic locking)
-- This may already exist from previous migrations
ALTER TABLE checkins ADD COLUMN IF NOT EXISTS version INTEGER DEFAULT 1;

-- Sync queue table - stores pending operations from offline clients
CREATE TABLE sync_queue (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Operation details
    operation_type VARCHAR(20) NOT NULL CHECK (operation_type IN ('create', 'update', 'delete')),
    resource_type VARCHAR(50) NOT NULL CHECK (resource_type IN ('checkin', 'category', 'tag')),
    resource_id UUID NOT NULL, -- Client-generated UUID for creates, existing ID for updates/deletes

    -- Idempotency and deduplication
    idempotency_key VARCHAR(255) NOT NULL, -- Client-generated unique key for operation
    client_timestamp TIMESTAMP WITH TIME ZONE NOT NULL, -- When operation was performed on client

    -- Sync metadata
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'conflicted')),
    retry_count INTEGER DEFAULT 0,
    last_retry_at TIMESTAMP WITH TIME ZONE,
    error_message TEXT,

    -- Operation payload (JSON)
    operation_data JSONB NOT NULL, -- The actual operation data (content, category_id, etc.)
    conflict_data JSONB, -- Stores conflict information if status='conflicted'

    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP WITH TIME ZONE,

    -- Ensure idempotency per user
    UNIQUE(user_id, idempotency_key)
);

CREATE INDEX idx_sync_queue_user_id ON sync_queue(user_id);
CREATE INDEX idx_sync_queue_status ON sync_queue(status);
CREATE INDEX idx_sync_queue_idempotency ON sync_queue(idempotency_key);
CREATE INDEX idx_sync_queue_resource ON sync_queue(resource_type, resource_id);
CREATE INDEX idx_sync_queue_created_at ON sync_queue(created_at);
CREATE INDEX idx_sync_queue_client_timestamp ON sync_queue(client_timestamp);

-- Sync status tracking - tracks overall sync progress for clients
CREATE TABLE sync_status (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Device/client identification
    device_id VARCHAR(255) NOT NULL, -- Client-generated device identifier

    -- Sync progress
    last_sync_at TIMESTAMP WITH TIME ZONE,
    last_successful_sync_at TIMESTAMP WITH TIME ZONE,
    pending_operations_count INTEGER DEFAULT 0,
    failed_operations_count INTEGER DEFAULT 0,

    -- Client state
    last_known_checkin_id UUID, -- Last checkin ID the client knows about
    last_known_timestamp TIMESTAMP WITH TIME ZONE, -- Last sync timestamp for delta sync

    -- Metadata
    client_version VARCHAR(50), -- App version for compatibility checks
    platform VARCHAR(20) CHECK (platform IN ('ios', 'android', 'web')),

    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    UNIQUE(user_id, device_id)
);

CREATE INDEX idx_sync_status_user_id ON sync_status(user_id);
CREATE INDEX idx_sync_status_device_id ON sync_status(device_id);
CREATE INDEX idx_sync_status_last_sync ON sync_status(last_sync_at);

-- Operation log - audit trail for all sync operations
CREATE TABLE sync_operation_log (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    sync_queue_id UUID REFERENCES sync_queue(id) ON DELETE SET NULL,

    -- Operation details
    operation_type VARCHAR(20) NOT NULL,
    resource_type VARCHAR(50) NOT NULL,
    resource_id UUID NOT NULL,

    -- Result
    status VARCHAR(20) NOT NULL CHECK (status IN ('success', 'failed', 'conflicted', 'skipped')),
    error_message TEXT,
    resolution_strategy VARCHAR(50), -- e.g., 'last_write_wins', 'client_wins', 'server_wins'

    -- Timing
    client_timestamp TIMESTAMP WITH TIME ZONE,
    server_timestamp TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    processing_duration_ms INTEGER,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_sync_operation_log_user_id ON sync_operation_log(user_id);
CREATE INDEX idx_sync_operation_log_sync_queue_id ON sync_operation_log(sync_queue_id);
CREATE INDEX idx_sync_operation_log_resource ON sync_operation_log(resource_type, resource_id);
CREATE INDEX idx_sync_operation_log_created_at ON sync_operation_log(created_at);

-- Idempotency tokens table - for HTTP API operations
CREATE TABLE idempotency_tokens (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Token and operation
    token VARCHAR(255) NOT NULL,
    operation_type VARCHAR(20) NOT NULL,
    resource_type VARCHAR(50) NOT NULL,

    -- Response caching
    response_status INTEGER, -- HTTP status code
    response_body JSONB, -- Cached response

    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT (CURRENT_TIMESTAMP + INTERVAL '24 hours'),

    UNIQUE(user_id, token)
);

CREATE INDEX idx_idempotency_tokens_token ON idempotency_tokens(token);
CREATE INDEX idx_idempotency_tokens_user_id ON idempotency_tokens(user_id);
CREATE INDEX idx_idempotency_tokens_expires_at ON idempotency_tokens(expires_at);

-- Trigger for sync_queue updated_at
CREATE TRIGGER update_sync_queue_updated_at BEFORE UPDATE ON sync_queue
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_sync_status_updated_at BEFORE UPDATE ON sync_status
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Function to clean up old completed sync queue entries
CREATE OR REPLACE FUNCTION cleanup_old_sync_operations()
RETURNS void AS $$
BEGIN
    -- Delete completed operations older than 30 days
    DELETE FROM sync_queue
    WHERE status = 'completed'
    AND completed_at < CURRENT_TIMESTAMP - INTERVAL '30 days';

    -- Delete old operation logs older than 90 days
    DELETE FROM sync_operation_log
    WHERE created_at < CURRENT_TIMESTAMP - INTERVAL '90 days';

    -- Delete expired idempotency tokens
    DELETE FROM idempotency_tokens
    WHERE expires_at < CURRENT_TIMESTAMP;
END;
$$ LANGUAGE plpgsql;

-- Function to update sync status
CREATE OR REPLACE FUNCTION update_sync_status_counts()
RETURNS TRIGGER AS $$
BEGIN
    -- Update pending and failed counts for the user
    UPDATE sync_status
    SET
        pending_operations_count = (
            SELECT COUNT(*) FROM sync_queue
            WHERE user_id = NEW.user_id AND status = 'pending'
        ),
        failed_operations_count = (
            SELECT COUNT(*) FROM sync_queue
            WHERE user_id = NEW.user_id AND status = 'failed'
        ),
        updated_at = CURRENT_TIMESTAMP
    WHERE user_id = NEW.user_id;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger to auto-update sync status when queue changes
CREATE TRIGGER update_sync_status_on_queue_change
AFTER INSERT OR UPDATE ON sync_queue
FOR EACH ROW EXECUTE FUNCTION update_sync_status_counts();
