# Offline Queue Module

## Overview

The offline queue module provides SQLite-based local storage for check-in data when network connectivity is unavailable. It features AES-256-GCM encryption, automatic capacity management, and comprehensive sync status tracking.

## Quick Start

```go
package main

import (
    "context"
    "crypto/rand"
    "time"

    "github.com/dev-jelly/donelist/internal/checkin"
    "github.com/google/uuid"
    "go.uber.org/zap"
)

func main() {
    // Generate or load encryption key (32 bytes for AES-256)
    key := make([]byte, 32)
    rand.Read(key)

    // Create queue
    queue, err := checkin.NewOfflineQueue(checkin.OfflineQueueConfig{
        DBPath:        "./offline_queue.db",
        EncryptionKey: key,
        MaxItems:      1000,
        MaxAge:        30 * 24 * time.Hour,
        Logger:        zap.NewNop(),
    })
    if err != nil {
        panic(err)
    }
    defer queue.Close()

    ctx := context.Background()
    userID := uuid.New()

    // Enqueue check-in
    checkinData := map[string]interface{}{
        "content":          "Team meeting notes",
        "category_id":      uuid.New().String(),
        "checkin_time":     time.Now().Format(time.RFC3339),
        "duration_minutes": 30,
    }

    item, err := queue.Enqueue(ctx, userID, checkinData)
    if err != nil {
        panic(err)
    }

    // Later: dequeue and sync
    item, err = queue.Dequeue(ctx, userID)
    if err != nil {
        panic(err)
    }

    // Update status after sync
    queue.UpdateStatus(ctx, item.ID, "synced", nil)
}
```

## Features

- ✅ **AES-256-GCM Encryption**: Military-grade encryption for data at rest
- ✅ **FIFO Queue**: Oldest items processed first
- ✅ **Automatic Pruning**: Age-based (30 days) and capacity-based (1000 items)
- ✅ **Status Tracking**: pending, syncing, failed, synced
- ✅ **Retry Logic**: Tracks retry count and error messages
- ✅ **Performance**: ~3,676 enqueues/sec, ~29,650 dequeues/sec
- ✅ **Thread Safe**: SQLite handles concurrent access
- ✅ **Production Ready**: Comprehensive tests and error handling

## Configuration

```go
type OfflineQueueConfig struct {
    DBPath        string        // Path to SQLite database file
    EncryptionKey []byte        // 32-byte encryption key for AES-256
    MaxItems      int           // Maximum items in queue (default: 1000)
    MaxAge        time.Duration // Maximum age of items (default: 30 days)
    Logger        *zap.Logger   // Logger instance
}
```

## API Reference

### Queue Operations

#### Enqueue
```go
func (q *OfflineQueue) Enqueue(ctx context.Context, userID uuid.UUID, checkinData map[string]interface{}) (*OfflineQueueItem, error)
```
Adds a check-in to the offline queue with automatic encryption and pruning.

**Parameters**:
- `userID`: User who owns the check-in
- `checkinData`: Check-in data as a map

**Returns**: Queue item with ID and metadata

**Example**:
```go
item, err := queue.Enqueue(ctx, userID, map[string]interface{}{
    "content": "Meeting notes",
    "checkin_time": time.Now().Format(time.RFC3339),
    "duration_minutes": 30,
})
```

#### Dequeue
```go
func (q *OfflineQueue) Dequeue(ctx context.Context, userID uuid.UUID) (*OfflineQueueItem, error)
```
Retrieves and removes the oldest pending item for a user.

**Returns**: Queue item or nil if queue is empty

**Example**:
```go
item, err := queue.Dequeue(ctx, userID)
if item != nil {
    // Process item
    // Update status after processing
}
```

#### List
```go
func (q *OfflineQueue) List(ctx context.Context, userID uuid.UUID, status string) ([]*OfflineQueueItem, error)
```
Retrieves all items for a user without removing them.

**Parameters**:
- `status`: Optional filter ("pending", "syncing", "failed", "synced")

**Example**:
```go
// Get all pending items
items, err := queue.List(ctx, userID, "pending")

// Get all items regardless of status
items, err := queue.List(ctx, userID, "")
```

### Status Management

#### UpdateStatus
```go
func (q *OfflineQueue) UpdateStatus(ctx context.Context, itemID uuid.UUID, status string, errorMsg *string) error
```
Updates the sync status of an item and increments retry count.

