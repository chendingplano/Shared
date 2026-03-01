# Automated Tester (AutoTester)

## Purpose
AutoTester is an automated testing framework that validates a **System-Under-Test (SUT)**. It orchestrates one or more **Tester** implementations to prepare the system, generate or load test cases, execute them, collect results, verify pass/fail, and log outcomes in a database.

This document defines the conventions and responsibilities for AutoTester and Tester implementations across `shared/` modules and application-specific codebases (e.g., `tax/`, `ChenWeb/`). It also expands the guidelines with operational details that make the system accurate, repeatable, and maintainable.

## Core Concepts

### System-Under-Test (SUT)
The SUT is the service, module, or feature being validated. A SUT can be:
- A single shared module (e.g., a shared Go package).
- An application service (e.g., `ChenWeb` API).
- A distributed set of dependencies (e.g., API + DB + external services).

The AutoTester must know how to:
- Locate and initialize the SUT (process, service address, or in-memory component).
- Reset or seed the SUT to a known baseline.
- Confirm readiness before running tests.

### Tester
A **Tester** is a Go implementation that tests one aspect of the SUT. Each Tester should be:
- Focused on a specific functional area or system capability.
- Runnable independently or as part of a suite.
- Self-contained in a `.go` file (or a small package if needed).

A Tester typically provides:
- Setup and teardown logic for its scope.
- Test case generation or loading.
- Execution and verification logic.
- Result reporting and logging.

### AutoTester
AutoTester is the orchestrator. It:
- Loads and configures one or more Testers.
- Applies test selection rules (purpose, type, scope).
- Provides common infrastructure (logging, randomness, config parsing, concurrency).
- Aggregates results and writes to the database.
- Provides a CLI interface to run tests consistently.

## Directory Structure and Placement

### Shared Testers
Shared testers for modules in `shared/` live in:
- `shared/go/api/autotesters/`

Each tester should be defined in its own `.go` file. If a tester requires helpers, it may use a small subpackage, but the main entry should remain visible and discoverable.

### Application-Specific Testers
Applications should define their own testers in:
- `server/api/autotesters/`

These are specialized for the app’s environment, endpoints, and domain data.

### AutoTester Command
Each app should provide an executable AutoTester at:
- `server/cmd/autotester/`

This directory should contain:
- `main.go` (the entrypoint)
- Additional Go files as needed (config, CLI, registry, utilities)

This executable is the canonical way to run automated tests for the app.

## AutoTester Lifecycle
An automated test run follows a consistent lifecycle:

1. **Prepare the System**
   - Start or connect to the SUT.
   - Clear or reset state (DB, cache, queues, files).
   - Apply seeds or fixtures.
   - Validate readiness (health checks, dependency checks).

2. **Create or Load Test Cases**
   - Generate test cases dynamically (often randomized) or load static, predefined cases.
   - If randomized, record the random seed for replay.
   - Validate that generated cases meet constraints.

3. **Run Test Cases**
   - Execute test cases in sequence or parallel based on the tester’s concurrency model.
   - Enforce timeouts and rate limits.
   - Capture raw inputs, outputs, and intermediate state where useful.

4. **Collect Test Results**
   - Record outcomes, metrics, and diagnostics.
   - Store structured results suitable for analysis and reporting.

5. **Verify Pass/Fail**
   - Apply deterministic assertions where possible.
   - Allow approximate or statistical verification for stochastic behavior.
   - Explicitly document acceptance criteria.

6. **Log Tests**
   - Persist all results to the designated database table.
   - Include metadata such as test purpose, tester name, seed, run ID, environment.

## Test Case Strategy

### Dynamic Tests (Randomized)
Dynamic tests explore a wide input space and uncover edge cases.
- Use controlled randomness with a seed.
- Store the seed in results for deterministic replay.
- Provide a “replay mode” that reruns specific cases from stored parameters.

### Static Tests (Hard-Coded)
Static tests validate known invariants and regressions.
- Keep these minimal but critical.
- Use explicit inputs and expected outputs.
- Use them as smoke tests and baseline verification.

