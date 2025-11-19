package statistics

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

// AggregationJob handles periodic aggregation of usage statistics
type AggregationJob struct {
	db       *sql.DB
	service  *UsageService
	logger   *zap.Logger
	cron     *cron.Cron
	cache    *UsageCache
	mu       sync.RWMutex
	running  bool
}

// UsageCache stores pre-computed usage statistics
type UsageCache struct {
	mu         sync.RWMutex
	dashboards map[string]*CachedDashboard
	trends     map[string]*CachedTrend
}

// CachedDashboard represents a cached dashboard
type CachedDashboard struct {
	UserID      uuid.UUID
	Period      string
	Dashboard   *UsageDashboard
	GeneratedAt time.Time
	ExpiresAt   time.Time
}

// CachedTrend represents cached trend data
type CachedTrend struct {
	Key         string
	Trends      []*UsageTrend
	GeneratedAt time.Time
	ExpiresAt   time.Time
}

// NewAggregationJob creates a new aggregation job
func NewAggregationJob(db *sql.DB, service *UsageService, logger *zap.Logger) *AggregationJob {
	return &AggregationJob{
		db:      db,
		service: service,
		logger:  logger,
		cron:    cron.New(cron.WithSeconds()),
		cache: &UsageCache{
			dashboards: make(map[string]*CachedDashboard),
			trends:     make(map[string]*CachedTrend),
		},
	}
}

// Start begins the aggregation job scheduler
func (j *AggregationJob) Start() error {
	j.mu.Lock()
	defer j.mu.Unlock()

	if j.running {
		return fmt.Errorf("aggregation job already running")
	}

	// Schedule jobs
	// Refresh materialized views every hour
	_, err := j.cron.AddFunc("0 0 * * * *", j.refreshMaterializedViews)
	if err != nil {
		return fmt.Errorf("failed to schedule materialized view refresh: %w", err)
	}

	// Aggregate daily statistics at 2 AM
	_, err = j.cron.AddFunc("0 0 2 * * *", j.aggregateDailyStats)
	if err != nil {
		return fmt.Errorf("failed to schedule daily aggregation: %w", err)
	}

	// Clean up old statistics weekly (Sunday at 3 AM)
	_, err = j.cron.AddFunc("0 0 3 * * 0", j.cleanupOldStatistics)
	if err != nil {
		return fmt.Errorf("failed to schedule cleanup: %w", err)
	}

	// Pre-compute popular dashboards every 30 minutes
	_, err = j.cron.AddFunc("0 */30 * * * *", j.precomputeDashboards)
	if err != nil {
		return fmt.Errorf("failed to schedule dashboard pre-computation: %w", err)
	}

	j.cron.Start()
	j.running = true

	j.logger.Info("Usage statistics aggregation job started")
	return nil
}

// Stop stops the aggregation job scheduler
func (j *AggregationJob) Stop() {
	j.mu.Lock()
	defer j.mu.Unlock()

	if !j.running {
		return
	}

	ctx := j.cron.Stop()
	<-ctx.Done()
	j.running = false

	j.logger.Info("Usage statistics aggregation job stopped")
}

// refreshMaterializedViews refreshes the materialized views for faster queries
func (j *AggregationJob) refreshMaterializedViews() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	start := time.Now()

	_, err := j.db.ExecContext(ctx, "SELECT refresh_usage_statistics()")
	if err != nil {
		j.logger.Error("Failed to refresh materialized views", zap.Error(err))
		return
	}

	duration := time.Since(start)
	j.logger.Info("Refreshed materialized views",
		zap.Duration("duration", duration))
}

