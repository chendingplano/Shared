package sysdatastores

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/databaseutil"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

func CreateLoginSessionsTable(
	logger *loggerutil.JimoLogger,
	db *sql.DB,
	db_type string,
	table_name string) error {
	logger.Info("Create table", "table_name", table_name)
	var stmt string
	switch db_type {
	case ApiTypes.MysqlName:
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" +
			"login_method VARCHAR(32), " +
			"session_id VARCHAR(256), " +
			"auth_token TEXT, " +
			"status VARCHAR(32) DEFAULT NULL, " +
			"user_name VARCHAR(64) NOT NULL PRIMARY KEY, " +
			"user_name_type VARCHAR(32) DEFAULT NULL, " +
			"user_reg_id VARCHAR(255) DEFAULT NULL, " +
			"user_email VARCHAR(255) DEFAULT NULL, " +
			"expires_at TIMESTAMP NOT NULL, " +
			"created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP, " +
			"INDEX idx_expires (expires_at), " +
			"INDEX idx_session_id (session_id) " +
			") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;"

	case ApiTypes.PgName:
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" +
			"login_method VARCHAR(32), " +
			"session_id VARCHAR(256), " +
			"auth_token TEXT, " +
			"status VARCHAR(32) DEFAULT NULL, " +
			"user_name VARCHAR(64) NOT NULL PRIMARY KEY, " +
			"user_name_type VARCHAR(32) DEFAULT NULL, " +
			"user_reg_id VARCHAR(255) DEFAULT NULL, " +
			"user_email VARCHAR(255) DEFAULT NULL, " +
			"expires_at TIMESTAMP NOT NULL, " +
			"created_at TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW())"

	default:
		err := fmt.Errorf("database type not supported:%s (MID_CTB_117)", db_type)
		log.Printf("***** Alarm:%s", err.Error())
		return err
	}

	err := databaseutil.ExecuteStatement(db, stmt)
	if err != nil {
		err1 := fmt.Errorf("failed creating table '%s' (MID_CTB_124), err: %w, stmt:%s", table_name, err, stmt)
		log.Printf("***** Alarm: %s", err1.Error())
		return err1
	}

	if db_type == ApiTypes.PgName {
		idx1 := `CREATE INDEX IF NOT EXISTS idx_expires ON ` + table_name + ` (expires_at);`
		databaseutil.ExecuteStatement(db, idx1)

		idx2 := `CREATE INDEX IF NOT EXISTS idx_session_id ON ` + table_name + ` (session_id);`
		databaseutil.ExecuteStatement(db, idx2)
	}

	logger.Info("Create table success", "table_name", table_name)
	return nil
}

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
	switch db_type {
	case ApiTypes.MysqlName:
		stmt = fmt.Sprintf(`INSERT INTO %s (login_method, session_id, auth_token, status,
                    user_name, user_name_type, user_reg_id, user_email, expires_at)
              VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
              ON DUPLICATE KEY UPDATE 
              session_id = VALUES(session_id), 
              status = "active",
              expires_at = VALUES(expires_at)`, table_name)
		db = ApiTypes.MySql_DB_miner

	case ApiTypes.PgName:
		stmt = fmt.Sprintf(`INSERT INTO %s (login_method, session_id, auth_token, status,
                    user_name, user_name_type, user_reg_id, user_email, expires_at)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
            ON CONFLICT (user_name)
            DO UPDATE SET
            session_id = EXCLUDED.session_id, 
            status = EXCLUDED.status,
            expires_at = EXCLUDED.expires_at`, table_name)
		db = ApiTypes.PG_DB_miner

	default:
		logger.Error("db_type not supported", "db_type", db_type)
		return fmt.Errorf("unsupported database type (SHD_DBS_234): %s", db_type)
	}

	result, err := db.Exec(stmt, login_method, session_id, auth_token, "active",
		user_name, user_name_type, user_reg_id, user_email, expiry)
	if err != nil {
		logger.Error("failed save session",
			"error", err,
			"session_id", session_id,
			"auth_token", auth_token)
		error_msg := fmt.Sprintf("failed save session (SHD_DBS_208): %v, session_id:%s, auth_token:%s, stmt:%s", err, session_id, auth_token, stmt)
		return fmt.Errorf("%s", error_msg)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		error_msg := fmt.Errorf("failed to get rows affected (SHD_USR_128): %w", err)
		logger.Error("failed to get rows affected", "error", err)
		return error_msg
	}

	if rowsAffected == 0 {
		error_msg := fmt.Errorf("no session record affected (SHD_TLS_134), session_id %s, stmt:%s", session_id, stmt)
		logger.Error("no session record affected",
			"session_id", session_id,
			"auth_token", auth_token,
			"stmt", stmt)
		return error_msg
	}

	logger.Info("saved session", "session_id", session_id, "auth_token", auth_token)

	if !need_update_user {
		return nil
	}

	return UpdateAuthTokenByEmail(rc, user_email, auth_token)
}

