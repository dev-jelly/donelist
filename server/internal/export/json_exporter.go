package export

import (
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
)

// JSONExporter exports data to JSON format
type JSONExporter struct {
	logger *zap.Logger
}

// NewJSONExporter creates a new JSON exporter
func NewJSONExporter(logger *zap.Logger) *JSONExporter {
	return &JSONExporter{
		logger: logger,
	}
}

// Export exports data to JSON format
func (e *JSONExporter) Export(data *ExportData) ([]byte, error) {
	// Pretty print JSON
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return jsonData, nil
}

// GetMimeType returns the MIME type for JSON
func (e *JSONExporter) GetMimeType() string {
	return "application/json"
}

// GetFileExtension returns the file extension for JSON
func (e *JSONExporter) GetFileExtension() string {
	return "json"
}
