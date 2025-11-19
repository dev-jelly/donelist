# Offline Sync Architecture

## Overview

The Donelist offline sync system enables mobile clients to operate fully offline, queuing operations locally and synchronizing with the server when connectivity is restored. The system uses a conflict-aware, last-write-wins approach with idempotency guarantees.

## Architecture Components

### 1. Client-Side (Mobile App)

#### Local Storage
- **SQLite Database**: Stores all user data locally
- **Sync Queue**: Pending operations waiting to be synced
- **Metadata**: Device ID, last sync timestamp, sync status

#### Offline Queue Structure
```json
{
  "id": "uuid",
  "operation_type": "create|update|delete",
  "resource_type": "checkin|category|tag",
  "resource_id": "uuid",
  "idempotency_key": "unique-key",
  "client_timestamp": "2024-01-01T12:00:00Z",
  "data": {...},
  "status": "pending|syncing|completed|failed|conflicted",
  "retry_count": 0
}
```

### 2. Server-Side (Backend API)

#### Database Tables

**sync_queue**
- Stores pending sync operations from clients
- Tracks operation status (pending, processing, completed, failed, conflicted)
- Enforces idempotency per user
- Retains completed operations for 30 days

**sync_status**
- Tracks sync state per device
- Records last sync timestamp for delta sync
- Counts pending and failed operations
- Stores client metadata (version, platform)

**sync_operation_log**
- Audit trail for all sync operations
- Records resolution strategies
- Tracks processing duration
- Retains logs for 90 days

**idempotency_tokens**
- Caches responses for duplicate requests
- Prevents duplicate operations during retries
- Expires after 24 hours

## Sync Protocol

### 1. Batch Sync Request

**Endpoint**: `POST /api/v1/sync`

**Request**:
```json
{
  "device_id": "device-uuid",
  "client_version": "1.0.0",
  "platform": "ios|android",
  "last_sync_at": "2024-01-01T12:00:00Z",
  "operations": [
    {
      "idempotency_key": "unique-key-1",
      "operation_type": "create",
      "resource_type": "checkin",
      "resource_id": "checkin-uuid",
      "client_timestamp": "2024-01-01T12:00:00Z",
      "data": {
        "content": "Morning workout",
        "category_id": "category-uuid",
        "checkin_time": "2024-01-01T08:00:00Z",
        "duration_minutes": 30,
        "version": 1
      }
    }
  ]
}
```

**Response**:
```json
{
  "success": true,
  "synced_at": "2024-01-01T12:05:00Z",
  "results": [
    {
      "idempotency_key": "unique-key-1",
      "resource_id": "checkin-uuid",
      "status": "success|failed|conflicted|skipped",
      "error": "error message if failed",
      "conflict_info": {
        "type": "version_mismatch|concurrent_edit|deleted",
        "client_version": 1,
        "server_version": 2,
        "client_timestamp": "2024-01-01T12:00:00Z",
        "server_timestamp": "2024-01-01T11:55:00Z",
        "server_data": {...},
        "recommended_action": "accept_server|force_client|manual_merge"
      }
    }
  ],
  "server_changes": [
    {
      "operation_type": "update",
      "resource_type": "checkin",
      "resource_id": "other-checkin-uuid",
      "data": {...},
      "server_timestamp": "2024-01-01T12:03:00Z",
      "version": 3
    }
  ],
  "has_more_changes": false,
  "conflicts_count": 0,
  "success_count": 1,
  "failure_count": 0
}
```

### 2. Sync Status Check

**Endpoint**: `GET /api/v1/sync/status?device_id={device_id}`

**Response**:
```json
{
  "device_id": "device-uuid",
  "last_sync_at": "2024-01-01T12:00:00Z",
  "last_successful_sync_at": "2024-01-01T12:00:00Z",
  "pending_operations_count": 0,
  "failed_operations_count": 0,
  "is_up_to_date": true,
  "needs_full_sync": false
}
```

### 3. Conflict Management

**Get Conflicts**: `GET /api/v1/sync/conflicts`

**Resolve Conflict**: `POST /api/v1/sync/conflicts/resolve`
```json
{
  "idempotency_key": "unique-key-1",
  "resolution": "accept_server|force_client",
  "data": {...}
}
```

## Conflict Resolution

### Detection

Conflicts are detected by comparing:
1. **Version numbers**: Using optimistic locking (version field)
2. **Timestamps**: Comparing client vs server modification times
3. **Existence**: Checking if resource was deleted

