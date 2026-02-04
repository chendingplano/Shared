#!/bin/bash
# archive_wal.sh - PostgreSQL WAL archive script
#
# This is a reference/template script. The actual script is generated
# during 'pgbackup init' with paths customized for your environment.
#
# PostgreSQL Configuration:
#   archive_command = '/path/to/archive_wal.sh %p %f'
#
# Arguments:
#   $1 = Full path to WAL file (%p)
#   $2 = WAL file name only (%f)
#
# Environment Variables:
#   PG_BACKUP_DIR - Base backup directory (default: ~/backups/postgresql)

set -euo pipefail

WAL_SOURCE="$1"
WAL_FILENAME="$2"

# Get archive directory from environment or use default
BACKUP_DIR="${PG_BACKUP_DIR:-$HOME/backups/postgresql}"
ARCHIVE_DIR="$BACKUP_DIR/wal_archive"
LOG_FILE="$BACKUP_DIR/logs/wal_archive.log"

log() {
    echo "$(date '+%Y-%m-%d %H:%M:%S') - $1" >> "$LOG_FILE"
}

# Ensure directories exist
mkdir -p "$ARCHIVE_DIR"
mkdir -p "$(dirname "$LOG_FILE")"

# Check if already archived (idempotent - PostgreSQL may retry)
DEST="$ARCHIVE_DIR/$WAL_FILENAME.gz"
if [ -f "$DEST" ]; then
    log "WAL file already archived: $WAL_FILENAME"
    exit 0
fi

# Archive the WAL file with compression
log "Archiving WAL file: $WAL_FILENAME"
TEMP_DEST="$DEST.tmp"

if gzip -c "$WAL_SOURCE" > "$TEMP_DEST"; then
    mv "$TEMP_DEST" "$DEST"
    # Get file size (compatible with both macOS and Linux)
    SIZE=$(stat -f%z "$DEST" 2>/dev/null || stat -c%s "$DEST" 2>/dev/null || echo "unknown")
    log "Successfully archived: $WAL_FILENAME ($SIZE bytes)"
    exit 0
else
    rm -f "$TEMP_DEST"
    log "ERROR: Failed to archive $WAL_FILENAME"
    exit 1
fi
