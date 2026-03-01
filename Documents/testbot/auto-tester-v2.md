# AutoTester: Automated Testing Framework (v2)

**Package:** `github.com/chendingplano/shared/go/api/autotesters`  
**Source:** [`shared/go/api/autotesters/`](../../go/api/autotesters/)

**Created:** 2026/02/20 by Qwen Code, compiled from auto-tester\*

AutoTester is a structured automated testing framework for validating a **System Under Test (SUT)**. It orchestrates one or more **Tester** implementations to prepare the system, generate or load test cases, execute them, collect results, verify pass/fail, and log outcomes in a database.

This document consolidates the architecture, conventions, and implementation guidelines for AutoTester across shared modules and application-specific codebases.

---

## Table of Contents

1. [Core Concepts](#1-core-concepts)
2. [Architecture](#2-architecture)
3. [Directory Structure](#3-directory-structure)
4. [Tester Interface and BaseTester](#4-tester-interface-and-basetester)
5. [Test Lifecycle](#5-test-lifecycle)
   - 5.1 [Prepare the System](#51-prepare-the-system)
   - 5.2 [Create Test Cases](#52-create-test-cases)
   - 5.3 [Run Test Cases](#53-run-test-cases)
   - 5.4 [Collect Results](#54-collect-results)
   - 5.5 [Verify Pass/Fail](#55-verify-passfail)
   - 5.6 [Log Tests](#56-log-tests)
6. [Data Structures](#6-data-structures)
7. [Directories and Files](#7-directories-files)
8. [Database Schema](#8-database-schema)
9. [Tester Registry](#8-tester-registry)
10. [Test Runner (Orchestrator)](#9-test-runner-orchestrator)
11. [CLI Entry Point](#10-cli-entry-point)
12. [Test Selection and Filtering](#11-test-selection-and-filtering)
13. [Randomness, Seeding, and Replay](#12-randomness-seeding-and-replay)
14. [Concurrency Model](#13-concurrency-model)
15. [Test Dependencies and Ordering](#14-test-dependencies-and-ordering)
16. [Test Data Management and Fixtures](#15-test-data-management-and-fixtures)
17. [Safety and Environment Isolation](#16-safety-and-environment-isolation)
18. [Error Classification and Reporting](#17-error-classification-and-reporting)
19. [CI/CD Integration](#18-cicd-integration)
20. [Best Practices](#19-best-practices)
21. [Examples](#20-examples)

---

## 1. Core Concepts

### System Under Test (SUT)

The SUT is whatever component is being validated. It may be:

- A function or package in the shared library (e.g., `shared/go/api/databaseutil`)
- An application service layer (e.g., `tax/server/api/services`)
- An HTTP API endpoint (tested via an `httptest.Server`)
- An external integration point (email, S3, third-party API)
- A full end-to-end workflow spanning multiple layers

The tester is responsible for knowing how to reach, initialize, and reset the SUT. This includes establishing the right database handle, seeding fixture data, or spinning up a test HTTP server.

### Tester

A **Tester** is a Go implementation that tests one aspect of the SUT. Each Tester should be:

- Focused on a specific functional area or system capability
- Runnable independently or as part of a suite
- Self-contained in a `.go` file (or small package if needed)
- Implementing the `Tester` interface
- Managing its own setup, test cases, execution, verification, and teardown
- Not depending on other Testers at runtime (cross-Tester dependencies are declared explicitly via `TestCase.Dependencies`, not via Go-level coupling)

### AutoTester

The **AutoTester** is the top-level orchestrator. It:

- Loads a set of registered Testers
- Applies command-line filters (purpose, type, tags, specific test IDs, etc.)
- Drives the full lifecycle of each Tester in order (or in parallel)
- Writes a `TestRun` record to the database before starting, and updates it when done
- Prints a human-readable summary and optionally a machine-readable JSON report
- Returns a non-zero exit code if any test failed or errored

### TestRun

A **TestRun** is a single execution of the AutoTester binary. It has a globally unique `run_id` (UUID), a start timestamp, configuration metadata (flags used, environment, seed), and aggregate counters for pass/fail/skip/error. It is the top-level record in the database log.

### TestCase

A **TestCase** is one scenario: an input (or absence of input), an expected outcome, and metadata (ID, purpose, type, tags, priority, retry count, timeout, dependencies). Test cases can be either:

- **Static (hard-coded):** deterministic, always the same, ideal for regression and smoke tests
- **Dynamic (generated at runtime):** driven by randomness, parametric ranges, or external data; ideal for fuzz-style, stress, and property-based coverage

### TestResult

A **TestResult** captures what actually happened when a TestCase was executed: the status (pass/fail/skip/error), timing, actual output value, side effects observed, any error message, retry count, and detailed log entries.

---

## 2. Architecture

```
┌────────────────────────────────────────────────────────────────┐
│                     go run .../autotester                      │
│         (server/cmd/autotester/main.go + config.go)            │
└────────────────────────────┬───────────────────────────────────┘
                             │  flags: --purpose --type --tags
                             │  --seed --parallel --retry ...
                             ▼
┌────────────────────────────────────────────────────────────────┐
│                      TestRunner                                │
│  ① create run_id, open auto_test_runs row                      │
│  ② iterate registered Testers                                  │
│  ③ apply filters (skip Testers that don't match)               │
│  ④ for each Tester: Prepare → cases → run → Cleanup            │
│  ⑤ stream results to DB as they arrive                         │
│  ⑥ close run row, print summary, exit                          │
└────────────────────────────┬───────────────────────────────────┘
                             │
          ┌──────────────────┼──────────────────┐
          ▼                  ▼                  ▼
 ┌─────────────────┐ ┌──────────────┐ ┌─────────────────┐
 │  Tester A       │ │  Tester B    │ │  Tester C       │
 │ shared module   │ │ app service  │ │ app API         │
 └────────┬────────┘ └──────┬───────┘ └────────┬────────┘
          │                 │                  │
          └─────────────────┼──────────────────┘
                            ▼
┌────────────────────────────────────────────────────────────────┐
│  PostgreSQL (ApiTypes.PG_DB_AutoTester) ← per-project DB       │
│  auto_test_runs  │  auto_test_results  │  auto_test_logs       │
└────────────────────────────────────────────────────────────────┘
```

The shared library (`shared/go/api/autotesters`) provides:

- The `Tester` interface and `BaseTester` embed
- `TestCase`, `TestResult`, `TestRun`, `RunConfig` data structures
- `TestRunner` orchestration logic
- Database persistence helpers
- Randomness utilities (`RandSource`, `NewSeededRand`)
- Common assertions (`AssertEqual`, `AssertNoError`, etc.)

Application code (`server/api/autotesters`) provides:

- Application-specific Tester implementations
- Optionally overrides or extends registry logic

The CLI entry point (`server/cmd/autotester`) wires everything together with a `main.go`.

---

## 3. Directory Structure

### Shared Library Testers

```
shared/
└── go/
    └── api/
        └── autotesters/
            ├── autotesters.go        # Tester interface, BaseTester, shared types
            ├── testcase.go           # TestCase, ExpectedResult, Priority, Status enums
            ├── testresult.go         # TestResult, LogEntry
            ├── testrun.go            # TestRun, RunConfig, RunSummary
            ├── runner.go             # TestRunner orchestration
            ├── registry.go           # TesterRegistry (register/lookup)
            ├── db.go                 # DB persistence (auto_test_runs, results, logs)
            ├── rand.go               # Seeded randomness helpers
            ├── assert.go             # Common assertion helpers
            ├── fixtures.go           # Fixture loading utilities
            │
            ├── tester_database.go    # Tester: database connectivity & CRUD
            ├── tester_databaseutil.go# Tester: databaseutil package
            ├── tester_auth.go        # Tester: JWT / OAuth auth module
            ├── tester_logger.go      # Tester: loggerutil package
            └── ...
```

### Application-Specific Testers

```
myapp/                          (e.g. tax/ or ChenWeb/)
└── server/
    ├── api/
    │   └── autotesters/
    │       ├── user_tester.go          # Tests: user service / API
    │       ├── document_tester.go      # Tests: document handling
    │       ├── project_tester.go       # Tests: project workflows
    │       ├── email_tester.go         # Tests: email sending (Resend)
    │       └── integration_tester.go   # Tests: cross-service flows
    └── cmd/
        └── autotester/
            ├── main.go                 # Entry point: parse flags, register, run
            ├── config.go               # Extend RunConfig with app-specific flags
            └── registry.go             # Register all shared + app testers
```

### Naming Rules

| Item | Convention | Example |
|---|---|---|
| Tester file | `<module>_tester.go` | `tester_database.go` |
| Tester struct | `<Module>Tester` | `DatabaseTester` |
| Constructor | `New<Module>Tester(...)` | `NewDatabaseTester(cfg)` |
| TestCase IDs | `<module>.<feature>.<variant>` | `db.conn.basic` |
| Run IDs | UUID v4 | `a3f8...` |

---

## 4. Tester Interface and BaseTester

### The `Tester` Interface

Every Tester must satisfy this interface, defined in `shared/go/api/autotesters/autotesters.go`:

```go
package autotesters

import "context"

// Tester is the contract every automated tester must implement.
type Tester interface {
    // Identity / metadata
    Name()        string   // unique machine name, e.g. "tester_database"
    Description() string   // human-readable summary
    Purpose()     string   // e.g. "validation", "regression", "smoke", "load"
    Type()        string   // e.g. "unit", "integration", "e2e"
    Tags()        []string // optional labels, e.g. ["database","critical"]

    // Lifecycle
    Prepare(ctx context.Context) error   // set up SUT, fixtures, connections
    Cleanup(ctx context.Context) error   // tear down, roll back, close connections

    // Test case supply (implement one or both)
    GenerateTestCases(ctx context.Context) ([]TestCase, error)
    // GenerateTestCases returns dynamically created cases.
    // Return nil (not error) to fall through to GetTestCases.

    GetTestCases() []TestCase
    // GetTestCases returns hard-coded static cases.
    // Return nil if the tester relies entirely on GenerateTestCases.

    // Execution
    RunTestCase(ctx context.Context, tc TestCase) TestResult
    // RunTestCase executes exactly one test case and returns the raw result.
    // The runner handles timing, retry, and logging wrappers.
}
```

The runner calls `GenerateTestCases` first. If it returns `nil, nil` (no cases, no error), it falls back to `GetTestCases`. A tester may implement both to combine static and dynamic cases.

### `BaseTester` — Embedding for Convenience

`BaseTester` provides default no-op implementations of every interface method except `Name`, `RunTestCase`, and the case supply methods. Embed it to reduce boilerplate:

```go
type BaseTester struct {
    name        string
    description string
    purpose     string
    testType    string
    tags        []string
}

func (b *BaseTester) Name()        string   { return b.name }
func (b *BaseTester) Description() string   { return b.description }
func (b *BaseTester) Purpose()     string   { return b.purpose }
func (b *BaseTester) Type()        string   { return b.testType }
func (b *BaseTester) Tags()        []string { return b.tags }

func (b *BaseTester) Prepare(ctx context.Context) error  { return nil }
func (b *BaseTester) Cleanup(ctx context.Context) error  { return nil }

func (b *BaseTester) GenerateTestCases(ctx context.Context) ([]TestCase, error) {
    return nil, nil // signal: use GetTestCases
}
func (b *BaseTester) GetTestCases() []TestCase { return nil }
```

A minimal tester only needs to define its metadata constants, implement `RunTestCase`, and supply cases through either `GetTestCases` or `GenerateTestCases`.

---

## 5. Test Lifecycle

### 5.1 Prepare the System

`Prepare(ctx)` is called exactly once per Tester before any test case runs. Its responsibilities:

**a) Acquire or verify the SUT**
- For in-process modules: instantiate the service with a test configuration
- For HTTP APIs: spin up an `httptest.Server` wrapping the real router
- For database-heavy tests: obtain a DB connection (typically `ApiTypes.PG_DB_Project`) and confirm it responds to a ping

**b) Establish baseline state**
- Truncate or soft-delete rows written by previous test runs (use a `test_run_id` column or a dedicated test schema to keep cleanup surgical)
- Insert required fixtures (reference data, lookup tables, parent records)
- Roll back any uncommitted transactions from a previous failed run

**c) Validate readiness**
- If the SUT has a health endpoint, call it and assert HTTP 200
- If the SUT depends on an external service (SMTP relay, S3 bucket), confirm connectivity
- If any prerequisite is unavailable, return a descriptive error — the runner will mark the entire Tester as errored and skip its test cases rather than running tests that are doomed to fail

**d) Record the test environment**
- Capture the Go runtime version, database version, hostname, and app config hash
- Store these in `TestRun.EnvMetadata` (persisted to `auto_test_runs.env_json`)

```go
func (t *UserTester) Prepare(ctx context.Context) error {
    // Confirm DB is available
    if err := ApiTypes.PG_DB_Project.PingContext(ctx); err != nil {
        return fmt.Errorf("postgres not reachable: %w", err)
    }

    // Seed required reference data
    if err := t.seedRoles(ctx); err != nil {
        return fmt.Errorf("seed roles: %w", err)
    }

    // Start test HTTP server
    t.server = httptest.NewServer(t.buildRouter())
    t.client = &http.Client{Timeout: 10 * time.Second}
    return nil
}
```

### 5.2 Create Test Cases

The runner calls `GenerateTestCases(ctx)` first, then `GetTestCases()` as a fallback.

#### Static (Hard-Coded) Test Cases

Static cases go in `GetTestCases()`. They encode known invariants, regressions, and boundary conditions that must pass on every run without variation:

```go
func (t *UserTester) GetTestCases() []TestCase {
    return []TestCase{
        {
            ID:       "TC_YYYYMMDDSS",
            Name:     "Create user with minimum required fields",
            Input:    CreateUserInput{Email: "test@example.com", Name: "Test"},
            Expected: ExpectedResult{Success: true},
            Priority: PriorityCritical,
        },
        {
            ID:       "TC_YYYYMMDDSS",
            Name:     "Creating a user with a duplicate email returns a conflict error",
            Input:    CreateUserInput{Email: "dupe@example.com", Name: "Dupe"},
            Expected: ExpectedResult{
                Success:       false,
                ExpectedError: "duplicate",
            },
            Priority: PriorityHigh,
        },
    }
}
```

Test cases are identified by `text ID`, which should be unique across all test cases. The default format is:
```text
TC_YYYYMMDDSS
```
where:
```text
YYYY: year (such as '2026')
MM: month
DD: day
SS: sequence number, fixed-length, 0-padded
```

Example:
```text
TC_2026022301
TC_2026022302
...
```

#### Dynamic (Generated) Test Cases

Dynamic cases are produced in `GenerateTestCases(ctx)`. They use `t.rand` (a seeded `*rand.Rand` obtained from the runner) to produce varied inputs. The seed is logged so a failing run can be replayed exactly:

```go
func (t *UserTester) GenerateTestCases(ctx context.Context) ([]TestCase, error) {
    cases := make([]TestCase, 0, 200)

    // Random valid users
    for i := 0; i < 100; i++ {
        u := generateRandomUser(t.rand) // uses t.rand from BaseTester
        cases = append(cases, TestCase{
            ID:       fmt.Sprintf("TC_YYYYMMDD%03d", i),
            Name:     fmt.Sprintf("Random user creation %d", i),
            Input:    u,
            Expected: ExpectedResult{Success: true, MaxDuration: 200 * time.Millisecond},
            Priority: PriorityMedium,
            Tags:     []string{"random"},
        })
    }

    // Edge cases driven by parameterized table
    for _, ec := range emailEdgeCases() {
        cases = append(cases, TestCase{
            ID:       fmt.Sprintf("TC_YYYYMMDD%3d", i),
            Name:     ec.description,
            Input:    CreateUserInput{Email: ec.email, Name: "Edge"},
            Expected: ExpectedResult{Success: ec.valid},
            Priority: PriorityHigh,
            Tags:     []string{"edge-case", "email"},
        })
    }

    return cases, nil
}
```

**When to use each approach:**

| Situation | Approach |
|---|---|
| Known regression scenario | Static |
| Core invariant that must always hold | Static |
| Smoke / deployment check | Static (priority: Critical) |
| Stress / volume test | Dynamic (generated in loop) |
| Property-based / fuzz coverage | Dynamic (random) |
| Data-driven from external file | Dynamic (read file, build cases) |
| Combination | Both (runner merges the slices) |

### 5.3 Run Test Cases

`RunTestCase(ctx, tc)` is called once per test case. It is responsible for:

1. **Invoking the SUT** with the test input
2. **Catching panics** — wrap the call body in a deferred recover; a panic should produce `Status: StatusError`, never crash the runner
3. **Measuring timing** — record `StartTime` before the call, `EndTime` after
4. **Recording the raw output** — store the actual return value in `TestResult.ActualValue` before any assertion, so it is always persisted regardless of pass/fail
5. **Observing side effects** — if the test is expected to create a DB row, check for it and append the observed side effect string to `TestResult.SideEffectsObserved`
6. **Not determining pass/fail** — `RunTestCase` fills in the raw facts; the runner calls `verifyResult(tc, result)` afterward to apply the assertions

```go
func (t *UserTester) RunTestCase(ctx context.Context, tc TestCase) TestResult {
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

    switch {
    case strings.HasPrefix(tc.ID, "user.create."):
        t.runCreateUser(ctx, tc, &result)
    case strings.HasPrefix(tc.ID, "user.get."):
        t.runGetUser(ctx, tc, &result)
    case strings.HasPrefix(tc.ID, "user.delete."):
        t.runDeleteUser(ctx, tc, &result)
    default:
        result.Status = StatusError
        result.Error = fmt.Sprintf("unknown test case ID: %s", tc.ID)
    }

    result.EndTime = time.Now()
    result.Duration = result.EndTime.Sub(result.StartTime)
    return result
}

func (t *UserTester) runCreateUser(ctx context.Context, tc TestCase, r *TestResult) {
    input := tc.Input.(CreateUserInput)
    user, err := t.userService.Create(ctx, input)
    if err != nil {
        r.Error = err.Error()
        return
    }
    r.ActualValue = user
    if user.ID != "" {
        r.SideEffectsObserved = append(r.SideEffectsObserved, "user_row_created")
    }
}
```

### 5.4 Collect Results

"Collecting results" means storing them durably as they arrive, not just aggregating them in memory. The runner streams each `TestResult` to the database immediately after `RunTestCase` returns, inside `TestRunner.recordResult(result)`. This means:

- Partial results are preserved if the runner is killed mid-run
- A separate monitoring process can tail the `auto_test_results` table in real time
- Memory usage stays constant regardless of the number of test cases

The runner also updates `auto_test_runs.passed_count` etc. atomically in the DB (or, for performance, via an in-memory counter that is flushed to the DB in `updateRunRecord` at the end). The choice between streaming DB updates vs. bulk-flush at the end is a performance trade-off: streaming is safer; bulk-flush is faster for large suites.

### 5.5 Verify Pass/Fail

`verifyResult(tc TestCase, result TestResult) TestResult` is called by the runner (not inside the Tester itself) to apply assertions after `RunTestCase` returns. Separating execution from assertion means:

- The raw output is always recorded, even when an assertion fails
- Assertion logic is centralized and consistent across all Testers

Verification order:

1. **Skip check** — if `tc.SkipReason != ""`, mark as `StatusSkip` and stop
2. **Dependency check** — if any listed dependency did not pass, mark as `StatusSkip` (or optionally `StatusFail` based on config) and stop
3. **Success/error expectation** — check whether an error was expected or unexpected
4. **Expected error content** — if `Expected.ExpectedError != ""`, the actual error string must contain it (case-insensitive substring match by default)
5. **Value equality** — if `Expected.ExpectedValue != nil`, compare with `reflect.DeepEqual` (or a custom comparator via `Expected.CustomValidator`)
6. **Duration constraint** — if `Expected.MaxDuration > 0`, fail if `result.Duration` exceeds it
7. **Side effects** — every string in `Expected.SideEffects` must appear in `result.SideEffectsObserved`
8. **Custom validator** — if `Expected.CustomValidator` is set, call it last; its return value overrides the status

Only after all checks pass does the result receive `StatusPass`.

### 5.6 Log Tests

Every test result — pass, fail, skip, or error — is persisted to `auto_test_results`. The `auto_test_logs` table stores per-test structured log lines emitted during execution. The `auto_test_runs` row is kept up to date and is marked `completed` (or `failed`) when the runner exits.

Logging is the non-negotiable part of the framework. Even if the verification step later changes, the raw facts of what happened must always be on record:

- What input was given
- What output was received
- How long it took
- What error (if any) occurred
- What the random seed was (for replay)

---

## 6. Data Structures

All types live in `shared/go/api/autotesters/`.

### `TestCase`

```go
type TestCase struct {
    ID           string        // Unique ID: "<module>.<feature>.<variant>"
    Name         string        // Human-readable name
    Description  string        // What this case validates and why
    Purpose      string        // "smoke", "regression", "load", "fuzz", "compliance"
    Type         string        // "unit", "integration", "e2e"
    Tags         []string      // Free-form labels for filtering
    Input        interface{}   // Any serializable input value
    Expected     ExpectedResult
    Priority     Priority
    RetryCount   int           // 0 = no retry; overrides RunConfig.RetryCount if > 0
    Timeout      time.Duration // 0 = use RunConfig.CaseTimeout
    Dependencies []string      // IDs of cases that must have StatusPass first
    SkipReason   string        // Non-empty = skip this case with this reason
}
```

### `ExpectedResult`

```go
type ExpectedResult struct {
    Success         bool          // true = expect no error; false = expect an error
    ExpectedError   string        // Substring expected in the error message
    ExpectedValue   interface{}   // Exact value to compare with ActualValue
    MaxDuration     time.Duration // Fail if execution exceeds this
    SideEffects     []string      // Side effect keys that must appear in the result
    CustomValidator func(actual interface{}, expected ExpectedResult) (pass bool, reason string)
}
```

### `TestResult`

```go
type TestResult struct {
    RunID               string
    TestCaseID          string
    TesterName          string
    Status              Status        // pass / fail / skip / error
    Message             string        // human explanation of outcome
    Error               string        // error string if Status != pass
    StartTime           time.Time
    EndTime             time.Time
    Duration            time.Duration
    RetryCount          int           // how many retries were actually performed
    ActualValue         interface{}   // raw output from the SUT
    SideEffectsObserved []string      // side effects actually observed
    Logs                []LogEntry    // structured log lines from this test
}
```

### `Status` and `Priority`

```go
type Status string
const (
    StatusPass  Status = "pass"
    StatusFail  Status = "fail"
    StatusSkip  Status = "skip"
    StatusError Status = "error"  // infrastructure / panic; not a test assertion failure
)

type Priority int
const (
    PriorityCritical Priority = iota // Must pass for deployment
    PriorityHigh                     // Core functionality
    PriorityMedium                   // Standard coverage
    PriorityLow                      // Nice-to-have, stress, fuzz
)
```

### `RunConfig`

```go
type RunConfig struct {
    Purposes     []string      // filter: include testers matching any of these
    Types        []string      // filter: include testers matching any of these
    Tags         []string      // filter: include testers tagged with any of these
    TesterNames  []string      // filter: run only these specific Testers by Name()
    TestIDs      []string      // filter: run only these specific TestCase IDs
    Seed         int64         // randomness seed; 0 = auto-generate and log
    Parallel     bool          // enable goroutine-per-Tester execution
    MaxParallel  int           // cap on concurrent goroutines (default: 4)
    RetryCount   int           // default retry count for failed cases (default: 0)
    CaseTimeout  time.Duration // per-test-case timeout (default: 30s)
    RunTimeout   time.Duration // overall run timeout (default: 30m)
    StopOnFail   bool          // abort run on first StatusFail
    SkipCleanup  bool          // skip Tester.Cleanup (for post-mortem debugging)
    Verbose      bool          // emit DEBUG-level logs to stdout
    JSONReport   string        // if non-empty, write JSON summary to this file path
    Environment  string        // "local", "test", "staging" (default: "local")
}
```

---

## 7. Directories and Files  

### Shared 
Code files in shared/ are in ./shared/go/api/testbot.

### Application
Code files for applications are stored in server/api/testbot and server/cmd/testbot

## 8. Database Schema

### Table Creation Pattern

Auto-test tables (`auto_test_runs`, `auto_test_results`, `auto_test_logs`) live in each project's **dedicated autotester DB** (`PG_DB_AutoTester`), not in the project data DB. This isolates test history from production data and from other projects' test runs.

The `CreateAutoTestXxxTable` functions live in `shared/go/api/autotesters/db.go`. The convenience wrapper `CreateAutoTestTables` creates all three in one call and is invoked from `server/cmd/autotester/main.go` at startup — **not** from `sysdatastores.CreateSysTables()`.

```go
// In server/cmd/autotester/main.go
if err := autotesters.CreateAutoTestTables(logger, ApiTypes.PG_DB_AutoTester, dbType); err != nil {
    log.Fatal("failed to create auto-test tables:", err)
}
```

```go
// In shared/go/api/autotesters/db.go

func CreateAutoTestRunsTable(logger ApiTypes.JimoLogger, db *sql.DB, dbType string, tableName string) error {
    logger.Info("Create table", "table_name", tableName)
    stmt := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
        id          BIGSERIAL PRIMARY KEY,
        run_id      VARCHAR(64)  NOT NULL UNIQUE,
        started_at  TIMESTAMPTZ  NOT NULL,
        ended_at    TIMESTAMPTZ,
        status      VARCHAR(20)  NOT NULL DEFAULT 'running'
                        CHECK (status IN ('running','completed','failed','partial')),
        env         VARCHAR(40)  NOT NULL DEFAULT 'local',
        seed        BIGINT       NOT NULL DEFAULT 0,
        config_json JSONB,
        env_json    JSONB,
        total       INTEGER      NOT NULL DEFAULT 0,
        passed      INTEGER      NOT NULL DEFAULT 0,
        failed      INTEGER      NOT NULL DEFAULT 0,
        skipped     INTEGER      NOT NULL DEFAULT 0,
        errored     INTEGER      NOT NULL DEFAULT 0,
        duration_ms BIGINT,
        report_path VARCHAR(512),
        created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
        updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
    )`, tableName)
    return databaseutil.ExecuteStatement(db, stmt)
}

func CreateAutoTestResultsTable(logger ApiTypes.JimoLogger, db *sql.DB, dbType string, tableName string) error {
    logger.Info("Create table", "table_name", tableName)
    stmt := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
        id                   BIGSERIAL    PRIMARY KEY,
        run_id               VARCHAR(64)  NOT NULL,
        test_case_id         VARCHAR(200) NOT NULL,
        tester_name          VARCHAR(128) NOT NULL,
        status               VARCHAR(20)  NOT NULL
                                 CHECK (status IN ('pass','fail','skip','error')),
        message              TEXT,
        error                TEXT,
        start_time           TIMESTAMPTZ  NOT NULL,
        end_time             TIMESTAMPTZ  NOT NULL,
        duration_ms          BIGINT       NOT NULL,
        retry_count          INTEGER      NOT NULL DEFAULT 0,
        actual_value_json    JSONB,
        side_effects         TEXT[],
        created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
        CONSTRAINT fk_auto_test_results_run
            FOREIGN KEY (run_id) REFERENCES auto_test_runs(run_id) ON DELETE CASCADE
    )`, tableName)
    return databaseutil.ExecuteStatement(db, stmt)
}

func CreateAutoTestLogsTable(logger ApiTypes.JimoLogger, db *sql.DB, dbType string, tableName string) error {
    logger.Info("Create table", "table_name", tableName)
    stmt := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
        id           BIGSERIAL    PRIMARY KEY,
        run_id       VARCHAR(64)  NOT NULL,
        test_case_id VARCHAR(200),
        tester_name  VARCHAR(128) NOT NULL,
        log_level    VARCHAR(10)  NOT NULL CHECK (log_level IN ('DEBUG','INFO','WARN','ERROR')),
        message      TEXT         NOT NULL,
        context_json JSONB,
        logged_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
        CONSTRAINT fk_auto_test_logs_run
            FOREIGN KEY (run_id) REFERENCES auto_test_runs(run_id) ON DELETE CASCADE
    )`, tableName)
    return databaseutil.ExecuteStatement(db, stmt)
}
```

### Indexes

```sql
-- auto_test_runs
CREATE INDEX IF NOT EXISTS idx_atr_started_at  ON auto_test_runs(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_atr_status      ON auto_test_runs(status);
CREATE INDEX IF NOT EXISTS idx_atr_env         ON auto_test_runs(env);

-- auto_test_results
CREATE INDEX IF NOT EXISTS idx_atres_run_id     ON auto_test_results(run_id);
CREATE INDEX IF NOT EXISTS idx_atres_tester     ON auto_test_results(tester_name);
CREATE INDEX IF NOT EXISTS idx_atres_status     ON auto_test_results(status);
CREATE INDEX IF NOT EXISTS idx_atres_case_id    ON auto_test_results(test_case_id);
CREATE INDEX IF NOT EXISTS idx_atres_start_time ON auto_test_results(start_time DESC);

-- auto_test_logs
CREATE INDEX IF NOT EXISTS idx_atlog_run_id     ON auto_test_logs(run_id);
CREATE INDEX IF NOT EXISTS idx_atlog_case_id    ON auto_test_logs(test_case_id);
CREATE INDEX IF NOT EXISTS idx_atlog_level      ON auto_test_logs(log_level);
```

### Table Name Configuration

Table names are configured in `shared/libconfig.toml` under `[system_table_names]` and loaded into `ApiTypes.LibConfig.SystemTableNames`:

```toml
# shared/libconfig.toml
[system_table_names]
table_name_auto_test_runs    = "auto_test_runs"
table_name_auto_test_results = "auto_test_results"
table_name_auto_test_logs    = "auto_test_logs"
```

The corresponding Go struct fields in `ApiTypes.SystemTableNames`:

```go
TableNameAutoTestRuns    string `mapstructure:"table_name_auto_test_runs"`
TableNameAutoTestResults string `mapstructure:"table_name_auto_test_results"`
TableNameAutoTestLogs    string `mapstructure:"table_name_auto_test_logs"`
```

Access the configured names via:

```go
ApiTypes.LibConfig.SystemTableNames.TableNameAutoTestRuns
ApiTypes.LibConfig.SystemTableNames.TableNameAutoTestResults
ApiTypes.LibConfig.SystemTableNames.TableNameAutoTestLogs
```

---

## 9. Tester Registry

The registry (`shared/go/api/autotesters/registry.go`) maps tester names to factory functions. This avoids import cycles and supports lazy construction (Testers receive their dependencies at registration time via closures):

```go
// TesterFactory is a function that constructs a Tester.
type TesterFactory func() Tester

// TesterRegistry holds the set of known Testers.
type TesterRegistry struct {
    factories map[string]TesterFactory
    mu        sync.RWMutex
}

var GlobalRegistry = &TesterRegistry{
    factories: make(map[string]TesterFactory),
}

// Register adds a Tester factory. Panics on duplicate name (caught at startup).
func (r *TesterRegistry) Register(name string, factory TesterFactory) {
    r.mu.Lock()
    defer r.mu.Unlock()
    if _, exists := r.factories[name]; exists {
        panic("duplicate tester name: " + name)
    }
    r.factories[name] = factory
}

// Build instantiates all registered Testers.
func (r *TesterRegistry) Build() []Tester {
    r.mu.RLock()
    defer r.mu.RUnlock()
    testers := make([]Tester, 0, len(r.factories))
    for _, factory := range r.factories {
        testers = append(testers, factory())
    }
    return testers
}
```

In `server/cmd/autotester/registry.go`, register all Testers before calling `runner.Run`:

```go
func registerAll(cfg *config.Config) {
    // Shared library testers
    autotesters.GlobalRegistry.Register("tester_database", func() autotesters.Tester {
        return autotesters.NewDatabaseTester()
    })
    autotesters.GlobalRegistry.Register("tester_auth", func() autotesters.Tester {
        return autotesters.NewAuthTester()
    })

    // Application-specific testers
    autotesters.GlobalRegistry.Register("user_tester", func() autotesters.Tester {
        return apptests.NewUserTester(cfg)
    })
    autotesters.GlobalRegistry.Register("project_tester", func() autotesters.Tester {
        return apptests.NewProjectTester(cfg)
    })
}
```

---

## 10. Test Runner (Orchestrator)

`TestRunner` in `shared/go/api/autotesters/runner.go` drives the full execution:

```go
type TestRunner struct {
    testers   []Tester
    config    *RunConfig
    runID     string
    seed      int64
    startTime time.Time
    logger    ApiTypes.JimoLogger

    mu       sync.Mutex
    summary  RunSummary
    passed   map[string]bool // test_case_id → pass; used for dependency checks
}

func (r *TestRunner) Run(ctx context.Context) error {
    r.runID = newRunID()        // UUID v4
    r.seed  = r.resolveSeed()   // from RunConfig or auto-generated
    r.startTime = time.Now()

    r.logger.Info("AutoTester run started",
        "run_id", r.runID,
        "seed",   r.seed,
        "env",    r.config.Environment,
    )

    // Persist run record immediately
    if err := r.createRunRecord(ctx); err != nil {
        return fmt.Errorf("create run record: %w", err)
    }

    // Apply overall timeout
    runCtx, cancel := context.WithTimeout(ctx, r.config.RunTimeout)
    defer cancel()

    // Execute testers
    if r.config.Parallel {
        r.executeParallelTesters(runCtx)
    } else {
        r.executeSequentialTesters(runCtx)
    }

    // Finalize
    r.finalizeRunRecord(ctx)
    r.printSummary()
    r.writeJSONReport()
    return nil
}
```

#### Sequential vs Parallel Tester Execution

By default, Testers run sequentially. With `--parallel`, each Tester runs in its own goroutine (bounded by `MaxParallel`). Note that **test cases within a single Tester always run sequentially** by default — the Tester itself can choose to parallelize internally, but it must ensure its `Cleanup` is goroutine-safe.

Within a Tester, case execution order is:

1. Cases with no dependencies → run first (respecting their declaration order)
2. Cases with satisfied dependencies → eligible as soon as all deps pass
3. Cases with unsatisfied or failed dependencies → skipped

---

## 11. CLI Entry Point

`server/cmd/autotester/main.go` is the canonical way to run automated tests for an application. It must:

1. Parse command-line flags
2. Load the application configuration (same `config.Config` as the main server)
3. Initialize the database connection (same startup path as the main server)
4. Register all Testers via the registry
5. Build and run the `TestRunner`
6. Exit with code 0 on all-pass, 1 on any failure

```go
package main

import (
    "context"
    "flag"
    "os"
    "strings"
    "time"

    "github.com/chendingplano/shared/go/api/autotesters"
    "github.com/chendingplano/shared/go/api/loggerutil"
    "github.com/dinglind/mirai/server/cmd/config"
    "github.com/dinglind/mirai/server/api/database"
)

func main() {
    purposes    := flag.String("purpose",      "",    "Comma-separated test purposes to run")
    types       := flag.String("type",         "",    "Comma-separated test types to run")
    tags        := flag.String("tags",         "",    "Comma-separated tags to include")
    testerNames := flag.String("tester",       "",    "Comma-separated Tester names to run")
    testIDs     := flag.String("test-id",      "",    "Comma-separated TestCase IDs to run")
    seed        := flag.Int64("seed",          0,     "Random seed (0 = auto-generate)")
    parallel    := flag.Bool("parallel",       false, "Enable parallel Tester execution")
    maxParallel := flag.Int("max-parallel",    4,     "Maximum concurrent Testers")
    retryCount  := flag.Int("retry",           0,     "Retry count for failed cases")
    caseTimeout := flag.Duration("case-timeout", 30*time.Second, "Per-case timeout")
    runTimeout  := flag.Duration("run-timeout",  30*time.Minute, "Overall run timeout")
    stopOnFail  := flag.Bool("stop-on-fail",   false, "Stop on first failure")
    skipCleanup := flag.Bool("skip-cleanup",   false, "Skip Cleanup (for debugging)")
    verbose     := flag.Bool("verbose",        false, "Verbose logging")
    jsonReport  := flag.String("json-report",  "",    "Write JSON report to this file")
    env         := flag.String("env",          "local", "Environment: local|test|staging")
    flag.Parse()

    log := loggerutil.CreateDefaultLogger("AUTO_TESTER")

    // Load config
    cfg, err := config.Load()
    if err != nil {
        log.Error("Config load failed", "error", err)
        os.Exit(2)
    }

    // Safety check: refuse to run against production
    if cfg.Database.Host == cfg.ProductionDBHost {
        log.Error("Refusing to run AutoTester against production database")
        os.Exit(2)
    }

    // Init database
    ctx := context.Background()
    if err := database.InitDB(ctx, cfg, log); err != nil {
        log.Error("Database init failed", "error", err)
        os.Exit(2)
    }
    defer database.CloseDatabase()

    // Register testers
    registerAll(cfg)

    // Build runner
    runner := autotesters.NewTestRunner(
        autotesters.GlobalRegistry.Build(),
        &autotesters.RunConfig{
            Purposes:    split(*purposes),
            Types:       split(*types),
            Tags:        split(*tags),
            TesterNames: split(*testerNames),
            TestIDs:     split(*testIDs),
            Seed:        *seed,
            Parallel:    *parallel,
            MaxParallel: *maxParallel,
            RetryCount:  *retryCount,
            CaseTimeout: *caseTimeout,
            RunTimeout:  *runTimeout,
            StopOnFail:  *stopOnFail,
            SkipCleanup: *skipCleanup,
            Verbose:     *verbose,
            JSONReport:  *jsonReport,
            Environment: *env,
        },
        log,
    )

    if err := runner.Run(ctx); err != nil {
        log.Error("Test run failed", "error", err)
        os.Exit(2)
    }

    if runner.Summary().Failed > 0 || runner.Summary().Errored > 0 {
        os.Exit(1)
    }
    os.Exit(0)
}

func split(s string) []string {
    if s == "" { return nil }
    return strings.Split(s, ",")
}
```

### Typical CLI Invocations

```bash
# Run all registered tests
go run ./server/cmd/autotester/

# Smoke tests only (quick deployment check)
go run ./server/cmd/autotester/ --purpose=smoke

# Integration tests with parallel execution
go run ./server/cmd/autotester/ --type=integration --parallel --max-parallel=8

# Replay a specific random run (for debugging a seed-dependent failure)
go run ./server/cmd/autotester/ --seed=8675309

# Run only one Tester with maximum verbosity, keep test data
go run ./server/cmd/autotester/ --tester=user_tester --verbose --skip-cleanup

# Run specific test case IDs
go run ./server/cmd/autotester/ --test-id=user.create.minimal,user.get.by_id

# CI mode: stop on failure, write JSON report
go run ./server/cmd/autotester/ --stop-on-fail --json-report=/tmp/autotester-report.json
```

---

## 12. Test Selection and Filtering

The runner resolves which Testers and TestCases to run by applying filters in order:

**Tester-level filters** (applied before Prepare is called):

| Flag | Matches when |
|---|---|
| `--tester=foo,bar` | `Tester.Name()` is in the list |
| `--purpose=smoke` | `Tester.Purpose()` is in the list |
| `--type=integration` | `Tester.Type()` is in the list |
| `--tags=critical` | `Tester.Tags()` shares at least one tag with the list |

A Tester is included if it matches **all specified filters** (logical AND). If no filters are specified for a dimension, that dimension is not filtered (all values pass).

**Case-level filters** (applied after GenerateTestCases / GetTestCases):

| Flag | Matches when |
|---|---|
| `--test-id=foo.bar` | `TestCase.ID` is in the list |
| `--purpose=smoke` | `TestCase.Purpose` is in the list (if set; falls back to Tester purpose) |
| `--type=unit` | `TestCase.Type` is in the list (if set; falls back to Tester type) |
| `--tags=edge-case` | `TestCase.Tags` shares at least one tag |

If `--test-id` is specified, all other case-level filters are ignored and only those exact IDs run.

---

## 13. Randomness, Seeding, and Replay

### Seed Resolution

At run start, the runner resolves the seed:

```go
func (r *TestRunner) resolveSeed() int64 {
    if r.config.Seed != 0 {
        return r.config.Seed      // deterministic: user-supplied
    }
    seed := time.Now().UnixNano() // non-deterministic: use current timestamp
    r.logger.Info("Auto-generated random seed", "seed", seed)
    return seed
}
```

The seed is stored in `auto_test_runs.seed`. A single `*rand.Rand` instance is created from this seed and threaded into every Tester that requests it via `BaseTester.SetRand(r *rand.Rand)`.

### Deterministic Replay

To replay a run that produced unexpected results:

```bash
# Find the seed in the logs or database
SELECT seed FROM auto_test_runs WHERE run_id = 'abc123';

# Replay exactly
go run ./server/cmd/autotester/ --seed=8675309
```

Because the seed is fixed, `GenerateTestCases` will produce the same cases in the same order, and the run will reproduce exactly. This is critical for debugging intermittent failures in randomized test suites.

### `RandSource` in BaseTester

```go
// BaseTester provides a seeded rand.Rand for dynamic case generation.
type BaseTester struct {
    // ... other fields ...
    rand *rand.Rand  // set by runner via SetRand before GenerateTestCases is called
}

func (b *BaseTester) SetRand(r *rand.Rand) { b.rand = r }
func (b *BaseTester) Rand() *rand.Rand     { return b.rand }
```

The Tester should always use `b.rand` (or `b.Rand()`) for random generation, never `math/rand` global functions, to keep runs deterministic.

---

## 14. Concurrency Model

### Parallel Testers

With `--parallel`, the runner spawns one goroutine per Tester, bounded by a semaphore:

```go
func (r *TestRunner) executeParallelTesters(ctx context.Context) {
    var wg sync.WaitGroup
    sem := make(chan struct{}, r.config.MaxParallel)

    for _, tester := range r.testers {
        if !r.testerMatches(tester) { continue }
        wg.Add(1)
        sem <- struct{}{}
        go func(t Tester) {
            defer wg.Done()
            defer func() { <-sem }()
            r.executeTester(ctx, t)
        }(tester)
    }
    wg.Wait()
}
```

### Thread Safety Requirements

- `TestRunner.recordResult` acquires `r.mu` before updating the summary counters
- Each Tester must be self-contained; shared state (e.g., global DB handles) must be read-only during test execution. If a Tester needs to write to a shared resource (e.g., insert rows), it must use its own connection or transaction to avoid races with other Testers
- `auto_test_results` inserts are individually atomic (each insert is a separate statement); no cross-Tester transaction is needed

### Cases Within a Tester

Cases within a Tester run sequentially by default. A Tester that explicitly wants parallel case execution should implement it internally, with appropriate locking. Most Testers should not need this — sequential within a Tester is simpler and avoids resource contention.

---

## 15. Test Dependencies and Ordering

Test cases can declare that they depend on the successful completion of other cases in the same Tester. The runner tracks passed case IDs in `r.passed` (a `map[string]bool`) and skips any case whose listed dependencies are not all in that map with `true`.

```go
TestCase{
    ID:           "user.update.name",
    Name:         "Update user name",
    Dependencies: []string{"user.create.minimal"},  // must have passed
    Input:        UpdateNameInput{Name: "New Name"},
    Expected:     ExpectedResult{Success: true},
}
```

A dependency skips (not fails) the dependent case. This is intentional: if the prerequisite failed for an unrelated reason, the downstream cases should not count as failures.

Cross-Tester dependencies are **not supported** at the test-case level. If Tester B conceptually depends on Tester A having run first, declare this at the Tester level by ordering registration sequentially. The runner runs Testers in registration order when not in parallel mode.

---

## 16. Test Data Management and Fixtures

### Principles

1. **Isolation**: test data must not bleed into production data. Use a separate database or a test schema, or tag all test rows with a `test_run_id` column so they can be found and deleted
2. **Cleanup**: `Cleanup()` must remove all rows inserted during the test run. If `--skip-cleanup` is passed, leave them in place with a log message noting the run ID so they can be found
3. **Idempotent Prepare**: `Prepare()` should succeed even if a previous run left orphaned data (e.g., by deleting any rows tagged with an old `test_run_id` first)
4. **Fixture versioning**: SQL fixture files stored in `server/testdata/fixtures/` are managed like migrations — name them `001_users_base.sql`, `002_projects_base.sql`, etc.

### Fixture Loading Helper

```go
// fixtures.go in shared/go/api/autotesters/
func LoadSQLFixtures(ctx context.Context, db *sql.DB, paths ...string) error {
    for _, path := range paths {
        data, err := os.ReadFile(path)
        if err != nil {
            return fmt.Errorf("read fixture %s: %w", path, err)
        }
        if _, err := db.ExecContext(ctx, string(data)); err != nil {
            return fmt.Errorf("execute fixture %s: %w", path, err)
        }
    }
    return nil
}
```

### Using Transactions for Isolation

For unit-style tests that need strong isolation, wrap each test case in a transaction and roll it back in cleanup:

```go
func (t *UserTester) runCreateUser(ctx context.Context, tc TestCase, r *TestResult) {
    tx, err := ApiTypes.PG_DB_Project.BeginTx(ctx, nil)
    if err != nil { r.Error = err.Error(); return }
    defer tx.Rollback() // always roll back; only commit if the test explicitly needs persistence

    input := tc.Input.(CreateUserInput)
    user, err := t.userService.CreateTx(ctx, tx, input)
    if err != nil { r.Error = err.Error(); return }
    r.ActualValue = user
    // Do NOT commit — rollback ensures no residue
}
```

---

## 17. Safety and Environment Isolation

**AutoTester must never run against production.**

The following safeguards are required:

1. **Explicit environment flag**: `--env` must be passed; default is `"local"`. The runner logs the environment at startup and includes it in every DB record
2. **Production guard in `main.go`**: Before initializing the DB, compare the resolved DB host against a known production hostname (from config). If they match, print an error and exit 2
3. **Config segregation**: The autotester command loads the same `config.Config` as the main server, but the test environment's config file must point to the test database. Never commit a config that points to production
4. **Namespace isolation**: When inserting test rows, always include the `run_id` so they can be distinguished and cleaned up. Consider prefixing test UUIDs with `test_` if the schema allows

---

## 18. Error Classification and Reporting

### Error Types

| Error Class | Meaning | DB Status |
|---|---|---|
| **Infrastructure error** | Prepare failed, DB unreachable, panic | `error` |
| **Assertion failure** | SUT returned wrong value or wrong error | `fail` |
| **Expected error** | SUT returned the expected error | `pass` |
| **Timeout** | Case exceeded `CaseTimeout` | `fail` (with `error` message) |
| **Skipped** | Dependency not met, or explicit `SkipReason` | `skip` |

Distinguishing infrastructure errors from assertion failures is important: a burst of `error` status (e.g., DB went down) looks very different from a burst of `fail` (code regression).

### Run Summary

Printed to stdout at the end of every run:

```
AutoTester Run Complete
  Run ID   : a3f8d012-...
  Seed     : 8675309
  Env      : test
  Duration : 4m 32s
  Total    : 247
  Passed   : 241 (97.6%)
  Failed   : 4  (1.6%)
  Skipped  : 1  (0.4%)
  Errored  : 1  (0.4%)

FAILURES:
  [fail]  user.create.duplicate_email      (0.8ms)  "expected error containing 'duplicate' but got: unique constraint violated on users_email_idx"
  [fail]  project.create.missing_client_id (1.1ms)  "expected status 400, got 500"
  ...
```

### JSON Report

When `--json-report=path` is specified, the runner writes a JSON file containing the full `RunSummary` and all `TestResult` entries. This allows CI pipelines to parse and publish results.

---

## 19. CI/CD Integration

AutoTester is designed to run in a non-interactive CI environment:

```yaml
# Example GitHub Actions workflow
name: AutoTester

on:
  push:
    branches: [main, develop]
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:15
        env:
          POSTGRES_PASSWORD: testpass
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
        ports:
          - 5432:5432

    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.21'

      - name: Run AutoTester
        env:
          DB_HOST: localhost
          DB_PORT: 5432
          DB_USER: postgres
          DB_PASSWORD: testpass
          DB_NAME: test_db
        run: |
          go run ./server/cmd/autotester/ \
            --env=test \
            --parallel \
            --max-parallel=4 \
            --json-report=/tmp/autotester-report.json

      - name: Upload test report
        uses: actions/upload-artifact@v4
        if: always()
        with:
          name: autotester-report
          path: /tmp/autotester-report.json
```

---

## 20. Best Practices

### Test Design

1. **Keep tests independent**: Each test case should be able to run in isolation
2. **Use meaningful IDs**: Test case IDs should be descriptive and unique
3. **Document test purposes**: Clearly state what each test validates
4. **Prioritize tests**: Mark critical tests that must pass for deployment
5. **Avoid test interdependencies**: Minimize dependencies between test cases

### Test Data Management

1. **Use fixtures**: Pre-populate test data through fixtures
2. **Clean up after tests**: Remove or roll back test data unless debugging
3. **Isolate test data**: Use unique identifiers or transactions to prevent conflicts
4. **Generate realistic data**: Use realistic test data for meaningful results

### Performance

1. **Enable parallel execution**: Run independent tests concurrently
2. **Set appropriate timeouts**: Prevent hung tests from blocking the run
3. **Use retries sparingly**: Only retry genuinely flaky tests
4. **Profile slow tests**: Identify and optimize bottlenecks

### Maintenance

1. **Keep testers updated**: Update testers when SUT changes
2. **Review failing tests**: Investigate failures promptly
3. **Archive old results**: Implement retention policies for test results
4. **Document known issues**: Track flaky tests and known limitations

---

## 21. Examples

### Complete Example: Database Tester

```go
package autotesters

import (
    "context"
    "database/sql"
    "fmt"
    "strings"
    "time"

    "github.com/chendingplano/shared/go/api/ApiTypes"
    "github.com/chendingplano/shared/go/api/database"
)

// DatabaseTester tests database connection and basic operations.
type DatabaseTester struct {
    BaseTester
    dbConfig *database.Config
    testDB   *sql.DB
}

// NewDatabaseTester creates a new database tester.
func NewDatabaseTester(cfg *database.Config) *DatabaseTester {
    return &DatabaseTester{
        BaseTester: BaseTester{
            name:        "tester_database",
            description: "Tests database connectivity and basic CRUD operations",
            purpose:     "validation",
            testType:    "integration",
            tags:        []string{"database", "core", "critical"},
        },
        dbConfig: cfg,
    }
}

// Prepare establishes a test database connection.
func (t *DatabaseTester) Prepare(ctx context.Context) error {
    var err error
    t.testDB, err = database.NewConnection(ctx, t.dbConfig)
    if err != nil {
        return fmt.Errorf("failed to create test DB connection: %w", err)
    }
    return nil
}

// GenerateTestCases creates dynamic test cases based on configuration.
func (t *DatabaseTester) GenerateTestCases(ctx context.Context) ([]TestCase, error) {
    testCases := []TestCase{
        {
            ID:          "db_conn_001",
            Name:        "Test database connection",
            Description: "Verify that a database connection can be established",
            Input:       nil,
            Expected:    ExpectedResult{Success: true},
            Priority:    PriorityHigh,
        },
        {
            ID:          "db_ping_002",
            Name:        "Test database ping",
            Description: "Verify that the database responds to ping",
            Input:       nil,
            Expected:    ExpectedResult{Success: true},
            Priority:    PriorityHigh,
        },
    }

    // Add random stress test cases if enabled
    if stressEnabled {
        for i := 0; i < 100; i++ {
            testCases = append(testCases, TestCase{
                ID:          fmt.Sprintf("db_stress_%03d", i),
                Name:        fmt.Sprintf("Stress test iteration %d", i),
                Description: "Random query stress test",
                Input:       generateRandomQuery(),
                Expected:    ExpectedResult{Success: true, MaxDuration: 100 * time.Millisecond},
                Priority:    PriorityLow,
            })
        }
    }

    return testCases, nil
}

// RunTestCase executes a single database test case.
func (t *DatabaseTester) RunTestCase(ctx context.Context, tc TestCase) TestResult {
    start := time.Now()
    result := TestResult{
        TestCaseID: tc.ID,
        StartTime:  start,
    }

    switch tc.ID {
    case "db_conn_001":
        result = t.testConnection(ctx, tc, result)
    case "db_ping_002":
        result = t.testPing(ctx, tc, result)
    default:
        if strings.HasPrefix(tc.ID, "db_stress_") {
            result = t.testStress(ctx, tc, result)
        }
    }

    result.EndTime = time.Now()
    result.Duration = result.EndTime.Sub(start)
    return result
}

func (t *DatabaseTester) testConnection(ctx context.Context, tc TestCase, result TestResult) TestResult {
    if t.testDB == nil {
        result.Status = StatusFail
        result.Error = "database connection is nil"
        return result
    }
    result.Status = StatusPass
    result.Message = "Database connection established successfully"
    return result
}

func (t *DatabaseTester) testPing(ctx context.Context, tc TestCase, result TestResult) TestResult {
    if err := t.testDB.PingContext(ctx); err != nil {
        result.Status = StatusFail
        result.Error = fmt.Sprintf("ping failed: %v", err)
        return result
    }
    result.Status = StatusPass
    result.Message = "Database ping successful"
    return result
}

func (t *DatabaseTester) testStress(ctx context.Context, tc TestCase, result TestResult) TestResult {
    query := tc.Input.(string)
    if _, err := t.testDB.ExecContext(ctx, query); err != nil {
        result.Status = StatusFail
        result.Error = fmt.Sprintf("query failed: %v", err)
        return result
    }
    result.Status = StatusPass
    return result
}

// Cleanup closes the test database connection.
func (t *DatabaseTester) Cleanup(ctx context.Context) error {
    if t.testDB != nil {
        return t.testDB.Close()
    }
    return nil
}
```

### Complete Example: User API Tester

```go
package autotesters

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "net/http/httptest"
    "strings"
    "time"
)

// UserAPITester tests user-related API endpoints.
type UserAPITester struct {
    BaseTester
    server *httptest.Server
    client *http.Client
}

func NewUserAPITester() *UserAPITester {
    return &UserAPITester{
        BaseTester: BaseTester{
            name:        "user_api_tester",
            description: "Tests user API endpoints",
            purpose:     "validation",
            testType:    "integration",
            tags:        []string{"api", "user", "critical"},
        },
        client: &http.Client{Timeout: 10 * time.Second},
    }
}

func (t *UserAPITester) Prepare(ctx context.Context) error {
    // Start test server
    router := setupTestRouter()
    t.server = httptest.NewServer(router)
    return nil
}

func (t *UserAPITester) GenerateTestCases(ctx context.Context) ([]TestCase, error) {
    return []TestCase{
        {
            ID:       "user_api_create_001",
            Name:     "Create user via API",
            Input:    map[string]string{"name": "Test User", "email": "test@example.com"},
            Expected: ExpectedResult{Success: true, SideEffects: []string{"user_created"}},
            Priority: PriorityCritical,
        },
        {
            ID:           "user_api_get_002",
            Name:         "Get user via API",
            Dependencies: []string{"user_api_create_001"},
            Input:        map[string]string{"ref": "user_api_create_001.result.id"},
            Expected:     ExpectedResult{Success: true},
            Priority:     PriorityCritical,
        },
        {
            ID:           "user_api_update_003",
            Name:         "Update user via API",
            Dependencies: []string{"user_api_create_001"},
            Input:        map[string]interface{}{
                "ref":  "user_api_create_001.result.id",
                "data": map[string]string{"name": "Updated Name"},
            },
            Expected: ExpectedResult{Success: true},
            Priority: PriorityHigh,
        },
        {
            ID:           "user_api_delete_004",
            Name:         "Delete user via API",
            Dependencies: []string{"user_api_create_001"},
            Input:        map[string]string{"ref": "user_api_create_001.result.id"},
            Expected:     ExpectedResult{Success: true, SideEffects: []string{"user_deleted"}},
            Priority:     PriorityHigh,
        },
    }, nil
}

func (t *UserAPITester) RunTestCase(ctx context.Context, tc TestCase) TestResult {
    result := TestResult{
        TestCaseID: tc.ID,
        StartTime:  time.Now(),
    }

    var err error
    switch tc.ID {
    case "user_api_create_001":
        err = t.testCreateUser(ctx, tc, &result)
    case "user_api_get_002":
        err = t.testGetUser(ctx, tc, &result)
    case "user_api_update_003":
        err = t.testUpdateUser(ctx, tc, &result)
    case "user_api_delete_004":
        err = t.testDeleteUser(ctx, tc, &result)
    }

    if err != nil {
        result.Status = StatusFail
        result.Error = err.Error()
    } else {
        result.Status = StatusPass
    }

    result.EndTime = time.Now()
    result.Duration = result.EndTime.Sub(result.StartTime)
    return result
}

func (t *UserAPITester) testCreateUser(ctx context.Context, tc TestCase, result *TestResult) error {
    input := tc.Input.(map[string]string)
    body, _ := json.Marshal(input)

    resp, err := t.client.Post(t.server.URL+"/api/users", "application/json",
        strings.NewReader(string(body)))
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusCreated {
        return fmt.Errorf("expected status 201, got %d", resp.StatusCode)
    }

    var user User
    if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
        return err
    }

    result.ActualValue = user
    result.SideEffectsObserved = []string{"user_created"}
    return nil
}

// ... implement other test methods similarly

func (t *UserAPITester) Cleanup(ctx context.Context) error {
    if t.server != nil {
        t.server.Close()
    }
    return nil
}
```

---

## Querying Results

```sql
-- Get summary of recent test runs
SELECT
    run_id,
    started_at,
    status,
    passed_count,
    failed_count,
    skipped_count,
    duration_ms
FROM auto_test_runs
ORDER BY started_at DESC
LIMIT 10;

-- Get failed test cases for a specific run
SELECT
    test_case_id,
    tester_name,
    error,
    duration_ms
FROM auto_test_results
WHERE run_id = 'your-run-id' AND status = 'fail';

-- Get detailed logs for a failed test
SELECT
    log_level,
    message,
    context_json,
    logged_at
FROM auto_test_logs
WHERE run_id = 'your-run-id' AND test_case_id = 'failing_test_id'
ORDER BY logged_at;
```

---

## Troubleshooting

### Common Issues

| Issue | Cause | Solution |
|---|---|---|
| Tests fail with "connection refused" | Database not initialized | Call `database.InitDB` before running tests |
| Tests hang indefinitely | Missing timeout or deadlock | Set `--timeout` flag and check for deadlocks |
| Flaky tests | Race conditions or external dependencies | Add retries, isolate tests, use mocks |
| Out of memory | Too many parallel tests | Reduce `--max-parallel` value |
| Missing test results | Database write failed | Check database connectivity and table permissions |

### Debugging Failed Tests

```bash
# Run with verbose logging
go run server/cmd/autotester/main.go --verbose --test-id=failing_test_id

# Run single test to isolate issue
go run server/cmd/autotester/main.go --test-id=specific_test --stop-on-fail

# Skip cleanup to inspect database state
go run server/cmd/autotester/main.go --test-id=failing_test --skip-cleanup
```

### Performance Tuning

If tests are running slowly:

1. **Enable parallel execution**: `--parallel --max-parallel=8`
2. **Reduce test data volume**: Generate fewer dynamic test cases
3. **Optimize database queries**: Add indexes, reduce N+1 queries
4. **Use connection pooling**: Ensure database connections are pooled
5. **Profile test execution**: Add timing logs to identify bottlenecks
