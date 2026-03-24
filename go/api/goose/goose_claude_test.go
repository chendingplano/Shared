// goose_pg_test.go — PostgreSQL-focused comprehensive tests for the goose
// database migration utilities package.
//
// # Running
//
//	go test ./api/goose/ -run TestGoosePG -testname goose_pg [-v]
//	go test ./api/goose/ -run TestGoosePG -testname goose_pg -pg-dsn "postgres://user:pass@localhost/testonly_goose?sslmode=disable"
//
// # Environment
//
//	PGTEST_DSN   PostgreSQL DSN used for integration tests when -pg-dsn is not
//	             given. The database name MUST start with "testonly_" for safety.
//	             Example: postgres://postgres:secret@localhost/testonly_goose?sslmode=disable
//
// # Flags
//
//	-testname <name>   Test-run label used in the DB log and the Markdown report.
//	                   Default: "goose_pg"
//	-pg-dsn   <dsn>   PostgreSQL DSN (overrides $PGTEST_DSN).
//	-v                 Verbose: prints per-case detail to stdout.
//
// # Report
//
//	<workspace>/<project>/docs/tests/testreport_<testname>.md
//
// # Test-case distribution
//
//	~70 % correct-path operations · ~30 % incorrect-path / error-handling
//
// Created: 2026/03/24 by Claude Code and Chen Ding
package goose

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/lib/pq"
	gooselib "github.com/pressly/goose/v3"
)

// ---------------------------------------------------------------------------
// CLI flags
// ---------------------------------------------------------------------------

var (
	flagTestName = flag.String("testname", "goose_pg", "name of this test run (used in DB log and report)")
	flagPgDSN    = flag.String("pg-dsn", "", "PostgreSQL DSN for integration tests (overrides $PGTEST_DSN)")
)

// ---------------------------------------------------------------------------
// Test-framework types
// ---------------------------------------------------------------------------

// tcRecord captures the outcome of one test case.
type tcRecord struct {
	TCID      int
	Purpose   string
	Statement string // SQL or code exercised; may be empty
	Result    string // "PASS" or "FAIL"
	ErrMsg    string
	TimeMs    int64
}

var (
	tcIDGen   int64 // atomic; increments once per runTC call
	tcRecMu   sync.Mutex
	tcRecords []tcRecord

	pgTestDB *sql.DB // shared PostgreSQL handle; nil when no DSN is provided
)

// nextTCID returns the next globally-unique test-case identifier.
func nextTCID() int {
	return int(atomic.AddInt64(&tcIDGen, 1))
}

// ---------------------------------------------------------------------------
// TestMain — setup · run · log · report
// ---------------------------------------------------------------------------

func TestMain(m *testing.M) {
	flag.Parse()

	// ---- optional PostgreSQL setup -----------------------------------------
	dsn := *flagPgDSN
	if dsn == "" {
		dsn = os.Getenv("PGTEST_DSN")
	}
	if dsn != "" {
		var err error
		pgTestDB, err = ensurePGDatabaseReady(dsn)
		if err != nil {
			log.Printf("***** failed creating the table")
			fmt.Fprintf(os.Stderr,
				"[goose_pg_test] WARNING: PostgreSQL unavailable (%v); integration tests will be skipped\n", err)
		} else {
			log.Printf("+++++ test table created")
			pgTestDB.SetMaxOpenConns(5)
			pgTestDB.SetConnMaxLifetime(30 * time.Minute)
			ensureTestLogTable(pgTestDB)
		}
	}

	// ---- run all tests ------------------------------------------------------
	exitCode := m.Run()

	// ---- persist results to DB and write report ----------------------------
	if pgTestDB != nil {
		if err := logResultsToDB(pgTestDB, *flagTestName, tcRecords); err != nil {
			fmt.Fprintf(os.Stderr, "[goose_pg_test] WARNING: DB log failed: %v\n", err)
		}
		pgTestDB.Close()
	}

	if err := generateMarkdownReport(*flagTestName, tcRecords); err != nil {
		fmt.Fprintf(os.Stderr, "[goose_pg_test] WARNING: report generation failed: %v\n", err)
	}

	os.Exit(exitCode)
}

// ---------------------------------------------------------------------------
// runTC — single test-case runner with TCID tracking
// ---------------------------------------------------------------------------

// runTC runs fn as a named sub-test, assigns it the next TCID, records the
// outcome, and — in verbose mode — prints the per-case detail line.
//
// purpose is a short human-readable description of what is being verified.
// stmt   is the SQL statement or code construct exercised (may be empty).
// fn     uses the standard *testing.T API; t.Fatal / t.Error signal failure.
func runTC(t *testing.T, purpose, stmt string, fn func(t *testing.T)) {
	t.Helper()
	tcid := nextTCID()

	rec := tcRecord{
		TCID:      tcid,
		Purpose:   purpose,
		Statement: stmt,
	}

	subName := fmt.Sprintf("TCID%02d", tcid)
	t.Run(subName, func(st *testing.T) {
		st.Helper()
		start := time.Now()
		fn(st)
		rec.TimeMs = time.Since(start).Milliseconds()

		if st.Failed() {
			rec.Result = "FAIL"
			rec.ErrMsg = fmt.Sprintf("see output for %s/%s", t.Name(), subName)
		} else {
			rec.Result = "PASS"
		}

		tcRecMu.Lock()
		tcRecords = append(tcRecords, rec)
		tcRecMu.Unlock()

		if testing.Verbose() {
			printVerbose(st, rec)
		}
	})
}

// printVerbose emits the per-case detail block required by the spec.
func printVerbose(t *testing.T, rec tcRecord) {
	t.Helper()
	var sb strings.Builder
	fmt.Fprintf(&sb, "\n--- TCID %02d ---\n", rec.TCID)
	fmt.Fprintf(&sb, "  Purpose   : %s\n", rec.Purpose)
	if rec.Statement != "" {
		fmt.Fprintf(&sb, "  Statement : %s\n", rec.Statement)
	}
	fmt.Fprintf(&sb, "  Result    : %s\n", rec.Result)
	if rec.ErrMsg != "" {
		fmt.Fprintf(&sb, "  Error     : %s\n", rec.ErrMsg)
	}
	fmt.Fprintf(&sb, "  Time      : %d ms\n", rec.TimeMs)
	t.Log(sb.String())
}

// ---------------------------------------------------------------------------
// DB helpers (autotester.test_log)
// ---------------------------------------------------------------------------

const createTestLogSQL = `
CREATE SCHEMA IF NOT EXISTS autotester;
CREATE TABLE IF NOT EXISTS autotester.test_log (
    id          BIGSERIAL PRIMARY KEY,
    run_id      TEXT        NOT NULL,
    testname    TEXT        NOT NULL,
    tcid        INTEGER     NOT NULL,
    purpose     TEXT        NOT NULL,
    statement   TEXT,
    result      TEXT        NOT NULL CHECK (result IN ('PASS','FAIL','SKIP')),
    error_msg   TEXT,
    time_ms     BIGINT      NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (testname, tcid, run_id)
);`

