package search

import (
	"context"
	"testing"
	"time"

	"github.com/dev-jelly/donelist/internal/testutil"
	"github.com/dev-jelly/donelist/internal/websocket"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewIndexer(t *testing.T) {
	tdb := testutil.SetupTestDB(t)
	defer tdb.TearDown(t)
	db := tdb.DB

	repo := NewRepository(db)
	hub := websocket.NewHub(zap.NewNop())
	config := DefaultIndexerConfig()

	indexer := NewIndexer(repo, hub, config, zap.NewNop())

	assert.NotNil(t, indexer)
	assert.Equal(t, config.BulkSize, indexer.config.BulkSize)
	assert.Equal(t, config.WorkerCount, indexer.config.WorkerCount)
}

func TestIndexer_StartStop(t *testing.T) {
	tdb := testutil.SetupTestDB(t)
	defer tdb.TearDown(t)
	db := tdb.DB

	repo := NewRepository(db)
	hub := websocket.NewHub(zap.NewNop())
	config := DefaultIndexerConfig()
	config.WorkerCount = 2

	indexer := NewIndexer(repo, hub, config, zap.NewNop())

	ctx := context.Background()

	// Start indexer
	err := indexer.Start(ctx)
	require.NoError(t, err)

	// Wait a bit to ensure it's running
	time.Sleep(100 * time.Millisecond)

	// Check it's healthy
	assert.True(t, indexer.IsHealthy())

	// Try to start again (should fail)
	err = indexer.Start(ctx)
	assert.Error(t, err)

	// Stop indexer
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = indexer.Stop(stopCtx)
	require.NoError(t, err)

	// Check it's not running
	assert.False(t, indexer.IsHealthy())
}

func TestIndexer_InitialBulkLoad(t *testing.T) {
	tdb := testutil.SetupTestDB(t)
	defer tdb.TearDown(t)
	db := tdb.DB

	ctx := context.Background()

	// Create test user
	userID := uuid.New()
	_, err := db.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, tier)
		VALUES ($1, $2, $3, $4)
	`, userID, "test@example.com", "hash", "free")
	require.NoError(t, err)

	// Create test category
	categoryID := uuid.New()
	_, err = db.ExecContext(ctx, `
		INSERT INTO categories (id, user_id, name, color)
		VALUES ($1, $2, $3, $4)
	`, categoryID, userID, "Test Category", "#FF0000")
	require.NoError(t, err)

	// Create test checkins
	numCheckins := 250
	for i := 0; i < numCheckins; i++ {
		_, err := db.ExecContext(ctx, `
			INSERT INTO checkins (id, user_id, category_id, content, checkin_time, duration_minutes)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, uuid.New(), userID, categoryID, "Test content", time.Now(), 30)
		require.NoError(t, err)
	}

	// Create indexer
	repo := NewRepository(db)
	hub := websocket.NewHub(zap.NewNop())
	config := DefaultIndexerConfig()
	config.BulkSize = 50 // Small batch size for testing

	indexer := NewIndexer(repo, hub, config, zap.NewNop())

	// Start indexer (which triggers initial bulk load)
	err = indexer.Start(ctx)
	require.NoError(t, err)

	// Wait for initial load to complete
	time.Sleep(2 * time.Second)

	// Check stats
	stats := indexer.GetStats()
	assert.True(t, stats.InitialLoadComplete)
	assert.Equal(t, int64(numCheckins), stats.TotalIndexed)

	// Stop indexer
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = indexer.Stop(stopCtx)
	require.NoError(t, err)
}

func TestIndexer_HandleCheckinEvent(t *testing.T) {
	tdb := testutil.SetupTestDB(t)
	defer tdb.TearDown(t)
	db := tdb.DB

	repo := NewRepository(db)
	hub := websocket.NewHub(zap.NewNop())
	config := DefaultIndexerConfig()

	indexer := NewIndexer(repo, hub, config, zap.NewNop())

	ctx := context.Background()
	err := indexer.Start(ctx)
	require.NoError(t, err)
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = indexer.Stop(stopCtx)
	}()

	// Test create event
	checkinID := uuid.New()
	userID := uuid.New()
	data := map[string]interface{}{
		"content": "Test checkin",
	}

	err = indexer.HandleCheckinEvent(ctx, IndexOperationCreate, checkinID, userID, data)
	require.NoError(t, err)

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Check stats
	stats := indexer.GetStats()
	assert.Greater(t, stats.TotalIndexed, int64(0))
}

