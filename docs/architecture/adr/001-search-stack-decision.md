# ADR-001: Search Stack Selection - PostgreSQL FTS vs Elasticsearch

**Status**: Accepted
**Date**: 2025-11-14
**Decision Makers**: Development Team
**Tags**: #search #architecture #performance #korean-language

---

## Context and Problem Statement

Donelist requires a robust full-text search system for checkin records with the following requirements:

1. **Korean Language Support**: Effective morphological analysis and tokenization for Korean text
2. **Search Capabilities**:
   - Full-text search across content, categories, and tags
   - Search result highlighting
   - Faceted search (aggregations by category, tags, date ranges)
   - Complex filter combinations
3. **Performance Requirements**:
   - Sub-100ms search latency for p95
   - Handle growing dataset (thousands of checkins per user)
   - Support for saved searches and search history
4. **Operational Constraints**:
   - Small team with limited operational capacity
   - Self-hosted on K3s infrastructure
   - Cost-effective solution
   - Minimal maintenance overhead

We need to decide between **PostgreSQL Full-Text Search** and **Elasticsearch** as our search backend.

---

## Decision Drivers

### Critical Requirements
1. **Korean Language Processing**: Quality of Korean tokenization and morphological analysis
2. **Operational Complexity**: Ease of deployment, backup, monitoring, and maintenance
3. **Performance**: Search latency and indexing throughput
4. **Cost**: Infrastructure and operational costs
5. **Team Expertise**: Learning curve and existing team capabilities
6. **Feature Completeness**: Support for required search features
7. **Scalability**: Ability to handle growth
8. **Integration**: Compatibility with existing PostgreSQL-based stack

### Nice-to-Have Features
- Advanced relevance tuning
- Machine learning-based ranking
- Fuzzy matching and typo tolerance
- Real-time search suggestions

---

## Considered Options

### Option 1: PostgreSQL Full-Text Search (with pg_trgm)

**Architecture**:
```sql
-- Already implemented in migrations 000009-000011
- tsvector columns for full-text search
- GIN indexes for fast text search
- pg_trgm extension for fuzzy matching
- Custom triggers for automatic index updates
- Weighted search (content > category > tags)
```

**Pros**:
- ✅ **Single Database**: No additional infrastructure needed
- ✅ **ACID Transactions**: Consistent search results with application data
- ✅ **Zero Operational Overhead**: Same backup, monitoring, and maintenance as main DB
- ✅ **Already Implemented**: Migrations 000009-000011 provide full foundation
- ✅ **Team Expertise**: Team already proficient in PostgreSQL
- ✅ **Cost-Effective**: No additional licensing or infrastructure costs
- ✅ **Korean Support**: Can configure Korean-specific text search configurations
- ✅ **Simple Deployment**: Runs in existing PostgreSQL instance
- ✅ **Guaranteed Consistency**: Search indexes update in same transaction as data
- ✅ **Built-in Features**: Native support for highlighting, ranking, phrase search

**Cons**:
- ⚠️ **Limited Korean Analysis**: Not as sophisticated as Elasticsearch Korean plugins
- ⚠️ **Facet Performance**: Aggregations require additional queries/joins
- ⚠️ **Scalability Ceiling**: May hit performance limits at very large scale
- ⚠️ **Feature Gaps**: Less advanced features (no ML ranking, limited fuzzy matching)
- ⚠️ **Relevance Tuning**: More manual work compared to Elasticsearch

**Performance Characteristics** (estimated):
```
Search Latency (p50): 10-30ms
Search Latency (p95): 50-100ms
Indexing Throughput: ~1000 docs/sec
Storage Overhead: ~20-30% of original data
```

---

### Option 2: Elasticsearch

**Architecture**:
```yaml
# Separate Elasticsearch cluster
- 3-node cluster for HA
- Korean analyzer plugin (nori)
- Dual-write from application
- CDC pipeline for sync
```

**Pros**:
- ✅ **Superior Korean Analysis**: Nori plugin provides excellent Korean tokenization
- ✅ **Advanced Features**: ML ranking, fuzzy matching, suggestions, typo tolerance
- ✅ **Facet Performance**: Extremely fast aggregations
- ✅ **Relevance Tuning**: Sophisticated scoring and ranking options
- ✅ **Scalability**: Proven at massive scale
- ✅ **Rich Ecosystem**: Kibana for analytics, extensive plugins
- ✅ **Real-time Suggestions**: Built-in completion suggester

