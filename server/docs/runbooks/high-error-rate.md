# Runbook: High Error Rate

## Alert Details

- **Alert Name**: `HighErrorRate` / `CriticalErrorRate`
- **Severity**: Warning (> 5%), Critical (> 15%)
- **SLO Impact**: High - directly affects availability SLO

## Symptoms

- Increased 5xx error responses
- User-facing errors
- Degraded service functionality
- Error budget consumption

## Impact

- **User Impact**: Users experiencing errors, unable to use features
- **Business Impact**: Potential revenue loss, user churn
- **SLO Impact**: Burns error budget, may violate 99.9% uptime SLO

## Diagnosis

### 1. Check Current Error Rate

```bash
# Get current error rate
kubectl port-forward -n donelist svc/prometheus 9090:9090 &
curl -G 'http://localhost:9090/api/v1/query' \
  --data-urlencode 'query=(sum(rate(http_requests_total{job="donelist-api-service",status=~"5.."}[5m])) / sum(rate(http_requests_total{job="donelist-api-service"}[5m]))) * 100'
```

### 2. Identify Error Types

Open Grafana API Overview Dashboard and check:
- Which endpoints are returning errors?
- What HTTP status codes? (500, 502, 503, 504)
- When did the error rate start increasing?

```bash
# Check error breakdown by endpoint
curl -G 'http://localhost:9090/api/v1/query' \
  --data-urlencode 'query=topk(10, sum(rate(http_requests_total{status=~"5..",job="donelist-api-service"}[5m])) by (path, status))'
```

### 3. Review Application Logs

```bash
# Get recent error logs
kubectl logs -n donelist deployment/donelist-api --tail=200 | grep -i error

# Filter for specific error patterns
kubectl logs -n donelist deployment/donelist-api --tail=500 | grep -E "(panic|fatal|error)" | tail -50

# Check for specific error types
kubectl logs -n donelist deployment/donelist-api --tail=500 | grep -i "database\|connection\|timeout"
```

### 4. Check Recent Deployments

```bash
# Check deployment history
kubectl rollout history deployment/donelist-api -n donelist

# Compare current with previous
kubectl rollout history deployment/donelist-api -n donelist --revision=<current>
kubectl rollout history deployment/donelist-api -n donelist --revision=<previous>
```

### 5. Check External Dependencies

```bash
# Check database connectivity
kubectl exec -it <pod-name> -n donelist -- wget -O- http://localhost:8080/health/detail

# Check Redis connectivity
kubectl exec -it <redis-pod> -n donelist -- redis-cli ping

# Check PostgreSQL status
kubectl exec -it <postgres-pod> -n donelist -- psql -U donelist -c "SELECT 1;"
```

## Common Causes and Solutions

### 1. Recent Deployment Issue

**Symptoms**: Errors started immediately after deployment

**Solution**: Rollback the deployment

```bash
# Rollback to previous version
kubectl rollout undo deployment/donelist-api -n donelist

# Monitor error rate after rollback
watch -n 5 'kubectl logs -n donelist deployment/donelist-api --tail=20 | grep -c ERROR'

# Verify in Grafana that error rate decreases
```

**Follow-up**:
- Review changes in the failed deployment
- Test in staging environment
- Create bug ticket

### 2. Database Connection Issues

**Symptoms**: Logs show database connection errors, timeouts

**Solution**: Check database health and connections

```bash
# Check database pod status
kubectl get pods -n donelist | grep postgres

# Check database connections
kubectl exec -it <postgres-pod> -n donelist -- psql -U donelist -c "SELECT count(*) FROM pg_stat_activity;"

# Check for blocked queries
kubectl exec -it <postgres-pod> -n donelist -- psql -U donelist -c "
SELECT pid, usename, application_name, state, query
FROM pg_stat_activity
WHERE state = 'active' AND wait_event IS NOT NULL
ORDER BY query_start;
"

# Restart database connections (if safe)
kubectl rollout restart deployment/donelist-api -n donelist
```

### 3. Memory Exhaustion / OOM Kills

**Symptoms**: Pods restarting, OOMKilled in pod status

**Solution**: Scale up or increase memory limits

```bash
# Check pod events for OOM
kubectl describe pod <pod-name> -n donelist | grep -A 10 "Last State"

# Check memory usage
kubectl top pods -n donelist

# Immediate mitigation: Scale up
kubectl scale deployment/donelist-api -n donelist --replicas=6

# Long-term fix: Update memory limits
kubectl edit deployment/donelist-api -n donelist
# Increase resources.limits.memory
```

