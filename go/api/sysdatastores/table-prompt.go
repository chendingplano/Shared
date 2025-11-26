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
	// prompt_store_selected_field_names = 
	// 	"prompt_id, 	 prompt_name,   prompt_desc,     prompt_content, prompt_status, " +
    //     "prompt_purpose, prompt_source, prompt_keywords, prompt_tags,    creator," +
	// 	"updater,        creator_loc,   updater_loc,     created_at,     updated_at"

	prompt_store_insert_field_names = 
		"prompt_id, 	 prompt_name,   prompt_desc,     prompt_content, prompt_status, " +
        "prompt_purpose, prompt_source, prompt_keywords, prompt_tags,    creator," +
		"updater,        creator_loc,   updater_loc"

    PromptStoreTableDescSimple = `
        PromptID        int64     # A unique sequence number
        PromptName 		string    # Prompt name, unique, a string of letters, digts and underscores only
        PromptDesc		string    # Prompt description
        PromptContent	string    # Prompt content
        Status          string    # Prompt status, enum: Active, Deleted, Suspended
        PromptPurpose	string    # Prompt purpose
        PromptSource 	string    # Prompt source, enum: Upload, User Entered, Web Crawled
        PromptKeywords	string    # Keywords for the prompt, used for searching
        PromptTags		string    # Tags for the prompt, used for searching
        Creator			string    # The user who created the prompt
        Updater			string    # The user who last updated the prompt
        CreatorLoc      string    # The caller (program location) that inserts the record
        UpdaterLoc      string    # The caller (program location) that last updates the record
        CreatedAt       *string   # The record creation time
        UpdatedAt       *string   # The record last update time
    `
    
	PromptStoreTableDesc = `
## Title: PromptStore Table
### Summary
This table stores prompts and their metadata

### Metadata
#### PromptID (prompt_id)
This is a 
`
)

type PromptRecordInfo struct {
    PromptID        int64     `json:"prompt_id"`
    PromptName 		string    `json:"prompt_name"`
    PromptDesc		string    `json:"prompt_desc"`
    PromptContent	string    `json:"prompt_content"`
    Status          string    `json:"prompt_status"`
    PromptPurpose	string    `json:"prompt_purpose"`
    PromptSource 	string    `json:"prompt_source"`
    PromptKeywords	string    `json:"prompt_keywords"`
    PromptTags		string    `json:"prompt_tags"`
    Creator			string    `json:"creator"`
    Updater			string    `json:"updater"`
    CreatorLoc      string    `json:"creator_loc"`
    UpdaterLoc      string    `json:"updater_loc"`
    CreatedAt       *string   `json:"created_at"`
    UpdatedAt       *string   `json:"updated_at"`
}

type AddPromptResponse struct {
	Status          bool        `json:"status"`
	ErrorMsg        string      `json:"error_msg"`
	Loc             string      `json:"loc,omitempty"`
}

func CreatePromptStoreTable(
            db *sql.DB, 
            db_type string,
            table_name string) error {
    var stmt string
    fields := fmt.Sprintf("prompt_id       	BIGINT          NOT NULL PRIMARY KEY, " + 
                          "prompt_name 		VARCHAR(256)    NOT NULL, " + 
                          "prompt_desc		VARCHAR(32)     NOT NULL, " + 
                          "prompt_content	VARCHAR(32)     NOT NULL, " + 
                          "prompt_status	VARCHAR(128)    NOT NULL, " + 
                          "prompt_purpose	VARCHAR(128)    NOT NULL, " + 
                          "prompt_source	VARCHAR(32)     NOT NULL, " + 
                          "prompt_keywords	VARCHAR(32)     NOT NULL, " + 
                          "prompt_tags		VARCHAR(32)     NOT NULL, " + 
                          "creator_loc		VARCHAR(32)     NOT NULL, " + 
                          "updater_loc		VARCHAR(32)     NOT NULL, " + 
                          "creator			VARCHAR(32)     NOT NULL, " + 
                          "updater			VARCHAR(32)     NOT NULL, " + 
                          "updated_at     	TIMESTAMP       DEFAULT CURRENT_TIMESTAMP," + 
                          "created_at     	TIMESTAMP       DEFAULT CURRENT_TIMESTAMP")

    switch db_type {
    case ApiTypes.MysqlName:
         stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" + fields +
            ", INDEX idx_created_at (created_at) " +
            ") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;"

    case ApiTypes.PgName:
         stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" + fields + ")"

    default:
        err := fmt.Errorf("database type not supported:%s (SHD_PST_117)", db_type)
        log.Printf("***** Alarm:%s", err.Error())
        return err
    }

    err := databaseutil.ExecuteStatement(db, stmt)
    if err != nil {
        error_msg := fmt.Errorf("failed creating table (SHD_PST_045), err: %w, stmt:%s", err, stmt)
        log.Printf("***** Alarm: %s", error_msg.Error())
        return error_msg
    }

    if db_type == ApiTypes.PgName{
        idx1 := `CREATE INDEX IF NOT EXISTS idx_created_at ON ` + table_name + ` (created_at);`
        databaseutil.ExecuteStatement(db, idx1)
    }

    log.Printf("Create table '%s' success (SHD_PST_188)", table_name)

    return nil
}

