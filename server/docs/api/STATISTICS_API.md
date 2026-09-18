# Statistics API Documentation

## Overview

The Statistics API provides comprehensive analytics and insights about user check-ins, productivity patterns, and trends over time. The API offers weekly aggregations with detailed breakdowns by day, category, time of day, and more.

## Base URL

```
/api/v1/statistics
```

## Authentication

All statistics endpoints require authentication via JWT Bearer token.

```http
Authorization: Bearer <your-jwt-token>
```

## Endpoints

### Get Weekly Statistics

Retrieves comprehensive weekly statistics including productivity metrics, patterns, trends, and week-over-week comparisons.

**Endpoint:** `GET /statistics/weekly`

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `date` | string | No | Current date | Any date within the desired week (YYYY-MM-DD format) |
| `week_start` | string | No | `monday` | First day of week: `monday` or `sunday` |
| `timezone` | string | No | `UTC` | IANA timezone (e.g., `America/New_York`, `Asia/Seoul`) |

**Example Request:**

```bash
curl -X GET "http://localhost:8080/api/v1/statistics/weekly?date=2024-01-15&week_start=monday&timezone=America/New_York" \
  -H "Authorization: Bearer <your-jwt-token>"
```

**Example Response:**

```json
{
  "year": 2024,
  "week_number": 3,
  "start_date": "2024-01-15",
  "end_date": "2024-01-21",
  "week_start_day": "monday",
  "timezone": "America/New_York",
  "summary": {
    "total_checkins": 47,
    "total_minutes": 2340,
    "days_with_checkins": 5,
    "average_per_day": 6.71,
    "completion_rate": 71.43,
    "most_productive_day": "2024-01-17",
    "most_productive_count": 12,
    "least_productive_day": "2024-01-19"
  },
  "daily_breakdown": [
    {
      "date": "2024-01-15",
      "day_of_week": "Monday",
      "checkin_count": 8,
      "total_minutes": 480,
      "is_today": false,
      "has_checkins": true,
      "completion_percent": 33.33
    },
    {
      "date": "2024-01-16",
      "day_of_week": "Tuesday",
      "checkin_count": 10,
      "total_minutes": 540,
      "is_today": false,
      "has_checkins": true,
      "completion_percent": 37.5
    }
    // ... 5 more days
  ],
  "day_of_week_analysis": [
    {
      "day_of_week": "Monday",
      "checkin_count": 8,
      "total_minutes": 480,
      "average_count": 8.0,
      "percentage": 17.02
    },
    {
      "day_of_week": "Tuesday",
      "checkin_count": 10,
      "total_minutes": 540,
      "average_count": 10.0,
      "percentage": 21.28
    }
    // ... other days
  ],
  "time_distribution": [
    {
      "time_of_day": "morning",
      "checkin_count": 15,
      "total_minutes": 900,
      "percentage": 31.91
    },
    {
      "time_of_day": "afternoon",
      "checkin_count": 18,
      "total_minutes": 1080,
      "percentage": 38.30
    },
    {
      "time_of_day": "evening",
      "checkin_count": 12,
      "total_minutes": 720,
      "percentage": 25.53
    },
    {
      "time_of_day": "night",
      "checkin_count": 2,
      "total_minutes": 120,
      "percentage": 4.26
    }
  ],
  "category_breakdown": [
    {
      "category_id": "550e8400-e29b-41d4-a716-446655440000",
      "category_name": "Work",
      "checkin_count": 25,
      "total_minutes": 1500,
      "percentage": 53.19,
      "average_per_day": 3.57
    },
    {
      "category_id": "550e8400-e29b-41d4-a716-446655440001",
      "category_name": "Personal",
      "checkin_count": 22,
      "total_minutes": 840,
      "percentage": 46.81,
      "average_per_day": 3.14
    }
  ],
  "comparison": {
    "previous_week_total": 42,
    "current_week_total": 47,
    "change": 5,
    "change_percentage": 11.90,
    "is_improvement": true,
    "previous_total_minutes": 2100,
    "current_total_minutes": 2340,
    "minutes_change": 240,
    "minutes_change_percent": 11.43
  },
  "streak": {
    "current_streak": 5,
    "longest_streak": 12,
    "is_streak_active": true,
    "last_checkin_date": "2024-01-21",
    "next_milestone": 7,
    "days_until_milestone": 2
  },
  "previous_week": "2024-W02",
  "next_week": "2024-W04",
  "generated_at": "2024-01-21T14:30:00Z",
  "cache_expiration": "2024-01-22T00:00:00Z"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `year` | integer | ISO year of the week |
| `week_number` | integer | ISO week number (1-53) |
| `start_date` | string | First day of the week (YYYY-MM-DD) |
| `end_date` | string | Last day of the week (YYYY-MM-DD) |
| `week_start_day` | string | Week start setting used (`monday` or `sunday`) |
| `timezone` | string | Timezone used for calculations |

**Summary Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `total_checkins` | integer | Total check-ins for the week |
| `total_minutes` | integer | Total duration in minutes |
| `days_with_checkins` | integer | Number of days with at least one check-in (0-7) |
| `average_per_day` | float | Average check-ins per day (total/7) |
| `completion_rate` | float | Percentage of days with check-ins (0-100) |
| `most_productive_day` | string | Date of day with most check-ins |
| `most_productive_count` | integer | Number of check-ins on most productive day |
| `least_productive_day` | string | Date of day with least check-ins (excluding 0) |

**Daily Breakdown Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `date` | string | Date (YYYY-MM-DD) |
| `day_of_week` | string | Day name (Monday, Tuesday, etc.) |
| `checkin_count` | integer | Number of check-ins |
| `total_minutes` | integer | Total duration in minutes |
| `is_today` | boolean | Whether this is the current day |
| `has_checkins` | boolean | Whether there are any check-ins |
| `completion_percent` | float | Percentage of day covered (based on 1440 minutes/day) |

**Time Distribution:**

Time periods are defined as:
- **Morning**: 06:00 - 12:00
- **Afternoon**: 12:00 - 18:00
- **Evening**: 18:00 - 23:00
- **Night**: 23:00 - 06:00

**Streak Milestones:**

The API tracks streak milestones at: 7, 14, 21, 30, 60, 90, 180, 365 days, then every 100 days.

**Status Codes:**

| Code | Description |
|------|-------------|
| 200 | Success - Returns weekly statistics |
| 400 | Bad Request - Invalid parameters (date format, week_start, or timezone) |
| 401 | Unauthorized - Missing or invalid authentication token |
| 500 | Internal Server Error |

**Error Response Example:**

```json
{
  "error": "invalid date format, use YYYY-MM-DD"
}
```

## Use Cases

### 1. Display Weekly Dashboard

Get the current week's statistics for the logged-in user:

```bash
curl -X GET "http://localhost:8080/api/v1/statistics/weekly" \
  -H "Authorization: Bearer <token>"
