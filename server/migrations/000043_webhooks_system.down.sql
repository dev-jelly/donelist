-- Drop triggers
DROP TRIGGER IF EXISTS trigger_log_webhook_event ON webhook_deliveries;
DROP TRIGGER IF EXISTS trigger_webhooks_updated_at ON webhooks;

-- Drop functions
DROP FUNCTION IF EXISTS log_webhook_event();
DROP FUNCTION IF EXISTS update_webhooks_updated_at();

-- Drop tables (in reverse order of dependencies)
DROP TABLE IF EXISTS webhook_event_logs;
DROP TABLE IF EXISTS webhook_dead_letter_queue;
DROP TABLE IF EXISTS webhook_deliveries;
DROP TABLE IF EXISTS webhooks;
