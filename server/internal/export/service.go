package export

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jung-kurt/gofpdf"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ExportType represents what data to export
type ExportType string

const (
	ExportTypeCheckins   ExportType = "checkins"
	ExportTypeCategories ExportType = "categories"
	ExportTypeAnalytics  ExportType = "analytics"
	ExportTypeFull      ExportType = "full"
)

// ServiceExportRequest represents a request to export data (internal service request)
type ServiceExportRequest struct {
	UserID    uuid.UUID
	Type      ExportType
	Format    ExportFormat
	StartDate *time.Time
	EndDate   *time.Time
	Options   ExportOptions
}

// ExportOptions contains additional export options
type ExportOptions struct {
	IncludeDeleted   bool                   `json:"include_deleted"`
	IncludeAnalytics bool                   `json:"include_analytics"`
	TimeZone         string                 `json:"timezone"`
	DateFormat       string                 `json:"date_format"`
	CustomFields     map[string]interface{} `json:"custom_fields"`
}

// ExportResult contains the exported data
type ExportResult struct {
	ID          uuid.UUID    `json:"id"`
	UserID      uuid.UUID    `json:"user_id"`
	Type        ExportType   `json:"type"`
	Format      ExportFormat `json:"format"`
	Data        []byte       `json:"-"` // Binary data
	FileName    string       `json:"filename"`
	Size        int64        `json:"size"`
	RecordCount int          `json:"record_count"`
	CreatedAt   time.Time    `json:"created_at"`
	ExpiresAt   time.Time    `json:"expires_at"`
	URL         string       `json:"url,omitempty"` // If stored in cloud storage
}

// Service handles data exports
type Service struct {
	db     *gorm.DB
	logger *zap.Logger
}

// NewService creates a new export service
func NewService(db *gorm.DB, logger *zap.Logger) *Service {
	return &Service{
		db:     db,
		logger: logger,
	}
}

// Export exports user data in the requested format
func (s *Service) Export(ctx context.Context, req *ServiceExportRequest) (*ExportResult, error) {
	// Validate request
	if err := s.validateRequest(req); err != nil {
		return nil, err
	}

	// Get data based on export type
	data, recordCount, err := s.getData(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get data: %w", err)
	}

	// Convert data to requested format
	exportData, err := s.formatData(data, req.Format, req.Options)
	if err != nil {
		return nil, fmt.Errorf("failed to format data: %w", err)
	}

	// Create export result
	result := &ExportResult{
		ID:          uuid.New(),
		UserID:      req.UserID,
		Type:        req.Type,
		Format:      req.Format,
		Data:        exportData,
		FileName:    s.generateFileName(req),
		Size:        int64(len(exportData)),
		RecordCount: recordCount,
		CreatedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(24 * time.Hour), // Exports expire after 24 hours
	}

	// Save export record (without data) to database
	if err := s.saveExportRecord(ctx, result); err != nil {
		s.logger.Error("Failed to save export record", zap.Error(err))
	}

	return result, nil
}

// validateRequest validates an export request
func (s *Service) validateRequest(req *ServiceExportRequest) error {
	if req.UserID == uuid.Nil {
		return fmt.Errorf("user ID is required")
	}

	// Validate date range
	if req.StartDate != nil && req.EndDate != nil {
		if req.StartDate.After(*req.EndDate) {
			return fmt.Errorf("start date must be before end date")
		}

		// Limit export range to 1 year
		if req.EndDate.Sub(*req.StartDate) > 365*24*time.Hour {
			return fmt.Errorf("export range cannot exceed 1 year")
		}
	}

	// Set defaults
	if req.Options.DateFormat == "" {
		req.Options.DateFormat = "2006-01-02 15:04:05"
	}
	if req.Options.TimeZone == "" {
		req.Options.TimeZone = "UTC"
	}

	return nil
}

// getData retrieves data based on export type
func (s *Service) getData(ctx context.Context, req *ServiceExportRequest) (interface{}, int, error) {
	switch req.Type {
	case ExportTypeCheckins:
		return s.getCheckinsData(ctx, req)
	case ExportTypeCategories:
		return s.getCategoriesData(ctx, req)
	case ExportTypeAnalytics:
		return s.getAnalyticsData(ctx, req)
	case ExportTypeFull:
		return s.getFullData(ctx, req)
	default:
		return nil, 0, fmt.Errorf("unsupported export type: %s", req.Type)
	}
}

