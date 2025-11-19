package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Service handles sync business logic
type Service struct {
	repo             *Repository
	conflictResolver *ConflictResolver
	checkinService   CheckinService
	logger           *zap.Logger
}

// CheckinService defines the interface for checkin operations
type CheckinService interface {
	CreateWithID(ctx context.Context, id uuid.UUID, userID uuid.UUID, data json.RawMessage) error
	UpdateWithVersion(ctx context.Context, id uuid.UUID, userID uuid.UUID, version int, data json.RawMessage) error
	Delete(ctx context.Context, id uuid.UUID, userID uuid.UUID) error
}

// NewService creates a new sync service
func NewService(
	repo *Repository,
	conflictResolver *ConflictResolver,
	checkinService CheckinService,
	logger *zap.Logger,
) *Service {
	return &Service{
		repo:             repo,
		conflictResolver: conflictResolver,
		checkinService:   checkinService,
		logger:           logger,
	}
}

// ProcessSyncRequest processes a batch sync request from a client
func (s *Service) ProcessSyncRequest(ctx context.Context, userID uuid.UUID, req *SyncRequest) (*SyncResponse, error) {
	startTime := time.Now()

	// Get or create sync status for this device
	syncStatus, err := s.repo.GetOrCreateSyncStatus(ctx, userID, req.DeviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get sync status: %w", err)
	}

	// Update sync status metadata
	now := time.Now()
	syncStatus.LastSyncAt = &now
	syncStatus.ClientVersion = &req.ClientVersion
	platform := req.Platform
	syncStatus.Platform = &platform

	// Process each operation
	results := make([]SyncOperationResult, 0, len(req.Operations))
	successCount := 0
	failureCount := 0
	conflictsCount := 0

	for _, op := range req.Operations {
		result := s.processOperation(ctx, userID, &op)
		results = append(results, result)

		switch result.Status {
		case "success":
			successCount++
		case "failed":
			failureCount++
		case "conflicted":
			conflictsCount++
		}
	}

	// Get server changes since last sync (delta sync)
	serverChanges, err := s.getServerChanges(ctx, userID, req.LastSyncAt)
	if err != nil {
		s.logger.Warn("Failed to get server changes",
			zap.Error(err),
			zap.String("user_id", userID.String()),
		)
		serverChanges = []ServerChange{} // Continue with empty changes
	}

	// Update sync status if all operations succeeded
	if failureCount == 0 && conflictsCount == 0 {
		syncStatus.LastSuccessfulSyncAt = &now
		syncStatus.LastKnownTimestamp = &now
	}

	if err := s.repo.UpdateSyncStatus(ctx, syncStatus); err != nil {
		s.logger.Warn("Failed to update sync status",
			zap.Error(err),
			zap.String("user_id", userID.String()),
		)
	}

	s.logger.Info("Sync request processed",
		zap.String("user_id", userID.String()),
		zap.String("device_id", req.DeviceID),
		zap.Int("operations", len(req.Operations)),
		zap.Int("success", successCount),
		zap.Int("failed", failureCount),
		zap.Int("conflicts", conflictsCount),
		zap.Duration("duration", time.Since(startTime)),
	)

	return &SyncResponse{
		Success:        failureCount == 0 && conflictsCount == 0,
		SyncedAt:       now,
		Results:        results,
		ServerChanges:  serverChanges,
		HasMoreChanges: false, // TODO: Implement pagination for large datasets
		ConflictsCount: conflictsCount,
		SuccessCount:   successCount,
		FailureCount:   failureCount,
	}, nil
}

