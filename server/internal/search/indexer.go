package search

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dev-jelly/donelist/internal/websocket"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// IndexerConfig holds configuration for the indexer
type IndexerConfig struct {
	// BulkSize is the number of documents to index in a single batch
	BulkSize int
	// WorkerCount is the number of concurrent indexing workers
	WorkerCount int
	// RetryAttempts is the number of times to retry failed indexing operations
	RetryAttempts int
	// RetryDelay is the initial delay between retry attempts (exponential backoff)
	RetryDelay time.Duration
	// IndexingInterval is how often to check for pending index operations
	IndexingInterval time.Duration
	// EnableCDC enables change data capture for real-time sync
	EnableCDC bool
}

// DefaultIndexerConfig returns default configuration
func DefaultIndexerConfig() IndexerConfig {
	return IndexerConfig{
		BulkSize:         100,
		WorkerCount:      4,
		RetryAttempts:    3,
		RetryDelay:       time.Second,
		IndexingInterval: 5 * time.Second,
		EnableCDC:        true,
	}
}

// IndexOperation represents a single indexing operation
type IndexOperation struct {
	ID        uuid.UUID
	Type      IndexOperationType
	CheckinID uuid.UUID
	UserID    uuid.UUID
	Timestamp time.Time
	Retries   int
	Data      map[string]interface{}
}

// IndexOperationType defines the type of index operation
type IndexOperationType string

const (
	IndexOperationCreate IndexOperationType = "create"
	IndexOperationUpdate IndexOperationType = "update"
	IndexOperationDelete IndexOperationType = "delete"
)

// IndexerStats tracks indexing statistics
type IndexerStats struct {
	mu                  sync.RWMutex
	TotalIndexed        int64
	TotalFailed         int64
	TotalRetried        int64
	LastIndexedAt       time.Time
	LastFailedAt        time.Time
	PendingOperations   int
	ProcessingRate      float64 // operations per second
	AverageLatency      time.Duration
	LastCompletedBatch  time.Time
	InitialLoadComplete bool
}

// Indexer handles search index synchronization
type Indexer struct {
	repo          *Repository
	hub           *websocket.Hub
	config        IndexerConfig
	logger        *zap.Logger

	// Operation queue for pending index updates
	operationQueue chan *IndexOperation

	// Failed operations that need retry
	retryQueue chan *IndexOperation

	// Stats tracking
	stats *IndexerStats

	// Control channels
	stopChan chan struct{}
	doneChan chan struct{}

	// WaitGroup for graceful shutdown
	wg sync.WaitGroup

	// Deduplication map to prevent duplicate operations
	dedupeMap sync.Map

	// Running state
	mu      sync.RWMutex
	running bool
}

// NewIndexer creates a new search indexer
func NewIndexer(repo *Repository, hub *websocket.Hub, config IndexerConfig, logger *zap.Logger) *Indexer {
	return &Indexer{
		repo:           repo,
		hub:            hub,
		config:         config,
		logger:         logger,
		operationQueue: make(chan *IndexOperation, 1000),
		retryQueue:     make(chan *IndexOperation, 500),
		stats:          &IndexerStats{},
		stopChan:       make(chan struct{}),
		doneChan:       make(chan struct{}),
	}
}

// Start begins the indexing pipeline
func (idx *Indexer) Start(ctx context.Context) error {
	idx.mu.Lock()
	if idx.running {
		idx.mu.Unlock()
		return fmt.Errorf("indexer already running")
	}
	idx.running = true
	idx.mu.Unlock()

	idx.logger.Info("Starting search indexer",
		zap.Int("workers", idx.config.WorkerCount),
		zap.Int("bulk_size", idx.config.BulkSize),
	)

	// Perform initial bulk load
	if err := idx.initialBulkLoad(ctx); err != nil {
		idx.logger.Error("Initial bulk load failed", zap.Error(err))
		return fmt.Errorf("initial bulk load failed: %w", err)
	}

	// Start worker goroutines
	for i := 0; i < idx.config.WorkerCount; i++ {
		idx.wg.Add(1)
		go idx.indexWorker(i)
	}

	// Start retry handler
	idx.wg.Add(1)
	go idx.retryHandler()

	// Start CDC listener if enabled
	if idx.config.EnableCDC {
		idx.wg.Add(1)
		go idx.cdcListener()
	}

	// Start stats reporter
	idx.wg.Add(1)
	go idx.statsReporter()

	return nil
}

