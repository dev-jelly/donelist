package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// DeliveryStatus represents the status of message delivery
type DeliveryStatus string

const (
	StatusPending      DeliveryStatus = "pending"
	StatusInFlight     DeliveryStatus = "in_flight"
	StatusDelivered    DeliveryStatus = "delivered"
	StatusFailed       DeliveryStatus = "failed"
	StatusAcknowledged DeliveryStatus = "acknowledged"
	StatusExpired      DeliveryStatus = "expired"
)

// RetransmissionConfig contains configuration for retransmission pipeline
type RetransmissionConfig struct {
	MaxRetries        int           // Maximum number of retry attempts
	RetryInterval     time.Duration // Initial retry interval
	MaxRetryInterval  time.Duration // Maximum retry interval (exponential backoff)
	AckTimeout        time.Duration // Timeout waiting for acknowledgment
	MessageTTL        time.Duration // Message time-to-live
	BatchSize         int           // Batch size for processing
	WorkerCount       int           // Number of concurrent workers
	EnableCompression bool          // Enable message compression
}

// DefaultRetransmissionConfig returns default configuration
func DefaultRetransmissionConfig() RetransmissionConfig {
	return RetransmissionConfig{
		MaxRetries:       3,
		RetryInterval:    1 * time.Second,
		MaxRetryInterval: 30 * time.Second,
		AckTimeout:       10 * time.Second,
		MessageTTL:       24 * time.Hour,
		BatchSize:        100,
		WorkerCount:      4,
		EnableCompression: false,
	}
}

// QueuedMessage represents a message in the retransmission queue
type QueuedMessage struct {
	ID             string         `json:"id"`
	UserID         string         `json:"user_id"`
	Message        *Message       `json:"message"`
	Status         DeliveryStatus `json:"status"`
	RetryCount     int            `json:"retry_count"`
	NextRetryAt    time.Time      `json:"next_retry_at"`
	CreatedAt      time.Time      `json:"created_at"`
	LastAttemptAt  *time.Time     `json:"last_attempt_at,omitempty"`
	DeliveredAt    *time.Time     `json:"delivered_at,omitempty"`
	AcknowledgedAt *time.Time     `json:"acknowledged_at,omitempty"`
	Error          string         `json:"error,omitempty"`
}

// RetransmissionPipeline manages at-least-once message delivery
type RetransmissionPipeline struct {
	redis   *redis.Client
	config  RetransmissionConfig
	logger  *zap.Logger

	// Worker management
	workers    []*retransmissionWorker
	workerPool chan struct{}
	shutdown   chan struct{}
	wg         sync.WaitGroup

	// Metrics
	messagesQueued     uint64
	messagesDelivered  uint64
	messagesFailed     uint64
	messagesExpired    uint64
	retransmissions    uint64

	// Delivery callbacks
	deliveryHandler func(userID string, message *Message) error
	mu              sync.RWMutex
}

// retransmissionWorker processes messages from the queue
type retransmissionWorker struct {
	id       int
	pipeline *RetransmissionPipeline
	shutdown chan struct{}
}

// NewRetransmissionPipeline creates a new retransmission pipeline
func NewRetransmissionPipeline(redis *redis.Client, config RetransmissionConfig, logger *zap.Logger) *RetransmissionPipeline {
	if config.WorkerCount == 0 {
		config.WorkerCount = 4
	}
	if config.BatchSize == 0 {
		config.BatchSize = 100
	}

	return &RetransmissionPipeline{
		redis:      redis,
		config:     config,
		logger:     logger,
		workerPool: make(chan struct{}, config.WorkerCount),
		shutdown:   make(chan struct{}),
		workers:    make([]*retransmissionWorker, 0, config.WorkerCount),
	}
}

// SetDeliveryHandler sets the function to call for message delivery
func (rp *RetransmissionPipeline) SetDeliveryHandler(handler func(userID string, message *Message) error) {
	rp.mu.Lock()
	defer rp.mu.Unlock()
	rp.deliveryHandler = handler
}

