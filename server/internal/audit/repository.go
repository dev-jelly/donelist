package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// Repository handles audit log database operations
type Repository struct {
	db *sqlx.DB
}

// NewRepository creates a new audit repository
func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

// Create creates a new audit log entry
func (r *Repository) Create(ctx context.Context, input CreateAuditLogInput) (*AuditLog, error) {
	// Convert details map to JSON
	var detailsJSON []byte
	var err error
	if input.Details != nil {
		detailsJSON, err = json.Marshal(input.Details)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal details: %w", err)
		}
	}

	query := `
		INSERT INTO audit_logs (
			event_type, severity, user_id, ip_address, user_agent,
			action, resource, resource_id, details, success,
			error_message, request_id, session_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, event_type, severity, user_id, ip_address, user_agent,
		          action, resource, resource_id, details, success,
		          error_message, request_id, session_id, created_at
	`

	var log AuditLog
	err = r.db.QueryRowContext(
		ctx, query,
		input.EventType, input.Severity, input.UserID, input.IPAddress, input.UserAgent,
		input.Action, input.Resource, input.ResourceID, detailsJSON, input.Success,
		input.ErrorMessage, input.RequestID, input.SessionID,
	).Scan(
		&log.ID, &log.EventType, &log.Severity, &log.UserID, &log.IPAddress, &log.UserAgent,
		&log.Action, &log.Resource, &log.ResourceID, &detailsJSON, &log.Success,
		&log.ErrorMessage, &log.RequestID, &log.SessionID, &log.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create audit log: %w", err)
	}

	// Unmarshal details
	if detailsJSON != nil {
		if err := json.Unmarshal(detailsJSON, &log.Details); err != nil {
			return nil, fmt.Errorf("failed to unmarshal details: %w", err)
		}
	}

	return &log, nil
}

// List retrieves audit logs based on options
func (r *Repository) List(ctx context.Context, opts ListAuditLogsOptions) ([]AuditLog, error) {
	query := `
		SELECT id, event_type, severity, user_id, ip_address, user_agent,
		       action, resource, resource_id, details, success,
		       error_message, request_id, session_id, created_at
		FROM audit_logs
		WHERE 1=1
	`

	args := []interface{}{}
	argIndex := 1

	if opts.UserID != nil {
		query += fmt.Sprintf(" AND user_id = $%d", argIndex)
		args = append(args, *opts.UserID)
		argIndex++
	}

	if opts.EventType != nil {
		query += fmt.Sprintf(" AND event_type = $%d", argIndex)
		args = append(args, *opts.EventType)
		argIndex++
	}

	if opts.Severity != nil {
		query += fmt.Sprintf(" AND severity = $%d", argIndex)
		args = append(args, *opts.Severity)
		argIndex++
	}

	if opts.StartDate != nil {
		query += fmt.Sprintf(" AND created_at >= $%d", argIndex)
		args = append(args, *opts.StartDate)
		argIndex++
	}

	if opts.EndDate != nil {
		query += fmt.Sprintf(" AND created_at <= $%d", argIndex)
		args = append(args, *opts.EndDate)
		argIndex++
	}

	if opts.IPAddress != nil {
		query += fmt.Sprintf(" AND ip_address = $%d", argIndex)
		args = append(args, *opts.IPAddress)
		argIndex++
	}

	if opts.Success != nil {
		query += fmt.Sprintf(" AND success = $%d", argIndex)
		args = append(args, *opts.Success)
		argIndex++
	}

	query += " ORDER BY created_at DESC"

	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argIndex)
		args = append(args, opts.Limit)
		argIndex++
	}

	if opts.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argIndex)
		args = append(args, opts.Offset)
		argIndex++
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list audit logs: %w", err)
	}
	defer rows.Close()

	var logs []AuditLog
	for rows.Next() {
		var log AuditLog
		var detailsJSON []byte

		err := rows.Scan(
			&log.ID, &log.EventType, &log.Severity, &log.UserID, &log.IPAddress, &log.UserAgent,
			&log.Action, &log.Resource, &log.ResourceID, &detailsJSON, &log.Success,
			&log.ErrorMessage, &log.RequestID, &log.SessionID, &log.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan audit log: %w", err)
		}

		// Unmarshal details
		if detailsJSON != nil {
			if err := json.Unmarshal(detailsJSON, &log.Details); err != nil {
				return nil, fmt.Errorf("failed to unmarshal details: %w", err)
			}
		}

		logs = append(logs, log)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating audit logs: %w", err)
	}

	return logs, nil
}

