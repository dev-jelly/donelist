package websocket

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestHandshakeRequest(t *testing.T) {
	t.Run("marshal and unmarshal handshake request", func(t *testing.T) {
		req := &HandshakeRequest{
			ClientVersion:     "1.0.0",
			SupportedFeatures: []string{"ping", "pong", "rooms"},
			LastAckSeq:        100,
			Capabilities: map[string]interface{}{
				"compression": true,
			},
			Platform: "web",
			DeviceID: "test-device-123",
		}

		data, err := MarshalHandshakeRequest(req)
		require.NoError(t, err)

		parsed, err := UnmarshalHandshakeRequest(data)
		require.NoError(t, err)

		assert.Equal(t, req.ClientVersion, parsed.ClientVersion)
		assert.Equal(t, req.SupportedFeatures, parsed.SupportedFeatures)
		assert.Equal(t, req.LastAckSeq, parsed.LastAckSeq)
		assert.Equal(t, req.Platform, parsed.Platform)
		assert.Equal(t, req.DeviceID, parsed.DeviceID)
	})
}

func TestHandshakeResponse(t *testing.T) {
	t.Run("marshal and unmarshal handshake response", func(t *testing.T) {
		resp := &HandshakeResponse{
			SessionID:         "session-123",
			ServerVersion:     "1.0.0",
			SupportedFeatures: []string{"ping", "pong", "rooms", "presence"},
			EnabledFeatures:   []string{"ping", "pong", "rooms"},
			HeartbeatInterval: 30000,
			HeartbeatTimeout:  60000,
			MaxMessageSize:    512000,
			ResumeFromSeq:     100,
			Status:            "accepted",
			Message:           "Handshake successful",
		}

		data, err := MarshalHandshakeResponse(resp)
		require.NoError(t, err)

		parsed, err := UnmarshalHandshakeResponse(data)
		require.NoError(t, err)

		assert.Equal(t, resp.SessionID, parsed.SessionID)
		assert.Equal(t, resp.ServerVersion, parsed.ServerVersion)
		assert.Equal(t, resp.EnabledFeatures, parsed.EnabledFeatures)
		assert.Equal(t, resp.HeartbeatInterval, parsed.HeartbeatInterval)
		assert.Equal(t, resp.Status, parsed.Status)
	})
}

