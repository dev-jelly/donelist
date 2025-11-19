package category

import (
	"context"
	"testing"

	"github.com/dev-jelly/donelist/internal/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepository_Create(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.TearDown(t)

	repo := NewRepository(testDB.DB)
	userID := testutil.CreateTestUser(t, testDB.DB, "test@example.com", "TestUser")

	t.Run("successful creation", func(t *testing.T) {
		color := "#FF5733"
		icon := "work"
		input := CreateCategoryInput{
			UserID: userID,
			Name:   "Work",
			Color:  &color,
			Icon:   &icon,
		}

		result, err := repo.Create(context.Background(), input)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.NotEqual(t, uuid.Nil, result.ID)
		assert.Equal(t, userID, result.UserID)
		assert.Equal(t, "Work", result.Name)
		assert.NotNil(t, result.Color)
		assert.Equal(t, color, *result.Color)
		assert.NotNil(t, result.Icon)
		assert.Equal(t, icon, *result.Icon)
		assert.False(t, result.CreatedAt.IsZero())
		assert.False(t, result.UpdatedAt.IsZero())
	})

	t.Run("creation without optional fields", func(t *testing.T) {
		input := CreateCategoryInput{
			UserID: userID,
			Name:   "Personal",
		}

		result, err := repo.Create(context.Background(), input)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "Personal", result.Name)
		assert.Nil(t, result.Color)
		assert.Nil(t, result.Icon)
	})

	t.Run("duplicate name for same user fails", func(t *testing.T) {
		input := CreateCategoryInput{
			UserID: userID,
			Name:   "Work", // Already exists from first test
		}

		result, err := repo.Create(context.Background(), input)

		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("same name for different users succeeds", func(t *testing.T) {
		otherUserID := testutil.CreateTestUser(t, testDB.DB, "other@example.com", "OtherUser")

		input := CreateCategoryInput{
			UserID: otherUserID,
			Name:   "Work", // Same name as first user
		}

		result, err := repo.Create(context.Background(), input)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "Work", result.Name)
		assert.Equal(t, otherUserID, result.UserID)
	})
}

func TestRepository_GetByID(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.TearDown(t)

	repo := NewRepository(testDB.DB)
	userID := testutil.CreateTestUser(t, testDB.DB, "test@example.com", "TestUser")

	// Create a category to retrieve
	color := "#FF5733"
	created, err := repo.Create(context.Background(), CreateCategoryInput{
		UserID: userID,
		Name:   "Work",
		Color:  &color,
	})
	require.NoError(t, err)

	t.Run("successful retrieval", func(t *testing.T) {
		result, err := repo.GetByID(context.Background(), created.ID, userID)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, created.ID, result.ID)
		assert.Equal(t, created.Name, result.Name)
		assert.Equal(t, created.Color, result.Color)
	})

	t.Run("non-existent category", func(t *testing.T) {
		nonExistentID := uuid.New()
		result, err := repo.GetByID(context.Background(), nonExistentID, userID)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("wrong user cannot access category", func(t *testing.T) {
		otherUserID := testutil.CreateTestUser(t, testDB.DB, "other@example.com", "OtherUser")

		result, err := repo.GetByID(context.Background(), created.ID, otherUserID)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestRepository_List(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.TearDown(t)

	repo := NewRepository(testDB.DB)
	userID := testutil.CreateTestUser(t, testDB.DB, "test@example.com", "TestUser")
	otherUserID := testutil.CreateTestUser(t, testDB.DB, "other@example.com", "OtherUser")

	// Create categories for first user
	_, err := repo.Create(context.Background(), CreateCategoryInput{
		UserID: userID,
		Name:   "Work",
	})
	require.NoError(t, err)

	_, err = repo.Create(context.Background(), CreateCategoryInput{
		UserID: userID,
		Name:   "Personal",
	})
	require.NoError(t, err)

	// Create category for other user
	_, err = repo.Create(context.Background(), CreateCategoryInput{
		UserID: otherUserID,
		Name:   "Other Category",
	})
	require.NoError(t, err)

	t.Run("list user categories", func(t *testing.T) {
		result, err := repo.List(context.Background(), userID)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result, 2)

		// Check that categories are sorted by name
		names := []string{result[0].Name, result[1].Name}
		assert.Contains(t, names, "Work")
		assert.Contains(t, names, "Personal")

		// Verify all categories belong to the user
		for _, cat := range result {
			assert.Equal(t, userID, cat.UserID)
		}
	})

	t.Run("list other user categories", func(t *testing.T) {
		result, err := repo.List(context.Background(), otherUserID)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result, 1)
		assert.Equal(t, "Other Category", result[0].Name)
	})

	t.Run("list for user with no categories", func(t *testing.T) {
		newUserID := testutil.CreateTestUser(t, testDB.DB, "new@example.com", "NewUser")

		result, err := repo.List(context.Background(), newUserID)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result, 0)
	})
}

func TestRepository_Update(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.TearDown(t)

	repo := NewRepository(testDB.DB)
	userID := testutil.CreateTestUser(t, testDB.DB, "test@example.com", "TestUser")

	// Create a category to update
	color := "#FF5733"
	icon := "work"
	created, err := repo.Create(context.Background(), CreateCategoryInput{
		UserID: userID,
		Name:   "Work",
		Color:  &color,
		Icon:   &icon,
	})
	require.NoError(t, err)

	t.Run("update all fields", func(t *testing.T) {
		newName := "Updated Work"
		newColor := "#00FF00"
		newIcon := "business"
		input := UpdateCategoryInput{
			Name:  &newName,
			Color: &newColor,
			Icon:  &newIcon,
		}

		result, err := repo.Update(context.Background(), created.ID, userID, input)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, newName, result.Name)
		assert.Equal(t, newColor, *result.Color)
		assert.Equal(t, newIcon, *result.Icon)
		assert.True(t, result.UpdatedAt.After(result.CreatedAt))
	})

	t.Run("update only name", func(t *testing.T) {
		newName := "Work - Office"
		input := UpdateCategoryInput{
			Name: &newName,
		}

		result, err := repo.Update(context.Background(), created.ID, userID, input)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, newName, result.Name)
		// Other fields should remain unchanged
		assert.NotNil(t, result.Color)
		assert.NotNil(t, result.Icon)
	})

	t.Run("update non-existent category", func(t *testing.T) {
		nonExistentID := uuid.New()
		newName := "Updated"
		input := UpdateCategoryInput{
			Name: &newName,
		}

		result, err := repo.Update(context.Background(), nonExistentID, userID, input)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("wrong user cannot update category", func(t *testing.T) {
		otherUserID := testutil.CreateTestUser(t, testDB.DB, "other@example.com", "OtherUser")

		newName := "Hacked"
		input := UpdateCategoryInput{
			Name: &newName,
		}

		result, err := repo.Update(context.Background(), created.ID, otherUserID, input)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "not found")

		// Verify original category wasn't changed
		original, err := repo.GetByID(context.Background(), created.ID, userID)
		require.NoError(t, err)
		assert.NotEqual(t, "Hacked", original.Name)
	})
}

