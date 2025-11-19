package notification

import (
	"time"
)

// Config holds the complete notification system configuration
type Config struct {
	// Queue configuration
	Queue QueueConfig

	// Scheduler configuration
	Scheduler SchedulerConfig

	// Provider configuration (for future subtasks)
	FCM  FCMConfig
	APNs APNsConfig

	// Feature flags
	Enabled           bool
	DryRun            bool // If true, don't actually send notifications
	EnableMetrics     bool
	EnableHealthCheck bool
}

// FCMConfig holds Firebase Cloud Messaging configuration
// Will be implemented in subtask 2.3
type FCMConfig struct {
	Enabled           bool
	ProjectID         string
	CredentialsPath   string
	CredentialsJSON   string
	Timeout           time.Duration
	MaxRetries        int
	RetryBackoffDelay time.Duration
}

// APNsConfig holds Apple Push Notification service configuration
// Will be implemented in subtask 2.3
type APNsConfig struct {
	Enabled           bool
	KeyID             string
	TeamID            string
	BundleID          string
	KeyPath           string
	KeyContent        string
	Production        bool // Use production or sandbox APNs
	Timeout           time.Duration
	MaxRetries        int
	RetryBackoffDelay time.Duration
}

// DefaultConfig returns the default notification system configuration
func DefaultConfig() *Config {
	return &Config{
		Queue:             DefaultQueueConfig(),
		Scheduler:         *DefaultSchedulerConfig(),
		Enabled:           true,
		DryRun:            false,
		EnableMetrics:     true,
		EnableHealthCheck: true,
		FCM: FCMConfig{
			Enabled:           false,
			Timeout:           30 * time.Second,
			MaxRetries:        3,
			RetryBackoffDelay: 1 * time.Second,
		},
		APNs: APNsConfig{
			Enabled:           false,
			Production:        false,
			Timeout:           30 * time.Second,
			MaxRetries:        3,
			RetryBackoffDelay: 1 * time.Second,
		},
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Add validation logic as needed
	// This will be extended in future subtasks
	return nil
}
