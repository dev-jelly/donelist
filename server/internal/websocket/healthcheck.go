package websocket

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

// HealthCheckConfig contains configuration for health checks
type HealthCheckConfig struct {
	// PingInterval is the interval between ping messages
	PingInterval time.Duration

	// PongTimeout is the timeout for receiving a pong response
	PongTimeout time.Duration

	// MaxMissedPings is the maximum number of missed pings before marking as unhealthy
	MaxMissedPings int

	// EnableAutoReconnect enables automatic reconnection on health check failure
	EnableAutoReconnect bool
}

// DefaultHealthCheckConfig returns default health check configuration
func DefaultHealthCheckConfig() HealthCheckConfig {
	return HealthCheckConfig{
		PingInterval:        30 * time.Second,
		PongTimeout:         10 * time.Second,
		MaxMissedPings:      3,
		EnableAutoReconnect: true,
	}
}

// HealthCheckStatus represents the health status of a connection
type HealthCheckStatus string

const (
	// HealthStatusHealthy indicates the connection is healthy
	HealthStatusHealthy HealthCheckStatus = "healthy"

	// HealthStatusDegraded indicates the connection is degraded but operational
	HealthStatusDegraded HealthCheckStatus = "degraded"

	// HealthStatusUnhealthy indicates the connection is unhealthy
	HealthStatusUnhealthy HealthCheckStatus = "unhealthy"

	// HealthStatusUnknown indicates the health status is unknown
	HealthStatusUnknown HealthCheckStatus = "unknown"
)

// HealthCheck represents a health check instance for a client
type HealthCheck struct {
	client             *Client
	config             HealthCheckConfig
	logger             *zap.Logger
	sessionManager     *SessionManager

	// State
	status             HealthCheckStatus
	lastPingSent       time.Time
	lastPongReceived   time.Time
	missedPingCount    int
	roundTripTime      time.Duration
	avgRoundTripTime   time.Duration

	// Control
	stopCh             chan struct{}
	stoppedCh          chan struct{}
	mu                 sync.RWMutex
}

// NewHealthCheck creates a new health check instance
func NewHealthCheck(client *Client, sessionManager *SessionManager, config HealthCheckConfig, logger *zap.Logger) *HealthCheck {
	if config.PingInterval == 0 {
		config = DefaultHealthCheckConfig()
	}

	return &HealthCheck{
		client:           client,
		config:           config,
		logger:           logger,
		sessionManager:   sessionManager,
		status:           HealthStatusHealthy,
		lastPongReceived: time.Now(),
		stopCh:           make(chan struct{}),
		stoppedCh:        make(chan struct{}),
	}
}

// Start starts the health check loop
func (hc *HealthCheck) Start(ctx context.Context) {
	go hc.healthCheckLoop(ctx)
}

// Stop stops the health check loop
func (hc *HealthCheck) Stop() {
	close(hc.stopCh)
	<-hc.stoppedCh
}

// healthCheckLoop runs the main health check loop
func (hc *HealthCheck) healthCheckLoop(ctx context.Context) {
	defer close(hc.stoppedCh)

	ticker := time.NewTicker(hc.config.PingInterval)
	defer ticker.Stop()

	hc.logger.Debug("Health check loop started",
		zap.String("user_id", hc.client.userID),
		zap.Duration("ping_interval", hc.config.PingInterval),
	)

	for {
		select {
		case <-ctx.Done():
			hc.logger.Debug("Health check loop stopped (context cancelled)",
				zap.String("user_id", hc.client.userID),
			)
			return

		case <-hc.stopCh:
			hc.logger.Debug("Health check loop stopped",
				zap.String("user_id", hc.client.userID),
			)
			return

		case <-ticker.C:
			hc.performHealthCheck()
		}
	}
}

