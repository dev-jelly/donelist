-- Drop functions
DROP FUNCTION IF EXISTS cleanup_old_rate_limits();
DROP FUNCTION IF EXISTS cleanup_old_api_key_usage();

-- Drop tables in reverse order
DROP TABLE IF EXISTS api_key_rate_limits;
DROP TABLE IF EXISTS api_key_usage;
DROP TABLE IF EXISTS api_keys;
