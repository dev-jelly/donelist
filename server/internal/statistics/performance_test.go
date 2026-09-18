package statistics_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/dev-jelly/donelist/internal/statistics"
	"github.com/dev-jelly/donelist/pkg/database"
	"github.com/dev-jelly/donelist/pkg/logger"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// BenchmarkWeeklyStatistics benchmarks the weekly statistics generation
func BenchmarkWeeklyStatistics(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	db, log := setupBenchDatabase(b)
	defer database.Close(db, log)

	repo := statistics.NewRepository(db)
	service := statistics.NewService(repo, log)

	userID := setupBenchUser(b, db)
	baseDate := time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC)
	setupBenchCheckins(b, db, userID, baseDate, 100) // 100 check-ins

	ctx := context.Background()
	opts := statistics.GetWeeklyOptions{
		UserID:       userID,
		Date:         baseDate.AddDate(0, 0, 2),
		WeekStartDay: statistics.WeekStartMonday,
		Timezone:     "UTC",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := service.GetWeeklyStatistics(ctx, opts)
		if err != nil {
			b.Fatalf("GetWeeklyStatistics failed: %v", err)
		}
	}
}

// BenchmarkWeeklyStatisticsWithCache benchmarks with caching enabled
func BenchmarkWeeklyStatisticsWithCache(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	db, log := setupBenchDatabase(b)
	defer database.Close(db, log)

	mr, err := miniredis.Run()
	require.NoError(b, err)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer redisClient.Close()

	repo := statistics.NewRepository(db)
	cache := statistics.NewCacheService(redisClient, log)
	service := statistics.NewService(repo, log)
	service.SetCache(cache)

	userID := setupBenchUser(b, db)
	baseDate := time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC)
	setupBenchCheckins(b, db, userID, baseDate, 100)

	ctx := context.Background()
	opts := statistics.GetWeeklyOptions{
		UserID:       userID,
		Date:         baseDate.AddDate(0, 0, 2),
		WeekStartDay: statistics.WeekStartMonday,
		Timezone:     "UTC",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := service.GetWeeklyStatistics(ctx, opts)
		if err != nil {
			b.Fatalf("GetWeeklyStatistics failed: %v", err)
		}
	}
}

// BenchmarkRepositoryQueries benchmarks individual repository operations
func BenchmarkRepositoryQueries(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	db, log := setupBenchDatabase(b)
	defer database.Close(db, log)

	repo := statistics.NewRepository(db)
	userID := setupBenchUser(b, db)
	baseDate := time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC)
	setupBenchCheckins(b, db, userID, baseDate, 100)

	ctx := context.Background()
	startDate := baseDate
	endDate := baseDate.AddDate(0, 0, 7)

	b.Run("GetDailyAggregates", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := repo.GetDailyAggregates(ctx, userID, startDate, endDate)
			if err != nil {
				b.Fatalf("GetDailyAggregates failed: %v", err)
			}
		}
	})

	b.Run("GetCategoryAggregates", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := repo.GetCategoryAggregates(ctx, userID, startDate, endDate)
			if err != nil {
				b.Fatalf("GetCategoryAggregates failed: %v", err)
			}
		}
	})

	b.Run("GetTimeOfDayAggregates", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := repo.GetTimeOfDayAggregates(ctx, userID, startDate, endDate)
			if err != nil {
				b.Fatalf("GetTimeOfDayAggregates failed: %v", err)
			}
		}
	})

	b.Run("GetDayOfWeekAggregates", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := repo.GetDayOfWeekAggregates(ctx, userID, startDate, endDate)
			if err != nil {
				b.Fatalf("GetDayOfWeekAggregates failed: %v", err)
			}
		}
	})

	b.Run("GetWeekTotal", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _, err := repo.GetWeekTotal(ctx, userID, startDate, endDate)
			if err != nil {
				b.Fatalf("GetWeekTotal failed: %v", err)
			}
		}
	})

	b.Run("GetStreakData", func(b *testing.B) {
		lookbackStart := startDate.AddDate(0, 0, -90)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := repo.GetStreakData(ctx, userID, lookbackStart, endDate)
			if err != nil {
				b.Fatalf("GetStreakData failed: %v", err)
			}
		}
	})
}

