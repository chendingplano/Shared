package logs2db

import (
	"context"
	"fmt"
	"strings"
)

// Location codes for insert operations
const (
	LOC_INSERT_TABLE  = "SHD_L2D_020"
	LOC_INSERT_BATCH  = "SHD_L2D_021"
	LOC_INSERT_TRUNC  = "SHD_L2D_022"
	LOC_INSERT_COUNT  = "SHD_L2D_023"
)

// EnsureTable creates the target table if it doesn't exist.
func (s *Log2DBService) EnsureTable(ctx context.Context) error {
	stmt := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		id               VARCHAR(40) PRIMARY KEY,
		entry_type       VARCHAR(20) NOT NULL,
		message          TEXT NOT NULL,
		sys_prompt       TEXT,
		sys_prompt_nlines INT,
		caller_filename  VARCHAR(120),
		caller_line      INT,
		json_obj         JSONB NOT NULL,
		log_filename     TEXT NOT NULL,
		log_line_num     INT NOT NULL,
		error_msg        TEXT,
		remarks          TEXT,
		created_at       TIMESTAMPTZ NOT NULL,
		UNIQUE(log_filename, log_line_num)
	)`, s.config.DBTableName)

	if _, err := s.db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("failed to create table %s: %w (%s)", s.config.DBTableName, err, LOC_INSERT_TABLE)
	}

	// Create indexes for common queries
	indexes := []string{
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_filename ON %s (log_filename)`,
			s.config.DBTableName, s.config.DBTableName),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_entry_type ON %s (entry_type)`,
			s.config.DBTableName, s.config.DBTableName),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_created_at ON %s (created_at)`,
			s.config.DBTableName, s.config.DBTableName),
	}

	for _, idx := range indexes {
		if _, err := s.db.ExecContext(ctx, idx); err != nil {
			return fmt.Errorf("failed to create index: %w (%s)", err, LOC_INSERT_TABLE)
		}
	}

	return nil
}

const batchSize = 100

// InsertBatch inserts a slice of LogEntry records using a transaction.
// Uses multi-row INSERT with ON CONFLICT DO NOTHING for idempotency.
func (s *Log2DBService) InsertBatch(ctx context.Context, entries []LogEntry) (int, error) {
	if len(entries) == 0 {
		return 0, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w (%s)", err, LOC_INSERT_BATCH)
	}
	defer tx.Rollback()

	totalInserted := 0
	const numCols = 13

	for i := 0; i < len(entries); i += batchSize {
		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}
		batch := entries[i:end]

		// Build multi-row VALUES clause
		valueStrings := make([]string, 0, len(batch))
		args := make([]any, 0, len(batch)*numCols)

		for j, e := range batch {
			offset := j * numCols
			valueStrings = append(valueStrings, fmt.Sprintf(
				"($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
				offset+1, offset+2, offset+3, offset+4, offset+5, offset+6, offset+7,
				offset+8, offset+9, offset+10, offset+11, offset+12, offset+13,
			))

			var jsonObj any
			if len(e.JSONObj) > 0 {
				jsonObj = string(e.JSONObj)
			} else {
				jsonObj = "{}"
			}

			var sysPromptNLines any
			if e.SysPromptNLines > 0 {
				sysPromptNLines = e.SysPromptNLines
			} else {
				sysPromptNLines = nil
			}

			var callerLine any
			if e.CallerLine > 0 {
				callerLine = e.CallerLine
			} else {
				callerLine = nil
			}

			var sysPrompt any
			if e.SysPrompt != "" {
				sysPrompt = e.SysPrompt
			} else {
				sysPrompt = nil
			}

			var callerFilename any
			if e.CallerFilename != "" {
				callerFilename = e.CallerFilename
			} else {
				callerFilename = nil
			}

			var errorMsg any
			if e.ErrorMsg != "" {
				errorMsg = e.ErrorMsg
			} else {
				errorMsg = nil
			}

			var remarks any
			if e.Remarks != "" {
				remarks = e.Remarks
			} else {
				remarks = nil
			}

			args = append(args,
				e.ID,
				e.EntryType,
				e.Message,
				sysPrompt,
				sysPromptNLines,
				callerFilename,
				callerLine,
				jsonObj,
				e.LogFilename,
				e.LogLineNum,
				errorMsg,
				remarks,
				e.CreatedAt,
			)
		}

		query := fmt.Sprintf(
			`INSERT INTO %s (id, entry_type, message, sys_prompt, sys_prompt_nlines,
			caller_filename, caller_line, json_obj, log_filename, log_line_num,
			error_msg, remarks, created_at)
			VALUES %s
			ON CONFLICT (log_filename, log_line_num) DO NOTHING`,
			s.config.DBTableName,
			strings.Join(valueStrings, ","),
		)

		result, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return totalInserted, fmt.Errorf("failed to insert batch: %w (%s)", err, LOC_INSERT_BATCH)
		}

		rowsAffected, _ := result.RowsAffected()
		totalInserted += int(rowsAffected)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w (%s)", err, LOC_INSERT_BATCH)
	}

	return totalInserted, nil
}

// TruncateTable removes all rows from the target table (for reload).
func (s *Log2DBService) TruncateTable(ctx context.Context) error {
	stmt := fmt.Sprintf("TRUNCATE TABLE %s", s.config.DBTableName)
	if _, err := s.db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("failed to truncate table %s: %w (%s)", s.config.DBTableName, err, LOC_INSERT_TRUNC)
	}
	return nil
}

// CountEntries returns the total number of rows in the target table.
func (s *Log2DBService) CountEntries(ctx context.Context) (int, error) {
	var count int
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", s.config.DBTableName)
	if err := s.db.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count entries: %w (%s)", err, LOC_INSERT_COUNT)
	}
	return count, nil
}
