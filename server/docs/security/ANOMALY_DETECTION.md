# Anomaly Detection and Auto-Blocking System

## Overview

The anomaly detection system provides comprehensive security monitoring, alerting, and automatic blocking capabilities to protect the API from various security threats including brute force attacks, suspicious behavior patterns, and abuse.

## Architecture

The system consists of three main components:

1. **Anomaly Detector** - Detects suspicious behavior patterns
2. **Alert Manager** - Sends notifications through various channels
3. **Auto Blocker** - Automatically blocks suspicious identifiers

### Component Diagram

```
┌─────────────────┐
│   API Request   │
└────────┬────────┘
         │
         ▼
┌─────────────────────────────┐
│  Anomaly Detection          │
│  Middleware                 │
└────────┬────────────────────┘
         │
         ├──► Check if Blocked
         │
         ├──► Run Detection Rules
         │    ├─ Rapid Requests
         │    ├─ High Failure Rate
         │    ├─ Brute Force
         │    ├─ Geographic Anomaly
         │    ├─ Device Anomaly
         │    ├─ Unusual Hours
         │    └─ Multiple IPs
         │
         ▼
┌─────────────────────────────┐
│  Anomaly Detected?          │
└────────┬────────────────────┘
         │
         ├──► Send Alert
         │    ├─ Log
         │    ├─ Webhook
         │    ├─ Slack
         │    └─ PagerDuty
         │
         └──► Evaluate Auto-Block
              └─ Block if criteria met
```

## Detection Rules

### 1. Rapid Requests

Detects unusually high request rates from a single identifier.

**Configuration:**
- `RapidRequestThreshold`: Max requests per window (default: 100)
- `RapidRequestWindow`: Time window (default: 1 minute)

**Severity Calculation:**
- Low: 1-2x threshold
- Medium: 2-3x threshold
- High: 3-5x threshold
- Critical: 5x+ threshold

### 2. High Failure Rate

Detects high percentage of failed requests.

**Configuration:**
- `FailureRateThreshold`: Max failure percentage (default: 0.5 = 50%)
- `FailureRateWindow`: Time window (default: 5 minutes)
- `FailureRateMinRequests`: Minimum requests to trigger (default: 10)

**Example:**
```
Requests: 10 total, 7 failed = 70% failure rate
Threshold: 50%
Result: ANOMALY DETECTED
```

### 3. Brute Force

Detects brute force authentication attempts.

**Configuration:**
- `BruteForceThreshold`: Max failed attempts (default: 10)
- `BruteForceWindow`: Time window (default: 10 minutes)

**Severity:** Always Critical

### 4. Geographic Anomaly

Detects impossible travel (location changes too quickly).

**Configuration:**
- `EnableGeoCheck`: Enable/disable (default: false, requires IP geolocation)
- `GeoChangeThreshold`: Minimum time for location change (default: 1 hour)

**Example:**
```
Last seen: US at 10:00
Current: China at 10:05
Time elapsed: 5 minutes
Result: ANOMALY DETECTED (impossible travel)
```

### 5. Device Anomaly

Detects rapid device/user agent changes.

**Configuration:**
- `EnableDeviceCheck`: Enable/disable (default: true)
- `DeviceChangeThreshold`: Minimum time for device change (default: 1 hour)

### 6. Unusual Hours

Detects activity during unusual hours (e.g., 2 AM - 5 AM).

**Configuration:**
- `UnusualHoursStart`: Start hour (default: 2)
- `UnusualHoursEnd`: End hour (default: 5)

**Severity:** Always Low (informational)

### 7. Multiple IPs

Detects access from too many different IP addresses.

**Configuration:**
- `MultipleIPsThreshold`: Max unique IPs (default: 5)
- `MultipleIPsWindow`: Time window (default: 10 minutes)

## Alert Management

### Alert Channels

1. **Log** - Structured logging via zap
2. **Webhook** - HTTP POST to configured endpoint
3. **Slack** - Slack webhook integration
4. **PagerDuty** - PagerDuty Events API v2

### Alert Routing

Alerts are routed to channels based on severity:

| Severity | Channels |
|----------|----------|
| Critical | Log + Webhook + Slack + PagerDuty |
| High     | Log + Webhook + Slack |
| Medium   | Log + Webhook |
| Low      | Log |

### Alert Throttling

To prevent alert storms:
- Maximum 10 alerts per identifier per 5-minute window
- Similar alerts are grouped

### Alert Lifecycle

```
┌─────────┐     ┌──────┐     ┌──────────┐     ┌─────────┐
│ Pending │ ──► │ Sent │ ──► │ Resolved │  or │ Ignored │
└─────────┘     └──────┘     └──────────┘     └─────────┘
```

