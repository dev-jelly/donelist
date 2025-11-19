package security

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSQLSecurityAuditor_IsSafeIdentifier(t *testing.T) {
	auditor := NewSQLSecurityAuditor()

	tests := []struct {
		name       string
		identifier string
		want       bool
	}{
		{
			name:       "Valid identifier",
			identifier: "users",
			want:       true,
		},
		{
			name:       "Valid identifier with underscore",
			identifier: "user_profiles",
			want:       true,
		},
		{
			name:       "Valid identifier starting with underscore",
			identifier: "_internal_table",
			want:       true,
		},
		{
			name:       "Invalid - starts with number",
			identifier: "1users",
			want:       false,
		},
		{
			name:       "Invalid - contains space",
			identifier: "user profiles",
			want:       false,
		},
		{
			name:       "Invalid - contains special chars",
			identifier: "users; DROP TABLE users;",
			want:       false,
		},
		{
			name:       "Invalid - empty",
			identifier: "",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := auditor.IsSafeIdentifier(tt.identifier)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestSQLSecurityAuditor_QuoteIdentifier(t *testing.T) {
	auditor := NewSQLSecurityAuditor()

	tests := []struct {
		name       string
		identifier string
		want       string
		wantErr    bool
	}{
		{
			name:       "Valid identifier",
			identifier: "users",
			want:       `"users"`,
			wantErr:    false,
		},
		{
			name:       "Valid identifier with underscore",
			identifier: "user_profiles",
			want:       `"user_profiles"`,
			wantErr:    false,
		},
		{
			name:       "Invalid identifier",
			identifier: "users; DROP TABLE",
			want:       "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := auditor.QuoteIdentifier(tt.identifier)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, result)
			}
		})
	}
}

func TestValidateSortColumn(t *testing.T) {
	allowedColumns := []string{"id", "created_at", "updated_at", "name"}

	tests := []struct {
		name    string
		column  string
		wantErr bool
	}{
		{
			name:    "Valid column",
			column:  "id",
			wantErr: false,
		},
		{
			name:    "Valid column - created_at",
			column:  "created_at",
			wantErr: false,
		},
		{
			name:    "Invalid column",
			column:  "malicious_column",
			wantErr: true,
		},
		{
			name:    "SQL injection attempt",
			column:  "id; DROP TABLE users",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSortColumn(tt.column, allowedColumns)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateSortDirection(t *testing.T) {
	tests := []struct {
		name      string
		direction string
		wantErr   bool
	}{
		{
			name:      "Valid ASC",
			direction: "ASC",
			wantErr:   false,
		},
		{
			name:      "Valid DESC",
			direction: "DESC",
			wantErr:   false,
		},
		{
			name:      "Valid lowercase asc",
			direction: "asc",
			wantErr:   false,
		},
		{
			name:      "Invalid direction",
			direction: "INVALID",
			wantErr:   true,
		},
		{
			name:      "SQL injection attempt",
			direction: "ASC; DROP TABLE",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSortDirection(tt.direction)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSafeOrderBy(t *testing.T) {
	allowedColumns := map[string]string{
		"id":         "users.id",
		"created_at": "users.created_at",
		"name":       "users.name",
	}

	tests := []struct {
		name      string
		column    string
		direction string
		want      string
		wantErr   bool
	}{
		{
			name:      "Valid ORDER BY",
			column:    "id",
			direction: "ASC",
			want:      "users.id ASC",
			wantErr:   false,
		},
		{
			name:      "Valid ORDER BY DESC",
			column:    "created_at",
			direction: "DESC",
			want:      "users.created_at DESC",
			wantErr:   false,
		},
		{
			name:      "Invalid column",
			column:    "malicious",
			direction: "ASC",
			want:      "",
			wantErr:   true,
		},
		{
			name:      "Invalid direction",
			column:    "id",
			direction: "INVALID",
			want:      "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := SafeOrderBy(tt.column, tt.direction, allowedColumns)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, result)
			}
		})
	}
}

func TestDetectSQLInjection(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "Clean input",
			input: "John Doe",
			want:  false,
		},
		{
			name:  "SQL injection with UNION",
			input: "admin' UNION SELECT * FROM users--",
			want:  true,
		},
		{
			name:  "SQL injection with OR 1=1",
			input: "admin' OR 1=1--",
			want:  true,
		},
		{
			name:  "SQL injection with comments",
			input: "test'; DROP TABLE users;--",
			want:  true,
		},
		{
			name:  "SQL injection with xp_",
			input: "'; EXEC xp_cmdshell('dir');--",
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectSQLInjection(tt.input)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestSanitizeLikePattern(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		expected string
	}{
		{
			name:     "Clean pattern",
			pattern:  "test",
			expected: "test",
		},
		{
			name:     "Pattern with %",
			pattern:  "test%pattern",
			expected: "test\\%pattern",
		},
		{
			name:     "Pattern with _",
			pattern:  "test_pattern",
			expected: "test\\_pattern",
		},
		{
			name:     "Pattern with backslash",
			pattern:  "test\\pattern",
			expected: "test\\\\pattern",
		},
		{
			name:     "Pattern with all special chars",
			pattern:  "test%_\\pattern",
			expected: "test\\%\\_\\\\pattern",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeLikePattern(tt.pattern)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildSafeLikeQuery(t *testing.T) {
	tests := []struct {
		name          string
		column        string
		pattern       string
		caseSensitive bool
		wantQuery     string
		wantPattern   string
	}{
		{
			name:          "Case insensitive search",
			column:        "name",
			pattern:       "test",
			caseSensitive: false,
			wantQuery:     "name ILIKE $",
			wantPattern:   "%test%",
		},
		{
			name:          "Case sensitive search",
			column:        "name",
			pattern:       "Test",
			caseSensitive: true,
			wantQuery:     "name LIKE $",
			wantPattern:   "%Test%",
		},
		{
			name:          "Pattern with special chars",
			column:        "name",
			pattern:       "test%_",
			caseSensitive: false,
			wantQuery:     "name ILIKE $",
			wantPattern:   "%test\\%\\_%",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, pattern := BuildSafeLikeQuery(tt.column, tt.pattern, tt.caseSensitive)
			assert.Equal(t, tt.wantQuery, query)
			assert.Equal(t, tt.wantPattern, pattern)
		})
	}
}

func TestValidateLimit(t *testing.T) {
	tests := []struct {
		name     string
		limit    int
		maxLimit int
		want     int
	}{
		{
			name:     "Valid limit",
			limit:    10,
			maxLimit: 100,
			want:     10,
		},
		{
			name:     "Negative limit - use default",
			limit:    -1,
			maxLimit: 100,
			want:     20,
		},
		{
			name:     "Zero limit - use default",
			limit:    0,
			maxLimit: 100,
			want:     20,
		},
		{
			name:     "Exceeds max - use max",
			limit:    200,
			maxLimit: 100,
			want:     100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateLimit(tt.limit, tt.maxLimit)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestValidateOffset(t *testing.T) {
	tests := []struct {
		name   string
		offset int
		want   int
	}{
		{
			name:   "Valid offset",
			offset: 10,
			want:   10,
		},
		{
			name:   "Zero offset",
			offset: 0,
			want:   0,
		},
		{
			name:   "Negative offset - use 0",
			offset: -10,
			want:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateOffset(tt.offset)
			assert.Equal(t, tt.want, result)
		})
	}
}
