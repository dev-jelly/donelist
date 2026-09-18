package jobs

import (
	"context"
	"fmt"

	"github.com/dev-jelly/donelist/internal/subscriptionjobs"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// SubscriptionJob handles subscription-related background jobs
type SubscriptionJob struct {
	db                   *sqlx.DB
	logger               *zap.Logger
	notificationService  *subscriptionjobs.NotificationService
	dunningService       *subscriptionjobs.DunningService
	notificationConfig   *subscriptionjobs.NotificationConfig
	dunningConfig        *subscriptionjobs.DunningConfig
}

// NewSubscriptionJob creates a new subscription job
func NewSubscriptionJob(
	db *sqlx.DB,
	logger *zap.Logger,
	notificationService *subscriptionjobs.NotificationService,
	dunningService *subscriptionjobs.DunningService,
) *SubscriptionJob {
	return &SubscriptionJob{
		db:                  db,
		logger:              logger,
		notificationService: notificationService,
		dunningService:      dunningService,
		notificationConfig:  subscriptionjobs.DefaultNotificationConfig(),
		dunningConfig:       subscriptionjobs.DefaultDunningConfig(),
	}
}

// SetNotificationConfig sets the notification configuration
func (j *SubscriptionJob) SetNotificationConfig(config *subscriptionjobs.NotificationConfig) {
	j.notificationConfig = config
}

// SetDunningConfig sets the dunning configuration
func (j *SubscriptionJob) SetDunningConfig(config *subscriptionjobs.DunningConfig) {
	j.dunningConfig = config
}

// RunExpiryNotifications processes subscription expiry notifications
func (j *SubscriptionJob) RunExpiryNotifications(ctx context.Context) error {
	j.logger.Info("Running subscription expiry notifications job")

	if err := j.notificationService.ProcessExpiryNotifications(ctx, j.notificationConfig); err != nil {
		j.logger.Error("Subscription expiry notifications job failed", zap.Error(err))
		return fmt.Errorf("failed to process expiry notifications: %w", err)
	}

	j.logger.Info("Subscription expiry notifications job completed successfully")
	return nil
}

// RunDunningProcess processes payment failures and retries
func (j *SubscriptionJob) RunDunningProcess(ctx context.Context) error {
	j.logger.Info("Running dunning process job")

	if err := j.dunningService.ProcessPaymentFailures(ctx, j.dunningConfig); err != nil {
		j.logger.Error("Dunning process job failed", zap.Error(err))
		return fmt.Errorf("failed to process payment failures: %w", err)
	}

	j.logger.Info("Dunning process job completed successfully")
	return nil
}

// RunAll runs all subscription-related jobs
func (j *SubscriptionJob) RunAll(ctx context.Context) error {
	j.logger.Info("Running all subscription jobs")

	// Run expiry notifications
	if err := j.RunExpiryNotifications(ctx); err != nil {
		j.logger.Error("Expiry notifications failed", zap.Error(err))
		// Continue with other jobs
	}

	// Run dunning process
	if err := j.RunDunningProcess(ctx); err != nil {
		j.logger.Error("Dunning process failed", zap.Error(err))
		// Continue with other jobs
	}

	j.logger.Info("All subscription jobs completed")
	return nil
}
