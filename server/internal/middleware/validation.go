package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/dev-jelly/donelist/internal/security"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ValidationConfig holds configuration for validation middleware
type ValidationConfig struct {
	Logger           *zap.Logger
	MaxBodySize      int64  // Maximum request body size in bytes
	SanitizeInputs   bool   // Whether to sanitize inputs
	StrictValidation bool   // Whether to use strict validation
	AllowedMethods   []string // HTTP methods to validate
}

// InputValidationMiddleware creates a middleware for input validation
func InputValidationMiddleware(cfg ValidationConfig) gin.HandlerFunc {
	validator := security.NewValidator()

	// Set defaults
	if cfg.MaxBodySize == 0 {
		cfg.MaxBodySize = 10 * 1024 * 1024 // 10MB default
	}
	if len(cfg.AllowedMethods) == 0 {
		cfg.AllowedMethods = []string{"POST", "PUT", "PATCH"}
	}

	// Create allowed methods map for faster lookup
	allowedMethods := make(map[string]bool)
	for _, method := range cfg.AllowedMethods {
		allowedMethods[method] = true
	}

	return func(c *gin.Context) {
		// Only validate specified HTTP methods
		if !allowedMethods[c.Request.Method] {
			c.Next()
			return
		}

		// Validate content type for JSON endpoints
		contentType := c.GetHeader("Content-Type")
		if strings.Contains(c.Request.URL.Path, "/api/") &&
		   !strings.Contains(contentType, "application/json") &&
		   !strings.Contains(contentType, "multipart/form-data") {
			cfg.Logger.Warn("Invalid content type",
				zap.String("path", c.Request.URL.Path),
				zap.String("content_type", contentType),
			)
		}

		// Check body size
		if c.Request.ContentLength > cfg.MaxBodySize {
			cfg.Logger.Warn("Request body too large",
				zap.String("path", c.Request.URL.Path),
				zap.Int64("size", c.Request.ContentLength),
				zap.Int64("max", cfg.MaxBodySize),
			)
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error": "Request body too large",
			})
			c.Abort()
			return
		}

		// Read and validate body if JSON
		if strings.Contains(contentType, "application/json") {
			// Read body
			bodyBytes, err := io.ReadAll(io.LimitReader(c.Request.Body, cfg.MaxBodySize))
			if err != nil {
				cfg.Logger.Error("Failed to read request body", zap.Error(err))
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "Failed to read request body",
				})
				c.Abort()
				return
			}
			c.Request.Body.Close()

			// Validate JSON structure
			if len(bodyBytes) > 0 {
				var jsonData interface{}
				if err := json.Unmarshal(bodyBytes, &jsonData); err != nil {
					cfg.Logger.Warn("Invalid JSON in request body",
						zap.Error(err),
						zap.String("path", c.Request.URL.Path),
					)
					c.JSON(http.StatusBadRequest, gin.H{
						"error": "Invalid JSON format",
					})
					c.Abort()
					return
				}

				// Sanitize inputs if enabled
				if cfg.SanitizeInputs {
					jsonData = sanitizeValidationJSONData(jsonData)
					bodyBytes, _ = json.Marshal(jsonData)
				}

				// Validate against dangerous patterns
				if cfg.StrictValidation {
					if err := validateJSONSafety(jsonData, validator); err != nil {
						cfg.Logger.Warn("Dangerous patterns detected in request",
							zap.Error(err),
							zap.String("path", c.Request.URL.Path),
						)
						c.JSON(http.StatusBadRequest, gin.H{
							"error": "Invalid input detected",
							"details": err.Error(),
						})
						c.Abort()
						return
					}
				}
			}

			// Restore body for handlers
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		// Validate query parameters
		for key, values := range c.Request.URL.Query() {
			for _, value := range values {
				// Check for dangerous patterns in query params
				if cfg.StrictValidation {
					if err := validator.ValidateVar(value, "no_sql_injection,no_xss"); err != nil {
						cfg.Logger.Warn("Dangerous pattern in query parameter",
							zap.String("param", key),
							zap.String("value", value),
							zap.Error(err),
						)
						c.JSON(http.StatusBadRequest, gin.H{
							"error": "Invalid query parameter",
							"param": key,
						})
						c.Abort()
						return
					}
				}

				// Sanitize if enabled
				if cfg.SanitizeInputs {
					sanitized := security.SanitizeInput(value, 1000)
					c.Request.URL.Query().Set(key, sanitized)
				}
			}
		}

		// Validate headers for dangerous patterns
		for key, values := range c.Request.Header {
			// Skip standard headers
			if isStandardHeader(key) {
				continue
			}

			for _, value := range values {
				if cfg.StrictValidation {
					if err := validator.ValidateVar(value, "no_sql_injection,no_xss"); err != nil {
						cfg.Logger.Warn("Dangerous pattern in header",
							zap.String("header", key),
							zap.Error(err),
						)
						// Don't abort for headers, just log
					}
				}
			}
		}

		c.Next()
	}
}

// sanitizeValidationJSONData recursively sanitizes JSON data for validation
func sanitizeValidationJSONData(data interface{}) interface{} {
	switch v := data.(type) {
	case map[string]interface{}:
		for key, value := range v {
			v[key] = sanitizeValidationJSONData(value)
		}
		return v
	case []interface{}:
		for i, value := range v {
			v[i] = sanitizeValidationJSONData(value)
		}
		return v
	case string:
		return security.SanitizeInput(v, 10000)
	default:
		return v
	}
}

// validateJSONSafety validates JSON data for security issues
func validateJSONSafety(data interface{}, validator *security.Validator) error {
	switch v := data.(type) {
	case map[string]interface{}:
		for _, value := range v {
			if err := validateJSONSafety(value, validator); err != nil {
				return err
			}
		}
	case []interface{}:
		for _, value := range v {
			if err := validateJSONSafety(value, validator); err != nil {
				return err
			}
		}
	case string:
		// Validate string for dangerous patterns
		if err := validator.ValidateVar(v, "no_xss,no_control_chars"); err != nil {
			return err
		}
	}
	return nil
}

// isStandardHeader checks if a header is a standard HTTP header
func isStandardHeader(header string) bool {
	standardHeaders := map[string]bool{
		"Authorization":    true,
		"Content-Type":     true,
		"Content-Length":   true,
		"Accept":           true,
		"Accept-Encoding":  true,
		"Accept-Language":  true,
		"User-Agent":       true,
		"Referer":          true,
		"Origin":           true,
		"Host":             true,
		"Connection":       true,
		"Cookie":           true,
		"Cache-Control":    true,
		"If-None-Match":    true,
		"If-Modified-Since": true,
	}
	return standardHeaders[header]
}

// CreateDefaultValidationConfig creates a default validation configuration
func CreateDefaultValidationConfig(logger *zap.Logger) ValidationConfig {
	return ValidationConfig{
		Logger:           logger,
		MaxBodySize:      10 * 1024 * 1024, // 10MB
		SanitizeInputs:   true,
		StrictValidation: true,
		AllowedMethods:   []string{"POST", "PUT", "PATCH"},
	}
}
