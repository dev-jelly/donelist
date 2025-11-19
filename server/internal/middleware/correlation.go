package middleware

import (
	"github.com/dev-jelly/donelist/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	// CorrelationIDHeader is the header name for correlation ID
	CorrelationIDHeader = "X-Correlation-ID"
	// RequestIDHeader is the header name for request ID
	RequestIDHeader = "X-Request-ID"
)

// CorrelationMiddleware creates a middleware that adds correlation IDs to requests
func CorrelationMiddleware(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get or generate correlation ID
		correlationID := c.GetHeader(CorrelationIDHeader)
		if correlationID == "" {
			correlationID = uuid.New().String()
		}

		// Generate request ID (always new for each request)
		requestID := uuid.New().String()

		// Set IDs in response headers
		c.Header(CorrelationIDHeader, correlationID)
		c.Header(RequestIDHeader, requestID)

		// Add IDs to context
		ctx := c.Request.Context()
		ctx = logger.SetCorrelationID(ctx, correlationID)
		ctx = logger.SetRequestID(ctx, requestID)

		// Extract user ID from JWT claims if available
		if claims, exists := c.Get("claims"); exists {
			if userClaims, ok := claims.(interface{ GetUserID() string }); ok {
				userID := userClaims.GetUserID()
				if userID != "" {
					ctx = logger.SetUserID(ctx, userID)
				}
			}
		}

		// Update request context
		c.Request = c.Request.WithContext(ctx)

		// Create child logger with correlation information
		childLogger := logger.WithContext(log, ctx)

		// Store logger in context for handlers to use
		ctx = logger.ToContext(ctx, childLogger)
		c.Request = c.Request.WithContext(ctx)

		// Also set in Gin context for easy access
		c.Set("logger", childLogger)
		c.Set("correlation_id", correlationID)
		c.Set("request_id", requestID)

		// Continue to next handler
		c.Next()
	}
}

// LoggingMiddleware creates a middleware that logs HTTP requests
func LoggingMiddleware(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get logger from context (may have correlation IDs)
		contextLogger := logger.FromContext(c.Request.Context())
		if contextLogger == nil {
			contextLogger = log
		}

		// Log request
		contextLogger.Info("incoming request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.String("query", c.Request.URL.RawQuery),
			zap.String("ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
		)

		// Process request
		c.Next()

		// Log response
		status := c.Writer.Status()
		logFunc := contextLogger.Info

		// Use different log levels based on status
		if status >= 500 {
			logFunc = contextLogger.Error
		} else if status >= 400 {
			logFunc = contextLogger.Warn
		}

		logFunc("request completed",
			zap.Int("status", status),
			zap.Int("size", c.Writer.Size()),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
		)
	}
}

// RecoveryMiddleware creates a middleware that recovers from panics
func RecoveryMiddleware(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Get logger from context
				contextLogger := logger.FromContext(c.Request.Context())
				if contextLogger == nil {
					contextLogger = log
				}

				contextLogger.Error("panic recovered",
					zap.Any("error", err),
					zap.String("method", c.Request.Method),
					zap.String("path", c.Request.URL.Path),
					zap.Stack("stack"),
				)

				// Return 500 error
				c.JSON(500, gin.H{
					"error":   "Internal server error",
					"message": "An unexpected error occurred",
				})
				c.Abort()
			}
		}()

		c.Next()
	}
}