```

### 2. Historical Analysis

Analyze a specific past week:

```bash
curl -X GET "http://localhost:8080/api/v1/statistics/weekly?date=2024-01-01" \
  -H "Authorization: Bearer <token>"
```

### 3. International Users

Get statistics in user's local timezone:

```bash
curl -X GET "http://localhost:8080/api/v1/statistics/weekly?timezone=Asia/Seoul&week_start=sunday" \
  -H "Authorization: Bearer <token>"
```

### 4. Week-over-Week Tracking

Fetch multiple consecutive weeks to build trend charts:

```bash
# Week 1
curl -X GET "http://localhost:8080/api/v1/statistics/weekly?date=2024-01-01" \
  -H "Authorization: Bearer <token>"

# Week 2
curl -X GET "http://localhost:8080/api/v1/statistics/weekly?date=2024-01-08" \
  -H "Authorization: Bearer <token>"

# Week 3
curl -X GET "http://localhost:8080/api/v1/statistics/weekly?date=2024-01-15" \
  -H "Authorization: Bearer <token>"
```

## Caching

The weekly statistics endpoint implements caching with:
- Cache expiration at midnight of the next day (local timezone)
- Results for historical weeks (not current week) are cached longer
- Cache key includes user ID, week start date, week start day, and timezone

## Performance Considerations

- The endpoint aggregates data from multiple database queries
- For optimal performance, results are cached in Redis
- Historical week queries are typically faster due to aggressive caching
- Timezone conversions are handled server-side for accuracy

## Best Practices

1. **Timezone Handling**: Always pass the user's timezone for accurate local time calculations
2. **Week Start Preference**: Store user's week start preference (Monday/Sunday) in profile settings
3. **Pagination**: For trend analysis over many weeks, fetch data in batches
4. **Caching**: Client-side caching of historical weeks is recommended
5. **Error Handling**: Always handle 400 errors for invalid timezone names

## Related Endpoints

- `GET /timeline/weekly` - Raw check-in data grouped by week
- `GET /timeline/daily` - Daily check-in timeline
- `GET /calendar/monthly` - Monthly calendar view with check-in indicators

## Changelog

### Version 1.0.0 (2024-01-24)
- Initial release of weekly statistics API
- Support for Monday/Sunday week start
- Timezone-aware calculations
- Comprehensive metrics including:
  - Weekly summary with totals and averages
  - Daily breakdown for 7 days
  - Day-of-week pattern analysis
  - Time-of-day distribution
  - Category breakdown
  - Week-over-week comparison
  - Streak tracking with milestones
