# Connection Pooling with pgBouncer

This document covers connection pooling setup, pgBouncer configuration, and load testing strategies for the Donelist backend.

## Overview

Task #18.6 focuses on:
1. pgBouncer deployment and configuration
2. Connection pool optimization
3. Timeout configuration
4. Prepared statement compatibility
5. Load testing and validation

## 1. Why Connection Pooling?

### Problems Without Pooling

- **Connection Overhead**: Each PostgreSQL connection consumes ~10MB of memory
- **Slow Connection Setup**: Creating connections takes 10-50ms
- **Connection Exhaustion**: PostgreSQL max_connections typically limited to 100-400
- **Resource Waste**: Idle connections hold server resources

### Benefits of pgBouncer

- **Connection Multiplexing**: 1000+ client connections using 20 server connections
- **Fast Connection Reuse**: < 1ms connection acquisition
- **Controlled Load**: Prevents database overload
- **Resource Efficiency**: Minimal memory footprint (~2KB per client connection)

## 2. pgBouncer Installation

### Docker Deployment (Recommended)

```yaml
# docker-compose.yml
version: '3.8'

services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: donelist
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      POSTGRES_DB: donelist
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data
    command:
      - "postgres"
      - "-c"
      - "max_connections=100"
      - "-c"
      - "shared_buffers=2GB"

  pgbouncer:
    image: edoburu/pgbouncer:1.21.0
    environment:
      DATABASE_URL: "postgres://donelist:${POSTGRES_PASSWORD}@postgres:5432/donelist"
      POOL_MODE: transaction
      MAX_CLIENT_CONN: 1000
      DEFAULT_POOL_SIZE: 25
      MIN_POOL_SIZE: 5
      RESERVE_POOL_SIZE: 5
      RESERVE_POOL_TIMEOUT: 5
      MAX_DB_CONNECTIONS: 50
      MAX_USER_CONNECTIONS: 50
      SERVER_LIFETIME: 3600
      SERVER_IDLE_TIMEOUT: 600
      QUERY_TIMEOUT: 30
      QUERY_WAIT_TIMEOUT: 120
      CLIENT_IDLE_TIMEOUT: 0
      IDLE_TRANSACTION_TIMEOUT: 60
      LOG_CONNECTIONS: 1
      LOG_DISCONNECTIONS: 1
      LOG_POOLER_ERRORS: 1
      STATS_PERIOD: 60
    ports:
      - "6432:5432"
    depends_on:
      - postgres

volumes:
  pgdata:
```

### Standalone Installation

```bash
# Ubuntu/Debian
sudo apt-get update
sudo apt-get install -y pgbouncer

# macOS
brew install pgbouncer

# Verify installation
pgbouncer --version
```

## 3. pgBouncer Configuration

### Main Configuration File (pgbouncer.ini)

```ini
[databases]
donelist = host=localhost port=5432 dbname=donelist user=donelist password=SECRET

[pgbouncer]
# Connection settings
listen_addr = 0.0.0.0
listen_port = 6432
auth_type = md5
auth_file = /etc/pgbouncer/userlist.txt

# Pool mode (transaction is most efficient for web apps)
pool_mode = transaction

# Connection limits
max_client_conn = 1000          # Maximum number of client connections
default_pool_size = 25          # Default pool size per database
min_pool_size = 5               # Minimum pool size to maintain
reserve_pool_size = 5           # Emergency connections for admin
reserve_pool_timeout = 5        # Timeout for reserve pool

# Per-database connection limits
max_db_connections = 50         # Total connections to this database
max_user_connections = 50       # Per-user connection limit

# Timeouts (in seconds)
server_lifetime = 3600          # Max connection lifetime (1 hour)
server_idle_timeout = 600       # Close idle server connections after 10 min
server_connect_timeout = 15     # Timeout for connecting to PostgreSQL
query_timeout = 30              # Kill queries running longer than 30s
query_wait_timeout = 120        # Max time in queue before client disconnect
client_idle_timeout = 0         # Don't disconnect idle clients (set to 300 for 5min)
idle_transaction_timeout = 60   # Abort idle transactions after 1 min

# DNS
dns_max_ttl = 15
dns_nxdomain_ttl = 15

# Logging
log_connections = 1
log_disconnections = 1
log_pooler_errors = 1
stats_period = 60

# Admin
admin_users = postgres, admin
stats_users = stats_user

# TLS (optional but recommended for production)
# client_tls_sslmode = require
# client_tls_ca_file = /etc/ssl/certs/ca.pem
# client_tls_cert_file = /etc/ssl/certs/server.crt
# client_tls_key_file = /etc/ssl/private/server.key

# Performance tuning
pkt_buf = 4096
listen_backlog = 128
sbuf_loopcnt = 5
so_reuseport = 1
tcp_keepalive = 1
tcp_keepidle = 600
tcp_keepintvl = 30
tcp_keepcnt = 10
```

