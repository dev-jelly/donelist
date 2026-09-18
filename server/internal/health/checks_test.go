package health

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func TestPostgresChecker_Healthy(t *testing.T) {
	// Create mock database
	mockDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	assert.NoError(t, err)
	defer mockDB.Close()

	db := sqlx.NewDb(mockDB, "postgres")

	// Set max open connections to avoid pool capacity issues with mock
	db.SetMaxOpenConns(10)

	// Expect ping
	mock.ExpectPing()

	// Expect SELECT 1 query
	rows := sqlmock.NewRows([]string{"?column?"}).AddRow(1)
	mock.ExpectQuery("SELECT 1").WillReturnRows(rows)

	// Create checker
	checker := NewPostgresChecker(db, 2*time.Second)

	// Perform check
	health := checker.Check()

	// Verify
	assert.Equal(t, "postgresql", health.Name)
	assert.Equal(t, StatusHealthy, health.Status)
	assert.Contains(t, health.Message, "healthy")
	assert.Empty(t, health.Error)
	assert.NotNil(t, health.Metadata)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPostgresChecker_PingFailed(t *testing.T) {
	// Create mock database
	mockDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	assert.NoError(t, err)
	defer mockDB.Close()

	db := sqlx.NewDb(mockDB, "postgres")

	// Expect ping to fail
	mock.ExpectPing().WillReturnError(context.DeadlineExceeded)

	// Create checker
	checker := NewPostgresChecker(db, 2*time.Second)

	// Perform check
	health := checker.Check()

	// Verify
	assert.Equal(t, "postgresql", health.Name)
	assert.Equal(t, StatusUnhealthy, health.Status)
	assert.Contains(t, health.Error, "ping failed")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPostgresChecker_QueryFailed(t *testing.T) {
	// Create mock database
	mockDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	assert.NoError(t, err)
	defer mockDB.Close()

	db := sqlx.NewDb(mockDB, "postgres")

	// Set max open connections to avoid pool capacity issues
	db.SetMaxOpenConns(10)

	// Expect ping to succeed
	mock.ExpectPing()

	// Expect SELECT 1 query to fail
	mock.ExpectQuery("SELECT 1").WillReturnError(context.DeadlineExceeded)

	// Create checker
	checker := NewPostgresChecker(db, 2*time.Second)

	// Perform check
	health := checker.Check()

	// Verify - should be degraded, not unhealthy
	assert.Equal(t, "postgresql", health.Name)
	assert.Equal(t, StatusDegraded, health.Status)
	assert.Contains(t, health.Error, "query failed")
	// Note: If connection pool is at capacity, the message might be different
	// Just verify it's degraded and has an error
	assert.NotEmpty(t, health.Message)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPostgresChecker_Timeout(t *testing.T) {
	// Create mock database
	mockDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	assert.NoError(t, err)
	defer mockDB.Close()

	db := sqlx.NewDb(mockDB, "postgres")

	// Expect ping with delay
	mock.ExpectPing().WillDelayFor(3 * time.Second)

	// Create checker with short timeout
	checker := NewPostgresChecker(db, 100*time.Millisecond)

	// Perform check
	health := checker.Check()

	// Verify - should timeout and be unhealthy
	assert.Equal(t, "postgresql", health.Name)
	assert.Equal(t, StatusUnhealthy, health.Status)
	assert.Contains(t, health.Error, "ping failed")
}

func TestRedisChecker_Healthy(t *testing.T) {
	// This test requires a running Redis instance or mock
	// For simplicity, we'll test the interface
	t.Skip("Requires Redis mock implementation")
}

func TestRedisChecker_Name(t *testing.T) {
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	defer client.Close()

	checker := NewRedisChecker(client, 2*time.Second)

	assert.Equal(t, "redis", checker.Name())
}

func TestSystemChecker_Healthy(t *testing.T) {
	getCurrentMem := func() int64 { return 512 } // 512MB
	getCurrentGoro := func() int { return 100 }

	checker := NewSystemChecker(getCurrentMem, getCurrentGoro)

	health := checker.Check()

	assert.Equal(t, "system", health.Name)
	assert.Equal(t, StatusHealthy, health.Status)
	assert.Contains(t, health.Message, "normal limits")
	assert.Equal(t, int64(512), health.Metadata["memory_mb"])
	assert.Equal(t, 100, health.Metadata["goroutines"])
}

func TestSystemChecker_HighMemory(t *testing.T) {
	getCurrentMem := func() int64 { return 2048 } // 2GB (exceeds 1GB threshold)
	getCurrentGoro := func() int { return 100 }

	checker := NewSystemChecker(getCurrentMem, getCurrentGoro)

	health := checker.Check()

	assert.Equal(t, "system", health.Name)
	assert.Equal(t, StatusDegraded, health.Status)
	assert.Contains(t, health.Message, "memory usage")
	assert.Contains(t, health.Message, "exceeds threshold")
}

func TestSystemChecker_HighGoroutines(t *testing.T) {
	getCurrentMem := func() int64 { return 512 }
	getCurrentGoro := func() int { return 15000 } // Exceeds 10000 threshold

	checker := NewSystemChecker(getCurrentMem, getCurrentGoro)

	health := checker.Check()

	assert.Equal(t, "system", health.Name)
	assert.Equal(t, StatusDegraded, health.Status)
	assert.Contains(t, health.Message, "goroutine count")
	assert.Contains(t, health.Message, "exceeds threshold")
}

func TestSystemChecker_BothHigh(t *testing.T) {
	getCurrentMem := func() int64 { return 2048 }
	getCurrentGoro := func() int { return 15000 }

	checker := NewSystemChecker(getCurrentMem, getCurrentGoro)

	health := checker.Check()

	assert.Equal(t, "system", health.Name)
	assert.Equal(t, StatusDegraded, health.Status)
	// Should report at least one issue
	assert.NotEmpty(t, health.Message)
}

func TestDummyChecker(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		checker := NewDummyChecker("test", StatusHealthy)

		assert.Equal(t, "test", checker.Name())

		health := checker.Check()
		assert.Equal(t, "test", health.Name)
		assert.Equal(t, StatusHealthy, health.Status)
		assert.Equal(t, "dummy check", health.Message)
	})

	t.Run("unhealthy", func(t *testing.T) {
		checker := NewDummyChecker("test", StatusUnhealthy)

		health := checker.Check()
		assert.Equal(t, "test", health.Name)
		assert.Equal(t, StatusUnhealthy, health.Status)
	})

	t.Run("degraded", func(t *testing.T) {
		checker := NewDummyChecker("test", StatusDegraded)

		health := checker.Check()
		assert.Equal(t, "test", health.Name)
		assert.Equal(t, StatusDegraded, health.Status)
	})
}

func BenchmarkPostgresChecker(b *testing.B) {
	// Create mock database
	mockDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		b.Fatal(err)
	}
	defer mockDB.Close()

	db := sqlx.NewDb(mockDB, "postgres")

	// Setup expectations
	for i := 0; i < b.N; i++ {
		mock.ExpectPing()
		rows := sqlmock.NewRows([]string{"?column?"}).AddRow(1)
		mock.ExpectQuery("SELECT 1").WillReturnRows(rows)
	}

	checker := NewPostgresChecker(db, 2*time.Second)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checker.Check()
	}
}

func BenchmarkSystemChecker(b *testing.B) {
	getCurrentMem := func() int64 { return 512 }
	getCurrentGoro := func() int { return 100 }

	checker := NewSystemChecker(getCurrentMem, getCurrentGoro)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checker.Check()
	}
}
