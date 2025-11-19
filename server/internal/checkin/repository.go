package checkin

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// Checkin represents a check-in entry
type Checkin struct {
	ID              uuid.UUID  `db:"id" json:"id"`
	UserID          uuid.UUID  `db:"user_id" json:"user_id"`
	CategoryID      *uuid.UUID `db:"category_id" json:"category_id,omitempty"`
	Content         string     `db:"content" json:"content"`
	CheckinTime     time.Time  `db:"checkin_time" json:"checkin_time"`
	DurationMinutes int        `db:"duration_minutes" json:"duration_minutes"`
	IsEdited        bool       `db:"is_edited" json:"is_edited"`
	EditCount       int        `db:"edit_count" json:"edit_count"`
	LastEditedAt    *time.Time `db:"last_edited_at" json:"last_edited_at,omitempty"`
	Version         int        `db:"version" json:"version"` // For optimistic locking
	TeamID          *uuid.UUID `db:"team_id" json:"team_id,omitempty"`
	Visibility      string     `db:"visibility" json:"visibility"` // private, team, public
	CreatedAt       time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time  `db:"updated_at" json:"updated_at"`
	DeletedAt       *time.Time `db:"deleted_at" json:"-"`
	Tags            []string   `json:"tags,omitempty"` // Will be populated separately
}

// CreateCheckinInput represents input for creating a check-in
type CreateCheckinInput struct {
	UserID          uuid.UUID
	CategoryID      *uuid.UUID
	Content         string
	CheckinTime     time.Time
	DurationMinutes int
	TagIDs          []uuid.UUID
	TeamID          *uuid.UUID // Optional team association
	Visibility      string     // private, team, public (defaults to private)
}

// UpdateCheckinInput represents input for updating a check-in
type UpdateCheckinInput struct {
	Content    *string
	CategoryID *uuid.UUID
	TagIDs     []uuid.UUID
	EditReason *string // Optional reason for the edit (for audit trail)
	Version    int     // Current version for optimistic locking
}

// ListOptions represents options for listing check-ins
type ListOptions struct {
	UserID     uuid.UUID
	StartDate  *time.Time
	EndDate    *time.Time
	CategoryID *uuid.UUID
	TeamID     *uuid.UUID // Filter by team (for team mode)
	Visibility *string    // Filter by visibility (private, team, public)
	Limit      int
	Offset     int
}

// Repository handles check-in database operations
type Repository struct {
	db *sqlx.DB
}

// NewRepository creates a new check-in repository
func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

