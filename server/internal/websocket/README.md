# WebSocket Real-time System

A production-ready WebSocket implementation for real-time communication in the Donelist application.

## Features

### Core Capabilities
- **Hub Pattern**: Centralized client management with efficient broadcasting
- **Room/Channel System**: Support for team collaboration and topic-based messaging
- **JWT Authentication**: Secure WebSocket connections with token-based auth
- **Presence Tracking**: Real-time user status (online, away, offline)
- **Typing Indicators**: Show when users are typing in rooms
- **Offline Message Queue**: Redis Streams-based message persistence for offline users
- **Reconnection Logic**: Exponential backoff with session recovery
- **Multi-node Scaling**: Redis Pub/Sub bridge for horizontal scaling

### Advanced Features
- **Message Schema Versioning**: Forward-compatible message envelope system
- **Message Validation**: Comprehensive payload validation with metrics
- **Room Topology**: Authorization framework for user/team/device rooms
- **Subscription Management**: Intelligent room membership tracking
- **Load Tested**: Handles 1000+ concurrent connections with 99K+ msg/sec throughput

## Architecture

### Components

#### Hub
Central coordinator that manages all WebSocket clients and message broadcasting.

```go
hub := NewHub(logger)
go hub.Run()

// Broadcast to all clients
hub.BroadcastToAll(message)

// Broadcast to specific user
hub.BroadcastToUser(userID, message)
```

#### Client
Represents a single WebSocket connection with read/write pumps.

```go
client := NewClient(conn, hub, userID, username, logger)
hub.Register(client)
client.Start() // Starts read and write pumps
```

#### Room
Channel-based communication for groups of users.

```go
room, err := roomManager.CreateRoom(roomID, name, owner)
room.Join <- client
room.Broadcast <- messageData
room.Leave <- client
```

#### Room Topology
Defines room types and access control:
- **User Rooms**: Personal rooms for each user (`user:{userID}`)
- **Team Rooms**: Shared rooms for teams (`team:{teamID}`)
- **Device Rooms**: Device-specific rooms (`device:{userID}:{deviceID}`)
- **User-Team Rooms**: User's view of team data (`user:{userID}:team:{teamID}`)

```go
topology := NewRoomTopology()
rooms := topology.GetUserRooms(userID, teamIDs, deviceID)
allowed := topology.ValidateRoomAccess(userID, teamIDs, roomID)
```

#### Subscription Manager
Manages room subscriptions with authorization and Redis integration.

```go
subscriptionMgr := NewSubscriptionManager(config)
resp, err := subscriptionMgr.JoinRoom(ctx, client, request)
err = subscriptionMgr.BroadcastToRoom(ctx, roomID, message)
```

#### Offline Queue
Redis Streams-based queue for storing messages when users are offline.

```go
queue := NewOfflineQueue(config)
err := queue.AddMessage(ctx, userID, message)
messages, err := queue.DeliverMessages(ctx, userID)
```

#### Reconnect Manager
Handles reconnection logic with exponential backoff.

```go
reconnectMgr := NewReconnectManager(config, offlineQueue, logger)
sessionID := reconnectMgr.OnClientDisconnect(userID)
session, err := reconnectMgr.OnClientReconnect(sessionID, userID)
```

#### Redis Bridge
Enables multi-node deployments with Redis Pub/Sub.

```go
bridge := NewRedisBridge(config)
err := bridge.Start()
err := bridge.PublishToRoom(ctx, roomID, message)
```

### Message Schema

#### Message Envelope (v1)
```json
{
  "version": "v1",
  "eventType": "checkin.created",
  "partitionKey": "user:123",
  "seq": 42,
  "ts": 1699896000000,
  "actorId": "user:123",
  "payload": {...},
  "metadata": {}
}
```

#### Supported Event Types
- `checkin.created`, `checkin.updated`, `checkin.deleted`
- `timeline.update`
- `room.join`, `room.leave`, `room.message`
- `typing.start`, `typing.stop`
- `presence.update`, `status.update`
- `ping`, `pong`
- `error`

