package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupTestRetransmissionPipeline(t *testing.T) (*RetransmissionPipeline, *miniredis.Miniredis) {
	mr := miniredis.RunT(t)

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	logger, _ := zap.NewDevelopment()

	config := DefaultRetransmissionConfig()
	config.WorkerCount = 2
	config.RetryInterval = 100 * time.Millisecond
	config.MaxRetryInterval = 500 * time.Millisecond
	config.MessageTTL = 1 * time.Hour

	pipeline := NewRetransmissionPipeline(redisClient, config, logger)

	return pipeline, mr
}

func TestQueueMessage(t *testing.T) {
	pipeline, mr := setupTestRetransmissionPipeline(t)
	defer mr.Close()

	ctx := context.Background()

	msg := &Message{
		ID:   "test_msg_1",
		Type: MessageTypeCheckin,
		Data: json.RawMessage(`{"test": "data"}`),
	}

	err := pipeline.QueueMessage(ctx, "user123", msg)
	require.NoError(t, err)

	// Verify message is in queue
	count, err := pipeline.redis.ZCard(ctx, pipeline.queueKey()).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Verify user-specific queue
	userQueueKey := pipeline.userQueueKey("user123")
	userCount, err := pipeline.redis.ZCard(ctx, userQueueKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), userCount)

	// Check metrics
	assert.Equal(t, uint64(1), atomic.LoadUint64(&pipeline.messagesQueued))
}

func TestProcessBatch(t *testing.T) {
	pipeline, mr := setupTestRetransmissionPipeline(t)
	defer mr.Close()

	ctx := context.Background()

	// Setup delivery handler
	deliveredMessages := make([]*Message, 0)
	var mu sync.Mutex

	pipeline.SetDeliveryHandler(func(userID string, message *Message) error {
		mu.Lock()
		deliveredMessages = append(deliveredMessages, message)
		mu.Unlock()
		return nil
	})

	// Queue multiple messages
	for i := 0; i < 5; i++ {
		msg := &Message{
			ID:   string(rune(i)),
			Type: MessageTypeCheckin,
			Data: json.RawMessage(`{"index": ` + string(rune(i)) + `}`),
		}
		err := pipeline.QueueMessage(ctx, "user123", msg)
		require.NoError(t, err)
	}

	// Process batch
	processed, err := pipeline.ProcessBatch(ctx)
	require.NoError(t, err)
	assert.Equal(t, 5, processed)

	// Verify messages were delivered
	mu.Lock()
	assert.Len(t, deliveredMessages, 5)
	mu.Unlock()

	// Verify queue is empty
	count, _ := pipeline.redis.ZCard(ctx, pipeline.queueKey()).Result()
	assert.Equal(t, int64(0), count)

	// Check metrics
	assert.Equal(t, uint64(5), atomic.LoadUint64(&pipeline.messagesDelivered))
}

