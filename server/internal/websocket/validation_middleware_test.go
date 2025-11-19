package websocket

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestValidationMiddleware_ValidateIncomingMessage(t *testing.T) {
	logger := zap.NewNop()

	t.Run("valid new format message", func(t *testing.T) {
		middleware := NewValidationMiddleware(ValidationMiddlewareConfig{
			Logger:     logger,
			StrictMode: true,
		})

		envelope, err := NewMessageEnvelope(
			EventTypeCheckinCreated,
			"user-123",
			"actor-456",
			1,
			CheckinPayload{
				CheckinID: "550e8400-e29b-41d4-a716-446655440000",
				UserID:    "550e8400-e29b-41d4-a716-446655440001",
				Action:    "created",
			},
		)
		require.NoError(t, err)

		data, err := envelope.ToBytes()
		require.NoError(t, err)

		client := &Client{
			userID: "user-123",
			logger: logger,
		}

		validated, err := middleware.ValidateIncomingMessage(data, client)
		assert.NoError(t, err)
		assert.NotNil(t, validated)
		assert.Equal(t, EventTypeCheckinCreated, validated.EventType)
	})

	t.Run("invalid message in strict mode", func(t *testing.T) {
		middleware := NewValidationMiddleware(ValidationMiddlewareConfig{
			Logger:     logger,
			StrictMode: true,
		})

		envelope, _ := NewMessageEnvelope(
			EventTypeCheckinCreated,
			"user-123",
			"actor-456",
			1,
			map[string]interface{}{},
		)
		envelope.Version = "v99" // Unsupported version

		data, err := envelope.ToBytes()
		require.NoError(t, err)

		client := &Client{
			userID: "user-123",
			logger: logger,
		}

		_, err = middleware.ValidateIncomingMessage(data, client)
		assert.Error(t, err)
	})

	t.Run("invalid message in non-strict mode", func(t *testing.T) {
		middleware := NewValidationMiddleware(ValidationMiddlewareConfig{
			Logger:     logger,
			StrictMode: false,
		})

		envelope, _ := NewMessageEnvelope(
			EventTypeCheckinCreated,
			"user-123",
			"actor-456",
			1,
			map[string]interface{}{},
		)
		envelope.Version = "v99"

		data, err := envelope.ToBytes()
		require.NoError(t, err)

		client := &Client{
			userID: "user-123",
			logger: logger,
		}

		validated, err := middleware.ValidateIncomingMessage(data, client)
		assert.NoError(t, err) // Should not error in non-strict mode
		assert.NotNil(t, validated)
	})

	t.Run("legacy message format", func(t *testing.T) {
		middleware := NewValidationMiddleware(ValidationMiddlewareConfig{
			Logger:     logger,
			StrictMode: false,
		})

		legacyMsg := Message{
			Type:      MessageTypeCheckinCreated,
			UserID:    "user-123",
			Timestamp: time.Now(),
			Payload: map[string]interface{}{
				"checkin_id": "550e8400-e29b-41d4-a716-446655440000",
			},
		}

		data, err := json.Marshal(legacyMsg)
		require.NoError(t, err)

		client := &Client{
			userID: "user-123",
			logger: logger,
		}

		envelope, err := middleware.ValidateIncomingMessage(data, client)
		assert.NoError(t, err)
		assert.NotNil(t, envelope)
		assert.Equal(t, EventTypeCheckinCreated, envelope.EventType)
		assert.Equal(t, SchemaV1, envelope.Version)
	})

	t.Run("malformed JSON", func(t *testing.T) {
		middleware := NewValidationMiddleware(ValidationMiddlewareConfig{
			Logger:     logger,
			StrictMode: true,
		})

		data := []byte(`{invalid json}`)

		client := &Client{
			userID: "user-123",
			logger: logger,
		}

		_, err := middleware.ValidateIncomingMessage(data, client)
		assert.Error(t, err)
	})
}

