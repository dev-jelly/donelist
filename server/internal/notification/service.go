package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Service provides high-level notification operations
type Service struct {
	db              *sqlx.DB
	logger          *zap.Logger
	queue           *Queue
	scheduler       *Scheduler
	settingsEngine  *SettingsEngine
	settingsRepo    *SettingsRepository
	intervalChecker *IntervalChecker
}

// ServiceConfig holds service configuration
type ServiceConfig struct {
	QueueConfig     QueueConfig
	SchedulerConfig *SchedulerConfig
	RedisClient     *redis.Client
}

// NewService creates a new notification service
func NewService(db *sqlx.DB, logger *zap.Logger, config *ServiceConfig) (*Service, error) {
	// Initialize queue with Redis
	var queue *Queue
	if config.RedisClient != nil {
		queue = NewQueue(config.RedisClient, logger, config.QueueConfig)
	}

	// Initialize settings repository
	settingsRepo := NewSettingsRepository(db, logger)

	// Initialize settings engine
	settingsEngine := NewSettingsEngine(logger, settingsRepo)

	// Initialize processor with settings engine
	processor := NewProcessor(logger)

	// Initialize scheduler
	scheduler := NewScheduler(config.SchedulerConfig, queue, processor, logger)

	return &Service{
		db:             db,
		logger:         logger,
		queue:          queue,
		scheduler:      scheduler,
		settingsEngine: settingsEngine,
		settingsRepo:   settingsRepo,
	}, nil
}

// SetIntervalChecker sets the interval checker for the service
func (s *Service) SetIntervalChecker(intervalChecker *IntervalChecker) {
	s.intervalChecker = intervalChecker
}

// Start starts the notification service
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting notification service")

	// Start the scheduler (which starts workers)
	if err := s.scheduler.Start(ctx); err != nil {
		return fmt.Errorf("failed to start scheduler: %w", err)
	}

	// Start reminder processing cron job
	go s.processRemindersLoop(ctx)

	// Start interval checker if configured
	if s.intervalChecker != nil {
		go s.intervalChecker.StartIntervalCheckLoop(ctx)
		s.logger.Info("Started interval checker")
	}

	s.logger.Info("Notification service started successfully")
	return nil
}

// Stop stops the notification service
func (s *Service) Stop() error {
	s.logger.Info("Stopping notification service")

	// Stop the scheduler
	if err := s.scheduler.Stop(); err != nil {
		return fmt.Errorf("failed to stop scheduler: %w", err)
	}

	s.logger.Info("Notification service stopped")
	return nil
}

// SendNotification sends a notification with settings applied
func (s *Service) SendNotification(ctx context.Context, notification *Notification) error {
	// Apply notification settings
	shouldSend, err := s.settingsEngine.ShouldSendNotification(
		ctx, notification.UserID, notification.Type, notification.ScheduledFor,
	)
	if err != nil {
		return fmt.Errorf("failed to check notification settings: %w", err)
	}

	if !shouldSend {
		s.logger.Info("Notification blocked by user settings",
			zap.String("notification_id", notification.ID.String()),
			zap.String("user_id", notification.UserID.String()),
			zap.String("type", string(notification.Type)),
		)
		return nil
	}

	// Check if we need to reschedule due to DnD
	availableTime, err := s.settingsEngine.GetNextAvailableTime(
		ctx, notification.UserID, notification.ScheduledFor,
	)
	if err != nil {
		s.logger.Warn("Failed to get next available time, using original schedule",
			zap.Error(err),
		)
	} else if !availableTime.Equal(notification.ScheduledFor) {
		notification.ScheduledFor = availableTime
		s.logger.Info("Rescheduled notification due to DnD",
			zap.String("notification_id", notification.ID.String()),
			zap.Time("new_time", availableTime),
		)
	}

	// Enqueue the notification - convert to job format
	if s.queue != nil {
		job := notificationToJob(notification)
		if err := s.queue.Enqueue(ctx, job); err != nil {
			return fmt.Errorf("failed to enqueue notification: %w", err)
		}
	}

	return nil
}

