package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContainsProfanity(t *testing.T) {
	tests := []struct {
		name           string
		content        string
		expectProfanity bool
	}{
		{
			name:           "clean content",
			content:        "Working on my project today",
			expectProfanity: false,
		},
		{
			name:           "clean Korean content",
			content:        "오늘 프로젝트 작업 중입니다",
			expectProfanity: false,
		},
		{
			name:           "profanity - English",
			content:        "This is fucking difficult",
			expectProfanity: true,
		},
		{
			name:           "profanity - Korean",
			content:        "이건 씨발 어렵다",
			expectProfanity: true,
		},
		{
			name:           "profanity at start",
			content:        "Fuck this task",
			expectProfanity: true,
		},
		{
			name:           "profanity at end",
			content:        "This task is shit",
			expectProfanity: true,
		},
		{
			name:           "profanity with punctuation",
			content:        "This is shit!",
			expectProfanity: true,
		},
		{
			name:           "profanity uppercase",
			content:        "FUCK THIS",
			expectProfanity: true,
		},
		{
			name:           "contains word with profanity substring (should not match)",
			content:        "Python programming language",
			expectProfanity: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasProfanity, word := ContainsProfanity(tt.content)
			assert.Equal(t, tt.expectProfanity, hasProfanity)
			if hasProfanity {
				assert.NotEmpty(t, word, "should return the matched profane word")
			}
		})
	}
}

func TestFilterProfanity(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "no profanity",
			content:  "Clean content",
			expected: "Clean content",
		},
		{
			name:     "filter profanity",
			content:  "This is shit work",
			expected: "This is **** work",
		},
		{
			name:     "filter multiple",
			content:  "Fuck this shit",
			expected: "**** this ****",
		},
		{
			name:     "filter with different cases",
			content:  "Fuck this SHIT work",
			expected: "**** this **** work",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterProfanity(tt.content)
			assert.Equal(t, tt.expected, result)
		})
	}
}
