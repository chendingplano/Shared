package sysdatastores

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/databaseutil"
)

const (
	resource_store_selected_field_names = "resource_id, 	 	resource_desc,    resource_type, " +
		"db_name,           table_name,     resource_status, resource_remarks, resource_def, " +
		"query_conds,       error_msg"

	ResourceStoreTableDescSimple = `
        ResourceID        	int64     # A unique sequence number
        ResourceName 		string    # Resource name, identify the resource
        ResourceOpr         string    # Resource action, identify the operation on the resource
        ResourceDesc		string    # Resource description
        ResourceType		string    # Resource type
        DBName              string    # Its db name, if the resource type is 'table'
        TableName           string    # Its table name, if the resource type is 'table'
        ResourceStatus      string    # Resource status, enum: Active, Deleted, Suspended
        ResourceRemarks 	string    # Additional remarks on the resource
        ResourceDef 		string    # Resource definition in JSON
        QueryConditions     string    # Query conditions in JSON
        Creator				string    # The user who created the resource
        Updater				string    # The user who last updated the resource
        CreatedAt       	*string   # The record creation time
        UpdatedAt       	*string   # The record last update time
    `
)

func CreateResourcesTable(
	logger ApiTypes.JimoLogger,
	db *sql.DB,
	db_type string,
	table_name string) error {
	logger.Info("Create table", "table_name", table_name)
	var stmt string
	fields_1 := "resource_name 		VARCHAR(128)    NOT NULL, " +
		"resource_opr 		VARCHAR(32)     NOT NULL, " +
		"resource_desc		TEXT		    NOT NULL, " +
		"resource_type		VARCHAR(32)     NOT NULL, " +
		"db_name            VARCHAR(64)     DEFAULT NULL, " +
		"table_name         VARCHAR(64)     DEFAULT NULL, " +
		"resource_status	VARCHAR(32)     NOT NULL, " +
		"resource_remarks   TEXT            DEFAULT NULL, " +
		"error_msg          TEXT            DEFAULT NULL, " +
		"loc                VARCHAR(32)     NOT NULL, "
	fields_2 := "creator			VARCHAR(64)     NOT NULL, " +
		"updater			VARCHAR(64)     NOT NULL, " +
		"updated_at    		TIMESTAMP       DEFAULT CURRENT_TIMESTAMP," +
		"created_at    		TIMESTAMP       DEFAULT CURRENT_TIMESTAMP)"

	switch db_type {
	case ApiTypes.MysqlName:
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" +
			"resource_id  NOT NULL AUTO_INCREMENT PRIMARY KEY, " + fields_1 +
			"resource_def 		JSON     	 	NOT NULL, " +
			"query_conds        JSON     	 	DEFAULT NULL, " + fields_2 +
			"ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;"

	case ApiTypes.PgName:
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" +
			"resource_id BIGSERIAL PRIMARY KEY," + fields_1 +
			"resource_def 		JSONB     	 	NOT NULL, " +
			"query_conds        JSONB     	 	DEFAULT NULL, " + fields_2

	default:
		err := fmt.Errorf("database type not supported:%s (SHD_RSC_117)", db_type)
		log.Printf("***** Alarm:%s", err.Error())
		return err
	}

	err := databaseutil.ExecuteStatement(db, stmt)
	if err != nil {
		error_msg := fmt.Errorf("failed creating table (SHD_RSC_045), err: %w, stmt:%s", err, stmt)
		log.Printf("***** Alarm: %s", error_msg.Error())
		return error_msg
	}

	if db_type == ApiTypes.PgName {
		idx1 := `CREATE INDEX IF NOT EXISTS idx_created_at ON ` + table_name + ` (created_at);`
		databaseutil.ExecuteStatement(db, idx1)
	}

	logger.Info("Create table success", "table_name", table_name)

	return nil
}

func GetResourceStoreTableDesc() string {
	return ResourceStoreTableDescSimple
}

// GetResourceByName retrieves a resource record by resource_name.
// Resources are identified by resource_name and resource_opr.
// Returns error if not found or other errors.
// Otherwise returns the resource record.
func GetResourceByName(rc ApiTypes.RequestContext, resource_name string, resource_action string) (ApiTypes.ResourceDef, error) {
	// This function retrieves a prompt record by prompt_name.
	var query string
	var db *sql.DB = ApiTypes.SharedDBHandle
	db_type := ApiTypes.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameResources
	var resource_info ApiTypes.ResourceDef
	switch db_type {
	case ApiTypes.MysqlName:
		query = fmt.Sprintf("SELECT %s FROM %s WHERE resource_name = ? AND resource_action = ? LIMIT 1",
			resource_store_selected_field_names, table_name)

	case ApiTypes.PgName:
		query = fmt.Sprintf("SELECT %s FROM %s WHERE resource_name = $1 AND resource_action = $2 LIMIT 1",
			resource_store_selected_field_names, table_name)

	default:
		err := fmt.Errorf("unsupported database type (SHD_RSC_326): %s", db_type)
		log.Printf("***** Alarm: %s", err.Error())
		return resource_info, err
	}

	// Query the database for dashboard data
	resource_json_str := sql.NullString{}
	query_conds_json_str := sql.NullString{}
	err := db.QueryRow(query, resource_name, resource_action).Scan(
		&resource_info.ResourceID,
		&resource_info.ResourceDesc,
		&resource_info.ResourceType,
		&resource_info.DBName,
		&resource_info.TableName,
		&resource_info.ResourceStatus,
		&resource_info.ResourceRemarks,
		&resource_json_str,
		&query_conds_json_str,
		&resource_info.ErrorMsg)

	if err != nil {
		error_msg := fmt.Sprintf("database error:%v (SHD_RSC_133)", err)
		log.Printf("%s", error_msg)
		return resource_info, err
	}

	if resource_json_str.Valid {
		err = json.Unmarshal([]byte(resource_json_str.String), &resource_info.ResourceJSON)
		if err != nil {
			error_msg := fmt.Errorf("invalid resource JSON (SHD_RSC_130): %v", err)
			log.Printf("***** Alarm:%s", error_msg)
		}
	}

	if query_conds_json_str.Valid {
		err = json.Unmarshal([]byte(query_conds_json_str.String), &resource_info.QueryCondsJSON)
		if err != nil {
			error_msg := fmt.Errorf("invalid query conditions JSON (SHD_RSC_131): %v", err)
			log.Printf("***** Alarm:%s", error_msg)
		}
	}

	return resource_info, nil
}
