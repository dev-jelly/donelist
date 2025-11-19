package search

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// Repository handles search database operations
type Repository struct {
	db *sqlx.DB
}

// NewRepository creates a new search repository
func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

// Search performs a full-text search with filters
func (r *Repository) Search(ctx context.Context, userID uuid.UUID, filters SearchFilters) ([]SearchResult, int64, error) {
	// Validate and set defaults
	filters.Limit = ValidateLimit(filters.Limit)
	filters.Offset = ValidateOffset(filters.Offset)

	// Build base query with full-text search ranking
	baseQuery := `
		SELECT DISTINCT
			checkins.id,
			checkins.user_id,
			checkins.category_id,
			categories.name as category_name,
			checkins.content,
			checkins.checkin_time,
			checkins.duration_minutes,
			checkins.is_edited,
			checkins.edit_count,
			checkins.created_at,
			checkins.updated_at
	`

	// Add ranking and snippet if full-text search is used
	if filters.Query != "" {
		baseQuery += `,
			ts_rank(checkins.search_vector, to_tsquery('english', $1)) as rank,
			ts_headline('english', checkins.content, to_tsquery('english', $1),
				'MaxWords=20, MinWords=10, MaxFragments=1') as snippet
		`
	}

	baseQuery += `
		FROM checkins
		LEFT JOIN categories ON checkins.category_id = categories.id
	`

	// Build query using query builder
	qb := NewQueryBuilder(baseQuery)

	// Add full-text search if query provided
	if filters.Query != "" {
		qb.AddFullTextSearch(filters.Query)
	}

	// Add user filter
	qb.AddUserFilter(userID)

	// Add deleted filter
	qb.AddDeletedFilter()

	// Add date range filter
	var startDateStr, endDateStr *string
	if filters.StartDate != nil {
		str := filters.StartDate.Format(time.RFC3339)
		startDateStr = &str
	}
	if filters.EndDate != nil {
		str := filters.EndDate.Format(time.RFC3339)
		endDateStr = &str
	}
	qb.AddDateRange(startDateStr, endDateStr)

	// Add category filter
	qb.AddCategoryFilter(filters.CategoryIDs)

	// Add tag filter
	if len(filters.TagIDs) > 0 {
		qb.AddTagFilter(filters.TagIDs)
	} else if len(filters.TagNames) > 0 {
		// Convert tag names to IDs
		tagIDs, err := r.getTagIDsByNames(ctx, userID, filters.TagNames)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to resolve tag names: %w", err)
		}
		qb.AddTagFilter(tagIDs)
	}

	// Add duration filter
	qb.AddDurationFilter(filters.MinDuration, filters.MaxDuration)

	// Add edited filter
	qb.AddEditedFilter(filters.IsEdited)

	// Build the query
	query, _  := qb.Build()

	// Get total count
	countQuery := "SELECT COUNT(DISTINCT checkins.id) FROM checkins"
	if filters.Query != "" {
		countQuery += " WHERE search_vector @@ to_tsquery('english', $1) AND user_id = $2 AND deleted_at IS NULL"
	} else {
		countQuery += " WHERE user_id = $1 AND deleted_at IS NULL"
	}

	var total int64
	var countArgs []interface{}
	if filters.Query != "" {
		tsQuery := sanitizeTsQuery(filters.Query)
		countArgs = []interface{}{tsQuery, userID}
	} else {
		countArgs = []interface{}{userID}
	}

	if err := r.db.GetContext(ctx, &total, countQuery, countArgs...); err != nil {
		return nil, 0, fmt.Errorf("failed to count results: %w", err)
	}

	// Add sorting
	sortField := ValidateSortField(filters.SortBy)
	sortDirection := ValidateSortDirection(filters.SortDirection)

	// If full-text search is used and no explicit sort, sort by relevance
	if filters.Query != "" && filters.SortBy == "" {
		sortField = "rank"
		sortDirection = "DESC"
	}

	query += fmt.Sprintf(" ORDER BY %s %s", sortField, sortDirection)

	// Add pagination
	query += fmt.Sprintf(" LIMIT %s OFFSET %s",
		qb.AppendArg(filters.Limit),
		qb.AppendArg(filters.Offset))

	// Execute query
	var results []SearchResult
	if err := r.db.SelectContext(ctx, &results, query, qb.GetArgs()...); err != nil {
		return nil, 0, fmt.Errorf("failed to execute search: %w", err)
	}

	// Load tags for each result
	for i := range results {
		tags, err := r.getCheckinTags(ctx, results[i].ID)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to load tags: %w", err)
		}
		results[i].Tags = tags
	}

	return results, total, nil
}

