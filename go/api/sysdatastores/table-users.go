package sysdatastores

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/databaseutil"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

// To generate short UUID
// "github.com/lithammer/shortuuid/v4"

var Users_selected_field_names = "id, " +
	"name, password, user_id_type, first_name, last_name, " +
	"email, user_mobile, user_address, verified, admin, " +
	"is_owner, email_visibility, auth_type, user_status, avatar, " +
	"locale, outlook_refresh_token, outlook_access_token, outlook_token_expires_at, " +
	"outlook_sub_id, outlook_sub_expires_at, " +
	"v_token_expires_at, created, updated"

var Users_insert_field_names = "name, " +
	"password, user_id_type, first_name, last_name, " +
	"email, user_mobile, user_address, verified, admin, " +
	"is_owner, email_visibility, auth_type, user_status, avatar, " +
	"locale, outlook_refresh_token, outlook_access_token, outlook_sub_id, " +
	"outlook_sub_expires_at, outlook_token_expires_at, v_token, v_token_expires_at"

func CreateUsersTable(
	logger *loggerutil.JimoLogger,
	db *sql.DB,
	db_type string,
	table_name string) error {

	logger.Info("Create table", "table_name", table_name)

	var stmt string
	fields :=
		"id      VARCHAR(64) PRIMARY KEY DEFAULT gen_random_uuid()::text, " +
			"name      				VARCHAR(128) 	NOT NULL, " +
			"password  				VARCHAR(128) 	DEFAULT NULL, " +
			"user_id_type   		VARCHAR(32)  	DEFAULT NULL, " +
			"first_name      		VARCHAR(128) 	DEFAULT NULL, " +
			"last_name       		VARCHAR(128) 	DEFAULT NULL, " +
			"email          		VARCHAR(255) 	NOT NULL, " +
			"user_mobile    		VARCHAR(64) 	DEFAULT NULL, " +
			"user_address   		TEXT 			DEFAULT NULL, " +
			"verified       		bool 			DEFAULT false, " +
			"admin        			bool 			DEFAULT false, " +
			"is_owner 				bool 			DEFAULT false, " +
			"email_visibility 		bool 			DEFAULT true, " +
			"auth_type      		VARCHAR(32) 	NOT NULL, " +
			"user_status    		VARCHAR(32) 	NOT NULL, " +
			"avatar         		text DEFAULT 	NULL, " +
			"locale         		VARCHAR(128) 	DEFAULT NULL, " +
			"outlook_refresh_token 	VARCHAR(128) 	DEFAULT NULL, " +
			"outlook_access_token 	VARCHAR(128) 	DEFAULT NULL, " +
			"outlook_token_expires_at TIMESTAMP 	DEFAULT NULL, " +
			"outlook_sub_id 		VARCHAR(64) 	DEFAULT NULL, " +
			"outlook_sub_expires_at TIMESTAMP 		DEFAULT NULL, " +
			"v_token      			VARCHAR(128) 	DEFAULT NULL, " +
			"v_token_expires_at		TIMESTAMP 		DEFAULT NULL, " +
			"created        		TIMESTAMP 		DEFAULT CURRENT_TIMESTAMP, " +
			"updated        		TIMESTAMP 		DEFAULT CURRENT_TIMESTAMP "

	switch db_type {
	case ApiTypes.MysqlName:
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" + fields +
			", INDEX idx_created_at (created_at) " +
			") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;"

	case ApiTypes.PgName:
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" + fields + ")"

	default:
		err := fmt.Errorf("database type not supported:%s (SHD_USR_117)", db_type)
		logger.Error("db_type not supported", "db_type", db_type)
		return err
	}

	err := databaseutil.ExecuteStatement(db, stmt)
	if err != nil {
		error_msg := fmt.Errorf("failed creating table (SHD_USR_045), err: %w, stmt:%s", err, stmt)
		logger.Error("failed creating table", "error", err, "stmt", stmt)
		return error_msg
	}

	if db_type == ApiTypes.PgName {
		idx1 := `CREATE INDEX IF NOT EXISTS idx_created_at ON ` + table_name + ` (created_at);`
		databaseutil.ExecuteStatement(db, idx1)

		idx2 := `CREATE UNIQUE INDEX IF NOT EXISTS users_email_unique_lower ON ` + table_name + ` (LOWER(email));`
		databaseutil.ExecuteStatement(db, idx2)
	}

	logger.Info("Create table success", "table_name", table_name)

	return nil
}

