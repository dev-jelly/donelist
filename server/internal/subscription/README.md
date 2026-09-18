# Subscription Expiry Notifications and Payment Failure Handling

## Overview

This module provides a comprehensive system for handling subscription expiry notifications and payment failure retry logic (dunning) for the DoneList application.

## Components

### 1. Notification Service (`notification_service.go`)

Handles subscription expiry notifications with configurable timing.

#### Features
- **Pre-expiry Notifications**: Sent before subscription expires (default: 7, 3, 1 days before)
- **Post-expiry Notifications**: Sent after subscription expires (default: 1, 3, 7 days after)
- **Duplicate Prevention**: Tracks sent notifications to prevent spam
- **Priority Management**: Higher priority for imminent expirations
- **Customizable Messages**: Context-aware notification content

#### Configuration

```go
config := &NotificationConfig{
    PreExpiryDays:      []int{7, 3, 1},
    PostExpiryDays:     []int{1, 3, 7},
    EnableNotifications: true,
}
```

#### Usage

```go
// Initialize service
notificationService := NewNotificationService(db, repo, notificationQueue, logger)

// Process notifications
err := notificationService.ProcessExpiryNotifications(ctx, config)
```

### 2. Dunning Service (`dunning_service.go`)

Implements payment failure retry logic with configurable retry intervals and grace periods.

#### Features
- **Automatic Retry Scheduling**: Creates retry sequence on payment failure
- **Exponential Backoff**: Configurable retry intervals (default: 3, 7, 14 days)
- **Grace Period**: Additional time after last retry before cancellation (default: 7 days)
- **Payment Recovery**: Handles successful retries and cancels remaining attempts
- **Detailed Status Tracking**: Complete audit trail of all retry attempts
- **Automatic Expiration**: Optionally expires subscriptions after grace period

#### Configuration

```go
config := &DunningConfig{
    MaxRetries: 3,
    RetryIntervals: []time.Duration{
        3 * 24 * time.Hour,  // 3 days
        7 * 24 * time.Hour,  // 7 days
        14 * 24 * time.Hour, // 14 days
    },
    GracePeriodDays:      7,
    EnableNotifications:  true,
    AutoCancelAfterGrace: true,
}
```

#### Usage

```go
// Initialize service
dunningService := NewDunningService(db, repo, notificationQueue, paymentService, logger)

// Process payment failures
err := dunningService.ProcessPaymentFailures(ctx, config)

// Check dunning status
status, err := dunningService.GetDunningStatus(ctx, subscriptionID)
```

### 3. Subscription Job (`jobs/subscription_job.go`)

Integrates with the job scheduler to run periodic tasks.

#### Scheduled Jobs

| Job | Frequency | Purpose |
|-----|-----------|---------|
| Expiry Notifications | Daily at 9 AM | Check and send subscription expiry notifications |
| Dunning Process | Every 6 hours | Process payment failures and retry attempts |

#### Usage

```go
// Initialize job
subscriptionJob := NewSubscriptionJob(db, logger, notificationService, dunningService)

// Set in scheduler
scheduler.SetSubscriptionJob(subscriptionJob)
```

## Database Schema

### subscription_notifications Table

Tracks sent notifications to prevent duplicates.

```sql
CREATE TABLE subscription_notifications (
    id UUID PRIMARY KEY,
    subscription_id UUID NOT NULL REFERENCES subscriptions(id),
    notification_type VARCHAR(50) NOT NULL,
    days_before_expiry INT DEFAULT 0,
    days_after_expiry INT DEFAULT 0,
    sent_at TIMESTAMP NOT NULL,
    UNIQUE(subscription_id, notification_type, days_before_expiry, days_after_expiry)
);
```

### dunning_attempts Table

Tracks payment retry attempts.

