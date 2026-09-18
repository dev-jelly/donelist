package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	// Redis key prefixes
	delayedQueueKey = "notification:delayed"  // ZSET for delayed notifications (score = timestamp)
	readyQueueKey   = "notification:ready"    // LIST for ready-to-send notifications
	processingKey   = "notification:processing" // SET for currently processing notifications
	deadLetterKey   = "notification:dlq"      // LIST for failed notifications

	// Processing configuration
	defaultBatchSize       = 100
	defaultProcessTimeout  = 30 * time.Second
	defaultVisibilityTimeout = 5 * time.Minute
)

// NotificationJob represents a notification job in the queue
type NotificationJob struct {
	ID          string    `json:"id"`
	UserID      uuid.UUID `json:"user_id"`
	Type        string    `json:"type"` // "checkin_reminder", "daily_summary", etc.
	Payload     map[string]interface{} `json:"payload"`
	ScheduledAt time.Time `json:"scheduled_at"`
	CreatedAt   time.Time `json:"created_at"`
	Attempts    int       `json:"attempts"`
	MaxAttempts int       `json:"max_attempts"`
	LastError   string    `json:"last_error,omitempty"`
}

// Queue handles Redis-based delayed notification queue operations
type Queue struct {
	redis  *redis.Client
	logger *zap.Logger
	config QueueConfig
}

// QueueConfig holds queue configuration
type QueueConfig struct {
	BatchSize          int
	ProcessTimeout     time.Duration
	VisibilityTimeout  time.Duration
	MaxRetries         int
	RetryBackoffFactor float64
}

// DefaultQueueConfig returns default queue configuration
func DefaultQueueConfig() QueueConfig {
	return QueueConfig{
		BatchSize:          defaultBatchSize,
		ProcessTimeout:     defaultProcessTimeout,
		VisibilityTimeout:  defaultVisibilityTimeout,
		MaxRetries:         3,
		RetryBackoffFactor: 2.0,
	}
}

// NewQueue creates a new notification queue
func NewQueue(redisClient *redis.Client, logger *zap.Logger, config QueueConfig) *Queue {
	return &Queue{
		redis:  redisClient,
		logger: logger,
		config: config,
	}
}

// Enqueue adds a notification to the delayed queue
func (q *Queue) Enqueue(ctx context.Context, job *NotificationJob) error {
	if job.ID == "" {
		job.ID = uuid.New().String()
	}
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now()
	}
	if job.MaxAttempts == 0 {
		job.MaxAttempts = q.config.MaxRetries
	}

	// Serialize job
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	// Add to delayed queue (ZSET) with scheduled time as score
	score := float64(job.ScheduledAt.Unix())
	if err := q.redis.ZAdd(ctx, delayedQueueKey, redis.Z{
		Score:  score,
		Member: data,
	}).Err(); err != nil {
		return fmt.Errorf("failed to enqueue notification: %w", err)
	}

	q.logger.Debug("Notification enqueued",
		zap.String("job_id", job.ID),
		zap.String("user_id", job.UserID.String()),
		zap.String("type", job.Type),
		zap.Time("scheduled_at", job.ScheduledAt),
	)

	return nil
}

// MoveDelayedToReady moves notifications from delayed queue to ready queue if their time has come
func (q *Queue) MoveDelayedToReady(ctx context.Context) (int, error) {
	now := time.Now().Unix()

	// Get all notifications that should be processed now (score <= now)
	result, err := q.redis.ZRangeByScore(ctx, delayedQueueKey, &redis.ZRangeBy{
		Min:   "-inf",
		Max:   fmt.Sprintf("%d", now),
		Count: int64(q.config.BatchSize),
	}).Result()

	if err != nil {
		return 0, fmt.Errorf("failed to get ready notifications: %w", err)
	}

	if len(result) == 0 {
		return 0, nil
	}

	// Use pipeline for atomic operations
	pipe := q.redis.Pipeline()

	for _, data := range result {
		// Add to ready queue
		pipe.RPush(ctx, readyQueueKey, data)
		// Remove from delayed queue
		pipe.ZRem(ctx, delayedQueueKey, data)
	}

	_, err = pipe.Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to move notifications to ready queue: %w", err)
	}

	q.logger.Debug("Moved notifications to ready queue",
		zap.Int("count", len(result)),
	)

	return len(result), nil
}

