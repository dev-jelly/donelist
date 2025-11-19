# Backup and Recovery System Documentation

## Quick Start Guide

### Immediate Actions After Setup

1. **Create Your First Backup**
   ```bash
   cd scripts/backup
   ./backup.sh
   ```

2. **Verify Backup Works**
   ```bash
   ./monitor.sh
   ```

3. **Test Restore (on non-production)**
   ```bash
   ./restore.sh --file /var/backups/donelist/latest_backup.sql.gz --database donelist_test --drop
   ```

## Production Deployment Checklist

- [ ] Configure S3 bucket and credentials
- [ ] Set up cron jobs for automated backups
- [ ] Configure WAL archiving in PostgreSQL
- [ ] Set up monitoring webhooks
- [ ] Test full disaster recovery procedure
- [ ] Document recovery time objectives (RTO)
- [ ] Document recovery point objectives (RPO)
- [ ] Train team on restore procedures
- [ ] Create runbook for on-call engineers

## Recovery Time Objectives (RTO)

| Scenario | Target RTO | Actual RTO (Tested) |
|----------|-----------|---------------------|
| Full database restore | 30 minutes | TBD |
| Point-in-time recovery | 1 hour | TBD |
| Data corruption recovery | 45 minutes | TBD |
| Server migration | 2 hours | TBD |

## Recovery Point Objectives (RPO)

| Backup Type | RPO |
|-------------|-----|
| Full daily backups | 24 hours |
| WAL archiving | < 5 minutes |
| S3 replication | < 1 hour |

## Disaster Recovery Scenarios

### Critical: Complete Data Center Failure

**Scenario**: Primary data center is completely offline

**Steps**:
1. Spin up new infrastructure in alternate region
2. Download latest backup from S3
3. Restore database
4. Apply latest WAL files
5. Update DNS to point to new infrastructure
6. Verify application functionality

**Estimated Time**: 2-4 hours

### High: Database Corruption

**Scenario**: Database has corrupted data but server is online

**Steps**:
1. Identify corruption point
2. Create snapshot of current state
3. Restore to temporary database
4. Extract uncorrupted data
5. Merge into production
6. Verify data integrity

**Estimated Time**: 1-2 hours

### Medium: Accidental Deletion

**Scenario**: User or admin accidentally deleted data

**Steps**:
1. Determine deletion timestamp
2. Find backup before deletion
3. Restore to temp database
4. Export deleted records
5. Import to production
6. Verify restoration

**Estimated Time**: 30-60 minutes

### Low: Developer Needs Test Data

**Scenario**: Need production data for development

**Steps**:
1. Use latest backup
2. Restore to dev environment
3. Anonymize sensitive data
4. Provide access to developers

**Estimated Time**: 15-30 minutes

## Monitoring and Alerts

### Critical Alerts

These require immediate action:
- Backup failed for 48+ hours
- S3 upload failed for 24+ hours
- Database connection lost
- Disk usage > 90%
- WAL archiving stopped

### Warning Alerts

These require investigation:
- Backup age > 24 hours
- Backup size significantly changed
- Disk usage > 80%
- S3 upload delayed

### Info Alerts

For awareness:
- Backup completed successfully
- Retention policy applied
- Health check passed

## Compliance and Audit

### Backup Audit Log

Every backup operation should log:
- Timestamp
- Database name
- Backup size
- Checksum
- S3 upload status
- Retention policy applied
- Any errors or warnings

### Compliance Requirements

Depending on your industry:
- **Healthcare (HIPAA)**: Encrypted backups, access logging
- **Finance (PCI-DSS)**: Encrypted storage, regular testing
- **General (GDPR)**: Right to erasure, data portability

## Cost Optimization

### S3 Storage Classes

- **Standard**: First 30 days
- **Standard-IA**: Monthly backups
- **Glacier**: Long-term archives (>1 year)

### Estimated Costs (AWS S3)

Assumptions:
- Database size: 50GB
- Daily backups: 7 days = 350GB
- Weekly backups: 4 weeks = 200GB
- Monthly backups: 12 months = 600GB
- **Total**: ~1.15TB/month

**S3 Standard-IA**: ~$15/month for storage
**Data Transfer**: ~$5/month
**Total**: ~$20/month

### Cost Reduction Tips

