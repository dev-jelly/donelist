#!/bin/bash

# Backup Monitoring Script for Donelist
# This script monitors backup health and sends alerts

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
S3_BUCKET="${S3_BUCKET:-}"
ALERT_WEBHOOK="${ALERT_WEBHOOK:-}"
MAX_BACKUP_AGE_HOURS="${MAX_BACKUP_AGE_HOURS:-48}"
MIN_BACKUP_SIZE_MB="${MIN_BACKUP_SIZE_MB:-1}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Tracking variables
ISSUES=0
WARNINGS=0

# Logging functions
log_success() {
    echo -e "${GREEN}✓${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
    ((WARNINGS++))
}

log_error() {
    echo -e "${RED}✗${NC} $1"
    ((ISSUES++))
}

# Send alert
send_alert() {
    local LEVEL=$1
    local MESSAGE=$2

    if [ -z "$ALERT_WEBHOOK" ]; then
        return 0
    fi

    curl -X POST "$ALERT_WEBHOOK" \
        -H "Content-Type: application/json" \
        -d "{\"level\":\"$LEVEL\",\"message\":\"$MESSAGE\",\"timestamp\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}" \
        > /dev/null 2>&1 || true
}

# Check local backups
check_local_backups() {
    echo "Checking local backups..."

    if [ ! -d "$BACKUP_DIR" ]; then
        log_error "Backup directory does not exist: $BACKUP_DIR"
        return
    fi

    # Find most recent backup
    LATEST_BACKUP=$(find "$BACKUP_DIR" -name "postgres_full_*.sql.gz" -type f -printf '%T@ %p\n' 2>/dev/null | sort -rn | head -1 | cut -d' ' -f2-)

    if [ -z "$LATEST_BACKUP" ]; then
        log_error "No local backups found"
        send_alert "error" "No local backups found in $BACKUP_DIR"
        return
    fi

    # Check backup age
    BACKUP_AGE_SECONDS=$(( $(date +%s) - $(stat -c %Y "$LATEST_BACKUP" 2>/dev/null || stat -f %m "$LATEST_BACKUP" 2>/dev/null) ))
    BACKUP_AGE_HOURS=$(( BACKUP_AGE_SECONDS / 3600 ))

    if [ $BACKUP_AGE_HOURS -gt $MAX_BACKUP_AGE_HOURS ]; then
        log_warning "Latest backup is $BACKUP_AGE_HOURS hours old (threshold: $MAX_BACKUP_AGE_HOURS hours)"
        send_alert "warning" "Latest backup is $BACKUP_AGE_HOURS hours old"
    else
        log_success "Latest backup age: $BACKUP_AGE_HOURS hours"
    fi

    # Check backup size
    BACKUP_SIZE=$(stat -c %s "$LATEST_BACKUP" 2>/dev/null || stat -f %z "$LATEST_BACKUP" 2>/dev/null)
    BACKUP_SIZE_MB=$(( BACKUP_SIZE / 1024 / 1024 ))

    if [ $BACKUP_SIZE_MB -lt $MIN_BACKUP_SIZE_MB ]; then
        log_warning "Latest backup size is ${BACKUP_SIZE_MB}MB (below threshold: ${MIN_BACKUP_SIZE_MB}MB)"
    else
        log_success "Latest backup size: ${BACKUP_SIZE_MB}MB"
    fi

    # Count total backups
    TOTAL_BACKUPS=$(find "$BACKUP_DIR" -name "postgres_full_*.sql.gz" -type f | wc -l)
    log_success "Total local backups: $TOTAL_BACKUPS"

    # Verify latest backup
    echo "Verifying latest backup integrity..."
    if pg_restore --list "$LATEST_BACKUP" > /dev/null 2>&1; then
        log_success "Latest backup integrity verified"
    else
        log_error "Latest backup integrity check failed"
        send_alert "error" "Latest backup integrity check failed: $LATEST_BACKUP"
    fi
}