// getCheckinsData retrieves checkins data
func (s *Service) getCheckinsData(ctx context.Context, req *ServiceExportRequest) (interface{}, int, error) {
	// This is a simplified version - in production, you'd query the actual checkins table
	type CheckinExport struct {
		ID         uuid.UUID  `json:"id"`
		UserID     uuid.UUID  `json:"user_id"`
		Title      string     `json:"title"`
		Duration   int        `json:"duration_minutes"`
		CategoryID *uuid.UUID `json:"category_id,omitempty"`
		Category   string     `json:"category,omitempty"`
		CreatedAt  time.Time  `json:"created_at"`
		UpdatedAt  time.Time  `json:"updated_at"`
		DeletedAt  *time.Time `json:"deleted_at,omitempty"`
	}

	var checkins []CheckinExport
	query := s.db.WithContext(ctx).
		Table("checkins").
		Select("checkins.*, categories.name as category").
		Joins("LEFT JOIN categories ON checkins.category_id = categories.id").
		Where("checkins.user_id = ?", req.UserID)

	// Apply date filters
	if req.StartDate != nil {
		query = query.Where("checkins.created_at >= ?", req.StartDate)
	}
	if req.EndDate != nil {
		query = query.Where("checkins.created_at <= ?", req.EndDate)
	}

	// Include deleted records if requested
	if !req.Options.IncludeDeleted {
		query = query.Where("checkins.deleted_at IS NULL")
	}

	err := query.Find(&checkins).Error
	if err != nil {
		return nil, 0, err
	}

	return checkins, len(checkins), nil
}

// getCategoriesData retrieves categories data
func (s *Service) getCategoriesData(ctx context.Context, req *ServiceExportRequest) (interface{}, int, error) {
	type CategoryExport struct {
		ID        uuid.UUID  `json:"id"`
		UserID    uuid.UUID  `json:"user_id"`
		Name      string     `json:"name"`
		Color     string     `json:"color"`
		Icon      string     `json:"icon"`
		CreatedAt time.Time  `json:"created_at"`
		UpdatedAt time.Time  `json:"updated_at"`
		DeletedAt *time.Time `json:"deleted_at,omitempty"`
	}

	var categories []CategoryExport
	query := s.db.WithContext(ctx).
		Table("categories").
		Where("user_id = ?", req.UserID)

	if !req.Options.IncludeDeleted {
		query = query.Where("deleted_at IS NULL")
	}

	err := query.Find(&categories).Error
	if err != nil {
		return nil, 0, err
	}

	return categories, len(categories), nil
}

// getAnalyticsData retrieves analytics data
func (s *Service) getAnalyticsData(ctx context.Context, req *ServiceExportRequest) (interface{}, int, error) {
	type AnalyticsExport struct {
		Date             string `json:"date"`
		TotalCheckins    int    `json:"total_checkins"`
		TotalDuration    int    `json:"total_duration_minutes"`
		UniqueCategories int    `json:"unique_categories"`
		StreakDays       int    `json:"streak_days"`
	}

	// This would aggregate analytics data
	// Simplified for example
	analytics := []AnalyticsExport{
		{
			Date:             time.Now().Format("2006-01-02"),
			TotalCheckins:    10,
			TotalDuration:    450,
			UniqueCategories: 3,
			StreakDays:       5,
		},
	}

	return analytics, len(analytics), nil
}

// getFullData retrieves all user data
func (s *Service) getFullData(ctx context.Context, req *ServiceExportRequest) (interface{}, int, error) {
	type FullExport struct {
		Checkins   interface{} `json:"checkins"`
		Categories interface{} `json:"categories"`
		Analytics  interface{} `json:"analytics"`
		ExportedAt time.Time   `json:"exported_at"`
		UserID     uuid.UUID   `json:"user_id"`
	}

	checkins, checkinsCount, err := s.getCheckinsData(ctx, req)
	if err != nil {
		return nil, 0, err
	}

	categories, _, err := s.getCategoriesData(ctx, req)
	if err != nil {
		return nil, 0, err
	}

	analytics, _, err := s.getAnalyticsData(ctx, req)
	if err != nil {
		return nil, 0, err
	}

	fullExport := FullExport{
		Checkins:   checkins,
		Categories: categories,
		Analytics:  analytics,
		ExportedAt: time.Now(),
		UserID:     req.UserID,
	}

	return fullExport, checkinsCount, nil
}

