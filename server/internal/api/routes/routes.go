package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/dev-jelly/donelist/internal/api/handlers"
	"github.com/dev-jelly/donelist/internal/api/middleware"
	"github.com/dev-jelly/donelist/internal/apikey"
	"github.com/dev-jelly/donelist/internal/auth"
	"github.com/dev-jelly/donelist/internal/mode"
	"go.uber.org/zap"

	ginSwagger "github.com/swaggo/gin-swagger"
	swaggerfiles "github.com/swaggo/files"
)

// SetupRoutes configures all application routes
func SetupRoutes(
	router *gin.Engine,
	authHandler *handlers.AuthHandler,
	userHandler *handlers.UserHandler,
	checkinHandler *handlers.CheckinHandler,
	timelineHandler *handlers.TimelineHandler,
	categoryHandler *handlers.CategoryHandler,
	tagHandler *handlers.TagHandler,
	wsHandler *handlers.WebSocketHandler,
	calendarHandler *handlers.CalendarHandler,
	statisticsHandler *handlers.StatisticsHandler,
	syncHandler *handlers.SyncHandler,
	searchHandler *handlers.SearchHandler,
	apiKeyHandler *handlers.APIKeyHandler,
	modeHandler *handlers.ModeHandler,
	teamHandler *handlers.TeamHandler,
	profileHandler *handlers.ProfileHandler,
	modeService *mode.Service,
	apiKeyService *apikey.Service,
	jwtManager *auth.JWTManager,
	logger *zap.Logger,
) {
	// Create auth middleware
	authMiddleware := middleware.AuthMiddleware(jwtManager, logger)

	// Create mode middleware
	modeMiddleware := mode.ModeMiddleware(modeService, logger)
	teamContextMiddleware := mode.TeamContextMiddleware(logger)

	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		// Public auth routes
		authGroup := v1.Group("/auth")
		{
			authGroup.POST("/register", authHandler.Register)
			authGroup.POST("/login", authHandler.Login)
			authGroup.POST("/refresh", authHandler.Refresh)
			authGroup.POST("/logout", authHandler.Logout)
		}

		// Protected auth routes
		authProtected := v1.Group("/auth")
		authProtected.Use(authMiddleware)
		{
			authProtected.POST("/logout-all", authHandler.LogoutAll)
		}

		// Protected user routes
		users := v1.Group("/users")
		users.Use(authMiddleware)
		{
			users.GET("/me", userHandler.GetMe)
			users.PATCH("/me", userHandler.UpdateMe)
			users.DELETE("/me", userHandler.DeleteMe)
		}

		// Protected profile routes
		profile := v1.Group("/profile")
		profile.Use(authMiddleware)
		{
			profile.GET("", profileHandler.GetProfile)
			profile.PATCH("", profileHandler.UpdateProfile)
			profile.GET("/all", profileHandler.GetAll)
		}

		// Protected settings routes
		settings := v1.Group("/settings")
		settings.Use(authMiddleware)
		{
			settings.GET("", profileHandler.GetSettings)
			settings.PATCH("", profileHandler.UpdateSettings)
			settings.GET("/notifications", profileHandler.GetNotificationSettings)
			settings.PATCH("/notifications", profileHandler.UpdateNotificationSettings)
			settings.GET("/data-retention", profileHandler.GetDataRetentionSettings)
			settings.PATCH("/data-retention", profileHandler.UpdateDataRetentionSettings)
		}

		// Protected checkin routes
		checkins := v1.Group("/checkins")
		checkins.Use(authMiddleware)
		{
			checkins.POST("", checkinHandler.Create)
			checkins.GET("", checkinHandler.List)
			checkins.GET("/:id", checkinHandler.GetByID)
			checkins.PATCH("/:id", checkinHandler.Update)
			checkins.DELETE("/:id", checkinHandler.Delete)
			checkins.GET("/:id/history", checkinHandler.GetEditHistory)
		}

		// Protected timeline routes
		timeline := v1.Group("/timeline")
		timeline.Use(authMiddleware)
		{
			timeline.GET("/daily", timelineHandler.GetDaily)
			timeline.GET("/daily/enhanced", timelineHandler.GetDailyEnhanced)
			timeline.GET("/weekly", timelineHandler.GetWeekly)
			timeline.GET("/monthly", timelineHandler.GetMonthly)
		}

		// Protected calendar routes
		calendar := v1.Group("/calendar")
		calendar.Use(authMiddleware)
		{
			calendar.GET("/monthly", calendarHandler.GetMonthlyCalendar)
			calendar.GET("/month", calendarHandler.GetMonthlyCalendarV2) // V2 with caching and ETags
			calendar.GET("/heatmap", calendarHandler.GetHeatmap)
		}

		// Protected statistics routes
		statistics := v1.Group("/statistics")
		statistics.Use(authMiddleware)
		{
			statistics.GET("/weekly", statisticsHandler.GetWeeklyStatistics)
		}

		// Protected category routes
		categories := v1.Group("/categories")
		categories.Use(authMiddleware)
		{
			categories.POST("", categoryHandler.Create)
			categories.GET("", categoryHandler.List)
			categories.GET("/:id", categoryHandler.GetByID)
			categories.PATCH("/:id", categoryHandler.Update)
			categories.DELETE("/:id", categoryHandler.Delete)
			categories.GET("/colors/recommendations", categoryHandler.GetRecommendedColors)
			categories.GET("/colors/info", categoryHandler.GetColorInfo)
		}

		// Protected tag routes
		tags := v1.Group("/tags")
		tags.Use(authMiddleware)
		{
			tags.GET("/autocomplete", tagHandler.Autocomplete)
			tags.GET("/popular", tagHandler.GetPopular)
			tags.POST("", tagHandler.Create)
			tags.GET("", tagHandler.List)
			tags.GET("/:id", tagHandler.GetByID)
		}

		// Protected sync routes
		sync := v1.Group("/sync")
		sync.Use(authMiddleware)
		{
			sync.POST("", syncHandler.Sync)
			sync.GET("/status", syncHandler.GetSyncStatus)
			sync.GET("/conflicts", syncHandler.GetConflicts)
			sync.POST("/conflicts/resolve", syncHandler.ResolveConflict)
		}

		// Protected search routes
		searchRoutes := v1.Group("/search")
		searchRoutes.Use(authMiddleware)
		{
			// Main search endpoint
			searchRoutes.POST("", searchHandler.Search)

			// Search suggestions and history
			searchRoutes.GET("/suggestions", searchHandler.GetSuggestions)
			searchRoutes.GET("/history", searchHandler.GetHistory)

			// Saved searches (Premium feature)
			searchRoutes.POST("/saved", searchHandler.CreateSavedSearch)
			searchRoutes.GET("/saved", searchHandler.ListSavedSearches)
			searchRoutes.GET("/saved/:id", searchHandler.GetSavedSearch)
			searchRoutes.PATCH("/saved/:id", searchHandler.UpdateSavedSearch)
			searchRoutes.DELETE("/saved/:id", searchHandler.DeleteSavedSearch)
			searchRoutes.POST("/saved/:id/execute", searchHandler.ExecuteSavedSearch)
		}

		// Protected API key management routes
		apiKeys := v1.Group("/api-keys")
		apiKeys.Use(authMiddleware)
		{
			apiKeys.GET("/scopes", apiKeyHandler.GetAvailableScopes)
			apiKeys.POST("", apiKeyHandler.CreateAPIKey)
			apiKeys.GET("", apiKeyHandler.ListAPIKeys)
			apiKeys.GET("/:id", apiKeyHandler.GetAPIKey)
			apiKeys.PATCH("/:id", apiKeyHandler.UpdateAPIKey)
			apiKeys.DELETE("/:id", apiKeyHandler.DeleteAPIKey)
			apiKeys.POST("/:id/revoke", apiKeyHandler.RevokeAPIKey)
			apiKeys.POST("/:id/rotate", apiKeyHandler.RotateAPIKey)
			apiKeys.GET("/:id/statistics", apiKeyHandler.GetAPIKeyUsageStatistics)
		}

		// Protected mode management routes
		modeRoutes := v1.Group("/users")
		modeRoutes.Use(authMiddleware)
		{
			modeRoutes.POST("/mode", modeHandler.SwitchMode)
			modeRoutes.GET("/mode", modeHandler.GetCurrentMode)
			modeRoutes.GET("/available-teams", modeHandler.GetAvailableTeams)
		}

		// Protected team management routes
		teams := v1.Group("/teams")
		teams.Use(authMiddleware)
		{
			teams.POST("", teamHandler.CreateTeam)
			teams.GET("", teamHandler.GetUserTeams)
			teams.GET("/:id", teamHandler.GetTeam)
			teams.PATCH("/:id", teamHandler.UpdateTeam)
			teams.DELETE("/:id", teamHandler.DeleteTeam)
			teams.POST("/:id/members", teamHandler.AddMember)
			teams.GET("/:id/members", teamHandler.GetTeamMembers)
			teams.PATCH("/:id/members/:user_id", teamHandler.UpdateMemberRole)
			teams.DELETE("/:id/members/:user_id", teamHandler.RemoveMember)
			teams.POST("/:id/transfer-ownership", teamHandler.TransferOwnership)
		}

		// Personal mode routes - /api/v1/personal/*
		personal := v1.Group("/personal")
		personal.Use(authMiddleware, modeMiddleware, mode.RequirePersonalMode(logger))
		{
			// Personal checkins
			personalCheckins := personal.Group("/checkins")
			{
				personalCheckins.POST("", checkinHandler.Create)
				personalCheckins.GET("", checkinHandler.List)
				personalCheckins.GET("/:id", checkinHandler.GetByID)
				personalCheckins.PATCH("/:id", checkinHandler.Update)
				personalCheckins.DELETE("/:id", checkinHandler.Delete)
				personalCheckins.GET("/:id/history", checkinHandler.GetEditHistory)
			}

			// Personal timeline
			personalTimeline := personal.Group("/timeline")
			{
				personalTimeline.GET("/daily", timelineHandler.GetDaily)
				personalTimeline.GET("/daily/enhanced", timelineHandler.GetDailyEnhanced)
				personalTimeline.GET("/weekly", timelineHandler.GetWeekly)
				personalTimeline.GET("/monthly", timelineHandler.GetMonthly)
			}

			// Personal calendar
			personalCalendar := personal.Group("/calendar")
			{
				personalCalendar.GET("/monthly", calendarHandler.GetMonthlyCalendar)
				personalCalendar.GET("/heatmap", calendarHandler.GetHeatmap)
			}

			// Personal statistics
			personalStatistics := personal.Group("/statistics")
			{
				personalStatistics.GET("/weekly", statisticsHandler.GetWeeklyStatistics)
			}

			// Personal categories
			personalCategories := personal.Group("/categories")
			{
				personalCategories.POST("", categoryHandler.Create)
				personalCategories.GET("", categoryHandler.List)
				personalCategories.GET("/:id", categoryHandler.GetByID)
				personalCategories.PATCH("/:id", categoryHandler.Update)
				personalCategories.DELETE("/:id", categoryHandler.Delete)
			}

			// Personal tags
			personalTags := personal.Group("/tags")
			{
				personalTags.GET("/autocomplete", tagHandler.Autocomplete)
				personalTags.GET("/popular", tagHandler.GetPopular)
				personalTags.POST("", tagHandler.Create)
				personalTags.GET("", tagHandler.List)
				personalTags.GET("/:id", tagHandler.GetByID)
			}
		}

		// Team mode routes - /api/v1/team/*
		teamMode := v1.Group("/team")
		teamMode.Use(authMiddleware, modeMiddleware, teamContextMiddleware, mode.RequireTeamMode(logger))
		{
			// Team checkins
			teamCheckins := teamMode.Group("/checkins")
			{
				teamCheckins.POST("", checkinHandler.Create)
				teamCheckins.GET("", checkinHandler.List)
				teamCheckins.GET("/:id", checkinHandler.GetByID)
				teamCheckins.PATCH("/:id", checkinHandler.Update)
				teamCheckins.DELETE("/:id", checkinHandler.Delete)
				teamCheckins.GET("/:id/history", checkinHandler.GetEditHistory)
			}

			// Team timeline
			teamTimeline := teamMode.Group("/timeline")
			{
				teamTimeline.GET("/daily", timelineHandler.GetDaily)
				teamTimeline.GET("/daily/enhanced", timelineHandler.GetDailyEnhanced)
				teamTimeline.GET("/weekly", timelineHandler.GetWeekly)
				teamTimeline.GET("/monthly", timelineHandler.GetMonthly)
			}

			// Team calendar
			teamCalendar := teamMode.Group("/calendar")
			{
				teamCalendar.GET("/monthly", calendarHandler.GetMonthlyCalendar)
				teamCalendar.GET("/heatmap", calendarHandler.GetHeatmap)
			}

			// Team statistics
			teamStatistics := teamMode.Group("/statistics")
			{
				teamStatistics.GET("/weekly", statisticsHandler.GetWeeklyStatistics)
			}

			// Team categories
			teamCategories := teamMode.Group("/categories")
			{
				teamCategories.POST("", categoryHandler.Create)
				teamCategories.GET("", categoryHandler.List)
				teamCategories.GET("/:id", categoryHandler.GetByID)
				teamCategories.PATCH("/:id", categoryHandler.Update)
				teamCategories.DELETE("/:id", categoryHandler.Delete)
			}

			// Team tags
			teamTags := teamMode.Group("/tags")
			{
				teamTags.GET("/autocomplete", tagHandler.Autocomplete)
				teamTags.GET("/popular", tagHandler.GetPopular)
				teamTags.POST("", tagHandler.Create)
				teamTags.GET("", tagHandler.List)
				teamTags.GET("/:id", tagHandler.GetByID)
			}
		}
	}

	// WebSocket route (protected by JWT auth middleware)
	router.GET("/ws", authMiddleware, wsHandler.HandleWebSocket)

	// Swagger documentation
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler))
}
