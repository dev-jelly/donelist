package websocket

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupTestReconnectionManager(t *testing.T) (*ReconnectionManager, *miniredis.Miniredis) {
	mr := miniredis.RunT(t)

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	logger, _ := zap.NewDevelopment()

	config := DefaultReconnectionConfig()
	config.BaseInterval = 100 * time.Millisecond
	config.MaxInterval = 1 * time.Second
	config.ResumptionWindow = 5 * time.Second
	config.SessionTTL = 1 * time.Hour

	manager := NewReconnectionManager(redisClient, config, logger)

	return manager, mr
}

func TestCreateSession(t *testing.T) {
	manager, mr := setupTestReconnectionManager(t)
	defer mr.Close()

	ctx := context.Background()

	// Set up providers
	manager.SetSequenceProvider(func(userID string) uint64 {
		return 100
	})

	manager.SetSubscriptionLoader(func(userID string) []string {
		return []string{"room1", "room2"}
	})

	metadata := map[string]interface{}{
		"device": "mobile",
		"app_version": "1.0.0",
	}

	session, err := manager.CreateSession(ctx, "user123", metadata)
	require.NoError(t, err)
	assert.NotEmpty(t, session.SessionID)
	assert.Equal(t, "user123", session.UserID)
	assert.Equal(t, uint64(100), session.LastAckSeq)
	assert.Equal(t, []string{"room1", "room2"}, session.Subscriptions)
	assert.Equal(t, metadata, session.Metadata)

	// Verify session is stored in Redis
	key := "session:" + session.SessionID
	exists := mr.Exists(key)
	assert.True(t, exists)
}

func TestAttemptResume(t *testing.T) {
	manager, mr := setupTestReconnectionManager(t)
	defer mr.Close()

	ctx := context.Background()

	// Create a session
	session, err := manager.CreateSession(ctx, "user123", nil)
	require.NoError(t, err)

	// Set sequence provider
	manager.SetSequenceProvider(func(userID string) uint64 {
		return 105 // 5 messages ahead
	})

	// Attempt to resume with valid session
	resumedSession, canResume, err := manager.AttemptResume(ctx, session.SessionID, 100)
	require.NoError(t, err)
	assert.True(t, canResume)
	assert.NotNil(t, resumedSession)
	assert.Equal(t, session.SessionID, resumedSession.SessionID)

	// Check metrics
	assert.Equal(t, uint64(1), atomic.LoadUint64(&manager.totalReconnects))
	assert.Equal(t, uint64(1), atomic.LoadUint64(&manager.successfulResumes))
}

func TestAttemptResumeExpiredSession(t *testing.T) {
	manager, mr := setupTestReconnectionManager(t)
	defer mr.Close()

	manager.config.ResumptionWindow = 1 * time.Millisecond // Very short for testing
	ctx := context.Background()

	// Create a session
	session, err := manager.CreateSession(ctx, "user123", nil)
	require.NoError(t, err)

	// Wait for session to expire
	time.Sleep(10 * time.Millisecond)

	// Attempt to resume expired session
	resumedSession, canResume, err := manager.AttemptResume(ctx, session.SessionID, 100)
	require.NoError(t, err)
	assert.False(t, canResume)
	assert.Nil(t, resumedSession)

	// Check metrics - should require full resync
	assert.Equal(t, uint64(1), atomic.LoadUint64(&manager.totalReconnects))
	assert.Equal(t, uint64(1), atomic.LoadUint64(&manager.fullResyncs))
}

func TestAttemptResumeLargeGap(t *testing.T) {
	manager, mr := setupTestReconnectionManager(t)
	defer mr.Close()

	manager.config.SnapshotThreshold = 10
	ctx := context.Background()

	// Create a session
	session, err := manager.CreateSession(ctx, "user123", nil)
	require.NoError(t, err)

	// Set sequence provider with large gap
	manager.SetSequenceProvider(func(userID string) uint64 {
		return 200 // 100 messages ahead (gap > threshold)
	})

	// Attempt to resume with large sequence gap
	resumedSession, canResume, err := manager.AttemptResume(ctx, session.SessionID, 100)
	require.NoError(t, err)
	assert.False(t, canResume)
	assert.Nil(t, resumedSession)

	// Check metrics - should require full resync
	assert.Equal(t, uint64(1), atomic.LoadUint64(&manager.totalReconnects))
	assert.Equal(t, uint64(1), atomic.LoadUint64(&manager.fullResyncs))
}

