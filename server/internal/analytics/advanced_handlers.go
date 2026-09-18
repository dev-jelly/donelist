package analytics

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

// AdvancedHandlers provides HTTP handlers for advanced analytics endpoints (Premium feature)
type AdvancedHandlers struct {
	service          *Service
	insightsEngine   *InsightsEngine
	predictiveEngine *PredictiveEngine
	logger           *zap.Logger
}

// NewAdvancedHandlers creates new advanced analytics HTTP handlers
func NewAdvancedHandlers(service *Service, logger *zap.Logger) *AdvancedHandlers {
	return &AdvancedHandlers{
		service:          service,
		insightsEngine:   NewInsightsEngine(service, logger),
		predictiveEngine: NewPredictiveEngine(service, logger),
		logger:           logger,
	}
}

// RegisterRoutes registers all advanced analytics HTTP routes
// Note: These routes should be wrapped with premium middleware
func (h *AdvancedHandlers) RegisterRoutes(r *mux.Router) {
	// Premium analytics endpoints
	r.HandleFunc("/api/v1/analytics/insights", h.GetInsights).Methods("GET")
	r.HandleFunc("/api/v1/analytics/patterns", h.GetPatterns).Methods("GET")
	r.HandleFunc("/api/v1/analytics/recommendations", h.GetRecommendations).Methods("GET")
	r.HandleFunc("/api/v1/analytics/focus-score", h.GetFocusScore).Methods("GET")

	// Predictive analytics
	r.HandleFunc("/api/v1/analytics/forecast", h.GetProductivityForecast).Methods("GET")
	r.HandleFunc("/api/v1/analytics/predict-days", h.PredictProductiveDays).Methods("GET")

	// Report generation
	r.HandleFunc("/api/v1/analytics/reports/generate", h.GenerateReport).Methods("POST")
}

// GetInsights handles GET /api/v1/analytics/insights
// @Summary Get AI-powered insights
// @Description Get comprehensive AI-powered insights about user behavior and patterns (Premium)
// @Tags Analytics
// @Accept json
// @Produce json
// @Security Bearer
// @Success 200 {array} Insight "List of insights"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 403 {object} map[string]string "Premium feature required"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /api/v1/analytics/insights [get]
func (h *AdvancedHandlers) GetInsights(w http.ResponseWriter, r *http.Request) {
	userID := h.getUserID(r)
	ctx := r.Context()

	insights, err := h.insightsEngine.GenerateInsights(ctx, userID)
	if err != nil {
		h.logger.Error("Failed to generate insights",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "Failed to generate insights")
		return
	}

	h.respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"insights":     insights,
		"total_count":  len(insights),
		"generated_at": insights[0].GeneratedAt,
	})
}

// GetPatterns handles GET /api/v1/analytics/patterns
// @Summary Get behavioral patterns
// @Description Detect and return behavioral patterns in user activity (Premium)
// @Tags Analytics
// @Accept json
// @Produce json
// @Security Bearer
// @Success 200 {array} Pattern "List of detected patterns"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 403 {object} map[string]string "Premium feature required"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /api/v1/analytics/patterns [get]
func (h *AdvancedHandlers) GetPatterns(w http.ResponseWriter, r *http.Request) {
	userID := h.getUserID(r)
	ctx := r.Context()

	patterns, err := h.insightsEngine.DetectPatterns(ctx, userID)
	if err != nil {
		h.logger.Error("Failed to detect patterns",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "Failed to detect patterns")
		return
	}

	// Group patterns by type
	patternsByType := make(map[string][]Pattern)
	for _, pattern := range patterns {
		patternsByType[pattern.PatternType] = append(patternsByType[pattern.PatternType], pattern)
	}

	h.respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"patterns":        patterns,
		"patterns_by_type": patternsByType,
		"total_count":     len(patterns),
	})
}

// GetRecommendations handles GET /api/v1/analytics/recommendations
// @Summary Get personalized recommendations
// @Description Get personalized recommendations based on behavior analysis (Premium)
// @Tags Analytics
// @Accept json
// @Produce json
// @Security Bearer
// @Success 200 {array} Recommendation "List of recommendations"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 403 {object} map[string]string "Premium feature required"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /api/v1/analytics/recommendations [get]
func (h *AdvancedHandlers) GetRecommendations(w http.ResponseWriter, r *http.Request) {
	userID := h.getUserID(r)
	ctx := r.Context()

	recommendations, err := h.insightsEngine.GenerateRecommendations(ctx, userID)
	if err != nil {
		h.logger.Error("Failed to generate recommendations",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "Failed to generate recommendations")
		return
	}

	// Group by priority
	byPriority := make(map[string][]Recommendation)
	for _, rec := range recommendations {
		byPriority[rec.Priority] = append(byPriority[rec.Priority], rec)
	}

	h.respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"recommendations":  recommendations,
		"by_priority":      byPriority,
		"total_count":      len(recommendations),
	})
}

