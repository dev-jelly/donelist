package security

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// AnomalyType represents different types of anomalies
type AnomalyType string

const (
	AnomalyTypeRapidRequests       AnomalyType = "rapid_requests"
	AnomalyTypeHighFailureRate     AnomalyType = "high_failure_rate"
	AnomalyTypeGeographicAnomaly   AnomalyType = "geographic_anomaly"
	AnomalyTypeDeviceAnomaly       AnomalyType = "device_anomaly"
	AnomalyTypeSuspiciousPattern   AnomalyType = "suspicious_pattern"
	AnomalyTypeBruteForce          AnomalyType = "brute_force"
	AnomalyTypeUnusualHours        AnomalyType = "unusual_hours"
	AnomalyTypeMultipleIPs         AnomalyType = "multiple_ips"
)

// AnomalySeverity represents the severity of an anomaly
type AnomalySeverity string

const (
	SeverityLow      AnomalySeverity = "low"
	SeverityMedium   AnomalySeverity = "medium"
	SeverityHigh     AnomalySeverity = "high"
	SeverityCritical AnomalySeverity = "critical"
)

// AnomalyEvent represents a detected anomaly
type AnomalyEvent struct {
	Type        AnomalyType
	Severity    AnomalySeverity
	Identifier  string // User ID, IP, etc.
	IPAddress   string
	UserAgent   string
	Description string
	Metadata    map[string]interface{}
	DetectedAt  time.Time
	Score       float64 // Anomaly score (0-100)
}

// AnomalyDetectorConfig holds configuration for anomaly detection
type AnomalyDetectorConfig struct {
	RedisClient *redis.Client
	Logger      *zap.Logger

	// Rate-based detection
	RapidRequestThreshold   int           // Requests per window
	RapidRequestWindow      time.Duration // Time window

	// Failure rate detection
	FailureRateThreshold    float64       // Percentage (0-1)
	FailureRateWindow       time.Duration
	FailureRateMinRequests  int           // Minimum requests to trigger

	// Brute force detection
	BruteForceThreshold     int           // Failed attempts
	BruteForceWindow        time.Duration

	// Geographic anomaly
	EnableGeoCheck          bool
	GeoChangeThreshold      time.Duration // Time before geo change is suspicious

	// Device anomaly
	EnableDeviceCheck       bool
	DeviceChangeThreshold   time.Duration

	// Unusual hours (e.g., 2 AM - 5 AM)
	UnusualHoursStart       int // Hour (0-23)
	UnusualHoursEnd         int // Hour (0-23)

	// Multiple IPs detection
	MultipleIPsThreshold    int           // Different IPs in window
	MultipleIPsWindow       time.Duration
}

// AnomalyDetector detects suspicious behavior patterns
type AnomalyDetector struct {
	config AnomalyDetectorConfig
	redis  *redis.Client
	logger *zap.Logger
}

// NewAnomalyDetector creates a new anomaly detector
func NewAnomalyDetector(config AnomalyDetectorConfig) *AnomalyDetector {
	// Set defaults
	if config.RapidRequestThreshold == 0 {
		config.RapidRequestThreshold = 100
	}
	if config.RapidRequestWindow == 0 {
		config.RapidRequestWindow = 1 * time.Minute
	}
	if config.FailureRateThreshold == 0 {
		config.FailureRateThreshold = 0.5 // 50%
	}
	if config.FailureRateWindow == 0 {
		config.FailureRateWindow = 5 * time.Minute
	}
	if config.FailureRateMinRequests == 0 {
		config.FailureRateMinRequests = 10
	}
	if config.BruteForceThreshold == 0 {
		config.BruteForceThreshold = 10
	}
	if config.BruteForceWindow == 0 {
		config.BruteForceWindow = 10 * time.Minute
	}
	if config.GeoChangeThreshold == 0 {
		config.GeoChangeThreshold = 1 * time.Hour
	}
	if config.DeviceChangeThreshold == 0 {
		config.DeviceChangeThreshold = 1 * time.Hour
	}
	if config.UnusualHoursStart == 0 {
		config.UnusualHoursStart = 2
	}
	if config.UnusualHoursEnd == 0 {
		config.UnusualHoursEnd = 5
	}
	if config.MultipleIPsThreshold == 0 {
		config.MultipleIPsThreshold = 5
	}
	if config.MultipleIPsWindow == 0 {
		config.MultipleIPsWindow = 10 * time.Minute
	}

	return &AnomalyDetector{
		config: config,
		redis:  config.RedisClient,
		logger: config.Logger,
	}
}

