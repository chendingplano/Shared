package autotesters

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/databaseutil"
	"github.com/lib/pq"
)

// DBPersistence handles database operations for AutoTester.
type DBPersistence struct {
	db *sql.DB
}

// NewDBPersistence creates a new DBPersistence instance.
func NewDBPersistence(db *sql.DB) *DBPersistence {
	return &DBPersistence{db: db}
}

// CreateRunRecord inserts a new auto_test_runs record.
func (p *DBPersistence) CreateRunRecord(ctx context.Context, run *TestRun, tableName string) error {
	configJSON, err := json.Marshal(run.Config)
	if err != nil {
		return fmt.Errorf("marshal config (MID_060222143044): %w", err)
	}

	envJSON, err := json.Marshal(run.EnvMetadata)
	if err != nil {
		return fmt.Errorf("failed marshal env (MID_060222143045): %w", err)
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (
			run_id, started_at, status, env, seed,
			config_json, env_json, total, passed, failed, skipped, errored
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, tableName)

	_, err = p.db.ExecContext(ctx, query,
		run.ID,
		run.StartedAt,
		run.Status,
		run.Environment,
		run.Seed,
		configJSON,
		envJSON,
		run.Total,
		run.Passed,
		run.Failed,
		run.Skipped,
		run.Errored,
	)

	if err != nil {
		return fmt.Errorf("failed inserting run record (MID_060222143145): %w", err)
	}
	return nil
}

// UpdateRunRecord updates an existing auto_test_runs record.
func (p *DBPersistence) UpdateRunRecord(ctx context.Context, run *TestRun, tableName string) error {
	query := fmt.Sprintf(`
		UPDATE %s SET
			ended_at = $1,
			status = $2,
			total = $3,
			passed = $4,
			failed = $5,
			skipped = $6,
			errored = $7,
			duration_ms = $8,
			report_path = $9,
			updated_at = NOW()
		WHERE run_id = $10
	`, tableName)

	_, err := p.db.ExecContext(ctx, query,
		run.EndedAt,
		run.Status,
		run.Total,
		run.Passed,
		run.Failed,
		run.Skipped,
		run.Errored,
		run.DurationMs,
		run.ReportPath,
		run.ID,
	)
	if err != nil {
		return fmt.Errorf("failed updating the run record (MID_060222132401): %w", err)
	}
	return nil
}

// InsertTestResult inserts a new auto_test_results record.
func (p *DBPersistence) InsertTestResult(ctx context.Context, result *TestResult, tableName string) error {
	actualValueJSON, err := json.Marshal(result.ActualValue)
	if err != nil {
		actualValueJSON = []byte("null")
	}

	sideEffects := pqStringArray(result.SideEffectsObserved)

	// Join error messages into a single string for database storage
	errorStr := ""
	if len(result.ErrorMsgs) > 0 {
		errorStr = strings.Join(result.ErrorMsgs, "; ")
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (
			run_id, test_case_id, tester_name, status, message, error,
			start_time, end_time, duration_ms, retry_count,
			actual_value_json, side_effects
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, tableName)

	_, err = p.db.ExecContext(ctx, query,
		result.RunID,
		result.TestCaseID,
		result.TesterName,
		result.Status,
		result.Message,
		errorStr,
		result.StartTime,
		result.EndTime,
		result.Duration.Milliseconds(),
		result.RetryCount,
		actualValueJSON,
		sideEffects,
	)
	if err != nil {
		return fmt.Errorf("failed inserting the record (MID_060222132400): %w", err)
	}
	return nil
}

// InsertTestLog inserts a new auto_test_logs record.
func (p *DBPersistence) InsertTestLog(ctx context.Context, runID, testCaseID, testerName, level, message string, contextMap map[string]interface{}, tableName string) error {
	contextJSON, err := json.Marshal(contextMap)
	if err != nil {
		contextJSON = []byte("{}")
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (run_id, test_case_id, tester_name, log_level, message, context_json)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, tableName)

	_, err = p.db.ExecContext(ctx, query,
		runID,
		testCaseID,
		testerName,
		level,
		message,
		contextJSON,
	)
	if err != nil {
		return fmt.Errorf("failed inserting the log record (MID_060222132402): %w", err)
	}
	return nil
}

// InsertTestLogs bulk inserts log entries for a test result.
func (p *DBPersistence) InsertTestLogs(ctx context.Context, result *TestResult, tableName string) error {
	for _, log := range result.Logs {
		if err := p.InsertTestLog(ctx, result.RunID, result.TestCaseID, result.TesterName, log.Level, log.Message, log.Context, tableName); err != nil {
			return fmt.Errorf("failed inserting test logs (MID_260222132403): %w", err)
		}
	}
	return nil
}

// GetRunRecord retrieves an auto_test_runs record by run_id.
func (p *DBPersistence) GetRunRecord(ctx context.Context, runID, tableName string) (*TestRun, error) {
	query := fmt.Sprintf(`
		SELECT run_id, started_at, ended_at, status, env, seed,
		       config_json, env_json, total, passed, failed, skipped, errored, duration_ms, report_path
		FROM %s WHERE run_id = $1
	`, tableName)

	row := p.db.QueryRowContext(ctx, query, runID)

	var run TestRun
	var configJSON, envJSON []byte

	err := row.Scan(
		&run.ID,
		&run.StartedAt,
		&run.EndedAt,
		&run.Status,
		&run.Environment,
		&run.Seed,
		&configJSON,
		&envJSON,
		&run.Total,
		&run.Passed,
		&run.Failed,
		&run.Skipped,
		&run.Errored,
		&run.DurationMs,
		&run.ReportPath,
	)
	if err != nil {
		return nil, fmt.Errorf("failed scanning run record (MID_260222132404): %w", err)
	}

	if err := json.Unmarshal(configJSON, &run.Config); err != nil {
		return nil, fmt.Errorf("unmarshal config (MID_060222143046): %w", err)
	}
	if err := json.Unmarshal(envJSON, &run.EnvMetadata); err != nil {
		return nil, fmt.Errorf("unmarshal env (MID_060222143047): %w", err)
	}

	return &run, nil
}

// GetTestResultsByRun retrieves all test results for a given run_id.
func (p *DBPersistence) GetTestResultsByRun(ctx context.Context, runID, tableName string) ([]TestResult, error) {
	query := fmt.Sprintf(`
		SELECT run_id, test_case_id, tester_name, status, message, error,
		       start_time, end_time, duration_ms, retry_count,
		       actual_value_json, side_effects
		FROM %s WHERE run_id = $1 ORDER BY start_time
	`, tableName)

	rows, err := p.db.QueryContext(ctx, query, runID)
	if err != nil {
		return nil, fmt.Errorf("failed querying test results (MID_260222132405): %w", err)
	}
	defer rows.Close()

	var results []TestResult
	for rows.Next() {
		var r TestResult
		var actualValueJSON []byte
		var sideEffects []string
		var errorStr string

		err := rows.Scan(
			&r.RunID,
			&r.TestCaseID,
			&r.TesterName,
			&r.Status,
			&r.Message,
			&errorStr,
			&r.StartTime,
			&r.EndTime,
			&r.Duration,
			&r.RetryCount,
			&actualValueJSON,
			pqArray(&sideEffects),
		)
		if err != nil {
			return nil, fmt.Errorf("failed scanning test result (MID_260222132406): %w", err)
		}

		r.Duration = time.Duration(r.Duration) * time.Millisecond
		if err := json.Unmarshal(actualValueJSON, &r.ActualValue); err != nil {
			r.ActualValue = nil
		}
		r.SideEffectsObserved = sideEffects
		// Split error string back into ErrorMsgs
		if errorStr != "" {
			r.ErrorMsgs = strings.Split(errorStr, "; ")
		}

		results = append(results, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed iterating test results (MID_260222132407): %w", err)
	}
	return results, nil
}

// pqStringArray converts a Go string slice to PostgreSQL text[] format.
func pqStringArray(items []string) interface{} {
	if items == nil {
		return pq.Array([]string{})
	}
	return pq.Array(items)
}

// pqArray is a helper for scanning PostgreSQL array columns.
func pqArray(dest *[]string) interface{} {
	return (*dest)
}

// CreateAutoTestTables creates all three auto-test tables in the given database.
// Call this during autotester startup using the project's dedicated autotester DB
// (ApiTypes.PG_DB_AutoTester), not the main project DB. Each project that uses
// AutoTester should have its own autotester DB so test data stays isolated.
func CreateAutoTestTables(logger ApiTypes.JimoLogger, db *sql.DB, dbType string) error {
	runsTable := ApiTypes.LibConfig.SystemTableNames.TableNameAutoTestRuns
	if runsTable == "" {
		runsTable = "auto_test_runs"
	}
	resultsTable := ApiTypes.LibConfig.SystemTableNames.TableNameAutoTestResults
	if resultsTable == "" {
		resultsTable = "auto_test_results"
	}
	logsTable := ApiTypes.LibConfig.SystemTableNames.TableNameAutoTestLogs
	if logsTable == "" {
		logsTable = "auto_test_logs"
	}

	if err := CreateAutoTestRunsTable(logger, db, dbType, runsTable); err != nil {
		return fmt.Errorf("failed creating auto test tables (runs) (MID_260222132408): %w", err)
	}
	if err := CreateAutoTestResultsTable(logger, db, dbType, resultsTable); err != nil {
		return fmt.Errorf("failed creating auto test tables (results) (MID_260222132409): %w", err)
	}
	if err := CreateAutoTestLogsTable(logger, db, dbType, logsTable); err != nil {
		return fmt.Errorf("failed creating auto test tables (logs) (MID_260222132410): %w", err)
	}
	return nil
}

// CreateAutoTestRunsTable creates the auto_test_runs table if it does not exist.
func CreateAutoTestRunsTable(logger ApiTypes.JimoLogger, db *sql.DB, dbType string, tableName string) error {
	logger.Info("Create table", "table_name", tableName)
	stmt := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		id          BIGSERIAL PRIMARY KEY,
		run_id      VARCHAR(64)  NOT NULL UNIQUE,
		started_at  TIMESTAMPTZ  NOT NULL,
		ended_at    TIMESTAMPTZ,
		status      VARCHAR(20)  NOT NULL DEFAULT 'running'
		                CHECK (status IN ('running','completed','failed','partial')),
		env         VARCHAR(40)  NOT NULL DEFAULT 'local',
		seed        BIGINT       NOT NULL DEFAULT 0,
		config_json JSONB,
		env_json    JSONB,
		total       INTEGER      NOT NULL DEFAULT 0,
		passed      INTEGER      NOT NULL DEFAULT 0,
		failed      INTEGER      NOT NULL DEFAULT 0,
		skipped     INTEGER      NOT NULL DEFAULT 0,
		errored     INTEGER      NOT NULL DEFAULT 0,
		duration_ms BIGINT,
		report_path VARCHAR(512),
		created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
		updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
	)`, tableName)
	if err := databaseutil.ExecuteStatement(db, stmt); err != nil {
		return fmt.Errorf("failed creating table %s (MID_060222143200): %w", tableName, err)
	}

	for _, idx := range []string{
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_atr_started_at ON %s (started_at DESC)`, tableName),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_atr_status ON %s (status)`, tableName),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_atr_env ON %s (env)`, tableName),
	} {
		databaseutil.ExecuteStatement(db, idx)
	}

	logger.Info("Create table success", "table_name", tableName)
	return nil
}

