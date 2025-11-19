package validation

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	// URL pattern for spam detection
	urlPattern = regexp.MustCompile(`https?://[^\s]+`)

	// Email pattern for spam detection
	emailPattern = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)

	// Common spam phrases
	spamPhrases = []string{
		"click here", "buy now", "limited time", "act now",
		"free money", "make money fast", "congratulations",
		"you have won", "claim your prize", "unsubscribe",
		"viagra", "cialis", "weight loss", "crypto",
		"bitcoin", "investment opportunity",
	}
)

// IsSpamContent checks if content appears to be spam
// Returns (isSpam bool, reason string)
func IsSpamContent(content string) (bool, string) {
	lowerContent := strings.ToLower(content)

	// Check for excessive URLs
	urlMatches := urlPattern.FindAllString(content, -1)
	if len(urlMatches) > 2 {
		return true, "contains excessive URLs"
	}

	// Check for email addresses (unusual in check-in content)
	emailMatches := emailPattern.FindAllString(content, -1)
	if len(emailMatches) > 1 {
		return true, "contains multiple email addresses"
	}

	// Check for spam phrases
	for _, phrase := range spamPhrases {
		if strings.Contains(lowerContent, phrase) {
			return true, "contains spam keywords"
		}
	}

	// Check for excessive capitalization (>70% caps)
	if hasExcessiveCaps(content) {
		return true, "excessive capitalization"
	}

	// Check for excessive repetition
	if hasExcessiveRepetition(content) {
		return true, "excessive character repetition"
	}

	// Check for excessive emojis or special characters
	if hasExcessiveSpecialChars(content) {
		return true, "excessive special characters"
	}

	return false, ""
}

// hasExcessiveCaps checks if content has too many capital letters
func hasExcessiveCaps(content string) bool {
	if len(content) < 10 {
		return false // Too short to judge
	}

	capsCount := 0
	letterCount := 0

	for _, r := range content {
		if unicode.IsLetter(r) {
			letterCount++
			if unicode.IsUpper(r) {
				capsCount++
			}
		}
	}

	if letterCount == 0 {
		return false
	}

	// More than 70% caps is suspicious
	capsRatio := float64(capsCount) / float64(letterCount)
	return capsRatio > 0.7 && letterCount > 10
}

// hasExcessiveRepetition checks for repeated characters
func hasExcessiveRepetition(content string) bool {
	if len(content) < 10 {
		return false
	}

	// Check for same character repeated more than 5 times
	maxRepeat := 1
	currentRepeat := 1
	var lastRune rune

	for i, r := range content {
		if i > 0 && r == lastRune {
			currentRepeat++
			if currentRepeat > maxRepeat {
				maxRepeat = currentRepeat
			}
		} else {
			currentRepeat = 1
		}
		lastRune = r
	}

	return maxRepeat > 5
}

// hasExcessiveSpecialChars checks for too many special characters/emojis
func hasExcessiveSpecialChars(content string) bool {
	if len(content) < 10 {
		return false
	}

	specialCount := 0
	totalCount := 0

	for _, r := range content {
		totalCount++
		// Count emojis and special symbols
		if !unicode.IsLetter(r) && !unicode.IsNumber(r) && !unicode.IsSpace(r) {
			// Common punctuation is okay
			if r != '.' && r != ',' && r != '!' && r != '?' && r != '-' && r != '\'' && r != '"' {
				specialCount++
			}
		}
	}

	if totalCount == 0 {
		return false
	}

	// More than 30% special chars is suspicious
	specialRatio := float64(specialCount) / float64(totalCount)
	return specialRatio > 0.3
}
