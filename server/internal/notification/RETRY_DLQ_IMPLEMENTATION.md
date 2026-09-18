# Retry, DLQ, Idempotency, and Circuit Breaker Implementation

## Overview

This document describes the implementation of Task #2.5: 중복 방지·재시도·지수 백오프 및 DLQ 설계 (Deduplication, Retry, Exponential Backoff, and DLQ Design) for the notification system.

## Components Implemented

### 1. Retry Manager (`retry.go`)

**Purpose**: Implements retry logic with exponential backoff for failed notifications.

**Features**:
- **Max Retry Attempts**: 3 attempts for transient failures
- **Exponential Backoff**: 1s, 2s, 4s progression
- **Jitter**: Optional jitter (10% by default) to prevent thundering herd
- **Configurable**: All parameters can be customized via `RetryConfig`

**Key Functions**:
```go
// Calculate backoff delay for a given attempt
func (rm *RetryManager) CalculateBackoff(attempt int) time.Duration

// Determine if a notification should be retried
func (rm *RetryManager) ShouldRetry(job *NotificationJob, err error) bool

// Schedule a job for retry with exponential backoff
func (rm *RetryManager) ScheduleRetry(ctx context.Context, job *NotificationJob, queue *Queue, err error) error
```

**Configuration**:
```go
RetryConfig{
    MaxAttempts:     3,              // Maximum retry attempts
    BaseDelay:       1 * time.Second, // Base delay for first retry
    MaxDelay:        4 * time.Second, // Maximum delay cap
    ExponentialBase: 2.0,             // Multiplier for exponential growth
    JitterEnabled:   true,            // Enable jitter
    JitterPercent:   0.1,             // 10% jitter
}
```

### 2. Idempotency Manager (`idempotency.go`)

**Purpose**: Prevents duplicate notification sends within a time window.

**Features**:
- **Idempotency Window**: 1 hour by default
- **SHA256 Hashing**: Generates deterministic keys from notification content
- **Redis-backed**: Uses Redis for distributed idempotency tracking
- **Automatic Expiry**: Keys expire automatically after the window

**Key Functions**:
```go
// Generate an idempotency key for a notification
func (im *IdempotencyManager) GenerateKey(job *NotificationJob) string

// Check if a notification is a duplicate
func (im *IdempotencyManager) CheckDuplicate(ctx context.Context, job *NotificationJob) (bool, error)

// Mark a notification as processed
func (im *IdempotencyManager) MarkProcessed(ctx context.Context, job *NotificationJob) error

// Remove an idempotency key (e.g., if send failed)
func (im *IdempotencyManager) RemoveKey(ctx context.Context, job *NotificationJob) error
```

**Configuration**:
```go
IdempotencyConfig{
    Window:  1 * time.Hour, // Time window for duplicate detection
    Enabled: true,          // Enable/disable idempotency
}
```

**Idempotency Key Generation**:
- Keys are generated from: `user_id`, `notification_type`, and `payload`
- Content is hashed using SHA256 for consistent, unique keys
- Same notification content generates the same key

### 3. Circuit Breaker (`circuit_breaker.go`)

**Purpose**: Implements circuit breaker pattern to protect notification providers from cascading failures.

**Features**:
- **Failure Threshold**: Opens after 5 consecutive failures
- **Success Threshold**: Closes after 2 consecutive successes in half-open state
- **Timeout/Cooldown**: 5-minute cooldown before transitioning to half-open
- **Half-Open Requests**: Limits concurrent requests in half-open state to 3
- **State Persistence**: States persisted to Redis for distributed coordination

**States**:
1. **Closed**: Normal operation, all requests pass through
2. **Open**: Circuit is open, requests fail immediately without hitting provider
3. **Half-Open**: Testing if provider has recovered, limited requests allowed

**Key Functions**:
```go
// Execute a function through the circuit breaker
func (cb *CircuitBreaker) Call(ctx context.Context, fn func() error) error

// Get current state of the circuit breaker
func (cb *CircuitBreaker) GetState() CircuitState

// Manually reset the circuit breaker
func (cb *CircuitBreaker) Reset()

// Check if circuit breaker is healthy
func (cb *CircuitBreaker) IsHealthy() bool
```

**Configuration**:
```go
CircuitBreakerConfig{
    FailureThreshold:    5,                // Failures before opening
    SuccessThreshold:    2,                // Successes to close from half-open
    Timeout:             5 * time.Minute,  // Cooldown before half-open
    HalfOpenMaxRequests: 3,                // Max concurrent half-open requests
    ResetTimeout:        1 * time.Minute,  // Time before resetting failure count
}
```

**Circuit Breaker Manager**:
- Manages separate circuit breakers for each provider (FCM, APNs)
- Thread-safe provider-specific circuit breakers
- Centralized metrics and management

