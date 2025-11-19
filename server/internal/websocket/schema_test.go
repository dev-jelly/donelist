package websocket

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMessageEnvelope(t *testing.T) {
	payload := map[string]interface{}{
		"test": "data",
		"num":  123,
	}

	envelope, err := NewMessageEnvelope(
		EventTypeCheckinCreated,
		"user-123",
		"actor-456",
		1,
		payload,
	)

	require.NoError(t, err)
	assert.Equal(t, SchemaV1, envelope.Version)
	assert.Equal(t, EventTypeCheckinCreated, envelope.EventType)
	assert.Equal(t, "user-123", envelope.PartitionKey)
	assert.Equal(t, "actor-456", envelope.ActorID)
	assert.Equal(t, int64(1), envelope.Seq)
	assert.True(t, envelope.Timestamp > 0)
	assert.NotNil(t, envelope.Payload)
	assert.NotNil(t, envelope.Metadata)
}

func TestMessageEnvelopeGetPayload(t *testing.T) {
	originalPayload := CheckinPayload{
		CheckinID: "550e8400-e29b-41d4-a716-446655440000",
		UserID:    "550e8400-e29b-41d4-a716-446655440001",
		Action:    "created",
		Title:     "Test Checkin",
		Duration:  3600,
	}

	envelope, err := NewMessageEnvelope(
		EventTypeCheckinCreated,
		"user-123",
		"actor-456",
		1,
		originalPayload,
	)
	require.NoError(t, err)

	var retrievedPayload CheckinPayload
	err = envelope.GetPayload(&retrievedPayload)
	require.NoError(t, err)

	assert.Equal(t, originalPayload.CheckinID, retrievedPayload.CheckinID)
	assert.Equal(t, originalPayload.UserID, retrievedPayload.UserID)
	assert.Equal(t, originalPayload.Action, retrievedPayload.Action)
	assert.Equal(t, originalPayload.Title, retrievedPayload.Title)
	assert.Equal(t, originalPayload.Duration, retrievedPayload.Duration)
}

func TestMessageEnvelopeSetPayload(t *testing.T) {
	envelope, err := NewMessageEnvelope(
		EventTypeRoomMessage,
		"room-123",
		"user-456",
		1,
		map[string]interface{}{},
	)
	require.NoError(t, err)

	newPayload := RoomMessagePayload{
		RoomID:   "room-123",
		Content:  "Hello, World!",
		Username: "testuser",
		UserID:   "user-456",
	}

	err = envelope.SetPayload(newPayload)
	require.NoError(t, err)

	var retrieved RoomMessagePayload
	err = envelope.GetPayload(&retrieved)
	require.NoError(t, err)

	assert.Equal(t, newPayload.RoomID, retrieved.RoomID)
	assert.Equal(t, newPayload.Content, retrieved.Content)
	assert.Equal(t, newPayload.Username, retrieved.Username)
}

func TestMessageEnvelopeSerialization(t *testing.T) {
	payload := map[string]interface{}{
		"test": "data",
		"num":  123,
	}

	original, err := NewMessageEnvelope(
		EventTypeCheckinCreated,
		"user-123",
		"actor-456",
		42,
		payload,
	)
	require.NoError(t, err)

	// Serialize
	data, err := original.ToBytes()
	require.NoError(t, err)
	assert.True(t, len(data) > 0)

	// Deserialize
	decoded, err := FromBytes(data)
	require.NoError(t, err)

	assert.Equal(t, original.Version, decoded.Version)
	assert.Equal(t, original.EventType, decoded.EventType)
	assert.Equal(t, original.PartitionKey, decoded.PartitionKey)
	assert.Equal(t, original.Seq, decoded.Seq)
	assert.Equal(t, original.ActorID, decoded.ActorID)
	assert.Equal(t, original.Timestamp, decoded.Timestamp)
}

