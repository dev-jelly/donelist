package search

import (
	"time"

	"github.com/dev-jelly/donelist/internal/health"
)

// IndexerHealthChecker checks the health of the search indexer
type IndexerHealthChecker struct {
	indexer *Indexer
}

// NewIndexerHealthChecker creates a new indexer health checker
func NewIndexerHealthChecker(indexer *Indexer) *IndexerHealthChecker {
	return &IndexerHealthChecker{
		indexer: indexer,
	}
}

// Name returns the name of this health checker
func (c *IndexerHealthChecker) Name() string {
	return "search_indexer"
}

// Check performs the health check
func (c *IndexerHealthChecker) Check() health.ComponentHealth {
	result := health.ComponentHealth{
		Name:      c.Name(),
		Timestamp: time.Now(),
	}

	if !c.indexer.IsHealthy() {
		result.Status = health.StatusDegraded
		result.Error = "indexer is not healthy"

		// Get stats for additional context
		stats := c.indexer.GetStats()
		result.Metadata = map[string]interface{}{
			"pending_operations": stats.PendingOperations,
			"total_failed":       stats.TotalFailed,
			"last_indexed_at":    stats.LastIndexedAt,
		}
		return result
	}

	stats := c.indexer.GetStats()
	result.Status = health.StatusHealthy
	result.Metadata = map[string]interface{}{
		"total_indexed":         stats.TotalIndexed,
		"total_failed":          stats.TotalFailed,
		"total_retried":         stats.TotalRetried,
		"pending_operations":    stats.PendingOperations,
		"processing_rate":       stats.ProcessingRate,
		"average_latency_ms":    stats.AverageLatency.Milliseconds(),
		"initial_load_complete": stats.InitialLoadComplete,
		"last_indexed_at":       stats.LastIndexedAt,
	}

	return result
}
