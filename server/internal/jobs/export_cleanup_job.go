package jobs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// ExportCleanupJob handles cleanup of expired data export files
type ExportCleanupJob struct {
	db             *sqlx.DB
	storageService StorageService
	logger         *zap.Logger
}

// StorageService interface for file operations
type StorageService interface {
	DeleteFile(ctx context.Context, fileURL string) error
}

// NewExportCleanupJob creates a new export cleanup job processor
func NewExportCleanupJob(db *sqlx.DB, logger *zap.Logger) *ExportCleanupJob {
	return &ExportCleanupJob{
		db:     db,
		logger: logger,
	}
}

// SetStorageService sets the storage service for file operations
func (j *ExportCleanupJob) SetStorageService(storage StorageService) {
	j.storageService = storage
}

// Run processes cleanup of expired exports
func (j *ExportCleanupJob) Run(ctx context.Context) error {
	j.logger.Info("Starting export cleanup job")

	// Clean up expired exports
	if err := j.cleanupExpiredExports(ctx); err != nil {
		j.logger.Error("Failed to cleanup expired exports", zap.Error(err))
	}

	// Clean up failed exports older than 24 hours
	if err := j.cleanupFailedExports(ctx); err != nil {
		j.logger.Error("Failed to cleanup failed exports", zap.Error(err))
	}

	// Clean up orphaned export files
	if err := j.cleanupOrphanedFiles(ctx); err != nil {
		j.logger.Error("Failed to cleanup orphaned files", zap.Error(err))
	}

	return nil
}

// cleanupExpiredExports removes expired export files and records
func (j *ExportCleanupJob) cleanupExpiredExports(ctx context.Context) error {
	// Get expired exports
	var expiredExports []struct {
		ID      string  `db:"id"`
		FileURL *string `db:"file_url"`
	}

	err := j.db.SelectContext(ctx, &expiredExports,
		`SELECT id, file_url
		 FROM data_export_requests
		 WHERE status = 'completed'
		   AND expires_at < CURRENT_TIMESTAMP`)
	if err != nil {
		return fmt.Errorf("failed to get expired exports: %w", err)
	}

	if len(expiredExports) == 0 {
		j.logger.Debug("No expired exports to cleanup")
		return nil
	}

	j.logger.Info("Cleaning up expired exports", zap.Int("count", len(expiredExports)))

	for _, export := range expiredExports {
		// Delete file from storage if exists
		if export.FileURL != nil && j.storageService != nil {
			if err := j.storageService.DeleteFile(ctx, *export.FileURL); err != nil {
				j.logger.Warn("Failed to delete export file",
					zap.String("export_id", export.ID),
					zap.String("file_url", *export.FileURL),
					zap.Error(err))
			}
		}

		// Update status to expired
		_, err := j.db.ExecContext(ctx,
			`UPDATE data_export_requests
			 SET status = 'expired',
			     file_url = NULL
			 WHERE id = $1`,
			export.ID)
		if err != nil {
			j.logger.Error("Failed to update export status",
				zap.String("export_id", export.ID),
				zap.Error(err))
		}
	}

	// Delete old expired records (older than 30 days)
	result, err := j.db.ExecContext(ctx,
		`DELETE FROM data_export_requests
		 WHERE status = 'expired'
		   AND expires_at < CURRENT_TIMESTAMP - INTERVAL '30 days'`)
	if err != nil {
		j.logger.Warn("Failed to delete old expired export records", zap.Error(err))
	} else {
		rowsDeleted, _ := result.RowsAffected()
		if rowsDeleted > 0 {
			j.logger.Info("Deleted old expired export records", zap.Int64("count", rowsDeleted))
		}
	}

	return nil
}

// cleanupFailedExports removes old failed export requests
func (j *ExportCleanupJob) cleanupFailedExports(ctx context.Context) error {
	// Delete failed exports older than 24 hours
	result, err := j.db.ExecContext(ctx,
		`DELETE FROM data_export_requests
		 WHERE status = 'failed'
		   AND requested_at < CURRENT_TIMESTAMP - INTERVAL '24 hours'`)
	if err != nil {
		return fmt.Errorf("failed to delete old failed exports: %w", err)
	}

	rowsDeleted, _ := result.RowsAffected()
	if rowsDeleted > 0 {
		j.logger.Info("Deleted old failed export requests", zap.Int64("count", rowsDeleted))
	}

	return nil
}

