# Database Monitoring and Operations Runbook

This document provides operational procedures, monitoring setup, and incident response for PostgreSQL database management in the Donelist backend.

## Overview

Task #18.10 covers:
1. Performance monitoring setup
2. Replication monitoring
3. Backup verification
4. Alerting rules configuration
5. Incident response procedures

## 1. Monitoring Stack

### Components

```
┌─────────────────┐
│   PostgreSQL    │
│   + Extensions  │
└────────┬────────┘
         │ Metrics Export
         ▼
┌─────────────────┐
│ postgres_export │
│   (Port 9187)   │
└────────┬────────┘
         │ Scrape
         ▼
┌─────────────────┐
│   Prometheus    │
│  (Port 9090)    │
└────────┬────────┘
         │ Query
         ▼
┌─────────────────┐     ┌─────────────────┐
│    Grafana      │────▶│  Alertmanager   │
│  (Port 3000)    │     │   (Port 9093)   │
└─────────────────┘     └────────┬────────┘
                                 │ Notify
                                 ▼
                        ┌─────────────────┐
                        │  PagerDuty/Slack│
                        └─────────────────┘
```

### Docker Compose Setup

```yaml
# docker-compose.monitoring.yml
version: '3.8'

services:
  postgres-exporter:
    image: prometheuscommunity/postgres-exporter:latest
    environment:
      DATA_SOURCE_NAME: "postgresql://monitoring_user:${MONITOR_PASSWORD}@postgres:5432/donelist?sslmode=disable"
    ports:
      - "9187:9187"
    depends_on:
      - postgres

  prometheus:
    image: prom/prometheus:latest
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
      - ./alert_rules.yml:/etc/prometheus/alert_rules.yml
      - prometheus_data:/prometheus
    ports:
      - "9090:9090"
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.retention.time=30d'

  grafana:
    image: grafana/grafana:latest
    environment:
      GF_SECURITY_ADMIN_PASSWORD: ${GRAFANA_PASSWORD}
      GF_INSTALL_PLUGINS: grafana-piechart-panel
    volumes:
      - grafana_data:/var/lib/grafana
      - ./grafana/dashboards:/etc/grafana/provisioning/dashboards
      - ./grafana/datasources:/etc/grafana/provisioning/datasources
    ports:
      - "3000:3000"
    depends_on:
      - prometheus

  alertmanager:
    image: prom/alertmanager:latest
    volumes:
      - ./alertmanager.yml:/etc/alertmanager/alertmanager.yml
      - alertmanager_data:/alertmanager
    ports:
      - "9093:9093"
    command:
      - '--config.file=/etc/alertmanager/alertmanager.yml'

volumes:
  prometheus_data:
  grafana_data:
  alertmanager_data:
```

## 2. Monitoring Configuration

### Create Monitoring User

```sql
-- Create read-only monitoring user
CREATE USER monitoring_user WITH PASSWORD 'strong_monitoring_password';

-- Grant necessary permissions
GRANT CONNECT ON DATABASE donelist TO monitoring_user;
GRANT pg_monitor TO monitoring_user;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO monitoring_user;
GRANT SELECT ON ALL TABLES IN SCHEMA performance TO monitoring_user;

-- For pg_stat_statements
GRANT SELECT ON pg_stat_statements TO monitoring_user;
```

### Enable Required Extensions

```sql
-- Performance monitoring
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

-- Connection tracking
ALTER SYSTEM SET track_activities = on;
ALTER SYSTEM SET track_counts = on;
ALTER SYSTEM SET track_io_timing = on;
ALTER SYSTEM SET track_functions = 'all';

-- pg_stat_statements settings
ALTER SYSTEM SET pg_stat_statements.max = 10000;
ALTER SYSTEM SET pg_stat_statements.track = 'all';

SELECT pg_reload_conf();
```

### Prometheus Configuration

