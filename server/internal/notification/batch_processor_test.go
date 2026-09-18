package notification

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestBatchProcessor_Creation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tracer, _ := NewNotificationTracer(logger)
	processor := NewProcessor(logger)
	queue := &Queue{}

	bp := NewBatchProcessor(nil, queue, processor, logger, tracer)

	assert.NotNil(t, bp)
	assert.Equal(t, 100, bp.config.BatchSize)
	assert.Equal(t, 10, bp.config.MaxConcurrency)
}

func TestBatchProcessor_CustomConfig(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tracer, _ := NewNotificationTracer(logger)
	processor := NewProcessor(logger)
	queue := &Queue{}

	config := &BatchConfig{
		BatchSize:      50,
		MaxConcurrency: 20,
		FlushInterval:  10 * time.Second,
	}

	bp := NewBatchProcessor(config, queue, processor, logger, tracer)

	assert.Equal(t, 50, bp.config.BatchSize)
	assert.Equal(t, 20, bp.config.MaxConcurrency)
	assert.Equal(t, 10*time.Second, bp.config.FlushInterval)
}

func TestBatchProcessor_AddToBatch(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tracer, _ := NewNotificationTracer(logger)
	processor := NewProcessor(logger)
	queue := &Queue{}

	bp := NewBatchProcessor(nil, queue, processor, logger, tracer)
	ctx := context.Background()

	job := &NotificationJob{
		ID:     uuid.New().String(),
		UserID: uuid.New(),
		Type:   "push",
	}

	err := bp.AddToBatch(ctx, job)
	assert.NoError(t, err)

	stats := bp.GetStats()
	assert.Equal(t, 1, stats["current_batch_size"])
}

func TestBatchProcessor_AutoFlush(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tracer, _ := NewNotificationTracer(logger)
	processor := NewProcessor(logger)
	queue := &Queue{}

	config := &BatchConfig{
		BatchSize:      5,
		MaxConcurrency: 2,
		FlushInterval:  time.Hour, // Long interval to test size-based flush
	}

	bp := NewBatchProcessor(config, queue, processor, logger, tracer)
	ctx := context.Background()

	// Add jobs up to batch size
	for i := 0; i < 5; i++ {
		job := &NotificationJob{
			ID:     uuid.New().String(),
			UserID: uuid.New(),
			Type:   "push",
		}
		bp.AddToBatch(ctx, job)
	}

	// Give it a moment to process
	time.Sleep(100 * time.Millisecond)

	// Batch should be flushed
	stats := bp.GetStats()
	assert.Equal(t, 0, stats["current_batch_size"])
}

func TestBatchProcessor_StartStop(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tracer, _ := NewNotificationTracer(logger)
	processor := NewProcessor(logger)
	queue := &Queue{}

	bp := NewBatchProcessor(nil, queue, processor, logger, tracer)
	ctx := context.Background()

	err := bp.Start(ctx)
	assert.NoError(t, err)

	err = bp.Stop()
	assert.NoError(t, err)
}

func TestBatchProcessor_FlushOnStop(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tracer, _ := NewNotificationTracer(logger)
	processor := NewProcessor(logger)
	queue := &Queue{}

	bp := NewBatchProcessor(nil, queue, processor, logger, tracer)
	ctx := context.Background()

	// Add some jobs
	for i := 0; i < 3; i++ {
		job := &NotificationJob{
			ID:     uuid.New().String(),
			UserID: uuid.New(),
			Type:   "push",
		}
		bp.AddToBatch(ctx, job)
	}

	// Stop should flush remaining jobs
	err := bp.Stop()
	assert.NoError(t, err)
}

func TestBatchProcessor_ProcessBatchOptimized(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tracer, _ := NewNotificationTracer(logger)
	processor := NewProcessor(logger)
	queue := &Queue{}

	config := &BatchConfig{
		BatchSize:        10,
		MaxConcurrency:   5,
		EnablePipelining: true,
	}

	bp := NewBatchProcessor(config, queue, processor, logger, tracer)
	ctx := context.Background()

	// Create test jobs
	jobs := make([]*NotificationJob, 20)
	for i := 0; i < 20; i++ {
		jobs[i] = &NotificationJob{
			ID:     uuid.New().String(),
			UserID: uuid.New(),
			Type:   "push",
			Payload: map[string]interface{}{
				"title": "Test",
				"body":  "Test notification",
			},
		}
	}

	err := bp.ProcessBatchOptimized(ctx, jobs)
	assert.NoError(t, err)
}

func TestBatchProcessor_ProcessBatchSequential(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tracer, _ := NewNotificationTracer(logger)
	processor := NewProcessor(logger)
	queue := &Queue{}

	config := &BatchConfig{
		BatchSize:        10,
		MaxConcurrency:   5,
		EnablePipelining: false, // Disable pipelining
	}

	bp := NewBatchProcessor(config, queue, processor, logger, tracer)
	ctx := context.Background()

	jobs := make([]*NotificationJob, 10)
	for i := 0; i < 10; i++ {
		jobs[i] = &NotificationJob{
			ID:     uuid.New().String(),
			UserID: uuid.New(),
			Type:   "email",
		}
	}

	err := bp.processBatchSequential(ctx, jobs)
	assert.NoError(t, err)
}

