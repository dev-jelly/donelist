package websocket

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// MessageType represents the type of WebSocket message
type MessageType string

const (
	// MessageTypeCheckinCreated is sent when a check-in is created
	MessageTypeCheckinCreated MessageType = "checkin.created"

	// MessageTypeCheckinUpdated is sent when a check-in is updated
	MessageTypeCheckinUpdated MessageType = "checkin.updated"

	// MessageTypeCheckinDeleted is sent when a check-in is deleted
	MessageTypeCheckinDeleted MessageType = "checkin.deleted"

	// MessageTypeTimelineUpdate is sent when timeline needs refresh
	MessageTypeTimelineUpdate MessageType = "timeline.update"

	// MessageTypePing is a keepalive message
	MessageTypePing MessageType = "ping"

	// MessageTypePong is a response to ping
	MessageTypePong MessageType = "pong"

	// MessageTypeHandshake is for connection handshake
	MessageTypeHandshake MessageType = "handshake"

	// MessageTypeHandshakeResponse is the server's response to handshake
	MessageTypeHandshakeResponse MessageType = "handshake.response"

	// MessageTypeHealthCheck is for health check status
	MessageTypeHealthCheck MessageType = "health.check"

	// MessageTypeSeqAck acknowledges receipt of a sequence
	MessageTypeSeqAck MessageType = "seq.ack"

	// MessageTypeSeqRetransmit requests retransmission of missing sequences
	MessageTypeSeqRetransmit MessageType = "seq.retransmit"
)

// Message represents a WebSocket message
type Message struct {
	// Type of the message
	Type MessageType `json:"type"`

	// UserID is the target user (empty string means broadcast to all)
	UserID string `json:"user_id,omitempty"`

	// RoomID for room-specific messages
	RoomID string `json:"room_id,omitempty"`

	// Sequence number for message ordering (0 means no sequence tracking)
	Sequence uint64 `json:"sequence,omitempty"`

	// Data contains the message payload (deprecated, use Payload)
	Data interface{} `json:"data,omitempty"`

	// Payload contains the message data
	Payload map[string]interface{} `json:"payload,omitempty"`

	// Timestamp when the message was created
	Timestamp time.Time `json:"timestamp"`
}

// NewMessage creates a new message with current timestamp
func NewMessage(msgType MessageType, data interface{}) *Message {
	return &Message{
		Type:      msgType,
		Data:      data,
		Timestamp: time.Now().UTC(),
	}
}

// ToJSON serializes the message to JSON (deprecated, use Marshal)
func (m *Message) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

// Marshal serializes the message to JSON
func (m *Message) Marshal() ([]byte, error) {
	return json.Marshal(m)
}

// FromJSON deserializes a message from JSON
func FromJSON(data []byte) (*Message, error) {
	var msg Message
	err := json.Unmarshal(data, &msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// CheckinEventData represents data for check-in events
type CheckinEventData struct {
	CheckinID  uuid.UUID `json:"checkin_id"`
	UserID     uuid.UUID `json:"user_id"`
	Action     string    `json:"action"` // "created", "updated", "deleted"
	Title      string    `json:"title,omitempty"`
	Duration   int       `json:"duration,omitempty"`
	CategoryID *uuid.UUID `json:"category_id,omitempty"`
}