// aggregateDailyStats aggregates daily statistics into a summary table
func (j *AggregationJob) aggregateDailyStats() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	yesterday := time.Now().AddDate(0, 0, -1)
	startOfDay := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, yesterday.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	// Create daily aggregation table if not exists
	_, err := j.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS usage_daily_aggregates (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL,
			date DATE NOT NULL,
			total_checkins INT DEFAULT 0,
			categories_used INT DEFAULT 0,
			tags_used INT DEFAULT 0,
			peak_hour INT,
			peak_hour_count INT DEFAULT 0,
			total_duration_minutes INT DEFAULT 0,
			metadata JSONB,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(user_id, date)
		)
	`)
	if err != nil {
		j.logger.Error("Failed to create aggregation table", zap.Error(err))
		return
	}

	// Get all active users
	rows, err := j.db.QueryContext(ctx, `
		SELECT DISTINCT user_id
		FROM checkins
		WHERE created_at BETWEEN $1 AND $2
			AND deleted_at IS NULL
	`, startOfDay, endOfDay)
	if err != nil {
		j.logger.Error("Failed to get active users", zap.Error(err))
		return
	}
	defer rows.Close()

	var userIDs []uuid.UUID
	for rows.Next() {
		var userID uuid.UUID
		if err := rows.Scan(&userID); err != nil {
			continue
		}
		userIDs = append(userIDs, userID)
	}

	// Aggregate stats for each user
	for _, userID := range userIDs {
		j.aggregateUserDailyStats(ctx, userID, startOfDay, endOfDay)
	}

	j.logger.Info("Completed daily statistics aggregation",
		zap.Int("users_processed", len(userIDs)),
		zap.Time("date", startOfDay))
}

// aggregateUserDailyStats aggregates daily stats for a specific user
func (j *AggregationJob) aggregateUserDailyStats(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) {
	var stats struct {
		TotalCheckins    int
		CategoriesUsed   int
		TagsUsed         int
		PeakHour         sql.NullInt32
		PeakHourCount    int
		TotalMinutes     int
	}

	// Get basic stats
	err := j.db.QueryRowContext(ctx, `
		SELECT
			COUNT(DISTINCT c.id) as total_checkins,
			COUNT(DISTINCT c.category_id) as categories_used,
			COUNT(DISTINCT ct.tag_id) as tags_used,
			COALESCE(SUM(c.duration_minutes), 0) as total_minutes
		FROM checkins c
		LEFT JOIN checkin_tags ct ON ct.checkin_id = c.id
		WHERE c.user_id = $1
			AND c.created_at BETWEEN $2 AND $3
			AND c.deleted_at IS NULL
	`, userID, startDate, endDate).Scan(
		&stats.TotalCheckins,
		&stats.CategoriesUsed,
		&stats.TagsUsed,
		&stats.TotalMinutes,
	)
	if err != nil {
		j.logger.Error("Failed to get user daily stats",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		return
	}

	// Get peak hour
	err = j.db.QueryRowContext(ctx, `
		SELECT
			EXTRACT(HOUR FROM created_at) as hour,
			COUNT(*) as count
		FROM checkins
		WHERE user_id = $1
			AND created_at BETWEEN $2 AND $3
			AND deleted_at IS NULL
		GROUP BY EXTRACT(HOUR FROM created_at)
		ORDER BY count DESC
		LIMIT 1
	`, userID, startDate, endDate).Scan(&stats.PeakHour, &stats.PeakHourCount)
	if err != nil && err != sql.ErrNoRows {
		j.logger.Warn("Failed to get peak hour",
			zap.String("user_id", userID.String()),
			zap.Error(err))
	}

	// Build metadata
	metadata := map[string]interface{}{
		"day_of_week": startDate.Weekday().String(),
		"week_number": getWeekNumber(startDate),
		"month":       startDate.Month().String(),
		"quarter":     getQuarter(startDate),
	}
	metadataJSON, _ := json.Marshal(metadata)

	// Insert or update aggregate
	_, err = j.db.ExecContext(ctx, `
		INSERT INTO usage_daily_aggregates (
			user_id, date, total_checkins, categories_used, tags_used,
			peak_hour, peak_hour_count, total_duration_minutes, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (user_id, date) DO UPDATE SET
			total_checkins = EXCLUDED.total_checkins,
			categories_used = EXCLUDED.categories_used,
			tags_used = EXCLUDED.tags_used,
			peak_hour = EXCLUDED.peak_hour,
			peak_hour_count = EXCLUDED.peak_hour_count,
			total_duration_minutes = EXCLUDED.total_duration_minutes,
			metadata = EXCLUDED.metadata,
			created_at = CURRENT_TIMESTAMP
	`, userID, startDate.Format("2006-01-02"), stats.TotalCheckins,
		stats.CategoriesUsed, stats.TagsUsed, stats.PeakHour,
		stats.PeakHourCount, stats.TotalMinutes, metadataJSON)

	if err != nil {
		j.logger.Error("Failed to insert daily aggregate",
			zap.String("user_id", userID.String()),
			zap.Error(err))
	}
}

// cleanupOldStatistics removes old aggregated statistics
func (j *AggregationJob) cleanupOldStatistics() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Keep aggregates for 1 year
	cutoffDate := time.Now().AddDate(-1, 0, 0)

	result, err := j.db.ExecContext(ctx, `
		DELETE FROM usage_daily_aggregates
		WHERE date < $1
	`, cutoffDate)

	if err != nil {
		j.logger.Error("Failed to cleanup old statistics", zap.Error(err))
		return
	}

	rowsAffected, _ := result.RowsAffected()
	j.logger.Info("Cleaned up old statistics",
		zap.Int64("rows_deleted", rowsAffected),
		zap.Time("cutoff_date", cutoffDate))

	// Clean cache
	j.cleanCache()
}

// precomputeDashboards pre-computes dashboards for active users
func (j *AggregationJob) precomputeDashboards() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Get recently active users
	rows, err := j.db.QueryContext(ctx, `
		SELECT DISTINCT user_id
		FROM checkins
		WHERE created_at > NOW() - INTERVAL '24 hours'
			AND deleted_at IS NULL
		LIMIT 100
	`)
	if err != nil {
		j.logger.Error("Failed to get recently active users", zap.Error(err))
		return
	}
	defer rows.Close()

	var userIDs []uuid.UUID
	for rows.Next() {
		var userID uuid.UUID
		if err := rows.Scan(&userID); err != nil {
			continue
		}
		userIDs = append(userIDs, userID)
	}

	// Pre-compute dashboards for common periods
	periods := []string{"week", "month"}

	for _, userID := range userIDs {
		for _, period := range periods {
			opts := DashboardOptions{
				UserID:          userID,
				Period:          period,
				TopItemsLimit:   10,
				IncludePatterns: true,
				IncludeMatrix:   false,
				IncludeUnused:   false,
			}

			dashboard, err := j.service.GetUsageDashboard(ctx, opts)
			if err != nil {
				j.logger.Warn("Failed to pre-compute dashboard",
					zap.String("user_id", userID.String()),
					zap.String("period", period),
					zap.Error(err))
				continue
			}

			// Cache the dashboard
			j.cacheDashboard(userID, period, dashboard)
		}
	}

	j.logger.Info("Pre-computed dashboards",
		zap.Int("users", len(userIDs)),
		zap.Int("periods", len(periods)))
}

// cacheDashboard stores a dashboard in cache
func (j *AggregationJob) cacheDashboard(userID uuid.UUID, period string, dashboard *UsageDashboard) {
	j.cache.mu.Lock()
	defer j.cache.mu.Unlock()

	key := fmt.Sprintf("%s:%s", userID.String(), period)
	j.cache.dashboards[key] = &CachedDashboard{
		UserID:      userID,
		Period:      period,
		Dashboard:   dashboard,
		GeneratedAt: time.Now(),
		ExpiresAt:   time.Now().Add(30 * time.Minute),
	}
}

// GetCachedDashboard retrieves a cached dashboard if available
func (j *AggregationJob) GetCachedDashboard(userID uuid.UUID, period string) (*UsageDashboard, bool) {
	j.cache.mu.RLock()
	defer j.cache.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", userID.String(), period)
	cached, exists := j.cache.dashboards[key]
	if !exists {
		return nil, false
	}

	// Check if expired
	if time.Now().After(cached.ExpiresAt) {
		return nil, false
	}

	return cached.Dashboard, true
}

// cleanCache removes expired entries from cache
func (j *AggregationJob) cleanCache() {
	j.cache.mu.Lock()
	defer j.cache.mu.Unlock()

	now := time.Now()

	// Clean dashboards
	for key, cached := range j.cache.dashboards {
		if now.After(cached.ExpiresAt) {
			delete(j.cache.dashboards, key)
		}
	}

	// Clean trends
	for key, cached := range j.cache.trends {
		if now.After(cached.ExpiresAt) {
			delete(j.cache.trends, key)
		}
	}

	j.logger.Debug("Cleaned cache",
		zap.Int("dashboards_remaining", len(j.cache.dashboards)),
		zap.Int("trends_remaining", len(j.cache.trends)))
}

// Helper functions

func getWeekNumber(t time.Time) int {
	_, week := t.ISOWeek()
	return week
}

func getQuarter(t time.Time) int {
	month := int(t.Month())
	return (month-1)/3 + 1
}