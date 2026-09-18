# Authentication & Security Guide

Comprehensive guide to authentication flows, security best practices, and implementation details for the DoneList API.

## Table of Contents

1. [Authentication Overview](#authentication-overview)
2. [Authentication Flows](#authentication-flows)
3. [JWT Token Management](#jwt-token-management)
4. [Password Security](#password-security)
5. [API Security](#api-security)
6. [Rate Limiting](#rate-limiting)
7. [Security Best Practices](#security-best-practices)
8. [Threat Mitigation](#threat-mitigation)
9. [Compliance](#compliance)

## Authentication Overview

### Authentication Methods

DoneList API uses **JWT (JSON Web Tokens)** for authentication:

- **Access Token**: Short-lived (15 minutes), used for API requests
- **Refresh Token**: Long-lived (7 days), used to obtain new access tokens

### Token Flow

```
┌─────────┐                 ┌─────────┐                 ┌──────────┐
│ Client  │                 │   API   │                 │ Database │
└────┬────┘                 └────┬────┘                 └────┬─────┘
     │                           │                           │
     │  POST /auth/login         │                           │
     │──────────────────────────>│                           │
     │                           │   Verify credentials      │
     │                           │──────────────────────────>│
     │                           │<──────────────────────────│
     │                           │                           │
     │   Access + Refresh Token  │                           │
     │<──────────────────────────│                           │
     │                           │                           │
     │  GET /api/resource        │                           │
     │  + Authorization: Bearer  │                           │
     │──────────────────────────>│                           │
     │                           │   Validate JWT            │
     │                           │   Extract user context    │
     │                           │                           │
     │   Resource data           │                           │
     │<──────────────────────────│                           │
```

## Authentication Flows

### 1. User Registration

```
┌─────────┐                 ┌─────────┐
│ Client  │                 │   API   │
└────┬────┘                 └────┬────┘
     │                           │
     │ POST /auth/register       │
     │ {                         │
     │   email,                  │
     │   password,               │
     │   display_name            │
     │ }                         │
     │──────────────────────────>│
     │                           │
     │                           ├─> Validate email format
     │                           ├─> Check password strength
     │                           ├─> Check email uniqueness
     │                           ├─> Hash password (bcrypt)
     │                           ├─> Create user record
     │                           ├─> Generate JWT tokens
     │                           │
     │   {                       │
     │     user,                 │
     │     access_token,         │
     │     refresh_token         │
     │   }                       │
     │<──────────────────────────│
```

**Request:**
```json
POST /api/v1/auth/register
{
  "email": "user@example.com",
  "password": "SecurePass123!",
  "display_name": "John Doe"
}
```

**Response:**
```json
{
  "user": {
    "id": "uuid",
    "email": "user@example.com",
    "display_name": "John Doe",
    "premium_tier": "free"
  },
  "access_token": "eyJhbG...",
  "refresh_token": "eyJhbG...",
  "expires_at": "2025-11-24T11:00:00Z"
}
```

### 2. User Login

```
┌─────────┐                 ┌─────────┐
│ Client  │                 │   API   │
└────┬────┘                 └────┬────┘
     │                           │
     │ POST /auth/login          │
     │ {                         │
     │   email,                  │
     │   password                │
     │ }                         │
     │──────────────────────────>│
     │                           │
     │                           ├─> Verify email exists
     │                           ├─> Verify password (bcrypt.Compare)
     │                           ├─> Generate new JWT pair
     │                           ├─> Store refresh token
     │                           ├─> Update last_login timestamp
     │                           │
     │   {                       │
     │     access_token,         │
     │     refresh_token         │
     │   }                       │
     │<──────────────────────────│
```

**Security Measures:**
- Failed login attempts tracked
- Account lockout after 5 failed attempts (15 min)
- Rate limiting: 10 requests per minute per IP
- Password verification uses constant-time comparison

### 3. Token Refresh

```
┌─────────┐                 ┌─────────┐
│ Client  │                 │   API   │
└────┬────┘                 └────┬────┘
     │                           │
     │ POST /auth/refresh        │
     │ {                         │
     │   refresh_token           │
     │ }                         │
     │──────────────────────────>│
     │                           │
     │                           ├─> Validate refresh token signature
     │                           ├─> Check token not revoked
     │                           ├─> Verify expiration
     │                           ├─> Generate new access token
     │                           ├─> Optionally rotate refresh token
     │                           │
     │   {                       │
     │     access_token,         │
     │     refresh_token (new)   │
     │   }                       │
     │<──────────────────────────│
```

**Refresh Token Rotation:**
- New refresh token issued with each refresh
- Old refresh token invalidated
- Prevents token reuse attacks

### 4. Logout

```
┌─────────┐                 ┌─────────┐
│ Client  │                 │   API   │
└────┬────┘                 └────┬────┘
     │                           │
     │ POST /auth/logout         │
     │ Authorization: Bearer     │
     │──────────────────────────>│
     │                           │
     │                           ├─> Revoke refresh token
     │                           ├─> Blacklist access token (optional)
     │                           │
     │   204 No Content          │
     │<──────────────────────────│
     │                           │
     │ POST /auth/logout-all     │
     │──────────────────────────>│
     │                           │
     │                           ├─> Revoke ALL user's refresh tokens
     │                           ├─> Clear all sessions
     │                           │
     │   204 No Content          │
     │<──────────────────────────│
```

## JWT Token Management

### Token Structure

**Access Token Claims:**
```json
{
  "sub": "user-uuid",
  "email": "user@example.com",
  "premium_tier": "free",
  "iat": 1700000000,
  "exp": 1700000900,
  "jti": "token-uuid"
}
```

**Refresh Token Claims:**
```json
{
  "sub": "user-uuid",
  "type": "refresh",
  "iat": 1700000000,
  "exp": 1700604800,
  "jti": "token-uuid"
}
```

### Token Validation

```go
// Middleware validates access tokens
func AuthMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // 1. Extract token from Authorization header
        authHeader := r.Header.Get("Authorization")
        if !strings.HasPrefix(authHeader, "Bearer ") {
            unauthorized(w)
            return
        }
        tokenString := strings.TrimPrefix(authHeader, "Bearer ")

        // 2. Parse and validate JWT
        token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
            // Verify signing algorithm
            if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
                return nil, fmt.Errorf("unexpected signing method")
            }
            return []byte(jwtSecret), nil
        })

        if err != nil || !token.Valid {
            unauthorized(w)
            return
        }

        // 3. Extract claims
        claims := token.Claims.(jwt.MapClaims)
        userID := claims["sub"].(string)

        // 4. Add user context to request
        ctx := context.WithValue(r.Context(), "user_id", userID)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

### Token Storage

**DO:**
- ✅ Store tokens in httpOnly cookies (web)
- ✅ Store tokens in secure keychain (mobile)
- ✅ Store tokens in memory only (SPA)

**DON'T:**
- ❌ Store tokens in localStorage (XSS vulnerable)
- ❌ Store tokens in sessionStorage
- ❌ Include tokens in URLs

## Password Security

### Password Requirements

```
Minimum Requirements:
├── Length: 12-72 characters
├── Character Types: At least 3 of:
│   ├── Uppercase letters (A-Z)
│   ├── Lowercase letters (a-z)
│   ├── Digits (0-9)
│   └── Special characters (!@#$%^&*)
└── Not in common weak passwords list
```

### Password Hashing

```go
// Using bcrypt with cost factor 12
func HashPassword(password string) (string, error) {
    // Validate password requirements
    if err := IsValidPassword(password); err != nil {
        return "", err
    }

    // Generate hash with cost 12 (2^12 iterations)
    hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
    if err != nil {
        return "", err
    }

    return string(hash), nil
}

// Constant-time password verification
func VerifyPassword(password, hash string) error {
    err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
    if err != nil {
        if err == bcrypt.ErrMismatchedHashAndPassword {
            return ErrInvalidPassword
        }
        return err
    }
    return nil
}
```

### Password Reset Flow

```
┌─────────┐                 ┌─────────┐                 ┌────────┐
│  User   │                 │   API   │                 │  Email │
└────┬────┘                 └────┬────┘                 └───┬────┘
     │                           │                          │
     │ POST /auth/forgot-password│                          │
     │──────────────────────────>│                          │
     │                           │                          │
     │                           ├─> Generate reset token   │
     │                           ├─> Store token (1 hour)   │
     │                           │                          │
     │                           │  Send reset email        │
     │                           │─────────────────────────>│
     │                           │                          │
     │   Reset email sent        │                          │
     │<──────────────────────────│                          │
     │                           │                          │
     │ Click reset link          │                          │
     │ GET /reset/{token}        │                          │
     │──────────────────────────>│                          │
     │                           │                          │
     │   Reset form              │                          │
     │<──────────────────────────│                          │
     │                           │                          │
     │ POST /auth/reset-password │                          │
     │ { token, new_password }   │                          │
     │──────────────────────────>│                          │
     │                           │                          │
     │                           ├─> Validate token         │
     │                           ├─> Hash new password      │
     │                           ├─> Update user password   │
     │                           ├─> Invalidate token       │
     │                           ├─> Revoke all sessions    │
     │                           │                          │
     │   Password reset          │                          │
     │<──────────────────────────│                          │
```

**Security Features:**
- Reset tokens expire after 1 hour
- Tokens are single-use only
- Password change revokes all sessions
- Rate limited to 3 requests per hour

## API Security

### HTTPS/TLS

**Production Requirements:**
- ✅ TLS 1.2 or higher only
- ✅ Strong cipher suites
- ✅ Certificate from trusted CA
- ✅ HSTS header enabled
- ✅ Perfect Forward Secrecy

```nginx
# Nginx configuration
ssl_protocols TLSv1.2 TLSv1.3;
ssl_ciphers 'ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256...';
ssl_prefer_server_ciphers on;

add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
```

### CORS Configuration

```go
// CORS middleware
cors.New(cors.Options{
    AllowedOrigins: []string{
        "https://app.donelist.io",
        "https://admin.donelist.io",
    },
    AllowedMethods: []string{
        "GET",
        "POST",
        "PUT",
        "PATCH",
        "DELETE",
        "OPTIONS",
    },
    AllowedHeaders: []string{
        "Authorization",
        "Content-Type",
        "X-Request-ID",
    },
    ExposedHeaders: []string{
        "X-RateLimit-Limit",
        "X-RateLimit-Remaining",
        "X-RateLimit-Reset",
    },
    AllowCredentials: true,
    MaxAge:           300,
})
```

### Security Headers

```go
// Security headers middleware
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Prevent clickjacking
        w.Header().Set("X-Frame-Options", "DENY")

        // Prevent MIME sniffing
        w.Header().Set("X-Content-Type-Options", "nosniff")

        // Enable XSS filter
        w.Header().Set("X-XSS-Protection", "1; mode=block")

        // Content Security Policy
        w.Header().Set("Content-Security-Policy",
            "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'")

        // Referrer policy
        w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

        // Permissions policy
        w.Header().Set("Permissions-Policy",
            "geolocation=(), microphone=(), camera=()")

        next.ServeHTTP(w, r)
    })
}
```

### Input Validation

```go
// Request validation
func ValidateCreateCheckinRequest(req *CreateCheckinRequest) error {
    // Title validation
    if len(req.Title) < 1 || len(req.Title) > 200 {
        return ErrInvalidTitle
    }

    // HTML sanitization
    req.Title = bluemonday.StrictPolicy().Sanitize(req.Title)
    if req.Description != nil {
        *req.Description = bluemonday.UGCPolicy().Sanitize(*req.Description)
    }

    // SQL injection prevention (using parameterized queries)
    // XSS prevention (HTML escaping)
    // Path traversal prevention (UUID validation)

    return nil
}
```

### SQL Injection Prevention

```go
// ✅ SAFE: Parameterized queries
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*Checkin, error) {
    query := `SELECT * FROM checkins WHERE id = $1 AND user_id = $2`
    var checkin Checkin
    err := r.db.GetContext(ctx, &checkin, query, id, userID)
    return &checkin, err
}

// ❌ UNSAFE: String concatenation
// func GetByID(id string) {
//     query := "SELECT * FROM checkins WHERE id = '" + id + "'"
//     // Vulnerable to SQL injection!
// }
```

## Rate Limiting

### Rate Limit Tiers

| Tier | Requests per Hour | Burst |
|------|-------------------|-------|
| Anonymous | 100 | 10 |
| Authenticated | 1,000 | 50 |
| Premium | 5,000 | 100 |
| Admin | 10,000 | 200 |

### Implementation

```go
// Rate limiter middleware
func RateLimitMiddleware(limiter *cache.RateLimiter) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Get identifier (user ID or IP)
            identifier := getUserID(r)
            if identifier == "" {
                identifier = getClientIP(r)
            }

            // Check rate limit
            if !limiter.Allow(identifier) {
                w.Header().Set("Retry-After", "60")
                respondError(w, http.StatusTooManyRequests, "Rate limit exceeded")
                return
            }

            // Add rate limit headers
            limit, remaining, reset := limiter.GetLimitInfo(identifier)
            w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
            w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
            w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset, 10))

            next.ServeHTTP(w, r)
        })
    }
}
```

### Response Headers

```
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 999
X-RateLimit-Reset: 1700000000
Retry-After: 60
```

## Security Best Practices

### For API Developers

1. **Always Use HTTPS**
   - Never send tokens over HTTP
   - Enforce HTTPS in production

2. **Validate All Input**
   - Sanitize HTML content
   - Validate data types
   - Check length limits
   - Use allow-lists, not deny-lists

3. **Use Parameterized Queries**
   - Never concatenate SQL
   - Use ORM or prepared statements

4. **Implement Proper Error Handling**
   - Don't leak sensitive information
   - Log errors securely
   - Return generic error messages

5. **Keep Dependencies Updated**
   - Regularly update dependencies
   - Monitor security advisories
   - Use `go mod verify`

### For API Consumers

1. **Store Tokens Securely**
   - Use secure storage mechanisms
   - Never log tokens
   - Rotate tokens regularly

2. **Implement Token Refresh**
   - Refresh before expiration
   - Handle refresh failures gracefully

3. **Handle Errors Properly**
   - Check for 401/403 responses
   - Implement retry logic
   - Respect rate limits

4. **Validate Responses**
   - Check response signatures
   - Validate data types
   - Handle malformed responses

## Threat Mitigation

### Cross-Site Request Forgery (CSRF)

**Protection:**
- JWT tokens in Authorization header (not cookies)
- SameSite cookie attribute if using cookies
- CSRF tokens for state-changing operations

### Cross-Site Scripting (XSS)

**Protection:**
- HTML sanitization (bluemonday)
- Content Security Policy headers
- Output encoding
- Input validation

### Man-in-the-Middle (MITM)

**Protection:**
- Enforce HTTPS/TLS
- HSTS headers
- Certificate pinning (mobile apps)
- Perfect Forward Secrecy

### Brute Force Attacks

**Protection:**
- Rate limiting
- Account lockout (5 failed attempts)
- CAPTCHA after multiple failures
- Strong password requirements

### Session Hijacking

**Protection:**
- Short-lived access tokens
- Refresh token rotation
- Logout revokes tokens
- IP/User-Agent validation (optional)

### API Abuse

**Protection:**
- Rate limiting per user/IP
- Request size limits
- Query complexity limits
- Monitoring and alerting

## Compliance

### GDPR Compliance

- ✅ User consent for data collection
- ✅ Right to access personal data
- ✅ Right to deletion (account deletion)
- ✅ Right to data portability (export feature)
- ✅ Data encryption at rest and in transit
- ✅ Security incident notification

### Data Encryption

**At Rest:**
- Database encryption (PostgreSQL encryption)
- File encryption for exports
- Encrypted backups

**In Transit:**
- TLS 1.2+ for all connections
- Encrypted database connections
- Encrypted Redis connections

### Audit Logging

```go
// Security event logging
func LogSecurityEvent(ctx context.Context, event SecurityEvent) {
    log := SecurityLog{
        UserID:    getUserID(ctx),
        EventType: event.Type,
        IPAddress: getClientIP(ctx),
        UserAgent: getUserAgent(ctx),
        Timestamp: time.Now(),
        Details:   event.Details,
    }

    // Store in audit log table
    auditRepo.Create(ctx, &log)

    // Alert on critical events
    if event.IsCritical {
        alerting.Notify(log)
    }
}
```

### Security Events Tracked

- Failed login attempts
- Successful logins
- Password changes
- Password resets
- Account deletions
- Permission changes
- API key usage
- Rate limit violations

## Security Monitoring

### Metrics to Monitor

1. **Authentication Metrics**
   - Failed login rate
   - Account lockouts
   - Password reset requests

2. **API Metrics**
   - Rate limit violations
   - 401/403 error rates
   - Unusual traffic patterns

3. **Security Events**
   - Multiple failed logins from same IP
   - Brute force attempts
   - Unusual API usage patterns

### Alerting Rules

```yaml
# Example Prometheus alerting rules
groups:
  - name: security
    rules:
      - alert: HighFailedLoginRate
        expr: rate(auth_failed_login_total[5m]) > 10
        for: 5m
        annotations:
          summary: "High failed login rate detected"

      - alert: RateLimitViolations
        expr: rate(rate_limit_exceeded_total[5m]) > 100
        for: 5m
        annotations:
          summary: "High rate of rate limit violations"
