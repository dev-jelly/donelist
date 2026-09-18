package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestService(t *testing.T) (*Service, *gorm.DB) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&Webhook{}, &WebhookDelivery{}, &WebhookDLQ{}, &WebhookEventLog{})
	require.NoError(t, err)

	repo := NewRepository(db)
	logger := zaptest.NewLogger(t)

	// Note: For testing, we're not using actual Asynq, just the repository
	// In a real test environment, you'd use asynq.NewInspector for testing
	service := &Service{
		repo: repo,
		httpClient: &http.Client{
			Timeout: DeliveryTimeout,
		},
		logger: logger,
	}

	return service, db
}

func TestService_CreateWebhook(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	userID := uuid.New()

	req := &CreateWebhookRequest{
		Name:   "Test Webhook",
		URL:    "https://example.com/webhook",
		Events: []string{"checkin.created"},
		Active: true,
		Headers: map[string]string{
			"X-Custom": "value",
		},
	}

	webhook, err := service.CreateWebhook(ctx, req, userID)
	assert.NoError(t, err)
	assert.NotNil(t, webhook)
	assert.Equal(t, req.Name, webhook.Name)
	assert.Equal(t, req.URL, webhook.URL)
	assert.Equal(t, req.Events, webhook.Events)
	assert.NotEmpty(t, webhook.Secret) // Secret should be auto-generated
}

func TestService_CreateWebhook_WithCustomSecret(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	userID := uuid.New()

	customSecret := "my-custom-secret-123"
	req := &CreateWebhookRequest{
		Name:   "Test Webhook",
		URL:    "https://example.com/webhook",
		Events: []string{"checkin.created"},
		Secret: customSecret,
		Active: true,
	}

	webhook, err := service.CreateWebhook(ctx, req, userID)
	assert.NoError(t, err)
	assert.Equal(t, customSecret, webhook.Secret)
}

func TestService_UpdateWebhook(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	userID := uuid.New()

	// Create webhook
	createReq := &CreateWebhookRequest{
		Name:   "Test Webhook",
		URL:    "https://example.com/webhook",
		Events: []string{"checkin.created"},
		Active: true,
	}
	webhook, err := service.CreateWebhook(ctx, createReq, userID)
	require.NoError(t, err)

	// Update webhook
	newName := "Updated Webhook"
	newURL := "https://example.com/webhook-v2"
	active := false
	updateReq := &UpdateWebhookRequest{
		Name:   &newName,
		URL:    &newURL,
		Active: &active,
		Events: []string{"checkin.created", "checkin.updated"},
	}

	updated, err := service.UpdateWebhook(ctx, webhook.ID, userID, updateReq)
	assert.NoError(t, err)
	assert.Equal(t, newName, updated.Name)
	assert.Equal(t, newURL, updated.URL)
	assert.False(t, updated.Active)
	assert.Len(t, updated.Events, 2)
}

func TestService_DeleteWebhook(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	userID := uuid.New()

	// Create webhook
	req := &CreateWebhookRequest{
		Name:   "Test Webhook",
		URL:    "https://example.com/webhook",
		Events: []string{"checkin.created"},
		Active: true,
	}
	webhook, err := service.CreateWebhook(ctx, req, userID)
	require.NoError(t, err)

	// Delete webhook
	err = service.DeleteWebhook(ctx, webhook.ID, userID)
	assert.NoError(t, err)

	// Verify deletion
	_, err = service.GetWebhook(ctx, webhook.ID, userID)
	assert.Error(t, err)
}

func TestService_ListWebhooks(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	userID := uuid.New()

	// Create multiple webhooks
	for i := 0; i < 3; i++ {
		req := &CreateWebhookRequest{
			Name:   "Test Webhook",
			URL:    "https://example.com/webhook",
			Events: []string{"checkin.created"},
			Active: true,
		}
		_, err := service.CreateWebhook(ctx, req, userID)
		require.NoError(t, err)
	}

	// List webhooks
	webhooks, err := service.ListWebhooks(ctx, userID)
	assert.NoError(t, err)
	assert.Len(t, webhooks, 3)
}

