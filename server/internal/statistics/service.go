package statistics

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Service handles statistics business logic
type Service struct {
	statsRepo RepositoryInterface
	logger    *zap.Logger
}

// NewService creates a new statistics service
func NewService(statsRepo RepositoryInterface, logger *zap.Logger) *Service {
	return &Service{
		statsRepo: statsRepo,
		logger:    logger,
	}
}

// GetWeeklyOptions represents options for getting weekly statistics
type GetWeeklyOptions struct {
	UserID       uuid.UUID
	Date         time.Time    // Any date within the desired week
	WeekStartDay WeekStartDay // "sunday" or "monday"
	Timezone     string       // IANA timezone (e.g., "America/New_York", "Asia/Seoul")
}

// GetWeeklyStatistics generates comprehensive weekly statistics and analysis
func (s *Service) GetWeeklyStatistics(ctx context.Context, opts GetWeeklyOptions) (*WeeklyStatistics, error) {
	// Default to Monday start if not specified
	if opts.WeekStartDay == "" {
		opts.WeekStartDay = WeekStartMonday
	}

	// Load timezone
	loc := time.UTC
	if opts.Timezone != "" {
		var err error
		loc, err = time.LoadLocation(opts.Timezone)
		if err != nil {
			s.logger.Warn("Invalid timezone, using UTC", zap.String("timezone", opts.Timezone), zap.Error(err))
			loc = time.UTC
		}
	}

	// Calculate week bounds
	startOfWeek, endOfWeek := CalculateWeekBounds(opts.Date, opts.WeekStartDay, loc)
	startOfWeekUTC := startOfWeek.UTC()
	endOfWeekUTC := endOfWeek.UTC()

	// Get previous week bounds for comparison
	prevWeekStart := startOfWeek.AddDate(0, 0, -7)
	prevWeekEnd := prevWeekStart.AddDate(0, 0, 7)
	prevWeekStartUTC := prevWeekStart.UTC()
	prevWeekEndUTC := prevWeekEnd.UTC()

	// Fetch all necessary data in parallel (conceptually - Go will optimize)
	dailyAggs, err := s.statsRepo.GetDailyAggregates(ctx, opts.UserID, startOfWeekUTC, endOfWeekUTC)
	if err != nil {
		s.logger.Error("Failed to get daily aggregates", zap.Error(err))
		return nil, fmt.Errorf("failed to get daily aggregates: %w", err)
	}

	categoryAggs, err := s.statsRepo.GetCategoryAggregates(ctx, opts.UserID, startOfWeekUTC, endOfWeekUTC)
	if err != nil {
		s.logger.Error("Failed to get category aggregates", zap.Error(err))
		return nil, fmt.Errorf("failed to get category aggregates: %w", err)
	}

	timeOfDayAggs, err := s.statsRepo.GetTimeOfDayAggregates(ctx, opts.UserID, startOfWeekUTC, endOfWeekUTC)
	if err != nil {
		s.logger.Error("Failed to get time of day aggregates", zap.Error(err))
		return nil, fmt.Errorf("failed to get time of day aggregates: %w", err)
	}

	dayOfWeekAggs, err := s.statsRepo.GetDayOfWeekAggregates(ctx, opts.UserID, startOfWeekUTC, endOfWeekUTC)
	if err != nil {
		s.logger.Error("Failed to get day of week aggregates", zap.Error(err))
		return nil, fmt.Errorf("failed to get day of week aggregates: %w", err)
	}

	// Get previous week totals for comparison
	prevWeekCount, prevWeekMinutes, err := s.statsRepo.GetWeekTotal(ctx, opts.UserID, prevWeekStartUTC, prevWeekEndUTC)
	if err != nil {
		s.logger.Error("Failed to get previous week total", zap.Error(err))
		return nil, fmt.Errorf("failed to get previous week total: %w", err)
	}

	// Build aggregate map for quick lookup
	aggregateMap := make(map[string]DailyAggregate)
	for _, agg := range dailyAggs {
		localDate := agg.Date.In(loc)
		dateKey := localDate.Format("2006-01-02")
		aggregateMap[dateKey] = agg
	}

	// Get ISO week number
	year, week := GetISOWeekNumber(startOfWeek)

	// Build the statistics structure
	stats := &WeeklyStatistics{
		Year:         year,
		WeekNumber:   week,
		StartDate:    startOfWeek.Format("2006-01-02"),
		EndDate:      endOfWeek.AddDate(0, 0, -1).Format("2006-01-02"),
		WeekStartDay: opts.WeekStartDay,
		Timezone:     opts.Timezone,
		Summary:      s.calculateWeeklySummary(startOfWeek, endOfWeek, aggregateMap, loc),
		DailyBreakdown: s.buildDailyBreakdown(startOfWeek, endOfWeek, aggregateMap, loc),
		DayOfWeekAnalysis: s.buildDayOfWeekAnalysis(dayOfWeekAggs, opts.WeekStartDay),
		TimeDistribution: s.buildTimeDistribution(timeOfDayAggs),
		CategoryBreakdown: s.buildCategoryBreakdown(categoryAggs, dailyAggs),
		Comparison:       s.buildWeekComparison(dailyAggs, prevWeekCount, prevWeekMinutes),
		GeneratedAt:      time.Now().UTC(),
	}

	// Calculate streak
	stats.Streak, err = s.calculateStreak(ctx, opts.UserID, startOfWeek, endOfWeek, aggregateMap, loc)
	if err != nil {
		s.logger.Warn("Failed to calculate streak", zap.Error(err))
		// Don't fail the entire request if streak calculation fails
		stats.Streak = &StreakInfo{}
	}

	// Add navigation
	prevYear, prevWeek := GetISOWeekNumber(prevWeekStart)
	nextYear, nextWeek := GetISOWeekNumber(endOfWeek)
	stats.PreviousWeek = FormatISOWeek(prevYear, prevWeek)
	stats.NextWeek = FormatISOWeek(nextYear, nextWeek)

	// Calculate cache expiration (expire at midnight of current day + 1 day)
	now := time.Now().In(loc)
	tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, loc)
	stats.CacheExpiration = tomorrow.UTC()

	return stats, nil
}