// Dequeue retrieves notifications from the ready queue for processing
func (q *Queue) Dequeue(ctx context.Context, count int) ([]*NotificationJob, error) {
	if count <= 0 {
		count = 1
	}
	if count > q.config.BatchSize {
		count = q.config.BatchSize
	}

	jobs := make([]*NotificationJob, 0, count)

	for i := 0; i < count; i++ {
		// Pop from ready queue and add to processing set atomically
		result, err := q.redis.LPop(ctx, readyQueueKey).Result()
		if err == redis.Nil {
			// No more items in queue
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to dequeue notification: %w", err)
		}

		var job NotificationJob
		if err := json.Unmarshal([]byte(result), &job); err != nil {
			q.logger.Error("Failed to unmarshal job, moving to DLQ",
				zap.Error(err),
				zap.String("data", result),
			)
			q.redis.RPush(ctx, deadLetterKey, result)
			continue
		}

		// Add to processing set with visibility timeout
		processingData, _ := json.Marshal(map[string]interface{}{
			"job":        job,
			"started_at": time.Now(),
		})
		q.redis.SetEx(ctx, fmt.Sprintf("%s:%s", processingKey, job.ID),
			processingData, q.config.VisibilityTimeout)

		jobs = append(jobs, &job)
	}

	return jobs, nil
}

// Complete marks a notification as successfully processed
func (q *Queue) Complete(ctx context.Context, jobID string) error {
	// Remove from processing set
	err := q.redis.Del(ctx, fmt.Sprintf("%s:%s", processingKey, jobID)).Err()
	if err != nil {
		return fmt.Errorf("failed to complete job: %w", err)
	}

	q.logger.Debug("Notification completed",
		zap.String("job_id", jobID),
	)

	return nil
}

// Fail marks a notification as failed and potentially retries it
func (q *Queue) Fail(ctx context.Context, job *NotificationJob, err error) error {
	job.Attempts++
	job.LastError = err.Error()

	if job.Attempts >= job.MaxAttempts {
		// Move to dead letter queue
		data, _ := json.Marshal(job)
		if err := q.redis.RPush(ctx, deadLetterKey, data).Err(); err != nil {
			return fmt.Errorf("failed to move job to DLQ: %w", err)
		}

		q.logger.Warn("Notification moved to DLQ",
			zap.String("job_id", job.ID),
			zap.Int("attempts", job.Attempts),
			zap.String("error", job.LastError),
		)
	} else {
		// Calculate exponential backoff
		backoffDuration := time.Duration(float64(time.Minute) *
			q.config.RetryBackoffFactor * float64(job.Attempts))
		job.ScheduledAt = time.Now().Add(backoffDuration)

		// Re-enqueue with new scheduled time
		if err := q.Enqueue(ctx, job); err != nil {
			return fmt.Errorf("failed to retry job: %w", err)
		}

		q.logger.Info("Notification rescheduled for retry",
			zap.String("job_id", job.ID),
			zap.Int("attempt", job.Attempts),
			zap.Time("retry_at", job.ScheduledAt),
		)
	}

	// Remove from processing set
	q.redis.Del(ctx, fmt.Sprintf("%s:%s", processingKey, job.ID))

	return nil
}

// GetQueueStats returns statistics about the queue
func (q *Queue) GetQueueStats(ctx context.Context) (map[string]interface{}, error) {
	pipe := q.redis.Pipeline()

	delayedCount := pipe.ZCard(ctx, delayedQueueKey)
	readyCount := pipe.LLen(ctx, readyQueueKey)
	dlqCount := pipe.LLen(ctx, deadLetterKey)

	// Count processing items by scanning keys
	processingKeys := pipe.Keys(ctx, fmt.Sprintf("%s:*", processingKey))

	_, err := pipe.Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get queue stats: %w", err)
	}

	stats := map[string]interface{}{
		"delayed_count":    delayedCount.Val(),
		"ready_count":      readyCount.Val(),
		"processing_count": len(processingKeys.Val()),
		"dlq_count":        dlqCount.Val(),
	}

	return stats, nil
}

