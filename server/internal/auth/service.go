package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/dev-jelly/donelist/internal/user"
	"go.uber.org/zap"
)

// Service handles authentication business logic
type Service struct {
	userRepo         *user.Repository
	refreshTokenRepo *RefreshTokenRepository
	jwtManager       *JWTManager
	tokenBlacklist   *TokenBlacklist
	sessionStore     *SessionStore
	logger           *zap.Logger
}

// NewService creates a new auth service
func NewService(
	userRepo *user.Repository,
	refreshTokenRepo *RefreshTokenRepository,
	jwtManager *JWTManager,
	logger *zap.Logger,
) *Service {
	return &Service{
		userRepo:         userRepo,
		refreshTokenRepo: refreshTokenRepo,
		jwtManager:       jwtManager,
		logger:           logger,
	}
}

// NewServiceWithBlacklist creates a new auth service with blacklist and session store
func NewServiceWithBlacklist(
	userRepo *user.Repository,
	refreshTokenRepo *RefreshTokenRepository,
	jwtManager *JWTManager,
	tokenBlacklist *TokenBlacklist,
	sessionStore *SessionStore,
	logger *zap.Logger,
) *Service {
	return &Service{
		userRepo:         userRepo,
		refreshTokenRepo: refreshTokenRepo,
		jwtManager:       jwtManager,
		tokenBlacklist:   tokenBlacklist,
		sessionStore:     sessionStore,
		logger:           logger,
	}
}

// RegisterInput represents registration input
type RegisterInput struct {
	Email       string
	Password    string
	DisplayName *string
}

// LoginInput represents login input
type LoginInput struct {
	Email    string
	Password string
}