// processOperation processes a single sync operation
func (s *Service) processOperation(ctx context.Context, userID uuid.UUID, op *SyncOperationData) SyncOperationResult {
	startTime := time.Now()

	// Check for existing operation with this idempotency key
	existing, err := s.repo.GetQueueItemByIdempotencyKey(ctx, userID, op.IdempotencyKey)
	if err != nil {
		s.logger.Error("Failed to check idempotency",
			zap.Error(err),
			zap.String("idempotency_key", op.IdempotencyKey),
		)
		errMsg := "internal error"
		return SyncOperationResult{
			IdempotencyKey: op.IdempotencyKey,
			ResourceID:     op.ResourceID,
			Status:         "failed",
			Error:          &errMsg,
		}
	}

	// If operation already exists and is completed, return cached result
	if existing != nil && existing.Status == StatusCompleted {
		return SyncOperationResult{
			IdempotencyKey: op.IdempotencyKey,
			ResourceID:     op.ResourceID,
			Status:         "success",
		}
	}

	// Detect conflicts
	conflict, err := s.conflictResolver.DetectConflict(ctx, op, userID)
	if err != nil {
		s.logger.Error("Failed to detect conflict",
			zap.Error(err),
			zap.String("resource_id", op.ResourceID.String()),
		)
		errMsg := "conflict detection failed"
		return SyncOperationResult{
			IdempotencyKey: op.IdempotencyKey,
			ResourceID:     op.ResourceID,
			Status:         "failed",
			Error:          &errMsg,
		}
	}

	// If conflict detected, apply resolution strategy
	if conflict != nil {
		shouldProceed, strategy, err := s.conflictResolver.ResolveWithStrategy(
			ctx,
			conflict,
			StrategyLastWriteWins, // Default strategy
		)

		if err != nil || !shouldProceed {
			// Log the conflict
			s.logOperation(ctx, userID, op, LogConflicted, &strategy, nil, time.Since(startTime))

			// Store conflict in queue for manual resolution
			queueItem := &SyncQueueItem{
				UserID:          userID,
				OperationType:   op.OperationType,
				ResourceType:    op.ResourceType,
				ResourceID:      op.ResourceID,
				IdempotencyKey:  op.IdempotencyKey,
				ClientTimestamp: op.ClientTimestamp,
				Status:          StatusConflicted,
				OperationData:   op.Data,
			}
			conflictData, _ := json.Marshal(conflict)
			queueItem.ConflictData = conflictData

			if err := s.repo.EnqueueOperation(ctx, queueItem); err != nil {
				s.logger.Error("Failed to enqueue conflicted operation",
					zap.Error(err),
					zap.String("idempotency_key", op.IdempotencyKey),
				)
			}

			return SyncOperationResult{
				IdempotencyKey:     op.IdempotencyKey,
				ResourceID:         op.ResourceID,
				Status:             "conflicted",
				ConflictInfo:       conflict,
				ResolutionStrategy: &strategy,
			}
		}
	}

	// Execute the operation
	err = s.executeOperation(ctx, userID, op)
	if err != nil {
		s.logger.Error("Failed to execute operation",
			zap.Error(err),
			zap.String("operation_type", string(op.OperationType)),
			zap.String("resource_id", op.ResourceID.String()),
		)

		// Log the failure
		s.logOperation(ctx, userID, op, LogFailed, nil, &err, time.Since(startTime))

		errMsg := err.Error()
		return SyncOperationResult{
			IdempotencyKey: op.IdempotencyKey,
			ResourceID:     op.ResourceID,
			Status:         "failed",
			Error:          &errMsg,
		}
	}

	// Log the success
	resolution := "last_write_wins"
	s.logOperation(ctx, userID, op, LogSuccess, &resolution, nil, time.Since(startTime))

	return SyncOperationResult{
		IdempotencyKey: op.IdempotencyKey,
		ResourceID:     op.ResourceID,
		Status:         "success",
	}
}

// executeOperation executes a sync operation
func (s *Service) executeOperation(ctx context.Context, userID uuid.UUID, op *SyncOperationData) error {
	switch op.ResourceType {
	case ResourceCheckin:
		return s.executeCheckinOperation(ctx, userID, op)
	case ResourceCategory:
		return s.executeCategoryOperation(ctx, userID, op)
	case ResourceTag:
		return s.executeTagOperation(ctx, userID, op)
	default:
		return fmt.Errorf("unsupported resource type: %s", op.ResourceType)
	}
}

// executeCheckinOperation executes a checkin operation
func (s *Service) executeCheckinOperation(ctx context.Context, userID uuid.UUID, op *SyncOperationData) error {
	switch op.OperationType {
	case OperationCreate:
		return s.checkinService.CreateWithID(ctx, op.ResourceID, userID, op.Data)

	case OperationUpdate:
		// Parse version from data
		var data struct {
			Version int `json:"version"`
		}
		if err := json.Unmarshal(op.Data, &data); err != nil {
			return fmt.Errorf("failed to parse version: %w", err)
		}
		return s.checkinService.UpdateWithVersion(ctx, op.ResourceID, userID, data.Version, op.Data)

	case OperationDelete:
		return s.checkinService.Delete(ctx, op.ResourceID, userID)

	default:
		return fmt.Errorf("unsupported operation type: %s", op.OperationType)
	}
}

// executeCategoryOperation executes a category operation (placeholder)
func (s *Service) executeCategoryOperation(ctx context.Context, userID uuid.UUID, op *SyncOperationData) error {
	// TODO: Implement category sync operations
	return fmt.Errorf("category sync not yet implemented")
}

// executeTagOperation executes a tag operation (placeholder)
func (s *Service) executeTagOperation(ctx context.Context, userID uuid.UUID, op *SyncOperationData) error {
	// TODO: Implement tag sync operations
	return fmt.Errorf("tag sync not yet implemented")
}

// getServerChanges retrieves changes from the server since last sync
func (s *Service) getServerChanges(ctx context.Context, userID uuid.UUID, since *time.Time) ([]ServerChange, error) {
	// TODO: Implement delta sync to get changes since last sync
	// This would query checkins, categories, tags that were updated since 'since' timestamp

	// For now, return empty changes
	return []ServerChange{}, nil
}

