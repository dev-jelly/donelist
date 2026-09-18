package subscriptionjobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dev-jelly/donelist/internal/subscription"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// MockPaymentRetryService is a mock implementation of PaymentRetryService
type MockPaymentRetryService struct {
	retryError error
	retryCount int
}

func (m *MockPaymentRetryService) RetryPayment(ctx context.Context, subscriptionID uuid.UUID) error {
	m.retryCount++
	return m.retryError
}

func setupDunningTestDB(t *testing.T) (*sqlx.DB, func()) {
	db, err := sqlx.Connect("postgres", "postgres://localhost/donelist_test?sslmode=disable")
	if err != nil {
		t.Skip("Skipping test: PostgreSQL not available")
	}

	// Create test tables
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS subscriptions (
			id UUID PRIMARY KEY,
			user_id UUID NOT NULL,
			plan_id VARCHAR(50) NOT NULL,
			status VARCHAR(50) NOT NULL,
			current_period_start TIMESTAMP NOT NULL,
			current_period_end TIMESTAMP NOT NULL,
			cancel_at_period_end BOOLEAN DEFAULT false,
			canceled_at TIMESTAMP,
			trial_start TIMESTAMP,
			trial_end TIMESTAMP,
			stripe_customer_id VARCHAR(255),
			stripe_subscription_id VARCHAR(255),
			metadata JSONB,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)
	`)
	require.NoError(t, err)

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS subscription_events (
			id UUID PRIMARY KEY,
			subscription_id UUID NOT NULL,
			event_type VARCHAR(100) NOT NULL,
			previous_value JSONB,
			new_value JSONB,
			reason TEXT,
			created_at TIMESTAMP NOT NULL
		)
	`)
	require.NoError(t, err)

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS dunning_attempts (
			id UUID PRIMARY KEY,
			subscription_id UUID NOT NULL,
			payment_id UUID,
			attempt_number INT NOT NULL,
			status VARCHAR(50) NOT NULL,
			scheduled_for TIMESTAMP NOT NULL,
			attempted_at TIMESTAMP,
			succeeded_at TIMESTAMP,
			failed_at TIMESTAMP,
			failure_reason TEXT,
			next_retry_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			UNIQUE(subscription_id, attempt_number)
		)
	`)
	require.NoError(t, err)

	cleanup := func() {
		db.Exec("DROP TABLE IF EXISTS dunning_attempts")
		db.Exec("DROP TABLE IF EXISTS subscription_events")
		db.Exec("DROP TABLE IF EXISTS subscriptions")
		db.Close()
	}

	return db, cleanup
}

func TestDunningService_CreateDunningSequence(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db, cleanup := setupDunningTestDB(t)
	defer cleanup()

	logger := zaptest.NewLogger(t)
	repo := NewRepository(db)
	queue := &MockNotificationQueue{}
	paymentService := &MockPaymentRetryService{}
	service := NewDunningService(db, repo, queue, paymentService, logger)

	ctx := context.Background()
	now := time.Now()

	// Create past due subscription
	sub := &Subscription{
		ID:                 uuid.New(),
		UserID:             uuid.New(),
		PlanID:             string(subscription.PlanPremium),
		Status:             subscription.StatusPastDue,
		CurrentPeriodStart: now.Add(-30 * 24 * time.Hour),
		CurrentPeriodEnd:   now.Add(1 * 24 * time.Hour),
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	err := repo.CreateSubscription(ctx, sub)
	require.NoError(t, err)

	config := DefaultDunningConfig()
	err = service.createDunningSequence(ctx, sub, config)
	require.NoError(t, err)

	// Verify dunning attempts were created
	var attempts []*DunningAttempt
	err = db.SelectContext(ctx, &attempts,
		"SELECT * FROM dunning_attempts WHERE subscription_id = $1 ORDER BY attempt_number",
		sub.ID)
	require.NoError(t, err)

	assert.Len(t, attempts, config.MaxRetries, "Expected correct number of dunning attempts")

	for i, attempt := range attempts {
		assert.Equal(t, i+1, attempt.AttemptNumber)
		assert.Equal(t, DunningStatusPending, attempt.Status)
		assert.True(t, attempt.ScheduledFor.After(now))
	}

	// Verify notification was sent
	assert.NotEmpty(t, queue.notifications, "Expected payment failure notification")
}

func TestDunningService_ProcessDunningAttempt_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db, cleanup := setupDunningTestDB(t)
	defer cleanup()

	logger := zaptest.NewLogger(t)
	repo := NewRepository(db)
	queue := &MockNotificationQueue{}
	paymentService := &MockPaymentRetryService{retryError: nil} // Success
	service := NewDunningService(db, repo, queue, paymentService, logger)

	ctx := context.Background()
	now := time.Now()

	// Create subscription and dunning attempt
	sub := &Subscription{
		ID:                 uuid.New(),
		UserID:             uuid.New(),
		PlanID:             string(subscription.PlanPremium),
		Status:             subscription.StatusPastDue,
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   now.Add(30 * 24 * time.Hour),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	err := repo.CreateSubscription(ctx, sub)
	require.NoError(t, err)

	attempt := &DunningAttempt{
		ID:             uuid.New(),
		SubscriptionID: sub.ID,
		AttemptNumber:  1,
		Status:         DunningStatusPending,
		ScheduledFor:   now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	err = service.createDunningAttempt(ctx, attempt)
	require.NoError(t, err)

	config := DefaultDunningConfig()
	err = service.processDunningAttempt(ctx, attempt, config)
	require.NoError(t, err)

	// Verify payment retry was called
	assert.Equal(t, 1, paymentService.retryCount)

	// Verify attempt was marked as succeeded
	var updatedAttempt DunningAttempt
	err = db.GetContext(ctx, &updatedAttempt,
		"SELECT * FROM dunning_attempts WHERE id = $1", attempt.ID)
	require.NoError(t, err)
	assert.Equal(t, DunningStatusSucceeded, updatedAttempt.Status)
	assert.NotNil(t, updatedAttempt.SucceededAt)

	// Verify success notification was sent
	assert.NotEmpty(t, queue.notifications)
	lastNotif := queue.notifications[len(queue.notifications)-1]
	assert.Contains(t, lastNotif.Title, "Successful")
}

func TestDunningService_ProcessDunningAttempt_Failure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db, cleanup := setupDunningTestDB(t)
	defer cleanup()

	logger := zaptest.NewLogger(t)
	repo := NewRepository(db)
	queue := &MockNotificationQueue{}
	paymentService := &MockPaymentRetryService{
		retryError: errors.New("insufficient funds"),
	}
	service := NewDunningService(db, repo, queue, paymentService, logger)

	ctx := context.Background()
	now := time.Now()

	// Create subscription and dunning attempt
	sub := &Subscription{
		ID:                 uuid.New(),
		UserID:             uuid.New(),
		PlanID:             string(subscription.PlanPremium),
		Status:             subscription.StatusPastDue,
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   now.Add(30 * 24 * time.Hour),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	err := repo.CreateSubscription(ctx, sub)
	require.NoError(t, err)

	attempt := &DunningAttempt{
		ID:             uuid.New(),
		SubscriptionID: sub.ID,
		AttemptNumber:  1,
		Status:         DunningStatusPending,
		ScheduledFor:   now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	err = service.createDunningAttempt(ctx, attempt)
	require.NoError(t, err)

	config := DefaultDunningConfig()
	err = service.processDunningAttempt(ctx, attempt, config)
	require.NoError(t, err) // Should not return error, just mark as failed

	// Verify attempt was marked as failed
	var updatedAttempt DunningAttempt
	err = db.GetContext(ctx, &updatedAttempt,
		"SELECT * FROM dunning_attempts WHERE id = $1", attempt.ID)
	require.NoError(t, err)
	assert.Equal(t, DunningStatusFailed, updatedAttempt.Status)
	assert.NotNil(t, updatedAttempt.FailedAt)
	assert.NotNil(t, updatedAttempt.FailureReason)
	assert.Contains(t, *updatedAttempt.FailureReason, "insufficient funds")

	// Verify failure notification was sent
	assert.NotEmpty(t, queue.notifications)
}

func TestDunningService_ProcessNewFailures(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db, cleanup := setupDunningTestDB(t)
	defer cleanup()

	logger := zaptest.NewLogger(t)
	repo := NewRepository(db)
	queue := &MockNotificationQueue{}
	paymentService := &MockPaymentRetryService{}
	service := NewDunningService(db, repo, queue, paymentService, logger)

	ctx := context.Background()
	now := time.Now()

	// Create multiple past due subscriptions
	subs := []*Subscription{
		{
			ID:                 uuid.New(),
			UserID:             uuid.New(),
			PlanID:             string(subscription.PlanPremium),
			Status:             subscription.StatusPastDue,
			CurrentPeriodStart: now,
			CurrentPeriodEnd:   now.Add(30 * 24 * time.Hour),
			CreatedAt:          now,
			UpdatedAt:          now,
		},
		{
			ID:                 uuid.New(),
			UserID:             uuid.New(),
			PlanID:             string(subscription.PlanPremium),
			Status:             subscription.StatusPastDue,
			CurrentPeriodStart: now,
			CurrentPeriodEnd:   now.Add(30 * 24 * time.Hour),
			CreatedAt:          now,
			UpdatedAt:          now,
		},
	}

	for _, sub := range subs {
		err := repo.CreateSubscription(ctx, sub)
		require.NoError(t, err)
	}

	config := DefaultDunningConfig()
	err := service.processNewFailures(ctx, config)
	require.NoError(t, err)

	// Verify dunning attempts were created for all subscriptions
	var totalAttempts int
	err = db.GetContext(ctx, &totalAttempts,
		"SELECT COUNT(*) FROM dunning_attempts")
	require.NoError(t, err)
	assert.Equal(t, len(subs)*config.MaxRetries, totalAttempts)
}

func TestDunningService_ProcessGracePeriodExpiration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db, cleanup := setupDunningTestDB(t)
	defer cleanup()

	logger := zaptest.NewLogger(t)
	repo := NewRepository(db)
	queue := &MockNotificationQueue{}
	paymentService := &MockPaymentRetryService{}
	service := NewDunningService(db, repo, queue, paymentService, logger)

	ctx := context.Background()
	now := time.Now()

	// Create past due subscription
	sub := &Subscription{
		ID:                 uuid.New(),
		UserID:             uuid.New(),
		PlanID:             string(subscription.PlanPremium),
		Status:             subscription.StatusPastDue,
		CurrentPeriodStart: now.Add(-30 * 24 * time.Hour),
		CurrentPeriodEnd:   now.Add(1 * 24 * time.Hour),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	err := repo.CreateSubscription(ctx, sub)
	require.NoError(t, err)

	// Create all dunning attempts as failed (past grace period)
	config := DefaultDunningConfig()
	pastGracePeriod := now.Add(-time.Duration(config.GracePeriodDays+1) * 24 * time.Hour)

	for i := 0; i < config.MaxRetries; i++ {
		attempt := &DunningAttempt{
			ID:             uuid.New(),
			SubscriptionID: sub.ID,
			AttemptNumber:  i + 1,
			Status:         DunningStatusFailed,
			ScheduledFor:   pastGracePeriod,
			AttemptedAt:    &pastGracePeriod,
			FailedAt:       &pastGracePeriod,
			CreatedAt:      pastGracePeriod,
			UpdatedAt:      pastGracePeriod,
		}
		err = service.createDunningAttempt(ctx, attempt)
		require.NoError(t, err)
	}

	// Process grace period expirations
	err = service.processGracePeriodExpirations(ctx, config)
	require.NoError(t, err)

	// Verify subscription was expired
	updatedSub, err := repo.GetSubscription(ctx, sub.ID)
	require.NoError(t, err)
	assert.Equal(t, subscription.StatusExpired, updatedSub.Status)

	// Verify notification was sent
	assert.NotEmpty(t, queue.notifications)
	lastNotif := queue.notifications[len(queue.notifications)-1]
	assert.Contains(t, lastNotif.Body, "canceled")
}

func TestDunningService_GetDunningStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db, cleanup := setupDunningTestDB(t)
	defer cleanup()

	logger := zaptest.NewLogger(t)
	repo := NewRepository(db)
	queue := &MockNotificationQueue{}
	paymentService := &MockPaymentRetryService{}
	service := NewDunningService(db, repo, queue, paymentService, logger)

	ctx := context.Background()
	now := time.Now()
	subID := uuid.New()

	// Create dunning attempts with various statuses
	attempts := []*DunningAttempt{
		{
			ID:             uuid.New(),
			SubscriptionID: subID,
			AttemptNumber:  1,
			Status:         DunningStatusFailed,
			ScheduledFor:   now,
			FailedAt:       &now,
			CreatedAt:      now,
			UpdatedAt:      now,
		},
		{
			ID:             uuid.New(),
			SubscriptionID: subID,
			AttemptNumber:  2,
			Status:         DunningStatusPending,
			ScheduledFor:   now.Add(3 * 24 * time.Hour),
			CreatedAt:      now,
			UpdatedAt:      now,
		},
	}

	for _, attempt := range attempts {
		err := service.createDunningAttempt(ctx, attempt)
		require.NoError(t, err)
	}

	status, err := service.GetDunningStatus(ctx, subID)
	require.NoError(t, err)

	assert.Equal(t, subID.String(), status["subscription_id"])
	assert.Equal(t, 2, status["total_attempts"])
	assert.Equal(t, 1, status["pending_attempts"])
	assert.Equal(t, 1, status["failed_attempts"])
	assert.True(t, status["in_grace_period"].(bool))
}

func TestDunningService_CancelRemainingAttempts(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db, cleanup := setupDunningTestDB(t)
	defer cleanup()

	logger := zaptest.NewLogger(t)
	repo := NewRepository(db)
	queue := &MockNotificationQueue{}
	paymentService := &MockPaymentRetryService{}
	service := NewDunningService(db, repo, queue, paymentService, logger)

	ctx := context.Background()
	now := time.Now()
	subID := uuid.New()

	// Create multiple pending attempts
	for i := 0; i < 3; i++ {
		attempt := &DunningAttempt{
			ID:             uuid.New(),
			SubscriptionID: subID,
			AttemptNumber:  i + 1,
			Status:         DunningStatusPending,
			ScheduledFor:   now.Add(time.Duration(i) * 24 * time.Hour),
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		err := service.createDunningAttempt(ctx, attempt)
		require.NoError(t, err)
	}

	// Cancel remaining attempts
	err := service.cancelRemainingAttempts(ctx, subID)
	require.NoError(t, err)

	// Verify all attempts were canceled
	var attempts []*DunningAttempt
	err = db.SelectContext(ctx, &attempts,
		"SELECT * FROM dunning_attempts WHERE subscription_id = $1", subID)
	require.NoError(t, err)

	for _, attempt := range attempts {
		assert.Equal(t, DunningStatusCanceled, attempt.Status)
	}
}

func TestDefaultDunningConfig(t *testing.T) {
	config := DefaultDunningConfig()

	assert.Equal(t, 3, config.MaxRetries)
	assert.Len(t, config.RetryIntervals, 3)
	assert.Equal(t, 7, config.GracePeriodDays)
	assert.True(t, config.EnableNotifications)
	assert.True(t, config.AutoCancelAfterGrace)

	// Verify retry intervals are increasing
	for i := 1; i < len(config.RetryIntervals); i++ {
		assert.True(t, config.RetryIntervals[i] > config.RetryIntervals[i-1])
	}
}
