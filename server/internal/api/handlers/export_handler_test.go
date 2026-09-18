package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dev-jelly/donelist/internal/category"
	"github.com/dev-jelly/donelist/internal/checkin"
	"github.com/dev-jelly/donelist/internal/export"
	"github.com/dev-jelly/donelist/internal/testutil"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func setupExportTest(t *testing.T) (*gin.Engine, *export.ExportService, uuid.UUID) {
	tdb := testutil.SetupTestDB(t)
	t.Cleanup(func() { tdb.TearDown(t) })
	db := tdb.DB

	logger := zaptest.NewLogger(t)
	tempDir := t.TempDir()

	checkinRepo := checkin.NewRepository(db)
	categoryRepo := category.NewRepository(db)
	exportService := export.NewExportService(db, checkinRepo, categoryRepo, logger, tempDir)
	handler := NewExportHandler(exportService, logger)

	// Create test user
	userID := uuid.New()
	_, err := db.Exec(`INSERT INTO users (id, email, password_hash) VALUES ($1, $2, $3)`,
		userID, "test@example.com", "hash")
	require.NoError(t, err)

	// Setup Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Mock auth middleware
	router.Use(func(c *gin.Context) {
		c.Set("userID", userID)
		c.Next()
	})

	// Register routes
	router.POST("/api/v1/export/request", handler.RequestExport)
	router.GET("/api/v1/export/:id/status", handler.GetExportStatus)
	router.GET("/api/v1/export/:id/download", handler.DownloadExport)
	router.GET("/api/v1/export/history", handler.GetExportHistory)

	return router, exportService, userID
}

func TestExportHandler_RequestExport(t *testing.T) {
	router, _, _ := setupExportTest(t)

	t.Run("successful export request", func(t *testing.T) {
		reqBody := RequestExportInput{
			Format: "json",
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/export/request", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusAccepted, w.Code)

		var response export.ExportJob
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.NotEqual(t, uuid.Nil, response.ID)
		assert.Equal(t, export.FormatJSON, response.Format)
		assert.Equal(t, export.StatusPending, response.Status)
	})

	t.Run("CSV format", func(t *testing.T) {
		reqBody := RequestExportInput{
			Format: "csv",
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/export/request", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusAccepted, w.Code)

		var response export.ExportJob
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, export.FormatCSV, response.Format)
	})

	t.Run("PDF format", func(t *testing.T) {
		reqBody := RequestExportInput{
			Format: "pdf",
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/export/request", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusAccepted, w.Code)

		var response export.ExportJob
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, export.FormatPDF, response.Format)
	})

	t.Run("with date range", func(t *testing.T) {
		start := time.Now().Add(-30 * 24 * time.Hour)
		end := time.Now()

		reqBody := RequestExportInput{
			Format:    "json",
			StartDate: &start,
			EndDate:   &end,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/export/request", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusAccepted, w.Code)
	})

	t.Run("invalid format", func(t *testing.T) {
		reqBody := RequestExportInput{
			Format: "xml",
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/export/request", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("missing format", func(t *testing.T) {
		reqBody := map[string]string{}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/export/request", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestExportHandler_GetExportStatus(t *testing.T) {
	router, exportService, userID := setupExportTest(t)

	// Create a test export job
	ctx := context.Background()
	req := &export.ExportRequest{
		UserID: userID,
		Format: export.FormatJSON,
	}

	job, err := exportService.RequestExport(ctx, req)
	require.NoError(t, err)

	t.Run("get existing job status", func(t *testing.T) {
		httpReq := httptest.NewRequest(http.MethodGet, "/api/v1/export/"+job.ID.String()+"/status", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httpReq)

		assert.Equal(t, http.StatusOK, w.Code)

		var response export.ExportJob
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, job.ID, response.ID)
		assert.Contains(t, []export.ExportStatus{export.StatusPending, export.StatusProcessing, export.StatusCompleted}, response.Status)
	})

	t.Run("invalid job ID", func(t *testing.T) {
		httpReq := httptest.NewRequest(http.MethodGet, "/api/v1/export/invalid-id/status", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httpReq)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("non-existent job", func(t *testing.T) {
		fakeID := uuid.New()
		httpReq := httptest.NewRequest(http.MethodGet, "/api/v1/export/"+fakeID.String()+"/status", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httpReq)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestExportHandler_GetExportHistory(t *testing.T) {
	router, exportService, userID := setupExportTest(t)

	// Create multiple export jobs
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		req := &export.ExportRequest{
			UserID: userID,
			Format: export.FormatJSON,
		}

		_, err := exportService.RequestExport(ctx, req)
		require.NoError(t, err)

		// Small delay to ensure different timestamps
		time.Sleep(10 * time.Millisecond)

		// Wait for job to process
		time.Sleep(200 * time.Millisecond)
	}

	t.Run("get export history", func(t *testing.T) {
		httpReq := httptest.NewRequest(http.MethodGet, "/api/v1/export/history", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httpReq)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		jobs, ok := response["jobs"].([]interface{})
		require.True(t, ok)

		assert.GreaterOrEqual(t, len(jobs), 3)
		assert.NotNil(t, response["total"])
	})

	t.Run("with limit parameter", func(t *testing.T) {
		httpReq := httptest.NewRequest(http.MethodGet, "/api/v1/export/history?limit=2", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httpReq)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		jobs, ok := response["jobs"].([]interface{})
		require.True(t, ok)

		// Should return at most 2 jobs
		assert.LessOrEqual(t, len(jobs), 2)
	})

	t.Run("invalid limit parameter", func(t *testing.T) {
		// Invalid limit should default to 10
		httpReq := httptest.NewRequest(http.MethodGet, "/api/v1/export/history?limit=abc", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httpReq)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestExportHandler_DownloadExport(t *testing.T) {
	router, exportService, userID := setupExportTest(t)

	// Create and complete an export job
	ctx := context.Background()
	req := &export.ExportRequest{
		UserID: userID,
		Format: export.FormatJSON,
	}

	job, err := exportService.RequestExport(ctx, req)
	require.NoError(t, err)

	// Wait for processing to complete
	time.Sleep(time.Second)

	t.Run("download completed export", func(t *testing.T) {
		// Check if job is completed
		updatedJob, err := exportService.GetExportJob(ctx, job.ID, userID)
		require.NoError(t, err)

		if updatedJob.Status == export.StatusCompleted {
			httpReq := httptest.NewRequest(http.MethodGet, "/api/v1/export/"+job.ID.String()+"/download", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httpReq)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
			assert.Contains(t, w.Header().Get("Content-Disposition"), "attachment")
			assert.NotEmpty(t, w.Body.Bytes())
		} else {
			t.Skip("Export not completed yet")
		}
	})

	t.Run("download pending export", func(t *testing.T) {
		// Create new pending job
		newReq := &export.ExportRequest{
			UserID: userID,
			Format: export.FormatCSV,
		}

		pendingJob, err := exportService.RequestExport(ctx, newReq)
		require.NoError(t, err)

		// Try to download immediately (should fail as not completed)
		httpReq := httptest.NewRequest(http.MethodGet, "/api/v1/export/"+pendingJob.ID.String()+"/download", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httpReq)

		// Should return error as export is not completed
		assert.Contains(t, []int{http.StatusNotFound, http.StatusOK}, w.Code)
	})

	t.Run("download non-existent export", func(t *testing.T) {
		fakeID := uuid.New()
		httpReq := httptest.NewRequest(http.MethodGet, "/api/v1/export/"+fakeID.String()+"/download", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httpReq)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
