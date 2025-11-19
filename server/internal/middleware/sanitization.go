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

// SanitizationConfig holds configuration for sanitization middleware
type SanitizationConfig struct {
	Logger              *zap.Logger
	EnableHTMLSanitize  bool // Sanitize HTML from inputs
	EnableSQLDetection  bool // Detect SQL injection attempts
	EnableXSSDetection  bool // Detect XSS attempts
	MaxStringLength     int  // Maximum length for string fields
	StrictMode          bool // Reject requests with dangerous patterns instead of sanitizing
	AllowedContentTypes []string
}

// SanitizationMiddleware creates a middleware for input sanitization
func SanitizationMiddleware(cfg SanitizationConfig) gin.HandlerFunc {
	// Set defaults
	if cfg.MaxStringLength == 0 {
		cfg.MaxStringLength = 10000
	}
	if len(cfg.AllowedContentTypes) == 0 {
		cfg.AllowedContentTypes = []string{
			"application/json",
			"multipart/form-data",
			"application/x-www-form-urlencoded",
		}
	}

	return func(c *gin.Context) {
		// Check content type
		contentType := c.GetHeader("Content-Type")
		if !isAllowedContentType(contentType, cfg.AllowedContentTypes) {
			cfg.Logger.Warn("Unsupported content type",
				zap.String("content_type", contentType),
				zap.String("path", c.Request.URL.Path),
			)
		}

		// Sanitize query parameters
		sanitizeQueryParams(c, cfg)

		// Sanitize path parameters
		sanitizePathParams(c, cfg)

		// Sanitize JSON body if applicable
		if strings.Contains(contentType, "application/json") {
			if err := sanitizeJSONBody(c, cfg); err != nil {
				cfg.Logger.Error("Failed to sanitize JSON body", zap.Error(err))
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "Invalid request body",
				})
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

// sanitizeQueryParams sanitizes query parameters
func sanitizeQueryParams(c *gin.Context, cfg SanitizationConfig) {
	query := c.Request.URL.Query()
	modified := false

	for key, values := range query {
		for i, value := range values {
			// Check for dangerous patterns
			if cfg.EnableSQLDetection && security.DetectSQLInjection(value) {
				cfg.Logger.Warn("SQL injection pattern detected in query parameter",
					zap.String("param", key),
					zap.String("value", value),
					zap.String("path", c.Request.URL.Path),
				)

				if cfg.StrictMode {
					c.JSON(http.StatusBadRequest, gin.H{
						"error": "Invalid query parameter detected",
						"param": key,
					})
					c.Abort()
					return
				}
			}

			// Sanitize the value
			sanitized := security.SanitizeInput(value, cfg.MaxStringLength)
			if cfg.EnableHTMLSanitize {
				sanitized = security.SanitizeHTML(sanitized)
			}

			if sanitized != value {
				query[key][i] = sanitized
				modified = true
			}
		}
	}

	if modified {
		c.Request.URL.RawQuery = query.Encode()
	}
}

// sanitizePathParams sanitizes path parameters
func sanitizePathParams(c *gin.Context, cfg SanitizationConfig) {
	params := c.Params

	for i, param := range params {
		// Check for dangerous patterns
		if cfg.EnableSQLDetection && security.DetectSQLInjection(param.Value) {
			cfg.Logger.Warn("SQL injection pattern detected in path parameter",
				zap.String("param", param.Key),
				zap.String("value", param.Value),
				zap.String("path", c.Request.URL.Path),
			)

			if cfg.StrictMode {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "Invalid path parameter detected",
					"param": param.Key,
				})
				c.Abort()
				return
			}
		}

		// Sanitize the value
		sanitized := security.SanitizeInput(param.Value, cfg.MaxStringLength)
		if cfg.EnableHTMLSanitize {
			sanitized = security.SanitizeHTML(sanitized)
		}

		if sanitized != param.Value {
			params[i].Value = sanitized
		}
	}
}

