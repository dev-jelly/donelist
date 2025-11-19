package integration

import (
	"context"
	"testing"
	"time"

	"github.com/dev-jelly/donelist/internal/category"
	"github.com/dev-jelly/donelist/internal/checkin"
	"github.com/dev-jelly/donelist/internal/tag"
	"github.com/dev-jelly/donelist/internal/testutil"
	"github.com/dev-jelly/donelist/internal/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestPremiumHistoricalEdit_E2E_Comprehensive provides comprehensive integration tests
// for the premium historical edit feature, covering permissions, edit history,
// and concurrent edit scenarios with optimistic locking
func TestPremiumHistoricalEdit_E2E_Comprehensive(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test environment
	testDB := testutil.SetupTestDB(t)
	defer testDB.TearDown(t)

	// Create repositories
	userRepo := user.NewRepository(testDB.DB)
	checkinRepo := checkin.NewRepository(testDB.DB)
	categoryRepo := category.NewRepository(testDB.DB)
	tagRepo := tag.NewRepository(testDB.DB)
	editHistoryRepo := checkin.NewEditHistoryRepository(testDB.DB)

	// Create service
	logger := zap.NewNop()
	checkinService := checkin.NewService(checkinRepo, categoryRepo, tagRepo, userRepo, nil, logger)

	ctx := context.Background()

	// Run subtests
	t.Run("Permission_Validation", func(t *testing.T) {
		testDB.CleanTables(t)
		testPermissionValidation(t, ctx, testDB, checkinService, userRepo, checkinRepo)
	})

	t.Run("Edit_History_Tracking", func(t *testing.T) {
		testDB.CleanTables(t)
		testEditHistoryTracking(t, ctx, testDB, checkinService, editHistoryRepo)
	})

	t.Run("Optimistic_Locking", func(t *testing.T) {
		testDB.CleanTables(t)
		testOptimisticLocking(t, ctx, testDB, checkinService, checkinRepo)
	})

	t.Run("Concurrent_Edit_Scenarios", func(t *testing.T) {
		testDB.CleanTables(t)
		testConcurrentEditScenarios(t, ctx, testDB, checkinService, checkinRepo)
	})

	t.Run("Version_Conflict_Resolution", func(t *testing.T) {
		testDB.CleanTables(t)
		testVersionConflictResolution(t, ctx, testDB, checkinService, checkinRepo)
	})

	t.Run("Audit_Log_Verification", func(t *testing.T) {
		testDB.CleanTables(t)
		testAuditLogVerification(t, ctx, testDB, checkinService, editHistoryRepo)
	})

	t.Run("E2E_User_Workflows", func(t *testing.T) {
		testDB.CleanTables(t)
		testE2EUserWorkflows(t, ctx, testDB, checkinService, editHistoryRepo)
	})
}

// testPermissionValidation tests permission checks for free vs premium users
func testPermissionValidation(t *testing.T, ctx context.Context, testDB *testutil.TestDB,
	service *checkin.Service, userRepo *user.Repository, checkinRepo *checkin.Repository) {

	fixtures := testutil.NewFixtures(testDB)

	t.Run("FreeUser_CanEdit_Within2Hours", func(t *testing.T) {
		// Create free user
		userID := fixtures.CreateTestUser(t, "free@test.com", "password", "freeuser")

		// Create recent checkin (1 hour ago)
		checkinID := uuid.New()
		recentTime := time.Now().UTC().Add(-1 * time.Hour)
		_, err := testDB.DB.Exec(`
			INSERT INTO checkins (id, user_id, content, checkin_time, duration_minutes, version, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, checkinID, userID, "Original content", recentTime, 30, 0, recentTime, recentTime)
		require.NoError(t, err)

		// Attempt to edit - should succeed
		newContent := "Updated content"
		updated, err := service.Update(ctx, checkinID, userID, checkin.UpdateInput{
			Content: &newContent,
			Version: 0,
		})

		require.NoError(t, err)
		assert.Equal(t, newContent, updated.Content)
		assert.True(t, updated.IsEdited)
		assert.Equal(t, 1, updated.EditCount)
		assert.Equal(t, 1, updated.Version) // Version incremented
	})

	t.Run("FreeUser_CannotEdit_Beyond2Hours", func(t *testing.T) {
		// Create free user
		userID := fixtures.CreateTestUser(t, "free2@test.com", "password", "freeuser2")

		// Create old checkin (3 hours ago)
		checkinID := uuid.New()
		oldTime := time.Now().UTC().Add(-3 * time.Hour)
		_, err := testDB.DB.Exec(`
			INSERT INTO checkins (id, user_id, content, checkin_time, duration_minutes, version, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, checkinID, userID, "Original content", oldTime, 30, 0, oldTime, oldTime)
		require.NoError(t, err)

		// Attempt to edit - should fail
		newContent := "Updated content"
		_, err = service.Update(ctx, checkinID, userID, checkin.UpdateInput{
			Content: &newContent,
			Version: 0,
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "edit permission denied")
	})

	t.Run("PremiumUser_CanEdit_AnyTime", func(t *testing.T) {
		// Create premium user
		userID := fixtures.CreatePremiumUser(t, "premium@test.com", "password", "premiumuser")

		// Create very old checkin (1 week ago)
		checkinID := uuid.New()
		veryOldTime := time.Now().UTC().Add(-7 * 24 * time.Hour)
		_, err := testDB.DB.Exec(`
			INSERT INTO checkins (id, user_id, content, checkin_time, duration_minutes, version, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, checkinID, userID, "Original content", veryOldTime, 30, 0, veryOldTime, veryOldTime)
		require.NoError(t, err)

		// Attempt to edit - should succeed
		newContent := "Updated content by premium user"
		updated, err := service.Update(ctx, checkinID, userID, checkin.UpdateInput{
			Content: &newContent,
			Version: 0,
		})

		require.NoError(t, err)
		assert.Equal(t, newContent, updated.Content)
		assert.True(t, updated.IsEdited)
		assert.Equal(t, 1, updated.EditCount)
	})

	t.Run("FreeUser_At2HourBoundary", func(t *testing.T) {
		// Create free user
		userID := fixtures.CreateTestUser(t, "free3@test.com", "password", "freeuser3")

		// Create checkin at exactly 2 hours ago
		checkinID := uuid.New()
		boundaryTime := time.Now().UTC().Add(-2 * time.Hour)
		_, err := testDB.DB.Exec(`
			INSERT INTO checkins (id, user_id, content, checkin_time, duration_minutes, version, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, checkinID, userID, "Original content", boundaryTime, 30, 0, boundaryTime, boundaryTime)
		require.NoError(t, err)

		// Attempt to edit - should fail (at or beyond boundary)
		newContent := "Updated content"
		_, err = service.Update(ctx, checkinID, userID, checkin.UpdateInput{
			Content: &newContent,
			Version: 0,
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "edit permission denied")
	})

	t.Run("FreeUser_JustUnder2Hours", func(t *testing.T) {
		// Create free user
		userID := fixtures.CreateTestUser(t, "free4@test.com", "password", "freeuser4")

		// Create checkin at 119 minutes ago (just under 2 hours)
		checkinID := uuid.New()
		justUnderTime := time.Now().UTC().Add(-119 * time.Minute)
		_, err := testDB.DB.Exec(`
			INSERT INTO checkins (id, user_id, content, checkin_time, duration_minutes, version, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, checkinID, userID, "Original content", justUnderTime, 30, 0, justUnderTime, justUnderTime)
		require.NoError(t, err)

		// Attempt to edit - should succeed
		newContent := "Updated content"
		updated, err := service.Update(ctx, checkinID, userID, checkin.UpdateInput{
			Content: &newContent,
			Version: 0,
		})

		require.NoError(t, err)
		assert.Equal(t, newContent, updated.Content)
	})
}

// testEditHistoryTracking verifies that edit history is properly tracked
func testEditHistoryTracking(t *testing.T, ctx context.Context, testDB *testutil.TestDB,
	service *checkin.Service, editHistoryRepo *checkin.EditHistoryRepository) {

	fixtures := testutil.NewFixtures(testDB)

	t.Run("SingleEdit_CreatesHistoryEntry", func(t *testing.T) {
		// Create premium user (to avoid time restrictions)
		userID := fixtures.CreatePremiumUser(t, "premium1@test.com", "password", "premium1")

		// Create checkin
		checkinID := uuid.New()
		originalContent := "Original content"
		checkinTime := time.Now().UTC()
		_, err := testDB.DB.Exec(`
			INSERT INTO checkins (id, user_id, content, checkin_time, duration_minutes, version, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, checkinID, userID, originalContent, checkinTime, 30, 0, checkinTime, checkinTime)
		require.NoError(t, err)

		// Edit checkin
		newContent := "Updated content"
		editReason := "Fixed typo"
		_, err = service.Update(ctx, checkinID, userID, checkin.UpdateInput{
			Content:    &newContent,
			EditReason: &editReason,
			Version:    0,
		})
		require.NoError(t, err)

		// Verify history entry was created
		history, err := editHistoryRepo.GetByCheckinID(ctx, checkinID)
		require.NoError(t, err)
		require.Len(t, history, 1)

		assert.Equal(t, checkinID, history[0].CheckinID)
		assert.Equal(t, userID, history[0].UserID)
		assert.Equal(t, originalContent, history[0].PreviousContent)
		assert.NotNil(t, history[0].EditReason)
		assert.Equal(t, editReason, *history[0].EditReason)
	})

	t.Run("MultipleEdits_CreateMultipleHistoryEntries", func(t *testing.T) {
		// Create premium user
		userID := fixtures.CreatePremiumUser(t, "premium2@test.com", "password", "premium2")

		// Create checkin
		checkinID := uuid.New()
		content1 := "Version 1"
		checkinTime := time.Now().UTC()
		_, err := testDB.DB.Exec(`
			INSERT INTO checkins (id, user_id, content, checkin_time, duration_minutes, version, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, checkinID, userID, content1, checkinTime, 30, 0, checkinTime, checkinTime)
		require.NoError(t, err)

		// First edit
		content2 := "Version 2"
		reason1 := "First edit"
		_, err = service.Update(ctx, checkinID, userID, checkin.UpdateInput{
			Content:    &content2,
			EditReason: &reason1,
			Version:    0,
		})
		require.NoError(t, err)

		// Second edit
		content3 := "Version 3"
		reason2 := "Second edit"
		_, err = service.Update(ctx, checkinID, userID, checkin.UpdateInput{
			Content:    &content3,
			EditReason: &reason2,
			Version:    1,
		})
		require.NoError(t, err)

		// Third edit
		content4 := "Version 4"
		reason3 := "Third edit"
		_, err = service.Update(ctx, checkinID, userID, checkin.UpdateInput{
			Content:    &content4,
			EditReason: &reason3,
			Version:    2,
		})
		require.NoError(t, err)

		// Verify history entries
		history, err := editHistoryRepo.GetByCheckinID(ctx, checkinID)
		require.NoError(t, err)
		require.Len(t, history, 3)

		// History should be in reverse chronological order (newest first)
		assert.Equal(t, content3, history[0].PreviousContent) // Most recent edit
		assert.Equal(t, content2, history[1].PreviousContent)
		assert.Equal(t, content1, history[2].PreviousContent) // Oldest edit
	})

	t.Run("CategoryChange_TrackedInHistory", func(t *testing.T) {
		// Create premium user
		userID := fixtures.CreatePremiumUser(t, "premium3@test.com", "password", "premium3")

		// Create categories
		cat1ID := fixtures.CreateTestCategory(t, userID, "Work", testutil.StringPtr("#FF0000"), nil)
		cat2ID := fixtures.CreateTestCategory(t, userID, "Personal", testutil.StringPtr("#00FF00"), nil)

		// Create checkin with first category
		checkinID := uuid.New()
		content := "Checkin content"
		checkinTime := time.Now().UTC()
		_, err := testDB.DB.Exec(`
			INSERT INTO checkins (id, user_id, category_id, content, checkin_time, duration_minutes, version, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, checkinID, userID, cat1ID, content, checkinTime, 30, 0, checkinTime, checkinTime)
		require.NoError(t, err)

		// Change category
		_, err = service.Update(ctx, checkinID, userID, checkin.UpdateInput{
			CategoryID: &cat2ID,
			Version:    0,
		})
		require.NoError(t, err)

		// Verify category change in history
		history, err := editHistoryRepo.GetByCheckinID(ctx, checkinID)
		require.NoError(t, err)
		require.Len(t, history, 1)

		assert.NotNil(t, history[0].PreviousCategoryID)
		assert.Equal(t, cat1ID, *history[0].PreviousCategoryID)
	})

	t.Run("EditCount_IncrementsProperly", func(t *testing.T) {
		// Create premium user
		userID := fixtures.CreatePremiumUser(t, "premium4@test.com", "password", "premium4")

		// Create checkin
		checkinID := uuid.New()
		content := "Original"
		checkinTime := time.Now().UTC()
		_, err := testDB.DB.Exec(`
			INSERT INTO checkins (id, user_id, content, checkin_time, duration_minutes, version, edit_count, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, checkinID, userID, content, checkinTime, 30, 0, 0, checkinTime, checkinTime)
		require.NoError(t, err)

		// Perform 5 edits
		for i := 1; i <= 5; i++ {
			newContent := "Edit " + string(rune(i))
			_, err = service.Update(ctx, checkinID, userID, checkin.UpdateInput{
				Content: &newContent,
				Version: i - 1,
			})
			require.NoError(t, err)
		}

		// Verify edit count
		var editCount int
		err = testDB.DB.Get(&editCount, "SELECT edit_count FROM checkins WHERE id = $1", checkinID)
		require.NoError(t, err)
		assert.Equal(t, 5, editCount)

		// Verify history count matches
		count, err := editHistoryRepo.CountByCheckinID(ctx, checkinID)
		require.NoError(t, err)
		assert.Equal(t, 5, count)
	})
}

// testOptimisticLocking verifies optimistic locking mechanism
func testOptimisticLocking(t *testing.T, ctx context.Context, testDB *testutil.TestDB,
	service *checkin.Service, checkinRepo *checkin.Repository) {

	fixtures := testutil.NewFixtures(testDB)

	t.Run("CorrectVersion_UpdateSucceeds", func(t *testing.T) {
		// Create premium user
		userID := fixtures.CreatePremiumUser(t, "lock1@test.com", "password", "lock1")

		// Create checkin
		checkinID := uuid.New()
		content := "Original"
		checkinTime := time.Now().UTC()
		_, err := testDB.DB.Exec(`
			INSERT INTO checkins (id, user_id, content, checkin_time, duration_minutes, version, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, checkinID, userID, content, checkinTime, 30, 0, checkinTime, checkinTime)
		require.NoError(t, err)

		// Update with correct version
		newContent := "Updated"
		updated, err := service.Update(ctx, checkinID, userID, checkin.UpdateInput{
			Content: &newContent,
			Version: 0, // Correct version
		})

		require.NoError(t, err)
		assert.Equal(t, 1, updated.Version) // Version incremented
		assert.Equal(t, newContent, updated.Content)
	})

	t.Run("IncorrectVersion_UpdateFails", func(t *testing.T) {
		// Create premium user
		userID := fixtures.CreatePremiumUser(t, "lock2@test.com", "password", "lock2")

		// Create checkin
		checkinID := uuid.New()
		content := "Original"
		checkinTime := time.Now().UTC()
		_, err := testDB.DB.Exec(`
			INSERT INTO checkins (id, user_id, content, checkin_time, duration_minutes, version, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, checkinID, userID, content, checkinTime, 30, 0, checkinTime, checkinTime)
		require.NoError(t, err)

		// First update (version 0 -> 1)
		content1 := "First update"
		_, err = service.Update(ctx, checkinID, userID, checkin.UpdateInput{
			Content: &content1,
			Version: 0,
		})
		require.NoError(t, err)

		// Try to update with old version (should fail)
		content2 := "Second update"
		_, err = service.Update(ctx, checkinID, userID, checkin.UpdateInput{
			Content: &content2,
			Version: 0, // Wrong version (should be 1)
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "concurrent edit detected")
	})

	t.Run("VersionIncrementsSequentially", func(t *testing.T) {
		// Create premium user
		userID := fixtures.CreatePremiumUser(t, "lock3@test.com", "password", "lock3")

		// Create checkin
		checkinID := uuid.New()
		content := "V0"
		checkinTime := time.Now().UTC()
		_, err := testDB.DB.Exec(`
			INSERT INTO checkins (id, user_id, content, checkin_time, duration_minutes, version, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, checkinID, userID, content, checkinTime, 30, 0, checkinTime, checkinTime)
		require.NoError(t, err)

		// Perform 10 sequential updates
		for i := 1; i <= 10; i++ {
			newContent := "V" + string(rune(i))
			updated, err := service.Update(ctx, checkinID, userID, checkin.UpdateInput{
				Content: &newContent,
				Version: i - 1, // Use correct previous version
			})
			require.NoError(t, err, "Update %d failed", i)
			assert.Equal(t, i, updated.Version, "Version should be %d after update %d", i, i)
		}

		// Verify final version
		var version int
		err = testDB.DB.Get(&version, "SELECT version FROM checkins WHERE id = $1", checkinID)
		require.NoError(t, err)
		assert.Equal(t, 10, version)
	})
}

// testConcurrentEditScenarios simulates concurrent edit scenarios
func testConcurrentEditScenarios(t *testing.T, ctx context.Context, testDB *testutil.TestDB,
	service *checkin.Service, checkinRepo *checkin.Repository) {

	fixtures := testutil.NewFixtures(testDB)

	t.Run("TwoUsers_SimultaneousEdit_OneSucceedsOneFails", func(t *testing.T) {
		// Create premium user
		userID := fixtures.CreatePremiumUser(t, "concurrent1@test.com", "password", "concurrent1")

		// Create checkin
		checkinID := uuid.New()
		content := "Original"
		checkinTime := time.Now().UTC()
		_, err := testDB.DB.Exec(`
			INSERT INTO checkins (id, user_id, content, checkin_time, duration_minutes, version, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, checkinID, userID, content, checkinTime, 30, 0, checkinTime, checkinTime)
		require.NoError(t, err)

		// Simulate two concurrent edit attempts with same version
		content1 := "Edit by user 1"
		content2 := "Edit by user 2"

		// First edit succeeds
		_, err1 := service.Update(ctx, checkinID, userID, checkin.UpdateInput{
			Content: &content1,
			Version: 0,
		})
		require.NoError(t, err1)

		// Second edit fails (version mismatch)
		_, err2 := service.Update(ctx, checkinID, userID, checkin.UpdateInput{
			Content: &content2,
			Version: 0, // Same version as first edit
		})
		require.Error(t, err2)
		assert.Contains(t, err2.Error(), "concurrent edit detected")

		// Verify only first edit was applied
		checkin, err := checkinRepo.GetByID(ctx, checkinID, userID)
		require.NoError(t, err)
		assert.Equal(t, content1, checkin.Content)
		assert.Equal(t, 1, checkin.Version)
		assert.Equal(t, 1, checkin.EditCount)
	})

	t.Run("RapidSuccessiveEdits_AllWithCorrectVersions", func(t *testing.T) {
		// Create premium user
		userID := fixtures.CreatePremiumUser(t, "concurrent2@test.com", "password", "concurrent2")

		// Create checkin
		checkinID := uuid.New()
		content := "V0"
		checkinTime := time.Now().UTC()
		_, err := testDB.DB.Exec(`
			INSERT INTO checkins (id, user_id, content, checkin_time, duration_minutes, version, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, checkinID, userID, content, checkinTime, 30, 0, checkinTime, checkinTime)
		require.NoError(t, err)

		// Simulate rapid successive edits
		successCount := 0
		for i := 1; i <= 20; i++ {
			newContent := "Rapid edit " + string(rune(i))
			_, err := service.Update(ctx, checkinID, userID, checkin.UpdateInput{
				Content: &newContent,
				Version: i - 1,
			})
			if err == nil {
				successCount++
			}
		}

		// All should succeed since we're using correct versions
		assert.Equal(t, 20, successCount)

		// Verify final state
		checkin, err := checkinRepo.GetByID(ctx, checkinID, userID)
		require.NoError(t, err)
		assert.Equal(t, 20, checkin.Version)
		assert.Equal(t, 20, checkin.EditCount)
	})
}

// testVersionConflictResolution tests version conflict scenarios
func testVersionConflictResolution(t *testing.T, ctx context.Context, testDB *testutil.TestDB,
	service *checkin.Service, checkinRepo *checkin.Repository) {

	fixtures := testutil.NewFixtures(testDB)

	t.Run("StaleVersion_ClientMustRefresh", func(t *testing.T) {
		// Create premium user
		userID := fixtures.CreatePremiumUser(t, "version1@test.com", "password", "version1")

		// Create checkin
		checkinID := uuid.New()
		content := "Original"
		checkinTime := time.Now().UTC()
		_, err := testDB.DB.Exec(`
			INSERT INTO checkins (id, user_id, content, checkin_time, duration_minutes, version, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, checkinID, userID, content, checkinTime, 30, 0, checkinTime, checkinTime)
		require.NoError(t, err)

		// Server performs update
		serverContent := "Server update"
		_, err = service.Update(ctx, checkinID, userID, checkin.UpdateInput{
			Content: &serverContent,
			Version: 0,
		})
		require.NoError(t, err)

		// Client tries to update with stale version
		clientContent := "Client update"
		_, err = service.Update(ctx, checkinID, userID, checkin.UpdateInput{
			Content: &clientContent,
			Version: 0, // Stale version
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "concurrent edit detected")

		// Client refreshes and retries with correct version
		refreshed, err := checkinRepo.GetByID(ctx, checkinID, userID)
		require.NoError(t, err)
		assert.Equal(t, 1, refreshed.Version)

		// Now retry with correct version
		_, err = service.Update(ctx, checkinID, userID, checkin.UpdateInput{
			Content: &clientContent,
			Version: refreshed.Version,
		})
		require.NoError(t, err)
	})

	t.Run("DeletedCheckin_UpdateFails", func(t *testing.T) {
		// Create premium user
		userID := fixtures.CreatePremiumUser(t, "version2@test.com", "password", "version2")

		// Create checkin
		checkinID := uuid.New()
		content := "Original"
		checkinTime := time.Now().UTC()
		_, err := testDB.DB.Exec(`
			INSERT INTO checkins (id, user_id, content, checkin_time, duration_minutes, version, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, checkinID, userID, content, checkinTime, 30, 0, checkinTime, checkinTime)
		require.NoError(t, err)

		// Delete checkin
		err = service.Delete(ctx, checkinID, userID)
		require.NoError(t, err)

		// Try to update deleted checkin
		newContent := "Update after delete"
		_, err = service.Update(ctx, checkinID, userID, checkin.UpdateInput{
			Content: &newContent,
			Version: 0,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

// testAuditLogVerification verifies audit logging functionality
func testAuditLogVerification(t *testing.T, ctx context.Context, testDB *testutil.TestDB,
	service *checkin.Service, editHistoryRepo *checkin.EditHistoryRepository) {

	fixtures := testutil.NewFixtures(testDB)

	t.Run("AllEdits_HaveAuditTrail", func(t *testing.T) {
		// Create premium user
		userID := fixtures.CreatePremiumUser(t, "audit1@test.com", "password", "audit1")

		// Create checkin
		checkinID := uuid.New()
		content := "V0"
		checkinTime := time.Now().UTC()
		_, err := testDB.DB.Exec(`
			INSERT INTO checkins (id, user_id, content, checkin_time, duration_minutes, version, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, checkinID, userID, content, checkinTime, 30, 0, checkinTime, checkinTime)
		require.NoError(t, err)

		// Perform 5 edits with reasons
		for i := 1; i <= 5; i++ {
			newContent := "V" + string(rune(i))
			reason := "Edit reason " + string(rune(i))
			_, err := service.Update(ctx, checkinID, userID, checkin.UpdateInput{
				Content:    &newContent,
				EditReason: &reason,
				Version:    i - 1,
			})
			require.NoError(t, err)
		}

		// Verify all edits have audit trail
		history, err := editHistoryRepo.GetByCheckinID(ctx, checkinID)
		require.NoError(t, err)
		require.Len(t, history, 5)

		// Verify all have reasons
		for _, entry := range history {
			assert.NotNil(t, entry.EditReason)
			assert.NotEmpty(t, *entry.EditReason)
		}
	})

	t.Run("EditHistory_ChronologicalOrder", func(t *testing.T) {
		// Create premium user
		userID := fixtures.CreatePremiumUser(t, "audit2@test.com", "password", "audit2")

		// Create checkin
		checkinID := uuid.New()
		content := "Original"
		checkinTime := time.Now().UTC()
		_, err := testDB.DB.Exec(`
			INSERT INTO checkins (id, user_id, content, checkin_time, duration_minutes, version, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, checkinID, userID, content, checkinTime, 30, 0, checkinTime, checkinTime)
		require.NoError(t, err)

		// Perform edits with delays
		editTimes := make([]time.Time, 0)
		for i := 1; i <= 3; i++ {
			time.Sleep(10 * time.Millisecond) // Small delay to ensure different timestamps
			newContent := "Edit " + string(rune(i))
			_, err := service.Update(ctx, checkinID, userID, checkin.UpdateInput{
				Content: &newContent,
				Version: i - 1,
			})
			require.NoError(t, err)
			editTimes = append(editTimes, time.Now())
		}

		// Verify chronological order
		history, err := editHistoryRepo.GetByCheckinID(ctx, checkinID)
		require.NoError(t, err)
		require.Len(t, history, 3)

		// History should be in reverse chronological order
		for i := 0; i < len(history)-1; i++ {
			assert.True(t, history[i].EditedAt.After(history[i+1].EditedAt),
				"History should be in reverse chronological order")
		}
	})

	t.Run("UserEditHistory_Pagination", func(t *testing.T) {
		// Create premium user
		userID := fixtures.CreatePremiumUser(t, "audit3@test.com", "password", "audit3")

		// Create multiple checkins and edit them
		for j := 0; j < 5; j++ {
			checkinID := uuid.New()
			content := "Checkin " + string(rune(j))
			checkinTime := time.Now().UTC()
			_, err := testDB.DB.Exec(`
				INSERT INTO checkins (id, user_id, content, checkin_time, duration_minutes, version, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			`, checkinID, userID, content, checkinTime, 30, 0, checkinTime, checkinTime)
			require.NoError(t, err)

			// Edit each checkin twice
			for i := 1; i <= 2; i++ {
				newContent := "Edit " + string(rune(i))
				_, err := service.Update(ctx, checkinID, userID, checkin.UpdateInput{
					Content: &newContent,
					Version: i - 1,
				})
				require.NoError(t, err)
			}
		}

		// Test pagination (10 total edits)
		page1, err := editHistoryRepo.GetByUserID(ctx, userID, 5, 0)
		require.NoError(t, err)
		assert.Len(t, page1, 5)

		page2, err := editHistoryRepo.GetByUserID(ctx, userID, 5, 5)
		require.NoError(t, err)
		assert.Len(t, page2, 5)

		// Verify no duplicates
		seen := make(map[uuid.UUID]bool)
		for _, entry := range page1 {
			seen[entry.ID] = true
		}
		for _, entry := range page2 {
			assert.False(t, seen[entry.ID], "Should not have duplicate entries")
		}
	})
}

// testE2EUserWorkflows tests complete user workflows
func testE2EUserWorkflows(t *testing.T, ctx context.Context, testDB *testutil.TestDB,
	service *checkin.Service, editHistoryRepo *checkin.EditHistoryRepository) {

	fixtures := testutil.NewFixtures(testDB)

	t.Run("FreeUser_UpgradeToPremium_CanEditOldCheckins", func(t *testing.T) {
		// Create free user
		userID := fixtures.CreateTestUser(t, "upgrade@test.com", "password", "upgradeuser")

		// Create old checkin (3 hours ago)
		checkinID := uuid.New()
		oldTime := time.Now().UTC().Add(-3 * time.Hour)
		_, err := testDB.DB.Exec(`
			INSERT INTO checkins (id, user_id, content, checkin_time, duration_minutes, version, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, checkinID, userID, "Old content", oldTime, 30, 0, oldTime, oldTime)
		require.NoError(t, err)

		// Try to edit as free user - should fail
		newContent := "Updated content"
		_, err = service.Update(ctx, checkinID, userID, checkin.UpdateInput{
			Content: &newContent,
			Version: 0,
		})
		require.Error(t, err)

		// Upgrade to premium
		_, err = testDB.DB.Exec("UPDATE users SET tier = 'premium' WHERE id = $1", userID)
		require.NoError(t, err)

		// Now should be able to edit
		_, err = service.Update(ctx, checkinID, userID, checkin.UpdateInput{
			Content: &newContent,
			Version: 0,
		})
		require.NoError(t, err)
	})

	t.Run("PremiumUser_MultipleEdits_FullAuditTrail", func(t *testing.T) {
		// Create premium user
		userID := fixtures.CreatePremiumUser(t, "premium@test.com", "password", "premiumuser")

		// Create checkin
		checkinID := uuid.New()
		originalContent := "Meeting with team"
		checkinTime := time.Now().UTC().Add(-5 * time.Hour)
		_, err := testDB.DB.Exec(`
			INSERT INTO checkins (id, user_id, content, checkin_time, duration_minutes, version, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, checkinID, userID, originalContent, checkinTime, 30, 0, checkinTime, checkinTime)
		require.NoError(t, err)

		// User realizes they made a typo
		typoFix := "Meeting with team about Q1 goals"
		reason1 := "Added missing detail"
		_, err = service.Update(ctx, checkinID, userID, checkin.UpdateInput{
			Content:    &typoFix,
			EditReason: &reason1,
			Version:    0,
		})
		require.NoError(t, err)

		// User adds more context
		moreContext := "Meeting with team about Q1 goals - decided on 3 key initiatives"
		reason2 := "Added outcome"
		_, err = service.Update(ctx, checkinID, userID, checkin.UpdateInput{
			Content:    &moreContext,
			EditReason: &reason2,
			Version:    1,
		})
		require.NoError(t, err)

		// User corrects a mistake
		correction := "Meeting with team about Q2 goals - decided on 3 key initiatives"
		reason3 := "Fixed quarter (Q2 not Q1)"
		_, err = service.Update(ctx, checkinID, userID, checkin.UpdateInput{
			Content:    &correction,
			EditReason: &reason3,
			Version:    2,
		})
		require.NoError(t, err)

		// Verify complete audit trail
		history, err := editHistoryRepo.GetByCheckinID(ctx, checkinID)
		require.NoError(t, err)
		require.Len(t, history, 3)

		// Verify progression
		assert.Equal(t, moreContext, history[0].PreviousContent)
		assert.Equal(t, typoFix, history[1].PreviousContent)
		assert.Equal(t, originalContent, history[2].PreviousContent)

		// Verify all reasons are present
		assert.Equal(t, reason3, *history[0].EditReason)
		assert.Equal(t, reason2, *history[1].EditReason)
		assert.Equal(t, reason1, *history[2].EditReason)
	})

	t.Run("CollaborativeEditing_ConflictDetection", func(t *testing.T) {
		// Simulate scenario where user edits from two different devices
		userID := fixtures.CreatePremiumUser(t, "multidevice@test.com", "password", "multidevice")

		// Create checkin
		checkinID := uuid.New()
		content := "Original from phone"
		checkinTime := time.Now().UTC()
		_, err := testDB.DB.Exec(`
			INSERT INTO checkins (id, user_id, content, checkin_time, duration_minutes, version, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, checkinID, userID, content, checkinTime, 30, 0, checkinTime, checkinTime)
		require.NoError(t, err)

		// Device 1 (phone) makes an edit
		phoneEdit := "Updated from phone"
		_, err = service.Update(ctx, checkinID, userID, checkin.UpdateInput{
			Content: &phoneEdit,
			Version: 0,
		})
		require.NoError(t, err)

		// Device 2 (laptop) tries to edit with stale version
		laptopEdit := "Updated from laptop"
		_, err = service.Update(ctx, checkinID, userID, checkin.UpdateInput{
			Content: &laptopEdit,
			Version: 0, // Stale version
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "concurrent edit detected")

		// Device 2 refreshes and sees phone's edit
		// Then makes its edit
		_, err = service.Update(ctx, checkinID, userID, checkin.UpdateInput{
			Content: &laptopEdit,
			Version: 1, // Correct version after refresh
		})
		require.NoError(t, err)

		// Verify final state
		history, err := editHistoryRepo.GetByCheckinID(ctx, checkinID)
		require.NoError(t, err)
		require.Len(t, history, 2)

		// Verify both edits are in history
		assert.Equal(t, phoneEdit, history[0].PreviousContent)
		assert.Equal(t, content, history[1].PreviousContent)
	})
}
