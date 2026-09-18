# Calendar Service

The Calendar Service provides monthly calendar views and heatmap visualizations for check-in data, enabling users to track their productivity patterns over time.

## Features

- **Monthly Calendar View**: Complete calendar layout with weeks, days, and daily summaries
- **Heatmap Visualization**: GitHub-style contribution graph showing activity intensity
- **Streak Tracking**: Current and longest consecutive day streaks
- **Productivity Metrics**: Daily summaries, completion percentages, and color-coded intensity
- **Category Breakdown**: Distribution of check-ins by category
- **Timezone Support**: Full support for IANA timezones
- **Flexible Week Start**: Support for Sunday or Monday as the first day of the week
- **Navigation**: Easy month-to-month navigation with previous/next links

## API Endpoints

### GET /api/v1/calendar/monthly

Retrieves a complete monthly calendar view with daily summaries and statistics.

**Query Parameters:**
- `year` (optional): Year (default: current year)
  - Example: `2024`
  - Range: 1970-2100
- `month` (optional): Month 1-12 (default: current month)
  - Example: `11`
  - Range: 1-12
- `start_day` (optional): First day of week
  - Values: `sunday`, `monday`
  - Default: `monday`
- `timezone` (optional): IANA timezone name
  - Example: `America/New_York`, `Asia/Seoul`
  - Default: `UTC`

**Response:**
```json
{
  "year": 2024,
  "month": 11,
  "month_name": "November",
  "start_day": "monday",
  "weeks": [
    {
      "days": [
        {
          "date": "2024-11-01",
          "checkin_count": 10,
          "total_minutes": 480,
          "completion_percent": 33.33,
          "color_intensity": 2,
          "is_current_month": true,
          "is_today": false,
          "has_checkins": true
        }
        // ... 6 more days
      ]
    }
    // ... more weeks
  ],
  "summary": {
    "total_checkins": 150,
    "total_minutes": 7200,
    "days_with_checkins": 20,
    "total_days_in_month": 30,
    "average_per_day": 5.0,
    "completion_rate": 66.67,
    "most_productive_day": "2024-11-15",
    "most_productive_count": 15,
    "current_streak": 5,
    "longest_streak": 7
  },
  "categories": [
    {
      "category_id": "uuid",
      "category_name": "Work",
      "count": 100,
      "percentage": 66.67
    }
  ],
  "previous_month": "2024-10",
  "next_month": "2024-12",
  "generated_at": "2024-11-24T10:00:00Z",
  "cache_expiration": "2024-11-25T00:00:00Z"
}
```

### GET /api/v1/calendar/heatmap

