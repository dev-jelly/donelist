package search

import (
	"time"

	"github.com/google/uuid"
)

// SearchTemplate represents a pre-configured search template
type SearchTemplate struct {
	Name        string
	Description string
	Filters     SearchFilters
}

// Common search templates for quick filtering
var (
	// TodayTemplate searches for today's checkins
	TodayTemplate = SearchTemplate{
		Name:        "Today",
		Description: "All check-ins from today",
		Filters: SearchFilters{
			StartDate: timePtr(startOfDay(time.Now())),
			EndDate:   timePtr(endOfDay(time.Now())),
			SortBy:    "checkin_time",
			Limit:     20,
		},
	}

	// ThisWeekTemplate searches for this week's checkins
	ThisWeekTemplate = SearchTemplate{
		Name:        "This Week",
		Description: "All check-ins from this week",
		Filters: SearchFilters{
			StartDate: timePtr(startOfWeek(time.Now())),
			EndDate:   timePtr(time.Now()),
			SortBy:    "checkin_time",
			Limit:     50,
		},
	}

	// ThisMonthTemplate searches for this month's checkins
	ThisMonthTemplate = SearchTemplate{
		Name:        "This Month",
		Description: "All check-ins from this month",
		Filters: SearchFilters{
			StartDate: timePtr(startOfMonth(time.Now())),
			EndDate:   timePtr(time.Now()),
			SortBy:    "checkin_time",
			Limit:     100,
		},
	}

	// EditedCheckinsTemplate searches for edited checkins
	EditedCheckinsTemplate = SearchTemplate{
		Name:        "Edited Check-ins",
		Description: "All check-ins that have been edited",
		Filters: SearchFilters{
			IsEdited: boolPtr(true),
			SortBy:   "updated_at",
			Limit:    20,
		},
	}

	// LongDurationTemplate searches for long duration checkins
	LongDurationTemplate = SearchTemplate{
		Name:        "Long Sessions",
		Description: "Check-ins with duration >= 60 minutes",
		Filters: SearchFilters{
			MinDuration: intPtr(60),
			SortBy:      "duration_minutes",
			SortDirection: "DESC",
			Limit:       20,
		},
	}

	// ShortDurationTemplate searches for short duration checkins
	ShortDurationTemplate = SearchTemplate{
		Name:        "Quick Tasks",
		Description: "Check-ins with duration <= 15 minutes",
		Filters: SearchFilters{
			MaxDuration: intPtr(15),
			SortBy:      "checkin_time",
			Limit:       20,
		},
	}

	// RecentTemplate searches for recent checkins
	RecentTemplate = SearchTemplate{
		Name:        "Recent",
		Description: "Most recently created check-ins",
		Filters: SearchFilters{
			SortBy:        "created_at",
			SortDirection: "DESC",
			Limit:         20,
		},
	}
)

// GetAllTemplates returns all available search templates
func GetAllTemplates() []SearchTemplate {
	return []SearchTemplate{
		TodayTemplate,
		ThisWeekTemplate,
		ThisMonthTemplate,
		EditedCheckinsTemplate,
		LongDurationTemplate,
		ShortDurationTemplate,
		RecentTemplate,
	}
}

// GetTemplateByName returns a search template by name
func GetTemplateByName(name string) *SearchTemplate {
	templates := GetAllTemplates()
	for _, template := range templates {
		if template.Name == name {
			return &template
		}
	}
	return nil
}

// ApplyTemplate applies a template to create search filters
func ApplyTemplate(templateName string, overrides *SearchFilters) SearchFilters {
	template := GetTemplateByName(templateName)
	if template == nil {
		// Return default filters if template not found
		return SearchFilters{
			Limit: 20,
		}
	}

	filters := template.Filters

	// Apply overrides if provided
	if overrides != nil {
		if overrides.Query != "" {
			filters.Query = overrides.Query
		}
		if overrides.StartDate != nil {
			filters.StartDate = overrides.StartDate
		}
		if overrides.EndDate != nil {
			filters.EndDate = overrides.EndDate
		}
		if len(overrides.CategoryIDs) > 0 {
			filters.CategoryIDs = overrides.CategoryIDs
		}
		if len(overrides.TagIDs) > 0 {
			filters.TagIDs = overrides.TagIDs
		}
		if len(overrides.TagNames) > 0 {
			filters.TagNames = overrides.TagNames
		}
		if overrides.MinDuration != nil {
			filters.MinDuration = overrides.MinDuration
		}
		if overrides.MaxDuration != nil {
			filters.MaxDuration = overrides.MaxDuration
		}
		if overrides.IsEdited != nil {
			filters.IsEdited = overrides.IsEdited
		}
		if overrides.SortBy != "" {
			filters.SortBy = overrides.SortBy
		}
		if overrides.SortDirection != "" {
			filters.SortDirection = overrides.SortDirection
		}
		if overrides.Limit > 0 {
			filters.Limit = overrides.Limit
		}
		if overrides.Offset > 0 {
			filters.Offset = overrides.Offset
		}
	}

	return filters
}