## Auto-Blocking

### Blocking Rules

1. **Critical Severity**: Immediate block
2. **Multiple High Severity**: Block after N anomalies in window
   - Default: 3 anomalies in 10 minutes

### Block Durations

| Severity | Duration |
|----------|----------|
| Critical | 24 hours |
| High     | 2 hours  |
| Medium   | 30 minutes |
| Low      | 5 minutes |

### Block Storage

- **Redis**: Fast lookup cache with TTL
- **PostgreSQL**: Persistent storage and audit trail

### Whitelist

Prevent blocking of trusted identifiers:
- Configure in `WhitelistedIdentifiers`
- Configure in `WhitelistedIPs`
- Add dynamically via `AddToWhitelist()`

### Manual Operations

```go
// Manually block
blocker.BlockIdentifier(ctx, identifier, ip, reason, description, severity, metadata)

// Manually unblock
blocker.UnblockIdentifier(ctx, identifier, adminID, "reason for unblock")

// Check block status
isBlocked, err := blocker.IsBlocked(ctx, identifier)
blockInfo, err := blocker.GetBlockInfo(ctx, identifier)
```

## Configuration

### Production Configuration

```go
config := security.DefaultSecurityConfig(db, redis, logger)
```

### Development Configuration

```go
config := security.DevelopmentSecurityConfig(db, redis, logger)
// - Relaxed thresholds
// - Auto-blocking disabled
// - Log-only alerts
```

### Strict Configuration

```go
config := security.StrictSecurityConfig(db, redis, logger)
// - Stricter thresholds
// - All alert channels enabled
// - Aggressive auto-blocking
```

### Custom Configuration

```go
config := &security.SecurityConfig{
    EnableAnomalyDetection: true,
    EnableAlerts:           true,
    EnableAutoBlocking:     true,

    AnomalyDetection: security.AnomalyDetectorConfig{
        RapidRequestThreshold: 50,
        RapidRequestWindow:    30 * time.Second,
        BruteForceThreshold:   5,
        // ... other settings
    },

    AlertManagement: security.AlertManagerConfig{
        EnabledChannels: []security.AlertChannel{
            security.AlertChannelLog,
            security.AlertChannelSlack,
        },
        SlackWebhookURL: "https://hooks.slack.com/...",
        // ... other settings
    },

    AutoBlocking: security.AutoBlockerConfig{
        EnableAutoBlock:       true,
        AutoBlockOnCritical:   true,
        CriticalBlockDuration: 48 * time.Hour,
        // ... other settings
    },
}

manager := security.NewSecurityManager(config)
manager.Start()
```

## Integration

### Gin Middleware

```go
import (
    "github.com/dev-jelly/donelist/internal/middleware"
    "github.com/dev-jelly/donelist/internal/security"
)

// Setup security manager
config := security.DefaultSecurityConfig(db, redis, logger)
securityManager := security.NewSecurityManager(config)
securityManager.Start()

// Add middleware
router.Use(middleware.AnomalyDetectionMiddleware(
    middleware.CreateDefaultAnomalyDetectionConfig(
        securityManager.Detector,
        securityManager.AlertManager,
        securityManager.AutoBlocker,
        logger,
    ),
))

// Or use lightweight block-only middleware
router.Use(middleware.BlockCheckMiddleware(
    securityManager.AutoBlocker,
    logger,
))
```

### Manual Detection

```go
// Check for anomalies
anomaly, err := detector.CheckRapidRequests(ctx, userID)
if err != nil {
    return err
}

if anomaly != nil {
    // Send alert
    if err := alertManager.SendAlert(ctx, anomaly); err != nil {
        logger.Error("Failed to send alert", zap.Error(err))
    }

    // Evaluate blocking
    shouldBlock, err := autoBlocker.EvaluateBlock(ctx, anomaly)
    if err != nil {
        return err
    }

    if shouldBlock {
        logger.Warn("User auto-blocked", zap.String("user_id", userID))
    }
}
```

## Database Schema

### security_alerts Table

```sql
CREATE TABLE security_alerts (
    id UUID PRIMARY KEY,
    anomaly_type VARCHAR(50) NOT NULL,
    severity VARCHAR(20) NOT NULL,
    priority VARCHAR(20) NOT NULL,
    identifier VARCHAR(255) NOT NULL,
    ip_address VARCHAR(45) NOT NULL,
    user_agent TEXT,
    description TEXT NOT NULL,
    metadata JSONB,
    score DECIMAL(5,2),
    status VARCHAR(20) NOT NULL,
    channels JSONB,
    sent_at TIMESTAMP,
    resolved_at TIMESTAMP,
    resolved_by UUID,
    notes TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);
```

