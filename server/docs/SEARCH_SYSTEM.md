# Search System Documentation

## Overview

The Donelist search system provides full-text search capabilities with advanced filtering, faceting, caching, and real-time indexing for check-in records. It's built on PostgreSQL's full-text search with Korean language support.

## Architecture

### Components

1. **Repository Layer** (`internal/search/repository.go`)
   - Database queries and data access
   - Full-text search using PostgreSQL tsvector
   - Faceted search aggregations
   - Search history and saved searches

2. **Service Layer** (`internal/search/service.go`)
   - Business logic and orchestration
   - Redis caching integration
   - Metrics tracking
   - Permission checks for premium features

3. **Indexer** (`internal/search/indexer.go`)
   - Real-time index synchronization
   - Bulk initial loading
   - Event-based CDC (Change Data Capture)
   - Retry logic with exponential backoff

4. **Cache Service** (`internal/search/cache.go`)
   - Redis-based result caching
   - Facet caching
   - Cache invalidation strategies
   - MD5-based cache keys

5. **Query Builder** (`internal/search/query_builder.go`)
   - Secure parameterized query construction
   - Filter composition
   - SQL injection prevention

6. **Advanced Query** (`internal/search/advanced_query.go`)
   - Configurable highlighting
   - Korean text support
   - Query boosting
   - Fuzzy matching

7. **Metrics** (`internal/search/metrics.go`)
   - Prometheus metrics
   - Performance tracking
   - Query complexity analysis
   - Cache hit rates

8. **Reindexer** (`internal/search/reindex.go`)
   - Manual reindexing operations
   - Index verification
   - Batch processing
   - Zero-downtime reindexing

## Features

### Full-Text Search

```go
filters := SearchFilters{
    Query: "운동 workout",  // Supports Korean and English
    Limit: 20,
}

results, err := searchService.Search(ctx, userID, filters)
```

### Advanced Filtering

```go
filters := SearchFilters{
    Query:         "study",
    StartDate:     &startDate,
    EndDate:       &endDate,
    CategoryIDs:   []uuid.UUID{categoryID},
    TagIDs:        []uuid.UUID{tag1, tag2},
    MinDuration:   intPtr(30),
    MaxDuration:   intPtr(120),
    IsEdited:      boolPtr(true),
    SortBy:        "checkin_time",
    SortDirection: "DESC",
    Limit:         20,
    Offset:        0,
}
```

### Faceted Search

Get aggregated counts for filtering:

```go
facets, err := searchRepo.GetFacets(ctx, userID, filters)

// Returns:
// - Categories with document counts
// - Tags with document counts
// - Duration buckets (0-15, 15-30, 30-60, etc.)
// - Date range histograms (Today, Yesterday, Last 7 days, etc.)
```

### Search Templates

Pre-configured search templates for common use cases:

```go
// Today's check-ins
filters := ApplyTemplate("Today", nil)

// This week with custom query
filters := ApplyTemplate("This Week", &SearchFilters{
    Query: "workout",
})

// Available templates:
// - Today
// - This Week
// - This Month
// - Edited Check-ins
// - Long Sessions
// - Quick Tasks
// - Recent
```

### Highlighting

Configurable text highlighting with Korean support:

```go
opts := HighlightOptions{
    PreTag:       "<mark>",
    PostTag:      "</mark>",
    MaxFragments: 1,
    MaxWords:     30,
    EnableKorean: true,
}

// Automatically applied in search results as "snippet"
```

### Caching

Redis-based caching for performance:

```go
// Automatically cached:
// - Search results (10 min TTL)
// - Facets (10 min TTL)

// Manual cache invalidation:
searchCache.InvalidateUserSearches(ctx, userID)
```

### Saved Searches (Premium)

Users with premium subscriptions can save search configurations:

```go
savedSearch, err := searchService.CreateSavedSearch(ctx, userID, CreateSavedSearchInput{
    Name:        "Weekly Workouts",
    Description: "All my workout sessions this week",
    Query:       "workout",
    Filters: map[string]interface{}{
        "start_date": startDate,
        "tag_names": []string{"fitness", "health"},
    },
    IsFavorite: true,
})

// Execute saved search
results, err := searchService.ExecuteSavedSearch(ctx, userID, savedSearch.ID)
```

## Indexing

### Real-time Indexing

The indexer automatically processes check-in events:

```go
// In checkin service
searchEventHandler.OnCreate(ctx, checkinID, userID, checkinData)
searchEventHandler.OnUpdate(ctx, checkinID, userID, checkinData)
searchEventHandler.OnDelete(ctx, checkinID, userID)
```

### Initial Bulk Load

On application startup, the indexer performs initial bulk loading:

```go
indexer := search.NewIndexer(repo, hub, config, logger)
err := indexer.Start(ctx)  // Performs initial load automatically
```

