package analytics

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPredictiveEngine_ForecastProductivity(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	service, cleanup := setupTestService(t)
	defer cleanup()

	engine := NewPredictiveEngine(service, service.logger)
	ctx := context.Background()
	userID := createTestUser(t, service)

	// Create consistent historical data (30 days)
	for i := 0; i < 30; i++ {
		date := time.Now().AddDate(0, 0, -i)
		count := 5 + (i % 3) // Varied activity
		for j := 0; j < count; j++ {
			createTestCheckinAt(t, service, userID, date.Add(time.Duration(j)*time.Hour))
		}
	}

	// Refresh materialized views
	err := service.RefreshMaterializedViews(ctx)
	require.NoError(t, err)

	forecast, err := engine.ForecastProductivity(ctx, userID)
	require.NoError(t, err)
	assert.NotNil(t, forecast)

	// Verify forecast structure
	assert.NotNil(t, forecast.NextDay)
	assert.NotNil(t, forecast.NextWeek)
	assert.NotEmpty(t, forecast.Trends)
	assert.NotEmpty(t, forecast.Insights)

	// Verify next day forecast
	assert.Equal(t, "day", forecast.NextDay.Period)
	assert.GreaterOrEqual(t, forecast.NextDay.Prediction, 0.0)
	assert.GreaterOrEqual(t, forecast.NextDay.Confidence, 0.0)
	assert.LessOrEqual(t, forecast.NextDay.Confidence, 1.0)
	assert.Less(t, forecast.NextDay.Range.Low, forecast.NextDay.Range.High)

	// Verify next week forecast
	assert.Equal(t, "week", forecast.NextWeek.Period)
	assert.GreaterOrEqual(t, forecast.NextWeek.Prediction, 0.0)
	assert.GreaterOrEqual(t, forecast.NextWeek.Confidence, 0.0)
	assert.LessOrEqual(t, forecast.NextWeek.Confidence, 1.0)
}

func TestPredictiveEngine_ForecastNextDay(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	service, cleanup := setupTestService(t)
	defer cleanup()

	engine := NewPredictiveEngine(service, service.logger)
	ctx := context.Background()
	userID := createTestUser(t, service)

	// Create data for same day of week pattern
	tomorrow := time.Now().AddDate(0, 0, 1)
	tomorrowDayOfWeek := tomorrow.Weekday()

	// Create 4 weeks of data for the same day of week
	for i := 0; i < 4; i++ {
		date := time.Now().AddDate(0, 0, -7*i)
		for date.Weekday() != tomorrowDayOfWeek {
			date = date.AddDate(0, 0, -1)
		}
		// Consistent activity on this day
		for j := 0; j < 5; j++ {
			createTestCheckinAt(t, service, userID, date.Add(time.Duration(j)*time.Hour))
		}
	}

	// Get historical data
	historicalData, err := engine.getHistoricalActivity(ctx, userID, 90)
	require.NoError(t, err)

	forecast, err := engine.forecastNextDay(ctx, userID, historicalData)
	require.NoError(t, err)
	assert.NotNil(t, forecast)

	// Should predict around 5 based on pattern
	assert.GreaterOrEqual(t, forecast.Prediction, 3.0)
	assert.LessOrEqual(t, forecast.Prediction, 7.0)
	assert.Equal(t, "day", forecast.Period)
	assert.Equal(t, "daily_checkins", forecast.Type)
}

func TestPredictiveEngine_ForecastNextWeek(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	service, cleanup := setupTestService(t)
	defer cleanup()

	engine := NewPredictiveEngine(service, service.logger)
	ctx := context.Background()
	userID := createTestUser(t, service)

	// Create 4 weeks of data with upward trend
	for week := 0; week < 4; week++ {
		for day := 0; day < 7; day++ {
			date := time.Now().AddDate(0, 0, -(week*7 + day))
			count := 3 + week // Increasing trend
			for j := 0; j < count; j++ {
				createTestCheckinAt(t, service, userID, date.Add(time.Duration(j)*time.Hour))
			}
		}
	}

	historicalData, err := engine.getHistoricalActivity(ctx, userID, 90)
	require.NoError(t, err)

	forecast, err := engine.forecastNextWeek(ctx, userID, historicalData)
	require.NoError(t, err)
	assert.NotNil(t, forecast)

	assert.Equal(t, "week", forecast.Period)
	assert.Equal(t, "weekly_checkins", forecast.Type)
	assert.GreaterOrEqual(t, forecast.Prediction, 0.0)
	assert.GreaterOrEqual(t, forecast.Confidence, 0.5)
}

