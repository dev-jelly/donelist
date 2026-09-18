package subscriptionjobs

import (
	"context"
	"testing"
	"time"

	"github.com/dev-jelly/donelist/internal/notification"
	"github.com/dev-jelly/donelist/internal/subscription"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// MockNotificationQueue is a mock implementation of NotificationQueue
type MockNotificationQueue struct {
	notifications []*notification.Notification
	sendError     error
}

func (m *MockNotificationQueue) SendNotification(ctx context.Context, notif *notification.Notification) error {
	if m.sendError != nil {
		return m.sendError
	}
	m.notifications = append(m.notifications, notif)
	return nil
}

func (m *MockNotificationQueue) Reset() {
	m.notifications = nil
	m.sendError = nil
}

func setupNotificationTestDB(t *testing.T) (*sqlx.DB, func()) {
	// Use in-memory SQLite for testing
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
		CREATE TABLE IF NOT EXISTS subscription_notifications (
			id UUID PRIMARY KEY,
			subscription_id UUID NOT NULL,
			notification_type VARCHAR(50) NOT NULL,
			days_before_expiry INT DEFAULT 0,
			days_after_expiry INT DEFAULT 0,
			sent_at TIMESTAMP NOT NULL,
			UNIQUE(subscription_id, notification_type, days_before_expiry, days_after_expiry)
		)
	`)
	require.NoError(t, err)

	cleanup := func() {
		db.Exec("DROP TABLE IF EXISTS subscription_notifications")
		db.Exec("DROP TABLE IF EXISTS subscriptions")
		db.Close()
	}

	return db, cleanup
}

func TestNotificationService_ProcessPreExpiryNotifications(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db, cleanup := setupNotificationTestDB(t)
	defer cleanup()

	logger := zaptest.NewLogger(t)
	repo := NewRepository(db)
	queue := &MockNotificationQueue{}
	service := NewNotificationService(db, repo, queue, logger)

	ctx := context.Background()

	// Create test subscriptions
	now := time.Now()
	testCases := []struct {
		name       string
		expiresIn  time.Duration
		shouldSend bool
	}{
		{"expires_in_7_days", 7 * 24 * time.Hour, true},
		{"expires_in_3_days", 3 * 24 * time.Hour, true},
		{"expires_in_1_day", 24 * time.Hour, true},
		{"expires_in_10_days", 10 * 24 * time.Hour, false},
		{"expires_in_5_hours", 5 * time.Hour, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			queue.Reset()

			sub := &Subscription{
				ID:                 uuid.New(),
				UserID:             uuid.New(),
				PlanID:             string(subscription.PlanPremium),
				Status:             subscription.StatusActive,
				CurrentPeriodStart: now,
				CurrentPeriodEnd:   now.Add(tc.expiresIn),
				CreatedAt:          now,
				UpdatedAt:          now,
			}

			err := repo.CreateSubscription(ctx, sub)
			require.NoError(t, err)

			config := DefaultNotificationConfig()
			err = service.processPreExpiryNotifications(ctx, config)
			require.NoError(t, err)

			if tc.shouldSend {
				assert.NotEmpty(t, queue.notifications, "Expected notification to be sent")
				if len(queue.notifications) > 0 {
					assert.Equal(t, sub.UserID, queue.notifications[0].UserID)
					assert.Equal(t, notification.NotificationTypeSubscription, queue.notifications[0].Type)
				}
			} else {
				assert.Empty(t, queue.notifications, "Expected no notification to be sent")
			}
		})
	}
}

func TestNotificationService_ProcessPostExpiryNotifications(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db, cleanup := setupNotificationTestDB(t)
	defer cleanup()

	logger := zaptest.NewLogger(t)
	repo := NewRepository(db)
	queue := &MockNotificationQueue{}
	service := NewNotificationService(db, repo, queue, logger)

	ctx := context.Background()

	// Create expired subscription
	now := time.Now()
	sub := &Subscription{
		ID:                 uuid.New(),
		UserID:             uuid.New(),
		PlanID:             string(subscription.PlanPremium),
		Status:             subscription.StatusExpired,
		CurrentPeriodStart: now.Add(-30 * 24 * time.Hour),
		CurrentPeriodEnd:   now.Add(-1 * 24 * time.Hour), // Expired 1 day ago
		CreatedAt:          now.Add(-30 * 24 * time.Hour),
		UpdatedAt:          now,
	}

	err := repo.CreateSubscription(ctx, sub)
	require.NoError(t, err)

	config := DefaultNotificationConfig()
	err = service.processPostExpiryNotifications(ctx, config)
	require.NoError(t, err)

	assert.NotEmpty(t, queue.notifications, "Expected post-expiry notification")
	if len(queue.notifications) > 0 {
		assert.Equal(t, sub.UserID, queue.notifications[0].UserID)
		assert.Contains(t, queue.notifications[0].Body, "expired")
	}
}

func TestNotificationService_PreventDuplicateNotifications(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db, cleanup := setupNotificationTestDB(t)
	defer cleanup()

	logger := zaptest.NewLogger(t)
	repo := NewRepository(db)
	queue := &MockNotificationQueue{}
	service := NewNotificationService(db, repo, queue, logger)

	ctx := context.Background()

	// Create subscription expiring in 7 days
	now := time.Now()
	sub := &Subscription{
		ID:                 uuid.New(),
		UserID:             uuid.New(),
		PlanID:             string(subscription.PlanPremium),
		Status:             subscription.StatusActive,
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   now.Add(7 * 24 * time.Hour),
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	err := repo.CreateSubscription(ctx, sub)
	require.NoError(t, err)

	config := DefaultNotificationConfig()

	// First run should send notification
	err = service.processPreExpiryNotifications(ctx, config)
	require.NoError(t, err)
	assert.Len(t, queue.notifications, 1, "Expected one notification")

	// Second run should not send duplicate
	queue.Reset()
	err = service.processPreExpiryNotifications(ctx, config)
	require.NoError(t, err)
	assert.Empty(t, queue.notifications, "Expected no duplicate notification")
}

func TestNotificationService_NotificationPriority(t *testing.T) {
	logger := zaptest.NewLogger(t)
	service := &NotificationService{logger: logger}

	tests := []struct {
		days     int
		expected notification.Priority
	}{
		{1, notification.PriorityHigh},
		{2, notification.PriorityNormal},
		{3, notification.PriorityNormal},
		{7, notification.PriorityLow},
		{14, notification.PriorityLow},
	}

	for _, tt := range tests {
		t.Run(string(tt.expected), func(t *testing.T) {
			priority := service.getNotificationPriority(tt.days)
			assert.Equal(t, tt.expected, priority)
		})
	}
}

func TestNotificationService_MessageGeneration(t *testing.T) {
	logger := zaptest.NewLogger(t)
	service := &NotificationService{logger: logger}

	sub := &Subscription{
		PlanID: string(subscription.PlanPremium),
	}

	// Test pre-expiry messages
	t.Run("pre_expiry_messages", func(t *testing.T) {
		msg1 := service.getPreExpiryMessage(sub, 1)
		assert.Contains(t, msg1, "tomorrow")
		assert.Contains(t, msg1, "Premium")

		msg3 := service.getPreExpiryMessage(sub, 3)
		assert.Contains(t, msg3, "3 days")

		msg7 := service.getPreExpiryMessage(sub, 7)
		assert.Contains(t, msg7, "7 days")
	})

	// Test post-expiry messages
	t.Run("post_expiry_messages", func(t *testing.T) {
		msg1 := service.getPostExpiryMessage(sub, 1)
		assert.Contains(t, msg1, "yesterday")
		assert.Contains(t, msg1, "Premium")

		msg3 := service.getPostExpiryMessage(sub, 3)
		assert.Contains(t, msg3, "3 days")

		msg7 := service.getPostExpiryMessage(sub, 7)
		assert.Contains(t, msg7, "week")
	})
}

func TestNotificationService_GetSubscriptionsExpiringInDays(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db, cleanup := setupNotificationTestDB(t)
	defer cleanup()

	logger := zaptest.NewLogger(t)
	repo := NewRepository(db)
	queue := &MockNotificationQueue{}
	service := NewNotificationService(db, repo, queue, logger)

	ctx := context.Background()
	now := time.Now()

	// Create subscriptions with different expiry dates
	subs := []*Subscription{
		{
			ID:                 uuid.New(),
			UserID:             uuid.New(),
			PlanID:             string(subscription.PlanPremium),
			Status:             subscription.StatusActive,
			CurrentPeriodStart: now,
			CurrentPeriodEnd:   now.Add(7 * 24 * time.Hour), // Expires in 7 days
			CreatedAt:          now,
			UpdatedAt:          now,
		},
		{
			ID:                 uuid.New(),
			UserID:             uuid.New(),
			PlanID:             string(subscription.PlanPremium),
			Status:             subscription.StatusActive,
			CurrentPeriodStart: now,
			CurrentPeriodEnd:   now.Add(3 * 24 * time.Hour), // Expires in 3 days
			CreatedAt:          now,
			UpdatedAt:          now,
		},
		{
			ID:                 uuid.New(),
			UserID:             uuid.New(),
			PlanID:             string(subscription.PlanPremium),
			Status:             subscription.StatusActive,
			CurrentPeriodStart: now,
			CurrentPeriodEnd:   now.Add(10 * 24 * time.Hour), // Expires in 10 days
			CreatedAt:          now,
			UpdatedAt:          now,
		},
	}

	for _, sub := range subs {
		err := repo.CreateSubscription(ctx, sub)
		require.NoError(t, err)
	}

	// Get subscriptions expiring in 7 days
	results, err := service.getSubscriptionsExpiringInDays(ctx, 7)
	require.NoError(t, err)
	assert.Len(t, results, 1, "Expected one subscription expiring in 7 days")
	assert.Equal(t, subs[0].ID, results[0].ID)

	// Get subscriptions expiring in 3 days
	results, err = service.getSubscriptionsExpiringInDays(ctx, 3)
	require.NoError(t, err)
	assert.Len(t, results, 1, "Expected one subscription expiring in 3 days")
	assert.Equal(t, subs[1].ID, results[0].ID)

	// Get subscriptions expiring in 1 day
	results, err = service.getSubscriptionsExpiringInDays(ctx, 1)
	require.NoError(t, err)
	assert.Empty(t, results, "Expected no subscriptions expiring in 1 day")
}
