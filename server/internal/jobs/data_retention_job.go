package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// DataRetentionJob handles data retention policies and automatic cleanup
type DataRetentionJob struct {
	db     *sqlx.DB
	logger *zap.Logger
}

// NewDataRetentionJob creates a new data retention job processor
func NewDataRetentionJob(db *sqlx.DB, logger *zap.Logger) *DataRetentionJob {
	return &DataRetentionJob{
		db:     db,
		logger: logger,
	}
}

// Run processes data retention policies for all users
func (j *DataRetentionJob) Run(ctx context.Context) error {
	j.logger.Info("Starting data retention job")

	// Process users with auto-delete enabled
	if err := j.processAutoDeletePolicies(ctx); err != nil {
		j.logger.Error("Failed to process auto-delete policies", zap.Error(err))
	}

	// Process inactive account cleanup
	if err := j.processInactiveAccounts(ctx); err != nil {
		j.logger.Error("Failed to process inactive accounts", zap.Error(err))
	}

	// Clean up old security events
	if err := j.cleanupSecurityEvents(ctx); err != nil {
		j.logger.Error("Failed to cleanup security events", zap.Error(err))
	}

	// Clean up expired sessions and tokens
	if err := j.cleanupExpiredTokens(ctx); err != nil {
		j.logger.Error("Failed to cleanup expired tokens", zap.Error(err))
	}

	return nil
}

// processAutoDeletePolicies handles automatic data deletion based on retention settings
func (j *DataRetentionJob) processAutoDeletePolicies(ctx context.Context) error {
	// Get all users with auto-delete enabled
	var policies []struct {
		UserID        uuid.UUID `db:"user_id"`
		RetentionDays int       `db:"retention_days"`
	}

	err := j.db.SelectContext(ctx, &policies,
		`SELECT user_id, retention_days
		 FROM data_retention_settings
		 WHERE auto_delete_enabled = true
		   AND retention_days IS NOT NULL`)
	if err != nil {
		return fmt.Errorf("failed to get retention policies: %w", err)
	}

	for _, policy := range policies {
		if err := j.applyRetentionPolicy(ctx, policy.UserID, policy.RetentionDays); err != nil {
			j.logger.Error("Failed to apply retention policy",
				zap.String("user_id", policy.UserID.String()),
				zap.Int("retention_days", policy.RetentionDays),
				zap.Error(err))
			continue
		}
	}

	return nil
}

