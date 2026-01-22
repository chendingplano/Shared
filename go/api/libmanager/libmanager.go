package libmanager

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/chendingplano/shared/go/api/auth"
	"github.com/chendingplano/shared/go/api/stores"
	"github.com/chendingplano/shared/go/api/sysdatastores"
	"github.com/chendingplano/shared/go/authmiddleware"
	"github.com/spf13/viper"
)

func LoadLibConfig(ctx context.Context, config_path string) {
	// config_path should be "~/Workspace/Shared/libconfig.toml"
	// 1. DB Must be initialized properly
	call_flow := ctx.Value(ApiTypes.CallFlowKey).(string)

	log.Printf("Loading config from %s (SHD_LMG_047)", config_path)
	viper.SetConfigFile(config_path)
	viper.SetConfigType("toml")

	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Printf("***** Alarm: config file not found (%s->SHD_LMG_054): %s", call_flow, config_path)
			os.Exit(1)
		}
		log.Printf("***** Alarm: error reading config (%s->SHD_LMG_056): %v", call_flow, err)
		os.Exit(1)
	}

	// Override with environment variables (e.g., DATABASE_URL)
	viper.AutomaticEnv()

	// Unmarshal into struct
	if err := viper.Unmarshal(&ApiTypes.LibConfig); err != nil {
		log.Printf("***** Alarm: unable to decode config (%s->SHD_LMG_064): %v", call_flow, err)
		os.Exit(1)
	}
}

func InitLib(ctx context.Context, config_path string) {
	log.Printf("Lib Config, sessions:%s", ApiTypes.LibConfig.SystemTableNames.TableNameLoginSessions)
	log.Printf("Lib Config, email_store:%s", ApiTypes.LibConfig.SystemTableNames.TableNameEmailStore)
	log.Printf("Lib Config, test:%s", ApiTypes.LibConfig.SystemTableNames.TableNameTest)

	authmiddleware.Init()
	auth.SetAuthInfo(ApiTypes.GetDBType(),
		ApiUtils.GetDefahotHomeURL(),
		ApiTypes.LibConfig.SystemTableNames.TableNameLoginSessions,
		ApiTypes.LibConfig.SystemTableNames.TableNameUsers)

	var db *sql.DB
	db_type := ApiTypes.DatabaseInfo.DBType
	switch db_type {
	case ApiTypes.MysqlName:
		db = ApiTypes.MySql_DB_miner

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner

	default:
		error_msg := fmt.Sprintf("Unrecognized database type (SHD_LMG_026):%s", db_type)
		log.Printf("***** Alarm:%s", error_msg)
		os.Exit(1)
	}

	if db == nil {
		error_msg := "'db' is nil (SHD_LMG_071)"
		log.Printf("***** Alarm:%s", error_msg)
		os.Exit(1)
	}

	stores.InitSharedStores(db_type, db)
	sysdatastores.InitActivityLogCache(
		db_type,
		ApiTypes.LibConfig.SystemTableNames.TableNameActivityLog,
		db)

	// 1. Upsert the activity_log id record
	rc := EchoFactory.NewRCAsAdmin("SHD_LMG_089")
	defer rc.Close()
	err := sysdatastores.UpsertActivityLogIDDef(rc)
	if err != nil {
		log.Printf("Failed upsert the system id record (SHD_LMG_021), err:%v", err)
		os.Exit(1)
	}

	// 2. Init SessionLog
	sysdatastores.InitSessionLogCache(db_type, ApiTypes.LibConfig.SystemTableNames.TableNameSessionLog, db)
}

func ExitLib() {
	stores.StopInMemStore()
	sysdatastores.StopActivityLogCache()
	sysdatastores.StopSessionLogCache()
}
