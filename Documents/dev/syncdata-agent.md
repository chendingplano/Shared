# syncdata Agent Documentation

This document provides technical specifications for agents and automated systems interacting with the syncdata module.

## Module Overview

**Purpose**: Synchronize specific PostgreSQL tables from production to local instance via logical decoding change files.

**Location**: `shared/go/api/table-syncher/`

**CLI**: `shared/go/cmd/syncdata/`

## Quick Reference

### CLI Commands

```bash
# Daemon control
syncdata start                    # Start daemon (foreground)
syncdata stop                     # Stop running daemon
syncdata status                   # Show daemon status

# Table management
syncdata add-tables <t1> [t2...]  # Add tables to whitelist
syncdata remove-tables <t1>...    # Remove tables from whitelist
syncdata list-tables              # List whitelisted tables

# Data operations
syncdata clear                    # Truncate all synced tables (prompts)
syncdata resync <table>           # Re-sync specific table

# Global flags
syncdata -v|--verbose <cmd>       # Enable debug logging
```

### Required Environment Variables

```bash
DATA_SYNC_CONFIG=/path/to/config.toml  # Required
PG_PASSWORD=secret                      # Required if not in TOML
PG_DB_NAME=mydb                         # Required if not in TOML
```

### Configuration File (TOML)

```toml
archive_host = "backup.example.com"
archive_user = "backupuser"
archive_dir = "/backups/postgresql/changes"
archive_port = 22

pg_host = "127.0.0.1"
pg_port = 5432
pg_user = "admin"
pg_password = "secret"
pg_database = "mydb"

data_sync_freq = 600
metric_freq = 24
```

## Package Structure

```
table-syncher/
├── types.go      # Data types: ChangeRecord, SyncResult, DaemonStatus
├── config.go     # LoadConfig(), SyncConfig struct
├── state.go      # StateManager for checkpoint persistence
├── tables.go     # EnsureTables(), AddTables(), RemoveTables(), ListTables()
├── sync.go       # SFTPClient, ParseChangeFile(), ApplyChanges()
├── metrics.go    # MetricsAggregator
├── status.go     # GetDaemonStatus(), FormatStatus(), PID management
└── service.go    # SyncDataService (main orchestrator)
```

## Key Types

### SyncConfig
```go
type SyncConfig struct {
    ArchiveHost  string  // Remote backup machine
    ArchiveUser  string  // SSH user
    ArchiveDir   string  // Directory with change files
    ArchivePort  int     // SSH port (default: 22)

    PGHost       string  // Local PostgreSQL
    PGPort       int
    PGUser       string
    PGPassword   string
    PGDatabase   string

    DataSyncFreq int     // Polling interval (seconds)
    MetricFreq   int     // Metrics aggregation (hours)

    StateFilePath string // Auto-derived
    PIDFilePath   string // Auto-derived
    ConfigDir     string // Auto-derived
}
```

### ChangeRecord
```go
type ChangeRecord struct {
    Table   string         // Table name
    Op      ChangeOperation // INSERT, UPDATE, DELETE
    Data    map[string]any // New values
    OldKeys map[string]any // PK values for UPDATE/DELETE
    LSN     string         // Log Sequence Number
    TS      time.Time      // Change timestamp
}
```

### SyncResult
```go
type SyncResult struct {
    FilesProcessed  int
    RecordsAdded    int64
    RecordsUpdated  int64
    RecordsDeleted  int64
    RecordsSkipped  int64  // Not in whitelist
    RecordsFailed   int64
    Duration        time.Duration
    LastLSN         string
}
```

### DaemonStatus
```go
type DaemonStatus struct {
    Status        SyncStatus  // "active" or "not-started"
    SyncFrequency int         // Configured frequency
    StartTime     time.Time
    RecordsSynced int64
    Errors        int64
    LastSyncTime  time.Time
    Tables        []TableInfo
}
```

## API Usage

### Initialize Service

```go
import tablesyncher "github.com/chendingplano/shared/go/api/table-syncher"

config, err := tablesyncher.LoadConfig()
if err != nil {
    return err
}

logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
service := tablesyncher.NewService(config, logger)

if err := service.Initialize(ctx); err != nil {
    return err
}
defer service.Close()
```

### Run Sync Loop

