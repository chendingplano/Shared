# Migration Tester Documentation

**Package:** `tester_migration`  
**File:** `tester_migration.go` (and related files)  
**Purpose:** Automated testing for the Goose database migration system

---

## Overview

The `tester_migration` package provides a comprehensive testing framework for validating the Goose database migration system. It tests migration apply/rollback cycles, version tracking, and edge cases in an isolated test environment.

### Key Features

- **Full migration lifecycle testing**: Apply (`Up`), rollback (`Down`), and targeted version operations
- **Stateful testing**: Tracks and validates migration state across operations
- **Synthetic migration generation**: Creates test migration files dynamically
- **Comprehensive test coverage**: 19+ static test cases plus dynamic case generation
- **Safety mechanisms**: Enforces `testonly_` naming conventions to prevent production accidents

---

## Architecture

### Core Components

```
┌─────────────────────────────────────────────────────────────┐
│                     MigrationTester                          │
├─────────────────────────────────────────────────────────────┤
│  - BaseTester (inherited)                                   │
│  - cfg: MigrationTesterConfig                               │
│  - dutDB: *sql.DB (Device Under Test)                       │
│  - testMigrationsDir: string                                │
│  - state: MigrationSUTState                                 │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                   MigrationTesterConfig                      │
├─────────────────────────────────────────────────────────────┤
│  - DUTDB: *sql.DB                                           │
│  - DUTDBType: "postgres" | "mysql"                          │
│  - MigrationsDir: string (must start with "testonly_")      │
│  - TableName: string (default: "db_migrations")             │
│  - NumDynamicCases: int (default: 80)                       │
│  - MaxMigrationsInPool: int (default: 20)                   │
└─────────────────────────────────────────────────────────────┘
```

### File Structure

| File | Purpose |
|------|---------|
| `tester_migration.go` | Core tester implementation, Prepare/Cleanup, migration file generation |
| `tester_migration_types.go` | Type definitions, constants, configuration structs |
| `tester_migration_state.go` | State synchronization and reset logic |
| `tester_migration_handlers.go` | Operation handlers (Up, Down, Status, etc.) |
| `tester_migration_cases.go` | Static and dynamic test case generation |

---

## Migration Operations

The tester supports the following Goose migration operations:

| Operation | Description |
|-----------|-------------|
| `Up` | Apply all pending migrations |
| `UpByOne` | Apply exactly one pending migration |
| `UpTo` | Apply migrations up to a target version |
| `Down` | Rollback one migration |
| `DownTo` | Rollback to a target version |
| `Status` | Get applied/pending migration status |
| `GetVersion` | Get current migration version |
| `HasPending` | Check if pending migrations exist |
| `CreateAndApply` | Create a migration file and apply it |
| `ListSources` | List all migration sources |

---

## Lifecycle

### 1. Preparation (`Prepare`)

Sets up the test environment:

1. Verifies DUT (Device Under Test) database connectivity
2. Validates DUT name starts with `testonly_`
3. Validates migrations directory starts with `testonly_`
4. Creates migrations directory
5. Drops goose tracking table from DUT
6. Drops all `testonly_` tables from DUT
7. Clears the migrations directory
8. Builds a pool of synthetic migration files
9. Initializes internal state

### 2. Test Execution (`RunTestCase`)

For each test case:

1. Resets DUT to the pre-state expected by the case
2. Builds a migrator with case-specific configuration
3. Dispatches to the appropriate operation handler
4. Observes side effects (table creation, file writes)
5. Syncs internal state from DUT ground truth

### 3. Cleanup (`Cleanup`)

Tears down the test environment:

1. Drops all `testonly_` tables from DUT
2. Drops the tracking table
3. Deletes all `.sql` files from the migrations directory

---

## State Management

### MigrationSUTState

```go
type MigrationSUTState struct {
    Applied        []MigrationRecord      // Applied migrations
    FilesInDir     []MigrationFile        // Migration files in directory
    Tables         map[string]bool        // testonly_ tables in DUT
    CurrentVersion int64                  // Highest applied version
}
```

### State Synchronization

The `syncState()` method queries the DUT to maintain accurate state:

1. Queries current version from tracking table
2. Queries all applied migrations
3. Scans migrations directory for `.sql` files
4. Queries all `testonly_` tables in DUT

### State Reset

The `resetToState()` method brings the DUT to a specific pre-state:

1. Drops tracking table and all `testonly_` tables
2. Clears and repopulates migrations directory
3. Rebuilds the migrator
4. Applies migrations using `UpByOne`
5. Verifies state matches expected state

---

## Test Cases

### Static Test Cases (19 total)

#### Category 1: Basic Apply Operations
| ID | Name | Purpose |
|----|------|---------|
| `TC_2026022301` | Apply all migrations from empty DB | Tests `Up` from empty database |
| `TC_2026022302` | Up is no-op when already current | Tests `Up` when fully applied |
| `TC_2026022303` | Apply migrations one by one | Tests `UpByOne` functionality |
| `TC_2026022304` | UpByOne returns error when all applied | Tests `UpByOne` error handling |
| `TC_2026022305` | UpTo applies up to target version | Tests `UpTo` functionality |
| `TC_2026022306` | UpTo returns error for nonexistent version | Tests `UpTo` error handling |

#### Category 2: Rollback Operations
| ID | Name | Purpose |
|----|------|---------|
| `TC_2026022307` | Roll back one migration | Tests `Down` with Down SQL |
| `TC_2026022308` | Down returns error when nothing applied | Tests `Down` error handling |
| `TC_2026022309` | DownTo rolls back to target version | Tests `DownTo` functionality |
| `TC_2026022310` | DownTo(0) rolls back all migrations | Tests full rollback |

