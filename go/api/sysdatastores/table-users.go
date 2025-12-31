package sysdatastores

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/databaseutil"
)

var Users_selected_field_names = "user_id, user_name, user_password, user_id_type, firstName, lastName, " +
	"email, user_mobile, user_address, user_type, user_status, avatar, " +
	"locale, v_token"

var Users_insert_field_names = "user_id, user_name, user_password, user_id_type, firstName, lastName, " +
	"email, user_mobile, user_address, user_type, user_status, avatar, " +
	"locale, v_token"

func CreateUsersTable(
	db *sql.DB,
	db_type string,
	table_name string) error {
	var stmt string
	fields := "user_id      VARCHAR(128) 	DEFAULT NULL, " +
		"user_name      	VARCHAR(128) 	NOT NULL PRIMARY KEY, " +
		"user_password  	VARCHAR(128) 	DEFAULT NULL, " +
		"user_id_type   	VARCHAR(32)  	DEFAULT NULL, " +
		"firstName      	VARCHAR(128) 	DEFAULT NULL, " +
		"lastName       	VARCHAR(128) 	DEFAULT NULL, " +
		"email          	VARCHAR(255) 	NOT NULL, " +
		"user_mobile    	VARCHAR(64) 	DEFAULT NULL, " +
		"user_address   	TEXT 			DEFAULT NULL, " +
		"verified       	bool 			DEFAULT false, " +
		"is_admin        	bool 			DEFAULT false, " +
		"emailVisibility 	bool 			DEFAULT true, " +
		"user_type      	VARCHAR(32) 	NOT NULL, " +
		"user_status    	VARCHAR(32) 	NOT NULL, " +
		"avatar         	text DEFAULT 	NULL, " +
		"locale         	VARCHAR(128) 	DEFAULT NULL, " +
		"userToken      	VARCHAR(128) 	DEFAULT NULL, " +
		"created        	TIMESTAMP 		DEFAULT CURRENT_TIMESTAMP, " +
		"updated        	TIMESTAMP 		DEFAULT CURRENT_TIMESTAMP "

	switch db_type {
	case ApiTypes.MysqlName:
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" + fields +
			", INDEX idx_created_at (created_at) " +
			") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;"

	case ApiTypes.PgName:
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" + fields + ")"

	default:
		err := fmt.Errorf("database type not supported:%s (SHD_USR_117)", db_type)
		log.Printf("***** Alarm:%s", err.Error())
		return err
	}

	err := databaseutil.ExecuteStatement(db, stmt)
	if err != nil {
		error_msg := fmt.Errorf("failed creating table (SHD_USR_045), err: %w, stmt:%s", err, stmt)
		log.Printf("***** Alarm: %s", error_msg.Error())
		return error_msg
	}

	if db_type == ApiTypes.PgName {
		idx1 := `CREATE INDEX IF NOT EXISTS idx_created_at ON ` + table_name + ` (created_at);`
		databaseutil.ExecuteStatement(db, idx1)
	}

	log.Printf("Create table '%s' success (SHD_USR_188)", table_name)

	return nil
}

/*
func UserExists(user_name string) bool {
	// This function checks whether 'user_name' is used in the users table.
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameUsers
	var query string
	var db *sql.DB
	switch db_type {
	case ApiTypes.MysqlName:
		query = fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE user_name = ?", table_name)
		db = ApiTypes.MySql_DB_miner

	case ApiTypes.PgName:
		query = fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE user_name = $1", table_name)
		db = ApiTypes.PG_DB_miner

	default:
		err := fmt.Errorf("unsupported database type (SHD_USR_153): %s", db_type)
		log.Printf("[req=%s] ***** Alarm: %s", err.Error())
		return false
	}

	var count int
	err := db.QueryRow(query, user_name).Scan(&count)
	if err != nil {
		error_msg := fmt.Errorf("failed to validate session (SHD_USR_288): %w", err)
		log.Printf("[req=%s] ***** Alarm:%s", error_msg)
		return false
	}
	log.Printf("[req=%s] Check user name (SHD_USR_292), stmt: %s, count:%d", query, count)
	return count > 0
}
*/

