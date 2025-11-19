package notification

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

// Scheduler handles periodic notification scheduling and worker management
type Scheduler struct {
	config      *SchedulerConfig
	cron        *cron.Cron
	queue       *Queue
	processor   *Processor
	logger      *zap.Logger
	stopChan    chan struct{}
	workerPool  *WorkerPool
	mu          sync.RWMutex
	isRunning   bool
}

// SchedulerConfig holds scheduler configuration
type SchedulerConfig struct {
	// Cron schedule for moving delayed jobs to ready queue (e.g., "*/1 * * * *" for every minute)
	ScanInterval string

	// Number of concurrent workers processing notifications
	WorkerCount int

	// Worker polling interval
	PollInterval time.Duration

	// Health check configuration
	HealthCheckEnabled  bool
	HealthCheckInterval time.Duration

	// Monitoring
	MetricsEnabled bool
}

// DefaultSchedulerConfig returns default scheduler configuration
func DefaultSchedulerConfig() *SchedulerConfig {
	return &SchedulerConfig{
		ScanInterval:        "*/1 * * * *", // Every minute
		WorkerCount:         5,
		PollInterval:        5 * time.Second,
		HealthCheckEnabled:  true,
		HealthCheckInterval: 30 * time.Second,
		MetricsEnabled:      true,
	}
}

// NewScheduler creates a new notification scheduler
func NewScheduler(
	config *SchedulerConfig,
	queue *Queue,
	processor *Processor,
	logger *zap.Logger,
) *Scheduler {
	if config == nil {
		config = DefaultSchedulerConfig()
	}

	return &Scheduler{
		config:     config,
		cron:       cron.New(cron.WithSeconds()),
		queue:      queue,
		processor:  processor,
		logger:     logger,
		stopChan:   make(chan struct{}),
		workerPool: NewWorkerPool(config.WorkerCount, queue, processor, logger),
	}
}

// Start starts the notification scheduler and workers
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.isRunning {
		s.mu.Unlock()
		return fmt.Errorf("scheduler is already running")
	}
	s.isRunning = true
	s.mu.Unlock()

	s.logger.Info("Starting notification scheduler",
		zap.String("scan_interval", s.config.ScanInterval),
		zap.Int("worker_count", s.config.WorkerCount),
	)

	// Schedule delayed queue scanner
	_, err := s.cron.AddFunc(s.config.ScanInterval, func() {
		s.scanDelayedQueue()
	})
	if err != nil {
		return fmt.Errorf("failed to schedule delayed queue scanner: %w", err)
	}

	// Schedule health check if enabled
	if s.config.HealthCheckEnabled {
		_, err = s.cron.AddFunc(fmt.Sprintf("@every %s", s.config.HealthCheckInterval), func() {
			s.performHealthCheck()
		})
		if err != nil {
			return fmt.Errorf("failed to schedule health check: %w", err)
		}
	}

	// Schedule stuck job recovery (every 5 minutes)
	_, err = s.cron.AddFunc("*/5 * * * *", func() {
		s.recoverStuckJobs()
	})
	if err != nil {
		return fmt.Errorf("failed to schedule stuck job recovery: %w", err)
	}

	// Start cron scheduler
	s.cron.Start()

	// Start worker pool
	if err := s.workerPool.Start(ctx); err != nil {
		return fmt.Errorf("failed to start worker pool: %w", err)
	}

	s.logger.Info("Notification scheduler started successfully",
		zap.Int("scheduled_jobs", len(s.cron.Entries())),
		zap.Int("workers", s.config.WorkerCount),
	)

	return nil
}

// Stop gracefully stops the scheduler and workers
func (s *Scheduler) Stop() error {
	s.mu.Lock()
	if !s.isRunning {
		s.mu.Unlock()
		return fmt.Errorf("scheduler is not running")
	}
	s.isRunning = false
	s.mu.Unlock()

	s.logger.Info("Stopping notification scheduler")

	// Stop worker pool first
	if err := s.workerPool.Stop(); err != nil {
		s.logger.Error("Failed to stop worker pool gracefully", zap.Error(err))
	}

	// Stop cron scheduler
	cronCtx := s.cron.Stop()
	<-cronCtx.Done()

	close(s.stopChan)

	s.logger.Info("Notification scheduler stopped")
	return nil
}

// scanDelayedQueue moves ready notifications from delayed queue to ready queue
func (s *Scheduler) scanDelayedQueue() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	count, err := s.queue.MoveDelayedToReady(ctx)
	if err != nil {
		s.logger.Error("Failed to scan delayed queue", zap.Error(err))
		return
	}

	if count > 0 {
		s.logger.Info("Moved notifications to ready queue",
			zap.Int("count", count),
		)
	}
}

// performHealthCheck checks the health of the notification system
func (s *Scheduler) performHealthCheck() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stats, err := s.queue.GetQueueStats(ctx)
	if err != nil {
		s.logger.Error("Health check failed: unable to get queue stats", zap.Error(err))
		return
	}

	s.logger.Debug("Notification system health check",
		zap.Any("queue_stats", stats),
		zap.Int("active_workers", s.workerPool.ActiveWorkerCount()),
	)

	// Check for concerning conditions
	if dlqCount, ok := stats["dlq_count"].(int64); ok && dlqCount > 100 {
		s.logger.Warn("High number of failed notifications in DLQ",
			zap.Int64("dlq_count", dlqCount),
		)
	}

	if readyCount, ok := stats["ready_count"].(int64); ok && readyCount > 1000 {
		s.logger.Warn("Large backlog in ready queue",
			zap.Int64("ready_count", readyCount),
		)
	}
}

// recoverStuckJobs finds and recovers jobs that have been stuck in processing
func (s *Scheduler) recoverStuckJobs() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	count, err := s.queue.RecoverStuckJobs(ctx)
	if err != nil {
		s.logger.Error("Failed to recover stuck jobs", zap.Error(err))
		return
	}

	if count > 0 {
		s.logger.Info("Recovered stuck jobs",
			zap.Int("count", count),
		)
	}
}

// GetScheduleInfo returns information about scheduled jobs
func (s *Scheduler) GetScheduleInfo() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries := s.cron.Entries()
	jobs := make([]map[string]interface{}, len(entries))
	for i, entry := range entries {
		jobs[i] = map[string]interface{}{
			"id":        entry.ID,
			"next_run":  entry.Next,
			"prev_run":  entry.Prev,
		}
	}

	return map[string]interface{}{
		"running":       s.isRunning,
		"scan_interval": s.config.ScanInterval,
		"worker_count":  s.config.WorkerCount,
		"active_workers": s.workerPool.ActiveWorkerCount(),
		"total_jobs":    len(entries),
		"jobs":          jobs,
	}
}

// GetHealthStatus returns the current health status of the scheduler
func (s *Scheduler) GetHealthStatus(ctx context.Context) (map[string]interface{}, error) {
	s.mu.RLock()
	isRunning := s.isRunning
	s.mu.RUnlock()

	if !isRunning {
		return map[string]interface{}{
			"status":  "stopped",
			"healthy": false,
		}, nil
	}

	stats, err := s.queue.GetQueueStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get queue stats: %w", err)
	}

	return map[string]interface{}{
		"status":         "running",
		"healthy":        true,
		"queue_stats":    stats,
		"active_workers": s.workerPool.ActiveWorkerCount(),
		"total_workers":  s.config.WorkerCount,
	}, nil
}

// IsRunning returns whether the scheduler is currently running
func (s *Scheduler) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isRunning
}
