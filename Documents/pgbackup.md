# PostgreSQL WAL Archiving / PITR Backup Solution

## Overview

`pgbackup` is a PostgreSQL backup management tool that provides:

- **WAL Archiving**: Continuous archiving of Write-Ahead Log files
- **Base Backups**: Full database backups using `pg_basebackup`
- **Point-in-Time Recovery (PITR)**: Restore to any moment within the recovery window
- **Retention Management**: Automatic cleanup of old backups
- **Verification**: Backup integrity checking

## Architecture

### How WAL Archiving Works

PostgreSQL uses Write-Ahead Logging (WAL) to ensure data durability. Every change to the database is first written to WAL files before being applied to data files. WAL archiving copies these files to a safe location as they're completed.

```
PostgreSQL Server
       │
       ▼
  WAL Files (pg_wal/)
       │
       │ archive_command
       ▼
  Archive Directory (wal_archive/)
       │
       ├──── (compressed .gz files) ──── Point-in-Time Recovery
       │
       │ rsync over SSH (non-blocking)
       ▼
  Remote Machine
  (base/ + wal_archive/)
```

### Components

1. **Base Backup**: A full snapshot of the database at a point in time
2. **WAL Archive**: Continuous stream of all changes since the backup
3. **Recovery**: Base backup + WAL replay = database at any point in time

### Directory Structure

```text
$PG_BACKUP_DIR/
├── base/                    # Base backups
│   ├── 20260202_020000/     # Backup from Feb 2, 2026 at 2:00 AM
│   │   ├── base.tar.gz      # Main data files
│   │   ├── pg_wal.tar.gz    # WAL files during backup
│   │   └── pgbackup_manifest.json
│   └── 20260201_020000/
├── wal_archive/             # Archived WAL files
│   ├── 000000010000000000000001.gz
│   ├── 000000010000000000000002.gz
│   └── ...
├── scripts/
│   └── archive_wal.sh       # WAL archiving script
└── logs/
    ├── wal_archive.log      # WAL archiving log
    ├── backup-stdout.log    # Scheduled backup output
    └── backup-stderr.log    # Scheduled backup errors
```

### Files Created
Core Library (shared/go/api/pgbackup/):

- config.go - Configuration loading from environment variables
- backup.go - Base backup using pg_basebackup
- restore.go - PITR restore with recovery.signal
- retention.go - Cleanup old backups and WAL files
- verify.go - Backup integrity verification
- status.go - Status reporting

### CLI Tool:

- shared/go/cmd/pgbackup/main.go - Cobra CLI with all commands

### Scripts:

- shared/go/scripts/archive_wal.sh - WAL archiving script template

### Scheduled Jobs:

- ~/Library/LaunchAgents/com.shared.pgbackup.plist - Daily backup at 2 AM
- ~/Library/LaunchAgents/com.shared.pgbackup-cleanup.plist - Weekly cleanup on Sundays

### Prerequisites

- PostgreSQL 12+ (uses `recovery.signal` method) (we are using 18.1)
- PostgreSQL user with `REPLICATION` privilege
- `pg_basebackup` available in PATH
- Sufficient disk space for backups and WAL archives
- macOS (for launchd scheduling) or Linux (for cron)

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `PG_USER_NAME` | Yes | - | PostgreSQL username |
| `PG_PASSWORD` | Yes | - | PostgreSQL password |
| `PG_DB_NAME` | Yes | - | Database name |
| `PG_HOST` | No | 127.0.0.1 | PostgreSQL host |
| `PG_PORT` | No | 5432 | PostgreSQL port |
| `PG_BACKUP_DIR` | Yes | - | Base directory for backups |
| `PGDATA` | For restore | - | PostgreSQL data directory |
| `PG_BACKUP_RETAIN_DAYS` | No | 7 | Days to keep backups |
| `PG_BACKUP_RETAIN_COUNT` | No | 3 | Minimum backups to retain |
| `PG_BACKUP_RETAIN_WAL_DAYS` | No | 14 | Days to keep WAL files |
| `PG_BACKUP_REMOTE_HOST` | No | - | Remote hostname/IP for rsync. Remote sync disabled if empty |
| `PG_BACKUP_REMOTE_USER` | No | current user | SSH username for remote host |
| `PG_BACKUP_REMOTE_DIR` | No | same as `PG_BACKUP_DIR` | Remote directory path for backups |
| `PG_BACKUP_REMOTE_PORT` | No | 22 | SSH port for remote host |

