package security

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupTestDetector(t *testing.T) (*AnomalyDetector, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	logger := zap.NewNop()

	config := AnomalyDetectorConfig{
		RedisClient:             redisClient,
		Logger:                  logger,
		RapidRequestThreshold:   10,
		RapidRequestWindow:      1 * time.Minute,
		FailureRateThreshold:    0.5,
		FailureRateWindow:       5 * time.Minute,
		FailureRateMinRequests:  5,
		BruteForceThreshold:     5,
		BruteForceWindow:        10 * time.Minute,
		EnableGeoCheck:          true,
		GeoChangeThreshold:      1 * time.Hour,
		EnableDeviceCheck:       true,
		DeviceChangeThreshold:   1 * time.Hour,
		UnusualHoursStart:       2,
		UnusualHoursEnd:         5,
		MultipleIPsThreshold:    3,
		MultipleIPsWindow:       10 * time.Minute,
	}

	detector := NewAnomalyDetector(config)

	return detector, mr
}

func TestCheckRapidRequests(t *testing.T) {
	detector, mr := setupTestDetector(t)
	defer mr.Close()

	ctx := context.Background()
	identifier := "user123"

	// First 10 requests should be fine
	for i := 0; i < 10; i++ {
		anomaly, err := detector.CheckRapidRequests(ctx, identifier)
		require.NoError(t, err)
		assert.Nil(t, anomaly)
	}

	// 11th request should trigger anomaly
	anomaly, err := detector.CheckRapidRequests(ctx, identifier)
	require.NoError(t, err)
	require.NotNil(t, anomaly)

	assert.Equal(t, AnomalyTypeRapidRequests, anomaly.Type)
	assert.Equal(t, identifier, anomaly.Identifier)
	assert.Greater(t, anomaly.Score, 100.0)
}

func TestCheckFailureRate(t *testing.T) {
	detector, mr := setupTestDetector(t)
	defer mr.Close()

	ctx := context.Background()
	identifier := "user123"

	// 3 successful, 7 failed = 70% failure rate
	for i := 0; i < 3; i++ {
		anomaly, err := detector.CheckFailureRate(ctx, identifier, true)
		require.NoError(t, err)
		assert.Nil(t, anomaly)
	}

	for i := 0; i < 6; i++ {
		anomaly, err := detector.CheckFailureRate(ctx, identifier, false)
		require.NoError(t, err)
		assert.Nil(t, anomaly) // Not enough requests yet
	}

	// 10th request should trigger (70% > 50% threshold)
	anomaly, err := detector.CheckFailureRate(ctx, identifier, false)
	require.NoError(t, err)
	require.NotNil(t, anomaly)

	assert.Equal(t, AnomalyTypeHighFailureRate, anomaly.Type)
	assert.Equal(t, identifier, anomaly.Identifier)
}

func TestCheckBruteForce(t *testing.T) {
	detector, mr := setupTestDetector(t)
	defer mr.Close()

	ctx := context.Background()
	identifier := "user123"

	// First 5 attempts should be fine
	for i := 0; i < 5; i++ {
		anomaly, err := detector.CheckBruteForce(ctx, identifier)
		require.NoError(t, err)
		assert.Nil(t, anomaly)
	}

	// 6th attempt should trigger
	anomaly, err := detector.CheckBruteForce(ctx, identifier)
	require.NoError(t, err)
	require.NotNil(t, anomaly)

	assert.Equal(t, AnomalyTypeBruteForce, anomaly.Type)
	assert.Equal(t, SeverityCritical, anomaly.Severity)
	assert.Equal(t, 100.0, anomaly.Score)
}

func TestCheckGeographicAnomaly(t *testing.T) {
	detector, mr := setupTestDetector(t)
	defer mr.Close()

	ctx := context.Background()
	identifier := "user123"

	// First location should be recorded
	anomaly, err := detector.CheckGeographicAnomaly(ctx, identifier, "US")
	require.NoError(t, err)
	assert.Nil(t, anomaly)

	// Same location should be fine
	anomaly, err = detector.CheckGeographicAnomaly(ctx, identifier, "US")
	require.NoError(t, err)
	assert.Nil(t, anomaly)

	// Different location too quickly should trigger
	anomaly, err = detector.CheckGeographicAnomaly(ctx, identifier, "CN")
	require.NoError(t, err)
	require.NotNil(t, anomaly)

	assert.Equal(t, AnomalyTypeGeographicAnomaly, anomaly.Type)
	assert.Equal(t, SeverityHigh, anomaly.Severity)
}

