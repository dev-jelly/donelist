package middleware

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/dev-jelly/donelist/internal/user"
	"go.uber.org/zap"
)

// UserRepository interface for admin middleware
type UserRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*user.User, error)
}

// RequireAdmin middleware ensures the authenticated user has admin role
// This middleware must be used after AuthMiddleware
func RequireAdmin(userRepo UserRepository, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get user ID from context (set by AuthMiddleware)
		userID, err := GetUserID(c)
		if err != nil {
			logger.Warn("Admin check failed: user not authenticated",
				zap.String("path", c.Request.URL.Path),
			)
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "authentication required",
			})
			c.Abort()
			return
		}

		// Fetch user from database to get current role
		u, err := userRepo.GetByID(c.Request.Context(), userID)
		if err != nil {
			logger.Error("Admin check failed: failed to fetch user",
				zap.Error(err),
				zap.String("user_id", userID.String()),
			)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "failed to verify permissions",
			})
			c.Abort()
			return
		}

		// Check if user has admin role
		if !u.IsAdmin() {
			logger.Warn("Admin access denied: insufficient permissions",
				zap.String("user_id", userID.String()),
				zap.String("role", u.Role),
				zap.String("path", c.Request.URL.Path),
			)
			c.JSON(http.StatusForbidden, gin.H{
				"error": "admin access required",
			})
			c.Abort()
			return
		}

		// Store admin user in context for audit logging
		c.Set("admin_user", u)
		c.Set("user_role", u.Role)

		logger.Debug("Admin access granted",
			zap.String("user_id", userID.String()),
			zap.String("role", u.Role),
			zap.String("path", c.Request.URL.Path),
		)

		c.Next()
	}
}

// RequireSuperAdmin middleware ensures the authenticated user has superadmin role
// This middleware must be used after AuthMiddleware
func RequireSuperAdmin(userRepo UserRepository, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get user ID from context (set by AuthMiddleware)
		userID, err := GetUserID(c)
		if err != nil {
			logger.Warn("Superadmin check failed: user not authenticated",
				zap.String("path", c.Request.URL.Path),
			)
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "authentication required",
			})
			c.Abort()
			return
		}

		// Fetch user from database to get current role
		u, err := userRepo.GetByID(c.Request.Context(), userID)
		if err != nil {
			logger.Error("Superadmin check failed: failed to fetch user",
				zap.Error(err),
				zap.String("user_id", userID.String()),
			)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "failed to verify permissions",
			})
			c.Abort()
			return
		}

		// Check if user has superadmin role
		if !u.IsSuperAdmin() {
			logger.Warn("Superadmin access denied: insufficient permissions",
				zap.String("user_id", userID.String()),
				zap.String("role", u.Role),
				zap.String("path", c.Request.URL.Path),
			)
			c.JSON(http.StatusForbidden, gin.H{
				"error": "superadmin access required",
			})
			c.Abort()
			return
		}

		// Store admin user in context for audit logging
		c.Set("admin_user", u)
		c.Set("user_role", u.Role)

		logger.Debug("Superadmin access granted",
			zap.String("user_id", userID.String()),
			zap.String("role", u.Role),
			zap.String("path", c.Request.URL.Path),
		)

		c.Next()
	}
}

// RequireAdminFromToken checks admin role from JWT token (faster, no DB lookup)
// This is useful for high-throughput endpoints but less secure as role changes
// won't be reflected until token refresh
func RequireAdminFromToken(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get role from JWT claims (set by AuthMiddleware)
		roleVal, exists := c.Get("user_role")
		if !exists {
			// Try to get from token claims directly
			roleVal, exists = c.Get("role")
		}

		if !exists {
			logger.Warn("Admin check failed: role not found in token",
				zap.String("path", c.Request.URL.Path),
			)
			c.JSON(http.StatusForbidden, gin.H{
				"error": "admin access required",
			})
			c.Abort()
			return
		}

		role, ok := roleVal.(string)
		if !ok {
			logger.Error("Admin check failed: invalid role type in context")
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "failed to verify permissions",
			})
			c.Abort()
			return
		}

		// Check if role is admin or superadmin
		if role != string(user.RoleAdmin) && role != string(user.RoleSuperAdmin) {
			userID, _ := GetUserID(c)
			logger.Warn("Admin access denied: insufficient permissions",
				zap.String("user_id", userID.String()),
				zap.String("role", role),
				zap.String("path", c.Request.URL.Path),
			)
			c.JSON(http.StatusForbidden, gin.H{
				"error": "admin access required",
			})
			c.Abort()
			return
		}

		c.Set("user_role", role)
		c.Next()
	}
}

// GetAdminUser retrieves the admin user from context
func GetAdminUser(c *gin.Context) (*user.User, bool) {
	adminUserVal, exists := c.Get("admin_user")
	if !exists {
		return nil, false
	}

	adminUser, ok := adminUserVal.(*user.User)
	return adminUser, ok
}

// GetUserRole retrieves the user role from context
func GetUserRole(c *gin.Context) (string, bool) {
	roleVal, exists := c.Get("user_role")
	if !exists {
		return "", false
	}

	role, ok := roleVal.(string)
	return role, ok
}