// notificationToJob converts a Notification to a NotificationJob
func notificationToJob(n *Notification) *NotificationJob {
	return &NotificationJob{
		ID:          n.ID.String(),
		UserID:      n.UserID,
		Type:        string(n.Type),
		Payload:     n.Data,
		ScheduledAt: n.ScheduledFor,
		CreatedAt:   n.CreatedAt,
		Attempts:    n.RetryCount,
		MaxAttempts: n.MaxRetries,
	}
}

// SendBatchNotifications sends multiple notifications with settings applied
func (s *Service) SendBatchNotifications(ctx context.Context, notifications []*Notification) error {
	// Apply settings to batch
	filtered, err := s.settingsEngine.ApplyBatchSettings(ctx, notifications)
	if err != nil {
		return fmt.Errorf("failed to apply batch settings: %w", err)
	}

	s.logger.Info("Sending batch notifications",
		zap.Int("original_count", len(notifications)),
		zap.Int("filtered_count", len(filtered)),
	)

	// Enqueue filtered notifications
	if s.queue != nil {
		for _, notification := range filtered {
			job := notificationToJob(notification)
			if err := s.queue.Enqueue(ctx, job); err != nil {
				s.logger.Error("Failed to enqueue notification",
					zap.String("notification_id", notification.ID.String()),
					zap.Error(err),
				)
				// Continue with other notifications
			}
		}
	}

	return nil
}

// UpdateUserNotificationSettings updates a user's notification settings
func (s *Service) UpdateUserNotificationSettings(ctx context.Context, settings *NotificationSettings) error {
	// Validate DnD settings
	if err := s.settingsEngine.dndScheduler.ValidateDNDSettings(settings); err != nil {
		return fmt.Errorf("invalid DnD settings: %w", err)
	}

	// Update settings in database
	if err := s.settingsRepo.UpdateNotificationSettings(ctx, settings); err != nil {
		return fmt.Errorf("failed to update notification settings: %w", err)
	}

	s.logger.Info("Updated notification settings",
		zap.String("user_id", settings.UserID.String()),
		zap.Bool("dnd_enabled", settings.DNDEnabled),
	)

	return nil
}

// GetUserNotificationSettings retrieves a user's notification settings
func (s *Service) GetUserNotificationSettings(ctx context.Context, userID uuid.UUID) (*NotificationSettings, error) {
	return s.settingsRepo.GetUserNotificationSettings(ctx, userID)
}

// GetUserNotificationStatus returns the current notification status for a user
func (s *Service) GetUserNotificationStatus(ctx context.Context, userID uuid.UUID) (map[string]interface{}, error) {
	return s.settingsEngine.GetUserNotificationStatus(ctx, userID)
}

// GetDNDStatus returns detailed DND status for a user
func (s *Service) GetDNDStatus(ctx context.Context, userID uuid.UUID) (map[string]interface{}, error) {
	settings, err := s.settingsRepo.GetUserNotificationSettings(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user settings: %w", err)
	}

	return s.settingsEngine.dndScheduler.GetDNDStatus(settings), nil
}

// CreateDNDOverride creates a temporary DND override
func (s *Service) CreateDNDOverride(ctx context.Context, override *DNDOverride) error {
	if override.EndTime.Before(override.StartTime) {
		return fmt.Errorf("end time must be after start time")
	}

	if override.StartTime.Before(time.Now()) {
		return fmt.Errorf("start time must be in the future")
	}

	return s.settingsRepo.CreateDNDOverride(ctx, override)
}

// processRemindersLoop runs the reminder processing loop
func (s *Service) processRemindersLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute) // Check every 5 minutes
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Reminder processing loop stopped")
			return
		case <-ticker.C:
			if err := s.processReminders(ctx); err != nil {
				s.logger.Error("Failed to process reminders", zap.Error(err))
			}
		}
	}
}

