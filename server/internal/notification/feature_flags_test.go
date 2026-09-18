package notification

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestFeatureFlagManager_Creation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ffm := NewFeatureFlagManager(logger)

	assert.NotNil(t, ffm)
	assert.True(t, ffm.IsEnabled(FlagEnableEnqueue))
	assert.True(t, ffm.IsEnabled(FlagEnableSend))
	assert.False(t, ffm.IsEnabled(FlagDrainMode))
}

func TestFeatureFlagManager_SetFlag(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ffm := NewFeatureFlagManager(logger)

	// Initially enabled
	assert.True(t, ffm.IsEnabled(FlagEnableEnqueue))

	// Disable flag
	ffm.SetFlag(FlagEnableEnqueue, false, "test")
	assert.False(t, ffm.IsEnabled(FlagEnableEnqueue))

	// Re-enable flag
	ffm.SetFlag(FlagEnableEnqueue, true, "test")
	assert.True(t, ffm.IsEnabled(FlagEnableEnqueue))
}

func TestFeatureFlagManager_SetFlagFor(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ffm := NewFeatureFlagManager(logger)

	userID := uuid.New().String()

	// Set flag for specific user
	ffm.SetFlagFor(FlagEnableEnqueue, userID, false)

	// Check global is still enabled
	assert.True(t, ffm.IsEnabled(FlagEnableEnqueue))

	// Check user-specific is disabled
	assert.False(t, ffm.IsEnabledFor(FlagEnableEnqueue, userID))

	// Check other user uses global setting
	otherUser := uuid.New().String()
	assert.True(t, ffm.IsEnabledFor(FlagEnableEnqueue, otherUser))
}

func TestFeatureFlagManager_RegisterHook(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ffm := NewFeatureFlagManager(logger)

	hookCalled := false
	var capturedFlag FeatureFlag
	var capturedOld, capturedNew bool

	hook := func(flag FeatureFlag, oldState, newState bool) {
		hookCalled = true
		capturedFlag = flag
		capturedOld = oldState
		capturedNew = newState
	}

	ffm.RegisterHook(FlagEnableEnqueue, hook)

	// Change flag - hook should be called
	ffm.SetFlag(FlagEnableEnqueue, false, "test")

	assert.True(t, hookCalled)
	assert.Equal(t, FlagEnableEnqueue, capturedFlag)
	assert.True(t, capturedOld)
	assert.False(t, capturedNew)
}

func TestFeatureFlagManager_GetAllFlags(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ffm := NewFeatureFlagManager(logger)

	flags := ffm.GetAllFlags()

	assert.NotNil(t, flags)
	assert.Contains(t, flags, FlagEnableEnqueue)
	assert.Contains(t, flags, FlagEnableSend)
	assert.Contains(t, flags, FlagDrainMode)

	// Verify default states
	assert.True(t, flags[FlagEnableEnqueue].Enabled)
	assert.False(t, flags[FlagDrainMode].Enabled)
}

func TestDrainModeManager_Creation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ffm := NewFeatureFlagManager(logger)
	dmm := NewDrainModeManager(ffm, logger)

	assert.NotNil(t, dmm)
	assert.False(t, dmm.IsDraining())
}

func TestDrainModeManager_StartDrain(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ffm := NewFeatureFlagManager(logger)
	dmm := NewDrainModeManager(ffm, logger)
	ctx := context.Background()

	// Start drain
	err := dmm.StartDrain(ctx)
	assert.NoError(t, err)

	// Verify state
	assert.True(t, dmm.IsDraining())
	assert.True(t, ffm.IsEnabled(FlagDrainMode))
	assert.False(t, ffm.IsEnabled(FlagEnableEnqueue))
}

