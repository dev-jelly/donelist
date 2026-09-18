package auth

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// AccountManagementService handles account security operations
type AccountManagementService struct {
	db     *sqlx.DB
	logger *zap.Logger
}

// NewAccountManagementService creates a new account management service
func NewAccountManagementService(db *sqlx.DB, logger *zap.Logger) *AccountManagementService {
	return &AccountManagementService{
		db:     db,
		logger: logger,
	}
}

// ChangePasswordRequest represents a password change request
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// ChangePassword changes a user's password
func (s *AccountManagementService) ChangePassword(ctx context.Context, userID uuid.UUID, req ChangePasswordRequest) error {
	// Validate new password
	if err := IsValidPassword(req.NewPassword); err != nil {
		return fmt.Errorf("invalid new password: %w", err)
	}

	// Get current password hash
	var currentHash string
	err := s.db.GetContext(ctx, &currentHash,
		"SELECT password_hash FROM users WHERE id = $1", userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Verify current password
	if err := VerifyPassword(req.CurrentPassword, currentHash); err != nil {
		return fmt.Errorf("current password is incorrect")
	}

	// Check password history to prevent reuse
	if err := s.checkPasswordHistory(ctx, userID, req.NewPassword); err != nil {
		return err
	}

	// Hash new password
	newHash, err := HashPassword(req.NewPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Store old password in history
	_, err = tx.ExecContext(ctx,
		`INSERT INTO password_history (user_id, password_hash, change_reason)
		 VALUES ($1, $2, 'user_request')`,
		userID, currentHash)
	if err != nil {
		return fmt.Errorf("failed to store password history: %w", err)
	}

	// Update user password
	_, err = tx.ExecContext(ctx,
		`UPDATE users
		 SET password_hash = $1, updated_at = CURRENT_TIMESTAMP
		 WHERE id = $2`,
		newHash, userID)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	// Invalidate all refresh tokens (force re-login)
	_, err = tx.ExecContext(ctx,
		`UPDATE refresh_tokens
		 SET revoked = true, revoked_at = CURRENT_TIMESTAMP
		 WHERE user_id = $1 AND revoked = false`,
		userID)
	if err != nil {
		s.logger.Warn("Failed to revoke refresh tokens", zap.Error(err))
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Log security event
	s.logSecurityEvent(ctx, userID, "password_changed", map[string]interface{}{
		"reason": "user_request",
	}, true)

	return nil
}

// checkPasswordHistory checks if password was recently used
func (s *AccountManagementService) checkPasswordHistory(ctx context.Context, userID uuid.UUID, newPassword string) error {
	// Get last 5 password hashes
	rows, err := s.db.QueryContext(ctx,
		`SELECT password_hash FROM password_history
		 WHERE user_id = $1
		 ORDER BY changed_at DESC
		 LIMIT 5`,
		userID)
	if err != nil {
		return fmt.Errorf("failed to get password history: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			continue
		}

		// Check if new password matches any old password
		if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(newPassword)); err == nil {
			return fmt.Errorf("password was recently used, please choose a different password")
		}
	}

	// Also check current password
	var currentHash string
	err = s.db.GetContext(ctx, &currentHash,
		"SELECT password_hash FROM users WHERE id = $1", userID)
	if err == nil {
		if err := bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(newPassword)); err == nil {
			return fmt.Errorf("new password cannot be the same as current password")
		}
	}

	return nil
}

// ResetPasswordRequest represents a password reset request
type ResetPasswordRequest struct {
	Email string `json:"email"`
}

// InitiatePasswordReset initiates a password reset process
func (s *AccountManagementService) InitiatePasswordReset(ctx context.Context, req ResetPasswordRequest) error {
	// Get user by email
	var userID uuid.UUID
	err := s.db.GetContext(ctx, &userID,
		"SELECT id FROM users WHERE email = $1 AND deleted_at IS NULL", req.Email)
	if err != nil {
		// Don't reveal if email exists
		s.logger.Info("Password reset requested for non-existent email", zap.String("email", req.Email))
		return nil
	}

	// Generate reset token
	token, err := generateSecureToken(32)
	if err != nil {
		return fmt.Errorf("failed to generate reset token: %w", err)
	}

	// Hash token for storage
	tokenHash, err := bcrypt.GenerateFromPassword([]byte(token), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash token: %w", err)
	}

	// Store token with expiration (24 hours)
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO account_recovery_tokens (user_id, token_hash, token_type, expires_at)
		 VALUES ($1, $2, 'password_reset', $3)`,
		userID, string(tokenHash), time.Now().Add(24*time.Hour))
	if err != nil {
		return fmt.Errorf("failed to store reset token: %w", err)
	}

	// Log security event
	s.logSecurityEvent(ctx, userID, "password_reset_requested", nil, true)

	// TODO: Send email with reset link containing token
	s.logger.Info("Password reset token generated",
		zap.String("user_id", userID.String()),
		zap.String("token", token)) // Remove in production

	return nil
}

// CompletePasswordReset completes a password reset using a token
func (s *AccountManagementService) CompletePasswordReset(ctx context.Context, token string, newPassword string) error {
	// Validate new password
	if err := IsValidPassword(newPassword); err != nil {
		return fmt.Errorf("invalid password: %w", err)
	}

	// Find valid token
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, token_hash FROM account_recovery_tokens
		 WHERE token_type = 'password_reset'
		   AND used = false
		   AND expires_at > CURRENT_TIMESTAMP`,
	)
	if err != nil {
		return fmt.Errorf("failed to get recovery tokens: %w", err)
	}
	defer rows.Close()

	var tokenID, userID uuid.UUID
	var tokenFound bool

	for rows.Next() {
		var id, uid uuid.UUID
		var hash string
		if err := rows.Scan(&id, &uid, &hash); err != nil {
			continue
		}

		// Check if token matches
		if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(token)); err == nil {
			tokenID = id
			userID = uid
			tokenFound = true
			break
		}
	}

	if !tokenFound {
		return fmt.Errorf("invalid or expired reset token")
	}

	// Hash new password
	newHash, err := HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Get current password for history
	var currentHash string
	err = tx.GetContext(ctx, &currentHash,
		"SELECT password_hash FROM users WHERE id = $1", userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Store old password in history
	_, err = tx.ExecContext(ctx,
		`INSERT INTO password_history (user_id, password_hash, change_reason)
		 VALUES ($1, $2, 'password_reset')`,
		userID, currentHash)
	if err != nil {
		return fmt.Errorf("failed to store password history: %w", err)
	}

	// Update password
	_, err = tx.ExecContext(ctx,
		`UPDATE users
		 SET password_hash = $1, updated_at = CURRENT_TIMESTAMP
		 WHERE id = $2`,
		newHash, userID)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	// Mark token as used
	_, err = tx.ExecContext(ctx,
		`UPDATE account_recovery_tokens
		 SET used = true, used_at = CURRENT_TIMESTAMP
		 WHERE id = $1`,
		tokenID)
	if err != nil {
		return fmt.Errorf("failed to mark token as used: %w", err)
	}

	// Invalidate all refresh tokens
	_, err = tx.ExecContext(ctx,
		`UPDATE refresh_tokens
		 SET revoked = true, revoked_at = CURRENT_TIMESTAMP
		 WHERE user_id = $1 AND revoked = false`,
		userID)
	if err != nil {
		s.logger.Warn("Failed to revoke refresh tokens", zap.Error(err))
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Log security event
	s.logSecurityEvent(ctx, userID, "password_reset_completed", nil, true)

	return nil
}

// UpdateRecoveryEmail updates the recovery email
func (s *AccountManagementService) UpdateRecoveryEmail(ctx context.Context, userID uuid.UUID, email string, password string) error {
	// Verify password
	var passwordHash string
	err := s.db.GetContext(ctx, &passwordHash,
		"SELECT password_hash FROM users WHERE id = $1", userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	if err := VerifyPassword(password, passwordHash); err != nil {
		return fmt.Errorf("invalid password")
	}

	// Update recovery email
	_, err = s.db.ExecContext(ctx,
		`UPDATE users
		 SET recovery_email = $1, recovery_email_verified = false
		 WHERE id = $2`,
		email, userID)
	if err != nil {
		return fmt.Errorf("failed to update recovery email: %w", err)
	}

	// TODO: Send verification email

	return nil
}

// DeleteAccount initiates account deletion
func (s *AccountManagementService) DeleteAccount(ctx context.Context, userID uuid.UUID, password string, reason string) error {
	// Verify password
	var passwordHash string
	err := s.db.GetContext(ctx, &passwordHash,
		"SELECT password_hash FROM users WHERE id = $1", userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	if err := VerifyPassword(password, passwordHash); err != nil {
		return fmt.Errorf("invalid password")
	}

	// Check if deletion already requested
	var existingRequest sql.NullString
	err = s.db.GetContext(ctx, &existingRequest,
		`SELECT id FROM account_deletion_requests
		 WHERE user_id = $1 AND status = 'pending'`,
		userID)
	if err == nil && existingRequest.Valid {
		return fmt.Errorf("account deletion already requested")
	}

	// Create deletion request with 30-day grace period
	scheduledDeletion := time.Now().Add(30 * 24 * time.Hour)
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO account_deletion_requests (user_id, scheduled_deletion_at, reason)
		 VALUES ($1, $2, $3)`,
		userID, scheduledDeletion, reason)
	if err != nil {
		return fmt.Errorf("failed to create deletion request: %w", err)
	}

	// Log security event
	s.logSecurityEvent(ctx, userID, "account_deletion_requested", map[string]interface{}{
		"scheduled_for": scheduledDeletion,
		"reason":        reason,
	}, true)

	return nil
}

// CancelAccountDeletion cancels a pending account deletion
func (s *AccountManagementService) CancelAccountDeletion(ctx context.Context, userID uuid.UUID) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE account_deletion_requests
		 SET status = 'cancelled', cancelled_at = CURRENT_TIMESTAMP
		 WHERE user_id = $1 AND status = 'pending'`,
		userID)
	if err != nil {
		return fmt.Errorf("failed to cancel deletion: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check result: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("no pending deletion request found")
	}

	// Log security event
	s.logSecurityEvent(ctx, userID, "account_deletion_cancelled", nil, true)

	return nil
}

// logSecurityEvent logs a security event
func (s *AccountManagementService) logSecurityEvent(ctx context.Context, userID uuid.UUID, eventType string, details map[string]interface{}, success bool) {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO security_events (user_id, event_type, event_details, success)
		 VALUES ($1, $2, $3, $4)`,
		userID, eventType, details, success)
	if err != nil {
		s.logger.Error("Failed to log security event",
			zap.Error(err),
			zap.String("user_id", userID.String()),
			zap.String("event_type", eventType))
	}
}