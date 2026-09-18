# Webhook System

A robust, production-ready webhook delivery system with job queue management, retry logic, dead letter queue, and comprehensive monitoring.

## Features

- **Reliable Delivery**: Asynchronous webhook delivery with Asynq job queue
- **Retry Logic**: Exponential backoff retry strategy (1m, 2m, 4m, 8m, 16m, max 1h)
- **Security**: HMAC-SHA256 signature verification
- **Dead Letter Queue**: Failed deliveries after max retries are moved to DLQ
- **Monitoring**: Prometheus metrics for delivery success/failure rates
- **Event Logging**: Comprehensive audit trail of all webhook events
- **REST API**: Full CRUD operations for webhook management

## Architecture

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│   Event     │────▶│   Service    │────▶│   Asynq     │
│  Trigger    │     │              │     │   Queue     │
└─────────────┘     └──────────────┘     └─────────────┘
                            │                     │
                            ▼                     ▼
                    ┌──────────────┐     ┌─────────────┐
                    │  Repository  │     │   Worker    │
                    │              │     │             │
                    └──────────────┘     └─────────────┘
                            │                     │
                            ▼                     ▼
                    ┌──────────────┐     ┌─────────────┐
                    │  PostgreSQL  │     │  HTTP POST  │
                    │              │     │  Delivery   │
                    └──────────────┘     └─────────────┘
```

## Database Schema

### webhooks
Stores webhook configurations.

```sql
CREATE TABLE webhooks (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    url TEXT NOT NULL,
    secret VARCHAR(255) NOT NULL,
    events JSONB NOT NULL,
    active BOOLEAN DEFAULT true,
    headers JSONB,
    last_triggered_at TIMESTAMPTZ,
    total_deliveries INTEGER DEFAULT 0,
    failed_deliveries INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    deleted_at TIMESTAMPTZ
);
```

### webhook_deliveries
Tracks delivery attempts and status.

```sql
CREATE TABLE webhook_deliveries (
    id UUID PRIMARY KEY,
    webhook_id UUID NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    event_id VARCHAR(255) NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(50) NOT NULL,
    attempts INTEGER DEFAULT 0,
    status_code INTEGER,
    response TEXT,
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    delivered_at TIMESTAMPTZ,
    next_retry_at TIMESTAMPTZ
);
```

### webhook_dead_letter_queue
Stores permanently failed deliveries.

```sql
CREATE TABLE webhook_dead_letter_queue (
    id UUID PRIMARY KEY,
    webhook_id UUID NOT NULL,
    webhook_url TEXT NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    event_id VARCHAR(255) NOT NULL,
    payload JSONB NOT NULL,
    total_attempts INTEGER NOT NULL,
    last_error TEXT,
    last_status_code INTEGER,
    original_delivery_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    failed_at TIMESTAMPTZ NOT NULL,
    metadata JSONB
);
```

### webhook_event_logs
Audit trail of all webhook events.

```sql
CREATE TABLE webhook_event_logs (
    id BIGSERIAL PRIMARY KEY,
    webhook_id UUID NOT NULL,
    delivery_id UUID NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    status VARCHAR(50) NOT NULL,
    attempt_number INTEGER NOT NULL,
    duration_ms INTEGER,
    status_code INTEGER,
    error TEXT,
    logged_at TIMESTAMPTZ NOT NULL
);
```

## Available Events

| Event Type | Description |
|-----------|-------------|
| `checkin.created` | Triggered when a new check-in is created |
| `checkin.updated` | Triggered when a check-in is updated |
| `checkin.deleted` | Triggered when a check-in is deleted |
| `category.created` | Triggered when a new category is created |
| `category.updated` | Triggered when a category is updated |
| `category.deleted` | Triggered when a category is deleted |
| `user.updated` | Triggered when user profile is updated |
| `user.upgraded` | Triggered when user upgrades subscription |
| `subscription.changed` | Triggered when subscription status changes |

## API Endpoints

### Create Webhook
```http
POST /api/v1/webhooks
Content-Type: application/json
Authorization: Bearer <token>

{
  "name": "My Webhook",
  "url": "https://example.com/webhook",
  "events": ["checkin.created", "checkin.updated"],
  "active": true,
  "headers": {
    "X-Custom-Header": "value"
  }
}
```

### List Webhooks
```http
GET /api/v1/webhooks
Authorization: Bearer <token>
```

### Get Webhook
```http
GET /api/v1/webhooks/:id
Authorization: Bearer <token>
```

### Update Webhook
```http
PUT /api/v1/webhooks/:id
Content-Type: application/json
Authorization: Bearer <token>