```sql
CREATE TABLE dunning_attempts (
    id UUID PRIMARY KEY,
    subscription_id UUID NOT NULL REFERENCES subscriptions(id),
    payment_id UUID REFERENCES payments(id),
    attempt_number INT NOT NULL,
    status VARCHAR(50) NOT NULL,
    scheduled_for TIMESTAMP NOT NULL,
    attempted_at TIMESTAMP,
    succeeded_at TIMESTAMP,
    failed_at TIMESTAMP,
    failure_reason TEXT,
    next_retry_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    UNIQUE(subscription_id, attempt_number)
);
```

## Workflow Diagrams

### Payment Failure Dunning Workflow

```
Payment Failure Detected
    ↓
Create Dunning Sequence (3 attempts)
    ↓
Attempt 1 (3 days later)
    ↓ [Failed]
Send Notification
    ↓
Attempt 2 (7 days later)
    ↓ [Failed]
Send Notification
    ↓
Attempt 3 (14 days later)
    ↓ [Failed]
Send Notification
    ↓
Grace Period (7 days)
    ↓ [No payment]
Expire Subscription
    ↓
Send Final Notification
```

### Successful Recovery Workflow

```
Payment Failure
    ↓
Dunning Attempt Created
    ↓
Retry Payment
    ↓ [Success]
Cancel Remaining Attempts
    ↓
Update Subscription Status
    ↓
Send Success Notification
```

### Subscription Expiry Notification Timeline

```
Day -7: Pre-expiry notification (Low priority)
Day -3: Pre-expiry notification (Normal priority)
Day -1: Pre-expiry notification (High priority)
Day 0:  Subscription expires
Day +1: Post-expiry notification
Day +3: Post-expiry notification
Day +7: Post-expiry notification
```

## State Transitions

### Dunning Attempt States

```
pending → retrying → succeeded
                 ↓
                failed → [grace period] → subscription expired
```

### Subscription Status Flow

```
active → past_due → [dunning attempts] → active (recovered)
                                       ↓
                                    expired (grace period ended)
```

## Grace Period Policy

The grace period provides users additional time to update payment information after all retry attempts have failed.

**Default Policy:**
- 3 retry attempts over 24 days (3 + 7 + 14 days)
- 7-day grace period after last retry
- Total: 31 days from initial failure to expiration
- Notifications sent at each retry and final expiration

**Grace Period Behavior:**
1. All dunning attempts must be exhausted
2. Grace period timer starts from last failed attempt
3. Subscription remains in `past_due` status
4. User can still use service during grace period (configurable)
5. After grace period: subscription status → `expired`

## Notification Types

### Pre-Expiry Notifications

**Priority Levels:**
- 7 days: Low priority (reminder)
- 3 days: Normal priority (warning)
- 1 day: High priority (urgent)

**Content:**
- Days until expiration
- Current plan name
- Call to action (renew)
- Link to billing portal

### Payment Failure Notifications

**Information Included:**
- Attempt number (e.g., "1 of 3")
- Next retry date
- Update payment method link
- Grace period information

### Post-Expiry Notifications

**Content:**
- Days since expiration
- Lost features reminder
- Resubscription incentive
- Easy reactivation link

## Testing

### Unit Tests

```bash
# Run notification service tests
go test -v ./internal/subscription -run TestNotificationService

# Run dunning service tests
go test -v ./internal/subscription -run TestDunningService
```

### Integration Tests

```bash
# Run all subscription tests (requires PostgreSQL)
go test -v ./internal/subscription

# Skip integration tests
go test -short ./internal/subscription
```

### Test Coverage

```bash
go test -cover ./internal/subscription
```

## Monitoring

### Key Metrics to Track

1. **Notification Metrics**
   - Notifications sent per day
   - Notification delivery rate
   - Notification type distribution

2. **Dunning Metrics**
   - Payment retry success rate
   - Average recovery time
   - Grace period expiration rate
   - Revenue recovery amount

3. **Subscription Metrics**
   - Churn rate by reason
   - Recovery rate by attempt number
   - Time to recovery

### Health Checks

```go
// Check dunning status for a subscription
status, err := dunningService.GetDunningStatus(ctx, subscriptionID)

// Monitor job execution
jobStatuses, err := scheduler.GetJobStatus(ctx)
```