func TestValidationMiddleware_ValidateOutgoingMessage(t *testing.T) {
	logger := zap.NewNop()

	t.Run("valid outgoing message", func(t *testing.T) {
		middleware := NewValidationMiddleware(ValidationMiddlewareConfig{
			Logger:     logger,
			StrictMode: true,
		})

		envelope, err := NewMessageEnvelope(
			EventTypeCheckinCreated,
			"user-123",
			"actor-456",
			1,
			CheckinPayload{
				CheckinID: "550e8400-e29b-41d4-a716-446655440000",
				UserID:    "550e8400-e29b-41d4-a716-446655440001",
				Action:    "created",
			},
		)
		require.NoError(t, err)

		data, err := middleware.ValidateOutgoingMessage(envelope)
		assert.NoError(t, err)
		assert.NotEmpty(t, data)
	})

	t.Run("invalid outgoing message in strict mode", func(t *testing.T) {
		middleware := NewValidationMiddleware(ValidationMiddlewareConfig{
			Logger:     logger,
			StrictMode: true,
		})

		envelope, _ := NewMessageEnvelope(
			EventTypeCheckinCreated,
			"user-123",
			"actor-456",
			1,
			map[string]interface{}{},
		)
		envelope.Version = "v99"

		_, err := middleware.ValidateOutgoingMessage(envelope)
		assert.Error(t, err)
	})

	t.Run("invalid outgoing message in non-strict mode", func(t *testing.T) {
		middleware := NewValidationMiddleware(ValidationMiddlewareConfig{
			Logger:     logger,
			StrictMode: false,
		})

		envelope, _ := NewMessageEnvelope(
			EventTypeCheckinCreated,
			"user-123",
			"actor-456",
			1,
			map[string]interface{}{},
		)
		envelope.Version = "v99"

		data, err := middleware.ValidateOutgoingMessage(envelope)
		assert.NoError(t, err)
		assert.NotEmpty(t, data)
	})
}

func TestValidationMiddleware_LegacyConversion(t *testing.T) {
	logger := zap.NewNop()
	middleware := NewValidationMiddleware(ValidationMiddlewareConfig{
		Logger:     logger,
		StrictMode: false,
	})

	tests := []struct {
		name          string
		legacyType    MessageType
		expectedEvent EventType
	}{
		{"checkin created", MessageTypeCheckinCreated, EventTypeCheckinCreated},
		{"checkin updated", MessageTypeCheckinUpdated, EventTypeCheckinUpdated},
		{"checkin deleted", MessageTypeCheckinDeleted, EventTypeCheckinDeleted},
		{"timeline update", MessageTypeTimelineUpdate, EventTypeTimelineUpdate},
		{"ping", MessageTypePing, EventTypePing},
		{"pong", MessageTypePong, EventTypePong},
		{"join room", "join_room", EventTypeRoomJoin},
		{"leave room", "leave_room", EventTypeRoomLeave},
		{"room message", "room_message", EventTypeRoomMessage},
		{"typing start", "typing_start", EventTypeTypingStart},
		{"typing stop", "typing_stop", EventTypeTypingStop},
		{"update status", "update_status", EventTypeStatusUpdate},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			legacyMsg := Message{
				Type:      tt.legacyType,
				UserID:    "user-123",
				Timestamp: time.Now(),
				Payload: map[string]interface{}{
					"test": "data",
				},
			}

			data, err := json.Marshal(legacyMsg)
			require.NoError(t, err)

			client := &Client{
				userID: "user-123",
				logger: logger,
			}

			envelope, err := middleware.ValidateIncomingMessage(data, client)
			assert.NoError(t, err)
			assert.NotNil(t, envelope)
			assert.Equal(t, tt.expectedEvent, envelope.EventType)
			assert.Equal(t, SchemaV1, envelope.Version)
			assert.True(t, envelope.Seq > 0)
		})
	}
}

func TestValidationMiddleware_SequenceTracking(t *testing.T) {
	logger := zap.NewNop()
	middleware := NewValidationMiddleware(ValidationMiddlewareConfig{
		Logger:     logger,
		StrictMode: false,
	})

	client := &Client{
		userID: "user-123",
		logger: logger,
	}

	partitionKey := "user-123"

	// First message
	envelope1, _ := NewMessageEnvelope(
		EventTypeCheckinCreated,
		partitionKey,
		"actor-456",
		1,
		map[string]interface{}{"test": "data1"},
	)
	data1, _ := envelope1.ToBytes()
	_, err := middleware.ValidateIncomingMessage(data1, client)
	assert.NoError(t, err)

	// Second message with correct sequence
	envelope2, _ := NewMessageEnvelope(
		EventTypeCheckinCreated,
		partitionKey,
		"actor-456",
		2,
		map[string]interface{}{"test": "data2"},
	)
	data2, _ := envelope2.ToBytes()
	_, err = middleware.ValidateIncomingMessage(data2, client)
	assert.NoError(t, err)

	// Message with gap in sequence (should log warning but not fail)
	envelope3, _ := NewMessageEnvelope(
		EventTypeCheckinCreated,
		partitionKey,
		"actor-456",
		5,
		map[string]interface{}{"test": "data3"},
	)
	data3, _ := envelope3.ToBytes()
	_, err = middleware.ValidateIncomingMessage(data3, client)
	assert.NoError(t, err) // Should not fail, just log

	// Old sequence number (should log warning but not fail)
	envelope4, _ := NewMessageEnvelope(
		EventTypeCheckinCreated,
		partitionKey,
		"actor-456",
		3,
		map[string]interface{}{"test": "data4"},
	)
	data4, _ := envelope4.ToBytes()
	_, err = middleware.ValidateIncomingMessage(data4, client)
	assert.NoError(t, err)
}