// Start starts the retransmission pipeline workers
func (rp *RetransmissionPipeline) Start(ctx context.Context) error {
	rp.logger.Info("Starting retransmission pipeline",
		zap.Int("workers", rp.config.WorkerCount),
		zap.Int("batch_size", rp.config.BatchSize),
	)

	// Start worker pool
	for i := 0; i < rp.config.WorkerCount; i++ {
		worker := &retransmissionWorker{
			id:       i,
			pipeline: rp,
			shutdown: make(chan struct{}),
		}
		rp.workers = append(rp.workers, worker)
		rp.wg.Add(1)
		go worker.run(ctx)
	}

	// Start monitoring goroutine
	rp.wg.Add(1)
	go rp.monitor(ctx)

	return nil
}

// Stop stops the retransmission pipeline
func (rp *RetransmissionPipeline) Stop() {
	rp.logger.Info("Stopping retransmission pipeline")

	close(rp.shutdown)

	// Stop all workers
	for _, worker := range rp.workers {
		close(worker.shutdown)
	}

	// Wait for all workers to finish
	rp.wg.Wait()

	rp.logger.Info("Retransmission pipeline stopped",
		zap.Uint64("messages_queued", atomic.LoadUint64(&rp.messagesQueued)),
		zap.Uint64("messages_delivered", atomic.LoadUint64(&rp.messagesDelivered)),
		zap.Uint64("messages_failed", atomic.LoadUint64(&rp.messagesFailed)),
	)
}

