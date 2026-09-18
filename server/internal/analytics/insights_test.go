package analytics

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInsightsEngine_GenerateInsights(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	service, cleanup := setupTestService(t)
	defer cleanup()

	engine := NewInsightsEngine(service, service.logger)
	ctx := context.Background()
	userID := createTestUser(t, service)

	// Create test data
	createTestCheckins(t, service, userID, 30)

	insights, err := engine.GenerateInsights(ctx, userID)
	require.NoError(t, err)
	assert.NotEmpty(t, insights)

	// Verify insight types
	insightTypes := make(map[string]bool)
	for _, insight := range insights {
		insightTypes[insight.Type] = true
		assert.NotEmpty(t, insight.Title)
		assert.NotEmpty(t, insight.Description)
		assert.NotEmpty(t, insight.Severity)
		assert.GreaterOrEqual(t, insight.Score, 0.0)
		assert.LessOrEqual(t, insight.Score, 100.0)
	}

	// Should have at least one insight type
	assert.NotEmpty(t, insightTypes)
}

func TestInsightsEngine_DetectPatterns(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	service, cleanup := setupTestService(t)
	defer cleanup()

	engine := NewInsightsEngine(service, service.logger)
	ctx := context.Background()
	userID := createTestUser(t, service)

	// Create consistent pattern data
	for i := 0; i < 28; i++ { // 4 weeks
		date := time.Now().AddDate(0, 0, -i)
		if date.Weekday() == time.Monday {
			// Create consistent Monday morning pattern
			createTestCheckinAt(t, service, userID, date.Add(9*time.Hour))
			createTestCheckinAt(t, service, userID, date.Add(10*time.Hour))
		}
	}

	patterns, err := engine.DetectPatterns(ctx, userID)
	require.NoError(t, err)
	assert.NotEmpty(t, patterns)

	// Verify pattern structure
	for _, pattern := range patterns {
		assert.NotEmpty(t, pattern.Name)
		assert.NotEmpty(t, pattern.PatternType)
		assert.GreaterOrEqual(t, pattern.Confidence, 0.0)
		assert.LessOrEqual(t, pattern.Confidence, 1.0)
		assert.NotEmpty(t, pattern.Frequency)
	}
}

func TestInsightsEngine_GenerateRecommendations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	service, cleanup := setupTestService(t)
	defer cleanup()

	engine := NewInsightsEngine(service, service.logger)
	ctx := context.Background()
	userID := createTestUser(t, service)

	// Create test data with various patterns
	createTestCheckins(t, service, userID, 20)

	recommendations, err := engine.GenerateRecommendations(ctx, userID)
	require.NoError(t, err)
	assert.NotEmpty(t, recommendations)

	// Verify recommendation structure
	for _, rec := range recommendations {
		assert.NotEmpty(t, rec.Type)
		assert.NotEmpty(t, rec.Title)
		assert.NotEmpty(t, rec.Description)
		assert.Contains(t, []string{"high", "medium", "low"}, rec.Priority)
		assert.Contains(t, []string{"high", "medium", "low"}, rec.Effort)
		assert.NotEmpty(t, rec.Impact)
	}
}

func TestInsightsEngine_CalculateFocusScore(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	service, cleanup := setupTestService(t)
	defer cleanup()

	engine := NewInsightsEngine(service, service.logger)
	ctx := context.Background()

	tests := []struct {
		name           string
		checkinsCount  int
		expectedLevel  string
		minScore       float64
		maxScore       float64
	}{
		{
			name:          "Low activity",
			checkinsCount: 5,
			expectedLevel: "low",
			minScore:      0,
			maxScore:      40,
		},
		{
			name:          "Medium activity",
			checkinsCount: 15,
			expectedLevel: "medium",
			minScore:      30,
			maxScore:      70,
		},
		{
			name:          "High activity",
			checkinsCount: 40,
			expectedLevel: "high",
			minScore:      50,
			maxScore:      100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID := createTestUser(t, service)
			createTestCheckins(t, service, userID, tt.checkinsCount)

			score, err := engine.CalculateFocusScore(ctx, userID)
			require.NoError(t, err)
			assert.NotNil(t, score)
			assert.GreaterOrEqual(t, score.Score, tt.minScore)
			assert.LessOrEqual(t, score.Score, tt.maxScore)
			assert.NotEmpty(t, score.Level)
			assert.NotEmpty(t, score.Factors)
		})
	}
}

func TestInsightsEngine_StreakInsights(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	service, cleanup := setupTestService(t)
	defer cleanup()

	engine := NewInsightsEngine(service, service.logger)
	ctx := context.Background()
	userID := createTestUser(t, service)

	// Create a streak
	for i := 0; i < 7; i++ {
		date := time.Now().AddDate(0, 0, -i)
		createTestCheckinAt(t, service, userID, date)
	}

	insights, err := engine.generateStreakInsights(ctx, userID)
	require.NoError(t, err)
	assert.NotEmpty(t, insights)

	// Should have streak-related insights
	hasStreakInsight := false
	for _, insight := range insights {
		if insight.Type == "streak_active" || insight.Type == "consistency_score" {
			hasStreakInsight = true
			break
		}
	}
	assert.True(t, hasStreakInsight, "Should generate streak insights")
}

