package backup

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"go.uber.org/zap"
)

// PostgresBackup handles PostgreSQL backup operations
type PostgresBackup struct {
	config *Config
	logger *zap.Logger
}

// NewPostgresBackup creates a new PostgreSQL backup handler
func NewPostgresBackup(config *Config, logger *zap.Logger) *PostgresBackup {
	return &PostgresBackup{
		config: config,
		logger: logger,
	}
}

// BackupResult contains information about a backup operation
type BackupResult struct {
	Filename     string
	FilePath     string
	Size         int64
	Checksum     string
	StartTime    time.Time
	EndTime      time.Time
	Duration     time.Duration
	Compressed   bool
	Encrypted    bool
	BackupType   string // "full", "incremental"
	WALFile      string
	Error        error
}

// CreateFullBackup creates a full PostgreSQL backup using pg_dump
func (pb *PostgresBackup) CreateFullBackup(ctx context.Context) (*BackupResult, error) {
	startTime := time.Now()

	result := &BackupResult{
		StartTime:  startTime,
		BackupType: "full",
		Compressed: pb.config.CompressionEnabled,
		Encrypted:  pb.config.EncryptionEnabled,
	}

	pb.logger.Info("Starting full PostgreSQL backup",
		zap.String("database", pb.config.DBName),
		zap.String("host", pb.config.DBHost),
	)

	// Create backup directory if it doesn't exist
	if err := os.MkdirAll(pb.config.BackupDir, 0755); err != nil {
		result.Error = fmt.Errorf("failed to create backup directory: %w", err)
		return result, result.Error
	}

	// Generate backup filename with timestamp
	timestamp := startTime.Format("20060102_150405")
	filename := fmt.Sprintf("postgres_full_%s_%s.sql", pb.config.DBName, timestamp)
	if pb.config.CompressionEnabled {
		filename += ".gz"
	}
	if pb.config.EncryptionEnabled {
		filename += ".enc"
	}

	filepath := filepath.Join(pb.config.BackupDir, filename)
	result.Filename = filename
	result.FilePath = filepath

	// Execute pg_dump
	if err := pb.executePgDump(ctx, filepath); err != nil {
		result.Error = err
		return result, err
	}

	// Get file info
	fileInfo, err := os.Stat(filepath)
	if err != nil {
		result.Error = fmt.Errorf("failed to stat backup file: %w", err)
		return result, result.Error
	}
	result.Size = fileInfo.Size()

	// Calculate checksum
	checksum, err := pb.calculateChecksum(filepath)
	if err != nil {
		pb.logger.Warn("Failed to calculate checksum", zap.Error(err))
	} else {
		result.Checksum = checksum
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	pb.logger.Info("Full PostgreSQL backup completed",
		zap.String("filename", filename),
		zap.Int64("size", result.Size),
		zap.Duration("duration", result.Duration),
	)

	return result, nil
}

// executePgDump executes the pg_dump command
func (pb *PostgresBackup) executePgDump(ctx context.Context, outputPath string) error {
	// Prepare pg_dump command
	args := []string{
		"-h", pb.config.DBHost,
		"-p", fmt.Sprintf("%d", pb.config.DBPort),
		"-U", pb.config.DBUser,
		"-d", pb.config.DBName,
		"--format=custom", // Custom format allows parallel restore
		"--verbose",
		"--no-owner",
		"--no-acl",
	}

	// Create output file
	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	var writer io.Writer = outFile

	// Add compression if enabled
	if pb.config.CompressionEnabled {
		gzipWriter := gzip.NewWriter(outFile)
		defer gzipWriter.Close()
		writer = gzipWriter
	}

	// Add encryption if enabled
	if pb.config.EncryptionEnabled {
		encWriter, err := pb.createEncryptionWriter(writer)
		if err != nil {
			return fmt.Errorf("failed to create encryption writer: %w", err)
		}
		writer = encWriter
	}

	// Execute pg_dump
	cmd := exec.CommandContext(ctx, "pg_dump", args...)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("PGPASSWORD=%s", pb.config.DBPassword),
	)
	cmd.Stdout = writer

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pg_dump failed: %w, stderr: %s", err, stderr.String())
	}

	return nil
}

// createEncryptionWriter creates an encrypted writer using AES-256-GCM
func (pb *PostgresBackup) createEncryptionWriter(w io.Writer) (io.Writer, error) {
	if pb.config.EncryptionKey == "" {
		return nil, fmt.Errorf("encryption key is required")
	}

	// Derive 32-byte key from password
	key := sha256.Sum256([]byte(pb.config.EncryptionKey))

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Write nonce to output
	if _, err := w.Write(nonce); err != nil {
		return nil, fmt.Errorf("failed to write nonce: %w", err)
	}

	// Create streaming encryption writer
	return &encryptedWriter{
		w:     w,
		gcm:   gcm,
		nonce: nonce,
	}, nil
}

// encryptedWriter implements io.Writer for streaming encryption
type encryptedWriter struct {
	w     io.Writer
	gcm   cipher.AEAD
	nonce []byte
}

func (ew *encryptedWriter) Write(p []byte) (n int, err error) {
	encrypted := ew.gcm.Seal(nil, ew.nonce, p, nil)
	return ew.w.Write(encrypted)
}

// calculateChecksum calculates SHA-256 checksum of a file
func (pb *PostgresBackup) calculateChecksum(filepath string) (string, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// VerifyBackup verifies the integrity of a backup file
func (pb *PostgresBackup) VerifyBackup(filepath string, expectedChecksum string) error {
	pb.logger.Info("Verifying backup", zap.String("file", filepath))

	// Check if file exists
	if _, err := os.Stat(filepath); err != nil {
		return fmt.Errorf("backup file not found: %w", err)
	}

	// Verify checksum if provided
	if expectedChecksum != "" {
		actualChecksum, err := pb.calculateChecksum(filepath)
		if err != nil {
			return fmt.Errorf("failed to calculate checksum: %w", err)
		}
		if actualChecksum != expectedChecksum {
			return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actualChecksum)
		}
	}

	// Verify pg_restore can read the backup (dry run)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "pg_restore", "--list", filepath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("backup verification failed: %w, stderr: %s", err, stderr.String())
	}

	pb.logger.Info("Backup verification successful", zap.String("file", filepath))
	return nil
}

// GetBackupInfo retrieves information about a backup file
func (pb *PostgresBackup) GetBackupInfo(filepath string) (*BackupResult, error) {
	fileInfo, err := os.Stat(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	checksum, err := pb.calculateChecksum(filepath)
	if err != nil {
		checksum = ""
	}

	return &BackupResult{
		Filename:   fileInfo.Name(),
		FilePath:   filepath,
		Size:       fileInfo.Size(),
		Checksum:   checksum,
		BackupType: "full",
	}, nil
}
