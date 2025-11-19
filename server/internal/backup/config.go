package backup

import (
	"fmt"
	"time"
)

// Config holds the backup configuration
type Config struct {
	// Database Configuration
	DBHost     string
	DBPort     int
	DBName     string
	DBUser     string
	DBPassword string

	// Redis Configuration
	RedisHost     string
	RedisPort     int
	RedisPassword string
	RedisDB       int

	// S3 Configuration
	S3Endpoint        string
	S3AccessKeyID     string
	S3SecretAccessKey string
	S3Bucket          string
	S3Region          string
	S3UseSSL          bool

	// Backup Configuration
	BackupDir           string
	ScheduleEnabled     bool
	ScheduleCron        string // Default: "0 2 * * *" (2 AM daily)
	RetentionDays       int    // Daily backups retention
	RetentionWeeks      int    // Weekly backups retention
	RetentionMonths     int    // Monthly backups retention
	CompressionEnabled  bool
	EncryptionEnabled   bool
	EncryptionKey       string
	WALArchivingEnabled bool
	WALArchivePath      string

	// Monitoring
	MonitoringEnabled    bool
	AlertWebhookURL      string
	AlertOnFailure       bool
	AlertOnSuccess       bool
	HealthCheckInterval  time.Duration
	BackupSizeThreshold  int64 // Alert if backup size exceeds this (bytes)
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.DBHost == "" {
		return fmt.Errorf("database host is required")
	}
	if c.DBPort == 0 {
		return fmt.Errorf("database port is required")
	}
	if c.DBName == "" {
		return fmt.Errorf("database name is required")
	}
	if c.DBUser == "" {
		return fmt.Errorf("database user is required")
	}

	if c.S3Bucket == "" {
		return fmt.Errorf("S3 bucket is required")
	}
	if c.S3AccessKeyID == "" {
		return fmt.Errorf("S3 access key ID is required")
	}
	if c.S3SecretAccessKey == "" {
		return fmt.Errorf("S3 secret access key is required")
	}

	if c.BackupDir == "" {
		c.BackupDir = "/var/backups/donelist"
	}

	if c.RetentionDays == 0 {
		c.RetentionDays = 7
	}
	if c.RetentionWeeks == 0 {
		c.RetentionWeeks = 4
	}
	if c.RetentionMonths == 0 {
		c.RetentionMonths = 12
	}

	if c.ScheduleCron == "" {
		c.ScheduleCron = "0 2 * * *" // 2 AM daily
	}

	if c.HealthCheckInterval == 0 {
		c.HealthCheckInterval = 5 * time.Minute
	}

	return nil
}

// DefaultConfig returns a configuration with default values
func DefaultConfig() *Config {
	return &Config{
		DBPort:              5432,
		RedisPort:           6379,
		RedisDB:             0,
		BackupDir:           "/var/backups/donelist",
		ScheduleEnabled:     true,
		ScheduleCron:        "0 2 * * *",
		RetentionDays:       7,
		RetentionWeeks:      4,
		RetentionMonths:     12,
		CompressionEnabled:  true,
		EncryptionEnabled:   false,
		WALArchivingEnabled: false,
		MonitoringEnabled:   true,
		AlertOnFailure:      true,
		AlertOnSuccess:      false,
		HealthCheckInterval: 5 * time.Minute,
		BackupSizeThreshold: 10 * 1024 * 1024 * 1024, // 10 GB
		S3UseSSL:            true,
	}
}
