# Rate Limiting Integration Guide

This guide shows how to integrate the rate limiting service into the Donelist API server.

## Step 1: Update main.go

Add rate limiting service initialization after Redis client setup:

```go
// In cmd/api/main.go

import (
    // ... existing imports
    "github.com/dev-jelly/donelist/internal/ratelimit"
    "github.com/ulule/limiter/v3"
)

func main() {
    // ... existing setup code ...

    // Initialize Redis connection (already exists)
    redisClient, err := database.NewRedis(redisConfig, log)
    if err != nil {
        log.Fatal("Failed to connect to Redis", zap.Error(err))
    }
    defer database.CloseRedis(redisClient, log)

    // Initialize rate limiting service
    rateLimitConfig := ratelimit.CreateDefaultConfig(redisClient)

    // Optional: Customize configuration
    rateLimitConfig.Endpoints = append(rateLimitConfig.Endpoints,
        ratelimit.EndpointConfig{
            Path: "/api/v1/search",
            Rate: limiter.Rate{
                Period: 1 * time.Minute,
                Limit:  30,
            },
            Tiers: &ratelimit.TierLimits{
                Free: limiter.Rate{
                    Period: 1 * time.Minute,
                    Limit:  10,
                },
                Premium: limiter.Rate{
                    Period: 1 * time.Minute,
                    Limit:  50,
                },
                Enterprise: limiter.Rate{
                    Period: 1 * time.Minute,
                    Limit:  200,
                },
            },
            Burst: &ratelimit.BurstConfig{
                AllowBurst:  true,
                BurstSize:   5,
                BurstPeriod: 10 * time.Second,
            },
        },
    )

    rateLimitService, err := ratelimit.NewService(rateLimitConfig)
    if err != nil {
        log.Fatal("Failed to initialize rate limiting", zap.Error(err))
    }

    // ... rest of the setup ...

    // Pass rate limit service to routes
    routes.SetupRoutes(
        router,
        authHandler,
        userHandler,
        checkinHandler,
        timelineHandler,
        categoryHandler,
        tagHandler,
        wsHandler,
        calendarHandler,
        statisticsHandler,
        syncHandler,
        searchHandler,
        jwtManager,
        rateLimitService,  // Add this parameter
        userService,       // Add this parameter
        log,
    )
}
```

## Step 2: Update routes.go

Modify the routes setup to include rate limiting middleware:

```go
// In internal/api/routes/routes.go

import (
    // ... existing imports
    "github.com/dev-jelly/donelist/internal/ratelimit"
)

func SetupRoutes(
    router *gin.Engine,
    authHandler *handlers.AuthHandler,
    userHandler *handlers.UserHandler,
    checkinHandler *handlers.CheckinHandler,
    timelineHandler *handlers.TimelineHandler,
    categoryHandler *handlers.CategoryHandler,
    tagHandler *handlers.TagHandler,
    wsHandler *handlers.WebSocketHandler,
    calendarHandler *handlers.CalendarHandler,
    statisticsHandler *handlers.StatisticsHandler,
    syncHandler *handlers.SyncHandler,
    searchHandler *handlers.SearchHandler,
    jwtManager *auth.JWTManager,
    rateLimitService *ratelimit.Service,  // Add this
    userService *user.Service,             // Add this
    logger *zap.Logger,
) {
    // Create auth middleware
    authMiddleware := middleware.AuthMiddleware(jwtManager, logger)

    // Create rate limit middleware
    rateLimitMiddleware := middleware.RateLimitMiddleware(
        middleware.RateLimitConfig{
            RateLimitService: rateLimitService,
            UserService:      userService,
            SkipPaths: []string{
                "/health",
                "/health/detail",
                "/ready",
                "/live",
                "/metrics",
            },
        },
    )

    // API v1 routes
    v1 := router.Group("/api/v1")
    v1.Use(rateLimitMiddleware)  // Apply rate limiting to all API routes
    {
        // Public auth routes - strict rate limiting is applied
        authGroup := v1.Group("/auth")
        {
            authGroup.POST("/register", authHandler.Register)
            authGroup.POST("/login", authHandler.Login)
            authGroup.POST("/refresh", authHandler.Refresh)
            authGroup.POST("/logout", authHandler.Logout)
        }

        // Protected routes
        authProtected := v1.Group("/auth")
        authProtected.Use(authMiddleware)
        {
            authProtected.POST("/logout-all", authHandler.LogoutAll)
        }

        // ... rest of the routes remain the same ...
    }

    // WebSocket route (protected by JWT auth middleware)
    // Apply rate limiting to WebSocket upgrade requests
    router.GET("/ws", rateLimitMiddleware, authMiddleware, wsHandler.HandleWebSocket)
}
```

