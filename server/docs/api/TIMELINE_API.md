# Timeline API Documentation

## Overview

The Timeline API provides comprehensive views of user check-ins across different time periods. It includes enhanced analytics, time block visualization, gap detection, and intelligent caching for optimal performance.

**Base URL**: `/api/v1/timeline`

**Authentication**: All endpoints require Bearer token authentication.

---

## Endpoints

### 1. Get Daily Timeline (Simple)

Retrieves check-ins for a specific day without additional analytics.

```http
GET /timeline/daily
```

#### Query Parameters

| Parameter | Type   | Required | Default | Description                    |
|-----------|--------|----------|---------|--------------------------------|
| `date`    | string | No       | today   | Date in YYYY-MM-DD format      |

#### Example Request

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "https://api.example.com/api/v1/timeline/daily?date=2024-01-15"
```

#### Example Response (200 OK)

```json
{
  "date": "2024-01-15",
  "checkins": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "user_id": "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
      "title": "Morning standup",
      "description": "Daily team sync",
      "category_id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
      "checkin_time": "2024-01-15T09:00:00Z",
      "duration_minutes": 30,
      "tags": ["meeting", "team"],
      "created_at": "2024-01-15T09:00:00Z",
      "updated_at": "2024-01-15T09:00:00Z"
    }
  ],
  "total": 1
}
```

#### Error Responses

- **400 Bad Request**: Invalid date format
- **401 Unauthorized**: Missing or invalid authentication token
- **500 Internal Server Error**: Server error

---

### 2. Get Enhanced Daily Timeline

Retrieves an enhanced daily timeline with time blocks, gaps, analytics, and optional pagination. Supports intelligent caching.

```http
GET /timeline/daily/enhanced
```

#### Query Parameters

| Parameter  | Type    | Required | Default | Description                                        |
|------------|---------|----------|---------|----------------------------------------------------|
| `date`     | string  | No       | today   | Date in YYYY-MM-DD format                          |
| `block`    | integer | No       | 30      | Block granularity in minutes (15, 30, 45, or 120)  |
| `timezone` | string  | No       | UTC     | IANA timezone (e.g., "America/New_York")           |
| `limit`    | integer | No       | 0       | Number of blocks per page (0 = no pagination)      |
| `cursor`   | string  | No       | -       | Pagination cursor from previous response           |

#### Request Headers

| Header           | Required | Description                            |
|------------------|----------|----------------------------------------|
| `Authorization`  | Yes      | Bearer token                           |
| `If-None-Match`  | No       | ETag for conditional requests          |

#### Response Headers (Non-Paginated)

| Header           | Description                            |
|------------------|----------------------------------------|
| `Cache-Control`  | Cache directives (e.g., "private, max-age=300") |
| `ETag`           | Entity tag for caching                 |
| `Last-Modified`  | Last modification timestamp            |

#### Example Request (Non-Paginated)

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "https://api.example.com/api/v1/timeline/daily/enhanced?date=2024-01-15&block=30&timezone=UTC"
```

#### Example Response (200 OK) - Non-Paginated

```json
{
  "date": "2024-01-15",
  "timezone": "UTC",
  "block_granularity": 30,
  "blocks": [
    {
      "start_time": "2024-01-15T08:00:00Z",
      "end_time": "2024-01-15T08:30:00Z",
      "duration_mins": 30,
      "checkins": [],
      "is_empty": true,
      "is_gap": false
    },
    {
      "start_time": "2024-01-15T08:30:00Z",
      "end_time": "2024-01-15T09:00:00Z",
      "duration_mins": 30,
      "checkins": [
        {
          "id": "550e8400-e29b-41d4-a716-446655440000",
          "user_id": "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
          "title": "Morning standup",
          "description": "Daily team sync",
          "category_id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
          "category_name": "Work",
          "category_color": "#3498db",
          "category_icon": "briefcase",
          "checkin_time": "2024-01-15T08:30:00Z",
          "duration_minutes": 30,
          "tags": ["meeting"],
          "created_at": "2024-01-15T08:30:00Z",
          "updated_at": "2024-01-15T08:30:00Z"
        }
      ],
      "is_empty": false,
      "is_gap": false
    }
  ],
  "gaps": [
    {
      "start_time": "2024-01-15T09:00:00Z",
      "end_time": "2024-01-15T10:30:00Z",
      "duration_mins": 90
    }
  ],
  "summary": {
    "date": "2024-01-15",
    "total_checkins": 5,
    "total_minutes": 150,
    "first_checkin_time": "2024-01-15T08:30:00Z",
    "last_checkin_time": "2024-01-15T18:00:00Z",
    "active_hours": 9.5,
    "gap_count": 2,
    "total_gap_minutes": 90,
    "completion_percent": 10.42,
    "average_gap_minutes": 45.0
  },
  "category_legend": [
    {
      "category_id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
      "category_name": "Work",
      "color": "#3498db",
      "icon": "briefcase",
      "count": 3
    }
  ],
  "previous_day": "2024-01-14",
  "next_day": "2024-01-16",
  "generated_at": "2024-01-15T10:30:00Z"
}
```