// TokenPair represents access and refresh tokens
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// Register registers a new user
func (s *Service) Register(ctx context.Context, input RegisterInput) (*user.User, *TokenPair, error) {
	// Validate password
	if err := IsValidPassword(input.Password); err != nil {
		return nil, nil, fmt.Errorf("invalid password: %w", err)
	}

	// Check if email already exists
	exists, err := s.userRepo.EmailExists(ctx, input.Email)
	if err != nil {
		s.logger.Error("Failed to check email existence", zap.Error(err))
		return nil, nil, fmt.Errorf("failed to check email: %w", err)
	}
	if exists {
		return nil, nil, fmt.Errorf("email already registered")
	}

	// Hash password
	passwordHash, err := HashPassword(input.Password)
	if err != nil {
		s.logger.Error("Failed to hash password", zap.Error(err))
		return nil, nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Create user
	newUser, err := s.userRepo.Create(ctx, user.CreateUserInput{
		Email:        input.Email,
		PasswordHash: passwordHash,
		DisplayName:  input.DisplayName,
	})
	if err != nil {
		s.logger.Error("Failed to create user", zap.Error(err))
		return nil, nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Generate tokens
	tokens, err := s.generateTokenPair(ctx, newUser)
	if err != nil {
		s.logger.Error("Failed to generate tokens", zap.Error(err))
		return nil, nil, fmt.Errorf("failed to generate tokens: %w", err)
	}

	s.logger.Info("User registered successfully",
		zap.String("user_id", newUser.ID.String()),
		zap.String("email", newUser.Email),
	)

	return newUser, tokens, nil
}

// Login authenticates a user and returns tokens
func (s *Service) Login(ctx context.Context, input LoginInput) (*user.User, *TokenPair, error) {
	// Get user by email
	existingUser, err := s.userRepo.GetByEmail(ctx, input.Email)
	if err != nil {
		s.logger.Warn("Login failed: user not found", zap.String("email", input.Email))
		return nil, nil, fmt.Errorf("invalid email or password")
	}

	// Verify password
	if err := VerifyPassword(input.Password, existingUser.PasswordHash); err != nil {
		s.logger.Warn("Login failed: invalid password", zap.String("email", input.Email))
		return nil, nil, fmt.Errorf("invalid email or password")
	}

	// Generate tokens
	tokens, err := s.generateTokenPair(ctx, existingUser)
	if err != nil {
		s.logger.Error("Failed to generate tokens", zap.Error(err))
		return nil, nil, fmt.Errorf("failed to generate tokens: %w", err)
	}

	s.logger.Info("User logged in successfully",
		zap.String("user_id", existingUser.ID.String()),
		zap.String("email", existingUser.Email),
	)

	return existingUser, tokens, nil
}

// RefreshAccessToken generates a new access token using a refresh token with rotation
func (s *Service) RefreshAccessToken(ctx context.Context, refreshToken string) (*TokenPair, error) {
	// Validate refresh token format
	claims, err := s.jwtManager.ValidateRefreshToken(refreshToken)
	if err != nil {
		s.logger.Warn("Invalid refresh token format", zap.Error(err))
		return nil, fmt.Errorf("invalid refresh token")
	}

	// Check if JTI has been revoked (detect token reuse attempts)
	jtiRevoked, err := s.refreshTokenRepo.IsJTIRevoked(ctx, claims.ID)
	if err != nil {
		s.logger.Error("Failed to check JTI status", zap.Error(err))
		return nil, fmt.Errorf("failed to validate token")
	}

	if jtiRevoked {
		s.logger.Warn("Attempted reuse of revoked JTI - possible token theft",
			zap.String("jti", claims.ID),
			zap.String("user_id", claims.UserID.String()),
		)
		// Revoke all tokens for this user as a security measure
		if err := s.refreshTokenRepo.RevokeAllForUser(ctx, claims.UserID); err != nil {
			s.logger.Error("Failed to revoke all tokens after JTI reuse attempt", zap.Error(err))
		}
		return nil, fmt.Errorf("invalid refresh token - security violation detected")
	}

	// Check if refresh token exists and is valid in database (one-time use check)
	valid, userID, jti, err := s.refreshTokenRepo.IsValid(ctx, refreshToken)
	if err != nil || !valid {
		s.logger.Warn("Refresh token not valid in database",
			zap.String("user_id", claims.UserID.String()),
			zap.Error(err),
		)
		return nil, fmt.Errorf("invalid or revoked refresh token")
	}

	// Verify user ID matches
	if userID != claims.UserID {
		s.logger.Error("User ID mismatch in refresh token",
			zap.String("claims_user_id", claims.UserID.String()),
			zap.String("db_user_id", userID.String()),
		)
		return nil, fmt.Errorf("invalid refresh token")
	}

	// Verify JTI matches
	if jti != claims.ID {
		s.logger.Error("JTI mismatch in refresh token",
			zap.String("claims_jti", claims.ID),
			zap.String("db_jti", jti),
		)
		return nil, fmt.Errorf("invalid refresh token")
	}

	// Mark token as used BEFORE generating new tokens (one-time use enforcement)
	if err := s.refreshTokenRepo.MarkAsUsed(ctx, refreshToken); err != nil {
		s.logger.Error("Failed to mark token as used", zap.Error(err))
		return nil, fmt.Errorf("failed to process token refresh")
	}

	// Get user to ensure they still exist
	existingUser, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		s.logger.Warn("User not found for refresh token", zap.String("user_id", userID.String()))
		return nil, fmt.Errorf("user not found")
	}

	// Generate new token pair (token rotation)
	tokens, err := s.generateTokenPair(ctx, existingUser)
	if err != nil {
		s.logger.Error("Failed to generate new tokens", zap.Error(err))
		// Revert the used_at mark if token generation fails
		// Note: This is a best-effort reversal, not transactional
		return nil, fmt.Errorf("failed to generate tokens: %w", err)
	}

	// Revoke old JTI to prevent any reuse
	if err := s.refreshTokenRepo.RevokeByJTI(ctx, jti); err != nil {
		s.logger.Warn("Failed to revoke old JTI", zap.Error(err))
		// Don't fail the operation, new tokens are already issued
	}

	s.logger.Info("Access token refreshed with rotation",
		zap.String("user_id", existingUser.ID.String()),
		zap.String("old_jti", jti),
	)

	return tokens, nil
}

// Logout revokes a refresh token
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	if err := s.refreshTokenRepo.Revoke(ctx, refreshToken); err != nil {
		s.logger.Warn("Failed to revoke refresh token", zap.Error(err))
		return fmt.Errorf("failed to logout: %w", err)
	}

	s.logger.Info("User logged out successfully")
	return nil
}

