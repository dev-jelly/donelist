package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

// CSRFTokenManager manages CSRF tokens
type CSRFTokenManager struct {
	secret []byte
	ttl    time.Duration
}

// NewCSRFTokenManager creates a new CSRF token manager
func NewCSRFTokenManager(secret string, ttl time.Duration) *CSRFTokenManager {
	return &CSRFTokenManager{
		secret: []byte(secret),
		ttl:    ttl,
	}
}

// GenerateToken generates a new CSRF token for a user session
func (m *CSRFTokenManager) GenerateToken(sessionID string) (string, error) {
	// Generate random bytes
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Create token payload: timestamp|sessionID|randomBytes
	timestamp := time.Now().Unix()
	payload := fmt.Sprintf("%d|%s|%s", timestamp, sessionID, base64.URLEncoding.EncodeToString(randomBytes))

	// Sign the payload
	signature := m.sign(payload)

	// Combine payload and signature
	token := fmt.Sprintf("%s.%s", payload, signature)

	return base64.URLEncoding.EncodeToString([]byte(token)), nil
}

// ValidateToken validates a CSRF token
func (m *CSRFTokenManager) ValidateToken(token, sessionID string) error {
	// Decode the token
	decoded, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return fmt.Errorf("invalid token encoding: %w", err)
	}

	// Split token into payload and signature
	parts := strings.Split(string(decoded), ".")
	if len(parts) != 2 {
		return fmt.Errorf("invalid token format")
	}

	payload := parts[0]
	signature := parts[1]

	// Verify signature
	expectedSignature := m.sign(payload)
	if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
		return fmt.Errorf("invalid token signature")
	}

	// Parse payload
	payloadParts := strings.Split(payload, "|")
	if len(payloadParts) != 3 {
		return fmt.Errorf("invalid payload format")
	}

	// Extract timestamp and session ID
	var timestamp int64
	if _, err := fmt.Sscanf(payloadParts[0], "%d", &timestamp); err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}

	tokenSessionID := payloadParts[1]

	// Validate session ID
	if tokenSessionID != sessionID {
		return fmt.Errorf("session ID mismatch")
	}

	// Check token expiration
	tokenTime := time.Unix(timestamp, 0)
	if time.Since(tokenTime) > m.ttl {
		return fmt.Errorf("token expired")
	}

	return nil
}

// sign creates an HMAC signature for the payload
func (m *CSRFTokenManager) sign(payload string) string {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(payload))
	signature := mac.Sum(nil)
	return base64.URLEncoding.EncodeToString(signature)
}

// GenerateSessionID generates a cryptographically secure session ID
func GenerateSessionID() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate session ID: %w", err)
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// DoubleSubmitCookie represents a double-submit cookie CSRF token
type DoubleSubmitCookie struct {
	Token     string
	SessionID string
	ExpiresAt time.Time
}

// GenerateDoubleSubmitToken generates a token for double-submit cookie pattern
func (m *CSRFTokenManager) GenerateDoubleSubmitToken(sessionID string) (*DoubleSubmitCookie, error) {
	token, err := m.GenerateToken(sessionID)
	if err != nil {
		return nil, err
	}

	return &DoubleSubmitCookie{
		Token:     token,
		SessionID: sessionID,
		ExpiresAt: time.Now().Add(m.ttl),
	}, nil
}

// ValidateDoubleSubmitToken validates a double-submit cookie token
func (m *CSRFTokenManager) ValidateDoubleSubmitToken(headerToken, cookieToken, sessionID string) error {
	// Both tokens must be present
	if headerToken == "" || cookieToken == "" {
		return fmt.Errorf("missing CSRF token")
	}

	// Tokens must match
	if headerToken != cookieToken {
		return fmt.Errorf("CSRF token mismatch")
	}

	// Validate the token
	return m.ValidateToken(headerToken, sessionID)
}
