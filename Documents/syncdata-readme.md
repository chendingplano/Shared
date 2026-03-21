# Data Sync Utility (syncdata)

## Overview

The **Data Sync Utility** (`syncdata`) is a Go-based service that synchronizes specific PostgreSQL tables from a production system to a local instance. It operates by consuming logical decoding change files archived from production, ensuring local environments stay updated for read-only analysis or reporting.

## Architecture

```
Production Server                    Backup Machine                    Local Machine
┌─────────────────┐                 ┌──────────────────┐              ┌─────────────────┐
│  PostgreSQL     │                 │                  │              │  PostgreSQL     │
│  (wal_level=    │   rsync/ssh     │  changes/*.json  │   sftp       │  (local)        │
│   logical)      │ ──────────────▶ │                  │ ◀────────────│                 │
│                 │                 │  wal_archive/    │              │  syncdata       │
│  archive_       │                 │                  │              │  daemon         │
│  changes.sh     │                 └──────────────────┘              └─────────────────┘
└─────────────────┘

Change files are JSON with one record per line:
{"table": "users", "op": "INSERT", "data": {...}, "lsn": "0/16B3D40", "ts": "..."}
```

## Prerequisites

### Production Server
- **PostgreSQL 18.1** with logical replication enabled
- **wal2json** extension installed
- A logical replication slot created for syncdata
- The `archive_changes.sh` script running as a daemon

### Backup Machine
- SSH access from production (for archiving)
- SSH access from local (for syncdata to read)
- Sufficient disk space for change files

### Local Machine
- **PostgreSQL 18.1** (same version as production)
- **Go 1.25+** for building syncdata
- SSH key-based authentication to backup machine
- Identical table schemas to production (for synced tables)

## Installation

### 1. Build the CLI

```bash
cd ~/Workspace/shared/go
GOWORK=off go build -o bin/syncdata ./cmd/syncdata
```

### 2. Create Configuration File

Create a TOML configuration file (e.g., `~/.config/syncdata/config.toml`):

```toml
# Archive source (backup machine)
archive_host = "backup.example.com"
archive_user = "backupuser"
archive_dir = "/backups/postgresql/changes"
archive_port = 22

# Local PostgreSQL
pg_host = "127.0.0.1"
pg_port = 5432
pg_user = "admin"
# pg_password can be set via PG_PASSWORD env var
# pg_database can be set via PG_DB_NAME env var

# Sync settings
data_sync_freq = 600   # Poll every 10 minutes
metric_freq = 24       # Aggregate metrics every 24 hours
```

### 3. Set Environment Variables

```bash
export DATA_SYNC_CONFIG=~/.config/syncdata/config.toml
export PG_PASSWORD=your_password
export PG_DB_NAME=your_database
```

If you use mise, add the environment variables in mise.local.toml:
```text
[env]
DATA_SYNC_CONFIG=~/.config/syncdata/config.toml
PG_PASSWORD=<password>
PG_DB_NAME=<dbname>
```

## Production Setup

### 1. Enable Logical Replication

On the production server:

```bash
cd ~/Workspace/shared/go/scripts
./setup_logical_replication.sh --create-slot --create-user
```

This will:
- Check that `wal_level = 'logical'`
- Create the `syncdata_slot` replication slot
- Optionally create a `syncdata_user` with REPLICATION privilege

If `wal_level` needs to change, restart PostgreSQL:

```bash
# macOS
brew services restart postgresql@18

# Linux
sudo systemctl restart postgresql
```

### 2. Start the Change Archiver

On the production server:

```bash
cd ~/Workspace/shared/go/scripts
./archive_changes.sh --tables users,orders,products
```

For daemon mode (run in background):

```bash
nohup ./archive_changes.sh --tables users,orders,products > /var/log/archive_changes.log 2>&1 &
```

The script will:
- Poll the replication slot every `DATA_SYNC_FREQ` seconds
- Write change files to `$PG_BACKUP_DIR/changes/`
- Sync files to the remote backup machine

## Usage

### Add Tables to Sync

```bash
syncdata add-tables users orders products
```

### List Synced Tables

```bash
syncdata list-tables
```

### Start the Daemon

```bash
syncdata start
```

The daemon runs in foreground. Use a process manager (systemd, launchd) for production.

### Check Status

```bash
syncdata status
```

Output:
```
status: active
sync frequency: 600 seconds
start time: 2026-02-05T10:30:00Z
last sync: 2026-02-05T10:45:00Z
records synced: 1234
errors: 0

synced tables (3):
  - users
  - orders
  - products
```

### Stop the Daemon

```bash
syncdata stop
```

### Resync a Table

If a table gets out of sync, you can resync it:

```bash
syncdata resync users
```

This truncates the local table and replays all changes from the archive.

### Clear All Data

```bash
syncdata clear
```

**Warning:** This truncates all synced tables!

### Remove Tables

```bash
syncdata remove-tables products
```

## Configuration Reference

### TOML Configuration

