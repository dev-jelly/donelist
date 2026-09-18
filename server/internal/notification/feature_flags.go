package notification

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// FeatureFlag represents a feature toggle
type FeatureFlag string

const (
	// FlagEnableEnqueue controls notification enqueueing
	FlagEnableEnqueue FeatureFlag = "enable_enqueue"

	// FlagEnableSend controls notification sending
	FlagEnableSend FeatureFlag = "enable_send"

	// FlagEnableDND controls DND functionality
	FlagEnableDND FeatureFlag = "enable_dnd"

	// FlagEnableRetries controls retry mechanism
	FlagEnableRetries FeatureFlag = "enable_retries"

	// FlagEnableBatching controls batch processing
	FlagEnableBatching FeatureFlag = "enable_batching"

	// FlagDrainMode enables drain mode (stop accepting new, process existing)
	FlagDrainMode FeatureFlag = "drain_mode"
)

// FeatureFlagManager manages feature flags
type FeatureFlagManager struct {
	flags   map[FeatureFlag]*FlagState
	mu      sync.RWMutex
	logger  *zap.Logger
	hooks   map[FeatureFlag][]FlagChangeHook
}

// FlagState represents the state of a feature flag
type FlagState struct {
	Enabled     bool                   `json:"enabled"`
	EnabledFor  map[string]bool        `json:"enabled_for,omitempty"` // User/tenant specific
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	UpdatedAt   time.Time              `json:"updated_at"`
	UpdatedBy   string                 `json:"updated_by,omitempty"`
}

// FlagChangeHook is called when a flag changes
type FlagChangeHook func(flag FeatureFlag, oldState, newState bool)

// NewFeatureFlagManager creates a new feature flag manager
func NewFeatureFlagManager(logger *zap.Logger) *FeatureFlagManager {
	ffm := &FeatureFlagManager{
		flags:  make(map[FeatureFlag]*FlagState),
		logger: logger,
		hooks:  make(map[FeatureFlag][]FlagChangeHook),
	}

	// Initialize default flags (all enabled by default)
	defaults := []FeatureFlag{
		FlagEnableEnqueue,
		FlagEnableSend,
		FlagEnableDND,
		FlagEnableRetries,
		FlagEnableBatching,
	}

	for _, flag := range defaults {
		ffm.flags[flag] = &FlagState{
			Enabled:   true,
			EnabledFor: make(map[string]bool),
			Metadata:  make(map[string]interface{}),
			UpdatedAt: time.Now(),
		}
	}

	// Drain mode is disabled by default
	ffm.flags[FlagDrainMode] = &FlagState{
		Enabled:   false,
		UpdatedAt: time.Now(),
	}

	return ffm
}

// IsEnabled checks if a flag is enabled globally
func (ffm *FeatureFlagManager) IsEnabled(flag FeatureFlag) bool {
	ffm.mu.RLock()
	defer ffm.mu.RUnlock()

	if state, exists := ffm.flags[flag]; exists {
		return state.Enabled
	}

	return false
}

// IsEnabledFor checks if a flag is enabled for a specific user/tenant
func (ffm *FeatureFlagManager) IsEnabledFor(flag FeatureFlag, identifier string) bool {
	ffm.mu.RLock()
	defer ffm.mu.RUnlock()

	state, exists := ffm.flags[flag]
	if !exists {
		return false
	}

	// Check user/tenant specific override first
	if enabled, hasOverride := state.EnabledFor[identifier]; hasOverride {
		return enabled
	}

	// Fall back to global setting
	return state.Enabled
}

// SetFlag sets a feature flag globally
func (ffm *FeatureFlagManager) SetFlag(flag FeatureFlag, enabled bool, updatedBy string) {
	ffm.mu.Lock()
	defer ffm.mu.Unlock()

	state, exists := ffm.flags[flag]
	if !exists {
		state = &FlagState{
			EnabledFor: make(map[string]bool),
			Metadata:  make(map[string]interface{}),
		}
		ffm.flags[flag] = state
	}

	oldState := state.Enabled
	state.Enabled = enabled
	state.UpdatedAt = time.Now()
	state.UpdatedBy = updatedBy

	ffm.logger.Info("Feature flag updated",
		zap.String("flag", string(flag)),
		zap.Bool("old_state", oldState),
		zap.Bool("new_state", enabled),
		zap.String("updated_by", updatedBy),
	)

	// Call hooks
	if hooks, exists := ffm.hooks[flag]; exists {
		for _, hook := range hooks {
			hook(flag, oldState, enabled)
		}
	}
}