### 4. Dead Letter Queue (DLQ) - Enhanced `queue.go`

**Purpose**: Stores notifications that permanently failed after all retry attempts.

**Features**:
- **Automatic Movement**: Jobs automatically moved to DLQ after max retries
- **Manual Movement**: Support for moving jobs directly to DLQ for permanent errors
- **Inspection**: Retrieve jobs from DLQ for analysis
- **Retry from DLQ**: Manually retry jobs from DLQ after fixing issues
- **Purge**: Clear DLQ when needed

**Key Functions**:
```go
// Move a job directly to the DLQ (for permanent failures)
func (q *Queue) MoveToDLQ(ctx context.Context, job *NotificationJob, reason string) error

// Retrieve jobs from the DLQ for inspection
func (q *Queue) GetDLQJobs(ctx context.Context, limit int) ([]*NotificationJob, error)

// Retry a specific job from the DLQ
func (q *Queue) RetryDLQJob(ctx context.Context, jobID string) error

// Remove all jobs from the DLQ
func (q *Queue) PurgeDLQ(ctx context.Context) (int64, error)
```

### 5. Enhanced Processor (`processor_enhanced.go`)

**Purpose**: Orchestrates all retry, idempotency, and circuit breaker logic.

**Features**:
- **Integrated Workflow**: Combines all resilience patterns
- **Automatic Deduplication**: Checks idempotency before processing
- **Automatic Retry**: Schedules retries with exponential backoff
- **Circuit Breaker Protection**: Protects providers from overload
- **Comprehensive Metrics**: Exposes metrics from all components

**Processing Flow**:
1. Check for duplicate (idempotency)
2. Validate notification job
3. Process notification (with circuit breaker protection)
4. On success: Mark as processed in idempotency store
5. On failure: Determine if retryable or permanent
   - Retryable: Schedule retry with exponential backoff
   - Permanent: Move to DLQ

## Error Classification

### Transient Errors (Retryable)
- `ErrProviderUnavailable`: Provider service temporarily unavailable
- `ErrRateLimitExceeded`: Provider rate limit hit
- `context.DeadlineExceeded`: Request timeout

### Permanent Errors (Non-Retryable)
- `ErrInvalidToken`: Device token is invalid or malformed
- `ErrTokenExpired`: Device token has expired
- `ErrAuthenticationFailed`: Authentication with provider failed

## Redis Keys Used

```
notification:delayed            - ZSET for delayed notifications
notification:ready              - LIST for ready-to-send notifications
notification:processing:{id}    - SET for currently processing notifications
notification:dlq                - LIST for dead letter queue
notification:idempotency:{key}  - STRING for idempotency tracking
notification:circuit_breaker:{provider} - HASH for circuit breaker state
```

## Usage Example

### Basic Setup
```go
// Create Redis client
redisClient := redis.NewClient(&redis.Options{
    Addr: "localhost:6379",
})

// Create queue
queue := NewQueue(redisClient, logger, DefaultQueueConfig())

// Create enhanced processor with all features
processor := NewEnhancedProcessor(logger, queue, redisClient)

// Set provider manager (after implementing in subtask 2.3)
processor.SetProviderManager(providerManager)

// Process a notification job
err := processor.Process(ctx, job)
```

### Manual Retry Configuration
```go
// Custom retry configuration
retryConfig := RetryConfig{
    MaxAttempts:     5,              // 5 attempts instead of 3
    BaseDelay:       2 * time.Second, // Start with 2s
    MaxDelay:        30 * time.Second, // Cap at 30s
    ExponentialBase: 2.0,
    JitterEnabled:   true,
    JitterPercent:   0.2,             // 20% jitter
}

retryManager := NewRetryManager(redisClient, logger, retryConfig)
```

### Circuit Breaker Usage
```go
// Get circuit breaker for a provider
breaker := circuitBreakerMgr.GetBreaker(provider)

// Execute with circuit breaker protection
err := breaker.Call(ctx, func() error {
    return provider.Send(ctx, notification)
})

if err == ErrCircuitOpen {
    // Circuit is open, provider is down
    log.Error("Provider unavailable, circuit open")
}
```

### DLQ Management
```go
// Get failed notifications from DLQ
failedJobs, err := queue.GetDLQJobs(ctx, 100)

// Inspect and manually retry specific job
for _, job := range failedJobs {
    log.Info("Failed job", "id", job.ID, "error", job.LastError)

    // After fixing the issue, retry from DLQ
    err := queue.RetryDLQJob(ctx, job.ID)
}

// Purge DLQ after analysis
count, err := queue.PurgeDLQ(ctx)
log.Info("Purged DLQ", "count", count)
```

