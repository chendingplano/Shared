# Database Migration Tester - Implementation Document V1

**Version:** 1.0
**Date:** 2026-02-24
**Status:** Implementation Complete
**Based On:** `tester-migration-qwen-v4.md` (V0.6)

---

## Table of Contents

1. [Overview](#1-overview)
2. [Implementation Summary](#2-implementation-summary)
3. [File Structure](#3-file-structure)
4. [Type Definitions](#4-type-definitions)
5. [Core Implementation](#5-core-implementation)
6. [Test Cases](#6-test-cases)
7. [Integration Guide](#7-integration-guide)
8. [Usage Examples](#8-usage-examples)
9. [Deviation from Specification](#9-deviation-from-specification)
10. [Testing and Verification](#10-testing-and-verification)

---

## 1. Overview

This document describes the implementation of the **Migration Tester** — a Testbot implementation within the AutoTester framework whose System Under Test (SUT) is the goose-based database migration system (`shared/go/api/goose`).

The implementation follows the specification in `tester-migration-qwen-v4.md` and implements the `autotesters.Tester` interface.

### 1.1 Implementation Location

```
shared/go/api/testers/tester-migration/
```

### 1.2 Key Design Decisions

- **Package Name:** `tester_migration` (matches tester identity name)
- **File Organization:** Split into 5 files by concern (types, main, state, handlers, cases)
- **State Isolation:** Each test case resets DUT to required pre-state before execution
- **Safety:** Enforces `testonly_` prefix for database names and migration directories

---

## 2. Implementation Summary

### 2.1 Completed Components

| Component | Status | File |
|-----------|--------|------|
| `MigrationTester` struct | ✅ Complete | `tester_migration.go` |
| `Prepare()` lifecycle | ✅ Complete | `tester_migration.go` |
| `Cleanup()` lifecycle | ✅ Complete | `tester_migration.go` |
| `RunTestCase()` execution | ✅ Complete | `tester_migration.go` |
| State tracking (`syncState`) | ✅ Complete | `tester_migration_state.go` |
| State reset (`resetToState`) | ✅ Complete | `tester_migration_state.go` |
| Operation handlers (10 ops) | ✅ Complete | `tester_migration_handlers.go` |
| Static test cases (19 cases) | ✅ Complete | `tester_migration_cases.go` |
| Dynamic case generation | ✅ Partial | `tester_migration_cases.go` |

### 2.2 Interface Compliance

The `MigrationTester` implements all required `autotesters.Tester` interface methods:

```go
type Tester interface {
    // Identity / metadata
    Name() string        // ✅ Returns "tester_migration"
    Description() string // ✅ Returns human-readable summary
    Purpose() string     // ✅ Returns "regression"
    Type() string        // ✅ Returns "integration"
    Tags() []string      // ✅ Returns ["database", "migration", "goose", "shared"]

    // Lifecycle
    Prepare(ctx context.Context) error  // ✅ Implemented
    Cleanup(ctx context.Context) error  // ✅ Implemented

    // Test case supply
    GenerateTestCases(ctx context.Context) ([]TestCase, error) // ✅ Implemented
    GetTestCases() []TestCase                                   // ✅ Implemented

    // Execution
    RunTestCase(ctx context.Context, tc TestCase) TestResult // ✅ Implemented

    // Statistics (via BaseTester embed)
    IncrementSuccess() // ✅ Inherited
    IncrementFail()    // ✅ Inherited
    IncrementError()   // ✅ Inherited
    SetEndTime(t time.Time) // ✅ Inherited
}
```

---

## 3. File Structure

```
shared/go/api/testers/tester-migration/
├── tester_migration.go           # Main MigrationTester struct and lifecycle
├── tester_migration_types.go     # Type definitions (config, state, input)
├── tester_migration_state.go     # State tracking (syncState, resetToState)
├── tester_migration_handlers.go  # Per-operation handlers (Up, Down, etc.)
└── tester_migration_cases.go     # Static and dynamic test cases
```

### 3.1 File Responsibilities

| File | Lines | Responsibility |
|------|-------|----------------|
| `tester_migration_types.go` | ~180 | Type definitions, config validation, constants |
| `tester_migration.go` | ~395 | Main struct, lifecycle methods, test execution |
| `tester_migration_state.go` | ~280 | State synchronization, reset utilities |
| `tester_migration_handlers.go` | ~150 | Operation-specific execution logic |
| `tester_migration_cases.go` | ~400 | Static cases, dynamic generation |

---

## 4. Type Definitions

### 4.1 MigrationTesterConfig

```go
type MigrationTesterConfig struct {
    DUTDB               *sql.DB  // Isolated test database (MUST start with "testonly_")
    DUTDBType           string   // "postgres" or "mysql" (default: "postgres")
    DUTDBName           string   // Database name for logging (MUST start with "testonly_")
    MigrationsDir       string   // Directory for migration files (MUST start with "testonly_")
    TableName           string   // Goose version-tracking table (default: "db_migrations")
    NumDynamicCases     int      // Number of dynamic test cases (default: 80)
    MaxMigrationsInPool int      // Size of migrations pool (default: 20)
}
```

**Validation Rules:**
- `DUTDBName` must start with `testonly_` (validated in `Prepare()`)
- `MigrationsDir` must start with `testonly_` (validated in `Prepare()`)
- `DUTDB` must not be nil

### 4.2 MigrationOperation

```go
type MigrationOperation string

const (
    OpUp            MigrationOperation = "Up"
    OpUpByOne       MigrationOperation = "UpByOne"
    OpUpTo          MigrationOperation = "UpTo"
    OpDown          MigrationOperation = "Down"
    OpDownTo        MigrationOperation = "DownTo"
    OpStatus        MigrationOperation = "Status"
    OpGetVersion    MigrationOperation = "GetVersion"
    OpHasPending    MigrationOperation = "HasPending"
    OpCreateAndApply MigrationOperation = "CreateAndApply"
    OpListSources   MigrationOperation = "ListSources"
)
```

### 4.3 MigrationSUTState

```go
type MigrationSUTState struct {
    Applied        []MigrationRecord   // Migrations applied to DUT
    FilesInDir     []MigrationFile     // All .sql files in migrations directory
    Tables         map[string]bool     // Tables in DUT schema (testonly_ only)
    CurrentVersion int64               // Highest applied version; 0 if nothing applied
}
```

### 4.4 migrationInput

```go
type migrationInput struct {
    Operation       MigrationOperation  // Operation to invoke
    TargetVersion   int64               // For UpTo / DownTo
    UpSQL           string              // For CreateAndApply
    DownSQL         string              // For CreateAndApply; "" = no down SQL
    Description     string              // For CreateAndApply
    AllowOutOfOrder bool                // Migrator Config.AllowOutOfOrder
    PreState        MigrationSUTState   // Required DUT state before execution
}
```

---

## 5. Core Implementation

### 5.1 MigrationTester Struct

```go
type MigrationTester struct {
    autotesters.BaseTester      // Embed for default implementations

    cfg *MigrationTesterConfig  // Configuration

    dutDB *sql.DB               // Runtime database connection

    testMigrationsDir string    // Migration directory path

    state MigrationSUTState     // Current state tracking
}
```

### 5.2 Constructor

```go
func NewMigrationTester(cfg *MigrationTesterConfig) *MigrationTester
```

**Behavior:**
1. Applies default values to configuration
2. Creates `BaseTester` with identity metadata:
   - Name: `"tester_migration"`
   - Description: `"Tests the goose database migration system..."`
   - Purpose: `"regression"`
   - Type: `"integration"`
   - Tags: `["database", "migration", "goose", "shared"]`

### 5.3 Prepare Phase

```go
func (t *MigrationTester) Prepare(ctx context.Context) error
```

**Steps:**
1. Verify DUT is reachable via `PingContext()`
2. Validate DUT name starts with `testonly_`
3. Validate migrations directory starts with `testonly_`
4. Create migrations directory if needed
5. Drop goose tracking table from DUT
6. Drop all `testonly_` tables from DUT
7. Clear migrations directory (delete `.sql` files)
8. Build migrations pool (pre-generate synthetic migrations)
9. Initialize state via `syncState()`

**Error Handling:** Returns descriptive errors with MID codes for tracing

### 5.4 Cleanup Phase

```go
func (t *MigrationTester) Cleanup(ctx context.Context) error
```

**Steps:**
1. Drop all `testonly_` tables from DUT
2. Drop `db_migrations` tracking table
3. Delete all `.sql` files from migrations directory

**Note:** Cleanup is skipped if `--skip-cleanup` flag is passed to runner

### 5.5 RunTestCase Execution

```go
func (t *MigrationTester) RunTestCase(ctx context.Context, tc autotesters.TestCase) autotesters.TestResult
```

**Execution Flow:**
1. **Panic Recovery:** Deferred function catches panics and sets error status
2. **Input Validation:** Cast `TestCase.Input` to `migrationInput`
3. **State Reset:** Call `resetToState(ctx, input.PreState)` to isolate test case
4. **Migrator Build:** Create migrator with case-specific `AllowOutOfOrder` setting
5. **Operation Dispatch:** Route to appropriate handler based on `Operation`
6. **Side Effect Observation:** Inspect DUT for observed side effects
7. **State Sync:** Update internal state from DUT ground truth
8. **Result Finalization:** Set end time and duration

### 5.6 Operation Dispatch

```go
func (t *MigrationTester) dispatch(ctx context.Context, input migrationInput, 
                                     migrator *sharedgoose.Migrator, 
                                     result *autotesters.TestResult)
```

**Routing Table:**

| Operation | Handler |
|-----------|---------|
| `OpUp` | `handleUp()` |
| `OpUpByOne` | `handleUpByOne()` |
| `OpUpTo` | `handleUpTo()` |
| `OpDown` | `handleDown()` |
| `OpDownTo` | `handleDownTo()` |
| `OpStatus` | `handleStatus()` |
| `OpGetVersion` | `handleGetVersion()` |
| `OpHasPending` | `handleHasPending()` |
| `OpCreateAndApply` | `handleCreateAndApply()` |
| `OpListSources` | `handleListSources()` |

### 5.7 State Tracking

#### syncState

```go
func (t *MigrationTester) syncState(ctx context.Context) error
```

**Queries DUT for:**
1. Current version from tracking table (`MAX(version_id)`)
2. Applied migrations from tracking table
3. Migration files in directory (scan `.sql` files)
4. Tables in DUT schema (query `information_schema` for `testonly_%`)

#### resetToState

```go
func (t *MigrationTester) resetToState(ctx context.Context, preState MigrationSUTState) error
```

**Steps:**
1. Call `resetDUT()` - drop tracking table + all `testonly_` tables
2. Clear migrations directory
3. Write all files from `preState.FilesInDir`
4. Rebuild migrator
5. Apply migrations from `preState.Applied` using `UpByOne` repeatedly
6. Verify state matches via `syncState()`

---

## 6. Test Cases

### 6.1 Static Test Cases (GetTestCases)

**Total:** 19 static test cases

| ID | Name | Category | Priority |
|----|------|----------|----------|
| `TC_2026022301` | Apply all migrations from empty DB | Apply | Critical |
| `TC_2026022302` | Up is no-op when already current | Apply | High |
| `TC_2026022303` | Apply migrations one by one with UpByOne | Apply | High |
| `TC_2026022304` | UpByOne returns error when all applied | Apply | High |
| `TC_2026022305` | UpTo applies migrations up to target version | Apply | High |
| `TC_2026022306` | UpTo returns error for nonexistent version | Apply | Medium |
| `TC_2026022307` | Roll back one migration (has Down SQL) | Rollback | Critical |
| `TC_2026022308` | Down returns error when nothing applied | Rollback | High |
| `TC_2026022309` | DownTo rolls back to target version | Rollback | High |
| `TC_2026022310` | DownTo(0) rolls back all migrations | Rollback | High |
| `TC_2026022311` | GetVersion returns 0 when nothing applied | Status | Medium |
| `TC_2026022312` | HasPending returns true when pending exist | Status | Medium |
| `TC_2026022313` | HasPending returns false when fully applied | Status | Medium |
| `TC_2026022314` | Status returns correct applied/pending counts | Status | Medium |
| `TC_2026022315` | CreateAndApply writes file and applies | Create | High |
| `TC_2026022316` | CreateAndApply with empty downSQL succeeds | Create | High |
| `TC_2026022317` | CreateAndApply with invalid SQL returns error | Create | High |
| `TC_2026022318` | Tracking table is auto-created on first Up | Edge | Critical |
| `TC_2026022319` | ListSources returns migration files | Status | Medium |

### 6.2 Dynamic Test Cases (GenerateTestCases)

**Generation Algorithm:**
1. Use seeded RNG from `BaseTester`
2. Generate `NumDynamicCases` test cases (default: 80)
3. Select operation based on weighted distribution
4. Generate random pre-state
5. Build test case with migration input

**Operation Weights:**
- `Up`: 30%
- `Down`: 20%
- `UpByOne`: 15%
- `UpTo`: 10%
- `DownTo`: 10%
- `Status`/`GetVersion`/`HasPending`: 15%
- `CreateAndApply`: 0% (not implemented in dynamic generation)

**Case ID Format:** `TC_DYN_NNNN` (e.g., `TC_DYN_0001`)

### 6.3 Expected Results

Each test case specifies `ExpectedResult`:

```go
type ExpectedResult struct {
    Success         bool           // true = no error expected
    ExpectedError   string         // Substring in error message
    ExpectedValue   interface{}    // For inspection operations
    MaxDuration     time.Duration  // Timeout (default: 500ms)
    SideEffects     []string       // Required side effect keys
    CustomValidator func(...)      // Custom validation logic
}
```

**Side Effect Keys:**
- `tracking_table_created` - `db_migrations` table created
- `schema_table_applied` - A `testonly_` table was created
- `schema_table_dropped` - A `testonly_` table was dropped
- `migration_file_written` - A new `.sql` file was written

---

## 7. Integration Guide

### 7.1 Registration

Register the Migration Tester in your application's autotester registry:

```go
// server/cmd/autotester/registry.go
func registerAll(cfg *Config) {
    // DUT: Separate test database, NEVER production
    dutDB := openTestDB(cfg.DUTDSN)

    autotesters.GlobalRegistry.Register("tester_migration", func() autotesters.Tester {
        return autotesters.NewMigrationTester(&MigrationTesterConfig{
            DUTDB:               dutDB,
            DUTDBType:           "postgres",
            DUTDBName:           cfg.DUTDBName,  // Must start with "testonly_"
            MigrationsDir:       "testonly_migrations",
            TableName:           "db_migrations",
            NumDynamicCases:     80,
            MaxMigrationsInPool: 20,
        })
    })
}
```

### 7.2 Database Setup

The Migration Tester requires a dedicated test database:

```toml
# mise.local.toml

# DUT (DB-Under-Test) for MigrationTester
# CRITICAL: Name MUST start with "testonly_" for safety
DUT_DB_NAME = "testonly_myapp_dut"
DUT_DSN = "postgres://admin:password@127.0.0.1:5432/testonly_myapp_dut?sslmode=disable"
```

### 7.3 Table Creation

AutoTester tables must be created before running:

```go
import "github.com/chendingplano/shared/go/api/autotesters"

// Create tables at startup
autotesters.CreateAutoTestTables(logger, db, dbType)
```

Tables created:
- `auto_test_runs` - Test run metadata
- `auto_test_results` - Individual test case results
- `auto_test_logs` - Structured log entries

---

## 8. Usage Examples

### 8.1 Run All Testers

```bash
go run ./server/cmd/autotester/
```

### 8.2 Run Only Migration Tester

```bash
go run ./server/cmd/autotester/ --tester=tester_migration
```

### 8.3 Run with Specific Seed (Reproducibility)

```bash
go run ./server/cmd/autotester/ --tester=tester_migration --seed=12345
```

### 8.4 Run Specific Test Cases

```bash
go run ./server/cmd/autotester/ --tester=tester_migration \
  --test-ids=TC_2026022301,TC_2026022307,TC_2026022318
```

### 8.5 Run with Parallel Execution

```bash
go run ./server/cmd/autotester/ --tester=tester_migration --parallel
```

### 8.6 Filter by Tags

```bash
go run ./server/cmd/autotester/ --tester=tester_migration --tags=critical
```

### 8.7 Skip Cleanup (Debugging)

```bash
go run ./server/cmd/autotester/ --tester=tester_migration --skip-cleanup
```

### 8.8 Stop on First Failure

```bash
go run ./server/cmd/autotester/ --tester=tester_migration --stop-on-fail
```

### 8.9 Verbose Logging

```bash
go run ./server/cmd/autotester/ --tester=tester_migration --verbose
```

---

## 9. Deviation from Specification

### 9.1 Implemented vs. Specified

| Feature | Specified | Implemented | Notes |
|---------|-----------|-------------|-------|
| Static test cases | 25 | 19 | Consolidated overlapping cases |
| Dynamic case generation | Full state machine | Basic weighted random | State machine model deferred |
| Coverage tracking | Yes | No | Deferred to future iteration |
| Custom validators | Yes | No | Deferred to future iteration |
| Error path testing | Yes | Partial | Basic error cases covered |
| NO TRANSACTION testing | Yes | No | Deferred |
| Out-of-order testing | Yes | Partial | Basic support via AllowOutOfOrder |

### 9.2 Deferred Features

1. **State Machine Model:** Dynamic generation uses basic weighted random instead of full state machine traversal
2. **Coverage Tracking:** No coverage metrics tracking or coverage-driven generation
3. **Custom Validators:** No semantic DB state comparison validators
4. **SQL Template System:** Hard-coded SQL templates instead of template system
5. **PartialError Handling:** Basic error handling, no detailed partial failure tracking

### 9.3 Design Decisions

1. **Package Location:** Placed in `testers/` directory instead of `autotesters/` to match existing tester organization
2. **Package Name:** `tester_migration` instead of `autotesters` subpackage
3. **Logger:** Implemented `nopLogger` instead of using framework logger
4. **State Reset:** Full DUT reset per test case instead of incremental state changes

---

## 10. Testing and Verification

### 10.1 Build Verification

```bash
cd /Users/cding/Workspace/shared/go/api/testers/tester-migration
go build ./...
```

**Result:** ✅ Compiles without errors

### 10.2 Static Analysis

```bash
cd /Users/cding/Workspace/shared/go/api
go vet ./testers/tester-migration/...
```

**Result:** ✅ No issues detected

### 10.3 Integration Testing

To test the implementation:

1. **Setup Test Database:**
   ```sql
   CREATE DATABASE testonly_migration_dut;
   ```

2. **Configure Environment:**
   ```toml
   # mise.local.toml
   DUT_DB_NAME = "testonly_migration_dut"
   DUT_DSN = "postgres://admin:password@127.0.0.1:5432/testonly_migration_dut?sslmode=disable"
   ```

3. **Run Tests:**
   ```bash
   go run ./server/cmd/autotester/ --tester=tester_migration --seed=42 --verbose
   ```

### 10.4 Expected Output

```
AutoTester run started
  run_id: a3f8d012-...
  seed:   42
  env:    local

Running MigrationTester...
  [PASS] TC_2026022301 (45ms) - Apply all migrations from empty DB
  [PASS] TC_2026022302 (32ms) - Up is no-op when already current
  [PASS] TC_2026022303 (89ms) - Apply migrations one by one with UpByOne
  ...

AutoTester Run Complete
  Run ID   : a3f8d012-...
  Seed     : 42
  Env      : local
  Duration : 2m 15s
  Total    : 99
  Passed   : 95 (96.0%)
  Failed   : 2  (2.0%)
  Skipped  : 2  (2.0%)
  Errored  : 0  (0.0%)
```

---

## Appendix A: Error Codes

The implementation uses MID (Message ID) codes for error tracing:

| Code Range | Component |
|------------|-----------|
| `MID_260224100001` - `MID_260224100099` | Prepare/Cleanup |
| `MID_260224100030` - `MID_260224100049` | State tracking |
| `MID_260224100050` - `MID_260224100059` | Operation handlers |
| `MID_260224100019` - `MID_260224100029` | Test execution |

---

## Appendix B: Dependencies

### Go Dependencies

```go
import (
    "github.com/chendingplano/shared/go/api/ApiTypes"
    "github.com/chendingplano/shared/go/api/autotesters"
    "github.com/chendingplano/shared/go/api/databaseutil"
    "github.com/chendingplano/shared/go/api/goose"
    "github.com/pressly/goose/v3"
)
```

### Database Requirements

- PostgreSQL 12+ or MySQL 8+
- User must have permissions to:
  - Create/drop tables
  - Create/drop databases (for test isolation)
  - Query `information_schema`

---

## Appendix C: Future Enhancements

### Phase 2 (Next Iteration)

1. **Full State Machine Generation:** Implement state machine model for dynamic case generation
2. **Coverage Tracking:** Add coverage metrics and coverage-driven generation
3. **Custom Validators:** Implement semantic DB state comparison
4. **SQL Template System:** Add template-based SQL generation for varied test data
5. **NO TRANSACTION Testing:** Add support for testing migrations with `-- +goose NO TRANSACTION`

### Phase 3 (Future)

1. **MySQL Support:** Full MySQL testing (currently PostgreSQL-focused)
2. **Large Data Volumes:** Test with realistic table sizes
3. **Data Migration Testing:** Test migrations that modify data (not just schema)
4. **Parallel Migration Testing:** Test concurrent migration operations

---

## Appendix D: References

- Specification: `tester-migration-qwen-v4.md`
- Testbot Framework: `../../../Testbot/testbot.md`
- AutoTester Framework: `../../../Testbot/auto-tester-v2.md`
- Goose Migration: `../../dev/goose-v1.md`
- Goose Upstream: https://github.com/pressly/goose

---

**Document Created:** 2026-02-24  
**Implementation Status:** Complete (Phase 1)  
**Next Review:** After Phase 2 implementation
