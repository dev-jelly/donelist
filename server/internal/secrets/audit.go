package secrets

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// AuditEvent represents a secret access or operation event
type AuditEvent struct {
	Timestamp   time.Time              `json:"timestamp"`
	SecretID    string                 `json:"secret_id"`
	Action      string                 `json:"action"`
	Success     bool                   `json:"success"`
	UserID      string                 `json:"user_id,omitempty"`
	IP          string                 `json:"ip,omitempty"`
	Environment Environment            `json:"environment"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// FileAuditLogger implements audit logging to file
type FileAuditLogger struct {
	file        *os.File
	encoder     *json.Encoder
	mu          sync.Mutex
	environment Environment
}

// NewFileAuditLogger creates a file-based audit logger
func NewFileAuditLogger(filepath string, env Environment) (*FileAuditLogger, error) {
	file, err := os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open audit log file: %w", err)
	}

	return &FileAuditLogger{
		file:        file,
		encoder:     json.NewEncoder(file),
		environment: env,
	}, nil
}

// LogSecretAccess logs a secret access event
func (l *FileAuditLogger) LogSecretAccess(ctx context.Context, secretID string, action string, success bool, metadata map[string]interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	event := AuditEvent{
		Timestamp:   time.Now(),
		SecretID:    secretID,
		Action:      action,
		Success:     success,
		Environment: l.environment,
		Metadata:    metadata,
	}

	// Try to extract user ID from context
	if userID, ok := ctx.Value("user_id").(string); ok {
		event.UserID = userID
	}

	// Try to extract IP from context
	if ip, ok := ctx.Value("ip").(string); ok {
		event.IP = ip
	}

	// Write to file
	if err := l.encoder.Encode(event); err != nil {
		// Log error but don't fail the operation
		fmt.Fprintf(os.Stderr, "failed to write audit log: %v\n", err)
	}

	// Sync to disk for critical events
	if action == "rotate" || action == "delete" || !success {
		_ = l.file.Sync()
	}
}

// Close closes the audit log file
func (l *FileAuditLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.file.Close()
}

// MemoryAuditLogger implements in-memory audit logging (for testing)
type MemoryAuditLogger struct {
	events      []AuditEvent
	mu          sync.RWMutex
	environment Environment
}

// NewMemoryAuditLogger creates an in-memory audit logger
func NewMemoryAuditLogger(env Environment) *MemoryAuditLogger {
	return &MemoryAuditLogger{
		events:      make([]AuditEvent, 0),
		environment: env,
	}
}

// LogSecretAccess logs a secret access event to memory
func (l *MemoryAuditLogger) LogSecretAccess(ctx context.Context, secretID string, action string, success bool, metadata map[string]interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	event := AuditEvent{
		Timestamp:   time.Now(),
		SecretID:    secretID,
		Action:      action,
		Success:     success,
		Environment: l.environment,
		Metadata:    metadata,
	}

	// Try to extract user ID from context
	if userID, ok := ctx.Value("user_id").(string); ok {
		event.UserID = userID
	}

	l.events = append(l.events, event)
}

// GetEvents returns all logged events
func (l *MemoryAuditLogger) GetEvents() []AuditEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()

	// Return a copy
	events := make([]AuditEvent, len(l.events))
	copy(events, l.events)
	return events
}

// GetEventsBySecretID returns events for a specific secret
func (l *MemoryAuditLogger) GetEventsBySecretID(secretID string) []AuditEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var filtered []AuditEvent
	for _, event := range l.events {
		if event.SecretID == secretID {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

// GetEventsByAction returns events for a specific action
func (l *MemoryAuditLogger) GetEventsByAction(action string) []AuditEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var filtered []AuditEvent
	for _, event := range l.events {
		if event.Action == action {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

// GetFailedEvents returns all failed events
func (l *MemoryAuditLogger) GetFailedEvents() []AuditEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var filtered []AuditEvent
	for _, event := range l.events {
		if !event.Success {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

// Clear clears all events (for testing)
func (l *MemoryAuditLogger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.events = make([]AuditEvent, 0)
}

// Count returns the number of events
func (l *MemoryAuditLogger) Count() int {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return len(l.events)
}

// MultiAuditLogger logs to multiple audit loggers
type MultiAuditLogger struct {
	loggers []AuditLogger
}

// NewMultiAuditLogger creates a logger that writes to multiple destinations
func NewMultiAuditLogger(loggers ...AuditLogger) *MultiAuditLogger {
	return &MultiAuditLogger{
		loggers: loggers,
	}
}

// LogSecretAccess logs to all configured loggers
func (l *MultiAuditLogger) LogSecretAccess(ctx context.Context, secretID string, action string, success bool, metadata map[string]interface{}) {
	for _, logger := range l.loggers {
		logger.LogSecretAccess(ctx, secretID, action, success, metadata)
	}
}
