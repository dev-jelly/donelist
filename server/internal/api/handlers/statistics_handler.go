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
// GET /statistics/weekly?date=2006-01-02&week_start=monday&timezone=America/New_York
//
// Query Parameters:
// - date: Optional. Any date within the desired week (YYYY-MM-DD format). Defaults to current date.
// - week_start: Optional. First day of week: "monday" or "sunday". Defaults to "monday".
// - timezone: Optional. IANA timezone (e.g., "America/New_York", "Asia/Seoul"). Defaults to UTC.
//
// Response: WeeklyStatistics object with comprehensive analysis including:
// - Weekly summary (totals, averages, completion rate)
// - Daily breakdown (7 days with individual stats)
// - Day-of-week analysis (which days are most productive)
// - Time distribution (morning/afternoon/evening/night patterns)
// - Category breakdown (top categories by count and duration)
// - Week-over-week comparison (vs previous week)
// - Streak information (current streak, longest streak, milestones)
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
