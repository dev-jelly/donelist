# WebSocket Room Topology System

## Overview

The WebSocket room topology system provides a structured approach to managing real-time communication channels for the Donelist application. It supports user-specific, team-based, and device-specific rooms with proper access control and multi-node scalability via Redis Pub/Sub.

## Architecture

### Components

1. **RoomTopology** - Defines room naming conventions and access control rules
2. **SubscriptionManager** - Manages room subscriptions and membership
3. **RedisBridge** - Enables multi-node communication via Redis Pub/Sub
4. **Room** - Individual communication channel with client management
5. **Hub** - Central coordinator for all WebSocket connections

### Room Types

#### 1. User Room (`user:{userID}`)
- **Purpose**: Personal updates for a specific user
- **Access**: Only the user themselves
- **Example**: `user:123e4567-e89b-12d3-a456-426614174000`
- **Use Cases**:
  - Personal notifications
  - Account status updates
  - Private messages

#### 2. Team Room (`team:{teamID}`)
- **Purpose**: Broadcast to all team members
- **Access**: Any user who is a member of the team
- **Example**: `team:team-alpha`
- **Use Cases**:
  - Team-wide announcements
  - Shared checkin updates
  - Collaboration notifications

#### 3. Device Room (`device:{userID}:{deviceID}`)
- **Purpose**: Device-specific updates
- **Access**: Only the user who owns the device
- **Example**: `device:123e4567-e89b-12d3-a456-426614174000:iPhone-X`
- **Use Cases**:
  - Device-specific sync
  - Push notification targeting
  - Device health monitoring

#### 4. User-Team Room (`user:{userID}:team:{teamID}`)
- **Purpose**: User-specific updates within a team context
- **Access**: Only the specific user who is a team member
- **Example**: `user:123e4567-e89b-12d3-a456-426614174000:team:team-alpha`
- **Use Cases**:
  - Personal tasks within team context
  - User-specific team notifications
  - Role-based updates

## Implementation Details

### Room Naming Convention

Room IDs follow a structured format:
```
{type}:{identifier}[:{type}:{identifier}]
```

Examples:
- `user:user-123`
- `team:team-456`
- `device:user-123:device-789`
- `user:user-123:team:team-456`

### Access Control

Access control is enforced at subscription time using the `ValidateRoomAccess` function:

```go
func (rt *RoomTopology) ValidateRoomAccess(
    userID string,
    teamIDs []string,
    roomID string
) bool
```

**Rules**:
1. Users can only access their own user rooms
2. Users can only access team rooms they belong to
3. Users can only access their own device rooms
4. Users can only access user-team rooms for teams they belong to

### Automatic Subscription

When a user connects, they are automatically subscribed to relevant rooms:

```go
func (sm *SubscriptionManager) SubscribeUserToRooms(
    ctx context.Context,
    client *Client,
    userID string,
    teamIDs []string,
    deviceID string
) error
```

This subscribes the user to:
- Their personal user room
- All team rooms they belong to
- Their user-team rooms for each team
- Their device room (if deviceID provided)

## Multi-Node Support via Redis

### Redis Pub/Sub Architecture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Node 1    │     │   Node 2    │     │   Node 3    │
│             │     │             │     │             │
│  ┌───────┐  │     │  ┌───────┐  │     │  ┌───────┐  │
│  │ Hub   │  │     │  │ Hub   │  │     │  │ Hub   │  │
│  └───┬───┘  │     │  └───┬───┘  │     │  └───┬───┘  │
│      │      │     │      │      │     │      │      │
│  ┌───▼───┐  │     │  ┌───▼───┐  │     │  ┌───▼───┐  │
│  │Bridge │  │     │  │Bridge │  │     │  │Bridge │  │
│  └───┬───┘  │     │  └───┬───┘  │     │  └───┬───┘  │
└──────┼──────┘     └──────┼──────┘     └──────┼──────┘
       │                   │                   │
       └───────────────────┼───────────────────┘
                           │
                    ┌──────▼──────┐
                    │    Redis    │
                    │   Pub/Sub   │
                    └─────────────┘
```

### Message Flow

1. **Local Broadcast**: Message sent to local room clients
2. **Redis Publish**: Message published to Redis channel `ws:room:{roomID}`
3. **Fanout**: All nodes subscribed to that channel receive the message
4. **Local Delivery**: Each node delivers to its local clients

### Channel Naming

Redis channels follow the pattern:
```
ws:room:{roomID}
```

Examples:
- `ws:room:user:user-123`
- `ws:room:team:team-456`

### Deduplication

Messages are not deduplicated by default. Each node will receive and broadcast all messages from Redis. This ensures eventual consistency but means clients should handle duplicate messages if connected to multiple nodes.

## Sharding Strategy

For load balancing and horizontal scaling, rooms are sharded based on a consistent key:

```go
func (rt *RoomTopology) GetShardingKey(roomID string) string
```

**Sharding Rules**:
- User rooms, Device rooms, User-Team rooms → Shard by `userID`
- Team rooms → Shard by `teamID`

This ensures:
- All of a user's connections typically route to the same node
- Team rooms can be distributed across nodes
- Load is evenly distributed based on user activity

## Usage Examples

### Basic Room Subscription

```go
// Create subscription manager
sm := websocket.NewSubscriptionManager(websocket.SubscriptionConfig{
    Hub:         hub,
    RoomManager: roomManager,
    Bridge:      redisBridge,
    Logger:      logger,
})

