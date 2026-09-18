# Analytics Package

## Overview

This package provides comprehensive analytics functionality for the DoneList application, including:

- Basic analytics (daily, weekly, monthly summaries)
- Advanced analytics (AI-powered insights, pattern detection)
- Predictive analytics (forecasting, productive day predictions)
- Event tracking and real-time metrics
- Automated report generation

## Package Structure

```
internal/analytics/
├── service.go              # Core analytics service
├── handlers.go             # Basic analytics HTTP handlers
├── advanced_handlers.go    # Premium analytics HTTP handlers
├── insights.go             # AI-powered insights engine
├── predictive.go           # Predictive analytics engine
├── cache.go               # Redis caching layer
├── events.go              # Event tracking system
├── reporting.go           # Report generation
├── *_test.go             # Comprehensive tests
└── README.md              # This file
```

## Quick Start

### 1. Initialize the Service

```go
import (
    "github.com/dev-jelly/donelist/internal/analytics"
    "github.com/redis/go-redis/v9"
    "go.uber.org/zap"
    "gorm.io/gorm"
)

// Initialize analytics service
analyticsService := analytics.NewService(db, redisClient, logger)

// Initialize handlers
basicHandlers := analytics.NewHandlers(analyticsService, logger)
advancedHandlers := analytics.NewAdvancedHandlers(analyticsService, logger)
```

### 2. Register Routes

```go
import (
    "github.com/dev-jelly/donelist/internal/premium"
    "github.com/gorilla/mux"
)

// Basic analytics routes (available to all users)
router := mux.NewRouter()
basicHandlers.RegisterRoutes(router)

// Advanced analytics routes (Premium only)
premiumRouter := router.PathPrefix("/api/v1/analytics").Subrouter()
premiumRouter.Use(premiumMiddleware.RequireFeature(premium.FeatureAdvancedAnalytics))
advancedHandlers.RegisterRoutes(premiumRouter)
```

### 3. Track Events

```go
// Track a user event
event := analytics.NewEventBuilder(analytics.EventCheckinCreated).
    WithUser(userID).
    WithSession(sessionID).
    WithData("checkin_id", checkinID.String()).
    Build()

err := analyticsService.TrackEvent(ctx, event)
```

### 4. Refresh Materialized Views

```go
// Set up periodic refresh (e.g., via cron)
func refreshAnalyticsViews(service *analytics.Service) {
    ctx := context.Background()

    // Full refresh every 6 hours
    if err := service.RefreshMaterializedViews(ctx); err != nil {
        log.Error("Failed to refresh views", zap.Error(err))
    }
}

// Or real-time refresh for specific views
func refreshRealtimeViews(service *analytics.Service) {
    ctx := context.Background()

    // Refresh time-sensitive views every 5 minutes
    if err := service.RefreshRealtimeViews(ctx); err != nil {
        log.Error("Failed to refresh real-time views", zap.Error(err))
    }
}
```

## API Usage Examples

### Basic Analytics

```bash
# Get weekly activity
curl -H "Authorization: Bearer TOKEN" \
  "http://localhost:8080/api/v1/analytics/weekly?week_start=2025-11-18"

# Get streak metrics
curl -H "Authorization: Bearer TOKEN" \
  "http://localhost:8080/api/v1/analytics/streaks"

# Get category performance
curl -H "Authorization: Bearer TOKEN" \
  "http://localhost:8080/api/v1/analytics/categories?limit=10"
```

### Advanced Analytics (Premium)

```bash
# Get AI insights
curl -H "Authorization: Bearer TOKEN" \
  "http://localhost:8080/api/v1/analytics/insights"

# Detect patterns
curl -H "Authorization: Bearer TOKEN" \
  "http://localhost:8080/api/v1/analytics/patterns"

# Get recommendations
curl -H "Authorization: Bearer TOKEN" \
  "http://localhost:8080/api/v1/analytics/recommendations"

# Get focus score
curl -H "Authorization: Bearer TOKEN" \
  "http://localhost:8080/api/v1/analytics/focus-score"

# Get productivity forecast
curl -H "Authorization: Bearer TOKEN" \
  "http://localhost:8080/api/v1/analytics/forecast"

# Predict productive days
curl -H "Authorization: Bearer TOKEN" \
  "http://localhost:8080/api/v1/analytics/predict-days?days=7"

# Generate report
curl -X POST \
  -H "Authorization: Bearer TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"report_type": "comprehensive", "format": "json"}' \
  "http://localhost:8080/api/v1/analytics/reports/generate"
```

