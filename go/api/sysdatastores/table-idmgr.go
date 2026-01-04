package sysdatastores

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/databaseutil"
)

type IDMgrDef struct {
	IDName    string `json:"id_name"`
	CrtValue  int64  `json:"crt_value"`
	IDDesc    string `json:"id_desc"`
	CallerLoc string `json:"caller_loc"`
	UpdatedAt string `json:"updated_at"`
	CreatedAt string `json:"created_at"`
}

const id_mgr_insert_fieldnames = "id_name, crt_value, id_desc, caller_loc"

func CreateIDMgrTable(
	db *sql.DB,
	db_type string,
	table_name string) error {
	var stmt string
	fields :=
		"id_name            VARCHAR(128) NOT NULL PRIMARY KEY, " +
			"crt_value          BIGINT NOT NULL, " +
			"id_desc            TEXT NOT NULL, " +
			"caller_loc         VARCHAR(20) NOT NULL, " +
			"updated_at         TIMESTAMP DEFAULT CURRENT_TIMESTAMP, " +
			"created_at         TIMESTAMP DEFAULT CURRENT_TIMESTAMP"

	switch db_type {
	case ApiTypes.MysqlName:
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" + fields +
			") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;"

	case ApiTypes.PgName:
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" + fields + ")"

	default:
		err := fmt.Errorf("database type not supported:%s (SHD_IMG_117)", db_type)
		log.Printf("***** Alarm:%s", err.Error())
		return err
	}

	err := databaseutil.ExecuteStatement(db, stmt)
	if err != nil {
		error_msg := fmt.Errorf("failed creating table (SHD_IMG_052), err: %w, stmt:%s", err, stmt)
		log.Printf("***** Alarm: %s", error_msg.Error())
		return error_msg
	}

	log.Printf("Create table '%s' success (SHD_IMG_188)", table_name)

	return nil
}

func AddOneID(record IDMgrDef) error {
	var stmt string
	var db *sql.DB
	db_type := ApiTypes.GetDBType()
	table_name := ApiTypes.GetIDMgrTableName()

	switch db_type {
	case ApiTypes.MysqlName:
		stmt = fmt.Sprintf(`INSERT INTO %s (%s)
              	VALUES (?, ?, ?, ?)`, table_name, id_mgr_insert_fieldnames)
		db = ApiTypes.MySql_DB_miner

	case ApiTypes.PgName:
		stmt = fmt.Sprintf(`INSERT INTO %s (%s)
				VALUES ($1, $2, $3, $4)`, table_name, id_mgr_insert_fieldnames)
		db = ApiTypes.PG_DB_miner

	default:
		error_msg := fmt.Sprintf("unsupported database type (SHD_IMG_034): %s", db_type)
		log.Printf("***** Alarm:%s", error_msg)
		return fmt.Errorf("%s", error_msg)
	}

	_, err := db.Exec(stmt,
		record.IDName,
		record.CrtValue,
		record.IDDesc,
		record.CallerLoc)
	if err != nil {
		error_msg := fmt.Sprintf("failed to save activity log (SHD_IMG_047): %v, stmt:%s", err, stmt)
		log.Printf("***** Alarm %s", error_msg)
		return fmt.Errorf("%s", error_msg)
	}
	return nil
}

func UpsertActivityLogIDDef(ctx context.Context) error {
	// 2. Insert a record to id_mgr for activity_log id.
	field_names := "id_name, crt_value, id_desc, caller_loc"
	var stmt string
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameIDMgr
	if table_name == "" {
		call_flow := ctx.Value(ApiTypes.CallFlowKey).(string)
		error_msg := fmt.Sprintf("IDMgr table name is empty (%s->SHD_IMG_200)", call_flow)
		log.Printf("***** Alarm: %s", error_msg)
		return fmt.Errorf("%s", error_msg)
	}

	var db *sql.DB
	switch db_type {
	case ApiTypes.MysqlName:
		stmt = fmt.Sprintf(`INSERT INTO %s (%s) VALUES (?, ?, ?, ?)
              ON DUPLICATE KEY UPDATE id_name = id_name`, table_name, field_names)
		db = ApiTypes.MySql_DB_miner

	case ApiTypes.PgName:
		stmt = fmt.Sprintf(`INSERT INTO %s (%s) VALUES ($1, $2, $3, $4)
            ON CONFLICT (id_name)
            DO NOTHING`, table_name, field_names)
		db = ApiTypes.PG_DB_miner

	default:
		// SHOULD NEVER HAPPEN!!!
		error_msg := fmt.Sprintf("unrecognized db_type:%s (SHD_IMG_033)", db_type)
		log.Printf("***** Alarm: %s", error_msg)
		return fmt.Errorf("%s", error_msg)
	}

	_, err := db.Exec(stmt, "activity_log_id", 10000, "activity_log_id", "SHD_IMG_043")
	if err != nil {
		error_msg := fmt.Sprintf("failed to insert activity_log_id record (SHD_IMG_126): %v, stmt:%s", err, stmt)
		log.Printf("***** Alarm %s", error_msg)
		return fmt.Errorf("%s", error_msg)
	}

	return nil
}

func NextIDBlock(id_name string, inc_size int) (int64, error) {
	// This function retrieves a block of IDs and updates the record.
	// Upon success, it returns the start log ID of the ID block.
	var db *sql.DB
	db_type := ApiTypes.GetDBType()
	table_name := ApiTypes.GetIDMgrTableName()
	var query string

	switch db_type {
	case ApiTypes.MysqlName:
		// Support MySQL 8.0.21+
		db = ApiTypes.MySql_DB_miner
		query = fmt.Sprintf("UPDATE %s SET crt_value = crt_value + ? WHERE id_name = ? RETURNING crt_value", table_name)

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner
		query = fmt.Sprintf("UPDATE %s SET crt_value = crt_value + $1 WHERE id_name = $2 RETURNING crt_value", table_name)

	default:
		error_msg := fmt.Sprintf("unsupported database type (SHD_IMG_034): %s", db_type)
		log.Printf("***** Alarm:%s", error_msg)
		return -1, fmt.Errorf("%s", error_msg)
	}

	tx, err := db.Begin()
	if err != nil {
		error_msg := fmt.Sprintf("failed to start transaction: %v", err)
		log.Printf("***** Alarm:%s", error_msg)
		return 0, fmt.Errorf("%s", error_msg)
	}

	defer func() {
		// Rollback if the function exits with an error (e.g., query failure)
		if tx != nil && err != nil {
			tx.Rollback()
		}
	}()

	// Execute the update and scan the original value
	var originalValue int64
	err = tx.QueryRow(query, inc_size, id_name).Scan(&originalValue)
	if err != nil {
		if err == sql.ErrNoRows {
			error_msg := fmt.Sprintf("record with id_name '%s' not found, stmt:%s (SHD_IMG_135)", id_name, query)
			log.Printf("***** Alarm:%s", error_msg)
			return 0, fmt.Errorf("%s", error_msg)
		}

		error_msg := fmt.Sprintf("failed to update and retrieve: %v, stmt:%s (SHD_IMG_140)", err, query)
		log.Printf("***** Alarm:%s", error_msg)
		return 0, fmt.Errorf("%s", error_msg)
	}

	// Commit the transaction
	if err = tx.Commit(); err != nil {
		error_msg := fmt.Sprintf("failed to commit transaction (SHD_IMG_136): %v", err)
		log.Printf("***** Alarm:%s", error_msg)
		return 0, fmt.Errorf("%s", error_msg)
	}

	return originalValue, nil
}
