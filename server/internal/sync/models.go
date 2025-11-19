package sync

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// OperationType defines the type of sync operation
type OperationType string

const (
	OperationCreate OperationType = "create"
	OperationUpdate OperationType = "update"
	OperationDelete OperationType = "delete"
)

// ResourceType defines the type of resource being synced
type ResourceType string

const (
	ResourceCheckin  ResourceType = "checkin"
	ResourceCategory ResourceType = "category"
	ResourceTag      ResourceType = "tag"
)

// QueueStatus defines the status of a sync queue item
type QueueStatus string

const (
	StatusPending    QueueStatus = "pending"
	StatusProcessing QueueStatus = "processing"
	StatusCompleted  QueueStatus = "completed"
	StatusFailed     QueueStatus = "failed"
	StatusConflicted QueueStatus = "conflicted"
)

// LogStatus defines the status of a sync operation log entry
type LogStatus string

const (
	LogSuccess    LogStatus = "success"
	LogFailed     LogStatus = "failed"
	LogConflicted LogStatus = "conflicted"
	LogSkipped    LogStatus = "skipped"
)

// Platform defines the client platform
type Platform string

const (
	PlatformIOS     Platform = "ios"
	PlatformAndroid Platform = "android"
	PlatformWeb     Platform = "web"
)

// SyncQueueItem represents a pending sync operation
type SyncQueueItem struct {
	ID              uuid.UUID       `db:"id" json:"id"`
	UserID          uuid.UUID       `db:"user_id" json:"user_id"`
	OperationType   OperationType   `db:"operation_type" json:"operation_type"`
	ResourceType    ResourceType    `db:"resource_type" json:"resource_type"`
	ResourceID      uuid.UUID       `db:"resource_id" json:"resource_id"`
	IdempotencyKey  string          `db:"idempotency_key" json:"idempotency_key"`
	ClientTimestamp time.Time       `db:"client_timestamp" json:"client_timestamp"`
	Status          QueueStatus     `db:"status" json:"status"`
	RetryCount      int             `db:"retry_count" json:"retry_count"`
	LastRetryAt     *time.Time      `db:"last_retry_at" json:"last_retry_at,omitempty"`
	ErrorMessage    *string         `db:"error_message" json:"error_message,omitempty"`
	OperationData   json.RawMessage `db:"operation_data" json:"operation_data"`
	ConflictData    json.RawMessage `db:"conflict_data" json:"conflict_data,omitempty"`
	CreatedAt       time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time       `db:"updated_at" json:"updated_at"`
	CompletedAt     *time.Time      `db:"completed_at" json:"completed_at,omitempty"`
}

// SyncStatus tracks the sync status for a device
type SyncStatus struct {
	ID                      uuid.UUID  `db:"id" json:"id"`
	UserID                  uuid.UUID  `db:"user_id" json:"user_id"`
	DeviceID                string     `db:"device_id" json:"device_id"`
	LastSyncAt              *time.Time `db:"last_sync_at" json:"last_sync_at,omitempty"`
	LastSuccessfulSyncAt    *time.Time `db:"last_successful_sync_at" json:"last_successful_sync_at,omitempty"`
	PendingOperationsCount  int        `db:"pending_operations_count" json:"pending_operations_count"`
	FailedOperationsCount   int        `db:"failed_operations_count" json:"failed_operations_count"`
	LastKnownCheckinID      *uuid.UUID `db:"last_known_checkin_id" json:"last_known_checkin_id,omitempty"`
	LastKnownTimestamp      *time.Time `db:"last_known_timestamp" json:"last_known_timestamp,omitempty"`
	ClientVersion           *string    `db:"client_version" json:"client_version,omitempty"`
	Platform                *Platform  `db:"platform" json:"platform,omitempty"`
	CreatedAt               time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt               time.Time  `db:"updated_at" json:"updated_at"`
}

// SyncOperationLog is an audit trail for sync operations
type SyncOperationLog struct {
	ID                   uuid.UUID     `db:"id" json:"id"`
	UserID               uuid.UUID     `db:"user_id" json:"user_id"`
	SyncQueueID          *uuid.UUID    `db:"sync_queue_id" json:"sync_queue_id,omitempty"`
	OperationType        OperationType `db:"operation_type" json:"operation_type"`
	ResourceType         ResourceType  `db:"resource_type" json:"resource_type"`
	ResourceID           uuid.UUID     `db:"resource_id" json:"resource_id"`
	Status               LogStatus     `db:"status" json:"status"`
	ErrorMessage         *string       `db:"error_message" json:"error_message,omitempty"`
	ResolutionStrategy   *string       `db:"resolution_strategy" json:"resolution_strategy,omitempty"`
	ClientTimestamp      *time.Time    `db:"client_timestamp" json:"client_timestamp,omitempty"`
	ServerTimestamp      time.Time     `db:"server_timestamp" json:"server_timestamp"`
	ProcessingDurationMs *int          `db:"processing_duration_ms" json:"processing_duration_ms,omitempty"`
	CreatedAt            time.Time     `db:"created_at" json:"created_at"`
}