func scanUserRecord(
	row *sql.Row,
	user_info *ApiTypes.UserInfo) error {
	// Use sql.NullTime for nullable timestamp columns to handle NULL values
	var outlookTokenExpiresAt, outlookSubExpiresAt, vTokenExpiresAt, created, updated sql.NullTime
	// Use sql.NullString for nullable token columns
	var outlookRefreshToken, outlookAccessToken sql.NullString

	err := row.Scan(
		&user_info.UserId,
		&user_info.UserName,
		&user_info.Password,
		&user_info.UserIdType,
		&user_info.FirstName,
		&user_info.LastName,
		&user_info.Email,
		&user_info.UserMobile,
		&user_info.UserAddress,
		&user_info.Verified,
		&user_info.Admin,
		&user_info.IsOwner,
		&user_info.EmailVisibility,
		&user_info.AuthType,
		&user_info.UserStatus,
		&user_info.Avatar,
		&user_info.Locale,
		&outlookRefreshToken,
		&outlookAccessToken,
		&outlookTokenExpiresAt,
		&user_info.OutlookSubID,
		&outlookSubExpiresAt,
		&vTokenExpiresAt,
		&created,
		&updated,
	)
	if err != nil {
		return err
	}

	// Copy valid OAuth tokens to UserInfo (empty string if NULL)
	if outlookRefreshToken.Valid {
		user_info.OutlookRefreshToken = outlookRefreshToken.String
	}
	if outlookAccessToken.Valid {
		user_info.OutlookAccessToken = outlookAccessToken.String
	}

	// Copy valid timestamps to UserInfo (zero value if NULL)
	if outlookTokenExpiresAt.Valid {
		user_info.OutlookTokenExpiresAt = outlookTokenExpiresAt.Time
	}
	if outlookSubExpiresAt.Valid {
		user_info.OutlookSubExpiresAt = outlookSubExpiresAt.Time
	}
	if vTokenExpiresAt.Valid {
		user_info.VTokenExpiresAt = vTokenExpiresAt.Time
	}
	if created.Valid {
		user_info.Created = created.Time
	}
	if updated.Valid {
		user_info.Updated = updated.Time
	}

	return nil
}

// GetUserInfoByEmail retrieves UserInfo by email.
// IMPORTANT: if the user does not exist, it returns nil, nil
// The caller MUST check whether user_info is valid, even if
// err is nil!!!
func GetUserInfoByEmail(
	rc ApiTypes.RequestContext,
	user_email string) (*ApiTypes.UserInfo, error) {
	logger := rc.GetLogger()
	var query string
	var db *sql.DB
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameUsers
	switch db_type {
	case ApiTypes.MysqlName:
		query = fmt.Sprintf("SELECT %s FROM %s WHERE email = ? LIMIT 1", Users_selected_field_names, table_name)
		db = ApiTypes.MySql_DB_miner

	case ApiTypes.PgName:
		query = fmt.Sprintf("SELECT %s FROM %s WHERE email = $1 LIMIT 1", Users_selected_field_names, table_name)
		db = ApiTypes.PG_DB_miner

	default:
		err := fmt.Errorf("unsupported database type (SHD_USR_326): %s", db_type)
		logger.Error("unsupported db type", "db_type", db_type)
		return nil, err
	}

	row := db.QueryRow(query, user_email)
	user_info := new(ApiTypes.UserInfo)
	err := scanUserRecord(row, user_info)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			logger.Warn("user not found", "email", user_email)
			return nil, nil
		}
		logger.Error("failed scanning user record", "error", err)
		return nil, err
	}

	logger.Info("User info retrieved",
		"status", user_info.UserStatus,
		"is_admin", user_info.Admin,
		"email", user_info.Email)
	return user_info, nil
}

func GetUserInfoByUserID(
	rc ApiTypes.RequestContext,
	user_id string) (*ApiTypes.UserInfo, error) {
	// This function checks whether 'user_email' is used in the users table.
	var query string
	var db *sql.DB
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameUsers
	logger := rc.GetLogger()
	switch db_type {
	case ApiTypes.MysqlName:
		query = fmt.Sprintf("SELECT %s FROM %s WHERE id = ? LIMIT 1", Users_selected_field_names, table_name)
		db = ApiTypes.MySql_DB_miner

	case ApiTypes.PgName:
		query = fmt.Sprintf("SELECT %s FROM %s WHERE id = $1 LIMIT 1", Users_selected_field_names, table_name)
		db = ApiTypes.PG_DB_miner

	default:
		err := fmt.Errorf("unsupported database type (SHD_USR_326): %s", db_type)
		logger.Error("unsupported db_type", "db_type", db_type)
		return nil, err
	}

	row := db.QueryRow(query, user_id)
	user_info := new(ApiTypes.UserInfo)
	err := scanUserRecord(row, user_info)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			logger.Warn("user not found", "user_id", user_id)
			return nil, err
		}
		logger.Error("failed scanning user record", "error", err)
		return nil, err
	}

	logger.Info("User info retrieved",
		"status", user_info.UserStatus,
		"user_id", user_id,
		"email", user_info.Email,
		"is_admin", user_info.Admin)
	return user_info, nil
}

// ErrTokenExpired is returned when a password reset token has expired
var ErrTokenExpired = errors.New("password reset token has expired")

