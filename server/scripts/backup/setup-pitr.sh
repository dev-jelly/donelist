#!/bin/bash

# PostgreSQL PITR Setup Script
# This script configures PostgreSQL for Point-In-Time Recovery with WAL archiving

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
POSTGRES_VERSION="${POSTGRES_VERSION:-15}"
POSTGRES_DATA_DIR="${POSTGRES_DATA_DIR:-/var/lib/postgresql/$POSTGRES_VERSION/main}"
POSTGRES_CONF="${POSTGRES_CONF:-/etc/postgresql/$POSTGRES_VERSION/main/postgresql.conf}"
POSTGRES_USER="${POSTGRES_USER:-postgres}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# Logging
log_info() {
    echo -e "${GREEN}[INFO]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1"
}

log_step() {
    echo -e "${BLUE}[STEP]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1"
}

log_success() {
    echo -e "${CYAN}[SUCCESS]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1"
}

error_exit() {
    log_error "$1"
    exit 1
}

# Check if running as root or with sudo
check_permissions() {
    if [ "$EUID" -ne 0 ]; then
        error_exit "This script must be run as root or with sudo"
    fi
}

# Detect PostgreSQL installation
detect_postgres() {
    log_step "Detecting PostgreSQL installation..."

    if ! command -v psql &> /dev/null; then
        error_exit "PostgreSQL is not installed"
    fi

    # Try to find PostgreSQL version
    if [ -d "/etc/postgresql" ]; then
        POSTGRES_VERSION=$(ls /etc/postgresql/ | sort -V | tail -1)
        POSTGRES_CONF="/etc/postgresql/$POSTGRES_VERSION/main/postgresql.conf"
        POSTGRES_DATA_DIR="/var/lib/postgresql/$POSTGRES_VERSION/main"
    fi

    log_info "PostgreSQL version: $POSTGRES_VERSION"
    log_info "Config file: $POSTGRES_CONF"
    log_info "Data directory: $POSTGRES_DATA_DIR"

    if [ ! -f "$POSTGRES_CONF" ]; then
        error_exit "PostgreSQL configuration file not found: $POSTGRES_CONF"
    fi
}

# Create WAL archive directory
create_wal_archive_dir() {
    log_step "Creating WAL archive directory..."

    mkdir -p "$WAL_ARCHIVE_DIR"
    chown postgres:postgres "$WAL_ARCHIVE_DIR"
    chmod 700 "$WAL_ARCHIVE_DIR"

    log_success "WAL archive directory created: $WAL_ARCHIVE_DIR"
}

# Backup current PostgreSQL configuration
backup_postgres_config() {
    log_step "Backing up PostgreSQL configuration..."

    local backup_file="${POSTGRES_CONF}.backup.$(date +%Y%m%d_%H%M%S)"
    cp "$POSTGRES_CONF" "$backup_file"

    log_success "Configuration backed up to: $backup_file"
}

# Configure PostgreSQL for WAL archiving
configure_wal_archiving() {
    log_step "Configuring PostgreSQL for WAL archiving..."

    # Check if already configured
    if grep -q "^archive_mode = on" "$POSTGRES_CONF"; then
        log_warn "WAL archiving appears to be already configured"
        read -p "Do you want to reconfigure? (yes/no): " -r
        if [[ ! $REPLY =~ ^[Yy][Ee][Ss]$ ]]; then
            return 0
        fi
    fi

    # Configure WAL settings
    cat >> "$POSTGRES_CONF" <<EOF

# ======================================
# Point-In-Time Recovery Configuration
# Added by setup-pitr.sh on $(date)
# ======================================

# WAL Level - 'replica' is required for PITR
wal_level = replica

# Enable WAL archiving
archive_mode = on

# Archive command - copies WAL files to archive location
archive_command = '${SCRIPT_DIR}/wal-archive.sh %p %f'

# Archive timeout - force archiving after this time (0 = disabled)
archive_timeout = 300  # 5 minutes

# WAL senders for streaming replication (optional, but recommended)
max_wal_senders = 3

# Keep at least this much WAL for recovery
wal_keep_size = 1GB  # PostgreSQL 13+
# For PostgreSQL 12 and below, use: wal_keep_segments = 64

# WAL retention for pg_rewind (optional)
wal_log_hints = on

# WAL file size (optional tuning)
# max_wal_size = 2GB
# min_wal_size = 80MB

# Checkpoint settings for better recovery performance
checkpoint_timeout = 15min
checkpoint_completion_target = 0.9

# Full page writes (required for PITR)
full_page_writes = on

# ======================================
EOF

    log_success "PostgreSQL configuration updated"
}

