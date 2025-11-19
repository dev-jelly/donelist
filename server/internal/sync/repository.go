package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// Repository handles sync-related database operations
type Repository struct {
	db *sqlx.DB
}

// NewRepository creates a new sync repository
func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

// --- Sync Queue Operations ---

// EnqueueOperation adds a new operation to the sync queue
func (r *Repository) EnqueueOperation(ctx context.Context, item *SyncQueueItem) error {
	query := `
		INSERT INTO sync_queue (
			user_id, operation_type, resource_type, resource_id,
			idempotency_key, client_timestamp, status, operation_data
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (user_id, idempotency_key) DO NOTHING
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRowxContext(
		ctx, query,
		item.UserID, item.OperationType, item.ResourceType, item.ResourceID,
		item.IdempotencyKey, item.ClientTimestamp, StatusPending, item.OperationData,
	).Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt)

	if err == sql.ErrNoRows {
		// Operation with this idempotency key already exists
		return nil
	}

	return err
}

// GetPendingOperations retrieves pending operations for a user
func (r *Repository) GetPendingOperations(ctx context.Context, userID uuid.UUID, limit int) ([]*SyncQueueItem, error) {
	query := `
		SELECT id, user_id, operation_type, resource_type, resource_id,
		       idempotency_key, client_timestamp, status, retry_count,
		       last_retry_at, error_message, operation_data, conflict_data,
		       created_at, updated_at, completed_at
		FROM sync_queue
		WHERE user_id = $1 AND status = $2
		ORDER BY client_timestamp ASC, created_at ASC
		LIMIT $3
	`

	var items []*SyncQueueItem
	err := r.db.SelectContext(ctx, &items, query, userID, StatusPending, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending operations: %w", err)
	}

	return items, nil
}

// GetQueueItemByIdempotencyKey retrieves a queue item by idempotency key
func (r *Repository) GetQueueItemByIdempotencyKey(ctx context.Context, userID uuid.UUID, key string) (*SyncQueueItem, error) {
	query := `
		SELECT id, user_id, operation_type, resource_type, resource_id,
		       idempotency_key, client_timestamp, status, retry_count,
		       last_retry_at, error_message, operation_data, conflict_data,
		       created_at, updated_at, completed_at
		FROM sync_queue
		WHERE user_id = $1 AND idempotency_key = $2
	`

	var item SyncQueueItem
	err := r.db.GetContext(ctx, &item, query, userID, key)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get queue item: %w", err)
	}

	return &item, nil
}

// UpdateQueueItemStatus updates the status of a queue item
func (r *Repository) UpdateQueueItemStatus(
	ctx context.Context,
	id uuid.UUID,
	status QueueStatus,
	errorMsg *string,
	conflictData json.RawMessage,
) error {
	query := `
		UPDATE sync_queue
		SET status = $2,
		    error_message = $3,
		    conflict_data = $4,
		    completed_at = CASE WHEN $2 IN ('completed', 'failed') THEN CURRENT_TIMESTAMP ELSE NULL END,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`

	_, err := r.db.ExecContext(ctx, query, id, status, errorMsg, conflictData)
	if err != nil {
		return fmt.Errorf("failed to update queue item status: %w", err)
	}

	return nil
}

// IncrementRetryCount increments the retry count for a queue item
func (r *Repository) IncrementRetryCount(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE sync_queue
		SET retry_count = retry_count + 1,
		    last_retry_at = CURRENT_TIMESTAMP,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`

	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to increment retry count: %w", err)
	}

	return nil
}

// DeleteCompletedOperations removes old completed operations
func (r *Repository) DeleteCompletedOperations(ctx context.Context, olderThan time.Time) (int64, error) {
	query := `
		DELETE FROM sync_queue
		WHERE status = $1 AND completed_at < $2
	`

	result, err := r.db.ExecContext(ctx, query, StatusCompleted, olderThan)
	if err != nil {
		return 0, fmt.Errorf("failed to delete completed operations: %w", err)
	}

	count, _ := result.RowsAffected()
	return count, nil
}

// --- Sync Status Operations ---

// GetOrCreateSyncStatus gets or creates a sync status for a device
func (r *Repository) GetOrCreateSyncStatus(ctx context.Context, userID uuid.UUID, deviceID string) (*SyncStatus, error) {
	// Try to get existing status
	query := `
		SELECT id, user_id, device_id, last_sync_at, last_successful_sync_at,
		       pending_operations_count, failed_operations_count,
		       last_known_checkin_id, last_known_timestamp,
		       client_version, platform, created_at, updated_at
		FROM sync_status
		WHERE user_id = $1 AND device_id = $2
	`

	var status SyncStatus
	err := r.db.GetContext(ctx, &status, query, userID, deviceID)
	if err == nil {
		return &status, nil
	}

	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get sync status: %w", err)
	}

	// Create new status
	insertQuery := `
		INSERT INTO sync_status (user_id, device_id)
		VALUES ($1, $2)
		RETURNING id, user_id, device_id, last_sync_at, last_successful_sync_at,
		          pending_operations_count, failed_operations_count,
		          last_known_checkin_id, last_known_timestamp,
		          client_version, platform, created_at, updated_at
	`

	err = r.db.GetContext(ctx, &status, insertQuery, userID, deviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to create sync status: %w", err)
	}

	return &status, nil
}