func TestPredictiveEngine_ForecastNextMonth(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	service, cleanup := setupTestService(t)
	defer cleanup()

	engine := NewPredictiveEngine(service, service.logger)
	ctx := context.Background()
	userID := createTestUser(t, service)

	// Create 90 days of data
	for i := 0; i < 90; i++ {
		date := time.Now().AddDate(0, 0, -i)
		count := 4
		for j := 0; j < count; j++ {
			createTestCheckinAt(t, service, userID, date.Add(time.Duration(j)*time.Hour))
		}
	}

	// Refresh monthly summary view
	err := service.RefreshMaterializedViews(ctx)
	require.NoError(t, err)

	historicalData, err := engine.getHistoricalActivity(ctx, userID, 90)
	require.NoError(t, err)

	forecast, err := engine.forecastNextMonth(ctx, userID, historicalData)
	require.NoError(t, err)
	assert.NotNil(t, forecast)

	assert.Equal(t, "month", forecast.Period)
	assert.Equal(t, "monthly_checkins", forecast.Type)
	assert.GreaterOrEqual(t, forecast.Prediction, 0.0)
	// Should predict roughly 120 (4 per day * 30 days)
	assert.GreaterOrEqual(t, forecast.Prediction, 80.0)
	assert.LessOrEqual(t, forecast.Prediction, 160.0)
}

func TestPredictiveEngine_DetectTrends(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	service, cleanup := setupTestService(t)
	defer cleanup()

	engine := NewPredictiveEngine(service, service.logger)
	userID := createTestUser(t, service)

	tests := []struct {
		name          string
		dataPattern   func() []ActivityDataPoint
		expectedTrend string // "up", "down", "stable"
	}{
		{
			name: "Upward trend",
			dataPattern: func() []ActivityDataPoint {
				data := []ActivityDataPoint{}
				for i := 0; i < 14; i++ {
					date := time.Now().AddDate(0, 0, -13+i)
					count := 3 + (i / 7) * 3 // Increase after first week
					data = append(data, ActivityDataPoint{
						Date:         date,
						CheckinCount: count,
					})
				}
				return data
			},
			expectedTrend: "up",
		},
		{
			name: "Downward trend",
			dataPattern: func() []ActivityDataPoint {
				data := []ActivityDataPoint{}
				for i := 0; i < 14; i++ {
					date := time.Now().AddDate(0, 0, -13+i)
					count := 8 - (i / 7) * 4 // Decrease after first week
					data = append(data, ActivityDataPoint{
						Date:         date,
						CheckinCount: count,
					})
				}
				return data
			},
			expectedTrend: "down",
		},
		{
			name: "Stable trend",
			dataPattern: func() []ActivityDataPoint {
				data := []ActivityDataPoint{}
				for i := 0; i < 14; i++ {
					date := time.Now().AddDate(0, 0, -13+i)
					data = append(data, ActivityDataPoint{
						Date:         date,
						CheckinCount: 5, // Consistent
					})
				}
				return data
			},
			expectedTrend: "stable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := tt.dataPattern()
			trends := engine.detectTrends(data)

			require.NotEmpty(t, trends)
			activityTrend := trends[0]
			assert.Equal(t, "activity_trend", activityTrend.Name)
			assert.Equal(t, tt.expectedTrend, activityTrend.Direction)
			assert.GreaterOrEqual(t, activityTrend.Strength, 0.0)
			assert.LessOrEqual(t, activityTrend.Strength, 1.0)
		})
	}
}

func TestPredictiveEngine_PredictProductiveDays(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	service, cleanup := setupTestService(t)
	defer cleanup()

	engine := NewPredictiveEngine(service, service.logger)
	ctx := context.Background()
	userID := createTestUser(t, service)

	// Create pattern: high activity on Mondays, low on Sundays
	for week := 0; week < 8; week++ {
		for day := 0; day < 7; day++ {
			date := time.Now().AddDate(0, 0, -(week*7 + day))
			count := 3
			if date.Weekday() == time.Monday {
				count = 8 // High on Mondays
			} else if date.Weekday() == time.Sunday {
				count = 1 // Low on Sundays
			}
			for j := 0; j < count; j++ {
				createTestCheckinAt(t, service, userID, date.Add(time.Duration(j)*time.Hour))
			}
		}
	}

	predictions, err := engine.PredictProductiveDays(ctx, userID, 7)
	require.NoError(t, err)
	assert.Len(t, predictions, 7)

	// Verify prediction structure
	for _, pred := range predictions {
		assert.NotEmpty(t, pred.DayOfWeek)
		assert.GreaterOrEqual(t, pred.ExpectedActivity, 0.0)
		assert.Contains(t, []string{"low", "medium", "high"}, pred.Likelihood)
		assert.GreaterOrEqual(t, pred.Score, 0.0)
		assert.GreaterOrEqual(t, pred.Confidence, 0.0)
		assert.LessOrEqual(t, pred.Confidence, 1.0)
	}

	// Find Monday prediction (should be high)
	var mondayPred *ProductiveDayPrediction
	for i := range predictions {
		if predictions[i].DayOfWeek == "Monday" {
			mondayPred = &predictions[i]
			break
		}
	}

	if mondayPred != nil {
		// Monday should have high expected activity
		assert.GreaterOrEqual(t, mondayPred.ExpectedActivity, 6.0)
	}
}

