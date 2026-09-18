package export

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dev-jelly/donelist/internal/category"
	"github.com/dev-jelly/donelist/internal/checkin"
	"github.com/dev-jelly/donelist/internal/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestExportService_RequestExport(t *testing.T) {
	tdb := testutil.SetupTestDB(t)
	defer tdb.TearDown(t)
	db := tdb.DB

	logger := zaptest.NewLogger(t)
	tempDir := t.TempDir()

	checkinRepo := checkin.NewRepository(db)
	categoryRepo := category.NewRepository(db)
	exportService := NewExportService(db, checkinRepo, categoryRepo, logger, tempDir)

	ctx := context.Background()

	// Create test user
	userID := uuid.New()
	_, err := db.Exec(`INSERT INTO users (id, email, password_hash) VALUES ($1, $2, $3)`,
		userID, "test@example.com", "hash")
	require.NoError(t, err)

	t.Run("successful export request", func(t *testing.T) {
		req := &ExportRequest{
			UserID: userID,
			Format: FormatJSON,
		}

		job, err := exportService.RequestExport(ctx, req)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, job.ID)
		assert.Equal(t, userID, job.UserID)
		assert.Equal(t, FormatJSON, job.Format)
		assert.Equal(t, StatusPending, job.Status)

		// Wait a bit for background processing
		time.Sleep(500 * time.Millisecond)

		// Check job status
		updatedJob, err := exportService.GetExportJob(ctx, job.ID, userID)
		require.NoError(t, err)
		assert.Contains(t, []ExportStatus{StatusProcessing, StatusCompleted}, updatedJob.Status)
	})

	t.Run("prevent duplicate exports", func(t *testing.T) {
		req := &ExportRequest{
			UserID: userID,
			Format: FormatCSV,
		}

		// First request
		_, err := exportService.RequestExport(ctx, req)
		require.NoError(t, err)

		// Second request should fail
		_, err = exportService.RequestExport(ctx, req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already have an export in progress")
	})

	t.Run("invalid format", func(t *testing.T) {
		req := &ExportRequest{
			UserID: userID,
			Format: "invalid",
		}

		_, err := exportService.RequestExport(ctx, req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid format")
	})

	t.Run("date range validation", func(t *testing.T) {
		start := time.Now()
		end := start.Add(-24 * time.Hour) // End before start

		req := &ExportRequest{
			UserID:    userID,
			Format:    FormatPDF,
			StartDate: &start,
			EndDate:   &end,
		}

		_, err := exportService.RequestExport(ctx, req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "start date must be before end date")
	})
}

func TestExportService_GetExportJob(t *testing.T) {
	tdb := testutil.SetupTestDB(t)
	defer tdb.TearDown(t)
	db := tdb.DB

	logger := zaptest.NewLogger(t)
	tempDir := t.TempDir()

	checkinRepo := checkin.NewRepository(db)
	categoryRepo := category.NewRepository(db)
	exportService := NewExportService(db, checkinRepo, categoryRepo, logger, tempDir)

	ctx := context.Background()

	// Create test users
	userID1 := uuid.New()
	userID2 := uuid.New()
	_, err := db.Exec(`INSERT INTO users (id, email, password_hash) VALUES ($1, $2, $3), ($4, $5, $6)`,
		userID1, "user1@example.com", "hash",
		userID2, "user2@example.com", "hash")
	require.NoError(t, err)

	// Create export job for user1
	req := &ExportRequest{
		UserID: userID1,
		Format: FormatJSON,
	}

	job, err := exportService.RequestExport(ctx, req)
	require.NoError(t, err)

	t.Run("owner can access job", func(t *testing.T) {
		retrieved, err := exportService.GetExportJob(ctx, job.ID, userID1)
		require.NoError(t, err)
		assert.Equal(t, job.ID, retrieved.ID)
	})

	t.Run("non-owner cannot access job", func(t *testing.T) {
		_, err := exportService.GetExportJob(ctx, job.ID, userID2)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unauthorized")
	})
}

func TestExportService_CleanupExpiredExports(t *testing.T) {
	tdb := testutil.SetupTestDB(t)
	defer tdb.TearDown(t)
	db := tdb.DB

	logger := zaptest.NewLogger(t)
	tempDir := t.TempDir()

	checkinRepo := checkin.NewRepository(db)
	categoryRepo := category.NewRepository(db)
	exportService := NewExportService(db, checkinRepo, categoryRepo, logger, tempDir)

	ctx := context.Background()

	// Create test user
	userID := uuid.New()
	_, err := db.Exec(`INSERT INTO users (id, email, password_hash) VALUES ($1, $2, $3)`,
		userID, "test@example.com", "hash")
	require.NoError(t, err)

	// Create expired export job
	expiredJob := &ExportJob{
		ID:          uuid.New(),
		UserID:      userID,
		Format:      FormatCSV,
		Status:      StatusCompleted,
		RecordCount: 10,
		ExpiresAt:   time.Now().Add(-1 * time.Hour), // Expired
		CreatedAt:   time.Now().Add(-8 * 24 * time.Hour),
	}

	repo := NewRepository(db)
	err = repo.Create(ctx, expiredJob)
	require.NoError(t, err)

	// Create old file
	oldFile := filepath.Join(tempDir, "old_export.csv")
	err = os.WriteFile(oldFile, []byte("test"), 0644)
	require.NoError(t, err)

	// Change file modification time to 8 days ago
	oldTime := time.Now().Add(-8 * 24 * time.Hour)
	err = os.Chtimes(oldFile, oldTime, oldTime)
	require.NoError(t, err)

	// Run cleanup
	err = exportService.CleanupExpiredExports(ctx)
	assert.NoError(t, err)

	// Verify expired job is deleted
	_, err = repo.GetByID(ctx, expiredJob.ID)
	assert.Error(t, err)

	// Verify old file is removed
	_, err = os.Stat(oldFile)
	assert.True(t, os.IsNotExist(err))
}

func TestExportService_GetExportFile(t *testing.T) {
	tdb := testutil.SetupTestDB(t)
	defer tdb.TearDown(t)
	db := tdb.DB

	logger := zaptest.NewLogger(t)
	tempDir := t.TempDir()

	checkinRepo := checkin.NewRepository(db)
	categoryRepo := category.NewRepository(db)
	exportService := NewExportService(db, checkinRepo, categoryRepo, logger, tempDir)

	ctx := context.Background()

	// Create test user
	userID := uuid.New()
	_, err := db.Exec(`INSERT INTO users (id, email, password_hash) VALUES ($1, $2, $3)`,
		userID, "test@example.com", "hash")
	require.NoError(t, err)

	// Create completed export job with file
	filePath := filepath.Join(tempDir, "test_export.json")
	err = os.WriteFile(filePath, []byte(`{"test": "data"}`), 0644)
	require.NoError(t, err)

	fileSize := int64(16)
	completedAt := time.Now()

	job := &ExportJob{
		ID:          uuid.New(),
		UserID:      userID,
		Format:      FormatJSON,
		Status:      StatusCompleted,
		FilePath:    &filePath,
		FileSize:    &fileSize,
		RecordCount: 1,
		CompletedAt: &completedAt,
		ExpiresAt:   time.Now().Add(7 * 24 * time.Hour),
		CreatedAt:   time.Now(),
	}

	repo := NewRepository(db)
	err = repo.Create(ctx, job)
	require.NoError(t, err)

	t.Run("get existing file", func(t *testing.T) {
		path, err := exportService.GetExportFile(ctx, job.ID, userID)
		require.NoError(t, err)
		assert.Equal(t, filePath, path)
	})

	t.Run("incomplete job returns error", func(t *testing.T) {
		pendingJob := &ExportJob{
			ID:          uuid.New(),
			UserID:      userID,
			Format:      FormatJSON,
			Status:      StatusPending,
			RecordCount: 0,
			ExpiresAt:   time.Now().Add(7 * 24 * time.Hour),
			CreatedAt:   time.Now(),
		}

		err := repo.Create(ctx, pendingJob)
		require.NoError(t, err)

		_, err = exportService.GetExportFile(ctx, pendingJob.ID, userID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not yet completed")
	})
}
