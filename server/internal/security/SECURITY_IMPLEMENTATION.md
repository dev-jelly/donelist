# Security Implementation Guide

## Overview

This document describes the comprehensive input validation and security implementation for the DoneList API server. The implementation provides multiple layers of defense against common web application attacks including SQL injection, XSS, command injection, and more.

## Architecture

### Multi-Layer Security Approach

```
┌─────────────────────────────────────────┐
│     HTTP Request                        │
└─────────────────┬───────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────┐
│  1. Security Headers Middleware         │
│     - CORS, CSP, HSTS, etc.            │
└─────────────────┬───────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────┐
│  2. Rate Limiting Middleware            │
│     - Prevent brute force              │
└─────────────────┬───────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────┐
│  3. Input Validation Middleware         │
│     - Strict validation rules          │
│     - Size limits, format checks       │
└─────────────────┬───────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────┐
│  4. Sanitization Middleware             │
│     - SQL injection detection          │
│     - XSS pattern removal              │
│     - HTML sanitization                │
└─────────────────┬───────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────┐
│  5. DTO Validation (Handler Level)     │
│     - Schema-based validation          │
│     - Business logic checks            │
└─────────────────┬───────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────┐
│  6. Parameterized Queries (ORM)        │
│     - SQLX/GORM with placeholders      │
│     - No string concatenation          │
└─────────────────┬───────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────┐
│     Database                            │
└─────────────────────────────────────────┘
```

## Components

### 1. Validation System (`validation.go`)

#### Core Validator
- Built on `go-playground/validator/v10`
- Custom validation rules for security
- Comprehensive error formatting

#### Custom Validation Tags

| Tag | Description | Example |
|-----|-------------|---------|
| `no_sql_injection` | Blocks SQL injection patterns | `field:"validate:no_sql_injection"` |
| `no_xss` | Blocks XSS attack patterns | `field:"validate:no_xss"` |
| `safe_path` | Prevents path traversal | `field:"validate:safe_path"` |
| `safe_filename` | Validates safe filenames | `field:"validate:safe_filename"` |
| `safe_url` | Validates URL safety | `field:"validate:safe_url"` |
| `safe_username` | Validates username format | `field:"validate:safe_username"` |
| `safe_mime_type` | Validates MIME types | `field:"validate:safe_mime_type"` |
| `max_file_size` | Validates file size limits | `field:"validate:max_file_size"` |
| `no_ldap_injection` | Blocks LDAP injection | `field:"validate:no_ldap_injection"` |
| `no_command_injection` | Blocks command injection | `field:"validate:no_command_injection"` |
| `safe_json` | Prevents prototype pollution | `field:"validate:safe_json"` |
| `no_control_chars` | Blocks control characters | `field:"validate:no_control_chars"` |
| `hex_color` | Validates hex color codes | `field:"validate:hex_color"` |
| `timezone` | Validates timezone strings | `field:"validate:timezone"` |

#### Sanitization Functions

```go
// Remove dangerous characters and limit length
SanitizeInput(input string, maxLength int) string

// Remove HTML tags and scripts
SanitizeHTML(input string) string

// Sanitize tag names
SanitizeTag(tag string) string

// Batch sanitize tags
SanitizeTags(tags []string) []string
```

### 2. DTO Validation Schemas (`dto_schemas.go`)

Reusable validation schemas for common input types:

```go
schemas := security.NewDTOValidationSchemas()

// Validate email
err := schemas.ValidateEmail("user@example.com")

// Validate username
err := schemas.ValidateUsername("john_doe")

// Validate text content
err := schemas.ValidateTextContent("Hello world")

// Validate pagination
err := schemas.ValidatePagination(1, 20)

// Batch validate
result := schemas.BatchValidateEmails([]string{"email1", "email2"})
```

#### Available DTO Schemas

- `UserEmailDTO` - Email validation
- `UserPasswordDTO` - Password strength
- `UsernameDTO` - Username format
- `TextContentDTO` - General text content
- `ShortTextDTO` - Titles, names, etc.
- `TagNameDTO` - Tag validation
- `URLInputDTO` - URL validation
- `FileUploadDTO` - File upload parameters
- `PaginationDTO` - Pagination parameters
- `SearchQueryDTO` - Search queries

### 3. SQL Security (`sql_security.go`)

#### SQL Security Auditor

```go
auditor := security.NewSQLSecurityAuditor()

// Validate identifier safety
safe := auditor.IsSafeIdentifier("users") // true
safe := auditor.IsSafeIdentifier("users; DROP TABLE") // false

// Quote identifiers safely
quoted, err := auditor.QuoteIdentifier("users") // "users"

// Safe ORDER BY clause
orderBy, err := security.SafeOrderBy("name", "ASC", allowedColumns)
```

#### Best Practices