// UpdateSyncStatus updates the sync status after a sync operation
func (r *Repository) UpdateSyncStatus(ctx context.Context, status *SyncStatus) error {
	query := `
		UPDATE sync_status
		SET last_sync_at = $3,
		    last_successful_sync_at = $4,
		    last_known_checkin_id = $5,
		    last_known_timestamp = $6,
		    client_version = $7,
		    platform = $8,
		    updated_at = CURRENT_TIMESTAMP
		WHERE user_id = $1 AND device_id = $2
	`

	_, err := r.db.ExecContext(
		ctx, query,
		status.UserID, status.DeviceID,
		status.LastSyncAt, status.LastSuccessfulSyncAt,
		status.LastKnownCheckinID, status.LastKnownTimestamp,
		status.ClientVersion, status.Platform,
	)

	if err != nil {
		return fmt.Errorf("failed to update sync status: %w", err)
	}

	return nil
}

// --- Idempotency Token Operations ---

// SaveIdempotencyToken saves an idempotency token with cached response
func (r *Repository) SaveIdempotencyToken(ctx context.Context, token *IdempotencyToken) error {
	query := `
		INSERT INTO idempotency_tokens (
			user_id, token, operation_type, resource_type,
			response_status, response_body, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id, token) DO UPDATE
		SET response_status = EXCLUDED.response_status,
		    response_body = EXCLUDED.response_body
		RETURNING id, created_at
	`

	return r.db.QueryRowxContext(
		ctx, query,
		token.UserID, token.Token, token.OperationType, token.ResourceType,
		token.ResponseStatus, token.ResponseBody, token.ExpiresAt,
	).Scan(&token.ID, &token.CreatedAt)
}

// GetIdempotencyToken retrieves an idempotency token
func (r *Repository) GetIdempotencyToken(ctx context.Context, userID uuid.UUID, token string) (*IdempotencyToken, error) {
	query := `
		SELECT id, user_id, token, operation_type, resource_type,
		       response_status, response_body, created_at, expires_at
		FROM idempotency_tokens
		WHERE user_id = $1 AND token = $2 AND expires_at > CURRENT_TIMESTAMP
	`

	var idempToken IdempotencyToken
	err := r.db.GetContext(ctx, &idempToken, query, userID, token)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get idempotency token: %w", err)
	}

	return &idempToken, nil
}

// DeleteExpiredTokens removes expired idempotency tokens
func (r *Repository) DeleteExpiredTokens(ctx context.Context) (int64, error) {
	query := `DELETE FROM idempotency_tokens WHERE expires_at < CURRENT_TIMESTAMP`

	result, err := r.db.ExecContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired tokens: %w", err)
	}

	count, _ := result.RowsAffected()
	return count, nil
}

// --- Operation Log ---

// LogOperation creates a log entry for a sync operation
func (r *Repository) LogOperation(ctx context.Context, log *SyncOperationLog) error {
	query := `
		INSERT INTO sync_operation_log (
			user_id, sync_queue_id, operation_type, resource_type, resource_id,
			status, error_message, resolution_strategy,
			client_timestamp, server_timestamp, processing_duration_ms
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, created_at
	`

	return r.db.QueryRowxContext(
		ctx, query,
		log.UserID, log.SyncQueueID, log.OperationType, log.ResourceType, log.ResourceID,
		log.Status, log.ErrorMessage, log.ResolutionStrategy,
		log.ClientTimestamp, log.ServerTimestamp, log.ProcessingDurationMs,
	).Scan(&log.ID, &log.CreatedAt)
}

// GetOperationLogs retrieves operation logs for a user
func (r *Repository) GetOperationLogs(ctx context.Context, userID uuid.UUID, limit int, offset int) ([]*SyncOperationLog, error) {
	query := `
		SELECT id, user_id, sync_queue_id, operation_type, resource_type, resource_id,
		       status, error_message, resolution_strategy,
		       client_timestamp, server_timestamp, processing_duration_ms, created_at
		FROM sync_operation_log
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	var logs []*SyncOperationLog
	err := r.db.SelectContext(ctx, &logs, query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get operation logs: %w", err)
	}

	return logs, nil
}

// GetConflictedOperations retrieves operations with conflicts
func (r *Repository) GetConflictedOperations(ctx context.Context, userID uuid.UUID) ([]*SyncQueueItem, error) {
	query := `
		SELECT id, user_id, operation_type, resource_type, resource_id,
		       idempotency_key, client_timestamp, status, retry_count,
		       last_retry_at, error_message, operation_data, conflict_data,
		       created_at, updated_at, completed_at
		FROM sync_queue
		WHERE user_id = $1 AND status = $2
		ORDER BY created_at ASC
	`

	var items []*SyncQueueItem
	err := r.db.SelectContext(ctx, &items, query, userID, StatusConflicted)
	if err != nil {
		return nil, fmt.Errorf("failed to get conflicted operations: %w", err)
	}

	return items, nil
}
