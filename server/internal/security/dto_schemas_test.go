package security

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestDTOValidationSchemas_ValidateEmail(t *testing.T) {
	schemas := NewDTOValidationSchemas()

	tests := []struct {
		name    string
		email   string
		wantErr bool
	}{
		{
			name:    "Valid email",
			email:   "user@example.com",
			wantErr: false,
		},
		{
			name:    "Valid email with subdomain",
			email:   "user@mail.example.com",
			wantErr: false,
		},
		{
			name:    "Invalid - no @",
			email:   "userexample.com",
			wantErr: true,
		},
		{
			name:    "Invalid - XSS attempt",
			email:   "user<script>@example.com",
			wantErr: true,
		},
		{
			name:    "Invalid - SQL injection",
			email:   "user'@example.com",
			wantErr: true,
		},
		{
			name:    "Empty email",
			email:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := schemas.ValidateEmail(tt.email)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDTOValidationSchemas_ValidatePassword(t *testing.T) {
	schemas := NewDTOValidationSchemas()

	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{
			name:     "Valid password",
			password: "SecurePass123!",
			wantErr:  false,
		},
		{
			name:     "Valid long password",
			password: "ThisIsAVeryLongAndSecurePassword123456",
			wantErr:  false,
		},
		{
			name:     "Too short",
			password: "Short1!",
			wantErr:  true,
		},
		{
			name:     "Empty password",
			password: "",
			wantErr:  true,
		},
		{
			name:     "Contains control chars",
			password: "Pass\x00word123",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := schemas.ValidatePassword(tt.password)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDTOValidationSchemas_ValidateUsername(t *testing.T) {
	schemas := NewDTOValidationSchemas()

	tests := []struct {
		name     string
		username string
		wantErr  bool
	}{
		{
			name:     "Valid username",
			username: "john_doe",
			wantErr:  false,
		},
		{
			name:     "Valid username with numbers",
			username: "user123",
			wantErr:  false,
		},
		{
			name:     "Valid username with hyphen",
			username: "john-doe-123",
			wantErr:  false,
		},
		{
			name:     "Too short",
			username: "ab",
			wantErr:  true,
		},
		{
			name:     "Contains spaces",
			username: "john doe",
			wantErr:  true,
		},
		{
			name:     "Contains admin",
			username: "admin123",
			wantErr:  true,
		},
		{
			name:     "Contains root",
			username: "rootuser",
			wantErr:  true,
		},
		{
			name:     "XSS attempt",
			username: "user<script>",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := schemas.ValidateUsername(tt.username)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDTOValidationSchemas_ValidateTextContent(t *testing.T) {
	schemas := NewDTOValidationSchemas()

	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name:    "Valid content",
			content: "This is a normal text content.",
			wantErr: false,
		},
		{
			name:    "Valid long content",
			content: "This is a much longer content with multiple sentences. It should still be valid as long as it doesn't exceed the maximum length.",
			wantErr: false,
		},
		{
			name:    "Empty content",
			content: "",
			wantErr: true,
		},
		{
			name:    "XSS attempt",
			content: "Hello <script>alert('xss')</script> World",
			wantErr: true,
		},
		{
			name:    "Control characters",
			content: "Hello\x00World",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := schemas.ValidateTextContent(tt.content)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDTOValidationSchemas_ValidateTagName(t *testing.T) {
	schemas := NewDTOValidationSchemas()

	tests := []struct {
		name    string
		tag     string
		wantErr bool
	}{
		{
			name:    "Valid tag",
			tag:     "Work",
			wantErr: false,
		},
		{
			name:    "Valid tag with spaces",
			tag:     "Work Project",
			wantErr: false,
		},
		{
			name:    "Valid tag with numbers",
			tag:     "Project 2024",
			wantErr: false,
		},
		{
			name:    "Empty tag",
			tag:     "",
			wantErr: true,
		},
		{
			name:    "Special characters",
			tag:     "Work@Home",
			wantErr: true,
		},
		{
			name:    "XSS attempt",
			tag:     "<script>alert('xss')</script>",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := schemas.ValidateTagName(tt.tag)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDTOValidationSchemas_ValidateURL(t *testing.T) {
	schemas := NewDTOValidationSchemas()

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "Valid HTTPS URL",
			url:     "https://example.com",
			wantErr: false,
		},
		{
			name:    "Valid HTTP URL",
			url:     "http://example.com/path",
			wantErr: false,
		},
		{
			name:    "Invalid - no protocol",
			url:     "example.com",
			wantErr: true,
		},
		{
			name:    "Invalid - javascript protocol",
			url:     "javascript:alert('xss')",
			wantErr: true,
		},
		{
			name:    "Invalid - data protocol",
			url:     "data:text/html,<script>alert('xss')</script>",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := schemas.ValidateURL(tt.url)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDTOValidationSchemas_ValidatePagination(t *testing.T) {
	schemas := NewDTOValidationSchemas()

	tests := []struct {
		name    string
		page    int
		limit   int
		wantErr bool
	}{
		{
			name:    "Valid pagination",
			page:    1,
			limit:   20,
			wantErr: false,
		},
		{
			name:    "Valid large page",
			page:    100,
			limit:   50,
			wantErr: false,
		},
		{
			name:    "Invalid - zero page",
			page:    0,
			limit:   20,
			wantErr: true,
		},
		{
			name:    "Invalid - negative limit",
			page:    1,
			limit:   -1,
			wantErr: true,
		},
		{
			name:    "Invalid - limit too large",
			page:    1,
			limit:   1000,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := schemas.ValidatePagination(tt.page, tt.limit)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDTOValidationSchemas_ValidateSearchQuery(t *testing.T) {
	schemas := NewDTOValidationSchemas()

	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{
			name:    "Valid query",
			query:   "search term",
			wantErr: false,
		},
		{
			name:    "Valid query with numbers",
			query:   "project 2024",
			wantErr: false,
		},
		{
			name:    "Empty query",
			query:   "",
			wantErr: true,
		},
		{
			name:    "SQL injection attempt",
			query:   "search' OR '1'='1",
			wantErr: true,
		},
		{
			name:    "XSS attempt",
			query:   "search<script>alert('xss')</script>",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := schemas.ValidateSearchQuery(tt.query)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateMIMEType(t *testing.T) {
	allowedTypes := []string{"image/jpeg", "image/png", "application/pdf"}

	tests := []struct {
		name     string
		mimeType string
		want     bool
	}{
		{
			name:     "Valid JPEG",
			mimeType: "image/jpeg",
			want:     true,
		},
		{
			name:     "Valid PNG",
			mimeType: "image/png",
			want:     true,
		},
		{
			name:     "Valid PDF",
			mimeType: "application/pdf",
			want:     true,
		},
		{
			name:     "Invalid type",
			mimeType: "application/x-executable",
			want:     false,
		},
		{
			name:     "Empty type",
			mimeType: "",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateMIMEType(tt.mimeType, allowedTypes)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestValidateFileSize(t *testing.T) {
	maxSize := int64(10 * 1024 * 1024) // 10MB

	tests := []struct {
		name string
		size int64
		want bool
	}{
		{
			name: "Valid size",
			size: 1024 * 1024, // 1MB
			want: true,
		},
		{
			name: "Max size",
			size: maxSize,
			want: true,
		},
		{
			name: "Too large",
			size: maxSize + 1,
			want: false,
		},
		{
			name: "Zero size",
			size: 0,
			want: false,
		},
		{
			name: "Negative size",
			size: -1,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateFileSize(tt.size, maxSize)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestValidateFileExtension(t *testing.T) {
	allowedExtensions := []string{"jpg", "png", "pdf", "txt"}

	tests := []struct {
		name     string
		filename string
		want     bool
	}{
		{
			name:     "Valid JPG",
			filename: "image.jpg",
			want:     true,
		},
		{
			name:     "Valid PNG",
			filename: "photo.png",
			want:     true,
		},
		{
			name:     "Valid PDF",
			filename: "document.pdf",
			want:     true,
		},
		{
			name:     "Invalid extension",
			filename: "script.exe",
			want:     false,
		},
		{
			name:     "No extension",
			filename: "filename",
			want:     false,
		},
		{
			name:     "Multiple dots",
			filename: "file.backup.txt",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateFileExtension(tt.filename, allowedExtensions)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestSanitizeTag(t *testing.T) {
	tests := []struct {
		name     string
		tag      string
		expected string
	}{
		{
			name:     "Clean tag",
			tag:      "Work",
			expected: "Work",
		},
		{
			name:     "Tag with whitespace",
			tag:      "  Work  ",
			expected: "Work",
		},
		{
			name:     "Tag with dangerous chars",
			tag:      "Work<script>",
			expected: "Work",
		},
		{
			name:     "Tag with quotes",
			tag:      "Work\"Project\"",
			expected: "WorkProject",
		},
		{
			name:     "Tag with ampersand",
			tag:      "Work&Home",
			expected: "WorkHome",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeTag(tt.tag)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeTags(t *testing.T) {
	tests := []struct {
		name     string
		tags     []string
		expected []string
	}{
		{
			name:     "Clean tags",
			tags:     []string{"Work", "Project", "Important"},
			expected: []string{"Work", "Project", "Important"},
		},
		{
			name:     "Tags with duplicates",
			tags:     []string{"Work", "work", "Work"},
			expected: []string{"Work", "work"},
		},
		{
			name:     "Tags with dangerous chars",
			tags:     []string{"Work<script>", "Project\"", "Important&Urgent"},
			expected: []string{"Work", "Project", "ImportantUrgent"},
		},
		{
			name:     "Tags with empty strings",
			tags:     []string{"Work", "", "Project", "   "},
			expected: []string{"Work", "Project"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeTags(tt.tags)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateArrayLength(t *testing.T) {
	tests := []struct {
		name   string
		arr    []string
		minLen int
		maxLen int
		want   bool
	}{
		{
			name:   "Valid length",
			arr:    []string{"a", "b", "c"},
			minLen: 1,
			maxLen: 5,
			want:   true,
		},
		{
			name:   "Too short",
			arr:    []string{},
			minLen: 1,
			maxLen: 5,
			want:   false,
		},
		{
			name:   "Too long",
			arr:    []string{"a", "b", "c", "d", "e", "f"},
			minLen: 1,
			maxLen: 5,
			want:   false,
		},
		{
			name:   "Exact min",
			arr:    []string{"a"},
			minLen: 1,
			maxLen: 5,
			want:   true,
		},
		{
			name:   "Exact max",
			arr:    []string{"a", "b", "c", "d", "e"},
			minLen: 1,
			maxLen: 5,
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateArrayLength(tt.arr, tt.minLen, tt.maxLen)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestBatchValidateEmails(t *testing.T) {
	schemas := NewDTOValidationSchemas()

	emails := []string{
		"valid@example.com",
		"invalid-email",
		"another@example.com",
		"bad<script>@example.com",
	}

	result := schemas.BatchValidateEmails(emails)

	assert.Len(t, result.Valid, 2)
	assert.Len(t, result.Invalid, 2)
	assert.Contains(t, result.Valid, "valid@example.com")
	assert.Contains(t, result.Valid, "another@example.com")
	assert.Contains(t, result.Invalid, "invalid-email")
	assert.Contains(t, result.Invalid, "bad<script>@example.com")
}

func TestBatchValidateTags(t *testing.T) {
	schemas := NewDTOValidationSchemas()

	tags := []string{
		"ValidTag",
		"Another Valid Tag",
		"<script>xss</script>",
		"Tag@Invalid",
	}

	result := schemas.BatchValidateTags(tags)

	assert.Len(t, result.Valid, 2)
	assert.Len(t, result.Invalid, 2)
	assert.Contains(t, result.Valid, "ValidTag")
	assert.Contains(t, result.Valid, "Another Valid Tag")
}

func TestDTOValidationSchemas_ValidateUUID(t *testing.T) {
	schemas := NewDTOValidationSchemas()

	tests := []struct {
		name    string
		id      uuid.UUID
		wantErr bool
	}{
		{
			name:    "Valid UUID",
			id:      uuid.New(),
			wantErr: false,
		},
		{
			name:    "Nil UUID",
			id:      uuid.Nil,
			wantErr: false, // Nil UUID is still a valid UUID structure
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := schemas.ValidateUUID(tt.id)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
