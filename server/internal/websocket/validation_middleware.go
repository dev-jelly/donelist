package websocket

import (
	"encoding/json"
	"sync/atomic"

	"go.uber.org/zap"
)

// ValidationMiddleware provides message validation for WebSocket connections
type ValidationMiddleware struct {
	validator     *MessageValidator
	logger        *zap.Logger
	strictMode    bool
	sequenceStore *SequenceStore
}

// ValidationMiddlewareConfig holds configuration for validation middleware
type ValidationMiddlewareConfig struct {
	Logger     *zap.Logger
	StrictMode bool // If true, reject invalid messages; if false, log and allow
}

// NewValidationMiddleware creates a new validation middleware
func NewValidationMiddleware(config ValidationMiddlewareConfig) *ValidationMiddleware {
	if config.Logger == nil {
		config.Logger = zap.NewNop()
	}

	return &ValidationMiddleware{
		validator:     NewMessageValidator(config.Logger),
		logger:        config.Logger,
		strictMode:    config.StrictMode,
		sequenceStore: NewSequenceStore(),
	}
}

// ValidateIncomingMessage validates an incoming WebSocket message
func (m *ValidationMiddleware) ValidateIncomingMessage(data []byte, client *Client) (*MessageEnvelope, error) {
	// Try to parse as new envelope format first
	envelope, err := FromBytes(data)
	if err != nil {
		// Try to parse as legacy Message format for backward compatibility
		return m.handleLegacyMessage(data, client)
	}

	// Check if envelope is valid (has required fields)
	// If not, it might be a legacy message that was successfully parsed as JSON
	// but doesn't match the envelope schema
	if !envelope.IsValid() {
		// Try to parse as legacy Message format
		return m.handleLegacyMessage(data, client)
	}

	// Validate the envelope
	result := m.validator.ValidateMessage(envelope)

	// Log validation metrics
	m.logValidationResult(envelope, result, client)

	// Handle validation failures
	if !result.Valid {
		if m.strictMode {
			// In strict mode, return error
			return nil, &ValidationError{
				Field:   "envelope",
				Message: "message validation failed",
			}
		}
		// In non-strict mode, log and continue
		m.logger.Warn("Message validation failed but continuing in non-strict mode",
			zap.String("user_id", client.userID),
			zap.Int("error_count", len(result.Errors)),
		)
	}

	// Verify sequence order for ordered events
	if err := m.verifySequence(envelope, client); err != nil {
		m.logger.Warn("Sequence verification failed",
			zap.String("user_id", client.userID),
			zap.String("partition_key", envelope.PartitionKey),
			zap.Int64("seq", envelope.Seq),
			zap.Error(err),
		)

		if m.strictMode {
			return nil, err
		}
	}

	return envelope, nil
}

// ValidateOutgoingMessage validates an outgoing WebSocket message
func (m *ValidationMiddleware) ValidateOutgoingMessage(envelope *MessageEnvelope) ([]byte, error) {
	// Validate the envelope
	result := m.validator.ValidateMessage(envelope)

	if !result.Valid {
		m.logger.Error("Outgoing message validation failed",
			zap.String("eventType", string(envelope.EventType)),
			zap.Int("error_count", len(result.Errors)),
			zap.Any("errors", result.Errors),
		)

		if m.strictMode {
			return nil, &ValidationError{
				Field:   "envelope",
				Message: "outgoing message validation failed",
			}
		}
	}

	// Serialize to bytes
	return envelope.ToBytes()
}

// handleLegacyMessage handles backward compatibility with old Message format
func (m *ValidationMiddleware) handleLegacyMessage(data []byte, client *Client) (*MessageEnvelope, error) {
	var legacyMsg Message
	if err := json.Unmarshal(data, &legacyMsg); err != nil {
		m.logger.Error("Failed to parse message in any format",
			zap.String("user_id", client.userID),
			zap.Error(err),
		)
		return nil, err
	}

	m.logger.Debug("Received legacy message format, converting to envelope",
		zap.String("type", string(legacyMsg.Type)),
		zap.String("user_id", client.userID),
	)

	// Convert legacy message to envelope format
	envelope, err := m.convertLegacyToEnvelope(&legacyMsg, client)
	if err != nil {
		m.logger.Error("Failed to convert legacy message to envelope",
			zap.Error(err),
		)
		return nil, err
	}

	// Validate the converted envelope (but don't be strict since it's a legacy message)
	result := m.validator.ValidateMessage(envelope)
	if !result.Valid {
		m.logger.Warn("Legacy message validation failed after conversion",
			zap.String("user_id", client.userID),
			zap.Int("error_count", len(result.Errors)),
		)

		// In non-strict mode, allow invalid legacy messages to pass through
		// In strict mode, this would reject them
		if m.strictMode {
			return nil, &ValidationError{
				Field:   "envelope",
				Message: "converted legacy message validation failed",
			}
		}
	}

	return envelope, nil
}

// convertLegacyToEnvelope converts a legacy Message to MessageEnvelope
func (m *ValidationMiddleware) convertLegacyToEnvelope(msg *Message, client *Client) (*MessageEnvelope, error) {
	// Map legacy message type to event type
	eventType := m.mapLegacyTypeToEventType(msg.Type)

	// Determine partition key (use room_id if available, otherwise user_id)
	partitionKey := msg.RoomID
	if partitionKey == "" {
		partitionKey = client.userID
	}

	// Get next sequence number for this partition
	seq := m.sequenceStore.NextSequence(partitionKey)

	// Create payload from legacy data
	var payload interface{}
	if msg.Payload != nil {
		payload = msg.Payload
	} else if msg.Data != nil {
		payload = msg.Data
	} else {
		payload = map[string]interface{}{}
	}

	// Create envelope
	envelope, err := NewMessageEnvelope(
		eventType,
		partitionKey,
		client.userID,
		seq,
		payload,
	)
	if err != nil {
		return nil, err
	}

	// Preserve original timestamp if available
	if !msg.Timestamp.IsZero() {
		envelope.Timestamp = msg.Timestamp.UnixMilli()
	}

	return envelope, nil
}

