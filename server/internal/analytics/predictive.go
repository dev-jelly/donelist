package analytics

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// PredictiveEngine provides predictive analytics and forecasting
type PredictiveEngine struct {
	service *Service
	logger  *zap.Logger
}

// NewPredictiveEngine creates a new predictive analytics engine
func NewPredictiveEngine(service *Service, logger *zap.Logger) *PredictiveEngine {
	return &PredictiveEngine{
		service: service,
		logger:  logger,
	}
}

// Forecast represents a predictive forecast
type Forecast struct {
	Type        string                 `json:"type"`
	Period      string                 `json:"period"` // "day", "week", "month"
	Prediction  float64                `json:"prediction"`
	Confidence  float64                `json:"confidence"` // 0-1
	Range       ForecastRange          `json:"range"`
	BasedOn     string                 `json:"based_on"`
	Data        map[string]interface{} `json:"data,omitempty"`
	GeneratedAt time.Time              `json:"generated_at"`
}

// ForecastRange represents the prediction range
type ForecastRange struct {
	Low    float64 `json:"low"`
	High   float64 `json:"high"`
	Median float64 `json:"median"`
}

// ProductivityForecast represents a comprehensive productivity forecast
type ProductivityForecast struct {
	NextDay   *Forecast   `json:"next_day"`
	NextWeek  *Forecast   `json:"next_week"`
	NextMonth *Forecast   `json:"next_month"`
	Trends    []Trend     `json:"trends"`
	Insights  []string    `json:"insights"`
	CreatedAt time.Time   `json:"created_at"`
}

// Trend represents a detected trend
type Trend struct {
	Name        string    `json:"name"`
	Direction   string    `json:"direction"` // "up", "down", "stable"
	Strength    float64   `json:"strength"`  // 0-1
	Description string    `json:"description"`
	StartDate   time.Time `json:"start_date"`
}

// ForecastProductivity forecasts user productivity
func (p *PredictiveEngine) ForecastProductivity(ctx context.Context, userID uuid.UUID) (*ProductivityForecast, error) {
	// Try cache first
	var forecast ProductivityForecast
	cacheKey := p.service.cache.CacheKey("forecast", userID.String())
	if found, err := p.service.cache.Get(ctx, cacheKey, &forecast); err == nil && found {
		return &forecast, nil
	}

	// Get historical data (last 90 days)
	historicalData, err := p.getHistoricalActivity(ctx, userID, 90)
	if err != nil {
		return nil, fmt.Errorf("failed to get historical data: %w", err)
	}

	if len(historicalData) < 7 {
		return nil, fmt.Errorf("insufficient data for forecasting (need at least 7 days)")
	}

	// Generate forecasts
	nextDay, err := p.forecastNextDay(ctx, userID, historicalData)
	if err != nil {
		p.logger.Warn("Failed to forecast next day", zap.Error(err))
	}

	nextWeek, err := p.forecastNextWeek(ctx, userID, historicalData)
	if err != nil {
		p.logger.Warn("Failed to forecast next week", zap.Error(err))
	}

	nextMonth, err := p.forecastNextMonth(ctx, userID, historicalData)
	if err != nil {
		p.logger.Warn("Failed to forecast next month", zap.Error(err))
	}

	// Detect trends
	trends := p.detectTrends(historicalData)

	// Generate insights
	insights := p.generateForecastInsights(nextDay, nextWeek, trends)

	forecast = ProductivityForecast{
		NextDay:   nextDay,
		NextWeek:  nextWeek,
		NextMonth: nextMonth,
		Trends:    trends,
		Insights:  insights,
		CreatedAt: time.Now(),
	}

	// Cache the results
	p.service.cache.Set(ctx, cacheKey, forecast, CacheTTLHourly)

	return &forecast, nil
}

// ActivityDataPoint represents a single day's activity
type ActivityDataPoint struct {
	Date         time.Time
	CheckinCount int
	TotalMinutes int
	Categories   int
}

