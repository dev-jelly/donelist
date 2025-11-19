package security

import (
	"regexp"
	"time"

	"github.com/google/uuid"
)

// DTOValidationSchemas provides reusable validation schemas for common data types
type DTOValidationSchemas struct {
	validator *Validator
}

// NewDTOValidationSchemas creates a new DTO validation schemas instance
func NewDTOValidationSchemas() *DTOValidationSchemas {
	return &DTOValidationSchemas{
		validator: NewValidator(),
	}
}

// Common DTO validation schemas

// UserEmailDTO validates email input
type UserEmailDTO struct {
	Email string `validate:"required,email,max=255,no_xss,no_sql_injection"`
}

// UserPasswordDTO validates password input
type UserPasswordDTO struct {
	Password string `validate:"required,min=8,max=128,no_control_chars"`
}

// UsernameDTO validates username input
type UsernameDTO struct {
	Username string `validate:"required,min=3,max=50,no_xss,no_sql_injection,safe_username"`
}

// UUIDInputDTO validates UUID input
type UUIDInputDTO struct {
	ID uuid.UUID `validate:"required,uuid"`
}

// TextContentDTO validates general text content
type TextContentDTO struct {
	Content string `validate:"required,min=1,max=5000,no_xss,no_control_chars"`
}

// ShortTextDTO validates short text fields (titles, names, etc.)
type ShortTextDTO struct {
	Text string `validate:"required,min=1,max=255,no_xss,no_sql_injection,no_control_chars"`
}

// TagNameDTO validates tag names
type TagNameDTO struct {
	Name string `validate:"required,min=1,max=50,alphanumeric_spaces,no_xss"`
}

// URLInputDTO validates URL input
type URLInputDTO struct {
	URL string `validate:"required,url,safe_url,max=2048"`
}

// FileUploadDTO validates file upload parameters
type FileUploadDTO struct {
	Filename    string `validate:"required,safe_filename,max=255"`
	ContentType string `validate:"required,safe_mime_type,max=100"`
	Size        int64  `validate:"required,min=1,max_file_size"`
}

// PaginationDTO validates pagination parameters
type PaginationDTO struct {
	Page  int `validate:"min=1,max=10000"`
	Limit int `validate:"min=1,max=100"`
}

// DateRangeDTO validates date range input
type DateRangeDTO struct {
	StartDate time.Time `validate:"required"`
	EndDate   time.Time `validate:"required,gtfield=StartDate"`
}

// SearchQueryDTO validates search query input
type SearchQueryDTO struct {
	Query string `validate:"required,min=1,max=500,no_xss,no_sql_injection"`
}

// ColorCodeDTO validates color codes
type ColorCodeDTO struct {
	Color string `validate:"required,hex_color"`
}

// TimezoneDTO validates timezone input
type TimezoneDTO struct {
	Timezone string `validate:"required,timezone"`
}

// Custom validators for DTOs

// ValidateEmail validates an email address
func (d *DTOValidationSchemas) ValidateEmail(email string) error {
	dto := UserEmailDTO{Email: email}
	return d.validator.Validate(dto)
}

// ValidatePassword validates a password
func (d *DTOValidationSchemas) ValidatePassword(password string) error {
	dto := UserPasswordDTO{Password: password}
	return d.validator.Validate(dto)
}

// ValidateUsername validates a username
func (d *DTOValidationSchemas) ValidateUsername(username string) error {
	dto := UsernameDTO{Username: username}
	return d.validator.Validate(dto)
}

// ValidateUUID validates a UUID
func (d *DTOValidationSchemas) ValidateUUID(id uuid.UUID) error {
	dto := UUIDInputDTO{ID: id}
	return d.validator.Validate(dto)
}

// ValidateTextContent validates text content
func (d *DTOValidationSchemas) ValidateTextContent(content string) error {
	dto := TextContentDTO{Content: content}
	return d.validator.Validate(dto)
}

// ValidateShortText validates short text
func (d *DTOValidationSchemas) ValidateShortText(text string) error {
	dto := ShortTextDTO{Text: text}
	return d.validator.Validate(dto)
}