### Hybrid Approach
Most testers should combine both:
- Static tests for core invariants.
- Dynamic tests to explore variability and emergent behavior.

## Configuration and Test Selection

When running AutoTester, a user should be able to specify:
- **Purpose**: e.g., `smoke`, `regression`, `load`, `fuzz`, `compliance`.
- **Type**: e.g., `unit`, `integration`, `end-to-end`.
- **Scope**: specific testers or test suites to run.
- **Environment**: local, staging, test, or CI environment.
- **Seed**: fixed seed for deterministic replay.
- **Concurrency**: level of parallel execution.

The CLI should map these to tester selection and configuration options.

## Logging and Database Storage
All test runs must be logged in a database table. The exact schema can vary, but should include at least:
- **Run ID**: unique identifier for the AutoTester run.
- **Tester Name**: which tester produced the result.
- **Test Case ID**: unique ID for a case (or derived from parameters).
- **Timestamp**: start/end timestamps.
- **Result**: pass/fail/skip/error.
- **Seed**: randomness seed for replay.
- **Environment**: environment metadata.
- **Parameters**: serialized test input and context.
- **Output**: observed output or response.
- **Diagnostics**: error messages, traces, or additional notes.

Logging should be:
- Reliable (no partial writes).
- Structured (JSON columns or normalized tables).
- Queryable for analytics and debugging.

## Determinism and Replay
To make tests reproducible:
- Always log the random seed for each run.
- If test cases are generated, store the generated inputs or enough parameters to regenerate them.
- Provide CLI options for `--seed` and `--replay`.

## Isolation and Safety
AutoTester must avoid interfering with production systems:
- Never run against production unless explicitly allowed.
- Use explicit environment selection flags.
- Perform safety checks (e.g., confirm DB host is non-prod).
- Use namespace or tenant isolation for test data.

## Test Data Management
Test data should be:
- Generated or seeded in a controlled way.
- Cleaned up after the run (unless retention is needed for debugging).
- Versioned if fixtures are stored in files.

## Concurrency and Performance
For large test suites:
- Allow configurable concurrency per tester.
- Ensure thread-safety of shared resources.
- Avoid global state unless protected.
- Capture timing metrics (latency, throughput).

## Error Handling and Reporting
Errors should be:
- Clearly classified (setup failure vs. test failure vs. infrastructure error).
- Logged with enough context to debug.
- Reported in summary output.

AutoTester should provide:
- A run-level summary (pass/fail counts, duration).
- Per-tester summaries.
- Optionally, machine-readable output (JSON report).

## Extensibility and Maintenance
To keep the system evolvable:
- Testers should register themselves via a registry pattern.
- AutoTester should load testers dynamically or via explicit registration.
- Avoid coupling testers to specific app internals unless necessary.

Naming conventions (recommended):
- Tester files: `*_tester.go`
- Tester IDs: `module.feature.variant`

## Example Layout (Recommended)

- `shared/go/api/autotesters/`
  - `tester_auth.go`
  - `router_tester.go`
- `ChenWeb/server/api/autotesters/`
  - `checkout_tester.go`
  - `search_tester.go`
- `ChenWeb/server/cmd/autotester/`
  - `main.go`
  - `config.go`
  - `registry.go`

## Example Tester Responsibilities (Pseudo)

```go
// 1. Validate config
// 2. Prepare environment
// 3. Generate test cases
// 4. Run cases
// 5. Verify outputs
// 6. Log results
```

## Integration with CI/CD
AutoTester should be runnable in CI:
- Provide non-interactive CLI flags.
- Return non-zero exit code on failure.
- Output concise logs plus a structured report file if needed.

## Summary
AutoTester provides a consistent, extensible approach to automated testing across shared modules and application services. It separates test orchestration (AutoTester) from test logic (Tester), provides reproducibility, logs results to a database, and supports configuration-driven selection of test purpose and type.

This document defines the architecture, lifecycle, logging requirements, and operational safeguards necessary to implement AutoTester in a reliable and maintainable way.
