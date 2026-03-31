# Plan: Migration Testbot (v3)

**Author:** Claude <br>
**Date:** 2026-02-23 <br>
**Status:** Draft <br>
**References:**<br>
&emsp;&emsp;1. AutoTester framework: [`Testbot/auto-tester-v2.md`](../../../../Testbot/auto-tester-v2.md) <br>
&emsp;&emsp;2. Database migration: [`shared/Documents/dev/goose-v1.md`](../dev/goose-v1.md) <br>
&emsp;&emsp;3. Prior plans: [`tester-migration-v1.md`](./tester-migration-claude-v1.md), [`tester-migration-v2.md`](./tester-migration-qwen-v1.md)

---

## Change Log

### 2026-02-24 — Three-DB Architecture & Configuration Section

**Reason:** The original design used two dedicated test databases (DUT and AutoTester DB). The system now requires a third — the **Migration DB** — to keep the goose version-tracking table (`db_migrations`) isolated from the schema-change database (DUT). This separation allows tracking state and schema state to be inspected and reset independently, giving the tester finer-grained control over pre-conditions and making failures easier to diagnose.

**Changes made:**

| Location | Change |
|---|---|
| §2.2 Database Setup Requirements | Clarified that DUT holds only DDL schema changes; `db_migrations` now lives in the new Migration DB. Reordered entries so DUT → Migration DB → AutoTester DB matches the data-flow direction. Updated multi-track description to list DUT/Migration DB pairs per track. |
| §2.3 Prepare Invariants | Items 2 and 3 now explicitly name which database each reset targets (DUT for `testonly_*` tables; Migration DB for `db_migrations`). |
| §3.1 MigrationTesterConfig | Added `MigrationDB *sql.DB` and `MigrationDBName string` (required, single-track). Added `MigrationProjectDB`, `MigrationSharedDB`, `MigrationAutoTesterDB` (optional, multi-track). |
| §3.2 Constructor | Added nil-guard: `NewMigrationTester` panics early if `MigrationDB` is not provided. |
| §6.1 Prepare | Added step 2 (ping Migration DB). Renumbered subsequent steps. Steps 4 and 5 now target Migration DB and DUT respectively. Step 9 (`buildMigrator`) passes both `dutDB` and `migrationDB`. Step 10 (multi-track) covers both DUT and Migration DB pairs. |
| §6.5 Cleanup | Steps 1 and 2 now name DUT and Migration DB explicitly. Step 4 updated to reference DUT/Migration DB pairs. |
| §8.1 Architecture diagram | Registration snippet updated to show `MigrationDB` and all multi-track `Migration*DB` fields. Bottom of diagram split into separate DUT(s) and Migration DB(s) boxes to reflect the two-DB-per-track model. |
| §8.2 Internal Components | `buildMigrator` description updated to reflect dual-DB binding. `prepareMultiTrackDBs`/`cleanupMultiTrackDBs` descriptions updated to cover Migration DB pairs. |
| §9.1 Environment Variables | Added `PG_DB_NAME_MIGRATION` (required) and `PG_DB_NAME_MIGRATION_*` per-track variants (optional). Grouped and labelled all env vars by role for clarity. |
| §9.4 Registration | Updated code example to pass `MigrationDB` and all `Migration*DB` fields. |
| §9.5 Database Provisioning *(new)* | Added missing configuration section covering how to create and grant access to all three (or nine, for full multi-track) test databases, with minimum and full SQL setup examples plus a note on required permissions. |

---

## Table of Contents

