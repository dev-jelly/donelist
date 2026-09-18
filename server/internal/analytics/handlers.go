package analytics

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

// Handlers provides HTTP handlers for analytics endpoints
type Handlers struct {
	service *Service
	logger  *zap.Logger
}

// NewHandlers creates new analytics HTTP handlers
func NewHandlers(service *Service, logger *zap.Logger) *Handlers {
	return &Handlers{
		service: service,
		logger:  logger,
	}
}

// RegisterRoutes registers all analytics HTTP routes
func (h *Handlers) RegisterRoutes(r *mux.Router) {
	// Analytics summary endpoints
	r.HandleFunc("/api/v1/analytics/summary", h.GetSummary).Methods("GET")
	r.HandleFunc("/api/v1/analytics/daily/{date}", h.GetDailyActivity).Methods("GET")
	r.HandleFunc("/api/v1/analytics/weekly", h.GetWeeklyActivity).Methods("GET")
	r.HandleFunc("/api/v1/analytics/monthly-comparison", h.GetMonthlyComparison).Methods("GET")

	// Category analytics
	r.HandleFunc("/api/v1/analytics/categories", h.GetCategoryPerformance).Methods("GET")

	// Streak analytics
	r.HandleFunc("/api/v1/analytics/streaks", h.GetStreakMetrics).Methods("GET")

	// Real-time metrics
	r.HandleFunc("/api/v1/analytics/realtime", h.GetRealTimeMetrics).Methods("GET")

	// Cache management
	r.HandleFunc("/api/v1/analytics/cache/invalidate", h.InvalidateCache).Methods("POST")
	r.HandleFunc("/api/v1/analytics/cache/stats", h.GetCacheStats).Methods("GET")

	// Materialized views
	r.HandleFunc("/api/v1/analytics/refresh", h.RefreshMaterializedViews).Methods("POST")
}

// GetSummary handles GET /api/v1/analytics/summary
// @Summary Get analytics summary
// @Description Get comprehensive analytics summary for the user
// @Tags Analytics
// @Accept json
// @Produce json
// @Security Bearer
// @Param period query string false "Period (week, month, quarter)" default(month)
// @Success 200 {object} map[string]interface{} "Analytics summary"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /api/v1/analytics/summary [get]
func (h *Handlers) GetSummary(w http.ResponseWriter, r *http.Request) {
	userID := h.getUserID(r)
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "month"
	}

	ctx := r.Context()

	// Get streak metrics
	streaks, err := h.service.GetStreakMetrics(ctx, userID)
	if err != nil {
		h.logger.Error("Failed to get streak metrics", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "Failed to get streak metrics")
		return
	}

	// Get category performance
	categories, err := h.service.GetCategoryPerformance(ctx, userID, 10)
	if err != nil {
		h.logger.Error("Failed to get category performance", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "Failed to get category performance")
		return
	}

	// Get weekly activity
	now := time.Now()
	weekStart := now.AddDate(0, 0, -int(now.Weekday()))
	weekStart = time.Date(weekStart.Year(), weekStart.Month(), weekStart.Day(), 0, 0, 0, 0, time.UTC)

	weeklyActivity, err := h.service.GetWeeklyActivity(ctx, userID, weekStart)
	if err != nil {
		h.logger.Error("Failed to get weekly activity", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "Failed to get weekly activity")
		return
	}

	summary := map[string]interface{}{
		"period":           period,
		"generated_at":     time.Now(),
		"streaks":          streaks,
		"top_categories":   categories,
		"weekly_activity":  weeklyActivity,
	}

	h.respondWithJSON(w, http.StatusOK, summary)
}

// GetDailyActivity handles GET /api/v1/analytics/daily/{date}
// @Summary Get daily activity
// @Description Get detailed activity metrics for a specific day
// @Tags Analytics
// @Accept json
// @Produce json
// @Security Bearer
// @Param date path string true "Date (YYYY-MM-DD)"
// @Success 200 {object} DailyActivitySummary "Daily activity summary"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /api/v1/analytics/daily/{date} [get]
func (h *Handlers) GetDailyActivity(w http.ResponseWriter, r *http.Request) {
	userID := h.getUserID(r)

	vars := mux.Vars(r)
	dateStr := vars["date"]

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		h.respondWithError(w, http.StatusBadRequest, "Invalid date format. Use YYYY-MM-DD")
		return
	}

	ctx := r.Context()
	activity, err := h.service.GetDailyActivity(ctx, userID, date)
	if err != nil {
		h.logger.Error("Failed to get daily activity",
			zap.String("user_id", userID.String()),
			zap.Time("date", date),
			zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "Failed to get daily activity")
		return
	}

	if activity == nil {
		h.respondWithJSON(w, http.StatusOK, map[string]interface{}{
			"date":    dateStr,
			"message": "No activity for this date",
		})
		return
	}

	h.respondWithJSON(w, http.StatusOK, activity)
}

