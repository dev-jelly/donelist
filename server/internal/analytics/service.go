package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Service provides comprehensive analytics functionality
type Service struct {
	db           *gorm.DB
	sqlDB        *sql.DB
	cache        *AnalyticsCache
	eventService *EventService
	logger       *zap.Logger
}

// NewService creates a new analytics service
func NewService(db *gorm.DB, redis *redis.Client, logger *zap.Logger) *Service {
	sqlDB, err := db.DB()
	if err != nil {
		logger.Fatal("Failed to get SQL DB", zap.Error(err))
	}

	return &Service{
		db:           db,
		sqlDB:        sqlDB,
		cache:        NewAnalyticsCache(redis, logger),
		eventService: NewEventService(db, redis, logger),
		logger:       logger,
	}
}

// DailyActivitySummary represents daily user activity metrics
type DailyActivitySummary struct {
	UserID            uuid.UUID `json:"user_id"`
	ActivityDate      time.Time `json:"activity_date"`
	TotalCheckins     int       `json:"total_checkins"`
	UniqueCategories  int       `json:"unique_categories"`
	ActiveHours       int       `json:"active_hours"`
	TotalMinutes      int       `json:"total_minutes"`
	FirstCheckin      time.Time `json:"first_checkin"`
	LastCheckin       time.Time `json:"last_checkin"`
	DayOfWeek         int       `json:"day_of_week"`
	FirstActivityHour int       `json:"first_activity_hour"`
	LastActivityHour  int       `json:"last_activity_hour"`
}

// GetDailyActivity retrieves daily activity summary from materialized view with caching
func (s *Service) GetDailyActivity(ctx context.Context, userID uuid.UUID, date time.Time) (*DailyActivitySummary, error) {
	// Try cache first
	var summary DailyActivitySummary
	if found, err := s.cache.GetDailySummary(ctx, userID, date, &summary); err == nil && found {
		s.logger.Debug("Daily activity cache hit",
			zap.String("user_id", userID.String()),
			zap.Time("date", date))
		return &summary, nil
	}

	// Query materialized view
	dateStr := date.Format("2006-01-02")
	err := s.sqlDB.QueryRowContext(ctx, `
		SELECT
			user_id, activity_date, total_checkins, unique_categories,
			active_hours, total_minutes, first_checkin, last_checkin,
			day_of_week, first_activity_hour, last_activity_hour
		FROM mv_daily_user_activity
		WHERE user_id = $1 AND activity_date = $2
	`, userID, dateStr).Scan(
		&summary.UserID, &summary.ActivityDate, &summary.TotalCheckins,
		&summary.UniqueCategories, &summary.ActiveHours, &summary.TotalMinutes,
		&summary.FirstCheckin, &summary.LastCheckin, &summary.DayOfWeek,
		&summary.FirstActivityHour, &summary.LastActivityHour,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get daily activity: %w", err)
	}

	// Cache the result
	s.cache.CacheDailySummary(ctx, userID, date, &summary)

	return &summary, nil
}

// WeeklyActivityReport represents weekly activity metrics
type WeeklyActivityReport struct {
	UserID            uuid.UUID              `json:"user_id"`
	WeekStart         time.Time              `json:"week_start"`
	WeekEnd           time.Time              `json:"week_end"`
	TotalCheckins     int                    `json:"total_checkins"`
	DailyBreakdown    []*DailyActivitySummary `json:"daily_breakdown"`
	AveragePerDay     float64                `json:"average_per_day"`
	MostProductiveDay time.Time              `json:"most_productive_day"`
	PeakHour          int                    `json:"peak_hour"`
	TotalMinutes      int                    `json:"total_minutes"`
}