// cleanupOrphanedFiles removes export files that no longer have database records
func (j *ExportCleanupJob) cleanupOrphanedFiles(ctx context.Context) error {
	// This would typically involve:
	// 1. Listing all files in the export storage location
	// 2. Checking each file against the database
	// 3. Deleting files that don't have corresponding records

	// For now, we'll just log that this should be implemented
	j.logger.Debug("Orphaned file cleanup not yet implemented")
	return nil
}

// GetCleanupStats returns statistics about export cleanup
func (j *ExportCleanupJob) GetCleanupStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Count exports by status
	var statusCounts []struct {
		Status string `db:"status"`
		Count  int    `db:"count"`
	}

	err := j.db.SelectContext(ctx, &statusCounts,
		`SELECT status, COUNT(*) as count
		 FROM data_export_requests
		 GROUP BY status`)
	if err == nil {
		statusMap := make(map[string]int)
		for _, sc := range statusCounts {
			statusMap[sc.Status] = sc.Count
		}
		stats["exports_by_status"] = statusMap
	}

	// Count exports expiring soon (within 24 hours)
	var expiringSoon int
	err = j.db.GetContext(ctx, &expiringSoon,
		`SELECT COUNT(*)
		 FROM data_export_requests
		 WHERE status = 'completed'
		   AND expires_at BETWEEN CURRENT_TIMESTAMP AND CURRENT_TIMESTAMP + INTERVAL '24 hours'`)
	if err == nil {
		stats["expiring_soon"] = expiringSoon
	}

	// Calculate total storage used (approximate)
	var totalSize int64
	err = j.db.GetContext(ctx, &totalSize,
		`SELECT COALESCE(SUM(
		   CASE
		     WHEN format = 'json' THEN 100000  -- Estimate 100KB for JSON
		     WHEN format = 'csv' THEN 50000    -- Estimate 50KB for CSV
		     WHEN format = 'pdf' THEN 200000   -- Estimate 200KB for PDF
		     ELSE 100000
		   END
		 ), 0)
		 FROM data_export_requests
		 WHERE status = 'completed' AND file_url IS NOT NULL`)
	if err == nil {
		stats["estimated_storage_bytes"] = totalSize
		stats["estimated_storage_mb"] = fmt.Sprintf("%.2f", float64(totalSize)/1024/1024)
	}

	return stats, nil
}

// SendExportExpirationNotifications sends notifications for exports expiring soon
func (j *ExportCleanupJob) SendExportExpirationNotifications(ctx context.Context) error {
	// Get exports expiring in the next 24 hours
	var expiringExports []struct {
		UserID    string    `db:"user_id"`
		Email     string    `db:"email"`
		ExportID  string    `db:"id"`
		ExpiresAt time.Time `db:"expires_at"`
		Format    string    `db:"format"`
	}

	err := j.db.SelectContext(ctx, &expiringExports,
		`SELECT der.id as id, der.user_id, u.email, der.expires_at, der.format
		 FROM data_export_requests der
		 JOIN users u ON der.user_id = u.id
		 WHERE der.status = 'completed'
		   AND der.expires_at BETWEEN CURRENT_TIMESTAMP + INTERVAL '23 hours'
		                         AND CURRENT_TIMESTAMP + INTERVAL '25 hours'
		   AND der.file_url IS NOT NULL`)
	if err != nil {
		return fmt.Errorf("failed to get expiring exports: %w", err)
	}

	for _, export := range expiringExports {
		hoursUntilExpiry := int(time.Until(export.ExpiresAt).Hours())
		j.logger.Info("Sending export expiration notification",
			zap.String("user_id", export.UserID),
			zap.String("email", export.Email),
			zap.String("export_id", export.ExportID),
			zap.String("format", strings.ToUpper(export.Format)),
			zap.Int("hours_until_expiry", hoursUntilExpiry))

		// TODO: Send actual email notification
		// notificationService.SendExportExpirationWarning(export.Email, export.ExportID, hoursUntilExpiry)
	}

	return nil
}