```yaml
# prometheus.yml
global:
  scrape_interval: 15s
  evaluation_interval: 15s
  external_labels:
    cluster: 'donelist-prod'
    environment: 'production'

rule_files:
  - 'alert_rules.yml'

alerting:
  alertmanagers:
    - static_configs:
        - targets: ['alertmanager:9093']

scrape_configs:
  - job_name: 'postgres-primary'
    static_configs:
      - targets: ['postgres-exporter:9187']
        labels:
          role: 'primary'
          instance: 'postgres-primary'

  - job_name: 'postgres-replica'
    static_configs:
      - targets: ['postgres-replica-exporter:9187']
        labels:
          role: 'replica'
          instance: 'postgres-replica-1'

  - job_name: 'application'
    static_configs:
      - targets: ['app:8080']
        labels:
          service: 'donelist-api'
```

## 3. Key Metrics and Thresholds

### Database Performance

| Metric | Threshold | Severity | Action |
|--------|-----------|----------|--------|
| Query p95 latency | > 200ms | Warning | Review slow queries |
| Query p99 latency | > 500ms | Critical | Immediate investigation |
| QPS (queries/sec) | > 10000 | Info | Scale consideration |
| Active connections | > 80% max | Warning | Investigate connections |
| Connection wait count | > 10/sec | Critical | Add connections |

### Replication

| Metric | Threshold | Severity | Action |
|--------|-----------|----------|--------|
| Replication lag | > 10s | Warning | Check network/load |
| Replication lag | > 60s | Critical | Immediate action |
| Replica down | N/A | Critical | Failover consideration |
| WAL files pending | > 100 | Warning | Check replica health |

### Storage

| Metric | Threshold | Severity | Action |
|--------|-----------|----------|--------|
| Disk usage | > 80% | Warning | Plan expansion |
| Disk usage | > 90% | Critical | Urgent expansion |
| WAL disk usage | > 75% | Warning | Archive/cleanup |
| Table bloat | > 40% | Info | Schedule VACUUM |
| Index bloat | > 30% | Info | Consider REINDEX |

### Backup and Recovery

| Metric | Threshold | Severity | Action |
|--------|-----------|----------|--------|
| Last backup age | > 25 hours | Warning | Check backup job |
| Last backup age | > 48 hours | Critical | Urgent backup |
| Backup size change | > 50% | Info | Investigate growth |
| Restore test age | > 7 days | Warning | Test restore |

## 4. Alert Rules

### alert_rules.yml

