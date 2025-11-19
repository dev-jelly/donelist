package database

import (
	"database/sql"
	"testing"

	"github.com/dev-jelly/donelist/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrationRunner(t *testing.T) {
	// Setup test database
	testDB := testutil.SetupTestDB(t)
	defer testDB.Cleanup()

	// Get migrations path
	migrationsPath := "../../migrations"

	t.Run("Up migrations", func(t *testing.T) {
		runner, err := NewMigrationRunner(testDB.DB.DB, migrationsPath)
		require.NoError(t, err)
		defer runner.Close()

		// Run all migrations
		err = runner.Up()
		require.NoError(t, err)

		// Check version
		version, dirty, err := runner.Version()
		require.NoError(t, err)
		assert.False(t, dirty)
		assert.Equal(t, uint(16), version) // We have 16 migrations

		// Verify tables exist
		tables := []string{
			"users", "categories", "tags", "checkins", "checkin_tags",
			"refresh_tokens", "analytics_events", "webhooks", "webhook_deliveries",
			"subscriptions", "payments", "subscription_events", "checkin_history",
		}

		for _, table := range tables {
			var exists bool
			err := testDB.DB.QueryRow(`
				SELECT EXISTS (
					SELECT FROM information_schema.tables
					WHERE table_schema = 'public'
					AND table_name = $1
				)`, table).Scan(&exists)
			require.NoError(t, err)
			assert.True(t, exists, "Table %s should exist", table)
		}
	})

	t.Run("Down migration", func(t *testing.T) {
		runner, err := NewMigrationRunner(testDB.DB.DB, migrationsPath)
		require.NoError(t, err)
		defer runner.Close()

		// Run up first
		err = runner.Up()
		require.NoError(t, err)

		// Get initial version
		initialVersion, _, err := runner.Version()
		require.NoError(t, err)

		// Roll back one migration
		err = runner.Down()
		require.NoError(t, err)

		// Check version decreased
		newVersion, dirty, err := runner.Version()
		require.NoError(t, err)
		assert.False(t, dirty)
		assert.Equal(t, initialVersion-1, newVersion)
	})

	t.Run("DownAll migrations", func(t *testing.T) {
		runner, err := NewMigrationRunner(testDB.DB.DB, migrationsPath)
		require.NoError(t, err)
		defer runner.Close()

		// Run up first
		err = runner.Up()
		require.NoError(t, err)

		// Roll back all migrations
		err = runner.DownAll()
		require.NoError(t, err)

		// Check no tables exist (except schema_migrations)
		var count int
		err = testDB.DB.QueryRow(`
			SELECT COUNT(*) FROM information_schema.tables
			WHERE table_schema = 'public'
			AND table_name != 'schema_migrations'
		`).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count, "No tables should exist after DownAll")
	})

	t.Run("Idempotent Up", func(t *testing.T) {
		runner, err := NewMigrationRunner(testDB.DB.DB, migrationsPath)
		require.NoError(t, err)
		defer runner.Close()

		// Run migrations twice
		err = runner.Up()
		require.NoError(t, err)

		err = runner.Up()
		require.NoError(t, err) // Should not error on second run
	})

	t.Run("Force version", func(t *testing.T) {
		runner, err := NewMigrationRunner(testDB.DB.DB, migrationsPath)
		require.NoError(t, err)
		defer runner.Close()

		// Force to version 3
		err = runner.Force(3)
		require.NoError(t, err)

		version, dirty, err := runner.Version()
		require.NoError(t, err)
		assert.False(t, dirty)
		assert.Equal(t, uint(3), version)
	})
}

func TestRunMigrations(t *testing.T) {
	// Setup test database
	testDB := testutil.SetupTestDB(t)
	defer testDB.Cleanup()

	// Get migrations path
	migrationsPath := "../../migrations"

	// Run migrations using convenience function
	err := RunMigrations(testDB.URL, migrationsPath)
	require.NoError(t, err)

	// Verify migrations ran
	db, err := sql.Open("postgres", testDB.URL)
	require.NoError(t, err)
	defer db.Close()

	var version int
	err = db.QueryRow("SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1").Scan(&version)
	require.NoError(t, err)
	assert.Equal(t, 16, version)
}