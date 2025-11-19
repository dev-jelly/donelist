package checkin

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/dev-jelly/donelist/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "github.com/lib/pq" // PostgreSQL driver
)

func TestCheckinRepository_Create(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.TearDown(t)

	repo := NewRepository(testDB.DB)
	ctx := context.Background()
	fixtures := testutil.NewFixtures(testDB)

	// Create test user and category
	testUser := fixtures.CreateTestUser(t, "checkin@example.com", "password123")
	testCategory := fixtures.CreateTestCategory(t, testUser.ID, "Work")

	t.Run("Create checkin successfully", func(t *testing.T) {
		input := CreateCheckinInput{
			UserID:          testUser.ID,
			CategoryID:      &testCategory.ID,
			Content:         "Completed task",
			CheckinTime:     time.Now(),
			DurationMinutes: 30,
		}

		checkin, err := repo.Create(ctx, input)
		require.NoError(t, err)
		assert.NotNil(t, checkin)
		assert.NotEqual(t, uuid.Nil, checkin.ID)
		assert.Equal(t, input.UserID, checkin.UserID)
		assert.Equal(t, input.CategoryID, checkin.CategoryID)
		assert.Equal(t, input.Content, checkin.Content)
		assert.Equal(t, input.DurationMinutes, checkin.DurationMinutes)
	})

	t.Run("Create checkin with tags", func(t *testing.T) {
		tag1 := fixtures.CreateTestTag(t, testUser.ID, "urgent")
		tag2 := fixtures.CreateTestTag(t, testUser.ID, "backend")

		input := CreateCheckinInput{
			UserID:          testUser.ID,
			Content:         "Fixed bug",
			CheckinTime:     time.Now(),
			DurationMinutes: 45,
			TagIDs:          []uuid.UUID{tag1.ID, tag2.ID},
		}

		checkin, err := repo.Create(ctx, input)
		require.NoError(t, err)
		assert.NotNil(t, checkin)
		assert.Len(t, checkin.Tags, 2)
		assert.Equal(t, tag1.Name, checkin.Tags[0].Name)
	})
}

func TestCheckinRepository_GetByID(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.TearDown(t)

	repo := NewRepository(testDB.DB)
	ctx := context.Background()
	fixtures := testutil.NewFixtures(testDB)

	// Create test data
	testUser := fixtures.CreateTestUser(t, "getbyid@example.com", "password123")
	testCheckin := fixtures.CreateTestCheckin(t, testUser.ID, nil, "Test checkin", 30)

	t.Run("Get existing checkin", func(t *testing.T) {
		checkin, err := repo.GetByID(ctx, testCheckin.ID, testUser.ID)
		require.NoError(t, err)
		assert.NotNil(t, checkin)
		assert.Equal(t, testCheckin.ID, checkin.ID)
		assert.Equal(t, testCheckin.Content, checkin.Content)
	})

	t.Run("Return error for non-existent checkin", func(t *testing.T) {
		checkin, err := repo.GetByID(ctx, uuid.New(), testUser.ID)
		assert.Error(t, err)
		assert.Nil(t, checkin)
	})

	t.Run("Return error for wrong user", func(t *testing.T) {
		otherUser := fixtures.CreateTestUser(t, "other@example.com", "password123")
		checkin, err := repo.GetByID(ctx, testCheckin.ID, otherUser.ID)
		assert.Error(t, err)
		assert.Nil(t, checkin)
	})
}

func TestCheckinRepository_List(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.TearDown(t)

	repo := NewRepository(testDB.DB)
	ctx := context.Background()
	fixtures := testutil.NewFixtures(testDB)

	// Create test data
	testUser := fixtures.CreateTestUser(t, "list@example.com", "password123")
	testCategory := fixtures.CreateTestCategory(t, testUser.ID, "Work")

	// Create multiple checkins
	for i := 0; i < 5; i++ {
		if i < 3 {
			fixtures.CreateTestCheckin(t, testUser.ID, &testCategory.ID, "Checkin with category", 30)
		} else {
			fixtures.CreateTestCheckin(t, testUser.ID, nil, "Checkin without category", 45)
		}
	}

	t.Run("List all checkins for user", func(t *testing.T) {
		checkins, total, err := repo.List(ctx, ListOptions{
			UserID: testUser.ID,
			Limit:  10,
			Offset: 0,
		})
		require.NoError(t, err)
		assert.Len(t, checkins, 5)
		assert.Equal(t, 5, total)
	})

	t.Run("List with pagination", func(t *testing.T) {
		checkins, total, err := repo.List(ctx, ListOptions{
			UserID: testUser.ID,
			Limit:  2,
			Offset: 0,
		})
		require.NoError(t, err)
		assert.Len(t, checkins, 2)
		assert.Equal(t, 5, total)
	})

	t.Run("Filter by category", func(t *testing.T) {
		checkins, total, err := repo.List(ctx, ListOptions{
			UserID:     testUser.ID,
			CategoryID: &testCategory.ID,
			Limit:      10,
			Offset:     0,
		})
		require.NoError(t, err)
		assert.Len(t, checkins, 3)
		assert.Equal(t, 3, total)
		for _, c := range checkins {
			assert.Equal(t, &testCategory.ID, c.CategoryID)
		}
	})

	t.Run("Filter by date range", func(t *testing.T) {
		startDate := time.Now().Add(-1 * time.Hour)
		endDate := time.Now().Add(1 * time.Hour)

		checkins, total, err := repo.List(ctx, ListOptions{
			UserID:    testUser.ID,
			StartDate: &startDate,
			EndDate:   &endDate,
			Limit:     10,
			Offset:    0,
		})
		require.NoError(t, err)
		assert.Equal(t, 5, total)
		assert.Len(t, checkins, 5)
	})
}

