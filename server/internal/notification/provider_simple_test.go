package notification

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderInterface(t *testing.T) {
	t.Run("MockProviderImplementsInterface", func(t *testing.T) {
		// Ensure MockProvider implements the interface
		var _ PushNotificationProvider = (*MockProvider)(nil)

		provider := NewMockProvider(&MockProviderConfig{
			AlwaysSucceed: true,
		})

		assert.Equal(t, PushProviderMock, provider.GetProviderType())
		assert.True(t, provider.IsAvailable(context.Background()))
	})

	t.Run("BasicMockProviderSend", func(t *testing.T) {
		provider := NewMockProvider(&MockProviderConfig{
			AlwaysSucceed: true,
		})

		notification := &PushNotification{
			DeviceToken: "test_token_123",
			Title:       "Test Title",
			Body:        "Test Body",
			Data:        map[string]string{"key": "value"},
		}

		result, err := provider.Send(context.Background(), notification)
		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.NotEmpty(t, result.MessageID)
		assert.False(t, result.ShouldRemoveToken)

		// Check that notification was recorded
		sent := provider.GetSentNotifications()
		assert.Len(t, sent, 1)
		assert.Equal(t, "Test Title", sent[0].Title)
	})

	t.Run("TokenValidation", func(t *testing.T) {
		provider := NewMockProvider(&MockProviderConfig{
			InvalidTokenPattern: "invalid_",
		})

		// Test invalid token
		err := provider.ValidateToken("invalid_token")
		assert.Error(t, err)
		assert.Equal(t, ErrInvalidToken, err)

		// Test valid token
		err = provider.ValidateToken("valid_token")
		assert.NoError(t, err)
	})

	t.Run("SimulatedFailure", func(t *testing.T) {
		provider := NewMockProvider(&MockProviderConfig{
			AlwaysFail: true,
		})

		notification := &PushNotification{
			DeviceToken: "test_token",
			Title:       "Test",
			Body:        "Body",
		}

		result, err := provider.Send(context.Background(), notification)
		require.Error(t, err)
		assert.False(t, result.Success)
		assert.True(t, result.Retryable)
	})

	t.Run("RateLimiting", func(t *testing.T) {
		provider := NewMockProvider(&MockProviderConfig{
			SimulateRateLimit: true,
			RateLimitAfter:    2,
		})

		notification := &PushNotification{
			DeviceToken: "test_token",
			Title:       "Test",
			Body:        "Body",
		}

		// First two should succeed
		for i := 0; i < 2; i++ {
			result, _ := provider.Send(context.Background(), notification)
			if result.Error == nil {
				assert.True(t, result.Success, "Request %d should succeed", i+1)
			}
		}

		// Third should hit rate limit
		result, err := provider.Send(context.Background(), notification)
		require.Error(t, err)
		assert.False(t, result.Success)
		assert.Equal(t, ErrRateLimitExceeded, result.Error)
		assert.True(t, result.Retryable)
	})

	t.Run("BatchSend", func(t *testing.T) {
		provider := NewMockProvider(&MockProviderConfig{
			AlwaysSucceed: true,
		})

		notifications := []*PushNotification{
			{DeviceToken: "token1", Title: "Title1", Body: "Body1"},
			{DeviceToken: "token2", Title: "Title2", Body: "Body2"},
			{DeviceToken: "token3", Title: "Title3", Body: "Body3"},
		}

		results, err := provider.SendBatch(context.Background(), notifications)
		require.NoError(t, err)
		assert.Len(t, results, 3)

		successCount := 0
		for _, result := range results {
			if result.Success {
				successCount++
			}
		}
		assert.Greater(t, successCount, 0, "At least some notifications should succeed")
	})

	t.Run("Metrics", func(t *testing.T) {
		provider := NewMockProvider(&MockProviderConfig{
			AlwaysSucceed: true,
		})

		// Send a few notifications
		for i := 0; i < 5; i++ {
			notification := &PushNotification{
				DeviceToken: "test_token",
				Title:       "Test",
				Body:        "Body",
			}
			provider.Send(context.Background(), notification)
		}

		metrics := provider.GetMetrics()
		assert.Equal(t, "mock", metrics["provider"])
		assert.Equal(t, int64(5), metrics["total_calls"])
		assert.Greater(t, metrics["total_sent"].(int64), int64(0))
	})
}