// Stop gracefully shuts down the indexer
func (idx *Indexer) Stop(ctx context.Context) error {
	idx.mu.Lock()
	if !idx.running {
		idx.mu.Unlock()
		return nil
	}
	idx.mu.Unlock()

	idx.logger.Info("Stopping search indexer")
	close(idx.stopChan)

	// Wait for all workers to finish with timeout
	done := make(chan struct{})
	go func() {
		idx.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		idx.logger.Info("Search indexer stopped gracefully")
	case <-ctx.Done():
		idx.logger.Warn("Search indexer shutdown timed out")
		return ctx.Err()
	}

	idx.mu.Lock()
	idx.running = false
	idx.mu.Unlock()

	return nil
}

// initialBulkLoad performs the initial indexing of all existing data
func (idx *Indexer) initialBulkLoad(ctx context.Context) error {
	idx.logger.Info("Starting initial bulk load")
	startTime := time.Now()

	// Get total count of documents to index
	var totalCount int64
	if err := idx.repo.db.GetContext(ctx, &totalCount,
		"SELECT COUNT(*) FROM checkins WHERE deleted_at IS NULL"); err != nil {
		return fmt.Errorf("failed to get total count: %w", err)
	}

	idx.logger.Info("Bulk indexing checkins", zap.Int64("total", totalCount))

	// Process in batches
	offset := 0
	totalIndexed := 0

	for {
		// Check if we should stop
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-idx.stopChan:
			return fmt.Errorf("indexer stopped during bulk load")
		default:
		}

		// Fetch batch
		query := `
			SELECT id, user_id, category_id, content, checkin_time,
			       duration_minutes, is_edited, edit_count, created_at, updated_at
			FROM checkins
			WHERE deleted_at IS NULL
			ORDER BY created_at
			LIMIT $1 OFFSET $2
		`

		var checkins []struct {
			ID              uuid.UUID  `db:"id"`
			UserID          uuid.UUID  `db:"user_id"`
			CategoryID      *uuid.UUID `db:"category_id"`
			Content         string     `db:"content"`
			CheckinTime     time.Time  `db:"checkin_time"`
			DurationMinutes int        `db:"duration_minutes"`
			IsEdited        bool       `db:"is_edited"`
			EditCount       int        `db:"edit_count"`
			CreatedAt       time.Time  `db:"created_at"`
			UpdatedAt       time.Time  `db:"updated_at"`
		}

		if err := idx.repo.db.SelectContext(ctx, &checkins, query, idx.config.BulkSize, offset); err != nil {
			return fmt.Errorf("failed to fetch checkins batch: %w", err)
		}

		if len(checkins) == 0 {
			break
		}

		// Index batch
		for _, checkin := range checkins {
			data := map[string]interface{}{
				"id":               checkin.ID,
				"user_id":          checkin.UserID,
				"category_id":      checkin.CategoryID,
				"content":          checkin.Content,
				"checkin_time":     checkin.CheckinTime,
				"duration_minutes": checkin.DurationMinutes,
				"is_edited":        checkin.IsEdited,
				"edit_count":       checkin.EditCount,
				"created_at":       checkin.CreatedAt,
				"updated_at":       checkin.UpdatedAt,
			}

			op := &IndexOperation{
				ID:        uuid.New(),
				Type:      IndexOperationCreate,
				CheckinID: checkin.ID,
				UserID:    checkin.UserID,
				Timestamp: time.Now(),
				Data:      data,
			}

			// Send to operation queue (non-blocking)
			select {
			case idx.operationQueue <- op:
				totalIndexed++
			case <-time.After(5 * time.Second):
				idx.logger.Warn("Operation queue full, backing off")
				time.Sleep(time.Second)
			}
		}

		offset += len(checkins)

		// Log progress
		if offset%1000 == 0 {
			idx.logger.Info("Bulk load progress",
				zap.Int("processed", offset),
				zap.Int64("total", totalCount),
				zap.Float64("percent", float64(offset)/float64(totalCount)*100),
			)
		}
	}

	duration := time.Since(startTime)
	idx.logger.Info("Initial bulk load completed",
		zap.Int("total_indexed", totalIndexed),
		zap.Duration("duration", duration),
		zap.Float64("rate_per_sec", float64(totalIndexed)/duration.Seconds()),
	)

	idx.stats.mu.Lock()
	idx.stats.InitialLoadComplete = true
	idx.stats.LastCompletedBatch = time.Now()
	idx.stats.mu.Unlock()

	return nil
}

