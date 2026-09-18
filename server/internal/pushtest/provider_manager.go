package notification

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// ProviderManager manages multiple push notification providers
type ProviderManager struct {
	providers   map[PushProvider]PushNotificationProvider
	config      *PushProviderConfig
	logger      *zap.Logger
	metrics     *providerManagerMetrics
	mu          sync.RWMutex
	primaryFCM  PushNotificationProvider
	primaryAPNs PushNotificationProvider
}

type providerManagerMetrics struct {
	totalRoutedFCM  atomic.Int64
	totalRoutedAPNs atomic.Int64
	totalRoutedMock atomic.Int64
	totalFailed     atomic.Int64
	totalRetries    atomic.Int64
}

// NewProviderManager creates a new provider manager
func NewProviderManager(config *PushProviderConfig, logger *zap.Logger) *ProviderManager {
	if config == nil {
		config = DefaultPushProviderConfig()
	}

	return &ProviderManager{
		providers: make(map[PushProvider]PushNotificationProvider),
		config:    config,
		logger:    logger,
		metrics:   &providerManagerMetrics{},
	}
}

// RegisterProvider registers a push notification provider
func (pm *ProviderManager) RegisterProvider(provider PushNotificationProvider) error {
	if provider == nil {
		return fmt.Errorf("provider cannot be nil")
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	providerType := provider.GetProviderType()

	// Check if provider already exists
	if existing, exists := pm.providers[providerType]; exists {
		// Close the existing provider
		if err := existing.Close(); err != nil {
			pm.logger.Warn("Failed to close existing provider",
				zap.String("provider", string(providerType)),
				zap.Error(err))
		}
	}

	pm.providers[providerType] = provider

	// Set as primary provider for the type
	switch providerType {
	case PushProviderFCM:
		pm.primaryFCM = provider
	case PushProviderAPNs:
		pm.primaryAPNs = provider
	}

	pm.logger.Info("Registered push notification provider",
		zap.String("provider", string(providerType)))

	return nil
}

// InitializeProviders initializes FCM and APNs providers based on config
func (pm *ProviderManager) InitializeProviders() error {
	var initErrors []error

	// Initialize FCM if configured
	if pm.config.FCMCredentialsJSON != "" {
		fcmProvider, err := NewFCMProvider(pm.config, pm.logger)
		if err != nil {
			pm.logger.Error("Failed to initialize FCM provider", zap.Error(err))
			initErrors = append(initErrors, fmt.Errorf("FCM init failed: %w", err))
		} else {
			if err := pm.RegisterProvider(fcmProvider); err != nil {
				initErrors = append(initErrors, fmt.Errorf("FCM registration failed: %w", err))
			}
		}
	} else {
		pm.logger.Info("FCM provider not configured, skipping initialization")
	}

	// Initialize APNs if configured
	if pm.config.APNsAuthKeyPath != "" && pm.config.APNsKeyID != "" && pm.config.APNsTeamID != "" {
		apnsProvider, err := NewAPNsProvider(pm.config, pm.logger)
		if err != nil {
			pm.logger.Error("Failed to initialize APNs provider", zap.Error(err))
			initErrors = append(initErrors, fmt.Errorf("APNs init failed: %w", err))
		} else {
			if err := pm.RegisterProvider(apnsProvider); err != nil {
				initErrors = append(initErrors, fmt.Errorf("APNs registration failed: %w", err))
			}
		}
	} else {
		pm.logger.Info("APNs provider not configured, skipping initialization")
	}

	// Return error if no providers were initialized successfully
	if len(pm.providers) == 0 && len(initErrors) > 0 {
		return fmt.Errorf("failed to initialize any providers: %v", initErrors)
	}

	pm.logger.Info("Provider manager initialized",
		zap.Int("providers_count", len(pm.providers)))

	return nil
}

// Send sends a notification using the appropriate provider
func (pm *ProviderManager) Send(ctx context.Context, notification *PushNotification, providerType PushProvider) (*PushResult, error) {
	// Get the provider
	provider, err := pm.getProvider(providerType)
	if err != nil {
		pm.metrics.totalFailed.Add(1)
		return &PushResult{
			Success: false,
			Error:   err,
			SentAt:  time.Now(),
		}, err
	}

	// Track metrics
	switch providerType {
	case PushProviderFCM:
		pm.metrics.totalRoutedFCM.Add(1)
	case PushProviderAPNs:
		pm.metrics.totalRoutedAPNs.Add(1)
	case PushProviderMock:
		pm.metrics.totalRoutedMock.Add(1)
	}

	// Send with retry logic
	var lastError error
	var result *PushResult

	for attempt := 0; attempt <= pm.config.MaxRetries; attempt++ {
		if attempt > 0 {
			pm.metrics.totalRetries.Add(1)

			// Calculate retry delay
			delay := GetRetryDelay(attempt, pm.config.RetryBackoff)

			// Check if we have a specific retry delay from the provider
			if result != nil && result.RetryAfter != nil {
				delay = *result.RetryAfter
			}

			pm.logger.Debug("Retrying notification send",
				zap.String("provider", string(providerType)),
				zap.Int("attempt", attempt),
				zap.Duration("delay", delay))

			// Wait before retry
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return &PushResult{
					Success: false,
					Error:   ctx.Err(),
					SentAt:  time.Now(),
				}, ctx.Err()
			}
		}

		// Attempt to send
		result, lastError = provider.Send(ctx, notification)

		// Check if successful or non-retryable error
		if result != nil && (result.Success || !result.Retryable) {
			return result, lastError
		}

		// Check if token should be removed
		if result != nil && result.ShouldRemoveToken {
			pm.logger.Info("Device token should be removed",
				zap.String("provider", string(providerType)),
				zap.String("token", notification.DeviceToken[:10]+"...")) // Log partial token for security
			return result, lastError
		}
	}

	// All retries exhausted
	pm.metrics.totalFailed.Add(1)

	if result == nil {
		result = &PushResult{
			Success: false,
			Error:   lastError,
			SentAt:  time.Now(),
		}
	}

	return result, lastError
}