// CheckRapidRequests detects rapid request patterns
func (d *AnomalyDetector) CheckRapidRequests(ctx context.Context, identifier string) (*AnomalyEvent, error) {
	key := fmt.Sprintf("anomaly:rapid:%s", identifier)

	// Increment counter
	count, err := d.redis.Incr(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to increment rapid request counter: %w", err)
	}

	// Set expiry on first request
	if count == 1 {
		d.redis.Expire(ctx, key, d.config.RapidRequestWindow)
	}

	// Check threshold
	if count > int64(d.config.RapidRequestThreshold) {
		severity := d.calculateSeverity(float64(count), float64(d.config.RapidRequestThreshold))
		return &AnomalyEvent{
			Type:        AnomalyTypeRapidRequests,
			Severity:    severity,
			Identifier:  identifier,
			Description: fmt.Sprintf("Rapid requests detected: %d requests in %v", count, d.config.RapidRequestWindow),
			DetectedAt:  time.Now(),
			Score:       (float64(count) / float64(d.config.RapidRequestThreshold)) * 100,
			Metadata: map[string]interface{}{
				"request_count": count,
				"threshold":     d.config.RapidRequestThreshold,
				"window":        d.config.RapidRequestWindow.String(),
			},
		}, nil
	}

	return nil, nil
}

// CheckFailureRate detects high failure rates
func (d *AnomalyDetector) CheckFailureRate(ctx context.Context, identifier string, isSuccess bool) (*AnomalyEvent, error) {
	totalKey := fmt.Sprintf("anomaly:failure:total:%s", identifier)
	failedKey := fmt.Sprintf("anomaly:failure:failed:%s", identifier)

	// Increment counters
	total, err := d.redis.Incr(ctx, totalKey).Result()
	if err != nil {
		return nil, err
	}

	if total == 1 {
		d.redis.Expire(ctx, totalKey, d.config.FailureRateWindow)
	}

	var failed int64
	if !isSuccess {
		failed, err = d.redis.Incr(ctx, failedKey).Result()
		if err != nil {
			return nil, err
		}
		if failed == 1 {
			d.redis.Expire(ctx, failedKey, d.config.FailureRateWindow)
		}
	} else {
		failed, _ = d.redis.Get(ctx, failedKey).Int64()
	}

	// Check if we have enough data
	if total < int64(d.config.FailureRateMinRequests) {
		return nil, nil
	}

	// Calculate failure rate
	failureRate := float64(failed) / float64(total)

	// Check threshold
	if failureRate > d.config.FailureRateThreshold {
		severity := d.calculateSeverity(failureRate, d.config.FailureRateThreshold)
		return &AnomalyEvent{
			Type:        AnomalyTypeHighFailureRate,
			Severity:    severity,
			Identifier:  identifier,
			Description: fmt.Sprintf("High failure rate: %.1f%% (%d/%d)", failureRate*100, failed, total),
			DetectedAt:  time.Now(),
			Score:       (failureRate / d.config.FailureRateThreshold) * 100,
			Metadata: map[string]interface{}{
				"failure_rate": failureRate,
				"failed":       failed,
				"total":        total,
				"threshold":    d.config.FailureRateThreshold,
			},
		}, nil
	}

	return nil, nil
}

