package search

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// Benchmark configurations
const (
	benchmarkUserID = "00000000-0000-0000-0000-000000000001"
)

func setupBenchmarkDB(b *testing.B) *sqlx.DB {
	// Use a test database for benchmarks
	dsn := "postgres://postgres:postgres@localhost:5432/donelist_test?sslmode=disable"
	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		b.Skipf("Skipping benchmark: cannot connect to test database: %v", err)
	}
	return db
}

func BenchmarkSearch_SimpleQuery(b *testing.B) {
	db := setupBenchmarkDB(b)
	defer db.Close()

	repo := NewRepository(db)
	ctx := context.Background()
	userID := uuid.MustParse(benchmarkUserID)

	filters := SearchFilters{
		Query:  "test",
		Limit:  20,
		Offset: 0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := repo.Search(ctx, userID, filters)
		if err != nil {
			b.Fatalf("Search failed: %v", err)
		}
	}
}

func BenchmarkSearch_ComplexQuery(b *testing.B) {
	db := setupBenchmarkDB(b)
	defer db.Close()

	repo := NewRepository(db)
	ctx := context.Background()
	userID := uuid.MustParse(benchmarkUserID)

	now := time.Now()
	startDate := now.AddDate(0, -1, 0)
	minDuration := 30
	maxDuration := 120

	filters := SearchFilters{
		Query:       "test workout",
		StartDate:   &startDate,
		EndDate:     &now,
		MinDuration: &minDuration,
		MaxDuration: &maxDuration,
		Limit:       20,
		Offset:      0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := repo.Search(ctx, userID, filters)
		if err != nil {
			b.Fatalf("Search failed: %v", err)
		}
	}
}

func BenchmarkSearch_WithFacets(b *testing.B) {
	db := setupBenchmarkDB(b)
	defer db.Close()

	repo := NewRepository(db)
	ctx := context.Background()
	userID := uuid.MustParse(benchmarkUserID)

	filters := SearchFilters{
		Query:  "test",
		Limit:  20,
		Offset: 0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Search
		_, _, err := repo.Search(ctx, userID, filters)
		if err != nil {
			b.Fatalf("Search failed: %v", err)
		}

		// Facets
		_, err = repo.GetFacets(ctx, userID, filters)
		if err != nil {
			b.Fatalf("GetFacets failed: %v", err)
		}
	}
}

func BenchmarkSearch_Pagination(b *testing.B) {
	db := setupBenchmarkDB(b)
	defer db.Close()

	repo := NewRepository(db)
	ctx := context.Background()
	userID := uuid.MustParse(benchmarkUserID)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		offset := (i % 10) * 20 // Paginate through first 10 pages

		filters := SearchFilters{
			Limit:  20,
			Offset: offset,
		}

		_, _, err := repo.Search(ctx, userID, filters)
		if err != nil {
			b.Fatalf("Search failed: %v", err)
		}
	}
}

func BenchmarkQueryBuilder_Build(b *testing.B) {
	userID := uuid.MustParse(benchmarkUserID)
	now := time.Now()
	startDate := now.AddDate(0, -1, 0).Format(time.RFC3339)
	endDate := now.Format(time.RFC3339)
	minDuration := 30
	maxDuration := 120
	categoryIDs := []uuid.UUID{uuid.New(), uuid.New()}
	tagIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		qb := NewQueryBuilder("SELECT * FROM checkins")
		qb.AddFullTextSearch("test workout")
		qb.AddUserFilter(userID)
		qb.AddDeletedFilter()
		qb.AddDateRange(&startDate, &endDate)
		qb.AddCategoryFilter(categoryIDs)
		qb.AddTagFilter(tagIDs)
		qb.AddDurationFilter(&minDuration, &maxDuration)
		qb.Build()
	}
}

func BenchmarkSanitizeQueryForKorean(b *testing.B) {
	testCases := []string{
		"간단한 검색",
		"complex query with English and 한글",
		"운동 했어요 today!",
		"일본어も入れて test",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		query := testCases[i%len(testCases)]
		SanitizeQueryForKorean(query)
	}
}

