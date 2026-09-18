package admin

import (
	"context"
	"fmt"
	"time"

	"github.com/dev-jelly/donelist/internal/subscription"
	"go.uber.org/zap"
)

// PaymentMonitor monitors payment health and generates alerts
type PaymentMonitor struct {
	subscriptionRepo *subscription.Repository
	logger           *zap.Logger
	alertThresholds  *AlertThresholds
}

// AlertThresholds defines thresholds for triggering alerts
type AlertThresholds struct {
	FailureRatePercent     float64       // Alert if failure rate exceeds this percentage
	FailedPaymentsCount    int           // Alert if number of failed payments exceeds this
	PastDueGracePeriod     time.Duration // Grace period before alerting on past due
	ChurnRatePercent       float64       // Alert if churn rate exceeds this percentage
	RevenueDropPercent     float64       // Alert if revenue drops by this percentage
}

// DefaultAlertThresholds returns sensible default thresholds
func DefaultAlertThresholds() *AlertThresholds {
	return &AlertThresholds{
		FailureRatePercent:  10.0, // Alert if >10% of payments fail
		FailedPaymentsCount: 5,    // Alert if >5 payments fail in monitoring period
		PastDueGracePeriod:  72 * time.Hour, // 3 days
		ChurnRatePercent:    5.0,  // Alert if monthly churn >5%
		RevenueDropPercent:  15.0, // Alert if MRR drops >15%
	}
}

// NewPaymentMonitor creates a new payment monitor
func NewPaymentMonitor(
	subscriptionRepo *subscription.Repository,
	logger *zap.Logger,
	thresholds *AlertThresholds,
) *PaymentMonitor {
	if thresholds == nil {
		thresholds = DefaultAlertThresholds()
	}

	return &PaymentMonitor{
		subscriptionRepo: subscriptionRepo,
		logger:           logger,
		alertThresholds:  thresholds,
	}
}

