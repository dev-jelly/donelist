# Donelist API - Monitoring Quick Reference Card

## Emergency Response

### 🚨 Critical Alert Response (< 5 minutes)

```bash
# 1. Check service status
kubectl get pods -n donelist
curl -i https://api.donelist.example.com/health

# 2. View recent logs
kubectl logs -n donelist deployment/donelist-api --tail=100

# 3. Check error rate
# Open Grafana: https://grafana.example.com/d/donelist-overview

# 4. If needed, rollback deployment
kubectl rollout undo deployment/donelist-api -n donelist
```

### ⚠️ Warning Alert Response (< 15 minutes)

```bash
# 1. Identify issue
kubectl logs -n donelist deployment/donelist-api --tail=200 | grep -i error

# 2. Check resource usage
kubectl top pods -n donelist
kubectl top nodes

# 3. Review metrics in Grafana

# 4. Scale if needed
kubectl scale deployment donelist-api -n donelist --replicas=5
```

---

## Quick Access URLs

| Resource | URL | Purpose |
|----------|-----|---------|
| Grafana | https://grafana.example.com | Dashboards & Metrics |
| Prometheus | https://prometheus.example.com | Metrics & Alerts |
| AlertManager | https://alertmanager.example.com | Alert Management |
| Jaeger | https://jaeger.example.com | Distributed Tracing |
| API Health | https://api.donelist.example.com/health | Service Status |

---

## Key Dashboards

### 1. API Overview
**URL**: https://grafana.example.com/d/donelist-overview

**Use When**: General health check, investigating performance

**Key Panels**:
- Request rate (top left)
- Response time p95 (top right)
- Error rate (middle left)
- Active connections (middle right)

### 2. Database Performance
**URL**: https://grafana.example.com/d/donelist-database

**Use When**: Slow queries, database issues, cache problems

**Key Panels**:
- DB connection pool
- Query duration percentiles
- Cache hit rate
- Slowest queries

### 3. Business Metrics
**URL**: https://grafana.example.com/d/donelist-business

**Use When**: Product questions, usage analysis

**Key Panels**:
- DAU/WAU/MAU
- Check-in creation rate
- Feature usage stats

### 4. SLO Dashboard
**URL**: https://grafana.example.com/d/donelist-slo

**Use When**: SLO compliance, error budget tracking

**Key Panels**:
- Availability (99.9% target)
- Success rate (99.5% target)
- P95 latency (< 500ms target)
- Error budget remaining

---

## Common Commands

### Service Health

```bash
# Check all pods
kubectl get pods -n donelist

# Check specific pod details
kubectl describe pod -n donelist <pod-name>

# Check recent events
kubectl get events -n donelist --sort-by='.lastTimestamp' | tail -10

# Test health endpoint
curl -i https://api.donelist.example.com/health
curl -i https://api.donelist.example.com/health/detail
```

### Logs

```bash
# Tail logs from all pods
kubectl logs -n donelist -l app=donelist --tail=100 -f

# Tail logs from specific pod
kubectl logs -n donelist <pod-name> -f

# Get logs from previous crashed pod
kubectl logs -n donelist <pod-name> --previous

# Filter logs by level
kubectl logs -n donelist deployment/donelist-api --tail=500 | jq 'select(.level=="error")'

# Search logs in timeframe
kubectl logs -n donelist deployment/donelist-api --since=1h | grep "error"
```

### Metrics

```bash
# View Prometheus metrics
kubectl port-forward -n donelist svc/donelist-api 8080:8080
curl http://localhost:8080/metrics

# Check specific metric
curl -s http://localhost:8080/metrics | grep http_requests_total

# Query Prometheus API
curl -G 'http://prometheus:9090/api/v1/query' \
  --data-urlencode 'query=rate(http_requests_total[5m])'
```

### Database

```bash
# Check database connection
kubectl exec -n donelist deployment/donelist-api -- \
  psql $DATABASE_URL -c "SELECT 1;"

# Check active connections
kubectl exec -n donelist deployment/donelist-api -- \
  psql $DATABASE_URL -c "SELECT count(*) FROM pg_stat_activity;"

# View slow queries
kubectl exec -n donelist deployment/donelist-api -- \
  psql $DATABASE_URL -c "
    SELECT query, mean_exec_time, calls
    FROM pg_stat_statements
    ORDER BY mean_exec_time DESC
    LIMIT 10;
  "

# Check database size
kubectl exec -n donelist deployment/donelist-api -- \
  psql $DATABASE_URL -c "
    SELECT pg_size_pretty(pg_database_size('donelist'));
  "
```

### Cache (Redis)

```bash
# Test Redis connection
kubectl exec -n donelist deployment/donelist-api -- \
  redis-cli -h redis ping

# Check Redis info
kubectl exec -n donelist statefulset/redis -- \
  redis-cli info

# Check connection count
kubectl exec -n donelist statefulset/redis -- \
  redis-cli info clients | grep connected_clients

# Check memory usage
kubectl exec -n donelist statefulset/redis -- \
  redis-cli info memory | grep used_memory_human
```

