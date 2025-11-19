package audit

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Service provides audit logging functionality
type Service struct {
	repo   *Repository
	logger *zap.Logger
}

// NewService creates a new audit service
func NewService(repo *Repository, logger *zap.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
	}
}

// Log creates a new audit log entry
func (s *Service) Log(ctx context.Context, input CreateAuditLogInput) error {
	// Set defaults
	if input.Severity == "" {
		input.Severity = SeverityInfo
	}

	log, err := s.repo.Create(ctx, input)
	if err != nil {
		s.logger.Error("Failed to create audit log",
			zap.Error(err),
			zap.String("event_type", string(input.EventType)),
			zap.String("action", input.Action),
		)
		return fmt.Errorf("failed to create audit log: %w", err)
	}

	// Also log to structured logger for real-time monitoring
	s.logToStructuredLogger(log)

	return nil
}

// LogSuccess logs a successful operation
func (s *Service) LogSuccess(ctx context.Context, eventType EventType, action string, userID *uuid.UUID, ipAddress, userAgent string, details map[string]interface{}) error {
	return s.Log(ctx, CreateAuditLogInput{
		EventType:  eventType,
		Severity:   SeverityInfo,
		UserID:     userID,
		IPAddress:  ipAddress,
		UserAgent:  userAgent,
		Action:     action,
		Details:    details,
		Success:    true,
	})
}

// LogFailure logs a failed operation
func (s *Service) LogFailure(ctx context.Context, eventType EventType, action string, userID *uuid.UUID, ipAddress, userAgent string, errorMsg string, details map[string]interface{}) error {
	severity := SeverityWarning
	if isSecurityEvent(eventType) {
		severity = SeverityError
	}

	return s.Log(ctx, CreateAuditLogInput{
		EventType:    eventType,
		Severity:     severity,
		UserID:       userID,
		IPAddress:    ipAddress,
		UserAgent:    userAgent,
		Action:       action,
		Details:      details,
		Success:      false,
		ErrorMessage: &errorMsg,
	})
}

// LogSecurityEvent logs a security-related event
func (s *Service) LogSecurityEvent(ctx context.Context, eventType EventType, action string, userID *uuid.UUID, ipAddress, userAgent string, details map[string]interface{}) error {
	return s.Log(ctx, CreateAuditLogInput{
		EventType:  eventType,
		Severity:   SeverityCritical,
		UserID:     userID,
		IPAddress:  ipAddress,
		UserAgent:  userAgent,
		Action:     action,
		Details:    details,
		Success:    false,
	})
}

// ListLogs retrieves audit logs based on options
func (s *Service) ListLogs(ctx context.Context, opts ListAuditLogsOptions) ([]AuditLog, error) {
	// Set default limit if not specified
	if opts.Limit == 0 {
		opts.Limit = 100
	}
	if opts.Limit > 1000 {
		opts.Limit = 1000 // Max limit
	}

	logs, err := s.repo.List(ctx, opts)
	if err != nil {
		s.logger.Error("Failed to list audit logs", zap.Error(err))
		return nil, fmt.Errorf("failed to list audit logs: %w", err)
	}

	return logs, nil
}

// GetLog retrieves an audit log by ID
func (s *Service) GetLog(ctx context.Context, id uuid.UUID) (*AuditLog, error) {
	log, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get audit log: %w", err)
	}

	return log, nil
}

// GetStatistics retrieves audit statistics for a date range
func (s *Service) GetStatistics(ctx context.Context, startDate, endDate time.Time) (*AuditStatistics, error) {
	stats, err := s.repo.GetStatistics(ctx, startDate, endDate)
	if err != nil {
		s.logger.Error("Failed to get audit statistics", zap.Error(err))
		return nil, fmt.Errorf("failed to get audit statistics: %w", err)
	}

	return stats, nil
}

// CleanupOldLogs deletes audit logs older than the retention period
func (s *Service) CleanupOldLogs(ctx context.Context, retentionDays int) (int64, error) {
	if retentionDays < 1 {
		return 0, fmt.Errorf("retention days must be at least 1")
	}

	deleted, err := s.repo.DeleteOldLogs(ctx, retentionDays)
	if err != nil {
		s.logger.Error("Failed to cleanup old audit logs", zap.Error(err))
		return 0, fmt.Errorf("failed to cleanup old audit logs: %w", err)
	}

	s.logger.Info("Cleaned up old audit logs",
		zap.Int64("deleted_count", deleted),
		zap.Int("retention_days", retentionDays),
	)

	return deleted, nil
}