func TestMessageEnvelopeIsValid(t *testing.T) {
	tests := []struct {
		name     string
		envelope *MessageEnvelope
		expected bool
	}{
		{
			name: "valid envelope",
			envelope: &MessageEnvelope{
				Version:      SchemaV1,
				EventType:    EventTypeCheckinCreated,
				PartitionKey: "user-123",
				Seq:          1,
				Timestamp:    time.Now().UnixMilli(),
				ActorID:      "actor-456",
				Payload:      json.RawMessage(`{"test":"data"}`),
			},
			expected: true,
		},
		{
			name: "missing version",
			envelope: &MessageEnvelope{
				EventType:    EventTypeCheckinCreated,
				PartitionKey: "user-123",
				Seq:          1,
				Timestamp:    time.Now().UnixMilli(),
				ActorID:      "actor-456",
				Payload:      json.RawMessage(`{"test":"data"}`),
			},
			expected: false,
		},
		{
			name: "missing event type",
			envelope: &MessageEnvelope{
				Version:      SchemaV1,
				PartitionKey: "user-123",
				Seq:          1,
				Timestamp:    time.Now().UnixMilli(),
				ActorID:      "actor-456",
				Payload:      json.RawMessage(`{"test":"data"}`),
			},
			expected: false,
		},
		{
			name: "missing partition key",
			envelope: &MessageEnvelope{
				Version:   SchemaV1,
				EventType: EventTypeCheckinCreated,
				Seq:       1,
				Timestamp: time.Now().UnixMilli(),
				ActorID:   "actor-456",
				Payload:   json.RawMessage(`{"test":"data"}`),
			},
			expected: false,
		},
		{
			name: "negative sequence",
			envelope: &MessageEnvelope{
				Version:      SchemaV1,
				EventType:    EventTypeCheckinCreated,
				PartitionKey: "user-123",
				Seq:          -1,
				Timestamp:    time.Now().UnixMilli(),
				ActorID:      "actor-456",
				Payload:      json.RawMessage(`{"test":"data"}`),
			},
			expected: false,
		},
		{
			name: "missing timestamp",
			envelope: &MessageEnvelope{
				Version:      SchemaV1,
				EventType:    EventTypeCheckinCreated,
				PartitionKey: "user-123",
				Seq:          1,
				ActorID:      "actor-456",
				Payload:      json.RawMessage(`{"test":"data"}`),
			},
			expected: false,
		},
		{
			name: "missing actor ID",
			envelope: &MessageEnvelope{
				Version:      SchemaV1,
				EventType:    EventTypeCheckinCreated,
				PartitionKey: "user-123",
				Seq:          1,
				Timestamp:    time.Now().UnixMilli(),
				Payload:      json.RawMessage(`{"test":"data"}`),
			},
			expected: false,
		},
		{
			name: "empty payload",
			envelope: &MessageEnvelope{
				Version:      SchemaV1,
				EventType:    EventTypeCheckinCreated,
				PartitionKey: "user-123",
				Seq:          1,
				Timestamp:    time.Now().UnixMilli(),
				ActorID:      "actor-456",
				Payload:      json.RawMessage(``),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.envelope.IsValid())
		})
	}
}

func TestSchemaRegistry(t *testing.T) {
	registry := NewSchemaRegistry()

	t.Run("check supported versions", func(t *testing.T) {
		assert.True(t, registry.IsVersionSupported(SchemaV1))
		assert.False(t, registry.IsVersionSupported(SchemaV2))
		assert.False(t, registry.IsVersionSupported("v99"))
	})

	t.Run("get current version", func(t *testing.T) {
		assert.Equal(t, SchemaV1, registry.GetCurrentVersion())
	})

	t.Run("migrate message same version", func(t *testing.T) {
		envelope, _ := NewMessageEnvelope(
			EventTypeCheckinCreated,
			"user-123",
			"actor-456",
			1,
			map[string]interface{}{"test": "data"},
		)

		migrated, err := registry.MigrateMessage(envelope, SchemaV1)
		require.NoError(t, err)
		assert.Equal(t, envelope, migrated)
	})

	t.Run("migrate to unsupported version", func(t *testing.T) {
		envelope, _ := NewMessageEnvelope(
			EventTypeCheckinCreated,
			"user-123",
			"actor-456",
			1,
			map[string]interface{}{"test": "data"},
		)

		_, err := registry.MigrateMessage(envelope, SchemaV2)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported target version")
	})
}

