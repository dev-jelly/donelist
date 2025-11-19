package websocket

import (
	"fmt"
	"sync"
	"time"
)

// Room represents a WebSocket room/channel
type Room struct {
	ID          string
	Name        string
	Owner       string // User ID of room owner
	Clients     map[*Client]bool
	Broadcast   chan []byte
	Join        chan *Client
	Leave       chan *Client
	Private     bool
	CreatedAt   time.Time
	MaxClients  int
	// Sequence manager for message ordering
	seqManager *SequenceManager
	mu         sync.RWMutex
}

// NewRoom creates a new room
func NewRoom(id, name, owner string) *Room {
	return &Room{
		ID:         id,
		Name:       name,
		Owner:      owner,
		Clients:    make(map[*Client]bool),
		Broadcast:  make(chan []byte, 256),
		Join:       make(chan *Client),
		Leave:      make(chan *Client),
		CreatedAt:  time.Now(),
		MaxClients: 100, // Default max clients per room
		seqManager: NewSequenceManager(),
	}
}

// NewRoomWithSequenceManager creates a new room with a shared sequence manager
func NewRoomWithSequenceManager(id, name, owner string, seqManager *SequenceManager) *Room {
	return &Room{
		ID:         id,
		Name:       name,
		Owner:      owner,
		Clients:    make(map[*Client]bool),
		Broadcast:  make(chan []byte, 256),
		Join:       make(chan *Client),
		Leave:      make(chan *Client),
		CreatedAt:  time.Now(),
		MaxClients: 100, // Default max clients per room
		seqManager: seqManager,
	}
}

// Run starts the room's event loop
func (r *Room) Run() {
	for {
		select {
		case client := <-r.Join:
			r.addClient(client)

		case client := <-r.Leave:
			r.removeClient(client)

		case message := <-r.Broadcast:
			r.broadcastToClients(message)
		}
	}
}

// addClient adds a client to the room
func (r *Room) addClient(client *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if room is full
	if len(r.Clients) >= r.MaxClients {
		client.Send <- []byte(`{"type":"error","payload":{"message":"Room is full"}}`)
		return
	}

	r.Clients[client] = true

	// Update client's rooms map with proper locking
	client.mu.Lock()
	client.rooms[r.ID] = r
	client.mu.Unlock()

	// Notify other clients
	notification := Message{
		Type:      "user_joined",
		RoomID:    r.ID,
		UserID:    client.userID,
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"user_id":  client.userID,
			"room_id":  r.ID,
			"username": client.username,
		},
	}

	if data, err := notification.Marshal(); err == nil {
		r.broadcastToClientsExcept(data, client)
	}
}

// removeClient removes a client from the room
func (r *Room) removeClient(client *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.Clients[client]; ok {
		delete(r.Clients, client)

		// Update client's rooms map with proper locking
		client.mu.Lock()
		delete(client.rooms, r.ID)
		client.mu.Unlock()

		// Notify other clients
		notification := Message{
			Type:      "user_left",
			RoomID:    r.ID,
			UserID:    client.userID,
			Timestamp: time.Now(),
			Payload: map[string]interface{}{
				"user_id":  client.userID,
				"room_id":  r.ID,
				"username": client.username,
			},
		}

		if data, err := notification.Marshal(); err == nil {
			r.broadcastToClientsExcept(data, client)
		}
	}
}

// broadcastToClients sends a message to all clients in the room
func (r *Room) broadcastToClients(message []byte) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for client := range r.Clients {
		select {
		case client.Send <- message:
		default:
			// Client's send channel is full, close it
			close(client.Send)
			delete(r.Clients, client)
		}
	}
}

// broadcastToClientsExcept sends a message to all clients except one
func (r *Room) broadcastToClientsExcept(message []byte, except *Client) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for client := range r.Clients {
		if client == except {
			continue
		}
		select {
		case client.Send <- message:
		default:
			// Client's send channel is full, close it
			close(client.Send)
			delete(r.Clients, client)
		}
	}
}

// GetClientCount returns the number of clients in the room
func (r *Room) GetClientCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.Clients)
}

// GetClientList returns a list of client IDs in the room
func (r *Room) GetClientList() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	clientIDs := make([]string, 0, len(r.Clients))
	for client := range r.Clients {
		clientIDs = append(clientIDs, client.userID)
	}
	return clientIDs
}

// IsUserInRoom checks if a user is in the room
func (r *Room) IsUserInRoom(userID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for client := range r.Clients {
		if client.userID == userID {
			return true
		}
	}
	return false
}

// RoomManager manages all WebSocket rooms
type RoomManager struct {
	rooms      map[string]*Room
	createRoom chan *Room
	deleteRoom chan string
	mu         sync.RWMutex
}

// NewRoomManager creates a new room manager
func NewRoomManager() *RoomManager {
	return &RoomManager{
		rooms:      make(map[string]*Room),
		createRoom: make(chan *Room),
		deleteRoom: make(chan string),
	}
}

// Run starts the room manager's event loop
func (rm *RoomManager) Run() {
	for {
		select {
		case room := <-rm.createRoom:
			rm.mu.Lock()
			rm.rooms[room.ID] = room
			rm.mu.Unlock()
			go room.Run()

		case roomID := <-rm.deleteRoom:
			rm.mu.Lock()
			if room, exists := rm.rooms[roomID]; exists {
				// Remove all clients from room
				for client := range room.Clients {
					delete(client.rooms, roomID)
				}
				delete(rm.rooms, roomID)
			}
			rm.mu.Unlock()
		}
	}
}

// CreateRoom creates a new room
func (rm *RoomManager) CreateRoom(id, name, owner string) (*Room, error) {
	rm.mu.RLock()
	if _, exists := rm.rooms[id]; exists {
		rm.mu.RUnlock()
		return nil, fmt.Errorf("room %s already exists", id)
	}
	rm.mu.RUnlock()

	room := NewRoom(id, name, owner)
	rm.createRoom <- room
	return room, nil
}

// GetRoom returns a room by ID
func (rm *RoomManager) GetRoom(id string) (*Room, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	room, exists := rm.rooms[id]
	return room, exists
}

// DeleteRoom deletes a room by ID
func (rm *RoomManager) DeleteRoom(id string) {
	rm.deleteRoom <- id
}

// ListRooms returns a list of all room IDs
func (rm *RoomManager) ListRooms() []string {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	roomIDs := make([]string, 0, len(rm.rooms))
	for id := range rm.rooms {
		roomIDs = append(roomIDs, id)
	}
	return roomIDs
}

// GetRoomInfo returns information about all rooms
func (rm *RoomManager) GetRoomInfo() []map[string]interface{} {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	info := make([]map[string]interface{}, 0, len(rm.rooms))
	for _, room := range rm.rooms {
		info = append(info, map[string]interface{}{
			"id":           room.ID,
			"name":         room.Name,
			"owner":        room.Owner,
			"client_count": room.GetClientCount(),
			"created_at":   room.CreatedAt,
			"private":      room.Private,
		})
	}
	return info
}

// BroadcastMessageWithSequence broadcasts a message to room with sequence number
func (r *Room) BroadcastMessageWithSequence(msg *Message) error {
	// Allocate sequence number for this room
	seq := r.seqManager.NextSequence(r.ID)
	msg.Sequence = seq

	// Marshal message
	data, err := msg.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Broadcast to all clients
	r.broadcastToClients(data)
	return nil
}

// GetSequenceManager returns the room's sequence manager
func (r *Room) GetSequenceManager() *SequenceManager {
	return r.seqManager
}