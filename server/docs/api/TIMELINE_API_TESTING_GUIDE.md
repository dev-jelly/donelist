# Timeline API Testing Guide

## Quick Validation

All deliverables for Task 5.7 have been validated:

```bash
# Validate OpenAPI specification
python3 -c "import yaml; yaml.safe_load(open('docs/api/openapi.yaml')); print('✅ OpenAPI YAML is valid')"

# Validate example responses
python3 -c "import json; json.load(open('docs/api/timeline_examples.json')); print('✅ Examples JSON is valid')"

# Compile contract tests
go test -tags=contract -c ./internal/api/handlers/
```

---

## Running Tests

### Contract Tests

Contract tests validate that the API implementation adheres to the OpenAPI specification.

```bash
# Run all contract tests
go test -tags=contract ./internal/api/handlers/

# Run with verbose output
go test -tags=contract -v ./internal/api/handlers/

# Run with coverage
go test -tags=contract -cover ./internal/api/handlers/

# Run specific test
go test -tags=contract -v -run TestTimelineAPIContract ./internal/api/handlers/
```

**Requirements**:
- PostgreSQL database (test database will be created)
- Environment variables or `.env` file with database connection

### Integration Tests

```bash
# Run timeline integration tests (requires Redis)
go test ./internal/timeline/

# Skip integration tests
go test -short ./internal/timeline/

# Run with race detector
go test -race ./internal/timeline/
```

**Requirements**:
- Redis running on localhost:6379
- PostgreSQL database

### Unit Tests

```bash
# Run service tests
go test ./internal/timeline/ -run TestService

# Run cache tests
go test ./internal/timeline/ -run TestCache

# Run pagination tests
go test ./internal/timeline/ -run TestPagination
```

---

## Manual Testing

### Setup

```bash
# Set environment variables
export API_BASE_URL="http://localhost:8080"
export AUTH_TOKEN="your-jwt-token-here"

# Or use .env file
cat > .env <<EOF
API_BASE_URL=http://localhost:8080
AUTH_TOKEN=your-jwt-token-here
EOF
```

### Basic Endpoint Tests

```bash
# 1. Test simple daily timeline
curl -H "Authorization: Bearer $AUTH_TOKEN" \
  "$API_BASE_URL/api/v1/timeline/daily?date=2024-01-15" | jq .

# 2. Test enhanced daily timeline
curl -H "Authorization: Bearer $AUTH_TOKEN" \
  "$API_BASE_URL/api/v1/timeline/daily/enhanced?date=2024-01-15" | jq .

# 3. Test with different block sizes
curl -H "Authorization: Bearer $AUTH_TOKEN" \
  "$API_BASE_URL/api/v1/timeline/daily/enhanced?date=2024-01-15&block=15" | jq .

# 4. Test with timezone
curl -H "Authorization: Bearer $AUTH_TOKEN" \
  "$API_BASE_URL/api/v1/timeline/daily/enhanced?date=2024-01-15&timezone=America/New_York" | jq .

# 5. Test pagination
curl -H "Authorization: Bearer $AUTH_TOKEN" \
  "$API_BASE_URL/api/v1/timeline/daily/enhanced?date=2024-01-15&limit=24" | jq .

# 6. Test weekly timeline
curl -H "Authorization: Bearer $AUTH_TOKEN" \
  "$API_BASE_URL/api/v1/timeline/weekly?date=2024-01-15" | jq .

# 7. Test monthly timeline
curl -H "Authorization: Bearer $AUTH_TOKEN" \
  "$API_BASE_URL/api/v1/timeline/monthly?date=2024-01-15" | jq .
```

### Cache Testing

```bash
# Test ETag support
RESPONSE=$(curl -i -H "Authorization: Bearer $AUTH_TOKEN" \
  "$API_BASE_URL/api/v1/timeline/daily/enhanced?date=2024-01-15")

# Extract ETag
ETAG=$(echo "$RESPONSE" | grep -i "etag:" | cut -d' ' -f2 | tr -d '\r')

# Test conditional request (should return 304)
curl -i -H "Authorization: Bearer $AUTH_TOKEN" \
  -H "If-None-Match: $ETAG" \
  "$API_BASE_URL/api/v1/timeline/daily/enhanced?date=2024-01-15"
```

### Error Testing

```bash
# Test invalid date format
curl -i -H "Authorization: Bearer $AUTH_TOKEN" \
  "$API_BASE_URL/api/v1/timeline/daily/enhanced?date=invalid-date"
# Expected: 400 Bad Request

# Test invalid block granularity
curl -i -H "Authorization: Bearer $AUTH_TOKEN" \
  "$API_BASE_URL/api/v1/timeline/daily/enhanced?date=2024-01-15&block=25"
# Expected: 400 Bad Request

# Test invalid limit
curl -i -H "Authorization: Bearer $AUTH_TOKEN" \
  "$API_BASE_URL/api/v1/timeline/daily/enhanced?date=2024-01-15&limit=2000"
# Expected: 400 Bad Request

# Test unauthorized access
curl -i "$API_BASE_URL/api/v1/timeline/daily/enhanced?date=2024-01-15"
# Expected: 401 Unauthorized
```