func TestNotificationValidation(t *testing.T) {
	t.Run("ValidNotification", func(t *testing.T) {
		notification := &PushNotification{
			DeviceToken: "valid_token",
			Title:       "Test Title",
			Body:        "Test Body",
			TTL:         3600,
		}

		err := ValidatePushNotification(notification)
		assert.NoError(t, err)
	})

	t.Run("EmptyToken", func(t *testing.T) {
		notification := &PushNotification{
			Title: "Test Title",
			Body:  "Test Body",
		}

		err := ValidatePushNotification(notification)
		assert.Error(t, err)
	})

	t.Run("PayloadTooLarge", func(t *testing.T) {
		largeBody := make([]byte, 5000)
		for i := range largeBody {
			largeBody[i] = 'a'
		}

		notification := &PushNotification{
			DeviceToken: "valid_token",
			Title:       "Test",
			Body:        string(largeBody),
		}

		err := ValidatePushNotification(notification)
		assert.Error(t, err)
		assert.Equal(t, ErrPayloadTooLarge, err)
	})

	t.Run("InvalidTTL", func(t *testing.T) {
		notification := &PushNotification{
			DeviceToken: "valid_token",
			Title:       "Test",
			Body:        "Body",
			TTL:         -1,
		}

		err := ValidatePushNotification(notification)
		assert.Error(t, err)
	})
}

func TestRetryLogic(t *testing.T) {
	t.Run("RetryableErrors", func(t *testing.T) {
		assert.True(t, IsRetryableError(ErrProviderUnavailable))
		assert.True(t, IsRetryableError(ErrRateLimitExceeded))
		assert.True(t, IsRetryableError(context.DeadlineExceeded))
		assert.False(t, IsRetryableError(ErrInvalidToken))
		assert.False(t, IsRetryableError(ErrTokenExpired))
	})

	t.Run("PermanentErrors", func(t *testing.T) {
		assert.True(t, IsPermanentError(ErrInvalidToken))
		assert.True(t, IsPermanentError(ErrTokenExpired))
		assert.True(t, IsPermanentError(ErrAuthenticationFailed))
		assert.False(t, IsPermanentError(ErrProviderUnavailable))
	})

	t.Run("RetryDelay", func(t *testing.T) {
		baseDelay := 100 * time.Millisecond

		// First retry
		delay1 := GetRetryDelay(1, baseDelay)
		assert.GreaterOrEqual(t, delay1, baseDelay)
		assert.LessOrEqual(t, delay1, baseDelay*2)

		// Second retry (should be longer)
		delay2 := GetRetryDelay(2, baseDelay)
		assert.Greater(t, delay2, delay1)

		// Should not exceed max delay
		delayMax := GetRetryDelay(10, baseDelay)
		assert.LessOrEqual(t, delayMax, 5*time.Minute)
	})
}

func TestTokenFormatting(t *testing.T) {
	t.Run("APNsTokenFormatting", func(t *testing.T) {
		// Test removing spaces and brackets
		token := "<abc 123 def 456>"
		formatted := FormatDeviceToken(token, PushProviderAPNs)
		assert.Equal(t, "abc123def456", formatted)

		// Test already clean token
		token = "abc123def456"
		formatted = FormatDeviceToken(token, PushProviderAPNs)
		assert.Equal(t, "abc123def456", formatted)
	})

	t.Run("FCMTokenFormatting", func(t *testing.T) {
		// FCM tokens should not be modified
		token := "fcm_token:with-special_chars.123"
		formatted := FormatDeviceToken(token, PushProviderFCM)
		assert.Equal(t, token, formatted)
	})
}