// CheckBruteForce detects brute force attempts
func (d *AnomalyDetector) CheckBruteForce(ctx context.Context, identifier string) (*AnomalyEvent, error) {
	key := fmt.Sprintf("anomaly:bruteforce:%s", identifier)

	// Increment counter
	attempts, err := d.redis.Incr(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	if attempts == 1 {
		d.redis.Expire(ctx, key, d.config.BruteForceWindow)
	}

	// Check threshold
	if attempts > int64(d.config.BruteForceThreshold) {
		return &AnomalyEvent{
			Type:        AnomalyTypeBruteForce,
			Severity:    SeverityCritical,
			Identifier:  identifier,
			Description: fmt.Sprintf("Brute force attack detected: %d attempts in %v", attempts, d.config.BruteForceWindow),
			DetectedAt:  time.Now(),
			Score:       100,
			Metadata: map[string]interface{}{
				"attempts":  attempts,
				"threshold": d.config.BruteForceThreshold,
				"window":    d.config.BruteForceWindow.String(),
			},
		}, nil
	}

	return nil, nil
}

// CheckGeographicAnomaly detects geographic anomalies
func (d *AnomalyDetector) CheckGeographicAnomaly(ctx context.Context, identifier, currentLocation string) (*AnomalyEvent, error) {
	if !d.config.EnableGeoCheck {
		return nil, nil
	}

	key := fmt.Sprintf("anomaly:geo:%s", identifier)

	// Get last known location and timestamp
	lastData, err := d.redis.HGetAll(ctx, key).Result()
	if err != nil && err != redis.Nil {
		return nil, err
	}

	if len(lastData) == 0 {
		// First time seeing this identifier
		d.redis.HSet(ctx, key, map[string]interface{}{
			"location": currentLocation,
			"time":     time.Now().Unix(),
		})
		d.redis.Expire(ctx, key, 24*time.Hour)
		return nil, nil
	}

	lastLocation := lastData["location"]
	lastTimeUnix, _ := d.redis.HGet(ctx, key, "time").Int64()
	lastTime := time.Unix(lastTimeUnix, 0)

	// Check if location changed too quickly
	if lastLocation != currentLocation {
		timeSince := time.Since(lastTime)
		if timeSince < d.config.GeoChangeThreshold {
			// Update location
			d.redis.HSet(ctx, key, map[string]interface{}{
				"location": currentLocation,
				"time":     time.Now().Unix(),
			})

			return &AnomalyEvent{
				Type:        AnomalyTypeGeographicAnomaly,
				Severity:    SeverityHigh,
				Identifier:  identifier,
				Description: fmt.Sprintf("Geographic anomaly: location changed from %s to %s in %v", lastLocation, currentLocation, timeSince),
				DetectedAt:  time.Now(),
				Score:       80,
				Metadata: map[string]interface{}{
					"previous_location": lastLocation,
					"current_location":  currentLocation,
					"time_since_change": timeSince.String(),
				},
			}, nil
		}
	}

	// Update location
	d.redis.HSet(ctx, key, map[string]interface{}{
		"location": currentLocation,
		"time":     time.Now().Unix(),
	})

	return nil, nil
}

// CheckDeviceAnomaly detects device/user agent anomalies
func (d *AnomalyDetector) CheckDeviceAnomaly(ctx context.Context, identifier, currentUserAgent string) (*AnomalyEvent, error) {
	if !d.config.EnableDeviceCheck {
		return nil, nil
	}

	key := fmt.Sprintf("anomaly:device:%s", identifier)

	// Get last known user agent and timestamp
	lastData, err := d.redis.HGetAll(ctx, key).Result()
	if err != nil && err != redis.Nil {
		return nil, err
	}

	if len(lastData) == 0 {
		// First time seeing this identifier
		d.redis.HSet(ctx, key, map[string]interface{}{
			"user_agent": currentUserAgent,
			"time":       time.Now().Unix(),
		})
		d.redis.Expire(ctx, key, 24*time.Hour)
		return nil, nil
	}

	lastUserAgent := lastData["user_agent"]
	lastTimeUnix, _ := d.redis.HGet(ctx, key, "time").Int64()
	lastTime := time.Unix(lastTimeUnix, 0)

	// Check if device changed too quickly
	if lastUserAgent != currentUserAgent {
		timeSince := time.Since(lastTime)
		if timeSince < d.config.DeviceChangeThreshold {
			// Update device
			d.redis.HSet(ctx, key, map[string]interface{}{
				"user_agent": currentUserAgent,
				"time":       time.Now().Unix(),
			})

			return &AnomalyEvent{
				Type:        AnomalyTypeDeviceAnomaly,
				Severity:    SeverityMedium,
				Identifier:  identifier,
				Description: fmt.Sprintf("Device anomaly: user agent changed in %v", timeSince),
				DetectedAt:  time.Now(),
				Score:       60,
				Metadata: map[string]interface{}{
					"previous_user_agent": lastUserAgent,
					"current_user_agent":  currentUserAgent,
					"time_since_change":   timeSince.String(),
				},
			}, nil
		}
	}

	// Update device
	d.redis.HSet(ctx, key, map[string]interface{}{
		"user_agent": currentUserAgent,
		"time":       time.Now().Unix(),
	})

	return nil, nil
}

// CheckUnusualHours detects requests during unusual hours
func (d *AnomalyDetector) CheckUnusualHours(ctx context.Context, identifier string) (*AnomalyEvent, error) {
	currentHour := time.Now().Hour()

	// Check if current hour is in unusual range
	isUnusual := false
	if d.config.UnusualHoursStart < d.config.UnusualHoursEnd {
		isUnusual = currentHour >= d.config.UnusualHoursStart && currentHour < d.config.UnusualHoursEnd
	} else {
		// Handle wrap-around (e.g., 22:00 - 06:00)
		isUnusual = currentHour >= d.config.UnusualHoursStart || currentHour < d.config.UnusualHoursEnd
	}

	if isUnusual {
		return &AnomalyEvent{
			Type:        AnomalyTypeUnusualHours,
			Severity:    SeverityLow,
			Identifier:  identifier,
			Description: fmt.Sprintf("Request during unusual hours: %d:00", currentHour),
			DetectedAt:  time.Now(),
			Score:       30,
			Metadata: map[string]interface{}{
				"hour":         currentHour,
				"unusual_start": d.config.UnusualHoursStart,
				"unusual_end":   d.config.UnusualHoursEnd,
			},
		}, nil
	}

	return nil, nil
}

// CheckMultipleIPs detects multiple IPs for same identifier
func (d *AnomalyDetector) CheckMultipleIPs(ctx context.Context, identifier, ipAddress string) (*AnomalyEvent, error) {
	key := fmt.Sprintf("anomaly:ips:%s", identifier)

	// Add IP to set
	d.redis.SAdd(ctx, key, ipAddress)
	d.redis.Expire(ctx, key, d.config.MultipleIPsWindow)

	// Count unique IPs
	count, err := d.redis.SCard(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	if count > int64(d.config.MultipleIPsThreshold) {
		ips, _ := d.redis.SMembers(ctx, key).Result()
		return &AnomalyEvent{
			Type:        AnomalyTypeMultipleIPs,
			Severity:    SeverityHigh,
			Identifier:  identifier,
			IPAddress:   ipAddress,
			Description: fmt.Sprintf("Multiple IPs detected: %d IPs in %v", count, d.config.MultipleIPsWindow),
			DetectedAt:  time.Now(),
			Score:       75,
			Metadata: map[string]interface{}{
				"ip_count":  count,
				"threshold": d.config.MultipleIPsThreshold,
				"ips":       ips,
			},
		}, nil
	}

	return nil, nil
}

// calculateSeverity calculates severity based on how much threshold is exceeded
func (d *AnomalyDetector) calculateSeverity(actual, threshold float64) AnomalySeverity {
	ratio := actual / threshold

	switch {
	case ratio >= 5.0:
		return SeverityCritical
	case ratio >= 3.0:
		return SeverityHigh
	case ratio >= 2.0:
		return SeverityMedium
	default:
		return SeverityLow
	}
}

// ResetAnomalyCounters resets all anomaly counters for an identifier
func (d *AnomalyDetector) ResetAnomalyCounters(ctx context.Context, identifier string) error {
	keys := []string{
		fmt.Sprintf("anomaly:rapid:%s", identifier),
		fmt.Sprintf("anomaly:failure:total:%s", identifier),
		fmt.Sprintf("anomaly:failure:failed:%s", identifier),
		fmt.Sprintf("anomaly:bruteforce:%s", identifier),
		fmt.Sprintf("anomaly:geo:%s", identifier),
		fmt.Sprintf("anomaly:device:%s", identifier),
		fmt.Sprintf("anomaly:ips:%s", identifier),
	}

	pipe := d.redis.Pipeline()
	for _, key := range keys {
		pipe.Del(ctx, key)
	}
	_, err := pipe.Exec(ctx)

	return err
}
