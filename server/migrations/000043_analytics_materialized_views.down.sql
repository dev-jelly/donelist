-- Drop materialized views and related functions

-- Drop functions
DROP FUNCTION IF EXISTS log_mv_refresh(TEXT);
DROP FUNCTION IF EXISTS refresh_realtime_analytics_views();
DROP FUNCTION IF EXISTS refresh_analytics_materialized_views();

-- Drop refresh log table
DROP TABLE IF EXISTS mv_refresh_log;

-- Drop materialized views
DROP MATERIALIZED VIEW IF EXISTS mv_analytics_events_summary;
DROP MATERIALIZED VIEW IF EXISTS mv_monthly_summary;
DROP MATERIALIZED VIEW IF EXISTS mv_category_performance;
DROP MATERIALIZED VIEW IF EXISTS mv_user_streaks;
DROP MATERIALIZED VIEW IF EXISTS mv_hourly_activity_patterns;
DROP MATERIALIZED VIEW IF EXISTS mv_category_usage_trends;
DROP MATERIALIZED VIEW IF EXISTS mv_daily_user_activity;