// SetFlagFor sets a feature flag for a specific user/tenant
func (ffm *FeatureFlagManager) SetFlagFor(flag FeatureFlag, identifier string, enabled bool) {
	ffm.mu.Lock()
	defer ffm.mu.Unlock()

	state, exists := ffm.flags[flag]
	if !exists {
		state = &FlagState{
			Enabled:   false,
			EnabledFor: make(map[string]bool),
			Metadata:  make(map[string]interface{}),
			UpdatedAt: time.Now(),
		}
		ffm.flags[flag] = state
	}

	state.EnabledFor[identifier] = enabled
	state.UpdatedAt = time.Now()

	ffm.logger.Info("Feature flag updated for identifier",
		zap.String("flag", string(flag)),
		zap.String("identifier", identifier),
		zap.Bool("enabled", enabled),
	)
}

// RegisterHook registers a callback for flag changes
func (ffm *FeatureFlagManager) RegisterHook(flag FeatureFlag, hook FlagChangeHook) {
	ffm.mu.Lock()
	defer ffm.mu.Unlock()

	ffm.hooks[flag] = append(ffm.hooks[flag], hook)
}

// GetAllFlags returns all flag states
func (ffm *FeatureFlagManager) GetAllFlags() map[FeatureFlag]*FlagState {
	ffm.mu.RLock()
	defer ffm.mu.RUnlock()

	result := make(map[FeatureFlag]*FlagState)
	for flag, state := range ffm.flags {
		// Create a copy to prevent external modification
		stateCopy := &FlagState{
			Enabled:   state.Enabled,
			EnabledFor: make(map[string]bool),
			Metadata:  make(map[string]interface{}),
			UpdatedAt: state.UpdatedAt,
			UpdatedBy: state.UpdatedBy,
		}
		for k, v := range state.EnabledFor {
			stateCopy.EnabledFor[k] = v
		}
		for k, v := range state.Metadata {
			stateCopy.Metadata[k] = v
		}
		result[flag] = stateCopy
	}

	return result
}

// DrainModeManager handles graceful shutdown and drain mode
type DrainModeManager struct {
	draining       atomic.Bool
	flagManager    *FeatureFlagManager
	logger         *zap.Logger
	onDrainStart   []func()
	onDrainComplete []func()
	mu             sync.Mutex
}

// NewDrainModeManager creates a new drain mode manager
func NewDrainModeManager(flagManager *FeatureFlagManager, logger *zap.Logger) *DrainModeManager {
	return &DrainModeManager{
		flagManager:    flagManager,
		logger:         logger,
		onDrainStart:   make([]func(), 0),
		onDrainComplete: make([]func(), 0),
	}
}

// IsDraining returns true if the system is in drain mode
func (dmm *DrainModeManager) IsDraining() bool {
	return dmm.draining.Load()
}

// StartDrain initiates drain mode
func (dmm *DrainModeManager) StartDrain(ctx context.Context) error {
	if dmm.draining.Swap(true) {
		return fmt.Errorf("already draining")
	}

	dmm.logger.Info("Starting drain mode")

	// Set drain mode flag
	dmm.flagManager.SetFlag(FlagDrainMode, true, "drain_mode_manager")

	// Disable new enqueueing
	dmm.flagManager.SetFlag(FlagEnableEnqueue, false, "drain_mode_manager")

	// Execute drain start hooks
	dmm.mu.Lock()
	for _, hook := range dmm.onDrainStart {
		hook()
	}
	dmm.mu.Unlock()

	return nil
}

// CompleteDrain completes the drain process
func (dmm *DrainModeManager) CompleteDrain(ctx context.Context) error {
	if !dmm.draining.Load() {
		return fmt.Errorf("not in drain mode")
	}

	dmm.logger.Info("Completing drain mode")

	// Disable sending
	dmm.flagManager.SetFlag(FlagEnableSend, false, "drain_mode_manager")

	// Execute drain complete hooks
	dmm.mu.Lock()
	for _, hook := range dmm.onDrainComplete {
		hook()
	}
	dmm.mu.Unlock()

	dmm.draining.Store(false)

	return nil
}

