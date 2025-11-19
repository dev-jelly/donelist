#!/bin/bash

# Recovery Rehearsal and Testing Script
# This script simulates disaster recovery scenarios and validates backup/restore procedures

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
TEST_DB="${TEST_DB:-donelist_rehearsal}"
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_NAME="${DB_NAME:-donelist}"
DB_USER="${DB_USER:-donelist}"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
REHEARSAL_LOG="$BACKUP_DIR/rehearsal_${TIMESTAMP}.log"
REPORT_FILE="$BACKUP_DIR/rehearsal_report_${TIMESTAMP}.md"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# Test results tracking
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0
WARNINGS=0
START_TIME=$(date +%s)

# Logging functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1" | tee -a "$REHEARSAL_LOG"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1" | tee -a "$REHEARSAL_LOG"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1" | tee -a "$REHEARSAL_LOG"
    ((WARNINGS++))
}

log_test() {
    echo -e "${BLUE}[TEST]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1" | tee -a "$REHEARSAL_LOG"
}

log_success() {
    echo -e "${CYAN}[PASS]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1" | tee -a "$REHEARSAL_LOG"
    ((PASSED_TESTS++))
}

log_fail() {
    echo -e "${RED}[FAIL]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1" | tee -a "$REHEARSAL_LOG"
    ((FAILED_TESTS++))
}

# Test execution wrapper
run_test() {
    local test_name="$1"
    local test_function="$2"

    ((TOTAL_TESTS++))
    log_test "Running: $test_name"

    if $test_function; then
        log_success "$test_name"
        return 0
    else
        log_fail "$test_name"
        return 1
    fi
}

# Cleanup function
cleanup() {
    log_info "Cleaning up test environment..."

    export PGPASSWORD="$DB_PASSWORD"

    # Drop test database if exists
    dropdb -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" --if-exists "$TEST_DB" 2>/dev/null || true

    # Remove temporary files
    rm -f /tmp/rehearsal_*.sql 2>/dev/null || true

    unset PGPASSWORD

    log_info "Cleanup completed"
}

# Trap cleanup on exit
trap cleanup EXIT

# Test 1: Backup script availability and permissions
test_backup_script_exists() {
    if [ -x "$SCRIPT_DIR/backup.sh" ]; then
        return 0
    else
        return 1
    fi
}

# Test 2: Restore script availability and permissions
test_restore_script_exists() {
    if [ -x "$SCRIPT_DIR/restore.sh" ]; then
        return 0
    else
        return 1
    fi
}

# Test 3: PITR script availability and permissions
test_pitr_script_exists() {
    if [ -x "$SCRIPT_DIR/pitr-restore.sh" ]; then
        return 0
    else
        return 1
    fi
}

# Test 4: Database connectivity
test_database_connection() {
    export PGPASSWORD="$DB_PASSWORD"

    if psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -c "SELECT 1" > /dev/null 2>&1; then
        unset PGPASSWORD
        return 0
    else
        unset PGPASSWORD
        return 1
    fi
}

# Test 5: Backup directory exists and is writable
test_backup_directory() {
    if [ -d "$BACKUP_DIR" ] && [ -w "$BACKUP_DIR" ]; then
        return 0
    else
        return 1
    fi
}

# Test 6: Recent backup exists
test_recent_backup_exists() {
    local recent_backup=$(find "$BACKUP_DIR" -name "postgres_full_*.sql.gz" -mtime -2 -type f | head -1)

    if [ -n "$recent_backup" ]; then
        log_info "Found recent backup: $recent_backup"
        return 0
    else
        log_warn "No backup found within last 2 days"
        return 1
    fi
}

# Test 7: Create test backup
test_create_backup() {
    log_info "Creating test backup..."

    export PGPASSWORD="$DB_PASSWORD"

    local test_backup="$BACKUP_DIR/rehearsal_backup_${TIMESTAMP}.sql.gz"

    if pg_dump -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" \
               --format=custom --no-owner --no-acl 2>>"$REHEARSAL_LOG" | gzip > "$test_backup"; then

        # Verify backup file was created and has size > 0
        if [ -f "$test_backup" ] && [ -s "$test_backup" ]; then
            local size=$(stat -f%z "$test_backup" 2>/dev/null || stat -c%s "$test_backup" 2>/dev/null)
            log_info "Test backup created: $test_backup ($(($size / 1024 / 1024))MB)"
            unset PGPASSWORD
            return 0
        fi
    fi

    unset PGPASSWORD
    return 1
}