// MigrateUsersTable_AddVTokenExpiresAt adds the v_token_expires_at column
// to existing users tables. This migration is idempotent - safe to run multiple times.
func MigrateUsersTable_AddVTokenExpiresAt(
	logger *loggerutil.JimoLogger,
	db *sql.DB,
	db_type string,
	table_name string) error {
	logger.Info("Running migration: add v_token_expires_at column", "table_name", table_name)

	var stmt string
	switch db_type {
	case ApiTypes.MysqlName:
		// MySQL: Check if column exists before adding
		stmt = fmt.Sprintf(`
			SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
			WHERE TABLE_NAME = '%s' AND COLUMN_NAME = 'v_token_expires_at'
		`, table_name)
		var count int
		err := db.QueryRow(stmt).Scan(&count)
		if err != nil {
			logger.Error("failed to check column existence", "error", err)
			return fmt.Errorf("migration check failed (SHD_MIG_001): %w", err)
		}
		if count > 0 {
			logger.Info("Column v_token_expires_at already exists, skipping migration")
			return nil
		}
		stmt = fmt.Sprintf("ALTER TABLE %s ADD COLUMN v_token_expires_at TIMESTAMP DEFAULT NULL", table_name)

	case ApiTypes.PgName:
		// PostgreSQL: Use IF NOT EXISTS (available in PG 9.6+)
		stmt = fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS v_token_expires_at TIMESTAMP DEFAULT NULL", table_name)

	default:
		err := fmt.Errorf("unsupported database type (SHD_MIG_002): %s", db_type)
		logger.Error("db_type not supported", "db_type", db_type)
		return err
	}

	err := databaseutil.ExecuteStatement(db, stmt)
	if err != nil {
		// For MySQL, check if the error is "duplicate column" (already exists)
		if db_type == ApiTypes.MysqlName && strings.Contains(err.Error(), "Duplicate column") {
			logger.Info("Column v_token_expires_at already exists, skipping")
			return nil
		}
		logger.Error("migration failed", "error", err, "stmt", stmt)
		return fmt.Errorf("migration failed (SHD_MIG_003): %w", err)
	}

	logger.Info("Migration completed: v_token_expires_at column added", "table_name", table_name)
	return nil
}

func GetUserInfoByToken(
	rc ApiTypes.RequestContext,
	token string) (*ApiTypes.UserInfo, error) {
	// This function checks whether 'user_email' is used in the users table.
	var query string
	var db *sql.DB
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameUsers
	logger := rc.GetLogger()
	switch db_type {
	case ApiTypes.MysqlName:
		query = fmt.Sprintf("SELECT %s FROM %s WHERE v_token = ? LIMIT 1", Users_selected_field_names, table_name)
		db = ApiTypes.MySql_DB_miner

	case ApiTypes.PgName:
		query = fmt.Sprintf("SELECT %s FROM %s WHERE v_token = $1 LIMIT 1", Users_selected_field_names, table_name)
		db = ApiTypes.PG_DB_miner

	default:
		err := fmt.Errorf("unsupported database type (SHD_USR_326): %s", db_type)
		logger.Error("db_type not supported", "db_type", db_type)
		return nil, err
	}

	row := db.QueryRow(query, token)

	user_info := new(ApiTypes.UserInfo)
	err := scanUserRecord(row, user_info)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			logger.Warn("no user found", "token", token)
		} else {
			logger.Error("failed scanning user record",
				"error", err,
				"token", token)
		}
		return nil, err
	}

	// SECURITY: Check if token has expired (24-hour validity)
	if !user_info.VTokenExpiresAt.IsZero() && time.Now().After(user_info.VTokenExpiresAt) {
		logger.Warn("password reset token expired",
			"email", user_info.Email,
			"expired_at", user_info.VTokenExpiresAt)
		return nil, ErrTokenExpired
	}

	logger.Info("User info retrieved",
		"status", user_info.UserStatus,
		"token", token,
		"email", user_info.Email,
		"is_admin", user_info.Admin)
	return user_info, nil
}

func GetUserInfoByUserName(
	rc ApiTypes.RequestContext,
	user_name string) (*ApiTypes.UserInfo, error) {
	// This function checks whether 'user_email' is used in the users table.
	var query string
	var db *sql.DB
	db_type := ApiTypes.DatabaseInfo.DBType
	logger := rc.GetLogger()
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameUsers

	switch db_type {
	case ApiTypes.MysqlName:
		query = fmt.Sprintf("SELECT %s FROM %s WHERE name= ? LIMIT 1", Users_selected_field_names, table_name)
		db = ApiTypes.MySql_DB_miner

	case ApiTypes.PgName:
		query = fmt.Sprintf("SELECT %s FROM %s WHERE name= $1 LIMIT 1", Users_selected_field_names, table_name)
		db = ApiTypes.PG_DB_miner

	default:
		err := fmt.Errorf("unsupported database type (SHD_USR_443): %s", db_type)
		logger.Error("db_type not supported", "db_type", db_type)
		return nil, err
	}

	row := db.QueryRow(query, user_name)
	user_info := new(ApiTypes.UserInfo)
	err := scanUserRecord(row, user_info)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			logger.Warn("no user found", "user_name", user_name)
		} else {
			logger.Error("failed scanning user record", "error", err)
		}
		return nil, err
	}

	logger.Info("User info retrieved",
		"status", user_info.UserStatus,
		"user_name", user_name,
		"email", user_info.Email,
		"is_admin", user_info.Admin)
	return user_info, nil
}