// logOperation creates a log entry for an operation
func (s *Service) logOperation(
	ctx context.Context,
	userID uuid.UUID,
	op *SyncOperationData,
	status LogStatus,
	strategy *string,
	err *error,
	duration time.Duration,
) {
	var errorMsg *string
	if err != nil {
		msg := (*err).Error()
		errorMsg = &msg
	}

	durationMs := int(duration.Milliseconds())

	log := &SyncOperationLog{
		UserID:               userID,
		OperationType:        op.OperationType,
		ResourceType:         op.ResourceType,
		ResourceID:           op.ResourceID,
		Status:               status,
		ErrorMessage:         errorMsg,
		ResolutionStrategy:   strategy,
		ClientTimestamp:      &op.ClientTimestamp,
		ProcessingDurationMs: &durationMs,
	}

	if err := s.repo.LogOperation(ctx, log); err != nil {
		s.logger.Warn("Failed to log operation",
			zap.Error(err),
			zap.String("operation_type", string(op.OperationType)),
		)
	}
}

// GetSyncStatus retrieves the sync status for a device
func (s *Service) GetSyncStatus(ctx context.Context, userID uuid.UUID, deviceID string) (*SyncStatusResponse, error) {
	status, err := s.repo.GetOrCreateSyncStatus(ctx, userID, deviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get sync status: %w", err)
	}

	// Check if full sync is needed (first sync or very old last sync)
	needsFullSync := status.LastSuccessfulSyncAt == nil
	if status.LastSuccessfulSyncAt != nil {
		// Full sync if last sync was more than 30 days ago
		if time.Since(*status.LastSuccessfulSyncAt) > 30*24*time.Hour {
			needsFullSync = true
		}
	}

	return &SyncStatusResponse{
		DeviceID:               deviceID,
		LastSyncAt:             status.LastSyncAt,
		LastSuccessfulSyncAt:   status.LastSuccessfulSyncAt,
		PendingOperationsCount: status.PendingOperationsCount,
		FailedOperationsCount:  status.FailedOperationsCount,
		IsUpToDate:             status.PendingOperationsCount == 0 && status.FailedOperationsCount == 0,
		NeedsFullSync:          needsFullSync,
	}, nil
}

// GetConflicts retrieves all conflicted operations for a user
func (s *Service) GetConflicts(ctx context.Context, userID uuid.UUID) ([]*SyncQueueItem, error) {
	return s.repo.GetConflictedOperations(ctx, userID)
}

// ResolveConflict manually resolves a conflict
func (s *Service) ResolveConflict(ctx context.Context, userID uuid.UUID, req *ConflictResolutionRequest) error {
	// Get the conflicted operation
	item, err := s.repo.GetQueueItemByIdempotencyKey(ctx, userID, req.IdempotencyKey)
	if err != nil {
		return fmt.Errorf("failed to get queue item: %w", err)
	}

	if item == nil {
		return fmt.Errorf("operation not found")
	}

	if item.Status != StatusConflicted {
		return fmt.Errorf("operation is not in conflicted state")
	}

	// Parse conflict info
	var conflict ConflictInfo
	if err := json.Unmarshal(item.ConflictData, &conflict); err != nil {
		return fmt.Errorf("failed to parse conflict data: %w", err)
	}

	// Validate resolution
	if err := s.conflictResolver.ValidateConflictResolution(&conflict, req.Resolution, req.Data); err != nil {
		return err
	}

	switch req.Resolution {
	case "accept_server":
		// Mark as completed (client accepted server version)
		return s.repo.UpdateQueueItemStatus(ctx, item.ID, StatusCompleted, nil, nil)

	case "force_client":
		// Execute the operation with client data
		op := &SyncOperationData{
			IdempotencyKey:  item.IdempotencyKey,
			OperationType:   item.OperationType,
			ResourceType:    item.ResourceType,
			ResourceID:      item.ResourceID,
			ClientTimestamp: item.ClientTimestamp,
			Data:            req.Data,
		}

		if err := s.executeOperation(ctx, userID, op); err != nil {
			errMsg := err.Error()
			return s.repo.UpdateQueueItemStatus(ctx, item.ID, StatusFailed, &errMsg, nil)
		}

		return s.repo.UpdateQueueItemStatus(ctx, item.ID, StatusCompleted, nil, nil)

	default:
		return fmt.Errorf("unsupported resolution: %s", req.Resolution)
	}
}

// CleanupOldOperations removes old completed sync operations
func (s *Service) CleanupOldOperations(ctx context.Context, retentionDays int) error {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	count, err := s.repo.DeleteCompletedOperations(ctx, cutoff)
	if err != nil {
		return fmt.Errorf("failed to cleanup old operations: %w", err)
	}

	s.logger.Info("Cleaned up old sync operations",
		zap.Int64("count", count),
		zap.Int("retention_days", retentionDays),
	)

	return nil
}
