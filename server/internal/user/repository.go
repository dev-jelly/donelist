package user

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// UserRole represents a user's role in the system
type UserRole string

const (
	RoleUser       UserRole = "user"
	RoleAdmin      UserRole = "admin"
	RoleSuperAdmin UserRole = "superadmin"
)

// User represents a user in the system
type User struct {
	ID             uuid.UUID  `db:"id" json:"id"`
	Email          string     `db:"email" json:"email"`
	PasswordHash   string     `db:"password_hash" json:"-"`
	DisplayName    *string    `db:"display_name" json:"display_name,omitempty"`
	Role           string     `db:"role" json:"role"`
	Tier           string     `db:"tier" json:"tier"`
	TierExpiresAt  *time.Time `db:"tier_expires_at" json:"tier_expires_at,omitempty"`
	ModePreference string     `db:"mode_preference" json:"mode_preference"`
	CreatedAt      time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time  `db:"updated_at" json:"updated_at"`
	DeletedAt      *time.Time `db:"deleted_at" json:"-"`
}

// IsAdmin checks if user has admin role
func (u *User) IsAdmin() bool {
	return u.Role == string(RoleAdmin) || u.Role == string(RoleSuperAdmin)
}

// IsSuperAdmin checks if user has superadmin role
func (u *User) IsSuperAdmin() bool {
	return u.Role == string(RoleSuperAdmin)
}

// CreateUserInput represents input for creating a user
type CreateUserInput struct {
	Email        string
	PasswordHash string
	DisplayName  *string
}

// UpdateUserInput represents input for updating a user
type UpdateUserInput struct {
	DisplayName *string
}

// Repository handles user database operations
type Repository struct {
	db *sqlx.DB
}

// NewRepository creates a new user repository
func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

// Create creates a new user
func (r *Repository) Create(ctx context.Context, input CreateUserInput) (*User, error) {
	query := `
		INSERT INTO users (email, password_hash, display_name)
		VALUES ($1, $2, $3)
		RETURNING id, email, password_hash, display_name, role, tier, tier_expires_at, created_at, updated_at, deleted_at
	`

	var user User
	err := r.db.GetContext(ctx, &user, query, input.Email, input.PasswordHash, input.DisplayName)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return &user, nil
}

// GetByID retrieves a user by ID
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*User, error) {
	query := `
		SELECT id, email, password_hash, display_name, role, tier, tier_expires_at, mode_preference, created_at, updated_at, deleted_at
		FROM users
		WHERE id = $1 AND deleted_at IS NULL
	`

	var user User
	err := r.db.GetContext(ctx, &user, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}

// GetByEmail retrieves a user by email
func (r *Repository) GetByEmail(ctx context.Context, email string) (*User, error) {
	query := `
		SELECT id, email, password_hash, display_name, role, tier, tier_expires_at, mode_preference, created_at, updated_at, deleted_at
		FROM users
		WHERE email = $1 AND deleted_at IS NULL
	`

	var user User
	err := r.db.GetContext(ctx, &user, query, email)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}

// Update updates a user
func (r *Repository) Update(ctx context.Context, id uuid.UUID, input UpdateUserInput) (*User, error) {
	query := `
		UPDATE users
		SET display_name = COALESCE($2, display_name),
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, email, password_hash, display_name, role, tier, tier_expires_at, mode_preference, created_at, updated_at, deleted_at
	`

	var user User
	err := r.db.GetContext(ctx, &user, query, id, input.DisplayName)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	return &user, nil
}

// Delete soft deletes a user
func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE users
		SET deleted_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

// EmailExists checks if an email is already registered
func (r *Repository) EmailExists(ctx context.Context, email string) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM users WHERE email = $1 AND deleted_at IS NULL
		)
	`

	var exists bool
	err := r.db.GetContext(ctx, &exists, query, email)
	if err != nil {
		return false, fmt.Errorf("failed to check email existence: %w", err)
	}

	return exists, nil
}

// UpdateTier updates user's subscription tier
func (r *Repository) UpdateTier(ctx context.Context, id uuid.UUID, tier string, expiresAt *time.Time) error {
	query := `
		UPDATE users
		SET tier = $2,
		    tier_expires_at = $3,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query, id, tier, expiresAt)
	if err != nil {
		return fmt.Errorf("failed to update tier: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

// UpdateModePreference updates user's mode preference
func (r *Repository) UpdateModePreference(ctx context.Context, id uuid.UUID, modePreference string) error {
	query := `
		UPDATE users
		SET mode_preference = $2,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query, id, modePreference)
	if err != nil {
		return fmt.Errorf("failed to update mode preference: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

// UpdateRole updates a user's role (admin operation)
// SECURITY: This should only be called by handlers protected by RequireSuperAdmin middleware
// adminID parameter is for audit logging purposes
func (r *Repository) UpdateRole(ctx context.Context, id uuid.UUID, role UserRole, adminID uuid.UUID) error {
	// Validate role is a known value
	validRoles := []UserRole{RoleUser, RoleAdmin, RoleSuperAdmin}
	valid := false
	for _, validRole := range validRoles {
		if role == validRole {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid role: %s", role)
	}

	// NOTE: adminID is logged by the middleware layer for audit trail
	query := `
		UPDATE users
		SET role = $2,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query, id, string(role))
	if err != nil {
		return fmt.Errorf("failed to update role: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}