// GetByID retrieves an audit log by ID
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*AuditLog, error) {
	query := `
		SELECT id, event_type, severity, user_id, ip_address, user_agent,
		       action, resource, resource_id, details, success,
		       error_message, request_id, session_id, created_at
		FROM audit_logs
		WHERE id = $1
	`

	var log AuditLog
	var detailsJSON []byte

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&log.ID, &log.EventType, &log.Severity, &log.UserID, &log.IPAddress, &log.UserAgent,
		&log.Action, &log.Resource, &log.ResourceID, &detailsJSON, &log.Success,
		&log.ErrorMessage, &log.RequestID, &log.SessionID, &log.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("audit log not found")
		}
		return nil, fmt.Errorf("failed to get audit log: %w", err)
	}

	// Unmarshal details
	if detailsJSON != nil {
		if err := json.Unmarshal(detailsJSON, &log.Details); err != nil {
			return nil, fmt.Errorf("failed to unmarshal details: %w", err)
		}
	}

	return &log, nil
}

// GetStatistics retrieves audit statistics for a date range
func (r *Repository) GetStatistics(ctx context.Context, startDate, endDate time.Time) (*AuditStatistics, error) {
	query := `
		SELECT
			COUNT(*) as total_events,
			COUNT(*) FILTER (WHERE success = true) as successful_events,
			COUNT(*) FILTER (WHERE success = false) as failed_events,
			COUNT(DISTINCT user_id) as unique_users,
			COUNT(DISTINCT ip_address) as unique_ips
		FROM audit_logs
		WHERE created_at >= $1 AND created_at <= $2
	`

	var stats AuditStatistics
	err := r.db.QueryRowContext(ctx, query, startDate, endDate).Scan(
		&stats.TotalEvents,
		&stats.SuccessfulEvents,
		&stats.FailedEvents,
		&stats.UniqueUsers,
		&stats.UniqueIPAddresses,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get audit statistics: %w", err)
	}

	// Get events by type
	typeQuery := `
		SELECT event_type, COUNT(*) as count
		FROM audit_logs
		WHERE created_at >= $1 AND created_at <= $2
		GROUP BY event_type
	`

	rows, err := r.db.QueryContext(ctx, typeQuery, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get events by type: %w", err)
	}
	defer rows.Close()

	stats.EventsByType = make(map[EventType]int64)
	for rows.Next() {
		var eventType EventType
		var count int64
		if err := rows.Scan(&eventType, &count); err != nil {
			return nil, fmt.Errorf("failed to scan event type: %w", err)
		}
		stats.EventsByType[eventType] = count
	}

	// Get events by severity
	severityQuery := `
		SELECT severity, COUNT(*) as count
		FROM audit_logs
		WHERE created_at >= $1 AND created_at <= $2
		GROUP BY severity
	`

	rows, err = r.db.QueryContext(ctx, severityQuery, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get events by severity: %w", err)
	}
	defer rows.Close()

	stats.EventsBySeverity = make(map[Severity]int64)
	for rows.Next() {
		var severity Severity
		var count int64
		if err := rows.Scan(&severity, &count); err != nil {
			return nil, fmt.Errorf("failed to scan severity: %w", err)
		}
		stats.EventsBySeverity[severity] = count
	}

	return &stats, nil
}

// DeleteOldLogs deletes audit logs older than the specified retention period
func (r *Repository) DeleteOldLogs(ctx context.Context, retentionDays int) (int64, error) {
	cutoffDate := time.Now().AddDate(0, 0, -retentionDays)

	query := `
		DELETE FROM audit_logs
		WHERE created_at < $1
	`

	result, err := r.db.ExecContext(ctx, query, cutoffDate)
	if err != nil {
		return 0, fmt.Errorf("failed to delete old audit logs: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return rowsAffected, nil
}
