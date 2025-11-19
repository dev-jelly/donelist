package tag

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// Tag represents a check-in tag
type Tag struct {
	ID         uuid.UUID  `db:"id" json:"id"`
	UserID     uuid.UUID  `db:"user_id" json:"user_id"`
	Name       string     `db:"name" json:"name"`
	Slug       string     `db:"slug" json:"slug"`
	UsageCount int        `db:"usage_count" json:"usage_count"`
	CreatedAt  time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt  time.Time  `db:"updated_at" json:"updated_at"`
	DeletedAt  *time.Time `db:"deleted_at" json:"deleted_at,omitempty"`
}

// CreateTagInput represents input for creating a tag
type CreateTagInput struct {
	UserID uuid.UUID
	Name   string
}

// Repository handles tag database operations
type Repository struct {
	db *sqlx.DB
}

// NewRepository creates a new tag repository
func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

// Create creates a new tag
func (r *Repository) Create(ctx context.Context, input CreateTagInput) (*Tag, error) {
	// Generate slug from name (lowercase, replace spaces with hyphens, remove special chars)
	slug := generateSlug(input.Name)

	query := `
		INSERT INTO tags (user_id, name, slug)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, name, slug, usage_count, created_at, updated_at, deleted_at
	`

	var tag Tag
	err := r.db.GetContext(ctx, &tag, query, input.UserID, input.Name, slug)
	if err != nil {
		return nil, fmt.Errorf("failed to create tag: %w", err)
	}

	return &tag, nil
}

// generateSlug creates a URL-friendly slug from a name
func generateSlug(name string) string {
	// Convert to lowercase
	slug := strings.ToLower(name)
	// Replace spaces with hyphens
	slug = strings.ReplaceAll(slug, " ", "-")
	// Remove special characters (keep only alphanumeric, hyphens, and underscores)
	var result []rune
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			result = append(result, r)
		}
	}
	return string(result)
}

// GetByID retrieves a tag by ID
func (r *Repository) GetByID(ctx context.Context, id, userID uuid.UUID) (*Tag, error) {
	query := `
		SELECT id, user_id, name, slug, usage_count, created_at, updated_at, deleted_at
		FROM tags
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
	`

	var tag Tag
	err := r.db.GetContext(ctx, &tag, query, id, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("tag not found")
		}
		return nil, fmt.Errorf("failed to get tag: %w", err)
	}

	return &tag, nil
}

// GetByName retrieves a tag by name
func (r *Repository) GetByName(ctx context.Context, userID uuid.UUID, name string) (*Tag, error) {
	query := `
		SELECT id, user_id, name, slug, usage_count, created_at, updated_at, deleted_at
		FROM tags
		WHERE user_id = $1 AND name = $2 AND deleted_at IS NULL
	`

	var tag Tag
	err := r.db.GetContext(ctx, &tag, query, userID, name)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("tag not found")
		}
		return nil, fmt.Errorf("failed to get tag: %w", err)
	}

	return &tag, nil
}

// List retrieves all tags for a user
func (r *Repository) List(ctx context.Context, userID uuid.UUID) ([]*Tag, error) {
	query := `
		SELECT id, user_id, name, slug, usage_count, created_at, updated_at, deleted_at
		FROM tags
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY usage_count DESC, name
	`

	var tags []*Tag
	if err := r.db.SelectContext(ctx, &tags, query, userID); err != nil {
		return nil, fmt.Errorf("failed to list tags: %w", err)
	}

	return tags, nil
}