**Cons**:
- ❌ **Operational Complexity**: Separate cluster to deploy, monitor, backup
- ❌ **Consistency Challenges**: Eventual consistency, dual-write complexity
- ❌ **Infrastructure Cost**: Additional 3+ nodes for production HA setup
- ❌ **Learning Curve**: Team needs to learn Elasticsearch operations
- ❌ **Maintenance Overhead**: Version upgrades, cluster management, reindexing
- ❌ **Resource Usage**: Memory-hungry (recommend 8GB+ heap per node)
- ❌ **Data Synchronization**: Need CDC or dual-write mechanism
- ❌ **Increased Attack Surface**: More systems to secure and monitor
- ❌ **Backup Complexity**: Additional backup strategy needed

**Performance Characteristics** (estimated):
```
Search Latency (p50): 5-20ms
Search Latency (p95): 20-50ms
Indexing Throughput: ~5000 docs/sec
Storage Overhead: ~40-60% of original data
```

**Infrastructure Requirements**:
```yaml
Minimum for HA:
  - 3 ES nodes (8GB RAM each) = 24GB RAM
  - 3 volumes (100GB each) = 300GB storage
  - Additional monitoring (Prometheus, Grafana)
  - Backup storage

Estimated Monthly Cost (cloud):
  - ~$300-500/month for managed ES
  - ~$150-250/month for self-hosted on K3s
```

---

## Comparative Analysis

### 1. Korean Language Processing

| Feature | PostgreSQL FTS | Elasticsearch |
|---------|---------------|---------------|
| Korean Tokenization | Basic (configurable) | Excellent (Nori plugin) |
| Morphological Analysis | Limited | Advanced |
| Custom Dictionary | Possible but manual | Built-in with easy updates |
| Search Quality | Good for most cases | Superior for complex Korean |

**Analysis**: Elasticsearch has a clear advantage for Korean language processing. However, for Donelist's use case (short checkin messages, categories, tags), PostgreSQL's Korean support may be sufficient.

### 2. Feature Comparison

| Feature | PostgreSQL FTS | Elasticsearch |
|---------|---------------|---------------|
| Full-text Search | ✅ Native | ✅ Native |
| Highlighting | ✅ ts_headline | ✅ Built-in |
| Faceted Search | ⚠️ Requires joins | ✅ Aggregations |
| Fuzzy Matching | ⚠️ pg_trgm (limited) | ✅ Advanced |
| Phrase Search | ✅ Yes | ✅ Yes |
| Saved Searches | ✅ Application layer | ✅ Percolator |
| Search Suggestions | ⚠️ Manual | ✅ Completion API |
| Typo Tolerance | ⚠️ Limited | ✅ Excellent |
| ML Ranking | ❌ No | ✅ Yes |
| Custom Scoring | ⚠️ ts_rank | ✅ Function score |

### 3. Operational Complexity

| Aspect | PostgreSQL FTS | Elasticsearch |
|--------|---------------|---------------|
| Deployment | ✅ Already deployed | ❌ New cluster needed |
| Backup | ✅ Same as DB | ❌ Additional strategy |
| Monitoring | ✅ Existing tools | ❌ New tools needed |
| Scaling | ⚠️ Vertical scaling | ✅ Horizontal scaling |
| Upgrades | ✅ With DB upgrades | ❌ Separate process |
| Expertise Required | ✅ Team has it | ❌ New learning |
| MTTR (failures) | ✅ 30min | ⚠️ 2-4 hours |

### 4. Performance Benchmarks (Projected)

**Test Dataset**: 100,000 checkins with Korean content

| Metric | PostgreSQL FTS | Elasticsearch |
|--------|---------------|---------------|
| Simple Search (p50) | 15ms | 8ms |
| Simple Search (p95) | 60ms | 25ms |
| Complex Filter (p50) | 40ms | 15ms |
| Complex Filter (p95) | 120ms | 45ms |
| Facet Query (p50) | 80ms | 12ms |
| Indexing Rate | 800/sec | 3000/sec |
| Storage Size | 120% of data | 150% of data |

**Analysis**: Elasticsearch is 2-3x faster for most operations, but PostgreSQL FTS meets our <100ms p95 requirement.

### 5. Cost Analysis (Annual)

| Item | PostgreSQL FTS | Elasticsearch |
|------|---------------|---------------|
| Infrastructure | $0 (existing) | $2,400 - $6,000 |
| Operational Time | 2 hrs/month | 20 hrs/month |
| Monitoring Tools | $0 (existing) | $480 |
| Backup Storage | $0 (existing) | $240 |
| **Total Annual** | **~$0** | **$3,120 - $6,720** |

---

## Decision Outcome

**Chosen Option**: **PostgreSQL Full-Text Search**

### Rationale

