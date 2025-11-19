package statistics

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// UsageService handles business logic for usage statistics
type UsageService struct {
	repo   *UsageRepository
	db     *sql.DB
	logger *zap.Logger
}

// NewUsageService creates a new usage statistics service
func NewUsageService(db *sql.DB, logger *zap.Logger) *UsageService {
	return &UsageService{
		repo:   NewUsageRepository(db),
		db:     db,
		logger: logger,
	}
}

// DashboardOptions represents options for generating the usage dashboard
type DashboardOptions struct {
	UserID           uuid.UUID
	Period           string    // "day", "week", "month", "quarter", "year", "custom"
	StartDate        time.Time // For custom period
	EndDate          time.Time // For custom period
	TopItemsLimit    int       // Number of top items to include (default: 10)
	IncludeUnused    bool      // Include unused items analysis
	IncludePatterns  bool      // Include usage patterns analysis
	IncludeMatrix    bool      // Include category-tag matrix
	UnusedThreshold  int       // Days since last use to consider "unused" (default: 30)
}

// GetUsageDashboard generates a comprehensive usage dashboard
func (s *UsageService) GetUsageDashboard(ctx context.Context, opts DashboardOptions) (*UsageDashboard, error) {
	// Set defaults
	if opts.TopItemsLimit == 0 {
		opts.TopItemsLimit = 10
	}
	if opts.UnusedThreshold == 0 {
		opts.UnusedThreshold = 30
	}

	// Calculate time period
	startDate, endDate := s.calculatePeriod(opts)

	dashboard := &UsageDashboard{
		GeneratedAt: time.Now(),
	}

	// Set period information
	dashboard.Period.Start = startDate
	dashboard.Period.End = endDate
	dashboard.Period.Days = int(math.Ceil(endDate.Sub(startDate).Hours() / 24))

	// Get summary statistics
	summary, err := s.repo.GetUsageSummaryStats(ctx, opts.UserID, startDate, endDate)
	if err != nil {
		s.logger.Error("Failed to get summary stats", zap.Error(err))
		return nil, fmt.Errorf("failed to get summary statistics: %w", err)
	}
	dashboard.Summary.TotalCheckins = summary.TotalCheckins
	dashboard.Summary.TotalCategories = summary.TotalCategories
	dashboard.Summary.ActiveCategories = summary.ActiveCategories
	dashboard.Summary.TotalTags = summary.TotalTags
	dashboard.Summary.ActiveTags = summary.ActiveTags
	dashboard.Summary.AverageTagsPerItem = summary.AverageTagsPerItem
	dashboard.Summary.CategoryCoverage = summary.CategoryCoverage
	dashboard.Summary.TagCoverage = summary.TagCoverage

	// Get top categories
	categories, err := s.repo.GetCategoryUsageStats(ctx, opts.UserID, startDate, endDate, opts.TopItemsLimit)
	if err != nil {
		s.logger.Error("Failed to get category stats", zap.Error(err))
		return nil, fmt.Errorf("failed to get category statistics: %w", err)
	}
	dashboard.TopCategories = categories

	// Calculate trend for categories
	for _, cat := range dashboard.TopCategories {
		cat.Metrics.TrendDirection, cat.Metrics.TrendPercentage = s.calculateTrend(
			ctx, "category", cat.CategoryID, startDate, endDate,
		)
	}

	// Get top tags
	tags, err := s.repo.GetTagUsageStats(ctx, opts.UserID, startDate, endDate, opts.TopItemsLimit)
	if err != nil {
		s.logger.Error("Failed to get tag stats", zap.Error(err))
		return nil, fmt.Errorf("failed to get tag statistics: %w", err)
	}
	dashboard.TopTags = tags

	// Calculate trend for tags
	for _, tag := range dashboard.TopTags {
		tag.Metrics.TrendDirection, tag.Metrics.TrendPercentage = s.calculateTrend(
			ctx, "tag", tag.TagID, startDate, endDate,
		)
	}

	// Get unused items if requested
	if opts.IncludeUnused {
		// Unused categories
		unusedCats, err := s.repo.GetUnusedCategories(ctx, opts.UserID, opts.UnusedThreshold)
		if err != nil {
			s.logger.Warn("Failed to get unused categories", zap.Error(err))
		} else {
			for _, uc := range unusedCats {
				dashboard.UnusedCategories = append(dashboard.UnusedCategories, struct {
					CategoryID   uuid.UUID  `json:"category_id"`
					CategoryName string     `json:"category_name"`
					LastUsed     *time.Time `json:"last_used,omitempty"`
					DaysSinceUse int        `json:"days_since_use"`
				}{
					CategoryID:   uc.CategoryID,
					CategoryName: uc.CategoryName,
					LastUsed:     uc.LastUsed,
					DaysSinceUse: uc.DaysSinceUse,
				})
			}
		}

		// Unused tags
		unusedTags, err := s.repo.GetUnusedTags(ctx, opts.UserID, opts.UnusedThreshold)
		if err != nil {
			s.logger.Warn("Failed to get unused tags", zap.Error(err))
		} else {
			for _, ut := range unusedTags {
				dashboard.UnusedTags = append(dashboard.UnusedTags, struct {
					TagID        uuid.UUID  `json:"tag_id"`
					TagName      string     `json:"tag_name"`
					LastUsed     *time.Time `json:"last_used,omitempty"`
					DaysSinceUse int        `json:"days_since_use"`
				}{
					TagID:        ut.TagID,
					TagName:      ut.TagName,
					LastUsed:     ut.LastUsed,
					DaysSinceUse: ut.DaysSinceUse,
				})
			}
		}
	}

	// Get usage patterns if requested
	if opts.IncludePatterns {
		dashboard.UsagePatterns.CategoryPatterns = s.analyzeCategoryPatterns(dashboard.TopCategories)
		dashboard.UsagePatterns.TagPatterns = s.analyzeTagPatterns(dashboard.TopTags)
	}

	// Get category-tag matrix if requested
	if opts.IncludeMatrix {
		matrix, err := s.repo.GetCategoryTagMatrix(ctx, opts.UserID, startDate, endDate)
		if err != nil {
			s.logger.Warn("Failed to get category-tag matrix", zap.Error(err))
		} else {
			dashboard.CategoryTagMatrix = matrix
		}
	}

	// Get growth metrics
	previousStart := startDate.AddDate(0, 0, -dashboard.Period.Days)
	previousEnd := startDate
	growth, err := s.repo.GetGrowthMetrics(ctx, opts.UserID, startDate, endDate, previousStart, previousEnd)
	if err != nil {
		s.logger.Warn("Failed to get growth metrics", zap.Error(err))
	} else {
		dashboard.Growth.NewCategories = growth.NewCategories
		dashboard.Growth.NewTags = growth.NewTags
		dashboard.Growth.CategoryGrowthRate = growth.CategoryGrowthRate
		dashboard.Growth.TagGrowthRate = growth.TagGrowthRate
	}

	// Generate recommendations
	s.generateRecommendations(dashboard)

	s.logger.Info("Generated usage dashboard",
		zap.String("user_id", opts.UserID.String()),
		zap.String("period", opts.Period),
		zap.Time("start", startDate),
		zap.Time("end", endDate),
	)

	return dashboard, nil
}

