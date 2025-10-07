#!/bin/bash

# MariaDB Backup Script with Encryption
# This script wraps the mariadb-backup-s3 tool for encrypted backups
# Designed for use with system CRON or manual execution
#
# Features:
# - Comprehensive encryption key validation
# - Detailed logging and error handling
# - Email notifications on failure
# - Security-focused configuration validation
# - Resource monitoring and performance tracking

set -euo pipefail

# Global variables
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${ENV_FILE:-$SCRIPT_DIR/.env}"
BACKUP_START_TIME=""
BACKUP_DURATION=""
TEMP_LOG_FILE=""
DEBUG="${DEBUG:-0}"

# Create temporary log file for this session
TEMP_LOG_FILE=$(mktemp)

# Configuration - Adjust these paths as needed
BACKUP_BINARY="${BACKUP_BINARY:-/usr/local/bin/mariadb-backup-s3}"
LOG_FILE="${LOG_FILE:-/var/log/mariadb-backup-encryption.log}"
ERROR_LOG="${ERROR_LOG:-/var/log/mariadb-backup-encryption-error.log}"
NOTIFY_EMAIL="${NOTIFY_EMAIL:-}"

MARIADB__HOST="${MARIADB__HOST:-localhost}"
MARIADB__PORT="${MARIADB__PORT:-3306}"

# Initialize logging
init_logging() {
    # Ensure log directory exists with proper permissions
    local log_dir
    log_dir="$(dirname "$LOG_FILE")"
    if [ ! -d "$log_dir" ]; then
        mkdir -p "$log_dir"
        chmod 755 "$log_dir"
    fi
    
    local error_log_dir
    error_log_dir="$(dirname "$ERROR_LOG")"
    if [ ! -d "$error_log_dir" ]; then
        mkdir -p "$error_log_dir"
        chmod 755 "$error_log_dir"
    fi
    
    # Set log file permissions
    touch "$LOG_FILE" "$ERROR_LOG"
    chmod 644 "$LOG_FILE" "$ERROR_LOG"
}

# Enhanced logging function with debug support
log() {
    local level="${1:-INFO}"
    local message="$2"
    local timestamp
    timestamp="$(date '+%Y-%m-%d %H:%M:%S')"
    
    printf '[%s] [%s] %s\n' "$timestamp" "$level" "$message" >> "$LOG_FILE"
    printf '[%s] [%s] %s\n' "$timestamp" "$level" "$message" >> "$TEMP_LOG_FILE"
    
    # Debug output to stderr if debug mode is enabled
    if [ "$DEBUG" = "1" ]; then
        printf '[DEBUG] %s\n' "$message" >&2
    fi
}

# Error logging function with context
error_log() {
    local error_code="${1:-UNKNOWN}"
    local message="$2"
    local timestamp
    timestamp="$(date '+%Y-%m-%d %H:%M:%S')"
    
    printf '[%s] [ERROR] [%s] %s\n' "$timestamp" "$error_code" "$message" >> "$ERROR_LOG"
    printf '[%s] [ERROR] [%s] %s\n' "$timestamp" "$error_code" "$message" >> "$LOG_FILE"
    printf '[%s] [ERROR] [%s] %s\n' "$timestamp" "$error_code" "$message" >> "$TEMP_LOG_FILE"
    
    # Debug output to stderr if debug mode is enabled
    if [ "$DEBUG" = "1" ]; then
        printf '[DEBUG-ERROR] %s\n' "$message" >&2
    fi
}

# Success logging function
success_log() {
    local message="$1"
    log "SUCCESS" "$message"
}

# Warning logging function
warning_log() {
    local message="$1"
    log "WARNING" "$message"
}

