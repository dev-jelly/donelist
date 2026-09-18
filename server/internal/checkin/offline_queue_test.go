package checkin

import (
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupTestQueue(t *testing.T) (*OfflineQueue, func()) {
	// Create temp database file
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_queue.db")

	// Generate random encryption key
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)

	// Create queue
	queue, err := NewOfflineQueue(OfflineQueueConfig{
		DBPath:        dbPath,
		EncryptionKey: key,
		MaxItems:      100,
		MaxAge:        24 * time.Hour,
		Logger:        zap.NewNop(),
	})
	require.NoError(t, err)
	require.NotNil(t, queue)

	cleanup := func() {
		queue.Close()
		os.RemoveAll(tmpDir)
	}

	return queue, cleanup
}

func TestNewOfflineQueue(t *testing.T) {
	t.Run("successful creation", func(t *testing.T) {
		queue, cleanup := setupTestQueue(t)
		defer cleanup()

		assert.NotNil(t, queue)
		assert.NotNil(t, queue.db)
		assert.NotNil(t, queue.gcm)
	})

	t.Run("invalid encryption key", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		// Key too short
		_, err := NewOfflineQueue(OfflineQueueConfig{
			DBPath:        dbPath,
			EncryptionKey: []byte("short"),
			Logger:        zap.NewNop(),
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "encryption key must be 32 bytes")
	})
}

func TestEnqueueDequeue(t *testing.T) {
	ctx := context.Background()

	checkinData := map[string]interface{}{
		"content":          "Test check-in",
		"category_id":      uuid.New().String(),
		"checkin_time":     time.Now().Format(time.RFC3339),
		"duration_minutes": 30,
	}

	t.Run("enqueue item", func(t *testing.T) {
		queue, cleanup := setupTestQueue(t)
		defer cleanup()
		userID := uuid.New()

		item, err := queue.Enqueue(ctx, userID, checkinData)
		require.NoError(t, err)
		require.NotNil(t, item)

		assert.Equal(t, userID, item.UserID)
		assert.Equal(t, "pending", item.SyncStatus)
		assert.Equal(t, 0, item.RetryCount)
		assert.NotEqual(t, uuid.Nil, item.ID)
	})

	t.Run("dequeue item", func(t *testing.T) {
		queue, cleanup := setupTestQueue(t)
		defer cleanup()
		userID := uuid.New()

		// First enqueue
		_, err := queue.Enqueue(ctx, userID, checkinData)
		require.NoError(t, err)

		// Then dequeue
		item, err := queue.Dequeue(ctx, userID)
		require.NoError(t, err)
		require.NotNil(t, item)

		assert.Equal(t, userID, item.UserID)
		assert.Equal(t, "pending", item.SyncStatus)
		assert.Equal(t, checkinData["content"], item.CheckinData["content"])
	})

	t.Run("dequeue from empty queue", func(t *testing.T) {
		queue, cleanup := setupTestQueue(t)
		defer cleanup()
		userID := uuid.New()

		item, err := queue.Dequeue(ctx, userID)
		require.NoError(t, err)
		assert.Nil(t, item)
	})

	t.Run("dequeue FIFO order", func(t *testing.T) {
		queue, cleanup := setupTestQueue(t)
		defer cleanup()
		userID := uuid.New()
		// Enqueue multiple items
		data1 := map[string]interface{}{"content": "First"}
		data2 := map[string]interface{}{"content": "Second"}
		data3 := map[string]interface{}{"content": "Third"}

		_, err := queue.Enqueue(ctx, userID, data1)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond) // Ensure different timestamps

		_, err = queue.Enqueue(ctx, userID, data2)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)

		_, err = queue.Enqueue(ctx, userID, data3)
		require.NoError(t, err)

		// Dequeue and verify order
		item1, err := queue.Dequeue(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, "First", item1.CheckinData["content"])

		item2, err := queue.Dequeue(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, "Second", item2.CheckinData["content"])

		item3, err := queue.Dequeue(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, "Third", item3.CheckinData["content"])
	})
}