// Alert represents a monitoring alert
type Alert struct {
	ID          string                 `json:"id"`
	Type        AlertType              `json:"type"`
	Severity    AlertSeverity          `json:"severity"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Data        map[string]interface{} `json:"data"`
	Timestamp   time.Time              `json:"timestamp"`
	Resolved    bool                   `json:"resolved"`
}

// AlertType represents the type of alert
type AlertType string

const (
	AlertTypePaymentFailure   AlertType = "payment_failure"
	AlertTypeHighChurnRate    AlertType = "high_churn_rate"
	AlertTypeRevenueDrop      AlertType = "revenue_drop"
	AlertTypePastDueSubscriptions AlertType = "past_due_subscriptions"
	AlertTypeHighFailureRate  AlertType = "high_failure_rate"
)

// AlertSeverity represents alert severity levels
type AlertSeverity string

const (
	AlertSeverityInfo     AlertSeverity = "info"
	AlertSeverityWarning  AlertSeverity = "warning"
	AlertSeverityCritical AlertSeverity = "critical"
)

// MonitoringReport contains the results of a monitoring check
type MonitoringReport struct {
	Timestamp       time.Time              `json:"timestamp"`
	OverallHealth   HealthStatus           `json:"overall_health"`
	Alerts          []*Alert               `json:"alerts"`
	Metrics         *PaymentHealthMetrics  `json:"metrics"`
	Recommendations []string               `json:"recommendations"`
}

// HealthStatus represents overall system health
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
)

// PaymentHealthMetrics contains payment health indicators
type PaymentHealthMetrics struct {
	TotalPaymentsLast24h    int     `json:"total_payments_last_24h"`
	FailedPaymentsLast24h   int     `json:"failed_payments_last_24h"`
	FailureRate             float64 `json:"failure_rate"`
	PastDueCount            int     `json:"past_due_count"`
	CurrentMRR              float64 `json:"current_mrr"`
	PreviousMRR             float64 `json:"previous_mrr"`
	MRRChange               float64 `json:"mrr_change"`
	ChurnRate               float64 `json:"churn_rate"`
	AveragePaymentAmount    float64 `json:"average_payment_amount"`
}

// RunHealthCheck performs a comprehensive health check
func (m *PaymentMonitor) RunHealthCheck(ctx context.Context) (*MonitoringReport, error) {
	report := &MonitoringReport{
		Timestamp:       time.Now(),
		Alerts:          make([]*Alert, 0),
		Recommendations: make([]string, 0),
	}

	// Calculate health metrics
	metrics, err := m.calculateHealthMetrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate health metrics: %w", err)
	}
	report.Metrics = metrics

	// Check for payment failures
	if alert := m.checkPaymentFailures(ctx, metrics); alert != nil {
		report.Alerts = append(report.Alerts, alert)
	}

	// Check for high failure rate
	if alert := m.checkFailureRate(metrics); alert != nil {
		report.Alerts = append(report.Alerts, alert)
	}

	// Check for past due subscriptions
	if alert := m.checkPastDueSubscriptions(ctx, metrics); alert != nil {
		report.Alerts = append(report.Alerts, alert)
	}

	// Check for revenue drops
	if alert := m.checkRevenueDrop(metrics); alert != nil {
		report.Alerts = append(report.Alerts, alert)
	}

	// Check for high churn rate
	if alert := m.checkChurnRate(metrics); alert != nil {
		report.Alerts = append(report.Alerts, alert)
	}

	// Determine overall health
	report.OverallHealth = m.determineOverallHealth(report.Alerts)

	// Generate recommendations
	report.Recommendations = m.generateRecommendations(report)

	// Log critical alerts
	for _, alert := range report.Alerts {
		if alert.Severity == AlertSeverityCritical {
			m.logger.Error("Critical payment alert",
				zap.String("alert_type", string(alert.Type)),
				zap.String("title", alert.Title),
				zap.Any("data", alert.Data),
			)
		}
	}

	return report, nil
}

// calculateHealthMetrics calculates payment health metrics
func (m *PaymentMonitor) calculateHealthMetrics(ctx context.Context) (*PaymentHealthMetrics, error) {
	metrics := &PaymentHealthMetrics{}

	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)

	// Get recent payments
	allPayments, err := m.subscriptionRepo.ListPayments(ctx, nil, nil, 1000)
	if err != nil {
		return nil, fmt.Errorf("failed to get payments: %w", err)
	}

	// Calculate payment metrics for last 24 hours
	var totalAmount int
	for _, payment := range allPayments {
		if payment.CreatedAt.After(yesterday) {
			metrics.TotalPaymentsLast24h++
			totalAmount += payment.Amount

			if payment.Status == subscription.PaymentStatusFailed {
				metrics.FailedPaymentsLast24h++
			}
		}
	}

	// Calculate failure rate
	if metrics.TotalPaymentsLast24h > 0 {
		metrics.FailureRate = (float64(metrics.FailedPaymentsLast24h) / float64(metrics.TotalPaymentsLast24h)) * 100
		metrics.AveragePaymentAmount = float64(totalAmount) / float64(metrics.TotalPaymentsLast24h)
	}

	// Get past due subscriptions
	pastDueSubs, err := m.subscriptionRepo.GetPastDueSubscriptions(ctx, 0)
	if err != nil {
		m.logger.Warn("Failed to get past due subscriptions", zap.Error(err))
	} else {
		metrics.PastDueCount = len(pastDueSubs)
	}

	// Calculate MRR (simplified - in production you'd want more sophisticated calculation)
	activeFilter := subscription.StatusActive
	activeSubs, err := m.subscriptionRepo.ListSubscriptions(ctx, subscription.SubscriptionFilter{
		Status: &activeFilter,
		Limit:  10000,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get active subscriptions: %w", err)
	}

	for _, sub := range activeSubs {
		plan, exists := subscription.GetPlan(subscription.PlanID(sub.PlanID))
		if exists {
			// Normalize to monthly revenue
			duration := sub.CurrentPeriodEnd.Sub(sub.CurrentPeriodStart)
			monthlyRevenue := float64(plan.MonthlyPrice)
			if duration > 31*24*time.Hour {
				monthlyRevenue = float64(plan.YearlyPrice) / 12
			}
			metrics.CurrentMRR += monthlyRevenue
		}
	}

	// For MRR change, you'd typically store historical MRR values
	// For now, we'll set it to 0
	metrics.PreviousMRR = metrics.CurrentMRR
	metrics.MRRChange = 0

	return metrics, nil
}

// checkPaymentFailures checks for payment failure alerts
func (m *PaymentMonitor) checkPaymentFailures(ctx context.Context, metrics *PaymentHealthMetrics) *Alert {
	if metrics.FailedPaymentsLast24h > m.alertThresholds.FailedPaymentsCount {
		return &Alert{
			ID:        generateAlertID(AlertTypePaymentFailure),
			Type:      AlertTypePaymentFailure,
			Severity:  AlertSeverityWarning,
			Title:     "High number of payment failures",
			Description: fmt.Sprintf("%d payments failed in the last 24 hours (threshold: %d)",
				metrics.FailedPaymentsLast24h, m.alertThresholds.FailedPaymentsCount),
			Data: map[string]interface{}{
				"failed_payments": metrics.FailedPaymentsLast24h,
				"threshold":       m.alertThresholds.FailedPaymentsCount,
			},
			Timestamp: time.Now(),
		}
	}
	return nil
}

// checkFailureRate checks for high payment failure rate
func (m *PaymentMonitor) checkFailureRate(metrics *PaymentHealthMetrics) *Alert {
	if metrics.FailureRate > m.alertThresholds.FailureRatePercent {
		severity := AlertSeverityWarning
		if metrics.FailureRate > m.alertThresholds.FailureRatePercent*2 {
			severity = AlertSeverityCritical
		}

		return &Alert{
			ID:       generateAlertID(AlertTypeHighFailureRate),
			Type:     AlertTypeHighFailureRate,
			Severity: severity,
			Title:    "High payment failure rate detected",
			Description: fmt.Sprintf("Payment failure rate is %.1f%% (threshold: %.1f%%)",
				metrics.FailureRate, m.alertThresholds.FailureRatePercent),
			Data: map[string]interface{}{
				"failure_rate":  metrics.FailureRate,
				"threshold":     m.alertThresholds.FailureRatePercent,
				"total_payments": metrics.TotalPaymentsLast24h,
				"failed_payments": metrics.FailedPaymentsLast24h,
			},
			Timestamp: time.Now(),
		}
	}
	return nil
}

// checkPastDueSubscriptions checks for subscriptions past due beyond grace period
func (m *PaymentMonitor) checkPastDueSubscriptions(ctx context.Context, metrics *PaymentHealthMetrics) *Alert {
	if metrics.PastDueCount > 0 {
		gracePeriodDays := int(m.alertThresholds.PastDueGracePeriod.Hours() / 24)
		pastDueSubs, err := m.subscriptionRepo.GetPastDueSubscriptions(ctx, gracePeriodDays)
		if err != nil {
			m.logger.Warn("Failed to get past due subscriptions for alert", zap.Error(err))
			return nil
		}

		if len(pastDueSubs) > 0 {
			return &Alert{
				ID:       generateAlertID(AlertTypePastDueSubscriptions),
				Type:     AlertTypePastDueSubscriptions,
				Severity: AlertSeverityWarning,
				Title:    "Subscriptions past due beyond grace period",
				Description: fmt.Sprintf("%d subscriptions have been past due for more than %d days",
					len(pastDueSubs), gracePeriodDays),
				Data: map[string]interface{}{
					"count":              len(pastDueSubs),
					"grace_period_days":  gracePeriodDays,
					"total_past_due":     metrics.PastDueCount,
				},
				Timestamp: time.Now(),
			}
		}
	}
	return nil
}

// checkRevenueDrop checks for significant revenue drops
func (m *PaymentMonitor) checkRevenueDrop(metrics *PaymentHealthMetrics) *Alert {
	if metrics.PreviousMRR > 0 {
		dropPercent := ((metrics.PreviousMRR - metrics.CurrentMRR) / metrics.PreviousMRR) * 100
		if dropPercent > m.alertThresholds.RevenueDropPercent {
			return &Alert{
				ID:       generateAlertID(AlertTypeRevenueDrop),
				Type:     AlertTypeRevenueDrop,
				Severity: AlertSeverityCritical,
				Title:    "Significant MRR drop detected",
				Description: fmt.Sprintf("MRR dropped by %.1f%% (from $%.2f to $%.2f)",
					dropPercent, metrics.PreviousMRR/100, metrics.CurrentMRR/100),
				Data: map[string]interface{}{
					"previous_mrr": metrics.PreviousMRR,
					"current_mrr":  metrics.CurrentMRR,
					"drop_percent": dropPercent,
					"threshold":    m.alertThresholds.RevenueDropPercent,
				},
				Timestamp: time.Now(),
			}
		}
	}
	return nil
}

// checkChurnRate checks for high churn rate
func (m *PaymentMonitor) checkChurnRate(metrics *PaymentHealthMetrics) *Alert {
	if metrics.ChurnRate > m.alertThresholds.ChurnRatePercent {
		severity := AlertSeverityWarning
		if metrics.ChurnRate > m.alertThresholds.ChurnRatePercent*2 {
			severity = AlertSeverityCritical
		}

		return &Alert{
			ID:       generateAlertID(AlertTypeHighChurnRate),
			Type:     AlertTypeHighChurnRate,
			Severity: severity,
			Title:    "High churn rate detected",
			Description: fmt.Sprintf("Churn rate is %.1f%% (threshold: %.1f%%)",
				metrics.ChurnRate, m.alertThresholds.ChurnRatePercent),
			Data: map[string]interface{}{
				"churn_rate": metrics.ChurnRate,
				"threshold":  m.alertThresholds.ChurnRatePercent,
			},
			Timestamp: time.Now(),
		}
	}
	return nil
}

// determineOverallHealth determines overall system health based on alerts
func (m *PaymentMonitor) determineOverallHealth(alerts []*Alert) HealthStatus {
	if len(alerts) == 0 {
		return HealthStatusHealthy
	}

	hasCritical := false
	for _, alert := range alerts {
		if alert.Severity == AlertSeverityCritical {
			hasCritical = true
			break
		}
	}

	if hasCritical {
		return HealthStatusUnhealthy
	}

	if len(alerts) > 2 {
		return HealthStatusDegraded
	}

	return HealthStatusDegraded
}

// generateRecommendations generates actionable recommendations based on the report
func (m *PaymentMonitor) generateRecommendations(report *MonitoringReport) []string {
	recommendations := []string{}

	for _, alert := range report.Alerts {
		switch alert.Type {
		case AlertTypePaymentFailure, AlertTypeHighFailureRate:
			recommendations = append(recommendations,
				"Review payment gateway logs for common failure patterns",
				"Check if card decline rates have increased",
				"Consider implementing retry logic for failed payments",
				"Review payment method requirements with users",
			)

		case AlertTypePastDueSubscriptions:
			recommendations = append(recommendations,
				"Send payment reminder emails to past due users",
				"Review dunning management strategy",
				"Consider offering payment plan options",
				"Check if payment methods need updating",
			)

		case AlertTypeRevenueDrop:
			recommendations = append(recommendations,
				"Review recent cancellations for patterns",
				"Analyze plan downgrades",
				"Consider retention campaign",
				"Review pricing strategy",
			)

		case AlertTypeHighChurnRate:
			recommendations = append(recommendations,
				"Conduct exit surveys to understand churn reasons",
				"Review product changes that may have caused churn",
				"Implement win-back campaigns",
				"Analyze churn by cohort and plan",
			)
		}
	}

	// Remove duplicates
	seen := make(map[string]bool)
	unique := []string{}
	for _, rec := range recommendations {
		if !seen[rec] {
			seen[rec] = true
			unique = append(unique, rec)
		}
	}

	return unique
}

// generateAlertID generates a unique alert ID
func generateAlertID(alertType AlertType) string {
	return fmt.Sprintf("%s_%d", alertType, time.Now().Unix())
}
