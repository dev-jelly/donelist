package team

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// Repository handles team data persistence
type Repository struct {
	db     *sqlx.DB
	logger *zap.Logger
}

// NewRepository creates a new team repository
func NewRepository(db *sqlx.DB, logger *zap.Logger) *Repository {
	return &Repository{
		db:     db,
		logger: logger,
	}
}

// CreateTeamInput represents input for creating a team
type CreateTeamInput struct {
	Name        string
	Description *string
	OwnerID     uuid.UUID
	Settings    Settings
}

// UpdateTeamInput represents input for updating a team
type UpdateTeamInput struct {
	Name        *string
	Description *string
	Settings    *Settings
}

// CreateTeam creates a new team
func (r *Repository) CreateTeam(ctx context.Context, input CreateTeamInput) (*Team, error) {
	settingsJSON, err := json.Marshal(input.Settings)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal settings: %w", err)
	}

	query := `
		INSERT INTO teams (name, description, owner_id, settings, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		RETURNING id, name, description, owner_id, settings, created_at, updated_at, deleted_at
	`

	var team Team
	var settingsStr string
	err = r.db.QueryRowContext(
		ctx,
		query,
		input.Name,
		input.Description,
		input.OwnerID,
		settingsJSON,
	).Scan(
		&team.ID,
		&team.Name,
		&team.Description,
		&team.OwnerID,
		&settingsStr,
		&team.CreatedAt,
		&team.UpdatedAt,
		&team.DeletedAt,
	)
	if err != nil {
		r.logger.Error("Failed to create team", zap.Error(err))
		return nil, fmt.Errorf("failed to create team: %w", err)
	}

	if err := json.Unmarshal([]byte(settingsStr), &team.Settings); err != nil {
		return nil, fmt.Errorf("failed to unmarshal settings: %w", err)
	}

	r.logger.Info("Team created",
		zap.String("team_id", team.ID.String()),
		zap.String("name", team.Name),
		zap.String("owner_id", input.OwnerID.String()),
	)

	return &team, nil
}

// GetTeamByID retrieves a team by ID
func (r *Repository) GetTeamByID(ctx context.Context, id uuid.UUID) (*Team, error) {
	query := `
		SELECT id, name, description, owner_id, settings, created_at, updated_at, deleted_at
		FROM teams
		WHERE id = $1 AND deleted_at IS NULL
	`

	var team Team
	var settingsStr string
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&team.ID,
		&team.Name,
		&team.Description,
		&team.OwnerID,
		&settingsStr,
		&team.CreatedAt,
		&team.UpdatedAt,
		&team.DeletedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("team not found")
	}
	if err != nil {
		r.logger.Error("Failed to get team", zap.Error(err), zap.String("team_id", id.String()))
		return nil, fmt.Errorf("failed to get team: %w", err)
	}

	if err := json.Unmarshal([]byte(settingsStr), &team.Settings); err != nil {
		return nil, fmt.Errorf("failed to unmarshal settings: %w", err)
	}

	return &team, nil
}

// UpdateTeam updates a team
func (r *Repository) UpdateTeam(ctx context.Context, id uuid.UUID, input UpdateTeamInput) (*Team, error) {
	// Build dynamic update query
	updates := []string{}
	args := []interface{}{}
	argCount := 1

	if input.Name != nil {
		updates = append(updates, fmt.Sprintf("name = $%d", argCount))
		args = append(args, *input.Name)
		argCount++
	}

	if input.Description != nil {
		updates = append(updates, fmt.Sprintf("description = $%d", argCount))
		args = append(args, *input.Description)
		argCount++
	}

	if input.Settings != nil {
		settingsJSON, err := json.Marshal(input.Settings)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal settings: %w", err)
		}
		updates = append(updates, fmt.Sprintf("settings = $%d", argCount))
		args = append(args, settingsJSON)
		argCount++
	}

	if len(updates) == 0 {
		return r.GetTeamByID(ctx, id)
	}

	// Add updated_at
	updates = append(updates, "updated_at = NOW()")

	// Add team ID as last parameter
	args = append(args, id)

	query := fmt.Sprintf(`
		UPDATE teams
		SET %s
		WHERE id = $%d AND deleted_at IS NULL
		RETURNING id, name, description, owner_id, settings, created_at, updated_at, deleted_at
	`, string(updates[0]), argCount)

	for i := 1; i < len(updates); i++ {
		query = fmt.Sprintf(`
			UPDATE teams
			SET %s
			WHERE id = $%d AND deleted_at IS NULL
			RETURNING id, name, description, owner_id, settings, created_at, updated_at, deleted_at
		`, updates[0]+", "+updates[i], argCount)
	}

	var team Team
	var settingsStr string
	err := r.db.QueryRowContext(ctx, query, args...).Scan(
		&team.ID,
		&team.Name,
		&team.Description,
		&team.OwnerID,
		&settingsStr,
		&team.CreatedAt,
		&team.UpdatedAt,
		&team.DeletedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("team not found")
	}
	if err != nil {
		r.logger.Error("Failed to update team", zap.Error(err), zap.String("team_id", id.String()))
		return nil, fmt.Errorf("failed to update team: %w", err)
	}

	if err := json.Unmarshal([]byte(settingsStr), &team.Settings); err != nil {
		return nil, fmt.Errorf("failed to unmarshal settings: %w", err)
	}

	r.logger.Info("Team updated", zap.String("team_id", team.ID.String()))

	return &team, nil
}