// GetWeeklyActivity retrieves weekly activity report
func (s *Service) GetWeeklyActivity(ctx context.Context, userID uuid.UUID, weekStart time.Time) (*WeeklyActivityReport, error) {
	// Try cache first
	var report WeeklyActivityReport
	if found, err := s.cache.GetWeeklySummary(ctx, userID, weekStart, &report); err == nil && found {
		return &report, nil
	}

	weekEnd := weekStart.AddDate(0, 0, 7)
	report = WeeklyActivityReport{
		UserID:    userID,
		WeekStart: weekStart,
		WeekEnd:   weekEnd,
	}

	// Get daily breakdown for the week
	dailyBreakdown := make([]*DailyActivitySummary, 0, 7)
	for i := 0; i < 7; i++ {
		currentDay := weekStart.AddDate(0, 0, i)
		summary, err := s.GetDailyActivity(ctx, userID, currentDay)
		if err != nil {
			s.logger.Warn("Failed to get daily activity",
				zap.Time("date", currentDay),
				zap.Error(err))
			continue
		}
		if summary != nil {
			dailyBreakdown = append(dailyBreakdown, summary)
			report.TotalCheckins += summary.TotalCheckins
			report.TotalMinutes += summary.TotalMinutes
		}
	}

	report.DailyBreakdown = dailyBreakdown

	if len(dailyBreakdown) > 0 {
		report.AveragePerDay = float64(report.TotalCheckins) / 7.0

		// Find most productive day
		maxCheckins := 0
		for _, day := range dailyBreakdown {
			if day.TotalCheckins > maxCheckins {
				maxCheckins = day.TotalCheckins
				report.MostProductiveDay = day.ActivityDate
			}
		}
	}

	// Get peak hour from hourly patterns
	var peakHour int
	err := s.sqlDB.QueryRowContext(ctx, `
		SELECT hour_of_day
		FROM mv_hourly_activity_patterns
		WHERE user_id = $1
		GROUP BY hour_of_day
		ORDER BY SUM(checkin_count) DESC
		LIMIT 1
	`, userID).Scan(&peakHour)

	if err == nil {
		report.PeakHour = peakHour
	}

	// Cache the result
	s.cache.CacheWeeklySummary(ctx, userID, weekStart, &report)

	return &report, nil
}

// CategoryPerformance represents category usage performance metrics
type CategoryPerformance struct {
	CategoryID        uuid.UUID  `json:"category_id"`
	CategoryName      string     `json:"category_name"`
	Color             *string    `json:"color,omitempty"`
	TotalUsage        int        `json:"total_usage"`
	DaysUsed          int        `json:"days_used"`
	TotalMinutes      int        `json:"total_minutes"`
	AvgDuration       float64    `json:"avg_duration_minutes"`
	FirstUsed         time.Time  `json:"first_used"`
	LastUsed          time.Time  `json:"last_used"`
	TimeSinceLastUse  *int       `json:"hours_since_last_use,omitempty"`
	UsageLast7Days    int        `json:"usage_last_7_days"`
	UsagePrev7Days    int        `json:"usage_prev_7_days"`
	TrendPercentage   float64    `json:"trend_percentage"`
	TrendDirection    string     `json:"trend_direction"` // "up", "down", "stable"
}

// GetCategoryPerformance retrieves category performance metrics
func (s *Service) GetCategoryPerformance(ctx context.Context, userID uuid.UUID, limit int) ([]*CategoryPerformance, error) {
	// Try cache first
	var performance []*CategoryPerformance
	filters := map[string]string{"user_id": userID.String(), "limit": fmt.Sprintf("%d", limit)}

	if found, err := s.cache.GetMaterializedView(ctx, "category_performance", filters, &performance); err == nil && found {
		return performance, nil
	}

	rows, err := s.sqlDB.QueryContext(ctx, `
		SELECT
			category_id, category_name, color, total_usage, days_used,
			total_minutes, avg_duration_minutes, first_used, last_used,
			EXTRACT(EPOCH FROM time_since_last_use) / 3600 as hours_since_last_use,
			usage_last_7_days, usage_prev_7_days
		FROM mv_category_performance
		WHERE user_id = $1
		ORDER BY total_usage DESC
		LIMIT $2
	`, userID, limit)

	if err != nil {
		return nil, fmt.Errorf("failed to get category performance: %w", err)
	}
	defer rows.Close()

	performance = make([]*CategoryPerformance, 0, limit)

	for rows.Next() {
		var cp CategoryPerformance
		var hoursSinceLastUse sql.NullFloat64

		err := rows.Scan(
			&cp.CategoryID, &cp.CategoryName, &cp.Color, &cp.TotalUsage,
			&cp.DaysUsed, &cp.TotalMinutes, &cp.AvgDuration, &cp.FirstUsed,
			&cp.LastUsed, &hoursSinceLastUse, &cp.UsageLast7Days, &cp.UsagePrev7Days,
		)

		if err != nil {
			s.logger.Error("Failed to scan category performance", zap.Error(err))
			continue
		}

		if hoursSinceLastUse.Valid {
			hours := int(hoursSinceLastUse.Float64)
			cp.TimeSinceLastUse = &hours
		}

		// Calculate trend
		if cp.UsagePrev7Days > 0 {
			cp.TrendPercentage = ((float64(cp.UsageLast7Days) - float64(cp.UsagePrev7Days)) / float64(cp.UsagePrev7Days)) * 100
		}

		if cp.TrendPercentage > 10 {
			cp.TrendDirection = "up"
		} else if cp.TrendPercentage < -10 {
			cp.TrendDirection = "down"
		} else {
			cp.TrendDirection = "stable"
		}

		performance = append(performance, &cp)
	}

	// Cache the results
	s.cache.CacheMaterializedView(ctx, "category_performance", filters, performance)

	return performance, nil
}