---

## Testing with Postman

### Import Collection

A Postman collection is available at `docs/api/DoneList_API.postman_collection.json`.

**Import Steps**:
1. Open Postman
2. Click Import
3. Select `docs/api/DoneList_API.postman_collection.json`
4. Set environment variable `baseUrl` to your API URL
5. Set environment variable `token` to your JWT token

### Timeline Requests

The collection includes:
- Daily timeline (simple)
- Daily timeline (enhanced)
- Daily timeline (paginated)
- Weekly timeline
- Monthly timeline
- Error scenarios

---

## Load Testing

### Using Apache Bench (ab)

```bash
# Test non-paginated endpoint
ab -n 1000 -c 10 \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  "$API_BASE_URL/api/v1/timeline/daily/enhanced?date=2024-01-15"

# Test paginated endpoint
ab -n 1000 -c 10 \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  "$API_BASE_URL/api/v1/timeline/daily/enhanced?date=2024-01-15&limit=24"
```

### Using k6

```javascript
import http from 'k6/http';
import { check, sleep } from 'k6';

export let options = {
  vus: 10,
  duration: '30s',
};

export default function() {
  const token = __ENV.AUTH_TOKEN;
  const baseUrl = __ENV.API_BASE_URL;

  const params = {
    headers: {
      'Authorization': `Bearer ${token}`,
    },
  };

  // Test enhanced timeline
  let res = http.get(
    `${baseUrl}/api/v1/timeline/daily/enhanced?date=2024-01-15`,
    params
  );

  check(res, {
    'status is 200': (r) => r.status === 200,
    'has blocks': (r) => JSON.parse(r.body).blocks !== undefined,
  });

  sleep(1);
}
```

Run k6 test:
```bash
k6 run -e AUTH_TOKEN="$AUTH_TOKEN" -e API_BASE_URL="$API_BASE_URL" timeline_load_test.js
```

---

## Performance Benchmarks

### Expected Performance

Based on implementation and caching:

| Scenario | Target | Acceptable | Warning |
|----------|--------|------------|---------|
| Cache hit | <10ms | <50ms | >100ms |
| Cache miss | <100ms | <200ms | >500ms |
| Paginated | <100ms | <200ms | >500ms |

### Running Benchmarks

```bash
# Run Go benchmarks
go test -bench=. -benchmem ./internal/timeline/

# Sample output:
# BenchmarkCacheOperations/Set-8    5000    250000 ns/op    1024 B/op    15 allocs/op
# BenchmarkCacheOperations/Get-8    10000   150000 ns/op    512 B/op     10 allocs/op
```

---

## OpenAPI Validation Tools

### Using Swagger Validator

```bash
# Install swagger-cli
npm install -g @apidevtools/swagger-cli

# Validate OpenAPI spec
swagger-cli validate docs/api/openapi.yaml
```

### Using openapi-spec-validator

```bash
# Install validator
pip install openapi-spec-validator

# Validate spec
openapi-spec-validator docs/api/openapi.yaml
```

### Using Docker

```bash
# Swagger UI
docker run -p 8080:8080 \
  -e SWAGGER_JSON=/openapi.yaml \
  -v $(pwd)/docs/api/openapi.yaml:/openapi.yaml \
  swaggerapi/swagger-ui

# Access at http://localhost:8080

# Redoc
docker run -p 8080:80 \
  -e SPEC_URL=openapi.yaml \
  -v $(pwd)/docs/api/openapi.yaml:/usr/share/nginx/html/openapi.yaml \
  redocly/redoc

# Access at http://localhost:8080
```

---

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Timeline API Tests

on: [push, pull_request]

jobs:
  contract-tests:
    runs-on: ubuntu-latest

    services:
      postgres:
        image: postgres:15
        env:
          POSTGRES_PASSWORD: testpass
          POSTGRES_DB: testdb
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
        ports:
          - 5432:5432

      redis:
        image: redis:7
        options: >-
          --health-cmd "redis-cli ping"
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
        ports:
          - 6379:6379

    steps:
      - uses: actions/checkout@v3

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.22'

      - name: Run Contract Tests
        run: go test -tags=contract -v ./internal/api/handlers/
        env:
          DATABASE_URL: postgres://postgres:testpass@localhost:5432/testdb

      - name: Run Integration Tests
        run: go test -v ./internal/timeline/
        env:
          REDIS_URL: redis://localhost:6379

      - name: Validate OpenAPI Spec
        run: |
          pip install openapi-spec-validator
          openapi-spec-validator docs/api/openapi.yaml
