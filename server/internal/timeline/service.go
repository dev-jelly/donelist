package timeline

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/dev-jelly/donelist/internal/category"
	"github.com/dev-jelly/donelist/internal/checkin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// BlockGranularity represents the time block size in minutes
type BlockGranularity int

const (
	Block15Min BlockGranularity = 15
	Block30Min BlockGranularity = 30
	Block45Min BlockGranularity = 45
	Block2Hour BlockGranularity = 120
)

// ValidateBlockGranularity validates and returns the block granularity
func ValidateBlockGranularity(minutes int) (BlockGranularity, error) {
	switch minutes {
	case 15:
		return Block15Min, nil
	case 30:
		return Block30Min, nil
	case 45:
		return Block45Min, nil
	case 120:
		return Block2Hour, nil
	default:
		return Block30Min, fmt.Errorf("invalid block granularity: %d, use 15, 30, 45, or 120", minutes)
	}
}

// TimeBlock represents a time slot in the timeline
type TimeBlock struct {
	StartTime    time.Time          `json:"start_time"`    // RFC3339 format
	EndTime      time.Time          `json:"end_time"`      // RFC3339 format
	DurationMins int                `json:"duration_mins"` // Block duration
	Checkins     []*CheckinWithMeta `json:"checkins"`      // Check-ins in this block
	IsEmpty      bool               `json:"is_empty"`      // True if no check-ins
	IsGap        bool               `json:"is_gap"`        // True if gap between check-ins
}

// CheckinWithMeta wraps a check-in with additional metadata
type CheckinWithMeta struct {
	*checkin.Checkin
	CategoryName  *string `json:"category_name,omitempty"`
	CategoryColor *string `json:"category_color,omitempty"`
	CategoryIcon  *string `json:"category_icon,omitempty"`
}

// Gap represents a time gap between check-ins
type Gap struct {
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	DurationMins int       `json:"duration_mins"`
}

// DailySummary represents aggregate statistics for a day
type DailySummary struct {
	Date              string  `json:"date"`                // YYYY-MM-DD
	TotalCheckins     int     `json:"total_checkins"`      // Total number of check-ins
	TotalMinutes      int     `json:"total_minutes"`       // Total tracked time
	FirstCheckinTime  *string `json:"first_checkin_time"`  // Time of first check-in
	LastCheckinTime   *string `json:"last_checkin_time"`   // Time of last check-in
	ActiveHours       float64 `json:"active_hours"`        // Hours with activity
	GapCount          int     `json:"gap_count"`           // Number of gaps
	TotalGapMinutes   int     `json:"total_gap_minutes"`   // Total gap duration
	CompletionPercent float64 `json:"completion_percent"`  // Percentage of day tracked
	AverageGapMinutes float64 `json:"average_gap_minutes"` // Average gap duration
}

// CategoryLegend represents category metadata for color coding
type CategoryLegend struct {
	CategoryID   uuid.UUID `json:"category_id"`
	CategoryName string    `json:"category_name"`
	Color        *string   `json:"color,omitempty"`
	Icon         *string   `json:"icon,omitempty"`
	Count        int       `json:"count"` // Number of check-ins in this category
}

// DayView represents a single day's check-ins
type DayView struct {
	Date     string             `json:"date"`
	Checkins []*checkin.Checkin `json:"checkins"`
	Total    int                `json:"total"`
}

// EnhancedDayView represents an enhanced daily timeline view with blocks and analytics
type EnhancedDayView struct {
	Date             string            `json:"date"`              // YYYY-MM-DD
	Timezone         string            `json:"timezone"`          // IANA timezone
	BlockGranularity int               `json:"block_granularity"` // Block size in minutes
	Blocks           []*TimeBlock      `json:"blocks"`            // Time blocks with check-ins
	Gaps             []*Gap            `json:"gaps"`              // Gaps between check-ins
	Summary          *DailySummary     `json:"summary"`           // Daily statistics
	CategoryLegend   []*CategoryLegend `json:"category_legend"`   // Category metadata
	PreviousDay      string            `json:"previous_day"`      // YYYY-MM-DD for navigation
	NextDay          string            `json:"next_day"`          // YYYY-MM-DD for navigation
	GeneratedAt      time.Time         `json:"generated_at"`      // Timestamp of generation
}

// Service handles timeline business logic
type Service struct {
	checkinRepo  *checkin.Repository
	categoryRepo *category.Repository
	cache        *CacheService
	logger       *zap.Logger
}

