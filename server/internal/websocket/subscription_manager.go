package websocket

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// SubscriptionManager manages room subscriptions for clients
type SubscriptionManager struct {
	hub         *Hub
	roomManager *RoomManager
	topology    *RoomTopology
	bridge      *RedisBridge
	logger      *zap.Logger

	// Track room membership for ACK
	memberships map[string]map[string]bool // roomID -> clientID -> bool
	mu          sync.RWMutex
}

// SubscriptionConfig contains configuration for subscription manager
type SubscriptionConfig struct {
	Hub         *Hub
	RoomManager *RoomManager
	Bridge      *RedisBridge
	Logger      *zap.Logger
}

// NewSubscriptionManager creates a new subscription manager
func NewSubscriptionManager(config SubscriptionConfig) *SubscriptionManager {
	return &SubscriptionManager{
		hub:         config.Hub,
		roomManager: config.RoomManager,
		topology:    NewRoomTopology(),
		bridge:      config.Bridge,
		logger:      config.Logger,
		memberships: make(map[string]map[string]bool),
	}
}

// JoinRoomRequest represents a request to join a room
type JoinRoomRequest struct {
	RoomID   string
	UserID   string
	TeamIDs  []string
	DeviceID string
}

// JoinRoomResponse represents the response to a join request
type JoinRoomResponse struct {
	Success   bool                   `json:"success"`
	RoomID    string                 `json:"room_id"`
	Error     string                 `json:"error,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// JoinRoom handles a client joining a room with authorization
func (sm *SubscriptionManager) JoinRoom(ctx context.Context, client *Client, req JoinRoomRequest) (*JoinRoomResponse, error) {
	// Validate room access
	if !sm.topology.ValidateRoomAccess(req.UserID, req.TeamIDs, req.RoomID) {
		sm.logger.Warn("Unauthorized room access attempt",
			zap.String("user_id", req.UserID),
			zap.String("room_id", req.RoomID),
		)
		return &JoinRoomResponse{
			Success:   false,
			RoomID:    req.RoomID,
			Error:     "Unauthorized: You do not have access to this room",
			Timestamp: time.Now(),
		}, fmt.Errorf("unauthorized room access")
	}

	// Get or create room
	room, exists := sm.roomManager.GetRoom(req.RoomID)
	if !exists {
		// Parse room key to set ownership
		roomKey, err := ParseRoomKey(req.RoomID)
		if err != nil {
			return &JoinRoomResponse{
				Success:   false,
				RoomID:    req.RoomID,
				Error:     "Invalid room ID format",
				Timestamp: time.Now(),
			}, err
		}

		// Create room with appropriate owner
		owner := req.UserID
		if roomKey.Type == RoomTypeTeam {
			owner = roomKey.TeamID
		}

		room, err = sm.roomManager.CreateRoom(req.RoomID, req.RoomID, owner)
		if err != nil {
			return &JoinRoomResponse{
				Success:   false,
				RoomID:    req.RoomID,
				Error:     "Failed to create room",
				Timestamp: time.Now(),
			}, err
		}

		// Subscribe to Redis channel for this room
		if sm.bridge != nil {
			if err := sm.bridge.SubscribeToRoom(req.RoomID); err != nil {
				sm.logger.Error("Failed to subscribe to Redis channel",
					zap.Error(err),
					zap.String("room_id", req.RoomID),
				)
				// Continue anyway - local room will work
			}
		}
	}

	// Add client to room using goroutine to avoid blocking
	go func() {
		room.Join <- client
	}()

	// Track membership
	sm.mu.Lock()
	if sm.memberships[req.RoomID] == nil {
		sm.memberships[req.RoomID] = make(map[string]bool)
	}
	sm.memberships[req.RoomID][client.userID] = true
	memberCount := len(sm.memberships[req.RoomID])
	sm.mu.Unlock()

	sm.logger.Info("Client joined room",
		zap.String("user_id", client.userID),
		zap.String("room_id", req.RoomID),
		zap.Int("member_count", memberCount),
	)

	// Return success response
	return &JoinRoomResponse{
		Success: true,
		RoomID:  req.RoomID,
		Metadata: map[string]interface{}{
			"member_count": memberCount,
			"room_type":    sm.getRoomType(req.RoomID),
		},
		Timestamp: time.Now(),
	}, nil
}

// LeaveRoom handles a client leaving a room
func (sm *SubscriptionManager) LeaveRoom(ctx context.Context, client *Client, roomID string) error {
	// Get room
	room, exists := sm.roomManager.GetRoom(roomID)
	if !exists {
		return fmt.Errorf("room not found: %s", roomID)
	}

	// Remove client from room using goroutine to avoid blocking
	go func() {
		room.Leave <- client
	}()

	// Update membership tracking
	sm.mu.Lock()
	if sm.memberships[roomID] != nil {
		delete(sm.memberships[roomID], client.userID)

		// Clean up empty membership maps
		if len(sm.memberships[roomID]) == 0 {
			delete(sm.memberships, roomID)

			// Unsubscribe from Redis if no local members
			if sm.bridge != nil {
				go sm.bridge.UnsubscribeFromRoom(roomID)
			}
		}
	}
	sm.mu.Unlock()

	sm.logger.Info("Client left room",
		zap.String("user_id", client.userID),
		zap.String("room_id", roomID),
	)

	return nil
}

// SubscribeUserToRooms subscribes a user to all their relevant rooms
func (sm *SubscriptionManager) SubscribeUserToRooms(ctx context.Context, client *Client, userID string, teamIDs []string, deviceID string) error {
	rooms := sm.topology.GetUserRooms(userID, teamIDs, deviceID)

	sm.logger.Info("Subscribing user to rooms",
		zap.String("user_id", userID),
		zap.Strings("rooms", rooms),
	)

	for _, roomID := range rooms {
		req := JoinRoomRequest{
			RoomID:   roomID,
			UserID:   userID,
			TeamIDs:  teamIDs,
			DeviceID: deviceID,
		}

		_, err := sm.JoinRoom(ctx, client, req)
		if err != nil {
			sm.logger.Error("Failed to join room during subscription",
				zap.Error(err),
				zap.String("room_id", roomID),
			)
			// Continue with other rooms
		}
	}

	return nil
}

// UnsubscribeUserFromRooms unsubscribes a user from all rooms
func (sm *SubscriptionManager) UnsubscribeUserFromRooms(ctx context.Context, client *Client) error {
	// Get all rooms the client is in
	roomIDs := client.GetRooms()

	sm.logger.Info("Unsubscribing user from rooms",
		zap.String("user_id", client.userID),
		zap.Int("room_count", len(roomIDs)),
	)

	for _, roomID := range roomIDs {
		if err := sm.LeaveRoom(ctx, client, roomID); err != nil {
			sm.logger.Error("Failed to leave room during unsubscribe",
				zap.Error(err),
				zap.String("room_id", roomID),
			)
		}
	}

	return nil
}

// BroadcastToRoom broadcasts a message to a room (locally and via Redis)
func (sm *SubscriptionManager) BroadcastToRoom(ctx context.Context, roomID string, message *Message) error {
	// Publish via Redis for multi-node fanout
	if sm.bridge != nil {
		if err := sm.bridge.PublishToRoom(ctx, roomID, message); err != nil {
			sm.logger.Error("Failed to publish to Redis",
				zap.Error(err),
				zap.String("room_id", roomID),
			)
			// Continue to local broadcast as fallback
		}
	}

	// Also broadcast locally for immediate delivery
	room, exists := sm.roomManager.GetRoom(roomID)
	if exists {
		data, err := message.Marshal()
		if err != nil {
			return err
		}
		room.Broadcast <- data
	}

	return nil
}

// GetRoomMembers returns the list of members in a room
func (sm *SubscriptionManager) GetRoomMembers(roomID string) []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if members, exists := sm.memberships[roomID]; exists {
		memberList := make([]string, 0, len(members))
		for userID := range members {
			memberList = append(memberList, userID)
		}
		return memberList
	}

	return []string{}
}

// GetRoomMemberCount returns the number of members in a room
func (sm *SubscriptionManager) GetRoomMemberCount(roomID string) int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if members, exists := sm.memberships[roomID]; exists {
		return len(members)
	}
	return 0
}

// GetUserSubscriptions returns all rooms a user is subscribed to
func (sm *SubscriptionManager) GetUserSubscriptions(userID string) []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	rooms := []string{}
	for roomID, members := range sm.memberships {
		if members[userID] {
			rooms = append(rooms, roomID)
		}
	}
	return rooms
}

// getRoomType returns the type of room based on room ID
func (sm *SubscriptionManager) getRoomType(roomID string) string {
	roomKey, err := ParseRoomKey(roomID)
	if err != nil {
		return "unknown"
	}
	return string(roomKey.Type)
}

// GetStats returns statistics about subscriptions
func (sm *SubscriptionManager) GetStats() map[string]interface{} {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	totalMembers := 0
	for _, members := range sm.memberships {
		totalMembers += len(members)
	}

	return map[string]interface{}{
		"total_rooms":   len(sm.memberships),
		"total_members": totalMembers,
	}
}