func GetPromptStoreTableDesc() string {
    return PromptStoreTableDesc
}

func GetPromptInfoByName(prompt_name string) (PromptRecordInfo, error) {
    // This function retrieves a prompt record by prompt_name.
    var query string
    var db *sql.DB
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableName_PromptStore
    var prompt_info PromptRecordInfo
    switch db_type {
    case ApiTypes.MysqlName:
         query = fmt.Sprintf("SELECT %s FROM %s WHERE prompt_name = ? LIMIT 1", emailstore_selected_field_names, table_name)
         db = ApiTypes.MySql_DB_miner

    case ApiTypes.PgName:
         query = fmt.Sprintf("SELECT %s FROM %s WHERE prompt_name = $1 LIMIT 1", emailstore_selected_field_names, table_name)
         db = ApiTypes.PG_DB_miner

    default:
         err := fmt.Errorf("unsupported database type (SHD_PST_326): %s", db_type)
         log.Printf("***** Alarm: %s", err.Error())
         return prompt_info, err
    }

    err := db.QueryRow(query, prompt_name).Scan(
            &prompt_info.PromptID,
            &prompt_info.PromptName,
            &prompt_info.PromptDesc,
            &prompt_info.Status,
            &prompt_info.PromptPurpose,
            &prompt_info.PromptSource,
            &prompt_info.PromptKeywords,
            &prompt_info.PromptTags,
            &prompt_info.Creator,
            &prompt_info.Updater,
            &prompt_info.CreatorLoc,
            &prompt_info.UpdaterLoc,
            &prompt_info.CreatedAt,
            &prompt_info.UpdatedAt)

    if err != nil {
        err := fmt.Errorf("failed to retrieve prompt, prompt_name:%s, error:%v (SHD_PST_345)", prompt_name, err)
        log.Printf("***** Alarm:%s", err)
        return prompt_info, err
    }
    log.Printf("Prompt info retrieved (SHD_PST_349), prompt_name:%s, status:%s", prompt_info.PromptName, prompt_info.Status)
    return prompt_info, nil
}

func GetPromptStatus(prompt_name string) string {
    var query string
    var db *sql.DB
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableName_PromptStore
    switch db_type {
    case ApiTypes.MysqlName:
         query = fmt.Sprintf("SELECT user_status FROM %s WHERE prompt_name = ? LIMIT 1", table_name)
         db = ApiTypes.MySql_DB_miner

    case ApiTypes.PgName:
         query = fmt.Sprintf("SELECT user_status FROM %s WHERE prompt_name = $1 LIMIT 1", table_name)
         db = ApiTypes.PG_DB_miner

    default:
         err_msg := fmt.Sprintf("error: unsupported database type (SHD_PST_326): %s", db_type)
         log.Printf("***** Alarm: %s", err_msg)
         return err_msg
    }

    var prompt_status string
    err := db.QueryRow(query, prompt_name).Scan(&prompt_status)
    if err != nil {
        if err == sql.ErrNoRows {
            log.Printf("Prompt not found (SHD_PST_443): %s", prompt_name)
            return "prompt not found" // or handle as "not found"
        }

        error_msg := fmt.Sprintf("error: failed to retrieve prompt status, prompt_name:%s, error:%v (SHD_PST_334)", prompt_name, err)
        log.Printf("***** Alarm:%s", error_msg)
        return error_msg
    }
    log.Printf("Retrieved prompt status (SHD_PST_338), db_type:%s, prompt_name:%s, status:%s", db_type, prompt_name, prompt_status)
    return prompt_status
}

