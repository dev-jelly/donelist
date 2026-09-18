package handlers

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dev-jelly/donelist/internal/api/middleware"
	"github.com/dev-jelly/donelist/internal/calendar"
	"github.com/dev-jelly/donelist/pkg/cache"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// CalendarHandler handles calendar view endpoints
type CalendarHandler struct {
	calendarService *calendar.Service
	cache           *cache.Cache  // Added for V2 functionality
	logger          *zap.Logger
}

// NewCalendarHandler creates a new calendar handler
func NewCalendarHandler(calendarService *calendar.Service, logger *zap.Logger) *CalendarHandler {
	return &CalendarHandler{
		calendarService: calendarService,
		logger:          logger,
	}
}

// SetCache sets the cache layer for V2 functionality
func (h *CalendarHandler) SetCache(cache *cache.Cache) {
	h.cache = cache
}

// GetMonthlyCalendar retrieves calendar view for a month
// @Summary Get monthly calendar view
// @Description Retrieves a complete monthly calendar view with daily summaries, streaks, and statistics
// @Tags calendar
// @Accept json
// @Produce json
// @Param year query int false "Year (default: current year)" example(2024)
// @Param month query int false "Month 1-12 (default: current month)" example(11)
// @Param start_day query string false "First day of week: sunday or monday (default: monday)" Enums(sunday, monday)
// @Param timezone query string false "IANA timezone name (default: UTC)" example(America/New_York)
// @Success 200 {object} calendar.MonthlyCalendar "Monthly calendar data"
// @Failure 400 {object} map[string]string "Invalid parameters"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /calendar/monthly [get]
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

// GetHeatmap retrieves heatmap visualization data for a date range
// @Summary Get heatmap data
// @Description Retrieves daily activity heatmap data for visualization (GitHub-style contribution graph)
// @Tags calendar
// @Accept json
// @Produce json
// @Param start_date query string true "Start date in YYYY-MM-DD format" example(2024-01-01)
// @Param end_date query string true "End date in YYYY-MM-DD format" example(2024-12-31)
// @Param timezone query string false "IANA timezone name (default: UTC)" example(America/New_York)
// @Success 200 {object} calendar.HeatmapData "Heatmap data"
// @Failure 400 {object} map[string]string "Invalid parameters or date range too large"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /calendar/heatmap [get]
func (h *CalendarHandler) GetHeatmap(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Parse start_date parameter
	startDateStr := c.Query("start_date")
	if startDateStr == "" {
		h.logger.Warn("Missing start_date parameter")
		c.JSON(http.StatusBadRequest, gin.H{"error": "start_date parameter is required (format: YYYY-MM-DD)"})
		return
	}

	startDate, err := time.Parse("2006-01-02", startDateStr)
	if err != nil {
		h.logger.Warn("Invalid start_date format", zap.String("start_date", startDateStr))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start_date format, use YYYY-MM-DD"})
		return
	}

	// Parse end_date parameter
	endDateStr := c.Query("end_date")
	if endDateStr == "" {
		h.logger.Warn("Missing end_date parameter")
		c.JSON(http.StatusBadRequest, gin.H{"error": "end_date parameter is required (format: YYYY-MM-DD)"})
		return
	}

	endDate, err := time.Parse("2006-01-02", endDateStr)
	if err != nil {
		h.logger.Warn("Invalid end_date format", zap.String("end_date", endDateStr))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid end_date format, use YYYY-MM-DD"})
		return
	}

	// Validate date range
	if endDate.Before(startDate) {
		h.logger.Warn("Invalid date range", zap.String("start_date", startDateStr), zap.String("end_date", endDateStr))
		c.JSON(http.StatusBadRequest, gin.H{"error": "end_date must be after start_date"})
		return
	}

	// Limit date range to 1 year for performance
	maxDays := 366
	daysDiff := int(endDate.Sub(startDate).Hours() / 24)
	if daysDiff > maxDays {
		h.logger.Warn("Date range too large", zap.Int("days", daysDiff), zap.Int("max_days", maxDays))
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("date range cannot exceed %d days", maxDays)})
		return
	}

	// Parse timezone parameter
	timezone := c.DefaultQuery("timezone", "UTC")

	// Get heatmap data
	heatmapData, err := h.calendarService.GetHeatmap(c.Request.Context(), calendar.HeatmapOptions{
		UserID:    userID,
		StartDate: startDate,
		EndDate:   endDate,
		Timezone:  timezone,
	})
	if err != nil {
		h.logger.Error("Failed to get heatmap data",
			zap.Error(err),
			zap.String("user_id", userID.String()),
			zap.String("start_date", startDateStr),
			zap.String("end_date", endDateStr),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get heatmap data"})
		return
	}

	// Set cache headers for heatmap data (cache for 1 hour)
	c.Header("Cache-Control", "public, max-age=3600")

	c.JSON(http.StatusOK, heatmapData)
}