#### Example Request (Paginated)

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "https://api.example.com/api/v1/timeline/daily/enhanced?date=2024-01-15&limit=24"
```

#### Example Response (200 OK) - Paginated

```json
{
  "date": "2024-01-15",
  "timezone": "UTC",
  "block_granularity": 30,
  "blocks": [
    {
      "start_time": "2024-01-15T00:00:00Z",
      "end_time": "2024-01-15T00:30:00Z",
      "duration_mins": 30,
      "checkins": [],
      "is_empty": true,
      "is_gap": false
    }
  ],
  "pagination": {
    "limit": 24,
    "has_next": true,
    "next_cursor": "eyJ0aW1lIjoiMjAyNC0wMS0xNVQxMjowMDowMFoifQ==",
    "total_count": 48
  },
  "gaps": [],
  "summary": {
    "date": "2024-01-15",
    "total_checkins": 5,
    "total_minutes": 150
  },
  "category_legend": [],
  "previous_day": "2024-01-14",
  "next_day": "2024-01-16",
  "generated_at": "2024-01-15T10:30:00Z"
}
```

#### Conditional Requests (ETag)

```bash
# First request - get ETag
RESPONSE=$(curl -i -H "Authorization: Bearer $TOKEN" \
  "https://api.example.com/api/v1/timeline/daily/enhanced?date=2024-01-15")

# Extract ETag
ETAG=$(echo "$RESPONSE" | grep -i "etag:" | cut -d' ' -f2 | tr -d '\r')

# Subsequent request with If-None-Match
curl -H "Authorization: Bearer $TOKEN" \
  -H "If-None-Match: $ETAG" \
  "https://api.example.com/api/v1/timeline/daily/enhanced?date=2024-01-15"
```

#### Response (304 Not Modified)

When ETag matches, returns 304 with empty body. Client should use cached version.

#### Error Responses

- **400 Bad Request**: Invalid parameters
  ```json
  {
    "error": "invalid block granularity, use 15, 30, 45, or 120"
  }
  ```
- **401 Unauthorized**: Missing/invalid token
- **500 Internal Server Error**: Server error

---

### 3. Get Weekly Timeline

Retrieves check-ins for a week (Monday-Sunday), grouped by day.

```http
GET /timeline/weekly
```

#### Query Parameters

| Parameter | Type   | Required | Default      | Description                            |
|-----------|--------|----------|--------------|----------------------------------------|
| `date`    | string | No       | current week | Any date within the target week        |

#### Example Request

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "https://api.example.com/api/v1/timeline/weekly?date=2024-01-15"
```

#### Example Response (200 OK)

```json
{
  "start_date": "2024-01-15",
  "end_date": "2024-01-21",
  "days": [
    {
      "date": "2024-01-15",
      "checkins": [
        {
          "id": "550e8400-e29b-41d4-a716-446655440000",
          "user_id": "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
          "title": "Morning standup",
          "checkin_time": "2024-01-15T09:00:00Z",
          "duration_minutes": 30,
          "created_at": "2024-01-15T09:00:00Z",
          "updated_at": "2024-01-15T09:00:00Z"
        }
      ],
      "total": 1
    },
    {
      "date": "2024-01-16",
      "checkins": [],
      "total": 0
    }
  ],
  "total": 1
}
```

---

### 4. Get Monthly Timeline

Retrieves check-ins for a month, grouped by day.

