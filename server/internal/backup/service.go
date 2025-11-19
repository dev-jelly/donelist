package backup

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// Service is the main backup service that coordinates all backup operations
type Service struct {
	config          *Config
	postgresBackup  *PostgresBackup
	storage         *S3Storage
	rotationManager *RotationManager
	walArchiver     *WALArchiver
	monitor         *Monitor
	scheduler       *Scheduler
	logger          *zap.Logger
}

// NewService creates a new backup service
func NewService(config *Config, logger *zap.Logger) (*Service, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Initialize components
	postgresBackup := NewPostgresBackup(config, logger)

	storage, err := NewS3Storage(config, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize S3 storage: %w", err)
	}

	rotationManager := NewRotationManager(config, storage, logger)
	walArchiver := NewWALArchiver(config, storage, logger)
	monitor := NewMonitor(config, logger)

	service := &Service{
		config:          config,
		postgresBackup:  postgresBackup,
		storage:         storage,
		rotationManager: rotationManager,
		walArchiver:     walArchiver,
		monitor:         monitor,
		logger:          logger,
	}

	// Initialize scheduler
	scheduler := NewScheduler(config, service, logger)
	service.scheduler = scheduler

	return service, nil
}

// Start starts the backup service
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting backup service")

	// Ensure S3 bucket exists
	if err := s.storage.EnsureBucketExists(ctx); err != nil {
		return fmt.Errorf("failed to ensure S3 bucket exists: %w", err)
	}

	// Start scheduler if enabled
	if s.config.ScheduleEnabled {
		if err := s.scheduler.Start(ctx); err != nil {
			return fmt.Errorf("failed to start scheduler: %w", err)
		}
	}

	s.logger.Info("Backup service started successfully")
	return nil
}

// Stop stops the backup service
func (s *Service) Stop() error {
	s.logger.Info("Stopping backup service")

	if s.scheduler != nil {
		if err := s.scheduler.Stop(); err != nil {
			s.logger.Error("Failed to stop scheduler", zap.Error(err))
		}
	}

	s.logger.Info("Backup service stopped")
	return nil
}

// PerformScheduledBackup performs a scheduled backup operation
func (s *Service) PerformScheduledBackup(ctx context.Context) error {
	s.logger.Info("Starting scheduled backup")

	// Create full backup
	result, err := s.CreateFullBackup(ctx)
	if err != nil {
		// Notify failure
		if notifyErr := s.monitor.NotifyBackupFailure(ctx, err, "scheduled_full"); notifyErr != nil {
			s.logger.Error("Failed to send failure notification", zap.Error(notifyErr))
		}
		return err
	}

	// Notify success
	if notifyErr := s.monitor.NotifyBackupSuccess(ctx, result); notifyErr != nil {
		s.logger.Error("Failed to send success notification", zap.Error(notifyErr))
	}

	s.logger.Info("Scheduled backup completed successfully")
	return nil
}

// CreateFullBackup creates a full PostgreSQL backup
func (s *Service) CreateFullBackup(ctx context.Context) (*BackupResult, error) {
	s.logger.Info("Creating full backup")

	// Create backup
	result, err := s.postgresBackup.CreateFullBackup(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create backup: %w", err)
	}

	// Log metrics
	s.monitor.LogBackupMetrics(result)

	// Upload to S3
	if err := s.storage.UploadBackup(ctx, result.FilePath); err != nil {
		s.logger.Error("Failed to upload backup to S3", zap.Error(err))
		// Don't return error, backup still exists locally
	} else {
		s.logger.Info("Backup uploaded to S3 successfully")
	}

	return result, nil
}

// RestoreFromBackup restores a database from a backup
func (s *Service) RestoreFromBackup(ctx context.Context, opts RestoreOptions) (*RestoreResult, error) {
	s.logger.Info("Starting database restore",
		zap.String("backup_file", opts.BackupFile),
	)

	result, err := s.postgresBackup.RestoreFromBackup(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to restore backup: %w", err)
	}

	s.logger.Info("Database restore completed successfully")
	return result, nil
}

// ApplyRetentionPolicy applies the backup retention policy
func (s *Service) ApplyRetentionPolicy(ctx context.Context) error {
	s.logger.Info("Applying retention policy")

	if err := s.rotationManager.ApplyRetentionPolicy(ctx); err != nil {
		return fmt.Errorf("failed to apply retention policy: %w", err)
	}

	// Get retention stats
	stats, err := s.rotationManager.GetRetentionStats(ctx)
	if err != nil {
		s.logger.Error("Failed to get retention stats", zap.Error(err))
	} else {
		s.monitor.LogRetentionMetrics(stats)
	}

	s.logger.Info("Retention policy applied successfully")
	return nil
}

