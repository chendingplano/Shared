// Package tester_migration provides automated testing for the goose database migration system.
// It tests migration apply/rollback cycles, version tracking, and edge cases.
//
// Documents:
// - shared/Documents/code/testbots/tester-migration/tester-migration-overview.md
// - shared/Documents/code/testbots/tester-migration/tester-migration.md

package tester_migration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	autotesters "github.com/chendingplano/shared/go/api/autotester"
	"github.com/chendingplano/shared/go/api/databaseutil"
	sharedgoose "github.com/chendingplano/shared/go/api/goose"
)

// MigrationTester tests the Goose migration system.
type MigrationTester struct {
	autotesters.BaseTester

	// Configuration
	cfg *MigrationTesterConfig

	// Runtime state
	dutDB *sql.DB

	// Migration directories (created for testing)
	testMigrationsDir string

	// State tracking
	state MigrationSUTState
}

// NewMigrationTester creates a new MigrationTester instance.
func NewMigrationTester(cfg *MigrationTesterConfig) *MigrationTester {
	cfg.ApplyDefaults()

	return &MigrationTester{
		BaseTester: autotesters.NewBaseTester(
			"tester_migration",
			"Tests the goose database migration system (Up, Down, CreateAndApply, version tracking)",
			"regression",
			"integration",
			[]string{"database", "migration", "goose", "shared"},
		),
		cfg: cfg,
	}
}

// Prepare sets up the test environment for migration testing.
func (t *MigrationTester) Prepare(ctx context.Context) error {
	// 1. Verify DUT is reachable
	if err := t.cfg.DUTDB.PingContext(ctx); err != nil {
		return fmt.Errorf("DUT not reachable (MID_260224100001): %w", err)
	}

	// 2. Validate DUT name starts with "testonly_"
	if t.cfg.DUTDBName != "" {
		if !strings.HasPrefix(t.cfg.DUTDBName, "testonly_") {
			return fmt.Errorf("DUT name must start with 'testonly_' (MID_260224100002), got: %s", t.cfg.DUTDBName)
		}
	}

	// 3. Validate migrations directory starts with "testonly_"
	if !strings.HasPrefix(t.cfg.MigrationsDir, "testonly_") {
		return fmt.Errorf("migrations dir must start with 'testonly_' (MID_260224100003), got: %s", t.cfg.MigrationsDir)
	}

	t.dutDB = t.cfg.DUTDB
	t.testMigrationsDir = t.cfg.MigrationsDir

	// 4. Create migrations directory if it doesn't exist
	if err := os.MkdirAll(t.testMigrationsDir, 0755); err != nil {
		return fmt.Errorf("create migrations dir (MID_260224100004): %w", err)
	}

	// 5. Drop goose tracking table from DUT
	_, err := t.cfg.DUTDB.ExecContext(ctx, "DROP TABLE IF EXISTS "+t.cfg.TableName)
	if err != nil {
		return fmt.Errorf("drop tracking table (MID_260224100005): %w", err)
	}

	// 6. Drop all testonly_ tables from DUT
	if err := t.dropTestTables(ctx); err != nil {
		return fmt.Errorf("drop test tables (MID_260224100006): %w", err)
	}

	// 7. Clear the testonly_ directory
	if err := t.clearMigrationsDir(ctx); err != nil {
		return fmt.Errorf("clear migrations dir (MID_260224100007): %w", err)
	}

	// 8. Build the migrations pool
	if err := t.buildMigrationsPool(ctx); err != nil {
		return fmt.Errorf("build migrations pool (MID_260224100008): %w", err)
	}

	// 9. Initialize state
	if err := t.syncState(ctx); err != nil {
		return fmt.Errorf("sync state (MID_260224100009): %w", err)
	}

	return nil
}

// Cleanup tears down the test environment.
func (t *MigrationTester) Cleanup(ctx context.Context) error {
	// 1. Drop all testonly_ tables from DUT
	if err := t.dropTestTables(ctx); err != nil {
		return fmt.Errorf("drop test tables (MID_260224100010): %w", err)
	}

	// 2. Drop db_migrations from DUT
	_, err := t.cfg.DUTDB.ExecContext(ctx, "DROP TABLE IF EXISTS "+t.cfg.TableName)
	if err != nil {
		return fmt.Errorf("drop tracking table (MID_260224100011): %w", err)
	}

	// 3. Delete all .sql files from the testonly_ directory
	return t.clearMigrationsDir(ctx)
}