// NewService creates a new timeline service
func NewService(checkinRepo *checkin.Repository, categoryRepo *category.Repository, cache *CacheService, logger *zap.Logger) *Service {
	return &Service{
		checkinRepo:  checkinRepo,
		categoryRepo: categoryRepo,
		cache:        cache,
		logger:       logger,
	}
}

// WeekView represents a week's check-ins grouped by day
type WeekView struct {
	StartDate string     `json:"start_date"`
	EndDate   string     `json:"end_date"`
	Days      []*DayView `json:"days"`
	Total     int        `json:"total"`
}

// MonthView represents a month's check-ins grouped by day
type MonthView struct {
	Year  int        `json:"year"`
	Month int        `json:"month"`
	Days  []*DayView `json:"days"`
	Total int        `json:"total"`
}

// GetDailyEnhanced retrieves an enhanced daily timeline with time blocks and analytics
// Supports optional pagination and caching
func (s *Service) GetDailyEnhanced(ctx context.Context, userID uuid.UUID, date time.Time, blockGranularity int, timezone string) (*EnhancedDayView, error) {
	return s.GetDailyEnhancedPaginated(ctx, userID, date, blockGranularity, timezone, "", 0)
}

// GetDailyEnhancedPaginated retrieves an enhanced daily timeline with pagination support
func (s *Service) GetDailyEnhancedPaginated(ctx context.Context, userID uuid.UUID, date time.Time, blockGranularity int, timezone, cursor string, limit int) (*EnhancedDayView, error) {
	// Validate block granularity
	granularity, err := ValidateBlockGranularity(blockGranularity)
	if err != nil {
		s.logger.Warn("Invalid block granularity, using default 30", zap.Error(err))
		granularity = Block30Min
	}

	// Load timezone
	loc := time.UTC
	if timezone != "" {
		var err error
		loc, err = time.LoadLocation(timezone)
		if err != nil {
			s.logger.Warn("Invalid timezone, using UTC", zap.String("timezone", timezone), zap.Error(err))
			loc = time.UTC
		}
	}

	// Get start and end of day in the specified timezone
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, loc)
	dateStr := startOfDay.Format("2006-01-02")

	// Try to get from cache if caching is enabled
	if s.cache != nil && cursor == "" && limit == 0 {
		cacheKey := s.cache.CacheKey(userID, dateStr, timezone, int(granularity), limit, cursor)
		cached, err := s.cache.Get(ctx, cacheKey)
		if err == nil && cached != nil {
			s.logger.Debug("Timeline cache hit",
				zap.String("user_id", userID.String()),
				zap.String("date", dateStr))
			return cached, nil
		}
	}

	endOfDay := startOfDay.Add(24 * time.Hour)

	// Convert to UTC for database queries
	startOfDayUTC := startOfDay.UTC()
	endOfDayUTC := endOfDay.UTC()

	// Get check-ins for the day
	checkins, _, err := s.checkinRepo.List(ctx, checkin.ListOptions{
		UserID:    userID,
		StartDate: &startOfDayUTC,
		EndDate:   &endOfDayUTC,
		Limit:     1000, // High limit for a single day
		Offset:    0,
	})
	if err != nil {
		s.logger.Error("Failed to get daily timeline", zap.Error(err))
		return nil, fmt.Errorf("failed to get daily timeline: %w", err)
	}

	// Get category information for all check-ins
	categoryMap, err := s.getCategoryMap(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to get category map", zap.Error(err))
		return nil, fmt.Errorf("failed to get category map: %w", err)
	}

	// Enrich check-ins with category metadata
	enrichedCheckins := s.enrichCheckinsWithCategories(checkins, categoryMap)

	// Generate time blocks
	blocks := s.generateTimeBlocks(startOfDay, endOfDay, int(granularity), enrichedCheckins, loc)

	// Apply pagination if requested
	if limit > 0 {
		blocks, _, err = PaginateBlocks(blocks, cursor, limit)
		if err != nil {
			s.logger.Error("Failed to paginate blocks", zap.Error(err))
			return nil, fmt.Errorf("failed to paginate blocks: %w", err)
		}
	}

	// Detect gaps between check-ins
	gaps := s.detectGaps(enrichedCheckins, loc)

	// Calculate daily summary
	summary := s.calculateDailySummary(startOfDay, enrichedCheckins, gaps)

	// Build category legend
	categoryLegend := s.buildCategoryLegend(enrichedCheckins, categoryMap)

	// Navigation
	prevDay := startOfDay.AddDate(0, 0, -1).Format("2006-01-02")
	nextDay := startOfDay.AddDate(0, 0, 1).Format("2006-01-02")

	view := &EnhancedDayView{
		Date:             dateStr,
		Timezone:         loc.String(),
		BlockGranularity: int(granularity),
		Blocks:           blocks,
		Gaps:             gaps,
		Summary:          summary,
		CategoryLegend:   categoryLegend,
		PreviousDay:      prevDay,
		NextDay:          nextDay,
		GeneratedAt:      time.Now().UTC(),
	}

	// Cache the result if caching is enabled and not paginated
	if s.cache != nil && cursor == "" && limit == 0 {
		cacheKey := s.cache.CacheKey(userID, dateStr, timezone, int(granularity), limit, cursor)
		ttl := CalculateTTL(startOfDay)

		if err := s.cache.Set(ctx, cacheKey, view, ttl); err != nil {
			s.logger.Warn("Failed to cache timeline", zap.Error(err))
			// Don't fail the request if caching fails
		}

		// Set ETag for conditional requests
		etag := ComputeETag(view)
		if err := s.cache.SetETag(ctx, cacheKey, etag, ttl); err != nil {
			s.logger.Warn("Failed to cache ETag", zap.Error(err))
		}

		// Set last modified timestamp
		if err := s.cache.SetLastModified(ctx, cacheKey, view.GeneratedAt, ttl); err != nil {
			s.logger.Warn("Failed to cache last modified", zap.Error(err))
		}
	}

	return view, nil
}

