package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// FeatureFlag represents a feature flag configuration
type FeatureFlag struct {
	// Name is the unique identifier for the feature
	Name string `json:"name"`

	// Enabled indicates if the feature is globally enabled
	Enabled bool `json:"enabled"`

	// RolloutPercentage is the percentage of users to enable the feature for (0-100)
	RolloutPercentage int `json:"rollout_percentage"`

	// UserWhitelist contains specific user IDs to enable the feature for
	UserWhitelist []string `json:"user_whitelist"`

	// UserBlacklist contains specific user IDs to disable the feature for
	UserBlacklist []string `json:"user_blacklist"`

	// Conditions contains additional conditions for enabling the feature
	Conditions map[string]interface{} `json:"conditions"`

	// ExpiresAt is when the feature flag expires (optional)
	ExpiresAt *time.Time `json:"expires_at,omitempty"`

	// Description provides human-readable description
	Description string `json:"description"`

	// CreatedAt is when the flag was created
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the flag was last updated
	UpdatedAt time.Time `json:"updated_at"`
}

// FeatureFlagManager manages feature flags for the WebSocket system
type FeatureFlagManager struct {
	flags  map[string]*FeatureFlag
	mu     sync.RWMutex
	redis  *redis.Client
	logger *zap.Logger

	// Cache settings
	cacheDuration time.Duration
	lastRefresh   time.Time
}

// NewFeatureFlagManager creates a new feature flag manager
func NewFeatureFlagManager(redisClient *redis.Client, cacheDuration time.Duration, logger *zap.Logger) *FeatureFlagManager {
	if cacheDuration == 0 {
		cacheDuration = 5 * time.Minute
	}

	ffm := &FeatureFlagManager{
		flags:         make(map[string]*FeatureFlag),
		redis:         redisClient,
		logger:        logger,
		cacheDuration: cacheDuration,
	}

	// Start background refresh
	go ffm.startRefreshLoop(context.Background())

	return ffm
}

// IsEnabled checks if a feature is enabled for a specific user
func (ffm *FeatureFlagManager) IsEnabled(featureName string, userID string, context map[string]interface{}) bool {
	ffm.mu.RLock()
	flag, exists := ffm.flags[featureName]
	ffm.mu.RUnlock()

	if !exists {
		// Try to load from Redis
		if err := ffm.refreshFlag(featureName); err != nil {
			ffm.logger.Debug("Feature flag not found",
				zap.String("feature", featureName),
				zap.Error(err),
			)
			return false
		}

		ffm.mu.RLock()
		flag, exists = ffm.flags[featureName]
		ffm.mu.RUnlock()

		if !exists {
			return false
		}
	}

	// Check if flag has expired
	if flag.ExpiresAt != nil && flag.ExpiresAt.Before(time.Now()) {
		ffm.logger.Debug("Feature flag expired",
			zap.String("feature", featureName),
			zap.Time("expired_at", *flag.ExpiresAt),
		)
		return false
	}

	// Check if globally disabled
	if !flag.Enabled {
		return false
	}

	// Check blacklist
	for _, blacklistedID := range flag.UserBlacklist {
		if blacklistedID == userID {
			return false
		}
	}

	// Check whitelist
	for _, whitelistedID := range flag.UserWhitelist {
		if whitelistedID == userID {
			return true
		}
	}

	// Check rollout percentage
	if flag.RolloutPercentage < 100 {
		if !ffm.isUserInRollout(featureName, userID, flag.RolloutPercentage) {
			return false
		}
	}

	// Check additional conditions
	if len(flag.Conditions) > 0 {
		if !ffm.evaluateConditions(flag.Conditions, context) {
			return false
		}
	}

	return true
}

// isUserInRollout determines if a user is in the rollout percentage
func (ffm *FeatureFlagManager) isUserInRollout(featureName, userID string, percentage int) bool {
	// Use consistent hashing to ensure the same user always gets the same result
	h := fnv.New32a()
	h.Write([]byte(featureName + ":" + userID))
	hash := h.Sum32()

	// Convert to percentage (0-100)
	userPercentage := int(hash % 100)
	return userPercentage < percentage
}

// evaluateConditions evaluates additional conditions for a feature flag
func (ffm *FeatureFlagManager) evaluateConditions(conditions map[string]interface{}, context map[string]interface{}) bool {
	for key, expectedValue := range conditions {
		actualValue, exists := context[key]
		if !exists {
			return false
		}

		// Simple equality check for now
		// In production, this could support more complex operators
		if actualValue != expectedValue {
			return false
		}
	}
	return true
}

