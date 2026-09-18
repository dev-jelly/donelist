package websocket

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// AuthConfig contains authentication configuration
type AuthConfig struct {
	// JWTSecret is the secret key for JWT validation
	JWTSecret string

	// JWTIssuer is the expected issuer of the JWT
	JWTIssuer string

	// JWTAudience is the expected audience of the JWT
	JWTAudience string

	// TokenHeader is the header containing the JWT token (default: Authorization)
	TokenHeader string

	// RequireAuth indicates if authentication is required
	RequireAuth bool

	// AllowAnonymous allows anonymous connections (with limited permissions)
	AllowAnonymous bool

	// SessionTimeout is the maximum session duration
	SessionTimeout time.Duration
}

// DefaultAuthConfig returns default authentication configuration
func DefaultAuthConfig() AuthConfig {
	return AuthConfig{
		TokenHeader:    "Authorization",
		RequireAuth:    true,
		AllowAnonymous: false,
		SessionTimeout: 24 * time.Hour,
	}
}

// RateLimitConfig contains rate limiting configuration
type RateLimitConfig struct {
	// RequestsPerSecond is the rate limit per second
	RequestsPerSecond float64

	// BurstSize is the maximum burst size
	BurstSize int

	// PerUserLimits enables per-user rate limiting
	PerUserLimits bool

	// GlobalLimit is the global rate limit (if PerUserLimits is false)
	GlobalLimit *rate.Limiter

	// MessageSizeLimit is the maximum message size in bytes
	MessageSizeLimit int64

	// MaxConnectionsPerUser is the maximum concurrent connections per user
	MaxConnectionsPerUser int
}

// DefaultRateLimitConfig returns default rate limit configuration
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		RequestsPerSecond:     10,
		BurstSize:             20,
		PerUserLimits:         true,
		MessageSizeLimit:      1024 * 1024, // 1MB
		MaxConnectionsPerUser: 5,
	}
}

// AuthManager handles WebSocket authentication and authorization
type AuthManager struct {
	config AuthConfig
	logger *zap.Logger
}

// NewAuthManager creates a new authentication manager
func NewAuthManager(config AuthConfig, logger *zap.Logger) *AuthManager {
	if config.TokenHeader == "" {
		config.TokenHeader = "Authorization"
	}

	return &AuthManager{
		config: config,
		logger: logger,
	}
}

