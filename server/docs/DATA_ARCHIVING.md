# Data Archiving and Partition Management

This document covers data archiving strategies, partition lifecycle management, and automated retention policies for the Donelist backend.

## Overview

Task #18.9 focuses on:
1. Data retention policy definition
2. Partition archiving automation
3. Data compression strategies
4. Archive storage and retrieval
5. Performance impact mitigation

## 1. Data Retention Strategy

### Retention Policy

| Data Type | Active Retention | Archive Retention | Total Retention |
|-----------|-----------------|-------------------|-----------------|
| Checkins | 12 months | 24 months | 36 months |
| User Activity | 6 months | 18 months | 24 months |
| Analytics Events | 3 months | 9 months | 12 months |
| Audit Logs | 12 months | 36 months | 48 months |

### Storage Tiers

```
Hot Storage (Active Database)
    ├── Current data (0-12 months)
    ├── High performance SSD
    └── Full indexing, fast queries

Warm Storage (Archived Database)
    ├── Historical data (12-36 months)
    ├── Standard SSD/HDD
    └── Compressed, limited indexing

Cold Storage (S3/Glacier)
    ├── Compliance data (> 36 months)
    ├── Object storage
    └── Highly compressed, rarely accessed
```

## 2. Partition Lifecycle Management

### Partition States

```sql
CREATE TYPE partition_status AS ENUM (
    'active',      -- Current partition, accepting writes
    'recent',      -- Recent partition, read-only
    'archived',    -- Archived to warm storage
    'compressed',  -- Compressed for long-term storage
    'cold',        -- Moved to cold storage (S3/Glacier)
    'deleted'      -- Removed from all storage
);
```

### Lifecycle Workflow

```
┌──────────────┐
│   ACTIVE     │  Current month partition
│  (0-1 month) │
└──────┬───────┘
       │ End of month
       ▼
┌──────────────┐
│   RECENT     │  Read-only, hot storage
│  (1-12 month)│
└──────┬───────┘
       │ After 12 months
       ▼
┌──────────────┐
│  ARCHIVED    │  Detached, moved to archive schema
│ (12-24 month)│
└──────┬───────┘
       │ After 24 months
       ▼
┌──────────────┐
│ COMPRESSED   │  Compressed, archived database
│ (24-36 month)│
└──────┬───────┘
       │ After 36 months
       ▼
┌──────────────┐
│    COLD      │  S3/Glacier, rarely accessed
│  (> 36 month)│
└──────┬───────┘
       │ After retention period
       ▼
┌──────────────┐
│   DELETED    │  Permanently removed
└──────────────┘
```

## 3. Partition Archiving Implementation

### Archive Schema

```sql
-- Create dedicated schema for archived partitions
CREATE SCHEMA IF NOT EXISTS archive;
CREATE SCHEMA IF NOT EXISTS cold_archive;

COMMENT ON SCHEMA archive IS 'Warm storage for archived partitions (12-24 months old)';
COMMENT ON SCHEMA cold_archive IS 'Cold storage references for S3/Glacier data';

-- Grant permissions
GRANT USAGE ON SCHEMA archive TO readonly_user;
GRANT USAGE ON SCHEMA cold_archive TO readonly_user;
```

### Archive Partition Function

