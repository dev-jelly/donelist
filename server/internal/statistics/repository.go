package statistics

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// RepositoryInterface defines the interface for statistics repository operations
type RepositoryInterface interface {
	GetDailyAggregates(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) ([]DailyAggregate, error)
	GetCategoryAggregates(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) ([]CategoryAggregate, error)
	GetTimeOfDayAggregates(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) ([]TimeOfDayAggregate, error)
	GetDayOfWeekAggregates(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) ([]DayOfWeekAggregate, error)
	GetWeekTotal(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) (int, int, error)
	GetStreakData(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) (map[string]bool, error)
	GetLastCheckinDate(ctx context.Context, userID uuid.UUID) (*time.Time, error)
}

// Repository handles statistics data operations
type Repository struct {
	db *sqlx.DB
}

// NewRepository creates a new statistics repository
func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

// DailyAggregate represents aggregated check-in data for a single day
type DailyAggregate struct {
	Date         time.Time  `db:"date"`
	CheckinCount int        `db:"checkin_count"`
	TotalMinutes int        `db:"total_minutes"`
	CategoryID   *uuid.UUID `db:"category_id"`
}

// CategoryAggregate represents check-in statistics by category
type CategoryAggregate struct {
	CategoryID   *uuid.UUID `db:"category_id"`
	CategoryName string     `db:"category_name"`
	CheckinCount int        `db:"checkin_count"`
	TotalMinutes int        `db:"total_minutes"`
}

// TimeOfDayAggregate represents check-in statistics by time of day
type TimeOfDayAggregate struct {
	Hour         int `db:"hour"`
	CheckinCount int `db:"checkin_count"`
	TotalMinutes int `db:"total_minutes"`
}

// DayOfWeekAggregate represents check-in statistics by day of week
type DayOfWeekAggregate struct {
	DayOfWeek    int `db:"day_of_week"` // 0 = Sunday, 1 = Monday, etc.
	CheckinCount int `db:"checkin_count"`
	TotalMinutes int `db:"total_minutes"`
}

// GetDailyAggregates retrieves daily aggregated check-in data for a date range
func (r *Repository) GetDailyAggregates(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) ([]DailyAggregate, error) {
	query := `
		SELECT
			DATE(checkin_time AT TIME ZONE 'UTC') as date,
			COUNT(*) as checkin_count,
			COALESCE(SUM(duration_minutes), 0) as total_minutes
		FROM checkins
		WHERE user_id = $1
			AND checkin_time >= $2
			AND checkin_time < $3
			AND deleted_at IS NULL
		GROUP BY DATE(checkin_time AT TIME ZONE 'UTC')
		ORDER BY date ASC
	`

	var aggregates []DailyAggregate
	err := r.db.SelectContext(ctx, &aggregates, query, userID, startDate, endDate)
	if err != nil {
		if err == sql.ErrNoRows {
			return []DailyAggregate{}, nil
		}
		return nil, err
	}

	return aggregates, nil
}

// GetCategoryAggregates retrieves category breakdown for a date range
func (r *Repository) GetCategoryAggregates(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) ([]CategoryAggregate, error) {
	query := `
		SELECT
			c.category_id,
			COALESCE(cat.name, 'Uncategorized') as category_name,
			COUNT(*) as checkin_count,
			COALESCE(SUM(c.duration_minutes), 0) as total_minutes
		FROM checkins c
		LEFT JOIN categories cat ON c.category_id = cat.id
		WHERE c.user_id = $1
			AND c.checkin_time >= $2
			AND c.checkin_time < $3
			AND c.deleted_at IS NULL
		GROUP BY c.category_id, cat.name
		ORDER BY checkin_count DESC
	`

	var aggregates []CategoryAggregate
	err := r.db.SelectContext(ctx, &aggregates, query, userID, startDate, endDate)
	if err != nil {
		if err == sql.ErrNoRows {
			return []CategoryAggregate{}, nil
		}
		return nil, err
	}

	return aggregates, nil
}