func scanUserRecordFromRows(
	rows *sql.Rows,
	user_info *ApiTypes.UserInfo) error {
	// Use sql.NullTime for nullable timestamp columns to handle NULL values
	var outlookTokenExpiresAt, outlookSubExpiresAt, vTokenExpiresAt, created, updated sql.NullTime
	// Use sql.NullString for nullable token columns
	var outlookRefreshToken, outlookAccessToken sql.NullString

	err := rows.Scan(
		&user_info.UserId,
		&user_info.UserName,
		&user_info.Password,
		&user_info.UserIdType,
		&user_info.FirstName,
		&user_info.LastName,
		&user_info.Email,
		&user_info.UserMobile,
		&user_info.UserAddress,
		&user_info.Verified,
		&user_info.Admin,
		&user_info.IsOwner,
		&user_info.EmailVisibility,
		&user_info.AuthType,
		&user_info.UserStatus,
		&user_info.Avatar,
		&user_info.Locale,
		&outlookRefreshToken,
		&outlookAccessToken,
		&outlookTokenExpiresAt,
		&user_info.OutlookSubID,
		&outlookSubExpiresAt,
		&vTokenExpiresAt,
		&created,
		&updated,
	)
	if err != nil {
		return err
	}

	// Copy valid OAuth tokens to UserInfo (empty string if NULL)
	if outlookRefreshToken.Valid {
		user_info.OutlookRefreshToken = outlookRefreshToken.String
	}
	if outlookAccessToken.Valid {
		user_info.OutlookAccessToken = outlookAccessToken.String
	}

	// Copy valid timestamps to UserInfo (zero value if NULL)
	if outlookTokenExpiresAt.Valid {
		user_info.OutlookTokenExpiresAt = outlookTokenExpiresAt.Time
	}
	if outlookSubExpiresAt.Valid {
		user_info.OutlookSubExpiresAt = outlookSubExpiresAt.Time
	}
	if vTokenExpiresAt.Valid {
		user_info.VTokenExpiresAt = vTokenExpiresAt.Time
	}
	if created.Valid {
		user_info.Created = created.Time
	}
	if updated.Valid {
		user_info.Updated = updated.Time
	}

	return nil
}

func GetAllUsers(rc ApiTypes.RequestContext) ([]*ApiTypes.UserInfo, error) {
	var query string
	var db *sql.DB
	var admins []*ApiTypes.UserInfo
	db_type := ApiTypes.DatabaseInfo.DBType
	logger := rc.GetLogger()
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameUsers

	switch db_type {
	case ApiTypes.MysqlName:
		query = fmt.Sprintf("SELECT %s FROM %s", Users_selected_field_names, table_name)
		db = ApiTypes.MySql_DB_miner

	case ApiTypes.PgName:
		query = fmt.Sprintf("SELECT %s FROM %s", Users_selected_field_names, table_name)
		db = ApiTypes.PG_DB_miner

	default:
		err := fmt.Errorf("unsupported database type (SHD_USR_353): %s", db_type)
		logger.Error("db_type not supported", "db_type", db_type)
		return nil, err
	}

	rows, err := db.Query(query)
	if err != nil {
		logger.Error("failed to query admins", "error", err)
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		user_info := new(ApiTypes.UserInfo)
		err := scanUserRecordFromRows(rows, user_info)
		if err != nil {
			logger.Error("failed scanning user record", "error", err)
			return nil, err
		}
		admins = append(admins, user_info)
	}

	if err := rows.Err(); err != nil {
		logger.Error("error iterating rows", "error", err)
		return nil, err
	}

	logger.Info("admins retrieved", "count", len(admins))
	return admins, nil
}

func CountOwners(rc ApiTypes.RequestContext) (int, error) {
	var query string
	var db *sql.DB
	db_type := ApiTypes.DatabaseInfo.DBType
	logger := rc.GetLogger()
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameUsers

	switch db_type {
	case ApiTypes.MysqlName:
		query = fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE is_owner = true", table_name)
		db = ApiTypes.MySql_DB_miner

	case ApiTypes.PgName:
		query = fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE is_owner = true", table_name)
		db = ApiTypes.PG_DB_miner

	default:
		err := fmt.Errorf("unsupported database type (SHD_USR_384): %s", db_type)
		logger.Error("db_type not supported", "db_type", db_type)
		return 0, err
	}

	var count int
	err := db.QueryRow(query).Scan(&count)
	if err != nil {
		logger.Error("failed to count owners", "error", err)
		return 0, err
	}

	logger.Info("owners counted", "count", count)
	return count, nil
}