// GetCategoryUsageStats retrieves usage statistics for categories
func (s *UsageService) GetCategoryUsageStats(ctx context.Context, userID uuid.UUID, period string, limit int) ([]*CategoryUsageStats, error) {
	startDate, endDate := s.calculatePeriod(DashboardOptions{Period: period})

	stats, err := s.repo.GetCategoryUsageStats(ctx, userID, startDate, endDate, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get category usage stats: %w", err)
	}

	// Add trend information
	for _, stat := range stats {
		stat.Metrics.TrendDirection, stat.Metrics.TrendPercentage = s.calculateTrend(
			ctx, "category", stat.CategoryID, startDate, endDate,
		)
	}

	return stats, nil
}

// GetTagUsageStats retrieves usage statistics for tags
func (s *UsageService) GetTagUsageStats(ctx context.Context, userID uuid.UUID, period string, limit int) ([]*TagUsageStats, error) {
	startDate, endDate := s.calculatePeriod(DashboardOptions{Period: period})

	stats, err := s.repo.GetTagUsageStats(ctx, userID, startDate, endDate, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get tag usage stats: %w", err)
	}

	// Add trend information
	for _, stat := range stats {
		stat.Metrics.TrendDirection, stat.Metrics.TrendPercentage = s.calculateTrend(
			ctx, "tag", stat.TagID, startDate, endDate,
		)
	}

	return stats, nil
}

