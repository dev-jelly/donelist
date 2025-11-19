# Disaster Recovery Runbook

## Table of Contents

- [Emergency Contacts](#emergency-contacts)
- [Quick Reference](#quick-reference)
- [Recovery Scenarios](#recovery-scenarios)
- [Detailed Procedures](#detailed-procedures)
- [Verification Checklist](#verification-checklist)
- [Post-Recovery Tasks](#post-recovery-tasks)

## Emergency Contacts

| Role | Name | Contact | Availability |
|------|------|---------|--------------|
| Database Lead | TBD | TBD | 24/7 |
| DevOps Lead | TBD | TBD | 24/7 |
| On-Call Engineer | TBD | TBD | 24/7 |
| Backup Contact | TBD | TBD | Business Hours |

**Emergency Escalation**: If primary contact doesn't respond within 15 minutes, escalate to next level.

## Quick Reference

### Recovery Time Objectives (RTO)

| Scenario | Target RTO | Expected Data Loss (RPO) |
|----------|-----------|--------------------------|
| Full Database Restore | 30 minutes | Last backup (max 24h) |
| Point-in-Time Recovery | 1-2 hours | Seconds to minutes |
| Single Table Recovery | 15-30 minutes | Last backup (max 24h) |
| Corruption Fix | 2-4 hours | Minutes to hours |

### Critical Commands

```bash
# Quick health check
./scripts/backup/monitor.sh

# List recent backups
ls -lht /var/backups/donelist/*.sql.gz | head -5

# Check S3 backups
aws s3 ls s3://donelist-backups/backups/ --recursive | tail -10

# Emergency restore (use with caution!)
./scripts/backup/restore.sh --file <backup-file> --drop --verify

# PITR recovery
./scripts/backup/pitr-restore.sh --base-backup <file> --target-time "YYYY-MM-DD HH:MM:SS"
```

## Recovery Scenarios

### Scenario Matrix

| Scenario | Complexity | RTO | RPO | Procedure |
|----------|-----------|-----|-----|-----------|
| 1. Complete Database Loss | High | 30m | 24h | [Procedure 1](#procedure-1-complete-database-loss) |
| 2. Data Corruption | High | 2h | 1h | [Procedure 2](#procedure-2-data-corruption-with-pitr) |
| 3. Accidental Deletion | Medium | 1h | 24h | [Procedure 3](#procedure-3-accidental-data-deletion) |
| 4. Table Corruption | Medium | 30m | 24h | [Procedure 4](#procedure-4-single-table-recovery) |
| 5. Server Migration | Low | 1h | 0m | [Procedure 5](#procedure-5-server-migration) |
| 6. Ransomware Attack | Critical | 4h | 48h | [Procedure 6](#procedure-6-security-incident-recovery) |
| 7. WAL Archive Full | Medium | 15m | 0m | [Procedure 7](#procedure-7-wal-archive-management) |

## Detailed Procedures

### Procedure 1: Complete Database Loss

**Situation**: Database server crashed, data directory corrupted, or complete data loss.

**Impact**: High - Complete service outage

**Prerequisites**:
- Access to backup files or S3 bucket
- Database credentials
- Root/sudo access to database server

#### Step-by-Step Recovery

**Phase 1: Assessment (5 minutes)**

1. **Confirm the situation**
   ```bash
   # Try to connect to database
   psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME

   # Check PostgreSQL service status
   systemctl status postgresql

   # Check data directory
   ls -la /var/lib/postgresql/15/main/
   ```

2. **Notify stakeholders**
   - Send alert to #incidents channel
   - Update status page
   - Start incident log

3. **Identify latest backup**
   ```bash
   # Local backups
   ls -lht /var/backups/donelist/*.sql.gz | head -5

   # S3 backups (with timestamps)
   aws s3 ls s3://donelist-backups/backups/ --recursive | grep ".sql.gz" | sort | tail -10
   ```

**Phase 2: Preparation (5 minutes)**

4. **Prepare environment**
   ```bash
   # Stop application servers
   systemctl stop donelist-api

   # Ensure PostgreSQL is running
   systemctl start postgresql
   systemctl status postgresql

   # Verify connectivity
   psql -h localhost -p 5432 -U postgres -c "SELECT version();"
   ```

5. **Document backup details**
   ```bash
   # Record backup information
   BACKUP_FILE="/var/backups/donelist/postgres_full_donelist_YYYYMMDD_HHMMSS.sql.gz"
   BACKUP_TIME="YYYY-MM-DD HH:MM:SS"  # From filename

   # Or download from S3
   S3_BACKUP="s3://donelist-backups/backups/YYYY/MM/DD/postgres_full_donelist_YYYYMMDD_HHMMSS.sql.gz"
   ```

**Phase 3: Recovery (15-20 minutes)**

6. **Perform restore**
   ```bash
   # Option A: From local backup
   cd /path/to/donelist/server
   ./scripts/backup/restore.sh \
       --file "$BACKUP_FILE" \
       --database donelist \
       --drop \
       --verify

   # Option B: From S3
   ./scripts/backup/restore.sh \
       --download "$S3_BACKUP" \
       --database donelist \
       --drop \
       --verify
   ```

7. **Monitor restore progress**
   ```bash
   # Watch restore log (in another terminal)
   tail -f /var/backups/donelist/restore_*.log

   # Monitor PostgreSQL logs
   tail -f /var/log/postgresql/postgresql-15-main.log
   ```

**Phase 4: Verification (5 minutes)**

8. **Verify database restoration**
   ```bash
   # Check database exists
   psql -h localhost -U donelist -d donelist -c "\dt"

   # Verify table counts
   psql -h localhost -U donelist -d donelist -c "
   SELECT
       schemaname,
       tablename,
       n_live_tup as row_count
   FROM pg_stat_user_tables
   ORDER BY n_live_tup DESC
   LIMIT 10;
   "

   # Check latest data
   psql -h localhost -U donelist -d donelist -c "
   SELECT created_at, email FROM users ORDER BY created_at DESC LIMIT 5;
   "
   ```

9. **Verify data integrity**
   ```bash
   # Run database consistency checks
   psql -h localhost -U donelist -d donelist -c "
   SELECT pg_database.datname,
          pg_size_pretty(pg_database_size(pg_database.datname)) AS size
   FROM pg_database;
   "

   # Check for corrupted indexes
   psql -h localhost -U donelist -d donelist -c "
   SELECT * FROM pg_indexes WHERE schemaname = 'public';
   "
   ```

**Phase 5: Service Restoration (5 minutes)**

10. **Start application services**
    ```bash
    # Start API server
    systemctl start donelist-api
    systemctl status donelist-api

    # Verify health endpoint
    curl http://localhost:8080/health

    # Check application logs
    journalctl -u donelist-api -f
    ```

11. **Perform smoke tests**
    ```bash
    # Test authentication
    curl -X POST http://localhost:8080/api/v1/auth/login \
         -H "Content-Type: application/json" \
         -d '{"email":"test@example.com","password":"test"}'

    # Test data retrieval
    curl http://localhost:8080/api/v1/users/me \
         -H "Authorization: Bearer $TOKEN"
    ```

**Phase 6: Post-Recovery (Ongoing)**

12. **Update stakeholders**
    - Update status page
    - Send all-clear notification
    - Schedule post-mortem

13. **Document recovery**
    ```bash
    # Create incident report
    cat > /tmp/incident_report_$(date +%Y%m%d).md <<EOF
    # Database Recovery Incident Report

    Date: $(date)
    Severity: High

    ## Timeline
    - Incident detected: [TIME]
    - Recovery started: [TIME]
    - Service restored: [TIME]
    - Total downtime: [DURATION]

    ## Root Cause
    [DESCRIPTION]

    ## Recovery Method
    - Backup used: $BACKUP_FILE
    - Data loss: [ESTIMATE]

    ## Action Items
    - [ ] Review backup frequency
    - [ ] Update runbook with learnings
    - [ ] Test recovery procedures
    EOF
    ```

14. **Verify backup system**
    ```bash
    # Ensure backups resume normally
    ./scripts/backup/backup.sh

    # Check monitoring
    ./scripts/backup/monitor.sh
    ```

---

### Procedure 2: Data Corruption with PITR

**Situation**: Data corruption detected, need to restore to a specific point in time before corruption.

**Impact**: Medium-High - Service degradation or partial data loss

#### Step-by-Step Recovery

**Phase 1: Detection and Analysis (10-15 minutes)**

1. **Identify corruption**
   ```bash
   # Check for errors in application logs
   journalctl -u donelist-api --since "1 hour ago" | grep -i error

   # Check PostgreSQL logs
   tail -100 /var/log/postgresql/postgresql-15-main.log | grep -i "corrupt\|error"

   # Identify affected tables
   psql -h localhost -U donelist -d donelist -c "
   SELECT tablename, last_vacuum, last_autovacuum, last_analyze
   FROM pg_stat_user_tables
   WHERE last_autovacuum IS NULL OR last_vacuum IS NULL;
   "
   ```

2. **Determine corruption timeline**
   ```bash
   # Review audit logs to identify when corruption started
   psql -h localhost -U donelist -d donelist -c "
   SELECT * FROM audit_logs
   WHERE action LIKE '%corruption%'
   ORDER BY created_at DESC
   LIMIT 20;
   "

   # Check last known good data timestamp
   # Example: Find last successful daily report
   ```

3. **Calculate recovery point**
   ```bash
   # Identify target recovery time (before corruption)
   RECOVERY_TIME="2024-01-15 14:30:00"  # Example: 30 minutes before corruption

   # Find base backup before corruption
   ls -lt /var/backups/donelist/*.sql.gz | grep -B5 "Jan 15"
   ```

**Phase 2: Preparation (10 minutes)**

4. **Stop application writes**
   ```bash
   # Put application in read-only mode
   psql -h localhost -U donelist -d donelist -c "
   ALTER DATABASE donelist SET default_transaction_read_only = on;
   "

   # Or stop application
   systemctl stop donelist-api
   ```

5. **Create emergency backup of current state**
   ```bash
   # Backup corrupted database for forensics
   ./scripts/backup/backup.sh

   # Rename to indicate corruption
   mv /var/backups/donelist/postgres_full_donelist_*.sql.gz \
      /var/backups/donelist/corrupted_backup_$(date +%Y%m%d_%H%M%S).sql.gz
   ```

6. **Verify WAL files availability**
   ```bash
   # Check WAL archive
   ls -lht /var/backups/donelist/wal/ | head -20

   # Download from S3 if needed
   aws s3 sync s3://donelist-backups/wal/ /var/backups/donelist/wal/

   # Verify WAL coverage
   WAL_COUNT=$(ls /var/backups/donelist/wal/ | wc -l)
   echo "Available WAL files: $WAL_COUNT"
   ```

**Phase 3: PITR Execution (30-60 minutes)**

7. **Identify base backup**
   ```bash
   # Find backup taken before corruption
   BASE_BACKUP="/var/backups/donelist/postgres_full_donelist_20240115_020000.sql.gz"

   # Verify backup is before corruption time
   BACKUP_DATE=$(basename "$BASE_BACKUP" | sed 's/postgres_full_donelist_\([0-9_]*\).sql.gz/\1/')
   echo "Using base backup from: $BACKUP_DATE"
   ```

8. **Perform PITR recovery to temporary database**
   ```bash
   # Recover to temporary database first
   ./scripts/backup/pitr-restore.sh \
       --base-backup "$BASE_BACKUP" \
       --target-time "$RECOVERY_TIME" \
       --database donelist_recovery \
       --verify
   ```

9. **Monitor recovery progress**
   ```bash
   # Watch recovery log
   tail -f /var/backups/donelist/pitr_restore_*.log

   # Check PostgreSQL recovery progress
   psql -h localhost -U donelist -d donelist_recovery -c "
   SELECT pg_is_in_recovery(),
          pg_last_wal_receive_lsn(),
          pg_last_wal_replay_lsn();
   "
   ```

**Phase 4: Verification (15-20 minutes)**

10. **Verify recovered data**
    ```bash
    # Connect to recovered database
    psql -h localhost -U donelist -d donelist_recovery

    # Verify data at recovery point
    SELECT COUNT(*) FROM users;
    SELECT MAX(created_at) FROM users;
    SELECT MAX(updated_at) FROM check_ins;

    # Compare with corrupted database
    # Check critical records
    SELECT * FROM users WHERE id IN (1, 2, 3);
    ```

11. **Run data integrity checks**
    ```bash
    psql -h localhost -U donelist -d donelist_recovery -c "
    -- Check for referential integrity
    SELECT conrelid::regclass AS table_name,
           conname AS constraint_name,
           pg_get_constraintdef(oid)
    FROM pg_constraint
    WHERE contype = 'f'
    AND connamespace = 'public'::regnamespace;

    -- Verify all constraints
    SELECT * FROM pg_constraint WHERE conrelid::regclass::text LIKE 'public.%';
    "
    ```

12. **Test application functionality**
    ```bash
    # Update .env to point to recovery database temporarily
    # DB_NAME=donelist_recovery

    # Start application in test mode
    SERVER_ENV=test DB_NAME=donelist_recovery ./bin/donelist-api

    # Run smoke tests
    # - User authentication
    # - Check-in operations
    # - Data retrieval
    ```

**Phase 5: Cutover (10 minutes)**

13. **Backup corrupted database**
    ```bash
    # Rename corrupted database for later analysis
    psql -h localhost -U postgres -c "
    ALTER DATABASE donelist RENAME TO donelist_corrupted_$(date +%Y%m%d);
    "
    ```

14. **Promote recovered database**
    ```bash
    # Rename recovered database to production name
    psql -h localhost -U postgres -c "
    ALTER DATABASE donelist_recovery RENAME TO donelist;
    "

    # Verify
    psql -h localhost -U donelist -d donelist -c "\l donelist"
    ```

15. **Restart application**
    ```bash
    # Update .env back to production settings
    # DB_NAME=donelist

    # Start application
    systemctl start donelist-api

    # Verify health
    curl http://localhost:8080/health
    ```

**Phase 6: Post-Recovery (Ongoing)**

16. **Analyze corrupted database**
    ```bash
    # Export corrupted data for analysis
    pg_dump -h localhost -U donelist -d donelist_corrupted_* \
            --schema-only \
            > /tmp/corrupted_schema.sql

    # Identify corruption cause
    # - Check for disk errors
    # - Review application logs
    # - Analyze database logs
    ```

17. **Document recovery**
    ```bash
    cat > /tmp/pitr_recovery_report_$(date +%Y%m%d).md <<EOF
    # PITR Recovery Report

    ## Incident Details
    - Detection time: [TIME]
    - Corruption type: [TYPE]
    - Affected tables: [TABLES]

    ## Recovery Details
    - Base backup: $BASE_BACKUP
    - Recovery target: $RECOVERY_TIME
    - Data loss: [ESTIMATE]
    - Recovery duration: [DURATION]

    ## Verification Results
    - Tables recovered: [COUNT]
    - Records recovered: [COUNT]
    - Integrity checks: PASSED/FAILED

    ## Root Cause Analysis
    [ANALYSIS]

    ## Preventive Measures
    - [ ] Action item 1
    - [ ] Action item 2
    EOF
    ```

---

### Procedure 3: Accidental Data Deletion

**Situation**: Important data was accidentally deleted (users, check-ins, etc.).

**Impact**: Medium - Partial data loss

#### Quick Recovery Steps

1. **Immediate Response**
   ```bash
   # Stop application to prevent further changes
   systemctl stop donelist-api

   # Identify deletion time
   # Check audit logs or application logs
   ```

2. **Find pre-deletion backup**
   ```bash
   # List recent backups
   ls -lht /var/backups/donelist/*.sql.gz | head -10

   # Identify backup taken before deletion
   BACKUP_FILE="[backup-before-deletion]"
   ```

3. **Restore to temporary database**
   ```bash
   ./scripts/backup/restore.sh \
       --file "$BACKUP_FILE" \
       --database donelist_temp \
       --drop
   ```

4. **Extract deleted data**
   ```bash
   # Dump only the deleted table/data
   pg_dump -h localhost -U donelist -d donelist_temp \
           -t users \
           -t check_ins \
           --data-only \
           --inserts \
           > /tmp/deleted_data.sql

   # Or export specific records
   psql -h localhost -U donelist -d donelist_temp -c "
   COPY (
       SELECT * FROM users WHERE id IN (1, 2, 3)
   ) TO '/tmp/deleted_users.csv' CSV HEADER;
   "
   ```

5. **Restore data to production**
   ```bash
   # Option A: Using SQL file
   psql -h localhost -U donelist -d donelist -f /tmp/deleted_data.sql

   # Option B: Using CSV
   psql -h localhost -U donelist -d donelist -c "
   COPY users FROM '/tmp/deleted_users.csv' CSV HEADER;
   "
   ```

6. **Verify and cleanup**
   ```bash
   # Verify restored data
   psql -h localhost -U donelist -d donelist -c "
   SELECT COUNT(*) FROM users;
   "

   # Drop temporary database
   dropdb -h localhost -U postgres donelist_temp

   # Restart application
   systemctl start donelist-api
   ```

---

### Procedure 4: Single Table Recovery

**Situation**: Specific table is corrupted or needs restoration.

#### Quick Recovery Steps

1. **Restore to temporary database**
   ```bash
   ./scripts/backup/restore.sh \
       --file "[latest-backup]" \
       --database donelist_temp \
       --drop
   ```

2. **Export table from temporary database**
   ```bash
   pg_dump -h localhost -U donelist -d donelist_temp \
           -t table_name \
           --schema-and-data \
           > /tmp/table_backup.sql
   ```

3. **Backup current table**
   ```bash
   pg_dump -h localhost -U donelist -d donelist \
           -t table_name \
           > /tmp/table_corrupted.sql
   ```

4. **Drop and restore table**
   ```bash
   psql -h localhost -U donelist -d donelist -c "
   DROP TABLE IF EXISTS table_name CASCADE;
   "

   psql -h localhost -U donelist -d donelist -f /tmp/table_backup.sql
   ```

5. **Verify and cleanup**
   ```bash
   # Verify table
   psql -h localhost -U donelist -d donelist -c "\d table_name"

   # Drop temp database
   dropdb -h localhost -U postgres donelist_temp
   ```

---

### Procedure 5: Server Migration

**Situation**: Moving database to new server.

See main [README.md](README.md) Scenario 4 for detailed steps.

---

### Procedure 6: Security Incident Recovery

**Situation**: Ransomware, unauthorized access, or security breach.

**Impact**: Critical - Potential complete data loss or compromise

#### Step-by-Step Recovery

1. **Immediate Isolation**
   ```bash
   # Disconnect from network
   sudo ifconfig eth0 down

   # Stop all services
   systemctl stop donelist-api
   systemctl stop postgresql
   systemctl stop nginx
   ```

2. **Secure Clean System**
   - Provision new, clean server
   - Install fresh OS and dependencies
   - Apply all security patches

3. **Identify Clean Backup**
   ```bash
   # Find backup before security incident
   # Use S3 backups (more likely to be safe)
   aws s3 ls s3://donelist-backups/backups/ --recursive | tail -20

   # Download backup from before incident
   CLEAN_BACKUP="[backup-from-before-incident]"
   ```

4. **Restore to Clean System**
   ```bash
   # On new, clean server
   ./scripts/backup/restore.sh \
       --download "$CLEAN_BACKUP" \
       --database donelist \
       --drop \
       --verify
   ```

5. **Security Hardening**
   ```bash
   # Rotate all credentials
   # - Database passwords
   # - API keys
   # - JWT secrets
   # - SSH keys

   # Update firewall rules
   # Enable security monitoring
   # Review access logs
   ```

6. **Verify No Compromise**
   - Scan database for suspicious data
   - Review user accounts
   - Check for backdoors
   - Audit all changes

---

### Procedure 7: WAL Archive Management

**Situation**: WAL archive directory is full or growing too large.

#### Quick Fix

1. **Check WAL archive size**
   ```bash
   du -sh /var/backups/donelist/wal/
   ls -lht /var/backups/donelist/wal/ | head -20
   ```

2. **Archive old WAL files to S3**
   ```bash
   # Upload to S3
   aws s3 sync /var/backups/donelist/wal/ \
              s3://donelist-backups/wal-archive/$(date +%Y/%m)/ \
              --storage-class GLACIER
   ```

3. **Clean local WAL files**
   ```bash
   # Keep last 7 days only
   find /var/backups/donelist/wal/ -name "0*" -type f -mtime +7 -delete

   # Verify space recovered
   df -h /var/backups/donelist/
   ```

4. **Monitor ongoing**
   ```bash
   # Add to cron for automated cleanup
   echo "0 2 * * * find /var/backups/donelist/wal/ -name '0*' -type f -mtime +7 -delete" | crontab -
   ```

---

## Verification Checklist

After any recovery procedure, complete this checklist:

### Database Verification

- [ ] Database accessible via psql
- [ ] All expected tables present
- [ ] Row counts match expectations
- [ ] Indexes are valid
- [ ] Foreign key constraints intact
- [ ] Recent data visible
- [ ] No corruption errors in logs

### Application Verification

- [ ] API server starts successfully
- [ ] Health endpoint responds
- [ ] Authentication works
- [ ] User can log in
- [ ] Data operations work (CRUD)
- [ ] WebSocket connections work
- [ ] No errors in application logs

### Performance Verification

- [ ] Query response times normal
- [ ] Database connections stable
- [ ] No blocking queries
- [ ] Cache warming complete
- [ ] Indexes being used

### Monitoring Verification

- [ ] Backup monitoring active
- [ ] Alerts configured
- [ ] Metrics collecting
- [ ] Logs being written
- [ ] Dashboard showing green

---

## Post-Recovery Tasks

### Immediate (Within 1 hour)

1. **Update status page** - Inform users service is restored
2. **Notify stakeholders** - Send recovery summary
3. **Document incident** - Record timeline and actions
4. **Verify backups** - Ensure backup system is operational
5. **Monitor closely** - Watch for any issues

### Short-term (Within 24 hours)

1. **Schedule post-mortem** - Team meeting to review
2. **Update runbook** - Add learnings from recovery
3. **Test failover** - Verify recovery procedures work
4. **Review monitoring** - Ensure alerts worked as expected
5. **Communicate lessons** - Share with team

### Long-term (Within 1 week)

1. **Conduct DR drill** - Test recovery on non-production
2. **Update documentation** - Reflect any process changes
3. **Review backup strategy** - Adjust frequency/retention
4. **Security audit** - If incident was security-related
5. **Implement improvements** - From post-mortem action items

---

## Recovery Rehearsal Schedule

| Rehearsal Type | Frequency | Next Due |
|---------------|-----------|----------|
| Full Database Restore | Quarterly | TBD |
| PITR Recovery | Semi-annually | TBD |
| Single Table Recovery | Monthly | TBD |
| Backup Verification | Weekly | Automated |
| DR Drill (Full) | Annually | TBD |

---

## Success Criteria

A recovery is considered successful when:

1. ✅ Database is accessible and operational
2. ✅ Application services are running
3. ✅ Data integrity verified (no corruption)
4. ✅ Users can access the system
5. ✅ No critical errors in logs
6. ✅ Performance is within acceptable range
7. ✅ Monitoring shows all systems green
8. ✅ Backup system is operational

---

## Continuous Improvement

After each recovery:

1. Update this runbook with lessons learned
2. Add any new commands or procedures discovered
3. Note time estimates for accuracy
4. Document any issues encountered
5. Share knowledge with team

---

**Last Updated**: 2024-01-15
**Version**: 1.0
**Maintained By**: DevOps Team
