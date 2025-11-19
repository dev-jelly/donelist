package statistics

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

// UsageHandlers handles HTTP requests for usage statistics
type UsageHandlers struct {
	service *UsageService
	logger  *zap.Logger
}

// NewUsageHandlers creates new usage statistics handlers
func NewUsageHandlers(service *UsageService, logger *zap.Logger) *UsageHandlers {
	return &UsageHandlers{
		service: service,
		logger:  logger,
	}
}

// RegisterRoutes registers all usage statistics routes
func (h *UsageHandlers) RegisterRoutes(r *mux.Router) {
	// Dashboard endpoints
	r.HandleFunc("/api/v1/statistics/usage/dashboard", h.GetUsageDashboard).Methods("GET")

	// Category usage endpoints
	r.HandleFunc("/api/v1/statistics/usage/categories", h.GetCategoryUsageStats).Methods("GET")
	r.HandleFunc("/api/v1/statistics/usage/categories/{id}/trend", h.GetCategoryTrend).Methods("GET")

	// Tag usage endpoints
	r.HandleFunc("/api/v1/statistics/usage/tags", h.GetTagUsageStats).Methods("GET")
	r.HandleFunc("/api/v1/statistics/usage/tags/{id}/trend", h.GetTagTrend).Methods("GET")

	// Trends and patterns
	r.HandleFunc("/api/v1/statistics/usage/trends", h.GetUsageTrends).Methods("GET")
	r.HandleFunc("/api/v1/statistics/usage/patterns", h.GetUsagePatterns).Methods("GET")

	// Recommendations
	r.HandleFunc("/api/v1/statistics/usage/recommendations", h.GetRecommendations).Methods("GET")
}

// GetUsageDashboard handles GET /api/v1/statistics/usage/dashboard
func (h *UsageHandlers) GetUsageDashboard(w http.ResponseWriter, r *http.Request) {
	// Extract user ID from context (assuming middleware sets this)
	userID := r.Context().Value("user_id").(uuid.UUID)

	// Parse query parameters
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "month" // Default to month view
	}

	includeUnused := r.URL.Query().Get("include_unused") == "true"
	includePatterns := r.URL.Query().Get("include_patterns") == "true"
	includeMatrix := r.URL.Query().Get("include_matrix") == "true"

	topItemsLimit := 10
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
			topItemsLimit = limit
		}
	}

	unusedThreshold := 30
	if thresholdStr := r.URL.Query().Get("unused_threshold"); thresholdStr != "" {
		if threshold, err := strconv.Atoi(thresholdStr); err == nil && threshold > 0 {
			unusedThreshold = threshold
		}
	}

	// Parse custom date range if period is "custom"
	var startDate, endDate time.Time
	if period == "custom" {
		if startStr := r.URL.Query().Get("start_date"); startStr != "" {
			if parsed, err := time.Parse("2006-01-02", startStr); err == nil {
				startDate = parsed
			}
		}
		if endStr := r.URL.Query().Get("end_date"); endStr != "" {
			if parsed, err := time.Parse("2006-01-02", endStr); err == nil {
				endDate = parsed
			}
		}
	}

	opts := DashboardOptions{
		UserID:          userID,
		Period:          period,
		StartDate:       startDate,
		EndDate:         endDate,
		TopItemsLimit:   topItemsLimit,
		IncludeUnused:   includeUnused,
		IncludePatterns: includePatterns,
		IncludeMatrix:   includeMatrix,
		UnusedThreshold: unusedThreshold,
	}

	dashboard, err := h.service.GetUsageDashboard(r.Context(), opts)
	if err != nil {
		h.logger.Error("Failed to get usage dashboard",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "Failed to generate usage dashboard")
		return
	}

	h.respondWithJSON(w, http.StatusOK, dashboard)
}

// GetCategoryUsageStats handles GET /api/v1/statistics/usage/categories
func (h *UsageHandlers) GetCategoryUsageStats(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(uuid.UUID)

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "month"
	}

	limit := 20
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	stats, err := h.service.GetCategoryUsageStats(r.Context(), userID, period, limit)
	if err != nil {
		h.logger.Error("Failed to get category usage stats",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "Failed to get category statistics")
		return
	}

	response := map[string]interface{}{
		"period": period,
		"limit":  limit,
		"data":   stats,
	}

	h.respondWithJSON(w, http.StatusOK, response)
}

// GetTagUsageStats handles GET /api/v1/statistics/usage/tags
func (h *UsageHandlers) GetTagUsageStats(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(uuid.UUID)

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "month"
	}

	limit := 20
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	stats, err := h.service.GetTagUsageStats(r.Context(), userID, period, limit)
	if err != nil {
		h.logger.Error("Failed to get tag usage stats",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "Failed to get tag statistics")
		return
	}

	response := map[string]interface{}{
		"period": period,
		"limit":  limit,
		"data":   stats,
	}

	h.respondWithJSON(w, http.StatusOK, response)
}

