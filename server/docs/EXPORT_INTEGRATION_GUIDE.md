# Export System Integration Guide

## Quick Integration Steps

### Step 1: Add to main.go

After line ~245 where `profileService` is initialized, add:

```go
// Export storage directory configuration
exportStorageDir := os.Getenv("EXPORT_STORAGE_DIR")
if exportStorageDir == "" {
	exportStorageDir = "./exports"
}

// Initialize export service
exportService := export.NewExportService(
	db,
	checkinRepo,
	categoryRepo,
	log,
	exportStorageDir,
)
log.Info("Export service initialized", zap.String("storage_dir", exportStorageDir))

// Initialize export handler
exportHandler := handlers.NewExportHandler(exportService, log)

// Start periodic export cleanup job
go func() {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	log.Info("Starting export cleanup job")
	for range ticker.C {
		ctx := context.Background()
		if err := exportService.CleanupExpiredExports(ctx); err != nil {
			log.Error("Failed to cleanup expired exports", zap.Error(err))
		} else {
			log.Info("Export cleanup completed successfully")
		}
	}
}()
```

Also add the import:
```go
"github.com/dev-jelly/donelist/internal/export"
```

### Step 2: Update routes.go

Update the `SetupRoutes` function signature to include exportHandler:

```go
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
	exportHandler *handlers.ExportHandler, // ADD THIS
	modeService *mode.Service,
	apiKeyService *apikey.Service,
	jwtManager *auth.JWTManager,
	logger *zap.Logger,
) {
```

Then add the export routes after the settings routes (around line 93):

```go
	// Protected export routes
	exports := v1.Group("/export")
	exports.Use(authMiddleware)
	{
		exports.POST("/request", exportHandler.RequestExport)
		exports.GET("/:id/status", exportHandler.GetExportStatus)
		exports.GET("/:id/download", exportHandler.DownloadExport)
		exports.GET("/history", exportHandler.GetExportHistory)
	}
```

### Step 3: Update main.go SetupRoutes call

Find the `routes.SetupRoutes()` call and add exportHandler parameter:

```go
routes.SetupRoutes(
	router,
	authHandler,
	userHandler,
	checkinHandler,
	timelineHandler,
	categoryHandler,
	tagHandler,
	wsHandler,
	calendarHandler,
	statisticsHandler,
	syncHandler,
	searchHandler,
	apiKeyHandler,
	modeHandler,
	teamHandler,
	profileHandler,
	exportHandler, // ADD THIS
	modeService,
	apiKeyService,
	jwtManager,
	log,
)
```

### Step 4: Run Migration

```bash
# Apply the export_jobs table migration
cd server
migrate -path migrations -database "postgresql://user:password@localhost:5432/donelist?sslmode=disable" up

# Or if you have a Makefile target:
make migrate-up
```

### Step 5: Create Export Directory

```bash
# Create the export storage directory
mkdir -p ./exports
chmod 755 ./exports

# For production, use a dedicated directory
mkdir -p /var/donelist/exports
chmod 755 /var/donelist/exports
```

### Step 6: Update Environment Variables

Add to `.env`:

```env
# Export System Configuration
EXPORT_STORAGE_DIR=./exports
```

For production:

```env
EXPORT_STORAGE_DIR=/var/donelist/exports
```

### Step 7: Test the Integration

Start the server:

```bash
go run cmd/api/main.go
```

Test the endpoints:

```bash
# 1. Request an export
curl -X POST http://localhost:8080/api/v1/export/request \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"format": "json"}'

# Response should be 202 with job details
# {"id": "550e8400-...", "status": "pending", ...}

# 2. Check status (use the ID from above)
curl -X GET http://localhost:8080/api/v1/export/550e8400-.../status \
  -H "Authorization: Bearer YOUR_TOKEN"

# 3. Download when completed
curl -X GET http://localhost:8080/api/v1/export/550e8400-.../download \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -o export.json

# 4. Get export history
curl -X GET http://localhost:8080/api/v1/export/history \
  -H "Authorization: Bearer YOUR_TOKEN"
```

## Complete Code Changes

### main.go Changes

Location: After profileService initialization (around line 245)