// GetTimeOfDayAggregates retrieves check-in distribution by hour of day
func (r *Repository) GetTimeOfDayAggregates(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) ([]TimeOfDayAggregate, error) {
	query := `
		SELECT
			EXTRACT(HOUR FROM checkin_time AT TIME ZONE 'UTC') as hour,
			COUNT(*) as checkin_count,
			COALESCE(SUM(duration_minutes), 0) as total_minutes
		FROM checkins
		WHERE user_id = $1
			AND checkin_time >= $2
			AND checkin_time < $3
			AND deleted_at IS NULL
		GROUP BY EXTRACT(HOUR FROM checkin_time AT TIME ZONE 'UTC')
		ORDER BY hour ASC
	`

	var aggregates []TimeOfDayAggregate
	err := r.db.SelectContext(ctx, &aggregates, query, userID, startDate, endDate)
	if err != nil {
		if err == sql.ErrNoRows {
			return []TimeOfDayAggregate{}, nil
		}
		return nil, err
	}

	return aggregates, nil
}

// GetDayOfWeekAggregates retrieves check-in distribution by day of week
func (r *Repository) GetDayOfWeekAggregates(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) ([]DayOfWeekAggregate, error) {
	query := `
		SELECT
			EXTRACT(DOW FROM checkin_time AT TIME ZONE 'UTC') as day_of_week,
			COUNT(*) as checkin_count,
			COALESCE(SUM(duration_minutes), 0) as total_minutes
		FROM checkins
		WHERE user_id = $1
			AND checkin_time >= $2
			AND checkin_time < $3
			AND deleted_at IS NULL
		GROUP BY EXTRACT(DOW FROM checkin_time AT TIME ZONE 'UTC')
		ORDER BY day_of_week ASC
	`

	var aggregates []DayOfWeekAggregate
	err := r.db.SelectContext(ctx, &aggregates, query, userID, startDate, endDate)
	if err != nil {
		if err == sql.ErrNoRows {
			return []DayOfWeekAggregate{}, nil
		}
		return nil, err
	}

	return aggregates, nil
}

// GetWeekTotal retrieves total check-in count for a week (for comparison)
func (r *Repository) GetWeekTotal(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) (int, int, error) {
	query := `
		SELECT
			COUNT(*) as checkin_count,
			COALESCE(SUM(duration_minutes), 0) as total_minutes
		FROM checkins
		WHERE user_id = $1
			AND checkin_time >= $2
			AND checkin_time < $3
			AND deleted_at IS NULL
	`

	var result struct {
		CheckinCount int `db:"checkin_count"`
		TotalMinutes int `db:"total_minutes"`
	}

	err := r.db.GetContext(ctx, &result, query, userID, startDate, endDate)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, 0, nil
		}
		return 0, 0, err
	}

	return result.CheckinCount, result.TotalMinutes, nil
}

// GetStreakData retrieves data needed for streak calculation
// Returns a map of dates (YYYY-MM-DD) that have check-ins
func (r *Repository) GetStreakData(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) (map[string]bool, error) {
	query := `
		SELECT DISTINCT DATE(checkin_time AT TIME ZONE 'UTC') as date
		FROM checkins
		WHERE user_id = $1
			AND checkin_time >= $2
			AND checkin_time < $3
			AND deleted_at IS NULL
		ORDER BY date DESC
	`

	var dates []time.Time
	err := r.db.SelectContext(ctx, &dates, query, userID, startDate, endDate)
	if err != nil {
		if err == sql.ErrNoRows {
			return map[string]bool{}, nil
		}
		return nil, err
	}

	dateMap := make(map[string]bool)
	for _, date := range dates {
		dateKey := date.Format("2006-01-02")
		dateMap[dateKey] = true
	}

	return dateMap, nil
}

// GetLastCheckinDate retrieves the most recent check-in date for a user
func (r *Repository) GetLastCheckinDate(ctx context.Context, userID uuid.UUID) (*time.Time, error) {
	query := `
		SELECT DATE(checkin_time AT TIME ZONE 'UTC') as date
		FROM checkins
		WHERE user_id = $1
			AND deleted_at IS NULL
		ORDER BY checkin_time DESC
		LIMIT 1
	`

	var date time.Time
	err := r.db.GetContext(ctx, &date, query, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &date, nil
}
