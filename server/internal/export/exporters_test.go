package export

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func getTestExportData() *ExportData {
	desc := "Test description"
	return &ExportData{
		Checkins: []CheckinExportData{
			{
				ID:          uuid.New(),
				Title:       "Test Checkin 1",
				Description: &desc,
				Category:    "Work",
				Tags:        []string{"important", "urgent"},
				Priority:    "high",
				Status:      "completed",
				CreatedAt:   time.Now().Add(-24 * time.Hour),
				UpdatedAt:   time.Now(),
			},
			{
				ID:          uuid.New(),
				Title:       "Test Checkin 2",
				Description: nil,
				Category:    "Personal",
				Tags:        []string{"routine"},
				Priority:    "medium",
				Status:      "pending",
				CreatedAt:   time.Now().Add(-12 * time.Hour),
				UpdatedAt:   time.Now(),
			},
		},
		Metadata: ExportMetadata{
			ExportedAt:    time.Now(),
			ExportFormat:  FormatCSV,
			TotalRecords:  2,
			FilterApplied: false,
		},
	}
}

func TestCSVExporter(t *testing.T) {
	logger := zaptest.NewLogger(t)
	exporter := NewCSVExporter(logger)

	t.Run("export to CSV", func(t *testing.T) {
		data := getTestExportData()
		result, err := exporter.Export(data)

		require.NoError(t, err)
		assert.NotEmpty(t, result)

		// Verify CSV content
		csvContent := string(result)
		assert.Contains(t, csvContent, "ID,Title,Description,Category,Tags,Priority,Status")
		assert.Contains(t, csvContent, "Test Checkin 1")
		assert.Contains(t, csvContent, "Test Checkin 2")
		assert.Contains(t, csvContent, "Work")
		assert.Contains(t, csvContent, "important,urgent")
	})

	t.Run("empty data", func(t *testing.T) {
		data := &ExportData{
			Checkins: []CheckinExportData{},
			Metadata: ExportMetadata{
				ExportedAt:    time.Now(),
				ExportFormat:  FormatCSV,
				TotalRecords:  0,
				FilterApplied: false,
			},
		}

		result, err := exporter.Export(data)
		require.NoError(t, err)

		// Should still have headers
		csvContent := string(result)
		assert.Contains(t, csvContent, "ID,Title,Description")
	})

	t.Run("mime type and extension", func(t *testing.T) {
		assert.Equal(t, "text/csv", exporter.GetMimeType())
		assert.Equal(t, "csv", exporter.GetFileExtension())
	})
}

func TestJSONExporter(t *testing.T) {
	logger := zaptest.NewLogger(t)
	exporter := NewJSONExporter(logger)

	t.Run("export to JSON", func(t *testing.T) {
		data := getTestExportData()
		result, err := exporter.Export(data)

		require.NoError(t, err)
		assert.NotEmpty(t, result)

		// Verify JSON content
		jsonContent := string(result)
		assert.Contains(t, jsonContent, "\"checkins\"")
		assert.Contains(t, jsonContent, "\"metadata\"")
		assert.Contains(t, jsonContent, "Test Checkin 1")
		assert.Contains(t, jsonContent, "Test Checkin 2")
		assert.Contains(t, jsonContent, "\"priority\": \"high\"")

		// Should be pretty-printed (indented)
		assert.True(t, strings.Contains(jsonContent, "\n"))
	})

	t.Run("empty data", func(t *testing.T) {
		data := &ExportData{
			Checkins: []CheckinExportData{},
			Metadata: ExportMetadata{
				ExportedAt:    time.Now(),
				ExportFormat:  FormatJSON,
				TotalRecords:  0,
				FilterApplied: false,
			},
		}

		result, err := exporter.Export(data)
		require.NoError(t, err)

		jsonContent := string(result)
		assert.Contains(t, jsonContent, "\"checkins\": []")
	})

	t.Run("mime type and extension", func(t *testing.T) {
		assert.Equal(t, "application/json", exporter.GetMimeType())
		assert.Equal(t, "json", exporter.GetFileExtension())
	})
}

