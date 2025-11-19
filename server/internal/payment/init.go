package payment

import (
	"context"
	"fmt"

	"github.com/dev-jelly/donelist/internal/config"
	"go.uber.org/zap"
)

// Manager holds all payment-related services
type Manager struct {
	Client         *StripeClient
	Service        *Service
	ProductManager *ProductManager
	WebhookHandler *WebhookHandler
	logger         *zap.Logger
}

// NewManager creates a new payment manager with all services initialized
func NewManager(cfg *config.StripeConfig, logger *zap.Logger) (*Manager, error) {
	if cfg.SecretKey == "" {
		logger.Warn("Stripe secret key not configured, payment features will be disabled")
		return nil, nil
	}

	// Initialize Stripe client
	client, err := NewStripeClient(&Config{
		SecretKey:     cfg.SecretKey,
		WebhookSecret: cfg.WebhookSecret,
		Logger:        logger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Stripe client: %w", err)
	}

	// Initialize services
	service := NewService(client, logger)
	productManager := NewProductManager(client, logger)
	webhookHandler := NewWebhookHandler(service, logger)

	manager := &Manager{
		Client:         client,
		Service:        service,
		ProductManager: productManager,
		WebhookHandler: webhookHandler,
		logger:         logger,
	}

	// Log initialization details
	if client.IsTestMode() {
		logger.Info("Stripe initialized in TEST mode")
	} else {
		logger.Warn("Stripe initialized in LIVE mode - real charges will be made!")
	}

	// Optionally sync products on startup
	if cfg.SyncProductsOnStartup {
		if err := manager.SyncProducts(context.Background()); err != nil {
			logger.Error("Failed to sync products with Stripe", zap.Error(err))
			// Don't fail initialization if product sync fails
		}
	}

	return manager, nil
}

// SyncProducts syncs local plan definitions with Stripe
func (m *Manager) SyncProducts(ctx context.Context) error {
	if m == nil || m.ProductManager == nil {
		return fmt.Errorf("payment manager not initialized")
	}

	m.logger.Info("Starting Stripe product synchronization")
	if err := m.ProductManager.SyncProducts(ctx); err != nil {
		return fmt.Errorf("product sync failed: %w", err)
	}

	m.logger.Info("Stripe product synchronization completed")
	return nil
}

// IsEnabled returns whether payment functionality is enabled
func (m *Manager) IsEnabled() bool {
	return m != nil && m.Client != nil
}

// IsTestMode returns whether Stripe is in test mode
func (m *Manager) IsTestMode() bool {
	if !m.IsEnabled() {
		return true // Default to test mode if not configured
	}
	return m.Client.IsTestMode()
}

// Shutdown performs any cleanup needed
func (m *Manager) Shutdown(ctx context.Context) error {
	if !m.IsEnabled() {
		return nil
	}

	m.logger.Info("Shutting down payment manager")
	// Add any cleanup logic here if needed
	return nil
}