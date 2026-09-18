package search

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Search operation metrics
	searchDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "search_duration_seconds",
			Help:    "Duration of search operations in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation"},
	)

	searchResultCount = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "search_result_count",
			Help:    "Number of results returned by search",
			Buckets: []float64{0, 1, 5, 10, 20, 50, 100, 200, 500, 1000},
		},
		[]string{"operation"},
	)

	searchErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "search_errors_total",
			Help: "Total number of search errors",
		},
		[]string{"operation", "error_type"},
	)

	// Cache metrics
	searchCacheHits = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "search_cache_hits_total",
			Help: "Total number of search cache hits",
		},
	)

	searchCacheMisses = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "search_cache_misses_total",
			Help: "Total number of search cache misses",
		},
	)

	searchCacheErrors = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "search_cache_errors_total",
			Help: "Total number of search cache errors",
		},
	)

	// Indexer metrics
	indexOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "search_index_operations_total",
			Help: "Total number of index operations",
		},
		[]string{"operation_type", "status"},
	)

	indexOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "search_index_operation_duration_seconds",
			Help:    "Duration of index operations",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation_type"},
	)

	indexQueueSize = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "search_index_queue_size",
			Help: "Current size of the index operation queue",
		},
	)

	indexRetryQueueSize = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "search_index_retry_queue_size",
			Help: "Current size of the index retry queue",
		},
	)

	indexBatchSize = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "search_index_batch_size",
			Help:    "Size of index batches processed",
			Buckets: []float64{1, 5, 10, 25, 50, 100, 200, 500},
		},
	)

	// Query metrics
	queryComplexity = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "search_query_complexity",
			Help:    "Complexity score of search queries",
			Buckets: []float64{1, 2, 3, 5, 7, 10, 15, 20},
		},
		[]string{"has_fulltext", "has_filters"},
	)

	facetComputeDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "search_facet_compute_duration_seconds",
			Help:    "Duration of facet computation",
			Buckets: prometheus.DefBuckets,
		},
	)
)

// Metrics provides structured metrics tracking
type Metrics struct {
	// Query metrics
	TotalSearches      int64
	TotalCachedHits    int64
	TotalErrors        int64
	AverageQueryTimeMs float64

	// Result metrics
	AverageResultCount float64
	EmptyResults       int64

	// Cache metrics
	CacheHitRate float64

	mu sync.RWMutex
}

// NewMetrics creates a new metrics tracker
func NewMetrics() *Metrics {
	return &Metrics{}
}

// RecordSearch records a search operation
func (m *Metrics) RecordSearch(duration time.Duration, resultCount int, cached bool, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.TotalSearches++

	// Record Prometheus metrics
	searchDuration.WithLabelValues("search").Observe(duration.Seconds())
	searchResultCount.WithLabelValues("search").Observe(float64(resultCount))

	if err != nil {
		m.TotalErrors++
		searchErrors.WithLabelValues("search", "query_error").Inc()
		return
	}

	if cached {
		m.TotalCachedHits++
		searchCacheHits.Inc()
	} else {
		searchCacheMisses.Inc()
	}

	if resultCount == 0 {
		m.EmptyResults++
	}

	// Update averages
	m.AverageQueryTimeMs = (m.AverageQueryTimeMs*float64(m.TotalSearches-1) + float64(duration.Milliseconds())) / float64(m.TotalSearches)
	m.AverageResultCount = (m.AverageResultCount*float64(m.TotalSearches-1) + float64(resultCount)) / float64(m.TotalSearches)

	// Update cache hit rate
	if m.TotalSearches > 0 {
		m.CacheHitRate = float64(m.TotalCachedHits) / float64(m.TotalSearches) * 100
	}
}

// RecordIndexOperation records an index operation
func (m *Metrics) RecordIndexOperation(operationType string, duration time.Duration, success bool) {
	status := "success"
	if !success {
		status = "failure"
	}

	indexOperationsTotal.WithLabelValues(operationType, status).Inc()
	indexOperationDuration.WithLabelValues(operationType).Observe(duration.Seconds())
}

// RecordBatchSize records the size of a processed batch
func (m *Metrics) RecordBatchSize(size int) {
	indexBatchSize.Observe(float64(size))
}

// UpdateQueueSizes updates the queue size metrics
func (m *Metrics) UpdateQueueSizes(operationQueue, retryQueue int) {
	indexQueueSize.Set(float64(operationQueue))
	indexRetryQueueSize.Set(float64(retryQueue))
}

// RecordQueryComplexity records query complexity
func (m *Metrics) RecordQueryComplexity(hasFullText bool, hasFilters bool, complexity int) {
	fulltext := "false"
	if hasFullText {
		fulltext = "true"
	}
	filters := "false"
	if hasFilters {
		filters = "true"
	}

	queryComplexity.WithLabelValues(fulltext, filters).Observe(float64(complexity))
}

// RecordFacetComputation records facet computation time
func (m *Metrics) RecordFacetComputation(duration time.Duration) {
	facetComputeDuration.Observe(duration.Seconds())
}

// RecordCacheError records a cache error
func (m *Metrics) RecordCacheError() {
	m.mu.Lock()
	defer m.mu.Unlock()
	searchCacheErrors.Inc()
}

// GetSnapshot returns a snapshot of current metrics
func (m *Metrics) GetSnapshot() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return MetricsSnapshot{
		TotalSearches:      m.TotalSearches,
		TotalCachedHits:    m.TotalCachedHits,
		TotalErrors:        m.TotalErrors,
		AverageQueryTimeMs: m.AverageQueryTimeMs,
		AverageResultCount: m.AverageResultCount,
		EmptyResults:       m.EmptyResults,
		CacheHitRate:       m.CacheHitRate,
	}
}

// MetricsSnapshot represents a point-in-time snapshot of metrics
type MetricsSnapshot struct {
	TotalSearches      int64   `json:"total_searches"`
	TotalCachedHits    int64   `json:"total_cached_hits"`
	TotalErrors        int64   `json:"total_errors"`
	AverageQueryTimeMs float64 `json:"average_query_time_ms"`
	AverageResultCount float64 `json:"average_result_count"`
	EmptyResults       int64   `json:"empty_results"`
	CacheHitRate       float64 `json:"cache_hit_rate_percent"`
}

// Reset resets all metrics
func (m *Metrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.TotalSearches = 0
	m.TotalCachedHits = 0
	m.TotalErrors = 0
	m.AverageQueryTimeMs = 0
	m.AverageResultCount = 0
	m.EmptyResults = 0
	m.CacheHitRate = 0
}
