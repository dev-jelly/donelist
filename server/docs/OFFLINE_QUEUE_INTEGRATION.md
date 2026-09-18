# Offline Queue Integration Guide

## Overview

This guide provides examples for integrating the offline storage queue into mobile clients (iOS/Android) and the backend sync service.

## Backend Integration

### 1. Initialize Offline Queue Service

```go
package main

import (
    "context"
    "crypto/rand"
    "log"

    "github.com/dev-jelly/donelist/internal/checkin"
    "go.uber.org/zap"
)

func setupOfflineQueue(logger *zap.Logger) (*checkin.OfflineQueue, error) {
    // Generate or load encryption key (32 bytes for AES-256)
    // In production, load this from secure storage
    encryptionKey := make([]byte, 32)
    if _, err := rand.Read(encryptionKey); err != nil {
        return nil, err
    }

    queue, err := checkin.NewOfflineQueue(checkin.OfflineQueueConfig{
        DBPath:        "/path/to/offline_queue.db",
        EncryptionKey: encryptionKey,
        MaxItems:      1000,
        MaxAge:        30 * 24 * time.Hour, // 30 days
        Logger:        logger,
    })

    if err != nil {
        return nil, err
    }

    return queue, nil
}
```

### 2. Enqueue Check-in When Offline

```go
func handleOfflineCheckin(queue *checkin.OfflineQueue, userID uuid.UUID) error {
    ctx := context.Background()

    checkinData := map[string]interface{}{
        "content":          "Team standup notes",
        "category_id":      "uuid-string",
        "checkin_time":     time.Now().Format(time.RFC3339),
        "duration_minutes": 30,
        "tag_names":        []string{"work", "meeting"},
        "visibility":       "private",
    }

    item, err := queue.Enqueue(ctx, userID, checkinData)
    if err != nil {
        return fmt.Errorf("failed to queue check-in: %w", err)
    }

    log.Printf("Check-in queued: %s", item.ID)
    return nil
}
```

### 3. Sync Queued Items When Online

```go
func syncOfflineQueue(
    queue *checkin.OfflineQueue,
    checkinService *checkin.Service,
    userID uuid.UUID,
) error {
    ctx := context.Background()

    // Get all pending items for user
    items, err := queue.List(ctx, userID, "pending")
    if err != nil {
        return fmt.Errorf("failed to list queue: %w", err)
    }

    log.Printf("Syncing %d queued check-ins", len(items))

    for _, item := range items {
        // Mark as syncing
        if err := queue.UpdateStatus(ctx, item.ID, "syncing", nil); err != nil {
            log.Printf("Failed to update status: %v", err)
            continue
        }

        // Attempt to create check-in on server
        checkin, err := createCheckinFromQueueItem(checkinService, item)
        if err != nil {
            // Mark as failed with error message
            errMsg := err.Error()
            queue.UpdateStatus(ctx, item.ID, "failed", &errMsg)
            log.Printf("Failed to sync check-in %s: %v", item.ID, err)
            continue
        }

        // Successfully synced - remove from queue
        queue.Dequeue(ctx, userID)
        log.Printf("Successfully synced check-in: %s -> %s", item.ID, checkin.ID)
    }

    return nil
}

func createCheckinFromQueueItem(
    service *checkin.Service,
    item *checkin.OfflineQueueItem,
) (*checkin.Checkin, error) {
    ctx := context.Background()

    // Parse check-in data from queue item
    input := checkin.CreateInput{
        UserID:          item.UserID,
        Content:         item.CheckinData["content"].(string),
        DurationMinutes: int(item.CheckinData["duration_minutes"].(float64)),
    }

    // Parse optional fields
    if categoryID, ok := item.CheckinData["category_id"].(string); ok && categoryID != "" {
        id := uuid.MustParse(categoryID)
        input.CategoryID = &id
    }

    if checkinTime, ok := item.CheckinData["checkin_time"].(string); ok {
        t, err := time.Parse(time.RFC3339, checkinTime)
        if err == nil {
            input.CheckinTime = t
        } else {
            input.CheckinTime = time.Now()
        }
    }

    if tagNames, ok := item.CheckinData["tag_names"].([]interface{}); ok {
        for _, tag := range tagNames {
            input.TagNames = append(input.TagNames, tag.(string))
        }
    }

    return service.Create(ctx, input)
}
```