#### Category 3: Status Inspection
| ID | Name | Purpose |
|----|------|---------|
| `TC_2026022311` | GetVersion returns 0 when empty | Tests version tracking |
| `TC_2026022312` | HasPending returns true | Tests pending detection |
| `TC_2026022313` | HasPending returns false when fully applied | Tests pending detection |
| `TC_2026022314` | Status returns correct counts | Tests status reporting |

#### Category 4: Migration Creation
| ID | Name | Purpose |
|----|------|---------|
| `TC_2026022315` | CreateAndApply writes and applies | Tests dynamic creation |
| `TC_2026022316` | CreateAndApply with empty downSQL | Tests edge case |
| `TC_2026022317` | CreateAndApply with invalid SQL | Tests error handling |

#### Category 5: Edge Cases
| ID | Name | Purpose |
|----|------|---------|
| `TC_2026022318` | Tracking table auto-created | Tests auto-creation |
| `TC_2026022319` | ListSources returns migration files | Tests source listing |

### Dynamic Test Cases

The `GenerateTestCases()` method creates additional test cases at runtime:

- **Configurable count**: Default 80 dynamic cases
- **Weighted operation selection**: Operations selected based on weights
- **Random pre-states**: Generates varied starting conditions
- **Comprehensive coverage**: Tests edge cases not covered by static cases

---

## Safety Mechanisms

### Naming Conventions

- **Database names**: Must start with `testonly_`
- **Migrations directory**: Must start with `testonly_`
- **Test tables**: All created with `testonly_` prefix

### Validation

```go
func (cfg *MigrationTesterConfig) Validate() error {
    if cfg.DUTDB == nil {
        return sql.ErrNoRows
    }
    // Validates DUTDBName starts with "testonly_"
    // Validates MigrationsDir starts with "testonly_"
    return nil
}
```

### Cleanup Guarantees

The `Cleanup()` method ensures:
- All `testonly_` tables are dropped
- Tracking table is removed
- All migration files are deleted

---

## Migration File Format

Generated migration files follow the Goose format:

```sql
-- +goose Up
-- +goose StatementBegin
CREATE TABLE testonly_table_01 (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS testonly_table_01
-- +goose StatementEnd
```

### Filename Format

```
{version}_{description}.sql
```

Example: `2026010112000001_create_table_01.sql`

---

## Error Codes

Error messages include traceable error codes in the format `MID_YYMMDD######`:

| Error Code | Description |
|------------|-------------|
| `MID_260224100001` | DUT not reachable |
| `MID_260224100002` | Invalid DUT name |
| `MID_260224100003` | Invalid migrations directory |
| `MID_260224100004` | Failed to create migrations directory |
| `MID_260224100005` | Failed to drop tracking table |
| `MID_260224100050` - `MID_260224100059` | Operation-specific errors |

---

## Usage Example

```go
import (
    "context"
    "database/sql"
    
    "github.com/chendingplano/shared/go/api/testers/tester-migration"
)

func main() {
    // Create configuration
    cfg := &tester_migration.MigrationTesterConfig{
        DUTDB:               testDB,
        DUTDBType:           "postgres",
        DUTDBName:           "testonly_migrations_test",
        MigrationsDir:       "testonly_migrations",
        TableName:           "db_migrations",
        NumDynamicCases:     80,
        MaxMigrationsInPool: 20,
    }
    
    // Create tester
    tester := tester_migration.NewMigrationTester(cfg)
    
    // Prepare test environment
    ctx := context.Background()
    if err := tester.Prepare(ctx); err != nil {
        log.Fatal(err)
    }
    defer tester.Cleanup(ctx)
    
    // Get test cases
    cases := tester.GetTestCases()
    
    // Run test cases
    for _, tc := range cases {
        result := tester.RunTestCase(ctx, tc)
        // Process result...
    }
}
```

---

## Side Effects

The tester observes and reports the following side effects:

| Side Effect | Description |
|-------------|-------------|
| `tracking_table_created` | Goose version tracking table was created |
| `schema_table_applied` | A testonly_ table was created by migration |
| `schema_table_dropped` | A testonly_ table was dropped by rollback |
| `migration_file_written` | A new migration file was created |

---

## Dependencies

- **Goose**: `github.com/pressly/goose/v3` - Migration framework
- **ApiTypes**: `github.com/chendingplano/shared/go/api/ApiTypes` - Migration configuration types
- **autotester**: `github.com/chendingplano/shared/go/api/autotester` - Testing framework base
- **databaseutil**: `github.com/chendingplano/shared/go/api/databaseutil` - Database utilities
- **sharedgoose**: `github.com/chendingplano/shared/go/api/goose` - Wrapped Goose implementation

---

## Best Practices

1. **Always use `testonly_` prefix**: Never test against production databases
2. **Call `Prepare()` and `Cleanup()`**: Ensure proper setup and teardown
3. **Check error codes**: Use `MID_` codes for debugging failures
4. **Review side effects**: Validate expected side effects occurred
5. **Use dynamic cases**: Enable `GenerateTestCases()` for comprehensive coverage

---

## Change Log

### 2026-02-24

- Initial implementation based on `tester-migration-qwen-v4.md`
- Removed six unused helper functions:
  - `getAppliedMigrations()` - Redundant with `syncState()`
  - `getCurrentVersion()` - Redundant with `syncState()`
  - `hasPendingMigrations()` - Handlers call `migrator.HasPending()` directly
  - `getMigrationStatus()` - Handlers call `migrator.Status()` directly
  - `listTables()` - `syncState()` queries tables inline
  - `listMigrationFiles()` - `syncState()` scans files inline

---

## Related Documentation

- Goose migration framework: `github.com/pressly/goose/v3`
- Autotester framework: `shared/go/api/autotester`
- Database utilities: `shared/go/api/databaseutil`