// BenchmarkCacheOperations benchmarks cache operations
func BenchmarkCacheOperations(b *testing.B) {
	mr, err := miniredis.Run()
	require.NoError(b, err)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer redisClient.Close()

	log, err := logger.New("bench", "error")
	require.NoError(b, err)

	cache := statistics.NewCacheService(redisClient, log)
	ctx := context.Background()

	stats := &statistics.WeeklyStatistics{
		Year:         2024,
		WeekNumber:   3,
		StartDate:    "2024-01-15",
		EndDate:      "2024-01-21",
		WeekStartDay: statistics.WeekStartMonday,
		Timezone:     "UTC",
		Summary: &statistics.WeeklySummary{
			TotalCheckins:    100,
			TotalMinutes:     5000,
			DaysWithCheckins: 7,
			AveragePerDay:    14.29,
			CompletionRate:   100.0,
		},
		GeneratedAt: time.Now().UTC(),
	}

	cacheKey := "bench-user:2024-01-15:monday:UTC"

	b.Run("SetCache", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			err := cache.SetWeeklyStats(ctx, cacheKey, stats)
			if err != nil {
				b.Fatalf("SetWeeklyStats failed: %v", err)
			}
		}
	})

	// Set initial value for get test
	_ = cache.SetWeeklyStats(ctx, cacheKey, stats)

	b.Run("GetCache", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := cache.GetWeeklyStats(ctx, cacheKey)
			if err != nil {
				b.Fatalf("GetWeeklyStats failed: %v", err)
			}
		}
	})
}

// TestPerformanceMetrics measures actual performance metrics
func TestPerformanceMetrics(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	db, log := setupBenchDatabase(t)
	defer database.Close(db, log)

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer redisClient.Close()

	repo := statistics.NewRepository(db)
	cache := statistics.NewCacheService(redisClient, log)
	service := statistics.NewService(repo, log)
	service.SetCache(cache)

	userID := setupBenchUser(t, db)
	baseDate := time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC)

	ctx := context.Background()
	opts := statistics.GetWeeklyOptions{
		UserID:       userID,
		Date:         baseDate.AddDate(0, 0, 2),
		WeekStartDay: statistics.WeekStartMonday,
		Timezone:     "UTC",
	}

	testCases := []struct {
		name         string
		checkinCount int
		targetMs     int64 // Target response time in milliseconds
	}{
		{"SmallDataset", 10, 50},
		{"MediumDataset", 100, 100},
		{"LargeDataset", 500, 200},
		{"ExtraLargeDataset", 1000, 300},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup data
			setupBenchCheckins(t, db, userID, baseDate, tc.checkinCount)

			// Clear cache
			err := cache.InvalidateUserStats(ctx, userID.String())
			require.NoError(t, err)

			// Measure first request (cache miss)
			start := time.Now()
			stats, err := service.GetWeeklyStatistics(ctx, opts)
			duration := time.Since(start)
			require.NoError(t, err)
			require.NotNil(t, stats)

			cacheMissMs := duration.Milliseconds()
			t.Logf("Cache miss: %dms (target: <%dms)", cacheMissMs, tc.targetMs)

			if cacheMissMs > tc.targetMs {
				t.Logf("WARNING: Cache miss exceeded target (%dms > %dms)", cacheMissMs, tc.targetMs)
			}

			// Measure second request (cache hit)
			start = time.Now()
			stats, err = service.GetWeeklyStatistics(ctx, opts)
			duration = time.Since(start)
			require.NoError(t, err)
			require.NotNil(t, stats)

			cacheHitMs := duration.Milliseconds()
			t.Logf("Cache hit: %dms", cacheHitMs)

			// Cache hit should be much faster
			speedup := float64(cacheMissMs) / float64(cacheHitMs)
			t.Logf("Speedup: %.2fx", speedup)

			// Cleanup
			cleanupBenchData(t, db, userID)
		})
	}
}