// GetFacets returns faceted search results for filtering
func (r *Repository) GetFacets(ctx context.Context, userID uuid.UUID, filters SearchFilters) (*SearchFacets, error) {
	facets := &SearchFacets{
		Categories: make([]CategoryFacet, 0),
		Tags:       make([]TagFacet, 0),
		Durations:  make([]DurationFacet, 0),
	}

	// Get category facets
	categoryQuery := `
		SELECT
			c.id,
			c.name,
			c.color,
			COUNT(DISTINCT ch.id) as count
		FROM categories c
		INNER JOIN checkins ch ON ch.category_id = c.id
		WHERE ch.user_id = $1 AND ch.deleted_at IS NULL
		GROUP BY c.id, c.name, c.color
		ORDER BY count DESC
		LIMIT 20
	`

	if err := r.db.SelectContext(ctx, &facets.Categories, categoryQuery, userID); err != nil {
		return nil, fmt.Errorf("failed to get category facets: %w", err)
	}

	// Get tag facets
	tagQuery := `
		SELECT
			t.id,
			t.name,
			COUNT(DISTINCT ct.checkin_id) as count
		FROM tags t
		INNER JOIN checkin_tags ct ON ct.tag_id = t.id
		INNER JOIN checkins ch ON ch.id = ct.checkin_id
		WHERE ch.user_id = $1 AND ch.deleted_at IS NULL
		GROUP BY t.id, t.name
		ORDER BY count DESC
		LIMIT 20
	`

	if err := r.db.SelectContext(ctx, &facets.Tags, tagQuery, userID); err != nil {
		return nil, fmt.Errorf("failed to get tag facets: %w", err)
	}

	// Get duration facets
	durationQuery := `
		SELECT
			duration_minutes as duration,
			COUNT(*) as count
		FROM checkins
		WHERE user_id = $1 AND deleted_at IS NULL
		GROUP BY duration_minutes
		ORDER BY duration_minutes
	`

	if err := r.db.SelectContext(ctx, &facets.Durations, durationQuery, userID); err != nil {
		return nil, fmt.Errorf("failed to get duration facets: %w", err)
	}

	return facets, nil
}

// GetSuggestions returns search suggestions based on partial input
func (r *Repository) GetSuggestions(ctx context.Context, userID uuid.UUID, prefix string, limit int) ([]SuggestionResult, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}

	suggestions := make([]SuggestionResult, 0)

	// Get tag suggestions
	tagQuery := `
		SELECT DISTINCT t.name
		FROM tags t
		INNER JOIN checkin_tags ct ON ct.tag_id = t.id
		INNER JOIN checkins ch ON ch.id = ct.checkin_id
		WHERE ch.user_id = $1
			AND LOWER(t.name) LIKE LOWER($2)
			AND ch.deleted_at IS NULL
		ORDER BY t.name
		LIMIT $3
	`

	var tagNames []string
	if err := r.db.SelectContext(ctx, &tagNames, tagQuery, userID, prefix+"%", limit); err != nil {
		return nil, fmt.Errorf("failed to get tag suggestions: %w", err)
	}

	for _, name := range tagNames {
		suggestions = append(suggestions, SuggestionResult{
			Type:  "tag",
			Value: name,
		})
	}

	// Get category suggestions if we have room
	if len(suggestions) < limit {
		categoryQuery := `
			SELECT DISTINCT c.name
			FROM categories c
			INNER JOIN checkins ch ON ch.category_id = c.id
			WHERE ch.user_id = $1
				AND LOWER(c.name) LIKE LOWER($2)
				AND ch.deleted_at IS NULL
			ORDER BY c.name
			LIMIT $3
		`

		var categoryNames []string
		if err := r.db.SelectContext(ctx, &categoryNames, categoryQuery, userID, prefix+"%", limit-len(suggestions)); err != nil {
			return nil, fmt.Errorf("failed to get category suggestions: %w", err)
		}

		for _, name := range categoryNames {
			suggestions = append(suggestions, SuggestionResult{
				Type:  "category",
				Value: name,
			})
		}
	}

	return suggestions, nil
}

