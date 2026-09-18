-- Subscription notifications tracking table
CREATE TABLE IF NOT EXISTS subscription_notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subscription_id UUID NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
    notification_type VARCHAR(50) NOT NULL CHECK (notification_type IN ('pre_expiry', 'post_expiry')),
    days_before_expiry INT DEFAULT 0,
    days_after_expiry INT DEFAULT 0,
    sent_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    -- Ensure we don't send duplicate notifications
    UNIQUE(subscription_id, notification_type, days_before_expiry, days_after_expiry)
);

-- Dunning attempts table for payment retry tracking
CREATE TABLE IF NOT EXISTS dunning_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subscription_id UUID NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
    payment_id UUID REFERENCES payments(id) ON DELETE SET NULL,
    attempt_number INT NOT NULL CHECK (attempt_number > 0),
    status VARCHAR(50) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'retrying', 'succeeded', 'failed', 'canceled')),
    scheduled_for TIMESTAMP NOT NULL,
    attempted_at TIMESTAMP,
    succeeded_at TIMESTAMP,
    failed_at TIMESTAMP,
    failure_reason TEXT,
    next_retry_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    -- Ensure unique attempt numbers per subscription
    UNIQUE(subscription_id, attempt_number)
);

-- Indexes for subscription_notifications
CREATE INDEX IF NOT EXISTS idx_subscription_notifications_subscription_id
    ON subscription_notifications(subscription_id);
CREATE INDEX IF NOT EXISTS idx_subscription_notifications_sent_at
    ON subscription_notifications(sent_at);
CREATE INDEX IF NOT EXISTS idx_subscription_notifications_type
    ON subscription_notifications(notification_type);

-- Indexes for dunning_attempts
CREATE INDEX IF NOT EXISTS idx_dunning_attempts_subscription_id
    ON dunning_attempts(subscription_id);
CREATE INDEX IF NOT EXISTS idx_dunning_attempts_status
    ON dunning_attempts(status);
CREATE INDEX IF NOT EXISTS idx_dunning_attempts_scheduled_for
    ON dunning_attempts(scheduled_for);
CREATE INDEX IF NOT EXISTS idx_dunning_attempts_status_scheduled
    ON dunning_attempts(status, scheduled_for) WHERE status = 'pending';

-- Trigger to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_dunning_attempts_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER dunning_attempts_updated_at
    BEFORE UPDATE ON dunning_attempts
    FOR EACH ROW
    EXECUTE FUNCTION update_dunning_attempts_updated_at();

-- Comments for documentation
COMMENT ON TABLE subscription_notifications IS 'Tracks sent subscription expiry notifications to prevent duplicates';
COMMENT ON TABLE dunning_attempts IS 'Tracks payment retry attempts for failed subscription payments';
COMMENT ON COLUMN dunning_attempts.attempt_number IS 'Sequential retry attempt number (1, 2, 3...)';
COMMENT ON COLUMN dunning_attempts.scheduled_for IS 'When this retry attempt should be executed';
COMMENT ON COLUMN dunning_attempts.next_retry_at IS 'When the next retry should occur if this one fails';