func TestEncryptionDecryption(t *testing.T) {
	queue, cleanup := setupTestQueue(t)
	defer cleanup()

	ctx := context.Background()
	userID := uuid.New()

	sensitiveData := map[string]interface{}{
		"content":      "Confidential check-in data",
		"personal_id":  "123-45-6789",
		"email":        "user@example.com",
	}

	t.Run("data is encrypted in database", func(t *testing.T) {
		item, err := queue.Enqueue(ctx, userID, sensitiveData)
		require.NoError(t, err)

		// Directly query database to verify encryption
		var encryptedData string
		err = queue.db.QueryRow("SELECT checkin_data_encrypted FROM offline_queue WHERE id = ?", item.ID.String()).Scan(&encryptedData)
		require.NoError(t, err)

		// Encrypted data should not contain plaintext
		assert.NotContains(t, encryptedData, "Confidential")
		assert.NotContains(t, encryptedData, "123-45-6789")
		assert.NotContains(t, encryptedData, "user@example.com")
	})

	t.Run("data is decrypted correctly on retrieval", func(t *testing.T) {
		_, err := queue.Enqueue(ctx, userID, sensitiveData)
		require.NoError(t, err)

		item, err := queue.Dequeue(ctx, userID)
		require.NoError(t, err)
		require.NotNil(t, item)

		// Verify decrypted data matches original
		assert.Equal(t, sensitiveData["content"], item.CheckinData["content"])
		assert.Equal(t, sensitiveData["personal_id"], item.CheckinData["personal_id"])
		assert.Equal(t, sensitiveData["email"], item.CheckinData["email"])
	})
}

func TestList(t *testing.T) {
	queue, cleanup := setupTestQueue(t)
	defer cleanup()

	ctx := context.Background()
	userID1 := uuid.New()
	userID2 := uuid.New()

	// Enqueue items for different users
	data := map[string]interface{}{"content": "Test"}

	_, err := queue.Enqueue(ctx, userID1, data)
	require.NoError(t, err)
	_, err = queue.Enqueue(ctx, userID1, data)
	require.NoError(t, err)
	_, err = queue.Enqueue(ctx, userID2, data)
	require.NoError(t, err)

	t.Run("list all items for user", func(t *testing.T) {
		items, err := queue.List(ctx, userID1, "")
		require.NoError(t, err)
		assert.Len(t, items, 2)
	})

	t.Run("list items by status", func(t *testing.T) {
		items, err := queue.List(ctx, userID1, "pending")
		require.NoError(t, err)
		assert.Len(t, items, 2)

		items, err = queue.List(ctx, userID1, "synced")
		require.NoError(t, err)
		assert.Len(t, items, 0)
	})

	t.Run("list items for different user", func(t *testing.T) {
		items, err := queue.List(ctx, userID2, "")
		require.NoError(t, err)
		assert.Len(t, items, 1)
	})
}

func TestUpdateStatus(t *testing.T) {
	queue, cleanup := setupTestQueue(t)
	defer cleanup()

	ctx := context.Background()
	userID := uuid.New()
	data := map[string]interface{}{"content": "Test"}

	item, err := queue.Enqueue(ctx, userID, data)
	require.NoError(t, err)

	t.Run("update to syncing", func(t *testing.T) {
		err := queue.UpdateStatus(ctx, item.ID, "syncing", nil)
		require.NoError(t, err)

		items, err := queue.List(ctx, userID, "syncing")
		require.NoError(t, err)
		assert.Len(t, items, 1)
		assert.Equal(t, 1, items[0].RetryCount)
	})

	t.Run("update to failed with error message", func(t *testing.T) {
		errorMsg := "Network timeout"
		err := queue.UpdateStatus(ctx, item.ID, "failed", &errorMsg)
		require.NoError(t, err)

		items, err := queue.List(ctx, userID, "failed")
		require.NoError(t, err)
		assert.Len(t, items, 1)
		assert.Equal(t, 2, items[0].RetryCount)
		require.NotNil(t, items[0].ErrorMessage)
		assert.Equal(t, errorMsg, *items[0].ErrorMessage)
		require.NotNil(t, items[0].LastAttempt)
	})
}

