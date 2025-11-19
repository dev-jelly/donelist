# Advanced Search and Filtering System

This package implements a comprehensive PostgreSQL full-text search and filtering system for the Donelist API.

## Features

### 1. Full-Text Search
- **PostgreSQL tsvector/tsquery**: Native PostgreSQL full-text search using `tsvector` and `tsquery`
- **Weighted Search**: Content (A), Category (B), Tags (C) weighted for relevance ranking
- **Search Ranking**: Results ranked by relevance using `ts_rank`
- **Text Snippets**: Highlighted search results using `ts_headline`
- **Auto-updating**: Triggers automatically maintain search vectors when data changes

### 2. Multi-Field Filtering
- **Date Range**: Filter by checkin time (start_date, end_date)
- **Categories**: Filter by one or more category IDs
- **Tags**: Filter by tag IDs or names (AND logic - all tags must match)
- **Duration**: Filter by duration range (min_duration, max_duration)
- **Edit Status**: Filter by edited/unedited checkins
- **Sorting**: Sort by multiple fields with ASC/DESC direction
- **Pagination**: Efficient offset-based pagination with configurable limits

### 3. Faceted Search
- **Category Facets**: Aggregate count of checkins by category
- **Tag Facets**: Aggregate count of checkins by tag
- **Duration Facets**: Distribution of checkins by duration
- **Date Range Facets**: Temporal distribution of checkins

### 4. Search Suggestions
- **Auto-complete**: Prefix-based suggestions for tags and categories
- **Usage-based**: Suggestions based on user's actual data
- **Multi-type**: Returns suggestions from tags and categories

### 5. Search History
- **Automatic Tracking**: All searches automatically saved to history
- **Query Storage**: Stores query string and filter parameters
- **Result Counts**: Tracks number of results for each search
- **Recent History**: Retrieve user's recent searches

### 6. Saved Searches (Premium Feature)
- **Save Configurations**: Save frequently used search configurations
- **Named Searches**: User-friendly names and descriptions
- **Favorites**: Mark important searches as favorites
- **Usage Tracking**: Track how often each saved search is used
- **Quick Execute**: Execute saved searches with a single API call

## Architecture

### Database Layer

#### Tables
- **search_history**: Tracks user search queries
- **saved_searches**: Stores saved search configurations (Premium)

#### Indexes
- **GIN Indexes**: Fast full-text search on tsvector columns
- **Composite Indexes**: Optimized for common filter combinations
- **Partial Indexes**: Efficient filtering of deleted records

#### Triggers
- **Auto-update Search Vectors**: Automatically maintain tsvector columns
- **Tag Changes**: Update checkin search vectors when tags change
- **Timestamp Updates**: Maintain updated_at timestamps

### Application Layer

#### Components
1. **Models** (`models.go`): Data structures and types
2. **Query Builder** (`query_builder.go`): Secure SQL query construction
3. **Repository** (`repository.go`): Database operations
4. **Service** (`service.go`): Business logic
5. **Handler** (`handler.go`): HTTP endpoints

## API Endpoints

### Search
```
POST /api/v1/search
```

**Request Body:**
```json
{
  "query": "database optimization",
  "start_date": "2024-01-01T00:00:00Z",
  "end_date": "2024-12-31T23:59:59Z",
  "category_ids": ["uuid1", "uuid2"],
  "tag_ids": ["uuid3", "uuid4"],
  "tag_names": ["golang", "backend"],
  "min_duration": 30,
  "max_duration": 120,
  "is_edited": false,
  "sort_by": "relevance",
  "sort_direction": "desc",
  "limit": 20,
  "offset": 0
}
```

**Response:**
```json
{
  "results": [
    {
      "id": "uuid",
      "user_id": "uuid",
      "category_id": "uuid",
      "category_name": "Work",
      "content": "Working on database optimization...",
      "checkin_time": "2024-11-13T10:00:00Z",
      "duration_minutes": 45,
      "is_edited": false,
      "edit_count": 0,
      "created_at": "2024-11-13T10:00:00Z",
      "updated_at": "2024-11-13T10:00:00Z",
      "rank": 0.85,
      "snippet": "Working on <mark>database</mark> <mark>optimization</mark>...",
      "tags": ["golang", "backend"]
    }
  ],
  "total": 42,
  "limit": 20,
  "offset": 0,
  "facets": {
    "categories": [
      {"id": "uuid", "name": "Work", "color": "#FF5733", "count": 25}
    ],
    "tags": [
      {"id": "uuid", "name": "golang", "count": 15}
    ],
    "durations": [
      {"duration": 15, "count": 5},
      {"duration": 30, "count": 10}
    ]
  },
  "took_ms": 45
}
```

### Get Suggestions
```
GET /api/v1/search/suggestions?q=gola&limit=10
```

**Response:**
```json
{
  "suggestions": [
    {"type": "tag", "value": "golang"},
    {"type": "tag", "value": "golang-backend"},
    {"type": "category", "value": "Goals"}
  ]
}
```

### Get Search History
```
GET /api/v1/search/history?limit=20
```

**Response:**
```json
{
  "history": [
    {
      "id": "uuid",
      "user_id": "uuid",
      "query": "database optimization",
      "filters": {"category_ids": ["uuid"]},
      "result_count": 5,
      "created_at": "2024-11-13T10:00:00Z"
    }
  ]
}
```