// QueueMessage adds a message to the retransmission queue
func (rp *RetransmissionPipeline) QueueMessage(ctx context.Context, userID string, message *Message) error {
	messageID := fmt.Sprintf("%s:%s:%d", userID, message.Type, time.Now().UnixNano())

	queuedMsg := &QueuedMessage{
		ID:          messageID,
		UserID:      userID,
		Message:     message,
		Status:      StatusPending,
		RetryCount:  0,
		NextRetryAt: time.Now(),
		CreatedAt:   time.Now(),
	}

	// Serialize the message
	data, err := json.Marshal(queuedMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Add to Redis sorted set (score is next retry timestamp)
	score := float64(queuedMsg.NextRetryAt.Unix())
	err = rp.redis.ZAdd(ctx, rp.queueKey(), redis.Z{
		Score:  score,
		Member: data,
	}).Err()

	if err != nil {
		return fmt.Errorf("failed to queue message: %w", err)
	}

	// Also add to user-specific queue for tracking
	userQueueKey := rp.userQueueKey(userID)
	err = rp.redis.ZAdd(ctx, userQueueKey, redis.Z{
		Score:  score,
		Member: messageID,
	}).Err()

	if err != nil {
		rp.logger.Warn("Failed to add message to user queue",
			zap.Error(err),
			zap.String("user_id", userID),
		)
	}

	// Set TTL on user queue
	rp.redis.Expire(ctx, userQueueKey, rp.config.MessageTTL)

	atomic.AddUint64(&rp.messagesQueued, 1)

	rp.logger.Debug("Message queued for retransmission",
		zap.String("message_id", messageID),
		zap.String("user_id", userID),
	)

	return nil
}

// ProcessBatch processes a batch of messages ready for delivery
func (rp *RetransmissionPipeline) ProcessBatch(ctx context.Context) (int, error) {
	now := time.Now()

	// Get messages ready for delivery (score <= now)
	messages, err := rp.redis.ZRangeByScoreWithScores(ctx, rp.queueKey(), &redis.ZRangeBy{
		Min:   "-inf",
		Max:   fmt.Sprintf("%d", now.Unix()),
		Count: int64(rp.config.BatchSize),
	}).Result()

	if err != nil {
		return 0, fmt.Errorf("failed to fetch messages: %w", err)
	}

	processed := 0
	for _, z := range messages {
		// Parse the message
		var queuedMsg QueuedMessage
		if err := json.Unmarshal([]byte(z.Member.(string)), &queuedMsg); err != nil {
			rp.logger.Error("Failed to unmarshal queued message",
				zap.Error(err),
			)
			// Remove corrupted message
			rp.redis.ZRem(ctx, rp.queueKey(), z.Member)
			continue
		}

		// Check if message has expired
		if now.Sub(queuedMsg.CreatedAt) > rp.config.MessageTTL {
			rp.handleExpiredMessage(ctx, &queuedMsg)
			processed++
			continue
		}

		// Attempt delivery
		if err := rp.attemptDelivery(ctx, &queuedMsg); err != nil {
			rp.handleDeliveryFailure(ctx, &queuedMsg, err)
		} else {
			rp.handleDeliverySuccess(ctx, &queuedMsg)
		}

		processed++
	}

	return processed, nil
}

// attemptDelivery attempts to deliver a message
func (rp *RetransmissionPipeline) attemptDelivery(ctx context.Context, msg *QueuedMessage) error {
	rp.mu.RLock()
	handler := rp.deliveryHandler
	rp.mu.RUnlock()

	if handler == nil {
		return fmt.Errorf("no delivery handler configured")
	}

	now := time.Now()
	msg.LastAttemptAt = &now
	msg.Status = StatusInFlight

	// Call the delivery handler
	err := handler(msg.UserID, msg.Message)
	if err != nil {
		return err
	}

	deliveredAt := time.Now()
	msg.DeliveredAt = &deliveredAt
	msg.Status = StatusDelivered

	return nil
}

// handleDeliverySuccess handles successful message delivery
func (rp *RetransmissionPipeline) handleDeliverySuccess(ctx context.Context, msg *QueuedMessage) {
	// Remove from main queue
	data, _ := json.Marshal(msg)
	rp.redis.ZRem(ctx, rp.queueKey(), data)

	// Update user queue
	rp.redis.ZRem(ctx, rp.userQueueKey(msg.UserID), msg.ID)

	// Store delivery confirmation for audit
	confirmationKey := fmt.Sprintf("delivery:confirmed:%s", msg.ID)
	confirmationData, _ := json.Marshal(map[string]interface{}{
		"message_id":   msg.ID,
		"user_id":      msg.UserID,
		"delivered_at": msg.DeliveredAt,
		"retry_count":  msg.RetryCount,
	})
	rp.redis.Set(ctx, confirmationKey, confirmationData, 24*time.Hour)

	atomic.AddUint64(&rp.messagesDelivered, 1)

	rp.logger.Debug("Message delivered successfully",
		zap.String("message_id", msg.ID),
		zap.String("user_id", msg.UserID),
		zap.Int("retry_count", msg.RetryCount),
	)
}

// handleDeliveryFailure handles failed message delivery
func (rp *RetransmissionPipeline) handleDeliveryFailure(ctx context.Context, msg *QueuedMessage, err error) {
	msg.RetryCount++
	msg.Status = StatusFailed
	msg.Error = err.Error()

	// Check if we've exceeded max retries
	if msg.RetryCount >= rp.config.MaxRetries {
		rp.handleMaxRetriesExceeded(ctx, msg)
		return
	}

	// Calculate next retry time with exponential backoff
	backoff := rp.calculateBackoff(msg.RetryCount)
	msg.NextRetryAt = time.Now().Add(backoff)

	// Update the message in queue
	oldData, _ := json.Marshal(msg)
	rp.redis.ZRem(ctx, rp.queueKey(), oldData)

	newData, _ := json.Marshal(msg)
	score := float64(msg.NextRetryAt.Unix())
	rp.redis.ZAdd(ctx, rp.queueKey(), redis.Z{
		Score:  score,
		Member: newData,
	})

	atomic.AddUint64(&rp.retransmissions, 1)

	rp.logger.Warn("Message delivery failed, scheduling retry",
		zap.String("message_id", msg.ID),
		zap.String("user_id", msg.UserID),
		zap.Error(err),
		zap.Int("retry_count", msg.RetryCount),
		zap.Time("next_retry_at", msg.NextRetryAt),
	)
}

// handleMaxRetriesExceeded handles messages that exceeded max retries
func (rp *RetransmissionPipeline) handleMaxRetriesExceeded(ctx context.Context, msg *QueuedMessage) {
	// Move to dead letter queue
	dlqKey := rp.deadLetterQueueKey()
	data, _ := json.Marshal(msg)
	rp.redis.ZAdd(ctx, dlqKey, redis.Z{
		Score:  float64(time.Now().Unix()),
		Member: data,
	})

	// Remove from main queue
	rp.redis.ZRem(ctx, rp.queueKey(), data)
	rp.redis.ZRem(ctx, rp.userQueueKey(msg.UserID), msg.ID)

	atomic.AddUint64(&rp.messagesFailed, 1)

	rp.logger.Error("Message exceeded max retries, moved to DLQ",
		zap.String("message_id", msg.ID),
		zap.String("user_id", msg.UserID),
		zap.Int("retry_count", msg.RetryCount),
		zap.String("last_error", msg.Error),
	)
}

// handleExpiredMessage handles expired messages
func (rp *RetransmissionPipeline) handleExpiredMessage(ctx context.Context, msg *QueuedMessage) {
	msg.Status = StatusExpired

	// Remove from queues
	data, _ := json.Marshal(msg)
	rp.redis.ZRem(ctx, rp.queueKey(), data)
	rp.redis.ZRem(ctx, rp.userQueueKey(msg.UserID), msg.ID)

	// Store in expired queue for audit
	expiredKey := fmt.Sprintf("messages:expired:%s", msg.UserID)
	rp.redis.ZAdd(ctx, expiredKey, redis.Z{
		Score:  float64(time.Now().Unix()),
		Member: msg.ID,
	})
	rp.redis.Expire(ctx, expiredKey, 7*24*time.Hour)

	atomic.AddUint64(&rp.messagesExpired, 1)

	rp.logger.Info("Message expired",
		zap.String("message_id", msg.ID),
		zap.String("user_id", msg.UserID),
		zap.Duration("age", time.Since(msg.CreatedAt)),
	)
}

// AcknowledgeMessage marks a message as acknowledged by the client
func (rp *RetransmissionPipeline) AcknowledgeMessage(ctx context.Context, messageID string) error {
	// Update acknowledgment status
	ackKey := fmt.Sprintf("ack:%s", messageID)
	ackData := map[string]interface{}{
		"message_id":      messageID,
		"acknowledged_at": time.Now().Unix(),
	}

	data, _ := json.Marshal(ackData)
	err := rp.redis.Set(ctx, ackKey, data, 24*time.Hour).Err()

	if err != nil {
		return fmt.Errorf("failed to acknowledge message: %w", err)
	}

	rp.logger.Debug("Message acknowledged",
		zap.String("message_id", messageID),
	)

	return nil
}

// GetUserQueueStatus returns the status of a user's message queue
func (rp *RetransmissionPipeline) GetUserQueueStatus(ctx context.Context, userID string) (map[string]interface{}, error) {
	userQueueKey := rp.userQueueKey(userID)

	// Get queue size
	queueSize, err := rp.redis.ZCard(ctx, userQueueKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get queue size: %w", err)
	}

	// Get oldest and newest message timestamps
	oldestMessages, _ := rp.redis.ZRangeWithScores(ctx, userQueueKey, 0, 0).Result()
	newestMessages, _ := rp.redis.ZRangeWithScores(ctx, userQueueKey, -1, -1).Result()

	var oldestTimestamp, newestTimestamp *time.Time
	if len(oldestMessages) > 0 {
		ts := time.Unix(int64(oldestMessages[0].Score), 0)
		oldestTimestamp = &ts
	}
	if len(newestMessages) > 0 {
		ts := time.Unix(int64(newestMessages[0].Score), 0)
		newestTimestamp = &ts
	}

	return map[string]interface{}{
		"user_id":          userID,
		"queue_size":       queueSize,
		"oldest_message":   oldestTimestamp,
		"newest_message":   newestTimestamp,
	}, nil
}

// GetMetrics returns pipeline metrics
func (rp *RetransmissionPipeline) GetMetrics() map[string]interface{} {
	return map[string]interface{}{
		"messages_queued":     atomic.LoadUint64(&rp.messagesQueued),
		"messages_delivered":  atomic.LoadUint64(&rp.messagesDelivered),
		"messages_failed":     atomic.LoadUint64(&rp.messagesFailed),
		"messages_expired":    atomic.LoadUint64(&rp.messagesExpired),
		"retransmissions":     atomic.LoadUint64(&rp.retransmissions),
		"delivery_rate":       rp.calculateDeliveryRate(),
		"worker_count":        len(rp.workers),
	}
}

// calculateBackoff calculates exponential backoff duration
func (rp *RetransmissionPipeline) calculateBackoff(retryCount int) time.Duration {
	backoff := rp.config.RetryInterval * time.Duration(1<<uint(retryCount-1))
	if backoff > rp.config.MaxRetryInterval {
		backoff = rp.config.MaxRetryInterval
	}
	return backoff
}

// calculateDeliveryRate calculates the delivery success rate
func (rp *RetransmissionPipeline) calculateDeliveryRate() float64 {
	delivered := atomic.LoadUint64(&rp.messagesDelivered)
	failed := atomic.LoadUint64(&rp.messagesFailed)
	total := delivered + failed
	if total == 0 {
		return 100.0
	}
	return float64(delivered) / float64(total) * 100.0
}

// monitor runs periodic monitoring tasks
func (rp *RetransmissionPipeline) monitor(ctx context.Context) {
	defer rp.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-rp.shutdown:
			return
		case <-ticker.C:
			metrics := rp.GetMetrics()
			rp.logger.Info("Retransmission pipeline metrics",
				zap.Any("metrics", metrics),
			)
		}
	}
}

