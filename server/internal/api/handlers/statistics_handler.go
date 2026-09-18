package handlers

import (
	"net/http"
	"time"

	"github.com/dev-jelly/donelist/internal/api/middleware"
	"github.com/dev-jelly/donelist/internal/statistics"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// StatisticsHandler handles statistics endpoints
type StatisticsHandler struct {
	statisticsService *statistics.Service
	logger            *zap.Logger
}

// NewStatisticsHandler creates a new statistics handler
func NewStatisticsHandler(statisticsService *statistics.Service, logger *zap.Logger) *StatisticsHandler {
	return &StatisticsHandler{
		statisticsService: statisticsService,
		logger:            logger,
	}
}

// GetWeeklyStatistics retrieves comprehensive weekly statistics and analysis
// @Summary Get weekly statistics
// @Description Retrieves comprehensive weekly statistics including productivity metrics, patterns, trends, and comparisons
// @Tags statistics
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param date query string false "Any date within the desired week (YYYY-MM-DD format). Defaults to current date." example(2024-01-15)
// @Param week_start query string false "First day of week: 'monday' or 'sunday'. Defaults to 'monday'." default(monday) Enums(monday, sunday)
// @Param timezone query string false "IANA timezone (e.g., 'America/New_York', 'Asia/Seoul'). Defaults to UTC." default(UTC) example(America/New_York)
// @Success 200 {object} statistics.WeeklyStatistics "Comprehensive weekly statistics with summary, daily breakdown, patterns, and trends"
// @Failure 400 {object} map[string]string "Invalid request parameters (invalid date format, week_start, or timezone)"
// @Failure 401 {object} map[string]string "Unauthorized - missing or invalid authentication token"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /statistics/weekly [get]
func (h *StatisticsHandler) GetWeeklyStatistics(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Parse date parameter (default to today)
	dateStr := c.DefaultQuery("date", time.Now().Format("2006-01-02"))
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		h.logger.Warn("Invalid date format", zap.String("date", dateStr), zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid date format, use YYYY-MM-DD",
		})
		return
	}

	// Parse week start day (default to Monday)
	weekStartStr := c.DefaultQuery("week_start", "monday")
	var weekStart statistics.WeekStartDay
	switch weekStartStr {
	case "sunday":
		weekStart = statistics.WeekStartSunday
	case "monday":
		weekStart = statistics.WeekStartMonday
	default:
		h.logger.Warn("Invalid week_start parameter", zap.String("week_start", weekStartStr))
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid week_start parameter, use 'monday' or 'sunday'",
		})
		return
	}

	// Parse timezone (default to UTC)
	timezone := c.DefaultQuery("timezone", "UTC")

	// Validate timezone
	if _, err := time.LoadLocation(timezone); err != nil {
		h.logger.Warn("Invalid timezone", zap.String("timezone", timezone), zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid timezone, use IANA timezone format (e.g., 'America/New_York')",
		})
		return
	}

	// Get weekly statistics
	stats, err := h.statisticsService.GetWeeklyStatistics(c.Request.Context(), statistics.GetWeeklyOptions{
		UserID:       userID,
		Date:         date,
		WeekStartDay: weekStart,
		Timezone:     timezone,
	})
	if err != nil {
		h.logger.Error("Failed to get weekly statistics", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get weekly statistics",
		})
		return
	}

	c.JSON(http.StatusOK, stats)
}