// performHealthCheck performs a single health check
func (hc *HealthCheck) performHealthCheck() {
	hc.mu.Lock()

	// Check if last pong was received within timeout
	timeSinceLastPong := time.Since(hc.lastPongReceived)

	if timeSinceLastPong > hc.config.PongTimeout {
		hc.missedPingCount++

		hc.logger.Warn("Missed pong response",
			zap.String("user_id", hc.client.userID),
			zap.Int("missed_count", hc.missedPingCount),
			zap.Duration("time_since_last_pong", timeSinceLastPong),
		)

		// Update session manager
		if hc.sessionManager != nil {
			// Get client's session ID (we'll need to add this to Client struct)
			// For now, we'll skip this
		}

		// Update health status
		if hc.missedPingCount >= hc.config.MaxMissedPings {
			hc.status = HealthStatusUnhealthy
			hc.mu.Unlock()

			hc.logger.Error("Connection unhealthy, closing",
				zap.String("user_id", hc.client.userID),
				zap.Int("missed_pings", hc.missedPingCount),
			)

			// Close the connection
			hc.client.conn.Close()
			return
		} else if hc.missedPingCount >= hc.config.MaxMissedPings/2 {
			hc.status = HealthStatusDegraded
		}
	} else {
		// Connection is healthy
		if hc.status != HealthStatusHealthy {
			hc.logger.Info("Connection recovered",
				zap.String("user_id", hc.client.userID),
			)
		}
		hc.status = HealthStatusHealthy
		hc.missedPingCount = 0
	}

	// Send ping
	hc.lastPingSent = time.Now()
	hc.mu.Unlock()

	// Send ping message (this is already handled by client.writePump)
	// We just track the health status here
}

// OnPongReceived should be called when a pong is received
func (hc *HealthCheck) OnPongReceived() {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	now := time.Now()
	hc.lastPongReceived = now

	// Calculate round-trip time
	if !hc.lastPingSent.IsZero() {
		hc.roundTripTime = now.Sub(hc.lastPingSent)

		// Calculate moving average RTT
		if hc.avgRoundTripTime == 0 {
			hc.avgRoundTripTime = hc.roundTripTime
		} else {
			// Exponential moving average with alpha = 0.2
			hc.avgRoundTripTime = time.Duration(
				0.8*float64(hc.avgRoundTripTime) + 0.2*float64(hc.roundTripTime),
			)
		}

		hc.logger.Debug("Pong received",
			zap.String("user_id", hc.client.userID),
			zap.Duration("rtt", hc.roundTripTime),
			zap.Duration("avg_rtt", hc.avgRoundTripTime),
		)
	}

	// Reset missed ping count
	hc.missedPingCount = 0

	// Update health status
	if hc.status != HealthStatusHealthy {
		hc.logger.Info("Connection health restored",
			zap.String("user_id", hc.client.userID),
		)
		hc.status = HealthStatusHealthy
	}
}

// GetStatus returns the current health status
func (hc *HealthCheck) GetStatus() HealthCheckStatus {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	return hc.status
}

// GetMetrics returns health check metrics
func (hc *HealthCheck) GetMetrics() map[string]interface{} {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	return map[string]interface{}{
		"status":             string(hc.status),
		"missed_ping_count":  hc.missedPingCount,
		"last_ping_sent":     hc.lastPingSent.Unix(),
		"last_pong_received": hc.lastPongReceived.Unix(),
		"round_trip_time_ms": hc.roundTripTime.Milliseconds(),
		"avg_rtt_ms":         hc.avgRoundTripTime.Milliseconds(),
		"time_since_pong_s":  time.Since(hc.lastPongReceived).Seconds(),
	}
}

// HealthMonitor monitors all client health checks
type HealthMonitor struct {
	healthChecks   map[string]*HealthCheck // userID -> HealthCheck
	mu             sync.RWMutex
	logger         *zap.Logger
	sessionManager *SessionManager
	config         HealthCheckConfig
}

