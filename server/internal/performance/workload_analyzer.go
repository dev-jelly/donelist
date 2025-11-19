package performance

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
)

// WorkloadAnalyzer analyzes database workload and suggests optimizations
type WorkloadAnalyzer struct {
	db *sqlx.DB
}

// NewWorkloadAnalyzer creates a new workload analyzer
func NewWorkloadAnalyzer(db *sqlx.DB) *WorkloadAnalyzer {
	return &WorkloadAnalyzer{db: db}
}

// QueryPattern represents a categorized query pattern
type QueryPattern struct {
	Pattern       string   `db:"pattern"`
	QueryType     string   `db:"query_type"`
	Calls         int64    `db:"calls"`
	TotalTimeMS   float64  `db:"total_time_ms"`
	MeanTimeMS    float64  `db:"mean_time_ms"`
	TablesAccessed []string `db:"tables_accessed"`
	SampleQuery   string   `db:"sample_query"`
}

// IndexCandidate represents a potential index to create
type IndexCandidate struct {
	TableName      string   `json:"table_name"`
	ColumnNames    []string `json:"column_names"`
	IndexType      string   `json:"index_type"`
	Reason         string   `json:"reason"`
	EstimatedGain  string   `json:"estimated_gain"`
	Priority       int      `json:"priority"`
	CreateSQL      string   `json:"create_sql"`
	SequentialScans int64   `json:"sequential_scans"`
	TableRows      int64    `json:"table_rows"`
}

// UnusedIndex represents an index that is not being used
type UnusedIndex struct {
	SchemaName string `db:"schemaname"`
	TableName  string `db:"tablename"`
	IndexName  string `db:"indexname"`
	IndexSize  string `db:"index_size"`
	IndexScans int64  `db:"idx_scan"`
}

// MissingIndexInfo represents a table that might benefit from an index
type MissingIndexInfo struct {
	TableName       string `db:"table_name"`
	SequentialScans int64  `db:"seq_scan"`
	RowsSeqRead     int64  `db:"seq_tup_read"`
	LiveTuples      int64  `db:"n_live_tup"`
	AvgRowsPerScan  int64  `db:"avg_rows_per_scan"`
}

// WorkloadReport contains comprehensive workload analysis
type WorkloadReport struct {
	GeneratedAt      string            `json:"generated_at"`
	TopQueries       []SlowQuery       `json:"top_queries"`
	QueryPatterns    []QueryPattern    `json:"query_patterns"`
	IndexCandidates  []IndexCandidate  `json:"index_candidates"`
	UnusedIndexes    []UnusedIndex     `json:"unused_indexes"`
	MissingIndexes   []MissingIndexInfo `json:"missing_indexes"`
	Recommendations  []string          `json:"recommendations"`
}

// AnalyzeWorkload performs comprehensive workload analysis
func (wa *WorkloadAnalyzer) AnalyzeWorkload(ctx context.Context) (*WorkloadReport, error) {
	report := &WorkloadReport{
		GeneratedAt: "NOW()",
		Recommendations: []string{},
	}

	// Get top slow queries
	monitor := NewMonitor(wa.db)
	topQueries, err := monitor.GetTopSlowQueries(ctx, 50)
	if err != nil {
		return nil, fmt.Errorf("failed to get top queries: %w", err)
	}
	report.TopQueries = topQueries

	// Identify unused indexes
	unusedIndexes, err := wa.GetUnusedIndexes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get unused indexes: %w", err)
	}
	report.UnusedIndexes = unusedIndexes

	// Identify tables with high sequential scans
	missingIndexes, err := wa.GetTablesNeedingIndexes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get missing indexes: %w", err)
	}
	report.MissingIndexes = missingIndexes

	// Generate index candidates
	indexCandidates, err := wa.GenerateIndexCandidates(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to generate index candidates: %w", err)
	}
	report.IndexCandidates = indexCandidates

	// Generate recommendations
	report.Recommendations = wa.GenerateRecommendations(report)

	return report, nil
}