// RecoverStuckJobs finds and recovers jobs that have been processing for too long
func (q *Queue) RecoverStuckJobs(ctx context.Context) (int, error) {
	keys, err := q.redis.Keys(ctx, fmt.Sprintf("%s:*", processingKey)).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get processing keys: %w", err)
	}

	recovered := 0
	for _, key := range keys {
		// Check if job has expired
		ttl, err := q.redis.TTL(ctx, key).Result()
		if err != nil {
			continue
		}

		// If TTL is -1 (no expiry) or -2 (key doesn't exist), clean it up
		if ttl < 0 {
			q.redis.Del(ctx, key)
			recovered++
		}
	}

	if recovered > 0 {
		q.logger.Info("Recovered stuck jobs",
			zap.Int("count", recovered),
		)
	}

	return recovered, nil
}

// GetDLQJobs retrieves jobs from the dead letter queue for inspection
func (q *Queue) GetDLQJobs(ctx context.Context, limit int) ([]*NotificationJob, error) {
	if limit <= 0 {
		limit = 100
	}

	// Get jobs from DLQ without removing them
	results, err := q.redis.LRange(ctx, deadLetterKey, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get DLQ jobs: %w", err)
	}

	jobs := make([]*NotificationJob, 0, len(results))
	for _, data := range results {
		var job NotificationJob
		if err := json.Unmarshal([]byte(data), &job); err != nil {
			q.logger.Error("Failed to unmarshal DLQ job",
				zap.Error(err),
				zap.String("data", data),
			)
			continue
		}
		jobs = append(jobs, &job)
	}

	return jobs, nil
}

// RetryDLQJob moves a job from DLQ back to the delayed queue for retry
func (q *Queue) RetryDLQJob(ctx context.Context, jobID string) error {
	// Get all jobs from DLQ
	results, err := q.redis.LRange(ctx, deadLetterKey, 0, -1).Result()
	if err != nil {
		return fmt.Errorf("failed to get DLQ jobs: %w", err)
	}

	// Find the specific job
	for i, data := range results {
		var job NotificationJob
		if err := json.Unmarshal([]byte(data), &job); err != nil {
			continue
		}

		if job.ID == jobID {
			// Remove from DLQ
			if err := q.redis.LRem(ctx, deadLetterKey, 1, data).Err(); err != nil {
				return fmt.Errorf("failed to remove job from DLQ: %w", err)
			}

			// Reset attempts and schedule for immediate retry
			job.Attempts = 0
			job.ScheduledAt = time.Now()
			job.LastError = ""

			// Re-enqueue
			if err := q.Enqueue(ctx, &job); err != nil {
				// If re-enqueue fails, put back in DLQ
				q.redis.RPush(ctx, deadLetterKey, data)
				return fmt.Errorf("failed to re-enqueue DLQ job: %w", err)
			}

			q.logger.Info("Retrying job from DLQ",
				zap.String("job_id", jobID),
				zap.Int("original_attempts", i),
			)

			return nil
		}
	}

	return fmt.Errorf("job not found in DLQ: %s", jobID)
}

// PurgeDLQ removes all jobs from the dead letter queue
func (q *Queue) PurgeDLQ(ctx context.Context) (int64, error) {
	count, err := q.redis.LLen(ctx, deadLetterKey).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get DLQ length: %w", err)
	}

	if count == 0 {
		return 0, nil
	}

	if err := q.redis.Del(ctx, deadLetterKey).Err(); err != nil {
		return 0, fmt.Errorf("failed to purge DLQ: %w", err)
	}

	q.logger.Info("Purged DLQ",
		zap.Int64("count", count),
	)

	return count, nil
}

// MoveToDLQ moves a job directly to the dead letter queue (for permanent failures)
func (q *Queue) MoveToDLQ(ctx context.Context, job *NotificationJob, reason string) error {
	job.LastError = reason

	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	if err := q.redis.RPush(ctx, deadLetterKey, data).Err(); err != nil {
		return fmt.Errorf("failed to move job to DLQ: %w", err)
	}

	// Remove from processing if it was there
	q.redis.Del(ctx, fmt.Sprintf("%s:%s", processingKey, job.ID))

	q.logger.Warn("Notification moved to DLQ",
		zap.String("job_id", job.ID),
		zap.String("user_id", job.UserID.String()),
		zap.String("type", job.Type),
		zap.String("reason", reason),
	)

	return nil
}
