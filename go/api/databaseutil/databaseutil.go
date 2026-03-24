package databaseutil

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

var AllowedOps = map[string]bool{
	"=":    true,
	"!=":   true,
	">":    true,
	"<":    true,
	"LIKE": true,
	// add others as needed
}

var AllowedLogicOps = map[string]bool{
	"AND": true,
	"OR":  true,
}

func InitDB(ctx context.Context, commonConfig ApiTypes.CommonConfigDef) error {
	logger := loggerutil.CreateDefaultLogger("SHD_DBU_035")
	call_flow, _ := ctx.Value(ApiTypes.CallFlowKey).(string)
	logger.Info("InitDB called (SHD_DBS_0 45)", "caller", call_flow)

	if commonConfig.AppInfo.DatabaseType != "pg" {
		error_msg := fmt.Errorf("(MID_26031070) invalid PG config name:%s",
			commonConfig.AppInfo.DatabaseType)
		logger.Error("Invalid PG config name", "error", error_msg)
		return error_msg
	}

	if commonConfig.PGConf.Create {
		logger.Info("To create PGDBMiner (SHD_DBS_024)")
		err := ApiUtils.CreatePGDB(logger, &commonConfig.PGConf)
		if err != nil {
			logger.Error("Failed creating PG connection (SHD_DBS_026)", "error", err)
			return err
		}
		logger.Info("PostgreSQL configured (SHD_DBS_033)", "call_flow", call_flow)
	} else {
		logger.Info("PostgreSQL not configured (SHD_DBS_033)", "call_flow", call_flow)
	}

	if commonConfig.MySQLConf.Create {
		err := ApiUtils.CreateMySqlDB(logger, commonConfig.MySQLConf)
		if err != nil {
			logger.Error("Failed creating MySQL connection (SHD_DBS_032)", "call_flow", call_flow, "error", err)
			return err
		}
		logger.Info("MySQL configured (SHD_DBS_044)", "call_flow", call_flow)
	}

	return nil
}

// Helper to validate table names (prevents SQL injection)
func IsValidTableName(name string) bool {
	// To prevent SQL injection, table names should be made of alphanumerics
	// and underscores only;
	return regexp.MustCompile(`^[a-zA-Z0-9_]+$`).MatchString(name)
}

func HandleSelect(c echo.Context,
	logger ApiTypes.JimoLogger,
	base_stmt string,
	db *sql.DB,
	allowedFields map[string]bool,
	whereClauses []string,
	args []interface{},
	limit string) (*sql.Rows, error) {
	// This function handles dynamic WHERE clause construction.
	// The conditions are passed as query parameters:
	// field_0, op_0, val_0, logic_opr_0
	// field_1, op_1, val_1, logic_opr_1
	// ...
	// IMPORTANT: This function assumes the query conditions are
	// passed through the query portion of the URL (from echo.Context)
	//
	// SECURITY: Uses parameterized queries to prevent SQL injection.
	// User input (val) is NEVER interpolated into the query string.

	logger.Info("To retrieve data for Documents (SHD_DBS_024)")

	i := 0
	for {
		logger.Info("Processing filter index", "index", i)
		field := c.QueryParam(fmt.Sprintf("field_%d", i))
		if field == "" {
			break
		}

		op := c.QueryParam(fmt.Sprintf("op_%d", i))
		logic_opr := "AND"
		if i > 0 {
			logic_opr = c.QueryParam(fmt.Sprintf("logic_opr_%d", i))
			if logic_opr == "" || !AllowedLogicOps[logic_opr] {
				error_msg := fmt.Errorf("(MID_26031072) invalid logic operator:%s", logic_opr)
				logger.Error("Invalid logic operator in HandleSelect", "logic_opr", logic_opr, "error", error_msg)
				return nil, error_msg
			}
		}

		if !allowedFields[field] {
			error_msg := fmt.Errorf("(MID_26031073) invalid field:%s", field)
			logger.Error("Invalid field in HandleSelect", "field", field)
			return nil, error_msg
		}

		if !AllowedOps[op] {
			error_msg := fmt.Errorf("(MID_26031074) invalid operator:%s", op)
			return nil, error_msg
		}

		val := c.QueryParam(fmt.Sprintf("val_%d", i))

		// SECURITY: Build WHERE clause with placeholders only - never interpolate val
		if i > 0 {
			whereClauses = append(whereClauses, fmt.Sprintf("%s %s %s ?", logic_opr, field, op))
		} else {
			whereClauses = append(whereClauses, fmt.Sprintf("%s %s ?", field, op))
		}
		args = append(args, val)

		logger.Info("Received filter", "field", field, "op", op, "logic_opr", logic_opr)
		i++
	}

	// SECURITY: Construct query using parameterized placeholders only
	query := base_stmt
	if len(whereClauses) > 0 {
		query += " WHERE " + strings.Join(whereClauses, " ")
	}

	if limit != "" {
		query += " " + limit
	}

	logger.Info("Constructed query", "query", query)
	rows, err := db.Query(query, args...)
	if err != nil {
		error_msg := fmt.Errorf("(MID_26031075) select failed, err:%v, query:%s", err, query)
		logger.Error("Failed to execute query", "error", error_msg)
		return nil, error_msg
	}
	return rows, nil
}