func GetUserInfoByEmail(reqID string, user_email string) (ApiTypes.UserInfo, error) {
	// This function checks whether 'user_email' is used in the users table.
	var query string
	var db *sql.DB
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameUsers
	var user_info ApiTypes.UserInfo
	switch db_type {
	case ApiTypes.MysqlName:
		query = fmt.Sprintf("SELECT %s FROM %s WHERE email = ? LIMIT 1", Users_selected_field_names, table_name)
		db = ApiTypes.MySql_DB_miner

	case ApiTypes.PgName:
		query = fmt.Sprintf("SELECT %s FROM %s WHERE email = $1 LIMIT 1", Users_selected_field_names, table_name)
		db = ApiTypes.PG_DB_miner

	default:
		err := fmt.Errorf("unsupported database type (SHD_USR_326): %s", db_type)
		log.Printf("[req=%s] ***** Alarm: %s", reqID, err.Error())
		return user_info, err
	}

	err := db.QueryRow(query, user_email).Scan(
		&user_info.UserId,
		&user_info.UserName,
		&user_info.Password,
		&user_info.UserIdType,
		&user_info.FirstName,
		&user_info.LastName,
		&user_info.Email,
		&user_info.UserMobile,
		&user_info.UserAddress,
		&user_info.AuthType,
		&user_info.UserStatus,
		&user_info.Avatar,
		&user_info.Locale,
		&user_info.VToken)
	if err != nil {
		return user_info, err
	}
	log.Printf("[req=%s] User info retrieved (SHD_USR_349), user: %s, status:%s",
		reqID, user_info.UserName, user_info.UserStatus)
	return user_info, nil
}

func GetUserInfoByUserName(reqID string, user_name string) (ApiTypes.UserInfo, error) {
	// This function checks whether 'user_email' is used in the users table.
	var query string
	var db *sql.DB
	var user_info ApiTypes.UserInfo
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameUsers

	switch db_type {
	case ApiTypes.MysqlName:
		query = fmt.Sprintf("SELECT %s FROM %s WHERE user_name= ? LIMIT 1", Users_selected_field_names, table_name)
		db = ApiTypes.MySql_DB_miner

	case ApiTypes.PgName:
		query = fmt.Sprintf("SELECT %s FROM %s WHERE user_name= $1 LIMIT 1", Users_selected_field_names, table_name)
		db = ApiTypes.PG_DB_miner

	default:
		err := fmt.Errorf("unsupported database type (SHD_USR_443): %s", db_type)
		log.Printf("[req=%s] ***** Alarm: %s", reqID, err.Error())
		return user_info, err
	}

	err := db.QueryRow(query, user_name).Scan(
		&user_info.UserId,
		&user_info.UserName,
		&user_info.Password,
		&user_info.UserIdType,
		&user_info.FirstName,
		&user_info.LastName,
		&user_info.Email,
		&user_info.UserMobile,
		&user_info.UserAddress,
		&user_info.AuthType,
		&user_info.UserStatus,
		&user_info.Avatar,
		&user_info.Locale,
		&user_info.VToken)
	if err != nil {
		err := fmt.Errorf("failed to retrieve user info (SHD_USR_459): %w, query:%s, user_name:%s, table:%s:%s",
			err, query, user_name, db_type, table_name)
		log.Printf("[req=%s] ***** Alarm:%s", reqID, err)
		return user_info, err
	}
	log.Printf("[req=%s] User info retrieved (SHD_USR_463), user: %s, status:%s",
		reqID, user_info.UserName, user_info.UserStatus)
	return user_info, nil
}