### 4. External API Failures

**Symptoms**: Errors when calling external services, timeout errors

**Solution**: Enable circuit breaker, add fallbacks

```bash
# Check external API health
curl -I https://external-api.example.com/health

# Review trace data for slow external calls
# Open Jaeger and filter for slow traces

# If temporary: Wait for external service recovery
# If persistent: Disable feature flag or enable fallback
```

### 5. Unexpected Traffic Spike

**Symptoms**: Increased request rate, timeout errors under load

**Solution**: Scale horizontally

```bash
# Check request rate
curl -G 'http://localhost:9090/api/v1/query' \
  --data-urlencode 'query=sum(rate(http_requests_total{job="donelist-api-service"}[5m]))'

# Scale up immediately
kubectl scale deployment/donelist-api -n donelist --replicas=10

# Enable HPA if not already enabled
kubectl autoscale deployment/donelist-api -n donelist --min=3 --max=20 --cpu-percent=70

# Check if scaling helps
kubectl get hpa -n donelist -w
```

### 6. Database Query Performance

**Symptoms**: Slow database queries, query timeouts

**Solution**: Identify and optimize slow queries

```bash
# Check slow queries
kubectl exec -it <postgres-pod> -n donelist -- psql -U donelist -c "
SELECT
  query,
  calls,
  total_exec_time,
  mean_exec_time,
  max_exec_time
FROM pg_stat_statements
ORDER BY mean_exec_time DESC
LIMIT 10;
"

# Check for missing indexes
kubectl exec -it <postgres-pod> -n donelist -- psql -U donelist -c "
SELECT
  schemaname,
  tablename,
  seq_scan,
  seq_tup_read,
  idx_scan
FROM pg_stat_user_tables
WHERE seq_scan > idx_scan
ORDER BY seq_scan DESC
LIMIT 10;
"

# Consider adding read replicas or caching
```

## Mitigation Checklist

- [ ] Acknowledge alert
- [ ] Check Grafana for error patterns
- [ ] Review recent deployments
- [ ] Check application logs
- [ ] Verify database connectivity
- [ ] Verify Redis connectivity
- [ ] Check external dependencies
- [ ] Apply appropriate solution (rollback/scale/restart)
- [ ] Verify error rate decreases
- [ ] Update incident ticket
- [ ] Notify stakeholders

## Communication Template

### Initial Notification (Slack #incidents)

```
🚨 **Incident**: High Error Rate Detected
**Status**: Investigating
**Impact**: Users may experience errors when [affected functionality]
**Started**: <timestamp>
**On-Call**: @engineer

Current actions:
- Checking recent deployments
- Reviewing error logs
- Monitoring metrics

Will update in 15 minutes.
```

### Update Notification

```
📊 **Incident Update**: High Error Rate
**Status**: Mitigating / Resolved
**Root Cause**: [Brief description]
**Action Taken**: [What was done]

Error rate has decreased from X% to Y%.
Continuing to monitor.
```

### Resolution Notification

```
✅ **Incident Resolved**: High Error Rate
**Duration**: X minutes
**Root Cause**: [Description]
**Resolution**: [What fixed it]

Post-mortem will be conducted on [date].
Follow-up tickets: [links]
```

## Prevention

1. **Monitoring**: Ensure comprehensive error tracking
2. **Testing**: Increase test coverage, add load tests
3. **Gradual Rollouts**: Use canary deployments
4. **Circuit Breakers**: Add circuit breakers for external calls
5. **Rate Limiting**: Implement rate limiting for APIs
6. **Capacity Planning**: Regular load testing and capacity reviews

## Related Runbooks

- [Critical Error Rate](./critical-error-rate.md)
- [High Response Time](./high-response-time.md)
- [Database Connection Failure](./database-connection-failure.md)
- [Deployment Rollout Stuck](./deployment-stuck.md)

## Post-Incident Actions

- [ ] Write post-mortem (if P0/P1)
- [ ] Create follow-up tickets
- [ ] Update monitoring/alerting if needed
- [ ] Update this runbook with learnings
- [ ] Share findings with team

## References

- [Grafana API Overview Dashboard](https://grafana.example.com/d/donelist-api)
- [Error Budget Dashboard](https://grafana.example.com/d/donelist-slo)
- [Deployment Guide](../deploy/k8s/DEPLOYMENT.md)
