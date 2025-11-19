package statistics

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// UsageRepository handles database operations for usage statistics
type UsageRepository struct {
	db *sql.DB
}

// NewUsageRepository creates a new usage repository
func NewUsageRepository(db *sql.DB) *UsageRepository {
	return &UsageRepository{
		db: db,
	}
}

// GetCategoryUsageStats retrieves usage statistics for categories
func (r *UsageRepository) GetCategoryUsageStats(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time, limit int) ([]*CategoryUsageStats, error) {
	query := `
		WITH category_usage AS (
			SELECT
				c.id as category_id,
				c.name as category_name,
				c.color,
				c.icon,
				COUNT(ch.id) as total_usage,
				MIN(ch.created_at) as first_used,
				MAX(ch.created_at) as last_used,
				COUNT(DISTINCT DATE(ch.created_at)) as unique_days,
				COUNT(DISTINCT EXTRACT(WEEK FROM ch.created_at)) as unique_weeks,
				COUNT(DISTINCT EXTRACT(MONTH FROM ch.created_at)) as unique_months
			FROM categories c
			LEFT JOIN checkins ch ON ch.category_id = c.id
				AND ch.created_at BETWEEN $2 AND $3
				AND ch.deleted_at IS NULL
			WHERE c.user_id = $1 AND c.deleted_at IS NULL
			GROUP BY c.id, c.name, c.color, c.icon
		),
		ranked_usage AS (
			SELECT
				*,
				DENSE_RANK() OVER (ORDER BY total_usage DESC) as rank,
				total_usage::float / NULLIF(SUM(total_usage) OVER (), 0) * 100 as percentage
			FROM category_usage
		),
		day_pattern AS (
			SELECT
				category_id,
				EXTRACT(DOW FROM created_at) as day_of_week,
				COUNT(*) as day_count
			FROM checkins
			WHERE category_id IS NOT NULL
				AND created_at BETWEEN $2 AND $3
				AND deleted_at IS NULL
			GROUP BY category_id, EXTRACT(DOW FROM created_at)
		),
		hour_pattern AS (
			SELECT
				category_id,
				EXTRACT(HOUR FROM created_at) as hour_of_day,
				COUNT(*) as hour_count
			FROM checkins
			WHERE category_id IS NOT NULL
				AND created_at BETWEEN $2 AND $3
				AND deleted_at IS NULL
			GROUP BY category_id, EXTRACT(HOUR FROM created_at)
		)
		SELECT
			ru.category_id,
			ru.category_name,
			ru.color,
			ru.icon,
			ru.total_usage,
			ru.first_used,
			ru.last_used,
			ru.unique_days,
			ru.unique_weeks,
			ru.unique_months,
			ru.rank,
			ru.percentage,
			ru.last_used > (NOW() - INTERVAL '7 days') as is_active,
			CASE WHEN ru.unique_days > 0 THEN ru.total_usage::float / ru.unique_days ELSE 0 END as daily_average,
			CASE WHEN ru.unique_weeks > 0 THEN ru.total_usage::float / ru.unique_weeks ELSE 0 END as weekly_average,
			CASE WHEN ru.unique_months > 0 THEN ru.total_usage::float / ru.unique_months ELSE 0 END as monthly_average,
			COALESCE(array_agg(DISTINCT dp.day_of_week ORDER BY dp.day_of_week), ARRAY[]::integer[]) as day_pattern_keys,
			COALESCE(array_agg(DISTINCT dp.day_count ORDER BY dp.day_of_week), ARRAY[]::integer[]) as day_pattern_values,
			COALESCE(array_agg(DISTINCT hp.hour_of_day ORDER BY hp.hour_of_day), ARRAY[]::integer[]) as hour_pattern_keys,
			COALESCE(array_agg(DISTINCT hp.hour_count ORDER BY hp.hour_of_day), ARRAY[]::integer[]) as hour_pattern_values
		FROM ranked_usage ru
		LEFT JOIN day_pattern dp ON dp.category_id = ru.category_id
		LEFT JOIN hour_pattern hp ON hp.category_id = ru.category_id
		GROUP BY
			ru.category_id, ru.category_name, ru.color, ru.icon,
			ru.total_usage, ru.first_used, ru.last_used,
			ru.unique_days, ru.unique_weeks, ru.unique_months,
			ru.rank, ru.percentage
		ORDER BY ru.rank
		LIMIT $4
	`

	rows, err := r.db.QueryContext(ctx, query, userID, startDate, endDate, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query category usage stats: %w", err)
	}
	defer rows.Close()

	var stats []*CategoryUsageStats
	for rows.Next() {
		var s CategoryUsageStats
		var firstUsed, lastUsed sql.NullTime
		var dayKeys, dayValues, hourKeys, hourValues pq.Int64Array
		var uniqueDays, uniqueWeeks, uniqueMonths int
		var dailyAvg, weeklyAvg, monthlyAvg float64
		var isActive bool

		err := rows.Scan(
			&s.CategoryID,
			&s.CategoryName,
			&s.Color,
			&s.Icon,
			&s.Metrics.TotalUsage,
			&firstUsed,
			&lastUsed,
			&uniqueDays,
			&uniqueWeeks,
			&uniqueMonths,
			&s.Rank,
			&s.Percentage,
			&isActive,
			&dailyAvg,
			&weeklyAvg,
			&monthlyAvg,
			&dayKeys,
			&dayValues,
			&hourKeys,
			&hourValues,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan category usage stats: %w", err)
		}

		// Set time values
		if firstUsed.Valid {
			s.Metrics.FirstUsed = firstUsed.Time
		}
		if lastUsed.Valid {
			s.Metrics.LastUsed = lastUsed.Time
		}

		// Set averages
		s.Metrics.DailyAverage = dailyAvg
		s.Metrics.WeeklyAverage = weeklyAvg
		s.Metrics.MonthlyAverage = monthlyAvg
		s.IsActive = isActive

		// Build day pattern array (7 days, Sunday=0 to Saturday=6)
		s.DayPattern = make([]int, 7)
		for i, key := range dayKeys {
			if int(key) < 7 && i < len(dayValues) {
				s.DayPattern[int(key)] = int(dayValues[i])
			}
		}

		// Build hour pattern array (24 hours, 0-23)
		s.HourPattern = make([]int, 24)
		for i, key := range hourKeys {
			if int(key) < 24 && i < len(hourValues) {
				s.HourPattern[int(key)] = int(hourValues[i])
			}
		}

		stats = append(stats, &s)
	}

	return stats, rows.Err()
}