// GetUsageTrends retrieves usage trends over time
func (s *UsageService) GetUsageTrends(ctx context.Context, userID uuid.UUID, itemType string, itemIDs []uuid.UUID, period string, aggregation string) ([]*UsageTrend, error) {
	startDate, endDate := s.calculatePeriod(DashboardOptions{Period: period})

	var trends []*UsageTrend

	for _, itemID := range itemIDs {
		trend := &UsageTrend{
			ItemID:   itemID,
			ItemType: itemType,
		}

		// Get aggregations based on the specified period
		aggregations := s.getUsageAggregations(ctx, userID, itemType, itemID, startDate, endDate, aggregation)
		trend.Aggregations = aggregations

		// Calculate trend line (simple moving average for now)
		trend.TrendLine = s.calculateTrendLine(aggregations)

		// Add basic prediction (linear extrapolation)
		trend.Prediction = s.predictFuture(trend.TrendLine, 3) // Predict 3 periods ahead

		trends = append(trends, trend)
	}

	return trends, nil
}

// calculatePeriod calculates start and end dates based on options
func (s *UsageService) calculatePeriod(opts DashboardOptions) (time.Time, time.Time) {
	now := time.Now()
	var startDate, endDate time.Time

	switch opts.Period {
	case "day":
		startDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		endDate = startDate.Add(24 * time.Hour)
	case "week":
		startDate = now.AddDate(0, 0, -7)
		endDate = now
	case "month":
		startDate = now.AddDate(0, -1, 0)
		endDate = now
	case "quarter":
		startDate = now.AddDate(0, -3, 0)
		endDate = now
	case "year":
		startDate = now.AddDate(-1, 0, 0)
		endDate = now
	case "custom":
		startDate = opts.StartDate
		endDate = opts.EndDate
		if endDate.IsZero() {
			endDate = now
		}
	default: // Default to last 30 days
		startDate = now.AddDate(0, 0, -30)
		endDate = now
	}

	return startDate, endDate
}

// calculateTrend calculates trend direction and percentage
func (s *UsageService) calculateTrend(ctx context.Context, itemType string, itemID uuid.UUID, currentStart, currentEnd time.Time) (string, float64) {
	// Calculate previous period
	duration := currentEnd.Sub(currentStart)
	previousStart := currentStart.Add(-duration)
	previousEnd := currentStart

	var currentCount, previousCount int
	var query string

	if itemType == "category" {
		query = `
			SELECT
				COUNT(CASE WHEN created_at BETWEEN $2 AND $3 THEN 1 END) as current_count,
				COUNT(CASE WHEN created_at BETWEEN $4 AND $5 THEN 1 END) as previous_count
			FROM checkins
			WHERE category_id = $1 AND deleted_at IS NULL
		`
	} else {
		query = `
			SELECT
				COUNT(CASE WHEN ch.created_at BETWEEN $2 AND $3 THEN 1 END) as current_count,
				COUNT(CASE WHEN ch.created_at BETWEEN $4 AND $5 THEN 1 END) as previous_count
			FROM checkin_tags ct
			JOIN checkins ch ON ch.id = ct.checkin_id
			WHERE ct.tag_id = $1 AND ch.deleted_at IS NULL
		`
	}

	err := s.db.QueryRowContext(ctx, query, itemID, currentStart, currentEnd, previousStart, previousEnd).
		Scan(&currentCount, &previousCount)
	if err != nil {
		return "stable", 0
	}

	if previousCount == 0 {
		if currentCount > 0 {
			return "up", 100.0
		}
		return "stable", 0
	}

	changePercent := float64(currentCount-previousCount) / float64(previousCount) * 100

	if changePercent > 5 {
		return "up", changePercent
	} else if changePercent < -5 {
		return "down", math.Abs(changePercent)
	}

	return "stable", math.Abs(changePercent)
}