// GetFocusScore handles GET /api/v1/analytics/focus-score
// @Summary Get focus score
// @Description Calculate comprehensive focus score based on multiple factors (Premium)
// @Tags Analytics
// @Accept json
// @Produce json
// @Security Bearer
// @Success 200 {object} FocusScore "Focus score details"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 403 {object} map[string]string "Premium feature required"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /api/v1/analytics/focus-score [get]
func (h *AdvancedHandlers) GetFocusScore(w http.ResponseWriter, r *http.Request) {
	userID := h.getUserID(r)
	ctx := r.Context()

	focusScore, err := h.insightsEngine.CalculateFocusScore(ctx, userID)
	if err != nil {
		h.logger.Error("Failed to calculate focus score",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "Failed to calculate focus score")
		return
	}

	h.respondWithJSON(w, http.StatusOK, focusScore)
}

// GetProductivityForecast handles GET /api/v1/analytics/forecast
// @Summary Get productivity forecast
// @Description Get predictive analytics for future productivity (Premium)
// @Tags Analytics
// @Accept json
// @Produce json
// @Security Bearer
// @Success 200 {object} ProductivityForecast "Productivity forecast"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 403 {object} map[string]string "Premium feature required"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /api/v1/analytics/forecast [get]
func (h *AdvancedHandlers) GetProductivityForecast(w http.ResponseWriter, r *http.Request) {
	userID := h.getUserID(r)
	ctx := r.Context()

	forecast, err := h.predictiveEngine.ForecastProductivity(ctx, userID)
	if err != nil {
		h.logger.Error("Failed to generate forecast",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "Failed to generate forecast")
		return
	}

	h.respondWithJSON(w, http.StatusOK, forecast)
}

// PredictProductiveDays handles GET /api/v1/analytics/predict-days
// @Summary Predict productive days
// @Description Predict which upcoming days will be most productive (Premium)
// @Tags Analytics
// @Accept json
// @Produce json
// @Security Bearer
// @Param days query int false "Number of days to predict" default(7) minimum(1) maximum(30)
// @Success 200 {array} ProductiveDayPrediction "List of day predictions"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 403 {object} map[string]string "Premium feature required"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /api/v1/analytics/predict-days [get]
func (h *AdvancedHandlers) PredictProductiveDays(w http.ResponseWriter, r *http.Request) {
	userID := h.getUserID(r)
	ctx := r.Context()

	// Parse days parameter
	days := 7
	if daysStr := r.URL.Query().Get("days"); daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil && d > 0 && d <= 30 {
			days = d
		}
	}

	predictions, err := h.predictiveEngine.PredictProductiveDays(ctx, userID, days)
	if err != nil {
		h.logger.Error("Failed to predict productive days",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "Failed to predict productive days")
		return
	}

	// Separate into high, medium, low likelihood
	byLikelihood := make(map[string][]ProductiveDayPrediction)
	for _, pred := range predictions {
		byLikelihood[pred.Likelihood] = append(byLikelihood[pred.Likelihood], pred)
	}

	h.respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"predictions":    predictions,
		"by_likelihood":  byLikelihood,
		"days_predicted": len(predictions),
	})
}

// ReportRequest represents a report generation request
type ReportRequest struct {
	ReportType string `json:"report_type"` // "weekly", "monthly", "custom"
	Format     string `json:"format"`      // "json", "pdf", "csv"
	StartDate  string `json:"start_date,omitempty"`
	EndDate    string `json:"end_date,omitempty"`
	Options    map[string]interface{} `json:"options,omitempty"`
}

// GenerateReport handles POST /api/v1/analytics/reports/generate
// @Summary Generate analytics report
// @Description Generate a comprehensive analytics report (Premium)
// @Tags Analytics
// @Accept json
// @Produce json
// @Security Bearer
// @Param request body ReportRequest true "Report generation request"
// @Success 200 {object} map[string]interface{} "Generated report"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 403 {object} map[string]string "Premium feature required"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /api/v1/analytics/reports/generate [post]
func (h *AdvancedHandlers) GenerateReport(w http.ResponseWriter, r *http.Request) {
	userID := h.getUserID(r)
	ctx := r.Context()

	var req ReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate report type
	if req.ReportType == "" {
		req.ReportType = "weekly"
	}

	// Validate format
	if req.Format == "" {
		req.Format = "json"
	}

	// Generate report based on type
	var report interface{}
	var err error

	switch req.ReportType {
	case "weekly":
		report, err = h.generateWeeklyReport(ctx, userID)
	case "monthly":
		report, err = h.generateMonthlyReport(ctx, userID)
	case "insights":
		report, err = h.generateInsightsReport(ctx, userID)
	case "comprehensive":
		report, err = h.generateComprehensiveReport(ctx, userID)
	default:
		h.respondWithError(w, http.StatusBadRequest, "Invalid report type. Supported: weekly, monthly, insights, comprehensive")
		return
	}

	if err != nil {
		h.logger.Error("Failed to generate report",
			zap.String("user_id", userID.String()),
			zap.String("report_type", req.ReportType),
			zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "Failed to generate report")
		return
	}

	// TODO: Handle non-JSON formats (PDF, CSV) in future
	if req.Format != "json" {
		h.respondWithError(w, http.StatusBadRequest, "Only JSON format is currently supported")
		return
	}

	h.respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"report":      report,
		"report_type": req.ReportType,
		"format":      req.Format,
		"generated_at": h.service.cache.CacheKey("report", userID.String()),
	})
}

