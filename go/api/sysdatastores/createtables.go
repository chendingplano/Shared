package sysdatastores

import (
	"database/sql"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ipdb"
	_ "github.com/lib/pq"
)

func CreateSysTables(logger ApiTypes.JimoLogger) error {
	// This function creates all the tables.
	var db *sql.DB = ApiTypes.SharedDBHandle
	database_type := ApiTypes.DBType

	CreateLoginSessionsTable(logger, db, database_type, ApiTypes.LibConfig.SystemTableNames.TableNameLoginSessions)
	CreateIDMgrTable(logger, db, database_type, ApiTypes.LibConfig.SystemTableNames.TableNameIDMgr)
	CreateActivityLogTable(logger, db, database_type, ApiTypes.LibConfig.SystemTableNames.TableNameActivityLog)
	CreateSessionLogTable(logger, db, database_type, ApiTypes.LibConfig.SystemTableNames.TableNameSessionLog)
	CreateEmailStoreTable(logger, db, database_type, ApiTypes.LibConfig.SystemTableNames.TableNameEmailStore)
	CreatePromptStoreTable(logger, db, database_type, ApiTypes.LibConfig.SystemTableNames.TableNamePromptStore)
	CreateResourcesTable(logger, db, database_type, ApiTypes.LibConfig.SystemTableNames.TableNameResources)
	CreateTableManagerTable(logger)
	CreateIconsTable(logger, db, database_type, ApiTypes.LibConfig.SystemTableNames.TableNameResources)
	ipdb.CreateTables(logger)

	// Run migrations for existing tables
	RunMigrations(logger, db, database_type)

	return nil
}

// RunMigrations applies schema migrations to existing tables.
// Each migration is idempotent - safe to run multiple times.
func RunMigrations(logger ApiTypes.JimoLogger, db *sql.DB, db_type string) {
	logger.Info("Running database migrations")

	logger.Info("Database migrations completed")
}
