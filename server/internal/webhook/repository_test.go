package webhook

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Auto migrate tables
	err = db.AutoMigrate(&Webhook{}, &WebhookDelivery{}, &WebhookDLQ{}, &WebhookEventLog{})
	require.NoError(t, err)

	return db
}

func TestRepository_CreateWebhook(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	webhook := &Webhook{
		ID:      uuid.New(),
		UserID:  uuid.New(),
		Name:    "Test Webhook",
		URL:     "https://example.com/webhook",
		Secret:  "test-secret",
		Events:  []string{"checkin.created"},
		Active:  true,
		Headers: map[string]string{"X-Custom": "value"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := repo.CreateWebhook(ctx, webhook)
	assert.NoError(t, err)

	// Verify creation
	retrieved, err := repo.GetWebhookByID(ctx, webhook.ID)
	assert.NoError(t, err)
	assert.Equal(t, webhook.Name, retrieved.Name)
	assert.Equal(t, webhook.URL, retrieved.URL)
	assert.Equal(t, webhook.Events, retrieved.Events)
}

func TestRepository_UpdateWebhook(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	webhook := &Webhook{
		ID:      uuid.New(),
		UserID:  uuid.New(),
		Name:    "Test Webhook",
		URL:     "https://example.com/webhook",
		Secret:  "test-secret",
		Events:  []string{"checkin.created"},
		Active:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := repo.CreateWebhook(ctx, webhook)
	require.NoError(t, err)

	// Update webhook
	webhook.Name = "Updated Webhook"
	webhook.Active = false
	err = repo.UpdateWebhook(ctx, webhook)
	assert.NoError(t, err)

	// Verify update
	retrieved, err := repo.GetWebhookByID(ctx, webhook.ID)
	assert.NoError(t, err)
	assert.Equal(t, "Updated Webhook", retrieved.Name)
	assert.False(t, retrieved.Active)
}

func TestRepository_DeleteWebhook(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	webhook := &Webhook{
		ID:      uuid.New(),
		UserID:  uuid.New(),
		Name:    "Test Webhook",
		URL:     "https://example.com/webhook",
		Secret:  "test-secret",
		Events:  []string{"checkin.created"},
		Active:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := repo.CreateWebhook(ctx, webhook)
	require.NoError(t, err)

	// Delete webhook
	err = repo.DeleteWebhook(ctx, webhook.ID)
	assert.NoError(t, err)

	// Verify deletion (soft delete)
	_, err = repo.GetWebhookByID(ctx, webhook.ID)
	assert.Error(t, err)
}

func TestRepository_ListWebhooksByUser(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	userID := uuid.New()

	// Create multiple webhooks
	for i := 0; i < 3; i++ {
		webhook := &Webhook{
			ID:      uuid.New(),
			UserID:  userID,
			Name:    "Test Webhook",
			URL:     "https://example.com/webhook",
			Secret:  "test-secret",
			Events:  []string{"checkin.created"},
			Active:  true,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		err := repo.CreateWebhook(ctx, webhook)
		require.NoError(t, err)
	}

	// List webhooks
	webhooks, err := repo.ListWebhooksByUser(ctx, userID)
	assert.NoError(t, err)
	assert.Len(t, webhooks, 3)
}

func TestRepository_GetActiveWebhooksForEvent(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	userID := uuid.New()

	// Create webhook with specific event
	webhook1 := &Webhook{
		ID:      uuid.New(),
		UserID:  userID,
		Name:    "Checkin Webhook",
		URL:     "https://example.com/webhook",
		Secret:  "test-secret",
		Events:  []string{"checkin.created"},
		Active:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := repo.CreateWebhook(ctx, webhook1)
	require.NoError(t, err)

	// Create webhook with all events
	webhook2 := &Webhook{
		ID:      uuid.New(),
		UserID:  userID,
		Name:    "All Events Webhook",
		URL:     "https://example.com/webhook2",
		Secret:  "test-secret",
		Events:  []string{"*"},
		Active:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = repo.CreateWebhook(ctx, webhook2)
	require.NoError(t, err)

	// Create inactive webhook
	webhook3 := &Webhook{
		ID:      uuid.New(),
		UserID:  userID,
		Name:    "Inactive Webhook",
		URL:     "https://example.com/webhook3",
		Secret:  "test-secret",
		Events:  []string{"checkin.created"},
		Active:  false,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = repo.CreateWebhook(ctx, webhook3)
	require.NoError(t, err)

	// Get active webhooks for checkin.created
	webhooks, err := repo.GetActiveWebhooksForEvent(ctx, userID, EventCheckinCreated)
	assert.NoError(t, err)
	assert.Len(t, webhooks, 2) // webhook1 and webhook2, not webhook3
}

func TestRepository_CreateDelivery(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	webhookID := uuid.New()
	delivery := &WebhookDelivery{
		ID:        uuid.New(),
		WebhookID: webhookID,
		EventType: EventCheckinCreated,
		EventID:   "test-event-1",
		Payload:   []byte(`{"test": "data"}`),
		Status:    StatusPending,
		Attempts:  0,
		CreatedAt: time.Now(),
	}

	err := repo.CreateDelivery(ctx, delivery)
	assert.NoError(t, err)

	// Verify creation
	retrieved, err := repo.GetDeliveryByID(ctx, delivery.ID)
	assert.NoError(t, err)
	assert.Equal(t, delivery.EventType, retrieved.EventType)
	assert.Equal(t, delivery.Status, retrieved.Status)
}

func TestRepository_UpdateDelivery(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	delivery := &WebhookDelivery{
		ID:        uuid.New(),
		WebhookID: uuid.New(),
		EventType: EventCheckinCreated,
		EventID:   "test-event-1",
		Payload:   []byte(`{"test": "data"}`),
		Status:    StatusPending,
		Attempts:  0,
		CreatedAt: time.Now(),
	}

	err := repo.CreateDelivery(ctx, delivery)
	require.NoError(t, err)

	// Update delivery
	delivery.Status = StatusDelivered
	delivery.Attempts = 1
	now := time.Now()
	delivery.DeliveredAt = &now

	err = repo.UpdateDelivery(ctx, delivery)
	assert.NoError(t, err)

	// Verify update
	retrieved, err := repo.GetDeliveryByID(ctx, delivery.ID)
	assert.NoError(t, err)
	assert.Equal(t, StatusDelivered, retrieved.Status)
	assert.Equal(t, 1, retrieved.Attempts)
	assert.NotNil(t, retrieved.DeliveredAt)
}

func TestRepository_GetPendingRetries(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	webhookID := uuid.New()

	// Create delivery that needs retry
	pastTime := time.Now().Add(-1 * time.Minute)
	delivery1 := &WebhookDelivery{
		ID:          uuid.New(),
		WebhookID:   webhookID,
		EventType:   EventCheckinCreated,
		EventID:     "test-event-1",
		Payload:     []byte(`{"test": "data"}`),
		Status:      StatusRetrying,
		Attempts:    1,
		NextRetryAt: &pastTime,
		CreatedAt:   time.Now(),
	}
	err := repo.CreateDelivery(ctx, delivery1)
	require.NoError(t, err)

	// Create delivery that doesn't need retry yet
	futureTime := time.Now().Add(1 * time.Hour)
	delivery2 := &WebhookDelivery{
		ID:          uuid.New(),
		WebhookID:   webhookID,
		EventType:   EventCheckinCreated,
		EventID:     "test-event-2",
		Payload:     []byte(`{"test": "data"}`),
		Status:      StatusRetrying,
		Attempts:    1,
		NextRetryAt: &futureTime,
		CreatedAt:   time.Now(),
	}
	err = repo.CreateDelivery(ctx, delivery2)
	require.NoError(t, err)

	// Get pending retries
	retries, err := repo.GetPendingRetries(ctx, 10)
	assert.NoError(t, err)
	assert.Len(t, retries, 1)
	assert.Equal(t, delivery1.ID, retries[0].ID)
}

func TestRepository_MoveToDLQ(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	webhook := &Webhook{
		ID:      uuid.New(),
		UserID:  uuid.New(),
		Name:    "Test Webhook",
		URL:     "https://example.com/webhook",
		Secret:  "test-secret",
		Events:  []string{"checkin.created"},
		Active:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := repo.CreateWebhook(ctx, webhook)
	require.NoError(t, err)

	errMsg := "max retries exceeded"
	statusCode := 500
	delivery := &WebhookDelivery{
		ID:         uuid.New(),
		WebhookID:  webhook.ID,
		EventType:  EventCheckinCreated,
		EventID:    "test-event-1",
		Payload:    []byte(`{"test": "data"}`),
		Status:     StatusFailed,
		Attempts:   5,
		StatusCode: &statusCode,
		Error:      &errMsg,
		CreatedAt:  time.Now(),
	}
	err = repo.CreateDelivery(ctx, delivery)
	require.NoError(t, err)

	// Move to DLQ
	err = repo.MoveToDLQ(ctx, delivery, webhook)
	assert.NoError(t, err)

	// Verify DLQ entry
	entries, err := repo.GetDLQEntries(ctx, webhook.ID, 10)
	assert.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, delivery.ID, entries[0].OriginalDeliveryID)
	assert.Equal(t, 5, entries[0].TotalAttempts)
}

func TestRepository_UpdateWebhookStats(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	webhook := &Webhook{
		ID:      uuid.New(),
		UserID:  uuid.New(),
		Name:    "Test Webhook",
		URL:     "https://example.com/webhook",
		Secret:  "test-secret",
		Events:  []string{"checkin.created"},
		Active:  true,
		TotalDeliveries: 0,
		FailedDeliveries: 0,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := repo.CreateWebhook(ctx, webhook)
	require.NoError(t, err)

	// Update stats for success
	err = repo.UpdateWebhookStats(ctx, webhook.ID, true)
	assert.NoError(t, err)

	// Verify stats
	retrieved, err := repo.GetWebhookByID(ctx, webhook.ID)
	assert.NoError(t, err)
	assert.Equal(t, 1, retrieved.TotalDeliveries)
	assert.Equal(t, 0, retrieved.FailedDeliveries)

	// Update stats for failure
	err = repo.UpdateWebhookStats(ctx, webhook.ID, false)
	assert.NoError(t, err)

	// Verify stats again
	retrieved, err = repo.GetWebhookByID(ctx, webhook.ID)
	assert.NoError(t, err)
	assert.Equal(t, 2, retrieved.TotalDeliveries)
	assert.Equal(t, 1, retrieved.FailedDeliveries)
}

func TestRepository_GetWebhookStats(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	webhookID := uuid.New()

	// Create successful delivery
	delivery1 := &WebhookDelivery{
		ID:        uuid.New(),
		WebhookID: webhookID,
		EventType: EventCheckinCreated,
		EventID:   "test-event-1",
		Payload:   []byte(`{"test": "data"}`),
		Status:    StatusDelivered,
		Attempts:  1,
		CreatedAt: time.Now(),
	}
	err := repo.CreateDelivery(ctx, delivery1)
	require.NoError(t, err)

	// Create failed delivery
	delivery2 := &WebhookDelivery{
		ID:        uuid.New(),
		WebhookID: webhookID,
		EventType: EventCheckinCreated,
		EventID:   "test-event-2",
		Payload:   []byte(`{"test": "data"}`),
		Status:    StatusFailed,
		Attempts:  5,
		CreatedAt: time.Now(),
	}
	err = repo.CreateDelivery(ctx, delivery2)
	require.NoError(t, err)

	// Get stats
	since := time.Now().Add(-1 * time.Hour)
	stats, err := repo.GetWebhookStats(ctx, webhookID, since)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), stats.TotalDeliveries)
	assert.Equal(t, int64(1), stats.SuccessfulDeliveries)
	assert.Equal(t, int64(1), stats.FailedDeliveries)
}

func TestRepository_CleanupOldDeliveries(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	webhookID := uuid.New()

	// Create old delivery
	oldTime := time.Now().AddDate(0, 0, -60)
	delivery1 := &WebhookDelivery{
		ID:        uuid.New(),
		WebhookID: webhookID,
		EventType: EventCheckinCreated,
		EventID:   "test-event-1",
		Payload:   []byte(`{"test": "data"}`),
		Status:    StatusDelivered,
		Attempts:  1,
		CreatedAt: oldTime,
	}
	err := repo.CreateDelivery(ctx, delivery1)
	require.NoError(t, err)

	// Create recent delivery
	delivery2 := &WebhookDelivery{
		ID:        uuid.New(),
		WebhookID: webhookID,
		EventType: EventCheckinCreated,
		EventID:   "test-event-2",
		Payload:   []byte(`{"test": "data"}`),
		Status:    StatusDelivered,
		Attempts:  1,
		CreatedAt: time.Now(),
	}
	err = repo.CreateDelivery(ctx, delivery2)
	require.NoError(t, err)

	// Cleanup old deliveries
	cutoff := time.Now().AddDate(0, 0, -30)
	deleted, err := repo.CleanupOldDeliveries(ctx, cutoff)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	// Verify only recent delivery remains
	deliveries, err := repo.ListDeliveriesByWebhook(ctx, webhookID, 10)
	assert.NoError(t, err)
	assert.Len(t, deliveries, 1)
	assert.Equal(t, delivery2.ID, deliveries[0].ID)
}