// GetUnusedIndexes finds indexes that are never or rarely used
func (wa *WorkloadAnalyzer) GetUnusedIndexes(ctx context.Context) ([]UnusedIndex, error) {
	query := `
		SELECT
			schemaname,
			tablename,
			indexname,
			pg_size_pretty(pg_relation_size(indexrelid::regclass)) as index_size,
			idx_scan
		FROM pg_stat_user_indexes
		WHERE schemaname NOT IN ('pg_catalog', 'information_schema', 'performance')
			AND idx_scan < 50  -- Adjust threshold as needed
			AND indexrelname NOT LIKE '%_pkey'  -- Exclude primary keys
		ORDER BY pg_relation_size(indexrelid::regclass) DESC
	`

	var indexes []UnusedIndex
	err := wa.db.SelectContext(ctx, &indexes, query)
	if err != nil {
		return nil, err
	}

	return indexes, nil
}

// GetTablesNeedingIndexes identifies tables with high sequential scans
func (wa *WorkloadAnalyzer) GetTablesNeedingIndexes(ctx context.Context) ([]MissingIndexInfo, error) {
	query := `
		SELECT
			relname as table_name,
			seq_scan,
			seq_tup_read as seq_tup_read,
			n_live_tup,
			CASE WHEN seq_scan > 0 THEN seq_tup_read / seq_scan ELSE 0 END as avg_rows_per_scan
		FROM pg_stat_user_tables
		WHERE schemaname NOT IN ('pg_catalog', 'information_schema', 'performance')
			AND seq_scan > 100  -- High number of sequential scans
			AND n_live_tup > 1000  -- Significant table size
			AND CASE WHEN seq_scan > 0 THEN seq_tup_read / seq_scan ELSE 0 END > 100  -- High rows per scan
		ORDER BY seq_scan * n_live_tup DESC
		LIMIT 20
	`

	var tables []MissingIndexInfo
	err := wa.db.SelectContext(ctx, &tables, query)
	if err != nil {
		return nil, err
	}

	return tables, nil
}

// GenerateIndexCandidates creates index recommendations based on workload
func (wa *WorkloadAnalyzer) GenerateIndexCandidates(ctx context.Context) ([]IndexCandidate, error) {
	candidates := []IndexCandidate{}

	// Analyze checkins table (most critical)
	checkinsIndexes := wa.analyzeCheckinsTable(ctx)
	candidates = append(candidates, checkinsIndexes...)

	// Analyze other high-traffic tables
	analyticsIndexes := wa.analyzeAnalyticsEvents(ctx)
	candidates = append(candidates, analyticsIndexes...)

	searchIndexes := wa.analyzeSearchHistory(ctx)
	candidates = append(candidates, searchIndexes...)

	syncIndexes := wa.analyzeSyncQueue(ctx)
	candidates = append(candidates, syncIndexes...)

	return candidates, nil
}