// getHistoricalActivity retrieves historical activity data
func (p *PredictiveEngine) getHistoricalActivity(ctx context.Context, userID uuid.UUID, days int) ([]ActivityDataPoint, error) {
	rows, err := p.service.sqlDB.QueryContext(ctx, `
		SELECT
			activity_date,
			total_checkins,
			total_minutes,
			unique_categories
		FROM mv_daily_user_activity
		WHERE user_id = $1
		  AND activity_date >= CURRENT_DATE - $2
		ORDER BY activity_date ASC
	`, userID, fmt.Sprintf("%d days", days))

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	data := []ActivityDataPoint{}
	for rows.Next() {
		var point ActivityDataPoint
		if err := rows.Scan(&point.Date, &point.CheckinCount, &point.TotalMinutes, &point.Categories); err != nil {
			continue
		}
		data = append(data, point)
	}

	return data, nil
}

// forecastNextDay forecasts activity for the next day
func (p *PredictiveEngine) forecastNextDay(ctx context.Context, userID uuid.UUID, historicalData []ActivityDataPoint) (*Forecast, error) {
	if len(historicalData) < 7 {
		return nil, fmt.Errorf("insufficient data")
	}

	tomorrow := time.Now().AddDate(0, 0, 1)
	tomorrowDayOfWeek := int(tomorrow.Weekday())

	// Get same-day-of-week average from last 4 weeks
	sameDayData := []int{}
	for _, point := range historicalData {
		if int(point.Date.Weekday()) == tomorrowDayOfWeek {
			sameDayData = append(sameDayData, point.CheckinCount)
		}
	}

	if len(sameDayData) == 0 {
		// Fallback to overall average
		sum := 0
		for _, point := range historicalData {
			sum += point.CheckinCount
		}
		avg := float64(sum) / float64(len(historicalData))

		return &Forecast{
			Type:       "daily_checkins",
			Period:     "day",
			Prediction: avg,
			Confidence: 0.5,
			Range: ForecastRange{
				Low:    avg * 0.7,
				High:   avg * 1.3,
				Median: avg,
			},
			BasedOn:     "overall_average",
			GeneratedAt: time.Now(),
		}, nil
	}

	// Calculate weighted average (more recent = higher weight)
	weightedSum := 0.0
	totalWeight := 0.0
	for i, count := range sameDayData {
		weight := float64(i+1) / float64(len(sameDayData))
		weightedSum += float64(count) * weight
		totalWeight += weight
	}
	prediction := weightedSum / totalWeight

	// Calculate standard deviation for confidence
	variance := 0.0
	for _, count := range sameDayData {
		variance += math.Pow(float64(count)-prediction, 2)
	}
	stdDev := math.Sqrt(variance / float64(len(sameDayData)))

	// Confidence based on consistency (lower stddev = higher confidence)
	confidence := math.Max(0.5, 1.0-(stdDev/prediction))

	return &Forecast{
		Type:       "daily_checkins",
		Period:     "day",
		Prediction: prediction,
		Confidence: confidence,
		Range: ForecastRange{
			Low:    math.Max(0, prediction-stdDev),
			High:   prediction + stdDev,
			Median: prediction,
		},
		BasedOn: fmt.Sprintf("%s_pattern", tomorrow.Weekday().String()),
		Data: map[string]interface{}{
			"day_of_week":      tomorrow.Weekday().String(),
			"historical_count": len(sameDayData),
		},
		GeneratedAt: time.Now(),
	}, nil
}

