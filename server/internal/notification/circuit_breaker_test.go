package notification

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupCircuitBreakerTest(t *testing.T) (*CircuitBreaker, *redis.Client, *miniredis.Miniredis) {
	// Create miniredis instance
	mr, err := miniredis.Run()
	require.NoError(t, err)

	// Create Redis client
	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	logger := zap.NewNop()
	config := DefaultCircuitBreakerConfig()
	// Use shorter timeout for testing
	config.Timeout = 100 * time.Millisecond

	// Create mock provider
	mockProvider := &MockPushProvider{
		providerType: PushProviderFCM,
	}

	cb := NewCircuitBreaker(client, logger, config, mockProvider)

	return cb, client, mr
}

func TestDefaultCircuitBreakerConfig(t *testing.T) {
	config := DefaultCircuitBreakerConfig()

	assert.Equal(t, 5, config.FailureThreshold, "Should have 5 failure threshold")
	assert.Equal(t, 2, config.SuccessThreshold, "Should have 2 success threshold")
	assert.Equal(t, 5*time.Minute, config.Timeout, "Should have 5 minute timeout")
	assert.Equal(t, 3, config.HalfOpenMaxRequests, "Should have 3 half-open max requests")
}

func TestCircuitBreaker_InitialState(t *testing.T) {
	cb, _, mr := setupCircuitBreakerTest(t)
	defer mr.Close()

	state := cb.GetState()
	assert.Equal(t, CircuitStateClosed, state, "Initial state should be closed")
}

func TestCircuitBreaker_OpenAfterThreshold(t *testing.T) {
	cb, _, mr := setupCircuitBreakerTest(t)
	defer mr.Close()
	ctx := context.Background()

	// Trigger failures up to threshold
	for i := 0; i < cb.config.FailureThreshold; i++ {
		err := cb.Call(ctx, func() error {
			return errors.New("test error")
		})
		assert.Error(t, err)
	}

	// Circuit should be open
	state := cb.GetState()
	assert.Equal(t, CircuitStateOpen, state, "Circuit should be open after threshold failures")
}

func TestCircuitBreaker_FailImmediatelyWhenOpen(t *testing.T) {
	cb, _, mr := setupCircuitBreakerTest(t)
	defer mr.Close()
	ctx := context.Background()

	// Trigger failures to open circuit
	for i := 0; i < cb.config.FailureThreshold; i++ {
		cb.Call(ctx, func() error {
			return errors.New("test error")
		})
	}

	// Verify circuit is open
	assert.Equal(t, CircuitStateOpen, cb.GetState())

	// Next call should fail immediately without executing function
	called := false
	err := cb.Call(ctx, func() error {
		called = true
		return nil
	})

	assert.Error(t, err, "Should fail when circuit is open")
	assert.Equal(t, ErrCircuitOpen, err, "Should return circuit open error")
	assert.False(t, called, "Function should not be called when circuit is open")
}

func TestCircuitBreaker_TransitionToHalfOpen(t *testing.T) {
	cb, _, mr := setupCircuitBreakerTest(t)
	defer mr.Close()
	ctx := context.Background()

	// Open the circuit
	for i := 0; i < cb.config.FailureThreshold; i++ {
		cb.Call(ctx, func() error {
			return errors.New("test error")
		})
	}

	assert.Equal(t, CircuitStateOpen, cb.GetState())

	// Wait for timeout
	time.Sleep(cb.config.Timeout + 10*time.Millisecond)

	// Next call should transition to half-open
	cb.Call(ctx, func() error {
		return nil
	})

	state := cb.GetState()
	assert.Equal(t, CircuitStateHalfOpen, state, "Should transition to half-open after timeout")
}

func TestCircuitBreaker_CloseAfterSuccessInHalfOpen(t *testing.T) {
	cb, _, mr := setupCircuitBreakerTest(t)
	defer mr.Close()
	ctx := context.Background()

	// Open the circuit
	for i := 0; i < cb.config.FailureThreshold; i++ {
		cb.Call(ctx, func() error {
			return errors.New("test error")
		})
	}

	// Wait for timeout to transition to half-open
	time.Sleep(cb.config.Timeout + 10*time.Millisecond)

	// Make successful calls to close circuit
	for i := 0; i < cb.config.SuccessThreshold; i++ {
		err := cb.Call(ctx, func() error {
			return nil
		})
		assert.NoError(t, err)
	}

	state := cb.GetState()
	assert.Equal(t, CircuitStateClosed, state, "Should close after success threshold in half-open")
}