```go
// Blocking - runs until context cancelled
err := service.RunLoop(ctx)
```

### Run Single Sync

```go
result, err := service.RunOnce(ctx)
if err != nil {
    return err
}
fmt.Printf("Synced %d records\n", result.RecordsAdded)
```

### Table Management

```go
// Add tables
added, err := service.AddTables(ctx, []string{"users", "orders"})

// Remove tables
removed, err := service.RemoveTables(ctx, []string{"old_table"})

// List tables
tables, err := service.ListTables(ctx)

// Resync specific table
result, err := service.Resync(ctx, "users")

// Clear all tables
err := service.Clear(ctx)
```

### Get Status

```go
status, err := service.GetStatus(ctx)
fmt.Println(tablesyncher.FormatStatus(status))
```

## Database Tables

### data_sync_logs

Audit every sync event.

```sql
SELECT * FROM data_sync_logs
WHERE status = 'FAILED'
ORDER BY sync_time DESC
LIMIT 10;
```

### data_sync_metrics

Aggregated metrics.

```sql
SELECT table_name, SUM(records_added) as total_added
FROM data_sync_metrics
WHERE period_type = 'WEEK'
GROUP BY table_name;
```

### tables_to_sync

Whitelist.

```sql
SELECT table_name FROM tables_to_sync;
```

## Change File Format

Files in archive directory: `changes_YYYYMMDD_HHMMSS.json`

One JSON record per line:

```json
{"table":"users","op":"INSERT","data":{"id":1,"name":"John"},"lsn":"0/16B3D40","ts":"2026-02-05T10:30:00Z"}
{"table":"users","op":"UPDATE","data":{"id":1,"name":"Jane"},"old_keys":{"id":1},"lsn":"0/16B3D50","ts":"2026-02-05T10:31:00Z"}
{"table":"users","op":"DELETE","old_keys":{"id":1},"lsn":"0/16B3D60","ts":"2026-02-05T10:32:00Z"}
```

## Error Codes

Location codes for error tracing:

| Code | Description |
|------|-------------|
| `SHD_SYN_001` | Config load error |
| `SHD_SYN_002` | Config validation error |
| `SHD_SYN_030` | State load error |
| `SHD_SYN_031` | State save error |
| `SHD_SYN_050` | Schema creation error |
| `SHD_SYN_060` | SFTP connection error |
| `SHD_SYN_061` | File discovery error |
| `SHD_SYN_064` | Change apply error |
| `SHD_SYN_090` | Service init error |

## State File

Location: `<config_dir>/.syncdata_state.json`

```json
{
  "version": 1,
  "last_file": "changes_20260205_103000.json",
  "last_file_time": "2026-02-05T10:30:00Z",
  "global_lsn": "0/16B3D60",
  "tables": {
    "users": {
      "last_lsn": "0/16B3D60",
      "last_synced_at": "2026-02-05T10:35:00Z",
      "record_count": 1234
    }
  },
  "total_synced": 1234
}
```

## PID File

Location: `<config_dir>/.syncdata.pid`

Contains the process ID of the running daemon.

## Dependencies

```go
// Required
"github.com/pkg/sftp"          // SFTP client
"golang.org/x/crypto/ssh"      // SSH client
"github.com/spf13/viper"       // TOML parsing
"github.com/spf13/cobra"       // CLI framework
"github.com/lib/pq"            // PostgreSQL driver
```

## Build

```bash
cd ~/Workspace/shared/go
GOWORK=off go build -o bin/syncdata ./cmd/syncdata
```

## Testing

### Unit Tests
```bash
cd ~/Workspace/shared/go/api/table-syncher
go test -v ./...
```

### Integration Test
```bash
# Set up test config
export DATA_SYNC_CONFIG=/tmp/test_syncdata.toml
export PG_PASSWORD=test
export PG_DB_NAME=test_db

# Run daemon
./bin/syncdata start &

# Check status
./bin/syncdata status

# Add tables
./bin/syncdata add-tables test_table

# Stop
./bin/syncdata stop
```

## Production Setup

1. **Production Server**: Run `setup_logical_replication.sh`
2. **Production Server**: Run `archive_changes.sh` as daemon
3. **Local Machine**: Configure TOML and environment
4. **Local Machine**: Run `syncdata start`

See `syncdata-readme.md` for detailed instructions.
