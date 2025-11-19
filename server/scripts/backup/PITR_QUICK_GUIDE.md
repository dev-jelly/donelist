# Point-In-Time Recovery (PITR) Quick Guide

## What is PITR?

Point-In-Time Recovery allows you to restore a database to any specific moment in time, not just to when a backup was taken. This is achieved by:

1. **Base Backup**: A full database backup taken periodically
2. **WAL (Write-Ahead Log) Files**: Continuous log of all database changes
3. **Recovery Process**: Replaying WAL files on top of base backup to reach desired point in time

## When to Use PITR

- **Data corruption detected**: Restore to just before corruption occurred
- **Accidental deletion**: Restore to moment before data was deleted
- **Bad migration**: Roll back to before migration was executed
- **Testing scenarios**: Create database snapshots at specific points
- **Compliance**: Meet point-in-time recovery requirements

## Quick Start

### 1. Setup PITR (One-time)

```bash
# Run as root/sudo
sudo ./scripts/backup/setup-pitr.sh
```

This script will:
- Create WAL archive directory
- Configure PostgreSQL for WAL archiving
- Set up archive command
- Create initial base backup
- Configure monitoring

### 2. Verify PITR is Working

```bash
# Check WAL archiving status
psql -U postgres -c "SELECT * FROM pg_stat_archiver;"

# Check WAL files are being created
ls -lht /var/backups/donelist/wal/ | head -10

# Monitor backup system
./scripts/backup/monitor.sh
```

### 3. Perform PITR Recovery

```bash
# Basic PITR to specific time
./scripts/backup/pitr-restore.sh \
    --base-backup /path/to/base/backup.sql.gz \
    --target-time "2024-01-15 14:30:00" \
    --database donelist_recovery

# PITR to transaction ID
./scripts/backup/pitr-restore.sh \
    --base-backup backup.sql.gz \
    --target-xid 12345678 \
    --database donelist_recovery

# PITR to named restore point
./scripts/backup/pitr-restore.sh \
    --base-backup backup.sql.gz \
    --target-name "before_migration" \
    --database donelist_recovery
```

## Recovery Time Objectives

| Scenario | Data Loss (RPO) | Recovery Time (RTO) |
|----------|-----------------|---------------------|
| Full Restore (no PITR) | Up to 24 hours | 30 minutes |
| PITR Recovery | Seconds to minutes | 1-2 hours |
| Streaming Replication | Near-zero | 5-15 minutes |

## Common Recovery Scenarios

### Scenario 1: Corruption Detected at 3:00 PM

```bash
# Find base backup from before corruption (e.g., 2 AM daily backup)
BASE_BACKUP="/var/backups/donelist/postgres_full_donelist_20240115_020000.sql.gz"

# Recover to 2:55 PM (5 minutes before corruption)
./scripts/backup/pitr-restore.sh \
    --base-backup "$BASE_BACKUP" \
    --target-time "2024-01-15 14:55:00" \
    --database donelist_recovery \
    --verify

# Verify recovered database
psql -h localhost -U donelist -d donelist_recovery -c "
SELECT MAX(created_at) FROM users;
SELECT MAX(updated_at) FROM check_ins;
"

# If data looks good, promote to production
psql -h localhost -U postgres -c "
ALTER DATABASE donelist RENAME TO donelist_old;
ALTER DATABASE donelist_recovery RENAME TO donelist;
"

# Restart application
systemctl restart donelist-api
```

### Scenario 2: Accidental Data Deletion

```bash
# User deleted at 11:30 AM, discovered at 2:00 PM

# Option A: Full database recovery to 11:29 AM
./scripts/backup/pitr-restore.sh \
    --base-backup morning_backup.sql.gz \
    --target-time "2024-01-15 11:29:00" \
    --database donelist_temp

# Extract deleted data
psql -h localhost -U donelist -d donelist_temp -c "
COPY (SELECT * FROM users WHERE id = 12345)
TO '/tmp/deleted_user.csv' CSV HEADER;
"

# Restore to production
psql -h localhost -U donelist -d donelist -c "
COPY users FROM '/tmp/deleted_user.csv' CSV HEADER;
"

# Cleanup
dropdb donelist_temp
```

