# Premium Features Gateway

This package implements a comprehensive premium features gateway with subscription management, tier-based access control, and usage limiting.

## Features

### Subscription Tiers
- **Free**: Basic features with limits
  - 50 check-ins per day
  - 5 categories maximum
  - 2-hour edit window
  - 30-day data retention

- **Premium** ($9.99/month): Power user features
  - Unlimited check-ins
  - Unlimited categories
  - Unlimited edit history
  - Advanced analytics
  - API access
  - Webhooks
  - Priority support

- **Enterprise** ($29.99/month): Team features
  - All Premium features
  - Team collaboration
  - SSO authentication
  - Audit logs
  - Custom integrations
  - Dedicated support

### Core Components

#### 1. Service (`service.go`)
- Tier management with grace period support (7 days after expiration)
- Usage limit checking and tracking
- Subscription upgrade/downgrade/extension
- Event auditing for compliance

#### 2. Middleware (`middleware.go`)
- `RequireTier`: Enforces minimum tier requirements
- `RequireFeature`: Checks specific feature access
- `CheckLimit`: Validates usage limits with rate limiting headers
- `InjectTierInfo`: Adds tier information to response headers

#### 3. Handlers (`handlers.go`)
REST API endpoints for subscription management:
- `GET /api/v1/subscription/current` - Get current subscription
- `GET /api/v1/subscription/plans` - List available plans
- `POST /api/v1/subscription/upgrade` - Upgrade subscription
- `POST /api/v1/subscription/downgrade` - Downgrade subscription
- `POST /api/v1/subscription/cancel` - Cancel subscription
- `GET /api/v1/subscription/usage` - Check usage status
- `GET /api/v1/subscription/features` - List available features
- `GET /api/v1/subscription/events` - Get audit log

#### 4. Features (`features.go`)
- Feature flag definitions
- Tier comparison utilities
- Limit management
- Edit permission checks

## Usage

### Basic Tier Check
```go
// In your route setup
router.GET("/analytics",
    authMiddleware.RequireAuth(),
    premiumMiddleware.RequireTier(premium.TierPremium),
    handlers.GetAnalytics,
)
```

### Feature-Based Access
```go
// Check specific feature
router.POST("/webhooks",
    authMiddleware.RequireAuth(),
    premiumMiddleware.RequireFeature(premium.FeatureWebhooks),
    handlers.CreateWebhook,
)
```

### Usage Limiting
```go
// Rate limit API calls
router.GET("/api/data",
    authMiddleware.RequireAuth(),
    premiumMiddleware.CheckLimit("api_calls_per_hour"),
    handlers.GetData,
)
```

### Check Edit Permission
```go
// In your handler
canEdit, reason := premium.CheckEditPermission(userTier, checkinTime)
if !canEdit {
    return errors.New(reason)
}
```

## Grace Period

When a subscription expires, users enter a 7-day grace period where:
- They retain access to premium features
- Warning headers are added to responses
- Users are encouraged to renew
- After grace period, features are restricted

## Response Headers

The middleware adds informative headers:
- `X-User-Tier`: Current subscription tier
- `X-Tier-Expires`: Subscription expiration date
- `X-Grace-Period`: Whether in grace period
- `X-Grace-Ends`: Grace period end date
- `X-Available-Features`: Comma-separated feature list
- `X-RateLimit-Limit`: Usage limit
- `X-RateLimit-Remaining`: Remaining usage
- `X-RateLimit-Reset`: When limit resets

## Database Schema

Required tables:
- `users`: Must have `tier` and `tier_expires_at` columns
- `subscriptions`: Subscription records
- `subscription_events`: Audit log
- `export_logs`: Track export usage
- `api_logs`: Track API usage

## Testing

Comprehensive test coverage includes:
- `service_test.go`: Tier management, grace periods, usage limits
- `features_test.go`: Feature checks, tier comparisons
- `middleware_test.go`: All middleware functions

Run tests:
```bash
go test ./internal/premium/...
```

## Integration

1. Initialize the service:
```go
premiumService := premium.NewService(db, userRepo, subRepo, logger)
premiumMiddleware := premium.NewMiddleware(premiumService, logger)
premiumHandlers := premium.NewHandler(premiumService, logger)
```

2. Add routes:
```go
subscription := router.Group("/api/v1/subscription")
subscription.Use(authMiddleware.RequireAuth())
{
    subscription.GET("/current", premiumHandlers.GetCurrentSubscription)
    subscription.GET("/plans", premiumHandlers.GetAvailablePlans)
    subscription.POST("/upgrade", premiumHandlers.UpgradeSubscription)
    // ... more routes
}
```

3. Protect premium features:
```go
premium := router.Group("/api/v1/premium")
premium.Use(authMiddleware.RequireAuth())
premium.Use(premiumMiddleware.RequireTier(premium.TierPremium))
{
    // Premium-only routes
}
```

## Security Considerations

- All tier checks require authentication
- Grace period access is logged for auditing
- Usage limits prevent abuse
- Subscription events are tracked for compliance
- Proper error messages avoid information leakage