package secrets

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RotationStrategy defines how secrets should be rotated
type RotationStrategy string

const (
	// RotationStrategyImmediate rotates immediately (risky)
	RotationStrategyImmediate RotationStrategy = "immediate"

	// RotationStrategyGradual uses gradual rollout with dual-key support
	RotationStrategyGradual RotationStrategy = "gradual"

	// RotationStrategyScheduled rotates at scheduled time
	RotationStrategyScheduled RotationStrategy = "scheduled"
)

// RotationConfig configures rotation behavior
type RotationConfig struct {
	Strategy         RotationStrategy
	GracePeriod      time.Duration // How long old keys remain valid
	RolloutPercentage float64      // For gradual rollout (0-100)
	ScheduledTime    *time.Time    // For scheduled rotation
	AutoRollback     bool          // Automatically rollback on errors
}

// RotationScheduler handles automated secret rotation
type RotationScheduler struct {
	manager  *SecretManager
	interval time.Duration
	stopCh   chan struct{}
	wg       sync.WaitGroup

	// Rotation hooks for custom logic
	preRotateHook  func(ctx context.Context, metadata *SecretMetadata) error
	postRotateHook func(ctx context.Context, oldMeta, newMeta *SecretMetadata) error
	rollbackHook   func(ctx context.Context, metadata *SecretMetadata) error
}

// NewRotationScheduler creates a rotation scheduler
func NewRotationScheduler(manager *SecretManager, checkInterval time.Duration) *RotationScheduler {
	if checkInterval == 0 {
		checkInterval = 1 * time.Hour
	}

	return &RotationScheduler{
		manager:  manager,
		interval: checkInterval,
		stopCh:   make(chan struct{}),
	}
}

// SetPreRotateHook sets a hook to run before rotation
func (rs *RotationScheduler) SetPreRotateHook(hook func(ctx context.Context, metadata *SecretMetadata) error) {
	rs.preRotateHook = hook
}

// SetPostRotateHook sets a hook to run after rotation
func (rs *RotationScheduler) SetPostRotateHook(hook func(ctx context.Context, oldMeta, newMeta *SecretMetadata) error) {
	rs.postRotateHook = hook
}

// SetRollbackHook sets a hook to run on rollback
func (rs *RotationScheduler) SetRollbackHook(hook func(ctx context.Context, metadata *SecretMetadata) error) {
	rs.rollbackHook = hook
}

// Start starts the rotation scheduler
func (rs *RotationScheduler) Start(ctx context.Context) {
	rs.wg.Add(1)
	go rs.run(ctx)
}

// Stop stops the rotation scheduler
func (rs *RotationScheduler) Stop() {
	close(rs.stopCh)
	rs.wg.Wait()
}

// run is the main scheduler loop
func (rs *RotationScheduler) run(ctx context.Context) {
	defer rs.wg.Done()

	ticker := time.NewTicker(rs.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-rs.stopCh:
			return
		case <-ticker.C:
			if err := rs.checkAndRotate(ctx); err != nil {
				// Log error but continue
				fmt.Printf("rotation check failed: %v\n", err)
			}
		}
	}
}

// checkAndRotate checks for secrets due for rotation and rotates them
func (rs *RotationScheduler) checkAndRotate(ctx context.Context) error {
	// Get secrets due for rotation
	dueSecrets, err := rs.manager.GetSecretsDueForRotation(ctx)
	if err != nil {
		return fmt.Errorf("failed to get secrets due for rotation: %w", err)
	}

	if len(dueSecrets) == 0 {
		return nil
	}

	fmt.Printf("Found %d secrets due for rotation\n", len(dueSecrets))

	for _, metadata := range dueSecrets {
		if err := rs.rotateSecret(ctx, metadata); err != nil {
			fmt.Printf("failed to rotate secret %s: %v\n", metadata.Name, err)
			// Continue with other secrets
			continue
		}
	}

	return nil
}

// rotateSecret rotates a single secret with gradual rollout
func (rs *RotationScheduler) rotateSecret(ctx context.Context, metadata *SecretMetadata) error {
	// Run pre-rotate hook
	if rs.preRotateHook != nil {
		if err := rs.preRotateHook(ctx, metadata); err != nil {
			return fmt.Errorf("pre-rotate hook failed: %w", err)
		}
	}

	// Get current secret value
	currentValue, err := rs.manager.GetSecret(ctx, metadata.Name)
	if err != nil {
		return fmt.Errorf("failed to get current secret: %w", err)
	}

	// Generate new secret value (this should be customized per secret type)
	newValue, err := rs.generateNewValue(metadata.Type, currentValue.Value)
	if err != nil {
		return fmt.Errorf("failed to generate new value: %w", err)
	}

	// Rotate the secret
	newMetadata, err := rs.manager.RotateSecret(ctx, metadata.Name, newValue)
	if err != nil {
		return fmt.Errorf("failed to rotate secret: %w", err)
	}

	// Run post-rotate hook
	if rs.postRotateHook != nil {
		if err := rs.postRotateHook(ctx, metadata, newMetadata); err != nil {
			// Attempt rollback
			if rs.rollbackHook != nil {
				_ = rs.rollbackHook(ctx, metadata)
			}
			return fmt.Errorf("post-rotate hook failed: %w", err)
		}
	}

	fmt.Printf("Successfully rotated secret: %s (v%d -> v%d)\n",
		metadata.Name, metadata.Version, newMetadata.Version)

	return nil
}