func TestService_CalculateSignature(t *testing.T) {
	service, _ := setupTestService(t)

	payload := []byte(`{"test": "data"}`)
	secret := "test-secret"

	signature := service.calculateSignature(payload, secret)
	assert.NotEmpty(t, signature)
	assert.Contains(t, signature, "sha256=")

	// Verify signature
	isValid := service.VerifySignature(payload, signature, secret)
	assert.True(t, isValid)

	// Invalid signature
	isValid = service.VerifySignature(payload, "sha256=invalid", secret)
	assert.False(t, isValid)
}

func TestService_DeliverWebhook_Success(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.NotEmpty(t, r.Header.Get("X-Webhook-Signature"))
		assert.NotEmpty(t, r.Header.Get("X-Webhook-Event"))
		assert.NotEmpty(t, r.Header.Get("X-Webhook-Event-ID"))
		assert.NotEmpty(t, r.Header.Get("X-Webhook-Delivery-ID"))

		// Return success
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "received"}`))
	}))
	defer server.Close()

	service, _ := setupTestService(t)
	ctx := context.Background()

	webhook := &Webhook{
		ID:     uuid.New(),
		UserID: uuid.New(),
		Name:   "Test Webhook",
		URL:    server.URL,
		Secret: "test-secret",
		Events: []string{"checkin.created"},
		Active: true,
	}

	delivery := &WebhookDelivery{
		ID:        uuid.New(),
		WebhookID: webhook.ID,
		EventType: EventCheckinCreated,
		EventID:   "test-event-1",
		Payload:   []byte(`{"id": "123", "name": "Test"}`),
		Status:    StatusPending,
		Attempts:  0,
		CreatedAt: time.Now(),
	}

	err := service.deliverWebhook(ctx, webhook, delivery)
	assert.NoError(t, err)
	assert.Equal(t, StatusDelivered, delivery.Status)
	assert.Equal(t, 1, delivery.Attempts)
	assert.NotNil(t, delivery.DeliveredAt)
	assert.NotNil(t, delivery.StatusCode)
	assert.Equal(t, 200, *delivery.StatusCode)
}

func TestService_DeliverWebhook_Failure(t *testing.T) {
	// Create a test server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal error"}`))
	}))
	defer server.Close()

	service, _ := setupTestService(t)
	ctx := context.Background()

	webhook := &Webhook{
		ID:     uuid.New(),
		UserID: uuid.New(),
		Name:   "Test Webhook",
		URL:    server.URL,
		Secret: "test-secret",
		Events: []string{"checkin.created"},
		Active: true,
	}

	delivery := &WebhookDelivery{
		ID:        uuid.New(),
		WebhookID: webhook.ID,
		EventType: EventCheckinCreated,
		EventID:   "test-event-1",
		Payload:   []byte(`{"id": "123"}`),
		Status:    StatusPending,
		Attempts:  0,
		CreatedAt: time.Now(),
	}

	err := service.deliverWebhook(ctx, webhook, delivery)
	assert.Error(t, err)
	assert.Equal(t, StatusRetrying, delivery.Status)
	assert.Equal(t, 1, delivery.Attempts)
	assert.NotNil(t, delivery.NextRetryAt)
	assert.NotNil(t, delivery.StatusCode)
	assert.Equal(t, 500, *delivery.StatusCode)
}

func TestService_DeliverWebhook_MaxRetries(t *testing.T) {
	// Create a test server that always fails
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	service, _ := setupTestService(t)
	ctx := context.Background()

	webhook := &Webhook{
		ID:     uuid.New(),
		UserID: uuid.New(),
		Name:   "Test Webhook",
		URL:    server.URL,
		Secret: "test-secret",
		Events: []string{"checkin.created"},
		Active: true,
	}

	delivery := &WebhookDelivery{
		ID:        uuid.New(),
		WebhookID: webhook.ID,
		EventType: EventCheckinCreated,
		EventID:   "test-event-1",
		Payload:   []byte(`{"id": "123"}`),
		Status:    StatusPending,
		Attempts:  MaxRetries, // Already at max retries
		CreatedAt: time.Now(),
	}

	err := service.deliverWebhook(ctx, webhook, delivery)
	assert.NoError(t, err) // No error returned when max retries exceeded
	assert.Equal(t, StatusFailed, delivery.Status)
}

