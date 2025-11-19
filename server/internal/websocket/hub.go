package websocket

import (
	"sync"

	"go.uber.org/zap"
)

// Hub maintains the set of active clients and broadcasts messages to the clients
type Hub struct {
	// Registered clients
	clients map[*Client]bool

	// Inbound messages from the clients
	broadcast chan *Message

	// Register requests from the clients
	register chan *Client

	// Unregister requests from clients
	unregister chan *Client

	// Room manager
	roomManager *RoomManager

	// Session manager
	sessionManager *SessionManager

	// Global sequence manager for user-level messages
	seqManager *SequenceManager

	// Mutex for thread-safe client operations
	mu sync.RWMutex

	// Logger
	logger *zap.Logger
}

// NewHub creates a new Hub
func NewHub(logger *zap.Logger) *Hub {
	roomManager := NewRoomManager()
	go roomManager.Run()

	sessionManager := NewSessionManager(logger)
	seqManager := NewSequenceManager()

	return &Hub{
		clients:        make(map[*Client]bool),
		broadcast:      make(chan *Message, 256),
		register:       make(chan *Client),
		unregister:     make(chan *Client),
		roomManager:    roomManager,
		sessionManager: sessionManager,
		seqManager:     seqManager,
		logger:         logger,
	}
}

// Register adds a client to the hub
func (h *Hub) Register(client *Client) {
	h.register <- client
}

// Unregister removes a client from the hub
func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

// Run starts the hub's main loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			h.logger.Info("Client registered",
				zap.String("user_id", client.userID),
				zap.Int("total_clients", len(h.clients)),
			)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.Send)
				h.logger.Info("Client unregistered",
					zap.String("user_id", client.userID),
					zap.Int("total_clients", len(h.clients)),
				)
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				// Only send messages to the target user
				if message.UserID == "" || client.userID == message.UserID {
					data, err := message.Marshal()
					if err != nil {
						h.logger.Error("Failed to marshal message", zap.Error(err))
						continue
					}
					select {
					case client.Send <- data:
					default:
						// Client's send channel is full, close it
						h.mu.RUnlock()
						h.mu.Lock()
						close(client.Send)
						delete(h.clients, client)
						h.mu.Unlock()
						h.mu.RLock()
						h.logger.Warn("Client send buffer full, closing connection",
							zap.String("user_id", client.userID),
						)
					}
				}
			}
			h.mu.RUnlock()
		}
	}
}

// BroadcastToUser sends a message to a specific user
func (h *Hub) BroadcastToUser(userID string, message *Message) {
	message.UserID = userID
	h.broadcast <- message
}

// BroadcastToAll sends a message to all connected clients
func (h *Hub) BroadcastToAll(message *Message) {
	message.UserID = "" // Empty string means broadcast to all
	h.broadcast <- message
}

// GetClientCount returns the number of connected clients
func (h *Hub) GetClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// GetUserClientCount returns the number of connections for a specific user
func (h *Hub) GetUserClientCount(userID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	count := 0
	for client := range h.clients {
		if client.userID == userID {
			count++
		}
	}
	return count
}

// GetRoomManager returns the room manager
func (h *Hub) GetRoomManager() *RoomManager {
	return h.roomManager
}

// GetSessionManager returns the session manager
func (h *Hub) GetSessionManager() *SessionManager {
	return h.sessionManager
}

// GetSequenceManager returns the global sequence manager
func (h *Hub) GetSequenceManager() *SequenceManager {
	return h.seqManager
}

// BroadcastToUserWithSequence sends a message to a specific user with sequence number
func (h *Hub) BroadcastToUserWithSequence(userID string, message *Message) {
	// Allocate sequence number for this user
	seq := h.seqManager.NextSequence(userID)
	message.Sequence = seq
	message.UserID = userID
	h.broadcast <- message
}