// forecastNextWeek forecasts activity for the next week
func (p *PredictiveEngine) forecastNextWeek(ctx context.Context, userID uuid.UUID, historicalData []ActivityDataPoint) (*Forecast, error) {
	if len(historicalData) < 14 {
		return nil, fmt.Errorf("insufficient data")
	}

	// Calculate weekly averages
	weeklyTotals := []int{}
	currentWeekTotal := 0
	currentWeekStart := historicalData[0].Date

	for _, point := range historicalData {
		// Check if we've moved to a new week
		if point.Date.Sub(currentWeekStart).Hours() >= 168 { // 7 days
			if currentWeekTotal > 0 {
				weeklyTotals = append(weeklyTotals, currentWeekTotal)
			}
			currentWeekTotal = 0
			currentWeekStart = point.Date
		}
		currentWeekTotal += point.CheckinCount
	}

	if currentWeekTotal > 0 {
		weeklyTotals = append(weeklyTotals, currentWeekTotal)
	}

	if len(weeklyTotals) == 0 {
		return nil, fmt.Errorf("no weekly data available")
	}

	// Calculate trend-adjusted prediction
	prediction := 0.0
	if len(weeklyTotals) >= 2 {
		// Simple linear regression for trend
		recentWeeks := weeklyTotals
		if len(weeklyTotals) > 4 {
			recentWeeks = weeklyTotals[len(weeklyTotals)-4:] // Last 4 weeks
		}

		sum := 0.0
		for i, total := range recentWeeks {
			weight := float64(i+1) / float64(len(recentWeeks))
			sum += float64(total) * weight
		}
		prediction = sum / float64(len(recentWeeks))
	} else {
		prediction = float64(weeklyTotals[len(weeklyTotals)-1])
	}

	// Calculate confidence
	variance := 0.0
	for _, total := range weeklyTotals {
		variance += math.Pow(float64(total)-prediction, 2)
	}
	stdDev := math.Sqrt(variance / float64(len(weeklyTotals)))
	confidence := math.Max(0.6, 1.0-(stdDev/prediction))

	return &Forecast{
		Type:       "weekly_checkins",
		Period:     "week",
		Prediction: prediction,
		Confidence: confidence,
		Range: ForecastRange{
			Low:    math.Max(0, prediction-stdDev*1.5),
			High:   prediction + stdDev*1.5,
			Median: prediction,
		},
		BasedOn: "trend_analysis",
		Data: map[string]interface{}{
			"weeks_analyzed": len(weeklyTotals),
		},
		GeneratedAt: time.Now(),
	}, nil
}

