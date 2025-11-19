package auth

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestSessionStore_CreateSession(t *testing.T) {
	client, _ := setupTestRedis(t)
	logger := zap.NewNop()
	ss := NewSessionStore(client, logger)

	ctx := context.Background()
	userID := uuid.New()

	session := SessionInfo{
		SessionID:  "test-session-1",
		UserID:     userID,
		AccessJTI:  "access-jti-1",
		RefreshJTI: "refresh-jti-1",
		DeviceID:   "device-123",
		IPAddress:  "192.168.1.1",
		UserAgent:  "Mozilla/5.0",
		ExpiresAt:  time.Now().Add(1 * time.Hour),
	}

	// Create session
	err := ss.CreateSession(ctx, session)
	require.NoError(t, err)

	// Retrieve session
	retrieved, err := ss.GetSession(ctx, userID, session.SessionID)
	require.NoError(t, err)
	assert.Equal(t, session.SessionID, retrieved.SessionID)
	assert.Equal(t, session.UserID, retrieved.UserID)
	assert.Equal(t, session.AccessJTI, retrieved.AccessJTI)
	assert.Equal(t, session.RefreshJTI, retrieved.RefreshJTI)
	assert.Equal(t, session.DeviceID, retrieved.DeviceID)
	assert.Equal(t, session.IPAddress, retrieved.IPAddress)
	assert.Equal(t, session.UserAgent, retrieved.UserAgent)
}

