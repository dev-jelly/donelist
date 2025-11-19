package calendar

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Service handles calendar business logic
type Service struct {
	calendarRepo *Repository
	logger       *zap.Logger
}

// NewService creates a new calendar service
func NewService(calendarRepo *Repository, logger *zap.Logger) *Service {
	return &Service{
		calendarRepo: calendarRepo,
		logger:       logger,
	}
}

// GetOptions represents options for getting calendar data
type GetOptions struct {
	UserID   uuid.UUID
	Year     int
	Month    int
	StartDay StartDay // "sunday" or "monday"
	Timezone string   // IANA timezone (e.g., "America/New_York", "Asia/Seoul")
}

// GetMonthlyCalendar generates a complete monthly calendar view
func (s *Service) GetMonthlyCalendar(ctx context.Context, opts GetOptions) (*MonthlyCalendar, error) {
	// Default to Monday start if not specified
	if opts.StartDay == "" {
		opts.StartDay = StartDayMonday
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

	// Get start and end of month in the specified timezone
	startOfMonth := time.Date(opts.Year, time.Month(opts.Month), 1, 0, 0, 0, 0, loc)
	endOfMonth := startOfMonth.AddDate(0, 1, 0)

	// Convert to UTC for database queries
	startOfMonthUTC := startOfMonth.UTC()
	endOfMonthUTC := endOfMonth.UTC()

	// Get daily aggregates from repository
	aggregates, err := s.calendarRepo.GetDailyAggregates(ctx, opts.UserID, startOfMonthUTC, endOfMonthUTC)
	if err != nil {
		s.logger.Error("Failed to get daily aggregates", zap.Error(err))
		return nil, fmt.Errorf("failed to get daily aggregates: %w", err)
	}

	// Build map for quick lookup
	aggregateMap := make(map[string]DailyAggregate)
	for _, agg := range aggregates {
		// Convert UTC date to local timezone
		localDate := agg.Date.In(loc)
		dateKey := localDate.Format("2006-01-02")
		aggregateMap[dateKey] = agg
	}

	// Get category aggregates
	categoryAggs, err := s.calendarRepo.GetCategoryAggregates(ctx, opts.UserID, startOfMonthUTC, endOfMonthUTC)
	if err != nil {
		s.logger.Error("Failed to get category aggregates", zap.Error(err))
		return nil, fmt.Errorf("failed to get category aggregates: %w", err)
	}

	// Build calendar structure
	calendar := &MonthlyCalendar{
		Year:        opts.Year,
		Month:       opts.Month,
		MonthName:   time.Month(opts.Month).String(),
		StartDay:    opts.StartDay,
		Weeks:       s.buildCalendarWeeks(startOfMonth, endOfMonth, opts.StartDay, aggregateMap, loc),
		Summary:     s.calculateMonthlySummary(startOfMonth, endOfMonth, aggregateMap, loc),
		Categories:  s.buildCategoryBreakdown(categoryAggs, aggregates),
		GeneratedAt: time.Now().UTC(),
	}

	// Calculate cache expiration (expire at midnight of current day + 1 day)
	now := time.Now().In(loc)
	tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, loc)
	calendar.CacheExpiration = tomorrow.UTC()

	// Add navigation links
	prevMonth := startOfMonth.AddDate(0, -1, 0)
	nextMonth := startOfMonth.AddDate(0, 1, 0)
	calendar.PreviousMonth = fmt.Sprintf("%04d-%02d", prevMonth.Year(), prevMonth.Month())
	calendar.NextMonth = fmt.Sprintf("%04d-%02d", nextMonth.Year(), nextMonth.Month())

	return calendar, nil
}

