# Database Migration Tester - V4

**Version:** 0.6
**Date:** 2026-02-24
**Status:** Draft
**Author:** Combined from tester-migration-v2.md, tester-migration-v3.md, tester-migration-qwen-v2.md, and tester-migration-qwen-v3.md

**References:**
- Testbot framework: [`Testbot/testbot.md`](../../../Testbot/testbot.md)
- AutoTester framework: [`Testbot/auto-tester-v2.md`](../../../Testbot/auto-tester-v2.md)
- Database migration: [`shared/Documents/dev/goose-v1.md`](../../dev/goose-v1.md)

---

## Table of Contents

1. [Overview](#1-overview)
2. [Goals](#2-goals)
3. [Test Model](#3-test-model)
4. [SUT Operations](#4-sut-operations)
5. [Integration with AutoTester Framework](#5-integration-with-autotester-framework)
6. [Tester Identity](#6-tester-identity)
7. [Tester Design](#7-tester-design)
8. [Test Cases](#8-test-cases)
9. [Tester Lifecycle](#9-tester-lifecycle)
10. [Internal State Tracking](#10-internal-state-tracking)
11. [Configuration](#11-configuration)
12. [Architecture](#12-architecture)
13. [Implementation Plan](#13-implementation-plan)
14. [File Structure](#14-file-structure)
15. [Open Items](#15-open-items)
16. [Change Log](#16-change-log)

---

## 1. Overview

This document describes the plan for building a **Migration Tester** — a **Testbot** (see [`Testbot/testbot.md`](../../../Testbot/testbot.md)) implementation within the **AutoTester framework** (`shared/go/api/autotesters`, see `auto-tester-v2.md`) whose System Under Test (SUT) is the **goose-based database migration system** (`shared/go/api/goose`).

The `MigrationTester` implements the `autotesters.Tester` interface (which extends the Testbot concept) and is registered with the `TesterRegistry`. The AutoTester `TestRunner` drives its full lifecycle: `Prepare` → case supply → `RunTestCase` per case → `Cleanup`. Results are persisted to `PG_DB_AutoTester` by the framework's built-in database persistence layer — the tester does not implement its own logging or reporting.

### 1.1 Background

The **production system** uses **Goose** for database migrations with **four separate databases**:

| Database | Purpose |
|---|---|
| **Project DB** | Application-specific tables and data |
| **Shared DB** | Shared library tables (common across all projects) |
| **Migration DB (Project)** | Goose version tracking for Project DB migrations |
| **Migration DB (Shared)** | Goose version tracking for Shared DB migrations |

Each migration track maintains independent version tracking in its respective Migration DB, requiring comprehensive testing to ensure:
- Migrations apply correctly in order
- Rollbacks (Down migrations) work as expected
- Version tracking is accurate
- Edge cases are handled properly

The **MigrationTester** does **not** interact with the production Migration DBs. It uses a **separate, isolated test database (DUT)** for all migration testing operations.

### 1.2 Goals

- Automatically and randomly test the migration system's correctness
- Verify that applying and rolling back migrations leaves the database in the expected state
- Detect regressions in version tracking, ordering, partial-failure handling, and rollback correctness
- Exercise the programmatic Go API: `Up`, `Down`, `UpByOne`, `UpTo`, `DownTo`, `Status`, `GetVersion`, `HasPending`, `CreateAndApply`

### 1.3 Non-Goals

- This tester does **not** test application business logic — only the migration infrastructure
- This tester does **not** test the upstream `pressly/goose` library internals
- This tester does **not** validate production migration files — it generates synthetic ones
- This tester does **not** access or modify the production Migration DBs (Project or Shared)
- Performance benchmarking of migrations
- Testing application-specific migration SQL logic (that's the developer's responsibility)

### 1.4 Why This Matters

Migration bugs are high-impact: a failed `Up` or incorrect `Down` can corrupt a production database. Automated testing with random migration sequences catches ordering bugs, partial-failure handling, and rollback correctness that are hard to exercise manually.

**Critical isolation requirement:** The MigrationTester must never touch the production Migration DBs. All testing occurs in an isolated DUT to prevent any risk to production migration state.

---

## 2. Goals

### 2.1 Primary Goals

1. **Automated Migration Testing** — Automatically test migration apply/rollback cycles
2. **Version Tracking Verification** — Ensure the `db_migrations` table accurately reflects applied migrations
3. **Edge Case Coverage** — Test boundary conditions (empty migrations, NO TRANSACTION, out-of-order, etc.)
4. **Error Handling** — Verify proper error responses for invalid operations
5. **Regression Prevention** — Catch breaking changes in the migration system
6. **AutoTester Integration** — Seamlessly integrate with the AutoTester framework and runner
7. **Production Isolation** — Never access production Migration DBs; use isolated DUT only

---

## 3. Test Model

This section defines the **Test Model** for the Migration Tester, following the Testbot framework definition: `TM = {SUT, C, T, P}`.

### 3.1 Test Model Definition

```
TM_migration = {SUT_migration, C_migration, T_migration, P_migration}
```

| Symbol | Name | Description | Section |
|---|---|---|---|
| `SUT_migration` | System Under Test | The goose database migration system | [Section 3.2](#32-sut-definition) |
| `C_migration` | Configuration | Database connections, migrations directory, table names | [Section 11](#11-configuration) |
| `T_migration` | Tools | Go API methods to interact with the migration system | [Section 4](#4-sut-operations) |
| `P_migration` | Parameters | Inputs that control migration behavior | [Section 3.5](#35-sut-parameters) |

### 3.2 SUT Definition

**SUT:** The goose migration wrapper in `shared/go/api/goose/goose.go`, accessed via `sharedgoose.NewWithDB(dutDB, ...)`.

The SUT is a **black box** that:
- Accepts migration operations (Up, Down, Status, etc.)
- Maintains internal state (applied migrations, current version)
- Produces observable outcomes (schema changes, version tracking updates, errors)

**SUT Boundary:** The SUT includes the goose migration wrapper logic but **excludes**:
- The upstream `pressly/goose` library internals (not tested here)
- Application business logic
- Production database infrastructure

### 3.3 Configuration (C_migration)

| Config Item | Default | Description |
|---|---|---|
| `DUTDB` | (required) | Database connection for DB-Under-Test; **MUST** have name starting with `testonly_` |
| `DUTDBType` | `"postgres"` | Database type: `"postgres"` or `"mysql"` |
| `MigrationsDir` | `"testonly_migrations"` | Directory for migration files; **MUST** start with `testonly_` |
| `TableName` | `"db_migrations"` | Goose version-tracking table name in DUT |
| `NumDynamicCases` | `80` | Number of dynamic test cases to generate per run |
| `MaxMigrationsInPool` | `20` | Size of pre-generated migrations pool in `Prepare` |
| `AllowOutOfOrder` | `true` | Whether out-of-order migrations are allowed |

### 3.4 Tools (T_migration)

The Tools are the Go API methods available to interact with the SUT. Each tool is a method on the `Migrator` struct:

| Tool | Method | Description |
|---|---|---|
| `T_up` | `Up(ctx) error` | Apply all pending migrations |
| `T_upByOne` | `UpByOne(ctx) error` | Apply exactly one pending migration |
| `T_upTo` | `UpTo(ctx, version int64) error` | Apply up to and including a specific version |
| `T_down` | `Down(ctx) error` | Rollback one migration |
| `T_downTo` | `DownTo(ctx, version int64) error` | Rollback to a specific version |
| `T_status` | `Status(ctx) ([]MigrationStatus, error)` | Get applied/pending status of all migrations |
| `T_getVersion` | `GetVersion(ctx) (int64, error)` | Get current (highest applied) version |
| `T_hasPending` | `HasPending(ctx) (bool, error)` | Check if any migrations are pending |
| `T_createAndApply` | `CreateAndApply(ctx, desc, upSQL, downSQL string) error` | Create migration file and apply |
| `T_listSources` | `ListSources() []string` | List all available migration sources |

### 3.5 SUT Parameters (P_migration)

SUT parameters are the inputs that control the behavior of the migration system. These are used to generate test cases.

| Parameter | Type | Valid Range | Invalid Range | Description |
|---|---|---|---|---|
| `Operation` | enum | `{Up, UpByOne, UpTo, Down, DownTo, Status, GetVersion, HasPending, CreateAndApply}` | — | Migration operation to invoke |
| `NumMigrationsInDir` | integer | `[0, 20]` | `< 0` | How many `.sql` files exist in the migrations directory |
| `NumApplied` | integer | `[0, NumMigrationsInDir]` | `> NumMigrationsInDir` | How many migrations are already applied |
| `TargetVersion` | integer | A valid version in the dir, or `0` | Version not in dir | Used by `UpTo`, `DownTo` |
| `UpSQL` | string | Valid DDL (`CREATE TABLE`, `ALTER TABLE`, etc.) | Syntactically invalid SQL | Used by `CreateAndApply` |
| `DownSQL` | string | Valid DDL inverse, or `""` (no down) | Syntactically invalid SQL | Used by `CreateAndApply`; empty is valid |
| `AllowOutOfOrder` | bool | `{true, false}` | — | Whether out-of-order migrations are allowed |
| `HasNoTransaction` | bool | `{true, false}` | — | Whether migration uses `-- +goose NO TRANSACTION` |
| `CurrentVersion` | integer | `[0, max_version]` | `< 0` | Current highest applied version (state-dependent) |

### 3.6 SUT Parameter Value Generation

1. Parameter values MUST be generated randomly using seeded RNG
2. Generation MUST produce both valid and invalid values
3. The distribution of generated values MUST follow the **Closeness principle** (weighted probability distributions)
4. Generated values must cover edge cases: boundary values, empty values, maximum length values, special characters, etc.

**Weighted Distributions (Closeness Principle):**

| Parameter | Distribution |
|---|---|
| `Operation` | `Up`: 30%, `Down`: 20%, `UpByOne`: 15%, `UpTo`: 10%, `DownTo`: 10%, `Status`/`GetVersion`/`HasPending`: 10%, `CreateAndApply`: 5% |
| `NumMigrationsInDir` | `[1,5]`: 60%, `[6,10]`: 25%, `[11,20]`: 10%, `0`: 5% |
| `NumApplied` (relative to dir size) | `0%` applied: 20%, `50%` applied: 40%, `100%` applied: 30%, random partial: 10% |
| `TargetVersion` — valid vs. invalid | Valid: 85%, Invalid/nonexistent: 15% |
| `UpSQL` — valid vs. invalid DDL | Valid DDL: 90%, invalid SQL: 10% |
| `DownSQL` — present vs. empty | Present: 70%, empty: 30% |
| `AllowOutOfOrder` | `true`: 70%, `false`: 30% |

### 3.7 State Space

The SUT state space is defined by:

| State Variable | Type | Description |
|---|---|---|
| `AppliedMigrations` | `[]MigrationRecord` | List of applied migrations (version, filename, upSQL, downSQL) |
| `FilesInDir` | `[]MigrationFile` | All `.sql` files in the migrations directory |
| `CurrentVersion` | `int64` | Highest applied version; 0 if nothing applied |
| `TablesInDUT` | `map[string]bool` | Tables that exist in DUT (testonly_ tables) |
| `TrackingTableExists` | `bool` | Whether `db_migrations` table exists |

**State Transitions:** Each operation causes a state transition:
- `Up` → Adds pending migrations to `AppliedMigrations`, updates `CurrentVersion`, creates tables
- `Down` → Removes migrations from `AppliedMigrations`, updates `CurrentVersion`, drops tables
- `CreateAndApply` → Adds file to `FilesInDir`, then applies it

---

## 4. SUT Operations

This section provides a **complete and exhaustive** list of all SUT operations. Each operation is analyzed for:
- **Purpose**: What the operation does
- **Preconditions**: What must be true before the operation
- **Effects**: What changes after the operation
- **Error Conditions**: When and why the operation fails
- **Edge Cases**: Special scenarios to test

### 4.1 Operation: Up

**Purpose:** Apply all pending migrations in ascending version order.

**Preconditions:**
- DUT is accessible
- Migrations directory exists and contains `.sql` files
- Migration files are valid SQL

**Effects:**
- All pending migrations are applied
- `db_migrations` table is created (if not exists) and updated
- Schema changes from Up SQL are applied to DUT
- `CurrentVersion` is updated to highest version

**Error Conditions:**
- `InfrastructureError`: DUT unavailable
- `InvalidSQL`: Syntax error in migration SQL
- `PartialError`: Some migrations succeeded, then one failed
- `DuplicateTable`: Migration creates a table that already exists

**Edge Cases:**
- Empty migrations directory → No-op, no error
- All migrations already applied → No-op, no error
- Migration with `NO TRANSACTION` → Applied outside transaction
- Out-of-order migration with `AllowOutOfOrder=true` → Applied
- Out-of-order migration with `AllowOutOfOrder=false` → Error

### 4.2 Operation: UpByOne

**Purpose:** Apply exactly one pending migration (the next in ascending version order).

**Preconditions:**
- DUT is accessible
- At least one pending migration exists

**Effects:**
- Exactly one migration is applied (lowest pending version)
- `db_migrations` table is updated
- `CurrentVersion` is incremented by one (or to the applied version)

**Error Conditions:**
- `InfrastructureError`: DUT unavailable
- `InvalidSQL`: Syntax error in migration SQL
- `ErrNoNextVersion`: No pending migrations exist
- `DuplicateTable`: Migration creates a table that already exists

**Edge Cases:**
- No pending migrations → Returns `ErrNoNextVersion`
- Only one migration pending → Applied, then no more pending
- Migration with no Down SQL → Applied successfully (but Down will fail later)

### 4.3 Operation: UpTo

**Purpose:** Apply all pending migrations up to and including a target version.

**Preconditions:**
- DUT is accessible
- Target version exists in migrations directory

**Effects:**
- All migrations with version <= target are applied (if not already)
- `db_migrations` table is updated
- `CurrentVersion` is updated to target (or highest applied if some failed)

**Error Conditions:**
- `InfrastructureError`: DUT unavailable
- `InvalidSQL`: Syntax error in migration SQL
- `ErrVersionNotFound`: Target version does not exist
- `PartialError`: Some migrations succeeded, then one failed

**Edge Cases:**
- Target version already applied → No-op for that version
- Target version < CurrentVersion → Error (already past target)
- Target version = 0 → No-op (no migrations to apply)

### 4.4 Operation: Down

**Purpose:** Rollback exactly one migration (the most recently applied).

**Preconditions:**
- DUT is accessible
- At least one migration is applied

**Effects:**
- Most recently applied migration is rolled back
- Down SQL is executed
- `db_migrations` table is updated
- `CurrentVersion` is decremented

**Error Conditions:**
- `InfrastructureError`: DUT unavailable
- `InvalidSQL`: Syntax error in Down SQL
- `ErrNoNextVersion`: No migrations are applied
- `NoDownSQL`: Migration has no Down SQL (empty or missing)
- `TableNotFound`: Down SQL drops a table that doesn't exist

**Edge Cases:**
- Migration with no Down SQL → Returns error
- Last remaining migration → Rolled back, `CurrentVersion` = 0
- Down SQL is idempotent → Can be run multiple times safely

### 4.5 Operation: DownTo

**Purpose:** Rollback all migrations newer than a target version.

**Preconditions:**
- DUT is accessible
- Target version is less than CurrentVersion

**Effects:**
- All migrations with version > target are rolled back
- `db_migrations` table is updated
- `CurrentVersion` is updated to target

**Error Conditions:**
- `InfrastructureError`: DUT unavailable
- `InvalidSQL`: Syntax error in Down SQL
- `ErrVersionNotFound`: Target version does not exist
- `PartialError`: Some rollbacks succeeded, then one failed

**Edge Cases:**
- Target version = 0 → Rollback all migrations
- Target version = CurrentVersion → No-op
- Target version > CurrentVersion → Error (nothing to rollback)

### 4.6 Operation: Status

**Purpose:** Return the applied/pending state of all migrations.

**Preconditions:**
- DUT is accessible

**Effects:**
- Returns list of all migrations with their status (Applied/Pending)
- No state changes

**Error Conditions:**
- `InfrastructureError`: DUT unavailable
- `TrackingTableMissing`: `db_migrations` table does not exist

**Edge Cases:**
- Empty migrations directory → Returns empty list
- No migrations applied → All shown as Pending
- All migrations applied → All shown as Applied

### 4.7 Operation: GetVersion

**Purpose:** Return the highest applied version number.

**Preconditions:**
- DUT is accessible

**Effects:**
- Returns `CurrentVersion` (int64)
- No state changes

**Error Conditions:**
- `InfrastructureError`: DUT unavailable
- `TrackingTableMissing`: `db_migrations` table does not exist

**Edge Cases:**
- No migrations applied → Returns 0
- After full rollback → Returns 0

### 4.8 Operation: HasPending

**Purpose:** Return true if any migrations are pending.

**Preconditions:**
- DUT is accessible
- Migrations directory exists

**Effects:**
- Returns bool indicating if pending migrations exist
- No state changes

**Error Conditions:**
- `InfrastructureError`: DUT unavailable

**Edge Cases:**
- Empty migrations directory → Returns false
- All migrations applied → Returns false

### 4.9 Operation: CreateAndApply

**Purpose:** Create a new migration file and immediately apply it.

**Preconditions:**
- DUT is accessible
- Migrations directory is writable

**Effects:**
- New `.sql` file is created with timestamp-based version
- Migration is applied immediately
- `db_migrations` table is updated
- Schema changes are applied

**Error Conditions:**
- `InfrastructureError`: DUT unavailable
- `InvalidSQL`: Syntax error in Up SQL
- `FileWriteError`: Cannot write to migrations directory
- `DuplicateTable`: Migration creates a table that already exists

**Edge Cases:**
- Empty Down SQL → Succeeds, but Down will fail later
- Invalid description → Sanitized for filename
- Concurrent CreateAndApply → Unique timestamps prevent collision

### 4.10 Operation: ListSources

**Purpose:** Return all known migration sources (file paths).

**Preconditions:**
- Migrations directory exists

**Effects:**
- Returns list of migration file paths
- No state changes

**Error Conditions:**
- `DirectoryNotFound`: Migrations directory does not exist

**Edge Cases:**
- Empty directory → Returns empty list
- Non-.sql files present → Filtered out

---

## 5. Integration with AutoTester Framework

### 5.1 Architecture Position

```
┌────────────────────────────────────────────────────────────────┐
│                      TestRunner                                │
│  (shared/go/api/autotesters/runner.go)                         │
└────────────────────────────┬───────────────────────────────────┘
                             │
          ┌──────────────────┼──────────────────┐
          ▼                  ▼                  ▼
 ┌─────────────────┐ ┌──────────────┐ ┌─────────────────┐
 │  DatabaseTester │ │ Migration    │ │  AuthTester     │
 │  (shared)       │ │ Tester       │ │  (shared)       │
 │                 │ │ (this plan)  │ │                 │
 └─────────────────┘ └──────┬───────┘ └─────────────────┘
                            │
                            ▼
              ┌─────────────────────────────┐
              │  PostgreSQL                 │
              │  (PG_DB_AutoTester)         │
              │  auto_test_runs             │
              │  auto_test_results          │
              │  auto_test_logs             │
              └─────────────────────────────┘

Production Databases (NEVER accessed by MigrationTester):
┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐
│   Project DB    │ │   Shared DB     │ │ Migration DB    │ │ Migration DB    │
│  (app tables)   │ │ (shared tables) │ │   (Project)     │ │   (Shared)      │
│                 │ │                 │ │  (version track)│ │  (version track)│
└─────────────────┘ └─────────────────┘ └─────────────────┘ └─────────────────┘

Test Database (exclusively used by MigrationTester):
┌─────────────────┐
│      DUT        │
│ Name: testonly_*│
│ (test migrations│
│  + db_migrations│
└─────────────────┘
```

### 5.2 Tester Interface Implementation

The Migration Tester implements the `autotesters.Tester` interface:

```go
type Tester interface {
    // Identity / metadata
    Name()        string
    Description() string
    Purpose()     string
    Type()        string
    Tags()        []string

    // Lifecycle
    Prepare(ctx context.Context) error
    Cleanup(ctx context.Context) error

    // Test case supply
    GenerateTestCases(ctx context.Context) ([]TestCase, error)
    GetTestCases() []TestCase

    // Execution
    RunTestCase(ctx context.Context, tc TestCase) TestResult
}
```

### 5.3 Directory Structure

```
shared/
└── go/
    └── api/
        └── autotesters/
            ├── tester_migration.go         # MigrationTester implementation
            └── ...

myapp/  (e.g., tax/ or ChenWeb/)
└── server/
    └── cmd/
        └── autotester/
            ├── main.go                     # Entry point
            ├── config.go                   # CLI flags
            └── registry.go                 # Register MigrationTester
```

### 5.4 Database Logging

The Migration Tester logs results to the AutoTester database tables:

| Table | Purpose |
|---|---|
| `auto_test_runs` | One row per test run (created by TestRunner) |
| `auto_test_results` | One row per TestCase execution |
| `auto_test_logs` | Structured log entries from test execution |

Tables are created at startup via:
```go
autotesters.CreateAutoTestTables(logger, ApiTypes.PG_DB_AutoTester, dbType)
```

### 5.5 Registration

The Migration Tester is registered in `server/cmd/autotester/registry.go`:

```go
autotesters.GlobalRegistry.Register("tester_migration", func() autotesters.Tester {
    return autotesters.NewMigrationTester(&MigrationTesterConfig{...})
})
```

### 5.6 CLI Usage

```bash
# Run all testers including MigrationTester
go run ./server/cmd/autotester/

# Run only MigrationTester
go run ./server/cmd/autotester/ --tester=tester_migration

# Run with specific seed for reproducibility
go run ./server/cmd/autotester/ --tester=tester_migration --seed=12345

# Run with parallel execution
go run ./server/cmd/autotester/ --tester=tester_migration --parallel

# Filter by tags
go run ./server/cmd/autotester/ --tags=migration,critical
```

---

## 6. Tester Identity

These values are returned by the `Tester` interface metadata methods, used by the AutoTester runner for filtering, logging, and registry lookup.

| Method | Value |
|---|---|
| `Name()` | `"tester_migration"` |
| `Description()` | `"Tests the goose database migration system (Up, Down, CreateAndApply, version tracking)"` |
| `Purpose()` | `"regression"` |
| `Type()` | `"integration"` |
| `Tags()` | `["database", "migration", "goose", "shared"]` |

**Struct:** `MigrationTester`
**Constructor:** `NewMigrationTester(cfg *MigrationTesterConfig) *MigrationTester`
**File:** `shared/go/api/autotesters/tester_migration.go`

### 6.1 Constructor Configuration

```go
type MigrationTesterConfig struct {
    // DUT: Isolated test database for migration testing; NEVER production.
    // Database name MUST start with "testonly_" for safety.
    DUTDB     *sql.DB
    DUTDBType string // "postgres" or "mysql"; default: "postgres"
    DUTDBName string // for logging only; MUST start with "testonly_"

    // Directory for synthetic migration files; MUST start with "testonly_".
    MigrationsDir string // default: "testonly_migrations"

    // Goose version-tracking table name.
    TableName string // default: "db_migrations"

    // Number of dynamic test cases to generate per run.
    NumDynamicCases int // default: 80

    // Size of the pre-generated migrations pool created during Prepare.
    MaxMigrationsInPool int // default: 20
}
```

`MigrationTester` embeds `autotesters.BaseTester`, so it inherits the seeded `*rand.Rand` set by the runner via `SetRand` before `GenerateTestCases` is called.

---

## 7. Tester Design

### 7.1 MigrationTester Struct

```go
package autotesters

import (
    "context"
    "database/sql"
    "math/rand"
    "os"
    "path/filepath"

    sharedgoose "github.com/chendingplano/shared/go/api/goose"
    "github.com/chendingplano/shared/go/api/ApiTypes"
)

// MigrationTester tests the Goose migration system.
type MigrationTester struct {
    BaseTester  // Embed for default implementations

    // Configuration
    cfg *MigrationTesterConfig

    // Runtime state
    dutDB *sql.DB

    // Migration directories (created for testing)
    testMigrationsDir string

    // State tracking
    state MigrationSUTState
}
```

### 7.2 Prepare Phase

```go
func (t *MigrationTester) Prepare(ctx context.Context) error {
    // 1. Verify DUT is reachable
    if err := t.cfg.DUTDB.PingContext(ctx); err != nil {
        return fmt.Errorf("DUT not reachable: %w", err)
    }

    // 2. Validate DUT name starts with "testonly_"
    if !strings.HasPrefix(t.cfg.DUTDBName, "testonly_") {
        return fmt.Errorf("DUT name must start with 'testonly_'")
    }

    // 3. Validate migrations directory starts with "testonly_"
    if !strings.HasPrefix(t.cfg.MigrationsDir, "testonly_") {
        return fmt.Errorf("migrations dir must start with 'testonly_'")
    }

    // 4. Drop goose tracking table from DUT
    _, err := t.cfg.DUTDB.ExecContext(ctx, "DROP TABLE IF EXISTS "+t.cfg.TableName)
    if err != nil {
        return fmt.Errorf("drop tracking table: %w", err)
    }

    // 5. Drop all testonly_ tables from DUT
    if err := t.dropTestTables(ctx); err != nil {
        return fmt.Errorf("drop test tables: %w", err)
    }

    // 6. Clear the testonly_ directory
    if err := t.clearMigrationsDir(ctx); err != nil {
        return fmt.Errorf("clear migrations dir: %w", err)
    }

    // 7. Build the migrations pool
    if err := t.buildMigrationsPool(ctx); err != nil {
        return fmt.Errorf("build migrations pool: %w", err)
    }

    // 8. Initialize state
    t.syncState(ctx)

    return nil
}
```

### 7.3 Cleanup Phase

```go
func (t *MigrationTester) Cleanup(ctx context.Context) error {
    // 1. Drop all testonly_ tables from DUT
    if err := t.dropTestTables(ctx); err != nil {
        return err
    }

    // 2. Drop db_migrations from DUT
    _, err := t.cfg.DUTDB.ExecContext(ctx, "DROP TABLE IF EXISTS "+t.cfg.TableName)
    if err != nil {
        return err
    }

    // 3. Delete all .sql files from the testonly_ directory
    return t.clearMigrationsDir(ctx)
}
```

If `--skip-cleanup` is passed to the runner, `Cleanup` is not called, leaving DUT state intact for post-mortem inspection.

---

## 8. Test Cases

The tester supplies two pools of test cases following the AutoTester convention:

- **Static cases** (`GetTestCases`): Hard-coded, deterministic. Cover known invariants, edge cases, and regression scenarios that must pass on every run.
- **Dynamic cases** (`GenerateTestCases`): Randomly generated using `b.Rand()`. Cover the combinatorial parameter space defined in Section 3.

### 8.1 Static Test Cases (`GetTestCases`)

ID format: `TC_YYYYMMDDSS` where SS is a zero-padded sequence.

| ID | Name | Category | Priority | Dependencies |
|---|---|---|---|---|
| `TC_2026022301` | Apply all migrations from empty DB | A – Apply | Critical | — |
| `TC_2026022302` | Up is no-op when already current | A – Apply | High | `TC_2026022301` |
| `TC_2026022303` | Apply migrations one by one with UpByOne | A – Apply | High | — |
| `TC_2026022304` | UpByOne returns ErrNoNextVersion when all applied | A – Apply | High | `TC_2026022303` |
| `TC_2026022305` | UpTo applies migrations up to a target version | A – Apply | High | — |
| `TC_2026022306` | UpTo returns ErrVersionNotFound for nonexistent version | A – Apply | Medium | — |
| `TC_2026022307` | Roll back one migration (has Down SQL) | B – Rollback | Critical | `TC_2026022301` |
| `TC_2026022308` | Down returns error when nothing is applied | B – Rollback | High | — |
| `TC_2026022309` | Down returns error for migration with no Down SQL | B – Rollback | High | — |
| `TC_2026022310` | DownTo rolls back to a target version | B – Rollback | High | `TC_2026022301` |
| `TC_2026022311` | DownTo(0) rolls back all migrations | B – Rollback | High | `TC_2026022301` |
| `TC_2026022312` | Status returns correct applied/pending counts | C – Inspect | Medium | — |
| `TC_2026022313` | GetVersion returns 0 when nothing is applied | C – Inspect | Medium | — |
| `TC_2026022314` | HasPending returns true when pending migrations exist | C – Inspect | Medium | — |
| `TC_2026022315` | HasPending returns false when fully applied | C – Inspect | Medium | `TC_2026022301` |
| `TC_2026022316` | CreateAndApply writes file and applies migration | D – Create | High | — |
| `TC_2026022317` | CreateAndApply with empty downSQL succeeds; Down later fails | D – Create | High | — |
| `TC_2026022318` | CreateAndApply with invalid SQL returns error | D – Create | High | — |
| `TC_2026022319` | CreateAndApply filename follows YYYYMMDDHHMMSS_slug.sql naming | D – Create | Medium | — |
| `TC_2026022320` | Out-of-order migration with AllowOutOfOrder=true is applied | E – Order | Medium | — |
| `TC_2026022321` | Out-of-order migration with AllowOutOfOrder=false is rejected | E – Order | Medium | — |
| `TC_2026022322` | Up with empty migrations directory is a no-op | F – Edge | Medium | — |
| `TC_2026022323` | Tracking table is auto-created on first Up | F – Edge | Critical | — |
| `TC_2026022324` | Partial failure: PartialError lists what succeeded | F – Edge | High | — |
| `TC_2026022325` | Migration with NO TRANSACTION applies successfully | F – Edge | Medium | — |

**State isolation between static cases:** Each static case specifies its required pre-state in `TestCase.Input` (see Section 8.3). `RunTestCase` calls `resetToState(ctx, preState)` at the start of every case, so cases are independent of each other's side effects regardless of execution order.

### 8.2 Dynamic Test Cases (`GenerateTestCases`)

Dynamic test case generation is the **core testing mechanism**. This section provides detailed specification for the implementation.

#### 8.2.1 Generation Algorithm

```
GenerateTestCases(ctx, numCases int) []TestCase:
    1. Initialize generator with seeded RNG from BaseTester
    2. Query current MigrationSUTState from DUT
    3. FOR i = 1 to numCases:
        a. Select Operation based on weighted distribution
        b. Generate parameters respecting state-dependent constraints
        c. Compute ExpectedResult based on current state
        d. Build TestCase with migrationInput
        e. Update internal simulated state (for next iteration)
        f. Append TestCase to result list
    4. Return result list
```

#### 8.2.2 State Machine Model

The generator maintains an **internal simulated state machine** that mirrors the SUT state:

```
State Machine States:
  - Empty: No migrations in directory, nothing applied
  - HasPending: Migrations exist in directory, some or none applied
  - FullyApplied: All migrations in directory are applied
  - PartiallyApplied: Some migrations applied, some pending

State Transitions:
  Empty --(CreateAndApply)--> HasPending
  HasPending --(Up)--> FullyApplied
  HasPending --(UpByOne)--> PartiallyApplied (or FullyApplied if last one)
  HasPending --(Down)--> ERROR (nothing to rollback)
  FullyApplied --(Down)--> PartiallyApplied
  FullyApplied --(DownTo(0))--> Empty
  PartiallyApplied --(Up)--> FullyApplied
  PartiallyApplied --(Down)--> PartiallyApplied (or Empty if last one)
```

#### 8.2.3 Constraint-Based Parameter Selection

Parameters are generated with **state-dependent constraints**:

| Current State | Constraint | Rationale |
|---|---|---|
| `len(Applied) == 0` | `Down` operation → Skip or expect error | Cannot rollback nothing |
| `len(Applied) == len(FilesInDir)` | `Up` operation → Expect no-op | Already fully applied |
| `TargetVersion > CurrentVersion` | `UpTo` → Valid only if target exists | Cannot apply to nonexistent version |
| `TargetVersion >= CurrentVersion` | `DownTo` → Skip or expect error | Cannot rollback forward |
| `AllowOutOfOrder == false` | Out-of-order migration → Expect error | Enforce ordering |

#### 8.2.4 Expected Result Computation

For each generated test case, the **ExpectedResult** is computed deterministically:

```go
computeExpectedResult(op Operation, state MigrationSUTState, params Params) ExpectedResult {
    switch op {
    case OpUp:
        if state.HasPending():
            return ExpectedResult{Success: true, SideEffects: ["schema_table_applied"]}
        else:
            return ExpectedResult{Success: true, SideEffects: []} // No-op

    case OpDown:
        if len(state.Applied) == 0:
            return ExpectedResult{Success: false, ExpectedError: "no next version"}
        elif state.Applied[len-1].DownSQL == "":
            return ExpectedResult{Success: false, ExpectedError: "no down sql"}
        else:
            return ExpectedResult{Success: true, SideEffects: ["schema_table_dropped"]}

    case OpUpTo:
        if params.TargetVersion not in state.FilesInDir:
            return ExpectedResult{Success: false, ExpectedError: "version not found"}
        elif params.TargetVersion <= state.CurrentVersion:
            return ExpectedResult{Success: false, ExpectedError: "already applied"}
        else:
            return ExpectedResult{Success: true, ExpectedValue: params.TargetVersion}

    // ... similar for other operations
    }
}
```

#### 8.2.5 Coverage Goals

The generator tracks coverage metrics to ensure comprehensive testing:

| Coverage Dimension | Goal | Tracking Mechanism |
|---|---|---|
| **Operation Coverage** | All 9 operations exercised | Count per operation type |
| **State Coverage** | All 4 state machine states visited | State histogram |
| **Edge Case Coverage** | Empty dir, NO TRANSACTION, out-of-order | Tag-based tracking |
| **Error Path Coverage** | All error conditions triggered | Error type histogram |
| **Parameter Range Coverage** | Boundary values, invalid inputs | Range distribution tracking |

**Coverage-Driven Generation:** After initial random generation, the generator may bias toward under-covered areas:

```go
if coverage.OperationCoverage[OpDownTo] < threshold:
    weight[OpDownTo] *= 2.0  // Increase probability
if coverage.ErrorPathCoverage[PartialError] == 0:
    forceGenerateErrorCase(PartialError)
```

#### 8.2.6 Dynamic Case ID Format

```
TC_DYN_NNNN

Where:
  - TC_DYN_ = Prefix for dynamic cases
  - NNNN = Zero-padded sequence number (0001, 0002, ...)

Examples: TC_DYN_0001, TC_DYN_0042, TC_DYN_0100
```

Dynamic cases use `Priority: PriorityLow` by default. Cases that exercise edge cases are tagged `["edge-case"]`.

#### 8.2.7 Reproducibility

The seed is stored in `auto_test_runs.seed` by the runner. Any failing dynamic case can be replayed exactly by re-running with `--seed=<seed>`:

```bash
# Replay a failed test run
go run ./server/cmd/autotester/ --tester=tester_migration --seed=12345
```

### 8.3 `TestCase.Input`: The `migrationInput` Struct

All test cases (static and dynamic) store a `migrationInput` as `TestCase.Input`:

```go
// migrationInput is the typed value stored in TestCase.Input.
type migrationInput struct {
    Operation       MigrationOperation // Up, UpByOne, UpTo, Down, DownTo, Status, ...
    TargetVersion   int64              // for UpTo / DownTo
    UpSQL           string             // for CreateAndApply
    DownSQL         string             // for CreateAndApply; "" = no down SQL
    Description     string             // for CreateAndApply
    AllowOutOfOrder bool               // migrator Config.AllowOutOfOrder for this case

    // PreState is the DUT state this case requires before execution.
    // RunTestCase calls resetToState(ctx, PreState) before invoking the SUT.
    PreState MigrationSUTState
}
```

### 8.4 ExpectedResult and Verification

The `ExpectedResult` for each test case specifies:

| Field | Usage |
|---|---|
| `Success` | `true` for operations expected to succeed; `false` for error cases |
| `ExpectedError` | Substring expected in the error string |
| `ExpectedValue` | For inspection ops: expected version (`int64`) or `bool` for `HasPending` |
| `SideEffects` | Keys that must appear in `result.SideEffectsObserved` |
| `CustomValidator` | Semantic DB state comparison — queries `db_migrations` and `information_schema` |
| `MaxDuration` | `500ms` per test case; DDL on a local test DB should be fast |

**Side effect keys:**

| Key | Meaning |
|---|---|
| `tracking_table_created` | `db_migrations` table did not exist before and exists afterward |
| `schema_table_applied` | A `testonly_` table was created in DUT (Up SQL ran) |
| `schema_table_dropped` | A `testonly_` table was dropped from DUT (Down SQL ran) |
| `migration_file_written` | A new `.sql` file was written (by `CreateAndApply`) |

### 8.5 Test Case Categories

#### Category 1: Basic Apply/Rollback

| ID | Name | Input | Expected | Priority |
|---|---|---|---|---|
| `migration.basic.apply_all` | Apply all migrations | 3 files, Up | All applied, version=3 | Critical |
| `migration.basic.rollback_all` | Rollback all | 3 applied, DownTo(0) | All rolled back | Critical |
| `migration.basic.apply_one_by_one` | Apply incrementally | 5 files, UpByOne x5 | Version increments | High |
| `migration.basic.rollback_one_by_one` | Rollback incrementally | 5 applied, Down x5 | Version decrements | High |

#### Category 2: Status Inspection

| ID | Name | Input | Expected | Priority |
|---|---|---|---|---|
| `migration.status.pending` | Status with pending | 3 files, none applied | 3 pending | High |
| `migration.status.applied` | Status with applied | 3 files, all applied | 3 applied | High |
| `migration.status.has_pending_true` | HasPending when pending | 2 files, none applied | true | Medium |
| `migration.status.has_pending_false` | HasPending when applied | 2 files, all applied | false | Medium |

#### Category 3: Migration Creation

| ID | Name | Input | Expected | Priority |
|---|---|---|---|---|
| `migration.creation.create_table` | CreateAndApply table | CREATE TABLE SQL | File created, table exists | High |
| `migration.creation.add_column` | CreateAndApply column | ADD COLUMN SQL | File created, column exists | High |
| `migration.creation.no_down` | CreateAndApply with no down | SQL, downSQL="" | Succeeds, Down() fails | Medium |

#### Category 4: Edge Cases

| ID | Name | Input | Expected | Priority |
|---|---|---|---|---|
| `migration.edge.empty_dir` | Empty migration directory | No files, Up | No-op, no error | High |
| `migration.edge.no_transaction` | NO TRANSACTION migration | 1 file with NO TRANSACTION | Succeeds | High |
| `migration.edge.out_of_order` | Out-of-order apply | Apply v1, v3, then v2 | v2 applies | Medium |

#### Category 5: Error Handling

| ID | Name | Input | Expected | Priority |
|---|---|---|---|---|
| `migration.error.invalid_sql` | Invalid SQL syntax | Malformed SQL, Up | Error, rolled back | Critical |
| `migration.error.partial_error` | PartialError handling | 3 files, 2nd bad SQL | PartialError | High |

---

## 9. Tester Lifecycle

This maps directly to the AutoTester `Tester` interface lifecycle as driven by `TestRunner`.

### 9.1 Prepare(ctx) error

Called once before any test case runs.

1. **Verify DUT is reachable** — `dutDB.PingContext(ctx)`; return descriptive error if not
2. **Validate DUT name** — confirm `DUTDBName` starts with `testonly_`; return error if not
3. **Validate migrations directory** — confirm `MigrationsDir` starts with `testonly_`; return error if not
4. **Drop goose tracking table** — `DROP TABLE IF EXISTS db_migrations` on DUT
5. **Drop all `testonly_` tables** — query `information_schema.tables` for tables with names starting `testonly_`; drop each
6. **Clear the `testonly_` directory** — delete all `.sql` files from `MigrationsDir`
7. **Build the migrations pool** — pre-generate `MaxMigrationsInPool` synthetic migration files
8. **Initialize `MigrationSUTState`** — `Applied: []`, `FilesInDir: all pool files`, `CurrentVersion: 0`
9. **Build the migrator** — `sharedgoose.NewWithDB(dutDB, ...)`
10. **Record environment metadata** — capture PostgreSQL version from DUT

### 9.2 GenerateTestCases(ctx) ([]TestCase, error)

Called once after `Prepare`. Uses `b.Rand()` to generate `NumDynamicCases` test cases following the algorithm in Section 8.2.

### 9.3 GetTestCases() []TestCase

Returns the 25 static cases listed in Section 8.1.

### 9.4 RunTestCase(ctx, tc) TestResult

```go
func (t *MigrationTester) RunTestCase(ctx context.Context, tc TestCase) TestResult {
    result := TestResult{
        TestCaseID: tc.ID,
        TesterName: t.Name(),
        StartTime:  time.Now(),
    }

    defer func() {
        if r := recover(); r != nil {
            result.Status = StatusError
            result.Error = fmt.Sprintf("panic: %v\n%s", r, debug.Stack())
            result.EndTime = time.Now()
            result.Duration = result.EndTime.Sub(result.StartTime)
        }
    }()

    input := tc.Input.(migrationInput)

    // 1. Reset DUT to the pre-state expected by this case
    if err := t.resetToState(ctx, input.PreState); err != nil {
        result.Status = StatusError
        result.Error = fmt.Sprintf("resetToState failed: %v", err)
        result.EndTime = time.Now()
        result.Duration = result.EndTime.Sub(result.StartTime)
        return result
    }

    // 2. Rebuild migrator with case-specific AllowOutOfOrder setting
    migrator := t.buildMigrator(input.AllowOutOfOrder)

    // 3. Dispatch to per-operation handler
    t.dispatch(ctx, input, migrator, &result)

    // 4. Observe side effects
    t.observeSideEffects(ctx, &result)

    // 5. Sync internal state from DUT ground truth
    t.syncState(ctx)

    result.EndTime = time.Now()
    result.Duration = result.EndTime.Sub(result.StartTime)
    return result
}
```

### 9.5 Cleanup(ctx) error

1. Drop all `testonly_` tables from DUT
2. Drop `db_migrations` from DUT
3. Delete all `.sql` files from the `testonly_` migrations directory

---

## 10. Internal State Tracking

`MigrationTester` maintains `MigrationSUTState` as an internal field, updated after each `RunTestCase` by re-querying DUT.

```go
type MigrationSUTState struct {
    Applied          []MigrationRecord  // Migrations applied to DUT
    FilesInDir       []MigrationFile    // All .sql files in testonly_ dir
    Tables           map[string]bool    // Tables in DUT schema (testonly_ only)
    CurrentVersion   int64              // Highest applied version; 0 if nothing applied
}
```

### 10.1 State-Dependent Generation Rules

| Rule | Description |
|---|---|
| `Down` only generated if `len(Applied) > 0` | Prevents trivially invalid test cases |
| `UpTo(T)` valid target from `FilesInDir` versions only | For positive tests |
| `DownTo(T)` target satisfies `T < CurrentVersion` | For valid rollback |
| `CreateAndApply` generates unique table name | Avoids collision |

### 10.2 resetToState(ctx, preState MigrationSUTState) error

Used by `RunTestCase` to bring DUT into the exact state described by `preState`:

1. Call `resetDUT(ctx)` — drop tracking table + all `testonly_` tables
2. Clear and repopulate the `testonly_` dir with files from `preState.FilesInDir`
3. Rebuild the migrator
4. Apply exactly the migrations in `preState.Applied` using `UpByOne` repeatedly
5. Verify state matches `preState` via `syncState(ctx)`

---

## 11. Configuration

### 11.1 Database Configuration

The system uses **six databases** in total: **four production databases** and **two test databases**.

```toml
# mise.local.toml

# PostgreSQL connection settings
PG_HOST = "127.0.0.1"
PG_PORT = "5432"
PG_USER_NAME = "admin"
PG_PASSWORD = "<password>"

# ===== Production Databases (4 DBs) =====

# Project DB - application-specific tables and data
PG_DB_NAME = "<project_db>"

# Shared DB - shared library tables (common across all projects)

# Migration DB (Project) - goose version tracking for Project DB migrations
# CRITICAL: MigrationTester MUST NEVER access this
PG_DB_NAME_MIGRATIONS_PROJECT = "<migrations_project_db>"

# Migration DB (Shared) - goose version tracking for Shared DB migrations
# CRITICAL: MigrationTester MUST NEVER access this
PG_DB_NAME_MIGRATIONS_SHARED = "<migrations_shared_db>"

# ===== Test Databases (2 DBs) =====

# AutoTester DB - test results storage (managed by AutoTester framework)
PG_DB_NAME_AUTOTESTER = "<autotester_db>"

# DUT (DB-Under-Test) for MigrationTester
# CRITICAL: Name MUST start with "testonly_" for safety
# Configured programmatically in registry.go, NOT via environment variables
# Recommended naming: testonly_<project>_dut
```

### 11.2 Database Access Matrix

| Database | MigrationTester Access | Purpose |
|---|---|---|
| Project DB | May read/write | Application data tests |
| Shared DB | May read/write | Shared data tests |
| Migration DB (Project) | **NEVER** | Production migration tracking |
| Migration DB (Shared) | **NEVER** | Production migration tracking |
| AutoTester DB | Read/Write | Test result logging |
| DUT | Full access | Migration testing (isolated) |

### 11.3 MigrationTesterConfig

```go
// server/cmd/autotester/registry.go
func registerAll(cfg *Config) {
    // DUT: Separate test database, NEVER a production database
    // Name MUST start with "testonly_" for safety
    dutDB := openTestDB(cfg.DUTDSN)

    autotesters.GlobalRegistry.Register("tester_migration", func() autotesters.Tester {
        return autotesters.NewMigrationTester(&MigrationTesterConfig{
            DUTDB:               dutDB,
            DUTDBType:           "postgres",
            DUTDBName:           cfg.DUTDBName,  // Must start with "testonly_"
            MigrationsDir:       "testonly_migrations",  // Must start with "testonly_"
            TableName:           "db_migrations",
            NumDynamicCases:     80,
            MaxMigrationsInPool: 20,
        })
    })
}
```

| Field | Type | Default | Description |
|---|---|---|---|
| `DUTDB` | `*sql.DB` | — | Database connection for DUT |
| `DUTDBType` | `string` | `"postgres"` | Database type |
| `DUTDBName` | `string` | — | Database name; **MUST** start with `testonly_` |
| `MigrationsDir` | `string` | `"testonly_migrations"` | Directory; **MUST** start with `testonly_` |
| `TableName` | `string` | `"db_migrations"` | Goose version-tracking table name |
| `NumDynamicCases` | `int` | `80` | Number of dynamic test cases |
| `MaxMigrationsInPool` | `int` | `20` | Size of migrations pool |

### 11.4 CLI Configuration

```bash
# Run MigrationTester with custom seed
go run ./server/cmd/autotester/ --tester=tester_migration --seed=12345

# Run with parallel execution
go run ./server/cmd/autotester/ --tester=tester_migration --parallel

# Filter by tags
go run ./server/cmd/autotester/ --tags=migration,critical

# Run specific test cases
go run ./server/cmd/autotester/ --tester=tester_migration --test-ids=TC_2026022301,TC_2026022307

# Skip cleanup for debugging
go run ./server/cmd/autotester/ --tester=tester_migration --skip-cleanup

# Stop on first failure
go run ./server/cmd/autotester/ --tester=tester_migration --stop-on-fail
```

### 11.5 RunConfig Usage

| Config Field | Usage |
|---|---|
| `Seed` | Random seed for reproducible test generation |
| `Tags` | Filter test cases by tags |
| `TestIDs` | Run specific test case IDs |
| `Parallel` | Enable parallel Tester execution |
| `RetryCount` | Retry failed test cases |
| `CaseTimeout` | Per-test-case timeout (default: 30s) |
| `StopOnFail` | Stop on first failure |
| `SkipCleanup` | Skip Cleanup for debugging |
| `Verbose` | Enable debug logging |

### 11.6 Test Case Priority Distribution

| Priority | Count | Percentage |
|---|---|---|
| Critical | 8 | 16% |
| High | 18 | 36% |
| Medium | 18 | 36% |
| Low | 6 | 12% |

---

## 12. Architecture

### 12.1 Position Within AutoTester Framework

```
┌──────────────────────────────────────────────────────────────────┐
│        server/cmd/autotester/main.go  (any consuming project)    │
│                                                                  │
│  registerAll(cfg) {                                              │
│    autotesters.GlobalRegistry.Register("tester_migration",       │
│      func() autotesters.Tester {                                 │
│        return autotesters.NewMigrationTester(&MigrationTesterConfig{ │
│            DUTDB:     openTestDB(cfg), // DUT - NEVER production │
│            MigrationsDir: "testonly_migrations",                 │
│        })                                                        │
│      })                                                          │
│    // ... other testers                                          │
│  }                                                               │
└──────────────────────────┬───────────────────────────────────────┘
                           │
                           ▼
┌──────────────────────────────────────────────────────────────────┐
│                       TestRunner                                 │
│  ① SetRand(seededRand) on each Tester                            │
│  ② Prepare()                                                     │
│  ③ GenerateTestCases() + GetTestCases()  (merged)                │
│  ④ RunTestCase(tc) → verifyResult(tc, result) × N               │
│  ⑤ stream results → PG_DB_AutoTester                             │
│  ⑥ Cleanup()                                                     │
└──────────────────────────┬───────────────────────────────────────┘
                           │
              ┌────────────┴────────────┐
              ▼                         ▼
┌──────────────────────┐   ┌───────────────────────────┐
│   MigrationTester    │   │  PostgreSQL               │
│   (Tester interface) │   │  PG_DB_AutoTester         │
│                      │   │  auto_test_runs           │
│  ┌────────────────┐  │   │  auto_test_results        │
│  │ BaseTester     │  │   │  auto_test_logs           │
│  └────────────────┘  │   │                         │
│  - state tracking    │   └─────────────────────────┘
│  - resetToState      │
│  - dispatch          │
│  - observe effects   │
│  - syncState         │
└──────────────────────┘

Production Databases (NEVER accessed by MigrationTester):
┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐
│   Project DB    │ │   Shared DB     │ │ Migration DB    │ │ Migration DB    │
│  (app tables)   │ │ (shared tables) │ │   (Project)     │ │   (Shared)      │
│                 │ │                 │ │  (version track)│ │  (version track)│
└─────────────────┘ └─────────────────┘ └─────────────────┘ └─────────────────┘

Test Database (exclusively used by MigrationTester):
┌─────────────────┐
│      DUT        │
│ Name: testonly_*│
│ (test migrations│
│  + db_migrations│
└─────────────────┘
```

### 12.2 Verification Flow

```go
func verifyResult(tc TestCase, result TestResult) TestResult {
    // 1. Check skip
    if tc.SkipReason != "" {
        result.Status = StatusSkip
        result.Message = tc.SkipReason
        return result
    }

    // 2. Check dependencies
    for _, dep := range tc.Dependencies {
        if !passed[dep] {
            result.Status = StatusSkip
            result.Message = fmt.Sprintf("dependency %s not passed", dep)
            return result
        }
    }

    // 3. Check error expectation
    if tc.Expected.Success && result.Error != "" {
        result.Status = StatusFail
        result.Message = fmt.Sprintf("expected success, got error: %s", result.Error)
        return result
    }

    // 4. Check expected error content
    if tc.Expected.ExpectedError != "" {
        if !strings.Contains(result.Error, tc.Expected.ExpectedError) {
            result.Status = StatusFail
            result.Message = fmt.Sprintf("expected error containing %q, got: %s",
                tc.Expected.ExpectedError, result.Error)
            return result
        }
    }

    // 5. Check duration constraint
    if tc.Expected.MaxDuration > 0 && result.Duration > tc.Expected.MaxDuration {
        result.Status = StatusFail
        result.Message = fmt.Sprintf("exceeded max duration %v", tc.Expected.MaxDuration)
        return result
    }

    // 6. Check side effects
    for _, expected := range tc.Expected.SideEffects {
        found := false
        for _, observed := range result.SideEffectsObserved {
            if observed == expected {
                found = true
                break
            }
        }
        if !found {
            result.Status = StatusFail
            result.Message = fmt.Sprintf("expected side effect %q not observed", expected)
            return result
        }
    }

    // 7. Custom validator (for semantic DB state comparison)
    if tc.Expected.CustomValidator != nil {
        pass, reason := tc.Expected.CustomValidator(result.ActualValue, tc.Expected)
        if !pass {
            result.Status = StatusFail
            result.Message = reason
            return result
        }
    }

    // All checks passed
    result.Status = StatusPass
    return result
}
```

---

## 13. Implementation Plan

### Phase 1: Foundation (Week 1)

| Task | Description | Priority |
|---|---|---|
| P1-1 | Create `tester_migration.go` in `shared/go/api/autotesters/` | High |
| P1-2 | Implement `MigrationTester` struct with `BaseTester` embedding | High |
| P1-3 | Implement `Prepare` and `Cleanup` methods | High |
| P1-4 | Implement test migration file creation utilities | High |
| P1-5 | Implement version table reset utilities | High |

### Phase 2: Core Test Cases (Week 2)

| Task | Description | Priority |
|---|---|---|
| P2-1 | Implement static test cases (Section 8.1) | High |
| P2-2 | Implement dynamic case generator with weighted distributions | High |
| P2-3 | Implement `RunTestCase` routing and execution | High |
| P2-4 | Implement side effect observation | High |
| P2-5 | Implement `resetToState` for state isolation | High |
| P2-6 | Register tester in `server/cmd/autotester/registry.go` | High |

### Phase 3: Advanced Test Cases (Week 3)

| Task | Description | Priority |
|---|---|---|
| P3-1 | Implement state-dependent generation rules | Medium |
| P3-2 | Implement `CustomValidator` for semantic DB state comparison | Medium |
| P3-3 | Implement error handling test cases | Medium |
| P3-4 | Implement edge cases | Medium |
| P3-5 | Add comprehensive logging via `auto_test_logs` | Medium |

### Phase 4: Polish & Integration (Week 4)

| Task | Description | Priority |
|---|---|---|
| P4-1 | Implement test case filtering by tags | Medium |
| P4-2 | Write documentation and usage examples | High |
| P4-3 | Integration testing with real projects | High |
| P4-4 | Add to CI/CD pipeline | Medium |
| P4-5 | Performance optimization | Low |

---

## 14. File Structure

```
shared/
└── go/
    └── api/
        └── autotesters/
            ├── tester_migration.go         # MigrationTester implementation
            ├── tester_migration_types.go   # MigrationTesterConfig, MigrationSUTState, etc.
            ├── tester_migration_cases.go   # Static and dynamic case generation
            ├── tester_migration_handlers.go# Per-operation handlers
            └── tester_migration_state.go   # State tracking (resetToState, syncState)

myapp/  (e.g., tax/ or ChenWeb/)
└── server/
    └── cmd/
        └── autotester/
            ├── main.go                     # Entry point
            ├── config.go                   # CLI flags
            └── registry.go                 # Register MigrationTester
```

---

## 15. Open Items

### 15.1 TBD Items

| Item | Description |
|---|---|
| **SQL Template System** | How to generate varied but valid SQL for dynamic tests |
| **Transaction Handling** | Whether to wrap each test case in a transaction for isolation |
| **MySQL Support** | Whether to test MySQL migrations in addition to PostgreSQL |
| **Large Data Volumes** | Whether to test with realistic table sizes |

### 15.2 Questions

1. Should the tester run against all three tracks in parallel or sequentially?
2. Should failed migrations generate SQL fix suggestions?
3. How to handle migrations that modify data (not just schema)?
4. Should we maintain a library of pre-defined migration templates?

---

## 16. Change Log

### Version 0.6 (2026-02-24) - V4

**Major Changes:**

1. **Added Testbot Framework Reference**
   - Added `Testbot/testbot.md` to References section
   - Clarified that MigrationTester is a Testbot implementation within AutoTester framework

2. **Added Test Model Section (Section 3)**
   - New Section 3 defines the Test Model following `TM = {SUT, C, T, P}` from testbot.md
   - Explicitly maps Migration Tester components to Test Model elements
   - Added State Space definition (Section 3.7)

3. **Expanded SUT Operations (Section 4)**
   - Renamed from "SUT Definition" to "SUT Operations"
   - Added exhaustive analysis of all 10 operations
   - Each operation now includes: Purpose, Preconditions, Effects, Error Conditions, Edge Cases
   - Ensures complete coverage of the goose migration API surface

4. **Enhanced Dynamic Test Cases (Section 8.2)**
   - Added Generation Algorithm with step-by-step pseudocode
   - Added State Machine Model with 4 states and transitions
   - Added Constraint-Based Parameter Selection table
   - Added Expected Result Computation with example code
   - Added Coverage Goals with tracking mechanisms
   - Added Coverage-Driven Generation for under-covered areas
   - Clarified Dynamic Case ID format and reproducibility

5. **testonly_ Prefix Convention**
   - DUT database name **MUST** start with `testonly_` (Section 6.1, 7.2, 11.1, 11.3)
   - Migrations directory **MUST** start with `testonly_` (Section 7.2, 11.3)
   - Added validation in `Prepare` phase (Section 7.2)
   - Updated architecture diagrams to show `testonly_*` naming
   - This prevents accidental use of production databases

6. **Updated Configuration Section (Section 11)**
   - Added explicit `testonly_` naming requirements
   - Updated code examples with safety comments
   - Added validation rules for DUT name and migrations directory

7. **Reorganized Table of Contents**
   - Added Test Model as Section 3
   - Moved SUT Operations to Section 4
   - Renumbered subsequent sections
   - Added Change Log as Section 16

8. **Removed Changes from V2 Section**
   - Replaced with comprehensive Change Log format
   - Change Log now tracks all version changes going forward

**Why These Changes:**
- Aligns document with Testbot framework terminology and structure
- Makes Test Model explicit for clarity and implementation guidance
- Ensures SUT operations are complete and well-specified
- Provides detailed guidance for dynamic test case generation implementation
- Adds safety mechanism (`testonly_` prefix) to prevent production database accidents
- Establishes Change Log for future version tracking

---

## Appendix A: Example Test Run

```bash
# Run MigrationTester with verbose logging
$ go run ./server/cmd/autotester/ --tester=tester_migration --verbose --seed=12345

AutoTester run started
  run_id: a3f8d012-...
  seed:   12345
  env:    local

Running MigrationTester...
  [PASS] TC_2026022301 (45ms) - Apply all migrations from empty DB
  [PASS] TC_2026022302 (32ms) - Up is no-op when already current
  [PASS] TC_2026022303 (89ms) - Apply migrations one by one with UpByOne
  [FAIL] TC_2026022320 (12ms) - Out-of-order migration with AllowOutOfOrder=true
  [SKIP] TC_DYN_0042 (0ms) "dependency TC_2026022320 not passed"
  ...

AutoTester Run Complete
  Run ID   : a3f8d012-...
  Seed     : 12345
  Env      : local
  Duration : 2m 15s
  Total    : 31
  Passed   : 28 (90.3%)
  Failed   : 2  (6.5%)
  Skipped  : 1  (3.2%)
  Errored  : 0  (0.0%)
```

---

## Appendix B: SQL Templates

```go
var sqlTemplates = map[string]string{
    "create_table": `CREATE TABLE IF NOT EXISTS {{table_name}} (
        id BIGSERIAL PRIMARY KEY,
        name VARCHAR(255) NOT NULL,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
    )`,

    "drop_table": `DROP TABLE IF EXISTS {{table_name}}`,

    "add_column": `ALTER TABLE {{table_name}} ADD COLUMN IF NOT EXISTS {{column_name}} {{column_type}}`,

    "drop_column": `ALTER TABLE {{table_name}} DROP COLUMN IF EXISTS {{column_name}}`,

    "create_index": `CREATE INDEX IF NOT EXISTS idx_{{table_name}}_{{column_name}} ON {{table_name}}({{column_name}})`,

    "drop_index": `DROP INDEX IF EXISTS idx_{{table_name}}_{{column_name}}`,
}
```

---

## Appendix C: State Isolation Example

```go
// Example: resetToState brings DUT to exact pre-state before each test case
func (t *MigrationTester) resetToState(ctx context.Context, preState MigrationSUTState) error {
    // 1. Drop tracking table + all testonly_ tables
    if err := t.resetDUT(ctx); err != nil {
        return err
    }

    // 2. Clear and repopulate testonly_ dir
    if err := t.clearMigrationsDir(ctx); err != nil {
        return err
    }
    for _, file := range preState.FilesInDir {
        if err := t.writeMigrationFile(file); err != nil {
            return err
        }
    }

    // 3. Rebuild migrator
    migrator := t.buildMigrator(true)

    // 4. Apply exactly the migrations in preState.Applied
    for _, record := range preState.Applied {
        if err := migrator.UpByOne(ctx); err != nil {
            return fmt.Errorf("apply migration %d: %w", record.Version, err)
        }
    }

    // 5. Verify state matches
    if err := t.syncState(ctx); err != nil {
        return err
    }
    if t.state.CurrentVersion != preState.CurrentVersion {
        return fmt.Errorf("state mismatch: expected version %d, got %d",
            preState.CurrentVersion, t.state.CurrentVersion)
    }

    return nil
}
```

---

## Appendix D: References

- Testbot Framework: [`Testbot/testbot.md`](../../../Testbot/testbot.md)
- AutoTester Framework: [`Testbot/auto-tester-v2.md`](../../../Testbot/auto-tester-v2.md)
- Goose Migration Docs: [`shared/Documents/dev/goose-v1.md`](../../dev/goose-v1.md)
- Goose Upstream: https://github.com/pressly/goose
- Shared Autotesters: `shared/go/api/autotesters/`
