package backup

import (
	"context"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

// Scheduler handles automated backup scheduling
type Scheduler struct {
	config   *Config
	cron     *cron.Cron
	service  *Service
	logger   *zap.Logger
	stopChan chan struct{}
}

// NewScheduler creates a new backup scheduler
func NewScheduler(config *Config, service *Service, logger *zap.Logger) *Scheduler {
	return &Scheduler{
		config:   config,
		cron:     cron.New(cron.WithSeconds()),
		service:  service,
		logger:   logger,
		stopChan: make(chan struct{}),
	}
}

// Start starts the backup scheduler
func (s *Scheduler) Start(ctx context.Context) error {
	if !s.config.ScheduleEnabled {
		s.logger.Info("Backup scheduling is disabled")
		return nil
	}

	s.logger.Info("Starting backup scheduler", zap.String("schedule", s.config.ScheduleCron))

	// Schedule backup job
	_, err := s.cron.AddFunc(s.config.ScheduleCron, func() {
		s.logger.Info("Scheduled backup starting")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		if err := s.service.PerformScheduledBackup(ctx); err != nil {
			s.logger.Error("Scheduled backup failed", zap.Error(err))
		}
	})
	if err != nil {
		return fmt.Errorf("failed to schedule backup job: %w", err)
	}

	// Schedule rotation job (run daily at 3 AM)
	_, err = s.cron.AddFunc("0 0 3 * * *", func() {
		s.logger.Info("Scheduled rotation starting")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		if err := s.service.ApplyRetentionPolicy(ctx); err != nil {
			s.logger.Error("Scheduled rotation failed", zap.Error(err))
		}
	})
	if err != nil {
		return fmt.Errorf("failed to schedule rotation job: %w", err)
	}

	// Schedule health check (run every configured interval)
	if s.config.MonitoringEnabled {
		_, err = s.cron.AddFunc(fmt.Sprintf("@every %s", s.config.HealthCheckInterval), func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			if err := s.service.PerformHealthCheck(ctx); err != nil {
				s.logger.Error("Health check failed", zap.Error(err))
			}
		})
		if err != nil {
			return fmt.Errorf("failed to schedule health check: %w", err)
		}
	}

	// Start the cron scheduler
	s.cron.Start()

	s.logger.Info("Backup scheduler started successfully",
		zap.Int("scheduled_jobs", len(s.cron.Entries())),
	)

	return nil
}

// Stop stops the backup scheduler
func (s *Scheduler) Stop() error {
	s.logger.Info("Stopping backup scheduler")

	ctx := s.cron.Stop()
	<-ctx.Done()

	close(s.stopChan)

	s.logger.Info("Backup scheduler stopped")
	return nil
}

// GetNextRun returns the next scheduled backup time
func (s *Scheduler) GetNextRun() time.Time {
	entries := s.cron.Entries()
	if len(entries) == 0 {
		return time.Time{}
	}
	return entries[0].Next
}

// GetScheduleInfo returns information about scheduled jobs
func (s *Scheduler) GetScheduleInfo() map[string]interface{} {
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
		"enabled":        s.config.ScheduleEnabled,
		"cron_schedule":  s.config.ScheduleCron,
		"total_jobs":     len(entries),
		"jobs":           jobs,
	}
}