/*
func GetUserStatusByEmail(reqID string, user_email string) (string, error) {
	// This function checks whether 'user_email' is used in the users table.
	var query string
	var db *sql.DB
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameUsers
	switch db_type {
	case ApiTypes.MysqlName:
		query = fmt.Sprintf("SELECT user_status FROM %s WHERE email = ? LIMIT 1", table_name)
		db = ApiTypes.MySql_DB_miner

	case ApiTypes.PgName:
		query = fmt.Sprintf("SELECT user_status FROM %s WHERE email = $1 LIMIT 1", table_name)
		db = ApiTypes.PG_DB_miner

	default:
		err_msg := fmt.Sprintf("error: unsupported database type (SHD_USR_326): %s", db_type)
		log.Printf("[req=%s] ***** Alarm: %s", err_msg)
		return "", fmt.Errorf("%s", err_msg)
	}

	var user_status string
	err := db.QueryRow(query, user_email).Scan(&user_status)
	if err != nil {
		if err == sql.ErrNoRows {
			// No user found with that user_name
			error_msg := fmt.Sprintf("User not found (SHD_USR_443): user_email:%s", user_email)
			log.Printf("[req=%s] +++++ Warning:%s", user_email)
			return "", fmt.Errorf("%s", error_msg)
		}

		error_msg := fmt.Sprintf("failed to retrieve user status (SHD_USR_334): %v, email:%s", err, user_email)
		log.Printf("[req=%s] ***** Alarm:%s", error_msg)
		return "", fmt.Errorf("%s", error_msg)
	}
	log.Printf("[req=%s] User status (SHD_USR_338), db_type:%s, email:%s, status:%s", db_type, user_email, user_status)
	return user_status, nil
}

func GetUserNameAndPasswordByEmail(user_email string) (string, *string, string) {
	// This function retrieves user name and password by email. If the user does not exist,
	// it returns "", "". If user status is not 'active', returns "", "".
	// Otherwise, it returns user name and password.
	var query string
	var db *sql.DB
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameUsers

	switch db_type {
	case ApiTypes.MysqlName:
		query = fmt.Sprintf("SELECT user_name, user_status, user_password FROM %s WHERE email = ? LIMIT 1", table_name)
		db = ApiTypes.MySql_DB_miner

	case ApiTypes.PgName:
		query = fmt.Sprintf("SELECT user_name, user_status, user_password FROM %s WHERE email = $1 LIMIT 1", table_name)
		db = ApiTypes.PG_DB_miner

	default:
		err := fmt.Errorf("error: unsupported database type (SHD_USR_358): %s", db_type)
		log.Printf("[req=%s] ***** Alarm: %s", err.Error())
		return "", nil, err.Error()
	}

	var user_name, status string
	var password_out *string
	err := db.QueryRow(query, user_email).Scan(&user_name, &status, &password_out)
	if err != nil {
		e_msg := err.Error()
		if strings.HasPrefix(e_msg, "sql: no rows") {
			log.Printf("[req=%s] User not found (SHD_USR_478), email: %s", user_email)
			return "", nil, "user not found (SHD_USR_478)"
		}

		error_msg := fmt.Errorf("error: failed to retrieve user info (SHD_USR_475): %w", err)
		log.Printf("[req=%s] ***** Alarm:%s", error_msg)
		return "", nil, error_msg.Error()
	}

	if status == "" {
		log.Printf("[req=%s] User status empty (SHD_USR_481), email: %s", user_email)
		return "", nil, "user not found (SHD_USR_482)"
	}

	if status == "pending_verify" {
		log.Printf("[req=%s] User not verified (SHD_USR_478), email: %s, status:%s", user_email, status)
		err_msg := "user pending verify (SHD_USR_479)"
		return "", nil, err_msg
	}

	if status != "active" {
		log.Printf("[req=%s] invalid user (SHD_USR_480), email: %s, status:%s", user_email, status)
		err_msg := fmt.Sprintf("invalid user, status:%s (SHD_USR_483)", status)
		return "", nil, err_msg
	}

	log.Printf("[req=%s] User password retrieved (SHD_USR_370), email: %s", user_email)
	return user_name, password_out, ""
}
*/

