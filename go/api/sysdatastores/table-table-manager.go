package sysdatastores

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/databaseutil"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

func CreateTableManagerTable(logger *loggerutil.JimoLogger) error {
    db_type := ApiTypes.DatabaseInfo.DBType
    table_name := ApiTypes.LibConfig.SystemTableNames.TableNameTableManager
    var stmt string
    const common_fields = 
        "db_type VARCHAR(32) NOT NULL, " +
        "db_name VARCHAR(255) NOT NULL, " +
        "table_name VARCHAR(255) NOT NULL, " +
        "table_type VARCHAR(32) NOT NULL, " +
        "table_desc VARCHAR(255) NOT NULL, " +
        "table_def JSON NOT NULL, " +
        "remarks VARCHAR(255) NOT NULL, " +
        "created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP, " +
        "updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP"

    var db *sql.DB
    switch db_type {
    case ApiTypes.MysqlName:
         stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" + common_fields +
            ") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;"
         db = ApiTypes.MySql_DB_miner


    case ApiTypes.PgName:
         stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" +
            common_fields + ")"
         db = ApiTypes.PG_DB_miner

    default:
        err := fmt.Errorf("database type not supported:%s (MID_TMG_042)", db_type)
        log.Printf("***** Alarm:%s", err.Error())
        return err
    }

    err := databaseutil.ExecuteStatement(db, stmt)
    if err != nil {
        err1 := fmt.Errorf("failed creating table '%s' (MID_TMG_049), err: %w, stmt:%s", table_name, err, stmt)
        log.Printf("***** Alarm: %s", err1.Error())
        return err1
    }

	logger.Info("Create table success", "table_name", table_name)

    return nil
}