// analyzeCheckinsTable generates index candidates for checkins table
func (wa *WorkloadAnalyzer) analyzeCheckinsTable(ctx context.Context) []IndexCandidate {
	candidates := []IndexCandidate{}

	// Get table statistics
	var seqScans, liveRows int64
	query := `SELECT seq_scan, n_live_tup FROM pg_stat_user_tables WHERE relname = 'checkins'`
	wa.db.QueryRowContext(ctx, query).Scan(&seqScans, &liveRows)

	// Composite index for user_id + checkin_time range queries
	if seqScans > 100 {
		candidates = append(candidates, IndexCandidate{
			TableName:   "checkins",
			ColumnNames: []string{"user_id", "checkin_time", "deleted_at"},
			IndexType:   "btree",
			Reason:      "Optimize user timeline queries with date range filtering and soft delete support",
			EstimatedGain: "High - used in most checkin list queries",
			Priority:    1,
			CreateSQL:   "CREATE INDEX CONCURRENTLY idx_checkins_user_time_deleted ON checkins(user_id, checkin_time DESC, deleted_at) WHERE deleted_at IS NULL;",
			SequentialScans: seqScans,
			TableRows:   liveRows,
		})

		// Category + time index for category-based filtering
		candidates = append(candidates, IndexCandidate{
			TableName:   "checkins",
			ColumnNames: []string{"category_id", "checkin_time"},
			IndexType:   "btree",
			Reason:      "Speed up category-filtered timeline queries",
			EstimatedGain: "Medium - used in category filtering",
			Priority:    2,
			CreateSQL:   "CREATE INDEX CONCURRENTLY idx_checkins_category_time ON checkins(category_id, checkin_time DESC) WHERE category_id IS NOT NULL AND deleted_at IS NULL;",
			SequentialScans: seqScans,
			TableRows:   liveRows,
		})

		// Partial index for edited checkins
		candidates = append(candidates, IndexCandidate{
			TableName:   "checkins",
			ColumnNames: []string{"user_id", "is_edited", "last_edited_at"},
			IndexType:   "btree",
			Reason:      "Optimize queries for edited checkins (premium feature)",
			EstimatedGain: "Low - specific use case",
			Priority:    4,
			CreateSQL:   "CREATE INDEX CONCURRENTLY idx_checkins_edited ON checkins(user_id, last_edited_at DESC) WHERE is_edited = true;",
			SequentialScans: seqScans,
			TableRows:   liveRows,
		})

		// GIN index for full-text search on content
		candidates = append(candidates, IndexCandidate{
			TableName:   "checkins",
			ColumnNames: []string{"content"},
			IndexType:   "gin",
			Reason:      "Enable fast full-text search on checkin content",
			EstimatedGain: "High - critical for search functionality",
			Priority:    1,
			CreateSQL:   "CREATE INDEX CONCURRENTLY idx_checkins_content_fts ON checkins USING gin(to_tsvector('english', content));",
			SequentialScans: seqScans,
			TableRows:   liveRows,
		})
	}

	return candidates
}

// analyzeAnalyticsEvents generates index candidates for analytics_events table
func (wa *WorkloadAnalyzer) analyzeAnalyticsEvents(ctx context.Context) []IndexCandidate {
	candidates := []IndexCandidate{}

	var seqScans, liveRows int64
	query := `SELECT seq_scan, n_live_tup FROM pg_stat_user_tables WHERE relname = 'analytics_events'`
	wa.db.QueryRowContext(ctx, query).Scan(&seqScans, &liveRows)

	if seqScans > 50 {
		// Composite index for analytics queries
		candidates = append(candidates, IndexCandidate{
			TableName:   "analytics_events",
			ColumnNames: []string{"user_id", "event_type", "created_at"},
			IndexType:   "btree",
			Reason:      "Optimize analytics aggregation queries by user and event type",
			EstimatedGain: "Medium - used in analytics reporting",
			Priority:    3,
			CreateSQL:   "CREATE INDEX CONCURRENTLY idx_analytics_user_type_time ON analytics_events(user_id, event_type, created_at DESC);",
			SequentialScans: seqScans,
			TableRows:   liveRows,
		})

		// Time-based partitioning suggestion
		if liveRows > 100000 {
			candidates = append(candidates, IndexCandidate{
				TableName:   "analytics_events",
				ColumnNames: []string{"created_at"},
				IndexType:   "partition",
				Reason:      "Table is large - consider partitioning by month for better query performance and maintenance",
				EstimatedGain: "High - improves query performance and enables efficient data archiving",
				Priority:    2,
				CreateSQL:   "-- See partitioning migration for analytics_events by month",
				SequentialScans: seqScans,
				TableRows:   liveRows,
			})
		}
	}

	return candidates
}