// indexWorker processes index operations from the queue
func (idx *Indexer) indexWorker(workerID int) {
	defer idx.wg.Done()

	idx.logger.Info("Index worker started", zap.Int("worker_id", workerID))

	batch := make([]*IndexOperation, 0, idx.config.BulkSize)
	ticker := time.NewTicker(idx.config.IndexingInterval)
	defer ticker.Stop()

	processBatch := func() {
		if len(batch) == 0 {
			return
		}

		startTime := time.Now()
		if err := idx.processBatch(batch); err != nil {
			idx.logger.Error("Failed to process batch",
				zap.Int("worker_id", workerID),
				zap.Int("batch_size", len(batch)),
				zap.Error(err),
			)

			// Send failed operations to retry queue
			for _, op := range batch {
				op.Retries++
				if op.Retries <= idx.config.RetryAttempts {
					select {
					case idx.retryQueue <- op:
					default:
						idx.logger.Error("Retry queue full, dropping operation",
							zap.String("checkin_id", op.CheckinID.String()),
						)
						idx.stats.mu.Lock()
						idx.stats.TotalFailed++
						idx.stats.mu.Unlock()
					}
				} else {
					idx.logger.Error("Operation exceeded retry attempts",
						zap.String("checkin_id", op.CheckinID.String()),
						zap.Int("retries", op.Retries),
					)
					idx.stats.mu.Lock()
					idx.stats.TotalFailed++
					idx.stats.mu.Unlock()
				}
			}
		} else {
			idx.stats.mu.Lock()
			idx.stats.TotalIndexed += int64(len(batch))
			idx.stats.LastIndexedAt = time.Now()
			idx.stats.AverageLatency = time.Since(startTime) / time.Duration(len(batch))
			idx.stats.mu.Unlock()
		}

		// Clear batch
		batch = batch[:0]
	}

	for {
		select {
		case <-idx.stopChan:
			// Process remaining batch before stopping
			processBatch()
			idx.logger.Info("Index worker stopped", zap.Int("worker_id", workerID))
			return

		case op := <-idx.operationQueue:
			// Check for duplicate operations
			key := fmt.Sprintf("%s:%s", op.Type, op.CheckinID.String())
			if _, exists := idx.dedupeMap.LoadOrStore(key, time.Now()); exists {
				continue
			}

			// Add to batch
			batch = append(batch, op)

			// Process if batch is full
			if len(batch) >= idx.config.BulkSize {
				processBatch()
			}

		case <-ticker.C:
			// Process batch periodically even if not full
			processBatch()
		}
	}
}

// processBatch processes a batch of index operations
func (idx *Indexer) processBatch(operations []*IndexOperation) error {
	if len(operations) == 0 {
		return nil
	}

	// Group operations by type for efficient processing
	creates := make([]*IndexOperation, 0)
	updates := make([]*IndexOperation, 0)
	deletes := make([]*IndexOperation, 0)

	for _, op := range operations {
		switch op.Type {
		case IndexOperationCreate:
			creates = append(creates, op)
		case IndexOperationUpdate:
			updates = append(updates, op)
		case IndexOperationDelete:
			deletes = append(deletes, op)
		}
	}

	// Process each type
	// Note: In this implementation, we're using PostgreSQL's built-in full-text search
	// The search_vector tsvector column is automatically updated via triggers
	// So we don't need to do anything here - the database handles it

	// For a dedicated search engine (Elasticsearch, Algolia, etc.), you would:
	// 1. Bulk index creates/updates
	// 2. Bulk delete

	idx.logger.Debug("Batch processed",
		zap.Int("creates", len(creates)),
		zap.Int("updates", len(updates)),
		zap.Int("deletes", len(deletes)),
	)

	// Clear deduplication map for processed operations
	for _, op := range operations {
		key := fmt.Sprintf("%s:%s", op.Type, op.CheckinID.String())
		idx.dedupeMap.Delete(key)
	}

	return nil
}

