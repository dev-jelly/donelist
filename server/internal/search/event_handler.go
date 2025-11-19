package search

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// EventHandler handles checkin events for search indexing
type EventHandler struct {
	indexer *Indexer
	logger  *zap.Logger
}

// NewEventHandler creates a new event handler
func NewEventHandler(indexer *Indexer, logger *zap.Logger) *EventHandler {
	return &EventHandler{
		indexer: indexer,
		logger:  logger,
	}
}

// OnCheckinCreated handles checkin creation events
func (h *EventHandler) OnCheckinCreated(ctx context.Context, checkinID, userID uuid.UUID, data map[string]interface{}) error {
	h.logger.Debug("Handling checkin created event",
		zap.String("checkin_id", checkinID.String()),
		zap.String("user_id", userID.String()),
	)

	return h.indexer.HandleCheckinEvent(ctx, IndexOperationCreate, checkinID, userID, data)
}

// OnCheckinUpdated handles checkin update events
func (h *EventHandler) OnCheckinUpdated(ctx context.Context, checkinID, userID uuid.UUID, data map[string]interface{}) error {
	h.logger.Debug("Handling checkin updated event",
		zap.String("checkin_id", checkinID.String()),
		zap.String("user_id", userID.String()),
	)

	return h.indexer.HandleCheckinEvent(ctx, IndexOperationUpdate, checkinID, userID, data)
}

// OnCheckinDeleted handles checkin deletion events
func (h *EventHandler) OnCheckinDeleted(ctx context.Context, checkinID, userID uuid.UUID) error {
	h.logger.Debug("Handling checkin deleted event",
		zap.String("checkin_id", checkinID.String()),
		zap.String("user_id", userID.String()),
	)

	return h.indexer.HandleCheckinEvent(ctx, IndexOperationDelete, checkinID, userID, nil)
}
