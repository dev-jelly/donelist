package signing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSigner_SignRequest(t *testing.T) {
	signer := NewSigner(AlgorithmHMACSHA256)
	signingKey, err := GenerateSigningKey()
	require.NoError(t, err)

	method := "POST"
	path := "/api/v1/users/me"
	body := []byte(`{"display_name":"Test User"}`)
	timestamp := time.Now().Unix()
	nonce := "test-nonce-123"

	t.Run("successful signing", func(t *testing.T) {
		signature, err := signer.SignRequest(signingKey, method, path, body, timestamp, nonce)
		assert.NoError(t, err)
		assert.NotEmpty(t, signature)
	})

	t.Run("empty signing key", func(t *testing.T) {
		_, err := signer.SignRequest("", method, path, body, timestamp, nonce)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "signing key cannot be empty")
	})

	t.Run("invalid signing key format", func(t *testing.T) {
		_, err := signer.SignRequest("invalid-key", method, path, body, timestamp, nonce)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid signing key format")
	})

	t.Run("consistent signatures for same input", func(t *testing.T) {
		sig1, err := signer.SignRequest(signingKey, method, path, body, timestamp, nonce)
		require.NoError(t, err)

		sig2, err := signer.SignRequest(signingKey, method, path, body, timestamp, nonce)
		require.NoError(t, err)

		assert.Equal(t, sig1, sig2, "Same input should produce same signature")
	})

	t.Run("different signatures for different inputs", func(t *testing.T) {
		sig1, err := signer.SignRequest(signingKey, method, path, body, timestamp, nonce)
		require.NoError(t, err)

		// Different body
		sig2, err := signer.SignRequest(signingKey, method, path, []byte(`{"different":"body"}`), timestamp, nonce)
		require.NoError(t, err)
		assert.NotEqual(t, sig1, sig2)

		// Different timestamp
		sig3, err := signer.SignRequest(signingKey, method, path, body, timestamp+1, nonce)
		require.NoError(t, err)
		assert.NotEqual(t, sig1, sig3)

		// Different nonce
		sig4, err := signer.SignRequest(signingKey, method, path, body, timestamp, "different-nonce")
		require.NoError(t, err)
		assert.NotEqual(t, sig1, sig4)
	})
}

func TestSigner_VerifySignature(t *testing.T) {
	signer := NewSigner(AlgorithmHMACSHA256)
	signingKey, err := GenerateSigningKey()
	require.NoError(t, err)

	method := "POST"
	path := "/api/v1/users/me"
	body := []byte(`{"display_name":"Test User"}`)
	timestamp := time.Now().Unix()
	nonce := "test-nonce-123"

	signature, err := signer.SignRequest(signingKey, method, path, body, timestamp, nonce)
	require.NoError(t, err)

	t.Run("valid signature", func(t *testing.T) {
		err := signer.VerifySignature(signingKey, method, path, body, timestamp, nonce, signature)
		assert.NoError(t, err)
	})

	t.Run("empty signing key", func(t *testing.T) {
		err := signer.VerifySignature("", method, path, body, timestamp, nonce, signature)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "signing key cannot be empty")
	})

	t.Run("empty signature", func(t *testing.T) {
		err := signer.VerifySignature(signingKey, method, path, body, timestamp, nonce, "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "signature cannot be empty")
	})

	t.Run("invalid signature", func(t *testing.T) {
		err := signer.VerifySignature(signingKey, method, path, body, timestamp, nonce, "invalid-signature")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "signature mismatch")
	})

	t.Run("tampered body", func(t *testing.T) {
		tamperedBody := []byte(`{"display_name":"Tampered"}`)
		err := signer.VerifySignature(signingKey, method, path, tamperedBody, timestamp, nonce, signature)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "signature mismatch")
	})

	t.Run("tampered method", func(t *testing.T) {
		err := signer.VerifySignature(signingKey, "PUT", path, body, timestamp, nonce, signature)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "signature mismatch")
	})

	t.Run("tampered path", func(t *testing.T) {
		err := signer.VerifySignature(signingKey, method, "/api/v1/users/other", body, timestamp, nonce, signature)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "signature mismatch")
	})

	t.Run("wrong signing key", func(t *testing.T) {
		wrongKey, err := GenerateSigningKey()
		require.NoError(t, err)

		err = signer.VerifySignature(wrongKey, method, path, body, timestamp, nonce, signature)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "signature mismatch")
	})
}

