package export

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dev-jelly/donelist/internal/checkin"
	"github.com/dev-jelly/donelist/internal/category"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// ExportService handles data export operations with background processing
type ExportService struct {
	db           *sqlx.DB
	repo         *Repository
	checkinRepo  *checkin.Repository
	categoryRepo *category.Repository
	csvExporter  *CSVExporter
	jsonExporter *JSONExporter
	pdfExporter  *PDFExporter
	logger       *zap.Logger
	storageDir   string // Directory to store export files
}

// NewExportService creates a new export service
func NewExportService(
	db *sqlx.DB,
	checkinRepo *checkin.Repository,
	categoryRepo *category.Repository,
	logger *zap.Logger,
	storageDir string,
) *ExportService {
	// Ensure storage directory exists
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		logger.Error("Failed to create export storage directory", zap.Error(err))
	}

	return &ExportService{
		db:           db,
		repo:         NewRepository(db),
		checkinRepo:  checkinRepo,
		categoryRepo: categoryRepo,
		csvExporter:  NewCSVExporter(logger),
		jsonExporter: NewJSONExporter(logger),
		pdfExporter:  NewPDFExporter(logger),
		logger:       logger,
		storageDir:   storageDir,
	}
}

// RequestExport creates a new export job and starts processing asynchronously
func (s *ExportService) RequestExport(ctx context.Context, req *ExportRequest) (*ExportJob, error) {
	// Validate request
	if err := s.validateRequest(req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Check for existing pending exports
	existingJobs, err := s.repo.ListByUserID(ctx, req.UserID, 5)
	if err == nil {
		for _, job := range existingJobs {
			if job.Status == StatusPending || job.Status == StatusProcessing {
				return nil, fmt.Errorf("you already have an export in progress")
			}
		}
	}

	// Create export job
	job := &ExportJob{
		ID:          uuid.New(),
		UserID:      req.UserID,
		Format:      req.Format,
		Status:      StatusPending,
		RecordCount: 0,
		ExpiresAt:   time.Now().Add(7 * 24 * time.Hour), // Expires in 7 days
		CreatedAt:   time.Now(),
	}

	if err := s.repo.Create(ctx, job); err != nil {
		return nil, fmt.Errorf("failed to create export job: %w", err)
	}

	s.logger.Info("Export job created",
		zap.String("job_id", job.ID.String()),
		zap.String("user_id", req.UserID.String()),
		zap.String("format", string(req.Format)),
	)

	// Process export in background
	go s.processExport(context.Background(), job.ID, req)

	return job, nil
}

// processExport handles the actual export processing
func (s *ExportService) processExport(ctx context.Context, jobID uuid.UUID, req *ExportRequest) {
	startTime := time.Now()

	// Update job status to processing
	job, err := s.repo.GetByID(ctx, jobID)
	if err != nil {
		s.logger.Error("Failed to get export job", zap.Error(err))
		return
	}

	now := time.Now()
	job.Status = StatusProcessing
	job.StartedAt = &now

	if err := s.repo.Update(ctx, job); err != nil {
		s.logger.Error("Failed to update job status", zap.Error(err))
		return
	}

	s.logger.Info("Starting export processing",
		zap.String("job_id", jobID.String()),
		zap.String("format", string(req.Format)),
	)

	// Gather data
	exportData, err := s.gatherExportData(ctx, req)
	if err != nil {
		s.markJobFailed(ctx, jobID, fmt.Sprintf("failed to gather data: %v", err))
		return
	}

	// Select appropriate exporter
	var exporter Exporter
	switch req.Format {
	case FormatCSV:
		exporter = s.csvExporter
	case FormatJSON:
		exporter = s.jsonExporter
	case FormatPDF:
		exporter = s.pdfExporter
	default:
		s.markJobFailed(ctx, jobID, fmt.Sprintf("unsupported format: %s", req.Format))
		return
	}

	// Export data
	data, err := exporter.Export(exportData)
	if err != nil {
		s.markJobFailed(ctx, jobID, fmt.Sprintf("export failed: %v", err))
		return
	}

	// Save file to storage
	fileName := fmt.Sprintf("%s.%s", jobID.String(), exporter.GetFileExtension())
	filePath := filepath.Join(s.storageDir, fileName)

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		s.markJobFailed(ctx, jobID, fmt.Sprintf("failed to save file: %v", err))
		return
	}

	// Update job as completed
	job, err = s.repo.GetByID(ctx, jobID)
	if err != nil {
		s.logger.Error("Failed to get job for completion", zap.Error(err))
		return
	}

	completedAt := time.Now()
	fileSize := int64(len(data))
	job.Status = StatusCompleted
	job.FilePath = &filePath
	job.FileSize = &fileSize
	job.RecordCount = len(exportData.Checkins)
	job.CompletedAt = &completedAt

	if err := s.repo.Update(ctx, job); err != nil {
		s.logger.Error("Failed to mark job as completed", zap.Error(err))
		return
	}

	s.logger.Info("Export completed successfully",
		zap.String("job_id", jobID.String()),
		zap.Int("record_count", job.RecordCount),
		zap.Int64("file_size", fileSize),
		zap.Duration("duration", time.Since(startTime)),
	)
}

