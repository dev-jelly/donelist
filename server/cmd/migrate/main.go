package main

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/dev-jelly/donelist/internal/config"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Create migration instance
	m, err := migrate.New(
		"file://migrations",
		cfg.Database.DSN(),
	)
	if err != nil {
		log.Fatalf("Failed to create migrate instance: %v", err)
	}
	defer m.Close()

	command := os.Args[1]

	switch command {
	case "up":
		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			log.Fatalf("Failed to run migrations: %v", err)
		}
		fmt.Println("✅ Migrations applied successfully")

	case "down":
		if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			log.Fatalf("Failed to rollback migrations: %v", err)
		}
		fmt.Println("✅ Migrations rolled back successfully")

	case "force":
		if len(os.Args) < 3 {
			fmt.Println("Error: version number required for force command")
			printUsage()
			os.Exit(1)
		}
		var version int
		_, err := fmt.Sscanf(os.Args[2], "%d", &version)
		if err != nil {
			log.Fatalf("Invalid version number: %v", err)
		}
		if err := m.Force(version); err != nil {
			log.Fatalf("Failed to force version: %v", err)
		}
		fmt.Printf("✅ Forced migration version to %d\n", version)

	case "version":
		version, dirty, err := m.Version()
		if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
			log.Fatalf("Failed to get version: %v", err)
		}
		if errors.Is(err, migrate.ErrNilVersion) {
			fmt.Println("No migrations applied yet")
		} else {
			fmt.Printf("Version: %d\n", version)
			fmt.Printf("Dirty: %v\n", dirty)
		}

	case "reset":
		// Down then Up
		fmt.Println("⚠️  Resetting database...")
		if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			log.Fatalf("Failed to rollback migrations: %v", err)
		}
		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			log.Fatalf("Failed to run migrations: %v", err)
		}
		fmt.Println("✅ Database reset complete")

	case "seed":
		fmt.Println("⚠️  Seed functionality not implemented yet")
		fmt.Println("TODO: Implement seed data logic")

	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Database Migration Tool")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  go run cmd/migrate/main.go <command>")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  up          Apply all pending migrations")
	fmt.Println("  down        Rollback all migrations")
	fmt.Println("  force <v>   Force set migration version (use with caution)")
	fmt.Println("  version     Show current migration version")
	fmt.Println("  reset       Rollback and re-apply all migrations (dangerous!)")
	fmt.Println("  seed        Seed database with test data (not implemented)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  go run cmd/migrate/main.go up")
	fmt.Println("  go run cmd/migrate/main.go version")
	fmt.Println("  go run cmd/migrate/main.go force 1")
}