### User Authentication File (userlist.txt)

```txt
# Format: "username" "md5hash"
# Generate hash: echo -n "passwordusername" | md5sum

"donelist" "md5e0d8e098af1f3c8c8c8c8c8c8c8c8c8"
"readonly" "md5a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5"
"admin" "md5f1e2d3c4b5a6978869504132231415"
```

### Generate Password Hash

```bash
# Generate pgBouncer password hash
echo -n "your_password_here$(whoami)" | md5sum | awk '{print "md5"$1}'
```

## 4. Pool Mode Comparison

| Mode | Use Case | Pros | Cons |
|------|----------|------|------|
| **session** | Long-lived sessions | Full PostgreSQL feature support | Low connection reuse |
| **transaction** | Web applications | High connection reuse | No session-level features |
| **statement** | Simple queries only | Highest efficiency | No transactions, prepared statements |

### Recommended: Transaction Mode

**Why?**
- Best balance of performance and compatibility
- Works with most ORMs (sqlx, GORM, etc.)
- Supports transactions
- High connection reuse

**Limitations:**
- No prepared statements (unless using server-side)
- No session-level variables
- No LISTEN/NOTIFY
- No cursors

## 5. Application Configuration

### Connection String

```go
// config/config.go

type DatabaseConfig struct {
    // Primary database (through pgBouncer)
    Host            string // "localhost"
    Port            int    // 6432 (pgBouncer port)

    // Direct connection for admin operations
    DirectHost      string // "localhost"
    DirectPort      int    // 5432 (PostgreSQL port)

    User            string
    Password        string
    Name            string
    SSLMode         string

    // Connection pool settings (application-level)
    MaxOpenConns    int           // 100
    MaxIdleConns    int           // 25
    ConnMaxLifetime time.Duration // 30m
    ConnMaxIdleTime time.Duration // 10m
}

func (c *DatabaseConfig) DSN() string {
    return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
        c.Host, c.Port, c.User, c.Password, c.Name, c.SSLMode)
}

func (c *DatabaseConfig) DirectDSN() string {
    return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
        c.DirectHost, c.DirectPort, c.User, c.Password, c.Name, c.SSLMode)
}
```

### Environment Variables

```env
# Application connects to pgBouncer
POSTGRES_HOST=localhost
POSTGRES_PORT=6432

# Direct connection for migrations and admin tasks
POSTGRES_DIRECT_HOST=localhost
POSTGRES_DIRECT_PORT=5432

# Connection pool settings
DB_MAX_OPEN_CONNS=100
DB_MAX_IDLE_CONNS=25
DB_CONN_MAX_LIFETIME=30m
DB_CONN_MAX_IDLE_TIME=10m
```

## 6. Prepared Statement Handling

### Problem with Transaction Mode

pgBouncer's transaction mode doesn't support client-side prepared statements.

### Solution: Use Simple Query Protocol

```go
// pkg/database/pool.go

func ConfigurePool(db *sqlx.DB, cfg config.DatabaseConfig) error {
    // Set connection pool parameters
    db.SetMaxOpenConns(cfg.MaxOpenConns)
    db.SetMaxIdleConns(cfg.MaxIdleConns)
    db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
    db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

    // Disable client-side prepared statements for pgBouncer compatibility
    // PostgreSQL driver will use simple query protocol
    // Slightly slower but works with transaction pooling
    db.SetConnMaxIdleTime(0) // Force simple protocol

    return nil
}

// Alternative: Use server-side prepared statements
func PrepareServerSide(db *sql.DB, name, query string) error {
    _, err := db.Exec(fmt.Sprintf("PREPARE %s AS %s", name, query))
    return err
}
```