# Check S3 backups
check_s3_backups() {
    if [ -z "$S3_BUCKET" ] || ! command -v aws &> /dev/null; then
        echo "Skipping S3 backup check (not configured or AWS CLI not available)"
        return
    fi

    echo "Checking S3 backups..."

    # List S3 backups
    S3_BACKUP_COUNT=$(aws s3 ls "s3://$S3_BUCKET/backups/" --recursive | grep -c "\.sql\.gz$" || echo "0")

    if [ "$S3_BACKUP_COUNT" -eq 0 ]; then
        log_warning "No backups found in S3 bucket"
    else
        log_success "Total S3 backups: $S3_BACKUP_COUNT"
    fi

    # Check most recent S3 backup
    LATEST_S3_BACKUP=$(aws s3 ls "s3://$S3_BUCKET/backups/" --recursive | grep "\.sql\.gz$" | sort | tail -1)

    if [ -n "$LATEST_S3_BACKUP" ]; then
        LATEST_S3_DATE=$(echo "$LATEST_S3_BACKUP" | awk '{print $1" "$2}')
        LATEST_S3_SIZE=$(echo "$LATEST_S3_BACKUP" | awk '{print $3}')
        LATEST_S3_SIZE_MB=$(( LATEST_S3_SIZE / 1024 / 1024 ))

        log_success "Latest S3 backup: $LATEST_S3_DATE (${LATEST_S3_SIZE_MB}MB)"
    fi
}

# Check WAL archiving
check_wal_archiving() {
    WAL_ARCHIVE_DIR="${WAL_ARCHIVE_DIR:-/var/backups/donelist/wal}"

    if [ ! -d "$WAL_ARCHIVE_DIR" ]; then
        echo "Skipping WAL archiving check (not configured)"
        return
    fi

    echo "Checking WAL archiving..."

    WAL_COUNT=$(find "$WAL_ARCHIVE_DIR" -type f | wc -l)

    if [ "$WAL_COUNT" -eq 0 ]; then
        log_warning "No WAL files found in archive"
    else
        log_success "Total WAL files: $WAL_COUNT"
    fi

    # Check most recent WAL file
    LATEST_WAL=$(find "$WAL_ARCHIVE_DIR" -type f -printf '%T@ %p\n' 2>/dev/null | sort -rn | head -1 | cut -d' ' -f2-)

    if [ -n "$LATEST_WAL" ]; then
        WAL_AGE_SECONDS=$(( $(date +%s) - $(stat -c %Y "$LATEST_WAL" 2>/dev/null || stat -f %m "$LATEST_WAL" 2>/dev/null) ))
        WAL_AGE_MINUTES=$(( WAL_AGE_SECONDS / 60 ))

        log_success "Latest WAL age: $WAL_AGE_MINUTES minutes"
    fi
}

# Check disk space
check_disk_space() {
    echo "Checking disk space..."

    DISK_USAGE=$(df -h "$BACKUP_DIR" | tail -1 | awk '{print $5}' | sed 's/%//')

    if [ "$DISK_USAGE" -gt 90 ]; then
        log_error "Disk usage is at ${DISK_USAGE}% (critical)"
        send_alert "error" "Backup disk usage critical: ${DISK_USAGE}%"
    elif [ "$DISK_USAGE" -gt 80 ]; then
        log_warning "Disk usage is at ${DISK_USAGE}% (warning)"
        send_alert "warning" "Backup disk usage high: ${DISK_USAGE}%"
    else
        log_success "Disk usage: ${DISK_USAGE}%"
    fi
}

# Check database connectivity
check_database() {
    echo "Checking database connectivity..."

    DB_HOST="${DB_HOST:-localhost}"
    DB_PORT="${DB_PORT:-5432}"
    DB_USER="${DB_USER:-donelist}"

    export PGPASSWORD="$DB_PASSWORD"

    if psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d postgres -c "SELECT 1" > /dev/null 2>&1; then
        log_success "Database connection successful"
    else
        log_error "Failed to connect to database"
        send_alert "error" "Failed to connect to database server"
    fi

    unset PGPASSWORD
}

# Generate report
generate_report() {
    echo ""
    echo "====================================="
    echo "Backup Monitoring Summary"
    echo "====================================="
    echo "Timestamp: $(date '+%Y-%m-%d %H:%M:%S')"
    echo "Issues: $ISSUES"
    echo "Warnings: $WARNINGS"

    if [ $ISSUES -eq 0 ] && [ $WARNINGS -eq 0 ]; then
        echo "Status: ${GREEN}HEALTHY${NC}"
        send_alert "info" "Backup system healthy"
    elif [ $ISSUES -eq 0 ]; then
        echo "Status: ${YELLOW}WARNING${NC}"
    else
        echo "Status: ${RED}CRITICAL${NC}"
        send_alert "error" "Backup system has $ISSUES issues and $WARNINGS warnings"
    fi

    echo "====================================="
}

# Main execution
main() {
    echo "Starting backup monitoring..."
    echo ""

    check_database
    check_local_backups
    check_s3_backups
    check_wal_archiving
    check_disk_space

    generate_report
}

# Run main function
main "$@"
