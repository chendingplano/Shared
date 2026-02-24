// Package tester_migration provides automated testing for the goose database migration system.
// It tests migration apply/rollback cycles, version tracking, and edge cases.
//
// Created: 2026/02/24 based on tester-migration-qwen-v4.md

package tester_migration

import (
	"database/sql"

	gooselib "github.com/pressly/goose/v3"
)

// MigrationOperation represents the type of migration operation to test.
type MigrationOperation string

const (
	OpUp            MigrationOperation = "Up"
	OpUpByOne       MigrationOperation = "UpByOne"
	OpUpTo          MigrationOperation = "UpTo"
	OpDown          MigrationOperation = "Down"
	OpDownTo        MigrationOperation = "DownTo"
	OpStatus        MigrationOperation = "Status"
	OpGetVersion    MigrationOperation = "GetVersion"
	OpHasPending    MigrationOperation = "HasPending"
	OpCreateAndApply MigrationOperation = "CreateAndApply"
	OpListSources   MigrationOperation = "ListSources"
)

// MigrationTesterConfig holds the configuration for the MigrationTester.
type MigrationTesterConfig struct {
	// DUT: Isolated test database for migration testing; NEVER production.
	// Database name MUST start with "testonly_" for safety.
	DUTDB     *sql.DB
	DUTDBType string // "postgres" or "mysql"; default: "postgres"
	DUTDBName string // for logging only; MUST start with "testonly_"

	// Directory for synthetic migration files; MUST start with "testonly_".
	MigrationsDir string // default: "testonly_migrations"

	// Goose version-tracking table name.
	TableName string // default: "db_migrations"

	// Number of dynamic test cases to generate per run.
	NumDynamicCases int // default: 80

	// Size of the pre-generated migrations pool created during Prepare.
	MaxMigrationsInPool int // default: 20
}

// ApplyDefaults applies default values to the configuration.
func (cfg *MigrationTesterConfig) ApplyDefaults() {
	if cfg.DUTDBType == "" {
		cfg.DUTDBType = "postgres"
	}
	if cfg.MigrationsDir == "" {
		cfg.MigrationsDir = "testonly_migrations"
	}
	if cfg.TableName == "" {
		cfg.TableName = "db_migrations"
	}
	if cfg.NumDynamicCases == 0 {
		cfg.NumDynamicCases = 80
	}
	if cfg.MaxMigrationsInPool == 0 {
		cfg.MaxMigrationsInPool = 20
	}
}

// Validate checks that the configuration is valid.
func (cfg *MigrationTesterConfig) Validate() error {
	if cfg.DUTDB == nil {
		return sql.ErrNoRows // placeholder error
	}
	if cfg.DUTDBName != "" && len(cfg.DUTDBName) >= 9 && cfg.DUTDBName[:9] != "testonly_" {
		return sql.ErrNoRows // placeholder error
	}
	if len(cfg.MigrationsDir) >= 9 && cfg.MigrationsDir[:9] != "testonly_" {
		return sql.ErrNoRows // placeholder error
	}
	return nil
}

// MigrationFile represents a migration file in the migrations directory.
type MigrationFile struct {
	Version   int64
	Filename  string
	UpSQL     string
	DownSQL   string
	IsApplied bool
}

// MigrationRecord represents an applied migration.
type MigrationRecord struct {
	Version  int64
	Filename string
	UpSQL    string
	DownSQL  string
	Applied  bool
}

// MigrationSUTState represents the current state of the migration system under test.
type MigrationSUTState struct {
	// Applied is the list of migrations that have been applied to the DUT.
	Applied []MigrationRecord

	// FilesInDir is the list of all .sql files in the migrations directory.
	FilesInDir []MigrationFile

	// Tables is a map of table names that exist in the DUT (testonly_ tables only).
	Tables map[string]bool

	// CurrentVersion is the highest applied version; 0 if nothing applied.
	CurrentVersion int64
}

// migrationInput is the typed value stored in TestCase.Input.
type migrationInput struct {
	// Operation is the migration operation to invoke.
	Operation MigrationOperation

	// TargetVersion is used by UpTo / DownTo operations.
	TargetVersion int64

	// UpSQL is used by CreateAndApply.
	UpSQL string

	// DownSQL is used by CreateAndApply; "" = no down SQL.
	DownSQL string

	// Description is used by CreateAndApply.
	Description string

	// AllowOutOfOrder is the migrator Config.AllowOutOfOrder for this case.
	AllowOutOfOrder bool

	// PreState is the DUT state this case requires before execution.
	// RunTestCase calls resetToState(ctx, PreState) before invoking the SUT.
	PreState MigrationSUTState
}

// sideEffectKey represents an observed side effect during test execution.
type sideEffectKey string

const (
	SideEffectTrackingTableCreated sideEffectKey = "tracking_table_created"
	SideEffectSchemaTableApplied   sideEffectKey = "schema_table_applied"
	SideEffectSchemaTableDropped   sideEffectKey = "schema_table_dropped"
	SideEffectMigrationFileWritten sideEffectKey = "migration_file_written"
)

// MigrationStatus wraps gooselib.MigrationStatus for easier handling.
type MigrationStatus struct {
	Version  int64
	IsApplied bool
	FileName string
	FilePath string
	Type     string
}

// ToMigrationStatus converts a gooselib.MigrationStatus to MigrationStatus.
func ToMigrationStatus(gs *gooselib.MigrationStatus) MigrationStatus {
	return MigrationStatus{
		Version:   gs.Source.Version,
		IsApplied: gs.State == gooselib.StateApplied,
		FileName:  extractFilename(gs.Source.Path),
		FilePath:  gs.Source.Path,
		Type:      string(gs.Source.Type),
	}
}

// extractFilename extracts the filename from a full path.
func extractFilename(path string) string {
	// Simple extraction - find last slash
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[i+1:]
		}
	}
	return path
}
