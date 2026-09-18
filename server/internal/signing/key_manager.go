package signing

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// SigningKey represents a signing key for a user
type SigningKey struct {
	ID        uuid.UUID  `db:"id" json:"id"`
	UserID    uuid.UUID  `db:"user_id" json:"user_id"`
	KeyHash   string     `db:"key_hash" json:"-"` // Never expose the actual key
	Name      string     `db:"name" json:"name"`
	Algorithm Algorithm  `db:"algorithm" json:"algorithm"`
	IsActive  bool       `db:"is_active" json:"is_active"`
	LastUsed  *time.Time `db:"last_used" json:"last_used,omitempty"`
	CreatedAt time.Time  `db:"created_at" json:"created_at"`
	ExpiresAt *time.Time `db:"expires_at" json:"expires_at,omitempty"`
}

// KeyManager manages signing keys for users
type KeyManager struct {
	db *sqlx.DB
}

// NewKeyManager creates a new key manager
func NewKeyManager(db *sqlx.DB) *KeyManager {
	return &KeyManager{
		db: db,
	}
}

// CreateKey creates a new signing key for a user
func (km *KeyManager) CreateKey(ctx context.Context, userID uuid.UUID, name string, expiresAt *time.Time) (*SigningKey, string, error) {
	// Generate a new signing key
	keyValue, err := GenerateSigningKey()
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate signing key: %w", err)
	}

	// Hash the key for storage (we store the hash, return the actual key once)
	keyHash := hashKey(keyValue)

	// Create the signing key record
	key := &SigningKey{
		ID:        uuid.New(),
		UserID:    userID,
		KeyHash:   keyHash,
		Name:      name,
		Algorithm: AlgorithmHMACSHA256,
		IsActive:  true,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
	}

	query := `
		INSERT INTO signing_keys (id, user_id, key_hash, name, algorithm, is_active, created_at, expires_at)
		VALUES (:id, :user_id, :key_hash, :name, :algorithm, :is_active, :created_at, :expires_at)
	`

	if _, err := km.db.NamedExecContext(ctx, query, key); err != nil {
		return nil, "", fmt.Errorf("failed to create signing key: %w", err)
	}

	// Return the key record and the actual key value (only time it's returned)
	return key, keyValue, nil
}

// GetActiveKeyByUserID retrieves the active signing key for a user
func (km *KeyManager) GetActiveKeyByUserID(ctx context.Context, userID uuid.UUID) (*SigningKey, error) {
	var key SigningKey

	query := `
		SELECT id, user_id, key_hash, name, algorithm, is_active, last_used, created_at, expires_at
		FROM signing_keys
		WHERE user_id = $1 AND is_active = true
		AND (expires_at IS NULL OR expires_at > NOW())
		ORDER BY created_at DESC
		LIMIT 1
	`

	if err := km.db.GetContext(ctx, &key, query, userID); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no active signing key found for user")
		}
		return nil, fmt.Errorf("failed to get signing key: %w", err)
	}

	return &key, nil
}

// GetKeyByID retrieves a signing key by its ID
func (km *KeyManager) GetKeyByID(ctx context.Context, keyID uuid.UUID) (*SigningKey, error) {
	var key SigningKey

	query := `
		SELECT id, user_id, key_hash, name, algorithm, is_active, last_used, created_at, expires_at
		FROM signing_keys
		WHERE id = $1
	`

	if err := km.db.GetContext(ctx, &key, query, keyID); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("signing key not found")
		}
		return nil, fmt.Errorf("failed to get signing key: %w", err)
	}

	return &key, nil
}

// ListKeysByUserID lists all signing keys for a user
func (km *KeyManager) ListKeysByUserID(ctx context.Context, userID uuid.UUID) ([]*SigningKey, error) {
	var keys []*SigningKey

	query := `
		SELECT id, user_id, key_hash, name, algorithm, is_active, last_used, created_at, expires_at
		FROM signing_keys
		WHERE user_id = $1
		ORDER BY created_at DESC
	`

	if err := km.db.SelectContext(ctx, &keys, query, userID); err != nil {
		return nil, fmt.Errorf("failed to list signing keys: %w", err)
	}

	return keys, nil
}

// RevokeKey revokes a signing key (deactivates it)
func (km *KeyManager) RevokeKey(ctx context.Context, keyID uuid.UUID, userID uuid.UUID) error {
	query := `
		UPDATE signing_keys
		SET is_active = false
		WHERE id = $1 AND user_id = $2
	`

	result, err := km.db.ExecContext(ctx, query, keyID, userID)
	if err != nil {
		return fmt.Errorf("failed to revoke signing key: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check affected rows: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("signing key not found or already revoked")
	}

	return nil
}

// UpdateLastUsed updates the last used timestamp for a key
func (km *KeyManager) UpdateLastUsed(ctx context.Context, keyID uuid.UUID) error {
	query := `
		UPDATE signing_keys
		SET last_used = NOW()
		WHERE id = $1
	`

	if _, err := km.db.ExecContext(ctx, query, keyID); err != nil {
		return fmt.Errorf("failed to update last used timestamp: %w", err)
	}

	return nil
}

// VerifyKeyHash verifies that a key value matches the stored hash
func (km *KeyManager) VerifyKeyHash(keyValue, keyHash string) bool {
	return hashKey(keyValue) == keyHash
}

// CleanupExpiredKeys removes expired keys from the database
func (km *KeyManager) CleanupExpiredKeys(ctx context.Context) (int64, error) {
	query := `
		DELETE FROM signing_keys
		WHERE expires_at IS NOT NULL AND expires_at < NOW()
	`

	result, err := km.db.ExecContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup expired keys: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get affected rows: %w", err)
	}

	return rows, nil
}

// hashKey creates a SHA-256 hash of the key for storage
func hashKey(key string) string {
	// Use SHA-256 to hash the key
	hash := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", hash[:])
}