// Claims represents JWT claims for WebSocket connections
type Claims struct {
	jwt.RegisteredClaims
	UserID      string   `json:"user_id"`
	Email       string   `json:"email"`
	Roles       []string `json:"roles,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
}

// AuthenticateRequest authenticates a WebSocket upgrade request
func (am *AuthManager) AuthenticateRequest(r *http.Request) (*Claims, error) {
	// Extract token from header or query parameter
	token := am.extractToken(r)
	if token == "" {
		if am.config.AllowAnonymous {
			return am.createAnonymousClaims(), nil
		}
		return nil, fmt.Errorf("no authentication token provided")
	}

	// Parse and validate JWT
	claims, err := am.validateJWT(token)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	// Additional validation
	if err := am.validateClaims(claims); err != nil {
		return nil, fmt.Errorf("invalid claims: %w", err)
	}

	am.logger.Debug("WebSocket authentication successful",
		zap.String("user_id", claims.UserID),
		zap.String("email", claims.Email),
	)

	return claims, nil
}

// extractToken extracts the JWT token from the request
func (am *AuthManager) extractToken(r *http.Request) string {
	// Try header first
	authHeader := r.Header.Get(am.config.TokenHeader)
	if authHeader != "" {
		// Remove "Bearer " prefix if present
		if strings.HasPrefix(authHeader, "Bearer ") {
			return strings.TrimPrefix(authHeader, "Bearer ")
		}
		return authHeader
	}

	// Try query parameter as fallback (for WebSocket connections from browsers)
	return r.URL.Query().Get("token")
}

// validateJWT validates and parses a JWT token
func (am *AuthManager) validateJWT(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(am.config.JWTSecret), nil
	})

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}

// validateClaims performs additional claim validation
func (am *AuthManager) validateClaims(claims *Claims) error {
	// Check expiration
	if claims.ExpiresAt != nil && claims.ExpiresAt.Before(time.Now()) {
		return fmt.Errorf("token expired")
	}

	// Check not before
	if claims.NotBefore != nil && claims.NotBefore.After(time.Now()) {
		return fmt.Errorf("token not yet valid")
	}

	// Check issuer if configured
	if am.config.JWTIssuer != "" && claims.Issuer != am.config.JWTIssuer {
		return fmt.Errorf("invalid issuer: %s", claims.Issuer)
	}

	// Check audience if configured
	if am.config.JWTAudience != "" {
		audienceValid := false
		for _, aud := range claims.Audience {
			if aud == am.config.JWTAudience {
				audienceValid = true
				break
			}
		}
		if !audienceValid {
			return fmt.Errorf("invalid audience")
		}
	}

	// Check required fields
	if claims.UserID == "" {
		return fmt.Errorf("user_id is required")
	}

	return nil
}

// createAnonymousClaims creates claims for anonymous users
func (am *AuthManager) createAnonymousClaims() *Claims {
	return &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		},
		UserID:      fmt.Sprintf("anonymous_%d", time.Now().UnixNano()),
		Permissions: []string{"read"}, // Limited permissions for anonymous users
	}
}

// HasPermission checks if the claims have a specific permission
func (c *Claims) HasPermission(permission string) bool {
	for _, p := range c.Permissions {
		if p == permission || p == "*" { // "*" is a wildcard for all permissions
			return true
		}
	}
	return false
}

// HasRole checks if the claims have a specific role
func (c *Claims) HasRole(role string) bool {
	for _, r := range c.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// IsAnonymous checks if the user is anonymous
func (c *Claims) IsAnonymous() bool {
	return strings.HasPrefix(c.UserID, "anonymous_")
}

// RateLimiter manages rate limiting for WebSocket connections
type RateLimiter struct {
	config      RateLimitConfig
	userLimiters map[string]*rate.Limiter
	logger      *zap.Logger
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(config RateLimitConfig, logger *zap.Logger) *RateLimiter {
	rl := &RateLimiter{
		config: config,
		logger: logger,
	}

	if config.PerUserLimits {
		rl.userLimiters = make(map[string]*rate.Limiter)
	} else {
		rl.config.GlobalLimit = rate.NewLimiter(rate.Limit(config.RequestsPerSecond), config.BurstSize)
	}

	return rl
}

// CheckLimit checks if a request is within rate limits
func (rl *RateLimiter) CheckLimit(userID string) error {
	if rl.config.PerUserLimits {
		limiter := rl.getUserLimiter(userID)
		if !limiter.Allow() {
			rl.logger.Warn("Rate limit exceeded",
				zap.String("user_id", userID),
			)
			return fmt.Errorf("rate limit exceeded for user %s", userID)
		}
	} else {
		if !rl.config.GlobalLimit.Allow() {
			rl.logger.Warn("Global rate limit exceeded")
			return fmt.Errorf("global rate limit exceeded")
		}
	}
	return nil
}

// getUserLimiter gets or creates a rate limiter for a user
func (rl *RateLimiter) getUserLimiter(userID string) *rate.Limiter {
	// This should be protected by a mutex in production
	if limiter, exists := rl.userLimiters[userID]; exists {
		return limiter
	}

	limiter := rate.NewLimiter(rate.Limit(rl.config.RequestsPerSecond), rl.config.BurstSize)
	rl.userLimiters[userID] = limiter
	return limiter
}

// CheckMessageSize checks if a message is within size limits
func (rl *RateLimiter) CheckMessageSize(size int64) error {
	if size > rl.config.MessageSizeLimit {
		return fmt.Errorf("message size %d exceeds limit %d", size, rl.config.MessageSizeLimit)
	}
	return nil
}

// AuthMiddleware is a middleware for WebSocket authentication
func AuthMiddleware(authManager *AuthManager, upgrader *websocket.Upgrader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Authenticate the request
		claims, err := authManager.AuthenticateRequest(r)
		if err != nil {
			if authManager.config.RequireAuth && !authManager.config.AllowAnonymous {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}

		// Store claims in context for later use
		ctx := context.WithValue(r.Context(), "claims", claims)
		r = r.WithContext(ctx)

		// Proceed with WebSocket upgrade
		// The actual upgrade should be handled by the next handler
	}
}

// SessionProtection provides session-level security
type SessionProtection struct {
	maxIdleTime    time.Duration
	maxSessionTime time.Duration
	logger         *zap.Logger
}

// NewSessionProtection creates a new session protection instance
func NewSessionProtection(maxIdleTime, maxSessionTime time.Duration, logger *zap.Logger) *SessionProtection {
	return &SessionProtection{
		maxIdleTime:    maxIdleTime,
		maxSessionTime: maxSessionTime,
		logger:         logger,
	}
}

// ValidateSession checks if a session is still valid
func (sp *SessionProtection) ValidateSession(sessionStart, lastActivity time.Time) error {
	now := time.Now()

	// Check max session time
	if now.Sub(sessionStart) > sp.maxSessionTime {
		return fmt.Errorf("session expired: exceeded maximum session time")
	}

	// Check idle time
	if now.Sub(lastActivity) > sp.maxIdleTime {
		return fmt.Errorf("session expired: exceeded maximum idle time")
	}

	return nil
}