package mode

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Service handles mode detection and switching
type Service struct {
	logger *zap.Logger
}

// NewService creates a new mode service
func NewService(logger *zap.Logger) *Service {
	return &Service{
		logger: logger,
	}
}

// ModeContext represents the user's current mode context
type ModeContext struct {
	UserID      uuid.UUID
	CurrentMode Mode
	TeamID      *uuid.UUID // Only set when in team mode
}

// SwitchModeRequest represents a request to switch modes
type SwitchModeRequest struct {
	UserID  uuid.UUID
	NewMode Mode
	TeamID  *uuid.UUID // Required when switching to team mode
}

// SwitchMode switches a user's operational mode
func (s *Service) SwitchMode(ctx context.Context, req SwitchModeRequest) (*ModeContext, error) {
	// Validate mode
	if !req.NewMode.IsValid() {
		return nil, fmt.Errorf("invalid mode: %s", req.NewMode)
	}

	// Validate team mode requirements
	if req.NewMode == ModeTeam && req.TeamID == nil {
		return nil, fmt.Errorf("team_id is required when switching to team mode")
	}

	// Personal mode should not have team ID
	if req.NewMode == ModePersonal && req.TeamID != nil {
		s.logger.Warn("Team ID provided for personal mode, ignoring",
			zap.String("user_id", req.UserID.String()),
		)
		req.TeamID = nil
	}

	// Create mode context
	modeCtx := &ModeContext{
		UserID:      req.UserID,
		CurrentMode: req.NewMode,
		TeamID:      req.TeamID,
	}

	s.logger.Info("Mode switched",
		zap.String("user_id", req.UserID.String()),
		zap.String("mode", string(req.NewMode)),
		zap.Any("team_id", req.TeamID),
	)

	return modeCtx, nil
}

// GetModeFromContext extracts mode from context
func (s *Service) GetModeFromContext(ctx context.Context) Mode {
	if mode, ok := ctx.Value("mode").(Mode); ok {
		return mode
	}
	return DefaultMode()
}

// GetTeamIDFromContext extracts team ID from context
func (s *Service) GetTeamIDFromContext(ctx context.Context) *uuid.UUID {
	if teamID, ok := ctx.Value("team_id").(*uuid.UUID); ok {
		return teamID
	}
	return nil
}

// IsTeamMode checks if current context is in team mode
func (s *Service) IsTeamMode(ctx context.Context) bool {
	return s.GetModeFromContext(ctx) == ModeTeam
}

// IsPersonalMode checks if current context is in personal mode
func (s *Service) IsPersonalMode(ctx context.Context) bool {
	return s.GetModeFromContext(ctx) == ModePersonal
}

// ValidateModeAccess validates if user has access to the requested mode
func (s *Service) ValidateModeAccess(ctx context.Context, userID uuid.UUID, requestedMode Mode) error {
	// For now, all users have access to personal mode
	if requestedMode == ModePersonal {
		return nil
	}

	// Team mode requires additional validation
	if requestedMode == ModeTeam {
		teamID := s.GetTeamIDFromContext(ctx)
		if teamID == nil {
			return fmt.Errorf("no team context available for team mode")
		}

		// TODO: Validate user is member of the team
		// This will be implemented when we add team membership management

		return nil
	}

	return fmt.Errorf("invalid mode: %s", requestedMode)
}