```http
GET /timeline/monthly
```

#### Query Parameters

| Parameter | Type   | Required | Default       | Description                            |
|-----------|--------|----------|---------------|----------------------------------------|
| `date`    | string | No       | current month | Any date within the target month       |

#### Example Request

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "https://api.example.com/api/v1/timeline/monthly?date=2024-01-15"
```

#### Example Response (200 OK)

```json
{
  "year": 2024,
  "month": 1,
  "days": [
    {
      "date": "2024-01-01",
      "checkins": [],
      "total": 0
    },
    {
      "date": "2024-01-15",
      "checkins": [
        {
          "id": "550e8400-e29b-41d4-a716-446655440000",
          "title": "Project work",
          "checkin_time": "2024-01-15T14:00:00Z",
          "duration_minutes": 120
        }
      ],
      "total": 1
    }
  ],
  "total": 1
}
```

---

## Data Models

### TimeBlock

Represents a time slot in the enhanced timeline.

| Field           | Type                  | Description                              |
|-----------------|-----------------------|------------------------------------------|
| `start_time`    | string (RFC3339)      | Block start time                         |
| `end_time`      | string (RFC3339)      | Block end time                           |
| `duration_mins` | integer               | Block duration in minutes                |
| `checkins`      | array[CheckinWithMeta]| Check-ins within this block              |
| `is_empty`      | boolean               | True if no check-ins                     |
| `is_gap`        | boolean               | True if gap between check-ins            |

### CheckinWithMeta

Check-in enriched with category metadata.

Extends base `Checkin` with:

| Field             | Type   | Description              |
|-------------------|--------|--------------------------|
| `category_name`   | string | Category name            |
| `category_color`  | string | Category color (hex)     |
| `category_icon`   | string | Category icon            |

### Gap

Represents a gap between consecutive check-ins.

| Field           | Type             | Description                |
|-----------------|------------------|----------------------------|
| `start_time`    | string (RFC3339) | Gap start time             |
| `end_time`      | string (RFC3339) | Gap end time               |
| `duration_mins` | integer          | Gap duration in minutes    |

### DailySummary

Aggregate statistics for a day.

| Field                  | Type    | Description                              |
|------------------------|---------|------------------------------------------|
| `date`                 | string  | Date (YYYY-MM-DD)                        |
| `total_checkins`       | integer | Total number of check-ins                |
| `total_minutes`        | integer | Total tracked time                       |
| `first_checkin_time`   | string  | Time of first check-in (RFC3339)         |
| `last_checkin_time`    | string  | Time of last check-in (RFC3339)          |
| `active_hours`         | float   | Hours with activity                      |
| `gap_count`            | integer | Number of gaps                           |
| `total_gap_minutes`    | integer | Total gap duration                       |
| `completion_percent`   | float   | Percentage of day tracked (0-100)        |
| `average_gap_minutes`  | float   | Average gap duration                     |

### CategoryLegend

Category metadata with usage count.

| Field           | Type   | Description                       |
|-----------------|--------|-----------------------------------|
| `category_id`   | string | Category UUID                     |
| `category_name` | string | Category name                     |
| `color`         | string | Category color (hex)              |
| `icon`          | string | Category icon                     |
| `count`         | integer| Number of check-ins in category   |

### PaginationMeta

Pagination metadata.

| Field         | Type    | Description                        |
|---------------|---------|------------------------------------|
| `limit`       | integer | Page size                          |
| `has_next`    | boolean | Whether more pages exist           |
| `next_cursor` | string  | Cursor for next page (if has_next) |
| `total_count` | integer | Total number of items              |

---

## Caching Strategy

### Cache Behavior

**Cached Requests** (Non-Paginated):
- Full day timeline requests without pagination
- Cache TTL varies by date:
  - Current day: 5 minutes
  - Past days: 1 hour
  - Future days: 15 minutes

**Non-Cached Requests**:
- Paginated requests (with `limit` parameter)
- After cache invalidation

### Cache Invalidation

Cache is automatically invalidated when:
1. User creates a check-in on that date
2. User updates a check-in on that date
3. User deletes a check-in on that date

### Using ETags

Clients should store ETags and use them in subsequent requests:

```javascript
// Store ETag from response
const etag = response.headers.get('ETag');