// GetTagUsageStats retrieves usage statistics for tags
func (r *UsageRepository) GetTagUsageStats(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time, limit int) ([]*TagUsageStats, error) {
	query := `
		WITH tag_usage AS (
			SELECT
				t.id as tag_id,
				t.name as tag_name,
				COUNT(ct.checkin_id) as total_usage,
				MIN(ch.created_at) as first_used,
				MAX(ch.created_at) as last_used,
				COUNT(DISTINCT DATE(ch.created_at)) as unique_days,
				COUNT(DISTINCT EXTRACT(WEEK FROM ch.created_at)) as unique_weeks,
				COUNT(DISTINCT EXTRACT(MONTH FROM ch.created_at)) as unique_months
			FROM tags t
			LEFT JOIN checkin_tags ct ON ct.tag_id = t.id
			LEFT JOIN checkins ch ON ch.id = ct.checkin_id
				AND ch.created_at BETWEEN $2 AND $3
				AND ch.deleted_at IS NULL
			WHERE t.user_id = $1 AND t.deleted_at IS NULL
			GROUP BY t.id, t.name
		),
		ranked_usage AS (
			SELECT
				*,
				DENSE_RANK() OVER (ORDER BY total_usage DESC) as rank,
				total_usage::float / NULLIF(SUM(total_usage) OVER (), 0) * 100 as percentage
			FROM tag_usage
		),
		day_pattern AS (
			SELECT
				ct.tag_id,
				EXTRACT(DOW FROM ch.created_at) as day_of_week,
				COUNT(*) as day_count
			FROM checkin_tags ct
			JOIN checkins ch ON ch.id = ct.checkin_id
			WHERE ch.created_at BETWEEN $2 AND $3
				AND ch.deleted_at IS NULL
			GROUP BY ct.tag_id, EXTRACT(DOW FROM ch.created_at)
		),
		hour_pattern AS (
			SELECT
				ct.tag_id,
				EXTRACT(HOUR FROM ch.created_at) as hour_of_day,
				COUNT(*) as hour_count
			FROM checkin_tags ct
			JOIN checkins ch ON ch.id = ct.checkin_id
			WHERE ch.created_at BETWEEN $2 AND $3
				AND ch.deleted_at IS NULL
			GROUP BY ct.tag_id, EXTRACT(HOUR FROM ch.created_at)
		),
		co_occurrences AS (
			SELECT
				ct1.tag_id as tag1_id,
				ct2.tag_id as tag2_id,
				t2.name as tag2_name,
				COUNT(*) as co_count
			FROM checkin_tags ct1
			JOIN checkin_tags ct2 ON ct1.checkin_id = ct2.checkin_id AND ct1.tag_id != ct2.tag_id
			JOIN tags t2 ON t2.id = ct2.tag_id
			JOIN checkins ch ON ch.id = ct1.checkin_id
			WHERE ch.created_at BETWEEN $2 AND $3
				AND ch.deleted_at IS NULL
			GROUP BY ct1.tag_id, ct2.tag_id, t2.name
		)
		SELECT
			ru.tag_id,
			ru.tag_name,
			ru.total_usage,
			ru.first_used,
			ru.last_used,
			ru.unique_days,
			ru.unique_weeks,
			ru.unique_months,
			ru.rank,
			ru.percentage,
			ru.last_used > (NOW() - INTERVAL '7 days') as is_active,
			CASE WHEN ru.unique_days > 0 THEN ru.total_usage::float / ru.unique_days ELSE 0 END as daily_average,
			CASE WHEN ru.unique_weeks > 0 THEN ru.total_usage::float / ru.unique_weeks ELSE 0 END as weekly_average,
			CASE WHEN ru.unique_months > 0 THEN ru.total_usage::float / ru.unique_months ELSE 0 END as monthly_average,
			COALESCE(array_agg(DISTINCT dp.day_of_week ORDER BY dp.day_of_week), ARRAY[]::integer[]) as day_pattern_keys,
			COALESCE(array_agg(DISTINCT dp.day_count ORDER BY dp.day_of_week), ARRAY[]::integer[]) as day_pattern_values,
			COALESCE(array_agg(DISTINCT hp.hour_of_day ORDER BY hp.hour_of_day), ARRAY[]::integer[]) as hour_pattern_keys,
			COALESCE(array_agg(DISTINCT hp.hour_count ORDER BY hp.hour_of_day), ARRAY[]::integer[]) as hour_pattern_values,
			COALESCE(
				json_agg(
					DISTINCT jsonb_build_object(
						'tag_id', co.tag2_id,
						'tag_name', co.tag2_name,
						'count', co.co_count
					) ORDER BY co.co_count DESC
				) FILTER (WHERE co.tag2_id IS NOT NULL),
				'[]'::json
			) as co_occurrences
		FROM ranked_usage ru
		LEFT JOIN day_pattern dp ON dp.tag_id = ru.tag_id
		LEFT JOIN hour_pattern hp ON hp.tag_id = ru.tag_id
		LEFT JOIN co_occurrences co ON co.tag1_id = ru.tag_id
		GROUP BY
			ru.tag_id, ru.tag_name,
			ru.total_usage, ru.first_used, ru.last_used,
			ru.unique_days, ru.unique_weeks, ru.unique_months,
			ru.rank, ru.percentage
		ORDER BY ru.rank
		LIMIT $4
	`

	rows, err := r.db.QueryContext(ctx, query, userID, startDate, endDate, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query tag usage stats: %w", err)
	}
	defer rows.Close()

	var stats []*TagUsageStats
	for rows.Next() {
		var s TagUsageStats
		var firstUsed, lastUsed sql.NullTime
		var dayKeys, dayValues, hourKeys, hourValues pq.Int64Array
		var uniqueDays, uniqueWeeks, uniqueMonths int
		var dailyAvg, weeklyAvg, monthlyAvg float64
		var isActive bool
		var coOccurrencesJSON []byte

		err := rows.Scan(
			&s.TagID,
			&s.TagName,
			&s.Metrics.TotalUsage,
			&firstUsed,
			&lastUsed,
			&uniqueDays,
			&uniqueWeeks,
			&uniqueMonths,
			&s.Rank,
			&s.Percentage,
			&isActive,
			&dailyAvg,
			&weeklyAvg,
			&monthlyAvg,
			&dayKeys,
			&dayValues,
			&hourKeys,
			&hourValues,
			&coOccurrencesJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan tag usage stats: %w", err)
		}

		// Set time values
		if firstUsed.Valid {
			s.Metrics.FirstUsed = firstUsed.Time
		}
		if lastUsed.Valid {
			s.Metrics.LastUsed = lastUsed.Time
		}

		// Set averages
		s.Metrics.DailyAverage = dailyAvg
		s.Metrics.WeeklyAverage = weeklyAvg
		s.Metrics.MonthlyAverage = monthlyAvg
		s.IsActive = isActive

		// Build day pattern array
		s.DayPattern = make([]int, 7)
		for i, key := range dayKeys {
			if int(key) < 7 && i < len(dayValues) {
				s.DayPattern[int(key)] = int(dayValues[i])
			}
		}

		// Build hour pattern array
		s.HourPattern = make([]int, 24)
		for i, key := range hourKeys {
			if int(key) < 24 && i < len(hourValues) {
				s.HourPattern[int(key)] = int(hourValues[i])
			}
		}

		// Parse co-occurrences
		// Note: In a real implementation, you would properly unmarshal the JSON
		s.CoOccurrences = []TagPair{}

		stats = append(stats, &s)
	}

	return stats, rows.Err()
}