# Test 8: Verify backup integrity
test_backup_integrity() {
    local latest_backup=$(find "$BACKUP_DIR" -name "rehearsal_backup_*.sql.gz" -type f | sort | tail -1)

    if [ -z "$latest_backup" ]; then
        log_warn "No test backup found to verify"
        return 1
    fi

    log_info "Verifying backup integrity: $latest_backup"

    if pg_restore --list "$latest_backup" > /dev/null 2>&1; then
        return 0
    else
        return 1
    fi
}

# Test 9: Restore to test database
test_restore_to_test_db() {
    local latest_backup=$(find "$BACKUP_DIR" -name "rehearsal_backup_*.sql.gz" -type f | sort | tail -1)

    if [ -z "$latest_backup" ]; then
        log_warn "No test backup found for restore"
        return 1
    fi

    log_info "Restoring to test database: $TEST_DB"

    export PGPASSWORD="$DB_PASSWORD"

    # Drop test DB if exists
    dropdb -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" --if-exists "$TEST_DB" 2>/dev/null || true

    # Create test DB
    createdb -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" "$TEST_DB" || {
        unset PGPASSWORD
        return 1
    }

    # Restore backup
    if pg_restore -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$TEST_DB" \
                  --no-owner --no-acl "$latest_backup" 2>>"$REHEARSAL_LOG"; then
        unset PGPASSWORD
        return 0
    else
        unset PGPASSWORD
        return 1
    fi
}

