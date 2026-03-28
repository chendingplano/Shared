package sysdatastores

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/databaseutil"
)

const (
	// prompt_store_selected_field_names =
	// 	"prompt_id, 	 prompt_name,   prompt_desc,     prompt_content, prompt_status, " +
	//     "prompt_purpose, prompt_source, prompt_keywords, prompt_tags,    creator," +
	// 	"updater,        created_at,     updated_at"

	// prompt_store_insert_field_names = "prompt_id, 	 prompt_name,   prompt_desc,     prompt_content, prompt_status, " +
	// 	"prompt_purpose, prompt_source, prompt_keywords, prompt_tags,    creator," +
	// 	"updater"

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
	PromptID       int64   `json:"prompt_id"`
	PromptName     string  `json:"prompt_name"`
	PromptDesc     string  `json:"prompt_desc"`
	PromptContent  string  `json:"prompt_content"`
	Status         string  `json:"prompt_status"`
	PromptPurpose  string  `json:"prompt_purpose"`
	PromptSource   *string `json:"prompt_source"`
	PromptKeywords *string `json:"prompt_keywords"`
	PromptTags     *string `json:"prompt_tags"`
	Creator        string  `json:"creator"`
	Updater        string  `json:"updater"`
	CreatedAt      *string `json:"created_at"`
	UpdatedAt      *string `json:"updated_at"`
}

type AddPromptResponse struct {
	Status   bool   `json:"status"`
	ErrorMsg string `json:"error_msg"`
	Loc      string `json:"loc,omitempty"`
}

func CreatePromptStoreTable(
	logger ApiTypes.JimoLogger,
	db *sql.DB,
	db_type string,
	table_name string) error {
	logger.Info("Create table", "table_name", table_name)
	var stmt string
	fields_1 := fmt.Sprintf(
		"prompt_name 		VARCHAR(256)    NOT NULL, " +
			"prompt_desc		TEXT            DEFAULT NULL, " +
			"prompt_content	TEXT            NOT NULL, " +
			"prompt_status	VARCHAR(128)    NOT NULL, " +
			"prompt_source	VARCHAR(32)     DEFAULT NULL, " +
			"prompt_keywords	VARCHAR(32)     DEFAULT NULL, ")

	fields_2 := fmt.Sprintf(
		"creator			VARCHAR(32)     NOT NULL, " +
			"updater			VARCHAR(32)     NOT NULL, " +
			"updated_at     	TIMESTAMP       DEFAULT CURRENT_TIMESTAMP," +
			"created_at     	TIMESTAMP       DEFAULT CURRENT_TIMESTAMP")

	switch db_type {
	case ApiTypes.MysqlName:
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" +
			"prompt_id NOT NULL AUTO_INCREMENT PRIMARY KEY, " + fields_1 +
			"prompt_purpose VARCHAR(256) DEFAULT NULL, " +
			"prompt_tags VARCHAR(256) DEFAULT NULL, " + fields_2 +
			", INDEX idx_created_at (created_at) " +
			") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;"

	case ApiTypes.PgName:
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" +
			"prompt_id BIGSERIAL PRIMARY KEY, " + fields_1 +
			"prompt_purpose VARCHAR(40)[] DEFAULT NULL, " +
			"prompt_tags VARCHAR(50)[] DEFAULT NULL, " + fields_2 + ")"

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

	if db_type == ApiTypes.PgName {
		idx1 := `CREATE INDEX IF NOT EXISTS idx_created_at ON ` + table_name + ` (created_at);`
		databaseutil.ExecuteStatement(db, idx1)
	}

	logger.Info("Create table success", "table_name", table_name)

	return nil
}

func GetPromptStoreTableDesc() string {
	return PromptStoreTableDesc
}

func GetPromptInfoByName(rc ApiTypes.RequestContext, prompt_name string) (PromptRecordInfo, error) {
	// This function retrieves a prompt record by prompt_name.
	var query string
	var db *sql.DB = ApiTypes.SharedDBHandle
	db_type := ApiTypes.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNamePromptStore
	var prompt_info PromptRecordInfo
	switch db_type {
	case ApiTypes.MysqlName:
		query = fmt.Sprintf("SELECT %s FROM %s WHERE prompt_name = ? LIMIT 1", emailstore_selected_field_names, table_name)

	case ApiTypes.PgName:
		query = fmt.Sprintf("SELECT %s FROM %s WHERE prompt_name = $1 LIMIT 1", emailstore_selected_field_names, table_name)

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

func GetPromptStatus(rc ApiTypes.RequestContext, prompt_name string) string {
	var query string
	var db *sql.DB = ApiTypes.SharedDBHandle
	db_type := ApiTypes.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNamePromptStore
	switch db_type {
	case ApiTypes.MysqlName:
		query = fmt.Sprintf("SELECT user_status FROM %s WHERE prompt_name = ? LIMIT 1", table_name)

	case ApiTypes.PgName:
		query = fmt.Sprintf("SELECT user_status FROM %s WHERE prompt_name = $1 LIMIT 1", table_name)

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
