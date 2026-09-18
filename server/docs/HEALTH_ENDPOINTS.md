# Health Check Endpoints Documentation

This document describes the health check endpoints available in the Donelist API, designed for monitoring, load balancing, and Kubernetes orchestration.

## Overview

The application provides multiple health check endpoints with varying levels of detail:

- **Liveness Probes** (`/live`, `/livez`) - Verify the application is running
- **Readiness Probes** (`/ready`, `/readyz`) - Verify the application can accept traffic
- **Health Checks** (`/health`, `/healthz`) - Basic health status
- **Detailed Health** (`/health/detail`, `/healthz/detail`) - Comprehensive health information

## Endpoints

### 1. Liveness Probe

**Endpoints:** `GET /live` or `GET /livez`

**Purpose:** Determines if the application process is alive and responsive. Used by Kubernetes liveness probes to restart unhealthy pods.

**Response Codes:**
- `200 OK` - Application is alive

**Response Example:**
```json
{
  "status": "alive",
  "time": "2025-11-24T08:00:00Z",
  "version": "1.0.0",
  "git_commit": "abc123def456",
  "git_branch": "main",
  "build_time": "2025-11-24T00:00:00Z",
  "uptime": 3600.5
}
```

**Fields:**
- `status` - Always "alive" if responding
- `time` - Current server time in RFC3339 format
- `version` - Application version
- `git_commit` - Git commit SHA
- `git_branch` - Git branch name
- `build_time` - When the binary was built
- `uptime` - Application uptime in seconds

**Use Cases:**
- Kubernetes liveness probe
- Basic "is the server running" check
- Monitoring uptime

### 2. Readiness Probe

**Endpoints:** `GET /ready` or `GET /readyz`

**Purpose:** Determines if the application is ready to accept traffic by checking all critical dependencies (database, Redis). Used by Kubernetes readiness probes to control traffic routing.

**Response Codes:**
- `200 OK` - Application is ready to accept traffic
- `503 Service Unavailable` - Application is not ready (dependencies unavailable)

**Successful Response Example:**
```json
{
  "status": "ready",
  "version": "1.0.0",
  "git_commit": "abc123def456",
  "git_branch": "main",
  "build_time": "2025-11-24T00:00:00Z",
  "uptime": 3600.5,
  "database": {
    "status": "connected",
    "message": "database is healthy and responding"
  },
  "redis": {
    "status": "connected",
    "message": "redis is healthy and responding"
  }
}
```

**Failed Response Example:**
```json
{
  "status": "not ready",
  "error": "database not available",
  "details": "ping failed: connection refused",
  "version": "1.0.0",
  "git_commit": "abc123def456",
  "git_branch": "main",
  "build_time": "2025-11-24T00:00:00Z",
  "uptime": 120.3
}
```

**Critical Dependencies Checked:**
1. **PostgreSQL** - Database connectivity and query execution
2. **Redis** - Cache connectivity and basic operations

**Use Cases:**
- Kubernetes readiness probe
- Load balancer health checks
- Deployment verification
- Zero-downtime deployments

### 3. Basic Health Check

**Endpoints:** `GET /health` or `GET /healthz`

**Purpose:** Simple health status check including all registered health checkers.

**Response Codes:**
- `200 OK` - All components healthy
- `503 Service Unavailable` - One or more components unhealthy or degraded

**Response Example:**
```json
{
  "status": "healthy",
  "time": "2025-11-24T08:00:00Z",
  "version": "1.0.0",
  "git_commit": "abc123def456"
}
```

**Status Values:**
- `healthy` - All components operational
- `degraded` - Some components have issues but service is functional
- `unhealthy` - Critical components failed

**Use Cases:**
- Load balancer health checks
- Simple monitoring checks
- Quick status verification

### 4. Detailed Health Check

**Endpoints:** `GET /health/detail` or `GET /healthz/detail`

**Purpose:** Comprehensive health information including individual component status, build information, and performance metrics.

**Response Codes:**
- `200 OK` - All components healthy
- `503 Service Unavailable` - One or more components unhealthy or degraded

**Response Example:**
```json
{
  "status": "healthy",
  "version": "1.0.0",
  "build_info": {
    "version": "1.0.0",
    "git_commit": "abc123def456",
    "git_branch": "main",
    "build_time": "2025-11-24T00:00:00Z",
    "go_version": "go1.24.10",
    "build_host": "ci-builder-01"
  },
  "timestamp": "2025-11-24T08:00:00Z",
  "uptime_seconds": 3600.5,
  "components": {
    "postgresql": {
      "name": "postgresql",
      "status": "healthy",
      "message": "database is healthy and responding",
      "timestamp": "2025-11-24T08:00:00Z",
      "duration_ms": 5,
      "metadata": {
        "open_connections": 10,
        "in_use": 3,
        "idle": 7,
        "wait_count": 0,
        "max_open_connections": 100
      }
    },
    "redis": {
      "name": "redis",
      "status": "healthy",
      "message": "redis is healthy and responding",
      "timestamp": "2025-11-24T08:00:00Z",
      "duration_ms": 2,
      "metadata": {
        "hits": 1250,
        "misses": 50,
        "timeouts": 0,
        "total_conns": 10,
        "idle_conns": 5,
        "stale_conns": 0
      }
    },
    "system": {
      "name": "system",
      "status": "healthy",
      "message": "system resources are within normal limits",
      "timestamp": "2025-11-24T08:00:00Z",
      "duration_ms": 1,
      "metadata": {
        "memory_mb": 256,
        "goroutines": 45
      }
    }
  }
}
```

