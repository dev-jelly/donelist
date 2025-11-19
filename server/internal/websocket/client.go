package websocket

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dev-jelly/donelist/internal/auth"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period (must be less than pongWait)
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer
	maxMessageSize = 512
)

// Upgrader is used to upgrade HTTP connections to WebSocket connections
var Upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for now (should be restricted in production)
		return true
	},
}

// Client represents a single WebSocket connection
type Client struct {
	// The WebSocket connection
	conn *websocket.Conn

	// Hub that manages this client
	hub *Hub

	// Buffered channel of outbound messages
	Send chan []byte

	// User ID associated with this client
	userID   string
	username string

	// Session ID for this connection
	sessionID string

	// Rooms this client is part of
	rooms map[string]*Room

	// Presence information
	status    string // online, away, offline
	lastSeen  time.Time
	isTyping  bool
	typingIn  string // room ID where user is typing

	// Health check
	healthCheck *HealthCheck

	// Logger
	logger *zap.Logger

	// Mutex for thread-safe operations
	mu sync.RWMutex
}

// NewClient creates a new client
func NewClient(conn *websocket.Conn, hub *Hub, userID, username string, logger *zap.Logger) *Client {
	return &Client{
		conn:     conn,
		hub:      hub,
		Send:     make(chan []byte, 256),
		userID:   userID,
		username: username,
		rooms:    make(map[string]*Room),
		status:   "online",
		lastSeen: time.Now(),
		logger:   logger,
	}
}

// AuthenticateWebSocket authenticates a WebSocket connection using JWT from query params
func AuthenticateWebSocket(r *http.Request, jwtManager *auth.JWTManager) (string, string, error) {
	// Get token from query parameter
	token := r.URL.Query().Get("token")
	if token == "" {
		// Try to get from Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && parts[0] == "Bearer" {
				token = parts[1]
			}
		}
	}

	if token == "" {
		return "", "", fmt.Errorf("no authentication token provided")
	}

	// Validate token
	claims, err := jwtManager.ValidateAccessToken(token)
	if err != nil {
		return "", "", fmt.Errorf("invalid token: %w", err)
	}

	// Get username from claims
	username := claims.Email
	if username == "" {
		username = claims.UserID.String()
	}

	return claims.UserID.String(), username, nil
}

// readPump pumps messages from the WebSocket connection to the hub
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))

		// Notify health check if available
		if c.healthCheck != nil {
			c.healthCheck.OnPongReceived()
		}

		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.logger.Error("WebSocket read error",
					zap.Error(err),
					zap.String("user_id", c.userID),
				)
			}
			break
		}

		// Parse incoming message
		msg, err := FromJSON(message)
		if err != nil {
			c.logger.Warn("Failed to parse WebSocket message",
				zap.Error(err),
				zap.String("user_id", c.userID),
			)
			continue
		}

		// Handle different message types
		switch msg.Type {
		case MessageTypePing:
			pong := NewMessage(MessageTypePong, nil)
			if data, err := json.Marshal(pong); err == nil {
				c.Send <- data
			}
			continue
		case MessageTypeHandshake:
			c.handleHandshake(message)
		case "join_room":
			c.handleJoinRoom(msg)
		case "leave_room":
			c.handleLeaveRoom(msg)
		case "room_message":
			c.handleRoomMessage(msg)
		case "typing_start":
			c.handleTypingStart(msg)
		case "typing_stop":
			c.handleTypingStop(msg)
		case "update_status":
			c.handleStatusUpdate(msg)
		default:
			c.logger.Debug("Received WebSocket message",
				zap.String("type", string(msg.Type)),
				zap.String("user_id", c.userID),
			)
		}
	}
}

// writePump pumps messages from the hub to the WebSocket connection
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// Write message to WebSocket
			err := c.conn.WriteMessage(websocket.TextMessage, message)
			if err != nil {
				c.logger.Error("Failed to write message",
					zap.Error(err),
					zap.String("user_id", c.userID),
				)
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// Start starts the client's read and write pumps
func (c *Client) Start() {
	go c.writePump()
	go c.readPump()
}

// Message handler methods

func (c *Client) handleHandshake(rawMessage []byte) {
	// Parse handshake request
	req, err := UnmarshalHandshakeRequest(rawMessage)
	if err != nil {
		c.logger.Error("Failed to parse handshake request",
			zap.Error(err),
			zap.String("user_id", c.userID),
		)
		c.sendError("Invalid handshake request")
		return
	}

	// Get session manager from hub (we'll need to add this)
	if c.hub.sessionManager == nil {
		c.logger.Error("Session manager not available",
			zap.String("user_id", c.userID),
		)
		c.sendError("Session manager not available")
		return
	}

	// Process handshake
	resp, err := c.hub.sessionManager.HandleHandshake(c.userID, req)
	if err != nil {
		c.logger.Error("Handshake failed",
			zap.Error(err),
			zap.String("user_id", c.userID),
		)
		c.sendError("Handshake failed")
		return
	}

	// Store session ID
	c.mu.Lock()
	c.sessionID = resp.SessionID
	c.mu.Unlock()

	// Send handshake response
	respData, err := MarshalHandshakeResponse(resp)
	if err != nil {
		c.logger.Error("Failed to marshal handshake response",
			zap.Error(err),
			zap.String("user_id", c.userID),
		)
		return
	}

	c.Send <- respData

	c.logger.Info("Handshake completed",
		zap.String("user_id", c.userID),
		zap.String("session_id", resp.SessionID),
		zap.String("client_version", req.ClientVersion),
	)
}

func (c *Client) handleJoinRoom(msg *Message) {
	roomID, ok := msg.Payload["room_id"].(string)
	if !ok {
		c.sendError("Invalid room_id")
		return
	}

	// Get room from hub's room manager
	if c.hub.roomManager != nil {
		room, exists := c.hub.roomManager.GetRoom(roomID)
		if !exists {
			c.sendError("Room not found")
			return
		}

		// Join the room
		room.Join <- c

		// Send confirmation
		response := Message{
			Type:      "joined_room",
			RoomID:    roomID,
			UserID:    c.userID,
			Timestamp: time.Now(),
			Payload: map[string]interface{}{
				"room_id": roomID,
				"success": true,
			},
		}
		if data, err := response.Marshal(); err == nil {
			c.Send <- data
		}
	}
}

func (c *Client) handleLeaveRoom(msg *Message) {
	roomID, ok := msg.Payload["room_id"].(string)
	if !ok {
		c.sendError("Invalid room_id")
		return
	}

	// Check if client is in the room
	c.mu.RLock()
	room, inRoom := c.rooms[roomID]
	c.mu.RUnlock()

	if !inRoom {
		c.sendError("Not in room")
		return
	}

	// Leave the room
	room.Leave <- c

	// Send confirmation
	response := Message{
		Type:      "left_room",
		RoomID:    roomID,
		UserID:    c.userID,
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"room_id": roomID,
			"success": true,
		},
	}
	if data, err := response.Marshal(); err == nil {
		c.Send <- data
	}
}

