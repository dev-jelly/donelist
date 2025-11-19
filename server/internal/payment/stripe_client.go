package payment

import (
	"fmt"
	"os"

	"github.com/stripe/stripe-go/v79"
	"github.com/stripe/stripe-go/v79/client"
	"go.uber.org/zap"
)

// StripeClient wraps the Stripe API client
type StripeClient struct {
	client       *client.API
	logger       *zap.Logger
	webhookSecret string
	isTestMode   bool
}

// Config holds the Stripe configuration
type Config struct {
	SecretKey     string
	WebhookSecret string
	Logger        *zap.Logger
}

// NewStripeClient creates a new Stripe client with configuration
func NewStripeClient(cfg *Config) (*StripeClient, error) {
	if cfg.SecretKey == "" {
		return nil, fmt.Errorf("stripe secret key is required")
	}

	// Set the Stripe API key globally
	stripe.Key = cfg.SecretKey

	// Create a new Stripe client with custom configuration
	config := &stripe.BackendConfig{
		MaxNetworkRetries: stripe.Int64(2),
		LeveledLogger:     &stripeLogger{logger: cfg.Logger},
	}

	// Initialize the Stripe client
	sc := &client.API{}
	sc.Init(cfg.SecretKey, &stripe.Backends{
		API:     stripe.GetBackendWithConfig(stripe.APIBackend, config),
		Uploads: stripe.GetBackendWithConfig(stripe.UploadsBackend, config),
	})

	// Determine if we're in test mode based on the key prefix
	isTestMode := len(cfg.SecretKey) > 7 && cfg.SecretKey[:7] == "sk_test"

	return &StripeClient{
		client:       sc,
		logger:       cfg.Logger,
		webhookSecret: cfg.WebhookSecret,
		isTestMode:   isTestMode,
	}, nil
}

// NewStripeClientFromEnv creates a Stripe client from environment variables
func NewStripeClientFromEnv(logger *zap.Logger) (*StripeClient, error) {
	secretKey := os.Getenv("STRIPE_SECRET_KEY")
	webhookSecret := os.Getenv("STRIPE_WEBHOOK_SECRET")

	if secretKey == "" {
		return nil, fmt.Errorf("STRIPE_SECRET_KEY environment variable is not set")
	}

	return NewStripeClient(&Config{
		SecretKey:     secretKey,
		WebhookSecret: webhookSecret,
		Logger:        logger,
	})
}

// GetClient returns the underlying Stripe client
func (s *StripeClient) GetClient() *client.API {
	return s.client
}

// IsTestMode returns whether the client is using test keys
func (s *StripeClient) IsTestMode() bool {
	return s.isTestMode
}

// GetWebhookSecret returns the webhook secret for signature verification
func (s *StripeClient) GetWebhookSecret() string {
	return s.webhookSecret
}

// stripeLogger implements Stripe's LeveledLogger interface
type stripeLogger struct {
	logger *zap.Logger
}

// Debugf logs a debug message
func (l *stripeLogger) Debugf(format string, v ...interface{}) {
	if l.logger != nil {
		l.logger.Debug(fmt.Sprintf(format, v...))
	}
}

// Errorf logs an error message
func (l *stripeLogger) Errorf(format string, v ...interface{}) {
	if l.logger != nil {
		l.logger.Error(fmt.Sprintf(format, v...))
	}
}

// Infof logs an info message
func (l *stripeLogger) Infof(format string, v ...interface{}) {
	if l.logger != nil {
		l.logger.Info(fmt.Sprintf(format, v...))
	}
}

// Warnf logs a warning message
func (l *stripeLogger) Warnf(format string, v ...interface{}) {
	if l.logger != nil {
		l.logger.Warn(fmt.Sprintf(format, v...))
	}
}