// +build integration

package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupIntegrationTest(t *testing.T) (*Service, *miniredis.Miniredis, *gorm.DB, func()) {
	// Setup in-memory Redis
	mr, err := miniredis.Run()
	require.NoError(t, err)

	// Setup in-memory database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&Webhook{}, &WebhookDelivery{}, &WebhookDLQ{}, &WebhookEventLog{})
	require.NoError(t, err)

	repo := NewRepository(db)
	logger := zaptest.NewLogger(t)

	service := NewService(repo, mr.Addr(), logger)

	cleanup := func() {
		service.Stop()
		mr.Close()
	}

	return service, mr, db, cleanup
}

func TestIntegration_WebhookDeliveryFlow(t *testing.T) {
	service, _, _, cleanup := setupIntegrationTest(t)
	defer cleanup()

	// Start service
	err := service.Start()
	require.NoError(t, err)

	// Create test HTTP server
	delivered := make(chan bool, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify webhook request
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.NotEmpty(t, r.Header.Get("X-Webhook-Signature"))
		assert.NotEmpty(t, r.Header.Get("X-Webhook-Event"))

		// Parse payload
		var payload WebhookPayload
		err := json.NewDecoder(r.Body).Decode(&payload)
		assert.NoError(t, err)
		assert.Equal(t, "checkin.created", payload.Event)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
		delivered <- true
	}))
	defer server.Close()

	ctx := context.Background()
	userID := uuid.New()

	// Create webhook
	req := &CreateWebhookRequest{
		Name:   "Test Webhook",
		URL:    server.URL,
		Events: []string{"checkin.created"},
		Active: true,
	}

	webhook, err := service.CreateWebhook(ctx, req, userID)
	require.NoError(t, err)

	// Trigger event
	eventData := map[string]interface{}{
		"id":        "123",
		"name":      "Test Check-in",
		"completed": true,
	}

	err = service.TriggerEvent(ctx, EventCheckinCreated, userID, eventData)
	require.NoError(t, err)

	// Wait for delivery
	select {
	case <-delivered:
		// Success!
	case <-time.After(10 * time.Second):
		t.Fatal("Webhook was not delivered within timeout")
	}

	// Verify delivery record
	time.Sleep(500 * time.Millisecond) // Give time for async update
	deliveries, err := service.GetDeliveries(ctx, webhook.ID, userID, 10)
	require.NoError(t, err)
	assert.Len(t, deliveries, 1)
	assert.Equal(t, StatusDelivered, deliveries[0].Status)
}

func TestIntegration_WebhookRetry(t *testing.T) {
	service, _, _, cleanup := setupIntegrationTest(t)
	defer cleanup()

	err := service.Start()
	require.NoError(t, err)

	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount < 2 {
			// Fail first attempt
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		// Succeed on second attempt
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx := context.Background()
	userID := uuid.New()

	req := &CreateWebhookRequest{
		Name:   "Retry Webhook",
		URL:    server.URL,
		Events: []string{"checkin.created"},
		Active: true,
	}

	webhook, err := service.CreateWebhook(ctx, req, userID)
	require.NoError(t, err)

	err = service.TriggerEvent(ctx, EventCheckinCreated, userID, map[string]string{"test": "data"})
	require.NoError(t, err)

	// Wait for retries
	time.Sleep(5 * time.Second)

	// Verify multiple attempts were made
	assert.GreaterOrEqual(t, attemptCount, 2, "Should have retried at least once")
}

func TestIntegration_WebhookDLQ(t *testing.T) {
	service, _, _, cleanup := setupIntegrationTest(t)
	defer cleanup()

	err := service.Start()
	require.NoError(t, err)

	// Create server that always fails
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	ctx := context.Background()
	userID := uuid.New()

	req := &CreateWebhookRequest{
		Name:   "Failing Webhook",
		URL:    server.URL,
		Events: []string{"checkin.created"},
		Active: true,
	}

	webhook, err := service.CreateWebhook(ctx, req, userID)
	require.NoError(t, err)

	err = service.TriggerEvent(ctx, EventCheckinCreated, userID, map[string]string{"test": "data"})
	require.NoError(t, err)

	// Wait for all retries to be exhausted
	time.Sleep(time.Duration(MaxRetries+1) * InitialRetryDelay * 2)

	// Verify DLQ entry
	dlqEntries, err := service.GetDLQEntries(ctx, webhook.ID, userID, 10)
	require.NoError(t, err)
	assert.NotEmpty(t, dlqEntries, "Should have entry in DLQ after max retries")
}

func TestIntegration_MultipleWebhooks(t *testing.T) {
	service, _, _, cleanup := setupIntegrationTest(t)
	defer cleanup()

	err := service.Start()
	require.NoError(t, err)

	delivered := make(map[string]bool)
	deliveredChan := make(chan string, 3)

	// Create multiple servers
	servers := make([]*httptest.Server, 3)
	for i := 0; i < 3; i++ {
		name := "webhook" + string(rune('A'+i))
		servers[i] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			deliveredChan <- r.URL.Path
			w.WriteHeader(http.StatusOK)
		}))
		defer servers[i].Close()
	}

	ctx := context.Background()
	userID := uuid.New()

	// Create multiple webhooks
	for i, server := range servers {
		req := &CreateWebhookRequest{
			Name:   "Webhook " + string(rune('A'+i)),
			URL:    server.URL,
			Events: []string{"checkin.created"},
			Active: true,
		}
		_, err := service.CreateWebhook(ctx, req, userID)
		require.NoError(t, err)
	}

	// Trigger single event
	err = service.TriggerEvent(ctx, EventCheckinCreated, userID, map[string]string{"test": "data"})
	require.NoError(t, err)

	// Wait for all deliveries
	timeout := time.After(10 * time.Second)
	deliveryCount := 0
	for deliveryCount < 3 {
		select {
		case path := <-deliveredChan:
			delivered[path] = true
			deliveryCount++
		case <-timeout:
			t.Fatal("Not all webhooks were delivered within timeout")
		}
	}

	assert.Len(t, delivered, 3, "All three webhooks should be delivered")
}