// SetFlag creates or updates a feature flag
func (ffm *FeatureFlagManager) SetFlag(flag *FeatureFlag) error {
	if flag.Name == "" {
		return fmt.Errorf("feature name is required")
	}

	now := time.Now()
	if flag.CreatedAt.IsZero() {
		flag.CreatedAt = now
	}
	flag.UpdatedAt = now

	// Store in Redis
	data, err := json.Marshal(flag)
	if err != nil {
		return fmt.Errorf("failed to marshal flag: %w", err)
	}

	ctx := context.Background()
	key := fmt.Sprintf("feature_flag:%s", flag.Name)
	if err := ffm.redis.Set(ctx, key, data, 0).Err(); err != nil {
		return fmt.Errorf("failed to store flag in Redis: %w", err)
	}

	// Update local cache
	ffm.mu.Lock()
	ffm.flags[flag.Name] = flag
	ffm.mu.Unlock()

	ffm.logger.Info("Feature flag updated",
		zap.String("feature", flag.Name),
		zap.Bool("enabled", flag.Enabled),
		zap.Int("rollout_percentage", flag.RolloutPercentage),
	)

	return nil
}

// GetFlag retrieves a feature flag
func (ffm *FeatureFlagManager) GetFlag(featureName string) (*FeatureFlag, error) {
	ffm.mu.RLock()
	flag, exists := ffm.flags[featureName]
	ffm.mu.RUnlock()

	if exists {
		return flag, nil
	}

	// Try to load from Redis
	if err := ffm.refreshFlag(featureName); err != nil {
		return nil, err
	}

	ffm.mu.RLock()
	flag, exists = ffm.flags[featureName]
	ffm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("feature flag not found: %s", featureName)
	}

	return flag, nil
}

// refreshFlag refreshes a single flag from Redis
func (ffm *FeatureFlagManager) refreshFlag(featureName string) error {
	ctx := context.Background()
	key := fmt.Sprintf("feature_flag:%s", featureName)

	data, err := ffm.redis.Get(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("failed to get flag from Redis: %w", err)
	}

	var flag FeatureFlag
	if err := json.Unmarshal([]byte(data), &flag); err != nil {
		return fmt.Errorf("failed to unmarshal flag: %w", err)
	}

	ffm.mu.Lock()
	ffm.flags[featureName] = &flag
	ffm.mu.Unlock()

	return nil
}

// RefreshAll refreshes all feature flags from Redis
func (ffm *FeatureFlagManager) RefreshAll() error {
	ctx := context.Background()

	// Get all feature flag keys
	keys, err := ffm.redis.Keys(ctx, "feature_flag:*").Result()
	if err != nil {
		return fmt.Errorf("failed to list feature flags: %w", err)
	}

	newFlags := make(map[string]*FeatureFlag)

	for _, key := range keys {
		data, err := ffm.redis.Get(ctx, key).Result()
		if err != nil {
			ffm.logger.Warn("Failed to get feature flag",
				zap.String("key", key),
				zap.Error(err),
			)
			continue
		}

		var flag FeatureFlag
		if err := json.Unmarshal([]byte(data), &flag); err != nil {
			ffm.logger.Warn("Failed to unmarshal feature flag",
				zap.String("key", key),
				zap.Error(err),
			)
			continue
		}

		newFlags[flag.Name] = &flag
	}

	ffm.mu.Lock()
	ffm.flags = newFlags
	ffm.lastRefresh = time.Now()
	ffm.mu.Unlock()

	ffm.logger.Debug("Feature flags refreshed",
		zap.Int("count", len(newFlags)),
	)

	return nil
}

// startRefreshLoop starts a background loop to refresh feature flags
func (ffm *FeatureFlagManager) startRefreshLoop(ctx context.Context) {
	ticker := time.NewTicker(ffm.cacheDuration)
	defer ticker.Stop()

	// Initial refresh
	if err := ffm.RefreshAll(); err != nil {
		ffm.logger.Error("Failed to refresh feature flags", zap.Error(err))
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := ffm.RefreshAll(); err != nil {
				ffm.logger.Error("Failed to refresh feature flags", zap.Error(err))
			}
		}
	}
}