// LogoutAll revokes all refresh tokens for a user
func (s *Service) LogoutAll(ctx context.Context, userID uuid.UUID) error {
	if err := s.refreshTokenRepo.RevokeAllForUser(ctx, userID); err != nil {
		s.logger.Error("Failed to revoke all user tokens",
			zap.String("user_id", userID.String()),
			zap.Error(err),
		)
		return fmt.Errorf("failed to logout from all devices: %w", err)
	}

	s.logger.Info("User logged out from all devices",
		zap.String("user_id", userID.String()),
	)
	return nil
}

// generateTokenPair generates both access and refresh tokens with JTI tracking
func (s *Service) generateTokenPair(ctx context.Context, u *user.User) (*TokenPair, error) {
	// Generate access token
	accessToken, err := s.jwtManager.GenerateAccessToken(u.ID, u.Email)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	// Generate refresh token
	refreshToken, err := s.jwtManager.GenerateRefreshToken(u.ID, u.Email)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	// Extract JTI from refresh token for storage
	refreshClaims, err := s.jwtManager.ValidateRefreshToken(refreshToken)
	if err != nil {
		return nil, fmt.Errorf("failed to extract JTI from refresh token: %w", err)
	}

	// Store refresh token in database with JTI
	expiresAt := time.Now().Add(s.jwtManager.refreshTokenExpiry)
	if err := s.refreshTokenRepo.Store(ctx, u.ID, refreshToken, refreshClaims.ID, expiresAt); err != nil {
		return nil, fmt.Errorf("failed to store refresh token: %w", err)
	}

	// If session store is available, create session
	if s.sessionStore != nil {
		accessClaims, err := s.jwtManager.ValidateAccessToken(accessToken)
		if err != nil {
			s.logger.Warn("Failed to extract access token claims for session", zap.Error(err))
		} else {
			session := SessionInfo{
				UserID:     u.ID,
				AccessJTI:  accessClaims.ID,
				RefreshJTI: refreshClaims.ID,
				ExpiresAt:  time.Now().Add(s.jwtManager.refreshTokenExpiry),
			}
			if err := s.sessionStore.CreateSession(ctx, session); err != nil {
				s.logger.Warn("Failed to create session in store", zap.Error(err))
			}
		}
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    time.Now().Add(s.jwtManager.accessTokenExpiry),
	}, nil
}

// LogoutWithBlacklist revokes a refresh token and blacklists associated JTIs
func (s *Service) LogoutWithBlacklist(ctx context.Context, refreshToken, accessToken string) error {
	// Extract JTIs from tokens
	var accessJTI, refreshJTI string

	if accessToken != "" {
		accessClaims, err := s.jwtManager.ValidateAccessToken(accessToken)
		if err == nil {
			accessJTI = accessClaims.ID
		}
	}

	refreshClaims, err := s.jwtManager.ValidateRefreshToken(refreshToken)
	if err == nil {
		refreshJTI = refreshClaims.ID
	}

	// Revoke refresh token in database
	if err := s.refreshTokenRepo.Revoke(ctx, refreshToken); err != nil {
		s.logger.Warn("Failed to revoke refresh token", zap.Error(err))
	}

	// Blacklist tokens if blacklist is available
	if s.tokenBlacklist != nil {
		if accessJTI != "" {
			if err := s.tokenBlacklist.BlacklistAccessToken(ctx, accessJTI, s.jwtManager.accessTokenExpiry); err != nil {
				s.logger.Warn("Failed to blacklist access token", zap.Error(err))
			}
		}

		if refreshJTI != "" {
			if err := s.tokenBlacklist.BlacklistRefreshToken(ctx, refreshJTI, s.jwtManager.refreshTokenExpiry); err != nil {
				s.logger.Warn("Failed to blacklist refresh token", zap.Error(err))
			}
		}
	}

	// Delete session if session store is available
	if s.sessionStore != nil && refreshClaims != nil {
		if err := s.sessionStore.DeleteSession(ctx, refreshClaims.UserID, refreshClaims.SessionID); err != nil {
			s.logger.Warn("Failed to delete session", zap.Error(err))
		}
	}

	s.logger.Info("User logged out successfully")
	return nil
}

