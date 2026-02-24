// Description
// Goose migration tracking table
package sysdatastores

import (
	"database/sql"
	"fmt"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/databaseutil"
)

func CreateGooseTable(
	logger ApiTypes.JimoLogger,
	db *sql.DB,
	db_type string,
	table_name string) error {
	logger.Info("Create table", "table_name", table_name)

	var stmt string
	fields :=
		`id              BIGSERIAL PRIMARY KEY,
		version_id    BIGINT       NOT NULL,
		is_applied    BOOL         NOT NULL,
		timestamp     TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP`

	switch db_type {
	case ApiTypes.MysqlName:
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" + fields +
			", INDEX idx_version_id (version_id), " +
			"INDEX idx_timestamp (timestamp) " +
			") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;"

	case ApiTypes.PgName:
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" + fields + ")"

	default:
		err := fmt.Errorf("database type not supported:%s (SHD_GSE_117)", db_type)
		logger.Error("database type not supported", "db_type", db_type)
		return err
	}

	err := databaseutil.ExecuteStatement(db, stmt)
	if err != nil {
		error_msg := fmt.Errorf("failed creating table (SHD_GSE_045), err: %w, stmt:%s", err, stmt)
		logger.Error("failed creating table", "table_name", table_name, "error", err)
		return error_msg
	}

	if db_type == ApiTypes.PgName {
		idx1 := `CREATE INDEX IF NOT EXISTS idx_version_id ON ` + table_name + ` (version_id);`
		databaseutil.ExecuteStatement(db, idx1)

		idx2 := `CREATE INDEX IF NOT EXISTS idx_timestamp ON ` + table_name + ` (timestamp);`
		databaseutil.ExecuteStatement(db, idx2)
	}

	logger.Info("Create table success", "table_name", table_name)

	return nil
}