// calculateWeeklySummary computes aggregate statistics for the week
func (s *Service) calculateWeeklySummary(startOfWeek, endOfWeek time.Time, aggregateMap map[string]DailyAggregate, loc *time.Location) *WeeklySummary {
	summary := &WeeklySummary{}

	var mostProductiveCount int
	var leastProductiveCount int = -1 // -1 means not set yet
	var mostProductiveDay string
	var leastProductiveDay string

	// Calculate totals and find most/least productive days
	currentDay := startOfWeek
	for i := 0; i < 7; i++ {
		dateKey := currentDay.Format("2006-01-02")
		if agg, exists := aggregateMap[dateKey]; exists {
			summary.TotalCheckins += agg.CheckinCount
			summary.TotalMinutes += agg.TotalMinutes
			summary.DaysWithCheckins++

			// Most productive
			if agg.CheckinCount > mostProductiveCount {
				mostProductiveCount = agg.CheckinCount
				mostProductiveDay = dateKey
			}

			// Least productive (excluding zero)
			if leastProductiveCount == -1 || (agg.CheckinCount < leastProductiveCount && agg.CheckinCount > 0) {
				leastProductiveCount = agg.CheckinCount
				leastProductiveDay = dateKey
			}
		}
		currentDay = currentDay.AddDate(0, 0, 1)
	}

	// Calculate averages
	if summary.DaysWithCheckins > 0 {
		summary.AveragePerDay = float64(summary.TotalCheckins) / 7.0
	}
	summary.CompletionRate = (float64(summary.DaysWithCheckins) / 7.0) * 100.0

	summary.MostProductiveDay = mostProductiveDay
	summary.MostProductiveCount = mostProductiveCount
	if leastProductiveCount > 0 {
		summary.LeastProductiveDay = leastProductiveDay
	}

	return summary
}

// buildDailyBreakdown creates daily statistics for each day in the week
func (s *Service) buildDailyBreakdown(startOfWeek, endOfWeek time.Time, aggregateMap map[string]DailyAggregate, loc *time.Location) []*DailyBreakdown {
	breakdown := make([]*DailyBreakdown, 0, 7)
	today := time.Now().In(loc).Format("2006-01-02")

	currentDay := startOfWeek
	for i := 0; i < 7; i++ {
		dateKey := currentDay.Format("2006-01-02")
		agg, hasData := aggregateMap[dateKey]

		day := &DailyBreakdown{
			Date:         dateKey,
			DayOfWeek:    GetDayOfWeekName(currentDay),
			CheckinCount: 0,
			TotalMinutes: 0,
			IsToday:      dateKey == today,
			HasCheckins:  hasData,
		}

		if hasData {
			day.CheckinCount = agg.CheckinCount
			day.TotalMinutes = agg.TotalMinutes
			day.CompletionPercent = calculateCompletionPercent(agg.TotalMinutes)
		}

		breakdown = append(breakdown, day)
		currentDay = currentDay.AddDate(0, 0, 1)
	}

	return breakdown
}