// formatData formats data into the requested format
func (s *Service) formatData(data interface{}, format ExportFormat, options ExportOptions) ([]byte, error) {
	switch format {
	case FormatJSON:
		return s.formatJSON(data)
	case FormatCSV:
		return s.formatCSV(data)
	case FormatPDF:
		return s.formatPDF(data, options)
	case FormatExcel:
		return s.formatExcel(data)
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// formatJSON formats data as JSON
func (s *Service) formatJSON(data interface{}) ([]byte, error) {
	return json.MarshalIndent(data, "", "  ")
}

// formatCSV formats data as CSV
func (s *Service) formatCSV(data interface{}) ([]byte, error) {
	// Convert data to []map[string]interface{} for CSV formatting
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	var records []map[string]interface{}
	if err := json.Unmarshal(jsonData, &records); err != nil {
		// Try as single record
		var record map[string]interface{}
		if err := json.Unmarshal(jsonData, &record); err != nil {
			return nil, fmt.Errorf("data cannot be formatted as CSV")
		}
		records = []map[string]interface{}{record}
	}

	if len(records) == 0 {
		return []byte{}, nil
	}

	// Get headers from first record
	var headers []string
	for key := range records[0] {
		headers = append(headers, key)
	}

	// Create CSV
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	// Write headers
	if err := w.Write(headers); err != nil {
		return nil, err
	}

	// Write records
	for _, record := range records {
		row := make([]string, len(headers))
		for i, header := range headers {
			if val, ok := record[header]; ok {
				row[i] = fmt.Sprintf("%v", val)
			}
		}
		if err := w.Write(row); err != nil {
			return nil, err
		}
	}

	w.Flush()
	return buf.Bytes(), w.Error()
}

// formatPDF formats data as PDF
func (s *Service) formatPDF(data interface{}, options ExportOptions) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 16)

	// Title
	pdf.Cell(190, 10, "Data Export Report")
	pdf.Ln(12)

	// Metadata
	pdf.SetFont("Arial", "", 10)
	pdf.Cell(190, 6, fmt.Sprintf("Generated: %s", time.Now().Format(options.DateFormat)))
	pdf.Ln(8)

	// Convert data to JSON for display
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, err
	}

	// Add content (simplified - in production, you'd format this better)
	pdf.SetFont("Arial", "", 8)
	pdf.MultiCell(190, 4, string(jsonData), "", "", false)

	var buf bytes.Buffer
	err = pdf.Output(&buf)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// formatExcel formats data as Excel (simplified - would use excelize library in production)
func (s *Service) formatExcel(data interface{}) ([]byte, error) {
	// For simplicity, convert to CSV
	// In production, use github.com/xuri/excelize/v2
	return s.formatCSV(data)
}

// generateFileName generates a filename for the export
func (s *Service) generateFileName(req *ServiceExportRequest) string {
	timestamp := time.Now().Format("20060102-150405")
	return fmt.Sprintf("export-%s-%s-%s.%s",
		req.Type,
		req.UserID.String()[:8],
		timestamp,
		req.Format,
	)
}

// saveExportRecord saves an export record to the database
func (s *Service) saveExportRecord(ctx context.Context, result *ExportResult) error {
	// In production, you'd save this to a database table
	// For now, just log it
	s.logger.Info("Export created",
		zap.String("id", result.ID.String()),
		zap.String("user_id", result.UserID.String()),
		zap.String("type", string(result.Type)),
		zap.String("format", string(result.Format)),
		zap.Int("record_count", result.RecordCount),
		zap.Int64("size", result.Size),
	)
	return nil
}

// GetExportHistory gets export history for a user
func (s *Service) GetExportHistory(ctx context.Context, userID uuid.UUID, limit int) ([]*ExportResult, error) {
	// In production, query from database
	// For now, return empty
	return []*ExportResult{}, nil
}

// CleanupExpiredExports removes expired exports
func (s *Service) CleanupExpiredExports(ctx context.Context) error {
	// In production, delete expired exports from storage and database
	s.logger.Info("Cleaning up expired exports")
	return nil
}