package category

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// MergeService handles category merge and batch operations
type MergeService struct {
	db     *sqlx.DB
	repo   RepositoryInterface
	logger *zap.Logger
}

// NewMergeService creates a new merge service
func NewMergeService(db *sqlx.DB, repo RepositoryInterface, logger *zap.Logger) *MergeService {
	return &MergeService{
		db:     db,
		repo:   repo,
		logger: logger,
	}
}

// MergeInput represents input for merging categories
type MergeInput struct {
	SourceCategoryIDs []uuid.UUID `json:"source_category_ids"`
	TargetCategoryID  uuid.UUID   `json:"target_category_id"`
	UserID            uuid.UUID   `json:"-"`
}

// MergeResult represents the result of a merge operation
type MergeResult struct {
	TargetCategory     *Category `json:"target_category"`
	MergedCount        int       `json:"merged_count"`
	UpdatedCheckinsCount int     `json:"updated_checkins_count"`
	DeletedCategories  []uuid.UUID `json:"deleted_categories"`
}

// MergeCategories merges multiple source categories into a target category
func (s *MergeService) MergeCategories(ctx context.Context, input MergeInput) (*MergeResult, error) {
	if len(input.SourceCategoryIDs) == 0 {
		return nil, fmt.Errorf("at least one source category is required")
	}

	// Validate that target category exists and belongs to user
	targetCategory, err := s.repo.GetByID(ctx, input.TargetCategoryID, input.UserID)
	if err != nil {
		return nil, fmt.Errorf("target category not found")
	}

	// Validate all source categories exist and belong to user
	for _, sourceID := range input.SourceCategoryIDs {
		if sourceID == input.TargetCategoryID {
			return nil, fmt.Errorf("source category cannot be the same as target category")
		}

		_, err := s.repo.GetByID(ctx, sourceID, input.UserID)
		if err != nil {
			return nil, fmt.Errorf("source category %s not found", sourceID.String())
		}
	}

	// Begin transaction for atomic merge
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	updatedCheckinsCount := 0
	deletedCategories := []uuid.UUID{}

	// Update all checkins from source categories to target category
	for _, sourceID := range input.SourceCategoryIDs {
		// Update checkins
		updateQuery := `
			UPDATE checkins
			SET category_id = $1, updated_at = CURRENT_TIMESTAMP
			WHERE category_id = $2 AND user_id = $3
		`
		result, err := tx.ExecContext(ctx, updateQuery, input.TargetCategoryID, sourceID, input.UserID)
		if err != nil {
			return nil, fmt.Errorf("failed to update checkins: %w", err)
		}

		rowsAffected, _ := result.RowsAffected()
		updatedCheckinsCount += int(rowsAffected)

		// Update edit history to reflect category changes
		editHistoryQuery := `
			UPDATE edit_history
			SET previous_category_id = $1
			WHERE previous_category_id = $2 AND user_id = $3
		`
		_, err = tx.ExecContext(ctx, editHistoryQuery, input.TargetCategoryID, sourceID, input.UserID)
		if err != nil {
			s.logger.Warn("Failed to update edit history during merge",
				zap.String("source_id", sourceID.String()),
				zap.Error(err))
			// Continue with merge even if edit history update fails
		}

		// Delete the source category
		deleteQuery := `
			DELETE FROM categories
			WHERE id = $1 AND user_id = $2
		`
		_, err = tx.ExecContext(ctx, deleteQuery, sourceID, input.UserID)
		if err != nil {
			return nil, fmt.Errorf("failed to delete source category: %w", err)
		}

		deletedCategories = append(deletedCategories, sourceID)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	s.logger.Info("Categories merged successfully",
		zap.String("target_id", input.TargetCategoryID.String()),
		zap.Int("source_count", len(input.SourceCategoryIDs)),
		zap.Int("updated_checkins", updatedCheckinsCount),
	)

	return &MergeResult{
		TargetCategory:      targetCategory,
		MergedCount:         len(input.SourceCategoryIDs),
		UpdatedCheckinsCount: updatedCheckinsCount,
		DeletedCategories:   deletedCategories,
	}, nil
}

// BatchUpdateInput represents input for batch updating categories
type BatchUpdateInput struct {
	CategoryIDs []uuid.UUID `json:"category_ids"`
	UserID      uuid.UUID   `json:"-"`
	Updates     BatchUpdateFields `json:"updates"`
}

// BatchUpdateFields represents fields that can be batch updated
type BatchUpdateFields struct {
	Color *string `json:"color,omitempty"`
	Icon  *string `json:"icon,omitempty"`
}

// BatchUpdateResult represents the result of a batch update operation
type BatchUpdateResult struct {
	UpdatedCategories []*Category `json:"updated_categories"`
	UpdatedCount      int         `json:"updated_count"`
	Errors            []string    `json:"errors,omitempty"`
}

// BatchUpdateCategories updates multiple categories with the same values
func (s *MergeService) BatchUpdateCategories(ctx context.Context, input BatchUpdateInput) (*BatchUpdateResult, error) {
	if len(input.CategoryIDs) == 0 {
		return nil, fmt.Errorf("at least one category is required")
	}

	// Validate that at least one field is being updated
	if input.Updates.Color == nil && input.Updates.Icon == nil {
		return nil, fmt.Errorf("at least one field must be updated")
	}

	// Validate all categories exist and belong to user
	existingCategories := make([]*Category, 0, len(input.CategoryIDs))
	for _, categoryID := range input.CategoryIDs {
		cat, err := s.repo.GetByID(ctx, categoryID, input.UserID)
		if err != nil {
			return nil, fmt.Errorf("category %s not found", categoryID.String())
		}
		existingCategories = append(existingCategories, cat)
	}

	// Begin transaction for atomic update
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	updatedCategories := make([]*Category, 0, len(input.CategoryIDs))
	errors := []string{}

	// Update each category
	for _, categoryID := range input.CategoryIDs {
		updateQuery := `
			UPDATE categories
			SET `

		args := []interface{}{}
		argIndex := 1

		// Build dynamic update query
		updates := []string{}
		if input.Updates.Color != nil {
			updates = append(updates, fmt.Sprintf("color = $%d", argIndex))
			args = append(args, *input.Updates.Color)
			argIndex++
		}
		if input.Updates.Icon != nil {
			updates = append(updates, fmt.Sprintf("icon = $%d", argIndex))
			args = append(args, *input.Updates.Icon)
			argIndex++
		}

		// Add updated_at
		updates = append(updates, "updated_at = CURRENT_TIMESTAMP")

		// Complete query
		updateQuery += fmt.Sprintf("%s WHERE id = $%d AND user_id = $%d",
			joinStrings(updates, ", "), argIndex, argIndex+1)
		updateQuery += " RETURNING id, user_id, name, color, icon, created_at, updated_at"

		args = append(args, categoryID, input.UserID)

		var updatedCategory Category
		err := tx.GetContext(ctx, &updatedCategory, updateQuery, args...)
		if err != nil {
			errors = append(errors, fmt.Sprintf("failed to update category %s: %v", categoryID.String(), err))
			continue
		}

		updatedCategories = append(updatedCategories, &updatedCategory)
	}

	// Commit transaction if we have successful updates
	if len(updatedCategories) > 0 {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("failed to commit transaction: %w", err)
		}
	} else {
		// Rollback if no successful updates
		tx.Rollback()
		return nil, fmt.Errorf("no categories were updated")
	}

	s.logger.Info("Categories batch updated",
		zap.Int("requested", len(input.CategoryIDs)),
		zap.Int("updated", len(updatedCategories)),
		zap.Int("errors", len(errors)),
	)

	return &BatchUpdateResult{
		UpdatedCategories: updatedCategories,
		UpdatedCount:      len(updatedCategories),
		Errors:            errors,
	}, nil
}