// TestQueryPerformance measures individual query performance
func TestQueryPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	db, log := setupBenchDatabase(t)
	defer database.Close(db, log)

	repo := statistics.NewRepository(db)
	userID := setupBenchUser(t, db)
	baseDate := time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC)
	setupBenchCheckins(t, db, userID, baseDate, 500)

	ctx := context.Background()
	startDate := baseDate
	endDate := baseDate.AddDate(0, 0, 7)

	queries := []struct {
		name     string
		run      func() error
		targetMs int64
	}{
		{
			name: "GetDailyAggregates",
			run: func() error {
				_, err := repo.GetDailyAggregates(ctx, userID, startDate, endDate)
				return err
			},
			targetMs: 20,
		},
		{
			name: "GetCategoryAggregates",
			run: func() error {
				_, err := repo.GetCategoryAggregates(ctx, userID, startDate, endDate)
				return err
			},
			targetMs: 20,
		},
		{
			name: "GetTimeOfDayAggregates",
			run: func() error {
				_, err := repo.GetTimeOfDayAggregates(ctx, userID, startDate, endDate)
				return err
			},
			targetMs: 20,
		},
		{
			name: "GetDayOfWeekAggregates",
			run: func() error {
				_, err := repo.GetDayOfWeekAggregates(ctx, userID, startDate, endDate)
				return err
			},
			targetMs: 20,
		},
		{
			name: "GetWeekTotal",
			run: func() error {
				_, _, err := repo.GetWeekTotal(ctx, userID, startDate, endDate)
				return err
			},
			targetMs: 15,
		},
		{
			name: "GetStreakData",
			run: func() error {
				lookbackStart := startDate.AddDate(0, 0, -90)
				_, err := repo.GetStreakData(ctx, userID, lookbackStart, endDate)
				return err
			},
			targetMs: 50,
		},
	}

	for _, q := range queries {
		t.Run(q.name, func(t *testing.T) {
			// Warmup
			err := q.run()
			require.NoError(t, err)

			// Measure
			const iterations = 10
			var totalDuration time.Duration

			for i := 0; i < iterations; i++ {
				start := time.Now()
				err := q.run()
				totalDuration += time.Since(start)
				require.NoError(t, err)
			}

			avgMs := totalDuration.Milliseconds() / iterations
			t.Logf("Average: %dms (target: <%dms)", avgMs, q.targetMs)

			if avgMs > q.targetMs {
				t.Logf("WARNING: Query exceeded target (%dms > %dms)", avgMs, q.targetMs)
			}
		})
	}

	cleanupBenchData(t, db, userID)
}

// Helper functions for benchmarks

func setupBenchDatabase(tb testing.TB) (*sqlx.DB, *zap.Logger) {
	cfg := database.PostgresConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "postgres",
		Password: "postgres",
		Database: "donelist_test",
		SSLMode:  "disable",
	}

	log, err := logger.New("bench", "error")
	require.NoError(tb, err)

	db, err := database.NewPostgres(cfg, log)
	if err != nil {
		tb.Skipf("Could not connect to test database: %v", err)
	}

	return db, log
}

func setupBenchUser(tb testing.TB, db *sqlx.DB) uuid.UUID {
	userID := uuid.New()
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `
		INSERT INTO users (id, email, username, password_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, userID, fmt.Sprintf("bench-%s@example.com", userID.String()), "benchuser", "hashedpassword")
	require.NoError(tb, err)

	return userID
}

func setupBenchCheckins(tb testing.TB, db *sqlx.DB, userID uuid.UUID, baseDate time.Time, count int) {
	ctx := context.Background()

	// Distribute check-ins across the week
	for i := 0; i < count; i++ {
		dayOffset := i % 7
		hour := (i % 16) + 6 // 6 AM to 10 PM
		minute := i % 60

		checkinDate := baseDate.AddDate(0, 0, dayOffset).
			Add(time.Duration(hour) * time.Hour).
			Add(time.Duration(minute) * time.Minute)

		_, err := db.ExecContext(ctx, `
			INSERT INTO checkins (id, user_id, checkin_time, duration_minutes, description, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		`, uuid.New(), userID, checkinDate, 30, fmt.Sprintf("Bench checkin %d", i))
		require.NoError(tb, err)
	}
}

func cleanupBenchData(tb testing.TB, db *sqlx.DB, userID uuid.UUID) {
	ctx := context.Background()
	_, _ = db.ExecContext(ctx, "DELETE FROM checkins WHERE user_id = $1", userID)
	_, _ = db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", userID)
}
