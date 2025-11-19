package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// Monitor handles backup monitoring and alerting
type Monitor struct {
	config *Config
	logger *zap.Logger
}

// NewMonitor creates a new backup monitor
func NewMonitor(config *Config, logger *zap.Logger) *Monitor {
	return &Monitor{
		config: config,
		logger: logger,
	}
}

// Alert represents a backup alert
type Alert struct {
	Level     string                 `json:"level"`      // "info", "warning", "error", "critical"
	Type      string                 `json:"type"`       // "backup_success", "backup_failure", "backup_warning"
	Message   string                 `json:"message"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// HealthCheckResult represents the result of a health check
type HealthCheckResult struct {
	Healthy            bool                   `json:"healthy"`
	LastBackupTime     time.Time              `json:"last_backup_time"`
	LastBackupSize     int64                  `json:"last_backup_size"`
	TimeSinceLastBackup time.Duration         `json:"time_since_last_backup"`
	TotalBackups       int                    `json:"total_backups"`
	TotalSize          int64                  `json:"total_size"`
	Issues             []string               `json:"issues,omitempty"`
	Warnings           []string               `json:"warnings,omitempty"`
	Timestamp          time.Time              `json:"timestamp"`
	Details            map[string]interface{} `json:"details,omitempty"`
}

// SendAlert sends an alert via webhook
func (m *Monitor) SendAlert(ctx context.Context, alert Alert) error {
	if !m.config.MonitoringEnabled {
		return nil
	}

	if m.config.AlertWebhookURL == "" {
		m.logger.Debug("Alert webhook URL not configured, skipping alert",
			zap.String("alert_type", alert.Type),
		)
		return nil
	}

	// Skip success alerts if not configured
	if alert.Level == "info" && !m.config.AlertOnSuccess {
		return nil
	}

	// Always send failure alerts if monitoring is enabled
	if alert.Level == "error" || alert.Level == "critical" {
		if !m.config.AlertOnFailure {
			return nil
		}
	}

	alert.Timestamp = time.Now()

	m.logger.Info("Sending alert",
		zap.String("level", alert.Level),
		zap.String("type", alert.Type),
		zap.String("message", alert.Message),
	)

	// Marshal alert to JSON
	alertJSON, err := json.Marshal(alert)
	if err != nil {
		return fmt.Errorf("failed to marshal alert: %w", err)
	}

	// Send webhook request
	req, err := http.NewRequestWithContext(ctx, "POST", m.config.AlertWebhookURL, bytes.NewBuffer(alertJSON))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send alert: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned non-success status: %d", resp.StatusCode)
	}

	m.logger.Info("Alert sent successfully",
		zap.String("alert_type", alert.Type),
		zap.Int("status_code", resp.StatusCode),
	)

	return nil
}

// NotifyBackupSuccess sends a success notification
func (m *Monitor) NotifyBackupSuccess(ctx context.Context, result *BackupResult) error {
	alert := Alert{
		Level:   "info",
		Type:    "backup_success",
		Message: fmt.Sprintf("Backup completed successfully: %s", result.Filename),
		Metadata: map[string]interface{}{
			"filename":    result.Filename,
			"size":        result.Size,
			"duration":    result.Duration.String(),
			"backup_type": result.BackupType,
			"checksum":    result.Checksum,
		},
	}

	return m.SendAlert(ctx, alert)
}

// NotifyBackupFailure sends a failure notification
func (m *Monitor) NotifyBackupFailure(ctx context.Context, err error, backupType string) error {
	alert := Alert{
		Level:   "error",
		Type:    "backup_failure",
		Message: fmt.Sprintf("Backup failed: %s", err.Error()),
		Metadata: map[string]interface{}{
			"error":       err.Error(),
			"backup_type": backupType,
		},
	}

	return m.SendAlert(ctx, alert)
}

// NotifyBackupWarning sends a warning notification
func (m *Monitor) NotifyBackupWarning(ctx context.Context, message string, metadata map[string]interface{}) error {
	alert := Alert{
		Level:    "warning",
		Type:     "backup_warning",
		Message:  message,
		Metadata: metadata,
	}

	return m.SendAlert(ctx, alert)
}

// NotifyRetentionPolicyApplied sends notification about retention policy
func (m *Monitor) NotifyRetentionPolicyApplied(ctx context.Context, deletedCount int, totalSize int64) error {
	alert := Alert{
		Level:   "info",
		Type:    "retention_policy_applied",
		Message: fmt.Sprintf("Retention policy applied: %d backups deleted", deletedCount),
		Metadata: map[string]interface{}{
			"deleted_count": deletedCount,
			"freed_space":   totalSize,
		},
	}

	return m.SendAlert(ctx, alert)
}

// CheckBackupHealth checks the health of the backup system
func (m *Monitor) CheckBackupHealth(ctx context.Context, storage *S3Storage) (*HealthCheckResult, error) {
	m.logger.Info("Performing backup health check")

	result := &HealthCheckResult{
		Healthy:   true,
		Timestamp: time.Now(),
		Details:   make(map[string]interface{}),
	}

	// List all backups
	backups, err := storage.ListBackups(ctx, "backups/")
	if err != nil {
		result.Healthy = false
		result.Issues = append(result.Issues, fmt.Sprintf("Failed to list backups: %s", err.Error()))
		return result, nil
	}

	result.TotalBackups = len(backups)

	// Find most recent backup
	var mostRecentBackup *time.Time
	for _, backup := range backups {
		if backup.LastModified == nil {
			continue
		}

		result.TotalSize += *backup.Size

		if mostRecentBackup == nil || backup.LastModified.After(*mostRecentBackup) {
			mostRecentBackup = backup.LastModified
			result.LastBackupSize = *backup.Size
		}
	}

	if mostRecentBackup != nil {
		result.LastBackupTime = *mostRecentBackup
		result.TimeSinceLastBackup = time.Since(*mostRecentBackup)

		// Check if last backup is too old (more than 48 hours)
		if result.TimeSinceLastBackup > 48*time.Hour {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Last backup is %s old (more than 48 hours)", result.TimeSinceLastBackup))
		}
	} else {
		result.Healthy = false
		result.Issues = append(result.Issues, "No backups found")
	}

	// Check if we have at least one backup
	if result.TotalBackups == 0 {
		result.Healthy = false
		result.Issues = append(result.Issues, "No backups available")
	}

	// Check backup size threshold
	if m.config.BackupSizeThreshold > 0 && result.LastBackupSize > m.config.BackupSizeThreshold {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Last backup size (%d bytes) exceeds threshold (%d bytes)",
				result.LastBackupSize, m.config.BackupSizeThreshold))
	}

	// Add retention policy info to details
	result.Details["retention_policy"] = map[string]int{
		"daily_retention_days":     m.config.RetentionDays,
		"weekly_retention_weeks":   m.config.RetentionWeeks,
		"monthly_retention_months": m.config.RetentionMonths,
	}

	m.logger.Info("Backup health check completed",
		zap.Bool("healthy", result.Healthy),
		zap.Int("total_backups", result.TotalBackups),
		zap.Int("issues", len(result.Issues)),
		zap.Int("warnings", len(result.Warnings)),
	)

	return result, nil
}

// LogBackupMetrics logs backup metrics for monitoring systems
func (m *Monitor) LogBackupMetrics(result *BackupResult) {
	m.logger.Info("Backup metrics",
		zap.String("filename", result.Filename),
		zap.Int64("size_bytes", result.Size),
		zap.Duration("duration", result.Duration),
		zap.String("backup_type", result.BackupType),
		zap.Bool("compressed", result.Compressed),
		zap.Bool("encrypted", result.Encrypted),
		zap.String("checksum", result.Checksum),
	)
}

// LogRetentionMetrics logs retention policy metrics
func (m *Monitor) LogRetentionMetrics(stats map[string]interface{}) {
	m.logger.Info("Retention metrics",
		zap.Any("stats", stats),
	)
}

// LogWALMetrics logs WAL archiving metrics
func (m *Monitor) LogWALMetrics(stats map[string]interface{}) {
	m.logger.Info("WAL archiving metrics",
		zap.Any("stats", stats),
	)
}
