-- Webhooks table for storing webhook configurations
CREATE TABLE IF NOT EXISTS webhooks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    url TEXT NOT NULL,
    secret VARCHAR(255) NOT NULL,
    events JSONB NOT NULL DEFAULT '[]'::jsonb,
    active BOOLEAN NOT NULL DEFAULT true,
    headers JSONB DEFAULT '{}'::jsonb,

    -- Statistics
    last_triggered_at TIMESTAMPTZ,
    total_deliveries INTEGER NOT NULL DEFAULT 0,
    failed_deliveries INTEGER NOT NULL DEFAULT 0,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,

    -- Constraints
    CONSTRAINT valid_url CHECK (url ~* '^https?://'),
    CONSTRAINT valid_events CHECK (jsonb_typeof(events) = 'array')
);

-- Indexes for webhooks table
CREATE INDEX idx_webhooks_user_id ON webhooks(user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_webhooks_active ON webhooks(active) WHERE deleted_at IS NULL;
CREATE INDEX idx_webhooks_events ON webhooks USING GIN(events) WHERE deleted_at IS NULL;
CREATE INDEX idx_webhooks_deleted_at ON webhooks(deleted_at);

-- Webhook deliveries table for tracking delivery attempts
CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webhook_id UUID NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    event_type VARCHAR(100) NOT NULL,
    event_id VARCHAR(255) NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    status_code INTEGER,
    response TEXT,
    error TEXT,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered_at TIMESTAMPTZ,
    next_retry_at TIMESTAMPTZ,

    -- Constraints
    CONSTRAINT valid_status CHECK (status IN ('pending', 'delivered', 'failed', 'retrying', 'skipped')),
    CONSTRAINT valid_attempts CHECK (attempts >= 0 AND attempts <= 10)
);

-- Indexes for webhook_deliveries table
CREATE INDEX idx_webhook_deliveries_webhook_id ON webhook_deliveries(webhook_id);
CREATE INDEX idx_webhook_deliveries_status ON webhook_deliveries(status);
CREATE INDEX idx_webhook_deliveries_event_type ON webhook_deliveries(event_type);
CREATE INDEX idx_webhook_deliveries_created_at ON webhook_deliveries(created_at DESC);
CREATE INDEX idx_webhook_deliveries_retry ON webhook_deliveries(status, next_retry_at)
    WHERE status = 'retrying';

-- Webhook dead letter queue for permanently failed deliveries
CREATE TABLE IF NOT EXISTS webhook_dead_letter_queue (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webhook_id UUID NOT NULL,
    webhook_url TEXT NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    event_id VARCHAR(255) NOT NULL,
    payload JSONB NOT NULL,
    total_attempts INTEGER NOT NULL,
    last_error TEXT,
    last_status_code INTEGER,
    original_delivery_id UUID NOT NULL,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    failed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Metadata for debugging
    metadata JSONB DEFAULT '{}'::jsonb
);

-- Indexes for dead letter queue
CREATE INDEX idx_webhook_dlq_webhook_id ON webhook_dead_letter_queue(webhook_id);
CREATE INDEX idx_webhook_dlq_failed_at ON webhook_dead_letter_queue(failed_at DESC);
CREATE INDEX idx_webhook_dlq_event_type ON webhook_dead_letter_queue(event_type);

-- Webhook event logs for monitoring and debugging
CREATE TABLE IF NOT EXISTS webhook_event_logs (
    id BIGSERIAL PRIMARY KEY,
    webhook_id UUID NOT NULL,
    delivery_id UUID NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    status VARCHAR(50) NOT NULL,
    attempt_number INTEGER NOT NULL,
    duration_ms INTEGER,
    status_code INTEGER,
    error TEXT,

    -- Timestamps
    logged_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Partition webhook_event_logs by month for better performance
CREATE INDEX idx_webhook_event_logs_webhook_id ON webhook_event_logs(webhook_id, logged_at DESC);
CREATE INDEX idx_webhook_event_logs_delivery_id ON webhook_event_logs(delivery_id);
CREATE INDEX idx_webhook_event_logs_logged_at ON webhook_event_logs(logged_at DESC);

-- Create a function to automatically update updated_at timestamp
CREATE OR REPLACE FUNCTION update_webhooks_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger for webhooks table
CREATE TRIGGER trigger_webhooks_updated_at
    BEFORE UPDATE ON webhooks
    FOR EACH ROW
    EXECUTE FUNCTION update_webhooks_updated_at();

-- Create a function to log webhook events
CREATE OR REPLACE FUNCTION log_webhook_event()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'UPDATE' THEN
        -- Only log when status changes or attempts increase
        IF NEW.status != OLD.status OR NEW.attempts != OLD.attempts THEN
            INSERT INTO webhook_event_logs (
                webhook_id,
                delivery_id,
                event_type,
                status,
                attempt_number,
                status_code,
                error,
                logged_at
            ) VALUES (
                NEW.webhook_id,
                NEW.id,
                NEW.event_type,
                NEW.status,
                NEW.attempts,
                NEW.status_code,
                NEW.error,
                NOW()
            );
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger for webhook_deliveries table
CREATE TRIGGER trigger_log_webhook_event
    AFTER UPDATE ON webhook_deliveries
    FOR EACH ROW
    EXECUTE FUNCTION log_webhook_event();

-- Add comments for documentation
COMMENT ON TABLE webhooks IS 'Stores webhook configurations for third-party integrations';
COMMENT ON TABLE webhook_deliveries IS 'Tracks webhook delivery attempts and their status';
COMMENT ON TABLE webhook_dead_letter_queue IS 'Stores permanently failed webhook deliveries for manual review';
COMMENT ON TABLE webhook_event_logs IS 'Audit log of all webhook events for monitoring and debugging';

COMMENT ON COLUMN webhooks.secret IS 'Secret key used for HMAC signature generation';
COMMENT ON COLUMN webhooks.events IS 'JSON array of event types this webhook subscribes to';
COMMENT ON COLUMN webhooks.headers IS 'Custom HTTP headers to include in webhook requests';