// applyRetentionPolicy deletes old data based on retention policy
func (j *DataRetentionJob) applyRetentionPolicy(ctx context.Context, userID uuid.UUID, retentionDays int) error {
	cutoffDate := time.Now().AddDate(0, 0, -retentionDays)

	tx, err := j.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete old checkins
	result, err := tx.ExecContext(ctx,
		`DELETE FROM checkins
		 WHERE user_id = $1
		   AND created_at < $2
		   AND status = 'completed'`,
		userID, cutoffDate)
	if err != nil {
		return fmt.Errorf("failed to delete old checkins: %w", err)
	}

	checkinsDeleted, _ := result.RowsAffected()

	// Delete old completed tasks
	result, err = tx.ExecContext(ctx,
		`DELETE FROM tasks
		 WHERE user_id = $1
		   AND created_at < $2
		   AND status = 'completed'`,
		userID, cutoffDate)
	if err != nil {
		j.logger.Warn("Failed to delete old tasks", zap.Error(err))
	}

	tasksDeleted, _ := result.RowsAffected()

	// Update last activity timestamp
	_, err = tx.ExecContext(ctx,
		`UPDATE data_retention_settings
		 SET last_activity_at = CURRENT_TIMESTAMP
		 WHERE user_id = $1`,
		userID)
	if err != nil {
		return fmt.Errorf("failed to update last activity: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	if checkinsDeleted > 0 || tasksDeleted > 0 {
		j.logger.Info("Applied retention policy",
			zap.String("user_id", userID.String()),
			zap.Int64("checkins_deleted", checkinsDeleted),
			zap.Int64("tasks_deleted", tasksDeleted))
	}

	return nil
}

// processInactiveAccounts handles deletion of inactive accounts
func (j *DataRetentionJob) processInactiveAccounts(ctx context.Context) error {
	// Get users who have been inactive beyond their threshold
	var inactiveUsers []struct {
		UserID                   uuid.UUID `db:"user_id"`
		Email                    string    `db:"email"`
		LastActivityAt           time.Time `db:"last_activity_at"`
		DeleteAfterInactivityDays int       `db:"delete_after_inactivity_days"`
	}

	err := j.db.SelectContext(ctx, &inactiveUsers,
		`SELECT drs.user_id, u.email, drs.last_activity_at, drs.delete_after_inactivity_days
		 FROM data_retention_settings drs
		 JOIN users u ON drs.user_id = u.id
		 WHERE drs.delete_after_inactivity_days IS NOT NULL
		   AND drs.last_activity_at < CURRENT_TIMESTAMP - (drs.delete_after_inactivity_days || ' days')::INTERVAL
		   AND u.deleted_at IS NULL`)
	if err != nil {
		return fmt.Errorf("failed to get inactive users: %w", err)
	}

	for _, user := range inactiveUsers {
		daysSinceActivity := int(time.Since(user.LastActivityAt).Hours() / 24)
		j.logger.Info("Initiating deletion for inactive account",
			zap.String("user_id", user.UserID.String()),
			zap.String("email", user.Email),
			zap.Int("days_inactive", daysSinceActivity))

		// Create deletion request with 30-day grace period
		_, err := j.db.ExecContext(ctx,
			`INSERT INTO account_deletion_requests (user_id, scheduled_deletion_at, reason)
			 VALUES ($1, CURRENT_TIMESTAMP + INTERVAL '30 days', $2)
			 ON CONFLICT (user_id) WHERE status = 'pending' DO NOTHING`,
			user.UserID, fmt.Sprintf("Account inactive for %d days", daysSinceActivity))
		if err != nil {
			j.logger.Error("Failed to create deletion request",
				zap.String("user_id", user.UserID.String()),
				zap.Error(err))
		}

		// TODO: Send warning email about pending deletion
	}

	return nil
}

// cleanupSecurityEvents removes old security event logs
func (j *DataRetentionJob) cleanupSecurityEvents(ctx context.Context) error {
	// Keep security events for 1 year
	cutoffDate := time.Now().AddDate(-1, 0, 0)

	result, err := j.db.ExecContext(ctx,
		`DELETE FROM security_events
		 WHERE created_at < $1`,
		cutoffDate)
	if err != nil {
		return fmt.Errorf("failed to delete old security events: %w", err)
	}

	rowsDeleted, _ := result.RowsAffected()
	if rowsDeleted > 0 {
		j.logger.Info("Cleaned up old security events", zap.Int64("count", rowsDeleted))
	}

	// Archive important security events before deletion
	_, err = j.db.ExecContext(ctx,
		`INSERT INTO security_events_archive (user_id, event_type, event_details, created_at)
		 SELECT user_id, event_type, event_details, created_at
		 FROM security_events
		 WHERE created_at < $1
		   AND event_type IN ('password_changed', '2fa_enabled', '2fa_disabled', 'account_deletion_requested')
		 ON CONFLICT DO NOTHING`,
		cutoffDate)
	if err != nil {
		j.logger.Warn("Failed to archive security events", zap.Error(err))
	}

	return nil
}

// cleanupExpiredTokens removes expired refresh tokens and recovery tokens
func (j *DataRetentionJob) cleanupExpiredTokens(ctx context.Context) error {
	// Delete expired refresh tokens
	result, err := j.db.ExecContext(ctx,
		`DELETE FROM refresh_tokens
		 WHERE expires_at < CURRENT_TIMESTAMP
		    OR (revoked = true AND revoked_at < CURRENT_TIMESTAMP - INTERVAL '7 days')`)
	if err != nil {
		j.logger.Warn("Failed to delete expired refresh tokens", zap.Error(err))
	} else {
		rowsDeleted, _ := result.RowsAffected()
		if rowsDeleted > 0 {
			j.logger.Debug("Deleted expired refresh tokens", zap.Int64("count", rowsDeleted))
		}
	}

	// Delete expired recovery tokens
	result, err = j.db.ExecContext(ctx,
		`DELETE FROM account_recovery_tokens
		 WHERE expires_at < CURRENT_TIMESTAMP
		    OR (used = true AND used_at < CURRENT_TIMESTAMP - INTERVAL '30 days')`)
	if err != nil {
		j.logger.Warn("Failed to delete expired recovery tokens", zap.Error(err))
	} else {
		rowsDeleted, _ := result.RowsAffected()
		if rowsDeleted > 0 {
			j.logger.Debug("Deleted expired recovery tokens", zap.Int64("count", rowsDeleted))
		}
	}

	// Delete old used backup codes
	result, err = j.db.ExecContext(ctx,
		`DELETE FROM two_factor_backup_codes
		 WHERE used = true
		   AND used_at < CURRENT_TIMESTAMP - INTERVAL '90 days'`)
	if err != nil {
		j.logger.Warn("Failed to delete old backup codes", zap.Error(err))
	} else {
		rowsDeleted, _ := result.RowsAffected()
		if rowsDeleted > 0 {
			j.logger.Debug("Deleted old used backup codes", zap.Int64("count", rowsDeleted))
		}
	}

	return nil
}

// GetRetentionStats returns statistics about data retention
func (j *DataRetentionJob) GetRetentionStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Count users with auto-delete enabled
	var autoDeleteCount int
	err := j.db.GetContext(ctx, &autoDeleteCount,
		`SELECT COUNT(*) FROM data_retention_settings WHERE auto_delete_enabled = true`)
	if err == nil {
		stats["users_with_auto_delete"] = autoDeleteCount
	}

	// Count pending account deletions
	var pendingDeletions int
	err = j.db.GetContext(ctx, &pendingDeletions,
		`SELECT COUNT(*) FROM account_deletion_requests WHERE status = 'pending'`)
	if err == nil {
		stats["pending_deletions"] = pendingDeletions
	}

	// Get average retention days
	var avgRetentionDays float64
	err = j.db.GetContext(ctx, &avgRetentionDays,
		`SELECT AVG(retention_days)
		 FROM data_retention_settings
		 WHERE retention_days IS NOT NULL`)
	if err == nil {
		stats["avg_retention_days"] = avgRetentionDays
	}

	return stats, nil
}