### Deployment Management

```bash
# Check deployment status
kubectl get deployment -n donelist

# View rollout history
kubectl rollout history deployment/donelist-api -n donelist

# Rollback to previous version
kubectl rollout undo deployment/donelist-api -n donelist

# Rollback to specific revision
kubectl rollout undo deployment/donelist-api -n donelist --to-revision=3

# Check rollout status
kubectl rollout status deployment/donelist-api -n donelist

# Restart deployment (rolling restart)
kubectl rollout restart deployment/donelist-api -n donelist
```

### Scaling

```bash
# Scale replicas
kubectl scale deployment donelist-api -n donelist --replicas=5

# Check current scale
kubectl get deployment donelist-api -n donelist

# View HPA status
kubectl get hpa -n donelist

# Adjust HPA min/max
kubectl patch hpa donelist-api -n donelist --patch '
spec:
  minReplicas: 3
  maxReplicas: 10
'
```

---

## Key Metrics Reference

### HTTP Metrics

```promql
# Request rate (requests per second)
rate(http_requests_total[5m])

# Error rate (percentage)
(sum(rate(http_requests_total{status=~"5.."}[5m])) /
 sum(rate(http_requests_total[5m]))) * 100

# P50 latency
histogram_quantile(0.50, rate(http_request_duration_seconds_bucket[5m]))

# P95 latency
histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m]))

# P99 latency
histogram_quantile(0.99, rate(http_request_duration_seconds_bucket[5m]))

# Requests in flight
sum(http_requests_in_flight)
```

### Database Metrics

```promql
# Active connections
db_connections_total

# Query duration P95
histogram_quantile(0.95, rate(db_operation_duration_seconds_bucket[5m]))

# Query error rate
rate(db_operation_errors_total[5m])
```

### Cache Metrics

```promql
# Cache hit rate
(sum(rate(cache_hits_total[5m])) /
 (sum(rate(cache_hits_total[5m])) + sum(rate(cache_misses_total[5m])))) * 100

# Cache operation latency
histogram_quantile(0.95, rate(cache_operation_duration_seconds_bucket[5m]))
```

### Business Metrics

```promql
# Check-ins per minute
rate(donelist_checkin_creations_total[1m]) * 60

# Daily active users
donelist_users_active_daily

# Webhook success rate
(sum(rate(donelist_webhook_deliveries_total{status="success"}[5m])) /
 sum(rate(donelist_webhook_deliveries_total[5m]))) * 100
```

### Resource Metrics

```promql
# Memory usage percentage
(container_memory_working_set_bytes /
 container_spec_memory_limit_bytes) * 100

# CPU usage percentage
rate(container_cpu_usage_seconds_total[5m]) * 100

# Pod restart count (last hour)
increase(kube_pod_container_status_restarts_total[1h])
```

---

## Alert Thresholds

| Alert | Warning | Critical | Action |
|-------|---------|----------|--------|
| Error Rate | 5% | 15% | Check logs, rollback if needed |
| Response Time (p95) | 1s | 2.5s | Check DB, scale up |
| Memory Usage | 85% | 95% | Increase limits, check leaks |
| CPU Usage | 85% | 95% | Scale up, check hot paths |
| DB Connections | 80% | 95% | Reset pool, increase limit |
| Pod Restarts | 1/15m | 3/15m | Check logs, rollback |
| Cache Hit Rate | < 70% | < 50% | Check Redis, warming |

---

## Troubleshooting Flowchart

```
Alert Received
     |
     v
Check Dashboard → No Data? → Check Prometheus scraping
     |
     v
Service Up? → No → Check pods, rollback if recent deploy
     |
     v
Recent Deploy? → Yes → Rollback deployment
     |
     v
High Error Rate? → Yes → Check logs, DB, cache
     |
     v
High Latency? → Yes → Check DB queries, cache hit rate
     |
     v
High Memory? → Yes → Check for leaks, restart pods
     |
     v
High CPU? → Yes → Profile code, scale up
     |
     v
DB Issues? → Yes → Check connections, slow queries
     |
     v
Still Issues? → Escalate to team lead
```

---

## Incident Response Checklist

### Initial Response (0-5 minutes)

- [ ] Acknowledge alert in Slack/PagerDuty
- [ ] Post initial status in #donelist-incidents
- [ ] Check Grafana dashboard for overview
- [ ] Verify alert is real (not false positive)
- [ ] Assess user impact

### Investigation (5-15 minutes)

- [ ] Check recent deployments
- [ ] Review error logs
- [ ] Check resource usage
- [ ] Test critical endpoints
- [ ] Identify root cause

### Mitigation (15-30 minutes)

- [ ] Apply fix (rollback, scale, config change)
- [ ] Monitor for improvement
- [ ] Verify metrics returning to normal
- [ ] Update incident channel

### Resolution (30+ minutes)

- [ ] Confirm incident resolved
- [ ] Update status page
- [ ] Post resolution summary
- [ ] Create post-mortem ticket
- [ ] Update runbooks if needed

