package security

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidator_NoSQLInjection(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "Clean input",
			input:   "Hello World",
			wantErr: false,
		},
		{
			name:    "SQL injection with single quote",
			input:   "admin' OR '1'='1",
			wantErr: true,
		},
		{
			name:    "SQL injection with union",
			input:   "1 UNION SELECT * FROM users",
			wantErr: true,
		},
		{
			name:    "SQL injection with drop",
			input:   "DROP TABLE users",
			wantErr: true,
		},
		{
			name:    "SQL comment injection",
			input:   "admin'--",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateVar(tt.input, "no_sql_injection")
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_NoXSS(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "Clean input",
			input:   "Hello World",
			wantErr: false,
		},
		{
			name:    "XSS with script tag",
			input:   "<script>alert('xss')</script>",
			wantErr: true,
		},
		{
			name:    "XSS with javascript protocol",
			input:   "javascript:alert('xss')",
			wantErr: true,
		},
		{
			name:    "XSS with onerror",
			input:   "<img src=x onerror=alert('xss')>",
			wantErr: true,
		},
		{
			name:    "XSS with iframe",
			input:   "<iframe src='evil.com'></iframe>",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateVar(tt.input, "no_xss")
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_SafePath(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "Safe path",
			input:   "documents/report.pdf",
			wantErr: false,
		},
		{
			name:    "Path traversal with ..",
			input:   "../../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "Path with backslash",
			input:   "..\\..\\windows\\system32",
			wantErr: true,
		},
		{
			name:    "Path with tilde",
			input:   "~/secrets",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateVar(tt.input, "safe_path")
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_SafeFilename(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "Safe filename",
			input:   "report.pdf",
			wantErr: false,
		},
		{
			name:    "Filename with path",
			input:   "../../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "Filename with colon",
			input:   "file:name.txt",
			wantErr: true,
		},
		{
			name:    "Empty filename",
			input:   "",
			wantErr: true,
		},
		{
			name:    "Just dots",
			input:   "..",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateVar(tt.input, "safe_filename")
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_SafeURL(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "Valid HTTP URL",
			input:   "http://example.com",
			wantErr: false,
		},
		{
			name:    "Valid HTTPS URL",
			input:   "https://example.com/path",
			wantErr: false,
		},
		{
			name:    "JavaScript protocol",
			input:   "javascript:alert('xss')",
			wantErr: true,
		},
		{
			name:    "Data protocol",
			input:   "data:text/html,<script>alert('xss')</script>",
			wantErr: true,
		},
		{
			name:    "File protocol",
			input:   "file:///etc/passwd",
			wantErr: true,
		},
		{
			name:    "No protocol",
			input:   "example.com",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateVar(tt.input, "safe_url")
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_HexColor(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "Valid 6-digit hex",
			input:   "#FF5733",
			wantErr: false,
		},
		{
			name:    "Valid 3-digit hex",
			input:   "#F57",
			wantErr: false,
		},
		{
			name:    "Invalid hex no hash",
			input:   "FF5733",
			wantErr: true,
		},
		{
			name:    "Invalid hex too long",
			input:   "#FF57331",
			wantErr: true,
		},
		{
			name:    "Invalid hex characters",
			input:   "#GG5733",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateVar(tt.input, "hex_color")
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_HSLColor(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "Valid HSL with commas",
			input:   "hsl(120, 100%, 50%)",
			wantErr: false,
		},
		{
			name:    "Valid HSL without commas",
			input:   "hsl(120 100% 50%)",
			wantErr: false,
		},
		{
			name:    "Valid HSL with decimals",
			input:   "hsl(120.5, 99.9%, 50.1%)",
			wantErr: false,
		},
		{
			name:    "Invalid HSL missing percent",
			input:   "hsl(120, 100, 50)",
			wantErr: true,
		},
		{
			name:    "Invalid HSL missing parentheses",
			input:   "hsl 120, 100%, 50%",
			wantErr: true,
		},
		{
			name:    "Invalid HSL wrong format",
			input:   "120, 100%, 50%",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateVar(tt.input, "hsl_color")
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_Color(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "Valid hex color",
			input:   "#FF5733",
			wantErr: false,
		},
		{
			name:    "Valid HSL color",
			input:   "hsl(120, 100%, 50%)",
			wantErr: false,
		},
		{
			name:    "Valid 3-digit hex",
			input:   "#F57",
			wantErr: false,
		},
		{
			name:    "Invalid RGB format",
			input:   "rgb(255, 87, 51)",
			wantErr: true,
		},
		{
			name:    "Invalid format",
			input:   "red",
			wantErr: true,
		},
		{
			name:    "Empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateVar(tt.input, "color")
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSanitizeInput(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		maxLength int
		expected  string
	}{
		{
			name:      "Remove null bytes",
			input:     "hello\x00world",
			maxLength: 0,
			expected:  "helloworld",
		},
		{
			name:      "Trim whitespace",
			input:     "  hello world  ",
			maxLength: 0,
			expected:  "hello world",
		},
		{
			name:      "Limit length",
			input:     "this is a very long string",
			maxLength: 10,
			expected:  "this is a ",
		},
		{
			name:      "No changes needed",
			input:     "hello",
			maxLength: 10,
			expected:  "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeInput(tt.input, tt.maxLength)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeHTML(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Remove script tags",
			input:    "Hello <script>alert('xss')</script> World",
			expected: "Hello  World",
		},
		{
			name:     "Remove HTML tags",
			input:    "<p>Hello <b>World</b></p>",
			expected: "Hello World",
		},
		{
			name:     "Remove event handlers",
			input:    `<div onclick="alert('xss')">Click me</div>`,
			expected: "Click me",
		},
		{
			name:     "Clean text unchanged",
			input:    "Hello World",
			expected: "Hello World",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeHTML(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsValidUUID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "Valid UUID",
			input: "550e8400-e29b-41d4-a716-446655440000",
			want:  true,
		},
		{
			name:  "Invalid UUID - no dashes",
			input: "550e8400e29b41d4a716446655440000",
			want:  false,
		},
		{
			name:  "Invalid UUID - wrong format",
			input: "not-a-uuid",
			want:  false,
		},
		{
			name:  "Empty string",
			input: "",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidUUID(tt.input)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestIsValidEmail(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "Valid email",
			input: "user@example.com",
			want:  true,
		},
		{
			name:  "Valid email with subdomain",
			input: "user@mail.example.com",
			want:  true,
		},
		{
			name:  "Invalid email - no @",
			input: "userexample.com",
			want:  false,
		},
		{
			name:  "Invalid email - no domain",
			input: "user@",
			want:  false,
		},
		{
			name:  "Invalid email - no TLD",
			input: "user@example",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidEmail(tt.input)
			assert.Equal(t, tt.want, result)
		})
	}
}
