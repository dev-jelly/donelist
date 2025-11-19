package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// RedisBridge handles Redis Pub/Sub for multi-node WebSocket fanout
type RedisBridge struct {
	redisClient *redis.Client
	hub         *Hub
	logger      *zap.Logger
	topology    *RoomTopology

	// Subscription management
	pubsub      *redis.PubSub
	subscribers map[string]bool // Track subscribed channels
	mu          sync.RWMutex

	// Context for graceful shutdown
	ctx    context.Context
	cancel context.CancelFunc
}

// RedisBridgeConfig contains configuration for the Redis bridge
type RedisBridgeConfig struct {
	RedisClient *redis.Client
	Hub         *Hub
	Logger      *zap.Logger
}

// NewRedisBridge creates a new Redis bridge for multi-node support
func NewRedisBridge(config RedisBridgeConfig) *RedisBridge {
	ctx, cancel := context.WithCancel(context.Background())

	return &RedisBridge{
		redisClient: config.RedisClient,
		hub:         config.Hub,
		logger:      config.Logger,
		topology:    NewRoomTopology(),
		subscribers: make(map[string]bool),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start begins listening for Redis Pub/Sub messages
func (rb *RedisBridge) Start() error {
	rb.mu.Lock()
	rb.pubsub = rb.redisClient.PSubscribe(rb.ctx, "ws:room:*")
	rb.mu.Unlock()

	// Wait for subscription confirmation
	_, err := rb.pubsub.Receive(rb.ctx)
	if err != nil {
		return fmt.Errorf("failed to subscribe to Redis: %w", err)
	}

	rb.logger.Info("Redis bridge started, listening for messages")

	// Start message processing loop
	go rb.processMessages()

	return nil
}

// Stop gracefully shuts down the Redis bridge
func (rb *RedisBridge) Stop() error {
	rb.logger.Info("Stopping Redis bridge")
	rb.cancel()

	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.pubsub != nil {
		return rb.pubsub.Close()
	}

	return nil
}

// processMessages processes incoming Redis Pub/Sub messages
func (rb *RedisBridge) processMessages() {
	ch := rb.pubsub.Channel()

	for {
		select {
		case <-rb.ctx.Done():
			rb.logger.Info("Redis bridge message processor stopped")
			return

		case msg := <-ch:
			if msg == nil {
				continue
			}

			rb.handleRedisMessage(msg)
		}
	}
}

// handleRedisMessage processes a single Redis message
func (rb *RedisBridge) handleRedisMessage(msg *redis.Message) {
	// Parse room ID from channel
	// Channel format: "ws:room:{roomID}"
	roomID := msg.Channel[8:] // Remove "ws:room:" prefix

	// Unmarshal message
	var wsMsg Message
	if err := json.Unmarshal([]byte(msg.Payload), &wsMsg); err != nil {
		rb.logger.Error("Failed to unmarshal Redis message",
			zap.Error(err),
			zap.String("channel", msg.Channel),
		)
		return
	}

	// Get the room
	roomManager := rb.hub.GetRoomManager()
	room, exists := roomManager.GetRoom(roomID)
	if !exists {
		rb.logger.Debug("Room not found for Redis message",
			zap.String("room_id", roomID),
		)
		return
	}

	// Broadcast to local clients in the room
	data, err := wsMsg.Marshal()
	if err != nil {
		rb.logger.Error("Failed to marshal message for broadcast",
			zap.Error(err),
		)
		return
	}

	select {
	case room.Broadcast <- data:
		rb.logger.Debug("Broadcasted Redis message to room",
			zap.String("room_id", roomID),
			zap.String("type", string(wsMsg.Type)),
		)
	default:
		rb.logger.Warn("Room broadcast channel full",
			zap.String("room_id", roomID),
		)
	}
}

// PublishToRoom publishes a message to a specific room via Redis
func (rb *RedisBridge) PublishToRoom(ctx context.Context, roomID string, message *Message) error {
	channel := fmt.Sprintf("ws:room:%s", roomID)

	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Publish to Redis
	err = rb.redisClient.Publish(ctx, channel, data).Err()
	if err != nil {
		return fmt.Errorf("failed to publish to Redis: %w", err)
	}

	rb.logger.Debug("Published message to Redis",
		zap.String("channel", channel),
		zap.String("type", string(message.Type)),
	)

	return nil
}

// PublishToUser publishes a message to all of a user's rooms
func (rb *RedisBridge) PublishToUser(ctx context.Context, userID string, teamIDs []string, deviceID string, message *Message) error {
	rooms := rb.topology.GetUserRooms(userID, teamIDs, deviceID)

	for _, roomID := range rooms {
		if err := rb.PublishToRoom(ctx, roomID, message); err != nil {
			rb.logger.Error("Failed to publish to user room",
				zap.Error(err),
				zap.String("room_id", roomID),
			)
			// Continue with other rooms even if one fails
		}
	}

	return nil
}

// SubscribeToRoom subscribes to a specific room channel
func (rb *RedisBridge) SubscribeToRoom(roomID string) error {
	channel := fmt.Sprintf("ws:room:%s", roomID)

	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.subscribers[channel] {
		return nil // Already subscribed
	}

	if rb.pubsub == nil {
		return fmt.Errorf("pubsub not initialized")
	}

	err := rb.pubsub.Subscribe(rb.ctx, channel)
	if err != nil {
		return fmt.Errorf("failed to subscribe to channel %s: %w", channel, err)
	}

	rb.subscribers[channel] = true
	rb.logger.Info("Subscribed to room channel",
		zap.String("channel", channel),
	)

	return nil
}

// UnsubscribeFromRoom unsubscribes from a specific room channel
func (rb *RedisBridge) UnsubscribeFromRoom(roomID string) error {
	channel := fmt.Sprintf("ws:room:%s", roomID)

	rb.mu.Lock()
	defer rb.mu.Unlock()

	if !rb.subscribers[channel] {
		return nil // Not subscribed
	}

	if rb.pubsub == nil {
		return fmt.Errorf("pubsub not initialized")
	}

	err := rb.pubsub.Unsubscribe(rb.ctx, channel)
	if err != nil {
		return fmt.Errorf("failed to unsubscribe from channel %s: %w", channel, err)
	}

	delete(rb.subscribers, channel)
	rb.logger.Info("Unsubscribed from room channel",
		zap.String("channel", channel),
	)

	return nil
}

// GetSubscriberCount returns the number of subscribers on a channel
func (rb *RedisBridge) GetSubscriberCount(ctx context.Context, roomID string) (int64, error) {
	channel := fmt.Sprintf("ws:room:%s", roomID)

	result, err := rb.redisClient.PubSubNumSub(ctx, channel).Result()
	if err != nil {
		return 0, err
	}

	if count, ok := result[channel]; ok {
		return count, nil
	}

	return 0, nil
}

// HealthCheck verifies the Redis connection is healthy
func (rb *RedisBridge) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	return rb.redisClient.Ping(ctx).Err()
}

// GetStats returns statistics about the bridge
func (rb *RedisBridge) GetStats() map[string]interface{} {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	return map[string]interface{}{
		"subscribed_channels": len(rb.subscribers),
		"channels":            rb.getChannelList(),
	}
}

func (rb *RedisBridge) getChannelList() []string {
	channels := make([]string, 0, len(rb.subscribers))
	for channel := range rb.subscribers {
		channels = append(channels, channel)
	}
	return channels
}