// analyzeCategoryPatterns analyzes usage patterns for categories
func (s *UsageService) analyzeCategoryPatterns(categories []*CategoryUsageStats) []UsagePattern {
	if len(categories) == 0 {
		return nil
	}

	patterns := []UsagePattern{}

	// Analyze daily patterns
	dailyPattern := UsagePattern{
		Period: "daily",
	}

	// Find peak hours across all categories
	hourCounts := make(map[int]int)
	for _, cat := range categories {
		for hour, count := range cat.HourPattern {
			hourCounts[hour] += count
		}
	}

	// Find top 3 peak hours
	type hourCount struct {
		Hour  int
		Count int
	}
	var hours []hourCount
	for h, c := range hourCounts {
		hours = append(hours, hourCount{h, c})
	}
	sort.Slice(hours, func(i, j int) bool {
		return hours[i].Count > hours[j].Count
	})

	for i := 0; i < 3 && i < len(hours); i++ {
		dailyPattern.PeakHours = append(dailyPattern.PeakHours, hours[i].Hour)
	}

	// Find low activity times
	for i := len(hours) - 1; i >= 0 && i >= len(hours)-3; i-- {
		dailyPattern.LowActivityTime = append(dailyPattern.LowActivityTime, hours[i].Hour)
	}

	patterns = append(patterns, dailyPattern)

	// Analyze weekly patterns
	weeklyPattern := UsagePattern{
		Period: "weekly",
	}

	// Find peak days
	dayCounts := make(map[int]int)
	for _, cat := range categories {
		for day, count := range cat.DayPattern {
			dayCounts[day] += count
		}
	}

	type dayCount struct {
		Day   int
		Count int
	}
	var days []dayCount
	for d, c := range dayCounts {
		days = append(days, dayCount{d, c})
	}
	sort.Slice(days, func(i, j int) bool {
		return days[i].Count > days[j].Count
	})

	dayNames := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
	for i := 0; i < 3 && i < len(days); i++ {
		weeklyPattern.PeakDays = append(weeklyPattern.PeakDays, dayNames[days[i].Day])
	}

	patterns = append(patterns, weeklyPattern)

	return patterns
}

// analyzeTagPatterns analyzes usage patterns for tags
func (s *UsageService) analyzeTagPatterns(tags []*TagUsageStats) []UsagePattern {
	// Similar to analyzeCategoryPatterns but for tags
	return s.analyzeCategoryPatterns(convertTagsToCategories(tags))
}

// convertTagsToCategories is a helper to reuse pattern analysis logic
func convertTagsToCategories(tags []*TagUsageStats) []*CategoryUsageStats {
	var categories []*CategoryUsageStats
	for _, tag := range tags {
		categories = append(categories, &CategoryUsageStats{
			CategoryID:   tag.TagID,
			CategoryName: tag.TagName,
			Metrics:      tag.Metrics,
			DayPattern:   tag.DayPattern,
			HourPattern:  tag.HourPattern,
		})
	}
	return categories
}