// SendToDevice sends a notification to a device, automatically selecting the provider
func (pm *ProviderManager) SendToDevice(ctx context.Context, deviceToken string, notification *PushNotification) (*PushResult, error) {
	// Copy notification and set the device token
	notif := *notification
	notif.DeviceToken = deviceToken

	// Determine provider based on token format
	providerType := pm.determineProviderType(deviceToken)

	return pm.Send(ctx, &notif, providerType)
}

// SendBatch sends multiple notifications using the appropriate provider
func (pm *ProviderManager) SendBatch(ctx context.Context, notifications []*PushNotification, providerType PushProvider) ([]*PushResult, error) {
	provider, err := pm.getProvider(providerType)
	if err != nil {
		// Return error results for all notifications
		results := make([]*PushResult, len(notifications))
		for i := range results {
			results[i] = &PushResult{
				Success: false,
				Error:   err,
				SentAt:  time.Now(),
			}
		}
		return results, err
	}

	return provider.SendBatch(ctx, notifications)
}

// SendMultiProvider sends the same notification to multiple providers
func (pm *ProviderManager) SendMultiProvider(ctx context.Context, notification *PushNotification, providers []PushProvider) map[PushProvider]*PushResult {
	results := make(map[PushProvider]*PushResult)
	var wg sync.WaitGroup

	for _, providerType := range providers {
		wg.Add(1)
		go func(pt PushProvider) {
			defer wg.Done()

			result, _ := pm.Send(ctx, notification, pt)
			pm.mu.Lock()
			results[pt] = result
			pm.mu.Unlock()
		}(providerType)
	}

	wg.Wait()
	return results
}

// GetProvider gets a specific provider
func (pm *ProviderManager) GetProvider(providerType PushProvider) (PushNotificationProvider, error) {
	return pm.getProvider(providerType)
}

// GetAvailableProviders returns a list of available providers
func (pm *ProviderManager) GetAvailableProviders(ctx context.Context) []PushProvider {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var available []PushProvider
	for providerType, provider := range pm.providers {
		if provider.IsAvailable(ctx) {
			available = append(available, providerType)
		}
	}

	return available
}

