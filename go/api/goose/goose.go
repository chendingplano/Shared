// Package goose provides database schema migration utilities built on top of
// pressly/goose. It integrates with the shared library's global database
// connections and JimoLogger so callers only need to supply a migration
// filesystem and a handful of options.
//
// Typical usage (apply existing migrations on startup):
//
//	//go:embed migrations
//	var embedMigrations embed.FS
//
//	migrationsFS, _ := fs.Sub(embedMigrations, "migrations")
//	migrator, err := goose.New(goose.Config{
//	    MigrationsFS: migrationsFS,
//	})
//	if err != nil { ... }
//	if err := migrator.Up(ctx); err != nil { ... }
//
// To create a new migration file and immediately apply it (e.g. when altering
// a table at runtime or during a management command):
//
//	filename, err := migrator.CreateAndApply(ctx,
//	    "add_tags_column",
//	    "ALTER TABLE documents ADD COLUMN IF NOT EXISTS tags TEXT[] DEFAULT '{}';",
//	    "ALTER TABLE documents DROP COLUMN IF EXISTS tags;",
//	)
//
// For more information, refer to shared/Documents/dev/goose.md
//
// Created: 2026/02/20 by Claude Code and Chen Ding

package goose

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	gooselib "github.com/pressly/goose/v3"
	goosedb "github.com/pressly/goose/v3/database"
)

// Migrator wraps a goose.Provider and integrates with the shared library's
// logging and global database connections.
type Migrator struct {
	provider *gooselib.Provider
	logger   ApiTypes.JimoLogger
	db       *sql.DB
	dialect  gooselib.Dialect
	cfg      ApiTypes.MigrationConfig
}

var ProjectMigrator *Migrator
var SharedMigrator *Migrator
var AutoTesterMigrator *Migrator

// dialectFor maps the shared library's DB type constants to goose Dialect values.
func dialectFor(dbType string) (gooselib.Dialect, error) {
	switch dbType {
	case ApiTypes.PgName:
		return gooselib.DialectPostgres, nil
	case ApiTypes.MysqlName:
		return gooselib.DialectMySQL, nil
	default:
		return "", fmt.Errorf("unsupported database type (MID_060221143033): %s", dbType)
	}
}

type gooseTableExistsChecker interface {
	TableExists(ctx context.Context, db goosedb.DBTxConn) (bool, error)
}

func ensureGooseVersionTable(
	ctx context.Context,
	db *sql.DB,
	dialect gooselib.Dialect,
	tableName string,
) error {
	if db == nil {
		return fmt.Errorf("database connection is nil (MID_26033103)")
	}

	store, err := goosedb.NewStore(goosedb.Dialect(dialect), tableName)
	if err != nil {
		return fmt.Errorf("failed to create goose store (MID_26033104): %w", err)
	}

	checker, ok := store.(gooseTableExistsChecker)
	if !ok {
		return fmt.Errorf("goose store does not support table existence check (MID_26033105)")
	}

	exists, err := checker.TableExists(ctx, db)
	if err != nil {
		return fmt.Errorf("failed to check goose version table existence (MID_26033106): %w", err)
	}

	if !exists {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed to begin goose table init tx (MID_26033107): %w", err)
		}
		defer tx.Rollback()

		if err := store.CreateVersionTable(ctx, tx); err != nil {
			return fmt.Errorf("failed to create goose version table (MID_26033108): %w", err)
		}
		if err := store.Insert(ctx, tx, goosedb.InsertRequest{Version: 0}); err != nil {
			return fmt.Errorf("failed to seed goose version table (MID_26033109): %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit goose table init tx (MID_26033110): %w", err)
		}
		return nil
	}

	// The table may exist from legacy/manual setup but be missing version 0.
	if _, err := store.GetMigration(ctx, db, 0); err != nil {
		if !errors.Is(err, goosedb.ErrVersionNotFound) {
			return fmt.Errorf("failed to validate goose version table seed row (MID_26033111): %w", err)
		}
		if err := store.Insert(ctx, db, goosedb.InsertRequest{Version: 0}); err != nil {
			return fmt.Errorf("failed to insert goose version seed row (MID_26033112): %w", err)
		}
	}

	return nil
}

