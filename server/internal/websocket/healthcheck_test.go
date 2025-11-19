package websocket

import (
	"context"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestHealthCheckConfig(t *testing.T) {
	t.Run("default health check config", func(t *testing.T) {
		config := DefaultHealthCheckConfig()

		assert.Equal(t, 30*time.Second, config.PingInterval)
		assert.Equal(t, 10*time.Second, config.PongTimeout)
		assert.Equal(t, 3, config.MaxMissedPings)
		assert.True(t, config.EnableAutoReconnect)
	})
}

func TestHealthCheckStatus(t *testing.T) {
	t.Run("health status constants", func(t *testing.T) {
		statuses := []HealthCheckStatus{
			HealthStatusHealthy,
			HealthStatusDegraded,
			HealthStatusUnhealthy,
			HealthStatusUnknown,
		}

		for _, status := range statuses {
			assert.NotEmpty(t, string(status))
		}
	})
}

func TestHealthCheck(t *testing.T) {
	logger := zap.NewNop()

	t.Run("create health check", func(t *testing.T) {
		hub := NewHub(logger)
		conn := &websocket.Conn{} // Mock connection
		client := NewClient(conn, hub, "user-123", "testuser", logger)

		config := HealthCheckConfig{
			PingInterval:   1 * time.Second,
			PongTimeout:    500 * time.Millisecond,
			MaxMissedPings: 3,
		}

		hc := NewHealthCheck(client, hub.sessionManager, config, logger)

		assert.NotNil(t, hc)
		assert.Equal(t, HealthStatusHealthy, hc.GetStatus())
	})

	t.Run("pong received updates status", func(t *testing.T) {
		hub := NewHub(logger)
		conn := &websocket.Conn{}
		client := NewClient(conn, hub, "user-123", "testuser", logger)

		config := HealthCheckConfig{
			PingInterval:   100 * time.Millisecond,
			PongTimeout:    50 * time.Millisecond,
			MaxMissedPings: 2,
		}

		hc := NewHealthCheck(client, hub.sessionManager, config, logger)

		// Simulate pong received
		hc.OnPongReceived()

		metrics := hc.GetMetrics()
		assert.Equal(t, "healthy", metrics["status"])
		assert.Equal(t, 0, metrics["missed_ping_count"])
	})

	t.Run("calculate round trip time", func(t *testing.T) {
		hub := NewHub(logger)
		conn := &websocket.Conn{}
		client := NewClient(conn, hub, "user-123", "testuser", logger)

		config := DefaultHealthCheckConfig()
		hc := NewHealthCheck(client, hub.sessionManager, config, logger)

		// Simulate ping sent
		hc.mu.Lock()
		hc.lastPingSent = time.Now().Add(-50 * time.Millisecond)
		hc.mu.Unlock()

		// Simulate pong received
		hc.OnPongReceived()

		metrics := hc.GetMetrics()
		rtt := metrics["round_trip_time_ms"].(int64)
		assert.Greater(t, rtt, int64(0))
		assert.Less(t, rtt, int64(1000))
	})

	t.Run("missed pings degrade health", func(t *testing.T) {
		hub := NewHub(logger)
		conn := &websocket.Conn{}
		client := NewClient(conn, hub, "user-123", "testuser", logger)

		config := HealthCheckConfig{
			PingInterval:   100 * time.Millisecond,
			PongTimeout:    50 * time.Millisecond,
			MaxMissedPings: 3,
		}

		hc := NewHealthCheck(client, hub.sessionManager, config, logger)

		// Set last pong to old time
		hc.mu.Lock()
		hc.lastPongReceived = time.Now().Add(-200 * time.Millisecond)
		hc.mu.Unlock()

		// Perform health check (will detect missed pong)
		hc.performHealthCheck()

		// Should mark as degraded after one missed ping
		assert.Equal(t, 1, hc.missedPingCount)

		// Simulate another missed ping
		hc.performHealthCheck()

		// Should become degraded (halfway to max)
		status := hc.GetStatus()
		assert.Equal(t, HealthStatusDegraded, status)
	})

	t.Run("recovery from degraded state", func(t *testing.T) {
		hub := NewHub(logger)
		conn := &websocket.Conn{}
		client := NewClient(conn, hub, "user-123", "testuser", logger)

		config := DefaultHealthCheckConfig()
		hc := NewHealthCheck(client, hub.sessionManager, config, logger)

		// Mark as degraded
		hc.mu.Lock()
		hc.status = HealthStatusDegraded
		hc.missedPingCount = 1
		hc.mu.Unlock()

		// Receive pong
		hc.OnPongReceived()

		// Should recover to healthy
		assert.Equal(t, HealthStatusHealthy, hc.GetStatus())
		assert.Equal(t, 0, hc.missedPingCount)
	})

	t.Run("health check metrics", func(t *testing.T) {
		hub := NewHub(logger)
		conn := &websocket.Conn{}
		client := NewClient(conn, hub, "user-123", "testuser", logger)

		config := DefaultHealthCheckConfig()
		hc := NewHealthCheck(client, hub.sessionManager, config, logger)

		metrics := hc.GetMetrics()

		assert.Contains(t, metrics, "status")
		assert.Contains(t, metrics, "missed_ping_count")
		assert.Contains(t, metrics, "last_ping_sent")
		assert.Contains(t, metrics, "last_pong_received")
		assert.Contains(t, metrics, "round_trip_time_ms")
		assert.Contains(t, metrics, "avg_rtt_ms")
		assert.Contains(t, metrics, "time_since_pong_s")
	})
}

func TestHealthMonitor(t *testing.T) {
	logger := zap.NewNop()

	t.Run("create health monitor", func(t *testing.T) {
		sm := NewSessionManager(logger)
		config := DefaultHealthCheckConfig()

		hm := NewHealthMonitor(sm, config, logger)

		assert.NotNil(t, hm)
	})

	t.Run("register and unregister client", func(t *testing.T) {
		hub := NewHub(logger)
		config := HealthCheckConfig{
			PingInterval:   100 * time.Millisecond,
			PongTimeout:    50 * time.Millisecond,
			MaxMissedPings: 3,
		}

		hm := NewHealthMonitor(hub.sessionManager, config, logger)

		conn := &websocket.Conn{}
		client := NewClient(conn, hub, "user-123", "testuser", logger)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Register client
		hm.RegisterClient(client, ctx)

		// Check status
		status := hm.GetClientStatus("user-123")
		assert.Equal(t, HealthStatusHealthy, status)

		// Unregister client
		hm.UnregisterClient("user-123")

		// Status should be unknown
		status = hm.GetClientStatus("user-123")
		assert.Equal(t, HealthStatusUnknown, status)
	})

	t.Run("handle pong received", func(t *testing.T) {
		hub := NewHub(logger)
		config := DefaultHealthCheckConfig()

		hm := NewHealthMonitor(hub.sessionManager, config, logger)

		conn := &websocket.Conn{}
		client := NewClient(conn, hub, "user-123", "testuser", logger)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		hm.RegisterClient(client, ctx)

		// Notify pong received
		hm.OnPongReceived("user-123")

		// Get metrics
		metrics := hm.GetClientMetrics("user-123")
		assert.Equal(t, "healthy", metrics["status"])
	})

	t.Run("get all metrics", func(t *testing.T) {
		hub := NewHub(logger)
		config := DefaultHealthCheckConfig()

		hm := NewHealthMonitor(hub.sessionManager, config, logger)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Register multiple clients
		for i := 1; i <= 3; i++ {
			conn := &websocket.Conn{}
			userID := string(rune('0' + i))
			client := NewClient(conn, hub, "user-"+userID, "user"+userID, logger)
			hm.RegisterClient(client, ctx)
		}

		metrics := hm.GetAllMetrics()

		assert.Equal(t, 3, metrics["total_clients"])
		assert.Equal(t, 3, metrics["healthy"])
		assert.Contains(t, metrics, "client_metrics")
	})

	t.Run("get metrics for non-existent client", func(t *testing.T) {
		hub := NewHub(logger)
		config := DefaultHealthCheckConfig()

		hm := NewHealthMonitor(hub.sessionManager, config, logger)

		metrics := hm.GetClientMetrics("non-existent-user")

		assert.Equal(t, "unknown", metrics["status"])
		assert.Contains(t, metrics, "error")
	})

	t.Run("cleanup unhealthy clients", func(t *testing.T) {
		hub := NewHub(logger)
		config := HealthCheckConfig{
			PingInterval:   50 * time.Millisecond,
			PongTimeout:    25 * time.Millisecond,
			MaxMissedPings: 1,
		}

		hm := NewHealthMonitor(hub.sessionManager, config, logger)

		conn := &websocket.Conn{}
		client := NewClient(conn, hub, "user-123", "testuser", logger)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		hm.RegisterClient(client, ctx)

		// Manually mark as unhealthy
		hm.mu.Lock()
		if hc, exists := hm.healthChecks["user-123"]; exists {
			hc.mu.Lock()
			hc.status = HealthStatusUnhealthy
			hc.mu.Unlock()
		}
		hm.mu.Unlock()

		// Run cleanup
		hm.cleanupUnhealthyClients()

		// Client should be removed
		status := hm.GetClientStatus("user-123")
		assert.Equal(t, HealthStatusUnknown, status)
	})
}

func TestHealthCheckIntegration(t *testing.T) {
	logger := zap.NewNop()

	t.Run("full lifecycle test", func(t *testing.T) {
		hub := NewHub(logger)
		config := HealthCheckConfig{
			PingInterval:   200 * time.Millisecond,
			PongTimeout:    100 * time.Millisecond,
			MaxMissedPings: 2,
		}

		hm := NewHealthMonitor(hub.sessionManager, config, logger)

		conn := &websocket.Conn{}
		client := NewClient(conn, hub, "user-123", "testuser", logger)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// Register client
		hm.RegisterClient(client, ctx)

		// Verify healthy initially
		status := hm.GetClientStatus("user-123")
		assert.Equal(t, HealthStatusHealthy, status)

		// Simulate pong responses
		for i := 0; i < 3; i++ {
			time.Sleep(150 * time.Millisecond)
			hm.OnPongReceived("user-123")
		}

		// Should still be healthy
		status = hm.GetClientStatus("user-123")
		assert.Equal(t, HealthStatusHealthy, status)

		// Get final metrics
		metrics := hm.GetClientMetrics("user-123")
		assert.Equal(t, "healthy", metrics["status"])
		assert.Equal(t, 0, metrics["missed_ping_count"])

		// Cleanup
		hm.UnregisterClient("user-123")
	})

	t.Run("average RTT calculation", func(t *testing.T) {
		hub := NewHub(logger)
		conn := &websocket.Conn{}
		client := NewClient(conn, hub, "user-123", "testuser", logger)

		config := DefaultHealthCheckConfig()
		hc := NewHealthCheck(client, hub.sessionManager, config, logger)

		// Simulate multiple pings with different RTTs
		delays := []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 30 * time.Millisecond}

		for _, delay := range delays {
			hc.mu.Lock()
			hc.lastPingSent = time.Now().Add(-delay)
			hc.mu.Unlock()

			hc.OnPongReceived()
		}

		metrics := hc.GetMetrics()
		avgRTT := metrics["avg_rtt_ms"].(int64)

		// Average RTT should be between min and max delays
		assert.Greater(t, avgRTT, int64(5))
		assert.Less(t, avgRTT, int64(50))
	})
}

