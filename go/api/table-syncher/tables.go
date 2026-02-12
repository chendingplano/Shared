package tablesyncher

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Location codes for table operations
const (
	LOC_TBL_SCHEMA = "SHD_SYN_050"
	LOC_TBL_ADD    = "SHD_SYN_051"
	LOC_TBL_REMOVE = "SHD_SYN_052"
	LOC_TBL_LIST   = "SHD_SYN_053"
	LOC_TBL_CLEAR  = "SHD_SYN_054"
)

// SQL statements for creating the sync tables
const (
	createSyncLogsTable = `
CREATE TABLE IF NOT EXISTS data_sync_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    table_name TEXT NOT NULL,
    status TEXT NOT NULL,
    rows_synced INT DEFAULT 0,
    archive_ref TEXT,
    error_detail TEXT,
    sync_time TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_sync_logs_table_time ON data_sync_logs(table_name, sync_time);
CREATE INDEX IF NOT EXISTS idx_sync_logs_status ON data_sync_logs(status);
`

	createSyncMetricsTable = `
CREATE TABLE IF NOT EXISTS data_sync_metrics (
    id SERIAL PRIMARY KEY,
    table_name TEXT NOT NULL,
    period_start TIMESTAMPTZ NOT NULL,
    period_end TIMESTAMPTZ NOT NULL,
    period_type TEXT NOT NULL,
    records_added BIGINT DEFAULT 0,
    records_updated BIGINT DEFAULT 0,
    records_deleted BIGINT DEFAULT 0,
    UNIQUE(table_name, period_start, period_type)
);
CREATE INDEX IF NOT EXISTS idx_sync_metrics_table_period ON data_sync_metrics(table_name, period_start);
`

	createTablesToSyncTable = `
CREATE TABLE IF NOT EXISTS tables_to_sync (
    id SERIAL PRIMARY KEY,
    table_name TEXT NOT NULL,
    creator TEXT DEFAULT NULL,
    created_at TIMESTAMPTZ DEFAULT now(),
    UNIQUE(table_name)
);
`
)

// EnsureTables creates the sync tables if they don't exist.
func EnsureTables(ctx context.Context, db *sql.DB, logger *slog.Logger) error {
	tables := []struct {
		name string
		sql  string
	}{
		{"data_sync_logs", createSyncLogsTable},
		{"data_sync_metrics", createSyncMetricsTable},
		{"tables_to_sync", createTablesToSyncTable},
	}

	for _, t := range tables {
		logger.Debug("Creating table if not exists", "table", t.name)
		if _, err := db.ExecContext(ctx, t.sql); err != nil {
			return fmt.Errorf("failed to create table %s: %w (%s)", t.name, err, LOC_TBL_SCHEMA)
		}
	}

	logger.Info("Sync tables ensured", "loc", LOC_TBL_SCHEMA)
	return nil
}

// AddTables adds one or more tables to the sync whitelist.
func AddTables(ctx context.Context, db *sql.DB, tableNames []string, creator string, logger *slog.Logger) ([]string, error) {
	if len(tableNames) == 0 {
		return nil, nil
	}

	added := make([]string, 0, len(tableNames))

	for _, name := range tableNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		_, err := db.ExecContext(ctx,
			`INSERT INTO tables_to_sync (table_name, creator) VALUES ($1, $2)
			 ON CONFLICT (table_name) DO NOTHING`,
			name, creator)
		if err != nil {
			logger.Error("Failed to add table to sync list",
				"table", name,
				"error", err,
				"loc", LOC_TBL_ADD)
			return added, fmt.Errorf("failed to add table %s: %w (%s)", name, err, LOC_TBL_ADD)
		}

		added = append(added, name)
		logger.Info("Added table to sync list", "table", name, "loc", LOC_TBL_ADD)
	}

	return added, nil
}

// RemoveTables removes one or more tables from the sync whitelist.
func RemoveTables(ctx context.Context, db *sql.DB, tableNames []string, logger *slog.Logger) ([]string, error) {
	if len(tableNames) == 0 {
		return nil, nil
	}

	removed := make([]string, 0, len(tableNames))

	for _, name := range tableNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		result, err := db.ExecContext(ctx,
			`DELETE FROM tables_to_sync WHERE table_name = $1`,
			name)
		if err != nil {
			logger.Error("Failed to remove table from sync list",
				"table", name,
				"error", err,
				"loc", LOC_TBL_REMOVE)
			return removed, fmt.Errorf("failed to remove table %s: %w (%s)", name, err, LOC_TBL_REMOVE)
		}

		rowsAffected, _ := result.RowsAffected()
		if rowsAffected > 0 {
			removed = append(removed, name)
			logger.Info("Removed table from sync list", "table", name, "loc", LOC_TBL_REMOVE)
		}
	}

	return removed, nil
}

