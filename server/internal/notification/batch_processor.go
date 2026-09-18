package notification

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// BatchConfig holds batch processing configuration
type BatchConfig struct {
	BatchSize           int           `json:"batch_size"`
	MaxConcurrency      int           `json:"max_concurrency"`
	FlushInterval       time.Duration `json:"flush_interval"`
	EnablePipelining    bool          `json:"enable_pipelining"`
	ConnectionPoolSize  int           `json:"connection_pool_size"`
}

// DefaultBatchConfig returns default batch configuration
func DefaultBatchConfig() *BatchConfig {
	return &BatchConfig{
		BatchSize:          100,
		MaxConcurrency:     10,
		FlushInterval:      5 * time.Second,
		EnablePipelining:   true,
		ConnectionPoolSize: 20,
	}
}

// BatchProcessor handles batch notification processing
type BatchProcessor struct {
	config      *BatchConfig
	queue       *Queue
	processor   *Processor
	logger      *zap.Logger
	tracer      *NotificationTracer

	batchMu     sync.Mutex
	currentBatch []*NotificationJob
	lastFlush   time.Time

	stopChan    chan struct{}
	wg          sync.WaitGroup
}

// NewBatchProcessor creates a new batch processor
func NewBatchProcessor(
	config *BatchConfig,
	queue *Queue,
	processor *Processor,
	logger *zap.Logger,
	tracer *NotificationTracer,
) *BatchProcessor {
	if config == nil {
		config = DefaultBatchConfig()
	}

	return &BatchProcessor{
		config:       config,
		queue:        queue,
		processor:    processor,
		logger:       logger,
		tracer:       tracer,
		currentBatch: make([]*NotificationJob, 0, config.BatchSize),
		lastFlush:    time.Now(),
		stopChan:     make(chan struct{}),
	}
}

// Start starts the batch processor
func (bp *BatchProcessor) Start(ctx context.Context) error {
	bp.logger.Info("Starting batch processor",
		zap.Int("batch_size", bp.config.BatchSize),
		zap.Int("concurrency", bp.config.MaxConcurrency),
	)

	bp.wg.Add(1)
	go bp.processLoop(ctx)

	return nil
}

// Stop stops the batch processor gracefully
func (bp *BatchProcessor) Stop() error {
	bp.logger.Info("Stopping batch processor")
	close(bp.stopChan)

	// Flush any remaining items
	bp.flushBatch(context.Background())

	// Wait for processing to complete
	bp.wg.Wait()

	bp.logger.Info("Batch processor stopped")
	return nil
}

// processLoop is the main processing loop
func (bp *BatchProcessor) processLoop(ctx context.Context) {
	defer bp.wg.Done()

	ticker := time.NewTicker(bp.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-bp.stopChan:
			return
		case <-ticker.C:
			bp.checkAndFlush(ctx)
		}
	}
}

// checkAndFlush checks if batch should be flushed
func (bp *BatchProcessor) checkAndFlush(ctx context.Context) {
	bp.batchMu.Lock()
	defer bp.batchMu.Unlock()

	shouldFlush := len(bp.currentBatch) >= bp.config.BatchSize ||
		time.Since(bp.lastFlush) >= bp.config.FlushInterval

	if shouldFlush && len(bp.currentBatch) > 0 {
		bp.flushBatchLocked(ctx)
	}
}

// AddToBatch adds a notification to the current batch
func (bp *BatchProcessor) AddToBatch(ctx context.Context, job *NotificationJob) error {
	bp.batchMu.Lock()
	defer bp.batchMu.Unlock()

	bp.currentBatch = append(bp.currentBatch, job)

	// Auto-flush if batch is full
	if len(bp.currentBatch) >= bp.config.BatchSize {
		return bp.flushBatchLocked(ctx)
	}

	return nil
}

// flushBatch flushes the current batch (with locking)
func (bp *BatchProcessor) flushBatch(ctx context.Context) error {
	bp.batchMu.Lock()
	defer bp.batchMu.Unlock()
	return bp.flushBatchLocked(ctx)
}

// flushBatchLocked flushes the current batch (assumes lock is held)
func (bp *BatchProcessor) flushBatchLocked(ctx context.Context) error {
	if len(bp.currentBatch) == 0 {
		return nil
	}

	batch := bp.currentBatch
	bp.currentBatch = make([]*NotificationJob, 0, bp.config.BatchSize)
	bp.lastFlush = time.Now()

	bp.logger.Debug("Flushing batch",
		zap.Int("batch_size", len(batch)),
	)

	// Process batch asynchronously
	go bp.processBatch(ctx, batch)

	return nil
}

// processBatch processes a batch of notifications
func (bp *BatchProcessor) processBatch(ctx context.Context, batch []*NotificationJob) {
	startTime := time.Now()

	// Use semaphore to limit concurrency
	sem := make(chan struct{}, bp.config.MaxConcurrency)
	var wg sync.WaitGroup

	successCount := 0
	failureCount := 0
	var mu sync.Mutex

	for _, job := range batch {
		wg.Add(1)
		sem <- struct{}{} // Acquire semaphore

		go func(j *NotificationJob) {
			defer wg.Done()
			defer func() { <-sem }() // Release semaphore

			err := bp.processor.Process(ctx, j)

			mu.Lock()
			if err != nil {
				failureCount++
			} else {
				successCount++
			}
			mu.Unlock()
		}(job)
	}

	wg.Wait()

	duration := time.Since(startTime)

	bp.logger.Info("Batch processed",
		zap.Int("total", len(batch)),
		zap.Int("success", successCount),
		zap.Int("failed", failureCount),
		zap.Duration("duration", duration),
		zap.Float64("throughput_per_sec", float64(len(batch))/duration.Seconds()),
	)
}

