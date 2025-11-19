package team

import (
	"time"

	"github.com/google/uuid"
)

// Team represents a collaboration team
type Team struct {
	ID          uuid.UUID  `db:"id" json:"id"`
	Name        string     `db:"name" json:"name"`
	Description *string    `db:"description" json:"description,omitempty"`
	OwnerID     uuid.UUID  `db:"owner_id" json:"owner_id"`
	Settings    Settings   `db:"settings" json:"settings"` // JSONB
	CreatedAt   time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at" json:"updated_at"`
	DeletedAt   *time.Time `db:"deleted_at" json:"-"`
}

// Settings represents team settings stored as JSONB
type Settings struct {
	AllowPublicCheckins bool   `json:"allow_public_checkins"`
	DefaultVisibility   string `json:"default_visibility"` // public, team, private
	RequireApproval     bool   `json:"require_approval"`
	TimeZone            string `json:"timezone"`
}

// TeamMember represents a member of a team
type TeamMember struct {
	ID        uuid.UUID  `db:"id" json:"id"`
	TeamID    uuid.UUID  `db:"team_id" json:"team_id"`
	UserID    uuid.UUID  `db:"user_id" json:"user_id"`
	Role      MemberRole `db:"role" json:"role"`
	JoinedAt  time.Time  `db:"joined_at" json:"joined_at"`
	UpdatedAt time.Time  `db:"updated_at" json:"updated_at"`
	DeletedAt *time.Time `db:"deleted_at" json:"-"`
}

// MemberRole represents a team member's role
type MemberRole string

const (
	RoleOwner  MemberRole = "owner"
	RoleAdmin  MemberRole = "admin"
	RoleMember MemberRole = "member"
	RoleGuest  MemberRole = "guest"
)

// IsValid checks if a role is valid
func (r MemberRole) IsValid() bool {
	switch r {
	case RoleOwner, RoleAdmin, RoleMember, RoleGuest:
		return true
	default:
		return false
	}
}

// CanManageMembers checks if role can manage team members
func (r MemberRole) CanManageMembers() bool {
	return r == RoleOwner || r == RoleAdmin
}

// CanEditTeamSettings checks if role can edit team settings
func (r MemberRole) CanEditTeamSettings() bool {
	return r == RoleOwner || r == RoleAdmin
}

// CanViewAllCheckins checks if role can view all team checkins
func (r MemberRole) CanViewAllCheckins() bool {
	return r != RoleGuest
}