func GetAllAdmins(rc ApiTypes.RequestContext, is_admin bool) ([]*ApiTypes.UserInfo, error) {
	var query string
	var db *sql.DB
	var admins []*ApiTypes.UserInfo
	db_type := ApiTypes.DatabaseInfo.DBType
	logger := rc.GetLogger()
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameUsers

	switch db_type {
	case ApiTypes.MysqlName:
		query = fmt.Sprintf("SELECT %s FROM %s WHERE admin = ?", Users_selected_field_names, table_name)
		db = ApiTypes.MySql_DB_miner

	case ApiTypes.PgName:
		query = fmt.Sprintf("SELECT %s FROM %s WHERE admin = $1", Users_selected_field_names, table_name)
		db = ApiTypes.PG_DB_miner

	default:
		err := fmt.Errorf("unsupported database type (SHD_USR_322): %s", db_type)
		logger.Error("db_type not supported", "db_type", db_type)
		return nil, err
	}

	rows, err := db.Query(query, is_admin)
	if err != nil {
		logger.Error("failed to query admins", "error", err, "is_admin", is_admin, "stmt", query)
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		user_info := new(ApiTypes.UserInfo)
		err := scanUserRecordFromRows(rows, user_info)
		if err != nil {
			logger.Error("failed scanning user record", "error", err, "is_admin", is_admin)
			return nil, err
		}
		admins = append(admins, user_info)
	}

	if err := rows.Err(); err != nil {
		logger.Error("error iterating rows", "error", err, "is_admin", is_admin)
		return nil, err
	}

	logger.Info("admins retrieved", "count", len(admins), "is_admin", is_admin)
	return admins, nil
}

func GetAllOwners(rc ApiTypes.RequestContext) ([]*ApiTypes.UserInfo, error) {
	var query string
	var db *sql.DB
	var owners []*ApiTypes.UserInfo
	db_type := ApiTypes.DatabaseInfo.DBType
	logger := rc.GetLogger()
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameUsers

	switch db_type {
	case ApiTypes.MysqlName:
		query = fmt.Sprintf("SELECT %s FROM %s WHERE is_owner = true", Users_selected_field_names, table_name)
		db = ApiTypes.MySql_DB_miner

	case ApiTypes.PgName:
		query = fmt.Sprintf("SELECT %s FROM %s WHERE is_owner = true", Users_selected_field_names, table_name)
		db = ApiTypes.PG_DB_miner

	default:
		err := fmt.Errorf("unsupported database type (SHD_USR_486): %s", db_type)
		logger.Error("db_type not supported", "db_type", db_type)
		return nil, err
	}

	rows, err := db.Query(query)
	if err != nil {
		logger.Error("failed to query owners", "error", err)
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		user_info := new(ApiTypes.UserInfo)
		err := scanUserRecordFromRows(rows, user_info)
		if err != nil {
			logger.Error("failed scanning user record", "error", err)
			return nil, err
		}
		owners = append(owners, user_info)
	}

	if err := rows.Err(); err != nil {
		logger.Error("error iterating rows", "error", err)
		return nil, err
	}

	logger.Info("owners retrieved", "count", len(owners))
	return owners, nil
}

