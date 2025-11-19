package apikey

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeyGenerator_GenerateKey(t *testing.T) {
	gen := NewKeyGenerator()

	t.Run("generates valid key format", func(t *testing.T) {
		prefix, fullKey, hash, err := gen.GenerateKey()
		require.NoError(t, err)

		// Check prefix format
		assert.True(t, strings.HasPrefix(prefix, "dl_"))
		assert.Len(t, strings.Split(prefix, "_"), 2)

		// Check full key format
		parts := strings.Split(fullKey, "_")
		assert.Len(t, parts, 3, "Key should have 3 parts: prefix, random, checksum")
		assert.Equal(t, "dl", parts[0])
		assert.NotEmpty(t, parts[1]) // random part
		assert.Len(t, parts[2], 8)   // checksum should be 8 chars

		// Check hash is not empty
		assert.NotEmpty(t, hash)
		assert.Len(t, hash, 64) // SHA256 hex should be 64 chars
	})

	t.Run("generates unique keys", func(t *testing.T) {
		keys := make(map[string]bool)
		hashes := make(map[string]bool)

		for i := 0; i < 100; i++ {
			_, fullKey, hash, err := gen.GenerateKey()
			require.NoError(t, err)

			// Check uniqueness
			assert.False(t, keys[fullKey], "Generated duplicate key")
			assert.False(t, hashes[hash], "Generated duplicate hash")

			keys[fullKey] = true
			hashes[hash] = true
		}
	})
}

func TestKeyGenerator_ValidateKeyFormat(t *testing.T) {
	gen := NewKeyGenerator()

	t.Run("validates correctly formatted key", func(t *testing.T) {
		_, fullKey, _, err := gen.GenerateKey()
		require.NoError(t, err)

		err = gen.ValidateKeyFormat(fullKey)
		assert.NoError(t, err)
	})

	t.Run("rejects invalid prefix", func(t *testing.T) {
		_, fullKey, _, err := gen.GenerateKey()
		require.NoError(t, err)

		invalidKey := strings.Replace(fullKey, "dl_", "xx_", 1)
		err = gen.ValidateKeyFormat(invalidKey)
		assert.ErrorIs(t, err, ErrInvalidKeyFormat)
	})

	t.Run("rejects wrong number of parts", func(t *testing.T) {
		err := gen.ValidateKeyFormat("dl_only_two_parts")
		assert.ErrorIs(t, err, ErrInvalidKeyFormat)

		err = gen.ValidateKeyFormat("dl_too_many_parts_here_extra")
		assert.ErrorIs(t, err, ErrInvalidKeyFormat)
	})

	t.Run("rejects invalid checksum", func(t *testing.T) {
		_, fullKey, _, err := gen.GenerateKey()
		require.NoError(t, err)

		parts := strings.Split(fullKey, "_")
		// Tamper with checksum
		invalidKey := parts[0] + "_" + parts[1] + "_" + "00000000"

		err = gen.ValidateKeyFormat(invalidKey)
		assert.ErrorIs(t, err, ErrInvalidKeyFormat)
	})

	t.Run("rejects tampered key", func(t *testing.T) {
		_, fullKey, _, err := gen.GenerateKey()
		require.NoError(t, err)

		parts := strings.Split(fullKey, "_")
		// Tamper with random part
		tamperedRandom := parts[1][:len(parts[1])-1] + "X"
		invalidKey := parts[0] + "_" + tamperedRandom + "_" + parts[2]

		err = gen.ValidateKeyFormat(invalidKey)
		assert.ErrorIs(t, err, ErrInvalidKeyFormat)
	})
}

func TestKeyGenerator_HashKey(t *testing.T) {
	gen := NewKeyGenerator()

	t.Run("generates consistent hash", func(t *testing.T) {
		_, fullKey, hash1, err := gen.GenerateKey()
		require.NoError(t, err)

		hash2 := gen.HashKey(fullKey)
		assert.Equal(t, hash1, hash2)
	})

	t.Run("generates different hashes for different keys", func(t *testing.T) {
		_, key1, hash1, err := gen.GenerateKey()
		require.NoError(t, err)

		_, key2, hash2, err := gen.GenerateKey()
		require.NoError(t, err)

		assert.NotEqual(t, key1, key2)
		assert.NotEqual(t, hash1, hash2)
	})
}

func TestKeyGenerator_ExtractPrefix(t *testing.T) {
	gen := NewKeyGenerator()

	t.Run("extracts correct prefix", func(t *testing.T) {
		prefix, fullKey, _, err := gen.GenerateKey()
		require.NoError(t, err)

		extractedPrefix := gen.ExtractPrefix(fullKey)
		assert.Equal(t, prefix, extractedPrefix)
	})

	t.Run("handles invalid key", func(t *testing.T) {
		prefix := gen.ExtractPrefix("invalid")
		assert.Equal(t, "", prefix)
	})
}

func TestValidateScopes(t *testing.T) {
	t.Run("validates correct scopes", func(t *testing.T) {
		scopes := []Scope{
			ScopeCheckinsRead,
			ScopeCheckinsWrite,
			ScopeTagsRead,
		}
		err := ValidateScopes(scopes)
		assert.NoError(t, err)
	})

	t.Run("rejects invalid scope", func(t *testing.T) {
		scopes := []Scope{
			ScopeCheckinsRead,
			Scope("invalid:scope"),
		}
		err := ValidateScopes(scopes)
		assert.ErrorIs(t, err, ErrInvalidScope)
	})

	t.Run("validates empty scopes", func(t *testing.T) {
		scopes := []Scope{}
		err := ValidateScopes(scopes)
		assert.NoError(t, err)
	})
}

func TestAPIKey_Methods(t *testing.T) {
	now := time.Now()
	future := now.AddDate(0, 0, 1)
	past := now.AddDate(0, 0, -1)

	t.Run("IsExpired", func(t *testing.T) {
		// No expiry
		key := &APIKey{}
		assert.False(t, key.IsExpired())

		// Future expiry
		key.ExpiresAt = &future
		assert.False(t, key.IsExpired())

		// Past expiry
		key.ExpiresAt = &past
		assert.True(t, key.IsExpired())
	})

	t.Run("IsValid", func(t *testing.T) {
		// Valid key
		key := &APIKey{Revoked: false}
		assert.True(t, key.IsValid())

		// Revoked key
		key.Revoked = true
		assert.False(t, key.IsValid())

		// Expired key
		key.Revoked = false
		key.ExpiresAt = &past
		assert.False(t, key.IsValid())
	})

	t.Run("HasScope", func(t *testing.T) {
		key := &APIKey{
			Scopes: []string{
				string(ScopeCheckinsRead),
				string(ScopeCheckinsWrite),
			},
		}

		assert.True(t, key.HasScope(ScopeCheckinsRead))
		assert.True(t, key.HasScope(ScopeCheckinsWrite))
		assert.False(t, key.HasScope(ScopeTagsRead))
	})

	t.Run("HasAnyScope", func(t *testing.T) {
		key := &APIKey{
			Scopes: []string{
				string(ScopeCheckinsRead),
			},
		}

		assert.True(t, key.HasAnyScope(ScopeCheckinsRead, ScopeTagsRead))
		assert.False(t, key.HasAnyScope(ScopeTagsRead, ScopeTagsWrite))
	})
}
