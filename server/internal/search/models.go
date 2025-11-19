package search

import (
	"time"

	"github.com/google/uuid"
)

// SearchFilters contains all available search filter options
type SearchFilters struct {
	Query          string       `json:"query,omitempty"`           // Full-text search query
	StartDate      *time.Time   `json:"start_date,omitempty"`      // Filter by checkin_time >= start_date
	EndDate        *time.Time   `json:"end_date,omitempty"`        // Filter by checkin_time <= end_date
	CategoryIDs    []uuid.UUID  `json:"category_ids,omitempty"`    // Filter by category IDs
	TagIDs         []uuid.UUID  `json:"tag_ids,omitempty"`         // Filter by tag IDs (AND logic)
	TagNames       []string     `json:"tag_names,omitempty"`       // Filter by tag names (AND logic)
	MinDuration    *int         `json:"min_duration,omitempty"`    // Filter by duration >= min
	MaxDuration    *int         `json:"max_duration,omitempty"`    // Filter by duration <= max
	IsEdited       *bool        `json:"is_edited,omitempty"`       // Filter by edit status
	SortBy         string       `json:"sort_by,omitempty"`         // Sort field (default: checkin_time)
	SortDirection  string       `json:"sort_direction,omitempty"`  // Sort direction: asc or desc (default: desc)
	Limit          int          `json:"limit"`                     // Page size (default: 20, max: 100)
	Offset         int          `json:"offset"`                    // Pagination offset
}

// SearchResult represents a single search result item
type SearchResult struct {
	ID              uuid.UUID  `db:"id" json:"id"`
	UserID          uuid.UUID  `db:"user_id" json:"user_id"`
	CategoryID      *uuid.UUID `db:"category_id" json:"category_id,omitempty"`
	CategoryName    *string    `db:"category_name" json:"category_name,omitempty"`
	Content         string     `db:"content" json:"content"`
	CheckinTime     time.Time  `db:"checkin_time" json:"checkin_time"`
	DurationMinutes int        `db:"duration_minutes" json:"duration_minutes"`
	IsEdited        bool       `db:"is_edited" json:"is_edited"`
	EditCount       int        `db:"edit_count" json:"edit_count"`
	CreatedAt       time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time  `db:"updated_at" json:"updated_at"`
	Rank            *float32   `db:"rank" json:"rank,omitempty"`    // Full-text search relevance rank
	Snippet         *string    `db:"snippet" json:"snippet,omitempty"` // Text snippet with highlights
	Tags            []string   `json:"tags,omitempty"`              // Will be populated separately
}

// SearchResponse represents the complete search response with metadata
type SearchResponse struct {
	Results    []SearchResult      `json:"results"`
	Total      int64               `json:"total"`
	Limit      int                 `json:"limit"`
	Offset     int                 `json:"offset"`
	Facets     *SearchFacets       `json:"facets,omitempty"`
	TookMs     int64               `json:"took_ms"`
}

// SearchFacets contains aggregated data for faceted search
type SearchFacets struct {
	Categories []CategoryFacet `json:"categories,omitempty"`
	Tags       []TagFacet      `json:"tags,omitempty"`
	Durations  []DurationFacet `json:"durations,omitempty"`
	DateRanges []DateRangeFacet `json:"date_ranges,omitempty"`
}

// CategoryFacet represents a category facet with count
type CategoryFacet struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Color string    `json:"color"`
	Count int       `json:"count"`
}

// TagFacet represents a tag facet with count
type TagFacet struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Count int       `json:"count"`
}

// DurationFacet represents a duration bucket with count
type DurationFacet struct {
	Duration int `json:"duration"`
	Count    int `json:"count"`
}

// DateRangeFacet represents a date range bucket with count
type DateRangeFacet struct {
	Label string `json:"label"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	Count int `json:"count"`
}

// SearchHistory represents a search history entry
type SearchHistory struct {
	ID          uuid.UUID       `db:"id" json:"id"`
	UserID      uuid.UUID       `db:"user_id" json:"user_id"`
	Query       string          `db:"query" json:"query"`
	Filters     map[string]interface{} `db:"filters" json:"filters"`
	ResultCount int             `db:"result_count" json:"result_count"`
	CreatedAt   time.Time       `db:"created_at" json:"created_at"`
}

// SavedSearch represents a saved search configuration
type SavedSearch struct {
	ID          uuid.UUID       `db:"id" json:"id"`
	UserID      uuid.UUID       `db:"user_id" json:"user_id"`
	Name        string          `db:"name" json:"name"`
	Description *string         `db:"description" json:"description,omitempty"`
	Query       *string         `db:"query" json:"query,omitempty"`
	Filters     map[string]interface{} `db:"filters" json:"filters"`
	IsFavorite  bool            `db:"is_favorite" json:"is_favorite"`
	LastUsedAt  *time.Time      `db:"last_used_at" json:"last_used_at,omitempty"`
	UseCount    int             `db:"use_count" json:"use_count"`
	CreatedAt   time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time       `db:"updated_at" json:"updated_at"`
}

// CreateSavedSearchInput represents input for creating a saved search
type CreateSavedSearchInput struct {
	UserID      uuid.UUID
	Name        string
	Description *string
	Query       *string
	Filters     map[string]interface{}
	IsFavorite  bool
}

// UpdateSavedSearchInput represents input for updating a saved search
type UpdateSavedSearchInput struct {
	Name        *string
	Description *string
	Query       *string
	Filters     map[string]interface{}
	IsFavorite  *bool
}

// SuggestionResult represents a search suggestion
type SuggestionResult struct {
	Type  string `json:"type"`  // "tag", "category", "content"
	Value string `json:"value"`
	Count int    `json:"count,omitempty"`
}
