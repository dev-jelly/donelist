package websocket

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestMessageValidator_ValidateMessage(t *testing.T) {
	logger := zap.NewNop()
	validator := NewMessageValidator(logger)

	t.Run("valid message", func(t *testing.T) {
		envelope, err := NewMessageEnvelope(
			EventTypeCheckinCreated,
			"user-123",
			"actor-456",
			1,
			CheckinPayload{
				CheckinID: "550e8400-e29b-41d4-a716-446655440000",
				UserID:    "550e8400-e29b-41d4-a716-446655440001",
				Action:    "created",
				Title:     "Test",
				Duration:  100,
			},
		)
		require.NoError(t, err)

		result := validator.ValidateMessage(envelope)
		assert.True(t, result.Valid)
		assert.Empty(t, result.Errors)
	})

	t.Run("unsupported version", func(t *testing.T) {
		envelope, _ := NewMessageEnvelope(
			EventTypeCheckinCreated,
			"user-123",
			"actor-456",
			1,
			map[string]interface{}{"test": "data"},
		)
		envelope.Version = "v99"

		result := validator.ValidateMessage(envelope)
		assert.False(t, result.Valid)
		assert.NotEmpty(t, result.Errors)
		assert.Contains(t, result.Errors[0].Message, "unsupported schema version")
	})

	t.Run("missing required fields", func(t *testing.T) {
		envelope := &MessageEnvelope{
			Version:   SchemaV1,
			EventType: EventTypeCheckinCreated,
			// Missing other required fields
		}

		result := validator.ValidateMessage(envelope)
		assert.False(t, result.Valid)
		assert.NotEmpty(t, result.Errors)
	})

	t.Run("empty partition key", func(t *testing.T) {
		envelope, _ := NewMessageEnvelope(
			EventTypeCheckinCreated,
			"user-123",
			"actor-456",
			1,
			map[string]interface{}{"test": "data"},
		)
		envelope.PartitionKey = ""

		result := validator.ValidateMessage(envelope)
		assert.False(t, result.Valid)
	})

	t.Run("negative sequence", func(t *testing.T) {
		envelope, _ := NewMessageEnvelope(
			EventTypeCheckinCreated,
			"user-123",
			"actor-456",
			1,
			map[string]interface{}{"test": "data"},
		)
		envelope.Seq = -1

		result := validator.ValidateMessage(envelope)
		assert.False(t, result.Valid)
	})

	t.Run("ping message without payload validation", func(t *testing.T) {
		envelope, err := NewMessageEnvelope(
			EventTypePing,
			"user-123",
			"user-123",
			1,
			map[string]interface{}{},
		)
		require.NoError(t, err)

		result := validator.ValidateMessage(envelope)
		assert.True(t, result.Valid)
		assert.Empty(t, result.Errors)
	})
}

func TestMessageValidator_ValidatePayload_Checkin(t *testing.T) {
	logger := zap.NewNop()
	validator := NewMessageValidator(logger)

	t.Run("valid checkin payload", func(t *testing.T) {
		envelope, err := NewMessageEnvelope(
			EventTypeCheckinCreated,
			"user-123",
			"actor-456",
			1,
			CheckinPayload{
				CheckinID: "550e8400-e29b-41d4-a716-446655440000",
				UserID:    "550e8400-e29b-41d4-a716-446655440001",
				Action:    "created",
				Title:     "Test Checkin",
				Duration:  3600,
			},
		)
		require.NoError(t, err)

		result := validator.ValidateMessage(envelope)
		assert.True(t, result.Valid)
		assert.Empty(t, result.Errors)
	})

	t.Run("invalid checkin payload - missing required fields", func(t *testing.T) {
		envelope, err := NewMessageEnvelope(
			EventTypeCheckinCreated,
			"user-123",
			"actor-456",
			1,
			CheckinPayload{
				// Missing required fields
				Title: "Test",
			},
		)
		require.NoError(t, err)

		result := validator.ValidateMessage(envelope)
		assert.False(t, result.Valid)
		assert.NotEmpty(t, result.Errors)
	})

	t.Run("invalid checkin payload - invalid UUID", func(t *testing.T) {
		envelope, err := NewMessageEnvelope(
			EventTypeCheckinCreated,
			"user-123",
			"actor-456",
			1,
			CheckinPayload{
				CheckinID: "not-a-uuid",
				UserID:    "550e8400-e29b-41d4-a716-446655440001",
				Action:    "created",
			},
		)
		require.NoError(t, err)

		result := validator.ValidateMessage(envelope)
		assert.False(t, result.Valid)
	})

	t.Run("invalid checkin payload - invalid action", func(t *testing.T) {
		envelope, err := NewMessageEnvelope(
			EventTypeCheckinCreated,
			"user-123",
			"actor-456",
			1,
			CheckinPayload{
				CheckinID: "550e8400-e29b-41d4-a716-446655440000",
				UserID:    "550e8400-e29b-41d4-a716-446655440001",
				Action:    "invalid_action",
			},
		)
		require.NoError(t, err)

		result := validator.ValidateMessage(envelope)
		assert.False(t, result.Valid)
	})
}

