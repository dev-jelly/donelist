# Security Package Documentation

## Overview

The security package provides comprehensive security hardening features for the Donelist API, including:

- Input validation and sanitization
- CSRF protection
- SQL injection prevention
- Account lockout for failed login attempts
- Security headers
- Audit logging

## Components

### 1. Input Validation (`validation.go`)

Provides custom validators and sanitization functions using `go-playground/validator/v10`.

#### Custom Validators

- `no_sql_injection` - Detects SQL injection patterns
- `no_xss` - Detects XSS attack patterns
- `safe_path` - Prevents directory traversal
- `safe_url` - Validates URL format and protocol
- `no_control_chars` - Ensures no control characters
- `safe_filename` - Validates filenames
- `hex_color` - Validates hex color format
- `timezone` - Validates timezone strings

#### Usage

```go
validator := security.NewValidator()

// Validate a struct
type UserInput struct {
    Email    string `validate:"required,email"`
    Name     string `validate:"required,no_xss,max=100"`
    Website  string `validate:"omitempty,safe_url"`
}

err := validator.Validate(userInput)

// Sanitize input
sanitized := security.SanitizeInput(userInput, 1000)
cleaned := security.SanitizeHTML(htmlContent)
```

### 2. CSRF Protection (`csrf.go`)

Implements double-submit cookie pattern for CSRF protection.

#### Features

- Token generation and validation
- Automatic token rotation
- Configurable TTL
- Session binding

#### Usage

```go
// Create CSRF token manager
tokenManager := security.NewCSRFTokenManager(
    "your-secret-key",
    1 * time.Hour, // TTL
)

// Generate token
token, err := tokenManager.GenerateToken(sessionID)

// Validate token
err = tokenManager.ValidateDoubleSubmitToken(headerToken, cookieToken, sessionID)
```

### 3. SQL Security (`sql_security.go`)

Tools for SQL injection prevention and safe query building.

#### Features

- Query validation
- Safe identifier quoting
- Whitelist-based sorting
- LIKE pattern sanitization
- Parameterized query helpers

#### Usage

```go
auditor := security.NewSQLSecurityAuditor()

// Validate identifier
if !auditor.IsSafeIdentifier("table_name") {
    return errors.New("invalid identifier")
}

// Safe ORDER BY
orderBy, err := security.SafeOrderBy("created_at", "DESC", allowedColumns)

// Sanitize LIKE pattern
pattern := security.SanitizeLikePattern(userInput)
```

### 4. Account Lockout (`lockout.go`)

Prevents brute force attacks with progressive delays and account lockout.

#### Configuration

```go
lockoutManager := security.NewLockoutManager(security.LockoutConfig{
    RedisClient:     redisClient,
    MaxAttempts:     5,                  // Lock after 5 failed attempts
    LockoutDuration: 15 * time.Minute,   // Lock for 15 minutes
    AttemptWindow:   30 * time.Minute,   // Track attempts over 30 minutes
})
```

#### Usage

```go
// Record failed login
err := lockoutManager.RecordFailedAttempt(ctx, email)

// Check if locked out
isLocked, unlockTime, err := lockoutManager.IsLockedOut(ctx, email)

// Record successful login (clears attempts)
err := lockoutManager.RecordSuccessfulAttempt(ctx, email)

// Get lockout info
info, err := lockoutManager.GetInfo(ctx, email)
```

## Middleware

### 1. Validation Middleware (`middleware/validation.go`)

Validates and sanitizes all incoming requests.

```go
validationCfg := middleware.CreateDefaultValidationConfig(logger)
router.Use(middleware.InputValidationMiddleware(validationCfg))
```

### 2. CSRF Middleware (`middleware/csrf.go`)

Protects against CSRF attacks on state-changing operations.

```go
csrfCfg := middleware.CreateDefaultCSRFConfig(tokenManager, logger, isProduction)
router.Use(middleware.CSRFMiddleware(csrfCfg))
```

### 3. Security Headers Middleware (`middleware/security_headers.go`)

Adds comprehensive security headers to all responses.

```go
headersCfg := middleware.CreateDefaultSecurityHeadersConfig(logger, isProduction)
router.Use(middleware.SecurityHeadersMiddleware(headersCfg))
```

### 4. Lockout Middleware (`middleware/lockout.go`)

Protects login endpoints from brute force attacks.

```go
lockoutCfg := middleware.CreateDefaultLockoutConfig(lockoutManager, auditService, logger)
router.Use(middleware.LockoutMiddleware(lockoutCfg))
```

### 5. Audit Middleware (`middleware/audit.go`)

Automatically logs security-relevant events.

```go
auditCfg := middleware.CreateDefaultAuditConfig(auditService, logger)
router.Use(middleware.AuditMiddleware(auditCfg))
```

## Security Headers

The following security headers are automatically added:

### Content Security Policy (CSP)
```
default-src 'self'
script-src 'self' 'unsafe-inline' 'unsafe-eval'
style-src 'self' 'unsafe-inline'
img-src 'self' data: https:
frame-ancestors 'none'
```

### HTTP Strict Transport Security (HSTS)
```
Strict-Transport-Security: max-age=31536000; includeSubDomains
```

### Other Headers
- `X-Frame-Options: DENY`
- `X-Content-Type-Options: nosniff`
- `X-XSS-Protection: 1; mode=block`
- `Referrer-Policy: strict-origin-when-cross-origin`
- `Cross-Origin-Embedder-Policy: require-corp`
- `Cross-Origin-Opener-Policy: same-origin`
- `Cross-Origin-Resource-Policy: same-origin`

## Best Practices

### 1. Always Use Parameterized Queries

```go
// GOOD
db.Query("SELECT * FROM users WHERE id = $1", userID)

// BAD
db.Query(fmt.Sprintf("SELECT * FROM users WHERE id = '%s'", userID))
```

### 2. Validate All User Input

```go
type CreateUserInput struct {
    Email    string `validate:"required,email,no_xss"`
    Name     string `validate:"required,min=1,max=100,no_sql_injection"`
    Password string `validate:"required,min=8"`
}
```

### 3. Use CSRF Protection for State-Changing Operations

All POST, PUT, PATCH, DELETE requests should require valid CSRF tokens.

### 4. Implement Rate Limiting

Combine account lockout with rate limiting for comprehensive protection.

### 5. Enable Audit Logging

Log all security-relevant events for compliance and incident response.

### 6. Keep Dependencies Updated

Regularly update security-related dependencies:
- `go-playground/validator`
- `golang.org/x/crypto`
- Database drivers

## Testing

### Run Security Tests

```bash
go test ./internal/security/... -v
go test ./internal/middleware/... -v
```

### Security Scanning

```bash
# OWASP ZAP
zap-cli quick-scan http://localhost:8080

# Go security checker
gosec ./...

# Dependency vulnerability check
go list -json -m all | nancy sleuth
```

## Monitoring

### Key Metrics to Monitor

1. Failed login attempts
2. CSRF validation failures
3. Rate limit exceedances
4. SQL injection attempts
5. Account lockouts

### Alerts

Set up alerts for:
- Spike in failed login attempts
- Multiple CSRF failures from same IP
- Repeated SQL injection patterns
- Sustained rate limit violations

## Compliance

This security implementation helps meet requirements for:

- OWASP Top 10 protection
- GDPR (audit logging)
- SOC 2 (security controls)
- HIPAA (access controls and audit trails)

## Configuration Examples

### Production Configuration

```go
// Strict validation
validationCfg := middleware.ValidationConfig{
    Logger:           logger,
    MaxBodySize:      1 * 1024 * 1024, // 1MB
    SanitizeInputs:   true,
    StrictValidation: true,
}

// Strong CSRF protection
csrfCfg := middleware.CSRFConfig{
    TokenManager:   tokenManager,
    CookieSecure:   true,  // HTTPS only
    CookieSameSite: http.SameSiteStrictMode,
}

// Aggressive lockout
lockoutCfg := security.LockoutConfig{
    MaxAttempts:     3,
    LockoutDuration: 30 * time.Minute,
    AttemptWindow:   15 * time.Minute,
}
```

### Development Configuration

```go
// Relaxed for development
validationCfg := middleware.ValidationConfig{
    Logger:           logger,
    MaxBodySize:      10 * 1024 * 1024, // 10MB
    SanitizeInputs:   false,
    StrictValidation: false,
}

// Permissive CSRF for local testing
csrfCfg := middleware.CSRFConfig{
    TokenManager:   tokenManager,
    CookieSecure:   false,  // Allow HTTP
    CookieSameSite: http.SameSiteLaxMode,
}
```

## Troubleshooting

### CSRF Token Mismatch

- Ensure cookies are enabled
- Check `SameSite` cookie settings
- Verify HTTPS in production
- Check for clock skew

### Account Lockout Issues

- Verify Redis connectivity
- Check lockout duration settings
- Ensure failed attempts are cleared on success
- Use admin unlock endpoint if needed

### False Positive Validation

- Review custom validators
- Adjust validation rules
- Check for encoding issues
- Sanitize before validation

## Security Incident Response

1. **Detection**: Monitor audit logs and alerts
2. **Containment**: Use account lockout and rate limiting
3. **Investigation**: Query audit logs by user, IP, or event type
4. **Remediation**: Fix vulnerability, reset affected accounts
5. **Documentation**: Record incident in audit logs

## References

- [OWASP Cheat Sheet Series](https://cheatsheetseries.owasp.org/)
- [Go Security](https://golang.org/doc/security)
- [NIST Cybersecurity Framework](https://www.nist.gov/cyberframework)
