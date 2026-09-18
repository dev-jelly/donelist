package notification

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestMockProvider(t *testing.T) {
	t.Run("BasicSend", func(t *testing.T) {
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
		assert.Equal(t, "test_token_123", sent[0].DeviceToken)
	})

	t.Run("ConfiguredFailure", func(t *testing.T) {
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

	t.Run("InvalidToken", func(t *testing.T) {
		provider := NewMockProvider(&MockProviderConfig{
			InvalidTokenPattern: "invalid_",
		})

		notification := &PushNotification{
			DeviceToken: "invalid_token_123",
			Title:       "Test",
			Body:        "Body",
		}

		result, err := provider.Send(context.Background(), notification)
		require.Error(t, err)
		assert.False(t, result.Success)
		assert.True(t, result.ShouldRemoveToken)
		assert.Equal(t, ErrInvalidToken, result.Error)
	})

	t.Run("ExpiredToken", func(t *testing.T) {
		provider := NewMockProvider(&MockProviderConfig{
			ExpiredTokenPattern: "expired_",
		})

		notification := &PushNotification{
			DeviceToken: "expired_token_456",
			Title:       "Test",
			Body:        "Body",
		}

		result, err := provider.Send(context.Background(), notification)
		require.Error(t, err)
		assert.False(t, result.Success)
		assert.True(t, result.ShouldRemoveToken)
		assert.Equal(t, ErrTokenExpired, result.Error)
	})

	t.Run("RateLimiting", func(t *testing.T) {
		provider := NewMockProvider(&MockProviderConfig{
			SimulateRateLimit: true,
			RateLimitAfter:    2,
			AlwaysSucceed:     true,
		})

		notification := &PushNotification{
			DeviceToken: "test_token",
			Title:       "Test",
			Body:        "Body",
		}

		// First two should succeed
		for i := 0; i < 2; i++ {
			result, err := provider.Send(context.Background(), notification)
			require.NoError(t, err)
			assert.True(t, result.Success)
		}

		// Third should hit rate limit
		result, err := provider.Send(context.Background(), notification)
		require.Error(t, err)
		assert.False(t, result.Success)
		assert.Equal(t, ErrRateLimitExceeded, result.Error)
		assert.True(t, result.Retryable)
		assert.NotNil(t, result.RetryAfter)
	})

	t.Run("SimulatedDelay", func(t *testing.T) {
		provider := NewMockProvider(&MockProviderConfig{
			AlwaysSucceed: true,
			SimulateDelay: 100 * time.Millisecond,
		})

		notification := &PushNotification{
			DeviceToken: "test_token",
			Title:       "Test",
			Body:        "Body",
		}

		start := time.Now()
		result, err := provider.Send(context.Background(), notification)
		elapsed := time.Since(start)

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.GreaterOrEqual(t, elapsed, 100*time.Millisecond)
	})

	t.Run("ContextCancellation", func(t *testing.T) {
		provider := NewMockProvider(&MockProviderConfig{
			SimulateDelay: 1 * time.Second,
		})

		notification := &PushNotification{
			DeviceToken: "test_token",
			Title:       "Test",
			Body:        "Body",
		}

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		result, err := provider.Send(ctx, notification)
		require.Error(t, err)
		assert.False(t, result.Success)
		assert.Equal(t, context.DeadlineExceeded, err)
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

		for i, result := range results {
			assert.True(t, result.Success, "Notification %d should succeed", i)
			assert.NotEmpty(t, result.MessageID)
		}

		sent := provider.GetSentNotifications()
		assert.Len(t, sent, 3)
	})

	t.Run("CustomResponseFunction", func(t *testing.T) {
		customFunc := func(n *PushNotification) *PushResult {
			if n.DeviceToken == "special_token" {
				return &PushResult{
					MessageID:      "custom_id",
					Success:        true,
					CanonicalToken: "canonical_token",
					SentAt:         time.Now(),
				}
			}
			return &PushResult{
				Success: false,
				Error:   errors.New("custom error"),
				SentAt:  time.Now(),
			}
		}

		provider := NewMockProvider(&MockProviderConfig{
			CustomResponseFunc: customFunc,
		})

		// Test special token
		notification := &PushNotification{
			DeviceToken: "special_token",
			Title:       "Test",
			Body:        "Body",
		}

		result, err := provider.Send(context.Background(), notification)
		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Equal(t, "custom_id", result.MessageID)
		assert.Equal(t, "canonical_token", result.CanonicalToken)

		// Test other token
		notification.DeviceToken = "other_token"
		result, err = provider.Send(context.Background(), notification)
		require.Error(t, err)
		assert.False(t, result.Success)
		assert.Equal(t, "custom error", err.Error())
	})

	t.Run("Metrics", func(t *testing.T) {
		provider := NewMockProvider(&MockProviderConfig{
			FailureRate: 0.5, // 50% failure rate
		})

		// Send multiple notifications
		for i := 0; i < 10; i++ {
			notification := &PushNotification{
				DeviceToken: "test_token",
				Title:       "Test",
				Body:        "Body",
			}
			provider.Send(context.Background(), notification)
		}

		metrics := provider.GetMetrics()
		assert.Equal(t, "mock", metrics["provider"])
		assert.Equal(t, int64(10), metrics["total_calls"])

		totalSent := metrics["total_sent"].(int64)
		totalFailed := metrics["total_failed"].(int64)
		assert.Equal(t, int64(10), totalSent+totalFailed)
	})
}

func TestProviderManager(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar().Desugar()

	t.Run("RegisterProvider", func(t *testing.T) {
		manager := NewProviderManager(DefaultPushProviderConfig(), logger)

		mockProvider := NewMockProvider(&MockProviderConfig{
			AlwaysSucceed: true,
		})

		err := manager.RegisterProvider(mockProvider)
		require.NoError(t, err)

		provider, err := manager.GetProvider(PushProviderMock)
		require.NoError(t, err)
		assert.NotNil(t, provider)
		assert.Equal(t, PushProviderMock, provider.GetProviderType())
	})

	t.Run("SendWithRetry", func(t *testing.T) {
		config := DefaultPushProviderConfig()
		config.MaxRetries = 2
		config.RetryBackoff = 10 * time.Millisecond

		manager := NewProviderManager(config, logger)

		mockProvider := NewMockProvider(&MockProviderConfig{})
		mockProvider.SetShouldFail(true)
		mockProvider.SetNextError(ErrProviderUnavailable)

		err := manager.RegisterProvider(mockProvider)
		require.NoError(t, err)

		notification := &PushNotification{
			DeviceToken: "test_token",
			Title:       "Test",
			Body:        "Body",
		}

		// Should fail after retries
		result, err := manager.Send(context.Background(), notification, PushProviderMock)
		require.Error(t, err)
		assert.False(t, result.Success)

		// Check that retries happened
		metrics := manager.GetMetrics()
		assert.Greater(t, metrics["total_retries"].(int64), int64(0))
	})

	t.Run("MultiProviderSend", func(t *testing.T) {
		manager := NewProviderManager(DefaultPushProviderConfig(), logger)

		// Register multiple mock providers
		fcmMock := NewMockProvider(&MockProviderConfig{AlwaysSucceed: true})
		apnsMock := NewMockProvider(&MockProviderConfig{AlwaysSucceed: true})

		// We need to wrap them to return correct provider types
		fcmWrapper := &mockProviderWrapper{MockProvider: fcmMock, providerType: PushProviderFCM}
		apnsWrapper := &mockProviderWrapper{MockProvider: apnsMock, providerType: PushProviderAPNs}

		manager.RegisterProvider(fcmWrapper)
		manager.RegisterProvider(apnsWrapper)

		notification := &PushNotification{
			DeviceToken: "test_token",
			Title:       "Test",
			Body:        "Body",
		}

		results := manager.SendMultiProvider(context.Background(), notification,
			[]PushProvider{PushProviderFCM, PushProviderAPNs})

		assert.Len(t, results, 2)
		assert.True(t, results[PushProviderFCM].Success)
		assert.True(t, results[PushProviderAPNs].Success)
	})

	t.Run("GetAvailableProviders", func(t *testing.T) {
		manager := NewProviderManager(DefaultPushProviderConfig(), logger)

		available := NewMockProvider(&MockProviderConfig{AlwaysSucceed: true})
		unavailable := NewMockProvider(&MockProviderConfig{AlwaysSucceed: true})
		unavailable.SetAvailable(false)

		manager.RegisterProvider(&mockProviderWrapper{available, PushProviderFCM})
		manager.RegisterProvider(&mockProviderWrapper{unavailable, PushProviderAPNs})

		providers := manager.GetAvailableProviders(context.Background())
		assert.Len(t, providers, 1)
		assert.Equal(t, PushProviderFCM, providers[0])
	})
}

func TestProviderValidation(t *testing.T) {
	t.Run("ValidatePushNotification", func(t *testing.T) {
		tests := []struct {
			name         string
			notification *PushNotification
			wantErr      bool
			expectedErr  error
		}{
			{
				name:         "NilNotification",
				notification: nil,
				wantErr:      true,
			},
			{
				name: "EmptyDeviceToken",
				notification: &PushNotification{
					Title: "Test",
					Body:  "Body",
				},
				wantErr: true,
			},
			{
				name: "PayloadTooLarge",
				notification: &PushNotification{
					DeviceToken: "test_token",
					Title:       string(make([]byte, 2000)),
					Body:        string(make([]byte, 2500)),
				},
				wantErr:     true,
				expectedErr: ErrPayloadTooLarge,
			},
			{
				name: "NegativeTTL",
				notification: &PushNotification{
					DeviceToken: "test_token",
					Title:       "Test",
					Body:        "Body",
					TTL:         -1,
				},
				wantErr: true,
			},
			{
				name: "ValidNotification",
				notification: &PushNotification{
					DeviceToken:  "test_token",
					Title:        "Test Title",
					Body:         "Test Body",
					TTL:          3600,
					HighPriority: true,
					Data:         map[string]string{"key": "value"},
				},
				wantErr: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := ValidatePushNotification(tt.notification)
				if tt.wantErr {
					assert.Error(t, err)
					if tt.expectedErr != nil {
						assert.ErrorIs(t, err, tt.expectedErr)
					}
				} else {
					assert.NoError(t, err)
				}
			})
		}
	})

	t.Run("FormatDeviceToken", func(t *testing.T) {
		tests := []struct {
			token    string
			provider PushProvider
			expected string
		}{
			{
				token:    "abc123def456",
				provider: PushProviderAPNs,
				expected: "abc123def456",
			},
			{
				token:    "<abc 123 def 456>",
				provider: PushProviderAPNs,
				expected: "abc123def456",
			},
			{
				token:    "fcm_token_with_special_chars:123",
				provider: PushProviderFCM,
				expected: "fcm_token_with_special_chars:123",
			},
		}

		for _, tt := range tests {
			t.Run(tt.provider.String(), func(t *testing.T) {
				result := FormatDeviceToken(tt.token, tt.provider)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("IsRetryableError", func(t *testing.T) {
		assert.True(t, IsRetryableError(ErrProviderUnavailable))
		assert.True(t, IsRetryableError(ErrRateLimitExceeded))
		assert.True(t, IsRetryableError(context.DeadlineExceeded))
		assert.False(t, IsRetryableError(context.Canceled))
		assert.False(t, IsRetryableError(ErrInvalidToken))
	})

	t.Run("IsPermanentError", func(t *testing.T) {
		assert.True(t, IsPermanentError(ErrInvalidToken))
		assert.True(t, IsPermanentError(ErrTokenExpired))
		assert.True(t, IsPermanentError(ErrAuthenticationFailed))
		assert.False(t, IsPermanentError(ErrProviderUnavailable))
		assert.False(t, IsPermanentError(ErrRateLimitExceeded))
	})
}

// Helper to get string representation of provider
func (p PushProvider) String() string {
	return string(p)
}

// mockProviderWrapper wraps a MockProvider to return a different provider type
type mockProviderWrapper struct {
	*MockProvider
	providerType PushProvider
}

func (m *mockProviderWrapper) GetProviderType() PushProvider {
	return m.providerType
}