func TestBackoffWithJitter(t *testing.T) {
	manager, _ := setupTestReconnectionManager(t)

	manager.config.JitterFactor = 0.3

	attempt := &ReconnectAttempt{
		Attempt: 0,
	}

	// Test multiple backoff calculations
	backoffs := make([]time.Duration, 10)
	for i := 0; i < 10; i++ {
		attempt.Attempt = i
		backoffs[i] = manager.calculateBackoffWithJitter(attempt)
	}

	// Verify exponential growth with cap
	for i := 1; i < len(backoffs); i++ {
		if backoffs[i] < manager.config.MaxInterval {
			// Should grow exponentially (with jitter variance)
			assert.Greater(t, backoffs[i], backoffs[i-1]*time.Duration(1.5)) // At least 1.5x growth
		} else {
			// Should be capped at max interval
			assert.LessOrEqual(t, backoffs[i], manager.config.MaxInterval)
		}
	}

	// Test jitter - run multiple times and check for variance
	jitterBackoffs := make([]time.Duration, 100)
	attempt.Attempt = 3 // Fixed attempt number
	for i := 0; i < 100; i++ {
		jitterBackoffs[i] = manager.calculateBackoffWithJitter(attempt)
	}

	// Check that we have variance due to jitter
	min := jitterBackoffs[0]
	max := jitterBackoffs[0]
	for _, b := range jitterBackoffs {
		if b < min {
			min = b
		}
		if b > max {
			max = b
		}
	}

	// Jitter should create noticeable variance
	variance := float64(max-min) / float64(max+min) * 2
	assert.Greater(t, variance, 0.1) // At least 10% variance
}

func TestGetReconnectStrategy(t *testing.T) {
	manager, _ := setupTestReconnectionManager(t)

	// Test initial strategy (no previous attempts)
	strategy := manager.GetReconnectStrategy("unknown_session")
	assert.Equal(t, "exponential_backoff_with_jitter", strategy["strategy"])
	assert.Equal(t, int64(100), strategy["initial_delay"])
	assert.Equal(t, int64(1000), strategy["max_delay"])
	assert.Equal(t, 2.0, strategy["multiplier"])
	assert.Equal(t, 0.3, strategy["jitter_factor"])
	assert.True(t, strategy["can_resume"].(bool))

	// Create an attempt and test strategy
	manager.reconnectAttempts["session123"] = &ReconnectAttempt{
		SessionID:      "session123",
		UserID:         "user123",
		Attempt:        3,
		CurrentBackoff: 400 * time.Millisecond,
		State:          "backing_off",
	}

	strategy = manager.GetReconnectStrategy("session123")
	assert.Equal(t, 4, strategy["attempt"]) // Next attempt would be 4
	assert.Equal(t, 7, strategy["remaining_tries"]) // 10 max - 3 done = 7
	assert.Equal(t, "backing_off", strategy["state"])
}

func TestPerformFullResync(t *testing.T) {
	manager, mr := setupTestReconnectionManager(t)
	defer mr.Close()

	ctx := context.Background()

	// Set snapshot provider
	testSnapshot := json.RawMessage(`{"state": "full_snapshot", "data": [1,2,3]}`)
	manager.SetSnapshotProvider(func(userID string) (json.RawMessage, error) {
		return testSnapshot, nil
	})

	session, err := manager.PerformFullResync(ctx, "user456")
	require.NoError(t, err)
	assert.NotNil(t, session)
	assert.Equal(t, "user456", session.UserID)
	assert.Equal(t, testSnapshot, session.Snapshot)

	// Check metadata
	assert.Equal(t, "full_resync", session.Metadata["resync_reason"])
}

func TestUpdateSessionActivity(t *testing.T) {
	manager, mr := setupTestReconnectionManager(t)
	defer mr.Close()

	ctx := context.Background()

	// Create a session
	session, err := manager.CreateSession(ctx, "user123", nil)
	require.NoError(t, err)

	originalActivity := session.LastActivity

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	// Update activity
	manager.UpdateSessionActivity(session.SessionID, 150, "msg_150")

	// Check updated values
	manager.sessionsMu.RLock()
	updatedSession := manager.sessions[session.SessionID]
	manager.sessionsMu.RUnlock()

	assert.Equal(t, uint64(150), updatedSession.LastAckSeq)
	assert.Equal(t, "msg_150", updatedSession.LastMessageID)
	assert.True(t, updatedSession.LastActivity.After(originalActivity))
}

