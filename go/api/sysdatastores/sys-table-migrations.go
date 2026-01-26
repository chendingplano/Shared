package sysdatastores

import (
	"database/sql"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

// RunSchemaMigrations runs idempotent schema migrations for PostgreSQL
// These are constraint updates, column additions, etc. that need to be applied
// to existing databases. Each migration should be safe to run multiple times.
func RunSchemaMigrations(
	logger *loggerutil.JimoLogger,
	db *sql.DB,
	db_type string) error {
	logger.Info("Running sys-table migrations")
	return nil
}