Retrieves daily activity heatmap data for visualization (similar to GitHub's contribution graph).

**Query Parameters:**
- `start_date` (required): Start date in YYYY-MM-DD format
  - Example: `2024-01-01`
- `end_date` (required): End date in YYYY-MM-DD format
  - Example: `2024-12-31`
  - Maximum range: 366 days
- `timezone` (optional): IANA timezone name
  - Example: `America/New_York`
  - Default: `UTC`

**Response:**
```json
{
  "start_date": "2024-01-01",
  "end_date": "2024-12-31",
  "days": [
    {
      "date": "2024-01-01",
      "checkin_count": 8,
      "total_minutes": 360,
      "completion_percent": 25.0,
      "color_intensity": 1
    }
    // ... all days in range
  ],
  "total_days": 366,
  "active_days": 250,
  "total_checkins": 2000,
  "generated_at": "2024-11-24T10:00:00Z"
}
```

## Data Models

### CalendarDay

Represents a single day in the calendar view.

```go
type CalendarDay struct {
    Date              string  // YYYY-MM-DD format
    CheckinCount      int     // Number of check-ins
    TotalMinutes      int     // Total duration
    CompletionPercent float64 // Percentage of day covered (0-100)
    ColorIntensity    int     // Intensity level (0-4)
    IsCurrentMonth    bool    // Belongs to current month
    IsToday           bool    // Is today's date
    HasCheckins       bool    // Has any check-ins
}
```

### MonthlyCalendar

Complete monthly calendar view.

```go
type MonthlyCalendar struct {
    Year            int
    Month           int                  // 1-12
    MonthName       string               // e.g., "November"
    StartDay        StartDay             // "sunday" or "monday"
    Weeks           []*CalendarWeek      // 4-6 weeks
    Summary         *MonthlySummary      // Aggregate statistics
    Categories      []*CategoryBreakdown // Category distribution
    PreviousMonth   string               // YYYY-MM format
    NextMonth       string               // YYYY-MM format
    GeneratedAt     time.Time
    CacheExpiration time.Time
}
```

### HeatmapData

Heatmap visualization data.

```go
type HeatmapData struct {
    StartDate     string        // YYYY-MM-DD format
    EndDate       string        // YYYY-MM-DD format
    Days          []*HeatmapDay // All days in range
    TotalDays     int           // Total days in range
    ActiveDays    int           // Days with check-ins
    TotalCheckins int           // Total check-ins
    GeneratedAt   time.Time
}
```

## Color Intensity Levels

The calendar uses a 5-level color intensity system (similar to GitHub):

- **Level 0**: No activity (0%)
- **Level 1**: Low activity (1-25%)
- **Level 2**: Medium-low activity (26-50%)
- **Level 3**: Medium-high activity (51-75%)
- **Level 4**: High activity (76-100%)

Completion percentage is calculated based on total minutes tracked vs. 1440 minutes (24 hours) per day.

## Streak Calculation

Streaks track consecutive days with at least one check-in:

- **Current Streak**: Consecutive days with check-ins leading up to today
- **Longest Streak**: Highest consecutive days in the month
- Breaks: Any day without check-ins breaks the streak

## Timezone Handling

The calendar service properly handles timezones:

1. All dates stored in UTC in the database
2. Query parameters specify the user's timezone
3. Dates are converted to the user's timezone for display
4. Day boundaries respect the specified timezone

Example: A check-in at `2024-11-24 23:00 UTC` appears on:
- November 24 in UTC timezone
- November 25 in Asia/Tokyo timezone (+9 hours)

## Caching

Calendar data is cached with the following strategy:

- **Monthly Calendar**: Cached until midnight of the next day
- **Heatmap**: Cached for 1 hour
- **Cache Headers**: HTTP cache headers set for client-side caching
- **Invalidation**: Automatically expires based on time

Cache keys:
```go
calendar:{user_id}:{year}:{month}
heatmap:{user_id}:{start_date}:{end_date}
```

## Performance Optimization

The calendar service is optimized for performance:

1. **Efficient SQL Queries**: Uses aggregation at the database level
2. **Minimal Data Transfer**: Only daily summaries, not individual check-ins
3. **Date Range Limits**: Heatmap limited to 366 days maximum
4. **Index Support**: Queries use indexes on `user_id` and `checkin_time`

Expected query times:
- Monthly calendar: < 50ms
- Heatmap (1 year): < 100ms

## Usage Examples

### Basic Monthly Calendar

```bash
curl -X GET "http://localhost:8080/api/v1/calendar/monthly?year=2024&month=11" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

### Calendar with Custom Week Start

```bash
curl -X GET "http://localhost:8080/api/v1/calendar/monthly?year=2024&month=11&start_day=sunday" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

### Calendar with Timezone

```bash
curl -X GET "http://localhost:8080/api/v1/calendar/monthly?year=2024&month=11&timezone=Asia/Seoul" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

### Year-Long Heatmap

```bash
curl -X GET "http://localhost:8080/api/v1/calendar/heatmap?start_date=2024-01-01&end_date=2024-12-31" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

### Last 90 Days Heatmap

```bash
# Calculate dates dynamically
START_DATE=$(date -u -d '90 days ago' '+%Y-%m-%d')
END_DATE=$(date -u '+%Y-%m-%d')

curl -X GET "http://localhost:8080/api/v1/calendar/heatmap?start_date=$START_DATE&end_date=$END_DATE" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

## Testing

Run integration tests:

```bash
# Run all calendar tests
go test -v ./internal/calendar/...

# Run integration tests only
go test -v ./internal/calendar/ -run TestMonthlyCalendarIntegration
go test -v ./internal/calendar/ -run TestHeatmapIntegration

# Run with database
DATABASE_URL="postgres://postgres:postgres@localhost:5432/donelist_test?sslmode=disable" \
  go test -v ./internal/calendar/integration_test.go
```

## Error Handling

Common errors and responses:

| Error | Status | Response |
|-------|--------|----------|
| Invalid year | 400 | `{"error": "invalid year, must be between 1970 and 2100"}` |
| Invalid month | 400 | `{"error": "invalid month, must be between 1 and 12"}` |
| Missing start_date | 400 | `{"error": "start_date parameter is required (format: YYYY-MM-DD)"}` |
| Invalid date format | 400 | `{"error": "invalid start_date format, use YYYY-MM-DD"}` |
| Date range too large | 400 | `{"error": "date range cannot exceed 366 days"}` |
| Unauthorized | 401 | `{"error": "unauthorized"}` |
| Database error | 500 | `{"error": "failed to get monthly calendar"}` |

## Architecture

```
┌─────────────────┐
│  HTTP Handler   │  - Parse query parameters
│                 │  - Validate input
│                 │  - Return JSON
└────────┬────────┘
         │
┌────────▼────────┐
│  Service Layer  │  - Business logic
│                 │  - Date calculations
│                 │  - Aggregation
│                 │  - Timezone handling
└────────┬────────┘
         │
┌────────▼────────┐
│   Repository    │  - SQL queries
│                 │  - Data aggregation
│                 │  - Database access
└─────────────────┘
```

## Future Enhancements

Potential features for future versions:

- [ ] Year view with monthly summaries
- [ ] Custom date range statistics
- [ ] Comparison between months/years
- [ ] Export calendar data (CSV, PDF)
- [ ] Calendar sharing (team calendars)
- [ ] Weekly view with hourly breakdown
- [ ] Goal tracking and progress indicators
- [ ] Calendar events and reminders
- [ ] Integration with external calendars
- [ ] Advanced filtering by category/tags

## Related Services

- **Timeline Service**: Provides chronological check-in lists
- **Statistics Service**: Provides advanced analytics and insights
- **Checkin Service**: Creates and manages check-ins
- **Category Service**: Manages check-in categories

## References

- [IANA Time Zone Database](https://www.iana.org/time-zones)
- [ISO 8601 Date Format](https://en.wikipedia.org/wiki/ISO_8601)
- [GitHub Contribution Graph](https://docs.github.com/en/account-and-profile/setting-up-and-managing-your-github-profile/managing-contribution-graphs-on-your-profile/viewing-contributions-on-your-profile)
