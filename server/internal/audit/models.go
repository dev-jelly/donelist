package audit

import (
	"time"

	"github.com/google/uuid"
)

// EventType represents the type of audit event
type EventType string

const (
	// Authentication events
	EventTypeLoginSuccess      EventType = "auth.login.success"
	EventTypeLoginFailed       EventType = "auth.login.failed"
	EventTypeLogout            EventType = "auth.logout"
	EventTypeLogoutAll         EventType = "auth.logout_all"
	EventTypeRegister          EventType = "auth.register"
	EventTypeRefreshToken      EventType = "auth.refresh_token"
	EventTypePasswordChange    EventType = "auth.password_change"

	// User events
	EventTypeUserCreate        EventType = "user.create"
	EventTypeUserUpdate        EventType = "user.update"
	EventTypeUserDelete        EventType = "user.delete"
	EventTypeUserTierUpdate    EventType = "user.tier_update"

	// Data events
	EventTypeCheckinCreate     EventType = "checkin.create"
	EventTypeCheckinUpdate     EventType = "checkin.update"
	EventTypeCheckinDelete     EventType = "checkin.delete"
	EventTypeCategoryCreate    EventType = "category.create"
	EventTypeCategoryUpdate    EventType = "category.update"
	EventTypeCategoryDelete    EventType = "category.delete"

	// Security events
	EventTypeCSRFValidationFailed    EventType = "security.csrf_failed"
	EventTypeRateLimitExceeded       EventType = "security.rate_limit_exceeded"
	EventTypeInvalidInput            EventType = "security.invalid_input"
	EventTypeUnauthorizedAccess      EventType = "security.unauthorized_access"
	EventTypeSuspiciousActivity      EventType = "security.suspicious_activity"
	EventTypeAccountLocked           EventType = "security.account_locked"
	EventTypeAccountUnlocked         EventType = "security.account_unlocked"

	// Admin events
	EventTypeAdminAction      EventType = "admin.action"
	EventTypeConfigChange     EventType = "admin.config_change"

	// System events
	EventTypeSystemError      EventType = "system.error"
	EventTypeSystemWarning    EventType = "system.warning"
)

// Severity represents the severity level of an audit event
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityError    Severity = "error"
	SeverityCritical Severity = "critical"
)

// AuditLog represents an audit log entry
type AuditLog struct {
	ID            uuid.UUID         `db:"id" json:"id"`
	EventType     EventType         `db:"event_type" json:"event_type"`
	Severity      Severity          `db:"severity" json:"severity"`
	UserID        *uuid.UUID        `db:"user_id" json:"user_id,omitempty"`
	IPAddress     string            `db:"ip_address" json:"ip_address"`
	UserAgent     string            `db:"user_agent" json:"user_agent"`
	Action        string            `db:"action" json:"action"`
	Resource      string            `db:"resource" json:"resource,omitempty"`
	ResourceID    *uuid.UUID        `db:"resource_id" json:"resource_id,omitempty"`
	Details       map[string]interface{} `db:"details" json:"details,omitempty"`
	Success       bool              `db:"success" json:"success"`
	ErrorMessage  *string           `db:"error_message" json:"error_message,omitempty"`
	RequestID     string            `db:"request_id" json:"request_id,omitempty"`
	SessionID     string            `db:"session_id" json:"session_id,omitempty"`
	CreatedAt     time.Time         `db:"created_at" json:"created_at"`
}

// CreateAuditLogInput represents input for creating an audit log
type CreateAuditLogInput struct {
	EventType    EventType
	Severity     Severity
	UserID       *uuid.UUID
	IPAddress    string
	UserAgent    string
	Action       string
	Resource     string
	ResourceID   *uuid.UUID
	Details      map[string]interface{}
	Success      bool
	ErrorMessage *string
	RequestID    string
	SessionID    string
}

// ListAuditLogsOptions represents options for listing audit logs
type ListAuditLogsOptions struct {
	UserID     *uuid.UUID
	EventType  *EventType
	Severity   *Severity
	StartDate  *time.Time
	EndDate    *time.Time
	IPAddress  *string
	Success    *bool
	Limit      int
	Offset     int
}

// AuditStatistics represents aggregated audit statistics
type AuditStatistics struct {
	TotalEvents         int64            `json:"total_events"`
	SuccessfulEvents    int64            `json:"successful_events"`
	FailedEvents        int64            `json:"failed_events"`
	EventsByType        map[EventType]int64 `json:"events_by_type"`
	EventsBySeverity    map[Severity]int64  `json:"events_by_severity"`
	UniqueUsers         int64            `json:"unique_users"`
	UniqueIPAddresses   int64            `json:"unique_ip_addresses"`
}