func TestRepository_Delete(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.TearDown(t)

	repo := NewRepository(testDB.DB)
	userID := testutil.CreateTestUser(t, testDB.DB, "test@example.com", "TestUser")

	t.Run("successful deletion", func(t *testing.T) {
		// Create a category to delete
		created, err := repo.Create(context.Background(), CreateCategoryInput{
			UserID: userID,
			Name:   "ToDelete",
		})
		require.NoError(t, err)

		err = repo.Delete(context.Background(), created.ID, userID)

		require.NoError(t, err)

		// Verify it's actually deleted
		result, err := repo.GetByID(context.Background(), created.ID, userID)
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("delete non-existent category", func(t *testing.T) {
		nonExistentID := uuid.New()

		err := repo.Delete(context.Background(), nonExistentID, userID)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("wrong user cannot delete category", func(t *testing.T) {
		// Create a category for first user
		created, err := repo.Create(context.Background(), CreateCategoryInput{
			UserID: userID,
			Name:   "Protected",
		})
		require.NoError(t, err)

		// Try to delete as other user
		otherUserID := testutil.CreateTestUser(t, testDB.DB, "other@example.com", "OtherUser")

		err = repo.Delete(context.Background(), created.ID, otherUserID)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")

		// Verify category still exists
		result, err := repo.GetByID(context.Background(), created.ID, userID)
		require.NoError(t, err)
		assert.Equal(t, "Protected", result.Name)
	})
}

func TestRepository_NameExists(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.TearDown(t)

	repo := NewRepository(testDB.DB)
	userID := testutil.CreateTestUser(t, testDB.DB, "test@example.com", "TestUser")
	otherUserID := testutil.CreateTestUser(t, testDB.DB, "other@example.com", "OtherUser")

	// Create a category
	_, err := repo.Create(context.Background(), CreateCategoryInput{
		UserID: userID,
		Name:   "Existing",
	})
	require.NoError(t, err)

	t.Run("existing name returns true", func(t *testing.T) {
		exists, err := repo.NameExists(context.Background(), userID, "Existing")

		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("non-existing name returns false", func(t *testing.T) {
		exists, err := repo.NameExists(context.Background(), userID, "NonExisting")

		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("same name for different user returns false", func(t *testing.T) {
		exists, err := repo.NameExists(context.Background(), otherUserID, "Existing")

		require.NoError(t, err)
		assert.False(t, exists)
	})
}