### Scenario 3: Testing Migration

```bash
# Before migration, create named restore point
psql -h localhost -U donelist -d donelist -c "
SELECT pg_create_restore_point('before_v2_migration');
"

# Run migration
./run-migration.sh

# If migration fails or has issues, recover
./scripts/backup/pitr-restore.sh \
    --base-backup latest_backup.sql.gz \
    --target-name "before_v2_migration" \
    --database donelist
```

## PITR Workflow Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                      Time Timeline                           │
└─────────────────────────────────────────────────────────────┘

2:00 AM          10:00 AM       2:00 PM        2:50 PM   3:00 PM
   │                │              │              │         │
   │                │              │              │         │
   ▼                ▼              ▼              ▼         ▼
[Backup]      [WAL Files]    [WAL Files]    [WAL Files] [Corrupt]

Recovery Options:
1. Restore to 2:00 AM   ──────▶ Use backup only (no WAL replay)
2. Restore to 10:00 AM  ──────▶ Backup + WAL replay to 10:00
3. Restore to 2:50 PM   ──────▶ Backup + WAL replay to 2:50 (5 min before corruption)

Data Loss:
- Option 1: 13 hours of data
- Option 2: 5 hours of data
- Option 3: 10 minutes of data ✅ Best option
```

## Best Practices

### 1. Regular Base Backups

```bash
# Schedule daily backups
0 2 * * * /path/to/scripts/backup/backup.sh

# Keep multiple base backups
RETENTION_DAYS=7  # At least a week
```

### 2. Monitor WAL Archive

```bash
# Check archiver is working
psql -U postgres -c "
SELECT
    archived_count,
    failed_count,
    last_archived_time,
    last_failed_time
FROM pg_stat_archiver;
"

# Alert if archiving fails
if [ failed_count -gt 0 ]; then
    send_alert "WAL archiving failing!"
fi
```

### 3. Test Recovery Regularly

```bash
# Quarterly recovery drills
./scripts/backup/recovery-rehearsal.sh

# Document results
cat /var/backups/donelist/rehearsal_report_*.md
```

### 4. Create Named Restore Points

```bash
# Before risky operations
psql -c "SELECT pg_create_restore_point('before_major_update');"

# After successful milestones
psql -c "SELECT pg_create_restore_point('after_data_migration');"
```

### 5. Maintain WAL Archive Health

```bash
# Monitor WAL disk space
df -h /var/backups/donelist/wal/

# Archive old WALs to S3
aws s3 sync /var/backups/donelist/wal/ \
           s3://donelist-backups/wal-archive/

# Clean local WALs older than 7 days
find /var/backups/donelist/wal/ -mtime +7 -delete
```

## Troubleshooting

### Issue: WAL Files Not Being Archived

```bash
# Check PostgreSQL config
psql -U postgres -c "SHOW archive_mode;"
psql -U postgres -c "SHOW archive_command;"

# Check archive script permissions
ls -l /path/to/scripts/backup/wal-archive.sh
# Should be: -rwxr-xr-x

# Check WAL directory permissions
ls -ld /var/backups/donelist/wal/
# Should be: drwx------ postgres postgres

# Check PostgreSQL logs
tail -f /var/log/postgresql/postgresql-15-main.log | grep archive
```

### Issue: PITR Restore Fails

```bash
# Verify base backup integrity
pg_restore --list backup.sql.gz

# Check WAL files availability
ls /var/backups/donelist/wal/ | wc -l

# Verify WAL files cover recovery period
# Use pg_waldump to inspect (PostgreSQL 10+)
pg_waldump /var/backups/donelist/wal/000000010000000000000001

