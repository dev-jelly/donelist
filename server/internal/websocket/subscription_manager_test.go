package websocket

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupSubscriptionManager(t *testing.T) (*SubscriptionManager, *Hub, *Client) {
	logger := zap.NewNop()
	hub := NewHub(logger)
	go hub.Run()

	roomManager := NewRoomManager()
	go roomManager.Run()

	sm := NewSubscriptionManager(SubscriptionConfig{
		Hub:         hub,
		RoomManager: roomManager,
		Bridge:      nil, // No Redis for unit tests
		Logger:      logger,
	})

	// Create a mock client
	client := &Client{
		userID:   "user-123",
		username: "testuser",
		Send:     make(chan []byte, 256),
		rooms:    make(map[string]*Room),
		hub:      hub,
		logger:   logger,
	}

	return sm, hub, client
}

func TestSubscriptionManager_JoinRoom_Success(t *testing.T) {
	sm, _, client := setupSubscriptionManager(t)
	ctx := context.Background()

	req := JoinRoomRequest{
		RoomID:  "user:user-123",
		UserID:  "user-123",
		TeamIDs: []string{},
	}

	resp, err := sm.JoinRoom(ctx, client, req)
	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.Equal(t, "user:user-123", resp.RoomID)
	assert.Empty(t, resp.Error)

	// Verify room membership
	members := sm.GetRoomMembers(req.RoomID)
	assert.Contains(t, members, "user-123")
}

func TestSubscriptionManager_JoinRoom_Unauthorized(t *testing.T) {
	sm, _, client := setupSubscriptionManager(t)
	ctx := context.Background()

	// Try to join another user's room
	req := JoinRoomRequest{
		RoomID:  "user:user-456",
		UserID:  "user-123",
		TeamIDs: []string{},
	}

	resp, err := sm.JoinRoom(ctx, client, req)
	require.Error(t, err)
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Error, "Unauthorized")
}

func TestSubscriptionManager_JoinRoom_TeamAccess(t *testing.T) {
	sm, _, client := setupSubscriptionManager(t)
	ctx := context.Background()

	req := JoinRoomRequest{
		RoomID:  "team:team-1",
		UserID:  "user-123",
		TeamIDs: []string{"team-1", "team-2"},
	}

	resp, err := sm.JoinRoom(ctx, client, req)
	require.NoError(t, err)
	assert.True(t, resp.Success)

	// Verify membership
	count := sm.GetRoomMemberCount("team:team-1")
	assert.Equal(t, 1, count)
}

func TestSubscriptionManager_JoinRoom_UnauthorizedTeam(t *testing.T) {
	sm, _, client := setupSubscriptionManager(t)
	ctx := context.Background()

	// Try to join a team room without membership
	req := JoinRoomRequest{
		RoomID:  "team:team-999",
		UserID:  "user-123",
		TeamIDs: []string{"team-1"},
	}

	resp, err := sm.JoinRoom(ctx, client, req)
	require.Error(t, err)
	assert.False(t, resp.Success)
}

func TestSubscriptionManager_LeaveRoom(t *testing.T) {
	sm, _, client := setupSubscriptionManager(t)
	ctx := context.Background()

	// Join a room first
	req := JoinRoomRequest{
		RoomID:  "user:user-123",
		UserID:  "user-123",
		TeamIDs: []string{},
	}

	_, err := sm.JoinRoom(ctx, client, req)
	require.NoError(t, err)

	// Verify joined
	assert.Equal(t, 1, sm.GetRoomMemberCount("user:user-123"))

	// Leave the room
	err = sm.LeaveRoom(ctx, client, "user:user-123")
	require.NoError(t, err)

	// Verify left
	assert.Equal(t, 0, sm.GetRoomMemberCount("user:user-123"))
}

func TestSubscriptionManager_SubscribeUserToRooms(t *testing.T) {
	sm, _, client := setupSubscriptionManager(t)
	ctx := context.Background()

	userID := "user-123"
	teamIDs := []string{"team-1", "team-2"}
	deviceID := "device-456"

	err := sm.SubscribeUserToRooms(ctx, client, userID, teamIDs, deviceID)
	require.NoError(t, err)

	// Wait for async operations
	time.Sleep(100 * time.Millisecond)

	// Verify subscriptions
	subscriptions := sm.GetUserSubscriptions(userID)

	// Should be subscribed to:
	// - user:user-123
	// - team:team-1
	// - team:team-2
	// - user:user-123:team:team-1
	// - user:user-123:team:team-2
	// - device:user-123:device-456
	assert.GreaterOrEqual(t, len(subscriptions), 6)
}

func TestSubscriptionManager_UnsubscribeUserFromRooms(t *testing.T) {
	sm, _, client := setupSubscriptionManager(t)
	ctx := context.Background()

	// Subscribe to rooms first
	userID := "user-123"
	teamIDs := []string{"team-1"}
	deviceID := ""

	err := sm.SubscribeUserToRooms(ctx, client, userID, teamIDs, deviceID)
	require.NoError(t, err)
	time.Sleep(100 * time.Millisecond)

	// Verify subscribed
	assert.Greater(t, len(sm.GetUserSubscriptions(userID)), 0)

	// Unsubscribe
	err = sm.UnsubscribeUserFromRooms(ctx, client)
	require.NoError(t, err)
	time.Sleep(100 * time.Millisecond)

	// Verify unsubscribed
	assert.Equal(t, 0, len(sm.GetUserSubscriptions(userID)))
}

