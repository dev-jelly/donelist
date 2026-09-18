# PostgreSQL Read Replicas Setup and Management

This document covers PostgreSQL streaming replication setup, read traffic distribution, and replica lag management for the Donelist backend.

## Overview

Task #18.8 focuses on:
1. Streaming replication configuration
2. Read replica setup and monitoring
3. Replication lag management
4. Read traffic routing strategies
5. Failover and promotion procedures

## 1. Why Read Replicas?

### Benefits

- **Horizontal Scaling**: Distribute read load across multiple servers
- **High Availability**: Replicas can be promoted during primary failure
- **Geographic Distribution**: Place replicas closer to users
- **Analytics Workload**: Run heavy queries on replicas
- **Backup Source**: Use replicas for backups without impacting primary

### Use Cases

- Read-heavy applications (typical web apps: 80-90% reads)
- Reporting and analytics queries
- Cross-region deployments
- Zero-downtime upgrades

## 2. Replication Architecture

```
┌─────────────────┐
│  Primary (RW)   │  Port 5432
│  Write Traffic  │
└────────┬────────┘
         │
         │ WAL Stream
         ├──────────────────┬──────────────────┐
         │                  │                  │
┌────────▼────────┐ ┌──────▼──────────┐ ┌────▼───────────┐
│   Replica 1     │ │   Replica 2     │ │   Replica 3    │
│  Read Traffic   │ │  Read Traffic   │ │  Analytics     │
│  (US East)      │ │  (US West)      │ │  (Reporting)   │
└─────────────────┘ └─────────────────┘ └────────────────┘
```

## 3. Primary Database Configuration

### Enable WAL Archiving

```sql
-- Edit postgresql.conf on PRIMARY

-- WAL level must be 'replica' or 'logical'
wal_level = replica

-- Number of WAL sender processes (one per replica + 1-2 spare)
max_wal_senders = 10

-- Number of replication slots (one per replica)
max_replication_slots = 10

-- WAL keep size (keep enough WAL for replica catchup)
wal_keep_size = 1GB  -- PostgreSQL 13+
# wal_keep_segments = 64  -- PostgreSQL 12 and earlier

-- WAL archiving (recommended for point-in-time recovery)
archive_mode = on
archive_command = 'test ! -f /var/lib/postgresql/wal_archive/%f && cp %p /var/lib/postgresql/wal_archive/%f'
archive_timeout = 300  -- Force archive every 5 minutes

-- Hot standby feedback (prevents query conflicts)
hot_standby_feedback = on

-- Synchronous commit (optional, for critical consistency)
synchronous_commit = on
# synchronous_standby_names = 'replica1,replica2'  -- For synchronous replication
```

### Restart PostgreSQL

```bash
# Restart to apply configuration
sudo systemctl restart postgresql

# Or in Docker
docker-compose restart postgres
```

### Create Replication User

```sql
-- Create dedicated replication user
CREATE USER replicator WITH REPLICATION ENCRYPTED PASSWORD 'strong_password_here';

-- Grant connection permission
GRANT CONNECT ON DATABASE donelist TO replicator;

-- Optional: Grant read access for monitoring
GRANT SELECT ON ALL TABLES IN SCHEMA public TO replicator;
```

### Configure pg_hba.conf

```conf
# Add replication access rules
# TYPE  DATABASE        USER            ADDRESS                 METHOD

# Local replication
host    replication     replicator      127.0.0.1/32            md5
host    replication     replicator      ::1/128                 md5

# Replica servers (adjust IPs for your setup)
host    replication     replicator      10.0.1.0/24             md5
host    replication     replicator      192.168.1.0/24          md5

# For AWS/cloud deployment with dynamic IPs
host    replication     replicator      0.0.0.0/0               md5
hostssl replication     replicator      0.0.0.0/0               md5
```

### Reload Configuration

```bash
# Reload pg_hba.conf without restart
sudo systemctl reload postgresql

# Or send signal
pg_ctl reload
```

## 4. Replica Setup

### Method 1: Using pg_basebackup (Recommended)

```bash
# On REPLICA server

# Stop PostgreSQL if running
sudo systemctl stop postgresql

# Remove existing data directory
sudo rm -rf /var/lib/postgresql/14/main/*

# Create base backup from primary
sudo -u postgres pg_basebackup \
    -h primary.example.com \
    -D /var/lib/postgresql/14/main \
    -U replicator \
    -P \
    -v \
    -R \
    -X stream \
    -C -S replica1_slot

# Flags:
# -h: Primary server hostname
# -D: Data directory
# -U: Replication user
# -P: Show progress
# -v: Verbose
# -R: Create recovery configuration (standby.signal + connection info)
# -X stream: Stream WAL during backup
# -C: Create replication slot
# -S: Replication slot name

# Verify standby.signal file created
ls -l /var/lib/postgresql/14/main/standby.signal

# Start replica
sudo systemctl start postgresql
```