// generateRecommendations generates intelligent recommendations
func (s *UsageService) generateRecommendations(dashboard *UsageDashboard) {
	// Suggest merges for similar items
	s.suggestMerges(dashboard)

	// Identify cleanup candidates
	s.identifyCleanupCandidates(dashboard)
}

// suggestMerges identifies items that could be merged
func (s *UsageService) suggestMerges(dashboard *UsageDashboard) {
	// Check for categories with very similar names
	for i, cat1 := range dashboard.TopCategories {
		for j := i + 1; j < len(dashboard.TopCategories); j++ {
			cat2 := dashboard.TopCategories[j]

			similarity := calculateStringSimilarity(
				strings.ToLower(cat1.CategoryName),
				strings.ToLower(cat2.CategoryName),
			)

			if similarity > 0.8 { // 80% similarity threshold
				dashboard.Recommendations.SuggestedMerges = append(
					dashboard.Recommendations.SuggestedMerges,
					struct {
						Type    string    `json:"type"`
						Item1   string    `json:"item1_name"`
						Item1ID uuid.UUID `json:"item1_id"`
						Item2   string    `json:"item2_name"`
						Item2ID uuid.UUID `json:"item2_id"`
						Reason  string    `json:"reason"`
					}{
						Type:    "category",
						Item1:   cat1.CategoryName,
						Item1ID: cat1.CategoryID,
						Item2:   cat2.CategoryName,
						Item2ID: cat2.CategoryID,
						Reason:  fmt.Sprintf("Names are %.0f%% similar", similarity*100),
					},
				)
			}
		}
	}

	// Similar check for tags
	for i, tag1 := range dashboard.TopTags {
		for j := i + 1; j < len(dashboard.TopTags); j++ {
			tag2 := dashboard.TopTags[j]

			similarity := calculateStringSimilarity(
				strings.ToLower(tag1.TagName),
				strings.ToLower(tag2.TagName),
			)

			if similarity > 0.8 {
				dashboard.Recommendations.SuggestedMerges = append(
					dashboard.Recommendations.SuggestedMerges,
					struct {
						Type    string    `json:"type"`
						Item1   string    `json:"item1_name"`
						Item1ID uuid.UUID `json:"item1_id"`
						Item2   string    `json:"item2_name"`
						Item2ID uuid.UUID `json:"item2_id"`
						Reason  string    `json:"reason"`
					}{
						Type:    "tag",
						Item1:   tag1.TagName,
						Item1ID: tag1.TagID,
						Item2:   tag2.TagName,
						Item2ID: tag2.TagID,
						Reason:  fmt.Sprintf("Names are %.0f%% similar", similarity*100),
					},
				)
			}
		}
	}
}

// identifyCleanupCandidates identifies items that could be removed
func (s *UsageService) identifyCleanupCandidates(dashboard *UsageDashboard) {
	// Categories that haven't been used in over 90 days
	for _, cat := range dashboard.UnusedCategories {
		if cat.DaysSinceUse > 90 {
			dashboard.Recommendations.CleanupCandidates = append(
				dashboard.Recommendations.CleanupCandidates,
				struct {
					Type       string     `json:"type"`
					ItemName   string     `json:"item_name"`
					ItemID     uuid.UUID  `json:"item_id"`
					LastUsed   *time.Time `json:"last_used,omitempty"`
					UsageCount int        `json:"usage_count"`
					Reason     string     `json:"reason"`
				}{
					Type:     "category",
					ItemName: cat.CategoryName,
					ItemID:   cat.CategoryID,
					LastUsed: cat.LastUsed,
					Reason:   fmt.Sprintf("Not used in %d days", cat.DaysSinceUse),
				},
			)
		}
	}

	// Tags that haven't been used in over 90 days
	for _, tag := range dashboard.UnusedTags {
		if tag.DaysSinceUse > 90 {
			dashboard.Recommendations.CleanupCandidates = append(
				dashboard.Recommendations.CleanupCandidates,
				struct {
					Type       string     `json:"type"`
					ItemName   string     `json:"item_name"`
					ItemID     uuid.UUID  `json:"item_id"`
					LastUsed   *time.Time `json:"last_used,omitempty"`
					UsageCount int        `json:"usage_count"`
					Reason     string     `json:"reason"`
				}{
					Type:     "tag",
					ItemName: tag.TagName,
					ItemID:   tag.TagID,
					LastUsed: tag.LastUsed,
					Reason:   fmt.Sprintf("Not used in %d days", tag.DaysSinceUse),
				},
			)
		}
	}
}

