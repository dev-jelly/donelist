#!/bin/bash

# PostgreSQL Backup Script for Donelist
# This script creates a full PostgreSQL backup and uploads it to S3

set -euo pipefail

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Load environment variables
if [ -f "$PROJECT_ROOT/.env" ]; then
    source "$PROJECT_ROOT/.env"
fi

# Configuration
BACKUP_DIR="${BACKUP_DIR:-/var/backups/donelist}"
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_NAME="${DB_NAME:-donelist}"
DB_USER="${DB_USER:-donelist}"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="postgres_full_${DB_NAME}_${TIMESTAMP}.sql.gz"
BACKUP_PATH="$BACKUP_DIR/$BACKUP_FILE"
LOG_FILE="$BACKUP_DIR/backup_${TIMESTAMP}.log"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1" | tee -a "$LOG_FILE"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1" | tee -a "$LOG_FILE"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1" | tee -a "$LOG_FILE"
}

# Error handler
error_exit() {
    log_error "$1"
    exit 1
}

# Check dependencies
check_dependencies() {
    log_info "Checking dependencies..."

    if ! command -v pg_dump &> /dev/null; then
        error_exit "pg_dump is not installed"
    fi

    if ! command -v aws &> /dev/null; then
        log_warn "AWS CLI is not installed. S3 upload will be skipped."
    fi

    log_info "Dependencies check passed"
}

# Create backup directory
create_backup_dir() {
    if [ ! -d "$BACKUP_DIR" ]; then
        log_info "Creating backup directory: $BACKUP_DIR"
        mkdir -p "$BACKUP_DIR" || error_exit "Failed to create backup directory"
    fi
}

# Perform PostgreSQL backup
perform_backup() {
    log_info "Starting PostgreSQL backup"
    log_info "Database: $DB_NAME"
    log_info "Backup file: $BACKUP_PATH"

    # Set password for pg_dump
    export PGPASSWORD="$DB_PASSWORD"

    # Perform backup with compression
    pg_dump -h "$DB_HOST" \
            -p "$DB_PORT" \
            -U "$DB_USER" \
            -d "$DB_NAME" \
            --format=custom \
            --verbose \
            --no-owner \
            --no-acl \
            2>>"$LOG_FILE" | gzip > "$BACKUP_PATH" || error_exit "Backup failed"

    unset PGPASSWORD

    log_info "Backup completed successfully"
}

# Calculate checksum
calculate_checksum() {
    log_info "Calculating checksum..."

    CHECKSUM=$(sha256sum "$BACKUP_PATH" | awk '{print $1}')
    echo "$CHECKSUM" > "${BACKUP_PATH}.sha256"

    log_info "Checksum: $CHECKSUM"
}

# Get file size
get_file_size() {
    FILE_SIZE=$(stat -f%z "$BACKUP_PATH" 2>/dev/null || stat -c%s "$BACKUP_PATH" 2>/dev/null)
    FILE_SIZE_MB=$((FILE_SIZE / 1024 / 1024))

    log_info "Backup size: ${FILE_SIZE_MB}MB"
}

# Upload to S3
upload_to_s3() {
    if ! command -v aws &> /dev/null; then
        log_warn "Skipping S3 upload (AWS CLI not installed)"
        return 0
    fi

    if [ -z "${S3_BUCKET:-}" ]; then
        log_warn "S3_BUCKET not configured, skipping upload"
        return 0
    fi

    log_info "Uploading backup to S3..."

    S3_PATH="s3://$S3_BUCKET/backups/$(date +%Y/%m/%d)/$BACKUP_FILE"

    aws s3 cp "$BACKUP_PATH" "$S3_PATH" \
        --metadata "backup-date=$(date -u +%Y-%m-%dT%H:%M:%SZ),database=$DB_NAME,checksum=$CHECKSUM" \
        || log_warn "S3 upload failed, but local backup is available"

    # Upload checksum file
    aws s3 cp "${BACKUP_PATH}.sha256" "${S3_PATH}.sha256" || true

    log_info "Backup uploaded to: $S3_PATH"
}

# Verify backup
verify_backup() {
    log_info "Verifying backup..."

    export PGPASSWORD="$DB_PASSWORD"

    # Test if backup can be listed (dry run)
    if pg_restore --list "$BACKUP_PATH" > /dev/null 2>&1; then
        log_info "Backup verification successful"
    else
        log_error "Backup verification failed"
        return 1
    fi

    unset PGPASSWORD
}

# Cleanup old backups
cleanup_old_backups() {
    RETENTION_DAYS="${RETENTION_DAYS:-7}"

    log_info "Cleaning up backups older than $RETENTION_DAYS days..."

    find "$BACKUP_DIR" -name "postgres_full_*.sql.gz" -mtime +$RETENTION_DAYS -delete 2>/dev/null || true
    find "$BACKUP_DIR" -name "*.sha256" -mtime +$RETENTION_DAYS -delete 2>/dev/null || true
    find "$BACKUP_DIR" -name "backup_*.log" -mtime +$RETENTION_DAYS -delete 2>/dev/null || true

    log_info "Cleanup completed"
}

# Send notification
send_notification() {
    local STATUS=$1
    local MESSAGE=$2

    if [ -z "${WEBHOOK_URL:-}" ]; then
        return 0
    fi

    curl -X POST "$WEBHOOK_URL" \
        -H "Content-Type: application/json" \
        -d "{\"status\":\"$STATUS\",\"message\":\"$MESSAGE\",\"timestamp\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}" \
        > /dev/null 2>&1 || true
}

# Main execution
main() {
    log_info "=== Starting Donelist PostgreSQL Backup ==="

    check_dependencies
    create_backup_dir
    perform_backup
    calculate_checksum
    get_file_size
    verify_backup
    upload_to_s3
    cleanup_old_backups

    log_info "=== Backup Completed Successfully ==="
    log_info "Backup file: $BACKUP_PATH"
    log_info "Checksum: $CHECKSUM"
    log_info "Size: ${FILE_SIZE_MB}MB"

    send_notification "success" "Backup completed successfully: $BACKUP_FILE (${FILE_SIZE_MB}MB)"
}

# Run main function
main "$@"