// Create creates a new check-in
func (r *Repository) Create(ctx context.Context, input CreateCheckinInput) (*Checkin, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Set default visibility if not provided
	visibility := input.Visibility
	if visibility == "" {
		visibility = "private"
	}

	query := `
		INSERT INTO checkins (user_id, category_id, content, checkin_time, duration_minutes, team_id, visibility)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, user_id, category_id, content, checkin_time, duration_minutes,
		          is_edited, edit_count, last_edited_at, version, team_id, visibility,
		          created_at, updated_at, deleted_at
	`

	var checkin Checkin
	err = tx.GetContext(ctx, &checkin, query,
		input.UserID, input.CategoryID, input.Content, input.CheckinTime, input.DurationMinutes,
		input.TeamID, visibility)
	if err != nil {
		return nil, fmt.Errorf("failed to create check-in: %w", err)
	}

	// Add tags if provided
	if len(input.TagIDs) > 0 {
		for _, tagID := range input.TagIDs {
			tagQuery := `INSERT INTO checkin_tags (checkin_id, tag_id) VALUES ($1, $2)`
			if _, err := tx.ExecContext(ctx, tagQuery, checkin.ID, tagID); err != nil {
				return nil, fmt.Errorf("failed to add tag: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &checkin, nil
}

// GetByID retrieves a check-in by ID
func (r *Repository) GetByID(ctx context.Context, id, userID uuid.UUID) (*Checkin, error) {
	query := `
		SELECT id, user_id, category_id, content, checkin_time, duration_minutes,
		       is_edited, edit_count, last_edited_at, version, team_id, visibility,
		       created_at, updated_at, deleted_at
		FROM checkins
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
	`

	var checkin Checkin
	err := r.db.GetContext(ctx, &checkin, query, id, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("check-in not found")
		}
		return nil, fmt.Errorf("failed to get check-in: %w", err)
	}

	// Load tags
	if err := r.loadTags(ctx, &checkin); err != nil {
		return nil, err
	}

	return &checkin, nil
}

// GetLastCheckin retrieves the most recent check-in for a user
// Returns nil if no check-ins exist (not an error)
func (r *Repository) GetLastCheckin(ctx context.Context, userID uuid.UUID) (*Checkin, error) {
	query := `
		SELECT id, user_id, category_id, content, checkin_time, duration_minutes,
		       is_edited, edit_count, last_edited_at, version, team_id, visibility,
		       created_at, updated_at, deleted_at
		FROM checkins
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY checkin_time DESC, created_at DESC
		LIMIT 1
	`

	var checkin Checkin
	err := r.db.GetContext(ctx, &checkin, query, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			// No check-ins exist yet - this is not an error
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get last check-in: %w", err)
	}

	// Load tags
	if err := r.loadTags(ctx, &checkin); err != nil {
		return nil, err
	}

	return &checkin, nil
}

// CheckDuplicateAtTime checks if a check-in exists at or near the specified time
// Returns true if a duplicate exists within a 5-second window
func (r *Repository) CheckDuplicateAtTime(ctx context.Context, userID uuid.UUID, checkinTime time.Time) (bool, error) {
	// Check for check-ins within ±5 seconds of the specified time
	// This prevents accidental duplicates from double-clicks or network retries
	windowStart := checkinTime.Add(-5 * time.Second)
	windowEnd := checkinTime.Add(5 * time.Second)

	query := `
		SELECT EXISTS(
			SELECT 1 FROM checkins
			WHERE user_id = $1
			  AND checkin_time >= $2
			  AND checkin_time <= $3
			  AND deleted_at IS NULL
		)
	`

	var exists bool
	err := r.db.GetContext(ctx, &exists, query, userID, windowStart, windowEnd)
	if err != nil {
		return false, fmt.Errorf("failed to check duplicate: %w", err)
	}

	return exists, nil
}

// List retrieves check-ins with filtering options
func (r *Repository) List(ctx context.Context, opts ListOptions) ([]*Checkin, int, error) {
	query := `
		SELECT id, user_id, category_id, content, checkin_time, duration_minutes,
		       is_edited, edit_count, last_edited_at, version, team_id, visibility,
		       created_at, updated_at, deleted_at
		FROM checkins
		WHERE user_id = $1 AND deleted_at IS NULL
	`
	countQuery := `SELECT COUNT(*) FROM checkins WHERE user_id = $1 AND deleted_at IS NULL`

	args := []interface{}{opts.UserID}
	argIndex := 2

	if opts.StartDate != nil {
		query += fmt.Sprintf(" AND checkin_time >= $%d", argIndex)
		countQuery += fmt.Sprintf(" AND checkin_time >= $%d", argIndex)
		args = append(args, opts.StartDate)
		argIndex++
	}

	if opts.EndDate != nil {
		query += fmt.Sprintf(" AND checkin_time <= $%d", argIndex)
		countQuery += fmt.Sprintf(" AND checkin_time <= $%d", argIndex)
		args = append(args, opts.EndDate)
		argIndex++
	}

	if opts.CategoryID != nil {
		query += fmt.Sprintf(" AND category_id = $%d", argIndex)
		countQuery += fmt.Sprintf(" AND category_id = $%d", argIndex)
		args = append(args, opts.CategoryID)
		argIndex++
	}

	if opts.TeamID != nil {
		query += fmt.Sprintf(" AND team_id = $%d", argIndex)
		countQuery += fmt.Sprintf(" AND team_id = $%d", argIndex)
		args = append(args, opts.TeamID)
		argIndex++
	}

	if opts.Visibility != nil {
		query += fmt.Sprintf(" AND visibility = $%d", argIndex)
		countQuery += fmt.Sprintf(" AND visibility = $%d", argIndex)
		args = append(args, opts.Visibility)
		argIndex++
	}

	query += " ORDER BY checkin_time DESC"

	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argIndex)
		args = append(args, opts.Limit)
		argIndex++
	}

	if opts.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argIndex)
		args = append(args, opts.Offset)
	}

	// Get total count
	var total int
	countArgs := args[:argIndex-1] // Exclude LIMIT/OFFSET from count
	if opts.Limit > 0 {
		countArgs = args[:len(args)-1]
		if opts.Offset > 0 {
			countArgs = args[:len(args)-2]
		}
	}
	if err := r.db.GetContext(ctx, &total, countQuery, countArgs...); err != nil {
		return nil, 0, fmt.Errorf("failed to count check-ins: %w", err)
	}

	// Get check-ins
	var checkins []*Checkin
	if err := r.db.SelectContext(ctx, &checkins, query, args...); err != nil {
		return nil, 0, fmt.Errorf("failed to list check-ins: %w", err)
	}

	// Load tags for each check-in
	for _, checkin := range checkins {
		if err := r.loadTags(ctx, checkin); err != nil {
			return nil, 0, err
		}
	}

	return checkins, total, nil
}

