package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dev-jelly/donelist/internal/backup"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	// Command flags
	var (
		configFile   = flag.String("config", "", "Path to configuration file")
		command      = flag.String("command", "status", "Command to execute: backup, restore, status, schedule, verify, cleanup")
		backupFile   = flag.String("backup-file", "", "Backup file path (for restore)")
		targetDB     = flag.String("target-db", "", "Target database name (for restore)")
		walFile      = flag.String("wal-file", "", "WAL file name")
		walPath      = flag.String("wal-path", "", "WAL file path")
		dropExisting = flag.Bool("drop-existing", false, "Drop existing database before restore")
		verify       = flag.Bool("verify", false, "Verify backup after creation")
	)

	flag.Parse()

	// Initialize logger
	logger := initLogger()
	defer logger.Sync()

	// Load configuration
	config := loadConfig(*configFile, logger)

	// Create backup service
	service, err := backup.NewService(config, logger)
	if err != nil {
		logger.Fatal("Failed to create backup service", zap.Error(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Info("Received shutdown signal")
		cancel()
	}()

	// Execute command
	switch *command {
	case "backup":
		executeBackup(ctx, service, *verify, logger)

	case "restore":
		executeRestore(ctx, service, *backupFile, *targetDB, *dropExisting, logger)

	case "status":
		executeStatus(ctx, service, logger)

	case "schedule":
		executeSchedule(ctx, service, logger)

	case "daemon":
		executeDaemon(ctx, service, logger)

	case "verify":
		executeVerify(service, *backupFile, logger)

	case "cleanup":
		executeCleanup(ctx, service, logger)

	case "archive-wal":
		executeArchiveWAL(ctx, service, *walPath, logger)

	case "restore-wal":
		executeRestoreWAL(ctx, service, *walFile, *walPath, logger)

	default:
		logger.Fatal("Unknown command", zap.String("command", *command))
	}
}

func initLogger() *zap.Logger {
	config := zap.NewProductionConfig()
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	logger, err := config.Build()
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}

	return logger
}

func loadConfig(configFile string, logger *zap.Logger) *backup.Config {
	config := backup.DefaultConfig()

	// Load from environment variables
	if dbHost := os.Getenv("DB_HOST"); dbHost != "" {
		config.DBHost = dbHost
	}
	if dbPort := os.Getenv("DB_PORT"); dbPort != "" {
		fmt.Sscanf(dbPort, "%d", &config.DBPort)
	}
	if dbName := os.Getenv("DB_NAME"); dbName != "" {
		config.DBName = dbName
	}
	if dbUser := os.Getenv("DB_USER"); dbUser != "" {
		config.DBUser = dbUser
	}
	if dbPassword := os.Getenv("DB_PASSWORD"); dbPassword != "" {
		config.DBPassword = dbPassword
	}

	// S3 configuration
	if s3Endpoint := os.Getenv("S3_ENDPOINT"); s3Endpoint != "" {
		config.S3Endpoint = s3Endpoint
	}
	if s3Bucket := os.Getenv("S3_BUCKET"); s3Bucket != "" {
		config.S3Bucket = s3Bucket
	}
	if s3AccessKey := os.Getenv("S3_ACCESS_KEY"); s3AccessKey != "" {
		config.S3AccessKeyID = s3AccessKey
	}
	if s3SecretKey := os.Getenv("S3_SECRET_KEY"); s3SecretKey != "" {
		config.S3SecretAccessKey = s3SecretKey
	}
	if s3Region := os.Getenv("S3_REGION"); s3Region != "" {
		config.S3Region = s3Region
	}

	// Backup configuration
	if backupDir := os.Getenv("BACKUP_DIR"); backupDir != "" {
		config.BackupDir = backupDir
	}
	if scheduleCron := os.Getenv("BACKUP_SCHEDULE"); scheduleCron != "" {
		config.ScheduleCron = scheduleCron
	}

	// Set default S3 region if not specified
	if config.S3Region == "" {
		config.S3Region = "us-east-1"
	}

	return config
}

func executeBackup(ctx context.Context, service *backup.Service, verify bool, logger *zap.Logger) {
	logger.Info("Creating backup")

	result, err := service.CreateFullBackup(ctx)
	if err != nil {
		logger.Fatal("Backup failed", zap.Error(err))
	}

	logger.Info("Backup created successfully",
		zap.String("filename", result.Filename),
		zap.Int64("size", result.Size),
		zap.Duration("duration", result.Duration),
		zap.String("checksum", result.Checksum),
	)

	if verify {
		logger.Info("Verifying backup")
		if err := service.VerifyBackup(result.FilePath, result.Checksum); err != nil {
			logger.Fatal("Backup verification failed", zap.Error(err))
		}
		logger.Info("Backup verification successful")
	}
}