### Method 2: Manual Setup

```bash
# 1. Take base backup
pg_basebackup -h primary.example.com -D /backup/replica -U replicator -P -v

# 2. Copy to replica server
rsync -avz /backup/replica/ replica.example.com:/var/lib/postgresql/14/main/

# 3. Create standby.signal file
touch /var/lib/postgresql/14/main/standby.signal

# 4. Configure recovery settings
cat >> /var/lib/postgresql/14/main/postgresql.auto.conf <<EOF
primary_conninfo = 'host=primary.example.com port=5432 user=replicator password=strong_password_here application_name=replica1'
primary_slot_name = 'replica1_slot'
hot_standby = on
EOF

# 5. Start replica
sudo systemctl start postgresql
```

### Docker Setup

```yaml
# docker-compose.yml
version: '3.8'

services:
  postgres-primary:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: donelist
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      POSTGRES_DB: donelist
    ports:
      - "5432:5432"
    volumes:
      - pg-primary-data:/var/lib/postgresql/data
      - ./postgresql-primary.conf:/etc/postgresql/postgresql.conf
    command: postgres -c config_file=/etc/postgresql/postgresql.conf

  postgres-replica:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: donelist
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      PGDATA: /var/lib/postgresql/data
    ports:
      - "5433:5432"
    volumes:
      - pg-replica-data:/var/lib/postgresql/data
    command: >
      bash -c "
      if [ ! -f /var/lib/postgresql/data/standby.signal ]; then
        rm -rf /var/lib/postgresql/data/*
        pg_basebackup -h postgres-primary -D /var/lib/postgresql/data -U replicator -P -v -R -X stream
      fi
      postgres
      "
    depends_on:
      - postgres-primary

volumes:
  pg-primary-data:
  pg-replica-data:
```

## 5. Replica Configuration

### postgresql.conf on Replica

```ini
# Hot standby allows read queries on replica
hot_standby = on

# Feedback to prevent query conflicts
hot_standby_feedback = on

# Max delay before cancelling conflicting queries
max_standby_streaming_delay = 30s
max_standby_archive_delay = 30s

# Replica-specific settings
wal_receiver_status_interval = 10s
wal_retrieve_retry_interval = 5s

# Read-only queries settings
hot_standby_feedback = on
```

### Verify Replication Status

```sql
-- On PRIMARY: Check replication status
SELECT
    client_addr,
    application_name,
    state,
    sync_state,
    replay_lag,
    write_lag,
    flush_lag,
    sent_lsn,
    replay_lsn
FROM pg_stat_replication;

-- On REPLICA: Check recovery status
SELECT
    pg_is_in_recovery() as is_replica,
    pg_last_wal_receive_lsn() as receive_lsn,
    pg_last_wal_replay_lsn() as replay_lsn,
    pg_last_xact_replay_timestamp() as last_replay_time;

-- On REPLICA: Check replication lag
SELECT
    EXTRACT(EPOCH FROM (now() - pg_last_xact_replay_timestamp()))::INT as lag_seconds;
```

## 6. Replication Lag Management

### Monitoring Lag

```sql
-- Create monitoring view
CREATE OR REPLACE VIEW replication_lag_monitor AS
SELECT
    application_name,
    client_addr,
    state,
    sync_state,
    -- Lag in seconds
    EXTRACT(EPOCH FROM replay_lag)::INT as replay_lag_seconds,
    EXTRACT(EPOCH FROM write_lag)::INT as write_lag_seconds,
    EXTRACT(EPOCH FROM flush_lag)::INT as flush_lag_seconds,
    -- Bytes behind
    pg_wal_lsn_diff(sent_lsn, replay_lsn) as bytes_behind,
    -- Last activity
    backend_start,
    EXTRACT(EPOCH FROM (now() - backend_start))::INT as connection_age_seconds
FROM pg_stat_replication;

-- Query lag
SELECT * FROM replication_lag_monitor;
```

### Acceptable Lag Thresholds

| Application Type | Max Acceptable Lag |
|-----------------|-------------------|
| Real-time dashboard | < 1 second |
| User-facing queries | < 5 seconds |
| Analytics/Reporting | < 60 seconds |
| Backups/ETL | < 300 seconds |

### Managing High Lag

