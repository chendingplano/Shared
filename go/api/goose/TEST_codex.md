# Goose Package Test Documentation

This document explains the test suite in `goose_test.go` for:

- `shared/go/api/goose/goose.go`

## Purpose

The tests are designed to validate the package's behavior in three major areas:

1. Pure logic and helper functions
2. Migration file creation behavior
3. Migrator lifecycle and wrapper initialization behavior

The suite emphasizes deterministic coverage for config handling, edge cases, and no-op safety when the goose provider is not initialized.

## Test File

- `goose_test.go`

## Test Logger

A minimal `testLogger` implements `ApiTypes.JimoLogger` so tests can call package APIs without relying on production logging infrastructure.

## Covered Test Areas

### 1. Dialect Mapping

- `TestDialectFor`
- Verifies:
  - `ApiTypes.PgName` maps to `goose.DialectPostgres`
  - `ApiTypes.MysqlName` maps to `goose.DialectMySQL`
  - Unsupported values return an error

### 2. Config Defaults and Overrides

- `TestApplyDefaults`
- Verifies:
  - Default values for migrations dir, table name, verbose mode, and out-of-order setting
  - Explicit config values override defaults correctly

### 3. Migration File Discovery

- `TestHasMigrationFiles`
- Verifies:
  - `nil` filesystem handling
  - Empty directory behavior
  - `.sql` and `.go` migration detection
  - Error propagation on `ReadDir` failures

### 4. Slug and SQL Content Builders

- `TestSlugify`
- Verifies normalization, special character handling, fallback to `migration`, and 60-char truncation.

- `TestBuildMigrationSQL`
- Verifies generated goose sections and trimming behavior with and without `Down` SQL.

### 5. CreateMigration Behavior

- `TestCreateMigration`
- Verifies:
  - Validation errors when `MigrationsDir` is missing
  - Validation errors when `upSQL` is empty
  - Generated filename format: `YYYYMMDDHHMMSS_slug.sql`
  - Written file content matches expected goose format

### 6. CreateAndApply Behavior

- `TestCreateAndApply`
- Verifies:
  - Success path when migration FS contains no files (provider remains nil, file still created)
  - Rebuild failure path returns the generated filename plus wrapped error

### 7. Provider Rebuild with Empty Migrations

- `TestRebuildProviderWithNoMigrations`
- Verifies provider remains nil and no error when no migration files exist.

### 8. NewWithDB Initialization Paths

- `TestNewWithDB`
- Verifies:
  - Error for unsupported DB type
  - Valid migrator creation for empty migration directory (provider nil)

### 9. RunMigrations Behavior

- `TestRunMigrations`
- Verifies:
  - Directory creation failure path
  - Successful initialization flow with empty migrations

### 10. Wrapper Initializers and Globals

- `TestRunWrapperInitializers`
- Verifies `RunProjectMigrations`, `RunSharedMigrations`, and `RunAutoTesterMigrations` for:
  - Nil DB errors
  - Successful first initialization
  - Repeated initialization guard errors

### 11. No-Op Safety with Nil Provider

- `TestNoOpMethodsWhenProviderNil`
- Verifies safe no-op behavior for:
  - `Up`, `UpByOne`, `UpTo`, `Down`, `DownTo`
  - `Status`, `GetVersion`, `HasPending`, `ListSources`

## How to Run

From workspace module root:

```bash
cd /Users/cding/Workspace/shared/go
go test -v ./api/goose -cover
```

`-v` is required to display per-test-case detail logs.

## Per-Case Runtime Output

Each test case now logs:

1. Purpose of the test case
2. Actual statement being executed/validated
3. Execution status (`success` / `fail`)
4. Error message hint if failed (with full assertion output from `go test`)
5. Time used (milliseconds)

## Latest Verified Result

Most recent run after creating the suite:

- Command: `go test ./api/goose -cover`
- Result: `ok`
- Coverage: `68.6% of statements`

## Coverage Notes

Current tests strongly cover deterministic logic and no-provider branches. Some provider-backed runtime paths (e.g., full migration execution against a live DB/provider object) are intentionally not exercised in this suite to keep tests fast and deterministic without integration DB dependencies.

If deeper provider execution coverage is desired, add integration tests with a real test database and migration fixtures.
