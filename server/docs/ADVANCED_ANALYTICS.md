# Advanced Analytics System

## Overview

The Advanced Analytics System provides Premium users with AI-powered insights, pattern detection, predictive analytics, and personalized recommendations based on their check-in data.

## Features

### 1. AI-Powered Insights

Automatically generates comprehensive insights about user behavior:

- **Streak Insights**: Track current streaks, approaching milestones, consistency scores
- **Productivity Insights**: Weekly trends, peak hours, most productive days
- **Category Insights**: Top categories, trending categories, neglected categories
- **Time Pattern Insights**: Chronotype detection (early bird vs night owl)

### 2. Pattern Detection

Detects behavioral patterns in user activity:

- **Time Routines**: Consistent check-in times on specific days
- **Category Habits**: Regular usage patterns for specific categories
- **Weekend Patterns**: Weekend warrior vs weekday focused behavior

### 3. Predictive Analytics

Forecasts future productivity:

- **Next Day Forecast**: Predict tomorrow's activity with confidence scores
- **Next Week Forecast**: Trend-based weekly predictions
- **Next Month Forecast**: Long-term activity forecasting
- **Productive Day Predictions**: Identify which upcoming days will be most productive

### 4. Personalized Recommendations

Generates actionable recommendations:

- **Consistency Improvements**: Tips for maintaining streaks
- **Schedule Optimization**: Leverage natural productive times
- **Category Management**: Review neglected or overused categories
- **Engagement Tips**: Build better tracking habits

### 5. Focus Score

Comprehensive focus score (0-100) based on:

- Streak consistency (0-25 points)
- Weekly activity (0-25 points)
- Category diversity (0-20 points)
- Time efficiency (0-30 points)

### 6. Analytics Reports

Generate comprehensive reports:

- **Weekly Reports**: Week-over-week activity comparison
- **Monthly Reports**: Month-over-month trends
- **Insights Reports**: Focus on patterns and recommendations
- **Comprehensive Reports**: All analytics combined

## API Endpoints

### GET /api/v1/analytics/insights

Get AI-powered insights about user behavior.

**Response:**
```json
{
  "insights": [
    {
      "type": "streak_active",
      "title": "7-Day Streak!",
      "description": "You're on a 7-day streak! Keep it going!",
      "severity": "success",
      "score": 85,
      "data": {
        "current_streak": 7,
        "longest_streak": 14
      },
      "action_items": [],
      "generated_at": "2025-11-25T10:00:00Z"
    }
  ],
  "total_count": 8,
  "generated_at": "2025-11-25T10:00:00Z"
}
```

### GET /api/v1/analytics/patterns

Detect behavioral patterns in user activity.

**Response:**
```json
{
  "patterns": [
    {
      "pattern_type": "time_routine",
      "name": "Monday at 9:00 Routine",
      "description": "You consistently check in on Mondays around 9:00",
      "confidence": 0.85,
      "frequency": "weekly",
      "data": {
        "hour": 9,
        "day_of_week": 1
      },
      "occurrence_count": 8,
      "last_seen": "2025-11-25T09:00:00Z"
    }
  ],
  "patterns_by_type": {
    "time_routine": [...],
    "category_habit": [...],
    "weekend_warrior": [...]
  },
  "total_count": 5
}
```

### GET /api/v1/analytics/recommendations

Get personalized recommendations based on behavior.

**Response:**
```json
{
  "recommendations": [
    {
      "type": "consistency",
      "priority": "high",
      "title": "Rebuild Your Streak",
      "description": "Start a new streak today...",
      "impact": "Rebuilding your streak can increase motivation by 40%",
      "effort": "low",
      "benefits": [
        "Increased motivation and accountability",
        "Better habit formation"
      ],
      "generated_at": "2025-11-25T10:00:00Z"
    }
  ],
  "by_priority": {
    "high": [...],
    "medium": [...],
    "low": [...]
  },
  "total_count": 6
}
```

### GET /api/v1/analytics/focus-score

Calculate comprehensive focus score.

**Response:**
```json
{
  "score": 72.5,
  "level": "high",
  "factors": {
    "streak_consistency": 20,
    "weekly_activity": 22,
    "category_diversity": 16,
    "time_efficiency": 14.5
  },
  "recommendations": [
    "Establish consistent time routines"
  ],
  "calculated_at": "2025-11-25T10:00:00Z"
}
```

