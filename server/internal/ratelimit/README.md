# Rate Limiting Implementation

This package provides comprehensive rate limiting with user-based, IP-based, and burst control mechanisms.

## Features

- **User-Based Rate Limiting**: Different limits for authenticated users based on tier (Free, Premium, Enterprise)
- **IP-Based Rate Limiting**: Rate limits for anonymous users based on IP address
- **Endpoint-Specific Limits**: Custom rate limits for specific endpoints
- **Burst Control**: Prevent rapid request bursts within short time windows
- **Redis-Backed Storage**: Distributed rate limiting using Redis
- **Sliding Window Algorithm**: Fair rate limiting using the ulule/limiter library
- **Standard Headers**: X-RateLimit-* headers following RFC 6585

## Architecture

### Components

1. **Service** (`service.go`): Core rate limiting logic
2. **Middleware** (`middleware/ratelimit.go`): HTTP middleware integration
3. **Config**: Flexible configuration for tiers and endpoints

### Rate Limiting Strategies

#### 1. Sliding Window
- Uses Redis-backed sliding window algorithm
- Provides fair distribution of requests over time
- Implemented via `ulule/limiter` library

#### 2. Burst Control
- Independent burst detection on top of sliding window
- Prevents rapid successive requests
- Configurable per endpoint

## Configuration

### Default Configuration

```go
cfg := ratelimit.CreateDefaultConfig(redisClient)
service, err := ratelimit.NewService(cfg)
```

### Custom Configuration

```go
cfg := ratelimit.Config{
    RedisClient: redisClient,
    DefaultRate: limiter.Rate{
        Period: 1 * time.Hour,
        Limit:  100,
    },
    TierLimits: ratelimit.TierLimits{
        Free: limiter.Rate{
            Period: 1 * time.Hour,
            Limit:  100,
        },
        Premium: limiter.Rate{
            Period: 1 * time.Hour,
            Limit:  1000,
        },
        Enterprise: limiter.Rate{
            Period: 1 * time.Hour,
            Limit:  10000,
        },
    },
    Endpoints: []ratelimit.EndpointConfig{
        {
            Path: "/api/v1/auth/login",
            Rate: limiter.Rate{
                Period: 5 * time.Minute,
                Limit:  10,
            },
            Burst: &ratelimit.BurstConfig{
                AllowBurst:  true,
                BurstSize:   3,  // Max 3 requests
                BurstPeriod: 30 * time.Second,  // in 30 seconds
            },
        },
    },
}
```

## Usage

### Middleware Setup

```go
// In main.go or router setup
rateLimitService, err := ratelimit.NewService(cfg)
if err != nil {
    log.Fatal(err)
}

rateLimitConfig := middleware.RateLimitConfig{
    RateLimitService: rateLimitService,
    UserService:      userService,
    SkipPaths: []string{
        "/health",
        "/metrics",
    },
}

router.Use(middleware.RateLimitMiddleware(rateLimitConfig))
```

### Checking Limits Programmatically

```go
// For authenticated users
allowed, remaining, resetTime, err := service.CheckLimit(
    ctx,
    userID,
    endpoint,
    tier,
)

// For IP addresses
allowed, remaining, resetTime, err := service.CheckLimitForIP(
    ctx,
    ipAddress,
    endpoint,
)

// Check burst
burstAllowed, err := service.CheckBurst(ctx, userID, endpoint)
```

## Response Headers

When rate limiting is active, the following headers are included:

### Standard Rate Limit Headers

- `X-RateLimit-Limit`: Maximum requests allowed in the period
- `X-RateLimit-Remaining`: Requests remaining in current period
- `X-RateLimit-Reset`: Unix timestamp when the rate limit resets

### Burst Control Headers

- `X-RateLimit-Burst-Limit`: Maximum burst size allowed
- `X-RateLimit-Burst-Remaining`: Burst tokens remaining
- `X-RateLimit-Burst-Exceeded`: Set to "true" when burst limit is hit

### Retry Headers

- `Retry-After`: Seconds to wait before retrying (when limit exceeded)

## Error Responses

### Rate Limit Exceeded (429)

```json
{
  "error": "Rate limit exceeded",
  "message": "Too many requests. Please implement exponential backoff.",
  "details": {
    "retry_after_seconds": 60,
    "reset_time": 1699564800,
    "reset_time_iso": "2023-11-10T00:00:00Z",
    "suggested_backoff": {
      "next_retry_seconds": 2,
      "strategy": "exponential",
      "guidance": "Implement exponential backoff: wait 1s, then 2s, then 4s, etc."
    }
  }
}
```

### Burst Limit Exceeded (429)