### Conflict Types

1. **Version Mismatch**: Client version doesn't match server version
   - Indicates concurrent edits
   - Server has newer version

2. **Concurrent Edit**: Multiple clients edited same resource
   - Detected by comparing timestamps
   - Last-write-wins by default

3. **Deleted**: Resource exists on client but deleted on server
   - Client should remove local copy

4. **Already Exists**: Client tries to create resource that exists
   - Use existing server version

### Resolution Strategies

#### 1. Last-Write-Wins (Default)
- Compare client_timestamp vs server_timestamp
- Most recent change wins
- Automatic resolution

**Implementation**:
```go
if clientTimestamp.After(serverTimestamp) {
    return "force_client"
}
return "accept_server"
```

#### 2. Client Wins
- Always apply client changes
- Use for trusted operations
- Override server state

#### 3. Server Wins
- Always keep server state
- Discard client changes
- Safe default for conflicts

#### 4. Manual Merge
- Require user intervention
- Present both versions to user
- User manually merges changes

### Conflict Resolution Flow

```
1. Client sends operation
2. Server detects conflict
3. Server applies resolution strategy (Last-Write-Wins)
4. If auto-resolvable:
   a. Apply resolution
   b. Return result
5. If requires manual resolution:
   a. Mark as conflicted
   b. Return conflict info
   c. Store in sync_queue with status=conflicted
   d. Client presents options to user
   e. User selects resolution
   f. Client sends resolution request
   g. Server applies resolution
```

## Idempotency

### Idempotency Keys

- Client generates unique key per operation
- Format: `{device_id}-{operation_type}-{resource_id}-{timestamp}`
- Example: `device-123-create-checkin-abc-1704110400000`

### Guarantees

1. **Same operation submitted twice**: Returns same result
2. **Network retry**: Safe to retry any request
3. **Duplicate prevention**: Prevents accidental duplicates

### Implementation

```sql
CREATE UNIQUE INDEX ON sync_queue (user_id, idempotency_key);
```

Server checks idempotency key:
- If exists and completed → Return cached response
- If exists and processing → Return 409 Conflict
- If not exists → Process operation

## Optimistic Updates

### Client Flow

1. User performs action (create/update/delete)
2. Immediately update local UI (optimistic)
3. Add operation to sync queue
4. Mark operation as "pending"
5. When online, sync with server
6. If success: Mark as "completed"
7. If conflict: Mark as "conflicted", show UI indicator
8. If failure: Mark as "failed", schedule retry

### Retry Strategy

**Exponential Backoff with Jitter**:
```
retry_interval = min(
    initial_interval * (multiplier ^ retry_count),
    max_interval
)
retry_interval += random(-jitter, +jitter)
```

**Default Configuration**:
- Initial interval: 1 second
- Max interval: 60 seconds
- Multiplier: 2.0
- Max retries: 10
- Jitter: ±30%

**Circuit Breaker**:
- After 5 consecutive failures: Enter "circuit open" state
- Wait 5 minutes before trying again
- Prevents hammering server during outage

## Delta Sync

### Server-Side Changes

Server tracks changes since `last_sync_at`:
```sql
SELECT * FROM checkins
WHERE user_id = ? AND updated_at > ?
ORDER BY updated_at ASC
```

Returns changes in `server_changes` array:
- CREATE: New resources created on other devices
- UPDATE: Resources updated on other devices
- DELETE: Resources deleted on other devices

### Client Application

Client processes server changes:
1. For each server change
2. Check if local version exists
3. If not exists: Apply server change
4. If exists: Compare versions
5. If local version is older: Update from server
6. If local version is newer: Keep local (already in sync queue)

## Data Security

### Encryption at Rest (Client)

- SQLite database encrypted using SQLCipher
- Encryption key derived from user password
- AES-256 encryption

### Encryption in Transit

- All API requests over HTTPS/TLS 1.3
- Certificate pinning on mobile apps
- Request signing using JWT

### Data Integrity

- SHA-256 checksums for operation payloads
- Verify payload hasn't been tampered with
- Detect corruption during sync

## Performance Optimization

### Batch Operations

- Send up to 100 operations per sync request
- Server processes in single transaction
- Reduces network round trips

### Compression

- Request/response gzip compression
- Reduces bandwidth usage
- Important for mobile data usage

### Pagination

- Server changes paginated (100 per request)
- Client requests next page if `has_more_changes=true`
- Prevents overwhelming client with large datasets

### Background Sync

