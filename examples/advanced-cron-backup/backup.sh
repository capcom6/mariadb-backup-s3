#!/bin/bash

# MariaDB Backup Script for CRON
# This script wraps the mariadb-backup-s3 tool for use with system CRON

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${ENV_FILE:-$SCRIPT_DIR/backup.env}"

# Check if environment file exists
if [ ! -f "$ENV_FILE" ]; then
    ts="$(date '+%Y-%m-%d %H:%M:%S')"
    msg="Environment file not found: $ENV_FILE"
    printf '[%s] ERROR: %s\n' "$ts" "$msg" >&2
    exit 1
fi

# Export all variables
set -a
source "$ENV_FILE"
set +a

# Configuration - Adjust these paths as needed
BACKUP_BINARY="${BACKUP_BINARY:-/usr/local/bin/mariadb-backup-s3}"
LOG_FILE="${LOG_FILE:-/var/log/mariadb-backup.log}"
ERROR_LOG="${ERROR_LOG:-/var/log/mariadb-backup-error.log}"
NOTIFY_EMAIL="${NOTIFY_EMAIL:-}"

# Ensure log directory exists
mkdir -p "$(dirname "$LOG_FILE")"
mkdir -p "$(dirname "$ERROR_LOG")"

# Logging function
log() {
    printf '[%s] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$1" >> "$LOG_FILE"
}

# Error logging function
error_log() {
    printf '[%s] ERROR: %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$1" >> "$ERROR_LOG"
    printf '[%s] ERROR: %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$1" >> "$LOG_FILE"
}

# Email notification function
send_notification() {
    if [ -n "$NOTIFY_EMAIL" ]; then
        local subject="MariaDB Backup Failed - $(hostname)"
        local message="MariaDB backup failed on $(hostname) at $(date).

Error: $1

Check the logs for more details:
- Main log: $LOG_FILE
- Error log: $ERROR_LOG"

        echo "$message" | mail -s "$subject" "$NOTIFY_EMAIL" || error_log "Failed to send notification email"
    fi
}

# Check if backup binary exists
if [ ! -f "$BACKUP_BINARY" ]; then
    error_log "Backup binary not found: $BACKUP_BINARY"
    send_notification "Backup binary not found: $BACKUP_BINARY"
    exit 1
fi

# Check if backup binary is executable
if [ ! -x "$BACKUP_BINARY" ]; then
    error_log "Backup binary is not executable: $BACKUP_BINARY"
    send_notification "Backup binary is not executable: $BACKUP_BINARY"
    exit 1
fi

# Log backup start
log "Starting MariaDB backup"

# Run the backup with environment variables
set +euo pipefail  # Temporarily disable for the backup command
if output=$("$BACKUP_BINARY" 2>&1); then
    set -euo pipefail
    log "Backup completed successfully"
    log "Output: $output"
    exit 0
else
    exit_code=$?
    set -euo pipefail
    error_log "Backup failed with exit code $exit_code"
    error_log "Output: $output"
    send_notification "Backup failed with exit code $exit_code. Check logs for details."
    exit $exit_code
fi
