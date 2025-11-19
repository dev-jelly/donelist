package search_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/dev-jelly/donelist/internal/search"
	"github.com/dev-jelly/donelist/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearch(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	repo := search.NewRepository(db)
	ctx := context.Background()

	// Create test user
	userID := uuid.New()
	err := testutil.CreateTestUser(db, userID, "test@example.com")
	require.NoError(t, err)

	// Create test category
	categoryID := uuid.New()
	err = testutil.CreateTestCategory(db, categoryID, userID, "Work")
	require.NoError(t, err)

	// Create test checkin with content
	checkinID := uuid.New()
	err = testutil.CreateTestCheckin(db, checkinID, userID, &categoryID, "Working on database optimization and full-text search implementation")
	require.NoError(t, err)

	t.Run("FullTextSearch", func(t *testing.T) {
		filters := search.SearchFilters{
			Query:  "database optimization",
			Limit:  20,
			Offset: 0,
		}

		results, total, err := repo.Search(ctx, userID, filters)
		require.NoError(t, err)
		assert.Greater(t, total, int64(0))
		assert.Greater(t, len(results), 0)

		// Check that result has rank and snippet
		if len(results) > 0 {
			assert.NotNil(t, results[0].Rank)
			assert.NotNil(t, results[0].Snippet)
		}
	})

	t.Run("CategoryFilter", func(t *testing.T) {
		filters := search.SearchFilters{
			CategoryIDs: []uuid.UUID{categoryID},
			Limit:       20,
			Offset:      0,
		}

		results, total, err := repo.Search(ctx, userID, filters)
		require.NoError(t, err)
		assert.Greater(t, total, int64(0))
		assert.Equal(t, categoryID, *results[0].CategoryID)
	})

	t.Run("DateRangeFilter", func(t *testing.T) {
		now := time.Now()
		yesterday := now.Add(-24 * time.Hour)
		tomorrow := now.Add(24 * time.Hour)

		filters := search.SearchFilters{
			StartDate: &yesterday,
			EndDate:   &tomorrow,
			Limit:     20,
			Offset:    0,
		}

		results, total, err := repo.Search(ctx, userID, filters)
		require.NoError(t, err)
		assert.Greater(t, total, int64(0))
	})

	t.Run("Pagination", func(t *testing.T) {
		// Create multiple checkins
		for i := 0; i < 5; i++ {
			checkinID := uuid.New()
			err = testutil.CreateTestCheckin(db, checkinID, userID, &categoryID, "Test checkin content")
			require.NoError(t, err)
		}

		filters := search.SearchFilters{
			Limit:  2,
			Offset: 0,
		}

		results, total, err := repo.Search(ctx, userID, filters)
		require.NoError(t, err)
		assert.Greater(t, total, int64(2))
		assert.Len(t, results, 2)

		// Test next page
		filters.Offset = 2
		results2, total2, err := repo.Search(ctx, userID, filters)
		require.NoError(t, err)
		assert.Equal(t, total, total2)
		assert.Greater(t, len(results2), 0)
	})
}

func TestGetFacets(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	repo := search.NewRepository(db)
	ctx := context.Background()

	// Create test data
	userID := uuid.New()
	err := testutil.CreateTestUser(db, userID, "test@example.com")
	require.NoError(t, err)

	categoryID := uuid.New()
	err = testutil.CreateTestCategory(db, categoryID, userID, "Work")
	require.NoError(t, err)

	checkinID := uuid.New()
	err = testutil.CreateTestCheckin(db, checkinID, userID, &categoryID, "Test content")
	require.NoError(t, err)

	t.Run("GetFacets", func(t *testing.T) {
		filters := search.SearchFilters{}
		facets, err := repo.GetFacets(ctx, userID, filters)
		require.NoError(t, err)
		assert.NotNil(t, facets)
		assert.NotNil(t, facets.Categories)
		assert.NotNil(t, facets.Tags)
		assert.NotNil(t, facets.Durations)
	})
}

func TestSearchHistory(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	repo := search.NewRepository(db)
	ctx := context.Background()

	userID := uuid.New()
	err := testutil.CreateTestUser(db, userID, "test@example.com")
	require.NoError(t, err)

	t.Run("SaveAndRetrieveHistory", func(t *testing.T) {
		query := "test search query"
		filters := map[string]interface{}{
			"category_ids": []string{"test-category"},
		}

		// Save search history
		err := repo.SaveSearchHistory(ctx, userID, query, filters, 5)
		require.NoError(t, err)

		// Retrieve history
		history, err := repo.GetSearchHistory(ctx, userID, 10)
		require.NoError(t, err)
		assert.Greater(t, len(history), 0)
		assert.Equal(t, query, history[0].Query)
		assert.Equal(t, 5, history[0].ResultCount)
	})
}