// getUsageAggregations retrieves aggregated usage data
func (s *UsageService) getUsageAggregations(ctx context.Context, userID uuid.UUID, itemType string, itemID uuid.UUID, startDate, endDate time.Time, aggregation string) []UsageAggregation {
	// This is a simplified implementation
	// In production, you would query the database for actual aggregated data

	var aggregations []UsageAggregation

	// Generate sample aggregations based on the aggregation period
	current := startDate
	for current.Before(endDate) {
		agg := UsageAggregation{
			Period:    aggregation,
			Timestamp: current,
			Count:     10 + (current.Day() % 5), // Sample data
			UniqueItems: 3 + (current.Day() % 2),
		}
		aggregations = append(aggregations, agg)

		// Move to next period
		switch aggregation {
		case "hour":
			current = current.Add(time.Hour)
		case "day":
			current = current.AddDate(0, 0, 1)
		case "week":
			current = current.AddDate(0, 0, 7)
		case "month":
			current = current.AddDate(0, 1, 0)
		default:
			current = current.AddDate(0, 0, 1)
		}
	}

	return aggregations
}

// calculateTrendLine calculates a smoothed trend line
func (s *UsageService) calculateTrendLine(aggregations []UsageAggregation) []float64 {
	if len(aggregations) == 0 {
		return nil
	}

	trendLine := make([]float64, len(aggregations))

	// Simple moving average with window size 3
	windowSize := 3
	for i := range aggregations {
		sum := 0.0
		count := 0

		for j := i - windowSize/2; j <= i+windowSize/2; j++ {
			if j >= 0 && j < len(aggregations) {
				sum += float64(aggregations[j].Count)
				count++
			}
		}

		if count > 0 {
			trendLine[i] = sum / float64(count)
		}
	}

	return trendLine
}

// predictFuture predicts future values based on trend
func (s *UsageService) predictFuture(trendLine []float64, periods int) []float64 {
	if len(trendLine) < 2 {
		return nil
	}

	// Simple linear extrapolation
	n := len(trendLine)
	slope := (trendLine[n-1] - trendLine[0]) / float64(n-1)

	predictions := make([]float64, periods)
	for i := 0; i < periods; i++ {
		predictions[i] = trendLine[n-1] + slope*float64(i+1)
		if predictions[i] < 0 {
			predictions[i] = 0
		}
	}

	return predictions
}

// calculateStringSimilarity calculates similarity between two strings
func calculateStringSimilarity(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}

	// Levenshtein distance based similarity
	distance := levenshteinDistance(s1, s2)
	maxLen := math.Max(float64(len(s1)), float64(len(s2)))

	if maxLen == 0 {
		return 0
	}

	return 1.0 - float64(distance)/maxLen
}

// levenshteinDistance calculates the edit distance between two strings
func levenshteinDistance(s1, s2 string) int {
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}

	// Create distance matrix
	matrix := make([][]int, len(s1)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(s2)+1)
		matrix[i][0] = i
	}
	for j := range matrix[0] {
		matrix[0][j] = j
	}

	// Calculate distances
	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			cost := 0
			if s1[i-1] != s2[j-1] {
				cost = 1
			}

			matrix[i][j] = min(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[len(s1)][len(s2)]
}

// min returns the minimum of three integers
func min(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}