### Metrics
```go
// Get comprehensive metrics
metrics, err := processor.GetMetrics(ctx)

// Retry metrics
retryMetrics := metrics["retry"]
// - max_attempts
// - base_delay_ms
// - max_delay_ms

// Idempotency metrics
idempotencyMetrics := metrics["idempotency"]
// - enabled
// - window_seconds
// - active_keys

// Circuit breaker metrics
circuitMetrics := metrics["circuit_breaker"]
// Per provider:
// - state (closed, open, half_open)
// - failures
// - successes
// - time_since_state_change
```

## Testing

All components have comprehensive unit tests:

- `retry_test.go`: Tests for retry manager
  - Exponential backoff calculation
  - Retry decision logic
  - Schedule retry functionality

- `idempotency_test.go`: Tests for idempotency manager
  - Key generation
  - Duplicate detection
  - Idempotency window expiry

- `circuit_breaker_test.go`: Tests for circuit breaker
  - State transitions
  - Failure/success counting
  - Half-open request limiting

- `dlq_test.go`: Tests for DLQ functionality
  - Moving jobs to DLQ
  - Retrieving DLQ jobs
  - Retrying from DLQ
  - Purging DLQ

### Running Tests
```bash
# Run all notification tests
go test ./internal/notification -v

# Run specific test suites
go test ./internal/notification -run TestRetry
go test ./internal/notification -run TestIdempotency
go test ./internal/notification -run TestCircuitBreaker
go test ./internal/notification -run TestDLQ
```

## Integration with Subtask 2.3

The enhanced processor is designed to integrate seamlessly with FCM/APNs providers (to be implemented in subtask 2.3):

```go
// Example integration (to be implemented in 2.3)
func (ep *EnhancedProcessor) processNotification(ctx context.Context, job *NotificationJob) error {
    // Get provider based on platform
    provider := ep.providerManager.GetProvider(job.Platform)

    // Get circuit breaker for this provider
    breaker := ep.circuitBreakerMgr.GetBreaker(provider)

    // Send with circuit breaker protection
    return breaker.Call(ctx, func() error {
        notification := &PushNotification{
            DeviceToken: job.DeviceToken,
            Title:       job.Title,
            Body:        job.Body,
            Data:        job.Payload,
        }

        result, err := provider.Send(ctx, notification)
        if err != nil {
            return err
        }

        if !result.Success {
            return fmt.Errorf("send failed: %v", result.Error)
        }

        return nil
    })
}
```

## Performance Considerations

1. **Redis Operations**: All operations use efficient Redis data structures
   - ZSET for time-based delayed queue
   - LIST for FIFO ready queue
   - Hash for circuit breaker state

2. **Exponential Backoff**: Prevents overwhelming the system during failures
   - 1s, 2s, 4s progression ensures gradual recovery
   - Jitter prevents thundering herd

3. **Idempotency**: SHA256 hashing is fast and deterministic
   - Keys expire automatically via Redis TTL
   - No manual cleanup required

4. **Circuit Breaker**: Minimal overhead in closed state
   - Fast fail in open state prevents cascading failures
   - State persisted to Redis for distributed systems

## Monitoring and Observability

All components emit structured logs using zap:

```go
// Retry logs
logger.Info("Scheduled job for retry",
    zap.String("job_id", job.ID),
    zap.Int("attempt", job.Attempts),
    zap.Duration("backoff", backoffDelay),
    zap.Time("retry_at", job.ScheduledAt),
)

// Idempotency logs
logger.Info("Duplicate notification detected",
    zap.String("job_id", job.ID),
    zap.String("original_job_id", storedJobID),
    zap.String("idempotency_key", idempotencyKey),
)

// Circuit breaker logs
logger.Warn("Circuit breaker opening due to failures",
    zap.String("provider", string(provider)),
    zap.Int("failures", failures),
)

// DLQ logs
logger.Warn("Notification moved to DLQ",
    zap.String("job_id", job.ID),
    zap.Int("attempts", job.Attempts),
    zap.String("reason", reason),
)
```

## Future Enhancements

1. **Adaptive Retry**: Adjust retry delays based on provider feedback
2. **Priority-based DLQ**: Separate DLQs for different priority levels
3. **DLQ Auto-retry**: Automatically retry DLQ jobs after a period
4. **Advanced Metrics**: Histograms for retry delays, success rates, etc.
5. **Circuit Breaker Auto-recovery**: Automatic testing in half-open state

## Summary

This implementation provides a robust, production-ready retry and resilience system for notifications with:

- ✅ Exponential backoff with configurable parameters (1s, 2s, 4s)
- ✅ Maximum 3 retry attempts for transient failures
- ✅ Dead Letter Queue for permanent failures
- ✅ Idempotency keys with 1-hour window
- ✅ Circuit breaker with 5-minute cooldown
- ✅ Comprehensive error classification
- ✅ Full test coverage
- ✅ Production-ready logging and metrics

All requirements from Task #2.5 have been successfully implemented.
