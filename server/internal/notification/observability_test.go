package notification

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestNotificationTracer_Creation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tracer, err := NewNotificationTracer(logger)

	require.NoError(t, err)
	assert.NotNil(t, tracer)
	assert.NotNil(t, tracer.tracer)
	assert.NotNil(t, tracer.meter)
}

func TestNotificationTracer_RecordSent(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tracer, err := NewNotificationTracer(logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Should not panic
	tracer.RecordSent(ctx, "push", "fcm", "tenant-1", PriorityNormal)
	tracer.RecordSent(ctx, "email", "smtp", "tenant-2", PriorityHigh)
}

func TestNotificationTracer_RecordFailed(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tracer, err := NewNotificationTracer(logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Should not panic
	tracer.RecordFailed(ctx, "push", "fcm", "tenant-1", "network_error")
	tracer.RecordFailed(ctx, "email", "smtp", "tenant-1", "invalid_recipient")
}

func TestNotificationTracer_RecordRetry(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tracer, err := NewNotificationTracer(logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Should not panic
	tracer.RecordRetry(ctx, "push", "fcm", "tenant-1", 1)
	tracer.RecordRetry(ctx, "push", "fcm", "tenant-1", 2)
}

func TestNotificationTracer_RecordDuration(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tracer, err := NewNotificationTracer(logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Test various durations
	durations := []time.Duration{
		100 * time.Millisecond,
		1 * time.Second,
		5 * time.Second,
		10 * time.Second,
	}

	for _, duration := range durations {
		tracer.RecordDuration(ctx, "push", "fcm", "success", duration)
	}
}

func TestNotificationTracer_StartSpan(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tracer, err := NewNotificationTracer(logger)
	require.NoError(t, err)

	ctx := context.Background()

	spanCtx, span := tracer.StartSpan(ctx, "test-operation")
	assert.NotNil(t, spanCtx)
	assert.NotNil(t, span)

	span.End()
}

func TestNotificationTracer_RecordQueueDepth(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tracer, err := NewNotificationTracer(logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Record various queue depths
	tracer.RecordQueueDepth(ctx, PriorityHigh, 0, 100)
	tracer.RecordQueueDepth(ctx, PriorityNormal, 1, 50)
	tracer.RecordQueueDepth(ctx, PriorityLow, 2, 10)
}

func TestNotificationTracer_RecordActiveWorkers(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tracer, err := NewNotificationTracer(logger)
	require.NoError(t, err)

	// Should not panic
	tracer.RecordActiveWorkers(0, 5)
	tracer.RecordActiveWorkers(1, 3)
	tracer.RecordActiveWorkers(2, 7)
}

func TestNotificationTracer_RecordQuotaUsage(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tracer, err := NewNotificationTracer(logger)
	require.NoError(t, err)

	// Test various quota levels
	tests := []struct {
		tenant string
		period string
		usage  int
		limit  int
	}{
		{"tenant-1", "hourly", 50, 100},
		{"tenant-2", "hourly", 90, 100}, // High usage
		{"tenant-1", "daily", 500, 1000},
	}

	for _, tt := range tests {
		tracer.RecordQuotaUsage(tt.tenant, tt.period, tt.usage, tt.limit)
	}
}

func TestNotificationTracer_RecordDNDBlock(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tracer, err := NewNotificationTracer(logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Should not panic
	tracer.RecordDNDBlock(ctx, "push", "tenant-1")
	tracer.RecordDNDBlock(ctx, "email", "tenant-2")
}

func TestNotificationTracer_RecordDLQMessage(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tracer, err := NewNotificationTracer(logger)
	require.NoError(t, err)

	// Should not panic
	tracer.RecordDLQMessage("max_retries_exceeded", 5)
	tracer.RecordDLQMessage("invalid_format", 2)
}

func TestCorrelationID(t *testing.T) {
	ctx := context.Background()

	// Test adding correlation ID
	correlationID := "test-correlation-123"
	ctx = WithCorrelationID(ctx, correlationID)

	// Test retrieving correlation ID
	retrieved := GetCorrelationID(ctx)
	assert.Equal(t, correlationID, retrieved)
}

func TestCorrelationID_NotPresent(t *testing.T) {
	ctx := context.Background()

	// Test when no correlation ID is present
	retrieved := GetCorrelationID(ctx)
	assert.Empty(t, retrieved)
}

func TestLogWithCorrelation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	// Without correlation ID
	loggerWithoutID := LogWithCorrelation(logger, ctx)
	assert.NotNil(t, loggerWithoutID)

	// With correlation ID
	ctx = WithCorrelationID(ctx, "test-123")
	loggerWithID := LogWithCorrelation(logger, ctx)
	assert.NotNil(t, loggerWithID)
}

func TestGetMetrics(t *testing.T) {
	logger := zaptest.NewLogger(t)
	router := NewShardRouter(ShardingByUser, 10, logger)

	metrics := GetMetrics(router)
	assert.NotNil(t, metrics)
	assert.NotNil(t, metrics.QueueDepth)
	assert.NotNil(t, metrics.QuotaUsage)
}

func TestNotificationTracer_ConcurrentRecords(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tracer, err := NewNotificationTracer(logger)
	require.NoError(t, err)

	ctx := context.Background()
	done := make(chan bool)

	// Concurrent metric recording
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				tracer.RecordSent(ctx, "push", "fcm", "tenant-1", PriorityNormal)
				tracer.RecordFailed(ctx, "push", "fcm", "tenant-1", "error")
				tracer.RecordDuration(ctx, "push", "fcm", "success", time.Millisecond*100)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestNotificationTracer_MultipleSpans(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tracer, err := NewNotificationTracer(logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Create nested spans
	ctx1, span1 := tracer.StartSpan(ctx, "outer-operation")
	ctx2, span2 := tracer.StartSpan(ctx1, "inner-operation-1")
	ctx3, span3 := tracer.StartSpan(ctx1, "inner-operation-2")

	assert.NotNil(t, ctx1)
	assert.NotNil(t, ctx2)
	assert.NotNil(t, ctx3)

	span3.End()
	span2.End()
	span1.End()
}

func BenchmarkRecordSent(b *testing.B) {
	logger := zaptest.NewLogger(b)
	tracer, _ := NewNotificationTracer(logger)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tracer.RecordSent(ctx, "push", "fcm", "tenant-1", PriorityNormal)
	}
}

func BenchmarkRecordDuration(b *testing.B) {
	logger := zaptest.NewLogger(b)
	tracer, _ := NewNotificationTracer(logger)
	ctx := context.Background()
	duration := 100 * time.Millisecond

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tracer.RecordDuration(ctx, "push", "fcm", "success", duration)
	}
}

func BenchmarkStartSpan(b *testing.B) {
	logger := zaptest.NewLogger(b)
	tracer, _ := NewNotificationTracer(logger)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, span := tracer.StartSpan(ctx, "test-span")
		span.End()
	}
}