// dropTestTables drops all tables with names starting with "testonly_".
func (t *MigrationTester) dropTestTables(ctx context.Context) error {
	// Query for all testonly_ tables
	query := `
		SELECT table_name 
		FROM information_schema.tables 
		WHERE table_schema = 'public' 
		  AND table_name LIKE 'testonly_%'
	`
	rows, err := t.cfg.DUTDB.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("query test tables (MID_260224100012): %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return fmt.Errorf("scan table name (MID_260224100013): %w", err)
		}
		tables = append(tables, tableName)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate tables (MID_260224100014): %w", err)
	}

	// Drop each table
	for _, table := range tables {
		if !databaseutil.IsValidTableName(table) {
			continue // Skip invalid table names for safety
		}
		dropQuery := fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", table)
		if _, err := t.cfg.DUTDB.ExecContext(ctx, dropQuery); err != nil {
			return fmt.Errorf("drop table %s (MID_260224100015): %w", table, err)
		}
	}

	return nil
}

// clearMigrationsDir deletes all .sql files from the migrations directory.
func (t *MigrationTester) clearMigrationsDir(_ context.Context) error {
	entries, err := os.ReadDir(t.testMigrationsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Directory doesn't exist, nothing to clear
		}
		return fmt.Errorf("read migrations dir (MID_260224100016): %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			fullPath := filepath.Join(t.testMigrationsDir, entry.Name())
			if err := os.Remove(fullPath); err != nil {
				return fmt.Errorf("remove migration file %s (MID_260224100017): %w", entry.Name(), err)
			}
		}
	}

	return nil
}

// buildMigrationsPool creates synthetic migration files for testing.
func (t *MigrationTester) buildMigrationsPool(_ context.Context) error {
	for i := 1; i <= t.cfg.MaxMigrationsInPool; i++ {
		version := time.Now().UTC().Format("20060102150405")
		if i < 10 {
			version = fmt.Sprintf("%s%02d", version[:12], i)
		} else {
			version = fmt.Sprintf("%s%d", version[:12], i)
		}

		tableName := fmt.Sprintf("testonly_table_%02d", i)
		upSQL := fmt.Sprintf(`CREATE TABLE %s (
			id BIGSERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`, tableName)

		downSQL := fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName)

		filename := fmt.Sprintf("%s_create_table_%02d.sql", version, i)
		content := buildMigrationFileContent(upSQL, downSQL)

		fullPath := filepath.Join(t.testMigrationsDir, filename)
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("write migration file %s (MID_260224100018): %w", filename, err)
		}
	}

	return nil
}

// buildMigrationFileContent produces the annotated SQL content for a migration file.
func buildMigrationFileContent(upSQL, downSQL string) string {
	var b strings.Builder
	b.WriteString("-- +goose Up\n")
	b.WriteString("-- +goose StatementBegin\n")
	b.WriteString(strings.TrimSpace(upSQL))
	b.WriteString("\n-- +goose StatementEnd\n")
	if strings.TrimSpace(downSQL) != "" {
		b.WriteString("\n-- +goose Down\n")
		b.WriteString("-- +goose StatementBegin\n")
		b.WriteString(strings.TrimSpace(downSQL))
		b.WriteString("\n-- +goose StatementEnd\n")
	}
	return b.String()
}

// buildMigrator creates a new migrator with the given configuration.
func (t *MigrationTester) buildMigrator(allowOutOfOrder bool) *sharedgoose.Migrator {
	migrateCfg := ApiTypes.MigrationConfig{
		MigrationsFS:  "",
		MigrationsDir: t.testMigrationsDir,
		TableName:     t.cfg.TableName,
		Verbose:       "false",
		AllowOutOfOrder: func() string {
			if allowOutOfOrder {
				return "true"
			}
			return "false"
		}(),
	}

	logger := &nopLogger{}
	migrator, err := sharedgoose.NewWithDB(t.dutDB, t.cfg.DUTDBType, migrateCfg, logger)
	if err != nil {
		return nil
	}
	return migrator
}

// nopLogger is a no-op logger for migration operations.
type nopLogger struct{}

