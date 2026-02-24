# TestRunner

**Package:** `autotesters`  
**File:** `runner.go`

## Overview

`TestRunner` is the central orchestration component of the auto-testing framework. It manages the complete lifecycle of test execution, from initializing test runs through executing individual test cases to generating final reports.

The runner supports:
- **Sequential and parallel** test execution
- **Filtering** by tester name, purpose, type, and tags
- **Retry logic** for flaky tests
- **Dependency management** between test cases
- **Database persistence** for run records, results, and logs
- **JSON report** generation
- **Configurable timeouts** at both run and test-case levels

---

## Type Definition

```go
type TestRunner struct {
    // Internal fields (not exported)
}
```

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `testers` | `[]Tester` | Registered testers to execute |
| `config` | `*RunConfig` | Run configuration (filters, timeouts, retries) |
| `runID` | `string` | Unique UUID for this test run |
| `seed` | `int64` | Random seed for deterministic test generation |
| `startTime` | `time.Time` | When the run started |
| `logger` | `ApiTypes.JimoLogger` | Logger instance |
| `db` | `*DBPersistence` | Database persistence layer (optional) |
| `summary` | `RunSummary` | Accumulated run statistics |
| `passed` | `map[string]bool` | Track passed test cases for dependency checks |
| `runsTable` | `string` | Database table name for run records |
| `resultsTable` | `string` | Database table name for test results |
| `logsTable` | `string` | Database table name for test logs |

---

## Constructor

### `NewTestRunner`

```go
func NewTestRunner(testers []Tester, config *RunConfig, logger ApiTypes.JimoLogger) *TestRunner
```

Creates a new `TestRunner` instance with the provided testers, configuration, and logger.

**Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `testers` | `[]Tester` | List of testers to orchestrate |
| `config` | `*RunConfig` | Run configuration |
| `logger` | `ApiTypes.JimoLogger` | Logger for output |

**Default Configuration Values:**
- `MaxParallel`: 4 (if `<= 0`)
- `CaseTimeout`: 30 seconds (if `<= 0`)
- `RunTimeout`: 30 minutes (if `<= 0`)
- `Environment`: `"local"` (if empty)

**Example:**
```go
testers := []Tester{NewDatabaseTester(), NewAPITester()}
config := &RunConfig{
    Parallel:    true,
    MaxParallel: 8,
    RetryCount:  2,
}
runner := NewTestRunner(testers, config, logger)
```

---

## Configuration Methods

### `SetDBPersistence`

```go
func (r *TestRunner) SetDBPersistence(db *DBPersistence)
```

Configures the database persistence layer for storing run records, test results, and logs.

### `SetTableNames`

```go
func (r *TestRunner) SetTableNames(runs, results, logs string)
```

Sets custom database table names for auto-test tables.

**Default table names:**
- Runs: `"auto_test_runs"`
- Results: `"auto_test_results"`
- Logs: `"auto_test_logs"`

---

## Execution Methods

### `Run`

```go
func (r *TestRunner) Run(ctx context.Context) error
```

Executes all registered testers and returns the summary.

**Lifecycle:**
1. Generates a unique `runID` (UUID v4)
2. Resolves the random seed
3. Creates initial run record in database (if DB configured)
4. Applies overall run timeout
5. Executes testers (parallel or sequential based on config)
6. Finalizes run record with statistics
7. Prints summary to logger
8. Writes JSON report (if configured)

**Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `ctx` | `context.Context` | Context for cancellation and timeout |

**Returns:**
- `error` - Non-nil if run record creation fails

**Example:**
```go
ctx := context.Background()
if err := runner.Run(ctx); err != nil {
    log.Fatalf("Run failed: %v", err)
}
```

### `Summary`

```go
func (r *TestRunner) Summary() RunSummary
```

Returns the final run summary containing statistics and failure details.

**Returns:**
- `RunSummary` - Thread-safe copy of the run summary

---

## Internal Execution Flow

### Tester Execution Modes

#### Sequential Execution (`executeSequentialTesters`)
Runs testers one after another in order.

#### Parallel Execution (`executeParallelTesters`)
Runs testers concurrently with a semaphore limiting concurrency to `MaxParallel`.

### Test Case Lifecycle

For each tester:

1. **Prepare** - Call `tester.Prepare(ctx)` for setup
2. **Collect Cases** - Get cases from `GenerateTestCases()` and/or `GetTestCases()`
3. **Filter Cases** - Apply case-level filters (TestIDs, purposes, types, tags)
4. **Execute Cases** - Run each case with retry logic
5. **Cleanup** - Call `tester.Cleanup(ctx)` (unless `SkipCleanup` is set)

### Retry Logic (`runTestCase`)

```go
for attempt = 0; attempt <= retryCount; attempt++ {
    result = tester.RunTestCase(caseCtx, tc)
    if result.Status == StatusPass || result.Status == StatusSkip {
        break // Success, stop retrying
    }
    // Continue retrying for failures and errors
}
```

