package database

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupTestPerformanceMonitor(t *testing.T) (*PerformanceMonitor, sqlmock.Sqlmock, func()) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	logger, _ := zap.NewDevelopment()

	pm := NewPerformanceMonitor(sqlxDB, PerformanceConfig{
		SlowQueryThreshold: 50 * time.Millisecond,
		Enabled:            true,
	}, logger)

	cleanup := func() {
		db.Close()
	}

	return pm, mock, cleanup
}

func TestPerformanceMonitor_TrackQuery(t *testing.T) {
	pm, _, cleanup := setupTestPerformanceMonitor(t)
	defer cleanup()

	ctx := context.Background()
	query := "SELECT * FROM users WHERE id = $1"

	// Execute a fast query
	err := pm.TrackQuery(ctx, query, func() error {
		time.Sleep(10 * time.Millisecond)
		return nil
	})
	require.NoError(t, err)

	// Verify metrics were recorded
	metrics := pm.GetMetrics()
	require.Len(t, metrics, 1)
	assert.Equal(t, query, metrics[0].Query)
	assert.Equal(t, int64(1), metrics[0].Count)
	assert.Greater(t, metrics[0].TotalDuration, time.Duration(0))
}

func TestPerformanceMonitor_SlowQueryDetection(t *testing.T) {
	pm, _, cleanup := setupTestPerformanceMonitor(t)
	defer cleanup()

	ctx := context.Background()
	slowQuery := "SELECT * FROM users ORDER BY created_at DESC"

	// Execute a slow query (exceeds 50ms threshold)
	err := pm.TrackQuery(ctx, slowQuery, func() error {
		time.Sleep(60 * time.Millisecond)
		return nil
	})
	require.NoError(t, err)

	// Verify it's detected as slow
	slowQueries := pm.GetSlowQueries()
	require.Len(t, slowQueries, 1)
	assert.Equal(t, slowQuery, slowQueries[0].Query)
	assert.Greater(t, slowQueries[0].AvgDuration, 50*time.Millisecond)
}

func TestPerformanceMonitor_ErrorTracking(t *testing.T) {
	pm, _, cleanup := setupTestPerformanceMonitor(t)
	defer cleanup()

	ctx := context.Background()
	query := "INVALID SQL QUERY"
	expectedErr := errors.New("syntax error")

	// Execute query with error
	err := pm.TrackQuery(ctx, query, func() error {
		return expectedErr
	})
	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)

	// Verify error was counted
	metrics := pm.GetMetrics()
	require.Len(t, metrics, 1)
	assert.Equal(t, int64(1), metrics[0].Errors)
}

func TestPerformanceMonitor_MultipleExecutions(t *testing.T) {
	pm, _, cleanup := setupTestPerformanceMonitor(t)
	defer cleanup()

	ctx := context.Background()
	query := "SELECT * FROM users WHERE id = $1"

	// Execute query multiple times
	for i := 0; i < 5; i++ {
		pm.TrackQuery(ctx, query, func() error {
			time.Sleep(time.Duration(i*10) * time.Millisecond)
			return nil
		})
	}

	// Verify aggregated metrics
	metrics := pm.GetMetrics()
	require.Len(t, metrics, 1)
	assert.Equal(t, int64(5), metrics[0].Count)
	assert.Greater(t, metrics[0].AvgDuration, time.Duration(0))
	assert.Less(t, metrics[0].MinDuration, metrics[0].MaxDuration)
}

func TestPerformanceMonitor_EnableDisable(t *testing.T) {
	pm, _, cleanup := setupTestPerformanceMonitor(t)
	defer cleanup()

	ctx := context.Background()
	query := "SELECT * FROM users"

	// Initially enabled
	assert.True(t, pm.IsEnabled())

	// Track a query
	pm.TrackQuery(ctx, query, func() error { return nil })
	assert.Len(t, pm.GetMetrics(), 1)

	// Disable tracking
	pm.Disable()
	assert.False(t, pm.IsEnabled())

	// Track another query (should not be recorded)
	pm.TrackQuery(ctx, "SELECT * FROM categories", func() error { return nil })
	assert.Len(t, pm.GetMetrics(), 1) // Still only 1 metric

	// Re-enable
	pm.Enable()
	assert.True(t, pm.IsEnabled())
}