func TestCheckDeviceAnomaly(t *testing.T) {
	detector, mr := setupTestDetector(t)
	defer mr.Close()

	ctx := context.Background()
	identifier := "user123"

	userAgent1 := "Mozilla/5.0 (Windows NT 10.0; Win64; x64)"
	userAgent2 := "Mozilla/5.0 (iPhone; CPU iPhone OS 14_0 like Mac OS X)"

	// First device should be recorded
	anomaly, err := detector.CheckDeviceAnomaly(ctx, identifier, userAgent1)
	require.NoError(t, err)
	assert.Nil(t, anomaly)

	// Same device should be fine
	anomaly, err = detector.CheckDeviceAnomaly(ctx, identifier, userAgent1)
	require.NoError(t, err)
	assert.Nil(t, anomaly)

	// Different device too quickly should trigger
	anomaly, err = detector.CheckDeviceAnomaly(ctx, identifier, userAgent2)
	require.NoError(t, err)
	require.NotNil(t, anomaly)

	assert.Equal(t, AnomalyTypeDeviceAnomaly, anomaly.Type)
	assert.Equal(t, SeverityMedium, anomaly.Severity)
}

func TestCheckMultipleIPs(t *testing.T) {
	detector, mr := setupTestDetector(t)
	defer mr.Close()

	ctx := context.Background()
	identifier := "user123"

	// Add 3 different IPs
	ips := []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"}
	for _, ip := range ips {
		anomaly, err := detector.CheckMultipleIPs(ctx, identifier, ip)
		require.NoError(t, err)
		assert.Nil(t, anomaly)
	}

	// 4th IP should trigger (threshold is 3)
	anomaly, err := detector.CheckMultipleIPs(ctx, identifier, "4.4.4.4")
	require.NoError(t, err)
	require.NotNil(t, anomaly)

	assert.Equal(t, AnomalyTypeMultipleIPs, anomaly.Type)
	assert.Equal(t, SeverityHigh, anomaly.Severity)
}

func TestCheckUnusualHours(t *testing.T) {
	detector, mr := setupTestDetector(t)
	defer mr.Close()

	ctx := context.Background()
	identifier := "user123"

	// Mock time is difficult, so we just test the logic
	anomaly, err := detector.CheckUnusualHours(ctx, identifier)
	require.NoError(t, err)

	// Anomaly may or may not be detected depending on current hour
	currentHour := time.Now().Hour()
	isUnusualHour := currentHour >= 2 && currentHour < 5

	if isUnusualHour {
		require.NotNil(t, anomaly)
		assert.Equal(t, AnomalyTypeUnusualHours, anomaly.Type)
		assert.Equal(t, SeverityLow, anomaly.Severity)
	} else {
		assert.Nil(t, anomaly)
	}
}

func TestCalculateSeverity(t *testing.T) {
	detector, mr := setupTestDetector(t)
	defer mr.Close()

	tests := []struct {
		name      string
		actual    float64
		threshold float64
		expected  AnomalySeverity
	}{
		{"Low", 11.0, 10.0, SeverityLow},
		{"Medium", 21.0, 10.0, SeverityMedium},
		{"High", 31.0, 10.0, SeverityHigh},
		{"Critical", 51.0, 10.0, SeverityCritical},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			severity := detector.calculateSeverity(tt.actual, tt.threshold)
			assert.Equal(t, tt.expected, severity)
		})
	}
}

func TestResetAnomalyCounters(t *testing.T) {
	detector, mr := setupTestDetector(t)
	defer mr.Close()

	ctx := context.Background()
	identifier := "user123"

	// Trigger some anomalies
	for i := 0; i < 15; i++ {
		detector.CheckRapidRequests(ctx, identifier)
	}

	// Verify anomaly is detected
	anomaly, err := detector.CheckRapidRequests(ctx, identifier)
	require.NoError(t, err)
	require.NotNil(t, anomaly)

	// Reset counters
	err = detector.ResetAnomalyCounters(ctx, identifier)
	require.NoError(t, err)

	// Verify anomaly is no longer detected
	anomaly, err = detector.CheckRapidRequests(ctx, identifier)
	require.NoError(t, err)
	assert.Nil(t, anomaly)
}

func BenchmarkCheckRapidRequests(b *testing.B) {
	detector, mr := setupTestDetector(&testing.T{})
	defer mr.Close()

	ctx := context.Background()
	identifier := "user123"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detector.CheckRapidRequests(ctx, identifier)
	}
}

func BenchmarkCheckFailureRate(b *testing.B) {
	detector, mr := setupTestDetector(&testing.T{})
	defer mr.Close()

	ctx := context.Background()
	identifier := "user123"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detector.CheckFailureRate(ctx, identifier, i%2 == 0)
	}
}
