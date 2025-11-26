package sysdatastores

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/databaseutil"
	"github.com/chendingplano/shared/go/api/stores"
	middleware "github.com/chendingplano/shared/go/auth-middleware"
	"github.com/labstack/echo/v4"
)

const (
	resource_store_selected_field_names = 
		"resource_id, 	 	resource_name,      resource_opr,   resource_desc, resource_type, " +
        "resource_status,   resource_remarks, 	resource_def"

	resource_store_insert_field_names = 
		"resource_id, 	 	resource_name,      resource_opr,   resource_desc, resource_type, " +
        "resource_status,   resource_remarks, 	resource_def,   creator,		updater"

    ResourceStoreTableDescSimple = `
        ResourceID        	int64     # A unique sequence number
        ResourceName 		string    # Resource name, identify the resource
        ResourceOpr         string    # Resource opr, identify the operation on the resource
        ResourceDesc		string    # Resource description
        ResourceType		string    # Resource type
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

type ResourceRecordInfo struct {
    ResourceID        	int64     	`json:"resource_id"`
    ResourceName 		string    	`json:"resource_name"`
    ResourceOpr         string    	`json:"resource_opr"`
    ResourceDesc		string    	`json:"resource_desc"`
    ResourceType 		string    	`json:"resource_type"`
    ResourceStatus      string    	`json:"resource_status"`
    ResourceRemarks 	string    	`json:"resource_remarks"`
	ResourceDef 		interface{}	`json:"resource_def"`
	QueryConditions     interface{}	`json:"query_conditions"`
    Creator				string    	`json:"creator"`
    Updater				string    	`json:"updater"`
    CreatedAt       	*string   	`json:"created_at"`
    UpdatedAt       	*string   	`json:"updated_at"`
}

func CreateResourcesTable(
            db *sql.DB, 
            db_type string,
            table_name string) error {
    var stmt string
    fields_1 := "resource_id       	BIGINT          NOT NULL PRIMARY KEY, " + 
                "resource_name 		VARCHAR(128)    NOT NULL, " + 
                "resource_opr 		VARCHAR(32)     NOT NULL, " + 
                "resource_desc		TEXT		    NOT NULL, " + 
                "resource_type		VARCHAR(32)     NOT NULL, " + 
                "resource_status	VARCHAR(32)     NOT NULL, "
	fields_2 := "creator			VARCHAR(64)     NOT NULL, " + 
                "updater			VARCHAR(64)     NOT NULL, " + 
                "updated_at    		TIMESTAMP       DEFAULT CURRENT_TIMESTAMP," + 
                "created_at    		TIMESTAMP       DEFAULT CURRENT_TIMESTAMP)"

    switch db_type {
    case ApiTypes.MysqlName:
         stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" + fields_1 +
				"resource_def 		JSON     	 	NOT NULL, " +
				"query_conditions   JSON     	 	NOT NULL, " + fields_2 +
            	"ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;"

    case ApiTypes.PgName:
         stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" + fields_1 + 
				"resource_def 		JSONB     	 	NOT NULL, " +
				"query_conditions   JSONB     	 	DEFAULT NULL, " + fields_2

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

    if db_type == ApiTypes.PgName{
        idx1 := `CREATE INDEX IF NOT EXISTS idx_created_at ON ` + table_name + ` (created_at);`
        databaseutil.ExecuteStatement(db, idx1)
    }

    log.Printf("Create table '%s' success (SHD_RSC_188)", table_name)

    return nil
}

func GetResourceStoreTableDesc() string {
    return ResourceStoreTableDescSimple
}

func GetResourceByName(resource_name string, resource_opr string) (ResourceRecordInfo, error) {
    // This function retrieves a prompt record by prompt_name.
    var query string
    var db *sql.DB
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableName_Resources
    var resource_info ResourceRecordInfo
    switch db_type {
    case ApiTypes.MysqlName:
         query = fmt.Sprintf("SELECT %s FROM %s WHERE resource_name = ? AND resource_opr = ? LIMIT 1", 
                    resource_store_selected_field_names, table_name)
         db = ApiTypes.MySql_DB_miner

    case ApiTypes.PgName:
         query = fmt.Sprintf("SELECT %s FROM %s WHERE resource_name = $1 AND resource_opr = $2 LIMIT 1", 
                    resource_store_selected_field_names, table_name)
         db = ApiTypes.PG_DB_miner

    default:
         err := fmt.Errorf("unsupported database type (SHD_RSC_326): %s", db_type)
         log.Printf("***** Alarm: %s", err.Error())
         return resource_info, err
    }

	// Query the database for dashboard data
	err := db.QueryRow(query, resource_name).Scan(
        &resource_info.ResourceID,
        &resource_info.ResourceName,
        &resource_info.ResourceOpr,
        &resource_info.ResourceDesc,
        &resource_info.ResourceType,
        &resource_info.ResourceStatus,
        &resource_info.ResourceRemarks,
        &resource_info.ResourceDef)

	if err != nil {
		error_msg := fmt.Sprintf("database error:%v (SHD_RSC_133)", err)
		log.Printf("%s", error_msg)
		return resource_info, err
	}

    return resource_info, nil
}

func AddResourceFromFrontend(c echo.Context) error {
    user_name, err := middleware.IsAuthenticated(c, "SHD_PSC_221")
    if err != nil {
	    log_id := NextActivityLogID()
		error_msg := fmt.Sprintf("auth failed, err:%v, log_id:%d (SHD_RSC_224)", err, log_id)
		AddActivityLog(ApiTypes.ActivityLogDef{
            LogID:              log_id,         
			ActivityName: 		ApiTypes.Activity_AddRecord,
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
	var req ResourceRecordInfo
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
	    log_id := NextActivityLogID()
		error_msg := fmt.Sprintf("invalid request body, log_id:%d (SHD_RSC_043)", log_id)
		AddActivityLog(ApiTypes.ActivityLogDef{
            LogID:              log_id,         
			ActivityName: 		ApiTypes.Activity_AddRecord,
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

    req.Creator = user_name
    req.Updater = user_name
    log.Printf("Prompt to add, PromptName:%s, User:%s (SHD_RSC_239)", 
        req.ResourceName, user_name)

    prompt_id, err_str := AddResource(req)
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
			ActivityName: 		ApiTypes.Activity_AddRecord,
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
		ActivityName: 		ApiTypes.Activity_AddRecord,
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

func AddResource(resource_info ResourceRecordInfo) (int64, string) {
    var db *sql.DB
    var stmt string
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableName_Resources
    switch db_type {
    case ApiTypes.MysqlName:
         db = ApiTypes.MySql_DB_miner
         stmt = fmt.Sprintf("INSERT INTO %s (%s) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
                    table_name, resource_store_insert_field_names)

    case ApiTypes.PgName:
         db = ApiTypes.PG_DB_miner
         stmt = fmt.Sprintf("INSERT INTO %s (%s) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)", 
                    table_name, resource_store_insert_field_names)

    default:
         err := fmt.Errorf("unsupported database type (SHD_RSC_313): %s", db_type)
         log.Printf("***** Alarm:%s", err.Error())
         return -1, err.Error()
    }

    resource_info.ResourceID = stores.NextManagedID("prompt_store_id")
    log.Printf("Resource ID (SHD_RSC_306):%d", resource_info.ResourceID)
    _, err := db.Exec(stmt, 
            resource_info.ResourceID,
            resource_info.ResourceName,
            resource_info.ResourceOpr,
            resource_info.ResourceDesc,
            resource_info.ResourceType,
            resource_info.ResourceStatus,
            resource_info.ResourceRemarks,
            resource_info.ResourceDef,
            resource_info.Creator,
            resource_info.Updater)

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