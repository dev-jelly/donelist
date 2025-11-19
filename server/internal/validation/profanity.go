package validation

import (
	"strings"
)

// Common profanity words (basic list - should be expanded or use external service)
// This is a minimal implementation. For production, consider:
// - Using a comprehensive profanity filter library
// - Supporting multiple languages (especially Korean for this app)
// - Using a moderation API like Azure Content Moderator or AWS Comprehend
var profanityList = []string{
	// English basic profanity
	"fuck", "shit", "ass", "bitch", "damn", "bastard",
	"cunt", "dick", "cock", "pussy", "fag", "nigger",
	// Add Korean profanity here
	"씨발", "병신", "개새끼", "미친", "좆",
	// Add more as needed
}

// ContainsProfanity checks if content contains profanity
// Returns (hasProfanity bool, matchedWord string)
func ContainsProfanity(content string) (bool, string) {
	// Convert to lowercase for case-insensitive matching
	lowerContent := strings.ToLower(content)

	for _, word := range profanityList {
		// Check for exact word match (with word boundaries)
		if containsWord(lowerContent, strings.ToLower(word)) {
			return true, word
		}
	}

	return false, ""
}

// containsWord checks if a word exists in content with word boundaries
func containsWord(content, word string) bool {
	// Simple word boundary detection
	// For production, use regex or more sophisticated matching
	if strings.Contains(content, word) {
		return true
	}

	// Check with common separators
	separators := []string{" ", ".", ",", "!", "?", ";", ":", "\n", "\t"}
	for _, sep := range separators {
		if strings.Contains(content, sep+word+sep) ||
			strings.HasPrefix(content, word+sep) ||
			strings.HasSuffix(content, sep+word) {
			return true
		}
	}

	return false
}

// FilterProfanity replaces profanity with asterisks (optional utility)
func FilterProfanity(content string) string {
	filtered := content
	lowerContent := strings.ToLower(content)

	for _, word := range profanityList {
		lowerWord := strings.ToLower(word)
		if containsWord(lowerContent, lowerWord) {
			// Replace with asterisks of same length
			replacement := strings.Repeat("*", len(word))
			filtered = strings.ReplaceAll(filtered, word, replacement)
			filtered = strings.ReplaceAll(filtered, strings.Title(word), replacement)
			filtered = strings.ReplaceAll(filtered, strings.ToUpper(word), replacement)
		}
	}

	return filtered
}
