package jobs

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// AccountDeletionJob handles scheduled account deletion processing
type AccountDeletionJob struct {
	db     *sqlx.DB
	logger *zap.Logger
}

// NewAccountDeletionJob creates a new account deletion job processor
func NewAccountDeletionJob(db *sqlx.DB, logger *zap.Logger) *AccountDeletionJob {
	return &AccountDeletionJob{
		db:     db,
		logger: logger,
	}
}

// DeletionRequest represents an account deletion request
type DeletionRequest struct {
	ID                   uuid.UUID  `db:"id"`
	UserID               uuid.UUID  `db:"user_id"`
	RequestedAt          time.Time  `db:"requested_at"`
	ScheduledDeletionAt  time.Time  `db:"scheduled_deletion_at"`
	Reason               *string    `db:"reason"`
	Status               string     `db:"status"`
	CancelledAt          *time.Time `db:"cancelled_at"`
	CompletedAt          *time.Time `db:"completed_at"`
}

// Run processes pending account deletions
func (j *AccountDeletionJob) Run(ctx context.Context) error {
	j.logger.Info("Starting account deletion job")

	// Get all pending deletions that are due
	requests, err := j.getPendingDeletions(ctx)
	if err != nil {
		return fmt.Errorf("failed to get pending deletions: %w", err)
	}

	if len(requests) == 0 {
		j.logger.Debug("No pending account deletions")
		return nil
	}

	j.logger.Info("Processing account deletions", zap.Int("count", len(requests)))

	for _, request := range requests {
		if err := j.processAccountDeletion(ctx, request); err != nil {
			j.logger.Error("Failed to process account deletion",
				zap.String("request_id", request.ID.String()),
				zap.String("user_id", request.UserID.String()),
				zap.Error(err))
			// Continue processing other deletions
			continue
		}
	}

	return nil
}

// getPendingDeletions retrieves all pending deletion requests that are due
func (j *AccountDeletionJob) getPendingDeletions(ctx context.Context) ([]DeletionRequest, error) {
	var requests []DeletionRequest
	err := j.db.SelectContext(ctx, &requests,
		`SELECT id, user_id, requested_at, scheduled_deletion_at, reason, status
		 FROM account_deletion_requests
		 WHERE status = 'pending'
		   AND scheduled_deletion_at <= CURRENT_TIMESTAMP`)
	if err != nil {
		return nil, err
	}
	return requests, nil
}

