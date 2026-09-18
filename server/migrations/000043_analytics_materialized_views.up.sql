-- Analytics Materialized Views for Performance Optimization
-- These views cache expensive aggregations for quick access

-- 1. Daily User Activity Summary
CREATE MATERIALIZED VIEW IF NOT EXISTS mv_daily_user_activity AS
SELECT
    user_id,
    DATE(checkin_time AT TIME ZONE 'UTC') as activity_date,
    COUNT(DISTINCT id) as total_checkins,
    COUNT(DISTINCT category_id) as unique_categories,
    COUNT(DISTINCT DATE_TRUNC('hour', checkin_time)) as active_hours,
    COALESCE(SUM(duration_minutes), 0) as total_minutes,
    MIN(checkin_time) as first_checkin,
    MAX(checkin_time) as last_checkin,
    EXTRACT(DOW FROM checkin_time AT TIME ZONE 'UTC') as day_of_week,
    EXTRACT(HOUR FROM MIN(checkin_time) AT TIME ZONE 'UTC') as first_activity_hour,
    EXTRACT(HOUR FROM MAX(checkin_time) AT TIME ZONE 'UTC') as last_activity_hour
FROM checkins
WHERE deleted_at IS NULL
GROUP BY user_id, DATE(checkin_time AT TIME ZONE 'UTC'), EXTRACT(DOW FROM checkin_time AT TIME ZONE 'UTC');

-- Create indexes for fast lookups
CREATE INDEX idx_mv_daily_user_activity_user_date ON mv_daily_user_activity(user_id, activity_date DESC);
CREATE INDEX idx_mv_daily_user_activity_date ON mv_daily_user_activity(activity_date DESC);

-- 2. Category Usage Trends
CREATE MATERIALIZED VIEW IF NOT EXISTS mv_category_usage_trends AS
SELECT
    user_id,
    category_id,
    DATE_TRUNC('week', checkin_time AT TIME ZONE 'UTC') as week_start,
    COUNT(*) as usage_count,
    COUNT(DISTINCT DATE(checkin_time AT TIME ZONE 'UTC')) as active_days,
    COALESCE(SUM(duration_minutes), 0) as total_minutes,
    COALESCE(AVG(duration_minutes), 0) as avg_duration,
    MIN(checkin_time) as first_use_in_period,
    MAX(checkin_time) as last_use_in_period,
    ARRAY_AGG(DISTINCT EXTRACT(DOW FROM checkin_time AT TIME ZONE 'UTC')::int ORDER BY EXTRACT(DOW FROM checkin_time AT TIME ZONE 'UTC')::int) as active_days_of_week
FROM checkins
WHERE deleted_at IS NULL
    AND category_id IS NOT NULL
GROUP BY user_id, category_id, DATE_TRUNC('week', checkin_time AT TIME ZONE 'UTC');

CREATE INDEX idx_mv_category_trends_user_cat ON mv_category_usage_trends(user_id, category_id, week_start DESC);
CREATE INDEX idx_mv_category_trends_week ON mv_category_usage_trends(week_start DESC);

-- 3. Hourly Activity Patterns
CREATE MATERIALIZED VIEW IF NOT EXISTS mv_hourly_activity_patterns AS
SELECT
    user_id,
    EXTRACT(HOUR FROM checkin_time AT TIME ZONE 'UTC') as hour_of_day,
    EXTRACT(DOW FROM checkin_time AT TIME ZONE 'UTC') as day_of_week,
    COUNT(*) as checkin_count,
    COUNT(DISTINCT DATE(checkin_time AT TIME ZONE 'UTC')) as occurrence_days,
    COALESCE(SUM(duration_minutes), 0) as total_minutes,
    COALESCE(AVG(duration_minutes), 0) as avg_duration_minutes,
    COUNT(DISTINCT category_id) as unique_categories
FROM checkins
WHERE deleted_at IS NULL
    AND checkin_time >= CURRENT_DATE - INTERVAL '90 days'
GROUP BY user_id,
         EXTRACT(HOUR FROM checkin_time AT TIME ZONE 'UTC'),
         EXTRACT(DOW FROM checkin_time AT TIME ZONE 'UTC');

CREATE INDEX idx_mv_hourly_patterns_user ON mv_hourly_activity_patterns(user_id, hour_of_day, day_of_week);