// BatchDeleteInput represents input for batch deleting categories
type BatchDeleteInput struct {
	CategoryIDs []uuid.UUID `json:"category_ids"`
	UserID      uuid.UUID   `json:"-"`
	// Optional: what to do with associated checkins
	DeleteOrphanedCheckins bool `json:"delete_orphaned_checkins"`
}

// BatchDeleteResult represents the result of a batch delete operation
type BatchDeleteResult struct {
	DeletedCount         int      `json:"deleted_count"`
	DeletedCategories    []uuid.UUID `json:"deleted_categories"`
	OrphanedCheckinsCount int      `json:"orphaned_checkins_count"`
	Errors               []string `json:"errors,omitempty"`
}

// BatchDeleteCategories deletes multiple categories
func (s *MergeService) BatchDeleteCategories(ctx context.Context, input BatchDeleteInput) (*BatchDeleteResult, error) {
	if len(input.CategoryIDs) == 0 {
		return nil, fmt.Errorf("at least one category is required")
	}

	// Validate all categories exist and belong to user
	for _, categoryID := range input.CategoryIDs {
		_, err := s.repo.GetByID(ctx, categoryID, input.UserID)
		if err != nil {
			return nil, fmt.Errorf("category %s not found", categoryID.String())
		}
	}

	// Begin transaction
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	deletedCategories := []uuid.UUID{}
	orphanedCheckinsCount := 0
	errors := []string{}

	for _, categoryID := range input.CategoryIDs {
		// Handle associated checkins
		if input.DeleteOrphanedCheckins {
			// Delete checkins associated with this category
			deleteCheckinsQuery := `
				DELETE FROM checkins
				WHERE category_id = $1 AND user_id = $2
			`
			result, err := tx.ExecContext(ctx, deleteCheckinsQuery, categoryID, input.UserID)
			if err != nil {
				errors = append(errors, fmt.Sprintf("failed to delete checkins for category %s: %v", categoryID.String(), err))
				continue
			}
			rowsAffected, _ := result.RowsAffected()
			orphanedCheckinsCount += int(rowsAffected)
		} else {
			// Set category_id to NULL for associated checkins
			updateCheckinsQuery := `
				UPDATE checkins
				SET category_id = NULL, updated_at = CURRENT_TIMESTAMP
				WHERE category_id = $1 AND user_id = $2
			`
			result, err := tx.ExecContext(ctx, updateCheckinsQuery, categoryID, input.UserID)
			if err != nil {
				errors = append(errors, fmt.Sprintf("failed to update checkins for category %s: %v", categoryID.String(), err))
				continue
			}
			rowsAffected, _ := result.RowsAffected()
			orphanedCheckinsCount += int(rowsAffected)
		}

		// Delete the category
		deleteQuery := `
			DELETE FROM categories
			WHERE id = $1 AND user_id = $2
		`
		_, err := tx.ExecContext(ctx, deleteQuery, categoryID, input.UserID)
		if err != nil {
			errors = append(errors, fmt.Sprintf("failed to delete category %s: %v", categoryID.String(), err))
			continue
		}

		deletedCategories = append(deletedCategories, categoryID)
	}

	// Commit transaction if we have successful deletions
	if len(deletedCategories) > 0 {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("failed to commit transaction: %w", err)
		}
	} else {
		tx.Rollback()
		return nil, fmt.Errorf("no categories were deleted")
	}

	s.logger.Info("Categories batch deleted",
		zap.Int("requested", len(input.CategoryIDs)),
		zap.Int("deleted", len(deletedCategories)),
		zap.Int("orphaned_checkins", orphanedCheckinsCount),
		zap.Int("errors", len(errors)),
	)

	return &BatchDeleteResult{
		DeletedCount:          len(deletedCategories),
		DeletedCategories:     deletedCategories,
		OrphanedCheckinsCount: orphanedCheckinsCount,
		Errors:                errors,
	}, nil
}

// Helper function to join strings
func joinStrings(strs []string, separator string) string {
	result := ""
	for i, str := range strs {
		if i > 0 {
			result += separator
		}
		result += str
	}
	return result
}