// WaitForDrain waits for all in-flight notifications to complete
func (dmm *DrainModeManager) WaitForDrain(ctx context.Context, queue *Queue, timeout time.Duration) error {
	dmm.logger.Info("Waiting for queue to drain",
		zap.Duration("timeout", timeout),
	)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	deadline := time.Now().Add(timeout)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			stats, err := queue.GetQueueStats(ctx)
			if err != nil {
				return fmt.Errorf("failed to get queue stats: %w", err)
			}

			depth := int64(0)
			if d, ok := stats["depth"].(int64); ok {
				depth = d
			}

			if depth == 0 {
				dmm.logger.Info("Queue drained successfully")
				return nil
			}

			dmm.logger.Info("Waiting for queue to drain",
				zap.Int64("remaining", depth),
			)

			if time.Now().After(deadline) {
				return fmt.Errorf("drain timeout: %d messages remaining", depth)
			}
		}
	}
}

// RegisterDrainStartHook registers a callback for drain start
func (dmm *DrainModeManager) RegisterDrainStartHook(hook func()) {
	dmm.mu.Lock()
	defer dmm.mu.Unlock()
	dmm.onDrainStart = append(dmm.onDrainStart, hook)
}

// RegisterDrainCompleteHook registers a callback for drain completion
func (dmm *DrainModeManager) RegisterDrainCompleteHook(hook func()) {
	dmm.mu.Lock()
	defer dmm.mu.Unlock()
	dmm.onDrainComplete = append(dmm.onDrainComplete, hook)
}

// RollbackDrain cancels drain mode and re-enables normal operation
func (dmm *DrainModeManager) RollbackDrain(ctx context.Context) error {
	if !dmm.draining.Load() {
		return fmt.Errorf("not in drain mode")
	}

	dmm.logger.Warn("Rolling back drain mode")

	// Re-enable flags
	dmm.flagManager.SetFlag(FlagEnableEnqueue, true, "drain_mode_manager")
	dmm.flagManager.SetFlag(FlagEnableSend, true, "drain_mode_manager")
	dmm.flagManager.SetFlag(FlagDrainMode, false, "drain_mode_manager")

	dmm.draining.Store(false)

	return nil
}

// ServiceController integrates feature flags with service
type ServiceController struct {
	service     *Service
	flagManager *FeatureFlagManager
	drainManager *DrainModeManager
	logger      *zap.Logger
}

// NewServiceController creates a new service controller
func NewServiceController(service *Service, logger *zap.Logger) *ServiceController {
	flagManager := NewFeatureFlagManager(logger)
	drainManager := NewDrainModeManager(flagManager, logger)

	return &ServiceController{
		service:     service,
		flagManager: flagManager,
		drainManager: drainManager,
		logger:      logger,
	}
}

// CanEnqueue checks if enqueueing is allowed
func (sc *ServiceController) CanEnqueue(userID uuid.UUID) bool {
	// Check drain mode
	if sc.drainManager.IsDraining() {
		return false
	}

	// Check global enqueue flag
	if !sc.flagManager.IsEnabled(FlagEnableEnqueue) {
		return false
	}

	// Check user-specific flag
	return sc.flagManager.IsEnabledFor(FlagEnableEnqueue, userID.String())
}

// CanSend checks if sending is allowed
func (sc *ServiceController) CanSend() bool {
	return sc.flagManager.IsEnabled(FlagEnableSend)
}

// InitiateGracefulShutdown initiates graceful shutdown
func (sc *ServiceController) InitiateGracefulShutdown(ctx context.Context, timeout time.Duration) error {
	sc.logger.Info("Initiating graceful shutdown")

	// Start drain mode
	if err := sc.drainManager.StartDrain(ctx); err != nil {
		return err
	}

	// Wait for queue to drain
	if err := sc.drainManager.WaitForDrain(ctx, sc.service.queue, timeout); err != nil {
		sc.logger.Warn("Drain timeout, forcing shutdown", zap.Error(err))
	}

	// Complete drain
	if err := sc.drainManager.CompleteDrain(ctx); err != nil {
		return err
	}

	// Stop service
	return sc.service.Stop()
}

// GetStatus returns the current status of the service
func (sc *ServiceController) GetStatus() map[string]interface{} {
	return map[string]interface{}{
		"draining":      sc.drainManager.IsDraining(),
		"can_enqueue":   sc.flagManager.IsEnabled(FlagEnableEnqueue),
		"can_send":      sc.flagManager.IsEnabled(FlagEnableSend),
		"feature_flags": sc.flagManager.GetAllFlags(),
	}
}

// EmergencyStop immediately stops all notification processing
func (sc *ServiceController) EmergencyStop(ctx context.Context) error {
	sc.logger.Warn("Emergency stop initiated")

	// Disable all processing
	sc.flagManager.SetFlag(FlagEnableEnqueue, false, "emergency_stop")
	sc.flagManager.SetFlag(FlagEnableSend, false, "emergency_stop")

	// Stop service
	return sc.service.Stop()
}