### 4. Background Sync Job

```go
func startBackgroundSync(queue *checkin.OfflineQueue, service *checkin.Service) {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()

    for range ticker.C {
        ctx := context.Background()

        // Get queue statistics
        stats, err := queue.GetStats(ctx)
        if err != nil {
            log.Printf("Failed to get queue stats: %v", err)
            continue
        }

        totalItems := stats["total_items"].(int)
        if totalItems == 0 {
            continue
        }

        log.Printf("Background sync: %d items in queue", totalItems)

        // In production, you'd need to get all users with pending items
        // and sync each user's queue
        // For now, this is a simplified example
    }
}
```

### 5. Automatic Pruning Job

```go
func startPruningJob(queue *checkin.OfflineQueue) {
    ticker := time.NewTicker(1 * time.Hour)
    defer ticker.Stop()

    for range ticker.C {
        ctx := context.Background()

        if err := queue.Prune(ctx); err != nil {
            log.Printf("Pruning job failed: %v", err)
        } else {
            log.Println("Pruning job completed successfully")
        }
    }
}
```

## iOS Integration

### 1. Swift Offline Queue Wrapper

```swift
import Foundation
import SQLite3

class OfflineQueue {
    private var db: OpaquePointer?
    private let encryptionKey: Data
    private let maxItems: Int
    private let maxAge: TimeInterval

    init(dbPath: String, encryptionKey: Data, maxItems: Int = 1000, maxAge: TimeInterval = 30*24*60*60) {
        self.encryptionKey = encryptionKey
        self.maxItems = maxItems
        self.maxAge = maxAge

        // Open database
        if sqlite3_open(dbPath, &db) != SQLITE_OK {
            print("Error opening database")
        }

        initSchema()
    }

    deinit {
        sqlite3_close(db)
    }

    private func initSchema() {
        let schema = """
        CREATE TABLE IF NOT EXISTS offline_queue (
            id TEXT PRIMARY KEY,
            user_id TEXT NOT NULL,
            checkin_data_encrypted TEXT NOT NULL,
            created_at INTEGER NOT NULL,
            sync_status TEXT NOT NULL DEFAULT 'pending',
            retry_count INTEGER NOT NULL DEFAULT 0,
            last_attempt INTEGER,
            error_message TEXT
        );
        CREATE INDEX IF NOT EXISTS idx_user_id ON offline_queue(user_id);
        CREATE INDEX IF NOT EXISTS idx_sync_status ON offline_queue(sync_status);
        CREATE INDEX IF NOT EXISTS idx_created_at ON offline_queue(created_at);
        """

        var error: UnsafeMutablePointer<Int8>?
        if sqlite3_exec(db, schema, nil, nil, &error) != SQLITE_OK {
            let errorString = String(cString: error!)
            print("Error creating schema: \(errorString)")
            sqlite3_free(error)
        }
    }

    func enqueue(userID: String, checkinData: [String: Any]) throws -> String {
        // Prune if needed
        try pruneIfNeeded()

        let itemID = UUID().uuidString
        let dataJSON = try JSONSerialization.data(withJSONObject: checkinData)

        // Encrypt data
        let encryptedData = try encrypt(data: dataJSON)
        let encryptedString = encryptedData.base64EncodedString()

        let query = """
        INSERT INTO offline_queue (id, user_id, checkin_data_encrypted, created_at, sync_status, retry_count)
        VALUES (?, ?, ?, ?, 'pending', 0)
        """

        var statement: OpaquePointer?
        defer { sqlite3_finalize(statement) }

        if sqlite3_prepare_v2(db, query, -1, &statement, nil) == SQLITE_OK {
            sqlite3_bind_text(statement, 1, itemID, -1, nil)
            sqlite3_bind_text(statement, 2, userID, -1, nil)
            sqlite3_bind_text(statement, 3, encryptedString, -1, nil)
            sqlite3_bind_int64(statement, 4, Int64(Date().timeIntervalSince1970))

            if sqlite3_step(statement) != SQLITE_DONE {
                throw OfflineQueueError.insertFailed
            }
        }

        return itemID
    }

    func list(userID: String, status: String? = nil) throws -> [[String: Any]] {
        var query = "SELECT id, user_id, checkin_data_encrypted, created_at, sync_status, retry_count FROM offline_queue WHERE user_id = ?"
        if let status = status {
            query += " AND sync_status = '\(status)'"
        }
        query += " ORDER BY created_at ASC"

        var statement: OpaquePointer?
        defer { sqlite3_finalize(statement) }

        var items: [[String: Any]] = []

        if sqlite3_prepare_v2(db, query, -1, &statement, nil) == SQLITE_OK {
            sqlite3_bind_text(statement, 1, userID, -1, nil)

            while sqlite3_step(statement) == SQLITE_ROW {
                let id = String(cString: sqlite3_column_text(statement, 0))
                let encryptedString = String(cString: sqlite3_column_text(statement, 2))
                let createdAt = sqlite3_column_int64(statement, 3)
                let syncStatus = String(cString: sqlite3_column_text(statement, 4))
                let retryCount = sqlite3_column_int(statement, 5)

                // Decrypt data
                guard let encryptedData = Data(base64Encoded: encryptedString) else {
                    continue
                }

                do {
                    let decryptedData = try decrypt(data: encryptedData)
                    let checkinData = try JSONSerialization.jsonObject(with: decryptedData) as? [String: Any]

                    items.append([
                        "id": id,
                        "user_id": userID,
                        "checkin_data": checkinData ?? [:],
                        "created_at": Date(timeIntervalSince1970: TimeInterval(createdAt)),
                        "sync_status": syncStatus,
                        "retry_count": retryCount
                    ])
                } catch {
                    print("Failed to decrypt item: \(error)")
                }
            }
        }

        return items
    }

    func updateStatus(itemID: String, status: String, errorMessage: String? = nil) throws {
        let query = """
        UPDATE offline_queue
        SET sync_status = ?,
            retry_count = retry_count + 1,
            last_attempt = ?,
            error_message = ?
        WHERE id = ?
        """

        var statement: OpaquePointer?
        defer { sqlite3_finalize(statement) }

        if sqlite3_prepare_v2(db, query, -1, &statement, nil) == SQLITE_OK {
            sqlite3_bind_text(statement, 1, status, -1, nil)
            sqlite3_bind_int64(statement, 2, Int64(Date().timeIntervalSince1970))
            if let errorMessage = errorMessage {
                sqlite3_bind_text(statement, 3, errorMessage, -1, nil)
            } else {
                sqlite3_bind_null(statement, 3)
            }
            sqlite3_bind_text(statement, 4, itemID, -1, nil)

            if sqlite3_step(statement) != SQLITE_DONE {
                throw OfflineQueueError.updateFailed
            }
        }
    }

    private func encrypt(data: Data) throws -> Data {
        // Implement AES-256-GCM encryption
        // Use CryptoKit on iOS 13+
        // This is a placeholder - actual implementation needed
        return data
    }

    private func decrypt(data: Data) throws -> Data {
        // Implement AES-256-GCM decryption
        // Use CryptoKit on iOS 13+
        // This is a placeholder - actual implementation needed
        return data
    }

    private func pruneIfNeeded() throws {
        // Prune old items
        let cutoffTime = Int64(Date().addingTimeInterval(-maxAge).timeIntervalSince1970)
        let deleteOld = "DELETE FROM offline_queue WHERE created_at < ?"

        var statement: OpaquePointer?
        if sqlite3_prepare_v2(db, deleteOld, -1, &statement, nil) == SQLITE_OK {
            sqlite3_bind_int64(statement, 1, cutoffTime)
            sqlite3_step(statement)
        }
        sqlite3_finalize(statement)

        // Prune excess items
        let countQuery = "SELECT COUNT(*) FROM offline_queue"
        var count: Int = 0

        if sqlite3_prepare_v2(db, countQuery, -1, &statement, nil) == SQLITE_OK {
            if sqlite3_step(statement) == SQLITE_ROW {
                count = Int(sqlite3_column_int(statement, 0))
            }
        }
        sqlite3_finalize(statement)

        if count > maxItems {
            let excess = count - maxItems
            let deleteExcess = """
            DELETE FROM offline_queue
            WHERE id IN (
                SELECT id FROM offline_queue
                ORDER BY created_at ASC
                LIMIT ?
            )
            """

            if sqlite3_prepare_v2(db, deleteExcess, -1, &statement, nil) == SQLITE_OK {
                sqlite3_bind_int(statement, 1, Int32(excess))
                sqlite3_step(statement)
            }
            sqlite3_finalize(statement)
        }
    }
}

enum OfflineQueueError: Error {
    case insertFailed
    case updateFailed
    case encryptionFailed
    case decryptionFailed
}
```

