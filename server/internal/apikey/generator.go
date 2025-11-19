package apikey

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	// KeyPrefix is the prefix for all API keys
	KeyPrefix = "dl"

	// KeyLength is the total length of the random part (before base64 encoding)
	KeyLength = 32

	// PrefixDisplayLength is how many characters of the key to show after the prefix
	PrefixDisplayLength = 8
)

// KeyGenerator handles API key generation
type KeyGenerator struct{}

// NewKeyGenerator creates a new key generator
func NewKeyGenerator() *KeyGenerator {
	return &KeyGenerator{}
}

// GenerateKey generates a new API key with format: dl_<random>_<checksum>
// Returns: prefix for display, full key, hash for storage
func (g *KeyGenerator) GenerateKey() (prefix, fullKey, hash string, err error) {
	// Generate random bytes
	randomBytes := make([]byte, KeyLength)
	_, err = rand.Read(randomBytes)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Encode to base64 (URL-safe, no padding)
	randomPart := base64.RawURLEncoding.EncodeToString(randomBytes)

	// Calculate checksum (first 8 chars of SHA256)
	checksumBytes := sha256.Sum256(randomBytes)
	checksum := hex.EncodeToString(checksumBytes[:])[:8]

	// Construct full key: dl_<random>_<checksum>
	fullKey = fmt.Sprintf("%s_%s_%s", KeyPrefix, randomPart, checksum)

	// Create display prefix: dl_<first8chars>
	displayPrefix := fmt.Sprintf("%s_%s", KeyPrefix, randomPart[:PrefixDisplayLength])

	// Hash for storage (SHA256)
	hashBytes := sha256.Sum256([]byte(fullKey))
	hash = hex.EncodeToString(hashBytes[:])

	return displayPrefix, fullKey, hash, nil
}

// ValidateKeyFormat validates the format of an API key
func (g *KeyGenerator) ValidateKeyFormat(key string) error {
	parts := strings.Split(key, "_")

	// Should have exactly 3 parts: prefix, random, checksum
	if len(parts) != 3 {
		return ErrInvalidKeyFormat
	}

	// Check prefix
	if parts[0] != KeyPrefix {
		return ErrInvalidKeyFormat
	}

	// Verify checksum
	randomPart := parts[1]
	providedChecksum := parts[2]

	// Decode the random part to get original bytes
	randomBytes, err := base64.RawURLEncoding.DecodeString(randomPart)
	if err != nil {
		return ErrInvalidKeyFormat
	}

	// Calculate expected checksum
	checksumBytes := sha256.Sum256(randomBytes)
	expectedChecksum := hex.EncodeToString(checksumBytes[:])[:8]

	// Compare checksums
	if providedChecksum != expectedChecksum {
		return ErrInvalidKeyFormat
	}

	return nil
}

// HashKey hashes an API key for storage
func (g *KeyGenerator) HashKey(key string) string {
	hashBytes := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hashBytes[:])
}

// ExtractPrefix extracts the display prefix from a full key
func (g *KeyGenerator) ExtractPrefix(key string) string {
	parts := strings.Split(key, "_")
	if len(parts) < 2 {
		return ""
	}
	randomPart := parts[1]
	if len(randomPart) < PrefixDisplayLength {
		return fmt.Sprintf("%s_%s", KeyPrefix, randomPart)
	}
	return fmt.Sprintf("%s_%s", KeyPrefix, randomPart[:PrefixDisplayLength])
}