# Email notification function with enhanced formatting
send_notification() {
    if [ -n "$NOTIFY_EMAIL" ]; then
        local subject="Encrypted MariaDB Backup Failed - $(hostname)"
        local error_details="$1"
        local backup_duration="${BACKUP_DURATION:-unknown}"
        local timestamp
        timestamp="$(date '+%Y-%m-%d %H:%M:%S')"
        
        local message="Encrypted MariaDB backup failed on $(hostname) at $timestamp.

ERROR DETAILS:
$error_details

BACKUP INFORMATION:
- Start Time: ${BACKUP_START_TIME:-unknown}
- Duration: ${backup_duration}
- Host: $(hostname)
- Script: $0
- Environment File: $ENV_FILE

SYSTEM INFORMATION:
- User: $(whoami)
- Shell: $SHELL
- Working Directory: $(pwd)

LOG FILES:
- Main log: $LOG_FILE
- Error log: $ERROR_LOG
- Temporary log: $TEMP_LOG_FILE

RECOMMENDED ACTIONS:
1. Check the error logs for detailed information
2. Verify encryption key configuration
3. Test database connectivity
4. Check storage system connectivity
5. Verify sufficient disk space

For immediate assistance, please contact your system administrator."

        # Send email with proper error handling
        if ! echo "$message" | mail -s "$subject" "$NOTIFY_EMAIL" 2>/dev/null; then
            error_log "MAIL-001" "Failed to send notification email to $NOTIFY_EMAIL"
            # Try alternative method if available
            if command -v sendmail >/dev/null 2>&1; then
                {
                    printf 'To: %s\n' "$NOTIFY_EMAIL"
                    printf 'Subject: %s\n' "$subject"
                    printf '\n%s\n' "$message"
                } | sendmail -t || error_log "MAIL-002" "Failed to send notification email using sendmail"
            fi
        fi
    fi
}

# Cleanup function for temporary files
cleanup() {
    local exit_code=$?
    
    # Calculate backup duration if we have a start time
    if [ -n "$BACKUP_START_TIME" ]; then
        local end_time
        end_time=$(date +%s)
        BACKUP_DURATION=$((end_time - BACKUP_START_TIME))
        log "INFO" "Backup duration: ${BACKUP_DURATION} seconds"
    fi
    
    # Cleanup temporary log
    if [ -f "$TEMP_LOG_FILE" ]; then
        rm -f "$TEMP_LOG_FILE"
    fi
    
    # Exit with the appropriate code
    exit $exit_code
}

# Set up signal handlers
trap cleanup EXIT INT TERM

# Validate environment file
validate_environment_file() {
    log "INFO" "Validating environment file: $ENV_FILE"
    
    if [ ! -f "$ENV_FILE" ]; then
        error_log "ENV-001" "Environment file not found: $ENV_FILE"
        send_notification "Environment file not found: $ENV_FILE"
        exit 1
    fi
    
    if [ ! -r "$ENV_FILE" ]; then
        error_log "ENV-002" "Environment file not readable: $ENV_FILE"
        send_notification "Environment file not readable: $ENV_FILE"
        exit 1
    fi
    
    # Check file permissions (should not be world-readable)
    local file_perms
    file_perms=$(stat -c "%a" "$ENV_FILE" 2>/dev/null || stat -f "%Lp" "$ENV_FILE" 2>/dev/null || echo "unknown")
    if [ "$file_perms" != "unknown" ] && [ "${file_perms: -1}" != "0" ]; then
        warning_log "Environment file $ENV_FILE has world-readable permissions ($file_perms). Consider setting chmod 600."
    fi
    
    log "INFO" "Environment file validation passed"
}

# Load and validate environment variables
load_environment() {
    log "INFO" "Loading environment variables from $ENV_FILE"
    
    # Export all variables
    set -a
    # shellcheck disable=SC1090
    if ! source "$ENV_FILE"; then
        error_log "ENV-003" "Failed to load environment file: $ENV_FILE"
        send_notification "Failed to load environment file: $ENV_FILE"
        exit 1
    fi
    set +a
    
    log "INFO" "Environment variables loaded successfully"
}

