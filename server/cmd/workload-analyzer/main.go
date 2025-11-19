package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/dev-jelly/donelist/internal/config"
	"github.com/dev-jelly/donelist/internal/performance"
	"github.com/dev-jelly/donelist/pkg/database"
)

var (
	logger     *zap.Logger
	outputJSON bool
	outputFile string
)

func main() {
	var err error
	logger, err = zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	rootCmd := &cobra.Command{
		Use:   "workload-analyzer",
		Short: "PostgreSQL Workload Analysis and Index Recommendation Tool",
		Long:  "Analyze database workload patterns and generate index recommendations",
	}

	rootCmd.PersistentFlags().BoolVar(&outputJSON, "json", false, "Output results in JSON format")
	rootCmd.PersistentFlags().StringVar(&outputFile, "output", "", "Write output to file instead of stdout")

	rootCmd.AddCommand(
		analyzeCmd(),
		unusedIndexesCmd(),
		missingIndexesCmd(),
		indexCandidatesCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		logger.Error("Command execution failed", zap.Error(err))
		os.Exit(1)
	}
}

func analyzeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "analyze",
		Short: "Run comprehensive workload analysis",
		Long:  "Analyze query patterns, identify slow queries, and generate index recommendations",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			analyzer, cleanup, err := createAnalyzer()
			if err != nil {
				return err
			}
			defer cleanup()

			fmt.Println("Analyzing workload... (this may take a moment)")
			report, err := analyzer.AnalyzeWorkload(ctx)
			if err != nil {
				return fmt.Errorf("failed to analyze workload: %w", err)
			}

			if outputJSON {
				return outputReportJSON(report)
			}

			printReport(report)
			return nil
		},
	}
}

func unusedIndexesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unused-indexes",
		Short: "Show indexes that are rarely or never used",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			analyzer, cleanup, err := createAnalyzer()
			if err != nil {
				return err
			}
			defer cleanup()

			indexes, err := analyzer.GetUnusedIndexes(ctx)
			if err != nil {
				return fmt.Errorf("failed to get unused indexes: %w", err)
			}

			if outputJSON {
				data, _ := json.MarshalIndent(indexes, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "SCHEMA\tTABLE\tINDEX\tSCANS\tSIZE")
			fmt.Fprintln(w, "------\t-----\t-----\t-----\t----")

			for _, idx := range indexes {
				fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n",
					idx.SchemaName, idx.TableName, idx.IndexName, idx.IndexScans, idx.IndexSize)
			}

			w.Flush()
			fmt.Printf("\nTotal unused indexes: %d\n", len(indexes))
			fmt.Println("\nConsider dropping indexes with 0 scans to save storage and improve write performance.")
			fmt.Println("Use: DROP INDEX CONCURRENTLY <index_name>;")

			return nil
		},
	}
}

func missingIndexesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "missing-indexes",
		Short: "Identify tables that might benefit from indexes",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			analyzer, cleanup, err := createAnalyzer()
			if err != nil {
				return err
			}
			defer cleanup()

			tables, err := analyzer.GetTablesNeedingIndexes(ctx)
			if err != nil {
				return fmt.Errorf("failed to get tables needing indexes: %w", err)
			}

			if outputJSON {
				data, _ := json.MarshalIndent(tables, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "TABLE\tSEQ SCANS\tLIVE ROWS\tAVG ROWS/SCAN\tIMPACT")
			fmt.Fprintln(w, "-----\t---------\t---------\t-------------\t------")

			for _, tbl := range tables {
				impact := "Low"
				score := tbl.SequentialScans * tbl.AvgRowsPerScan
				if score > 1000000 {
					impact = "HIGH"
				} else if score > 100000 {
					impact = "Medium"
				}

				fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%s\n",
					tbl.TableName, tbl.SequentialScans, tbl.LiveTuples, tbl.AvgRowsPerScan, impact)
			}

			w.Flush()
			fmt.Printf("\nTotal tables needing indexes: %d\n", len(tables))
			fmt.Println("\nTables with HIGH impact should be prioritized for index creation.")

			return nil
		},
	}
}

func indexCandidatesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "index-candidates",
		Short: "Generate index creation recommendations",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			analyzer, cleanup, err := createAnalyzer()
			if err != nil {
				return err
			}
			defer cleanup()

			candidates, err := analyzer.GenerateIndexCandidates(ctx)
			if err != nil {
				return fmt.Errorf("failed to generate index candidates: %w", err)
			}

			if outputJSON {
				data, _ := json.MarshalIndent(candidates, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			// Group by priority
			byPriority := make(map[int][]performance.IndexCandidate)
			for _, c := range candidates {
				byPriority[c.Priority] = append(byPriority[c.Priority], c)
			}

			// Print by priority
			for priority := 1; priority <= 4; priority++ {
				items := byPriority[priority]
				if len(items) == 0 {
					continue
				}

				priorityLabel := "CRITICAL"
				switch priority {
				case 2:
					priorityLabel = "HIGH"
				case 3:
					priorityLabel = "MEDIUM"
				case 4:
					priorityLabel = "LOW"
				}

				fmt.Printf("\n=== %s PRIORITY ===\n\n", priorityLabel)

				for i, c := range items {
					fmt.Printf("%d. Table: %s\n", i+1, c.TableName)
					fmt.Printf("   Columns: %v\n", c.ColumnNames)
					fmt.Printf("   Type: %s\n", c.IndexType)
					fmt.Printf("   Reason: %s\n", c.Reason)
					fmt.Printf("   Estimated Gain: %s\n", c.EstimatedGain)
					fmt.Printf("   Table Stats: %d seq scans, %d rows\n", c.SequentialScans, c.TableRows)
					fmt.Printf("   SQL: %s\n\n", c.CreateSQL)
				}
			}

			fmt.Printf("Total index candidates: %d\n", len(candidates))
			fmt.Println("\nNote: Use CREATE INDEX CONCURRENTLY to avoid locking the table during index creation.")

			return nil
		},
	}
}

func printReport(report *performance.WorkloadReport) {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("WORKLOAD ANALYSIS REPORT")
	fmt.Println(strings.Repeat("=", 80))

	// Top slow queries
	fmt.Println("\n--- TOP SLOW QUERIES ---")
	if len(report.TopQueries) == 0 {
		fmt.Println("No slow queries found.")
	} else {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "CALLS\tAVG (ms)\tTOTAL (ms)\tQUERY")
		fmt.Fprintln(w, "-----\t--------\t---------\t-----")
		for i, q := range report.TopQueries {
			if i >= 10 {
				break
			}
			preview := q.QueryPreview
			if len(preview) > 60 {
				preview = preview[:57] + "..."
			}
			fmt.Fprintf(w, "%d\t%.2f\t%.2f\t%s\n", q.Calls, q.AvgTimeMS, q.TotalTimeMS, preview)
		}
		w.Flush()
	}

	// Unused indexes
	fmt.Println("\n--- UNUSED INDEXES ---")
	if len(report.UnusedIndexes) == 0 {
		fmt.Println("No unused indexes found.")
	} else {
		fmt.Printf("Found %d unused or rarely used indexes\n", len(report.UnusedIndexes))
		for i, idx := range report.UnusedIndexes {
			if i >= 5 {
				fmt.Printf("... and %d more\n", len(report.UnusedIndexes)-5)
				break
			}
			fmt.Printf("  - %s.%s (%s, %d scans)\n", idx.TableName, idx.IndexName, idx.IndexSize, idx.IndexScans)
		}
	}

	// Missing indexes
	fmt.Println("\n--- TABLES NEEDING INDEXES ---")
	if len(report.MissingIndexes) == 0 {
		fmt.Println("No obvious missing indexes found.")
	} else {
		for i, tbl := range report.MissingIndexes {
			if i >= 5 {
				break
			}
			fmt.Printf("  - %s: %d seq scans, %d rows, avg %d rows/scan\n",
				tbl.TableName, tbl.SequentialScans, tbl.LiveTuples, tbl.AvgRowsPerScan)
		}
	}

	// Index candidates
	fmt.Println("\n--- INDEX CANDIDATES ---")
	if len(report.IndexCandidates) == 0 {
		fmt.Println("No index candidates identified.")
	} else {
		highPriority := 0
		for _, c := range report.IndexCandidates {
			if c.Priority == 1 {
				highPriority++
			}
		}
		fmt.Printf("Found %d index candidates (%d high priority)\n", len(report.IndexCandidates), highPriority)
		fmt.Println("\nRun 'workload-analyzer index-candidates' for detailed recommendations")
	}

	// Recommendations
	fmt.Println("\n--- RECOMMENDATIONS ---")
	if len(report.Recommendations) == 0 {
		fmt.Println("No specific recommendations at this time.")
	} else {
		for i, rec := range report.Recommendations {
			fmt.Printf("%d. %s\n", i+1, rec)
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 80))
}

func outputReportJSON(report *performance.WorkloadReport) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}

	if outputFile != "" {
		err = os.WriteFile(outputFile, data, 0644)
		if err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}
		fmt.Printf("Report written to %s\n", outputFile)
	} else {
		fmt.Println(string(data))
	}

	return nil
}

func createAnalyzer() (*performance.WorkloadAnalyzer, func(), error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load config: %w", err)
	}

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

	analyzer := performance.NewWorkloadAnalyzer(db)
	cleanup := func() {
		database.Close(db, logger)
	}

	return analyzer, cleanup, nil
}