func (c *Client) handleRoomMessage(msg *Message) {
	roomID, ok := msg.Payload["room_id"].(string)
	if !ok {
		c.sendError("Invalid room_id")
		return
	}

	content, ok := msg.Payload["content"].(string)
	if !ok {
		c.sendError("Invalid message content")
		return
	}

	// Check if client is in the room
	c.mu.RLock()
	room, inRoom := c.rooms[roomID]
	c.mu.RUnlock()

	if !inRoom {
		c.sendError("Not in room")
		return
	}

	// Create room message
	roomMsg := Message{
		Type:      "room_message",
		RoomID:    roomID,
		UserID:    c.userID,
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"content":  content,
			"username": c.username,
			"user_id":  c.userID,
		},
	}

	// Broadcast to room
	if data, err := roomMsg.Marshal(); err == nil {
		room.Broadcast <- data
	}
}

func (c *Client) handleTypingStart(msg *Message) {
	roomID, ok := msg.Payload["room_id"].(string)
	if !ok {
		return
	}

	c.mu.Lock()
	c.isTyping = true
	c.typingIn = roomID
	c.mu.Unlock()

	// Notify room members
	c.notifyTypingStatus(roomID, true)
}

func (c *Client) handleTypingStop(msg *Message) {
	roomID, ok := msg.Payload["room_id"].(string)
	if !ok {
		return
	}

	c.mu.Lock()
	c.isTyping = false
	c.typingIn = ""
	c.mu.Unlock()

	// Notify room members
	c.notifyTypingStatus(roomID, false)
}

func (c *Client) handleStatusUpdate(msg *Message) {
	status, ok := msg.Payload["status"].(string)
	if !ok {
		return
	}

	// Validate status
	if status != "online" && status != "away" && status != "offline" {
		c.sendError("Invalid status")
		return
	}

	c.mu.Lock()
	c.status = status
	c.lastSeen = time.Now()
	c.mu.Unlock()

	// Broadcast status to all rooms
	c.broadcastPresence()
}

func (c *Client) notifyTypingStatus(roomID string, isTyping bool) {
	c.mu.RLock()
	room, exists := c.rooms[roomID]
	c.mu.RUnlock()

	if !exists {
		return
	}

	// Create typing notification
	notification := Message{
		Type:      "typing_status",
		RoomID:    roomID,
		UserID:    c.userID,
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"user_id":   c.userID,
			"username":  c.username,
			"is_typing": isTyping,
			"room_id":   roomID,
		},
	}

	if data, err := notification.Marshal(); err == nil {
		room.broadcastToClientsExcept(data, c)
	}
}

func (c *Client) broadcastPresence() {
	c.mu.RLock()
	status := c.status
	lastSeen := c.lastSeen
	rooms := make([]*Room, 0, len(c.rooms))
	for _, room := range c.rooms {
		rooms = append(rooms, room)
	}
	c.mu.RUnlock()

	// Create presence update
	presence := Message{
		Type:      "presence_update",
		UserID:    c.userID,
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"user_id":   c.userID,
			"username":  c.username,
			"status":    status,
			"last_seen": lastSeen.Unix(),
		},
	}

	data, err := presence.Marshal()
	if err != nil {
		return
	}

	// Broadcast to all rooms
	for _, room := range rooms {
		room.broadcastToClientsExcept(data, c)
	}
}

func (c *Client) sendError(message string) {
	errMsg := Message{
		Type:      "error",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"message": message,
		},
	}

	if data, err := errMsg.Marshal(); err == nil {
		c.Send <- data
	}
}

// GetStatus returns the client's current status
func (c *Client) GetStatus() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}

// GetRooms returns the list of rooms the client is in
func (c *Client) GetRooms() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	roomIDs := make([]string, 0, len(c.rooms))
	for id := range c.rooms {
		roomIDs = append(roomIDs, id)
	}
	return roomIDs
}

// GetSessionID returns the client's session ID
func (c *Client) GetSessionID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sessionID
}

// SetHealthCheck sets the health check for this client
func (c *Client) SetHealthCheck(hc *HealthCheck) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.healthCheck = hc
}
