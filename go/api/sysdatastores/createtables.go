package sysdatastores

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
	_ "github.com/lib/pq"
)

func CreateTables(logger *loggerutil.JimoLogger) error {
	// This function creates all the tables.
	var db *sql.DB
	database_type := ApiTypes.DatabaseInfo.DBType
	switch database_type {
	case ApiTypes.MysqlName:
		db = ApiTypes.MySql_DB_miner

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner

	default:
		return fmt.Errorf("***** Unrecognized database type (MID_DBS_124): %s", database_type)
	}

	log.Printf("LoginSessions:%s", ApiTypes.LibConfig.SystemTableNames.TableNameLoginSessions)
	log.Printf("EmailStore:%s", ApiTypes.LibConfig.SystemTableNames.TableNameEmailStore)

	CreateLoginSessionsTable(db, database_type, ApiTypes.LibConfig.SystemTableNames.TableNameLoginSessions)
	CreateUsersTable(logger, db, database_type, ApiTypes.LibConfig.SystemTableNames.TableNameUsers)
	CreateIDMgrTable(db, database_type, ApiTypes.LibConfig.SystemTableNames.TableNameIDMgr)
	CreateActivityLogTable(db, database_type, ApiTypes.LibConfig.SystemTableNames.TableNameActivityLog)
	CreateSessionLogTable(db, database_type, ApiTypes.LibConfig.SystemTableNames.TableNameSessionLog)
	CreateEmailStoreTable(db, database_type, ApiTypes.LibConfig.SystemTableNames.TableNameEmailStore)
	CreatePromptStoreTable(db, database_type, ApiTypes.LibConfig.SystemTableNames.TableNamePromptStore)
	CreateResourcesTable(db, database_type, ApiTypes.LibConfig.SystemTableNames.TableNameResources)
	CreateTableManagerTable()
	return nil
}