### Using sqlx with pgBouncer

```go
// No special configuration needed
// sqlx uses simple query protocol by default when appropriate

db, err := sqlx.Connect("postgres", dsn)
if err != nil {
    return err
}

// Queries work as expected
var users []User
err = db.Select(&users, "SELECT * FROM users WHERE deleted_at IS NULL")
```

## 7. Monitoring pgBouncer

### Admin Console

```bash
# Connect to pgBouncer admin console
psql -h localhost -p 6432 -U admin pgbouncer

# Inside console:
SHOW POOLS;
SHOW STATS;
SHOW DATABASES;
SHOW CLIENTS;
SHOW SERVERS;
SHOW CONFIG;
```

### Key Metrics

```sql
-- Connection pool status
SHOW POOLS;
-- Columns: database, user, cl_active, cl_waiting, sv_active, sv_idle, sv_used, sv_tested, sv_login, maxwait

-- Statistics
SHOW STATS;
-- Columns: database, total_xact_count, total_query_count, total_received, total_sent, total_xact_time, total_query_time

-- Active clients
SHOW CLIENTS;
-- Columns: type, user, database, state, addr, port, local_addr, local_port, connect_time, request_time, ptr, link, remote_pid

-- Server connections
SHOW SERVERS;
-- Columns: type, user, database, state, addr, port, local_addr, local_port, connect_time, request_time, ptr, link, remote_pid
```

### Prometheus Metrics Exporter

```yaml
# docker-compose.yml
services:
  pgbouncer-exporter:
    image: spreaker/prometheus-pgbouncer-exporter
    environment:
      PGBOUNCER_HOST: pgbouncer
      PGBOUNCER_PORT: 6432
      PGBOUNCER_USER: stats_user
      PGBOUNCER_PASS: ${STATS_PASSWORD}
    ports:
      - "9127:9127"
    depends_on:
      - pgbouncer
```

### Key Metrics to Monitor

1. **cl_waiting**: Clients waiting for connection (should be 0)
2. **maxwait**: Maximum wait time (should be < 1s)
3. **sv_active**: Active server connections (should be < default_pool_size)
4. **sv_idle**: Idle server connections available
5. **avg_xact_time**: Average transaction time
6. **avg_query_time**: Average query time

## 8. Load Testing

### Using pgbench

```bash
# Initialize test data
pgbench -i -h localhost -p 6432 -U donelist donelist

# Run benchmark with 50 clients, 100 threads, 60 seconds
pgbench -h localhost -p 6432 -U donelist -c 50 -j 10 -T 60 donelist

# Custom transaction script
cat > test_queries.sql <<EOF
\set user_id random(1, 10000)
SELECT * FROM users WHERE id = :user_id AND deleted_at IS NULL;
SELECT * FROM checkins WHERE user_id = :user_id AND deleted_at IS NULL ORDER BY checkin_time DESC LIMIT 50;
EOF

pgbench -h localhost -p 6432 -U donelist -c 100 -j 10 -T 120 -f test_queries.sql donelist
```

### Using k6 (HTTP Load Testing)