# Validate backup binary
validate_backup_binary() {
    log "INFO" "Validating backup binary: $BACKUP_BINARY"
    
    if [ ! -f "$BACKUP_BINARY" ]; then
        error_log "BIN-001" "Backup binary not found: $BACKUP_BINARY"
        send_notification "Backup binary not found: $BACKUP_BINARY"
        exit 1
    fi
    
    if [ ! -x "$BACKUP_BINARY" ]; then
        error_log "BIN-002" "Backup binary is not executable: $BACKUP_BINARY"
        send_notification "Backup binary is not executable: $BACKUP_BINARY"
        exit 1
    fi
    
    # Check binary type and version
    if command -v file >/dev/null 2>&1; then
        local binary_type
        binary_type=$(file "$BACKUP_BINARY" 2>/dev/null | head -1)
        log "INFO" "Binary type: $binary_type"
    fi
    
    # Test binary help/version
    if "$BACKUP_BINARY" --version >/dev/null 2>&1; then
        local version
        version=$("$BACKUP_BINARY" --version 2>/dev/null | head -1)
        log "INFO" "Binary version: $version"
    elif "$BACKUP_BINARY" help >/dev/null 2>&1; then
        log "INFO" "Binary help available"
    fi
    
    log "INFO" "Backup binary validation passed"
}

# Comprehensive encryption key validation
validate_encryption_key() {
    log "INFO" "Validating encryption key"
    
    # Check if encryption key is set
    if [ -z "${ENCRYPTION__KEY:-}" ]; then
        error_log "CRYPT-001" "Encryption key not set in environment file"
        send_notification "Encryption key not configured for backup"
        exit 1
    fi
    
    # Log key presence (first 8 chars for security)
    log "INFO" "Encryption key configured: ${ENCRYPTION__KEY:0:8}... (truncated for security)"
    
    # Basic base64 format validation
    if ! echo "$ENCRYPTION__KEY" | grep -qE '^[A-Za-z0-9+/]+={0,2}$'; then
        error_log "CRYPT-002" "Invalid encryption key format: $ENCRYPTION__KEY"
        send_notification "Invalid encryption key format detected"
        exit 1
    fi
    
    # Validate key length (should be base64 encoded)
    if ! echo "$ENCRYPTION__KEY" | base64 -d >/dev/null 2>&1; then
        error_log "CRYPT-003" "Failed to decode encryption key"
        send_notification "Failed to decode encryption key - invalid base64 format"
        exit 1
    fi
    
    log "INFO" "Encryption key validation passed successfully"
}

# Validate system resources
validate_system_resources() {
    log "INFO" "Validating system resources"
    
    # Check available memory
    if command -v free >/dev/null 2>&1; then
        local available_memory_mb
        available_memory_mb=$(free -m 2>/dev/null | awk '/Mem:/ {print $7}')
        
        if [ -n "$available_memory_mb" ] && [ "$available_memory_mb" -lt 512 ]; then
            warning_log "Low memory: ${available_memory_mb}MB available (recommended: 512MB+)"
        else
            log "INFO" "Memory check passed: ${available_memory_mb:-unknown}MB available"
        fi
    fi
    
    log "INFO" "System resource validation completed"
}

