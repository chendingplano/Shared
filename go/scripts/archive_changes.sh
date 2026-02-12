#!/bin/bash
#
# archive_changes.sh
#
# Polls the logical replication slot and archives change files.
# This script should run as a daemon on the production server.
#
# Prerequisites:
#   - PostgreSQL with logical replication enabled
#   - Replication slot created (see setup_logical_replication.sh)
#   - PG_* environment variables set
#   - PG_BACKUP_DIR set for local archive
#   - Optional: PG_BACKUP_REMOTE_* for remote sync
#
# Usage:
#   ./archive_changes.sh [--once] [--tables table1,table2,...]
#
# The script outputs JSON change files to $PG_BACKUP_DIR/changes/
# Each file contains one JSON record per line:
#   {"table": "users", "op": "INSERT", "data": {...}, "lsn": "0/16B3D40", "ts": "..."}
#

set -e

# Configuration
POLL_INTERVAL="${DATA_SYNC_FREQ:-300}"  # Default 5 minutes
SLOT_NAME="${SYNCDATA_SLOT:-syncdata_slot}"
RUN_ONCE=false
TABLE_FILTER=""

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --once)
            RUN_ONCE=true
            shift
            ;;
        --tables)
            TABLE_FILTER="$2"
            shift 2
            ;;
        --slot)
            SLOT_NAME="$2"
            shift 2
            ;;
        --interval)
            POLL_INTERVAL="$2"
            shift 2
            ;;
        -h|--help)
            echo "Usage: $0 [options]"
            echo ""
            echo "Options:"
            echo "  --once           Run once and exit (don't loop)"
            echo "  --tables LIST    Comma-separated list of tables to include"
            echo "  --slot NAME      Replication slot name (default: syncdata_slot)"
            echo "  --interval SEC   Poll interval in seconds (default: 300)"
            echo "  -h, --help       Show this help"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Check required environment variables
check_env() {
    local var_name=$1
    local var_value="${!var_name}"
    if [[ -z "$var_value" ]]; then
        echo "Error: $var_name is not set"
        exit 1
    fi
}

check_env "PG_USER_NAME"
check_env "PG_PASSWORD"
check_env "PG_DB_NAME"
check_env "PG_BACKUP_DIR"

PG_HOST="${PG_HOST:-127.0.0.1}"
PG_PORT="${PG_PORT:-5432}"

export PGPASSWORD="$PG_PASSWORD"

# Create changes directory
CHANGES_DIR="$PG_BACKUP_DIR/changes"
mkdir -p "$CHANGES_DIR"

# Log file
LOG_FILE="$PG_BACKUP_DIR/logs/archive_changes.log"
mkdir -p "$(dirname "$LOG_FILE")"

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" | tee -a "$LOG_FILE"
}

log "Starting change archiver"
log "  Slot: $SLOT_NAME"
log "  Interval: ${POLL_INTERVAL}s"
log "  Output: $CHANGES_DIR"
if [[ -n "$TABLE_FILTER" ]]; then
    log "  Tables: $TABLE_FILTER"
fi