# Test archive command
test_archive_command() {
    log_step "Testing archive command..."

    # Make sure wal-archive.sh is executable
    if [ ! -x "$SCRIPT_DIR/wal-archive.sh" ]; then
        chmod +x "$SCRIPT_DIR/wal-archive.sh"
    fi

    # Create a dummy WAL file for testing
    local test_wal_file="/tmp/test_wal_$(date +%s)"
    echo "Test WAL file" > "$test_wal_file"

    # Test archive command as postgres user
    if su - postgres -c "$SCRIPT_DIR/wal-archive.sh $test_wal_file TEST_WAL_FILE" 2>/dev/null; then
        log_success "Archive command test passed"
        rm -f "$test_wal_file"
        return 0
    else
        log_error "Archive command test failed"
        rm -f "$test_wal_file"
        return 1
    fi
}

# Restart PostgreSQL
restart_postgres() {
    log_step "Restarting PostgreSQL..."

    echo -e "${YELLOW}PostgreSQL will be restarted to apply changes.${NC}"
    echo "This will cause a brief service interruption."
    read -p "Do you want to continue? (yes/no): " -r
    echo

    if [[ ! $REPLY =~ ^[Yy][Ee][Ss]$ ]]; then
        log_warn "PostgreSQL restart skipped. Changes will take effect after next restart."
        return 0
    fi

    if systemctl restart postgresql; then
        log_success "PostgreSQL restarted successfully"
        sleep 3  # Wait for PostgreSQL to fully start
    else
        error_exit "Failed to restart PostgreSQL"
    fi
}

# Verify PostgreSQL is running
verify_postgres_running() {
    log_step "Verifying PostgreSQL is running..."

    if systemctl is-active --quiet postgresql; then
        log_success "PostgreSQL is running"
    else
        error_exit "PostgreSQL is not running"
    fi
}

# Check WAL archiving status
check_wal_archiving_status() {
    log_step "Checking WAL archiving status..."

    local status=$(su - postgres -c "psql -t -c 'SHOW archive_mode;'" | xargs)
    local command=$(su - postgres -c "psql -t -c 'SHOW archive_command;'" | xargs)

    log_info "Archive mode: $status"
    log_info "Archive command: $command"

    if [ "$status" = "on" ]; then
        log_success "WAL archiving is enabled"
    else
        log_error "WAL archiving is not enabled"
        return 1
    fi

    # Check archiver statistics
    echo ""
    log_info "Archiver Statistics:"
    su - postgres -c "psql -c 'SELECT * FROM pg_stat_archiver;'"
}

# Create initial base backup
create_base_backup() {
    log_step "Creating initial base backup..."

    echo -e "${YELLOW}Would you like to create an initial base backup now?${NC}"
    echo "This is recommended to establish a baseline for PITR."
    read -p "Create backup? (yes/no): " -r
    echo

    if [[ $REPLY =~ ^[Yy][Ee][Ss]$ ]]; then
        if [ -x "$SCRIPT_DIR/backup.sh" ]; then
            log_info "Running backup script..."
            "$SCRIPT_DIR/backup.sh"
        else
            log_warn "Backup script not found or not executable"
        fi
    fi
}

# Setup monitoring
setup_monitoring() {
    log_step "Setting up WAL archiving monitoring..."

    # Add cron job for monitoring
    local cron_line="0 * * * * $SCRIPT_DIR/monitor.sh >> /var/log/donelist-monitor.log 2>&1"

    if crontab -l -u root 2>/dev/null | grep -F "$SCRIPT_DIR/monitor.sh" > /dev/null; then
        log_info "Monitoring cron job already exists"
    else
        echo -e "${YELLOW}Add monitoring cron job?${NC}"
        echo "This will check backup status every hour."
        read -p "Add cron job? (yes/no): " -r
        echo

        if [[ $REPLY =~ ^[Yy][Ee][Ss]$ ]]; then
            (crontab -l -u root 2>/dev/null; echo "$cron_line") | crontab -u root -
            log_success "Monitoring cron job added"
        fi
    fi
}

