# Testbot - Software Development Requirements

**Version:** 0.1 (Initial Iteration)
**Language:** Go
**Status:** Draft - Some sections marked [TBD] for further refinement

---

## Table of Contents

1. [Overview](#1-overview)
2. [Design Principles](#2-design-principles)
3. [Test Model](#3-test-model)
4. [Test Cases](#4-test-cases)
5. [Test Session](#5-test-session)
6. [Control Flow](#6-control-flow)
7. [Logging and Reporting](#7-logging-and-reporting)
8. [Architecture](#8-architecture)
9. [Configuration](#9-configuration)
10. [Interfaces](#10-interfaces)
11. [Error Handling](#11-error-handling)
12. [Glossary](#12-glossary)

---

## 1. Overview

Testbot is a program written in Go that automates testing a System-Under-Test (SUT).
It randomly generates test cases,
executes them automatically, collects results, and verifies whether the tests pass or fail.

### 1.1 Goals

The primary goal of Testbot is to automate the testing of a SUT as simply as possible yet as automatically and comprehensively as possible.

### 1.2 Non-Goals

- Testbot is **not** a unit testing framework. It tests a SUT as a black box.
- Testbot does **not** require knowledge of the SUT's internal implementation.
- Testbot does **not** replace manual exploratory testing or formal verification.

---

## 2. Design Principles

Testbot is designed based on the following six principles:

### 2.1 Test Model

Testbot treats the SUT as a black box. The SUT exposes a number of parameters (SUT Parameters) that control or affect its usage. The set of SUT parameters defines a Test Model, which serves as the foundation for Testbot.

Refer to [Section 3: Test Model](#3-test-model) for details.

### 2.2 Randomness

Tests MUST be generated randomly. No test case is hand-crafted. The randomness ensures broad coverage of the SUT's parameter space without human bias.

### 2.3 Closeness

The randomness is not blind. The random generation MUST closely simulate how the SUT is used in the real world. Parameter values are generated according to weighted probability distributions that reflect realistic usage patterns.

**Example:** Username length distribution for a login system:

| Generation Case | Weight | Assumption |
|---|---|---|
| length in [3, 15] | 100 (high) | Normal behavior |
| length in [1, 2] | 3 (very low) | Allowed, though rarely used |
| length in [16, 30] | 40 (low) | Normal, but not very common |
| length in [31, 64] | 15 (lower) | Rare but allowed |
| length in [65, 128] | 5 (even lower) | Very rare but allowed |
| length > 128 | 2 (very low) | Not allowed, should be rejected by SUT |

When randomly generating parameter values, the generator follows weighted distributions like the above to simulate real-world usage patterns while still exercising edge cases and invalid inputs.

### 2.4 Verifiability

When generating a test case, the generator MUST also produce the expected results so that pass/fail can be determined by comparing actual results against expected results.

**Example:** If the SUT is a Login service, starting with no users:

| Step | Test Case | Expected Result | Reason |
|---|---|---|---|
| 1 | A valid login attempt | "login failed" | The system has no users yet |
| 2 | Create a user (alice) | "user created" | Should succeed since the user doesn't exist |
| 3 | Login with a username not in the database | "login failed" | User not in the database |
| 4 | Login with the username just created (alice) | "login success" | The user exists in the database |

Generating verifiable test cases is extremely important. This is achievable because during test case generation, we can establish any assumptions needed so that results are predictable.

**Rule: Assumptions.** The torturer MUST implement the assumptions required by the test case generation module. In the login example, "Start SUT with no users" is an assumption that makes it possible to accurately predict expected results.

**Rule: State Tracking.** The torturer MUST keep track of what has happened so far in the SUT during a test session. In the login example, tracking which users have been created is essential for accurately predicting login pass/fail outcomes.

### 2.5 Automation

Testbot automates the entire testing pipeline:

1. Randomly generate one batch of test cases
2. Automatically execute them against the SUT
3. Collect the results
4. Analyze the results to determine pass/fail
5. Log the activities and results
6. Determine whether to stop the test or continue with the next batch

### 2.6 Idempotency

Testbot assumes: "With the same SUT (i.e., exactly the same software version, hardware, and configurations) and the same set of test cases, the results SHOULD be 'identical' regardless of how many times the test cases are run."

"Identical" is defined at the **semantic level**. For instance, when a login test case runs successfully, the pass/fail determination is based on the essential outcome. Non-essential information (e.g., response time, timestamps) is ignored during comparison.

---

## 3. Test Model

### 3.1 Definition

A Test Model is defined as:

```
TM = {SUT, C, T, P}
```

Where:

| Symbol | Name | Description |
|---|---|---|
| SUT | System Under Test | The system being tested, treated as a black box |
| C | Configuration | The configurations for the SUT and Testbot. See [3.3 Configuration](#33-configuration) |
| T | Tools | A set of tools, command lines, and APIs to interact with the SUT. See [3.4 Tools](#34-tools) |
| P | Parameters | A set of parameters [p1, p2, ..., pn] that control/affect the usage of the SUT. See [3.5 SUT Parameters](#35-sut-parameters) |

### 3.2 SUT Definition

A SUT is defined by a markdown document: `SUT.md`

This document describes:
- What the SUT does
- How users interact with it
- What operations it supports
- What constitutes valid and invalid inputs
- Any constraints, rate limits, or behavioral rules

### 3.3 Configuration

| Config Item | Default | Description |
|---|---|---|
| `torturer_name` | (required) | The torturer's name. Must be unique and non-empty |
| `log_level` | `"full"` | Log verbosity. Currently only `"full"` is supported |
| `log_dbname` | `"torturer"` | Database name for torturer logs |
| `log_tablename` | `"test_logs"` | Table name for torturer logs |
| `max_errors` | `10` | Maximum number of errors (execution failures, timeouts, etc.) before terminating the test |
| `max_failed` | `10` | Maximum number of failed test cases before terminating the test |
| `test_dur` | `100` | Test duration in seconds |
| `num_tcs_to_run` | `100` | Number of test cases to run. If both `test_dur` and `num_tcs_to_run` are configured, the test finishes when whichever limit is reached first |
| `sut_config` | `"./sut_config"` | Path to SUT configuration. Set to `"none"` if the SUT requires no configuration |
| `seed` | (random) | Random seed for reproducibility. If omitted, a random seed is generated and logged |
| `batch_size` | [TBD] | Number of test cases per batch |

### 3.4 Tools

`T` is a set of tools required for Testbot to run test cases and manipulate the SUT. Tools are documented in a `TOOLS.md` file specific to the SUT.

Tools may include:
- CLI commands to start/stop/reset the SUT
- API endpoints to interact with the SUT
- Scripts to prepare/clean up the SUT environment
- Utilities to retrieve results from the SUT

### 3.5 SUT Parameters

SUT parameters are the inputs that control the behavior of the SUT. They are used to generate test cases.

Each SUT parameter must define:
1. **Name** - Identifier for the parameter
2. **Type** - Data type (string, integer, enum, etc.)
3. **Valid Value Range** - The set of acceptable values
4. **Invalid Value Ranges** - Values outside the valid range, used for negative testing
5. **Weighted Distribution** - Probability weights for different value ranges (Closeness principle)
6. **Dependencies** - Other parameters this one depends on (e.g., "password" only applies when operation is "login with password")

**Example** (Login SUT):

| Parameter | Type | Valid Range | Description |
|---|---|---|---|
| Operation | enum | {signup_email, signup_google, signup_github, signup_userpass, login_email, login_google, login_github, forgot_password} | The operation to perform |
| Email | string | Valid email format, length in [N, M], charset restricted | Applicable to operations requiring email |
| UserName | string | Length in [n, m], charset restricted | Applicable to username+password operations |
| Password | string | Length in [p, q], must meet complexity rules | Applicable to password-based operations |
| URL | string | Must be a valid URL | The URL for the Login page |
| Timeout | integer | Range [n, m] | Max seconds before a session must finish |
| MaxTries | integer | Range [r, s] | Max login attempts per N seconds |
| MaxEmails | integer | Range [x, y] | Max different emails used per N seconds |

### 3.6 SUT Parameter Value Generation

1. Parameter values MUST be generated randomly
2. Generation MUST produce both valid and invalid values
3. The distribution of generated values MUST follow the Closeness principle (weighted probability distributions)
4. Generated values must cover edge cases: boundary values, empty values, maximum length values, special characters, etc.

---

## 4. Test Cases

### 4.1 Definition

A Test Case is defined as:

```
TC = {C, M, V, R, E, T}
```

| Symbol | Name | Description |
|---|---|---|
| C | Setup Commands | A set of commands or tools to prepare the SUT before executing the test case. May use values in V. Can be empty |
| M | Method | The method (command, API call, etc.) to actually run the test |
| V | Values | An array of name-value pairs, where name identifies the parameter and value is the generated value |
| R | Result Retrieval | A method to retrieve test results after execution |
| E | Expected Results | The expected outcome, expressed as a match pattern, regular expression, or structured comparison |
| T | Timer | Timeout in seconds. The test case MUST finish before this timer expires, otherwise it fails |

### 4.2 Test Case Execution

The control flow of executing a single test case:

```
1. Run setup commands (C) to prepare the SUT for this test case
2. Start timer (T)
3. Execute method (M) with values (V)
4. IF the test case does not finish before timer T expires:
     -> Mark test case as TIMEOUT (fail)
5. ELSE:
     -> Stop the timer
     -> Use retrieval method (R) to collect actual results
     -> Compare actual results against expected results (E)
     -> Mark test case as PASS or FAIL
6. Log the execution details and outcome
7. Determine whether to continue testing or terminate
```

### 4.3 Test Case States

A test case can be in one of the following states:

| State | Description |
|---|---|
| `PASS` | Actual results match expected results |
| `FAIL` | Actual results do not match expected results |
| `TIMEOUT` | Test case did not finish within the timer |
| `ERROR` | An unexpected error occurred during setup or execution (e.g., SUT crashed, network error) |
| `SKIP` | Test case was skipped (e.g., dependency not met) |

### 4.4 Expected Result Matching

[TBD] Define the matching strategies:

- **Exact match** - Actual result must exactly equal expected result
- **Pattern match** - Actual result must match a regex or glob pattern
- **Semantic match** - Actual result must be semantically equivalent (ignoring non-essential fields like timestamps)
- **Custom matcher** - A user-defined function that returns pass/fail

---

## 5. Test Session

### 5.1 Definition

A Test Session is a single run of the Testbot against a SUT. It consists of one or more batches of test cases.

```
Session = {SessionID, TM, Batches[], State, StartTime, EndTime, Summary}
```

### 5.2 Session State

A session maintains the following state throughout its lifecycle:

- **SUT State Tracker** - Tracks what has been done to the SUT (e.g., users created, data inserted) to support the Verifiability principle
- **Test Statistics** - Running counts of pass/fail/error/timeout/skip
- **Random Seed** - The seed used for this session (for reproducibility)

### 5.3 Batches

A batch is a group of test cases generated and executed together. The Testbot generates test cases in batches because:

1. Batch generation allows the generator to create sequences of related test cases (e.g., create user, then login with that user)
2. State tracking is maintained across test cases within and across batches
3. The decision to continue or stop is made after each batch

### 5.4 Termination Conditions

The test session terminates when ANY of the following conditions is met:

1. `num_tcs_to_run` test cases have been executed
2. `test_dur` seconds have elapsed
3. The number of errors exceeds `max_errors`
4. The number of failed test cases exceeds `max_failed`
5. The user manually interrupts the session (e.g., Ctrl+C)

---

## 6. Control Flow

### 6.1 High-Level Flow

```
1. Load configuration
2. Validate configuration
3. Initialize the SUT (apply sut_config, reset state if needed)
4. Initialize the session (create session ID, set random seed, init state tracker)
5. LOOP:
   a. Generate a batch of test cases (using random generation + state tracker)
   b. FOR each test case in the batch:
      i.   Execute the test case (setup -> run -> collect -> compare)
      ii.  Log the result
      iii. Update session statistics and state tracker
      iv.  Check termination conditions -> if met, BREAK
   c. Check termination conditions -> if met, BREAK
6. Generate summary report
7. Clean up (optional: reset SUT state)
```

### 6.2 SUT Lifecycle

[TBD] Define how Testbot manages the SUT lifecycle:

- How the SUT is started/stopped
- How the SUT is reset to a known state before a session
- Whether the SUT runs in-process, as a separate process, or as a remote service

---

## 7. Logging and Reporting

### 7.1 Log Storage

Logs are stored in a database (PostgreSQL):
- Database: configured by `log_dbname` (default: `"torturer"`)
- Table: configured by `log_tablename` (default: `"test_logs"`)

### 7.2 Log Schema

[TBD] Define the log table schema. At minimum, each log entry should contain:

| Column | Type | Description |
|---|---|---|
| `id` | serial | Primary key |
| `session_id` | string | Unique session identifier |
| `batch_id` | integer | Batch number within the session |
| `tc_index` | integer | Test case index within the batch |
| `tc_values` | jsonb | The parameter values (V) used |
| `expected_result` | text | Expected result (E) |
| `actual_result` | text | Actual result from the SUT |
| `status` | enum | PASS, FAIL, TIMEOUT, ERROR, SKIP |
| `duration_ms` | integer | Execution time in milliseconds |
| `error_message` | text | Error message if status is ERROR |
| `timestamp` | timestamptz | When the test case was executed |

### 7.3 Session Summary Report

At the end of a session, Testbot generates a summary report containing:

- Session ID, start time, end time, total duration
- Random seed used
- Configuration used
- Total test cases: executed, passed, failed, timed out, errored, skipped
- Pass rate
- Termination reason
- [TBD] Top failure categories / patterns

---

## 8. Architecture

### 8.1 Component Overview

```
+-------------------------------------------------------------------+
|                          Testbot                                  |
|                                                                    |
|  +--------------+    +----------------+    +-------------------+   |
|  | Config       |    | Test Model     |    | Session Manager   |   |
|  | Loader       |--->| (SUT, C, T, P) |--->| (state, stats)    |   |
|  +--------------+    +----------------+    +-------------------+   |
|                             |                       |              |
|                             v                       v              |
|                      +----------------+    +-------------------+   |
|                      | Test Case      |    | Test Case         |   |
|                      | Generator      |    | Executor          |   |
|                      | (random, close)|    | (run, collect)    |   |
|                      +----------------+    +-------------------+   |
|                                                     |              |
|                                                     v              |
|                      +----------------+    +-------------------+   |
|                      | Result         |    | Logger /          |   |
|                      | Comparator     |    | Reporter          |   |
|                      +----------------+    +-------------------+   |
+-------------------------------------------------------------------+
                              |
                              v
                     +------------------+
                     |   SUT (Black Box) |
                     +------------------+
```

### 8.2 Key Components

| Component | Responsibility |
|---|---|
| **Config Loader** | Loads and validates the Testbot and SUT configuration |
| **Test Model** | Holds the SUT definition, parameters, tools, and configuration |
| **Test Case Generator** | Randomly generates test cases with expected results, following Closeness and Verifiability principles |
| **Session Manager** | Manages session lifecycle, state tracking, statistics, and termination conditions |
| **Test Case Executor** | Executes test cases: runs setup commands, invokes test method, enforces timeouts, collects results |
| **Result Comparator** | Compares actual results against expected results using configured matching strategy |
| **Logger / Reporter** | Persists test case logs to the database and generates the session summary report |

### 8.3 SUT Adapter Interface

[TBD] Each SUT requires an adapter that implements a standard interface. This adapter translates Testbot's generic operations into SUT-specific commands.

```go
// Conceptual interface - exact API TBD
type SUTAdapter interface {
    // Initialize the SUT to a known state
    Setup(config SUTConfig) error

    // Execute a test case against the SUT
    Execute(tc TestCase) (ActualResult, error)

    // Retrieve results if not returned by Execute
    RetrieveResult(tc TestCase) (ActualResult, error)

    // Clean up / reset the SUT
    Teardown() error
}
```

### 8.4 Parameter Generator Interface

[TBD] Each SUT parameter type needs a generator that respects the Closeness principle.

```go
// Conceptual interface - exact API TBD
type ParamGenerator interface {
    // Generate a random value according to weighted distribution
    Generate(rng *rand.Rand) interface{}

    // Return the parameter definition (ranges, weights, etc.)
    Definition() ParamDefinition
}
```

---

## 9. Configuration

### 9.1 Configuration Files

A Testbot instance for a specific SUT requires the following files:

| File | Required | Description |
|---|---|---|
| `SUT.md` | Yes | Describes the SUT: what it does, operations, constraints |
| `TOOLS.md` | Yes | Documents the tools available to interact with the SUT |
| `torturer.yaml` | Yes | Testbot configuration (see [3.3 Configuration](#33-configuration)) |
| `sut_config` | Conditional | SUT-specific configuration. Set `sut_config: none` if not needed |

### 9.2 Configuration File Format

[TBD] Define the exact YAML/JSON schema for `torturer.yaml`, including:
- How SUT parameters are declared
- How weighted distributions are specified
- How tools are referenced
- How expected result matchers are configured

---

## 10. Interfaces

### 10.1 CLI Interface

[TBD] Define the CLI commands:

```
torturer run <config-path>       # Run a test session
torturer validate <config-path>  # Validate configuration without running
torturer report <session-id>     # View a past session report
torturer replay <session-id>     # Re-run a past session (using saved seed + config)
```

### 10.2 Programmatic Interface

[TBD] Define the Go API for embedding Testbot in other programs or for writing SUT adapters.

---

## 11. Error Handling

### 11.1 Error Categories

| Category | Description | Action |
|---|---|---|
| **Configuration Error** | Invalid or missing configuration | Abort before starting the session |
| **SUT Setup Error** | Failed to initialize or reset the SUT | Abort session |
| **Execution Error** | Unexpected error during test case execution (not a test failure) | Log, increment error count, continue |
| **Timeout** | Test case exceeded its timer | Log as TIMEOUT, continue |
| **Infrastructure Error** | Database unavailable, network error, etc. | [TBD] Retry policy or abort |

### 11.2 Graceful Shutdown

When the user interrupts the session (e.g., Ctrl+C):
1. Stop generating new test cases
2. Wait for the currently executing test case to finish (or timeout)
3. Log all collected results
4. Generate the session summary report
5. Exit cleanly

---

## 12. Glossary

| Term | Definition |
|---|---|
| **SUT** | System Under Test. The software being tested, treated as a black box |
| **Test Model** | The formal model TM = {SUT, C, T, P} that describes everything needed to test a SUT |
| **Test Case** | A single test defined as TC = {C, M, V, R, E, T} |
| **Test Session** | A complete run of the Testbot against a SUT, consisting of one or more batches |
| **Batch** | A group of test cases generated and executed together |
| **Closeness** | The principle that random generation must approximate real-world usage patterns |
| **Verifiability** | The principle that every test case must have predictable expected results |
| **Idempotency** | The principle that the same tests on the same SUT should produce semantically identical results |
| **State Tracker** | Internal bookkeeping of what has been done to the SUT during a session |
| **SUT Adapter** | The SUT-specific implementation that translates Testbot operations into SUT commands |

---

## Appendix A: Open Items [TBD]

The following items are identified for further refinement in subsequent iterations:

1. **Parameter Generator DSL** - How to declaratively specify parameter types, ranges, and weighted distributions in configuration files
2. **SUT Adapter Contract** - Exact Go interfaces and lifecycle hooks for SUT adapters
3. **Result Matching Strategies** - Detailed specification of exact, pattern, semantic, and custom matching
4. **Batch Generation Strategy** - How test cases within a batch are sequenced to create meaningful scenarios (e.g., create-then-use patterns)
5. **Concurrency** - Whether test cases within a batch can be executed in parallel, and implications for state tracking
6. **Replay Mechanism** - Exact semantics of replaying a session: what is preserved, what can differ
7. **SUT Lifecycle Management** - How Testbot starts, stops, and resets the SUT
8. **Configuration Schema** - Formal YAML/JSON schema for `torturer.yaml`
9. **Database Migration** - Schema versioning and migration strategy for the log database
10. **Reporting Formats** - Additional output formats (JSON, HTML, etc.) beyond database storage