func TestCheckinPayload(t *testing.T) {
	categoryID := "550e8400-e29b-41d4-a716-446655440002"
	payload := CheckinPayload{
		CheckinID:  "550e8400-e29b-41d4-a716-446655440000",
		UserID:     "550e8400-e29b-41d4-a716-446655440001",
		Action:     "created",
		Title:      "Test Checkin",
		Duration:   3600,
		CategoryID: &categoryID,
	}

	envelope, err := NewMessageEnvelope(
		EventTypeCheckinCreated,
		payload.UserID,
		payload.UserID,
		1,
		payload,
	)
	require.NoError(t, err)

	var retrieved CheckinPayload
	err = envelope.GetPayload(&retrieved)
	require.NoError(t, err)

	assert.Equal(t, payload.CheckinID, retrieved.CheckinID)
	assert.Equal(t, payload.UserID, retrieved.UserID)
	assert.Equal(t, payload.Action, retrieved.Action)
	assert.Equal(t, payload.Title, retrieved.Title)
	assert.Equal(t, payload.Duration, retrieved.Duration)
	assert.NotNil(t, retrieved.CategoryID)
	assert.Equal(t, *payload.CategoryID, *retrieved.CategoryID)
}

func TestRoomMessagePayload(t *testing.T) {
	payload := RoomMessagePayload{
		RoomID:   "room-123",
		Content:  "Hello, World!",
		Username: "testuser",
		UserID:   "550e8400-e29b-41d4-a716-446655440000",
	}

	envelope, err := NewMessageEnvelope(
		EventTypeRoomMessage,
		payload.RoomID,
		payload.UserID,
		1,
		payload,
	)
	require.NoError(t, err)

	var retrieved RoomMessagePayload
	err = envelope.GetPayload(&retrieved)
	require.NoError(t, err)

	assert.Equal(t, payload.RoomID, retrieved.RoomID)
	assert.Equal(t, payload.Content, retrieved.Content)
	assert.Equal(t, payload.Username, retrieved.Username)
	assert.Equal(t, payload.UserID, retrieved.UserID)
}

func TestPresencePayload(t *testing.T) {
	payload := PresencePayload{
		UserID:   "550e8400-e29b-41d4-a716-446655440000",
		Username: "testuser",
		Status:   "online",
		LastSeen: time.Now().Unix(),
	}

	envelope, err := NewMessageEnvelope(
		EventTypePresenceUpdate,
		payload.UserID,
		payload.UserID,
		1,
		payload,
	)
	require.NoError(t, err)

	var retrieved PresencePayload
	err = envelope.GetPayload(&retrieved)
	require.NoError(t, err)

	assert.Equal(t, payload.UserID, retrieved.UserID)
	assert.Equal(t, payload.Username, retrieved.Username)
	assert.Equal(t, payload.Status, retrieved.Status)
	assert.Equal(t, payload.LastSeen, retrieved.LastSeen)
}

func TestErrorPayload(t *testing.T) {
	payload := ErrorPayload{
		Code:    "INVALID_MESSAGE",
		Message: "The message format is invalid",
		Details: "Field 'user_id' is required",
	}

	envelope, err := NewMessageEnvelope(
		EventTypeError,
		"system",
		"system",
		1,
		payload,
	)
	require.NoError(t, err)

	var retrieved ErrorPayload
	err = envelope.GetPayload(&retrieved)
	require.NoError(t, err)

	assert.Equal(t, payload.Code, retrieved.Code)
	assert.Equal(t, payload.Message, retrieved.Message)
	assert.Equal(t, payload.Details, retrieved.Details)
}
