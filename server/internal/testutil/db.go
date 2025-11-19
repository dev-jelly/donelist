package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestDB represents a test database instance
type TestDB struct {
	DB        *sqlx.DB
	Container testcontainers.Container
	URL       string
}

// SetupTestDB creates a new PostgreSQL container and returns a test database
func SetupTestDB(t *testing.T) *TestDB {
	ctx := context.Background()

	// Get the path to migrations
	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Dir(filepath.Dir(filepath.Dir(filename)))
	migrationsPath := filepath.Join(projectRoot, "migrations")

	// Create PostgreSQL container
	pgContainer, err := postgres.Run(ctx,
		"postgres:15-alpine",
		postgres.WithDatabase("donelist_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.WithInitScripts(filepath.Join(migrationsPath, "000001_initial_schema.up.sql")),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("Failed to start PostgreSQL container: %v", err)
	}

	// Get connection string
	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("Failed to get connection string: %v", err)
	}

	// Connect to database
	db, err := sqlx.Connect("postgres", connStr)
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	return &TestDB{
		DB:        db,
		Container: pgContainer,
		URL:       connStr,
	}
}

// Cleanup cleans up the test database
func (tdb *TestDB) Cleanup() {
	ctx := context.Background()

	// Close database connection
	if err := tdb.DB.Close(); err != nil {
		// Log error but continue cleanup
		_ = err
	}

	// Stop and remove container
	if err := tdb.Container.Terminate(ctx); err != nil {
		// Log error but continue cleanup
		_ = err
	}
}

// TearDown cleans up the test database
func (tdb *TestDB) TearDown(t *testing.T) {
	ctx := context.Background()

	// Close database connection
	if err := tdb.DB.Close(); err != nil {
		t.Logf("Failed to close database connection: %v", err)
	}

	// Stop and remove container
	if err := tdb.Container.Terminate(ctx); err != nil {
		t.Logf("Failed to terminate container: %v", err)
	}
}

// CleanTables removes all data from tables (for test isolation)
func (tdb *TestDB) CleanTables(t *testing.T) {
	tables := []string{
		"checkin_tags",
		"checkin_edit_history",
		"checkins",
		"tags",
		"categories",
		"refresh_tokens",
		"users",
	}

	for _, table := range tables {
		_, err := tdb.DB.Exec(fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table))
		if err != nil {
			t.Fatalf("Failed to truncate table %s: %v", table, err)
		}
	}
}

// ExecSQL executes a SQL statement (for test data setup)
func (tdb *TestDB) ExecSQL(t *testing.T, query string, args ...interface{}) sql.Result {
	result, err := tdb.DB.Exec(query, args...)
	if err != nil {
		t.Fatalf("Failed to execute SQL: %v\nQuery: %s", err, query)
	}
	return result
}

// MustExec executes a SQL statement and panics on error
func (tdb *TestDB) MustExec(query string, args ...interface{}) sql.Result {
	result, err := tdb.DB.Exec(query, args...)
	if err != nil {
		panic(fmt.Sprintf("Failed to execute SQL: %v\nQuery: %s", err, query))
	}
	return result
}