## Components

### Analytics Service

Main service coordinating all analytics operations:

```go
type Service struct {
    db           *gorm.DB
    sqlDB        *sql.DB
    cache        *AnalyticsCache
    eventService *EventService
    logger       *zap.Logger
}
```

**Key Methods:**
- `GetDailyActivity(ctx, userID, date)`: Daily activity summary
- `GetWeeklyActivity(ctx, userID, weekStart)`: Weekly report
- `GetMonthlyComparison(ctx, userID, month)`: Month comparison
- `GetCategoryPerformance(ctx, userID, limit)`: Category metrics
- `GetStreakMetrics(ctx, userID)`: Streak information
- `RefreshMaterializedViews(ctx)`: Refresh all views
- `TrackEvent(ctx, event)`: Track analytics event

### Insights Engine

Generates AI-powered insights:

```go
type InsightsEngine struct {
    service *Service
    logger  *zap.Logger
}
```

**Key Methods:**
- `GenerateInsights(ctx, userID)`: Generate all insights
- `DetectPatterns(ctx, userID)`: Detect behavioral patterns
- `GenerateRecommendations(ctx, userID)`: Create recommendations
- `CalculateFocusScore(ctx, userID)`: Calculate focus score

### Predictive Engine

Provides forecasting and predictions:

```go
type PredictiveEngine struct {
    service *Service
    logger  *zap.Logger
}
```

**Key Methods:**
- `ForecastProductivity(ctx, userID)`: Forecast next day/week/month
- `PredictProductiveDays(ctx, userID, days)`: Predict productive days

### Analytics Cache

Redis-based caching layer:

```go
type AnalyticsCache struct {
    redis  *redis.Client
    logger *zap.Logger
    prefix string
}
```

**Cache TTLs:**
- Real-time: 1 minute
- Hourly: 5 minutes
- Daily: 30 minutes
- Weekly: 2 hours
- Monthly: 6 hours
- Insights/Patterns: 1 hour
- Materialized views: 1 hour

## Data Models

### Insight

```go
type Insight struct {
    Type        string                 `json:"type"`
    Title       string                 `json:"title"`
    Description string                 `json:"description"`
    Severity    string                 `json:"severity"` // "info", "warning", "success", "critical"
    Score       float64                `json:"score"`    // 0-100
    Data        map[string]interface{} `json:"data,omitempty"`
    ActionItems []string               `json:"action_items,omitempty"`
    GeneratedAt time.Time              `json:"generated_at"`
}
```

### Pattern

```go
type Pattern struct {
    PatternType     string                 `json:"pattern_type"`
    Name            string                 `json:"name"`
    Description     string                 `json:"description"`
    Confidence      float64                `json:"confidence"` // 0-1
    Frequency       string                 `json:"frequency"`  // "daily", "weekly", "occasional"
    Data            map[string]interface{} `json:"data,omitempty"`
    FirstSeen       time.Time              `json:"first_seen"`
    LastSeen        time.Time              `json:"last_seen"`
    OccurrenceCount int                    `json:"occurrence_count"`
}
```

### Forecast

```go
type Forecast struct {
    Type        string                 `json:"type"`
    Period      string                 `json:"period"` // "day", "week", "month"
    Prediction  float64                `json:"prediction"`
    Confidence  float64                `json:"confidence"` // 0-1
    Range       ForecastRange          `json:"range"`
    BasedOn     string                 `json:"based_on"`
    Data        map[string]interface{} `json:"data,omitempty"`
    GeneratedAt time.Time              `json:"generated_at"`
}
```

### Recommendation

```go
type Recommendation struct {
    Type        string    `json:"type"`
    Priority    string    `json:"priority"` // "high", "medium", "low"
    Title       string    `json:"title"`
    Description string    `json:"description"`
    Impact      string    `json:"impact"`
    Effort      string    `json:"effort"` // "low", "medium", "high"
    Benefits    []string  `json:"benefits"`
    GeneratedAt time.Time `json:"generated_at"`
}
```