func AosExecuteStatement(
	db *sql.DB,
	db_type string,
	stmt string) error {
	switch db_type {
	case ApiTypes.MysqlName:
		return ExecuteStatement(db, stmt)

	case ApiTypes.PgName:
		return ExecuteStatement(db, stmt)

	default:
		return fmt.Errorf("(MID_26031076) unsupported database type: %s", db_type)
	}
}

func ExecuteStatement(db *sql.DB, stmt string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("(MID_26031077) failed to begin transaction: %w", err)
	}

	defer func() {
		_ = tx.Rollback() // Rollback if not committed
	}()

	_, err1 := tx.Exec(stmt)
	if err1 != nil {
		return fmt.Errorf("(MID_26031078) failed to execute query, error: %w, stmt:%s", err1, stmt)
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("(MID_26031079) failed to commit transaction: %w", err)
	}

	return nil
}

func StrPtr(s string) *string {
	return &s
}

func CloseDatabase(config ApiTypes.CommonConfigDef) {
	if config.MySQLConf.ProjectDBHandle != nil {
		config.MySQLConf.ProjectDBHandle.Close()
	}

	if config.MySQLConf.SharedDBHandle != nil && config.MySQLConf.SharedDBHandle != config.MySQLConf.ProjectDBHandle {
		config.MySQLConf.SharedDBHandle.Close()
	}

	if config.MySQLConf.AutotesterDBHandle != nil {
		config.MySQLConf.AutotesterDBHandle.Close()
	}

	if config.PGConf.ProjectDBHandle != nil {
		config.PGConf.ProjectDBHandle.Close()
	}

	if config.PGConf.SharedDBHandle != nil && config.PGConf.SharedDBHandle != config.PGConf.ProjectDBHandle {
		config.PGConf.SharedDBHandle.Close()
	}

	if config.PGConf.AutotesterDBHandle != nil {
		config.PGConf.AutotesterDBHandle.Close()
	}
}

func CreateGenericTable(
	rc ApiTypes.RequestContext,
	appInfo ApiTypes.AppInfo,
	mysqlConfig ApiTypes.DatabaseConfig,
	pgConfig ApiTypes.DatabaseConfig,
	table_name string) error {
	db_type := appInfo.DatabaseType
	logger := rc.GetLogger()
	const common_fields = "record_type VARCHAR(255) NOT NULL, " +
		"doc_type VARCHAR(255) NOT NULL, " +
		"doc_name VARCHAR(255) NOT NULL, " +
		"doc_desc TEXT NOT NULL, " +
		"json_doc JSON NOT NULL, " +
		"record_data TEXT NOT NULL, " +
		"remarks TEXT DEFAULT NULL, " +
		"del_flag TINYINT(1) DEFAULT 0, " +
		"created_by VARCHAR(255) NOT NULL, " +
		"updated_by VARCHAR(255) NOT NULL, " +
		"created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP, " +
		"updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP, "

	var stmt string
	var db *sql.DB
	switch db_type {
	case ApiTypes.MysqlName:
		db = mysqlConfig.ProjectDBHandle
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" +
			"doc_id BIGINT AUTO_INCREMENT PRIMARY KEY, " + common_fields +
			") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;"

	case ApiTypes.PgName:
		db = pgConfig.ProjectDBHandle
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" + common_fields + ");"

	default:
		return fmt.Errorf("(MID_26031080) database type not supported:%s", db_type)
	}

	_, err := db.Exec(stmt)
	if err != nil {
		return fmt.Errorf("(MID_26031081) failed to create table: %w", err)
	}
	logger.Info("Table created successfully (SHD_DBS_322)", "table_name", table_name)
	return nil
}
