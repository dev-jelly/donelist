package middleware

import (
	"bytes"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/dev-jelly/donelist/internal/admin"
	"go.uber.org/zap"
)

// AdminAuditMiddleware logs all admin actions
func AdminAuditMiddleware(auditRepo *admin.AuditRepository, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only log for admin endpoints (those that passed RequireAdmin middleware)
		_, hasAdminUser := c.Get("admin_user")
		if !hasAdminUser {
			c.Next()
			return
		}

		// Get admin user ID
		adminID, err := GetUserID(c)
		if err != nil {
			logger.Warn("Failed to get admin ID for audit logging", zap.Error(err))
			c.Next()
			return
		}

		// Capture request body for audit
		var requestBody []byte
		if c.Request.Body != nil {
			requestBody, _ = io.ReadAll(c.Request.Body)
			// Restore body for handler
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}

		// Capture request metadata
		ipAddress := c.ClientIP()
		userAgent := c.Request.UserAgent()
		requestPath := c.Request.URL.Path
		requestMethod := c.Request.Method

		// Create custom response writer to capture status code
		writer := &responseWriter{
			ResponseWriter: c.Writer,
			statusCode:     http.StatusOK,
		}
		c.Writer = writer

		// Process request
		c.Next()

		// Log after request completes (async to not block response)
		go func() {
			// Determine action from method and path
			action := determineAction(requestMethod, requestPath)

			// Extract resource information from context if available
			resourceType := extractResourceType(requestPath)
			var resourceID *uuid.UUID
			if resID, exists := c.Get("resource_id"); exists {
				if id, ok := resID.(uuid.UUID); ok {
					resourceID = &id
				}
			}

			// Extract target user ID if available
			var targetUserID *uuid.UUID
			if targetID, exists := c.Get("target_user_id"); exists {
				if id, ok := targetID.(uuid.UUID); ok {
					targetUserID = &id
				}
			}

			// Prepare request body for logging (limit size)
			var requestBodyInterface interface{}
			if len(requestBody) > 0 && len(requestBody) < 10000 {
				requestBodyInterface = string(requestBody)
			}

			// Log the audit entry
			statusCode := writer.statusCode
			err := auditRepo.Log(c.Request.Context(), admin.AuditLogInput{
				AdminID:        adminID,
				Action:         action,
				ResourceType:   resourceType,
				ResourceID:     resourceID,
				TargetUserID:   targetUserID,
				IPAddress:      &ipAddress,
				UserAgent:      &userAgent,
				RequestPath:    &requestPath,
				RequestMethod:  &requestMethod,
				RequestBody:    requestBodyInterface,
				ResponseStatus: &statusCode,
			})

			if err != nil {
				logger.Error("Failed to log admin action",
					zap.Error(err),
					zap.String("admin_id", adminID.String()),
					zap.String("action", action),
				)
			}
		}()
	}
}

// responseWriter wraps gin.ResponseWriter to capture status code
type responseWriter struct {
	gin.ResponseWriter
	statusCode int
}

func (w *responseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *responseWriter) Write(data []byte) (int, error) {
	return w.ResponseWriter.Write(data)
}

// determineAction determines the audit action based on HTTP method and path
func determineAction(method, path string) string {
	switch method {
	case http.MethodPost:
		return "create"
	case http.MethodPut, http.MethodPatch:
		return "update"
	case http.MethodDelete:
		return "delete"
	case http.MethodGet:
		return "view"
	default:
		return "unknown"
	}
}

// extractResourceType extracts resource type from path
func extractResourceType(path string) string {
	// Simple extraction - can be enhanced based on your routing structure
	// Example: /api/v1/admin/users/123 -> users
	if len(path) > 0 {
		// Remove leading /api/v1/admin/
		// This is a simple heuristic and should be adjusted for your routes
		return "resource"
	}
	return "unknown"
}
