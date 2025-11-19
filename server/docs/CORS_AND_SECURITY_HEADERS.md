# CORS and Security Headers Implementation

## Overview

This document describes the CORS (Cross-Origin Resource Sharing) and security headers implementation for the Donelist API server. The implementation provides strict origin validation, comprehensive security headers, and environment-specific configurations.

## CORS Implementation

### Features

1. **Strict Origin Validation**
   - Whitelist-based origin validation
   - Wildcard pattern support (e.g., `*.example.com`, `http://localhost:*`)
   - Automatic rejection of unauthorized origins
   - Logging of unauthorized access attempts

2. **Preflight Request Handling**
   - Automatic handling of OPTIONS requests
   - Configurable preflight cache duration
   - Method and header validation

3. **Credentials Support**
   - Configurable credential handling
   - Secure cookie transmission support

4. **Environment-Specific Configurations**
   - Strict configuration for production
   - Permissive configuration for development

### Configuration

CORS is configured via environment variables:

```env
# Allowed origins (comma-separated)
CORS_ALLOWED_ORIGINS=http://localhost:3000,http://localhost:5173,https://app.example.com

# Allowed HTTP methods (comma-separated)
CORS_ALLOWED_METHODS=GET,POST,PUT,DELETE,OPTIONS,PATCH

# Allowed request headers (comma-separated)
CORS_ALLOWED_HEADERS=Accept,Authorization,Content-Type,X-CSRF-Token

# Headers exposed to the browser (comma-separated)
CORS_EXPOSE_HEADERS=Link,X-RateLimit-Limit,X-RateLimit-Remaining

# Allow credentials (cookies, authorization headers)
CORS_ALLOW_CREDENTIALS=true

# Preflight cache duration in seconds
CORS_MAX_AGE=300
```

### Origin Patterns

The CORS middleware supports various origin patterns:

1. **Exact Match**
   ```
   http://localhost:3000
   https://app.example.com
   ```

2. **Subdomain Wildcard**
   ```
   https://*.example.com
   ```
   Matches: `https://app.example.com`, `https://admin.example.com`, etc.

3. **Port Wildcard**
   ```
   http://localhost:*
   ```
   Matches: `http://localhost:3000`, `http://localhost:5173`, etc.

4. **Universal Wildcard** (NOT RECOMMENDED for production)
   ```
   *
   ```
   Matches all origins

### Security Behavior

1. **Unauthorized Origins**
   - Requests from unauthorized origins receive HTTP 403 Forbidden
   - No CORS headers are set for unauthorized origins
   - Attempts are logged with origin, path, and method

2. **Missing Origin Header**
   - Requests without an Origin header are treated as same-origin requests
   - No CORS headers are added
   - Request processing continues normally

3. **Preflight Requests**
   - OPTIONS requests are handled automatically
   - Returns HTTP 204 No Content with appropriate CORS headers
   - Cached by browser based on MaxAge setting

## Security Headers Implementation

### Headers Applied

The security headers middleware applies the following headers:

#### 1. Content Security Policy (CSP)

Defines which resources can be loaded:

```
Content-Security-Policy: default-src 'self'; script-src 'self' 'unsafe-inline'; ...
```

**Development Mode:**
- Report-Only mode (violations logged but not enforced)
- More permissive policies for local development

**Production Mode:**
- Enforcement mode (violations blocked)
- Strict policies:
  - `default-src 'self'`
  - `frame-ancestors 'none'`
  - `object-src 'none'`
  - `upgrade-insecure-requests`

**API-Specific:**
- Minimal CSP: `default-src 'none'; frame-ancestors 'none'`
- No script or style sources needed

#### 2. HTTP Strict Transport Security (HSTS)

Forces HTTPS connections:

```
Strict-Transport-Security: max-age=31536000; includeSubDomains
```

**Configuration:**
- Only enabled in production (requires HTTPS)
- 1-year max-age (31536000 seconds)
- Includes subdomains
- Preload option available (disabled by default)

#### 3. X-Frame-Options

Prevents clickjacking attacks:

```
X-Frame-Options: DENY
```

**Options:**
- `DENY`: Cannot be embedded in any frame
- `SAMEORIGIN`: Can only be embedded by same origin
- `ALLOW-FROM uri`: Deprecated, use CSP instead

#### 4. X-Content-Type-Options

Prevents MIME-type sniffing:

```
X-Content-Type-Options: nosniff
```

Forces browsers to respect declared Content-Type.

#### 5. X-XSS-Protection

Legacy XSS filter (for older browsers):

```
X-XSS-Protection: 1; mode=block
```

**Note:** Modern browsers rely on CSP instead.

#### 6. Referrer-Policy

Controls referrer information:

```
Referrer-Policy: strict-origin-when-cross-origin
```

**Policy Options:**
- `no-referrer`: Never send referrer
- `strict-origin-when-cross-origin`: Send origin for cross-origin (recommended)
- `same-origin`: Only send for same-origin requests

#### 7. Permissions-Policy

Controls browser features:

```
Permissions-Policy: geolocation=(), microphone=(), camera=(), payment=()
```

**Denied Features:**
- Geolocation
- Microphone
- Camera
- Payment API
- USB
- Magnetometer
- Gyroscope
- Accelerometer

**Allowed Features (self only):**
- Autoplay
- Encrypted media
- Fullscreen
- Picture-in-picture

#### 8. Cross-Origin Policies

**Cross-Origin-Embedder-Policy:**
```
Cross-Origin-Embedder-Policy: require-corp
```

**Cross-Origin-Opener-Policy:**
```
Cross-Origin-Opener-Policy: same-origin
```

**Cross-Origin-Resource-Policy:**
```
Cross-Origin-Resource-Policy: same-origin
```

### Environment Configurations

#### Production Configuration

```go
securityHeadersConfig := middleware.CreateAPISecurityHeadersConfig(log, true)
```

- Strict CSP with enforcement
- HSTS enabled (requires HTTPS)
- All security headers enabled
- Server information headers removed

#### Development Configuration

```go
securityHeadersConfig := middleware.CreateDevelopmentSecurityHeadersConfig(log)
```

- CSP in report-only mode
- No HSTS (allows HTTP)
- Relaxed cross-origin policies
- Supports WebSocket connections
- Server information headers retained

## Integration

### Middleware Order

Security middleware is applied in this order:

```go
router.Use(middleware.RecoveryMiddleware(log))       // 1. Panic recovery
router.Use(middleware.CorrelationMiddleware(log))    // 2. Request IDs
router.Use(middleware.CORSMiddleware(cfg.CORS, log)) // 3. CORS
router.Use(middleware.SecurityHeadersMiddleware(...)) // 4. Security headers
router.Use(metrics.MetricsMiddleware(appMetrics))    // 5. Metrics
router.Use(middleware.LoggingMiddleware(log))        // 6. Logging
```

**Order Rationale:**
1. Recovery first to catch all panics
2. Correlation IDs for request tracking
3. CORS to reject unauthorized origins early
4. Security headers for all allowed requests
5. Metrics and logging for monitoring

## Testing

### CORS Testing

Run CORS tests:

```bash
cd /Users/jelly/personal/donelist/server
go test ./internal/middleware -run TestCORS -v
```

Test scenarios:
- Allowed origins (exact match, wildcard)
- Disallowed origins
- Preflight requests
- Credentials handling
- Missing origin headers

### Security Headers Testing

Run security headers tests:

```bash
cd /Users/jelly/personal/donelist/server
go test ./internal/middleware -run TestSecurityHeaders -v
```

### Manual Testing

#### Test CORS with curl:

```bash
# Test allowed origin
curl -X GET http://localhost:8080/api/v1/health \
  -H "Origin: http://localhost:3000" \
  -v

# Test unauthorized origin
curl -X GET http://localhost:8080/api/v1/health \
  -H "Origin: http://evil.com" \
  -v

# Test preflight request
curl -X OPTIONS http://localhost:8080/api/v1/auth/login \
  -H "Origin: http://localhost:3000" \
  -H "Access-Control-Request-Method: POST" \
  -H "Access-Control-Request-Headers: Content-Type,Authorization" \
  -v
```