### 2. Usage in iOS App

```swift
// Initialize queue
let documentsPath = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask)[0]
let dbPath = documentsPath.appendingPathComponent("offline_queue.db").path

// Get encryption key from Keychain
let encryptionKey = KeychainManager.shared.getEncryptionKey()

let queue = OfflineQueue(
    dbPath: dbPath,
    encryptionKey: encryptionKey,
    maxItems: 1000,
    maxAge: 30*24*60*60
)

// Queue check-in when offline
do {
    let checkinData: [String: Any] = [
        "content": "Team meeting notes",
        "category_id": categoryID,
        "checkin_time": ISO8601DateFormatter().string(from: Date()),
        "duration_minutes": 30,
        "tag_names": ["work", "meeting"]
    ]

    let itemID = try queue.enqueue(userID: currentUser.id, checkinData: checkinData)
    print("Queued check-in: \(itemID)")
} catch {
    print("Failed to queue check-in: \(error)")
}

// Sync when online
func syncOfflineQueue() async {
    do {
        let items = try queue.list(userID: currentUser.id, status: "pending")

        for item in items {
            guard let itemID = item["id"] as? String,
                  let checkinData = item["checkin_data"] as? [String: Any] else {
                continue
            }

            do {
                // Mark as syncing
                try queue.updateStatus(itemID: itemID, status: "syncing")

                // Send to server
                try await apiClient.createCheckin(checkinData)

                // Mark as synced (or delete)
                try queue.updateStatus(itemID: itemID, status: "synced")

            } catch {
                try queue.updateStatus(
                    itemID: itemID,
                    status: "failed",
                    errorMessage: error.localizedDescription
                )
            }
        }
    } catch {
        print("Sync failed: \(error)")
    }
}
```