```

---

## Debugging Tips

### Enable Debug Logging

```bash
# Set log level
export LOG_LEVEL=debug

# Run server
go run cmd/api/main.go
```

### Check Cache State

```bash
# Connect to Redis
redis-cli

# List all timeline cache keys
KEYS timeline:*

# Get specific cache entry
GET timeline:user-id:2024-01-15:UTC:30:0:

# Check TTL
TTL timeline:user-id:2024-01-15:UTC:30:0:

# Clear all timeline cache
FLUSHDB
```

### Database Query Analysis

```sql
-- Check for slow queries
SELECT query, mean_exec_time, calls
FROM pg_stat_statements
WHERE query LIKE '%checkins%'
ORDER BY mean_exec_time DESC
LIMIT 10;

-- Verify index usage
SELECT schemaname, tablename, indexname, idx_scan
FROM pg_stat_user_indexes
WHERE tablename = 'checkins'
ORDER BY idx_scan DESC;

-- Check table statistics
SELECT * FROM pg_stat_user_tables WHERE relname = 'checkins';
```

---

## Response Validation

### Using jq

```bash
# Validate response structure
curl -s -H "Authorization: Bearer $AUTH_TOKEN" \
  "$API_BASE_URL/api/v1/timeline/daily/enhanced?date=2024-01-15" | \
  jq -e '.date and .timezone and .blocks and .summary' && \
  echo "✅ Required fields present"

# Check block granularity
curl -s -H "Authorization: Bearer $AUTH_TOKEN" \
  "$API_BASE_URL/api/v1/timeline/daily/enhanced?date=2024-01-15&block=30" | \
  jq -e '.block_granularity == 30' && \
  echo "✅ Block granularity correct"

# Validate date format
curl -s -H "Authorization: Bearer $AUTH_TOKEN" \
  "$API_BASE_URL/api/v1/timeline/daily/enhanced?date=2024-01-15" | \
  jq -e '.date | test("^[0-9]{4}-[0-9]{2}-[0-9]{2}$")' && \
  echo "✅ Date format valid"
```

---

## Test Data Setup

### Creating Test Check-ins

```bash
# Create category first
CATEGORY_RESPONSE=$(curl -s -X POST \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"Work","color":"#3498db","icon":"briefcase"}' \
  "$API_BASE_URL/api/v1/categories")

CATEGORY_ID=$(echo $CATEGORY_RESPONSE | jq -r '.id')

# Create check-ins for today
for hour in 9 11 14 16; do
  curl -s -X POST \
    -H "Authorization: Bearer $AUTH_TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
      \"title\": \"Task at ${hour}:00\",
      \"description\": \"Test checkin\",
      \"category_id\": \"$CATEGORY_ID\",
      \"checkin_time\": \"$(date -u +%Y-%m-%d)T${hour}:00:00Z\",
      \"duration_minutes\": 30
    }" \
    "$API_BASE_URL/api/v1/checkins"
done

echo "✅ Test data created"
```

---

## Common Issues and Solutions

### Issue: Contract tests fail with compilation errors

**Solution**: Ensure dependencies are up to date
```bash
go mod tidy
go mod download
```

### Issue: 401 Unauthorized errors

**Solution**: Check JWT token is valid and not expired
```bash
# Decode JWT to check expiration
echo $AUTH_TOKEN | cut -d. -f2 | base64 -d | jq .
```

### Issue: Cache not working

**Solution**: Verify Redis connection
```bash
redis-cli ping
# Should return: PONG
```

### Issue: Slow response times

**Solution**: Check database indexes
```sql
\d+ checkins
-- Verify indexes exist:
-- - idx_checkins_user_time_deleted
-- - idx_checkins_user_category_time
```

---

## Test Checklist

Before deploying, verify:

- [ ] All contract tests pass
- [ ] Integration tests pass with Redis
- [ ] OpenAPI spec validates
- [ ] Example JSON validates
- [ ] Manual curl tests work for all endpoints
- [ ] Error cases return appropriate status codes
- [ ] ETag support works (304 responses)
- [ ] Pagination works correctly
- [ ] Cache hit rate >75% after warmup
- [ ] Response times meet targets
- [ ] Documentation is complete and accurate

---

## Additional Resources

- **OpenAPI Spec**: `/docs/api/openapi.yaml`
- **API Documentation**: `/docs/api/TIMELINE_API.md`
- **Quick Reference**: `/docs/TIMELINE_API_QUICK_REFERENCE.md`
- **Examples**: `/docs/api/timeline_examples.json`
- **Contract Summary**: `/docs/TIMELINE_API_CONTRACT_SUMMARY.md`

---

**Last Updated**: 2024-11-24