// ============================================================================
// V2 METHODS WITH CACHING AND ETAG SUPPORT
// ============================================================================

// MonthlyCalendarResponseV2 wraps the calendar data with versioning
type MonthlyCalendarResponseV2 struct {
	SchemaVersion string                   `json:"schemaVersion"`
	Data          *calendar.MonthlyCalendar `json:"data"`
	ETag          string                   `json:"-"` // Not included in JSON, used for headers
}

// GetMonthlyCalendarV2 retrieves calendar view for a month with caching and ETags
// @Summary Get monthly calendar view (v2)
// @Description Retrieves a complete monthly calendar view with caching, ETags, and versioning
// @Tags calendar
// @Accept json
// @Produce json
// @Param year query int true "Year" example(2024)
// @Param month query int true "Month (1-12)" example(11) minimum(1) maximum(12)
// @Param startOfWeek query string false "Start of week: sun or mon (default: mon)" Enums(sun, mon)
// @Param timezone query string false "IANA timezone name (default: UTC)" example(America/New_York)
// @Param If-None-Match header string false "ETag value for conditional requests"
// @Success 200 {object} MonthlyCalendarResponseV2 "Monthly calendar data with version"
// @Success 304 "Not Modified - returned when ETag matches"
// @Failure 400 {object} map[string]string "Invalid parameters"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /api/v1/calendar/month [get]
func (h *CalendarHandler) GetMonthlyCalendarV2(c *gin.Context) {
	// Check if cache is available
	if h.cache == nil {
		h.logger.Warn("Cache not available, falling back to v1 behavior")
		h.GetMonthlyCalendar(c)
		return
	}

	// Extract user ID from auth middleware
	userID, err := middleware.GetUserID(c)
	if err != nil {
		h.logger.Error("Failed to get user ID", zap.Error(err))
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Parse and validate year parameter
	yearStr := c.Query("year")
	if yearStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "year parameter is required"})
		return
	}
	year, err := strconv.Atoi(yearStr)
	if err != nil || year < 1970 || year > 2100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid year, must be between 1970 and 2100"})
		return
	}

	// Parse and validate month parameter
	monthStr := c.Query("month")
	if monthStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "month parameter is required"})
		return
	}
	month, err := strconv.Atoi(monthStr)
	if err != nil || month < 1 || month > 12 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid month, must be between 1 and 12"})
		return
	}

	// Parse startOfWeek parameter
	startOfWeek := c.DefaultQuery("startOfWeek", "mon")
	var startDay calendar.StartDay
	switch startOfWeek {
	case "sun":
		startDay = calendar.StartDaySunday
	case "mon":
		startDay = calendar.StartDayMonday
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid startOfWeek, must be 'sun' or 'mon'"})
		return
	}

	// Parse timezone parameter
	timezone := c.DefaultQuery("timezone", "UTC")

	// Validate timezone
	if _, err := time.LoadLocation(timezone); err != nil {
		h.logger.Warn("Invalid timezone, defaulting to UTC",
			zap.String("timezone", timezone),
			zap.Error(err))
		timezone = "UTC"
	}

	// Current schema version
	schemaVersion := "1.0.0"

	// Build cache key including all parameters and version
	cacheKey := h.buildCacheKey(userID, year, month, string(startDay), schemaVersion)

	// Check if client has ETag
	clientETag := c.GetHeader("If-None-Match")
	if clientETag != "" {
		// Remove quotes if present
		clientETag = strings.Trim(clientETag, `"`)
	}

	// Try to get from cache first
	var cachedResponse MonthlyCalendarResponseV2

	ctx := c.Request.Context()
	err = h.cache.Get(ctx, cacheKey, &cachedResponse)
	if err == nil {
		// Cache hit - check ETag
		if clientETag != "" && clientETag == cachedResponse.ETag {
			// ETag matches - return 304 Not Modified
			h.logger.Debug("ETag match, returning 304",
				zap.String("user_id", userID.String()),
				zap.Int("year", year),
				zap.Int("month", month),
				zap.String("etag", clientETag))

			c.Header("ETag", fmt.Sprintf(`"%s"`, cachedResponse.ETag))
			c.Header("Cache-Control", "private, max-age=900") // 15 minutes
			c.Status(http.StatusNotModified)
			return
		}

		// Cache hit but ETag doesn't match - return cached data
		h.logger.Debug("Cache hit",
			zap.String("user_id", userID.String()),
			zap.Int("year", year),
			zap.Int("month", month))

		h.sendCalendarResponse(c, cachedResponse)
		return
	}

	// Cache miss - fetch from service
	h.logger.Debug("Cache miss, fetching from service",
		zap.String("user_id", userID.String()),
		zap.Int("year", year),
		zap.Int("month", month))

	calendarData, err := h.calendarService.GetMonthlyCalendar(ctx, calendar.GetOptions{
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
			zap.Int("month", month))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get monthly calendar"})
		return
	}

	// Build response with versioning
	response := MonthlyCalendarResponseV2{
		SchemaVersion: schemaVersion,
		Data:          calendarData,
	}

	// Generate ETag from response data
	response.ETag = h.generateETag(response)

	// Store in cache with TTL of 15 minutes
	ttl := 15 * time.Minute
	if err := h.cache.Set(ctx, cacheKey, response, ttl); err != nil {
		// Log error but don't fail the request
		h.logger.Warn("Failed to cache calendar response",
			zap.Error(err),
			zap.String("cache_key", cacheKey))
	}

	// Send response
	h.sendCalendarResponse(c, response)
}

