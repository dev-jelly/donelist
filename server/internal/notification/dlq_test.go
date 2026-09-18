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

func setupDLQTest(t *testing.T) (*Queue, *redis.Client, *miniredis.Miniredis) {
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

	return queue, client, mr
}

func TestMoveToDLQ(t *testing.T) {
	queue, _, mr := setupDLQTest(t)
	defer mr.Close()
	ctx := context.Background()

	job := &NotificationJob{
		ID:          uuid.New().String(),
		UserID:      uuid.New(),
		Type:        "test",
		Payload:     map[string]interface{}{"test": "data"},
		ScheduledAt: time.Now(),
		CreatedAt:   time.Now(),
		Attempts:    3,
		MaxAttempts: 3,
	}

	t.Run("Successfully move to DLQ", func(t *testing.T) {
		err := queue.MoveToDLQ(ctx, job, "permanent failure")
		assert.NoError(t, err)

		// Verify job is in DLQ
		stats, err := queue.GetQueueStats(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(1), stats["dlq_count"], "Should have 1 job in DLQ")
	})

	t.Run("Verify error reason is set", func(t *testing.T) {
		jobs, err := queue.GetDLQJobs(ctx, 10)
		require.NoError(t, err)
		require.Len(t, jobs, 1)
		assert.Equal(t, "permanent failure", jobs[0].LastError)
	})
}

func TestGetDLQJobs(t *testing.T) {
	queue, _, mr := setupDLQTest(t)
	defer mr.Close()
	ctx := context.Background()

	// Add multiple jobs to DLQ
	jobIDs := make([]string, 5)
	for i := 0; i < 5; i++ {
		job := &NotificationJob{
			ID:          uuid.New().String(),
			UserID:      uuid.New(),
			Type:        "test",
			Payload:     map[string]interface{}{"index": i},
			ScheduledAt: time.Now(),
			CreatedAt:   time.Now(),
			Attempts:    3,
			MaxAttempts: 3,
		}
		jobIDs[i] = job.ID

		err := queue.MoveToDLQ(ctx, job, "test failure")
		require.NoError(t, err)
	}

	t.Run("Get all DLQ jobs", func(t *testing.T) {
		jobs, err := queue.GetDLQJobs(ctx, 10)
		assert.NoError(t, err)
		assert.Len(t, jobs, 5, "Should retrieve all 5 jobs")
	})

	t.Run("Get limited DLQ jobs", func(t *testing.T) {
		jobs, err := queue.GetDLQJobs(ctx, 3)
		assert.NoError(t, err)
		assert.Len(t, jobs, 3, "Should retrieve only 3 jobs")
	})

	t.Run("Get DLQ jobs with default limit", func(t *testing.T) {
		jobs, err := queue.GetDLQJobs(ctx, 0)
		assert.NoError(t, err)
		assert.Len(t, jobs, 5, "Should use default limit and get all jobs")
	})

	t.Run("Empty DLQ returns empty list", func(t *testing.T) {
		// Purge DLQ
		_, err := queue.PurgeDLQ(ctx)
		require.NoError(t, err)

		jobs, err := queue.GetDLQJobs(ctx, 10)
		assert.NoError(t, err)
		assert.Len(t, jobs, 0, "Should return empty list")
	})
}

func TestRetryDLQJob(t *testing.T) {
	queue, _, mr := setupDLQTest(t)
	defer mr.Close()
	ctx := context.Background()

	job := &NotificationJob{
		ID:          uuid.New().String(),
		UserID:      uuid.New(),
		Type:        "test",
		Payload:     map[string]interface{}{"test": "data"},
		ScheduledAt: time.Now(),
		CreatedAt:   time.Now(),
		Attempts:    3,
		MaxAttempts: 3,
		LastError:   "original error",
	}

	// Move to DLQ
	err := queue.MoveToDLQ(ctx, job, "test failure")
	require.NoError(t, err)

	t.Run("Successfully retry DLQ job", func(t *testing.T) {
		err := queue.RetryDLQJob(ctx, job.ID)
		assert.NoError(t, err)

		// Verify DLQ is empty
		stats, err := queue.GetQueueStats(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(0), stats["dlq_count"], "DLQ should be empty")

		// Verify job is back in delayed queue
		assert.Greater(t, stats["delayed_count"].(int64), int64(0), "Job should be in delayed queue")
	})

	t.Run("Retry non-existent job fails", func(t *testing.T) {
		err := queue.RetryDLQJob(ctx, "non-existent-job-id")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found", "Should indicate job not found")
	})

	t.Run("Retried job has reset attempts", func(t *testing.T) {
		// Add another job to DLQ
		job2 := &NotificationJob{
			ID:          uuid.New().String(),
			UserID:      uuid.New(),
			Type:        "test",
			Payload:     map[string]interface{}{"test": "data2"},
			ScheduledAt: time.Now(),
			CreatedAt:   time.Now(),
			Attempts:    3,
			MaxAttempts: 3,
			LastError:   "original error",
		}

		err := queue.MoveToDLQ(ctx, job2, "test failure")
		require.NoError(t, err)

		// Retry the job
		err = queue.RetryDLQJob(ctx, job2.ID)
		require.NoError(t, err)

		// Move delayed jobs to ready queue
		_, err = queue.MoveDelayedToReady(ctx)
		require.NoError(t, err)

		// Dequeue and verify attempts reset
		jobs, err := queue.Dequeue(ctx, 1)
		require.NoError(t, err)
		require.Len(t, jobs, 1)
		assert.Equal(t, 0, jobs[0].Attempts, "Attempts should be reset to 0")
		assert.Empty(t, jobs[0].LastError, "Last error should be cleared")
	})
}