```sql
-- Identify cause of lag on PRIMARY
SELECT
    pid,
    state,
    application_name,
    wait_event_type,
    wait_event,
    query
FROM pg_stat_activity
WHERE backend_type = 'walsender';

-- Check for long-running queries on REPLICA causing conflicts
SELECT
    pid,
    usename,
    state,
    now() - query_start as duration,
    query
FROM pg_stat_activity
WHERE state = 'active'
  AND pid <> pg_backend_pid()
ORDER BY duration DESC;

-- Kill problematic query on replica
SELECT pg_terminate_backend(pid);
```

### Automatic Lag Prevention

```sql
-- Set statement timeout on replicas to prevent conflicts
ALTER DATABASE donelist SET statement_timeout = '30s';

-- Or per-session
SET statement_timeout = '30s';
```

## 7. Read Traffic Routing

### Application-Level Routing

```go
// pkg/database/replica_pool.go

type DatabasePool struct {
    Primary  *sqlx.DB
    Replicas []*sqlx.DB
    mu       sync.RWMutex
    current  int
}

func NewDatabasePool(primaryDSN string, replicaDSNs []string) (*DatabasePool, error) {
    primary, err := sqlx.Connect("postgres", primaryDSN)
    if err != nil {
        return nil, fmt.Errorf("connect to primary: %w", err)
    }

    var replicas []*sqlx.DB
    for i, dsn := range replicaDSNs {
        replica, err := sqlx.Connect("postgres", dsn)
        if err != nil {
            return nil, fmt.Errorf("connect to replica %d: %w", i, err)
        }
        replicas = append(replicas, replica)
    }

    return &DatabasePool{
        Primary:  primary,
        Replicas: replicas,
    }, nil
}

// GetPrimary returns connection for write operations
func (p *DatabasePool) GetPrimary() *sqlx.DB {
    return p.Primary
}

// GetReplica returns connection for read operations (round-robin)
func (p *DatabasePool) GetReplica() *sqlx.DB {
    if len(p.Replicas) == 0 {
        return p.Primary
    }

    p.mu.Lock()
    defer p.mu.Unlock()

    replica := p.Replicas[p.current]
    p.current = (p.current + 1) % len(p.Replicas)
    return replica
}

// GetReplicaWithLagCheck returns replica if lag is acceptable
func (p *DatabasePool) GetReplicaWithLagCheck(maxLagSeconds int) *sqlx.DB {
    for _, replica := range p.Replicas {
        var lagSeconds int
        err := replica.Get(&lagSeconds, `
            SELECT EXTRACT(EPOCH FROM (now() - pg_last_xact_replay_timestamp()))::INT
        `)
        if err == nil && lagSeconds <= maxLagSeconds {
            return replica
        }
    }
    // Fallback to primary if all replicas are lagging
    return p.Primary
}
```

### Repository Pattern

```go
// internal/checkin/repository.go

type Repository struct {
    pool *database.DatabasePool
}

// Read operation - uses replica
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*Checkin, error) {
    db := r.pool.GetReplica()
    var checkin Checkin
    err := db.GetContext(ctx, &checkin, `
        SELECT * FROM checkins WHERE id = $1 AND deleted_at IS NULL
    `, id)
    return &checkin, err
}

// Write operation - uses primary
func (r *Repository) Create(ctx context.Context, checkin *Checkin) error {
    db := r.pool.GetPrimary()
    return db.GetContext(ctx, checkin, `
        INSERT INTO checkins (user_id, content, category_id, checkin_time)
        VALUES ($1, $2, $3, $4)
        RETURNING *
    `, checkin.UserID, checkin.Content, checkin.CategoryID, checkin.CheckinTime)
}

// Read-after-write - uses primary to avoid replication lag
func (r *Repository) GetByIDConsistent(ctx context.Context, id uuid.UUID) (*Checkin, error) {
    db := r.pool.GetPrimary()
    var checkin Checkin
    err := db.GetContext(ctx, &checkin, `
        SELECT * FROM checkins WHERE id = $1 AND deleted_at IS NULL
    `, id)
    return &checkin, err
}
```

### Load Balancer Routing (HAProxy)

```haproxy
# haproxy.cfg

global
    maxconn 10000

defaults
    mode tcp
    timeout connect 10s
    timeout client 30s
    timeout server 30s

# Primary (write) endpoint
listen postgres-primary
    bind *:5432
    option pgsql-check user healthcheck
    server primary primary.example.com:5432 check

# Read replicas (read) endpoint
listen postgres-replicas
    bind *:5433
    balance roundrobin
    option pgsql-check user healthcheck
    server replica1 replica1.example.com:5432 check
    server replica2 replica2.example.com:5432 check
    server replica3 replica3.example.com:5432 check backup
```

### Kubernetes Service

