package export

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// Repository handles export job database operations
type Repository struct {
	db *sqlx.DB
}

// NewRepository creates a new export repository
func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

// Create creates a new export job
func (r *Repository) Create(ctx context.Context, job *ExportJob) error {
	query := `
		INSERT INTO export_jobs (
			id, user_id, format, status,
			record_count, expires_at, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7
		)
	`

	_, err := r.db.ExecContext(ctx, query,
		job.ID,
		job.UserID,
		job.Format,
		job.Status,
		job.RecordCount,
		job.ExpiresAt,
		job.CreatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create export job: %w", err)
	}

	return nil
}

// Update updates an export job
func (r *Repository) Update(ctx context.Context, job *ExportJob) error {
	query := `
		UPDATE export_jobs
		SET status = $2,
			file_path = $3,
			file_size = $4,
			record_count = $5,
			error = $6,
			started_at = $7,
			completed_at = $8
		WHERE id = $1
	`

	_, err := r.db.ExecContext(ctx, query,
		job.ID,
		job.Status,
		job.FilePath,
		job.FileSize,
		job.RecordCount,
		job.Error,
		job.StartedAt,
		job.CompletedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to update export job: %w", err)
	}

	return nil
}

// GetByID retrieves an export job by ID
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*ExportJob, error) {
	var job ExportJob

	query := `
		SELECT id, user_id, format, status, file_path, file_size,
			   record_count, error, started_at, completed_at,
			   expires_at, created_at
		FROM export_jobs
		WHERE id = $1
	`

	err := r.db.GetContext(ctx, &job, query, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get export job: %w", err)
	}

	return &job, nil
}

// ListByUserID retrieves export jobs for a user
func (r *Repository) ListByUserID(ctx context.Context, userID uuid.UUID, limit int) ([]*ExportJob, error) {
	var jobs []*ExportJob

	query := `
		SELECT id, user_id, format, status, file_path, file_size,
			   record_count, error, started_at, completed_at,
			   expires_at, created_at
		FROM export_jobs
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	err := r.db.SelectContext(ctx, &jobs, query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list export jobs: %w", err)
	}

	return jobs, nil
}

// DeleteExpired deletes expired export jobs
func (r *Repository) DeleteExpired(ctx context.Context) (int64, error) {
	query := `
		DELETE FROM export_jobs
		WHERE expires_at < NOW()
	`

	result, err := r.db.ExecContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired jobs: %w", err)
	}

	count, _ := result.RowsAffected()
	return count, nil
}