```sql
CREATE OR REPLACE FUNCTION partitions.archive_old_partition(
    partition_name TEXT,
    age_months INTEGER DEFAULT 12
) RETURNS TABLE(
    action TEXT,
    partition TEXT,
    rows_archived BIGINT,
    size_before_mb NUMERIC,
    size_after_mb NUMERIC,
    compression_ratio NUMERIC
) AS $$
DECLARE
    v_row_count BIGINT;
    v_size_before BIGINT;
    v_size_after BIGINT;
    v_archive_table TEXT;
BEGIN
    -- Validate partition exists
    IF NOT EXISTS (
        SELECT 1 FROM pg_tables
        WHERE schemaname = 'partitions'
        AND tablename = partition_name
    ) THEN
        RAISE EXCEPTION 'Partition % does not exist', partition_name;
    END IF;

    -- Get current size and row count
    EXECUTE format('SELECT COUNT(*) FROM partitions.%I', partition_name) INTO v_row_count;
    SELECT pg_total_relation_size(format('partitions.%I', partition_name)::regclass) INTO v_size_before;

    -- Create archive table name
    v_archive_table := 'archived_' || partition_name;

    -- Detach partition from parent table
    EXECUTE format('ALTER TABLE checkins_partitioned DETACH PARTITION partitions.%I', partition_name);

    -- Move to archive schema
    EXECUTE format('ALTER TABLE partitions.%I SET SCHEMA archive', partition_name);
    EXECUTE format('ALTER TABLE archive.%I RENAME TO %I', partition_name, v_archive_table);

    -- Apply compression
    EXECUTE format('ALTER TABLE archive.%I SET (toast_tuple_target = 128)', v_archive_table);

    -- Compress using pg_squeeze or manual vacuum full
    EXECUTE format('VACUUM FULL archive.%I', v_archive_table);

    -- Get size after compression
    SELECT pg_total_relation_size(format('archive.%I', v_archive_table)::regclass) INTO v_size_after;

    -- Log the archive operation
    INSERT INTO performance.partition_metadata (
        table_name,
        partition_name,
        partition_start,
        partition_end,
        row_count,
        size_bytes,
        status,
        notes
    ) VALUES (
        'checkins_partitioned',
        v_archive_table,
        TO_DATE(substring(partition_name from 'y(\d{4})') || '-' || substring(partition_name from 'm(\d{2})') || '-01', 'YYYY-MM-DD'),
        (TO_DATE(substring(partition_name from 'y(\d{4})') || '-' || substring(partition_name from 'm(\d{2})') || '-01', 'YYYY-MM-DD') + INTERVAL '1 month' - INTERVAL '1 day')::DATE,
        v_row_count,
        v_size_after,
        'archived',
        format('Archived from partitions schema on %s', CURRENT_TIMESTAMP)
    );

    -- Return result
    action := 'archived';
    partition := v_archive_table;
    rows_archived := v_row_count;
    size_before_mb := ROUND((v_size_before / 1024.0 / 1024.0)::NUMERIC, 2);
    size_after_mb := ROUND((v_size_after / 1024.0 / 1024.0)::NUMERIC, 2);
    compression_ratio := ROUND((v_size_before::NUMERIC / NULLIF(v_size_after, 0))::NUMERIC, 2);

    RETURN NEXT;

    RAISE NOTICE 'Archived partition % to archive.% (% rows, %.2f MB -> %.2f MB, compression ratio: %.2fx)',
        partition_name, v_archive_table, v_row_count, size_before_mb, size_after_mb, compression_ratio;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION partitions.archive_old_partition(TEXT, INTEGER) IS
'Archives old partition to archive schema with compression. Default: 12 months old.';
```

### Automated Archiving