### Manual Reindexing

Use the reindex command for maintenance:

```bash
# Full reindex
./bin/reindex

# Reindex specific user
./bin/reindex --user-id=<uuid>

# Reindex date range
./bin/reindex --start-date=2024-01-01T00:00:00Z --end-date=2024-12-31T23:59:59Z

# Verify index integrity
./bin/reindex --verify

# Dry run
./bin/reindex --dry-run --verbose

# With compaction
./bin/reindex --compact

# Custom batch size
./bin/reindex --batch-size=500
```

### Zero-Downtime Reindexing

The system supports reindexing without downtime:

1. New events continue to be indexed in real-time
2. Batch processing happens in background
3. PostgreSQL triggers maintain consistency
4. No locking or blocking of search queries

## Performance

### Optimization Strategies

1. **Indexing**
   - GIN index on `search_vector` column
   - Composite indexes on frequently filtered columns
   - Trigger-based automatic tsvector updates

2. **Caching**
   - Redis caching with 10-minute TTL
   - MD5-based cache keys
   - Separate caching for results and facets

3. **Query Optimization**
   - Parameterized queries prevent SQL injection
   - Efficient JOIN strategies
   - Pagination with LIMIT/OFFSET
   - Selective column fetching

4. **Faceting**
   - Bucketed duration aggregations
   - Pre-computed date ranges
   - Limited result sets (top 20)

### Benchmarks

Run performance benchmarks:

```bash
# All benchmarks
go test -bench=. -benchmem ./internal/search/

# Specific benchmark
go test -bench=BenchmarkSearch_SimpleQuery -benchmem ./internal/search/

# Concurrent load test
go test -run=TestLoadTest_Medium ./internal/search/
```

Expected performance:
- Simple queries: <50ms (p95)
- Complex queries: <200ms (p95)
- Facet computation: <100ms (p95)
- Cache hit latency: <5ms (p95)
- Throughput: 100+ req/s per core

## Monitoring

### Prometheus Metrics

Available metrics:

```
# Search operations
search_duration_seconds{operation="search"}
search_result_count{operation="search"}
search_errors_total{operation="search",error_type="query_error"}

# Cache performance
search_cache_hits_total
search_cache_misses_total
search_cache_errors_total

# Indexer health
search_index_operations_total{operation_type="create|update|delete",status="success|failure"}
search_index_operation_duration_seconds{operation_type="create|update|delete"}
search_index_queue_size
search_index_retry_queue_size
search_index_batch_size

# Query analysis
search_query_complexity{has_fulltext="true|false",has_filters="true|false"}
search_facet_compute_duration_seconds
```

### Health Checks

Search system health is exposed via health endpoints:

```bash
# Overall health
curl http://localhost:8080/health

# Detailed health with search indexer status
curl http://localhost:8080/health/detail
```

Health checker verifies:
- Indexer is running
- Operation queue not backed up (<90% full)
- Recent indexing activity (within 5 minutes)

## Korean Language Support

### Text Search Configuration

The system uses PostgreSQL's `simple` configuration for Korean support:

```sql
-- Automatic tsvector generation (via trigger)
to_tsvector('simple', content)

-- Query processing
to_tsquery('simple', sanitized_query)
```

### Query Sanitization

Korean-aware query sanitization:

```go
// Preserves Korean characters
// Removes special characters
// No aggressive stemming
sanitized := SanitizeQueryForKorean("운동 했어요!")
// Result: "운동 & 했어요"
```

### Highlighting

Korean text highlighting with proper character boundary detection:

```sql
ts_headline('simple', content, query,
    'MaxWords=30, MinWords=10, StartSel=<mark>, StopSel=</mark>')
```

## API Examples

### Basic Search

```bash
POST /api/v1/search
{
  "query": "운동",
  "limit": 20,
  "offset": 0
}
```

### Advanced Search with Filters

```bash
POST /api/v1/search
{
  "query": "workout",
  "start_date": "2024-01-01T00:00:00Z",
  "end_date": "2024-12-31T23:59:59Z",
  "category_ids": ["uuid1", "uuid2"],
  "tag_names": ["fitness", "health"],
  "min_duration": 30,
  "max_duration": 120,
  "is_edited": false,
  "sort_by": "checkin_time",
  "sort_direction": "DESC",
  "limit": 20,
  "offset": 0
}
```

### Get Facets

```bash
GET /api/v1/search/facets?query=workout
```

### Search Suggestions

```bash
GET /api/v1/search/suggestions?prefix=work&limit=10
```

### Search History

```bash
GET /api/v1/search/history?limit=20
```

### Create Saved Search (Premium)

```bash
POST /api/v1/search/saved
{
  "name": "Weekly Workouts",
  "description": "My workout sessions",
  "query": "workout",
  "filters": {
    "tag_names": ["fitness"]
  },
  "is_favorite": true
}
```