// PerformHealthCheck performs a health check of the backup system
func (s *Service) PerformHealthCheck(ctx context.Context) error {
	s.logger.Info("Performing health check")

	healthResult, err := s.monitor.CheckBackupHealth(ctx, s.storage)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	if !healthResult.Healthy {
		// Send warning alert
		metadata := map[string]interface{}{
			"issues":   healthResult.Issues,
			"warnings": healthResult.Warnings,
		}
		if err := s.monitor.NotifyBackupWarning(ctx, "Backup health check failed", metadata); err != nil {
			s.logger.Error("Failed to send health check warning", zap.Error(err))
		}
	}

	return nil
}

// ArchiveWALFile archives a WAL file
func (s *Service) ArchiveWALFile(ctx context.Context, walFilePath string) error {
	return s.walArchiver.ArchiveWALFile(ctx, walFilePath)
}

// RestoreWALFile restores a WAL file
func (s *Service) RestoreWALFile(ctx context.Context, walFileName, targetPath string) error {
	return s.walArchiver.RestoreWALFile(ctx, walFileName, targetPath)
}

// CleanupOldWALFiles cleans up old WAL files
func (s *Service) CleanupOldWALFiles(ctx context.Context, retentionDays int) error {
	return s.walArchiver.CleanupOldWALFiles(ctx, retentionDays)
}

// GetStatus returns the current status of the backup system
func (s *Service) GetStatus(ctx context.Context) (map[string]interface{}, error) {
	status := map[string]interface{}{
		"service": "backup",
		"status":  "running",
		"config": map[string]interface{}{
			"schedule_enabled":      s.config.ScheduleEnabled,
			"schedule_cron":         s.config.ScheduleCron,
			"compression_enabled":   s.config.CompressionEnabled,
			"encryption_enabled":    s.config.EncryptionEnabled,
			"wal_archiving_enabled": s.config.WALArchivingEnabled,
			"monitoring_enabled":    s.config.MonitoringEnabled,
		},
	}

	// Add scheduler info
	if s.scheduler != nil {
		status["scheduler"] = s.scheduler.GetScheduleInfo()
	}

	// Add retention stats
	retentionStats, err := s.rotationManager.GetRetentionStats(ctx)
	if err != nil {
		s.logger.Error("Failed to get retention stats", zap.Error(err))
	} else {
		status["retention"] = retentionStats
	}

	// Add WAL stats if enabled
	if s.config.WALArchivingEnabled {
		walStats, err := s.walArchiver.GetWALArchiveStats(ctx)
		if err != nil {
			s.logger.Error("Failed to get WAL stats", zap.Error(err))
		} else {
			status["wal_archiving"] = walStats
		}
	}

	// Add health check
	healthResult, err := s.monitor.CheckBackupHealth(ctx, s.storage)
	if err != nil {
		s.logger.Error("Failed to perform health check", zap.Error(err))
	} else {
		status["health"] = healthResult
	}

	status["timestamp"] = time.Now()

	return status, nil
}

// ListBackups lists all available backups
func (s *Service) ListBackups(ctx context.Context) ([]interface{}, error) {
	objects, err := s.storage.ListBackups(ctx, "backups/")
	if err != nil {
		return nil, fmt.Errorf("failed to list backups: %w", err)
	}

	backups := make([]interface{}, len(objects))
	for i, obj := range objects {
		backup := map[string]interface{}{
			"key":  *obj.Key,
			"size": *obj.Size,
		}
		if obj.LastModified != nil {
			backup["last_modified"] = *obj.LastModified
			backup["age"] = time.Since(*obj.LastModified).String()
		}
		backups[i] = backup
	}

	return backups, nil
}

// DownloadBackup downloads a backup from S3 to local storage
func (s *Service) DownloadBackup(ctx context.Context, s3Key, localPath string) error {
	return s.storage.DownloadBackup(ctx, s3Key, localPath)
}

// VerifyBackup verifies the integrity of a backup
func (s *Service) VerifyBackup(filepath string, expectedChecksum string) error {
	return s.postgresBackup.VerifyBackup(filepath, expectedChecksum)
}
