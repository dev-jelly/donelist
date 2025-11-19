package security

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidator_SafeUsername(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "Valid username",
			input:   "john_doe",
			wantErr: false,
		},
		{
			name:    "Valid username with numbers",
			input:   "user123",
			wantErr: false,
		},
		{
			name:    "Valid username with hyphen",
			input:   "john-doe",
			wantErr: false,
		},
		{
			name:    "Invalid - starts with underscore",
			input:   "_username",
			wantErr: true,
		},
		{
			name:    "Invalid - contains admin",
			input:   "admin123",
			wantErr: true,
		},
		{
			name:    "Invalid - contains root",
			input:   "rootuser",
			wantErr: true,
		},
		{
			name:    "Invalid - contains system",
			input:   "systemadmin",
			wantErr: true,
		},
		{
			name:    "Invalid - special characters",
			input:   "user@name",
			wantErr: true,
		},
		{
			name:    "Invalid - spaces",
			input:   "user name",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateVar(tt.input, "safe_username")
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_SafeMIMEType(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "Valid JPEG",
			input:   "image/jpeg",
			wantErr: false,
		},
		{
			name:    "Valid PNG",
			input:   "image/png",
			wantErr: false,
		},
		{
			name:    "Valid PDF",
			input:   "application/pdf",
			wantErr: false,
		},
		{
			name:    "Valid GIF",
			input:   "image/gif",
			wantErr: false,
		},
		{
			name:    "Valid WebP",
			input:   "image/webp",
			wantErr: false,
		},
		{
			name:    "Invalid - executable",
			input:   "application/x-executable",
			wantErr: true,
		},
		{
			name:    "Invalid - php",
			input:   "application/x-php",
			wantErr: true,
		},
		{
			name:    "Invalid - unknown",
			input:   "unknown/type",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateVar(tt.input, "safe_mime_type")
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_MaxFileSize(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		input   int64
		wantErr bool
	}{
		{
			name:    "Valid size - 1MB",
			input:   1024 * 1024,
			wantErr: false,
		},
		{
			name:    "Valid size - 5MB",
			input:   5 * 1024 * 1024,
			wantErr: false,
		},
		{
			name:    "Valid size - exactly 10MB",
			input:   10 * 1024 * 1024,
			wantErr: false,
		},
		{
			name:    "Invalid - exceeds 10MB",
			input:   11 * 1024 * 1024,
			wantErr: true,
		},
		{
			name:    "Invalid - zero size",
			input:   0,
			wantErr: true,
		},
		{
			name:    "Invalid - negative size",
			input:   -1,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateVar(tt.input, "max_file_size")
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_NoLDAPInjection(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "Clean input",
			input:   "johndoe",
			wantErr: false,
		},
		{
			name:    "LDAP wildcard",
			input:   "john*",
			wantErr: true,
		},
		{
			name:    "LDAP filter - parenthesis",
			input:   "(cn=john)",
			wantErr: true,
		},
		{
			name:    "LDAP OR operator",
			input:   "john|admin",
			wantErr: true,
		},
		{
			name:    "LDAP AND operator",
			input:   "john&admin",
			wantErr: true,
		},
		{
			name:    "LDAP NOT operator",
			input:   "!john",
			wantErr: true,
		},
		{
			name:    "LDAP equals",
			input:   "cn=admin",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateVar(tt.input, "no_ldap_injection")
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_NoCommandInjection(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "Clean input",
			input:   "normalfile.txt",
			wantErr: false,
		},
		{
			name:    "Command separator - semicolon",
			input:   "file.txt; rm -rf /",
			wantErr: true,
		},
		{
			name:    "Command separator - pipe",
			input:   "file.txt | cat",
			wantErr: true,
		},
		{
			name:    "Command separator - ampersand",
			input:   "file.txt & ls",
			wantErr: true,
		},
		{
			name:    "Command substitution",
			input:   "$(whoami)",
			wantErr: true,
		},
		{
			name:    "Backtick substitution",
			input:   "`whoami`",
			wantErr: true,
		},
		{
			name:    "Logical OR",
			input:   "cmd || ls",
			wantErr: true,
		},
		{
			name:    "Logical AND",
			input:   "cmd && ls",
			wantErr: true,
		},
		{
			name:    "Redirect output",
			input:   "cmd > file.txt",
			wantErr: true,
		},
		{
			name:    "Newline injection",
			input:   "cmd\nrm -rf",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateVar(tt.input, "no_command_injection")
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_SafeJSON(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "Clean JSON string",
			input:   `{"name": "John", "age": 30}`,
			wantErr: false,
		},
		{
			name:    "Prototype pollution attempt",
			input:   `{"__proto__": {"admin": true}}`,
			wantErr: true,
		},
		{
			name:    "Constructor pollution",
			input:   `{"constructor": {"admin": true}}`,
			wantErr: true,
		},
		{
			name:    "Prototype chain attack",
			input:   `{"prototype": {"isAdmin": true}}`,
			wantErr: true,
		},
		{
			name:    "Function injection",
			input:   `{"func": "function() { alert('xss') }"}`,
			wantErr: true,
		},
		{
			name:    "Eval attempt",
			input:   `{"code": "eval('malicious code')"}`,
			wantErr: true,
		},
		{
			name:    "setTimeout attempt",
			input:   `{"timer": "setTimeout(function() {}, 1000)"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateVar(tt.input, "safe_json")
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_CombinedValidations(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		input   string
		tags    string
		wantErr bool
	}{
		{
			name:    "Valid input with multiple validations",
			input:   "This is a clean input",
			tags:    "required,min=5,max=100,no_xss,no_sql_injection",
			wantErr: false,
		},
		{
			name:    "Invalid - XSS and SQL injection",
			input:   "<script>SELECT * FROM users</script>",
			tags:    "required,min=5,max=100,no_xss,no_sql_injection",
			wantErr: true,
		},
		{
			name:    "Invalid - too short",
			input:   "Hi",
			tags:    "required,min=5,max=100,no_xss,no_sql_injection",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateVar(tt.input, tt.tags)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_EdgeCases(t *testing.T) {
	v := NewValidator()

	t.Run("Empty string with no_xss", func(t *testing.T) {
		err := v.ValidateVar("", "no_xss")
		assert.NoError(t, err) // Empty string should pass no_xss
	})

	t.Run("Unicode characters", func(t *testing.T) {
		err := v.ValidateVar("Hello 世界", "no_xss,no_sql_injection")
		assert.NoError(t, err)
	})

	t.Run("Very long string", func(t *testing.T) {
		longString := string(make([]byte, 10000))
		err := v.ValidateVar(longString, "no_xss")
		assert.NoError(t, err)
	})

	t.Run("Mixed case SQL keywords", func(t *testing.T) {
		err := v.ValidateVar("SeLeCt * FrOm users", "no_sql_injection")
		assert.Error(t, err) // Should detect case-insensitive
	})

	t.Run("Mixed case XSS patterns", func(t *testing.T) {
		err := v.ValidateVar("<ScRiPt>alert('xss')</ScRiPt>", "no_xss")
		assert.Error(t, err) // Should detect case-insensitive
	})
}

func TestSanitizeHTML_Advanced(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Nested script tags",
			input:    "<div><script>alert('xss')</script></div>",
			expected: "",
		},
		{
			name:     "Multiple event handlers",
			input:    `<div onclick="alert(1)" onmouseover="alert(2)">Text</div>`,
			expected: "Text",
		},
		{
			name:     "Mixed content",
			input:    "Hello <b>World</b><script>alert('xss')</script> Test",
			expected: "Hello World Test",
		},
		{
			name:     "Case insensitive script tag",
			input:    "Hello <SCRIPT>alert('xss')</SCRIPT> World",
			expected: "Hello  World",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeHTML(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeInput_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		maxLength int
		expected  string
	}{
		{
			name:      "Multiple null bytes",
			input:     "hello\x00\x00world\x00",
			maxLength: 0,
			expected:  "helloworld",
		},
		{
			name:      "Only whitespace",
			input:     "     ",
			maxLength: 0,
			expected:  "",
		},
		{
			name:      "UTF-8 characters with length limit",
			input:     "Hello 世界 Test",
			maxLength: 8,
			expected:  "Hello 世界",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeInput(tt.input, tt.maxLength)
			assert.Equal(t, tt.expected, result)
		})
	}
}