1. Enable compression (50-80% reduction)
2. Use lifecycle policies
3. Delete old WAL files regularly
4. Use S3 Intelligent-Tiering
5. Consider alternative providers (Backblaze B2, Wasabi)

## Advanced Configuration

### PostgreSQL Configuration for Optimal Backups

```conf
# postgresql.conf

# WAL Configuration
wal_level = replica
archive_mode = on
archive_command = '/path/to/wal-archive.sh %p %f'
archive_timeout = 300  # 5 minutes

# Replication Settings
max_wal_senders = 3
wal_keep_size = 1GB
hot_standby = on

# Performance
checkpoint_timeout = 15min
max_wal_size = 2GB
min_wal_size = 80MB

# Logging
log_destination = 'stderr'
logging_collector = on
log_directory = 'log'
log_filename = 'postgresql-%Y-%m-%d.log'
log_rotation_age = 1d
log_min_duration_statement = 1000  # Log queries > 1s
```

### Automated Testing

Create a test script to verify backups:

```bash
#!/bin/bash
# test-restore.sh

# Create backup
./backup.sh

# Restore to test database
./restore.sh --file $LATEST_BACKUP --database test_restore --drop

# Run verification queries
psql -U donelist -d test_restore -c "
    SELECT
        COUNT(*) as table_count
    FROM information_schema.tables
    WHERE table_schema = 'public';
"

# Cleanup
psql -U postgres -c "DROP DATABASE test_restore;"
```

### Monitoring Integration

#### Prometheus Metrics

```yaml
# prometheus.yml

- job_name: 'donelist-backups'
  static_configs:
    - targets: ['localhost:9090']
  metrics_path: '/metrics/backup'
```

#### Grafana Dashboard

Import the provided Grafana dashboard JSON for backup monitoring.

## FAQ

### Q: How often should I test restores?

**A**: At minimum, quarterly. Ideally, monthly automated tests.

### Q: Should I stop the application during backup?

**A**: No. PostgreSQL backups are consistent without downtime using `pg_dump`.

### Q: What if my backup is larger than expected?

**A**: Investigate database growth. May need to adjust retention or storage.

### Q: Can I restore to a different PostgreSQL version?

**A**: Generally yes, but test compatibility. Major version upgrades may require `pg_upgrade`.

### Q: What happens if S3 upload fails?

**A**: Backup is still stored locally. Monitor will alert. Manual upload possible.

### Q: How do I backup Redis data?

**A**: Redis persistence is configured separately. Use RDB snapshots or AOF.

### Q: Can I exclude certain tables from backup?

**A**: Yes, use `pg_dump` exclude options. Update backup.sh accordingly.

### Q: What about database connection pooling during restore?

**A**: Close all connections before restore. Script handles this automatically.

## Troubleshooting Guide

### Backup Script Exits with Error Code 1

```bash
# Check logs
tail -f /var/backups/donelist/backup_*.log

# Common causes:
# - Database connection failed
# - Insufficient disk space
# - Permission issues
# - pg_dump not found
```

### S3 Upload Timeouts

```bash
# Increase timeout
export AWS_CLI_TIMEOUT=600

# Use multipart upload for large files
aws configure set default.s3.multipart_threshold 64MB
aws configure set default.s3.multipart_chunksize 16MB
```

### Restore Stuck or Very Slow

```bash
# Check database locks
psql -U postgres -c "SELECT * FROM pg_locks WHERE NOT granted;"

# Check system resources
htop
iostat -x 1

# Use parallel restore (for custom format)
pg_restore --jobs=4 ...
```

### WAL Files Piling Up

```bash
# Check archive_command status
psql -U postgres -c "SELECT * FROM pg_stat_archiver;"

# Manually test archive_command
sudo -u postgres /path/to/wal-archive.sh /path/to/wal/file filename

# Clear failed attempts
# WARNING: Only in emergency, may lose PITR capability
pg_ctl reload
```

## Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.0.0 | 2024-01-15 | Initial implementation |
| 1.1.0 | TBD | Add Redis backup support |
| 1.2.0 | TBD | Add automated testing |

## Contributing

When updating backup procedures:
1. Test in staging environment
2. Update documentation
3. Update runbooks
4. Train team members
5. Schedule DR drill

## License

Internal use only. Confidential.