## Android Integration

### 1. Kotlin Offline Queue

```kotlin
import android.content.Context
import android.database.sqlite.SQLiteDatabase
import android.database.sqlite.SQLiteOpenHelper
import org.json.JSONObject
import java.util.*
import javax.crypto.Cipher
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.SecretKeySpec

class OfflineQueue(
    context: Context,
    private val encryptionKey: ByteArray,
    private val maxItems: Int = 1000,
    private val maxAge: Long = 30L * 24 * 60 * 60 * 1000 // 30 days in milliseconds
) : SQLiteOpenHelper(context, DATABASE_NAME, null, DATABASE_VERSION) {

    companion object {
        private const val DATABASE_NAME = "offline_queue.db"
        private const val DATABASE_VERSION = 1
        private const val TABLE_NAME = "offline_queue"
    }

    override fun onCreate(db: SQLiteDatabase) {
        val createTable = """
            CREATE TABLE $TABLE_NAME (
                id TEXT PRIMARY KEY,
                user_id TEXT NOT NULL,
                checkin_data_encrypted TEXT NOT NULL,
                created_at INTEGER NOT NULL,
                sync_status TEXT NOT NULL DEFAULT 'pending',
                retry_count INTEGER NOT NULL DEFAULT 0,
                last_attempt INTEGER,
                error_message TEXT
            )
        """.trimIndent()

        db.execSQL(createTable)
        db.execSQL("CREATE INDEX idx_user_id ON $TABLE_NAME(user_id)")
        db.execSQL("CREATE INDEX idx_sync_status ON $TABLE_NAME(sync_status)")
        db.execSQL("CREATE INDEX idx_created_at ON $TABLE_NAME(created_at)")
    }

    override fun onUpgrade(db: SQLiteDatabase, oldVersion: Int, newVersion: Int) {
        db.execSQL("DROP TABLE IF EXISTS $TABLE_NAME")
        onCreate(db)
    }

    fun enqueue(userID: String, checkinData: Map<String, Any>): String {
        pruneIfNeeded()

        val itemID = UUID.randomUUID().toString()
        val json = JSONObject(checkinData).toString()
        val encryptedData = encrypt(json.toByteArray())
        val encryptedString = Base64.getEncoder().encodeToString(encryptedData)

        val db = writableDatabase
        db.execSQL(
            """
            INSERT INTO $TABLE_NAME (id, user_id, checkin_data_encrypted, created_at, sync_status, retry_count)
            VALUES (?, ?, ?, ?, 'pending', 0)
            """.trimIndent(),
            arrayOf(itemID, userID, encryptedString, System.currentTimeMillis())
        )

        return itemID
    }

    fun list(userID: String, status: String? = null): List<Map<String, Any>> {
        val query = buildString {
            append("SELECT * FROM $TABLE_NAME WHERE user_id = ?")
            if (status != null) {
                append(" AND sync_status = '$status'")
            }
            append(" ORDER BY created_at ASC")
        }

        val db = readableDatabase
        val cursor = db.rawQuery(query, arrayOf(userID))
        val items = mutableListOf<Map<String, Any>>()

        cursor.use {
            while (it.moveToNext()) {
                val id = it.getString(it.getColumnIndexOrThrow("id"))
                val encryptedString = it.getString(it.getColumnIndexOrThrow("checkin_data_encrypted"))
                val createdAt = it.getLong(it.getColumnIndexOrThrow("created_at"))
                val syncStatus = it.getString(it.getColumnIndexOrThrow("sync_status"))
                val retryCount = it.getInt(it.getColumnIndexOrThrow("retry_count"))

                try {
                    val encryptedData = Base64.getDecoder().decode(encryptedString)
                    val decryptedData = decrypt(encryptedData)
                    val checkinData = JSONObject(String(decryptedData))

                    items.add(mapOf(
                        "id" to id,
                        "user_id" to userID,
                        "checkin_data" to checkinData,
                        "created_at" to Date(createdAt),
                        "sync_status" to syncStatus,
                        "retry_count" to retryCount
                    ))
                } catch (e: Exception) {
                    // Log decryption error
                }
            }
        }

        return items
    }

    fun updateStatus(itemID: String, status: String, errorMessage: String? = null) {
        val db = writableDatabase
        db.execSQL(
            """
            UPDATE $TABLE_NAME
            SET sync_status = ?,
                retry_count = retry_count + 1,
                last_attempt = ?,
                error_message = ?
            WHERE id = ?
            """.trimIndent(),
            arrayOf(status, System.currentTimeMillis(), errorMessage, itemID)
        )
    }

    private fun encrypt(data: ByteArray): ByteArray {
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        val secretKey = SecretKeySpec(encryptionKey, "AES")
        cipher.init(Cipher.ENCRYPT_MODE, secretKey)

        val iv = cipher.iv
        val encryptedData = cipher.doFinal(data)

        // Prepend IV to encrypted data
        return iv + encryptedData
    }

    private fun decrypt(data: ByteArray): ByteArray {
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        val secretKey = SecretKeySpec(encryptionKey, "AES")

        val iv = data.take(12).toByteArray()
        val encryptedData = data.drop(12).toByteArray()

        val spec = GCMParameterSpec(128, iv)
        cipher.init(Cipher.DECRYPT_MODE, secretKey, spec)

        return cipher.doFinal(encryptedData)
    }

    private fun pruneIfNeeded() {
        val db = writableDatabase

        // Prune old items
        val cutoffTime = System.currentTimeMillis() - maxAge
        db.execSQL("DELETE FROM $TABLE_NAME WHERE created_at < ?", arrayOf(cutoffTime))

        // Prune excess items
        val cursor = db.rawQuery("SELECT COUNT(*) FROM $TABLE_NAME", null)
        cursor.use {
            if (it.moveToFirst()) {
                val count = it.getInt(0)
                if (count > maxItems) {
                    val excess = count - maxItems
                    db.execSQL(
                        """
                        DELETE FROM $TABLE_NAME
                        WHERE id IN (
                            SELECT id FROM $TABLE_NAME
                            ORDER BY created_at ASC
                            LIMIT ?
                        )
                        """.trimIndent(),
                        arrayOf(excess)
                    )
                }
            }
        }
    }
}
```