// InvalidateCache invalidates cached timeline data for a user
func (s *Service) InvalidateCache(ctx context.Context, userID uuid.UUID) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.InvalidateUserTimeline(ctx, userID)
}

// InvalidateDateCache invalidates cached timeline data for a specific date
func (s *Service) InvalidateDateCache(ctx context.Context, userID uuid.UUID, date string) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.InvalidateDate(ctx, userID, date)
}

// GetDaily retrieves check-ins for a specific day
func (s *Service) GetDaily(ctx context.Context, userID uuid.UUID, date time.Time) (*DayView, error) {
	// Get start and end of day
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	checkins, total, err := s.checkinRepo.List(ctx, checkin.ListOptions{
		UserID:    userID,
		StartDate: &startOfDay,
		EndDate:   &endOfDay,
		Limit:     1000, // High limit for a single day
		Offset:    0,
	})
	if err != nil {
		s.logger.Error("Failed to get daily timeline", zap.Error(err))
		return nil, fmt.Errorf("failed to get daily timeline")
	}

	return &DayView{
		Date:     startOfDay.Format("2006-01-02"),
		Checkins: checkins,
		Total:    total,
	}, nil
}

// GetWeekly retrieves check-ins for a week, grouped by day
func (s *Service) GetWeekly(ctx context.Context, userID uuid.UUID, date time.Time) (*WeekView, error) {
	// Get start of week (Monday)
	weekday := int(date.Weekday())
	if weekday == 0 {
		weekday = 7 // Sunday is 7, not 0
	}
	startOfWeek := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location()).AddDate(0, 0, -(weekday - 1))
	endOfWeek := startOfWeek.AddDate(0, 0, 7)

	// Get all check-ins for the week
	checkins, total, err := s.checkinRepo.List(ctx, checkin.ListOptions{
		UserID:    userID,
		StartDate: &startOfWeek,
		EndDate:   &endOfWeek,
		Limit:     1000,
		Offset:    0,
	})
	if err != nil {
		s.logger.Error("Failed to get weekly timeline", zap.Error(err))
		return nil, fmt.Errorf("failed to get weekly timeline")
	}

	// Group by day
	dayMap := make(map[string][]*checkin.Checkin)
	for _, c := range checkins {
		dayKey := c.CheckinTime.Format("2006-01-02")
		dayMap[dayKey] = append(dayMap[dayKey], c)
	}

	// Create day views
	days := make([]*DayView, 0, 7)
	currentDay := startOfWeek
	for i := 0; i < 7; i++ {
		dayKey := currentDay.Format("2006-01-02")
		dayCheckins := dayMap[dayKey]
		if dayCheckins == nil {
			dayCheckins = []*checkin.Checkin{}
		}

		days = append(days, &DayView{
			Date:     dayKey,
			Checkins: dayCheckins,
			Total:    len(dayCheckins),
		})

		currentDay = currentDay.AddDate(0, 0, 1)
	}

	return &WeekView{
		StartDate: startOfWeek.Format("2006-01-02"),
		EndDate:   endOfWeek.AddDate(0, 0, -1).Format("2006-01-02"),
		Days:      days,
		Total:     total,
	}, nil
}