// StreakMetrics represents user streak information
type StreakMetrics struct {
	UserID            uuid.UUID `json:"user_id"`
	LongestStreak     int       `json:"longest_streak"`
	CurrentStreak     int       `json:"current_streak"`
	LastCheckinDate   time.Time `json:"last_checkin_date"`
	TotalStreakPeriods int      `json:"total_streak_periods"`
	AvgStreakLength   float64   `json:"avg_streak_length"`
	TotalActiveDays   int       `json:"total_active_days"`
	IsActive          bool      `json:"is_active"`
}

// GetStreakMetrics retrieves user streak metrics
func (s *Service) GetStreakMetrics(ctx context.Context, userID uuid.UUID) (*StreakMetrics, error) {
	// Try cache first
	var metrics StreakMetrics
	if found, err := s.cache.GetStreakInfo(ctx, userID, &metrics); err == nil && found {
		return &metrics, nil
	}

	err := s.sqlDB.QueryRowContext(ctx, `
		SELECT
			user_id, longest_streak, current_streak, last_checkin_date,
			total_streak_periods, avg_streak_length, total_active_days
		FROM mv_user_streaks
		WHERE user_id = $1
	`, userID).Scan(
		&metrics.UserID, &metrics.LongestStreak, &metrics.CurrentStreak,
		&metrics.LastCheckinDate, &metrics.TotalStreakPeriods,
		&metrics.AvgStreakLength, &metrics.TotalActiveDays,
	)

	if err == sql.ErrNoRows {
		return &StreakMetrics{UserID: userID}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get streak metrics: %w", err)
	}

	metrics.IsActive = metrics.CurrentStreak > 0

	// Cache the result
	s.cache.CacheStreakInfo(ctx, userID, &metrics)

	return &metrics, nil
}

// RefreshMaterializedViews refreshes all analytics materialized views
func (s *Service) RefreshMaterializedViews(ctx context.Context) error {
	s.logger.Info("Refreshing analytics materialized views")

	_, err := s.sqlDB.ExecContext(ctx, "SELECT refresh_analytics_materialized_views()")
	if err != nil {
		s.logger.Error("Failed to refresh materialized views", zap.Error(err))
		return fmt.Errorf("failed to refresh materialized views: %w", err)
	}

	// Invalidate all materialized view caches
	s.cache.InvalidatePattern(ctx, s.cache.CacheKey("mv", "*"))

	s.logger.Info("Analytics materialized views refreshed successfully")
	return nil
}

// RefreshRealtimeViews refreshes time-sensitive materialized views
func (s *Service) RefreshRealtimeViews(ctx context.Context) error {
	s.logger.Debug("Refreshing real-time analytics views")

	_, err := s.sqlDB.ExecContext(ctx, "SELECT refresh_realtime_analytics_views()")
	if err != nil {
		s.logger.Error("Failed to refresh real-time views", zap.Error(err))
		return fmt.Errorf("failed to refresh real-time views: %w", err)
	}

	// Invalidate relevant caches
	s.cache.InvalidatePattern(ctx, s.cache.CacheKey("daily", "*"))
	s.cache.InvalidatePattern(ctx, s.cache.CacheKey("streak", "*"))

	return nil
}

// InvalidateUserAnalytics invalidates all analytics cache for a user
func (s *Service) InvalidateUserAnalytics(ctx context.Context, userID uuid.UUID) error {
	return s.cache.InvalidateUserCache(ctx, userID)
}

// GetCacheStats returns analytics cache statistics
func (s *Service) GetCacheStats(ctx context.Context) (map[string]interface{}, error) {
	return s.cache.GetCacheStats(ctx)
}

// TrackEvent is a convenience method to track analytics events
func (s *Service) TrackEvent(ctx context.Context, event *Event) error {
	return s.eventService.TrackEvent(ctx, event)
}

// GetRealTimeMetrics gets real-time metrics from Redis
func (s *Service) GetRealTimeMetrics(ctx context.Context) (map[string]interface{}, error) {
	return s.eventService.GetRealTimeMetrics(ctx)
}

