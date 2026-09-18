# Donelist API - On-Call Runbook Index

## Overview

This directory contains operational runbooks for responding to alerts and incidents in the Donelist API production environment.

## Quick Reference

### Emergency Contacts

- **On-Call Engineer**: Check PagerDuty rotation
- **Engineering Lead**: [Contact Info]
- **DevOps Lead**: [Contact Info]
- **Database Admin**: [Contact Info]

### Key Links

- **Grafana Dashboards**: https://grafana.example.com/d/donelist
- **Prometheus Alerts**: https://prometheus.example.com/alerts
- **Kibana Logs**: https://kibana.example.com
- **Jaeger Traces**: https://jaeger.example.com
- **Kubernetes Dashboard**: https://k8s.example.com

### Service Status

- **Status Page**: https://status.donelist.io
- **Incident Management**: https://incidents.example.com

## Runbook Categories

### 1. High Error Rate Alerts

- [High 5xx Error Rate](./high-error-rate.md)
- [High 4xx Error Rate](./high-4xx-rate.md)
- [Critical Error Rate](./critical-error-rate.md)

### 2. Performance Degradation

- [High Response Time](./high-response-time.md)
- [Slow Database Queries](./slow-db-queries.md)
- [Cache Performance Issues](./cache-issues.md)

### 3. Infrastructure Issues

- [Pod Not Ready](./pod-not-ready.md)
- [Pod Crash Looping](./pod-crash-looping.md)
- [Low Replica Count](./low-replica-count.md)
- [Deployment Rollout Stuck](./deployment-stuck.md)

### 4. Resource Exhaustion

- [High Memory Usage](./high-memory-usage.md)
- [High CPU Usage](./high-cpu-usage.md)
- [Disk Space Issues](./disk-space.md)

### 5. Database & Cache

- [Database Connection Failure](./database-connection-failure.md)
- [Redis Connection Failure](./redis-connection-failure.md)
- [Database Performance Degradation](./db-performance.md)

### 6. Application Specific

- [Webhook Delivery Failures](./webhook-failures.md)
- [WebSocket Connection Issues](./websocket-issues.md)
- [Sync Conflicts](./sync-conflicts.md)

## General Incident Response Process

### 1. Acknowledge the Alert

- Check the alert in Slack or PagerDuty
- Acknowledge the incident
- Determine severity:
  - **P0/Critical**: Complete service outage, data loss
  - **P1/High**: Major functionality impaired
  - **P2/Medium**: Minor functionality degraded
  - **P3/Low**: No immediate user impact

### 2. Initial Assessment

```bash
# Check service health
kubectl get pods -n donelist
kubectl describe pod <pod-name> -n donelist
kubectl logs -n donelist <pod-name> --tail=100

# Check Grafana dashboards
# - API Overview Dashboard
# - Database Performance Dashboard
# - SLO Dashboard

# Check recent deployments
kubectl rollout history deployment/donelist-api -n donelist
```

### 3. Triage and Diagnosis

- Review metrics in Grafana
- Check logs in Kibana
- Examine traces in Jaeger
- Review recent changes (git, deployments)
- Check external dependencies (DNS, external APIs)

### 4. Mitigation

Follow the specific runbook for the alert type. Common mitigations:

- **Rollback**: If caused by recent deployment
- **Scale**: If resource constrained
- **Restart**: If transient issue
- **Traffic Control**: If external load spike

### 5. Communication

- Update status page
- Post in #incidents Slack channel
- Notify stakeholders for P0/P1 incidents
- Document actions taken

### 6. Resolution

- Verify metrics return to normal
- Run smoke tests
- Update incident ticket
- Resolve alert in PagerDuty

### 7. Post-Incident

- Write post-mortem (for P0/P1)
- Create Jira tickets for follow-up actions
- Update runbooks if needed
- Schedule blameless retrospective

## Common Commands

### Kubernetes Operations

