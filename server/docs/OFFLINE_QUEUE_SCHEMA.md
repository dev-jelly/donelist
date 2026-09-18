# Offline Storage Queue Schema Design

## Overview

The offline storage queue system provides SQLite-based local storage for check-in data when the network is unavailable. This enables mobile clients to queue check-ins offline and sync them when connectivity is restored.

## Architecture

### Components

1. **SQLite Database**: Lightweight, embedded database for local storage
2. **Encryption Layer**: AES-256-GCM encryption for data at rest
3. **Queue Management**: FIFO queue with retry logic and status tracking
4. **Automatic Pruning**: Capacity limits and time-based cleanup

## Database Schema

### Table: `offline_queue`

```sql
CREATE TABLE offline_queue (
    id TEXT PRIMARY KEY,                    -- UUID of queue item
    user_id TEXT NOT NULL,                  -- User who created the check-in
    checkin_data_encrypted TEXT NOT NULL,   -- Encrypted JSON check-in data
    created_at INTEGER NOT NULL,            -- Unix timestamp (creation time)
    sync_status TEXT NOT NULL DEFAULT 'pending',  -- pending, syncing, failed, synced
    retry_count INTEGER NOT NULL DEFAULT 0, -- Number of sync attempts
    last_attempt INTEGER,                   -- Unix timestamp of last sync attempt
    error_message TEXT                      -- Error message if sync failed
);
```

### Indexes

```sql
CREATE INDEX idx_user_id ON offline_queue(user_id);
CREATE INDEX idx_sync_status ON offline_queue(sync_status);
CREATE INDEX idx_created_at ON offline_queue(created_at);
CREATE INDEX idx_user_status ON offline_queue(user_id, sync_status);
```