// GetWeeklyActivity handles GET /api/v1/analytics/weekly
// @Summary Get weekly activity
// @Description Get weekly activity report with daily breakdown
// @Tags Analytics
// @Accept json
// @Produce json
// @Security Bearer
// @Param week_start query string false "Week start date (YYYY-MM-DD)" default(current week)
// @Success 200 {object} WeeklyActivityReport "Weekly activity report"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /api/v1/analytics/weekly [get]
func (h *Handlers) GetWeeklyActivity(w http.ResponseWriter, r *http.Request) {
	userID := h.getUserID(r)

	weekStartStr := r.URL.Query().Get("week_start")
	var weekStart time.Time

	if weekStartStr != "" {
		var err error
		weekStart, err = time.Parse("2006-01-02", weekStartStr)
		if err != nil {
			h.respondWithError(w, http.StatusBadRequest, "Invalid week_start format. Use YYYY-MM-DD")
			return
		}
	} else {
		// Default to current week (Monday start)
		now := time.Now()
		weekStart = now.AddDate(0, 0, -int(now.Weekday())+1)
		weekStart = time.Date(weekStart.Year(), weekStart.Month(), weekStart.Day(), 0, 0, 0, 0, time.UTC)
	}

	ctx := r.Context()
	report, err := h.service.GetWeeklyActivity(ctx, userID, weekStart)
	if err != nil {
		h.logger.Error("Failed to get weekly activity",
			zap.String("user_id", userID.String()),
			zap.Time("week_start", weekStart),
			zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "Failed to get weekly activity")
		return
	}

	h.respondWithJSON(w, http.StatusOK, report)
}

// GetMonthlyComparison handles GET /api/v1/analytics/monthly-comparison
// @Summary Get monthly comparison
// @Description Get month-over-month comparison report
// @Tags Analytics
// @Accept json
// @Produce json
// @Security Bearer
// @Param month query string false "Month (YYYY-MM)" default(current month)
// @Success 200 {object} MonthlyComparisonReport "Monthly comparison report"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /api/v1/analytics/monthly-comparison [get]
func (h *Handlers) GetMonthlyComparison(w http.ResponseWriter, r *http.Request) {
	userID := h.getUserID(r)

	monthStr := r.URL.Query().Get("month")
	var month time.Time

	if monthStr != "" {
		var err error
		month, err = time.Parse("2006-01", monthStr)
		if err != nil {
			h.respondWithError(w, http.StatusBadRequest, "Invalid month format. Use YYYY-MM")
			return
		}
	} else {
		// Default to current month
		now := time.Now()
		month = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	}

	ctx := r.Context()
	report, err := h.service.GetMonthlyComparison(ctx, userID, month)
	if err != nil {
		h.logger.Error("Failed to get monthly comparison",
			zap.String("user_id", userID.String()),
			zap.Time("month", month),
			zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "Failed to get monthly comparison")
		return
	}

	h.respondWithJSON(w, http.StatusOK, report)
}

// GetCategoryPerformance handles GET /api/v1/analytics/categories
// @Summary Get category performance
// @Description Get performance metrics for all categories
// @Tags Analytics
// @Accept json
// @Produce json
// @Security Bearer
// @Param limit query int false "Limit results" default(20)
// @Success 200 {array} CategoryPerformance "Category performance metrics"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /api/v1/analytics/categories [get]
func (h *Handlers) GetCategoryPerformance(w http.ResponseWriter, r *http.Request) {
	userID := h.getUserID(r)

	limit := 20
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	ctx := r.Context()
	performance, err := h.service.GetCategoryPerformance(ctx, userID, limit)
	if err != nil {
		h.logger.Error("Failed to get category performance",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "Failed to get category performance")
		return
	}

	h.respondWithJSON(w, http.StatusOK, performance)
}