func TestInsightsEngine_ProductivityInsights(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	service, cleanup := setupTestService(t)
	defer cleanup()

	engine := NewInsightsEngine(service, service.logger)
	ctx := context.Background()
	userID := createTestUser(t, service)

	// Create varied activity over 2 weeks
	for i := 0; i < 14; i++ {
		date := time.Now().AddDate(0, 0, -i)
		count := 3
		if i < 7 {
			count = 5 // More active in recent week
		}
		for j := 0; j < count; j++ {
			createTestCheckinAt(t, service, userID, date.Add(time.Duration(j)*time.Hour))
		}
	}

	insights, err := engine.generateProductivityInsights(ctx, userID)
	require.NoError(t, err)
	assert.NotEmpty(t, insights)

	// Should detect weekly trend
	hasTrendInsight := false
	for _, insight := range insights {
		if insight.Type == "weekly_trend" {
			hasTrendInsight = true
			assert.Contains(t, []string{"info", "success", "warning"}, insight.Severity)
			break
		}
	}
	assert.True(t, hasTrendInsight, "Should generate productivity trend insight")
}

func TestInsightsEngine_CategoryInsights(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	service, cleanup := setupTestService(t)
	defer cleanup()

	engine := NewInsightsEngine(service, service.logger)
	ctx := context.Background()
	userID := createTestUser(t, service)
	categoryID := createTestCategory(t, service, userID, "Work")

	// Create frequent usage of a category
	for i := 0; i < 20; i++ {
		createTestCheckinWithCategory(t, service, userID, categoryID)
	}

	insights, err := engine.generateCategoryInsights(ctx, userID)
	require.NoError(t, err)
	assert.NotEmpty(t, insights)

	// Should have top category insight
	hasTopCategory := false
	for _, insight := range insights {
		if insight.Type == "top_category" {
			hasTopCategory = true
			assert.Equal(t, "success", insight.Severity)
			break
		}
	}
	assert.True(t, hasTopCategory, "Should identify top category")
}

func TestInsightsEngine_TimePatternInsights(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	service, cleanup := setupTestService(t)
	defer cleanup()

	engine := NewInsightsEngine(service, service.logger)
	ctx := context.Background()
	userID := createTestUser(t, service)

	// Create early morning pattern
	for i := 0; i < 20; i++ {
		date := time.Now().AddDate(0, 0, -i)
		earlyMorning := time.Date(date.Year(), date.Month(), date.Day(), 7, 0, 0, 0, time.UTC)
		createTestCheckinAt(t, service, userID, earlyMorning)
	}

	insights, err := engine.generateTimePatternInsights(ctx, userID)
	require.NoError(t, err)

	// May or may not have chronotype insight depending on data density
	// Just verify structure if present
	for _, insight := range insights {
		if insight.Type == "chronotype" {
			assert.NotEmpty(t, insight.Title)
			assert.NotEmpty(t, insight.Description)
		}
	}
}

func TestInsightsEngine_Caching(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	service, cleanup := setupTestService(t)
	defer cleanup()

	engine := NewInsightsEngine(service, service.logger)
	ctx := context.Background()
	userID := createTestUser(t, service)
	createTestCheckins(t, service, userID, 10)

	// First call - should compute
	insights1, err := engine.GenerateInsights(ctx, userID)
	require.NoError(t, err)

	// Second call - should use cache
	insights2, err := engine.GenerateInsights(ctx, userID)
	require.NoError(t, err)

	// Should return same data
	assert.Equal(t, len(insights1), len(insights2))
}

// Helper functions

func setupTestService(t *testing.T) (*Service, func()) {
	return setupTestAnalyticsService(t)
}

func createTestUser(t *testing.T, service *Service) uuid.UUID {
	return uuid.New()
}

func createTestCheckins(t *testing.T, service *Service, userID uuid.UUID, count int) {
	for i := 0; i < count; i++ {
		date := time.Now().AddDate(0, 0, -i)
		createTestCheckinAt(t, service, userID, date)
	}
}

func createTestCheckinAt(t *testing.T, service *Service, userID uuid.UUID, checkinTime time.Time) {
	_, err := service.db.Exec(`
		INSERT INTO checkins (id, user_id, checkin_time, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
	`, uuid.New(), userID, checkinTime, time.Now(), time.Now())
	require.NoError(t, err)
}

func createTestCheckinWithCategory(t *testing.T, service *Service, userID, categoryID uuid.UUID) {
	_, err := service.db.Exec(`
		INSERT INTO checkins (id, user_id, category_id, checkin_time, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, uuid.New(), userID, categoryID, time.Now(), time.Now(), time.Now())
	require.NoError(t, err)
}

func createTestCategory(t *testing.T, service *Service, userID uuid.UUID, name string) uuid.UUID {
	categoryID := uuid.New()
	_, err := service.db.Exec(`
		INSERT INTO categories (id, user_id, name, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
	`, categoryID, userID, name, time.Now(), time.Now())
	require.NoError(t, err)
	return categoryID
}