func executeRestore(ctx context.Context, service *backup.Service, backupFile, targetDB string, dropExisting bool, logger *zap.Logger) {
	if backupFile == "" {
		logger.Fatal("Backup file is required for restore")
	}
	if targetDB == "" {
		logger.Fatal("Target database is required for restore")
	}

	logger.Info("Restoring database",
		zap.String("backup_file", backupFile),
		zap.String("target_db", targetDB),
	)

	opts := backup.RestoreOptions{
		BackupFile:        backupFile,
		TargetDatabase:    targetDB,
		DropExisting:      dropExisting,
		CreateDatabase:    dropExisting,
		Clean:             true,
		IfExists:          true,
		SingleTransaction: true,
		NoOwner:           true,
		NoACL:             true,
		Verbose:           true,
	}

	result, err := service.RestoreFromBackup(ctx, opts)
	if err != nil {
		logger.Fatal("Restore failed", zap.Error(err))
	}

	logger.Info("Restore completed successfully",
		zap.Duration("duration", result.Duration),
	)
}

func executeStatus(ctx context.Context, service *backup.Service, logger *zap.Logger) {
	logger.Info("Getting backup status")

	status, err := service.GetStatus(ctx)
	if err != nil {
		logger.Fatal("Failed to get status", zap.Error(err))
	}

	fmt.Printf("\nBackup System Status:\n")
	fmt.Printf("====================\n\n")

	if health, ok := status["health"].(*backup.HealthCheckResult); ok {
		fmt.Printf("Health: %v\n", health.Healthy)
		fmt.Printf("Total Backups: %d\n", health.TotalBackups)
		fmt.Printf("Total Size: %d bytes\n", health.TotalSize)
		if !health.LastBackupTime.IsZero() {
			fmt.Printf("Last Backup: %s (%s ago)\n",
				health.LastBackupTime.Format(time.RFC3339),
				health.TimeSinceLastBackup)
		}
		if len(health.Issues) > 0 {
			fmt.Printf("Issues: %v\n", health.Issues)
		}
		if len(health.Warnings) > 0 {
			fmt.Printf("Warnings: %v\n", health.Warnings)
		}
	}

	logger.Info("Status retrieved successfully", zap.Any("status", status))
}

func executeSchedule(ctx context.Context, service *backup.Service, logger *zap.Logger) {
	logger.Info("Starting scheduled backup service")

	if err := service.Start(ctx); err != nil {
		logger.Fatal("Failed to start service", zap.Error(err))
	}

	<-ctx.Done()

	if err := service.Stop(); err != nil {
		logger.Error("Failed to stop service", zap.Error(err))
	}
}

func executeDaemon(ctx context.Context, service *backup.Service, logger *zap.Logger) {
	logger.Info("Starting backup daemon")

	if err := service.Start(ctx); err != nil {
		logger.Fatal("Failed to start service", zap.Error(err))
	}

	logger.Info("Backup daemon running")

	<-ctx.Done()

	logger.Info("Shutting down backup daemon")

	if err := service.Stop(); err != nil {
		logger.Error("Failed to stop service", zap.Error(err))
	}
}

func executeVerify(service *backup.Service, backupFile string, logger *zap.Logger) {
	if backupFile == "" {
		logger.Fatal("Backup file is required for verification")
	}

	logger.Info("Verifying backup", zap.String("file", backupFile))

	if err := service.VerifyBackup(backupFile, ""); err != nil {
		logger.Fatal("Verification failed", zap.Error(err))
	}

	logger.Info("Verification successful")
}

func executeCleanup(ctx context.Context, service *backup.Service, logger *zap.Logger) {
	logger.Info("Applying retention policy")

	if err := service.ApplyRetentionPolicy(ctx); err != nil {
		logger.Fatal("Cleanup failed", zap.Error(err))
	}

	logger.Info("Cleanup completed successfully")
}

func executeArchiveWAL(ctx context.Context, service *backup.Service, walPath string, logger *zap.Logger) {
	if walPath == "" {
		logger.Fatal("WAL file path is required")
	}

	if err := service.ArchiveWALFile(ctx, walPath); err != nil {
		logger.Fatal("WAL archiving failed", zap.Error(err))
	}

	logger.Info("WAL file archived successfully")
}

func executeRestoreWAL(ctx context.Context, service *backup.Service, walFile, walPath string, logger *zap.Logger) {
	if walFile == "" || walPath == "" {
		logger.Fatal("Both WAL file name and path are required")
	}

	if err := service.RestoreWALFile(ctx, walFile, walPath); err != nil {
		logger.Fatal("WAL restore failed", zap.Error(err))
	}

	logger.Info("WAL file restored successfully")
}