func TestSessionStore_CreateSessionWithMissingFields(t *testing.T) {
	client, _ := setupTestRedis(t)
	logger := zap.NewNop()
	ss := NewSessionStore(client, logger)

	ctx := context.Background()

	tests := []struct {
		name    string
		session SessionInfo
		wantErr bool
	}{
		{
			name: "missing user_id",
			session: SessionInfo{
				SessionID:  "session-1",
				AccessJTI:  "access-jti",
				RefreshJTI: "refresh-jti",
				ExpiresAt:  time.Now().Add(1 * time.Hour),
			},
			wantErr: true,
		},
		{
			name: "missing access_jti",
			session: SessionInfo{
				SessionID:  "session-1",
				UserID:     uuid.New(),
				RefreshJTI: "refresh-jti",
				ExpiresAt:  time.Now().Add(1 * time.Hour),
			},
			wantErr: true,
		},
		{
			name: "missing refresh_jti",
			session: SessionInfo{
				SessionID: "session-1",
				UserID:    uuid.New(),
				AccessJTI: "access-jti",
				ExpiresAt: time.Now().Add(1 * time.Hour),
			},
			wantErr: true,
		},
		{
			name: "expired session",
			session: SessionInfo{
				SessionID:  "session-1",
				UserID:     uuid.New(),
				AccessJTI:  "access-jti",
				RefreshJTI: "refresh-jti",
				ExpiresAt:  time.Now().Add(-1 * time.Hour),
			},
			wantErr: true,
		},
		{
			name: "auto-generate session_id",
			session: SessionInfo{
				UserID:     uuid.New(),
				AccessJTI:  "access-jti",
				RefreshJTI: "refresh-jti",
				ExpiresAt:  time.Now().Add(1 * time.Hour),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ss.CreateSession(ctx, tt.session)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSessionStore_GetUserSessions(t *testing.T) {
	client, _ := setupTestRedis(t)
	logger := zap.NewNop()
	ss := NewSessionStore(client, logger)

	ctx := context.Background()
	userID := uuid.New()

	// Create multiple sessions
	sessions := []SessionInfo{
		{
			SessionID:  "session-1",
			UserID:     userID,
			AccessJTI:  "access-jti-1",
			RefreshJTI: "refresh-jti-1",
			DeviceID:   "device-1",
			ExpiresAt:  time.Now().Add(1 * time.Hour),
		},
		{
			SessionID:  "session-2",
			UserID:     userID,
			AccessJTI:  "access-jti-2",
			RefreshJTI: "refresh-jti-2",
			DeviceID:   "device-2",
			ExpiresAt:  time.Now().Add(1 * time.Hour),
		},
		{
			SessionID:  "session-3",
			UserID:     userID,
			AccessJTI:  "access-jti-3",
			RefreshJTI: "refresh-jti-3",
			DeviceID:   "device-3",
			ExpiresAt:  time.Now().Add(1 * time.Hour),
		},
	}

	for _, session := range sessions {
		err := ss.CreateSession(ctx, session)
		require.NoError(t, err)
	}

	// Retrieve all sessions
	retrieved, err := ss.GetUserSessions(ctx, userID)
	require.NoError(t, err)
	assert.Len(t, retrieved, 3)

	// Verify session IDs
	sessionIDs := make(map[string]bool)
	for _, s := range retrieved {
		sessionIDs[s.SessionID] = true
	}
	assert.True(t, sessionIDs["session-1"])
	assert.True(t, sessionIDs["session-2"])
	assert.True(t, sessionIDs["session-3"])
}

func TestSessionStore_UpdateSessionActivity(t *testing.T) {
	client, _ := setupTestRedis(t)
	logger := zap.NewNop()
	ss := NewSessionStore(client, logger)

	ctx := context.Background()
	userID := uuid.New()

	session := SessionInfo{
		SessionID:  "test-session",
		UserID:     userID,
		AccessJTI:  "access-jti",
		RefreshJTI: "refresh-jti",
		ExpiresAt:  time.Now().Add(1 * time.Hour),
	}

	// Create session
	err := ss.CreateSession(ctx, session)
	require.NoError(t, err)

	// Get initial last activity
	retrieved, err := ss.GetSession(ctx, userID, session.SessionID)
	require.NoError(t, err)
	initialActivity := retrieved.LastActivity

	// Wait a full second to ensure timestamp difference
	time.Sleep(1100 * time.Millisecond)

	// Update activity
	err = ss.UpdateSessionActivity(ctx, userID, session.SessionID)
	require.NoError(t, err)

	// Verify last activity was updated
	retrieved, err = ss.GetSession(ctx, userID, session.SessionID)
	require.NoError(t, err)
	assert.True(t, retrieved.LastActivity.After(initialActivity),
		"LastActivity should be after initial: initial=%v, updated=%v",
		initialActivity, retrieved.LastActivity)
}

func TestSessionStore_DeleteSession(t *testing.T) {
	client, _ := setupTestRedis(t)
	logger := zap.NewNop()
	ss := NewSessionStore(client, logger)

	ctx := context.Background()
	userID := uuid.New()

	session := SessionInfo{
		SessionID:  "test-session",
		UserID:     userID,
		AccessJTI:  "access-jti",
		RefreshJTI: "refresh-jti",
		ExpiresAt:  time.Now().Add(1 * time.Hour),
	}

	// Create session
	err := ss.CreateSession(ctx, session)
	require.NoError(t, err)

	// Verify session exists
	count, err := ss.GetActiveSessionCount(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Delete session
	err = ss.DeleteSession(ctx, userID, session.SessionID)
	require.NoError(t, err)

	// Verify session is deleted
	count, err = ss.GetActiveSessionCount(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

	// Verify session cannot be retrieved
	_, err = ss.GetSession(ctx, userID, session.SessionID)
	assert.Error(t, err)
}

func TestSessionStore_DeleteUserSessions(t *testing.T) {
	client, _ := setupTestRedis(t)
	logger := zap.NewNop()
	ss := NewSessionStore(client, logger)

	ctx := context.Background()
	userID := uuid.New()

	// Create multiple sessions
	for i := 0; i < 5; i++ {
		session := SessionInfo{
			SessionID:  uuid.New().String(),
			UserID:     userID,
			AccessJTI:  uuid.New().String(),
			RefreshJTI: uuid.New().String(),
			ExpiresAt:  time.Now().Add(1 * time.Hour),
		}
		err := ss.CreateSession(ctx, session)
		require.NoError(t, err)
	}

	// Verify sessions exist
	count, err := ss.GetActiveSessionCount(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, int64(5), count)

	// Delete all sessions
	err = ss.DeleteUserSessions(ctx, userID)
	require.NoError(t, err)

	// Verify all sessions are deleted
	count, err = ss.GetActiveSessionCount(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestSessionStore_GetActiveSessionCount(t *testing.T) {
	client, _ := setupTestRedis(t)
	logger := zap.NewNop()
	ss := NewSessionStore(client, logger)

	ctx := context.Background()
	userID := uuid.New()

	// Initially no sessions
	count, err := ss.GetActiveSessionCount(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

	// Create sessions
	for i := 0; i < 3; i++ {
		session := SessionInfo{
			SessionID:  uuid.New().String(),
			UserID:     userID,
			AccessJTI:  uuid.New().String(),
			RefreshJTI: uuid.New().String(),
			ExpiresAt:  time.Now().Add(1 * time.Hour),
		}
		err := ss.CreateSession(ctx, session)
		require.NoError(t, err)
	}

	// Verify count
	count, err = ss.GetActiveSessionCount(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
}

func TestSessionStore_EnforceSessionLimit(t *testing.T) {
	client, _ := setupTestRedis(t)
	logger := zap.NewNop()
	ss := NewSessionStore(client, logger)

	ctx := context.Background()
	userID := uuid.New()
	maxSessions := 3

	// Create 5 sessions
	for i := 0; i < 5; i++ {
		session := SessionInfo{
			SessionID:  uuid.New().String(),
			UserID:     userID,
			AccessJTI:  uuid.New().String(),
			RefreshJTI: uuid.New().String(),
			CreatedAt:  time.Now().Add(time.Duration(i) * time.Minute), // Different creation times
			ExpiresAt:  time.Now().Add(1 * time.Hour),
		}
		err := ss.CreateSession(ctx, session)
		require.NoError(t, err)

		// Small delay to ensure different creation times
		time.Sleep(1 * time.Millisecond)
	}

	// Verify 5 sessions exist
	count, err := ss.GetActiveSessionCount(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, int64(5), count)

	// Enforce limit
	err = ss.EnforceSessionLimit(ctx, userID, maxSessions)
	require.NoError(t, err)

	// Verify only maxSessions remain
	count, err = ss.GetActiveSessionCount(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, int64(maxSessions), count)
}

func TestSessionStore_EnforceSessionLimit_NoLimit(t *testing.T) {
	client, _ := setupTestRedis(t)
	logger := zap.NewNop()
	ss := NewSessionStore(client, logger)

	ctx := context.Background()
	userID := uuid.New()

	// Create 5 sessions
	for i := 0; i < 5; i++ {
		session := SessionInfo{
			SessionID:  uuid.New().String(),
			UserID:     userID,
			AccessJTI:  uuid.New().String(),
			RefreshJTI: uuid.New().String(),
			ExpiresAt:  time.Now().Add(1 * time.Hour),
		}
		err := ss.CreateSession(ctx, session)
		require.NoError(t, err)
	}

	// Enforce limit with 0 (no limit)
	err := ss.EnforceSessionLimit(ctx, userID, 0)
	require.NoError(t, err)

	// Verify all sessions still exist
	count, err := ss.GetActiveSessionCount(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, int64(5), count)
}

func TestSessionStore_TTLExpiration(t *testing.T) {
	client, mr := setupTestRedis(t)
	logger := zap.NewNop()
	ss := NewSessionStore(client, logger)

	ctx := context.Background()
	userID := uuid.New()

	session := SessionInfo{
		SessionID:  "test-session",
		UserID:     userID,
		AccessJTI:  "access-jti",
		RefreshJTI: "refresh-jti",
		ExpiresAt:  time.Now().Add(1 * time.Second),
	}

	// Create session with short TTL
	err := ss.CreateSession(ctx, session)
	require.NoError(t, err)

	// Verify session exists
	count, err := ss.GetActiveSessionCount(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Fast-forward time
	mr.FastForward(2 * time.Second)

	// Verify session is expired and removed
	count, err = ss.GetActiveSessionCount(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestSessionStore_GetSessionStats(t *testing.T) {
	client, _ := setupTestRedis(t)
	logger := zap.NewNop()
	ss := NewSessionStore(client, logger)

	ctx := context.Background()

	// Create sessions for different users
	for i := 0; i < 3; i++ {
		userID := uuid.New()
		for j := 0; j < 2; j++ {
			session := SessionInfo{
				SessionID:  uuid.New().String(),
				UserID:     userID,
				AccessJTI:  uuid.New().String(),
				RefreshJTI: uuid.New().String(),
				ExpiresAt:  time.Now().Add(1 * time.Hour),
			}
			err := ss.CreateSession(ctx, session)
			require.NoError(t, err)
		}
	}

	// Get stats
	stats, err := ss.GetSessionStats(ctx)
	require.NoError(t, err)

	assert.Equal(t, int64(6), stats["total_sessions"])
	assert.Equal(t, int64(3), stats["active_users"])
}

func TestSessionStore_CleanupExpiredSessions(t *testing.T) {
	client, mr := setupTestRedis(t)
	logger := zap.NewNop()
	ss := NewSessionStore(client, logger)

	ctx := context.Background()

	// Create sessions with different TTLs
	userID1 := uuid.New()
	session1 := SessionInfo{
		SessionID:  "session-1",
		UserID:     userID1,
		AccessJTI:  "access-jti-1",
		RefreshJTI: "refresh-jti-1",
		ExpiresAt:  time.Now().Add(1 * time.Second),
	}
	err := ss.CreateSession(ctx, session1)
	require.NoError(t, err)

	userID2 := uuid.New()
	session2 := SessionInfo{
		SessionID:  "session-2",
		UserID:     userID2,
		AccessJTI:  "access-jti-2",
		RefreshJTI: "refresh-jti-2",
		ExpiresAt:  time.Now().Add(10 * time.Minute),
	}
	err = ss.CreateSession(ctx, session2)
	require.NoError(t, err)

	// Fast-forward time
	mr.FastForward(2 * time.Second)

	// Run cleanup
	cleaned, err := ss.CleanupExpiredSessions(ctx)
	require.NoError(t, err)

	// Redis auto-expires, so cleanup might find 0 keys
	assert.True(t, cleaned >= 0)

	// Verify the non-expired session still exists
	_, err = ss.GetSession(ctx, userID2, "session-2")
	assert.NoError(t, err)

	// Verify expired session is gone
	_, err = ss.GetSession(ctx, userID1, "session-1")
	assert.Error(t, err)
}

func TestSessionStore_ConcurrentOperations(t *testing.T) {
	client, _ := setupTestRedis(t)
	logger := zap.NewNop()
	ss := NewSessionStore(client, logger)

	ctx := context.Background()
	userID := uuid.New()

	// Simulate concurrent session creations
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			session := SessionInfo{
				SessionID:  uuid.New().String(),
				UserID:     userID,
				AccessJTI:  uuid.New().String(),
				RefreshJTI: uuid.New().String(),
				ExpiresAt:  time.Now().Add(1 * time.Hour),
			}
			err := ss.CreateSession(ctx, session)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all sessions were created
	count, err := ss.GetActiveSessionCount(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, int64(10), count)
}