// GetStreakMetrics handles GET /api/v1/analytics/streaks
// @Summary Get streak metrics
// @Description Get detailed streak information for the user
// @Tags Analytics
// @Accept json
// @Produce json
// @Security Bearer
// @Success 200 {object} StreakMetrics "Streak metrics"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /api/v1/analytics/streaks [get]
func (h *Handlers) GetStreakMetrics(w http.ResponseWriter, r *http.Request) {
	userID := h.getUserID(r)

	ctx := r.Context()
	metrics, err := h.service.GetStreakMetrics(ctx, userID)
	if err != nil {
		h.logger.Error("Failed to get streak metrics",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "Failed to get streak metrics")
		return
	}

	h.respondWithJSON(w, http.StatusOK, metrics)
}

// GetRealTimeMetrics handles GET /api/v1/analytics/realtime
// @Summary Get real-time metrics
// @Description Get real-time analytics metrics from Redis
// @Tags Analytics
// @Accept json
// @Produce json
// @Security Bearer
// @Success 200 {object} map[string]interface{} "Real-time metrics"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /api/v1/analytics/realtime [get]
func (h *Handlers) GetRealTimeMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	metrics, err := h.service.GetRealTimeMetrics(ctx)
	if err != nil {
		h.logger.Error("Failed to get real-time metrics", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "Failed to get real-time metrics")
		return
	}

	h.respondWithJSON(w, http.StatusOK, metrics)
}

// InvalidateCache handles POST /api/v1/analytics/cache/invalidate
// @Summary Invalidate analytics cache
// @Description Invalidate analytics cache for the current user
// @Tags Analytics
// @Accept json
// @Produce json
// @Security Bearer
// @Success 200 {object} map[string]string "Success message"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /api/v1/analytics/cache/invalidate [post]
func (h *Handlers) InvalidateCache(w http.ResponseWriter, r *http.Request) {
	userID := h.getUserID(r)

	ctx := r.Context()
	err := h.service.InvalidateUserAnalytics(ctx, userID)
	if err != nil {
		h.logger.Error("Failed to invalidate cache",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "Failed to invalidate cache")
		return
	}

	h.respondWithJSON(w, http.StatusOK, map[string]string{
		"message": "Cache invalidated successfully",
	})
}

// GetCacheStats handles GET /api/v1/analytics/cache/stats
// @Summary Get cache statistics
// @Description Get analytics cache statistics
// @Tags Analytics
// @Accept json
// @Produce json
// @Security Bearer
// @Success 200 {object} map[string]interface{} "Cache statistics"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /api/v1/analytics/cache/stats [get]
func (h *Handlers) GetCacheStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	stats, err := h.service.GetCacheStats(ctx)
	if err != nil {
		h.logger.Error("Failed to get cache stats", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "Failed to get cache stats")
		return
	}

	h.respondWithJSON(w, http.StatusOK, stats)
}

// RefreshMaterializedViews handles POST /api/v1/analytics/refresh
// @Summary Refresh materialized views
// @Description Manually trigger refresh of analytics materialized views
// @Tags Analytics
// @Accept json
// @Produce json
// @Security Bearer
// @Param realtime_only query bool false "Refresh only real-time views" default(false)
// @Success 200 {object} map[string]string "Success message"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /api/v1/analytics/refresh [post]
func (h *Handlers) RefreshMaterializedViews(w http.ResponseWriter, r *http.Request) {
	realtimeOnly := r.URL.Query().Get("realtime_only") == "true"

	ctx := r.Context()
	var err error

	if realtimeOnly {
		err = h.service.RefreshRealtimeViews(ctx)
	} else {
		err = h.service.RefreshMaterializedViews(ctx)
	}

	if err != nil {
		h.logger.Error("Failed to refresh materialized views", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "Failed to refresh materialized views")
		return
	}

	message := "All materialized views refreshed successfully"
	if realtimeOnly {
		message = "Real-time materialized views refreshed successfully"
	}

	h.respondWithJSON(w, http.StatusOK, map[string]string{
		"message": message,
	})
}

// Helper methods

func (h *Handlers) getUserID(r *http.Request) uuid.UUID {
	// Extract user ID from context (set by authentication middleware)
	userID, ok := r.Context().Value("user_id").(uuid.UUID)
	if !ok {
		// Fallback - this should not happen if auth middleware is properly configured
		h.logger.Warn("User ID not found in context")
		return uuid.Nil
	}
	return userID
}

func (h *Handlers) respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, err := json.Marshal(payload)
	if err != nil {
		h.logger.Error("Failed to marshal response", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}

func (h *Handlers) respondWithError(w http.ResponseWriter, code int, message string) {
	h.respondWithJSON(w, code, map[string]string{"error": message})
}
