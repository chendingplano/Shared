package goose

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	gooselib "github.com/pressly/goose/v3"
)

type testLogger struct{}

func (testLogger) Debug(message string, args ...any) {}
func (testLogger) Line(message string, args ...any)  {}
func (testLogger) Info(message string, args ...any)  {}
func (testLogger) Warn(message string, args ...any)  {}
func (testLogger) Error(message string, args ...any) {}
func (testLogger) Trace(message string)              {}
func (testLogger) Close()                            {}

type failingReadDirFS struct{ err error }

func (f failingReadDirFS) Open(name string) (fs.File, error) {
	return nil, errors.New("not implemented")
}

func (f failingReadDirFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return nil, f.err
}

func startCaseReport(t *testing.T, purpose, statement string) func() {
	start := time.Now()
	t.Logf("Purpose: %s", purpose)
	t.Logf("Statement: %s", statement)
	return func() {
		status := "success"
		if t.Failed() {
			status = "fail"
		}
		t.Logf("Execution status: %s", status)
		if t.Failed() {
			t.Logf("Error message: see failure output for %s", t.Name())
		}
		t.Logf("Time used: %d ms", time.Since(start).Milliseconds())
	}
}

func TestDialectFor(t *testing.T) {
	tests := []struct {
		name    string
		dbType  string
		want    gooselib.Dialect
		wantErr string
	}{
		{name: "pg", dbType: ApiTypes.PgName, want: gooselib.DialectPostgres},
		{name: "mysql", dbType: ApiTypes.MysqlName, want: gooselib.DialectMySQL},
		{name: "unsupported", dbType: "sqlite", wantErr: "unsupported database type"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer startCaseReport(
				t,
				"Verify database type to goose dialect mapping.",
				"Call dialectFor with configured dbType and validate returned dialect or error.",
			)()
			got, err := dialectFor(tt.dbType)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("dialect mismatch: got %q want %q", got, tt.want)
			}
		})
	}
}

func TestApplyDefaults(t *testing.T) {
	t.Run("all defaults", func(t *testing.T) {
		defer startCaseReport(
			t,
			"Validate default migration configuration values.",
			"Call applyDefaults with empty config and assert default fields.",
		)()
		cfg := applyDefaults(ApiTypes.MigrationConfig{})

		if cfg.MigrationsDir != "migrations" {
			t.Fatalf("MigrationsDir: got %q want %q", cfg.MigrationsDir, "migrations")
		}
		if cfg.TableName != "db_migrations" {
			t.Fatalf("TableName: got %q want %q", cfg.TableName, "db_migrations")
		}
		if !cfg.Verbose {
			t.Fatalf("Verbose default should be true")
		}
		if !cfg.AllowOutOfOrder {
			t.Fatalf("AllowOutOfOrder default should be true")
		}
	})

	t.Run("custom values", func(t *testing.T) {
		defer startCaseReport(
			t,
			"Validate explicit migration config values override defaults.",
			"Call applyDefaults with populated MigrationConfig and assert resulting values.",
		)()
		baseDir := t.TempDir()
		cfg := applyDefaults(ApiTypes.MigrationConfig{
			MigrationsFS:    baseDir,
			MigrationsDir:   "custom-migrations",
			TableName:       "goose_versions",
			Verbose:         "false",
			AllowOutOfOrder: "false",
		})

		if cfg.MigrationsDir != "custom-migrations" {
			t.Fatalf("MigrationsDir: got %q", cfg.MigrationsDir)
		}
		if cfg.TableName != "goose_versions" {
			t.Fatalf("TableName: got %q", cfg.TableName)
		}
		if cfg.Verbose {
			t.Fatalf("Verbose should be false")
		}
		if cfg.AllowOutOfOrder {
			t.Fatalf("AllowOutOfOrder should be false")
		}
	})
}