// CreateAutoTestResultsTable creates the auto_test_results table if it does not exist.
func CreateAutoTestResultsTable(logger ApiTypes.JimoLogger, db *sql.DB, dbType string, tableName string) error {
	logger.Info("Create table", "table_name", tableName)
	stmt := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		id                   BIGSERIAL    PRIMARY KEY,
		run_id               VARCHAR(64)  NOT NULL,
		test_case_id         VARCHAR(200) NOT NULL,
		tester_name          VARCHAR(128) NOT NULL,
		status               VARCHAR(20)  NOT NULL
		                         CHECK (status IN ('pass','fail','skip','error')),
		message              TEXT,
		error                TEXT,
		start_time           TIMESTAMPTZ  NOT NULL,
		end_time             TIMESTAMPTZ  NOT NULL,
		duration_ms          BIGINT       NOT NULL,
		retry_count          INTEGER      NOT NULL DEFAULT 0,
		actual_value_json    JSONB,
		side_effects         TEXT[],
		created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
		CONSTRAINT fk_auto_test_results_run
		    FOREIGN KEY (run_id) REFERENCES auto_test_runs(run_id) ON DELETE CASCADE
	)`, tableName)
	if err := databaseutil.ExecuteStatement(db, stmt); err != nil {
		return fmt.Errorf("failed creating table %s (MID_060222143201): %w", tableName, err)
	}

	for _, idx := range []string{
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_atres_run_id ON %s (run_id)`, tableName),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_atres_tester ON %s (tester_name)`, tableName),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_atres_status ON %s (status)`, tableName),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_atres_case_id ON %s (test_case_id)`, tableName),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_atres_start_time ON %s (start_time DESC)`, tableName),
	} {
		databaseutil.ExecuteStatement(db, idx)
	}

	logger.Info("Create table success", "table_name", tableName)
	return nil
}