func applyDefaults(migrate_cfg *ApiTypes.MigrationConfig) {
	// Verbose defaults to true
	migrate_cfg.Verbose = migrate_cfg.VerboseStr != "false"

	// AllowOutOfOrder defaults to true
	migrate_cfg.AllowOutOfOrder = migrate_cfg.AllowOutOfOrderStr != "false"
}

func RunProjectMigrations(ctx context.Context,
	logger ApiTypes.JimoLogger,
	db *sql.DB) error {
	if ProjectMigrator != nil {
		return fmt.Errorf("project migrator already initialized (MID_060221143034)")
	}

	if db == nil {
		return fmt.Errorf("database connection is not initialized (MID_060221143035)")
	}

	var cfg ApiTypes.MigrationConfig

	// Prefer MigrationsDir already set in CommonConfig (e.g. normalized to an
	// absolute path by the caller via normalizeMigrationPaths). Fall back to the
	// PROJECT_MIGRATION_DIR env var so existing deployments are unaffected.
	if strings.TrimSpace(ApiTypes.CommonConfig.MigrationConfig.MigrationsDir) != "" {
		cfg.MigrationsDir = ApiTypes.CommonConfig.MigrationConfig.MigrationsDir
		if ApiTypes.CommonConfig.MigrationConfig.MigrationsFS != nil {
			cfg.MigrationsFS = ApiTypes.CommonConfig.MigrationConfig.MigrationsFS
		} else {
			cfg.MigrationsFS = os.DirFS(cfg.MigrationsDir)
		}
	} else {
		cfg.MigrationsDir = os.Getenv("PROJECT_MIGRATION_DIR")
		if strings.TrimSpace(cfg.MigrationsDir) == "" {
			logger.Warn("missing env var PROJECT_MIGRATION_DIR. Default to: project_migrations")
			cfg.MigrationsDir = "project_migrations"
		}
		cfg.MigrationsFS = os.DirFS(cfg.MigrationsDir)
	}

	// Prefer TableName already set in CommonConfig. Fall back to env var.
	if strings.TrimSpace(ApiTypes.CommonConfig.MigrationConfig.TableName) != "" {
		cfg.TableName = ApiTypes.CommonConfig.MigrationConfig.TableName
	} else {
		cfg.TableName = os.Getenv("PG_MIGRATION_TNAME_PROJECT")
		if cfg.TableName == "" {
			logger.Warn("missing env var PG_MIGRATION_TNAME_PROJECT, default to 'project_db_migration'")
			cfg.TableName = "project_db_migration"
		}
	}

	logger.Info("==== tablename", "tablename", cfg.TableName)

	var err error
	ProjectMigrator, err = RunMigrations(ctx, "project_migration", logger, cfg, db)
	if err != nil {
		return fmt.Errorf("failed to run project migrations (MID_060221143003): %w", err)
	}
	return nil
}

func RunSharedMigrations(ctx context.Context,
	logger ApiTypes.JimoLogger,
	db *sql.DB) error {
	if SharedMigrator != nil {
		return fmt.Errorf("shared migrator already initialized (MID_060221143001)")
	}

	if db == nil {
		return fmt.Errorf("database connection is not initialized (MID_060221143002)")
	}

	var cfg ApiTypes.MigrationConfig

	// Prefer SharedMigrationsDir already set in CommonConfig. Fall back to the
	// SHARED_MIGRATION_DIR env var so existing deployments are unaffected.
	if strings.TrimSpace(ApiTypes.CommonConfig.MigrationConfig.SharedMigrationsDir) != "" {
		cfg.MigrationsDir = ApiTypes.CommonConfig.MigrationConfig.SharedMigrationsDir
		cfg.MigrationsFS = os.DirFS(cfg.MigrationsDir)
	} else {
		cfg.MigrationsDir = os.Getenv("SHARED_MIGRATION_DIR")
		if strings.TrimSpace(cfg.MigrationsDir) == "" {
			logger.Warn("missing env var SHARED_MIGRATION_DIR. Default to: shared_migrations")
			cfg.MigrationsDir = "shared_migrations"
		}
		cfg.MigrationsFS = os.DirFS(cfg.MigrationsDir)
	}

	// Prefer SharedTableName already set in CommonConfig. Fall back to env var.
	if strings.TrimSpace(ApiTypes.CommonConfig.MigrationConfig.SharedTableName) != "" {
		cfg.TableName = ApiTypes.CommonConfig.MigrationConfig.SharedTableName
	} else {
		cfg.TableName = os.Getenv("PG_MIGRATION_TNAME_SHARED")
		if cfg.TableName == "" {
			logger.Warn("missing PG_MIGRATION_TNAME_SHARED, default to 'shared_db_migration'")
			cfg.TableName = "shared_db_migration"
		}
	}

	var err error
	SharedMigrator, err = RunMigrations(ctx, "shared_migration", logger, cfg, db)
	if err != nil {
		return fmt.Errorf("failed to run shared migrations (MID_060221143004): %w", err)
	}
	return nil
}

