package signing

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"
)

// Algorithm represents the signing algorithm
type Algorithm string

const (
	// AlgorithmHMACSHA256 uses HMAC with SHA-256
	AlgorithmHMACSHA256 Algorithm = "hmac-sha256"
)

// Signer handles request signature generation and verification
type Signer struct {
	algorithm Algorithm
}

// NewSigner creates a new Signer with the specified algorithm
func NewSigner(algorithm Algorithm) *Signer {
	return &Signer{
		algorithm: algorithm,
	}
}

// SignRequest creates a signature for a request
// The signature is computed over: timestamp + nonce + method + path + body
func (s *Signer) SignRequest(signingKey, method, path string, body []byte, timestamp int64, nonce string) (string, error) {
	if signingKey == "" {
		return "", fmt.Errorf("signing key cannot be empty")
	}

	// Create the string to sign
	stringToSign := s.createStringToSign(method, path, body, timestamp, nonce)

	// Generate signature based on algorithm
	switch s.algorithm {
	case AlgorithmHMACSHA256:
		return s.signHMACSHA256(signingKey, stringToSign)
	default:
		return "", fmt.Errorf("unsupported signing algorithm: %s", s.algorithm)
	}
}

// VerifySignature verifies that a signature is valid for the given request
func (s *Signer) VerifySignature(signingKey, method, path string, body []byte, timestamp int64, nonce, providedSignature string) error {
	if signingKey == "" {
		return fmt.Errorf("signing key cannot be empty")
	}

	if providedSignature == "" {
		return fmt.Errorf("signature cannot be empty")
	}

	// Generate expected signature
	expectedSignature, err := s.SignRequest(signingKey, method, path, body, timestamp, nonce)
	if err != nil {
		return fmt.Errorf("failed to generate expected signature: %w", err)
	}

	// Use constant-time comparison to prevent timing attacks
	if !constantTimeCompare(expectedSignature, providedSignature) {
		return fmt.Errorf("signature mismatch")
	}

	return nil
}

// VerifyTimestamp checks if the timestamp is within the allowed window
func (s *Signer) VerifyTimestamp(timestamp int64, maxAgeSeconds int64) error {
	now := time.Now().Unix()
	age := now - timestamp

	// Check if timestamp is too old
	if age > maxAgeSeconds {
		return fmt.Errorf("timestamp too old: %d seconds (max: %d)", age, maxAgeSeconds)
	}

	// Check if timestamp is in the future (with small tolerance for clock skew)
	if age < -30 { // Allow 30 seconds of clock skew
		return fmt.Errorf("timestamp is in the future: %d seconds ahead", -age)
	}

	return nil
}

// createStringToSign creates the canonical string to sign
func (s *Signer) createStringToSign(method, path string, body []byte, timestamp int64, nonce string) string {
	// Format: METHOD\nPATH\nTIMESTAMP\nNONCE\nBODY_HASH
	bodyHash := sha256.Sum256(body)
	return fmt.Sprintf("%s\n%s\n%d\n%s\n%s",
		method,
		path,
		timestamp,
		nonce,
		hex.EncodeToString(bodyHash[:]),
	)
}

// signHMACSHA256 creates an HMAC-SHA256 signature
func (s *Signer) signHMACSHA256(signingKey, stringToSign string) (string, error) {
	// Decode the signing key (assumed to be base64 encoded)
	keyBytes, err := base64.StdEncoding.DecodeString(signingKey)
	if err != nil {
		return "", fmt.Errorf("invalid signing key format: %w", err)
	}

	// Create HMAC
	h := hmac.New(sha256.New, keyBytes)
	if _, err := h.Write([]byte(stringToSign)); err != nil {
		return "", fmt.Errorf("failed to write to HMAC: %w", err)
	}

	// Return base64-encoded signature
	signature := h.Sum(nil)
	return base64.StdEncoding.EncodeToString(signature), nil
}

// constantTimeCompare compares two strings in constant time to prevent timing attacks
func constantTimeCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// GenerateSigningKey generates a new random signing key suitable for HMAC-SHA256
func GenerateSigningKey() (string, error) {
	// Generate 32 bytes (256 bits) of random data
	key := make([]byte, 32)
	if _, err := cryptoRandRead(key); err != nil {
		return "", fmt.Errorf("failed to generate random key: %w", err)
	}

	// Return base64-encoded key
	return base64.StdEncoding.EncodeToString(key), nil
}
