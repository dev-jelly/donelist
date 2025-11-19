package validation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsSpamContent(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		isSpam   bool
		reason   string
	}{
		{
			name:    "normal content",
			content: "Working on documentation today",
			isSpam:  false,
		},
		{
			name:    "content with one URL",
			content: "Check out https://example.com for more info",
			isSpam:  false,
		},
		{
			name:    "excessive URLs",
			content: "Visit https://site1.com and https://site2.com and https://site3.com",
			isSpam:  true,
			reason:  "excessive URLs",
		},
		{
			name:    "spam phrase - click here",
			content: "Click here to claim your prize",
			isSpam:  true,
			reason:  "spam keywords",
		},
		{
			name:    "spam phrase - buy now",
			content: "Buy now while stocks last",
			isSpam:  true,
			reason:  "spam keywords",
		},
		{
			name:    "spam phrase - free money",
			content: "Get free money today",
			isSpam:  true,
			reason:  "spam keywords",
		},
		{
			name:    "excessive caps",
			content: "THIS IS VERY IMPORTANT PLEASE READ NOW!!!",
			isSpam:  true,
			reason:  "excessive capitalization",
		},
		{
			name:    "normal caps usage",
			content: "Working on API documentation today",
			isSpam:  false,
		},
		{
			name:    "excessive repetition",
			content: "Woooooow this is amazing!!!!!",
			isSpam:  true,
			reason:  "excessive character repetition",
		},
		{
			name:    "normal repetition",
			content: "Wooow this is great!",
			isSpam:  false,
		},
		{
			name:    "excessive special chars",
			content: "$$$%%% ***!!! @@@### ^^^&&& ~~~|||",
			isSpam:  true,
			reason:  "excessive special characters",
		},
		{
			name:    "normal punctuation",
			content: "Great work today! Let's continue tomorrow.",
			isSpam:  false,
		},
		{
			name:    "multiple emails",
			content: "Contact spam@test.com or fake@example.com for details",
			isSpam:  true,
			reason:  "multiple email addresses",
		},
		{
			name:    "single email (acceptable)",
			content: "Contact support@company.com for help",
			isSpam:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isSpam, reason := IsSpamContent(tt.content)
			assert.Equal(t, tt.isSpam, isSpam)
			if isSpam {
				assert.Contains(t, reason, tt.reason)
			}
		})
	}
}

func TestHasExcessiveCaps(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "normal capitalization",
			content:  "Working on the Project Today",
			expected: false,
		},
		{
			name:     "all caps short text",
			content:  "API",
			expected: false, // Too short to judge
		},
		{
			name:     "excessive caps",
			content:  "THIS IS VERY IMPORTANT",
			expected: true,
		},
		{
			name:     "mostly lowercase",
			content:  "this is mostly lowercase text",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasExcessiveCaps(tt.content)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasExcessiveRepetition(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "normal text",
			content:  "This is normal text",
			expected: false,
		},
		{
			name:     "short text",
			content:  "Hi",
			expected: false, // Too short
		},
		{
			name:     "excessive repetition",
			content:  "Nooooooo way!!!!!!",
			expected: true,
		},
		{
			name:     "acceptable repetition",
			content:  "Nooo way!",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasExcessiveRepetition(tt.content)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasExcessiveSpecialChars(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "normal text",
			content:  "Working on project today!",
			expected: false,
		},
		{
			name:     "short text",
			content:  "Hi!",
			expected: false, // Too short
		},
		{
			name:     "excessive special chars",
			content:  "$$$ *** @@@ ### %%% ^^^ &&& !!!",
			expected: true,
		},
		{
			name:     "normal punctuation",
			content:  "This is great! Let's continue tomorrow.",
			expected: false,
		},
		{
			name:     "emojis counted as special",
			content:  strings.Repeat("😀", 20),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasExcessiveSpecialChars(tt.content)
			assert.Equal(t, tt.expected, result)
		})
	}
}
