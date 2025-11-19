#!/bin/bash

# PostgreSQL Restore Script for Donelist
# This script restores a PostgreSQL database from a backup file

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
LOG_FILE="$BACKUP_DIR/restore_${TIMESTAMP}.log"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
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

log_step() {
    echo -e "${BLUE}[STEP]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1" | tee -a "$LOG_FILE"
}

# Error handler
error_exit() {
    log_error "$1"
    exit 1
}

# Usage information
usage() {
    cat <<EOF
Usage: $0 [OPTIONS]

Restore PostgreSQL database from backup

Options:
    -f, --file FILE         Backup file to restore (required)
    -d, --database NAME     Target database name (default: $DB_NAME)
    -h, --host HOST         Database host (default: $DB_HOST)
    -p, --port PORT         Database port (default: $DB_PORT)
    -u, --user USER         Database user (default: $DB_USER)
    --drop                  Drop existing database before restore
    --verify                Verify backup before restore
    --download S3_PATH      Download backup from S3 before restore
    --help                  Show this help message

Examples:
    # Restore from local file
    $0 --file /var/backups/donelist/postgres_full_donelist_20240101_120000.sql.gz

    # Restore and drop existing database
    $0 --file backup.sql.gz --drop

    # Download from S3 and restore
    $0 --download s3://my-bucket/backups/2024/01/01/backup.sql.gz --drop

EOF
    exit 1
}

# Parse command line arguments
BACKUP_FILE=""
TARGET_DB="$DB_NAME"
DROP_EXISTING=false
VERIFY=false
S3_DOWNLOAD=""

while [[ $# -gt 0 ]]; do
    case $1 in
        -f|--file)
            BACKUP_FILE="$2"
            shift 2
            ;;
        -d|--database)
            TARGET_DB="$2"
            shift 2
            ;;
        --drop)
            DROP_EXISTING=true
            shift
            ;;
        --verify)
            VERIFY=true
            shift
            ;;
        --download)
            S3_DOWNLOAD="$2"
            shift 2
            ;;
        --help)
            usage
            ;;
        *)
            log_error "Unknown option: $1"
            usage
            ;;
    esac
done

# Validate required parameters
if [ -z "$BACKUP_FILE" ] && [ -z "$S3_DOWNLOAD" ]; then
    log_error "Backup file or S3 path is required"
    usage
fi

# Check dependencies
check_dependencies() {
    log_step "Checking dependencies..."

    if ! command -v pg_restore &> /dev/null; then
        error_exit "pg_restore is not installed"
    fi

    if ! command -v dropdb &> /dev/null; then
        error_exit "dropdb is not installed"
    fi

    if ! command -v createdb &> /dev/null; then
        error_exit "createdb is not installed"
    fi

    if [ -n "$S3_DOWNLOAD" ] && ! command -v aws &> /dev/null; then
        error_exit "AWS CLI is required for S3 download"
    fi

    log_info "Dependencies check passed"
}

# Download from S3
download_from_s3() {
    if [ -z "$S3_DOWNLOAD" ]; then
        return 0
    fi

    log_step "Downloading backup from S3..."

    BACKUP_FILE="$BACKUP_DIR/downloaded_backup_${TIMESTAMP}.sql.gz"

    aws s3 cp "$S3_DOWNLOAD" "$BACKUP_FILE" || error_exit "Failed to download from S3"

    log_info "Downloaded to: $BACKUP_FILE"

    # Download checksum if available
    aws s3 cp "${S3_DOWNLOAD}.sha256" "${BACKUP_FILE}.sha256" 2>/dev/null || true
}

# Verify backup
verify_backup() {
    log_step "Verifying backup file..."

    if [ ! -f "$BACKUP_FILE" ]; then
        error_exit "Backup file not found: $BACKUP_FILE"
    fi

    # Check if file is readable
    if [ ! -r "$BACKUP_FILE" ]; then
        error_exit "Backup file is not readable: $BACKUP_FILE"
    fi

    # Verify checksum if available
    if [ -f "${BACKUP_FILE}.sha256" ]; then
        log_info "Verifying checksum..."
        EXPECTED_CHECKSUM=$(cat "${BACKUP_FILE}.sha256")
        ACTUAL_CHECKSUM=$(sha256sum "$BACKUP_FILE" | awk '{print $1}')

        if [ "$EXPECTED_CHECKSUM" = "$ACTUAL_CHECKSUM" ]; then
            log_info "Checksum verification passed"
        else
            error_exit "Checksum verification failed!"
        fi
    fi

    # Verify backup can be read by pg_restore
    if $VERIFY; then
        log_info "Testing backup file integrity..."
        if pg_restore --list "$BACKUP_FILE" > /dev/null 2>&1; then
            log_info "Backup file integrity check passed"
        else
            error_exit "Backup file is corrupted or invalid"
        fi
    fi

    log_info "Backup verification completed"
}