func TestMessageValidator_ValidatePayload_RoomMessage(t *testing.T) {
	logger := zap.NewNop()
	validator := NewMessageValidator(logger)

	t.Run("valid room message payload", func(t *testing.T) {
		envelope, err := NewMessageEnvelope(
			EventTypeRoomMessage,
			"room-123",
			"user-456",
			1,
			RoomMessagePayload{
				RoomID:   "room-123",
				Content:  "Hello, World!",
				Username: "testuser",
				UserID:   "550e8400-e29b-41d4-a716-446655440000",
			},
		)
		require.NoError(t, err)

		result := validator.ValidateMessage(envelope)
		assert.True(t, result.Valid)
		assert.Empty(t, result.Errors)
	})

	t.Run("invalid room message - empty content", func(t *testing.T) {
		envelope, err := NewMessageEnvelope(
			EventTypeRoomMessage,
			"room-123",
			"user-456",
			1,
			RoomMessagePayload{
				RoomID:  "room-123",
				Content: "",
			},
		)
		require.NoError(t, err)

		result := validator.ValidateMessage(envelope)
		assert.False(t, result.Valid)
	})

	t.Run("invalid room message - content too long", func(t *testing.T) {
		longContent := make([]byte, 10001)
		for i := range longContent {
			longContent[i] = 'a'
		}

		envelope, err := NewMessageEnvelope(
			EventTypeRoomMessage,
			"room-123",
			"user-456",
			1,
			RoomMessagePayload{
				RoomID:  "room-123",
				Content: string(longContent),
			},
		)
		require.NoError(t, err)

		result := validator.ValidateMessage(envelope)
		assert.False(t, result.Valid)
	})
}

func TestMessageValidator_ValidatePayload_Presence(t *testing.T) {
	logger := zap.NewNop()
	validator := NewMessageValidator(logger)

	t.Run("valid presence payload", func(t *testing.T) {
		envelope, err := NewMessageEnvelope(
			EventTypePresenceUpdate,
			"user-123",
			"user-123",
			1,
			PresencePayload{
				UserID:   "550e8400-e29b-41d4-a716-446655440000",
				Username: "testuser",
				Status:   "online",
				LastSeen: time.Now().Unix(),
			},
		)
		require.NoError(t, err)

		result := validator.ValidateMessage(envelope)
		assert.True(t, result.Valid)
		assert.Empty(t, result.Errors)
	})

	t.Run("invalid presence - invalid status", func(t *testing.T) {
		envelope, err := NewMessageEnvelope(
			EventTypePresenceUpdate,
			"user-123",
			"user-123",
			1,
			PresencePayload{
				UserID:   "550e8400-e29b-41d4-a716-446655440000",
				Username: "testuser",
				Status:   "invalid_status",
				LastSeen: time.Now().Unix(),
			},
		)
		require.NoError(t, err)

		result := validator.ValidateMessage(envelope)
		assert.False(t, result.Valid)
	})
}

func TestMessageValidator_ValidatePayload_Error(t *testing.T) {
	logger := zap.NewNop()
	validator := NewMessageValidator(logger)

	t.Run("valid error payload", func(t *testing.T) {
		envelope, err := NewMessageEnvelope(
			EventTypeError,
			"system",
			"system",
			1,
			ErrorPayload{
				Code:    "INVALID_MESSAGE",
				Message: "The message is invalid",
				Details: "Field validation failed",
			},
		)
		require.NoError(t, err)

		result := validator.ValidateMessage(envelope)
		assert.True(t, result.Valid)
		assert.Empty(t, result.Errors)
	})

	t.Run("invalid error payload - missing code", func(t *testing.T) {
		envelope, err := NewMessageEnvelope(
			EventTypeError,
			"system",
			"system",
			1,
			ErrorPayload{
				Message: "The message is invalid",
			},
		)
		require.NoError(t, err)

		result := validator.ValidateMessage(envelope)
		assert.False(t, result.Valid)
	})
}

func TestMessageValidator_Metrics(t *testing.T) {
	logger := zap.NewNop()
	validator := NewMessageValidator(logger)

	// Reset metrics
	validator.ResetMetrics()
	metrics := validator.GetMetrics()
	assert.Equal(t, uint64(0), metrics["total_validations"])

	// Validate a valid message
	validEnvelope, _ := NewMessageEnvelope(
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
	validator.ValidateMessage(validEnvelope)

	metrics = validator.GetMetrics()
	assert.Equal(t, uint64(1), metrics["total_validations"])
	assert.Equal(t, uint64(0), metrics["failed_validations"])

	// Validate an invalid message
	invalidEnvelope, _ := NewMessageEnvelope(
		EventTypeCheckinCreated,
		"user-123",
		"actor-456",
		1,
		map[string]interface{}{},
	)
	invalidEnvelope.Version = "v99"
	validator.ValidateMessage(invalidEnvelope)

	metrics = validator.GetMetrics()
	assert.Equal(t, uint64(2), metrics["total_validations"])
	assert.Equal(t, uint64(1), metrics["failed_validations"])
	assert.Equal(t, uint64(1), metrics["invalid_version"])

	// Reset and verify
	validator.ResetMetrics()
	metrics = validator.GetMetrics()
	assert.Equal(t, uint64(0), metrics["total_validations"])
	assert.Equal(t, uint64(0), metrics["failed_validations"])
}

func TestValidationError(t *testing.T) {
	err := &ValidationError{
		Field:   "test_field",
		Tag:     "required",
		Value:   nil,
		Message: "field is required",
	}

	assert.Contains(t, err.Error(), "test_field")
	assert.Contains(t, err.Error(), "field is required")
}

func BenchmarkMessageValidation(b *testing.B) {
	logger := zap.NewNop()
	validator := NewMessageValidator(logger)

	envelope, _ := NewMessageEnvelope(
		EventTypeCheckinCreated,
		"user-123",
		"actor-456",
		1,
		CheckinPayload{
			CheckinID: "550e8400-e29b-41d4-a716-446655440000",
			UserID:    "550e8400-e29b-41d4-a716-446655440001",
			Action:    "created",
			Title:     "Test",
			Duration:  100,
		},
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.ValidateMessage(envelope)
	}
}
