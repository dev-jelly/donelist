package health

import "time"

// Status represents the health status
type Status string

const (
	// StatusHealthy indicates a healthy component
	StatusHealthy Status = "healthy"
	// StatusUnhealthy indicates an unhealthy component
	StatusUnhealthy Status = "unhealthy"
	// StatusDegraded indicates a degraded component (functional but with issues)
	StatusDegraded Status = "degraded"
)

// ComponentHealth represents the health of a single component
type ComponentHealth struct {
	Name      string                 `json:"name"`
	Status    Status                 `json:"status"`
	Message   string                 `json:"message,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Duration  time.Duration          `json:"duration_ms,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// HealthReport represents the overall health report
type HealthReport struct {
	Status     Status                     `json:"status"`
	Version    string                     `json:"version"`
	Timestamp  time.Time                  `json:"timestamp"`
	Uptime     time.Duration              `json:"uptime_seconds"`
	Components map[string]ComponentHealth `json:"components"`
}

// Checker is an interface for health checks
type Checker interface {
	// Name returns the name of the component being checked
	Name() string
	// Check performs the health check and returns the result
	Check() ComponentHealth
}
