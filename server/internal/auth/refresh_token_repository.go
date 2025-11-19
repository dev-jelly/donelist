package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// RefreshTokenRecord represents a refresh token in the database
type RefreshTokenRecord struct {
	ID        uuid.UUID  `db:"id"`
	UserID    uuid.UUID  `db:"user_id"`
	Token     string     `db:"token"`         // Token hash stored in DB
	JTI       string     `db:"jti"`           // JWT ID for tracking
	ExpiresAt time.Time  `db:"expires_at"`
	RevokedAt *time.Time `db:"revoked_at"`
	UsedAt    *time.Time `db:"used_at"`       // Timestamp when token was used for refresh
	CreatedAt time.Time  `db:"created_at"`
}

// RefreshTokenRepository handles refresh token database operations
type RefreshTokenRepository struct {
	db *sqlx.DB
}

// NewRefreshTokenRepository creates a new refresh token repository
func NewRefreshTokenRepository(db *sqlx.DB) *RefreshTokenRepository {
	return &RefreshTokenRepository{db: db}
}

// HashToken hashes a token string for storage
func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// Store stores a refresh token in the database with JTI tracking
func (r *RefreshTokenRepository) Store(ctx context.Context, userID uuid.UUID, token string, jti string, expiresAt time.Time) error {
	tokenHash := HashToken(token)

	query := `
		INSERT INTO refresh_tokens (user_id, token, jti, expires_at)
		VALUES ($1, $2, $3, $4)
	`

	_, err := r.db.ExecContext(ctx, query, userID, tokenHash, jti, expiresAt)
	if err != nil {
		return fmt.Errorf("failed to store refresh token: %w", err)
	}

	return nil
}

// IsJTIRevoked checks if a JTI has been revoked (prevents token reuse)
func (r *RefreshTokenRepository) IsJTIRevoked(ctx context.Context, jti string) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM refresh_tokens
			WHERE jti = $1 AND (revoked = TRUE OR used_at IS NOT NULL)
		)
	`

	var exists bool
	err := r.db.QueryRowContext(ctx, query, jti).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check JTI status: %w", err)
	}

	return exists, nil
}

// IsValid checks if a refresh token is valid (exists, not expired, not revoked, not used)
func (r *RefreshTokenRepository) IsValid(ctx context.Context, token string) (bool, uuid.UUID, string, error) {
	tokenHash := HashToken(token)

	query := `
		SELECT user_id, jti, expires_at, revoked_at, used_at
		FROM refresh_tokens
		WHERE token = $1
	`

	var userID uuid.UUID
	var jti string
	var expiresAt time.Time
	var revokedAt *time.Time
	var usedAt *time.Time

	err := r.db.QueryRowContext(ctx, query, tokenHash).Scan(&userID, &jti, &expiresAt, &revokedAt, &usedAt)
	if err != nil {
		return false, uuid.Nil, "", fmt.Errorf("refresh token not found: %w", err)
	}

	// Check if token is revoked
	if revokedAt != nil {
		return false, uuid.Nil, "", fmt.Errorf("refresh token has been revoked")
	}

	// Check if token has already been used (one-time use enforcement)
	if usedAt != nil {
		return false, uuid.Nil, "", fmt.Errorf("refresh token has already been used")
	}

	// Check if token is expired
	if time.Now().After(expiresAt) {
		return false, uuid.Nil, "", fmt.Errorf("refresh token has expired")
	}

	return true, userID, jti, nil
}

// MarkAsUsed marks a refresh token as used (for one-time use enforcement)
func (r *RefreshTokenRepository) MarkAsUsed(ctx context.Context, token string) error {
	tokenHash := HashToken(token)

	query := `
		UPDATE refresh_tokens
		SET used_at = CURRENT_TIMESTAMP
		WHERE token = $1 AND used_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query, tokenHash)
	if err != nil {
		return fmt.Errorf("failed to mark token as used: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("token not found or already used")
	}

	return nil
}

// Revoke revokes a refresh token
func (r *RefreshTokenRepository) Revoke(ctx context.Context, token string) error {
	tokenHash := HashToken(token)

	query := `
		UPDATE refresh_tokens
		SET revoked_at = CURRENT_TIMESTAMP
		WHERE token = $1
	`

	result, err := r.db.ExecContext(ctx, query, tokenHash)
	if err != nil {
		return fmt.Errorf("failed to revoke refresh token: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("refresh token not found")
	}

	return nil
}

// RevokeByJTI revokes a refresh token by its JTI
func (r *RefreshTokenRepository) RevokeByJTI(ctx context.Context, jti string) error {
	query := `
		UPDATE refresh_tokens
		SET revoked_at = CURRENT_TIMESTAMP
		WHERE jti = $1
	`

	result, err := r.db.ExecContext(ctx, query, jti)
	if err != nil {
		return fmt.Errorf("failed to revoke refresh token by JTI: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("refresh token not found")
	}

	return nil
}

// RevokeAllForUser revokes all refresh tokens for a user
func (r *RefreshTokenRepository) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	query := `
		UPDATE refresh_tokens
		SET revoked_at = CURRENT_TIMESTAMP
		WHERE user_id = $1 AND revoked_at IS NULL
	`

	_, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to revoke user tokens: %w", err)
	}

	return nil
}

// GetActiveTokenCount returns the number of active refresh tokens for a user
func (r *RefreshTokenRepository) GetActiveTokenCount(ctx context.Context, userID uuid.UUID) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM refresh_tokens
		WHERE user_id = $1
			AND revoked_at IS NULL
			AND used_at IS NULL
			AND expires_at > CURRENT_TIMESTAMP
	`

	var count int
	err := r.db.QueryRowContext(ctx, query, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count active tokens: %w", err)
	}

	return count, nil
}

// DeleteExpired deletes all expired refresh tokens (for cleanup tasks)
func (r *RefreshTokenRepository) DeleteExpired(ctx context.Context) (int64, error) {
	query := `
		DELETE FROM refresh_tokens
		WHERE expires_at < CURRENT_TIMESTAMP
	`

	result, err := r.db.ExecContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired tokens: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return rowsAffected, nil
}

// DeleteRevokedOlderThan deletes revoked tokens older than the specified duration
func (r *RefreshTokenRepository) DeleteRevokedOlderThan(ctx context.Context, duration time.Duration) (int64, error) {
	cutoffTime := time.Now().Add(-duration)

	query := `
		DELETE FROM refresh_tokens
		WHERE revoked = TRUE AND created_at < $1
	`

	result, err := r.db.ExecContext(ctx, query, cutoffTime)
	if err != nil {
		return 0, fmt.Errorf("failed to delete old revoked tokens: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return rowsAffected, nil
}