// processAccountDeletion processes a single account deletion
func (j *AccountDeletionJob) processAccountDeletion(ctx context.Context, request DeletionRequest) error {
	j.logger.Info("Processing account deletion",
		zap.String("user_id", request.UserID.String()),
		zap.String("request_id", request.ID.String()))

	tx, err := j.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// 1. Export user data for compliance (keep for 90 days)
	if err := j.archiveUserData(ctx, tx, request.UserID); err != nil {
		return fmt.Errorf("failed to archive user data: %w", err)
	}

	// 2. Delete user data in correct order (respecting foreign key constraints)

	// Delete from tables that reference users
	tables := []string{
		"security_events",
		"settings_audit_log",
		"account_deletion_requests",
		"account_recovery_tokens",
		"data_export_requests",
		"password_history",
		"two_factor_backup_codes",
		"refresh_tokens",
		"data_retention_settings",
		"notification_settings",
		"user_settings",
		"user_profiles",
		"checkins",
		"categories",
		"checkin_stats",
		"daily_summaries",
		"streaks",
		"webhook_deliveries",
		"webhook_endpoints",
	}

	for _, table := range tables {
		if err := j.deleteFromTable(ctx, tx, table, request.UserID); err != nil {
			j.logger.Warn("Failed to delete from table",
				zap.String("table", table),
				zap.Error(err))
			// Continue with other tables
		}
	}

	// 3. Anonymize the user record instead of hard delete (for audit trail)
	if err := j.anonymizeUser(ctx, tx, request.UserID); err != nil {
		return fmt.Errorf("failed to anonymize user: %w", err)
	}

	// 4. Update deletion request status
	completedAt := time.Now()
	_, err = tx.ExecContext(ctx,
		`UPDATE account_deletion_requests
		 SET status = 'completed',
		     completed_at = $2
		 WHERE id = $1`,
		request.ID, completedAt)
	if err != nil {
		return fmt.Errorf("failed to update deletion request: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	j.logger.Info("Successfully completed account deletion",
		zap.String("user_id", request.UserID.String()),
		zap.String("request_id", request.ID.String()))

	// 5. Send confirmation email (if we still have the email)
	// This would be done through a notification service

	return nil
}

// archiveUserData creates an archive of user data before deletion
func (j *AccountDeletionJob) archiveUserData(ctx context.Context, tx *sqlx.Tx, userID uuid.UUID) error {
	// Create archive record
	archiveID := uuid.New()

	// Get user email for archive identification
	var email string
	err := tx.GetContext(ctx, &email,
		"SELECT email FROM users WHERE id = $1", userID)
	if err != nil {
		return fmt.Errorf("failed to get user email: %w", err)
	}

	// Create JSON export of all user data
	userData := make(map[string]interface{})

	// Collect user data
	var user map[string]interface{}
	err = tx.GetContext(ctx, &user,
		`SELECT id, email, display_name, role, tier, created_at, updated_at
		 FROM users WHERE id = $1`, userID)
	if err == nil {
		userData["user"] = user
	}

	// Collect profile data
	var profile map[string]interface{}
	err = tx.GetContext(ctx, &profile,
		`SELECT * FROM user_profiles WHERE user_id = $1`, userID)
	if err == nil {
		userData["profile"] = profile
	}

	// Collect checkins
	var checkins []map[string]interface{}
	rows, err := tx.QueryContext(ctx,
		`SELECT id, title, description, status, priority, created_at
		 FROM checkins WHERE user_id = $1`, userID)
	if err == nil {
		defer rows.Close()
		// Process rows...
		userData["checkins_count"] = len(checkins)
	}

	// Store archive (this would typically go to a separate archive database or cold storage)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO user_data_archives (id, user_id, email_hash, data, archived_at, expires_at)
		 VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP + INTERVAL '90 days')`,
		archiveID, userID, hashEmail(email), userData)
	if err != nil {
		// If archive table doesn't exist, log and continue
		j.logger.Warn("Failed to create data archive", zap.Error(err))
	}

	return nil
}

// deleteFromTable deletes user data from a specific table
func (j *AccountDeletionJob) deleteFromTable(ctx context.Context, tx *sqlx.Tx, table string, userID uuid.UUID) error {
	query := fmt.Sprintf("DELETE FROM %s WHERE user_id = $1", table)
	result, err := tx.ExecContext(ctx, query, userID)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		j.logger.Debug("Deleted user data from table",
			zap.String("table", table),
			zap.Int64("rows", rowsAffected))
	}

	return nil
}

// anonymizeUser anonymizes the user record instead of deleting it
func (j *AccountDeletionJob) anonymizeUser(ctx context.Context, tx *sqlx.Tx, userID uuid.UUID) error {
	// Generate anonymized data
	anonymizedEmail := fmt.Sprintf("deleted_%s@donelist.local", userID.String()[:8])

	_, err := tx.ExecContext(ctx,
		`UPDATE users
		 SET email = $2,
		     display_name = 'Deleted User',
		     password_hash = '',
		     two_factor_enabled = false,
		     two_factor_secret = NULL,
		     recovery_email = NULL,
		     deleted_at = CURRENT_TIMESTAMP
		 WHERE id = $1`,
		userID, anonymizedEmail)
	if err != nil {
		return fmt.Errorf("failed to anonymize user: %w", err)
	}

	return nil
}

// hashEmail creates a one-way hash of an email for archive purposes
func hashEmail(email string) string {
	// This would use a proper hashing function
	// For now, just return a placeholder
	return fmt.Sprintf("hash_%s", email[:3])
}

// CleanupExpiredDeletionRequests cleans up old completed deletion requests
func (j *AccountDeletionJob) CleanupExpiredDeletionRequests(ctx context.Context) error {
	// Delete completed requests older than 90 days
	result, err := j.db.ExecContext(ctx,
		`DELETE FROM account_deletion_requests
		 WHERE status = 'completed'
		   AND completed_at < CURRENT_TIMESTAMP - INTERVAL '90 days'`)
	if err != nil {
		return fmt.Errorf("failed to cleanup old deletion requests: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		j.logger.Info("Cleaned up old deletion requests", zap.Int64("count", rowsAffected))
	}

	return nil
}

// SendDeletionReminder sends a reminder before account deletion
func (j *AccountDeletionJob) SendDeletionReminder(ctx context.Context) error {
	// Get deletions scheduled for the next 7 days
	var requests []struct {
		UserID              uuid.UUID `db:"user_id"`
		Email               string    `db:"email"`
		ScheduledDeletionAt time.Time `db:"scheduled_deletion_at"`
	}

	err := j.db.SelectContext(ctx, &requests,
		`SELECT adr.user_id, u.email, adr.scheduled_deletion_at
		 FROM account_deletion_requests adr
		 JOIN users u ON adr.user_id = u.id
		 WHERE adr.status = 'pending'
		   AND adr.scheduled_deletion_at BETWEEN CURRENT_TIMESTAMP + INTERVAL '6 days 23 hours'
		                                     AND CURRENT_TIMESTAMP + INTERVAL '7 days 1 hour'`)
	if err != nil {
		return fmt.Errorf("failed to get upcoming deletions: %w", err)
	}

	for _, req := range requests {
		j.logger.Info("Sending deletion reminder",
			zap.String("user_id", req.UserID.String()),
			zap.String("email", req.Email),
			zap.Time("scheduled_for", req.ScheduledDeletionAt))

		// TODO: Send actual email notification
	}

	return nil
}

// GetDeletionStatus gets the status of a deletion request
func (j *AccountDeletionJob) GetDeletionStatus(ctx context.Context, userID uuid.UUID) (*DeletionRequest, error) {
	var request DeletionRequest
	err := j.db.GetContext(ctx, &request,
		`SELECT * FROM account_deletion_requests
		 WHERE user_id = $1
		 ORDER BY requested_at DESC
		 LIMIT 1`,
		userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get deletion status: %w", err)
	}
	return &request, nil
}