// GetMonthly retrieves check-ins for a month, grouped by day
func (s *Service) GetMonthly(ctx context.Context, userID uuid.UUID, date time.Time) (*MonthView, error) {
	// Get start and end of month
	startOfMonth := time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, date.Location())
	endOfMonth := startOfMonth.AddDate(0, 1, 0)

	// Get all check-ins for the month
	checkins, total, err := s.checkinRepo.List(ctx, checkin.ListOptions{
		UserID:    userID,
		StartDate: &startOfMonth,
		EndDate:   &endOfMonth,
		Limit:     5000, // High limit for a month
		Offset:    0,
	})
	if err != nil {
		s.logger.Error("Failed to get monthly timeline", zap.Error(err))
		return nil, fmt.Errorf("failed to get monthly timeline")
	}

	// Group by day
	dayMap := make(map[string][]*checkin.Checkin)
	for _, c := range checkins {
		dayKey := c.CheckinTime.Format("2006-01-02")
		dayMap[dayKey] = append(dayMap[dayKey], c)
	}

	// Create day views for all days in month
	days := make([]*DayView, 0)
	currentDay := startOfMonth
	for currentDay.Before(endOfMonth) {
		dayKey := currentDay.Format("2006-01-02")
		dayCheckins := dayMap[dayKey]
		if dayCheckins == nil {
			dayCheckins = []*checkin.Checkin{}
		}

		days = append(days, &DayView{
			Date:     dayKey,
			Checkins: dayCheckins,
			Total:    len(dayCheckins),
		})

		currentDay = currentDay.AddDate(0, 0, 1)
	}

	return &MonthView{
		Year:  startOfMonth.Year(),
		Month: int(startOfMonth.Month()),
		Days:  days,
		Total: total,
	}, nil
}

// Helper methods for enhanced timeline

// getCategoryMap retrieves all categories for a user and returns them in a map
func (s *Service) getCategoryMap(ctx context.Context, userID uuid.UUID) (map[uuid.UUID]*category.Category, error) {
	categories, err := s.categoryRepo.List(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list categories: %w", err)
	}

	categoryMap := make(map[uuid.UUID]*category.Category)
	for _, cat := range categories {
		categoryMap[cat.ID] = cat
	}

	return categoryMap, nil
}

// enrichCheckinsWithCategories adds category metadata to check-ins
func (s *Service) enrichCheckinsWithCategories(checkins []*checkin.Checkin, categoryMap map[uuid.UUID]*category.Category) []*CheckinWithMeta {
	enriched := make([]*CheckinWithMeta, 0, len(checkins))

	for _, c := range checkins {
		meta := &CheckinWithMeta{
			Checkin: c,
		}

		if c.CategoryID != nil {
			if cat, ok := categoryMap[*c.CategoryID]; ok {
				meta.CategoryName = &cat.Name
				meta.CategoryColor = cat.Color
				meta.CategoryIcon = cat.Icon
			}
		}

		enriched = append(enriched, meta)
	}

	return enriched
}

// generateTimeBlocks creates time blocks and assigns check-ins to them
func (s *Service) generateTimeBlocks(startOfDay, endOfDay time.Time, blockMinutes int, checkins []*CheckinWithMeta, loc *time.Location) []*TimeBlock {
	blocks := make([]*TimeBlock, 0)

	// Generate all blocks for the day
	currentTime := startOfDay
	for currentTime.Before(endOfDay) {
		blockEnd := currentTime.Add(time.Duration(blockMinutes) * time.Minute)
		if blockEnd.After(endOfDay) {
			blockEnd = endOfDay
		}

		block := &TimeBlock{
			StartTime:    currentTime,
			EndTime:      blockEnd,
			DurationMins: blockMinutes,
			Checkins:     make([]*CheckinWithMeta, 0),
			IsEmpty:      true,
			IsGap:        false,
		}

		// Assign check-ins to this block
		for _, c := range checkins {
			checkinTime := c.CheckinTime.In(loc)
			// Check if check-in falls within this block
			if (checkinTime.Equal(currentTime) || checkinTime.After(currentTime)) && checkinTime.Before(blockEnd) {
				block.Checkins = append(block.Checkins, c)
				block.IsEmpty = false
			}
		}

		blocks = append(blocks, block)
		currentTime = blockEnd
	}

	return blocks
}