func TestValidationMiddleware_GetMetrics(t *testing.T) {
	logger := zap.NewNop()
	middleware := NewValidationMiddleware(ValidationMiddlewareConfig{
		Logger:     logger,
		StrictMode: false,
	})

	client := &Client{
		userID: "user-123",
		logger: logger,
	}

	// Valid message
	envelope1, _ := NewMessageEnvelope(
		EventTypeCheckinCreated,
		"user-123",
		"actor-456",
		1,
		CheckinPayload{
			CheckinID: "550e8400-e29b-41d4-a716-446655440000",
			UserID:    "550e8400-e29b-41d4-a716-446655440001",
			Action:    "created",
		},
	)
	data1, _ := envelope1.ToBytes()
	middleware.ValidateIncomingMessage(data1, client)

	// Invalid message
	envelope2, _ := NewMessageEnvelope(
		EventTypeCheckinCreated,
		"user-123",
		"actor-456",
		2,
		map[string]interface{}{},
	)
	envelope2.Version = "v99"
	data2, _ := envelope2.ToBytes()
	middleware.ValidateIncomingMessage(data2, client)

	metrics := middleware.GetMetrics()
	assert.Greater(t, metrics["total_validations"], uint64(0))
	assert.Greater(t, metrics["failed_validations"], uint64(0))
}

func TestSequenceStore(t *testing.T) {
	store := NewSequenceStore()

	t.Run("next sequence increments", func(t *testing.T) {
		partition := "user-123"

		seq1 := store.NextSequence(partition)
		assert.Equal(t, int64(1), seq1)

		seq2 := store.NextSequence(partition)
		assert.Equal(t, int64(2), seq2)

		seq3 := store.NextSequence(partition)
		assert.Equal(t, int64(3), seq3)
	})

	t.Run("different partitions have separate sequences", func(t *testing.T) {
		partition1 := "user-123"
		partition2 := "user-456"

		seq1 := store.NextSequence(partition1)
		seq2 := store.NextSequence(partition2)

		assert.NotEqual(t, seq1, seq2)
		assert.Equal(t, int64(1), seq2) // partition2 starts at 1
	})

	t.Run("get expected sequence", func(t *testing.T) {
		store := NewSequenceStore()
		partition := "user-789"

		// Before any sequence
		expected := store.GetExpectedSequence(partition)
		assert.Equal(t, int64(0), expected)

		// After first sequence
		store.NextSequence(partition)
		expected = store.GetExpectedSequence(partition)
		assert.Equal(t, int64(2), expected)
	})

	t.Run("update sequence", func(t *testing.T) {
		store := NewSequenceStore()
		partition := "user-999"

		store.UpdateSequence(partition, 10)
		expected := store.GetExpectedSequence(partition)
		assert.Equal(t, int64(11), expected)

		next := store.NextSequence(partition)
		assert.Equal(t, int64(11), next)
	})
}

func BenchmarkValidationMiddleware_ValidateIncoming(b *testing.B) {
	logger := zap.NewNop()
	middleware := NewValidationMiddleware(ValidationMiddlewareConfig{
		Logger:     logger,
		StrictMode: true,
	})

	envelope, _ := NewMessageEnvelope(
		EventTypeCheckinCreated,
		"user-123",
		"actor-456",
		1,
		CheckinPayload{
			CheckinID: "550e8400-e29b-41d4-a716-446655440000",
			UserID:    "550e8400-e29b-41d4-a716-446655440001",
			Action:    "created",
		},
	)
	data, _ := envelope.ToBytes()

	client := &Client{
		userID: "user-123",
		logger: logger,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		middleware.ValidateIncomingMessage(data, client)
	}
}