// buildDayOfWeekAnalysis creates day-of-week statistics
func (s *Service) buildDayOfWeekAnalysis(dayOfWeekAggs []DayOfWeekAggregate, startDay WeekStartDay) []*DayOfWeekStats {
	// Map day number to statistics
	dayMap := make(map[int]DayOfWeekAggregate)
	totalCheckins := 0

	for _, agg := range dayOfWeekAggs {
		dayMap[agg.DayOfWeek] = agg
		totalCheckins += agg.CheckinCount
	}

	// Build stats for each day
	stats := make([]*DayOfWeekStats, 0, 7)

	// Determine starting day (0 = Sunday, 1 = Monday)
	startDayNum := 1 // Monday
	if startDay == WeekStartSunday {
		startDayNum = 0
	}

	for i := 0; i < 7; i++ {
		dayNum := (startDayNum + i) % 7
		dayName := time.Weekday(dayNum).String()

		dayStat := &DayOfWeekStats{
			DayOfWeek:    dayName,
			CheckinCount: 0,
			TotalMinutes: 0,
			AverageCount: 0,
			Percentage:   0,
		}

		if agg, exists := dayMap[dayNum]; exists {
			dayStat.CheckinCount = agg.CheckinCount
			dayStat.TotalMinutes = agg.TotalMinutes
			dayStat.AverageCount = float64(agg.CheckinCount) // For a single week, this is just the count
			if totalCheckins > 0 {
				dayStat.Percentage = (float64(agg.CheckinCount) / float64(totalCheckins)) * 100.0
			}
		}

		stats = append(stats, dayStat)
	}

	return stats
}

// buildTimeDistribution creates time-of-day distribution statistics
func (s *Service) buildTimeDistribution(timeOfDayAggs []TimeOfDayAggregate) []*TimeDistribution {
	// Aggregate by time period
	periodMap := make(map[TimeOfDay]*TimeDistribution)
	totalCheckins := 0

	for _, agg := range timeOfDayAggs {
		period := GetTimeOfDay(agg.Hour)
		if dist, exists := periodMap[period]; exists {
			dist.CheckinCount += agg.CheckinCount
			dist.TotalMinutes += agg.TotalMinutes
		} else {
			periodMap[period] = &TimeDistribution{
				TimeOfDay:    period,
				CheckinCount: agg.CheckinCount,
				TotalMinutes: agg.TotalMinutes,
			}
		}
		totalCheckins += agg.CheckinCount
	}

	// Calculate percentages and build result
	distribution := make([]*TimeDistribution, 0, 4)
	periods := []TimeOfDay{TimeOfDayMorning, TimeOfDayAfternoon, TimeOfDayEvening, TimeOfDayNight}

	for _, period := range periods {
		if dist, exists := periodMap[period]; exists {
			if totalCheckins > 0 {
				dist.Percentage = (float64(dist.CheckinCount) / float64(totalCheckins)) * 100.0
			}
			distribution = append(distribution, dist)
		} else {
			// Include periods with no data
			distribution = append(distribution, &TimeDistribution{
				TimeOfDay:    period,
				CheckinCount: 0,
				TotalMinutes: 0,
				Percentage:   0,
			})
		}
	}

	return distribution
}

// buildCategoryBreakdown creates category statistics
func (s *Service) buildCategoryBreakdown(categoryAggs []CategoryAggregate, dailyAggs []DailyAggregate) []*CategoryStats {
	// Calculate total for percentages
	var totalCheckins int
	for _, agg := range dailyAggs {
		totalCheckins += agg.CheckinCount
	}

	if totalCheckins == 0 {
		return []*CategoryStats{}
	}

	breakdown := make([]*CategoryStats, 0, len(categoryAggs))
	for _, agg := range categoryAggs {
		percentage := (float64(agg.CheckinCount) / float64(totalCheckins)) * 100.0
		avgPerDay := float64(agg.CheckinCount) / 7.0 // Week has 7 days

		breakdown = append(breakdown, &CategoryStats{
			CategoryID:    agg.CategoryID,
			CategoryName:  agg.CategoryName,
			CheckinCount:  agg.CheckinCount,
			TotalMinutes:  agg.TotalMinutes,
			Percentage:    percentage,
			AveragePerDay: avgPerDay,
		})
	}

	return breakdown
}

