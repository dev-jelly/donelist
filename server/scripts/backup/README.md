# Donelist Backup and Recovery System

Comprehensive backup and disaster recovery system for the Donelist server application.

## Table of Contents

- [Overview](#overview)
- [Features](#features)
- [Architecture](#architecture)
- [Setup](#setup)
- [Usage](#usage)
- [Disaster Recovery Procedures](#disaster-recovery-procedures)
- [Monitoring](#monitoring)
- [Troubleshooting](#troubleshooting)

## Overview

This backup system provides:
- **Automated PostgreSQL backups** with compression and encryption
- **S3-compatible storage** for off-site backup storage
- **WAL archiving** for point-in-time recovery
- **Automated retention policies** (daily, weekly, monthly)
- **Backup verification** and integrity checking
- **Monitoring and alerting** via webhooks
- **Disaster recovery procedures** with step-by-step guides

## Features

### Backup Features
- ✅ Full PostgreSQL database backups using `pg_dump`
- ✅ Compression (gzip) to reduce storage costs
- ✅ Optional encryption (AES-256-GCM)
- ✅ SHA-256 checksums for integrity verification
- ✅ Automated scheduling via cron
- ✅ S3/MinIO upload for off-site storage

### Recovery Features
- ✅ Full database restoration
- ✅ Point-in-time recovery (PITR) using WAL files
- ✅ Backup verification before restore
- ✅ Checksum validation
- ✅ Automated database recreation

### Retention Policy
- **Daily backups**: Kept for 7 days
- **Weekly backups**: Kept for 4 weeks (one per week)
- **Monthly backups**: Kept for 12 months (one per month)
- **WAL files**: Kept for 7 days (configurable)

### Monitoring
- ✅ Backup age monitoring
- ✅ Backup size monitoring
- ✅ Integrity verification
- ✅ Disk space monitoring
- ✅ Webhook alerts for failures
- ✅ Health check reports

## Architecture

```
┌─────────────────┐
│   PostgreSQL    │
│    Database     │
└────────┬────────┘
         │
         ▼
┌─────────────────┐       ┌──────────────┐
│   pg_dump +     │──────▶│ Local Backup │
│   Compression   │       │   Storage    │
└─────────────────┘       └──────┬───────┘
                                 │
                                 ▼
                          ┌──────────────┐
                          │  S3/MinIO    │
                          │   Storage    │
                          └──────────────┘

┌─────────────────┐       ┌──────────────┐
│  WAL Archiving  │──────▶│ WAL Storage  │
│  (PostgreSQL)   │       │  (S3/Local)  │
└─────────────────┘       └──────────────┘

┌─────────────────┐       ┌──────────────┐
│   Scheduler     │──────▶│  Monitoring  │
│   (Cron/Go)     │       │   & Alerts   │
└─────────────────┘       └──────────────┘
```

## Setup

### Prerequisites

1. **PostgreSQL** 12+ with client tools (`pg_dump`, `pg_restore`)
2. **Go** 1.24+ (for Go-based tools)
3. **AWS CLI** (optional, for S3 storage)
4. **S3-compatible storage** (AWS S3, MinIO, etc.)

### Configuration

#### 1. Environment Variables

Create or update `.env` file:

```bash
# Database Configuration
DB_HOST=localhost
DB_PORT=5432
DB_NAME=donelist
DB_USER=donelist
DB_PASSWORD=your_secure_password

# Backup Configuration
BACKUP_DIR=/var/backups/donelist
BACKUP_SCHEDULE="0 2 * * *"  # 2 AM daily
RETENTION_DAYS=7
RETENTION_WEEKS=4
RETENTION_MONTHS=12

# S3 Configuration
S3_ENDPOINT=https://s3.amazonaws.com  # Or MinIO endpoint
S3_BUCKET=donelist-backups
S3_ACCESS_KEY=your_access_key
S3_SECRET_KEY=your_secret_key
S3_REGION=us-east-1

# WAL Archiving
WAL_ARCHIVING_ENABLED=true
WAL_ARCHIVE_DIR=/var/backups/donelist/wal

# Monitoring
ALERT_WEBHOOK=https://your-webhook-url.com/alerts
MAX_BACKUP_AGE_HOURS=48
MIN_BACKUP_SIZE_MB=10
```

#### 2. Create Backup Directory

```bash
sudo mkdir -p /var/backups/donelist
sudo mkdir -p /var/backups/donelist/wal
sudo chown -R $USER:$USER /var/backups/donelist
chmod 750 /var/backups/donelist
```

#### 3. Make Scripts Executable

```bash
chmod +x scripts/backup/*.sh
```

#### 4. Configure PostgreSQL for WAL Archiving (Optional)

Edit `postgresql.conf`:

```conf
# WAL Configuration
wal_level = replica
archive_mode = on
archive_command = '/path/to/donelist/scripts/backup/wal-archive.sh %p %f'
max_wal_senders = 3
wal_keep_size = 1GB
```

Restart PostgreSQL:

```bash
sudo systemctl restart postgresql
```

#### 5. Setup Cron Jobs

Add to crontab (`crontab -e`):

```cron
# Daily backup at 2 AM
0 2 * * * /path/to/donelist/scripts/backup/backup.sh >> /var/log/donelist-backup.log 2>&1

# Hourly monitoring
0 * * * * /path/to/donelist/scripts/backup/monitor.sh >> /var/log/donelist-monitor.log 2>&1

# Weekly cleanup (Sundays at 3 AM)
0 3 * * 0 find /var/backups/donelist -name "*.sql.gz" -mtime +7 -delete
```

## Usage

### Manual Backup

Create a backup manually:

```bash
./scripts/backup/backup.sh
```

With verification:

```bash
VERIFY_BACKUP=true ./scripts/backup/backup.sh
```

### Restore Database

#### From Local Backup

```bash
./scripts/backup/restore.sh --file /var/backups/donelist/postgres_full_donelist_20240101_120000.sql.gz --drop
```

#### From S3 Backup

```bash
./scripts/backup/restore.sh \
    --download s3://donelist-backups/backups/2024/01/01/postgres_full_donelist_20240101_120000.sql.gz \
    --drop \
    --verify
```

#### Restore to Different Database

```bash
./scripts/backup/restore.sh \
    --file backup.sql.gz \
    --database donelist_restored \
    --drop
```

### Using Go CLI Tool

#### Build the Tool

```bash
cd cmd/backup
go build -o backup-tool
```

#### Create Backup

```bash
./backup-tool --command backup --verify
```

#### Check Status

```bash
./backup-tool --command status
```

#### Run as Daemon

```bash
./backup-tool --daemon
```

### Monitoring

Run health check:

```bash
./scripts/backup/monitor.sh
```

Expected output:

```
Checking database connectivity...
✓ Database connection successful
Checking local backups...
✓ Latest backup age: 2 hours
✓ Latest backup size: 245MB
✓ Total local backups: 7
✓ Latest backup integrity verified
Checking S3 backups...
✓ Total S3 backups: 42
✓ Latest S3 backup: 2024-01-15 02:00:00 (245MB)
Checking disk space...
✓ Disk usage: 45%

=====================================
Backup Monitoring Summary
=====================================
Timestamp: 2024-01-15 14:30:00
Issues: 0
Warnings: 0
Status: HEALTHY
=====================================
```

## Disaster Recovery Procedures

### Scenario 1: Complete Database Loss

**Situation**: Database server crashed and all data is lost.

**Recovery Steps**:

1. **Identify Latest Backup**

```bash
# List available backups
aws s3 ls s3://donelist-backups/backups/ --recursive | grep .sql.gz | sort | tail -5
```

2. **Download Backup**

```bash
./scripts/backup/restore.sh \
    --download s3://donelist-backups/backups/2024/01/15/postgres_full_donelist_20240115_020000.sql.gz \
    --database donelist \
    --drop \
    --verify
```

3. **Verify Restoration**

```bash
psql -h localhost -U donelist -d donelist -c "SELECT COUNT(*) FROM users;"
```

4. **Resume Normal Operations**

```bash
sudo systemctl restart donelist-api
```

**Estimated Recovery Time**: 15-30 minutes (depending on backup size)

### Scenario 2: Data Corruption Detected

**Situation**: Data corruption detected, need to restore to a specific point in time.

**Recovery Steps**:

1. **Stop Application**

```bash
sudo systemctl stop donelist-api
```

2. **Identify Recovery Point**

Find the backup closest to the desired recovery time:

```bash
./scripts/backup/monitor.sh
```

3. **Restore Base Backup**

```bash
./scripts/backup/restore.sh \
    --file /var/backups/donelist/postgres_full_donelist_20240115_020000.sql.gz \
    --database donelist_recovery \
    --drop
```

4. **Apply WAL Files (Point-in-Time Recovery)**

If WAL archiving is enabled, apply WAL files up to the corruption point:

```bash
# This requires manual configuration of recovery.conf or postgresql.auto.conf
# See PostgreSQL documentation for point-in-time recovery
```

5. **Verify Data**

```bash
psql -h localhost -U donelist -d donelist_recovery -c "SELECT * FROM users ORDER BY created_at DESC LIMIT 10;"
```

6. **Switch to Recovered Database**

```bash
# Rename current database
psql -h localhost -U postgres -c "ALTER DATABASE donelist RENAME TO donelist_corrupted;"

# Rename recovered database
psql -h localhost -U postgres -c "ALTER DATABASE donelist_recovery RENAME TO donelist;"
```

7. **Resume Operations**

```bash
sudo systemctl start donelist-api
```

### Scenario 3: Accidental Data Deletion

**Situation**: Important data was accidentally deleted.

**Recovery Steps**:

1. **Identify When Data Was Deleted**

Check application logs to determine deletion time.

2. **Find Pre-Deletion Backup**

```bash
# List backups before deletion
aws s3 ls s3://donelist-backups/backups/2024/01/14/ --recursive
```

3. **Restore to Temporary Database**

```bash
./scripts/backup/restore.sh \
    --download s3://donelist-backups/backups/2024/01/14/postgres_full_donelist_20240114_020000.sql.gz \
    --database donelist_temp \
    --drop
```

4. **Extract Deleted Data**

```bash
pg_dump -h localhost -U donelist -d donelist_temp \
    -t deleted_table \
    --data-only \
    > deleted_data.sql
```

5. **Restore Data to Production**

```bash
psql -h localhost -U donelist -d donelist -f deleted_data.sql
```

6. **Cleanup**

```bash
psql -h localhost -U postgres -c "DROP DATABASE donelist_temp;"
rm deleted_data.sql
```

### Scenario 4: Server Migration

**Situation**: Moving to a new server.

**Recovery Steps**:

1. **On Old Server - Create Final Backup**

```bash
./scripts/backup/backup.sh
```

2. **Upload to S3**

Ensure backup is uploaded to S3 (automatic if configured).

3. **On New Server - Install Dependencies**

```bash
# Install PostgreSQL
sudo apt-get update
sudo apt-get install postgresql-15

# Install AWS CLI
sudo apt-get install awscli
```

4. **Configure New Server**

Copy configuration files to new server.

5. **Download and Restore**

```bash
./scripts/backup/restore.sh \
    --download s3://donelist-backups/backups/2024/01/15/postgres_full_donelist_20240115_120000.sql.gz \
    --database donelist \
    --drop
```

6. **Verify and Test**

```bash
# Test database connectivity
psql -h localhost -U donelist -d donelist -c "SELECT version();"

# Run application tests
./scripts/test.sh
```

7. **Update DNS/Load Balancer**

Point traffic to new server.

## Monitoring

### Health Check Indicators

| Indicator | Healthy | Warning | Critical |
|-----------|---------|---------|----------|
| Backup Age | < 24h | 24h - 48h | > 48h |
| Backup Size | > 10MB | 1MB - 10MB | < 1MB |
| Disk Usage | < 80% | 80% - 90% | > 90% |
| WAL Age | < 60min | 60min - 120min | > 120min |
| S3 Upload | Success | - | Failed |

### Alert Configuration

Configure webhooks in `.env`:

```bash
ALERT_WEBHOOK=https://hooks.slack.com/services/YOUR/WEBHOOK/URL
```

Or use email notifications (requires additional setup):

```bash
ALERT_EMAIL=ops@example.com
```

### Monitoring Dashboard

For production environments, integrate with:
- **Prometheus** - for metrics collection
- **Grafana** - for visualization
- **PagerDuty** - for on-call alerting
- **Datadog** - for comprehensive monitoring

## Troubleshooting

### Backup Fails

**Problem**: Backup script fails with connection error

**Solution**:
```bash
# Check database connectivity
psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "SELECT 1"

# Check pg_dump availability
which pg_dump

# Check disk space
df -h /var/backups/donelist
```

### S3 Upload Fails

**Problem**: Backup created but S3 upload fails

**Solution**:
```bash
# Test S3 connectivity
aws s3 ls s3://donelist-backups/

# Check AWS credentials
aws sts get-caller-identity

# Manual upload
aws s3 cp /var/backups/donelist/backup.sql.gz s3://donelist-backups/backups/
```

### Restore Fails

**Problem**: Database restore fails with errors

**Solution**:
```bash
# Verify backup integrity
pg_restore --list backup.sql.gz

# Check PostgreSQL logs
tail -f /var/log/postgresql/postgresql-15-main.log

# Try restore with more verbose output
./scripts/backup/restore.sh --file backup.sql.gz --drop --verify
```

### WAL Archiving Issues

**Problem**: WAL files not being archived

**Solution**:
```bash
# Check PostgreSQL archive_command
psql -U postgres -c "SHOW archive_command;"

# Check archive status
psql -U postgres -c "SELECT * FROM pg_stat_archiver;"

# Test archive script manually
./scripts/backup/wal-archive.sh /var/lib/postgresql/15/main/pg_wal/000000010000000000000001 000000010000000000000001
```

## Best Practices

1. **Test Restores Regularly**: Perform quarterly restore tests to verify backups work
2. **Monitor Backup Size**: Sudden size changes may indicate issues
3. **Keep Multiple Copies**: Follow the 3-2-1 rule (3 copies, 2 different media, 1 offsite)
4. **Encrypt Sensitive Data**: Enable encryption for compliance requirements
5. **Document Procedures**: Keep runbooks updated
6. **Automate Everything**: Use cron jobs and monitoring
7. **Version Control**: Keep backup scripts in git
8. **Test Disaster Recovery**: Run DR drills annually

## Security Considerations

- Store database passwords in environment variables, not in scripts
- Use IAM roles for S3 access when possible
- Encrypt backups at rest and in transit
- Restrict access to backup files (chmod 600)
- Audit backup access logs
- Rotate S3 access keys regularly
- Use separate S3 buckets for different environments

## Performance Optimization

- Use `--format=custom` for faster restores
- Enable compression to reduce storage costs
- Schedule backups during low-traffic periods
- Use incremental backups (WAL archiving) for large databases
- Consider parallel pg_dump for very large databases
- Monitor backup duration trends

## Maintenance

### Weekly Tasks
- Review backup logs
- Check disk space trends
- Verify S3 uploads

### Monthly Tasks
- Test restore procedure
- Review retention policy
- Audit backup sizes
- Update documentation

### Quarterly Tasks
- Full disaster recovery drill
- Review and update procedures
- Performance optimization review
- Security audit

## Support

For issues or questions:
- Check logs: `/var/log/donelist-backup.log`
- Review monitoring output
- Consult PostgreSQL documentation
- Contact: ops@example.com

## References

- [PostgreSQL Backup Documentation](https://www.postgresql.org/docs/current/backup.html)
- [AWS S3 CLI Documentation](https://docs.aws.amazon.com/cli/latest/reference/s3/)
- [Point-in-Time Recovery](https://www.postgresql.org/docs/current/continuous-archiving.html)