```yaml
groups:
  - name: postgresql_performance
    interval: 30s
    rules:
      # Query Performance
      - alert: SlowQueriesHigh
        expr: |
          rate(pg_stat_statements_mean_exec_time_seconds[5m]) > 0.5
        for: 5m
        labels:
          severity: warning
          component: database
        annotations:
          summary: "High number of slow queries"
          description: "Average query time is {{ $value }}s on {{ $labels.instance }}"

      - alert: DatabaseDown
        expr: pg_up == 0
        for: 1m
        labels:
          severity: critical
          component: database
        annotations:
          summary: "PostgreSQL is down"
          description: "PostgreSQL instance {{ $labels.instance }} is down"

      # Connection Pooling
      - alert: HighConnectionUsage
        expr: |
          (pg_stat_database_numbackends / pg_settings_max_connections) > 0.8
        for: 5m
        labels:
          severity: warning
          component: database
        annotations:
          summary: "Connection pool usage high"
          description: "{{ $value | humanizePercentage }} of connections in use on {{ $labels.instance }}"

      - alert: ConnectionExhaustion
        expr: |
          pg_stat_database_numbackends >= pg_settings_max_connections * 0.95
        for: 1m
        labels:
          severity: critical
          component: database
        annotations:
          summary: "Connection exhaustion imminent"
          description: "{{ $value }} connections active, nearing max_connections limit"

      # Replication
      - alert: ReplicationLagHigh
        expr: |
          pg_replication_lag_seconds > 10
        for: 5m
        labels:
          severity: warning
          component: replication
        annotations:
          summary: "Replication lag is high"
          description: "Replica {{ $labels.application_name }} is {{ $value }}s behind primary"

      - alert: ReplicationLagCritical
        expr: |
          pg_replication_lag_seconds > 60
        for: 2m
        labels:
          severity: critical
          component: replication
        annotations:
          summary: "Replication lag is critical"
          description: "Replica {{ $labels.application_name }} is {{ $value }}s behind primary. Data may be stale."

      - alert: ReplicaDown
        expr: |
          up{job="postgres-replica"} == 0
        for: 1m
        labels:
          severity: critical
          component: replication
        annotations:
          summary: "Database replica is down"
          description: "Replica {{ $labels.instance }} is unreachable"

      # Storage
      - alert: DiskSpaceWarning
        expr: |
          (node_filesystem_avail_bytes{mountpoint="/var/lib/postgresql"} / node_filesystem_size_bytes{mountpoint="/var/lib/postgresql"}) < 0.2
        for: 5m
        labels:
          severity: warning
          component: storage
        annotations:
          summary: "Low disk space on database server"
          description: "Only {{ $value | humanizePercentage }} disk space remaining"

      - alert: DiskSpaceCritical
        expr: |
          (node_filesystem_avail_bytes{mountpoint="/var/lib/postgresql"} / node_filesystem_size_bytes{mountpoint="/var/lib/postgresql"}) < 0.1
        for: 1m
        labels:
          severity: critical
          component: storage
        annotations:
          summary: "Critical disk space on database server"
          description: "Only {{ $value | humanizePercentage }} disk space remaining. Immediate action required."

      # Backup
      - alert: BackupOverdue
        expr: |
          time() - pg_last_backup_timestamp > 86400 * 1.5
        for: 30m
        labels:
          severity: warning
          component: backup
        annotations:
          summary: "Database backup is overdue"
          description: "Last backup was {{ $value | humanizeDuration }} ago"

      - alert: BackupFailure
        expr: |
          time() - pg_last_successful_backup_timestamp > 172800
        for: 1h
        labels:
          severity: critical
          component: backup
        annotations:
          summary: "Database backup has failed"
          description: "No successful backup in 48+ hours"

      # Table Bloat
      - alert: TableBloatHigh
        expr: |
          pg_stat_user_tables_bloat_percent > 40
        for: 1h
        labels:
          severity: info
          component: maintenance
        annotations:
          summary: "High table bloat detected"
          description: "Table {{ $labels.relname }} has {{ $value }}% bloat. Consider VACUUM FULL."

      # Deadlocks
      - alert: DeadlocksDetected
        expr: |
          rate(pg_stat_database_deadlocks[5m]) > 0
        for: 5m
        labels:
          severity: warning
          component: database
        annotations:
          summary: "Deadlocks detected"
          description: "{{ $value }} deadlocks per second on database {{ $labels.datname }}"
```

### Alertmanager Configuration

```yaml
# alertmanager.yml
global:
  resolve_timeout: 5m
  slack_api_url: 'https://hooks.slack.com/services/YOUR/WEBHOOK/URL'

route:
  group_by: ['alertname', 'cluster', 'service']
  group_wait: 10s
  group_interval: 10s
  repeat_interval: 12h
  receiver: 'default'
  routes:
    - match:
        severity: critical
      receiver: 'pagerduty-critical'
      continue: true

    - match:
        severity: warning
      receiver: 'slack-warnings'

    - match:
        component: backup
      receiver: 'backup-team'

receivers:
  - name: 'default'
    slack_configs:
      - channel: '#database-alerts'
        title: '{{ .GroupLabels.alertname }}'
        text: '{{ range .Alerts }}{{ .Annotations.description }}{{ end }}'

  - name: 'pagerduty-critical'
    pagerduty_configs:
      - service_key: 'YOUR_PAGERDUTY_SERVICE_KEY'
        description: '{{ .GroupLabels.alertname }}: {{ .CommonAnnotations.summary }}'

  - name: 'slack-warnings'
    slack_configs:
      - channel: '#database-warnings'
        title: 'Warning: {{ .GroupLabels.alertname }}'
        text: '{{ range .Alerts }}{{ .Annotations.description }}{{ end }}'

  - name: 'backup-team'
    email_configs:
      - to: 'backup-team@example.com'
        subject: 'Backup Alert: {{ .GroupLabels.alertname }}'
```