// buildCalendarWeeks creates the week structure for the calendar
func (s *Service) buildCalendarWeeks(startOfMonth, endOfMonth time.Time, startDay StartDay, aggregateMap map[string]DailyAggregate, loc *time.Location) []*CalendarWeek {
	// Determine the first day to display (may be from previous month)
	firstDayOfMonth := startOfMonth
	firstWeekday := int(firstDayOfMonth.Weekday())

	// Adjust based on start day preference
	var daysBack int
	if startDay == StartDaySunday {
		// Sunday = 0, so we go back firstWeekday days
		daysBack = firstWeekday
	} else {
		// Monday start: Sunday = 0 becomes 6 days back, Monday = 1 becomes 0 days back
		if firstWeekday == 0 {
			daysBack = 6
		} else {
			daysBack = firstWeekday - 1
		}
	}

	calendarStart := firstDayOfMonth.AddDate(0, 0, -daysBack)

	// Build weeks until we've covered the entire month
	var weeks []*CalendarWeek
	currentDay := calendarStart
	today := time.Now().In(loc).Format("2006-01-02")

	for currentDay.Before(endOfMonth) || len(weeks) == 0 || len(weeks[len(weeks)-1].Days) < 7 {
		// Start a new week if needed
		if len(weeks) == 0 || len(weeks[len(weeks)-1].Days) == 7 {
			weeks = append(weeks, &CalendarWeek{Days: make([]*CalendarDay, 0, 7)})
		}

		// Build calendar day
		dateKey := currentDay.Format("2006-01-02")
		agg, hasData := aggregateMap[dateKey]

		calDay := &CalendarDay{
			Date:           dateKey,
			CheckinCount:   0,
			TotalMinutes:   0,
			IsCurrentMonth: currentDay.Month() == startOfMonth.Month(),
			IsToday:        dateKey == today,
			HasCheckins:    hasData,
		}

		if hasData {
			calDay.CheckinCount = agg.CheckinCount
			calDay.TotalMinutes = agg.TotalMinutes
			calDay.CompletionPercent = CalculateCompletionPercent(agg.TotalMinutes)
			calDay.ColorIntensity = GetColorIntensity(calDay.CompletionPercent)
		}

		weeks[len(weeks)-1].Days = append(weeks[len(weeks)-1].Days, calDay)
		currentDay = currentDay.AddDate(0, 0, 1)

		// Stop when we have at least 4 weeks and have passed the end of month
		if len(weeks) >= 4 && currentDay.After(endOfMonth) && len(weeks[len(weeks)-1].Days) == 7 {
			break
		}
	}

	return weeks
}

// calculateMonthlySummary computes aggregate statistics for the month
func (s *Service) calculateMonthlySummary(startOfMonth, endOfMonth time.Time, aggregateMap map[string]DailyAggregate, loc *time.Location) *MonthlySummary {
	summary := &MonthlySummary{
		TotalDaysInMonth: int(endOfMonth.Sub(startOfMonth).Hours() / 24),
	}

	var mostProductiveCount int
	var mostProductiveDay string

	// Calculate totals and find most productive day
	currentDay := startOfMonth
	for currentDay.Before(endOfMonth) {
		dateKey := currentDay.Format("2006-01-02")
		if agg, exists := aggregateMap[dateKey]; exists {
			summary.TotalCheckins += agg.CheckinCount
			summary.TotalMinutes += agg.TotalMinutes
			summary.DaysWithCheckins++

			if agg.CheckinCount > mostProductiveCount {
				mostProductiveCount = agg.CheckinCount
				mostProductiveDay = dateKey
			}
		}
		currentDay = currentDay.AddDate(0, 0, 1)
	}

	// Calculate averages
	if summary.TotalDaysInMonth > 0 {
		summary.AveragePerDay = float64(summary.TotalCheckins) / float64(summary.TotalDaysInMonth)
		summary.CompletionRate = (float64(summary.DaysWithCheckins) / float64(summary.TotalDaysInMonth)) * 100.0
	}

	summary.MostProductiveDay = mostProductiveDay
	summary.MostProductiveCount = mostProductiveCount

	// Calculate streaks
	summary.CurrentStreak, summary.LongestStreak = s.calculateStreaks(startOfMonth, endOfMonth, aggregateMap)

	return summary
}

// calculateStreaks computes current and longest streaks
func (s *Service) calculateStreaks(startOfMonth, endOfMonth time.Time, aggregateMap map[string]DailyAggregate) (int, int) {
	var currentStreak, longestStreak int
	var tempStreak int

	currentDay := startOfMonth
	today := time.Now().Format("2006-01-02")
	isStreakActive := true

	for currentDay.Before(endOfMonth) {
		dateKey := currentDay.Format("2006-01-02")

		if _, hasCheckins := aggregateMap[dateKey]; hasCheckins {
			tempStreak++
			if tempStreak > longestStreak {
				longestStreak = tempStreak
			}
		} else {
			// Streak broken
			if isStreakActive && dateKey <= today {
				// Current streak ended
				isStreakActive = false
			}
			tempStreak = 0
		}

		// Update current streak if we're still active
		if isStreakActive {
			currentStreak = tempStreak
		}

		currentDay = currentDay.AddDate(0, 0, 1)
	}

	return currentStreak, longestStreak
}

// buildCategoryBreakdown creates category statistics
func (s *Service) buildCategoryBreakdown(categoryAggs []CategoryAggregate, dailyAggs []DailyAggregate) []*CategoryBreakdown {
	// Calculate total for percentages
	var total int
	for _, agg := range dailyAggs {
		total += agg.CheckinCount
	}

	if total == 0 {
		return []*CategoryBreakdown{}
	}

	breakdown := make([]*CategoryBreakdown, 0, len(categoryAggs))
	for _, agg := range categoryAggs {
		percentage := (float64(agg.Count) / float64(total)) * 100.0
		breakdown = append(breakdown, &CategoryBreakdown{
			CategoryID:   agg.CategoryID,
			CategoryName: agg.CategoryName,
			Count:        agg.Count,
			Percentage:   percentage,
		})
	}

	return breakdown
}
