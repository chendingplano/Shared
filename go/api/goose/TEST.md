# TEST_CLAUDE.md — goose_pg_test.go

**Package:** `github.com/chendingplano/shared/go/api/goose`
**File:** `goose_pg_test.go`
**Created:** 2026/03/24 by Claude Code and Chen Ding

---

## Overview

`goose_pg_test.go` is a comprehensive, PostgreSQL-focused test suite for the
`goose` migration package. It extends Go's standard `testing` framework with a
lightweight custom layer that adds:

- **TCID tracking** — every test case receives a globally unique integer ID
- **Structured output** — verbose mode prints per-case detail lines
- **PostgreSQL audit log** — results are inserted into `autotester.test_log`
- **Markdown report** — a human-readable report is written to disk after each run

There are **71 test cases** in two tiers:

| Tier | TCIDs | Count | PostgreSQL required |
|---|---|---|:---:|
| Unit | 1 – 57 | 57 | No |
| Integration | 58 – 71 | 14 | Yes |

Distribution: **≈ 71 % correct-path** · **≈ 29 % error-path**

---

## Running the Tests

### Unit tests only (no database needed)

```bash
cd shared/go

# brief output
go test ./api/goose/ -run TestGoosePG -testname goose_pg

# verbose output (per-case detail)
go test ./api/goose/ -run TestGoosePG -testname goose_pg -v
```

### Unit + integration tests (PostgreSQL required)

```bash
# via environment variable (recommended for CI)
export PGTEST_DSN="postgres://postgres:secret@localhost/testonly_goose?sslmode=disable"
go test ./api/goose/ -run TestGoosePG -testname goose_pg -v

# via flag (useful locally)
go test ./api/goose/ -run TestGoosePG -testname goose_pg \
    -pg-dsn "postgres://postgres:secret@localhost/testonly_goose?sslmode=disable" -v
```

> **Safety rule:** The PostgreSQL database name used for integration tests
> **must** start with `testonly_`. Tests create and drop real tables; never
> point them at a production database.

### Running all package tests (unit + the existing codex suite)

```bash
go test ./api/goose/ -v
```

---

## CLI Flags

| Flag | Default | Description |
|---|---|---|
| `-testname <name>` | `goose_pg` | Label for this run. Used as the key in `autotester.test_log` and as part of the report filename. |
| `-pg-dsn <dsn>` | *(empty)* | PostgreSQL connection string. Overrides `$PGTEST_DSN`. |
| `-v` | *(off)* | Standard Go verbose flag. Triggers per-case detail output via `t.Log`. |

---

## Output Modes

### Brief (default)

Standard Go test output — one `PASS`/`FAIL`/`SKIP` line per subtest:

```
--- PASS: TestGoosePGUnit_DialectMapping/TCID01 (0.00s)
--- PASS: TestGoosePGUnit_DialectMapping/TCID02 (0.00s)
...
PASS
[goose_pg_test] Report written → /Users/.../shared/docs/tests/testreport_goose_pg.md
```

### Verbose (`-v`)

Each test case also emits a structured block via `t.Log`:

```
--- TCID 01 ---
  Purpose   : dialectFor maps PgName to gooselib.DialectPostgres
  Statement : dialectFor(ApiTypes.PgName)
  Result    : PASS
  Time      : 0 ms
```

Fields:

| Field | Meaning |
|---|---|
| `Purpose` | Human-readable description of what is being verified |
| `Statement` | The Go expression or SQL statement exercised (empty for pure state checks) |
| `Result` | `PASS` or `FAIL` |
| `Error` | Present only on failure; directs to the subtest output |
| `Time` | Wall-clock duration in milliseconds |

---

## PostgreSQL Audit Log

On every run where `PGTEST_DSN` / `-pg-dsn` is set, results are persisted to
`autotester.test_log`. The schema is created automatically if it does not exist.

Important runtime behavior:
- If `PGTEST_DSN` is empty and `-pg-dsn` is not provided, the PostgreSQL setup branch is skipped entirely. In that case, no database/table creation and no DB logging will occur.
- `go test` output is compact by default. Setup/debug logs (including `TestMain` logs) may not be shown unless you run with `-v` or the test fails.

Quick verification command:

```bash
go test ./api/goose/ -run TestGoosePG -v -count=1 \
  -pg-dsn "postgres://postgres:secret@localhost/testonly_goose?sslmode=disable"
```

### Table definition

```sql
CREATE SCHEMA IF NOT EXISTS autotester;

CREATE TABLE IF NOT EXISTS autotester.test_log (
    id          BIGSERIAL PRIMARY KEY,
    run_id      TEXT        NOT NULL,          -- "<testname>_<unix_ms>"
    testname    TEXT        NOT NULL,
    tcid        INTEGER     NOT NULL,
    purpose     TEXT        NOT NULL,
    statement   TEXT,                          -- NULL when not applicable
    result      TEXT        NOT NULL CHECK (result IN ('PASS','FAIL','SKIP')),
    error_msg   TEXT,                          -- NULL on PASS
    time_ms     BIGINT      NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (testname, tcid, run_id)
);
```

### Querying results

```sql
-- Confirm table exists
SELECT schemaname, tablename
FROM   pg_tables
WHERE  schemaname = 'autotester'
  AND  tablename  = 'test_log';

-- Latest run for a given test name
SELECT tcid, purpose, result, time_ms
FROM   autotester.test_log
WHERE  testname = 'goose_pg'
ORDER  BY run_id DESC, tcid;

-- Failure summary across all runs
SELECT testname, run_id, COUNT(*) AS failures
FROM   autotester.test_log
WHERE  result = 'FAIL'
GROUP  BY testname, run_id
ORDER  BY run_id DESC;
```

---

## Markdown Report

After every run the framework writes a report to:

```
~/Workspace/<project>/docs/tests/testreport_<testname>.md
```

For this test suite that resolves to:

```
shared/docs/tests/testreport_goose_pg.md
```

The project name is derived automatically by stripping the `~/Workspace/`
prefix from the working directory and taking the first path component.

The report contains:
- Run metadata (project, package, timestamp)
- Summary table (total / pass / fail / pass-rate)
- Per-case detail table (TCID, purpose, statement, result, error, time)

---

## Test Case Reference

### Unit Tests (TCIDs 1 – 57)

#### `dialectFor` mapping (TCIDs 1 – 4)

| TCID | Kind | Description |
|---|:---:|---|
| 1 | ✓ | `PgName` → `gooselib.DialectPostgres` |
| 2 | ✓ | `MysqlName` → `gooselib.DialectMySQL` |
| 3 | ✗ | Unsupported type `"sqlite3"` returns error |
| 4 | ✗ | Empty string returns error |

#### `applyDefaults` (TCIDs 5 – 12)

| TCID | Kind | Description |
|---|:---:|---|
| 5 | ✓ | Default `MigrationsDir` is `"migrations"` |
| 6 | ✓ | Default `TableName` is `"db_migrations"` |
| 7 | ✓ | Default `Verbose` is `true` |
| 8 | ✓ | Default `AllowOutOfOrder` is `true` |
| 9 | ✓ | `Verbose="false"` produces `false` |
| 10 | ✓ | `AllowOutOfOrder="false"` produces `false` |
| 11 | ✓ | Custom `TableName` is preserved |
| 12 | ✓ | Custom `MigrationsDir` is preserved |

#### `hasMigrationFiles` (TCIDs 13 – 18)

| TCID | Kind | Description |
|---|:---:|---|
| 13 | ✓ | `nil` FS → `false`, no error |
| 14 | ✓ | Empty directory → `false` |
| 15 | ✓ | Directory with `.sql` file → `true` |
| 16 | ✓ | Directory with `.go` file → `true` |
| 17 | ✓ | Only `.txt`/`.md` files → `false` (extension filter) |
| 18 | ✗ | `ReadDir` failure propagates as error |

#### `slugify` (TCIDs 19 – 24)

| TCID | Kind | Description |
|---|:---:|---|
| 19 | ✓ | Mixed-case text → lowercase with underscores |
| 20 | ✓ | Consecutive special characters collapse to single `_` |
| 21 | ✓ | Whitespace-only → fallback `"migration"` |
| 22 | ✓ | 80-char input truncated to 60 |
| 23 | ✓ | Leading/trailing `_` stripped |
| 24 | ✓ | Digits preserved |

#### `buildMigrationSQL` (TCIDs 25 – 28)