func TestRetryMechanism(t *testing.T) {
	pipeline, mr := setupTestRetransmissionPipeline(t)
	defer mr.Close()

	ctx := context.Background()

	attemptCount := 0
	var mu sync.Mutex

	// Setup delivery handler that fails first 2 attempts
	pipeline.SetDeliveryHandler(func(userID string, message *Message) error {
		mu.Lock()
		defer mu.Unlock()
		attemptCount++
		if attemptCount <= 2 {
			return errors.New("delivery failed")
		}
		return nil
	})

	msg := &Message{
		ID:   "retry_test",
		Type: MessageTypeCheckin,
		Data: json.RawMessage(`{"test": "retry"}`),
	}

	err := pipeline.QueueMessage(ctx, "user123", msg)
	require.NoError(t, err)

	// First attempt - should fail
	processed, err := pipeline.ProcessBatch(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	// Check that message is still in queue with updated retry time
	count, _ := pipeline.redis.ZCard(ctx, pipeline.queueKey()).Result()
	assert.Equal(t, int64(1), count)

	// Wait for retry interval
	time.Sleep(150 * time.Millisecond)

	// Second attempt - should fail again
	processed, err = pipeline.ProcessBatch(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	// Wait for next retry
	time.Sleep(250 * time.Millisecond)

	// Third attempt - should succeed
	processed, err = pipeline.ProcessBatch(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	// Verify queue is now empty
	count, _ = pipeline.redis.ZCard(ctx, pipeline.queueKey()).Result()
	assert.Equal(t, int64(0), count)

	// Check attempt count
	mu.Lock()
	assert.Equal(t, 3, attemptCount)
	mu.Unlock()

	// Check metrics
	assert.Equal(t, uint64(1), atomic.LoadUint64(&pipeline.messagesDelivered))
	assert.Equal(t, uint64(2), atomic.LoadUint64(&pipeline.retransmissions))
}

func TestMaxRetriesExceeded(t *testing.T) {
	pipeline, mr := setupTestRetransmissionPipeline(t)
	defer mr.Close()

	pipeline.config.MaxRetries = 2
	ctx := context.Background()

	// Setup delivery handler that always fails
	pipeline.SetDeliveryHandler(func(userID string, message *Message) error {
		return errors.New("permanent failure")
	})

	msg := &Message{
		ID:   "dlq_test",
		Type: MessageTypeCheckin,
		Data: json.RawMessage(`{"test": "dlq"}`),
	}

	err := pipeline.QueueMessage(ctx, "user123", msg)
	require.NoError(t, err)

	// Process until max retries exceeded
	for i := 0; i < pipeline.config.MaxRetries; i++ {
		processed, err := pipeline.ProcessBatch(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, processed)
		time.Sleep(150 * time.Millisecond)
	}

	// Verify message moved to DLQ
	dlqCount, _ := pipeline.redis.ZCard(ctx, pipeline.deadLetterQueueKey()).Result()
	assert.Equal(t, int64(1), dlqCount)

	// Verify main queue is empty
	count, _ := pipeline.redis.ZCard(ctx, pipeline.queueKey()).Result()
	assert.Equal(t, int64(0), count)

	// Check metrics
	assert.Equal(t, uint64(1), atomic.LoadUint64(&pipeline.messagesFailed))
}

func TestMessageExpiration(t *testing.T) {
	pipeline, mr := setupTestRetransmissionPipeline(t)
	defer mr.Close()

	pipeline.config.MessageTTL = 1 * time.Millisecond // Very short TTL for testing
	ctx := context.Background()

	msg := &Message{
		ID:   "expired_test",
		Type: MessageTypeCheckin,
		Data: json.RawMessage(`{"test": "expired"}`),
	}

	// Queue a message with very old creation time
	messageID := "user123:expired_test:12345"
	queuedMsg := &QueuedMessage{
		ID:          messageID,
		UserID:      "user123",
		Message:     msg,
		Status:      StatusPending,
		RetryCount:  0,
		NextRetryAt: time.Now(),
		CreatedAt:   time.Now().Add(-2 * time.Hour), // Old message
	}

	data, _ := json.Marshal(queuedMsg)
	score := float64(queuedMsg.NextRetryAt.Unix())
	err := pipeline.redis.ZAdd(ctx, pipeline.queueKey(), redis.Z{
		Score:  score,
		Member: data,
	}).Err()
	require.NoError(t, err)

	// Process batch - should handle as expired
	processed, err := pipeline.ProcessBatch(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	// Verify message removed from queue
	count, _ := pipeline.redis.ZCard(ctx, pipeline.queueKey()).Result()
	assert.Equal(t, int64(0), count)

	// Check metrics
	assert.Equal(t, uint64(1), atomic.LoadUint64(&pipeline.messagesExpired))
}

func TestAcknowledgeMessage(t *testing.T) {
	pipeline, mr := setupTestRetransmissionPipeline(t)
	defer mr.Close()

	ctx := context.Background()

	err := pipeline.AcknowledgeMessage(ctx, "msg123")
	require.NoError(t, err)

	// Verify acknowledgment stored
	ackKey := "ack:msg123"
	exists := mr.Exists(ackKey)
	assert.True(t, exists)
}

func TestGetUserQueueStatus(t *testing.T) {
	pipeline, mr := setupTestRetransmissionPipeline(t)
	defer mr.Close()

	ctx := context.Background()

	// Queue multiple messages for user
	for i := 0; i < 3; i++ {
		msg := &Message{
			ID:   string(rune(i)),
			Type: MessageTypeCheckin,
			Data: json.RawMessage(`{"index": ` + string(rune(i)) + `}`),
		}
		err := pipeline.QueueMessage(ctx, "user456", msg)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond) // Small delay to ensure different timestamps
	}

	status, err := pipeline.GetUserQueueStatus(ctx, "user456")
	require.NoError(t, err)

	assert.Equal(t, "user456", status["user_id"])
	assert.Equal(t, int64(3), status["queue_size"])
	assert.NotNil(t, status["oldest_message"])
	assert.NotNil(t, status["newest_message"])
}

func TestConcurrentProcessing(t *testing.T) {
	pipeline, mr := setupTestRetransmissionPipeline(t)
	defer mr.Close()

	ctx := context.Background()
	pipeline.config.WorkerCount = 4

	var deliveredCount int32
	pipeline.SetDeliveryHandler(func(userID string, message *Message) error {
		atomic.AddInt32(&deliveredCount, 1)
		time.Sleep(10 * time.Millisecond) // Simulate processing time
		return nil
	})

	// Start pipeline
	err := pipeline.Start(ctx)
	require.NoError(t, err)
	defer pipeline.Stop()

	// Queue many messages
	for i := 0; i < 20; i++ {
		msg := &Message{
			ID:   string(rune(i)),
			Type: MessageTypeCheckin,
			Data: json.RawMessage(`{"index": ` + string(rune(i)) + `}`),
		}
		err := pipeline.QueueMessage(ctx, "user789", msg)
		require.NoError(t, err)
	}

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	// Check that all messages were delivered
	assert.Equal(t, int32(20), atomic.LoadInt32(&deliveredCount))
}

func TestReprocessDLQ(t *testing.T) {
	pipeline, mr := setupTestRetransmissionPipeline(t)
	defer mr.Close()

	ctx := context.Background()

	// Add messages to DLQ
	dlqKey := pipeline.deadLetterQueueKey()
	for i := 0; i < 5; i++ {
		msg := &QueuedMessage{
			ID:     string(rune(i)),
			UserID: "user_dlq",
			Message: &Message{
				ID:   string(rune(i)),
				Type: MessageTypeCheckin,
				Data: json.RawMessage(`{"dlq": true}`),
			},
			Status:     StatusFailed,
			RetryCount: 3,
			CreatedAt:  time.Now(),
		}

		data, _ := json.Marshal(msg)
		pipeline.redis.ZAdd(ctx, dlqKey, redis.Z{
			Score:  float64(time.Now().Unix()),
			Member: data,
		})
	}

	// Verify DLQ has messages
	dlqCount, _ := pipeline.redis.ZCard(ctx, dlqKey).Result()
	assert.Equal(t, int64(5), dlqCount)

	// Reprocess DLQ
	reprocessed, err := pipeline.ReprocessDLQ(ctx, 3)
	require.NoError(t, err)
	assert.Equal(t, 3, reprocessed)

	// Verify messages moved from DLQ to main queue
	dlqCount, _ = pipeline.redis.ZCard(ctx, dlqKey).Result()
	assert.Equal(t, int64(2), dlqCount) // 5 - 3 = 2 remaining

	mainCount, _ := pipeline.redis.ZCard(ctx, pipeline.queueKey()).Result()
	assert.Equal(t, int64(3), mainCount)
}

func TestCalculateBackoff(t *testing.T) {
	pipeline, _ := setupTestRetransmissionPipeline(t)

	// Test exponential backoff
	backoff1 := pipeline.calculateBackoff(1)
	assert.Equal(t, 100*time.Millisecond, backoff1)

	backoff2 := pipeline.calculateBackoff(2)
	assert.Equal(t, 200*time.Millisecond, backoff2)

	backoff3 := pipeline.calculateBackoff(3)
	assert.Equal(t, 400*time.Millisecond, backoff3)

	// Test max backoff cap
	backoff10 := pipeline.calculateBackoff(10)
	assert.Equal(t, 500*time.Millisecond, backoff10) // Capped at MaxRetryInterval
}

func TestGetMetrics(t *testing.T) {
	pipeline, mr := setupTestRetransmissionPipeline(t)
	defer mr.Close()

	// Set some metric values
	atomic.StoreUint64(&pipeline.messagesQueued, 100)
	atomic.StoreUint64(&pipeline.messagesDelivered, 90)
	atomic.StoreUint64(&pipeline.messagesFailed, 5)
	atomic.StoreUint64(&pipeline.messagesExpired, 5)
	atomic.StoreUint64(&pipeline.retransmissions, 15)

	metrics := pipeline.GetMetrics()

	assert.Equal(t, uint64(100), metrics["messages_queued"])
	assert.Equal(t, uint64(90), metrics["messages_delivered"])
	assert.Equal(t, uint64(5), metrics["messages_failed"])
	assert.Equal(t, uint64(5), metrics["messages_expired"])
	assert.Equal(t, uint64(15), metrics["retransmissions"])

	// Check delivery rate calculation
	expectedRate := 90.0 / 95.0 * 100.0 // 90 delivered / (90 + 5 failed)
	assert.InDelta(t, expectedRate, metrics["delivery_rate"], 0.01)
}

func BenchmarkQueueMessage(b *testing.B) {
	pipeline, mr := setupTestRetransmissionPipeline(b)
	defer mr.Close()

	ctx := context.Background()
	msg := &Message{
		ID:   "bench_msg",
		Type: MessageTypeCheckin,
		Data: json.RawMessage(`{"bench": true}`),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pipeline.QueueMessage(ctx, "bench_user", msg)
	}
}

func BenchmarkProcessBatch(b *testing.B) {
	pipeline, mr := setupTestRetransmissionPipeline(b)
	defer mr.Close()

	ctx := context.Background()

	// Setup simple delivery handler
	pipeline.SetDeliveryHandler(func(userID string, message *Message) error {
		return nil
	})

	// Pre-populate queue
	for i := 0; i < 100; i++ {
		msg := &Message{
			ID:   string(rune(i)),
			Type: MessageTypeCheckin,
			Data: json.RawMessage(`{"index": ` + string(rune(i)) + `}`),
		}
		_ = pipeline.QueueMessage(ctx, "bench_user", msg)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = pipeline.ProcessBatch(ctx)
	}
}