// GetMetrics returns aggregated metrics from all providers
func (pm *ProviderManager) GetMetrics() map[string]interface{} {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	metrics := map[string]interface{}{
		"total_routed_fcm":  pm.metrics.totalRoutedFCM.Load(),
		"total_routed_apns": pm.metrics.totalRoutedAPNs.Load(),
		"total_routed_mock": pm.metrics.totalRoutedMock.Load(),
		"total_failed":      pm.metrics.totalFailed.Load(),
		"total_retries":     pm.metrics.totalRetries.Load(),
		"providers":         make(map[string]interface{}),
	}

	// Collect metrics from each provider
	for providerType, provider := range pm.providers {
		metrics["providers"].(map[string]interface{})[string(providerType)] = provider.GetMetrics()
	}

	return metrics
}

// Close closes all registered providers
func (pm *ProviderManager) Close() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	var closeErrors []error

	for providerType, provider := range pm.providers {
		if err := provider.Close(); err != nil {
			closeErrors = append(closeErrors, fmt.Errorf("failed to close %s: %w", providerType, err))
		}
		delete(pm.providers, providerType)
	}

	if len(closeErrors) > 0 {
		return fmt.Errorf("errors closing providers: %v", closeErrors)
	}

	pm.logger.Info("Provider manager closed successfully")
	return nil
}

// ValidateToken validates a device token for a specific provider
func (pm *ProviderManager) ValidateToken(token string, providerType PushProvider) error {
	provider, err := pm.getProvider(providerType)
	if err != nil {
		return err
	}

	return provider.ValidateToken(token)
}

// Private helper methods

func (pm *ProviderManager) getProvider(providerType PushProvider) (PushNotificationProvider, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	provider, exists := pm.providers[providerType]
	if !exists {
		return nil, fmt.Errorf("provider %s not registered", providerType)
	}

	if !provider.IsAvailable(context.Background()) {
		return nil, ErrProviderUnavailable
	}

	return provider, nil
}

func (pm *ProviderManager) determineProviderType(deviceToken string) PushProvider {
	// Simple heuristic to determine provider type based on token format
	// APNs tokens are 64 hex characters
	// FCM tokens are longer and contain different characters

	if len(deviceToken) == 64 {
		// Check if all characters are hex
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

	// Default to FCM for longer tokens or tokens with special characters
	return PushProviderFCM
}

// ProviderFactory creates providers based on configuration
type ProviderFactory struct {
	logger *zap.Logger
}

// NewProviderFactory creates a new provider factory
func NewProviderFactory(logger *zap.Logger) *ProviderFactory {
	return &ProviderFactory{
		logger: logger,
	}
}

// CreateProvider creates a provider based on type and config
func (f *ProviderFactory) CreateProvider(providerType PushProvider, config *PushProviderConfig) (PushNotificationProvider, error) {
	switch providerType {
	case PushProviderFCM:
		return NewFCMProvider(config, f.logger)

	case PushProviderAPNs:
		return NewAPNsProvider(config, f.logger)

	case PushProviderMock:
		mockConfig := &MockProviderConfig{
			AlwaysSucceed: true,
		}
		return NewMockProvider(mockConfig), nil

	default:
		return nil, fmt.Errorf("unknown provider type: %s", providerType)
	}
}

// CreateFromEnvironment creates providers based on environment configuration
func (f *ProviderFactory) CreateFromEnvironment(config *PushProviderConfig) ([]PushNotificationProvider, error) {
	var providers []PushNotificationProvider
	var errors []error

	// Try to create FCM provider
	if config.FCMCredentialsJSON != "" {
		fcmProvider, err := NewFCMProvider(config, f.logger)
		if err != nil {
			errors = append(errors, fmt.Errorf("FCM: %w", err))
		} else {
			providers = append(providers, fcmProvider)
		}
	}

	// Try to create APNs provider
	if config.APNsAuthKeyPath != "" {
		apnsProvider, err := NewAPNsProvider(config, f.logger)
		if err != nil {
			errors = append(errors, fmt.Errorf("APNs: %w", err))
		} else {
			providers = append(providers, apnsProvider)
		}
	}

	if len(providers) == 0 && len(errors) > 0 {
		return nil, fmt.Errorf("failed to create any providers: %v", errors)
	}

	return providers, nil
}