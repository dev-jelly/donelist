package auth

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

const (
	// DefaultCost is the default bcrypt cost for password hashing
	DefaultCost = 12
)

// HashPassword hashes a password using bcrypt
func HashPassword(password string) (string, error) {
	if password == "" {
		return "", fmt.Errorf("password cannot be empty")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}

	return string(hash), nil
}

// VerifyPassword verifies a password against a hash
func VerifyPassword(password, hash string) error {
	if password == "" {
		return fmt.Errorf("password cannot be empty")
	}
	if hash == "" {
		return fmt.Errorf("hash cannot be empty")
	}

	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		if err == bcrypt.ErrMismatchedHashAndPassword {
			return fmt.Errorf("invalid password")
		}
		return fmt.Errorf("failed to verify password: %w", err)
	}

	return nil
}

// Common weak passwords to reject
var commonWeakPasswords = map[string]bool{
	"password": true, "password1": true, "password123": true,
	"12345678": true, "123456789": true, "1234567890": true,
	"qwerty": true, "qwerty123": true, "qwertyuiop": true,
	"letmein": true, "welcome": true, "welcome1": true,
	"monkey": true, "dragon": true, "master": true,
	"admin": true, "admin123": true, "login": true,
	"passw0rd": true, "p@ssword": true, "p@ssw0rd": true,
}

// IsValidPassword checks if a password meets security requirements
// Requirements:
//   - Minimum 12 characters (increased from 8 for better security)
//   - Maximum 72 characters (bcrypt limitation)
//   - At least 3 character categories (uppercase, lowercase, digit, special)
//   - Not in common weak passwords list
func IsValidPassword(password string) error {
	// Length checks
	if len(password) < 12 {
		return fmt.Errorf("password must be at least 12 characters long")
	}
	if len(password) > 72 {
		// bcrypt has a maximum password length of 72 bytes
		return fmt.Errorf("password must be less than 72 characters long")
	}

	// Check for common weak passwords (case-insensitive)
	if commonWeakPasswords[strings.ToLower(password)] {
		return fmt.Errorf("password is too common, please choose a stronger password")
	}

	// Count character categories
	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsDigit(char):
			hasDigit = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	categoriesCount := 0
	if hasUpper {
		categoriesCount++
	}
	if hasLower {
		categoriesCount++
	}
	if hasDigit {
		categoriesCount++
	}
	if hasSpecial {
		categoriesCount++
	}

	if categoriesCount < 3 {
		return fmt.Errorf("password must contain at least 3 of: uppercase letters, lowercase letters, digits, special characters")
	}

	return nil
}