// gatherExportData collects all data for export
func (s *ExportService) gatherExportData(ctx context.Context, req *ExportRequest) (*ExportData, error) {
	// Build list options
	listOpts := checkin.ListOptions{
		UserID:    req.UserID,
		StartDate: req.StartDate,
		EndDate:   req.EndDate,
		Limit:     10000, // Max export limit
		Offset:    0,
	}

	// Fetch checkins
	checkins, _, err := s.checkinRepo.List(ctx, listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch checkins: %w", err)
	}

	// Get category map for lookups
	categories, err := s.categoryRepo.List(ctx, req.UserID)
	if err != nil {
		s.logger.Warn("Failed to fetch categories", zap.Error(err))
	}

	categoryMap := make(map[uuid.UUID]string)
	for _, cat := range categories {
		categoryMap[cat.ID] = cat.Name
	}

	// Transform to export format
	exportCheckins := make([]CheckinExportData, len(checkins))
	for i, c := range checkins {
		categoryName := "Uncategorized"
		if c.CategoryID != nil {
			if name, ok := categoryMap[*c.CategoryID]; ok {
				categoryName = name
			}
		}

		// Determine priority and status - these would come from your checkin structure
		priority := "medium"
		status := "completed"

		exportCheckins[i] = CheckinExportData{
			ID:          c.ID,
			Title:       c.Content, // Using content as title
			Description: nil,       // Add if you have description field
			Category:    categoryName,
			Tags:        c.Tags,
			Priority:    priority,
			Status:      status,
			CreatedAt:   c.CreatedAt,
			UpdatedAt:   c.UpdatedAt,
		}
	}

	// Build metadata
	metadata := ExportMetadata{
		ExportedAt:    time.Now(),
		ExportFormat:  req.Format,
		TotalRecords:  len(exportCheckins),
		FilterApplied: req.StartDate != nil || req.EndDate != nil,
	}

	if req.StartDate != nil && req.EndDate != nil {
		metadata.DateRange = &DateRange{
			Start: *req.StartDate,
			End:   *req.EndDate,
		}
	}

	return &ExportData{
		Checkins: exportCheckins,
		Metadata: metadata,
	}, nil
}

// validateRequest validates an export request
func (s *ExportService) validateRequest(req *ExportRequest) error {
	if req.UserID == uuid.Nil {
		return fmt.Errorf("user ID is required")
	}

	if req.Format != FormatCSV && req.Format != FormatJSON && req.Format != FormatPDF {
		return fmt.Errorf("invalid format: must be csv, json, or pdf")
	}

	if req.StartDate != nil && req.EndDate != nil {
		if req.StartDate.After(*req.EndDate) {
			return fmt.Errorf("start date must be before end date")
		}

		// Limit export range to 1 year
		if req.EndDate.Sub(*req.StartDate) > 365*24*time.Hour {
			return fmt.Errorf("export range cannot exceed 1 year")
		}
	}

	return nil
}

// markJobFailed marks a job as failed with error message
func (s *ExportService) markJobFailed(ctx context.Context, jobID uuid.UUID, errorMsg string) {
	job, err := s.repo.GetByID(ctx, jobID)
	if err != nil {
		s.logger.Error("Failed to get job for failure update", zap.Error(err))
		return
	}

	job.Status = StatusFailed
	job.Error = &errorMsg

	if err := s.repo.Update(ctx, job); err != nil {
		s.logger.Error("Failed to mark job as failed", zap.Error(err))
	}

	s.logger.Error("Export job failed",
		zap.String("job_id", jobID.String()),
		zap.String("error", errorMsg),
	)
}

// GetExportJob retrieves an export job by ID
func (s *ExportService) GetExportJob(ctx context.Context, jobID, userID uuid.UUID) (*ExportJob, error) {
	job, err := s.repo.GetByID(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("export job not found: %w", err)
	}

	// Verify ownership
	if job.UserID != userID {
		return nil, fmt.Errorf("unauthorized access to export job")
	}

	return job, nil
}

// GetExportHistory retrieves export history for a user
func (s *ExportService) GetExportHistory(ctx context.Context, userID uuid.UUID, limit int) ([]*ExportJob, error) {
	return s.repo.ListByUserID(ctx, userID, limit)
}

// GetExportFile retrieves the file path for a completed export
func (s *ExportService) GetExportFile(ctx context.Context, jobID, userID uuid.UUID) (string, error) {
	job, err := s.GetExportJob(ctx, jobID, userID)
	if err != nil {
		return "", err
	}

	if job.Status != StatusCompleted {
		return "", fmt.Errorf("export is not yet completed (status: %s)", job.Status)
	}

	if job.FilePath == nil {
		return "", fmt.Errorf("file path not available")
	}

	// Check if file exists
	if _, err := os.Stat(*job.FilePath); os.IsNotExist(err) {
		return "", fmt.Errorf("export file not found")
	}

	// Check if expired
	if time.Now().After(job.ExpiresAt) {
		return "", fmt.Errorf("export has expired")
	}

	return *job.FilePath, nil
}

// CleanupExpiredExports removes expired export files
func (s *ExportService) CleanupExpiredExports(ctx context.Context) error {
	// Delete expired jobs from database
	count, err := s.repo.DeleteExpired(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete expired jobs: %w", err)
	}

	s.logger.Info("Cleaned up expired exports",
		zap.Int64("count", count),
	)

	// Clean up orphaned files in storage directory
	files, err := os.ReadDir(s.storageDir)
	if err != nil {
		return fmt.Errorf("failed to read storage directory: %w", err)
	}

	cleaned := 0
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		// Check if file is older than 7 days
		info, err := file.Info()
		if err != nil {
			continue
		}

		if time.Since(info.ModTime()) > 7*24*time.Hour {
			filePath := filepath.Join(s.storageDir, file.Name())
			if err := os.Remove(filePath); err != nil {
				s.logger.Warn("Failed to remove old export file",
					zap.String("file", filePath),
					zap.Error(err),
				)
			} else {
				cleaned++
			}
		}
	}

	s.logger.Info("Cleaned up orphaned export files",
		zap.Int("count", cleaned),
	)

	return nil
}
