package handlers

import (
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/dev-jelly/donelist/internal/export"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ExportHandler handles data export HTTP requests
type ExportHandler struct {
	exportService *export.ExportService
	logger        *zap.Logger
}

// NewExportHandler creates a new export handler
func NewExportHandler(exportService *export.ExportService, logger *zap.Logger) *ExportHandler {
	return &ExportHandler{
		exportService: exportService,
		logger:        logger,
	}
}

// RequestExportInput represents the input for requesting an export
type RequestExportInput struct {
	Format    string     `json:"format" binding:"required,oneof=csv json pdf"`
	StartDate *time.Time `json:"start_date,omitempty"`
	EndDate   *time.Time `json:"end_date,omitempty"`
}

// RequestExport handles POST /api/v1/export/request
// @Summary Request a data export
// @Description Request a data export in CSV, JSON, or PDF format. The export is processed asynchronously.
// @Tags export
// @Accept json
// @Produce json
// @Param request body RequestExportInput true "Export request parameters"
// @Success 202 {object} export.ExportJob "Export job created"
// @Failure 400 {object} map[string]string "Invalid request"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 429 {object} map[string]string "Export already in progress"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /export/request [post]
func (h *ExportHandler) RequestExport(c *gin.Context) {
	// Get user ID from context (set by auth middleware)
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	uid, ok := userID.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user ID format"})
		return
	}

	// Parse request body
	var input RequestExportInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Convert format string to ExportFormat
	var format export.ExportFormat
	switch input.Format {
	case "csv":
		format = export.FormatCSV
	case "json":
		format = export.FormatJSON
	case "pdf":
		format = export.FormatPDF
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid format"})
		return
	}

	// Create export request
	req := &export.ExportRequest{
		UserID:    uid,
		Format:    format,
		StartDate: input.StartDate,
		EndDate:   input.EndDate,
	}

	// Request export
	job, err := h.exportService.RequestExport(c.Request.Context(), req)
	if err != nil {
		if err.Error() == "you already have an export in progress" {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
			return
		}

		h.logger.Error("Failed to request export",
			zap.Error(err),
			zap.String("user_id", uid.String()),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to request export"})
		return
	}

	h.logger.Info("Export requested",
		zap.String("job_id", job.ID.String()),
		zap.String("user_id", uid.String()),
		zap.String("format", string(format)),
	)

	c.JSON(http.StatusAccepted, job)
}

// GetExportStatus handles GET /api/v1/export/:id/status
// @Summary Get export status
// @Description Get the status of an export job
// @Tags export
// @Produce json
// @Param id path string true "Export Job ID"
// @Success 200 {object} export.ExportJob "Export job details"
// @Failure 400 {object} map[string]string "Invalid job ID"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "Export job not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /export/{id}/status [get]
func (h *ExportHandler) GetExportStatus(c *gin.Context) {
	// Get user ID from context
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	uid, ok := userID.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user ID format"})
		return
	}

	// Parse job ID from URL
	jobIDStr := c.Param("id")
	jobID, err := uuid.Parse(jobIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job ID"})
		return
	}

	// Get export job
	job, err := h.exportService.GetExportJob(c.Request.Context(), jobID, uid)
	if err != nil {
		if err.Error() == "unauthorized access to export job" {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}

		h.logger.Error("Failed to get export job",
			zap.Error(err),
			zap.String("job_id", jobID.String()),
		)
		c.JSON(http.StatusNotFound, gin.H{"error": "export job not found"})
		return
	}

	c.JSON(http.StatusOK, job)
}

// DownloadExport handles GET /api/v1/export/:id/download
// @Summary Download export file
// @Description Download the exported data file. Only available for completed exports.
// @Tags export
// @Produce octet-stream
// @Param id path string true "Export Job ID"
// @Success 200 {file} file "Export file"
// @Failure 400 {object} map[string]string "Invalid job ID"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "Export not found or not ready"
// @Failure 410 {object} map[string]string "Export has expired"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /export/{id}/download [get]
func (h *ExportHandler) DownloadExport(c *gin.Context) {
	// Get user ID from context
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	uid, ok := userID.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user ID format"})
		return
	}

	// Parse job ID from URL
	jobIDStr := c.Param("id")
	jobID, err := uuid.Parse(jobIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job ID"})
		return
	}

	// Get export file path
	filePath, err := h.exportService.GetExportFile(c.Request.Context(), jobID, uid)
	if err != nil {
		statusCode := http.StatusNotFound
		if err.Error() == "export has expired" {
			statusCode = http.StatusGone
		} else if err.Error() == "unauthorized access to export job" {
			statusCode = http.StatusForbidden
		}

		h.logger.Warn("Failed to get export file",
			zap.Error(err),
			zap.String("job_id", jobID.String()),
		)
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}

	// Get job to determine filename
	job, err := h.exportService.GetExportJob(c.Request.Context(), jobID, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get job details"})
		return
	}

	// Determine content type
	var contentType string
	switch job.Format {
	case export.FormatCSV:
		contentType = "text/csv"
	case export.FormatJSON:
		contentType = "application/json"
	case export.FormatPDF:
		contentType = "application/pdf"
	default:
		contentType = "application/octet-stream"
	}

	// Generate download filename
	timestamp := job.CreatedAt.Format("20060102")
	filename := filepath.Base(filePath)
	downloadName := timestamp + "_donelist_export." + string(job.Format)

	h.logger.Info("Export downloaded",
		zap.String("job_id", jobID.String()),
		zap.String("user_id", uid.String()),
		zap.String("filename", filename),
	)

	// Serve file
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Transfer-Encoding", "binary")
	c.Header("Content-Disposition", "attachment; filename="+downloadName)
	c.Header("Content-Type", contentType)
	c.File(filePath)
}

// GetExportHistory handles GET /api/v1/export/history
// @Summary Get export history
// @Description Get the list of recent export jobs for the current user
// @Tags export
// @Produce json
// @Param limit query int false "Maximum number of jobs to return" default(10)
// @Success 200 {array} export.ExportJob "List of export jobs"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /export/history [get]
func (h *ExportHandler) GetExportHistory(c *gin.Context) {
	// Get user ID from context
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	uid, ok := userID.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user ID format"})
		return
	}

	// Parse limit query parameter
	limit := 10
	if limitStr := c.Query("limit"); limitStr != "" {
		var err error
		if _, err = fmt.Sscanf(limitStr, "%d", &limit); err != nil || limit < 1 || limit > 50 {
			limit = 10
		}
	}

	// Get export history
	jobs, err := h.exportService.GetExportHistory(c.Request.Context(), uid, limit)
	if err != nil {
		h.logger.Error("Failed to get export history",
			zap.Error(err),
			zap.String("user_id", uid.String()),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get export history"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"jobs":  jobs,
		"total": len(jobs),
	})
}
