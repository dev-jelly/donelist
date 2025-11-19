package category

import (
	"context"
	"fmt"

	"github.com/dev-jelly/donelist/internal/color"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Service handles category business logic
type Service struct {
	repo   RepositoryInterface
	logger *zap.Logger
}

// NewService creates a new category service
func NewService(repo RepositoryInterface, logger *zap.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
	}
}

// CreateInput represents service-level create input
type CreateInput struct {
	UserID uuid.UUID
	Name   string
	Color  *string
	Icon   *string
}

// UpdateInput represents service-level update input
type UpdateInput struct {
	Name  *string
	Color *string
	Icon  *string
}

// Create creates a new category
func (s *Service) Create(ctx context.Context, input CreateInput) (*Category, error) {
	// Validate name
	if input.Name == "" {
		return nil, fmt.Errorf("name cannot be empty")
	}
	if len(input.Name) > 50 {
		return nil, fmt.Errorf("name must be less than 50 characters")
	}

	// Check if name already exists
	exists, err := s.repo.NameExists(ctx, input.UserID, input.Name)
	if err != nil {
		s.logger.Error("Failed to check category name", zap.Error(err))
		return nil, fmt.Errorf("failed to check category name")
	}
	if exists {
		return nil, fmt.Errorf("category with this name already exists")
	}

	// Validate and normalize color format if provided (hex or HSL)
	var normalizedColor *string
	if input.Color != nil && len(*input.Color) > 0 {
		parsedColor, err := color.ValidateColor(*input.Color)
		if err != nil {
			s.logger.Warn("Invalid color format, using default",
				zap.String("input_color", *input.Color),
				zap.Error(err))
			// Use default fallback color
			fallback := color.DefaultFallbackColor
			normalizedColor = &fallback
		} else {
			// Normalize to hex format for storage
			normalizedColor = &parsedColor.Hex

			// Check if color is too similar to existing colors
			existingCategories, err := s.repo.List(ctx, input.UserID)
			if err == nil {
				for _, existing := range existingCategories {
					if existing.Color != nil {
						existingColor, err := color.ParseHexColor(*existing.Color)
						if err == nil && color.IsSimilarColor(parsedColor, existingColor, 0.15) {
							return nil, fmt.Errorf("color is too similar to existing category '%s'", existing.Name)
						}
					}
				}
			}
		}
	} else {
		normalizedColor = input.Color
	}

	category, err := s.repo.Create(ctx, CreateCategoryInput{
		UserID: input.UserID,
		Name:   input.Name,
		Color:  normalizedColor,
		Icon:   input.Icon,
	})
	if err != nil {
		s.logger.Error("Failed to create category", zap.Error(err))
		return nil, fmt.Errorf("failed to create category")
	}

	s.logger.Info("Category created",
		zap.String("category_id", category.ID.String()),
		zap.String("user_id", input.UserID.String()),
	)

	return category, nil
}

// GetByID retrieves a category by ID
func (s *Service) GetByID(ctx context.Context, id, userID uuid.UUID) (*Category, error) {
	category, err := s.repo.GetByID(ctx, id, userID)
	if err != nil {
		return nil, fmt.Errorf("category not found")
	}

	return category, nil
}

// List retrieves all categories for a user
func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]*Category, error) {
	categories, err := s.repo.List(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to list categories", zap.Error(err))
		return nil, fmt.Errorf("failed to list categories")
	}

	return categories, nil
}