// Update updates a check-in with optimistic locking support
func (r *Repository) Update(ctx context.Context, id, userID uuid.UUID, input UpdateCheckinInput) (*Checkin, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Store old version in edit_history with edit reason
	historyQuery := `
		INSERT INTO edit_history (checkin_id, user_id, previous_content, previous_category_id, edit_reason)
		SELECT id, user_id, content, category_id, $3
		FROM checkins
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
	`
	if _, err := tx.ExecContext(ctx, historyQuery, id, userID, input.EditReason); err != nil {
		return nil, fmt.Errorf("failed to save edit history: %w", err)
	}

	// Update with optimistic locking - increment version and check current version
	query := `
		UPDATE checkins
		SET content = COALESCE($3, content),
		    category_id = COALESCE($4, category_id),
		    is_edited = TRUE,
		    edit_count = edit_count + 1,
		    last_edited_at = CURRENT_TIMESTAMP,
		    version = version + 1,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND user_id = $2 AND version = $5 AND deleted_at IS NULL
		RETURNING id, user_id, category_id, content, checkin_time, duration_minutes,
		          is_edited, edit_count, last_edited_at, version, created_at, updated_at, deleted_at
	`

	var checkin Checkin
	err = tx.GetContext(ctx, &checkin, query, id, userID, input.Content, input.CategoryID, input.Version)
	if err != nil {
		if err == sql.ErrNoRows {
			// Check if checkin exists at all
			var exists bool
			checkQuery := `SELECT EXISTS(SELECT 1 FROM checkins WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL)`
			if checkErr := tx.GetContext(ctx, &exists, checkQuery, id, userID); checkErr == nil && exists {
				// Checkin exists but version mismatch - concurrent edit detected
				return nil, fmt.Errorf("concurrent edit detected: check-in was modified by another request")
			}
			return nil, fmt.Errorf("check-in not found")
		}
		return nil, fmt.Errorf("failed to update check-in: %w", err)
	}

	// Update tags if provided
	if input.TagIDs != nil {
		// Delete existing tags
		deleteTagsQuery := `DELETE FROM checkin_tags WHERE checkin_id = $1`
		if _, err := tx.ExecContext(ctx, deleteTagsQuery, id); err != nil {
			return nil, fmt.Errorf("failed to delete old tags: %w", err)
		}

		// Add new tags
		for _, tagID := range input.TagIDs {
			tagQuery := `INSERT INTO checkin_tags (checkin_id, tag_id) VALUES ($1, $2)`
			if _, err := tx.ExecContext(ctx, tagQuery, id, tagID); err != nil {
				return nil, fmt.Errorf("failed to add tag: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Load tags
	if err := r.loadTags(ctx, &checkin); err != nil {
		return nil, err
	}

	return &checkin, nil
}

// Delete soft deletes a check-in
func (r *Repository) Delete(ctx context.Context, id, userID uuid.UUID) error {
	query := `
		UPDATE checkins
		SET deleted_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query, id, userID)
	if err != nil {
		return fmt.Errorf("failed to delete check-in: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("check-in not found")
	}

	return nil
}

// loadTags loads tags for a check-in
func (r *Repository) loadTags(ctx context.Context, checkin *Checkin) error {
	query := `
		SELECT t.name
		FROM tags t
		INNER JOIN checkin_tags ct ON ct.tag_id = t.id
		WHERE ct.checkin_id = $1
		ORDER BY t.name
	`

	var tags []string
	if err := r.db.SelectContext(ctx, &tags, query, checkin.ID); err != nil {
		return fmt.Errorf("failed to load tags: %w", err)
	}

	checkin.Tags = tags
	return nil
}

// GetEditHistory retrieves edit history for a check-in (Premium feature)
func (r *Repository) GetEditHistory(ctx context.Context, checkinID, userID uuid.UUID) ([]EditHistory, error) {
	query := `
		SELECT id, checkin_id, user_id, previous_content, previous_category_id, edit_reason, edited_at
		FROM edit_history
		WHERE checkin_id = $1 AND user_id = $2
		ORDER BY edited_at DESC
	`

	var history []EditHistory
	if err := r.db.SelectContext(ctx, &history, query, checkinID, userID); err != nil {
		return nil, fmt.Errorf("failed to get edit history: %w", err)
	}

	return history, nil
}