func TestPurgeDLQ(t *testing.T) {
	queue, _, mr := setupDLQTest(t)
	defer mr.Close()
	ctx := context.Background()

	// Add multiple jobs to DLQ
	for i := 0; i < 10; i++ {
		job := &NotificationJob{
			ID:          uuid.New().String(),
			UserID:      uuid.New(),
			Type:        "test",
			Payload:     map[string]interface{}{"index": i},
			ScheduledAt: time.Now(),
			CreatedAt:   time.Now(),
			Attempts:    3,
			MaxAttempts: 3,
		}

		err := queue.MoveToDLQ(ctx, job, "test failure")
		require.NoError(t, err)
	}

	t.Run("Purge DLQ", func(t *testing.T) {
		count, err := queue.PurgeDLQ(ctx)
		assert.NoError(t, err)
		assert.Equal(t, int64(10), count, "Should purge 10 jobs")

		// Verify DLQ is empty
		stats, err := queue.GetQueueStats(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(0), stats["dlq_count"], "DLQ should be empty")
	})

	t.Run("Purge empty DLQ", func(t *testing.T) {
		count, err := queue.PurgeDLQ(ctx)
		assert.NoError(t, err)
		assert.Equal(t, int64(0), count, "Should return 0 for empty DLQ")
	})
}

func TestDLQIntegration(t *testing.T) {
	queue, _, mr := setupDLQTest(t)
	defer mr.Close()
	ctx := context.Background()

	t.Run("Job moves to DLQ after max retries", func(t *testing.T) {
		job := &NotificationJob{
			ID:          uuid.New().String(),
			UserID:      uuid.New(),
			Type:        "test",
			Payload:     map[string]interface{}{"test": "data"},
			ScheduledAt: time.Now(),
			CreatedAt:   time.Now(),
			Attempts:    0,
			MaxAttempts: 3,
		}

		// Enqueue job
		err := queue.Enqueue(ctx, job)
		require.NoError(t, err)

		// Move to ready
		_, err = queue.MoveDelayedToReady(ctx)
		require.NoError(t, err)

		// Dequeue and fail multiple times
		for i := 0; i < 3; i++ {
			jobs, err := queue.Dequeue(ctx, 1)
			require.NoError(t, err)
			require.Len(t, jobs, 1)

			// Fail the job
			err = queue.Fail(ctx, jobs[0], ErrProviderUnavailable)
			require.NoError(t, err)

			// If not last attempt, move delayed to ready for next iteration
			if i < 2 {
				time.Sleep(10 * time.Millisecond) // Small delay for scheduled time
				_, err = queue.MoveDelayedToReady(ctx)
				require.NoError(t, err)
			}
		}

		// Verify job is in DLQ
		stats, err := queue.GetQueueStats(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(1), stats["dlq_count"], "Job should be in DLQ after max retries")
	})

	t.Run("Permanent failure moves directly to DLQ", func(t *testing.T) {
		job := &NotificationJob{
			ID:          uuid.New().String(),
			UserID:      uuid.New(),
			Type:        "test",
			Payload:     map[string]interface{}{"test": "data"},
			ScheduledAt: time.Now(),
			CreatedAt:   time.Now(),
			Attempts:    0,
			MaxAttempts: 3,
		}

		err := queue.MoveToDLQ(ctx, job, "invalid token - permanent failure")
		require.NoError(t, err)

		// Verify it went straight to DLQ
		jobs, err := queue.GetDLQJobs(ctx, 10)
		require.NoError(t, err)

		found := false
		for _, j := range jobs {
			if j.ID == job.ID {
				found = true
				assert.Contains(t, j.LastError, "permanent failure")
				break
			}
		}
		assert.True(t, found, "Job should be in DLQ")
	})
}

func TestDLQStats(t *testing.T) {
	queue, _, mr := setupDLQTest(t)
	defer mr.Close()
	ctx := context.Background()

	t.Run("Initial stats show empty DLQ", func(t *testing.T) {
		stats, err := queue.GetQueueStats(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(0), stats["dlq_count"], "DLQ should be empty initially")
	})

	t.Run("Stats update after adding to DLQ", func(t *testing.T) {
		// Add 3 jobs to DLQ
		for i := 0; i < 3; i++ {
			job := &NotificationJob{
				ID:          uuid.New().String(),
				UserID:      uuid.New(),
				Type:        "test",
				Payload:     map[string]interface{}{"index": i},
				ScheduledAt: time.Now(),
				CreatedAt:   time.Now(),
			}
			err := queue.MoveToDLQ(ctx, job, "test failure")
			require.NoError(t, err)
		}

		stats, err := queue.GetQueueStats(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(3), stats["dlq_count"], "DLQ should have 3 jobs")
	})

	t.Run("Stats update after purging DLQ", func(t *testing.T) {
		_, err := queue.PurgeDLQ(ctx)
		require.NoError(t, err)

		stats, err := queue.GetQueueStats(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(0), stats["dlq_count"], "DLQ should be empty after purge")
	})
}