| TCID | Kind | Description |
|---|:---:|---|
| 25 | ✓ | `-- +goose Up` section always present |
| 26 | ✓ | `-- +goose Down` emitted when `downSQL` is non-empty |
| 27 | ✓ | `-- +goose Down` absent when `downSQL` is empty/whitespace |
| 28 | ✓ | SQL content is trimmed inside `StatementBegin/End` |

#### `CreateMigration` (TCIDs 29 – 32)

| TCID | Kind | Description |
|---|:---:|---|
| 29 | ✗ | Empty `MigrationsDir` → error `MID_060221143011` |
| 30 | ✗ | Empty/whitespace `upSQL` → error `MID_060221143012` |
| 31 | ✓ | Valid args write a timestamped, correctly annotated file |
| 32 | ✓ | File omits Down section when `downSQL` is empty |

#### `NewWithDB` (TCIDs 33 – 34)

| TCID | Kind | Description |
|---|:---:|---|
| 33 | ✗ | Invalid db type → `"unsupported database type"` error |
| 34 | ✓ | Empty migrations dir → migrator with `nil` provider |

#### `rebuildProvider` (TCID 35)

| TCID | Kind | Description |
|---|:---:|---|
| 35 | ✓ | Empty `MigrationsFS` keeps `provider` as `nil` |

#### `CreateAndApply` (TCIDs 36 – 37)

| TCID | Kind | Description |
|---|:---:|---|
| 36 | ✓ | Empty `MigrationsFS` dir → file created, `provider` stays `nil`, no error |
| 37 | ✗ | `nil` DB with non-empty FS → rebuild fails; filename still returned alongside error |

#### Nil-provider no-ops (TCIDs 38 – 46)

| TCID | Kind | Description |
|---|:---:|---|
| 38 | ✓ | `Up(ctx)` → `nil` |
| 39 | ✓ | `UpByOne(ctx)` → `nil` |
| 40 | ✓ | `UpTo(ctx, 42)` → `nil` |
| 41 | ✓ | `Down(ctx)` → `nil` |
| 42 | ✓ | `DownTo(ctx, 0)` → `nil` |
| 43 | ✓ | `Status(ctx)` → `(nil, nil)` |
| 44 | ✓ | `GetVersion(ctx)` → `(0, nil)` |
| 45 | ✓ | `HasPending(ctx)` → `(false, nil)` |
| 46 | ✓ | `ListSources()` → `nil` |

#### `RunMigrations` (TCIDs 47 – 48)

| TCID | Kind | Description |
|---|:---:|---|
| 47 | ✗ | `MigrationsDir` points to existing file → mkdir error `MID_060221143005` |
| 48 | ✓ | Empty dir + valid `DBType` → migrator with `nil` provider |

#### Singleton wrapper initializers (TCIDs 49 – 57)

| TCID | Kind | Description |
|---|:---:|---|
| 49 | ✗ | `RunProjectMigrations` nil DB → error `MID_060221143035` |
| 50 | ✗ | `RunSharedMigrations` nil DB → error `MID_060221143002` |
| 51 | ✗ | `RunAutoTesterMigrations` nil DB → error `MID_060221143012` |
| 52 | ✓ | `RunProjectMigrations` first call succeeds; `ProjectMigrator` set |
| 53 | ✗ | `RunProjectMigrations` second call → error `MID_060221143034` |
| 54 | ✓ | `RunSharedMigrations` first call succeeds |
| 55 | ✗ | `RunSharedMigrations` second call → error `MID_060221143001` |
| 56 | ✓ | `RunAutoTesterMigrations` first call succeeds |
| 57 | ✗ | `RunAutoTesterMigrations` second call → error `MID_060221143011` |

---

### Integration Tests (TCIDs 58 – 71)

Skipped automatically when neither `PGTEST_DSN` nor `-pg-dsn` is set.

Each integration test function creates its own isolated migration table
(unique name per run) and registers a `t.Cleanup` to drop it.

#### `TestGoosePGIntegration_NewWithDB` (TCID 58)

| TCID | Kind | Description |
|---|:---:|---|
| 58 | ✓ | Real PG + empty dir → migrator with `nil` provider, no error |

#### `TestGoosePGIntegration_UpDownCycle` (TCIDs 59 – 64)

