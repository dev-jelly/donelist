package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/dev-jelly/donelist/internal/api/middleware"
	"github.com/dev-jelly/donelist/internal/search"
	"go.uber.org/zap"
)

// SearchHandler handles search-related endpoints
type SearchHandler struct {
	searchService *search.Service
	logger        *zap.Logger
}

// NewSearchHandler creates a new search handler
func NewSearchHandler(searchService *search.Service, logger *zap.Logger) *SearchHandler {
	return &SearchHandler{
		searchService: searchService,
		logger:        logger,
	}
}

// SearchRequest represents the search request body
type SearchRequest struct {
	Query         string      `json:"query"`
	StartDate     *time.Time  `json:"start_date,omitempty"`
	EndDate       *time.Time  `json:"end_date,omitempty"`
	CategoryIDs   []uuid.UUID `json:"category_ids,omitempty"`
	TagIDs        []uuid.UUID `json:"tag_ids,omitempty"`
	TagNames      []string    `json:"tag_names,omitempty"`
	MinDuration   *int        `json:"min_duration,omitempty"`
	MaxDuration   *int        `json:"max_duration,omitempty"`
	IsEdited      *bool       `json:"is_edited,omitempty"`
	SortBy        string      `json:"sort_by,omitempty"`
	SortDirection string      `json:"sort_direction,omitempty"`
	Limit         int         `json:"limit,omitempty"`
	Offset        int         `json:"offset,omitempty"`
}

// Search performs a search with filters
// POST /api/v1/search
func (h *SearchHandler) Search(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req SearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Invalid search request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Convert request to filters
	filters := search.SearchFilters{
		Query:         req.Query,
		StartDate:     req.StartDate,
		EndDate:       req.EndDate,
		CategoryIDs:   req.CategoryIDs,
		TagIDs:        req.TagIDs,
		TagNames:      req.TagNames,
		MinDuration:   req.MinDuration,
		MaxDuration:   req.MaxDuration,
		IsEdited:      req.IsEdited,
		SortBy:        req.SortBy,
		SortDirection: req.SortDirection,
		Limit:         req.Limit,
		Offset:        req.Offset,
	}

	// Perform search
	response, err := h.searchService.Search(c.Request.Context(), userID, filters)
	if err != nil {
		h.logger.Error("Search failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "search failed"})
		return
	}

	c.JSON(http.StatusOK, response)
}

// GetSuggestions returns search suggestions
// GET /api/v1/search/suggestions?q=prefix&limit=10
func (h *SearchHandler) GetSuggestions(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	prefix := c.Query("q")
	if prefix == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter 'q' is required"})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	suggestions, err := h.searchService.GetSuggestions(c.Request.Context(), userID, prefix, limit)
	if err != nil {
		h.logger.Error("Failed to get suggestions", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get suggestions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"suggestions": suggestions})
}

// GetHistory returns recent search history
// GET /api/v1/search/history?limit=20
func (h *SearchHandler) GetHistory(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	history, err := h.searchService.GetSearchHistory(c.Request.Context(), userID, limit)
	if err != nil {
		h.logger.Error("Failed to get search history", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get search history"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"history": history})
}

// CreateSavedSearchRequest represents the request to create a saved search
type CreateSavedSearchRequest struct {
	Name        string                 `json:"name" binding:"required,min=1,max=100"`
	Description *string                `json:"description,omitempty"`
	Query       *string                `json:"query,omitempty"`
	Filters     map[string]interface{} `json:"filters,omitempty"`
	IsFavorite  bool                   `json:"is_favorite"`
}

// CreateSavedSearch creates a new saved search (Premium feature)
// POST /api/v1/search/saved
func (h *SearchHandler) CreateSavedSearch(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req CreateSavedSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Invalid create saved search request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	input := search.CreateSavedSearchInput{
		UserID:      userID,
		Name:        req.Name,
		Description: req.Description,
		Query:       req.Query,
		Filters:     req.Filters,
		IsFavorite:  req.IsFavorite,
	}

	savedSearch, err := h.searchService.CreateSavedSearch(c.Request.Context(), userID, input)
	if err != nil {
		if err.Error() == "premium subscription required for saved searches" {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		h.logger.Error("Failed to create saved search", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, savedSearch)
}

// ListSavedSearches lists all saved searches for the user
// GET /api/v1/search/saved
func (h *SearchHandler) ListSavedSearches(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	searches, err := h.searchService.ListSavedSearches(c.Request.Context(), userID)
	if err != nil {
		if err.Error() == "premium subscription required for saved searches" {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		h.logger.Error("Failed to list saved searches", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list saved searches"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"saved_searches": searches})
}

// GetSavedSearch retrieves a saved search by ID
// GET /api/v1/search/saved/:id
func (h *SearchHandler) GetSavedSearch(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	searchID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid search id"})
		return
	}

	savedSearch, err := h.searchService.GetSavedSearch(c.Request.Context(), userID, searchID)
	if err != nil {
		if err.Error() == "premium subscription required for saved searches" {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "saved search not found"})
		return
	}

	c.JSON(http.StatusOK, savedSearch)
}

// UpdateSavedSearchRequest represents the request to update a saved search
type UpdateSavedSearchRequest struct {
	Name        *string                `json:"name,omitempty"`
	Description *string                `json:"description,omitempty"`
	Query       *string                `json:"query,omitempty"`
	Filters     map[string]interface{} `json:"filters,omitempty"`
	IsFavorite  *bool                  `json:"is_favorite,omitempty"`
}

// UpdateSavedSearch updates a saved search
// PATCH /api/v1/search/saved/:id
func (h *SearchHandler) UpdateSavedSearch(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	searchID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid search id"})
		return
	}

	var req UpdateSavedSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Invalid update saved search request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	input := search.UpdateSavedSearchInput{
		Name:        req.Name,
		Description: req.Description,
		Query:       req.Query,
		Filters:     req.Filters,
		IsFavorite:  req.IsFavorite,
	}

	savedSearch, err := h.searchService.UpdateSavedSearch(c.Request.Context(), userID, searchID, input)
	if err != nil {
		if err.Error() == "premium subscription required for saved searches" {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		h.logger.Error("Failed to update saved search", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, savedSearch)
}

// DeleteSavedSearch deletes a saved search
// DELETE /api/v1/search/saved/:id
func (h *SearchHandler) DeleteSavedSearch(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	searchID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid search id"})
		return
	}

	if err := h.searchService.DeleteSavedSearch(c.Request.Context(), userID, searchID); err != nil {
		if err.Error() == "premium subscription required for saved searches" {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		h.logger.Error("Failed to delete saved search", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete saved search"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "saved search deleted successfully"})
}

// ExecuteSavedSearch executes a saved search and returns results
// POST /api/v1/search/saved/:id/execute
func (h *SearchHandler) ExecuteSavedSearch(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	searchID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid search id"})
		return
	}

	response, err := h.searchService.ExecuteSavedSearch(c.Request.Context(), userID, searchID)
	if err != nil {
		if err.Error() == "premium subscription required for saved searches" {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		h.logger.Error("Failed to execute saved search", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to execute saved search"})
		return
	}

	c.JSON(http.StatusOK, response)
}