```

## Incident Response

### Security Incident Checklist

1. **Detect & Assess**
   - [ ] Identify the incident type
   - [ ] Assess impact and scope
   - [ ] Document findings

2. **Contain**
   - [ ] Revoke compromised tokens
   - [ ] Block malicious IPs
   - [ ] Isolate affected systems

3. **Eradicate**
   - [ ] Remove malicious code
   - [ ] Patch vulnerabilities
   - [ ] Update credentials

4. **Recover**
   - [ ] Restore from backups if needed
   - [ ] Verify system integrity
   - [ ] Resume normal operations

5. **Post-Incident**
   - [ ] Document incident details
   - [ ] Conduct post-mortem
   - [ ] Update security measures
   - [ ] Notify affected users (if required)

## Resources

### Internal Documentation
- [API Documentation](api/README.md)
- [Database Security](DATABASE_SECURITY.md)
- [Deployment Security](deploy/k8s/SECURITY.md)

### External Resources
- [OWASP Top 10](https://owasp.org/www-project-top-ten/)
- [JWT Best Practices](https://datatracker.ietf.org/doc/html/rfc8725)
- [NIST Password Guidelines](https://pages.nist.gov/800-63-3/)
- [GDPR Compliance Guide](https://gdpr.eu/)

---

**Last Updated**: 2025-11-24
**Version**: 1.0.0
**Security Contact**: security@donelist.io
**Maintained By**: DoneList Security Team