#### Test security headers:

```bash
# Check security headers
curl -X GET http://localhost:8080/api/v1/health -I

# Expected headers:
# Content-Security-Policy: ...
# X-Frame-Options: DENY
# X-Content-Type-Options: nosniff
# Referrer-Policy: strict-origin-when-cross-origin
# Permissions-Policy: ...
```

## Production Deployment Checklist

### CORS Configuration

- [ ] Set `CORS_ALLOWED_ORIGINS` to actual frontend domains
- [ ] Remove wildcard patterns if not needed
- [ ] Keep `CORS_ALLOW_CREDENTIALS=true` if using cookies
- [ ] Set appropriate `CORS_MAX_AGE` (300-600 seconds recommended)
- [ ] Verify `CORS_ALLOWED_METHODS` includes only needed methods
- [ ] Limit `CORS_ALLOWED_HEADERS` to required headers only

### Security Headers Configuration

- [ ] Set `APP_ENV=production` to enable HSTS
- [ ] Ensure HTTPS is enabled before enabling HSTS
- [ ] Review CSP directives for your specific needs
- [ ] Consider enabling CSP report URI for violation monitoring
- [ ] Test CSP in report-only mode first
- [ ] Remove or restrict development-only CORS origins
- [ ] Verify `X-Frame-Options` is set to DENY or SAMEORIGIN
- [ ] Enable HSTS preload only after thorough testing

### Monitoring

- [ ] Monitor logs for unauthorized CORS access attempts
- [ ] Set up alerts for security header violations
- [ ] Review CSP violation reports regularly
- [ ] Monitor rate limiting in conjunction with CORS

## Best Practices

### CORS

1. **Never use `*` for CORS_ALLOWED_ORIGINS in production**
2. **Always use specific origins with protocol and port**
3. **Use wildcard patterns sparingly and carefully**
4. **Log and monitor unauthorized access attempts**
5. **Keep preflight cache duration reasonable (5-10 minutes)**

### Security Headers

1. **Test CSP in report-only mode first**
2. **Enable HSTS only after confirming HTTPS works**
3. **Don't enable HSTS preload until thoroughly tested**
4. **Review and update CSP directives regularly**
5. **Use strict CSP for APIs (default-src 'none')**
6. **Remove server identification headers in production**

### Development vs Production

1. **Use separate configurations for each environment**
2. **Never use development configs in production**
3. **Test production configs in staging environment**
4. **Document any deviations from default configs**

## Troubleshooting

### CORS Errors

**Error:** `Access to XMLHttpRequest has been blocked by CORS policy`

**Solutions:**
1. Verify origin is in `CORS_ALLOWED_ORIGINS`
2. Check origin matches exactly (including protocol and port)
3. Verify wildcard pattern is correct
4. Check server logs for rejected origin
5. Ensure preflight request succeeds

**Error:** `CORS credentials flag is true, but the Access-Control-Allow-Credentials header is ''`

**Solution:**
- Set `CORS_ALLOW_CREDENTIALS=true` in environment variables

### CSP Violations

**Error:** `Refused to load script because it violates CSP directive`

**Solutions:**
1. Add required source to CSP directives
2. Use CSP report-only mode for testing
3. Review CSP violation reports
4. Consider using nonces or hashes instead of 'unsafe-inline'

### HSTS Issues

**Error:** Can't access HTTP site after enabling HSTS

**Solution:**
- HSTS forces HTTPS for the specified duration
- Wait for max-age to expire or clear browser HSTS cache
- Ensure HTTPS is working before enabling HSTS

## References

- [MDN CORS Documentation](https://developer.mozilla.org/en-US/docs/Web/HTTP/CORS)
- [MDN Content Security Policy](https://developer.mozilla.org/en-US/docs/Web/HTTP/CSP)
- [OWASP Secure Headers Project](https://owasp.org/www-project-secure-headers/)
- [HSTS Preload List](https://hstspreload.org/)