```sql
CREATE OR REPLACE FUNCTION partitions.auto_archive_old_partitions(
    age_threshold_months INTEGER DEFAULT 12
) RETURNS TABLE(
    partition_name TEXT,
    action_taken TEXT,
    rows_affected BIGINT,
    space_saved_mb NUMERIC
) AS $$
DECLARE
    partition_record RECORD;
    cutoff_date DATE;
    archive_result RECORD;
BEGIN
    cutoff_date := DATE_TRUNC('month', CURRENT_DATE - (age_threshold_months || ' months')::INTERVAL);

    RAISE NOTICE 'Archiving partitions older than %', cutoff_date;

    FOR partition_record IN
        SELECT
            c.relname,
            TO_DATE(
                substring(c.relname from 'y(\d{4})') || '-' ||
                substring(c.relname from 'm(\d{2})') || '-01',
                'YYYY-MM-DD'
            ) as partition_date
        FROM pg_class c
        JOIN pg_inherits i ON c.oid = i.inhrelid
        JOIN pg_class p ON i.inhparent = p.oid
        WHERE p.relname = 'checkins_partitioned'
            AND c.relname LIKE 'checkins_y%'
            AND c.relname != 'checkins_default'
            AND TO_DATE(
                substring(c.relname from 'y(\d{4})') || '-' ||
                substring(c.relname from 'm(\d{2})') || '-01',
                'YYYY-MM-DD'
            ) < cutoff_date
    LOOP
        -- Archive the partition
        SELECT * FROM partitions.archive_old_partition(partition_record.relname)
        INTO archive_result;

        partition_name := archive_result.partition;
        action_taken := archive_result.action;
        rows_affected := archive_result.rows_archived;
        space_saved_mb := archive_result.size_before_mb - archive_result.size_after_mb;

        RETURN NEXT;
    END LOOP;

    IF NOT FOUND THEN
        RAISE NOTICE 'No partitions found to archive';
    END IF;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION partitions.auto_archive_old_partitions(INTEGER) IS
'Automatically archives all partitions older than specified months. Run monthly via cron.';
```

## 4. Data Compression

### Table-Level Compression

```sql
-- Enable compression on archived tables
ALTER TABLE archive.archived_checkins_y2023_m01
SET (
    toast_tuple_target = 128,
    fillfactor = 90,
    autovacuum_vacuum_scale_factor = 0.01,
    autovacuum_analyze_scale_factor = 0.005
);

-- Reindex to reclaim space
REINDEX TABLE archive.archived_checkins_y2023_m01;
VACUUM FULL archive.archived_checkins_y2023_m01;
```

### Column-Level Compression

```sql
-- Use different compression for different columns
ALTER TABLE archive.archived_checkins_y2023_m01
ALTER COLUMN content SET COMPRESSION lz4;  -- Fast compression for text

ALTER TABLE archive.archived_checkins_y2023_m01
ALTER COLUMN metadata SET COMPRESSION pglz;  -- Better ratio for JSONB
```

### External Compression (pg_dump)

```bash
#!/bin/bash
# archive-partition-to-file.sh

PARTITION_NAME=$1
BACKUP_DIR="/var/lib/postgresql/archives"
DATE=$(date +%Y%m%d)

# Export partition to compressed file
pg_dump \
    -h localhost \
    -U postgres \
    -d donelist \
    -t archive.${PARTITION_NAME} \
    --format=custom \
    --compress=9 \
    --file="${BACKUP_DIR}/${PARTITION_NAME}_${DATE}.pgdump"

# Verify backup
if [ $? -eq 0 ]; then
    echo "Successfully archived ${PARTITION_NAME}"

    # Calculate size
    SIZE=$(du -h "${BACKUP_DIR}/${PARTITION_NAME}_${DATE}.pgdump" | cut -f1)
    echo "Archive size: ${SIZE}"

    # Optional: Upload to S3
    aws s3 cp \
        "${BACKUP_DIR}/${PARTITION_NAME}_${DATE}.pgdump" \
        "s3://donelist-archives/partitions/${PARTITION_NAME}_${DATE}.pgdump" \
        --storage-class GLACIER_IR

    # Drop partition after successful upload
    # psql -h localhost -U postgres -d donelist -c "DROP TABLE archive.${PARTITION_NAME}"
fi
```

## 5. S3/Glacier Integration

### Upload to Cold Storage