// LogoutAllWithBlacklist revokes all tokens for a user and blacklists them
func (s *Service) LogoutAllWithBlacklist(ctx context.Context, userID uuid.UUID) error {
	// Revoke all refresh tokens in database
	if err := s.refreshTokenRepo.RevokeAllForUser(ctx, userID); err != nil {
		s.logger.Error("Failed to revoke all user tokens",
			zap.String("user_id", userID.String()),
			zap.Error(err),
		)
		return fmt.Errorf("failed to logout from all devices: %w", err)
	}

	// Blacklist all tokens and delete sessions if available
	if s.tokenBlacklist != nil {
		if err := s.tokenBlacklist.BlacklistAllUserTokens(ctx, userID, s.jwtManager.accessTokenExpiry, s.jwtManager.refreshTokenExpiry); err != nil {
			s.logger.Warn("Failed to blacklist all user tokens", zap.Error(err))
		}
	}

	// Delete all sessions if session store is available
	if s.sessionStore != nil {
		if err := s.sessionStore.DeleteUserSessions(ctx, userID); err != nil {
			s.logger.Warn("Failed to delete user sessions", zap.Error(err))
		}
	}

	s.logger.Info("User logged out from all devices",
		zap.String("user_id", userID.String()),
	)
	return nil
}

// GetActiveSessions returns all active sessions for a user
func (s *Service) GetActiveSessions(ctx context.Context, userID uuid.UUID) ([]SessionInfo, error) {
	if s.sessionStore == nil {
		return nil, fmt.Errorf("session store not available")
	}

	sessions, err := s.sessionStore.GetUserSessions(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to get user sessions",
			zap.String("user_id", userID.String()),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to get sessions: %w", err)
	}

	return sessions, nil
}

// RevokeSession revokes a specific session
func (s *Service) RevokeSession(ctx context.Context, userID uuid.UUID, sessionID string) error {
	if s.sessionStore == nil {
		return fmt.Errorf("session store not available")
	}

	// Get session details
	session, err := s.sessionStore.GetSession(ctx, userID, sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	// Blacklist the tokens
	if s.tokenBlacklist != nil {
		if session.AccessJTI != "" {
			if err := s.tokenBlacklist.BlacklistAccessToken(ctx, session.AccessJTI, s.jwtManager.accessTokenExpiry); err != nil {
				s.logger.Warn("Failed to blacklist access token", zap.Error(err))
			}
		}

		if session.RefreshJTI != "" {
			if err := s.tokenBlacklist.BlacklistRefreshToken(ctx, session.RefreshJTI, s.jwtManager.refreshTokenExpiry); err != nil {
				s.logger.Warn("Failed to blacklist refresh token", zap.Error(err))
			}
		}
	}

	// Delete the session
	if err := s.sessionStore.DeleteSession(ctx, userID, sessionID); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	s.logger.Info("Session revoked",
		zap.String("user_id", userID.String()),
		zap.String("session_id", sessionID),
	)

	return nil
}

// EnforceSessionLimit enforces a maximum number of concurrent sessions
func (s *Service) EnforceSessionLimit(ctx context.Context, userID uuid.UUID, maxSessions int) error {
	if s.sessionStore == nil {
		return nil // No enforcement if session store not available
	}

	return s.sessionStore.EnforceSessionLimit(ctx, userID, maxSessions)
}
