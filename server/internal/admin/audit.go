package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// AuditLog represents an admin action audit log entry
type AuditLog struct {
	ID             uuid.UUID       `db:"id" json:"id"`
	AdminID        uuid.UUID       `db:"admin_id" json:"admin_id"`
	Action         string          `db:"action" json:"action"`
	ResourceType   string          `db:"resource_type" json:"resource_type"`
	ResourceID     *uuid.UUID      `db:"resource_id" json:"resource_id,omitempty"`
	TargetUserID   *uuid.UUID      `db:"target_user_id" json:"target_user_id,omitempty"`
	IPAddress      *string         `db:"ip_address" json:"ip_address,omitempty"`
	UserAgent      *string         `db:"user_agent" json:"user_agent,omitempty"`
	RequestPath    *string         `db:"request_path" json:"request_path,omitempty"`
	RequestMethod  *string         `db:"request_method" json:"request_method,omitempty"`
	RequestBody    json.RawMessage `db:"request_body" json:"request_body,omitempty"`
	ResponseStatus *int            `db:"response_status" json:"response_status,omitempty"`
	Metadata       json.RawMessage `db:"metadata" json:"metadata,omitempty"`
	CreatedAt      time.Time       `db:"created_at" json:"created_at"`
}

// AuditLogInput represents input for creating an audit log
type AuditLogInput struct {
	AdminID        uuid.UUID
	Action         string
	ResourceType   string
	ResourceID     *uuid.UUID
	TargetUserID   *uuid.UUID
	IPAddress      *string
	UserAgent      *string
	RequestPath    *string
	RequestMethod  *string
	RequestBody    interface{}
	ResponseStatus *int
	Metadata       interface{}
}

// AuditRepository handles admin audit log database operations
type AuditRepository struct {
	db *sqlx.DB
}

// NewAuditRepository creates a new audit repository
func NewAuditRepository(db *sqlx.DB) *AuditRepository {
	return &AuditRepository{db: db}
}