func TestSavedSearches(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	repo := search.NewRepository(db)
	ctx := context.Background()

	userID := uuid.New()
	err := testutil.CreateTestUser(db, userID, "test@example.com")
	require.NoError(t, err)

	t.Run("CreateSavedSearch", func(t *testing.T) {
		input := search.CreateSavedSearchInput{
			UserID:     userID,
			Name:       "My Work Search",
			Query:      stringPtr("work tasks"),
			Filters:    map[string]interface{}{"category_ids": []string{"work"}},
			IsFavorite: true,
		}

		savedSearch, err := repo.CreateSavedSearch(ctx, input)
		require.NoError(t, err)
		assert.NotNil(t, savedSearch)
		assert.Equal(t, input.Name, savedSearch.Name)
		assert.True(t, savedSearch.IsFavorite)
	})

	t.Run("ListSavedSearches", func(t *testing.T) {
		searches, err := repo.ListSavedSearches(ctx, userID)
		require.NoError(t, err)
		assert.Greater(t, len(searches), 0)
	})

	t.Run("UpdateSavedSearch", func(t *testing.T) {
		// Create a saved search first
		input := search.CreateSavedSearchInput{
			UserID:  userID,
			Name:    "Test Search",
			Filters: map[string]interface{}{},
		}

		savedSearch, err := repo.CreateSavedSearch(ctx, input)
		require.NoError(t, err)

		// Update it
		newName := "Updated Search"
		updateInput := search.UpdateSavedSearchInput{
			Name: &newName,
		}

		updated, err := repo.UpdateSavedSearch(ctx, savedSearch.ID, userID, updateInput)
		require.NoError(t, err)
		assert.Equal(t, newName, updated.Name)
	})

	t.Run("DeleteSavedSearch", func(t *testing.T) {
		// Create a saved search
		input := search.CreateSavedSearchInput{
			UserID:  userID,
			Name:    "To Delete",
			Filters: map[string]interface{}{},
		}

		savedSearch, err := repo.CreateSavedSearch(ctx, input)
		require.NoError(t, err)

		// Delete it
		err = repo.DeleteSavedSearch(ctx, savedSearch.ID, userID)
		require.NoError(t, err)

		// Verify it's deleted
		_, err = repo.GetSavedSearch(ctx, savedSearch.ID, userID)
		assert.Error(t, err)
	})
}

func TestGetSuggestions(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	repo := search.NewRepository(db)
	ctx := context.Background()

	userID := uuid.New()
	err := testutil.CreateTestUser(db, userID, "test@example.com")
	require.NoError(t, err)

	// Create test data with tags
	categoryID := uuid.New()
	err = testutil.CreateTestCategory(db, categoryID, userID, "Programming")
	require.NoError(t, err)

	tagID := uuid.New()
	err = testutil.CreateTestTag(db, tagID, userID, "golang")
	require.NoError(t, err)

	checkinID := uuid.New()
	err = testutil.CreateTestCheckin(db, checkinID, userID, &categoryID, "Working on Go project")
	require.NoError(t, err)

	err = testutil.LinkCheckinTag(db, checkinID, tagID)
	require.NoError(t, err)

	t.Run("TagSuggestions", func(t *testing.T) {
		suggestions, err := repo.GetSuggestions(ctx, userID, "go", 10)
		require.NoError(t, err)
		assert.Greater(t, len(suggestions), 0)

		// Should include the golang tag
		found := false
		for _, s := range suggestions {
			if s.Type == "tag" && s.Value == "golang" {
				found = true
				break
			}
		}
		assert.True(t, found, "Should find golang tag suggestion")
	})

	t.Run("CategorySuggestions", func(t *testing.T) {
		suggestions, err := repo.GetSuggestions(ctx, userID, "pro", 10)
		require.NoError(t, err)
		assert.Greater(t, len(suggestions), 0)

		// Should include Programming category
		found := false
		for _, s := range suggestions {
			if s.Type == "category" && s.Value == "Programming" {
				found = true
				break
			}
		}
		assert.True(t, found, "Should find Programming category suggestion")
	})
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}