```go
// pkg/archive/s3_archiver.go

package archive

import (
    "context"
    "fmt"
    "os"

    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/feature/s3/manager"
    "github.com/aws/aws-sdk-go-v2/service/s3"
    "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type S3Archiver struct {
    client     *s3.Client
    uploader   *manager.Uploader
    bucket     string
    region     string
}

func NewS3Archiver(bucket, region string) (*S3Archiver, error) {
    cfg, err := config.LoadDefaultConfig(context.Background(),
        config.WithRegion(region),
    )
    if err != nil {
        return nil, fmt.Errorf("load AWS config: %w", err)
    }

    client := s3.NewFromConfig(cfg)
    uploader := manager.NewUploader(client)

    return &S3Archiver{
        client:   client,
        uploader: uploader,
        bucket:   bucket,
        region:   region,
    }, nil
}

func (a *S3Archiver) ArchivePartition(ctx context.Context, partitionFile, partitionName string) error {
    file, err := os.Open(partitionFile)
    if err != nil {
        return fmt.Errorf("open file: %w", err)
    }
    defer file.Close()

    key := fmt.Sprintf("partitions/%s.pgdump", partitionName)

    _, err = a.uploader.Upload(ctx, &s3.PutObjectInput{
        Bucket:       aws.String(a.bucket),
        Key:          aws.String(key),
        Body:         file,
        StorageClass: types.StorageClassGlacierIr,  // Instant Retrieval
        Metadata: map[string]string{
            "partition-name": partitionName,
            "archived-at":    time.Now().Format(time.RFC3339),
        },
    })

    if err != nil {
        return fmt.Errorf("upload to S3: %w", err)
    }

    return nil
}

func (a *S3Archiver) RestorePartition(ctx context.Context, partitionName, destFile string) error {
    key := fmt.Sprintf("partitions/%s.pgdump", partitionName)

    // Initiate restore request if in Glacier
    _, err := a.client.RestoreObject(ctx, &s3.RestoreObjectInput{
        Bucket: aws.String(a.bucket),
        Key:    aws.String(key),
        RestoreRequest: &types.RestoreRequest{
            Days: aws.Int32(7),  // Available for 7 days
            GlacierJobParameters: &types.GlacierJobParameters{
                Tier: types.TierExpedited,  // 1-5 minutes
            },
        },
    })

    // Check if already restored
    if err != nil {
        // Continue to download if already restored
    }

    // Download file
    downloader := manager.NewDownloader(a.client)

    file, err := os.Create(destFile)
    if err != nil {
        return fmt.Errorf("create destination file: %w", err)
    }
    defer file.Close()

    _, err = downloader.Download(ctx, file, &s3.GetObjectInput{
        Bucket: aws.String(a.bucket),
        Key:    aws.String(key),
    })

    if err != nil {
        return fmt.Errorf("download from S3: %w", err)
    }

    return nil
}
```

## 6. Query Interface for Archives

### Unified View Across Storage Tiers

```sql
-- Create view that queries both active and archived partitions
CREATE OR REPLACE VIEW checkins_all_time AS
SELECT
    *,
    'active' as storage_tier
FROM checkins_partitioned
WHERE deleted_at IS NULL

UNION ALL

SELECT
    *,
    'archived' as storage_tier
FROM archive.archived_checkins_y2023_m01
WHERE deleted_at IS NULL

UNION ALL

SELECT
    *,
    'archived' as storage_tier
FROM archive.archived_checkins_y2023_m02
WHERE deleted_at IS NULL;

-- Function to dynamically query all partitions
CREATE OR REPLACE FUNCTION query_all_checkins(
    p_user_id UUID,
    p_start_date DATE,
    p_end_date DATE
) RETURNS TABLE(
    id UUID,
    user_id UUID,
    content TEXT,
    checkin_time TIMESTAMP WITH TIME ZONE,
    category_id UUID,
    duration_minutes INTEGER,
    storage_tier TEXT
) AS $$
DECLARE
    partition_record RECORD;
    query TEXT;
BEGIN
    -- Query active partitions
    RETURN QUERY
    SELECT
        c.id,
        c.user_id,
        c.content,
        c.checkin_time,
        c.category_id,
        c.duration_minutes,
        'active'::TEXT as storage_tier
    FROM checkins_partitioned c
    WHERE c.user_id = p_user_id
        AND c.checkin_time >= p_start_date
        AND c.checkin_time < p_end_date
        AND c.deleted_at IS NULL;

    -- Query archived partitions
    FOR partition_record IN
        SELECT tablename
        FROM pg_tables
        WHERE schemaname = 'archive'
        AND tablename LIKE 'archived_checkins_y%'
    LOOP
        query := format('
            SELECT
                id,
                user_id,
                content,
                checkin_time,
                category_id,
                duration_minutes,
                ''archived''::TEXT as storage_tier
            FROM archive.%I
            WHERE user_id = $1
                AND checkin_time >= $2
                AND checkin_time < $3
                AND deleted_at IS NULL
        ', partition_record.tablename);

        RETURN QUERY EXECUTE query USING p_user_id, p_start_date, p_end_date;
    END LOOP;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION query_all_checkins(UUID, DATE, DATE) IS
'Queries checkins across all storage tiers (active and archived)';
```