// ListTables returns all tables in the sync whitelist.
func ListTables(ctx context.Context, db *sql.DB) ([]TableInfo, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, table_name, creator, created_at FROM tables_to_sync ORDER BY table_name`)
	if err != nil {
		return nil, fmt.Errorf("failed to list tables: %w (%s)", err, LOC_TBL_LIST)
	}
	defer rows.Close()

	var tables []TableInfo
	for rows.Next() {
		var t TableInfo
		var creator sql.NullString
		if err := rows.Scan(&t.ID, &t.TableName, &creator, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan table row: %w (%s)", err, LOC_TBL_LIST)
		}
		if creator.Valid {
			t.Creator = creator.String
		}
		tables = append(tables, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating table rows: %w (%s)", err, LOC_TBL_LIST)
	}

	return tables, nil
}

// GetTableNames returns just the table names from the whitelist.
func GetTableNames(ctx context.Context, db *sql.DB) ([]string, error) {
	tables, err := ListTables(ctx, db)
	if err != nil {
		return nil, err
	}

	names := make([]string, len(tables))
	for i, t := range tables {
		names[i] = t.TableName
	}
	return names, nil
}

// IsTableInWhitelist checks if a table is in the sync whitelist.
func IsTableInWhitelist(ctx context.Context, db *sql.DB, tableName string) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tables_to_sync WHERE table_name = $1`,
		tableName).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check table whitelist: %w (%s)", err, LOC_TBL_LIST)
	}
	return count > 0, nil
}

// ClearTable truncates a synced table (use with caution).
func ClearTable(ctx context.Context, db *sql.DB, tableName string, logger *slog.Logger) error {
	// Verify table is in whitelist first
	inWhitelist, err := IsTableInWhitelist(ctx, db, tableName)
	if err != nil {
		return err
	}
	if !inWhitelist {
		return fmt.Errorf("table %s is not in sync whitelist (%s)", tableName, LOC_TBL_CLEAR)
	}

	// Use quoted identifier to prevent SQL injection
	_, err = db.ExecContext(ctx, fmt.Sprintf(`TRUNCATE TABLE %s`, quoteIdentifier(tableName)))
	if err != nil {
		return fmt.Errorf("failed to truncate table %s: %w (%s)", tableName, err, LOC_TBL_CLEAR)
	}

	logger.Info("Cleared table", "table", tableName, "loc", LOC_TBL_CLEAR)
	return nil
}

// ClearAllTables truncates all synced tables.
func ClearAllTables(ctx context.Context, db *sql.DB, logger *slog.Logger) error {
	tables, err := GetTableNames(ctx, db)
	if err != nil {
		return err
	}

	for _, tableName := range tables {
		if err := ClearTable(ctx, db, tableName, logger); err != nil {
			// Log error but continue with other tables
			logger.Error("Failed to clear table", "table", tableName, "error", err)
		}
	}

	return nil
}

// LogSyncEvent records a sync event to the data_sync_logs table.
func LogSyncEvent(ctx context.Context, db *sql.DB, tableName, status string, rowsSynced int, archiveRef, errorDetail string) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO data_sync_logs (table_name, status, rows_synced, archive_ref, error_detail)
		 VALUES ($1, $2, $3, $4, $5)`,
		tableName, status, rowsSynced, archiveRef, errorDetail)
	if err != nil {
		return fmt.Errorf("failed to log sync event: %w (%s)", err, LOC_TBL_SCHEMA)
	}
	return nil
}

// GetRecentSyncLogs returns recent sync log entries.
func GetRecentSyncLogs(ctx context.Context, db *sql.DB, limit int) ([]SyncLogEntry, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, table_name, status, rows_synced, archive_ref, error_detail, sync_time
		 FROM data_sync_logs
		 ORDER BY sync_time DESC
		 LIMIT $1`,
		limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get sync logs: %w (%s)", err, LOC_TBL_LIST)
	}
	defer rows.Close()

	var logs []SyncLogEntry
	for rows.Next() {
		var l SyncLogEntry
		var archiveRef, errorDetail sql.NullString
		if err := rows.Scan(&l.ID, &l.TableName, &l.Status, &l.RowsSynced, &archiveRef, &errorDetail, &l.SyncTime); err != nil {
			return nil, fmt.Errorf("failed to scan log row: %w (%s)", err, LOC_TBL_LIST)
		}
		if archiveRef.Valid {
			l.ArchiveRef = archiveRef.String
		}
		if errorDetail.Valid {
			l.ErrorDetail = errorDetail.String
		}
		logs = append(logs, l)
	}

	return logs, nil
}

// GetErrorCount returns the number of errors since a given time.
func GetErrorCount(ctx context.Context, db *sql.DB, since time.Time) (int64, error) {
	var count int64
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM data_sync_logs WHERE status = 'FAILED' AND sync_time >= $1`,
		since).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get error count: %w (%s)", err, LOC_TBL_LIST)
	}
	return count, nil
}

// GetTotalRowsSynced returns the total rows synced since a given time.
func GetTotalRowsSynced(ctx context.Context, db *sql.DB, since time.Time) (int64, error) {
	var total sql.NullInt64
	err := db.QueryRowContext(ctx,
		`SELECT SUM(rows_synced) FROM data_sync_logs WHERE status = 'SUCCESS' AND sync_time >= $1`,
		since).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("failed to get total rows synced: %w (%s)", err, LOC_TBL_LIST)
	}
	if total.Valid {
		return total.Int64, nil
	}
	return 0, nil
}

// quoteIdentifier safely quotes a SQL identifier.
func quoteIdentifier(name string) string {
	// Replace any double quotes with two double quotes (SQL escape)
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