func TestSessionManager(t *testing.T) {
	logger := zap.NewNop()

	t.Run("create new session", func(t *testing.T) {
		sm := NewSessionManager(logger)
		req := &HandshakeRequest{
			ClientVersion:     "1.0.0",
			SupportedFeatures: []string{"ping", "pong", "rooms"},
			Platform:          "web",
			DeviceID:          "device-123",
		}

		resp, err := sm.HandleHandshake("user-123", req)
		require.NoError(t, err)

		assert.NotEmpty(t, resp.SessionID)
		assert.Equal(t, "accepted", resp.Status)
		assert.Equal(t, "1.0.0", resp.ServerVersion)
		assert.Greater(t, len(resp.EnabledFeatures), 0)

		// Verify session was created
		session, err := sm.GetSession(resp.SessionID)
		require.NoError(t, err)
		assert.Equal(t, "user-123", session.UserID)
		assert.Equal(t, StateConnected, session.State)
	})

	t.Run("reconnect to existing session", func(t *testing.T) {
		sm := NewSessionManager(logger)

		// Create initial session
		req1 := &HandshakeRequest{
			ClientVersion:     "1.0.0",
			SupportedFeatures: []string{"ping", "pong"},
			LastAckSeq:        50,
		}
		resp1, err := sm.HandleHandshake("user-123", req1)
		require.NoError(t, err)

		sessionID := resp1.SessionID

		// Update last ack seq
		err = sm.UpdateLastAckSeq(sessionID, 100)
		require.NoError(t, err)

		// Simulate disconnection
		err = sm.CloseSession(sessionID)
		require.NoError(t, err)

		// Reconnect with session ID
		req2 := &HandshakeRequest{
			ClientVersion:     "1.0.0",
			SupportedFeatures: []string{"ping", "pong"},
			SessionID:         sessionID,
		}
		resp2, err := sm.HandleHandshake("user-123", req2)
		require.NoError(t, err)

		assert.Equal(t, sessionID, resp2.SessionID)
		assert.Equal(t, int64(100), resp2.ResumeFromSeq)
		assert.Contains(t, resp2.Message, "Reconnected")
	})

	t.Run("reject invalid session ID", func(t *testing.T) {
		sm := NewSessionManager(logger)

		// Try to reconnect with invalid session ID
		req := &HandshakeRequest{
			ClientVersion:     "1.0.0",
			SupportedFeatures: []string{"ping"},
			SessionID:         "invalid-session-id",
		}
		resp, err := sm.HandleHandshake("user-123", req)
		require.NoError(t, err)

		// Should create new session instead
		assert.NotEqual(t, "invalid-session-id", resp.SessionID)
		assert.Equal(t, int64(0), resp.ResumeFromSeq)
	})

	t.Run("feature negotiation", func(t *testing.T) {
		sm := NewSessionManager(logger)

		// Client supports a subset of server features
		req := &HandshakeRequest{
			ClientVersion:     "1.0.0",
			SupportedFeatures: []string{"ping", "pong", "unknown-feature"},
		}
		resp, err := sm.HandleHandshake("user-123", req)
		require.NoError(t, err)

		// Should only enable features supported by both
		assert.Contains(t, resp.EnabledFeatures, "ping")
		assert.Contains(t, resp.EnabledFeatures, "pong")
		assert.NotContains(t, resp.EnabledFeatures, "unknown-feature")
	})

	t.Run("heartbeat tracking", func(t *testing.T) {
		sm := NewSessionManager(logger)

		req := &HandshakeRequest{
			ClientVersion: "1.0.0",
		}
		resp, err := sm.HandleHandshake("user-123", req)
		require.NoError(t, err)

		sessionID := resp.SessionID

		// Record heartbeat
		err = sm.RecordHeartbeat(sessionID)
		require.NoError(t, err)

		session, err := sm.GetSession(sessionID)
		require.NoError(t, err)
		assert.Equal(t, StateConnected, session.State)
		assert.Equal(t, 0, session.HeartbeatMissCount)

		// Record missed heartbeat
		err = sm.RecordMissedHeartbeat(sessionID)
		require.NoError(t, err)

		session, err = sm.GetSession(sessionID)
		require.NoError(t, err)
		assert.Equal(t, 1, session.HeartbeatMissCount)

		// Record second missed heartbeat
		err = sm.RecordMissedHeartbeat(sessionID)
		require.NoError(t, err)

		session, err = sm.GetSession(sessionID)
		require.NoError(t, err)
		assert.Equal(t, StateStale, session.State)
		assert.Equal(t, 2, session.HeartbeatMissCount)

		// Recover with heartbeat
		err = sm.RecordHeartbeat(sessionID)
		require.NoError(t, err)

		session, err = sm.GetSession(sessionID)
		require.NoError(t, err)
		assert.Equal(t, StateConnected, session.State)
		assert.Equal(t, 0, session.HeartbeatMissCount)
	})

	t.Run("user sessions tracking", func(t *testing.T) {
		sm := NewSessionManager(logger)

		// Create multiple sessions for same user
		for i := 0; i < 3; i++ {
			req := &HandshakeRequest{
				ClientVersion: "1.0.0",
			}
			resp, err := sm.HandleHandshake("user-123", req)
			require.NoError(t, err)
			assert.NotEmpty(t, resp.SessionID)
		}

		sessions := sm.GetUserSessions("user-123")
		assert.Equal(t, 3, len(sessions))

		// All sessions should belong to same user
		for _, session := range sessions {
			assert.Equal(t, "user-123", session.UserID)
		}
	})

	t.Run("cleanup stale sessions", func(t *testing.T) {
		sm := NewSessionManager(logger)
		sm.staleSessionTimeout = 100 * time.Millisecond

		// Create session and mark as disconnected
		req := &HandshakeRequest{
			ClientVersion: "1.0.0",
		}
		resp, err := sm.HandleHandshake("user-123", req)
		require.NoError(t, err)

		sessionID := resp.SessionID

		// Close session
		err = sm.CloseSession(sessionID)
		require.NoError(t, err)

		// Manually update last activity to be old
		sm.mu.Lock()
		session := sm.sessions[sessionID]
		session.LastActivity = time.Now().Add(-1 * time.Minute)
		sm.mu.Unlock()

		// Run cleanup
		count := sm.CleanupStaleSessions()
		assert.Equal(t, 1, count)

		// Session should be removed
		_, err = sm.GetSession(sessionID)
		assert.Error(t, err)
	})

	t.Run("session stats", func(t *testing.T) {
		sm := NewSessionManager(logger)

		// Create sessions with different states
		req := &HandshakeRequest{
			ClientVersion: "1.0.0",
		}

		// Create connected session
		_, err := sm.HandleHandshake("user-1", req)
		require.NoError(t, err)

		// Create stale session
		resp2, err := sm.HandleHandshake("user-2", req)
		require.NoError(t, err)
		err = sm.RecordMissedHeartbeat(resp2.SessionID)
		require.NoError(t, err)
		err = sm.RecordMissedHeartbeat(resp2.SessionID)
		require.NoError(t, err)

		// Create disconnected session
		resp3, err := sm.HandleHandshake("user-3", req)
		require.NoError(t, err)
		err = sm.CloseSession(resp3.SessionID)
		require.NoError(t, err)

		stats := sm.GetSessionStats()
		assert.Equal(t, 3, stats["total_sessions"])
		assert.Equal(t, 3, stats["unique_users"])
		assert.Equal(t, 1, stats["connected"])
		assert.Equal(t, 1, stats["stale"])
		assert.Equal(t, 1, stats["disconnected"])
	})

	t.Run("update last ack seq", func(t *testing.T) {
		sm := NewSessionManager(logger)

		req := &HandshakeRequest{
			ClientVersion: "1.0.0",
		}
		resp, err := sm.HandleHandshake("user-123", req)
		require.NoError(t, err)

		// Update sequence number
		err = sm.UpdateLastAckSeq(resp.SessionID, 500)
		require.NoError(t, err)

		session, err := sm.GetSession(resp.SessionID)
		require.NoError(t, err)
		assert.Equal(t, int64(500), session.LastAckSeq)
	})
}

func TestConnectionState(t *testing.T) {
	t.Run("connection state transitions", func(t *testing.T) {
		states := []ConnectionState{
			StateConnecting,
			StateHandshaking,
			StateConnected,
			StateStale,
			StateReconnecting,
			StateDisconnected,
		}

		for _, state := range states {
			assert.NotEmpty(t, string(state))
		}
	})
}