### 2. Usage in Android App

```kotlin
// Initialize queue
val encryptionKey = KeyStoreManager.getEncryptionKey()
val queue = OfflineQueue(context, encryptionKey)

// Queue check-in when offline
val checkinData = mapOf(
    "content" to "Team meeting notes",
    "category_id" to categoryID,
    "checkin_time" to Instant.now().toString(),
    "duration_minutes" to 30,
    "tag_names" to listOf("work", "meeting")
)

val itemID = queue.enqueue(currentUser.id, checkinData)
Log.d("OfflineQueue", "Queued check-in: $itemID")

// Sync when online
suspend fun syncOfflineQueue() {
    val items = queue.list(currentUser.id, "pending")

    items.forEach { item ->
        val itemID = item["id"] as String
        val checkinData = item["checkin_data"] as JSONObject

        try {
            queue.updateStatus(itemID, "syncing")

            apiClient.createCheckin(checkinData)

            queue.updateStatus(itemID, "synced")
        } catch (e: Exception) {
            queue.updateStatus(itemID, "failed", e.message)
        }
    }
}
```

## Best Practices

### Security
1. **Encryption Key Management**:
   - Store encryption keys in secure storage (Keychain on iOS, KeyStore on Android)
   - Never hardcode encryption keys
   - Consider key rotation policy