## Installation & Setup

### 1. Set up your environment variables

Refer to the above section.

### 2. Build (from shared/go directory):

- cd $HOME/Workspace/shared/go
- GOWORK=off go build -o bin/pgbackup ./cmd/pgbackup

### 3. Initialize:

- ./bin/pgbackup init (in $HOME/Workspace/shared/go)

### 4. Configure PostgreSQL (as superuser):

```sql
ALTER SYSTEM SET wal_level = 'replica';
ALTER SYSTEM SET archive_mode = 'on';
ALTER SYSTEM SET archive_command = '$PG_BACKUP_DIR/scripts/archive_wal.sh %p %f';
ALTER SYSTEM SET archive_timeout = 300;  -- Archive every 5 minutes if no activity
```

PosgreSQL normally only archives a WAL file when it's full (16 MB by default). On a low-activity database, it could take hours or days to fill a single WAL segment. 'archive_timeout' forces PostgreSQL to switch to a new WAL file after 300 seconds of the current one being open, even if it is not full yet. The partially-filled file then gets archived. If, however, there is truly zero write activity (no inserts, updates, deletes or even autovacuum writes), PostgreSQL will not force a switch. The timer only applies when there is an open WAL sgement with at least some data written to it.

If there are some write activities within the timeout and then the system crashes and data corrupted, data writes within this period of time will get lost. It is for this reason this value should not be too big. If the time is 300 seconds, we are running the risk of losing up to 5 minute data writes if:
- There are some writes during this period of time
- The system not only crashes, but the storage is damaged, too

System crashes do happen quite often, which includes software bugs (the most common ones), hardware failure (less likely) and some other unforseen events. Data corruption may happen, but very rarely, unless the storage media is too old (the older, the more chance to fail). Unless the system requires extremely high reliability, we do not want to set this value too small. Otherwise, it will generate too many small files.

**Restart PostgreSQL** to apply changes:

```bash
# macOS Homebrew
brew services restart postgresql@18

# Linux systemd
sudo systemctl restart postgresql
```

### 5. Verify configuration:

```bash
./bin/pgbackup status
```

### 6. Create first backup:

```bash
./bin/pgbackup backup
```

### 7. Enable scheduled jobs:

```bash
launchctl load ~/Library/LaunchAgents/com.shared.pgbackup.plist 
# Loads and activates the scheduled PostgreSQL backup job. Once loaded, the system will automatically run the backup task according to whatever schedule is defined in that plist file.

launchctl load ~/Library/LaunchAgents/com.shared.pgbackup-cleanup.plist
# Loads and activates a separate scheduled clearnup job, which presumably removes old backup files on a schedule.
```

Both are user-level launch Agents (stored in ~/Library/LaunchAgents/), meaning they run in the context of the current user and only while thatt user is logged in. After loading, launchd manages them - starting them on schedule, restarting if configured, etc.

**IMPORTANT**
Before running the above commands, check whether you have already run them:
```bash
launchctl list | grep pgbackup
```

If they were already started, you should see:
```text
-	0	com.shared.pgbackup-cleanup
-	1	com.shared.pgbackup
```

If you run:

```bash
launchctl load ~/Library/LaunchAgents/com.shared.pgbackup.plist
Load failed: 5: Input/output error
```

the script may have already run. Try the following:
```bash
# Unload and reload
launchctl bootout gui/$(id -u) ~/Library/LaunchAgents/com.shared.pgbackup.plist
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.shared.pgbackup.plist
```

The same is tru:
```bash
# Unload and reload
launchctl bootout gui/$(id -u) ~/Library/LaunchAgents/com.shared.pgbackup-cleanup.plist
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.shared.pgbackup-cleanup.plist
```

## CLI Commands

### `pgbackup init`

Initialize the backup environment:

```bash
pgbackup init
```

- Creates backup directories
- Installs WAL archive script
- Verifies PostgreSQL configuration

### `pgbackup backup`

Create a new base backup:

```bash
pgbackup backup
```

- Uses `pg_basebackup` for consistent snapshots
- Streams WAL during backup
- Compresses with gzip
- Creates manifest file

### `pgbackup restore`

Restore from a backup:

```bash
# Restore latest backup
pgbackup restore 20260202_020000

# Point-in-time recovery
pgbackup restore 20260202_020000 --target-time "2026-02-02 14:30:00"

# Dry run (validate without restoring)
pgbackup restore 20260202_020000 --dry-run

# Restore to different directory
pgbackup restore 20260202_020000 --target-dir /path/to/new/data
```

**Important**: PostgreSQL must be STOPPED before restore.

### `pgbackup verify`

Verify backup integrity:

```bash
# Verify latest backup
pgbackup verify

# Verify specific backup
pgbackup verify 20260202_020000

# Verify all backups
pgbackup verify --all
```

### `pgbackup cleanup`

Apply retention policy:

```bash
pgbackup cleanup
```

- Keeps minimum `PG_BACKUP_RETAIN_COUNT` backups
- Deletes backups older than `PG_BACKUP_RETAIN_DAYS`
- Cleans orphaned WAL files

### `pgbackup sync`

Sync all backups to a remote host:

```bash
pgbackup sync
```

- Requires `PG_BACKUP_REMOTE_HOST` to be set
- Uses rsync over SSH (key-based auth required)
- Syncs both base backups and WAL archive files
- Useful as a periodic catch-up for any WAL files that failed inline sync

### `pgbackup status`

Show comprehensive status:

```bash
pgbackup status
```

### `pgbackup list`

List all available backups:

```bash
pgbackup list
```

## Recovery Procedures

### Full Recovery (Latest State)

1. **Stop PostgreSQL**
   ```bash
   brew services stop postgresql@16
   # or: sudo systemctl stop postgresql
   ```

2. **Back up current data (optional but recommended)**
   ```bash
   mv /usr/local/var/postgres /usr/local/var/postgres.old
   ```

3. **Create empty target directory**
   ```bash
   mkdir /usr/local/var/postgres
   chmod 700 /usr/local/var/postgres
   ```

4. **Restore from backup**
   ```bash
   pgbackup restore 20260202_020000
   ```

5. **Start PostgreSQL**
   ```bash
   brew services start postgresql@16
   ```

6. **Monitor recovery**
   ```bash
   tail -f /usr/local/var/postgres/log/postgresql*.log
   ```

### Point-in-Time Recovery (PITR)

Recover to a specific moment (e.g., just before accidental data deletion):

```bash
# Stop PostgreSQL
brew services stop postgresql@16

# Restore to specific time
pgbackup restore 20260202_020000 --target-time "2026-02-02 14:30:00"

# Start PostgreSQL
brew services start postgresql@16
```

PostgreSQL will:
1. Extract the base backup
2. Replay WAL files up to the target time
3. Automatically promote to normal operation

### Recovery to a New Server

```bash
# On new server, set up environment
export PG_BACKUP_DIR="/path/to/backup/copy"
export PGDATA="/var/lib/postgresql/16/data"

# Restore
pgbackup restore 20260202_020000 --target-dir $PGDATA

# Start PostgreSQL
systemctl start postgresql
```

## Scheduled Jobs

### macOS (launchd)

Two plist files are installed in `~/Library/LaunchAgents/`:

| Job | Schedule | Purpose |
|-----|----------|---------|
| `com.shared.pgbackup.plist` | Daily at 2:00 AM | Create base backup |
| `com.shared.pgbackup-cleanup.plist` | Sunday at 3:00 AM | Apply retention policy |

**Commands:**

```bash
# Load jobs
launchctl load ~/Library/LaunchAgents/com.shared.pgbackup.plist
launchctl load ~/Library/LaunchAgents/com.shared.pgbackup-cleanup.plist

# Unload jobs
launchctl unload ~/Library/LaunchAgents/com.shared.pgbackup.plist
launchctl unload ~/Library/LaunchAgents/com.shared.pgbackup-cleanup.plist

# Check status
launchctl list | grep pgbackup

# Run backup manually
launchctl start com.shared.pgbackup

# View logs
tail -f ~/backups/postgresql/logs/backup-stdout.log
```

### Linux (cron)

Add to crontab (`crontab -e`):

```cron
# Daily backup at 2 AM
0 2 * * * /path/to/pgbackup backup >> /var/log/pgbackup.log 2>&1

# Weekly cleanup on Sunday at 3 AM
0 3 * * 0 /path/to/pgbackup cleanup >> /var/log/pgbackup-cleanup.log 2>&1
```

## Remote Sync