-- 4. Streak Analytics
CREATE MATERIALIZED VIEW IF NOT EXISTS mv_user_streaks AS
WITH daily_checkins AS (
    SELECT
        user_id,
        DATE(checkin_time AT TIME ZONE 'UTC') as checkin_date
    FROM checkins
    WHERE deleted_at IS NULL
    GROUP BY user_id, DATE(checkin_time AT TIME ZONE 'UTC')
),
streak_groups AS (
    SELECT
        user_id,
        checkin_date,
        checkin_date - (ROW_NUMBER() OVER (PARTITION BY user_id ORDER BY checkin_date))::int AS streak_group
    FROM daily_checkins
),
streaks AS (
    SELECT
        user_id,
        MIN(checkin_date) as streak_start,
        MAX(checkin_date) as streak_end,
        COUNT(*) as streak_length
    FROM streak_groups
    GROUP BY user_id, streak_group
)
SELECT
    user_id,
    MAX(streak_length) as longest_streak,
    CASE
        WHEN MAX(CASE WHEN streak_end = CURRENT_DATE OR streak_end = CURRENT_DATE - 1 THEN streak_length ELSE 0 END) > 0
        THEN MAX(CASE WHEN streak_end = CURRENT_DATE OR streak_end = CURRENT_DATE - 1 THEN streak_length ELSE 0 END)
        ELSE 0
    END as current_streak,
    MAX(streak_end) as last_checkin_date,
    COUNT(*) as total_streak_periods,
    COALESCE(AVG(streak_length), 0) as avg_streak_length,
    (SELECT COUNT(DISTINCT DATE(checkin_time AT TIME ZONE 'UTC'))
     FROM checkins c
     WHERE c.user_id = streaks.user_id
       AND c.deleted_at IS NULL) as total_active_days
FROM streaks
GROUP BY user_id;

CREATE INDEX idx_mv_user_streaks_user ON mv_user_streaks(user_id);

-- 5. Category Performance Metrics
CREATE MATERIALIZED VIEW IF NOT EXISTS mv_category_performance AS
SELECT
    c.user_id,
    c.category_id,
    cat.name as category_name,
    cat.color,
    COUNT(*) as total_usage,
    COUNT(DISTINCT DATE(c.checkin_time AT TIME ZONE 'UTC')) as days_used,
    COALESCE(SUM(c.duration_minutes), 0) as total_minutes,
    COALESCE(AVG(c.duration_minutes), 0) as avg_duration_minutes,
    MIN(c.checkin_time) as first_used,
    MAX(c.checkin_time) as last_used,
    CURRENT_TIMESTAMP - MAX(c.checkin_time) as time_since_last_use,
    -- Trend calculation (last 7 days vs previous 7 days)
    (SELECT COUNT(*)
     FROM checkins c2
     WHERE c2.user_id = c.user_id
       AND c2.category_id = c.category_id
       AND c2.deleted_at IS NULL
       AND c2.checkin_time >= CURRENT_DATE - INTERVAL '7 days') as usage_last_7_days,
    (SELECT COUNT(*)
     FROM checkins c3
     WHERE c3.user_id = c.user_id
       AND c3.category_id = c.category_id
       AND c3.deleted_at IS NULL
       AND c3.checkin_time >= CURRENT_DATE - INTERVAL '14 days'
       AND c3.checkin_time < CURRENT_DATE - INTERVAL '7 days') as usage_prev_7_days
FROM checkins c
LEFT JOIN categories cat ON c.category_id = cat.id
WHERE c.deleted_at IS NULL
    AND c.category_id IS NOT NULL
GROUP BY c.user_id, c.category_id, cat.name, cat.color;

CREATE INDEX idx_mv_category_perf_user ON mv_category_performance(user_id, total_usage DESC);
CREATE INDEX idx_mv_category_perf_cat ON mv_category_performance(category_id, total_usage DESC);

-- 6. Monthly Activity Summary
CREATE MATERIALIZED VIEW IF NOT EXISTS mv_monthly_summary AS
SELECT
    user_id,
    DATE_TRUNC('month', checkin_time AT TIME ZONE 'UTC') as month,
    COUNT(*) as total_checkins,
    COUNT(DISTINCT category_id) as unique_categories,
    COUNT(DISTINCT DATE(checkin_time AT TIME ZONE 'UTC')) as active_days,
    COALESCE(SUM(duration_minutes), 0) as total_minutes,
    COALESCE(AVG(duration_minutes), 0) as avg_duration_per_checkin,
    COUNT(*) FILTER (WHERE EXTRACT(DOW FROM checkin_time AT TIME ZONE 'UTC') IN (0, 6)) as weekend_checkins,
    COUNT(*) FILTER (WHERE EXTRACT(DOW FROM checkin_time AT TIME ZONE 'UTC') IN (1, 2, 3, 4, 5)) as weekday_checkins,
    COUNT(*) FILTER (WHERE EXTRACT(HOUR FROM checkin_time AT TIME ZONE 'UTC') BETWEEN 6 AND 12) as morning_checkins,
    COUNT(*) FILTER (WHERE EXTRACT(HOUR FROM checkin_time AT TIME ZONE 'UTC') BETWEEN 12 AND 18) as afternoon_checkins,
    COUNT(*) FILTER (WHERE EXTRACT(HOUR FROM checkin_time AT TIME ZONE 'UTC') BETWEEN 18 AND 23) as evening_checkins,
    MODE() WITHIN GROUP (ORDER BY category_id) as most_used_category