// sanitizeJSONBody sanitizes JSON request body
func sanitizeJSONBody(c *gin.Context, cfg SanitizationConfig) error {
	// Read body
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return err
	}
	c.Request.Body.Close()

	// Parse JSON
	if len(bodyBytes) == 0 {
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		return nil
	}

	var jsonData interface{}
	if err := json.Unmarshal(bodyBytes, &jsonData); err != nil {
		// Not valid JSON, restore original body
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		return nil
	}

	// Sanitize JSON data
	sanitized, blocked := sanitizeJSONData(jsonData, cfg)
	if blocked && cfg.StrictMode {
		return &SanitizationError{Message: "Dangerous pattern detected in request body"}
	}

	// Re-marshal the sanitized data
	sanitizedBytes, err := json.Marshal(sanitized)
	if err != nil {
		return err
	}

	// Restore body with sanitized data
	c.Request.Body = io.NopCloser(bytes.NewBuffer(sanitizedBytes))
	return nil
}

// sanitizeJSONData recursively sanitizes JSON data
func sanitizeJSONData(data interface{}, cfg SanitizationConfig) (interface{}, bool) {
	blocked := false

	switch v := data.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{})
		for key, value := range v {
			// Check key for dangerous patterns
			if cfg.EnableXSSDetection && containsXSS(key) {
				cfg.Logger.Warn("XSS pattern detected in JSON key", zap.String("key", key))
				blocked = true
				continue
			}

			sanitizedValue, valueBlocked := sanitizeJSONData(value, cfg)
			if valueBlocked {
				blocked = true
			}
			result[key] = sanitizedValue
		}
		return result, blocked

	case []interface{}:
		result := make([]interface{}, 0, len(v))
		for _, item := range v {
			sanitizedItem, itemBlocked := sanitizeJSONData(item, cfg)
			if itemBlocked {
				blocked = true
			}
			result = append(result, sanitizedItem)
		}
		return result, blocked

	case string:
		// Check for dangerous patterns
		if cfg.EnableSQLDetection && security.DetectSQLInjection(v) {
			cfg.Logger.Warn("SQL injection pattern detected in JSON value", zap.String("value", v))
			blocked = true
		}

		if cfg.EnableXSSDetection && containsXSS(v) {
			cfg.Logger.Warn("XSS pattern detected in JSON value", zap.String("value", v))
			blocked = true
		}

		// Sanitize the string
		sanitized := security.SanitizeInput(v, cfg.MaxStringLength)
		if cfg.EnableHTMLSanitize {
			sanitized = security.SanitizeHTML(sanitized)
		}

		return sanitized, blocked

	default:
		return v, blocked
	}
}

// containsXSS checks if string contains XSS patterns
func containsXSS(s string) bool {
	s = strings.ToLower(s)
	xssPatterns := []string{
		"<script",
		"javascript:",
		"onerror=",
		"onload=",
		"onclick=",
		"<iframe",
		"<object",
		"<embed",
	}

	for _, pattern := range xssPatterns {
		if strings.Contains(s, pattern) {
			return true
		}
	}
	return false
}

// isAllowedContentType checks if content type is allowed
func isAllowedContentType(contentType string, allowedTypes []string) bool {
	if contentType == "" {
		return true // Allow empty content type for GET requests
	}

	for _, allowed := range allowedTypes {
		if strings.Contains(contentType, allowed) {
			return true
		}
	}
	return false
}

// SanitizationError represents a sanitization error
type SanitizationError struct {
	Message string
}

func (e *SanitizationError) Error() string {
	return e.Message
}

// CreateDefaultSanitizationConfig creates a default sanitization configuration
func CreateDefaultSanitizationConfig(logger *zap.Logger) SanitizationConfig {
	return SanitizationConfig{
		Logger:              logger,
		EnableHTMLSanitize:  true,
		EnableSQLDetection:  true,
		EnableXSSDetection:  true,
		MaxStringLength:     10000,
		StrictMode:          false, // Don't reject, just sanitize by default
		AllowedContentTypes: []string{"application/json", "multipart/form-data"},
	}
}

// CreateStrictSanitizationConfig creates a strict sanitization configuration
func CreateStrictSanitizationConfig(logger *zap.Logger) SanitizationConfig {
	return SanitizationConfig{
		Logger:              logger,
		EnableHTMLSanitize:  true,
		EnableSQLDetection:  true,
		EnableXSSDetection:  true,
		MaxStringLength:     10000,
		StrictMode:          true, // Reject dangerous patterns
		AllowedContentTypes: []string{"application/json", "multipart/form-data"},
	}
}
