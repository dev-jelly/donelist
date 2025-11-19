package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// OfflineQueueConfig contains configuration for offline message queue
type OfflineQueueConfig struct {
	RedisClient    *redis.Client
	MaxMessages    int64         // Max messages per user
	RetentionTime  time.Duration // How long to keep messages
	Logger         *zap.Logger
}

// OfflineQueue manages offline messages using Redis Streams
type OfflineQueue struct {
	redis          *redis.Client
	maxMessages    int64
	retentionTime  time.Duration
	logger         *zap.Logger
}

// NewOfflineQueue creates a new offline message queue
func NewOfflineQueue(cfg OfflineQueueConfig) *OfflineQueue {
	if cfg.MaxMessages == 0 {
		cfg.MaxMessages = 1000 // Default: 1000 messages per user
	}
	if cfg.RetentionTime == 0 {
		cfg.RetentionTime = 7 * 24 * time.Hour // Default: 7 days
	}

	return &OfflineQueue{
		redis:         cfg.RedisClient,
		maxMessages:   cfg.MaxMessages,
		retentionTime: cfg.RetentionTime,
		logger:        cfg.Logger,
	}
}

// StreamKey returns the Redis stream key for a user
func (q *OfflineQueue) streamKey(userID string) string {
	return fmt.Sprintf("offline:messages:%s", userID)
}

// AddMessage adds a message to user's offline queue
func (q *OfflineQueue) AddMessage(ctx context.Context, userID string, message *Message) error {
	// Serialize message
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	streamKey := q.streamKey(userID)

	// Add to stream with automatic ID
	args := &redis.XAddArgs{
		Stream: streamKey,
		MaxLen: q.maxMessages, // Limit stream size
		Values: map[string]interface{}{
			"message": data,
			"timestamp": time.Now().Unix(),
		},
	}

	_, err = q.redis.XAdd(ctx, args).Result()
	if err != nil {
		return fmt.Errorf("failed to add message to stream: %w", err)
	}

	// Set expiration on the stream
	q.redis.Expire(ctx, streamKey, q.retentionTime)

	q.logger.Debug("Added offline message",
		zap.String("user_id", userID),
		zap.String("message_type", string(message.Type)),
	)

	return nil
}

// GetMessages retrieves all offline messages for a user
func (q *OfflineQueue) GetMessages(ctx context.Context, userID string) ([]*Message, error) {
	streamKey := q.streamKey(userID)

	// Read all messages from stream
	messages, err := q.redis.XRange(ctx, streamKey, "-", "+").Result()
	if err != nil {
		if err == redis.Nil {
			return []*Message{}, nil
		}
		return nil, fmt.Errorf("failed to read messages: %w", err)
	}

	result := make([]*Message, 0, len(messages))
	for _, msg := range messages {
		if data, ok := msg.Values["message"].(string); ok {
			var message Message
			if err := json.Unmarshal([]byte(data), &message); err != nil {
				q.logger.Warn("Failed to unmarshal offline message",
					zap.Error(err),
					zap.String("message_id", msg.ID),
				)
				continue
			}
			result = append(result, &message)
		}
	}

	q.logger.Debug("Retrieved offline messages",
		zap.String("user_id", userID),
		zap.Int("count", len(result)),
	)

	return result, nil
}

// DeliverMessages retrieves and removes offline messages for a user
func (q *OfflineQueue) DeliverMessages(ctx context.Context, userID string) ([]*Message, error) {
	// Get messages
	messages, err := q.GetMessages(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Clear the stream after delivery
	if len(messages) > 0 {
		streamKey := q.streamKey(userID)
		if err := q.redis.Del(ctx, streamKey).Err(); err != nil {
			q.logger.Warn("Failed to clear offline messages",
				zap.Error(err),
				zap.String("user_id", userID),
			)
		}
	}

	return messages, nil
}

// GetMessageCount returns the number of offline messages for a user
func (q *OfflineQueue) GetMessageCount(ctx context.Context, userID string) (int64, error) {
	streamKey := q.streamKey(userID)

	count, err := q.redis.XLen(ctx, streamKey).Result()
	if err != nil {
		if err == redis.Nil {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to get message count: %w", err)
	}

	return count, nil
}

// ClearMessages removes all offline messages for a user
func (q *OfflineQueue) ClearMessages(ctx context.Context, userID string) error {
	streamKey := q.streamKey(userID)

	err := q.redis.Del(ctx, streamKey).Err()
	if err != nil && err != redis.Nil {
		return fmt.Errorf("failed to clear messages: %w", err)
	}

	q.logger.Debug("Cleared offline messages",
		zap.String("user_id", userID),
	)

	return nil
}

// CleanupOldMessages removes expired messages from all users
func (q *OfflineQueue) CleanupOldMessages(ctx context.Context) error {
	// This would typically be run as a periodic background task
	// Implementation depends on how you track user streams

	// For now, we rely on Redis TTL to automatically remove old streams
	// More sophisticated cleanup could scan for old messages within streams

	return nil
}

// BroadcastOfflineToRoom adds an offline message for all room members
func (q *OfflineQueue) BroadcastOfflineToRoom(ctx context.Context, room *Room, message *Message, excludeUserID string) error {
	room.mu.RLock()
	userIDs := make([]string, 0)
	for client := range room.Clients {
		if client.userID != excludeUserID {
			userIDs = append(userIDs, client.userID)
		}
	}
	room.mu.RUnlock()

	// Add message for each offline user in the room
	// In production, you'd check who's actually offline
	for _, userID := range userIDs {
		if err := q.AddMessage(ctx, userID, message); err != nil {
			q.logger.Warn("Failed to add offline message for user",
				zap.Error(err),
				zap.String("user_id", userID),
				zap.String("room_id", room.ID),
			)
		}
	}

	return nil
}