func RunAutoTesterMigrations(ctx context.Context,
	logger ApiTypes.JimoLogger,
	db *sql.DB) error {
	if AutoTesterMigrator != nil {
		return fmt.Errorf("autotester migrator already initialized (MID_060221143011)")
	}

	if db == nil {
		return fmt.Errorf("autotester database connection is not initialized (MID_060221143012)")
	}

	var cfg ApiTypes.MigrationConfig

	cfg.MigrationsDir = os.Getenv("AUTOTESTER_MIGRATION_Dir")
	if strings.TrimSpace(cfg.MigrationsDir) == "" {
		logger.Warn("missing env var AUTOTESTER_MIGRATION_DIR. Default to: autotester_migrations")
		cfg.MigrationsDir = "autotester_migrations"
	}
	cfg.MigrationsFS = os.DirFS(cfg.MigrationsDir)

	cfg.TableName = os.Getenv("PG_MIGRATION_TNAME_AUTOTESTER")
	if cfg.TableName == "" {
		logger.Warn("missing PG_MIGRATION_TNAME_AUTOTESTER, default to 'autotester_db_migrations'")
		cfg.TableName = "autotester_db_migrations"
	}

	var err error
	AutoTesterMigrator, err = RunMigrations(ctx, "autotester", logger, cfg, db)
	if err != nil {
		return fmt.Errorf("failed to run autotester migrations (MID_060221143013): %w", err)
	}
	return nil
}

func RunMigrations(
	ctx context.Context,
	migrationName string,
	logger ApiTypes.JimoLogger,
	cfg ApiTypes.MigrationConfig,
	db *sql.DB) (*Migrator, error) {
	// MigrationsDir is the path on disk for creating new migration files.
	// MigrationsFS is the fs.FS used by goose to read migration files.
	// For os.MkdirAll, we use MigrationsDir directly.
	if cfg.MigrationsDir == "" {
		return nil, fmt.Errorf("(MID_26033101) missing migrationPath")
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(cfg.MigrationsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create migrations directory (MID_260221143005): %w, path:%s",
			err, cfg.MigrationsDir)
	}

	logger.Info("==== tablename", "tablename", cfg.TableName)
	migrator, err := NewWithDB(db, ApiTypes.DBType, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create migrator (MID_060221143006): %w", err)
	}
	if err := migrator.Up(ctx); err != nil {
		return nil, fmt.Errorf("failed to apply migrations (MID_060221143007): %w", err)
	}
	logger.Info("Migrations applied successfully", "migrator", migrationName)
	return migrator, nil
}