Exercises the complete migration lifecycle with a real table (`pg_test_tbl_cycle`).

| TCID | Kind | Description |
|---|:---:|---|
| 59 | ✓ | `Up` applies pending migration |
| 60 | ✓ | `HasPending` returns `false` after `Up` |
| 61 | ✓ | `GetVersion` returns version > 0 after `Up` |
| 62 | ✓ | `Status` returns one entry with `StateApplied` |
| 63 | ✓ | `Down` rolls back last migration |
| 64 | ✓ | `HasPending` returns `true` after `Down` |

#### `TestGoosePGIntegration_CreateAndApply` (TCIDs 65 – 69)

Creates a migration at runtime and verifies database state.

| TCID | Kind | Description |
|---|:---:|---|
| 65 | ✓ | `CreateAndApply` writes file and applies migration |
| 66 | ✓ | Migration file exists on disk after `CreateAndApply` |
| 67 | ✓ | `provider` is non-`nil` after `CreateAndApply` |
| 68 | ✓ | Target table exists in PostgreSQL after migration |
| 69 | ✗ | Duplicate `CREATE TABLE` (no `IF NOT EXISTS`) surfaces PG error |

#### `TestGoosePGIntegration_ListSources` (TCIDs 70 – 71)

Seeds two migration files and tests ordered listing and partial apply.

| TCID | Kind | Description |
|---|:---:|---|
| 70 | ✓ | `ListSources` returns two entries in ascending version order |
| 71 | ✓ | `UpTo(version)` applies exactly up to the given version |

---

## Internal Framework Reference

### `runTC`

```go
func runTC(t *testing.T, purpose, stmt string, fn func(t *testing.T))
```

Assigns the next atomic TCID, runs `fn` as a `t.Run` subtest named
`TCID<nn>`, records the outcome in `tcRecords`, and (in verbose mode)
calls `printVerbose`.

### `tcRecord`

```go
type tcRecord struct {
    TCID      int
    Purpose   string
    Statement string
    Result    string // "PASS" or "FAIL"
    ErrMsg    string
    TimeMs    int64
}
```

All records are accumulated in the package-level `tcRecords` slice, which
is safe for concurrent use via `tcRecMu`.

### `TestMain`

Orchestrates the full lifecycle:

1. `flag.Parse()` — resolves `-testname` and `-pg-dsn`
2. `sql.Open` + `Ping` — optional PostgreSQL setup; logs a warning and
   continues if unavailable
3. `ensureTestLogTable` — idempotently creates the audit schema/table
4. `m.Run()` — executes all `Test*` functions
5. `logResultsToDB` — bulk-inserts `tcRecords` in one transaction
6. `generateMarkdownReport` — writes the `.md` report file
7. `os.Exit(code)` — propagates the Go test exit code

### `pgFailFS`

A minimal `fs.FS` implementation that always returns a configurable error
from `ReadDir`. Used exclusively in TCID 18 to exercise the error-propagation
path of `hasMigrationFiles`.

---

## Adding New Test Cases

1. Choose the appropriate `TestGoosePGUnit_*` or `TestGoosePGIntegration_*`
   function (or create a new one following the naming convention).
2. Call `runTC` with a unique `purpose`, an optional `stmt`, and a test closure.
3. TCIDs are assigned automatically in call order — no manual numbering needed.
4. For integration tests, begin with `db := requirePGDB(t)` to auto-skip when
   no DSN is configured.

```go
runTC(t,
    "MyFunc returns X when Y",       // purpose
    "MyFunc(validInput)",             // stmt (empty string if not applicable)
    func(t *testing.T) {
        got, err := MyFunc(validInput)
        if err != nil {
            t.Fatalf("unexpected error: %v", err)
        }
        if got != expectedValue {
            t.Fatalf("got %v, want %v", got, expectedValue)
        }
    })
```

---

## See Also

- [`goose.go`](goose.go) — source under test
- [`goose_codex_test.go`](goose_codex_test.go) — companion unit-test suite
  (runs under the same `TestMain`, not TCID-tracked)
- [`shared/Documents/dev/goose-v1.md`](../../../../Documents/dev/goose-v1.md) — migration usage guide
- `shared/docs/tests/testreport_goose_pg.md` — most recent generated report
