package auth

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// TokenType represents the type of JWT token
type TokenType string

const (
	AccessToken  TokenType = "access"
	RefreshToken TokenType = "refresh"
)

// SigningMethod represents the JWT signing algorithm
type SigningMethod string

const (
	SigningMethodHS256 SigningMethod = "HS256"
	SigningMethodRS256 SigningMethod = "RS256"
)

// Claims represents JWT claims with standard fields
type Claims struct {
	UserID    uuid.UUID `json:"sub"` // Standard 'sub' claim for user ID
	Email     string    `json:"email"`
	Type      TokenType `json:"type"`
	SessionID string    `json:"sid,omitempty"` // Session ID for tracking
	jwt.RegisteredClaims
}

// JWTManager handles JWT token operations
type JWTManager struct {
	signingMethod       SigningMethod
	secret              []byte
	privateKey          *rsa.PrivateKey
	publicKey           *rsa.PublicKey
	accessTokenExpiry   time.Duration
	refreshTokenExpiry  time.Duration
	clockSkewTolerance  time.Duration // Clock drift tolerance
}

// JWTConfig represents JWT configuration options
type JWTConfig struct {
	SigningMethod      SigningMethod
	Secret             string
	PrivateKeyPEM      string
	PublicKeyPEM       string
	AccessExpiry       time.Duration
	RefreshExpiry      time.Duration
	ClockSkewTolerance time.Duration
}

// NewJWTManager creates a new JWT manager with support for multiple signing methods
func NewJWTManager(config JWTConfig) (*JWTManager, error) {
	if config.AccessExpiry <= 0 {
		return nil, fmt.Errorf("access token expiry must be positive")
	}
	if config.RefreshExpiry <= 0 {
		return nil, fmt.Errorf("refresh token expiry must be positive")
	}

	// Default clock skew tolerance: 30 seconds
	clockSkew := config.ClockSkewTolerance
	if clockSkew == 0 {
		clockSkew = 30 * time.Second
	}

	manager := &JWTManager{
		signingMethod:      config.SigningMethod,
		accessTokenExpiry:  config.AccessExpiry,
		refreshTokenExpiry: config.RefreshExpiry,
		clockSkewTolerance: clockSkew,
	}

	switch config.SigningMethod {
	case SigningMethodHS256:
		if config.Secret == "" {
			return nil, fmt.Errorf("JWT secret cannot be empty for HS256")
		}
		manager.secret = []byte(config.Secret)

	case SigningMethodRS256:
		if config.PrivateKeyPEM == "" || config.PublicKeyPEM == "" {
			return nil, fmt.Errorf("private and public keys required for RS256")
		}

		privateKey, err := parseRSAPrivateKey(config.PrivateKeyPEM)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}

		publicKey, err := parseRSAPublicKey(config.PublicKeyPEM)
		if err != nil {
			return nil, fmt.Errorf("failed to parse public key: %w", err)
		}

		manager.privateKey = privateKey
		manager.publicKey = publicKey

	default:
		return nil, fmt.Errorf("unsupported signing method: %s", config.SigningMethod)
	}

	return manager, nil
}

// parseRSAPrivateKey parses PEM encoded RSA private key
func parseRSAPrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS1 format
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("key is not RSA private key")
	}

	return rsaKey, nil
}

// parseRSAPublicKey parses PEM encoded RSA public key
func parseRSAPublicKey(pemStr string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	rsaKey, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("key is not RSA public key")
	}

	return rsaKey, nil
}

// GenerateAccessToken generates an access token for a user
func (m *JWTManager) GenerateAccessToken(userID uuid.UUID, email string) (string, error) {
	return m.generateToken(userID, email, "", AccessToken, m.accessTokenExpiry)
}

// GenerateAccessTokenWithSession generates an access token with session ID
func (m *JWTManager) GenerateAccessTokenWithSession(userID uuid.UUID, email, sessionID string) (string, error) {
	return m.generateToken(userID, email, sessionID, AccessToken, m.accessTokenExpiry)
}

// GenerateRefreshToken generates a refresh token for a user
func (m *JWTManager) GenerateRefreshToken(userID uuid.UUID, email string) (string, error) {
	return m.generateToken(userID, email, "", RefreshToken, m.refreshTokenExpiry)
}

// GenerateRefreshTokenWithSession generates a refresh token with session ID
func (m *JWTManager) GenerateRefreshTokenWithSession(userID uuid.UUID, email, sessionID string) (string, error) {
	return m.generateToken(userID, email, sessionID, RefreshToken, m.refreshTokenExpiry)
}

// generateToken generates a JWT token with proper standard claims
func (m *JWTManager) generateToken(userID uuid.UUID, email, sessionID string, tokenType TokenType, expiry time.Duration) (string, error) {
	now := time.Now()
	jti := uuid.New().String() // Unique JWT ID for revocation tracking

	claims := Claims{
		UserID:    userID,
		Email:     email,
		Type:      tokenType,
		SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),            // Standard 'sub' claim
			ExpiresAt: jwt.NewNumericDate(now.Add(expiry)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ID:        jti,                        // Standard 'jti' claim for token ID
		},
	}

	var token *jwt.Token
	var signedToken string
	var err error

	switch m.signingMethod {
	case SigningMethodHS256:
		token = jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		signedToken, err = token.SignedString(m.secret)

	case SigningMethodRS256:
		token = jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		signedToken, err = token.SignedString(m.privateKey)

	default:
		return "", fmt.Errorf("unsupported signing method: %s", m.signingMethod)
	}

	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return signedToken, nil
}

// ValidateToken validates a JWT token and returns the claims with clock skew tolerance
func (m *JWTManager) ValidateToken(tokenString string) (*Claims, error) {
	// Configure parser with clock skew tolerance
	parser := jwt.NewParser(
		jwt.WithLeeway(m.clockSkewTolerance),
		jwt.WithValidMethods([]string{string(m.signingMethod)}),
	)

	token, err := parser.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method matches configured method
		switch m.signingMethod {
		case SigningMethodHS256:
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v (expected HS256)", token.Header["alg"])
			}
			return m.secret, nil

		case SigningMethodRS256:
			if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v (expected RS256)", token.Header["alg"])
			}
			return m.publicKey, nil

		default:
			return nil, fmt.Errorf("unsupported signing method: %s", m.signingMethod)
		}
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	return claims, nil
}

// ValidateAccessToken validates an access token specifically
func (m *JWTManager) ValidateAccessToken(tokenString string) (*Claims, error) {
	claims, err := m.ValidateToken(tokenString)
	if err != nil {
		return nil, err
	}

	if claims.Type != AccessToken {
		return nil, fmt.Errorf("token is not an access token")
	}

	return claims, nil
}

// ValidateRefreshToken validates a refresh token specifically
func (m *JWTManager) ValidateRefreshToken(tokenString string) (*Claims, error) {
	claims, err := m.ValidateToken(tokenString)
	if err != nil {
		return nil, err
	}

	if claims.Type != RefreshToken {
		return nil, fmt.Errorf("token is not a refresh token")
	}

	return claims, nil
}

// ExtractUserID extracts user ID from token without full validation
// Useful for logging/tracing, but should not be used for authorization
func ExtractUserID(tokenString string) (uuid.UUID, error) {
	token, _, err := jwt.NewParser().ParseUnverified(tokenString, &Claims{})
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return uuid.Nil, fmt.Errorf("invalid token claims")
	}

	return claims.UserID, nil
}
