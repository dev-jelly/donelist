package websocket

import (
	"encoding/json"
	"fmt"
	"time"
)

// SchemaVersion represents the message schema version
type SchemaVersion string

const (
	// SchemaV1 is version 1 of the message schema (current)
	SchemaV1 SchemaVersion = "v1"
	// SchemaV2 is version 2 with enhanced features (future)
	SchemaV2 SchemaVersion = "v2"
)

// EventType represents the type of event in the message
type EventType string

const (
	// EventTypeCheckinCreated is sent when a check-in is created
	EventTypeCheckinCreated EventType = "checkin.created"
	// EventTypeCheckinUpdated is sent when a check-in is updated
	EventTypeCheckinUpdated EventType = "checkin.updated"
	// EventTypeCheckinDeleted is sent when a check-in is deleted
	EventTypeCheckinDeleted EventType = "checkin.deleted"
	// EventTypeTimelineUpdate is sent when timeline needs refresh
	EventTypeTimelineUpdate EventType = "timeline.update"
	// EventTypeRoomJoin is sent when joining a room
	EventTypeRoomJoin EventType = "room.join"
	// EventTypeRoomLeave is sent when leaving a room
	EventTypeRoomLeave EventType = "room.leave"
	// EventTypeRoomMessage is sent for room messages
	EventTypeRoomMessage EventType = "room.message"
	// EventTypeTypingStart is sent when typing starts
	EventTypeTypingStart EventType = "typing.start"
	// EventTypeTypingStop is sent when typing stops
	EventTypeTypingStop EventType = "typing.stop"
	// EventTypePresenceUpdate is sent for presence updates
	EventTypePresenceUpdate EventType = "presence.update"
	// EventTypeStatusUpdate is sent for status updates
	EventTypeStatusUpdate EventType = "status.update"
	// EventTypePing is a keepalive message
	EventTypePing EventType = "ping"
	// EventTypePong is a response to ping
	EventTypePong EventType = "pong"
	// EventTypeError is sent for error messages
	EventTypeError EventType = "error"
)

// MessageEnvelope represents the standard message envelope with version and tracking
type MessageEnvelope struct {
	// Version of the message schema
	Version SchemaVersion `json:"version" validate:"required,oneof=v1 v2"`

	// EventType specifies the type of event
	EventType EventType `json:"eventType" validate:"required"`

	// PartitionKey for message ordering (typically user_id or room_id)
	PartitionKey string `json:"partitionKey" validate:"required,min=1,max=256"`

	// Seq is the incremental sequence number for this partition
	Seq int64 `json:"seq" validate:"required,min=0"`

	// Timestamp when the message was created (Unix milliseconds)
	Timestamp int64 `json:"ts" validate:"required,min=0"`

	// ActorID is the ID of the user who initiated this event
	ActorID string `json:"actorId" validate:"required,min=1,max=256"`

	// Payload contains the event-specific data
	Payload json.RawMessage `json:"payload" validate:"required"`

	// Metadata contains optional metadata (not validated strictly)
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// NewMessageEnvelope creates a new message envelope with the current schema version
func NewMessageEnvelope(eventType EventType, partitionKey, actorID string, seq int64, payload interface{}) (*MessageEnvelope, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	return &MessageEnvelope{
		Version:      SchemaV1,
		EventType:    eventType,
		PartitionKey: partitionKey,
		Seq:          seq,
		Timestamp:    time.Now().UnixMilli(),
		ActorID:      actorID,
		Payload:      payloadBytes,
		Metadata:     make(map[string]interface{}),
	}, nil
}

// GetPayload unmarshals the payload into the provided structure
func (m *MessageEnvelope) GetPayload(v interface{}) error {
	return json.Unmarshal(m.Payload, v)
}

// SetPayload sets the payload from the provided structure
func (m *MessageEnvelope) SetPayload(v interface{}) error {
	payloadBytes, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}
	m.Payload = payloadBytes
	return nil
}

// ToBytes serializes the message envelope to JSON bytes
func (m *MessageEnvelope) ToBytes() ([]byte, error) {
	return json.Marshal(m)
}

