package search

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// QueryBuilder helps build secure parameterized SQL queries for search
type QueryBuilder struct {
	baseQuery     string
	whereClauses  strings.Builder
	args          []interface{}
	argIndex      int
	joins         map[string]bool
}

// NewQueryBuilder creates a new query builder
func NewQueryBuilder(baseQuery string) *QueryBuilder {
	return &QueryBuilder{
		baseQuery: baseQuery,
		args:      make([]interface{}, 0),
		argIndex:  1,
		joins:     make(map[string]bool),
	}
}

// AddFullTextSearch adds PostgreSQL full-text search clause using tsvector
func (qb *QueryBuilder) AddFullTextSearch(query string) {
	if query == "" {
		return
	}

	// Sanitize the search query by converting to tsquery format
	// This prevents SQL injection by using parameterized queries
	tsQuery := sanitizeTsQuery(query)

	qb.addWhere(fmt.Sprintf("search_vector @@ to_tsquery('english', $%d)", qb.argIndex))
	qb.args = append(qb.args, tsQuery)
	qb.argIndex++
}

// AddUserFilter adds user_id filter
func (qb *QueryBuilder) AddUserFilter(userID uuid.UUID) {
	qb.addWhere(fmt.Sprintf("checkins.user_id = $%d", qb.argIndex))
	qb.args = append(qb.args, userID)
	qb.argIndex++
}

// AddDateRange adds date range filter
func (qb *QueryBuilder) AddDateRange(startDate, endDate *string) {
	if startDate != nil && *startDate != "" {
		qb.addWhere(fmt.Sprintf("checkins.checkin_time >= $%d", qb.argIndex))
		qb.args = append(qb.args, *startDate)
		qb.argIndex++
	}

	if endDate != nil && *endDate != "" {
		qb.addWhere(fmt.Sprintf("checkins.checkin_time <= $%d", qb.argIndex))
		qb.args = append(qb.args, *endDate)
		qb.argIndex++
	}
}

// AddCategoryFilter adds category filter
func (qb *QueryBuilder) AddCategoryFilter(categoryIDs []uuid.UUID) {
	if len(categoryIDs) == 0 {
		return
	}

	qb.addWhere(fmt.Sprintf("checkins.category_id = ANY($%d)", qb.argIndex))
	qb.args = append(qb.args, categoryIDs)
	qb.argIndex++
}

// AddTagFilter adds tag filter using junction table
func (qb *QueryBuilder) AddTagFilter(tagIDs []uuid.UUID) {
	if len(tagIDs) == 0 {
		return
	}

	// Use EXISTS clause for AND logic (checkin must have ALL specified tags)
	for i, tagID := range tagIDs {
		existsClause := fmt.Sprintf(
			"EXISTS (SELECT 1 FROM checkin_tags ct%d WHERE ct%d.checkin_id = checkins.id AND ct%d.tag_id = $%d)",
			i, i, i, qb.argIndex,
		)
		qb.addWhere(existsClause)
		qb.args = append(qb.args, tagID)
		qb.argIndex++
	}
}

// AddDurationFilter adds duration range filter
func (qb *QueryBuilder) AddDurationFilter(minDuration, maxDuration *int) {
	if minDuration != nil {
		qb.addWhere(fmt.Sprintf("checkins.duration_minutes >= $%d", qb.argIndex))
		qb.args = append(qb.args, *minDuration)
		qb.argIndex++
	}

	if maxDuration != nil {
		qb.addWhere(fmt.Sprintf("checkins.duration_minutes <= $%d", qb.argIndex))
		qb.args = append(qb.args, *maxDuration)
		qb.argIndex++
	}
}

// AddEditedFilter adds is_edited filter
func (qb *QueryBuilder) AddEditedFilter(isEdited *bool) {
	if isEdited == nil {
		return
	}

	qb.addWhere(fmt.Sprintf("checkins.is_edited = $%d", qb.argIndex))
	qb.args = append(qb.args, *isEdited)
	qb.argIndex++
}

// AddDeletedFilter adds deleted_at IS NULL filter
func (qb *QueryBuilder) AddDeletedFilter() {
	qb.addWhere("checkins.deleted_at IS NULL")
}

// addWhere adds a WHERE clause
func (qb *QueryBuilder) addWhere(clause string) {
	if qb.whereClauses.Len() > 0 {
		qb.whereClauses.WriteString(" AND ")
	}
	qb.whereClauses.WriteString(clause)
}

// Build builds the final SQL query with all clauses
func (qb *QueryBuilder) Build() (string, []interface{}) {
	query := qb.baseQuery

	// Add WHERE clause if we have conditions
	if qb.whereClauses.Len() > 0 {
		query += " WHERE " + qb.whereClauses.String()
	}

	return query, qb.args
}

// GetArgIndex returns the current argument index for additional clauses
func (qb *QueryBuilder) GetArgIndex() int {
	return qb.argIndex
}

// GetArgs returns the current arguments
func (qb *QueryBuilder) GetArgs() []interface{} {
	return qb.args
}

// AppendArg adds a new argument and returns its placeholder
func (qb *QueryBuilder) AppendArg(arg interface{}) string {
	placeholder := fmt.Sprintf("$%d", qb.argIndex)
	qb.args = append(qb.args, arg)
	qb.argIndex++
	return placeholder
}

// sanitizeTsQuery converts a user query to safe tsquery format
// This function ensures that special characters are properly escaped
// and that the query follows PostgreSQL tsquery syntax
func sanitizeTsQuery(query string) string {
	// Remove leading/trailing whitespace
	query = strings.TrimSpace(query)

	if query == "" {
		return ""
	}

	// Split the query into words
	words := strings.Fields(query)

	// Escape special characters and join with & (AND operator)
	sanitizedWords := make([]string, 0, len(words))
	for _, word := range words {
		// Remove special tsquery characters that could cause syntax errors
		word = strings.Map(func(r rune) rune {
			switch r {
			case '&', '|', '!', '(', ')', '<', '>', ':', '\'', '"':
				return -1 // Remove these characters
			default:
				return r
			}
		}, word)

		// Skip empty words after sanitization
		if len(word) > 0 {
			// Add prefix matching with :* for better UX
			sanitizedWords = append(sanitizedWords, word+":*")
		}
	}

	// Join with & for AND logic (all words must match)
	return strings.Join(sanitizedWords, " & ")
}

// ValidateSortField validates and returns a safe sort field
func ValidateSortField(field string) string {
	// Whitelist of allowed sort fields
	allowedFields := map[string]string{
		"checkin_time":     "checkins.checkin_time",
		"created_at":       "checkins.created_at",
		"updated_at":       "checkins.updated_at",
		"duration_minutes": "checkins.duration_minutes",
		"edit_count":       "checkins.edit_count",
		"relevance":        "rank", // For full-text search ranking
	}

	if safeField, ok := allowedFields[field]; ok {
		return safeField
	}

	// Default to checkin_time if invalid field provided
	return "checkins.checkin_time"
}

// ValidateSortDirection validates and returns a safe sort direction
func ValidateSortDirection(direction string) string {
	direction = strings.ToUpper(strings.TrimSpace(direction))
	if direction == "ASC" {
		return "ASC"
	}
	return "DESC" // Default to DESC
}

// ValidateLimit validates and returns a safe limit value
func ValidateLimit(limit int) int {
	if limit <= 0 {
		return 20 // Default
	}
	if limit > 100 {
		return 100 // Max
	}
	return limit
}

// ValidateOffset validates and returns a safe offset value
func ValidateOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}
