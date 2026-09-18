package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/dev-jelly/donelist/internal/notification"
	"go.uber.org/zap"
)

func main() {
	// Create a logger
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	fmt.Println("=== Push Notification Provider Demo ===\n")

	// Create a mock provider for testing
	mockConfig := &notification.MockProviderConfig{
		AlwaysSucceed:   true,
		SimulateDelay:   100 * time.Millisecond,
		RateLimitAfter:  5,
		SimulateRateLimit: true,
	}

	mockProvider := notification.NewMockProvider(mockConfig)

	// Create provider manager
	config := notification.DefaultPushProviderConfig()
	config.MaxRetries = 3
	config.RetryBackoff = time.Second

	manager := notification.NewProviderManager(config, logger)

	// Register the mock provider
	err := manager.RegisterProvider(mockProvider)
	if err != nil {
		log.Fatal("Failed to register provider:", err)
	}

	fmt.Println("Provider registered successfully")
	fmt.Println("Provider Type:", mockProvider.GetProviderType())
	fmt.Println("Is Available:", mockProvider.IsAvailable(context.Background()))
	fmt.Println()

	// Test 1: Send a simple notification
	fmt.Println("Test 1: Sending a simple notification")
	notification1 := &notification.PushNotification{
		DeviceToken:  "test_device_token_123",
		Title:        "Hello World",
		Body:         "This is a test notification",
		HighPriority: true,
		Data: map[string]string{
			"action": "open_app",
			"screen": "home",
		},
	}

	ctx := context.Background()
	result, err := manager.Send(ctx, notification1, notification.PushProviderMock)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Success: %v, Message ID: %s\n", result.Success, result.MessageID)
	}
	fmt.Println()

	// Test 2: Batch send
	fmt.Println("Test 2: Sending batch notifications")
	notifications := []*notification.PushNotification{
		{
			DeviceToken: "device_1",
			Title:       "Batch Notification 1",
			Body:        "First notification in batch",
		},
		{
			DeviceToken: "device_2",
			Title:       "Batch Notification 2",
			Body:        "Second notification in batch",
		},
		{
			DeviceToken: "device_3",
			Title:       "Batch Notification 3",
			Body:        "Third notification in batch",
		},
	}

	results, err := manager.SendBatch(ctx, notifications, notification.PushProviderMock)
	if err != nil {
		fmt.Printf("Batch error: %v\n", err)
	}

	for i, result := range results {
		fmt.Printf("  Notification %d: Success=%v, MessageID=%s\n",
			i+1, result.Success, result.MessageID)
	}
	fmt.Println()

	// Test 3: Test rate limiting
	fmt.Println("Test 3: Testing rate limiting (sending 7 notifications)")
	for i := 1; i <= 7; i++ {
		notif := &notification.PushNotification{
			DeviceToken: fmt.Sprintf("device_%d", i),
			Title:       fmt.Sprintf("Notification %d", i),
			Body:        "Testing rate limiting",
		}

		result, err := mockProvider.Send(ctx, notif)
		if err != nil {
			fmt.Printf("  Notification %d: Failed - %v\n", i, err)
		} else {
			fmt.Printf("  Notification %d: Success=%v\n", i, result.Success)
		}
	}
	fmt.Println()

	// Test 4: Invalid token handling
	fmt.Println("Test 4: Testing invalid token handling")
	mockProvider.Clear()
	mockConfig2 := &notification.MockProviderConfig{
		InvalidTokenPattern: "invalid_",
		ExpiredTokenPattern: "expired_",
	}
	mockProvider2 := notification.NewMockProvider(mockConfig2)

	invalidNotif := &notification.PushNotification{
		DeviceToken: "invalid_token_123",
		Title:       "Test",
		Body:        "This should fail",
	}

	result, err = mockProvider2.Send(ctx, invalidNotif)
	fmt.Printf("  Invalid token result: Success=%v, ShouldRemove=%v, Error=%v\n",
		result.Success, result.ShouldRemoveToken, err)

	expiredNotif := &notification.PushNotification{
		DeviceToken: "expired_token_456",
		Title:       "Test",
		Body:        "This should also fail",
	}

	result, err = mockProvider2.Send(ctx, expiredNotif)
	fmt.Printf("  Expired token result: Success=%v, ShouldRemove=%v, Error=%v\n",
		result.Success, result.ShouldRemoveToken, err)
	fmt.Println()

	// Test 5: Get metrics
	fmt.Println("Test 5: Provider Metrics")
	metrics := manager.GetMetrics()
	fmt.Printf("  Total routed to mock: %v\n", metrics["total_routed_mock"])
	fmt.Printf("  Total failed: %v\n", metrics["total_failed"])
	fmt.Printf("  Total retries: %v\n", metrics["total_retries"])

	mockMetrics := mockProvider.GetMetrics()
	fmt.Printf("  Mock provider sent: %v\n", mockMetrics["total_sent"])
	fmt.Printf("  Mock provider failed: %v\n", mockMetrics["total_failed"])
	fmt.Printf("  Mock provider call count: %v\n", mockMetrics["total_calls"])
	fmt.Println()

	// Test 6: Validation
	fmt.Println("Test 6: Notification Validation")

	// Valid notification
	validNotif := &notification.PushNotification{
		DeviceToken: "valid_token",
		Title:       "Valid Title",
		Body:        "Valid Body",
		TTL:         3600,
	}
	err = notification.ValidatePushNotification(validNotif)
	fmt.Printf("  Valid notification: %v\n", err == nil)

	// Missing token
	missingTokenNotif := &notification.PushNotification{
		Title: "Title",
		Body:  "Body",
	}
	err = notification.ValidatePushNotification(missingTokenNotif)
	fmt.Printf("  Missing token: Error=%v\n", err != nil)

	// Payload too large
	largeNotif := &notification.PushNotification{
		DeviceToken: "token",
		Title:       string(make([]byte, 2000)),
		Body:        string(make([]byte, 3000)),
	}
	err = notification.ValidatePushNotification(largeNotif)
	fmt.Printf("  Large payload: IsPayloadTooLarge=%v\n", err == notification.ErrPayloadTooLarge)

	fmt.Println("\n=== Demo Complete ===")
}