// NewWithDB creates a Migrator with an explicit *sql.DB and dbType string.
// dbType must be one of ApiTypes.PgName ("postgres") or ApiTypes.MysqlName ("mysql").
//
// Use this variant when you need to run migrations against a database that is
// separate from the shared library's global connection pool.
//
// 'db' is the migration DB
// 'TableName' is the name of the table for tracking migration
func NewWithDB(db *sql.DB,
	dbType string,
	cfg ApiTypes.MigrationConfig,
	logger ApiTypes.JimoLogger) (*Migrator, error) {

	applyDefaults(&cfg)

	logger.Info("==== tablename", "tablename", cfg.TableName)

	if cfg.TableName == "" {
		return nil, fmt.Errorf("missing tablename in migration config (SHD_20260221092501)")
	}

	dialect, err := dialectFor(dbType)
	if err != nil {
		logger.Error("Invalid database type for goose migrator (SHD_GSE_083)", "db_type", dbType, "error", err)
		return nil, err
	}

	if err := ensureGooseVersionTable(context.Background(), db, dialect, cfg.TableName); err != nil {
		logger.Error("Failed to ensure goose version table", "table_name", cfg.TableName, "error", err)
		return nil, err
	}

	opts := buildProviderOptions(cfg)
	logger.Info("Initializing goose migrator",
		"db_type", dbType,
		"table_name", cfg.TableName,
		"verbose", cfg.Verbose,
		"allow_out_of_order", cfg.AllowOutOfOrder,
	)

	// Check if there are any migration files in the filesystem before creating
	// the provider. This prevents ErrNoMigrations when the migrations directory
	// is empty (valid for new projects or before any migrations are created).
	hasMigrations, err := hasMigrationFiles(cfg.MigrationsFS)
	if err != nil {
		logger.Warn("Failed to check for migration files, proceeding anyway",
			"error", err)
	} else if !hasMigrations {
		logger.Info("No migration files found - skipping migration initialization",
			"db_type", dbType,
			"table_name", cfg.TableName,
			"migrations_dir", cfg.MigrationsDir)
		// Return a migrator with nil provider - Up/Down operations will be no-ops
		return &Migrator{
			logger:  logger,
			db:      db,
			dialect: dialect,
			cfg:     cfg,
		}, nil
	}

	provider, err := gooselib.NewProvider(dialect, db, cfg.MigrationsFS, opts...)
	if err != nil {
		logger.Error("Failed to create goose provider (SHD_GSE_090)",
			"error", err,
			"config", cfg)
		return nil, fmt.Errorf("failed to create goose provider (SHD_GSE_091): %w", err)
	}

	logger.Info("Goose migrator initialised", "db_type", dbType, "table", cfg.TableName)

	return &Migrator{
		provider: provider,
		logger:   logger,
		db:       db,
		dialect:  dialect,
		cfg:      cfg,
	}, nil
}

// rebuildProvider recreates the internal goose.Provider so that newly written
// migration files on disk are visible. Called automatically by CreateAndApply.
func (m *Migrator) rebuildProvider() error {
	// Check if there are migration files after the new file was created
	hasMigrations, err := hasMigrationFiles(m.cfg.MigrationsFS)
	if err != nil {
		m.logger.Warn("Failed to check for migration files during rebuild", "error", err)
	}

	// If no migrations exist, keep provider as nil
	if !hasMigrations {
		m.provider = nil
		return nil
	}

	opts := buildProviderOptions(m.cfg)
	provider, err := gooselib.NewProvider(m.dialect, m.db, m.cfg.MigrationsFS, opts...)
	if err != nil {
		return fmt.Errorf("failed to rebuild goose provider (MID_060221143010): %w", err)
	}
	m.provider = provider
	return nil
}

func buildProviderOptions(goose_cfg ApiTypes.MigrationConfig) []gooselib.ProviderOption {
	var opts []gooselib.ProviderOption
	if goose_cfg.TableName != "" {
		opts = append(opts, gooselib.WithTableName(goose_cfg.TableName))
	}
	if goose_cfg.Verbose {
		opts = append(opts, gooselib.WithVerbose(true))
	}
	if goose_cfg.AllowOutOfOrder {
		opts = append(opts, gooselib.WithAllowOutofOrder(true))
	}
	return opts
}

// hasMigrationFiles checks if the given filesystem contains any .sql or .go
// migration files. Returns true if at least one migration file is found.
func hasMigrationFiles(fsys fs.FS) (bool, error) {
	if fsys == nil {
		return false, nil
	}

	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return false, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".sql") || strings.HasSuffix(name, ".go") {
			return true, nil
		}
	}
	return false, nil
}

// nonAlphanumRE matches anything that should become an underscore in a slug.
var nonAlphanumRE = regexp.MustCompile(`[^a-z0-9]+`)

// slugify converts a human description to a safe filename component:
// lowercase, all non-alphanumeric runs replaced with a single underscore,
// leading/trailing underscores trimmed, capped at 60 characters.
func slugify(description string) string {
	s := strings.ToLower(strings.TrimSpace(description))
	s = nonAlphanumRE.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	if len(s) > 60 {
		s = s[:60]
	}
	if s == "" {
		s = "migration"
	}
	return s
}

