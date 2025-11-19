package user

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/dev-jelly/donelist/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	_ "github.com/lib/pq" // PostgreSQL driver
)

func TestUserRepository_Create(t *testing.T) {
	// Setup test database
	testDB := testutil.SetupTestDB(t)
	defer testDB.TearDown(t)

	repo := NewRepository(testDB.DB)
	ctx := context.Background()

	// Test case: Create new user
	t.Run("Create new user successfully", func(t *testing.T) {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
		require.NoError(t, err)

		displayName := "Test User"
		input := CreateUserInput{
			Email:        "test@example.com",
			DisplayName:  &displayName,
			PasswordHash: string(hashedPassword),
		}

		user, err := repo.Create(ctx, input)
		require.NoError(t, err)
		assert.NotNil(t, user)
		assert.NotEqual(t, uuid.Nil, user.ID)
		assert.Equal(t, input.Email, user.Email)
		assert.Equal(t, input.DisplayName, user.DisplayName)
		assert.Equal(t, "free", user.Tier)
		assert.False(t, user.CreatedAt.IsZero())
		assert.False(t, user.UpdatedAt.IsZero())
	})

	// Test case: Duplicate email
	t.Run("Fail with duplicate email", func(t *testing.T) {
		testDB.CleanTables(t)
		hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)

		displayName1 := "User One"
		input := CreateUserInput{
			Email:        "duplicate@example.com",
			DisplayName:  &displayName1,
			PasswordHash: string(hashedPassword),
		}

		// Create first user
		_, err := repo.Create(ctx, input)
		require.NoError(t, err)

		// Try to create second user with same email
		displayName2 := "User Two"
		input.DisplayName = &displayName2
		_, err = repo.Create(ctx, input)
		assert.Error(t, err)
	})
}

func TestUserRepository_GetByID(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.TearDown(t)

	repo := NewRepository(testDB.DB)
	ctx := context.Background()
	fixtures := testutil.NewFixtures(testDB)

	// Create test user
	testUserID := fixtures.CreateTestUser(t, "getbyid@example.com", "password123", "testuser")

	// Test case: Get existing user
	t.Run("Get existing user successfully", func(t *testing.T) {
		user, err := repo.GetByID(ctx, testUserID)
		require.NoError(t, err)
		assert.NotNil(t, user)
		assert.Equal(t, testUserID, user.ID)
		assert.Equal(t, "getbyid@example.com", user.Email)
	})

	// Test case: User not found
	t.Run("Return error for non-existent user", func(t *testing.T) {
		nonExistentID := uuid.New()
		user, err := repo.GetByID(ctx, nonExistentID)
		assert.Error(t, err)
		assert.Nil(t, user)
	})
}

func TestUserRepository_GetByEmail(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.TearDown(t)

	repo := NewRepository(testDB.DB)
	ctx := context.Background()
	fixtures := testutil.NewFixtures(testDB)

	// Create test user
	testUserID := fixtures.CreateTestUser(t, "getbyemail@example.com", "password123", "testuser")

	// Test case: Get existing user by email
	t.Run("Get existing user by email successfully", func(t *testing.T) {
		user, err := repo.GetByEmail(ctx, "getbyemail@example.com")
		require.NoError(t, err)
		assert.NotNil(t, user)
		assert.Equal(t, testUserID, user.ID)
		assert.Equal(t, "getbyemail@example.com", user.Email)
	})

	// Test case: User not found by email
	t.Run("Return error for non-existent email", func(t *testing.T) {
		user, err := repo.GetByEmail(ctx, "nonexistent@example.com")
		assert.Error(t, err)
		assert.Nil(t, user)
	})
}