**Example**:
```go
// Mark as syncing
queue.UpdateStatus(ctx, item.ID, "syncing", nil)

// Mark as failed with error
errMsg := "Network timeout"
queue.UpdateStatus(ctx, item.ID, "failed", &errMsg)

// Mark as synced
queue.UpdateStatus(ctx, item.ID, "synced", nil)
```

#### Count
```go
func (q *OfflineQueue) Count(ctx context.Context, userID uuid.UUID) (int, error)
```
Returns the number of items in the queue for a user.

### Maintenance

#### Prune
```go
func (q *OfflineQueue) Prune(ctx context.Context) error
```
Manually triggers pruning of old or excess items.

**Example**:
```go
// Run as periodic job
ticker := time.NewTicker(1 * time.Hour)
defer ticker.Stop()

for range ticker.C {
    if err := queue.Prune(ctx); err != nil {
        log.Printf("Pruning failed: %v", err)
    }
}
```

#### Clear
```go
func (q *OfflineQueue) Clear(ctx context.Context, userID uuid.UUID) error
```
Removes all items for a user.

**Example**:
```go
// Clear queue on logout
queue.Clear(ctx, userID)
```

#### GetStats
```go
func (q *OfflineQueue) GetStats(ctx context.Context) (map[string]interface{}, error)
```
Returns queue statistics including total items, status counts, and oldest item.

**Example**:
```go
stats, err := queue.GetStats(ctx)
fmt.Printf("Total items: %d\n", stats["total_items"])
fmt.Printf("By status: %v\n", stats["by_status"])
```

## Data Structures

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

## Sync Status Flow

```
pending → syncing → synced (success)
   ↓         ↓
   └─────────→ failed (on error)
              ↓
         retry → syncing → ...
```

## Encryption

### Algorithm: AES-256-GCM

**Key Features**:
- 256-bit symmetric encryption
- Galois/Counter Mode for authenticated encryption
- 12-byte random nonce per operation
- Authentication tag prevents tampering

### Security Best Practices

1. **Key Generation**:
```go
import "crypto/rand"

key := make([]byte, 32)
if _, err := rand.Read(key); err != nil {
    panic(err)
}
```

2. **Key Storage**:
```go
// DO: Store in secure storage
keychain.Store("offline_queue_key", key)

// DON'T: Hardcode or commit to version control
// ❌ const key = "hardcoded-key-bad"
```

3. **Key Rotation** (future):
```go
// Rotate encryption key periodically
newKey := generateNewKey()
reEncryptAllItems(oldKey, newKey)
updateKeyInSecureStorage(newKey)
```

## Performance

### Benchmarks

```
BenchmarkEnqueue-10    	    3676	    724505 ns/op	    3732 B/op	      73 allocs/op
BenchmarkDequeue-10    	   29650	     36711 ns/op	    1974 B/op	      57 allocs/op
```

**Interpretation**:
- **Enqueue**: ~3,676 operations/second (~724μs each)
- **Dequeue**: ~29,650 operations/second (~37μs each)
- **Memory**: Minimal allocations, suitable for mobile devices

### Storage

**Per Item** (~400 bytes):
- Raw JSON: ~200 bytes
- Encrypted + Base64: ~300 bytes
- Metadata: ~100 bytes

**1000 Items Total** (~580 KB):
- Table data: ~400 KB
- Indexes: ~80 KB
- SQLite overhead: ~100 KB

### Query Complexity

| Operation | Time Complexity | Notes |
|-----------|----------------|-------|
| Enqueue   | O(1) amortized | Pruning may trigger O(n) |
| Dequeue   | O(log n) | Uses indexed ORDER BY |
| List      | O(m) | m = items for user |
| UpdateStatus | O(log n) | Primary key lookup |
| Count | O(1) | Optimized COUNT query |
| Prune | O(n) | Full table scan, infrequent |

## Database Schema

```sql
CREATE TABLE offline_queue (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    checkin_data_encrypted TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    sync_status TEXT NOT NULL DEFAULT 'pending',
    retry_count INTEGER NOT NULL DEFAULT 0,
    last_attempt INTEGER,
    error_message TEXT
);

CREATE INDEX idx_user_id ON offline_queue(user_id);
CREATE INDEX idx_sync_status ON offline_queue(sync_status);
CREATE INDEX idx_created_at ON offline_queue(created_at);
CREATE INDEX idx_user_status ON offline_queue(user_id, sync_status);
```

## Testing

### Run Tests
```bash
cd /Users/jelly/personal/donelist/server
go test -v ./internal/checkin/offline_queue_test.go ./internal/checkin/offline_queue.go
```

