package backup

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
)

// WALArchiver handles PostgreSQL Write-Ahead Log (WAL) archiving
type WALArchiver struct {
	config  *Config
	storage *S3Storage
	logger  *zap.Logger
}

// NewWALArchiver creates a new WAL archiver
func NewWALArchiver(config *Config, storage *S3Storage, logger *zap.Logger) *WALArchiver {
	return &WALArchiver{
		config:  config,
		storage: storage,
		logger:  logger,
	}
}

// ArchiveWALFile archives a WAL file to S3
// This function is designed to be called by PostgreSQL's archive_command
func (wa *WALArchiver) ArchiveWALFile(ctx context.Context, walFilePath string) error {
	if !wa.config.WALArchivingEnabled {
		return fmt.Errorf("WAL archiving is not enabled")
	}

	walFileName := filepath.Base(walFilePath)

	wa.logger.Info("Archiving WAL file",
		zap.String("wal_file", walFileName),
		zap.String("path", walFilePath),
	)

	// Verify file exists and is readable
	fileInfo, err := os.Stat(walFilePath)
	if err != nil {
		return fmt.Errorf("failed to stat WAL file: %w", err)
	}

	// Open WAL file
	file, err := os.Open(walFilePath)
	if err != nil {
		return fmt.Errorf("failed to open WAL file: %w", err)
	}
	defer file.Close()

	// Generate S3 key for WAL file
	s3Key := fmt.Sprintf("wal/%s/%s",
		time.Now().Format("2006/01/02"),
		walFileName,
	)

	// Upload to S3
	if err := wa.uploadWALToS3(ctx, file, s3Key, fileInfo.Size()); err != nil {
		return fmt.Errorf("failed to upload WAL file: %w", err)
	}

	wa.logger.Info("WAL file archived successfully",
		zap.String("wal_file", walFileName),
		zap.String("s3_key", s3Key),
		zap.Int64("size", fileInfo.Size()),
	)

	return nil
}

// uploadWALToS3 uploads a WAL file to S3
func (wa *WALArchiver) uploadWALToS3(ctx context.Context, reader io.Reader, s3Key string, size int64) error {
	// Create temporary file for upload
	tmpFile, err := os.CreateTemp("", "wal-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Copy to temp file
	if _, err := io.Copy(tmpFile, reader); err != nil {
		return fmt.Errorf("failed to copy WAL data: %w", err)
	}

	// Reset file pointer
	if _, err := tmpFile.Seek(0, 0); err != nil {
		return fmt.Errorf("failed to seek temp file: %w", err)
	}

	// Upload using S3Storage (simplified version)
	// In a real implementation, you would use the S3Storage.UploadBackup method
	// or create a specific method for WAL uploads
	return fmt.Errorf("S3 upload not implemented in this simplified version")
}

// RestoreWALFile restores a WAL file from S3
// This function is designed to be called by PostgreSQL's restore_command
func (wa *WALArchiver) RestoreWALFile(ctx context.Context, walFileName, targetPath string) error {
	wa.logger.Info("Restoring WAL file",
		zap.String("wal_file", walFileName),
		zap.String("target_path", targetPath),
	)

	// Search for the WAL file in S3
	s3Key, err := wa.findWALInS3(ctx, walFileName)
	if err != nil {
		return fmt.Errorf("failed to find WAL file: %w", err)
	}

	// Download from S3
	if err := wa.storage.DownloadBackup(ctx, s3Key, targetPath); err != nil {
		return fmt.Errorf("failed to download WAL file: %w", err)
	}

	wa.logger.Info("WAL file restored successfully",
		zap.String("wal_file", walFileName),
		zap.String("s3_key", s3Key),
	)

	return nil
}

// findWALInS3 searches for a WAL file in S3
func (wa *WALArchiver) findWALInS3(ctx context.Context, walFileName string) (string, error) {
	// List all WAL files
	objects, err := wa.storage.ListBackups(ctx, "wal/")
	if err != nil {
		return "", err
	}

	// Find the matching WAL file
	for _, obj := range objects {
		if obj.Key == nil {
			continue
		}
		if strings.HasSuffix(*obj.Key, walFileName) {
			return *obj.Key, nil
		}
	}

	return "", fmt.Errorf("WAL file not found: %s", walFileName)
}

// ListArchivedWALFiles lists all archived WAL files
func (wa *WALArchiver) ListArchivedWALFiles(ctx context.Context) ([]string, error) {
	objects, err := wa.storage.ListBackups(ctx, "wal/")
	if err != nil {
		return nil, err
	}

	var walFiles []string
	for _, obj := range objects {
		if obj.Key != nil {
			walFiles = append(walFiles, *obj.Key)
		}
	}

	return walFiles, nil
}

// CleanupOldWALFiles removes WAL files older than the specified retention period
func (wa *WALArchiver) CleanupOldWALFiles(ctx context.Context, retentionDays int) error {
	wa.logger.Info("Cleaning up old WAL files",
		zap.Int("retention_days", retentionDays),
	)

	objects, err := wa.storage.ListBackups(ctx, "wal/")
	if err != nil {
		return err
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	var toDelete []string

	for _, obj := range objects {
		if obj.Key == nil || obj.LastModified == nil {
			continue
		}

		if obj.LastModified.Before(cutoff) {
			toDelete = append(toDelete, *obj.Key)
		}
	}

	if len(toDelete) > 0 {
		if err := wa.storage.DeleteMultipleBackups(ctx, toDelete); err != nil {
			return fmt.Errorf("failed to delete old WAL files: %w", err)
		}

		wa.logger.Info("Deleted old WAL files",
			zap.Int("count", len(toDelete)),
		)
	} else {
		wa.logger.Info("No old WAL files to delete")
	}

	return nil
}

// GenerateArchiveCommand generates the PostgreSQL archive_command
func (wa *WALArchiver) GenerateArchiveCommand(scriptPath string) string {
	return fmt.Sprintf("%s archive %%p %%f", scriptPath)
}

// GenerateRestoreCommand generates the PostgreSQL restore_command
func (wa *WALArchiver) GenerateRestoreCommand(scriptPath string) string {
	return fmt.Sprintf("%s restore %%f %%p", scriptPath)
}

// GetWALArchiveStats returns statistics about WAL archiving
func (wa *WALArchiver) GetWALArchiveStats(ctx context.Context) (map[string]interface{}, error) {
	objects, err := wa.storage.ListBackups(ctx, "wal/")
	if err != nil {
		return nil, err
	}

	var totalSize int64
	var oldestWAL, newestWAL time.Time

	for _, obj := range objects {
		if obj.Size != nil {
			totalSize += *obj.Size
		}
		if obj.LastModified != nil {
			if oldestWAL.IsZero() || obj.LastModified.Before(oldestWAL) {
				oldestWAL = *obj.LastModified
			}
			if newestWAL.IsZero() || obj.LastModified.After(newestWAL) {
				newestWAL = *obj.LastModified
			}
		}
	}

	stats := map[string]interface{}{
		"total_wal_files":  len(objects),
		"total_size_bytes": totalSize,
	}

	if !oldestWAL.IsZero() {
		stats["oldest_wal"] = oldestWAL
	}
	if !newestWAL.IsZero() {
		stats["newest_wal"] = newestWAL
	}

	return stats, nil
}