func UpsertUser(
	rc ApiTypes.RequestContext,
	user_info *ApiTypes.UserInfo) error {
	logger := rc.GetLogger()
	var db *sql.DB
	var insert_stmt string
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameUsers
	switch db_type {
	case ApiTypes.MysqlName:
		err := fmt.Errorf("mysql not supported yet")
		logger.Error("mysql not supported yet")
		return err

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner
		insert_stmt = fmt.Sprintf("INSERT INTO %s (%s) VALUES ("+
			"$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, "+
			"$11, $12, $13, $14, $15, $16, $17, $18, $19, $20, "+
			"$21, $22, $23) "+
			"ON CONFLICT (LOWER(email)) DO UPDATE SET v_token = EXCLUDED.v_token "+
			"RETURNING %s",
			table_name, Users_insert_field_names, Users_selected_field_names)

	default:
		err := fmt.Errorf("unsupported database type (SHD_USR_313): %s", db_type)
		logger.Error("unsupported database type", "db_type", db_type)
		return err
	}

	// Prepare the arguments in the same order as the field names
	args := []interface{}{
		user_info.UserName,
		user_info.Password,
		user_info.UserIdType,
		user_info.FirstName,
		user_info.LastName,
		user_info.Email,
		user_info.UserMobile,
		user_info.UserAddress,
		user_info.Verified,
		user_info.Admin,
		user_info.IsOwner,
		user_info.EmailVisibility,
		user_info.AuthType,
		user_info.UserStatus,
		user_info.Avatar,
		user_info.Locale,
		user_info.OutlookRefreshToken, // write-only (not read back for security)
		user_info.OutlookAccessToken,  // write-only (not read back for security)
		user_info.OutlookSubID,
		user_info.OutlookSubExpiresAt,
		user_info.OutlookTokenExpiresAt,
		user_info.VToken,            // write-only (not read back for security)
		user_info.VTokenExpiresAt,
	}

	row := db.QueryRow(insert_stmt, args...)
	var new_user_info ApiTypes.UserInfo
	err := scanUserRecord(row, &new_user_info)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			logger.Error("no user found")
		} else {
			logger.Error("scan error", "error", err)
		}

		error_msg := fmt.Sprintf("failed to scan user record (SHD_USR_213): %v, stmt:%s", err, insert_stmt)
		logger.Error("failed to scan user record",
			"error", err,
			"id", user_info.UserId,
			"email", user_info.Email,
			"stmt", insert_stmt)
		return fmt.Errorf("***** Alarm:%s", error_msg)
	}

	user_info.UserId = new_user_info.UserId

	// Check all other fields (except Created and Updated)
	//  - if its values in new_user_info and user_info are the same, ignore
	//  - if the value in new_user_info is empty but not empty in user_info,
	//	  update the field using user_info's value
	//  - otherwise, collect the error info
	// Update the record as needed
	// Report the errors, if any

	var fieldsToUpdate []string
	var updateArgs []interface{}
	var conflicts []string
	paramIndex := 1

	// Helper to check string fields
	checkStringField := func(fieldName string, dbVal, inputVal string, read_only bool) {
		// - If both are empty, do nothing
		// - If inputVal is not empty:
		//	 - If it is not immutable, override
		//	 - Otherwise, it is a conflict
		if dbVal == inputVal {
			return
		}

		if inputVal != "" {
			if dbVal == "" || !read_only {
				fieldsToUpdate = append(fieldsToUpdate, fmt.Sprintf("%s = $%d", fieldName, paramIndex))
				updateArgs = append(updateArgs, inputVal)
				paramIndex++
			} else if dbVal != inputVal {
				conflicts = append(conflicts, fmt.Sprintf("%s: db=%q, input=%q", fieldName, dbVal, inputVal))
			}
		}
	}

	// Helper to check bool fields
	checkBoolField := func(fieldName string, dbVal, inputVal bool) {
		if dbVal == inputVal {
			return
		}
		fieldsToUpdate = append(fieldsToUpdate, fmt.Sprintf("%s = $%d", fieldName, paramIndex))
		updateArgs = append(updateArgs, inputVal)
		paramIndex++
	}

	// Helper to check time fields
	checkTimeField := func(fieldName string, dbVal, inputVal time.Time, read_only bool) {
		if dbVal.Equal(inputVal) {
			return
		}
		if dbVal.IsZero() && !inputVal.IsZero() {
			if dbVal.IsZero() || !read_only {
				fieldsToUpdate = append(fieldsToUpdate, fmt.Sprintf("%s = $%d", fieldName, paramIndex))
				updateArgs = append(updateArgs, inputVal)
				paramIndex++
			} else if !dbVal.Equal(inputVal) {
				conflicts = append(conflicts, fmt.Sprintf("%s: db=%v, input=%v", fieldName, dbVal, inputVal))
			}
		}
	}

	// Check string fields
	checkStringField("name", new_user_info.UserName, user_info.UserName, false)
	checkStringField("password", new_user_info.Password, user_info.Password, false)
	checkStringField("user_id_type", new_user_info.UserIdType, user_info.UserIdType, true)
	checkStringField("first_name", new_user_info.FirstName, user_info.FirstName, false)
	checkStringField("last_name", new_user_info.LastName, user_info.LastName, false)
	checkStringField("email", new_user_info.Email, user_info.Email, false)
	checkStringField("user_mobile", new_user_info.UserMobile, user_info.UserMobile, false)
	checkStringField("user_address", new_user_info.UserAddress, user_info.UserAddress, false)
	checkStringField("auth_type", new_user_info.AuthType, user_info.AuthType, true)
	checkStringField("user_status", new_user_info.UserStatus, user_info.UserStatus, false)
	checkStringField("avatar", new_user_info.Avatar, user_info.Avatar, false)
	checkStringField("locale", new_user_info.Locale, user_info.Locale, false)
	checkStringField("outlook_sub_id", new_user_info.OutlookSubID, user_info.OutlookSubID, false)

	// Check bool fields
	checkBoolField("verified", new_user_info.Verified, user_info.Verified)
	checkBoolField("admin", new_user_info.Admin, user_info.Admin)
	checkBoolField("is_owner", new_user_info.IsOwner, user_info.IsOwner)
	checkBoolField("email_visibility", new_user_info.EmailVisibility, user_info.EmailVisibility)

	// Check time fields (excluding Created and Updated)
	checkTimeField("outlook_token_expires_at", new_user_info.OutlookTokenExpiresAt, user_info.OutlookTokenExpiresAt, false)
	checkTimeField("outlook_sub_expires_at", new_user_info.OutlookSubExpiresAt, user_info.OutlookSubExpiresAt, false)

	// Report conflicts if any
	if len(conflicts) > 0 {
		for _, conflict := range conflicts {
			logger.Error("field conflict detected", "conflict", conflict, "user_id", user_info.UserId)
		}
	}

	// Execute update if there are fields to update
	if len(fieldsToUpdate) > 0 {
		update_stmt := fmt.Sprintf("UPDATE %s SET %s, updated = CURRENT_TIMESTAMP WHERE id = $%d",
			table_name,
			strings.Join(fieldsToUpdate, ", "),
			paramIndex)
		updateArgs = append(updateArgs, user_info.UserId)

		_, err := db.Exec(update_stmt, updateArgs...)
		if err != nil {
			logger.Error("failed to update user record",
				"error", err,
				"user_id", user_info.UserId,
				"stmt", update_stmt)
			return fmt.Errorf("failed to update user record (SHD_USR_390): %w", err)
		}
		logger.Info("user record updated",
			"user_id", user_info.UserId,
			"fields_updated", fieldsToUpdate)
	}

	logger.Info("upsert user success", "token", user_info.VToken)
	return nil
}