// mapLegacyTypeToEventType maps legacy MessageType to new EventType
func (m *ValidationMiddleware) mapLegacyTypeToEventType(msgType MessageType) EventType {
	switch msgType {
	case MessageTypeCheckinCreated:
		return EventTypeCheckinCreated
	case MessageTypeCheckinUpdated:
		return EventTypeCheckinUpdated
	case MessageTypeCheckinDeleted:
		return EventTypeCheckinDeleted
	case MessageTypeTimelineUpdate:
		return EventTypeTimelineUpdate
	case MessageTypePing:
		return EventTypePing
	case MessageTypePong:
		return EventTypePong
	case "join_room":
		return EventTypeRoomJoin
	case "leave_room":
		return EventTypeRoomLeave
	case "room_message":
		return EventTypeRoomMessage
	case "typing_start":
		return EventTypeTypingStart
	case "typing_stop":
		return EventTypeTypingStop
	case "update_status":
		return EventTypeStatusUpdate
	case "presence_update":
		return EventTypePresenceUpdate
	case "error":
		return EventTypeError
	default:
		// For unknown types, preserve as-is
		return EventType(msgType)
	}
}

// verifySequence verifies that message sequence is in order
func (m *ValidationMiddleware) verifySequence(envelope *MessageEnvelope, client *Client) error {
	expectedSeq := m.sequenceStore.GetExpectedSequence(envelope.PartitionKey)

	// First message for this partition
	if expectedSeq == 0 {
		m.sequenceStore.UpdateSequence(envelope.PartitionKey, envelope.Seq)
		return nil
	}

	// Check if sequence is as expected
	if envelope.Seq < expectedSeq {
		m.logger.Warn("Out-of-order message (old sequence)",
			zap.String("partition_key", envelope.PartitionKey),
			zap.Int64("expected_seq", expectedSeq),
			zap.Int64("received_seq", envelope.Seq),
		)
		// Don't return error for old messages, just log
		return nil
	}

	if envelope.Seq > expectedSeq+1 {
		m.logger.Warn("Gap in message sequence",
			zap.String("partition_key", envelope.PartitionKey),
			zap.Int64("expected_seq", expectedSeq),
			zap.Int64("received_seq", envelope.Seq),
			zap.Int64("gap", envelope.Seq-expectedSeq),
		)
		// Update to new sequence
		m.sequenceStore.UpdateSequence(envelope.PartitionKey, envelope.Seq)
		return nil
	}

	// Sequence is correct
	m.sequenceStore.UpdateSequence(envelope.PartitionKey, envelope.Seq)
	return nil
}

// logValidationResult logs the validation result with structured logging
func (m *ValidationMiddleware) logValidationResult(envelope *MessageEnvelope, result *ValidationResult, client *Client) {
	if result.Valid {
		m.logger.Debug("Message validation passed",
			zap.String("eventType", string(envelope.EventType)),
			zap.String("version", string(envelope.Version)),
			zap.String("user_id", client.userID),
			zap.Int64("seq", envelope.Seq),
		)
	} else {
		// Increment validation failure counter
		errorDetails := make([]map[string]interface{}, len(result.Errors))
		for i, err := range result.Errors {
			errorDetails[i] = map[string]interface{}{
				"field":   err.Field,
				"tag":     err.Tag,
				"message": err.Message,
			}
		}

		m.logger.Warn("Message validation failed",
			zap.String("eventType", string(envelope.EventType)),
			zap.String("version", string(envelope.Version)),
			zap.String("user_id", client.userID),
			zap.Int64("seq", envelope.Seq),
			zap.Int("error_count", len(result.Errors)),
			zap.Any("validation_errors", errorDetails),
			zap.Any("metrics", m.validator.GetMetrics()),
		)
	}
}

// GetMetrics returns validation metrics
func (m *ValidationMiddleware) GetMetrics() map[string]uint64 {
	return m.validator.GetMetrics()
}

// SequenceStore tracks message sequences per partition
type SequenceStore struct {
	sequences map[string]*atomic.Int64
}

// NewSequenceStore creates a new sequence store
func NewSequenceStore() *SequenceStore {
	return &SequenceStore{
		sequences: make(map[string]*atomic.Int64),
	}
}

// NextSequence returns the next sequence number for a partition
func (s *SequenceStore) NextSequence(partitionKey string) int64 {
	if _, exists := s.sequences[partitionKey]; !exists {
		s.sequences[partitionKey] = &atomic.Int64{}
	}
	return s.sequences[partitionKey].Add(1)
}

// GetExpectedSequence returns the expected next sequence for a partition
func (s *SequenceStore) GetExpectedSequence(partitionKey string) int64 {
	if seq, exists := s.sequences[partitionKey]; exists {
		return seq.Load() + 1
	}
	return 0
}

// UpdateSequence updates the sequence number for a partition
func (s *SequenceStore) UpdateSequence(partitionKey string, seq int64) {
	if _, exists := s.sequences[partitionKey]; !exists {
		s.sequences[partitionKey] = &atomic.Int64{}
	}
	s.sequences[partitionKey].Store(seq)
}