## Configuration Best Practices

### Notification Timing

**Aggressive (High Engagement):**
```go
PreExpiryDays:  []int{14, 7, 3, 1}
PostExpiryDays: []int{1, 2, 3, 7, 14}
```

**Balanced (Default):**
```go
PreExpiryDays:  []int{7, 3, 1}
PostExpiryDays: []int{1, 3, 7}
```

**Minimal (Low Touch):**
```go
PreExpiryDays:  []int{7, 1}
PostExpiryDays: []int{1, 7}
```

### Dunning Strategy

**Aggressive Recovery:**
```go
MaxRetries: 5
RetryIntervals: []time.Duration{1*day, 3*day, 5*day, 7*day, 14*day}
GracePeriodDays: 3
```

**Standard Recovery (Default):**
```go
MaxRetries: 3
RetryIntervals: []time.Duration{3*day, 7*day, 14*day}
GracePeriodDays: 7
```

**Customer-Friendly:**
```go
MaxRetries: 4
RetryIntervals: []time.Duration{7*day, 14*day, 21*day, 28*day}
GracePeriodDays: 14
```

## Error Handling

### Notification Failures

- Failed notifications are logged but don't block processing
- Notification queue handles retries internally
- Duplicate prevention continues to work even if notification fails

### Payment Retry Failures

- Each failure is logged with reason
- Subsequent retries continue on schedule
- Grace period starts only after all retries exhausted

## Security Considerations

1. **PII Protection**: Notifications don't include payment details
2. **Rate Limiting**: Duplicate prevention prevents spam
3. **Audit Trail**: Complete history in dunning_attempts table
4. **Data Retention**: Old notifications can be purged safely

## Performance Optimization

### Database Indexes

Optimized queries using indexes on:
- `subscription_notifications(subscription_id, notification_type)`
- `dunning_attempts(subscription_id, status, scheduled_for)`

### Batch Processing

- Notifications processed in batches
- Configurable batch sizes
- Prevents database overload

## Integration Points

### Required Services

1. **NotificationQueue**: For sending notifications
2. **PaymentRetryService**: For retrying payments
3. **SubscriptionRepository**: For data access
4. **Job Scheduler**: For periodic execution

### Webhook Integration

Payment provider webhooks should trigger:
- `payment.failed` → Create dunning sequence
- `payment.succeeded` → Cancel dunning attempts
- `subscription.expired` → Update status

## Migration Guide

### Running Migrations

```bash
# Apply migration
migrate -path migrations -database "postgres://localhost/donelist" up

# Rollback if needed
migrate -path migrations -database "postgres://localhost/donelist" down 1
```

### Data Migration

No data migration needed for new installations. For existing systems:

1. Existing subscriptions: No notifications until next expiry cycle
2. Existing past_due: Dunning sequence starts on next job run

## Troubleshooting

### Notifications Not Sending

1. Check notification queue configuration
2. Verify scheduler is running
3. Check `subscription_notifications` for duplicates
4. Review logs for errors

### Dunning Not Processing

1. Verify subscription status is `past_due`
2. Check dunning_attempts table for existing attempts
3. Ensure PaymentRetryService is configured
4. Review grace period settings

### Performance Issues

1. Check database query performance
2. Verify indexes are created
3. Consider batch size adjustment
4. Monitor job execution time

## Future Enhancements

1. **A/B Testing**: Test different notification strategies
2. **Personalization**: Custom messages based on user behavior
3. **Multi-Channel**: SMS, email, push notifications
4. **Machine Learning**: Predict optimal retry times
5. **Dynamic Pricing**: Offer discounts during grace period
6. **Self-Service**: User-initiated payment retries

## References

- [Stripe Dunning Best Practices](https://stripe.com/docs/billing/revenue-recovery)
- [PCI DSS Compliance](https://www.pcisecuritystandards.org/)
- [SaaS Metrics Guide](https://www.cobloom.com/blog/saas-metrics)
