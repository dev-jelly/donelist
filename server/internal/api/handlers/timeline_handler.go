package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/dev-jelly/donelist/internal/api/middleware"
	"github.com/dev-jelly/donelist/internal/timeline"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// TimelineHandler handles timeline endpoints
type TimelineHandler struct {
	timelineService *timeline.Service
	logger          *zap.Logger
}

// NewTimelineHandler creates a new timeline handler
func NewTimelineHandler(timelineService *timeline.Service, logger *zap.Logger) *TimelineHandler {
	return &TimelineHandler{
		timelineService: timelineService,
		logger:          logger,
	}
}

// GetDaily retrieves check-ins for a specific day
// GET /timeline/daily?date=2006-01-02
func (h *TimelineHandler) GetDaily(c *gin.Context) {
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date format, use YYYY-MM-DD"})
		return
	}

	dayView, err := h.timelineService.GetDaily(c.Request.Context(), userID, date)
	if err != nil {
		h.logger.Error("Failed to get daily timeline", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get daily timeline"})
		return
	}

	c.JSON(http.StatusOK, dayView)
}

// GetDailyEnhanced retrieves an enhanced daily timeline with time blocks and analytics
// GET /timeline/daily/enhanced?date=2006-01-02&block=30&timezone=America/New_York
func (h *TimelineHandler) GetDailyEnhanced(c *gin.Context) {
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date format, use YYYY-MM-DD"})
		return
	}

	// Parse block granularity (default to 30 minutes)
	blockGranularity := 30
	if blockStr := c.Query("block"); blockStr != "" {
		if _, err := fmt.Sscanf(blockStr, "%d", &blockGranularity); err != nil {
			h.logger.Warn("Invalid block granularity", zap.String("block", blockStr), zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid block granularity, use 15, 30, 45, or 120"})
			return
		}
	}

	// Parse timezone (default to UTC)
	timezone := c.DefaultQuery("timezone", "UTC")

	enhancedView, err := h.timelineService.GetDailyEnhanced(c.Request.Context(), userID, date, blockGranularity, timezone)
	if err != nil {
		h.logger.Error("Failed to get enhanced daily timeline", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get enhanced daily timeline"})
		return
	}

	c.JSON(http.StatusOK, enhancedView)
}

// GetWeekly retrieves check-ins for a week, grouped by day
// GET /timeline/weekly?date=2006-01-02
func (h *TimelineHandler) GetWeekly(c *gin.Context) {
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date format, use YYYY-MM-DD"})
		return
	}

	weekView, err := h.timelineService.GetWeekly(c.Request.Context(), userID, date)
	if err != nil {
		h.logger.Error("Failed to get weekly timeline", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get weekly timeline"})
		return
	}

	c.JSON(http.StatusOK, weekView)
}

// GetMonthly retrieves check-ins for a month, grouped by day
// GET /timeline/monthly?date=2006-01-02
func (h *TimelineHandler) GetMonthly(c *gin.Context) {
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date format, use YYYY-MM-DD"})
		return
	}

	monthView, err := h.timelineService.GetMonthly(c.Request.Context(), userID, date)
	if err != nil {
		h.logger.Error("Failed to get monthly timeline", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get monthly timeline"})
		return
	}

	c.JSON(http.StatusOK, monthView)
}
