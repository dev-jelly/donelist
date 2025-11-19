package security

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/go-playground/validator/v10"
)

// Validator wraps go-playground validator with custom validations
type Validator struct {
	validate *validator.Validate
}

// NewValidator creates a new validator instance with custom validations
func NewValidator() *Validator {
	v := validator.New()

	// Register custom validations
	v.RegisterValidation("no_sql_injection", validateNoSQLInjection)
	v.RegisterValidation("no_xss", validateNoXSS)
	v.RegisterValidation("safe_path", validateSafePath)
	v.RegisterValidation("alphanumeric_spaces", validateAlphanumericSpaces)
	v.RegisterValidation("safe_url", validateSafeURL)
	v.RegisterValidation("no_control_chars", validateNoControlChars)
	v.RegisterValidation("safe_filename", validateSafeFilename)
	v.RegisterValidation("hex_color", validateHexColor)
	v.RegisterValidation("hsl_color", validateHSLColor)
	v.RegisterValidation("color", validateColor)
	v.RegisterValidation("timezone", validateTimezone)
	v.RegisterValidation("safe_username", validateSafeUsername)
	v.RegisterValidation("safe_mime_type", validateSafeMIMEType)
	v.RegisterValidation("max_file_size", validateMaxFileSize)
	v.RegisterValidation("no_ldap_injection", validateNoLDAPInjection)
	v.RegisterValidation("no_command_injection", validateNoCommandInjection)
	v.RegisterValidation("safe_json", validateSafeJSON)

	return &Validator{validate: v}
}

// Validate validates a struct
func (v *Validator) Validate(i interface{}) error {
	if err := v.validate.Struct(i); err != nil {
		// Return formatted validation errors
		return formatValidationErrors(err)
	}
	return nil
}

// ValidateVar validates a single variable
func (v *Validator) ValidateVar(field interface{}, tag string) error {
	if err := v.validate.Var(field, tag); err != nil {
		return formatValidationErrors(err)
	}
	return nil
}

// Custom validation functions

// validateNoSQLInjection checks for common SQL injection patterns
func validateNoSQLInjection(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	value = strings.ToLower(value)

	// Common SQL injection patterns
	sqlPatterns := []string{
		"'",
		"--",
		"/*",
		"*/",
		"xp_",
		"sp_",
		"union",
		"select",
		"insert",
		"update",
		"delete",
		"drop",
		"create",
		"alter",
		"execute",
		"exec",
		"script",
		"javascript",
		"<script",
		"onerror",
		"onload",
	}

	for _, pattern := range sqlPatterns {
		if strings.Contains(value, pattern) {
			return false
		}
	}
	return true
}

// validateNoXSS checks for XSS attack patterns
func validateNoXSS(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	value = strings.ToLower(value)

	// XSS patterns
	xssPatterns := []string{
		"<script",
		"javascript:",
		"onerror=",
		"onload=",
		"onclick=",
		"onfocus=",
		"onmouseover=",
		"onstart=",
		"onmouseenter=",
		"onmouseleave=",
		"<iframe",
		"<object",
		"<embed",
		"<applet",
		"<marquee",
		"<svg",
		"<math",
		"eval(",
		"expression(",
		"data:",
		"vbscript:",
	}

	for _, pattern := range xssPatterns {
		if strings.Contains(value, pattern) {
			return false
		}
	}
	return true
}

// validateSafePath ensures path doesn't contain directory traversal
func validateSafePath(fl validator.FieldLevel) bool {
	value := fl.Field().String()

	// Check for directory traversal
	dangerousPatterns := []string{
		"..",
		"./",
		"\\",
		"~",
	}

	for _, pattern := range dangerousPatterns {
		if strings.Contains(value, pattern) {
			return false
		}
	}
	return true
}

// validateAlphanumericSpaces allows only alphanumeric characters and spaces
func validateAlphanumericSpaces(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9\s]+$`, value)
	return matched
}

// validateSafeURL validates URL format and protocol
func validateSafeURL(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	value = strings.ToLower(value)

	// Must start with http:// or https://
	if !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
		return false
	}

	// Should not contain dangerous patterns
	dangerousPatterns := []string{
		"javascript:",
		"data:",
		"vbscript:",
		"file:",
	}

	for _, pattern := range dangerousPatterns {
		if strings.Contains(value, pattern) {
			return false
		}
	}
	return true
}

// validateNoControlChars ensures no control characters in string
func validateNoControlChars(fl validator.FieldLevel) bool {
	value := fl.Field().String()

	for _, r := range value {
		// Check for control characters (except newline, tab, carriage return)
		if r < 32 && r != 9 && r != 10 && r != 13 {
			return false
		}
	}
	return true
}

// validateSafeFilename ensures safe filename without path traversal
func validateSafeFilename(fl validator.FieldLevel) bool {
	value := fl.Field().String()

	// Check for dangerous patterns
	dangerousPatterns := []string{
		"..",
		"/",
		"\\",
		":",
		"*",
		"?",
		"\"",
		"<",
		">",
		"|",
	}

	for _, pattern := range dangerousPatterns {
		if strings.Contains(value, pattern) {
			return false
		}
	}

	// Must not be empty or just dots
	if value == "" || value == "." || value == ".." {
		return false
	}

	return true
}

// validateHexColor validates hex color format
func validateHexColor(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	matched, _ := regexp.MatchString(`^#([A-Fa-f0-9]{6}|[A-Fa-f0-9]{3})$`, value)
	return matched
}

