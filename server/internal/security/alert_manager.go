package security

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dev-jelly/donelist/internal/webhook"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// AlertChannel represents different alert notification channels
type AlertChannel string

const (
	AlertChannelWebhook   AlertChannel = "webhook"
	AlertChannelEmail     AlertChannel = "email"
	AlertChannelSlack     AlertChannel = "slack"
	AlertChannelPagerDuty AlertChannel = "pagerduty"
	AlertChannelLog       AlertChannel = "log"
)

// AlertPriority represents alert priority
type AlertPriority string

const (
	AlertPriorityLow      AlertPriority = "low"
	AlertPriorityMedium   AlertPriority = "medium"
	AlertPriorityHigh     AlertPriority = "high"
	AlertPriorityCritical AlertPriority = "critical"
)

// Alert represents a security alert
type Alert struct {
	ID          uuid.UUID              `json:"id" gorm:"type:uuid;primaryKey"`
	AnomalyType AnomalyType            `json:"anomaly_type"`
	Severity    AnomalySeverity        `json:"severity"`
	Priority    AlertPriority          `json:"priority"`
	Identifier  string                 `json:"identifier"`
	IPAddress   string                 `json:"ip_address"`
	UserAgent   string                 `json:"user_agent"`
	Description string                 `json:"description"`
	Metadata    map[string]interface{} `json:"metadata" gorm:"type:jsonb"`
	Score       float64                `json:"score"`
	Status      AlertStatus            `json:"status"`
	Channels    []AlertChannel         `json:"channels" gorm:"type:jsonb"`
	SentAt      *time.Time             `json:"sent_at,omitempty"`
	ResolvedAt  *time.Time             `json:"resolved_at,omitempty"`
	ResolvedBy  *uuid.UUID             `json:"resolved_by,omitempty" gorm:"type:uuid"`
	Notes       string                 `json:"notes,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// TableName returns the table name for Alert
func (Alert) TableName() string {
	return "security_alerts"
}

// AlertStatus represents the status of an alert
type AlertStatus string

const (
	AlertStatusPending   AlertStatus = "pending"
	AlertStatusSent      AlertStatus = "sent"
	AlertStatusResolved  AlertStatus = "resolved"
	AlertStatusIgnored   AlertStatus = "ignored"
)

// AlertManagerConfig holds configuration for alert manager
type AlertManagerConfig struct {
	DB              *gorm.DB
	WebhookService  *webhook.Service
	Logger          *zap.Logger

	// Channel configuration
	EnabledChannels []AlertChannel
	WebhookURL      string
	SlackWebhookURL string
	PagerDutyKey    string

	// Alert throttling
	ThrottleWindow  time.Duration // Time window to group similar alerts
	MaxAlertsPerWindow int        // Max alerts per identifier per window

	// Alert routing
	CriticalChannels []AlertChannel // Channels for critical alerts
	HighChannels     []AlertChannel // Channels for high severity alerts
	MediumChannels   []AlertChannel // Channels for medium severity alerts
	LowChannels      []AlertChannel // Channels for low severity alerts
}

// AlertManager manages security alerts and notifications
type AlertManager struct {
	config AlertManagerConfig
	db     *gorm.DB
	logger *zap.Logger
	webhook *webhook.Service
}

// NewAlertManager creates a new alert manager
func NewAlertManager(config AlertManagerConfig) *AlertManager {
	// Set defaults
	if config.ThrottleWindow == 0 {
		config.ThrottleWindow = 5 * time.Minute
	}
	if config.MaxAlertsPerWindow == 0 {
		config.MaxAlertsPerWindow = 10
	}
	if len(config.EnabledChannels) == 0 {
		config.EnabledChannels = []AlertChannel{AlertChannelLog}
	}

	// Set default routing
	if len(config.CriticalChannels) == 0 {
		config.CriticalChannels = config.EnabledChannels
	}
	if len(config.HighChannels) == 0 {
		config.HighChannels = config.EnabledChannels
	}
	if len(config.MediumChannels) == 0 {
		config.MediumChannels = []AlertChannel{AlertChannelLog}
	}
	if len(config.LowChannels) == 0 {
		config.LowChannels = []AlertChannel{AlertChannelLog}
	}

	return &AlertManager{
		config:  config,
		db:      config.DB,
		logger:  config.Logger,
		webhook: config.WebhookService,
	}
}

// SendAlert creates and sends a security alert from anomaly event
func (m *AlertManager) SendAlert(ctx context.Context, event *AnomalyEvent) error {
	// Check throttling
	if m.shouldThrottle(ctx, event) {
		m.logger.Info("Alert throttled",
			zap.String("type", string(event.Type)),
			zap.String("identifier", event.Identifier),
		)
		return nil
	}

	// Create alert
	alert := &Alert{
		ID:          uuid.New(),
		AnomalyType: event.Type,
		Severity:    event.Severity,
		Priority:    m.severityToPriority(event.Severity),
		Identifier:  event.Identifier,
		IPAddress:   event.IPAddress,
		UserAgent:   event.UserAgent,
		Description: event.Description,
		Metadata:    event.Metadata,
		Score:       event.Score,
		Status:      AlertStatusPending,
		Channels:    m.selectChannels(event.Severity),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Save alert to database
	if err := m.db.WithContext(ctx).Create(alert).Error; err != nil {
		return fmt.Errorf("failed to create alert: %w", err)
	}

	// Send notifications
	if err := m.sendNotifications(ctx, alert); err != nil {
		m.logger.Error("Failed to send alert notifications",
			zap.Error(err),
			zap.String("alert_id", alert.ID.String()),
		)
		return err
	}

	// Update alert status
	now := time.Now()
	alert.Status = AlertStatusSent
	alert.SentAt = &now
	m.db.WithContext(ctx).Save(alert)

	m.logger.Info("Security alert sent",
		zap.String("alert_id", alert.ID.String()),
		zap.String("type", string(alert.AnomalyType)),
		zap.String("severity", string(alert.Severity)),
		zap.String("identifier", alert.Identifier),
	)

	return nil
}

// shouldThrottle checks if alert should be throttled
func (m *AlertManager) shouldThrottle(ctx context.Context, event *AnomalyEvent) bool {
	since := time.Now().Add(-m.config.ThrottleWindow)

	var count int64
	err := m.db.WithContext(ctx).
		Model(&Alert{}).
		Where("identifier = ? AND anomaly_type = ? AND created_at > ?",
			event.Identifier, event.Type, since).
		Count(&count).Error

	if err != nil {
		m.logger.Error("Failed to check alert throttle", zap.Error(err))
		return false
	}

	return count >= int64(m.config.MaxAlertsPerWindow)
}

// selectChannels selects notification channels based on severity
func (m *AlertManager) selectChannels(severity AnomalySeverity) []AlertChannel {
	switch severity {
	case SeverityCritical:
		return m.config.CriticalChannels
	case SeverityHigh:
		return m.config.HighChannels
	case SeverityMedium:
		return m.config.MediumChannels
	case SeverityLow:
		return m.config.LowChannels
	default:
		return []AlertChannel{AlertChannelLog}
	}
}

// severityToPriority converts anomaly severity to alert priority
func (m *AlertManager) severityToPriority(severity AnomalySeverity) AlertPriority {
	switch severity {
	case SeverityCritical:
		return AlertPriorityCritical
	case SeverityHigh:
		return AlertPriorityHigh
	case SeverityMedium:
		return AlertPriorityMedium
	case SeverityLow:
		return AlertPriorityLow
	default:
		return AlertPriorityLow
	}
}

// sendNotifications sends alert through configured channels
func (m *AlertManager) sendNotifications(ctx context.Context, alert *Alert) error {
	var lastErr error

	for _, channel := range alert.Channels {
		if !m.isChannelEnabled(channel) {
			continue
		}

		var err error
		switch channel {
		case AlertChannelLog:
			err = m.sendLogNotification(alert)
		case AlertChannelWebhook:
			err = m.sendWebhookNotification(ctx, alert)
		case AlertChannelSlack:
			err = m.sendSlackNotification(ctx, alert)
		case AlertChannelPagerDuty:
			err = m.sendPagerDutyNotification(ctx, alert)
		}

		if err != nil {
			m.logger.Error("Failed to send notification",
				zap.String("channel", string(channel)),
				zap.String("alert_id", alert.ID.String()),
				zap.Error(err),
			)
			lastErr = err
		}
	}

	return lastErr
}

// isChannelEnabled checks if a channel is enabled
func (m *AlertManager) isChannelEnabled(channel AlertChannel) bool {
	for _, enabled := range m.config.EnabledChannels {
		if enabled == channel {
			return true
		}
	}
	return false
}

// sendLogNotification logs the alert
func (m *AlertManager) sendLogNotification(alert *Alert) error {
	m.logger.Warn("SECURITY ALERT",
		zap.String("alert_id", alert.ID.String()),
		zap.String("type", string(alert.AnomalyType)),
		zap.String("severity", string(alert.Severity)),
		zap.String("identifier", alert.Identifier),
		zap.String("ip_address", alert.IPAddress),
		zap.String("description", alert.Description),
		zap.Float64("score", alert.Score),
	)
	return nil
}

// sendWebhookNotification sends alert via webhook
func (m *AlertManager) sendWebhookNotification(ctx context.Context, alert *Alert) error {
	if m.config.WebhookURL == "" {
		return fmt.Errorf("webhook URL not configured")
	}

	payload := map[string]interface{}{
		"alert_id":    alert.ID.String(),
		"type":        alert.AnomalyType,
		"severity":    alert.Severity,
		"priority":    alert.Priority,
		"identifier":  alert.Identifier,
		"ip_address":  alert.IPAddress,
		"user_agent":  alert.UserAgent,
		"description": alert.Description,
		"score":       alert.Score,
		"metadata":    alert.Metadata,
		"timestamp":   alert.CreatedAt.Unix(),
	}

	// Use webhook service if available
	if m.webhook != nil {
		// This would trigger webhooks subscribed to security events
		return m.webhook.TriggerEvent(ctx, "security.alert", uuid.Nil, payload)
	}

	return nil
}

// sendSlackNotification sends alert to Slack
func (m *AlertManager) sendSlackNotification(ctx context.Context, alert *Alert) error {
	if m.config.SlackWebhookURL == "" {
		return fmt.Errorf("Slack webhook URL not configured")
	}

	// Build Slack message
	color := m.getSlackColor(alert.Severity)
	message := map[string]interface{}{
		"text": fmt.Sprintf("🚨 Security Alert: %s", alert.AnomalyType),
		"attachments": []map[string]interface{}{
			{
				"color": color,
				"fields": []map[string]interface{}{
					{"title": "Type", "value": string(alert.AnomalyType), "short": true},
					{"title": "Severity", "value": string(alert.Severity), "short": true},
					{"title": "Identifier", "value": alert.Identifier, "short": true},
					{"title": "IP Address", "value": alert.IPAddress, "short": true},
					{"title": "Score", "value": fmt.Sprintf("%.1f", alert.Score), "short": true},
					{"title": "Time", "value": alert.CreatedAt.Format(time.RFC3339), "short": true},
					{"title": "Description", "value": alert.Description, "short": false},
				},
				"footer": "DoneList Security",
				"ts":     alert.CreatedAt.Unix(),
			},
		},
	}

	// Send to Slack (implementation would use HTTP client)
	jsonData, _ := json.Marshal(message)
	m.logger.Info("Slack notification payload prepared",
		zap.String("payload", string(jsonData)),
	)

	// TODO: Implement actual HTTP POST to Slack webhook URL
	return nil
}

// sendPagerDutyNotification sends alert to PagerDuty
func (m *AlertManager) sendPagerDutyNotification(ctx context.Context, alert *Alert) error {
	if m.config.PagerDutyKey == "" {
		return fmt.Errorf("PagerDuty key not configured")
	}

	// Build PagerDuty event
	event := map[string]interface{}{
		"routing_key":  m.config.PagerDutyKey,
		"event_action": "trigger",
		"payload": map[string]interface{}{
			"summary":        alert.Description,
			"severity":       m.getPagerDutySeverity(alert.Severity),
			"source":         "donelist-security",
			"component":      string(alert.AnomalyType),
			"custom_details": alert.Metadata,
		},
	}

	jsonData, _ := json.Marshal(event)
	m.logger.Info("PagerDuty notification payload prepared",
		zap.String("payload", string(jsonData)),
	)

	// TODO: Implement actual HTTP POST to PagerDuty Events API
	return nil
}

// getSlackColor returns color for Slack message based on severity
func (m *AlertManager) getSlackColor(severity AnomalySeverity) string {
	switch severity {
	case SeverityCritical:
		return "danger"
	case SeverityHigh:
		return "warning"
	case SeverityMedium:
		return "#FFD700" // Gold
	case SeverityLow:
		return "good"
	default:
		return "#808080" // Gray
	}
}

// getPagerDutySeverity converts severity to PagerDuty severity
func (m *AlertManager) getPagerDutySeverity(severity AnomalySeverity) string {
	switch severity {
	case SeverityCritical:
		return "critical"
	case SeverityHigh:
		return "error"
	case SeverityMedium:
		return "warning"
	case SeverityLow:
		return "info"
	default:
		return "info"
	}
}

// GetAlert gets an alert by ID
func (m *AlertManager) GetAlert(ctx context.Context, alertID uuid.UUID) (*Alert, error) {
	var alert Alert
	err := m.db.WithContext(ctx).Where("id = ?", alertID).First(&alert).Error
	if err != nil {
		return nil, err
	}
	return &alert, nil
}

// ListAlerts lists alerts with filtering
func (m *AlertManager) ListAlerts(ctx context.Context, opts ListAlertsOptions) ([]*Alert, error) {
	query := m.db.WithContext(ctx).Model(&Alert{})

	if opts.Identifier != nil {
		query = query.Where("identifier = ?", *opts.Identifier)
	}
	if opts.Severity != nil {
		query = query.Where("severity = ?", *opts.Severity)
	}
	if opts.Status != nil {
		query = query.Where("status = ?", *opts.Status)
	}
	if opts.AnomalyType != nil {
		query = query.Where("anomaly_type = ?", *opts.AnomalyType)
	}
	if opts.StartDate != nil {
		query = query.Where("created_at >= ?", *opts.StartDate)
	}
	if opts.EndDate != nil {
		query = query.Where("created_at <= ?", *opts.EndDate)
	}

	if opts.Limit == 0 {
		opts.Limit = 100
	}

	var alerts []*Alert
	err := query.
		Order("created_at DESC").
		Limit(opts.Limit).
		Offset(opts.Offset).
		Find(&alerts).Error

	return alerts, err
}

// ListAlertsOptions represents options for listing alerts
type ListAlertsOptions struct {
	Identifier  *string
	Severity    *AnomalySeverity
	Status      *AlertStatus
	AnomalyType *AnomalyType
	StartDate   *time.Time
	EndDate     *time.Time
	Limit       int
	Offset      int
}

// ResolveAlert marks an alert as resolved
func (m *AlertManager) ResolveAlert(ctx context.Context, alertID uuid.UUID, resolvedBy *uuid.UUID, notes string) error {
	now := time.Now()
	err := m.db.WithContext(ctx).
		Model(&Alert{}).
		Where("id = ?", alertID).
		Updates(map[string]interface{}{
			"status":      AlertStatusResolved,
			"resolved_at": now,
			"resolved_by": resolvedBy,
			"notes":       notes,
			"updated_at":  now,
		}).Error

	if err != nil {
		return fmt.Errorf("failed to resolve alert: %w", err)
	}

	m.logger.Info("Alert resolved",
		zap.String("alert_id", alertID.String()),
		zap.String("notes", notes),
	)

	return nil
}

// IgnoreAlert marks an alert as ignored
func (m *AlertManager) IgnoreAlert(ctx context.Context, alertID uuid.UUID, notes string) error {
	now := time.Now()
	err := m.db.WithContext(ctx).
		Model(&Alert{}).
		Where("id = ?", alertID).
		Updates(map[string]interface{}{
			"status":     AlertStatusIgnored,
			"notes":      notes,
			"updated_at": now,
		}).Error

	if err != nil {
		return fmt.Errorf("failed to ignore alert: %w", err)
	}

	return nil
}

// GetAlertStatistics returns statistics about alerts
func (m *AlertManager) GetAlertStatistics(ctx context.Context, since time.Time) (*AlertStatistics, error) {
	stats := &AlertStatistics{
		BySeverity:  make(map[AnomalySeverity]int64),
		ByType:      make(map[AnomalyType]int64),
		ByStatus:    make(map[AlertStatus]int64),
	}

	// Count total alerts
	m.db.WithContext(ctx).
		Model(&Alert{}).
		Where("created_at >= ?", since).
		Count(&stats.TotalAlerts)

	// Count by severity
	var severityCounts []struct {
		Severity AnomalySeverity
		Count    int64
	}
	m.db.WithContext(ctx).
		Model(&Alert{}).
		Select("severity, COUNT(*) as count").
		Where("created_at >= ?", since).
		Group("severity").
		Scan(&severityCounts)

	for _, sc := range severityCounts {
		stats.BySeverity[sc.Severity] = sc.Count
	}

	// Count by type
	var typeCounts []struct {
		Type  AnomalyType
		Count int64
	}
	m.db.WithContext(ctx).
		Model(&Alert{}).
		Select("anomaly_type as type, COUNT(*) as count").
		Where("created_at >= ?", since).
		Group("anomaly_type").
		Scan(&typeCounts)

	for _, tc := range typeCounts {
		stats.ByType[tc.Type] = tc.Count
	}

	// Count by status
	var statusCounts []struct {
		Status AlertStatus
		Count  int64
	}
	m.db.WithContext(ctx).
		Model(&Alert{}).
		Select("status, COUNT(*) as count").
		Where("created_at >= ?", since).
		Group("status").
		Scan(&statusCounts)

	for _, sc := range statusCounts {
		stats.ByStatus[sc.Status] = sc.Count
	}

	return stats, nil
}

// AlertStatistics represents alert statistics
type AlertStatistics struct {
	TotalAlerts int64                       `json:"total_alerts"`
	BySeverity  map[AnomalySeverity]int64   `json:"by_severity"`
	ByType      map[AnomalyType]int64       `json:"by_type"`
	ByStatus    map[AlertStatus]int64       `json:"by_status"`
}