1. [Overview](#1-overview)
2. [SUT Definition](#2-sut-definition)
3. [Tester Identity](#3-tester-identity)
4. [SUT Parameters](#4-sut-parameters)
5. [Test Cases](#5-test-cases)
6. [Tester Lifecycle](#6-tester-lifecycle)
7. [Internal State Tracking](#7-internal-state-tracking)
8. [Architecture](#8-architecture)
9. [Configuration](#9-configuration) — 9.1 Env Vars · 9.2 RunConfig · 9.3 CLI · 9.4 Registration · 9.5 Database Provisioning
10. [Implementation Plan](#10-implementation-plan)
11. [File Structure](#11-file-structure)
12. [Open Items](#12-open-items)
13. [Appendix A: SQL Templates](#appendix-a-sql-templates)
14. [Appendix B: Example Test Run](#appendix-b-example-test-run)

---

## 1. Overview

This document describes the plan for developing a `MigrationTester` — a `Tester` implementation within the **AutoTester framework** (`shared/go/api/autotesters`, see `auto-tester-v2.md`) whose System Under Test (SUT) is the **goose-based database migration system** (`shared/go/api/goose`).

The `MigrationTester` implements the `autotesters.Tester` interface and is registered with the `TesterRegistry`. The AutoTester `TestRunner` drives its full lifecycle: `Prepare` → case supply → `RunTestCase` per case → `Cleanup`. Results are persisted to `PG_DB_AutoTester` by the framework's built-in database persistence layer — the tester does not implement its own logging or reporting.

### 1.1 Background

The system uses **Goose** for database migrations with three separate migration tracks:
- **Project migrations** — Application-specific tables
- **Shared migrations** — Shared library tables (common across all projects)
- **AutoTester migrations** — Per-project isolated test result tables

Each track maintains independent version tracking, requiring comprehensive testing to ensure:
- Migrations apply correctly in order
- Rollbacks (Down migrations) work as expected
- Version tracking is accurate
- Edge cases and error conditions are handled properly

### 1.2 Goals

- Automatically and randomly test the migration system's correctness
- Verify that applying and rolling back migrations leaves the database in the expected state
- Detect regressions in version tracking, ordering, partial-failure handling, and rollback correctness
- Exercise the programmatic Go API: `Up`, `Down`, `UpByOne`, `UpTo`, `DownTo`, `Status`, `GetVersion`, `HasPending`, `CreateMigration`, `CreateAndApply`
- Test cross-track isolation: confirm that operations on one migration track do not affect others

### 1.3 Non-Goals

- This tester does **not** test application business logic — only the migration infrastructure
- This tester does **not** test the upstream `pressly/goose` library internals
- This tester does **not** validate production migration files — it generates synthetic ones
- This tester does **not** benchmark migration performance

### 1.4 Why This Matters

Migration bugs are high-impact: a failed `Up` or incorrect `Down` can corrupt a production database. Automated testing with random migration sequences catches ordering bugs, partial-failure handling, and rollback correctness that are hard to exercise manually. Multi-track isolation testing ensures test history never contaminates production data.

---

## 2. SUT Definition

**SUT:** The goose migration wrapper in `shared/go/api/goose/goose.go`, accessed via `sharedgoose.NewWithDB(dutDB, ...)`.

The `MigrationTester` does **not** use the production migrators (`ProjectMigrator`, `SharedMigrator`, `AutoTesterMigrator`). It creates its own `Migrator` instances pointing at the DB-Under-Test (DUT), one per track when multi-track cases are exercised.

### 2.1 SUT Operations (Interface Surface)

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
| Create only | `CreateMigration(desc, upSQL, downSQL)` | Write a new `.sql` file without applying it |
| Create + apply | `CreateAndApply(ctx, desc, upSQL, downSQL)` | Write a new SQL file and immediately apply it |

### 2.2 Database Setup Requirements

This tester requires three dedicated databases, all separate from production:

1. **DB-Under-Test (DUT)** — the dedicated test database where schema changes (e.g., `CREATE TABLE`,
   `ALTER TABLE`) are applied. Only DDL side effects from migrations land here; no goose tracking state.
2. **Migration DB** — the dedicated database where the goose package stores its version-tracking table
   (`db_migrations`). Keeping this separate from the DUT allows the tracking state to be inspected and
   reset independently of the schema.
3. **AutoTester DB** (`PG_DB_AutoTester`) — where test results (`auto_test_runs`, `auto_test_results`,
   `auto_test_logs`) are stored. Managed entirely by the AutoTester framework, not by this tester.

For multi-track cases, three DUT/Migration DB pairs are used (one pair per track):
`DUT_Project`/`Migration_Project`, `DUT_Shared`/`Migration_Shared`, `DUT_AutoTester`/`Migration_AutoTester`.

**All databases must already exist before the tester runs.** The tester does not create or drop databases.

### 2.3 `Prepare` Invariants

Per the AutoTester `Prepare` contract, the following are enforced before any test case runs:

1. The migrations directory name **must** start with `testonly_` — this prevents accidental operations on production migration directories.
2. Upon `Prepare`, all tables whose names start with `testonly_` are dropped from **DUT**, so each run starts from a clean schema.
3. Upon `Prepare`, the goose tracking table (`db_migrations`) is dropped from **Migration DB**, so version tracking resets cleanly.
4. Upon `Prepare`, the `testonly_` migrations directory is emptied (all `.sql` files from previous runs deleted).

---

## 3. Tester Identity

These values are returned by the `Tester` interface metadata methods, used by the AutoTester runner for filtering, logging, and registry lookup.

| Method | Value |
|---|---|
| `Name()` | `"tester_migration"` |
| `Description()` | `"Tests the goose database migration system (Up, Down, CreateAndApply, version tracking, multi-track isolation)"` |
| `Purpose()` | `"regression"` |
| `Type()` | `"integration"` |
| `Tags()` | `["database", "migration", "goose", "shared", "critical"]` |

**Struct:** `MigrationTester`
**Constructor:** `NewMigrationTester(cfg *MigrationTesterConfig) *MigrationTester`
**File:** `shared/go/api/autotesters/tester_migration.go`

### 3.1 Constructor Configuration

```go
type MigrationTesterConfig struct {
    // DUT: pre-existing test database where schema changes (DDL) are applied.
    // Must NOT be the production DB, the Migration DB, or the AutoTester DB.
    DUTDB     *sql.DB
    DUTDBType string // "postgres" or "mysql"; default: "postgres"
    DUTDBName string // for logging only

    // MigrationDB: pre-existing database where the goose tracking table (db_migrations) lives.
    // Kept separate from DUT so schema state and version-tracking state reset independently.
    MigrationDB     *sql.DB
    MigrationDBName string // for logging only

    // For multi-track test cases, one DUT/Migration DB pair per track is used.
    // If any pair is nil, multi-track cases are skipped.
    DUTProjectDB    *sql.DB
    DUTSharedDB     *sql.DB
    DUTAutoTesterDB *sql.DB

    MigrationProjectDB    *sql.DB
    MigrationSharedDB     *sql.DB
    MigrationAutoTesterDB *sql.DB

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

### 3.2 Complete Struct Definition

```go
package autotesters

import (
    "context"
    "database/sql"

    sharedgoose "github.com/chendingplano/shared/go/api/goose"
)

// MigrationTester tests the Goose migration system.
type MigrationTester struct {
    BaseTester // Embed for default implementations and seeded rand

    cfg *MigrationTesterConfig

    // Runtime migrator (DUT-bound); rebuilt per case for AllowOutOfOrder variation
    migrator *sharedgoose.Migrator

    // Ground-truth state of what is in DUT and the migrations dir
    state MigrationSUTState
}

func NewMigrationTester(cfg *MigrationTesterConfig) *MigrationTester {
    if cfg.MigrationsDir == "" {
        cfg.MigrationsDir = "testonly_migrations"
    }
    if cfg.TableName == "" {
        cfg.TableName = "db_migrations"
    }
    if cfg.NumDynamicCases == 0 {
        cfg.NumDynamicCases = 80
    }
    if cfg.MaxMigrationsInPool == 0 {
        cfg.MaxMigrationsInPool = 20
    }
    if cfg.MigrationDB == nil {
        panic("MigrationTesterConfig.MigrationDB must not be nil")
    }
    return &MigrationTester{
        BaseTester: BaseTester{
            name:        "tester_migration",
            description: "Tests the goose database migration system (Up, Down, CreateAndApply, version tracking, multi-track isolation)",
            purpose:     "regression",
            testType:    "integration",
            tags:        []string{"database", "migration", "goose", "shared", "critical"},
        },
        cfg: cfg,
    }
}
```

---

## 4. SUT Parameters

These are the dimensions the tester randomizes when generating dynamic test cases via `GenerateTestCases`.

### 4.1 Parameter Table

| Parameter | Type | Valid Range | Invalid Range | Notes |
|---|---|---|---|---|
| `Operation` | enum | `{Up, UpByOne, UpTo, Down, DownTo, Status, GetVersion, HasPending, CreateMigration, CreateAndApply}` | — | Migration operation to invoke |
| `NumMigrationsInDir` | integer | `[0, 20]` | `< 0` | How many `.sql` files exist in the `testonly_` dir at case generation time |
| `NumApplied` | integer | `[0, NumMigrationsInDir]` | `> NumMigrationsInDir` | How many of those migrations are already applied |
| `TargetVersion` | integer | A valid version in the dir, or `0` | Version not in dir (negative tests) | Used by `UpTo`, `DownTo` |
| `UpSQL` | string | Valid DDL (`CREATE TABLE`, `ALTER TABLE`, etc.) | Syntactically invalid SQL | Used by `CreateMigration` and `CreateAndApply` |
| `DownSQL` | string | Valid DDL inverse, or `""` (no down) | Syntactically invalid SQL | `CreateMigration`/`CreateAndApply`; empty is valid |
| `AllowOutOfOrder` | bool | `{true, false}` | — | Controls `AllowOutOfOrder` in migrator Config for this case |
| `HasNoTransaction` | bool | `{true, false}` | — | Whether the migration file starts with `-- +goose NO TRANSACTION` |
| `Track` | enum | `{single, multi}` | — | `single` uses DUT; `multi` uses all three DUT DBs |

### 4.2 Weighted Distributions

| Parameter | Distribution |
|---|---|
| `Operation` | `Up`: 25%, `Down`: 15%, `UpByOne`: 15%, `UpTo`: 10%, `DownTo`: 10%, `Status`/`GetVersion`/`HasPending`: 10%, `CreateAndApply`: 8%, `CreateMigration`: 7% |
| `NumMigrationsInDir` | `[1,5]`: 60%, `[6,10]`: 25%, `[11,20]`: 10%, `0`: 5% |
| `NumApplied` (relative to dir size) | `0%` applied: 20%, `50%` applied: 40%, `100%` applied: 30%, random partial: 10% |
| `TargetVersion` — valid vs. invalid | Valid: 85%, Invalid/nonexistent: 15% |
| `UpSQL` — valid vs. invalid DDL | Valid DDL: 90%, invalid SQL: 10% |
| `DownSQL` — present vs. empty | Present: 70%, empty: 30% |
| `AllowOutOfOrder` | `true`: 70%, `false`: 30% |
| `Track` | `single`: 85%, `multi`: 15% |

---

## 5. Test Cases

The tester supplies two pools of test cases following the AutoTester convention:

- **Static cases** (`GetTestCases`): Hard-coded, deterministic. Cover known invariants, edge cases, and regression scenarios that must pass on every run.
- **Dynamic cases** (`GenerateTestCases`): Randomly generated using `b.Rand()`. Cover the combinatorial parameter space defined in Section 4.

### 5.1 Static Test Cases (`GetTestCases`)

ID format: `TC_YYYYMMDDSS` where SS is a zero-padded sequence (per `auto-tester-v2.md` convention).

#### Category A — Apply

| ID | Name | Priority | Dependencies |
|---|---|---|---|
| `TC_2026022301` | Apply all migrations from empty DB | Critical | — |
| `TC_2026022302` | Up is no-op when already current | High | `TC_2026022301` |
| `TC_2026022303` | Apply migrations one by one with UpByOne | High | — |
| `TC_2026022304` | UpByOne returns ErrNoNextVersion when all applied | High | `TC_2026022303` |
| `TC_2026022305` | UpTo applies migrations up to a target version | High | — |
| `TC_2026022306` | UpTo returns ErrVersionNotFound for nonexistent version | Medium | — |

#### Category B — Rollback

| ID | Name | Priority | Dependencies |
|---|---|---|---|
| `TC_2026022307` | Roll back one migration (has Down SQL) | Critical | `TC_2026022301` |
| `TC_2026022308` | Down returns error when nothing is applied | High | — |
| `TC_2026022309` | Down returns error for migration with no Down SQL | High | — |
| `TC_2026022310` | DownTo rolls back to a target version | High | `TC_2026022301` |
| `TC_2026022311` | DownTo(0) rolls back all migrations | High | `TC_2026022301` |

#### Category C — Status Inspection

| ID | Name | Priority | Dependencies |
|---|---|---|---|
| `TC_2026022312` | Status returns correct applied/pending counts | Medium | — |
| `TC_2026022313` | GetVersion returns 0 when nothing is applied | Medium | — |
| `TC_2026022314` | HasPending returns true when pending migrations exist | Medium | — |
| `TC_2026022315` | HasPending returns false when fully applied | Medium | `TC_2026022301` |

#### Category D — Create

| ID | Name | Priority | Dependencies |
|---|---|---|---|
| `TC_2026022316` | CreateAndApply writes file and applies migration | High | — |
| `TC_2026022317` | CreateAndApply with empty downSQL succeeds; Down later fails | High | — |
| `TC_2026022318` | CreateAndApply with invalid SQL returns error | High | — |
| `TC_2026022319` | CreateAndApply filename follows YYYYMMDDHHMMSS_slug.sql naming | Medium | — |
| `TC_2026022329` | CreateMigration writes file without applying it | Medium | — |

#### Category E — Ordering

| ID | Name | Priority | Dependencies |
|---|---|---|---|
| `TC_2026022320` | Out-of-order migration with AllowOutOfOrder=true is applied | Medium | — |
| `TC_2026022321` | Out-of-order migration with AllowOutOfOrder=false is rejected | Medium | — |

#### Category F — Edge Cases

| ID | Name | Priority | Dependencies |
|---|---|---|---|
| `TC_2026022322` | Up with empty migrations directory is a no-op | Medium | — |
| `TC_2026022323` | Tracking table is auto-created on first Up | Critical | — |
| `TC_2026022324` | Partial failure: PartialError lists what succeeded | High | — |
| `TC_2026022325` | Migration with NO TRANSACTION applies successfully | Medium | — |

#### Category G — Multi-Track

| ID | Name | Priority | Dependencies |
|---|---|---|---|
| `TC_2026022326` | Apply all three tracks independently; each has isolated state | High | — |
| `TC_2026022327` | Track isolation: mixed applied/pending state per track is maintained correctly | Medium | `TC_2026022326` |
| `TC_2026022328` | Cross-track rollback: DownTo(0) on project track does not affect shared or autotester tracks | Medium | `TC_2026022326` |

**Total static cases: 29**

**State isolation between static cases:** Each static case specifies its required pre-state in `TestCase.Input` (see Section 5.3). `RunTestCase` calls `resetToState(ctx, preState)` at the start of every case, so cases are independent of each other's side effects regardless of execution order.

### 5.2 Dynamic Test Cases (`GenerateTestCases`)

Dynamic cases are generated using `b.Rand()` (the seeded `*rand.Rand` set by the runner). The generator reads the current `MigrationSUTState`, applies weighted distributions from Section 4.2, enforces state-dependent constraints from Section 7.1, computes the expected outcome, and constructs a `TestCase`.

```
Dynamic case ID format: TC_DYN_NNNN  (NNNN = 0-padded sequence within the run)
Examples: TC_DYN_0001, TC_DYN_0042
```

Dynamic cases use `Priority: PriorityLow` by default. Cases that exercise an empty migrations directory are tagged `["edge-case"]`. Multi-track dynamic cases are tagged `["multi-track"]`.

Because the seed is stored in `auto_test_runs.seed` by the runner, any failing dynamic case can be replayed exactly by re-running with `--seed=<seed>`.

### 5.3 `TestCase.Input`: The `migrationInput` Struct

All test cases (static and dynamic) store a `migrationInput` as `TestCase.Input`:

```go
// migrationInput is the typed value stored in TestCase.Input.
type migrationInput struct {
    Operation       MigrationOperation // Up, UpByOne, UpTo, Down, DownTo, Status, ...
    TargetVersion   int64              // for UpTo / DownTo
    UpSQL           string             // for CreateMigration / CreateAndApply
    DownSQL         string             // for CreateMigration / CreateAndApply; "" = no down SQL
    Description     string             // for CreateMigration / CreateAndApply
    AllowOutOfOrder bool               // migrator Config.AllowOutOfOrder for this case
    Track           string             // "single" or "multi"

    // PreState is the DUT state this case requires before execution.
    // RunTestCase calls resetToState(ctx, PreState) before invoking the SUT.
    PreState MigrationSUTState
}

// MigrationOperation enumerates all testable SUT operations.
type MigrationOperation int

const (
    OpUp MigrationOperation = iota
    OpUpByOne
    OpUpTo
    OpDown
    OpDownTo
    OpStatus
    OpGetVersion
    OpHasPending
    OpCreateMigration
    OpCreateAndApply
)
```

### 5.4 `ExpectedResult` and Verification

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
| `migration_file_written` | A new `.sql` file was written to the `testonly_` dir (by `CreateMigration` or `CreateAndApply`) |
| `track_isolated` | Multi-track case: DUT state on the other tracks was unchanged |

---

## 6. Tester Lifecycle

This maps directly to the AutoTester `Tester` interface lifecycle as driven by `TestRunner`.

### 6.1 `Prepare(ctx) error`

Called once before any test case runs.

1. **Verify DUT is reachable** — `dutDB.PingContext(ctx)`; return descriptive error if not (runner marks tester as errored and skips all cases)
2. **Verify Migration DB is reachable** — `migrationDB.PingContext(ctx)`; return descriptive error if not
3. **Validate migrations directory** — confirm `MigrationsDir` starts with `testonly_`; return error if not
4. **Drop goose tracking table** — `DROP TABLE IF EXISTS db_migrations` on **Migration DB**; version tracking resets for each run
5. **Drop all `testonly_` tables** — query `information_schema.tables` for tables with names starting `testonly_` on **DUT**; drop each; ensures no DDL residue from previous runs
6. **Clear the `testonly_` directory** — delete all `.sql` files from `MigrationsDir`
7. **Build the migrations pool** — pre-generate `MaxMigrationsInPool` synthetic migration files, each creating and dropping a `testonly_<N>` table, using the naming convention `YYYYMMDDHHMMSS_testonly_<N>.sql`; write them to `MigrationsDir`
8. **Initialize `MigrationSUTState`** — `Applied: []`, `FilesInDir: all pool files`, `CurrentVersion: 0`
9. **Build the migrator** — `sharedgoose.NewWithDB(dutDB, migrationDB, cfg.DUTDBType, goose.Config{MigrationsDir: cfg.MigrationsDir, TableName: cfg.TableName, AllowOutOfOrder: true})`
10. **Prepare multi-track DUT/Migration DB pairs** (if configured) — repeat steps 4–6 on each pair: (`DUTProjectDB`, `MigrationProjectDB`), (`DUTSharedDB`, `MigrationSharedDB`), (`DUTAutoTesterDB`, `MigrationAutoTesterDB`)
11. **Record environment metadata** — capture PostgreSQL version from DUT, Go runtime version; stored in `TestRun.EnvMetadata`

```go
func (t *MigrationTester) Prepare(ctx context.Context) error {
    // 1. Verify DUT is reachable
    if err := t.cfg.DUTDB.PingContext(ctx); err != nil {
        return fmt.Errorf("DUT not reachable: %w", err)
    }

    // 2. Verify Migration DB is reachable
    if err := t.cfg.MigrationDB.PingContext(ctx); err != nil {
        return fmt.Errorf("MigrationDB not reachable: %w", err)
    }

    // 3. Validate migrations directory
    if !strings.HasPrefix(t.cfg.MigrationsDir, "testonly_") {
        return fmt.Errorf("MigrationsDir %q must start with 'testonly_'", t.cfg.MigrationsDir)
    }

    // 4–6. Reset DUT (schema tables), Migration DB (tracking table), and migrations dir
    if err := t.resetDUT(ctx); err != nil {
        return fmt.Errorf("resetDUT: %w", err)
    }

    // 7. Build migration pool
    if err := t.buildMigrationPool(ctx); err != nil {
        return fmt.Errorf("buildMigrationPool: %w", err)
    }

    // 8. Initialize state
    if err := t.syncState(ctx); err != nil {
        return fmt.Errorf("initial syncState: %w", err)
    }

    // 9. Build migrator (DUT for schema, MigrationDB for tracking)
    t.migrator = t.buildMigrator(true)

    // 10. Prepare multi-track DUT/Migration DB pairs (if configured)
    if err := t.prepareMultiTrackDBs(ctx); err != nil {
        return fmt.Errorf("prepareMultiTrackDBs: %w", err)
    }

    return nil
}
```

### 6.2 `GenerateTestCases(ctx) ([]TestCase, error)`

Called once after `Prepare`. Uses `b.Rand()` (the seeded random from `BaseTester`) to generate `NumDynamicCases` test cases. Each case is built by the internal `migrationCaseGenerator`:

1. Read current `MigrationSUTState` to understand what migrations exist and which are applied
2. Pick an `Operation` according to weighted distribution (Section 4.2)
3. Select parameter values respecting state-dependent constraints (Section 7.1)
4. Compute the expected outcome from the current state — this becomes `ExpectedResult`
5. Build a `migrationInput` with a `PreState` snapshot of the state at generation time
6. Construct and append the `TestCase`
7. **Update generator's internal state** — advance the simulated state to reflect what the generated case will do (so the next case builds on a coherent prior state)

The generator maintains its own simulated state separate from `MigrationSUTState`. `MigrationSUTState` is the ground truth queried from DUT after each `RunTestCase`; the generator's simulated state is used only to produce a coherent sequence during generation.

### 6.3 `GetTestCases() []TestCase`

Returns the 29 static cases listed in Section 5.1. The runner merges these with dynamic cases; together they form the full test suite for the run.

### 6.4 `RunTestCase(ctx, tc) TestResult`

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

func (t *MigrationTester) dispatch(ctx context.Context, input migrationInput, m *sharedgoose.Migrator, r *TestResult) {
    switch input.Operation {
    case OpUp:              t.runUp(ctx, m, r)
    case OpUpByOne:         t.runUpByOne(ctx, m, r)
    case OpUpTo:            t.runUpTo(ctx, m, input.TargetVersion, r)
    case OpDown:            t.runDown(ctx, m, r)
    case OpDownTo:          t.runDownTo(ctx, m, input.TargetVersion, r)
    case OpStatus:          t.runStatus(ctx, m, r)
    case OpGetVersion:      t.runGetVersion(ctx, m, r)
    case OpHasPending:      t.runHasPending(ctx, m, r)
    case OpCreateMigration: t.runCreateMigration(ctx, m, input.Description, input.UpSQL, input.DownSQL, r)
    case OpCreateAndApply:  t.runCreateAndApply(ctx, m, input.Description, input.UpSQL, input.DownSQL, r)
    default:
        r.Status = StatusError
        r.Error = fmt.Sprintf("unknown operation: %v", input.Operation)
    }
}
```

`RunTestCase` fills in raw facts and side effects. It does **not** set `result.Status` to pass or fail — that is done by the runner's `verifyResult` using `ExpectedResult`.

### 6.5 `Cleanup(ctx) error`

1. Drop all `testonly_` tables from **DUT** (same as `Prepare` step 5)
2. Drop `db_migrations` from **Migration DB** (same as `Prepare` step 4)
3. Delete all `.sql` files from the `testonly_` migrations directory
4. Repeat steps 1–3 on multi-track DUT/Migration DB pairs if configured

If `--skip-cleanup` is passed to the runner, `Cleanup` is not called, leaving DUT state intact for post-mortem inspection.

```go
func (t *MigrationTester) Cleanup(ctx context.Context) error {
    if err := t.resetDUT(ctx); err != nil {
        return fmt.Errorf("cleanup resetDUT: %w", err)
    }
    return t.cleanupMultiTrackDBs(ctx)
}
```

---

## 7. Internal State Tracking

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

### 7.1 State-Dependent Generation Rules

| Rule | Description |
|---|---|
| `Down` only generated if `len(Applied) > 0` | Prevents trivially invalid test cases |
| `UpTo(T)` valid target selected from `FilesInDir` versions only | For positive tests; negative cases deliberately pick a version outside this set |
| `DownTo(T)` target satisfies `T < CurrentVersion` | For valid rollback cases |
| `UpByOne` expected result increments `CurrentVersion` by exactly one | Generator uses state to compute expected next version |
| `CreateAndApply` always generates a new, unique table name | Avoids collision with existing `testonly_` tables in the pool |
| `CreateMigration` does not advance `CurrentVersion` | File is written but not applied; `HasPending` becomes true |
| Multi-track `Track=multi` only generated when all three DUT DBs are configured | Falls back to `single` if multi-track DBs are absent |

### 7.2 `resetToState(ctx, preState MigrationSUTState) error`

Used by `RunTestCase` to bring DUT into the exact state described by `preState` before invoking the SUT:

1. Call `resetDUT(ctx)` — drop tracking table + all `testonly_` tables
2. Clear and repopulate the `testonly_` dir with the files listed in `preState.FilesInDir`
3. Rebuild the migrator (pointing at the freshly populated dir)
4. Apply exactly the migrations in `preState.Applied` in ascending version order using `migrator.UpByOne` repeatedly
5. Verify the resulting state matches `preState` by calling `syncState(ctx)`; return error if mismatch

---

## 8. Architecture

### 8.1 Position Within AutoTester Framework

```
┌──────────────────────────────────────────────────────────────────┐
│        server/cmd/autotester/main.go  (any consuming project)    │
│                                                                  │
│  registerAll(cfg) {                                              │
│    autotesters.GlobalRegistry.Register("tester_migration",       │
│      func() autotesters.Tester {                                 │
│        return autotesters.NewMigrationTester(&MigrationTesterConfig{ │
│            DUTDB:         openDB(cfg, "dut"),                    │
│            MigrationDB:   openDB(cfg, "migration"),              │
│            MigrationsDir: "testonly_migrations",                 │
│            // optional multi-track pairs:                        │
│            DUTProjectDB:         openDB(cfg, "dut_project"),     │
│            MigrationProjectDB:   openDB(cfg, "mig_project"),     │
│            DUTSharedDB:          openDB(cfg, "dut_shared"),      │
│            MigrationSharedDB:    openDB(cfg, "mig_shared"),      │
│            DUTAutoTesterDB:      openDB(cfg, "dut_at"),          │
│            MigrationAutoTesterDB:openDB(cfg, "mig_at"),          │
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
│   MigrationTester    │   │  AutoTester DB            │
│   (Tester interface) │   │  PG_DB_AutoTester         │
│                      │   │  auto_test_runs           │
│  ┌────────────────┐  │   │  auto_test_results        │
│  │ MigrationSUT   │  │   │  auto_test_logs           │
│  │ State          │  │   └───────────────────────────┘
│  └────────────────┘  │
│  ┌────────────────┐  │
│  │ migration      │  │
│  │ CaseGenerator  │  │
│  └────────────────┘  │
│  ┌────────────────┐  │
│  │ sharedgoose    │  │
│  │ .Migrator      │  │
│  │ (DUT + Mig DB) │  │
│  └────────────────┘  │
└──────┬───────┬───────┘
       │       │
       ▼       ▼
┌──────────┐  ┌─────────────────────────────────────┐
│  DUT(s)  │  │  Migration DB(s)                    │
│          │  │                                     │
│ DDL only │  │  db_migrations (goose tracking)     │
│ testonly_│  │                                     │
│ * tables │  │  Optional multi-track:              │
│          │  │  Mig_Project │ Mig_Shared │ Mig_AT  │
│ Optional │  └─────────────────────────────────────┘
│ multi-   │
│ track:   │
│ DUT_Proj │
│ DUT_Shr  │
│ DUT_AT   │
└──────────┘
```

### 8.2 Internal Components

| Component | Type | Responsibility |
|---|---|---|
| `MigrationTester` | `struct` (implements `autotesters.Tester`) | Top-level tester; holds all state; implements all lifecycle methods |
| `MigrationSUTState` | `struct` | Ground-truth state: applied migrations, files in dir, schema tables, current version |
| `migrationCaseGenerator` | internal helper | Reads simulated state + `b.Rand()`; applies weighted distributions; builds `TestCase` structs |
| `migrationInput` | `struct` | Typed `TestCase.Input`: carries operation, parameters, track, and `PreState` snapshot |
| `resetToState(ctx, preState)` | method | Brings DUT to the specified pre-state (drop + re-apply) |
| `resetDUT(ctx)` | method | Full DUT reset: drop tracking table + all `testonly_` tables |
| `syncState(ctx)` | method | Re-queries DUT to refresh `MigrationSUTState` after each case |
| `observeSideEffects(ctx, result)` | method | Queries DUT after operation; appends observed side effect keys |
| `buildMigrator(allowOutOfOrder bool)` | method | Constructs a `sharedgoose.Migrator` bound to both DUT (schema) and Migration DB (tracking) |
| `buildMigrationPool(ctx)` | method | Pre-generates `MaxMigrationsInPool` synthetic SQL files in `testonly_` dir |
| `prepareMultiTrackDBs(ctx)` | method | Resets and initializes all three per-track DUT/Migration DB pairs |
| `cleanupMultiTrackDBs(ctx)` | method | Drops `testonly_` tables from per-track DUT DBs and `db_migrations` from per-track Migration DBs |

---

## 9. Configuration

### 9.1 Environment Variables

Configure all databases in your project's `mise.local.toml`:

```toml
PG_USER_NAME = "admin"
PG_PASSWORD = "<password>"
PG_HOST     = "127.0.0.1"
PG_PORT     = "5432"

# Application databases (production-equivalent; never used by the tester directly)
PG_DB_NAME        = "<project_db_name>"

# AutoTester DB — test result storage (framework-managed)
PG_DB_NAME_AUTOTESTER = "<project>_autotester"

# DUT — dedicated schema-change databases for the tester
PG_DB_NAME_DUT            = "<project>_dut"
# Optional: per-track DUTs for multi-track cases
PG_DB_NAME_DUT_PROJECT    = "<project>_dut_project"
PG_DB_NAME_DUT_SHARED     = "<project>_dut_shared"
PG_DB_NAME_DUT_AUTOTESTER = "<project>_dut_autotester"

# Migration DB — dedicated goose-tracking databases for the tester
PG_DB_NAME_MIGRATION            = "<project>_migration"
# Optional: per-track Migration DBs for multi-track cases
PG_DB_NAME_MIGRATION_PROJECT    = "<project>_migration_project"
PG_DB_NAME_MIGRATION_SHARED     = "<project>_migration_shared"
PG_DB_NAME_MIGRATION_AUTOTESTER = "<project>_migration_autotester"
```

### 9.2 RunConfig Integration

The Migration Tester respects all AutoTester `RunConfig` fields:

| Config Field | Usage |
|---|---|
| `Seed` | Random seed for reproducible dynamic case generation |
| `Tags` | Filter test cases by tags (e.g., `"migration,critical"`, `"edge-case"`, `"multi-track"`) |
| `TestIDs` | Run specific test case IDs (e.g., `"TC_2026022301,TC_DYN_0042"`) |
| `Parallel` | Enable parallel Tester execution (with other registered Testers) |
| `RetryCount` | Retry failed test cases |
| `CaseTimeout` | Per-test-case timeout (default: 30s; migration DDL on local DB should be much faster) |
| `StopOnFail` | Stop on first failure |
| `SkipCleanup` | Skip `Cleanup` — leaves DUT tables intact for post-mortem inspection |
| `Verbose` | Enable debug logging of DUT state before/after each case |

### 9.3 CLI Usage Examples

```bash
# Run all testers including MigrationTester
go run ./server/cmd/autotester/

# Run only MigrationTester
go run ./server/cmd/autotester/ --tester=tester_migration

# Run with a fixed seed for reproducible dynamic cases
go run ./server/cmd/autotester/ --tester=tester_migration --seed=12345

# Run only the static cases (all 29 TC_* IDs)
go run ./server/cmd/autotester/ --tester=tester_migration --type=integration --tags=migration

# Run only multi-track cases
go run ./server/cmd/autotester/ --tags=multi-track

# Debug a specific failing case with full DUT state preserved
go run ./server/cmd/autotester/ --test-id=TC_2026022324 --verbose --skip-cleanup

# Replay a failing dynamic run (seed taken from auto_test_runs)
go run ./server/cmd/autotester/ --tester=tester_migration --seed=<seed_from_db>
```

### 9.4 Registration

```go
// server/cmd/autotester/registry.go
autotesters.GlobalRegistry.Register("tester_migration", func() autotesters.Tester {
    return autotesters.NewMigrationTester(&autotesters.MigrationTesterConfig{
        DUTDB:           openDB(cfg, cfg.DUTDBName),
        MigrationDB:     openDB(cfg, cfg.MigrationDBName),
        DUTDBType:       "postgres",
        DUTDBName:       cfg.DUTDBName,
        MigrationDBName: cfg.MigrationDBName,
        MigrationsDir:   "testonly_migrations",
        // Multi-track optional — omit to skip multi-track cases
        DUTProjectDB:         openDB(cfg, cfg.DUTProjectDBName),
        MigrationProjectDB:   openDB(cfg, cfg.MigrationProjectDBName),
        DUTSharedDB:          openDB(cfg, cfg.DUTSharedDBName),
        MigrationSharedDB:    openDB(cfg, cfg.MigrationSharedDBName),
        DUTAutoTesterDB:      openDB(cfg, cfg.DUTAutoTesterDBName),
        MigrationAutoTesterDB:openDB(cfg, cfg.MigrationAutoTesterDBName),
    })
})
```

### 9.5 Database Provisioning

All databases must be pre-created before running the tester. The tester does **not** create or drop databases — only tables and the migration tracking table within them.

**Minimum setup (single-track only):**

```sql
-- Run as a superuser (e.g., postgres)
CREATE DATABASE <project>_dut;
CREATE DATABASE <project>_migration;
GRANT ALL PRIVILEGES ON DATABASE <project>_dut       TO <pg_user>;
GRANT ALL PRIVILEGES ON DATABASE <project>_migration TO <pg_user>;
```

**Full setup (including multi-track):**

```sql
-- DUT databases (schema changes land here)
CREATE DATABASE <project>_dut;
CREATE DATABASE <project>_dut_project;
CREATE DATABASE <project>_dut_shared;
CREATE DATABASE <project>_dut_autotester;

-- Migration databases (goose tracking tables land here)
CREATE DATABASE <project>_migration;
CREATE DATABASE <project>_migration_project;
CREATE DATABASE <project>_migration_shared;
CREATE DATABASE <project>_migration_autotester;

-- Grant access
GRANT ALL PRIVILEGES ON DATABASE <project>_dut                  TO <pg_user>;
GRANT ALL PRIVILEGES ON DATABASE <project>_dut_project          TO <pg_user>;
GRANT ALL PRIVILEGES ON DATABASE <project>_dut_shared           TO <pg_user>;
GRANT ALL PRIVILEGES ON DATABASE <project>_dut_autotester       TO <pg_user>;
GRANT ALL PRIVILEGES ON DATABASE <project>_migration            TO <pg_user>;
GRANT ALL PRIVILEGES ON DATABASE <project>_migration_project    TO <pg_user>;
GRANT ALL PRIVILEGES ON DATABASE <project>_migration_shared     TO <pg_user>;
GRANT ALL PRIVILEGES ON DATABASE <project>_migration_autotester TO <pg_user>;
```

**Required permissions per database:** `CREATE`, `DROP`, `SELECT`, `INSERT`, `DELETE` on tables within the database. No cross-database permissions are needed — each DB connection is isolated.

**AutoTester DB** (`PG_DB_NAME_AUTOTESTER`) is provisioned by the AutoTester framework on first run and does not need manual setup beyond `CREATE DATABASE`.

---

## 10. Implementation Plan

### Phase 1: Infrastructure Helpers

- [ ] Create `shared/go/api/autotesters/tester_migration.go` with `MigrationTester` skeleton, `MigrationTesterConfig`, and embedded `BaseTester`
- [ ] Define `MigrationSUTState`, `MigrationRecord`, `MigrationFile`, `migrationInput`, `MigrationOperation` types
- [ ] Implement `resetDUT(ctx)` — drop `db_migrations` and all `testonly_` tables from DUT
- [ ] Implement `syncState(ctx)` — query DUT to refresh `MigrationSUTState`
- [ ] Implement `resetToState(ctx, preState)` — repopulate dir and re-apply migrations
- [ ] Implement `observeSideEffects(ctx, result)` — detect schema changes and tracking table existence
- [ ] Implement migration file writer — write goose-formatted `.sql` files to `testonly_` dir
- [ ] Implement `buildMigrationPool(ctx)` — pre-generate `MaxMigrationsInPool` synthetic SQL files
- [ ] Implement `buildMigrator(allowOutOfOrder bool)` — construct `sharedgoose.Migrator`

### Phase 2: `Prepare` and `Cleanup`

- [ ] Implement `Prepare(ctx)` — full startup sequence: ping, validate dir, reset DUT, build pool, init state, build migrator, prepare multi-track DBs
- [ ] Implement `Cleanup(ctx)` — drop `testonly_` tables, drop tracking table, clear dir (single + multi-track)
- [ ] Integration tests for `Prepare` and `Cleanup` against a real PostgreSQL instance

### Phase 3: Static Test Cases — Single-Track (Categories A–F)

- [ ] Implement `GetTestCases()` returning all 26 single-track static cases (TC_2026022301–TC_2026022325 + TC_2026022329)
- [ ] Implement `RunTestCase` dispatcher (switch on `input.Operation`)
- [ ] Implement per-operation handlers: `runUp`, `runDown`, `runUpByOne`, `runUpTo`, `runDownTo`, `runStatus`, `runGetVersion`, `runHasPending`, `runCreateMigration`, `runCreateAndApply`
- [ ] Implement `CustomValidator` functions for semantic DB state comparison (query `db_migrations`, `information_schema`)
- [ ] Verify all 26 single-track static cases pass against a real PostgreSQL instance

### Phase 4: Static Test Cases — Multi-Track (Category G)

- [ ] Implement `prepareMultiTrackDBs(ctx)` and `cleanupMultiTrackDBs(ctx)`
- [ ] Implement Category G handlers: `runMultiTrackIndependent`, `runMultiTrackIsolation`, `runCrossTrackRollback`
- [ ] Add `TC_2026022326`, `TC_2026022327`, `TC_2026022328` to `GetTestCases()`
- [ ] Verify Category G cases pass against real PostgreSQL with three DUT DBs

### Phase 5: Dynamic Test Cases

- [ ] Implement `migrationCaseGenerator` with weighted distributions (Section 4.2) and state-dependent rules (Section 7.1), including multi-track generation
- [ ] Implement `GenerateTestCases(ctx)` calling the generator `NumDynamicCases` times
- [ ] Verify dynamic cases produce a coherent sequence and state tracker stays in sync
- [ ] Verify deterministic replay: fix seed, run twice, confirm identical case sequences

### Phase 6: Registration and Integration

- [ ] Register `"tester_migration"` in the application's `server/cmd/autotester/registry.go`
- [ ] Wire `MigrationTesterConfig` into the application's config/startup
- [ ] Run full AutoTester suite; confirm results appear in `auto_test_results` and `auto_test_runs`
- [ ] Run with `--skip-cleanup`; confirm DUT tables are left intact for inspection
- [ ] Confirm all cases complete within `RunConfig.RunTimeout` (default 30m)
- [ ] Add to CI/CD pipeline

---

## 11. File Structure

```
shared/go/
└── api/
    └── autotesters/
        ├── autotesters.go              # Tester interface, BaseTester (existing)
        ├── testcase.go                 # TestCase, ExpectedResult (existing)
        ├── testresult.go               # TestResult, LogEntry (existing)
        ├── testrun.go                  # TestRun, RunConfig (existing)
        ├── runner.go                   # TestRunner (existing)
        ├── registry.go                 # TesterRegistry (existing)
        ├── db.go                       # DB persistence (existing)
        ├── rand.go                     # Seeded randomness helpers (existing)
        ├── assert.go                   # Common assertion helpers (existing)
        │
        ├── tester_migration.go         # NEW: MigrationTester
        │                               #   MigrationTester struct + constructor
        │                               #   MigrationTesterConfig
        │                               #   MigrationSUTState, MigrationRecord, MigrationFile
        │                               #   migrationInput, MigrationOperation
        │                               #   Prepare, Cleanup
        │                               #   GenerateTestCases, GetTestCases
        │                               #   RunTestCase + per-operation handlers
        │                               #   migrationCaseGenerator
        │                               #   resetDUT, resetToState, syncState
        │                               #   observeSideEffects, buildMigrator
        │                               #   buildMigrationPool
        │                               #   prepareMultiTrackDBs, cleanupMultiTrackDBs
        └── tester_migration_test.go    # NEW: Integration tests for MigrationTester
```

Registration in the consuming application:

```
myapp/
└── server/
    └── cmd/
        └── autotester/
            ├── main.go       # existing — no changes needed
            └── registry.go   # ADD: Register("tester_migration", NewMigrationTester(...))
```

---

## 12. Open Items

| # | Item | Notes |
|---|---|---|
| 1 | **DUT DB connection ownership** | `MigrationTesterConfig.DUTDB` accepts a pre-opened `*sql.DB`. Decide whether the tester should accept a DSN string instead and open/close the connection itself in `Prepare`/`Cleanup`. |
| 2 | **MySQL support** | The goose wrapper supports MySQL. Decide whether `MigrationTester` tests both dialects or PostgreSQL only. If both, `MigrationTesterConfig.DUTDBType` controls which dialect the migrator and `information_schema` queries use. |
| 3 | **Concurrency stress cases** | `CreateAndApply` is documented as not concurrent-safe. A separate test category invoking it from two goroutines simultaneously would verify the documented warning is accurate. This requires a separate `ConcurrentMigrationTester` or an explicit concurrency section within this tester. |
| 4 | **Version table corruption tests** | Manually insert/delete rows in `db_migrations` via `dutDB` to test migrator resilience to a corrupted tracking table. Would be a new static case category. |
| 5 | **Embedded FS path** | Production migrators use `embed.FS`; this tester uses `os.DirFS`. A static case using a small pre-compiled embedded migration set would cover that code path. |
| 6 | **`resetToState` performance** | For large `PreState.Applied` slices, `resetToState` calls `UpByOne` repeatedly. If this becomes slow, batch-apply via `Up` with a bounded `UpTo` instead. |
| 7 | **SQL template library** | Whether to maintain a reusable library of pre-defined migration SQL templates (create table, add column, create index, etc.) for use in dynamic case generation. See Appendix A for initial candidates. |
| 8 | **Data migrations** | Current scope is DDL-only (schema changes). Whether to extend to data migrations (INSERT/UPDATE/DELETE inside migration files) and how to verify correctness of those operations. |
| 9 | **Multi-track parallel apply** | Whether to add a static case that applies all three tracks concurrently (one goroutine per track) to test isolation under concurrent load. |

---

## Appendix A: SQL Templates

The following templates are used by the migration pool builder and by dynamic case generators when creating synthetic `testonly_` migration files.

```go
var sqlTemplates = map[string]string{
    "create_table": `CREATE TABLE IF NOT EXISTS {{table_name}} (
        id         BIGSERIAL PRIMARY KEY,
        name       VARCHAR(255) NOT NULL,
        created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
    )`,

    "drop_table": `DROP TABLE IF EXISTS {{table_name}}`,

    "add_column": `ALTER TABLE {{table_name}}
        ADD COLUMN IF NOT EXISTS {{column_name}} {{column_type}}`,

    "drop_column": `ALTER TABLE {{table_name}}
        DROP COLUMN IF EXISTS {{column_name}}`,

    "create_index": `CREATE INDEX IF NOT EXISTS
        idx_{{table_name}}_{{column_name}}
        ON {{table_name}}({{column_name}})`,

    "drop_index": `DROP INDEX IF EXISTS
        idx_{{table_name}}_{{column_name}}`,

    "create_index_concurrent": `CREATE INDEX CONCURRENTLY IF NOT EXISTS
        idx_{{table_name}}_{{column_name}}
        ON {{table_name}}({{column_name}})`,
}

// invalidSQLTemplates are used in negative test cases (10% distribution)
var invalidSQLTemplates = []string{
    `CREATE TABEL {{table_name}} (id BIGSERIAL)`,         // typo: TABEL
    `ALTER TABLE {{table_name}} ADD COLUM name TEXT`,     // typo: COLUM
    `this is not valid sql at all`,
    `SELECT * FROM nonexistent_table_xyz_abc`,
}
```

---

## Appendix B: Example Test Run

```bash
# Run MigrationTester with verbose logging
$ go run ./server/cmd/autotester/ --tester=tester_migration --verbose --seed=12345

AutoTester run started
  run_id: a3f8d012-...
  seed:   12345
  env:    local

Running MigrationTester...
  Prepare: dropped 0 testonly_ tables, cleared testonly_migrations/, built 20-file pool
  Generating 80 dynamic cases (seed=12345)...

Static cases (29):
  [PASS] TC_2026022301  Apply all migrations from empty DB                      (42ms)
  [PASS] TC_2026022302  Up is no-op when already current                        (8ms)
  [PASS] TC_2026022303  Apply migrations one by one with UpByOne                (87ms)
  [PASS] TC_2026022304  UpByOne returns ErrNoNextVersion when all applied       (4ms)
  [PASS] TC_2026022305  UpTo applies migrations up to a target version          (31ms)
  [PASS] TC_2026022306  UpTo returns ErrVersionNotFound for nonexistent version (3ms)
  [PASS] TC_2026022307  Roll back one migration (has Down SQL)                  (29ms)
  [PASS] TC_2026022308  Down returns error when nothing is applied              (3ms)
  [PASS] TC_2026022309  Down returns error for migration with no Down SQL       (18ms)
  [PASS] TC_2026022310  DownTo rolls back to a target version                   (35ms)
  [PASS] TC_2026022311  DownTo(0) rolls back all migrations                     (67ms)
  [PASS] TC_2026022312  Status returns correct applied/pending counts           (6ms)
  [PASS] TC_2026022313  GetVersion returns 0 when nothing is applied            (2ms)
  [PASS] TC_2026022314  HasPending returns true when pending migrations exist   (2ms)
  [PASS] TC_2026022315  HasPending returns false when fully applied             (3ms)
  [PASS] TC_2026022316  CreateAndApply writes file and applies migration        (52ms)
  [PASS] TC_2026022317  CreateAndApply with empty downSQL succeeds; Down fails  (44ms)
  [PASS] TC_2026022318  CreateAndApply with invalid SQL returns error           (5ms)
  [PASS] TC_2026022319  CreateAndApply filename follows YYYYMMDDHHMMSS_slug    (49ms)
  [PASS] TC_2026022329  CreateMigration writes file without applying it        (12ms)
  [PASS] TC_2026022320  Out-of-order with AllowOutOfOrder=true is applied       (38ms)
  [PASS] TC_2026022321  Out-of-order with AllowOutOfOrder=false is rejected     (6ms)
  [PASS] TC_2026022322  Up with empty migrations directory is a no-op          (2ms)
  [PASS] TC_2026022323  Tracking table is auto-created on first Up             (14ms)
  [FAIL] TC_2026022324  Partial failure: PartialError lists what succeeded      (12ms)
         "expected PartialError.Applied to have length 1, got 0"
  [PASS] TC_2026022325  Migration with NO TRANSACTION applies successfully      (41ms)
  [PASS] TC_2026022326  Apply all three tracks independently                    (118ms)
  [PASS] TC_2026022327  Track isolation: mixed state per track                  (94ms)
  [PASS] TC_2026022328  Cross-track rollback: only project rolled back          (77ms)

Dynamic cases (80): ... 77 pass, 2 fail, 1 skip

AutoTester Run Complete
  Run ID   : a3f8d012-...
  Seed     : 12345
  Env      : local
  Duration : 3m 47s
  Total    : 109
  Passed   : 106 (97.2%)
  Failed   : 3   (2.8%)
  Skipped  : 0   (0.0%)
  Errored  : 0   (0.0%)

FAILURES:
  [fail]  TC_2026022324   (12ms)  "expected PartialError.Applied to have length 1, got 0"
  [fail]  TC_DYN_0023     (8ms)   "expected error containing 'version not found', got: <nil>"
  [fail]  TC_DYN_0061     (3ms)   "expected HasPending=true, got false"
```
