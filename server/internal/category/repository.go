package category

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// RepositoryInterface defines the interface for category repository operations
type RepositoryInterface interface {
	Create(ctx context.Context, input CreateCategoryInput) (*Category, error)
	GetByID(ctx context.Context, id, userID uuid.UUID) (*Category, error)
	List(ctx context.Context, userID uuid.UUID) ([]*Category, error)
	Update(ctx context.Context, id, userID uuid.UUID, input UpdateCategoryInput) (*Category, error)
	Delete(ctx context.Context, id, userID uuid.UUID) error
	NameExists(ctx context.Context, userID uuid.UUID, name string) (bool, error)
}

// Category represents a check-in category
type Category struct {
	ID        uuid.UUID `db:"id" json:"id"`
	UserID    uuid.UUID `db:"user_id" json:"user_id"`
	Name      string    `db:"name" json:"name"`
	Color     *string   `db:"color" json:"color,omitempty"`
	Icon      *string   `db:"icon" json:"icon,omitempty"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// CreateCategoryInput represents input for creating a category
type CreateCategoryInput struct {
	UserID uuid.UUID
	Name   string
	Color  *string
	Icon   *string
}

// UpdateCategoryInput represents input for updating a category
type UpdateCategoryInput struct {
	Name  *string
	Color *string
	Icon  *string
}

// Repository handles category database operations
type Repository struct {
	db *sqlx.DB
}

// NewRepository creates a new category repository
func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

// Create creates a new category
func (r *Repository) Create(ctx context.Context, input CreateCategoryInput) (*Category, error) {
	query := `
		INSERT INTO categories (user_id, name, color, icon)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, name, color, icon, created_at, updated_at
	`

	var category Category
	err := r.db.GetContext(ctx, &category, query, input.UserID, input.Name, input.Color, input.Icon)
	if err != nil {
		return nil, fmt.Errorf("failed to create category: %w", err)
	}

	return &category, nil
}

// GetByID retrieves a category by ID
func (r *Repository) GetByID(ctx context.Context, id, userID uuid.UUID) (*Category, error) {
	query := `
		SELECT id, user_id, name, color, icon, created_at, updated_at
		FROM categories
		WHERE id = $1 AND user_id = $2
	`

	var category Category
	err := r.db.GetContext(ctx, &category, query, id, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("category not found")
		}
		return nil, fmt.Errorf("failed to get category: %w", err)
	}

	return &category, nil
}

// List retrieves all categories for a user
func (r *Repository) List(ctx context.Context, userID uuid.UUID) ([]*Category, error) {
	query := `
		SELECT id, user_id, name, color, icon, created_at, updated_at
		FROM categories
		WHERE user_id = $1
		ORDER BY name
	`

	var categories []*Category
	if err := r.db.SelectContext(ctx, &categories, query, userID); err != nil {
		return nil, fmt.Errorf("failed to list categories: %w", err)
	}

	return categories, nil
}

// Update updates a category
func (r *Repository) Update(ctx context.Context, id, userID uuid.UUID, input UpdateCategoryInput) (*Category, error) {
	query := `
		UPDATE categories
		SET name = COALESCE($3, name),
		    color = COALESCE($4, color),
		    icon = COALESCE($5, icon),
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND user_id = $2
		RETURNING id, user_id, name, color, icon, created_at, updated_at
	`

	var category Category
	err := r.db.GetContext(ctx, &category, query, id, userID, input.Name, input.Color, input.Icon)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("category not found")
		}
		return nil, fmt.Errorf("failed to update category: %w", err)
	}

	return &category, nil
}

// Delete deletes a category
func (r *Repository) Delete(ctx context.Context, id, userID uuid.UUID) error {
	query := `
		DELETE FROM categories
		WHERE id = $1 AND user_id = $2
	`

	result, err := r.db.ExecContext(ctx, query, id, userID)
	if err != nil {
		return fmt.Errorf("failed to delete category: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("category not found")
	}

	return nil
}

// NameExists checks if a category name already exists for a user
func (r *Repository) NameExists(ctx context.Context, userID uuid.UUID, name string) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM categories WHERE user_id = $1 AND name = $2
		)
	`

	var exists bool
	if err := r.db.GetContext(ctx, &exists, query, userID, name); err != nil {
		return false, fmt.Errorf("failed to check category name: %w", err)
	}

	return exists, nil
}
