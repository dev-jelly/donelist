package middleware

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// SecurityHeadersConfig holds configuration for security headers
type SecurityHeadersConfig struct {
	Logger *zap.Logger

	// Content Security Policy
	EnableCSP           bool
	CSPDirectives       map[string][]string
	CSPReportOnly       bool
	CSPReportURI        string

	// HTTP Strict Transport Security
	EnableHSTS          bool
	HSTSMaxAge          int    // in seconds
	HSTSIncludeSubdomains bool
	HSTSPreload         bool

	// X-Frame-Options
	XFrameOptions       string // DENY, SAMEORIGIN, or ALLOW-FROM uri

	// X-Content-Type-Options
	XContentTypeOptions bool

	// X-XSS-Protection
	XXSSProtection      string // 0, 1, or 1; mode=block

	// Referrer-Policy
	ReferrerPolicy      string

	// Permissions-Policy (formerly Feature-Policy)
	PermissionsPolicy   map[string][]string

	// Cross-Origin policies
	CrossOriginEmbedderPolicy  string // require-corp, credentialless
	CrossOriginOpenerPolicy    string // same-origin, same-origin-allow-popups, unsafe-none
	CrossOriginResourcePolicy  string // same-origin, same-site, cross-origin

	// Remove headers that leak information
	RemoveServerHeader bool
	RemoveXPoweredBy   bool
}

// SecurityHeadersMiddleware creates a middleware for security headers
func SecurityHeadersMiddleware(cfg SecurityHeadersConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Content Security Policy
		if cfg.EnableCSP {
			csp := buildCSP(cfg.CSPDirectives)
			if csp != "" {
				if cfg.CSPReportOnly {
					c.Header("Content-Security-Policy-Report-Only", csp)
				} else {
					c.Header("Content-Security-Policy", csp)
				}
				if cfg.CSPReportURI != "" {
					c.Header("Report-To", fmt.Sprintf(`{"group":"csp-endpoint","max_age":10886400,"endpoints":[{"url":"%s"}]}`, cfg.CSPReportURI))
				}
			}
		}

		// HTTP Strict Transport Security (HSTS)
		if cfg.EnableHSTS {
			hstsValue := fmt.Sprintf("max-age=%d", cfg.HSTSMaxAge)
			if cfg.HSTSIncludeSubdomains {
				hstsValue += "; includeSubDomains"
			}
			if cfg.HSTSPreload {
				hstsValue += "; preload"
			}
			c.Header("Strict-Transport-Security", hstsValue)
		}

		// X-Frame-Options
		if cfg.XFrameOptions != "" {
			c.Header("X-Frame-Options", cfg.XFrameOptions)
		}

		// X-Content-Type-Options
		if cfg.XContentTypeOptions {
			c.Header("X-Content-Type-Options", "nosniff")
		}

		// X-XSS-Protection
		if cfg.XXSSProtection != "" {
			c.Header("X-XSS-Protection", cfg.XXSSProtection)
		}

		// Referrer-Policy
		if cfg.ReferrerPolicy != "" {
			c.Header("Referrer-Policy", cfg.ReferrerPolicy)
		}

		// Permissions-Policy
		if len(cfg.PermissionsPolicy) > 0 {
			permissionsPolicy := buildPermissionsPolicy(cfg.PermissionsPolicy)
			c.Header("Permissions-Policy", permissionsPolicy)
		}

		// Cross-Origin-Embedder-Policy
		if cfg.CrossOriginEmbedderPolicy != "" {
			c.Header("Cross-Origin-Embedder-Policy", cfg.CrossOriginEmbedderPolicy)
		}

		// Cross-Origin-Opener-Policy
		if cfg.CrossOriginOpenerPolicy != "" {
			c.Header("Cross-Origin-Opener-Policy", cfg.CrossOriginOpenerPolicy)
		}

		// Cross-Origin-Resource-Policy
		if cfg.CrossOriginResourcePolicy != "" {
			c.Header("Cross-Origin-Resource-Policy", cfg.CrossOriginResourcePolicy)
		}

		// Remove information-leaking headers
		if cfg.RemoveServerHeader {
			c.Header("Server", "")
		}
		if cfg.RemoveXPoweredBy {
			c.Header("X-Powered-By", "")
		}

		c.Next()
	}
}

// buildCSP builds a Content Security Policy string from directives
func buildCSP(directives map[string][]string) string {
	if len(directives) == 0 {
		return ""
	}

	var policies []string
	for directive, sources := range directives {
		if len(sources) > 0 {
			policy := fmt.Sprintf("%s %s", directive, strings.Join(sources, " "))
			policies = append(policies, policy)
		}
	}

	return strings.Join(policies, "; ")
}

