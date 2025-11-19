package notification

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupTestQueue(t *testing.T) (*Queue, *miniredis.Miniredis, func()) {
	t.Helper()

	// Create miniredis instance
	mr, err := miniredis.Run()
	require.NoError(t, err)

	// Create Redis client
	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	logger := zap.NewNop()
	config := DefaultQueueConfig()
	queue := NewQueue(client, logger, config)

	cleanup := func() {
		client.Close()
		mr.Close()
	}

	return queue, mr, cleanup
}

func TestQueue_Enqueue(t *testing.T) {
	queue, _, cleanup := setupTestQueue(t)
	defer cleanup()

	ctx := context.Background()
	userID := uuid.New()

	job := &NotificationJob{
		UserID:      userID,
		Type:        "checkin_reminder",
		Payload:     map[string]interface{}{"message": "Time to check in!"},
		ScheduledAt: time.Now().Add(5 * time.Minute),
	}

	err := queue.Enqueue(ctx, job)
	assert.NoError(t, err)
	assert.NotEmpty(t, job.ID)
	assert.False(t, job.CreatedAt.IsZero())
}

func TestQueue_MoveDelayedToReady(t *testing.T) {
	queue, mr, cleanup := setupTestQueue(t)
	defer cleanup()

	ctx := context.Background()
	userID := uuid.New()

	// Enqueue a job that's ready now
	jobReady := &NotificationJob{
		UserID:      userID,
		Type:        "checkin_reminder",
		Payload:     map[string]interface{}{"message": "Ready now"},
		ScheduledAt: time.Now().Add(-1 * time.Minute), // In the past
	}
	err := queue.Enqueue(ctx, jobReady)
	require.NoError(t, err)

	// Enqueue a job for the future
	jobFuture := &NotificationJob{
		UserID:      userID,
		Type:        "checkin_reminder",
		Payload:     map[string]interface{}{"message": "Future job"},
		ScheduledAt: time.Now().Add(10 * time.Minute),
	}
	err = queue.Enqueue(ctx, jobFuture)
	require.NoError(t, err)

	// Fast-forward time in miniredis
	mr.FastForward(2 * time.Minute)

	// Move ready jobs
	count, err := queue.MoveDelayedToReady(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 1, count) // Only the ready job should be moved
}

func TestQueue_DequeueAndComplete(t *testing.T) {
	queue, mr, cleanup := setupTestQueue(t)
	defer cleanup()

	ctx := context.Background()
	userID := uuid.New()

	// Enqueue and move to ready
	job := &NotificationJob{
		UserID:      userID,
		Type:        "checkin_reminder",
		ScheduledAt: time.Now().Add(-1 * time.Minute),
	}
	err := queue.Enqueue(ctx, job)
	require.NoError(t, err)

	mr.FastForward(2 * time.Minute)
	_, err = queue.MoveDelayedToReady(ctx)
	require.NoError(t, err)

	// Dequeue
	jobs, err := queue.Dequeue(ctx, 1)
	assert.NoError(t, err)
	assert.Len(t, jobs, 1)
	assert.Equal(t, job.ID, jobs[0].ID)

	// Complete
	err = queue.Complete(ctx, jobs[0].ID)
	assert.NoError(t, err)
}

func TestQueue_Fail_WithRetry(t *testing.T) {
	queue, _, cleanup := setupTestQueue(t)
	defer cleanup()

	ctx := context.Background()
	userID := uuid.New()

	job := &NotificationJob{
		ID:          uuid.New().String(),
		UserID:      userID,
		Type:        "checkin_reminder",
		ScheduledAt: time.Now(),
		MaxAttempts: 3,
		Attempts:    0,
	}

	// Fail the job (should retry)
	err := queue.Fail(ctx, job, assert.AnError)
	assert.NoError(t, err)
	assert.Equal(t, 1, job.Attempts)
	assert.NotEmpty(t, job.LastError)
	assert.True(t, job.ScheduledAt.After(time.Now()))

	// Check that it was re-enqueued to delayed queue
	stats, err := queue.GetQueueStats(ctx)
	assert.NoError(t, err)
	assert.Greater(t, stats["delayed_count"].(int64), int64(0))
}

func TestQueue_Fail_MoveToDLQ(t *testing.T) {
	queue, _, cleanup := setupTestQueue(t)
	defer cleanup()

	ctx := context.Background()
	userID := uuid.New()

	job := &NotificationJob{
		ID:          uuid.New().String(),
		UserID:      userID,
		Type:        "checkin_reminder",
		ScheduledAt: time.Now(),
		MaxAttempts: 2,
		Attempts:    1, // Already failed once
	}

	// Fail the job (should go to DLQ)
	err := queue.Fail(ctx, job, assert.AnError)
	assert.NoError(t, err)

	// Check DLQ stats
	stats, err := queue.GetQueueStats(ctx)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), stats["dlq_count"])
}

func TestQueue_GetQueueStats(t *testing.T) {
	queue, _, cleanup := setupTestQueue(t)
	defer cleanup()

	ctx := context.Background()
	userID := uuid.New()

	// Enqueue jobs with different scheduled times
	jobPast := &NotificationJob{
		UserID:      userID,
		Type:        "checkin_reminder",
		ScheduledAt: time.Now().Add(-1 * time.Minute), // Ready now
	}
	err := queue.Enqueue(ctx, jobPast)
	require.NoError(t, err)

	jobFuture := &NotificationJob{
		UserID:      userID,
		Type:        "checkin_reminder",
		ScheduledAt: time.Now().Add(10 * time.Minute), // Still delayed
	}
	err = queue.Enqueue(ctx, jobFuture)
	require.NoError(t, err)

	// Move ready jobs
	count, err := queue.MoveDelayedToReady(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	stats, err := queue.GetQueueStats(ctx)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), stats["delayed_count"])   // 1 still delayed
	assert.Equal(t, int64(1), stats["ready_count"])     // 1 ready
	assert.Equal(t, int64(0), stats["dlq_count"])       // 0 in DLQ
}

func TestQueue_RecoverStuckJobs(t *testing.T) {
	queue, _, cleanup := setupTestQueue(t)
	defer cleanup()

	ctx := context.Background()

	// Initially no stuck jobs
	count, err := queue.RecoverStuckJobs(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 0, count)
}