### GET /api/v1/analytics/forecast

Get productivity forecast for future periods.

**Response:**
```json
{
  "next_day": {
    "type": "daily_checkins",
    "period": "day",
    "prediction": 8.5,
    "confidence": 0.85,
    "range": {
      "low": 6.0,
      "high": 11.0,
      "median": 8.5
    },
    "based_on": "Monday_pattern",
    "generated_at": "2025-11-25T10:00:00Z"
  },
  "next_week": {
    "type": "weekly_checkins",
    "period": "week",
    "prediction": 45.0,
    "confidence": 0.75,
    "range": {...}
  },
  "next_month": {...},
  "trends": [
    {
      "name": "activity_trend",
      "direction": "up",
      "strength": 0.5,
      "description": "Your activity is trending up (25% increase)",
      "start_date": "2025-11-18T00:00:00Z"
    }
  ],
  "insights": [
    "High confidence prediction: expect around 9 check-ins tomorrow",
    "Your productivity is on an upward trend - keep it up!"
  ],
  "created_at": "2025-11-25T10:00:00Z"
}
```

### GET /api/v1/analytics/predict-days

Predict which upcoming days will be most productive.

**Query Parameters:**
- `days` (int, optional): Number of days to predict (1-30, default: 7)

**Response:**
```json
{
  "predictions": [
    {
      "date": "2025-11-26",
      "day_of_week": "Tuesday",
      "expected_activity": 8.5,
      "likelihood": "high",
      "score": 1.42,
      "confidence": 0.85
    }
  ],
  "by_likelihood": {
    "high": [...],
    "medium": [...],
    "low": [...]
  },
  "days_predicted": 7
}
```

### POST /api/v1/analytics/reports/generate

Generate a comprehensive analytics report.

**Request Body:**
```json
{
  "report_type": "comprehensive",
  "format": "json",
  "options": {}
}
```

**Report Types:**
- `weekly`: Week-over-week activity comparison
- `monthly`: Month-over-month trends
- `insights`: Patterns and recommendations
- `comprehensive`: All analytics combined

**Supported Formats:**
- `json`: JSON format (currently only format supported)

**Response:**
```json
{
  "report": {
    "weekly": {...},
    "monthly": {...},
    "insights": {...},
    "forecast": {...}
  },
  "report_type": "comprehensive",
  "format": "json",
  "generated_at": "2025-11-25T10:00:00Z"
}
```

## Premium Gating

All advanced analytics endpoints require Premium or Enterprise tier:

```go
// In main.go or routes setup
premiumAnalytics := r.PathPrefix("/api/v1/analytics").Subrouter()
premiumAnalytics.Use(premiumMiddleware.RequireFeature(premium.FeatureAdvancedAnalytics))
analyticsHandlers.RegisterRoutes(premiumAnalytics)
```

## Caching Strategy

The analytics system uses Redis caching with different TTLs:

- **Real-time metrics**: 1 minute
- **Hourly aggregates**: 5 minutes
- **Daily summaries**: 30 minutes
- **Weekly reports**: 2 hours
- **Monthly reports**: 6 hours
- **Insights/Patterns/Recommendations**: 1 hour
- **Materialized views**: 1 hour

## Algorithm Details

### Insights Generation

1. **Streak Insights**:
   - Active streak status with encouragement
   - Milestone tracking (approaching records)
   - Consistency score calculation

2. **Productivity Insights**:
   - Week-over-week comparison with percentage changes
   - Peak hour identification from activity patterns
   - Most productive day detection

3. **Category Insights**:
   - Top category by usage frequency
   - Neglected categories (>7 days inactive)
   - Trending categories (>50% increase)

4. **Time Pattern Insights**:
   - Early bird detection (5-9 AM activity)
   - Night owl detection (10 PM-2 AM activity)

### Pattern Detection

1. **Time Routines**:
   - Requires 4+ occurrences in specific hour/day combination
   - Confidence based on occurrence frequency (min 0, max 1.0)

2. **Category Habits**:
   - Requires 6+ weeks of consistent usage
   - Calculates average weekly usage
   - Confidence based on consistency (weeks_used / 12)

