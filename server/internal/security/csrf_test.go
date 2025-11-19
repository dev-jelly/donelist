package security

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCSRFTokenManager_GenerateToken(t *testing.T) {
	manager := NewCSRFTokenManager("test-secret-key", 1*time.Hour)
	sessionID := "test-session-123"

	token, err := manager.GenerateToken(sessionID)
	require.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestCSRFTokenManager_ValidateToken(t *testing.T) {
	manager := NewCSRFTokenManager("test-secret-key", 1*time.Hour)
	sessionID := "test-session-123"

	// Generate a token
	token, err := manager.GenerateToken(sessionID)
	require.NoError(t, err)

	// Validate the token
	err = manager.ValidateToken(token, sessionID)
	assert.NoError(t, err)
}

func TestCSRFTokenManager_ValidateToken_InvalidSessionID(t *testing.T) {
	manager := NewCSRFTokenManager("test-secret-key", 1*time.Hour)
	sessionID := "test-session-123"
	wrongSessionID := "wrong-session"

	// Generate a token
	token, err := manager.GenerateToken(sessionID)
	require.NoError(t, err)

	// Validate with wrong session ID
	err = manager.ValidateToken(token, wrongSessionID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "session ID mismatch")
}

func TestCSRFTokenManager_ValidateToken_ExpiredToken(t *testing.T) {
	// Create manager with very short TTL
	manager := NewCSRFTokenManager("test-secret-key", 1*time.Millisecond)
	sessionID := "test-session-123"

	// Generate a token
	token, err := manager.GenerateToken(sessionID)
	require.NoError(t, err)

	// Wait for token to expire
	time.Sleep(10 * time.Millisecond)

	// Validate expired token
	err = manager.ValidateToken(token, sessionID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "token expired")
}

func TestCSRFTokenManager_ValidateToken_InvalidSignature(t *testing.T) {
	manager := NewCSRFTokenManager("test-secret-key", 1*time.Hour)
	sessionID := "test-session-123"

	// Generate a token
	token, err := manager.GenerateToken(sessionID)
	require.NoError(t, err)

	// Create a different manager with different secret
	differentManager := NewCSRFTokenManager("different-secret", 1*time.Hour)

	// Try to validate with different manager (different signature)
	err = differentManager.ValidateToken(token, sessionID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid token signature")
}

func TestCSRFTokenManager_ValidateToken_InvalidFormat(t *testing.T) {
	manager := NewCSRFTokenManager("test-secret-key", 1*time.Hour)

	tests := []struct {
		name  string
		token string
	}{
		{
			name:  "Empty token",
			token: "",
		},
		{
			name:  "Invalid base64",
			token: "not-valid-base64!!!",
		},
		{
			name:  "No signature",
			token: "dGVzdA==",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.ValidateToken(tt.token, "session-id")
			assert.Error(t, err)
		})
	}
}

func TestGenerateSessionID(t *testing.T) {
	sessionID, err := GenerateSessionID()
	require.NoError(t, err)
	assert.NotEmpty(t, sessionID)

	// Generate another and ensure they're different
	sessionID2, err := GenerateSessionID()
	require.NoError(t, err)
	assert.NotEqual(t, sessionID, sessionID2)
}

func TestCSRFTokenManager_GenerateDoubleSubmitToken(t *testing.T) {
	manager := NewCSRFTokenManager("test-secret-key", 1*time.Hour)
	sessionID := "test-session-123"

	dsc, err := manager.GenerateDoubleSubmitToken(sessionID)
	require.NoError(t, err)
	assert.NotEmpty(t, dsc.Token)
	assert.Equal(t, sessionID, dsc.SessionID)
	assert.True(t, dsc.ExpiresAt.After(time.Now()))
}

func TestCSRFTokenManager_ValidateDoubleSubmitToken(t *testing.T) {
	manager := NewCSRFTokenManager("test-secret-key", 1*time.Hour)
	sessionID := "test-session-123"

	// Generate a token
	token, err := manager.GenerateToken(sessionID)
	require.NoError(t, err)

	// Validate with matching tokens
	err = manager.ValidateDoubleSubmitToken(token, token, sessionID)
	assert.NoError(t, err)
}

func TestCSRFTokenManager_ValidateDoubleSubmitToken_Mismatch(t *testing.T) {
	manager := NewCSRFTokenManager("test-secret-key", 1*time.Hour)
	sessionID := "test-session-123"

	// Generate two different tokens
	token1, err := manager.GenerateToken(sessionID)
	require.NoError(t, err)

	token2, err := manager.GenerateToken(sessionID)
	require.NoError(t, err)

	// Validate with mismatched tokens
	err = manager.ValidateDoubleSubmitToken(token1, token2, sessionID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "CSRF token mismatch")
}

func TestCSRFTokenManager_ValidateDoubleSubmitToken_Missing(t *testing.T) {
	manager := NewCSRFTokenManager("test-secret-key", 1*time.Hour)
	sessionID := "test-session-123"

	tests := []struct {
		name        string
		headerToken string
		cookieToken string
	}{
		{
			name:        "Missing header token",
			headerToken: "",
			cookieToken: "token",
		},
		{
			name:        "Missing cookie token",
			headerToken: "token",
			cookieToken: "",
		},
		{
			name:        "Both missing",
			headerToken: "",
			cookieToken: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.ValidateDoubleSubmitToken(tt.headerToken, tt.cookieToken, sessionID)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "missing CSRF token")
		})
	}
}