// CanaryRelease manages canary releases for WebSocket features
type CanaryRelease struct {
	// Name is the unique identifier for the canary release
	Name string `json:"name"`

	// Version is the version being released
	Version string `json:"version"`

	// StartPercentage is the initial rollout percentage
	StartPercentage int `json:"start_percentage"`

	// CurrentPercentage is the current rollout percentage
	CurrentPercentage int `json:"current_percentage"`

	// TargetPercentage is the target rollout percentage
	TargetPercentage int `json:"target_percentage"`

	// IncrementPercentage is how much to increase per step
	IncrementPercentage int `json:"increment_percentage"`

	// IncrementInterval is how often to increase the percentage
	IncrementInterval time.Duration `json:"increment_interval"`

	// StartTime is when the canary release started
	StartTime time.Time `json:"start_time"`

	// LastIncrement is when the percentage was last increased
	LastIncrement time.Time `json:"last_increment"`

	// Status is the current status of the canary release
	Status CanaryStatus `json:"status"`

	// Metrics contains metrics for the canary release
	Metrics *CanaryMetrics `json:"metrics"`

	// RollbackThreshold contains conditions for automatic rollback
	RollbackThreshold *RollbackThreshold `json:"rollback_threshold,omitempty"`
}

// CanaryStatus represents the status of a canary release
type CanaryStatus string

const (
	CanaryStatusPending    CanaryStatus = "pending"
	CanaryStatusInProgress CanaryStatus = "in_progress"
	CanaryStatusCompleted  CanaryStatus = "completed"
	CanaryStatusRolledBack CanaryStatus = "rolled_back"
	CanaryStatusPaused     CanaryStatus = "paused"
)

// CanaryMetrics contains metrics for monitoring a canary release
type CanaryMetrics struct {
	TotalUsers      int     `json:"total_users"`
	CanaryUsers     int     `json:"canary_users"`
	ErrorRate       float64 `json:"error_rate"`
	Latency95       float64 `json:"latency_95"`
	SuccessRate     float64 `json:"success_rate"`
	LastUpdated     time.Time `json:"last_updated"`
}

// RollbackThreshold defines conditions for automatic rollback
type RollbackThreshold struct {
	MaxErrorRate   float64 `json:"max_error_rate"`
	MaxLatency95   float64 `json:"max_latency_95"`
	MinSuccessRate float64 `json:"min_success_rate"`
}

// CanaryReleaseManager manages canary releases
type CanaryReleaseManager struct {
	releases map[string]*CanaryRelease
	mu       sync.RWMutex
	redis    *redis.Client
	logger   *zap.Logger
	ffm      *FeatureFlagManager
}

// NewCanaryReleaseManager creates a new canary release manager
func NewCanaryReleaseManager(redisClient *redis.Client, ffm *FeatureFlagManager, logger *zap.Logger) *CanaryReleaseManager {
	crm := &CanaryReleaseManager{
		releases: make(map[string]*CanaryRelease),
		redis:    redisClient,
		logger:   logger,
		ffm:      ffm,
	}

	// Start background processor
	go crm.startProcessor(context.Background())

	return crm
}