func (l *nopLogger) Debug(message string, args ...any) {}
func (l *nopLogger) Line(message string, args ...any)  {}
func (l *nopLogger) Info(message string, args ...any)  {}
func (l *nopLogger) Warn(message string, args ...any)  {}
func (l *nopLogger) Error(message string, args ...any) {}
func (l *nopLogger) Trace(message string)              {}
func (l *nopLogger) Close()                            {}

// RunTestCase executes a single test case.
func (t *MigrationTester) RunTestCase(ctx context.Context, tc autotesters.TestCase) autotesters.TestResult {
	result := autotesters.TestResult{
		TestCaseID: tc.ID,
		StartTime:  time.Now(),
	}

	// Guard against panics
	defer func() {
		if r := recover(); r != nil {
			result.Status = autotesters.StatusError
			result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("panic: %v", r))
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(result.StartTime)
		}
	}()

	input, ok := tc.Input.(migrationInput)
	if !ok {
		result.Status = autotesters.StatusError
		result.ErrorMsgs = append(result.ErrorMsgs, "invalid input type: expected migrationInput (MID_260224100019)")
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		return result
	}

	// 1. Reset DUT to the pre-state expected by this case
	if err := t.resetToState(ctx, input.PreState); err != nil {
		result.Status = autotesters.StatusError
		result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("resetToState failed (MID_260224100020): %v", err))
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		return result
	}

	// 2. Rebuild migrator with case-specific AllowOutOfOrder setting
	migrator := t.buildMigrator(input.AllowOutOfOrder)
	if migrator == nil {
		result.Status = autotesters.StatusError
		result.ErrorMsgs = append(result.ErrorMsgs, "failed to build migrator (MID_260224100021)")
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		return result
	}

	// 3. Dispatch to per-operation handler
	t.dispatch(ctx, input, migrator, &result)

	// 4. Observe side effects
	t.observeSideEffects(ctx, &result)

	// 5. Sync internal state from DUT ground truth
	if err := t.syncState(ctx); err != nil {
		result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("syncState warning (MID_260224100022): %v", err))
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	return result
}

// dispatch routes the operation to the appropriate handler.
func (t *MigrationTester) dispatch(ctx context.Context, input migrationInput, migrator *sharedgoose.Migrator, result *autotesters.TestResult) {
	switch input.Operation {
	case OpUp:
		t.handleUp(ctx, migrator, result)
	case OpUpByOne:
		t.handleUpByOne(ctx, migrator, result)
	case OpUpTo:
		t.handleUpTo(ctx, migrator, input.TargetVersion, result)
	case OpDown:
		t.handleDown(ctx, migrator, result)
	case OpDownTo:
		t.handleDownTo(ctx, migrator, input.TargetVersion, result)
	case OpStatus:
		t.handleStatus(ctx, migrator, result)
	case OpGetVersion:
		t.handleGetVersion(ctx, migrator, result)
	case OpHasPending:
		t.handleHasPending(ctx, migrator, result)
	case OpCreateAndApply:
		t.handleCreateAndApply(ctx, migrator, input.Description, input.UpSQL, input.DownSQL, result)
	case OpListSources:
		t.handleListSources(migrator, result)
	default:
		result.Status = autotesters.StatusError
		result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("unknown operation: %s (MID_260224100023)", input.Operation))
	}
}

// observeSideEffects inspects the DUT to determine what side effects occurred.
func (t *MigrationTester) observeSideEffects(ctx context.Context, result *autotesters.TestResult) {
	// Check if tracking table exists
	query := `
		SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = 'public' AND table_name = $1
		)
	`
	var trackingTableExists bool
	if err := t.cfg.DUTDB.QueryRowContext(ctx, query, t.cfg.TableName).Scan(&trackingTableExists); err == nil {
		if trackingTableExists {
			result.SideEffectsObserved = append(result.SideEffectsObserved, string(SideEffectTrackingTableCreated))
		}
	}

	// Check for testonly_ tables
	tablesQuery := `
		SELECT table_name FROM information_schema.tables 
		WHERE table_schema = 'public' AND table_name LIKE 'testonly_%'
	`
	rows, err := t.cfg.DUTDB.QueryContext(ctx, tablesQuery)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var tableName string
			if err := rows.Scan(&tableName); err == nil {
				result.SideEffectsObserved = append(result.SideEffectsObserved, string(SideEffectSchemaTableApplied))
				break
			}
		}
	}
}
