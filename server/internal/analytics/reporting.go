package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ReportingService handles analytics reporting and aggregation
type ReportingService struct {
	db           *gorm.DB
	eventService *EventService
	logger       *zap.Logger
}

// NewReportingService creates a new reporting service
func NewReportingService(db *gorm.DB, eventService *EventService, logger *zap.Logger) *ReportingService {
	return &ReportingService{
		db:           db,
		eventService: eventService,
		logger:       logger,
	}
}

// Report represents an analytics report
type Report struct {
	Type      ReportType             `json:"type"`
	Period    ReportPeriod           `json:"period"`
	StartDate time.Time              `json:"start_date"`
	EndDate   time.Time              `json:"end_date"`
	Data      map[string]interface{} `json:"data"`
	Generated time.Time              `json:"generated"`
}

// ReportType represents the type of report
type ReportType string

const (
	ReportTypeUserActivity      ReportType = "user_activity"
	ReportTypeCheckinStats      ReportType = "checkin_stats"
	ReportTypeEngagement        ReportType = "engagement"
	ReportTypeRevenue          ReportType = "revenue"
	ReportTypeFeatureUsage     ReportType = "feature_usage"
	ReportTypeSystemHealth     ReportType = "system_health"
	ReportTypeCustom           ReportType = "custom"
)

// ReportPeriod represents the period of a report
type ReportPeriod string

const (
	PeriodDaily   ReportPeriod = "daily"
	PeriodWeekly  ReportPeriod = "weekly"
	PeriodMonthly ReportPeriod = "monthly"
	PeriodYearly  ReportPeriod = "yearly"
	PeriodCustom  ReportPeriod = "custom"
)

// GenerateReport generates an analytics report
func (r *ReportingService) GenerateReport(ctx context.Context, reportType ReportType, period ReportPeriod, startDate, endDate time.Time) (*Report, error) {
	report := &Report{
		Type:      reportType,
		Period:    period,
		StartDate: startDate,
		EndDate:   endDate,
		Data:      make(map[string]interface{}),
		Generated: time.Now(),
	}

	switch reportType {
	case ReportTypeUserActivity:
		data, err := r.generateUserActivityReport(ctx, startDate, endDate)
		if err != nil {
			return nil, err
		}
		report.Data = data

	case ReportTypeCheckinStats:
		data, err := r.generateCheckinStatsReport(ctx, startDate, endDate)
		if err != nil {
			return nil, err
		}
		report.Data = data

	case ReportTypeEngagement:
		data, err := r.generateEngagementReport(ctx, startDate, endDate)
		if err != nil {
			return nil, err
		}
		report.Data = data

	case ReportTypeRevenue:
		data, err := r.generateRevenueReport(ctx, startDate, endDate)
		if err != nil {
			return nil, err
		}
		report.Data = data

	case ReportTypeFeatureUsage:
		data, err := r.generateFeatureUsageReport(ctx, startDate, endDate)
		if err != nil {
			return nil, err
		}
		report.Data = data

	case ReportTypeSystemHealth:
		data, err := r.generateSystemHealthReport(ctx, startDate, endDate)
		if err != nil {
			return nil, err
		}
		report.Data = data

	default:
		return nil, fmt.Errorf("unsupported report type: %s", reportType)
	}

	return report, nil
}

// generateUserActivityReport generates user activity report
func (r *ReportingService) generateUserActivityReport(ctx context.Context, start, end time.Time) (map[string]interface{}, error) {
	data := make(map[string]interface{})

	// New users
	var newUsers int64
	err := r.db.WithContext(ctx).
		Model(&Event{}).
		Where("event_type = ? AND timestamp BETWEEN ? AND ?", EventUserSignup, start, end).
		Count(&newUsers).Error
	if err != nil {
		return nil, err
	}
	data["new_users"] = newUsers

	// Active users
	var activeUsers int64
	err = r.db.WithContext(ctx).
		Model(&Event{}).
		Where("user_id IS NOT NULL AND timestamp BETWEEN ? AND ?", start, end).
		Distinct("user_id").
		Count(&activeUsers).Error
	if err != nil {
		return nil, err
	}
	data["active_users"] = activeUsers

	// Daily active users (DAU)
	dauByDay := make(map[string]int64)
	rows, err := r.db.WithContext(ctx).
		Model(&Event{}).
		Select("DATE(timestamp) as date, COUNT(DISTINCT user_id) as count").
		Where("user_id IS NOT NULL AND timestamp BETWEEN ? AND ?", start, end).
		Group("DATE(timestamp)").
		Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var date time.Time
		var count int64
		if err := rows.Scan(&date, &count); err == nil {
			dauByDay[date.Format("2006-01-02")] = count
		}
	}
	data["daily_active_users"] = dauByDay

	// User retention (simplified - users who returned after first day)
	var retentionRate float64
	// This would require more complex calculation in production
	data["retention_rate"] = retentionRate

	// Login events
	var loginCount int64
	err = r.db.WithContext(ctx).
		Model(&Event{}).
		Where("event_type = ? AND timestamp BETWEEN ? AND ?", EventUserLogin, start, end).
		Count(&loginCount).Error
	if err != nil {
		return nil, err
	}
	data["login_count"] = loginCount

	return data, nil
}