**Component Status Values:**
- `healthy` - Component fully operational
- `degraded` - Component functional but with issues (e.g., high latency, connection pool exhaustion)
- `unhealthy` - Component failed or unreachable

**Use Cases:**
- Detailed monitoring and alerting
- Performance analysis
- Troubleshooting
- Operations dashboard
- Metrics collection

## Health Check Components

### PostgreSQL Checker

**What it checks:**
- Database connection via ping
- Query execution (`SELECT 1`)
- Connection pool statistics
- Connection pool capacity

**Timeout:** 2 seconds

**Metadata Provided:**
- `open_connections` - Current open connections
- `in_use` - Connections currently in use
- `idle` - Idle connections available
- `wait_count` - Number of times waited for a connection
- `max_open_connections` - Maximum allowed connections

**Degraded Conditions:**
- Query execution fails (but ping succeeds)
- Connection pool at maximum capacity

### Redis Checker

**What it checks:**
- Redis connection via PING command
- Server information retrieval
- Connection pool statistics
- Timeout rate

**Timeout:** 2 seconds

**Metadata Provided:**
- `hits` - Cache hits
- `misses` - Cache misses
- `timeouts` - Connection timeouts
- `total_conns` - Total connections
- `idle_conns` - Idle connections
- `stale_conns` - Stale connections

**Degraded Conditions:**
- INFO command fails (but PING succeeds)
- High timeout rate (>10%)

### System Checker

**What it checks:**
- Memory usage
- Goroutine count

**Thresholds:**
- Memory: 1024 MB (1 GB)
- Goroutines: 10,000

**Metadata Provided:**
- `memory_mb` - Current memory usage in MB
- `goroutines` - Current goroutine count

**Degraded Conditions:**
- Memory usage exceeds threshold
- Goroutine count exceeds threshold

## Kubernetes Integration

### Liveness Probe Configuration

```yaml
livenessProbe:
  httpGet:
    path: /livez
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 10
  timeoutSeconds: 5
  failureThreshold: 3
```

**Behavior:**
- If 3 consecutive liveness probes fail, Kubernetes restarts the pod
- Use generous `failureThreshold` to avoid unnecessary restarts

### Readiness Probe Configuration

```yaml
readinessProbe:
  httpGet:
    path: /readyz
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 5
  timeoutSeconds: 3
  failureThreshold: 2
```

**Behavior:**
- If readiness probe fails, pod is removed from service endpoints
- Traffic stops flowing until probe succeeds again
- Does not trigger pod restart

### Startup Probe Configuration (Optional)

```yaml
startupProbe:
  httpGet:
    path: /ready
    port: 8080
  initialDelaySeconds: 0
  periodSeconds: 5
  timeoutSeconds: 3
  failureThreshold: 30  # Allow up to 150s for startup
```

**Use for:**
- Applications with slow startup times
- Database migrations during startup
- Resource-intensive initialization

## Load Balancer Integration

### HAProxy Configuration

```haproxy
backend api_servers
    option httpchk GET /health
    http-check expect status 200
    server api1 10.0.1.10:8080 check inter 5s fall 3 rise 2
    server api2 10.0.1.11:8080 check inter 5s fall 3 rise 2
```

### NGINX Configuration

```nginx
upstream api_backend {
    server 10.0.1.10:8080 max_fails=3 fail_timeout=30s;
    server 10.0.1.11:8080 max_fails=3 fail_timeout=30s;
}

location /health {
    access_log off;
    return 200;
}
```

### AWS Application Load Balancer

- **Health Check Path:** `/health`
- **Success Codes:** `200`
- **Interval:** 30 seconds
- **Timeout:** 5 seconds
- **Healthy Threshold:** 2
- **Unhealthy Threshold:** 3

## Monitoring and Alerting

### Prometheus Metrics

The application exposes Prometheus metrics at `/metrics`. You can create alerts based on health check data:

```yaml
groups:
  - name: donelist_health
    rules:
      - alert: ServiceUnhealthy
        expr: up{job="donelist-api"} == 0
        for: 2m
        annotations:
          summary: "Donelist API is down"

      - alert: DatabaseUnhealthy
        expr: health_component_status{component="postgresql"} != 1
        for: 1m
        annotations:
          summary: "Database health check failing"

      - alert: HighMemoryUsage
        expr: health_system_memory_mb > 900
        for: 5m
        annotations:
          summary: "Memory usage above 900MB"
```