// DeleteTeam soft deletes a team
func (r *Repository) DeleteTeam(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE teams
		SET deleted_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		r.logger.Error("Failed to delete team", zap.Error(err), zap.String("team_id", id.String()))
		return fmt.Errorf("failed to delete team: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("team not found")
	}

	r.logger.Info("Team deleted", zap.String("team_id", id.String()))

	return nil
}

// GetUserTeams retrieves all teams for a user
func (r *Repository) GetUserTeams(ctx context.Context, userID uuid.UUID) ([]Team, error) {
	query := `
		SELECT DISTINCT t.id, t.name, t.description, t.owner_id, t.settings, t.created_at, t.updated_at, t.deleted_at
		FROM teams t
		LEFT JOIN team_members tm ON t.id = tm.team_id
		WHERE (t.owner_id = $1 OR tm.user_id = $1)
		AND t.deleted_at IS NULL
		AND (tm.deleted_at IS NULL OR tm.deleted_at IS NULL)
		ORDER BY t.created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		r.logger.Error("Failed to get user teams", zap.Error(err), zap.String("user_id", userID.String()))
		return nil, fmt.Errorf("failed to get user teams: %w", err)
	}
	defer rows.Close()

	teams := []Team{}
	for rows.Next() {
		var team Team
		var settingsStr string
		err := rows.Scan(
			&team.ID,
			&team.Name,
			&team.Description,
			&team.OwnerID,
			&settingsStr,
			&team.CreatedAt,
			&team.UpdatedAt,
			&team.DeletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan team: %w", err)
		}

		if err := json.Unmarshal([]byte(settingsStr), &team.Settings); err != nil {
			return nil, fmt.Errorf("failed to unmarshal settings: %w", err)
		}

		teams = append(teams, team)
	}

	return teams, nil
}

// AddMember adds a member to a team
func (r *Repository) AddMember(ctx context.Context, teamID, userID uuid.UUID, role MemberRole) (*TeamMember, error) {
	query := `
		INSERT INTO team_members (team_id, user_id, role, joined_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		RETURNING id, team_id, user_id, role, joined_at, updated_at, deleted_at
	`

	var member TeamMember
	err := r.db.QueryRowContext(ctx, query, teamID, userID, role).Scan(
		&member.ID,
		&member.TeamID,
		&member.UserID,
		&member.Role,
		&member.JoinedAt,
		&member.UpdatedAt,
		&member.DeletedAt,
	)
	if err != nil {
		r.logger.Error("Failed to add team member",
			zap.Error(err),
			zap.String("team_id", teamID.String()),
			zap.String("user_id", userID.String()),
		)
		return nil, fmt.Errorf("failed to add team member: %w", err)
	}

	r.logger.Info("Team member added",
		zap.String("team_id", teamID.String()),
		zap.String("user_id", userID.String()),
		zap.String("role", string(role)),
	)

	return &member, nil
}

// RemoveMember soft deletes a team member
func (r *Repository) RemoveMember(ctx context.Context, teamID, userID uuid.UUID) error {
	query := `
		UPDATE team_members
		SET deleted_at = NOW()
		WHERE team_id = $1 AND user_id = $2 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query, teamID, userID)
	if err != nil {
		r.logger.Error("Failed to remove team member",
			zap.Error(err),
			zap.String("team_id", teamID.String()),
			zap.String("user_id", userID.String()),
		)
		return fmt.Errorf("failed to remove team member: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("team member not found")
	}

	r.logger.Info("Team member removed",
		zap.String("team_id", teamID.String()),
		zap.String("user_id", userID.String()),
	)

	return nil
}

// UpdateMemberRole updates a team member's role
func (r *Repository) UpdateMemberRole(ctx context.Context, teamID, userID uuid.UUID, role MemberRole) (*TeamMember, error) {
	query := `
		UPDATE team_members
		SET role = $1, updated_at = NOW()
		WHERE team_id = $2 AND user_id = $3 AND deleted_at IS NULL
		RETURNING id, team_id, user_id, role, joined_at, updated_at, deleted_at
	`

	var member TeamMember
	err := r.db.QueryRowContext(ctx, query, role, teamID, userID).Scan(
		&member.ID,
		&member.TeamID,
		&member.UserID,
		&member.Role,
		&member.JoinedAt,
		&member.UpdatedAt,
		&member.DeletedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("team member not found")
	}
	if err != nil {
		r.logger.Error("Failed to update member role",
			zap.Error(err),
			zap.String("team_id", teamID.String()),
			zap.String("user_id", userID.String()),
		)
		return nil, fmt.Errorf("failed to update member role: %w", err)
	}

	r.logger.Info("Member role updated",
		zap.String("team_id", teamID.String()),
		zap.String("user_id", userID.String()),
		zap.String("role", string(role)),
	)

	return &member, nil
}

// GetTeamMember retrieves a specific team member
func (r *Repository) GetTeamMember(ctx context.Context, teamID, userID uuid.UUID) (*TeamMember, error) {
	query := `
		SELECT id, team_id, user_id, role, joined_at, updated_at, deleted_at
		FROM team_members
		WHERE team_id = $1 AND user_id = $2 AND deleted_at IS NULL
	`

	var member TeamMember
	err := r.db.QueryRowContext(ctx, query, teamID, userID).Scan(
		&member.ID,
		&member.TeamID,
		&member.UserID,
		&member.Role,
		&member.JoinedAt,
		&member.UpdatedAt,
		&member.DeletedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("team member not found")
	}
	if err != nil {
		r.logger.Error("Failed to get team member",
			zap.Error(err),
			zap.String("team_id", teamID.String()),
			zap.String("user_id", userID.String()),
		)
		return nil, fmt.Errorf("failed to get team member: %w", err)
	}

	return &member, nil
}

// GetTeamMembers retrieves all members of a team
func (r *Repository) GetTeamMembers(ctx context.Context, teamID uuid.UUID) ([]TeamMember, error) {
	query := `
		SELECT id, team_id, user_id, role, joined_at, updated_at, deleted_at
		FROM team_members
		WHERE team_id = $1 AND deleted_at IS NULL
		ORDER BY joined_at ASC
	`

	rows, err := r.db.QueryContext(ctx, query, teamID)
	if err != nil {
		r.logger.Error("Failed to get team members", zap.Error(err), zap.String("team_id", teamID.String()))
		return nil, fmt.Errorf("failed to get team members: %w", err)
	}
	defer rows.Close()

	members := []TeamMember{}
	for rows.Next() {
		var member TeamMember
		err := rows.Scan(
			&member.ID,
			&member.TeamID,
			&member.UserID,
			&member.Role,
			&member.JoinedAt,
			&member.UpdatedAt,
			&member.DeletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan team member: %w", err)
		}
		members = append(members, member)
	}

	return members, nil
}

// IsTeamOwner checks if a user is the owner of a team
func (r *Repository) IsTeamOwner(ctx context.Context, teamID, userID uuid.UUID) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM teams
			WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL
		)
	`

	var exists bool
	err := r.db.QueryRowContext(ctx, query, teamID, userID).Scan(&exists)
	if err != nil {
		r.logger.Error("Failed to check team ownership",
			zap.Error(err),
			zap.String("team_id", teamID.String()),
			zap.String("user_id", userID.String()),
		)
		return false, fmt.Errorf("failed to check team ownership: %w", err)
	}

	return exists, nil
}

// ActiveTeamMember represents an active team member with user details
type ActiveTeamMember struct {
	ID          uuid.UUID  `db:"id" json:"id"`
	TeamID      uuid.UUID  `db:"team_id" json:"team_id"`
	UserID      uuid.UUID  `db:"user_id" json:"user_id"`
	Role        MemberRole `db:"role" json:"role"`
	JoinedAt    time.Time  `db:"joined_at" json:"joined_at"`
	Email       string     `db:"email" json:"email"`
	DisplayName string     `db:"display_name" json:"display_name"`
}

// GetActiveTeamMembers retrieves active team members with user details
func (r *Repository) GetActiveTeamMembers(ctx context.Context, teamID uuid.UUID) ([]ActiveTeamMember, error) {
	query := `
		SELECT id, team_id, user_id, role, joined_at, email, display_name
		FROM active_team_members
		WHERE team_id = $1
		ORDER BY joined_at ASC
	`

	rows, err := r.db.QueryContext(ctx, query, teamID)
	if err != nil {
		r.logger.Error("Failed to get active team members", zap.Error(err), zap.String("team_id", teamID.String()))
		return nil, fmt.Errorf("failed to get active team members: %w", err)
	}
	defer rows.Close()

	members := []ActiveTeamMember{}
	for rows.Next() {
		var member ActiveTeamMember
		err := rows.Scan(
			&member.ID,
			&member.TeamID,
			&member.UserID,
			&member.Role,
			&member.JoinedAt,
			&member.Email,
			&member.DisplayName,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan active team member: %w", err)
		}
		members = append(members, member)
	}

	return members, nil
}