func IsValidSession(rc ApiTypes.RequestContext, session_id string) (string, bool, error) {
	// This function checks whether 'session_id' is valid in the sessions table.
	// If valid, return user_name.
	var query string
	var db *sql.DB
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameLoginSessions
	switch db_type {
	case ApiTypes.MysqlName:
		db = ApiTypes.MySql_DB_miner
		query = fmt.Sprintf("SELECT user_name FROM %s WHERE session_id = ? AND expires_at > NOW() LIMIT 1", table_name)

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner
		query = fmt.Sprintf("SELECT user_name FROM %s WHERE session_id = $1 AND expires_at > NOW() LIMIT 1", table_name)

	default:
		error_msg := fmt.Errorf("unsupported database type (SHD_DBS_234): %s", db_type)
		log.Printf("***** Alarm %s:", error_msg.Error())
		return "", false, error_msg
	}

	var user_name string
	err := db.QueryRow(query, session_id).Scan(&user_name)
	if err != nil {
		if err == sql.ErrNoRows {
			error_msg := fmt.Sprintf("user not found:%s (SHD_DBS_333)", user_name)
			log.Printf("%s", error_msg)
			return "", false, nil

		}

		error_msg := fmt.Errorf("failed to validate session (SHD_DBS_240): %w", err)
		log.Printf("***** Alarm:%s", error_msg)
		return "", false, error_msg
	}
	log.Printf("Check session (SHD_DBS_158), stmt: %s, user_name:%s", query, user_name)
	return user_name, user_name != "", nil
}

func IsValidSessionByAuthToken(rc ApiTypes.RequestContext, auth_token string) (string, bool, error) {
	// This function checks whether 'auth_token' is valid in the sessions table.
	// If valid, return user_name.
	var query string
	var db *sql.DB
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameLoginSessions
	switch db_type {
	case ApiTypes.MysqlName:
		db = ApiTypes.MySql_DB_miner
		query = fmt.Sprintf("SELECT user_email FROM %s WHERE auth_token= ? AND expires_at > NOW() LIMIT 1", table_name)

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner
		query = fmt.Sprintf("SELECT user_email FROM %s WHERE auth_token= $1 AND expires_at > NOW() LIMIT 1", table_name)

	default:
		error_msg := fmt.Errorf("unsupported database type (SHD_DBS_180): %s", db_type)
		log.Printf("***** Alarm %s:", error_msg.Error())
		return "", false, error_msg
	}

	var user_email string
	err := db.QueryRow(query, auth_token).Scan(&user_email)
	if err != nil {
		if err == sql.ErrNoRows {
			error_msg := fmt.Sprintf("session not found, auth_token:%s (SHD_DBS_189)", auth_token)
			log.Printf("%s", error_msg)
			return "", false, nil

		}

		error_msg := fmt.Errorf("failed to validate session (SHD_DBS_195): %w", err)
		log.Printf("***** Alarm:%s", error_msg)
		return "", false, error_msg
	}
	log.Printf("Check session success (SHD_DBS_199), stmt: %s, user_email:%s", query, user_email)
	return user_email, user_email != "", nil
}

func DeleteSession(rc ApiTypes.RequestContext, session_id string) error {
	var db *sql.DB
	var stmt string
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameLoginSessions
	switch db_type {
	case ApiTypes.MysqlName:
		db = ApiTypes.MySql_DB_miner
		stmt = fmt.Sprintf("DELETE FROM %s WHERE session_id = ?", table_name)

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner
		stmt = fmt.Sprintf("DELETE FROM %s WHERE session_id = $1", table_name)

	default:
		err := fmt.Errorf("unsupported database type (SHD_DBS_565): %s", db_type)
		log.Printf("***** Alarm:%s", err.Error())
		return err
	}

	_, err := db.Exec(stmt, session_id)
	if err != nil {
		error_msg := fmt.Errorf("failed to delete session (SHD_DBS_771), stmt:%s, session_id:%s, err: %w", stmt, session_id, err)
		log.Printf("***** Alarm:%s", error_msg.Error())
		return error_msg
	}
	log.Printf("Session deleted (SHD_DBS_775), session_id:%s", session_id)
	return nil
}