func TestCleanupInactiveSessions(t *testing.T) {
	manager, mr := setupTestReconnectionManager(t)
	defer mr.Close()

	manager.config.SessionTTL = 1 * time.Millisecond // Very short for testing
	ctx := context.Background()

	// Create multiple sessions
	session1, _ := manager.CreateSession(ctx, "user1", nil)
	session2, _ := manager.CreateSession(ctx, "user2", nil)

	// Wait for sessions to expire
	time.Sleep(10 * time.Millisecond)

	// Create a new fresh session
	session3, _ := manager.CreateSession(ctx, "user3", nil)

	// Run cleanup
	manager.CleanupInactiveSessions(ctx)

	// Check that old sessions are removed
	manager.sessionsMu.RLock()
	_, exists1 := manager.sessions[session1.SessionID]
	_, exists2 := manager.sessions[session2.SessionID]
	_, exists3 := manager.sessions[session3.SessionID]
	manager.sessionsMu.RUnlock()

	assert.False(t, exists1)
	assert.False(t, exists2)
	assert.True(t, exists3) // Fresh session should still exist
}

func TestConcurrentSessionOperations(t *testing.T) {
	manager, mr := setupTestReconnectionManager(t)
	defer mr.Close()

	ctx := context.Background()
	var wg sync.WaitGroup
	sessionCount := 50

	// Create sessions concurrently
	sessions := make([]*SessionState, sessionCount)
	for i := 0; i < sessionCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			session, err := manager.CreateSession(ctx, string(rune(idx)), nil)
			if err == nil {
				sessions[idx] = session
			}
		}(i)
	}

	wg.Wait()

	// Verify all sessions were created
	for i, session := range sessions {
		if session == nil {
			t.Errorf("Session %d was not created", i)
			continue
		}
		assert.NotEmpty(t, session.SessionID)
		assert.Equal(t, string(rune(i)), session.UserID)
	}

	// Attempt concurrent resumes
	var successfulResumes uint32
	for _, session := range sessions {
		if session == nil {
			continue
		}
		wg.Add(1)
		go func(s *SessionState) {
			defer wg.Done()
			_, canResume, err := manager.AttemptResume(ctx, s.SessionID, 0)
			if err == nil && canResume {
				atomic.AddUint32(&successfulResumes, 1)
			}
		}(session)
	}

	wg.Wait()

	// Some resumes should have succeeded
	assert.Greater(t, atomic.LoadUint32(&successfulResumes), uint32(0))
}

func TestGetMetrics(t *testing.T) {
	manager, mr := setupTestReconnectionManager(t)
	defer mr.Close()

	ctx := context.Background()

	// Create some sessions and attempts
	manager.CreateSession(ctx, "user1", nil)
	manager.CreateSession(ctx, "user2", nil)

	// Simulate some reconnects
	atomic.StoreUint64(&manager.totalReconnects, 100)
	atomic.StoreUint64(&manager.successfulResumes, 85)
	atomic.StoreUint64(&manager.fullResyncs, 15)

	metrics := manager.GetMetrics()

	assert.Equal(t, 2, metrics["active_sessions"])
	assert.Equal(t, uint64(100), metrics["total_reconnects"])
	assert.Equal(t, uint64(85), metrics["successful_resumes"])
	assert.Equal(t, uint64(15), metrics["full_resyncs"])
	assert.InDelta(t, 85.0, metrics["resume_success_rate"], 0.01)

	// Check config is included
	config := metrics["config"].(map[string]interface{})
	assert.Equal(t, 0.1, config["base_interval"]) // 100ms in seconds
	assert.Equal(t, 1.0, config["max_interval"])  // 1s in seconds
	assert.Equal(t, 2.0, config["multiplier"])
	assert.Equal(t, 0.3, config["jitter_factor"])
}

func BenchmarkCalculateBackoffWithJitter(b *testing.B) {
	manager, _ := setupTestReconnectionManager(b)
	manager.config.JitterFactor = 0.3

	attempt := &ReconnectAttempt{
		Attempt: 5,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = manager.calculateBackoffWithJitter(attempt)
	}
}

func BenchmarkSessionCreation(b *testing.B) {
	manager, mr := setupTestReconnectionManager(b)
	defer mr.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = manager.CreateSession(ctx, "user"+string(rune(i)), nil)
	}
}

func BenchmarkSessionResumption(b *testing.B) {
	manager, mr := setupTestReconnectionManager(b)
	defer mr.Close()

	ctx := context.Background()

	// Pre-create a session
	session, _ := manager.CreateSession(ctx, "bench_user", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = manager.AttemptResume(ctx, session.SessionID, uint64(i))
	}
}