{
  "name": "Updated Webhook",
  "active": false
}
```

### Delete Webhook
```http
DELETE /api/v1/webhooks/:id
Authorization: Bearer <token>
```

### Test Webhook
```http
POST /api/v1/webhooks/:id/test
Authorization: Bearer <token>
```

### Get Webhook Statistics
```http
GET /api/v1/webhooks/:id/stats?since=2024-01-01T00:00:00Z
Authorization: Bearer <token>
```

### Get Delivery History
```http
GET /api/v1/webhooks/:id/deliveries?limit=50
Authorization: Bearer <token>
```

### Resend Failed Delivery
```http
POST /api/v1/webhooks/deliveries/:delivery_id/resend
Authorization: Bearer <token>
```

### Get Dead Letter Queue
```http
GET /api/v1/webhooks/dlq?webhook_id=<id>&limit=50
Authorization: Bearer <token>
```

## Webhook Payload Format

All webhooks receive a POST request with the following JSON payload:

```json
{
  "event": "checkin.created",
  "event_id": "evt_abc123",
  "webhook": "My Webhook",
  "timestamp": 1699564800,
  "data": {
    "id": "123",
    "name": "My Check-in",
    "completed": true,
    "created_at": "2024-01-15T10:30:00Z"
  }
}
```

### Headers

| Header | Description |
|--------|-------------|
| `Content-Type` | `application/json` |
| `User-Agent` | `DoneList-Webhook/2.0` |
| `X-Webhook-Event` | Event type (e.g., `checkin.created`) |
| `X-Webhook-Event-ID` | Unique event identifier |
| `X-Webhook-Delivery-ID` | Unique delivery identifier |
| `X-Webhook-Timestamp` | Unix timestamp |
| `X-Webhook-Signature` | HMAC-SHA256 signature |
| `X-Webhook-Signature-256` | Alternative signature header |

## Security

### HMAC Signature Verification

All webhook requests include an HMAC-SHA256 signature in the `X-Webhook-Signature` header.

**Format:** `sha256=<hex_encoded_signature>`

**Verification (Node.js):**
```javascript
const crypto = require('crypto');

function verifySignature(payload, signature, secret) {
  const hmac = crypto.createHmac('sha256', secret);
  hmac.update(payload);
  const expectedSignature = 'sha256=' + hmac.digest('hex');
  return crypto.timingSafeEqual(
    Buffer.from(signature),
    Buffer.from(expectedSignature)
  );
}
```

**Verification (Python):**
```python
import hmac
import hashlib

def verify_signature(payload, signature, secret):
    expected = 'sha256=' + hmac.new(
        secret.encode(),
        payload.encode(),
        hashlib.sha256
    ).hexdigest()
    return hmac.compare_digest(signature, expected)
```

**Verification (Go):**
```go
import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
)

func verifySignature(payload []byte, signature, secret string) bool {
    h := hmac.New(sha256.New, []byte(secret))
    h.Write(payload)
    expected := "sha256=" + hex.EncodeToString(h.Sum(nil))
    return hmac.Equal([]byte(signature), []byte(expected))
}
```

## Retry Strategy

### Exponential Backoff

| Attempt | Delay |
|---------|-------|
| 1 | 1 minute |
| 2 | 2 minutes |
| 3 | 4 minutes |
| 4 | 8 minutes |
| 5 | 16 minutes |
| 6+ | 32 minutes (capped at 1 hour) |

### Retry Conditions

Webhooks are retried when:
- HTTP status code >= 500 (server errors)
- Network timeout or connection error
- DNS resolution failure

Webhooks are NOT retried for:
- HTTP status code 2xx (success)
- HTTP status code 4xx (client errors)
- After max retries exceeded (5 attempts)

### Dead Letter Queue

After exhausting all retry attempts, failed deliveries are moved to the dead letter queue (DLQ) for manual review and potential resending.

## Monitoring

### Prometheus Metrics

```
# Total deliveries by status
webhook_deliveries_total{webhook_id, event_type, status}

# Delivery duration
webhook_delivery_duration_seconds{webhook_id, event_type}

# Delivery attempts distribution
webhook_delivery_attempts{webhook_id, event_type}

# Active webhooks count
webhook_active_count{user_id}

# Queue sizes
webhook_queue_size
webhook_dlq_size