func TestHasMigrationFiles(t *testing.T) {
	t.Run("nil fs", func(t *testing.T) {
		defer startCaseReport(
			t,
			"Ensure nil filesystem is handled safely.",
			"Call hasMigrationFiles(nil) and expect false with no error.",
		)()
		ok, err := hasMigrationFiles(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Fatalf("expected false for nil fs")
		}
	})

	t.Run("empty dir", func(t *testing.T) {
		defer startCaseReport(
			t,
			"Ensure empty migrations directory reports no migration files.",
			"Call hasMigrationFiles on empty temp dir and expect false with no error.",
		)()
		dir := t.TempDir()
		ok, err := hasMigrationFiles(os.DirFS(dir))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Fatalf("expected false for empty fs")
		}
	})

	t.Run("find sql and go files", func(t *testing.T) {
		defer startCaseReport(
			t,
			"Ensure migration file discovery detects both SQL and Go migration files.",
			"Create .sql and .go files in temp dirs, call hasMigrationFiles, and expect true.",
		)()
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "20260101010101_test.sql"), []byte("-- sql"), 0o644); err != nil {
			t.Fatalf("write sql: %v", err)
		}
		ok, err := hasMigrationFiles(os.DirFS(dir))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected true when .sql exists")
		}

		dir2 := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir2, "20260101010101_test.go"), []byte("package main"), 0o644); err != nil {
			t.Fatalf("write go: %v", err)
		}
		ok, err = hasMigrationFiles(os.DirFS(dir2))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected true when .go exists")
		}
	})

	t.Run("readdir error", func(t *testing.T) {
		defer startCaseReport(
			t,
			"Ensure filesystem read errors are propagated.",
			"Call hasMigrationFiles on failing fs and verify returned error wraps original error.",
		)()
		want := errors.New("boom")
		_, err := hasMigrationFiles(failingReadDirFS{err: want})
		if !errors.Is(err, want) {
			t.Fatalf("expected wrapped read error %v, got %v", want, err)
		}
	})
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		name string
		in   string
		out  string
	}{
		{name: "normal", in: "Add Tags Column", out: "add_tags_column"},
		{name: "special chars", in: " add---tags!!!column ", out: "add_tags_column"},
		{name: "empty to fallback", in: "   ", out: "migration"},
		{name: "truncate to 60", in: strings.Repeat("a", 80), out: strings.Repeat("a", 60)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer startCaseReport(
				t,
				"Validate migration description slug normalization.",
				"Call slugify on the input and compare with expected sanitized slug.",
			)()
			got := slugify(tt.in)
			if got != tt.out {
				t.Fatalf("slugify(%q): got %q want %q", tt.in, got, tt.out)
			}
		})
	}
}

func TestBuildMigrationSQL(t *testing.T) {
	defer startCaseReport(
		t,
		"Validate generated goose SQL sections for Up/Down migration content.",
		"Call buildMigrationSQL with and without down SQL and assert emitted section markers and trimming behavior.",
	)()
	withDown := buildMigrationSQL("  CREATE TABLE t(id INT);  ", "  DROP TABLE t;  ")
	if !strings.Contains(withDown, "-- +goose Up") || !strings.Contains(withDown, "-- +goose Down") {
		t.Fatalf("expected both Up and Down sections, got:\n%s", withDown)
	}
	if strings.Contains(withDown, "  CREATE TABLE") || strings.Contains(withDown, "DROP TABLE t;  ") {
		t.Fatalf("expected trimmed SQL blocks, got:\n%s", withDown)
	}

	withoutDown := buildMigrationSQL("SELECT 1;", "   ")
	if strings.Contains(withoutDown, "-- +goose Down") {
		t.Fatalf("did not expect Down section when downSQL is empty")
	}
}