func TestPerformanceMonitor_ResetMetrics(t *testing.T) {
	pm, _, cleanup := setupTestPerformanceMonitor(t)
	defer cleanup()

	ctx := context.Background()

	// Track some queries
	pm.TrackQuery(ctx, "SELECT 1", func() error { return nil })
	pm.TrackQuery(ctx, "SELECT 2", func() error { return nil })

	assert.Len(t, pm.GetMetrics(), 2)

	// Reset
	pm.ResetMetrics()
	assert.Len(t, pm.GetMetrics(), 0)
}

func TestPerformanceMonitor_GetPoolStats(t *testing.T) {
	pm, _, cleanup := setupTestPerformanceMonitor(t)
	defer cleanup()

	stats := pm.GetPoolStats()
	assert.NotNil(t, stats)
}

func TestPreparedStatementManager_PrepareAndGet(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	logger, _ := zap.NewDevelopment()
	psm := NewPreparedStatementManager(sqlxDB, logger)

	query := "SELECT \\* FROM users WHERE id = \\$1"
	mock.ExpectPrepare(query)

	// Prepare statement
	err = psm.Prepare("get_user", "SELECT * FROM users WHERE id = $1")
	require.NoError(t, err)

	// Get statement
	stmt, err := psm.Get("get_user")
	require.NoError(t, err)
	assert.NotNil(t, stmt)

	// Try to get non-existent statement
	_, err = psm.Get("nonexistent")
	assert.Error(t, err)
}

func TestPreparedStatementManager_PrepareDuplicate(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	logger, _ := zap.NewDevelopment()
	psm := NewPreparedStatementManager(sqlxDB, logger)

	query := "SELECT \\* FROM users WHERE id = \\$1"
	mock.ExpectPrepare(query)

	// Prepare statement
	err = psm.Prepare("get_user", "SELECT * FROM users WHERE id = $1")
	require.NoError(t, err)

	// Try to prepare again (should not error, just skip)
	err = psm.Prepare("get_user", "SELECT * FROM users WHERE id = $1")
	require.NoError(t, err)
}

func TestPreparedStatementManager_Close(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	logger, _ := zap.NewDevelopment()
	psm := NewPreparedStatementManager(sqlxDB, logger)

	query := "SELECT \\* FROM users WHERE id = \\$1"
	mock.ExpectPrepare(query)

	// Prepare statement
	err = psm.Prepare("get_user", "SELECT * FROM users WHERE id = $1")
	require.NoError(t, err)

	// Close all statements
	err = psm.Close()
	require.NoError(t, err)

	// Verify statement is no longer available
	_, err = psm.Get("get_user")
	assert.Error(t, err)
}

func TestQueryMetrics_Calculation(t *testing.T) {
	pm, _, cleanup := setupTestPerformanceMonitor(t)
	defer cleanup()

	ctx := context.Background()
	query := "SELECT * FROM test"

	// Execute with known durations
	durations := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
	}

	for _, d := range durations {
		pm.TrackQuery(ctx, query, func() error {
			time.Sleep(d)
			return nil
		})
	}

	metrics := pm.GetMetrics()
	require.Len(t, metrics, 1)

	m := metrics[0]
	assert.Equal(t, int64(3), m.Count)
	assert.Greater(t, m.MinDuration, 5*time.Millisecond)
	assert.Less(t, m.MinDuration, 15*time.Millisecond)
	assert.Greater(t, m.MaxDuration, 25*time.Millisecond)
	assert.Greater(t, m.AvgDuration, 15*time.Millisecond)
	assert.Less(t, m.AvgDuration, 25*time.Millisecond)
}