### security_blocks Table

```sql
CREATE TABLE security_blocks (
    id UUID PRIMARY KEY,
    identifier VARCHAR(255) NOT NULL,
    ip_address VARCHAR(45) NOT NULL,
    reason VARCHAR(50) NOT NULL,
    description TEXT NOT NULL,
    severity VARCHAR(20) NOT NULL,
    metadata JSONB,
    status VARCHAR(20) NOT NULL,
    blocked_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    released_at TIMESTAMP,
    released_by UUID,
    notes TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);
```

## API Endpoints

### Admin Endpoints

#### List Alerts
```
GET /api/v1/admin/security/alerts?identifier=user123&severity=high
```

#### Resolve Alert
```
POST /api/v1/admin/security/alerts/:id/resolve
{
    "notes": "False positive, user verified"
}
```

#### List Blocks
```
GET /api/v1/admin/security/blocks?status=active
```

#### Unblock Identifier
```
POST /api/v1/admin/security/blocks/:identifier/unblock
{
    "notes": "User verified, unblocking"
}
```

#### Get Statistics
```
GET /api/v1/admin/security/statistics?since=2024-01-01T00:00:00Z
```

Response:
```json
{
    "alerts": {
        "total_alerts": 150,
        "by_severity": {
            "critical": 5,
            "high": 20,
            "medium": 50,
            "low": 75
        },
        "by_type": {
            "rapid_requests": 50,
            "brute_force": 10,
            "high_failure_rate": 30
        }
    },
    "blocks": {
        "total_blocks": 25,
        "active_blocks": 5,
        "by_reason": {
            "brute_force": 10,
            "rapid_requests": 15
        }
    }
}
```

## Monitoring

### Key Metrics

1. **Detection Metrics**
   - Anomalies detected per hour
   - Anomalies by type
   - Detection latency

2. **Alert Metrics**
   - Alerts sent per hour
   - Alerts by severity
   - Alert delivery success rate
   - Throttled alerts

3. **Blocking Metrics**
   - Active blocks
   - Blocks created per hour
   - Average block duration
   - Manual unblocks

### Health Checks

```go
// Check if Redis is available
redisStatus := detector.redis.Ping(ctx).Err()

// Check database connectivity
dbStatus := autoBlocker.db.Exec("SELECT 1").Error

// Get system statistics
stats, err := manager.GetStatistics(time.Now().Add(-24 * time.Hour))
```

## Testing

### Unit Tests

```bash
go test -v ./internal/security/...
```

### Integration Tests

```bash
go test -v -tags=integration ./internal/security/...
```

### Load Testing

```bash
# Test rate limiting
for i in {1..150}; do
    curl http://localhost:8080/api/v1/health &
done

# Expected: First 100 succeed, remaining blocked
```

## Best Practices

1. **Start with Development Config** - Use relaxed thresholds initially
2. **Monitor False Positives** - Adjust thresholds based on legitimate traffic
3. **Use Whitelisting** - Whitelist known good actors (monitoring services, etc.)
4. **Review Alerts Regularly** - Check for patterns and adjust rules
5. **Test Blocking** - Verify unblock procedures work before going to production
6. **Configure Multiple Channels** - Don't rely on a single alert channel
7. **Set Up Dashboards** - Monitor key metrics in real-time
8. **Document Incidents** - Use alert notes to track resolutions

## Troubleshooting

### High False Positive Rate

1. Check thresholds are appropriate for traffic patterns
2. Review unusual hours configuration
3. Consider whitelisting legitimate high-volume users
4. Adjust failure rate threshold

### Alerts Not Sending

1. Check enabled channels configuration
2. Verify webhook URLs are accessible
3. Check for alert throttling
4. Review logs for errors

### Blocks Not Working

1. Verify auto-blocking is enabled
2. Check Redis connectivity
3. Verify middleware is registered
4. Check whitelist configuration

### Performance Issues

1. Ensure Redis is properly configured
2. Add indexes to database tables
3. Consider increasing throttle window
4. Review detection rule complexity

## Security Considerations

1. **Sensitive Data** - Alerts may contain IP addresses and user agents
2. **Admin Access** - Protect unblock endpoints with admin authentication
3. **Audit Trail** - All blocks and unblocks are logged
4. **Rate Limiting** - Apply rate limiting to admin endpoints
5. **Webhook Security** - Use HMAC signatures for webhook payloads

## Future Enhancements

1. Machine learning-based anomaly detection
2. IP geolocation integration
3. Behavioral analysis
4. User reputation scoring
5. Automated response playbooks
6. Integration with SIEM systems
7. Advanced correlation analysis
8. Custom detection rules via configuration