1. **Current Implementation**: Migrations 000009-000011 already provide a solid foundation
2. **Operational Simplicity**: Zero additional infrastructure overhead aligns with small team constraints
3. **Performance Adequate**: Meets p95 <100ms requirement for current and projected scale
4. **Cost-Effective**: No additional infrastructure costs
5. **Team Expertise**: Team is already proficient in PostgreSQL
6. **Korean Support**: While not as advanced as Elasticsearch, sufficient for our use case
7. **Consistency Guarantees**: ACID transactions ensure search results always consistent with data
8. **Rapid Iteration**: Can implement and test features faster without dual-system complexity

### When to Reconsider

We will reconsider Elasticsearch if:
- Search queries consistently exceed 100ms p95 latency
- User feedback indicates poor Korean search quality
- Dataset grows beyond 1M checkins per user
- Advanced ML-based ranking becomes a competitive requirement
- Facet query performance becomes a bottleneck
- Real-time search suggestions become critical

### Migration Path

If we need to migrate to Elasticsearch later:
1. Current search abstraction layer makes this feasible
2. Estimated migration effort: 2-3 weeks
3. Can run hybrid for gradual rollout
4. Existing PostgreSQL search can serve as fallback

---

## Implementation Plan

### Phase 1: Enhance Current PostgreSQL FTS ✅ COMPLETED
- [x] Migration 000009: Full-text search infrastructure
- [x] Migration 000010: GIN indexes for performance
- [x] Migration 000011: Search history and saved searches

### Phase 2: Korean Language Optimization (Task 14.2)
- [ ] Add Korean-specific text search configuration
- [ ] Implement custom Korean dictionary
- [ ] Test Korean search quality
- [ ] Fine-tune weights and ranking

### Phase 3: Advanced Features (Tasks 14.3-14.6)
- [ ] Implement search query DSL
- [ ] Add result highlighting
- [ ] Build facet aggregation API
- [ ] Create saved/favorite search functionality
- [ ] Implement search result caching

### Phase 4: Performance Optimization (Tasks 14.7-14.9)
- [ ] Add query caching layer
- [ ] Implement keyset pagination
- [ ] Optimize composite queries
- [ ] Load testing and tuning
- [ ] Monitoring and alerting

---

## Consequences

### Positive

1. **Faster Time to Market**: Build on existing implementation
2. **Lower Risk**: No new infrastructure to deploy and maintain
3. **Simpler Operations**: Single database to manage
4. **Cost Savings**: ~$3-7k/year saved
5. **Better Consistency**: ACID guarantees for search results
6. **Team Velocity**: No learning curve for new technology

### Negative

1. **Korean Search Quality**: May not match Elasticsearch's sophistication
2. **Scaling Limits**: Will need to revisit at very large scale
3. **Feature Gaps**: Some advanced features harder to implement
4. **Vendor Lock-in**: More work to switch later (but mitigated by abstraction)

### Mitigations

1. **Korean Quality**: Invest in custom dictionary and configuration tuning
2. **Scaling**: Monitor performance metrics; plan migration trigger points
3. **Features**: Prioritize features with best ROI; defer advanced ML ranking
4. **Abstraction**: Maintain clean search service interface for future flexibility

---

## Metrics and Monitoring

### Success Metrics

| Metric | Target | Alert Threshold |
|--------|--------|-----------------|
| Search Latency (p50) | <30ms | >50ms |
| Search Latency (p95) | <100ms | >150ms |
| Search Latency (p99) | <200ms | >300ms |
| Facet Query (p95) | <150ms | >250ms |
| Index Update Lag | <1s | >5s |
| Search Error Rate | <0.1% | >1% |

### Monitoring Plan

```yaml
Prometheus Metrics:
  - search_query_duration_seconds (histogram)
  - search_results_count (histogram)
  - search_error_total (counter)
  - facet_query_duration_seconds (histogram)

Grafana Dashboards:
  - Search Performance Overview
  - Korean Search Quality (user feedback)
  - Index Health and Size
  - Query Patterns Analysis

Alerts:
  - P95 latency > 150ms for 5min
  - Error rate > 1% for 2min
  - Index update lag > 10s
```

---

## References

### Documentation
- [PostgreSQL Full-Text Search](https://www.postgresql.org/docs/16/textsearch.html)
- [PostgreSQL pg_trgm Extension](https://www.postgresql.org/docs/16/pgtrgm.html)
- [Elasticsearch Korean (Nori) Plugin](https://www.elastic.co/guide/en/elasticsearch/plugins/current/analysis-nori.html)

### Internal Documents
- Migration 000009: Full-text search infrastructure
- Migration 000010: Search indexes
- Migration 000011: Search features
- Task 14: Search and Filtering System

### Related ADRs
- (Future) ADR-002: Search Caching Strategy
- (Future) ADR-003: Korean Dictionary Management

---

**Last Updated**: 2025-11-14
**Review Date**: 2026-02-14 (3 months)
**Status**: Active