### Saved Searches (Premium)

#### Create Saved Search
```
POST /api/v1/search/saved
```

**Request Body:**
```json
{
  "name": "My Work Tasks",
  "description": "All work-related tasks from this quarter",
  "query": "project tasks",
  "filters": {
    "category_ids": ["work-uuid"],
    "start_date": "2024-01-01T00:00:00Z"
  },
  "is_favorite": true
}
```

#### List Saved Searches
```
GET /api/v1/search/saved
```

#### Get Saved Search
```
GET /api/v1/search/saved/:id
```

#### Update Saved Search
```
PATCH /api/v1/search/saved/:id
```

#### Delete Saved Search
```
DELETE /api/v1/search/saved/:id
```

#### Execute Saved Search
```
POST /api/v1/search/saved/:id/execute
```

## Security

### SQL Injection Prevention
- **Parameterized Queries**: All user input passed as parameters
- **Query Sanitization**: Special characters escaped in search queries
- **Whitelist Validation**: Sort fields validated against whitelist
- **Input Validation**: All inputs validated before use

### Query Building
```go
// Safe query building with parameters
qb := NewQueryBuilder(baseQuery)
qb.AddFullTextSearch(userQuery)  // Automatically sanitized
qb.AddUserFilter(userID)         // Parameterized
qb.AddDateRange(start, end)      // Parameterized
query, args := qb.Build()        // Returns safe query with args
```

### Authorization
- All endpoints require authentication
- User isolation enforced at database level
- Premium features check subscription tier

## Performance

### Optimization Strategies

1. **GIN Indexes**: Fast full-text search (sub-millisecond for most queries)
2. **Composite Indexes**: Optimized for common filter combinations
3. **Partial Indexes**: Only index active (non-deleted) records
4. **Query Planning**: PostgreSQL statistics maintained with ANALYZE

### Benchmarks
- Full-text search: ~10-50ms for 100K+ records
- Filtered search: ~5-20ms for common filters
- Faceted aggregations: ~20-100ms depending on data size

### Monitoring
- Query timing tracked and logged
- Slow query logging enabled for queries >100ms
- Search history provides usage analytics

## Testing

### Test Coverage
- Repository tests with real database
- Full-text search accuracy tests
- SQL injection prevention tests
- Filter combination tests
- Pagination tests
- Premium feature authorization tests

### Running Tests
```bash
go test ./internal/search/... -v
```

### Integration Tests
```bash
go test ./tests/integration/search_test.go -v
```

## Migration

### Running Migrations
```bash
# Run all migrations
./migrate up

# Specific search migrations
./migrate up 9   # Full-text search infrastructure
./migrate up 10  # GIN indexes
./migrate up 11  # Search history and saved searches
```

### Rollback
```bash
# Rollback specific migration
./migrate down 11
```

## Usage Examples

### Basic Search
```go
filters := search.SearchFilters{
    Query: "database optimization",
    Limit: 20,
}

results, err := searchService.Search(ctx, userID, filters)
```

### Advanced Filtering
```go
filters := search.SearchFilters{
    Query:       "project work",
    CategoryIDs: []uuid.UUID{categoryID},
    TagNames:    []string{"golang", "backend"},
    StartDate:   &startDate,
    EndDate:     &endDate,
    MinDuration: intPtr(30),
    SortBy:      "relevance",
    Limit:       50,
}

results, err := searchService.Search(ctx, userID, filters)
```

### Saved Search (Premium)
```go
// Create
savedSearch, err := searchService.CreateSavedSearch(ctx, userID, input)

// Execute
results, err := searchService.ExecuteSavedSearch(ctx, userID, savedSearch.ID)
```

## Best Practices

### Query Optimization
1. Use specific queries over broad searches
2. Combine filters to narrow results
3. Use pagination for large result sets
4. Leverage facets for drill-down filtering

### Premium Features
1. Check user tier before saved search operations
2. Provide upgrade prompts for free users
3. Track usage metrics for saved searches

### Error Handling
1. Handle empty results gracefully
2. Validate user input before search
3. Log search errors for monitoring
4. Return user-friendly error messages

## Future Enhancements

### Potential Improvements
1. **Fuzzy Search**: Handle typos and similar words
2. **Synonym Support**: Search for related terms
3. **Relevance Tuning**: User-specific relevance adjustments
4. **Search Analytics**: Detailed search behavior insights
5. **Export Results**: Export search results to CSV/JSON
6. **Scheduled Searches**: Email/notify users of new matches
7. **Collaborative Searches**: Share searches between team members

## Troubleshooting

### Common Issues

**Slow Searches**
- Check EXPLAIN ANALYZE output
- Verify indexes are being used
- Update table statistics with ANALYZE

**Inaccurate Results**
- Review search vector weights
- Check trigger functions are working
- Verify data is properly indexed

**Missing Results**
- Check deleted_at filter
- Verify user_id isolation
- Review filter combinations

### Debug Mode
```go
// Enable SQL query logging
logger.Info("Search query", zap.String("sql", query), zap.Any("args", args))
```

## Support

For issues or questions:
1. Check logs for error messages
2. Review query execution plans
3. Verify database indexes exist
4. Check user permissions and tier

## License

Internal use only - Part of Donelist API.