# Check database connection
check_connection() {
    log_step "Checking database connection..."

    export PGPASSWORD="$DB_PASSWORD"

    if psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d postgres -c "SELECT 1" > /dev/null 2>&1; then
        log_info "Database connection successful"
    else
        error_exit "Failed to connect to database server"
    fi

    unset PGPASSWORD
}

# Check if database exists
database_exists() {
    export PGPASSWORD="$DB_PASSWORD"

    if psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d postgres -lqt | cut -d \| -f 1 | grep -qw "$TARGET_DB"; then
        return 0
    else
        return 1
    fi

    unset PGPASSWORD
}

# Drop existing database
drop_database() {
    if ! $DROP_EXISTING; then
        return 0
    fi

    log_step "Dropping existing database: $TARGET_DB"

    export PGPASSWORD="$DB_PASSWORD"

    # Terminate existing connections
    psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d postgres -c \
        "SELECT pg_terminate_backend(pg_stat_activity.pid)
         FROM pg_stat_activity
         WHERE pg_stat_activity.datname = '$TARGET_DB'
         AND pid <> pg_backend_pid();" > /dev/null 2>&1 || true

    # Drop database
    dropdb -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" --if-exists "$TARGET_DB" || error_exit "Failed to drop database"

    unset PGPASSWORD

    log_info "Database dropped successfully"
}

# Create database
create_database() {
    if database_exists; then
        log_info "Database already exists: $TARGET_DB"
        return 0
    fi

    log_step "Creating database: $TARGET_DB"

    export PGPASSWORD="$DB_PASSWORD"

    createdb -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" "$TARGET_DB" || error_exit "Failed to create database"

    unset PGPASSWORD

    log_info "Database created successfully"
}

# Perform restore
perform_restore() {
    log_step "Restoring database from backup..."
    log_info "Backup file: $BACKUP_FILE"
    log_info "Target database: $TARGET_DB"

    export PGPASSWORD="$DB_PASSWORD"

    # Perform restore
    pg_restore -h "$DB_HOST" \
               -p "$DB_PORT" \
               -U "$DB_USER" \
               -d "$TARGET_DB" \
               --clean \
               --if-exists \
               --single-transaction \
               --no-owner \
               --no-acl \
               --verbose \
               "$BACKUP_FILE" 2>>"$LOG_FILE" || error_exit "Restore failed"

    unset PGPASSWORD

    log_info "Database restore completed successfully"
}

# Verify restore
verify_restore() {
    log_step "Verifying restore..."

    export PGPASSWORD="$DB_PASSWORD"

    # Count tables
    TABLE_COUNT=$(psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$TARGET_DB" -t -c \
        "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public';" | xargs)

    log_info "Tables in restored database: $TABLE_COUNT"

    if [ "$TABLE_COUNT" -eq 0 ]; then
        log_warn "No tables found in restored database"
    fi

    unset PGPASSWORD
}

# Create restore point
create_restore_point() {
    log_step "Creating restore point..."

    RESTORE_POINT="restore_${TIMESTAMP}"

    export PGPASSWORD="$DB_PASSWORD"

    psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$TARGET_DB" -c \
        "SELECT pg_create_restore_point('$RESTORE_POINT');" > /dev/null 2>&1 || true

    unset PGPASSWORD

    log_info "Restore point created: $RESTORE_POINT"
}

# Confirmation prompt
confirm_restore() {
    if $DROP_EXISTING; then
        echo -e "${RED}WARNING: This will DROP the existing database '$TARGET_DB' and all its data!${NC}"
    fi

    echo -e "${YELLOW}You are about to restore:${NC}"
    echo -e "  Backup file: $BACKUP_FILE"
    echo -e "  Target database: $TARGET_DB"
    echo -e "  Host: $DB_HOST:$DB_PORT"
    echo ""

    read -p "Are you sure you want to continue? (yes/no): " -r
    echo

    if [[ ! $REPLY =~ ^[Yy][Ee][Ss]$ ]]; then
        log_info "Restore cancelled by user"
        exit 0
    fi
}

# Main execution
main() {
    log_info "=== Starting Donelist PostgreSQL Restore ==="

    check_dependencies
    download_from_s3
    verify_backup
    check_connection
    confirm_restore
    drop_database
    create_database
    perform_restore
    verify_restore
    create_restore_point

    log_info "=== Restore Completed Successfully ==="
    log_info "Database '$TARGET_DB' has been restored from: $BACKUP_FILE"
    log_info "Log file: $LOG_FILE"
}

# Run main function
main "$@"
