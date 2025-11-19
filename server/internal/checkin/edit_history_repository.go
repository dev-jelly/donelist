package checkin

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// EditHistory represents a single edit history entry
type EditHistory struct {
	ID                 uuid.UUID  `db:"id" json:"id"`
	CheckinID          uuid.UUID  `db:"checkin_id" json:"checkin_id"`
	UserID             uuid.UUID  `db:"user_id" json:"user_id"`
	PreviousContent    string     `db:"previous_content" json:"previous_content"`
	PreviousCategoryID *uuid.UUID `db:"previous_category_id" json:"previous_category_id,omitempty"`
	EditedAt           time.Time  `db:"edited_at" json:"edited_at"`
	EditReason         *string    `db:"edit_reason" json:"edit_reason,omitempty"`
}

// CreateEditHistoryInput represents input for creating an edit history entry
type CreateEditHistoryInput struct {
	CheckinID          uuid.UUID
	UserID             uuid.UUID
	PreviousContent    string
	PreviousCategoryID *uuid.UUID
	EditReason         *string
}

// EditHistoryRepository handles edit history database operations
type EditHistoryRepository struct {
	db *sqlx.DB
}

// NewEditHistoryRepository creates a new edit history repository
func NewEditHistoryRepository(db *sqlx.DB) *EditHistoryRepository {
	return &EditHistoryRepository{db: db}
}

// Create creates a new edit history entry
func (r *EditHistoryRepository) Create(ctx context.Context, input CreateEditHistoryInput) (*EditHistory, error) {
	query := `
		INSERT INTO edit_history (checkin_id, user_id, previous_content, previous_category_id, edit_reason)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, checkin_id, user_id, previous_content, previous_category_id, edited_at, edit_reason
	`

	var history EditHistory
	err := r.db.GetContext(ctx, &history, query,
		input.CheckinID,
		input.UserID,
		input.PreviousContent,
		input.PreviousCategoryID,
		input.EditReason,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create edit history: %w", err)
	}

	return &history, nil
}

// GetByCheckinID retrieves all edit history entries for a specific check-in
// Returns entries in reverse chronological order (newest first)
func (r *EditHistoryRepository) GetByCheckinID(ctx context.Context, checkinID uuid.UUID) ([]EditHistory, error) {
	query := `
		SELECT id, checkin_id, user_id, previous_content, previous_category_id, edited_at, edit_reason
		FROM edit_history
		WHERE checkin_id = $1
		ORDER BY edited_at DESC
	`

	var histories []EditHistory
	err := r.db.SelectContext(ctx, &histories, query, checkinID)
	if err != nil {
		return nil, fmt.Errorf("failed to get edit history: %w", err)
	}

	return histories, nil
}

// GetByUserID retrieves edit history entries for a specific user
// Supports pagination and time filtering
func (r *EditHistoryRepository) GetByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]EditHistory, error) {
	query := `
		SELECT id, checkin_id, user_id, previous_content, previous_category_id, edited_at, edit_reason
		FROM edit_history
		WHERE user_id = $1
		ORDER BY edited_at DESC
		LIMIT $2 OFFSET $3
	`

	var histories []EditHistory
	err := r.db.SelectContext(ctx, &histories, query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get user edit history: %w", err)
	}

	return histories, nil
}

// GetByDateRange retrieves edit history entries within a date range for a user
func (r *EditHistoryRepository) GetByDateRange(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) ([]EditHistory, error) {
	query := `
		SELECT id, checkin_id, user_id, previous_content, previous_category_id, edited_at, edit_reason
		FROM edit_history
		WHERE user_id = $1
		  AND edited_at >= $2
		  AND edited_at < $3
		ORDER BY edited_at DESC
	`

	var histories []EditHistory
	err := r.db.SelectContext(ctx, &histories, query, userID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get edit history by date range: %w", err)
	}

	return histories, nil
}

// CountByCheckinID returns the number of edits for a specific check-in
func (r *EditHistoryRepository) CountByCheckinID(ctx context.Context, checkinID uuid.UUID) (int, error) {
	query := `SELECT COUNT(*) FROM edit_history WHERE checkin_id = $1`

	var count int
	err := r.db.GetContext(ctx, &count, query, checkinID)
	if err != nil {
		return 0, fmt.Errorf("failed to count edit history: %w", err)
	}

	return count, nil
}

// DeleteByCheckinID deletes all edit history entries for a check-in (cascade delete handling)
func (r *EditHistoryRepository) DeleteByCheckinID(ctx context.Context, checkinID uuid.UUID) error {
	query := `DELETE FROM edit_history WHERE checkin_id = $1`

	_, err := r.db.ExecContext(ctx, query, checkinID)
	if err != nil {
		return fmt.Errorf("failed to delete edit history: %w", err)
	}

	return nil
}