FROM checkins
WHERE deleted_at IS NULL
GROUP BY user_id, DATE_TRUNC('month', checkin_time AT TIME ZONE 'UTC');

CREATE INDEX idx_mv_monthly_summary_user_month ON mv_monthly_summary(user_id, month DESC);

-- 7. Analytics Events Summary
CREATE MATERIALIZED VIEW IF NOT EXISTS mv_analytics_events_summary AS
SELECT
    DATE(timestamp AT TIME ZONE 'UTC') as event_date,
    event_type,
    user_id,
    COUNT(*) as event_count,
    COUNT(DISTINCT session_id) as unique_sessions,
    COUNT(DISTINCT ip) as unique_ips,
    MIN(timestamp) as first_event,
    MAX(timestamp) as last_event
FROM analytics_events
GROUP BY DATE(timestamp AT TIME ZONE 'UTC'), event_type, user_id;

CREATE INDEX idx_mv_analytics_summary_date ON mv_analytics_events_summary(event_date DESC, event_type);
CREATE INDEX idx_mv_analytics_summary_user ON mv_analytics_events_summary(user_id, event_date DESC);

-- Function to refresh all materialized views
CREATE OR REPLACE FUNCTION refresh_analytics_materialized_views()
RETURNS void
LANGUAGE plpgsql
AS $$
BEGIN
    REFRESH MATERIALIZED VIEW CONCURRENTLY mv_daily_user_activity;
    REFRESH MATERIALIZED VIEW CONCURRENTLY mv_category_usage_trends;
    REFRESH MATERIALIZED VIEW CONCURRENTLY mv_hourly_activity_patterns;
    REFRESH MATERIALIZED VIEW CONCURRENTLY mv_user_streaks;
    REFRESH MATERIALIZED VIEW CONCURRENTLY mv_category_performance;
    REFRESH MATERIALIZED VIEW CONCURRENTLY mv_monthly_summary;
    REFRESH MATERIALIZED VIEW CONCURRENTLY mv_analytics_events_summary;

    RAISE NOTICE 'All analytics materialized views refreshed successfully';
END;
$$;

-- Function to refresh specific views based on time sensitivity
CREATE OR REPLACE FUNCTION refresh_realtime_analytics_views()
RETURNS void
LANGUAGE plpgsql
AS $$
BEGIN
    -- Refresh only the most time-sensitive views
    REFRESH MATERIALIZED VIEW CONCURRENTLY mv_daily_user_activity;
    REFRESH MATERIALIZED VIEW CONCURRENTLY mv_user_streaks;
    REFRESH MATERIALIZED VIEW CONCURRENTLY mv_analytics_events_summary;

    RAISE NOTICE 'Real-time analytics views refreshed';
END;
$$;

-- Create a table to track refresh times
CREATE TABLE IF NOT EXISTS mv_refresh_log (
    id SERIAL PRIMARY KEY,
    view_name TEXT NOT NULL,
    refresh_started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    refresh_completed_at TIMESTAMP,
    duration_seconds NUMERIC,
    rows_affected BIGINT,
    status TEXT DEFAULT 'running', -- 'running', 'completed', 'failed'
    error_message TEXT
);

CREATE INDEX idx_mv_refresh_log_view ON mv_refresh_log(view_name, refresh_started_at DESC);

-- Function to log materialized view refreshes
CREATE OR REPLACE FUNCTION log_mv_refresh(view_name TEXT)
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
    start_time TIMESTAMP;
    log_id INTEGER;
BEGIN
    start_time := CURRENT_TIMESTAMP;

    -- Insert log entry
    INSERT INTO mv_refresh_log (view_name, status)
    VALUES (view_name, 'running')
    RETURNING id INTO log_id;

    -- Execute refresh
    EXECUTE 'REFRESH MATERIALIZED VIEW CONCURRENTLY ' || view_name;

    -- Update log with completion
    UPDATE mv_refresh_log
    SET refresh_completed_at = CURRENT_TIMESTAMP,
        duration_seconds = EXTRACT(EPOCH FROM (CURRENT_TIMESTAMP - start_time)),
        status = 'completed'
    WHERE id = log_id;

EXCEPTION WHEN OTHERS THEN
    -- Log error
    UPDATE mv_refresh_log
    SET status = 'failed',
        error_message = SQLERRM,
        refresh_completed_at = CURRENT_TIMESTAMP
    WHERE id = log_id;

    RAISE;
END;
$$;

-- Grant appropriate permissions
GRANT SELECT ON ALL TABLES IN SCHEMA public TO PUBLIC;
GRANT EXECUTE ON FUNCTION refresh_analytics_materialized_views() TO PUBLIC;
GRANT EXECUTE ON FUNCTION refresh_realtime_analytics_views() TO PUBLIC;