// ValidateFilters validates and sanitizes search filters
func ValidateFilters(filters *SearchFilters) error {
	// Validate limit
	if filters.Limit <= 0 {
		filters.Limit = 20
	}
	if filters.Limit > 100 {
		filters.Limit = 100
	}

	// Validate offset
	if filters.Offset < 0 {
		filters.Offset = 0
	}

	// Validate date range
	if filters.StartDate != nil && filters.EndDate != nil {
		if filters.EndDate.Before(*filters.StartDate) {
			// Swap them
			filters.StartDate, filters.EndDate = filters.EndDate, filters.StartDate
		}
	}

	// Validate duration range
	if filters.MinDuration != nil && filters.MaxDuration != nil {
		if *filters.MaxDuration < *filters.MinDuration {
			// Swap them
			filters.MinDuration, filters.MaxDuration = filters.MaxDuration, filters.MinDuration
		}
	}

	// Validate sort field
	validSortFields := map[string]bool{
		"checkin_time":     true,
		"created_at":       true,
		"updated_at":       true,
		"duration_minutes": true,
		"edit_count":       true,
		"relevance":        true,
		"":                 true, // Empty is valid (will use default)
	}

	if !validSortFields[filters.SortBy] {
		filters.SortBy = "checkin_time"
	}

	// Validate sort direction
	if filters.SortDirection != "ASC" && filters.SortDirection != "DESC" && filters.SortDirection != "" {
		filters.SortDirection = "DESC"
	}

	return nil
}

// Helper functions

func timePtr(t time.Time) *time.Time {
	return &t
}

func boolPtr(b bool) *bool {
	return &b
}

func intPtr(i int) *int {
	return &i
}

func startOfDay(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
}

func endOfDay(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 23, 59, 59, 999999999, t.Location())
}

func startOfWeek(t time.Time) time.Time {
	// Go to the most recent Monday
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7 // Sunday
	}
	daysBack := weekday - 1
	return startOfDay(t.AddDate(0, 0, -daysBack))
}

func startOfMonth(t time.Time) time.Time {
	year, month, _ := t.Date()
	return time.Date(year, month, 1, 0, 0, 0, 0, t.Location())
}

// FilterBuilder provides a fluent interface for building search filters
type FilterBuilder struct {
	filters SearchFilters
}

// NewFilterBuilder creates a new filter builder
func NewFilterBuilder() *FilterBuilder {
	return &FilterBuilder{
		filters: SearchFilters{
			Limit: 20,
		},
	}
}

// WithQuery sets the full-text search query
func (fb *FilterBuilder) WithQuery(query string) *FilterBuilder {
	fb.filters.Query = query
	return fb
}

// WithDateRange sets the date range filter
func (fb *FilterBuilder) WithDateRange(start, end time.Time) *FilterBuilder {
	fb.filters.StartDate = &start
	fb.filters.EndDate = &end
	return fb
}

// WithCategories sets the category filter
func (fb *FilterBuilder) WithCategories(categoryIDs ...uuid.UUID) *FilterBuilder {
	fb.filters.CategoryIDs = categoryIDs
	return fb
}

// WithTags sets the tag filter
func (fb *FilterBuilder) WithTags(tagIDs ...uuid.UUID) *FilterBuilder {
	fb.filters.TagIDs = tagIDs
	return fb
}

// WithTagNames sets the tag name filter
func (fb *FilterBuilder) WithTagNames(tagNames ...string) *FilterBuilder {
	fb.filters.TagNames = tagNames
	return fb
}

// WithDurationRange sets the duration range filter
func (fb *FilterBuilder) WithDurationRange(min, max int) *FilterBuilder {
	fb.filters.MinDuration = &min
	fb.filters.MaxDuration = &max
	return fb
}

// WithEdited sets the edited filter
func (fb *FilterBuilder) WithEdited(edited bool) *FilterBuilder {
	fb.filters.IsEdited = &edited
	return fb
}

// WithSort sets the sort field and direction
func (fb *FilterBuilder) WithSort(field, direction string) *FilterBuilder {
	fb.filters.SortBy = field
	fb.filters.SortDirection = direction
	return fb
}

// WithPagination sets pagination parameters
func (fb *FilterBuilder) WithPagination(limit, offset int) *FilterBuilder {
	fb.filters.Limit = limit
	fb.filters.Offset = offset
	return fb
}

// Build returns the constructed filters
func (fb *FilterBuilder) Build() SearchFilters {
	ValidateFilters(&fb.filters)
	return fb.filters
}