# Errors
webhook_errors_total{webhook_id, event_type, error_type}
```

### Logging

All webhook operations are logged with structured logging:

```json
{
  "level": "info",
  "msg": "Webhook delivered successfully",
  "webhook_id": "123e4567-e89b-12d3-a456-426614174000",
  "delivery_id": "987fcdeb-51a2-43f1-b678-9abc12345678",
  "status_code": 200,
  "duration_ms": 150
}
```

## Usage Examples

### Trigger Webhook Event
```go
import "github.com/dev-jelly/donelist/internal/webhook"

// Trigger webhook
err := webhookService.TriggerEvent(
    ctx,
    webhook.EventCheckinCreated,
    userID,
    map[string]interface{}{
        "id": "123",
        "name": "Morning Check-in",
        "completed": true,
    },
)
```

### Implementing Webhook Receiver
```go
func handleWebhook(w http.ResponseWriter, r *http.Request) {
    // Read payload
    payload, _ := ioutil.ReadAll(r.Body)

    // Verify signature
    signature := r.Header.Get("X-Webhook-Signature")
    if !verifySignature(payload, signature, webhookSecret) {
        http.Error(w, "Invalid signature", http.StatusUnauthorized)
        return
    }

    // Parse payload
    var event webhook.WebhookPayload
    if err := json.Unmarshal(payload, &event); err != nil {
        http.Error(w, "Invalid payload", http.StatusBadRequest)
        return
    }

    // Process event
    switch event.Event {
    case "checkin.created":
        // Handle checkin created
    case "checkin.updated":
        // Handle checkin updated
    }

    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{"status": "received"})
}
```

## Configuration

### Environment Variables

```bash
# Redis connection for Asynq
REDIS_ADDR=localhost:6379

# Webhook delivery timeout
WEBHOOK_TIMEOUT=30s

# Max retry attempts
WEBHOOK_MAX_RETRIES=5

# Queue concurrency
WEBHOOK_CONCURRENCY=10

# Cleanup retention days
WEBHOOK_RETENTION_DAYS=30
```

### Service Initialization

```go
// Create service
repo := webhook.NewRepository(db)
service := webhook.NewService(repo, redisAddr, logger)

// Start background workers
if err := service.Start(); err != nil {
    log.Fatal(err)
}
defer service.Stop()

// Schedule cleanup
service.ScheduleCleanup()
```

## Testing

### Run Tests
```bash
# Unit tests
go test ./internal/webhook/...

# Integration tests
go test -tags=integration ./internal/webhook/...

# With coverage
go test -cover ./internal/webhook/...
```

### Test Webhook Locally

Use tools like ngrok for local testing:
```bash
# Start ngrok
ngrok http 8080

# Use the HTTPS URL as your webhook URL
# https://abc123.ngrok.io/webhook
```

## Best Practices

### For Webhook Consumers

1. **Respond quickly**: Return HTTP 200 within 5 seconds
2. **Process asynchronously**: Queue webhook data for background processing
3. **Verify signatures**: Always verify HMAC signatures
4. **Handle idempotency**: Use `event_id` to prevent duplicate processing
5. **Log requests**: Keep logs of webhook requests for debugging

### For System Administrators

1. **Monitor DLQ**: Regularly review dead letter queue entries
2. **Set up alerts**: Alert on high failure rates or DLQ size
3. **Rotate secrets**: Periodically rotate webhook secrets
4. **Clean up old data**: Run cleanup jobs to remove old delivery records
5. **Scale workers**: Adjust Asynq concurrency based on load

## Troubleshooting

### Webhook Not Being Delivered

1. Check if webhook is active
2. Verify the URL is accessible
3. Check if event type matches subscription
4. Review delivery logs for errors

### High Failure Rate

1. Check target server availability
2. Verify timeout settings
3. Review error logs for patterns
4. Consider implementing circuit breaker

### Delivery Delays

1. Check Asynq queue size
2. Increase worker concurrency
3. Review Redis performance
4. Check for retry storms

## Performance Considerations

### Recommended Settings

- **Workers**: 10-20 concurrent workers
- **Queue size**: Monitor and scale based on throughput
- **Timeout**: 30 seconds default
- **Retention**: 30 days for delivery records
- **Batch size**: 100 retries per check

### Scaling

The webhook system scales horizontally:
- Add more Asynq workers by starting additional service instances
- All workers share the same Redis queue
- Database can be scaled with read replicas for stats/logs

## License

Proprietary - DoneList
