package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// MonitoringHandlers handles admin HTTP requests for monitoring
type MonitoringHandlers struct {
	monitor *PaymentMonitor
	logger  *zap.Logger
}

// NewMonitoringHandlers creates new monitoring handlers
func NewMonitoringHandlers(monitor *PaymentMonitor, logger *zap.Logger) *MonitoringHandlers {
	return &MonitoringHandlers{
		monitor: monitor,
		logger:  logger,
	}
}

// RegisterRoutes registers monitoring routes
func (h *MonitoringHandlers) RegisterRoutes(router *gin.RouterGroup) {
	monitoring := router.Group("/monitoring")
	{
		monitoring.GET("/health", h.GetHealthCheck)
		monitoring.GET("/alerts", h.GetAlerts)
	}
}

// GetHealthCheck handles GET /admin/monitoring/health
func (h *MonitoringHandlers) GetHealthCheck(c *gin.Context) {
	report, err := h.monitor.RunHealthCheck(c.Request.Context())
	if err != nil {
		h.logger.Error("Failed to run health check", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to run health check"})
		return
	}

	c.JSON(http.StatusOK, report)
}

// GetAlerts handles GET /admin/monitoring/alerts
func (h *MonitoringHandlers) GetAlerts(c *gin.Context) {
	report, err := h.monitor.RunHealthCheck(c.Request.Context())
	if err != nil {
		h.logger.Error("Failed to get alerts", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve alerts"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"alerts": report.Alerts,
		"count":  len(report.Alerts),
		"health": report.OverallHealth,
	})
}
