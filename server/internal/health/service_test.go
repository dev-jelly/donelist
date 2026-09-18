package health

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestNewService(t *testing.T) {
	logger := zap.NewNop()
	service := NewService("1.0.0", logger)

	assert.NotNil(t, service)
	assert.Equal(t, "1.0.0", service.version)
	assert.NotNil(t, service.checkers)
	assert.False(t, service.startTime.IsZero())
}

func TestRegisterChecker(t *testing.T) {
	logger := zap.NewNop()
	service := NewService("1.0.0", logger)

	checker := NewDummyChecker("test", StatusHealthy)
	service.RegisterChecker(checker)

	assert.Len(t, service.checkers, 1)
}

func TestCheckHealth_AllHealthy(t *testing.T) {
	logger := zap.NewNop()
	service := NewService("1.0.0", logger)

	service.RegisterChecker(NewDummyChecker("test1", StatusHealthy))
	service.RegisterChecker(NewDummyChecker("test2", StatusHealthy))

	report := service.CheckHealth()

	assert.Equal(t, StatusHealthy, report.Status)
	assert.Equal(t, "1.0.0", report.Version)
	assert.Len(t, report.Components, 2)
	assert.NotZero(t, report.Uptime)
}

func TestCheckHealth_OneUnhealthy(t *testing.T) {
	logger := zap.NewNop()
	service := NewService("1.0.0", logger)

	service.RegisterChecker(NewDummyChecker("healthy", StatusHealthy))
	service.RegisterChecker(NewDummyChecker("unhealthy", StatusUnhealthy))

	report := service.CheckHealth()

	assert.Equal(t, StatusUnhealthy, report.Status)
	assert.Len(t, report.Components, 2)

	// Check component statuses
	assert.Equal(t, StatusHealthy, report.Components["healthy"].Status)
	assert.Equal(t, StatusUnhealthy, report.Components["unhealthy"].Status)
}

func TestCheckHealth_OneDegraded(t *testing.T) {
	logger := zap.NewNop()
	service := NewService("1.0.0", logger)

	service.RegisterChecker(NewDummyChecker("healthy", StatusHealthy))
	service.RegisterChecker(NewDummyChecker("degraded", StatusDegraded))

	report := service.CheckHealth()

	assert.Equal(t, StatusDegraded, report.Status)
	assert.Len(t, report.Components, 2)
}

func TestCheckHealth_DegradedAndUnhealthy(t *testing.T) {
	logger := zap.NewNop()
	service := NewService("1.0.0", logger)

	service.RegisterChecker(NewDummyChecker("degraded", StatusDegraded))
	service.RegisterChecker(NewDummyChecker("unhealthy", StatusUnhealthy))

	report := service.CheckHealth()

	// Unhealthy takes precedence over degraded
	assert.Equal(t, StatusUnhealthy, report.Status)
	assert.Len(t, report.Components, 2)
}

func TestCheckHealthWithBuildInfo(t *testing.T) {
	logger := zap.NewNop()
	service := NewService("1.0.0", logger)

	service.RegisterChecker(NewDummyChecker("test", StatusHealthy))

	report := service.CheckHealthWithBuildInfo()

	assert.Equal(t, StatusHealthy, report.Status)
	assert.Equal(t, "1.0.0", report.Version)
	assert.NotNil(t, report.BuildInfo)
	assert.NotZero(t, report.Uptime)
	assert.Len(t, report.Components, 1)
}

func TestCheckHealthSimple(t *testing.T) {
	logger := zap.NewNop()
	service := NewService("1.0.0", logger)

	service.RegisterChecker(NewDummyChecker("test", StatusHealthy))

	status, err := service.CheckHealthSimple()

	assert.NoError(t, err)
	assert.Equal(t, StatusHealthy, status)
}

func TestGetUptime(t *testing.T) {
	logger := zap.NewNop()
	service := NewService("1.0.0", logger)

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	uptime := service.GetUptime()

	assert.True(t, uptime > 10*time.Millisecond)
	assert.True(t, uptime < 1*time.Second)
}

func TestIsHealthy(t *testing.T) {
	logger := zap.NewNop()

	t.Run("all healthy", func(t *testing.T) {
		service := NewService("1.0.0", logger)
		service.RegisterChecker(NewDummyChecker("test", StatusHealthy))

		assert.True(t, service.IsHealthy())
	})

	t.Run("one unhealthy", func(t *testing.T) {
		service := NewService("1.0.0", logger)
		service.RegisterChecker(NewDummyChecker("test", StatusUnhealthy))

		assert.False(t, service.IsHealthy())
	})

	t.Run("one degraded", func(t *testing.T) {
		service := NewService("1.0.0", logger)
		service.RegisterChecker(NewDummyChecker("test", StatusDegraded))

		assert.False(t, service.IsHealthy())
	})
}

func TestCheckHealth_ConcurrentSafety(t *testing.T) {
	logger := zap.NewNop()
	service := NewService("1.0.0", logger)

	// Register multiple checkers
	for i := 0; i < 10; i++ {
		service.RegisterChecker(NewDummyChecker("test", StatusHealthy))
	}

	// Run health checks concurrently
	done := make(chan bool)
	for i := 0; i < 5; i++ {
		go func() {
			service.CheckHealth()
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 5; i++ {
		<-done
	}
}
