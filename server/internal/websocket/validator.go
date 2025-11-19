package websocket

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-playground/validator/v10"
	"go.uber.org/zap"
)

// ValidationMetrics tracks validation statistics
type ValidationMetrics struct {
	TotalValidations  atomic.Uint64
	FailedValidations atomic.Uint64
	InvalidVersion    atomic.Uint64
	InvalidSchema     atomic.Uint64
	MissingFields     atomic.Uint64
	InvalidPayload    atomic.Uint64
}

// MessageValidator validates WebSocket messages
type MessageValidator struct {
	validate       *validator.Validate
	schemaRegistry *SchemaRegistry
	logger         *zap.Logger
	metrics        *ValidationMetrics
	mu             sync.RWMutex
}

// ValidationError represents a message validation error
type ValidationError struct {
	Field   string
	Tag     string
	Value   interface{}
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation failed on field '%s': %s", e.Field, e.Message)
}

// ValidationResult contains the result of message validation
type ValidationResult struct {
	Valid   bool
	Errors  []ValidationError
	Metrics map[string]interface{}
}

// NewMessageValidator creates a new message validator
func NewMessageValidator(logger *zap.Logger) *MessageValidator {
	v := validator.New()

	// Register custom validators
	v.RegisterValidation("event_type", validateEventType)
	v.RegisterValidation("schema_version", validateSchemaVersion)

	return &MessageValidator{
		validate:       v,
		schemaRegistry: NewSchemaRegistry(),
		logger:         logger,
		metrics:        &ValidationMetrics{},
	}
}

// ValidateMessage validates a message envelope
func (v *MessageValidator) ValidateMessage(envelope *MessageEnvelope) *ValidationResult {
	v.metrics.TotalValidations.Add(1)

	result := &ValidationResult{
		Valid:  true,
		Errors: make([]ValidationError, 0),
		Metrics: map[string]interface{}{
			"timestamp": time.Now().UnixMilli(),
		},
	}

	// Check if version is supported
	if !v.schemaRegistry.IsVersionSupported(envelope.Version) {
		v.metrics.InvalidVersion.Add(1)
		v.metrics.FailedValidations.Add(1)
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "version",
			Tag:     "unsupported",
			Value:   envelope.Version,
			Message: fmt.Sprintf("unsupported schema version: %s", envelope.Version),
		})

		v.logger.Warn("Unsupported schema version",
			zap.String("version", string(envelope.Version)),
			zap.String("eventType", string(envelope.EventType)),
			zap.String("actorId", envelope.ActorID),
		)
		return result
	}

	// Validate basic message structure
	if !envelope.IsValid() {
		v.metrics.MissingFields.Add(1)
		v.metrics.FailedValidations.Add(1)
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "envelope",
			Tag:     "required_fields",
			Message: "message envelope is missing required fields",
		})

		v.logger.Warn("Invalid message envelope",
			zap.Bool("hasVersion", envelope.Version != ""),
			zap.Bool("hasEventType", envelope.EventType != ""),
			zap.Bool("hasPartitionKey", envelope.PartitionKey != ""),
			zap.Bool("hasSeq", envelope.Seq >= 0),
			zap.Bool("hasTimestamp", envelope.Timestamp > 0),
			zap.Bool("hasActorID", envelope.ActorID != ""),
			zap.Bool("hasPayload", len(envelope.Payload) > 0),
		)
		return result
	}

	// Validate using struct tags
	if err := v.validate.Struct(envelope); err != nil {
		v.metrics.InvalidSchema.Add(1)
		v.metrics.FailedValidations.Add(1)
		result.Valid = false

		if validationErrors, ok := err.(validator.ValidationErrors); ok {
			for _, fieldErr := range validationErrors {
				validationError := ValidationError{
					Field:   fieldErr.Field(),
					Tag:     fieldErr.Tag(),
					Value:   fieldErr.Value(),
					Message: v.getErrorMessage(fieldErr),
				}
				result.Errors = append(result.Errors, validationError)
			}
		} else {
			result.Errors = append(result.Errors, ValidationError{
				Field:   "envelope",
				Tag:     "validation",
				Message: err.Error(),
			})
		}

		v.logger.Warn("Message validation failed",
			zap.String("eventType", string(envelope.EventType)),
			zap.String("actorId", envelope.ActorID),
			zap.Int("errorCount", len(result.Errors)),
			zap.Any("errors", result.Errors),
		)
		return result
	}

	// Validate payload structure based on event type
	if err := v.validatePayload(envelope); err != nil {
		v.metrics.InvalidPayload.Add(1)
		v.metrics.FailedValidations.Add(1)
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "payload",
			Tag:     "structure",
			Message: err.Error(),
		})

		v.logger.Warn("Payload validation failed",
			zap.String("eventType", string(envelope.EventType)),
			zap.Error(err),
		)
		return result
	}

	// Validation successful
	v.logger.Debug("Message validation passed",
		zap.String("eventType", string(envelope.EventType)),
		zap.String("version", string(envelope.Version)),
		zap.Int64("seq", envelope.Seq),
	)

	return result
}