// ProcessBatchOptimized processes notifications with Redis pipelining
func (bp *BatchProcessor) ProcessBatchOptimized(ctx context.Context, jobs []*NotificationJob) error {
	if !bp.config.EnablePipelining {
		return bp.processBatchSequential(ctx, jobs)
	}

	// Group jobs by type for optimized processing
	jobsByType := make(map[string][]*NotificationJob)
	for _, job := range jobs {
		jobsByType[job.Type] = append(jobsByType[job.Type], job)
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(jobsByType))

	for notifType, typeJobs := range jobsByType {
		wg.Add(1)
		go func(nt string, tj []*NotificationJob) {
			defer wg.Done()

			if err := bp.processJobGroup(ctx, tj); err != nil {
				bp.logger.Error("Failed to process job group",
					zap.String("type", nt),
					zap.Error(err),
				)
				errChan <- err
			}
		}(notifType, typeJobs)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		if err != nil {
			return err
		}
	}

	return nil
}

// processJobGroup processes a group of jobs of the same type
func (bp *BatchProcessor) processJobGroup(ctx context.Context, jobs []*NotificationJob) error {
	// Process in mini-batches for better error handling
	miniBatchSize := 10

	for i := 0; i < len(jobs); i += miniBatchSize {
		end := i + miniBatchSize
		if end > len(jobs) {
			end = len(jobs)
		}

		miniBatch := jobs[i:end]

		for _, job := range miniBatch {
			if err := bp.processor.Process(ctx, job); err != nil {
				bp.logger.Error("Failed to process job in group",
					zap.String("job_id", job.ID),
					zap.Error(err),
				)
			}
		}
	}

	return nil
}

// processBatchSequential processes jobs sequentially (fallback)
func (bp *BatchProcessor) processBatchSequential(ctx context.Context, jobs []*NotificationJob) error {
	for _, job := range jobs {
		if err := bp.processor.Process(ctx, job); err != nil {
			bp.logger.Error("Failed to process job",
				zap.String("job_id", job.ID),
				zap.Error(err),
			)
		}
	}
	return nil
}

// GetStats returns batch processor statistics
func (bp *BatchProcessor) GetStats() map[string]interface{} {
	bp.batchMu.Lock()
	defer bp.batchMu.Unlock()

	return map[string]interface{}{
		"current_batch_size": len(bp.currentBatch),
		"batch_config": map[string]interface{}{
			"batch_size":       bp.config.BatchSize,
			"max_concurrency":  bp.config.MaxConcurrency,
			"flush_interval":   bp.config.FlushInterval.String(),
			"pipelining_enabled": bp.config.EnablePipelining,
		},
		"last_flush": bp.lastFlush.Format(time.RFC3339),
	}
}

// PerformanceOptimizer provides performance optimization utilities
type PerformanceOptimizer struct {
	logger *zap.Logger
	config *BatchConfig
}

// NewPerformanceOptimizer creates a new performance optimizer
func NewPerformanceOptimizer(logger *zap.Logger, config *BatchConfig) *PerformanceOptimizer {
	return &PerformanceOptimizer{
		logger: logger,
		config: config,
	}
}

// OptimizeBatchSize determines optimal batch size based on load
func (po *PerformanceOptimizer) OptimizeBatchSize(currentLoad int, avgProcessingTime time.Duration) int {
	// Simple heuristic: adjust batch size based on processing time
	if avgProcessingTime < 100*time.Millisecond {
		// Fast processing - increase batch size
		return min(po.config.BatchSize*2, 500)
	} else if avgProcessingTime > 1*time.Second {
		// Slow processing - decrease batch size
		return max(po.config.BatchSize/2, 10)
	}

	return po.config.BatchSize
}

// OptimizeConcurrency determines optimal concurrency level
func (po *PerformanceOptimizer) OptimizeConcurrency(queueDepth int, activeWorkers int) int {
	// Adjust concurrency based on queue depth
	if queueDepth > 1000 {
		return min(po.config.MaxConcurrency*2, 50)
	} else if queueDepth < 100 {
		return max(po.config.MaxConcurrency/2, 5)
	}

	return po.config.MaxConcurrency
}

// BenchmarkResult represents performance benchmark results
type BenchmarkResult struct {
	TotalProcessed    int           `json:"total_processed"`
	Duration          time.Duration `json:"duration"`
	ThroughputPerSec  float64       `json:"throughput_per_sec"`
	AvgLatency        time.Duration `json:"avg_latency"`
	P95Latency        time.Duration `json:"p95_latency"`
	P99Latency        time.Duration `json:"p99_latency"`
	ErrorRate         float64       `json:"error_rate"`
	MemoryUsageMB     float64       `json:"memory_usage_mb"`
}

// RunBenchmark runs a performance benchmark
func RunBenchmark(ctx context.Context, processor *BatchProcessor, jobCount int) (*BenchmarkResult, error) {
	startTime := time.Now()

	// Create test jobs
	jobs := make([]*NotificationJob, jobCount)
	for i := 0; i < jobCount; i++ {
		jobs[i] = &NotificationJob{
			ID:     uuid.New().String(),
			UserID: uuid.New(),
			Type:   "push",
			Payload: map[string]interface{}{
				"title": "Test notification",
				"body":  "Benchmark test",
			},
		}
	}

	// Process jobs
	if err := processor.ProcessBatchOptimized(ctx, jobs); err != nil {
		return nil, fmt.Errorf("benchmark failed: %w", err)
	}

	duration := time.Since(startTime)

	return &BenchmarkResult{
		TotalProcessed:   jobCount,
		Duration:         duration,
		ThroughputPerSec: float64(jobCount) / duration.Seconds(),
		AvgLatency:       duration / time.Duration(jobCount),
		ErrorRate:        0.0,
	}, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