// StartRelease starts a new canary release
func (crm *CanaryReleaseManager) StartRelease(release *CanaryRelease) error {
	if release.Name == "" {
		return fmt.Errorf("release name is required")
	}

	now := time.Now()
	release.StartTime = now
	release.LastIncrement = now
	release.Status = CanaryStatusInProgress
	release.CurrentPercentage = release.StartPercentage

	// Create corresponding feature flag
	flag := &FeatureFlag{
		Name:              fmt.Sprintf("canary_%s", release.Name),
		Enabled:           true,
		RolloutPercentage: release.CurrentPercentage,
		Description:       fmt.Sprintf("Canary release for %s version %s", release.Name, release.Version),
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if err := crm.ffm.SetFlag(flag); err != nil {
		return fmt.Errorf("failed to create feature flag: %w", err)
	}

	// Store release configuration
	crm.mu.Lock()
	crm.releases[release.Name] = release
	crm.mu.Unlock()

	// Persist to Redis
	if err := crm.persistRelease(release); err != nil {
		return fmt.Errorf("failed to persist release: %w", err)
	}

	crm.logger.Info("Canary release started",
		zap.String("name", release.Name),
		zap.String("version", release.Version),
		zap.Int("start_percentage", release.StartPercentage),
		zap.Int("target_percentage", release.TargetPercentage),
	)

	return nil
}

// UpdateMetrics updates metrics for a canary release
func (crm *CanaryReleaseManager) UpdateMetrics(releaseName string, metrics *CanaryMetrics) error {
	crm.mu.Lock()
	release, exists := crm.releases[releaseName]
	if !exists {
		crm.mu.Unlock()
		return fmt.Errorf("release not found: %s", releaseName)
	}

	metrics.LastUpdated = time.Now()
	release.Metrics = metrics

	// Check rollback thresholds
	if release.RollbackThreshold != nil && release.Status == CanaryStatusInProgress {
		if crm.shouldRollback(metrics, release.RollbackThreshold) {
			crm.mu.Unlock()
			return crm.Rollback(releaseName, "Automatic rollback due to threshold breach")
		}
	}
	crm.mu.Unlock()

	// Persist updated release
	return crm.persistRelease(release)
}

// shouldRollback checks if metrics exceed rollback thresholds
func (crm *CanaryReleaseManager) shouldRollback(metrics *CanaryMetrics, threshold *RollbackThreshold) bool {
	if threshold.MaxErrorRate > 0 && metrics.ErrorRate > threshold.MaxErrorRate {
		return true
	}
	if threshold.MaxLatency95 > 0 && metrics.Latency95 > threshold.MaxLatency95 {
		return true
	}
	if threshold.MinSuccessRate > 0 && metrics.SuccessRate < threshold.MinSuccessRate {
		return true
	}
	return false
}

// IncrementPercentage increases the canary rollout percentage
func (crm *CanaryReleaseManager) IncrementPercentage(releaseName string) error {
	crm.mu.Lock()
	defer crm.mu.Unlock()

	release, exists := crm.releases[releaseName]
	if !exists {
		return fmt.Errorf("release not found: %s", releaseName)
	}

	if release.Status != CanaryStatusInProgress {
		return fmt.Errorf("release is not in progress: %s", release.Status)
	}

	// Calculate new percentage
	newPercentage := release.CurrentPercentage + release.IncrementPercentage
	if newPercentage > release.TargetPercentage {
		newPercentage = release.TargetPercentage
	}

	release.CurrentPercentage = newPercentage
	release.LastIncrement = time.Now()

	// Update feature flag
	flag := &FeatureFlag{
		Name:              fmt.Sprintf("canary_%s", release.Name),
		Enabled:           true,
		RolloutPercentage: newPercentage,
		UpdatedAt:         time.Now(),
	}

	if err := crm.ffm.SetFlag(flag); err != nil {
		return fmt.Errorf("failed to update feature flag: %w", err)
	}

	// Check if completed
	if newPercentage >= release.TargetPercentage {
		release.Status = CanaryStatusCompleted
		crm.logger.Info("Canary release completed",
			zap.String("name", release.Name),
			zap.String("version", release.Version),
		)
	}

	// Persist updated release
	return crm.persistRelease(release)
}

// Rollback rolls back a canary release
func (crm *CanaryReleaseManager) Rollback(releaseName string, reason string) error {
	crm.mu.Lock()
	defer crm.mu.Unlock()

	release, exists := crm.releases[releaseName]
	if !exists {
		return fmt.Errorf("release not found: %s", releaseName)
	}

	release.Status = CanaryStatusRolledBack
	release.CurrentPercentage = 0

	// Disable feature flag
	flag := &FeatureFlag{
		Name:      fmt.Sprintf("canary_%s", release.Name),
		Enabled:   false,
		UpdatedAt: time.Now(),
	}

	if err := crm.ffm.SetFlag(flag); err != nil {
		return fmt.Errorf("failed to disable feature flag: %w", err)
	}

	crm.logger.Warn("Canary release rolled back",
		zap.String("name", release.Name),
		zap.String("version", release.Version),
		zap.String("reason", reason),
	)

	// Persist updated release
	return crm.persistRelease(release)
}

// persistRelease persists a canary release to Redis
func (crm *CanaryReleaseManager) persistRelease(release *CanaryRelease) error {
	data, err := json.Marshal(release)
	if err != nil {
		return fmt.Errorf("failed to marshal release: %w", err)
	}

	ctx := context.Background()
	key := fmt.Sprintf("canary_release:%s", release.Name)
	if err := crm.redis.Set(ctx, key, data, 0).Err(); err != nil {
		return fmt.Errorf("failed to store release in Redis: %w", err)
	}

	return nil
}

// startProcessor starts a background processor for automatic increments
func (crm *CanaryReleaseManager) startProcessor(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			crm.processReleases()
		}
	}
}

// processReleases processes all active releases for automatic increments
func (crm *CanaryReleaseManager) processReleases() {
	crm.mu.RLock()
	releases := make([]*CanaryRelease, 0, len(crm.releases))
	for _, release := range crm.releases {
		releases = append(releases, release)
	}
	crm.mu.RUnlock()

	for _, release := range releases {
		if release.Status != CanaryStatusInProgress {
			continue
		}

		// Check if it's time to increment
		if time.Since(release.LastIncrement) >= release.IncrementInterval {
			if err := crm.IncrementPercentage(release.Name); err != nil {
				crm.logger.Error("Failed to increment canary percentage",
					zap.String("release", release.Name),
					zap.Error(err),
				)
			}
		}
	}
}