// Worker methods
func (w *retransmissionWorker) run(ctx context.Context) {
	defer w.pipeline.wg.Done()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.shutdown:
			return
		case <-ticker.C:
			// Try to acquire worker slot
			select {
			case w.pipeline.workerPool <- struct{}{}:
				// Process batch
				processed, err := w.pipeline.ProcessBatch(ctx)
				if err != nil {
					w.pipeline.logger.Error("Worker failed to process batch",
						zap.Int("worker_id", w.id),
						zap.Error(err),
					)
				} else if processed > 0 {
					w.pipeline.logger.Debug("Worker processed batch",
						zap.Int("worker_id", w.id),
						zap.Int("processed", processed),
					)
				}
				// Release worker slot
				<-w.pipeline.workerPool
			default:
				// Worker pool is full, skip this tick
			}
		}
	}
}

// Redis key helpers
func (rp *RetransmissionPipeline) queueKey() string {
	return "retransmission:queue"
}

func (rp *RetransmissionPipeline) userQueueKey(userID string) string {
	return fmt.Sprintf("retransmission:user:%s", userID)
}

func (rp *RetransmissionPipeline) deadLetterQueueKey() string {
	return "retransmission:dlq"
}

// ReprocessDLQ reprocesses messages from dead letter queue
func (rp *RetransmissionPipeline) ReprocessDLQ(ctx context.Context, limit int) (int, error) {
	dlqKey := rp.deadLetterQueueKey()

	// Get messages from DLQ
	messages, err := rp.redis.ZRange(ctx, dlqKey, 0, int64(limit-1)).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to fetch DLQ messages: %w", err)
	}

	reprocessed := 0
	for _, data := range messages {
		var msg QueuedMessage
		if err := json.Unmarshal([]byte(data), &msg); err != nil {
			continue
		}

		// Reset retry count and requeue
		msg.RetryCount = 0
		msg.Status = StatusPending
		msg.NextRetryAt = time.Now()
		msg.Error = ""

		if err := rp.QueueMessage(ctx, msg.UserID, msg.Message); err != nil {
			rp.logger.Error("Failed to requeue DLQ message",
				zap.String("message_id", msg.ID),
				zap.Error(err),
			)
			continue
		}

		// Remove from DLQ
		rp.redis.ZRem(ctx, dlqKey, data)
		reprocessed++
	}

	rp.logger.Info("Reprocessed DLQ messages",
		zap.Int("reprocessed", reprocessed),
	)

	return reprocessed, nil
}