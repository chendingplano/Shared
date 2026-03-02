package libmanager

import (
	"context"
	"database/sql"
	"os"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/chendingplano/shared/go/api/auth"
	"github.com/chendingplano/shared/go/api/icons"
	"github.com/chendingplano/shared/go/api/stores"
	"github.com/chendingplano/shared/go/api/sysdatastores"
	"github.com/chendingplano/shared/go/authmiddleware"
)

func InitLib(ctx context.Context, config_path string, loc string) {
	ApiUtils.LoadLibConfig(loc)
	admin_rc := EchoFactory.NewRCAsAdmin("SHD_LMG_050")
	defer admin_rc.Close()
	logger := admin_rc.GetLogger()
	logger.Info("Lib Config", "sessions", ApiTypes.LibConfig.SystemTableNames.TableNameLoginSessions)
	logger.Info("Lib Config", "email_store", ApiTypes.LibConfig.SystemTableNames.TableNameEmailStore)
	logger.Info("Lib Config", "test", ApiTypes.LibConfig.SystemTableNames.TableNameTest)

	authmiddleware.Init()

	// Wire up Kratos authenticator when AUTH_USE_KRATOS is enabled
	if os.Getenv("AUTH_USE_KRATOS") == "true" {
		authmiddleware.KratosAuthenticator = auth.IsAuthenticatedKratosFromRC
		logger.Info("Kratos authenticator enabled")
	}

	auth.SetAuthInfo(ApiTypes.DBType,
		ApiUtils.GetDefaultHomeURL(),
		ApiTypes.LibConfig.SystemTableNames.TableNameLoginSessions)

	var db *sql.DB = ApiTypes.ProjectDBHandle
	if db == nil {
		logger.Error("db is not set")
		os.Exit(1)
	}

	stores.InitSharedStores(ApiTypes.DBType, db)
	sysdatastores.InitActivityLogCache(
		ApiTypes.DBType,
		ApiTypes.LibConfig.SystemTableNames.TableNameActivityLog,
		db)

	// 1. InitKratosClient
	auth.InitKratosClient()

	// 2. Upsert the activity_log id record
	rc := EchoFactory.NewRCAsAdmin("SHD_LMG_089")
	defer rc.Close()
	err := sysdatastores.UpsertActivityLogIDDef(rc)
	if err != nil {
		logger.Error("Failed upsert the system id record", "error", err)
		os.Exit(1)
	}

	// 3. Init SessionLog
	sysdatastores.InitSessionLogCache(ApiTypes.DBType, ApiTypes.LibConfig.SystemTableNames.TableNameSessionLog, db)

	// 4. Init the icon service
	icons.InitIconService(admin_rc)
}

func ExitLib() {
	stores.StopInMemStore()
	sysdatastores.StopActivityLogCache()
	sysdatastores.StopSessionLogCache()
	// loggerutil.CloseFileLogging()
}
