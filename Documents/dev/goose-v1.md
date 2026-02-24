# Goose Database Migration Module

**Package path:** `github.com/chendingplano/shared/go/api/goose`

**Source:** [`shared/go/api/goose/goose.go`](../../go/api/goose/goose.go)

**Upstream:** [`github.com/pressly/goose/v3`](https://github.com/pressly/goose)

This module wraps the [pressly/goose](https://github.com/pressly/goose) migration library and integrates it with the shared library's global database connections and `JimoLogger`. Applications that use `databaseutil.InitDB` can run migrations with as little as three lines of code.

---

## Table of Contents

1. [Architecture: Project and Shared Migrations](#architecture-project-and-shared-migrations)
2. [Installation](#installation)
3. [How It Works](#how-it-works)
4. [Migration Files](#migration-files)
5. [Configuration](#configuration)
6. [Usage](#usage)
   - [For Application Developers](#for-application-developers)
   - [For Shared Library Developers](#for-shared-library-developers)
   - [Embedded Migrations](#embedded-migrations)
   - [Directory-Based Migrations](#directory-based-migrations)
   - [Explicit Database Connection](#explicit-database-connection)
7. [Creating and Applying Migrations Programmatically](#creating-and-applying-migrations-programmatically)
8. [API Reference](#api-reference)
9. [Integration with Application Startup](#integration-with-application-startup)
10. [Common Patterns](#common-patterns)
11. [Best Practices](#best-practices)
12. [Goose CLI](#goose-cli)
13. [Troubleshooting](#troubleshooting)

---

## Architecture: Project, Shared, and AutoTester Migrations

The system maintains **three separate migration tracks**, each targeting a different database:

1. **Project migrations** - Tables specific to an application (e.g., `tax/`, `ChenWeb/`)
2. **Shared migrations** - Tables in the `shared/` library reused across all applications
3. **AutoTester migrations** - Test result tables for each project's dedicated autotester DB

Accordingly, the package exposes three global migrators:
- `goose.go::ProjectMigrator` - for application-specific migrations (`PG_DB_Project`)
- `goose.go::SharedMigrator` - for shared library migrations (`PG_DB_Shared`)
- `goose.go::AutoTesterMigrator` - for autotester tables (`PG_DB_AutoTester`)

**Application code** should use `goose.ProjectMigrator` for its own database migrations.

**Code in `shared/`** should use `goose.SharedMigrator` for shared table changes.

**Autotester commands** should use `goose.AutoTesterMigrator` for auto-test result tables. Each project that uses AutoTester has its own dedicated autotester DB so test data is isolated from production data and from other projects.

The autotester entry point (`server/cmd/autotester/main.go`) calls all three:
- `goose.go::RunProjectMigrations(...)` - applies project-specific migrations
- `goose.go::RunSharedMigrations(...)` - applies shared library migrations
- `goose.go::RunAutoTesterMigrations(...)` - applies autotester table migrations

See [`ChenWeb/server/cmd/deepdoc/main.go`](../../ChenWeb/server/cmd/deepdoc/main.go) for a project/shared example.

---

## Quick Start

### Create the DBs
Goose uses database tables to keep track of migration status. There SHALL be three DBs:
- **Project DB** — application data (e.g., `mirai`, `chenweb`)
- **Shared DB** — shared library tables; all projects share this one DB (default name: `shared`)
- **AutoTester DB** — per-project test result storage (e.g., `mirai_autotester`); each project has its own so test data is completely isolated from production and from other projects' test runs

Configure all three in your `mise.local.toml` (see Environment Variables below).

### For Application Developers

```go
package main

import (
    "context"
    "database/sql"
    "embed"
    "io/fs"

    sharedgoose "github.com/chendingplano/shared/go/api/goose"
    "github.com/chendingplano/shared/go/api/ApiTypes"
    "github.com/chendingplano/shared/go/api/databaseutil"
    "github.com/chendingplano/shared/go/api/loggerutil"
)

//go:embed migrations
var embedMigrations embed.FS

func main() {
    ctx := context.Background()
    logger := loggerutil.CreateDefaultLogger("APP_001")

    // 1. Initialize database connections
    databaseutil.InitDB(ctx, mysqlCfg, pgCfg)
    defer databaseutil.CloseDatabase()

    // 2. Get the appropriate database connection for your project
    var projectDB *sql.DB
    if ApiTypes.DatabaseInfo.DBType == ApiTypes.PgName {
        projectDB = ApiTypes.PG_DB_Project
    } else {
        projectDB = ApiTypes.MySql_DB_Project
    }

    // 3. Configure migration settings
    migrateCfg := ApiTypes.MigrationConfig{
        MigrationsFS: "migrations",
        MigrationsDir: "migrations",
        DBName: "your_app_db",
        TableName: "db_migrations",  // See libconfig.toml configuration
    }

    // 4. Run project migrations (uses ProjectMigrator internally)
    if err := sharedgoose.RunProjectMigrations(ctx, logger, migrateCfg, projectDB); err != nil {
        logger.Error("project migrations failed", "error", err)
        return
    }

    // 5. Run shared migrations if your app uses shared tables
    var sharedDB *sql.DB
    if ApiTypes.DatabaseInfo.DBType == ApiTypes.PgName {
        sharedDB = ApiTypes.PG_DB_Shared
    } else {
        sharedDB = ApiTypes.MySql_DB_Shared
    }

    if err := sharedgoose.RunSharedMigrations(ctx, logger, migrateCfg, sharedDB); err != nil {
        logger.Error("shared migrations failed", "error", err)
        return
    }

    // Continue with application startup...
}
```

### For Shared Library Developers

When writing code in `shared/` that needs to create or modify shared tables:

```go
package shared

import (
    "context"
    sharedgoose "github.com/chendingplano/shared/go/api/goose"
)

func AddFeature(ctx context.Context) error {
    // Use SharedMigrator for shared table changes
    _, err := sharedgoose.SharedMigrator.CreateAndApply(ctx,
        "add_new_feature_table",
        `CREATE TABLE IF NOT EXISTS feature_data (...);`,
        `DROP TABLE IF EXISTS feature_data;`,
    )
    return err
}
```

---

## Installation

The dependency is already declared in `shared/go/go.mod`. No extra steps are needed when using the shared library via the Go workspace.

If you are adding the dependency to a standalone module:

```bash
go get github.com/pressly/goose/v3@latest
```

---

## How It Works

1. **Migration files** are plain `.sql` files named with a version prefix and annotated with `-- +goose Up` / `-- +goose Down` markers.
2. **On startup**, your application creates a `Migrator` and calls `Up`. Goose checks a small tracking table in the database (configured via `TableName` in `Config`, typically `db_migrations`) and applies only the migrations that have not yet been recorded there.
3. Each migration is executed inside a database transaction by default; if it fails, the transaction is rolled back automatically and an error is returned.
4. **The tracking table is created automatically** on the first run when you call `RunProjectMigrations` or `RunSharedMigrations`. The table creation is handled by the underlying `goose.Provider` when it initializes.

### How the Goose Tracking Table is Created

When `RunProjectMigrations` or `RunSharedMigrations` is called:

1. A `Migrator` is created via `NewWithDB`, which builds a `goose.Provider` with the configured options (including `TableName`)
2. When `migrator.Up(ctx)` is called, the `goose.Provider` automatically:
   - Checks if the version-tracking table exists
   - If not, creates the table with the schema appropriate for your database (PostgreSQL or MySQL)
   - The table stores: version number, migration type, timestamp applied, and other metadata

**Table schema (PostgreSQL example):**
```sql
CREATE TABLE db_migrations (
    id            BIGSERIAL PRIMARY KEY,
    version_id    BIGINT NOT NULL,
    migration_type VARCHAR(20) NOT NULL,
    applied_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    is_applied    BOOLEAN NOT NULL DEFAULT FALSE
);
```

**MySQL equivalent:**
```sql
CREATE TABLE db_migrations (
    id            BIGINT AUTO_INCREMENT PRIMARY KEY,
    version_id    BIGINT NOT NULL,
    migration_type VARCHAR(20) NOT NULL,
    applied_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    is_applied    BOOLEAN NOT NULL DEFAULT FALSE
);
```

> **Note:** The exact schema may vary slightly depending on the goose version. The table is created with appropriate indexes for efficient version lookups.

### Transaction Behavior

By default, each migration runs inside a transaction. This ensures atomicity: either all changes in the migration succeed, or none are applied.

**Important limitations:**
- Some DDL statements **cannot** run inside transactions (e.g., `CREATE INDEX CONCURRENTLY` in PostgreSQL, `CREATE DATABASE` in most databases).
- For such cases, add `-- +goose NO TRANSACTION` at the top of the migration file:

```sql
-- +goose NO TRANSACTION
-- +goose Up
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_email ON users(email);

-- +goose Down
DROP INDEX IF EXISTS idx_users_email;
```

---

## Migration Files

### Naming Convention

```
YYYYMMDDHHMMSS_short_description.sql
```

Examples:

```
20240101000001_create_users_table.sql
20240215143000_add_email_verified_column.sql
20241130090000_create_audit_log_index.sql
```

The numeric prefix is the **version number** (parsed as `int64`). Goose applies migrations in ascending version order.

### SQL File Format

Every SQL migration file must contain at minimum an `-- +goose Up` section.

```sql
-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS documents (
    id          BIGSERIAL PRIMARY KEY,
    title       VARCHAR(255) NOT NULL,
    body        TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS documents;
-- +goose StatementEnd
```

> **`StatementBegin` / `StatementEnd` blocks** are required whenever a statement contains semicolons internally (e.g. `DO $$ ... $$` blocks, procedure definitions, or `CREATE TYPE`). For simple single-statement DDL they are optional but recommended for clarity.

### Adding a Column Example

**PostgreSQL:**

```sql
-- +goose Up
ALTER TABLE documents ADD COLUMN IF NOT EXISTS tags TEXT[] DEFAULT '{}';

-- +goose Down
ALTER TABLE documents DROP COLUMN IF EXISTS tags;
```

**MySQL equivalent:**

```sql
-- +goose Up
ALTER TABLE documents ADD COLUMN IF NOT EXISTS tags JSON;

-- +goose Down
ALTER TABLE documents DROP COLUMN IF EXISTS tags;
```

### No-Down Migration

If rolling back a migration is not meaningful (e.g. dropping a column that no longer exists), omit the `-- +goose Down` section entirely. Goose will return an error if `Down()` is called on such a migration.

---

## Configuration

### Config Structure

```go
type Config struct {
    // MigrationsFS is the filesystem that contains the .sql migration files.
    // Defaults to os.DirFS("migrations") when nil.
    // Can also be fs.Sub(embedFS, "migrations") for an embedded filesystem.
    MigrationsFS fs.FS

    // MigrationsDir is the actual path to the migrations directory on disk.
    // Defaults to the MIGRATION_DIR environment variable, or "migrations" if unset.
    // Required when using CreateMigration or CreateAndApply; ignored otherwise.
    // Must point to the same directory that MigrationsFS reads from.
    // Example: "migrations" or "/app/migrations"
    MigrationsDir string

    // TableName overrides the version-tracking table name.
    // Default: "goose_db_version"
    // Typically configured via libconfig.toml as "db_migrations"
    TableName string

    // Verbose enables verbose output from the goose library.
    // Default: true
    Verbose bool

    // AllowOutOfOrder permits applying migrations whose version is lower
    // than the current database version. Useful with feature branches.
    // Default: true
    AllowOutOfOrder bool
}
```

### Three Migration Tracks

Each track runs against a different database so project data, shared library data, and test data never mix:

| Migrator | Scope | Database Connection | TableName |
|----------|-------|---------------------|-----------|
| `ProjectMigrator` | App-specific tables (e.g., `tax/`, `ChenWeb/`) | `ApiTypes.PG_DB_Project` / `MySql_DB_Project` | `db_migrations` |
| `SharedMigrator` | Shared library tables (all projects share this DB) | `ApiTypes.PG_DB_Shared` / `MySql_DB_Shared` | `db_migrations` |
| `AutoTesterMigrator` | Auto-test result tables — **per-project, isolated** | `ApiTypes.PG_DB_AutoTester` / `MySql_DB_AutoTester` | `db_migrations` |

Each track maintains its **own version-tracking table in its own database**, allowing fully independent migration schedules.

**AutoTester DB isolation** — `auto_test_runs`, `auto_test_results`, and `auto_test_logs` live exclusively in `PG_DB_AutoTester`. This means:
- Test history never touches production data
- `tax/` and `ChenWeb/` each own a separate autotester DB (e.g., `mirai_autotester`, `chenweb_autotester`)
- `autotesters.CreateAutoTestTables(logger, PG_DB_AutoTester, dbType)` creates the tables at startup
- `DBPersistence` in `server/cmd/autotester/main.go` connects to `PG_DB_AutoTester`, not `PG_DB_Project`

### Configuring TableName in libconfig.toml

Add the following entry to your project `config.toml`:

```toml
[migration]
tablename = "db_migrations"
```

This ensures all three migration tracks use a consistent version-tracking table name.

> **Why three separate migrators?** Each database has an independent lifecycle: the project DB, the shared DB, and the autotester DB can each be migrated, rolled back, or wiped without affecting the others. Separate tracking tables prevent version conflicts.

### Environment Variables

Add the following environment variables in your project ```text mise.local.toml```:

```text
PG_USER_NAME = "admin"
PG_PASSWORD = "<password"
PG_DB_NAME = "<project_db_name>"
PG_DB_NAME_SHARED = "<shared_db_name>"
PG_DB_NAME_AUTOTESTER = "<autotester_db_name>"
PG_HOST = "127.0.0.1"
PG_PORT = "5432"
```

---

## Usage

### For Application Developers

Application developers should use `RunProjectMigrations` in their `main.go` to apply application-specific migrations. This function initializes the global `ProjectMigrator`.

**Typical application startup flow:**

```go
func main() {
    ctx := context.Background()
    logger := loggerutil.CreateDefaultLogger("APP_001")

    // 1. Initialize database
    databaseutil.InitDB(ctx, mysqlCfg, pgCfg)
    defer databaseutil.CloseDatabase()

    // 2. Get project database connection
    var projectDB *sql.DB
    if ApiTypes.DatabaseInfo.DBType == ApiTypes.PgName {
        projectDB = ApiTypes.PG_DB_Project
    } else {
        projectDB = ApiTypes.MySql_DB_Project
    }

    // 3. Configure migrations
    migrateCfg := ApiTypes.MigrationConfig{
        MigrationsFS:  "migrations",
        MigrationsDir: "migrations",
        DBName:        "myapp",
        TableName:     "db_migrations",
    }

    // 4. Run project migrations
    if err := sharedgoose.RunProjectMigrations(ctx, logger, migrateCfg, projectDB); err != nil {
        logger.Error("project migrations failed", "error", err)
        os.Exit(1)
    }

    // 5. Run shared migrations (if your app uses shared tables)
    var sharedDB *sql.DB
    if ApiTypes.DatabaseInfo.DBType == ApiTypes.PgName {
        sharedDB = ApiTypes.PG_DB_Shared
    } else {
        sharedDB = ApiTypes.MySql_DB_Shared
    }

    if err := sharedgoose.RunSharedMigrations(ctx, logger, migrateCfg, sharedDB); err != nil {
        logger.Error("shared migrations failed", "error", err)
        os.Exit(1)
    }

    // Continue with application startup...
}
```

### For Shared Library Developers

When writing code in `shared/` that needs to modify shared library tables, use the global `SharedMigrator`:

```go
package myshared

import (
    "context"
    sharedgoose "github.com/chendingplano/shared/go/api/goose"
)

func InitializeFeature(ctx context.Context) error {
    // Use SharedMigrator for shared table schema changes
    _, err := sharedgoose.SharedMigrator.CreateAndApply(ctx,
        "add_feature_flags_table",
        `CREATE TABLE IF NOT EXISTS feature_flags (
            id          BIGSERIAL PRIMARY KEY,
            name        VARCHAR(255) NOT NULL,
            enabled     BOOLEAN NOT NULL DEFAULT false,
            created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
        );`,
        `DROP TABLE IF EXISTS feature_flags;`,
    )
    return err
}
```

> **Important:** The `SharedMigrator` is initialized by the application's `main.go` via `RunSharedMigrations`. Shared library code should assume it's available when needed.

### Directory-Based Migrations

For development or tooling scripts where embedding is not required:

**For project migrations:**
```go
import (
    "os"
    sharedgoose "github.com/chendingplano/shared/go/api/goose"
)

migrator := sharedgoose.ProjectMigrator
```

**For shared library migrations:**
```go
// Use the global SharedMigrator after RunSharedMigrations has been called
migrator := sharedgoose.SharedMigrator
```

---

## Creating and Applying Migrations Programmatically

Use this workflow whenever you need to alter a table schema at runtime or through a management command — for example, adding a column when a new feature is deployed, without requiring a manual `goose` CLI run.

### Which Migrator to Use

| Context | Use |
|---------|-----|
| Application code modifying app tables | `ProjectMigrator` |
| Shared library code modifying shared tables | `SharedMigrator` |
| Test code with isolated database | `NewWithDB(testDB, ...)` |

### The two functions

| Function | What it does |
|---|---|
| `CreateMigration(description, upSQL, downSQL string) (filename string, err error)` | Writes a new `.sql` file to `MigrationsDir`. Does **not** apply it. |
| `CreateAndApply(ctx, description, upSQL, downSQL string) (filename string, err error)` | Writes the file **and** immediately applies it via `UpByOne`. |

Both functions:
- Generate a timestamped filename (`YYYYMMDDHHMMSS_<slug>.sql`)
- Wrap the SQL in goose `-- +goose StatementBegin / StatementEnd` markers
- Log the filename and application result through `JimoLogger`
- Return the filename so you can commit it to version control

`downSQL` is optional — pass an empty string if rolling back is not needed. If `Down()` is called on a migration with no down SQL, it will return an error.

### Prerequisites

Make sure call goose.go::RunProjectMigrations(...) and goose.go::RunSharedMigrations(...) in your main.go (refer to the example in the Quick Start section)

```go
// In application code
_, err := sharedgoose.ProjectMigrator.CreateAndApply(ctx, ...)

// In shared library code
_, err := sharedgoose.SharedMigrator.CreateAndApply(ctx, ...)
```

### Example: add a column to an existing table

```go
filename, err := migrator.CreateAndApply(ctx,
    "add_verified_at_to_users",
    `ALTER TABLE users ADD COLUMN IF NOT EXISTS verified_at TIMESTAMPTZ;`,
    `ALTER TABLE users DROP COLUMN IF EXISTS verified_at;`,
)
if err != nil {
    return fmt.Errorf("schema change failed: %w", err)
}
log.Printf("Applied and saved migration: %s", filename)
// Commit the generated file so it becomes part of the repo history.
```

The generated file will look like:

```sql
-- +goose Up
-- +goose StatementBegin
ALTER TABLE users ADD COLUMN IF NOT EXISTS verified_at TIMESTAMPTZ;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE users DROP COLUMN IF EXISTS verified_at;
-- +goose StatementEnd
```

### Example: create a new index (no rollback needed)

```go
filename, err := migrator.CreateAndApply(ctx,
    "idx_documents_owner",
    `CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_documents_owner ON documents(owner_id);`,
    "", // no down migration
)
```

> **Concurrent use:** `CreateAndApply` is not safe for concurrent calls. If multiple goroutines may create migrations simultaneously, serialize them with a mutex or use a job queue.

### How the internal rebuild works

The goose `Provider` scans the migration directory at construction time. When `CreateAndApply` writes a new file it automatically calls `rebuildProvider()` to recreate the Provider so the new file is discovered before `UpByOne` is called. The version-tracking table (`goose_db_version`) ensures the new migration is applied exactly once.

---

## API Reference

### Global Variables

| Variable | Type | Description |
|----------|------|-------------|
| `ProjectMigrator` | `*Migrator` | Global migrator for application-specific migrations. Initialized by `RunProjectMigrations`. |
| `SharedMigrator` | `*Migrator` | Global migrator for shared library migrations. Initialized by `RunSharedMigrations`. |

**Usage:**
```go
// In application code (after RunProjectMigrations has been called)
_, err := sharedgoose.ProjectMigrator.CreateAndApply(ctx, "add_column", ...)

// In shared library code (after RunSharedMigrations has been called)
_, err := sharedgoose.SharedMigrator.CreateAndApply(ctx, "add_index", ...)
```

### Migration Initialization Functions

These functions are typically called from `main.go` during application startup.

| Function | Description | Returns |
|----------|-------------|---------|
| `RunProjectMigrations(ctx, logger, cfg, db) error` | Initialize and run migrations for application-specific tables. Sets `ProjectMigrator` global. | `error` |
| `RunSharedMigrations(ctx, logger, cfg, db) error` | Initialize and run migrations for shared library tables. Sets `SharedMigrator` global. | `error` |
| `RunMigrations(ctx, logger, name, cfg, db) (*Migrator, error)` | Low-level function to create and run a migrator. Returns the Migrator instance. | `*Migrator`, `error` |

**Typical usage in `main.go`:**
```go
// Run project migrations
if err := sharedgoose.RunProjectMigrations(ctx, logger, migrateCfg, projectDB); err != nil {
    log.Fatal("project migrations failed:", err)
}

// Run shared migrations
if err := sharedgoose.RunSharedMigrations(ctx, logger, migrateCfg, sharedDB); err != nil {
    log.Fatal("shared migrations failed:", err)
}
```

### Constructor Functions

| Function | Description |
|---|---|
| `goose.go::RunProjectMigrations(...)` | Create a Migrator for the project and runs the migration. |
| `goose.go::RunSharedMigrations` | Create a Migrator for the shared/ library and runs the migration. |

### Migrator Methods

**Schema creation**

| Method | Description | Returns |
|---|---|---|
| `CreateMigration(description, upSQL, downSQL string) (string, error)` | Write a new timestamped `.sql` file to `MigrationsDir`. Returns the filename. Does not apply. | `filename string`, `error` |
| `CreateAndApply(ctx, description, upSQL, downSQL string) (string, error)` | Write a new migration file and immediately apply it. Returns the filename. | `filename string`, `error` |

**Applying migrations**

| Method | Description | Returns | Errors |
|---|---|---|---|
| `Up(ctx) error` | Apply all pending migrations. No-op when nothing is pending. | `error` | `PartialError` if some fail |
| `UpByOne(ctx) error` | Apply the single next pending migration. | `error` | `ErrNoNextVersion` if none pending |
| `UpTo(ctx, version) error` | Apply migrations up to and including `version`. | `error` | `ErrVersionNotFound` if version doesn't exist |

**Rolling back**

| Method | Description | Returns | Errors |
|---|---|---|---|
| `Down(ctx) error` | Roll back the most recently applied migration. | `error` | Error if migration has no down SQL |
| `DownTo(ctx, version) error` | Roll back all applied migrations newer than `version`. Pass `0` to roll back everything. | `error` | `ErrNotApplied` if version not applied |

**Inspection**

| Method | Description | Returns | Concurrent-safe |
|---|---|---|---|
| `Status(ctx) ([]*goose.MigrationStatus, error)` | Return the applied/pending state of every migration. | `[]*MigrationStatus`, `error` | Yes |
| `GetVersion(ctx) (int64, error)` | Return the highest applied version number. Returns `0` if nothing is applied. | `int64`, `error` | Yes |
| `HasPending(ctx) (bool, error)` | Return `true` when at least one migration is pending. | `bool`, `error` | Yes |
| `ListSources() []*goose.Source` | Return all migration sources (version, type, path), ordered ascending. | `[]*Source` | Yes |

> **Note on version type:** The version number is stored as `int64`. The timestamp-based naming convention (e.g., `20240215143000`) fits comfortably within `int64` range for any practical date.

### Error Types (from goose/v3)

```go
import (
    "errors"
    "github.com/pressly/goose/v3"
)

// Check for specific conditions:
errors.Is(err, goose.ErrNoNextVersion)    // No pending migration for UpByOne
errors.Is(err, goose.ErrVersionNotFound)  // Version does not exist
errors.Is(err, goose.ErrAlreadyApplied)   // Migration already in database
errors.Is(err, goose.ErrNotApplied)       // Migration not yet applied

// Partial failures carry the list of what succeeded before the failure:
var partial *goose.PartialError
if errors.As(err, &partial) {
    fmt.Println("applied before failure:", len(partial.Applied))
    fmt.Println("failed migration:", partial.Failed.Source.Version)
}
```

---

## Integration with Application Startup

The recommended place to run migrations is immediately after the database connection is established and before the application begins serving traffic.

**Complete example following the pattern in `ChenWeb/server/cmd/deepdoc/main.go`:**

```go
func main() {
    ctx := context.Background()
    logger := loggerutil.CreateDefaultLogger("APP_001")

    // 1. Load config
    cfg := loadConfig()

    // 2. Initialise DB connections (shared library)
    if err := databaseutil.InitDB(ctx, mysqlCfg, pgCfg); err != nil {
        log.Fatal("database init failed:", err)
    }
    defer databaseutil.CloseDatabase()

    // 3. Create system tables (shared library)
    if err := sysdatastores.CreateSysTables(logger); err != nil {
        log.Fatal("system tables failed:", err)
    }

    // 4. Get database connections for project and shared migrations
    var projectDB, sharedDB *sql.DB
    if ApiTypes.DatabaseInfo.DBType == ApiTypes.PgName {
        projectDB = ApiTypes.PG_DB_Project
        sharedDB = ApiTypes.PG_DB_Shared
    } else {
        projectDB = ApiTypes.MySql_DB_Project
        sharedDB = ApiTypes.MySql_DB_Shared
    }

    // 5. Configure migrations
    migrateCfg := ApiTypes.MigrationConfig{
        MigrationsFS:  "migrations",
        MigrationsDir: "migrations",
        DBName:        "myapp",
        TableName:     "db_migrations",  // From libconfig.toml
    }

    // 6. Run project migrations (initializes ProjectMigrator)
    if err := sharedgoose.RunProjectMigrations(ctx, logger, migrateCfg, projectDB); err != nil {
        log.Fatal("project migrations failed:", err)
    }

    // 7. Run shared migrations (initializes SharedMigrator)
    if err := sharedgoose.RunSharedMigrations(ctx, logger, migrateCfg, sharedDB); err != nil {
        log.Fatal("shared migrations failed:", err)
    }

    // 8. Start server
    startServer()
}
```

> **Note:** After `RunProjectMigrations` and `RunSharedMigrations` complete, the global `ProjectMigrator` and `SharedMigrator` variables are available for use throughout your application.

---

## Common Patterns

### Check Before Migrating

```go
// For project migrations
pending, err := sharedgoose.ProjectMigrator.HasPending(ctx)
if err != nil {
    return err
}
if pending {
    log.Println("Applying pending database migrations...")
    if err := sharedgoose.ProjectMigrator.Up(ctx); err != nil {
        return err
    }
}
```

### Print Migration Status

```go
// For shared migrations
statuses, err := sharedgoose.SharedMigrator.Status(ctx)
if err != nil {
    return err
}
for _, s := range statuses {
    state := "pending"
    if s.State == goose.StateApplied {
        state = fmt.Sprintf("applied at %s", s.AppliedAt.Format(time.RFC3339))
    }
    fmt.Printf("v%d  %s  [%s]\n", s.Source.Version, s.Source.Path, state)
}
```

### Roll Back in Tests

```go
func TestMyFeature(t *testing.T) {
    ctx := context.Background()
    
    // Create a test-specific migrator
    migrator, _ := sharedgoose.NewWithDB(testDB, ApiTypes.PgName, cfg)

    // Apply migrations for this test
    require.NoError(t, migrator.Up(ctx))

    t.Cleanup(func() {
        // Roll back everything so the next test starts clean
        _ = migrator.DownTo(ctx, 0)
    })

    // ... run test ...
}
```

### Custom Tracking Table Per Schema / Tenant

```go
// For multi-tenant applications, each tenant can have its own version table
migrator, err := sharedgoose.NewWithDB(tenantDB, ApiTypes.PgName, sharedgoose.Config{
    MigrationsFS: migrationsFS,
    MigrationsDir: "migrations",
    DBName:        "tenant_42",
    TableName:     "tenant_42_schema_version",
})
```
**Note**: this feature is not implemented yet!

### Using ProjectMigrator in Application Code

```go
// After RunProjectMigrations has been called in main.go
func AddFeature(ctx context.Context) error {
    _, err := sharedgoose.ProjectMigrator.CreateAndApply(ctx,
        "add_user_preferences",
        `CREATE TABLE user_preferences (...);`,
        `DROP TABLE IF EXISTS user_preferences;`,
    )
    return err
}
```

### Using SharedMigrator in Shared Library Code

```go
// In shared/ package code
func InitializeSharedFeature(ctx context.Context) error {
    _, err := sharedgoose.SharedMigrator.CreateAndApply(ctx,
        "add_audit_log_index",
        `CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_log(created_at);`,
        "",  // No down migration needed
    )
    return err
}
```

---

## Best Practices

### Testing Migrations

1. **Test on staging first:** Always run migrations against a staging environment that mirrors production before deploying.
2. **Test the Down migration:** Before deploying, verify that rolling back works:
   ```bash
   goose -dir migrations postgres "$DATABASE_URL" down
   ```
3. **Test with realistic data volume:** Index creation and column additions may behave differently on large tables.

### Destructive Operations

Avoid destructive changes in a single migration:

| Instead of | Do this |
|---|---|
| `DROP COLUMN` | 1. Stop writing to the column<br>2. Deploy<br>3. Migrate existing data if needed<br>4. Drop column in a later migration |
| `DROP TABLE` | 1. Rename table first<br>2. Deploy and verify<br>3. Drop in a later migration |
| Changing column type | 1. Add new column<br>2. Backfill data<br>3. Switch application to use new column<br>4. Drop old column later |

### Transaction-Aware Migrations

- Use `-- +goose NO TRANSACTION` for operations that don't support transactions (e.g., `CREATE INDEX CONCURRENTLY`).
- Be aware that long-running migrations inside transactions may hold locks and block other operations.

### Version Control

- Always commit generated migration files to version control.
- Never edit an already-applied migration file. Create a new migration to fix issues.
- Use meaningful, descriptive filenames.

---

## Goose CLI

The `goose` command-line tool is useful during development for creating migration files, inspecting status, and manually applying or rolling back migrations without restarting your application.

### Install

```bash
go install github.com/pressly/goose/v3/cmd/goose@latest
```

### Common Commands

```bash
# Create a new timestamped SQL migration file
goose -dir migrations create add_tags_column sql

# Show current migration status
goose -dir migrations postgres "$DATABASE_URL" status

# Apply all pending migrations
goose -dir migrations postgres "$DATABASE_URL" up

# Roll back one migration
goose -dir migrations postgres "$DATABASE_URL" down

# Roll back all migrations
goose -dir migrations postgres "$DATABASE_URL" reset

# Apply exactly one migration
goose -dir migrations postgres "$DATABASE_URL" up-by-one

# Apply up to a specific version
goose -dir migrations postgres "$DATABASE_URL" up-to 20240215143000
```

The `postgres` dialect string is replaced with `mysql` for MySQL databases.

### DSN Format

```bash
# PostgreSQL
export DATABASE_URL="host=localhost port=5432 user=myuser password=secret dbname=mydb sslmode=disable"

# MySQL
export DATABASE_URL="myuser:secret@(localhost:3306)/mydb"
```

---

## Troubleshooting

### `failed to create goose provider`

This usually means:

- `cfg.MigrationsFS` is `nil`.
- The filesystem path is incorrect (no `.sql` files found at the root).
- The database is not reachable.

Verify the FS contains the expected files:

```go
entries, _ := fs.ReadDir(cfg.MigrationsFS, ".")
for _, e := range entries {
    fmt.Println(e.Name())
}
```

### `migration up failed: partial error`

At least one migration in the batch failed. The error wraps a `*goose.PartialError`:

```go
import (
    "errors"
    "github.com/pressly/goose/v3"
)

var partial *goose.PartialError
if errors.As(err, &partial) {
    fmt.Println("failed version:", partial.Failed.Source.Version)
    fmt.Println("cause:", partial.Failed.Error)
}
```

Fix the failing SQL and re-run `Up`. Successfully applied migrations are tracked and will not be re-applied.

### Out-of-Order Migrations

When two feature branches each add a migration and one is merged first, the other branch's migration will have a version lower than the current database version. By default goose rejects this. Enable out-of-order support:

```go
sharedgoose.New(sharedgoose.Config{
    MigrationsFS:    migrationsFS,
    AllowOutOfOrder: true,
})
```

### `ErrNoNextVersion` on `UpByOne`

There are no pending migrations. This is not an error condition in normal operation; check with `HasPending` first if you want to gate on it.

### Version Tracking Table Already Exists with Different Name

If you previously used a different table name (e.g. through the raw goose CLI), set `TableName` in `Config` to match the existing table name so the Migrator can find the recorded state.

### What if I accidentally applied a bad migration?

1. **Don't panic.** The migration is recorded in the version table, but you can fix it.
2. **Option A - Roll back:** If the migration has a `Down` section and it's safe to roll back:
   ```bash
   goose -dir migrations postgres "$DATABASE_URL" down
   ```
   Then fix the migration file and re-apply.
3. **Option B - Create a fix migration:** If rolling back is not safe or the migration has no `Down`:
   - Create a new migration that fixes the issue (e.g., adds a missing column, corrects a type).
   - Apply it normally.
4. **Manual intervention:** If the version table is out of sync:
   ```sql
   -- Check current version
   SELECT * FROM goose_db_version ORDER BY version_id DESC LIMIT 1;
   
   -- Remove a bad version entry (use with caution!)
   DELETE FROM goose_db_version WHERE version_id = <bad_version>;
   ```

### How do I edit an already-applied migration?

**Don't.** Once a migration is applied, treat it as immutable. If there's an error:
- Create a new migration that fixes the issue.
- This ensures all environments (dev, staging, production) stay in sync.

### The version table is corrupted

If the `goose_db_version` table is corrupted or has incorrect entries:

1. **Backup first:**
   ```sql
   CREATE TABLE goose_db_version_backup AS SELECT * FROM goose_db_version;
   ```

2. **Inspect the table:**
   ```sql
   SELECT * FROM goose_db_version ORDER BY version_id;
   ```

3. **Fix manually:** Remove or correct bad entries:
   ```sql
   DELETE FROM goose_db_version WHERE version_id = <problematic_version>;
   ```

4. **Re-apply migrations:**
   ```bash
   goose -dir migrations postgres "$DATABASE_URL" up
   ```