// generateCheckinStatsReport generates checkin statistics report
func (r *ReportingService) generateCheckinStatsReport(ctx context.Context, start, end time.Time) (map[string]interface{}, error) {
	data := make(map[string]interface{})

	// Total checkins
	var totalCheckins int64
	err := r.db.WithContext(ctx).
		Model(&Event{}).
		Where("event_type = ? AND timestamp BETWEEN ? AND ?", EventCheckinCreated, start, end).
		Count(&totalCheckins).Error
	if err != nil {
		return nil, err
	}
	data["total_checkins"] = totalCheckins

	// Checkins by day
	checkinsByDay := make(map[string]int64)
	rows, err := r.db.WithContext(ctx).
		Model(&Event{}).
		Select("DATE(timestamp) as date, COUNT(*) as count").
		Where("event_type = ? AND timestamp BETWEEN ? AND ?", EventCheckinCreated, start, end).
		Group("DATE(timestamp)").
		Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var date time.Time
		var count int64
		if err := rows.Scan(&date, &count); err == nil {
			checkinsByDay[date.Format("2006-01-02")] = count
		}
	}
	data["checkins_by_day"] = checkinsByDay

	// Average checkins per user
	var uniqueUsers int64
	err = r.db.WithContext(ctx).
		Model(&Event{}).
		Where("event_type = ? AND timestamp BETWEEN ? AND ? AND user_id IS NOT NULL", EventCheckinCreated, start, end).
		Distinct("user_id").
		Count(&uniqueUsers).Error
	if err != nil {
		return nil, err
	}

	avgCheckinsPerUser := float64(0)
	if uniqueUsers > 0 {
		avgCheckinsPerUser = float64(totalCheckins) / float64(uniqueUsers)
	}
	data["avg_checkins_per_user"] = avgCheckinsPerUser

	// Checkins by hour of day
	checkinsByHour := make(map[int]int64)
	hourRows, err := r.db.WithContext(ctx).
		Model(&Event{}).
		Select("EXTRACT(HOUR FROM timestamp) as hour, COUNT(*) as count").
		Where("event_type = ? AND timestamp BETWEEN ? AND ?", EventCheckinCreated, start, end).
		Group("EXTRACT(HOUR FROM timestamp)").
		Rows()
	if err != nil {
		r.logger.Warn("Failed to get checkins by hour", zap.Error(err))
	} else {
		defer hourRows.Close()
		for hourRows.Next() {
			var hour int
			var count int64
			if err := hourRows.Scan(&hour, &count); err == nil {
				checkinsByHour[hour] = count
			}
		}
	}
	data["checkins_by_hour"] = checkinsByHour

	// Streak statistics
	var streaksStarted, streaksBroken int64
	r.db.WithContext(ctx).
		Model(&Event{}).
		Where("event_type = ? AND timestamp BETWEEN ? AND ?", EventCheckinStreakStarted, start, end).
		Count(&streaksStarted)
	r.db.WithContext(ctx).
		Model(&Event{}).
		Where("event_type = ? AND timestamp BETWEEN ? AND ?", EventCheckinStreakBroken, start, end).
		Count(&streaksBroken)

	data["streaks_started"] = streaksStarted
	data["streaks_broken"] = streaksBroken

	return data, nil
}

