package security

import (
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// SecurityConfig holds comprehensive security configuration
type SecurityConfig struct {
	// Anomaly detection
	AnomalyDetection AnomalyDetectorConfig

	// Alert management
	AlertManagement AlertManagerConfig

	// Auto-blocking
	AutoBlocking AutoBlockerConfig

	// Feature flags
	EnableAnomalyDetection bool
	EnableAlerts           bool
	EnableAutoBlocking     bool
}

// DefaultSecurityConfig returns a production-ready security configuration
func DefaultSecurityConfig(db *gorm.DB, redis *redis.Client, logger *zap.Logger) *SecurityConfig {
	return &SecurityConfig{
		EnableAnomalyDetection: true,
		EnableAlerts:           true,
		EnableAutoBlocking:     true,

		AnomalyDetection: AnomalyDetectorConfig{
			RedisClient:             redis,
			Logger:                  logger,
			RapidRequestThreshold:   100,
			RapidRequestWindow:      1 * time.Minute,
			FailureRateThreshold:    0.5, // 50%
			FailureRateWindow:       5 * time.Minute,
			FailureRateMinRequests:  10,
			BruteForceThreshold:     10,
			BruteForceWindow:        10 * time.Minute,
			EnableGeoCheck:          false, // Requires IP geolocation service
			GeoChangeThreshold:      1 * time.Hour,
			EnableDeviceCheck:       true,
			DeviceChangeThreshold:   1 * time.Hour,
			UnusualHoursStart:       2, // 2 AM
			UnusualHoursEnd:         5, // 5 AM
			MultipleIPsThreshold:    5,
			MultipleIPsWindow:       10 * time.Minute,
		},

		AlertManagement: AlertManagerConfig{
			DB:                 db,
			Logger:             logger,
			EnabledChannels:    []AlertChannel{AlertChannelLog, AlertChannelWebhook},
			ThrottleWindow:     5 * time.Minute,
			MaxAlertsPerWindow: 10,
			CriticalChannels:   []AlertChannel{AlertChannelLog, AlertChannelWebhook, AlertChannelSlack, AlertChannelPagerDuty},
			HighChannels:       []AlertChannel{AlertChannelLog, AlertChannelWebhook, AlertChannelSlack},
			MediumChannels:     []AlertChannel{AlertChannelLog, AlertChannelWebhook},
			LowChannels:        []AlertChannel{AlertChannelLog},
		},

		AutoBlocking: AutoBlockerConfig{
			DB:                    db,
			RedisClient:           redis,
			Logger:                logger,
			EnableAutoBlock:       true,
			AutoBlockOnCritical:   true,
			AutoBlockOnHighCount:  3,
			AutoBlockWindow:       10 * time.Minute,
			CriticalBlockDuration: 24 * time.Hour,
			HighBlockDuration:     2 * time.Hour,
			MediumBlockDuration:   30 * time.Minute,
			LowBlockDuration:      5 * time.Minute,
			WhitelistedIdentifiers: []string{},
			WhitelistedIPs:        []string{"127.0.0.1", "::1"},
		},
	}
}

// DevelopmentSecurityConfig returns a development-friendly configuration
func DevelopmentSecurityConfig(db *gorm.DB, redis *redis.Client, logger *zap.Logger) *SecurityConfig {
	config := DefaultSecurityConfig(db, redis, logger)

	// Relax thresholds for development
	config.AnomalyDetection.RapidRequestThreshold = 200
	config.AnomalyDetection.BruteForceThreshold = 20
	config.AnomalyDetection.FailureRateThreshold = 0.7 // 70%

	config.AutoBlocking.EnableAutoBlock = false // Disable auto-blocking in dev
	config.AutoBlocking.CriticalBlockDuration = 5 * time.Minute
	config.AutoBlocking.HighBlockDuration = 2 * time.Minute

	config.AlertManagement.EnabledChannels = []AlertChannel{AlertChannelLog}

	return config
}

// TestSecurityConfig returns a configuration suitable for testing
func TestSecurityConfig(db *gorm.DB, redis *redis.Client, logger *zap.Logger) *SecurityConfig {
	config := DefaultSecurityConfig(db, redis, logger)

	// Very permissive thresholds for testing
	config.AnomalyDetection.RapidRequestThreshold = 1000
	config.AnomalyDetection.BruteForceThreshold = 50
	config.AnomalyDetection.FailureRateThreshold = 0.9 // 90%

	config.AutoBlocking.EnableAutoBlock = false
	config.AlertManagement.EnabledChannels = []AlertChannel{AlertChannelLog}

	return config
}

// StrictSecurityConfig returns a strict configuration for high-security environments
func StrictSecurityConfig(db *gorm.DB, redis *redis.Client, logger *zap.Logger) *SecurityConfig {
	config := DefaultSecurityConfig(db, redis, logger)

	// Stricter thresholds
	config.AnomalyDetection.RapidRequestThreshold = 50
	config.AnomalyDetection.RapidRequestWindow = 30 * time.Second
	config.AnomalyDetection.BruteForceThreshold = 5
	config.AnomalyDetection.FailureRateThreshold = 0.3 // 30%
	config.AnomalyDetection.EnableGeoCheck = true
	config.AnomalyDetection.GeoChangeThreshold = 30 * time.Minute

	config.AutoBlocking.AutoBlockOnCritical = true
	config.AutoBlocking.AutoBlockOnHighCount = 2
	config.AutoBlocking.CriticalBlockDuration = 7 * 24 * time.Hour // 1 week
	config.AutoBlocking.HighBlockDuration = 24 * time.Hour

	config.AlertManagement.EnabledChannels = []AlertChannel{
		AlertChannelLog,
		AlertChannelWebhook,
		AlertChannelSlack,
		AlertChannelPagerDuty,
	}

	return config
}

// SecurityManager orchestrates all security components
type SecurityManager struct {
	Detector     *AnomalyDetector
	AlertManager *AlertManager
	AutoBlocker  *AutoBlocker
	Config       *SecurityConfig
	Logger       *zap.Logger
}

// NewSecurityManager creates a new security manager with all components
func NewSecurityManager(config *SecurityConfig) *SecurityManager {
	var detector *AnomalyDetector
	var alertManager *AlertManager
	var autoBlocker *AutoBlocker

	if config.EnableAnomalyDetection {
		detector = NewAnomalyDetector(config.AnomalyDetection)
	}

	if config.EnableAlerts {
		alertManager = NewAlertManager(config.AlertManagement)
	}

	if config.EnableAutoBlocking {
		autoBlocker = NewAutoBlocker(config.AutoBlocking)
	}

	return &SecurityManager{
		Detector:     detector,
		AlertManager: alertManager,
		AutoBlocker:  autoBlocker,
		Config:       config,
		Logger:       config.AnomalyDetection.Logger,
	}
}

// Start starts background workers
func (sm *SecurityManager) Start() {
	// Start cleanup worker for expired blocks
	if sm.AutoBlocker != nil {
		go sm.AutoBlocker.StartCleanupWorker(nil)
	}
}

// GetStatistics returns comprehensive security statistics
func (sm *SecurityManager) GetStatistics(since time.Time) (*SecurityStatistics, error) {
	stats := &SecurityStatistics{}

	// Get alert statistics
	if sm.AlertManager != nil {
		alertStats, err := sm.AlertManager.GetAlertStatistics(nil, since)
		if err != nil {
			return nil, err
		}
		stats.Alerts = alertStats
	}

	// Get block statistics
	if sm.AutoBlocker != nil {
		blockStats, err := sm.AutoBlocker.GetBlockStatistics(nil, since)
		if err != nil {
			return nil, err
		}
		stats.Blocks = blockStats
	}

	return stats, nil
}

// SecurityStatistics represents comprehensive security statistics
type SecurityStatistics struct {
	Alerts *AlertStatistics `json:"alerts"`
	Blocks *BlockStatistics `json:"blocks"`
}