### Datadog Integration

```python
from datadog import api, initialize

initialize(api_key='your_api_key', app_key='your_app_key')

# Health check monitor
api.Monitor.create(
    type="service check",
    query="http_check.status".over("url:https://api.donelist.io/health").by("*").last(2).count_by_status(),
    name="Donelist API Health Check",
    message="API health check is failing @oncall",
    options={
        'thresholds': {
            'critical': 2,
            'warning': 1
        },
        'timeout_h': 0,
        'no_data_timeframe': 5
    }
)
```

## Best Practices

### 1. Endpoint Selection

- **Use `/livez` for:** Kubernetes liveness probes
- **Use `/readyz` for:** Kubernetes readiness probes and load balancer checks
- **Use `/health/detail` for:** Monitoring dashboards and debugging

### 2. Timeout Configuration

- Set client timeouts slightly higher than server check timeouts (2s + network overhead)
- Recommended client timeout: 5 seconds

### 3. Check Frequency

- **Liveness:** 10-30 seconds (less frequent, avoids unnecessary restarts)
- **Readiness:** 5-10 seconds (more frequent, quick traffic rerouting)
- **Load Balancer:** 5-15 seconds (balance between responsiveness and load)

### 4. Failure Thresholds

- Use at least 2-3 consecutive failures before taking action
- Prevents false positives from transient issues

### 5. Monitoring Strategy

```
┌─────────────┐
│ Liveness    │ → Ensures process is alive → Restart on failure
└─────────────┘

┌─────────────┐
│ Readiness   │ → Ensures can handle requests → Remove from rotation
└─────────────┘

┌─────────────┐
│ Detailed    │ → Provides deep insights → Alert on degradation
└─────────────┘
```

## Troubleshooting

### Readiness Check Failing

**Symptoms:** Service removed from load balancer, no traffic

**Possible Causes:**
1. Database connection lost
2. Redis connection lost
3. Connection pool exhausted

**Steps:**
1. Check `/health/detail` for specific component failure
2. Verify database connectivity: `psql -h <host> -U <user> -d <db>`
3. Verify Redis connectivity: `redis-cli -h <host> ping`
4. Check application logs for connection errors

### Liveness Check Failing

**Symptoms:** Kubernetes restarting pods repeatedly

**Possible Causes:**
1. Application deadlock
2. Resource exhaustion (memory, CPU)
3. Excessive load preventing response

**Steps:**
1. Check CPU and memory usage
2. Review application logs before restart
3. Examine goroutine count in `/health/detail`
4. Consider increasing `failureThreshold` if transient

### Degraded Status

**Symptoms:** Health check returns 503 but service partially functional

**Possible Causes:**
1. Connection pool at capacity
2. High cache timeout rate
3. Memory pressure
4. High goroutine count

**Steps:**
1. Review component metadata in `/health/detail`
2. Adjust resource limits if needed
3. Scale horizontally if capacity issue
4. Investigate application logic for leaks

## Build Information

Build information is injected at compile time using ldflags:

```bash
go build -ldflags="\
  -X 'github.com/dev-jelly/donelist/internal/health.Version=1.0.0' \
  -X 'github.com/dev-jelly/donelist/internal/health.GitCommit=$(git rev-parse HEAD)' \
  -X 'github.com/dev-jelly/donelist/internal/health.GitBranch=$(git rev-parse --abbrev-ref HEAD)' \
  -X 'github.com/dev-jelly/donelist/internal/health.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)' \
  -X 'github.com/dev-jelly/donelist/internal/health.GoVersion=$(go version | awk '{print $3}')' \
  -X 'github.com/dev-jelly/donelist/internal/health.BuildHost=$(hostname)' \
" ./cmd/api
```

This information appears in all health check responses for version tracking and debugging.

## API Security Considerations

### Public Endpoints

Health check endpoints are intentionally public (no authentication required) for the following reasons:

1. **Load Balancer Access** - Load balancers need unauthenticated access
2. **Kubernetes Access** - Kubelet needs to check health without auth
3. **Monitoring Tools** - External monitoring services need access

### Sensitive Information

- Health endpoints do **not** expose connection strings or credentials
- Metadata provides operational metrics only (connection counts, timing)
- Detailed endpoint may expose topology information (consider restricting in production)

### Rate Limiting

Consider implementing rate limiting for health endpoints to prevent abuse:

```go
// Example: 100 requests per minute per IP
rateLimiter := middleware.NewRateLimiter(100, time.Minute)
router.GET("/health/detail", rateLimiter, healthHandler.DetailedHealthCheck)
```

## Summary

The health check system provides a comprehensive solution for:

- ✅ Kubernetes orchestration (liveness, readiness, startup probes)
- ✅ Load balancer integration
- ✅ Monitoring and alerting
- ✅ Operational visibility
- ✅ Graceful degradation
- ✅ Zero-downtime deployments

All endpoints follow cloud-native best practices and include both standard `/health` and Kubernetes-style `/healthz` naming conventions.