## 5. Grafana Dashboards

### PostgreSQL Overview Dashboard

```json
{
  "dashboard": {
    "title": "PostgreSQL Overview",
    "panels": [
      {
        "title": "Queries Per Second",
        "targets": [
          {
            "expr": "rate(pg_stat_database_xact_commit[5m]) + rate(pg_stat_database_xact_rollback[5m])"
          }
        ]
      },
      {
        "title": "Query Latency (p95)",
        "targets": [
          {
            "expr": "histogram_quantile(0.95, rate(pg_stat_statements_total_time_bucket[5m]))"
          }
        ]
      },
      {
        "title": "Active Connections",
        "targets": [
          {
            "expr": "pg_stat_database_numbackends"
          }
        ]
      },
      {
        "title": "Cache Hit Ratio",
        "targets": [
          {
            "expr": "(pg_stat_database_blks_hit / (pg_stat_database_blks_hit + pg_stat_database_blks_read)) * 100"
          }
        ]
      },
      {
        "title": "Replication Lag",
        "targets": [
          {
            "expr": "pg_replication_lag_seconds"
          }
        ]
      },
      {
        "title": "Table Sizes",
        "targets": [
          {
            "expr": "pg_stat_user_tables_table_size_bytes"
          }
        ]
      }
    ]
  }
}
```

### Import Pre-built Dashboard

```bash
# PostgreSQL Dashboard ID: 9628
# Import in Grafana UI: Dashboards > Import > 9628
```

## 6. Operational Runbooks

### Runbook 1: High Replication Lag

**Symptoms:**
- Alert: `ReplicationLagHigh` or `ReplicationLagCritical`
- Users report stale data on read replicas

**Diagnosis:**
```sql
-- Check replication status on PRIMARY
SELECT
    application_name,
    client_addr,
    state,
    replay_lag,
    sync_state,
    replay_lsn,
    sent_lsn,
    pg_wal_lsn_diff(sent_lsn, replay_lsn) as bytes_behind
FROM pg_stat_replication;

-- Check for long-running queries on REPLICA
SELECT pid, usename, state, query_start, query
FROM pg_stat_activity
WHERE state = 'active' AND pid <> pg_backend_pid()
ORDER BY query_start;

-- Check WAL sender on PRIMARY
SELECT * FROM pg_stat_wal_receiver;
```

**Resolution:**
1. **If caused by long-running query on replica:**
   ```sql
   SELECT pg_terminate_backend(pid);
   ```

2. **If caused by high write load on primary:**
   - Scale vertically (more CPU/RAM on primary)
   - Enable synchronous_commit = off (trade durability for performance)
   - Add more replicas to distribute load

3. **If caused by network issues:**
   - Check network connectivity between primary and replica
   - Verify wal_keep_size is sufficient
   - Check for network saturation

4. **Temporary mitigation:**
   ```sql
   -- Route reads to primary until lag recovers
   -- In application: use primary for all queries temporarily
   ```

### Runbook 2: Connection Exhaustion

**Symptoms:**
- Alert: `HighConnectionUsage` or `ConnectionExhaustion`
- Application errors: "too many connections"

**Diagnosis:**
```sql
-- Check current connections
SELECT
    datname,
    usename,
    application_name,
    state,
    COUNT(*)
FROM pg_stat_activity
GROUP BY datname, usename, application_name, state
ORDER BY count DESC;

-- Find idle connections
SELECT COUNT(*) as idle_connections
FROM pg_stat_activity
WHERE state = 'idle' AND query_start < NOW() - INTERVAL '5 minutes';

-- Check max connections
SHOW max_connections;

-- Check connection pool settings
SELECT * FROM pg_stat_database;
```