// generateEngagementReport generates user engagement report
func (r *ReportingService) generateEngagementReport(ctx context.Context, start, end time.Time) (map[string]interface{}, error) {
	data := make(map[string]interface{})

	// Session duration analysis would require session tracking
	// For now, we'll track event frequency as a proxy

	// Events per user
	type UserEventCount struct {
		UserID uuid.UUID
		Count  int64
	}

	var userEventCounts []UserEventCount
	err := r.db.WithContext(ctx).
		Model(&Event{}).
		Select("user_id, COUNT(*) as count").
		Where("user_id IS NOT NULL AND timestamp BETWEEN ? AND ?", start, end).
		Group("user_id").
		Scan(&userEventCounts).Error
	if err != nil {
		return nil, err
	}

	// Calculate engagement metrics
	totalUsers := len(userEventCounts)
	totalEvents := int64(0)
	highlyEngaged := 0 // Users with >10 events

	for _, uec := range userEventCounts {
		totalEvents += uec.Count
		if uec.Count > 10 {
			highlyEngaged++
		}
	}

	avgEventsPerUser := float64(0)
	if totalUsers > 0 {
		avgEventsPerUser = float64(totalEvents) / float64(totalUsers)
	}

	data["total_users"] = totalUsers
	data["total_events"] = totalEvents
	data["avg_events_per_user"] = avgEventsPerUser
	data["highly_engaged_users"] = highlyEngaged

	// Feature adoption
	featureEvents := []EventType{
		EventExportCreated,
		EventWebhookTriggered,
		EventAPICallMade,
	}

	featureAdoption := make(map[string]int64)
	for _, eventType := range featureEvents {
		var count int64
		r.db.WithContext(ctx).
			Model(&Event{}).
			Where("event_type = ? AND timestamp BETWEEN ? AND ?", eventType, start, end).
			Distinct("user_id").
			Count(&count)
		featureAdoption[string(eventType)] = count
	}
	data["feature_adoption"] = featureAdoption

	return data, nil
}

// generateRevenueReport generates revenue report
func (r *ReportingService) generateRevenueReport(ctx context.Context, start, end time.Time) (map[string]interface{}, error) {
	data := make(map[string]interface{})

	// Subscription events
	var newSubscriptions, upgrades, downgrades, cancellations int64

	r.db.WithContext(ctx).
		Model(&Event{}).
		Where("event_type = ? AND timestamp BETWEEN ? AND ?", EventSubscriptionCreated, start, end).
		Count(&newSubscriptions)

	r.db.WithContext(ctx).
		Model(&Event{}).
		Where("event_type = ? AND timestamp BETWEEN ? AND ?", EventSubscriptionUpgraded, start, end).
		Count(&upgrades)

	r.db.WithContext(ctx).
		Model(&Event{}).
		Where("event_type = ? AND timestamp BETWEEN ? AND ?", EventSubscriptionDowngraded, start, end).
		Count(&downgrades)

	r.db.WithContext(ctx).
		Model(&Event{}).
		Where("event_type = ? AND timestamp BETWEEN ? AND ?", EventSubscriptionCanceled, start, end).
		Count(&cancellations)

	data["new_subscriptions"] = newSubscriptions
	data["upgrades"] = upgrades
	data["downgrades"] = downgrades
	data["cancellations"] = cancellations

	// Calculate MRR change (simplified)
	// In production, this would query actual payment data
	mrrChange := (newSubscriptions * 999) + (upgrades * 4000) - (downgrades * 4000) - (cancellations * 999)
	data["mrr_change_cents"] = mrrChange

	// Churn rate
	churnRate := float64(0)
	if newSubscriptions > 0 {
		churnRate = float64(cancellations) / float64(newSubscriptions) * 100
	}
	data["churn_rate"] = churnRate

	return data, nil
}