# Test 10: Verify restored data
test_verify_restored_data() {
    export PGPASSWORD="$DB_PASSWORD"

    # Count tables in test database
    local table_count=$(psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$TEST_DB" -t -c \
        "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public';" | xargs)

    log_info "Tables in test database: $table_count"

    if [ "$table_count" -gt 0 ]; then
        # Verify some data exists
        if psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$TEST_DB" -c \
            "SELECT COUNT(*) FROM users;" > /dev/null 2>&1; then

            local user_count=$(psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$TEST_DB" -t -c \
                "SELECT COUNT(*) FROM users;" | xargs)

            log_info "Users in test database: $user_count"
            unset PGPASSWORD
            return 0
        fi
    fi

    unset PGPASSWORD
    return 1
}

# Test 11: WAL archive directory exists
test_wal_archive_exists() {
    local wal_dir="${WAL_ARCHIVE_DIR:-/var/backups/donelist/wal}"

    if [ -d "$wal_dir" ]; then
        local wal_count=$(find "$wal_dir" -name "0*" -type f 2>/dev/null | wc -l | xargs)
        log_info "WAL files in archive: $wal_count"
        return 0
    else
        log_warn "WAL archive directory not found: $wal_dir"
        return 1
    fi
}

# Test 12: S3 connectivity (if configured)
test_s3_connectivity() {
    if [ -z "${S3_BUCKET:-}" ]; then
        log_info "S3 not configured, skipping test"
        return 0
    fi

    if ! command -v aws &> /dev/null; then
        log_warn "AWS CLI not installed"
        return 1
    fi

    log_info "Testing S3 connectivity..."

    if aws s3 ls "s3://$S3_BUCKET/" > /dev/null 2>&1; then
        return 0
    else
        log_warn "Cannot access S3 bucket: $S3_BUCKET"
        return 1
    fi
}

# Test 13: Backup checksum verification
test_backup_checksum() {
    local latest_backup=$(find "$BACKUP_DIR" -name "rehearsal_backup_*.sql.gz" -type f | sort | tail -1)

    if [ -z "$latest_backup" ]; then
        log_warn "No test backup found"
        return 1
    fi

    log_info "Calculating checksum..."

    local checksum=$(sha256sum "$latest_backup" | awk '{print $1}')
    echo "$checksum" > "${latest_backup}.sha256"

    log_info "Checksum: $checksum"

    # Verify checksum file was created
    if [ -f "${latest_backup}.sha256" ]; then
        return 0
    else
        return 1
    fi
}

# Test 14: Performance benchmark - backup speed
test_backup_performance() {
    log_info "Testing backup performance..."

    local start_time=$(date +%s)

    export PGPASSWORD="$DB_PASSWORD"

    local perf_backup="$BACKUP_DIR/perf_test_${TIMESTAMP}.sql.gz"

    if pg_dump -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" \
               --format=custom --no-owner --no-acl 2>/dev/null | gzip > "$perf_backup"; then

        local end_time=$(date +%s)
        local duration=$((end_time - start_time))
        local size=$(stat -f%z "$perf_backup" 2>/dev/null || stat -c%s "$perf_backup" 2>/dev/null)
        local size_mb=$((size / 1024 / 1024))

        log_info "Backup completed in ${duration}s (${size_mb}MB)"

        # Remove performance test backup
        rm -f "$perf_backup"

        unset PGPASSWORD

        # Consider successful if < 5 minutes
        if [ $duration -lt 300 ]; then
            return 0
        else
            log_warn "Backup took longer than expected: ${duration}s"
            return 1
        fi
    fi

    unset PGPASSWORD
    return 1
}

# Test 15: Performance benchmark - restore speed
test_restore_performance() {
    local latest_backup=$(find "$BACKUP_DIR" -name "rehearsal_backup_*.sql.gz" -type f | sort | tail -1)

    if [ -z "$latest_backup" ]; then
        log_warn "No test backup found"
        return 1
    fi

    log_info "Testing restore performance..."

    local start_time=$(date +%s)

    export PGPASSWORD="$DB_PASSWORD"

    local perf_db="${TEST_DB}_perf"

    # Drop if exists
    dropdb -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" --if-exists "$perf_db" 2>/dev/null || true

    # Create database
    createdb -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" "$perf_db" || {
        unset PGPASSWORD
        return 1
    }

    # Restore
    if pg_restore -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$perf_db" \
                  --no-owner --no-acl "$latest_backup" 2>/dev/null; then

        local end_time=$(date +%s)
        local duration=$((end_time - start_time))

        log_info "Restore completed in ${duration}s"

        # Cleanup
        dropdb -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" --if-exists "$perf_db" 2>/dev/null || true

        unset PGPASSWORD

        # Consider successful if < 10 minutes
        if [ $duration -lt 600 ]; then
            return 0
        else
            log_warn "Restore took longer than expected: ${duration}s"
            return 1
        fi
    fi

    unset PGPASSWORD
    return 1
}

# Generate rehearsal report
generate_report() {
    local end_time=$(date +%s)
    local total_duration=$((end_time - START_TIME))

    cat > "$REPORT_FILE" <<EOF
# Disaster Recovery Rehearsal Report

**Date**: $(date)
**Duration**: ${total_duration} seconds
**Environment**: ${SERVER_ENV:-development}

## Executive Summary

This rehearsal tested the disaster recovery procedures for the Donelist database backup system.

### Results Overview

- **Total Tests**: $TOTAL_TESTS
- **Passed**: ${GREEN}$PASSED_TESTS${NC}
- **Failed**: ${RED}$FAILED_TESTS${NC}
- **Warnings**: ${YELLOW}$WARNINGS${NC}
- **Success Rate**: $(( PASSED_TESTS * 100 / TOTAL_TESTS ))%

## Test Results

### Infrastructure Tests
EOF

    # Add detailed results
    echo "" >> "$REPORT_FILE"
    echo "See detailed log: $REHEARSAL_LOG" >> "$REPORT_FILE"

    echo "" >> "$REPORT_FILE"
    echo "## Backup Files Verified" >> "$REPORT_FILE"
    echo "" >> "$REPORT_FILE"

    find "$BACKUP_DIR" -name "rehearsal_backup_*.sql.gz" -type f | while read backup; do
        local size=$(stat -f%z "$backup" 2>/dev/null || stat -c%s "$backup" 2>/dev/null)
        local size_mb=$((size / 1024 / 1024))
        echo "- $(basename "$backup") - ${size_mb}MB" >> "$REPORT_FILE"
    done

    cat >> "$REPORT_FILE" <<EOF

## Recommendations

EOF

    if [ $FAILED_TESTS -gt 0 ]; then
        cat >> "$REPORT_FILE" <<EOF
### Critical Issues
- ❌ $FAILED_TESTS test(s) failed - immediate attention required
- Review failed tests in log: $REHEARSAL_LOG
- Do not proceed with production recovery until issues are resolved

EOF
    fi

    if [ $WARNINGS -gt 0 ]; then
        cat >> "$REPORT_FILE" <<EOF
### Warnings
- ⚠️  $WARNINGS warning(s) detected - review recommended
- Some optional features may not be available
- Consider addressing warnings before next rehearsal

EOF
    fi

    cat >> "$REPORT_FILE" <<EOF
### Next Steps

1. Review any failed tests and resolve issues
2. Update recovery procedures based on findings
3. Schedule next rehearsal (recommended: quarterly)
4. Train team members on recovery procedures
5. Update contact information in runbooks

## Sign-off

**Rehearsal Conducted By**: [Name]
**Reviewed By**: [Name]
**Approved By**: [Name]

---

**Status**: $([ $FAILED_TESTS -eq 0 ] && echo "✅ PASSED" || echo "❌ FAILED")

**Generated**: $(date)
EOF

    log_info "Report generated: $REPORT_FILE"
}

# Display summary
display_summary() {
    echo ""
    echo -e "${CYAN}=========================================${NC}"
    echo -e "${CYAN}  Recovery Rehearsal Summary${NC}"
    echo -e "${CYAN}=========================================${NC}"
    echo ""
    echo -e "Total Tests:    $TOTAL_TESTS"
    echo -e "Passed:         ${GREEN}$PASSED_TESTS${NC}"
    echo -e "Failed:         ${RED}$FAILED_TESTS${NC}"
    echo -e "Warnings:       ${YELLOW}$WARNINGS${NC}"
    echo -e "Success Rate:   $(( PASSED_TESTS * 100 / TOTAL_TESTS ))%"
    echo ""
    echo -e "Log File:       $REHEARSAL_LOG"
    echo -e "Report File:    $REPORT_FILE"
    echo ""

    if [ $FAILED_TESTS -eq 0 ]; then
        echo -e "${GREEN}✅ All tests passed! Recovery procedures are operational.${NC}"
    else
        echo -e "${RED}❌ Some tests failed. Review the log and fix issues before production recovery.${NC}"
    fi

    echo -e "${CYAN}=========================================${NC}"
    echo ""
}

# Main execution
main() {
    echo -e "${CYAN}"
    echo "========================================="
    echo "  Disaster Recovery Rehearsal"
    echo "  Donelist Backup System Testing"
    echo "========================================="
    echo -e "${NC}"

    log_info "=== Starting Recovery Rehearsal ==="
    log_info "Test database: $TEST_DB"
    log_info "Backup directory: $BACKUP_DIR"

    # Run all tests
    run_test "Backup script exists and is executable" test_backup_script_exists
    run_test "Restore script exists and is executable" test_restore_script_exists
    run_test "PITR script exists and is executable" test_pitr_script_exists
    run_test "Database connectivity" test_database_connection
    run_test "Backup directory exists and is writable" test_backup_directory
    run_test "Recent backup exists (within 2 days)" test_recent_backup_exists
    run_test "Create test backup" test_create_backup
    run_test "Verify backup integrity" test_backup_integrity
    run_test "Backup checksum generation" test_backup_checksum
    run_test "Restore backup to test database" test_restore_to_test_db
    run_test "Verify restored data" test_verify_restored_data
    run_test "WAL archive directory exists" test_wal_archive_exists
    run_test "S3 connectivity (if configured)" test_s3_connectivity
    run_test "Backup performance benchmark" test_backup_performance
    run_test "Restore performance benchmark" test_restore_performance

    # Generate report
    generate_report

    # Display summary
    display_summary

    log_info "=== Recovery Rehearsal Completed ==="

    # Exit with appropriate code
    if [ $FAILED_TESTS -eq 0 ]; then
        exit 0
    else
        exit 1
    fi
}

# Run main function
main "$@"