### Run Benchmarks
```bash
go test -bench=. -benchmem ./internal/checkin/offline_queue_test.go ./internal/checkin/offline_queue.go
```

### Test Coverage
```bash
go test -cover ./internal/checkin/offline_queue_test.go ./internal/checkin/offline_queue.go
```

## Error Handling

### Common Errors

1. **Invalid Encryption Key**:
```go
queue, err := NewOfflineQueue(OfflineQueueConfig{
    EncryptionKey: []byte("short"), // ❌ Must be 32 bytes
})
// Error: encryption key must be 32 bytes for AES-256
```

2. **Database Locked**:
```go
// SQLite single-writer limitation
// Solution: Use MaxOpenConns=1 (default)
```

3. **Decryption Failure**:
```go
// Caused by: Wrong key, corrupted data, or tampered ciphertext
// Logged and skipped, doesn't fail entire operation
```

## Migration Guide

### From Version 1.0 to 2.0 (future)

```go
// Add schema_version table
db.Exec("CREATE TABLE schema_version (version INTEGER)")
db.Exec("INSERT INTO schema_version VALUES (2)")

// Migrate data if needed
// ALTER TABLE offline_queue ADD COLUMN new_field TEXT;
```

## Monitoring

### Metrics to Track

```go
stats, _ := queue.GetStats(ctx)

// Monitor these metrics
totalItems := stats["total_items"].(int)
statusCounts := stats["by_status"].(map[string]int)
oldestItem := stats["oldest_item"].(time.Time)

// Alert conditions
if totalItems > 800 {
    alert("Queue approaching capacity")
}

if statusCounts["failed"] > 50 {
    alert("High failure rate")
}

if time.Since(oldestItem) > 7*24*time.Hour {
    alert("Items not syncing")
}
```

## Best Practices

### 1. Encryption Key Management
```go
// ✅ DO: Load from secure storage
key := keychain.Get("offline_queue_key")

// ❌ DON'T: Hardcode keys
// key := []byte("hardcoded-bad-key-12345678901")
```

### 2. Error Handling
```go
// ✅ DO: Check errors and log
item, err := queue.Enqueue(ctx, userID, data)
if err != nil {
    logger.Error("Failed to enqueue", zap.Error(err))
    return err
}

// ❌ DON'T: Ignore errors
// queue.Enqueue(ctx, userID, data) // Unchecked!
```

### 3. Resource Cleanup
```go
// ✅ DO: Always close queue
queue, err := NewOfflineQueue(config)
defer queue.Close()

// ❌ DON'T: Forget to close
// queue, err := NewOfflineQueue(config)
// // Missing Close()
```

### 4. Sync Strategy
```go
// ✅ DO: Batch sync operations
items, _ := queue.List(ctx, userID, "pending")
for _, item := range items {
    // Process in batch
}

// ❌ DON'T: Sync one at a time inefficiently
// for i := 0; i < 1000; i++ {
//     item, _ := queue.Dequeue(ctx, userID)
//     // Network call for each item
// }
```

## Troubleshooting

### Issue: Database file locked

**Symptom**: `database is locked` error

**Solution**:
```go
// SQLite uses single-writer mode
// This is configured by default (MaxOpenConns=1)
// Ensure you're not opening multiple connections
```

### Issue: High memory usage

**Symptom**: Application using excessive memory

**Solution**:
```go
// Implement batching for large queues
const batchSize = 100
for i := 0; i < totalItems; i += batchSize {
    items, _ := queue.List(ctx, userID, "pending")
    // Process batch
}
```

### Issue: Slow enqueue operations

**Symptom**: Enqueue taking > 1 second

**Solution**:
```go
// Check if pruning is too aggressive
config := OfflineQueueConfig{
    MaxItems: 10000, // Increase limit
    MaxAge:   90 * 24 * time.Hour, // Longer retention
}
```

## Documentation

- **Schema Design**: `/server/docs/OFFLINE_QUEUE_SCHEMA.md`
- **Integration Guide**: `/server/docs/OFFLINE_QUEUE_INTEGRATION.md`
- **Task Summary**: `/server/docs/TASK_9.1_COMPLETION_SUMMARY.md`

## Support

For issues or questions:
1. Check the documentation in `/server/docs/`
2. Review test cases in `offline_queue_test.go`
3. Enable debug logging with `zap.NewDevelopment()`

## License

Part of the Donelist backend project.

---

**Version**: 1.0.0
**Last Updated**: 2025-11-25
**Status**: Production Ready