func TestUserRepository_Update(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.TearDown(t)

	repo := NewRepository(testDB.DB)
	ctx := context.Background()
	fixtures := testutil.NewFixtures(testDB)

	// Create test user
	testUserID := fixtures.CreateTestUser(t, "update@example.com", "password123", "testuser")

	// Get initial user state
	initialUser, err := repo.GetByID(ctx, testUserID)
	require.NoError(t, err)

	// Test case: Update display name
	t.Run("Update display name successfully", func(t *testing.T) {
		newDisplayName := "Updated User"
		updatedUser, err := repo.Update(ctx, testUserID, UpdateUserInput{
			DisplayName: &newDisplayName,
		})
		require.NoError(t, err)
		assert.NotNil(t, updatedUser)
		assert.Equal(t, newDisplayName, *updatedUser.DisplayName)
		assert.Equal(t, "update@example.com", updatedUser.Email) // Email unchanged
		assert.True(t, updatedUser.UpdatedAt.After(initialUser.UpdatedAt))
	})

	// Test case: Update tier
	t.Run("Update tier successfully", func(t *testing.T) {
		expiresAt := time.Now().Add(30 * 24 * time.Hour)
		err := repo.UpdateTier(ctx, testUserID, "premium", &expiresAt)
		require.NoError(t, err)

		updatedUser, err := repo.GetByID(ctx, testUserID)
		require.NoError(t, err)
		assert.Equal(t, "premium", updatedUser.Tier)
		assert.NotNil(t, updatedUser.TierExpiresAt)
	})

	// Test case: No fields to update
	t.Run("Handle empty update", func(t *testing.T) {
		updatedUser, err := repo.Update(ctx, testUserID, UpdateUserInput{})
		require.NoError(t, err)
		assert.NotNil(t, updatedUser)
	})
}

func TestUserRepository_Delete(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.TearDown(t)

	repo := NewRepository(testDB.DB)
	ctx := context.Background()
	fixtures := testutil.NewFixtures(testDB)

	// Test case: Delete existing user
	t.Run("Delete existing user successfully", func(t *testing.T) {
		testUserID := fixtures.CreateTestUser(t, "delete@example.com", "password123", "deleteuser")

		err := repo.Delete(ctx, testUserID)
		require.NoError(t, err)

		// Verify user is deleted
		user, err := repo.GetByID(ctx, testUserID)
		assert.Error(t, err)
		assert.Nil(t, user)
	})

	// Test case: Delete non-existent user
	t.Run("Handle deletion of non-existent user", func(t *testing.T) {
		nonExistentID := uuid.New()
		err := repo.Delete(ctx, nonExistentID)
		// Should return error for non-existent user
		assert.Error(t, err)
	})
}

func TestUserRepository_UpdateModePreference(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.TearDown(t)

	repo := NewRepository(testDB.DB)
	ctx := context.Background()
	fixtures := testutil.NewFixtures(testDB)

	// Create test user
	testUserID := fixtures.CreateTestUser(t, "modepref@example.com", "password123", "prefuser")

	// Test case: Update mode preference
	t.Run("Update mode preference successfully", func(t *testing.T) {
		err := repo.UpdateModePreference(ctx, testUserID, "dark")
		require.NoError(t, err)

		// Verify mode preference was updated
		updatedUser, err := repo.GetByID(ctx, testUserID)
		require.NoError(t, err)
		assert.Equal(t, "dark", updatedUser.ModePreference)
	})

	// Test case: Invalid user ID
	t.Run("Fail with invalid user ID", func(t *testing.T) {
		nonExistentID := uuid.New()
		err := repo.UpdateModePreference(ctx, nonExistentID, "light")
		assert.Error(t, err)
	})
}

func TestUserRepository_EmailExists(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.TearDown(t)

	repo := NewRepository(testDB.DB)
	ctx := context.Background()
	fixtures := testutil.NewFixtures(testDB)

	// Create test user
	fixtures.CreateTestUser(t, "exists@example.com", "password123", "existsuser")

	// Test case: Email exists
	t.Run("Return true for existing email", func(t *testing.T) {
		exists, err := repo.EmailExists(ctx, "exists@example.com")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	// Test case: Email does not exist
	t.Run("Return false for non-existent email", func(t *testing.T) {
		exists, err := repo.EmailExists(ctx, "nonexistent@example.com")
		require.NoError(t, err)
		assert.False(t, exists)
	})
}