# Display configuration summary
display_summary() {
    echo ""
    echo -e "${CYAN}=========================================${NC}"
    echo -e "${CYAN}  PITR Configuration Summary${NC}"
    echo -e "${CYAN}=========================================${NC}"
    echo ""
    echo -e "PostgreSQL Version:   $POSTGRES_VERSION"
    echo -e "Config File:          $POSTGRES_CONF"
    echo -e "WAL Archive Dir:      $WAL_ARCHIVE_DIR"
    echo -e "Archive Script:       $SCRIPT_DIR/wal-archive.sh"
    echo ""
    echo -e "${GREEN}✓ WAL archiving enabled${NC}"
    echo -e "${GREEN}✓ Archive directory created${NC}"
    echo -e "${GREEN}✓ PostgreSQL configured${NC}"
    echo ""
    echo -e "${YELLOW}Next Steps:${NC}"
    echo "  1. Verify WAL files are being archived to: $WAL_ARCHIVE_DIR"
    echo "  2. Configure S3 upload in .env (S3_BUCKET, S3_ACCESS_KEY, etc.)"
    echo "  3. Set up regular backups with: $SCRIPT_DIR/backup.sh"
    echo "  4. Test recovery procedures with: $SCRIPT_DIR/recovery-rehearsal.sh"
    echo "  5. Monitor archiving with: $SCRIPT_DIR/monitor.sh"
    echo ""
    echo -e "${CYAN}=========================================${NC}"
    echo ""
}

# Display help information
show_help() {
    cat <<EOF
PostgreSQL PITR Setup Script

This script configures PostgreSQL for Point-In-Time Recovery (PITR)
by enabling WAL archiving and setting up the necessary infrastructure.

Usage: sudo $0 [OPTIONS]

Options:
    --help              Show this help message
    --wal-dir DIR       Set WAL archive directory (default: $WAL_ARCHIVE_DIR)
    --pg-version VER    Set PostgreSQL version (default: auto-detect)
    --skip-restart      Don't restart PostgreSQL (apply changes manually)
    --skip-backup       Don't create initial backup

Examples:
    # Basic setup with defaults
    sudo $0

    # Custom WAL directory
    sudo $0 --wal-dir /mnt/backup/wal

    # Specific PostgreSQL version
    sudo $0 --pg-version 14

Requirements:
    - PostgreSQL installed and running
    - Root or sudo access
    - Sufficient disk space for WAL files

For more information, see: scripts/backup/README.md
EOF
    exit 0
}

# Parse command line arguments
SKIP_RESTART=false
SKIP_BACKUP=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --help)
            show_help
            ;;
        --wal-dir)
            WAL_ARCHIVE_DIR="$2"
            shift 2
            ;;
        --pg-version)
            POSTGRES_VERSION="$2"
            shift 2
            ;;
        --skip-restart)
            SKIP_RESTART=true
            shift
            ;;
        --skip-backup)
            SKIP_BACKUP=true
            shift
            ;;
        *)
            log_error "Unknown option: $1"
            show_help
            ;;
    esac
done

# Main execution
main() {
    echo -e "${CYAN}"
    echo "========================================="
    echo "  PostgreSQL PITR Configuration"
    echo "  Point-In-Time Recovery Setup"
    echo "========================================="
    echo -e "${NC}"

    log_info "=== Starting PITR Setup ==="

    check_permissions
    detect_postgres
    create_wal_archive_dir
    backup_postgres_config
    configure_wal_archiving
    test_archive_command

    if ! $SKIP_RESTART; then
        restart_postgres
        verify_postgres_running
        check_wal_archiving_status
    else
        log_warn "PostgreSQL restart skipped. Remember to restart manually."
    fi

    if ! $SKIP_BACKUP; then
        create_base_backup
    fi

    setup_monitoring
    display_summary

    log_success "=== PITR Setup Completed ==="

    exit 0
}

# Run main function
main "$@"
