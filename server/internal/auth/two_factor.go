package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"fmt"
	"image/png"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/pquerna/otp/totp"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// TwoFactorService handles 2FA operations
type TwoFactorService struct {
	db     *sqlx.DB
	logger *zap.Logger
}

// NewTwoFactorService creates a new 2FA service
func NewTwoFactorService(db *sqlx.DB, logger *zap.Logger) *TwoFactorService {
	return &TwoFactorService{
		db:     db,
		logger: logger,
	}
}

// TwoFactorSetup represents the initial 2FA setup response
type TwoFactorSetup struct {
	Secret       string   `json:"secret"`
	QRCode       []byte   `json:"qr_code"`
	BackupCodes  []string `json:"backup_codes"`
	RecoveryURL  string   `json:"recovery_url"`
}

// GenerateTOTPSecret generates a new TOTP secret for a user
func (s *TwoFactorService) GenerateTOTPSecret(ctx context.Context, userID uuid.UUID, email string) (*TwoFactorSetup, error) {
	// Check if 2FA is already enabled
	var enabled bool
	err := s.db.GetContext(ctx, &enabled,
		"SELECT two_factor_enabled FROM users WHERE id = $1", userID)
	if err != nil {
		return nil, fmt.Errorf("failed to check 2FA status: %w", err)
	}
	if enabled {
		return nil, fmt.Errorf("2FA is already enabled for this account")
	}

	// Generate secret
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "DoneList",
		AccountName: email,
		SecretSize:  32,
	})
	if err != nil {
		s.logger.Error("Failed to generate TOTP key", zap.Error(err))
		return nil, fmt.Errorf("failed to generate TOTP secret: %w", err)
	}

	// Store secret temporarily (not enabled yet)
	_, err = s.db.ExecContext(ctx,
		"UPDATE users SET two_factor_secret = $1 WHERE id = $2",
		key.Secret(), userID)
	if err != nil {
		return nil, fmt.Errorf("failed to store 2FA secret: %w", err)
	}

	// Generate QR code
	var qrBuf bytes.Buffer
	img, err := key.Image(256, 256)
	if err != nil {
		return nil, fmt.Errorf("failed to generate QR image: %w", err)
	}
	if err := png.Encode(&qrBuf, img); err != nil {
		return nil, fmt.Errorf("failed to encode QR image: %w", err)
	}

	// Generate backup codes
	backupCodes, err := s.generateBackupCodes(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate backup codes: %w", err)
	}

	return &TwoFactorSetup{
		Secret:      key.Secret(),
		QRCode:      qrBuf.Bytes(),
		BackupCodes: backupCodes,
		RecoveryURL: key.URL(),
	}, nil
}

// VerifyAndEnableTOTP verifies the TOTP code and enables 2FA
func (s *TwoFactorService) VerifyAndEnableTOTP(ctx context.Context, userID uuid.UUID, code string) error {
	// Get the secret
	var secret sql.NullString
	err := s.db.GetContext(ctx, &secret,
		"SELECT two_factor_secret FROM users WHERE id = $1", userID)
	if err != nil {
		return fmt.Errorf("failed to get 2FA secret: %w", err)
	}
	if !secret.Valid || secret.String == "" {
		return fmt.Errorf("2FA setup not initiated")
	}

	// Verify the code
	valid := totp.Validate(code, secret.String)
	if !valid {
		return fmt.Errorf("invalid verification code")
	}

	// Enable 2FA
	_, err = s.db.ExecContext(ctx,
		`UPDATE users
		 SET two_factor_enabled = true,
		     two_factor_verified_at = CURRENT_TIMESTAMP
		 WHERE id = $1`,
		userID)
	if err != nil {
		return fmt.Errorf("failed to enable 2FA: %w", err)
	}

	// Log security event
	s.logSecurityEvent(ctx, userID, "2fa_enabled", nil, true)

	return nil
}

// VerifyTOTP verifies a TOTP code for login
func (s *TwoFactorService) VerifyTOTP(ctx context.Context, userID uuid.UUID, code string) error {
	// Get the secret and check if 2FA is enabled
	var secret sql.NullString
	var enabled bool
	err := s.db.QueryRowContext(ctx,
		`SELECT two_factor_secret, two_factor_enabled
		 FROM users WHERE id = $1`,
		userID).Scan(&secret, &enabled)
	if err != nil {
		return fmt.Errorf("failed to get 2FA status: %w", err)
	}

	if !enabled {
		return fmt.Errorf("2FA is not enabled for this account")
	}

	if !secret.Valid || secret.String == "" {
		return fmt.Errorf("2FA secret not found")
	}

	// Verify the code
	valid := totp.Validate(code, secret.String)
	if !valid {
		// Check if it's a backup code
		if err := s.verifyBackupCode(ctx, userID, code); err != nil {
			s.logSecurityEvent(ctx, userID, "2fa_verification_failed", nil, false)
			return fmt.Errorf("invalid 2FA code")
		}
	}

	return nil
}