// generateFeatureUsageReport generates feature usage report
func (r *ReportingService) generateFeatureUsageReport(ctx context.Context, start, end time.Time) (map[string]interface{}, error) {
	data := make(map[string]interface{})

	// Get all feature usage events
	var events []Event
	err := r.db.WithContext(ctx).
		Where("event_type = ? AND timestamp BETWEEN ? AND ?", EventFeatureUsed, start, end).
		Find(&events).Error
	if err != nil {
		return nil, err
	}

	// Aggregate by feature
	featureUsage := make(map[string]int64)
	featureUsers := make(map[string]map[string]bool)

	for _, event := range events {
		if featureName, ok := event.EventData["feature"].(string); ok {
			featureUsage[featureName]++

			// Track unique users per feature
			if featureUsers[featureName] == nil {
				featureUsers[featureName] = make(map[string]bool)
			}
			if event.UserID != nil {
				featureUsers[featureName][event.UserID.String()] = true
			}
		}
	}

	// Convert to final format
	features := make(map[string]map[string]interface{})
	for feature, count := range featureUsage {
		features[feature] = map[string]interface{}{
			"total_uses":   count,
			"unique_users": len(featureUsers[feature]),
		}
	}
	data["features"] = features

	// API usage
	var apiCalls int64
	err = r.db.WithContext(ctx).
		Model(&Event{}).
		Where("event_type = ? AND timestamp BETWEEN ? AND ?", EventAPICallMade, start, end).
		Count(&apiCalls).Error
	if err != nil {
		return nil, err
	}
	data["api_calls"] = apiCalls

	// Export usage
	var exports int64
	err = r.db.WithContext(ctx).
		Model(&Event{}).
		Where("event_type = ? AND timestamp BETWEEN ? AND ?", EventExportCreated, start, end).
		Count(&exports).Error
	if err != nil {
		return nil, err
	}
	data["exports"] = exports

	return data, nil
}

// generateSystemHealthReport generates system health report
func (r *ReportingService) generateSystemHealthReport(ctx context.Context, start, end time.Time) (map[string]interface{}, error) {
	data := make(map[string]interface{})

	// Error events
	var errorCount int64
	err := r.db.WithContext(ctx).
		Model(&Event{}).
		Where("event_type = ? AND timestamp BETWEEN ? AND ?", EventErrorOccurred, start, end).
		Count(&errorCount).Error
	if err != nil {
		return nil, err
	}
	data["error_count"] = errorCount

	// Rate limit hits
	var rateLimitHits int64
	err = r.db.WithContext(ctx).
		Model(&Event{}).
		Where("event_type = ? AND timestamp BETWEEN ? AND ?", EventRateLimitHit, start, end).
		Count(&rateLimitHits).Error
	if err != nil {
		return nil, err
	}
	data["rate_limit_hits"] = rateLimitHits

	// Get real-time metrics
	realTimeMetrics, err := r.eventService.GetRealTimeMetrics(ctx)
	if err != nil {
		r.logger.Warn("Failed to get real-time metrics", zap.Error(err))
	} else {
		data["real_time"] = realTimeMetrics
	}

	// Response time percentiles (would require APM data)
	// For now, placeholder
	data["response_time_p50"] = 25  // ms
	data["response_time_p95"] = 100 // ms
	data["response_time_p99"] = 250 // ms

	// Uptime (simplified)
	data["uptime_percentage"] = 99.9

	return data, nil
}

// GetDashboardMetrics gets metrics for the analytics dashboard
func (r *ReportingService) GetDashboardMetrics(ctx context.Context) (map[string]interface{}, error) {
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	weekStart := todayStart.AddDate(0, 0, -int(todayStart.Weekday()))
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	metrics := make(map[string]interface{})

	// Today's metrics
	todayReport, err := r.GenerateReport(ctx, ReportTypeUserActivity, PeriodDaily, todayStart, now)
	if err != nil {
		r.logger.Warn("Failed to generate today's report", zap.Error(err))
	} else {
		metrics["today"] = todayReport.Data
	}

	// This week's metrics
	weekReport, err := r.GenerateReport(ctx, ReportTypeCheckinStats, PeriodWeekly, weekStart, now)
	if err != nil {
		r.logger.Warn("Failed to generate weekly report", zap.Error(err))
	} else {
		metrics["week"] = weekReport.Data
	}

	// This month's metrics
	monthReport, err := r.GenerateReport(ctx, ReportTypeEngagement, PeriodMonthly, monthStart, now)
	if err != nil {
		r.logger.Warn("Failed to generate monthly report", zap.Error(err))
	} else {
		metrics["month"] = monthReport.Data
	}

	// Real-time metrics
	realTime, err := r.eventService.GetRealTimeMetrics(ctx)
	if err != nil {
		r.logger.Warn("Failed to get real-time metrics", zap.Error(err))
	} else {
		metrics["real_time"] = realTime
	}

	return metrics, nil
}