| Key | Default | Description |
|-----|---------|-------------|
| `archive_host` | *(required)* | Hostname of the backup machine |
| `archive_user` | *(required)* | SSH username for backup machine |
| `archive_dir` | *(required)* | Directory containing change files |
| `archive_port` | `22` | SSH port for backup machine |
| `pg_host` | `127.0.0.1` | Local PostgreSQL host |
| `pg_port` | `5432` | Local PostgreSQL port |
| `pg_user` | `admin` | Local PostgreSQL username |
| `pg_password` | *(required)* | Local PostgreSQL password |
| `pg_database` | *(required)* | Local PostgreSQL database |
| `data_sync_freq` | `600` | Sync frequency in seconds (min: 60) |
| `metric_freq` | `24` | Metrics aggregation frequency in hours |

### Environment Variables

Environment variables override TOML configuration:

| Variable | Overrides |
|----------|-----------|
| `DATA_SYNC_CONFIG` | Path to TOML config file (required) |
| `PG_PASSWORD` | `pg_password` |
| `PG_DB_NAME` | `pg_database` |
| `PG_USER_NAME` | `pg_user` |
| `PG_HOST` | `pg_host` |
| `PG_PORT` | `pg_port` |
| `DATA_SYNC_FREQ` | `data_sync_freq` |
| `METRIC_FREQ` | `metric_freq` |

## Database Schema

The syncdata utility creates three tables in the local database:

### data_sync_logs

Audit log of every sync event:

```sql
CREATE TABLE data_sync_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    table_name TEXT NOT NULL,
    status TEXT NOT NULL,  -- 'SUCCESS' or 'FAILED'
    rows_synced INT DEFAULT 0,
    archive_ref TEXT,      -- Filename or LSN
    error_detail TEXT,
    sync_time TIMESTAMPTZ DEFAULT now()
);
```

### data_sync_metrics

Aggregated sync metrics:

```sql
CREATE TABLE data_sync_metrics (
    id SERIAL PRIMARY KEY,
    table_name TEXT NOT NULL,
    period_start TIMESTAMPTZ NOT NULL,
    period_end TIMESTAMPTZ NOT NULL,
    period_type TEXT NOT NULL,  -- 'FREQ', 'WEEK', 'MONTH'
    records_added BIGINT DEFAULT 0,
    records_updated BIGINT DEFAULT 0,
    records_deleted BIGINT DEFAULT 0,
    UNIQUE(table_name, period_start, period_type)
);
```

### tables_to_sync

Whitelist of tables to synchronize:

```sql
CREATE TABLE tables_to_sync (
    id SERIAL PRIMARY KEY,
    table_name TEXT NOT NULL,
    creator TEXT DEFAULT NULL,
    created_at TIMESTAMPTZ DEFAULT now(),
    UNIQUE(table_name)
);
```

## Change File Format

Change files are JSON with one record per line:

```json
{"table": "users", "op": "INSERT", "data": {"id": 1, "name": "John"}, "lsn": "0/16B3D40", "ts": "2026-02-05T10:30:00Z"}
{"table": "users", "op": "UPDATE", "data": {"id": 1, "name": "Jane"}, "old_keys": {"id": 1}, "lsn": "0/16B3D50", "ts": "2026-02-05T10:31:00Z"}
{"table": "users", "op": "DELETE", "old_keys": {"id": 1}, "lsn": "0/16B3D60", "ts": "2026-02-05T10:32:00Z"}
```

| Field | Description |
|-------|-------------|
| `table` | Table name |
| `op` | Operation: `INSERT`, `UPDATE`, or `DELETE` |
| `data` | Column values (for INSERT/UPDATE) |
| `old_keys` | Primary key values (for UPDATE/DELETE) |
| `lsn` | Log Sequence Number |
| `ts` | Timestamp of change |

## Troubleshooting

### "SFTP client not connected"

Check SSH connectivity to the backup machine:

```bash
ssh -p 22 backupuser@backup.example.com ls /backups/postgresql/changes/
```

Ensure SSH key-based authentication is set up.

### "No tables in whitelist"

Add tables to sync:

```bash
syncdata add-tables users orders
```

### "Table not found locally"

Ensure the table exists in the local database with the same schema as production.

### "Failed to apply changes"

Check the error detail in `data_sync_logs`:

```sql
SELECT * FROM data_sync_logs WHERE status = 'FAILED' ORDER BY sync_time DESC LIMIT 10;
```

Common issues:
- Schema mismatch between production and local
- Missing foreign key references
- Data type incompatibility

### Resync after errors

If a table is out of sync:

```bash
syncdata resync table_name
```

## Files

| File | Description |
|------|-------------|
| `shared/go/api/table-syncher/*.go` | Core library |
| `shared/go/cmd/syncdata/main.go` | CLI entry point |
| `shared/go/scripts/setup_logical_replication.sh` | Production setup |
| `shared/go/scripts/archive_changes.sh` | Change archiver |

## Limitations

1. **Sync Latency**: Data synchronization is asynchronous; the local instance may lag behind production by at least `data_sync_freq` seconds.

2. **Read-Only**: Local tables are intended for read-only use. Manual modifications may cause sync failures.

3. **Schema Changes**: DDL changes (ALTER TABLE, etc.) are not automatically synced. You must manually apply schema changes to the local database.

4. **Large Tables**: Initial sync of large tables may take significant time. Consider using pg_dump for initial data load.

5. **Conflict Resolution**:
   - INSERT with existing PK: Treated as UPSERT
   - UPDATE/DELETE on missing row: Logged as warning, skipped