// DisableTOTP disables 2FA for a user
func (s *TwoFactorService) DisableTOTP(ctx context.Context, userID uuid.UUID, password string) error {
	// Verify password first
	var passwordHash string
	err := s.db.GetContext(ctx, &passwordHash,
		"SELECT password_hash FROM users WHERE id = $1", userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	if err := VerifyPassword(password, passwordHash); err != nil {
		return fmt.Errorf("invalid password")
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Disable 2FA
	_, err = tx.ExecContext(ctx,
		`UPDATE users
		 SET two_factor_enabled = false,
		     two_factor_secret = NULL,
		     two_factor_verified_at = NULL
		 WHERE id = $1`,
		userID)
	if err != nil {
		return fmt.Errorf("failed to disable 2FA: %w", err)
	}

	// Delete backup codes
	_, err = tx.ExecContext(ctx,
		"DELETE FROM two_factor_backup_codes WHERE user_id = $1", userID)
	if err != nil {
		return fmt.Errorf("failed to delete backup codes: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Log security event
	s.logSecurityEvent(ctx, userID, "2fa_disabled", nil, true)

	return nil
}

// generateBackupCodes generates and stores backup codes for 2FA recovery
func (s *TwoFactorService) generateBackupCodes(ctx context.Context, userID uuid.UUID) ([]string, error) {
	codes := make([]string, 10)

	// Delete existing backup codes
	_, err := s.db.ExecContext(ctx,
		"DELETE FROM two_factor_backup_codes WHERE user_id = $1", userID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete old backup codes: %w", err)
	}

	// Generate new codes
	for i := 0; i < 10; i++ {
		code, err := generateSecureToken(8)
		if err != nil {
			return nil, fmt.Errorf("failed to generate backup code: %w", err)
		}
		codes[i] = code

		// Hash and store the code
		hashedCode, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("failed to hash backup code: %w", err)
		}

		_, err = s.db.ExecContext(ctx,
			`INSERT INTO two_factor_backup_codes (user_id, code_hash)
			 VALUES ($1, $2)`,
			userID, string(hashedCode))
		if err != nil {
			return nil, fmt.Errorf("failed to store backup code: %w", err)
		}
	}

	return codes, nil
}

// verifyBackupCode verifies and marks a backup code as used
func (s *TwoFactorService) verifyBackupCode(ctx context.Context, userID uuid.UUID, code string) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, code_hash FROM two_factor_backup_codes
		 WHERE user_id = $1 AND used = false`,
		userID)
	if err != nil {
		return fmt.Errorf("failed to get backup codes: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var codeID uuid.UUID
		var codeHash string
		if err := rows.Scan(&codeID, &codeHash); err != nil {
			continue
		}

		// Check if this code matches
		if err := bcrypt.CompareHashAndPassword([]byte(codeHash), []byte(code)); err == nil {
			// Mark as used
			_, err := s.db.ExecContext(ctx,
				`UPDATE two_factor_backup_codes
				 SET used = true, used_at = CURRENT_TIMESTAMP
				 WHERE id = $1`,
				codeID)
			if err != nil {
				return fmt.Errorf("failed to mark backup code as used: %w", err)
			}
			return nil
		}
	}

	return fmt.Errorf("invalid backup code")
}

// RegenerateBackupCodes regenerates backup codes for a user
func (s *TwoFactorService) RegenerateBackupCodes(ctx context.Context, userID uuid.UUID, password string) ([]string, error) {
	// Verify password first
	var passwordHash string
	err := s.db.GetContext(ctx, &passwordHash,
		"SELECT password_hash FROM users WHERE id = $1", userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if err := VerifyPassword(password, passwordHash); err != nil {
		return nil, fmt.Errorf("invalid password")
	}

	return s.generateBackupCodes(ctx, userID)
}

// generateSecureToken generates a cryptographically secure random token
func generateSecureToken(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)[:length], nil
}

// logSecurityEvent logs a security event
func (s *TwoFactorService) logSecurityEvent(ctx context.Context, userID uuid.UUID, eventType string, details map[string]interface{}, success bool) {
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

// GetSecurityEvents retrieves security events for a user
func (s *TwoFactorService) GetSecurityEvents(ctx context.Context, userID uuid.UUID, limit int) ([]SecurityEvent, error) {
	var events []SecurityEvent
	err := s.db.SelectContext(ctx, &events,
		`SELECT id, user_id, event_type, event_details, ip_address, user_agent, success, created_at
		 FROM security_events
		 WHERE user_id = $1
		 ORDER BY created_at DESC
		 LIMIT $2`,
		userID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get security events: %w", err)
	}
	return events, nil
}

// SecurityEvent represents a security event
type SecurityEvent struct {
	ID           uuid.UUID              `db:"id" json:"id"`
	UserID       uuid.UUID              `db:"user_id" json:"user_id"`
	EventType    string                 `db:"event_type" json:"event_type"`
	EventDetails map[string]interface{} `db:"event_details" json:"event_details"`
	IPAddress    *string                `db:"ip_address" json:"ip_address,omitempty"`
	UserAgent    *string                `db:"user_agent" json:"user_agent,omitempty"`
	Success      bool                   `db:"success" json:"success"`
	CreatedAt    time.Time              `db:"created_at" json:"created_at"`
}