// Log creates a new audit log entry
func (r *AuditRepository) Log(ctx context.Context, input AuditLogInput) error {
	query := `
		INSERT INTO admin_audit_log (
			admin_id, action, resource_type, resource_id, target_user_id,
			ip_address, user_agent, request_path, request_method,
			request_body, response_status, metadata
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
		)
	`

	// Marshal request body if provided
	var requestBodyJSON []byte
	if input.RequestBody != nil {
		var err error
		requestBodyJSON, err = json.Marshal(input.RequestBody)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
	}

	// Marshal metadata if provided
	var metadataJSON []byte
	if input.Metadata != nil {
		var err error
		metadataJSON, err = json.Marshal(input.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	_, err := r.db.ExecContext(
		ctx,
		query,
		input.AdminID,
		input.Action,
		input.ResourceType,
		input.ResourceID,
		input.TargetUserID,
		input.IPAddress,
		input.UserAgent,
		input.RequestPath,
		input.RequestMethod,
		requestBodyJSON,
		input.ResponseStatus,
		metadataJSON,
	)

	if err != nil {
		return fmt.Errorf("failed to create audit log: %w", err)
	}

	return nil
}

// GetByAdminID retrieves audit logs for a specific admin
func (r *AuditRepository) GetByAdminID(ctx context.Context, adminID uuid.UUID, limit, offset int) ([]AuditLog, error) {
	query := `
		SELECT
			id, admin_id, action, resource_type, resource_id, target_user_id,
			ip_address, user_agent, request_path, request_method,
			request_body, response_status, metadata, created_at
		FROM admin_audit_log
		WHERE admin_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	var logs []AuditLog
	err := r.db.SelectContext(ctx, &logs, query, adminID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get audit logs: %w", err)
	}

	return logs, nil
}

// GetByResourceID retrieves audit logs for a specific resource
func (r *AuditRepository) GetByResourceID(ctx context.Context, resourceType string, resourceID uuid.UUID, limit, offset int) ([]AuditLog, error) {
	query := `
		SELECT
			id, admin_id, action, resource_type, resource_id, target_user_id,
			ip_address, user_agent, request_path, request_method,
			request_body, response_status, metadata, created_at
		FROM admin_audit_log
		WHERE resource_type = $1 AND resource_id = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`

	var logs []AuditLog
	err := r.db.SelectContext(ctx, &logs, query, resourceType, resourceID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get audit logs: %w", err)
	}

	return logs, nil
}

// GetByTargetUserID retrieves audit logs affecting a specific user
func (r *AuditRepository) GetByTargetUserID(ctx context.Context, targetUserID uuid.UUID, limit, offset int) ([]AuditLog, error) {
	query := `
		SELECT
			id, admin_id, action, resource_type, resource_id, target_user_id,
			ip_address, user_agent, request_path, request_method,
			request_body, response_status, metadata, created_at
		FROM admin_audit_log
		WHERE target_user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	var logs []AuditLog
	err := r.db.SelectContext(ctx, &logs, query, targetUserID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get audit logs: %w", err)
	}

	return logs, nil
}

// GetByAction retrieves audit logs for a specific action
func (r *AuditRepository) GetByAction(ctx context.Context, action string, limit, offset int) ([]AuditLog, error) {
	query := `
		SELECT
			id, admin_id, action, resource_type, resource_id, target_user_id,
			ip_address, user_agent, request_path, request_method,
			request_body, response_status, metadata, created_at
		FROM admin_audit_log
		WHERE action = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	var logs []AuditLog
	err := r.db.SelectContext(ctx, &logs, query, action, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get audit logs: %w", err)
	}

	return logs, nil
}

// GetByDateRange retrieves audit logs within a date range
func (r *AuditRepository) GetByDateRange(ctx context.Context, startDate, endDate time.Time, limit, offset int) ([]AuditLog, error) {
	query := `
		SELECT
			id, admin_id, action, resource_type, resource_id, target_user_id,
			ip_address, user_agent, request_path, request_method,
			request_body, response_status, metadata, created_at
		FROM admin_audit_log
		WHERE created_at >= $1 AND created_at <= $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`

	var logs []AuditLog
	err := r.db.SelectContext(ctx, &logs, query, startDate, endDate, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get audit logs: %w", err)
	}

	return logs, nil
}

// GetByID retrieves a specific audit log by ID
func (r *AuditRepository) GetByID(ctx context.Context, id uuid.UUID) (*AuditLog, error) {
	query := `
		SELECT
			id, admin_id, action, resource_type, resource_id, target_user_id,
			ip_address, user_agent, request_path, request_method,
			request_body, response_status, metadata, created_at
		FROM admin_audit_log
		WHERE id = $1
	`

	var log AuditLog
	err := r.db.GetContext(ctx, &log, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("audit log not found")
		}
		return nil, fmt.Errorf("failed to get audit log: %w", err)
	}

	return &log, nil
}

// CountByAdminID counts audit logs for a specific admin
func (r *AuditRepository) CountByAdminID(ctx context.Context, adminID uuid.UUID) (int, error) {
	query := `SELECT COUNT(*) FROM admin_audit_log WHERE admin_id = $1`

	var count int
	err := r.db.GetContext(ctx, &count, query, adminID)
	if err != nil {
		return 0, fmt.Errorf("failed to count audit logs: %w", err)
	}

	return count, nil
}

// CountByAction counts audit logs for a specific action
func (r *AuditRepository) CountByAction(ctx context.Context, action string) (int, error) {
	query := `SELECT COUNT(*) FROM admin_audit_log WHERE action = $1`

	var count int
	err := r.db.GetContext(ctx, &count, query, action)
	if err != nil {
		return 0, fmt.Errorf("failed to count audit logs: %w", err)
	}

	return count, nil
}