```go
// ==================== ADD THIS SECTION ====================

// Export storage directory configuration
exportStorageDir := os.Getenv("EXPORT_STORAGE_DIR")
if exportStorageDir == "" {
	exportStorageDir = "./exports"
}

// Initialize export service
exportService := export.NewExportService(
	db,
	checkinRepo,
	categoryRepo,
	log,
	exportStorageDir,
)
log.Info("Export service initialized", zap.String("storage_dir", exportStorageDir))

// Initialize export handler
exportHandler := handlers.NewExportHandler(exportService, log)

// Start periodic export cleanup job
go func() {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	log.Info("Starting export cleanup job")
	for range ticker.C {
		ctx := context.Background()
		if err := exportService.CleanupExpiredExports(ctx); err != nil {
			log.Error("Failed to cleanup expired exports", zap.Error(err))
		} else {
			log.Info("Export cleanup completed successfully")
		}
	}
}()

// ==================== END ADD SECTION ====================
```

And update the import section:

```go
import (
	// ... existing imports ...
	"github.com/dev-jelly/donelist/internal/export"  // ADD THIS
)
```

Then update routes.SetupRoutes call to include exportHandler:

```go
routes.SetupRoutes(
	router,
	authHandler,
	userHandler,
	checkinHandler,
	timelineHandler,
	categoryHandler,
	tagHandler,
	wsHandler,
	calendarHandler,
	statisticsHandler,
	syncHandler,
	searchHandler,
	apiKeyHandler,
	modeHandler,
	teamHandler,
	profileHandler,
	exportHandler,  // ADD THIS
	modeService,
	apiKeyService,
	jwtManager,
	log,
)
```

### routes.go Changes

1. Update function signature:

```go
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
	exportHandler *handlers.ExportHandler, // ADD THIS
	modeService *mode.Service,
	apiKeyService *apikey.Service,
	jwtManager *auth.JWTManager,
	logger *zap.Logger,
) {
```

2. Add export routes after settings routes:

```go
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

	// ==================== ADD THIS SECTION ====================
	// Protected export routes
	exports := v1.Group("/export")
	exports.Use(authMiddleware)
	{
		exports.POST("/request", exportHandler.RequestExport)
		exports.GET("/:id/status", exportHandler.GetExportStatus)
		exports.GET("/:id/download", exportHandler.DownloadExport)
		exports.GET("/history", exportHandler.GetExportHistory)
	}
	// ==================== END ADD SECTION ====================
```

## Verification Checklist

After integration, verify:

- [ ] Server starts without errors
- [ ] Export directory is created
- [ ] Migration 000050 is applied
- [ ] POST /api/v1/export/request returns 202
- [ ] GET /api/v1/export/:id/status returns job details
- [ ] Export files are created in storage directory
- [ ] GET /api/v1/export/:id/download serves files
- [ ] GET /api/v1/export/history returns job list
- [ ] Cleanup job logs appear in server logs
- [ ] Files are deleted after 7 days
- [ ] Swagger docs include export endpoints

## Troubleshooting

### "export: undefined" error
- Make sure you added the import: `"github.com/dev-jelly/donelist/internal/export"`

### "exportHandler: undefined" error in routes.go
- Make sure you added exportHandler to the SetupRoutes parameters

### "too few arguments to routes.SetupRoutes"
- Make sure you added exportHandler to the routes.SetupRoutes call in main.go

### Migration fails
- Check if migration 000050 already exists
- Verify database connection
- Check if export_jobs table already exists

### Export directory errors
- Ensure directory exists and has write permissions
- Check EXPORT_STORAGE_DIR environment variable
- Try absolute path instead of relative

### Tests failing
- Run migrations on test database
- Check testutil.SetupTestDB is working
- Verify go.mod has all dependencies

## Production Deployment Notes

### Storage Considerations
- Use dedicated volume for export storage
- Monitor disk usage
- Consider S3 for cloud deployments
- Implement backup strategy for exports directory

### Performance
- Export processing is asynchronous
- Monitor goroutine count
- Set up alerts for failed exports
- Consider worker pool for high volume

### Security
- Export directory should not be web-accessible
- Only serve files through authenticated API
- Implement rate limiting per user tier
- Regular security audits

### Monitoring
- Track export job success rate
- Monitor storage directory size
- Alert on cleanup job failures
- Log export performance metrics

## Next Steps

After successful integration:

1. Update API documentation
2. Add export feature to user guide
3. Create monitoring dashboards
4. Set up alerts for failures
5. Plan for premium tier features
6. Consider email notifications
7. Evaluate cloud storage migration

## Support

For issues or questions:
- Check server logs for errors
- Review `/internal/export/README.md`
- Test with provided curl commands
- Verify all migration steps completed
