package search

import (
	"fmt"
	"strings"
)

// HighlightOptions configures text highlighting in search results
type HighlightOptions struct {
	PreTag         string // HTML tag to wrap highlighted terms (default: <mark>)
	PostTag        string // Closing HTML tag (default: </mark>)
	MaxFragments   int    // Maximum number of text fragments to return (default: 1)
	FragmentSize   int    // Maximum words per fragment (default: 20)
	MinWords       int    // Minimum words per fragment (default: 10)
	MaxWords       int    // Maximum words per fragment (default: 30)
	StartSel       string // Prefix for highlights (default: <mark>)
	StopSel        string // Suffix for highlights (default: </mark>)
	ShortWord      int    // Words shorter than this won't be highlighted (default: 3)
	HighlightAll   bool   // Highlight all occurrences vs. first occurrence
	EnableKorean   bool   // Enable Korean-specific highlighting
}

// DefaultHighlightOptions returns sensible defaults
func DefaultHighlightOptions() HighlightOptions {
	return HighlightOptions{
		PreTag:       "<mark>",
		PostTag:      "</mark>",
		MaxFragments: 1,
		FragmentSize: 20,
		MinWords:     10,
		MaxWords:     30,
		StartSel:     "<mark>",
		StopSel:      "</mark>",
		ShortWord:    3,
		HighlightAll: false,
		EnableKorean: true,
	}
}

// BuildHighlightQuery constructs a ts_headline query with custom options
func BuildHighlightQuery(content string, query string, opts HighlightOptions) string {
	// Build ts_headline options string
	options := make([]string, 0)

	if opts.MaxWords > 0 {
		options = append(options, fmt.Sprintf("MaxWords=%d", opts.MaxWords))
	}
	if opts.MinWords > 0 {
		options = append(options, fmt.Sprintf("MinWords=%d", opts.MinWords))
	}
	if opts.ShortWord > 0 {
		options = append(options, fmt.Sprintf("ShortWord=%d", opts.ShortWord))
	}
	if opts.HighlightAll {
		options = append(options, "HighlightAll=true")
	}
	if opts.MaxFragments > 0 {
		options = append(options, fmt.Sprintf("MaxFragments=%d", opts.MaxFragments))
	}
	if opts.StartSel != "" {
		// Escape single quotes in start/stop selectors
		startSel := strings.ReplaceAll(opts.StartSel, "'", "''")
		options = append(options, fmt.Sprintf("StartSel='%s'", startSel))
	}
	if opts.StopSel != "" {
		stopSel := strings.ReplaceAll(opts.StopSel, "'", "''")
		options = append(options, fmt.Sprintf("StopSel='%s'", stopSel))
	}

	optionsStr := strings.Join(options, ", ")

	// Choose language configuration based on Korean support
	langConfig := "english"
	if opts.EnableKorean {
		// PostgreSQL supports various text search configurations
		// For Korean, we'd ideally use a custom configuration
		// For now, use 'simple' which doesn't stem words (better for mixed content)
		langConfig = "simple"
	}

	return fmt.Sprintf(
		"ts_headline('%s', %s, to_tsquery('%s', $QUERY), '%s')",
		langConfig, content, langConfig, optionsStr,
	)
}

// QueryBoost represents field-specific boosting configuration
type QueryBoost struct {
	ContentBoost  float64 // Boost for content field (default: 1.0)
	CategoryBoost float64 // Boost for category name (default: 0.5)
	TagBoost      float64 // Boost for tags (default: 0.3)
}

// DefaultQueryBoost returns default boost values
func DefaultQueryBoost() QueryBoost {
	return QueryBoost{
		ContentBoost:  1.0,
		CategoryBoost: 0.5,
		TagBoost:      0.3,
	}
}

// AdvancedSearchFilters extends SearchFilters with advanced options
type AdvancedSearchFilters struct {
	SearchFilters
	HighlightOptions HighlightOptions // Highlighting configuration
	Boost            QueryBoost       // Field boosting
	FuzzyMatch       bool             // Enable fuzzy matching for typos
	PhraseMatch      bool             // Require phrase match vs. any word match
	MinScore         float32          // Minimum relevance score to include
}

// BuildAdvancedSearchQuery constructs a complex search query with boosting
func BuildAdvancedSearchQuery(filters AdvancedSearchFilters) string {
	if filters.Query == "" {
		return "" // No full-text search needed
	}

	parts := make([]string, 0)

	// Content search with boost
	if filters.Boost.ContentBoost > 0 {
		contentPart := fmt.Sprintf(
			"setweight(to_tsvector('simple', coalesce(content, '')), 'A') * %.2f",
			filters.Boost.ContentBoost,
		)
		parts = append(parts, contentPart)
	}

	// Category name search with boost (if available)
	if filters.Boost.CategoryBoost > 0 {
		categoryPart := fmt.Sprintf(
			"setweight(to_tsvector('simple', coalesce(categories.name, '')), 'B') * %.2f",
			filters.Boost.CategoryBoost,
		)
		parts = append(parts, categoryPart)
	}

	if len(parts) == 0 {
		return "search_vector" // Fallback to basic search_vector
	}

	return strings.Join(parts, " || ")
}

// SanitizeQueryForKorean enhances query sanitization for Korean text
func SanitizeQueryForKorean(query string) string {
	// Remove leading/trailing whitespace
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}

	// For Korean, we want to preserve the characters and not apply aggressive stemming
	// Split into words while preserving Korean syllables
	words := strings.Fields(query)

	sanitizedWords := make([]string, 0, len(words))
	for _, word := range words {
		// Remove special tsquery characters that could cause syntax errors
		word = strings.Map(func(r rune) rune {
			switch r {
			case '&', '|', '!', '(', ')', '<', '>', '\'', '"':
				return -1 // Remove these characters
			default:
				return r
			}
		}, word)

		// Skip empty words after sanitization
		if len(word) == 0 {
			continue
		}

		// For Korean text, we don't want to add :* suffix as it can break character matching
		// Check if word contains Korean characters
		hasKorean := false
		for _, r := range word {
			if r >= 0xAC00 && r <= 0xD7AF { // Hangul syllables range
				hasKorean = true
				break
			}
		}

		if hasKorean {
			// For Korean, just use the word as-is
			sanitizedWords = append(sanitizedWords, word)
		} else {
			// For English/ASCII, add prefix matching
			sanitizedWords = append(sanitizedWords, word+":*")
		}
	}

	if len(sanitizedWords) == 0 {
		return ""
	}

	// Join with & for AND logic (all words must match)
	return strings.Join(sanitizedWords, " & ")
}

// BuildFuzzyQuery constructs a query that allows for typos
func BuildFuzzyQuery(query string, maxDistance int) string {
	if maxDistance < 1 || maxDistance > 3 {
		maxDistance = 1 // Safe default
	}

	words := strings.Fields(query)
	fuzzyWords := make([]string, 0, len(words))

	for _, word := range words {
		// Remove special characters
		word = strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
				(r >= '0' && r <= '9') || (r >= 0xAC00 && r <= 0xD7AF) {
				return r
			}
			return -1
		}, word)

		if len(word) == 0 {
			continue
		}

		// For words longer than 4 characters, add fuzzy matching using pg_trgm
		// Note: This requires the pg_trgm extension
		if len(word) > 4 {
			fuzzyWords = append(fuzzyWords, fmt.Sprintf("(%s | similarity)", word))
		} else {
			fuzzyWords = append(fuzzyWords, word+":*")
		}
	}

	return strings.Join(fuzzyWords, " & ")
}
