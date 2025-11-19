package security

// This file contains example code for integrating the anomaly detection system
// DO NOT use this in production - it's for documentation purposes only

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ExampleBasicSetup demonstrates basic setup
func ExampleBasicSetup(db *gorm.DB, redis *redis.Client, logger *zap.Logger) *SecurityManager {
	// Use default production configuration
	config := DefaultSecurityConfig(db, redis, logger)

	// Create security manager
	manager := NewSecurityManager(config)

	// Start background workers
	manager.Start()

	return manager
}

// ExampleCustomSetup demonstrates custom configuration
func ExampleCustomSetup(db *gorm.DB, redis *redis.Client, logger *zap.Logger) *SecurityManager {
	config := DefaultSecurityConfig(db, redis, logger)

	// Customize thresholds
	config.AnomalyDetection.RapidRequestThreshold = 50
	config.AnomalyDetection.BruteForceThreshold = 5

	// Enable all alert channels
	config.AlertManagement.EnabledChannels = []AlertChannel{
		AlertChannelLog,
		AlertChannelWebhook,
		AlertChannelSlack,
	}
	config.AlertManagement.SlackWebhookURL = "https://hooks.slack.com/services/YOUR/WEBHOOK/URL"

	// Configure aggressive blocking
	config.AutoBlocking.AutoBlockOnCritical = true
	config.AutoBlocking.AutoBlockOnHighCount = 2

	manager := NewSecurityManager(config)
	manager.Start()

	return manager
}

// ExampleMiddlewareIntegration demonstrates middleware setup
func ExampleMiddlewareIntegration(router *gin.Engine, manager *SecurityManager, logger *zap.Logger) {
	// Example only - do not use in production

	// Option 1: Full anomaly detection middleware
	// Checks all detection rules and handles alerts/blocking
	// router.Use(middleware.AnomalyDetectionMiddleware(
	//     middleware.CreateDefaultAnomalyDetectionConfig(
	//         manager.Detector,
	//         manager.AlertManager,
	//         manager.AutoBlocker,
	//         logger,
	//     ),
	// ))

	// Option 2: Lightweight block-check middleware
	// Only checks if identifier is blocked (faster)
	// router.Use(middleware.BlockCheckMiddleware(
	//     manager.AutoBlocker,
	//     logger,
	// ))
}

// ExampleManualDetection demonstrates manual anomaly checking
func ExampleManualDetection(ctx context.Context, manager *SecurityManager, userID, ipAddress string) error {
	// Check for rapid requests
	anomaly, err := manager.Detector.CheckRapidRequests(ctx, userID)
	if err != nil {
		return err
	}

	if anomaly != nil {
		anomaly.IPAddress = ipAddress

		// Send alert
		if err := manager.AlertManager.SendAlert(ctx, anomaly); err != nil {
			manager.Logger.Error("Failed to send alert", zap.Error(err))
		}

		// Evaluate auto-blocking
		_, err := manager.AutoBlocker.EvaluateBlock(ctx, anomaly)
		if err != nil {
			return err
		}
	}

	return nil
}

// ExampleBlockManagement demonstrates block operations
func ExampleBlockManagement(ctx context.Context, blocker *AutoBlocker) error {
	identifier := "suspicious-user-123"
	ipAddress := "1.2.3.4"

	// Check if blocked
	isBlocked, err := blocker.IsBlocked(ctx, identifier)
	if err != nil {
		return err
	}

	if isBlocked {
		// Get block info
		blockInfo, err := blocker.GetBlockInfo(ctx, identifier)
		if err != nil {
			return err
		}

		_ = blockInfo // Use block info

		// Manually unblock
		// err = blocker.UnblockIdentifier(ctx, identifier, nil, "Verified legitimate user")
		// if err != nil {
		//     return err
		// }
	} else {
		// Manually block
		// err = blocker.BlockIdentifier(
		//     ctx,
		//     identifier,
		//     ipAddress,
		//     BlockReasonManual,
		//     "Manual block due to investigation",
		//     SeverityHigh,
		//     map[string]interface{}{
		//         "admin_notes": "Under investigation",
		//     },
		// )
	}

	return nil
}

// ExampleAlertManagement demonstrates alert operations
func ExampleAlertManagement(ctx context.Context, alertManager *AlertManager) error {
	// List recent critical alerts
	opts := ListAlertsOptions{
		Severity: func() *AnomalySeverity { s := SeverityCritical; return &s }(),
		Status:   func() *AlertStatus { s := AlertStatusPending; return &s }(),
		Limit:    50,
	}

	alerts, err := alertManager.ListAlerts(ctx, opts)
	if err != nil {
		return err
	}

	for _, alert := range alerts {
		// Review alert
		_ = alert

		// Resolve if false positive
		// err = alertManager.ResolveAlert(ctx, alert.ID, nil, "False positive - normal behavior")
		// if err != nil {
		//     return err
		// }

		// Or ignore
		// err = alertManager.IgnoreAlert(ctx, alert.ID, "Known issue, ignoring")
		// if err != nil {
		//     return err
		// }
	}

	return nil
}

// ExampleStatistics demonstrates getting security statistics
func ExampleStatistics(ctx context.Context, manager *SecurityManager) error {
	// Get statistics for last 24 hours
	// stats, err := manager.GetStatistics(time.Now().Add(-24 * time.Hour))
	// if err != nil {
	//     return err
	// }

	// Use statistics for monitoring/reporting
	// _ = stats

	return nil
}

// ExampleWhitelisting demonstrates whitelist management
func ExampleWhitelisting(ctx context.Context, blocker *AutoBlocker) error {
	// Add to whitelist
	err := blocker.AddToWhitelist(ctx, "trusted-user-123")
	if err != nil {
		return err
	}

	// Remove from whitelist
	err = blocker.RemoveFromWhitelist(ctx, "trusted-user-123")
	if err != nil {
		return err
	}

	return nil
}