// validatePayload validates the payload based on event type
func (v *MessageValidator) validatePayload(envelope *MessageEnvelope) error {
	switch envelope.EventType {
	case EventTypeCheckinCreated, EventTypeCheckinUpdated, EventTypeCheckinDeleted:
		var payload CheckinPayload
		if err := envelope.GetPayload(&payload); err != nil {
			return fmt.Errorf("failed to parse checkin payload: %w", err)
		}
		if err := v.validate.Struct(payload); err != nil {
			return fmt.Errorf("invalid checkin payload: %w", err)
		}

	case EventTypeRoomMessage:
		var payload RoomMessagePayload
		if err := envelope.GetPayload(&payload); err != nil {
			return fmt.Errorf("failed to parse room message payload: %w", err)
		}
		if err := v.validate.Struct(payload); err != nil {
			return fmt.Errorf("invalid room message payload: %w", err)
		}

	case EventTypePresenceUpdate, EventTypeStatusUpdate:
		var payload PresencePayload
		if err := envelope.GetPayload(&payload); err != nil {
			return fmt.Errorf("failed to parse presence payload: %w", err)
		}
		if err := v.validate.Struct(payload); err != nil {
			return fmt.Errorf("invalid presence payload: %w", err)
		}

	case EventTypeTypingStart, EventTypeTypingStop:
		var payload TypingPayload
		if err := envelope.GetPayload(&payload); err != nil {
			return fmt.Errorf("failed to parse typing payload: %w", err)
		}
		if err := v.validate.Struct(payload); err != nil {
			return fmt.Errorf("invalid typing payload: %w", err)
		}

	case EventTypeError:
		var payload ErrorPayload
		if err := envelope.GetPayload(&payload); err != nil {
			return fmt.Errorf("failed to parse error payload: %w", err)
		}
		if err := v.validate.Struct(payload); err != nil {
			return fmt.Errorf("invalid error payload: %w", err)
		}

	case EventTypePing, EventTypePong:
		// Ping/Pong messages don't require payload validation
		return nil

	default:
		// For unknown event types, just check that payload is valid JSON
		// This allows for forward compatibility
		v.logger.Debug("Unknown event type, skipping payload validation",
			zap.String("eventType", string(envelope.EventType)),
		)
	}

	return nil
}

// GetMetrics returns current validation metrics
func (v *MessageValidator) GetMetrics() map[string]uint64 {
	return map[string]uint64{
		"total_validations":  v.metrics.TotalValidations.Load(),
		"failed_validations": v.metrics.FailedValidations.Load(),
		"invalid_version":    v.metrics.InvalidVersion.Load(),
		"invalid_schema":     v.metrics.InvalidSchema.Load(),
		"missing_fields":     v.metrics.MissingFields.Load(),
		"invalid_payload":    v.metrics.InvalidPayload.Load(),
	}
}

// ResetMetrics resets all validation metrics
func (v *MessageValidator) ResetMetrics() {
	v.metrics.TotalValidations.Store(0)
	v.metrics.FailedValidations.Store(0)
	v.metrics.InvalidVersion.Store(0)
	v.metrics.InvalidSchema.Store(0)
	v.metrics.MissingFields.Store(0)
	v.metrics.InvalidPayload.Store(0)
}

// getErrorMessage returns a human-readable error message for a validation error
func (v *MessageValidator) getErrorMessage(fieldErr validator.FieldError) string {
	switch fieldErr.Tag() {
	case "required":
		return fmt.Sprintf("field '%s' is required", fieldErr.Field())
	case "min":
		return fmt.Sprintf("field '%s' must be at least %s", fieldErr.Field(), fieldErr.Param())
	case "max":
		return fmt.Sprintf("field '%s' must be at most %s", fieldErr.Field(), fieldErr.Param())
	case "uuid":
		return fmt.Sprintf("field '%s' must be a valid UUID", fieldErr.Field())
	case "oneof":
		return fmt.Sprintf("field '%s' must be one of: %s", fieldErr.Field(), fieldErr.Param())
	default:
		return fmt.Sprintf("field '%s' failed validation: %s", fieldErr.Field(), fieldErr.Tag())
	}
}

// Custom validators

func validateEventType(fl validator.FieldLevel) bool {
	eventType := EventType(fl.Field().String())
	validTypes := []EventType{
		EventTypeCheckinCreated, EventTypeCheckinUpdated, EventTypeCheckinDeleted,
		EventTypeTimelineUpdate, EventTypeRoomJoin, EventTypeRoomLeave,
		EventTypeRoomMessage, EventTypeTypingStart, EventTypeTypingStop,
		EventTypePresenceUpdate, EventTypeStatusUpdate,
		EventTypePing, EventTypePong, EventTypeError,
	}

	for _, valid := range validTypes {
		if eventType == valid {
			return true
		}
	}
	return false
}

func validateSchemaVersion(fl validator.FieldLevel) bool {
	version := SchemaVersion(fl.Field().String())
	registry := NewSchemaRegistry()
	return registry.IsVersionSupported(version)
}
