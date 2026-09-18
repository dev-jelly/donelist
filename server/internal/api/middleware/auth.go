package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/dev-jelly/donelist/internal/auth"
	"go.uber.org/zap"
)

// AuthMiddleware creates an authentication middleware
func AuthMiddleware(jwtManager *auth.JWTManager, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract token from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			logger.Warn("Missing authorization header",
				zap.String("path", c.Request.URL.Path),
				zap.String("method", c.Request.Method),
			)
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "missing authorization header",
			})
			c.Abort()
			return
		}

		// Check Bearer prefix
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			logger.Warn("Invalid authorization header format",
				zap.String("header", authHeader),
			)
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "invalid authorization header format",
			})
			c.Abort()
			return
		}

		tokenString := parts[1]

		// Validate access token
		claims, err := jwtManager.ValidateAccessToken(tokenString)
		if err != nil {
			logger.Warn("Invalid access token",
				zap.Error(err),
				zap.String("path", c.Request.URL.Path),
			)
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "invalid or expired token",
			})
			c.Abort()
			return
		}

		// Set user information in context
		c.Set("user_id", claims.UserID)
		c.Set("user_email", claims.Email)
		c.Set("user_role", claims.Role) // Store role for authorization
		c.Set("jti", claims.ID) // Store JTI for potential blacklist operations

		logger.Debug("User authenticated",
			zap.String("user_id", claims.UserID.String()),
			zap.String("email", claims.Email),
			zap.String("role", claims.Role),
			zap.String("jti", claims.ID),
			zap.String("path", c.Request.URL.Path),
		)

		c.Next()
	}
}

// AuthMiddlewareWithBlacklist creates an authentication middleware with blacklist checking
func AuthMiddlewareWithBlacklist(jwtManager *auth.JWTManager, blacklist *auth.TokenBlacklist, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract token from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			logger.Warn("Missing authorization header",
				zap.String("path", c.Request.URL.Path),
				zap.String("method", c.Request.Method),
			)
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "missing authorization header",
			})
			c.Abort()
			return
		}

		// Check Bearer prefix
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			logger.Warn("Invalid authorization header format",
				zap.String("header", authHeader),
			)
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "invalid authorization header format",
			})
			c.Abort()
			return
		}

		tokenString := parts[1]

		// Validate access token
		claims, err := jwtManager.ValidateAccessToken(tokenString)
		if err != nil {
			logger.Warn("Invalid access token",
				zap.Error(err),
				zap.String("path", c.Request.URL.Path),
			)
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "invalid or expired token",
			})
			c.Abort()
			return
		}

		// Check if token is blacklisted
		if blacklist != nil {
			blacklisted, err := blacklist.IsAccessTokenBlacklisted(c.Request.Context(), claims.ID)
			if err != nil {
				logger.Error("Failed to check token blacklist",
					zap.Error(err),
					zap.String("jti", claims.ID),
				)
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": "authentication service error",
				})
				c.Abort()
				return
			}

			if blacklisted {
				logger.Warn("Blacklisted token used",
					zap.String("jti", claims.ID),
					zap.String("user_id", claims.UserID.String()),
					zap.String("path", c.Request.URL.Path),
				)
				c.JSON(http.StatusUnauthorized, gin.H{
					"error": "token has been revoked",
				})
				c.Abort()
				return
			}
		}

		// Set user information in context
		c.Set("user_id", claims.UserID)
		c.Set("user_email", claims.Email)
		c.Set("user_role", claims.Role) // Store role for authorization
		c.Set("jti", claims.ID) // Store JTI for potential blacklist operations
		c.Set("session_id", claims.SessionID) // Store session ID if present

		logger.Debug("User authenticated",
			zap.String("user_id", claims.UserID.String()),
			zap.String("email", claims.Email),
			zap.String("role", claims.Role),
			zap.String("jti", claims.ID),
			zap.String("path", c.Request.URL.Path),
		)

		c.Next()
	}
}

// GetUserID extracts user ID from gin context
func GetUserID(c *gin.Context) (uuid.UUID, error) {
	userID, exists := c.Get("user_id")
	if !exists {
		return uuid.Nil, gin.Error{
			Err:  nil,
			Type: gin.ErrorTypePublic,
			Meta: "user not authenticated",
		}
	}

	id, ok := userID.(uuid.UUID)
	if !ok {
		return uuid.Nil, gin.Error{
			Err:  nil,
			Type: gin.ErrorTypePrivate,
			Meta: "invalid user_id type in context",
		}
	}

	return id, nil
}

// GetUserEmail extracts user email from gin context
func GetUserEmail(c *gin.Context) (string, error) {
	email, exists := c.Get("user_email")
	if !exists {
		return "", gin.Error{
			Err:  nil,
			Type: gin.ErrorTypePublic,
			Meta: "user not authenticated",
		}
	}

	emailStr, ok := email.(string)
	if !ok {
		return "", gin.Error{
			Err:  nil,
			Type: gin.ErrorTypePrivate,
			Meta: "invalid user_email type in context",
		}
	}

	return emailStr, nil
}

// OptionalAuthMiddleware is like AuthMiddleware but doesn't abort if token is missing/invalid
func OptionalAuthMiddleware(jwtManager *auth.JWTManager, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.Next()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.Next()
			return
		}

		tokenString := parts[1]
		claims, err := jwtManager.ValidateAccessToken(tokenString)
		if err != nil {
			logger.Debug("Optional auth: invalid token", zap.Error(err))
			c.Next()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("user_email", claims.Email)
		c.Set("user_role", claims.Role)

		logger.Debug("Optional auth: user authenticated",
			zap.String("user_id", claims.UserID.String()),
			zap.String("role", claims.Role),
		)

		c.Next()
	}
}