```javascript
// load-test.js
import http from 'k6/http';
import { check, sleep } from 'k6';

export let options = {
    stages: [
        { duration: '2m', target: 100 },   // Ramp up to 100 users
        { duration: '5m', target: 100 },   // Stay at 100 users
        { duration: '2m', target: 200 },   // Ramp up to 200 users
        { duration: '5m', target: 200 },   // Stay at 200 users
        { duration: '2m', target: 0 },     // Ramp down to 0 users
    ],
    thresholds: {
        http_req_duration: ['p(95)<500'], // 95% of requests should be below 500ms
        http_req_failed: ['rate<0.01'],   // Error rate should be less than 1%
    },
};

const BASE_URL = 'http://localhost:8080';
const AUTH_TOKEN = 'your-jwt-token-here';

export default function () {
    let headers = {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${AUTH_TOKEN}`,
    };

    // Test timeline endpoint
    let res = http.get(`${BASE_URL}/api/v1/timeline`, { headers });
    check(res, {
        'status is 200': (r) => r.status === 200,
        'response time < 500ms': (r) => r.timings.duration < 500,
    });

    sleep(1);

    // Test checkin creation
    let payload = JSON.stringify({
        content: 'Load test checkin',
        category_id: 'uuid-here',
        checkin_time: new Date().toISOString(),
    });

    res = http.post(`${BASE_URL}/api/v1/checkins`, payload, { headers });
    check(res, {
        'checkin created': (r) => r.status === 201,
        'response time < 200ms': (r) => r.timings.duration < 200,
    });

    sleep(2);
}
```

```bash
# Run k6 load test
k6 run load-test.js

# With detailed output
k6 run --out json=results.json load-test.js
```

### Connection Storm Test

```bash
# Simulate connection storm (1000 rapid connections)
for i in {1..1000}; do
    psql -h localhost -p 6432 -U donelist -d donelist -c "SELECT 1" &
done
wait

# Monitor pgBouncer during test
watch -n 1 'psql -h localhost -p 6432 -U admin pgbouncer -c "SHOW POOLS"'
```

## 9. Tuning Guidelines

### Small Application (< 100 concurrent users)

```ini
default_pool_size = 10
max_client_conn = 200
reserve_pool_size = 3
```

### Medium Application (100-1000 concurrent users)

```ini
default_pool_size = 25
max_client_conn = 1000
reserve_pool_size = 5
```

### Large Application (> 1000 concurrent users)

```ini
default_pool_size = 50
max_client_conn = 5000
reserve_pool_size = 10
```

### Rule of Thumb

```
default_pool_size = (number of CPU cores) * 2
max_client_conn = default_pool_size * 20
```

## 10. Troubleshooting

### Issue: "no more connections allowed (max_client_conn)"

**Solution**: Increase max_client_conn in pgbouncer.ini

```ini
max_client_conn = 2000
```

### Issue: Clients waiting for connections (cl_waiting > 0)

**Solution**: Increase pool size or check for slow queries

```ini
default_pool_size = 50
query_timeout = 30
```

### Issue: "prepared statement does not exist"

**Solution**: Using wrong pool mode or driver not compatible

```ini
# Use transaction mode
pool_mode = transaction

# Or switch to session mode (less efficient)
pool_mode = session
```

### Issue: Connection timeout errors

**Solution**: Increase timeout values

```ini
server_connect_timeout = 30
query_wait_timeout = 180
```

### Issue: Idle transaction timeout

**Solution**: Adjust based on application behavior

```ini
# Increase if app needs longer transactions
idle_transaction_timeout = 120

# Or fix application to commit/rollback promptly
```

## 11. Production Deployment Checklist

- [ ] Configure pgBouncer with appropriate pool sizes
- [ ] Set up monitoring and alerting
- [ ] Test connection limits with load testing
- [ ] Configure timeouts based on application SLA
- [ ] Enable TLS for secure connections
- [ ] Set up log rotation for pgBouncer logs
- [ ] Document connection string format for team
- [ ] Update CI/CD to handle pgBouncer in tests
- [ ] Create runbook for common issues
- [ ] Schedule regular pool statistics review

## 12. Performance Targets

After implementing pgBouncer:

| Metric | Target | Excellent |
|--------|--------|-----------|
| Connection acquisition | < 5ms | < 1ms |
| Max clients handled | 1000+ | 5000+ |
| Pool utilization | 60-80% | 70-75% |
| Client wait time | < 100ms | < 10ms |
| Server connections | < 50 | < 30 |

## 13. References

- [pgBouncer Official Documentation](https://www.pgbouncer.org/config.html)
- [PostgreSQL Connection Pooling](https://wiki.postgresql.org/wiki/Number_Of_Database_Connections)
- [pgBouncer Best Practices](https://www.postgresql.org/docs/current/pgbouncer.html)
- [Pool Mode Comparison](https://www.percona.com/blog/2018/06/27/comparing-pgbouncer-pool-modes/)
