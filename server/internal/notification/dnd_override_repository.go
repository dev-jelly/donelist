package notification

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// DNDOverridePostgresRepository implements DNDOverrideRepository using PostgreSQL
type DNDOverridePostgresRepository struct {
	db     *sqlx.DB
	logger *zap.Logger
}

// NewDNDOverridePostgresRepository creates a new PostgreSQL-based DND override repository
func NewDNDOverridePostgresRepository(db *sqlx.DB, logger *zap.Logger) *DNDOverridePostgresRepository {
	return &DNDOverridePostgresRepository{
		db:     db,
		logger: logger,
	}
}

// CreateOverride creates a new DND override
func (r *DNDOverridePostgresRepository) CreateOverride(ctx context.Context, override *DNDOverride) error {
	query := `
		INSERT INTO dnd_overrides (
			id, user_id, start_time, end_time, reason, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6
		)
	`

	_, err := r.db.ExecContext(ctx, query,
		override.ID,
		override.UserID,
		override.StartTime,
		override.EndTime,
		override.Reason,
		override.CreatedAt,
	)

	if err != nil {
		r.logger.Error("Failed to create DND override",
			zap.String("override_id", override.ID.String()),
			zap.Error(err),
		)
		return fmt.Errorf("failed to create DND override: %w", err)
	}

	return nil
}

// GetActiveOverride retrieves the active DND override for a user at a specific time
func (r *DNDOverridePostgresRepository) GetActiveOverride(ctx context.Context, userID uuid.UUID, checkTime time.Time) (*DNDOverride, error) {
	query := `
		SELECT
			id, user_id, start_time, end_time, reason, created_at
		FROM dnd_overrides
		WHERE user_id = $1
			AND start_time <= $2
			AND end_time > $2
		ORDER BY start_time DESC
		LIMIT 1
	`

	var override DNDOverride
	err := r.db.GetContext(ctx, &override, query, userID, checkTime)

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		r.logger.Error("Failed to get active DND override",
			zap.String("user_id", userID.String()),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to get active DND override: %w", err)
	}

	return &override, nil
}