// validateHSLColor validates HSL color format
func validateHSLColor(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	// Match hsl(h, s%, l%) or hsl(h s% l%)
	matched, _ := regexp.MatchString(`^hsl\(\s*(\d+(?:\.\d+)?)\s*,?\s*(\d+(?:\.\d+)?)%\s*,?\s*(\d+(?:\.\d+)?)%\s*\)$`, value)
	return matched
}

// validateColor validates color in either hex or HSL format
func validateColor(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	value = strings.TrimSpace(value)

	// Check if it's a valid hex color
	if strings.HasPrefix(value, "#") {
		return validateHexColor(fl)
	}

	// Check if it's a valid HSL color
	if strings.HasPrefix(strings.ToLower(value), "hsl(") {
		return validateHSLColor(fl)
	}

	return false
}

// validateTimezone validates timezone string
func validateTimezone(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	// Simple validation for common timezone formats
	matched, _ := regexp.MatchString(`^[A-Za-z]+/[A-Za-z_]+$`, value)
	return matched || value == "UTC"
}

// formatValidationErrors formats validator errors into readable messages
func formatValidationErrors(err error) error {
	var messages []string

	if validationErrors, ok := err.(validator.ValidationErrors); ok {
		for _, e := range validationErrors {
			messages = append(messages, formatFieldError(e))
		}
	}

	if len(messages) > 0 {
		return fmt.Errorf("validation failed: %s", strings.Join(messages, "; "))
	}

	return err
}

// formatFieldError formats a single field error
func formatFieldError(e validator.FieldError) string {
	field := e.Field()
	tag := e.Tag()

	switch tag {
	case "required":
		return fmt.Sprintf("%s is required", field)
	case "email":
		return fmt.Sprintf("%s must be a valid email address", field)
	case "min":
		return fmt.Sprintf("%s must be at least %s characters", field, e.Param())
	case "max":
		return fmt.Sprintf("%s must be at most %s characters", field, e.Param())
	case "len":
		return fmt.Sprintf("%s must be exactly %s characters", field, e.Param())
	case "uuid":
		return fmt.Sprintf("%s must be a valid UUID", field)
	case "url":
		return fmt.Sprintf("%s must be a valid URL", field)
	case "no_sql_injection":
		return fmt.Sprintf("%s contains potentially dangerous SQL patterns", field)
	case "no_xss":
		return fmt.Sprintf("%s contains potentially dangerous XSS patterns", field)
	case "safe_path":
		return fmt.Sprintf("%s contains unsafe path patterns", field)
	case "alphanumeric_spaces":
		return fmt.Sprintf("%s must contain only alphanumeric characters and spaces", field)
	case "safe_url":
		return fmt.Sprintf("%s must be a safe URL with http or https protocol", field)
	case "no_control_chars":
		return fmt.Sprintf("%s contains invalid control characters", field)
	case "safe_filename":
		return fmt.Sprintf("%s must be a valid filename without path characters", field)
	case "hex_color":
		return fmt.Sprintf("%s must be a valid hex color", field)
	case "hsl_color":
		return fmt.Sprintf("%s must be a valid HSL color (hsl(h, s%%, l%%))", field)
	case "color":
		return fmt.Sprintf("%s must be a valid color (hex or HSL format)", field)
	case "timezone":
		return fmt.Sprintf("%s must be a valid timezone", field)
	case "safe_username":
		return fmt.Sprintf("%s must be a valid username (alphanumeric, underscore, hyphen only)", field)
	case "safe_mime_type":
		return fmt.Sprintf("%s must be an allowed MIME type", field)
	case "max_file_size":
		return fmt.Sprintf("%s exceeds maximum allowed file size (10MB)", field)
	case "no_ldap_injection":
		return fmt.Sprintf("%s contains potentially dangerous LDAP patterns", field)
	case "no_command_injection":
		return fmt.Sprintf("%s contains potentially dangerous command injection patterns", field)
	case "safe_json":
		return fmt.Sprintf("%s contains potentially dangerous JSON patterns", field)
	default:
		return fmt.Sprintf("%s failed validation: %s", field, tag)
	}
}