func TestPerformanceOptimizer_OptimizeBatchSize(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultBatchConfig()
	optimizer := NewPerformanceOptimizer(logger, config)

	tests := []struct {
		name              string
		avgProcessingTime time.Duration
		expectedMin       int
		expectedMax       int
	}{
		{
			name:              "Fast processing",
			avgProcessingTime: 50 * time.Millisecond,
			expectedMin:       100,
			expectedMax:       500,
		},
		{
			name:              "Normal processing",
			avgProcessingTime: 500 * time.Millisecond,
			expectedMin:       50,
			expectedMax:       150,
		},
		{
			name:              "Slow processing",
			avgProcessingTime: 2 * time.Second,
			expectedMin:       10,
			expectedMax:       100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			batchSize := optimizer.OptimizeBatchSize(100, tt.avgProcessingTime)
			assert.GreaterOrEqual(t, batchSize, tt.expectedMin)
			assert.LessOrEqual(t, batchSize, tt.expectedMax)
		})
	}
}

func TestPerformanceOptimizer_OptimizeConcurrency(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultBatchConfig()
	optimizer := NewPerformanceOptimizer(logger, config)

	tests := []struct {
		name          string
		queueDepth    int
		expectedMin   int
		expectedMax   int
	}{
		{
			name:        "High queue depth",
			queueDepth:  2000,
			expectedMin: 15,
			expectedMax: 50,
		},
		{
			name:        "Normal queue depth",
			queueDepth:  500,
			expectedMin: 8,
			expectedMax: 15,
		},
		{
			name:        "Low queue depth",
			queueDepth:  50,
			expectedMin: 5,
			expectedMax: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			concurrency := optimizer.OptimizeConcurrency(tt.queueDepth, 5)
			assert.GreaterOrEqual(t, concurrency, tt.expectedMin)
			assert.LessOrEqual(t, concurrency, tt.expectedMax)
		})
	}
}

func TestRunBenchmark(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping benchmark in short mode")
	}

	logger := zaptest.NewLogger(t)
	tracer, _ := NewNotificationTracer(logger)
	processor := NewProcessor(logger)
	queue := &Queue{}

	bp := NewBatchProcessor(nil, queue, processor, logger, tracer)
	ctx := context.Background()

	result, err := RunBenchmark(ctx, bp, 100)
	require.NoError(t, err)

	assert.Equal(t, 100, result.TotalProcessed)
	assert.Greater(t, result.ThroughputPerSec, 0.0)
	assert.Greater(t, result.Duration, time.Duration(0))
}

func TestBatchProcessor_GetStats(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tracer, _ := NewNotificationTracer(logger)
	processor := NewProcessor(logger)
	queue := &Queue{}

	bp := NewBatchProcessor(nil, queue, processor, logger, tracer)

	stats := bp.GetStats()
	assert.NotNil(t, stats)
	assert.Contains(t, stats, "current_batch_size")
	assert.Contains(t, stats, "batch_config")
	assert.Contains(t, stats, "last_flush")
}

func TestBatchProcessor_ConcurrentAddToBatch(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tracer, _ := NewNotificationTracer(logger)
	processor := NewProcessor(logger)
	queue := &Queue{}

	config := &BatchConfig{
		BatchSize:      1000, // Large enough to not auto-flush
		MaxConcurrency: 10,
		FlushInterval:  time.Hour,
	}

	bp := NewBatchProcessor(config, queue, processor, logger, tracer)
	ctx := context.Background()

	done := make(chan bool)
	jobCount := 100

	// Add jobs concurrently
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 10; j++ {
				job := &NotificationJob{
					ID:     uuid.New().String(),
					UserID: uuid.New(),
					Type:   "push",
				}
				bp.AddToBatch(ctx, job)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	stats := bp.GetStats()
	assert.Equal(t, jobCount, stats["current_batch_size"])
}

func BenchmarkBatchProcessor_AddToBatch(b *testing.B) {
	logger := zaptest.NewLogger(b)
	tracer, _ := NewNotificationTracer(logger)
	processor := NewProcessor(logger)
	queue := &Queue{}

	bp := NewBatchProcessor(nil, queue, processor, logger, tracer)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		job := &NotificationJob{
			ID:     uuid.New().String(),
			UserID: uuid.New(),
			Type:   "push",
		}
		bp.AddToBatch(ctx, job)
	}
}

func BenchmarkBatchProcessor_ProcessBatch(b *testing.B) {
	logger := zaptest.NewLogger(b)
	tracer, _ := NewNotificationTracer(logger)
	processor := NewProcessor(logger)
	queue := &Queue{}

	bp := NewBatchProcessor(nil, queue, processor, logger, tracer)
	ctx := context.Background()

	jobs := make([]*NotificationJob, 100)
	for i := 0; i < 100; i++ {
		jobs[i] = &NotificationJob{
			ID:     uuid.New().String(),
			UserID: uuid.New(),
			Type:   "push",
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bp.ProcessBatchOptimized(ctx, jobs)
	}
}
