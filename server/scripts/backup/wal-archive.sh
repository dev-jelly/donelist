#!/bin/bash

# PostgreSQL WAL Archive Script for Donelist
# This script is called by PostgreSQL's archive_command to archive WAL files

set -euo pipefail

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Load environment variables
if [ -f "$PROJECT_ROOT/.env" ]; then
    source "$PROJECT_ROOT/.env"
fi

# Configuration
WAL_ARCHIVE_DIR="${WAL_ARCHIVE_DIR:-/var/backups/donelist/wal}"
S3_BUCKET="${S3_BUCKET:-}"
LOG_FILE="${WAL_ARCHIVE_DIR}/wal-archive.log"

# WAL file parameters (passed by PostgreSQL)
WAL_PATH="${1:-}"
WAL_FILE="${2:-}"

# Logging functions
log_info() {
    echo "[INFO] $(date '+%Y-%m-%d %H:%M:%S') - $1" >> "$LOG_FILE"
}

log_error() {
    echo "[ERROR] $(date '+%Y-%m-%d %H:%M:%S') - $1" >> "$LOG_FILE"
}

# Validate parameters
if [ -z "$WAL_PATH" ] || [ -z "$WAL_FILE" ]; then
    log_error "WAL path and file name are required"
    exit 1
fi

# Create WAL archive directory
mkdir -p "$WAL_ARCHIVE_DIR" 2>/dev/null || true

# Archive WAL file locally
log_info "Archiving WAL file: $WAL_FILE"

# Copy to local archive
LOCAL_WAL_PATH="$WAL_ARCHIVE_DIR/$WAL_FILE"
cp "$WAL_PATH" "$LOCAL_WAL_PATH" || {
    log_error "Failed to copy WAL file to local archive"
    exit 1
}

log_info "WAL file copied to local archive: $LOCAL_WAL_PATH"

# Upload to S3 if configured
if [ -n "$S3_BUCKET" ] && command -v aws &> /dev/null; then
    S3_PATH="s3://$S3_BUCKET/wal/$(date +%Y/%m/%d)/$WAL_FILE"

    aws s3 cp "$LOCAL_WAL_PATH" "$S3_PATH" \
        --metadata "archive-date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        >> "$LOG_FILE" 2>&1 || {
        log_error "Failed to upload WAL file to S3"
        # Don't exit with error - local archive is sufficient
    }

    log_info "WAL file uploaded to S3: $S3_PATH"
fi

# Cleanup old WAL files (keep last 7 days)
find "$WAL_ARCHIVE_DIR" -name "0*" -type f -mtime +7 -delete 2>/dev/null || true

log_info "WAL archiving completed successfully"

exit 0