func TestHealthCheckEdgeCases(t *testing.T) {
	logger := zap.NewNop()

	t.Run("pong received before ping sent", func(t *testing.T) {
		hub := NewHub(logger)
		conn := &websocket.Conn{}
		client := NewClient(conn, hub, "user-123", "testuser", logger)

		config := DefaultHealthCheckConfig()
		hc := NewHealthCheck(client, hub.sessionManager, config, logger)

		// Receive pong without sending ping
		hc.OnPongReceived()

		// Should not crash and should be healthy
		assert.Equal(t, HealthStatusHealthy, hc.GetStatus())
	})

	t.Run("zero config uses defaults", func(t *testing.T) {
		hub := NewHub(logger)
		conn := &websocket.Conn{}
		client := NewClient(conn, hub, "user-123", "testuser", logger)

		config := HealthCheckConfig{}
		hc := NewHealthCheck(client, hub.sessionManager, config, logger)

		assert.NotNil(t, hc)
		assert.Equal(t, DefaultHealthCheckConfig().PingInterval, hc.config.PingInterval)
	})

	t.Run("concurrent pong receives", func(t *testing.T) {
		hub := NewHub(logger)
		conn := &websocket.Conn{}
		client := NewClient(conn, hub, "user-123", "testuser", logger)

		config := DefaultHealthCheckConfig()
		hc := NewHealthCheck(client, hub.sessionManager, config, logger)

		// Simulate concurrent pong receives
		done := make(chan bool, 10)
		for i := 0; i < 10; i++ {
			go func() {
				hc.OnPongReceived()
				done <- true
			}()
		}

		// Wait for all goroutines
		for i := 0; i < 10; i++ {
			<-done
		}

		// Should remain healthy
		assert.Equal(t, HealthStatusHealthy, hc.GetStatus())
	})
}
