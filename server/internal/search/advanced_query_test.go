package search

import (
	"strings"
	"testing"
)

func TestDefaultHighlightOptions(t *testing.T) {
	opts := DefaultHighlightOptions()

	if opts.PreTag != "<mark>" {
		t.Errorf("Expected PreTag to be '<mark>', got %s", opts.PreTag)
	}
	if opts.PostTag != "</mark>" {
		t.Errorf("Expected PostTag to be '</mark>', got %s", opts.PostTag)
	}
	if !opts.EnableKorean {
		t.Error("Expected EnableKorean to be true by default")
	}
}

func TestBuildHighlightQuery(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		query    string
		opts     HighlightOptions
		contains []string // Strings that should appear in output
	}{
		{
			name:    "basic english highlighting",
			content: "content",
			query:   "test",
			opts:    DefaultHighlightOptions(),
			contains: []string{
				"ts_headline",
				"content",
				"to_tsquery",
				"StartSel='<mark>'",
				"StopSel='</mark>'",
			},
		},
		{
			name:    "korean highlighting",
			content: "content",
			query:   "테스트",
			opts: HighlightOptions{
				EnableKorean: true,
				StartSel:     "<em>",
				StopSel:      "</em>",
				MaxWords:     20,
			},
			contains: []string{
				"ts_headline",
				"simple", // Should use 'simple' for Korean
				"StartSel='<em>'",
				"StopSel='</em>'",
			},
		},
		{
			name:    "custom options",
			content: "content",
			query:   "search",
			opts: HighlightOptions{
				MaxWords:     50,
				MinWords:     15,
				MaxFragments: 3,
				HighlightAll: true,
			},
			contains: []string{
				"MaxWords=50",
				"MinWords=15",
				"MaxFragments=3",
				"HighlightAll=true",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildHighlightQuery(tt.content, tt.query, tt.opts)

			for _, expected := range tt.contains {
				if !strings.Contains(result, expected) {
					t.Errorf("Expected result to contain %q, got: %s", expected, result)
				}
			}
		})
	}
}

func TestSanitizeQueryForKorean(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple english",
			input:    "hello world",
			expected: "hello:* & world:*",
		},
		{
			name:     "korean text",
			input:    "안녕하세요",
			expected: "안녕하세요",
		},
		{
			name:     "mixed korean and english",
			input:    "hello 세계",
			expected: "hello:* & 세계",
		},
		{
			name:     "special characters removed",
			input:    "test & query | search",
			expected: "test:* & query:* & search:*",
		},
		{
			name:     "quoted phrase",
			input:    "\"exact match\"",
			expected: "exact:* & match:*",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only special characters",
			input:    "&&||!!",
			expected: "",
		},
		{
			name:     "korean with particles",
			input:    "테스트를 하다",
			expected: "테스트를 & 하다",
		},
		{
			name:     "multiple spaces",
			input:    "test    query",
			expected: "test:* & query:*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeQueryForKorean(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeQueryForKorean(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDefaultQueryBoost(t *testing.T) {
	boost := DefaultQueryBoost()

	if boost.ContentBoost != 1.0 {
		t.Errorf("Expected ContentBoost to be 1.0, got %f", boost.ContentBoost)
	}
	if boost.CategoryBoost != 0.5 {
		t.Errorf("Expected CategoryBoost to be 0.5, got %f", boost.CategoryBoost)
	}
	if boost.TagBoost != 0.3 {
		t.Errorf("Expected TagBoost to be 0.3, got %f", boost.TagBoost)
	}
}

func TestBuildAdvancedSearchQuery(t *testing.T) {
	tests := []struct {
		name     string
		filters  AdvancedSearchFilters
		contains []string
	}{
		{
			name: "with content boost",
			filters: AdvancedSearchFilters{
				SearchFilters: SearchFilters{Query: "test"},
				Boost:         QueryBoost{ContentBoost: 1.5},
			},
			contains: []string{"to_tsvector", "content", "1.50"},
		},
		{
			name: "with category boost",
			filters: AdvancedSearchFilters{
				SearchFilters: SearchFilters{Query: "test"},
				Boost:         QueryBoost{ContentBoost: 1.0, CategoryBoost: 0.8},
			},
			contains: []string{"content", "categories.name", "0.80"},
		},
		{
			name: "no query returns empty",
			filters: AdvancedSearchFilters{
				SearchFilters: SearchFilters{Query: ""},
				Boost:         DefaultQueryBoost(),
			},
			contains: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildAdvancedSearchQuery(tt.filters)

			if tt.filters.SearchFilters.Query == "" {
				if result != "" {
					t.Errorf("Expected empty result for empty query, got %q", result)
				}
				return
			}

			for _, expected := range tt.contains {
				if !strings.Contains(result, expected) {
					t.Errorf("Expected result to contain %q, got: %s", expected, result)
				}
			}
		})
	}
}

func TestBuildFuzzyQuery(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		maxDistance int
		contains    []string
		notContains []string
	}{
		{
			name:        "short words use prefix match",
			query:       "test",
			maxDistance: 1,
			contains:    []string{"test:*"},
		},
		{
			name:        "long words use fuzzy match",
			query:       "testing",
			maxDistance: 1,
			contains:    []string{"testing", "similarity"},
		},
		{
			name:        "mixed length words",
			query:       "the testing",
			maxDistance: 1,
			contains:    []string{"the:*", "testing", "similarity"},
		},
		{
			name:        "korean text preserved",
			query:       "테스트",
			maxDistance: 1,
			contains:    []string{"테스트"},
		},
		{
			name:        "special characters removed",
			query:       "test@query!",
			maxDistance: 1,
			contains:    []string{"test:*", "query:*"},
			notContains: []string{"@", "!"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildFuzzyQuery(tt.query, tt.maxDistance)

			for _, expected := range tt.contains {
				if !strings.Contains(result, expected) {
					t.Errorf("Expected result to contain %q, got: %s", expected, result)
				}
			}

			for _, notExpected := range tt.notContains {
				if strings.Contains(result, notExpected) {
					t.Errorf("Expected result NOT to contain %q, got: %s", notExpected, result)
				}
			}
		})
	}
}

// Benchmark tests
func BenchmarkSanitizeQueryForKorean(b *testing.B) {
	queries := []string{
		"hello world",
		"테스트 쿼리",
		"mixed 한글 english",
		"special & characters | removed",
	}

	for i := 0; i < b.N; i++ {
		for _, q := range queries {
			SanitizeQueryForKorean(q)
		}
	}
}

func BenchmarkBuildHighlightQuery(b *testing.B) {
	opts := DefaultHighlightOptions()

	for i := 0; i < b.N; i++ {
		BuildHighlightQuery("content", "search query", opts)
	}
}
