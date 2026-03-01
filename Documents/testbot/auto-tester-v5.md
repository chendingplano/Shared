# AutoTester: Automated Testing Framework (v4)

**Package:** `github.com/chendingplano/shared/go/api/autotesters`
**Source:** [`shared/go/api/autotesters/`](../../go/api/autotesters/)

**Created:** 2026/02/20 by Qwen Code, compiled from auto-tester\*
**Updated:** 2026/02/26 — Tester Catalog and Global Enable/Disable via [[testers]] in testers.toml (v6)

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
9. [Tester Registry](#9-tester-registry)
10. [Tester Packaging](#10-tester-packaging)
   - 10.6 [TOML Configuration Files](#106-toml-configuration-files)
11. [Test Runner (Orchestrator)](#11-test-runner-orchestrator)
12. [CLI Entry Point](#12-cli-entry-point)
13. [Test Selection and Filtering](#13-test-selection-and-filtering)
14. [Randomness, Seeding, and Replay](#14-randomness-seeding-and-replay)
15. [Concurrency Model](#15-concurrency-model)
16. [Test Dependencies and Ordering](#16-test-dependencies-and-ordering)
17. [Test Data Management and Fixtures](#17-test-data-management-and-fixtures)
18. [Safety and Environment Isolation](#18-safety-and-environment-isolation)
19. [Error Classification and Reporting](#19-error-classification-and-reporting)
20. [CI/CD Integration](#20-cicd-integration)
21. [Best Practices](#21-best-practices)
22. [Examples](#22-examples)
23. [Querying Result](#23-querying-results)
24. [Troubleshooting](#24-troubleshooting)
25. [Change Log](#25-change-log)
    - 25.1 [V3 Tester Packaging](#251-v3--20260224-tester-packaging)
    - 25.2 [V4 Configurable Tester Packaging via testers.toml](#252-v4--20260225-configurable-tester-packaging-via-testerstoml)
    - 25.3 [V5 Fonfigurable Tester Packages via testers.toml (v2)](#253-v5--20260225-configurable-tester-packages-via-testerstoml-v2)
    - 25.4 [V6 Tester Catalog and Global Enable/Disable via [[testers]]](#254-v6--20260226-tester-catalog-and-global-enabledisable-via-testers)

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
            ├── config.go             # LoadTOMLConfig, RegisterPackagesFromTOML, LoadAndRegisterTOMLConfigs
            │
            ├── tester_database.go    # Tester: database connectivity & CRUD
            ├── tester_databaseutil.go# Tester: databaseutil package
            ├── tester_auth.go        # Tester: JWT / OAuth auth module
            ├── tester_logger.go      # Tester: loggerutil package
            └── ...
```

### Shared-Library Tester Registration

```
shared/
└── go/
    └── api/
        └── testers/
            ├── registertesters.go    # RegisterTesters(), RegisterPackages(), LoadTOMLPackages()
            ├── testers.toml          # Built-in shared package definitions (smoke, regression, complete)
            ├── testers.example.toml  # Annotated template for project-level testers.toml
            ├── tester_database.go
            ├── tester_databaseutil.go
            └── tester_logger.go
```

### Application-Specific Testers

Application projects normally have autotester
```
myapp/                          (e.g. tax/ or ChenWeb/)
└── server/
    ├── api/
    │   └── apptesters/
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
    PackageName  string        // select testers from a pre-defined TesterPackage
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

`PackageName` is resolved at `Run()` start via `GlobalPackageRegistry`. If both `PackageName` and `TesterNames` are set, `TesterNames` takes precedence (allowing per-run overrides of a package). See [Section 10](#10-tester-packaging) for full details.

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

## 10. Tester Packaging

**Tester Packaging** is a configurable mechanism that lets you define named, reusable collections of testers — called **packages** — and run a specific package by name rather than cherry-picking individual testers every time.

Common use cases:

| Package name | Typical contents | When to run |
|---|---|---|
| `smoke` | database, logger | Pre-deploy sanity check (< 1 min) |
| `regression` | databaseutil, logger | Post-merge regression gate |
| `complete` | all shared testers | Nightly full coverage |
| `nightly` | all app + shared testers | Scheduled CI pipeline |

---

### 10.1 Core Types (`autotester/package.go`)

```go
// TesterPackage is a named, ordered collection of tester names.
type TesterPackage struct {
    Name        string   // unique key, e.g. "smoke"
    Description string   // human-readable explanation
    TesterNames []string // ordered list of tester Name()s to include
}

// TesterPackageRegistry holds named packages.
type TesterPackageRegistry struct { /* thread-safe */ }

// GlobalPackageRegistry is the singleton used by default.
var GlobalPackageRegistry = &TesterPackageRegistry{...}
```

**Key methods on `TesterPackageRegistry`:**

| Method | Description |
|---|---|
| `Register(pkg)` | Add a package; panics on duplicate name |
| `Upsert(pkg)` | Add or replace a package; never panics — used by TOML loader |
| `Get(name)` | Return a package by name |
| `Has(name)` | True if the package is registered |
| `Names()` | Return all registered package names |
| `Build(name, registry)` | Instantiate testers for the package using a `TesterRegistry` |
| `Clear()` | Remove all packages (for testing) |

**Package-level convenience functions:**

```go
// RegisterPackage adds to GlobalPackageRegistry.
func RegisterPackage(pkg *TesterPackage)

// BuildPackage uses GlobalPackageRegistry + GlobalRegistry.
func BuildPackage(packageName string) ([]Tester, error)
```

---

### 10.2 Defining Packages

Packages are defined at application startup, **after** all individual testers have been registered, so that every name in `TesterNames` already exists in `GlobalRegistry`.

In the shared library (`shared/go/api/testers/registertesters.go`):

```go
func RegisterPackages() {
    autotester.GlobalPackageRegistry.Register(&autotester.TesterPackage{
        Name:        "smoke",
        Description: "Fast sanity check: database connectivity and logger",
        TesterNames: []string{"tester_database", "tester_logger"},
    })
    autotester.GlobalPackageRegistry.Register(&autotester.TesterPackage{
        Name:        "regression",
        Description: "Core regression suite: database utilities and logger",
        TesterNames: []string{"tester_databaseutil", "tester_logger"},
    })
    autotester.GlobalPackageRegistry.Register(&autotester.TesterPackage{
        Name:        "complete",
        Description: "Full shared-library suite: all three shared testers",
        TesterNames: []string{"tester_database", "tester_databaseutil", "tester_logger"},
    })
}
```

In the application (`server/cmd/autotester/registry.go`), call the shared registration first, then extend with app-specific packages:

```go
func registerAll(cfg *config.Config) {
    // 1. Register individual testers (shared + app)
    sharedtesters.RegisterTesters()
    autotesters.GlobalRegistry.Register("user_tester", func() autotesters.Tester {
        return apptests.NewUserTester(cfg)
    })

    // 2. Register shared packages (hard-coded defaults)
    sharedtesters.RegisterPackages()

    // 3. Load TOML overrides (shared + project-level testers.toml)
    if err := sharedtesters.LoadTOMLPackages(sharedDir, projectRoot); err != nil {
        log.Fatal("load testers.toml:", err)
    }

    // 4. Register app-specific packages programmatically (may include shared testers)
    autotesters.RegisterPackage(&autotesters.TesterPackage{
        Name:        "nightly",
        Description: "Full nightly: all shared + all app testers",
        TesterNames: []string{
            "tester_database", "tester_databaseutil", "tester_logger",
            "user_tester",
        },
    })
    autotesters.RegisterPackage(&autotesters.TesterPackage{
        Name:        "user-smoke",
        Description: "User service smoke: connectivity + user API",
        TesterNames: []string{"tester_database", "user_tester"},
    })
}
```

> **Rule:** Always call `RegisterTesters()` (and any app-specific `Register` calls) before `RegisterPackages()`. Registering a package whose `TesterNames` reference an unregistered tester will return an error at `Build` time.

**Alternative: define packages in `testers.toml`**

Instead of (or in addition to) calling `RegisterPackages()` and `RegisterPackage()` in Go code, you can declare packages in plain TOML files. Use `LoadTOMLPackages` (from `testers/registertesters.go`) or the lower-level `autotester.LoadAndRegisterTOMLConfigs` to load them at startup. Packages defined in TOML override any programmatically registered package with the same name. See [§10.6](#106-toml-configuration-files) for the full reference.

---

### 10.3 Running a Package

**Option A — via `RunConfig.PackageName` (recommended for CLI)**

Pass a `PackageName` in `RunConfig`. At the start of `runner.Run()`, the runner resolves the package to `TesterNames` using `GlobalPackageRegistry`. The full `GlobalRegistry.Build()` tester list is still passed to `NewTestRunner`, and the resolved names act as a filter.

```go
runner := autotesters.NewTestRunner(
    autotesters.GlobalRegistry.Build(),
    &autotesters.RunConfig{
        PackageName: "smoke",
        Environment: "local",
    },
    log,
)
runner.Run(ctx)
```

**Precedence rule:** If both `PackageName` and `TesterNames` are set, `TesterNames` takes precedence. This allows a one-off override without redefining the package.

**Option B — via `BuildPackage` (pre-flight construction)**

Use `autotesters.BuildPackage` to instantiate only the package's testers upfront. This is useful when you do not want to build all testers:

```go
testers, err := autotesters.BuildPackage("smoke")
if err != nil {
    log.Fatal("unknown package", err)
}
runner := autotesters.NewTestRunner(testers, config, log)
runner.Run(ctx)
```

---

### 10.4 CLI Integration

Add a `--package` flag to `server/cmd/autotester/main.go`:

```go
pkg := flag.String("package", "", "Run a named tester package (e.g. smoke, complete, regression)")
```

Pass it to `RunConfig`:

```go
runner := autotesters.NewTestRunner(
    autotesters.GlobalRegistry.Build(),
    &autotesters.RunConfig{
        PackageName: *pkg,
        // ...other flags...
    },
    log,
)
```

Example invocations:

```bash
# Run the smoke package (pre-deploy gate)
go run ./server/cmd/autotester/ --package=smoke

# Run the complete package in parallel (nightly)
go run ./server/cmd/autotester/ --package=complete --parallel --max-parallel=8

# Run the regression package and stop on first failure
go run ./server/cmd/autotester/ --package=regression --stop-on-fail

# Override a package with a specific tester (TesterNames takes precedence)
go run ./server/cmd/autotester/ --package=complete --tester=user_tester
```

---

### 10.5 Package Design Guidelines

- **Keep packages small and focused.** A `smoke` package should complete in under a minute.
- **Name packages after their trigger**, not their contents: `smoke`, `regression`, `nightly`, `ci`.
- **A tester may appear in multiple packages** — packages reference names, not instances.
- **Never share mutable state between testers** in a package. Each tester is independently instantiated.
- **Document package contents** in the `Description` field so future maintainers can understand the scope.
- **Validate at startup.** Call `Build` (or verify `Has`) during initialization to catch missing testers early.
- **Prefer TOML for user-facing packages.** Use `testers.toml` for packages that users or operators might want to adjust without touching Go source. Reserve `RegisterPackage()` in Go code for packages whose contents are derived programmatically.

---

### 10.6 TOML Configuration Files

Tester packages can be declared in `testers.toml` files rather than hard-coded in Go. This lets operators customise test suites without recompiling the binary and provides a clear, human-readable record of which testers belong to each package.

#### File Layout

| Location | Purpose |
|---|---|
| `shared/go/api/testers/testers.toml` | Shared-library baseline packages (smoke, regression, complete) |
| `<project-root>/testers.toml` | Project-specific packages; overrides shared packages with the same name |

Both files are **optional**. A missing file is silently skipped.

#### Format

```toml
# testers.toml
[[packages]]
name        = "smoke"
description = "Fast pre-deploy sanity check"
testers     = [
  { name = "tester_database", enable = true, num_tcs = 20, seconds = 60 },
  { name = "tester_logger", enable = true, num_tcs = 30, seconds = 120 }
]

[[packages]]
name        = "nightly"
description = "Full nightly regression: all shared + app testers"
testers     = [
  { name = "tester_database", enable = true, num_tcs = 0, seconds = 0 },
  { name = "tester_databaseutil", enable = true, num_tcs = 0, seconds = 0 },
  { name = "tester_logger", enable = true, num_tcs = 0, seconds = 0 },
  { name = "app_user_tester", enable = true, num_tcs = 0, seconds = 0 },
]
```

Each `[[packages]]` entry has three fields:

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Unique key; must match a value you can pass to `--package` |
| `description` | string | no | Human-readable summary |
| `testers` | array | yes | Array of tester configurations with `name`, `enable`, `num_tcs`, `seconds` |

**Tester Configuration Fields:**

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Tester name (must match a registered tester) |
| `enable` | bool | yes | If `true`, tester is included in the package; if `false`, excluded |
| `num_tcs` | int | no | Max test cases to run (0 = no limit) |
| `seconds` | int | no | Max execution time in seconds (0 = no limit) |

#### Override Semantics

Loading a later file **upserts** (replaces) any package with the same name registered earlier — whether from `RegisterPackages()`, `RegisterPackage()`, or a previous TOML file. The effective load order is:

1. Programmatic `RegisterPackages()` (if called)
2. Programmatic `RegisterPackage()` calls
3. `shared/go/api/testers/testers.toml`
4. `<project-root>/testers.toml`

Each layer can override the previous one by redefining the same `name`.

#### API Reference

**`autotester.LoadTOMLConfig(path string) (*TOMLConfig, error)`**
Parses a single `testers.toml` and returns the struct. A missing file returns an empty config (no error).

**`autotester.RegisterPackagesFromTOML(path string) error`**
Loads a single file and upserts every package it defines into `GlobalPackageRegistry`.

**`autotester.LoadAndRegisterTOMLConfigs(paths ...string) error`**
Processes multiple files in order; missing files are silently skipped.

**`sharedtesters.LoadTOMLPackages(sharedDir, projectRoot string) error`**
Convenience wrapper that calls `LoadAndRegisterTOMLConfigs` with the two conventional paths:
- `filepath.Join(sharedDir, "testers.toml")`
- `filepath.Join(projectRoot, "testers.toml")`

#### Usage in `registerAll`

**With programmatic defaults as fallback (recommended for existing projects):**

```go
func registerAll(cfg *config.Config) {
    // 1. Register individual testers
    sharedtesters.RegisterTesters()
    autotesters.GlobalRegistry.Register("app_user_tester", func() autotesters.Tester {
        return apptests.NewUserTester(cfg)
    })

    // 2. Hard-coded shared defaults (optional fallback)
    sharedtesters.RegisterPackages()

    // 3. TOML overrides — replace or extend the defaults above
    if err := sharedtesters.LoadTOMLPackages(sharedDir, projectRoot); err != nil {
        log.Fatal("load testers.toml:", err)
    }
}
```

**Pure TOML (for new projects):**

```go
func registerAll(cfg *config.Config) {
    sharedtesters.RegisterTesters()
    autotesters.GlobalRegistry.Register("app_user_tester", func() autotesters.Tester {
        return apptests.NewUserTester(cfg)
    })

    if err := sharedtesters.LoadTOMLPackages(sharedDir, projectRoot); err != nil {
        log.Fatal("load testers.toml:", err)
    }
}
```

#### Project-Level Template

Copy `shared/go/api/testers/testers.example.toml` to your project root and customise it:

```toml
# testers.toml — project-specific tester package configuration
# Packages defined here override any package with the same name from
# shared/go/api/testers/testers.toml or from RegisterPackages().

# Override the built-in "smoke" package
[[packages]]
name        = "smoke"
description = "Project smoke: DB + logger + app health check"
testers     = [
  { name = "tester_database", enable = true, num_tcs = 20, seconds = 60 },
  { name = "tester_logger", enable = true, num_tcs = 30, seconds = 120 },
  { name = "app_health_tester", enable = true, num_tcs = 10, seconds = 30 }
]

# Add a project-specific nightly suite
[[packages]]
name        = "nightly"
description = "Full nightly regression: all shared + app testers"
testers     = [
  { name = "tester_database", enable = true, num_tcs = 0, seconds = 0 },
  { name = "tester_databaseutil", enable = true, num_tcs = 0, seconds = 0 },
  { name = "tester_logger", enable = true, num_tcs = 0, seconds = 0 },
  { name = "app_user_tester", enable = true, num_tcs = 0, seconds = 0 },
  { name = "app_billing_tester", enable = true, num_tcs = 0, seconds = 0 },
]

# Example: temporarily disable a tester
[[packages]]
name        = "quick"
description = "Quick check: database only"
testers     = [
  { name = "tester_database", enable = true, num_tcs = 10, seconds = 30 },
  { name = "tester_logger", enable = false, num_tcs = 0, seconds = 0 }
]
```

---

### 10.7 Configuration, Loading, and Execution Flow

This section explains the complete flow from configuration to execution: how testers and packages are defined, loaded, filtered, and run.

#### 10.7.1 Configure Testers and Packages

Testers and packages are configured in `testers.toml` files. There are two levels:

| Level | Path | Purpose |
|---|---|---|
| **Shared** | `shared/go/api/testers/testers.toml` | Baseline packages for all projects |
| **Project** | `<project-root>/testers.toml` | Project-specific packages; overrides shared |

**Package Structure:**

```toml
[[packages]]
name        = "smoke"
description = "Fast pre-deploy sanity check"
testers = [
    { name = "tester_database", enable = true, num_tcs = 20, seconds = 60 },
    { name = "tester_logger", enable = false, num_tcs = 30, seconds = 120 }
]
```

**Fields:**

| Field | Type | Description |
|---|---|---|
| `name` | string | Unique package identifier (e.g., `"smoke"`, `"regression"`) |
| `description` | string | Human-readable summary |
| `testers` | array | List of tester configurations |

**Tester Configuration Fields:**

| Field | Type | Description |
|---|---|---|
| `name` | string | Must match a registered tester name |
| `enable` | bool | **Enforced** — if `false`, tester is excluded from the package |
| `num_tcs` | int | Max test cases to run (0 = no limit) |
| `seconds` | int | Max execution time in seconds (0 = no limit) |

> **Note:** The `enable` field at the **package level** is ignored (packages are always loaded). The `enable` field at the **tester level** is enforced: only testers with `enable = true` are included in the package.

#### 10.7.2 Load the Configuration

Configuration loading happens in two phases:

**Phase 1: Register Testers**

Call `RegisterTesters()` to register all tester factories in `GlobalRegistry`:

```go
// In main.go or registerAll()
sharedtesters.RegisterTesters()
// Register app-specific testers
autotester.GlobalRegistry.Register("app_user_tester", func() autotester.Tester {
    return NewUserTester(cfg)
})
```

This step makes testers available by name but does **not** select which ones to run.
Registering testers is hard coded, such as:
```go
func RegisterTesters() {
	autotester.GlobalRegistry.Register("tester_database", func() autotester.Tester {
		return NewDatabaseTester(nil) // DB config will be set in Prepare
	})
	autotester.GlobalRegistry.Register("tester_databaseutil", func() autotester.Tester {
		return NewDatabaseUtilTester()
	})
	autotester.GlobalRegistry.Register("tester_logger", func() autotester.Tester {
		return NewLoggerTester()
	})
	autotester.GlobalRegistry.Register("tester_migration", func() autotester.Tester {
		return tester_migration.NewMigrationTester(nil)
	})
}
```

**Phase 2: Load Packages from TOML**

Call `LoadTOMLPackages()` to parse `testers.toml` files and upsert packages into `GlobalPackageRegistry`:

```go
// In main.go or registerAll()
sharedDir := filepath.Join(wd, "api", "testers")
projectRoot := "/path/to/project"

if err := sharedtesters.LoadTOMLPackages(sharedDir, projectRoot); err != nil {
    log.Fatal("load testers.toml:", err)
}
```

**Load Order:**
1. `sharedDir/testers.toml` — baseline packages
2. `projectRoot/testers.toml` — project overrides

A package name defined in a later file **replaces** the same name from an earlier file (upsert semantics).

**What Gets Loaded:**
- Package metadata (name, description)
- Ordered list of **enabled** tester names for each package (testers with `enable = false` are excluded)
- Tester-level settings (num_tcs, seconds)

> **Important:** Loading filters testers by `enable = true`. Only enabled testers are included in the package's `TesterNames`. The package-level `enable` field is ignored.

#### 10.7.3 Process the Configuration

Package resolution happens at **runtime** when `TestRunner.Run()` is called:

```go
// In main.go
runConfig := &autotester.RunConfig{
    PackageName: *packageFlag,      // e.g., "smoke"
    TesterNames: split(*testerNames), // e.g., []string{"tester_database"}
    // ... other flags
}

runner := autotester.NewTestRunner(
    autotester.GlobalRegistry.Build(), // ALL registered testers
    runConfig,
    logger,
)
```

**Resolution Logic (in `TestRunner.Run`):**

```go
// Line 85-97 in runner.go
if r.config.PackageName != "" && len(r.config.TesterNames) == 0 {
    pkg, ok := GlobalPackageRegistry.Get(r.config.PackageName)
    if !ok {
        return fmt.Errorf("package %q not found", r.config.PackageName)
    }
    r.config.TesterNames = pkg.TesterNames
    r.logger.Info("Resolved tester package",
        "package", r.config.PackageName,
        "testers", strings.Join(r.config.TesterNames, ", "),
    )
}
```

**Priority:**
1. `TesterNames` (explicit `--tester` flags) — **highest priority**
2. `PackageName` (via `--package` flag) — resolved to `TesterNames` (already filtered by `enable`)
3. No filter — run **all** registered testers

> **Note:** By the time resolution happens, the package's `TesterNames` has already been filtered to include only enabled testers. The package-level `enable` field is ignored.

#### 10.7.4 Run Tester Packages

The autotester runs packages based on **explicit CLI selection**:

| CLI Flags | Behavior |
|---|---|
| `--package=smoke` | Run **enabled** testers listed in the `smoke` package |
| `--package=regression` | Run **enabled** testers listed in the `regression` package |
| `--tester=tester_database` | Run only `tester_database` (ignores package) |
| No flags | Run **all** registered testers |

**To run a specific package:**

Specify the package via `--package`. All testers in that package with `enable = true` will run.

**Example:**
```toml
[[packages]]
name = "smoke"
testers = [
    { name = "tester_database", enable = true, ... },
    { name = "tester_logger", enable = false, ... }
]
```

```bash
# Run the smoke package - only tester_database runs (tester_logger is disabled)
go run ./cmd/autotester --package=smoke
```

#### 10.7.5 For Each Enabled Package, Run Testers (Only Enabled Ones)

**Current Behavior:**

When a package is selected via `--package`, only testers with `enable = true` in that package's `testers` array are instantiated and run. Testers with `enable = false` are **excluded at load time**.

**Example:**
```toml
[[packages]]
name = "smoke"
testers = [
    { name = "tester_database", enable = true, ... },
    { name = "tester_logger", enable = false, ... },   # Excluded at load time
    { name = "tester_migration", enable = true, ... }
]
```

```bash
# This runs only tester_database and tester_migration
# tester_logger is excluded because enable = false
go run ./cmd/autotester --package=smoke
```

**To temporarily disable a tester:**

Set `enable = false` in the TOML file. The tester remains in the configuration but is not included in the package.

```toml
# Temporarily disable tester_logger for debugging
testers = [
    { name = "tester_database", enable = true, ... },
    { name = "tester_logger", enable = false, ... },
]
```

**To override at runtime:**

Use `--tester` flags to explicitly select testers, bypassing the package's `enable` settings:

```bash
# Override: run only tester_database (ignores package settings)
go run ./cmd/autotester --package=smoke \
    --tester=tester_database
```

#### 10.7.6 Summary: Configuration-to-Execution Pipeline

```
┌─────────────────────────────────────────────────────────────────┐
│ 1. testers.toml (Shared + Project)                              │
│    [[packages]]                                                  │
│      name = "smoke"                                              │
│      testers = [{name="tester_database", enable=true, ...}]      │
└──────────────────────┬──────────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────────┐
│ 2. LoadTOMLPackages(sharedDir, projectRoot)                     │
│    - Parse TOML files                                           │
│    - Upsert packages into GlobalPackageRegistry                 │
│    - Store: Package.Name, Description, TesterNames              │
│    - Filter: Only testers with enable=true are included         │
└──────────────────────┬──────────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────────┐
│ 3. TestRunner.Run()                                             │
│    - Resolve PackageName → TesterNames                          │
│    - Apply CLI filters (--package, --tester, --purpose, etc.)   │
│    - (TesterNames already filtered by enable at load time)      │
└──────────────────────┬──────────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────────┐
│ 4. executeTester() for each matching tester                     │
│    - Prepare()                                                  │
│    - GenerateTestCases() / GetTestCases()                       │
│    - RunTestCase() for each case                                │
│    - Cleanup()                                                  │
└─────────────────────────────────────────────────────────────────┘
```

**Key Points:**
- **Configuration** (`testers.toml`) defines packages and their tester memberships with `enable` flags
- **Loading** (`LoadTOMLPackages`) filters testers by `enable = true` when building `TesterNames`
- **Resolution** (`TestRunner.Run`) selects testers based on CLI flags; package tester lists are pre-filtered
- **Execution** (`executeTester`) runs all selected testers

**Package-level `enable`:** Ignored (packages are always loaded regardless of this field)

**Tester-level `enable`:** Enforced (only testers with `enable = true` are included in the package)

---

## 11. Test Runner (Orchestrator)

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

## 12. CLI Entry Point

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
    // Command-line flags (see table below for descriptions)
    purposes    := flag.String("purpose",      "",    "Comma-separated test purposes to run")
    types       := flag.String("type",         "",    "Comma-separated test types to run")
    tags        := flag.String("tags",         "",    "Comma-separated tags to include")
    testerNames := flag.String("tester",       "",    "Comma-separated Tester names to run")
    testIDs     := flag.String("test-id",      "",    "Comma-separated TestCase IDs to run")
    pkg         := flag.String("package",      "",    "Run a named tester package")
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
            PackageName: *pkg,
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

### Command-Line Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--purpose` | string | `""` | Comma-separated test purposes to run (e.g., `"smoke,regression"`) |
| `--type` | string | `""` | Comma-separated test types to run (e.g., `"unit,integration,e2e"`) |
| `--tags` | string | `""` | Comma-separated tags to include |
| `--tester` | string | `""` | Comma-separated tester names to run (overrides `--package`) |
| `--test-id` | string | `""` | Comma-separated test case IDs to run |
| `--package` | string | `""` | Run a named tester package (e.g., `smoke`, `complete`, `regression`) |
| `--seed` | int64 | `0` | Random seed (`0` = auto-generate and log) |
| `--parallel` | bool | `false` | Enable parallel tester execution |
| `--max-parallel` | int | `4` | Maximum concurrent testers (when `--parallel` is enabled) |
| `--retry` | int | `0` | Retry count for failed test cases |
| `--case-timeout` | duration | `30s` | Per-test-case timeout |
| `--run-timeout` | duration | `30m` | Overall run timeout |
| `--stop-on-fail` | bool | `false` | Stop on first failure |
| `--skip-cleanup` | bool | `false` | Skip cleanup (for post-mortem debugging) |
| `--verbose` | bool | `false` | Enable verbose (DEBUG-level) logging |
| `--json-report` | string | `""` | Write JSON report to this file path |
| `--env` | string | `"local"` | Environment name (`local`, `test`, `staging`) |

### Typical CLI Invocations

```bash
# Run all registered tests
go run ./server/cmd/autotester/

# Run a named package (fastest way to run a curated suite)
go run ./server/cmd/autotester/ --package=smoke
go run ./server/cmd/autotester/ --package=regression
go run ./server/cmd/autotester/ --package=complete --parallel --max-parallel=8

# Smoke tests only via purpose filter (without a package)
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
go run ./server/cmd/autotester/ --package=regression --stop-on-fail --json-report=/tmp/autotester-report.json
```

---

## 13. Test Selection and Filtering

The runner resolves which Testers and TestCases to run by applying filters in order:

**Tester-level filters** (applied before Prepare is called):

| Flag | Matches when |
|---|---|
| `--package=smoke` | Resolves to a `TesterNames` list from `GlobalPackageRegistry` (see §10) |
| `--tester=foo,bar` | `Tester.Name()` is in the list; **overrides** `--package` when both are set |
| `--purpose=smoke` | `Tester.Purpose()` is in the list |
| `--type=integration` | `Tester.Type()` is in the list |
| `--tags=critical` | `Tester.Tags()` shares at least one tag with the list |

**Filter resolution order:**
1. `--package` is resolved to a list of tester names via `GlobalPackageRegistry` at run start.
2. If `--tester` is also provided, it takes precedence and the package resolution is skipped.
3. Remaining dimension filters (`--purpose`, `--type`, `--tags`) are applied on top.

A Tester is included if it passes **all active filters** (logical AND). If no filters are specified for a dimension, that dimension is not filtered (all values pass).

**Case-level filters** (applied after GenerateTestCases / GetTestCases):

| Flag | Matches when |
|---|---|
| `--test-id=foo.bar` | `TestCase.ID` is in the list |
| `--purpose=smoke` | `TestCase.Purpose` is in the list (if set; falls back to Tester purpose) |
| `--type=unit` | `TestCase.Type` is in the list (if set; falls back to Tester type) |
| `--tags=edge-case` | `TestCase.Tags` shares at least one tag |

If `--test-id` is specified, all other case-level filters are ignored and only those exact IDs run.

---

## 14. Randomness, Seeding, and Replay

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

## 15. Concurrency Model

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

## 16. Test Dependencies and Ordering

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

## 17. Test Data Management and Fixtures

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

## 18. Safety and Environment Isolation

**AutoTester must never run against production.**

The following safeguards are required:

1. **Explicit environment flag**: `--env` must be passed; default is `"local"`. The runner logs the environment at startup and includes it in every DB record
2. **Production guard in `main.go`**: Before initializing the DB, compare the resolved DB host against a known production hostname (from config). If they match, print an error and exit 2
3. **Config segregation**: The autotester command loads the same `config.Config` as the main server, but the test environment's config file must point to the test database. Never commit a config that points to production
4. **Namespace isolation**: When inserting test rows, always include the `run_id` so they can be distinguished and cleaned up. Consider prefixing test UUIDs with `test_` if the schema allows

---

## 19. Error Classification and Reporting

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

## 20. CI/CD Integration

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

## 21. Best Practices

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

## 22. Examples

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

## 23 Querying Results

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

## 24 Troubleshooting

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

---

## 25 Change Log

### 25.1 v3 — 2026/02/24: Tester Packaging

**Feature: Tester Packaging** — configurable, named suites of testers.

#### Problem

`RegisterTesters()` hard-coded all testers into a single flat list. Every run executed
all testers unless individual names were passed via `--tester`. There was no way to define
a reusable "smoke", "regression", or "nightly" grouping without repeating the same flag
combinations at every call site.

#### Solution

Introduced a **Tester Packaging** mechanism consisting of:

| Component | File | Description |
|---|---|---|
| `TesterPackage` | `autotester/package.go` | Named, ordered list of tester names |
| `TesterPackageRegistry` | `autotester/package.go` | Thread-safe registry of packages |
| `GlobalPackageRegistry` | `autotester/package.go` | Singleton package registry |
| `RegisterPackage` / `BuildPackage` | `autotester/package.go` | Convenience package-level helpers |
| `RunConfig.PackageName` | `autotester/testrun.go` | New field: selects a package at run time |
| Package resolution in `Run()` | `autotester/runner.go` | Resolves `PackageName` → `TesterNames` |
| `RegisterPackages()` | `testers/registertesters.go` | Defines built-in shared packages |

#### Built-in Shared Packages

| Package | Testers | Purpose |
|---|---|---|
| `smoke` | `tester_database`, `tester_logger` | Fast pre-deploy sanity check |
| `regression` | `tester_databaseutil`, `tester_logger` | Core regression gate |
| `complete` | `tester_database`, `tester_databaseutil`, `tester_logger` | Full shared-library coverage |

#### Breaking Changes

None. All existing code continues to work unchanged:
- `RegisterTesters()` signature is unchanged.
- `RegisterPackages()` is a new opt-in function; not calling it means no packages are registered,
  and `--package` will return an error only if used.
- `RunConfig.PackageName` defaults to `""` (no-op), preserving all existing behavior.

#### Migration Guide

**Step 1.** Call `RegisterPackages()` after `RegisterTesters()` in your application's `registerAll`:

```go
func registerAll(cfg *config.Config) {
    sharedtesters.RegisterTesters()
    // ... app-specific Register calls ...

    sharedtesters.RegisterPackages()          // ← add this
    // ... app-specific RegisterPackage calls ...
}
```

**Step 2.** Add the `--package` flag to your `main.go`:

```go
pkg := flag.String("package", "", "Run a named tester package (e.g. smoke, complete, regression)")
// ...
&autotesters.RunConfig{
    PackageName: *pkg,
    // ... existing fields ...
}
```

**Step 3.** (Optional) Define application-specific packages:

```go
autotesters.RegisterPackage(&autotesters.TesterPackage{
    Name:        "nightly",
    Description: "Nightly: all shared + app testers",
    TesterNames: []string{"tester_database", "tester_databaseutil", "tester_logger", "user_tester"},
})
```

#### Files Changed

| File | Change |
|---|---|
| `shared/go/api/autotester/package.go` | **New** — `TesterPackage`, `TesterPackageRegistry`, `GlobalPackageRegistry`, helpers |
| `shared/go/api/autotester/testrun.go` | Added `PackageName string` field to `RunConfig` |
| `shared/go/api/autotester/runner.go` | `Run()` resolves `PackageName` → `TesterNames` at start |
| `shared/go/api/testers/registertesters.go` | Added `RegisterPackages()` with smoke/regression/complete |

### 25.2 v4 — 2026/02/25: Configurable Tester Packaging via testers.toml

**Feature: TOML-based tester package configuration** — packages can now be defined and customised in plain TOML files without recompiling.

#### Problem

All tester packages were defined exclusively in Go source (`RegisterPackages()` / `RegisterPackage()`). Operators who wanted to adjust which testers belonged to a package — or add a new package like `nightly` — had to modify Go code and rebuild the binary.

#### Solution

Introduced a TOML-based configuration layer on top of the existing programmatic registration:

| Component | File | Description |
|---|---|---|
| `TOMLConfig` / `PackageConfig` | `autotester/config.go` | TOML-deserializable structs for `testers.toml` |
| `LoadTOMLConfig` | `autotester/config.go` | Parse a single `testers.toml`; missing file is a no-op |
| `RegisterPackagesFromTOML` | `autotester/config.go` | Load a file and upsert its packages into `GlobalPackageRegistry` |
| `LoadAndRegisterTOMLConfigs` | `autotester/config.go` | Process multiple files in order |
| `TesterPackageRegistry.Upsert` | `autotester/package.go` | Register-or-replace without panic; used by the TOML loader |
| `LoadTOMLPackages` | `testers/registertesters.go` | Convenience wrapper for the two conventional paths |
| `testers/testers.toml` | `testers/testers.toml` | Built-in shared package definitions (smoke, regression, complete) |
| `testers/testers.example.toml` | `testers/testers.example.toml` | Annotated template for project-level files |

#### Breaking Changes

None. All existing code continues to work unchanged:
- `RegisterPackages()` and `RegisterPackage()` signatures are unchanged.
- `TesterPackageRegistry.Register()` still panics on duplicate; the new `Upsert` is additive.
- Not calling `LoadTOMLPackages` / `LoadAndRegisterTOMLConfigs` means no TOML is loaded, preserving all existing behaviour.

#### Migration Guide

**Option A — TOML overrides on top of programmatic defaults (recommended for existing projects):**

```go
func registerAll(cfg *config.Config) {
    sharedtesters.RegisterTesters()
    // ... app-specific Register calls ...

    sharedtesters.RegisterPackages()                                    // keep existing defaults
    sharedtesters.LoadTOMLPackages(sharedDir, projectRoot)              // ← add this
}
```

Place a `testers.toml` at your project root (copy from `testers.example.toml`) and define only the packages you want to customise or add.

**Option B — Pure TOML (for new projects):**

```go
func registerAll(cfg *config.Config) {
    sharedtesters.RegisterTesters()
    // ... app-specific Register calls ...

    sharedtesters.LoadTOMLPackages(sharedDir, projectRoot)
}
```

The shared-library defaults are read from `shared/go/api/testers/testers.toml`; project overrides come from `<project-root>/testers.toml`.

#### Files Changed

| File | Change |
|---|---|
| `shared/go/api/autotester/config.go` | **New** — `TOMLConfig`, `PackageConfig`, `LoadTOMLConfig`, `RegisterPackagesFromTOML`, `LoadAndRegisterTOMLConfigs` |
| `shared/go/api/autotester/package.go` | Added `TesterPackageRegistry.Upsert` |
| `shared/go/api/testers/registertesters.go` | Added `LoadTOMLPackages(sharedDir, projectRoot string) error` |
| `shared/go/api/testers/testers.toml` | **New** — shared-library built-in package definitions |
| `shared/go/api/testers/testers.example.toml` | **New** — annotated template for project-level `testers.toml` |
| `shared/go/go.mod` | Promoted `github.com/pelletier/go-toml/v2` from indirect to direct dependency |

---

### 25.3 v5 — 2026/02/25: Configurable Tester Packages via testers.toml (v2)

**Feature: TOML-Driven Tester Registration with Per-Tester Configuration** — complete separation of tester registration from package configuration, with fine-grained control over tester execution.

#### Problem

The v4 implementation had several limitations:

1. **Hard-coded package registration**: `RegisterPackages()` defined packages in Go code, requiring recompilation to change package composition.

2. **No per-tester execution control**: Packages defined only which testers to run, not how to run them. There was no way to:
   - Enable/disable individual testers within a package
   - Limit the number of test cases per tester
   - Set time limits per tester

3. **No shared autotester**: Only application-specific autotesters existed (e.g., `tax/server/cmd/autotester`). There was no standalone autotester for testing the shared library in isolation.

4. **Mixed concerns**: Tester registration (which testers exist) was coupled with package definition (which testers to run together).

#### Solution

Introduced a comprehensive TOML-driven configuration system:

**1. New TOML Format with Per-Tester Configuration**

The `testers.toml` format now supports fine-grained tester control:

```toml
[[packages]]
name = "smoke"
description = "Fast sanity check"
enable = true
testers = [
    { name = "tester_database", enable = true, num_tcs = 20, seconds = 60 },
    { name = "tester_logger", enable = true, num_tcs = 30, seconds = 120 }
]
```

Each tester configuration supports:
- `name`: Tester identifier (must match a registered tester)
- `enable`: Whether to run this tester in this package
- `num_tcs`: Maximum test cases to execute (0 = no limit)
- `seconds`: Maximum execution time in seconds (0 = no limit)

**2. Separation of Concerns**

- **Tester Registration** (`RegisterTesters()`): Registers tester factories in `GlobalRegistry`
- **Package Configuration** (`testers.toml`): Defines which testers to run and how

The `registertesters.go` no longer has `RegisterPackages()` — all package definitions come from TOML files.

**3. Two-Level TOML Loading**

Packages are loaded from two sources in order:
1. `shared/go/api/testers/testers.toml` — shared-library baseline
2. `<project-root>/testers.toml` — project-specific overrides/additions

Later definitions override earlier ones by package name.

**4. Shared Autotester**

Created `shared/go/cmd/autotester/` — a standalone autotester for testing the shared library without application-specific testers. It:
- Registers only shared testers (database, databaseutil, logger, migration)
- Loads packages from `shared/testers.toml` only
- Supports the same CLI flags as application autotesters

#### Architecture Changes

```
┌────────────────────────────────────────────────────────────────┐
│  Application Autotester (e.g., tax/server/cmd/autotester)     │
├────────────────────────────────────────────────────────────────┤
│  1. RegisterTesters()                                          │
│     - sharedtesters.RegisterTesters()  ← shared testers        │
│     - app-specific Register() calls    ← app testers           │
│                                                                │
│  2. LoadTOMLPackages(sharedDir, projectRoot)                   │
│     - Load shared/go/api/testers/testers.toml                  │
│     - Load tax/server/testers.toml (overrides shared)          │
│                                                                │
│  3. Run with package selection (--package smoke|complete|...)  │
└────────────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────────────┐
│  Shared Autotester (shared/go/cmd/autotester)                  │
├────────────────────────────────────────────────────────────────┤
│  1. RegisterTesters()                                          │
│     - sharedtesters.RegisterTesters()  ← shared testers only   │
│                                                                │
│  2. LoadTOMLPackages(sharedDir, "")                            │
│     - Load shared/go/api/testers/testers.toml only             │
│     - No project overrides                                     │
│                                                                │
│  3. Run with package selection (--package smoke|complete|...)  │
└────────────────────────────────────────────────────────────────┘
```

#### New Data Structures

**`TesterConfig`** (in `autotester/config.go`):
```go
type TesterConfig struct {
    Name    string `toml:"name"`    // Tester identifier
    Enable  bool   `toml:"enable"`  // Whether to run this tester
    NumTcs  int    `toml:"num_tcs"` // Max test cases (0 = no limit)
    Seconds int    `toml:"seconds"` // Max execution time (0 = no limit)
}
```

**Updated `PackageConfig`**:
```go
type PackageConfig struct {
    Name        string         `toml:"name"`
    Description string         `toml:"description"`
    Enable      bool           `toml:"enable"`
    Testers     []TesterConfig `toml:"testers"`  // Changed from []string
}
```

#### Breaking Changes

**1. Removed `RegisterPackages()`**

The hard-coded `RegisterPackages()` function has been removed from `registertesters.go`. All package definitions must now come from `testers.toml` files.

**Migration**: If your code calls `sharedtesters.RegisterPackages()`, remove that call and ensure you have a `testers.toml` file with the packages you need.

**2. Changed `PackageConfig.Testers` Type**

The `Testers` field in `PackageConfig` changed from `[]string` to `[]TesterConfig`. This affects code that directly constructs `PackageConfig` structs.

**Migration**: Update any code that constructs `PackageConfig` to use the new structure:

```go
// Old (no longer works):
PackageConfig{
    Name: "smoke",
    Testers: []string{"tester_database", "tester_logger"},
}

// New:
PackageConfig{
    Name: "smoke",
    Testers: []TesterConfig{
        { Name: "tester_database", Enable: true, NumTcs: 20, Seconds: 60 },
        { Name: "tester_logger", Enable: true, NumTcs: 30, Seconds: 120 },
    },
}
```

**3. Changed `RegisterAll()` Signature**

Application-specific `RegisterAll()` functions now require `sharedDir` and `projectRoot` parameters:

```go
// Old:
apptesters.RegisterAll(&cfg)

// New:
apptesters.RegisterAll(&cfg, sharedDir, projectRoot)
```

#### Migration Guide

**For Application Projects (e.g., tax/):**

1. **Update `RegisterAll()` call** in `main.go`:
   ```go
   // Determine paths
   sharedDir := filepath.Join("..", "shared", "go", "api", "testers")
   projectRoot := "."
   
   // Register testers and load packages from TOML
   apptesters.RegisterAll(&cfg, sharedDir, projectRoot)
   ```

2. **Create `testers.toml`** at project root (copy from `shared/go/api/testers/testers.example.toml`)

3. **Remove any `RegisterPackages()` calls** — packages are now defined in TOML

**For Shared Library Testing:**

Use the new shared autotester:
```bash
cd shared/go
go run ./cmd/autotester --package smoke
go run ./cmd/autotester --package complete
```

#### CLI Changes

Both application and shared autotesters now support:

| Flag | Description |
|---|---|
| `--shared-dir` | Path to `shared/go/api/testers` directory |
| `--project-root` | Path to project root (app autotester only) |
| `--package` | Run a named tester package (e.g., smoke, regression, complete) |

#### Files Changed

| File | Change |
|---|---|
| `shared/go/api/autotester/config.go` | Added `TesterConfig` struct; updated `PackageConfig.Testers` type |
| `shared/go/api/testers/registertesters.go` | Removed `RegisterPackages()`; updated `RegisterTesters()` to include `tester_migration` |
| `shared/go/api/testers/testers.toml` | Updated to new format with per-tester configuration |
| `shared/go/api/testers/testers.example.toml` | Updated to new format with examples |
| `shared/go/cmd/autotester/main.go` | **New** — shared library autotester entry point |
| `tax/server/api/apptesters/tester_registry.go` | Updated `RegisterAll()` to accept paths and call `LoadTOMLPackages()` |
| `tax/server/cmd/autotester/main.go` | Added `--shared-dir` and `--project-root` flags; updated `RegisterAll()` call |

#### Usage Examples

**Run shared autotester with smoke package:**
```bash
cd shared/go
go run ./cmd/autotester --package smoke
```

**Run application autotester with project-specific overrides:**
```bash
cd tax
go run ./server/cmd/autotester --package complete
```

**Override shared package in project testers.toml:**
```toml
# tax/server/testers.toml
[[packages]]
name = "smoke"
description = "Project smoke: includes user tester"
enable = true
testers = [
    { name = "tester_database", enable = true, num_tcs = 10, seconds = 30 },
    { name = "user_tester", enable = true, num_tcs = 20, seconds = 60 }
]
```

**Disable a tester in a package:**
```toml
[[packages]]
name = "regression"
enable = true
testers = [
    { name = "tester_database", enable = true, num_tcs = 20, seconds = 60 },
    { name = "tester_databaseutil", enable = false, num_tcs = 0, seconds = 0 },  # Disabled
    { name = "tester_logger", enable = true, num_tcs = 30, seconds = 120 }
]
```

---


### 25.4 v6 — 2026/02/26: Tester Catalog and Global Enable/Disable via [[testers]]

**Issue:** [chendingplano/Shared#10](https://github.com/chendingplano/Shared/issues/10)

**Feature: `[[testers]]` section in `testers.toml`** — a structured catalog of all available testers with per-tester metadata and a global on/off switch that overrides all package-level settings.

#### Problem

Prior to this change, `testers.toml` only contained `[[packages]]` entries. There was no structured way to:

1. **Declare the tester catalog** — what testers exist, their purpose, type, creator, and other metadata.
2. **Globally disable a tester** — to disable a tester you had to set `enable = false` in *every package* that referenced it, which was error-prone and easy to miss.
3. **Distinguish** between "tester exists but is disabled" and "tester is simply not referenced in this package".

#### Solution

Introduced a `[[testers]]` section in `testers.toml` that serves as the authoritative tester catalog. Each entry declares a tester's full metadata and its global `enabled` flag.

**New `[[testers]]` format:**

```toml
[[testers]]
name        = "tester_database"          # mandatory; letters/digits/dashes/underscores; max 64 chars
desc        = "Tests database connectivity and basic CRUD"  # optional
purpose     = "validation"               # optional
type        = "integration"              # optional; default "functional"
dynamic_tcs = true                       # mandatory; true = generates test cases at runtime
enabled     = true                       # optional; global on/off; default true
remarks     = ""                         # optional; additional notes
creator     = "AutoTester Framework"     # optional
created_at  = "2026-02-20T00:00:00Z"    # optional; ISO-8601 timestamp
```

**Tester definition fields:**

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | yes | — | Unique identifier (letters, digits, dashes, underscores; max 64 chars) |
| `desc` | string | no | `""` | Short description of what the tester tests |
| `purpose` | string | no | `""` | e.g. `"validation"`, `"regression"`, `"smoke"` |
| `type` | string | no | `"functional"` | e.g. `"functional"`, `"performance"`, `"compliance"`, `"integration"` |
| `dynamic_tcs` | bool | yes | — | `true` if the tester generates test cases dynamically at runtime |
| `enabled` | bool | no | `true` | Global on/off switch — `false` prevents the tester from running in any package |
| `remarks` | string | no | `""` | Additional notes about the tester |
| `creator` | string | no | `""` | Person or team who created the tester |
| `created_at` | string | no | `""` | ISO-8601 creation timestamp |

**Two-level enable/disable semantics:**

A tester is executed only when **both** conditions are true:

- Package-level `enable = true` (existing behavior, per-package control), AND
- Global `[[testers]]` definition has `enabled = true` (or the tester is not listed in `[[testers]]`)

Setting `enabled = false` in the `[[testers]]` definition disables the tester in **all packages** with a single change — no need to edit every package.

**Example — globally disable a tester:**

```toml
# Stops this tester across ALL packages
[[testers]]
name    = "app_email_tester"
enabled = false
remarks = "Disabled until Resend sandbox account is configured"
```

#### New Data Structures

**`TesterDefinition`** (in `autotester/tester_definition.go`):

```go
type TesterDefinition struct {
    Name       string `toml:"name"`
    Desc       string `toml:"desc"`
    Purpose    string `toml:"purpose"`
    Type       string `toml:"type"`
    DynamicTcs bool   `toml:"dynamic_tcs"`
    Enabled    *bool  `toml:"enabled"`  // nil = default true
    Remarks    string `toml:"remarks"`
    Creator    string `toml:"creator"`
    CreatedAt  string `toml:"created_at"`
}

func (td *TesterDefinition) IsEnabled() bool {
    if td.Enabled == nil { return true }
    return *td.Enabled
}
```

**`TesterDefinitionRegistry`** (in `autotester/tester_definition.go`):

```go
var GlobalTesterDefinitionRegistry = &TesterDefinitionRegistry{...}

// IsEnabled returns true if the tester is not in the registry (default: enabled)
// or if its Enabled field is nil or true.
func (r *TesterDefinitionRegistry) IsEnabled(name string) bool
```

#### Processing Order in `RegisterPackagesFromTOML`

When a `testers.toml` file is loaded, the two sections are processed in order:

1. **Phase 1 — `[[testers]]`**: All tester definitions are upserted into `GlobalTesterDefinitionRegistry`. Definitions from a later file (e.g., project-level) override those from an earlier file (e.g., shared-level).
2. **Phase 2 — `[[packages]]`**: Each package's tester list is built. A tester is included only when BOTH:
   - Its package-level `enable` is `true`, AND
   - `GlobalTesterDefinitionRegistry.IsEnabled(name)` returns `true`

#### Runner-Level Check

`TestRunner.testerMatches()` now also checks `GlobalTesterDefinitionRegistry.IsEnabled(tester.Name())` **before** any other filter. This ensures that even testers selected via `--tester` or `--package` CLI flags are excluded if globally disabled.

#### Breaking Changes

None. The `[[testers]]` section is optional. Existing `testers.toml` files without a `[[testers]]` section continue to work exactly as before because:

- `GlobalTesterDefinitionRegistry` starts empty.
- `IsEnabled()` returns `true` for any tester not in the registry (default: enabled).

#### Files Changed

| File | Change |
|------|--------|
| `shared/go/api/autotester/tester_definition.go` | **New** — `TesterDefinition` struct, `TesterDefinitionRegistry`, `GlobalTesterDefinitionRegistry` |
| `shared/go/api/autotester/config.go` | `TOMLConfig` updated to include `Testers []TesterDefinition`; `RegisterPackagesFromTOML` updated to register definitions (phase 1) then filter packages by global enabled status (phase 2) |
| `shared/go/api/autotester/runner.go` | `testerMatches` now checks `GlobalTesterDefinitionRegistry.IsEnabled` before other filters |
| `shared/go/api/testers/testers.toml` | Added `[[testers]]` entries for all four shared-library testers; updated file header |
| `shared/go/api/testers/testers.example.toml` | Added `[[testers]]` examples; updated all documentation comments |

---