1. **Always use parameterized queries**
   ```go
   // GOOD
   db.Query("SELECT * FROM users WHERE id = $1", userID)

   // BAD
   db.Query(fmt.Sprintf("SELECT * FROM users WHERE id = '%s'", userID))
   ```

2. **Whitelist dynamic identifiers**
   ```go
   allowedColumns := map[string]string{
       "name": "users.name",
       "created_at": "users.created_at",
   }
   orderBy, err := security.SafeOrderBy(column, direction, allowedColumns)
   ```

3. **Sanitize LIKE patterns**
   ```go
   pattern := security.SanitizeLikePattern(userInput)
   query, params := security.BuildSafeLikeQuery("name", pattern, false)
   ```

4. **Validate limits and offsets**
   ```go
   limit := security.ValidateLimit(userLimit, 100)
   offset := security.ValidateOffset(userOffset)
   ```

### 4. Input Validation Middleware (`middleware/validation.go`)

Automatically validates all incoming requests:

```go
// Create validation middleware
validationCfg := middleware.CreateDefaultValidationConfig(logger)
router.Use(middleware.InputValidationMiddleware(validationCfg))
```

**Features:**
- Validates content-type headers
- Enforces max body size (default: 10MB)
- Validates JSON structure
- Checks query parameters for attacks
- Validates custom headers
- Optional sanitization
- Strict mode for blocking attacks

### 5. Sanitization Middleware (`middleware/sanitization.go`)

Sanitizes inputs before they reach handlers:

```go
// Create sanitization middleware
sanitizeCfg := middleware.CreateDefaultSanitizationConfig(logger)
router.Use(middleware.SanitizationMiddleware(sanitizeCfg))

// Or use strict mode (rejects instead of sanitizing)
strictCfg := middleware.CreateStrictSanitizationConfig(logger)
```

**Features:**
- SQL injection detection
- XSS pattern detection
- HTML sanitization
- Query parameter sanitization
- Path parameter sanitization
- JSON body sanitization
- Strict mode vs. lenient mode

## Usage Examples

### Handler-Level Validation

```go
type CreateUserRequest struct {
    Email    string `json:"email" validate:"required,email,no_xss,no_sql_injection"`
    Username string `json:"username" validate:"required,min=3,max=50,safe_username"`
    Password string `json:"password" validate:"required,min=8,max=128,no_control_chars"`
    Tags     []string `json:"tags" validate:"max=10,dive,alphanumeric_spaces"`
}

func (h *Handler) CreateUser(c *gin.Context) {
    var req CreateUserRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
        return
    }

    // Additional validation with schemas
    schemas := security.NewDTOValidationSchemas()
    if err := schemas.ValidateEmail(req.Email); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    // Sanitize tags
    req.Tags = security.SanitizeTags(req.Tags)

    // Process request...
}
```

### File Upload Validation

```go
func (h *Handler) UploadFile(c *gin.Context) {
    file, err := c.FormFile("file")
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "file required"})
        return
    }

    // Validate file
    allowedTypes := []string{"image/jpeg", "image/png", "application/pdf"}
    if !security.ValidateMIMEType(file.Header.Get("Content-Type"), allowedTypes) {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file type"})
        return
    }

    maxSize := int64(10 * 1024 * 1024) // 10MB
    if !security.ValidateFileSize(file.Size, maxSize) {
        c.JSON(http.StatusBadRequest, gin.H{"error": "file too large"})
        return
    }

    allowedExts := []string{"jpg", "png", "pdf"}
    if !security.ValidateFileExtension(file.Filename, allowedExts) {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file extension"})
        return
    }

    // Process file...
}
```

### Database Query Example

```go
func (r *Repository) SearchUsers(ctx context.Context, query string, sortBy string, direction string, limit int, offset int) ([]User, error) {
    // Sanitize search query
    query = security.SanitizeLikePattern(query)

    // Validate sort parameters
    allowedColumns := map[string]string{
        "name": "users.name",
        "created_at": "users.created_at",
        "email": "users.email",
    }
    orderBy, err := security.SafeOrderBy(sortBy, direction, allowedColumns)
    if err != nil {
        return nil, err
    }

    // Validate pagination
    limit = security.ValidateLimit(limit, 100)
    offset = security.ValidateOffset(offset)

    // Use parameterized query
    sql := fmt.Sprintf(`
        SELECT * FROM users
        WHERE name ILIKE $1
        ORDER BY %s
        LIMIT $2 OFFSET $3
    `, orderBy)

    var users []User
    err = r.db.SelectContext(ctx, &users, sql, "%"+query+"%", limit, offset)
    return users, err
}
```

## Testing

### Running Security Tests

