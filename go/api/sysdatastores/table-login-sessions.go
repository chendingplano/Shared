package sysdatastores

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/databaseutil"
)

// CreateLoginSessionsTable creates the login sessions table.
// SECURITY: Uses session_id as PRIMARY KEY to allow multiple sessions per user
// (multi-device login). Previous versions used user_name as PK which forced
// single-session-per-user.
func CreateLoginSessionsTable(
	logger ApiTypes.JimoLogger,
	db *sql.DB,
	db_type string,
	table_name string) error {
	logger.Info("Create table", "table_name", table_name)
	var stmt string
	switch db_type {
	case ApiTypes.MysqlName:
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" +
			"session_id VARCHAR(256) NOT NULL PRIMARY KEY, " + // Changed: session_id is now PK
			"login_method VARCHAR(32), " +
			"auth_token TEXT, " +
			"status VARCHAR(32) DEFAULT NULL, " +
			"user_id VARCHAR(64) DEFAULT NULL, " + // Added: user_id for better identification
			"user_name VARCHAR(64) DEFAULT NULL, " + // Changed: no longer PK, can be NULL
			"user_name_type VARCHAR(32) DEFAULT NULL, " +
			"user_reg_id VARCHAR(255) DEFAULT NULL, " +
			"user_email VARCHAR(255) DEFAULT NULL, " +
			"expires_at TIMESTAMP NOT NULL, " +
			"created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP, " +
			"INDEX idx_expires (expires_at), " +
			"INDEX idx_user_id (user_id), " + // Added: index for user lookup
			"INDEX idx_user_email (user_email) " + // Added: index for email lookup
			") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;"

	case ApiTypes.PgName:
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" +
			"session_id VARCHAR(256) NOT NULL PRIMARY KEY, " + // Changed: session_id is now PK
			"login_method VARCHAR(32), " +
			"auth_token TEXT, " +
			"status VARCHAR(32) DEFAULT NULL, " +
			"user_id VARCHAR(64) DEFAULT NULL, " + // Added: user_id for better identification
			"user_name VARCHAR(64) DEFAULT NULL, " + // Changed: no longer PK, can be NULL
			"user_name_type VARCHAR(32) DEFAULT NULL, " +
			"user_reg_id VARCHAR(255) DEFAULT NULL, " +
			"user_email VARCHAR(255) DEFAULT NULL, " +
			"expires_at TIMESTAMP NOT NULL, " +
			"created_at TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW())"

	default:
		err := fmt.Errorf("database type not supported:%s (MID_CTB_117)", db_type)
		return err
	}

	err := databaseutil.ExecuteStatement(db, stmt)
	if err != nil {
		err1 := fmt.Errorf("failed creating table '%s' (MID_CTB_124), err: %w, stmt:%s", table_name, err, stmt)
		return err1
	}

	if db_type == ApiTypes.PgName {
		idx1 := `CREATE INDEX IF NOT EXISTS idx_expires ON ` + table_name + ` (expires_at);`
		databaseutil.ExecuteStatement(db, idx1)

		idx2 := `CREATE INDEX IF NOT EXISTS idx_user_id ON ` + table_name + ` (user_id);`
		databaseutil.ExecuteStatement(db, idx2)

		idx3 := `CREATE INDEX IF NOT EXISTS idx_user_email ON ` + table_name + ` (user_email);`
		databaseutil.ExecuteStatement(db, idx3)
	}

	logger.Info("Create table success", "table_name", table_name)
	return nil
}