**Resolution:**
1. **Kill idle connections:**
   ```sql
   SELECT pg_terminate_backend(pid)
   FROM pg_stat_activity
   WHERE state = 'idle'
     AND query_start < NOW() - INTERVAL '30 minutes'
     AND pid <> pg_backend_pid();
   ```

2. **Increase max_connections (requires restart):**
   ```sql
   ALTER SYSTEM SET max_connections = 200;
   -- Restart PostgreSQL
   ```

3. **Deploy pgBouncer:**
   - See CONNECTION_POOLING.md
   - Reduces connection overhead significantly

4. **Application fixes:**
   - Implement connection pooling in application
   - Fix connection leaks
   - Add connection timeouts

### Runbook 3: Slow Queries

**Symptoms:**
- Alert: `SlowQueriesHigh`
- Users report slow application responses

**Diagnosis:**
```sql
-- Top 10 slowest queries
SELECT
    substring(query, 1, 80) as query_short,
    calls,
    mean_exec_time,
    max_exec_time,
    total_exec_time,
    (total_exec_time / sum(total_exec_time) OVER ()) * 100 as pct_total_time
FROM pg_stat_statements
ORDER BY mean_exec_time DESC
LIMIT 10;

-- Active slow queries
SELECT
    pid,
    usename,
    datname,
    state,
    NOW() - query_start as duration,
    query
FROM pg_stat_activity
WHERE state = 'active'
  AND NOW() - query_start > INTERVAL '5 seconds'
ORDER BY duration DESC;

-- Check for missing indexes
SELECT
    schemaname,
    tablename,
    indexname,
    idx_scan,
    idx_tup_read,
    idx_tup_fetch
FROM pg_stat_user_indexes
WHERE idx_scan = 0
ORDER BY pg_relation_size(indexrelid) DESC;
```

**Resolution:**
1. **Kill problematic query:**
   ```sql
   SELECT pg_cancel_backend(pid);  -- Graceful
   SELECT pg_terminate_backend(pid);  -- Force kill
   ```

2. **Add missing indexes:**
   ```sql
   -- Analyze query plan
   EXPLAIN (ANALYZE, BUFFERS) <your_slow_query>;

   -- Create index
   CREATE INDEX CONCURRENTLY idx_name ON table(column);
   ```

3. **Update statistics:**
   ```sql
   ANALYZE table_name;
   ```

4. **Rewrite query:**
   - See QUERY_OPTIMIZATION.md
   - Use appropriate JOINs
   - Add WHERE clauses early
   - Avoid SELECT *

### Runbook 4: Disk Space Critical

**Symptoms:**
- Alert: `DiskSpaceCritical`
- Database writes failing

**Diagnosis:**
```bash
# Check disk usage
df -h /var/lib/postgresql

# Check largest tables
psql -c "
SELECT
    schemaname || '.' || tablename as table,
    pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) as size
FROM pg_tables
WHERE schemaname NOT IN ('pg_catalog', 'information_schema')
ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC
LIMIT 10;
"

# Check WAL files
ls -lh /var/lib/postgresql/14/main/pg_wal/ | wc -l
du -sh /var/lib/postgresql/14/main/pg_wal/
```

**Resolution:**
1. **Immediate actions (free space quickly):**
   ```sql
   -- Drop old partitions (be careful!)
   SELECT * FROM partitions.cleanup_old_partitions(24);

   -- Clean up temp files
   -- As postgres user
   find /var/lib/postgresql -name "pgsql_tmp*" -delete
   ```

2. **Archive old data:**
   ```sql
   SELECT * FROM partitions.auto_archive_old_partitions(12);
   ```

3. **VACUUM FULL (reclaim space):**
   ```sql
   -- Do this during maintenance window
   VACUUM FULL VERBOSE table_name;
   ```

4. **Expand storage:**
   ```bash
   # AWS EBS volume expansion
   aws ec2 modify-volume --volume-id vol-xxx --size 500

   # Resize filesystem
   sudo resize2fs /dev/xvdf
   ```

### Runbook 5: Backup Failure

**Symptoms:**
- Alert: `BackupOverdue` or `BackupFailure`
- No recent backup files

