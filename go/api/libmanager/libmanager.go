package libmanager

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/auth"
	"github.com/chendingplano/shared/go/api/stores"
	"github.com/chendingplano/shared/go/api/sysdatastores"
	"github.com/spf13/viper"
)

func InitLib() {
	// 1. DB Must be initialized properly
	config_path := "/Users/cding/Workspace/Shared/libconfig.toml"
	log.Printf("Loading config from %s (SHD_LMG_047)", config_path)
	viper.SetConfigFile(config_path)
	viper.SetConfigType("toml")

	// Optional: set defaults
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("debug", false)

	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Printf("***** Alarm: config file not found (SHD_LMG_054): %s", config_path)
			os.Exit(1)
		}
		log.Printf("***** Alarm: error reading config (SHD_LMG_056): %v", err)
		os.Exit(1)
	}

	// Override with environment variables (e.g., DATABASE_URL)
	viper.AutomaticEnv()

	// Unmarshal into struct
	if err := viper.Unmarshal(&ApiTypes.LibConfig); err != nil {
		log.Printf("***** Alarm: unable to decode config (SHD_LMG_064): %v", err)
		os.Exit(1)
	}

	log.Printf("Lib Config, sessions:%s", ApiTypes.LibConfig.SystemTableNames.TableName_Sessions)
	log.Printf("Lib Config, email_store:%s", ApiTypes.LibConfig.SystemTableNames.TableName_EmailStore)
	log.Printf("Lib Config, test:%s", ApiTypes.LibConfig.SystemTableNames.TableName_Test)
	
	auth.SetAuthInfo(ApiTypes.GetDBType(),
		ApiTypes.DatabaseInfo.HomeURL,
		ApiTypes.LibConfig.SystemTableNames.TableName_Sessions,
		ApiTypes.LibConfig.SystemTableNames.TableName_Users)

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

	stores.InitSharedStores(db_type, db)
	sysdatastores.InitActivityLogCache(db_type, ApiTypes.LibConfig.SystemTableNames.TableName_ActivityLog, db)
	err := sysdatastores.UpsertActivityLogIDDef()
	if err != nil {
		log.Printf("Failed upsert the system id record (SHD_LMG_021), err:%v", err)
		os.Exit(1)
	}
	
	// 2. Init SessionLog
	sysdatastores.InitSessionLogCache(db_type, ApiTypes.LibConfig.SystemTableNames.TableName_SessionLog, db)
}

func ExitLib() {
	stores.StopInMemStore()
	sysdatastores.StopActivityLogCache()
	sysdatastores.StopSessionLogCache()
}