// buildMigrationSQL produces the annotated SQL content for a migration file.
// downSQL may be empty, in which case the -- +goose Down section is omitted.
func buildMigrationSQL(upSQL, downSQL string) string {
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

// CreateMigration writes a new timestamped SQL migration file to MigrationsDir.
// It returns the generated filename (e.g. "20260220143000_add_tags_column.sql").
//
// The file is NOT applied to the database; call Up or UpByOne afterwards,
// or use CreateAndApply to do both in one step.
//
// downSQL may be empty if rolling back the migration is not meaningful.
// Config.MigrationsDir must be set.
func (m *Migrator) CreateMigration(description, upSQL, downSQL string) (string, error) {
	if m.cfg.MigrationsDir == "" {
		return "", fmt.Errorf("Config.MigrationsDir must be set to create migration files (MID_060221143011)")
	}
	if strings.TrimSpace(upSQL) == "" {
		return "", fmt.Errorf("upSQL must not be empty (MID_060221143012)")
	}

	version := time.Now().UTC().Format("20060102150405")
	slug := slugify(description)
	filename := fmt.Sprintf("%s_%s.sql", version, slug)
	fullPath := filepath.Join(m.cfg.MigrationsDir, filename)

	content := buildMigrationSQL(upSQL, downSQL)
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		m.logger.Error("Failed to write migration file (MID_060221143013)", "path", fullPath, "error", err)
		return "", fmt.Errorf("failed to write migration file (MID_060221143013): %w", err)
	}

	m.logger.Info("Migration file created (MID_060221143014)", "file", filename)
	return filename, nil
}

// CreateAndApply writes a new migration file to MigrationsDir, then immediately
// applies it to the database. It is the single-call equivalent of:
//
//	filename, err := migrator.CreateMigration(description, upSQL, downSQL)
//	err = migrator.UpByOne(ctx)
//
// Returns the generated filename so the caller can commit it to version control.
// Config.MigrationsDir must be set and MigrationsFS must be backed by the same
// on-disk directory (i.e. os.DirFS, not embed.FS).
//
// Not safe for concurrent use — callers that create migrations concurrently
// must synchronise externally.
func (m *Migrator) CreateAndApply(ctx context.Context, description, upSQL, downSQL string) (string, error) {
	filename, err := m.CreateMigration(description, upSQL, downSQL)
	if err != nil {
		return "", err
	}

	// The existing Provider was built before the new file existed; recreate it
	// so goose can discover and track the new migration.
	if err := m.rebuildProvider(); err != nil {
		return filename, fmt.Errorf("failed to reload migrations after creating %s (MID_060221143010): %w", filename, err)
	}

	if err := m.UpByOne(ctx); err != nil {
		return filename, fmt.Errorf("failed to apply migration %s (MID_060221143011): %w", filename, err)
	}

	return filename, nil
}

// Up applies all pending migrations in ascending version order.
// Returns nil when there are no pending migrations or no provider is initialized.
func (m *Migrator) Up(ctx context.Context) error {
	// No provider means no migrations directory or empty - nothing to apply
	if m.provider == nil {
		m.logger.Info("No migrations to apply - provider not initialized")
		return nil
	}

	m.logger.Info("Applying all pending migrations (MID_060221143012)")

	results, err := m.provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("migration up failed (MID_060221143015): %w", err)
	}

	for _, r := range results {
		m.logger.Info("Migration applied (MID_060221143016)",
			"version", r.Source.Version,
			"type", r.Source.Type,
			"duration_ms", r.Duration.Milliseconds(),
		)
	}
	m.logger.Info("All migrations applied (SHD_GSE_231)", "count", len(results))
	return nil
}

// UpByOne applies the single next pending migration.
// Returns nil when there is no provider initialized or no pending migrations.
// Returns gooselib.ErrNoNextVersion if there are no pending migrations.
func (m *Migrator) UpByOne(ctx context.Context) error {
	// No provider means no migrations - nothing to apply
	if m.provider == nil {
		m.logger.Info("No migrations to apply - provider not initialized")
		return nil
	}

	m.logger.Info("Applying next pending migration (SHD_GSE_237)")

	result, err := m.provider.UpByOne(ctx)
	if err != nil {
		return fmt.Errorf("migration up-by-one failed (MID_060221143017): %w", err)
	}

	m.logger.Info("Migration applied (MID_060221143018)",
		"version", result.Source.Version,
		"duration_ms", result.Duration.Milliseconds(),
	)
	return nil
}

