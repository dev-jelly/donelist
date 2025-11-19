package database

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
)

// MigrationRunner handles database migrations
type MigrationRunner struct {
	db         *sql.DB
	migrations *migrate.Migrate
}

// NewMigrationRunner creates a new migration runner
// Note: This loads migrations from file://migrations instead of embed
func NewMigrationRunner(db *sql.DB, migrationsPath string) (*MigrationRunner, error) {
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to create postgres driver: %w", err)
	}

	// Use file:// URL for migrations
	sourceURL := fmt.Sprintf("file://%s", migrationsPath)
	m, err := migrate.NewWithDatabaseInstance(sourceURL, "postgres", driver)
	if err != nil {
		return nil, fmt.Errorf("failed to create migrate instance: %w", err)
	}

	return &MigrationRunner{
		db:         db,
		migrations: m,
	}, nil
}

// Up runs all pending migrations
func (mr *MigrationRunner) Up() error {
	if err := mr.migrations.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	return nil
}

// Down rolls back one migration
func (mr *MigrationRunner) Down() error {
	if err := mr.migrations.Steps(-1); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to rollback migration: %w", err)
	}
	return nil
}

// DownAll rolls back all migrations
func (mr *MigrationRunner) DownAll() error {
	if err := mr.migrations.Down(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to rollback all migrations: %w", err)
	}
	return nil
}

// Version returns the current migration version
func (mr *MigrationRunner) Version() (uint, bool, error) {
	return mr.migrations.Version()
}

// Force sets a specific version
func (mr *MigrationRunner) Force(version int) error {
	return mr.migrations.Force(version)
}

// Close closes the migration runner
func (mr *MigrationRunner) Close() error {
	sourceErr, dbErr := mr.migrations.Close()
	if sourceErr != nil {
		return fmt.Errorf("failed to close source: %w", sourceErr)
	}
	if dbErr != nil {
		return fmt.Errorf("failed to close database: %w", dbErr)
	}
	return nil
}

// RunMigrations is a convenience function to run migrations
func RunMigrations(dbURL, migrationsPath string) error {
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	runner, err := NewMigrationRunner(db, migrationsPath)
	if err != nil {
		return fmt.Errorf("failed to create migration runner: %w", err)
	}
	defer runner.Close()

	log.Println("Running database migrations...")
	if err := runner.Up(); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	version, dirty, err := runner.Version()
	if err != nil {
		return fmt.Errorf("failed to get version: %w", err)
	}

	if dirty {
		log.Printf("Warning: Database is in dirty state at version %d", version)
	} else {
		log.Printf("Database migrated to version %d", version)
	}

	return nil
}