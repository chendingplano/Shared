package stores

import (
	"database/sql"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/spf13/viper"
)

func GetGroupEntries(groupName string) map[string]interface{} {
    // Get a sub-viper for the specified group
    groupViper := viper.Sub(groupName)
    if groupViper == nil {
        // Group doesn't exist
        return nil
    }
    
    // Get all settings within this group
    return groupViper.AllSettings()
}

func InitSharedStores(db_type string, db *sql.DB) {
	// 1. InitInMemStore
	var id_start_value = viper.GetInt("id_start_value")
	var id_inc_value = viper.GetInt("id_inc_value")
	id_config := GetGroupEntries("system_ids")
	InitInMemStore(db_type, ApiTypes.LibConfig.SystemTableNames.TableName_IDMgr, db, 
		id_start_value, id_inc_value, id_config)

	InitResourceStore(db_type, ApiTypes.LibConfig.SystemTableNames.TableName_Resources, db) 
}