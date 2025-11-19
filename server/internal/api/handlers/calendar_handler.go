package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/dev-jelly/donelist/internal/api/middleware"
	"github.com/dev-jelly/donelist/internal/calendar"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// CalendarHandler handles calendar view endpoints
type CalendarHandler struct {
	calendarService *calendar.Service
	logger          *zap.Logger
}

// NewCalendarHandler creates a new calendar handler
func NewCalendarHandler(calendarService *calendar.Service, logger *zap.Logger) *CalendarHandler {
	return &CalendarHandler{
		calendarService: calendarService,
		logger:          logger,
	}
}

// GetMonthlyCalendar retrieves calendar view for a month
// GET /calendar/monthly?year=2024&month=11&start_day=monday&timezone=America/New_York
// Query parameters:
//   - year: Year (default: current year)
//   - month: Month 1-12 (default: current month)
//   - start_day: "sunday" or "monday" (default: "monday")
//   - timezone: IANA timezone name (default: "UTC")
func (h *CalendarHandler) GetMonthlyCalendar(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Get current date for defaults
	now := time.Now()

	// Parse year parameter
	year := now.Year()
	if yearStr := c.Query("year"); yearStr != "" {
		if parsed, err := strconv.Atoi(yearStr); err == nil {
			year = parsed
		}
	}

	// Parse month parameter
	month := int(now.Month())
	if monthStr := c.Query("month"); monthStr != "" {
		if parsed, err := strconv.Atoi(monthStr); err == nil && parsed >= 1 && parsed <= 12 {
			month = parsed
		}
	}

	// Parse start_day parameter
	startDay := calendar.StartDayMonday
	if startDayStr := c.Query("start_day"); startDayStr != "" {
		if startDayStr == "sunday" {
			startDay = calendar.StartDaySunday
		} else if startDayStr == "monday" {
			startDay = calendar.StartDayMonday
		}
	}

	// Parse timezone parameter
	timezone := c.DefaultQuery("timezone", "UTC")

	// Validate year and month ranges
	if year < 1970 || year > 2100 {
		h.logger.Warn("Invalid year", zap.Int("year", year))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid year, must be between 1970 and 2100"})
		return
	}

	if month < 1 || month > 12 {
		h.logger.Warn("Invalid month", zap.Int("month", month))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid month, must be between 1 and 12"})
		return
	}

	// Get calendar data
	calendarView, err := h.calendarService.GetMonthlyCalendar(c.Request.Context(), calendar.GetOptions{
		UserID:   userID,
		Year:     year,
		Month:    month,
		StartDay: startDay,
		Timezone: timezone,
	})
	if err != nil {
		h.logger.Error("Failed to get monthly calendar",
			zap.Error(err),
			zap.String("user_id", userID.String()),
			zap.Int("year", year),
			zap.Int("month", month),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get monthly calendar"})
		return
	}

	// Set cache headers based on cache expiration
	if !calendarView.CacheExpiration.IsZero() {
		c.Header("Cache-Control", "public, max-age=3600") // Cache for 1 hour
		c.Header("Expires", calendarView.CacheExpiration.Format(http.TimeFormat))
	}

	c.JSON(http.StatusOK, calendarView)
}