// Subscribe user to their rooms
err := sm.SubscribeUserToRooms(
    ctx,
    client,
    "user-123",
    []string{"team-1", "team-2"},
    "device-456",
)
```

### Broadcasting a Message

```go
// Broadcast to a specific room
msg := &websocket.Message{
    Type:      "checkin.created",
    RoomID:    "user:user-123",
    Timestamp: time.Now(),
    Payload: map[string]interface{}{
        "checkin_id": checkinID,
        "title":      "Working on feature",
    },
}

err := sm.BroadcastToRoom(ctx, "user:user-123", msg)
```

### Manual Room Join with Authorization

```go
// Join a specific room
req := websocket.JoinRoomRequest{
    RoomID:   "team:team-alpha",
    UserID:   "user-123",
    TeamIDs:  []string{"team-alpha", "team-beta"},
    DeviceID: "",
}

resp, err := sm.JoinRoom(ctx, client, req)
if err != nil {
    // Handle unauthorized access
}
```

## Error Handling & Fallback

### Redis Connection Failure

If Redis is unavailable:
1. Subscription manager continues to work locally
2. Broadcasts only reach clients on the same node
3. Health checks report degraded state
4. Automatic reconnection attempts

### Room Creation Failure

If a room cannot be created:
1. Error returned to client
2. Client can retry
3. Logs error for monitoring

### Broadcast Failures

If broadcast fails:
1. Local broadcast continues
2. Redis publish failure is logged
3. Message delivery is best-effort

## Performance Considerations

### Memory Usage

- Each room maintains a map of clients
- Each client maintains a map of rooms
- Typical memory per client: ~1-2 KB
- Typical memory per room: ~100 bytes + (clients * 8 bytes)

### Network Usage

- Each message is published once to Redis
- Each node receives the message once
- Local fanout to N clients is O(N) per node
- No message amplification across nodes

### Latency

- Local broadcast: < 1ms
- Redis publish: ~1-5ms (network dependent)
- Total end-to-end: < 10ms typically

## Monitoring & Metrics

### Key Metrics

1. **Room Count**: `sm.GetStats()["total_rooms"]`
2. **Member Count**: `sm.GetRoomMemberCount(roomID)`
3. **Subscriptions**: `sm.GetUserSubscriptions(userID)`
4. **Redis Health**: `bridge.HealthCheck(ctx)`
5. **Message Rate**: Track via middleware

### Health Checks

```go
// Check Redis connectivity
err := redisBridge.HealthCheck(ctx)

// Get bridge statistics
stats := redisBridge.GetStats()
// Returns: subscribed_channels, channels[]

// Get subscription statistics
stats := subscriptionManager.GetStats()
// Returns: total_rooms, total_members
```

## Best Practices

### 1. Room Lifecycle

- Create rooms lazily on first join
- Clean up empty rooms to prevent memory leaks
- Use unsubscribe to remove unused Redis subscriptions

### 2. Message Design

- Keep messages small (< 1KB)
- Use message types for routing
- Include timestamps for ordering
- Add correlation IDs for tracing

### 3. Access Control

- Always validate room access before subscription
- Store team memberships in a fast cache
- Regularly sync team memberships from database

### 4. Scalability

- Use sharding keys for load balancing
- Limit max clients per room (default: 100)
- Monitor Redis memory usage
- Consider Redis Streams for large-scale deployments

## Testing

### Unit Tests

```bash
go test -v ./internal/websocket -run "TestRoomTopology"
go test -v ./internal/websocket -run "TestSubscriptionManager"
```

### Integration Tests

```bash
# Requires Redis running on localhost:6379
go test -v ./internal/websocket -run "TestRedisBridge"
```

## Migration Path

### From Simple Rooms to Topology-Based

1. Update client code to use topology-based room IDs
2. Migrate existing subscriptions to new format
3. Enable Redis bridge for multi-node support
4. Monitor and validate message delivery
5. Remove old room implementation

### Zero-Downtime Migration

1. Deploy new code with both systems running
2. Gradually migrate clients to new topology
3. Monitor both systems in parallel
4. Deprecate old system after full migration
5. Remove old code in next release

## Security Considerations

### 1. Room ID Validation

- Always parse and validate room IDs
- Reject malformed room IDs
- Rate-limit room creation

### 2. Access Control

- Enforce authorization on every join request
- Don't trust client-provided team IDs
- Verify team membership from authoritative source

### 3. Message Filtering

- Sanitize message content
- Limit message size
- Rate-limit broadcasts per user

### 4. Redis Security

- Use Redis AUTH for production
- Enable TLS for Redis connections
- Isolate Redis instance per environment
- Monitor for unusual pub/sub patterns

## Future Enhancements

### 1. Message Persistence

- Store messages in Redis Streams
- Enable message replay on reconnect
- Implement message retention policies

### 2. Advanced Routing

- Support wildcard subscriptions
- Implement message filtering
- Add priority queues

### 3. Analytics

- Track message delivery rates
- Monitor room activity
- Detect hot rooms for optimization

### 4. Horizontal Scaling

- Implement consistent hashing for room assignment
- Add room migration between nodes
- Support room replication for high-traffic rooms