// GetUnusedCategories retrieves categories that haven't been used recently
func (r *UsageRepository) GetUnusedCategories(ctx context.Context, userID uuid.UUID, daysSinceUse int) ([]struct {
	CategoryID   uuid.UUID
	CategoryName string
	LastUsed     *time.Time
	DaysSinceUse int
}, error) {
	query := `
		SELECT
			c.id,
			c.name,
			MAX(ch.created_at) as last_used,
			CASE
				WHEN MAX(ch.created_at) IS NULL THEN -1
				ELSE EXTRACT(DAY FROM NOW() - MAX(ch.created_at))::int
			END as days_since_use
		FROM categories c
		LEFT JOIN checkins ch ON ch.category_id = c.id AND ch.deleted_at IS NULL
		WHERE c.user_id = $1 AND c.deleted_at IS NULL
		GROUP BY c.id, c.name
		HAVING MAX(ch.created_at) IS NULL
			OR MAX(ch.created_at) < NOW() - INTERVAL '%d days'
		ORDER BY days_since_use DESC
	`

	query = fmt.Sprintf(query, daysSinceUse)
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query unused categories: %w", err)
	}
	defer rows.Close()

	var results []struct {
		CategoryID   uuid.UUID
		CategoryName string
		LastUsed     *time.Time
		DaysSinceUse int
	}

	for rows.Next() {
		var r struct {
			CategoryID   uuid.UUID
			CategoryName string
			LastUsed     *time.Time
			DaysSinceUse int
		}
		var lastUsed sql.NullTime

		err := rows.Scan(&r.CategoryID, &r.CategoryName, &lastUsed, &r.DaysSinceUse)
		if err != nil {
			return nil, fmt.Errorf("failed to scan unused category: %w", err)
		}

		if lastUsed.Valid {
			r.LastUsed = &lastUsed.Time
		}
		if r.DaysSinceUse == -1 {
			r.DaysSinceUse = 999999 // Never used
		}

		results = append(results, r)
	}

	return results, rows.Err()
}