```json
{
  "error": "Burst limit exceeded",
  "message": "Too many requests in a short period. Please slow down."
}
```

## Client Implementation Guide

### Handling Rate Limits

```javascript
// Example client-side implementation
async function apiCall(url, options) {
    const response = await fetch(url, options);

    // Check rate limit headers
    const remaining = response.headers.get('X-RateLimit-Remaining');
    const reset = response.headers.get('X-RateLimit-Reset');

    if (response.status === 429) {
        const data = await response.json();
        const retryAfter = data.details.retry_after_seconds;

        // Implement exponential backoff
        await sleep(retryAfter * 1000);
        return apiCall(url, options); // Retry
    }

    // Proactive rate limit handling
    if (remaining && parseInt(remaining) < 10) {
        console.warn(`Rate limit low: ${remaining} requests remaining`);
    }

    return response;
}
```

## Tier Limits

### Default Tier Configuration

| Tier       | Global Limit    | Login Limit        | Checkin Limit      |
|------------|-----------------|--------------------|--------------------|
| Free       | 100/hour        | 10/5min            | 30/minute          |
| Premium    | 1,000/hour      | 10/5min            | 100/minute         |
| Enterprise | 10,000/hour     | 10/5min            | 1,000/minute       |
| Anonymous  | 50/hour         | 10/5min            | N/A                |

### Endpoint-Specific Limits

- **Login**: 10 requests per 5 minutes (with 3 burst in 30s)
- **Register**: 5 requests per hour (with 2 burst in 1min)
- **Password Reset**: 3 requests per hour (with 1 burst in 1min)
- **Checkins**: Tier-based per minute limits

## Redis Key Schema

### Rate Limit Keys
```
rate:user:{userID}:global
rate:user:{userID}:{endpoint}
ip:{ipAddress}:{endpoint}
```

### Burst Control Keys
```
burst:{userID}:{endpoint}
```

## Testing

### Unit Tests

```bash
# Run all rate limit tests
go test ./internal/ratelimit/...

# Run specific tests
go test ./internal/ratelimit -run TestService_BurstControl
go test ./internal/ratelimit -run TestService_CheckLimit

# Run with Redis
docker run -d -p 6379:6379 redis:alpine
go test ./internal/ratelimit/... -v
```

### Load Testing

```bash
# Using Apache Bench
ab -n 1000 -c 10 -H "Authorization: Bearer $TOKEN" \
   http://localhost:8080/api/v1/checkins

# Using hey
hey -n 1000 -c 10 -H "Authorization: Bearer $TOKEN" \
    http://localhost:8080/api/v1/checkins
```

## Monitoring

### Metrics to Track

1. **Rate Limit Hits**: Number of 429 responses
2. **Burst Limit Hits**: Burst control activations
3. **Per-Tier Usage**: Request distribution by user tier
4. **Per-Endpoint Usage**: Hotspot identification

### Audit Logging

Rate limit events are logged to the audit system:

```go
auditService.LogRateLimitExceeded(
    ctx,
    userID,
    ipAddress,
    userAgent,
    endpoint,
)
```

## Performance Considerations

### Redis Performance

- Connection pooling enabled
- Pipeline operations for burst checks
- Minimal network round trips

### Fail-Open Strategy

If Redis is unavailable, the middleware:
- Logs the error
- Allows the request through (fail-open)
- Prevents cascading failures

### Scalability

- Redis supports horizontal scaling via clustering
- Stateless middleware design
- No local state, safe for multi-instance deployments

## Security Considerations

### IP Spoofing Protection

The middleware uses `c.ClientIP()` which:
- Checks X-Forwarded-For headers
- Validates proxy headers
- Falls back to direct connection IP

### Bypass Protection

Internal services can bypass rate limits using:
```go
cfg := RateLimitConfig{
    BypassHeader: "X-Internal-Service",
    BypassToken:  "secure-token-here",
}
```

### Brute Force Protection

Login endpoints have strict limits:
- 10 attempts per 5 minutes
- 3 burst requests in 30 seconds
- Automatic lockout after repeated violations

## Future Enhancements

1. **Adaptive Rate Limiting**: Adjust limits based on system load
2. **User Reputation Scoring**: Trust-based limit adjustment
3. **Geographic Rate Limiting**: Different limits per region
4. **Cost-Based Limiting**: Different costs for different operations
5. **Prometheus Metrics**: Detailed rate limit metrics export

## References

- [RFC 6585 - HTTP Status Code 429](https://tools.ietf.org/html/rfc6585)
- [ulule/limiter Documentation](https://github.com/ulule/limiter)
- [Redis Rate Limiting Patterns](https://redis.io/docs/manual/patterns/rate-limiter/)
