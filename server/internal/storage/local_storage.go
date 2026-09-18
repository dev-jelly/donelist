package storage

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
)

// LocalStorageService implements storage using local filesystem
type LocalStorageService struct {
	basePath string
	baseURL  string
	logger   *zap.Logger
}

// NewLocalStorageService creates a new local storage service
func NewLocalStorageService(basePath, baseURL string, logger *zap.Logger) *LocalStorageService {
	// Ensure base path exists
	if err := os.MkdirAll(basePath, 0755); err != nil {
		logger.Error("Failed to create storage directory", zap.Error(err))
	}

	return &LocalStorageService{
		basePath: basePath,
		baseURL:  strings.TrimSuffix(baseURL, "/"),
		logger:   logger,
	}
}

// UploadFile uploads a file to local storage
func (s *LocalStorageService) UploadFile(ctx context.Context, file multipart.File, filename string, contentType string) (string, error) {
	// Create full path
	fullPath := filepath.Join(s.basePath, filename)

	// Ensure directory exists
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// Create destination file
	dst, err := os.Create(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer dst.Close()

	// Copy file contents
	if _, err := io.Copy(dst, file); err != nil {
		os.Remove(fullPath) // Clean up on error
		return "", fmt.Errorf("failed to save file: %w", err)
	}

	// Generate public URL
	publicURL := fmt.Sprintf("%s/%s", s.baseURL, filename)

	s.logger.Info("File uploaded successfully",
		zap.String("filename", filename),
		zap.String("path", fullPath),
		zap.String("url", publicURL))

	return publicURL, nil
}

// DeleteFile deletes a file from local storage
func (s *LocalStorageService) DeleteFile(ctx context.Context, fileURL string) error {
	// Extract filename from URL
	filename := strings.TrimPrefix(fileURL, s.baseURL+"/")
	fullPath := filepath.Join(s.basePath, filename)

	// Check if file exists
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		s.logger.Warn("File not found for deletion",
			zap.String("url", fileURL),
			zap.String("path", fullPath))
		return nil // Not an error if file doesn't exist
	}

	// Delete file
	if err := os.Remove(fullPath); err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	s.logger.Info("File deleted successfully",
		zap.String("url", fileURL),
		zap.String("path", fullPath))

	return nil
}

// GetPresignedURL generates a temporary access URL (not applicable for local storage)
func (s *LocalStorageService) GetPresignedURL(ctx context.Context, fileURL string, duration time.Duration) (string, error) {
	// For local storage, we just return the regular URL
	// In production, you might want to implement token-based temporary access
	return fileURL, nil
}

// ServeFile serves a file over HTTP (for use with a file server)
func (s *LocalStorageService) ServeFile(filename string) (string, error) {
	fullPath := filepath.Join(s.basePath, filename)

	// Check if file exists
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return "", fmt.Errorf("file not found")
	}

	return fullPath, nil
}