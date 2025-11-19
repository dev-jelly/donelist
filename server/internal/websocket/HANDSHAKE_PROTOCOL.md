# WebSocket Handshake and Health Check Protocol

This document describes the handshake and health check protocol for WebSocket connections in the donelist server.

## Overview

The handshake protocol provides:
- Session management with reconnection support
- Feature negotiation between client and server
- Version compatibility checking
- Heartbeat configuration
- Connection state monitoring

## Connection Lifecycle

```
Client                          Server
  |                               |
  |------ TCP Connection -------->|
  |                               |
  |------ Handshake Request ----->|
  |                               |
  |<----- Handshake Response -----|
  |     (with Session ID)         |
  |                               |
  |<====== Connected ============>|
  |                               |
  |------ Ping Messages --------->|
  |<----- Pong Messages ----------|
  |                               |
  |       (if disconnected)       |
  |                               |
  |------ Reconnect Request ----->|
  |     (with Session ID)         |
  |                               |
  |<----- Resume Response --------|
  |  (with Resume Sequence)       |
  |                               |
  |<====== Reconnected ===========>|
```

## Handshake Request

The client sends a handshake request after establishing the WebSocket connection.

### Message Format

```json
{
  "type": "handshake",
  "payload": {
    "client_version": "1.0.0",
    "supported_features": ["ping", "pong", "rooms", "presence"],
    "last_ack_seq": 0,
    "capabilities": {
      "compression": false,
      "encryption": false
    },
    "session_id": "",
    "platform": "web",
    "device_id": "unique-device-id"
  }
}
```

### Fields

- **client_version** (required): Client application version (max 32 chars)
- **supported_features** (optional): List of features the client supports
- **last_ack_seq** (optional): Last acknowledged sequence number for reconnection
- **capabilities** (optional): Client capabilities (compression, encryption, etc.)
- **session_id** (optional): Session ID for reconnection (empty for new connections)
- **platform** (optional): Client platform (web, ios, android, etc.)
- **device_id** (optional): Unique device identifier

## Handshake Response

The server responds with a handshake response containing session information.

### Success Response

```json
{
  "type": "handshake.response",
  "payload": {
    "session_id": "uuid-v4",
    "server_version": "1.0.0",
    "supported_features": ["ping", "pong", "rooms", "presence", "typing"],
    "enabled_features": ["ping", "pong", "rooms", "presence"],
    "heartbeat_interval": 30000,
    "heartbeat_timeout": 60000,
    "max_message_size": 512000,
    "resume_from_seq": 0,
    "server_capabilities": {
      "compression": false,
      "encryption": false,
      "binary_messages": false
    },
    "status": "accepted",
    "message": "Handshake successful"
  }
}
```

### Fields

- **session_id** (required): Unique session identifier (UUID v4)
- **server_version** (required): Server version
- **supported_features** (required): Features the server supports
- **enabled_features** (required): Features enabled for this session (intersection)
- **heartbeat_interval** (required): Milliseconds between heartbeat pings
- **heartbeat_timeout** (required): Milliseconds before connection marked as stale
- **max_message_size** (required): Maximum message size in bytes
- **resume_from_seq** (optional): Sequence number to resume from (reconnection)
- **server_capabilities** (optional): Server capabilities
- **status** (required): "accepted" or "rejected"
- **message** (optional): Additional information

### Rejection Response

```json
{
  "type": "handshake.response",
  "payload": {
    "status": "rejected",
    "message": "Unsupported client version"
  }
}
```

## Feature Negotiation

The server and client negotiate features during handshake:

1. Client sends list of supported features
2. Server responds with:
   - **supported_features**: All features the server supports
   - **enabled_features**: Intersection of client and server features

Common features:
- **ping**: Client can send ping messages
- **pong**: Server responds to pings with pongs
- **rooms**: Multi-user room support
- **presence**: User presence tracking
- **typing**: Typing indicators
- **offline-queue**: Offline message queuing

## Health Check Protocol

### Heartbeat Configuration

The server specifies heartbeat parameters in the handshake response:

- **heartbeat_interval**: How often to send pings (default: 30s)
- **heartbeat_timeout**: Timeout for pong response (default: 60s)

### Ping/Pong Messages

The server sends ping messages at the configured interval:

```json
{
  "type": "ping",
  "timestamp": "2025-01-13T10:00:00Z"
}
```

The client responds with a pong:

```json
{
  "type": "pong",
  "timestamp": "2025-01-13T10:00:00Z"
}
```

### Connection States

A connection can be in one of these states:

- **connecting**: Initial connection being established
- **handshaking**: Handshake in progress
- **connected**: Fully established and healthy
- **stale**: Missing heartbeat responses
- **reconnecting**: Attempting to reconnect
- **disconnected**: Connection closed

### State Transitions

```
connecting -> handshaking -> connected
                             |
                             v
                          stale (after 2 missed pings)
                             |
                             v
                        reconnecting
                             |
                             v
                        disconnected (after timeout)
```

### Health Status

Health checks track the connection health:

- **healthy**: All pongs received on time
- **degraded**: Some missed pongs (< max)
- **unhealthy**: Too many missed pongs (>= max)
- **unknown**: No health check data

## Reconnection

### Reconnection Flow

1. Client detects disconnection
2. Client reconnects with session ID from previous handshake
3. Server validates session and user
4. Server returns messages from sequence number (resume_from_seq)
5. Client acknowledges and continues

### Reconnection Request

```json
{
  "type": "handshake",
  "payload": {
    "client_version": "1.0.0",
    "supported_features": ["ping", "pong", "rooms"],
    "session_id": "previous-session-uuid",
    "last_ack_seq": 150
  }
}
```

### Reconnection Response

```json
{
  "type": "handshake.response",
  "payload": {
    "session_id": "previous-session-uuid",
    "server_version": "1.0.0",
    "enabled_features": ["ping", "pong", "rooms"],
    "heartbeat_interval": 30000,
    "heartbeat_timeout": 60000,
    "max_message_size": 512000,
    "resume_from_seq": 150,
    "status": "accepted",
    "message": "Reconnected to existing session, resuming from seq 150"
  }
}
```

## Session Management

### Session Information

The server tracks these session details:

- Session ID (UUID)
- User ID
- Client version
- Platform and device ID
- Enabled features
- Connection state
- Creation timestamp
- Last heartbeat timestamp
- Last activity timestamp
- Heartbeat miss count
- Last acknowledged sequence

### Session Lifecycle

1. **Creation**: New session created on first connection
2. **Active**: Session is active while connected
3. **Disconnected**: Connection closed, session retained
4. **Stale**: No activity for configured timeout
5. **Cleanup**: Session removed after stale timeout (default: 5 minutes)

### Session Cleanup

The server periodically cleans up stale sessions:

- Runs every 1 minute
- Removes sessions that have been:
  - Disconnected for > stale timeout
  - Stale with no heartbeat for > stale timeout

## Error Handling

### Handshake Errors

If handshake fails, the server sends an error:

```json
{
  "type": "error",
  "payload": {
    "message": "Invalid handshake request"
  }
}
```

### Session Errors

If session is invalid during reconnection:

- Server creates new session instead
- Client receives new session ID
- No resume sequence provided

## Client Implementation Example

