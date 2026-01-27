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
	var db *sql.DB
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameResources
	var resource_info ApiTypes.ResourceDef
	switch db_type {
	case ApiTypes.MysqlName:
		query = fmt.Sprintf("SELECT %s FROM %s WHERE resource_name = ? AND resource_action = ? LIMIT 1",
			resource_store_selected_field_names, table_name)
		db = ApiTypes.MySql_DB_miner

	case ApiTypes.PgName:
		query = fmt.Sprintf("SELECT %s FROM %s WHERE resource_name = $1 AND resource_action = $2 LIMIT 1",
			resource_store_selected_field_names, table_name)
		db = ApiTypes.PG_DB_miner

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

/*
func AddResourceFromFrontend(c echo.Context) error {
    user_info := middleware.IsAuthenticated(c, "SHD_PSC_221")
    if user_info == nil {
	    log_id := NextActivityLogID()
		error_msg := fmt.Sprintf("auth failed, err:%v, log_id:%d (SHD_RSC_224)", err, log_id)
		AddActivityLog(ApiTypes.ActivityLogDef{
            LogID:              log_id,
			ActivityName: 		ApiTypes.ActivityName_AddRecord,
			ActivityType: 		ApiTypes.ActivityType_AuthFailure,
			AppName: 			ApiTypes.AppName_SysDataStore,
			ModuleName: 		ApiTypes.ModuleName_ResourceStore,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_RSC_232"})

        log.Printf("***** Alarm:%s", error_msg)
        resp := ApiTypes.JimoResponse{
            Status: false,
            ErrorMsg: error_msg,
            Loc: "CWB_PST_239",
        }
		return c.JSON(http.StatusBadRequest, resp)

    }
	r := c.Request()

	log.Printf("AddResourceFromFrontend called (SHD_RSC_076)")
	var req ApiTypes.ResourceDef
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
	    log_id := NextActivityLogID()
		error_msg := fmt.Sprintf("invalid request body, log_id:%d (SHD_RSC_043)", log_id)
		AddActivityLog(ApiTypes.ActivityLogDef{
            LogID:              log_id,
			ActivityName: 		ApiTypes.ActivityName_AddRecord,
			ActivityType: 		ApiTypes.ActivityType_BadRequest,
			AppName: 			ApiTypes.AppName_SysDataStore,
			ModuleName: 		ApiTypes.ModuleName_ResourceStore,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_RSC_119"})

        log.Printf("***** Alarm:%s", error_msg)
        resp := AddPromptResponse {
            Status: false,
            ErrorMsg: error_msg,
            Loc: "CWB_PST_239",
        }
		return c.JSON(http.StatusBadRequest, resp)
	}

    log.Printf("Prompt to add, PromptName:%s, User:%s (SHD_RSC_239)",
        req.ResourceName, user_info.UserName)

    prompt_id, err_str := AddResource(req, user_info.UserName,
            user_info.UserName, "SHD_RSC_231")
    if prompt_id > 0 {
        resp := AddPromptResponse {
            Status: true,
            ErrorMsg: "",
            Loc: "SHD_RSC_244",
        }

	    log_id := NextActivityLogID()
        msg := fmt.Sprintf("Add prompt success, prompt_id:%d, log_id:%d", prompt_id, log_id)
        log.Printf("%s (SHD_RSC_248)", msg)
		AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: 		ApiTypes.ActivityName_AddRecord,
			ActivityType: 		ApiTypes.ActivityType_RequestSuccess,
			AppName: 			ApiTypes.AppName_SysDataStore,
			ModuleName: 		ApiTypes.ModuleName_ResourceStore,
			ActivityMsg: 		&msg,
			CallerLoc: 			"SHD_RSC_254"})

		return c.JSON(http.StatusOK, resp)
    }

	log_id := NextActivityLogID()
    error_msg := fmt.Sprintf("failed adding prompt, error:%s, log_id:%d", err_str, log_id)
    log.Printf("***** Alarm:%s (SHD_RSC_261)", error_msg)
	AddActivityLog(ApiTypes.ActivityLogDef{
        LogID:              log_id,
		ActivityName: 		ApiTypes.ActivityName_AddRecord,
		ActivityType: 		ApiTypes.ActivityType_DatabaseError,
		AppName: 			ApiTypes.AppName_SysDataStore,
		ModuleName: 		ApiTypes.ModuleName_ResourceStore,
		ActivityMsg: 		&error_msg,
		CallerLoc: 			"SHD_RSC_267"})

    resp := AddPromptResponse {
        Status: false,
        ErrorMsg: error_msg,
        Loc: "SHD_RSC_274",
    }
    return c.JSON(http.StatusInternalServerError, resp)
}

func AddResource(
        resource_info ApiTypes.ResourceDef,
        creator string,
        updater string,
        loc string) (int64, string) {
    var db *sql.DB
    var stmt string
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameResources
    switch db_type {
    case ApiTypes.MysqlName:
         db = ApiTypes.MySql_DB_miner
         stmt = fmt.Sprintf("INSERT INTO %s (%s) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
                    table_name, resource_store_insert_field_names)

    case ApiTypes.PgName:
         db = ApiTypes.PG_DB_miner
         stmt = fmt.Sprintf("INSERT INTO %s (%s) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)",
                    table_name, resource_store_insert_field_names)

    default:
         err := fmt.Errorf("unsupported database type (SHD_RSC_313): %s", db_type)
         log.Printf("***** Alarm:%s", err.Error())
         return -1, err.Error()
    }

    log.Printf("Resource ID (SHD_RSC_306):%d", resource_info.ResourceID)
    resource_json, _ := json.Marshal(resource_info.ResourceJSON)
    query_conds_json, _ := json.Marshal(resource_info.QueryCondsJSON)
    _, err := db.Exec(stmt,
            resource_info.ResourceName,
            resource_info.ResourceAction,
            resource_info.ResourceDesc,
            resource_info.ResourceType,
            resource_info.DBName,
            resource_info.TableName,
            resource_info.ResourceStatus,
            resource_info.ResourceRemarks,
            string(resource_json),
            string(query_conds_json),
            resource_info.ErrorMsg,
            creator,
            updater,
            loc)

    if err != nil {
        if ApiUtils.IsDuplicateKeyError(err) {
            error_msg := fmt.Sprintf("Prompt name already exists (SHD_RSC_649):%s", resource_info.ResourceName)
            log.Printf("%s", error_msg)
            return -1, error_msg
        }

        error_msg := fmt.Sprintf("failed to add prompt (SHD_RSC_213): %v, stmt:%s", err, stmt)
        log.Printf("***** Alarm %s", error_msg)
        return -1, error_msg
    }

	log.Printf("Resource added, resource_name:%s (SHD_RSC_228)", resource_info.ResourceName)
    return resource_info.ResourceID, ""
}
*/
