package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/dev-jelly/donelist/internal/config"
	"github.com/dev-jelly/donelist/internal/performance"
	"github.com/dev-jelly/donelist/pkg/database"
)

var (
	cfgFile string
	logger  *zap.Logger
)

func main() {
	// Initialize logger
	var err error
	logger, err = zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	rootCmd := &cobra.Command{
		Use:   "perf-monitor",
		Short: "PostgreSQL Performance Monitoring Tool",
		Long:  "CLI tool for monitoring PostgreSQL performance metrics and baselines",
	}

	rootCmd.AddCommand(
		healthCmd(),
		statsCmd(),
		slowQueriesCmd(),
		tableStatsCmd(),
		indexStatsCmd(),
		baselineCmd(),
		captureCmd(),
		checkExtCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		logger.Error("Command execution failed", zap.Error(err))
		os.Exit(1)
	}
}

func healthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Check database performance health",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			monitor, cleanup, err := createMonitor()
			if err != nil {
				return err
			}
			defer cleanup()

			health, err := monitor.GetPerformanceHealth(ctx)
			if err != nil {
				return fmt.Errorf("failed to get health: %w", err)
			}

			fmt.Printf("Performance Health Status\n")
			fmt.Printf("=========================\n")
			fmt.Printf("Timestamp:          %s\n", health.Timestamp.Format(time.RFC3339))
			fmt.Printf("Healthy:            %v\n", health.Healthy)
			fmt.Printf("Cache Hit Ratio:    %.2f%%\n", health.CacheHitRatio)
			fmt.Printf("Active Connections: %d\n", health.ActiveConnections)
			fmt.Printf("Deadlocks:          %d\n", health.Deadlocks)

			if len(health.Issues) > 0 {
				fmt.Printf("\nIssues:\n")
				for _, issue := range health.Issues {
					fmt.Printf("  - %s\n", issue)
				}
			}

			return nil
		},
	}
}

func statsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Show current database statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			monitor, cleanup, err := createMonitor()
			if err != nil {
				return err
			}
			defer cleanup()

			stats, err := monitor.GetCurrentDBStats(ctx)
			if err != nil {
				return fmt.Errorf("failed to get stats: %w", err)
			}

			fmt.Printf("Current Database Statistics\n")
			fmt.Printf("===========================\n")
			fmt.Printf("Active Connections:       %d\n", stats.ActiveConnections)
			fmt.Printf("Cache Hit Ratio:          %.2f%%\n", stats.CacheHitRatioPct)
			fmt.Printf("Transactions Committed:   %d\n", stats.TransactionsCommitted)
			fmt.Printf("Transactions Rolled Back: %d\n", stats.TransactionsRolledBack)
			fmt.Printf("Blocks Read (Disk):       %d\n", stats.BlocksReadFromDisk)
			fmt.Printf("Blocks Read (Cache):      %d\n", stats.BlocksReadFromCache)
			fmt.Printf("Rows Inserted:            %d\n", stats.RowsInserted)
			fmt.Printf("Rows Updated:             %d\n", stats.RowsUpdated)
			fmt.Printf("Rows Deleted:             %d\n", stats.RowsDeleted)
			fmt.Printf("Deadlocks:                %d\n", stats.Deadlocks)
			fmt.Printf("Temp Files:               %d\n", stats.TempFiles)

			return nil
		},
	}
}

func slowQueriesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "slow-queries",
		Short: "Show top slow queries",
		RunE: func(cmd *cobra.Command, args []string) error {
			limit, _ := cmd.Flags().GetInt("limit")

			ctx := context.Background()
			monitor, cleanup, err := createMonitor()
			if err != nil {
				return err
			}
			defer cleanup()

			queries, err := monitor.GetTopSlowQueries(ctx, limit)
			if err != nil {
				return fmt.Errorf("failed to get slow queries: %w", err)
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "CALLS\tAVG TIME (ms)\tTOTAL TIME (ms)\tQUERY PREVIEW")
			fmt.Fprintln(w, "-----\t-------------\t---------------\t-------------")

			for _, q := range queries {
				fmt.Fprintf(w, "%d\t%.2f\t%.2f\t%s\n",
					q.Calls, q.AvgTimeMS, q.TotalTimeMS, q.QueryPreview)
			}

			w.Flush()
			return nil
		},
	}

	cmd.Flags().IntP("limit", "l", 20, "Number of queries to show")
	return cmd
}

func tableStatsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "table-stats",
		Short: "Show table statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			monitor, cleanup, err := createMonitor()
			if err != nil {
				return err
			}
			defer cleanup()

			stats, err := monitor.GetTableStats(ctx)
			if err != nil {
				return fmt.Errorf("failed to get table stats: %w", err)
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "TABLE\tSEQ SCANS\tIDX SCANS\tLIVE ROWS\tDEAD ROWS\tDEAD %")
			fmt.Fprintln(w, "-----\t---------\t---------\t---------\t---------\t------")

			for _, s := range stats {
				fmt.Fprintf(w, "%s.%s\t%d\t%d\t%d\t%d\t%.2f%%\n",
					s.SchemaName, s.TableName, s.SequentialScans, s.IndexScans,
					s.LiveRows, s.DeadRows, s.DeadRowPct)
			}

			w.Flush()
			return nil
		},
	}
}

func indexStatsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "index-stats",
		Short: "Show index usage statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			monitor, cleanup, err := createMonitor()
			if err != nil {
				return err
			}
			defer cleanup()

			stats, err := monitor.GetIndexUsageStats(ctx)
			if err != nil {
				return fmt.Errorf("failed to get index stats: %w", err)
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "TABLE\tINDEX\tSCANS\tSIZE")
			fmt.Fprintln(w, "-----\t-----\t-----\t----")

			for _, s := range stats {
				fmt.Fprintf(w, "%s\t%s\t%d\t%s\n",
					s.TableName, s.IndexName, s.IndexScans, s.IndexSize)
			}

			w.Flush()
			return nil
		},
	}
}

func baselineCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "baseline",
		Short: "Show baseline metrics",
		RunE: func(cmd *cobra.Command, args []string) error {
			days, _ := cmd.Flags().GetInt("days")

			ctx := context.Background()
			monitor, cleanup, err := createMonitor()
			if err != nil {
				return err
			}
			defer cleanup()

			metrics, err := monitor.GetBaselineMetrics(ctx, days)
			if err != nil {
				return fmt.Errorf("failed to get baseline metrics: %w", err)
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "DATE\tCACHE HIT %\tCONNS\tTOTAL QUERIES\tSLOW QUERIES")
			fmt.Fprintln(w, "----\t----------\t-----\t-------------\t------------")

			for _, m := range metrics {
				fmt.Fprintf(w, "%s\t%.4f\t%d\t%d\t%d\n",
					m.MeasurementDate.Format("2006-01-02"),
					m.CacheHitRatio, m.ActiveConnectionsAvg,
					m.TotalQueries, m.SlowQueriesCount)
			}

			w.Flush()
			return nil
		},
	}

	cmd.Flags().IntP("days", "d", 7, "Number of days to show")
	return cmd
}

func captureCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "capture",
		Short: "Capture metrics",
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "baseline",
			Short: "Capture baseline metrics",
			RunE: func(cmd *cobra.Command, args []string) error {
				ctx := context.Background()
				monitor, cleanup, err := createMonitor()
				if err != nil {
					return err
				}
				defer cleanup()

				if err := monitor.CaptureBaselineMetrics(ctx); err != nil {
					return fmt.Errorf("failed to capture baseline: %w", err)
				}

				fmt.Println("Baseline metrics captured successfully")
				return nil
			},
		},
		&cobra.Command{
			Use:   "snapshot",
			Short: "Capture query stats snapshot",
			RunE: func(cmd *cobra.Command, args []string) error {
				ctx := context.Background()
				monitor, cleanup, err := createMonitor()
				if err != nil {
					return err
				}
				defer cleanup()

				if err := monitor.CaptureQueryStatsSnapshot(ctx); err != nil {
					return fmt.Errorf("failed to capture snapshot: %w", err)
				}

				fmt.Println("Query stats snapshot captured successfully")
				return nil
			},
		},
	)

	return cmd
}

func checkExtCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check-ext",
		Short: "Check if pg_stat_statements is enabled",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			monitor, cleanup, err := createMonitor()
			if err != nil {
				return err
			}
			defer cleanup()

			enabled, err := monitor.CheckPgStatStatementsEnabled(ctx)
			if err != nil {
				return fmt.Errorf("failed to check extension: %w", err)
			}

			if enabled {
				fmt.Println("✓ pg_stat_statements is enabled")
			} else {
				fmt.Println("✗ pg_stat_statements is NOT enabled")
				fmt.Println("\nTo enable, add to postgresql.conf:")
				fmt.Println("  shared_preload_libraries = 'pg_stat_statements'")
				fmt.Println("\nThen restart PostgreSQL and run:")
				fmt.Println("  CREATE EXTENSION pg_stat_statements;")
			}

			return nil
		},
	}
}

func createMonitor() (*performance.Monitor, func(), error) {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Connect to database
	dbCfg := database.PostgresConfig{
		Host:            cfg.Database.Host,
		Port:            cfg.Database.Port,
		User:            cfg.Database.User,
		Password:        cfg.Database.Password,
		Database:        cfg.Database.Name,
		SSLMode:         cfg.Database.SSLMode,
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
	}

	db, err := database.NewPostgres(dbCfg, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	monitor := performance.NewMonitor(db)
	cleanup := func() {
		database.Close(db, logger)
	}

	return monitor, cleanup, nil
}