func TestSigner_VerifyTimestamp(t *testing.T) {
	signer := NewSigner(AlgorithmHMACSHA256)

	t.Run("valid timestamp", func(t *testing.T) {
		timestamp := time.Now().Unix()
		err := signer.VerifyTimestamp(timestamp, 300)
		assert.NoError(t, err)
	})

	t.Run("timestamp too old", func(t *testing.T) {
		timestamp := time.Now().Unix() - 400 // 400 seconds ago
		err := signer.VerifyTimestamp(timestamp, 300)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "timestamp too old")
	})

	t.Run("timestamp in future", func(t *testing.T) {
		timestamp := time.Now().Unix() + 60 // 60 seconds in future
		err := signer.VerifyTimestamp(timestamp, 300)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "timestamp is in the future")
	})

	t.Run("timestamp within clock skew tolerance", func(t *testing.T) {
		timestamp := time.Now().Unix() + 15 // 15 seconds in future (within 30s tolerance)
		err := signer.VerifyTimestamp(timestamp, 300)
		assert.NoError(t, err)
	})

	t.Run("timestamp at edge of window", func(t *testing.T) {
		timestamp := time.Now().Unix() - 299 // Just within 300 second window
		err := signer.VerifyTimestamp(timestamp, 300)
		assert.NoError(t, err)
	})
}

func TestGenerateSigningKey(t *testing.T) {
	t.Run("generate valid key", func(t *testing.T) {
		key, err := GenerateSigningKey()
		assert.NoError(t, err)
		assert.NotEmpty(t, key)

		// Key should be base64 encoded
		assert.Regexp(t, "^[A-Za-z0-9+/]+=*$", key)
	})

	t.Run("generate unique keys", func(t *testing.T) {
		key1, err := GenerateSigningKey()
		require.NoError(t, err)

		key2, err := GenerateSigningKey()
		require.NoError(t, err)

		assert.NotEqual(t, key1, key2, "Each generated key should be unique")
	})
}

func TestConstantTimeCompare(t *testing.T) {
	t.Run("equal strings", func(t *testing.T) {
		result := constantTimeCompare("test", "test")
		assert.True(t, result)
	})

	t.Run("different strings", func(t *testing.T) {
		result := constantTimeCompare("test", "best")
		assert.False(t, result)
	})

	t.Run("different lengths", func(t *testing.T) {
		result := constantTimeCompare("test", "testing")
		assert.False(t, result)
	})

	t.Run("empty strings", func(t *testing.T) {
		result := constantTimeCompare("", "")
		assert.True(t, result)
	})
}

func TestSigner_CreateStringToSign(t *testing.T) {
	signer := NewSigner(AlgorithmHMACSHA256)

	method := "POST"
	path := "/api/v1/test"
	body := []byte(`{"test":"data"}`)
	timestamp := int64(1234567890)
	nonce := "test-nonce"

	stringToSign := signer.createStringToSign(method, path, body, timestamp, nonce)

	// Should contain all components
	assert.Contains(t, stringToSign, method)
	assert.Contains(t, stringToSign, path)
	assert.Contains(t, stringToSign, "1234567890")
	assert.Contains(t, stringToSign, nonce)

	// Should include body hash
	assert.NotContains(t, stringToSign, `{"test":"data"}`)
	assert.Regexp(t, "[a-f0-9]{64}", stringToSign) // Should contain SHA-256 hex hash
}

func TestSigner_EmptyBody(t *testing.T) {
	signer := NewSigner(AlgorithmHMACSHA256)
	signingKey, err := GenerateSigningKey()
	require.NoError(t, err)

	method := "DELETE"
	path := "/api/v1/users/me"
	emptyBody := []byte{}
	timestamp := time.Now().Unix()
	nonce := "test-nonce"

	t.Run("sign request with empty body", func(t *testing.T) {
		signature, err := signer.SignRequest(signingKey, method, path, emptyBody, timestamp, nonce)
		assert.NoError(t, err)
		assert.NotEmpty(t, signature)
	})

	t.Run("verify signature with empty body", func(t *testing.T) {
		signature, err := signer.SignRequest(signingKey, method, path, emptyBody, timestamp, nonce)
		require.NoError(t, err)

		err = signer.VerifySignature(signingKey, method, path, emptyBody, timestamp, nonce, signature)
		assert.NoError(t, err)
	})
}