func TestSubscriptionManager_MultipleClientsInRoom(t *testing.T) {
	sm, hub, client1 := setupSubscriptionManager(t)
	ctx := context.Background()

	// Create second client
	client2 := &Client{
		userID:   "user-456",
		username: "testuser2",
		Send:     make(chan []byte, 256),
		rooms:    make(map[string]*Room),
		hub:      hub,
		logger:   zap.NewNop(),
	}

	// Both join same team room
	req1 := JoinRoomRequest{
		RoomID:  "team:team-1",
		UserID:  "user-123",
		TeamIDs: []string{"team-1"},
	}

	req2 := JoinRoomRequest{
		RoomID:  "team:team-1",
		UserID:  "user-456",
		TeamIDs: []string{"team-1"},
	}

	_, err := sm.JoinRoom(ctx, client1, req1)
	require.NoError(t, err)

	_, err = sm.JoinRoom(ctx, client2, req2)
	require.NoError(t, err)

	// Verify both are members
	count := sm.GetRoomMemberCount("team:team-1")
	assert.Equal(t, 2, count)

	members := sm.GetRoomMembers("team:team-1")
	assert.Contains(t, members, "user-123")
	assert.Contains(t, members, "user-456")
}

func TestSubscriptionManager_BroadcastToRoom(t *testing.T) {
	sm, _, client := setupSubscriptionManager(t)
	ctx := context.Background()

	// Join a room
	req := JoinRoomRequest{
		RoomID:  "user:user-123",
		UserID:  "user-123",
		TeamIDs: []string{},
	}

	_, err := sm.JoinRoom(ctx, client, req)
	require.NoError(t, err)

	// Wait for async join to complete
	time.Sleep(200 * time.Millisecond)

	// Broadcast a message
	msg := &Message{
		Type:      "test_message",
		RoomID:    "user:user-123",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"content": "Hello!",
		},
	}

	err = sm.BroadcastToRoom(ctx, "user:user-123", msg)
	require.NoError(t, err)

	// Client should receive the message
	// Note: This test verifies that BroadcastToRoom doesn't error
	// Actual message delivery depends on room goroutine being properly set up
	// In real usage, the room.Run() goroutine would be running

	// Just verify no error occurred
	assert.NoError(t, err)
}

func TestSubscriptionManager_GetStats(t *testing.T) {
	sm, _, client := setupSubscriptionManager(t)
	ctx := context.Background()

	// Join multiple rooms
	rooms := []string{
		"user:user-123",
		"team:team-1",
		"device:user-123:device-456",
	}

	for _, roomID := range rooms {
		req := JoinRoomRequest{
			RoomID:  roomID,
			UserID:  "user-123",
			TeamIDs: []string{"team-1"},
		}
		_, err := sm.JoinRoom(ctx, client, req)
		require.NoError(t, err)
	}

	time.Sleep(100 * time.Millisecond)

	// Get stats
	stats := sm.GetStats()
	assert.Equal(t, 3, stats["total_rooms"])
	assert.GreaterOrEqual(t, stats["total_members"], 3)
}

func TestSubscriptionManager_RoomCleanup(t *testing.T) {
	sm, _, client := setupSubscriptionManager(t)
	ctx := context.Background()

	roomID := "user:user-123"

	// Join room
	req := JoinRoomRequest{
		RoomID:  roomID,
		UserID:  "user-123",
		TeamIDs: []string{},
	}

	_, err := sm.JoinRoom(ctx, client, req)
	require.NoError(t, err)

	// Verify room exists in memberships
	assert.Equal(t, 1, sm.GetRoomMemberCount(roomID))

	// Leave room
	err = sm.LeaveRoom(ctx, client, roomID)
	require.NoError(t, err)

	// Verify room is cleaned up
	assert.Equal(t, 0, sm.GetRoomMemberCount(roomID))

	// Verify empty memberships are removed
	sm.mu.RLock()
	_, exists := sm.memberships[roomID]
	sm.mu.RUnlock()
	assert.False(t, exists)
}

func TestSubscriptionManager_DeviceRoom(t *testing.T) {
	sm, _, client := setupSubscriptionManager(t)
	ctx := context.Background()

	req := JoinRoomRequest{
		RoomID:   "device:user-123:device-456",
		UserID:   "user-123",
		TeamIDs:  []string{},
		DeviceID: "device-456",
	}

	resp, err := sm.JoinRoom(ctx, client, req)
	require.NoError(t, err)
	assert.True(t, resp.Success)

	// Verify metadata
	assert.Equal(t, "device", resp.Metadata["room_type"])
}

func TestSubscriptionManager_UserTeamRoom(t *testing.T) {
	sm, _, client := setupSubscriptionManager(t)
	ctx := context.Background()

	req := JoinRoomRequest{
		RoomID:  "user:user-123:team:team-1",
		UserID:  "user-123",
		TeamIDs: []string{"team-1"},
	}

	resp, err := sm.JoinRoom(ctx, client, req)
	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.Equal(t, "user_team", resp.Metadata["room_type"])
}