func TestCreateMigration(t *testing.T) {
	logger := testLogger{}

	t.Run("requires migrations dir", func(t *testing.T) {
		defer startCaseReport(
			t,
			"Ensure CreateMigration validates required migrations directory.",
			"Call CreateMigration with empty cfg.MigrationsDir and expect validation error.",
		)()
		m := &Migrator{logger: logger, cfg: GooseConfig{}}
		_, err := m.CreateMigration("desc", "SELECT 1", "")
		if err == nil || !strings.Contains(err.Error(), "MID_060221143011") {
			t.Fatalf("expected MigrationsDir error, got %v", err)
		}
	})

	t.Run("requires non-empty upSQL", func(t *testing.T) {
		defer startCaseReport(
			t,
			"Ensure CreateMigration validates non-empty upSQL.",
			"Call CreateMigration with blank upSQL and expect validation error.",
		)()
		m := &Migrator{logger: logger, cfg: GooseConfig{MigrationsDir: t.TempDir()}}
		_, err := m.CreateMigration("desc", "  ", "")
		if err == nil || !strings.Contains(err.Error(), "MID_060221143012") {
			t.Fatalf("expected upSQL error, got %v", err)
		}
	})

	t.Run("writes timestamped migration file", func(t *testing.T) {
		defer startCaseReport(
			t,
			"Verify CreateMigration writes correctly named and formatted migration file.",
			"Call CreateMigration, validate filename pattern, then compare written content with buildMigrationSQL output.",
		)()
		dir := t.TempDir()
		m := &Migrator{logger: logger, cfg: GooseConfig{MigrationsDir: dir}}

		filename, err := m.CreateMigration("Add Tags Column", "CREATE TABLE t(id INT);", "DROP TABLE t;")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		matched, err := regexp.MatchString(`^\d{14}_add_tags_column\.sql$`, filename)
		if err != nil {
			t.Fatalf("regex error: %v", err)
		}
		if !matched {
			t.Fatalf("unexpected filename format: %s", filename)
		}

		content, err := os.ReadFile(filepath.Join(dir, filename))
		if err != nil {
			t.Fatalf("failed to read migration file: %v", err)
		}
		want := buildMigrationSQL("CREATE TABLE t(id INT);", "DROP TABLE t;")
		if string(content) != want {
			t.Fatalf("migration content mismatch\nwant:\n%s\n\ngot:\n%s", want, string(content))
		}
	})
}

