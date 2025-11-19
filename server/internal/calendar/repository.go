package calendar

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// Repository handles calendar data operations
type Repository struct {
	db *sqlx.DB
}

// NewRepository creates a new calendar repository
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

// CategoryAggregate represents check-in count by category for a month
type CategoryAggregate struct {
	CategoryID   *uuid.UUID `db:"category_id"`
	CategoryName string     `db:"category_name"`
	Count        int        `db:"count"`
}

// GetDailyAggregates retrieves daily aggregated check-in data for a month
// Uses efficient SQL aggregation instead of fetching all check-ins
func (r *Repository) GetDailyAggregates(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) ([]DailyAggregate, error) {
	query := `
		SELECT
			DATE(checkin_time AT TIME ZONE 'UTC') as date,
			COUNT(*) as checkin_count,
			SUM(duration_minutes) as total_minutes
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

// GetCategoryAggregates retrieves category breakdown for a month
func (r *Repository) GetCategoryAggregates(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) ([]CategoryAggregate, error) {
	query := `
		SELECT
			c.category_id,
			COALESCE(cat.name, 'Uncategorized') as category_name,
			COUNT(*) as count
		FROM checkins c
		LEFT JOIN categories cat ON c.category_id = cat.id
		WHERE c.user_id = $1
			AND c.checkin_time >= $2
			AND c.checkin_time < $3
			AND c.deleted_at IS NULL
		GROUP BY c.category_id, cat.name
		ORDER BY count DESC
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

// GetMonthlyTotal retrieves total check-in count for a month (for caching key)
func (r *Repository) GetMonthlyTotal(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM checkins
		WHERE user_id = $1
			AND checkin_time >= $2
			AND checkin_time < $3
			AND deleted_at IS NULL
	`

	var total int
	err := r.db.GetContext(ctx, &total, query, userID, startDate, endDate)
	if err != nil {
		return 0, err
	}

	return total, nil
}