func TestPDFExporter(t *testing.T) {
	logger := zaptest.NewLogger(t)
	exporter := NewPDFExporter(logger)

	t.Run("export to PDF", func(t *testing.T) {
		data := getTestExportData()
		result, err := exporter.Export(data)

		require.NoError(t, err)
		assert.NotEmpty(t, result)

		// Verify PDF magic number (PDF files start with %PDF)
		assert.True(t, len(result) > 4)
		assert.Equal(t, "%PDF", string(result[:4]))
	})

	t.Run("empty data", func(t *testing.T) {
		data := &ExportData{
			Checkins: []CheckinExportData{},
			Metadata: ExportMetadata{
				ExportedAt:    time.Now(),
				ExportFormat:  FormatPDF,
				TotalRecords:  0,
				FilterApplied: false,
			},
		}

		result, err := exporter.Export(data)
		require.NoError(t, err)

		// Should still generate valid PDF
		assert.Equal(t, "%PDF", string(result[:4]))
	})

	t.Run("large dataset pagination", func(t *testing.T) {
		// Create data with many checkins to test pagination
		checkins := make([]CheckinExportData, 50)
		for i := 0; i < 50; i++ {
			desc := "Description for checkin"
			checkins[i] = CheckinExportData{
				ID:          uuid.New(),
				Title:       "Checkin " + string(rune(i+1)),
				Description: &desc,
				Category:    "Test",
				Tags:        []string{"tag1", "tag2"},
				Priority:    "medium",
				Status:      "completed",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}
		}

		data := &ExportData{
			Checkins: checkins,
			Metadata: ExportMetadata{
				ExportedAt:    time.Now(),
				ExportFormat:  FormatPDF,
				TotalRecords:  50,
				FilterApplied: false,
			},
		}

		result, err := exporter.Export(data)
		require.NoError(t, err)
		assert.NotEmpty(t, result)

		// Should handle multiple pages
		assert.True(t, len(result) > 1000) // Reasonable size for 50 records
	})

	t.Run("mime type and extension", func(t *testing.T) {
		assert.Equal(t, "application/pdf", exporter.GetMimeType())
		assert.Equal(t, "pdf", exporter.GetFileExtension())
	})

	t.Run("special characters in content", func(t *testing.T) {
		specialDesc := "Content with special chars: @#$%^&*()"
		data := &ExportData{
			Checkins: []CheckinExportData{
				{
					ID:          uuid.New(),
					Title:       "Test <script>alert('xss')</script>",
					Description: &specialDesc,
					Category:    "Test & Demo",
					Tags:        []string{"test", "special-chars"},
					Priority:    "low",
					Status:      "completed",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				},
			},
			Metadata: ExportMetadata{
				ExportedAt:    time.Now(),
				ExportFormat:  FormatPDF,
				TotalRecords:  1,
				FilterApplied: false,
			},
		}

		result, err := exporter.Export(data)
		require.NoError(t, err)
		assert.NotEmpty(t, result)

		// Should generate valid PDF even with special characters
		assert.Equal(t, "%PDF", string(result[:4]))
	})
}

func TestExportWithDateRange(t *testing.T) {
	logger := zaptest.NewLogger(t)

	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC)

	data := &ExportData{
		Checkins: []CheckinExportData{
			{
				ID:        uuid.New(),
				Title:     "Q1 Checkin",
				Category:  "Work",
				Tags:      []string{"q1"},
				Priority:  "high",
				Status:    "completed",
				CreatedAt: time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC),
			},
		},
		Metadata: ExportMetadata{
			ExportedAt:    time.Now(),
			ExportFormat:  FormatJSON,
			TotalRecords:  1,
			FilterApplied: true,
			DateRange: &DateRange{
				Start: start,
				End:   end,
			},
		},
	}

	t.Run("JSON with date range", func(t *testing.T) {
		exporter := NewJSONExporter(logger)
		result, err := exporter.Export(data)

		require.NoError(t, err)
		jsonContent := string(result)
		assert.Contains(t, jsonContent, "2024-01-01")
		assert.Contains(t, jsonContent, "2024-12-31")
		assert.Contains(t, jsonContent, "\"filter_applied\": true")
	})

	t.Run("PDF with date range", func(t *testing.T) {
		data.Metadata.ExportFormat = FormatPDF
		exporter := NewPDFExporter(logger)
		result, err := exporter.Export(data)

		require.NoError(t, err)
		assert.Equal(t, "%PDF", string(result[:4]))
		// PDF should contain date range information
		assert.True(t, len(result) > 100)
	})
}
