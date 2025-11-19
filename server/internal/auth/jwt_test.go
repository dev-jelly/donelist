package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJWTManager_HS256(t *testing.T) {
	config := JWTConfig{
		SigningMethod:      SigningMethodHS256,
		Secret:             "test-secret-key-for-testing",
		AccessExpiry:       15 * time.Minute,
		RefreshExpiry:      7 * 24 * time.Hour,
		ClockSkewTolerance: 30 * time.Second,
	}

	manager, err := NewJWTManager(config)
	require.NoError(t, err)
	require.NotNil(t, manager)

	userID := uuid.New()
	email := "test@example.com"

	t.Run("Generate and validate access token", func(t *testing.T) {
		token, err := manager.GenerateAccessToken(userID, email)
		require.NoError(t, err)
		assert.NotEmpty(t, token)

		claims, err := manager.ValidateAccessToken(token)
		require.NoError(t, err)
		assert.Equal(t, userID, claims.UserID)
		assert.Equal(t, email, claims.Email)
		assert.Equal(t, AccessToken, claims.Type)
		assert.NotEmpty(t, claims.ID) // JTI should be set
	})

	t.Run("Generate and validate refresh token", func(t *testing.T) {
		token, err := manager.GenerateRefreshToken(userID, email)
		require.NoError(t, err)
		assert.NotEmpty(t, token)

		claims, err := manager.ValidateRefreshToken(token)
		require.NoError(t, err)
		assert.Equal(t, userID, claims.UserID)
		assert.Equal(t, email, claims.Email)
		assert.Equal(t, RefreshToken, claims.Type)
		assert.NotEmpty(t, claims.ID) // JTI should be set
	})

	t.Run("Token with session ID", func(t *testing.T) {
		sessionID := "session-" + uuid.New().String()
		token, err := manager.GenerateAccessTokenWithSession(userID, email, sessionID)
		require.NoError(t, err)

		claims, err := manager.ValidateAccessToken(token)
		require.NoError(t, err)
		assert.Equal(t, sessionID, claims.SessionID)
	})

	t.Run("Reject wrong token type", func(t *testing.T) {
		refreshToken, err := manager.GenerateRefreshToken(userID, email)
		require.NoError(t, err)

		// Try to validate refresh token as access token
		_, err = manager.ValidateAccessToken(refreshToken)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not an access token")
	})

	t.Run("JTI uniqueness", func(t *testing.T) {
		token1, err := manager.GenerateAccessToken(userID, email)
		require.NoError(t, err)

		token2, err := manager.GenerateAccessToken(userID, email)
		require.NoError(t, err)

		claims1, _ := manager.ValidateAccessToken(token1)
		claims2, _ := manager.ValidateAccessToken(token2)

		// JTI should be unique for each token
		assert.NotEqual(t, claims1.ID, claims2.ID)
	})

	t.Run("Token expiration", func(t *testing.T) {
		// Create manager with very short expiry
		shortConfig := config
		shortConfig.AccessExpiry = 1 * time.Millisecond

		shortManager, err := NewJWTManager(shortConfig)
		require.NoError(t, err)

		token, err := shortManager.GenerateAccessToken(userID, email)
		require.NoError(t, err)

		// Wait for token to expire
		time.Sleep(100 * time.Millisecond)

		_, err = shortManager.ValidateAccessToken(token)
		assert.Error(t, err)
	})

	t.Run("Clock skew tolerance", func(t *testing.T) {
		// Create a token that's slightly in the future (within tolerance)
		token, err := manager.GenerateAccessToken(userID, email)
		require.NoError(t, err)

		// Token should still be valid due to clock skew tolerance
		claims, err := manager.ValidateAccessToken(token)
		require.NoError(t, err)
		assert.NotNil(t, claims)
	})
}

func generateRSAKeyPair() (string, string, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}

	// Encode private key
	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	// Encode public key
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", "", err
	}
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	return string(privateKeyPEM), string(publicKeyPEM), nil
}

