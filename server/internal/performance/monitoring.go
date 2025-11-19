package performance

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// Monitor handles performance monitoring and metrics collection
type Monitor struct {
	db *sqlx.DB
}

// NewMonitor creates a new performance monitor
func NewMonitor(db *sqlx.DB) *Monitor {
	return &Monitor{db: db}
}

// BaselineMetrics represents daily performance baseline measurements
type BaselineMetrics struct {
	ID                   int       `db:"id"`
	MeasurementDate      time.Time `db:"measurement_date"`
	TPSAvg               float64   `db:"tps_avg"`
	TPSPeak              float64   `db:"tps_peak"`
	LatencyP50MS         float64   `db:"latency_p50_ms"`
	LatencyP95MS         float64   `db:"latency_p95_ms"`
	LatencyP99MS         float64   `db:"latency_p99_ms"`
	ActiveConnectionsAvg int       `db:"active_connections_avg"`
	ActiveConnectionsPeak int      `db:"active_connections_peak"`
	CacheHitRatio        float64   `db:"cache_hit_ratio"`
	TotalQueries         int64     `db:"total_queries"`
	SlowQueriesCount     int       `db:"slow_queries_count"`
	CreatedAt            time.Time `db:"created_at"`
}

// CurrentDBStats represents current database statistics
type CurrentDBStats struct {
	ActiveConnections      int     `db:"active_connections"`
	TransactionsCommitted  int64   `db:"transactions_committed"`
	TransactionsRolledBack int64   `db:"transactions_rolled_back"`
	BlocksReadFromDisk     int64   `db:"blocks_read_from_disk"`
	BlocksReadFromCache    int64   `db:"blocks_read_from_cache"`
	CacheHitRatioPct       float64 `db:"cache_hit_ratio_pct"`
	RowsReturned           int64   `db:"rows_returned"`
	RowsFetched            int64   `db:"rows_fetched"`
	RowsInserted           int64   `db:"rows_inserted"`
	RowsUpdated            int64   `db:"rows_updated"`
	RowsDeleted            int64   `db:"rows_deleted"`
	Conflicts              int64   `db:"conflicts"`
	TempFiles              int64   `db:"temp_files"`
	TempBytes              int64   `db:"temp_bytes"`
	Deadlocks              int64   `db:"deadlocks"`
	StatsReset             sql.NullTime `db:"stats_reset"`
}

// SlowQuery represents a slow query entry
type SlowQuery struct {
	QueryHash      string  `db:"query_hash"`
	QueryPreview   string  `db:"query_preview"`
	Calls          int64   `db:"calls"`
	AvgTimeMS      float64 `db:"avg_time_ms"`
	TotalTimeMS    float64 `db:"total_time_ms"`
	MaxTimeMS      float64 `db:"max_time_ms"`
	RowsReturned   int64   `db:"rows_returned"`
	CacheHitPct    float64 `db:"cache_hit_pct"`
}

// TableStats represents table-level statistics
type TableStats struct {
	SchemaName       string         `db:"schemaname"`
	TableName        string         `db:"table_name"`
	SequentialScans  int64          `db:"sequential_scans"`
	RowsSeqScanned   int64          `db:"rows_seq_scanned"`
	IndexScans       int64          `db:"index_scans"`
	RowsIndexFetched int64          `db:"rows_index_fetched"`
	RowsInserted     int64          `db:"rows_inserted"`
	RowsUpdated      int64          `db:"rows_updated"`
	RowsDeleted      int64          `db:"rows_deleted"`
	HotUpdates       int64          `db:"hot_updates"`
	LiveRows         int64          `db:"live_rows"`
	DeadRows         int64          `db:"dead_rows"`
	DeadRowPct       float64        `db:"dead_row_pct"`
	LastVacuum       sql.NullTime   `db:"last_vacuum"`
	LastAutovacuum   sql.NullTime   `db:"last_autovacuum"`
	LastAnalyze      sql.NullTime   `db:"last_analyze"`
	LastAutoanalyze  sql.NullTime   `db:"last_autoanalyze"`
}

// IndexUsageStats represents index usage statistics
type IndexUsageStats struct {
	SchemaName    string `db:"schemaname"`
	TableName     string `db:"tablename"`
	IndexName     string `db:"indexname"`
	IndexScans    int64  `db:"index_scans"`
	TuplesRead    int64  `db:"tuples_read"`
	TuplesFetched int64  `db:"tuples_fetched"`
	IndexSize     string `db:"index_size"`
}

// CaptureBaselineMetrics collects and stores current performance baseline
func (m *Monitor) CaptureBaselineMetrics(ctx context.Context) error {
	query := `SELECT performance.capture_baseline_metrics()`
	_, err := m.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to capture baseline metrics: %w", err)
	}
	return nil
}