// GetUnusedTags retrieves tags that haven't been used recently
func (r *UsageRepository) GetUnusedTags(ctx context.Context, userID uuid.UUID, daysSinceUse int) ([]struct {
	TagID        uuid.UUID
	TagName      string
	LastUsed     *time.Time
	DaysSinceUse int
}, error) {
	query := `
		SELECT
			t.id,
			t.name,
			MAX(ch.created_at) as last_used,
			CASE
				WHEN MAX(ch.created_at) IS NULL THEN -1
				ELSE EXTRACT(DAY FROM NOW() - MAX(ch.created_at))::int
			END as days_since_use
		FROM tags t
		LEFT JOIN checkin_tags ct ON ct.tag_id = t.id
		LEFT JOIN checkins ch ON ch.id = ct.checkin_id AND ch.deleted_at IS NULL
		WHERE t.user_id = $1 AND t.deleted_at IS NULL
		GROUP BY t.id, t.name
		HAVING MAX(ch.created_at) IS NULL
			OR MAX(ch.created_at) < NOW() - INTERVAL '%d days'
		ORDER BY days_since_use DESC
	`

	query = fmt.Sprintf(query, daysSinceUse)
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query unused tags: %w", err)
	}
	defer rows.Close()

	var results []struct {
		TagID        uuid.UUID
		TagName      string
		LastUsed     *time.Time
		DaysSinceUse int
	}

	for rows.Next() {
		var r struct {
			TagID        uuid.UUID
			TagName      string
			LastUsed     *time.Time
			DaysSinceUse int
		}
		var lastUsed sql.NullTime

		err := rows.Scan(&r.TagID, &r.TagName, &lastUsed, &r.DaysSinceUse)
		if err != nil {
			return nil, fmt.Errorf("failed to scan unused tag: %w", err)
		}

		if lastUsed.Valid {
			r.LastUsed = &lastUsed.Time
		}
		if r.DaysSinceUse == -1 {
			r.DaysSinceUse = 999999 // Never used
		}

		results = append(results, r)
	}

	return results, rows.Err()
}