func TestCreateAndApply(t *testing.T) {
	ctx := context.Background()
	logger := testLogger{}

	t.Run("success when migrations fs has no files", func(t *testing.T) {
		defer startCaseReport(
			t,
			"Verify CreateAndApply succeeds when migration provider remains nil (no migration files visible).",
			"Call CreateAndApply with write dir and empty MigrationsFS dir; validate file created and provider remains nil.",
		)()
		writeDir := t.TempDir()
		emptyFSDir := t.TempDir()

		m := &Migrator{
			logger:  logger,
			dialect: gooselib.DialectPostgres,
			cfg: GooseConfig{
				MigrationsDir: writeDir,
				MigrationsFS:  os.DirFS(emptyFSDir),
				TableName:     "db_migrations",
			},
		}

		filename, err := m.CreateAndApply(ctx, "create sample", "SELECT 1;", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if filename == "" {
			t.Fatalf("expected filename")
		}
		if _, statErr := os.Stat(filepath.Join(writeDir, filename)); statErr != nil {
			t.Fatalf("expected migration file to exist: %v", statErr)
		}
		if m.provider != nil {
			t.Fatalf("provider should remain nil when MigrationsFS has no files")
		}
	})

	t.Run("returns filename when provider rebuild fails", func(t *testing.T) {
		defer startCaseReport(
			t,
			"Verify CreateAndApply returns generated filename even when provider rebuild fails.",
			"Call CreateAndApply with nil DB and non-empty migration FS after file creation; assert filename returned and error reported.",
		)()
		dir := t.TempDir()
		m := &Migrator{
			logger:  logger,
			dialect: gooselib.DialectPostgres,
			cfg: GooseConfig{
				MigrationsDir: dir,
				MigrationsFS:  os.DirFS(dir),
				TableName:     "db_migrations",
			},
			// nil DB forces gooselib.NewProvider to fail once migration file exists
			db: nil,
		}

		filename, err := m.CreateAndApply(ctx, "will fail rebuild", "SELECT 1;", "")
		if filename == "" {
			t.Fatalf("expected filename even on rebuild failure")
		}
		if err == nil || !strings.Contains(err.Error(), "failed to reload migrations") {
			t.Fatalf("expected rebuild failure, got %v", err)
		}
		if _, statErr := os.Stat(filepath.Join(dir, filename)); statErr != nil {
			t.Fatalf("expected migration file to exist after failure: %v", statErr)
		}
	})
}

func TestRebuildProviderWithNoMigrations(t *testing.T) {
	defer startCaseReport(
		t,
		"Ensure rebuildProvider is a safe no-op when no migration files exist.",
		"Call rebuildProvider with empty MigrationsFS and assert provider remains nil with no error.",
	)()
	m := &Migrator{
		logger:  testLogger{},
		dialect: gooselib.DialectPostgres,
		cfg: GooseConfig{
			MigrationsFS: os.DirFS(t.TempDir()),
			TableName:    "db_migrations",
		},
	}

	if err := m.rebuildProvider(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.provider != nil {
		t.Fatalf("expected nil provider when no migrations exist")
	}
}

func TestNewWithDB(t *testing.T) {
	logger := testLogger{}

	t.Run("invalid db type", func(t *testing.T) {
		defer startCaseReport(
			t,
			"Ensure NewWithDB rejects unsupported database types.",
			"Call NewWithDB with invalid dbType and expect unsupported database type error.",
		)()
		_, err := NewWithDB(nil, "bad-db", ApiTypes.MigrationConfig{}, logger)
		if err == nil || !strings.Contains(err.Error(), "unsupported database type") {
			t.Fatalf("expected unsupported db type error, got %v", err)
		}
	})

	t.Run("no migrations returns migrator with nil provider", func(t *testing.T) {
		defer startCaseReport(
			t,
			"Ensure NewWithDB initializes migrator successfully when migrations directory is empty.",
			"Call NewWithDB with valid dbType and empty migrations dir; validate non-nil migrator and nil provider.",
		)()
		dir := t.TempDir()
		m, err := NewWithDB(&sql.DB{}, ApiTypes.PgName, ApiTypes.MigrationConfig{
			MigrationsFS:  dir,
			MigrationsDir: dir,
			TableName:     "versions",
		}, logger)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m == nil {
			t.Fatalf("expected non-nil migrator")
		}
		if m.provider != nil {
			t.Fatalf("expected nil provider when migrations directory is empty")
		}
		if m.cfg.TableName != "versions" {
			t.Fatalf("expected custom table name to be preserved")
		}
	})
}

func TestRunMigrations(t *testing.T) {
	logger := testLogger{}
	ctx := context.Background()

	t.Run("mkdir failure", func(t *testing.T) {
		defer startCaseReport(
			t,
			"Ensure RunMigrations surfaces migrations directory creation failures.",
			"Set MigrationsDir to an existing file path and call RunMigrations; expect mkdir-related error.",
		)()
		base := t.TempDir()
		filePath := filepath.Join(base, "not-a-dir")
		if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
			t.Fatalf("setup file: %v", err)
		}

		_, err := RunMigrations(ctx, logger, "test", ApiTypes.MigrationConfig{MigrationsDir: filePath}, &sql.DB{})
		if err == nil || !strings.Contains(err.Error(), "MID_060221143005") {
			t.Fatalf("expected mkdir error, got %v", err)
		}
	})

	t.Run("success with empty migrations", func(t *testing.T) {
		defer startCaseReport(
			t,
			"Ensure RunMigrations succeeds when no migration files are present.",
			"Call RunMigrations with empty migrations directory and valid DBType; expect non-nil migrator and nil provider.",
		)()
		oldDBType := ApiTypes.DBType
		ApiTypes.DBType = ApiTypes.PgName
		t.Cleanup(func() { ApiTypes.DBType = oldDBType })

		dir := t.TempDir()
		m, err := RunMigrations(ctx, logger, "test", ApiTypes.MigrationConfig{
			MigrationsDir: dir,
			MigrationsFS:  dir,
		}, &sql.DB{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m == nil {
			t.Fatalf("expected migrator")
		}
		if m.provider != nil {
			t.Fatalf("expected nil provider for empty migrations")
		}
	})
}

func TestRunWrapperInitializers(t *testing.T) {
	defer startCaseReport(
		t,
		"Validate project/shared/autotester wrapper initializers for nil DB, first init, and repeated init guards.",
		"Call RunProjectMigrations, RunSharedMigrations, and RunAutoTesterMigrations across failure and success scenarios and verify expected error codes.",
	)()
	logger := testLogger{}
	ctx := context.Background()

	oldProject := ProjectMigrator
	oldShared := SharedMigrator
	oldAuto := AutoTesterMigrator
	oldDBType := ApiTypes.DBType
	ProjectMigrator = nil
	SharedMigrator = nil
	AutoTesterMigrator = nil
	ApiTypes.DBType = ApiTypes.PgName
	t.Cleanup(func() {
		ProjectMigrator = oldProject
		SharedMigrator = oldShared
		AutoTesterMigrator = oldAuto
		ApiTypes.DBType = oldDBType
	})

	cfg := ApiTypes.MigrationConfig{MigrationsDir: t.TempDir(), MigrationsFS: t.TempDir()}

	if err := RunProjectMigrations(ctx, logger, cfg, nil); err == nil || !strings.Contains(err.Error(), "MID_060221143035") {
		t.Fatalf("expected nil project DB error, got %v", err)
	}
	if err := RunSharedMigrations(ctx, logger, cfg, nil); err == nil || !strings.Contains(err.Error(), "MID_060221143002") {
		t.Fatalf("expected nil shared DB error, got %v", err)
	}
	if err := RunAutoTesterMigrations(ctx, logger, cfg, nil); err == nil || !strings.Contains(err.Error(), "MID_060221143012") {
		t.Fatalf("expected nil autotester DB error, got %v", err)
	}

	if err := RunProjectMigrations(ctx, logger, cfg, &sql.DB{}); err != nil {
		t.Fatalf("project init unexpected error: %v", err)
	}
	if err := RunProjectMigrations(ctx, logger, cfg, &sql.DB{}); err == nil || !strings.Contains(err.Error(), "MID_060221143034") {
		t.Fatalf("expected already initialized project error, got %v", err)
	}

	if err := RunSharedMigrations(ctx, logger, cfg, &sql.DB{}); err != nil {
		t.Fatalf("shared init unexpected error: %v", err)
	}
	if err := RunSharedMigrations(ctx, logger, cfg, &sql.DB{}); err == nil || !strings.Contains(err.Error(), "MID_060221143001") {
		t.Fatalf("expected already initialized shared error, got %v", err)
	}

	if err := RunAutoTesterMigrations(ctx, logger, cfg, &sql.DB{}); err != nil {
		t.Fatalf("autotester init unexpected error: %v", err)
	}
	if err := RunAutoTesterMigrations(ctx, logger, cfg, &sql.DB{}); err == nil || !strings.Contains(err.Error(), "MID_060221143011") {
		t.Fatalf("expected already initialized autotester error, got %v", err)
	}
}

func TestNoOpMethodsWhenProviderNil(t *testing.T) {
	defer startCaseReport(
		t,
		"Ensure migrator API methods are safe no-ops when provider is nil.",
		"Call Up/Down/Status/Version/Pending/Source methods with nil provider and assert nil or default results.",
	)()
	ctx := context.Background()
	m := &Migrator{logger: testLogger{}, provider: nil}

	if err := m.Up(ctx); err != nil {
		t.Fatalf("Up should be no-op: %v", err)
	}
	if err := m.UpByOne(ctx); err != nil {
		t.Fatalf("UpByOne should be no-op: %v", err)
	}
	if err := m.UpTo(ctx, 42); err != nil {
		t.Fatalf("UpTo should be no-op: %v", err)
	}
	if err := m.Down(ctx); err != nil {
		t.Fatalf("Down should be no-op: %v", err)
	}
	if err := m.DownTo(ctx, 0); err != nil {
		t.Fatalf("DownTo should be no-op: %v", err)
	}

	statuses, err := m.Status(ctx)
	if err != nil {
		t.Fatalf("Status should be no-op: %v", err)
	}
	if statuses != nil {
		t.Fatalf("expected nil statuses when provider is nil")
	}

	version, err := m.GetVersion(ctx)
	if err != nil {
		t.Fatalf("GetVersion should be no-op: %v", err)
	}
	if version != 0 {
		t.Fatalf("expected version 0, got %d", version)
	}

	pending, err := m.HasPending(ctx)
	if err != nil {
		t.Fatalf("HasPending should be no-op: %v", err)
	}
	if pending {
		t.Fatalf("expected pending=false when provider is nil")
	}

	if sources := m.ListSources(); sources != nil {
		t.Fatalf("expected nil sources when provider is nil")
	}
}