func TestJWTManager_RS256(t *testing.T) {
	privateKeyPEM, publicKeyPEM, err := generateRSAKeyPair()
	require.NoError(t, err)

	config := JWTConfig{
		SigningMethod:      SigningMethodRS256,
		PrivateKeyPEM:      privateKeyPEM,
		PublicKeyPEM:       publicKeyPEM,
		AccessExpiry:       15 * time.Minute,
		RefreshExpiry:      7 * 24 * time.Hour,
		ClockSkewTolerance: 30 * time.Second,
	}

	manager, err := NewJWTManager(config)
	require.NoError(t, err)
	require.NotNil(t, manager)

	userID := uuid.New()
	email := "test@example.com"

	t.Run("Generate and validate with RS256", func(t *testing.T) {
		token, err := manager.GenerateAccessToken(userID, email)
		require.NoError(t, err)
		assert.NotEmpty(t, token)

		claims, err := manager.ValidateAccessToken(token)
		require.NoError(t, err)
		assert.Equal(t, userID, claims.UserID)
		assert.Equal(t, email, claims.Email)
		assert.NotEmpty(t, claims.ID) // JTI should be set
	})

	t.Run("Reject token signed with wrong key", func(t *testing.T) {
		// Create another manager with different keys
		otherPrivateKeyPEM, _, err := generateRSAKeyPair()
		require.NoError(t, err)

		otherConfig := config
		otherConfig.PrivateKeyPEM = otherPrivateKeyPEM

		otherManager, err := NewJWTManager(otherConfig)
		require.NoError(t, err)

		// Generate token with other manager
		token, err := otherManager.GenerateAccessToken(userID, email)
		require.NoError(t, err)

		// Try to validate with original manager
		_, err = manager.ValidateAccessToken(token)
		assert.Error(t, err)
	})
}

func TestJWTManager_Configuration(t *testing.T) {
	t.Run("Missing secret for HS256", func(t *testing.T) {
		config := JWTConfig{
			SigningMethod: SigningMethodHS256,
			Secret:        "",
			AccessExpiry:  15 * time.Minute,
			RefreshExpiry: 7 * 24 * time.Hour,
		}

		_, err := NewJWTManager(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "secret cannot be empty")
	})

	t.Run("Missing keys for RS256", func(t *testing.T) {
		config := JWTConfig{
			SigningMethod: SigningMethodRS256,
			AccessExpiry:  15 * time.Minute,
			RefreshExpiry: 7 * 24 * time.Hour,
		}

		_, err := NewJWTManager(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "private and public keys required")
	})

	t.Run("Invalid expiry durations", func(t *testing.T) {
		config := JWTConfig{
			SigningMethod: SigningMethodHS256,
			Secret:        "test-secret",
			AccessExpiry:  0,
			RefreshExpiry: 7 * 24 * time.Hour,
		}

		_, err := NewJWTManager(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "access token expiry")
	})

	t.Run("Default clock skew tolerance", func(t *testing.T) {
		config := JWTConfig{
			SigningMethod:      SigningMethodHS256,
			Secret:             "test-secret",
			AccessExpiry:       15 * time.Minute,
			RefreshExpiry:      7 * 24 * time.Hour,
			ClockSkewTolerance: 0, // Should default to 30 seconds
		}

		manager, err := NewJWTManager(config)
		require.NoError(t, err)
		assert.Equal(t, 30*time.Second, manager.clockSkewTolerance)
	})
}

func TestExtractUserID(t *testing.T) {
	config := JWTConfig{
		SigningMethod: SigningMethodHS256,
		Secret:        "test-secret",
		AccessExpiry:  15 * time.Minute,
		RefreshExpiry: 7 * 24 * time.Hour,
	}

	manager, err := NewJWTManager(config)
	require.NoError(t, err)

	userID := uuid.New()
	email := "test@example.com"

	token, err := manager.GenerateAccessToken(userID, email)
	require.NoError(t, err)

	extractedUserID, err := ExtractUserID(token)
	require.NoError(t, err)
	assert.Equal(t, userID, extractedUserID)
}