// buildCacheKey creates a cache key for the calendar request
func (h *CalendarHandler) buildCacheKey(userID uuid.UUID, year, month int, startOfWeek, version string) string {
	return fmt.Sprintf("calendar:v2:%s:%d-%02d:%s:%s",
		userID.String(), year, month, startOfWeek, version)
}

// generateETag generates an ETag from the response data
func (h *CalendarHandler) generateETag(response MonthlyCalendarResponseV2) string {
	// Serialize response data for hashing
	data, err := json.Marshal(response)
	if err != nil {
		// Fallback to timestamp-based ETag
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}

	// Generate MD5 hash
	hasher := md5.New()
	hasher.Write(data)
	return hex.EncodeToString(hasher.Sum(nil))
}

// sendCalendarResponse sends the calendar response with appropriate headers
func (h *CalendarHandler) sendCalendarResponse(c *gin.Context, response MonthlyCalendarResponseV2) {
	// Set caching headers
	c.Header("ETag", fmt.Sprintf(`"%s"`, response.ETag))
	c.Header("Cache-Control", "private, max-age=900") // 15 minutes
	c.Header("X-Schema-Version", response.SchemaVersion)

	// Set expiry time
	expiryTime := time.Now().Add(15 * time.Minute)
	c.Header("Expires", expiryTime.UTC().Format(http.TimeFormat))

	// Send JSON response
	c.JSON(http.StatusOK, response)
}

// InvalidateUserCalendarCache invalidates all calendar cache entries for a user
// This should be called when check-ins are created, updated, or deleted
func (h *CalendarHandler) InvalidateUserCalendarCache(userID uuid.UUID) error {
	if h.cache == nil {
		return nil // No cache configured
	}

	ctx := context.Background()
	pattern := fmt.Sprintf("calendar:v2:%s:*", userID.String())

	h.logger.Debug("Invalidating calendar cache for user",
		zap.String("user_id", userID.String()),
		zap.String("pattern", pattern))

	if err := h.cache.DeletePattern(ctx, pattern); err != nil {
		h.logger.Error("Failed to invalidate calendar cache",
			zap.Error(err),
			zap.String("user_id", userID.String()))
		return err
	}

	return nil
}

// InvalidateMonthCalendarCache invalidates calendar cache for a specific month
func (h *CalendarHandler) InvalidateMonthCalendarCache(userID uuid.UUID, year, month int) error {
	if h.cache == nil {
		return nil // No cache configured
	}

	ctx := context.Background()
	pattern := fmt.Sprintf("calendar:v2:%s:%d-%02d:*", userID.String(), year, month)

	h.logger.Debug("Invalidating month calendar cache",
		zap.String("user_id", userID.String()),
		zap.Int("year", year),
		zap.Int("month", month))

	if err := h.cache.DeletePattern(ctx, pattern); err != nil {
		h.logger.Error("Failed to invalidate month calendar cache",
			zap.Error(err),
			zap.String("user_id", userID.String()),
			zap.Int("year", year),
			zap.Int("month", month))
		return err
	}

	return nil
}
