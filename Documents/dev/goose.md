# Goose Database Migration Module

**Package path:** `github.com/chendingplano/shared/go/api/goose`
**Source:** [`shared/go/api/goose/goose.go`](../../go/api/goose/goose.go)
**Upstream:** [`github.com/pressly/goose/v3`](https://github.com/pressly/goose)

This module wraps the [pressly/goose](https://github.com/pressly/goose) migration library and integrates it with the shared library's global database connections and `JimoLogger`. Applications that use `databaseutil.InitDB` can run migrations with as little as three lines of code.

---

## Table of Contents

1. [Installation](#installation)
2. [How It Works](#how-it-works)
3. [Migration Files](#migration-files)
4. [Configuration](#configuration)
5. [Usage](#usage)
   - [Recommended: Embedded Migrations](#recommended-embedded-migrations)
   - [Directory-Based Migrations](#directory-based-migrations)
   - [Explicit Database Connection](#explicit-database-connection)
6. [Creating and Applying Migrations Programmatically](#creating-and-applying-migrations-programmatically)
7. [API Reference](#api-reference)
8. [Integration with Application Startup](#integration-with-application-startup)
9. [Common Patterns](#common-patterns)
10. [Goose CLI](#goose-cli)
11. [Troubleshooting](#troubleshooting)

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
2. **On startup**, your application creates a `Migrator` and calls `Up`. Goose checks a small tracking table in the database (`goose_db_version` by default) and applies only the migrations that have not yet been recorded there.
3. Each migration is executed inside a database transaction; if it fails, the transaction is rolled back automatically and an error is returned.
4. The tracking table is created automatically on the first run.

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

The numeric prefix is the **version number**. Goose applies migrations in ascending version order.

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

```sql
-- +goose Up
ALTER TABLE documents ADD COLUMN IF NOT EXISTS tags TEXT[] DEFAULT '{}';

-- +goose Down
ALTER TABLE documents DROP COLUMN IF EXISTS tags;
```

### No-Down Migration

If rolling back a migration is not meaningful (e.g. dropping a column that no longer exists), omit the `-- +goose Down` section entirely. Goose will return an error if `Down()` is called on such a migration.

---

## Configuration

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

> **Why two separate fields?** `MigrationsFS` is a read-only `fs.FS` interface (which may be an embedded binary FS with no path on disk). `MigrationsDir` is the real filesystem path needed to *write* new files. When using `os.DirFS`, set both to the same directory path.

---

## Usage

### Recommended: Embedded Migrations

Embed your migration files into the binary so you never have to worry about deploying them separately.

**Directory layout:**

```
myapp/
├── main.go
└── migrations/
    ├── 20240101000001_create_users.sql
    └── 20240215143000_add_email_index.sql
```

**Code:**

```go
package main

import (
    "context"
    "io/fs"
    "log"

    // embed is imported for its side-effect (populating the embed.FS)
    _ "embed"

    sharedgoose "github.com/chendingplano/shared/go/api/goose"
)

//go:embed migrations
var embedMigrations embed.FS

func runMigrations(ctx context.Context) error {
    // Strip the top-level "migrations" directory so goose sees the
    // .sql files at the root of the filesystem it receives.
    migrationsFS, err := fs.Sub(embedMigrations, "migrations")
    if err != nil {
        return err
    }

    migrator, err := sharedgoose.New(sharedgoose.Config{
        MigrationsFS:  migrationsFS,
        MigrationsDir: "migrations", // required if you use CreateMigration / CreateAndApply
    })
    if err != nil {
        return err
    }

    return migrator.Up(ctx)
}
```

### Directory-Based Migrations

For development or tooling scripts where embedding is not required:

```go
import (
    "os"
    sharedgoose "github.com/chendingplano/shared/go/api/goose"
)

migrator, err := sharedgoose.New(sharedgoose.Config{
    MigrationsFS:  os.DirFS("migrations"),
    MigrationsDir: "migrations",
})
```

### Explicit Database Connection

Use `NewWithDB` when you need to run migrations against a database that is **not** the shared library's global pool (e.g. during tests, or a separate tenant database):

```go
import (
    "database/sql"
    "github.com/chendingplano/shared/go/api/ApiTypes"
    sharedgoose "github.com/chendingplano/shared/go/api/goose"
)

migrator, err := sharedgoose.NewWithDB(db, ApiTypes.PgName, sharedgoose.Config{
    MigrationsFS:  migrationsFS,
    MigrationsDir: "migrations",
})
```

---

## Creating and Applying Migrations Programmatically

Use this workflow whenever you need to alter a table schema at runtime or through a management command — for example, adding a column when a new feature is deployed, without requiring a manual `goose` CLI run.

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

`downSQL` is optional — pass an empty string if rolling back is not needed.

### Prerequisites

Set `MigrationsDir` in `Config` and use `os.DirFS` (not `embed.FS`) for `MigrationsFS`:

```go
migrator, err := sharedgoose.New(sharedgoose.Config{
    MigrationsFS:  os.DirFS("migrations"),
    MigrationsDir: "migrations",
})
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

### How the internal rebuild works

The goose `Provider` scans the migration directory at construction time. When `CreateAndApply` writes a new file it automatically calls `rebuildProvider()` to recreate the Provider so the new file is discovered before `UpByOne` is called. The version-tracking table (`goose_db_version`) ensures the new migration is applied exactly once.

> **Concurrent use:** `CreateAndApply` is not safe for concurrent calls. If multiple goroutines may create migrations simultaneously, serialise them with a mutex or use a job queue.

---

## API Reference

### Constructor Functions

| Function | Description |
|---|---|
| `New(cfg Config) (*Migrator, error)` | Create a Migrator using the shared library's global DB connection (`ApiTypes.PG_DB_Shared` or `MySql_DB_Migrate`). Call after `databaseutil.InitDB`. |
| `NewWithDB(db *sql.DB, dbType string, cfg Config) (*Migrator, error)` | Create a Migrator with an explicit connection. `dbType` must be `ApiTypes.PgName` or `ApiTypes.MysqlName`. |

### Migrator Methods

**Schema creation**

| Method | Description |
|---|---|
| `CreateMigration(description, upSQL, downSQL string) (string, error)` | Write a new timestamped `.sql` file to `MigrationsDir`. Returns the filename. Does not apply. |
| `CreateAndApply(ctx, description, upSQL, downSQL string) (string, error)` | Write a new migration file and immediately apply it. Returns the filename. |

**Applying migrations**

| Method | Description |
|---|---|
| `Up(ctx) error` | Apply all pending migrations. No-op when nothing is pending. |
| `UpByOne(ctx) error` | Apply the single next pending migration. Returns `gooselib.ErrNoNextVersion` if none. |
| `UpTo(ctx, version) error` | Apply migrations up to and including `version`. |

**Rolling back**

| Method | Description |
|---|---|
| `Down(ctx) error` | Roll back the most recently applied migration. |
| `DownTo(ctx, version) error` | Roll back all applied migrations newer than `version`. Pass `0` to roll back everything. |

**Inspection**

| Method | Description |
|---|---|
| `Status(ctx) ([]*gooselib.MigrationStatus, error)` | Return the applied/pending state of every migration. |
| `GetVersion(ctx) (int64, error)` | Return the highest applied version number. Returns `0` if nothing is applied. |
| `HasPending(ctx) (bool, error)` | Return `true` when at least one migration is pending. |
| `ListSources() []*gooselib.Source` | Return all migration sources (version, type, path), ordered ascending. |

### Error Types (from goose/v3)

```go
import gooselib "github.com/pressly/goose/v3"

// Check for specific conditions:
errors.Is(err, gooselib.ErrNoNextVersion)    // No pending migration for UpByOne
errors.Is(err, gooselib.ErrVersionNotFound)  // Version does not exist
errors.Is(err, gooselib.ErrAlreadyApplied)   // Migration already in database
errors.Is(err, gooselib.ErrNotApplied)       // Migration not yet applied

// Partial failures carry the list of what succeeded before the failure:
var partial *gooselib.PartialError
if errors.As(err, &partial) {
    fmt.Println("applied before failure:", len(partial.Applied))
    fmt.Println("failed migration:", partial.Failed.Source.Version)
}
```

---

## Integration with Application Startup

The recommended place to run migrations is immediately after the database connection is established and before the application begins serving traffic.

```go
func main() {
    ctx := context.Background()

    // 1. Load config
    cfg := loadConfig()

    // 2. Initialise DB connections (shared library)
    if err := databaseutil.InitDB(ctx, mysqlCfg, pgCfg); err != nil {
        log.Fatal("database init failed:", err)
    }
    defer databaseutil.CloseDatabase()

    // 3. Create system tables (shared library)
    logger := loggerutil.CreateDefaultLogger("APP_STR_030")
    if err := sysdatastores.CreateSysTables(logger); err != nil {
        log.Fatal("system tables failed:", err)
    }

    // 4. Run application migrations
    migrationsFS, _ := fs.Sub(embedMigrations, "migrations")
    migrator, err := sharedgoose.New(sharedgoose.Config{
        MigrationsFS: migrationsFS,
    })
    if err != nil {
        log.Fatal("migrator init failed:", err)
    }
    if err := migrator.Up(ctx); err != nil {
        log.Fatal("migrations failed:", err)
    }

    // 5. Start server
    startServer()
}
```

---

## Common Patterns

### Check Before Migrating

```go
pending, err := migrator.HasPending(ctx)
if err != nil {
    return err
}
if pending {
    log.Println("Applying pending database migrations...")
    if err := migrator.Up(ctx); err != nil {
        return err
    }
}
```

### Print Migration Status

```go
statuses, err := migrator.Status(ctx)
if err != nil {
    return err
}
for _, s := range statuses {
    state := "pending"
    if s.State == gooselib.StateApplied {
        state = fmt.Sprintf("applied at %s", s.AppliedAt.Format(time.RFC3339))
    }
    fmt.Printf("v%d  %s  [%s]\n", s.Source.Version, s.Source.Path, state)
}
```

### Roll Back in Tests

```go
func TestMyFeature(t *testing.T) {
    ctx := context.Background()
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
migrator, err := sharedgoose.New(sharedgoose.Config{
    MigrationsFS: migrationsFS,
    TableName:    "tenant_42_schema_version",
})
```

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

At least one migration in the batch failed. The error wraps a `*gooselib.PartialError`:

```go
var partial *gooselib.PartialError
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