// Use in subsequent request
fetch(url, {
  headers: {
    'Authorization': `Bearer ${token}`,
    'If-None-Match': etag
  }
});

// Handle 304 Not Modified
if (response.status === 304) {
  // Use cached data
  return cachedData;
}
```

---

## Best Practices

### For Clients

1. **Use ETags**: Store and send ETags for conditional requests
2. **Handle 304**: Implement logic to use cached data on 304 responses
3. **Pagination**: Use pagination for mobile apps to reduce data transfer
4. **Timezone**: Always specify user's timezone for accurate time blocks
5. **Error Handling**: Gracefully handle validation errors

### For Servers

1. **Monitor Cache Hit Rate**: Target >75% cache hit rate
2. **Database Indexes**: Ensure timeline queries use proper indexes
3. **Query Performance**: Monitor slow query logs
4. **Cache Memory**: Monitor Redis memory usage

---

## Performance Characteristics

### Response Times

| Scenario                    | Typical Response Time |
|-----------------------------|-----------------------|
| Cache hit                   | ~5ms                  |
| Cache miss (first request)  | ~50ms                 |
| Paginated request           | ~50ms                 |
| Database only (no cache)    | ~150ms                |

### Pagination Guidelines

| Use Case          | Recommended Limit |
|-------------------|-------------------|
| Mobile app        | 24-48 blocks      |
| Web dashboard     | 48-96 blocks      |
| Full day load     | No limit (0)      |

---

## Examples

See [timeline_examples.json](./timeline_examples.json) for comprehensive request/response examples including:

- Simple daily timeline
- Enhanced timeline with multiple categories
- Paginated responses
- Error responses
- Cache headers

---

## OpenAPI Specification

Full OpenAPI 3.0 specification available in [openapi.yaml](./openapi.yaml).

To view interactive documentation:

```bash
# Using Swagger UI
docker run -p 8080:8080 -e SWAGGER_JSON=/openapi.yaml \
  -v $(pwd)/openapi.yaml:/openapi.yaml \
  swaggerapi/swagger-ui

# Or using Redoc
docker run -p 8080:80 -e SPEC_URL=/openapi.yaml \
  -v $(pwd)/openapi.yaml:/usr/share/nginx/html/openapi.yaml \
  redocly/redoc
```

---

## Testing

### Contract Tests

Contract tests are available in `internal/api/handlers/timeline_contract_test.go`.

Run contract tests:

```bash
go test -tags=contract ./internal/api/handlers/...
```

### Integration Tests

Run integration tests (requires Redis):

```bash
go test ./internal/timeline/...
```

### Manual Testing with curl

See [TIMELINE_API_QUICK_REFERENCE.md](../TIMELINE_API_QUICK_REFERENCE.md) for curl examples and debugging tips.

---

## Migration Guide

### From Simple to Enhanced Timeline

```javascript
// Old: Simple timeline
const response = await fetch('/api/v1/timeline/daily?date=2024-01-15');

// New: Enhanced timeline (backward compatible)
const response = await fetch('/api/v1/timeline/daily/enhanced?date=2024-01-15');

// Benefits:
// - Time blocks for visualization
// - Gap detection
// - Analytics
// - Caching support
// - Category enrichment
```

### Adding Pagination

```javascript
// Non-paginated (default)
const response = await fetch('/api/v1/timeline/daily/enhanced?date=2024-01-15');

// Paginated (for mobile/infinite scroll)
let cursor = null;
do {
  const url = `/api/v1/timeline/daily/enhanced?date=2024-01-15&limit=24${cursor ? `&cursor=${cursor}` : ''}`;
  const response = await fetch(url);
  const data = await response.json();

  // Process blocks
  processBlocks(data.blocks);

  // Get next cursor
  cursor = data.pagination?.next_cursor;
} while (cursor);
```

---

## Support

For issues, questions, or feature requests:

- GitHub Issues: [Project Issues](https://github.com/yourorg/donelist/issues)
- API Documentation: [Main API Docs](../API_DOCUMENTATION.md)
- Performance Monitoring: [TIMELINE_OPTIMIZATION.md](../TIMELINE_OPTIMIZATION.md)

---

**Last Updated**: 2024-11-24
**API Version**: v1
**Specification Version**: 1.0.0