// forecastNextMonth forecasts activity for the next month
func (p *PredictiveEngine) forecastNextMonth(ctx context.Context, userID uuid.UUID, historicalData []ActivityDataPoint) (*Forecast, error) {
	if len(historicalData) < 30 {
		return nil, fmt.Errorf("insufficient data")
	}

	// Get last 3 months of data
	rows, err := p.service.sqlDB.QueryContext(ctx, `
		SELECT
			total_checkins
		FROM mv_monthly_summary
		WHERE user_id = $1
		  AND month >= DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '3 months'
		ORDER BY month ASC
	`, userID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	monthlyTotals := []int{}
	for rows.Next() {
		var total int
		if err := rows.Scan(&total); err != nil {
			continue
		}
		monthlyTotals = append(monthlyTotals, total)
	}

	if len(monthlyTotals) == 0 {
		// Fallback to calculating from daily data
		total := 0
		for _, point := range historicalData {
			total += point.CheckinCount
		}
		avgPerDay := float64(total) / float64(len(historicalData))
		prediction := avgPerDay * 30 // Approximate month

		return &Forecast{
			Type:       "monthly_checkins",
			Period:     "month",
			Prediction: prediction,
			Confidence: 0.6,
			Range: ForecastRange{
				Low:    prediction * 0.8,
				High:   prediction * 1.2,
				Median: prediction,
			},
			BasedOn:     "daily_average",
			GeneratedAt: time.Now(),
		}, nil
	}

	// Calculate trend
	prediction := 0.0
	for i, total := range monthlyTotals {
		weight := float64(i+1) / float64(len(monthlyTotals))
		prediction += float64(total) * weight
	}
	prediction = prediction / float64(len(monthlyTotals))

	// Calculate confidence
	variance := 0.0
	for _, total := range monthlyTotals {
		variance += math.Pow(float64(total)-prediction, 2)
	}
	stdDev := math.Sqrt(variance / float64(len(monthlyTotals)))
	confidence := math.Max(0.65, 1.0-(stdDev/prediction))

	return &Forecast{
		Type:       "monthly_checkins",
		Period:     "month",
		Prediction: prediction,
		Confidence: confidence,
		Range: ForecastRange{
			Low:    math.Max(0, prediction-stdDev*2),
			High:   prediction + stdDev*2,
			Median: prediction,
		},
		BasedOn: "monthly_trend",
		Data: map[string]interface{}{
			"months_analyzed": len(monthlyTotals),
		},
		GeneratedAt: time.Now(),
	}, nil
}

// detectTrends detects trends in historical data
func (p *PredictiveEngine) detectTrends(data []ActivityDataPoint) []Trend {
	trends := []Trend{}

	if len(data) < 14 {
		return trends
	}

	// Calculate 7-day moving averages
	recentWeek := data[len(data)-7:]
	previousWeek := data[len(data)-14 : len(data)-7]

	recentAvg := 0.0
	for _, point := range recentWeek {
		recentAvg += float64(point.CheckinCount)
	}
	recentAvg /= float64(len(recentWeek))

	previousAvg := 0.0
	for _, point := range previousWeek {
		previousAvg += float64(point.CheckinCount)
	}
	previousAvg /= float64(len(previousWeek))

	// Detect activity trend
	if previousAvg > 0 {
		change := (recentAvg - previousAvg) / previousAvg
		direction := "stable"
		description := "Your activity level is stable"

		if change > 0.15 {
			direction = "up"
			description = fmt.Sprintf("Your activity is trending up (%.0f%% increase)", change*100)
		} else if change < -0.15 {
			direction = "down"
			description = fmt.Sprintf("Your activity is trending down (%.0f%% decrease)", math.Abs(change)*100)
		}

		trends = append(trends, Trend{
			Name:        "activity_trend",
			Direction:   direction,
			Strength:    math.Min(1.0, math.Abs(change)),
			Description: description,
			StartDate:   recentWeek[0].Date,
		})
	}

	// Detect consistency trend
	varianceRecent := 0.0
	for _, point := range recentWeek {
		varianceRecent += math.Pow(float64(point.CheckinCount)-recentAvg, 2)
	}
	stdDevRecent := math.Sqrt(varianceRecent / float64(len(recentWeek)))

	variancePrevious := 0.0
	for _, point := range previousWeek {
		variancePrevious += math.Pow(float64(point.CheckinCount)-previousAvg, 2)
	}
	stdDevPrevious := math.Sqrt(variancePrevious / float64(len(previousWeek)))

	if stdDevPrevious > 0 {
		consistencyChange := (stdDevRecent - stdDevPrevious) / stdDevPrevious
		direction := "stable"
		description := "Your consistency is stable"

		if consistencyChange < -0.2 {
			direction = "up"
			description = "Your consistency is improving"
		} else if consistencyChange > 0.2 {
			direction = "down"
			description = "Your consistency is declining"
		}

		trends = append(trends, Trend{
			Name:        "consistency_trend",
			Direction:   direction,
			Strength:    math.Min(1.0, math.Abs(consistencyChange)),
			Description: description,
			StartDate:   recentWeek[0].Date,
		})
	}

	return trends
}

// generateForecastInsights generates insights from forecasts
func (p *PredictiveEngine) generateForecastInsights(nextDay, nextWeek *Forecast, trends []Trend) []string {
	insights := []string{}

	// Day forecast insights
	if nextDay != nil {
		if nextDay.Confidence > 0.8 {
			insights = append(insights, fmt.Sprintf("High confidence prediction: expect around %.0f check-ins tomorrow", nextDay.Prediction))
		}

		if nextDay.Prediction < nextDay.Range.Low {
			insights = append(insights, "Tomorrow may be a low-activity day based on your patterns")
		} else if nextDay.Prediction > nextDay.Range.High {
			insights = append(insights, "Tomorrow looks like it will be a highly productive day")
		}
	}

	// Week forecast insights
	if nextWeek != nil {
		if nextWeek.Prediction > 0 {
			avgPerDay := nextWeek.Prediction / 7
			insights = append(insights, fmt.Sprintf("Next week forecast: %.0f check-ins (avg %.1f per day)", nextWeek.Prediction, avgPerDay))
		}
	}

	// Trend insights
	for _, trend := range trends {
		if trend.Name == "activity_trend" && trend.Direction == "up" && trend.Strength > 0.3 {
			insights = append(insights, "Your productivity is on an upward trend - keep it up!")
		} else if trend.Name == "activity_trend" && trend.Direction == "down" && trend.Strength > 0.3 {
			insights = append(insights, "Activity has been declining recently - consider setting new goals")
		}

		if trend.Name == "consistency_trend" && trend.Direction == "up" {
			insights = append(insights, "Your consistency is improving - great progress!")
		}
	}

	return insights
}

// PredictProductiveDays predicts which upcoming days will be most productive
func (p *PredictiveEngine) PredictProductiveDays(ctx context.Context, userID uuid.UUID, days int) ([]ProductiveDayPrediction, error) {
	predictions := []ProductiveDayPrediction{}

	// Get historical data
	historicalData, err := p.getHistoricalActivity(ctx, userID, 90)
	if err != nil {
		return nil, err
	}

	if len(historicalData) < 7 {
		return nil, fmt.Errorf("insufficient historical data")
	}

	// Calculate average activity by day of week
	dayOfWeekAvg := make(map[int]float64)
	dayOfWeekCount := make(map[int]int)

	for _, point := range historicalData {
		dow := int(point.Date.Weekday())
		dayOfWeekAvg[dow] += float64(point.CheckinCount)
		dayOfWeekCount[dow]++
	}

	for dow := range dayOfWeekAvg {
		if dayOfWeekCount[dow] > 0 {
			dayOfWeekAvg[dow] /= float64(dayOfWeekCount[dow])
		}
	}

	// Calculate overall average
	overallSum := 0.0
	for _, avg := range dayOfWeekAvg {
		overallSum += avg
	}
	overallAvg := overallSum / float64(len(dayOfWeekAvg))

	// Generate predictions for next N days
	for i := 1; i <= days; i++ {
		futureDate := time.Now().AddDate(0, 0, i)
		dow := int(futureDate.Weekday())

		expectedActivity := dayOfWeekAvg[dow]
		if expectedActivity == 0 {
			expectedActivity = overallAvg
		}

		// Calculate likelihood based on how much above average
		likelihood := "medium"
		score := expectedActivity / overallAvg

		if score > 1.2 {
			likelihood = "high"
		} else if score < 0.8 {
			likelihood = "low"
		}

		predictions = append(predictions, ProductiveDayPrediction{
			Date:             futureDate,
			DayOfWeek:        futureDate.Weekday().String(),
			ExpectedActivity: expectedActivity,
			Likelihood:       likelihood,
			Score:            score,
			Confidence:       math.Min(0.9, float64(dayOfWeekCount[dow])/10.0),
		})
	}

	return predictions, nil
}

// ProductiveDayPrediction represents a prediction for a specific day
type ProductiveDayPrediction struct {
	Date             time.Time `json:"date"`
	DayOfWeek        string    `json:"day_of_week"`
	ExpectedActivity float64   `json:"expected_activity"`
	Likelihood       string    `json:"likelihood"` // "low", "medium", "high"
	Score            float64   `json:"score"`      // Relative to average
	Confidence       float64   `json:"confidence"` // 0-1
}