```yaml
# k8s/postgres-services.yaml

apiVersion: v1
kind: Service
metadata:
  name: postgres-primary
spec:
  selector:
    app: postgres
    role: primary
  ports:
    - port: 5432
      targetPort: 5432
---
apiVersion: v1
kind: Service
metadata:
  name: postgres-replicas
spec:
  selector:
    app: postgres
    role: replica
  ports:
    - port: 5432
      targetPort: 5432
```

## 8. Failover and Promotion

### Manual Promotion

```bash
# On REPLICA to be promoted

# Stop accepting connections
sudo systemctl stop postgresql

# Remove standby.signal to promote to primary
rm /var/lib/postgresql/14/main/standby.signal

# Start as primary
sudo systemctl start postgresql

# Verify promotion
psql -U postgres -c "SELECT pg_is_in_recovery();"
# Should return: f (false)
```

### Scripted Promotion

```bash
#!/bin/bash
# promote-replica.sh

REPLICA_HOST="replica1.example.com"
DATA_DIR="/var/lib/postgresql/14/main"

echo "Promoting replica at $REPLICA_HOST to primary..."

# Promote replica
ssh postgres@$REPLICA_HOST "pg_ctl promote -D $DATA_DIR"

# Wait for promotion to complete
sleep 5

# Verify promotion
ssh postgres@$REPLICA_HOST "psql -c 'SELECT pg_is_in_recovery()'"

echo "Promotion complete"
```

### Re-establishing Replication After Failover

```bash
# On OLD PRIMARY (now needs to become replica)

# Stop PostgreSQL
sudo systemctl stop postgresql

# Create standby.signal
touch /var/lib/postgresql/14/main/standby.signal

# Configure to follow NEW PRIMARY
cat >> /var/lib/postgresql/14/main/postgresql.auto.conf <<EOF
primary_conninfo = 'host=new-primary.example.com port=5432 user=replicator password=strong_password'
EOF

# Start as replica
sudo systemctl start postgresql
```

## 9. Monitoring and Alerting

### Prometheus Metrics

```yaml
# postgres_exporter configuration
scrape_configs:
  - job_name: 'postgres'
    static_configs:
      - targets: ['primary:9187', 'replica1:9187', 'replica2:9187']
```

### Key Metrics

```sql
-- Replication lag (export to Prometheus)
SELECT
    application_name,
    EXTRACT(EPOCH FROM replay_lag)::FLOAT as replication_lag_seconds,
    pg_wal_lsn_diff(sent_lsn, replay_lsn) as replication_lag_bytes
FROM pg_stat_replication;

-- Replica query conflicts
SELECT
    datname,
    confl_tablespace,
    confl_lock,
    confl_snapshot,
    confl_bufferpin,
    confl_deadlock
FROM pg_stat_database_conflicts;
```

### Alert Rules

```yaml
# alerting_rules.yml
groups:
  - name: postgresql_replication
    rules:
      - alert: ReplicationLagHigh
        expr: pg_replication_lag_seconds > 30
        for: 5m
        annotations:
          summary: "Replication lag is high (> 30s)"

      - alert: ReplicaDown
        expr: up{job="postgres",role="replica"} == 0
        for: 1m
        annotations:
          summary: "PostgreSQL replica is down"

      - alert: ReplicationSlotInactive
        expr: pg_replication_slots_active == 0
        for: 5m
        annotations:
          summary: "Replication slot is inactive"
```

## 10. Best Practices

### DO

- ✓ Monitor replication lag continuously
- ✓ Use replication slots to prevent WAL deletion
- ✓ Test failover procedures regularly
- ✓ Route read-after-write to primary
- ✓ Set appropriate timeouts on replicas
- ✓ Use dedicated network for replication
- ✓ Enable hot_standby_feedback
- ✓ Archive WAL for PITR

### DON'T

- ✗ Route writes to replicas
- ✗ Ignore replication lag warnings
- ✗ Run long-running queries on replicas without timeouts
- ✗ Forget to create replication slots
- ✗ Mix OLTP and OLAP workloads on same replica
- ✗ Ignore query conflicts

## 11. Performance Targets

| Metric | Target | Excellent |
|--------|--------|-----------|
| Replication lag | < 1s | < 100ms |
| Replica queries (p95) | < 50ms | < 10ms |
| Read traffic distributed | > 70% | > 90% |
| Failover time (RPO) | < 60s | < 30s |
| Primary CPU reduced | > 30% | > 50% |

## 12. References

- [PostgreSQL Replication Documentation](https://www.postgresql.org/docs/current/high-availability.html)
- [Streaming Replication Setup](https://www.postgresql.org/docs/current/warm-standby.html)
- [pg_basebackup](https://www.postgresql.org/docs/current/app-pgbasebackup.html)
- [Replication Monitoring](https://www.postgresql.org/docs/current/monitoring-stats.html#MONITORING-PG-STAT-REPLICATION-VIEW)