// UpTo applies all pending migrations up to and including the specified version.
// Returns nil when there is no provider or no pending migrations.
func (m *Migrator) UpTo(ctx context.Context, version int64) error {
	// No provider means no migrations - nothing to apply
	if m.provider == nil {
		m.logger.Info("No migrations to apply - provider not initialized")
		return nil
	}

	m.logger.Info("Applying migrations up to version (MID_060221143019)", "version", version)

	results, err := m.provider.UpTo(ctx, version)
	if err != nil {
		m.logger.Error("Failed to apply migrations (MID_060221143020)", "error", err, "version", version)
		return fmt.Errorf("migration up-to %d failed (MID_060221143021): %w", version, err)
	}

	m.logger.Info("Migrations applied (MID_060221143022)", "count", len(results), "target_version", version)
	return nil
}

// Down rolls back the most recently applied migration.
// Returns nil when there is no provider initialized.
func (m *Migrator) Down(ctx context.Context) error {
	// No provider means no migrations to roll back
	if m.provider == nil {
		m.logger.Info("No migrations to roll back - provider not initialized")
		return nil
	}

	m.logger.Info("Rolling back last migration")

	result, err := m.provider.Down(ctx)
	if err != nil {
		m.logger.Error("Failed to roll back migration (MID_060221143023)", "error", err)
		return fmt.Errorf("migration down failed (MID_060221143024): %w", err)
	}

	m.logger.Info("Migration rolled back (MID_060221143025)",
		"version", result.Source.Version,
		"duration_ms", result.Duration.Milliseconds(),
	)
	return nil
}

// DownTo rolls back all applied migrations that are newer than the specified
// version. The migration at version is NOT rolled back.
// Pass version 0 to roll back everything.
// Returns nil when there is no provider initialized.
func (m *Migrator) DownTo(ctx context.Context, version int64) error {
	// No provider means no migrations to roll back
	if m.provider == nil {
		m.logger.Info("No migrations to roll back - provider not initialized")
		return nil
	}

	m.logger.Info("Rolling back migrations to version (SHD_GSE_280)", "version", version)

	results, err := m.provider.DownTo(ctx, version)
	if err != nil {
		return fmt.Errorf("migration down-to %d failed (MID_060221143028): %w", version, err)
	}

	m.logger.Info("Migrations rolled back (MID_060221143029)", "count", len(results), "target_version", version)
	return nil
}

// Status returns the current state (applied / pending) of every migration
// known to the migrator, ordered by version ascending.
// Returns nil when there is no provider initialized.
func (m *Migrator) Status(ctx context.Context) ([]*gooselib.MigrationStatus, error) {
	if m.provider == nil {
		return nil, nil
	}

	statuses, err := m.provider.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("migration status failed (MID_060221143030): %w", err)
	}
	return statuses, nil
}

// GetVersion returns the highest migration version that has been applied to the
// database. Returns 0 when no migrations have been applied yet or no provider
// is initialized.
func (m *Migrator) GetVersion(ctx context.Context) (int64, error) {
	if m.provider == nil {
		return 0, nil
	}

	version, err := m.provider.GetDBVersion(ctx)
	if err != nil {
		return 0, fmt.Errorf("get version failed (MID_060221143031): %w", err)
	}
	return version, nil
}

// HasPending returns true when at least one migration has not yet been applied.
// Returns false when there is no provider initialized.
func (m *Migrator) HasPending(ctx context.Context) (bool, error) {
	if m.provider == nil {
		return false, nil
	}

	pending, err := m.provider.HasPending(ctx)
	if err != nil {
		return false, fmt.Errorf("has-pending check failed (MID_060221143032): %w", err)
	}
	return pending, nil
}

// ListSources returns every migration source known to this Migrator, ordered
// by version ascending. Each entry describes the migration type (SQL or Go),
// its file path, and its version number.
// Returns nil when there is no provider initialized.
func (m *Migrator) ListSources() []*gooselib.Source {
	if m.provider == nil {
		return nil
	}
	return m.provider.ListSources()
}