// SanitizeInput sanitizes user input by removing dangerous characters
func SanitizeInput(input string, maxLength int) string {
	// Remove null bytes
	input = strings.ReplaceAll(input, "\x00", "")

	// Trim whitespace
	input = strings.TrimSpace(input)

	// Limit length
	if maxLength > 0 && utf8.RuneCountInString(input) > maxLength {
		runes := []rune(input)
		input = string(runes[:maxLength])
	}

	return input
}

// SanitizeHTML removes HTML tags and dangerous content
func SanitizeHTML(input string) string {
	// Remove script tags and their content FIRST (before removing tags)
	re := regexp.MustCompile(`(?i)<script[^>]*>.*?</script>`)
	input = re.ReplaceAllString(input, "")

	// Remove style tags and their content
	re = regexp.MustCompile(`(?i)<style[^>]*>.*?</style>`)
	input = re.ReplaceAllString(input, "")

	// Remove event handlers
	re = regexp.MustCompile(`(?i)on\w+\s*=\s*["'][^"']*["']`)
	input = re.ReplaceAllString(input, "")

	// Remove all remaining HTML tags
	re = regexp.MustCompile(`<[^>]*>`)
	input = re.ReplaceAllString(input, "")

	return input
}

// IsValidUUID checks if a string is a valid UUID
func IsValidUUID(uuid string) bool {
	matched, _ := regexp.MatchString(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, uuid)
	return matched
}

// IsValidEmail checks if a string is a valid email
func IsValidEmail(email string) bool {
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`, email)
	return matched
}

// validateSafeUsername validates username format
func validateSafeUsername(fl validator.FieldLevel) bool {
	value := fl.Field().String()

	// Username must start with alphanumeric
	if !regexp.MustCompile(`^[a-zA-Z0-9]`).MatchString(value) {
		return false
	}

	// Only allow alphanumeric, underscore, hyphen
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9_-]+$`, value)
	if !matched {
		return false
	}

	// Check for suspicious patterns
	suspiciousPatterns := []string{
		"admin",
		"root",
		"system",
		"test",
		"null",
		"undefined",
	}

	lowerValue := strings.ToLower(value)
	for _, pattern := range suspiciousPatterns {
		if strings.Contains(lowerValue, pattern) {
			return false
		}
	}

	return true
}

// validateSafeMIMEType validates MIME type format
func validateSafeMIMEType(fl validator.FieldLevel) bool {
	value := fl.Field().String()

	// Allowed MIME types
	allowedMIMETypes := []string{
		"image/jpeg",
		"image/png",
		"image/gif",
		"image/webp",
		"application/pdf",
		"text/plain",
		"text/csv",
		"application/json",
		"application/zip",
	}

	for _, allowed := range allowedMIMETypes {
		if value == allowed {
			return true
		}
	}

	return false
}

// validateMaxFileSize validates file size
func validateMaxFileSize(fl validator.FieldLevel) bool {
	size := fl.Field().Int()

	// Max file size: 10MB
	maxSize := int64(10 * 1024 * 1024)

	return size > 0 && size <= maxSize
}

// validateNoLDAPInjection checks for LDAP injection patterns
func validateNoLDAPInjection(fl validator.FieldLevel) bool {
	value := fl.Field().String()

	// LDAP injection patterns
	ldapPatterns := []string{
		"*",
		"(",
		")",
		"|",
		"&",
		"!",
		"=",
		"~",
		">=",
		"<=",
	}

	for _, pattern := range ldapPatterns {
		if strings.Contains(value, pattern) {
			return false
		}
	}

	return true
}

// validateNoCommandInjection checks for command injection patterns
func validateNoCommandInjection(fl validator.FieldLevel) bool {
	value := fl.Field().String()

	// Command injection patterns
	cmdPatterns := []string{
		";",
		"|",
		"&",
		"$(",
		"`",
		"||",
		"&&",
		"\n",
		"\r",
		"<",
		">",
		">>",
		"<<",
	}

	for _, pattern := range cmdPatterns {
		if strings.Contains(value, pattern) {
			return false
		}
	}

	return true
}

// validateSafeJSON validates JSON string for dangerous content
func validateSafeJSON(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	value = strings.ToLower(value)

	// Check for prototype pollution attempts
	dangerousPatterns := []string{
		"__proto__",
		"constructor",
		"prototype",
		"function",
		"eval",
		"settimeout",
		"setinterval",
	}

	for _, pattern := range dangerousPatterns {
		if strings.Contains(value, pattern) {
			return false
		}
	}

	return true
}