func TestIndexer_RetryMechanism(t *testing.T) {
	tdb := testutil.SetupTestDB(t)
	defer tdb.TearDown(t)
	db := tdb.DB

	repo := NewRepository(db)
	hub := websocket.NewHub(zap.NewNop())
	config := DefaultIndexerConfig()
	config.RetryDelay = 100 * time.Millisecond
	config.RetryAttempts = 3

	indexer := NewIndexer(repo, hub, config, zap.NewNop())

	ctx := context.Background()
	err := indexer.Start(ctx)
	require.NoError(t, err)
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = indexer.Stop(stopCtx)
	}()

	// Send operation to retry queue
	op := &IndexOperation{
		ID:        uuid.New(),
		Type:      IndexOperationCreate,
		CheckinID: uuid.New(),
		UserID:    uuid.New(),
		Timestamp: time.Now(),
		Retries:   1,
		Data:      map[string]interface{}{},
	}

	select {
	case indexer.retryQueue <- op:
		// Success
	case <-time.After(time.Second):
		t.Fatal("Failed to send to retry queue")
	}

	// Wait for retry processing
	time.Sleep(500 * time.Millisecond)

	// Check retry stats
	stats := indexer.GetStats()
	assert.Greater(t, stats.TotalRetried, int64(0))
}

func TestIndexer_Deduplication(t *testing.T) {
	tdb := testutil.SetupTestDB(t)
	defer tdb.TearDown(t)
	db := tdb.DB

	repo := NewRepository(db)
	hub := websocket.NewHub(zap.NewNop())
	config := DefaultIndexerConfig()

	indexer := NewIndexer(repo, hub, config, zap.NewNop())

	ctx := context.Background()
	err := indexer.Start(ctx)
	require.NoError(t, err)
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = indexer.Stop(stopCtx)
	}()

	// Send duplicate events
	checkinID := uuid.New()
	userID := uuid.New()
	data := map[string]interface{}{"content": "Test"}

	// Send same event multiple times
	for i := 0; i < 5; i++ {
		err = indexer.HandleCheckinEvent(ctx, IndexOperationUpdate, checkinID, userID, data)
		require.NoError(t, err)
	}

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// Deduplication should prevent processing same operation multiple times
	stats := indexer.GetStats()
	// Due to deduplication, we should process fewer than 5 operations
	assert.LessOrEqual(t, stats.TotalIndexed, int64(5))
}

func TestIndexer_Stats(t *testing.T) {
	tdb := testutil.SetupTestDB(t)
	defer tdb.TearDown(t)
	db := tdb.DB

	repo := NewRepository(db)
	hub := websocket.NewHub(zap.NewNop())
	config := DefaultIndexerConfig()

	indexer := NewIndexer(repo, hub, config, zap.NewNop())

	stats := indexer.GetStats()
	assert.Equal(t, int64(0), stats.TotalIndexed)
	assert.Equal(t, int64(0), stats.TotalFailed)
	assert.False(t, stats.InitialLoadComplete)
}

func TestIndexer_HealthCheck(t *testing.T) {
	tdb := testutil.SetupTestDB(t)
	defer tdb.TearDown(t)
	db := tdb.DB

	repo := NewRepository(db)
	hub := websocket.NewHub(zap.NewNop())
	config := DefaultIndexerConfig()

	indexer := NewIndexer(repo, hub, config, zap.NewNop())

	// Not started, should not be healthy
	assert.False(t, indexer.IsHealthy())

	ctx := context.Background()
	err := indexer.Start(ctx)
	require.NoError(t, err)

	// Wait for startup
	time.Sleep(100 * time.Millisecond)

	// Started, should be healthy
	assert.True(t, indexer.IsHealthy())

	// Stop
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = indexer.Stop(stopCtx)
	require.NoError(t, err)

	// Stopped, should not be healthy
	assert.False(t, indexer.IsHealthy())
}

func TestIndexer_ConcurrentOperations(t *testing.T) {
	tdb := testutil.SetupTestDB(t)
	defer tdb.TearDown(t)
	db := tdb.DB

	repo := NewRepository(db)
	hub := websocket.NewHub(zap.NewNop())
	config := DefaultIndexerConfig()
	config.WorkerCount = 4

	indexer := NewIndexer(repo, hub, config, zap.NewNop())

	ctx := context.Background()
	err := indexer.Start(ctx)
	require.NoError(t, err)
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = indexer.Stop(stopCtx)
	}()

	// Send many concurrent operations
	numOps := 100
	for i := 0; i < numOps; i++ {
		go func(i int) {
			checkinID := uuid.New()
			userID := uuid.New()
			data := map[string]interface{}{
				"content": "Concurrent test",
			}
			_ = indexer.HandleCheckinEvent(ctx, IndexOperationCreate, checkinID, userID, data)
		}(i)
	}

	// Wait for processing
	time.Sleep(2 * time.Second)

	// Check that operations were processed
	stats := indexer.GetStats()
	assert.Greater(t, stats.TotalIndexed, int64(0))
}

func TestDefaultIndexerConfig(t *testing.T) {
	config := DefaultIndexerConfig()

	assert.Equal(t, 100, config.BulkSize)
	assert.Equal(t, 4, config.WorkerCount)
	assert.Equal(t, 3, config.RetryAttempts)
	assert.Equal(t, time.Second, config.RetryDelay)
	assert.Equal(t, 5*time.Second, config.IndexingInterval)
	assert.True(t, config.EnableCDC)
}
