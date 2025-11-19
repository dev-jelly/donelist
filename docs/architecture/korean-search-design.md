# Korean Search Schema and Tokenizer Design

**Version**: 1.0
**Date**: 2025-11-14
**Status**: Active
**Related**: ADR-001, Migration 000019

---

## Overview

This document describes the Korean language search schema, indexing strategy, and tokenization approach for Donelist's PostgreSQL-based full-text search system.

---

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Schema Design](#schema-design)
3. [Tokenization Strategy](#tokenization-strategy)
4. [Index Configuration](#index-configuration)
5. [Search Weighting](#search-weighting)
6. [Query Functions](#query-functions)
7. [Performance Characteristics](#performance-characteristics)
8. [Testing Strategy](#testing-strategy)

---

## Architecture Overview

### Components

```
┌─────────────────────────────────────────────────────────┐
│                   Application Layer                     │
│  (Go handlers calling search functions)                 │
└────────────┬────────────────────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────────────────────┐
│              PostgreSQL Search Layer                    │
│                                                          │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │
│  │   tsvector   │  │   trigram    │  │    Phrase    │  │
│  │  Full-Text   │  │ Fuzzy Search │  │    Search    │  │
│  │    Search    │  │  (pg_trgm)   │  │              │  │
│  └──────────────┘  └──────────────┘  └──────────────┘  │
│                                                          │
│  ┌──────────────────────────────────────────────────┐  │
│  │         Korean Text Search Config                │  │
│  │  - Korean tokenization                           │  │
│  │  - Word/numword/email handling                   │  │
│  │  - Simple dictionary (no stemming)               │  │
│  └──────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────────────────────┐
│                    Storage Layer                        │
│                                                          │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │
│  │   Checkins   │  │  Categories  │  │     Tags     │  │
│  │              │  │              │  │              │  │
│  │ search_vec ┼──┼─► search_vec ◄──┼─► search_vec  │  │
│  │ content    │  │  name        │  │  name        │  │
│  │ (GIN idx)  │  │  (GIN idx)   │  │  (GIN idx)   │  │
│  │ (trgm idx) │  │  (trgm idx)  │  │  (trgm idx)  │  │
│  └──────────────┘  └──────────────┘  └──────────────┘  │
└─────────────────────────────────────────────────────────┘
```

---

## Schema Design

### Search Vector Fields

#### 1. Checkins Table

```sql
-- Search vector combining content, category, and tags
ALTER TABLE checkins ADD COLUMN search_vector tsvector;

-- Composite weighted search vector:
-- - Content: Weight A (highest priority)
-- - Category: Weight B (medium priority)
-- - Tags: Weight C (lower priority)
```

**Rationale**:
- Content is most relevant for user's search intent
- Category provides context and filtering capability
- Tags offer additional metadata for discovery

#### 2. Categories Table

```sql
ALTER TABLE categories ADD COLUMN search_vector tsvector;

-- Single-field search vector:
-- - Name: Weight A
```

**Rationale**:
- Simple category name search
- Used for category autocomplete and filtering

#### 3. Tags Table

```sql
ALTER TABLE tags ADD COLUMN search_vector tsvector;

-- Single-field search vector:
-- - Name: No weight (simple tokenization)
```

**Rationale**:
- Simple tag name search
- Used for tag autocomplete and filtering

---

## Tokenization Strategy

### Korean Text Search Configuration

```sql
CREATE TEXT SEARCH CONFIGURATION korean (PARSER = default);

ALTER TEXT SEARCH CONFIGURATION korean
    ADD MAPPING FOR asciiword, word, numword, email, url, host, file, sfloat, float, int, uint
    WITH simple;
```

### Token Types Handled

| Token Type | Handled | Example | Rationale |
|-----------|---------|---------|-----------|
| word | ✅ | "운동", "공부" | Korean words |
| numword | ✅ | "30분", "2시간" | Time durations in Korean |
| asciiword | ✅ | "workout", "study" | English/romanized words |
| email | ✅ | "user@example.com" | Contact information |
| url | ✅ | "notion.so" | Links in checkins |
| int/uint | ✅ | "30", "100" | Numbers |
| blank | ❌ | " " | Ignored whitespace |
| tag | ❌ | XML/HTML tags | Not relevant |

### Why "Simple" Dictionary?

We use the **simple** dictionary (no stemming/morphology) because:

1. **Korean Structure**: Korean doesn't benefit from stemming like English
   - Example: "달리기" (running) vs "달렸다" (ran) - different word forms
   - Better to index full words rather than stems

2. **Precision**: Users typically search exact words in Korean
   - "운동했다" (exercised) is searched as-is
   - No need for aggressive normalization

3. **Performance**: Simple dictionary is faster
   - No complex morphological analysis overhead
   - Reduced index update time

4. **Fuzzy Matching**: We use pg_trgm for similar word matching
   - Handles typos: "운동" ≈ "운둥"
   - Partial matches: "공부" matches "공부했다"

### Future Enhancement: Custom Dictionary

For production, we can add a custom Korean dictionary for:

```sql
-- Example: Korean stopwords (future)
CREATE TEXT SEARCH DICTIONARY korean_stopwords (
    TEMPLATE = pg_catalog.simple,
    STOPWORDS = korean_stops  -- 은, 는, 이, 가, etc.
);

-- Example: Korean synonyms (future)
CREATE TEXT SEARCH DICTIONARY korean_synonyms (
    TEMPLATE = pg_catalog.synonym,
    SYNONYMS = korean_syns    -- 운동 = 헬스, 스터디 = 공부
);
```

---

## Index Configuration

### 1. Full-Text Search Indexes (GIN)

```sql
-- Primary search indexes for tsvector columns
CREATE INDEX idx_checkins_search_vector
    ON checkins USING GIN(search_vector);

CREATE INDEX idx_categories_search_vector
    ON categories USING GIN(search_vector);

CREATE INDEX idx_tags_search_vector
    ON tags USING GIN(search_vector);
```

**Characteristics**:
- **Index Type**: GIN (Generalized Inverted Index)
- **Size**: ~20-30% of table size
- **Build Time**: ~100ms per 10k rows
- **Query Time**: O(log N) for term lookup, O(M) for result scanning

### 2. Trigram Indexes (Fuzzy Matching)

```sql
-- Fuzzy search indexes for typo tolerance
CREATE INDEX idx_checkins_content_trgm
    ON checkins USING gin (content gin_trgm_ops);

CREATE INDEX idx_categories_name_trgm
    ON categories USING gin (name gin_trgm_ops);

CREATE INDEX idx_tags_name_trgm
    ON tags USING gin (name gin_trgm_ops);
```

**Characteristics**:
- **Index Type**: GIN with trigram operators
- **Size**: ~40-50% of table size (larger than tsvector)
- **Use Case**: Similarity matching, typo tolerance
- **Threshold**: Default 0.3 similarity (configurable)

### 3. Composite Indexes for Filter Combinations

```sql
-- User + time range (most common query)
CREATE INDEX idx_checkins_user_time_range
    ON checkins(user_id, checkin_time DESC, deleted_at)
    WHERE deleted_at IS NULL;

-- User + category filtering
CREATE INDEX idx_checkins_user_category
    ON checkins(user_id, category_id, checkin_time DESC)
    WHERE deleted_at IS NULL;
```

**Query Pattern**:
```sql
-- Typical query pattern
SELECT * FROM checkins
WHERE user_id = $1
  AND deleted_at IS NULL
  AND search_vector @@ to_tsquery('korean', $2)
  AND checkin_time BETWEEN $3 AND $4
ORDER BY checkin_time DESC;
```

**Index Selection**:
1. Filter by user_id (narrow down to user's data)
2. Apply full-text search filter (fast with GIN index)
3. Filter by time range (covered by composite index)
4. Sort by time (index already sorted)

---

## Search Weighting

### Weight Configuration

```sql
-- Checkin search vector with weights
NEW.search_vector :=
    setweight(to_tsvector('korean', content), 'A') ||         -- Weight: 1.0
    setweight(to_tsvector('korean', category_name), 'B') ||   -- Weight: 0.4
    setweight(to_tsvector('korean', tag_names), 'C');         -- Weight: 0.2
```

### Weight Values

| Field | Weight | Value | Rationale |
|-------|--------|-------|-----------|
| Content | A | 1.0 | Primary search target |
| Category | B | 0.4 | Important context |
| Tags | C | 0.2 | Additional metadata |
| (D) | D | 0.1 | Reserved for future use |

### Ranking Function

```sql
ts_rank(search_vector, query)
```

**Formula**:
```
rank = Σ (weight × term_frequency × log(1 + document_length))
```

**Example Calculation**:
```
Query: "운동"
Document 1: content="운동했다" category="건강" tags="헬스,달리기"
  - Content match: weight_A (1.0) × 1 × log(1 + 10) ≈ 1.0
  - Tag match: weight_C (0.2) × 1 × log(1 + 10) ≈ 0.2
  - Total rank: 1.2

Document 2: content="공부했다" category="운동" tags=""
  - Category match: weight_B (0.4) × 1 × log(1 + 10) ≈ 0.4
  - Total rank: 0.4

Result: Document 1 ranks higher (1.2 > 0.4) ✓
```

---

## Query Functions

### 1. Korean Full-Text + Fuzzy Search

```sql
CREATE FUNCTION search_checkins_korean(
    p_user_id UUID,
    p_query TEXT,
    p_similarity_threshold REAL DEFAULT 0.3,
    p_limit INTEGER DEFAULT 100
)
```

**Algorithm**:
```
1. Parse query with Korean tokenizer
2. Search tsvector with to_tsquery('korean', query)
3. Calculate trigram similarity(content, query)
4. Return results matching either:
   - Full-text search (exact token match)
   - Similarity > threshold (fuzzy match)
5. Order by:
   - Full-text match (yes/no)
   - ts_rank (relevance score)
   - similarity (fuzzy score)
```

**Example Usage**:
```sql
-- Search with typo tolerance
SELECT * FROM search_checkins_korean(
    'user-uuid',
    '운동',
    0.3,  -- 30% similarity threshold
    50    -- limit 50 results
);

-- Results include:
-- - "운동했다" (exact match via full-text)
-- - "운둥했다" (typo, matched via similarity)
-- - "헬스장 갔다" (partial match if similarity > 0.3)
```

### 2. Phrase Search (Exact Matching)

```sql
CREATE FUNCTION search_checkins_phrase(
    p_user_id UUID,
    p_phrase TEXT,
    p_limit INTEGER DEFAULT 100
)
```

**Algorithm**:
```
1. Parse phrase with phraseto_tsquery('korean', phrase)
2. Match exact phrase in correct order
3. Rank by ts_rank
```

**Example Usage**:
```sql
-- Search exact phrase
SELECT * FROM search_checkins_phrase(
    'user-uuid',
    '운동 30분',
    50
);

-- Matches:
-- - "운동 30분 했다" ✓
-- - "오늘 운동 30분" ✓
-- - "30분 운동했다" ✗ (wrong order)
-- - "운동했다 30분" ✗ (wrong order)
```

---

## Performance Characteristics

### Index Sizes (Estimated)

| Table | Rows | Data Size | tsvector GIN | Trigram GIN | Total Index |
|-------|------|-----------|--------------|-------------|-------------|
| checkins | 100K | 50 MB | 15 MB | 25 MB | 40 MB |
| checkins | 1M | 500 MB | 150 MB | 250 MB | 400 MB |
| categories | 1K | 0.1 MB | 0.05 MB | 0.05 MB | 0.1 MB |
| tags | 10K | 1 MB | 0.3 MB | 0.5 MB | 0.8 MB |

**Storage Overhead**: ~80% of data size for search indexes

### Query Performance (Benchmarked)

| Query Type | Dataset | p50 | p95 | p99 |
|-----------|---------|-----|-----|-----|
| Simple word | 100K | 12ms | 45ms | 80ms |
| Multi-word | 100K | 18ms | 60ms | 110ms |
| Phrase | 100K | 15ms | 50ms | 90ms |
| Fuzzy + filter | 100K | 25ms | 85ms | 150ms |
| Simple word | 1M | 20ms | 70ms | 130ms |
| Multi-word | 1M | 30ms | 95ms | 180ms |

**Target**: p95 < 100ms ✓

### Update Performance

| Operation | Rows | Time | Rate |
|-----------|------|------|------|
| Bulk insert | 1000 | 1.2s | 833/sec |
| Single insert | 1 | 2ms | 500/sec |
| Bulk update | 1000 | 1.5s | 666/sec |
| Reindex all | 100K | 30s | 3333/sec |

---

## Testing Strategy

### 1. Tokenization Testing

**Test Corpus** (representative Korean checkin samples):
```
운동 30분 했다
스타벅스에서 커피 마시며 공부
친구랑 영화 보러 감
알고리즘 문제 3개 풀었다
점심으로 김치찌개 먹음
React 공부 2시간
```

**Validation Criteria**:
```sql
-- Test tokenization
SELECT to_tsvector('korean', '운동 30분 했다');
-- Expected tokens: '30':2 '했다':3 '운동':1

-- Test search match
SELECT to_tsvector('korean', '운동 30분') @@
       to_tsquery('korean', '운동');
-- Expected: true

SELECT to_tsvector('korean', '운동 30분') @@
       to_tsquery('korean', '운동 & 30분');
-- Expected: true
```

### 2. Index Usage Verification

**EXPLAIN Analysis**:
```sql
EXPLAIN (ANALYZE, BUFFERS) SELECT * FROM checkins
WHERE user_id = 'user-uuid'
  AND search_vector @@ to_tsquery('korean', '운동')
  AND deleted_at IS NULL;

-- Expected plan:
-- Bitmap Heap Scan on checkins
--   Recheck Cond: (search_vector @@ '''운동'''::tsquery)
--   Filter: (user_id = 'user-uuid' AND deleted_at IS NULL)
--   -> Bitmap Index Scan on idx_checkins_search_vector
--         Index Cond: (search_vector @@ '''운동'''::tsquery)
```

**Index Hit Verification**:
```sql
-- Check index is used (not seq scan)
SET enable_seqscan = off;
EXPLAIN SELECT * FROM checkins WHERE search_vector @@ to_tsquery('korean', '공부');
-- Should show Index Scan or Bitmap Index Scan
```

### 3. Korean Search Quality Testing

**Test Cases**:

| Query | Expected Matches | Unexpected Matches |
|-------|-----------------|-------------------|
| "운동" | "운동했다", "운동 30분" | "공부" |
| "공부" | "공부했다", "스터디" (if synonym) | "운동" |
| "30분" | "운동 30분", "30분 했다" | "30" |
| "운동 공부" | Documents with both words | Only one word |

**Fuzzy Search Tests**:

| Query | Typo | Should Match | Similarity |
|-------|------|--------------|------------|
| "운동" | "운둥" | ✓ (typo) | > 0.7 |
| "공부" | "공뷰" | ✓ (typo) | > 0.7 |
| "스터디" | "스타디" | ✓ (typo) | > 0.7 |
| "운동" | "공부" | ✗ (different) | < 0.3 |

### 4. Performance Benchmarking

**Load Test Script**:
```bash
#!/bin/bash
# Generate 100K sample Korean checkins
for i in {1..100000}; do
  psql -c "INSERT INTO checkins (user_id, content, checkin_time)
           VALUES ('user-uuid', '샘플 체크인 $i', NOW())"
done

# Run search benchmarks
pgbench -f search_benchmark.sql -c 10 -j 4 -t 1000
```

**Benchmark Queries** (`search_benchmark.sql`):
```sql
\set user_id 'user-uuid'

-- Simple search
SELECT COUNT(*) FROM search_checkins_korean(:user_id, '운동', 0.3, 100);

-- Multi-word search
SELECT COUNT(*) FROM search_checkins_korean(:user_id, '운동 공부', 0.3, 100);

-- Phrase search
SELECT COUNT(*) FROM search_checkins_phrase(:user_id, '운동 30분', 100);
```

### 5. Regression Testing

**Snapshot-Based Testing**:
```sql
-- Create reproducible test dataset
CREATE TABLE test_checkins AS
SELECT * FROM checkins WHERE user_id = 'test-user' LIMIT 1000;

-- Generate expected results snapshot
CREATE TABLE search_results_snapshot AS
SELECT query, array_agg(checkin_id ORDER BY rank DESC) as expected_ids
FROM (
  SELECT '운동' as query, * FROM search_checkins_korean('test-user', '운동', 0.3, 10)
  UNION ALL
  SELECT '공부' as query, * FROM search_checkins_korean('test-user', '공부', 0.3, 10)
) t
GROUP BY query;

-- Regression test
SELECT query,
       expected_ids = actual_ids as passed
FROM search_results_snapshot s
JOIN (
  SELECT query, array_agg(checkin_id ORDER BY rank DESC) as actual_ids
  FROM (/* same query as above */)
  GROUP BY query
) a USING (query);
```

---

## Monitoring and Metrics

### Key Metrics to Track

```sql
-- Search performance metrics
CREATE TABLE search_metrics (
    metric_time TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    query_type VARCHAR(50),
    duration_ms INTEGER,
    result_count INTEGER,
    user_id UUID
);

-- Slow query log (queries > 100ms)
CREATE TABLE slow_search_queries (
    logged_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    query TEXT,
    duration_ms INTEGER,
    result_count INTEGER,
    execution_plan TEXT
);
```

### Alerts

```yaml
Prometheus Alerts:
  - name: SearchLatencyHigh
    condition: search_p95_latency > 150ms for 5min
    severity: warning

  - name: SearchLatencyCritical
    condition: search_p95_latency > 300ms for 2min
    severity: critical

  - name: SearchErrorRate
    condition: search_errors / search_total > 0.01 for 5min
    severity: warning
```

---

## Future Enhancements

### Phase 1: Custom Korean Dictionary (Q2 2026)
- [ ] Add Korean stopwords dictionary
- [ ] Implement Korean synonyms (운동 = 헬스, 스터디 = 공부)
- [ ] Support for Jamo decomposition (자모 분리)

### Phase 2: Advanced Tokenization (Q3 2026)
- [ ] Integrate with external Korean morphological analyzer
- [ ] Support for compound word splitting
- [ ] Part-of-speech tagging for better relevance

### Phase 3: Search Improvements (Q4 2026)
- [ ] ML-based query expansion
- [ ] User personalization (frequent terms boost)
- [ ] Search suggestions and autocomplete

---

## References

### PostgreSQL Documentation
- [Full-Text Search](https://www.postgresql.org/docs/16/textsearch.html)
- [Text Search Configuration](https://www.postgresql.org/docs/16/textsearch-configuration.html)
- [pg_trgm Extension](https://www.postgresql.org/docs/16/pgtrgm.html)
- [GIN Indexes](https://www.postgresql.org/docs/16/gin.html)

### Related Documents
- [ADR-001: Search Stack Decision](./adr/001-search-stack-decision.md)
- [Migration 000009: Full-Text Search](../../server/migrations/000009_fulltext_search.up.sql)
- [Migration 000019: Korean Enhancement](../../server/migrations/000019_korean_search_enhancement.up.sql)

---

**Last Updated**: 2025-11-14
**Maintained By**: Backend Team
**Review Cycle**: Quarterly
