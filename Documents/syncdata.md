## Data Sync 
### Purpose
This is a Go utility that syncs the selected tables in the Production system to the local machine.

### Pre-Requirements
- Both Product system and local machine use PostgrelSQL, with the same version (18.1)
- Database schemas are identical on both systems
- A backup service is running on the Production system (refer to [1])
- Backup files are archived on a remote machine, called Backup Machine. Refer to [1] for the information about how data in the Production system are backed up/archived, the access information to Backup Machine and archive file locations.
- The backup utility will generate an archive file every T seconds to the Backup Machine. We thus assume that the sync may delay up to T seconds. T is defined in [1].
- It tracks all the tables in the specified DB. But only the tables in 'tables_to_sync' are synchronized to the local database.
- The synched tables in the local database are normally read-only. Users should not normally modify them (inserts, updates, or deletes), otherwise, it may cause the data sync fail. When that happens, users can always re-sync the table.

### Requirements
- The service, once started, reads the archive files every DATA_SYNC_FREQ (refer to Configuration section) seconds. If there are new changes, sync the changes to the local database.
- The service should keep track of the synchronization so that changes are synced to local databases exactly once.
- Log the sync activities in the log table (refer to Database Schema section).
- If any errors occur, make sure they are logged with sufficient information

### Environment Variables 
Environment variables should be defined in a .toml file. The location of this .toml file is specified by environment variable DATA_SYNC_CONFIG (should be available before running any commands in this document).

### Configuration
```text
PG_HOST = <host>                # Default to '127.0.0.1'
PG_PORT = <port>                # Default to '5432' 
PG_USER_NAME = <username>       # Default to 'admin'
PG_PASSWORD = <password>        # No default (required)
PG_DB_NAME = "<dbname>"         # No default (required)
DATA_SYNC_FREQ = "<seconds>"    # Default to 600, the frequency in seconds to read the archieve files
METRIC_FREQ = "<hours>"         # Default to 24, the frequency in hours to generate/update metrics
```

### Database Tables
| Table Name | Explanations |
|:-----------|:-------------|
| tables_to_sync | list all the tables to track|
| data_sync_logs | Log data sync activities |
| data_sync_metrics | data sync metrics |

#### data_sync_logs Schema 
Please design the table schema.

#### data_sync_metrics
Please design the metrics and the table schema. It should have at least the following metrics:
- The number of records added/updated/deleted per table per METRIC_FREQ hours
- The number of records added/updated/deleted per table per week, per month

### CLI Commands
|Command | Explanation |
|:-------|:------------|
| start | Start the sync service |
| stop | Stop the sync service |
| status | Get the service status information |
| clear | Clear the local tables and re-sync |
| resync | It re-syncs a specified table, which means to clear the table and sync the table from the start |
| add-tables | add one or more tables to sync |
| remove-tables | remove one or more tables to sync |

### References
[1] pgbackup.md: shared/Documents/pgbackup.md