#### Legacy Message Format
```json
{
  "type": "room_message",
  "room_id": "room-1",
  "user_id": "user-123",
  "timestamp": "2024-11-13T10:00:00Z",
  "payload": {
    "content": "Hello, world!",
    "username": "alice"
  }
}
```

## Usage

### Client Connection

```go
// Upgrade HTTP connection to WebSocket
conn, err := websocket.Upgrader.Upgrade(w, r, nil)

// Authenticate (JWT from query param or header)
userID, username, err := AuthenticateWebSocket(r, jwtManager)

// Create and register client
client := NewClient(conn, hub, userID, username, logger)
hub.Register(client)
client.Start()
```

### Room Management

```go
// Join a room
joinMsg := Message{
    Type: "join_room",
    Payload: map[string]interface{}{
        "room_id": "room-1",
    },
}
conn.WriteJSON(joinMsg)

// Send room message
roomMsg := Message{
    Type: "room_message",
    Payload: map[string]interface{}{
        "room_id": "room-1",
        "content": "Hello everyone!",
    },
}
conn.WriteJSON(roomMsg)

// Leave room
leaveMsg := Message{
    Type: "leave_room",
    Payload: map[string]interface{}{
        "room_id": "room-1",
    },
}
conn.WriteJSON(leaveMsg)
```

### Presence & Typing

```go
// Update status
statusMsg := Message{
    Type: "update_status",
    Payload: map[string]interface{}{
        "status": "away",
    },
}
conn.WriteJSON(statusMsg)

// Start typing
typingMsg := Message{
    Type: "typing_start",
    Payload: map[string]interface{}{
        "room_id": "room-1",
    },
}
conn.WriteJSON(typingMsg)
```

## Configuration

### Basic Setup

```go
import (
    "github.com/dev-jelly/donelist/internal/websocket"
    "go.uber.org/zap"
)

logger := zap.NewProduction()
hub := websocket.NewHub(logger)
go hub.Run()
```

### With Redis (Multi-node)

```go
import (
    "github.com/redis/go-redis/v9"
)

// Setup Redis client
redisClient := redis.NewClient(&redis.Options{
    Addr: "localhost:6379",
})

// Create offline queue
offlineQueue := websocket.NewOfflineQueue(websocket.OfflineQueueConfig{
    RedisClient:   redisClient,
    MaxMessages:   1000,
    RetentionTime: 7 * 24 * time.Hour,
    Logger:        logger,
})

// Create Redis bridge
bridge := websocket.NewRedisBridge(websocket.RedisBridgeConfig{
    RedisClient: redisClient,
    Hub:         hub,
    Logger:      logger,
})
err := bridge.Start()

// Create reconnect manager
reconnectMgr := websocket.NewReconnectManager(
    websocket.DefaultReconnectConfig(),
    offlineQueue,
    logger,
)

// Create subscription manager
subscriptionMgr := websocket.NewSubscriptionManager(websocket.SubscriptionConfig{
    Hub:         hub,
    RoomManager: hub.GetRoomManager(),
    Bridge:      bridge,
    Logger:      logger,
})
```

## Performance

### Load Test Results

With 1000 concurrent connections:
- **Registration Time**: ~500ms for 1000 clients
- **Throughput**: ~100,000 messages/second
- **Broadcast Latency**: <1ms per client
- **Memory**: ~2MB per 1000 connections
- **CPU**: Minimal overhead with goroutine-based design

### Benchmarks

```
BenchmarkMessageBroadcast-8     50000    25000 ns/op    2400 B/op    40 allocs/op
BenchmarkRoomBroadcast-8        30000    35000 ns/op    3200 B/op    50 allocs/op
```

## Testing

### Run All Tests
```bash
go test ./internal/websocket/... -v
```