// CreateAutoTestLogsTable creates the auto_test_logs table if it does not exist.
func CreateAutoTestLogsTable(logger ApiTypes.JimoLogger, db *sql.DB, dbType string, tableName string) error {
	logger.Info("Create table", "table_name", tableName)
	stmt := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		id           BIGSERIAL    PRIMARY KEY,
		run_id       VARCHAR(64)  NOT NULL,
		test_case_id VARCHAR(200),
		tester_name  VARCHAR(128) NOT NULL,
		log_level    VARCHAR(10)  NOT NULL CHECK (log_level IN ('DEBUG','INFO','WARN','ERROR')),
		message      TEXT         NOT NULL,
		context_json JSONB,
		logged_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
		CONSTRAINT fk_auto_test_logs_run
		    FOREIGN KEY (run_id) REFERENCES auto_test_runs(run_id) ON DELETE CASCADE
	)`, tableName)
	if err := databaseutil.ExecuteStatement(db, stmt); err != nil {
		return fmt.Errorf("failed creating table %s (MID_060222143202): %w", tableName, err)
	}

	for _, idx := range []string{
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_atlog_run_id ON %s (run_id)`, tableName),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_atlog_case_id ON %s (test_case_id)`, tableName),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_atlog_level ON %s (log_level)`, tableName),
	} {
		databaseutil.ExecuteStatement(db, idx)
	}

	logger.Info("Create table success", "table_name", tableName)
	return nil
}
