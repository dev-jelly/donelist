-- Drop trigger and function
DROP TRIGGER IF EXISTS dunning_attempts_updated_at ON dunning_attempts;
DROP FUNCTION IF EXISTS update_dunning_attempts_updated_at();

-- Drop indexes
DROP INDEX IF EXISTS idx_dunning_attempts_status_scheduled;
DROP INDEX IF EXISTS idx_dunning_attempts_scheduled_for;
DROP INDEX IF EXISTS idx_dunning_attempts_status;
DROP INDEX IF EXISTS idx_dunning_attempts_subscription_id;

DROP INDEX IF EXISTS idx_subscription_notifications_type;
DROP INDEX IF EXISTS idx_subscription_notifications_sent_at;
DROP INDEX IF EXISTS idx_subscription_notifications_subscription_id;

-- Drop tables
DROP TABLE IF EXISTS dunning_attempts;
DROP TABLE IF EXISTS subscription_notifications;
