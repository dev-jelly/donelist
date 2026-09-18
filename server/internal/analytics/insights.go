package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// InsightsEngine provides AI-powered insights and pattern analysis
type InsightsEngine struct {
	service *Service
	logger  *zap.Logger
}

// NewInsightsEngine creates a new insights engine
func NewInsightsEngine(service *Service, logger *zap.Logger) *InsightsEngine {
	return &InsightsEngine{
		service: service,
		logger:  logger,
	}
}

// Insight represents a generated insight
type Insight struct {
	Type        string                 `json:"type"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Severity    string                 `json:"severity"` // "info", "warning", "success", "critical"
	Score       float64                `json:"score"`    // 0-100
	Data        map[string]interface{} `json:"data,omitempty"`
	ActionItems []string               `json:"action_items,omitempty"`
	GeneratedAt time.Time              `json:"generated_at"`
}

// Pattern represents a detected behavioral pattern
type Pattern struct {
	PatternType  string                 `json:"pattern_type"`
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	Confidence   float64                `json:"confidence"` // 0-1
	Frequency    string                 `json:"frequency"`  // "daily", "weekly", "occasional"
	Data         map[string]interface{} `json:"data,omitempty"`
	FirstSeen    time.Time              `json:"first_seen"`
	LastSeen     time.Time              `json:"last_seen"`
	OccurrenceCount int                 `json:"occurrence_count"`
}

// Recommendation represents a personalized recommendation
type Recommendation struct {
	Type        string    `json:"type"`
	Priority    string    `json:"priority"` // "high", "medium", "low"
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Impact      string    `json:"impact"`
	Effort      string    `json:"effort"` // "low", "medium", "high"
	Benefits    []string  `json:"benefits"`
	GeneratedAt time.Time `json:"generated_at"`
}

// GenerateInsights generates comprehensive insights for a user
func (e *InsightsEngine) GenerateInsights(ctx context.Context, userID uuid.UUID) ([]Insight, error) {
	insights := []Insight{}

	// Try cache first
	cacheKey := e.service.cache.CacheKey("insights", userID.String())
	if found, err := e.service.cache.Get(ctx, cacheKey, &insights); err == nil && found {
		return insights, nil
	}

	// Generate various insights
	streakInsights, err := e.generateStreakInsights(ctx, userID)
	if err != nil {
		e.logger.Warn("Failed to generate streak insights", zap.Error(err))
	} else {
		insights = append(insights, streakInsights...)
	}

	productivityInsights, err := e.generateProductivityInsights(ctx, userID)
	if err != nil {
		e.logger.Warn("Failed to generate productivity insights", zap.Error(err))
	} else {
		insights = append(insights, productivityInsights...)
	}

	categoryInsights, err := e.generateCategoryInsights(ctx, userID)
	if err != nil {
		e.logger.Warn("Failed to generate category insights", zap.Error(err))
	} else {
		insights = append(insights, categoryInsights...)
	}

	timePatternInsights, err := e.generateTimePatternInsights(ctx, userID)
	if err != nil {
		e.logger.Warn("Failed to generate time pattern insights", zap.Error(err))
	} else {
		insights = append(insights, timePatternInsights...)
	}

	// Cache the results
	e.service.cache.Set(ctx, cacheKey, insights, CacheTTLHourly)

	return insights, nil
}

// generateStreakInsights generates insights about user streaks
func (e *InsightsEngine) generateStreakInsights(ctx context.Context, userID uuid.UUID) ([]Insight, error) {
	insights := []Insight{}

	streakMetrics, err := e.service.GetStreakMetrics(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Insight: Current streak status
	if streakMetrics.CurrentStreak > 0 {
		severity := "info"
		if streakMetrics.CurrentStreak >= 7 {
			severity = "success"
		}
		if streakMetrics.CurrentStreak >= 30 {
			severity = "critical" // Positive critical
		}

		insights = append(insights, Insight{
			Type:        "streak_active",
			Title:       fmt.Sprintf("%d-Day Streak!", streakMetrics.CurrentStreak),
			Description: fmt.Sprintf("You're on a %d-day streak! Keep it going!", streakMetrics.CurrentStreak),
			Severity:    severity,
			Score:       math.Min(100, float64(streakMetrics.CurrentStreak)*3.33),
			Data: map[string]interface{}{
				"current_streak": streakMetrics.CurrentStreak,
				"longest_streak": streakMetrics.LongestStreak,
			},
			GeneratedAt: time.Now(),
		})
	} else {
		// Streak broken insight
		insights = append(insights, Insight{
			Type:        "streak_broken",
			Title:       "Streak Broken",
			Description: "Your streak has ended. Start a new one today!",
			Severity:    "warning",
			Score:       20,
			ActionItems: []string{
				"Create a check-in today to start a new streak",
				"Set a daily reminder to maintain consistency",
			},
			GeneratedAt: time.Now(),
		})
	}

	// Insight: Approaching longest streak
	if streakMetrics.CurrentStreak > 0 && streakMetrics.CurrentStreak >= streakMetrics.LongestStreak-3 {
		daysToRecord := streakMetrics.LongestStreak - streakMetrics.CurrentStreak + 1
		insights = append(insights, Insight{
			Type:        "streak_milestone",
			Title:       "Approaching Your Record!",
			Description: fmt.Sprintf("Just %d more day(s) to break your longest streak of %d days!", daysToRecord, streakMetrics.LongestStreak),
			Severity:    "success",
			Score:       90,
			Data: map[string]interface{}{
				"days_to_record": daysToRecord,
				"longest_streak": streakMetrics.LongestStreak,
			},
			GeneratedAt: time.Now(),
		})
	}

	// Insight: Consistency score
	if streakMetrics.TotalActiveDays > 0 {
		consistencyScore := (float64(streakMetrics.TotalActiveDays) / 90) * 100 // Based on last 90 days
		if consistencyScore > 100 {
			consistencyScore = 100
		}

		severity := "info"
		if consistencyScore >= 70 {
			severity = "success"
		} else if consistencyScore < 40 {
			severity = "warning"
		}

		insights = append(insights, Insight{
			Type:        "consistency_score",
			Title:       fmt.Sprintf("Consistency Score: %.0f%%", consistencyScore),
			Description: fmt.Sprintf("You've been active %d days. Keep building this habit!", streakMetrics.TotalActiveDays),
			Severity:    severity,
			Score:       consistencyScore,
			Data: map[string]interface{}{
				"consistency_score": consistencyScore,
				"active_days":       streakMetrics.TotalActiveDays,
			},
			GeneratedAt: time.Now(),
		})
	}

	return insights, nil
}

// generateProductivityInsights generates productivity-related insights
func (e *InsightsEngine) generateProductivityInsights(ctx context.Context, userID uuid.UUID) ([]Insight, error) {
	insights := []Insight{}

	// Get weekly activity for last 4 weeks
	now := time.Now()
	thisWeek := now.AddDate(0, 0, -int(now.Weekday()))
	thisWeek = time.Date(thisWeek.Year(), thisWeek.Month(), thisWeek.Day(), 0, 0, 0, 0, time.UTC)

	thisWeekReport, err := e.service.GetWeeklyActivity(ctx, userID, thisWeek)
	if err != nil {
		return nil, err
	}

	lastWeek := thisWeek.AddDate(0, 0, -7)
	lastWeekReport, err := e.service.GetWeeklyActivity(ctx, userID, lastWeek)
	if err != nil {
		e.logger.Warn("Failed to get last week report", zap.Error(err))
	}

	// Insight: Weekly trend
	if lastWeekReport != nil {
		changePercent := 0.0
		if lastWeekReport.TotalCheckins > 0 {
			changePercent = ((float64(thisWeekReport.TotalCheckins) - float64(lastWeekReport.TotalCheckins)) / float64(lastWeekReport.TotalCheckins)) * 100
		}

		severity := "info"
		title := "Activity Stable"
		description := fmt.Sprintf("You have %d check-ins this week, similar to last week.", thisWeekReport.TotalCheckins)

		if changePercent > 10 {
			severity = "success"
			title = fmt.Sprintf("Activity Up %.0f%%!", changePercent)
			description = fmt.Sprintf("Great work! You're %.0f%% more active this week with %d check-ins.", changePercent, thisWeekReport.TotalCheckins)
		} else if changePercent < -10 {
			severity = "warning"
			title = fmt.Sprintf("Activity Down %.0f%%", math.Abs(changePercent))
			description = fmt.Sprintf("Your activity decreased %.0f%% this week. Let's get back on track!", math.Abs(changePercent))
		}

		insights = append(insights, Insight{
			Type:        "weekly_trend",
			Title:       title,
			Description: description,
			Severity:    severity,
			Score:       50 + changePercent, // 50 baseline, adjust by percent change
			Data: map[string]interface{}{
				"this_week":      thisWeekReport.TotalCheckins,
				"last_week":      lastWeekReport.TotalCheckins,
				"change_percent": changePercent,
			},
			GeneratedAt: time.Now(),
		})
	}

	// Insight: Peak productivity time
	if thisWeekReport.PeakHour >= 0 {
		timeOfDay := "morning"
		if thisWeekReport.PeakHour >= 12 && thisWeekReport.PeakHour < 18 {
			timeOfDay = "afternoon"
		} else if thisWeekReport.PeakHour >= 18 {
			timeOfDay = "evening"
		}

		insights = append(insights, Insight{
			Type:        "peak_hour",
			Title:       fmt.Sprintf("Most Active at %d:00", thisWeekReport.PeakHour),
			Description: fmt.Sprintf("You're most productive in the %s. Consider scheduling important tasks during this time.", timeOfDay),
			Severity:    "info",
			Score:       70,
			Data: map[string]interface{}{
				"peak_hour":   thisWeekReport.PeakHour,
				"time_of_day": timeOfDay,
			},
			ActionItems: []string{
				fmt.Sprintf("Schedule important tasks around %d:00", thisWeekReport.PeakHour),
				"Block this time for focused work",
			},
			GeneratedAt: time.Now(),
		})
	}

	// Insight: Most productive day
	if !thisWeekReport.MostProductiveDay.IsZero() {
		dayName := thisWeekReport.MostProductiveDay.Weekday().String()
		insights = append(insights, Insight{
			Type:        "productive_day",
			Title:       fmt.Sprintf("%s is Your Power Day", dayName),
			Description: fmt.Sprintf("You're most active on %ss. Leverage this for challenging tasks!", dayName),
			Severity:    "success",
			Score:       75,
			Data: map[string]interface{}{
				"day_name": dayName,
				"date":     thisWeekReport.MostProductiveDay,
			},
			GeneratedAt: time.Now(),
		})
	}

	return insights, nil
}

// generateCategoryInsights generates category-related insights
func (e *InsightsEngine) generateCategoryInsights(ctx context.Context, userID uuid.UUID) ([]Insight, error) {
	insights := []Insight{}

	categories, err := e.service.GetCategoryPerformance(ctx, userID, 10)
	if err != nil {
		return nil, err
	}

	if len(categories) == 0 {
		return insights, nil
	}

	// Insight: Top category
	topCategory := categories[0]
	insights = append(insights, Insight{
		Type:        "top_category",
		Title:       fmt.Sprintf("Top Focus: %s", topCategory.CategoryName),
		Description: fmt.Sprintf("You've used %s %d times. This is your most tracked activity.", topCategory.CategoryName, topCategory.TotalUsage),
		Severity:    "success",
		Score:       80,
		Data: map[string]interface{}{
			"category_name": topCategory.CategoryName,
			"total_usage":   topCategory.TotalUsage,
			"days_used":     topCategory.DaysUsed,
		},
		GeneratedAt: time.Now(),
	})

	// Insight: Neglected categories
	for _, cat := range categories {
		if cat.TimeSinceLastUse != nil && *cat.TimeSinceLastUse > 168 { // More than 7 days
			daysSince := *cat.TimeSinceLastUse / 24
			insights = append(insights, Insight{
				Type:        "neglected_category",
				Title:       fmt.Sprintf("Haven't Used %s Recently", cat.CategoryName),
				Description: fmt.Sprintf("It's been %d days since you last used %s.", daysSince, cat.CategoryName),
				Severity:    "warning",
				Score:       40,
				Data: map[string]interface{}{
					"category_name": cat.CategoryName,
					"days_since":    daysSince,
				},
				ActionItems: []string{
					fmt.Sprintf("Consider if %s is still relevant to your goals", cat.CategoryName),
					"Archive unused categories to reduce clutter",
				},
				GeneratedAt: time.Now(),
			})
		}
	}

	// Insight: Rising category
	for _, cat := range categories {
		if cat.TrendDirection == "up" && cat.TrendPercentage > 50 {
			insights = append(insights, Insight{
				Type:        "trending_category",
				Title:       fmt.Sprintf("%s is Trending Up!", cat.CategoryName),
				Description: fmt.Sprintf("Usage of %s increased %.0f%% compared to last week.", cat.CategoryName, cat.TrendPercentage),
				Severity:    "success",
				Score:       85,
				Data: map[string]interface{}{
					"category_name":   cat.CategoryName,
					"trend_percent":   cat.TrendPercentage,
					"current_usage":   cat.UsageLast7Days,
					"previous_usage":  cat.UsagePrev7Days,
				},
				GeneratedAt: time.Now(),
			})
		}
	}

	return insights, nil
}

// generateTimePatternInsights generates time-based pattern insights
func (e *InsightsEngine) generateTimePatternInsights(ctx context.Context, userID uuid.UUID) ([]Insight, error) {
	insights := []Insight{}

	// Get hourly patterns
	rows, err := e.service.sqlDB.QueryContext(ctx, `
		SELECT
			hour_of_day,
			day_of_week,
			checkin_count,
			unique_categories
		FROM mv_hourly_activity_patterns
		WHERE user_id = $1
		ORDER BY checkin_count DESC
		LIMIT 10
	`, userID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type HourPattern struct {
		Hour             int
		DayOfWeek        int
		CheckinCount     int
		UniqueCategories int
	}

	patterns := []HourPattern{}
	for rows.Next() {
		var p HourPattern
		if err := rows.Scan(&p.Hour, &p.DayOfWeek, &p.CheckinCount, &p.UniqueCategories); err != nil {
			continue
		}
		patterns = append(patterns, p)
	}

	if len(patterns) > 0 {
		// Find early bird vs night owl pattern
		earlyMorningCount := 0
		lateNightCount := 0

		for _, p := range patterns {
			if p.Hour >= 5 && p.Hour < 9 {
				earlyMorningCount += p.CheckinCount
			} else if p.Hour >= 22 || p.Hour < 2 {
				lateNightCount += p.CheckinCount
			}
		}

		if earlyMorningCount > lateNightCount && earlyMorningCount > 10 {
			insights = append(insights, Insight{
				Type:        "chronotype",
				Title:       "You're an Early Bird!",
				Description: "You're most active in the early morning. Morning routines work best for you.",
				Severity:    "info",
				Score:       70,
				Data: map[string]interface{}{
					"early_morning_count": earlyMorningCount,
					"late_night_count":    lateNightCount,
				},
				ActionItems: []string{
					"Schedule your most important tasks in the morning",
					"Establish a consistent morning routine",
				},
				GeneratedAt: time.Now(),
			})
		} else if lateNightCount > earlyMorningCount && lateNightCount > 10 {
			insights = append(insights, Insight{
				Type:        "chronotype",
				Title:       "You're a Night Owl!",
				Description: "You're most active in the evening. Late-day productivity works best for you.",
				Severity:    "info",
				Score:       70,
				Data: map[string]interface{}{
					"early_morning_count": earlyMorningCount,
					"late_night_count":    lateNightCount,
				},
				ActionItems: []string{
					"Reserve complex tasks for evening hours",
					"Protect your evening focus time",
				},
				GeneratedAt: time.Now(),
			})
		}
	}

	return insights, nil
}

// DetectPatterns detects behavioral patterns in user activity
func (e *InsightsEngine) DetectPatterns(ctx context.Context, userID uuid.UUID) ([]Pattern, error) {
	patterns := []Pattern{}

	// Try cache first
	cacheKey := e.service.cache.CacheKey("patterns", userID.String())
	if found, err := e.service.cache.Get(ctx, cacheKey, &patterns); err == nil && found {
		return patterns, nil
	}

	// Detect various patterns
	timePatterns, err := e.detectTimePatterns(ctx, userID)
	if err != nil {
		e.logger.Warn("Failed to detect time patterns", zap.Error(err))
	} else {
		patterns = append(patterns, timePatterns...)
	}

	categoryPatterns, err := e.detectCategoryPatterns(ctx, userID)
	if err != nil {
		e.logger.Warn("Failed to detect category patterns", zap.Error(err))
	} else {
		patterns = append(patterns, categoryPatterns...)
	}

	weekendPatterns, err := e.detectWeekendPatterns(ctx, userID)
	if err != nil {
		e.logger.Warn("Failed to detect weekend patterns", zap.Error(err))
	} else {
		patterns = append(patterns, weekendPatterns...)
	}

	// Cache the results
	e.service.cache.Set(ctx, cacheKey, patterns, CacheTTLHourly)

	return patterns, nil
}

// detectTimePatterns detects time-based patterns
func (e *InsightsEngine) detectTimePatterns(ctx context.Context, userID uuid.UUID) ([]Pattern, error) {
	patterns := []Pattern{}

	// Query for consistent time patterns
	rows, err := e.service.sqlDB.QueryContext(ctx, `
		SELECT
			hour_of_day,
			day_of_week,
			checkin_count,
			occurrence_days
		FROM mv_hourly_activity_patterns
		WHERE user_id = $1
		  AND occurrence_days >= 4
		  AND checkin_count >= 10
		ORDER BY checkin_count DESC
		LIMIT 5
	`, userID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var hour, dayOfWeek, checkinCount, occurrenceDays int
		if err := rows.Scan(&hour, &dayOfWeek, &checkinCount, &occurrenceDays); err != nil {
			continue
		}

		dayName := time.Weekday(dayOfWeek).String()
		confidence := math.Min(1.0, float64(occurrenceDays)/10.0)

		patterns = append(patterns, Pattern{
			PatternType:     "time_routine",
			Name:            fmt.Sprintf("%s at %d:00 Routine", dayName, hour),
			Description:     fmt.Sprintf("You consistently check in on %ss around %d:00", dayName, hour),
			Confidence:      confidence,
			Frequency:       "weekly",
			OccurrenceCount: occurrenceDays,
			Data: map[string]interface{}{
				"hour":         hour,
				"day_of_week":  dayOfWeek,
				"checkin_count": checkinCount,
			},
			LastSeen: time.Now(),
		})
	}

	return patterns, nil
}

// detectCategoryPatterns detects category usage patterns
func (e *InsightsEngine) detectCategoryPatterns(ctx context.Context, userID uuid.UUID) ([]Pattern, error) {
	patterns := []Pattern{}

	// Get category trends
	rows, err := e.service.sqlDB.QueryContext(ctx, `
		SELECT
			c.name,
			COUNT(*) as weeks_used,
			AVG(usage_count) as avg_weekly_usage,
			MIN(week_start) as first_seen,
			MAX(week_start) as last_seen
		FROM mv_category_usage_trends t
		JOIN categories c ON t.category_id = c.id
		WHERE t.user_id = $1
		  AND t.week_start >= CURRENT_DATE - INTERVAL '12 weeks'
		GROUP BY t.category_id, c.name
		HAVING COUNT(*) >= 6
		ORDER BY COUNT(*) DESC
		LIMIT 5
	`, userID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		var weeksUsed int
		var avgWeeklyUsage float64
		var firstSeen, lastSeen time.Time

		if err := rows.Scan(&name, &weeksUsed, &avgWeeklyUsage, &firstSeen, &lastSeen); err != nil {
			continue
		}

		confidence := math.Min(1.0, float64(weeksUsed)/12.0)

		patterns = append(patterns, Pattern{
			PatternType:     "category_habit",
			Name:            fmt.Sprintf("Regular %s Activity", name),
			Description:     fmt.Sprintf("You consistently use %s category, averaging %.0f times per week", name, avgWeeklyUsage),
			Confidence:      confidence,
			Frequency:       "weekly",
			OccurrenceCount: weeksUsed,
			FirstSeen:       firstSeen,
			LastSeen:        lastSeen,
			Data: map[string]interface{}{
				"category_name":    name,
				"avg_weekly_usage": avgWeeklyUsage,
			},
		})
	}

	return patterns, nil
}

// detectWeekendPatterns detects weekend vs weekday patterns
func (e *InsightsEngine) detectWeekendPatterns(ctx context.Context, userID uuid.UUID) ([]Pattern, error) {
	patterns := []Pattern{}

	// Compare weekend vs weekday activity
	var weekdayAvg, weekendAvg float64
	err := e.service.sqlDB.QueryRowContext(ctx, `
		SELECT
			AVG(CASE WHEN day_of_week IN (1,2,3,4,5) THEN total_checkins ELSE 0 END) as weekday_avg,
			AVG(CASE WHEN day_of_week IN (0,6) THEN total_checkins ELSE 0 END) as weekend_avg
		FROM mv_daily_user_activity
		WHERE user_id = $1
		  AND activity_date >= CURRENT_DATE - INTERVAL '30 days'
	`, userID).Scan(&weekdayAvg, &weekendAvg)

	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	if weekdayAvg > 0 || weekendAvg > 0 {
		ratio := weekendAvg / math.Max(weekdayAvg, 1)

		if ratio > 1.3 {
			patterns = append(patterns, Pattern{
				PatternType:     "weekend_warrior",
				Name:            "Weekend Warrior",
				Description:     fmt.Sprintf("You're %.0f%% more active on weekends than weekdays", (ratio-1)*100),
				Confidence:      0.85,
				Frequency:       "weekly",
				OccurrenceCount: 4,
				Data: map[string]interface{}{
					"weekday_avg": weekdayAvg,
					"weekend_avg": weekendAvg,
					"ratio":       ratio,
				},
				LastSeen: time.Now(),
			})
		} else if ratio < 0.7 {
			patterns = append(patterns, Pattern{
				PatternType:     "weekday_focused",
				Name:            "Weekday Focused",
				Description:     fmt.Sprintf("You're %.0f%% more active on weekdays than weekends", (1/ratio-1)*100),
				Confidence:      0.85,
				Frequency:       "weekly",
				OccurrenceCount: 4,
				Data: map[string]interface{}{
					"weekday_avg": weekdayAvg,
					"weekend_avg": weekendAvg,
					"ratio":       ratio,
				},
				LastSeen: time.Now(),
			})
		}
	}

	return patterns, nil
}

// GenerateRecommendations generates personalized recommendations
func (e *InsightsEngine) GenerateRecommendations(ctx context.Context, userID uuid.UUID) ([]Recommendation, error) {
	recommendations := []Recommendation{}

	// Try cache first
	cacheKey := e.service.cache.CacheKey("recommendations", userID.String())
	if found, err := e.service.cache.Get(ctx, cacheKey, &recommendations); err == nil && found {
		return recommendations, nil
	}

	// Generate based on patterns and insights
	insights, err := e.GenerateInsights(ctx, userID)
	if err != nil {
		e.logger.Warn("Failed to generate insights for recommendations", zap.Error(err))
	}

	patterns, err := e.DetectPatterns(ctx, userID)
	if err != nil {
		e.logger.Warn("Failed to detect patterns for recommendations", zap.Error(err))
	}

	// Generate recommendations based on insights
	for _, insight := range insights {
		switch insight.Type {
		case "streak_broken":
			recommendations = append(recommendations, Recommendation{
				Type:        "consistency",
				Priority:    "high",
				Title:       "Rebuild Your Streak",
				Description: "Start a new streak today. Consistency is key to building lasting habits.",
				Impact:      "Rebuilding your streak can increase motivation by 40%",
				Effort:      "low",
				Benefits: []string{
					"Increased motivation and accountability",
					"Better habit formation",
					"Improved long-term consistency",
				},
				GeneratedAt: time.Now(),
			})

		case "neglected_category":
			if categoryName, ok := insight.Data["category_name"].(string); ok {
				recommendations = append(recommendations, Recommendation{
					Type:        "category_review",
					Priority:    "medium",
					Title:       fmt.Sprintf("Review %s Category", categoryName),
					Description: fmt.Sprintf("You haven't used %s recently. Consider if it's still relevant to your goals.", categoryName),
					Impact:      "Cleaning up unused categories reduces cognitive overhead",
					Effort:      "low",
					Benefits: []string{
						"Reduced clutter in your categories",
						"Better focus on active goals",
						"Improved app organization",
					},
					GeneratedAt: time.Now(),
				})
			}
		}
	}

	// Generate recommendations based on patterns
	for _, pattern := range patterns {
		switch pattern.PatternType {
		case "time_routine":
			if hour, ok := pattern.Data["hour"].(int); ok {
				recommendations = append(recommendations, Recommendation{
					Type:        "schedule_optimization",
					Priority:    "medium",
					Title:       "Leverage Your Routine",
					Description: fmt.Sprintf("You have a consistent routine at %d:00. Consider scheduling important tasks during this time.", hour),
					Impact:      "Aligning tasks with natural rhythms increases productivity by 25%",
					Effort:      "medium",
					Benefits: []string{
						"Better task completion rates",
						"Reduced decision fatigue",
						"Improved time management",
					},
					GeneratedAt: time.Now(),
				})
			}
		}
	}

	// Generic recommendations for improvement
	streakMetrics, err := e.service.GetStreakMetrics(ctx, userID)
	if err == nil && streakMetrics.CurrentStreak == 0 {
		recommendations = append(recommendations, Recommendation{
			Type:        "engagement",
			Priority:    "high",
			Title:       "Set a Daily Check-in Goal",
			Description: "Create at least one check-in per day to build consistency.",
			Impact:      "Daily check-ins increase habit success rate by 50%",
			Effort:      "low",
			Benefits: []string{
				"Build consistent tracking habits",
				"Better visibility into your activities",
				"Increased accountability",
			},
			GeneratedAt: time.Now(),
		})
	}

	// Cache the results
	e.service.cache.Set(ctx, cacheKey, recommendations, CacheTTLHourly)

	return recommendations, nil
}

// FocusScore calculates a user's focus score (0-100)
type FocusScore struct {
	Score          float64                `json:"score"`
	Level          string                 `json:"level"` // "low", "medium", "high", "excellent"
	Factors        map[string]float64     `json:"factors"`
	Recommendations []string              `json:"recommendations"`
	CalculatedAt   time.Time              `json:"calculated_at"`
}

// CalculateFocusScore calculates a comprehensive focus score
func (e *InsightsEngine) CalculateFocusScore(ctx context.Context, userID uuid.UUID) (*FocusScore, error) {
	factors := make(map[string]float64)

	// Factor 1: Streak consistency (0-25 points)
	streakMetrics, err := e.service.GetStreakMetrics(ctx, userID)
	if err == nil {
		streakScore := math.Min(25, float64(streakMetrics.CurrentStreak)*2.5)
		factors["streak_consistency"] = streakScore
	}

	// Factor 2: Weekly activity (0-25 points)
	now := time.Now()
	weekStart := now.AddDate(0, 0, -int(now.Weekday()))
	weekStart = time.Date(weekStart.Year(), weekStart.Month(), weekStart.Day(), 0, 0, 0, 0, time.UTC)

	weeklyReport, err := e.service.GetWeeklyActivity(ctx, userID, weekStart)
	if err == nil {
		// 25 points for 7 days of activity
		activityScore := math.Min(25, float64(len(weeklyReport.DailyBreakdown))*3.57)
		factors["weekly_activity"] = activityScore
	}

	// Factor 3: Category diversity (0-20 points)
	categories, err := e.service.GetCategoryPerformance(ctx, userID, 10)
	if err == nil && len(categories) > 0 {
		// More categories show broader focus
		diversityScore := math.Min(20, float64(len(categories))*4)
		factors["category_diversity"] = diversityScore
	}

	// Factor 4: Time efficiency (0-30 points)
	// Check for consistent time patterns
	var patternCount int
	err = e.service.sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM mv_hourly_activity_patterns
		WHERE user_id = $1
		  AND occurrence_days >= 3
	`, userID).Scan(&patternCount)

	if err == nil {
		efficiencyScore := math.Min(30, float64(patternCount)*6)
		factors["time_efficiency"] = efficiencyScore
	}

	// Calculate total score
	totalScore := 0.0
	for _, score := range factors {
		totalScore += score
	}

	// Determine level
	level := "low"
	if totalScore >= 80 {
		level = "excellent"
	} else if totalScore >= 60 {
		level = "high"
	} else if totalScore >= 40 {
		level = "medium"
	}

	// Generate recommendations
	recommendations := []string{}
	if factors["streak_consistency"] < 15 {
		recommendations = append(recommendations, "Build a consistent daily streak")
	}
	if factors["weekly_activity"] < 15 {
		recommendations = append(recommendations, "Increase weekly check-in frequency")
	}
	if factors["category_diversity"] < 10 {
		recommendations = append(recommendations, "Track more diverse activities")
	}
	if factors["time_efficiency"] < 18 {
		recommendations = append(recommendations, "Establish consistent time routines")
	}

	return &FocusScore{
		Score:           totalScore,
		Level:           level,
		Factors:         factors,
		Recommendations: recommendations,
		CalculatedAt:    time.Now(),
	}, nil
}
