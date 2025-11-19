package validation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid content",
			content: "Working on project documentation",
			wantErr: false,
		},
		{
			name:    "valid content with Korean",
			content: "프로젝트 문서 작업 중",
			wantErr: false,
		},
		{
			name:    "valid content at max length",
			content: strings.Repeat("ab ", 166) + "cd", // 500 characters, varied pattern
			wantErr: false,
		},
		{
			name:    "empty content",
			content: "",
			wantErr: true,
			errMsg:  "content cannot be empty",
		},
		{
			name:    "whitespace only",
			content: "   \t\n   ",
			wantErr: true,
			errMsg:  "content cannot be empty or only whitespace",
		},
		{
			name:    "content too long",
			content: strings.Repeat("a", 501),
			wantErr: true,
			errMsg:  "content must be less than 500 characters",
		},
		{
			name:    "content with profanity",
			content: "This is some shit work",
			wantErr: true,
			errMsg:  "content contains inappropriate language",
		},
		{
			name:    "content with Korean profanity",
			content: "이것은 씨발 작업이다",
			wantErr: true,
			errMsg:  "content contains inappropriate language",
		},
		{
			name:    "content with spam pattern",
			content: "Click here to buy now!!! Limited time offer!!!",
			wantErr: true,
			errMsg:  "content appears to be spam",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateContent(tt.content)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateTags(t *testing.T) {
	tests := []struct {
		name    string
		tags    []string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid tags",
			tags:    []string{"work", "project", "coding"},
			wantErr: false,
		},
		{
			name:    "valid Korean tags",
			tags:    []string{"작업", "프로젝트", "코딩"},
			wantErr: false,
		},
		{
			name:    "empty tags list",
			tags:    []string{},
			wantErr: false,
		},
		{
			name:    "single tag",
			tags:    []string{"work"},
			wantErr: false,
		},
		{
			name:    "tag with spaces (will be normalized)",
			tags:    []string{"work project"},
			wantErr: false,
		},
		{
			name:    "empty tag",
			tags:    []string{"work", "", "project"},
			wantErr: true,
			errMsg:  "tag cannot be empty",
		},
		{
			name:    "whitespace only tag",
			tags:    []string{"work", "   ", "project"},
			wantErr: true,
			errMsg:  "tag cannot be empty",
		},
		{
			name:    "tag too long",
			tags:    []string{strings.Repeat("a", 31)},
			wantErr: true,
			errMsg:  "tag must be less than 30 characters",
		},
		{
			name:    "duplicate tags (case-insensitive)",
			tags:    []string{"Work", "work", "WORK"},
			wantErr: true,
			errMsg:  "duplicate tag",
		},
		{
			name:    "too many tags",
			tags:    []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11"},
			wantErr: true,
			errMsg:  "maximum of 10 tags allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTags(tt.tags)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestNormalizeTag(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "lowercase conversion",
			input:    "Work",
			expected: "work",
		},
		{
			name:     "trim whitespace",
			input:    "  work  ",
			expected: "work",
		},
		{
			name:     "multiple spaces",
			input:    "work  project",
			expected: "work project",
		},
		{
			name:     "mixed case and spaces",
			input:    "  Work  PROJECT  ",
			expected: "work project",
		},
		{
			name:     "already normalized",
			input:    "work",
			expected: "work",
		},
		{
			name:     "Korean text",
			input:    "  작업  프로젝트  ",
			expected: "작업 프로젝트",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeTag(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeTags(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "normalize and deduplicate",
			input:    []string{"Work", "work", "PROJECT", "project"},
			expected: []string{"work", "project"},
		},
		{
			name:     "remove empty after normalization",
			input:    []string{"work", "   ", "project"},
			expected: []string{"work", "project"},
		},
		{
			name:     "preserve order of first occurrence",
			input:    []string{"work", "project", "Work"},
			expected: []string{"work", "project"},
		},
		{
			name:     "empty input",
			input:    []string{},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeTags(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestContentValidationError(t *testing.T) {
	err := &ContentValidationError{
		Field:   "content",
		Message: "test error",
		Value:   "test value",
	}

	assert.Equal(t, "content: test error", err.Error())
}
