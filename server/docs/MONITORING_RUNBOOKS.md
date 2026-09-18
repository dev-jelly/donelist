# Donelist API - Monitoring Runbooks and Incident Response

## Table of Contents

1. [Overview](#overview)
2. [Alert Severity Levels](#alert-severity-levels)
3. [On-Call Response Procedures](#on-call-response-procedures)
4. [Critical Alerts](#critical-alerts)
5. [Warning Alerts](#warning-alerts)
6. [Database and Cache Alerts](#database-and-cache-alerts)
7. [Infrastructure Alerts](#infrastructure-alerts)
8. [Performance Investigation](#performance-investigation)
9. [Incident Communication](#incident-communication)
10. [Post-Incident Review](#post-incident-review)

## Overview

This document provides runbooks for responding to alerts and incidents in the Donelist API production environment. Each runbook includes:
- Alert description and trigger conditions
- Severity classification
- Immediate response steps
- Investigation procedures
- Resolution strategies
- Escalation paths

### Quick Reference

| Alert | Severity | Response Time | Runbook |
|-------|----------|---------------|---------|
| CriticalErrorRate | Critical | 5 minutes | [Link](#critical-error-rate) |
| DatabaseConnectionFailure | Critical | 2 minutes | [Link](#database-connection-failure) |
| PodCrashLooping | Critical | 5 minutes | [Link](#pod-crash-looping) |
| DeploymentRolloutStuck | Critical | 10 minutes | [Link](#deployment-rollout-stuck) |
| HighErrorRate | Warning | 15 minutes | [Link](#high-error-rate) |
| HighResponseTime | Warning | 15 minutes | [Link](#high-response-time) |
| HighMemoryUsage | Warning | 30 minutes | [Link](#high-memory-usage) |
| HighCPUUsage | Warning | 30 minutes | [Link](#high-cpu-usage) |

## Alert Severity Levels

### Critical
- **Response Time**: < 5 minutes
- **Impact**: Service degradation or complete outage
- **On-Call**: Immediate page
- **Communication**: Incident channel + status page

### Warning
- **Response Time**: < 15 minutes
- **Impact**: Potential service degradation
- **On-Call**: Notification (no page)
- **Communication**: Incident channel

### Info
- **Response Time**: Next business day
- **Impact**: No immediate service impact
- **On-Call**: Logged only
- **Communication**: Regular team channels

## On-Call Response Procedures

### When You Receive an Alert

1. **Acknowledge Alert** (within 5 minutes for critical)
   ```bash
   # Via Slack, PagerDuty, or Alertmanager UI
   ```

2. **Check Status Dashboard**
   - Open Grafana: https://grafana.example.com/d/donelist-overview
   - Review current metrics and trends
   - Check if multiple alerts are firing

3. **Assess Severity and Impact**
   - Is the service responding?
   - Are users affected?
   - Is data at risk?

4. **Follow Runbook**
   - Locate specific runbook for the alert
   - Execute immediate mitigation steps
   - Begin investigation if needed

5. **Communicate**
   - Post initial assessment in #donelist-incidents
   - Update status page if user-facing
   - Set up incident call if needed

6. **Document**
   - Log all actions taken
   - Record timestamps
   - Note findings

## Critical Alerts

### Critical Error Rate

**Alert Name**: `CriticalErrorRate`

**Trigger**: Error rate > 15% for 2 minutes

**Impact**: Major service degradation, users experiencing failures

#### Immediate Response (< 2 minutes)

1. **Verify the alert is real**:
   ```bash
   # Check error rate from multiple pods
   kubectl logs -n donelist -l app=donelist --tail=100 | grep -i error

   # Check Prometheus query
   curl -G 'http://prometheus:9090/api/v1/query' \
     --data-urlencode 'query=sum(rate(http_requests_total{status=~"5.."}[5m]))/sum(rate(http_requests_total[5m]))'
   ```

2. **Check service health**:
   ```bash
   # Quick health check
   kubectl get pods -n donelist
   kubectl get deployment -n donelist

   # Check endpoint
   curl -i https://api.donelist.example.com/health
   ```

3. **Communicate impact**:
   ```
   #donelist-incidents
   🚨 CRITICAL: Error rate at 18%, investigating...
   - Service: Donelist API
   - Started: 2024-01-15 14:23 UTC
   - Status: Investigating
   ```

#### Investigation (< 5 minutes)

1. **Identify error patterns**:
   ```bash
   # Check error logs by endpoint
   kubectl logs -n donelist deployment/donelist-api --tail=500 | \
     jq 'select(.level=="error") | {time, path, error, statusCode}'

   # Check most common errors
   kubectl logs -n donelist deployment/donelist-api --tail=1000 | \
     jq -r 'select(.level=="error") | .error' | sort | uniq -c | sort -rn
   ```

2. **Check recent deployments**:
   ```bash
   # Check deployment history
   kubectl rollout history deployment/donelist-api -n donelist

   # Check current image
   kubectl get deployment donelist-api -n donelist -o jsonpath='{.spec.template.spec.containers[0].image}'
   ```

3. **Check database connectivity**:
   ```bash
   # Test database connection
   kubectl exec -n donelist deployment/donelist-api -- \
     psql $DATABASE_URL -c "SELECT 1;"

   # Check connection pool
   kubectl logs -n donelist deployment/donelist-api --tail=100 | grep -i "connection"
   ```

4. **Check dependencies**:
   ```bash
   # Redis health
   kubectl exec -n donelist deployment/donelist-api -- \
     redis-cli -h redis ping

   # External API availability
   curl -i https://external-api.example.com/health
   ```

#### Mitigation Options

**Option 1: Rollback to Previous Version** (if recent deployment)
```bash
# Check if deployment is recent
kubectl rollout history deployment/donelist-api -n donelist

# Rollback to previous version
kubectl rollout undo deployment/donelist-api -n donelist

# Monitor rollback
kubectl rollout status deployment/donelist-api -n donelist

# Verify error rate decreased
# Check Grafana dashboard
```

**Option 2: Scale Down/Up** (if pod issue)
```bash
# Force pod restart
kubectl rollout restart deployment/donelist-api -n donelist

# Or scale down and up
kubectl scale deployment donelist-api -n donelist --replicas=0
sleep 10
kubectl scale deployment donelist-api -n donelist --replicas=3
```

**Option 3: Enable Circuit Breaker** (if dependency issue)
```bash
# If Redis is causing issues, restart it
kubectl rollout restart statefulset/redis -n donelist

# Or temporarily disable caching via config
kubectl set env deployment/donelist-api -n donelist CACHE_ENABLED=false
```

**Option 4: Rate Limiting** (if overload)
```bash
# Increase rate limits temporarily
kubectl patch configmap donelist-config -n donelist \
  --patch '{"data":{"RATE_LIMIT":"1000"}}'

# Restart deployment to pick up config
kubectl rollout restart deployment/donelist-api -n donelist
```

#### Resolution Verification

```bash
# Verify error rate decreased
# Check Grafana: Error Rate panel

# Check application logs
kubectl logs -n donelist deployment/donelist-api --tail=100

# Test critical endpoints
curl -i https://api.donelist.example.com/api/v1/checkins
curl -i https://api.donelist.example.com/api/v1/users/me
```

#### Communication

```
#donelist-incidents
✅ RESOLVED: Error rate back to normal (0.5%)
- Root cause: [describe]
- Mitigation: [action taken]
- Duration: 8 minutes
- Next steps: [follow-up actions]
```

---

### Database Connection Failure

**Alert Name**: `DatabaseConnectionFailure`

**Trigger**: Database connection errors > 0 for 2 minutes

**Impact**: Service unable to read/write data

#### Immediate Response (< 1 minute)

1. **Check database availability**:
   ```bash
   # Quick connection test
   kubectl exec -n donelist deployment/donelist-api -- \
     pg_isready -h $DB_HOST -p 5432

   # Try to connect
   kubectl exec -n donelist deployment/donelist-api -- \
     psql $DATABASE_URL -c "SELECT NOW();"
   ```

2. **Check connection limits**:
   ```bash
   # Check active connections
   kubectl exec -n donelist deployment/donelist-api -- \
     psql $DATABASE_URL -c "SELECT count(*) FROM pg_stat_activity;"

   # Check connection limit
   kubectl exec -n donelist deployment/donelist-api -- \
     psql $DATABASE_URL -c "SHOW max_connections;"
   ```

3. **Communicate**:
   ```
   #donelist-incidents
   🚨 CRITICAL: Database connection failure
   - Database: PostgreSQL production
   - Started: [timestamp]
   - Status: Investigating
   - User Impact: Unable to access data
   ```

#### Investigation

1. **Check database pod/instance**:
   ```bash
   # For managed database (RDS, CloudSQL):
   # Check cloud console for instance status

   # For self-hosted:
   kubectl get pods -n database
   kubectl logs -n database statefulset/postgresql --tail=200
   ```

2. **Check network connectivity**:
   ```bash
   # Test network from API pod
   kubectl exec -n donelist deployment/donelist-api -- \
     nc -zv $DB_HOST 5432

   # Check DNS resolution
   kubectl exec -n donelist deployment/donelist-api -- \
     nslookup $DB_HOST
   ```

3. **Check database logs**:
   ```bash
   # For RDS
   aws rds download-db-log-file-portion \
     --db-instance-identifier donelist-prod \
     --log-file-name error/postgresql.log.2024-01-15-14 \
     --output text

   # For self-hosted
   kubectl logs -n database statefulset/postgresql --tail=500 | \
     grep -i "error\|fatal\|connection"
   ```

4. **Check application connection pool**:
   ```bash
   # View pool stats
   kubectl logs -n donelist deployment/donelist-api --tail=500 | \
     grep -i "pool\|connection" | tail -50
   ```

#### Mitigation Options

**Option 1: Reset Connection Pool**
```bash
# Restart API pods to reset connections
kubectl rollout restart deployment/donelist-api -n donelist

# Monitor logs
kubectl logs -n donelist deployment/donelist-api -f
```

**Option 2: Increase Connection Limits**
```bash
# For managed database, increase max_connections via console

# For self-hosted, update PostgreSQL config
kubectl exec -n database statefulset/postgresql-0 -- \
  psql -c "ALTER SYSTEM SET max_connections = 200;"

# Restart PostgreSQL
kubectl rollout restart statefulset/postgresql -n database
```

**Option 3: Scale Down API Replicas**
```bash
# Temporarily reduce API replicas to free connections
kubectl scale deployment donelist-api -n donelist --replicas=2

# Monitor connection count
watch -n 2 'kubectl exec -n donelist deployment/donelist-api -- \
  psql $DATABASE_URL -c "SELECT count(*) FROM pg_stat_activity;"'
```

**Option 4: Failover to Replica** (if primary is down)
```bash
# For managed databases, trigger failover via console

# For self-hosted with replication:
kubectl exec -n database statefulset/postgresql-1 -- \
  pg_ctl promote
```

#### Escalation

If issue persists > 5 minutes:
- **Database Team Lead**: page immediately
- **SRE Team**: engage for infrastructure support
- **Engineering Manager**: notify for business impact

---

### Pod Crash Looping

**Alert Name**: `PodCrashLooping`

**Trigger**: Pod restart rate > 0 for 5 minutes

**Impact**: Reduced capacity, potential service instability

#### Immediate Response

1. **Check pod status**:
   ```bash
   # View pod status
   kubectl get pods -n donelist -l app=donelist

   # Check restart count
   kubectl get pods -n donelist -l app=donelist \
     -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.containerStatuses[0].restartCount}{"\n"}{end}'
   ```

2. **Get pod logs**:
   ```bash
   # Get current logs
   kubectl logs -n donelist <pod-name>

   # Get previous crash logs
   kubectl logs -n donelist <pod-name> --previous
   ```

3. **Check events**:
   ```bash
   # View recent events
   kubectl get events -n donelist --sort-by='.lastTimestamp' | tail -20

   # Pod-specific events
   kubectl describe pod -n donelist <pod-name>
   ```

#### Investigation

1. **Identify crash cause**:
   ```bash
   # Common patterns to check:
   # - OOMKilled (out of memory)
   # - CrashLoopBackOff (application crash)
   # - ImagePullBackOff (image issues)
   # - Error (startup failure)

   kubectl describe pod -n donelist <pod-name> | \
     grep -A 5 "State:\|Last State:"
   ```

2. **Check resource usage**:
   ```bash
   # Current resource usage
   kubectl top pods -n donelist

   # Historical usage in Grafana
   # Check "Memory Usage" and "CPU Usage" panels
   ```

3. **Check configuration**:
   ```bash
   # View environment variables
   kubectl get deployment donelist-api -n donelist \
     -o jsonpath='{.spec.template.spec.containers[0].env}'

   # Check configmap
   kubectl get configmap donelist-config -n donelist -o yaml
   ```

#### Mitigation Options

**If OOMKilled** (memory limit exceeded):
```bash
# Increase memory limit
kubectl patch deployment donelist-api -n donelist --patch '
spec:
  template:
    spec:
      containers:
      - name: donelist-api
        resources:
          limits:
            memory: 1Gi
          requests:
            memory: 512Mi
'

# Monitor pods
kubectl get pods -n donelist -w
```

**If Application Crash**:
```bash
# Check if recent deployment
kubectl rollout history deployment/donelist-api -n donelist

# Rollback if needed
kubectl rollout undo deployment/donelist-api -n donelist

# Or fix config and redeploy
kubectl set env deployment/donelist-api -n donelist DATABASE_URL=$FIXED_URL
```

**If Liveness Probe Failure**:
```bash
# Adjust probe timing
kubectl patch deployment donelist-api -n donelist --patch '
spec:
  template:
    spec:
      containers:
      - name: donelist-api
        livenessProbe:
          initialDelaySeconds: 60
          periodSeconds: 20
          timeoutSeconds: 10
'
```

---

### Deployment Rollout Stuck

**Alert Name**: `DeploymentRolloutStuck`

**Trigger**: Deployment not progressing for 10 minutes

**Impact**: Unable to deploy updates, potential service issues

#### Immediate Response

1. **Check rollout status**:
   ```bash
   # View rollout status
   kubectl rollout status deployment/donelist-api -n donelist

   # Check deployment details
   kubectl describe deployment donelist-api -n donelist
   ```

2. **Check pod status**:
   ```bash
   # View all pods including pending/failed
   kubectl get pods -n donelist -l app=donelist -o wide

   # Check events
   kubectl get events -n donelist --sort-by='.lastTimestamp' | grep -i deploy
   ```

#### Investigation

1. **Identify blocking issue**:
   ```bash
   # Common issues:
   # - Insufficient resources
   # - Image pull errors
   # - Failed health checks
   # - Pod scheduling issues

   # Check pod describe for details
   kubectl describe pod -n donelist $(kubectl get pods -n donelist -l app=donelist --field-selector=status.phase!=Running -o name | head -1)
   ```

2. **Check resources**:
   ```bash
   # Node resources
   kubectl top nodes

   # Available resources
   kubectl describe nodes | grep -A 5 "Allocated resources"
   ```

3. **Check image availability**:
   ```bash
   # Check image pull status
   kubectl get events -n donelist | grep -i "pull"

   # Verify image exists
   docker pull <image-from-deployment>
   ```

#### Mitigation Options

**Option 1: Pause and Resume**
```bash
# Pause rollout
kubectl rollout pause deployment/donelist-api -n donelist

# Fix underlying issue (see below)

# Resume rollout
kubectl rollout resume deployment/donelist-api -n donelist
```

**Option 2: Rollback**
```bash
# Rollback to previous version
kubectl rollout undo deployment/donelist-api -n donelist

# Monitor rollback
kubectl rollout status deployment/donelist-api -n donelist
```

**Option 3: Scale Down Resources**
```bash
# If insufficient resources, temporarily reduce other workloads
kubectl scale deployment other-workload -n donelist --replicas=1

# Or add nodes to cluster (if cloud provider supports)
```

**Option 4: Fix Image Issues**
```bash
# If image pull failure, update image with correct tag
kubectl set image deployment/donelist-api -n donelist \
  donelist-api=ghcr.io/org/donelist-api:v1.2.3

# Or update imagePullSecrets if needed
```

---

## Warning Alerts

### High Error Rate

**Alert Name**: `HighErrorRate`

**Trigger**: Error rate > 5% for 5 minutes

**Impact**: Some users experiencing issues

#### Response Steps

1. **Verify and assess**:
   ```bash
   # Check current error rate
   kubectl logs -n donelist deployment/donelist-api --tail=200 | \
     jq 'select(.statusCode >= 500)' | wc -l

   # Check affected endpoints
   kubectl logs -n donelist deployment/donelist-api --tail=500 | \
     jq -r 'select(.statusCode >= 500) | .path' | sort | uniq -c | sort -rn
   ```

2. **Identify patterns**:
   - Specific endpoints failing?
   - Specific users affected?
   - Time-based pattern?
   - Correlated with load?

3. **Check for known issues**:
   ```bash
   # Recent changes
   git log --since="2 hours ago" --oneline

   # Recent deployments
   kubectl rollout history deployment/donelist-api -n donelist
   ```

4. **Monitor trend**:
   - Is it increasing?
   - Is it affecting more endpoints?
   - Check Grafana trends

5. **Take action if worsening**:
   - If error rate > 10%: escalate to critical
   - If stable at 5-8%: investigate root cause
   - If decreasing: monitor and document

---

### High Response Time

**Alert Name**: `HighResponseTime`

**Trigger**: p95 response time > 1 second for 5 minutes

**Impact**: Degraded user experience

#### Response Steps

1. **Identify slow endpoints**:
   ```bash
   # Check slowest endpoints in Grafana
   # Query: topk(10, histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m])) by (path))
   ```

2. **Check database performance**:
   ```bash
   # Slow queries
   kubectl exec -n donelist deployment/donelist-api -- \
     psql $DATABASE_URL -c "
       SELECT query, calls, mean_exec_time, max_exec_time
       FROM pg_stat_statements
       ORDER BY mean_exec_time DESC
       LIMIT 10;
     "
   ```

3. **Check cache hit rate**:
   ```bash
   # Prometheus query
   # (sum(rate(cache_hits_total[5m])) / (sum(rate(cache_hits_total[5m])) + sum(rate(cache_misses_total[5m])))) * 100
   ```

4. **Check resource usage**:
   ```bash
   kubectl top pods -n donelist
   ```

5. **Optimization options**:
   - Add missing indexes
   - Increase cache TTL
   - Scale up replicas
   - Optimize slow queries

---

### High Memory Usage

**Alert Name**: `HighMemoryUsage`

**Trigger**: Memory usage > 85% for 5 minutes

**Impact**: Risk of OOM kill

#### Response Steps

1. **Check current usage**:
   ```bash
   kubectl top pods -n donelist

   # Memory by container
   kubectl get pods -n donelist -o json | \
     jq -r '.items[] | .metadata.name + " " + (.spec.containers[].resources.limits.memory // "no limit")'
   ```

2. **Check for memory leaks**:
   ```bash
   # Monitor memory trend over time in Grafana
   # Look for steady increase vs. normal fluctuation
   ```

3. **Check application metrics**:
   ```bash
   # If Go app, check heap profile
   kubectl port-forward -n donelist deployment/donelist-api 6060:6060
   go tool pprof http://localhost:6060/debug/pprof/heap
   ```

4. **Mitigation options**:

   **Immediate** (if > 95%):
   ```bash
   # Restart pod to free memory
   kubectl delete pod -n donelist <pod-name>
   ```

   **Short-term**:
   ```bash
   # Increase memory limit
   kubectl patch deployment donelist-api -n donelist --patch '
   spec:
     template:
       spec:
         containers:
         - name: donelist-api
           resources:
             limits:
               memory: 1Gi
   '
   ```

   **Long-term**:
   - Investigate memory leak
   - Optimize caching strategy
   - Implement memory limits in code

---

### High CPU Usage

**Alert Name**: `HighCPUUsage`

**Trigger**: CPU usage > 85% for 5 minutes

**Impact**: Slower response times, potential throttling

#### Response Steps

1. **Check current usage**:
   ```bash
   kubectl top pods -n donelist
   kubectl top nodes
   ```

2. **Check CPU profile**:
   ```bash
   # For Go app
   kubectl port-forward -n donelist deployment/donelist-api 6060:6060
   go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30
   ```

3. **Check request rate**:
   ```bash
   # Prometheus query
   # sum(rate(http_requests_total[5m]))
   ```

4. **Mitigation options**:

   **Immediate**:
   ```bash
   # Scale up replicas to distribute load
   kubectl scale deployment donelist-api -n donelist --replicas=5
   ```

   **Short-term**:
   ```bash
   # Increase CPU limit
   kubectl patch deployment donelist-api -n donelist --patch '
   spec:
     template:
       spec:
         containers:
         - name: donelist-api
           resources:
             limits:
               cpu: 2000m
   '
   ```

   **Long-term**:
   - Profile and optimize hot code paths
   - Implement caching for expensive operations
   - Enable rate limiting
   - Optimize algorithms

---

## Database and Cache Alerts

### Redis Connection Failure

**Alert Name**: `RedisConnectionFailure`

**Trigger**: Redis connection errors > 0 for 2 minutes

**Impact**: Degraded performance, increased database load

#### Response Steps

1. **Check Redis health**:
   ```bash
   # Quick ping test
   kubectl exec -n donelist deployment/donelist-api -- \
     redis-cli -h redis ping

   # Check Redis status
   kubectl exec -n donelist statefulset/redis -- redis-cli info
   ```

2. **Check connection limits**:
   ```bash
   # Current connections
   kubectl exec -n donelist statefulset/redis -- \
     redis-cli info clients | grep connected_clients

   # Max connections
   kubectl exec -n donelist statefulset/redis -- \
     redis-cli config get maxclients
   ```

3. **Mitigation options**:

   **Immediate** (if Redis is down):
   ```bash
   # Disable caching temporarily
   kubectl set env deployment/donelist-api -n donelist \
     CACHE_ENABLED=false

   # Restart Redis
   kubectl rollout restart statefulset/redis -n donelist
   ```

   **If connection limit reached**:
   ```bash
   # Increase max clients
   kubectl exec -n donelist statefulset/redis -- \
     redis-cli config set maxclients 20000

   # Restart API to reset connections
   kubectl rollout restart deployment/donelist-api -n donelist
   ```

---

## Infrastructure Alerts

### Pod Not Ready

**Alert Name**: `PodNotReady`

**Trigger**: Pod not in Running state for 5 minutes

**Impact**: Reduced capacity

#### Response Steps

1. **Check pod status**:
   ```bash
   kubectl get pods -n donelist
   kubectl describe pod -n donelist <pod-name>
   ```

2. **Check readiness probe**:
   ```bash
   # View probe definition
   kubectl get deployment donelist-api -n donelist \
     -o jsonpath='{.spec.template.spec.containers[0].readinessProbe}'

   # Test endpoint manually
   kubectl exec -n donelist <pod-name> -- \
     curl -i http://localhost:8080/health/ready
   ```

3. **Check logs**:
   ```bash
   kubectl logs -n donelist <pod-name>
   ```

4. **Resolution**:
   - If probe timeout: adjust probe settings
   - If startup slow: increase initialDelaySeconds
   - If persistent failure: check application logs

---

### Low Replica Count

**Alert Name**: `LowReplicaCount`

**Trigger**: Available replicas < 2 for 5 minutes

**Impact**: Reduced capacity and resilience

#### Response Steps

1. **Check deployment status**:
   ```bash
   kubectl get deployment donelist-api -n donelist

   # Check replica details
   kubectl describe deployment donelist-api -n donelist | \
     grep -A 10 "Replicas:"
   ```

2. **Identify why replicas are low**:
   ```bash
   # Check pod status
   kubectl get pods -n donelist -l app=donelist

   # Check for pending pods
   kubectl get pods -n donelist -l app=donelist \
     --field-selector status.phase=Pending
   ```

3. **Common causes**:
   - Insufficient node resources
   - Pod crash looping
   - Image pull failures
   - HPA scaling down

4. **Resolution**:
   ```bash
   # If HPA issue, adjust min replicas
   kubectl patch hpa donelist-api -n donelist --patch '
   spec:
     minReplicas: 3
   '

   # If resource issue, add nodes or reduce resource requests
   ```

---

## Performance Investigation

### Investigating Slow Endpoints

1. **Identify slowest endpoints**:
   ```promql
   # Grafana/Prometheus query
   topk(10,
     histogram_quantile(0.95,
       rate(http_request_duration_seconds_bucket[5m])
     ) by (path)
   )
   ```

2. **Analyze database queries**:
   ```sql
   -- Top 10 slowest queries
   SELECT
     query,
     calls,
     total_exec_time,
     mean_exec_time,
     max_exec_time
   FROM pg_stat_statements
   WHERE query NOT LIKE '%pg_stat_statements%'
   ORDER BY mean_exec_time DESC
   LIMIT 10;
   ```

3. **Check cache effectiveness**:
   ```promql
   # Cache hit rate
   (
     sum(rate(cache_hits_total[5m]))
     /
     (sum(rate(cache_hits_total[5m])) + sum(rate(cache_misses_total[5m])))
   ) * 100
   ```

4. **Enable tracing** (if not already):
   ```bash
   # View traces in Jaeger
   kubectl port-forward -n monitoring svc/jaeger-query 16686:16686

   # Open http://localhost:16686
   # Search for slow traces by service=donelist-api and duration>1s
   ```

5. **Analyze trace**:
   - Which spans are slowest?
   - Database queries?
   - External API calls?
   - Internal processing?

6. **Optimization strategies**:
   - Add database indexes
   - Implement caching
   - Optimize queries (N+1 problems)
   - Parallelize operations
   - Add circuit breakers for external calls

---

### Investigating Memory Issues

1. **Get heap profile**:
   ```bash
   # For Go applications
   kubectl port-forward -n donelist deployment/donelist-api 6060:6060

   # Download heap profile
   curl http://localhost:6060/debug/pprof/heap > heap.prof

   # Analyze
   go tool pprof heap.prof
   # In pprof: top, list <function>, web
   ```

2. **Check for common issues**:
   - Large caches not being evicted
   - Goroutine leaks (for Go)
   - Unbounded slices/maps
   - Circular references preventing GC

3. **Monitor over time**:
   ```bash
   # Continuous monitoring
   watch -n 5 'kubectl top pods -n donelist'
   ```

4. **Verify limits**:
   ```bash
   # Check memory limits
   kubectl get pods -n donelist -o json | \
     jq '.items[] | {
       name: .metadata.name,
       limits: .spec.containers[0].resources.limits.memory,
       requests: .spec.containers[0].resources.requests.memory
     }'
   ```

---

## Incident Communication

### Communication Templates

#### Initial Incident Report
```
🚨 [SEVERITY] Incident: [Brief Description]

Status: Investigating
Started: [timestamp UTC]
Service: [affected service]
Impact: [user impact description]
Response Team: @oncall-engineer

Updates will be posted every 15 minutes.
```

#### Progress Update
```
🔄 Update: [Incident Name]

Current Status: [Investigating/Identified/Implementing Fix]
Findings: [what we've learned]
Next Steps: [what we're doing next]
ETA: [if known]
Duration: [how long since start]
```

#### Resolution
```
✅ RESOLVED: [Incident Name]

Duration: [total time]
Root Cause: [brief explanation]
Resolution: [what fixed it]
User Impact: [who was affected, how much]
Follow-up: [ticket link for post-mortem]
```

### Status Page Updates

Use status page for user-facing incidents:

```bash
# Example using statuspage.io API
curl -X POST https://api.statuspage.io/v1/pages/PAGE_ID/incidents \
  -H "Authorization: OAuth YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "incident": {
      "name": "Elevated error rates",
      "status": "investigating",
      "impact": "minor",
      "body": "We are investigating elevated error rates affecting some users.",
      "component_ids": ["donelist_api"]
    }
  }'
```

### Incident Call Procedures

**When to start incident call**:
- Severity: Critical
- Duration: > 15 minutes
- Multiple services affected
- Complex troubleshooting needed

**Incident Commander Responsibilities**:
1. Run the call
2. Assign roles (investigation lead, communication lead, scribe)
3. Make decisions
4. Provide updates every 15 minutes
5. Declare resolution

**Call Structure**:
```
1. Roll call - who's on the call?
2. Current situation - what's happening?
3. Timeline - what's happened so far?
4. Actions - what are we doing?
5. Updates every 5-10 minutes
6. Resolution confirmation
7. Hand off to post-incident review
```

---

## Post-Incident Review

### Within 24 Hours of Resolution

1. **Create Post-Incident Document**:
   ```markdown
   # Post-Incident Review: [Incident Name]

   ## Incident Summary
   - Date: [date]
   - Duration: [total duration]
   - Severity: [critical/warning]
   - Services Affected: [list]
   - Users Impacted: [number/percentage]

   ## Timeline
   - [timestamp] - Alert fired
   - [timestamp] - On-call engineer acknowledged
   - [timestamp] - Root cause identified
   - [timestamp] - Fix implemented
   - [timestamp] - Incident resolved

   ## Root Cause
   [Detailed explanation of what went wrong]

   ## Resolution
   [What fixed the issue]

   ## What Went Well
   - [Things that worked well in response]

   ## What Could Be Improved
   - [Things to improve for next time]

   ## Action Items
   - [ ] [Action 1] - @owner - [due date]
   - [ ] [Action 2] - @owner - [due date]

   ## Lessons Learned
   [Key takeaways]
   ```

2. **Schedule Review Meeting** (within 48 hours)
   - Invite: incident responders, engineering team, management
   - Duration: 30-60 minutes
   - Goal: Learn, not blame

3. **Track Action Items**:
   - Create tickets for all action items
   - Assign owners and due dates
   - Review in weekly team meeting

4. **Update Runbooks**:
   - Add new procedures discovered
   - Update mitigation steps
   - Add prevention measures

---

## Monitoring Quick Reference

### Key Dashboards

1. **Overview Dashboard**: https://grafana.example.com/d/donelist-overview
   - Request rate, error rate, latency
   - Active connections, memory, CPU
   - Quick health check

2. **Database Dashboard**: https://grafana.example.com/d/donelist-database
   - Query performance
   - Connection pool status
   - Cache hit rate

3. **Business Metrics**: https://grafana.example.com/d/donelist-business
   - Active users
   - Check-in rates
   - Feature usage

4. **SLO Dashboard**: https://grafana.example.com/d/donelist-slo
   - Availability tracking
   - Error budget consumption
   - SLI trends

### Key Commands

```bash
# Quick health check
kubectl get pods -n donelist
curl https://api.donelist.example.com/health

# View logs
kubectl logs -n donelist deployment/donelist-api --tail=100 -f

# Check metrics
kubectl port-forward -n donelist svc/donelist-api 8080:8080
curl http://localhost:8080/metrics

# Database quick check
kubectl exec -n donelist deployment/donelist-api -- \
  psql $DATABASE_URL -c "SELECT version();"

# Redis quick check
kubectl exec -n donelist statefulset/redis -- redis-cli ping

# Restart services
kubectl rollout restart deployment/donelist-api -n donelist

# Rollback deployment
kubectl rollout undo deployment/donelist-api -n donelist

# Scale replicas
kubectl scale deployment donelist-api -n donelist --replicas=5
```

### Contact Information

| Role | Contact | Backup |
|------|---------|--------|
| On-Call Engineer | PagerDuty rotation | #donelist-oncall |
| Database Team | db-team@example.com | @db-lead |
| Infrastructure Team | infra-team@example.com | @infra-lead |
| Engineering Manager | em@example.com | #engineering |

### Escalation Path

1. **Initial Response**: On-call engineer (auto-paged)
2. **15 minutes**: Notify team lead
3. **30 minutes**: Engage specialist (DB/Infra)
4. **1 hour**: Engineering manager + incident commander
5. **2 hours**: VP Engineering + external communication

---

## Appendix: Useful Queries

### Prometheus Queries

```promql
# Current error rate
sum(rate(http_requests_total{status=~"5.."}[5m])) / sum(rate(http_requests_total[5m]))

# p50, p95, p99 latency
histogram_quantile(0.50, rate(http_request_duration_seconds_bucket[5m]))
histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m]))
histogram_quantile(0.99, rate(http_request_duration_seconds_bucket[5m]))

# Request rate by endpoint
sum(rate(http_requests_total[5m])) by (path)

# Cache hit rate
sum(rate(cache_hits_total[5m])) / (sum(rate(cache_hits_total[5m])) + sum(rate(cache_misses_total[5m])))

# Database query duration p95
histogram_quantile(0.95, rate(db_operation_duration_seconds_bucket[5m]))

# Active WebSocket connections
sum(websocket_connections)

# Pod restarts (last hour)
increase(kube_pod_container_status_restarts_total{namespace="donelist"}[1h])
```

### Database Queries

```sql
-- Blocking queries
SELECT
  pid,
  usename,
  pg_blocking_pids(pid) as blocked_by,
  query as blocked_query
FROM pg_stat_activity
WHERE cardinality(pg_blocking_pids(pid)) > 0;

-- Table sizes
SELECT
  schemaname,
  tablename,
  pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) AS size
FROM pg_tables
WHERE schemaname = 'public'
ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC;

-- Index usage
SELECT
  schemaname,
  tablename,
  indexname,
  idx_scan,
  idx_tup_read,
  idx_tup_fetch
FROM pg_stat_user_indexes
ORDER BY idx_scan DESC;

-- Long running queries
SELECT
  pid,
  now() - pg_stat_activity.query_start AS duration,
  query,
  state
FROM pg_stat_activity
WHERE (now() - pg_stat_activity.query_start) > interval '5 minutes'
  AND state = 'active';
```

---

**Document Version**: 1.0
**Last Updated**: 2024-01-15
**Owner**: Platform Team
**Review Schedule**: Quarterly or after major incidents
