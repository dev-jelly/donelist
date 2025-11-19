package middleware

import (
	"bytes"
	"io"
	"time"

	"github.com/dev-jelly/donelist/internal/audit"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// AuditConfig holds configuration for audit middleware
type AuditConfig struct {
	AuditService    *audit.Service
	Logger          *zap.Logger
	EnabledPaths    []string // If empty, audit all paths
	DisabledPaths   []string // Paths to skip auditing
	AuditReadOps    bool     // Whether to audit read operations (GET)
	AuditBodyData   bool     // Whether to include request body in audit details
}

// AuditMiddleware creates an audit logging middleware
func AuditMiddleware(cfg AuditConfig) gin.HandlerFunc {
	// Build disabled paths map for faster lookup
	disabledPaths := make(map[string]bool)
	for _, path := range cfg.DisabledPaths {
		disabledPaths[path] = true
	}

	// Build enabled paths map if specified
	var enabledPaths map[string]bool
	if len(cfg.EnabledPaths) > 0 {
		enabledPaths = make(map[string]bool)
		for _, path := range cfg.EnabledPaths {
			enabledPaths[path] = true
		}
	}

	return func(c *gin.Context) {
		// Skip if path is disabled
		if disabledPaths[c.Request.URL.Path] {
			c.Next()
			return
		}

		// If enabled paths are specified, only audit those
		if enabledPaths != nil && !enabledPaths[c.Request.URL.Path] {
			c.Next()
			return
		}

		// Skip read operations if not enabled
		if !cfg.AuditReadOps && c.Request.Method == "GET" {
			c.Next()
			return
		}

		// Capture request details
		startTime := time.Now()
		requestID := c.GetString("request_id")
		if requestID == "" {
			requestID = uuid.New().String()
			c.Set("request_id", requestID)
		}

		// Get user info
		var userID *uuid.UUID
		if id, exists := c.Get("user_id"); exists {
			if uid, ok := id.(uuid.UUID); ok {
				userID = &uid
			}
		}

		ipAddress := c.ClientIP()
		userAgent := c.Request.UserAgent()
		path := c.Request.URL.Path
		method := c.Request.Method

		// Optionally capture request body for audit
		var requestBody []byte
		if cfg.AuditBodyData && (method == "POST" || method == "PUT" || method == "PATCH") {
			if c.Request.Body != nil {
				requestBody, _ = io.ReadAll(c.Request.Body)
				// Restore body for handlers
				c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
			}
		}

		// Process request
		c.Next()

		// Capture response details
		duration := time.Since(startTime)
		statusCode := c.Writer.Status()
		success := statusCode >= 200 && statusCode < 400

		// Determine event type based on path and method
		eventType := getEventTypeFromRequest(method, path, statusCode)

		// Build audit details
		details := map[string]interface{}{
			"method":       method,
			"path":         path,
			"status_code":  statusCode,
			"duration_ms":  duration.Milliseconds(),
		}

		if cfg.AuditBodyData && len(requestBody) > 0 && len(requestBody) < 10000 {
			// Only include body if it's not too large (< 10KB)
			details["request_body"] = string(requestBody)
		}

		// Add error information if request failed
		var errorMsg *string
		if !success {
			if errors := c.Errors.String(); errors != "" {
				errorMsg = &errors
			}
		}

		// Log to audit service
		go func() {
			// Run in background to not block request
			ctx := c.Copy().Request.Context()
			err := cfg.AuditService.Log(ctx, audit.CreateAuditLogInput{
				EventType:    eventType,
				Severity:     getSeverityFromStatusCode(statusCode),
				UserID:       userID,
				IPAddress:    ipAddress,
				UserAgent:    userAgent,
				Action:       method + " " + path,
				Resource:     getResourceFromPath(path),
				Details:      details,
				Success:      success,
				ErrorMessage: errorMsg,
				RequestID:    requestID,
			})
			if err != nil {
				cfg.Logger.Error("Failed to log audit event",
					zap.Error(err),
					zap.String("path", path),
					zap.String("method", method),
				)
			}
		}()
	}
}

// getEventTypeFromRequest determines the audit event type from request details
func getEventTypeFromRequest(method, path string, statusCode int) audit.EventType {
	// Authentication endpoints
	if path == "/api/v1/auth/login" {
		if statusCode >= 200 && statusCode < 300 {
			return audit.EventTypeLoginSuccess
		}
		return audit.EventTypeLoginFailed
	}
	if path == "/api/v1/auth/logout" {
		return audit.EventTypeLogout
	}
	if path == "/api/v1/auth/logout-all" {
		return audit.EventTypeLogoutAll
	}
	if path == "/api/v1/auth/register" {
		return audit.EventTypeRegister
	}
	if path == "/api/v1/auth/refresh" {
		return audit.EventTypeRefreshToken
	}

	// User operations
	if path == "/api/v1/users/me" {
		switch method {
		case "PATCH":
			return audit.EventTypeUserUpdate
		case "DELETE":
			return audit.EventTypeUserDelete
		}
	}

	// Checkin operations
	if path == "/api/v1/checkins" && method == "POST" {
		return audit.EventTypeCheckinCreate
	}
	// Check for checkin update (PATCH /api/v1/checkins/:id)
	if len(path) > 19 && path[:19] == "/api/v1/checkins/" && method == "PATCH" {
		return audit.EventTypeCheckinUpdate
	}
	// Check for checkin delete
	if len(path) > 19 && path[:19] == "/api/v1/checkins/" && method == "DELETE" {
		return audit.EventTypeCheckinDelete
	}

	// Category operations
	if path == "/api/v1/categories" && method == "POST" {
		return audit.EventTypeCategoryCreate
	}

	// Default to admin action for unrecognized paths
	return audit.EventTypeAdminAction
}

// getSeverityFromStatusCode determines severity based on HTTP status code
func getSeverityFromStatusCode(statusCode int) audit.Severity {
	switch {
	case statusCode >= 500:
		return audit.SeverityError
	case statusCode >= 400:
		return audit.SeverityWarning
	case statusCode >= 200 && statusCode < 300:
		return audit.SeverityInfo
	default:
		return audit.SeverityInfo
	}
}

// getResourceFromPath extracts the resource type from the path
func getResourceFromPath(path string) string {
	// Simple extraction - can be enhanced
	if len(path) > 8 && path[:8] == "/api/v1/" {
		remainder := path[8:]
		// Find first slash
		for i, c := range remainder {
			if c == '/' {
				return remainder[:i]
			}
		}
		return remainder
	}
	return ""
}

// CreateDefaultAuditConfig creates a default audit configuration
func CreateDefaultAuditConfig(auditService *audit.Service, logger *zap.Logger) AuditConfig {
	return AuditConfig{
		AuditService: auditService,
		Logger:       logger,
		DisabledPaths: []string{
			"/health",
			"/ready",
			"/metrics",
		},
		AuditReadOps:  false, // Don't audit GET requests by default
		AuditBodyData: false, // Don't include request body by default (privacy)
	}
}

// CreateSecurityAuditConfig creates an audit config focused on security events
func CreateSecurityAuditConfig(auditService *audit.Service, logger *zap.Logger) AuditConfig {
	return AuditConfig{
		AuditService: auditService,
		Logger:       logger,
		EnabledPaths: []string{
			"/api/v1/auth/login",
			"/api/v1/auth/register",
			"/api/v1/auth/logout",
			"/api/v1/auth/logout-all",
			"/api/v1/users/me",
		},
		AuditReadOps:  false,
		AuditBodyData: true, // Include body for security analysis
	}
}