func MarkUserVerified(
	rc ApiTypes.RequestContext,
	user_name string) error {
	var db *sql.DB
	var stmt string
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameUsers
	logger := rc.GetLogger()
	switch db_type {
	case ApiTypes.MysqlName:
		db = ApiTypes.MySql_DB_miner
		stmt = fmt.Sprintf("UPDATE %s SET user_status = 'active', verified = true WHERE name = ?", table_name)

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner
		stmt = fmt.Sprintf("UPDATE %s SET user_status = 'active', verified = true WHERE name = $1", table_name)

	default:
		err := fmt.Errorf("unsupported database type (SHD_USR_401): %s", db_type)
		logger.Error("db_type not supported", "db_type", db_type)
		return err
	}

	_, err := db.Exec(stmt, user_name)
	if err != nil {
		error_msg := fmt.Errorf("failed to update table (SHD_USR_404), stmt:%s, err: %w", stmt, err)
		logger.Error("failed to update user", "error", err, "stmt", stmt)
		return error_msg
	}
	logger.Info("Mark user verified success",
		"user_name", user_name)
	return nil
}

func UpdatePasswordByUserName(
	rc ApiTypes.RequestContext,
	user_name string, password string) error {
	var db *sql.DB
	var stmt string
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameUsers
	logger := rc.GetLogger()
	switch db_type {
	case ApiTypes.MysqlName:
		db = ApiTypes.MySql_DB_miner
		stmt = fmt.Sprintf("UPDATE %s SET password = ?, user_status = 'active' WHERE name = ?", table_name)

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner
		stmt = fmt.Sprintf("UPDATE %s SET password = $1, user_status = 'active' WHERE name = $2", table_name)

	default:
		err := fmt.Errorf("unsupported database type (SHD_USR_565): %s", db_type)
		logger.Error("db_type not supported", "db_type", db_type)
		return err
	}

	_, err := db.Exec(stmt, password, user_name)
	if err != nil {
		error_msg := fmt.Errorf("failed to update password (SHD_USR_572), stmt:%s, err: %w", stmt, err)
		logger.Error("failed to update password", "error", err, "stmt", stmt)
		return error_msg
	}
	logger.Info("Update password success", "user_name", user_name)
	return nil
}

func UpdatePasswordByEmail(
	rc ApiTypes.RequestContext,
	email string, password string) error {
	var db *sql.DB
	var stmt string
	logger := rc.GetLogger()
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameUsers
	switch db_type {
	case ApiTypes.MysqlName:
		db = ApiTypes.MySql_DB_miner
		stmt = fmt.Sprintf("UPDATE %s SET password = ?, user_status = 'active' WHERE email = ?", table_name)

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner
		stmt = fmt.Sprintf("UPDATE %s SET password = $1, user_status = 'active' WHERE email = $2", table_name)

	default:
		err := fmt.Errorf("unsupported database type (SHD_USR_565): %s", db_type)
		logger.Error("db_type not supported", "db_type", db_type)
		return err
	}

	_, err := db.Exec(stmt, password, email)
	if err != nil {
		error_msg := fmt.Errorf("failed to update password (SHD_USR_572), stmt:%s, err: %w", stmt, err)
		logger.Error("failed to update password", "error", err, "stmt", stmt)
		return error_msg
	}
	logger.Info("Update password success", "email", email)
	return nil
}

func UpdateAuthTokenByEmail(
	rc ApiTypes.RequestContext,
	email string,
	auth_token string) error {
	var db *sql.DB
	var stmt string
	logger := rc.GetLogger()
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameUsers
	switch db_type {
	case ApiTypes.MysqlName:
		db = ApiTypes.MySql_DB_miner
		stmt = fmt.Sprintf("UPDATE %s SET v_token= ? WHERE email = ?", table_name)

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner
		stmt = fmt.Sprintf("UPDATE %s SET v_token= $1 WHERE email = $2", table_name)

	default:
		err := fmt.Errorf("unsupported database type (SHD_USR_495): %s", db_type)
		logger.Error("unsupported database type", "db_type", db_type)
		return err
	}

	result, err := db.Exec(stmt, auth_token, email)
	if err != nil {
		error_msg := fmt.Errorf("failed to update auth token (SHD_USR_502), stmt:%s, err: %w", stmt, err)
		logger.Error("failed to update auth token", "stmt", stmt, "error", err)
		return error_msg
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		error_msg := fmt.Errorf("failed to get rows affected (SHD_USR_503): %w", err)
		logger.Error("failed to get rows affected", "error", err)
		return error_msg
	}
	if rowsAffected == 0 {
		error_msg := fmt.Errorf("no user found with email (SHD_USR_504): %s", email)
		logger.Error("no user found with email", "email", email)
		return error_msg
	}
	logger.Info("Update auth token success", "email", email, "token", ApiUtils.MaskToken(auth_token))
	return nil
}

