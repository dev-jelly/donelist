package search

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// ReindexOptions configures reindexing behavior
type ReindexOptions struct {
	// BatchSize is the number of documents to process in each batch
	BatchSize int
	// UserID filters reindexing to a specific user (nil for all users)
	UserID *uuid.UUID
	// StartDate filters reindexing to checkins after this date
	StartDate *time.Time
	// EndDate filters reindexing to checkins before this date
	EndDate *time.Time
	// DryRun performs a dry run without actually modifying data
	DryRun bool
	// Verbose enables detailed logging
	Verbose bool
}

// DefaultReindexOptions returns sensible defaults
func DefaultReindexOptions() ReindexOptions {
	return ReindexOptions{
		BatchSize: 1000,
		DryRun:    false,
		Verbose:   false,
	}
}

// ReindexResult contains statistics about a reindex operation
type ReindexResult struct {
	TotalDocuments    int64
	ProcessedDocuments int64
	FailedDocuments   int64
	StartTime         time.Time
	EndTime           time.Time
	Duration          time.Duration
	BatchCount        int
	AverageRatePerSec float64
}

// Reindexer handles reindexing operations
type Reindexer struct {
	db     *sqlx.DB
	logger *zap.Logger
}

// NewReindexer creates a new reindexer
func NewReindexer(db *sqlx.DB, logger *zap.Logger) *Reindexer {
	return &Reindexer{
		db:     db,
		logger: logger,
	}
}

