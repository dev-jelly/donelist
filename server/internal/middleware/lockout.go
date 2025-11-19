package middleware

import (
	"net/http"
	"time"

	"github.com/dev-jelly/donelist/internal/audit"
	"github.com/dev-jelly/donelist/internal/security"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// LockoutConfig holds configuration for lockout middleware
type LockoutConfig struct {
	LockoutManager *security.LockoutManager
	AuditService   *audit.Service
	Logger         *zap.Logger
	IdentifierFunc func(*gin.Context) string // Function to extract identifier (email, IP, etc.)
}

// LockoutMiddleware creates a middleware for account lockout protection
func LockoutMiddleware(cfg LockoutConfig) gin.HandlerFunc {
	// Default identifier function uses email from request body
	if cfg.IdentifierFunc == nil {
		cfg.IdentifierFunc = func(c *gin.Context) string {
			var body struct {
				Email string `json:"email"`
			}
			if err := c.ShouldBindJSON(&body); err != nil {
				return c.ClientIP() // Fall back to IP if can't get email
			}
			return body.Email
		}
	}

	return func(c *gin.Context) {
		// Only apply to login endpoints
		if c.Request.URL.Path != "/api/v1/auth/login" {
			c.Next()
			return
		}

		// Get identifier
		identifier := cfg.IdentifierFunc(c)
		if identifier == "" {
			identifier = c.ClientIP() // Fall back to IP
		}

		ctx := c.Request.Context()

		// Check if account is locked out
		isLocked, unlockTime, err := cfg.LockoutManager.IsLockedOut(ctx, identifier)
		if err != nil {
			cfg.Logger.Error("Failed to check lockout status", zap.Error(err))
			c.Next()
			return
		}

		if isLocked {
			// Log lockout attempt
			if cfg.AuditService != nil {
				cfg.AuditService.LogSecurityEvent(
					ctx,
					audit.EventTypeAccountLocked,
					"Login attempt while account locked",
					nil,
					c.ClientIP(),
					c.Request.UserAgent(),
					map[string]interface{}{
						"identifier":  identifier,
						"unlock_time": unlockTime,
					},
				)
			}

			cfg.Logger.Warn("Login attempt while account locked",
				zap.String("identifier", identifier),
				zap.Time("unlock_time", unlockTime),
			)

			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "Account temporarily locked",
				"message":     "Too many failed login attempts. Please try again later.",
				"unlock_time": unlockTime.Unix(),
				"retry_after": int(time.Until(unlockTime).Seconds()),
			})
			c.Abort()
			return
		}

		// Get failed attempts for progressive delay
		failedAttempts, err := cfg.LockoutManager.GetFailedAttempts(ctx, identifier)
		if err != nil {
			cfg.Logger.Error("Failed to get failed attempts", zap.Error(err))
		}

		// Apply progressive delay if there have been failed attempts
		if failedAttempts > 0 {
			delay := cfg.LockoutManager.GetDelayDuration(failedAttempts)
			time.Sleep(delay)
		}

		c.Next()
	}
}

// CreateDefaultLockoutConfig creates a default lockout configuration
func CreateDefaultLockoutConfig(lockoutManager *security.LockoutManager, auditService *audit.Service, logger *zap.Logger) LockoutConfig {
	return LockoutConfig{
		LockoutManager: lockoutManager,
		AuditService:   auditService,
		Logger:         logger,
		IdentifierFunc: func(c *gin.Context) string {
			// Try to get email from request
			var body struct {
				Email string `json:"email"`
			}
			if err := c.ShouldBindJSON(&body); err == nil && body.Email != "" {
				return body.Email
			}
			// Fall back to IP address
			return c.ClientIP()
		},
	}
}

// AccountLockoutHandler provides an endpoint to check lockout status
func AccountLockoutHandler(lockoutManager *security.LockoutManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		identifier := c.Query("identifier")
		if identifier == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "identifier parameter required",
			})
			return
		}

		ctx := c.Request.Context()
		info, err := lockoutManager.GetInfo(ctx, identifier)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to get lockout info",
			})
			return
		}

		c.JSON(http.StatusOK, info)
	}
}

// UnlockAccountHandler provides an endpoint to manually unlock an account (admin only)
func UnlockAccountHandler(lockoutManager *security.LockoutManager, auditService *audit.Service, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// This should only be accessible to admins - add admin auth check here

		identifier := c.Param("identifier")
		if identifier == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "identifier parameter required",
			})
			return
		}

		ctx := c.Request.Context()
		err := lockoutManager.UnlockAccount(ctx, identifier)
		if err != nil {
			logger.Error("Failed to unlock account", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to unlock account",
			})
			return
		}

		// Log unlock event
		if auditService != nil {
			auditService.LogSecurityEvent(
				ctx,
				audit.EventTypeAccountUnlocked,
				"Account manually unlocked by admin",
				nil,
				c.ClientIP(),
				c.Request.UserAgent(),
				map[string]interface{}{
					"identifier": identifier,
				},
			)
		}

		c.JSON(http.StatusOK, gin.H{
			"message":    "Account unlocked successfully",
			"identifier": identifier,
		})
	}
}