func BenchmarkFacets_Computation(b *testing.B) {
	db := setupBenchmarkDB(b)
	defer db.Close()

	repo := NewRepository(db)
	ctx := context.Background()
	userID := uuid.MustParse(benchmarkUserID)

	filters := SearchFilters{
		Limit: 20,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := repo.GetFacets(ctx, userID, filters)
		if err != nil {
			b.Fatalf("GetFacets failed: %v", err)
		}
	}
}

// Benchmark concurrent searches
func BenchmarkSearch_Concurrent(b *testing.B) {
	db := setupBenchmarkDB(b)
	defer db.Close()

	repo := NewRepository(db)
	ctx := context.Background()
	userID := uuid.MustParse(benchmarkUserID)

	b.RunParallel(func(pb *testing.PB) {
		queries := []string{"test", "workout", "study", "meeting", "project"}
		i := 0
		for pb.Next() {
			query := queries[i%len(queries)]
			filters := SearchFilters{
				Query:  query,
				Limit:  20,
				Offset: 0,
			}

			_, _, err := repo.Search(ctx, userID, filters)
			if err != nil {
				b.Fatalf("Search failed: %v", err)
			}
			i++
		}
	})
}

// LoadTest simulates realistic search load
func LoadTest(t *testing.T, duration time.Duration, concurrency int) {
	db := setupBenchmarkDB(&testing.B{})
	defer db.Close()

	repo := NewRepository(db)
	ctx := context.Background()
	userID := uuid.MustParse(benchmarkUserID)

	queries := []string{
		"운동",
		"study",
		"meeting",
		"project work",
		"reading book",
		"코딩 테스트",
		"식사",
		"휴식",
	}

	results := make(chan time.Duration, concurrency*100)
	errors := make(chan error, concurrency*100)
	done := make(chan bool)

	// Start workers
	for i := 0; i < concurrency; i++ {
		go func(workerID int) {
			for {
				select {
				case <-done:
					return
				default:
					start := time.Now()

					// Random query
					query := queries[rand.Intn(len(queries))]

					// Random filters
					var startDate, endDate *time.Time
					if rand.Float32() < 0.3 {
						now := time.Now()
						sd := now.AddDate(0, 0, -rand.Intn(90))
						startDate = &sd
					}

					filters := SearchFilters{
						Query:     query,
						StartDate: startDate,
						EndDate:   endDate,
						Limit:     20,
						Offset:    0,
					}

					_, _, err := repo.Search(ctx, userID, filters)
					elapsed := time.Since(start)

					if err != nil {
						errors <- err
					} else {
						results <- elapsed
					}

					// Random delay between requests
					time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)
				}
			}
		}(i)
	}

	// Let it run for specified duration
	time.Sleep(duration)
	close(done)

	// Collect results
	time.Sleep(100 * time.Millisecond) // Let remaining requests finish

	var totalDuration time.Duration
	var count int
	var errorCount int

	close(results)
	close(errors)

	for d := range results {
		totalDuration += d
		count++
	}

	for range errors {
		errorCount++
	}

	if count > 0 {
		avgLatency := totalDuration / time.Duration(count)
		fmt.Printf("\nLoad Test Results:\n")
		fmt.Printf("Duration: %s\n", duration)
		fmt.Printf("Concurrency: %d\n", concurrency)
		fmt.Printf("Total Requests: %d\n", count)
		fmt.Printf("Errors: %d\n", errorCount)
		fmt.Printf("Requests/sec: %.2f\n", float64(count)/duration.Seconds())
		fmt.Printf("Avg Latency: %s\n", avgLatency)
		fmt.Printf("Success Rate: %.2f%%\n", float64(count-errorCount)/float64(count)*100)
	}

	require.Equal(t, 0, errorCount, "Should have no errors")
}

func TestLoadTest_Light(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}
	LoadTest(t, 5*time.Second, 5)
}

func TestLoadTest_Medium(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}
	LoadTest(t, 10*time.Second, 20)
}

func TestLoadTest_Heavy(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}
	LoadTest(t, 30*time.Second, 50)
}