func TestCircuitBreaker_ReopenOnFailureInHalfOpen(t *testing.T) {
	cb, _, mr := setupCircuitBreakerTest(t)
	defer mr.Close()
	ctx := context.Background()

	// Open the circuit
	for i := 0; i < cb.config.FailureThreshold; i++ {
		cb.Call(ctx, func() error {
			return errors.New("test error")
		})
	}

	// Wait for timeout to transition to half-open
	time.Sleep(cb.config.Timeout + 10*time.Millisecond)

	// One successful call
	cb.Call(ctx, func() error {
		return nil
	})

	assert.Equal(t, CircuitStateHalfOpen, cb.GetState())

	// Fail in half-open state
	cb.Call(ctx, func() error {
		return errors.New("test error")
	})

	state := cb.GetState()
	assert.Equal(t, CircuitStateOpen, state, "Should reopen on failure in half-open state")
}

func TestCircuitBreaker_LimitRequestsInHalfOpen(t *testing.T) {
	cb, _, mr := setupCircuitBreakerTest(t)
	defer mr.Close()
	ctx := context.Background()

	// Open the circuit
	for i := 0; i < cb.config.FailureThreshold; i++ {
		cb.Call(ctx, func() error {
			return errors.New("test error")
		})
	}

	// Wait for timeout
	time.Sleep(cb.config.Timeout + 10*time.Millisecond)

	// Make max allowed requests in half-open
	for i := 0; i < cb.config.HalfOpenMaxRequests; i++ {
		err := cb.Call(ctx, func() error {
			return nil
		})
		assert.NoError(t, err)
	}

	// Next request should be rejected
	err := cb.Call(ctx, func() error {
		return nil
	})

	assert.Error(t, err, "Should reject requests beyond half-open max")
	assert.Equal(t, ErrCircuitHalfOpen, err)
}

func TestCircuitBreaker_ResetFailureCountAfterTimeout(t *testing.T) {
	cb, _, mr := setupCircuitBreakerTest(t)
	defer mr.Close()
	ctx := context.Background()

	// Use longer reset timeout for this test
	cb.config.ResetTimeout = 50 * time.Millisecond

	// Make some failures (but not enough to open circuit)
	for i := 0; i < 2; i++ {
		cb.Call(ctx, func() error {
			return errors.New("test error")
		})
	}

	assert.Equal(t, CircuitStateClosed, cb.GetState(), "Should still be closed")

	// Wait for reset timeout
	time.Sleep(cb.config.ResetTimeout + 10*time.Millisecond)

	// Make a successful call - failures should be reset
	err := cb.Call(ctx, func() error {
		return nil
	})
	assert.NoError(t, err)

	// Now make more failures - should need full threshold again
	for i := 0; i < cb.config.FailureThreshold-1; i++ {
		cb.Call(ctx, func() error {
			return errors.New("test error")
		})
	}

	// Should still be closed (failures were reset)
	assert.Equal(t, CircuitStateClosed, cb.GetState())
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb, _, mr := setupCircuitBreakerTest(t)
	defer mr.Close()
	ctx := context.Background()

	// Open the circuit
	for i := 0; i < cb.config.FailureThreshold; i++ {
		cb.Call(ctx, func() error {
			return errors.New("test error")
		})
	}

	assert.Equal(t, CircuitStateOpen, cb.GetState())

	// Manually reset
	cb.Reset()

	state := cb.GetState()
	assert.Equal(t, CircuitStateClosed, state, "Should be closed after reset")

	// Should work normally after reset
	err := cb.Call(ctx, func() error {
		return nil
	})
	assert.NoError(t, err)
}

func TestCircuitBreaker_GetMetrics(t *testing.T) {
	cb, _, mr := setupCircuitBreakerTest(t)
	defer mr.Close()
	ctx := context.Background()

	// Make some failures
	for i := 0; i < 2; i++ {
		cb.Call(ctx, func() error {
			return errors.New("test error")
		})
	}

	metrics := cb.GetMetrics()
	assert.NotNil(t, metrics)
	assert.Equal(t, string(PushProviderFCM), metrics["provider"])
	assert.Equal(t, string(CircuitStateClosed), metrics["state"])
	assert.Equal(t, 2, metrics["failures"])
	assert.Equal(t, 5, metrics["failure_threshold"])
}