// logToStructuredLogger logs audit events to structured logger for real-time monitoring
func (s *Service) logToStructuredLogger(log *AuditLog) {
	fields := []zap.Field{
		zap.String("event_type", string(log.EventType)),
		zap.String("severity", string(log.Severity)),
		zap.String("action", log.Action),
		zap.Bool("success", log.Success),
		zap.String("ip_address", log.IPAddress),
	}

	if log.UserID != nil {
		fields = append(fields, zap.String("user_id", log.UserID.String()))
	}

	if log.ResourceID != nil {
		fields = append(fields, zap.String("resource_id", log.ResourceID.String()))
	}

	if log.ErrorMessage != nil {
		fields = append(fields, zap.String("error", *log.ErrorMessage))
	}

	if log.RequestID != "" {
		fields = append(fields, zap.String("request_id", log.RequestID))
	}

	switch log.Severity {
	case SeverityCritical, SeverityError:
		s.logger.Error("Audit log", fields...)
	case SeverityWarning:
		s.logger.Warn("Audit log", fields...)
	default:
		s.logger.Info("Audit log", fields...)
	}
}

// isSecurityEvent checks if an event type is security-related
func isSecurityEvent(eventType EventType) bool {
	securityEvents := map[EventType]bool{
		EventTypeLoginFailed:             true,
		EventTypeCSRFValidationFailed:    true,
		EventTypeRateLimitExceeded:       true,
		EventTypeInvalidInput:            true,
		EventTypeUnauthorizedAccess:      true,
		EventTypeSuspiciousActivity:      true,
		EventTypeAccountLocked:           true,
	}

	return securityEvents[eventType]
}

// Helper methods for common audit operations

// LogLogin logs a login event
func (s *Service) LogLogin(ctx context.Context, userID uuid.UUID, ipAddress, userAgent string, success bool, errorMsg string) error {
	if success {
		return s.LogSuccess(ctx, EventTypeLoginSuccess, "User logged in", &userID, ipAddress, userAgent, nil)
	}

	return s.LogFailure(ctx, EventTypeLoginFailed, "Login attempt failed", &userID, ipAddress, userAgent, errorMsg, nil)
}

// LogLogout logs a logout event
func (s *Service) LogLogout(ctx context.Context, userID uuid.UUID, ipAddress, userAgent string) error {
	return s.LogSuccess(ctx, EventTypeLogout, "User logged out", &userID, ipAddress, userAgent, nil)
}

// LogRegistration logs a registration event
func (s *Service) LogRegistration(ctx context.Context, userID uuid.UUID, ipAddress, userAgent string, email string) error {
	return s.LogSuccess(ctx, EventTypeRegister, "User registered", &userID, ipAddress, userAgent, map[string]interface{}{
		"email": email,
	})
}

// LogUserDeletion logs a user deletion event
func (s *Service) LogUserDeletion(ctx context.Context, userID uuid.UUID, ipAddress, userAgent string) error {
	return s.LogSuccess(ctx, EventTypeUserDelete, "User deleted their account", &userID, ipAddress, userAgent, nil)
}

// LogRateLimitExceeded logs a rate limit exceeded event
func (s *Service) LogRateLimitExceeded(ctx context.Context, userID *uuid.UUID, ipAddress, userAgent, endpoint string) error {
	return s.LogSecurityEvent(ctx, EventTypeRateLimitExceeded, "Rate limit exceeded", userID, ipAddress, userAgent, map[string]interface{}{
		"endpoint": endpoint,
	})
}

// LogCSRFFailure logs a CSRF validation failure
func (s *Service) LogCSRFFailure(ctx context.Context, userID *uuid.UUID, ipAddress, userAgent, endpoint string) error {
	return s.LogSecurityEvent(ctx, EventTypeCSRFValidationFailed, "CSRF validation failed", userID, ipAddress, userAgent, map[string]interface{}{
		"endpoint": endpoint,
	})
}

// LogUnauthorizedAccess logs an unauthorized access attempt
func (s *Service) LogUnauthorizedAccess(ctx context.Context, userID *uuid.UUID, ipAddress, userAgent, resource string) error {
	return s.LogSecurityEvent(ctx, EventTypeUnauthorizedAccess, "Unauthorized access attempt", userID, ipAddress, userAgent, map[string]interface{}{
		"resource": resource,
	})
}
