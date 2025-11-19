package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/dev-jelly/donelist/internal/performance"
)

// PerformanceHandler handles performance monitoring endpoints
type PerformanceHandler struct {
	monitor *performance.Monitor
	logger  *zap.Logger
}

// NewPerformanceHandler creates a new performance handler
func NewPerformanceHandler(db *sqlx.DB, logger *zap.Logger) *PerformanceHandler {
	return &PerformanceHandler{
		monitor: performance.NewMonitor(db),
		logger:  logger,
	}
}

// RegisterRoutes registers performance monitoring routes
func (h *PerformanceHandler) RegisterRoutes(router *gin.RouterGroup) {
	perf := router.Group("/performance")
	{
		perf.GET("/health", h.GetHealth)
		perf.GET("/stats", h.GetStats)
		perf.GET("/slow-queries", h.GetSlowQueries)
		perf.GET("/table-stats", h.GetTableStats)
		perf.GET("/index-stats", h.GetIndexStats)
		perf.GET("/baseline", h.GetBaseline)
		perf.POST("/capture-baseline", h.CaptureBaseline)
		perf.POST("/capture-snapshot", h.CaptureSnapshot)
	}
}

// GetHealth returns current performance health status
// @Summary Get performance health
// @Description Get current database performance health status
// @Tags performance
// @Produce json
// @Success 200 {object} performance.HealthStatus
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/performance/health [get]
func (h *PerformanceHandler) GetHealth(c *gin.Context) {
	health, err := h.monitor.GetPerformanceHealth(c.Request.Context())
	if err != nil {
		h.logger.Error("Failed to get performance health", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to get performance health",
		})
		return
	}

	c.JSON(http.StatusOK, health)
}

// GetStats returns current database statistics
// @Summary Get database statistics
// @Description Get current database statistics including connections, cache hit ratio, etc.
// @Tags performance
// @Produce json
// @Success 200 {object} performance.CurrentDBStats
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/performance/stats [get]
func (h *PerformanceHandler) GetStats(c *gin.Context) {
	stats, err := h.monitor.GetCurrentDBStats(c.Request.Context())
	if err != nil {
		h.logger.Error("Failed to get database stats", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to get database stats",
		})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// GetSlowQueries returns top slow queries
// @Summary Get slow queries
// @Description Get list of slowest queries
// @Tags performance
// @Produce json
// @Param limit query int false "Number of queries to return" default(50)
// @Success 200 {array} performance.SlowQuery
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/performance/slow-queries [get]
func (h *PerformanceHandler) GetSlowQueries(c *gin.Context) {
	limit := 50
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	queries, err := h.monitor.GetTopSlowQueries(c.Request.Context(), limit)
	if err != nil {
		h.logger.Error("Failed to get slow queries", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to get slow queries",
		})
		return
	}

	c.JSON(http.StatusOK, queries)
}

// GetTableStats returns table statistics
// @Summary Get table statistics
// @Description Get statistics for all tables including scans, rows, and dead row percentage
// @Tags performance
// @Produce json
// @Success 200 {array} performance.TableStats
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/performance/table-stats [get]
func (h *PerformanceHandler) GetTableStats(c *gin.Context) {
	stats, err := h.monitor.GetTableStats(c.Request.Context())
	if err != nil {
		h.logger.Error("Failed to get table stats", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to get table stats",
		})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// GetIndexStats returns index usage statistics
// @Summary Get index statistics
// @Description Get index usage statistics to identify unused indexes
// @Tags performance
// @Produce json
// @Success 200 {array} performance.IndexUsageStats
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/performance/index-stats [get]
func (h *PerformanceHandler) GetIndexStats(c *gin.Context) {
	stats, err := h.monitor.GetIndexUsageStats(c.Request.Context())
	if err != nil {
		h.logger.Error("Failed to get index stats", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to get index stats",
		})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// GetBaseline returns baseline metrics
// @Summary Get baseline metrics
// @Description Get performance baseline metrics for the specified number of days
// @Tags performance
// @Produce json
// @Param days query int false "Number of days to retrieve" default(7)
// @Success 200 {array} performance.BaselineMetrics
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/performance/baseline [get]
func (h *PerformanceHandler) GetBaseline(c *gin.Context) {
	days := 7
	if daysStr := c.Query("days"); daysStr != "" {
		if parsedDays, err := strconv.Atoi(daysStr); err == nil && parsedDays > 0 {
			days = parsedDays
		}
	}

	metrics, err := h.monitor.GetBaselineMetrics(c.Request.Context(), days)
	if err != nil {
		h.logger.Error("Failed to get baseline metrics", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to get baseline metrics",
		})
		return
	}

	c.JSON(http.StatusOK, metrics)
}

// CaptureBaseline manually triggers baseline metrics capture
// @Summary Capture baseline metrics
// @Description Manually trigger capture of current performance baseline
// @Tags performance
// @Produce json
// @Success 200 {object} MessageResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/performance/capture-baseline [post]
func (h *PerformanceHandler) CaptureBaseline(c *gin.Context) {
	err := h.monitor.CaptureBaselineMetrics(c.Request.Context())
	if err != nil {
		h.logger.Error("Failed to capture baseline metrics", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to capture baseline metrics",
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Message: "Baseline metrics captured successfully",
	})
}

// CaptureSnapshot manually triggers query stats snapshot
// @Summary Capture query stats snapshot
// @Description Manually trigger capture of current query statistics
// @Tags performance
// @Produce json
// @Success 200 {object} SuccessResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/performance/capture-snapshot [post]
func (h *PerformanceHandler) CaptureSnapshot(c *gin.Context) {
	err := h.monitor.CaptureQueryStatsSnapshot(c.Request.Context())
	if err != nil {
		h.logger.Error("Failed to capture query stats snapshot", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to capture query stats snapshot",
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Message: "Query stats snapshot captured successfully",
	})
}
