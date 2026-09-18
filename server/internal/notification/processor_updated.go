package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ProcessorWithProviders handles the actual sending of notifications using push providers
type ProcessorWithProviders struct {
	logger          *zap.Logger
	providerManager *ProviderManager
	settingsRepo    *SettingsRepository
}

// NewProcessorWithProviders creates a new notification processor with provider support
func NewProcessorWithProviders(logger *zap.Logger, providerManager *ProviderManager, settingsRepo *SettingsRepository) *ProcessorWithProviders {
	return &ProcessorWithProviders{
		logger:          logger,
		providerManager: providerManager,
		settingsRepo:    settingsRepo,
	}
}

// ProcessNotification processes a notification by sending it through the appropriate provider
func (p *ProcessorWithProviders) ProcessNotification(ctx context.Context, notification *Notification) error {
	p.logger.Info("Processing notification with providers",
		zap.String("notification_id", notification.ID.String()),
		zap.String("user_id", notification.UserID.String()),
		zap.String("type", string(notification.Type)),
	)

	// Check notification type
	if notification.Type != NotificationTypePush {
		p.logger.Debug("Skipping non-push notification",
			zap.String("type", string(notification.Type)))
		return nil
	}

	// Validate required fields
	if notification.DeviceToken == nil || *notification.DeviceToken == "" {
		return fmt.Errorf("device token is required for push notification")
	}

	// Get user notification settings
	settings, err := p.settingsRepo.GetUserNotificationSettings(ctx, notification.UserID)
	if err != nil {
		p.logger.Warn("Failed to get user notification settings, using defaults",
			zap.String("user_id", notification.UserID.String()),
			zap.Error(err))
	}

	// Check if push notifications are enabled for this user
	if settings != nil && !settings.PushNotifications {
		p.logger.Info("Push notifications disabled for user",
			zap.String("user_id", notification.UserID.String()))

		// Update notification status
		notification.Status = StatusCancelled
		errorMsg := "Push notifications disabled by user"
		notification.Error = &errorMsg
		notification.UpdatedAt = time.Now()

		return nil
	}

	// Build push notification
	pushNotification := p.buildPushNotification(notification, settings)

	// Determine provider type
	var providerType PushProvider
	if notification.PushProvider != nil {
		providerType = PushProvider(*notification.PushProvider)
	} else {
		// Auto-detect based on token format
		providerType = p.determineProviderType(*notification.DeviceToken)
	}

	// Send through provider manager
	result, err := p.providerManager.Send(ctx, pushNotification, providerType)

	// Update notification based on result
	p.updateNotificationFromResult(notification, result, err)

	// Log device tokens that should be removed
	if result != nil && result.ShouldRemoveToken {
		p.handleInvalidToken(ctx, notification.UserID, *notification.DeviceToken, providerType)
	}

	// Update canonical token if provided
	if result != nil && result.CanonicalToken != "" {
		p.handleCanonicalToken(ctx, notification.UserID, *notification.DeviceToken, result.CanonicalToken)
	}

	return err
}

// ProcessBatch processes a batch of notifications
func (p *ProcessorWithProviders) ProcessBatch(ctx context.Context, notifications []*Notification) error {
	// Group notifications by provider type
	grouped := p.groupNotificationsByProvider(notifications)

	for providerType, batch := range grouped {
		p.logger.Info("Processing batch for provider",
			zap.String("provider", string(providerType)),
			zap.Int("count", len(batch)))

		// Convert to push notifications
		pushNotifications := make([]*PushNotification, len(batch))
		for i, notif := range batch {
			settings, _ := p.settingsRepo.GetUserNotificationSettings(ctx, notif.UserID)
			pushNotifications[i] = p.buildPushNotification(notif, settings)
		}

		// Send batch through provider
		results, err := p.providerManager.SendBatch(ctx, pushNotifications, providerType)
		if err != nil {
			p.logger.Error("Batch send failed",
				zap.String("provider", string(providerType)),
				zap.Error(err))
		}

		// Update notifications based on results
		for i, result := range results {
			p.updateNotificationFromResult(batch[i], result, nil)

			// Handle invalid tokens
			if result.ShouldRemoveToken && batch[i].DeviceToken != nil {
				p.handleInvalidToken(ctx, batch[i].UserID, *batch[i].DeviceToken, providerType)
			}
		}
	}

	return nil
}