// Update updates a category
func (s *Service) Update(ctx context.Context, id, userID uuid.UUID, input UpdateInput) (*Category, error) {
	// Validate name if provided
	if input.Name != nil {
		if *input.Name == "" {
			return nil, fmt.Errorf("name cannot be empty")
		}
		if len(*input.Name) > 50 {
			return nil, fmt.Errorf("name must be less than 50 characters")
		}
	}

	// Validate and normalize color format if provided
	var normalizedColor *string
	if input.Color != nil && len(*input.Color) > 0 {
		parsedColor, err := color.ValidateColor(*input.Color)
		if err != nil {
			s.logger.Warn("Invalid color format, using default",
				zap.String("input_color", *input.Color),
				zap.Error(err))
			// Use default fallback color
			fallback := color.DefaultFallbackColor
			normalizedColor = &fallback
		} else {
			// Normalize to hex format for storage
			normalizedColor = &parsedColor.Hex

			// Check if color is too similar to other user's categories (excluding current)
			existingCategories, err := s.repo.List(ctx, userID)
			if err == nil {
				for _, existing := range existingCategories {
					// Skip the current category being updated
					if existing.ID == id {
						continue
					}
					if existing.Color != nil {
						existingColor, err := color.ParseHexColor(*existing.Color)
						if err == nil && color.IsSimilarColor(parsedColor, existingColor, 0.15) {
							return nil, fmt.Errorf("color is too similar to existing category '%s'", existing.Name)
						}
					}
				}
			}
		}
	} else {
		normalizedColor = input.Color
	}

	category, err := s.repo.Update(ctx, id, userID, UpdateCategoryInput{
		Name:  input.Name,
		Color: normalizedColor,
		Icon:  input.Icon,
	})
	if err != nil {
		s.logger.Error("Failed to update category", zap.Error(err))
		return nil, fmt.Errorf("failed to update category")
	}

	s.logger.Info("Category updated",
		zap.String("category_id", id.String()),
		zap.String("user_id", userID.String()),
	)

	return category, nil
}

// Delete deletes a category
func (s *Service) Delete(ctx context.Context, id, userID uuid.UUID) error {
	if err := s.repo.Delete(ctx, id, userID); err != nil {
		s.logger.Error("Failed to delete category", zap.Error(err))
		return fmt.Errorf("failed to delete category")
	}

	s.logger.Info("Category deleted",
		zap.String("category_id", id.String()),
		zap.String("user_id", userID.String()),
	)

	return nil
}

// GetRecommendedColors returns a list of recommended colors for the user
// based on their existing categories
func (s *Service) GetRecommendedColors(ctx context.Context, userID uuid.UUID, count int) ([]string, error) {
	// Get existing categories to avoid similar colors
	existingCategories, err := s.repo.List(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to list categories for color recommendations", zap.Error(err))
		return nil, fmt.Errorf("failed to get color recommendations")
	}

	// Extract existing colors
	existingColors := []string{}
	for _, cat := range existingCategories {
		if cat.Color != nil {
			existingColors = append(existingColors, *cat.Color)
		}
	}

	// Generate recommendations
	recommendations := color.GenerateRecommendedPalette(existingColors, count)

	s.logger.Info("Generated color recommendations",
		zap.String("user_id", userID.String()),
		zap.Int("count", len(recommendations)),
	)

	return recommendations, nil
}

// ColorPaletteInfo provides information about a color including accessibility
type ColorPaletteInfo struct {
	Hex                string  `json:"hex"`
	ContrastWithWhite  float64 `json:"contrast_with_white"`
	ContrastWithBlack  float64 `json:"contrast_with_black"`
	RecommendedText    string  `json:"recommended_text_color"`
	MeetsWCAGAA        bool    `json:"meets_wcag_aa"`
	MeetsWCAGAAA       bool    `json:"meets_wcag_aaa"`
}

// GetColorInfo returns detailed information about a color including WCAG compliance
func (s *Service) GetColorInfo(colorStr string) (*ColorPaletteInfo, error) {
	parsedColor, err := color.ValidateColor(colorStr)
	if err != nil {
		return nil, fmt.Errorf("invalid color format")
	}

	white, _ := color.ParseHexColor("#FFFFFF")
	black, _ := color.ParseHexColor("#000000")

	contrastWhite := color.CalculateContrastRatio(parsedColor, white)
	contrastBlack := color.CalculateContrastRatio(parsedColor, black)
	recommendedText := color.GetContrastingTextColor(parsedColor)
	meetsAA := color.MeetsWCAGAA(parsedColor, white, false)
	meetsAAA := color.MeetsWCAGAAA(parsedColor, white, false)

	return &ColorPaletteInfo{
		Hex:               parsedColor.Hex,
		ContrastWithWhite: contrastWhite,
		ContrastWithBlack: contrastBlack,
		RecommendedText:   recommendedText,
		MeetsWCAGAA:       meetsAA,
		MeetsWCAGAAA:      meetsAAA,
	}, nil
}