- iOS: Background App Refresh
- Android: WorkManager periodic sync
- Sync every 15 minutes when online
- Immediate sync on network change (offline→online)

## Storage Management

### Client-Side

**Maximum Offline Queue Size**: 1000 operations
- Oldest operations removed if limit exceeded
- Warning shown to user

**Retention Policy**:
- Completed operations: Removed after successful sync
- Failed operations: Retained for 7 days
- Conflicted operations: Retained until resolved

### Server-Side

**Automatic Cleanup**:
```sql
-- Run daily via cron job
DELETE FROM sync_queue
WHERE status = 'completed'
AND completed_at < NOW() - INTERVAL '30 days';

DELETE FROM sync_operation_log
WHERE created_at < NOW() - INTERVAL '90 days';

DELETE FROM idempotency_tokens
WHERE expires_at < NOW();
```

## Monitoring & Observability

### Metrics

**Client Metrics**:
- Sync success rate
- Average sync duration
- Queue size
- Conflict rate
- Retry count distribution

**Server Metrics**:
- Sync requests per second
- Average processing time
- Conflict rate per user
- Failed operations rate
- Storage usage (sync_queue size)

### Logging

**Client Logs**:
```json
{
  "event": "sync_completed",
  "device_id": "uuid",
  "duration_ms": 1234,
  "operations_synced": 15,
  "conflicts": 0,
  "failures": 0
}
```

**Server Logs**:
```json
{
  "event": "sync_request_processed",
  "user_id": "uuid",
  "device_id": "uuid",
  "operations_count": 15,
  "success_count": 14,
  "conflict_count": 1,
  "failure_count": 0,
  "processing_time_ms": 234
}
```

### Alerts

- High conflict rate (>5%)
- High failure rate (>10%)
- Sync queue growing (>1000 operations/user)
- Long processing time (>5 seconds)
- Circuit breaker triggered

## Testing Strategy

### Unit Tests

- Conflict detection logic
- Resolution strategies
- Idempotency enforcement
- Retry mechanism
- Data validation

### Integration Tests

- Client-server sync flow
- Concurrent operations
- Network interruption scenarios
- Large batch operations
- Pagination

### End-to-End Tests

- Multi-device sync
- Offline operation queuing
- Conflict resolution flows
- Background sync
- Data consistency verification

## API Endpoints Summary

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/v1/sync` | Batch sync operations |
| GET | `/api/v1/sync/status` | Get sync status for device |
| GET | `/api/v1/sync/conflicts` | Get conflicted operations |
| POST | `/api/v1/sync/conflicts/resolve` | Manually resolve conflict |
| POST | `/api/v1/sync/checkins` | Create checkin with idempotency (optional) |

## Client Implementation Checklist

- [ ] Implement local SQLite database with sync_queue table
- [ ] Add operation queuing on create/update/delete
- [ ] Implement optimistic UI updates
- [ ] Add network state monitoring
- [ ] Implement batch sync API call
- [ ] Add conflict resolution UI
- [ ] Implement exponential backoff retry
- [ ] Add sync status indicators in UI
- [ ] Implement background sync
- [ ] Add idempotency key generation
- [ ] Implement delta sync (apply server changes)
- [ ] Add encryption at rest (SQLCipher)
- [ ] Implement data compression
- [ ] Add sync metrics/logging
- [ ] Test offline scenarios
- [ ] Test conflict resolution
- [ ] Test network interruption
- [ ] Performance testing with large datasets

## Future Enhancements

1. **Real-time Sync**: WebSocket-based push for instant updates
2. **Smart Conflict Resolution**: ML-based conflict prediction
3. **Partial Sync**: Sync only changed fields, not entire resources
4. **Peer-to-Peer Sync**: Direct device-to-device sync when on same network
5. **Collaborative Editing**: Operational Transformation for concurrent edits
6. **Sync Priority**: User-defined operation priority (urgent vs can-wait)
7. **Bandwidth Optimization**: Adaptive compression based on connection quality
8. **Cross-Platform Sync**: Browser extension, desktop app support

## References

- [Offline First Architecture](https://offlinefirst.org/)
- [CRDTs for Conflict-Free Replication](https://crdt.tech/)
- [Operational Transformation](https://en.wikipedia.org/wiki/Operational_transformation)
- [Idempotency Patterns](https://stripe.com/docs/api/idempotent_requests)
- [Last-Write-Wins vs CRDTs](https://jepsen.io/consistency/models/lww-element-set)
