package handlers

import (
	"net/http"
	"time"

	"github.com/dev-jelly/donelist/internal/health"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// HealthHandler handles health check endpoints
type HealthHandler struct {
	service *health.Service
	logger  *zap.Logger
}

// NewHealthHandler creates a new health handler
func NewHealthHandler(service *health.Service, logger *zap.Logger) *HealthHandler {
	return &HealthHandler{
		service: service,
		logger:  logger,
	}
}

// HealthCheck handles the basic health check endpoint
// @Summary Basic health check
// @Description Returns basic health status for load balancers
// @Tags health
// @Produce json
// @Success 200 {object} map[string]interface{} "Service is healthy"
// @Failure 503 {object} map[string]interface{} "Service is unhealthy"
// @Router /health [get]
// @Router /healthz [get]
func (h *HealthHandler) HealthCheck(c *gin.Context) {
	status, _ := h.service.CheckHealthSimple()
	httpStatus := http.StatusOK
	if status != health.StatusHealthy {
		httpStatus = http.StatusServiceUnavailable
		h.logger.Warn("health check failed", zap.String("status", string(status)))
	}

	buildInfo := health.GetBuildInfo()
	c.JSON(httpStatus, gin.H{
		"status":     status,
		"time":       time.Now().Format(time.RFC3339),
		"version":    buildInfo.Version,
		"git_commit": buildInfo.GitCommit,
	})
}

// DetailedHealthCheck handles the detailed health check endpoint
// @Summary Detailed health check
// @Description Returns detailed health status with component information and build details
// @Tags health
// @Produce json
// @Success 200 {object} health.UpdatedHealthReport "Detailed health report"
// @Failure 503 {object} health.UpdatedHealthReport "Service is unhealthy"
// @Router /health/detail [get]
// @Router /healthz/detail [get]
func (h *HealthHandler) DetailedHealthCheck(c *gin.Context) {
	report := h.service.CheckHealthWithBuildInfo()
	httpStatus := http.StatusOK
	if report.Status != health.StatusHealthy {
		httpStatus = http.StatusServiceUnavailable
		h.logger.Warn("detailed health check failed",
			zap.String("status", string(report.Status)),
			zap.Any("components", report.Components),
		)
	}

	c.JSON(httpStatus, report)
}

// ReadinessCheck handles the readiness check endpoint (for Kubernetes)
// @Summary Readiness probe
// @Description Checks if the service is ready to accept traffic (all critical dependencies available)
// @Tags health
// @Produce json
// @Success 200 {object} map[string]interface{} "Service is ready"
// @Failure 503 {object} map[string]interface{} "Service is not ready"
// @Router /ready [get]
// @Router /readyz [get]
func (h *HealthHandler) ReadinessCheck(c *gin.Context) {
	report := h.service.CheckHealth()
	buildInfo := health.GetBuildInfo()

	// Check if all critical components are healthy
	dbHealth, dbOk := report.Components["postgresql"]
	redisHealth, redisOk := report.Components["redis"]

	response := gin.H{
		"version":    buildInfo.Version,
		"git_commit": buildInfo.GitCommit,
		"git_branch": buildInfo.GitBranch,
		"build_time": buildInfo.BuildTime,
		"uptime":     h.service.GetUptime().Seconds(),
	}

	// Check database readiness
	if !dbOk || dbHealth.Status != health.StatusHealthy {
		response["status"] = "not ready"
		response["error"] = "database not available"
		if dbHealth.Error != "" {
			response["details"] = dbHealth.Error
		}
		h.logger.Warn("readiness check failed: database not available",
			zap.String("db_status", string(dbHealth.Status)),
			zap.String("db_error", dbHealth.Error),
		)
		c.JSON(http.StatusServiceUnavailable, response)
		return
	}

	// Check Redis readiness
	if !redisOk || redisHealth.Status != health.StatusHealthy {
		response["status"] = "not ready"
		response["error"] = "redis not available"
		if redisHealth.Error != "" {
			response["details"] = redisHealth.Error
		}
		h.logger.Warn("readiness check failed: redis not available",
			zap.String("redis_status", string(redisHealth.Status)),
			zap.String("redis_error", redisHealth.Error),
		)
		c.JSON(http.StatusServiceUnavailable, response)
		return
	}

	// All critical components are healthy
	response["status"] = "ready"
	response["database"] = gin.H{
		"status":  "connected",
		"message": dbHealth.Message,
	}
	response["redis"] = gin.H{
		"status":  "connected",
		"message": redisHealth.Message,
	}
	c.JSON(http.StatusOK, response)
}

// LivenessCheck handles the liveness check endpoint (for Kubernetes)
// @Summary Liveness probe
// @Description Checks if the service is alive (simple uptime check)
// @Tags health
// @Produce json
// @Success 200 {object} map[string]interface{} "Service is alive"
// @Router /live [get]
// @Router /livez [get]
func (h *HealthHandler) LivenessCheck(c *gin.Context) {
	buildInfo := health.GetBuildInfo()
	c.JSON(http.StatusOK, gin.H{
		"status":     "alive",
		"time":       time.Now().Format(time.RFC3339),
		"version":    buildInfo.Version,
		"git_commit": buildInfo.GitCommit,
		"git_branch": buildInfo.GitBranch,
		"build_time": buildInfo.BuildTime,
		"uptime":     h.service.GetUptime().Seconds(),
	})
}
