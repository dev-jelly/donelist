// @title Donelist API
// @version 1.0
// @description A comprehensive API for managing check-ins, timelines, categories, and user data. This API supports JWT-based authentication and provides real-time updates via WebSocket.
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.url https://github.com/dev-jelly/donelist
// @contact.email support@donelist.io

// @license.name MIT
// @license.url https://opensource.org/licenses/MIT

// @host localhost:8080
// @BasePath /api/v1

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Enter your bearer token in the format: Bearer {token}

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/dev-jelly/donelist/internal/api/handlers"
	"github.com/dev-jelly/donelist/internal/api/routes"
	"github.com/dev-jelly/donelist/internal/apikey"
	"github.com/dev-jelly/donelist/internal/auth"
	"github.com/dev-jelly/donelist/internal/calendar"
	"github.com/dev-jelly/donelist/internal/category"
	"github.com/dev-jelly/donelist/internal/checkin"
	"github.com/dev-jelly/donelist/internal/config"
	"github.com/dev-jelly/donelist/internal/health"
	"github.com/dev-jelly/donelist/internal/profile"
	"github.com/dev-jelly/donelist/internal/metrics"
	"github.com/dev-jelly/donelist/internal/middleware"
	"github.com/dev-jelly/donelist/internal/mode"
	"github.com/dev-jelly/donelist/internal/search"
	"github.com/dev-jelly/donelist/internal/statistics"
	"github.com/dev-jelly/donelist/internal/sync"
	"github.com/dev-jelly/donelist/internal/tag"
	"github.com/dev-jelly/donelist/internal/team"
	"github.com/dev-jelly/donelist/internal/timeline"
	"github.com/dev-jelly/donelist/internal/user"
	"github.com/dev-jelly/donelist/internal/websocket"
	"github.com/dev-jelly/donelist/pkg/database"
	"github.com/dev-jelly/donelist/pkg/logger"
	_ "github.com/dev-jelly/donelist/docs"
	"go.uber.org/zap"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log, err := logger.New(cfg.App.Env, cfg.App.LogLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync(log)

	log.Info("Starting Donelist API Server",
		zap.String("env", cfg.App.Env),
		zap.String("version", "1.0.0"),
	)

	// Initialize PostgreSQL connection
	pgConfig := database.PostgresConfig{
		Host:            cfg.Database.Host,
		Port:            cfg.Database.Port,
		User:            cfg.Database.User,
		Password:        cfg.Database.Password,
		Database:        cfg.Database.Name,
		SSLMode:         cfg.Database.SSLMode,
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
		ConnMaxIdleTime: 1 * time.Minute,
	}

	db, err := database.NewPostgres(pgConfig, log)
	if err != nil {
		log.Fatal("Failed to connect to PostgreSQL", zap.Error(err))
	}
	defer database.Close(db, log)

	// Initialize Redis connection
	redisConfig := database.RedisConfig{
		Host:     cfg.Redis.Host,
		Port:     cfg.Redis.Port,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	}

	redisClient, err := database.NewRedis(redisConfig, log)
	if err != nil {
		log.Fatal("Failed to connect to Redis", zap.Error(err))
	}
	defer database.CloseRedis(redisClient, log)

	// Initialize JWT manager
	jwtManager, err := auth.NewJWTManager(auth.JWTConfig{
		SigningMethod: auth.SigningMethodHS256,
		Secret:        cfg.JWT.Secret,
		AccessExpiry:  cfg.JWT.AccessTokenExpiry,
		RefreshExpiry: cfg.JWT.RefreshTokenExpiry,
	})
	if err != nil {
		log.Fatal("Failed to create JWT manager", zap.Error(err))
	}

	// Initialize metrics
	appMetrics := metrics.NewMetrics()
	log.Info("Metrics initialized")

	// Initialize health check service
	healthService := health.NewService("1.0.0", log)

	// Register health checkers
	healthService.RegisterChecker(health.NewPostgresChecker(db, 2*time.Second))
	healthService.RegisterChecker(health.NewRedisChecker(redisClient, 2*time.Second))
	healthService.RegisterChecker(health.NewSystemChecker(
		func() int64 {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			return int64(m.Alloc / 1024 / 1024) // Convert to MB
		},
		func() int {
			return runtime.NumGoroutine()
		},
	))
	log.Info("Health checks registered")

	// Initialize WebSocket hub
	hub := websocket.NewHub(log)
	go hub.Run() // Start hub in a goroutine

	// Initialize repositories
	userRepo := user.NewRepository(db)
	refreshTokenRepo := auth.NewRefreshTokenRepository(db)
	checkinRepo := checkin.NewRepository(db)
	categoryRepo := category.NewRepository(db)
	tagRepo := tag.NewRepository(db)
	calendarRepo := calendar.NewRepository(db)
	statisticsRepo := statistics.NewRepository(db)
	syncRepo := sync.NewRepository(db)
	searchRepo := search.NewRepository(db)
	apiKeyRepo := apikey.NewRepository(db)
	teamRepo := team.NewRepository(db, log)
	profileRepo := profile.NewRepository(db)

	// Initialize services
	authService := auth.NewService(userRepo, refreshTokenRepo, jwtManager, log)
	userService := user.NewService(userRepo, log)
	checkinService := checkin.NewService(checkinRepo, categoryRepo, tagRepo, userRepo, hub, log)
	timelineService := timeline.NewService(checkinRepo, categoryRepo, log)
	categoryService := category.NewService(categoryRepo, log)
	tagService := tag.NewService(tagRepo, log)
	calendarService := calendar.NewService(calendarRepo, log)
	statisticsService := statistics.NewService(statisticsRepo, log)
	// TODO: Fix sync service interface compatibility
	// conflictResolver := sync.NewConflictResolver(checkinRepo)
	syncService := sync.NewService(syncRepo, nil, nil, log)
	searchService := search.NewService(searchRepo, userRepo, log)
	apiKeyService := apikey.NewService(apiKeyRepo, log)
	modeService := mode.NewService(log)
	teamService := team.NewService(teamRepo, log)
	profileService := profile.NewService(profileRepo, log)

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(authService, log)
	userHandler := handlers.NewUserHandler(userService, log)
	checkinHandler := handlers.NewCheckinHandler(checkinService, log)
	timelineHandler := handlers.NewTimelineHandler(timelineService, log)
	categoryHandler := handlers.NewCategoryHandler(categoryService, log)
	categoryMergeHandler := handlers.NewCategoryMergeHandler(db, categoryRepo, log)
	tagHandler := handlers.NewTagHandler(tagService, log)
	wsHandler := handlers.NewWebSocketHandler(hub, log)
	calendarHandler := handlers.NewCalendarHandler(calendarService, log)
	statisticsHandler := handlers.NewStatisticsHandler(statisticsService, log)
	syncHandler := handlers.NewSyncHandler(syncService, log)
	searchHandler := handlers.NewSearchHandler(searchService, log)
	apiKeyHandler := handlers.NewAPIKeyHandler(apiKeyService, log)
	modeHandler := handlers.NewModeHandler(modeService, teamService, userRepo, log)
	teamHandler := handlers.NewTeamHandler(teamService, log)
	profileHandler := handlers.NewProfileHandler(profileService, log)

	// Set Gin mode
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	// Create Gin router without default middleware
	router := gin.New()

	// Add custom middleware in order of execution
	router.Use(middleware.RecoveryMiddleware(log))       // Panic recovery (first)
	router.Use(middleware.CorrelationMiddleware(log))    // Correlation IDs

	// Security middleware
	router.Use(middleware.CORSMiddleware(cfg.CORS, log)) // CORS policies

	// Create and apply security headers based on environment
	var securityHeadersConfig middleware.SecurityHeadersConfig
	if cfg.IsProduction() {
		securityHeadersConfig = middleware.CreateAPISecurityHeadersConfig(log, true)
	} else {
		securityHeadersConfig = middleware.CreateDevelopmentSecurityHeadersConfig(log)
	}
	router.Use(middleware.SecurityHeadersMiddleware(securityHeadersConfig))

	router.Use(metrics.MetricsMiddleware(appMetrics))    // Prometheus metrics
	router.Use(middleware.LoggingMiddleware(log))        // Request logging

	// Prometheus metrics endpoint
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Simple health check endpoint (for load balancers)
	healthHandler := func(c *gin.Context) {
		status, _ := healthService.CheckHealthSimple()
		httpStatus := http.StatusOK
		if status != health.StatusHealthy {
			httpStatus = http.StatusServiceUnavailable
		}

		buildInfo := health.GetBuildInfo()
		c.JSON(httpStatus, gin.H{
			"status":     status,
			"time":       time.Now().Format(time.RFC3339),
			"version":    buildInfo.Version,
			"git_commit": buildInfo.GitCommit,
		})
	}
	router.GET("/health", healthHandler)
	router.GET("/healthz", healthHandler) // Kubernetes-style endpoint

	// Detailed health check endpoint with full build info
	detailHandler := func(c *gin.Context) {
		report := healthService.CheckHealthWithBuildInfo()
		httpStatus := http.StatusOK
		if report.Status != health.StatusHealthy {
			httpStatus = http.StatusServiceUnavailable
		}

		c.JSON(httpStatus, report)
	}
	router.GET("/health/detail", detailHandler)
	router.GET("/healthz/detail", detailHandler) // Kubernetes-style endpoint

	// Readiness check endpoint (for Kubernetes)
	readyHandler := func(c *gin.Context) {
		report := healthService.CheckHealth()
		buildInfo := health.GetBuildInfo()

		// Check if all critical components are healthy
		dbHealth, dbOk := report.Components["postgresql"]
		redisHealth, redisOk := report.Components["redis"]

		response := gin.H{
			"version":    buildInfo.Version,
			"git_commit": buildInfo.GitCommit,
			"uptime":     healthService.GetUptime().Seconds(),
		}

		if !dbOk || dbHealth.Status != health.StatusHealthy {
			response["status"] = "not ready"
			response["error"] = "database not available"
			if dbHealth.Error != "" {
				response["details"] = dbHealth.Error
			}
			c.JSON(http.StatusServiceUnavailable, response)
			return
		}

		if !redisOk || redisHealth.Status != health.StatusHealthy {
			response["status"] = "not ready"
			response["error"] = "redis not available"
			if redisHealth.Error != "" {
				response["details"] = redisHealth.Error
			}
			c.JSON(http.StatusServiceUnavailable, response)
			return
		}

		response["status"] = "ready"
		response["database"] = "connected"
		response["redis"] = "connected"
		c.JSON(http.StatusOK, response)
	}
	router.GET("/ready", readyHandler)
	router.GET("/readyz", readyHandler) // Kubernetes-style endpoint

	// Liveness check endpoint (for Kubernetes)
	liveHandler := func(c *gin.Context) {
		buildInfo := health.GetBuildInfo()
		c.JSON(http.StatusOK, gin.H{
			"status":     "alive",
			"time":       time.Now().Format(time.RFC3339),
			"version":    buildInfo.Version,
			"git_commit": buildInfo.GitCommit,
			"uptime":     healthService.GetUptime().Seconds(),
		})
	}
	router.GET("/live", liveHandler)
	router.GET("/livez", liveHandler) // Kubernetes-style endpoint

	// Setup API routes
	routes.SetupRoutes(router, authHandler, userHandler, checkinHandler, timelineHandler, categoryHandler, categoryMergeHandler, tagHandler, wsHandler, calendarHandler, statisticsHandler, syncHandler, searchHandler, apiKeyHandler, modeHandler, teamHandler, profileHandler, modeService, apiKeyService, jwtManager, log)

	// Create HTTP server
	srv := &http.Server{
		Addr:           fmt.Sprintf(":%s", cfg.Server.Port),
		Handler:        router,
		ReadTimeout:    cfg.Server.ReadTimeout,
		WriteTimeout:   cfg.Server.WriteTimeout,
		MaxHeaderBytes: cfg.Server.MaxHeaderBytes,
	}

	// Start server in goroutine
	go func() {
		log.Info("Server starting", zap.String("port", cfg.Server.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown", zap.Error(err))
	}

	log.Info("Server exited gracefully")
}
