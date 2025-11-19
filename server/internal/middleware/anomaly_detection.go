package middleware

import (
	"net/http"

	"github.com/dev-jelly/donelist/internal/security"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AnomalyDetectionConfig holds configuration for anomaly detection middleware
type AnomalyDetectionConfig struct {
	Detector     *security.AnomalyDetector
	AlertManager *security.AlertManager
	AutoBlocker  *security.AutoBlocker
	Logger       *zap.Logger

	// Detection settings
	EnableRapidRequestCheck bool
	EnableFailureRateCheck  bool
	EnableBruteForceCheck   bool
	EnableGeoCheck          bool
	EnableDeviceCheck       bool
	EnableUnusualHoursCheck bool
	EnableMultipleIPsCheck  bool

	// Response behavior
	BlockOnDetection bool // If false, only log/alert but don't block request
}

// AnomalyDetectionMiddleware creates a middleware for anomaly detection
func AnomalyDetectionMiddleware(cfg AnomalyDetectionConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		// Extract identifier (user ID or IP)
		identifier := c.ClientIP()
		if userID, exists := c.Get("user_id"); exists {
			identifier = userID.(string)
		}

		ipAddress := c.ClientIP()
		userAgent := c.Request.UserAgent()

		// Check if blocked first
		if cfg.AutoBlocker != nil {
			isBlocked, err := cfg.AutoBlocker.IsBlocked(ctx, identifier)
			if err != nil {
				cfg.Logger.Error("Failed to check block status", zap.Error(err))
			} else if isBlocked {
				blockInfo, _ := cfg.AutoBlocker.GetBlockInfo(ctx, identifier)

				cfg.Logger.Warn("Blocked request",
					zap.String("identifier", identifier),
					zap.String("ip", ipAddress),
					zap.String("path", c.Request.URL.Path),
				)

				c.JSON(http.StatusForbidden, gin.H{
					"error":   "Access temporarily blocked",
					"message": "Your access has been temporarily blocked due to suspicious activity",
					"details": gin.H{
						"reason":     blockInfo.Reason,
						"expires_at": blockInfo.ExpiresAt,
						"blocked_at": blockInfo.BlockedAt,
					},
				})
				c.Abort()
				return
			}
		}

		// Perform anomaly checks
		var detectedAnomalies []*security.AnomalyEvent

		// Check rapid requests
		if cfg.EnableRapidRequestCheck && cfg.Detector != nil {
			if anomaly, err := cfg.Detector.CheckRapidRequests(ctx, identifier); err != nil {
				cfg.Logger.Error("Rapid request check failed", zap.Error(err))
			} else if anomaly != nil {
				anomaly.IPAddress = ipAddress
				anomaly.UserAgent = userAgent
				detectedAnomalies = append(detectedAnomalies, anomaly)
			}
		}

		// Check unusual hours
		if cfg.EnableUnusualHoursCheck && cfg.Detector != nil {
			if anomaly, err := cfg.Detector.CheckUnusualHours(ctx, identifier); err != nil {
				cfg.Logger.Error("Unusual hours check failed", zap.Error(err))
			} else if anomaly != nil {
				anomaly.IPAddress = ipAddress
				anomaly.UserAgent = userAgent
				detectedAnomalies = append(detectedAnomalies, anomaly)
			}
		}

		// Check multiple IPs
		if cfg.EnableMultipleIPsCheck && cfg.Detector != nil {
			if anomaly, err := cfg.Detector.CheckMultipleIPs(ctx, identifier, ipAddress); err != nil {
				cfg.Logger.Error("Multiple IPs check failed", zap.Error(err))
			} else if anomaly != nil {
				anomaly.UserAgent = userAgent
				detectedAnomalies = append(detectedAnomalies, anomaly)
			}
		}

		// Check geographic anomaly (requires location extraction from IP)
		if cfg.EnableGeoCheck && cfg.Detector != nil {
			// TODO: Implement IP geolocation
			// location := extractLocationFromIP(ipAddress)
			// if anomaly, err := cfg.Detector.CheckGeographicAnomaly(ctx, identifier, location); err != nil {
			//     cfg.Logger.Error("Geographic check failed", zap.Error(err))
			// } else if anomaly != nil {
			//     anomaly.IPAddress = ipAddress
			//     anomaly.UserAgent = userAgent
			//     detectedAnomalies = append(detectedAnomalies, anomaly)
			// }
		}

		// Check device anomaly
		if cfg.EnableDeviceCheck && cfg.Detector != nil {
			if anomaly, err := cfg.Detector.CheckDeviceAnomaly(ctx, identifier, userAgent); err != nil {
				cfg.Logger.Error("Device check failed", zap.Error(err))
			} else if anomaly != nil {
				anomaly.IPAddress = ipAddress
				detectedAnomalies = append(detectedAnomalies, anomaly)
			}
		}

		// Store anomalies in context for later use
		if len(detectedAnomalies) > 0 {
			c.Set("anomalies", detectedAnomalies)
		}

		// Process request
		c.Next()

		// After request, check failure rate and brute force based on response
		statusCode := c.Writer.Status()
		isSuccess := statusCode >= 200 && statusCode < 400

		// Check failure rate
		if cfg.EnableFailureRateCheck && cfg.Detector != nil {
			if anomaly, err := cfg.Detector.CheckFailureRate(ctx, identifier, isSuccess); err != nil {
				cfg.Logger.Error("Failure rate check failed", zap.Error(err))
			} else if anomaly != nil {
				anomaly.IPAddress = ipAddress
				anomaly.UserAgent = userAgent
				detectedAnomalies = append(detectedAnomalies, anomaly)
			}
		}

		// Check brute force on failed auth attempts
		if cfg.EnableBruteForceCheck && cfg.Detector != nil && !isSuccess {
			if c.Request.URL.Path == "/api/v1/auth/login" {
				if anomaly, err := cfg.Detector.CheckBruteForce(ctx, identifier); err != nil {
					cfg.Logger.Error("Brute force check failed", zap.Error(err))
				} else if anomaly != nil {
					anomaly.IPAddress = ipAddress
					anomaly.UserAgent = userAgent
					detectedAnomalies = append(detectedAnomalies, anomaly)
				}
			}
		}

		// Process detected anomalies
		for _, anomaly := range detectedAnomalies {
			// Send alert
			if cfg.AlertManager != nil {
				if err := cfg.AlertManager.SendAlert(ctx, anomaly); err != nil {
					cfg.Logger.Error("Failed to send alert",
						zap.Error(err),
						zap.String("anomaly_type", string(anomaly.Type)),
					)
				}
			}

			// Evaluate auto-blocking
			if cfg.AutoBlocker != nil {
				shouldBlock, err := cfg.AutoBlocker.EvaluateBlock(ctx, anomaly)
				if err != nil {
					cfg.Logger.Error("Failed to evaluate block",
						zap.Error(err),
						zap.String("identifier", identifier),
					)
				} else if shouldBlock {
					cfg.Logger.Warn("Identifier auto-blocked",
						zap.String("identifier", identifier),
						zap.String("reason", string(anomaly.Type)),
					)
				}
			}
		}
	}
}