func AddPromptFromFrontend(c echo.Context) error {
    user_name, err := middleware.IsAuthenticated(c, "SHD_PST_221")
    if err != nil {
	    log_id := NextActivityLogID()
		error_msg := fmt.Sprintf("auth failed, err:%v, log_id:%d (SHD_PST_224)", err, log_id)
		AddActivityLog(ApiTypes.ActivityLogDef{
            LogID:              log_id,         
			ActivityName: 		ApiTypes.Activity_AddRecord,
			ActivityType: 		ApiTypes.ActivityType_AuthFailure,
			AppName: 			ApiTypes.AppName_SysDataStore,
			ModuleName: 		ApiTypes.ModuleName_PromptStore,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_PST_232"})

        log.Printf("***** Alarm:%s", error_msg)
        resp := AddPromptResponse {
            Status: false,
            ErrorMsg: error_msg,
            Loc: "CWB_PST_239",
        }
		return c.JSON(http.StatusBadRequest, resp)

    }
	r := c.Request()

	log.Printf("AddPromptFromFrontend called (SHD_PST_076)")
	var req PromptRecordInfo
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
	    log_id := NextActivityLogID()
		error_msg := fmt.Sprintf("invalid request body, log_id:%d (SHD_PST_043)", log_id)
		AddActivityLog(ApiTypes.ActivityLogDef{
            LogID:              log_id,         
			ActivityName: 		ApiTypes.Activity_AddRecord,
			ActivityType: 		ApiTypes.ActivityType_BadRequest,
			AppName: 			ApiTypes.AppName_SysDataStore,
			ModuleName: 		ApiTypes.ModuleName_PromptStore,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_PST_119"})

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
    log.Printf("Prompt to add, PromptName:%s, User:%s, creator_loc:%s (SHD_PST_239)", 
        req.PromptName, user_name, req.CreatorLoc)

    prompt_id, err_str := AddPrompt(req)
    if prompt_id > 0 {
        resp := AddPromptResponse {
            Status: true,
            ErrorMsg: "",
            Loc: "SHD_PST_244",
        }

	    log_id := NextActivityLogID()
        msg := fmt.Sprintf("Add prompt success, prompt_id:%d, log_id:%d", prompt_id, log_id)
        log.Printf("%s (SHD_PST_248)", msg)
		AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: 		ApiTypes.Activity_AddRecord,
			ActivityType: 		ApiTypes.ActivityType_RequestSuccess,
			AppName: 			ApiTypes.AppName_SysDataStore,
			ModuleName: 		ApiTypes.ModuleName_PromptStore,
			ActivityMsg: 		&msg,
			CallerLoc: 			"SHD_PST_254"})

		return c.JSON(http.StatusOK, resp)
    }

	log_id := NextActivityLogID()
    error_msg := fmt.Sprintf("failed adding prompt, error:%s, log_id:%d", err_str, log_id)
    log.Printf("***** Alarm:%s (SHD_PST_261)", error_msg)
	AddActivityLog(ApiTypes.ActivityLogDef{
        LogID:              log_id,
		ActivityName: 		ApiTypes.Activity_AddRecord,
		ActivityType: 		ApiTypes.ActivityType_DatabaseError,
		AppName: 			ApiTypes.AppName_SysDataStore,
		ModuleName: 		ApiTypes.ModuleName_PromptStore,
		ActivityMsg: 		&error_msg,
		CallerLoc: 			"SHD_PST_267"})

    resp := AddPromptResponse {
        Status: false,
        ErrorMsg: error_msg,
        Loc: "SHD_PST_274",
    }
    return c.JSON(http.StatusInternalServerError, resp)
}

func AddPrompt(prompt_info PromptRecordInfo) (int64, string) {
    var db *sql.DB
    var stmt string
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableName_PromptStore
    switch db_type {
    case ApiTypes.MysqlName:
         db = ApiTypes.MySql_DB_miner
         stmt = fmt.Sprintf("INSERT INTO %s (%s) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
                    table_name, prompt_store_insert_field_names)

    case ApiTypes.PgName:
         db = ApiTypes.PG_DB_miner
         stmt = fmt.Sprintf("INSERT INTO %s (%s) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)", 
                    table_name, prompt_store_insert_field_names)

    default:
         err := fmt.Errorf("unsupported database type (SHD_PST_313): %s", db_type)
         log.Printf("***** Alarm:%s", err.Error())
         return -1, err.Error()
    }

    prompt_info.PromptID = stores.NextManagedID("prompt_store_id")
    log.Printf("Prompt ID (SHD_PST_306):%d", prompt_info.PromptID)
    _, err := db.Exec(stmt, 
            prompt_info.PromptID,
            prompt_info.PromptName,
            prompt_info.PromptDesc,
            prompt_info.PromptContent,
            prompt_info.Status,
            prompt_info.PromptPurpose,
            prompt_info.PromptSource,
            prompt_info.PromptKeywords,
            prompt_info.PromptTags,
            prompt_info.Creator,
            prompt_info.Updater,
            prompt_info.CreatorLoc,
            prompt_info.UpdaterLoc)

    if err != nil {
        if ApiUtils.IsDuplicateKeyError(err) {
            error_msg := fmt.Sprintf("Prompt name already exists (SHD_PST_649):%s", prompt_info.PromptName)
            log.Printf("%s", error_msg)
            return -1, error_msg
        }

        error_msg := fmt.Sprintf("failed to add prompt (SHD_PST_213): %v, stmt:%s", err, stmt)
        log.Printf("***** Alarm %s", error_msg)
        return -1, error_msg
    }

	log.Printf("Prompt added, prompt_name:%s (SHD_PST_228)", prompt_info.PromptName)
    return prompt_info.PromptID, ""
}