# Function to fetch and archive changes
archive_changes() {
    local timestamp=$(date '+%Y%m%d_%H%M%S')
    local output_file="$CHANGES_DIR/changes_${timestamp}.json"
    local temp_file="$CHANGES_DIR/.changes_${timestamp}.tmp"

    # Build wal2json options
    local options="'include-timestamp', 'true', 'include-lsn', 'true', 'format-version', '2'"

    if [[ -n "$TABLE_FILTER" ]]; then
        # Convert comma-separated to JSON array
        local tables_json=$(echo "$TABLE_FILTER" | tr ',' '\n' | sed 's/^/"/' | sed 's/$/"/' | tr '\n' ',' | sed 's/,$//')
        options="$options, 'add-tables', '[$tables_json]'"
    fi

    # Fetch changes using pg_logical_slot_get_changes
    # This consumes the changes (they won't be returned again)
    local query="SELECT data FROM pg_logical_slot_get_changes('$SLOT_NAME', NULL, NULL, $options);"

    # Execute query and transform output to our JSON format
    psql -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER_NAME" -d "$PG_DB_NAME" \
        -t -A -c "$query" 2>/dev/null | while read -r line; do

        # Skip empty lines
        [[ -z "$line" ]] && continue

        # wal2json format-version 2 outputs JSON objects
        # We need to transform them to our format
        # Input: {"action":"I","schema":"public","table":"users","columns":[...]}
        # Output: {"table":"users","op":"INSERT","data":{...},"lsn":"...","ts":"..."}

        echo "$line" | python3 -c "
import json
import sys

for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    try:
        data = json.loads(line)

        # Map action to operation
        action_map = {'I': 'INSERT', 'U': 'UPDATE', 'D': 'DELETE'}
        op = action_map.get(data.get('action', ''), data.get('action', ''))

        # Build output record
        out = {
            'table': data.get('table', ''),
            'op': op,
            'lsn': data.get('lsn', ''),
            'ts': data.get('timestamp', '')
        }

        # Extract column data
        if 'columns' in data:
            out['data'] = {col['name']: col['value'] for col in data['columns']}

        # Extract old keys for UPDATE/DELETE
        if 'identity' in data:
            out['old_keys'] = {col['name']: col['value'] for col in data['identity']}

        print(json.dumps(out))
    except json.JSONDecodeError:
        pass
" >> "$temp_file"

    done

    # Check if we got any changes
    if [[ -f "$temp_file" && -s "$temp_file" ]]; then
        mv "$temp_file" "$output_file"
        local count=$(wc -l < "$output_file")
        log "Archived $count changes to $output_file"

        # Sync to remote if configured
        if [[ -n "$PG_BACKUP_REMOTE_HOST" ]]; then
            sync_to_remote "$output_file"
        fi

        return 0
    else
        rm -f "$temp_file"
        return 1
    fi
}

# Function to sync file to remote
sync_to_remote() {
    local file="$1"

    local remote_user="${PG_BACKUP_REMOTE_USER:-$(whoami)}"
    local remote_dir="${PG_BACKUP_REMOTE_DIR:-$PG_BACKUP_DIR}/changes"
    local remote_port="${PG_BACKUP_REMOTE_PORT:-22}"

    # Ensure remote directory exists
    ssh -p "$remote_port" "$remote_user@$PG_BACKUP_REMOTE_HOST" "mkdir -p '$remote_dir'" 2>/dev/null || true

    # Sync file
    rsync -az -e "ssh -p $remote_port" "$file" "$remote_user@$PG_BACKUP_REMOTE_HOST:$remote_dir/" 2>/dev/null

    if [[ $? -eq 0 ]]; then
        log "Synced $(basename "$file") to remote"
    else
        log "Warning: Failed to sync $(basename "$file") to remote"
    fi
}

# Check slot exists
slot_exists() {
    local count=$(psql -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER_NAME" -d "$PG_DB_NAME" \
        -t -A -c "SELECT COUNT(*) FROM pg_replication_slots WHERE slot_name = '$SLOT_NAME';" 2>/dev/null)
    [[ "$count" == "1" ]]
}

# Main loop
main() {
    if ! slot_exists; then
        log "Error: Replication slot '$SLOT_NAME' does not exist"
        log "Run setup_logical_replication.sh --create-slot first"
        exit 1
    fi

    if [[ "$RUN_ONCE" == true ]]; then
        archive_changes || log "No changes to archive"
    else
        log "Starting poll loop (interval: ${POLL_INTERVAL}s)"

        while true; do
            archive_changes || true  # Don't exit on no changes

            sleep "$POLL_INTERVAL"
        done
    fi
}

# Handle signals
trap 'log "Received shutdown signal"; exit 0' SIGTERM SIGINT

main