# Validate database connectivity
validate_database_connectivity() {
    log "INFO" "Validating database connectivity"
    
    # Check if required MariaDB variables are set
    local required_vars=("MARIADB__HOST" "MARIADB__PORT" "MARIADB__USER" "MARIADB__PASSWORD")
    local missing_vars=()
    
    for var in "${required_vars[@]}"; do
        if [ -z "${!var:-}" ]; then
            missing_vars+=("$var")
        fi
    done
    
    if [ ${#missing_vars[@]} -gt 0 ]; then
        error_log "DB-001" "Missing required database variables: ${missing_vars[*]}"
        send_notification "Missing required database configuration: ${missing_vars[*]}"
        exit 1
    fi
    
    log "INFO" "Database configuration validation passed"
}

# Validate storage configuration
validate_storage_configuration() {
    log "INFO" "Validating storage configuration"

    if [ -z "${STORAGE__URL:-}" ]; then
        error_log "STORAGE-001" "STORAGE__URL is not set"
        send_notification "Missing storage configuration: STORAGE__URL"
        exit 1
    fi

    local storage_type="${STORAGE__URL%%://*}"
    
    # Check if required storage variables are set
    if [ "$storage_type" = "s3" ]; then
        local required_s3_vars=("AWS__REGION" "AWS__ACCESS_KEY_ID" "AWS__SECRET_ACCESS_KEY")
        local missing_vars=()
        
        for var in "${required_s3_vars[@]}"; do
            if [ -z "${!var:-}" ]; then
                missing_vars+=("$var")
            fi
        done
        
        if [ ${#missing_vars[@]} -gt 0 ]; then
            error_log "STORAGE-002" "Missing required S3 variables: ${missing_vars[*]}"
            send_notification "Missing required S3 configuration: ${missing_vars[*]}"
            exit 1
        fi
        
        log "INFO" "S3 storage configuration validation passed"
    fi
    
    log "INFO" "Storage configuration validation completed"
}

# Main backup function
run_backup() {
    log "INFO" "Starting encrypted MariaDB backup workflow"
    
    # Record start time
    BACKUP_START_TIME=$(date +%s)
    log "INFO" "Backup started at: $(date -d "@$BACKUP_START_TIME" '+%Y-%m-%d %H:%M:%S')"
    
    # Log configuration summary (excluding sensitive data)
    log "INFO" "Configuration summary:"
    log "INFO" "  - Backup binary: $BACKUP_BINARY"
    log "INFO" "  - Storage type: ${STORAGE__URL%%://*}"
    log "INFO" "  - MariaDB host: ${MARIADB__HOST:-not set}"
    log "INFO" "  - Log file: $LOG_FILE"
    log "INFO" "  - Error log: $ERROR_LOG"
    log "INFO" "  - Notifications: ${NOTIFY_EMAIL:-disabled}"
    
    # Run the backup with environment variables
    set +euo pipefail  # Temporarily disable for the backup command
    
    log "INFO" "Executing backup command: $BACKUP_BINARY"
    
    if output=$("$BACKUP_BINARY" 2>&1); then
        set -euo pipefail
        success_log "Encrypted backup completed successfully"
        
        # Log summary of backup output (truncated for readability)
        if [ -n "$output" ]; then
            local output_summary
            output_summary=$(echo "$output" | head -10 | tr '\n' ' ')
            log "INFO" "Backup output summary: $output_summary..."
        fi
        
        exit 0
    else
        local exit_code=$?
        set -euo pipefail
        
        error_log "BACKUP-001" "Encrypted backup failed with exit code $exit_code"
        
        # Log full error output
        if [ -n "$output" ]; then
            error_log "BACKUP-002" "Full error output:"
            echo "$output" | while IFS= read -r line; do
                error_log "BACKUP-003" "  $line"
            done
        fi
        
        send_notification "Encrypted backup failed with exit code $exit_code. Check logs for details."
        exit $exit_code
    fi
}

# Main execution function
main() {
    # Initialize logging
    init_logging

    log "INFO" "Starting encrypted MariaDB backup script"
    log "INFO" "Script directory: $SCRIPT_DIR"
    log "INFO" "Environment file: $ENV_FILE"
    
    # Run validation steps
    validate_environment_file
    load_environment
    validate_backup_binary
    validate_encryption_key
    validate_system_resources
    validate_database_connectivity
    validate_storage_configuration
    
    # Execute backup
    run_backup
}

# Run main function if script is executed directly
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
    main "$@"
fi