// SaveSearchHistory saves a search query to history
func (r *Repository) SaveSearchHistory(ctx context.Context, userID uuid.UUID, query string, filters map[string]interface{}, resultCount int) error {
	filtersJSON, err := json.Marshal(filters)
	if err != nil {
		return fmt.Errorf("failed to marshal filters: %w", err)
	}

	insertQuery := `
		INSERT INTO search_history (user_id, query, filters, result_count)
		VALUES ($1, $2, $3, $4)
	`

	_, err = r.db.ExecContext(ctx, insertQuery, userID, query, filtersJSON, resultCount)
	if err != nil {
		return fmt.Errorf("failed to save search history: %w", err)
	}

	return nil
}

// GetSearchHistory retrieves recent search history for a user
func (r *Repository) GetSearchHistory(ctx context.Context, userID uuid.UUID, limit int) ([]SearchHistory, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	query := `
		SELECT id, user_id, query, filters, result_count, created_at
		FROM search_history
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	var history []SearchHistory
	if err := r.db.SelectContext(ctx, &history, query, userID, limit); err != nil {
		return nil, fmt.Errorf("failed to get search history: %w", err)
	}

	return history, nil
}

// CreateSavedSearch creates a new saved search
func (r *Repository) CreateSavedSearch(ctx context.Context, input CreateSavedSearchInput) (*SavedSearch, error) {
	filtersJSON, err := json.Marshal(input.Filters)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal filters: %w", err)
	}

	query := `
		INSERT INTO saved_searches (user_id, name, description, query, filters, is_favorite)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, user_id, name, description, query, filters, is_favorite,
				  last_used_at, use_count, created_at, updated_at
	`

	var savedSearch SavedSearch
	err = r.db.GetContext(ctx, &savedSearch, query,
		input.UserID, input.Name, input.Description, input.Query, filtersJSON, input.IsFavorite)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return nil, fmt.Errorf("saved search with this name already exists")
		}
		return nil, fmt.Errorf("failed to create saved search: %w", err)
	}

	return &savedSearch, nil
}

// GetSavedSearch retrieves a saved search by ID
func (r *Repository) GetSavedSearch(ctx context.Context, id, userID uuid.UUID) (*SavedSearch, error) {
	query := `
		SELECT id, user_id, name, description, query, filters, is_favorite,
			   last_used_at, use_count, created_at, updated_at
		FROM saved_searches
		WHERE id = $1 AND user_id = $2
	`

	var savedSearch SavedSearch
	err := r.db.GetContext(ctx, &savedSearch, query, id, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("saved search not found")
		}
		return nil, fmt.Errorf("failed to get saved search: %w", err)
	}

	return &savedSearch, nil
}

// ListSavedSearches lists all saved searches for a user
func (r *Repository) ListSavedSearches(ctx context.Context, userID uuid.UUID) ([]SavedSearch, error) {
	query := `
		SELECT id, user_id, name, description, query, filters, is_favorite,
			   last_used_at, use_count, created_at, updated_at
		FROM saved_searches
		WHERE user_id = $1
		ORDER BY is_favorite DESC, last_used_at DESC NULLS LAST, created_at DESC
	`

	var searches []SavedSearch
	if err := r.db.SelectContext(ctx, &searches, query, userID); err != nil {
		return nil, fmt.Errorf("failed to list saved searches: %w", err)
	}

	return searches, nil
}

// UpdateSavedSearch updates a saved search
func (r *Repository) UpdateSavedSearch(ctx context.Context, id, userID uuid.UUID, input UpdateSavedSearchInput) (*SavedSearch, error) {
	// Build dynamic update query
	query := "UPDATE saved_searches SET updated_at = CURRENT_TIMESTAMP"
	args := []interface{}{}
	argIndex := 1

	if input.Name != nil {
		query += fmt.Sprintf(", name = $%d", argIndex)
		args = append(args, *input.Name)
		argIndex++
	}

	if input.Description != nil {
		query += fmt.Sprintf(", description = $%d", argIndex)
		args = append(args, *input.Description)
		argIndex++
	}

	if input.Query != nil {
		query += fmt.Sprintf(", query = $%d", argIndex)
		args = append(args, *input.Query)
		argIndex++
	}

	if input.Filters != nil {
		filtersJSON, err := json.Marshal(input.Filters)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal filters: %w", err)
		}
		query += fmt.Sprintf(", filters = $%d", argIndex)
		args = append(args, filtersJSON)
		argIndex++
	}

	if input.IsFavorite != nil {
		query += fmt.Sprintf(", is_favorite = $%d", argIndex)
		args = append(args, *input.IsFavorite)
		argIndex++
	}

	query += fmt.Sprintf(" WHERE id = $%d AND user_id = $%d", argIndex, argIndex+1)
	args = append(args, id, userID)

	query += ` RETURNING id, user_id, name, description, query, filters, is_favorite,
				last_used_at, use_count, created_at, updated_at`

	var savedSearch SavedSearch
	err := r.db.GetContext(ctx, &savedSearch, query, args...)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("saved search not found")
		}
		return nil, fmt.Errorf("failed to update saved search: %w", err)
	}

	return &savedSearch, nil
}

// DeleteSavedSearch deletes a saved search
func (r *Repository) DeleteSavedSearch(ctx context.Context, id, userID uuid.UUID) error {
	query := "DELETE FROM saved_searches WHERE id = $1 AND user_id = $2"

	result, err := r.db.ExecContext(ctx, query, id, userID)
	if err != nil {
		return fmt.Errorf("failed to delete saved search: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("saved search not found")
	}

	return nil
}

// IncrementSavedSearchUsage increments use count and updates last used time
func (r *Repository) IncrementSavedSearchUsage(ctx context.Context, id, userID uuid.UUID) error {
	query := `
		UPDATE saved_searches
		SET use_count = use_count + 1, last_used_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND user_id = $2
	`

	_, err := r.db.ExecContext(ctx, query, id, userID)
	if err != nil {
		return fmt.Errorf("failed to increment usage: %w", err)
	}

	return nil
}

// Helper functions

// getCheckinTags retrieves tag names for a checkin
func (r *Repository) getCheckinTags(ctx context.Context, checkinID uuid.UUID) ([]string, error) {
	query := `
		SELECT t.name
		FROM tags t
		INNER JOIN checkin_tags ct ON ct.tag_id = t.id
		WHERE ct.checkin_id = $1
		ORDER BY t.name
	`

	var tags []string
	if err := r.db.SelectContext(ctx, &tags, query, checkinID); err != nil {
		return nil, err
	}

	return tags, nil
}

// getTagIDsByNames converts tag names to IDs
func (r *Repository) getTagIDsByNames(ctx context.Context, userID uuid.UUID, tagNames []string) ([]uuid.UUID, error) {
	if len(tagNames) == 0 {
		return []uuid.UUID{}, nil
	}

	query := `
		SELECT id
		FROM tags
		WHERE user_id = $1 AND name = ANY($2)
	`

	var tagIDs []uuid.UUID
	if err := r.db.SelectContext(ctx, &tagIDs, query, userID, pq.Array(tagNames)); err != nil {
		return nil, err
	}

	return tagIDs, nil
}