// ValidateTagName validates a tag name
func (d *DTOValidationSchemas) ValidateTagName(name string) error {
	dto := TagNameDTO{Name: name}
	return d.validator.Validate(dto)
}

// ValidateURL validates a URL
func (d *DTOValidationSchemas) ValidateURL(url string) error {
	dto := URLInputDTO{URL: url}
	return d.validator.Validate(dto)
}

// ValidatePagination validates pagination parameters
func (d *DTOValidationSchemas) ValidatePagination(page, limit int) error {
	dto := PaginationDTO{Page: page, Limit: limit}
	return d.validator.Validate(dto)
}

// ValidateSearchQuery validates a search query
func (d *DTOValidationSchemas) ValidateSearchQuery(query string) error {
	dto := SearchQueryDTO{Query: query}
	return d.validator.Validate(dto)
}

// Additional validation helper functions

// ValidateMIMEType checks if MIME type is allowed
func ValidateMIMEType(mimeType string, allowedTypes []string) bool {
	for _, allowed := range allowedTypes {
		if mimeType == allowed {
			return true
		}
	}
	return false
}

// ValidateFileSize checks if file size is within limits
func ValidateFileSize(size int64, maxSize int64) bool {
	return size > 0 && size <= maxSize
}

// ValidateFileExtension checks if file extension is allowed
func ValidateFileExtension(filename string, allowedExtensions []string) bool {
	re := regexp.MustCompile(`\.([a-zA-Z0-9]+)$`)
	matches := re.FindStringSubmatch(filename)
	if len(matches) < 2 {
		return false
	}

	ext := matches[1]
	for _, allowed := range allowedExtensions {
		if ext == allowed {
			return true
		}
	}
	return false
}

// SanitizeTag sanitizes a tag name
func SanitizeTag(tag string) string {
	// Remove leading/trailing whitespace
	tag = SanitizeInput(tag, 50)

	// Remove potentially dangerous characters
	re := regexp.MustCompile(`[<>\"'&;]`)
	tag = re.ReplaceAllString(tag, "")

	return tag
}

// SanitizeTags sanitizes a slice of tags
func SanitizeTags(tags []string) []string {
	sanitized := make([]string, 0, len(tags))
	seen := make(map[string]bool)

	for _, tag := range tags {
		cleaned := SanitizeTag(tag)
		if cleaned != "" && !seen[cleaned] {
			sanitized = append(sanitized, cleaned)
			seen[cleaned] = true
		}
	}

	return sanitized
}

// ValidateArrayLength validates array length
func ValidateArrayLength(arr []string, minLen, maxLen int) bool {
	length := len(arr)
	return length >= minLen && length <= maxLen
}

// BatchValidationResult holds results of batch validation
type BatchValidationResult struct {
	Valid   []string
	Invalid []string
	Errors  map[string]error
}

// BatchValidateEmails validates multiple email addresses
func (d *DTOValidationSchemas) BatchValidateEmails(emails []string) BatchValidationResult {
	result := BatchValidationResult{
		Valid:   make([]string, 0),
		Invalid: make([]string, 0),
		Errors:  make(map[string]error),
	}

	for _, email := range emails {
		if err := d.ValidateEmail(email); err != nil {
			result.Invalid = append(result.Invalid, email)
			result.Errors[email] = err
		} else {
			result.Valid = append(result.Valid, email)
		}
	}

	return result
}

// BatchValidateTags validates multiple tags
func (d *DTOValidationSchemas) BatchValidateTags(tags []string) BatchValidationResult {
	result := BatchValidationResult{
		Valid:   make([]string, 0),
		Invalid: make([]string, 0),
		Errors:  make(map[string]error),
	}

	for _, tag := range tags {
		if err := d.ValidateTagName(tag); err != nil {
			result.Invalid = append(result.Invalid, tag)
			result.Errors[tag] = err
		} else {
			result.Valid = append(result.Valid, tag)
		}
	}

	return result
}
