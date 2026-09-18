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
// GET /timeline/daily/enhanced?date=2006-01-02&block=30&timezone=America/New_York&cursor=xyz&limit=48
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

	// Parse pagination parameters
	cursor := c.Query("cursor")
	limit := 0
	if limitStr := c.Query("limit"); limitStr != "" {
		if _, err := fmt.Sscanf(limitStr, "%d", &limit); err != nil {
			h.logger.Warn("Invalid limit", zap.String("limit", limitStr), zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit parameter"})
			return
		}
		if limit < 0 || limit > 1000 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be between 0 and 1000"})
			return
		}
	}

	enhancedView, err := h.timelineService.GetDailyEnhancedPaginated(c.Request.Context(), userID, date, blockGranularity, timezone, cursor, limit)
	if err != nil {
		h.logger.Error("Failed to get enhanced daily timeline", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get enhanced daily timeline"})
		return
	}

	// Check for If-None-Match header (ETag support)
	if ifNoneMatch := c.GetHeader("If-None-Match"); ifNoneMatch != "" {
		// For non-paginated requests with cache
		if cursor == "" && limit == 0 {
			// Compare with current ETag
			// Note: ETag generation should be consistent
			// We'll generate it from the view for comparison
			// In production, you'd use the cached ETag
			c.Header("ETag", fmt.Sprintf(`"%s"`, enhancedView.GeneratedAt.Format(time.RFC3339)))
			if ifNoneMatch == fmt.Sprintf(`"%s"`, enhancedView.GeneratedAt.Format(time.RFC3339)) {
				c.Status(http.StatusNotModified)
				return
			}
		}
	}

	// Set cache headers
	if cursor == "" && limit == 0 {
		// Only cache non-paginated requests
		c.Header("Cache-Control", "private, max-age=300") // 5 minutes
		c.Header("ETag", fmt.Sprintf(`"%s"`, enhancedView.GeneratedAt.Format(time.RFC3339)))
		c.Header("Last-Modified", enhancedView.GeneratedAt.Format(http.TimeFormat))
	} else {
		// Don't cache paginated requests
		c.Header("Cache-Control", "no-cache")
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