// buildPermissionsPolicy builds a Permissions-Policy string from directives
func buildPermissionsPolicy(directives map[string][]string) string {
	var policies []string
	for directive, sources := range directives {
		if len(sources) == 0 {
			// Empty sources means deny all
			policies = append(policies, fmt.Sprintf("%s=()", directive))
		} else if len(sources) == 1 && sources[0] == "*" {
			// Wildcard means allow all
			policies = append(policies, fmt.Sprintf("%s=*", directive))
		} else {
			// Specific origins
			quotedSources := make([]string, len(sources))
			for i, source := range sources {
				if source == "self" {
					quotedSources[i] = "self"
				} else {
					quotedSources[i] = fmt.Sprintf(`"%s"`, source)
				}
			}
			policies = append(policies, fmt.Sprintf("%s=(%s)", directive, strings.Join(quotedSources, " ")))
		}
	}

	return strings.Join(policies, ", ")
}

// CreateDefaultSecurityHeadersConfig creates a secure default configuration
func CreateDefaultSecurityHeadersConfig(logger *zap.Logger, isProduction bool) SecurityHeadersConfig {
	cfg := SecurityHeadersConfig{
		Logger: logger,

		// Enable CSP with strict defaults
		EnableCSP: true,
		CSPDirectives: map[string][]string{
			"default-src":        {"'self'"},
			"script-src":         {"'self'", "'unsafe-inline'", "'unsafe-eval'"}, // Consider removing unsafe-* in production
			"style-src":          {"'self'", "'unsafe-inline'"},
			"img-src":            {"'self'", "data:", "https:"},
			"font-src":           {"'self'", "data:"},
			"connect-src":        {"'self'"},
			"frame-ancestors":    {"'none'"},
			"base-uri":           {"'self'"},
			"form-action":        {"'self'"},
			"object-src":         {"'none'"},
			"media-src":          {"'self'"},
			"worker-src":         {"'self'"},
			"manifest-src":       {"'self'"},
			"upgrade-insecure-requests": {},
		},
		CSPReportOnly: !isProduction, // Use report-only in development

		// HSTS - only enable in production with HTTPS
		EnableHSTS:            isProduction,
		HSTSMaxAge:            31536000, // 1 year
		HSTSIncludeSubdomains: true,
		HSTSPreload:           false, // Set to true only after testing

		// X-Frame-Options
		XFrameOptions: "DENY",

		// X-Content-Type-Options
		XContentTypeOptions: true,

		// X-XSS-Protection
		XXSSProtection: "1; mode=block",

		// Referrer-Policy
		ReferrerPolicy: "strict-origin-when-cross-origin",

		// Permissions-Policy - deny dangerous features
		PermissionsPolicy: map[string][]string{
			"geolocation":                 {},
			"microphone":                  {},
			"camera":                      {},
			"payment":                     {},
			"usb":                         {},
			"magnetometer":                {},
			"gyroscope":                   {},
			"accelerometer":               {},
			"ambient-light-sensor":        {},
			"autoplay":                    {"self"},
			"encrypted-media":             {"self"},
			"fullscreen":                  {"self"},
			"picture-in-picture":          {"self"},
			"sync-xhr":                    {},
		},

		// Cross-Origin policies
		CrossOriginEmbedderPolicy: "require-corp",
		CrossOriginOpenerPolicy:   "same-origin",
		CrossOriginResourcePolicy: "same-origin",

		// Remove server information
		RemoveServerHeader: true,
		RemoveXPoweredBy:   true,
	}

	return cfg
}

// CreateDevelopmentSecurityHeadersConfig creates a less restrictive config for development
func CreateDevelopmentSecurityHeadersConfig(logger *zap.Logger) SecurityHeadersConfig {
	cfg := CreateDefaultSecurityHeadersConfig(logger, false)

	// Relax some policies for development
	cfg.CSPDirectives["connect-src"] = []string{"'self'", "ws:", "wss:", "http://localhost:*", "https://localhost:*"}
	cfg.CrossOriginEmbedderPolicy = ""
	cfg.CrossOriginOpenerPolicy = "unsafe-none"

	return cfg
}

// CreateAPISecurityHeadersConfig creates security headers config for API endpoints
func CreateAPISecurityHeadersConfig(logger *zap.Logger, isProduction bool) SecurityHeadersConfig {
	cfg := CreateDefaultSecurityHeadersConfig(logger, isProduction)

	// API-specific CSP - more restrictive
	cfg.CSPDirectives = map[string][]string{
		"default-src": {"'none'"},
		"frame-ancestors": {"'none'"},
	}

	// API doesn't need XSS protection (no HTML rendering)
	cfg.XXSSProtection = ""

	return cfg
}