```bash
# Run all security tests
go test ./internal/security/... -v

# Run attack simulation tests
go test ./internal/security -run TestAttack -v

# Run specific validation tests
go test ./internal/security -run TestValidator -v

# Run with coverage
go test ./internal/security/... -cover -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Test Coverage

The security package includes comprehensive tests:

- **Validation Tests** (`validation_test.go`)
  - 444 lines of tests
  - All custom validators tested
  - Edge cases covered

- **Extended Validation Tests** (`validation_extended_test.go`)
  - Additional validator tests
  - Combined validation scenarios
  - Edge case testing

- **DTO Schema Tests** (`dto_schemas_test.go`)
  - All DTO schemas tested
  - Batch validation tests
  - Helper function tests

- **SQL Security Tests** (`sql_security_test.go`)
  - 445 lines of tests
  - Query validation tests
  - Injection detection tests

- **Attack Simulation Tests** (`attack_simulation_test.go`)
  - 300+ attack vectors
  - SQL injection scenarios
  - XSS attack patterns
  - Command injection tests
  - Path traversal tests
  - LDAP injection tests
  - Prototype pollution tests

## Security Checklist

### For New Endpoints

- [ ] Add request DTO with validation tags
- [ ] Validate all user inputs
- [ ] Use DTO schemas for common fields
- [ ] Sanitize text inputs
- [ ] Use parameterized queries only
- [ ] Whitelist dynamic SQL identifiers
- [ ] Validate file uploads (type, size, extension)
- [ ] Limit array/collection sizes
- [ ] Validate pagination parameters
- [ ] Add appropriate error messages (don't leak info)
- [ ] Write tests for attack vectors
- [ ] Document security considerations

### For Database Queries

- [ ] Use placeholders ($1, $2, etc.)
- [ ] Never concatenate user input
- [ ] Whitelist ORDER BY columns
- [ ] Validate sort directions (ASC/DESC only)
- [ ] Sanitize LIKE patterns
- [ ] Validate LIMIT and OFFSET
- [ ] Use ORM/query builder when possible
- [ ] Review for second-order injection

### For File Operations

- [ ] Validate MIME type
- [ ] Check file extension whitelist
- [ ] Enforce file size limits
- [ ] Validate filename (no path traversal)
- [ ] Scan for malware (if applicable)
- [ ] Store files outside web root
- [ ] Generate random filenames
- [ ] Set appropriate permissions

## Performance Considerations

### Validation Performance

- Validation is fast (<1ms per request typically)
- Regex compilation is done once at startup
- Use batch validation for multiple items
- Consider caching validation results for static data

### Optimization Tips

```go
// Pre-compile validators at startup
var globalValidator = security.NewValidator()
var globalSchemas = security.NewDTOValidationSchemas()

// Reuse in handlers
func (h *Handler) Create(c *gin.Context) {
    err := globalValidator.Validate(req)
    // ...
}
```

## Common Vulnerabilities Prevented

### ✅ SQL Injection
- Parameterized queries enforced
- Identifier whitelisting
- Pattern detection

### ✅ Cross-Site Scripting (XSS)
- HTML sanitization
- Script tag removal
- Event handler blocking
- Protocol validation

### ✅ Command Injection
- Shell metacharacter blocking
- Command separator detection
- Subprocess input validation

### ✅ Path Traversal
- Directory traversal prevention
- Filename validation
- Path sanitization

### ✅ LDAP Injection
- Filter character blocking
- Operator validation

### ✅ XML External Entity (XXE)
- Content-type validation
- Input size limits

### ✅ Prototype Pollution
- JSON pattern detection
- Property name validation

### ✅ File Upload Attacks
- MIME type validation
- Extension whitelisting
- Size limits
- Filename sanitization

## Maintenance

### Adding New Validators

1. Add validator function to `validation.go`:
```go
func validateNewCheck(fl validator.FieldLevel) bool {
    value := fl.Field().String()
    // Validation logic
    return isValid
}
```

2. Register in `NewValidator()`:
```go
v.RegisterValidation("new_check", validateNewCheck)
```

3. Add error message in `formatFieldError()`:
```go
case "new_check":
    return fmt.Sprintf("%s failed new check", field)
```

4. Add tests in appropriate test file

### Updating Attack Patterns

Update pattern lists in:
- `validation.go` - Core validators
- `sql_security.go` - SQL patterns
- `attack_simulation_test.go` - Test vectors

### Security Updates

1. Review OWASP Top 10 regularly
2. Update attack patterns quarterly
3. Run security scans monthly
4. Review failed validation logs
5. Update documentation

## References

- [OWASP Top 10](https://owasp.org/www-project-top-ten/)
- [OWASP SQL Injection Prevention](https://cheatsheetseries.owasp.org/cheatsheets/SQL_Injection_Prevention_Cheat_Sheet.html)
- [OWASP XSS Prevention](https://cheatsheetseries.owasp.org/cheatsheets/Cross_Site_Scripting_Prevention_Cheat_Sheet.html)
- [Go Validator Documentation](https://pkg.go.dev/github.com/go-playground/validator/v10)

## Support

For security issues or questions:
1. Check this documentation
2. Review test files for examples
3. Check inline code comments
4. Consult OWASP resources
5. Report security vulnerabilities privately
