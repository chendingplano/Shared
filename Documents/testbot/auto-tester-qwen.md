# AutoTester Automated Testing Framework

**Package path:** `github.com/chendingplano/shared/go/api/autotesters`
**Source:** [`shared/go/api/autotesters/`](../../go/api/autotesters/)

The AutoTester framework provides a comprehensive, modular approach to automated testing of systems under test (SUT). It enables developers to define reusable testers, compose them into test suites, execute tests with configurable parameters, and persist results in a structured database for analysis and reporting.

---

## Table of Contents

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Core Concepts](#core-concepts)
4. [Installation](#installation)
5. [Directory Structure](#directory-structure)
6. [Defining a Tester](#defining-a-tester)
7. [Test Case Management](#test-case-management)
8. [Test Execution](#test-execution)
9. [Result Verification](#result-verification)
10. [Logging and Reporting](#logging-and-reporting)
11. [Database Schema](#database-schema)
12. [Configuration](#configuration)
13. [Running AutoTesters](#running-autotesters)
14. [Advanced Patterns](#advanced-patterns)
15. [Best Practices](#best-practices)
16. [Examples](#examples)
17. [Troubleshooting](#troubleshooting)

---

## Overview

The AutoTester framework is designed to support automated testing across multiple layers of an application:

- **Shared Module Testers**: Test core functionality in the `shared/` library
- **Application-Specific Testers**: Test business logic unique to each application (e.g., `tax/`, `ChenWeb/`)
- **Integration Testers**: Test interactions between modules and external systems
- **End-to-End Testers**: Test complete user workflows

### Key Features

| Feature | Description |
|---|---|
| **Modular Design** | Each tester is defined in its own `.go` file, promoting reusability and maintainability |
| **Dynamic Test Generation** | Test cases can be created dynamically with configurable randomness |
| **Flexible Execution** | Support for hard-coded, dynamically generated, or hybrid test case approaches |
| **Persistent Results** | All test results are logged to a database table for historical analysis |
| **Configurable Filtering** | Run tests by purpose, type, priority, or custom tags |
| **Parallel Execution** | Support for concurrent test execution to reduce overall test time |
| **Retry Mechanism** | Configurable retry logic for flaky tests |

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                      AutoTester Runner                          │
│                    (server/cmd/autotester/)                     │
└─────────────────────────┬───────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│                       Test Orchestrator                         │
│  • Parse command-line flags                                     │
│  • Filter testers by purpose/type                               │
│  • Manage execution order and dependencies                      │
│  • Collect and aggregate results                                │
└─────────────────────────┬───────────────────────────────────────┘
                          │
          ┌───────────────┼───────────────┐
          ▼               ▼               ▼
┌─────────────────┐ ┌─────────────┐ ┌─────────────────┐
│   Tester A      │ │  Tester B   │ │   Tester C      │
│ (shared/go/api/ │ │(server/api/ │ │ (server/api/    │
│  autotesters/)  │ │ autotesters/│ │  autotesters/)  │
└────────┬────────┘ └──────┬──────┘ └────────┬────────┘
         │                 │                  │
         ▼                 ▼                  ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Test Case Executor                         │
│  • Prepare system state                                         │
│  • Generate/instantiate test cases                              │
│  • Execute test cases                                           │
│  • Collect results                                              │
│  • Verify outcomes                                              │
└─────────────────────────┬───────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Result Database (MySQL/PostgreSQL)           │
│  • test_runs                                                    │
│  • test_cases                                                   │
│  • test_results                                                 │
│  • test_logs                                                    │
└─────────────────────────────────────────────────────────────────┘
```

---

## Core Concepts

### AutoTester

An **AutoTester** is the top-level test runner that orchestrates the execution of multiple testers. It is typically defined in `server/cmd/autotester/main.go` and provides:

- Command-line interface for configuring test runs
- Test discovery and filtering
- Execution coordination
- Result aggregation

### Tester

A **Tester** is a self-contained testing unit defined in a `.go` file within an `autotesters` directory. Each tester:

- Targets a specific module, service, or functionality
- Implements a standard interface for execution
- Manages its own test case generation and verification
- Reports results through a common logging mechanism

### System Under Test (SUT)

The **SUT** is the component, module, or application being tested. It can be:

- A shared library function (e.g., `shared/go/api/database/`)
- An application service (e.g., `tax/server/api/services/`)
- An API endpoint
- A complete workflow spanning multiple services

### Test Case

A **Test Case** is a single test scenario with:

- **Input data**: Parameters, payloads, or state required for the test
- **Expected outcome**: The expected result, error, or side effect
- **Metadata**: Purpose, type, priority, tags for filtering

### Test Run

A **Test Run** is a single execution of the AutoTester, identified by a unique run ID. It captures:

- Start and end timestamps
- Configuration parameters
- Overall status (pass/fail/partial)
- Summary statistics

---

## Installation

The AutoTester framework is part of the shared library. No additional installation is required when using the workspace.

For standalone usage:

```bash
go get github.com/chendingplano/shared/go/api/autotesters@latest
```

---

## Directory Structure

### Shared Library Testers

```
shared/
└── go/
    └── api/
        └── autotesters/
            ├── README.md
            ├── tester_registry.go      # Registry of all shared testers
            ├── tester_database.go      # Tests for database utilities
            ├── goose_tester.go         # Tests for migration module
            ├── tester_logger.go        # Tests for logging utilities
            ├── config_tester.go        # Tests for configuration management
            └── ...
```

### Application-Specific Testers

```
myapp/
├── server/
│   ├── api/
│   │   └── autotesters/
│   │       ├── user_tester.go        # Tests for user-related APIs
│   │       ├── document_tester.go    # Tests for document APIs
│   │       ├── report_tester.go      # Tests for reporting APIs
│   │       └── integration_tester.go # Cross-module integration tests
│   └── cmd/
│       └── autotester/
│           ├── main.go               # AutoTester entry point
│           ├── config.go             # Application-specific config
│           └── custom_testers.go     # Custom tester registrations
└── ...
```

### Recommended File Organization

| File | Purpose |
|---|---|
| `*_tester.go` | Individual tester implementation |
| `tester_registry.go` | Central registry for tester discovery |
| `test_case.go` | Test case data structures |
| `test_result.go` | Result data structures |
| `test_logger.go` | Logging utilities |
| `test_db.go` | Database result persistence |

---

## Defining a Tester

### Tester Interface

All testers should implement the `Tester` interface:

```go
package autotesters

import (
    "context"
)

// Tester defines the contract for all automated testers.
type Tester interface {
    // Name returns the unique identifier for this tester.
    Name() string

    // Description returns a human-readable description.
    Description() string

    // Purpose returns the testing purpose (e.g., "validation", "integration").
    Purpose() string

    // Type returns the test type (e.g., "unit", "integration", "e2e").
    Type() string

    // Tags returns optional tags for filtering.
    Tags() []string

    // Prepare sets up the system state before testing.
    Prepare(ctx context.Context) error

    // GenerateTestCases creates test cases dynamically.
    // Return nil to use hard-coded test cases via GetTestCases().
    GenerateTestCases(ctx context.Context) ([]TestCase, error)

    // GetTestCases returns hard-coded test cases.
    // Used when test cases are not generated dynamically.
    GetTestCases() []TestCase

    // RunTestCase executes a single test case and returns the result.
    RunTestCase(ctx context.Context, tc TestCase) TestResult

    // Cleanup releases resources after testing completes.
    Cleanup(ctx context.Context) error
}
```

### Base Tester Implementation

Use `BaseTester` to avoid implementing the full interface:

```go
package autotesters

// BaseTester provides default implementations for optional methods.
type BaseTester struct {
    name        string
    description string
    purpose     string
    testType    string
    tags        []string
}

func (b *BaseTester) Name() string                        { return b.name }
func (b *BaseTester) Description() string                 { return b.description }
func (b *BaseTester) Purpose() string                     { return b.purpose }
func (b *BaseTester) Type() string                        { return b.testType }
func (b *BaseTester) Tags() []string                      { return b.tags }
func (b *BaseTester) Prepare(ctx context.Context) error   { return nil }
func (b *BaseTester) Cleanup(ctx context.Context) error   { return nil }
func (b *BaseTester) GenerateTestCases(ctx context.Context) ([]TestCase, error) {
    return nil, nil // Use hard-coded test cases
}
func (b *BaseTester) GetTestCases() []TestCase {
    return nil // No hard-coded test cases
}
```

### Example: Database Tester

```go
package autotesters

import (
    "context"
    "fmt"
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

---

## Test Case Management

### TestCase Structure

```go
// TestCase represents a single test scenario.
type TestCase struct {
    // ID is a unique identifier for the test case.
    ID string `json:"id"`

    // Name is a human-readable name.
    Name string `json:"name"`

    // Description provides detailed context about what is being tested.
    Description string `json:"description"`

    // Purpose categorizes the test purpose (e.g., "validation", "regression").
    Purpose string `json:"purpose,omitempty"`

    // Type categorizes the test type (e.g., "unit", "integration").
    Type string `json:"type,omitempty"`

    // Tags are optional labels for filtering.
    Tags []string `json:"tags,omitempty"`

    // Input contains the test input data.
    Input interface{} `json:"input,omitempty"`

    // Expected defines the expected outcome.
    Expected ExpectedResult `json:"expected"`

    // Priority indicates test importance.
    Priority Priority `json:"priority"`

    // RetryCount specifies how many times to retry on failure.
    RetryCount int `json:"retry_count,omitempty"`

    // Timeout overrides the default timeout for this test case.
    Timeout time.Duration `json:"timeout,omitempty"`

    // Dependencies lists IDs of test cases that must pass first.
    Dependencies []string `json:"dependencies,omitempty"`

    // SkipReason provides a reason if the test should be skipped.
    SkipReason string `json:"skip_reason,omitempty"`
}
```

### ExpectedResult Structure

```go
// ExpectedResult defines what outcome is expected from a test.
type ExpectedResult struct {
    // Success indicates whether the test should succeed.
    Success bool `json:"success"`

    // ExpectedError is the expected error (if Success is false).
    ExpectedError string `json:"expected_error,omitempty"`

    // ExpectedValue is the expected return value.
    ExpectedValue interface{} `json:"expected_value,omitempty"`

    // MaxDuration is the maximum allowed execution time.
    MaxDuration time.Duration `json:"max_duration,omitempty"`

    // SideEffects lists expected side effects (e.g., "row_created").
    SideEffects []string `json:"side_effects,omitempty"`

    // Custom is a map for custom validation logic.
    Custom map[string]interface{} `json:"custom,omitempty"`
}
```

### Priority Levels

```go
type Priority int

const (
    PriorityCritical Priority = iota // Must pass for deployment
    PriorityHigh                     // Important functionality
    PriorityMedium                   // Standard tests
    PriorityLow                      // Nice-to-have or stress tests
)
```

### Dynamic Test Case Generation

For stress testing or property-based testing, generate test cases dynamically:

```go
func (t *UserTester) GenerateTestCases(ctx context.Context) ([]TestCase, error) {
    var testCases []TestCase

    // Generate random user creation tests
    for i := 0; i < 50; i++ {
        testCases = append(testCases, TestCase{
            ID:       fmt.Sprintf("user_create_random_%03d", i),
            Name:     fmt.Sprintf("Random user creation %d", i),
            Input:    generateRandomUser(),
            Expected: ExpectedResult{Success: true},
            Priority: PriorityMedium,
        })
    }

    // Generate edge case tests
    edgeCases := []struct {
        name  string
        email string
        valid bool
    }{
        {"empty email", "", false},
        {"invalid format", "notanemail", false},
        {"very long email", strings.Repeat("a", 300) + "@example.com", false},
        {"valid email", "valid@example.com", true},
    }

    for _, ec := range edgeCases {
        testCases = append(testCases, TestCase{
            ID:       fmt.Sprintf("user_email_%s", ec.name),
            Name:     fmt.Sprintf("User email validation: %s", ec.name),
            Input:    User{Email: ec.email},
            Expected: ExpectedResult{Success: ec.valid},
            Priority: PriorityHigh,
        })
    }

    return testCases, nil
}
```

---

## Test Execution

### Execution Flow

1. **Preparation Phase**
   - Call `Prepare()` on each tester
   - Set up test fixtures and mock data
   - Initialize database connections

2. **Generation Phase**
   - Call `GenerateTestCases()` if implemented
   - Fall back to `GetTestCases()` for hard-coded cases
   - Apply filters based on command-line flags

3. **Execution Phase**
   - Execute test cases in order (respecting dependencies)
   - Handle retries for flaky tests
   - Collect results in real-time

4. **Verification Phase**
   - Compare actual results against expected outcomes
   - Validate side effects (database changes, file creation, etc.)
   - Determine pass/fail status

5. **Cleanup Phase**
   - Call `Cleanup()` on each tester
   - Release resources
   - Roll back test data if needed

### Test Runner

```go
package autotesters

import (
    "context"
    "sync"
    "time"
)

// TestRunner orchestrates test execution.
type TestRunner struct {
    testers      []Tester
    results      []TestResult
    config       *RunConfig
    mu           sync.Mutex
    runID        string
    startTime    time.Time
    endTime      time.Time
}

// RunConfig holds configuration for a test run.
type RunConfig struct {
    // Purposes filters tests by purpose (empty = all).
    Purposes []string

    // Types filters tests by type (empty = all).
    Types []string

    // Tags filters tests by tags (empty = all).
    Tags []string

    // TestIDs runs only specific test case IDs (empty = all).
    TestIDs []string

    // Parallel enables parallel execution.
    Parallel bool

    // MaxParallel limits concurrent test execution.
    MaxParallel int

    // RetryCount is the default retry count for failed tests.
    RetryCount int

    // Timeout is the overall test run timeout.
    Timeout time.Duration

    // StopOnFail stops execution on first failure.
    StopOnFail bool

    // SkipCleanup keeps test data after completion.
    SkipCleanup bool
}

// Run executes all registered testers.
func (r *TestRunner) Run(ctx context.Context) error {
    r.runID = generateRunID()
    r.startTime = time.Now()

    // Create result record
    if err := r.createRunRecord(); err != nil {
        return err
    }

    // Execute each tester
    for _, tester := range r.testers {
        if err := r.executeTester(ctx, tester); err != nil {
            return err
        }
        if r.config.StopOnFail && r.hasFailures() {
            break
        }
    }

    r.endTime = time.Now()
    return r.updateRunRecord()
}

func (r *TestRunner) executeTester(ctx context.Context, tester Tester) error {
    // Prepare
    if err := tester.Prepare(ctx); err != nil {
        return fmt.Errorf("prepare failed for %s: %w", tester.Name(), err)
    }
    defer tester.Cleanup(ctx)

    // Get test cases
    testCases, err := tester.GenerateTestCases(ctx)
    if err != nil {
        return fmt.Errorf("GenerateTestCases failed: %w", err)
    }
    if testCases == nil {
        testCases = tester.GetTestCases()
    }

    // Filter test cases
    testCases = r.filterTestCases(testCases)

    // Execute test cases
    if r.config.Parallel {
        return r.executeParallel(ctx, tester, testCases)
    }
    return r.executeSequential(ctx, tester, testCases)
}

func (r *TestRunner) executeSequential(ctx context.Context, tester Tester, testCases []TestCase) error {
    for _, tc := range testCases {
        result := r.executeWithRetry(ctx, tester, tc)
        r.recordResult(result)
    }
    return nil
}

func (r *TestRunner) executeParallel(ctx context.Context, tester Tester, testCases []TestCase) error {
    var wg sync.WaitGroup
    sem := make(chan struct{}, r.config.MaxParallel)

    for _, tc := range testCases {
        wg.Add(1)
        sem <- struct{}{}
        go func(tc TestCase) {
            defer wg.Done()
            defer func() { <-sem }()
            result := r.executeWithRetry(ctx, tester, tc)
            r.recordResult(result)
        }(tc)
    }

    wg.Wait()
    return nil
}

func (r *TestRunner) executeWithRetry(ctx context.Context, tester Tester, tc TestCase) TestResult {
    var result TestResult
    for i := 0; i <= r.config.RetryCount; i++ {
        result = tester.RunTestCase(ctx, tc)
        if result.Status == StatusPass {
            break
        }
        if i < r.config.RetryCount {
            result.RetryCount++
            time.Sleep(time.Second) // Backoff
        }
    }
    return result
}
```

---

## Result Verification

### TestResult Structure

```go
// TestResult captures the outcome of a single test case execution.
type TestResult struct {
    // RunID is the unique identifier for the test run.
    RunID string `json:"run_id"`

    // TestCaseID is the ID of the executed test case.
    TestCaseID string `json:"test_case_id"`

    // TesterName is the name of the tester that executed this test.
    TesterName string `json:"tester_name"`

    // Status is the final status (pass/fail/skip).
    Status Status `json:"status"`

    // Message provides additional context about the result.
    Message string `json:"message,omitempty"`

    // Error contains the error message if the test failed.
    Error string `json:"error,omitempty"`

    // StartTime is when the test started.
    StartTime time.Time `json:"start_time"`

    // EndTime is when the test completed.
    EndTime time.Time `json:"end_time"`

    // Duration is the total execution time.
    Duration time.Duration `json:"duration"`

    // RetryCount is the number of retries performed.
    RetryCount int `json:"retry_count"`

    // ActualValue contains the actual return value for inspection.
    ActualValue interface{} `json:"actual_value,omitempty"`

    // SideEffectsObserved lists observed side effects.
    SideEffectsObserved []string `json:"side_effects_observed,omitempty"`

    // Logs contains detailed execution logs.
    Logs []LogEntry `json:"logs,omitempty"`
}
```

### Status Enum

```go
type Status string

const (
    StatusPass  Status = "pass"
    StatusFail  Status = "fail"
    StatusSkip  Status = "skip"
    StatusError Status = "error"
)
```

### Verification Logic

```go
func verifyResult(tc TestCase, result TestResult) TestResult {
    // Check if test was skipped
    if tc.SkipReason != "" {
        result.Status = StatusSkip
        result.Message = fmt.Sprintf("Skipped: %s", tc.SkipReason)
        return result
    }

    // Check success/failure expectation
    if tc.Expected.Success && result.Error != "" {
        result.Status = StatusFail
        result.Error = fmt.Sprintf("Expected success but got error: %s", result.Error)
        return result
    }

    if !tc.Expected.Success {
        if result.Error == "" {
            result.Status = StatusFail
            result.Error = "Expected error but test succeeded"
            return result
        }
        if tc.Expected.ExpectedError != "" && !strings.Contains(result.Error, tc.Expected.ExpectedError) {
            result.Status = StatusFail
            result.Error = fmt.Sprintf("Expected error containing '%s' but got: %s",
                tc.Expected.ExpectedError, result.Error)
            return result
        }
    }

    // Check duration constraint
    if tc.Expected.MaxDuration > 0 && result.Duration > tc.Expected.MaxDuration {
        result.Status = StatusFail
        result.Error = fmt.Sprintf("Exceeded max duration: %v > %v",
            result.Duration, tc.Expected.MaxDuration)
        return result
    }

    // Check expected value
    if tc.Expected.ExpectedValue != nil {
        if !reflect.DeepEqual(result.ActualValue, tc.Expected.ExpectedValue) {
            result.Status = StatusFail
            result.Error = fmt.Sprintf("Value mismatch: expected %v, got %v",
                tc.Expected.ExpectedValue, result.ActualValue)
            return result
        }
    }

    // Check side effects
    for _, expected := range tc.Expected.SideEffects {
        if !contains(result.SideEffectsObserved, expected) {
            result.Status = StatusFail
            result.Error = fmt.Sprintf("Missing expected side effect: %s", expected)
            return result
        }
    }

    result.Status = StatusPass
    return result
}
```

---

## Logging and Reporting

### LogEntry Structure

```go
// LogEntry represents a single log entry during test execution.
type LogEntry struct {
    Timestamp time.Time `json:"timestamp"`
    Level     string    `json:"level"` // DEBUG, INFO, WARN, ERROR
    Message   string    `json:"message"`
    Context   map[string]interface{} `json:"context,omitempty"`
}
```

### Test Logger

```go
package autotesters

import (
    "github.com/chendingplano/shared/go/api/logger"
)

// TestLogger wraps the shared logger for test-specific logging.
type TestLogger struct {
    logger  *logger.Logger
    runID   string
    entries []LogEntry
}

func (l *TestLogger) Debug(msg string, ctx map[string]interface{}) {
    l.log("DEBUG", msg, ctx)
}

func (l *TestLogger) Info(msg string, ctx map[string]interface{}) {
    l.log("INFO", msg, ctx)
}

func (l *TestLogger) Warn(msg string, ctx map[string]interface{}) {
    l.log("WARN", msg, ctx)
}

func (l *TestLogger) Error(msg string, ctx map[string]interface{}) {
    l.log("ERROR", msg, ctx)
}

func (l *TestLogger) log(level, msg string, ctx map[string]interface{}) {
    entry := LogEntry{
        Timestamp: time.Now(),
        Level:     level,
        Message:   msg,
        Context:   ctx,
    }
    l.entries = append(l.entries, entry)

    // Also log through shared logger
    logCtx := map[string]interface{}{
        "run_id": l.runID,
    }
    for k, v := range ctx {
        logCtx[k] = v
    }

    switch level {
    case "DEBUG":
        l.logger.Debug(msg, logCtx)
    case "INFO":
        l.logger.Info(msg, logCtx)
    case "WARN":
        l.logger.Warn(msg, logCtx)
    case "ERROR":
        l.logger.Error(msg, logCtx)
    }
}

// GetEntries returns all log entries for this test run.
func (l *TestLogger) GetEntries() []LogEntry {
    return l.entries
}
```

---

## Database Schema

### Table: `auto_test_runs`

Stores overall test run information.

```sql
CREATE TABLE auto_test_runs (
    id              BIGSERIAL PRIMARY KEY,
    run_id          VARCHAR(64) NOT NULL UNIQUE,
    started_at      TIMESTAMPTZ NOT NULL,
    ended_at        TIMESTAMPTZ,
    status          VARCHAR(20) NOT NULL, -- 'running', 'completed', 'failed', 'partial'
    config_json     JSONB,
    total_cases     INTEGER NOT NULL DEFAULT 0,
    passed_count    INTEGER NOT NULL DEFAULT 0,
    failed_count    INTEGER NOT NULL DEFAULT 0,
    skipped_count   INTEGER NOT NULL DEFAULT 0,
    error_count     INTEGER NOT NULL DEFAULT 0,
    duration_ms     BIGINT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_auto_test_runs_run_id ON auto_test_runs(run_id);
CREATE INDEX idx_auto_test_runs_started_at ON auto_test_runs(started_at);
CREATE INDEX idx_auto_test_runs_status ON auto_test_runs(status);
```

### Table: `auto_test_cases`

Stores test case definitions (optional, for historical tracking).

```sql
CREATE TABLE auto_test_cases (
    id              BIGSERIAL PRIMARY KEY,
    run_id          VARCHAR(64) NOT NULL,
    test_case_id    VARCHAR(128) NOT NULL,
    tester_name     VARCHAR(128) NOT NULL,
    name            VARCHAR(255) NOT NULL,
    description     TEXT,
    purpose         VARCHAR(64),
    type            VARCHAR(64),
    tags            TEXT[],
    priority        INTEGER,
    input_json      JSONB,
    expected_json   JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_run
        FOREIGN KEY (run_id)
        REFERENCES auto_test_runs(run_id)
        ON DELETE CASCADE
);

CREATE INDEX idx_auto_test_cases_run_id ON auto_test_cases(run_id);
CREATE INDEX idx_auto_test_cases_tester ON auto_test_cases(tester_name);
CREATE INDEX idx_auto_test_cases_purpose ON auto_test_cases(purpose);
CREATE INDEX idx_auto_test_cases_type ON auto_test_cases(type);
```

### Table: `auto_test_results`

Stores individual test results.

```sql
CREATE TABLE auto_test_results (
    id                      BIGSERIAL PRIMARY KEY,
    run_id                  VARCHAR(64) NOT NULL,
    test_case_id            VARCHAR(128) NOT NULL,
    tester_name             VARCHAR(128) NOT NULL,
    status                  VARCHAR(20) NOT NULL,
    message                 TEXT,
    error                   TEXT,
    start_time              TIMESTAMPTZ NOT NULL,
    end_time                TIMESTAMPTZ NOT NULL,
    duration_ms             BIGINT NOT NULL,
    retry_count             INTEGER NOT NULL DEFAULT 0,
    actual_value_json       JSONB,
    side_effects            TEXT[],
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_run
        FOREIGN KEY (run_id)
        REFERENCES auto_test_runs(run_id)
        ON DELETE CASCADE
);

CREATE INDEX idx_auto_test_results_run_id ON auto_test_results(run_id);
CREATE INDEX idx_auto_test_results_test_case ON auto_test_results(test_case_id);
CREATE INDEX idx_auto_test_results_status ON auto_test_results(status);
CREATE INDEX idx_auto_test_results_tester ON auto_test_results(tester_name);
CREATE INDEX idx_auto_test_results_start_time ON auto_test_results(start_time);
```

### Table: `auto_test_logs`

Stores detailed execution logs.

```sql
CREATE TABLE auto_test_logs (
    id              BIGSERIAL PRIMARY KEY,
    run_id          VARCHAR(64) NOT NULL,
    test_case_id    VARCHAR(128),
    tester_name     VARCHAR(128) NOT NULL,
    log_level       VARCHAR(20) NOT NULL,
    message         TEXT NOT NULL,
    context_json    JSONB,
    logged_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_run
        FOREIGN KEY (run_id)
        REFERENCES auto_test_runs(run_id)
        ON DELETE CASCADE
);

CREATE INDEX idx_auto_test_logs_run_id ON auto_test_logs(run_id);
CREATE INDEX idx_auto_test_logs_test_case ON auto_test_logs(test_case_id);
CREATE INDEX idx_auto_test_logs_level ON auto_test_logs(log_level);
CREATE INDEX idx_auto_test_logs_logged_at ON auto_test_logs(logged_at);
```

---

## Configuration

### RunConfig Options

| Option | Type | Default | Description |
|---|---|---|---|
| `Purposes` | `[]string` | `[]` | Filter by test purpose |
| `Types` | `[]string` | `[]` | Filter by test type |
| `Tags` | `[]string` | `[]` | Filter by tags |
| `TestIDs` | `[]string` | `[]` | Run specific test case IDs |
| `Parallel` | `bool` | `false` | Enable parallel execution |
| `MaxParallel` | `int` | `4` | Max concurrent tests |
| `RetryCount` | `int` | `0` | Default retry count |
| `Timeout` | `time.Duration` | `30m` | Overall timeout |
| `StopOnFail` | `bool` | `false` | Stop on first failure |
| `SkipCleanup` | `bool` | `false` | Keep test data after run |

### Configuration File

Optionally, configure via `libconfig.toml`:

```toml
[auto_tester]
# Default test run timeout in minutes
timeout_minutes = 60

# Enable parallel execution
parallel = true

# Maximum parallel test executions
max_parallel = 8

# Default retry count for flaky tests
default_retry_count = 2

# Database table names (override defaults)
table_name_runs = "auto_test_runs"
table_name_results = "auto_test_results"
table_name_logs = "auto_test_logs"

# Enable detailed logging
verbose_logging = true

# Retention period for test results (days)
result_retention_days = 90
```

---

## Running AutoTesters

### Command-Line Interface

The AutoTester is typically run as a standalone command:

```bash
# Run all tests
go run server/cmd/autotester/main.go

# Run tests by purpose
go run server/cmd/autotester/main.go --purpose=validation --purpose=regression

# Run tests by type
go run server/cmd/autotester/main.go --type=integration --type=e2e

# Run tests by tags
go run server/cmd/autotester/main.go --tags=database --tags=critical

# Run specific test cases
go run server/cmd/autotester/main.go --test-id=user_create_001 --test-id=user_delete_003

# Enable parallel execution
go run server/cmd/autotester/main.go --parallel --max-parallel=8

# Stop on first failure (useful for debugging)
go run server/cmd/autotester/main.go --stop-on-fail

# Skip cleanup to inspect test data
go run server/cmd/autotester/main.go --skip-cleanup

# Set timeout
go run server/cmd/autotester/main.go --timeout=120m

# Enable verbose logging
go run server/cmd/autotester/main.go --verbose
```

### main.go Example

```go
package main

import (
    "context"
    "flag"
    "log"
    "os"
    "time"

    "github.com/chendingplano/shared/go/api/autotesters"
    "github.com/chendingplano/shared/go/api/database"
    "github.com/chendingplano/shared/go/api/logger"
)

func main() {
    // Parse flags
    purposes := flag.String("purpose", "", "Comma-separated test purposes")
    types := flag.String("type", "", "Comma-separated test types")
    tags := flag.String("tags", "", "Comma-separated tags")
    testIDs := flag.String("test-id", "", "Comma-separated test case IDs")
    parallel := flag.Bool("parallel", false, "Enable parallel execution")
    maxParallel := flag.Int("max-parallel", 4, "Maximum parallel tests")
    retryCount := flag.Int("retry", 0, "Default retry count")
    timeout := flag.Duration("timeout", 30*time.Minute, "Overall timeout")
    stopOnFail := flag.Bool("stop-on-fail", false, "Stop on first failure")
    skipCleanup := flag.Bool("skip-cleanup", false, "Skip cleanup after tests")
    verbose := flag.Bool("verbose", false, "Enable verbose logging")
    flag.Parse()

    // Initialize logger
    log := logger.CreateDefaultLogger("AUTO_TESTER")
    if *verbose {
        log.SetLevel(logger.DebugLevel)
    }

    // Initialize database
    ctx := context.Background()
    if err := database.InitDB(ctx, nil, nil); err != nil {
        log.Fatal("Database initialization failed", "error", err)
    }
    defer database.CloseDatabase()

    // Create test runner
    runner := autotesters.NewTestRunner()

    // Register shared testers
    runner.Register(&autotesters.DatabaseTester{})
    runner.Register(&autotesters.GooseTester{})
    runner.Register(&autotesters.LoggerTester{})

    // Register application-specific testers
    runner.Register(&autotesters.UserTester{})
    runner.Register(&autotesters.DocumentTester{})

    // Build config
    config := &autotesters.RunConfig{
        Purposes:    split(*purposes),
        Types:       split(*types),
        Tags:        split(*tags),
        TestIDs:     split(*testIDs),
        Parallel:    *parallel,
        MaxParallel: *maxParallel,
        RetryCount:  *retryCount,
        Timeout:     *timeout,
        StopOnFail:  *stopOnFail,
        SkipCleanup: *skipCleanup,
    }

    // Run tests
    if err := runner.Run(ctx, config); err != nil {
        log.Fatal("Test run failed", "error", err)
    }

    // Print summary
    summary := runner.GetSummary()
    log.Info("Test run completed",
        "total", summary.Total,
        "passed", summary.Passed,
        "failed", summary.Failed,
        "skipped", summary.Skipped,
        "duration", summary.Duration,
    )

    // Exit with error code if tests failed
    if summary.Failed > 0 || summary.Error > 0 {
        os.Exit(1)
    }
}

func split(s string) []string {
    if s == "" {
        return nil
    }
    return strings.Split(s, ",")
}
```

---

## Advanced Patterns

### Test Dependencies

Some tests depend on the successful completion of other tests:

```go
testCases := []TestCase{
    {
        ID:       "user_create_001",
        Name:     "Create user",
        Input:    User{Name: "John"},
        Expected: ExpectedResult{Success: true},
    },
    {
        ID:           "user_get_002",
        Name:         "Get user by ID",
        Dependencies: []string{"user_create_001"},
        Input:        map[string]string{"ref": "user_create_001.result.id"},
        Expected:     ExpectedResult{Success: true},
    },
    {
        ID:           "user_update_003",
        Name:         "Update user",
        Dependencies: []string{"user_get_002"},
        Input:        map[string]string{"ref": "user_get_002.result.id", "name": "Jane"},
        Expected:     ExpectedResult{Success: true},
    },
}
```

### Data-Driven Testing

Use external data sources to drive test cases:

```go
func (t *ReportTester) GenerateTestCases(ctx context.Context) ([]TestCase, error) {
    // Load test data from CSV or JSON
    testData, err := loadTestData("testdata/report_scenarios.json")
    if err != nil {
        return nil, err
    }

    var testCases []TestCase
    for _, scenario := range testData.Scenarios {
        testCases = append(testCases, TestCase{
            ID:       scenario.ID,
            Name:     scenario.Name,
            Input:    scenario.Input,
            Expected: scenario.Expected,
            Priority: scenario.Priority,
        })
    }
    return testCases, nil
}
```

### Mocking and Fixtures

Use mocks for unit testing and fixtures for integration testing:

```go
type UserServiceTester struct {
    BaseTester
    mockDB    *mocks.Database
    fixtureDB *sql.DB
}

func (t *UserServiceTester) Prepare(ctx context.Context) error {
    // Use mock for unit tests
    if t.testType == "unit" {
        t.mockDB = mocks.NewDatabase()
        return nil
    }

    // Use real DB with fixtures for integration tests
    var err error
    t.fixtureDB, err = database.GetTestConnection()
    if err != nil {
        return err
    }

    // Load fixtures
    return loadFixtures(ctx, t.fixtureDB, "testdata/users_fixtures.sql")
}

func (t *UserServiceTester) Cleanup(ctx context.Context) error {
    if t.fixtureDB != nil {
        // Rollback fixtures
        _, err := t.fixtureDB.ExecContext(ctx, "DELETE FROM users WHERE test_run = true")
        return err
    }
    return nil
}
```

### Custom Validators

Implement custom validation logic:

```go
type CustomValidator func(actual interface{}, expected ExpectedResult) (bool, string)

func validateUserCreated(actual interface{}, expected ExpectedResult) (bool, string) {
    user, ok := actual.(*User)
    if !ok {
        return false, "Expected User type"
    }
    if user.ID == 0 {
        return false, "User ID should be set"
    }
    if user.CreatedAt.IsZero() {
        return false, "CreatedAt should be set"
    }
    return true, ""
}

// Usage in TestCase
TestCase{
    ID:       "user_create_001",
    Input:    User{Name: "John"},
    Expected: ExpectedResult{
        Success: true,
        Custom: map[string]interface{}{
            "validator": validateUserCreated,
        },
    },
}
```

---

## Best Practices

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

## Examples

### Complete Example: User API Tester

```go
package autotesters

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
    "net/http"
    "net/http/httptest"
    "time"

    "github.com/chendingplano/shared/go/api/ApiTypes"
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
                "ref": "user_api_create_001.result.id",
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

### Querying Test Results

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

### Performance Tuning

If tests are running slowly:

1. **Enable parallel execution**: `--parallel --max-parallel=8`
2. **Reduce test data volume**: Generate fewer dynamic test cases
3. **Optimize database queries**: Add indexes, reduce N+1 queries
4. **Use connection pooling**: Ensure database connections are pooled
5. **Profile test execution**: Add timing logs to identify bottlenecks
