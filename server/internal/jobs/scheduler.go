package jobs

import (
	"context"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// Scheduler manages background job execution
type Scheduler struct {
	db              *sqlx.DB
	logger          *zap.Logger
	accountDeletion *AccountDeletionJob
	dataRetention   *DataRetentionJob
	exportCleanup   *ExportCleanupJob
	subscription    *SubscriptionJob
	wg              sync.WaitGroup
	stopCh          chan struct{}
	mu              sync.Mutex
	running         bool
}

// NewScheduler creates a new job scheduler
func NewScheduler(db *sqlx.DB, logger *zap.Logger) *Scheduler {
	return &Scheduler{
		db:              db,
		logger:          logger,
		accountDeletion: NewAccountDeletionJob(db, logger),
		dataRetention:   NewDataRetentionJob(db, logger),
		exportCleanup:   NewExportCleanupJob(db, logger),
		subscription:    nil, // Will be set via SetSubscriptionJob
		stopCh:          make(chan struct{}),
	}
}

// SetSubscriptionJob sets the subscription job (to avoid circular dependencies)
func (s *Scheduler) SetSubscriptionJob(job *SubscriptionJob) {
	s.subscription = job
}

// Start begins running scheduled jobs
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = true
	s.mu.Unlock()

	s.logger.Info("Starting job scheduler")

	// Count total jobs to start
	jobCount := 4
	if s.subscription != nil {
		jobCount += 2 // Add subscription jobs
	}
	s.wg.Add(jobCount)

	// Account deletion job - runs every hour
	go s.runJob(ctx, "account_deletion", time.Hour, func(ctx context.Context) error {
		return s.accountDeletion.Run(ctx)
	})

	// Deletion reminder job - runs daily at 10 AM
	go s.runDailyJob(ctx, "deletion_reminder", 10, 0, func(ctx context.Context) error {
		return s.accountDeletion.SendDeletionReminder(ctx)
	})

	// Data retention job - runs daily at 2 AM
	go s.runDailyJob(ctx, "data_retention", 2, 0, func(ctx context.Context) error {
		return s.dataRetention.Run(ctx)
	})

	// Export cleanup job - runs every 6 hours
	go s.runJob(ctx, "export_cleanup", 6*time.Hour, func(ctx context.Context) error {
		return s.exportCleanup.Run(ctx)
	})

	// Subscription jobs (if configured)
	if s.subscription != nil {
		// Subscription expiry notifications - runs daily at 9 AM
		go s.runDailyJob(ctx, "subscription_expiry_notifications", 9, 0, func(ctx context.Context) error {
			return s.subscription.RunExpiryNotifications(ctx)
		})

		// Dunning process - runs every 6 hours
		go s.runJob(ctx, "subscription_dunning", 6*time.Hour, func(ctx context.Context) error {
			return s.subscription.RunDunningProcess(ctx)
		})
	}

	return nil
}

// Stop gracefully stops all scheduled jobs
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()

	s.logger.Info("Stopping job scheduler")
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("Job scheduler stopped")
}

// runJob runs a job on a regular interval
func (s *Scheduler) runJob(ctx context.Context, name string, interval time.Duration, job func(context.Context) error) {
	defer s.wg.Done()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run immediately on start
	s.executeJob(ctx, name, job)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Job stopped due to context cancellation", zap.String("job", name))
			return
		case <-s.stopCh:
			s.logger.Info("Job stopped", zap.String("job", name))
			return
		case <-ticker.C:
			s.executeJob(ctx, name, job)
		}
	}
}

// runDailyJob runs a job once per day at a specific time
func (s *Scheduler) runDailyJob(ctx context.Context, name string, hour, minute int, job func(context.Context) error) {
	defer s.wg.Done()

	for {
		// Calculate next run time
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
		if next.Before(now) {
			// If the time has already passed today, schedule for tomorrow
			next = next.Add(24 * time.Hour)
		}

		waitDuration := next.Sub(now)
		s.logger.Info("Scheduling daily job",
			zap.String("job", name),
			zap.Time("next_run", next),
			zap.Duration("wait", waitDuration))

		select {
		case <-ctx.Done():
			s.logger.Info("Daily job stopped due to context cancellation", zap.String("job", name))
			return
		case <-s.stopCh:
			s.logger.Info("Daily job stopped", zap.String("job", name))
			return
		case <-time.After(waitDuration):
			s.executeJob(ctx, name, job)
		}
	}
}

// executeJob executes a job with error handling and logging
func (s *Scheduler) executeJob(ctx context.Context, name string, job func(context.Context) error) {
	startTime := time.Now()
	s.logger.Info("Executing job", zap.String("job", name))

	// Create a timeout context for the job
	jobCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	if err := job(jobCtx); err != nil {
		s.logger.Error("Job failed",
			zap.String("job", name),
			zap.Error(err),
			zap.Duration("duration", time.Since(startTime)))
		// Record job failure in database
		s.recordJobExecution(name, false, err.Error(), time.Since(startTime))
	} else {
		s.logger.Info("Job completed successfully",
			zap.String("job", name),
			zap.Duration("duration", time.Since(startTime)))
		// Record job success in database
		s.recordJobExecution(name, true, "", time.Since(startTime))
	}
}

// recordJobExecution records job execution history
func (s *Scheduler) recordJobExecution(name string, success bool, errorMsg string, duration time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Store in a job_executions table for monitoring
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO job_executions (job_name, success, error_message, duration_ms, executed_at)
		 VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP)
		 ON CONFLICT (job_name) DO UPDATE
		 SET success = $2,
		     error_message = $3,
		     duration_ms = $4,
		     executed_at = CURRENT_TIMESTAMP,
		     execution_count = job_executions.execution_count + 1`,
		name, success, errorMsg, duration.Milliseconds())
	if err != nil {
		s.logger.Warn("Failed to record job execution",
			zap.String("job", name),
			zap.Error(err))
	}
}

// GetJobStatus returns the status of all scheduled jobs
func (s *Scheduler) GetJobStatus(ctx context.Context) ([]JobStatus, error) {
	var statuses []JobStatus
	err := s.db.SelectContext(ctx, &statuses,
		`SELECT job_name, success, error_message, duration_ms, executed_at, execution_count
		 FROM job_executions
		 ORDER BY job_name`)
	if err != nil {
		return nil, err
	}
	return statuses, nil
}

// JobStatus represents the status of a scheduled job
type JobStatus struct {
	JobName        string     `db:"job_name" json:"job_name"`
	Success        bool       `db:"success" json:"success"`
	ErrorMessage   *string    `db:"error_message" json:"error_message,omitempty"`
	DurationMS     int64      `db:"duration_ms" json:"duration_ms"`
	ExecutedAt     time.Time  `db:"executed_at" json:"executed_at"`
	ExecutionCount int        `db:"execution_count" json:"execution_count"`
}