## Testing

### Running Tests

```bash
# All tests
go test ./internal/analytics/...

# Unit tests only
go test -short ./internal/analytics/...

# Integration tests
go test -v ./internal/analytics/... -run Integration

# With coverage
go test -cover ./internal/analytics/...
go test -coverprofile=coverage.out ./internal/analytics/...
go tool cover -html=coverage.out
```

### Test Categories

1. **Unit Tests**: Fast, no external dependencies
2. **Integration Tests**: Require Docker (PostgreSQL + Redis)
3. **Load Tests**: Performance testing with ab/wrk

### Test Data Setup

```go
func setupTestService(t *testing.T) (*Service, func()) {
    // Setup test containers
    service, cleanup := setupTestAnalyticsService(t)

    // Create test data
    userID := createTestUser(t, service)
    createTestCheckins(t, service, userID, 30)

    return service, cleanup
}
```

## Performance

### Expected Response Times

- Basic analytics: 10-50ms (cached)
- Insights generation: 50-100ms (cached)
- Pattern detection: 100-200ms (cached)
- Forecasting: 200-300ms (cached)
- Report generation: 500-1000ms (uncached)

### Optimization Tips

1. **Use caching**: Most endpoints use Redis caching
2. **Refresh views**: Keep materialized views fresh
3. **Limit results**: Use `limit` parameter on list endpoints
4. **Batch operations**: Process multiple users in background jobs
5. **Monitor cache**: Track hit rates and memory usage

## Common Issues

### "Insufficient data for forecasting"

**Cause**: User has < 7 days of activity data

**Solution**:
- Show basic analytics instead
- Encourage user to create more check-ins
- Display message explaining minimum data requirements

### "Premium feature required"

**Cause**: User trying to access advanced analytics without Premium tier

**Solution**:
- Check user subscription status
- Display upgrade prompt
- Provide preview of Premium features

### Slow query performance

**Cause**: Materialized views not refreshed or missing indexes

**Solution**:
```sql
-- Check last refresh
SELECT * FROM mv_refresh_log ORDER BY refresh_started_at DESC LIMIT 10;

-- Manual refresh
SELECT refresh_analytics_materialized_views();

-- Check for missing indexes
\d+ mv_daily_user_activity
```

### Cache memory issues

**Cause**: Too many cached items or TTLs too long

**Solution**:
```go
// Monitor cache stats
stats, err := service.GetCacheStats(ctx)

// Clear cache if needed
err := service.cache.Flush(ctx)

// Adjust TTLs in cache.go
```

## Migration Guide

### From Basic to Advanced Analytics

1. **Update routes**:
```go
// Before
basicHandlers.RegisterRoutes(router)

// After
basicHandlers.RegisterRoutes(router)
premiumRouter := router.PathPrefix("/api/v1/analytics").Subrouter()
premiumRouter.Use(premiumMiddleware.RequireFeature(premium.FeatureAdvancedAnalytics))
advancedHandlers.RegisterRoutes(premiumRouter)
```

2. **Update database**:
```bash
# Run analytics materialized views migration
migrate up 000043_analytics_materialized_views.up.sql
```

3. **Configure caching**:
```go
// Ensure Redis is configured
redisClient := redis.NewClient(&redis.Options{
    Addr: os.Getenv("REDIS_URL"),
})
```

4. **Set up background jobs**:
```go
// Refresh views every 6 hours
cron.Every(6 * time.Hour).Do(func() {
    service.RefreshMaterializedViews(context.Background())
})

// Refresh real-time views every 5 minutes
cron.Every(5 * time.Minute).Do(func() {
    service.RefreshRealtimeViews(context.Background())
})
```

## Contributing

When adding new analytics features:

1. **Add algorithms** to appropriate engine (insights.go or predictive.go)
2. **Add handlers** to advanced_handlers.go
3. **Add tests** with >80% coverage
4. **Update documentation** in ADVANCED_ANALYTICS.md
5. **Add caching** for expensive operations
6. **Consider materialized views** for complex queries

## Related Documentation

- [Advanced Analytics Documentation](../../docs/ADVANCED_ANALYTICS.md)
- [API Documentation](../../docs/api/)
- [Database Schema](../../docs/database-schema.md)
- [Premium Features](../premium/README.md)
