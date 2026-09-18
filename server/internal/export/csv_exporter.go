package export

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strings"

	"go.uber.org/zap"
)

// CSVExporter exports data to CSV format
type CSVExporter struct {
	logger *zap.Logger
}

// NewCSVExporter creates a new CSV exporter
func NewCSVExporter(logger *zap.Logger) *CSVExporter {
	return &CSVExporter{
		logger: logger,
	}
}

// Export exports data to CSV format
func (e *CSVExporter) Export(data *ExportData) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Write header
	header := []string{
		"ID",
		"Title",
		"Description",
		"Category",
		"Tags",
		"Priority",
		"Status",
		"Created At",
		"Updated At",
	}

	if err := writer.Write(header); err != nil {
		return nil, fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write records
	for _, checkin := range data.Checkins {
		description := ""
		if checkin.Description != nil {
			description = *checkin.Description
		}

		record := []string{
			checkin.ID.String(),
			checkin.Title,
			description,
			checkin.Category,
			strings.Join(checkin.Tags, ","),
			checkin.Priority,
			checkin.Status,
			checkin.CreatedAt.Format("2006-01-02 15:04:05"),
			checkin.UpdatedAt.Format("2006-01-02 15:04:05"),
		}

		if err := writer.Write(record); err != nil {
			return nil, fmt.Errorf("failed to write CSV record: %w", err)
		}
	}

	writer.Flush()

	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("CSV writer error: %w", err)
	}

	return buf.Bytes(), nil
}

// GetMimeType returns the MIME type for CSV
func (e *CSVExporter) GetMimeType() string {
	return "text/csv"
}

// GetFileExtension returns the file extension for CSV
func (e *CSVExporter) GetFileExtension() string {
	return "csv"
}