# Try recovery with --dry-run first
./scripts/backup/pitr-restore.sh \
    --base-backup backup.sql.gz \
    --target-time "2024-01-15 14:30:00" \
    --dry-run
```

### Issue: Recovery Takes Too Long

```bash
# Check WAL replay progress
psql -c "
SELECT
    pg_is_in_recovery(),
    pg_last_wal_receive_lsn(),
    pg_last_wal_replay_lsn(),
    pg_last_xact_replay_timestamp();
"

# Monitor recovery progress
while true; do
    psql -c "SELECT pg_last_xact_replay_timestamp();"
    sleep 10
done
```

## Architecture Overview

```
┌───────────────────────────────────────────────────────────┐
│                   Production Database                      │
│                                                            │
│  ┌──────────────┐         ┌─────────────────────┐        │
│  │   Database   │────────▶│   WAL Files         │        │
│  │   (Active)   │  Write  │   (pg_wal/)         │        │
│  └──────────────┘         └──────────┬──────────┘        │
│                                      │                     │
└──────────────────────────────────────┼─────────────────────┘
                                       │
                                       │ Archive
                                       ▼
                         ┌────────────────────────┐
                         │   WAL Archive          │
                         │   /var/backups/.../wal │
                         └───────────┬────────────┘
                                     │
                                     │ Upload
                                     ▼
                         ┌────────────────────────┐
                         │   S3 Bucket            │
                         │   Long-term Storage    │
                         └────────────────────────┘

┌───────────────────────────────────────────────────────────┐
│                   Recovery Process                         │
│                                                            │
│  ┌──────────────┐    ┌──────────────┐    ┌────────────┐ │
│  │ Base Backup  │───▶│  Restore     │───▶│ Apply WAL  │ │
│  │  (2 AM)      │    │  Database    │    │ Files      │ │
│  └──────────────┘    └──────────────┘    └─────┬──────┘ │
│                                                  │         │
│                                                  ▼         │
│                                        ┌─────────────────┐│
│                                        │ Database at     ││
│                                        │ Target Time     ││
│                                        └─────────────────┘│
└───────────────────────────────────────────────────────────┘
```

## Cheat Sheet

```bash
# Setup PITR
sudo ./scripts/backup/setup-pitr.sh

# Create base backup
./scripts/backup/backup.sh

# Check PITR status
psql -U postgres -c "SELECT * FROM pg_stat_archiver;"

# Create named restore point
psql -c "SELECT pg_create_restore_point('checkpoint_name');"

# PITR to specific time
./scripts/backup/pitr-restore.sh -b backup.sql.gz -t "YYYY-MM-DD HH:MM:SS"

# PITR to transaction ID
./scripts/backup/pitr-restore.sh -b backup.sql.gz -x 12345678

# PITR to restore point
./scripts/backup/pitr-restore.sh -b backup.sql.gz -n "checkpoint_name"

# Test recovery
./scripts/backup/recovery-rehearsal.sh

# Monitor backups
./scripts/backup/monitor.sh

# View recovery runbook
cat ./scripts/backup/RECOVERY_RUNBOOK.md
```

## Resources

- **Full Documentation**: [README.md](README.md)
- **Recovery Runbook**: [RECOVERY_RUNBOOK.md](RECOVERY_RUNBOOK.md)
- **PostgreSQL PITR Docs**: https://www.postgresql.org/docs/current/continuous-archiving.html
- **Backup Scripts**: [scripts/backup/](.)

## Support

For issues or questions:
- Review logs: `/var/backups/donelist/pitr_restore_*.log`
- Check PostgreSQL logs: `/var/log/postgresql/`
- Run diagnostics: `./scripts/backup/monitor.sh`
- Consult runbook: [RECOVERY_RUNBOOK.md](RECOVERY_RUNBOOK.md)

---

**Last Updated**: 2024-01-15
**Version**: 1.0
**Maintained By**: DevOps Team