// SaveSession creates a new session record.
// SECURITY: Each login creates a NEW session (allows multi-device login).
// Old sessions for the same user are NOT automatically invalidated.
// Use DeleteUserSessions() to invalidate all sessions for a user if needed.
func SaveSession(
	rc ApiTypes.RequestContext,
	login_method string,
	session_id string,
	auth_token string,
	user_name string,
	user_name_type string,
	user_reg_id string,
	user_email string,
	expiry time.Time,
	need_update_user bool) error {
	logger := rc.GetLogger()
	var stmt string
	var db *sql.DB
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameLoginSessions

	// Get user_id if available (for better session tracking)
	var user_id string
	if user_info, exists := rc.GetUserInfoByEmail(user_email); exists && user_info != nil {
		user_id = user_info.UserId
	}

	switch db_type {
	case ApiTypes.MysqlName:
		// Simple INSERT - session_id is PK, so each session is unique
		stmt = fmt.Sprintf(`INSERT INTO %s (session_id, login_method, auth_token, status,
                    user_id, user_name, user_name_type, user_reg_id, user_email, expires_at)
              VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, table_name)
		db = ApiTypes.MySql_DB_miner

	case ApiTypes.PgName:
		// Simple INSERT - session_id is PK, so each session is unique
		stmt = fmt.Sprintf(`INSERT INTO %s (session_id, login_method, auth_token, status,
                    user_id, user_name, user_name_type, user_reg_id, user_email, expires_at)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`, table_name)
		db = ApiTypes.PG_DB_miner

	default:
		logger.Error("db_type not supported", "db_type", db_type)
		return fmt.Errorf("unsupported database type (SHD_DBS_234): %s", db_type)
	}

	result, err := db.Exec(stmt, session_id, login_method, auth_token, "active",
		user_id, user_name, user_name_type, user_reg_id, user_email, expiry)
	if err != nil {
		logger.Error("failed save session",
			"error", err,
			"session_id", ApiUtils.MaskToken(session_id),
			"auth_token", ApiUtils.MaskToken(auth_token))
		error_msg := fmt.Sprintf("failed save session (SHD_DBS_208): %v, session_id:%s",
			err, ApiUtils.MaskToken(session_id))
		return fmt.Errorf("%s", error_msg)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		error_msg := fmt.Errorf("failed to get rows affected (SHD_USR_128): %w", err)
		logger.Error("failed to get rows affected", "error", err)
		return error_msg
	}

	if rowsAffected == 0 {
		error_msg := fmt.Errorf("no session record affected (SHD_TLS_134), session_id %s",
			ApiUtils.MaskToken(session_id))
		logger.Error("no session record affected",
			"session_id", ApiUtils.MaskToken(session_id))
		return error_msg
	}

	logger.Info("saved session",
		"session_id", ApiUtils.MaskToken(session_id),
		"user_email", user_email)

	if !need_update_user {
		return nil
	}

	return UpdateAuthTokenByEmail(rc, user_email, auth_token)
}

// DeleteUserSessions removes all sessions for a given user_id or user_email.
// Use this for "logout from all devices" functionality.
func DeleteUserSessions(rc ApiTypes.RequestContext, user_email string) error {
	var db *sql.DB
	var stmt string
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameLoginSessions
	logger := rc.GetLogger()

	switch db_type {
	case ApiTypes.MysqlName:
		db = ApiTypes.MySql_DB_miner
		stmt = fmt.Sprintf("DELETE FROM %s WHERE user_email = ?", table_name)

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner
		stmt = fmt.Sprintf("DELETE FROM %s WHERE user_email = $1", table_name)

	default:
		err := fmt.Errorf("unsupported database type (SHD_DBS_DEL_001): %s", db_type)
		return err
	}

	result, err := db.Exec(stmt, user_email)
	if err != nil {
		error_msg := fmt.Errorf("failed to delete user sessions (SHD_DBS_DEL_002), email:%s, err: %w",
			user_email, err)
		return error_msg
	}

	rowsDeleted, _ := result.RowsAffected()
	logger.Info("Deleted sessions", "total", rowsDeleted, "email", user_email)
	return nil
}

func DeleteSession(rc ApiTypes.RequestContext, session_id string) error {
	var db *sql.DB
	var stmt string
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameLoginSessions
	logger := rc.GetLogger()

	switch db_type {
	case ApiTypes.MysqlName:
		db = ApiTypes.MySql_DB_miner
		stmt = fmt.Sprintf("DELETE FROM %s WHERE session_id = ?", table_name)

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner
		stmt = fmt.Sprintf("DELETE FROM %s WHERE session_id = $1", table_name)

	default:
		err := fmt.Errorf("unsupported database type (SHD_DBS_565): %s", db_type)
		return err
	}

	_, err := db.Exec(stmt, session_id)
	if err != nil {
		error_msg := fmt.Errorf("failed to delete session (SHD_DBS_771), stmt:%s, session_id:%s, err: %w", stmt, session_id, err)
		return error_msg
	}
	logger.Info("Session deleted", "session_id", session_id)
	return nil
}