---

## Escalation Contacts

| Time | Action | Contact |
|------|--------|---------|
| 0-15 min | On-call handles | @oncall-engineer |
| 15-30 min | Notify team lead | #donelist-oncall |
| 30-60 min | Engage specialists | @db-team or @infra-team |
| 60+ min | Escalate to management | @engineering-manager |

**Emergency**: Critical data loss or security breach → Escalate immediately

---

## Useful Links

- **Full Runbooks**: [MONITORING_RUNBOOKS.md](./MONITORING_RUNBOOKS.md)
- **Implementation Guide**: [MONITORING_IMPLEMENTATION_SUMMARY.md](./MONITORING_IMPLEMENTATION_SUMMARY.md)
- **Health Endpoints**: [HEALTH_ENDPOINTS.md](./HEALTH_ENDPOINTS.md)
- **Deployment Guide**: [../deploy/k8s/DEPLOYMENT.md](../deploy/k8s/DEPLOYMENT.md)

---

## Common Issues & Fixes

### Issue: Pod CrashLoopBackOff

```bash
# Check logs
kubectl logs -n donelist <pod-name> --previous

# Common causes:
# - Config error → Fix configmap
# - OOMKilled → Increase memory
# - Missing secret → Create secret
# - Liveness probe failing → Adjust probe timing
```

### Issue: High Error Rate

```bash
# Check what's failing
kubectl logs -n donelist deployment/donelist-api --tail=500 | \
  jq 'select(.statusCode >= 500)' | jq -r .path | sort | uniq -c

# If recent deploy → Rollback
kubectl rollout undo deployment/donelist-api -n donelist

# If database → Check connections
# If cache → Check Redis
# If external API → Check circuit breaker
```

### Issue: Slow Response Time

```bash
# Check database
kubectl exec -n donelist deployment/donelist-api -- \
  psql $DATABASE_URL -c "
    SELECT query, mean_exec_time
    FROM pg_stat_statements
    ORDER BY mean_exec_time DESC
    LIMIT 5;
  "

# Check cache hit rate
# Prometheus query: cache_hit_rate < 70%

# Scale up if needed
kubectl scale deployment donelist-api -n donelist --replicas=5
```

### Issue: Memory Leak

```bash
# Restart pod immediately
kubectl delete pod -n donelist <pod-name>

# Profile memory (Go app)
kubectl port-forward -n donelist <pod-name> 6060:6060
go tool pprof http://localhost:6060/debug/pprof/heap

# Increase memory limit temporarily
kubectl patch deployment donelist-api -n donelist --patch '
spec:
  template:
    spec:
      containers:
      - name: donelist-api
        resources:
          limits:
            memory: 2Gi
'
```

### Issue: Database Connection Exhaustion

```bash
# Check connection count
kubectl exec -n donelist deployment/donelist-api -- \
  psql $DATABASE_URL -c "
    SELECT count(*), state
    FROM pg_stat_activity
    GROUP BY state;
  "

# Restart API to reset pool
kubectl rollout restart deployment/donelist-api -n donelist

# If persistent, increase connection limit (requires DB restart)
```

---

## Testing Monitoring

### Generate Test Load

```bash
# Install hey (load testing)
go install github.com/rakyll/hey@latest

# Generate load
hey -n 10000 -c 100 -m GET https://api.donelist.example.com/api/v1/checkins

# Generate errors (invalid requests)
hey -n 1000 -c 50 -m POST https://api.donelist.example.com/api/v1/invalid
```

### Test Alerts

```bash
# Trigger high error rate
# (requires admin access or test endpoint)

# Trigger high memory
# (deploy memory-intensive version)

# Trigger pod crash
kubectl delete pod -n donelist <pod-name>
# Alert should fire within 5 minutes
```

### Verify Metrics

```bash
# Check metrics endpoint
curl -s http://localhost:8080/metrics | grep -E "http_requests|error|duration"

# Verify in Prometheus
# Query: http_requests_total
# Should see data points

# Verify in Grafana
# Open dashboard, should see graphs
```

---

## Tips & Tricks

### Quick Health Check
```bash
# One-liner health check
curl -s https://api.donelist.example.com/health | jq .
```

### Watch Logs Live
```bash
# Colorized log tail
kubectl logs -n donelist -l app=donelist -f | jq -C .
```

### Find Slow Requests
```bash
# From logs
kubectl logs -n donelist deployment/donelist-api --tail=1000 | \
  jq 'select(.duration > 1000) | {path, duration, method}'
```

### Check Recent Deploys
```bash
# Last 5 deployments
kubectl rollout history deployment/donelist-api -n donelist | tail -5
```

### Resource Usage Summary
```bash
# All pods resource usage
kubectl top pods -n donelist --sort-by=memory
kubectl top pods -n donelist --sort-by=cpu
```

---

**Quick Reference Version**: 1.0
**Last Updated**: 2024-01-15
**Print**: This document is designed to be printer-friendly (2 pages)