func TestIntegration_InactiveWebhook(t *testing.T) {
	service, _, _, cleanup := setupIntegrationTest(t)
	defer cleanup()

	err := service.Start()
	require.NoError(t, err)

	deliveryCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deliveryCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx := context.Background()
	userID := uuid.New()

	// Create inactive webhook
	req := &CreateWebhookRequest{
		Name:   "Inactive Webhook",
		URL:    server.URL,
		Events: []string{"checkin.created"},
		Active: false, // Inactive
	}

	_, err = service.CreateWebhook(ctx, req, userID)
	require.NoError(t, err)

	// Trigger event
	err = service.TriggerEvent(ctx, EventCheckinCreated, userID, map[string]string{"test": "data"})
	require.NoError(t, err)

	// Wait a bit
	time.Sleep(2 * time.Second)

	// Verify webhook was not called
	assert.False(t, deliveryCalled, "Inactive webhook should not be delivered")
}

func TestIntegration_SignatureVerification(t *testing.T) {
	service, _, _, cleanup := setupIntegrationTest(t)
	defer cleanup()

	err := service.Start()
	require.NoError(t, err)

	secret := "my-webhook-secret"
	var receivedSignature string
	var receivedPayload []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSignature = r.Header.Get("X-Webhook-Signature")

		// Read body
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		receivedPayload = buf[:n]

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx := context.Background()
	userID := uuid.New()

	req := &CreateWebhookRequest{
		Name:   "Signature Test",
		URL:    server.URL,
		Events: []string{"checkin.created"},
		Secret: secret,
		Active: true,
	}

	_, err = service.CreateWebhook(ctx, req, userID)
	require.NoError(t, err)

	err = service.TriggerEvent(ctx, EventCheckinCreated, userID, map[string]string{"test": "data"})
	require.NoError(t, err)

	// Wait for delivery
	time.Sleep(2 * time.Second)

	// Verify signature
	assert.NotEmpty(t, receivedSignature)
	assert.NotEmpty(t, receivedPayload)

	isValid := service.VerifySignature(receivedPayload, receivedSignature, secret)
	assert.True(t, isValid, "Signature should be valid")
}

func TestIntegration_CustomHeaders(t *testing.T) {
	service, _, _, cleanup := setupIntegrationTest(t)
	defer cleanup()

	err := service.Start()
	require.NoError(t, err)

	var receivedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx := context.Background()
	userID := uuid.New()

	customHeaders := map[string]string{
		"X-Custom-Header": "custom-value",
		"Authorization":   "Bearer token123",
	}

	req := &CreateWebhookRequest{
		Name:    "Custom Headers Test",
		URL:     server.URL,
		Events:  []string{"checkin.created"},
		Headers: customHeaders,
		Active:  true,
	}

	_, err = service.CreateWebhook(ctx, req, userID)
	require.NoError(t, err)

	err = service.TriggerEvent(ctx, EventCheckinCreated, userID, map[string]string{"test": "data"})
	require.NoError(t, err)

	// Wait for delivery
	time.Sleep(2 * time.Second)

	// Verify custom headers
	assert.Equal(t, "custom-value", receivedHeaders.Get("X-Custom-Header"))
	assert.Equal(t, "Bearer token123", receivedHeaders.Get("Authorization"))
}

func TestIntegration_WebhookStats(t *testing.T) {
	service, _, _, cleanup := setupIntegrationTest(t)
	defer cleanup()

	err := service.Start()
	require.NoError(t, err)

	successCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		successCount++
		if successCount <= 2 {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	ctx := context.Background()
	userID := uuid.New()

	req := &CreateWebhookRequest{
		Name:   "Stats Test",
		URL:    server.URL,
		Events: []string{"checkin.created"},
		Active: true,
	}

	webhook, err := service.CreateWebhook(ctx, req, userID)
	require.NoError(t, err)

	// Trigger multiple events
	for i := 0; i < 3; i++ {
		err = service.TriggerEvent(ctx, EventCheckinCreated, userID, map[string]string{"test": "data"})
		require.NoError(t, err)
		time.Sleep(500 * time.Millisecond)
	}

	// Wait for processing
	time.Sleep(3 * time.Second)

	// Get stats
	stats, err := service.GetWebhookStats(ctx, webhook.ID, userID, time.Now().Add(-1*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(3), stats.TotalDeliveries)
}

func TestIntegration_AsynqIntegration(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	// Create Asynq client
	client := asynq.NewClient(asynq.RedisClientOpt{Addr: mr.Addr()})
	defer client.Close()

	// Enqueue a task
	payload, _ := json.Marshal(map[string]string{"delivery_id": uuid.New().String()})
	task := asynq.NewTask(TypeWebhookDelivery, payload)

	info, err := client.Enqueue(task)
	require.NoError(t, err)
	assert.NotNil(t, info)
	assert.Equal(t, TypeWebhookDelivery, info.Type)
}