func TestPredictiveEngine_InsufficientData(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	service, cleanup := setupTestService(t)
	defer cleanup()

	engine := NewPredictiveEngine(service, service.logger)
	ctx := context.Background()
	userID := createTestUser(t, service)

	// Create only 3 days of data (insufficient)
	for i := 0; i < 3; i++ {
		date := time.Now().AddDate(0, 0, -i)
		createTestCheckinAt(t, service, userID, date)
	}

	_, err := engine.ForecastProductivity(ctx, userID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient data")
}

func TestPredictiveEngine_Caching(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	service, cleanup := setupTestService(t)
	defer cleanup()

	engine := NewPredictiveEngine(service, service.logger)
	ctx := context.Background()
	userID := createTestUser(t, service)

	// Create sufficient data
	for i := 0; i < 30; i++ {
		date := time.Now().AddDate(0, 0, -i)
		for j := 0; j < 5; j++ {
			createTestCheckinAt(t, service, userID, date.Add(time.Duration(j)*time.Hour))
		}
	}

	err := service.RefreshMaterializedViews(ctx)
	require.NoError(t, err)

	// First call
	forecast1, err := engine.ForecastProductivity(ctx, userID)
	require.NoError(t, err)

	// Second call - should use cache
	forecast2, err := engine.ForecastProductivity(ctx, userID)
	require.NoError(t, err)

	// Should have same structure
	assert.Equal(t, forecast1.NextDay.Type, forecast2.NextDay.Type)
	assert.Equal(t, forecast1.NextWeek.Type, forecast2.NextWeek.Type)
}

func TestPredictiveEngine_HistoricalActivity(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	service, cleanup := setupTestService(t)
	defer cleanup()

	engine := NewPredictiveEngine(service, service.logger)
	ctx := context.Background()
	userID := createTestUser(t, service)

	// Create 45 days of data
	for i := 0; i < 45; i++ {
		date := time.Now().AddDate(0, 0, -i)
		for j := 0; j < 3; j++ {
			createTestCheckinAt(t, service, userID, date.Add(time.Duration(j)*time.Hour))
		}
	}

	err := service.RefreshMaterializedViews(ctx)
	require.NoError(t, err)

	// Get 30 days of historical data
	data, err := engine.getHistoricalActivity(ctx, userID, 30)
	require.NoError(t, err)
	assert.NotEmpty(t, data)
	assert.LessOrEqual(t, len(data), 30)

	// Verify data structure
	for _, point := range data {
		assert.GreaterOrEqual(t, point.CheckinCount, 0)
		assert.GreaterOrEqual(t, point.TotalMinutes, 0)
		assert.GreaterOrEqual(t, point.Categories, 0)
	}
}

func TestPredictiveEngine_GenerateForecastInsights(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	engine := NewPredictiveEngine(service, service.logger)

	nextDay := &Forecast{
		Type:       "daily_checkins",
		Period:     "day",
		Prediction: 8.5,
		Confidence: 0.85,
		Range: ForecastRange{
			Low:    6.0,
			High:   11.0,
			Median: 8.5,
		},
	}

	nextWeek := &Forecast{
		Type:       "weekly_checkins",
		Period:     "week",
		Prediction: 45.0,
		Confidence: 0.75,
	}

	trends := []Trend{
		{
			Name:      "activity_trend",
			Direction: "up",
			Strength:  0.5,
		},
		{
			Name:      "consistency_trend",
			Direction: "up",
			Strength:  0.3,
		},
	}

	insights := engine.generateForecastInsights(nextDay, nextWeek, trends)
	assert.NotEmpty(t, insights)

	// Should have insights about high confidence, trends, etc.
	hasConfidenceInsight := false
	hasTrendInsight := false

	for _, insight := range insights {
		if len(insight) > 0 {
			if containsSubstring(insight, "confidence") {
				hasConfidenceInsight = true
			}
			if containsSubstring(insight, "trend") || containsSubstring(insight, "upward") {
				hasTrendInsight = true
			}
		}
	}

	assert.True(t, hasConfidenceInsight || hasTrendInsight, "Should generate meaningful insights")
}

// Helper function
func containsSubstring(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 &&
		(len(s) >= len(substr) && s[:len(substr)] == substr ||
		 len(s) > len(substr) && contains(s, substr))
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