**Diagnosis:**
```bash
# Check last backup
ls -lht /var/lib/postgresql/backups/ | head

# Check backup logs
tail -n 100 /var/log/postgresql/backup.log

# Test backup connectivity
pg_basebackup --help
```

**Resolution:**
1. **Manual backup:**
   ```bash
   pg_basebackup \
     -h localhost \
     -U postgres \
     -D /var/lib/postgresql/backups/manual_$(date +%Y%m%d_%H%M%S) \
     -F tar \
     -z \
     -P \
     -v
   ```

2. **Fix automated backup:**
   ```bash
   # Check cron job
   crontab -l

   # Test backup script
   /usr/local/bin/backup-postgres.sh --test
   ```

3. **Upload to remote storage:**
   ```bash
   aws s3 cp /var/lib/postgresql/backups/latest.tar.gz \
     s3://donelist-backups/$(date +%Y%m%d)/
   ```

## 7. Maintenance Procedures

### Weekly Maintenance (Low Traffic)

```sql
-- Update statistics
ANALYZE VERBOSE;

-- Check for bloat
SELECT
    schemaname,
    tablename,
    pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) as size,
    n_dead_tup,
    n_live_tup,
    ROUND(n_dead_tup * 100.0 / NULLIF(n_live_tup + n_dead_tup, 0), 2) as dead_ratio
FROM pg_stat_user_tables
WHERE n_dead_tup > 1000
ORDER BY n_dead_tup DESC;

-- VACUUM if needed
VACUUM ANALYZE;
```

### Monthly Maintenance

```sql
-- Archive old partitions
SELECT * FROM partitions.auto_archive_old_partitions(12);

-- Ensure future partitions exist
SELECT * FROM partitions.ensure_future_partitions(3);

-- Refresh materialized views
SELECT * FROM refresh_analytics_materialized_views();

-- Check index health
SELECT
    indexname,
    pg_size_pretty(pg_relation_size(indexname::regclass)) as size,
    idx_scan,
    idx_tup_read,
    idx_tup_fetch
FROM pg_stat_user_indexes
WHERE idx_scan = 0 AND pg_relation_size(indexname::regclass) > 1048576
ORDER BY pg_relation_size(indexname::regclass) DESC;
```

### Quarterly Maintenance

- Test database restore procedure
- Review and update alert thresholds
- Review slow query log
- Capacity planning review
- Disaster recovery drill

## 8. On-Call Checklist

### When You Get Paged

1. **Acknowledge** the alert in PagerDuty/Slack
2. **Assess severity** - Is this user-impacting?
3. **Check status page** - Are users affected?
4. **Review metrics** - Grafana dashboards
5. **Check recent changes** - Deployments, migrations
6. **Follow runbook** - Use procedures above
7. **Document actions** - Post-incident report
8. **Escalate if needed** - Don't hesitate to escalate

### First Responder Actions

```bash
# Quick health check
psql -c "SELECT version(); SELECT pg_is_in_recovery(); SELECT COUNT(*) FROM pg_stat_activity;"

# Check alerts
curl http://prometheus:9090/api/v1/alerts | jq .

# Check system resources
top
df -h
free -h
iostat -x 1 5
```

## 9. Contact Information

| Role | Contact | Escalation |
|------|---------|------------|
| Primary On-Call | #oncall-db | PagerDuty: @db-team |
| Database Lead | db-lead@example.com | Phone: +1-xxx-xxx-xxxx |
| Infrastructure | #infra-team | PagerDuty: @infra |
| Engineering Manager | manager@example.com | Phone: +1-xxx-xxx-xxxx |

## 10. References

- [PostgreSQL Monitoring](https://www.postgresql.org/docs/current/monitoring.html)
- [postgres_exporter](https://github.com/prometheus-community/postgres_exporter)
- [Grafana PostgreSQL Plugin](https://grafana.com/grafana/plugins/grafana-postgresql-datasource/)
- [Internal Wiki](http://wiki.example.com/database/operations)
