package notification

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	// idempotencyKeyPrefix is the Redis key prefix for idempotency tracking
	idempotencyKeyPrefix = "notification:idempotency"

	// Default idempotency window: 1 hour
	defaultIdempotencyWindow = 1 * time.Hour
)

// IdempotencyConfig holds idempotency configuration
type IdempotencyConfig struct {
	// Window is the time window for idempotency checking
	Window time.Duration

	// Enabled determines if idempotency checking is enabled
	Enabled bool
}

// DefaultIdempotencyConfig returns default idempotency configuration
// Idempotency window: 1 hour
func DefaultIdempotencyConfig() IdempotencyConfig {
	return IdempotencyConfig{
		Window:  defaultIdempotencyWindow,
		Enabled: true,
	}
}

// IdempotencyManager handles idempotency key tracking to prevent duplicate sends
type IdempotencyManager struct {
	redis  *redis.Client
	logger *zap.Logger
	config IdempotencyConfig
}

// NewIdempotencyManager creates a new idempotency manager
func NewIdempotencyManager(redisClient *redis.Client, logger *zap.Logger, config IdempotencyConfig) *IdempotencyManager {
	return &IdempotencyManager{
		redis:  redisClient,
		logger: logger,
		config: config,
	}
}

// GenerateKey generates an idempotency key for a notification job
// The key is based on user_id, notification type, and payload content
func (im *IdempotencyManager) GenerateKey(job *NotificationJob) string {
	// Create a deterministic key from job content
	data := struct {
		UserID  string                 `json:"user_id"`
		Type    string                 `json:"type"`
		Payload map[string]interface{} `json:"payload"`
	}{
		UserID:  job.UserID.String(),
		Type:    job.Type,
		Payload: job.Payload,
	}

	// Marshal to JSON for consistent hashing
	jsonData, err := json.Marshal(data)
	if err != nil {
		// Fallback to simple key
		return fmt.Sprintf("%s:%s", job.UserID.String(), job.Type)
	}

	// Generate SHA256 hash
	hash := sha256.Sum256(jsonData)
	return hex.EncodeToString(hash[:])
}

// GetRedisKey returns the full Redis key for an idempotency key
func (im *IdempotencyManager) GetRedisKey(idempotencyKey string) string {
	return fmt.Sprintf("%s:%s", idempotencyKeyPrefix, idempotencyKey)
}

// CheckDuplicate checks if a notification is a duplicate within the idempotency window
// Returns true if it's a duplicate, false otherwise
func (im *IdempotencyManager) CheckDuplicate(ctx context.Context, job *NotificationJob) (bool, error) {
	if !im.config.Enabled {
		return false, nil
	}

	idempotencyKey := im.GenerateKey(job)
	redisKey := im.GetRedisKey(idempotencyKey)

	// Check if key exists
	exists, err := im.redis.Exists(ctx, redisKey).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check idempotency key: %w", err)
	}

	if exists > 0 {
		// Get the stored job ID for logging
		storedJobID, _ := im.redis.Get(ctx, redisKey).Result()

		im.logger.Info("Duplicate notification detected",
			zap.String("job_id", job.ID),
			zap.String("original_job_id", storedJobID),
			zap.String("idempotency_key", idempotencyKey),
			zap.String("user_id", job.UserID.String()),
			zap.String("type", job.Type),
		)

		return true, nil
	}

	return false, nil
}

// MarkProcessed marks a notification as processed to prevent duplicates
// Stores the job ID with TTL equal to the idempotency window
func (im *IdempotencyManager) MarkProcessed(ctx context.Context, job *NotificationJob) error {
	if !im.config.Enabled {
		return nil
	}

	idempotencyKey := im.GenerateKey(job)
	redisKey := im.GetRedisKey(idempotencyKey)

	// Store job ID with TTL
	err := im.redis.Set(ctx, redisKey, job.ID, im.config.Window).Err()
	if err != nil {
		return fmt.Errorf("failed to mark notification as processed: %w", err)
	}

	im.logger.Debug("Marked notification as processed",
		zap.String("job_id", job.ID),
		zap.String("idempotency_key", idempotencyKey),
		zap.Duration("window", im.config.Window),
	)

	return nil
}

// RemoveKey removes an idempotency key (e.g., if send failed and should be retried)
func (im *IdempotencyManager) RemoveKey(ctx context.Context, job *NotificationJob) error {
	if !im.config.Enabled {
		return nil
	}

	idempotencyKey := im.GenerateKey(job)
	redisKey := im.GetRedisKey(idempotencyKey)

	err := im.redis.Del(ctx, redisKey).Err()
	if err != nil {
		return fmt.Errorf("failed to remove idempotency key: %w", err)
	}

	im.logger.Debug("Removed idempotency key",
		zap.String("job_id", job.ID),
		zap.String("idempotency_key", idempotencyKey),
	)

	return nil
}

// GetStats returns idempotency statistics
func (im *IdempotencyManager) GetStats(ctx context.Context) (map[string]interface{}, error) {
	// Count total idempotency keys
	pattern := fmt.Sprintf("%s:*", idempotencyKeyPrefix)
	keys, err := im.redis.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get idempotency stats: %w", err)
	}

	return map[string]interface{}{
		"enabled":         im.config.Enabled,
		"window_seconds":  im.config.Window.Seconds(),
		"active_keys":     len(keys),
	}, nil
}

// CleanupExpired removes expired idempotency keys
// Note: Redis handles this automatically via TTL, but this can be used for manual cleanup
func (im *IdempotencyManager) CleanupExpired(ctx context.Context) (int, error) {
	// Redis automatically removes expired keys, so this is mostly a no-op
	// Could be extended to scan and verify TTLs if needed
	return 0, nil
}