// MonthlyComparisonReport represents month-over-month comparison
type MonthlyComparisonReport struct {
	CurrentMonth      time.Time              `json:"current_month"`
	PreviousMonth     time.Time              `json:"previous_month"`
	CurrentStats      *MonthlyStats          `json:"current_stats"`
	PreviousStats     *MonthlyStats          `json:"previous_stats"`
	ChangePercentage  map[string]float64     `json:"change_percentage"`
	Improvements      []string               `json:"improvements"`
	Declines          []string               `json:"declines"`
}

// MonthlyStats represents monthly statistics
type MonthlyStats struct {
	TotalCheckins     int     `json:"total_checkins"`
	UniqueCategories  int     `json:"unique_categories"`
	ActiveDays        int     `json:"active_days"`
	TotalMinutes      int     `json:"total_minutes"`
	AvgDurationPerCheckin float64 `json:"avg_duration_per_checkin"`
	WeekendCheckins   int     `json:"weekend_checkins"`
	WeekdayCheckins   int     `json:"weekday_checkins"`
	MorningCheckins   int     `json:"morning_checkins"`
	AfternoonCheckins int     `json:"afternoon_checkins"`
	EveningCheckins   int     `json:"evening_checkins"`
}

// GetMonthlyComparison retrieves month-over-month comparison
func (s *Service) GetMonthlyComparison(ctx context.Context, userID uuid.UUID, month time.Time) (*MonthlyComparisonReport, error) {
	currentMonth := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
	previousMonth := currentMonth.AddDate(0, -1, 0)

	currentStats, err := s.getMonthlyStats(ctx, userID, currentMonth)
	if err != nil {
		return nil, fmt.Errorf("failed to get current month stats: %w", err)
	}

	previousStats, err := s.getMonthlyStats(ctx, userID, previousMonth)
	if err != nil {
		return nil, fmt.Errorf("failed to get previous month stats: %w", err)
	}

	report := &MonthlyComparisonReport{
		CurrentMonth:     currentMonth,
		PreviousMonth:    previousMonth,
		CurrentStats:     currentStats,
		PreviousStats:    previousStats,
		ChangePercentage: make(map[string]float64),
		Improvements:     []string{},
		Declines:         []string{},
	}

	// Calculate change percentages
	if previousStats.TotalCheckins > 0 {
		change := ((float64(currentStats.TotalCheckins) - float64(previousStats.TotalCheckins)) / float64(previousStats.TotalCheckins)) * 100
		report.ChangePercentage["total_checkins"] = change
		if change > 5 {
			report.Improvements = append(report.Improvements, fmt.Sprintf("Check-ins increased by %.1f%%", change))
		} else if change < -5 {
			report.Declines = append(report.Declines, fmt.Sprintf("Check-ins decreased by %.1f%%", -change))
		}
	}

	if previousStats.ActiveDays > 0 {
		change := ((float64(currentStats.ActiveDays) - float64(previousStats.ActiveDays)) / float64(previousStats.ActiveDays)) * 100
		report.ChangePercentage["active_days"] = change
		if change > 5 {
			report.Improvements = append(report.Improvements, fmt.Sprintf("Active days increased by %.1f%%", change))
		} else if change < -5 {
			report.Declines = append(report.Declines, fmt.Sprintf("Active days decreased by %.1f%%", -change))
		}
	}

	return report, nil
}

func (s *Service) getMonthlyStats(ctx context.Context, userID uuid.UUID, month time.Time) (*MonthlyStats, error) {
	var stats MonthlyStats

	err := s.sqlDB.QueryRowContext(ctx, `
		SELECT
			total_checkins, unique_categories, active_days, total_minutes,
			avg_duration_per_checkin, weekend_checkins, weekday_checkins,
			morning_checkins, afternoon_checkins, evening_checkins
		FROM mv_monthly_summary
		WHERE user_id = $1 AND month = $2
	`, userID, month).Scan(
		&stats.TotalCheckins, &stats.UniqueCategories, &stats.ActiveDays,
		&stats.TotalMinutes, &stats.AvgDurationPerCheckin, &stats.WeekendCheckins,
		&stats.WeekdayCheckins, &stats.MorningCheckins, &stats.AfternoonCheckins,
		&stats.EveningCheckins,
	)

	if err == sql.ErrNoRows {
		return &MonthlyStats{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get monthly stats: %w", err)
	}

	return &stats, nil
}