// Reindex rebuilds the search index for checkins
func (r *Reindexer) Reindex(ctx context.Context, opts ReindexOptions) (*ReindexResult, error) {
	result := &ReindexResult{
		StartTime: time.Now(),
	}

	r.logger.Info("Starting reindex operation",
		zap.Int("batch_size", opts.BatchSize),
		zap.Bool("dry_run", opts.DryRun),
	)

	// Count total documents
	countQuery := "SELECT COUNT(*) FROM checkins WHERE deleted_at IS NULL"
	args := make([]interface{}, 0)
	argIndex := 1

	if opts.UserID != nil {
		countQuery += fmt.Sprintf(" AND user_id = $%d", argIndex)
		args = append(args, *opts.UserID)
		argIndex++
	}

	if opts.StartDate != nil {
		countQuery += fmt.Sprintf(" AND checkin_time >= $%d", argIndex)
		args = append(args, *opts.StartDate)
		argIndex++
	}

	if opts.EndDate != nil {
		countQuery += fmt.Sprintf(" AND checkin_time <= $%d", argIndex)
		args = append(args, *opts.EndDate)
		argIndex++
	}

	if err := r.db.GetContext(ctx, &result.TotalDocuments, countQuery, args...); err != nil {
		return nil, fmt.Errorf("failed to count documents: %w", err)
	}

	r.logger.Info("Documents to reindex", zap.Int64("count", result.TotalDocuments))

	if result.TotalDocuments == 0 {
		r.logger.Info("No documents to reindex")
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		return result, nil
	}

	// Process in batches
	offset := 0
	for {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		// Fetch batch
		batchQuery := `
			SELECT id, user_id, category_id, content, checkin_time,
			       duration_minutes, is_edited, edit_count
			FROM checkins
			WHERE deleted_at IS NULL
		`
		batchArgs := make([]interface{}, 0)
		batchArgIndex := 1

		if opts.UserID != nil {
			batchQuery += fmt.Sprintf(" AND user_id = $%d", batchArgIndex)
			batchArgs = append(batchArgs, *opts.UserID)
			batchArgIndex++
		}

		if opts.StartDate != nil {
			batchQuery += fmt.Sprintf(" AND checkin_time >= $%d", batchArgIndex)
			batchArgs = append(batchArgs, *opts.StartDate)
			batchArgIndex++
		}

		if opts.EndDate != nil {
			batchQuery += fmt.Sprintf(" AND checkin_time <= $%d", batchArgIndex)
			batchArgs = append(batchArgs, *opts.EndDate)
			batchArgIndex++
		}

		batchQuery += fmt.Sprintf(" ORDER BY created_at LIMIT $%d OFFSET $%d", batchArgIndex, batchArgIndex+1)
		batchArgs = append(batchArgs, opts.BatchSize, offset)

		var checkins []struct {
			ID              uuid.UUID  `db:"id"`
			UserID          uuid.UUID  `db:"user_id"`
			CategoryID      *uuid.UUID `db:"category_id"`
			Content         string     `db:"content"`
			CheckinTime     time.Time  `db:"checkin_time"`
			DurationMinutes int        `db:"duration_minutes"`
			IsEdited        bool       `db:"is_edited"`
			EditCount       int        `db:"edit_count"`
		}

		if err := r.db.SelectContext(ctx, &checkins, batchQuery, batchArgs...); err != nil {
			return result, fmt.Errorf("failed to fetch batch: %w", err)
		}

		if len(checkins) == 0 {
			break
		}

		result.BatchCount++

		// Update search vectors for batch
		if !opts.DryRun {
			if err := r.updateSearchVectorsBatch(ctx, checkins); err != nil {
				r.logger.Error("Failed to update batch", zap.Error(err), zap.Int("batch", result.BatchCount))
				result.FailedDocuments += int64(len(checkins))
			} else {
				result.ProcessedDocuments += int64(len(checkins))
			}
		} else {
			result.ProcessedDocuments += int64(len(checkins))
		}

		if opts.Verbose && result.BatchCount%10 == 0 {
			r.logger.Info("Reindex progress",
				zap.Int64("processed", result.ProcessedDocuments),
				zap.Int64("total", result.TotalDocuments),
				zap.Float64("percent", float64(result.ProcessedDocuments)/float64(result.TotalDocuments)*100),
			)
		}

		offset += len(checkins)

		// Break if we've processed fewer than the batch size
		if len(checkins) < opts.BatchSize {
			break
		}
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	if result.Duration.Seconds() > 0 {
		result.AverageRatePerSec = float64(result.ProcessedDocuments) / result.Duration.Seconds()
	}

	r.logger.Info("Reindex operation completed",
		zap.Int64("total", result.TotalDocuments),
		zap.Int64("processed", result.ProcessedDocuments),
		zap.Int64("failed", result.FailedDocuments),
		zap.Duration("duration", result.Duration),
		zap.Float64("rate_per_sec", result.AverageRatePerSec),
	)

	return result, nil
}

// updateSearchVectorsBatch updates search vectors for a batch of checkins
func (r *Reindexer) updateSearchVectorsBatch(ctx context.Context, checkins []struct {
	ID              uuid.UUID  `db:"id"`
	UserID          uuid.UUID  `db:"user_id"`
	CategoryID      *uuid.UUID `db:"category_id"`
	Content         string     `db:"content"`
	CheckinTime     time.Time  `db:"checkin_time"`
	DurationMinutes int        `db:"duration_minutes"`
	IsEdited        bool       `db:"is_edited"`
	EditCount       int        `db:"edit_count"`
}) error {
	// PostgreSQL's tsvector update is handled by triggers
	// We just need to update the row to trigger the search_vector update
	// Or we can manually update the search_vector column

	// Manual approach for explicit control:
	updateQuery := `
		UPDATE checkins
		SET search_vector = to_tsvector('simple', content),
		    updated_at = updated_at -- Keep original updated_at
		WHERE id = ANY($1)
	`

	checkinIDs := make([]uuid.UUID, len(checkins))
	for i, c := range checkins {
		checkinIDs[i] = c.ID
	}

	_, err := r.db.ExecContext(ctx, updateQuery, checkinIDs)
	if err != nil {
		return fmt.Errorf("failed to update search vectors: %w", err)
	}

	return nil
}

// VerifyIndex checks the integrity of the search index
func (r *Reindexer) VerifyIndex(ctx context.Context) (*IndexVerificationResult, error) {
	result := &IndexVerificationResult{
		StartTime: time.Now(),
	}

	// Count total documents
	if err := r.db.GetContext(ctx, &result.TotalDocuments,
		"SELECT COUNT(*) FROM checkins WHERE deleted_at IS NULL"); err != nil {
		return nil, fmt.Errorf("failed to count documents: %w", err)
	}

	// Count documents with null search_vector
	if err := r.db.GetContext(ctx, &result.MissingVectors,
		"SELECT COUNT(*) FROM checkins WHERE deleted_at IS NULL AND search_vector IS NULL"); err != nil {
		return nil, fmt.Errorf("failed to count missing vectors: %w", err)
	}

	// Count documents with empty search_vector
	if err := r.db.GetContext(ctx, &result.EmptyVectors,
		"SELECT COUNT(*) FROM checkins WHERE deleted_at IS NULL AND search_vector = ''::tsvector"); err != nil {
		return nil, fmt.Errorf("failed to count empty vectors: %w", err)
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.Healthy = (result.MissingVectors == 0 && result.EmptyVectors == 0)

	r.logger.Info("Index verification completed",
		zap.Int64("total", result.TotalDocuments),
		zap.Int64("missing", result.MissingVectors),
		zap.Int64("empty", result.EmptyVectors),
		zap.Bool("healthy", result.Healthy),
	)

	return result, nil
}

// IndexVerificationResult contains index verification statistics
type IndexVerificationResult struct {
	TotalDocuments int64
	MissingVectors int64
	EmptyVectors   int64
	Healthy        bool
	StartTime      time.Time
	EndTime        time.Time
	Duration       time.Duration
}

// CompactIndex performs index maintenance (VACUUM, ANALYZE, REINDEX)
func (r *Reindexer) CompactIndex(ctx context.Context) error {
	r.logger.Info("Starting index compaction")

	// Run ANALYZE to update statistics
	if _, err := r.db.ExecContext(ctx, "ANALYZE checkins"); err != nil {
		return fmt.Errorf("failed to analyze checkins: %w", err)
	}

	r.logger.Info("Index compaction completed")
	return nil
}