// NameExists checks if a tag name already exists for a user
func (r *Repository) NameExists(ctx context.Context, userID uuid.UUID, name string) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM tags WHERE user_id = $1 AND name = $2 AND deleted_at IS NULL
		)
	`

	var exists bool
	if err := r.db.GetContext(ctx, &exists, query, userID, name); err != nil {
		return false, fmt.Errorf("failed to check tag name: %w", err)
	}

	return exists, nil
}

// GetOrCreate gets an existing tag or creates a new one
func (r *Repository) GetOrCreate(ctx context.Context, userID uuid.UUID, name string) (*Tag, error) {
	// Try to get existing tag
	tag, err := r.GetByName(ctx, userID, name)
	if err == nil {
		return tag, nil
	}

	// Create new tag if not found
	return r.Create(ctx, CreateTagInput{
		UserID: userID,
		Name:   name,
	})
}

// GetByNames gets multiple tags by names, creating them if they don't exist
func (r *Repository) GetByNames(ctx context.Context, userID uuid.UUID, names []string) ([]*Tag, error) {
	tags := make([]*Tag, 0, len(names))

	for _, name := range names {
		tag, err := r.GetOrCreate(ctx, userID, name)
		if err != nil {
			return nil, fmt.Errorf("failed to get or create tag %s: %w", name, err)
		}
		tags = append(tags, tag)
	}

	return tags, nil
}

// SearchByPrefix searches for tags matching the given prefix
func (r *Repository) SearchByPrefix(ctx context.Context, userID uuid.UUID, prefix string, limit int) ([]*Tag, error) {
	if limit <= 0 || limit > 50 {
		limit = 20 // Default limit
	}

	query := `
		SELECT id, user_id, name, slug, usage_count, created_at, updated_at, deleted_at
		FROM tags
		WHERE user_id = $1 AND name ILIKE $2 AND deleted_at IS NULL
		ORDER BY usage_count DESC, name
		LIMIT $3
	`

	// Use ILIKE for case-insensitive prefix matching
	searchPattern := prefix + "%"

	var tags []*Tag
	if err := r.db.SelectContext(ctx, &tags, query, userID, searchPattern, limit); err != nil {
		return nil, fmt.Errorf("failed to search tags: %w", err)
	}

	return tags, nil
}

// SearchWithTrigram searches for tags using trigram similarity
// This provides fuzzy matching and typo tolerance using pg_trgm extension
func (r *Repository) SearchWithTrigram(ctx context.Context, userID uuid.UUID, query string, limit int) ([]*Tag, error) {
	if limit <= 0 || limit > 50 {
		limit = 20 // Default limit
	}

	sqlQuery := `
		SELECT
			id,
			user_id,
			name,
			created_at,
			usage_count,
			similarity(name, $2) AS similarity_score
		FROM tags
		WHERE user_id = $1
			AND deleted_at IS NULL
			AND (
				name ILIKE $3  -- Exact prefix match gets priority
				OR similarity(name, $2) > 0.3  -- Trigram similarity threshold
			)
		ORDER BY
			CASE WHEN name ILIKE $3 THEN 0 ELSE 1 END,  -- Prefix matches first
			similarity(name, $2) DESC,  -- Then by similarity
			usage_count DESC,  -- Then by popularity
			name  -- Finally alphabetically
		LIMIT $4
	`

	searchPattern := query + "%"

	var tags []*Tag
	if err := r.db.SelectContext(ctx, &tags, sqlQuery, userID, query, searchPattern, limit); err != nil {
		return nil, fmt.Errorf("failed to search tags with trigram: %w", err)
	}

	return tags, nil
}

// SuggestTags provides intelligent tag suggestions based on:
// 1. Prefix matching (highest priority)
// 2. Trigram similarity (fuzzy matching)
// 3. Usage frequency
// 4. Recent usage patterns
func (r *Repository) SuggestTags(ctx context.Context, userID uuid.UUID, query string, limit int) ([]*Tag, error) {
	if limit <= 0 || limit > 50 {
		limit = 20 // Default limit
	}

	// If query is empty, return popular tags
	if query == "" {
		return r.GetPopularTags(ctx, userID, limit)
	}

	// For short queries (1-2 chars), use prefix matching only
	if len(query) <= 2 {
		return r.SearchByPrefix(ctx, userID, query, limit)
	}

	// For longer queries, use trigram similarity for better fuzzy matching
	return r.SearchWithTrigram(ctx, userID, query, limit)
}

// GetPopularTags retrieves the most frequently used tags for a user
func (r *Repository) GetPopularTags(ctx context.Context, userID uuid.UUID, limit int) ([]*Tag, error) {
	if limit <= 0 || limit > 50 {
		limit = 10 // Default limit
	}

	query := `
		SELECT id, user_id, name, slug, usage_count, created_at, updated_at, deleted_at
		FROM tags
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY usage_count DESC, name
		LIMIT $2
	`

	var tags []*Tag
	if err := r.db.SelectContext(ctx, &tags, query, userID, limit); err != nil {
		return nil, fmt.Errorf("failed to get popular tags: %w", err)
	}

	return tags, nil
}
