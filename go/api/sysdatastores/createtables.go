package sysdatastores

import (
	"database/sql"
	"fmt"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	_ "github.com/lib/pq"
)

func CreateSysTables(logger ApiTypes.JimoLogger) error {
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

	CreateLoginSessionsTable(logger, db, database_type, ApiTypes.LibConfig.SystemTableNames.TableNameLoginSessions)
	CreateIDMgrTable(logger, db, database_type, ApiTypes.LibConfig.SystemTableNames.TableNameIDMgr)
	CreateActivityLogTable(logger, db, database_type, ApiTypes.LibConfig.SystemTableNames.TableNameActivityLog)
	CreateSessionLogTable(logger, db, database_type, ApiTypes.LibConfig.SystemTableNames.TableNameSessionLog)
	CreateEmailStoreTable(logger, db, database_type, ApiTypes.LibConfig.SystemTableNames.TableNameEmailStore)
	CreatePromptStoreTable(logger, db, database_type, ApiTypes.LibConfig.SystemTableNames.TableNamePromptStore)
	CreateResourcesTable(logger, db, database_type, ApiTypes.LibConfig.SystemTableNames.TableNameResources)
	CreateTableManagerTable(logger)
	CreateIconsTable(logger, db, database_type, ApiTypes.LibConfig.SystemTableNames.TableNameResources)

	// Run migrations for existing tables
	RunMigrations(logger, db, database_type)

	return nil
}

// RunMigrations applies schema migrations to existing tables.
// Each migration is idempotent - safe to run multiple times.
func RunMigrations(logger ApiTypes.JimoLogger, db *sql.DB, db_type string) {
	logger.Info("Running database migrations")

	// NO-OP: Users table migration removed — users are now managed by Kratos.
	// The users table is no longer created (CreateUsersTable was removed)
	// and will be dropped by mirai's schema migrations.

	logger.Info("Database migrations completed")
}