```javascript
class WebSocketClient {
  constructor(url) {
    this.url = url;
    this.sessionId = null;
    this.lastAckSeq = 0;
    this.heartbeatInterval = 30000;
  }

  async connect() {
    this.ws = new WebSocket(this.url);

    this.ws.onopen = () => {
      this.sendHandshake();
    };

    this.ws.onmessage = (event) => {
      const msg = JSON.parse(event.data);
      this.handleMessage(msg);
    };

    this.ws.onclose = () => {
      this.reconnect();
    };
  }

  sendHandshake() {
    const handshake = {
      type: "handshake",
      payload: {
        client_version: "1.0.0",
        supported_features: ["ping", "pong", "rooms"],
        session_id: this.sessionId || "",
        last_ack_seq: this.lastAckSeq,
        platform: "web",
        device_id: this.getDeviceId()
      }
    };
    this.ws.send(JSON.stringify(handshake));
  }

  handleMessage(msg) {
    switch (msg.type) {
      case "handshake.response":
        this.handleHandshakeResponse(msg.payload);
        break;
      case "ping":
        this.sendPong();
        break;
      // ... other message types
    }
  }

  handleHandshakeResponse(payload) {
    if (payload.status === "accepted") {
      this.sessionId = payload.session_id;
      this.heartbeatInterval = payload.heartbeat_interval;
      this.lastAckSeq = payload.resume_from_seq || 0;
      console.log("Connected with session:", this.sessionId);
    } else {
      console.error("Handshake rejected:", payload.message);
    }
  }

  sendPong() {
    const pong = {
      type: "pong",
      timestamp: new Date().toISOString()
    };
    this.ws.send(JSON.stringify(pong));
  }

  reconnect() {
    setTimeout(() => {
      console.log("Reconnecting...");
      this.connect();
    }, 1000);
  }

  getDeviceId() {
    let deviceId = localStorage.getItem("device_id");
    if (!deviceId) {
      deviceId = crypto.randomUUID();
      localStorage.setItem("device_id", deviceId);
    }
    return deviceId;
  }
}
```

## Metrics and Monitoring

### Session Metrics

```go
stats := sessionManager.GetSessionStats()
// {
//   "total_sessions": 150,
//   "unique_users": 120,
//   "connected": 140,
//   "stale": 8,
//   "reconnecting": 0,
//   "disconnected": 2
// }
```

### Health Metrics

```go
metrics := healthMonitor.GetAllMetrics()
// {
//   "total_clients": 150,
//   "healthy": 145,
//   "degraded": 3,
//   "unhealthy": 2,
//   "unknown": 0,
//   "client_metrics": {
//     "user-123": {
//       "status": "healthy",
//       "missed_ping_count": 0,
//       "round_trip_time_ms": 45,
//       "avg_rtt_ms": 42
//     }
//   }
// }
```

## Best Practices

### Client Side

1. **Store Session ID**: Persist session ID for reconnection
2. **Track Sequence**: Track last acknowledged sequence number
3. **Implement Backoff**: Use exponential backoff for reconnection
4. **Handle Pongs**: Respond to all ping messages promptly
5. **Monitor Latency**: Track round-trip time for health monitoring

### Server Side

1. **Session Cleanup**: Run cleanup workers regularly
2. **Monitor Health**: Track connection health metrics
3. **Rate Limiting**: Limit handshake attempts per IP
4. **Version Checking**: Enforce minimum client versions
5. **Feature Flags**: Use features for gradual rollout

## Security Considerations

1. **Authentication**: Handshake must occur after authentication
2. **Session Hijacking**: Validate user ID matches session
3. **Rate Limiting**: Limit handshake and reconnection attempts
4. **Timeout Enforcement**: Enforce heartbeat timeouts strictly
5. **Session Expiry**: Clean up old sessions regularly

## Configuration

### Default Values

```go
// Session Manager
staleSessionTimeout = 5 * time.Minute

// Health Check
pingInterval = 30 * time.Second
pongTimeout = 60 * time.Second
maxMissedPings = 3

// Reconnection
maxReconnectAttempts = 10
reconnectBackoff = exponential (1s to 60s)
```

### Environment Variables

Configure via environment or code:

```go
sessionManager := NewSessionManager(logger)
sessionManager.heartbeatInterval = 30 * time.Second
sessionManager.heartbeatTimeout = 60 * time.Second
sessionManager.staleSessionTimeout = 5 * time.Minute
```

## Troubleshooting

### Connection Drops

- Check heartbeat configuration
- Monitor network conditions
- Review server logs for errors
- Check client implementation

### Failed Reconnections

- Verify session ID is valid
- Check session hasn't expired
- Ensure user ID matches
- Review timeout settings

### High Latency

- Monitor RTT metrics
- Check server load
- Review network path
- Consider adjusting timeouts