// NewHealthMonitor creates a new health monitor
func NewHealthMonitor(sessionManager *SessionManager, config HealthCheckConfig, logger *zap.Logger) *HealthMonitor {
	if config.PingInterval == 0 {
		config = DefaultHealthCheckConfig()
	}

	return &HealthMonitor{
		healthChecks:   make(map[string]*HealthCheck),
		logger:         logger,
		sessionManager: sessionManager,
		config:         config,
	}
}

// RegisterClient registers a client for health monitoring
func (hm *HealthMonitor) RegisterClient(client *Client, ctx context.Context) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	healthCheck := NewHealthCheck(client, hm.sessionManager, hm.config, hm.logger)
	hm.healthChecks[client.userID] = healthCheck

	healthCheck.Start(ctx)

	hm.logger.Debug("Client registered for health monitoring",
		zap.String("user_id", client.userID),
	)
}

// UnregisterClient unregisters a client from health monitoring
func (hm *HealthMonitor) UnregisterClient(userID string) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	if healthCheck, exists := hm.healthChecks[userID]; exists {
		healthCheck.Stop()
		delete(hm.healthChecks, userID)

		hm.logger.Debug("Client unregistered from health monitoring",
			zap.String("user_id", userID),
		)
	}
}

// OnPongReceived notifies the health monitor that a pong was received
func (hm *HealthMonitor) OnPongReceived(userID string) {
	hm.mu.RLock()
	healthCheck, exists := hm.healthChecks[userID]
	hm.mu.RUnlock()

	if exists {
		healthCheck.OnPongReceived()
	}
}

// GetClientStatus returns the health status of a client
func (hm *HealthMonitor) GetClientStatus(userID string) HealthCheckStatus {
	hm.mu.RLock()
	healthCheck, exists := hm.healthChecks[userID]
	hm.mu.RUnlock()

	if !exists {
		return HealthStatusUnknown
	}

	return healthCheck.GetStatus()
}

// GetClientMetrics returns health metrics for a client
func (hm *HealthMonitor) GetClientMetrics(userID string) map[string]interface{} {
	hm.mu.RLock()
	healthCheck, exists := hm.healthChecks[userID]
	hm.mu.RUnlock()

	if !exists {
		return map[string]interface{}{
			"status": string(HealthStatusUnknown),
			"error":  "client not found",
		}
	}

	return healthCheck.GetMetrics()
}

// GetAllMetrics returns health metrics for all clients
func (hm *HealthMonitor) GetAllMetrics() map[string]interface{} {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	metrics := make(map[string]interface{})
	statusCount := make(map[HealthCheckStatus]int)

	for userID, healthCheck := range hm.healthChecks {
		metrics[userID] = healthCheck.GetMetrics()
		statusCount[healthCheck.GetStatus()]++
	}

	return map[string]interface{}{
		"total_clients":   len(hm.healthChecks),
		"healthy":         statusCount[HealthStatusHealthy],
		"degraded":        statusCount[HealthStatusDegraded],
		"unhealthy":       statusCount[HealthStatusUnhealthy],
		"unknown":         statusCount[HealthStatusUnknown],
		"client_metrics":  metrics,
	}
}

// StartCleanupWorker starts a background worker to clean up disconnected clients
func (hm *HealthMonitor) StartCleanupWorker(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hm.cleanupUnhealthyClients()
		}
	}
}

func (hm *HealthMonitor) cleanupUnhealthyClients() {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	unhealthyClients := make([]string, 0)

	for userID, healthCheck := range hm.healthChecks {
		if healthCheck.GetStatus() == HealthStatusUnhealthy {
			unhealthyClients = append(unhealthyClients, userID)
		}
	}

	for _, userID := range unhealthyClients {
		if healthCheck, exists := hm.healthChecks[userID]; exists {
			healthCheck.Stop()
			delete(hm.healthChecks, userID)

			hm.logger.Info("Cleaned up unhealthy client",
				zap.String("user_id", userID),
			)
		}
	}

	if len(unhealthyClients) > 0 {
		hm.logger.Info("Cleaned up unhealthy clients",
			zap.Int("count", len(unhealthyClients)),
		)
	}
}