func TestCircuitBreaker_IsHealthy(t *testing.T) {
	cb, _, mr := setupCircuitBreakerTest(t)
	defer mr.Close()
	ctx := context.Background()

	// Closed state should be healthy
	assert.True(t, cb.IsHealthy(), "Closed state should be healthy")

	// Open the circuit
	for i := 0; i < cb.config.FailureThreshold; i++ {
		cb.Call(ctx, func() error {
			return errors.New("test error")
		})
	}

	// Open state should not be healthy
	assert.False(t, cb.IsHealthy(), "Open state should not be healthy")

	// Wait for half-open
	time.Sleep(cb.config.Timeout + 10*time.Millisecond)
	cb.Call(ctx, func() error {
		return nil
	})

	// Half-open state should be healthy
	assert.True(t, cb.IsHealthy(), "Half-open state should be healthy")
}

func TestCircuitBreakerManager(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	logger := zap.NewNop()
	config := DefaultCircuitBreakerConfig()

	cbm := NewCircuitBreakerManager(client, logger, config)

	mockProviderFCM := &MockPushProvider{providerType: PushProviderFCM}
	mockProviderAPNs := &MockPushProvider{providerType: PushProviderAPNs}

	t.Run("Get breaker for provider", func(t *testing.T) {
		breaker := cbm.GetBreaker(mockProviderFCM)
		assert.NotNil(t, breaker)
		assert.Equal(t, CircuitStateClosed, breaker.GetState())
	})

	t.Run("Get same breaker for same provider", func(t *testing.T) {
		breaker1 := cbm.GetBreaker(mockProviderFCM)
		breaker2 := cbm.GetBreaker(mockProviderFCM)
		assert.Same(t, breaker1, breaker2, "Should return same breaker instance")
	})

	t.Run("Get different breakers for different providers", func(t *testing.T) {
		breakerFCM := cbm.GetBreaker(mockProviderFCM)
		breakerAPNs := cbm.GetBreaker(mockProviderAPNs)
		assert.NotSame(t, breakerFCM, breakerAPNs, "Should return different breaker instances")
	})

	t.Run("Get all metrics", func(t *testing.T) {
		// Ensure both breakers are created
		cbm.GetBreaker(mockProviderFCM)
		cbm.GetBreaker(mockProviderAPNs)

		metrics := cbm.GetAllMetrics()
		assert.NotNil(t, metrics)
		assert.Contains(t, metrics, string(PushProviderFCM))
		assert.Contains(t, metrics, string(PushProviderAPNs))
	})

	t.Run("Reset all breakers", func(t *testing.T) {
		ctx := context.Background()

		// Open both breakers
		breakerFCM := cbm.GetBreaker(mockProviderFCM)
		for i := 0; i < config.FailureThreshold; i++ {
			breakerFCM.Call(ctx, func() error {
				return errors.New("test error")
			})
		}

		breakerAPNs := cbm.GetBreaker(mockProviderAPNs)
		for i := 0; i < config.FailureThreshold; i++ {
			breakerAPNs.Call(ctx, func() error {
				return errors.New("test error")
			})
		}

		assert.Equal(t, CircuitStateOpen, breakerFCM.GetState())
		assert.Equal(t, CircuitStateOpen, breakerAPNs.GetState())

		// Reset all
		cbm.ResetAll()

		assert.Equal(t, CircuitStateClosed, breakerFCM.GetState())
		assert.Equal(t, CircuitStateClosed, breakerAPNs.GetState())
	})
}

// MockPushProvider for testing
type MockPushProvider struct {
	providerType PushProvider
}

func (m *MockPushProvider) Send(ctx context.Context, notification *PushNotification) (*PushResult, error) {
	return &PushResult{Success: true}, nil
}

func (m *MockPushProvider) SendBatch(ctx context.Context, notifications []*PushNotification) ([]*PushResult, error) {
	results := make([]*PushResult, len(notifications))
	for i := range results {
		results[i] = &PushResult{Success: true}
	}
	return results, nil
}

func (m *MockPushProvider) ValidateToken(token string) error {
	return nil
}

func (m *MockPushProvider) GetProviderType() PushProvider {
	return m.providerType
}

func (m *MockPushProvider) IsAvailable(ctx context.Context) bool {
	return true
}

func (m *MockPushProvider) GetMetrics() map[string]interface{} {
	return make(map[string]interface{})
}

func (m *MockPushProvider) Close() error {
	return nil
}