// analyzeSearchHistory generates index candidates for search_history table
func (wa *WorkloadAnalyzer) analyzeSearchHistory(ctx context.Context) []IndexCandidate {
	candidates := []IndexCandidate{}

	var seqScans, liveRows int64
	query := `SELECT seq_scan, n_live_tup FROM pg_stat_user_tables WHERE relname = 'search_history'`
	wa.db.QueryRowContext(ctx, query).Scan(&seqScans, &liveRows)

	if seqScans > 50 {
		candidates = append(candidates, IndexCandidate{
			TableName:   "search_history",
			ColumnNames: []string{"user_id", "searched_at"},
			IndexType:   "btree",
			Reason:      "Speed up user search history retrieval",
			EstimatedGain: "Medium - used in search features",
			Priority:    3,
			CreateSQL:   "CREATE INDEX CONCURRENTLY idx_search_history_user_time ON search_history(user_id, searched_at DESC);",
			SequentialScans: seqScans,
			TableRows:   liveRows,
		})
	}

	return candidates
}

// analyzeSyncQueue generates index candidates for sync_queue table
func (wa *WorkloadAnalyzer) analyzeSyncQueue(ctx context.Context) []IndexCandidate {
	candidates := []IndexCandidate{}

	var seqScans, liveRows int64
	query := `SELECT seq_scan, n_live_tup FROM pg_stat_user_tables WHERE relname = 'sync_queue'`
	wa.db.QueryRowContext(ctx, query).Scan(&seqScans, &liveRows)

	if seqScans > 50 {
		// Composite index for pending sync items
		candidates = append(candidates, IndexCandidate{
			TableName:   "sync_queue",
			ColumnNames: []string{"user_id", "status", "created_at"},
			IndexType:   "btree",
			Reason:      "Optimize sync queue processing for pending items",
			EstimatedGain: "High - critical for offline sync performance",
			Priority:    1,
			CreateSQL:   "CREATE INDEX CONCURRENTLY idx_sync_queue_user_status ON sync_queue(user_id, status, created_at) WHERE status = 'pending';",
			SequentialScans: seqScans,
			TableRows:   liveRows,
		})
	}

	return candidates
}

// GenerateRecommendations creates actionable recommendations
func (wa *WorkloadAnalyzer) GenerateRecommendations(report *WorkloadReport) []string {
	recommendations := []string{}

	// Check for slow queries
	if len(report.TopQueries) > 0 {
		slowCount := 0
		for _, q := range report.TopQueries {
			if q.AvgTimeMS > 1000 {
				slowCount++
			}
		}
		if slowCount > 5 {
			recommendations = append(recommendations,
				fmt.Sprintf("Found %d queries averaging >1 second - review and optimize these queries", slowCount))
		}
	}

	// Check for unused indexes
	if len(report.UnusedIndexes) > 0 {
		for _, idx := range report.UnusedIndexes {
			if idx.IndexScans < 10 {
				recommendations = append(recommendations,
					fmt.Sprintf("Consider dropping unused index: %s.%s (%s)", idx.TableName, idx.IndexName, idx.IndexSize))
			}
		}
		if len(report.UnusedIndexes) > 5 {
			recommendations = append(recommendations,
				fmt.Sprintf("Found %d rarely used indexes - review and consider dropping to reduce storage", len(report.UnusedIndexes)))
		}
	}

	// Check for missing indexes
	if len(report.MissingIndexes) > 0 {
		for _, tbl := range report.MissingIndexes {
			if tbl.SequentialScans > 1000 && tbl.LiveTuples > 10000 {
				recommendations = append(recommendations,
					fmt.Sprintf("Table '%s' has %d seq scans with %d rows - strong index candidate",
						tbl.TableName, tbl.SequentialScans, tbl.LiveTuples))
			}
		}
	}

	// Priority index recommendations
	if len(report.IndexCandidates) > 0 {
		highPriority := 0
		for _, candidate := range report.IndexCandidates {
			if candidate.Priority == 1 {
				highPriority++
				recommendations = append(recommendations,
					fmt.Sprintf("HIGH PRIORITY: Create index on %s(%s) - %s",
						candidate.TableName, strings.Join(candidate.ColumnNames, ", "), candidate.Reason))
			}
		}
	}

	return recommendations
}
