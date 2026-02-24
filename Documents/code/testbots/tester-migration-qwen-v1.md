# Database Migration Testbot Development Plan

**Version:** 0.2
**Date:** 2026-02-23
**Status:** Draft

---

## Table of Contents

1. [Overview](#1-overview)
2. [Goals](#2-goals)
3. [Integration with AutoTester Framework](#3-integration-with-autotester-framework)
4. [Tester Design](#4-tester-design)
5. [Test Cases](#5-test-cases)
6. [Configuration](#6-configuration)
7. [Implementation Plan](#7-implementation-plan)
8. [Open Items](#8-open-items)

---

## 1. Overview

This document describes the plan for building a **Database Migration Tester** that automatically tests the correctness and reliability of database migrations using the **AutoTester framework** (see [`/Testbot/auto-tester-v2.md`](../../../Testbot/auto-tester-v2.md)) against the Goose migration system (see [`/shared/Documents/dev/goose-v1.md`](../../dev/goose-v1.md)).

The Migration Tester is a **Tester** implementation within the AutoTester framework, not a standalone testbot. It follows the AutoTester architecture and integrates with the shared `autotesters` package.

### 1.1 Background

The system uses **Goose** for database migrations with three separate migration tracks:
- **Project migrations** — Application-specific tables
- **Shared migrations** — Shared library tables (common across all projects)
- **AutoTester migrations** — Per-project isolated test result tables

Each track maintains independent version tracking, requiring comprehensive testing to ensure:
- Migrations apply correctly in order
- Rollbacks (Down migrations) work as expected
- Version tracking is accurate
- Edge cases are handled properly

### 1.2 SUT Definition

**System Under Test (SUT):** The Goose migration module (`shared/go/api/goose`)

The SUT exposes:
- Migration application (Up, UpByOne, UpTo)
- Migration rollback (Down, DownTo)
- Status inspection (Status, GetVersion, HasPending)
- Programmatic migration creation (CreateMigration, CreateAndApply)

---

## 2. Goals

### 2.1 Primary Goals

1. **Automated Migration Testing** — Automatically test migration apply/rollback cycles
2. **Version Tracking Verification** — Ensure the `db_migrations` table accurately reflects applied migrations
3. **Edge Case Coverage** — Test boundary conditions (empty migrations, NO TRANSACTION, out-of-order, etc.)
4. **Error Handling** — Verify proper error responses for invalid operations
5. **Regression Prevention** — Catch breaking changes in the migration system
6. **AutoTester Integration** — Seamlessly integrate with the AutoTester framework and runner

### 2.2 Non-Goals

- Testing application-specific migration SQL logic (that's the developer's responsibility)
- Performance benchmarking of migrations
- Testing the upstream `pressly/goose` library itself
- Replacing the AutoTester framework (this is a Tester, not a new framework)

---

## 3. Integration with AutoTester Framework

### 3.1 Architecture Position

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
```

### 3.2 Tester Interface Implementation

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

### 3.3 Directory Structure

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

### 3.4 Database Logging

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

### 3.5 Registration

The Migration Tester is registered in `server/cmd/autotester/registry.go`:

```go
autotesters.GlobalRegistry.Register("migration_tester", func() autotesters.Tester {
    return autotesters.NewMigrationTester()
})
```

### 3.6 CLI Usage

```bash
# Run all testers including MigrationTester
go run ./server/cmd/autotester/

# Run only MigrationTester
go run ./server/cmd/autotester/ --tester=migration_tester

# Run with specific seed for reproducibility
go run ./server/cmd/autotester/ --tester=migration_tester --seed=12345

# Run with parallel execution
go run ./server/cmd/autotester/ --tester=migration_tester --parallel

# Filter by tags
go run ./server/cmd/autotester/ --tags=migration,critical
```

---

## 4. Tester Design

### 4.1 MigrationTester Struct

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
    projectMigrationsDir string
    sharedMigrationsDir  string
    autotesterMigrationsDir string
    
    // Runtime state
    projectDB    *sql.DB
    sharedDB     *sql.DB
    autotesterDB *sql.DB
    
    // Migration directories (created for testing)
    testMigrationsDir string
    
    // Track being tested
    currentTrack string
    
    // State tracking
    appliedVersions []int64
    migrationFiles  []string
}
```

### 4.2 Metadata

```go
func NewMigrationTester() *MigrationTester {
    return &MigrationTester{
        BaseTester: BaseTester{
            name:        "migration_tester",
            description: "Tests database migration apply, rollback, and version tracking",
            purpose:     "validation",
            testType:    "integration",
            tags:        []string{"migration", "database", "goose", "critical"},
        },
    }
}
```

### 4.3 Prepare Phase

```go
func (t *MigrationTester) Prepare(ctx context.Context) error {
    // 1. Get database connections from ApiTypes globals
    t.projectDB = ApiTypes.PG_DB_Project
    t.sharedDB = ApiTypes.PG_DB_Shared
    t.autotesterDB = ApiTypes.PG_DB_AutoTester
    
    // 2. Verify connections
    if err := t.projectDB.PingContext(ctx); err != nil {
        return fmt.Errorf("project DB not reachable: %w", err)
    }
    
    // 3. Create temporary migration directory for test files
    t.testMigrationsDir, err = os.MkdirTemp("", "migration_test_*")
    if err != nil {
        return fmt.Errorf("create temp dir: %w", err)
    }
    
    // 4. Reset version tracking tables (drop and recreate for clean state)
    if err := t.resetVersionTables(ctx); err != nil {
        return fmt.Errorf("reset version tables: %w", err)
    }
    
    return nil
}
```

### 4.4 Cleanup Phase

```go
func (t *MigrationTester) Cleanup(ctx context.Context) error {
    // 1. Remove temporary migration directory
    if t.testMigrationsDir != "" {
        return os.RemoveAll(t.testMigrationsDir)
    }
    return nil
}
```

### 4.5 Test Case Generation

The tester uses **dynamic generation** via `GenerateTestCases`:

```go
func (t *MigrationTester) GenerateTestCases(ctx context.Context) ([]TestCase, error) {
    cases := make([]TestCase, 0, 50)
    
    // Use the seeded random source from BaseTester
    rng := t.Rand()
    
    // Category 1: Basic Apply/Rollback (6 cases)
    cases = append(cases, t.generateBasicCases(rng)...)
    
    // Category 2: Status Inspection (6 cases)
    cases = append(cases, t.generateStatusCases(rng)...)
    
    // Category 3: Migration Creation (5 cases)
    cases = append(cases, t.generateCreationCases(rng)...)
    
    // Category 4: Edge Cases (6 cases)
    cases = append(cases, t.generateEdgeCases(rng)...)
    
    // Category 5: Error Handling (5 cases)
    cases = append(cases, t.generateErrorCases(rng)...)
    
    // Category 6: Multi-Track (3 cases)
    cases = append(cases, t.generateMultiTrackCases(rng)...)
    
    return cases, nil
}
```

### 4.6 Test Case Execution

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
    
    // Route to appropriate handler based on test case ID prefix
    switch {
    case strings.HasPrefix(tc.ID, "migration.basic."):
        result = t.runBasicCase(ctx, tc, result)
    case strings.HasPrefix(tc.ID, "migration.status."):
        result = t.runStatusCase(ctx, tc, result)
    case strings.HasPrefix(tc.ID, "migration.creation."):
        result = t.runCreationCase(ctx, tc, result)
    case strings.HasPrefix(tc.ID, "migration.edge."):
        result = t.runEdgeCase(ctx, tc, result)
    case strings.HasPrefix(tc.ID, "migration.error."):
        result = t.runErrorCase(ctx, tc, result)
    case strings.HasPrefix(tc.ID, "migration.multitrack."):
        result = t.runMultiTrackCase(ctx, tc, result)
    default:
        result.Status = StatusError
        result.Error = fmt.Sprintf("unknown test case ID: %s", tc.ID)
    }
    
    result.EndTime = time.Now()
    result.Duration = result.EndTime.Sub(result.StartTime)
    return result
}
```

### 4.7 State Tracking

The tester maintains state in `BaseTester` or the struct:

- **Current track** — Which migration track is being tested (project/shared/autotester)
- **Applied versions** — List of migration versions currently applied
- **Migration files** — List of generated migration file paths
- **Random source** — `*rand.Rand` from `BaseTester.SetRand()` for reproducibility

---

## 5. Test Cases

### 5.1 Test Case Structure

Each test case follows the AutoTester `TestCase` structure:

```go
type TestCase struct {
    ID           string        // Unique ID: "migration.<category>.<variant>"
    Name         string        // Human-readable name
    Description  string        // What this case validates
    Purpose      string        // "smoke", "regression", "validation"
    Type         string        // "integration"
    Tags         []string      // e.g., ["migration", "critical"]
    Input        interface{}   // MigrationTestInput struct
    Expected     ExpectedResult
    Priority     Priority
    RetryCount   int
    Timeout      time.Duration
    Dependencies []string      // IDs of cases that must pass first
    SkipReason   string
}
```

### 5.2 Test Input Structure

```go
type MigrationTestInput struct {
    Track           string   // "project", "shared", "autotester"
    Operation       string   // "Up", "Down", "UpByOne", etc.
    TargetVersion   int64    // For UpTo/DownTo
    MigrationType   string   // "create_table", "add_column", etc.
    NumMigrations   int      // Number of migrations to create
    ExpectError     bool     // Whether error is expected
    ExpectedState   string   // "all_applied", "all_rolled_back", etc.
    SQLTemplate     string   // SQL template to use
}
```

### 5.3 Test Case Categories

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

### 5.4 Expected Result Verification

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
    
    // 6. Check side effects (e.g., "table_created", "version_applied")
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
    
    // All checks passed
    result.Status = StatusPass
    return result
}
```

---

## 6. Configuration

### 6.1 Environment Variables

```toml
# mise.local.toml
PG_USER_NAME = "admin"
PG_PASSWORD = "<password>"
PG_DB_NAME = "migration_test_project"
PG_DB_NAME_SHARED = "migration_test_shared"
PG_DB_NAME_AUTOTESTER = "migration_test_autotester"
PG_HOST = "127.0.0.1"
PG_PORT = "5432"
```

### 6.2 RunConfig Usage

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

### 6.3 Test Case Priority Distribution

| Priority | Count | Percentage |
|---|---|---|
| Critical | 8 | 16% |
| High | 18 | 36% |
| Medium | 18 | 36% |
| Low | 6 | 12% |

---

## 7. Implementation Plan

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
| P2-1 | Implement Category 1 (Basic Apply/Rollback) cases | High |
| P2-2 | Implement Category 2 (Status Inspection) cases | High |
| P2-3 | Implement `RunTestCase` routing and execution | High |
| P2-4 | Implement side effect observation (table created, version applied) | High |
| P2-5 | Register tester in `server/cmd/autotester/registry.go` | High |

### Phase 3: Advanced Test Cases (Week 3)

| Task | Description | Priority |
|---|---|---|
| P3-1 | Implement Category 3 (Migration Creation) cases | High |
| P3-2 | Implement Category 4 (Edge Cases) | Medium |
| P3-3 | Implement Category 5 (Error Handling) | Medium |
| P3-4 | Implement Category 6 (Multi-Track) | Low |
| P3-5 | Implement weighted random generation using `t.Rand()` | Medium |

### Phase 4: Polish & Integration (Week 4)

| Task | Description | Priority |
|---|---|---|
| P4-1 | Add comprehensive logging via `auto_test_logs` | Medium |
| P4-2 | Implement test case filtering by tags | Medium |
| P4-3 | Write documentation and usage examples | High |
| P4-4 | Integration testing with real projects | High |
| P4-5 | Add to CI/CD pipeline | Medium |

---

## 8. Open Items

### 8.1 TBD Items

| Item | Description |
|---|---|
| **SQL Template System** | How to generate varied but valid SQL for dynamic tests |
| **Transaction Handling** | Whether to wrap each test case in a transaction for isolation |
| **MySQL Support** | Whether to test MySQL migrations in addition to PostgreSQL |
| **Large Data Volumes** | Whether to test with realistic table sizes |

### 8.2 Questions

1. Should the tester run against all three tracks in parallel or sequentially?
2. Should failed migrations generate SQL fix suggestions?
3. How to handle migrations that modify data (not just schema)?
4. Should we maintain a library of pre-defined migration templates?

---

## Appendix A: Example Test Run

```bash
# Run MigrationTester with verbose logging
$ go run ./server/cmd/autotester/ --tester=migration_tester --verbose --seed=12345

AutoTester run started
  run_id: a3f8d012-...
  seed:   12345
  env:    local
  
Running MigrationTester...
  [PASS] migration.basic.apply_all (45ms)
  [PASS] migration.basic.rollback_all (32ms)
  [PASS] migration.basic.apply_one_by_one (89ms)
  [FAIL] migration.edge.out_of_order (12ms) "expected v2 to apply, got error: version already applied"
  [SKIP] migration.multitrack.independent (0ms) "dependency migration.edge.out_of_order not passed"
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

## Appendix C: References

- AutoTester Framework: `/Testbot/auto-tester-v2.md`
- Goose Migration Docs: `/shared/Documents/dev/goose-v1.md`
- Goose Upstream: https://github.com/pressly/goose
- Shared Autotesters: `shared/go/api/autotesters/`
