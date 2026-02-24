# Plan: Migration Tester

**Author:** Claude
**Date:** 2026-02-23
**Status:** Draft
**References:**
- AutoTester framework: [`Testbot/auto-tester-v2.md`](../../../../Testbot/auto-tester-v2.md)
- Database migration: [`shared/Documents/dev/goose-v1.md`](../dev/goose-v1.md)

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
9. [Implementation Plan](#9-implementation-plan)
10. [File Structure](#10-file-structure)
11. [Open Items](#11-open-items)

---

## 1. Overview

This document describes the plan for developing a **Migration Tester** — a `Tester` implementation within the **AutoTester framework** (`shared/go/api/autotesters`, see `auto-tester-v2.md`) whose System Under Test (SUT) is the **goose-based database migration system** (`shared/go/api/goose`).

The `MigrationTester` implements the `autotesters.Tester` interface and is registered with the `TesterRegistry`. The AutoTester `TestRunner` drives its full lifecycle: `Prepare` → case supply → `RunTestCase` per case → `Cleanup`. Results are persisted to `PG_DB_AutoTester` by the framework's built-in database persistence layer — the tester does not implement its own logging or reporting.

### 1.1 Goals

- Automatically and randomly test the migration system's correctness
- Verify that applying and rolling back migrations leaves the database in the expected state
- Detect regressions in version tracking, ordering, partial-failure handling, and rollback correctness
- Exercise the programmatic Go API: `Up`, `Down`, `UpByOne`, `UpTo`, `DownTo`, `Status`, `GetVersion`, `HasPending`, `CreateAndApply`

### 1.2 Non-Goals

- This tester does **not** test application business logic — only the migration infrastructure
- This tester does **not** test the upstream `pressly/goose` library internals
- This tester does **not** validate production migration files — it generates synthetic ones

### 1.3 Why This Matters

Migration bugs are high-impact: a failed `Up` or incorrect `Down` can corrupt a production database. Automated testing with random migration sequences catches ordering bugs, partial-failure handling, and rollback correctness that are hard to exercise manually.

---

## 2. SUT Definition

**SUT:** The goose migration wrapper in `shared/go/api/goose/goose.go`, accessed via `sharedgoose.NewWithDB(dutDB, ...)`.

The `MigrationTester` does **not** use the production migrators (`ProjectMigrator`, `SharedMigrator`, `AutoTesterMigrator`). It creates its own `Migrator` instance pointing at the DB-Under-Test (DUT).

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
| Create + apply | `CreateAndApply(ctx, desc, upSQL, downSQL)` | Write a new SQL file and immediately apply it |

### 2.2 Database Setup Requirements

This tester requires two pre-existing databases, both separate from production:

1. **DB-Under-Test (DUT)** — the dedicated test database where the goose migrator is exercised. Schema changes (e.g., `CREATE TABLE`) and the goose tracking table (`db_migrations`) live here. Database migration will use a different DB, which should be different from DUT.
2. **AutoTester DB** (`PG_DB_AutoTester`) — where test results (`auto_test_runs`, `auto_test_results`, `auto_test_logs`) are stored. Managed entirely by the AutoTester framework, not by this tester.

**Both databases must already exist before the tester runs.** The tester does not create or drop databases.

Configure both in the project's `mise.local.toml`:

```toml
PG_DB_NAME_AUTOTESTER = "<project>_autotester"
# DUT connection is passed directly via MigrationTesterConfig
```

### 2.3 `Prepare` Invariants

Per the AutoTester `Prepare` contract, the following are enforced before any test case runs:

1. The migrations directory name **must** start with `testonly_` — this prevents accidental operations on production migration directories.
2. Upon `Prepare`, all tables whose names start with `testonly_` are dropped from DUT, so each run starts from a clean schema.
3. Upon `Prepare`, the goose tracking table (`db_migrations`) is dropped from DUT, so version tracking resets cleanly.
4. Upon `Prepare`, the `testonly_` migrations directory is emptied (all `.sql` files from previous runs deleted).

---

## 3. Tester Identity

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

### 3.1 Constructor Configuration

```go
type MigrationTesterConfig struct {
    // DUT: pre-existing test database; NOT production; NOT the AutoTester DB.
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

## 4. SUT Parameters

These are the dimensions the tester randomizes when generating dynamic test cases via `GenerateTestCases`.

### 4.1 Parameter Table

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

### 4.2 Weighted Distributions (Closeness Principle)

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

## 5. Test Cases

The tester supplies two pools of test cases following the AutoTester convention:

- **Static cases** (`GetTestCases`): Hard-coded, deterministic. Cover known invariants, edge cases, and regression scenarios that must pass on every run.
- **Dynamic cases** (`GenerateTestCases`): Randomly generated using `b.Rand()`. Cover the combinatorial parameter space defined in Section 4.

### 5.1 Static Test Cases (`GetTestCases`)

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

**State isolation between static cases:** Each static case specifies its required pre-state in `TestCase.Input` (see Section 5.3). `RunTestCase` calls `resetToState(ctx, preState)` at the start of every case, so cases are independent of each other's side effects regardless of execution order.

### 5.2 Dynamic Test Cases (`GenerateTestCases`)

Dynamic cases are generated using `b.Rand()` (the seeded `*rand.Rand` set by the runner). The generator reads the current `MigrationSUTState`, applies weighted distributions from Section 4.2, enforces state-dependent constraints from Section 7.1, computes the expected outcome, and constructs a `TestCase`.

```
Dynamic case ID format: TC_DYN_NNNN  (NNNN = 0-padded sequence within the run)
Examples: TC_DYN_0001, TC_DYN_0042
```

Dynamic cases use `Priority: PriorityLow` by default. Cases that exercise an empty migrations directory are tagged `["edge-case"]`.

Because the seed is stored in `auto_test_runs.seed` by the runner, any failing dynamic case can be replayed exactly by re-running with `--seed=<seed>`.

### 5.3 `TestCase.Input`: The `migrationInput` Struct

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
| `migration_file_written` | A new `.sql` file was written to the `testonly_` dir (by `CreateAndApply`) |

---

## 6. Tester Lifecycle

This maps directly to the AutoTester `Tester` interface lifecycle as driven by `TestRunner`.

### 6.1 `Prepare(ctx) error`

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

Returns the 25 static cases listed in Section 5.1. The runner merges these with dynamic cases; together they form the full test suite for the run.

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

### 6.5 `Cleanup(ctx) error`

1. Drop all `testonly_` tables from DUT (same as `Prepare` step 4)
2. Drop `db_migrations` from DUT
3. Delete all `.sql` files from the `testonly_` migrations directory

If `--skip-cleanup` is passed to the runner, `Cleanup` is not called, leaving DUT state intact for post-mortem inspection.

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
│            DUTDB:     openTestDB(cfg), // pre-existing test DB   │
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
│  │ (DUT-bound)    │  │
│  └────────────────┘  │
└──────────┬───────────┘
           │
           ▼
┌──────────────────────┐
│  DUT                 │
│  (dedicated test DB) │
│  testonly_* tables   │
│  db_migrations       │
└──────────────────────┘
```

### 8.2 Internal Components

| Component | Type | Responsibility |
|---|---|---|
| `MigrationTester` | `struct` (implements `autotesters.Tester`) | Top-level tester; holds all state; implements all lifecycle methods |
| `MigrationSUTState` | `struct` | Ground-truth state: applied migrations, files in dir, schema tables, current version |
| `migrationCaseGenerator` | internal helper | Reads simulated state + `b.Rand()`; applies weighted distributions; builds `TestCase` structs |
| `migrationInput` | `struct` | Typed `TestCase.Input`: carries operation, parameters, and `PreState` snapshot |
| `resetToState(ctx, preState)` | method | Brings DUT to the specified pre-state (drop + re-apply) |
| `resetDUT(ctx)` | method | Full DUT reset: drop tracking table + all `testonly_` tables |
| `syncState(ctx)` | method | Re-queries DUT to refresh `MigrationSUTState` after each case |
| `observeSideEffects(ctx, result)` | method | Queries DUT after operation; appends observed side effect keys |
| `buildMigrator(allowOutOfOrder bool)` | method | Constructs a `sharedgoose.Migrator` with the given config |

---

## 9. Implementation Plan

### Phase 1: Infrastructure Helpers

- [ ] Create `shared/go/api/autotesters/tester_migration.go` with `MigrationTester` skeleton, `MigrationTesterConfig`, and embedded `BaseTester`
- [ ] Define `MigrationSUTState`, `MigrationRecord`, `MigrationFile`, `migrationInput`, `MigrationOperation` types
- [ ] Implement `resetDUT(ctx)` — drop `db_migrations` and all `testonly_` tables from DUT
- [ ] Implement `syncState(ctx)` — query DUT to refresh `MigrationSUTState`
- [ ] Implement `resetToState(ctx, preState)` — repopulate dir and re-apply migrations
- [ ] Implement `observeSideEffects(ctx, result)` — detect schema changes and tracking table existence
- [ ] Implement migration file writer — write goose-formatted `.sql` files to `testonly_` dir

### Phase 2: `Prepare` and `Cleanup`

- [ ] Implement `Prepare(ctx)` — full startup sequence: ping, validate dir, reset DUT, build pool, init state, build migrator
- [ ] Implement `Cleanup(ctx)` — drop `testonly_` tables, drop tracking table, clear dir
- [ ] Integration tests for `Prepare` and `Cleanup` against a real PostgreSQL instance

### Phase 3: Static Test Cases

- [ ] Implement `GetTestCases()` returning all 25 cases from Section 5.1 with correct `migrationInput` and `ExpectedResult`
- [ ] Implement `RunTestCase` dispatcher (switch on `input.Operation`)
- [ ] Implement per-operation handlers: `runUp`, `runDown`, `runUpByOne`, `runUpTo`, `runDownTo`, `runStatus`, `runGetVersion`, `runHasPending`, `runCreateAndApply`
- [ ] Implement `CustomValidator` functions for semantic DB state comparison (query `db_migrations`, `information_schema`)
- [ ] Verify all 25 static cases pass against a real PostgreSQL instance

### Phase 4: Dynamic Test Cases

- [ ] Implement `migrationCaseGenerator` with weighted distributions (Section 4.2) and state-dependent rules (Section 7.1)
- [ ] Implement `GenerateTestCases(ctx)` calling the generator `NumDynamicCases` times
- [ ] Verify dynamic cases produce a coherent sequence and state tracker stays in sync
- [ ] Verify deterministic replay: fix seed, run twice, confirm identical case sequences

### Phase 5: Registration and Integration

- [ ] Register `"tester_migration"` in the application's `server/cmd/autotester/registry.go`
- [ ] Wire `MigrationTesterConfig` into the application's config/startup
- [ ] Run full AutoTester suite; confirm results appear in `auto_test_results` and `auto_test_runs`
- [ ] Run with `--skip-cleanup`; confirm DUT tables are left intact for inspection
- [ ] Confirm all cases complete within `RunConfig.RunTimeout` (default 30m)

---

## 10. File Structure

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
        │                               #   Prepare, Cleanup, GenerateTestCases, GetTestCases
        │                               #   RunTestCase + per-operation handlers
        │                               #   migrationCaseGenerator
        │                               #   resetDUT, resetToState, syncState, observeSideEffects
        │                               #   buildMigrator
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

## 11. Open Items

| # | Item | Notes |
|---|---|---|
| 1 | **DUT DB connection ownership** | `MigrationTesterConfig.DUTDB` accepts a pre-opened `*sql.DB`. Decide whether the tester should accept a DSN string instead and open/close the connection itself in `Prepare`/`Cleanup`. |
| 2 | **MySQL support** | The goose wrapper supports MySQL. Decide whether `MigrationTester` tests both dialects or PostgreSQL only. If both, `MigrationTesterConfig.DUTDBType` controls which dialect the migrator and `information_schema` queries use. |
| 3 | **Concurrency stress cases** | `CreateAndApply` is documented as not concurrent-safe. A separate test category invoking it from two goroutines simultaneously would verify the documented warning is accurate. This requires a separate `ConcurrentMigrationTester` or an explicit concurrency section within this tester. |
| 4 | **`CreateMigration` (no apply)** | `CreateMigration` writes a file without applying it. This operation is not yet covered by the current test case categories and should be added as a category G. |
| 5 | **Version table corruption tests** | Manually insert/delete rows in `db_migrations` via `dutDB` to test migrator resilience to a corrupted tracking table. This would be a new static case category. |
| 6 | **Embedded FS path** | Production migrators use `embed.FS`; this tester uses `os.DirFS`. A static case using a small pre-compiled embedded migration set would cover that code path. |
| 7 | **`resetToState` performance** | For large `PreState.Applied` slices, `resetToState` calls `UpByOne` repeatedly. If this becomes slow, batch-apply via `Up` with a bounded `UpTo` instead. |