func TestCheckinRepository_Update(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.TearDown(t)

	repo := NewRepository(testDB.DB)
	ctx := context.Background()
	fixtures := testutil.NewFixtures(testDB)

	// Create test data
	testUser := fixtures.CreateTestUser(t, "update@example.com", "password123")
	testCategory := fixtures.CreateTestCategory(t, testUser.ID, "Work")
	testCheckin := fixtures.CreateTestCheckin(t, testUser.ID, nil, "Original content", 30)

	t.Run("Update content", func(t *testing.T) {
		newContent := "Updated content"
		updated, err := repo.Update(ctx, testCheckin.ID, testUser.ID, UpdateCheckinInput{
			Content: &newContent,
		})
		require.NoError(t, err)
		assert.Equal(t, newContent, updated.Content)
		assert.True(t, updated.UpdatedAt.After(testCheckin.UpdatedAt))
	})

	t.Run("Update category", func(t *testing.T) {
		updated, err := repo.Update(ctx, testCheckin.ID, testUser.ID, UpdateCheckinInput{
			CategoryID: &testCategory.ID,
		})
		require.NoError(t, err)
		assert.Equal(t, &testCategory.ID, updated.CategoryID)
	})

	t.Run("Add edit history", func(t *testing.T) {
		// First update
		content1 := "First edit"
		_, err := repo.Update(ctx, testCheckin.ID, testUser.ID, UpdateCheckinInput{
			Content: &content1,
		})
		require.NoError(t, err)

		// Second update
		content2 := "Second edit"
		_, err = repo.Update(ctx, testCheckin.ID, testUser.ID, UpdateCheckinInput{
			Content: &content2,
		})
		require.NoError(t, err)

		// Check edit history
		history, err := repo.GetEditHistory(ctx, testCheckin.ID, testUser.ID)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(history), 2)
	})
}

func TestCheckinRepository_Delete(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.TearDown(t)

	repo := NewRepository(testDB.DB)
	ctx := context.Background()
	fixtures := testutil.NewFixtures(testDB)

	// Create test data
	testUser := fixtures.CreateTestUser(t, "delete@example.com", "password123")
	testCheckin := fixtures.CreateTestCheckin(t, testUser.ID, nil, "To be deleted", 30)

	t.Run("Delete existing checkin", func(t *testing.T) {
		err := repo.Delete(ctx, testCheckin.ID, testUser.ID)
		require.NoError(t, err)

		// Verify deletion
		checkin, err := repo.GetByID(ctx, testCheckin.ID, testUser.ID)
		assert.Error(t, err)
		assert.Nil(t, checkin)
	})

	t.Run("Handle deletion of non-existent checkin", func(t *testing.T) {
		err := repo.Delete(ctx, uuid.New(), testUser.ID)
		assert.NoError(t, err) // Should be idempotent
	})
}

func TestCheckinRepository_GetEditHistory(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.TearDown(t)

	repo := NewRepository(testDB.DB)
	ctx := context.Background()
	fixtures := testutil.NewFixtures(testDB)

	// Create test data
	testUser := fixtures.CreateTestUser(t, "history@example.com", "password123")
	testCheckin := fixtures.CreateTestCheckin(t, testUser.ID, nil, "Original", 30)

	// Make several edits
	contents := []string{"Edit 1", "Edit 2", "Edit 3"}
	for _, content := range contents {
		time.Sleep(10 * time.Millisecond) // Ensure different timestamps
		c := content
		_, err := repo.Update(ctx, testCheckin.ID, testUser.ID, UpdateCheckinInput{
			Content: &c,
		})
		require.NoError(t, err)
	}

	t.Run("Get edit history", func(t *testing.T) {
		history, err := repo.GetEditHistory(ctx, testCheckin.ID, testUser.ID)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(history), 3)

		// Check history is ordered by time (newest first)
		for i := 0; i < len(history)-1; i++ {
			assert.True(t, history[i].EditedAt.After(history[i+1].EditedAt))
		}
	})

	t.Run("Return error for wrong user", func(t *testing.T) {
		otherUser := fixtures.CreateTestUser(t, "other2@example.com", "password123")
		history, err := repo.GetEditHistory(ctx, testCheckin.ID, otherUser.ID)
		assert.Error(t, err)
		assert.Nil(t, history)
	})
}