func AddUserNew(reqID string, user_info ApiTypes.UserInfo) (bool, error) {
	var db *sql.DB
	var stmt string
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameUsers
	switch db_type {
	case ApiTypes.MysqlName:
		db = ApiTypes.MySql_DB_miner
		stmt = fmt.Sprintf("INSERT INTO %s (%s) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			table_name, Users_insert_field_names)

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner
		stmt = fmt.Sprintf("INSERT INTO %s (%s) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)",
			table_name, Users_insert_field_names)

	default:
		err := fmt.Errorf("unsupported database type (SHD_USR_313): %s", db_type)
		log.Printf("[req=%s] ***** Alarm:%s", reqID, err.Error())
		return false, err
	}

	_, err := db.Exec(stmt,
		user_info.UserId,
		user_info.UserName,
		user_info.Password,
		user_info.UserIdType,
		user_info.FirstName,
		user_info.LastName,
		user_info.Email,
		user_info.UserMobile,
		user_info.UserAddress,
		user_info.AuthType,
		user_info.UserStatus,
		user_info.Avatar,
		user_info.Locale,
		user_info.VToken)

	if err != nil {
		if ApiUtils.IsDuplicateKeyError(err) {
			log.Printf("[req=%s] User already exists (SHD_USR_649), user_name:%s, email:%s",
				reqID, user_info.UserName, user_info.Email)
			return true, nil
		}

		error_msg := fmt.Sprintf("failed to add user (SHD_USR_213): %v, stmt:%s", err, stmt)
		log.Printf("[req=%s] ***** Alarm %s", reqID, error_msg)
		return false, fmt.Errorf("***** Alarm:%s", error_msg)
	}

	return true, nil
}

func LookupUserByToken(reqID string, token string) (ApiTypes.UserInfo, error) {
	var db *sql.DB
	var query string
	var user_info ApiTypes.UserInfo
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameUsers
	switch db_type {
	case ApiTypes.MysqlName:
		db = ApiTypes.MySql_DB_miner
		query = fmt.Sprintf("SELECT user_name, email FROM %s WHERE v_token = ? LIMIT 1", table_name)

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner
		query = fmt.Sprintf("SELECT user_name, email FROM %s WHERE v_token = $1 LIMIT 1", table_name)

	default:
		err := fmt.Errorf("unsupported database type (SHD_USR_474): %s", db_type)
		log.Printf("[req=%s] ***** Alarm:%s", reqID, err.Error())
		return user_info, err
	}

	err := db.QueryRow(query, token).Scan(&user_info.UserName, &user_info.Email)
	if err != nil {
		error_msg := fmt.Errorf("failed to retrieve user by token (SHD_USR_566), token:%s, err: %w, tablename:%s:%s",
			token, err, db_type, table_name)
		// log.Printf("[req=%s] %s", error_msg.Error())
		return user_info, error_msg
	}
	log.Printf("[req=%s] Lookup User by Token success (SHD_USR_382), token:%s, user_name:%s, email:%s",
		reqID, token, user_info.UserName, user_info.Email)
	return user_info, nil
}

func MarkUserVerified(reqID string, user_name string) error {
	var db *sql.DB
	var stmt string
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameUsers
	switch db_type {
	case ApiTypes.MysqlName:
		db = ApiTypes.MySql_DB_miner
		stmt = fmt.Sprintf("UPDATE %s SET user_status = ? WHERE user_name = ?", table_name)

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner
		stmt = fmt.Sprintf("UPDATE %s SET user_status = $1 WHERE user_name = $2", table_name)

	default:
		err := fmt.Errorf("unsupported database type (SHD_USR_401): %s", db_type)
		log.Printf("[req=%s] ***** Alarm:%s", reqID, err.Error())
		return err
	}

	_, err := db.Exec(stmt, "active", user_name)
	if err != nil {
		error_msg := fmt.Errorf("failed to update table (SHD_USR_404), stmt:%s, err: %w", stmt, err)
		log.Printf("[req=%s] ***** Alarm:%s", reqID, error_msg.Error())
		return error_msg
	}
	log.Printf("[req=%s] Mark user verified success (SHD_USR_408), user_name:%s", reqID, user_name)
	return nil
}

func UpdatePasswordByUserName(reqID string, user_name string, password string) error {
	var db *sql.DB
	var stmt string
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameUsers
	switch db_type {
	case ApiTypes.MysqlName:
		db = ApiTypes.MySql_DB_miner
		stmt = fmt.Sprintf("UPDATE %s SET user_password = ?, user_status = 'active' WHERE user_name = ?", table_name)

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner
		stmt = fmt.Sprintf("UPDATE %s SET user_password = $1, user_status = 'active' WHERE user_name = $2", table_name)

	default:
		err := fmt.Errorf("unsupported database type (SHD_USR_565): %s", db_type)
		log.Printf("[req=%s] ***** Alarm:%s", reqID, err.Error())
		return err
	}

	_, err := db.Exec(stmt, password, user_name)
	if err != nil {
		error_msg := fmt.Errorf("failed to update password (SHD_USR_572), stmt:%s, err: %w", stmt, err)
		log.Printf("[req=%s] ***** Alarm:%s", reqID, error_msg.Error())
		return error_msg
	}
	log.Printf("[req=%s] Update password success (SHD_USR_576), user_name:%s", reqID, user_name)
	return nil
}

func UpdatePasswordByEmail(reqID string, email string, password string) error {
	var db *sql.DB
	var stmt string
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameUsers
	switch db_type {
	case ApiTypes.MysqlName:
		db = ApiTypes.MySql_DB_miner
		stmt = fmt.Sprintf("UPDATE %s SET user_password = ?, user_status = 'active' WHERE email = ?", table_name)

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner
		stmt = fmt.Sprintf("UPDATE %s SET user_password = $1, user_status = 'active' WHERE email = $2", table_name)

	default:
		err := fmt.Errorf("unsupported database type (SHD_USR_565): %s", db_type)
		log.Printf("[req=%s] ***** Alarm:%s", reqID, err.Error())
		return err
	}

	_, err := db.Exec(stmt, password, email)
	if err != nil {
		error_msg := fmt.Errorf("failed to update password (SHD_USR_572), stmt:%s, err: %w", stmt, err)
		log.Printf("[req=%s] ***** Alarm:%s", reqID, error_msg.Error())
		return error_msg
	}
	log.Printf("[req=%s] Update password success (SHD_USR_576), email:%s", reqID, email)
	return nil
}