### Execute Saved Search

```bash
GET /api/v1/search/saved/{id}/execute
```

## Error Handling

### Common Errors

1. **Invalid Query Syntax**
   ```json
   {
     "error": "Invalid search query",
     "details": "Special characters not allowed"
   }
   ```

2. **Cache Errors**
   - System automatically falls back to database
   - Logged as warnings, not failures
   - Metrics track cache error rate

3. **Index Lag**
   - Recent changes may not appear immediately
   - Typical lag: <5 seconds (p95)
   - Can be monitored via metrics

4. **Premium Feature Access**
   ```json
   {
     "error": "Premium subscription required for saved searches"
   }
   ```

## Troubleshooting

### Search Returns No Results

1. Check if index is healthy:
   ```bash
   ./bin/reindex --verify
   ```

2. Check for recent indexing:
   ```bash
   curl http://localhost:8080/health/detail
   ```

3. Verify search_vector column:
   ```sql
   SELECT id, content, search_vector
   FROM checkins
   WHERE search_vector IS NULL
   LIMIT 10;
   ```

### Slow Search Performance

1. Check query complexity:
   - Review Prometheus `search_query_complexity` metric
   - Simplify filters if possible

2. Verify indexes:
   ```sql
   SELECT indexname, indexdef
   FROM pg_indexes
   WHERE tablename = 'checkins';
   ```

3. Check cache hit rate:
   - Review `search_cache_hits_total` / `search_cache_misses_total`
   - Low hit rate may indicate cache configuration issues

4. Review database query plan:
   ```sql
   EXPLAIN ANALYZE
   SELECT * FROM checkins
   WHERE search_vector @@ to_tsquery('simple', 'query')
   AND user_id = 'uuid'
   AND deleted_at IS NULL;
   ```

### High Index Queue Size

1. Check `search_index_queue_size` metric
2. If consistently high:
   - Increase worker count in IndexerConfig
   - Increase batch size
   - Check for database bottlenecks

3. Review error logs for failed indexing operations

### Cache Issues

1. Verify Redis connection:
   ```bash
   redis-cli PING
   ```

2. Check cache size:
   ```bash
   redis-cli INFO memory
   ```

3. Monitor cache metrics in Prometheus

## Security

### SQL Injection Prevention

All queries use parameterized statements:

```go
// Safe - uses parameterized query
qb := NewQueryBuilder("SELECT * FROM checkins")
qb.AddFullTextSearch(userQuery)
query, args := qb.Build()
db.Query(query, args...)

// Never concatenate user input directly
```

### Input Validation

All user inputs are validated:

```go
// Query sanitization
sanitized := SanitizeQueryForKorean(userQuery)

// Filter validation
ValidateFilters(&filters)

// Limit enforcement
filters.Limit = ValidateLimit(filters.Limit)  // Max 100
filters.Offset = ValidateOffset(filters.Offset)
```

### Permission Checks

Premium features require subscription check:

```go
// Saved searches require premium tier
if err := checkPremiumAccess(ctx, userID); err != nil {
    return nil, fmt.Errorf("premium subscription required")
}
```

## Maintenance

### Regular Tasks

1. **Index Verification** (Weekly)
   ```bash
   ./bin/reindex --verify
   ```

2. **Cache Monitoring** (Daily)
   - Check hit rate in Prometheus
   - Review cache size in Redis
   - Adjust TTL if needed

3. **Performance Review** (Monthly)
   - Review p95 latency metrics
   - Analyze slow query logs
   - Optimize frequently used queries

4. **Index Compaction** (Monthly)
   ```bash
   ./bin/reindex --compact
   ```

### Backup Considerations

1. **Database Backups**
   - Include checkins table with search_vector
   - Triggers are part of schema backup

2. **Redis Backups** (Optional)
   - Cache is ephemeral, can be rebuilt
   - Consider RDB snapshots for faster recovery

3. **Saved Searches**
   - Backed up with main database
   - Part of saved_searches table

## Future Enhancements

### Planned Features

1. **Typo Tolerance**
   - pg_trgm extension integration
   - Fuzzy matching for misspelled queries

2. **Synonym Support**
   - Custom dictionaries for Korean/English
   - Synonym expansion in queries

3. **Advanced Analytics**
   - Popular search terms
   - Search success metrics
   - Query refinement suggestions

4. **Multi-language Support**
   - Language detection
   - Per-language configurations
   - Mixed-language queries

5. **AI-Powered Search**
   - Semantic search using embeddings
   - Natural language query understanding
   - Contextual search results

## References

- [PostgreSQL Full-Text Search](https://www.postgresql.org/docs/current/textsearch.html)
- [Redis Caching Best Practices](https://redis.io/docs/manual/patterns/)
- [Prometheus Metrics](https://prometheus.io/docs/concepts/metric_types/)