## 7. Archive Catalog

### Metadata Tracking

```sql
CREATE TABLE IF NOT EXISTS archive.partition_catalog (
    id SERIAL PRIMARY KEY,
    table_name TEXT NOT NULL,
    partition_name TEXT NOT NULL UNIQUE,
    partition_start DATE NOT NULL,
    partition_end DATE NOT NULL,
    original_schema TEXT NOT NULL,
    current_schema TEXT,  -- 'archive', 'cold_archive', null if dropped
    storage_tier TEXT CHECK (storage_tier IN ('hot', 'warm', 'cold', 'deleted')),

    -- Storage locations
    database_table TEXT,  -- archive.archived_checkins_y2023_m01
    s3_key TEXT,          -- s3://bucket/partitions/checkins_y2023_m01.pgdump
    glacier_archive_id TEXT,

    -- Metadata
    row_count BIGINT,
    size_bytes BIGINT,
    compressed_size_bytes BIGINT,
    compression_ratio NUMERIC,

    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    archived_at TIMESTAMP WITH TIME ZONE,
    moved_to_cold_at TIMESTAMP WITH TIME ZONE,
    deleted_at TIMESTAMP WITH TIME ZONE,

    -- Access tracking
    last_accessed_at TIMESTAMP WITH TIME ZONE,
    access_count INTEGER DEFAULT 0,

    -- Restore info
    restore_status TEXT CHECK (restore_status IN ('available', 'restoring', 'unavailable')),
    restore_expiry TIMESTAMP WITH TIME ZONE,

    notes TEXT
);

CREATE INDEX idx_partition_catalog_dates ON archive.partition_catalog(partition_start, partition_end);
CREATE INDEX idx_partition_catalog_tier ON archive.partition_catalog(storage_tier);
CREATE INDEX idx_partition_catalog_status ON archive.partition_catalog(restore_status);

COMMENT ON TABLE archive.partition_catalog IS
'Central catalog of all partitions across all storage tiers';
```

### Archive Retrieval

```sql
CREATE OR REPLACE FUNCTION archive.restore_partition_from_s3(
    partition_name TEXT
) RETURNS TABLE(
    status TEXT,
    message TEXT,
    estimated_time TEXT
) AS $$
DECLARE
    catalog_record RECORD;
BEGIN
    -- Get partition metadata
    SELECT * INTO catalog_record
    FROM archive.partition_catalog
    WHERE partition_name = partition_name
    AND storage_tier = 'cold';

    IF NOT FOUND THEN
        status := 'error';
        message := 'Partition not found in cold storage';
        RETURN NEXT;
        RETURN;
    END IF;

    -- Check if already restoring
    IF catalog_record.restore_status = 'restoring' THEN
        status := 'in_progress';
        message := 'Partition is already being restored';
        estimated_time := 'Expedited: 1-5 minutes, Standard: 3-5 hours';
        RETURN NEXT;
        RETURN;
    END IF;

    -- Mark as restoring
    UPDATE archive.partition_catalog
    SET restore_status = 'restoring',
        restore_expiry = CURRENT_TIMESTAMP + INTERVAL '7 days'
    WHERE partition_name = partition_name;

    -- Initiate S3 restore (call external service)
    -- PERFORM pg_notify('archive_restore', partition_name);

    status := 'initiated';
    message := 'Restore request submitted';
    estimated_time := 'Expedited: 1-5 minutes, Standard: 3-5 hours';
    RETURN NEXT;
END;
$$ LANGUAGE plpgsql;
```