**Index Rationale**:
- `idx_user_id`: Fast retrieval of all items for a specific user
- `idx_sync_status`: Efficient filtering by sync status (pending items)
- `idx_created_at`: FIFO ordering and age-based pruning
- `idx_user_status`: Composite index for common query pattern (user's pending items)

## Data Model

### OfflineQueueItem

```go
type OfflineQueueItem struct {
    ID           uuid.UUID              // Unique identifier
    UserID       uuid.UUID              // User who owns the check-in
    CheckinData  map[string]interface{} // Decrypted check-in data
    CreatedAt    time.Time              // When item was queued
    SyncStatus   string                 // Current sync status
    RetryCount   int                    // Number of sync attempts
    LastAttempt  *time.Time            // When last sync was attempted
    ErrorMessage *string               // Error details if failed
}
```

### Check-in Data Structure

The `checkin_data` field (when decrypted) contains:

```json
{
  "content": "Check-in content text",
  "category_id": "uuid-string",
  "checkin_time": "2024-11-25T10:30:00Z",
  "duration_minutes": 30,
  "tag_names": ["work", "project"],
  "team_id": "uuid-string (optional)",
  "visibility": "private"
}
```

## Encryption

### Encryption Method: AES-256-GCM

**Why AES-256-GCM?**
- **AES-256**: Industry standard, highly secure symmetric encryption
- **GCM Mode**: Provides both confidentiality and authenticity
- **Authenticated Encryption**: Prevents tampering with encrypted data
- **Performance**: Hardware acceleration on most modern devices

### Key Management

- **Key Size**: 32 bytes (256 bits)
- **Key Storage**: Securely stored in device keychain/keystore
- **Key Rotation**: Support for periodic key rotation (future enhancement)
- **Nonce**: Randomly generated per encryption operation (96 bits)

### Encryption Process

1. Generate random nonce (12 bytes for GCM)
2. Encrypt plaintext JSON using AES-256-GCM with nonce
3. Prepend nonce to ciphertext
4. Encode as base64 for storage
5. Store encrypted data in SQLite

### Decryption Process

1. Decode base64 string
2. Extract nonce (first 12 bytes)
3. Extract ciphertext (remaining bytes)
4. Decrypt using AES-256-GCM
5. Verify authentication tag
6. Parse JSON to retrieve data

## Queue Operations

### 1. Enqueue

**Purpose**: Add a check-in to the offline queue

**Process**:
1. Check if pruning is needed (capacity/age limits)
2. Marshal check-in data to JSON
3. Encrypt JSON data
4. Generate UUID for queue item
5. Insert into database with 'pending' status
6. Return queue item

**Complexity**: O(1) average, O(n) if pruning triggered

### 2. Dequeue

**Purpose**: Retrieve and remove next pending item (FIFO)

**Process**:
1. Start transaction
2. Query oldest pending item for user (ORDER BY created_at ASC)
3. Decrypt check-in data
4. Delete item from database
5. Commit transaction
6. Return item

**Complexity**: O(log n) due to index lookup

### 3. List

**Purpose**: Retrieve items without removing them

**Process**:
1. Query items by user_id and optional status filter
2. Decrypt each item's check-in data
3. Return array of items

**Complexity**: O(m) where m is number of items for user

### 4. Update Status

**Purpose**: Track sync progress and failures

**Process**:
1. Update sync_status field
2. Increment retry_count
3. Set last_attempt timestamp
4. Set error_message if provided

**Complexity**: O(log n) due to primary key lookup

## Capacity Management

### Limits

1. **Max Items**: 1000 items total (configurable)
2. **Max Age**: 30 days (configurable)

### Pruning Strategy

**When**:
- Automatically on every enqueue operation
- Manually via Prune() method
- Periodically via background task (recommended)

**What**:
1. **Age-based**: Delete items older than max_age
2. **Capacity-based**: If total > max_items, delete oldest items

**Algorithm**:
```
1. DELETE items WHERE created_at < (now - max_age)
2. count = COUNT(*)
3. IF count > max_items THEN
   excess = count - max_items
   DELETE oldest `excess` items
```

### Pruning Performance

- Age pruning: O(n) scan, optimized by `idx_created_at`
- Capacity pruning: O(n log n) for finding oldest items
- Both operations use indexed queries for efficiency

## Sync Status Flow

```
pending → syncing → synced (success)
   ↓         ↓
   └─────────→ failed (on error)
              ↓
         retry → syncing → ...
```

### Status Definitions

- **pending**: Queued, not yet attempted
- **syncing**: Currently being synced to server
- **synced**: Successfully synced (can be deleted)
- **failed**: Sync failed, will retry

### Retry Logic

- Retry count incremented on each attempt
- Last attempt timestamp tracked
- Error message stored for debugging
- Client decides retry strategy (exponential backoff recommended)

## Security Considerations

### Data at Rest

- All check-in data encrypted with AES-256-GCM
- Encryption key never stored in database
- Key stored securely in device keychain

### Data in Transit

- Queue operations happen locally (no network)
- Sync to server uses TLS/HTTPS
- Server validates all synced data

### Privacy

- Each user's data isolated by user_id
- No cross-user data access
- Database file can be securely deleted on logout

## Performance Characteristics

### Operation Complexity

| Operation | Time Complexity | Space Complexity |
|-----------|----------------|------------------|
| Enqueue   | O(1) amortized | O(1)             |
| Dequeue   | O(log n)       | O(1)             |
| List      | O(m)           | O(m)             |
| Update    | O(log n)       | O(1)             |
| Count     | O(1)           | O(1)             |
| Prune     | O(n)           | O(1)             |

*where n = total items, m = items for user*

### Storage Overhead

- Encrypted data: ~33% overhead (base64 encoding)
- Nonce: 12 bytes per item
- GCM tag: 16 bytes per item
- Metadata: ~100 bytes per item
- Indexes: ~20% of table size

**Example**:
- Raw JSON: 200 bytes
- Encrypted + encoded: ~300 bytes
- Total with metadata: ~400 bytes per item
- 1000 items: ~400 KB

### Database File Size

**Typical scenario** (1000 items):
- Table data: ~400 KB
- Indexes: ~80 KB
- SQLite overhead: ~100 KB
- **Total**: ~580 KB

**Maximum scenario** (configurable limits):
- Can be tuned based on device constraints
- Mobile devices: 1000 items recommended
- Desktop clients: Higher limits acceptable

## Testing Strategy

### Unit Tests

1. **Schema Initialization**: Verify tables and indexes created
2. **Encryption/Decryption**: Test data confidentiality
3. **Queue Operations**: Test enqueue, dequeue, list, update
4. **Capacity Limits**: Test automatic pruning
5. **FIFO Ordering**: Verify oldest items dequeued first
6. **Concurrent Access**: Test thread safety (SQLite handles this)
7. **Error Handling**: Test invalid data, corrupted encryption

### Integration Tests

1. **End-to-end Flow**: Queue → List → Sync → Delete
2. **Large Dataset**: Test with max capacity
3. **Long-running**: Test 30-day retention
4. **Recovery**: Test database file corruption handling

### Performance Tests

1. **Enqueue Throughput**: Measure items/second
2. **Dequeue Latency**: Measure retrieval time
3. **Query Performance**: Test with full queue (1000 items)
4. **Pruning Performance**: Measure cleanup time

## Migration Considerations

### Future Enhancements

1. **Compression**: Add gzip compression before encryption
2. **Batch Operations**: Support bulk enqueue/dequeue
3. **Priority Queue**: Support priority-based ordering
4. **Partial Sync**: Sync subsets of data
5. **Conflict Resolution**: Handle server-side conflicts

### Schema Versioning

Current version: 1

Migration strategy:
- Add `schema_version` table
- Implement migration scripts for schema changes
- Test migrations thoroughly before deployment

## Mobile Client Integration

### iOS Example

```swift
let queue = OfflineQueue(
    dbPath: documentsPath + "/offline_queue.db",
    encryptionKey: keychain.getEncryptionKey(),
    maxItems: 1000,
    maxAge: 30 * 24 * 3600
)

// Queue check-in when offline
try queue.enqueue(
    userID: currentUser.id,
    checkinData: [
        "content": "Meeting notes",
        "category_id": categoryID,
        "checkin_time": ISO8601DateFormatter().string(from: Date()),
        "duration_minutes": 30
    ]
)

// Sync when online
while let item = try queue.dequeue(userID: currentUser.id) {
    do {
        try await api.createCheckin(item.checkinData)
    } catch {
        try queue.updateStatus(
            itemID: item.id,
            status: "failed",
            errorMessage: error.localizedDescription
        )
    }
}
```

### Android Example

```kotlin
val queue = OfflineQueue(
    dbPath = "${context.filesDir}/offline_queue.db",
    encryptionKey = keyStore.getEncryptionKey(),
    maxItems = 1000,
    maxAge = Duration.ofDays(30)
)

// Queue check-in when offline
val item = queue.enqueue(
    userID = currentUser.id,
    checkinData = mapOf(
        "content" to "Meeting notes",
        "category_id" to categoryID,
        "checkin_time" to Instant.now().toString(),
        "duration_minutes" to 30
    )
)

// Sync when online
queue.list(currentUser.id, "pending").forEach { item ->
    try {
        api.createCheckin(item.checkinData)
        queue.updateStatus(item.id, "synced", null)
    } catch (e: Exception) {
        queue.updateStatus(item.id, "failed", e.message)
    }
}
```

## Conclusion

This offline queue system provides:
- **Reliability**: SQLite is battle-tested and reliable
- **Security**: AES-256-GCM encryption protects data at rest
- **Performance**: Indexed queries and efficient operations
- **Scalability**: Configurable limits for different devices
- **Simplicity**: Straightforward API and clear data flow

The system is production-ready and can be integrated into mobile clients with minimal effort.