// FromBytes deserializes a message envelope from JSON bytes
func FromBytes(data []byte) (*MessageEnvelope, error) {
	var envelope MessageEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message envelope: %w", err)
	}
	return &envelope, nil
}

// IsValid checks if the message envelope has all required fields
func (m *MessageEnvelope) IsValid() bool {
	return m.Version != "" &&
		m.EventType != "" &&
		m.PartitionKey != "" &&
		m.Seq >= 0 &&
		m.Timestamp > 0 &&
		m.ActorID != "" &&
		len(m.Payload) > 0
}

// SchemaRegistry manages message schema versions and compatibility
type SchemaRegistry struct {
	supportedVersions map[SchemaVersion]bool
	currentVersion    SchemaVersion
}

// NewSchemaRegistry creates a new schema registry
func NewSchemaRegistry() *SchemaRegistry {
	return &SchemaRegistry{
		supportedVersions: map[SchemaVersion]bool{
			SchemaV1: true,
			SchemaV2: false, // Not yet supported
		},
		currentVersion: SchemaV1,
	}
}

// IsVersionSupported checks if a schema version is supported
func (r *SchemaRegistry) IsVersionSupported(version SchemaVersion) bool {
	supported, exists := r.supportedVersions[version]
	return exists && supported
}

// GetCurrentVersion returns the current schema version
func (r *SchemaRegistry) GetCurrentVersion() SchemaVersion {
	return r.currentVersion
}

// MigrateMessage migrates a message from one version to another
func (r *SchemaRegistry) MigrateMessage(envelope *MessageEnvelope, targetVersion SchemaVersion) (*MessageEnvelope, error) {
	if envelope.Version == targetVersion {
		return envelope, nil
	}

	// For now, we only support v1, so migration is not yet implemented
	if !r.IsVersionSupported(targetVersion) {
		return nil, fmt.Errorf("unsupported target version: %s", targetVersion)
	}

	// Future: Implement version migration logic here
	return envelope, nil
}

// Payload structures for different event types

// CheckinPayload represents the payload for check-in events
type CheckinPayload struct {
	CheckinID  string  `json:"checkin_id" validate:"required,uuid"`
	UserID     string  `json:"user_id" validate:"required,uuid"`
	Action     string  `json:"action" validate:"required,oneof=created updated deleted"`
	Title      string  `json:"title,omitempty" validate:"omitempty,max=500"`
	Duration   int     `json:"duration,omitempty" validate:"omitempty,min=0"`
	CategoryID *string `json:"category_id,omitempty" validate:"omitempty,uuid"`
}

// RoomMessagePayload represents the payload for room messages
type RoomMessagePayload struct {
	RoomID   string `json:"room_id" validate:"required,min=1,max=256"`
	Content  string `json:"content" validate:"required,min=1,max=10000"`
	Username string `json:"username,omitempty" validate:"omitempty,max=256"`
	UserID   string `json:"user_id,omitempty" validate:"omitempty,uuid"`
}

// PresencePayload represents the payload for presence updates
type PresencePayload struct {
	UserID   string `json:"user_id" validate:"required,uuid"`
	Username string `json:"username,omitempty" validate:"omitempty,max=256"`
	Status   string `json:"status" validate:"required,oneof=online away offline"`
	LastSeen int64  `json:"last_seen" validate:"required,min=0"`
}

// TypingPayload represents the payload for typing indicators
type TypingPayload struct {
	RoomID    string `json:"room_id" validate:"required,min=1,max=256"`
	UserID    string `json:"user_id" validate:"required,uuid"`
	Username  string `json:"username,omitempty" validate:"omitempty,max=256"`
	IsTyping  bool   `json:"is_typing"`
}

// ErrorPayload represents the payload for error messages
type ErrorPayload struct {
	Code    string `json:"code" validate:"required,min=1,max=64"`
	Message string `json:"message" validate:"required,min=1,max=1000"`
	Details string `json:"details,omitempty" validate:"omitempty,max=5000"`
}