// ClearVTokenByEmail clears the verification/reset token after successful use.
// SECURITY: This prevents token reuse attacks.
func ClearVTokenByEmail(
	rc ApiTypes.RequestContext,
	email string) error {
	var db *sql.DB
	var stmt string
	logger := rc.GetLogger()
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameUsers

	switch db_type {
	case ApiTypes.MysqlName:
		db = ApiTypes.MySql_DB_miner
		stmt = fmt.Sprintf("UPDATE %s SET v_token = NULL, v_token_expires_at = NULL, updated = CURRENT_TIMESTAMP WHERE email = ?", table_name)

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner
		stmt = fmt.Sprintf("UPDATE %s SET v_token = NULL, v_token_expires_at = NULL, updated = CURRENT_TIMESTAMP WHERE email = $1", table_name)

	default:
		err := fmt.Errorf("unsupported database type (SHD_USR_CLR_001): %s", db_type)
		logger.Error("unsupported database type", "db_type", db_type)
		return err
	}

	_, err := db.Exec(stmt, email)
	if err != nil {
		logger.Error("failed to clear v_token", "email", email, "error", err)
		return fmt.Errorf("failed to clear v_token (SHD_USR_CLR_002): %w", err)
	}

	logger.Info("Cleared v_token after successful use", "email", email)
	return nil
}

// RefreshTokenKey generates a new random token key for the user and updates it in the database.
// This invalidates all existing JWT sessions for the user.
func RefreshTokenKey(
	rc ApiTypes.RequestContext,
	userID string) error {
	var db *sql.DB
	var stmt string
	logger := rc.GetLogger()
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameUsers

	// Generate a random 32-byte token key
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		logger.Error("failed to generate random token key", "error", err)
		return fmt.Errorf("failed to generate token key (SHD_USR_810): %w", err)
	}
	newTokenKey := base64.URLEncoding.EncodeToString(tokenBytes)

	switch db_type {
	case ApiTypes.MysqlName:
		db = ApiTypes.MySql_DB_miner
		stmt = fmt.Sprintf("UPDATE %s SET v_token = ?, updated = CURRENT_TIMESTAMP WHERE id = ?", table_name)

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner
		stmt = fmt.Sprintf("UPDATE %s SET v_token = $1, updated = CURRENT_TIMESTAMP WHERE id = $2", table_name)

	default:
		err := fmt.Errorf("unsupported database type (SHD_USR_820): %s", db_type)
		logger.Error("unsupported database type", "db_type", db_type)
		return err
	}

	result, err := db.Exec(stmt, newTokenKey, userID)
	if err != nil {
		error_msg := fmt.Errorf("failed to refresh token key (SHD_USR_830), stmt:%s, err: %w", stmt, err)
		logger.Error("failed to refresh token key", "stmt", stmt, "error", err)
		return error_msg
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		error_msg := fmt.Errorf("failed to get rows affected (SHD_USR_835): %w", err)
		logger.Error("failed to get rows affected", "error", err)
		return error_msg
	}
	if rowsAffected == 0 {
		error_msg := fmt.Errorf("no user found with id (SHD_USR_840): %s", userID)
		logger.Error("no user found with id", "user_id", userID)
		return error_msg
	}
	logger.Info("Token key refreshed successfully", "user_id", userID)
	return nil
}

// UpdateUserProfile updates the user's profile fields (firstName, lastName, avatar).
// It returns the updated UserInfo.
func UpdateUserProfile(
	rc ApiTypes.RequestContext,
	userID string,
	firstName string,
	lastName string,
	avatar string) (*ApiTypes.UserInfo, error) {
	var db *sql.DB
	var stmt string
	logger := rc.GetLogger()
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameUsers

	switch db_type {
	case ApiTypes.MysqlName:
		db = ApiTypes.MySql_DB_miner
		stmt = fmt.Sprintf("UPDATE %s SET first_name = ?, last_name = ?, avatar = ?, updated = CURRENT_TIMESTAMP WHERE id = ?", table_name)

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner
		stmt = fmt.Sprintf("UPDATE %s SET first_name = $1, last_name = $2, avatar = $3, updated = CURRENT_TIMESTAMP WHERE id = $4", table_name)

	default:
		err := fmt.Errorf("unsupported database type (SHD_USR_650): %s", db_type)
		logger.Error("unsupported database type", "db_type", db_type)
		return nil, err
	}

	result, err := db.Exec(stmt, firstName, lastName, avatar, userID)
	if err != nil {
		error_msg := fmt.Errorf("failed to update user profile (SHD_USR_658), stmt:%s, err: %w", stmt, err)
		logger.Error("failed to update user profile", "error", err, "stmt", stmt)
		return nil, error_msg
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		error_msg := fmt.Errorf("failed to get rows affected (SHD_USR_665): %w", err)
		logger.Error("failed to get rows affected", "error", err)
		return nil, error_msg
	}

	if rowsAffected == 0 {
		error_msg := fmt.Errorf("no user found with id (SHD_USR_671): %s", userID)
		logger.Error("no user found with id", "user_id", userID)
		return nil, error_msg
	}

	// Fetch the updated user record
	user_info, err := GetUserInfoByUserID(rc, userID)
	if err != nil {
		error_msg := fmt.Errorf("failed to fetch updated user (SHD_USR_679): %w", err)
		logger.Error("failed to fetch updated user", "error", err, "user_id", userID)
		return nil, error_msg
	}

	logger.Info("Update user profile success",
		"user_id", userID,
		"first_name", firstName,
		"last_name", lastName)
	return user_info, nil
}