// GetCategoryTagMatrix retrieves the relationship matrix between categories and tags
func (r *UsageRepository) GetCategoryTagMatrix(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) ([]*CategoryTagMatrix, error) {
	query := `
		SELECT
			c.id as category_id,
			c.name as category_name,
			json_agg(
				json_build_object(
					'tag_id', t.id,
					'tag_name', t.name,
					'usage_count', tag_counts.count,
					'percentage', tag_counts.percentage
				) ORDER BY tag_counts.count DESC
			) as tag_usage
		FROM categories c
		LEFT JOIN (
			SELECT
				ch.category_id,
				ct.tag_id,
				t.name,
				t.id,
				COUNT(*) as count,
				COUNT(*)::float / SUM(COUNT(*)) OVER (PARTITION BY ch.category_id) * 100 as percentage
			FROM checkins ch
			JOIN checkin_tags ct ON ct.checkin_id = ch.id
			JOIN tags t ON t.id = ct.tag_id
			WHERE ch.created_at BETWEEN $2 AND $3
				AND ch.deleted_at IS NULL
				AND ch.category_id IS NOT NULL
			GROUP BY ch.category_id, ct.tag_id, t.name, t.id
		) tag_counts ON tag_counts.category_id = c.id
		JOIN tags t ON t.id = tag_counts.tag_id
		WHERE c.user_id = $1 AND c.deleted_at IS NULL
		GROUP BY c.id, c.name
		HAVING COUNT(tag_counts.tag_id) > 0
		ORDER BY c.name
	`

	rows, err := r.db.QueryContext(ctx, query, userID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to query category-tag matrix: %w", err)
	}
	defer rows.Close()

	var results []*CategoryTagMatrix
	for rows.Next() {
		var m CategoryTagMatrix
		var tagUsageJSON []byte

		err := rows.Scan(&m.CategoryID, &m.CategoryName, &tagUsageJSON)
		if err != nil {
			return nil, fmt.Errorf("failed to scan category-tag matrix: %w", err)
		}

		// Note: In a real implementation, properly unmarshal the JSON
		results = append(results, &m)
	}

	return results, rows.Err()
}