// CreateDefaultAnomalyDetectionConfig creates a default configuration
func CreateDefaultAnomalyDetectionConfig(
	detector *security.AnomalyDetector,
	alertManager *security.AlertManager,
	autoBlocker *security.AutoBlocker,
	logger *zap.Logger,
) AnomalyDetectionConfig {
	return AnomalyDetectionConfig{
		Detector:                detector,
		AlertManager:            alertManager,
		AutoBlocker:             autoBlocker,
		Logger:                  logger,
		EnableRapidRequestCheck: true,
		EnableFailureRateCheck:  true,
		EnableBruteForceCheck:   true,
		EnableGeoCheck:          false, // Requires IP geolocation service
		EnableDeviceCheck:       true,
		EnableUnusualHoursCheck: true,
		EnableMultipleIPsCheck:  true,
		BlockOnDetection:        true,
	}
}

// BlockCheckMiddleware is a lightweight middleware that only checks if identifier is blocked
func BlockCheckMiddleware(autoBlocker *security.AutoBlocker, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		// Extract identifier
		identifier := c.ClientIP()
		if userID, exists := c.Get("user_id"); exists {
			identifier = userID.(string)
		}

		// Check if blocked
		isBlocked, err := autoBlocker.IsBlocked(ctx, identifier)
		if err != nil {
			logger.Error("Failed to check block status", zap.Error(err))
			c.Next()
			return
		}

		if isBlocked {
			blockInfo, _ := autoBlocker.GetBlockInfo(ctx, identifier)

			logger.Warn("Blocked request",
				zap.String("identifier", identifier),
				zap.String("ip", c.ClientIP()),
				zap.String("path", c.Request.URL.Path),
			)

			response := gin.H{
				"error":   "Access temporarily blocked",
				"message": "Your access has been temporarily blocked due to suspicious activity",
			}

			if blockInfo != nil {
				response["details"] = gin.H{
					"reason":     blockInfo.Reason,
					"expires_at": blockInfo.ExpiresAt,
					"blocked_at": blockInfo.BlockedAt,
				}
			}

			c.JSON(http.StatusForbidden, response)
			c.Abort()
			return
		}

		c.Next()
	}
}
