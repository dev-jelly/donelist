package database

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// QueryMetrics holds performance metrics for database queries
type QueryMetrics struct {
	Query         string
	Count         int64
	TotalDuration time.Duration
	AvgDuration   time.Duration
	MinDuration   time.Duration
	MaxDuration   time.Duration
	Errors        int64
	LastExecuted  time.Time
}

// PerformanceMonitor monitors database query performance
type PerformanceMonitor struct {
	db                *sqlx.DB
	logger            *zap.Logger
	slowQueryThreshold time.Duration
	metrics           map[string]*QueryMetrics
	mu                sync.RWMutex
	enabled           bool
}

// PerformanceConfig holds configuration for performance monitoring
type PerformanceConfig struct {
	SlowQueryThreshold time.Duration
	Enabled            bool
}

// NewPerformanceMonitor creates a new performance monitor
func NewPerformanceMonitor(db *sqlx.DB, cfg PerformanceConfig, logger *zap.Logger) *PerformanceMonitor {
	if cfg.SlowQueryThreshold == 0 {
		cfg.SlowQueryThreshold = 100 * time.Millisecond
	}

	return &PerformanceMonitor{
		db:                 db,
		logger:             logger,
		slowQueryThreshold: cfg.SlowQueryThreshold,
		metrics:            make(map[string]*QueryMetrics),
		enabled:            cfg.Enabled,
	}
}

// Enable enables performance monitoring
func (pm *PerformanceMonitor) Enable() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.enabled = true
}

// Disable disables performance monitoring
func (pm *PerformanceMonitor) Disable() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.enabled = false
}

// IsEnabled returns whether performance monitoring is enabled
func (pm *PerformanceMonitor) IsEnabled() bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.enabled
}

// TrackQuery wraps a query execution with performance tracking
func (pm *PerformanceMonitor) TrackQuery(ctx context.Context, query string, fn func() error) error {
	if !pm.IsEnabled() {
		return fn()
	}

	start := time.Now()
	err := fn()
	duration := time.Since(start)

	pm.recordMetrics(query, duration, err)

	if duration > pm.slowQueryThreshold {
		pm.logger.Warn("Slow query detected",
			zap.String("query", query),
			zap.Duration("duration", duration),
			zap.Duration("threshold", pm.slowQueryThreshold),
			zap.Error(err),
		)
	}

	return err
}

// recordMetrics records query execution metrics
func (pm *PerformanceMonitor) recordMetrics(query string, duration time.Duration, err error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	metric, exists := pm.metrics[query]
	if !exists {
		metric = &QueryMetrics{
			Query:        query,
			MinDuration:  duration,
			MaxDuration:  duration,
			LastExecuted: time.Now(),
		}
		pm.metrics[query] = metric
	}

	metric.Count++
	metric.TotalDuration += duration
	metric.AvgDuration = time.Duration(int64(metric.TotalDuration) / metric.Count)
	metric.LastExecuted = time.Now()

	if duration < metric.MinDuration {
		metric.MinDuration = duration
	}
	if duration > metric.MaxDuration {
		metric.MaxDuration = duration
	}
	if err != nil {
		metric.Errors++
	}
}

// GetMetrics returns all query metrics
func (pm *PerformanceMonitor) GetMetrics() []QueryMetrics {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	metrics := make([]QueryMetrics, 0, len(pm.metrics))
	for _, m := range pm.metrics {
		metrics = append(metrics, *m)
	}
	return metrics
}

// GetSlowQueries returns queries exceeding the slow query threshold
func (pm *PerformanceMonitor) GetSlowQueries() []QueryMetrics {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var slowQueries []QueryMetrics
	for _, m := range pm.metrics {
		if m.AvgDuration > pm.slowQueryThreshold || m.MaxDuration > pm.slowQueryThreshold*2 {
			slowQueries = append(slowQueries, *m)
		}
	}
	return slowQueries
}

// ResetMetrics clears all collected metrics
func (pm *PerformanceMonitor) ResetMetrics() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.metrics = make(map[string]*QueryMetrics)
}

// GetPoolStats returns database connection pool statistics
func (pm *PerformanceMonitor) GetPoolStats() sql.DBStats {
	return pm.db.Stats()
}