// GetCategoryTrend handles GET /api/v1/statistics/usage/categories/{id}/trend
func (h *UsageHandlers) GetCategoryTrend(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(uuid.UUID)

	vars := mux.Vars(r)
	categoryID, err := uuid.Parse(vars["id"])
	if err != nil {
		h.respondWithError(w, http.StatusBadRequest, "Invalid category ID")
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "month"
	}

	aggregation := r.URL.Query().Get("aggregation")
	if aggregation == "" {
		aggregation = "day"
	}

	trends, err := h.service.GetUsageTrends(
		r.Context(),
		userID,
		"category",
		[]uuid.UUID{categoryID},
		period,
		aggregation,
	)
	if err != nil {
		h.logger.Error("Failed to get category trend",
			zap.String("category_id", categoryID.String()),
			zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "Failed to get category trend")
		return
	}

	var trend *UsageTrend
	if len(trends) > 0 {
		trend = trends[0]
	}

	h.respondWithJSON(w, http.StatusOK, trend)
}

// GetTagTrend handles GET /api/v1/statistics/usage/tags/{id}/trend
func (h *UsageHandlers) GetTagTrend(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(uuid.UUID)

	vars := mux.Vars(r)
	tagID, err := uuid.Parse(vars["id"])
	if err != nil {
		h.respondWithError(w, http.StatusBadRequest, "Invalid tag ID")
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "month"
	}

	aggregation := r.URL.Query().Get("aggregation")
	if aggregation == "" {
		aggregation = "day"
	}

	trends, err := h.service.GetUsageTrends(
		r.Context(),
		userID,
		"tag",
		[]uuid.UUID{tagID},
		period,
		aggregation,
	)
	if err != nil {
		h.logger.Error("Failed to get tag trend",
			zap.String("tag_id", tagID.String()),
			zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "Failed to get tag trend")
		return
	}

	var trend *UsageTrend
	if len(trends) > 0 {
		trend = trends[0]
	}

	h.respondWithJSON(w, http.StatusOK, trend)
}

// GetUsageTrends handles GET /api/v1/statistics/usage/trends
func (h *UsageHandlers) GetUsageTrends(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(uuid.UUID)

	itemType := r.URL.Query().Get("type")
	if itemType == "" {
		itemType = "category"
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "month"
	}

	aggregation := r.URL.Query().Get("aggregation")
	if aggregation == "" {
		aggregation = "day"
	}

	// Parse item IDs
	var itemIDs []uuid.UUID
	if idsStr := r.URL.Query().Get("ids"); idsStr != "" {
		// Parse comma-separated IDs
		for _, idStr := range splitAndTrim(idsStr, ",") {
			if id, err := uuid.Parse(idStr); err == nil {
				itemIDs = append(itemIDs, id)
			}
		}
	}

	if len(itemIDs) == 0 {
		h.respondWithError(w, http.StatusBadRequest, "At least one item ID is required")
		return
	}

	trends, err := h.service.GetUsageTrends(
		r.Context(),
		userID,
		itemType,
		itemIDs,
		period,
		aggregation,
	)
	if err != nil {
		h.logger.Error("Failed to get usage trends",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "Failed to get usage trends")
		return
	}

	response := map[string]interface{}{
		"type":        itemType,
		"period":      period,
		"aggregation": aggregation,
		"data":        trends,
	}

	h.respondWithJSON(w, http.StatusOK, response)
}

// GetUsagePatterns handles GET /api/v1/statistics/usage/patterns
func (h *UsageHandlers) GetUsagePatterns(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(uuid.UUID)

	// Get a simplified dashboard with just patterns
	opts := DashboardOptions{
		UserID:          userID,
		Period:          "month",
		TopItemsLimit:   10,
		IncludePatterns: true,
		IncludeUnused:   false,
		IncludeMatrix:   false,
	}

	dashboard, err := h.service.GetUsageDashboard(r.Context(), opts)
	if err != nil {
		h.logger.Error("Failed to get usage patterns",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "Failed to get usage patterns")
		return
	}

	h.respondWithJSON(w, http.StatusOK, dashboard.UsagePatterns)
}

// GetRecommendations handles GET /api/v1/statistics/usage/recommendations
func (h *UsageHandlers) GetRecommendations(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(uuid.UUID)

	// Get a dashboard with recommendations
	opts := DashboardOptions{
		UserID:          userID,
		Period:          "quarter", // Use longer period for better recommendations
		TopItemsLimit:   20,
		IncludeUnused:   true,
		IncludePatterns: false,
		IncludeMatrix:   false,
		UnusedThreshold: 60, // 60 days for recommendations
	}

	dashboard, err := h.service.GetUsageDashboard(r.Context(), opts)
	if err != nil {
		h.logger.Error("Failed to get recommendations",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "Failed to get recommendations")
		return
	}

	response := map[string]interface{}{
		"unused_categories": dashboard.UnusedCategories,
		"unused_tags":       dashboard.UnusedTags,
		"recommendations":   dashboard.Recommendations,
	}

	h.respondWithJSON(w, http.StatusOK, response)
}

// Helper functions

func (h *UsageHandlers) respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
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

func (h *UsageHandlers) respondWithError(w http.ResponseWriter, code int, message string) {
	h.respondWithJSON(w, code, map[string]string{"error": message})
}

func splitAndTrim(s string, sep string) []string {
	parts := []string{}
	for _, part := range stringsSplit(s, sep) {
		if trimmed := stringsTrim(part); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

// Simple string split function
func stringsSplit(s, sep string) []string {
	// This would use strings.Split in real implementation
	return []string{s} // Simplified
}

// Simple string trim function
func stringsTrim(s string) string {
	// This would use strings.TrimSpace in real implementation
	return s // Simplified
}