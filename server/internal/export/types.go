package export

import (
	"time"

	"github.com/google/uuid"
)

// ExportFormat represents the format of the export
type ExportFormat string

const (
	FormatCSV   ExportFormat = "csv"
	FormatJSON  ExportFormat = "json"
	FormatPDF   ExportFormat = "pdf"
	FormatExcel ExportFormat = "xlsx"
	FormatXML   ExportFormat = "xml"
)

// ExportStatus represents the status of an export job
type ExportStatus string

const (
	StatusPending    ExportStatus = "pending"
	StatusProcessing ExportStatus = "processing"
	StatusCompleted  ExportStatus = "completed"
	StatusFailed     ExportStatus = "failed"
)

// ExportJob represents an export job
type ExportJob struct {
	ID          uuid.UUID    `json:"id" db:"id"`
	UserID      uuid.UUID    `json:"user_id" db:"user_id"`
	Format      ExportFormat `json:"format" db:"format"`
	Status      ExportStatus `json:"status" db:"status"`
	FilePath    *string      `json:"file_path,omitempty" db:"file_path"`
	FileSize    *int64       `json:"file_size,omitempty" db:"file_size"`
	RecordCount int          `json:"record_count" db:"record_count"`
	Error       *string      `json:"error,omitempty" db:"error"`
	StartedAt   *time.Time   `json:"started_at,omitempty" db:"started_at"`
	CompletedAt *time.Time   `json:"completed_at,omitempty" db:"completed_at"`
	ExpiresAt   time.Time    `json:"expires_at" db:"expires_at"`
	CreatedAt   time.Time    `json:"created_at" db:"created_at"`
}

// ExportRequest represents a request to export data
type ExportRequest struct {
	UserID      uuid.UUID    `json:"user_id"`
	Format      ExportFormat `json:"format" binding:"required"`
	StartDate   *time.Time   `json:"start_date,omitempty"`
	EndDate     *time.Time   `json:"end_date,omitempty"`
	CategoryIDs []uuid.UUID  `json:"category_ids,omitempty"`
	Tags        []string     `json:"tags,omitempty"`
}

// ExportData represents the data to be exported
type ExportData struct {
	Checkins []CheckinExportData `json:"checkins"`
	Metadata ExportMetadata      `json:"metadata"`
}

// CheckinExportData represents a checkin for export
type CheckinExportData struct {
	ID          uuid.UUID  `json:"id" csv:"id"`
	Title       string     `json:"title" csv:"title"`
	Description *string    `json:"description,omitempty" csv:"description"`
	Category    string     `json:"category" csv:"category"`
	Tags        []string   `json:"tags" csv:"tags"`
	Priority    string     `json:"priority" csv:"priority"`
	Status      string     `json:"status" csv:"status"`
	CreatedAt   time.Time  `json:"created_at" csv:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" csv:"updated_at"`
}

// ExportMetadata represents metadata about the export
type ExportMetadata struct {
	ExportedAt    time.Time    `json:"exported_at"`
	ExportFormat  ExportFormat `json:"export_format"`
	TotalRecords  int          `json:"total_records"`
	FilterApplied bool         `json:"filter_applied"`
	DateRange     *DateRange   `json:"date_range,omitempty"`
}

// DateRange represents a date range filter
type DateRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Exporter interface defines the contract for format-specific exporters
type Exporter interface {
	// Export exports data to the specified format
	Export(data *ExportData) ([]byte, error)

	// GetMimeType returns the MIME type for the format
	GetMimeType() string

	// GetFileExtension returns the file extension for the format
	GetFileExtension() string
}