// processReminders processes reminder notifications
func (s *Service) processReminders(ctx context.Context) error {
	users, err := s.settingsRepo.GetUsersWithRemindersEnabled(ctx)
	if err != nil {
		return fmt.Errorf("failed to get users with reminders: %w", err)
	}

	s.logger.Debug("Processing reminders", zap.Int("user_count", len(users)))

	for _, userID := range users {
		settings, err := s.settingsRepo.GetUserNotificationSettings(ctx, userID)
		if err != nil {
			s.logger.Error("Failed to get user settings",
				zap.String("user_id", userID.String()),
				zap.Error(err),
			)
			continue
		}

		if !settings.CheckinReminders {
			continue
		}

		// Check if it's time to send a reminder
		now := time.Now()
		shouldSend, err := s.settingsEngine.ShouldSendNotification(
			ctx, userID, NotificationTypeReminder, now,
		)
		if err != nil {
			s.logger.Error("Failed to check reminder eligibility",
				zap.String("user_id", userID.String()),
				zap.Error(err),
			)
			continue
		}

		if shouldSend {
			// Create and send reminder notification
			notification := &Notification{
				ID:           uuid.New(),
				UserID:       userID,
				Type:         NotificationTypeReminder,
				Title:        "Check-in Reminder",
				Body:         "It's time to update your progress on your tasks!",
				Priority:     PriorityNormal,
				Status:       StatusPending,
				ScheduledFor: now,
				CreatedAt:    now,
				UpdatedAt:    now,
				MaxRetries:   3,
			}

			if err := s.SendNotification(ctx, notification); err != nil {
				s.logger.Error("Failed to send reminder notification",
					zap.String("user_id", userID.String()),
					zap.Error(err),
				)
				continue
			}

			// Update last reminder sent time
			if err := s.settingsRepo.UpdateLastReminderSent(ctx, userID, now); err != nil {
				s.logger.Error("Failed to update last reminder time",
					zap.String("user_id", userID.String()),
					zap.Error(err),
				)
			}
		}
	}

	return nil
}

// GetQueueStats returns queue statistics
func (s *Service) GetQueueStats(ctx context.Context) (map[string]interface{}, error) {
	stats, err := s.queue.GetQueueStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get queue stats: %w", err)
	}

	// Add scheduler info
	scheduleInfo := s.scheduler.GetScheduleInfo()
	for k, v := range scheduleInfo {
		stats[fmt.Sprintf("scheduler_%s", k)] = v
	}

	return stats, nil
}

// GetHealthStatus returns the health status of the notification service
func (s *Service) GetHealthStatus(ctx context.Context) (map[string]interface{}, error) {
	healthStatus, err := s.scheduler.GetHealthStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get health status: %w", err)
	}

	// Add service-specific health checks
	healthStatus["service"] = "notification"
	healthStatus["timestamp"] = time.Now().Format(time.RFC3339)

	return healthStatus, nil
}

// CleanupExpiredNotifications removes old notifications from the database
func (s *Service) CleanupExpiredNotifications(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)

	query := `
		DELETE FROM notifications
		WHERE created_at < $1
			AND status IN ('sent', 'failed', 'cancelled')
	`

	result, err := s.db.ExecContext(ctx, query, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup notifications: %w", err)
	}

	count, _ := result.RowsAffected()

	// Also cleanup expired DND overrides
	overrideCount, err := s.settingsRepo.DeleteExpiredDNDOverrides(ctx)
	if err != nil {
		s.logger.Warn("Failed to cleanup DND overrides", zap.Error(err))
	}

	s.logger.Info("Cleaned up expired notifications",
		zap.Int64("notification_count", count),
		zap.Int64("override_count", overrideCount),
	)

	return count, nil
}