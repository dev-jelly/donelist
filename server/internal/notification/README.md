# Notification System

Background push notification system for sending check-in reminders to users.

## Overview

This notification system implements a Redis-based delayed queue with cron-scheduled workers to send push notifications to users at configurable intervals (15min, 30min, 45min, 2hr).

## Architecture

### Components

1. **Queue** (`queue.go`) - Redis-based delayed queue implementation
   - Uses Redis ZSET for delayed notifications (scored by timestamp)
   - Uses Redis LIST for ready-to-process notifications
   - Implements retry logic with exponential backoff
   - Dead Letter Queue (DLQ) for failed notifications

2. **Scheduler** (`scheduler.go`) - Cron-based job scheduler
   - Moves delayed jobs to ready queue every minute
   - Manages worker pool lifecycle
   - Performs health checks
   - Recovers stuck jobs

3. **Worker Pool** (`worker.go`) - Concurrent notification processors
   - Configurable number of workers
   - Polls ready queue at regular intervals
   - Processes notifications through the Processor
   - Handles graceful shutdown

4. **Processor** (`processor.go`) - Notification sending logic
   - Validates and processes notification jobs
   - Will integrate with FCM/APNs in future subtasks
   - Checks user notification preferences and DnD settings

5. **Health Check** (`health.go`) - System health monitoring
   - Queue statistics
   - Scheduler status
   - Worker pool metrics

## Queue Keys

- `notification:delayed` - ZSET of scheduled notifications (score = unix timestamp)
- `notification:ready` - LIST of notifications ready to send
- `notification:processing:{job_id}` - Temporary key for jobs being processed
- `notification:dlq` - LIST of failed notifications (Dead Letter Queue)

## Configuration

### Queue Config
```go
type QueueConfig struct {
    BatchSize          int           // Number of jobs to process per batch
    ProcessTimeout     time.Duration // Timeout for processing a job
    VisibilityTimeout  time.Duration // How long a job is invisible after dequeue
    MaxRetries         int           // Maximum retry attempts
    RetryBackoffFactor float64       // Exponential backoff multiplier
}
```

### Scheduler Config
```go
type SchedulerConfig struct {
    ScanInterval        string        // Cron schedule for delayed queue scan
    WorkerCount         int           // Number of concurrent workers
    PollInterval        time.Duration // Worker polling interval
    HealthCheckEnabled  bool          // Enable health checks
    HealthCheckInterval time.Duration // Health check frequency
    MetricsEnabled      bool          // Enable metrics collection
}
```

## Usage

### Starting the Notification System

```go
import (
    "github.com/dev-jelly/donelist/internal/notification"
    "github.com/redis/go-redis/v9"
)

// Create Redis client
redisClient := redis.NewClient(&redis.Options{
    Addr: "localhost:6379",
})

// Create queue
queueConfig := notification.DefaultQueueConfig()
queue := notification.NewQueue(redisClient, logger, queueConfig)

// Create processor
processor := notification.NewProcessor(logger)

// Create scheduler
schedulerConfig := notification.DefaultSchedulerConfig()
scheduler := notification.NewScheduler(schedulerConfig, queue, processor, logger)

// Start the system
if err := scheduler.Start(ctx); err != nil {
    log.Fatal(err)
}

// Later, stop gracefully
defer scheduler.Stop()
```

### Enqueueing a Notification

```go
job := &notification.NotificationJob{
    UserID:      userID,
    Type:        "checkin_reminder",
    Payload: map[string]interface{}{
        "message": "Time to check in!",
        "interval": "15min",
    },
    ScheduledAt: time.Now().Add(15 * time.Minute),
}

if err := queue.Enqueue(ctx, job); err != nil {
    log.Error("Failed to enqueue notification", zap.Error(err))
}
```

## Job Types

- `checkin_reminder` - Reminder to check in based on last check-in time
- `daily_summary` - Daily summary of check-ins (future)
- `streak_milestone` - Streak achievement notifications (future)

## Retry Strategy

Failed notifications are retried with exponential backoff:
- Attempt 1: Retry after 2 minutes
- Attempt 2: Retry after 4 minutes
- Attempt 3: Move to Dead Letter Queue

Jobs can be configured with custom max attempts and backoff factors.

## Health Monitoring

The system provides health check endpoints that report:
- Queue statistics (delayed, ready, processing, DLQ counts)
- Scheduler status (running, worker count)
- Worker pool metrics (active workers)
- System warnings (high DLQ count, large backlogs)

## Testing

Unit tests use `miniredis` for in-memory Redis simulation:

```bash
go test ./internal/notification/...
```

## Future Enhancements (Upcoming Subtasks)

### Subtask 2.2
- Integration with checkin repository to query last check-in time
- Interval rule engine (15min, 30min, 45min, 2hr)

### Subtask 2.3
- FCM (Firebase Cloud Messaging) integration
- APNs (Apple Push Notification service) integration
- Provider abstraction and mocking

### Subtask 2.4
- Timezone handling (including DST)
- Do Not Disturb (DnD) rule enforcement
- User profile timezone detection

### Subtask 2.5
- Deduplication logic
- Advanced retry strategies
- Dead Letter Queue management UI

### Subtask 2.6
- Multi-tenant support
- Sharding strategy for high-volume scenarios

### Subtask 2.7
- Metrics collection (Prometheus)
- Structured logging
- Distributed tracing

### Subtask 2.8
- Batch processing optimization
- Performance tuning
- Load testing

### Subtask 2.9
- Comprehensive integration tests
- Provider mocking framework

### Subtask 2.10
- Feature flags for gradual rollout
- Drain mode for maintenance
- Circuit breakers

## Performance Characteristics

- **Throughput**: Depends on worker count and Redis performance
- **Latency**: Sub-second for job enqueueing, configurable worker poll interval
- **Scalability**: Horizontal scaling via multiple scheduler instances with Redis as shared queue
- **Reliability**: At-least-once delivery with retry and DLQ

## Dependencies

- `github.com/redis/go-redis/v9` - Redis client
- `github.com/robfig/cron/v3` - Cron scheduler
- `github.com/google/uuid` - UUID generation
- `go.uber.org/zap` - Structured logging

## License

Internal use only - Part of DoneList project
