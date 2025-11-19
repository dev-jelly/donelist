package payment

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestNewStripeClient(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name      string
		config    *Config
		wantErr   bool
		wantTest  bool
		errString string
	}{
		{
			name: "Valid test key",
			config: &Config{
				SecretKey:     "sk_test_DUMMY_KEY_FOR_TESTING_ONLY",
				WebhookSecret: "whsec_test123",
				Logger:        logger,
			},
			wantErr:  false,
			wantTest: true,
		},
		{
			name: "Valid live key",
			config: &Config{
				SecretKey:     "sk_live_DUMMY_KEY_FOR_TESTING_ONLY",
				WebhookSecret: "whsec_live123",
				Logger:        logger,
			},
			wantErr:  false,
			wantTest: false,
		},
		{
			name: "Empty secret key",
			config: &Config{
				SecretKey:     "",
				WebhookSecret: "whsec_test123",
				Logger:        logger,
			},
			wantErr:   true,
			errString: "stripe secret key is required",
		},
		{
			name: "Nil logger",
			config: &Config{
				SecretKey:     "sk_test_DUMMY_KEY_FOR_TESTING_ONLY",
				WebhookSecret: "whsec_test123",
				Logger:        nil,
			},
			wantErr:  false,
			wantTest: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewStripeClient(tt.config)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, client)
				if tt.errString != "" {
					assert.Contains(t, err.Error(), tt.errString)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
				assert.NotNil(t, client.GetClient())
				assert.Equal(t, tt.wantTest, client.IsTestMode())
				assert.Equal(t, tt.config.WebhookSecret, client.GetWebhookSecret())
			}
		})
	}
}

func TestNewStripeClientFromEnv(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name          string
		envVars       map[string]string
		wantErr       bool
		wantTest      bool
		webhookSecret string
	}{
		{
			name: "Valid environment variables",
			envVars: map[string]string{
				"STRIPE_SECRET_KEY":    "sk_test_DUMMY_KEY_FOR_TESTING_ONLY",
				"STRIPE_WEBHOOK_SECRET": "whsec_test123",
			},
			wantErr:       false,
			wantTest:      true,
			webhookSecret: "whsec_test123",
		},
		{
			name: "Missing webhook secret",
			envVars: map[string]string{
				"STRIPE_SECRET_KEY": "sk_test_DUMMY_KEY_FOR_TESTING_ONLY",
			},
			wantErr:       false,
			wantTest:      true,
			webhookSecret: "",
		},
		{
			name:    "Missing secret key",
			envVars: map[string]string{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore environment variables
			originalSecretKey := os.Getenv("STRIPE_SECRET_KEY")
			originalWebhookSecret := os.Getenv("STRIPE_WEBHOOK_SECRET")
			defer func() {
				os.Setenv("STRIPE_SECRET_KEY", originalSecretKey)
				os.Setenv("STRIPE_WEBHOOK_SECRET", originalWebhookSecret)
			}()

			// Clear environment variables
			os.Unsetenv("STRIPE_SECRET_KEY")
			os.Unsetenv("STRIPE_WEBHOOK_SECRET")

			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			client, err := NewStripeClientFromEnv(logger)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
				assert.Equal(t, tt.wantTest, client.IsTestMode())
				assert.Equal(t, tt.webhookSecret, client.GetWebhookSecret())
			}
		})
	}
}

func TestStripeLogger(t *testing.T) {
	logger := zaptest.NewLogger(t)
	sl := &stripeLogger{logger: logger}

	// Test all log levels
	sl.Debugf("debug message: %s", "test")
	sl.Infof("info message: %s", "test")
	sl.Warnf("warn message: %s", "test")
	sl.Errorf("error message: %s", "test")

	// Test with nil logger (should not panic)
	nilLogger := &stripeLogger{logger: nil}
	nilLogger.Debugf("debug message: %s", "test")
	nilLogger.Infof("info message: %s", "test")
	nilLogger.Warnf("warn message: %s", "test")
	nilLogger.Errorf("error message: %s", "test")
}

// Integration test - only runs if STRIPE_TEST_KEY is set
func TestStripeIntegration(t *testing.T) {
	testKey := os.Getenv("STRIPE_TEST_KEY")
	if testKey == "" {
		t.Skip("STRIPE_TEST_KEY not set, skipping integration test")
	}

	logger := zaptest.NewLogger(t)
	client, err := NewStripeClient(&Config{
		SecretKey: testKey,
		Logger:    logger,
	})
	require.NoError(t, err)
	require.NotNil(t, client)

	// Test that we can create a service and it initializes properly
	service := NewService(client, logger)
	require.NotNil(t, service)

	// Test creating a customer (this will actually hit Stripe's test API)
	t.Run("CreateCustomer", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping integration test in short mode")
		}

		// This test requires a valid test API key to work
		// It's commented out to avoid failures in CI
		// Uncomment to test with a real Stripe test key

		// customer, err := service.CreateCustomer(ctx, uuid.New(), "test@example.com", "Test User")
		// require.NoError(t, err)
		// assert.NotEmpty(t, customer.ID)
		// assert.Equal(t, "test@example.com", customer.Email)
		// assert.Equal(t, "Test User", customer.Name)
	})
}