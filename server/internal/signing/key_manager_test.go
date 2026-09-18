package signing

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) (*sqlx.DB, sqlmock.Sqlmock) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	sqlxDB := sqlx.NewDb(mockDB, "sqlmock")
	return sqlxDB, mock
}

func TestKeyManager_CreateKey(t *testing.T) {
	db, mock := setupTestDB(t)
	defer db.Close()

	km := NewKeyManager(db)
	ctx := context.Background()
	userID := uuid.New()
	keyName := "Test Key"

	t.Run("create key without expiration", func(t *testing.T) {
		mock.ExpectExec("INSERT INTO signing_keys").
			WithArgs(sqlmock.AnyArg(), userID, sqlmock.AnyArg(), keyName, AlgorithmHMACSHA256, true, sqlmock.AnyArg(), nil).
			WillReturnResult(sqlmock.NewResult(1, 1))

		key, keyValue, err := km.CreateKey(ctx, userID, keyName, nil)
		assert.NoError(t, err)
		assert.NotNil(t, key)
		assert.NotEmpty(t, keyValue)
		assert.Equal(t, userID, key.UserID)
		assert.Equal(t, keyName, key.Name)
		assert.Equal(t, AlgorithmHMACSHA256, key.Algorithm)
		assert.True(t, key.IsActive)
		assert.Nil(t, key.ExpiresAt)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("create key with expiration", func(t *testing.T) {
		expiresAt := time.Now().Add(30 * 24 * time.Hour) // 30 days

		mock.ExpectExec("INSERT INTO signing_keys").
			WithArgs(sqlmock.AnyArg(), userID, sqlmock.AnyArg(), keyName, AlgorithmHMACSHA256, true, sqlmock.AnyArg(), &expiresAt).
			WillReturnResult(sqlmock.NewResult(1, 1))

		key, keyValue, err := km.CreateKey(ctx, userID, keyName, &expiresAt)
		assert.NoError(t, err)
		assert.NotNil(t, key)
		assert.NotEmpty(t, keyValue)
		assert.NotNil(t, key.ExpiresAt)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("key hash is computed correctly", func(t *testing.T) {
		mock.ExpectExec("INSERT INTO signing_keys").
			WillReturnResult(sqlmock.NewResult(1, 1))

		key, keyValue, err := km.CreateKey(ctx, userID, keyName, nil)
		require.NoError(t, err)

		// Verify that the key hash matches the generated key
		assert.True(t, km.VerifyKeyHash(keyValue, key.KeyHash))

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestKeyManager_GetActiveKeyByUserID(t *testing.T) {
	db, mock := setupTestDB(t)
	defer db.Close()

	km := NewKeyManager(db)
	ctx := context.Background()
	userID := uuid.New()
	keyID := uuid.New()

	t.Run("get active key", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"id", "user_id", "key_hash", "name", "algorithm", "is_active", "last_used", "created_at", "expires_at"}).
			AddRow(keyID, userID, "test-hash", "Test Key", AlgorithmHMACSHA256, true, nil, time.Now(), nil)

		mock.ExpectQuery("SELECT (.+) FROM signing_keys").
			WithArgs(userID).
			WillReturnRows(rows)

		key, err := km.GetActiveKeyByUserID(ctx, userID)
		assert.NoError(t, err)
		assert.NotNil(t, key)
		assert.Equal(t, keyID, key.ID)
		assert.Equal(t, userID, key.UserID)
		assert.True(t, key.IsActive)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no active key found", func(t *testing.T) {
		mock.ExpectQuery("SELECT (.+) FROM signing_keys").
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "key_hash", "name", "algorithm", "is_active", "last_used", "created_at", "expires_at"}))

		key, err := km.GetActiveKeyByUserID(ctx, userID)
		assert.Error(t, err)
		assert.Nil(t, key)
		assert.Contains(t, err.Error(), "no active signing key found")

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestKeyManager_GetKeyByID(t *testing.T) {
	db, mock := setupTestDB(t)
	defer db.Close()

	km := NewKeyManager(db)
	ctx := context.Background()
	keyID := uuid.New()
	userID := uuid.New()

	t.Run("get existing key", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"id", "user_id", "key_hash", "name", "algorithm", "is_active", "last_used", "created_at", "expires_at"}).
			AddRow(keyID, userID, "test-hash", "Test Key", AlgorithmHMACSHA256, true, nil, time.Now(), nil)

		mock.ExpectQuery("SELECT (.+) FROM signing_keys").
			WithArgs(keyID).
			WillReturnRows(rows)

		key, err := km.GetKeyByID(ctx, keyID)
		assert.NoError(t, err)
		assert.NotNil(t, key)
		assert.Equal(t, keyID, key.ID)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("key not found", func(t *testing.T) {
		mock.ExpectQuery("SELECT (.+) FROM signing_keys").
			WithArgs(keyID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "key_hash", "name", "algorithm", "is_active", "last_used", "created_at", "expires_at"}))

		key, err := km.GetKeyByID(ctx, keyID)
		assert.Error(t, err)
		assert.Nil(t, key)
		assert.Contains(t, err.Error(), "signing key not found")

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestKeyManager_ListKeysByUserID(t *testing.T) {
	db, mock := setupTestDB(t)
	defer db.Close()

	km := NewKeyManager(db)
	ctx := context.Background()
	userID := uuid.New()

	t.Run("list multiple keys", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"id", "user_id", "key_hash", "name", "algorithm", "is_active", "last_used", "created_at", "expires_at"}).
			AddRow(uuid.New(), userID, "hash1", "Key 1", AlgorithmHMACSHA256, true, nil, time.Now(), nil).
			AddRow(uuid.New(), userID, "hash2", "Key 2", AlgorithmHMACSHA256, false, nil, time.Now(), nil)

		mock.ExpectQuery("SELECT (.+) FROM signing_keys").
			WithArgs(userID).
			WillReturnRows(rows)

		keys, err := km.ListKeysByUserID(ctx, userID)
		assert.NoError(t, err)
		assert.Len(t, keys, 2)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("list no keys", func(t *testing.T) {
		mock.ExpectQuery("SELECT (.+) FROM signing_keys").
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "key_hash", "name", "algorithm", "is_active", "last_used", "created_at", "expires_at"}))

		keys, err := km.ListKeysByUserID(ctx, userID)
		assert.NoError(t, err)
		assert.Empty(t, keys)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestKeyManager_RevokeKey(t *testing.T) {
	db, mock := setupTestDB(t)
	defer db.Close()

	km := NewKeyManager(db)
	ctx := context.Background()
	keyID := uuid.New()
	userID := uuid.New()

	t.Run("revoke existing key", func(t *testing.T) {
		mock.ExpectExec("UPDATE signing_keys").
			WithArgs(keyID, userID).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := km.RevokeKey(ctx, keyID, userID)
		assert.NoError(t, err)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("revoke non-existent key", func(t *testing.T) {
		mock.ExpectExec("UPDATE signing_keys").
			WithArgs(keyID, userID).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := km.RevokeKey(ctx, keyID, userID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "signing key not found")

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestKeyManager_UpdateLastUsed(t *testing.T) {
	db, mock := setupTestDB(t)
	defer db.Close()

	km := NewKeyManager(db)
	ctx := context.Background()
	keyID := uuid.New()

	t.Run("update last used", func(t *testing.T) {
		mock.ExpectExec("UPDATE signing_keys").
			WithArgs(keyID).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := km.UpdateLastUsed(ctx, keyID)
		assert.NoError(t, err)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestKeyManager_CleanupExpiredKeys(t *testing.T) {
	db, mock := setupTestDB(t)
	defer db.Close()

	km := NewKeyManager(db)
	ctx := context.Background()

	t.Run("cleanup expired keys", func(t *testing.T) {
		mock.ExpectExec("DELETE FROM signing_keys").
			WillReturnResult(sqlmock.NewResult(0, 3))

		count, err := km.CleanupExpiredKeys(ctx)
		assert.NoError(t, err)
		assert.Equal(t, int64(3), count)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no expired keys", func(t *testing.T) {
		mock.ExpectExec("DELETE FROM signing_keys").
			WillReturnResult(sqlmock.NewResult(0, 0))

		count, err := km.CleanupExpiredKeys(ctx)
		assert.NoError(t, err)
		assert.Equal(t, int64(0), count)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestKeyManager_VerifyKeyHash(t *testing.T) {
	db, _ := setupTestDB(t)
	defer db.Close()

	km := NewKeyManager(db)

	t.Run("verify correct hash", func(t *testing.T) {
		keyValue := "test-key-value"
		keyHash := hashKey(keyValue)

		result := km.VerifyKeyHash(keyValue, keyHash)
		assert.True(t, result)
	})

	t.Run("verify incorrect hash", func(t *testing.T) {
		keyValue := "test-key-value"
		wrongHash := "wrong-hash"

		result := km.VerifyKeyHash(keyValue, wrongHash)
		assert.False(t, result)
	})
}

func TestHashKey(t *testing.T) {
	t.Run("consistent hashing", func(t *testing.T) {
		key := "test-key"
		hash1 := hashKey(key)
		hash2 := hashKey(key)

		assert.Equal(t, hash1, hash2)
	})

	t.Run("different keys produce different hashes", func(t *testing.T) {
		hash1 := hashKey("key1")
		hash2 := hashKey("key2")

		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("hash format", func(t *testing.T) {
		key := "test-key"
		hash := hashKey(key)

		// Should be 64 character hex string (SHA-256)
		assert.Len(t, hash, 64)
		assert.Regexp(t, "^[a-f0-9]{64}$", hash)
	})
}