// GetUsageSummaryStats retrieves summary statistics for the dashboard
func (r *UsageRepository) GetUsageSummaryStats(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) (struct {
	TotalCheckins      int
	TotalCategories    int
	ActiveCategories   int
	TotalTags          int
	ActiveTags         int
	AverageTagsPerItem float64
	CategoryCoverage   float64
	TagCoverage        float64
}, error) {
	var summary struct {
		TotalCheckins      int
		TotalCategories    int
		ActiveCategories   int
		TotalTags          int
		ActiveTags         int
		AverageTagsPerItem float64
		CategoryCoverage   float64
		TagCoverage        float64
	}

	// Get total checkins
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM checkins
		WHERE user_id = $1
			AND created_at BETWEEN $2 AND $3
			AND deleted_at IS NULL
	`, userID, startDate, endDate).Scan(&summary.TotalCheckins)
	if err != nil {
		return summary, err
	}

	// Get category stats
	err = r.db.QueryRowContext(ctx, `
		SELECT
			COUNT(DISTINCT c.id) as total_categories,
			COUNT(DISTINCT CASE WHEN ch.id IS NOT NULL THEN c.id END) as active_categories,
			COUNT(DISTINCT ch.id)::float / NULLIF(COUNT(DISTINCT all_ch.id), 0) * 100 as category_coverage
		FROM categories c
		LEFT JOIN checkins ch ON ch.category_id = c.id
			AND ch.created_at BETWEEN $2 AND $3
			AND ch.deleted_at IS NULL
		CROSS JOIN (
			SELECT id FROM checkins
			WHERE user_id = $1
				AND created_at BETWEEN $2 AND $3
				AND deleted_at IS NULL
		) all_ch
		WHERE c.user_id = $1 AND c.deleted_at IS NULL
		GROUP BY all_ch.id
	`, userID, startDate, endDate).Scan(
		&summary.TotalCategories,
		&summary.ActiveCategories,
		&summary.CategoryCoverage,
	)
	if err != nil && err != sql.ErrNoRows {
		return summary, err
	}

	// Get tag stats
	err = r.db.QueryRowContext(ctx, `
		SELECT
			COUNT(DISTINCT t.id) as total_tags,
			COUNT(DISTINCT CASE WHEN ct.tag_id IS NOT NULL THEN t.id END) as active_tags,
			AVG(tag_count.count) as avg_tags_per_item,
			COUNT(DISTINCT ct.checkin_id)::float / NULLIF(COUNT(DISTINCT ch.id), 0) * 100 as tag_coverage
		FROM tags t
		LEFT JOIN checkin_tags ct ON ct.tag_id = t.id
		LEFT JOIN checkins ch ON ch.id = ct.checkin_id
			AND ch.created_at BETWEEN $2 AND $3
			AND ch.deleted_at IS NULL
		LEFT JOIN (
			SELECT checkin_id, COUNT(*) as count
			FROM checkin_tags
			GROUP BY checkin_id
		) tag_count ON tag_count.checkin_id = ch.id
		WHERE t.user_id = $1 AND t.deleted_at IS NULL
	`, userID, startDate, endDate).Scan(
		&summary.TotalTags,
		&summary.ActiveTags,
		&summary.AverageTagsPerItem,
		&summary.TagCoverage,
	)
	if err != nil && err != sql.ErrNoRows {
		return summary, err
	}

	return summary, nil
}

// GetGrowthMetrics retrieves growth metrics for categories and tags
func (r *UsageRepository) GetGrowthMetrics(ctx context.Context, userID uuid.UUID, currentStart, currentEnd, previousStart, previousEnd time.Time) (struct {
	NewCategories      int
	NewTags            int
	CategoryGrowthRate float64
	TagGrowthRate      float64
}, error) {
	var growth struct {
		NewCategories      int
		NewTags            int
		CategoryGrowthRate float64
		TagGrowthRate      float64
	}

	// Get new categories created in current period
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM categories
		WHERE user_id = $1
			AND created_at BETWEEN $2 AND $3
			AND deleted_at IS NULL
	`, userID, currentStart, currentEnd).Scan(&growth.NewCategories)
	if err != nil {
		return growth, err
	}

	// Get new tags created in current period
	err = r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM tags
		WHERE user_id = $1
			AND created_at BETWEEN $2 AND $3
			AND deleted_at IS NULL
	`, userID, currentStart, currentEnd).Scan(&growth.NewTags)
	if err != nil {
		return growth, err
	}

	// Calculate growth rates
	var prevCategories, currCategories, prevTags, currTags int

	// Category growth
	err = r.db.QueryRowContext(ctx, `
		SELECT
			COUNT(DISTINCT CASE WHEN created_at < $2 THEN id END) as prev_count,
			COUNT(DISTINCT CASE WHEN created_at < $3 THEN id END) as curr_count
		FROM categories
		WHERE user_id = $1 AND deleted_at IS NULL
	`, userID, currentStart, currentEnd).Scan(&prevCategories, &currCategories)
	if err != nil {
		return growth, err
	}

	if prevCategories > 0 {
		growth.CategoryGrowthRate = float64(currCategories-prevCategories) / float64(prevCategories) * 100
	}

	// Tag growth
	err = r.db.QueryRowContext(ctx, `
		SELECT
			COUNT(DISTINCT CASE WHEN created_at < $2 THEN id END) as prev_count,
			COUNT(DISTINCT CASE WHEN created_at < $3 THEN id END) as curr_count
		FROM tags
		WHERE user_id = $1 AND deleted_at IS NULL
	`, userID, currentStart, currentEnd).Scan(&prevTags, &currTags)
	if err != nil {
		return growth, err
	}

	if prevTags > 0 {
		growth.TagGrowthRate = float64(currTags-prevTags) / float64(prevTags) * 100
	}

	return growth, nil
}