// buildPushNotification builds a PushNotification from a Notification
func (p *ProcessorWithProviders) buildPushNotification(notification *Notification, settings *NotificationSettings) *PushNotification {
	pushNotif := &PushNotification{
		DeviceToken:  *notification.DeviceToken,
		Title:        notification.Title,
		Body:         notification.Body,
		HighPriority: notification.Priority == PriorityHigh || notification.Priority == PriorityUrgent,
		TTL:          3600, // Default 1 hour
	}

	// Add custom data if present
	if notification.Data != nil {
		data := make(map[string]string)
		for k, v := range notification.Data {
			// Convert to string (simple implementation)
			data[k] = fmt.Sprintf("%v", v)
		}
		pushNotif.Data = data
	}

	// Apply user settings if available
	if settings != nil {
		// Add default sound for push notifications (can be customized later)
		pushNotif.Sound = "default"

		// Badge can be added based on unread count from another service
		// For now, omit badge as it's not in the settings
	}

	// Set category for actionable notifications
	switch notification.Type {
	case NotificationTypeReminder:
		pushNotif.Category = "reminder"
	case NotificationTypeAlert:
		pushNotif.Category = "alert"
	}

	return pushNotif
}

// updateNotificationFromResult updates a notification based on the send result
func (p *ProcessorWithProviders) updateNotificationFromResult(notification *Notification, result *PushResult, err error) {
	now := time.Now()
	notification.UpdatedAt = now

	if result == nil {
		notification.Status = StatusFailed
		notification.FailedAt = &now
		if err != nil {
			errStr := err.Error()
			notification.Error = &errStr
		}
		return
	}

	if result.Success {
		notification.Status = StatusSent
		notification.SentAt = &now

		// Store provider response if needed
		if result.MessageID != "" {
			if notification.Data == nil {
				notification.Data = make(map[string]interface{})
			}
			notification.Data["provider_message_id"] = result.MessageID
		}
	} else {
		if result.Retryable && notification.ShouldRetry() {
			notification.Status = StatusQueued // Re-queue for retry
			notification.RetryCount++

			// Set next retry time if provided
			if result.RetryAfter != nil {
				retryTime := now.Add(*result.RetryAfter)
				notification.ScheduledFor = retryTime
			}
		} else {
			notification.Status = StatusFailed
			notification.FailedAt = &now
		}

		if result.Error != nil {
			errStr := result.Error.Error()
			notification.Error = &errStr
		}
	}
}

// determineProviderType determines the provider type based on token format
func (p *ProcessorWithProviders) determineProviderType(deviceToken string) PushProvider {
	// APNs tokens are 64 hex characters
	if len(deviceToken) == 64 {
		isHex := true
		for _, r := range deviceToken {
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
				isHex = false
				break
			}
		}
		if isHex {
			return PushProviderAPNs
		}
	}

	// Default to FCM
	return PushProviderFCM
}

// groupNotificationsByProvider groups notifications by their provider type
func (p *ProcessorWithProviders) groupNotificationsByProvider(notifications []*Notification) map[PushProvider][]*Notification {
	grouped := make(map[PushProvider][]*Notification)

	for _, notif := range notifications {
		if notif.DeviceToken == nil || *notif.DeviceToken == "" {
			continue
		}

		var providerType PushProvider
		if notif.PushProvider != nil {
			providerType = PushProvider(*notif.PushProvider)
		} else {
			providerType = p.determineProviderType(*notif.DeviceToken)
		}

		grouped[providerType] = append(grouped[providerType], notif)
	}

	return grouped
}

// handleInvalidToken handles an invalid or expired device token
func (p *ProcessorWithProviders) handleInvalidToken(ctx context.Context, userID uuid.UUID, token string, provider PushProvider) {
	p.logger.Info("Marking device token as invalid",
		zap.String("user_id", userID.String()),
		zap.String("provider", string(provider)),
		zap.String("token_prefix", token[:10]+"..."))

	// In a real implementation, this would:
	// 1. Mark the token as invalid in the database
	// 2. Notify the user through other channels if possible
	// 3. Clean up any related data

	// For now, just log it
	// TODO: Implement token invalidation in user device repository
}

// handleCanonicalToken handles a canonical token update from the provider
func (p *ProcessorWithProviders) handleCanonicalToken(ctx context.Context, userID uuid.UUID, oldToken, newToken string) {
	p.logger.Info("Updating device token to canonical token",
		zap.String("user_id", userID.String()),
		zap.String("old_token_prefix", oldToken[:10]+"..."),
		zap.String("new_token_prefix", newToken[:10]+"..."))

	// In a real implementation, this would:
	// 1. Update the device token in the database
	// 2. Remove the old token
	// 3. Ensure no duplicate tokens exist

	// TODO: Implement token update in user device repository
}

// GetProviderMetrics returns metrics from the provider manager
func (p *ProcessorWithProviders) GetProviderMetrics() map[string]interface{} {
	if p.providerManager != nil {
		return p.providerManager.GetMetrics()
	}
	return map[string]interface{}{}
}