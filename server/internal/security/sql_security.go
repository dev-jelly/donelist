package security

import (
	"fmt"
	"regexp"
	"strings"
)

// SQLSecurityAuditor provides utilities for SQL injection prevention
type SQLSecurityAuditor struct{}

// NewSQLSecurityAuditor creates a new SQL security auditor
func NewSQLSecurityAuditor() *SQLSecurityAuditor {
	return &SQLSecurityAuditor{}
}

// ValidateQuery checks if a SQL query uses parameterized queries
// This is a static analysis helper to detect potential SQL injection vulnerabilities
func (a *SQLSecurityAuditor) ValidateQuery(query string) error {
	// Check for string concatenation patterns
	dangerousPatterns := []struct {
		pattern string
		message string
	}{
		{
			pattern: `\+\s*["']`,
			message: "String concatenation detected in SQL query",
		},
		{
			pattern: `fmt\.Sprintf.*["'].*%s.*["']`,
			message: "fmt.Sprintf with %s detected - use parameterized queries instead",
		},
		{
			pattern: `["']\s*\+\s*\w+`,
			message: "Variable concatenation detected in SQL query",
		},
	}

	for _, dp := range dangerousPatterns {
		matched, _ := regexp.MatchString(dp.pattern, query)
		if matched {
			return fmt.Errorf("potential SQL injection vulnerability: %s", dp.message)
		}
	}

	return nil
}

// IsSafeIdentifier checks if a database identifier (table/column name) is safe
// Only allows alphanumeric characters and underscores
func (a *SQLSecurityAuditor) IsSafeIdentifier(identifier string) bool {
	if identifier == "" {
		return false
	}

	// Must start with a letter or underscore
	if !regexp.MustCompile(`^[a-zA-Z_]`).MatchString(identifier) {
		return false
	}

	// Can only contain letters, numbers, and underscores
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9_]+$`, identifier)
	return matched
}

// QuoteIdentifier safely quotes a database identifier
// Use this when you need to dynamically build table or column names
func (a *SQLSecurityAuditor) QuoteIdentifier(identifier string) (string, error) {
	if !a.IsSafeIdentifier(identifier) {
		return "", fmt.Errorf("invalid identifier: %s", identifier)
	}

	// PostgreSQL uses double quotes for identifiers
	return fmt.Sprintf(`"%s"`, identifier), nil
}

// ValidateSortColumn validates a sort column against a whitelist
func ValidateSortColumn(column string, allowedColumns []string) error {
	for _, allowed := range allowedColumns {
		if column == allowed {
			return nil
		}
	}
	return fmt.Errorf("invalid sort column: %s", column)
}

// ValidateSortDirection validates sort direction (ASC/DESC)
func ValidateSortDirection(direction string) error {
	upper := strings.ToUpper(strings.TrimSpace(direction))
	if upper != "ASC" && upper != "DESC" {
		return fmt.Errorf("invalid sort direction: must be ASC or DESC")
	}
	return nil
}

// SafeOrderBy builds a safe ORDER BY clause using whitelisted columns
func SafeOrderBy(column string, direction string, allowedColumns map[string]string) (string, error) {
	// Validate direction first
	if err := ValidateSortDirection(direction); err != nil {
		return "", err
	}

	// Check if column is in whitelist
	safeColumn, ok := allowedColumns[column]
	if !ok {
		return "", fmt.Errorf("column not allowed for sorting: %s", column)
	}

	direction = strings.ToUpper(strings.TrimSpace(direction))
	return fmt.Sprintf("%s %s", safeColumn, direction), nil
}

// SQLInjectionPatterns returns a list of common SQL injection patterns to block
func SQLInjectionPatterns() []string {
	return []string{
		// SQL keywords
		"union", "select", "insert", "update", "delete", "drop", "create", "alter",
		"exec", "execute", "script", "javascript",

		// SQL operators and characters
		"--", "/*", "*/", ";--", "';", "\"", "'", "''",

		// SQL functions
		"xp_", "sp_", "waitfor", "delay",

		// Boolean logic - basic patterns
		"or 1=1", "or '1'='1'", "and 1=1", "or true", "and true",
		"or '1'='1", "and '1'='1", "or 1", "and 1",

		// Hex encoding
		"0x",

		// Comment injection
		"#", "--+", "/**/",
	}
}

// DetectSQLInjection checks if input contains SQL injection patterns
func DetectSQLInjection(input string) bool {
	lower := strings.ToLower(input)

	patterns := SQLInjectionPatterns()
	for _, pattern := range patterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}

	return false
}

// SanitizeLikePattern sanitizes a LIKE pattern to prevent wildcard injection
func SanitizeLikePattern(pattern string) string {
	// Escape special LIKE characters
	pattern = strings.ReplaceAll(pattern, "\\", "\\\\")
	pattern = strings.ReplaceAll(pattern, "%", "\\%")
	pattern = strings.ReplaceAll(pattern, "_", "\\_")
	return pattern
}

// BuildSafeLikeQuery builds a safe LIKE query with escaped wildcards
func BuildSafeLikeQuery(column string, pattern string, caseSensitive bool) (string, string) {
	sanitized := SanitizeLikePattern(pattern)
	likePattern := "%" + sanitized + "%"

	operator := "LIKE"
	if !caseSensitive {
		operator = "ILIKE"
	}

	return fmt.Sprintf("%s %s $", column, operator), likePattern
}

// ValidateLimit ensures limit is within safe bounds
func ValidateLimit(limit int, maxLimit int) int {
	if limit <= 0 {
		return 20 // Default
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

// ValidateOffset ensures offset is non-negative
func ValidateOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}

// Best practices documentation
const SQLSecurityBestPractices = `
SQL Injection Prevention Best Practices
========================================

1. ALWAYS use parameterized queries (prepared statements)
   - GOOD: db.Query("SELECT * FROM users WHERE id = $1", userID)
   - BAD:  db.Query(fmt.Sprintf("SELECT * FROM users WHERE id = '%s'", userID))

2. Never concatenate user input into SQL queries
   - Even if you sanitize the input, use parameterized queries

3. Use whitelisting for dynamic identifiers (table/column names)
   - Create a map of allowed values
   - Reject any input not in the whitelist

4. Validate and sanitize LIKE patterns
   - Escape %, _, and \ characters
   - Use SanitizeLikePattern() function

5. Use ORM or query builders that enforce parameterization
   - sqlx with named parameters
   - QueryBuilder with parameter tracking

6. Validate numeric inputs
   - Ensure IDs are valid UUIDs
   - Ensure pagination values are within bounds

7. Use least privilege for database users
   - Application should not use admin/root database user
   - Grant only necessary permissions

8. Enable query logging in development
   - Review queries for potential vulnerabilities
   - Use SQLSecurityAuditor.ValidateQuery() in tests

9. Use database-specific security features
   - PostgreSQL: Use SECURITY DEFINER functions carefully
   - Enable row-level security where appropriate

10. Regular security audits
    - Run OWASP ZAP or sqlmap tests
    - Review all dynamic SQL generation
    - Check for string concatenation in queries
`