### Overview

Backups can be automatically copied to a remote machine via rsync over SSH. This provides offsite redundancy without blocking local backup operations.

### How It Works

Remote sync operates at two levels:

1. **Inline WAL sync**: Each time PostgreSQL archives a WAL file, the `archive_wal.sh` script attempts to rsync it to the remote host. If the remote is unreachable, the failure is logged but the local archive succeeds normally.

2. **Inline base backup sync**: After `pgbackup backup` creates a base backup, it rsyncs the backup directory to the remote host. Again, failure is non-blocking.

3. **Manual/scheduled catch-up**: `pgbackup sync` rsyncs the entire `base/` and `wal_archive/` directories to the remote, catching up any files that failed inline sync.

### Setup

1. **Set environment variables:**
   ```bash
   export PG_BACKUP_REMOTE_HOST="backup-server.example.com"
   export PG_BACKUP_REMOTE_USER="backupuser"        # optional, defaults to current user
   export PG_BACKUP_REMOTE_DIR="/backups/postgresql"  # optional, defaults to PG_BACKUP_DIR
   export PG_BACKUP_REMOTE_PORT=22                    # optional, defaults to 22
   ```

2. **Set up SSH key authentication** to the remote host (required — no password prompts):
   ```bash
   ssh-copy-id -p 22 backupuser@backup-server.example.com
   ```

3. **Re-run init** to regenerate the archive script with remote sync support:
   ```bash
   pgbackup init
   ```

4. **Test with a manual sync:**
   ```bash
   pgbackup sync
   ```

5. **Schedule periodic sync** (catches up any failed inline syncs):

   **macOS (launchd):** Add a plist, or **cron:**
   ```cron
   # Sync to remote every hour
   0 * * * * /path/to/pgbackup sync >> /var/log/pgbackup-sync.log 2>&1
   ```

### Failure Handling

- Inline WAL sync failure: Logged as a warning. The WAL file is safely archived locally. `pgbackup sync` will copy it later.
- Inline base backup sync failure: Logged as a warning. The backup is available locally. `pgbackup sync` will copy it later.
- `pgbackup sync` failure: Returns a non-zero exit code. Check SSH connectivity and remote disk space.

## Disk Space Requirements

### Estimating Backup Size

- **Base backup**: Approximately equal to your database size
- **WAL files**: ~16 MB per file, rate depends on write activity
- **Retention**: Plan for `(base_size × retain_count) + (daily_wal × retain_days)`

### Example Calculation

For a 10 GB database with moderate write activity:

```
Base backups: 10 GB × 3 (retain_count) = 30 GB
WAL files: ~500 MB/day × 14 days = 7 GB
Total: ~40 GB recommended
```

### Monitoring Disk Space

```bash
# Check backup sizes
du -sh ~/backups/postgresql/*

# Check disk usage
df -h ~/backups/postgresql
```

## Limitations

### Known Limitations

1. **Requires PostgreSQL Restart**
   - Enabling `archive_mode` requires a PostgreSQL restart
   - Cannot be changed with `pg_reload_conf()`

2. **Restore Requires Downtime**
   - PostgreSQL must be stopped for restore
   - For zero-downtime recovery, consider streaming replication

3. **Single Database Cluster**
   - Backs up entire PostgreSQL cluster, not individual databases
   - All databases in the cluster are backed up together

4. **Cloud Storage**
   - rsync-based remote sync covers SSH/SFTP targets
   - For cloud storage (S3, GCS), add custom sync scripts on top of the remote sync

5. **No Tablespace Support**
   - Tablespaces in non-standard locations may require additional configuration

6. **macOS Specific**
   - launchd configuration is macOS-specific
   - Use cron for Linux systems

### Security Considerations

1. **Credentials**
   - `PG_PASSWORD` is passed via environment variable
   - Never log or expose passwords
   - Consider using `.pgpass` file for automation

2. **Backup Directory Permissions**
   - Backup directories created with `0700` permissions
   - Only owner can read backups

3. **WAL Files**
   - WAL files may contain sensitive data
   - Ensure archive directory has appropriate permissions

## Reliability

### Data Integrity

- **Checksums**: PostgreSQL data checksums verify page integrity
- **WAL Verification**: Archive script verifies successful write
- **Backup Verification**: `pgbackup verify` tests tar/gzip integrity

### Failure Modes