## Step 3: Environment Configuration

Add rate limiting configuration to `.env`:

```bash
# Rate Limiting Configuration
RATE_LIMIT_ENABLED=true
RATE_LIMIT_REQUESTS_PER_MINUTE=60

# Redis Configuration (required for rate limiting)
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0
```

## Step 4: Docker Compose Integration

Ensure Redis is available in your docker-compose.yml:

```yaml
services:
  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data
    command: redis-server --appendonly yes
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 3s
      retries: 3

  api:
    depends_on:
      - postgres
      - redis  # Add Redis dependency
    environment:
      - REDIS_HOST=redis
      - REDIS_PORT=6379

volumes:
  redis_data:
```

## Step 5: Testing the Integration

### Manual Testing

```bash
# Start the server
docker-compose up -d redis
go run cmd/api/main.go

# Test rate limiting
for i in {1..15}; do
  curl -i http://localhost:8080/api/v1/auth/login \
    -H "Content-Type: application/json" \
    -d '{"email":"test@example.com","password":"password"}'
  echo "Request $i"
  sleep 1
done

# Check response headers
curl -i http://localhost:8080/api/v1/checkins \
  -H "Authorization: Bearer $TOKEN" | grep X-RateLimit
```

### Expected Headers

```
HTTP/1.1 200 OK
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 99
X-RateLimit-Reset: 1699564800
X-RateLimit-Burst-Limit: 5
X-RateLimit-Burst-Remaining: 4
```

### Expected 429 Response

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

## Step 6: Monitoring

### Add Prometheus Metrics

Create metrics for rate limiting:

```go
// In internal/metrics/metrics.go

var (
    RateLimitHits = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "rate_limit_hits_total",
            Help: "Total number of rate limit hits",
        },
        []string{"endpoint", "tier"},
    )

    BurstLimitHits = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "burst_limit_hits_total",
            Help: "Total number of burst limit hits",
        },
        []string{"endpoint"},
    )
)
```

### Update Middleware to Track Metrics

```go
// In internal/middleware/ratelimit.go
if !allowed {
    metrics.RateLimitHits.WithLabelValues(endpoint, tier).Inc()
    // ... existing code
}

if !burstAllowed {
    metrics.BurstLimitHits.WithLabelValues(endpoint).Inc()
    // ... existing code
}
```

## Step 7: Admin Endpoints

Add admin endpoints for managing rate limits:

```go
// In routes.go
admin := v1.Group("/admin")
admin.Use(authMiddleware, adminOnlyMiddleware)
{
    admin.GET("/ratelimit/:user_id", middleware.GetRateLimitInfo(rateLimitService))
    admin.POST("/ratelimit/:user_id/reset", middleware.ResetRateLimitHandler(rateLimitService))
}
```

## Migration Checklist

- [ ] Redis is running and accessible
- [ ] Environment variables are configured
- [ ] Rate limiting service is initialized in main.go
- [ ] Routes are updated with rate limiting middleware
- [ ] Health check endpoints are excluded from rate limiting
- [ ] Tests pass with rate limiting enabled
- [ ] Monitoring/metrics are configured
- [ ] Documentation is updated

## Rollback Plan

If issues occur after deployment:

1. **Disable rate limiting**: Set `RATE_LIMIT_ENABLED=false`
2. **Increase limits temporarily**: Adjust tier limits in config
3. **Check Redis health**: Ensure Redis is responding
4. **Review logs**: Check for rate limit errors

## Performance Impact

Expected performance characteristics:

- **Latency overhead**: ~2-5ms per request (Redis RTT)
- **Redis load**: ~2 operations per request (rate check + burst check)
- **Memory usage**: Minimal (~100 bytes per user per endpoint)
- **Network**: ~1KB additional response headers

## Security Considerations

1. **Fail-open strategy**: Requests allowed if Redis is down
2. **IP spoofing protection**: Uses validated client IP
3. **Bypass mechanism**: Secure tokens for internal services
4. **Audit logging**: All rate limit violations are logged

## Common Issues

### Issue: All requests getting 429

**Solution**: Check if default limits are too strict
```go
// Increase default limits temporarily
rateLimitConfig.DefaultRate.Limit = 1000
```

### Issue: Burst limits too aggressive

**Solution**: Adjust burst configuration
```go
Burst: &BurstConfig{
    BurstSize:   10,  // Increase from 3
    BurstPeriod: 60 * time.Second,  // Increase window
}
```

### Issue: Redis connection failures

**Solution**: Implement connection retry logic
```bash
# Check Redis health
redis-cli ping
docker logs donelist_redis

# Verify configuration
echo $REDIS_HOST
echo $REDIS_PORT
```
