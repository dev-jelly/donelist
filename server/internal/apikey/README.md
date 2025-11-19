# API Key Management

This package provides a comprehensive API key management system for third-party integrations.

## Features

### Key Generation
- **Format**: `dl_<random>_<checksum>`
- Cryptographically secure random generation
- Built-in checksum validation to detect tampering
- Display prefix for safe logging (e.g., `dl_AbCdEfGh...`)

### Security
- Keys are hashed with SHA256 before storage
- Plain text keys are never stored in the database
- Keys are only shown once upon creation
- Support for key rotation and revocation

### Scope-Based Permissions
Fine-grained permission control with the following scopes:

**Read Scopes:**
- `checkins:read` - View check-ins
- `timeline:read` - Access timeline data
- `statistics:read` - View statistics
- `categories:read` - View categories
- `tags:read` - View tags
- `search:read` - Use search functionality
- `calendar:read` - Access calendar data

**Write Scopes:**
- `checkins:write` - Create/modify check-ins
- `categories:write` - Manage categories
- `tags:write` - Manage tags

**Admin Scopes:**
- `api_keys:manage` - Manage API keys
- `user:read` - View user profile
- `user:write` - Update user profile

### Rate Limiting
- Configurable hourly and daily rate limits per key
- Default: 1,000 requests/hour, 10,000 requests/day
- Automatic tracking and enforcement
- Rate limit information in response headers

### Usage Tracking
- Detailed usage logs per API key
- Endpoint and method tracking
- Response time monitoring
- IP address and user agent logging
- Aggregated statistics and analytics

## Database Schema

### api_keys
- Core API key information
- Stores key hash, scopes, and rate limits
- Tracks last usage and expiry

### api_key_usage
- Individual request logs
- Performance metrics
- Client information

### api_key_rate_limits
- Time-windowed rate limit counters
- Automatic cleanup of old data

## API Endpoints

### Management Endpoints (JWT Auth Required)

```
GET    /api/v1/api-keys/scopes                 # List available scopes
POST   /api/v1/api-keys                        # Create API key
GET    /api/v1/api-keys                        # List user's API keys
GET    /api/v1/api-keys/:id                    # Get API key details
PATCH  /api/v1/api-keys/:id                    # Update API key
DELETE /api/v1/api-keys/:id                    # Delete API key
POST   /api/v1/api-keys/:id/revoke             # Revoke API key
POST   /api/v1/api-keys/:id/rotate             # Rotate API key
GET    /api/v1/api-keys/:id/statistics         # Get usage statistics
```

### Protected Endpoints (API Key Auth)

Use the `X-API-Key` header with any protected endpoint:

```bash
curl -H "X-API-Key: dl_AbCdEfGh..." https://api.example.com/api/v1/checkins
```

## Usage Examples

### Creating an API Key

```bash
curl -X POST https://api.example.com/api/v1/api-keys \
  -H "Authorization: Bearer <jwt_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Production Integration",
    "scopes": ["checkins:read", "checkins:write"],
    "rate_limit_per_hour": 500,
    "rate_limit_per_day": 5000,
    "expires_in_days": 365
  }'
```

Response:
```json
{
  "id": "123e4567-e89b-12d3-a456-426614174000",
  "name": "Production Integration",
  "key_prefix": "dl_AbCdEfGh",
  "scopes": ["checkins:read", "checkins:write"],
  "rate_limit_per_hour": 500,
  "rate_limit_per_day": 5000,
  "plain_text_key": "dl_AbCdEfGhIjKlMnOpQrStUvWxYzAa_a1b2c3d4",
  "created_at": "2024-01-01T00:00:00Z"
}
```

**Important**: Save the `plain_text_key` immediately - it will never be shown again!

### Using an API Key

```bash
curl https://api.example.com/api/v1/checkins \
  -H "X-API-Key: dl_AbCdEfGhIjKlMnOpQrStUvWxYzAa_a1b2c3d4"
```

Rate limit headers in response:
```
X-RateLimit-Limit-Hour: 500
X-RateLimit-Remaining-Hour: 499
X-RateLimit-Limit-Day: 5000
X-RateLimit-Remaining-Day: 4999
X-RateLimit-Reset: 2024-01-01T01:00:00Z
```

### Rotating an API Key

```bash
curl -X POST https://api.example.com/api/v1/api-keys/:id/rotate \
  -H "Authorization: Bearer <jwt_token>"
```

Creates a new key with same settings and revokes the old one.

### Viewing Usage Statistics

```bash
curl https://api.example.com/api/v1/api-keys/:id/statistics?days=30 \
  -H "Authorization: Bearer <jwt_token>"
```

Response:
```json
{
  "total_requests": 12450,
  "requests_by_day": {
    "2024-01-01": 450,
    "2024-01-02": 500
  },
  "requests_by_endpoint": {
    "/api/v1/checkins": 10000,
    "/api/v1/categories": 2450
  },
  "average_response_time_ms": 85.5
}
```

## Middleware Usage

### API Key Authentication

```go
import "github.com/dev-jelly/donelist/internal/middleware"

// Apply to routes
router.GET("/api/v1/data",
    middleware.APIKeyMiddleware(apiKeyService, logger),
    handler.GetData)
```

### Scope Requirements

```go
import "github.com/dev-jelly/donelist/internal/apikey"

// Require specific scopes
router.POST("/api/v1/checkins",
    middleware.APIKeyMiddleware(apiKeyService, logger),
    middleware.RequireScope(apikey.ScopeCheckinsWrite),
    handler.CreateCheckin)
```

## Error Handling

### 401 Unauthorized
- Missing API key
- Invalid key format
- Key not found
- Key revoked
- Key expired

### 403 Forbidden
- Insufficient permissions (scope mismatch)

### 429 Too Many Requests
- Rate limit exceeded
- Includes reset time in response

## Best Practices

### Security
1. **Never log full API keys** - Use key prefixes only
2. **Rotate keys regularly** - Especially after team member changes
3. **Use minimal scopes** - Grant only required permissions
4. **Set expiration dates** - For temporary integrations
5. **Monitor usage** - Check for unusual patterns

### Performance
1. **Implement caching** - Cache valid keys in Redis
2. **Batch operations** - Reduce API calls when possible
3. **Handle rate limits** - Implement exponential backoff

### Operations
1. **Name keys descriptively** - "Production Server", not "Key 1"
2. **Document scope requirements** - For each integration
3. **Set appropriate rate limits** - Based on expected usage
4. **Review keys periodically** - Revoke unused keys
5. **Track usage** - Monitor for anomalies

## Testing

Run tests:
```bash
go test ./internal/apikey/...
```

Test coverage includes:
- Key generation and validation
- Scope management
- Rate limiting
- Usage tracking
- Service operations
- Error handling

## Maintenance

### Cleanup Tasks

The database includes cleanup functions:

```sql
-- Clean up old usage records (90+ days)
SELECT cleanup_old_api_key_usage();

-- Clean up old rate limit records (2+ days)
SELECT cleanup_old_rate_limits();
```

Schedule these in a cron job or use a background worker.

### Monitoring

Key metrics to monitor:
- Total active API keys
- Rate limit violations
- Average response times
- Failed authentication attempts
- Keys nearing expiration

## Future Enhancements

Potential improvements:
- [ ] IP whitelisting
- [ ] Webhook signatures
- [ ] Key encryption at rest
- [ ] Advanced analytics dashboard
- [ ] Automatic key rotation
- [ ] Anomaly detection
- [ ] Multi-region support
- [ ] Custom rate limit windows