3. **Weekend Patterns**:
   - Compares weekend vs weekday averages
   - Ratio > 1.3: Weekend warrior
   - Ratio < 0.7: Weekday focused

### Predictive Forecasting

1. **Next Day Prediction**:
   - Uses same-day-of-week historical data
   - Weighted average (recent data weighted higher)
   - Confidence based on variance (lower stddev = higher confidence)

2. **Next Week Prediction**:
   - Uses last 4 weeks of data
   - Simple linear regression for trend
   - Accounts for upward/downward trends

3. **Next Month Prediction**:
   - Uses 3-month historical data
   - Trend-adjusted weighted average
   - Fallback to daily average if insufficient monthly data

### Focus Score Calculation

**Formula:**
```
Focus Score = Streak (25) + Weekly Activity (25) + Diversity (20) + Efficiency (30)
```

**Components:**
- Streak: current_streak * 2.5 (max 25)
- Weekly Activity: (active_days / 7) * 25 (max 25)
- Diversity: unique_categories * 4 (max 20)
- Efficiency: consistent_patterns * 6 (max 30)

**Levels:**
- 0-39: Low
- 40-59: Medium
- 60-79: High
- 80-100: Excellent

## Performance Considerations

### Materialized Views

The system relies on materialized views for performance:

- `mv_daily_user_activity`: Daily activity summaries
- `mv_category_performance`: Category usage metrics
- `mv_hourly_activity_patterns`: Time-based patterns
- `mv_user_streaks`: Streak calculations
- `mv_monthly_summary`: Monthly aggregations

**Refresh Strategy:**
- Full refresh: Every 6 hours (via cron job)
- Real-time refresh: On-demand for specific users (POST /api/v1/analytics/refresh)

### Query Optimization

1. Use materialized views for expensive aggregations
2. Cache results in Redis with appropriate TTLs
3. Limit result sets (default: 10-20 items)
4. Indexes on user_id, dates, and common filters

### Resource Usage

- Insights generation: ~50-100ms (cached)
- Pattern detection: ~100-200ms (cached)
- Forecast generation: ~200-300ms (cached)
- Report generation: ~500-1000ms (uncached)

## Error Handling

### Insufficient Data

Returns 500 with message:
```json
{
  "error": "insufficient data for forecasting (need at least 7 days)"
}
```

**Minimum Data Requirements:**
- Insights: 1 day of activity
- Patterns: 7 days of activity
- Forecasting: 7 days minimum, 30 days recommended

### Premium Feature Check

Returns 403 if user doesn't have Premium tier:
```json
{
  "error": "This feature requires Premium subscription"
}
```

## Testing

### Unit Tests

```bash
# Run analytics tests
go test ./internal/analytics/...

# Run with coverage
go test -cover ./internal/analytics/...

# Run only unit tests (skip integration)
go test -short ./internal/analytics/...
```

### Integration Tests

```bash
# Run integration tests (requires Docker)
go test -v ./internal/analytics/... -run Integration
```

### Load Testing

```bash
# Test insights endpoint
ab -n 1000 -c 10 -H "Authorization: Bearer TOKEN" \
   http://localhost:8080/api/v1/analytics/insights
```

## Monitoring

### Metrics to Track

1. **Response Times**:
   - P50, P95, P99 latencies for each endpoint
   - Cache hit rates

2. **Cache Performance**:
   - Hit/miss ratios
   - Eviction rates
   - Memory usage

3. **Error Rates**:
   - Insufficient data errors
   - Cache failures
   - Database timeout errors

### Logging

```go
logger.Info("Insight generation completed",
    zap.String("user_id", userID.String()),
    zap.Int("insights_count", len(insights)),
    zap.Duration("duration", time.Since(start)),
)
```

## Future Enhancements

### ML Integration (Planned)

- Replace statistical algorithms with ML models
- Train on historical data for better predictions
- Personalized recommendation engine
- Anomaly detection for unusual patterns

### Additional Features (Planned)

- PDF/CSV export formats for reports
- Scheduled automated reports
- Email notifications for insights
- Mobile-optimized insights cards
- Comparative analytics (vs similar users)
- Goal tracking and achievement predictions

## Related Documentation

- [Analytics System Architecture](./analytics-system-implementation.md)
- [Premium Features](./premium-features.md)
- [API Documentation](./api/)
- [Database Schema](./database-schema.md)