| Scenario | Impact | Recovery |
|----------|--------|----------|
| Backup fails | No new backup | Previous backups remain valid |
| WAL archive fails | WAL accumulates locally | PostgreSQL continues operating |
| Disk full | Backup/archive stops | Free space, retry backup |
| Corrupt backup | Cannot restore from it | Use earlier backup |

### Best Practices

1. **Monitor backup jobs**
   ```bash
   # Check recent backup logs
   tail -100 ~/backups/postgresql/logs/backup-stdout.log
   ```

2. **Verify backups regularly**
   ```bash
   pgbackup verify --all
   ```

3. **Test recovery periodically**
   - Restore to a test server
   - Verify data integrity

4. **Multiple backup locations**
   - Sync backups to remote storage
   - Keep offsite copies

5. **Alert on failures**
   - Monitor launchd/cron job exit codes
   - Set up email/Slack notifications

## Troubleshooting

### Common Issues

#### "archive_command failed"

PostgreSQL logs show archive failures:

```
LOG: archive command failed with exit code 1
```

**Solutions:**
1. Check archive script permissions: `chmod +x archive_wal.sh`
2. Verify archive directory exists and is writable
3. Check disk space
4. Review `wal_archive.log` for errors

#### "pg_basebackup: could not connect"

```bash
pg_basebackup: could not connect to server
```

**Solutions:**
1. Verify PostgreSQL is running
2. Check credentials in environment
3. Ensure user has REPLICATION privilege:
   ```sql
   ALTER USER backup_user REPLICATION;
   ```

#### "recovery.signal exists"

PostgreSQL won't start after failed recovery:

```
FATAL: database system is in recovery mode
```

**Solutions:**
1. Let recovery complete, or
2. Remove `recovery.signal` if recovery is complete:
   ```bash
   rm $PGDATA/recovery.signal
   ```

#### WAL Files Accumulating

WAL files filling up `pg_wal/`:

**Solutions:**
1. Check archive_command is working
2. Verify archive directory has space
3. Manually archive and clean:
   ```sql
   SELECT pg_switch_wal();
   ```

### Getting Help

1. Check logs:
   - PostgreSQL: `$PGDATA/log/`
   - Backup: `$PG_BACKUP_DIR/logs/`

2. Verbose output:
   ```bash
   pgbackup -v backup
   ```

3. PostgreSQL documentation:
   - [Continuous Archiving](https://www.postgresql.org/docs/current/continuous-archiving.html)
   - [pg_basebackup](https://www.postgresql.org/docs/current/app-pgbasebackup.html)

## Quick Reference

### Daily Operations

```bash
# Check status
pgbackup status

# Manual backup
pgbackup backup

# List backups
pgbackup list
```

### Recovery Commands

```bash
# Full recovery
pgbackup restore <backup-id>

# Point-in-time recovery
pgbackup restore <backup-id> --target-time "YYYY-MM-DD HH:MM:SS"

# Dry run
pgbackup restore <backup-id> --dry-run
```

### Maintenance

```bash
# Verify backups
pgbackup verify --all

# Clean old backups
pgbackup cleanup
```

### Scheduled Jobs (macOS)

```bash
# Status
launchctl list | grep pgbackup

# Start manually
launchctl start com.shared.pgbackup

# View logs
tail -f ~/backups/postgresql/logs/backup-stdout.log
```

## Migrate Data (if needed):

If your PostgreSQL was not 18, you need to migrate your data. Make sure you have both 18 executable and your old version executable in PATH. 

The following assumes:
- You have installed the PostgreSQL (18)
- Your old version is 17

### 1 Install your old binary (as needed):
```bash
brew install postgresql@17 (assuming you were using PostgreSQL 17)
```

### 2 Initialize your new pgdata
Run the command:
```bash
initdb -D /path/to/your/pgdata18 --no-data-checksums
```

### 3 The installation shows the directory in which the PostgreSQL resides, normally:
```bash
/opt/homebrew/opt/postgresql@17/bin
```
Run the following command:
```bash
pg_upgrade \
  --ld-datadir /Users/cding/pgdata \
  --new-datadir /path/to/your/pgdata18 \
  --old-bindir /path/to/pg17/bin \
  --new-bindir /Users/cding/.nix-profile/bin
```

### 4 Restart your PostgreSQL
pg_ctl -D /path/to/you/pgdata18 -l logfile start