// CaptureQueryStatsSnapshot captures current query statistics
func (m *Monitor) CaptureQueryStatsSnapshot(ctx context.Context) error {
	query := `SELECT performance.capture_query_stats_snapshot()`
	_, err := m.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to capture query stats snapshot: %w", err)
	}
	return nil
}

// GetCurrentDBStats retrieves current database statistics
func (m *Monitor) GetCurrentDBStats(ctx context.Context) (*CurrentDBStats, error) {
	var stats CurrentDBStats
	query := `SELECT * FROM performance.current_db_stats`
	err := m.db.GetContext(ctx, &stats, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get current db stats: %w", err)
	}
	return &stats, nil
}

// GetTopSlowQueries retrieves the slowest queries
func (m *Monitor) GetTopSlowQueries(ctx context.Context, limit int) ([]SlowQuery, error) {
	var queries []SlowQuery
	query := `
		SELECT
			query_hash,
			query_preview,
			calls,
			avg_time_ms,
			total_time_ms,
			max_time_ms,
			rows_returned,
			cache_hit_pct
		FROM performance.top_slow_queries
		LIMIT $1
	`
	err := m.db.SelectContext(ctx, &queries, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get top slow queries: %w", err)
	}
	return queries, nil
}

// GetTableStats retrieves table-level statistics
func (m *Monitor) GetTableStats(ctx context.Context) ([]TableStats, error) {
	var stats []TableStats
	query := `SELECT * FROM performance.table_stats`
	err := m.db.SelectContext(ctx, &stats, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get table stats: %w", err)
	}
	return stats, nil
}

// GetIndexUsageStats retrieves index usage statistics
func (m *Monitor) GetIndexUsageStats(ctx context.Context) ([]IndexUsageStats, error) {
	var stats []IndexUsageStats
	query := `SELECT * FROM performance.index_usage_stats`
	err := m.db.SelectContext(ctx, &stats, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get index usage stats: %w", err)
	}
	return stats, nil
}

// GetBaselineMetrics retrieves baseline metrics for a date range
func (m *Monitor) GetBaselineMetrics(ctx context.Context, days int) ([]BaselineMetrics, error) {
	var metrics []BaselineMetrics
	query := `
		SELECT *
		FROM performance.baseline_metrics
		WHERE measurement_date >= CURRENT_DATE - INTERVAL '$1 days'
		ORDER BY measurement_date DESC
	`
	err := m.db.SelectContext(ctx, &metrics, query, days)
	if err != nil {
		return nil, fmt.Errorf("failed to get baseline metrics: %w", err)
	}
	return metrics, nil
}

// CheckPgStatStatementsEnabled checks if pg_stat_statements is enabled
func (m *Monitor) CheckPgStatStatementsEnabled(ctx context.Context) (bool, error) {
	var exists bool
	query := `
		SELECT EXISTS (
			SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements'
		)
	`
	err := m.db.GetContext(ctx, &exists, query)
	if err != nil {
		return false, fmt.Errorf("failed to check pg_stat_statements: %w", err)
	}
	return exists, nil
}

// ResetPgStatStatements resets pg_stat_statements counters
func (m *Monitor) ResetPgStatStatements(ctx context.Context) error {
	query := `SELECT pg_stat_statements_reset()`
	_, err := m.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to reset pg_stat_statements: %w", err)
	}
	return nil
}

// GetPerformanceHealth returns an overall health assessment
func (m *Monitor) GetPerformanceHealth(ctx context.Context) (*HealthStatus, error) {
	stats, err := m.GetCurrentDBStats(ctx)
	if err != nil {
		return nil, err
	}

	health := &HealthStatus{
		Timestamp: time.Now(),
		Healthy:   true,
		Issues:    []string{},
	}

	// Check cache hit ratio
	if stats.CacheHitRatioPct < 90 {
		health.Healthy = false
		health.Issues = append(health.Issues, fmt.Sprintf("Low cache hit ratio: %.2f%% (threshold: 90%%)", stats.CacheHitRatioPct))
	}

	// Check deadlocks
	if stats.Deadlocks > 0 {
		health.Issues = append(health.Issues, fmt.Sprintf("Deadlocks detected: %d", stats.Deadlocks))
	}

	// Check temp file usage
	if stats.TempFiles > 100 {
		health.Issues = append(health.Issues, fmt.Sprintf("High temp file usage: %d files", stats.TempFiles))
	}

	health.CacheHitRatio = stats.CacheHitRatioPct
	health.ActiveConnections = stats.ActiveConnections
	health.Deadlocks = stats.Deadlocks

	return health, nil
}

// HealthStatus represents the current health status
type HealthStatus struct {
	Timestamp         time.Time `json:"timestamp"`
	Healthy           bool      `json:"healthy"`
	CacheHitRatio     float64   `json:"cache_hit_ratio"`
	ActiveConnections int       `json:"active_connections"`
	Deadlocks         int64     `json:"deadlocks"`
	Issues            []string  `json:"issues"`
}