// generateNewValue generates a new secret value based on type
// This is a simplified implementation - customize per secret type
func (rs *RotationScheduler) generateNewValue(secretType SecretType, currentValue string) (string, error) {
	switch secretType {
	case SecretTypeJWT:
		// Generate new JWT signing key
		return generateRandomKey(64)

	case SecretTypeAPIKey:
		// Generate new API key
		return generateRandomKey(32)

	case SecretTypeDatabase:
		// For database passwords, this should trigger a DB password change
		return generateRandomKey(32)

	case SecretTypeEncryption:
		// Generate new encryption key
		return generateRandomKey(32)

	default:
		// For unknown types, generate a random key
		return generateRandomKey(32)
	}
}

// generateRandomKey generates a random alphanumeric key
func generateRandomKey(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)

	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}

	return string(b), nil
}

// GradualRotationManager handles gradual secret rotation with dual-key support
type GradualRotationManager struct {
	manager      *SecretManager
	gracePeriod  time.Duration
	activeSecrets map[string]*RotationState
	mu           sync.RWMutex
}

// RotationState tracks the state of a rotation
type RotationState struct {
	OldKeyID      string
	NewKeyID      string
	StartedAt     time.Time
	GracePeriodEnd time.Time
	Percentage    float64 // Current rollout percentage
	Status        RotationStatus
}

// RotationStatus represents rotation status
type RotationStatus string

const (
	RotationStatusInProgress RotationStatus = "in_progress"
	RotationStatusCompleted  RotationStatus = "completed"
	RotationStatusRolledBack RotationStatus = "rolled_back"
	RotationStatusFailed     RotationStatus = "failed"
)

// NewGradualRotationManager creates a gradual rotation manager
func NewGradualRotationManager(manager *SecretManager, gracePeriod time.Duration) *GradualRotationManager {
	return &GradualRotationManager{
		manager:       manager,
		gracePeriod:   gracePeriod,
		activeSecrets: make(map[string]*RotationState),
	}
}

// StartGradualRotation starts a gradual rotation
func (grm *GradualRotationManager) StartGradualRotation(ctx context.Context, secretName string, newValue string) error {
	grm.mu.Lock()
	defer grm.mu.Unlock()

	// Check if already rotating
	if _, exists := grm.activeSecrets[secretName]; exists {
		return fmt.Errorf("rotation already in progress for: %s", secretName)
	}

	// Get current secret
	currentSecret, err := grm.manager.GetSecret(ctx, secretName)
	if err != nil {
		return fmt.Errorf("failed to get current secret: %w", err)
	}

	// Rotate the secret
	newMetadata, err := grm.manager.RotateSecret(ctx, secretName, newValue)
	if err != nil {
		return fmt.Errorf("failed to rotate secret: %w", err)
	}

	// Track rotation state
	grm.activeSecrets[secretName] = &RotationState{
		OldKeyID:       currentSecret.Metadata.KeyID,
		NewKeyID:       newMetadata.KeyID,
		StartedAt:      time.Now(),
		GracePeriodEnd: time.Now().Add(grm.gracePeriod),
		Percentage:     0,
		Status:         RotationStatusInProgress,
	}

	return nil
}

// IncrementRollout increases the rollout percentage
func (grm *GradualRotationManager) IncrementRollout(secretName string, increment float64) error {
	grm.mu.Lock()
	defer grm.mu.Unlock()

	state, exists := grm.activeSecrets[secretName]
	if !exists {
		return fmt.Errorf("no active rotation for: %s", secretName)
	}

	state.Percentage += increment
	if state.Percentage > 100 {
		state.Percentage = 100
	}

	if state.Percentage >= 100 {
		state.Status = RotationStatusCompleted
	}

	return nil
}

// CompleteRotation completes a gradual rotation
func (grm *GradualRotationManager) CompleteRotation(secretName string) error {
	grm.mu.Lock()
	defer grm.mu.Unlock()

	state, exists := grm.activeSecrets[secretName]
	if !exists {
		return fmt.Errorf("no active rotation for: %s", secretName)
	}

	state.Status = RotationStatusCompleted
	state.Percentage = 100

	return nil
}

// RollbackRotation rolls back a rotation
func (grm *GradualRotationManager) RollbackRotation(ctx context.Context, secretName string) error {
	grm.mu.Lock()
	defer grm.mu.Unlock()

	state, exists := grm.activeSecrets[secretName]
	if !exists {
		return fmt.Errorf("no active rotation for: %s", secretName)
	}

	// Mark old key as active again
	// This would require storing the old value securely
	state.Status = RotationStatusRolledBack

	return nil
}

// GetRotationState gets the current rotation state
func (grm *GradualRotationManager) GetRotationState(secretName string) (*RotationState, error) {
	grm.mu.RLock()
	defer grm.mu.RUnlock()

	state, exists := grm.activeSecrets[secretName]
	if !exists {
		return nil, fmt.Errorf("no active rotation for: %s", secretName)
	}

	// Return copy
	stateCopy := *state
	return &stateCopy, nil
}

// CleanupExpiredRotations removes completed or expired rotations
func (grm *GradualRotationManager) CleanupExpiredRotations() {
	grm.mu.Lock()
	defer grm.mu.Unlock()

	now := time.Now()
	for name, state := range grm.activeSecrets {
		if state.Status == RotationStatusCompleted ||
			state.Status == RotationStatusRolledBack ||
			now.After(state.GracePeriodEnd) {
			delete(grm.activeSecrets, name)
		}
	}
}