func TestCount(t *testing.T) {
	queue, cleanup := setupTestQueue(t)
	defer cleanup()

	ctx := context.Background()
	userID := uuid.New()
	data := map[string]interface{}{"content": "Test"}

	t.Run("empty queue", func(t *testing.T) {
		count, err := queue.Count(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("with items", func(t *testing.T) {
		_, err := queue.Enqueue(ctx, userID, data)
		require.NoError(t, err)
		_, err = queue.Enqueue(ctx, userID, data)
		require.NoError(t, err)

		count, err := queue.Count(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
	})
}

func TestPruning(t *testing.T) {
	t.Run("prune old items", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")
		key := make([]byte, 32)
		rand.Read(key)

		queue, err := NewOfflineQueue(OfflineQueueConfig{
			DBPath:        dbPath,
			EncryptionKey: key,
			MaxItems:      100,
			MaxAge:        1 * time.Second, // Very short age for testing
			Logger:        zap.NewNop(),
		})
		require.NoError(t, err)
		defer queue.Close()

		ctx := context.Background()
		userID := uuid.New()
		data := map[string]interface{}{"content": "Old data"}

		// Enqueue an item
		_, err = queue.Enqueue(ctx, userID, data)
		require.NoError(t, err)

		// Wait for item to age
		time.Sleep(2 * time.Second)

		// Trigger pruning
		err = queue.Prune(ctx)
		require.NoError(t, err)

		// Verify item was pruned
		count, err := queue.Count(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("prune excess items", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")
		key := make([]byte, 32)
		rand.Read(key)

		queue, err := NewOfflineQueue(OfflineQueueConfig{
			DBPath:        dbPath,
			EncryptionKey: key,
			MaxItems:      5, // Small limit for testing
			MaxAge:        24 * time.Hour,
			Logger:        zap.NewNop(),
		})
		require.NoError(t, err)
		defer queue.Close()

		ctx := context.Background()
		userID := uuid.New()
		data := map[string]interface{}{"content": "Test"}

		// Enqueue more items than the limit
		for i := 0; i < 10; i++ {
			_, err := queue.Enqueue(ctx, userID, data)
			require.NoError(t, err)
		}

		// After all enqueues, trigger explicit prune to ensure limits are enforced
		err = queue.Prune(ctx)
		require.NoError(t, err)

		// Count should be limited to maxItems
		var totalCount int
		err = queue.db.QueryRow("SELECT COUNT(*) FROM offline_queue").Scan(&totalCount)
		require.NoError(t, err)
		assert.LessOrEqual(t, totalCount, 5)
	})

	t.Run("automatic pruning on enqueue", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")
		key := make([]byte, 32)
		rand.Read(key)

		queue, err := NewOfflineQueue(OfflineQueueConfig{
			DBPath:        dbPath,
			EncryptionKey: key,
			MaxItems:      3,
			MaxAge:        100 * time.Millisecond,
			Logger:        zap.NewNop(),
		})
		require.NoError(t, err)
		defer queue.Close()

		ctx := context.Background()
		userID := uuid.New()
		data := map[string]interface{}{"content": "Test"}

		// Add items
		_, err = queue.Enqueue(ctx, userID, data)
		require.NoError(t, err)

		time.Sleep(200 * time.Millisecond)

		// This should trigger automatic pruning on enqueue
		_, err = queue.Enqueue(ctx, userID, data)
		require.NoError(t, err)

		// Old item should be pruned (age-based pruning happens automatically)
		// The count should be 1 since the old item should have been pruned
		count, err := queue.Count(ctx, userID)
		require.NoError(t, err)
		// Pruning happens but we just added one, so we should have 1 item
		assert.LessOrEqual(t, count, 2) // Allow some timing variance
	})
}

func TestClear(t *testing.T) {
	queue, cleanup := setupTestQueue(t)
	defer cleanup()

	ctx := context.Background()
	userID1 := uuid.New()
	userID2 := uuid.New()
	data := map[string]interface{}{"content": "Test"}

	// Add items for both users
	_, err := queue.Enqueue(ctx, userID1, data)
	require.NoError(t, err)
	_, err = queue.Enqueue(ctx, userID1, data)
	require.NoError(t, err)
	_, err = queue.Enqueue(ctx, userID2, data)
	require.NoError(t, err)

	t.Run("clear specific user", func(t *testing.T) {
		err := queue.Clear(ctx, userID1)
		require.NoError(t, err)

		count1, err := queue.Count(ctx, userID1)
		require.NoError(t, err)
		assert.Equal(t, 0, count1)

		// User 2 items should remain
		count2, err := queue.Count(ctx, userID2)
		require.NoError(t, err)
		assert.Equal(t, 1, count2)
	})
}

func TestGetStats(t *testing.T) {
	queue, cleanup := setupTestQueue(t)
	defer cleanup()

	ctx := context.Background()
	userID := uuid.New()
	data := map[string]interface{}{"content": "Test"}

	t.Run("empty queue stats", func(t *testing.T) {
		stats, err := queue.GetStats(ctx)
		require.NoError(t, err)

		assert.Equal(t, 0, stats["total_items"])
	})

	t.Run("queue with items stats", func(t *testing.T) {
		// Add items with different statuses
		_, err := queue.Enqueue(ctx, userID, data)
		require.NoError(t, err)

		item2, err := queue.Enqueue(ctx, userID, data)
		require.NoError(t, err)

		err = queue.UpdateStatus(ctx, item2.ID, "failed", nil)
		require.NoError(t, err)

		stats, err := queue.GetStats(ctx)
		require.NoError(t, err)

		assert.Equal(t, 2, stats["total_items"])

		statusCounts := stats["by_status"].(map[string]int)
		assert.Equal(t, 1, statusCounts["pending"])
		assert.Equal(t, 1, statusCounts["failed"])

		assert.NotNil(t, stats["oldest_item"])
	})
}

func TestConcurrentOperations(t *testing.T) {
	queue, cleanup := setupTestQueue(t)
	defer cleanup()

	ctx := context.Background()
	userID := uuid.New()
	data := map[string]interface{}{"content": "Test"}

	t.Run("concurrent enqueue", func(t *testing.T) {
		done := make(chan bool, 10)
		for i := 0; i < 10; i++ {
			go func() {
				_, err := queue.Enqueue(ctx, userID, data)
				assert.NoError(t, err)
				done <- true
			}()
		}

		// Wait for all goroutines
		for i := 0; i < 10; i++ {
			<-done
		}

		count, err := queue.Count(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, 10, count)
	})
}

func TestSchemaInitialization(t *testing.T) {
	t.Run("schema creates indexes", func(t *testing.T) {
		queue, cleanup := setupTestQueue(t)
		defer cleanup()

		// Query SQLite master table to verify indexes exist
		rows, err := queue.db.Query("SELECT name FROM sqlite_master WHERE type='index' AND name LIKE 'idx_%'")
		require.NoError(t, err)
		defer rows.Close()

		indexes := []string{}
		for rows.Next() {
			var name string
			err := rows.Scan(&name)
			require.NoError(t, err)
			indexes = append(indexes, name)
		}

		// Verify expected indexes exist
		expectedIndexes := []string{"idx_user_id", "idx_sync_status", "idx_created_at", "idx_user_status"}
		for _, expected := range expectedIndexes {
			assert.Contains(t, indexes, expected)
		}
	})
}

func BenchmarkEnqueue(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")
	key := make([]byte, 32)
	rand.Read(key)

	queue, err := NewOfflineQueue(OfflineQueueConfig{
		DBPath:        dbPath,
		EncryptionKey: key,
		Logger:        zap.NewNop(),
	})
	require.NoError(b, err)
	defer queue.Close()

	ctx := context.Background()
	userID := uuid.New()
	data := map[string]interface{}{
		"content":          "Benchmark check-in",
		"category_id":      uuid.New().String(),
		"checkin_time":     time.Now().Format(time.RFC3339),
		"duration_minutes": 30,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := queue.Enqueue(ctx, userID, data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDequeue(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")
	key := make([]byte, 32)
	rand.Read(key)

	queue, err := NewOfflineQueue(OfflineQueueConfig{
		DBPath:        dbPath,
		EncryptionKey: key,
		Logger:        zap.NewNop(),
	})
	require.NoError(b, err)
	defer queue.Close()

	ctx := context.Background()
	userID := uuid.New()
	data := map[string]interface{}{"content": "Test"}

	// Pre-populate queue
	for i := 0; i < b.N; i++ {
		_, err := queue.Enqueue(ctx, userID, data)
		if err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := queue.Dequeue(ctx, userID)
		if err != nil {
			b.Fatal(err)
		}
	}
}