// buildWeekComparison creates week-over-week comparison
func (s *Service) buildWeekComparison(dailyAggs []DailyAggregate, prevWeekCount, prevWeekMinutes int) *WeekComparison {
	var currentWeekCount, currentWeekMinutes int
	for _, agg := range dailyAggs {
		currentWeekCount += agg.CheckinCount
		currentWeekMinutes += agg.TotalMinutes
	}

	comparison := &WeekComparison{
		PreviousWeekTotal:    prevWeekCount,
		CurrentWeekTotal:     currentWeekCount,
		Change:               currentWeekCount - prevWeekCount,
		PreviousTotalMinutes: prevWeekMinutes,
		CurrentTotalMinutes:  currentWeekMinutes,
		MinutesChange:        currentWeekMinutes - prevWeekMinutes,
	}

	// Calculate percentage changes
	if prevWeekCount > 0 {
		comparison.ChangePercentage = (float64(comparison.Change) / float64(prevWeekCount)) * 100.0
	}

	if prevWeekMinutes > 0 {
		comparison.MinutesChangePercent = (float64(comparison.MinutesChange) / float64(prevWeekMinutes)) * 100.0
	}

	comparison.IsImprovement = currentWeekCount > prevWeekCount

	return comparison
}

// calculateStreak computes streak information
func (s *Service) calculateStreak(ctx context.Context, userID uuid.UUID, startOfWeek, endOfWeek time.Time, weekAggregateMap map[string]DailyAggregate, loc *time.Location) (*StreakInfo, error) {
	// Get streak data for a broader range (last 90 days to capture longer streaks)
	lookbackStart := startOfWeek.AddDate(0, 0, -90)
	streakDateMap, err := s.statsRepo.GetStreakData(ctx, userID, lookbackStart.UTC(), endOfWeek.UTC())
	if err != nil {
		return nil, err
	}

	// Get last check-in date
	lastCheckinDate, err := s.statsRepo.GetLastCheckinDate(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Calculate current streak
	currentStreak := 0
	longestStreak := 0
	tempStreak := 0
	today := time.Now().In(loc)
	todayStr := today.Format("2006-01-02")
	isStreakActive := false

	// Start from today and go backwards
	currentDay := today
	for i := 0; i < 90; i++ {
		dateKey := currentDay.Format("2006-01-02")

		if streakDateMap[dateKey] {
			tempStreak++
			if tempStreak > longestStreak {
				longestStreak = tempStreak
			}
			if i == 0 || (i == 1 && currentDay.Format("2006-01-02") == todayStr) {
				// Streak is active if there's a check-in today or yesterday
				isStreakActive = true
				currentStreak = tempStreak
			}
		} else {
			if isStreakActive && tempStreak > 0 {
				// Streak ended
				break
			}
			tempStreak = 0
		}

		currentDay = currentDay.AddDate(0, 0, -1)
	}

	streakInfo := &StreakInfo{
		CurrentStreak:  currentStreak,
		LongestStreak:  longestStreak,
		IsStreakActive: isStreakActive,
	}

	if lastCheckinDate != nil {
		streakInfo.LastCheckinDate = lastCheckinDate.Format("2006-01-02")
	}

	// Calculate milestones
	nextMilestone, daysUntil := CalculateStreakMilestones(currentStreak)
	streakInfo.NextMilestone = nextMilestone
	streakInfo.DaysUntilMilestone = daysUntil

	return streakInfo, nil
}

// calculateCompletionPercent calculates the percentage of day covered by check-ins
// Based on total minutes tracked vs 1440 minutes in a day (24 hours * 60 minutes)
func calculateCompletionPercent(totalMinutes int) float64 {
	const minutesPerDay = 1440 // 24 hours * 60 minutes
	if totalMinutes >= minutesPerDay {
		return 100.0
	}
	return (float64(totalMinutes) / float64(minutesPerDay)) * 100.0
}