// detectGaps finds gaps between consecutive check-ins
func (s *Service) detectGaps(checkins []*CheckinWithMeta, loc *time.Location) []*Gap {
	if len(checkins) < 2 {
		return []*Gap{}
	}

	// Sort check-ins by time
	sortedCheckins := make([]*CheckinWithMeta, len(checkins))
	copy(sortedCheckins, checkins)
	sort.Slice(sortedCheckins, func(i, j int) bool {
		return sortedCheckins[i].CheckinTime.Before(sortedCheckins[j].CheckinTime)
	})

	gaps := make([]*Gap, 0)

	for i := 0; i < len(sortedCheckins)-1; i++ {
		current := sortedCheckins[i]
		next := sortedCheckins[i+1]

		// Calculate end time of current check-in
		currentEnd := current.CheckinTime.Add(time.Duration(current.DurationMinutes) * time.Minute)

		// Check if there's a gap
		if next.CheckinTime.After(currentEnd) {
			gapDuration := int(next.CheckinTime.Sub(currentEnd).Minutes())

			// Only record gaps of at least 1 minute
			if gapDuration > 0 {
				gaps = append(gaps, &Gap{
					StartTime:    currentEnd.In(loc),
					EndTime:      next.CheckinTime.In(loc),
					DurationMins: gapDuration,
				})
			}
		}
	}

	return gaps
}

// calculateDailySummary computes aggregate statistics for the day
func (s *Service) calculateDailySummary(date time.Time, checkins []*CheckinWithMeta, gaps []*Gap) *DailySummary {
	summary := &DailySummary{
		Date:          date.Format("2006-01-02"),
		TotalCheckins: len(checkins),
		GapCount:      len(gaps),
	}

	if len(checkins) == 0 {
		return summary
	}

	// Sort check-ins by time
	sortedCheckins := make([]*CheckinWithMeta, len(checkins))
	copy(sortedCheckins, checkins)
	sort.Slice(sortedCheckins, func(i, j int) bool {
		return sortedCheckins[i].CheckinTime.Before(sortedCheckins[j].CheckinTime)
	})

	// First and last check-in times
	firstTime := sortedCheckins[0].CheckinTime.Format(time.RFC3339)
	lastTime := sortedCheckins[len(sortedCheckins)-1].CheckinTime.Format(time.RFC3339)
	summary.FirstCheckinTime = &firstTime
	summary.LastCheckinTime = &lastTime

	// Calculate total minutes and active hours
	totalMinutes := 0
	activeMinutesMap := make(map[int]bool) // Track unique hours with activity

	for _, c := range checkins {
		totalMinutes += c.DurationMinutes

		// Track active hours
		hour := c.CheckinTime.Hour()
		activeMinutesMap[hour] = true
	}

	summary.TotalMinutes = totalMinutes
	summary.ActiveHours = float64(len(activeMinutesMap))

	// Calculate completion percentage (based on 24 hours = 1440 minutes)
	summary.CompletionPercent = (float64(totalMinutes) / 1440.0) * 100.0
	if summary.CompletionPercent > 100.0 {
		summary.CompletionPercent = 100.0
	}

	// Calculate gap statistics
	totalGapMinutes := 0
	for _, gap := range gaps {
		totalGapMinutes += gap.DurationMins
	}
	summary.TotalGapMinutes = totalGapMinutes

	if len(gaps) > 0 {
		summary.AverageGapMinutes = float64(totalGapMinutes) / float64(len(gaps))
	}

	return summary
}

// buildCategoryLegend creates a legend of categories with counts
func (s *Service) buildCategoryLegend(checkins []*CheckinWithMeta, categoryMap map[uuid.UUID]*category.Category) []*CategoryLegend {
	// Count check-ins per category
	categoryCounts := make(map[uuid.UUID]int)

	for _, c := range checkins {
		if c.CategoryID != nil {
			categoryCounts[*c.CategoryID]++
		}
	}

	// Build legend
	legend := make([]*CategoryLegend, 0, len(categoryCounts))

	for categoryID, count := range categoryCounts {
		if cat, ok := categoryMap[categoryID]; ok {
			legend = append(legend, &CategoryLegend{
				CategoryID:   cat.ID,
				CategoryName: cat.Name,
				Color:        cat.Color,
				Icon:         cat.Icon,
				Count:        count,
			})
		}
	}

	// Sort by count descending
	sort.Slice(legend, func(i, j int) bool {
		return legend[i].Count > legend[j].Count
	})

	return legend
}
