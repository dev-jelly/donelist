package mode

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ModeMiddleware creates a Gin middleware for mode detection
func ModeMiddleware(service *Service, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Detect mode from URL path or query parameter
		mode := detectModeFromRequest(c)

		// Add mode to context
		ctx := context.WithValue(c.Request.Context(), "mode", mode)
		c.Request = c.Request.WithContext(ctx)

		// Log mode detection
		logger.Debug("Mode detected",
			zap.String("path", c.Request.URL.Path),
			zap.String("mode", string(mode)),
		)

		c.Next()
	}
}

// detectModeFromRequest detects the mode from request path or query parameters
func detectModeFromRequest(c *gin.Context) Mode {
	// Check URL path prefix (/api/v1/personal/* or /api/v1/team/*)
	path := c.Request.URL.Path
	
	if strings.HasPrefix(path, "/api/v1/team") || strings.HasPrefix(path, "/team") {
		return ModeTeam
	}
	
	if strings.HasPrefix(path, "/api/v1/personal") || strings.HasPrefix(path, "/personal") {
		return ModePersonal
	}

	// Check query parameter ?mode=team or ?mode=personal
	if modeParam := c.Query("mode"); modeParam != "" {
		mode := Mode(modeParam)
		if mode.IsValid() {
			return mode
		}
	}

	// Check header X-Mode
	if modeHeader := c.GetHeader("X-Mode"); modeHeader != "" {
		mode := Mode(modeHeader)
		if mode.IsValid() {
			return mode
		}
	}

	// Default to personal mode
	return DefaultMode()
}

// TeamContextMiddleware extracts team ID from request for team mode
func TeamContextMiddleware(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get mode from context
		mode := c.Request.Context().Value("mode")
		if mode != ModeTeam {
			c.Next()
			return
		}

		// Extract team ID from URL path parameter
		teamIDStr := c.Param("team_id")
		if teamIDStr == "" {
			// Try query parameter
			teamIDStr = c.Query("team_id")
		}

		if teamIDStr == "" {
			// Try header
			teamIDStr = c.GetHeader("X-Team-ID")
		}

		if teamIDStr != "" {
			if teamID, err := uuid.Parse(teamIDStr); err == nil {
				ctx := context.WithValue(c.Request.Context(), "team_id", &teamID)
				c.Request = c.Request.WithContext(ctx)
				
				logger.Debug("Team context added",
					zap.String("team_id", teamID.String()),
				)
			}
		}

		c.Next()
	}
}

// RequireMode creates a middleware that requires a specific mode
func RequireMode(requiredMode Mode, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		currentMode := c.Request.Context().Value("mode")
		
		if currentMode != requiredMode {
			logger.Warn("Mode mismatch",
				zap.String("required", string(requiredMode)),
				zap.Any("current", currentMode),
				zap.String("path", c.Request.URL.Path),
			)
			
			c.JSON(400, gin.H{
				"error": "invalid_mode",
				"message": "This endpoint requires " + string(requiredMode) + " mode",
				"required_mode": string(requiredMode),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireTeamMode is a convenience middleware for team mode
func RequireTeamMode(logger *zap.Logger) gin.HandlerFunc {
	return RequireMode(ModeTeam, logger)
}

// RequirePersonalMode is a convenience middleware for personal mode
func RequirePersonalMode(logger *zap.Logger) gin.HandlerFunc {
	return RequireMode(ModePersonal, logger)
}