// GetUserOverrides retrieves all DND overrides for a user
func (r *DNDOverridePostgresRepository) GetUserOverrides(ctx context.Context, userID uuid.UUID) ([]*DNDOverride, error) {
	query := `
		SELECT
			id, user_id, start_time, end_time, reason, created_at
		FROM dnd_overrides
		WHERE user_id = $1
		ORDER BY start_time DESC
	`

	var overrides []*DNDOverride
	err := r.db.SelectContext(ctx, &overrides, query, userID)

	if err != nil {
		r.logger.Error("Failed to get user DND overrides",
			zap.String("user_id", userID.String()),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to get user DND overrides: %w", err)
	}

	return overrides, nil
}

// DeleteOverride deletes a DND override
func (r *DNDOverridePostgresRepository) DeleteOverride(ctx context.Context, overrideID uuid.UUID) error {
	query := `DELETE FROM dnd_overrides WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query, overrideID)
	if err != nil {
		r.logger.Error("Failed to delete DND override",
			zap.String("override_id", overrideID.String()),
			zap.Error(err),
		)
		return fmt.Errorf("failed to delete DND override: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("DND override not found")
	}

	return nil
}

// CleanupExpiredOverrides removes expired DND overrides
func (r *DNDOverridePostgresRepository) CleanupExpiredOverrides(ctx context.Context) (int, error) {
	query := `
		DELETE FROM dnd_overrides
		WHERE end_time < NOW() - INTERVAL '7 days'
	`

	result, err := r.db.ExecContext(ctx, query)
	if err != nil {
		r.logger.Error("Failed to cleanup expired DND overrides", zap.Error(err))
		return 0, fmt.Errorf("failed to cleanup expired DND overrides: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return int(rowsAffected), nil
}

// GetActiveOverridesCount returns the count of currently active overrides
func (r *DNDOverridePostgresRepository) GetActiveOverridesCount(ctx context.Context) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM dnd_overrides
		WHERE start_time <= NOW()
			AND end_time > NOW()
	`

	var count int
	err := r.db.GetContext(ctx, &count, query)

	if err != nil {
		r.logger.Error("Failed to get active overrides count", zap.Error(err))
		return 0, fmt.Errorf("failed to get active overrides count: %w", err)
	}

	return count, nil
}

// GetUpcomingOverrides retrieves upcoming DND overrides for a user
func (r *DNDOverridePostgresRepository) GetUpcomingOverrides(ctx context.Context, userID uuid.UUID, limit int) ([]*DNDOverride, error) {
	query := `
		SELECT
			id, user_id, start_time, end_time, reason, created_at
		FROM dnd_overrides
		WHERE user_id = $1
			AND start_time > NOW()
		ORDER BY start_time ASC
		LIMIT $2
	`

	var overrides []*DNDOverride
	err := r.db.SelectContext(ctx, &overrides, query, userID, limit)

	if err != nil {
		r.logger.Error("Failed to get upcoming DND overrides",
			zap.String("user_id", userID.String()),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to get upcoming DND overrides: %w", err)
	}

	return overrides, nil
}

// GetOverridesByReason retrieves DND overrides by reason
func (r *DNDOverridePostgresRepository) GetOverridesByReason(ctx context.Context, userID uuid.UUID, reason string) ([]*DNDOverride, error) {
	query := `
		SELECT
			id, user_id, start_time, end_time, reason, created_at
		FROM dnd_overrides
		WHERE user_id = $1
			AND reason = $2
		ORDER BY start_time DESC
	`

	var overrides []*DNDOverride
	err := r.db.SelectContext(ctx, &overrides, query, userID, reason)

	if err != nil {
		r.logger.Error("Failed to get DND overrides by reason",
			zap.String("user_id", userID.String()),
			zap.String("reason", reason),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to get DND overrides by reason: %w", err)
	}

	return overrides, nil
}

// HasActiveOverride checks if a user has any active override
func (r *DNDOverridePostgresRepository) HasActiveOverride(ctx context.Context, userID uuid.UUID) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1
			FROM dnd_overrides
			WHERE user_id = $1
				AND start_time <= NOW()
				AND end_time > NOW()
		)
	`

	var exists bool
	err := r.db.GetContext(ctx, &exists, query, userID)

	if err != nil {
		r.logger.Error("Failed to check for active override",
			zap.String("user_id", userID.String()),
			zap.Error(err),
		)
		return false, fmt.Errorf("failed to check for active override: %w", err)
	}

	return exists, nil
}

// UpdateOverride updates an existing DND override
func (r *DNDOverridePostgresRepository) UpdateOverride(ctx context.Context, override *DNDOverride) error {
	query := `
		UPDATE dnd_overrides
		SET
			start_time = $2,
			end_time = $3,
			reason = $4
		WHERE id = $1
	`

	result, err := r.db.ExecContext(ctx, query,
		override.ID,
		override.StartTime,
		override.EndTime,
		override.Reason,
	)

	if err != nil {
		r.logger.Error("Failed to update DND override",
			zap.String("override_id", override.ID.String()),
			zap.Error(err),
		)
		return fmt.Errorf("failed to update DND override: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("DND override not found")
	}

	return nil
}

// GetOverrideStats returns statistics about DND overrides
func (r *DNDOverridePostgresRepository) GetOverrideStats(ctx context.Context, userID uuid.UUID) (map[string]interface{}, error) {
	query := `
		SELECT
			COUNT(*) as total_count,
			COUNT(*) FILTER (WHERE start_time <= NOW() AND end_time > NOW()) as active_count,
			COUNT(*) FILTER (WHERE start_time > NOW()) as upcoming_count,
			COUNT(*) FILTER (WHERE end_time < NOW()) as expired_count,
			COUNT(*) FILTER (WHERE reason = 'urgent') as urgent_count,
			COUNT(*) FILTER (WHERE reason = 'emergency') as emergency_count,
			COUNT(*) FILTER (WHERE reason = 'travel') as travel_count
		FROM dnd_overrides
		WHERE user_id = $1
	`

	var stats struct {
		TotalCount     int `db:"total_count"`
		ActiveCount    int `db:"active_count"`
		UpcomingCount  int `db:"upcoming_count"`
		ExpiredCount   int `db:"expired_count"`
		UrgentCount    int `db:"urgent_count"`
		EmergencyCount int `db:"emergency_count"`
		TravelCount    int `db:"travel_count"`
	}

	err := r.db.GetContext(ctx, &stats, query, userID)
	if err != nil {
		r.logger.Error("Failed to get override stats",
			zap.String("user_id", userID.String()),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to get override stats: %w", err)
	}

	return map[string]interface{}{
		"total_count":     stats.TotalCount,
		"active_count":    stats.ActiveCount,
		"upcoming_count":  stats.UpcomingCount,
		"expired_count":   stats.ExpiredCount,
		"urgent_count":    stats.UrgentCount,
		"emergency_count": stats.EmergencyCount,
		"travel_count":    stats.TravelCount,
	}, nil
}