func ensurePGDatabaseReady(targetDSN string) (*sql.DB, error) {
	db, err := sql.Open("postgres", targetDSN)
	if err != nil {
		return nil, fmt.Errorf("sql.Open target DB: %w", err)
	}
	if err = db.Ping(); err == nil {
		return db, nil
	}
	db.Close()

	if !isMissingDatabaseError(err) {
		return nil, fmt.Errorf("ping target DB: %w", err)
	}

	dbName, adminDSN, parseErr := parseTargetAndAdminDSN(targetDSN)
	if parseErr != nil {
		return nil, fmt.Errorf("target DB missing and DSN parse failed: %w", parseErr)
	}
	if createErr := createDatabaseIfNotExists(adminDSN, dbName); createErr != nil {
		return nil, fmt.Errorf("create database %q failed: %w", dbName, createErr)
	}

	db, err = sql.Open("postgres", targetDSN)
	if err != nil {
		return nil, fmt.Errorf("re-open target DB: %w", err)
	}
	if err = db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping target DB after create: %w", err)
	}
	return db, nil
}

func isMissingDatabaseError(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return string(pqErr.Code) == "3D000" // invalid_catalog_name (database does not exist)
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "does not exist")
}

func createDatabaseIfNotExists(adminDSN, dbName string) error {
	adminDB, err := sql.Open("postgres", adminDSN)
	if err != nil {
		return fmt.Errorf("open admin DB: %w", err)
	}
	defer adminDB.Close()

	if err := adminDB.Ping(); err != nil {
		return fmt.Errorf("ping admin DB: %w", err)
	}

	_, err = adminDB.Exec("CREATE DATABASE " + pq.QuoteIdentifier(dbName))
	if err == nil {
		return nil
	}

	var pqErr *pq.Error
	if errors.As(err, &pqErr) && string(pqErr.Code) == "42P04" {
		return nil // duplicate_database
	}
	return err
}

func parseTargetAndAdminDSN(dsn string) (dbName string, adminDSN string, err error) {
	if strings.Contains(dsn, "://") {
		u, parseErr := url.Parse(dsn)
		if parseErr != nil {
			return "", "", parseErr
		}
		dbName = strings.TrimPrefix(u.Path, "/")
		if dbName == "" {
			return "", "", fmt.Errorf("missing DB name in URL DSN")
		}
		u.Path = "/postgres"
		return dbName, u.String(), nil
	}

	// key=value DSN (e.g. "host=... user=... dbname=... sslmode=disable")
	re := regexp.MustCompile(`(?:^|\s)dbname=([^ ]+)`)
	m := re.FindStringSubmatch(dsn)
	if len(m) != 2 {
		return "", "", fmt.Errorf("missing dbname in DSN")
	}
	dbName = strings.Trim(m[1], `"'`)
	adminDSN = re.ReplaceAllString(dsn, " dbname=postgres")
	adminDSN = strings.TrimSpace(adminDSN)
	return dbName, adminDSN, nil
}

func ensureTestLogTable(db *sql.DB) {
	if _, err := db.Exec(createTestLogSQL); err != nil {
		fmt.Fprintf(os.Stderr,
			"[goose_pg_test] WARNING: could not create autotester.test_log: %v\n", err)
	}
}