// generateWeeklyReport generates a weekly analytics report
func (h *AdvancedHandlers) generateWeeklyReport(ctx context.Context, userID uuid.UUID) (interface{}, error) {
	// Get current week data
	now := time.Now()
	weekStart := now.AddDate(0, 0, -int(now.Weekday()))
	weekStart = time.Date(weekStart.Year(), weekStart.Month(), weekStart.Day(), 0, 0, 0, 0, time.UTC)

	weeklyActivity, err := h.service.GetWeeklyActivity(ctx, userID, weekStart)
	if err != nil {
		return nil, err
	}

	streakMetrics, err := h.service.GetStreakMetrics(ctx, userID)
	if err != nil {
		return nil, err
	}

	categoryPerf, err := h.service.GetCategoryPerformance(ctx, userID, 5)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"period":      "weekly",
		"week_start":  weekStart,
		"activity":    weeklyActivity,
		"streaks":     streakMetrics,
		"top_categories": categoryPerf,
	}, nil
}

// generateMonthlyReport generates a monthly analytics report
func (h *AdvancedHandlers) generateMonthlyReport(ctx context.Context, userID uuid.UUID) (interface{}, error) {
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	monthlyComparison, err := h.service.GetMonthlyComparison(ctx, userID, monthStart)
	if err != nil {
		return nil, err
	}

	streakMetrics, err := h.service.GetStreakMetrics(ctx, userID)
	if err != nil {
		return nil, err
	}

	categoryPerf, err := h.service.GetCategoryPerformance(ctx, userID, 10)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"period":     "monthly",
		"month":      monthStart,
		"comparison": monthlyComparison,
		"streaks":    streakMetrics,
		"categories": categoryPerf,
	}, nil
}

// generateInsightsReport generates an insights-focused report
func (h *AdvancedHandlers) generateInsightsReport(ctx context.Context, userID uuid.UUID) (interface{}, error) {
	insights, err := h.insightsEngine.GenerateInsights(ctx, userID)
	if err != nil {
		return nil, err
	}

	patterns, err := h.insightsEngine.DetectPatterns(ctx, userID)
	if err != nil {
		return nil, err
	}

	recommendations, err := h.insightsEngine.GenerateRecommendations(ctx, userID)
	if err != nil {
		return nil, err
	}

	focusScore, err := h.insightsEngine.CalculateFocusScore(ctx, userID)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"insights":        insights,
		"patterns":        patterns,
		"recommendations": recommendations,
		"focus_score":     focusScore,
	}, nil
}

// generateComprehensiveReport generates a comprehensive analytics report
func (h *AdvancedHandlers) generateComprehensiveReport(ctx context.Context, userID uuid.UUID) (interface{}, error) {
	// Get all major components
	weeklyReport, err := h.generateWeeklyReport(ctx, userID)
	if err != nil {
		return nil, err
	}

	monthlyReport, err := h.generateMonthlyReport(ctx, userID)
	if err != nil {
		return nil, err
	}

	insightsReport, err := h.generateInsightsReport(ctx, userID)
	if err != nil {
		return nil, err
	}

	forecast, err := h.predictiveEngine.ForecastProductivity(ctx, userID)
	if err != nil {
		h.logger.Warn("Failed to generate forecast for comprehensive report", zap.Error(err))
	}

	return map[string]interface{}{
		"weekly":     weeklyReport,
		"monthly":    monthlyReport,
		"insights":   insightsReport,
		"forecast":   forecast,
		"report_type": "comprehensive",
	}, nil
}

// Helper methods

func (h *AdvancedHandlers) getUserID(r *http.Request) uuid.UUID {
	// Extract user ID from context (set by authentication middleware)
	userID, ok := r.Context().Value("user_id").(uuid.UUID)
	if !ok {
		h.logger.Warn("User ID not found in context")
		return uuid.Nil
	}
	return userID
}

func (h *AdvancedHandlers) respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
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

func (h *AdvancedHandlers) respondWithError(w http.ResponseWriter, code int, message string) {
	h.respondWithJSON(w, code, map[string]string{"error": message})
}
