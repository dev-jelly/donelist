package integration

import (
	"testing"

	"github.com/dev-jelly/donelist/internal/payment"
	"github.com/dev-jelly/donelist/internal/subscription"
	"github.com/dev-jelly/donelist/internal/testutil"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// PaymentTestEnvironment holds all dependencies for payment integration tests
type PaymentTestEnvironment struct {
	TestDB           *testutil.TestDB
	DB               *sqlx.DB
	Logger           *zap.Logger
	SubscriptionRepo *subscription.Repository
	PaymentService   *payment.Service
	WebhookHandler   *payment.WebhookHandler
	StripeClient     *payment.StripeClient
}

// setupTestEnvironment creates a test environment with all necessary dependencies
func setupTestEnvironment(t *testing.T) *PaymentTestEnvironment {
	// Create test database connection
	testDB := testutil.SetupTestDB(t)

	// Create logger
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	// Create repositories
	subscriptionRepo := subscription.NewRepository(testDB.DB)

	// Create Stripe client with test configuration
	// Note: These should be test/mock credentials
	stripeClient, err := payment.NewStripeClient(&payment.Config{
		SecretKey:     "sk_test_mock_key",
		WebhookSecret: "whsec_test_mock_secret",
		Logger:        logger,
	})
	require.NoError(t, err)

	// Create payment service
	paymentService := payment.NewService(stripeClient, logger)

	// Create webhook handler
	webhookHandler := payment.NewWebhookHandler(
		paymentService,
		subscriptionRepo,
		logger,
	)

	return &PaymentTestEnvironment{
		TestDB:           testDB,
		DB:               testDB.DB,
		Logger:           logger,
		SubscriptionRepo: subscriptionRepo,
		PaymentService:   paymentService,
		WebhookHandler:   webhookHandler,
		StripeClient:     stripeClient,
	}
}

// Cleanup cleans up the test environment
func (e *PaymentTestEnvironment) Cleanup() {
	if e.TestDB != nil {
		e.TestDB.TearDown(nil) // Pass nil since we don't have t here
	}
}