### Run Load Tests
```bash
go test ./internal/websocket/... -v -run LoadTest -timeout=5m
```

### Run Benchmarks
```bash
go test ./internal/websocket/... -bench=. -benchmem
```

### With Redis
```bash
# Start Redis
docker run -d -p 6379:6379 redis:alpine

# Run tests that require Redis
go test ./internal/websocket/... -v
```

## Security

### Authentication
- JWT tokens required for all connections
- Tokens can be provided via query parameter or Authorization header
- Token validation happens before WebSocket upgrade

### Authorization
- Room access validated via RoomTopology
- Users can only join rooms they have access to
- Team membership checked for team rooms

### Best Practices
- Always validate message payloads
- Use MessageValidator for schema enforcement
- Implement rate limiting at the HTTP handler level
- Set appropriate CORS policies in production

## Monitoring

### Metrics

```go
// Hub metrics
clientCount := hub.GetClientCount()
userClientCount := hub.GetUserClientCount(userID)

// Room metrics
roomCount := len(roomManager.ListRooms())
clientsInRoom := room.GetClientCount()

// Validation metrics
metrics := validator.GetMetrics()
// Returns: total_validations, failed_validations, invalid_version, etc.

// Bridge metrics (for multi-node)
stats := bridge.GetStats()
// Returns: subscribed_channels, channels list
```

### Logging

The system uses structured logging (zap) for:
- Client connect/disconnect events
- Room join/leave operations
- Message validation failures
- Reconnection attempts
- Error conditions

## Scaling

### Horizontal Scaling

Use Redis Pub/Sub for multi-node deployments:

1. All nodes connect to the same Redis instance
2. Each node runs its own Hub
3. Redis Bridge handles cross-node message routing
4. Offline Queue persists messages in Redis
5. Clients can reconnect to any node

```
            Load Balancer
                  |
        +---------+---------+
        |                   |
   Node 1 (Hub)        Node 2 (Hub)
        |                   |
        +------- Redis -----+
            (Pub/Sub + Streams)
```

### Vertical Scaling

- Each client connection uses ~2KB memory
- Goroutine-based design scales well
- Tested with 1000+ concurrent connections per node
- CPU usage is minimal with proper buffering

## Troubleshooting

### Connection Issues

**Problem**: Clients can't connect
- Check JWT token is valid and not expired
- Verify CORS settings allow WebSocket upgrade
- Ensure CheckOrigin returns true for your domain

**Problem**: Connections drop frequently
- Adjust ping/pong timeouts (currently 60s)
- Check network stability
- Review server resource limits

### Message Delivery

**Problem**: Messages not received
- Verify client is in the target room
- Check message type is correctly formatted
- Review validation errors in logs

**Problem**: Delayed message delivery
- Check channel buffer sizes (default 256)
- Monitor CPU and memory usage
- Consider increasing buffer sizes for high throughput

### Redis Issues

**Problem**: Offline messages not persisting
- Verify Redis connection is healthy
- Check Redis memory limits
- Review stream retention settings

**Problem**: Cross-node messages not working
- Ensure all nodes connect to same Redis instance
- Check Pub/Sub channels are subscribed correctly
- Review bridge logs for errors

## API Reference

See individual files for detailed API documentation:
- `hub.go` - Hub and client management
- `room.go` - Room operations
- `message.go` - Message types
- `client.go` - Client connection handling
- `offline_queue.go` - Offline message queue
- `reconnect.go` - Reconnection logic
- `redis_bridge.go` - Multi-node support
- `schema.go` - Message schema versioning
- `validator.go` - Message validation
- `subscription_manager.go` - Subscription management
- `room_topology.go` - Room authorization

## Contributing

When adding new features:
1. Add tests (unit and integration)
2. Update this README
3. Add appropriate logging
4. Consider multi-node implications
5. Validate message schemas
6. Document breaking changes

## License

Copyright (c) 2024 Donelist. All rights reserved.