```bash
# Get pod status
kubectl get pods -n donelist

# View pod logs
kubectl logs -n donelist deployment/donelist-api --tail=100 -f

# Describe pod
kubectl describe pod <pod-name> -n donelist

# Execute command in pod
kubectl exec -it <pod-name> -n donelist -- /bin/sh

# Port forward to pod
kubectl port-forward -n donelist <pod-name> 8080:8080

# Check resource usage
kubectl top pods -n donelist
kubectl top nodes

# View events
kubectl get events -n donelist --sort-by='.lastTimestamp'

# Rollback deployment
kubectl rollout undo deployment/donelist-api -n donelist

# Scale deployment
kubectl scale deployment/donelist-api -n donelist --replicas=5

# Restart deployment
kubectl rollout restart deployment/donelist-api -n donelist
```

### Database Operations

```bash
# Check PostgreSQL connection
kubectl exec -it <postgres-pod> -n donelist -- psql -U donelist -d donelist

# View active queries
SELECT pid, age(query_start, clock_timestamp()), usename, query
FROM pg_stat_activity
WHERE query != '<IDLE>' AND query NOT ILIKE '%pg_stat_activity%'
ORDER BY query_start;

# Kill long-running query
SELECT pg_terminate_backend(<pid>);

# Check database size
SELECT pg_database_size('donelist');

# View slow query log
kubectl logs -n donelist <postgres-pod> | grep "duration:"
```

### Redis Operations

```bash
# Connect to Redis
kubectl exec -it <redis-pod> -n donelist -- redis-cli

# Check memory usage
INFO memory

# View slow log
SLOWLOG GET 10

# Monitor commands in real-time
MONITOR

# Check connected clients
CLIENT LIST

# Flush cache (use with caution!)
# FLUSHALL
```

### Metrics & Monitoring

```bash
# Query Prometheus
curl -G 'http://prometheus:9090/api/v1/query' \
  --data-urlencode 'query=up{job="donelist-api-service"}'

# Check metrics endpoint
kubectl port-forward -n donelist svc/donelist-api-service 8080:8080
curl http://localhost:8080/metrics

# View alert status
kubectl port-forward -n donelist svc/prometheus 9090:9090
# Browse to http://localhost:9090/alerts
```

## Escalation Paths

### Level 1: On-Call Engineer
- First responder
- Follow runbooks
- Escalate if needed within 15 minutes

### Level 2: Engineering Lead
- Complex technical issues
- Architecture decisions
- Coordinate with other teams

### Level 3: VP Engineering / CTO
- Major incidents (P0)
- Customer-facing communication
- Executive decisions

## Service Level Objectives (SLOs)

### API Availability
- **Target**: 99.9% uptime (43.2 minutes downtime per month)
- **Measurement**: Non-5xx responses / total responses

### Response Time
- **Target**: p95 < 500ms
- **Measurement**: 95th percentile of request duration

### Error Rate
- **Target**: < 0.5% errors
- **Measurement**: 5xx responses / total responses

## Monitoring Tools

### Grafana Dashboards
- **API Overview**: Request rates, latency, errors
- **Database Performance**: Query performance, connections
- **Business Metrics**: User activity, feature usage
- **SLO Dashboard**: SLI tracking and error budgets

### Log Aggregation
- **Fluent Bit**: Log collection from pods
- **Loki/Elasticsearch**: Log storage and search
- **Kibana**: Log visualization and analysis

### Distributed Tracing
- **OpenTelemetry**: Trace collection
- **Jaeger**: Trace visualization
- **Tempo**: Long-term trace storage

### Alerting
- **Prometheus**: Metric-based alerts
- **Alertmanager**: Alert routing and deduplication
- **PagerDuty**: On-call rotation and escalation

## Reference Documents

- [Architecture Overview](../ARCHITECTURE.md)
- [Deployment Guide](../deploy/k8s/DEPLOYMENT.md)
- [API Documentation](../api/README.md)
- [Database Schema](../../migrations/)

## Maintenance Windows

- **Preferred Time**: Sunday 02:00-06:00 UTC
- **Change Freeze**: Black Friday, Cyber Monday, Major Holidays
- **Communication**: Announce 7 days in advance on status page

## Recent Incidents

See [Incident Log](./INCIDENT_LOG.md) for historical incident data and trends.
