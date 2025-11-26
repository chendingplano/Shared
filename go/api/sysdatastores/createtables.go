package sysdatastores

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	_ "github.com/lib/pq"
)

func CreateTables() error {
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

    log.Printf("Sessions:%s", ApiTypes.LibConfig.SystemTableNames.TableName_Sessions)
    log.Printf("EmailStore:%s", ApiTypes.LibConfig.SystemTableNames.TableName_EmailStore)

    CreateLoginSessionsTable(db, database_type, ApiTypes.LibConfig.SystemTableNames.TableName_Sessions)
    CreateUsersTable(db, database_type, ApiTypes.LibConfig.SystemTableNames.TableName_Users)
    CreateIDMgrTable(db, database_type, ApiTypes.LibConfig.SystemTableNames.TableName_IDMgr)
    CreateActivityLogTable(db, database_type, ApiTypes.LibConfig.SystemTableNames.TableName_ActivityLog)
    CreateSessionLogTable(db, database_type, ApiTypes.LibConfig.SystemTableNames.TableName_SessionLog)
    CreateEmailStoreTable(db, database_type, ApiTypes.LibConfig.SystemTableNames.TableName_EmailStore)
    CreatePromptStoreTable(db, database_type, ApiTypes.LibConfig.SystemTableNames.TableName_PromptStore)
    CreateResourcesTable(db, database_type, ApiTypes.LibConfig.SystemTableNames.TableName_Resources)
    return nil
}