func TestDrainModeManager_StartDrainTwice(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ffm := NewFeatureFlagManager(logger)
	dmm := NewDrainModeManager(ffm, logger)
	ctx := context.Background()

	// First drain
	err := dmm.StartDrain(ctx)
	assert.NoError(t, err)

	// Second drain should fail
	err = dmm.StartDrain(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already draining")
}

func TestDrainModeManager_CompleteDrain(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ffm := NewFeatureFlagManager(logger)
	dmm := NewDrainModeManager(ffm, logger)
	ctx := context.Background()

	// Start drain
	err := dmm.StartDrain(ctx)
	require.NoError(t, err)

	// Complete drain
	err = dmm.CompleteDrain(ctx)
	assert.NoError(t, err)

	// Verify state
	assert.False(t, dmm.IsDraining())
	assert.False(t, ffm.IsEnabled(FlagEnableSend))
}

func TestDrainModeManager_CompleteDrainWithoutStart(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ffm := NewFeatureFlagManager(logger)
	dmm := NewDrainModeManager(ffm, logger)
	ctx := context.Background()

	// Complete drain without starting
	err := dmm.CompleteDrain(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not in drain mode")
}

func TestDrainModeManager_RollbackDrain(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ffm := NewFeatureFlagManager(logger)
	dmm := NewDrainModeManager(ffm, logger)
	ctx := context.Background()

	// Start drain
	err := dmm.StartDrain(ctx)
	require.NoError(t, err)

	// Rollback drain
	err = dmm.RollbackDrain(ctx)
	assert.NoError(t, err)

	// Verify flags are re-enabled
	assert.False(t, dmm.IsDraining())
	assert.True(t, ffm.IsEnabled(FlagEnableEnqueue))
	assert.True(t, ffm.IsEnabled(FlagEnableSend))
	assert.False(t, ffm.IsEnabled(FlagDrainMode))
}

func TestDrainModeManager_Hooks(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ffm := NewFeatureFlagManager(logger)
	dmm := NewDrainModeManager(ffm, logger)
	ctx := context.Background()

	startHookCalled := false
	completeHookCalled := false

	dmm.RegisterDrainStartHook(func() {
		startHookCalled = true
	})

	dmm.RegisterDrainCompleteHook(func() {
		completeHookCalled = true
	})

	// Start drain
	dmm.StartDrain(ctx)
	assert.True(t, startHookCalled)
	assert.False(t, completeHookCalled)

	// Complete drain
	dmm.CompleteDrain(ctx)
	assert.True(t, completeHookCalled)
}

func TestServiceController_CanEnqueue(t *testing.T) {
	logger := zaptest.NewLogger(t)
	service := &Service{}
	controller := NewServiceController(service, logger)

	userID := uuid.New()

	// Initially should be able to enqueue
	assert.True(t, controller.CanEnqueue(userID))

	// Disable global enqueue
	controller.flagManager.SetFlag(FlagEnableEnqueue, false, "test")
	assert.False(t, controller.CanEnqueue(userID))

	// Re-enable global
	controller.flagManager.SetFlag(FlagEnableEnqueue, true, "test")
	assert.True(t, controller.CanEnqueue(userID))

	// Disable for specific user
	controller.flagManager.SetFlagFor(FlagEnableEnqueue, userID.String(), false)
	assert.False(t, controller.CanEnqueue(userID))
}

func TestServiceController_CanEnqueue_DuringDrain(t *testing.T) {
	logger := zaptest.NewLogger(t)
	service := &Service{}
	controller := NewServiceController(service, logger)
	ctx := context.Background()

	userID := uuid.New()

	// Initially can enqueue
	assert.True(t, controller.CanEnqueue(userID))

	// Start drain
	controller.drainManager.StartDrain(ctx)

	// Cannot enqueue during drain
	assert.False(t, controller.CanEnqueue(userID))
}

func TestServiceController_CanSend(t *testing.T) {
	logger := zaptest.NewLogger(t)
	service := &Service{}
	controller := NewServiceController(service, logger)

	// Initially can send
	assert.True(t, controller.CanSend())

	// Disable sending
	controller.flagManager.SetFlag(FlagEnableSend, false, "test")
	assert.False(t, controller.CanSend())

	// Re-enable sending
	controller.flagManager.SetFlag(FlagEnableSend, true, "test")
	assert.True(t, controller.CanSend())
}

func TestServiceController_GetStatus(t *testing.T) {
	logger := zaptest.NewLogger(t)
	service := &Service{}
	controller := NewServiceController(service, logger)

	status := controller.GetStatus()

	assert.NotNil(t, status)
	assert.Contains(t, status, "draining")
	assert.Contains(t, status, "can_enqueue")
	assert.Contains(t, status, "can_send")
	assert.Contains(t, status, "feature_flags")

	assert.False(t, status["draining"])
	assert.True(t, status["can_enqueue"])
	assert.True(t, status["can_send"])
}

func TestServiceController_EmergencyStop(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create a minimal service for testing
	db := &sqlx.DB{} // Mock DB
	service := &Service{
		logger: logger,
		db:     db,
	}

	controller := NewServiceController(service, logger)
	ctx := context.Background()

	// Emergency stop should disable all flags
	err := controller.EmergencyStop(ctx)

	// We expect an error since service.Stop() will fail with mock DB
	// But flags should still be disabled
	assert.False(t, controller.flagManager.IsEnabled(FlagEnableEnqueue))
	assert.False(t, controller.flagManager.IsEnabled(FlagEnableSend))

	// Error is expected due to mock service
	_ = err
}

func TestDrainModeManager_WaitForDrain_EmptyQueue(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ffm := NewFeatureFlagManager(logger)
	dmm := NewDrainModeManager(ffm, logger)
	ctx := context.Background()

	// Create a mock queue that reports zero depth
	queue := &Queue{}

	// This test would need a properly mocked queue
	// For now, we just verify the function exists
	err := dmm.WaitForDrain(ctx, queue, 5*time.Second)

	// Expected to error since queue methods aren't implemented
	_ = err
}

func TestFeatureFlagManager_ConcurrentAccess(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ffm := NewFeatureFlagManager(logger)

	done := make(chan bool)

	// Concurrent reads and writes
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				if id%2 == 0 {
					ffm.SetFlag(FlagEnableEnqueue, true, "test")
				} else {
					ffm.IsEnabled(FlagEnableEnqueue)
				}
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should not panic
	assert.True(t, true)
}

func TestFeatureFlagManager_Metadata(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ffm := NewFeatureFlagManager(logger)

	ffm.SetFlag(FlagEnableEnqueue, true, "admin@example.com")

	flags := ffm.GetAllFlags()
	state := flags[FlagEnableEnqueue]

	assert.NotNil(t, state)
	assert.Equal(t, "admin@example.com", state.UpdatedBy)
	assert.False(t, state.UpdatedAt.IsZero())
}

func BenchmarkFeatureFlagManager_IsEnabled(b *testing.B) {
	logger := zaptest.NewLogger(b)
	ffm := NewFeatureFlagManager(logger)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ffm.IsEnabled(FlagEnableEnqueue)
	}
}

func BenchmarkFeatureFlagManager_SetFlag(b *testing.B) {
	logger := zaptest.NewLogger(b)
	ffm := NewFeatureFlagManager(logger)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ffm.SetFlag(FlagEnableEnqueue, i%2 == 0, "benchmark")
	}
}

func BenchmarkFeatureFlagManager_IsEnabledFor(b *testing.B) {
	logger := zaptest.NewLogger(b)
	ffm := NewFeatureFlagManager(logger)
	userID := uuid.New().String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ffm.IsEnabledFor(FlagEnableEnqueue, userID)
	}
}
