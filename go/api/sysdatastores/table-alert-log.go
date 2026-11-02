// Description
// Persists JimoLogger WARN/ERROR events to public.jimo_alert_log (PostgreSQL only).
package sysdatastores

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/databaseutil"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

const AlertLogTableName = "public.jimo_alert_log"

var alertLogOnce sync.Once

// CreateAlertLogTable creates public.jimo_alert_log and, on success, starts
// recording every JimoLogger Warn/Error into it. No-op for non-PostgreSQL.
func CreateAlertLogTable(logger ApiTypes.JimoLogger, db *sql.DB, db_type string) error {
	if db_type != ApiTypes.PgName || db == nil {
		return nil
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS ` + AlertLogTableName + ` (
			id         BIGSERIAL PRIMARY KEY,
			level      VARCHAR(8) NOT NULL,
			message    TEXT NOT NULL,
			req_id     VARCHAR(64),
			loc        VARCHAR(128),
			call_flow  TEXT,
			attrs      JSONB,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			status       VARCHAR(16) NOT NULL DEFAULT 'open',
			owner        VARCHAR(255),
			resolution   TEXT,
			resolver     VARCHAR(255),
			resolve_time TIMESTAMPTZ
		)`,
		`CREATE INDEX IF NOT EXISTS idx_jimo_alert_log_created_at ON ` + AlertLogTableName + ` (created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_jimo_alert_log_level ON ` + AlertLogTableName + ` (level, created_at)`,
	}
	for _, stmt := range stmts {
		if err := databaseutil.ExecuteStatement(db, stmt); err != nil {
			logger.Error("failed creating alert log table (SHD_ALT_031)", "error", err)
			return err
		}
	}
	StartAlertLogWriter(db)
	return nil
}

// StartAlertLogWriter begins recording JimoLogger Warn/Error into the table.
// The table itself comes from goose migration (ChenWeb shared_migrations) or
// CreateAlertLogTable. Safe to call more than once.
func StartAlertLogWriter(db *sql.DB) {
	if db == nil {
		return
	}
	alertLogOnce.Do(func() { startAlertLogWriter(db) })
}

// startAlertLogWriter installs the sink. Inserts happen on one background
// goroutine; if the buffer is full the record is dropped rather than blocking
// the caller. Failures go to slog directly — never through JimoLogger, which
// would re-enter this sink.
func startAlertLogWriter(db *sql.DB) {
	ch := make(chan loggerutil.AlertRecord, 1024)
	go func() {
		for r := range ch {
			attrs, _ := json.Marshal(r.Attrs)
			_, err := db.Exec(`INSERT INTO `+AlertLogTableName+
				` (level, message, req_id, loc, call_flow, attrs, created_at)
				  VALUES ($1,$2,$3,$4,$5,$6,$7)`,
				r.Level, r.Message, r.ReqID, r.Loc, r.CallFlow, attrs, r.Time)
			if err != nil {
				slog.Warn("alert log insert failed (SHD_ALT_062)", "error", err)
			}
		}
	}()
	loggerutil.SetAlertSink(func(r loggerutil.AlertRecord) {
		select {
		case ch <- r:
		default:
			slog.Warn("alert log buffer full, dropping record (SHD_ALT_070)")
		}
	})
}