// retryHandler handles retrying failed operations with exponential backoff
func (idx *Indexer) retryHandler() {
	defer idx.wg.Done()

	idx.logger.Info("Retry handler started")

	for {
		select {
		case <-idx.stopChan:
			idx.logger.Info("Retry handler stopped")
			return

		case op := <-idx.retryQueue:
			// Calculate backoff delay with exponential backoff
			delay := idx.config.RetryDelay * time.Duration(1<<uint(op.Retries-1))
			if delay > 30*time.Second {
				delay = 30 * time.Second
			}

			idx.logger.Info("Retrying operation",
				zap.String("checkin_id", op.CheckinID.String()),
				zap.Int("retry_attempt", op.Retries),
				zap.Duration("delay", delay),
			)

			time.Sleep(delay)

			// Re-queue the operation
			select {
			case idx.operationQueue <- op:
				idx.stats.mu.Lock()
				idx.stats.TotalRetried++
				idx.stats.mu.Unlock()
			case <-idx.stopChan:
				return
			}
		}
	}
}

// cdcListener listens to websocket events for change data capture
func (idx *Indexer) cdcListener() {
	defer idx.wg.Done()

	idx.logger.Info("CDC listener started")

	// Create a channel to receive websocket messages
	// Note: In a real implementation, you would subscribe to the websocket hub
	// or use a dedicated message queue (Redis Pub/Sub, Kafka, etc.)

	// For now, we'll document the pattern - the actual implementation would
	// depend on how the websocket hub broadcasts messages

	// Example pattern:
	// messageChan := idx.hub.Subscribe("checkin.*")
	// for {
	//     select {
	//     case <-idx.stopChan:
	//         return
	//     case msg := <-messageChan:
	//         idx.handleWebSocketMessage(msg)
	//     }
	// }

	// Since the websocket hub doesn't expose a subscription mechanism,
	// the CDC is effectively handled by the checkin service broadcasting
	// events, and we rely on the database triggers to update search_vector

	<-idx.stopChan
	idx.logger.Info("CDC listener stopped")
}

// HandleCheckinEvent processes a checkin event for indexing
// This should be called by the checkin service after CRUD operations
func (idx *Indexer) HandleCheckinEvent(ctx context.Context, eventType IndexOperationType, checkinID, userID uuid.UUID, data map[string]interface{}) error {
	op := &IndexOperation{
		ID:        uuid.New(),
		Type:      eventType,
		CheckinID: checkinID,
		UserID:    userID,
		Timestamp: time.Now(),
		Data:      data,
	}

	select {
	case idx.operationQueue <- op:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
		return fmt.Errorf("operation queue full")
	}
}

// statsReporter periodically logs indexer statistics
func (idx *Indexer) statsReporter() {
	defer idx.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-idx.stopChan:
			return
		case <-ticker.C:
			idx.stats.mu.RLock()
			stats := *idx.stats
			idx.stats.mu.RUnlock()

			idx.stats.mu.Lock()
			idx.stats.PendingOperations = len(idx.operationQueue)
			idx.stats.mu.Unlock()

			idx.logger.Info("Indexer stats",
				zap.Int64("total_indexed", stats.TotalIndexed),
				zap.Int64("total_failed", stats.TotalFailed),
				zap.Int64("total_retried", stats.TotalRetried),
				zap.Int("pending_operations", len(idx.operationQueue)),
				zap.Duration("avg_latency", stats.AverageLatency),
				zap.Bool("initial_load_complete", stats.InitialLoadComplete),
			)
		}
	}
}

// GetStats returns current indexer statistics
func (idx *Indexer) GetStats() IndexerStats {
	idx.stats.mu.RLock()
	defer idx.stats.mu.RUnlock()

	stats := *idx.stats
	stats.PendingOperations = len(idx.operationQueue)
	return stats
}

// IsHealthy returns whether the indexer is healthy
func (idx *Indexer) IsHealthy() bool {
	idx.mu.RLock()
	running := idx.running
	idx.mu.RUnlock()

	if !running {
		return false
	}

	// Check if operation queue is too backed up
	if len(idx.operationQueue) > cap(idx.operationQueue)*9/10 {
		return false
	}

	// Check if we've indexed recently (within last 5 minutes)
	idx.stats.mu.RLock()
	lastIndexed := idx.stats.LastIndexedAt
	idx.stats.mu.RUnlock()

	if !lastIndexed.IsZero() && time.Since(lastIndexed) > 5*time.Minute {
		return false
	}

	return true
}
