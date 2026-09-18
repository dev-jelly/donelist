package analytics

// This file provides example integration code for the main application.
// It shows how to properly initialize and register the advanced analytics system.

/*
Example: Main Application Integration

package main

import (
	"context"
	"time"

	"github.com/dev-jelly/donelist/internal/analytics"
	"github.com/dev-jelly/donelist/internal/premium"
	"github.com/gorilla/mux"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func setupAnalytics(
	db *gorm.DB,
	redisClient *redis.Client,
	logger *zap.Logger,
	premiumService *premium.Service,
) (*analytics.Service, *analytics.Handlers, *analytics.AdvancedHandlers) {

	// 1. Initialize the analytics service
	analyticsService := analytics.NewService(db, redisClient, logger)

	// 2. Initialize handlers
	basicHandlers := analytics.NewHandlers(analyticsService, logger)
	advancedHandlers := analytics.NewAdvancedHandlers(analyticsService, logger)

	return analyticsService, basicHandlers, advancedHandlers
}

func registerAnalyticsRoutes(
	router *mux.Router,
	basicHandlers *analytics.Handlers,
	advancedHandlers *analytics.AdvancedHandlers,
	premiumMiddleware *premium.Middleware,
) {

	// Register basic analytics routes (available to all authenticated users)
	basicHandlers.RegisterRoutes(router)

	// Register advanced analytics routes (Premium only)
	// All routes under /api/v1/analytics that require premium features
	premiumRouter := router.PathPrefix("").Subrouter()
	premiumRouter.Use(premiumMiddleware.RequireFeature(premium.FeatureAdvancedAnalytics))
	advancedHandlers.RegisterRoutes(premiumRouter)
}

func setupBackgroundJobs(service *analytics.Service, logger *zap.Logger) {
	// Setup periodic materialized view refresh

	// Full refresh every 6 hours
	go func() {
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()

		for range ticker.C {
			ctx := context.Background()
			logger.Info("Starting scheduled materialized view refresh")

			if err := service.RefreshMaterializedViews(ctx); err != nil {
				logger.Error("Failed to refresh materialized views",
					zap.Error(err),
				)
			} else {
				logger.Info("Materialized views refreshed successfully")
			}
		}
	}()

	// Real-time views refresh every 5 minutes
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			ctx := context.Background()

			if err := service.RefreshRealtimeViews(ctx); err != nil {
				logger.Warn("Failed to refresh real-time views",
					zap.Error(err),
				)
			}
		}
	}()

	logger.Info("Background analytics refresh jobs started")
}

// Example usage in main.go
func main() {
	// ... initialization code ...

	// Setup analytics
	analyticsService, basicHandlers, advancedHandlers := setupAnalytics(
		db,
		redisClient,
		logger,
		premiumService,
	)

	// Setup premium middleware
	premiumMiddleware := premium.NewMiddleware(premiumService, logger)

	// Register routes
	registerAnalyticsRoutes(router, basicHandlers, advancedHandlers, premiumMiddleware)

	// Start background jobs
	setupBackgroundJobs(analyticsService, logger)

	// ... rest of application setup ...
}

// Example: Tracking analytics events
func exampleTrackEvent(service *analytics.Service, userID uuid.UUID) error {
	ctx := context.Background()

	// Track a check-in created event
	event := analytics.NewEventBuilder(analytics.EventCheckinCreated).
		WithUser(userID).
		WithSession("session-123").
		WithData("checkin_id", "checkin-456").
		Build()

	return service.TrackEvent(ctx, event)
}

// Example: Checking Premium features before showing analytics UI
func exampleCheckPremiumAccess(premiumService *premium.Service, userID uuid.UUID) (bool, error) {
	ctx := context.Background()

	// Check if user has access to advanced analytics
	err := premiumService.CheckFeatureAccess(ctx, userID, premium.FeatureAdvancedAnalytics)
	if err != nil {
		if err == premium.ErrFeatureNotAvailable || err == premium.ErrSubscriptionExpired {
			return false, nil // User doesn't have access
		}
		return false, err // Actual error
	}

	return true, nil // User has access
}

// Example: Frontend integration
func exampleFrontendIntegration() {
	// JavaScript/TypeScript example

	// Check if user has Premium access
	const checkPremiumAccess = async () => {
		try {
			const response = await fetch('/api/v1/analytics/insights', {
				headers: {
					'Authorization': `Bearer ${token}`,
				},
			});

			if (response.status === 403) {
				// Show upgrade prompt
				showUpgradeModal();
				return false;
			}

			return response.ok;
		} catch (error) {
			console.error('Failed to check Premium access:', error);
			return false;
		}
	};

	// Fetch insights
	const fetchInsights = async () => {
		try {
			const response = await fetch('/api/v1/analytics/insights', {
				headers: {
					'Authorization': `Bearer ${token}`,
				},
			});

			if (!response.ok) {
				throw new Error('Failed to fetch insights');
			}

			const data = await response.json();
			displayInsights(data.insights);
		} catch (error) {
			console.error('Error fetching insights:', error);
		}
	};

	// Fetch forecast
	const fetchForecast = async () => {
		try {
			const response = await fetch('/api/v1/analytics/forecast', {
				headers: {
					'Authorization': `Bearer ${token}`,
				},
			});

			if (!response.ok) {
				throw new Error('Failed to fetch forecast');
			}

			const data = await response.json();
			displayForecast(data);
		} catch (error) {
			console.error('Error fetching forecast:', error);
		}
	};

	// Generate report
	const generateReport = async (reportType = 'comprehensive') => {
		try {
			const response = await fetch('/api/v1/analytics/reports/generate', {
				method: 'POST',
				headers: {
					'Authorization': `Bearer ${token}`,
					'Content-Type': 'application/json',
				},
				body: JSON.stringify({
					report_type: reportType,
					format: 'json',
				}),
			});

			if (!response.ok) {
				throw new Error('Failed to generate report');
			}

			const data = await response.json();
			downloadReport(data.report);
		} catch (error) {
			console.error('Error generating report:', error);
		}
	};
}

// Example: Testing setup
func exampleTestSetup() {
	import (
		"testing"

		"github.com/dev-jelly/donelist/internal/analytics"
		"github.com/dev-jelly/donelist/internal/testutil"
		"github.com/stretchr/testify/require"
	)

	func TestAdvancedAnalytics(t *testing.T) {
		// Setup test environment
		pgContainer, pgCleanup := testutil.SetupPostgresContainer(t)
		defer pgCleanup()

		redisClient, redisCleanup := testutil.SetupRedisContainer(t)
		defer redisCleanup()

		// Create analytics service
		db, err := gorm.Open(postgres.Open(pgContainer.ConnectionString), &gorm.Config{})
		require.NoError(t, err)

		logger := zap.NewNop()
		service := analytics.NewService(db, redisClient, logger)

		// Create test data
		userID := uuid.New()
		for i := 0; i < 30; i++ {
			// Create check-ins for the past 30 days
			createTestCheckin(t, service, userID, time.Now().AddDate(0, 0, -i))
		}

		// Refresh materialized views
		err = service.RefreshMaterializedViews(context.Background())
		require.NoError(t, err)

		// Test insights
		engine := analytics.NewInsightsEngine(service, logger)
		insights, err := engine.GenerateInsights(context.Background(), userID)
		require.NoError(t, err)
		require.NotEmpty(t, insights)

		// Test forecast
		predictiveEngine := analytics.NewPredictiveEngine(service, logger)
		forecast, err := predictiveEngine.ForecastProductivity(context.Background(), userID)
		require.NoError(t, err)
		require.NotNil(t, forecast.NextDay)
	}
}

// Example: Monitoring and observability
func exampleMonitoring(service *analytics.Service) {
	ctx := context.Background()

	// Get cache statistics
	stats, err := service.GetCacheStats(ctx)
	if err != nil {
		logger.Error("Failed to get cache stats", zap.Error(err))
	} else {
		logger.Info("Cache statistics",
			zap.Any("stats", stats),
		)
	}

	// Monitor response times
	start := time.Now()
	insights, err := engine.GenerateInsights(ctx, userID)
	duration := time.Since(start)

	logger.Info("Insights generation completed",
		zap.String("user_id", userID.String()),
		zap.Int("insights_count", len(insights)),
		zap.Duration("duration", duration),
	)

	// Alert if response time is slow
	if duration > 500*time.Millisecond {
		logger.Warn("Slow insights generation",
			zap.String("user_id", userID.String()),
			zap.Duration("duration", duration),
		)
	}
}

// Example: Manual cache management
func exampleCacheManagement(service *analytics.Service, userID uuid.UUID) {
	ctx := context.Background()

	// Invalidate user's analytics cache after major data update
	err := service.InvalidateUserAnalytics(ctx, userID)
	if err != nil {
		logger.Error("Failed to invalidate cache",
			zap.String("user_id", userID.String()),
			zap.Error(err),
		)
	}

	// Force refresh materialized views
	err = service.RefreshMaterializedViews(ctx)
	if err != nil {
		logger.Error("Failed to refresh views",
			zap.Error(err),
		)
	}
}
*/