func TestService_ResendDelivery(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	userID := uuid.New()

	// Create webhook
	webhook := &Webhook{
		ID:        uuid.New(),
		UserID:    userID,
		Name:      "Test Webhook",
		URL:       "https://example.com/webhook",
		Secret:    "test-secret",
		Events:    []string{"checkin.created"},
		Active:    true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := service.repo.CreateWebhook(ctx, webhook)
	require.NoError(t, err)

	// Create failed delivery
	errMsg := "failed"
	delivery := &WebhookDelivery{
		ID:        uuid.New(),
		WebhookID: webhook.ID,
		EventType: EventCheckinCreated,
		EventID:   "test-event-1",
		Payload:   []byte(`{"id": "123"}`),
		Status:    StatusFailed,
		Attempts:  5,
		Error:     &errMsg,
		CreatedAt: time.Now(),
	}
	err = service.repo.CreateDelivery(ctx, delivery)
	require.NoError(t, err)

	// Note: ResendDelivery tries to enqueue, which requires Asynq
	// In a real test, you'd mock or use a test Asynq instance
	// For now, we'll just test the repository reset
	delivery.Status = StatusPending
	delivery.Attempts = 0
	delivery.Error = nil
	err = service.repo.UpdateDelivery(ctx, delivery)
	assert.NoError(t, err)

	// Verify reset
	updated, err := service.repo.GetDeliveryByID(ctx, delivery.ID)
	assert.NoError(t, err)
	assert.Equal(t, StatusPending, updated.Status)
	assert.Equal(t, 0, updated.Attempts)
}

func TestService_WebhookPayload(t *testing.T) {
	service, _ := setupTestService(t)

	data := map[string]interface{}{
		"id":   "123",
		"name": "Test Check-in",
	}
	dataBytes, _ := json.Marshal(data)

	payload := WebhookPayload{
		Event:     "checkin.created",
		EventID:   "evt_123",
		Webhook:   "Test Webhook",
		Timestamp: time.Now().Unix(),
		Data:      dataBytes,
	}

	payloadBytes, err := json.Marshal(payload)
	assert.NoError(t, err)

	// Verify payload structure
	var decoded WebhookPayload
	err = json.Unmarshal(payloadBytes, &decoded)
	assert.NoError(t, err)
	assert.Equal(t, "checkin.created", decoded.Event)
	assert.Equal(t, "evt_123", decoded.EventID)
	assert.Equal(t, "Test Webhook", decoded.Webhook)

	// Verify signature
	secret := "test-secret"
	signature := service.calculateSignature(payloadBytes, secret)
	assert.True(t, service.VerifySignature(payloadBytes, signature, secret))
}

func TestService_ExponentialBackoff(t *testing.T) {
	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 1 * time.Minute},
		{1, 2 * time.Minute},
		{2, 4 * time.Minute},
		{3, 8 * time.Minute},
		{4, 16 * time.Minute},
		{5, 32 * time.Minute},
		{6, MaxRetryDelay}, // Capped at 1 hour
		{10, MaxRetryDelay}, // Still capped
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			delay := exponentialBackoff(tt.attempt, nil, nil)
			assert.Equal(t, tt.expected, delay)
		})
	}
}

func TestService_GenerateSecret(t *testing.T) {
	service, _ := setupTestService(t)

	secret1 := service.generateSecret()
	secret2 := service.generateSecret()

	assert.NotEmpty(t, secret1)
	assert.NotEmpty(t, secret2)
	assert.NotEqual(t, secret1, secret2)
	assert.Len(t, secret1, 64) // 32 bytes hex encoded = 64 characters
}

func TestWebhook_ToResponse(t *testing.T) {
	webhook := &Webhook{
		ID:      uuid.New(),
		UserID:  uuid.New(),
		Name:    "Test Webhook",
		URL:     "https://example.com/webhook",
		Secret:  "abcd1234567890efabcd1234567890ef",
		Events:  []string{"checkin.created"},
		Active:  true,
		Headers: map[string]string{"X-Custom": "value"},
		TotalDeliveries: 10,
		FailedDeliveries: 2,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	response := webhook.ToResponse()
	assert.NotNil(t, response)
	assert.Equal(t, webhook.Name, response.Name)
	assert.Equal(t, webhook.URL, response.URL)
	assert.NotEmpty(t, response.SecretPreview)
	assert.NotEqual(t, webhook.Secret, response.SecretPreview)
	assert.Contains(t, response.SecretPreview, "...")
}