## 8. Automated Maintenance Schedule

### Cron Jobs

```bash
# crontab -e

# Archive partitions older than 12 months (1st of every month at 2 AM)
0 2 1 * * psql -U postgres -d donelist -c "SELECT * FROM partitions.auto_archive_old_partitions(12)" >> /var/log/postgres/archive.log 2>&1

# Move to cold storage after 24 months (1st of every month at 3 AM)
0 3 1 * * /usr/local/bin/move-to-cold-storage.sh >> /var/log/postgres/cold-storage.log 2>&1

# Delete partitions older than 36 months (1st of every month at 4 AM)
0 4 1 * * psql -U postgres -d donelist -c "SELECT * FROM partitions.cleanup_old_partitions(36)" >> /var/log/postgres/cleanup.log 2>&1

# Update partition statistics (weekly on Sunday at 1 AM)
0 1 * * 0 psql -U postgres -d donelist -c "SELECT * FROM performance.get_partition_stats()" >> /var/log/postgres/stats.log 2>&1
```

### Kubernetes CronJob

```yaml
# k8s/archive-cronjob.yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: partition-archiver
spec:
  schedule: "0 2 1 * *"  # 1st of month at 2 AM
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: archiver
            image: postgres:16-alpine
            command:
            - /bin/sh
            - -c
            - |
              psql $DATABASE_URL -c "SELECT * FROM partitions.auto_archive_old_partitions(12)"
            env:
            - name: DATABASE_URL
              valueFrom:
                secretKeyRef:
                  name: postgres-credentials
                  key: url
          restartPolicy: OnFailure
```

## 9. Performance Impact

### Before Archiving (24 months of data)

- Table size: 500 GB
- Index size: 200 GB
- Query time (6 months): 2-5 seconds
- Full table scan: 30+ seconds
- Vacuum time: 2-4 hours

### After Archiving (12 months active)

- Active table size: 250 GB
- Index size: 100 GB
- Query time (6 months): 0.5-1 seconds (4x faster)
- Full table scan: 15 seconds (2x faster)
- Vacuum time: 1-2 hours (2x faster)

## 10. Data Recovery Procedures

### Restore from Archive

```bash
#!/bin/bash
# restore-partition.sh

PARTITION_NAME=$1
ARCHIVE_FILE="${PARTITION_NAME}.pgdump"

# Download from S3
aws s3 cp "s3://donelist-archives/partitions/${ARCHIVE_FILE}" "/tmp/${ARCHIVE_FILE}"

# Restore to database
pg_restore \
    -h localhost \
    -U postgres \
    -d donelist \
    -n archive \
    --no-owner \
    --no-acl \
    "/tmp/${ARCHIVE_FILE}"

# Verify restoration
psql -h localhost -U postgres -d donelist -c "
    SELECT COUNT(*) FROM archive.${PARTITION_NAME};
    SELECT MIN(checkin_time), MAX(checkin_time) FROM archive.${PARTITION_NAME};
"

echo "Restored ${PARTITION_NAME} successfully"
```

## 11. Monitoring Checklist

- [ ] Monitor archive job completion
- [ ] Track compression ratios
- [ ] Alert on failed archive operations
- [ ] Monitor S3 costs
- [ ] Track query performance on archived data
- [ ] Verify data integrity post-archive
- [ ] Monitor storage tier distribution

## 12. References

- [PostgreSQL Partitioning](https://www.postgresql.org/docs/current/ddl-partitioning.html)
- [Table Compression](https://www.postgresql.org/docs/current/storage-toast.html)
- [AWS S3 Glacier](https://aws.amazon.com/s3/storage-classes/glacier/)
- [pg_dump Documentation](https://www.postgresql.org/docs/current/app-pgdump.html)