// IdempotencyToken stores operation tokens to prevent duplicates
type IdempotencyToken struct {
	ID             uuid.UUID       `db:"id" json:"id"`
	UserID         uuid.UUID       `db:"user_id" json:"user_id"`
	Token          string          `db:"token" json:"token"`
	OperationType  OperationType   `db:"operation_type" json:"operation_type"`
	ResourceType   ResourceType    `db:"resource_type" json:"resource_type"`
	ResponseStatus *int            `db:"response_status" json:"response_status,omitempty"`
	ResponseBody   json.RawMessage `db:"response_body" json:"response_body,omitempty"`
	CreatedAt      time.Time       `db:"created_at" json:"created_at"`
	ExpiresAt      time.Time       `db:"expires_at" json:"expires_at"`
}

// SyncRequest represents a batch sync request from client
type SyncRequest struct {
	DeviceID      string              `json:"device_id" binding:"required"`
	ClientVersion string              `json:"client_version"`
	Platform      Platform            `json:"platform"`
	LastSyncAt    *time.Time          `json:"last_sync_at,omitempty"`
	Operations    []SyncOperationData `json:"operations" binding:"required,dive"`
}

// SyncOperationData represents a single operation in a sync request
type SyncOperationData struct {
	IdempotencyKey  string          `json:"idempotency_key" binding:"required"`
	OperationType   OperationType   `json:"operation_type" binding:"required,oneof=create update delete"`
	ResourceType    ResourceType    `json:"resource_type" binding:"required,oneof=checkin category tag"`
	ResourceID      uuid.UUID       `json:"resource_id" binding:"required"`
	ClientTimestamp time.Time       `json:"client_timestamp" binding:"required"`
	Data            json.RawMessage `json:"data" binding:"required"`
}

// SyncResponse represents the response to a sync request
type SyncResponse struct {
	Success          bool                  `json:"success"`
	SyncedAt         time.Time             `json:"synced_at"`
	Results          []SyncOperationResult `json:"results"`
	ServerChanges    []ServerChange        `json:"server_changes"`
	NextSyncToken    string                `json:"next_sync_token,omitempty"`
	HasMoreChanges   bool                  `json:"has_more_changes"`
	ConflictsCount   int                   `json:"conflicts_count"`
	SuccessCount     int                   `json:"success_count"`
	FailureCount     int                   `json:"failure_count"`
}

// SyncOperationResult represents the result of processing a single operation
type SyncOperationResult struct {
	IdempotencyKey     string        `json:"idempotency_key"`
	ResourceID         uuid.UUID     `json:"resource_id"`
	Status             string        `json:"status"` // success, failed, conflicted, skipped
	Error              *string       `json:"error,omitempty"`
	ConflictInfo       *ConflictInfo `json:"conflict_info,omitempty"`
	ResolutionStrategy *string       `json:"resolution_strategy,omitempty"`
}

// ConflictInfo contains information about a conflict
type ConflictInfo struct {
	Type             string          `json:"type"` // version_mismatch, concurrent_edit, deleted
	ClientVersion    *int            `json:"client_version,omitempty"`
	ServerVersion    *int            `json:"server_version,omitempty"`
	ClientTimestamp  time.Time       `json:"client_timestamp"`
	ServerTimestamp  time.Time       `json:"server_timestamp"`
	ServerData       json.RawMessage `json:"server_data,omitempty"`
	RecommendedAction string         `json:"recommended_action"` // accept_server, force_client, manual_merge
}

// ServerChange represents a change that happened on the server
type ServerChange struct {
	OperationType   OperationType   `json:"operation_type"`
	ResourceType    ResourceType    `json:"resource_type"`
	ResourceID      uuid.UUID       `json:"resource_id"`
	Data            json.RawMessage `json:"data"`
	ServerTimestamp time.Time       `json:"server_timestamp"`
	Version         *int            `json:"version,omitempty"`
}

// SyncStatusResponse represents the current sync status
type SyncStatusResponse struct {
	DeviceID               string     `json:"device_id"`
	LastSyncAt             *time.Time `json:"last_sync_at,omitempty"`
	LastSuccessfulSyncAt   *time.Time `json:"last_successful_sync_at,omitempty"`
	PendingOperationsCount int        `json:"pending_operations_count"`
	FailedOperationsCount  int        `json:"failed_operations_count"`
	IsUpToDate             bool       `json:"is_up_to_date"`
	NeedsFullSync          bool       `json:"needs_full_sync"`
}

// ConflictResolutionRequest represents a manual conflict resolution
type ConflictResolutionRequest struct {
	IdempotencyKey string          `json:"idempotency_key" binding:"required"`
	Resolution     string          `json:"resolution" binding:"required,oneof=accept_server force_client"`
	Data           json.RawMessage `json:"data,omitempty"` // Only needed for force_client
}