2. **Data Privacy**:
   - Encrypt all check-in data
   - Clear queue on logout
   - Implement secure deletion

### Performance
1. **Batching**:
   - Sync multiple items in a single network request
   - Use background tasks for sync operations

2. **Error Handling**:
   - Implement exponential backoff for retries
   - Set maximum retry count
   - Alert user of persistent sync failures

3. **Resource Management**:
   - Close database connections properly
   - Monitor storage usage
   - Implement periodic cleanup

### User Experience
1. **Feedback**:
   - Show sync status in UI
   - Indicate number of pending items
   - Provide manual sync trigger

2. **Conflict Resolution**:
   - Handle server-side conflicts gracefully
   - Inform users of sync failures
   - Allow users to review failed items

3. **Offline Indicators**:
   - Display offline mode indicator
   - Show queue status
   - Automatic sync when online

## Troubleshooting

### Common Issues

1. **Encryption/Decryption Errors**:
   - Verify encryption key is 32 bytes
   - Check key is correctly loaded from storage
   - Ensure nonce size is correct (12 bytes for GCM)

2. **Database Locked**:
   - SQLite single-writer limitation
   - Use connection pooling with MaxOpenConns=1
   - Implement proper transaction handling

3. **Sync Failures**:
   - Check network connectivity
   - Verify API authentication
   - Review server-side validation errors

4. **Memory Issues**:
   - Batch large sync operations
   - Implement pagination for list operations
   - Monitor queue size limits

## Monitoring

### Metrics to Track

1. **Queue Health**:
   - Number of pending items
   - Average retry count
   - Failed sync rate

2. **Performance**:
   - Enqueue latency
   - Dequeue latency
   - Sync success rate

3. **Storage**:
   - Database file size
   - Number of pruned items
   - Oldest item age

## Conclusion

The offline queue system enables robust offline support for mobile applications with:
- Secure encrypted storage
- Automatic synchronization
- Configurable capacity limits
- Comprehensive error handling

Follow this integration guide to implement offline check-in functionality in your mobile apps.
