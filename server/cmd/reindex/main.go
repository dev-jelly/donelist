package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/dev-jelly/donelist/internal/config"
	"github.com/dev-jelly/donelist/internal/search"
	"github.com/dev-jelly/donelist/pkg/database"
	"github.com/dev-jelly/donelist/pkg/logger"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func main() {
	// Command-line flags
	var (
		batchSize = flag.Int("batch-size", 1000, "Number of documents to process per batch")
		userID    = flag.String("user-id", "", "Optional: Reindex only for specific user ID")
		startDate = flag.String("start-date", "", "Optional: Reindex from this date (RFC3339 format)")
		endDate   = flag.String("end-date", "", "Optional: Reindex until this date (RFC3339 format)")
		dryRun    = flag.Bool("dry-run", false, "Perform dry run without modifying data")
		verbose   = flag.Bool("verbose", false, "Enable verbose logging")
		verify    = flag.Bool("verify", false, "Verify index integrity instead of reindexing")
		compact   = flag.Bool("compact", false, "Compact/optimize index after reindexing")
	)

	flag.Parse()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log, err := logger.New(cfg.App.Env, cfg.App.LogLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync(log)

	// Initialize database
	pgConfig := database.PostgresConfig{
		Host:            cfg.Database.Host,
		Port:            cfg.Database.Port,
		User:            cfg.Database.User,
		Password:        cfg.Database.Password,
		Database:        cfg.Database.Name,
		SSLMode:         cfg.Database.SSLMode,
		MaxOpenConns:    cfg.Database.MaxOpenConns,
		MaxIdleConns:    cfg.Database.MaxIdleConns,
		ConnMaxLifetime: cfg.Database.ConnMaxLifetime,
		ConnMaxIdleTime: cfg.Database.ConnMaxIdleTime,
	}

	db, err := database.NewPostgres(pgConfig, log)
	if err != nil {
		log.Fatal("Failed to connect to PostgreSQL", zap.Error(err))
	}
	defer database.Close(db, log)

	// Create reindexer
	reindexer := search.NewReindexer(db, log)

	ctx := context.Background()

	// Verify mode
	if *verify {
		log.Info("Verifying search index integrity")
		result, err := reindexer.VerifyIndex(ctx)
		if err != nil {
			log.Fatal("Verification failed", zap.Error(err))
		}

		fmt.Printf("\n=== Index Verification Results ===\n")
		fmt.Printf("Total Documents:  %d\n", result.TotalDocuments)
		fmt.Printf("Missing Vectors:  %d\n", result.MissingVectors)
		fmt.Printf("Empty Vectors:    %d\n", result.EmptyVectors)
		fmt.Printf("Index Healthy:    %t\n", result.Healthy)
		fmt.Printf("Duration:         %s\n", result.Duration)

		if !result.Healthy {
			fmt.Printf("\nIndex has issues. Run reindex to fix.\n")
			os.Exit(1)
		}

		fmt.Printf("\nIndex is healthy!\n")
		os.Exit(0)
	}

	// Build reindex options
	opts := search.DefaultReindexOptions()
	opts.BatchSize = *batchSize
	opts.DryRun = *dryRun
	opts.Verbose = *verbose

	// Parse user ID if provided
	if *userID != "" {
		uid, err := uuid.Parse(*userID)
		if err != nil {
			log.Fatal("Invalid user ID", zap.String("user_id", *userID), zap.Error(err))
		}
		opts.UserID = &uid
	}

	// Parse start date if provided
	if *startDate != "" {
		t, err := time.Parse(time.RFC3339, *startDate)
		if err != nil {
			log.Fatal("Invalid start date", zap.String("date", *startDate), zap.Error(err))
		}
		opts.StartDate = &t
	}

	// Parse end date if provided
	if *endDate != "" {
		t, err := time.Parse(time.RFC3339, *endDate)
		if err != nil {
			log.Fatal("Invalid end date", zap.String("date", *endDate), zap.Error(err))
		}
		opts.EndDate = &t
	}

	// Print configuration
	log.Info("Starting reindex operation",
		zap.Int("batch_size", opts.BatchSize),
		zap.Bool("dry_run", opts.DryRun),
		zap.Bool("verbose", opts.Verbose),
	)

	if opts.UserID != nil {
		log.Info("Filtering by user", zap.String("user_id", opts.UserID.String()))
	}

	if opts.StartDate != nil {
		log.Info("Filtering by start date", zap.Time("start_date", *opts.StartDate))
	}

	if opts.EndDate != nil {
		log.Info("Filtering by end date", zap.Time("end_date", *opts.EndDate))
	}

	// Execute reindex
	result, err := reindexer.Reindex(ctx, opts)
	if err != nil {
		log.Fatal("Reindex failed", zap.Error(err))
	}

	// Print results
	fmt.Printf("\n=== Reindex Results ===\n")
	fmt.Printf("Total Documents:    %d\n", result.TotalDocuments)
	fmt.Printf("Processed:          %d\n", result.ProcessedDocuments)
	fmt.Printf("Failed:             %d\n", result.FailedDocuments)
	fmt.Printf("Batches:            %d\n", result.BatchCount)
	fmt.Printf("Duration:           %s\n", result.Duration)
	fmt.Printf("Rate (docs/sec):    %.2f\n", result.AverageRatePerSec)

	if opts.DryRun {
		fmt.Printf("\n(Dry run - no changes made)\n")
	}

	// Compact index if requested
	if *compact && !opts.DryRun {
		log.Info("Compacting search index")
		if err := reindexer.CompactIndex(ctx); err != nil {
			log.Error("Compaction failed", zap.Error(err))
		} else {
			log.Info("Index compaction completed")
		}
	}

	// Verify after reindex
	if !opts.DryRun {
		log.Info("Verifying index after reindex")
		verifyResult, err := reindexer.VerifyIndex(ctx)
		if err != nil {
			log.Error("Post-reindex verification failed", zap.Error(err))
		} else if !verifyResult.Healthy {
			log.Warn("Index has issues after reindex",
				zap.Int64("missing", verifyResult.MissingVectors),
				zap.Int64("empty", verifyResult.EmptyVectors),
			)
		} else {
			log.Info("Index verified successfully")
		}
	}

	fmt.Printf("\nReindex completed successfully!\n")
}
