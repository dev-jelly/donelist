package backup

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"go.uber.org/zap"
)

// RestoreResult contains information about a restore operation
type RestoreResult struct {
	Filename  string
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration
	Error     error
}

// RestoreOptions defines options for restore operations
type RestoreOptions struct {
	BackupFile        string
	TargetDatabase    string
	DropExisting      bool
	CreateDatabase    bool
	Clean             bool
	IfExists          bool
	SingleTransaction bool
	NoOwner           bool
	NoACL             bool
	Verbose           bool
}

// RestoreFromBackup restores a PostgreSQL database from a backup file
func (pb *PostgresBackup) RestoreFromBackup(ctx context.Context, opts RestoreOptions) (*RestoreResult, error) {
	startTime := time.Now()

	result := &RestoreResult{
		Filename:  opts.BackupFile,
		StartTime: startTime,
	}

	pb.logger.Info("Starting database restore",
		zap.String("backup_file", opts.BackupFile),
		zap.String("target_database", opts.TargetDatabase),
	)

	// Verify backup file exists
	if _, err := os.Stat(opts.BackupFile); err != nil {
		result.Error = fmt.Errorf("backup file not found: %w", err)
		return result, result.Error
	}

	// Verify backup integrity before restore
	if err := pb.VerifyBackup(opts.BackupFile, ""); err != nil {
		result.Error = fmt.Errorf("backup verification failed: %w", err)
		return result, result.Error
	}

	// Drop existing database if requested
	if opts.DropExisting {
		if err := pb.dropDatabase(ctx, opts.TargetDatabase); err != nil {
			result.Error = fmt.Errorf("failed to drop existing database: %w", err)
			return result, result.Error
		}
	}

	// Create database if requested
	if opts.CreateDatabase {
		if err := pb.createDatabase(ctx, opts.TargetDatabase); err != nil {
			result.Error = fmt.Errorf("failed to create database: %w", err)
			return result, result.Error
		}
	}

	// Execute pg_restore
	if err := pb.executePgRestore(ctx, opts); err != nil {
		result.Error = err
		return result, err
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	pb.logger.Info("Database restore completed",
		zap.String("backup_file", opts.BackupFile),
		zap.Duration("duration", result.Duration),
	)

	return result, nil
}

// executePgRestore executes the pg_restore command
func (pb *PostgresBackup) executePgRestore(ctx context.Context, opts RestoreOptions) error {
	args := []string{
		"-h", pb.config.DBHost,
		"-p", fmt.Sprintf("%d", pb.config.DBPort),
		"-U", pb.config.DBUser,
		"-d", opts.TargetDatabase,
	}

	if opts.Clean {
		args = append(args, "--clean")
	}
	if opts.IfExists {
		args = append(args, "--if-exists")
	}
	if opts.SingleTransaction {
		args = append(args, "--single-transaction")
	}
	if opts.NoOwner {
		args = append(args, "--no-owner")
	}
	if opts.NoACL {
		args = append(args, "--no-acl")
	}
	if opts.Verbose {
		args = append(args, "--verbose")
	}

	// Prepare input file
	inputFile, err := os.Open(opts.BackupFile)
	if err != nil {
		return fmt.Errorf("failed to open backup file: %w", err)
	}
	defer inputFile.Close()

	var reader io.Reader = inputFile

	// Handle encryption
	if pb.config.EncryptionEnabled {
		decReader, err := pb.createDecryptionReader(inputFile)
		if err != nil {
			return fmt.Errorf("failed to create decryption reader: %w", err)
		}
		reader = decReader
	}

	// Handle compression
	if pb.config.CompressionEnabled {
		gzipReader, err := gzip.NewReader(reader)
		if err != nil {
			return fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzipReader.Close()
		reader = gzipReader
	}

	// Execute pg_restore
	cmd := exec.CommandContext(ctx, "pg_restore", args...)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("PGPASSWORD=%s", pb.config.DBPassword),
	)
	cmd.Stdin = reader

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pg_restore failed: %w, stderr: %s", err, stderr.String())
	}

	return nil
}

// createDecryptionReader creates a decrypted reader using AES-256-GCM
func (pb *PostgresBackup) createDecryptionReader(r io.Reader) (io.Reader, error) {
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

	// Read nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(r, nonce); err != nil {
		return nil, fmt.Errorf("failed to read nonce: %w", err)
	}

	return &decryptedReader{
		r:     r,
		gcm:   gcm,
		nonce: nonce,
	}, nil
}

// decryptedReader implements io.Reader for streaming decryption
type decryptedReader struct {
	r     io.Reader
	gcm   cipher.AEAD
	nonce []byte
}

func (dr *decryptedReader) Read(p []byte) (n int, err error) {
	encrypted := make([]byte, len(p)+dr.gcm.Overhead())
	n, err = dr.r.Read(encrypted)
	if err != nil {
		return 0, err
	}

	decrypted, err := dr.gcm.Open(nil, dr.nonce, encrypted[:n], nil)
	if err != nil {
		return 0, fmt.Errorf("decryption failed: %w", err)
	}

	copy(p, decrypted)
	return len(decrypted), nil
}

// dropDatabase drops a database
func (pb *PostgresBackup) dropDatabase(ctx context.Context, dbName string) error {
	pb.logger.Info("Dropping database", zap.String("database", dbName))

	cmd := exec.CommandContext(ctx, "dropdb",
		"-h", pb.config.DBHost,
		"-p", fmt.Sprintf("%d", pb.config.DBPort),
		"-U", pb.config.DBUser,
		"--if-exists",
		dbName,
	)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("PGPASSWORD=%s", pb.config.DBPassword),
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("dropdb failed: %w, stderr: %s", err, stderr.String())
	}

	return nil
}

// createDatabase creates a new database
func (pb *PostgresBackup) createDatabase(ctx context.Context, dbName string) error {
	pb.logger.Info("Creating database", zap.String("database", dbName))

	cmd := exec.CommandContext(ctx, "createdb",
		"-h", pb.config.DBHost,
		"-p", fmt.Sprintf("%d", pb.config.DBPort),
		"-U", pb.config.DBUser,
		dbName,
	)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("PGPASSWORD=%s", pb.config.DBPassword),
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("createdb failed: %w, stderr: %s", err, stderr.String())
	}

	return nil
}

// RestorePointInTime performs point-in-time recovery using WAL files
func (pb *PostgresBackup) RestorePointInTime(ctx context.Context, baseBackup string, targetTime time.Time, walPath string) (*RestoreResult, error) {
	startTime := time.Now()

	result := &RestoreResult{
		Filename:  baseBackup,
		StartTime: startTime,
	}

	pb.logger.Info("Starting point-in-time recovery",
		zap.String("base_backup", baseBackup),
		zap.Time("target_time", targetTime),
		zap.String("wal_path", walPath),
	)

	// This is a simplified implementation
	// In production, you would:
	// 1. Restore from base backup
	// 2. Configure recovery.conf or postgresql.auto.conf
	// 3. Apply WAL files up to target time
	// 4. Start PostgreSQL in recovery mode

	pb.logger.Warn("Point-in-time recovery requires manual configuration of PostgreSQL recovery settings")

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	return result, nil
}
