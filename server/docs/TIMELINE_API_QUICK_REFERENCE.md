# Timeline API Quick Reference

## API Endpoints

### Get Daily Timeline (Enhanced with Caching)
```http
GET /api/v1/timeline/daily/enhanced
```

**Query Parameters:**
- `date` (string): Date in YYYY-MM-DD format (default: today)
- `block` (int): Block granularity in minutes: 15, 30, 45, 120 (default: 30)
- `timezone` (string): IANA timezone (default: UTC)
- `limit` (int): Page size for pagination 0-1000 (optional, default: no pagination)
- `cursor` (string): Pagination cursor from previous response (optional)

**Request Headers:**
- `Authorization: Bearer <token>` (required)
- `If-None-Match: "<etag>"` (optional, for conditional requests)

**Response Headers:**
- `Cache-Control: private, max-age=300` (non-paginated only)
- `ETag: "<etag-value>"` (non-paginated only)
- `Last-Modified: <timestamp>` (non-paginated only)

**Response (200 OK):**
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
      "checkins": [...],
      "is_empty": false,
      "is_gap": false
    }
  ],
  "pagination": {
    "limit": 24,
    "has_next": true,
    "next_cursor": "eyJ0aW1lIjoiMjAyNC0...",
    "total_count": 48
  },
  "gaps": [...],
  "summary": {
    "date": "2024-01-15",
    "total_checkins": 15,
    "total_minutes": 450,
    "first_checkin_time": "2024-01-15T08:30:00Z",
    "last_checkin_time": "2024-01-15T18:00:00Z",
    "active_hours": 9.5,
    "gap_count": 2,
    "total_gap_minutes": 90,
    "completion_percent": 31.25,
    "average_gap_minutes": 45.0
  },
  "category_legend": [...],
  "previous_day": "2024-01-14",
  "next_day": "2024-01-16",
  "generated_at": "2024-01-15T10:30:00Z"
}
```

**Response (304 Not Modified):**
- Returns when ETag matches If-None-Match header
- No body returned
- Client should use cached version

**Error Responses:**
- `400 Bad Request`: Invalid parameters
- `401 Unauthorized`: Missing or invalid token
- `500 Internal Server Error`: Server error

## Usage Examples

### 1. Basic Timeline (Cached)
```bash
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/v1/timeline/daily/enhanced?date=2024-01-15"
```

### 2. With Custom Block Size and Timezone
```bash
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/v1/timeline/daily/enhanced?date=2024-01-15&block=15&timezone=America/New_York"
```

### 3. Paginated Request (First Page)
```bash
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/v1/timeline/daily/enhanced?date=2024-01-15&limit=24"
```

### 4. Paginated Request (Next Page)
```bash
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/v1/timeline/daily/enhanced?date=2024-01-15&limit=24&cursor=eyJ0aW1lI..."
```

### 5. Conditional Request (ETag)
```bash
# First request
RESPONSE=$(curl -i -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/v1/timeline/daily/enhanced?date=2024-01-15")
ETAG=$(echo "$RESPONSE" | grep -i "etag:" | cut -d' ' -f2 | tr -d '\r')

# Subsequent request
curl -H "Authorization: Bearer $TOKEN" \
  -H "If-None-Match: $ETAG" \
  "http://localhost:8080/api/v1/timeline/daily/enhanced?date=2024-01-15"
```

## Caching Behavior

### When Cache is Used:
- ✅ Non-paginated requests (`limit` not specified)
- ✅ Repeated requests for the same date/timezone/granularity
- ✅ After TTL expires, cache is refreshed

### When Cache is NOT Used:
- ❌ Paginated requests (`limit` specified)
- ❌ After check-in create/update/delete on that date
- ❌ After TTL expires (automatic refresh)

### TTL (Time-To-Live):
- **Current day**: 5 minutes
- **Past days**: 1 hour
- **Future days**: 15 minutes

### Cache Invalidation:
Cache is automatically invalidated when:
1. User creates a check-in → invalidates that date
2. User updates a check-in → invalidates that date
3. User deletes a check-in → invalidates that date

## Performance Characteristics

### Response Times (typical):

| Scenario | Response Time |
|----------|--------------|
| Cache hit | ~5ms |
| Cache miss (first request) | ~50ms |
| Paginated request | ~50ms |
| Database only (no cache) | ~150ms |

### Pagination Guidelines:

| Use Case | Recommended Limit |
|----------|------------------|
| Mobile app | 24-48 blocks |
| Web dashboard | 48-96 blocks |
| Full day load | No limit (default) |

## Database Indexes

All timeline queries use optimized indexes:

### Primary Index:
```sql
idx_checkins_user_time_deleted
-- Covers: WHERE user_id = ? AND checkin_time >= ? AND deleted_at IS NULL
```

### Category Filter:
```sql
idx_checkins_user_category_time
-- Covers: WHERE user_id = ? AND category_id = ? AND checkin_time >= ?
```

### Gap Detection:
```sql
idx_checkins_gap_detection
-- Covers: Gap calculations between consecutive check-ins
```

## Monitoring

### Cache Metrics:
```bash
# Check cache hit rate
redis-cli INFO stats | grep keyspace_hits
redis-cli INFO stats | grep keyspace_misses

# Count timeline cache keys
redis-cli --scan --pattern "timeline:*" | wc -l
```

### Database Performance:
```sql
-- Check query performance
SELECT query, mean_exec_time, calls
FROM pg_stat_statements
WHERE query LIKE '%checkins%'
ORDER BY mean_exec_time DESC
LIMIT 5;

-- Verify index usage
SELECT schemaname, tablename, indexname, idx_scan
FROM pg_stat_user_indexes
WHERE tablename = 'checkins'
ORDER BY idx_scan DESC;
```

## Troubleshooting

### Cache Not Working:
1. Check Redis connection: `redis-cli PING`
2. Verify logs for cache errors
3. Ensure Redis has sufficient memory

### Stale Data:
1. Check cache invalidation logs
2. Verify check-in service is wired up
3. Manually flush: `redis-cli DEL "timeline:*:user-id:date:*"`

### Slow Queries:
1. Check database indexes: `\d+ checkins`
2. Run EXPLAIN ANALYZE on slow queries
3. Check for table scans in query plan

### High Memory Usage:
1. Check Redis memory: `redis-cli INFO memory`
2. Review TTL settings
3. Consider LRU eviction policy

## Client SDK Examples

### JavaScript/TypeScript:
```typescript
interface TimelineParams {
  date: string;
  block?: number;
  timezone?: string;
  limit?: number;
  cursor?: string;
}

async function getTimeline(params: TimelineParams, etag?: string) {
  const url = new URL('/api/v1/timeline/daily/enhanced', baseURL);
  Object.entries(params).forEach(([key, value]) => {
    if (value !== undefined) url.searchParams.set(key, String(value));
  });

  const headers: HeadersInit = {
    'Authorization': `Bearer ${token}`,
  };
  if (etag) {
    headers['If-None-Match'] = etag;
  }

  const response = await fetch(url.toString(), { headers });

  if (response.status === 304) {
    return { cached: true }; // Use cached data
  }

  const data = await response.json();
  const newEtag = response.headers.get('ETag');

  return { data, etag: newEtag, cached: false };
}

// Usage
const { data, etag } = await getTimeline({ date: '2024-01-15' });

// Pagination
let cursor: string | undefined;
do {
  const { data } = await getTimeline({ date: '2024-01-15', limit: 24, cursor });
  // Process blocks
  cursor = data.pagination?.next_cursor;
} while (cursor);
```

### Swift (iOS):
```swift
struct TimelineRequest {
    let date: String
    var block: Int = 30
    var timezone: String = "UTC"
    var limit: Int?
    var cursor: String?
}

func fetchTimeline(params: TimelineRequest, etag: String? = nil) async throws -> TimelineResponse {
    var components = URLComponents(string: "\(baseURL)/api/v1/timeline/daily/enhanced")!
    components.queryItems = [
        URLQueryItem(name: "date", value: params.date),
        URLQueryItem(name: "block", value: "\(params.block)"),
        URLQueryItem(name: "timezone", value: params.timezone)
    ]
    if let limit = params.limit {
        components.queryItems?.append(URLQueryItem(name: "limit", value: "\(limit)"))
    }
    if let cursor = params.cursor {
        components.queryItems?.append(URLQueryItem(name: "cursor", value: cursor))
    }

    var request = URLRequest(url: components.url!)
    request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
    if let etag = etag {
        request.setValue(etag, forHTTPHeaderField: "If-None-Match")
    }

    let (data, response) = try await URLSession.shared.data(for: request)

    guard let httpResponse = response as? HTTPURLResponse else {
        throw NetworkError.invalidResponse
    }

    if httpResponse.statusCode == 304 {
        return .cached
    }

    let timeline = try JSONDecoder().decode(EnhancedDayView.self, from: data)
    let newEtag = httpResponse.value(forHTTPHeaderField: "ETag")

    return .success(timeline, etag: newEtag)
}
```

## Best Practices

### For Clients:
1. ✅ Store and use ETags for conditional requests
2. ✅ Implement exponential backoff for retries
3. ✅ Use pagination for infinite scroll
4. ✅ Cache responses locally with appropriate TTL
5. ✅ Handle 304 Not Modified responses

### For Servers:
1. ✅ Monitor cache hit rates (target: > 75%)
2. ✅ Set up alerts for high miss rates
3. ✅ Regularly analyze slow queries
4. ✅ Run VACUUM and ANALYZE on checkins table
5. ✅ Monitor Redis memory usage

### For Operations:
1. ✅ Schedule Redis backups
2. ✅ Monitor database query performance
3. ✅ Set up cache warming on deployment
4. ✅ Test cache invalidation after updates
5. ✅ Review index usage monthly

## Configuration Options

### Cache TTL:
```env
# Override default TTLs
TIMELINE_CACHE_CURRENT_TTL=5m
TIMELINE_CACHE_PAST_TTL=1h
TIMELINE_CACHE_FUTURE_TTL=15m
```

### Pagination:
```env
# Maximum page size
TIMELINE_MAX_PAGE_SIZE=1000

# Default page size
TIMELINE_DEFAULT_PAGE_SIZE=48
```

### Database:
```env
# Query timeout
DB_QUERY_TIMEOUT=30s

# Connection pool
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=5
```

---

**Last Updated**: 2024-11-24
**Version**: 1.0.0
