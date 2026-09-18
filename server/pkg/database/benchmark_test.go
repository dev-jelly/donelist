package database

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
	"github.com/dev-jelly/donelist/pkg/cache"
)

// BenchmarkConnectionPooling benchmarks database connection pool performance
func BenchmarkConnectionPooling(b *testing.B) {
	db, mock, err := sqlmock.New()
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")

	// Set up connection pool
	sqlxDB.SetMaxOpenConns(100)
	sqlxDB.SetMaxIdleConns(25)
	sqlxDB.SetConnMaxLifetime(30 * time.Minute)
	sqlxDB.SetConnMaxIdleTime(10 * time.Minute)

	ctx := context.Background()

	b.Run("QueryExecution", func(b *testing.B) {
		rows := sqlmock.NewRows([]string{"id", "name"}).
			AddRow(1, "test")
		mock.ExpectQuery("SELECT (.+) FROM users").WillReturnRows(rows)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			sqlxDB.QueryContext(ctx, "SELECT id, name FROM users WHERE id = $1", 1)
		}
	})
}

// BenchmarkCacheOperations benchmarks cache read/write performance
func BenchmarkCacheOperations(b *testing.B) {
	// This would require Redis to be available
	// For now, we'll benchmark the logic overhead
	b.Run("CacheKeyGeneration", func(b *testing.B) {
		userID := int64(123)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = cache.MakeUserKey(userID)
		}
	})

	b.Run("MultipleKeyGeneration", func(b *testing.B) {
		userID := int64(123)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = cache.MakeUserKey(userID)
			_ = cache.MakeCheckinsKey(userID, "2024-01-15")
			_ = cache.MakeCategoriesKey(userID)
			_ = cache.MakeTagsKey(userID)
		}
	})
}

// BenchmarkPerformanceMonitoring benchmarks the overhead of performance monitoring
func BenchmarkPerformanceMonitoring(b *testing.B) {
	db, _, err := sqlmock.New()
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	logger, _ := zap.NewDevelopment()

	b.Run("WithMonitoring", func(b *testing.B) {
		pm := NewPerformanceMonitor(sqlxDB, PerformanceConfig{
			SlowQueryThreshold: 100 * time.Millisecond,
			Enabled:            true,
		}, logger)

		ctx := context.Background()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			pm.TrackQuery(ctx, "SELECT 1", func() error {
				return nil
			})
		}
	})

	b.Run("WithoutMonitoring", func(b *testing.B) {
		pm := NewPerformanceMonitor(sqlxDB, PerformanceConfig{
			SlowQueryThreshold: 100 * time.Millisecond,
			Enabled:            false,
		}, logger)

		ctx := context.Background()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			pm.TrackQuery(ctx, "SELECT 1", func() error {
				return nil
			})
		}
	})
}

// BenchmarkPreparedStatements benchmarks prepared statement performance
func BenchmarkPreparedStatements(b *testing.B) {
	db, mock, err := sqlmock.New()
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	logger, _ := zap.NewDevelopment()
	psm := NewPreparedStatementManager(sqlxDB, logger)

	query := "SELECT \\* FROM users WHERE id = \\$1"
	mock.ExpectPrepare(query)

	err = psm.Prepare("get_user", "SELECT * FROM users WHERE id = $1")
	if err != nil {
		b.Skip("Skipping prepared statement benchmark due to sqlmock limitations")
	}

	b.Run("PreparedStatementManager", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = psm.Get("get_user")
		}
	})
}
