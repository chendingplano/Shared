# Data Sync Utility Specification

## 1. Purpose
The **Data Sync Utility** is a Go-based service designed to synchronize specific tables from a Production PostgreSQL system to a local instance. It operates by consuming archived data files stored on a remote backup machine, ensuring local environments stay updated for read-only analysis or reporting.

## 2. Pre-Requirements
* **Development Environment:** Refer to ~/Workspace/CLAUDE.md for the development environment, coding conventions, etc.
* **Version Parity:** Both the Production system and local machine must run the same version (currently, **PostgreSQL 18.1**).
* **Schema Consistency:** Database schemas (DDL) must be identical on both systems before synchronization begins.
* **Active Archiving:** The Production system must have WAL archiving enabled (see `pgbackup.md` [1]).
    * `wal_level = 'logical'`
    * `archive_mode = 'on'`
* **Archive Access:** The local utility requires read access to the **Backup Machine**. Archive files are generated every $T$ seconds (defined as `archive_timeout` in [1], currently 300s).
* **Sync Latency:** Data synchronization is asynchronous; the local instance may lag behind Production by at least $T$ seconds.
* **Read-Only Constraint:** Local tables are intended for read-only use. Manual modifications (INSERT, UPDATE, DELETE) to synchronized tables may cause sync failures. When that happens, you may do a full `resync`.

## 3. Functional Requirements
* **Polling Mechanism:** The service scans the archive directory at an interval defined by `DATA_SYNC_FREQ`.
* **Atomic Tracking:** The service must maintain a persistent state (checkpoints) to ensure that each archive or data change is applied **exactly once**.
* **Observability:** All activities, including row counts and transfer durations, must be logged to the `data_sync_logs` table.
* **Error Handling:** Failures must be logged with sufficient context, including the timestamp, target table, and the specific archive file causing the issue.

## 4. Environment & Configuration
The utility reads settings from a `.toml` file. The path to this file must be exported in the environment variable `DATA_SYNC_CONFIG`.

### Configuration Parameters
| Key | Default | Description |
| :--- | :--- | :--- |
| `PG_HOST` | `127.0.0.1` | Local PostgreSQL host address. |
| `PG_PORT` | `5432` | Local PostgreSQL port. |
| `PG_USER_NAME` | `admin` | Database username. |
| `PG_PASSWORD` | *(Required)* | Database password. |
| `PG_DB_NAME` | *(Required)* | Target database name. |
| `DATA_SYNC_FREQ`| `600` | Frequency (seconds) to check for new archives. |
| `METRIC_FREQ` | `24` | Frequency (hours) to aggregate and update metrics. |

## 5. Database Schema Design

### 5.1 Table: `data_sync_logs`
Used to audit every synchronization event.

```sql
CREATE TABLE data_sync_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    table_name TEXT NOT NULL,
    status TEXT NOT NULL, -- e.g., 'SUCCESS', 'FAILED'
    rows_synced INT DEFAULT 0,
    archive_ref TEXT, -- Filename or LSN of the processed archive
    error_detail TEXT,
    sync_time TIMESTAMPTZ DEFAULT now()
);
```

### 5.2 Table: `data_sync_metrics`
Aggregates synchronization performance and data throughput.

```sql
CREATE TABLE data_sync_metrics (
    id SERIAL PRIMARY KEY,
    table_name TEXT NOT NULL,
    period_start TIMESTAMPTZ NOT NULL,
    period_end TIMESTAMPTZ NOT NULL,
    period_type TEXT NOT NULL, -- 'FREQ', 'WEEK', 'MONTH'
    records_added BIGINT DEFAULT 0,
    records_updated BIGINT DEFAULT 0,
    records_deleted BIGINT DEFAULT 0,
    UNIQUE(table_name, period_start, period_type)
);
```

### 5.3 Table: `tables_to_sync`
List the table names to sync.

```sql
CREATE TABLE data_sync_metrics (
    id SERIAL PRIMARY KEY,
    table_name TEXT NOT NULL,
    creator TEXT DEFAULT NULL,
    created_at TIMESTAMPTZ DEFAULT now(),
    UNIQUE(table_name)
);
```

## 6. CLI Commands
| Command | Argument | Explanation |
|:--------|:---------|:------------|
| start | — | Initializes and runs the sync daemon. |
| stop | — | Gracefully shuts down the service. |
| status | — | Returns service uptime and last successful sync time. |
| clear | — | Wipes local data for all synced tables. |
| resync | <table_name> | Drops and recreates a specific table's data from scratch. |
| add-tables | <name1, ...> | Adds new tables to the synchronization whitelist. |
| remove-tables | <name1, ...> | Removes tables from the synchronization list.

### Status Format 
Below is the command 'status' output format:
```text
status: <'active' or 'not-started'>
sync frequency: <the value of DATA_SYNC_FREQ>
start time: <the service start time, if the service is running>
records synced: <the number of records synched since start>
errors: <the number of errors encountered since start>
```

### Files
Go files are in the directory shared/go/api/table-syncher

### Documentation
Please generate the following documents:
| Name | Location | Explanations |
|:-----|:---------|:-------------|
| syncdata-readme.md | shared/Documents/syncdata-readme.md | Should include all the information about this module, including the design, setup, configurations, operations, etc. |
| syncdata-agent.md | shared/Documents/syncdata-agent.md | Intended to be used by agents or agentic applications |

## 7. References
[1] pgbackup.md: Located at Workspace/shared/Documents/pgbackup.md. Provide details on production WAL levels, archive commands, and remote storage paths.
[2] CLAUDE.md: Located at Workspace/CLAUDE.md. Provide details about the local development environment.