func logResultsToDB(db *sql.DB, testname string, records []tcRecord) error {
	runID := fmt.Sprintf("%s_%d", testname, time.Now().UnixMilli())
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.Prepare(`
		INSERT INTO autotester.test_log
		       (run_id, testname, tcid, purpose, statement, result, error_msg, time_ms)
		VALUES ($1,     $2,       $3,   $4,      $5,        $6,    $7,       $8)
		ON CONFLICT (testname, tcid, run_id) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, r := range records {
		if _, err := stmt.Exec(
			runID, testname, r.TCID, r.Purpose,
			nullableStr(r.Statement), r.Result,
			nullableStr(r.ErrMsg), r.TimeMs,
		); err != nil {
			return fmt.Errorf("insert TCID %d: %w", r.TCID, err)
		}
	}
	return tx.Commit()
}

func nullableStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// ---------------------------------------------------------------------------
// Markdown report
// ---------------------------------------------------------------------------

// deriveProjectReportDir infers the project root and returns the docs/tests
// directory path, together with the project name, by stripping the leading
// ~/Workspace/<project>/ prefix from the current working directory.
func deriveProjectReportDir() (projectName, reportDir string) {
	wd, err := os.Getwd()
	if err != nil {
		return "unknown", filepath.Join("docs", "tests")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "unknown", filepath.Join("docs", "tests")
	}
	workspaceRoot := filepath.Join(home, "Workspace")
	rel, err := filepath.Rel(workspaceRoot, wd)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "unknown", filepath.Join("docs", "tests")
	}
	parts := strings.SplitN(filepath.ToSlash(rel), "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		return "unknown", filepath.Join("docs", "tests")
	}
	projectName = parts[0]
	reportDir = filepath.Join(workspaceRoot, projectName, "docs", "tests")
	return projectName, reportDir
}

func generateMarkdownReport(testname string, records []tcRecord) error {
	if len(records) == 0 {
		return nil
	}

	projectName, reportDir := deriveProjectReportDir()
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		return fmt.Errorf("create report dir %s: %w", reportDir, err)
	}

	reportPath := filepath.Join(reportDir, "testreport_"+testname+".md")

	// Summary counts
	pass, fail := 0, 0
	for _, r := range records {
		if r.Result == "PASS" {
			pass++
		} else {
			fail++
		}
	}

	var sb strings.Builder
	now := time.Now().UTC().Format("2006-01-02 15:04:05 UTC")

	fmt.Fprintf(&sb, "# Test Report: %s\n\n", testname)
	fmt.Fprintf(&sb, "**Project:** %s  \n", projectName)
	fmt.Fprintf(&sb, "**Package:** `github.com/chendingplano/shared/go/api/goose`  \n")
	fmt.Fprintf(&sb, "**Generated:** %s  \n\n", now)

	fmt.Fprintf(&sb, "## Summary\n\n")
	fmt.Fprintf(&sb, "| Total | Pass | Fail | Pass Rate |\n")
	fmt.Fprintf(&sb, "|------:|-----:|-----:|----------:|\n")
	total := pass + fail
	pct := 0.0
	if total > 0 {
		pct = float64(pass) / float64(total) * 100
	}
	fmt.Fprintf(&sb, "| %d | %d | %d | %.1f%% |\n\n", total, pass, fail, pct)

	fmt.Fprintf(&sb, "## Test Cases\n\n")
	fmt.Fprintf(&sb, "| TCID | Purpose | Statement | Result | Error | Time (ms) |\n")
	fmt.Fprintf(&sb, "|-----:|---------|-----------|:------:|-------|----------:|\n")

	// Sort by TCID for deterministic output
	sorted := make([]tcRecord, len(records))
	copy(sorted, records)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].TCID < sorted[j].TCID })

	for _, r := range sorted {
		resultBadge := "✅ PASS"
		if r.Result == "FAIL" {
			resultBadge = "❌ FAIL"
		}
		stmt := r.Statement
		if stmt == "" {
			stmt = "—"
		}
		errMsg := r.ErrMsg
		if errMsg == "" {
			errMsg = "—"
		}
		fmt.Fprintf(&sb, "| %d | %s | `%s` | %s | %s | %d |\n",
			r.TCID, r.Purpose, mdEscape(stmt), resultBadge, mdEscape(errMsg), r.TimeMs)
	}

	fmt.Fprintf(&sb, "\n---\n*Generated by `goose_pg_test.go` — testname: `%s`*\n", testname)

	if err := os.WriteFile(reportPath, []byte(sb.String()), 0o644); err != nil {
		return fmt.Errorf("write report %s: %w", reportPath, err)
	}
	fmt.Printf("[goose_pg_test] Report written → %s\n", reportPath)
	return nil
}

// mdEscape replaces pipe characters so they don't break Markdown tables.
func mdEscape(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}

// ---------------------------------------------------------------------------
// FailingReadDirFS — helper for error-path tests
// ---------------------------------------------------------------------------

// pgFailFS is a read-only FS that always returns an error from ReadDir.
// Distinct from the one in goose_codex_test.go to avoid redeclaration.
type pgFailFS struct{ err error }

func (f pgFailFS) Open(string) (fs.File, error)          { return nil, errors.New("not implemented") }
func (f pgFailFS) ReadDir(string) ([]fs.DirEntry, error) { return nil, f.err }

// ---------------------------------------------------------------------------
// Shared testLogger (re-declared to avoid duplicate; identical implementation)
// ---------------------------------------------------------------------------
// NOTE: testLogger is already declared in goose_codex_test.go (same package),
// so we reuse it directly.  No duplicate declaration needed here.

// ---------------------------------------------------------------------------
// ══════════════════════════════════════════════════════════════════════════
// UNIT TESTS — no PostgreSQL required
// ══════════════════════════════════════════════════════════════════════════
// ---------------------------------------------------------------------------

// TestGoosePGUnit_DialectMapping verifies dialectFor covers all supported and
// unsupported database type strings.
func TestGoosePGUnit_DialectMapping(t *testing.T) {
	// TCID 1 — CORRECT: PgName resolves to DialectPostgres
	runTC(t,
		"dialectFor maps PgName to gooselib.DialectPostgres",
		`dialectFor(ApiTypes.PgName)`,
		func(t *testing.T) {
			got, err := dialectFor(ApiTypes.PgName)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != gooselib.DialectPostgres {
				t.Fatalf("got %q, want %q", got, gooselib.DialectPostgres)
			}
		})

	// TCID 2 — CORRECT: MysqlName resolves to DialectMySQL
	runTC(t,
		"dialectFor maps MysqlName to gooselib.DialectMySQL",
		`dialectFor(ApiTypes.MysqlName)`,
		func(t *testing.T) {
			got, err := dialectFor(ApiTypes.MysqlName)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != gooselib.DialectMySQL {
				t.Fatalf("got %q, want %q", got, gooselib.DialectMySQL)
			}
		})

	// TCID 3 — INCORRECT: unknown db type must return an error
	runTC(t,
		"dialectFor rejects unsupported db type 'sqlite3'",
		`dialectFor("sqlite3")`,
		func(t *testing.T) {
			_, err := dialectFor("sqlite3")
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "unsupported database type") {
				t.Fatalf("error missing 'unsupported database type': %v", err)
			}
		})

	// TCID 4 — INCORRECT: empty string must be rejected
	runTC(t,
		"dialectFor rejects empty-string db type",
		`dialectFor("")`,
		func(t *testing.T) {
			_, err := dialectFor("")
			if err == nil {
				t.Fatalf("expected error for empty db type, got nil")
			}
		})
}

// TestGoosePGUnit_ApplyDefaults verifies GooseConfig defaults and explicit
// overrides produced by applyDefaults.
func TestGoosePGUnit_ApplyDefaults(t *testing.T) {
	// TCID 5 — CORRECT: empty config uses default MigrationsDir
	runTC(t,
		"applyDefaults sets MigrationsDir to 'migrations' when not specified",
		"applyDefaults(ApiTypes.MigrationConfig{})",
		func(t *testing.T) {
			cfg := applyDefaults(ApiTypes.MigrationConfig{})
			if cfg.MigrationsDir != "migrations" {
				t.Fatalf("got %q, want %q", cfg.MigrationsDir, "migrations")
			}
		})

	// TCID 6 — CORRECT: default TableName is "db_migrations"
	runTC(t,
		"applyDefaults sets TableName to 'db_migrations' when not specified",
		"applyDefaults(ApiTypes.MigrationConfig{})",
		func(t *testing.T) {
			cfg := applyDefaults(ApiTypes.MigrationConfig{})
			if cfg.TableName != "db_migrations" {
				t.Fatalf("got %q, want %q", cfg.TableName, "db_migrations")
			}
		})

	// TCID 7 — CORRECT: Verbose defaults to true
	runTC(t,
		"applyDefaults sets Verbose=true when Verbose field is empty",
		"applyDefaults(ApiTypes.MigrationConfig{})",
		func(t *testing.T) {
			cfg := applyDefaults(ApiTypes.MigrationConfig{})
			if !cfg.Verbose {
				t.Fatalf("expected Verbose=true, got false")
			}
		})

	// TCID 8 — CORRECT: AllowOutOfOrder defaults to true
	runTC(t,
		"applyDefaults sets AllowOutOfOrder=true when field is empty",
		"applyDefaults(ApiTypes.MigrationConfig{})",
		func(t *testing.T) {
			cfg := applyDefaults(ApiTypes.MigrationConfig{})
			if !cfg.AllowOutOfOrder {
				t.Fatalf("expected AllowOutOfOrder=true, got false")
			}
		})

	// TCID 9 — CORRECT: Verbose="false" is honoured
	runTC(t,
		"applyDefaults sets Verbose=false when Verbose='false'",
		`applyDefaults(ApiTypes.MigrationConfig{Verbose: "false"})`,
		func(t *testing.T) {
			cfg := applyDefaults(ApiTypes.MigrationConfig{Verbose: "false"})
			if cfg.Verbose {
				t.Fatalf("expected Verbose=false, got true")
			}
		})

	// TCID 10 — CORRECT: AllowOutOfOrder="false" is honoured
	runTC(t,
		"applyDefaults sets AllowOutOfOrder=false when AllowOutOfOrder='false'",
		`applyDefaults(ApiTypes.MigrationConfig{AllowOutOfOrder: "false"})`,
		func(t *testing.T) {
			cfg := applyDefaults(ApiTypes.MigrationConfig{AllowOutOfOrder: "false"})
			if cfg.AllowOutOfOrder {
				t.Fatalf("expected AllowOutOfOrder=false, got true")
			}
		})

	// TCID 11 — CORRECT: explicit custom table name is preserved
	runTC(t,
		"applyDefaults preserves custom TableName",
		`applyDefaults(ApiTypes.MigrationConfig{TableName: "my_migrations"})`,
		func(t *testing.T) {
			cfg := applyDefaults(ApiTypes.MigrationConfig{TableName: "my_migrations"})
			if cfg.TableName != "my_migrations" {
				t.Fatalf("got %q, want %q", cfg.TableName, "my_migrations")
			}
		})

	// TCID 12 — CORRECT: custom MigrationsDir is preserved
	runTC(t,
		"applyDefaults preserves custom MigrationsDir",
		`applyDefaults(ApiTypes.MigrationConfig{MigrationsDir: "db/migrations"})`,
		func(t *testing.T) {
			cfg := applyDefaults(ApiTypes.MigrationConfig{MigrationsDir: "db/migrations"})
			if cfg.MigrationsDir != "db/migrations" {
				t.Fatalf("got %q, want %q", cfg.MigrationsDir, "db/migrations")
			}
		})
}

// TestGoosePGUnit_HasMigrationFiles exercises hasMigrationFiles across valid
// and error-inducing inputs.
func TestGoosePGUnit_HasMigrationFiles(t *testing.T) {
	// TCID 13 — CORRECT: nil FS returns false with no error
	runTC(t,
		"hasMigrationFiles returns false for nil FS without error",
		"hasMigrationFiles(nil)",
		func(t *testing.T) {
			ok, err := hasMigrationFiles(nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ok {
				t.Fatalf("expected false, got true")
			}
		})

	// TCID 14 — CORRECT: empty directory returns false
	runTC(t,
		"hasMigrationFiles returns false for empty directory",
		"hasMigrationFiles(os.DirFS(emptyDir))",
		func(t *testing.T) {
			dir := t.TempDir()
			ok, err := hasMigrationFiles(os.DirFS(dir))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ok {
				t.Fatalf("expected false for empty directory")
			}
		})

	// TCID 15 — CORRECT: directory containing a .sql file returns true
	runTC(t,
		"hasMigrationFiles returns true when a .sql file is present",
		"hasMigrationFiles(os.DirFS(dirWithSQL))",
		func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, "20260101_create.sql"), "-- sql")
			ok, err := hasMigrationFiles(os.DirFS(dir))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !ok {
				t.Fatalf("expected true when .sql file exists")
			}
		})

	// TCID 16 — CORRECT: directory containing a .go file returns true
	runTC(t,
		"hasMigrationFiles returns true when a .go migration file is present",
		"hasMigrationFiles(os.DirFS(dirWithGo))",
		func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, "20260101_migration.go"), "package main")
			ok, err := hasMigrationFiles(os.DirFS(dir))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !ok {
				t.Fatalf("expected true when .go file exists")
			}
		})

	// TCID 17 — CORRECT: non-migration extensions (.txt, .md) are ignored → false
	runTC(t,
		"hasMigrationFiles ignores non-.sql non-.go files and returns false",
		"hasMigrationFiles(os.DirFS(dirWithTxt))",
		func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, "README.md"), "# readme")
			writeFile(t, filepath.Join(dir, "notes.txt"), "some notes")
			ok, err := hasMigrationFiles(os.DirFS(dir))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ok {
				t.Fatalf("expected false when only .md/.txt files present")
			}
		})

	// TCID 18 — INCORRECT: ReadDir failure propagates as error
	runTC(t,
		"hasMigrationFiles propagates ReadDir error to caller",
		"hasMigrationFiles(pgFailFS{err: boom})",
		func(t *testing.T) {
			boom := errors.New("disk read failure")
			_, err := hasMigrationFiles(pgFailFS{err: boom})
			if !errors.Is(err, boom) {
				t.Fatalf("expected wrapped %v, got %v", boom, err)
			}
		})
}

// TestGoosePGUnit_Slugify validates the slug normalisation rules.
func TestGoosePGUnit_Slugify(t *testing.T) {
	cases := []struct {
		tcPurpose string
		input     string
		want      string
	}{
		// TCID 19 — CORRECT: normal mixed-case text
		{"slugify converts mixed-case description to lowercase_underscored",
			"Add Tags Column", "add_tags_column"},
		// TCID 20 — CORRECT: multiple special characters collapse to single underscore
		{"slugify collapses consecutive special characters to one underscore",
			"  add---tags!!!column  ", "add_tags_column"},
		// TCID 21 — CORRECT: whitespace-only → fallback "migration"
		{"slugify returns 'migration' for whitespace-only input",
			"   ", "migration"},
		// TCID 22 — CORRECT: 80-char input is truncated to 60
		{"slugify truncates output to 60 characters",
			strings.Repeat("x", 80), strings.Repeat("x", 60)},
		// TCID 23 — CORRECT: leading/trailing non-alphanumeric stripped
		{"slugify strips leading and trailing underscores from result",
			"---add_column---", "add_column"},
		// TCID 24 — CORRECT: digits in description are preserved
		{"slugify preserves numeric characters in description",
			"Add 2nd Table v3", "add_2nd_table_v3"},
	}

	for _, tc := range cases {
		tc := tc
		runTC(t, tc.tcPurpose, fmt.Sprintf("slugify(%q)", tc.input), func(t *testing.T) {
			got := slugify(tc.input)
			if got != tc.want {
				t.Fatalf("slugify(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestGoosePGUnit_BuildMigrationSQL verifies the goose-annotation wrapper.
func TestGoosePGUnit_BuildMigrationSQL(t *testing.T) {
	// TCID 25 — CORRECT: Up section always present
	runTC(t,
		"buildMigrationSQL always emits a '-- +goose Up' section",
		"buildMigrationSQL(upSQL, downSQL)",
		func(t *testing.T) {
			out := buildMigrationSQL("SELECT 1;", "SELECT 2;")
			if !strings.Contains(out, "-- +goose Up") {
				t.Fatalf("missing '-- +goose Up' in output:\n%s", out)
			}
		})

	// TCID 26 — CORRECT: Down section emitted when downSQL is non-empty
	runTC(t,
		"buildMigrationSQL emits '-- +goose Down' when downSQL is non-empty",
		"buildMigrationSQL(upSQL, nonEmptyDownSQL)",
		func(t *testing.T) {
			out := buildMigrationSQL("CREATE TABLE t(id INT);", "DROP TABLE t;")
			if !strings.Contains(out, "-- +goose Down") {
				t.Fatalf("missing '-- +goose Down' in output:\n%s", out)
			}
		})

	// TCID 27 — CORRECT: Down section absent when downSQL is blank
	runTC(t,
		"buildMigrationSQL omits '-- +goose Down' when downSQL is empty/whitespace",
		"buildMigrationSQL(upSQL, \"  \")",
		func(t *testing.T) {
			out := buildMigrationSQL("SELECT 1;", "   ")
			if strings.Contains(out, "-- +goose Down") {
				t.Fatalf("unexpected Down section for empty downSQL:\n%s", out)
			}
		})

	// TCID 28 — CORRECT: SQL content is trimmed inside the StatementBegin/End block
	runTC(t,
		"buildMigrationSQL trims leading/trailing whitespace from SQL content",
		"buildMigrationSQL(\"  SELECT 1;  \", \"\")",
		func(t *testing.T) {
			out := buildMigrationSQL("  SELECT 1;  ", "")
			if strings.Contains(out, "  SELECT 1;  ") {
				t.Fatalf("expected SQL to be trimmed but found untrimmed content:\n%s", out)
			}
			if !strings.Contains(out, "SELECT 1;") {
				t.Fatalf("trimmed SQL not found in output:\n%s", out)
			}
		})
}

// TestGoosePGUnit_CreateMigration covers file-creation validation and happy path.
func TestGoosePGUnit_CreateMigration(t *testing.T) {
	logger := testLogger{}

	// TCID 29 — INCORRECT: empty MigrationsDir must be rejected
	runTC(t,
		"CreateMigration returns error when MigrationsDir is not set",
		"m.CreateMigration(desc, upSQL, downSQL) with empty cfg.MigrationsDir",
		func(t *testing.T) {
			m := &Migrator{logger: logger, cfg: GooseConfig{}}
			_, err := m.CreateMigration("add_table", "SELECT 1;", "")
			if err == nil {
				t.Fatalf("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), "MID_060221143011") {
				t.Fatalf("unexpected error %v", err)
			}
		})

	// TCID 30 — INCORRECT: empty upSQL must be rejected
	runTC(t,
		"CreateMigration returns error when upSQL is empty or whitespace",
		"m.CreateMigration(desc, \"  \", downSQL)",
		func(t *testing.T) {
			m := &Migrator{logger: logger, cfg: GooseConfig{MigrationsDir: t.TempDir()}}
			_, err := m.CreateMigration("add_table", "   ", "")
			if err == nil {
				t.Fatalf("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), "MID_060221143012") {
				t.Fatalf("unexpected error %v", err)
			}
		})

	// TCID 31 — CORRECT: valid args write a timestamped, correctly named file
	runTC(t,
		"CreateMigration writes a timestamped .sql file with correct goose annotations",
		"m.CreateMigration(\"Add Users Table\", createSQL, dropSQL)",
		func(t *testing.T) {
			dir := t.TempDir()
			m := &Migrator{logger: logger, cfg: GooseConfig{MigrationsDir: dir}}

			filename, err := m.CreateMigration("Add Users Table",
				"CREATE TABLE users (id SERIAL PRIMARY KEY);",
				"DROP TABLE users;")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// filename must match YYYYMMDDHHMMSS_add_users_table.sql
			if !strings.HasSuffix(filename, "_add_users_table.sql") {
				t.Fatalf("unexpected filename: %s", filename)
			}
			if len(filename) < 15 { // 14-digit prefix + underscore
				t.Fatalf("filename too short: %s", filename)
			}

			content, err := os.ReadFile(filepath.Join(dir, filename))
			if err != nil {
				t.Fatalf("read file: %v", err)
			}
			want := buildMigrationSQL(
				"CREATE TABLE users (id SERIAL PRIMARY KEY);",
				"DROP TABLE users;")
			if string(content) != want {
				t.Fatalf("content mismatch\nwant:\n%s\ngot:\n%s", want, string(content))
			}
		})

	// TCID 32 — CORRECT: CreateMigration without downSQL omits Down section
	runTC(t,
		"CreateMigration file has no Down section when downSQL is empty",
		"m.CreateMigration(desc, upSQL, \"\")",
		func(t *testing.T) {
			dir := t.TempDir()
			m := &Migrator{logger: logger, cfg: GooseConfig{MigrationsDir: dir}}
			filename, err := m.CreateMigration("one_way", "ALTER TABLE t ADD COLUMN c INT;", "")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			content, err := os.ReadFile(filepath.Join(dir, filename))
			if err != nil {
				t.Fatalf("read file: %v", err)
			}
			if strings.Contains(string(content), "-- +goose Down") {
				t.Fatalf("Down section should be absent when downSQL is empty")
			}
		})
}

// TestGoosePGUnit_NewWithDB validates Migrator construction without a live DB.
func TestGoosePGUnit_NewWithDB(t *testing.T) {
	logger := testLogger{}

	// TCID 33 — INCORRECT: invalid db type returns error
	runTC(t,
		"NewWithDB returns error for unsupported db type",
		"NewWithDB(nil, \"badtype\", cfg, logger)",
		func(t *testing.T) {
			_, err := NewWithDB(nil, "badtype", ApiTypes.MigrationConfig{}, logger)
			if err == nil {
				t.Fatalf("expected error for invalid db type, got nil")
			}
			if !strings.Contains(err.Error(), "unsupported database type") {
				t.Fatalf("error missing 'unsupported database type': %v", err)
			}
		})

	// TCID 34 — CORRECT: empty migrations dir produces migrator with nil provider
	runTC(t,
		"NewWithDB with empty migrations directory returns migrator with nil provider",
		"NewWithDB(&sql.DB{}, PgName, cfg{emptyDir}, logger)",
		func(t *testing.T) {
			dir := t.TempDir()
			m, err := NewWithDB(&sql.DB{}, ApiTypes.PgName,
				ApiTypes.MigrationConfig{
					MigrationsFS:  dir,
					MigrationsDir: dir,
					TableName:     "test_versions",
				}, logger)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if m == nil {
				t.Fatalf("expected non-nil migrator")
			}
			if m.provider != nil {
				t.Fatalf("expected nil provider for empty migrations dir")
			}
			if m.cfg.TableName != "test_versions" {
				t.Fatalf("TableName not preserved: got %q", m.cfg.TableName)
			}
		})
}

// TestGoosePGUnit_RebuildProvider verifies rebuildProvider behaviour with no
// migration files.
func TestGoosePGUnit_RebuildProvider(t *testing.T) {
	// TCID 35 — CORRECT: empty FS keeps provider nil after rebuild
	runTC(t,
		"rebuildProvider keeps provider nil when no migration files exist",
		"m.rebuildProvider() with empty MigrationsFS",
		func(t *testing.T) {
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
				t.Fatalf("expected nil provider when no migration files present")
			}
		})
}

// TestGoosePGUnit_CreateAndApply covers CreateAndApply without a live DB.
func TestGoosePGUnit_CreateAndApply(t *testing.T) {
	ctx := context.Background()
	logger := testLogger{}

	// TCID 36 — CORRECT: empty MigrationsFS succeeds; file created, provider nil
	runTC(t,
		"CreateAndApply succeeds when MigrationsFS dir is empty (nil provider path)",
		"m.CreateAndApply(ctx, desc, upSQL, \"\")",
		func(t *testing.T) {
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
			filename, err := m.CreateAndApply(ctx, "seed_schema", "SELECT 1;", "")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if filename == "" {
				t.Fatalf("expected non-empty filename")
			}
			if _, statErr := os.Stat(filepath.Join(writeDir, filename)); statErr != nil {
				t.Fatalf("migration file not found: %v", statErr)
			}
			if m.provider != nil {
				t.Fatalf("provider should remain nil when MigrationsFS is empty")
			}
		})

	// TCID 37 — INCORRECT: nil DB with non-empty FS triggers rebuild failure;
	//           filename must still be returned alongside the error.
	runTC(t,
		"CreateAndApply returns filename even when provider rebuild fails (nil DB)",
		"m.CreateAndApply(ctx, desc, upSQL, \"\") — nil DB, MigrationsFS = writeDir",
		func(t *testing.T) {
			dir := t.TempDir()
			m := &Migrator{
				logger:  logger,
				dialect: gooselib.DialectPostgres,
				cfg: GooseConfig{
					MigrationsDir: dir,
					MigrationsFS:  os.DirFS(dir),
					TableName:     "db_migrations",
				},
				db: nil, // forces gooselib.NewProvider to fail once file exists
			}
			filename, err := m.CreateAndApply(ctx, "will_fail", "SELECT 1;", "")
			if filename == "" {
				t.Fatalf("filename must be returned even on rebuild failure")
			}
			if err == nil || !strings.Contains(err.Error(), "failed to reload migrations") {
				t.Fatalf("expected rebuild error, got %v", err)
			}
		})
}

// TestGoosePGUnit_NilProviderNoOps checks that every public method is a safe
// no-op when the internal provider is nil.
func TestGoosePGUnit_NilProviderNoOps(t *testing.T) {
	ctx := context.Background()

	// TCID 38 — CORRECT: Up is a no-op when provider is nil
	runTC(t,
		"Up returns nil immediately when provider is nil (no migrations to apply)",
		"m.Up(ctx) with nil provider",
		func(t *testing.T) {
			m := &Migrator{logger: testLogger{}}
			if err := m.Up(ctx); err != nil {
				t.Fatalf("Up should be no-op: %v", err)
			}
		})

	// TCID 39 — CORRECT: UpByOne is a no-op
	runTC(t,
		"UpByOne returns nil immediately when provider is nil",
		"m.UpByOne(ctx) with nil provider",
		func(t *testing.T) {
			m := &Migrator{logger: testLogger{}}
			if err := m.UpByOne(ctx); err != nil {
				t.Fatalf("UpByOne should be no-op: %v", err)
			}
		})

	// TCID 40 — CORRECT: UpTo is a no-op
	runTC(t,
		"UpTo returns nil immediately when provider is nil",
		"m.UpTo(ctx, 42) with nil provider",
		func(t *testing.T) {
			m := &Migrator{logger: testLogger{}}
			if err := m.UpTo(ctx, 42); err != nil {
				t.Fatalf("UpTo should be no-op: %v", err)
			}
		})

	// TCID 41 — CORRECT: Down is a no-op
	runTC(t,
		"Down returns nil immediately when provider is nil",
		"m.Down(ctx) with nil provider",
		func(t *testing.T) {
			m := &Migrator{logger: testLogger{}}
			if err := m.Down(ctx); err != nil {
				t.Fatalf("Down should be no-op: %v", err)
			}
		})

	// TCID 42 — CORRECT: DownTo is a no-op
	runTC(t,
		"DownTo returns nil immediately when provider is nil",
		"m.DownTo(ctx, 0) with nil provider",
		func(t *testing.T) {
			m := &Migrator{logger: testLogger{}}
			if err := m.DownTo(ctx, 0); err != nil {
				t.Fatalf("DownTo should be no-op: %v", err)
			}
		})

	// TCID 43 — CORRECT: Status returns (nil, nil)
	runTC(t,
		"Status returns (nil, nil) when provider is nil",
		"m.Status(ctx) with nil provider",
		func(t *testing.T) {
			m := &Migrator{logger: testLogger{}}
			statuses, err := m.Status(ctx)
			if err != nil {
				t.Fatalf("Status should be no-op: %v", err)
			}
			if statuses != nil {
				t.Fatalf("expected nil statuses, got %v", statuses)
			}
		})

	// TCID 44 — CORRECT: GetVersion returns (0, nil)
	runTC(t,
		"GetVersion returns (0, nil) when provider is nil",
		"m.GetVersion(ctx) with nil provider",
		func(t *testing.T) {
			m := &Migrator{logger: testLogger{}}
			v, err := m.GetVersion(ctx)
			if err != nil {
				t.Fatalf("GetVersion should be no-op: %v", err)
			}
			if v != 0 {
				t.Fatalf("expected version 0, got %d", v)
			}
		})

	// TCID 45 — CORRECT: HasPending returns (false, nil)
	runTC(t,
		"HasPending returns (false, nil) when provider is nil",
		"m.HasPending(ctx) with nil provider",
		func(t *testing.T) {
			m := &Migrator{logger: testLogger{}}
			pending, err := m.HasPending(ctx)
			if err != nil {
				t.Fatalf("HasPending should be no-op: %v", err)
			}
			if pending {
				t.Fatalf("expected pending=false, got true")
			}
		})

	// TCID 46 — CORRECT: ListSources returns nil
	runTC(t,
		"ListSources returns nil when provider is nil",
		"m.ListSources() with nil provider",
		func(t *testing.T) {
			m := &Migrator{logger: testLogger{}}
			if sources := m.ListSources(); sources != nil {
				t.Fatalf("expected nil sources, got %v", sources)
			}
		})
}

// TestGoosePGUnit_RunMigrations tests the RunMigrations helper and the three
// named singleton wrappers.
func TestGoosePGUnit_RunMigrations(t *testing.T) {
	logger := testLogger{}
	ctx := context.Background()

	// TCID 47 — INCORRECT: MigrationsDir pointing to an existing file → mkdir error
	runTC(t,
		"RunMigrations returns error when MigrationsDir points to an existing file",
		"RunMigrations with MigrationsDir = path-to-a-regular-file",
		func(t *testing.T) {
			base := t.TempDir()
			filePath := filepath.Join(base, "not-a-dir")
			writeFile(t, filePath, "x")

			_, err := RunMigrations(ctx, logger, "test",
				ApiTypes.MigrationConfig{MigrationsDir: filePath}, &sql.DB{})
			if err == nil || !strings.Contains(err.Error(), "MID_060221143005") {
				t.Fatalf("expected mkdir error MID_060221143005, got %v", err)
			}
		})

	// TCID 48 — CORRECT: empty migrations dir succeeds; nil provider returned
	runTC(t,
		"RunMigrations succeeds for empty migrations directory with valid DBType",
		"RunMigrations(ctx, logger, name, cfg{emptyDir}, db)",
		func(t *testing.T) {
			old := ApiTypes.DBType
			ApiTypes.DBType = ApiTypes.PgName
			t.Cleanup(func() { ApiTypes.DBType = old })

			dir := t.TempDir()
			m, err := RunMigrations(ctx, logger, "pgtest",
				ApiTypes.MigrationConfig{MigrationsDir: dir, MigrationsFS: dir},
				&sql.DB{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if m == nil {
				t.Fatalf("expected non-nil migrator")
			}
			if m.provider != nil {
				t.Fatalf("expected nil provider for empty dir")
			}
		})
}

// TestGoosePGUnit_WrapperInitializers covers RunProjectMigrations,
// RunSharedMigrations, and RunAutoTesterMigrations guard rails.
func TestGoosePGUnit_WrapperInitializers(t *testing.T) {
	logger := testLogger{}
	ctx := context.Background()

	// Save and reset all global migrator state for this test.
	oldProject, oldShared, oldAuto := ProjectMigrator, SharedMigrator, AutoTesterMigrator
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

	dir := t.TempDir()
	cfg := ApiTypes.MigrationConfig{MigrationsDir: dir, MigrationsFS: dir}

	// TCID 49 — INCORRECT: RunProjectMigrations with nil DB returns error
	runTC(t,
		"RunProjectMigrations returns error when DB is nil",
		"RunProjectMigrations(ctx, logger, cfg, nil)",
		func(t *testing.T) {
			if err := RunProjectMigrations(ctx, logger, cfg, nil); err == nil ||
				!strings.Contains(err.Error(), "MID_060221143035") {
				t.Fatalf("expected nil-DB error MID_060221143035, got %v", err)
			}
		})

	// TCID 50 — INCORRECT: RunSharedMigrations with nil DB returns error
	runTC(t,
		"RunSharedMigrations returns error when DB is nil",
		"RunSharedMigrations(ctx, logger, cfg, nil)",
		func(t *testing.T) {
			if err := RunSharedMigrations(ctx, logger, cfg, nil); err == nil ||
				!strings.Contains(err.Error(), "MID_060221143002") {
				t.Fatalf("expected nil-DB error MID_060221143002, got %v", err)
			}
		})

	// TCID 51 — INCORRECT: RunAutoTesterMigrations with nil DB returns error
	runTC(t,
		"RunAutoTesterMigrations returns error when DB is nil",
		"RunAutoTesterMigrations(ctx, logger, cfg, nil)",
		func(t *testing.T) {
			if err := RunAutoTesterMigrations(ctx, logger, cfg, nil); err == nil ||
				!strings.Contains(err.Error(), "MID_060221143012") {
				t.Fatalf("expected nil-DB error MID_060221143012, got %v", err)
			}
		})

	// TCID 52 — CORRECT: RunProjectMigrations succeeds first time
	runTC(t,
		"RunProjectMigrations initialises ProjectMigrator successfully on first call",
		"RunProjectMigrations(ctx, logger, cfg, &sql.DB{})",
		func(t *testing.T) {
			if err := RunProjectMigrations(ctx, logger, cfg, &sql.DB{}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ProjectMigrator == nil {
				t.Fatalf("ProjectMigrator should be non-nil after init")
			}
		})

	// TCID 53 — INCORRECT: second call to RunProjectMigrations returns "already initialized"
	runTC(t,
		"RunProjectMigrations returns 'already initialized' error on second call",
		"RunProjectMigrations(ctx, logger, cfg, &sql.DB{}) — second call",
		func(t *testing.T) {
			if err := RunProjectMigrations(ctx, logger, cfg, &sql.DB{}); err == nil ||
				!strings.Contains(err.Error(), "MID_060221143034") {
				t.Fatalf("expected already-initialized error MID_060221143034, got %v", err)
			}
		})

	// TCID 54 — CORRECT: RunSharedMigrations succeeds first time
	runTC(t,
		"RunSharedMigrations initialises SharedMigrator successfully on first call",
		"RunSharedMigrations(ctx, logger, cfg, &sql.DB{})",
		func(t *testing.T) {
			if err := RunSharedMigrations(ctx, logger, cfg, &sql.DB{}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})

	// TCID 55 — INCORRECT: second call to RunSharedMigrations is rejected
	runTC(t,
		"RunSharedMigrations returns 'already initialized' error on second call",
		"RunSharedMigrations(ctx, logger, cfg, &sql.DB{}) — second call",
		func(t *testing.T) {
			if err := RunSharedMigrations(ctx, logger, cfg, &sql.DB{}); err == nil ||
				!strings.Contains(err.Error(), "MID_060221143001") {
				t.Fatalf("expected already-initialized error MID_060221143001, got %v", err)
			}
		})

	// TCID 56 — CORRECT: RunAutoTesterMigrations succeeds first time
	runTC(t,
		"RunAutoTesterMigrations initialises AutoTesterMigrator successfully on first call",
		"RunAutoTesterMigrations(ctx, logger, cfg, &sql.DB{})",
		func(t *testing.T) {
			if err := RunAutoTesterMigrations(ctx, logger, cfg, &sql.DB{}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})

	// TCID 57 — INCORRECT: second call to RunAutoTesterMigrations is rejected
	runTC(t,
		"RunAutoTesterMigrations returns 'already initialized' error on second call",
		"RunAutoTesterMigrations(ctx, logger, cfg, &sql.DB{}) — second call",
		func(t *testing.T) {
			if err := RunAutoTesterMigrations(ctx, logger, cfg, &sql.DB{}); err == nil ||
				!strings.Contains(err.Error(), "MID_060221143011") {
				t.Fatalf("expected already-initialized error MID_060221143011, got %v", err)
			}
		})
}

// ---------------------------------------------------------------------------
// ══════════════════════════════════════════════════════════════════════════
// POSTGRESQL INTEGRATION TESTS — skipped when PGTEST_DSN / -pg-dsn absent
// ══════════════════════════════════════════════════════════════════════════
// ---------------------------------------------------------------------------

// requirePGDB skips the test if no live PostgreSQL connection is available.
func requirePGDB(t *testing.T) *sql.DB {
	t.Helper()
	if pgTestDB == nil {
		t.Skip("PostgreSQL integration test skipped (set PGTEST_DSN or -pg-dsn)")
	}
	return pgTestDB
}

// TestGoosePGIntegration_NewWithDB verifies Migrator creation against a live
// PostgreSQL instance with an empty migrations directory.
func TestGoosePGIntegration_NewWithDB(t *testing.T) {
	db := requirePGDB(t)
	ctx := context.Background()
	_ = ctx

	// TCID 58 — CORRECT: NewWithDB with real PG + empty dir → nil provider
	runTC(t,
		"NewWithDB with real PostgreSQL and empty migrations dir returns nil provider",
		"NewWithDB(pgDB, PgName, cfg{emptyDir}, logger)",
		func(t *testing.T) {
			dir := t.TempDir()
			m, err := NewWithDB(db, ApiTypes.PgName,
				ApiTypes.MigrationConfig{
					MigrationsFS:  dir,
					MigrationsDir: dir,
					TableName:     uniqueTableName("new_pg"),
				}, testLogger{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if m == nil {
				t.Fatalf("expected non-nil migrator")
			}
			if m.provider != nil {
				t.Fatalf("expected nil provider for empty dir")
			}
		})
}

// TestGoosePGIntegration_UpDownCycle exercises a full Up → Status →
// GetVersion → HasPending → Down cycle using a real PostgreSQL database.
func TestGoosePGIntegration_UpDownCycle(t *testing.T) {
	db := requirePGDB(t)
	ctx := context.Background()

	dir := t.TempDir()
	tableName := uniqueTableName("pg_cycle")

	// Write a migration file directly (bypassing CreateMigration so the FS
	// and the write directory are the same path).
	migFile := filepath.Join(dir, "20260101000001_create_pg_test_tbl.sql")
	migContent := buildMigrationSQL(
		"CREATE TABLE IF NOT EXISTS pg_test_tbl_cycle (id SERIAL PRIMARY KEY, label TEXT);",
		"DROP TABLE IF EXISTS pg_test_tbl_cycle;",
	)
	writeFile(t, migFile, migContent)

	m, err := NewWithDB(db, ApiTypes.PgName,
		ApiTypes.MigrationConfig{
			MigrationsFS:  dir,
			MigrationsDir: dir,
			TableName:     tableName,
		}, testLogger{})
	if err != nil {
		t.Fatalf("NewWithDB: %v", err)
	}
	t.Cleanup(func() {
		cleanupPGTable(db, tableName)
		cleanupPGTable(db, "pg_test_tbl_cycle")
	})

	// TCID 59 — CORRECT: Up applies migration successfully
	runTC(t,
		"Up applies pending migrations against a real PostgreSQL database",
		"m.Up(ctx) — one pending migration",
		func(t *testing.T) {
			if err := m.Up(ctx); err != nil {
				t.Fatalf("Up failed: %v", err)
			}
		})

	// TCID 60 — CORRECT: HasPending returns false after all migrations applied
	runTC(t,
		"HasPending returns false immediately after all migrations are applied",
		"m.HasPending(ctx)",
		func(t *testing.T) {
			pending, err := m.HasPending(ctx)
			if err != nil {
				t.Fatalf("HasPending: %v", err)
			}
			if pending {
				t.Fatalf("expected HasPending=false after Up, got true")
			}
		})

	// TCID 61 — CORRECT: GetVersion returns the applied migration version
	runTC(t,
		"GetVersion returns the version number of the applied migration",
		"m.GetVersion(ctx)",
		func(t *testing.T) {
			v, err := m.GetVersion(ctx)
			if err != nil {
				t.Fatalf("GetVersion: %v", err)
			}
			if v <= 0 {
				t.Fatalf("expected version > 0, got %d", v)
			}
		})

	// TCID 62 — CORRECT: Status returns one applied migration record
	runTC(t,
		"Status returns exactly one migration entry with Applied=true",
		"m.Status(ctx)",
		func(t *testing.T) {
			statuses, err := m.Status(ctx)
			if err != nil {
				t.Fatalf("Status: %v", err)
			}
			if len(statuses) != 1 {
				t.Fatalf("expected 1 status entry, got %d", len(statuses))
			}
			if statuses[0].State != gooselib.StateApplied {
				t.Fatalf("expected applied state, got %v", statuses[0].State)
			}
		})

	// TCID 63 — CORRECT: Down rolls back the applied migration
	runTC(t,
		"Down rolls back the last applied migration",
		"m.Down(ctx)",
		func(t *testing.T) {
			if err := m.Down(ctx); err != nil {
				t.Fatalf("Down failed: %v", err)
			}
		})

	// TCID 64 — CORRECT: HasPending returns true after Down
	runTC(t,
		"HasPending returns true after Down rolls back the migration",
		"m.HasPending(ctx) — after Down",
		func(t *testing.T) {
			pending, err := m.HasPending(ctx)
			if err != nil {
				t.Fatalf("HasPending: %v", err)
			}
			if !pending {
				t.Fatalf("expected HasPending=true after Down, got false")
			}
		})
}

// TestGoosePGIntegration_CreateAndApply verifies the combined create-and-apply
// flow against a live PostgreSQL database.
func TestGoosePGIntegration_CreateAndApply(t *testing.T) {
	db := requirePGDB(t)
	ctx := context.Background()

	dir := t.TempDir()
	tableName := uniqueTableName("pg_caa")

	m, err := NewWithDB(db, ApiTypes.PgName,
		ApiTypes.MigrationConfig{
			MigrationsFS:  dir,
			MigrationsDir: dir,
			TableName:     tableName,
		}, testLogger{})
	if err != nil {
		t.Fatalf("NewWithDB: %v", err)
	}
	t.Cleanup(func() {
		cleanupPGTable(db, tableName)
		cleanupPGTable(db, "pg_test_tbl_caa")
	})

	var appliedFilename string

	// TCID 65 — CORRECT: CreateAndApply writes file and applies migration
	runTC(t,
		"CreateAndApply creates migration file and applies it to PostgreSQL",
		"m.CreateAndApply(ctx, desc, createSQL, dropSQL)",
		func(t *testing.T) {
			var err error
			appliedFilename, err = m.CreateAndApply(ctx, "create_pg_test_tbl_caa",
				"CREATE TABLE IF NOT EXISTS pg_test_tbl_caa (id SERIAL PRIMARY KEY);",
				"DROP TABLE IF EXISTS pg_test_tbl_caa;",
			)
			if err != nil {
				t.Fatalf("CreateAndApply: %v", err)
			}
			if appliedFilename == "" {
				t.Fatalf("expected non-empty filename")
			}
		})

	// TCID 66 — CORRECT: migration file exists on disk after CreateAndApply
	runTC(t,
		"CreateAndApply leaves migration file on disk",
		fmt.Sprintf("os.Stat(filepath.Join(dir, %q))", appliedFilename),
		func(t *testing.T) {
			if appliedFilename == "" {
				t.Skip("previous TCID did not produce a filename")
			}
			if _, err := os.Stat(filepath.Join(dir, appliedFilename)); err != nil {
				t.Fatalf("migration file not found on disk: %v", err)
			}
		})

	// TCID 67 — CORRECT: provider is non-nil after CreateAndApply creates files
	runTC(t,
		"CreateAndApply rebuilds provider so it is non-nil after application",
		"m.provider != nil",
		func(t *testing.T) {
			if m.provider == nil {
				t.Fatalf("expected non-nil provider after CreateAndApply")
			}
		})

	// TCID 68 — CORRECT: table was actually created in PostgreSQL
	runTC(t,
		"Applied migration creates target table in PostgreSQL",
		"SELECT 1 FROM pg_test_tbl_caa LIMIT 1",
		func(t *testing.T) {
			row := db.QueryRow("SELECT 1 FROM pg_test_tbl_caa LIMIT 1")
			if err := row.Scan(new(int)); err != nil && err != sql.ErrNoRows {
				t.Fatalf("table pg_test_tbl_caa does not appear to exist: %v", err)
			}
		})

	// TCID 69 — INCORRECT: duplicate CreateAndApply for the same table is
	//           idempotent only because of IF NOT EXISTS; a genuinely conflicting
	//           migration (without IF NOT EXISTS) should surface an error.
	runTC(t,
		"CreateAndApply with conflicting SQL surfaces PostgreSQL error at apply time",
		"m.CreateAndApply(ctx, desc, 'CREATE TABLE pg_test_tbl_caa', '') — no IF NOT EXISTS",
		func(t *testing.T) {
			_, err := m.CreateAndApply(ctx, "dup_table_no_ifnotexists",
				"CREATE TABLE pg_test_tbl_caa (id SERIAL PRIMARY KEY);",
				"",
			)
			if err == nil {
				t.Fatalf("expected error for duplicate CREATE TABLE without IF NOT EXISTS, got nil")
			}
		})
}

// TestGoosePGIntegration_ListSources verifies ListSources with a real provider.
func TestGoosePGIntegration_ListSources(t *testing.T) {
	db := requirePGDB(t)
	ctx := context.Background()
	_ = ctx

	dir := t.TempDir()
	tableName := uniqueTableName("pg_ls")

	// Seed two migration files.
	writeFile(t, filepath.Join(dir, "20260101000001_first.sql"),
		buildMigrationSQL("CREATE TABLE IF NOT EXISTS pg_ls_a (id INT);",
			"DROP TABLE IF EXISTS pg_ls_a;"))
	writeFile(t, filepath.Join(dir, "20260101000002_second.sql"),
		buildMigrationSQL("CREATE TABLE IF NOT EXISTS pg_ls_b (id INT);",
			"DROP TABLE IF EXISTS pg_ls_b;"))

	m, err := NewWithDB(db, ApiTypes.PgName,
		ApiTypes.MigrationConfig{
			MigrationsFS:  dir,
			MigrationsDir: dir,
			TableName:     tableName,
		}, testLogger{})
	if err != nil {
		t.Fatalf("NewWithDB: %v", err)
	}
	t.Cleanup(func() {
		m.DownTo(context.Background(), 0) //nolint:errcheck
		cleanupPGTable(db, tableName)
		cleanupPGTable(db, "pg_ls_a")
		cleanupPGTable(db, "pg_ls_b")
	})

	// TCID 70 — CORRECT: ListSources returns both migration sources
	runTC(t,
		"ListSources returns one entry per migration file in ascending order",
		"m.ListSources()",
		func(t *testing.T) {
			sources := m.ListSources()
			if len(sources) != 2 {
				t.Fatalf("expected 2 sources, got %d", len(sources))
			}
			if sources[0].Version >= sources[1].Version {
				t.Fatalf("sources not in ascending order: %d >= %d",
					sources[0].Version, sources[1].Version)
			}
		})

	// TCID 71 — CORRECT: UpTo applies only up to the specified version
	runTC(t,
		"UpTo applies only migrations up to and including the given version number",
		"m.UpTo(ctx, sources[0].Version)",
		func(t *testing.T) {
			sources := m.ListSources()
			if len(sources) == 0 {
				t.Skip("no sources available")
			}
			if err := m.UpTo(ctx, sources[0].Version); err != nil {
				t.Fatalf("UpTo(%d): %v", sources[0].Version, err)
			}
			v, err := m.GetVersion(ctx)
			if err != nil {
				t.Fatalf("GetVersion: %v", err)
			}
			if v != sources[0].Version {
				t.Fatalf("expected version %d, got %d", sources[0].Version, v)
			}
		})
}

// ---------------------------------------------------------------------------
// Utilities
// ---------------------------------------------------------------------------

// writeFile is a test helper that writes content to path, failing the test on error.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeFile(%s): %v", path, err)
	}
}

// uniqueTableName generates a short unique migration-tracking table name safe
// for use within a single test run.
func uniqueTableName(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano()%1_000_000)
}

// cleanupPGTable drops a table (or migration-version tracking table) if it
// exists, silently ignoring errors (best-effort cleanup).
func cleanupPGTable(db *sql.DB, name string) {
	db.Exec("DROP TABLE IF EXISTS " + name)        //nolint:errcheck
	db.Exec("DROP TABLE IF EXISTS public." + name) //nolint:errcheck
}