// LogPoolStats logs current connection pool statistics
func (pm *PerformanceMonitor) LogPoolStats() {
	stats := pm.GetPoolStats()
	pm.logger.Info("Database connection pool stats",
		zap.Int("max_open_connections", stats.MaxOpenConnections),
		zap.Int("open_connections", stats.OpenConnections),
		zap.Int("in_use", stats.InUse),
		zap.Int("idle", stats.Idle),
		zap.Int64("wait_count", stats.WaitCount),
		zap.Duration("wait_duration", stats.WaitDuration),
		zap.Int64("max_idle_closed", stats.MaxIdleClosed),
		zap.Int64("max_idle_time_closed", stats.MaxIdleTimeClosed),
		zap.Int64("max_lifetime_closed", stats.MaxLifetimeClosed),
	)
}

// PreparedStatementManager manages prepared statements for frequently used queries
type PreparedStatementManager struct {
	db         *sqlx.DB
	logger     *zap.Logger
	statements map[string]*sqlx.Stmt
	mu         sync.RWMutex
}

// NewPreparedStatementManager creates a new prepared statement manager
func NewPreparedStatementManager(db *sqlx.DB, logger *zap.Logger) *PreparedStatementManager {
	return &PreparedStatementManager{
		db:         db,
		logger:     logger,
		statements: make(map[string]*sqlx.Stmt),
	}
}

// Prepare prepares a statement and caches it
func (psm *PreparedStatementManager) Prepare(name, query string) error {
	psm.mu.Lock()
	defer psm.mu.Unlock()

	// Check if already prepared
	if _, exists := psm.statements[name]; exists {
		psm.logger.Debug("Statement already prepared", zap.String("name", name))
		return nil
	}

	stmt, err := psm.db.Preparex(query)
	if err != nil {
		psm.logger.Error("Failed to prepare statement",
			zap.String("name", name),
			zap.String("query", query),
			zap.Error(err),
		)
		return fmt.Errorf("prepare statement %s: %w", name, err)
	}

	psm.statements[name] = stmt
	psm.logger.Info("Prepared statement", zap.String("name", name))
	return nil
}

// Get retrieves a prepared statement by name
func (psm *PreparedStatementManager) Get(name string) (*sqlx.Stmt, error) {
	psm.mu.RLock()
	defer psm.mu.RUnlock()

	stmt, exists := psm.statements[name]
	if !exists {
		return nil, fmt.Errorf("statement %s not found", name)
	}

	return stmt, nil
}

// Close closes all prepared statements
func (psm *PreparedStatementManager) Close() error {
	psm.mu.Lock()
	defer psm.mu.Unlock()

	var errs []error
	for name, stmt := range psm.statements {
		if err := stmt.Close(); err != nil {
			psm.logger.Error("Failed to close statement",
				zap.String("name", name),
				zap.Error(err),
			)
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to close %d statements", len(errs))
	}

	psm.statements = make(map[string]*sqlx.Stmt)
	psm.logger.Info("Closed all prepared statements")
	return nil
}

// Common prepared statement names
const (
	StmtGetUserByID    = "get_user_by_id"
	StmtGetUserByEmail = "get_user_by_email"
	StmtCreateUser     = "create_user"
	StmtUpdateUser     = "update_user"

	StmtGetCheckinByID     = "get_checkin_by_id"
	StmtListUserCheckins   = "list_user_checkins"
	StmtCreateCheckin      = "create_checkin"
	StmtUpdateCheckin      = "update_checkin"
	StmtDeleteCheckin      = "delete_checkin"

	StmtListCategories     = "list_categories"
	StmtCreateCategory     = "create_category"
	StmtUpdateCategory     = "update_category"

	StmtListTags           = "list_tags"
	StmtCreateTag          = "create_tag"
)

// PrepareCommonStatements prepares frequently used SQL statements
func PrepareCommonStatements(psm *PreparedStatementManager) error {
	statements := map[string]string{
		StmtGetUserByID: `
			SELECT id, email, password_hash, display_name, role, tier, tier_expires_at, mode_preference, created_at, updated_at
			FROM users
			WHERE id = $1 AND deleted_at IS NULL
		`,
		StmtGetUserByEmail: `
			SELECT id, email, password_hash, display_name, role, tier, tier_expires_at, mode_preference, created_at, updated_at
			FROM users
			WHERE email = $1 AND deleted_at IS NULL
		`,
		// Add more common statements as needed
	}

	for name, query := range statements {
		if err := psm.Prepare(name, query); err != nil {
			return fmt.Errorf("prepare %s: %w", name, err)
		}
	}

	return nil
}
