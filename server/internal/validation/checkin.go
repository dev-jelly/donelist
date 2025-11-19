package validation

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Checkin validation constants
const (
	MinContentLength = 1
	MaxContentLength = 500
	MaxTagLength     = 30
	MaxTagsPerCheckin = 10
)

// ContentValidationError represents a content validation error
type ContentValidationError struct {
	Field   string
	Message string
	Value   string
}

func (e *ContentValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidateContent validates check-in content
func ValidateContent(content string) error {
	if content == "" {
		return &ContentValidationError{
			Field:   "content",
			Message: "content cannot be empty",
			Value:   content,
		}
	}

	// Trim whitespace for validation
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return &ContentValidationError{
			Field:   "content",
			Message: "content cannot be empty or only whitespace",
			Value:   content,
		}
	}

	// Count UTF-8 characters (not bytes)
	length := utf8.RuneCountInString(content)

	if length < MinContentLength {
		return &ContentValidationError{
			Field:   "content",
			Message: fmt.Sprintf("content must be at least %d character", MinContentLength),
			Value:   content,
		}
	}

	if length > MaxContentLength {
		return &ContentValidationError{
			Field:   "content",
			Message: fmt.Sprintf("content must be less than %d characters (got %d)", MaxContentLength, length),
			Value:   content,
		}
	}

	// Check for profanity
	if containsProfanity, badWord := ContainsProfanity(content); containsProfanity {
		return &ContentValidationError{
			Field:   "content",
			Message: fmt.Sprintf("content contains inappropriate language: %s", badWord),
			Value:   content,
		}
	}

	// Check for spam patterns
	if isSpam, reason := IsSpamContent(content); isSpam {
		return &ContentValidationError{
			Field:   "content",
			Message: fmt.Sprintf("content appears to be spam: %s", reason),
			Value:   content,
		}
	}

	return nil
}

// ValidateTags validates tag names
func ValidateTags(tags []string) error {
	if len(tags) > MaxTagsPerCheckin {
		return &ContentValidationError{
			Field:   "tags",
			Message: fmt.Sprintf("maximum of %d tags allowed per check-in (got %d)", MaxTagsPerCheckin, len(tags)),
		}
	}

	seen := make(map[string]bool)
	for _, tag := range tags {
		// Normalize for validation
		normalized := NormalizeTag(tag)

		if normalized == "" {
			return &ContentValidationError{
				Field:   "tags",
				Message: "tag cannot be empty or only whitespace",
				Value:   tag,
			}
		}

		if utf8.RuneCountInString(normalized) > MaxTagLength {
			return &ContentValidationError{
				Field:   "tags",
				Message: fmt.Sprintf("tag must be less than %d characters", MaxTagLength),
				Value:   tag,
			}
		}

		// Check for duplicates (case-insensitive)
		if seen[normalized] {
			return &ContentValidationError{
				Field:   "tags",
				Message: fmt.Sprintf("duplicate tag: %s", tag),
				Value:   tag,
			}
		}
		seen[normalized] = true
	}

	return nil
}

// NormalizeTag normalizes a tag name
func NormalizeTag(tag string) string {
	// Trim whitespace
	normalized := strings.TrimSpace(tag)

	// Convert to lowercase
	normalized = strings.ToLower(normalized)

	// Replace multiple spaces with single space
	normalized = strings.Join(strings.Fields(normalized), " ")

	return normalized
}

// NormalizeTags normalizes and deduplicates a list of tags
func NormalizeTags(tags []string) []string {
	seen := make(map[string]bool)
	normalized := make([]string, 0, len(tags))

	for _, tag := range tags {
		norm := NormalizeTag(tag)
		if norm != "" && !seen[norm] {
			seen[norm] = true
			normalized = append(normalized, norm)
		}
	}

	return normalized
}
