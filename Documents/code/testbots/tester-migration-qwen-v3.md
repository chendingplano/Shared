# Database Migration Tester - V4

**Version:** 0.5
**Date:** 2026-02-24
**Status:** Draft
**Author:** Combined from tester-migration-v2.md, tester-migration-v3.md, and tester-migration-qwen-v2.md

**References:**
- AutoTester framework: [`Testbot/auto-tester-v2.md`](../../../Testbot/auto-tester-v2.md)
- Database migration: [`shared/Documents/dev/goose-v1.md`](../../dev/goose-v1.md)

---

## Table of Contents

1. [Overview](#1-overview)
2. [Goals](#2-goals)
3. [SUT Definition](#3-sut-definition)
4. [Integration with AutoTester Framework](#4-integration-with-autotester-framework)
5. [Tester Identity](#5-tester-identity)
6. [Tester Design](#6-tester-design)
7. [SUT Parameters](#7-sut-parameters)
8. [Test Cases](#8-test-cases)
9. [Tester Lifecycle](#9-tester-lifecycle)
10. [Internal State Tracking](#10-internal-state-tracking)
11. [Configuration](#11-configuration)
12. [Architecture](#12-architecture)
13. [Implementation Plan](#13-implementation-plan)
14. [File Structure](#14-file-structure)
15. [Open Items](#15-open-items)
16. [Changes from V2](#16-changes-from-v2)

---

## 1. Overview

This document describes the plan for building a **Migration Tester** — a `Tester` implementation within the **AutoTester framework** (`shared/go/api/autotesters`, see `auto-tester-v2.md`) whose System Under Test (SUT) is the **goose-based database migration system** (`shared/go/api/goose`).

The `MigrationTester` implements the `autotesters.Tester` interface and is registered with the `TesterRegistry`. The AutoTester `TestRunner` drives its full lifecycle: `Prepare` → case supply → `RunTestCase` per case → `Cleanup`. Results are persisted to `PG_DB_AutoTester` by the framework's built-in database persistence layer — the tester does not implement its own logging or reporting.

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

## 3. SUT Definition

**SUT:** The goose migration wrapper in `shared/go/api/goose/goose.go`, accessed via `sharedgoose.NewWithDB(dutDB, ...)`.

The `MigrationTester` does **not** use the production migrators (`ProjectMigrator`, `SharedMigrator`, `AutoTesterMigrator`). It creates its own `Migrator` instance pointing at the **DB-Under-Test (DUT)** — a completely separate database used only for testing.

### 3.1 SUT Operations (Interface Surface)

| Operation | Method | Description |
|---|---|---|
| Apply all pending | `Up(ctx)` | Run all unapplied migrations |
| Apply one | `UpByOne(ctx)` | Apply exactly the next pending migration |
| Apply to version | `UpTo(ctx, version)` | Apply up to and including a specific version |
| Roll back one | `Down(ctx)` | Roll back the most recently applied migration |
| Roll back to version | `DownTo(ctx, version)` | Roll back all migrations newer than version |
| Check status | `Status(ctx)` | Return applied/pending state of all migrations |
| Get version | `GetVersion(ctx)` | Return highest applied version number |
| Check pending | `HasPending(ctx)` | Return true if any migration is pending |
| List sources | `ListSources()` | Return all known migration sources |
| Create + apply | `CreateAndApply(ctx, desc, upSQL, downSQL)` | Write a new SQL file and immediately apply it |

### 3.2 Database Architecture

#### 3.2.1 Production Databases (Four DBs)

The **production system** uses **four separate databases**:

| # | Database | Purpose | Migration Tester Access |
|---|---|---|---|
| 1 | **Project DB** | Application-specific tables and data | May read/write for data tests |
| 2 | **Shared DB** | Shared library tables (common across all projects) | May read/write for data tests |
| 3 | **Migration DB (Project)** | Goose version tracking table (`db_migrations`) for Project DB migrations | **NEVER ACCESS** — Production critical |
| 4 | **Migration DB (Shared)** | Goose version tracking table (`db_migrations`) for Shared DB migrations | **NEVER ACCESS** — Production critical |

#### 3.2.2 Test Databases

The **MigrationTester** uses **two separate test databases**:

| # | Database | Purpose | Migration Tester Access |
|---|---|---|---|
| 5 | **DUT (DB-Under-Test)** | Isolated test database for migration testing. All schema changes (`CREATE TABLE`, etc.) and the goose tracking table (`db_migrations`) live here. | **Full access** — Primary test target |
| 6 | **AutoTester DB** | Test result storage (`auto_test_runs`, `auto_test_results`, `auto_test_logs`). Managed by AutoTester framework. | **Read/Write** — Result logging |

#### 3.2.3 Critical Isolation Rule

> **The MigrationTester MUST NEVER access the production Migration DBs (Project or Shared).**
>
> All migration testing operations occur exclusively in the **DUT**, which is a completely separate database. This ensures:
> - Production migration state is never corrupted by tests
> - Tests can freely create/drop tables without affecting production
> - Each test run starts from a clean, isolated state

### 3.3 Database Setup Requirements

**All databases must already exist before the tester runs.** The tester does not create or drop databases.

Configure the production databases in the project's `mise.local.toml`:

```toml
# mise.local.toml

# PostgreSQL connection settings
PG_HOST = "127.0.0.1"
PG_PORT = "5432"
PG_USER_NAME = "admin"
PG_PASSWORD = "<password>"

# Production databases
PG_DB_NAME = "<project_db>"                    # Project DB
PG_DB_NAME_SHARED = "<shared_db>"              # Shared DB

# Production Migration DBs (NEVER accessed by MigrationTester)
PG_DB_NAME_MIGRATIONS_PROJECT = "<migrations_project_db>"  # Project migration tracking
PG_DB_NAME_MIGRATIONS_SHARED = "<migrations_shared_db>"    # Shared migration tracking

# AutoTester DB (test results)
PG_DB_NAME_AUTOTESTER = "<autotester_db>"

# DUT (DB-Under-Test) for MigrationTester
# Configured programmatically in registry.go, NOT via environment variables
# Recommended naming: <project_db>_test_dut
```

### 3.4 `Prepare` Invariants

Per the AutoTester `Prepare` contract, the following are enforced before any test case runs:

1. The migrations directory name **must** start with `testonly_` — this prevents accidental operations on production migration directories.
2. Upon `Prepare`, all tables whose names start with `testonly_` are dropped from **DUT**, so each run starts from a clean schema.
3. Upon `Prepare`, the goose tracking table (`db_migrations`) is dropped from **DUT**, so version tracking resets cleanly.
4. Upon `Prepare`, the `testonly_` migrations directory is emptied (all `.sql` files from previous runs deleted).

---

## 4. Integration with AutoTester Framework

### 4.1 Architecture Position

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
│ (test migrations│
│  + db_migrations│
└─────────────────┘
```

### 4.2 Tester Interface Implementation

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

### 4.3 Directory Structure

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

### 4.4 Database Logging

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

### 4.5 Registration

The Migration Tester is registered in `server/cmd/autotester/registry.go`:

```go
autotesters.GlobalRegistry.Register("tester_migration", func() autotesters.Tester {
    return autotesters.NewMigrationTester(&MigrationTesterConfig{...})
})
```

### 4.6 CLI Usage

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

## 5. Tester Identity

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

### 5.1 Constructor Configuration

```go
type MigrationTesterConfig struct {
    // DUT: Isolated test database for migration testing; NEVER production.
    DUTDB     *sql.DB
    DUTDBType string // "postgres" or "mysql"; default: "postgres"
    DUTDBName string // for logging only

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

## 6. Tester Design

### 6.1 MigrationTester Struct

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

### 6.2 Prepare Phase

```go
func (t *MigrationTester) Prepare(ctx context.Context) error {
    // 1. Verify DUT is reachable
    if err := t.cfg.DUTDB.PingContext(ctx); err != nil {
        return fmt.Errorf("DUT not reachable: %w", err)
    }

    // 2. Validate migrations directory starts with "testonly_"
    if !strings.HasPrefix(t.cfg.MigrationsDir, "testonly_") {
        return fmt.Errorf("migrations dir must start with 'testonly_'")
    }

    // 3. Drop goose tracking table from DUT
    _, err := t.cfg.DUTDB.ExecContext(ctx, "DROP TABLE IF EXISTS "+t.cfg.TableName)
    if err != nil {
        return fmt.Errorf("drop tracking table: %w", err)
    }

    // 4. Drop all testonly_ tables from DUT
    if err := t.dropTestTables(ctx); err != nil {
        return fmt.Errorf("drop test tables: %w", err)
    }

    // 5. Clear the testonly_ directory
    if err := t.clearMigrationsDir(ctx); err != nil {
        return fmt.Errorf("clear migrations dir: %w", err)
    }

    // 6. Build the migrations pool
    if err := t.buildMigrationsPool(ctx); err != nil {
        return fmt.Errorf("build migrations pool: %w", err)
    }

    // 7. Initialize state
    t.syncState(ctx)

    return nil
}
```

### 6.3 Cleanup Phase

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

## 7. SUT Parameters

These are the dimensions the tester randomizes when generating dynamic test cases via `GenerateTestCases`.

### 7.1 Parameter Table

| Parameter | Type | Valid Range | Invalid Range | Notes |
|---|---|---|---|---|
| `Operation` | enum | `{Up, UpByOne, UpTo, Down, DownTo, Status, GetVersion, HasPending, CreateAndApply}` | — | Migration operation to invoke |
| `NumMigrationsInDir` | integer | `[0, 20]` | `< 0` | How many `.sql` files exist in the `testonly_` dir at case generation time |
| `NumApplied` | integer | `[0, NumMigrationsInDir]` | `> NumMigrationsInDir` | How many of those migrations are already applied |
| `TargetVersion` | integer | A valid version in the dir, or `0` | Version not in dir (negative tests) | Used by `UpTo`, `DownTo` |
| `UpSQL` | string | Valid DDL (`CREATE TABLE`, `ALTER TABLE`, etc.) | Syntactically invalid SQL | Used by `CreateAndApply` |
| `DownSQL` | string | Valid DDL inverse, or `""` (no down) | Syntactically invalid SQL | `CreateAndApply`; empty is valid |
| `AllowOutOfOrder` | bool | `{true, false}` | — | Controls `AllowOutOfOrder` in migrator Config for this case |
| `HasNoTransaction` | bool | `{true, false}` | — | Whether the migration file starts with `-- +goose NO TRANSACTION` |

### 7.2 Weighted Distributions (Closeness Principle)

| Parameter | Distribution |
|---|---|
| `Operation` | `Up`: 30%, `Down`: 20%, `UpByOne`: 15%, `UpTo`: 10%, `DownTo`: 10%, `Status`/`GetVersion`/`HasPending`: 10%, `CreateAndApply`: 5% |
| `NumMigrationsInDir` | `[1,5]`: 60%, `[6,10]`: 25%, `[11,20]`: 10%, `0`: 5% |
| `NumApplied` (relative to dir size) | `0%` applied: 20%, `50%` applied: 40%, `100%` applied: 30%, random partial: 10% |
| `TargetVersion` — valid vs. invalid | Valid: 85%, Invalid/nonexistent: 15% |
| `UpSQL` — valid vs. invalid DDL | Valid DDL: 90%, invalid SQL: 10% |
| `DownSQL` — present vs. empty | Present: 70%, empty: 30% |
| `AllowOutOfOrder` | `true`: 70%, `false`: 30% |

---

## 8. Test Cases

The tester supplies two pools of test cases following the AutoTester convention:

- **Static cases** (`GetTestCases`): Hard-coded, deterministic. Cover known invariants, edge cases, and regression scenarios that must pass on every run.
- **Dynamic cases** (`GenerateTestCases`): Randomly generated using `b.Rand()`. Cover the combinatorial parameter space defined in Section 7.

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

Dynamic cases are generated using `b.Rand()` (the seeded `*rand.Rand` set by the runner). The generator reads the current `MigrationSUTState`, applies weighted distributions from Section 7.2, enforces state-dependent constraints from Section 10.1, computes the expected outcome, and constructs a `TestCase`.

```
Dynamic case ID format: TC_DYN_NNNN  (NNNN = 0-padded sequence within the run)
Examples: TC_DYN_0001, TC_DYN_0042
```

Dynamic cases use `Priority: PriorityLow` by default. Cases that exercise an empty migrations directory are tagged `["edge-case"]`.

Because the seed is stored in `auto_test_runs.seed` by the runner, any failing dynamic case can be replayed exactly by re-running with `--seed=<seed>`.

### 8.3 `TestCase.Input`: The `migrationInput` Struct

All test cases (static and dynamic) store a `migrationInput` as `TestCase.Input`:

```go
// migrationInput is the typed value stored in TestCase.Input.
type migrationInput struct {
    Operation       MigrationOperation // Up, UpByOne, UpTo, Down, DownTo, Status, ...
    TargetVersion   int64              // for UpTo / DownTo
    UpSQL           string             // for CreateAndApply
    DownSQL           string             // for CreateAndApply; "" = no down SQL
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
| `Success` | `true` for operations expected to succeed; `false` for error cases (invalid SQL, `ErrNoNextVersion`, etc.) |
| `ExpectedError` | Substring expected in the error string (e.g., `"no next version"`, `"version not found"`, `"partial"`) |
| `ExpectedValue` | For inspection ops: expected version (`int64`) or `bool` for `HasPending`; `nil` for ops with no scalar return |
| `SideEffects` | Keys that must appear in `result.SideEffectsObserved` |
| `CustomValidator` | Semantic DB state comparison — queries `db_migrations` and `information_schema` in DUT to verify the full tracking table state and schema; used for `Up`, `Down`, and rollback operations |
| `MaxDuration` | `500ms` per test case; DDL on a local test DB should be fast |

**Side effect keys:**

| Key | Meaning |
|---|---|
| `tracking_table_created` | `db_migrations` table did not exist before the operation and exists afterward |
| `schema_table_applied` | A `testonly_` table was created in DUT (Up SQL ran) |
| `schema_table_dropped` | A `testonly_` table was dropped from DUT (Down SQL ran) |
| `migration_file_written` | A new `.sql` file was written to the `testonly_` dir (by `CreateAndApply`) |

### 8.5 Test Case Categories (Alternative Organization)

#### Category 1: Basic Apply/Rollback

| ID | Name | Input | Expected | Priority |
|---|---|---|---|---|
| `migration.basic.apply_all` | Apply all migrations | 3 files, Up | All applied, version=3 | Critical |
| `migration.basic.rollback_all` | Rollback all | 3 applied, DownTo(0) | All rolled back | Critical |
| `migration.basic.apply_one_by_one` | Apply incrementally | 5 files, UpByOne x5 | Version increments | High |
| `migration.basic.rollback_one_by_one` | Rollback incrementally | 5 applied, Down x5 | Version decrements | High |
| `migration.basic.apply_to_version` | Apply to specific version | 5 files, UpTo(3) | First 3 applied | Medium |
| `migration.basic.rollback_to_version` | Rollback to version | 5 applied, DownTo(2) | First 2 remain | Medium |

#### Category 2: Status Inspection

| ID | Name | Input | Expected | Priority |
|---|---|---|---|---|
| `migration.status.pending` | Status with pending | 3 files, none applied | 3 pending | High |
| `migration.status.applied` | Status with applied | 3 files, all applied | 3 applied | High |
| `migration.status.has_pending_true` | HasPending when pending | 2 files, none applied | true | Medium |
| `migration.status.has_pending_false` | HasPending when applied | 2 files, all applied | false | Medium |
| `migration.status.get_version_empty` | GetVersion on empty DB | No migrations | 0 | Medium |
| `migration.status.get_version_after` | GetVersion after apply | 4 migrations applied | 4 | Medium |

#### Category 3: Migration Creation

| ID | Name | Input | Expected | Priority |
|---|---|---|---|---|
| `migration.creation.create_table` | CreateAndApply table | CREATE TABLE SQL | File created, table exists | High |
| `migration.creation.add_column` | CreateAndApply column | ADD COLUMN SQL | File created, column exists | High |
| `migration.creation.create_index` | CreateAndApply index | CREATE INDEX SQL | File created, index exists | Medium |
| `migration.creation.create_only` | CreateMigration without apply | CREATE TABLE SQL | File saved, not applied | Medium |
| `migration.creation.no_down` | CreateAndApply with no down | SQL, downSQL="" | Succeeds, Down() fails | Medium |

#### Category 4: Edge Cases

| ID | Name | Input | Expected | Priority |
|---|---|---|---|---|
| `migration.edge.empty_dir` | Empty migration directory | No files, Up | No-op, no error | High |
| `migration.edge.no_transaction` | NO TRANSACTION migration | 1 file with NO TRANSACTION | Succeeds | High |
| `migration.edge.out_of_order` | Out-of-order apply | Apply v1, v3, then v2 | v2 applies | Medium |
| `migration.edge.invalid_target` | Invalid target version | UpTo(999) | Error: version not found | Medium |
| `migration.edge.down_no_down` | Down on migration with no down | 1 migration, no down SQL | Error | Medium |
| `migration.edge.concurrent` | Concurrent HasPending | Multiple goroutines | Same result | Low |

#### Category 5: Error Handling

| ID | Name | Input | Expected | Priority |
|---|---|---|---|---|
| `migration.error.invalid_sql` | Invalid SQL syntax | Malformed SQL, Up | Error, rolled back | Critical |
| `migration.error.duplicate_table` | CREATE TABLE twice | Same table twice | Error on 2nd | High |
| `migration.error.drop_nonexistent` | DROP non-existent table | DROP TABLE x | Error | High |
| `migration.error.partial_error` | PartialError handling | 3 files, 2nd bad SQL | PartialError | High |
| `migration.error.infrastructure` | DB connection lost | Disconnect during Up | Infrastructure error | Medium |

#### Category 6: Multi-Track Testing

| ID | Name | Input | Expected | Priority |
|---|---|---|---|---|
| `migration.multitrack.independent` | Independent tracks | Up on all 3 tracks | Each independent | High |
| `migration.multitrack.isolation` | Track isolation | Mixed state per track | Correct per track | Medium |
| `migration.multitrack.cross_rollback` | Cross-track rollback | DownTo(0) on project only | Only project rolled back | Medium |

---

## 9. Tester Lifecycle

This maps directly to the AutoTester `Tester` interface lifecycle as driven by `TestRunner`.

### 9.1 Prepare(ctx) error

Called once before any test case runs.

1. **Verify DUT is reachable** — `dutDB.PingContext(ctx)`; return descriptive error if not (runner marks tester as errored and skips all cases)
2. **Validate migrations directory** — confirm `MigrationsDir` starts with `testonly_`; return error if not
3. **Drop goose tracking table** — `DROP TABLE IF EXISTS db_migrations` on DUT; version tracking resets for each run
4. **Drop all `testonly_` tables** — query `information_schema.tables` for tables with names starting `testonly_`; drop each; ensures no DDL residue from previous runs
5. **Clear the `testonly_` directory** — delete all `.sql` files from `MigrationsDir`
6. **Build the migrations pool** — pre-generate `MaxMigrationsInPool` synthetic migration files, each creating and dropping a `testonly_<N>` table, using the naming convention `YYYYMMDDHHMMSS_testonly_<N>.sql`; write them to `MigrationsDir`
7. **Initialize `MigrationSUTState`** — `Applied: []`, `FilesInDir: all pool files`, `CurrentVersion: 0`
8. **Build the migrator** — `sharedgoose.NewWithDB(dutDB, cfg.DUTDBType, goose.Config{MigrationsDir: cfg.MigrationsDir, TableName: cfg.TableName, AllowOutOfOrder: true})`
9. **Record environment metadata** — capture PostgreSQL version from DUT, Go runtime version; to be stored in `TestRun.EnvMetadata`

### 9.2 GenerateTestCases(ctx) ([]TestCase, error)

Called once after `Prepare`. Uses `b.Rand()` (the seeded random from `BaseTester`) to generate `NumDynamicCases` test cases. Each case is built by the internal `migrationCaseGenerator`:

1. Read current `MigrationSUTState` to understand what migrations exist and which are applied
2. Pick an `Operation` according to weighted distribution (Section 7.2)
3. Select parameter values respecting state-dependent constraints (Section 10.1)
4. Compute the expected outcome from the current state — this becomes `ExpectedResult`
5. Build a `migrationInput` with a `PreState` snapshot of the state at generation time
6. Construct and append the `TestCase`
7. **Update generator's internal state** — advance the simulated state to reflect what the generated case will do (so the next case builds on a coherent prior state)

The generator maintains its own simulated state separate from `MigrationSUTState`. `MigrationSUTState` is the ground truth queried from DUT after each `RunTestCase`; the generator's simulated state is used only to produce a coherent sequence during generation.

### 9.3 GetTestCases() []TestCase

Returns the 25 static cases listed in Section 8.1. The runner merges these with dynamic cases; together they form the full test suite for the run.

### 9.4 RunTestCase(ctx, tc) TestResult

```go
func (t *MigrationTester) RunTestCase(ctx context.Context, tc TestCase) TestResult {
    result := TestResult{
        TestCaseID: tc.ID,
        TesterName: t.Name(),
        StartTime:  time.Now(),
    }

    // Guard against panics
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
    // Runner calls verifyResult(tc, result) after this returns
}
```

Dispatch is by operation type for dynamic cases; by `tc.ID` for static cases:

```go
func (t *MigrationTester) dispatch(ctx context.Context, input migrationInput, m *sharedgoose.Migrator, r *TestResult) {
    switch input.Operation {
    case OpUp:           t.runUp(ctx, m, r)
    case OpUpByOne:      t.runUpByOne(ctx, m, r)
    case OpUpTo:         t.runUpTo(ctx, m, input.TargetVersion, r)
    case OpDown:         t.runDown(ctx, m, r)
    case OpDownTo:       t.runDownTo(ctx, m, input.TargetVersion, r)
    case OpStatus:       t.runStatus(ctx, m, r)
    case OpGetVersion:   t.runGetVersion(ctx, m, r)
    case OpHasPending:   t.runHasPending(ctx, m, r)
    case OpCreateAndApply:
        t.runCreateAndApply(ctx, m, input.Description, input.UpSQL, input.DownSQL, r)
    default:
        r.Status = StatusError
        r.Error = fmt.Sprintf("unknown operation: %v", input.Operation)
    }
}
```

`RunTestCase` fills in raw facts and side effects. It does **not** set `result.Status` to pass or fail — that is done by the runner's `verifyResult` using `ExpectedResult`.

### 9.5 Cleanup(ctx) error

1. Drop all `testonly_` tables from DUT (same as `Prepare` step 4)
2. Drop `db_migrations` from DUT
3. Delete all `.sql` files from the `testonly_` migrations directory

If `--skip-cleanup` is passed to the runner, `Cleanup` is not called, leaving DUT state intact for post-mortem inspection.

---

## 10. Internal State Tracking

`MigrationTester` maintains `MigrationSUTState` as an internal field, updated after each `RunTestCase` by re-querying DUT. This ground-truth sync prevents drift from accumulating.

```go
// MigrationSUTState is the canonical truth of what is in DUT and the migrations dir.
type MigrationSUTState struct {
    // Migrations applied to DUT, ascending version order
    Applied []MigrationRecord

    // All .sql files currently in the testonly_ dir
    FilesInDir []MigrationFile

    // Tables known to exist in DUT schema (testonly_ tables only)
    Tables map[string]bool

    // Highest applied version; 0 if nothing applied
    CurrentVersion int64
}

type MigrationRecord struct {
    Version  int64
    Filename string
    UpSQL    string
    DownSQL  string // empty = no down SQL
}

type MigrationFile struct {
    Version  int64
    Filename string
    UpSQL    string
    DownSQL  string
}
```

### 10.1 State-Dependent Generation Rules

| Rule | Description |
|---|---|
| `Down` only generated if `len(Applied) > 0` | Prevents trivially invalid test cases |
| `UpTo(T)` valid target selected from `FilesInDir` versions only | For positive tests; negative cases deliberately pick a version outside this set |
| `DownTo(T)` target satisfies `T < CurrentVersion` | For valid rollback cases |
| `UpByOne` expected result increments `CurrentVersion` by exactly one | Generator uses state to compute expected next version |
| `CreateAndApply` always generates a new, unique table name | Avoids collision with existing `testonly_` tables in the pool |

### 10.2 resetToState(ctx, preState MigrationSUTState) error

Used by `RunTestCase` to bring DUT into the exact state described by `preState` before invoking the SUT:

1. Call `resetDUT(ctx)` — drop tracking table + all `testonly_` tables
2. Clear and repopulate the `testonly_` dir with the files listed in `preState.FilesInDir`
3. Rebuild the migrator (pointing at the freshly populated dir)
4. Apply exactly the migrations in `preState.Applied` in ascending version order using `migrator.UpByOne` repeatedly
5. Verify the resulting state matches `preState` by calling `syncState(ctx)`; return error if mismatch

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
PG_DB_NAME_SHARED = "<shared_db>"

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
# Configured programmatically in registry.go, NOT via environment variables
# Recommended naming: <project_db>_test_dut
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

The `MigrationTester` is configured via `MigrationTesterConfig` in the project's registry:

```go
// server/cmd/autotester/registry.go
func registerAll(cfg *Config) {
    // DUT: Separate test database, NEVER a production database
    dutDB := openTestDB(cfg.DUTDSN)

    autotesters.GlobalRegistry.Register("tester_migration", func() autotesters.Tester {
        return autotesters.NewMigrationTester(&MigrationTesterConfig{
            DUTDB:               dutDB,
            DUTDBType:           "postgres", // or "mysql"
            DUTDBName:           cfg.DUTDBName,
            MigrationsDir:       "testonly_migrations", // MUST start with "testonly_"
            TableName:           "db_migrations",
            NumDynamicCases:     80,
            MaxMigrationsInPool: 20,
        })
    })
}
```

| Field | Type | Default | Description |
|---|---|---|---|
| `DUTDB` | `*sql.DB` | — | Database connection for the test database (DUT) |
| `DUTDBType` | `string` | `"postgres"` | Database type: `"postgres"` or `"mysql"` |
| `DUTDBName` | `string` | — | Database name (for logging only) |
| `MigrationsDir` | `string` | `"testonly_migrations"` | Directory for synthetic migrations; **must** start with `testonly_` |
| `TableName` | `string` | `"db_migrations"` | Goose version-tracking table name in DUT |
| `NumDynamicCases` | `int` | `80` | Number of dynamic test cases to generate per run |
| `MaxMigrationsInPool` | `int` | `20` | Size of pre-generated migrations pool in `Prepare` |

### 11.4 CLI Configuration

The AutoTester runner accepts the following flags:

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

The Migration Tester respects AutoTester's `RunConfig`:

| Config Field | Usage |
|---|---|
| `Seed` | Random seed for reproducible test generation |
| `Tags` | Filter test cases by tags (e.g., "migration,critical") |
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
│ (test migrations│
│  + db_migrations│
└─────────────────┘
```

### 12.2 Verification Flow

The AutoTester runner calls `verifyResult` after `RunTestCase`:

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
| P2-4 | Implement side effect observation (table created, version applied) | High |
| P2-5 | Implement `resetToState` for state isolation | High |
| P2-6 | Register tester in `server/cmd/autotester/registry.go` | High |

### Phase 3: Advanced Test Cases (Week 3)

| Task | Description | Priority |
|---|---|---|
| P3-1 | Implement state-dependent generation rules | Medium |
| P3-2 | Implement `CustomValidator` for semantic DB state comparison | Medium |
| P3-3 | Implement error handling test cases (invalid SQL, partial failures) | Medium |
| P3-4 | Implement edge cases (NO TRANSACTION, out-of-order, empty dir) | Medium |
| P3-5 | Add comprehensive logging via `auto_test_logs` | Medium |

### Phase 4: Polish & Integration (Week 4)

| Task | Description | Priority |
|---|---|---|
| P4-1 | Implement test case filtering by tags | Medium |
| P4-2 | Write documentation and usage examples | High |
| P4-3 | Integration testing with real projects | High |
| P4-4 | Add to CI/CD pipeline | Medium |
| P4-5 | Performance optimization for large test suites | Low |

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
            ├── tester_migration_handlers.go# Per-operation handlers (runUp, runDown, etc.)
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

## 16. Changes from V2

### 16.1 Database Architecture Clarification

**What Changed:**
- V2 incorrectly described the database architecture as having "three databases" or "four databases" with unclear distinctions.
- V3 correctly specifies **six databases total**: four production databases and two test databases.

**Production Databases (4):**
| Database | Purpose | MigrationTester Access |
|---|---|---|
| Project DB | Application-specific tables and data | May read/write |
| Shared DB | Shared library tables | May read/write |
| Migration DB (Project) | Goose version tracking for Project DB | **NEVER** |
| Migration DB (Shared) | Goose version tracking for Shared DB | **NEVER** |

**Test Databases (2):**
| Database | Purpose | MigrationTester Access |
|---|---|---|
| AutoTester DB | Test result storage | Read/Write |
| DUT | Isolated migration testing | Full access |

**Why:**
- The user clarified that the SUT (production system) uses **four databases**: Project DB, Shared DB, Migration DB for Project, and Migration DB for Shared.
- The MigrationTester may use tables in Project DB and Shared DB for data tests, but it **MUST NOT** access the production Migration DBs.
- All migration testing must occur in a separate, isolated DUT to prevent any risk to production migration state.

### 16.2 Critical Isolation Rule Added

**What Changed:**
- Added explicit "Critical Isolation Rule" in Section 3.2.3 stating that MigrationTester MUST NEVER access production Migration DBs.
- Added Database Access Matrix in Section 11.2 for quick reference.

**Why:**
- This is a critical safety requirement. Accessing production Migration DBs during testing could corrupt production migration state, leading to deployment failures or data corruption.

### 16.3 Configuration Section Updates

**What Changed:**
- Updated `mise.local.toml` example to include all six databases with clear comments.
- Added `PG_DB_NAME_MIGRATIONS_PROJECT` and `PG_DB_NAME_MIGRATIONS_SHARED` with explicit warnings.
- Clarified that DUT is configured programmatically, not via environment variables.

**Why:**
- Prevents accidental misconfiguration where a developer might point the tester at a production Migration DB.

### 16.4 Architecture Diagram Updates

**What Changed:**
- Updated architecture diagrams in Section 4.1 and Section 12.1 to show:
  - Four production databases (with Migration DBs marked as "NEVER accessed")
  - DUT as the exclusive test target
  - Clear visual separation between production and test databases

**Why:**
- Visual clarity helps developers understand the isolation boundaries at a glance.

### 16.5 Terminology Consistency

**What Changed:**
- Consistently uses "DUT (DB-Under-Test)" throughout the document.
- Clearly distinguishes between "production Migration DBs" and "test DUT".
- Added explicit notes in code comments (e.g., `// DUT: Separate test database, NEVER a production database`).

**Why:**
- Consistent terminology reduces confusion and prevents accidental misuse.

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

- AutoTester Framework: [`Testbot/auto-tester-v2.md`](../../../Testbot/auto-tester-v2.md)
- Goose Migration Docs: [`shared/Documents/dev/goose-v1.md`](../../dev/goose-v1.md)
- Goose Upstream: https://github.com/pressly/goose
- Shared Autotesters: `shared/go/api/autotesters/`
- Migration Tester V1: `shared/Documents/code/autotest/tester-migration-v1.md`
- Migration Tester V2: `shared/Documents/code/autotest/tester-migration-v2.md`