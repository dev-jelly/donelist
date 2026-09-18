package search

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/dev-jelly/donelist/internal/premium"
	"github.com/dev-jelly/donelist/internal/user"
	"go.uber.org/zap"
)

// Service handles search business logic
type Service struct {
	repo     *Repository
	userRepo *user.Repository
	cache    *CacheService
	metrics  *Metrics
	logger   *zap.Logger
}

// NewService creates a new search service
func NewService(repo *Repository, userRepo *user.Repository, logger *zap.Logger) *Service {
	return &Service{
		repo:     repo,
		userRepo: userRepo,
		metrics:  NewMetrics(),
		logger:   logger,
	}
}

// SetCache sets the cache service (optional)
func (s *Service) SetCache(cache *CacheService) {
	s.cache = cache
}

// Search performs a search with filters and returns results
func (s *Service) Search(ctx context.Context, userID uuid.UUID, filters SearchFilters) (*SearchResponse, error) {
	start := time.Now()
	cached := false

	// Try cache first if available
	if s.cache != nil {
		cachedResponse, err := s.cache.GetSearchResults(ctx, userID, filters)
		if err == nil && cachedResponse != nil {
			cached = true
			duration := time.Since(start)
			s.metrics.RecordSearch(duration, len(cachedResponse.Results), true, nil)
			return cachedResponse, nil
		}
		if err != nil {
			s.logger.Warn("Cache error, falling back to database", zap.Error(err))
			s.metrics.RecordCacheError()
		}
	}

	// Perform search
	results, total, err := s.repo.Search(ctx, userID, filters)
	if err != nil {
		duration := time.Since(start)
		s.metrics.RecordSearch(duration, 0, false, err)
		s.logger.Error("Search failed", zap.Error(err), zap.String("user_id", userID.String()))
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Get facets if requested (and results exist)
	var facets *SearchFacets
	if total > 0 {
		// Try cache for facets
		if s.cache != nil {
			cachedFacets, _ := s.cache.GetFacets(ctx, userID, filters)
			if cachedFacets != nil {
				facets = cachedFacets
			}
		}

		// Fetch facets if not cached
		if facets == nil {
			facetStart := time.Now()
			facets, err = s.repo.GetFacets(ctx, userID, filters)
			if err != nil {
				s.logger.Warn("Failed to get facets", zap.Error(err))
				facets = nil
			} else {
				s.metrics.RecordFacetComputation(time.Since(facetStart))
				// Cache facets
				if s.cache != nil {
					_ = s.cache.SetFacets(ctx, userID, filters, facets)
				}
			}
		}
	}

	// Save to search history (async, don't block response)
	if filters.Query != "" {
		go func() {
			historyCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			filtersMap := s.filtersToMap(filters)
			if err := s.repo.SaveSearchHistory(historyCtx, userID, filters.Query, filtersMap, int(total)); err != nil {
				s.logger.Warn("Failed to save search history", zap.Error(err))
			}
		}()
	}

	duration := time.Since(start)
	took := duration.Milliseconds()

	// Record metrics
	s.metrics.RecordSearch(duration, len(results), cached, nil)

	// Calculate query complexity
	complexity := s.calculateQueryComplexity(filters)
	hasFilters := s.hasFilters(filters)
	s.metrics.RecordQueryComplexity(filters.Query != "", hasFilters, complexity)

	s.logger.Info("Search completed",
		zap.String("user_id", userID.String()),
		zap.String("query", filters.Query),
		zap.Int64("total", total),
		zap.Int64("took_ms", took),
		zap.Bool("cached", cached),
	)

	response := &SearchResponse{
		Results: results,
		Total:   total,
		Limit:   filters.Limit,
		Offset:  filters.Offset,
		Facets:  facets,
		TookMs:  took,
	}

	// Cache the response
	if s.cache != nil && !cached {
		if err := s.cache.SetSearchResults(ctx, userID, filters, response); err != nil {
			s.logger.Warn("Failed to cache search results", zap.Error(err))
		}
	}

	return response, nil
}

// calculateQueryComplexity estimates query complexity for metrics
func (s *Service) calculateQueryComplexity(filters SearchFilters) int {
	complexity := 0

	if filters.Query != "" {
		complexity += 2
	}
	if filters.StartDate != nil || filters.EndDate != nil {
		complexity += 1
	}
	if len(filters.CategoryIDs) > 0 {
		complexity += 1
	}
	if len(filters.TagIDs) > 0 || len(filters.TagNames) > 0 {
		complexity += 2
	}
	if filters.MinDuration != nil || filters.MaxDuration != nil {
		complexity += 1
	}
	if filters.IsEdited != nil {
		complexity += 1
	}

	return complexity
}

// hasFilters checks if any filters are applied
func (s *Service) hasFilters(filters SearchFilters) bool {
	return filters.StartDate != nil ||
		filters.EndDate != nil ||
		len(filters.CategoryIDs) > 0 ||
		len(filters.TagIDs) > 0 ||
		len(filters.TagNames) > 0 ||
		filters.MinDuration != nil ||
		filters.MaxDuration != nil ||
		filters.IsEdited != nil
}

// GetSuggestions returns search suggestions
func (s *Service) GetSuggestions(ctx context.Context, userID uuid.UUID, prefix string, limit int) ([]SuggestionResult, error) {
	if prefix == "" {
		return []SuggestionResult{}, nil
	}

	suggestions, err := s.repo.GetSuggestions(ctx, userID, prefix, limit)
	if err != nil {
		s.logger.Error("Failed to get suggestions", zap.Error(err))
		return nil, fmt.Errorf("failed to get suggestions: %w", err)
	}

	return suggestions, nil
}

// GetSearchHistory retrieves recent search history
func (s *Service) GetSearchHistory(ctx context.Context, userID uuid.UUID, limit int) ([]SearchHistory, error) {
	history, err := s.repo.GetSearchHistory(ctx, userID, limit)
	if err != nil {
		s.logger.Error("Failed to get search history", zap.Error(err))
		return nil, fmt.Errorf("failed to get search history: %w", err)
	}

	return history, nil
}

// CreateSavedSearch creates a new saved search (Premium feature)
func (s *Service) CreateSavedSearch(ctx context.Context, userID uuid.UUID, input CreateSavedSearchInput) (*SavedSearch, error) {
	// Check if user has premium access
	if err := s.checkPremiumAccess(ctx, userID); err != nil {
		return nil, err
	}

	// Validate input
	if input.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	// Set user ID
	input.UserID = userID

	// Create saved search
	savedSearch, err := s.repo.CreateSavedSearch(ctx, input)
	if err != nil {
		s.logger.Error("Failed to create saved search", zap.Error(err))
		return nil, fmt.Errorf("failed to create saved search: %w", err)
	}

	s.logger.Info("Saved search created",
		zap.String("user_id", userID.String()),
		zap.String("search_id", savedSearch.ID.String()),
		zap.String("name", savedSearch.Name),
	)

	return savedSearch, nil
}

// GetSavedSearch retrieves a saved search by ID
func (s *Service) GetSavedSearch(ctx context.Context, userID, searchID uuid.UUID) (*SavedSearch, error) {
	// Check premium access
	if err := s.checkPremiumAccess(ctx, userID); err != nil {
		return nil, err
	}

	savedSearch, err := s.repo.GetSavedSearch(ctx, searchID, userID)
	if err != nil {
		return nil, err
	}

	// Increment usage counter
	if err := s.repo.IncrementSavedSearchUsage(ctx, searchID, userID); err != nil {
		s.logger.Warn("Failed to increment usage", zap.Error(err))
		// Don't fail the request if usage increment fails
	}

	return savedSearch, nil
}

// ListSavedSearches lists all saved searches for a user
func (s *Service) ListSavedSearches(ctx context.Context, userID uuid.UUID) ([]SavedSearch, error) {
	// Check premium access
	if err := s.checkPremiumAccess(ctx, userID); err != nil {
		return nil, err
	}

	searches, err := s.repo.ListSavedSearches(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to list saved searches", zap.Error(err))
		return nil, fmt.Errorf("failed to list saved searches: %w", err)
	}

	return searches, nil
}

// UpdateSavedSearch updates a saved search
func (s *Service) UpdateSavedSearch(ctx context.Context, userID, searchID uuid.UUID, input UpdateSavedSearchInput) (*SavedSearch, error) {
	// Check premium access
	if err := s.checkPremiumAccess(ctx, userID); err != nil {
		return nil, err
	}

	savedSearch, err := s.repo.UpdateSavedSearch(ctx, searchID, userID, input)
	if err != nil {
		s.logger.Error("Failed to update saved search", zap.Error(err))
		return nil, fmt.Errorf("failed to update saved search: %w", err)
	}

	s.logger.Info("Saved search updated",
		zap.String("user_id", userID.String()),
		zap.String("search_id", searchID.String()),
	)

	return savedSearch, nil
}

// DeleteSavedSearch deletes a saved search
func (s *Service) DeleteSavedSearch(ctx context.Context, userID, searchID uuid.UUID) error {
	// Check premium access
	if err := s.checkPremiumAccess(ctx, userID); err != nil {
		return err
	}

	if err := s.repo.DeleteSavedSearch(ctx, searchID, userID); err != nil {
		s.logger.Error("Failed to delete saved search", zap.Error(err))
		return fmt.Errorf("failed to delete saved search: %w", err)
	}

	s.logger.Info("Saved search deleted",
		zap.String("user_id", userID.String()),
		zap.String("search_id", searchID.String()),
	)

	return nil
}

// ExecuteSavedSearch executes a saved search and returns results
func (s *Service) ExecuteSavedSearch(ctx context.Context, userID, searchID uuid.UUID) (*SearchResponse, error) {
	// Get the saved search
	savedSearch, err := s.GetSavedSearch(ctx, userID, searchID)
	if err != nil {
		return nil, err
	}

	// Convert saved search to filters
	filters := s.mapToFilters(savedSearch.Filters)
	if savedSearch.Query != nil {
		filters.Query = *savedSearch.Query
	}

	// Execute search
	return s.Search(ctx, userID, filters)
}

// checkPremiumAccess checks if user has premium or enterprise tier
func (s *Service) checkPremiumAccess(ctx context.Context, userID uuid.UUID) error {
	u, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	userTier := premium.Tier(u.Tier)
	if userTier != premium.TierPremium && userTier != premium.TierEnterprise {
		return fmt.Errorf("premium subscription required for saved searches")
	}

	return nil
}

// filtersToMap converts SearchFilters to a map for storage
func (s *Service) filtersToMap(filters SearchFilters) map[string]interface{} {
	m := make(map[string]interface{})

	if filters.Query != "" {
		m["query"] = filters.Query
	}
	if filters.StartDate != nil {
		m["start_date"] = filters.StartDate
	}
	if filters.EndDate != nil {
		m["end_date"] = filters.EndDate
	}
	if len(filters.CategoryIDs) > 0 {
		m["category_ids"] = filters.CategoryIDs
	}
	if len(filters.TagIDs) > 0 {
		m["tag_ids"] = filters.TagIDs
	}
	if len(filters.TagNames) > 0 {
		m["tag_names"] = filters.TagNames
	}
	if filters.MinDuration != nil {
		m["min_duration"] = *filters.MinDuration
	}
	if filters.MaxDuration != nil {
		m["max_duration"] = *filters.MaxDuration
	}
	if filters.IsEdited != nil {
		m["is_edited"] = *filters.IsEdited
	}
	if filters.SortBy != "" {
		m["sort_by"] = filters.SortBy
	}
	if filters.SortDirection != "" {
		m["sort_direction"] = filters.SortDirection
	}

	return m
}

// mapToFilters converts a map to SearchFilters
func (s *Service) mapToFilters(m map[string]interface{}) SearchFilters {
	filters := SearchFilters{
		Limit:  20,
		Offset: 0,
	}

	if q, ok := m["query"].(string); ok {
		filters.Query = q
	}
	if startDate, ok := m["start_date"].(time.Time); ok {
		filters.StartDate = &startDate
	}
	if endDate, ok := m["end_date"].(time.Time); ok {
		filters.EndDate = &endDate
	}
	if categoryIDs, ok := m["category_ids"].([]uuid.UUID); ok {
		filters.CategoryIDs = categoryIDs
	}
	if tagIDs, ok := m["tag_ids"].([]uuid.UUID); ok {
		filters.TagIDs = tagIDs
	}
	if tagNames, ok := m["tag_names"].([]string); ok {
		filters.TagNames = tagNames
	}
	if minDuration, ok := m["min_duration"].(int); ok {
		filters.MinDuration = &minDuration
	}
	if maxDuration, ok := m["max_duration"].(int); ok {
		filters.MaxDuration = &maxDuration
	}
	if isEdited, ok := m["is_edited"].(bool); ok {
		filters.IsEdited = &isEdited
	}
	if sortBy, ok := m["sort_by"].(string); ok {
		filters.SortBy = sortBy
	}
	if sortDirection, ok := m["sort_direction"].(string); ok {
		filters.SortDirection = sortDirection
	}

	return filters
}