**Retry count resolution:**
- Uses `tc.RetryCount` if set (> 0)
- Otherwise uses `config.RetryCount`

**Timeout resolution:**
- Uses `tc.Timeout` if set (> 0)
- Otherwise uses `config.CaseTimeout`

### Result Verification (`verifyResult`)

Applies assertions to determine pass/fail status:

| Check | Description |
|-------|-------------|
| Skip reason | If set, mark as skipped |
| Execution errors | Propagate error status |
| Success expectation | Fail if error expected but got success |
| Expected error content | Verify error message contains expected substring |
| Value equality | DeepEqual comparison with `ExpectedValue` |
| Duration constraint | Fail if execution exceeds `MaxDuration` |
| Side effects | Verify all expected side effects were observed |
| Custom validator | Run custom validation function if provided |

### Dependency Management

Test cases can declare dependencies on other test cases:

```go
TestCase{
    ID:           "module.feature.variant",
    Dependencies: []string{"module.setup.complete"},
}
```

Dependencies are checked before execution. If any dependency has not passed, the test case is **skipped**.

---

## Filtering

### Tester-Level Filters

Applied in `testerMatches()`:

| Filter | Config Field | Match Logic |
|--------|--------------|-------------|
| Names | `TesterNames` | Tester name must match one in list |
| Purpose | `Purposes` | Tester purpose must match one in list |
| Type | `Types` | Tester type must match one in list |
| Tags | `Tags` | Tester tags must contain at least one tag from list |

### Test Case-Level Filters

Applied in `filterTestCases()`:

| Filter | Config Field | Behavior |
|--------|--------------|----------|
| Test IDs | `TestIDs` | Only run specified IDs (ignores other filters) |
| Purpose | `Purposes` | Case purpose must match |
| Type | `Types` | Case type must match |
| Tags | `Tags` | Case tags must contain at least one tag from list |

---

## Database Integration

### Run Record Creation

When DB is configured, `createRunRecord()` stores:

```go
type TestRun struct {
    ID          string
    StartedAt   time.Time
    Status      string  // "running", "completed", "failed", "partial"
    Environment string
    Seed        int64
    Config      *RunConfig
    EnvMetadata map[string]string  // Go version, OS, hostname, etc.
}
```

### Result Persistence

Each test result is persisted via `db.InsertTestResult()` and logs via `db.InsertTestLogs()`.

### Run Finalization

`finalizeRunRecord()` updates the run record with:
- Final status (`completed`, `failed`, `partial`)
- End time and duration
- Final counters (total, passed, failed, skipped, errored)

---

## Reporting

### Console Summary

`printSummary()` outputs:
- Run metadata (runID, seed, environment, duration)
- Counters (total, passed, failed, skipped, errored)
- Pass rate percentage
- Detailed failure list

**Example output:**
```
AutoTester Run Complete
  run_id: 550e8400-e29b-41d4-a716-446655440000
  seed: 1234567890
  env: staging
  duration: 45s
  total: 100
  passed: 95
  failed: 3
  skipped: 2
  errored: 0
  pass_rate: 97.4%

FAILURES:
  [fail] module.feature.case1 (120ms): expected value mismatch
  [error] module.feature.case2 (50ms): connection timeout
```

### JSON Report

If `config.JSONReport` is set, `writeJSONReport()` writes the `RunSummary` as formatted JSON:

```json
{
  "RunID": "550e8400-e29b-41d4-a716-446655440000",
  "Seed": 1234567890,
  "Environment": "staging",
  "StartedAt": "2024-01-15T10:00:00Z",
  "EndedAt": "2024-01-15T10:00:45Z",
  "Duration": 45000000000,
  "Total": 100,
  "Passed": 95,
  "Failed": 3,
  "Skipped": 2,
  "Errored": 0,
  "Failures": [...]
}
```

---

## Thread Safety

`TestRunner` uses a `sync.Mutex` to protect shared state:
- `summary` - Updated by concurrent test executors
- `passed` - Dependency tracking map

All public methods that access shared state acquire the lock appropriately.

---

## Related Types

| Type | Package | Description |
|------|---------|-------------|
| `Tester` | `autotesters` | Interface for test implementations |
| `TestCase` | `autotesters` | Individual test case definition |
| `TestResult` | `autotesters` | Execution result of a test case |
| `RunConfig` | `autotesters` | Configuration for a test run |
| `RunSummary` | `autotesters` | Final statistics and failure details |
| `TestRun` | `autotesters` | Database record for a test run |
| `DBPersistence` | `autotesters` | Database persistence layer |

---

## Error Codes

| Code | Location | Description |
|------|----------|-------------|
| `MID_060221143044` | `createRunRecord` | Failed to create run record |
| `MID_060221143040` | `collectTestCases` | Failed to generate test cases |
| `MID_060221143041` | `verifyResult` | Expected error assertion |
| `MID_060221143042` | `writeJSONReport` | Failed to marshal summary |
| `MID_060221143043` | `writeJSONReport` | Failed to write report file |
